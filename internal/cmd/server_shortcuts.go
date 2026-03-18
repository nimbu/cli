package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/muesli/cancelreader"
)

type serverShortcutLinks struct {
	DevURL   string
	LiveURL  string
	AdminURL string
}

func serverShortcutLinksFromSummary(summary serverSummary) serverShortcutLinks {
	devURL := displayServerURL(summary.ReadyURL)
	liveURL := displaySiteURL(summary.SiteHost)
	adminURL := ""
	if liveURL != "" {
		adminURL = strings.TrimRight(liveURL, "/") + "/admin"
	}

	return serverShortcutLinks{
		DevURL:   devURL,
		LiveURL:  liveURL,
		AdminURL: adminURL,
	}
}

func (l serverShortcutLinks) Hint() string {
	parts := make([]string, 0, 3)
	if l.DevURL != "" {
		parts = append(parts, "[o] open dev")
	}
	if l.LiveURL != "" {
		parts = append(parts, "[l] open live")
	}
	if l.AdminURL != "" {
		parts = append(parts, "[b] open admin")
	}
	return strings.Join(parts, "  ")
}

func (l serverShortcutLinks) target(action serverShortcutAction) (label string, target string, ok bool) {
	switch action {
	case serverShortcutOpenDev:
		if l.DevURL == "" {
			return "", "", false
		}
		return "dev", l.DevURL, true
	case serverShortcutOpenLive:
		if l.LiveURL == "" {
			return "", "", false
		}
		return "live", l.LiveURL, true
	case serverShortcutOpenAdmin:
		if l.AdminURL == "" {
			return "", "", false
		}
		return "admin", l.AdminURL, true
	default:
		return "", "", false
	}
}

type serverShortcutAction uint8

const (
	serverShortcutNone serverShortcutAction = iota
	serverShortcutLogMarker
	serverShortcutOpenDev
	serverShortcutOpenLive
	serverShortcutOpenAdmin
)

type serverShortcutDecision struct {
	Action  serverShortcutAction
	Pending bool
}

func decideServerShortcut(b byte, ready bool, links serverShortcutLinks) serverShortcutDecision {
	if b >= 'A' && b <= 'Z' {
		b += 'a' - 'A'
	}

	action := serverShortcutNone
	switch b {
	case '\n', '\r':
		action = serverShortcutLogMarker
	case 'o':
		if links.DevURL != "" {
			action = serverShortcutOpenDev
		}
	case 'l':
		if links.LiveURL != "" {
			action = serverShortcutOpenLive
		}
	case 'b':
		if links.AdminURL != "" {
			action = serverShortcutOpenAdmin
		}
	}

	if action == serverShortcutNone {
		return serverShortcutDecision{}
	}
	if action == serverShortcutLogMarker {
		return serverShortcutDecision{Action: action}
	}
	if !ready {
		return serverShortcutDecision{Action: action, Pending: true}
	}
	return serverShortcutDecision{Action: action}
}

type serverShortcutListener struct {
	closeOnce sync.Once
	errCh     chan error
	events    chan byte
	input     *os.File
	ownsInput bool
	reader    cancelreader.CancelReader
	restore   func() error
}

func newServerShortcutListener() (*serverShortcutListener, error) {
	input, ownsInput, err := openServerShortcutInput()
	if err != nil {
		return nil, fmt.Errorf("open shortcut input: %w", err)
	}

	restore, err := prepareServerShortcutInput(input)
	if err != nil {
		if ownsInput {
			_ = input.Close()
		}
		return nil, fmt.Errorf("enable shortcut input: %w", err)
	}

	reader, err := cancelreader.NewReader(input)
	if err != nil {
		if restore != nil {
			_ = restore()
		}
		if ownsInput {
			_ = input.Close()
		}
		return nil, fmt.Errorf("prepare shortcut input: %w", err)
	}

	listener := &serverShortcutListener{
		errCh:     make(chan error, 1),
		events:    make(chan byte, 16),
		input:     input,
		ownsInput: ownsInput,
		reader:    reader,
		restore:   restore,
	}
	go listener.readLoop()
	return listener, nil
}

func (l *serverShortcutListener) readLoop() {
	defer close(l.events)
	defer close(l.errCh)

	var buf [1]byte
	for {
		n, err := l.reader.Read(buf[:])
		if err != nil {
			if errors.Is(err, cancelreader.ErrCanceled) || errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) {
				return
			}
			l.errCh <- err
			return
		}
		if n == 1 {
			select {
			case l.events <- buf[0]:
			default:
			}
		}
	}
}

func (l *serverShortcutListener) Events() <-chan byte {
	if l == nil {
		return nil
	}
	return l.events
}

func (l *serverShortcutListener) Errors() <-chan error {
	if l == nil {
		return nil
	}
	return l.errCh
}

func (l *serverShortcutListener) Close() error {
	if l == nil {
		return nil
	}

	var closeErr error
	l.closeOnce.Do(func() {
		if l.reader != nil {
			_ = l.reader.Cancel()
			if err := l.reader.Close(); err != nil && !errors.Is(err, cancelreader.ErrCanceled) {
				closeErr = err
			}
		}
		if l.restore != nil {
			if err := l.restore(); err != nil && closeErr == nil {
				closeErr = err
			}
		}
		if l.ownsInput && l.input != nil {
			if err := l.input.Close(); err != nil && closeErr == nil {
				closeErr = err
			}
		}
	})
	return closeErr
}

func serverBrowserCommand(goos string, target string) (string, []string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", nil, fmt.Errorf("empty browser target")
	}

	switch goos {
	case "darwin":
		return "open", []string{target}, nil
	case "linux":
		return "xdg-open", []string{target}, nil
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", target}, nil
	default:
		return "", nil, fmt.Errorf("unsupported platform %q", goos)
	}
}

func openServerBrowserURL(target string, runner func(string, ...string) error) error {
	command, args, err := serverBrowserCommand(runtime.GOOS, target)
	if err != nil {
		return err
	}
	if runner == nil {
		runner = func(name string, args ...string) error {
			return exec.Command(name, args...).Start()
		}
	}
	return runner(command, args...)
}

func writeServerShortcutLogMarker(out io.Writer) error {
	_, err := io.WriteString(out, "\n")
	return err
}
