package devserver

import (
	"context"
	"fmt"
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
}

// Process supervises the child development server.
type Process struct {
	config ChildConfig

	mu      sync.Mutex
	cmd     *exec.Cmd
	exitCh  chan error
	running bool
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
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start child dev server: %w", err)
	}
	p.mu.Lock()
	p.cmd = cmd
	p.running = true
	exitCh := p.exitCh
	p.mu.Unlock()

	go func() {
		waitErr := cmd.Wait()

		p.mu.Lock()
		p.running = false
		p.cmd = nil
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

func (p *Process) Stop(gracePeriod time.Duration) error {
	p.mu.Lock()
	cmd := p.cmd
	exitCh := p.exitCh
	running := p.running
	p.mu.Unlock()

	if !running || cmd == nil || cmd.Process == nil {
		return nil
	}
	if gracePeriod <= 0 {
		gracePeriod = 5 * time.Second
	}

	_ = interruptChild(cmd)

	select {
	case <-time.After(gracePeriod):
		_ = killChild(cmd)
		<-exitCh
	case <-exitCh:
	}

	return nil
}
