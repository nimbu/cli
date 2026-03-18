//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris || zos

package cmd

import (
	"testing"

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
