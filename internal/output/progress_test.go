package output

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTaskFinishIsIdempotentAfterFail(t *testing.T) {
	var errBuf bytes.Buffer
	ctx := WithWriter(context.Background(), &Writer{
		Out:   &errBuf,
		Err:   &errBuf,
		NoTTY: true,
	})

	progress := NewProgress(ctx)
	task := progress.Counter("copy records", 1)
	task.Fail(errors.New("boom"))
	task.Done("done")

	if got := strings.Count(errBuf.String(), "failed  copy records"); got != 1 {
		t.Fatalf("expected one failed final line, got %d\n%s", got, errBuf.String())
	}
	if strings.Contains(errBuf.String(), "done  copy records") {
		t.Fatalf("unexpected done line after failure:\n%s", errBuf.String())
	}
}

func TestChildTaskDoneAfterFailKeepsParentActive(t *testing.T) {
	var errBuf bytes.Buffer
	ctx := WithWriter(context.Background(), &Writer{
		Out:   &errBuf,
		Err:   &errBuf,
		NoTTY: true,
	})

	progress := NewProgress(ctx)
	parent := progress.Counter("parent", 2)
	child := progress.Transfer("child", 10)

	child.Fail(errors.New("boom"))
	if progress.task != parent {
		t.Fatalf("expected parent task to resume after child failure")
	}

	child.Done("done")
	if progress.task != parent {
		t.Fatalf("expected second child finalization to be ignored")
	}

	parent.Done("done")
	if got := strings.Count(errBuf.String(), "failed  child"); got != 1 {
		t.Fatalf("expected one failed child line, got %d\n%s", got, errBuf.String())
	}
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func newAnimatedProgressForTest(t *testing.T, delay time.Duration) (*Progress, *syncBuffer) {
	t.Helper()
	buf := &syncBuffer{}
	p := newProgress(nil, buf, Theme{}, GlyphSet(false), true)
	p.mu.Lock()
	p.spinnerDelay = delay
	p.mu.Unlock()
	t.Cleanup(p.Close)
	return p, buf
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (p *Progress) lineIsOpen() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lineOpen
}

func TestProgressHoldsSpinnerBackForShortWork(t *testing.T) {
	p, buf := newAnimatedProgressForTest(t, time.Hour)

	task := p.Phase("quick work")
	task.Add(1)
	p.mu.Lock()
	p.renderLocked(false)
	p.mu.Unlock()
	if got := buf.String(); got != "" {
		t.Fatalf("expected no spinner before the delay, got %q", got)
	}

	task.Done("done")
	if got := buf.String(); got != clearLine+"✓ done  quick work  0s\n" {
		t.Fatalf("expected only the final line, got %q", got)
	}
}

func TestProgressCloseErasesUnfinishedLine(t *testing.T) {
	p, buf := newAnimatedProgressForTest(t, 0)

	p.Phase("slow work")
	waitFor(t, p.lineIsOpen)

	p.Close()
	p.Close() // idempotent

	out := buf.String()
	if !strings.HasSuffix(out, clearLine) {
		t.Fatalf("expected close to erase the live line, got %q", out)
	}
	if strings.Contains(out, "done") {
		t.Fatalf("unfinished task must not be reported as done, got %q", out)
	}
}

// Regression: progress used to arm its own SIGINT/SIGTERM handler and re-raise
// the signal, which the root handler then counted as a second Ctrl-C and
// force-exited. Progress now only follows the command's context.
func TestProgressCancelErasesLineAndStopsDrawing(t *testing.T) {
	cancel := make(chan struct{})
	buf := &syncBuffer{}
	p := newProgress(cancel, buf, Theme{}, GlyphSet(false), true)
	p.mu.Lock()
	p.spinnerDelay = 0
	p.mu.Unlock()
	t.Cleanup(p.Close)

	task := p.Transfer("upload", 100)
	waitFor(t, p.lineIsOpen)

	close(cancel)
	<-p.doneCh

	out := buf.String()
	if !strings.HasSuffix(out, clearLine) {
		t.Fatalf("expected interrupted line to be erased, got %q", out)
	}
	if p.lineIsOpen() {
		t.Fatal("line still marked open after cancel")
	}
	task.Fail(errors.New("canceled"))
	if got := buf.String(); got != out {
		t.Fatalf("expected no drawing after cancel, got extra %q", strings.TrimPrefix(got, out))
	}
}

func TestProgressFinalLineUsesASCIIGlyphs(t *testing.T) {
	var buf bytes.Buffer
	p := newProgress(nil, &buf, Theme{}, GlyphSet(true), false)
	p.Phase("fetch").Done("done")
	p.Phase("push").Fail(errors.New("boom"))

	out := buf.String()
	for _, want := range []string{"ok done  fetch", "x failed  push"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in %q", want, out)
		}
	}
}
