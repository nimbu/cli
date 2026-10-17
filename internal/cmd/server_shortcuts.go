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
	"syscall"
	"time"

	"github.com/muesli/cancelreader"
)

type serverShortcutLinks struct {
	DevURL   string
	LiveURL  string
	AdminURL string
	// Active is set when the key listener runs; the hint is shown only then.
	Active bool
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

// Hint is the one-line key summary printed under the startup summary. When
// the listener runs, Enter, help and quit are always available.
func (l serverShortcutLinks) Hint() string {
	if !l.Active {
		return ""
	}
	parts := make([]string, 0, 6)
	if l.DevURL != "" {
		parts = append(parts, "[o] open dev")
	}
	if l.LiveURL != "" {
		parts = append(parts, "[l] open live")
	}
	if l.AdminURL != "" {
		parts = append(parts, "[b] open admin")
	}
	parts = append(parts, "[enter] mark", "[?] help", "[q] quit")
	return strings.Join(parts, "  ")
}

// HelpRows lists every shortcut with a full description, for the '?' help.
func (l serverShortcutLinks) HelpRows() [][2]string {
	rows := make([][2]string, 0, 6)
	if l.DevURL != "" {
		rows = append(rows, [2]string{"o", "open the dev server  " + l.DevURL})
	}
	if l.LiveURL != "" {
		rows = append(rows, [2]string{"l", "open the live site   " + l.LiveURL})
	}
	if l.AdminURL != "" {
		rows = append(rows, [2]string{"b", "open the site admin  " + l.AdminURL})
	}
	return append(rows,
		[2]string{"enter", "print a blank line to mark a spot in the log"},
		[2]string{"? h", "show this help"},
		[2]string{"q", "quit (Ctrl-C works too; press it twice to force)"},
	)
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
	serverShortcutHelp
	serverShortcutQuit
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
	case '?', 'h':
		action = serverShortcutHelp
	case 'q':
		action = serverShortcutQuit
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
	switch action {
	case serverShortcutLogMarker, serverShortcutHelp, serverShortcutQuit:
		// Local actions work before the child is ready.
		return serverShortcutDecision{Action: action}
	}
	if !ready {
		return serverShortcutDecision{Action: action, Pending: true}
	}
	return serverShortcutDecision{Action: action}
}

// serverTermMode re-applies and restores the shortcut input mode.
type serverTermMode struct {
	apply   func() error
	restore func() error
	// foreground reports whether nimbu's job owns the terminal; nil means
	// always (no job control).
	foreground func() bool
}

// serverForegroundPoll is how often a listener paused in the background
// checks whether the job is back in the foreground, for a `fg` that sends no
// SIGCONT. A var for tests.
var serverForegroundPoll = time.Second

type serverShortcutListener struct {
	closeOnce sync.Once
	errCh     chan error
	events    chan byte
	input     *os.File
	ownsInput bool
	reader    cancelreader.CancelReader
	// done is closed by Close; resumed is signaled by Resume. Both wake a
	// read loop paused while the job runs in the background.
	done    chan struct{}
	resumed chan struct{}

	// modeMu guards the terminal mode against Suspend/Resume racing Close.
	modeMu sync.Mutex
	mode   serverTermMode
	closed bool
}

func newServerShortcutListener() (*serverShortcutListener, error) {
	input, ownsInput, err := openServerShortcutInput()
	if err != nil {
		return nil, fmt.Errorf("open shortcut input: %w", err)
	}

	mode, err := prepareServerShortcutInput(input)
	if err != nil {
		if ownsInput {
			_ = input.Close()
		}
		return nil, fmt.Errorf("enable shortcut input: %w", err)
	}

	reader, err := cancelreader.NewReader(input)
	if err != nil {
		if mode.restore != nil {
			_ = mode.restore()
		}
		if ownsInput {
			_ = input.Close()
		}
		return nil, fmt.Errorf("prepare shortcut input: %w", err)
	}

	return newServerShortcutListenerFrom(reader, input, ownsInput, mode), nil
}

func newServerShortcutListenerFrom(reader cancelreader.CancelReader, input *os.File, ownsInput bool, mode serverTermMode) *serverShortcutListener {
	listener := &serverShortcutListener{
		errCh:     make(chan error, 1),
		events:    make(chan byte, 16),
		input:     input,
		ownsInput: ownsInput,
		reader:    reader,
		done:      make(chan struct{}),
		resumed:   make(chan struct{}, 1),
		mode:      mode,
	}
	go listener.readLoop()
	return listener
}

func (l *serverShortcutListener) readLoop() {
	defer l.RestoreOnPanic()
	defer close(l.events)
	defer close(l.errCh)

	var buf [1]byte
	for {
		n, err := l.reader.Read(buf[:])
		if err != nil {
			if errors.Is(err, cancelreader.ErrCanceled) || errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) {
				return
			}
			if l.inBackground(err) {
				if !l.waitForeground() {
					return
				}
				continue
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

// inBackground reports a read that failed because the job runs in the
// background (Ctrl-Z, then `bg`): with SIGTTIN ignored such a read fails with
// EIO instead of stopping nimbu. The listener pauses rather than giving up.
func (l *serverShortcutListener) inBackground(err error) bool {
	return errors.Is(err, syscall.EIO) && l.mode.foreground != nil && !l.mode.foreground()
}

// waitForeground blocks until the job is in the foreground again, then
// re-applies shortcut mode. It reports false when the listener was closed.
func (l *serverShortcutListener) waitForeground() bool {
	ticker := time.NewTicker(serverForegroundPoll)
	defer ticker.Stop()
	for {
		select {
		case <-l.done:
			return false
		case <-l.resumed:
		case <-ticker.C:
		}
		if l.mode.foreground() {
			// A no-op when SIGCONT already re-applied it.
			_ = l.Resume()
			return true
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
		if l.done != nil {
			close(l.done)
		}
		l.modeMu.Lock()
		l.closed = true
		if l.mode.restore != nil {
			if err := l.mode.restore(); err != nil && closeErr == nil {
				closeErr = err
			}
		}
		l.modeMu.Unlock()
		if l.ownsInput && l.input != nil {
			if err := l.input.Close(); err != nil && closeErr == nil {
				closeErr = err
			}
		}
	})
	return closeErr
}

// Suspend hands the terminal back in its original mode (Ctrl-Z) without
// stopping the listener.
func (l *serverShortcutListener) Suspend() error {
	if l == nil {
		return nil
	}
	l.modeMu.Lock()
	defer l.modeMu.Unlock()
	if l.closed || l.mode.restore == nil {
		return nil
	}
	return l.mode.restore()
}

// Resume re-applies shortcut mode after the job is continued (fg), and
// wakes a read loop paused in the background.
func (l *serverShortcutListener) Resume() error {
	if l == nil {
		return nil
	}
	l.modeMu.Lock()
	defer l.modeMu.Unlock()
	if l.closed || l.mode.apply == nil {
		return nil
	}
	err := l.mode.apply()
	select {
	case l.resumed <- struct{}{}:
	default:
	}
	return err
}

// RestoreOnPanic is deferred by every goroutine nimbu server starts. A panic
// there would otherwise kill the process before Run's deferred Close, leaving
// the user's shell without echo. It restores the terminal and re-panics.
func (l *serverShortcutListener) RestoreOnPanic() {
	if r := recover(); r != nil {
		_ = l.Close()
		panic(r)
	}
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
