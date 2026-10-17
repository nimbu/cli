//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris || zos

package cmd

import (
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/muesli/cancelreader"
	"golang.org/x/sys/unix"
)

func TestServerShortcutTermiosPreservesOutputAndSignals(t *testing.T) {
	initial := unix.Termios{
		Oflag: unix.OPOST,
		Lflag: unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN,
	}
	initial.Cc[unix.VMIN] = 4
	initial.Cc[unix.VTIME] = 9

	got := serverShortcutTermios(initial)

	if got.Oflag&unix.OPOST == 0 {
		t.Fatal("expected OPOST to remain enabled")
	}
	if got.Lflag&unix.ISIG == 0 {
		t.Fatal("expected ISIG to remain enabled")
	}
	if got.Lflag&unix.ECHO != 0 {
		t.Fatal("expected ECHO to be disabled")
	}
	if got.Lflag&unix.ECHONL != 0 {
		t.Fatal("expected ECHONL to be disabled")
	}
	if got.Lflag&unix.ICANON != 0 {
		t.Fatal("expected ICANON to be disabled")
	}
	if got.Cc[unix.VMIN] != 1 {
		t.Fatalf("VMIN = %d, want 1", got.Cc[unix.VMIN])
	}
	if got.Cc[unix.VTIME] != 0 {
		t.Fatalf("VTIME = %d, want 0", got.Cc[unix.VTIME])
	}
}

// fakeTTYReader stands in for /dev/tty: the first Read fails with EIO, as a
// read from a background job does once SIGTTIN is ignored; later Reads
// return queued bytes until Cancel.
type fakeTTYReader struct {
	eio      atomic.Bool
	bytes    chan byte
	canceled chan struct{}
	once     sync.Once
}

func newFakeTTYReader() *fakeTTYReader {
	r := &fakeTTYReader{bytes: make(chan byte, 4), canceled: make(chan struct{})}
	r.eio.Store(true)
	return r
}

func (r *fakeTTYReader) Read(p []byte) (int, error) {
	if r.eio.Swap(false) {
		return 0, &os.PathError{Op: "read", Path: "/dev/tty", Err: syscall.EIO}
	}
	select {
	case b := <-r.bytes:
		p[0] = b
		return 1, nil
	case <-r.canceled:
		return 0, cancelreader.ErrCanceled
	}
}

func (r *fakeTTYReader) Cancel() bool {
	r.once.Do(func() { close(r.canceled) })
	return true
}

func (r *fakeTTYReader) Close() error { return nil }

func newFakeTTYListener(reader *fakeTTYReader, foreground *atomic.Bool, applied *atomic.Int32) *serverShortcutListener {
	return newServerShortcutListenerFrom(reader, nil, false, serverTermMode{
		apply:      func() error { applied.Add(1); return nil },
		restore:    func() error { return nil },
		foreground: foreground.Load,
	})
}

// Ctrl-Z then `bg`: a read from the background job fails with EIO. The
// listener must pause, not stop for good, and pick up keys again (in
// shortcut mode) once the job is back in the foreground.
func TestServerShortcutListenerPausesInBackground(t *testing.T) {
	for _, viaResume := range []bool{true, false} {
		name := "poll"
		if viaResume {
			name = "SIGCONT"
		}
		t.Run(name, func(t *testing.T) {
			if !viaResume {
				orig := serverForegroundPoll
				serverForegroundPoll = 10 * time.Millisecond
				t.Cleanup(func() { serverForegroundPoll = orig })
			}
			reader := newFakeTTYReader()
			var foreground atomic.Bool
			var applied atomic.Int32
			l := newFakeTTYListener(reader, &foreground, &applied)
			defer func() { _ = l.Close() }()

			select {
			case err := <-l.Errors():
				t.Fatalf("background read ended the listener: %v", err)
			case <-time.After(50 * time.Millisecond):
			}

			foreground.Store(true)
			if viaResume {
				_ = l.Resume()
			}
			reader.bytes <- 'o'
			select {
			case key := <-l.Events():
				if key != 'o' {
					t.Fatalf("key = %q", key)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("listener did not resume in the foreground")
			}
			if applied.Load() == 0 {
				t.Fatal("shortcut mode was not re-applied in the foreground")
			}
		})
	}
}

// Closing while paused in the background must not hang.
func TestServerShortcutListenerClosesWhilePaused(t *testing.T) {
	reader := newFakeTTYReader()
	var foreground atomic.Bool
	var applied atomic.Int32
	l := newFakeTTYListener(reader, &foreground, &applied)
	time.Sleep(20 * time.Millisecond)
	_ = l.Close()
	select {
	case _, ok := <-l.Events():
		if ok {
			t.Fatal("unexpected key")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("paused listener did not stop on Close")
	}
}
