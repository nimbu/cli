package devserver

import (
	"os"
	"testing"
	"time"
)

// blockingWriter never returns from Write, like a terminal whose foreground
// job is stopped (Ctrl-Z) or a console Write stuck behind a held lock.
type blockingWriter struct{ release chan struct{} }

func (w blockingWriter) Write(p []byte) (int, error) {
	<-w.release
	return len(p), nil
}

func TestChildOutputDrainDoesNotWaitForeverOnStuckCopy(t *testing.T) {
	release := make(chan struct{})
	defer close(release)

	var out childOutput
	end, err := out.attach(blockingWriter{release: release}, os.Stdout)
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	pipeEnd, ok := end.(*os.File)
	if !ok {
		t.Fatalf("attach returned %T, want the pipe's write end", end)
	}
	if _, err := pipeEnd.Write([]byte("line\n")); err != nil {
		t.Fatalf("write to pipe: %v", err)
	}
	out.started()

	done := make(chan struct{})
	go func() {
		out.drain(50 * time.Millisecond)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("drain blocked on a copy stuck in Write")
	}
}
