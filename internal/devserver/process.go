package devserver

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"
)

// ChildConfig configures the child development server process.
type ChildConfig struct {
	Args     []string
	Command  string
	CWD      string
	Env      map[string]string
	ReadyURL string

	// Stdout and Stderr receive the child's output. Nil means the child
	// inherits nimbu's own stdout/stderr (raw passthrough).
	Stdout io.Writer
	Stderr io.Writer
}

// Process supervises the child development server.
type Process struct {
	config ChildConfig

	mu      sync.Mutex
	cmd     *exec.Cmd
	group   *childGroup
	exitCh  chan error
	running bool
	// exited is set once the direct child is gone while the wait goroutine
	// still reaps what it left in its group; running stays true until then.
	exited bool
}

func NewProcess(config ChildConfig) *Process {
	return &Process{config: config, exitCh: make(chan error, 1)}
}

func (p *Process) Start() error {
	if p.config.Command == "" {
		return fmt.Errorf("child command is required")
	}
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return fmt.Errorf("child dev server already running")
	}
	p.exitCh = make(chan error, 1)
	p.mu.Unlock()

	cmd := exec.Command(p.config.Command, p.config.Args...)
	configureChildProcess(cmd)
	if p.config.CWD != "" {
		cmd.Dir = p.config.CWD
	}

	env := os.Environ()
	for key, value := range p.config.Env {
		env = append(env, key+"="+value)
	}
	cmd.Env = env

	output := &childOutput{}
	stdout, err := output.attach(p.config.Stdout, os.Stdout)
	if err != nil {
		return fmt.Errorf("connect child stdout: %w", err)
	}
	stderr, err := output.attach(p.config.Stderr, os.Stderr)
	if err != nil {
		output.abort()
		return fmt.Errorf("connect child stderr: %w", err)
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		output.abort()
		return fmt.Errorf("start child dev server: %w", err)
	}
	output.started()
	group := attachChildGroup(cmd)
	p.mu.Lock()
	p.cmd = cmd
	p.group = group
	p.running = true
	exitCh := p.exitCh
	p.mu.Unlock()

	go func() {
		waitErr := cmd.Wait()

		p.mu.Lock()
		p.exited = true
		p.mu.Unlock()
		releaseChildGroup(group)
		// Reaping the group closed every other writer, so this is quick.
		output.drain(outputDrainDelay)

		p.mu.Lock()
		p.running = false
		p.exited = false
		p.cmd = nil
		p.group = nil
		p.mu.Unlock()
		exitCh <- waitErr
		close(exitCh)
	}()

	return nil
}

func (p *Process) ExitCh() <-chan error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.exitCh
}

func (p *Process) WaitReady(ctx context.Context, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	readyCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	p.mu.Lock()
	running := p.running
	exitCh := p.exitCh
	p.mu.Unlock()
	if !running {
		return fmt.Errorf("child process is not running")
	}

	if p.config.ReadyURL == "" {
		ticker := time.NewTimer(1 * time.Second)
		defer ticker.Stop()

		select {
		case <-readyCtx.Done():
			return fmt.Errorf("child readiness timeout: %w", readyCtx.Err())
		case err := <-exitCh:
			if err == nil {
				return fmt.Errorf("child exited before ready")
			}
			return fmt.Errorf("child exited before ready: %w", err)
		case <-ticker.C:
			return nil
		}
	}

	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-readyCtx.Done():
			return fmt.Errorf("child readiness timeout for %s: %w", p.config.ReadyURL, readyCtx.Err())
		case err := <-exitCh:
			if err == nil {
				return fmt.Errorf("child exited before ready")
			}
			return fmt.Errorf("child exited before ready: %w", err)
		case <-ticker.C:
			req, err := http.NewRequestWithContext(readyCtx, http.MethodGet, p.config.ReadyURL, nil)
			if err != nil {
				return fmt.Errorf("build readiness request: %w", err)
			}

			resp, err := client.Do(req)
			if err != nil {
				continue
			}
			_ = resp.Body.Close()
			if resp.StatusCode < 500 {
				return nil
			}
		}
	}
}

// Stop asks the child's process tree to exit and force-kills it after
// gracePeriod. If the interrupt cannot be delivered at all, it kills at once
// instead of waiting out the grace period.
func (p *Process) Stop(gracePeriod time.Duration) error {
	p.mu.Lock()
	cmd := p.cmd
	group := p.group
	exitCh := p.exitCh
	running := p.running
	exited := p.exited
	p.mu.Unlock()

	if !running || cmd == nil || cmd.Process == nil {
		return nil
	}
	if exited {
		// The child is gone; its leftovers are being reaped (bounded by
		// leftoverGracePeriod), so returning now could leave one running.
		<-exitCh
		return nil
	}
	if gracePeriod <= 0 {
		gracePeriod = 5 * time.Second
	}

	if err := interruptChild(cmd, group); err != nil {
		_ = killChild(cmd, group)
		<-exitCh
		return nil
	}

	timer := time.NewTimer(gracePeriod)
	defer timer.Stop()
	select {
	case <-timer.C:
		_ = killChild(cmd, group)
		<-exitCh
	case <-exitCh:
	}

	return nil
}

// Kill force-kills the child's whole process tree immediately and waits up to
// timeout for it to be reaped. It is the second-Ctrl-C path.
func (p *Process) Kill(timeout time.Duration) error {
	p.mu.Lock()
	cmd := p.cmd
	group := p.group
	exitCh := p.exitCh
	running := p.running
	exited := p.exited
	p.mu.Unlock()

	if !running || cmd == nil || cmd.Process == nil {
		return nil
	}
	if exited {
		// Skip the leftovers' grace period; the child's PID may be reused,
		// so only the group is targeted.
		killLeftovers(group)
	} else if err := killChild(cmd, group); err != nil {
		return fmt.Errorf("kill child dev server: %w", err)
	}
	if timeout <= 0 {
		return nil
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-exitCh:
	case <-timer.C:
		return fmt.Errorf("child dev server did not exit within %s after kill", timeout)
	}
	return nil
}
