package output

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
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
	p := newProgress(buf, Theme{}, GlyphSet(false), true)
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

func stubInterrupts(t *testing.T) (armed *atomic.Int32, raised chan os.Signal) {
	t.Helper()
	armed = &atomic.Int32{}
	raised = make(chan os.Signal, 1)
	origNotify, origStop, origRaise := notifyInterrupt, stopInterrupt, raiseInterrupt
	notifyInterrupt = func(chan<- os.Signal, ...os.Signal) { armed.Add(1) }
	stopInterrupt = func(chan<- os.Signal) { armed.Add(-1) }
	raiseInterrupt = func(sig os.Signal) { raised <- sig }
	t.Cleanup(func() {
		notifyInterrupt, stopInterrupt, raiseInterrupt = origNotify, origStop, origRaise
	})
	return armed, raised
}

func TestProgressHoldsSpinnerBackForShortWork(t *testing.T) {
	stubInterrupts(t)
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

func TestProgressCloseErasesUnfinishedLineAndDisarms(t *testing.T) {
	armed, _ := stubInterrupts(t)
	p, buf := newAnimatedProgressForTest(t, 0)

	p.Phase("slow work")
	waitFor(t, p.lineIsOpen)
	if armed.Load() != 1 {
		t.Fatalf("expected interrupt handler armed while a line is open, got %d", armed.Load())
	}

	p.Close()
	p.Close() // idempotent

	out := buf.String()
	if !strings.HasSuffix(out, clearLine) {
		t.Fatalf("expected close to erase the live line, got %q", out)
	}
	if strings.Contains(out, "done") {
		t.Fatalf("unfinished task must not be reported as done, got %q", out)
	}
	if armed.Load() != 0 {
		t.Fatalf("expected interrupt handler disarmed after close, got %d", armed.Load())
	}
}

func TestProgressInterruptErasesLineThenReraises(t *testing.T) {
	armed, raised := stubInterrupts(t)
	p, buf := newAnimatedProgressForTest(t, 0)

	p.Transfer("upload", 100)
	waitFor(t, p.lineIsOpen)

	p.interrupts <- os.Interrupt
	select {
	case sig := <-raised:
		if sig != os.Interrupt {
			t.Fatalf("expected os.Interrupt re-raised, got %v", sig)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("signal was not re-raised")
	}
	<-p.doneCh

	if out := buf.String(); !strings.HasSuffix(out, clearLine) {
		t.Fatalf("expected interrupted line to be erased, got %q", out)
	}
	if armed.Load() != 0 {
		t.Fatalf("expected handler disarmed before re-raise, got %d", armed.Load())
	}
}

func TestProgressFinalLineUsesASCIIGlyphs(t *testing.T) {
	var buf bytes.Buffer
	p := newProgress(&buf, Theme{}, GlyphSet(true), false)
	p.Phase("fetch").Done("done")
	p.Phase("push").Fail(errors.New("boom"))

	out := buf.String()
	for _, want := range []string{"ok done  fetch", "x failed  push"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in %q", want, out)
		}
	}
}
