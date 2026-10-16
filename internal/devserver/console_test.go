package devserver

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

type syncBuffer struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	writes int
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.writes++
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func newTestConsole() *Console {
	// Long delay: tests drive partial flushes explicitly.
	return &Console{flushDelay: time.Hour}
}

func TestConsolePrefixesCompleteLines(t *testing.T) {
	var out syncBuffer
	c := newTestConsole()
	w := c.Writer(&out, "dev | ")

	_, _ = w.Write([]byte("one\ntwo\r\nthr"))
	_, _ = w.Write([]byte("ee\n"))

	if got, want := out.String(), "dev | one\ndev | two\ndev | three\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestConsoleKeepsPartialLinesOffScreenUntilNewline(t *testing.T) {
	var out syncBuffer
	c := newTestConsole()
	child := c.Writer(&out, "dev | ")
	nimbu := c.Writer(&out, "")

	_, _ = child.Write([]byte("compiling"))
	_, _ = nimbu.Write([]byte("shortcuts available after ready\n"))
	_, _ = child.Write([]byte(" done\n"))

	want := "shortcuts available after ready\ndev | compiling done\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestConsoleEndsFlushedPartialLineBeforeAnotherSource(t *testing.T) {
	var out syncBuffer
	c := newTestConsole()
	child := c.Writer(&out, "dev | ")
	nimbu := c.Writer(&out, "")

	_, _ = child.Write([]byte("Compiling..."))
	child.flushPartial() // the idle timer fired: the prompt is on screen
	_, _ = nimbu.Write([]byte("open live: failed\n"))
	_, _ = child.Write([]byte(" ok\n"))

	want := "dev | Compiling...\nopen live: failed\ndev |  ok\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestConsoleContinuesOwnPartialLineWithoutRepeatingPrefix(t *testing.T) {
	var out syncBuffer
	c := newTestConsole()
	child := c.Writer(&out, "dev | ")

	_, _ = child.Write([]byte("50%"))
	child.flushPartial()
	_, _ = child.Write([]byte(" 100%\n"))

	if got, want := out.String(), "dev | 50% 100%\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestConsoleRepeatsPrefixAfterCarriageReturn(t *testing.T) {
	var out syncBuffer
	c := newTestConsole()
	child := c.Writer(&out, "dev | ")

	_, _ = child.Write([]byte("10%\r20%\n"))

	if got, want := out.String(), "dev | 10%\rdev | 20%\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestConsoleFlushesPartialLineAfterDelay(t *testing.T) {
	var out syncBuffer
	c := &Console{flushDelay: 10 * time.Millisecond}
	child := c.Writer(&out, "dev | ")

	_, _ = child.Write([]byte("? Pick a port"))

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if out.String() == "dev | ? Pick a port" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("partial line not flushed, got %q", out.String())
}

func TestConsoleCloseTerminatesOpenLine(t *testing.T) {
	var out syncBuffer
	c := newTestConsole()
	child := c.Writer(&out, "dev | ")

	_, _ = child.Write([]byte("tail"))
	if err := child.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if got, want := out.String(), "dev | tail\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestConsoleConcurrentSourcesNeverSplitLines(t *testing.T) {
	var out syncBuffer
	c := newTestConsole()
	child := c.Writer(&out, "dev | ")
	proxy := c.Writer(&out, "")

	var wg sync.WaitGroup
	// One goroutine per source mirrors exec's pipe copier; each child line is
	// split across two writes, like a pipe read would.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			_, _ = fmt.Fprintf(child, "child-%02d-", i)
			_, _ = child.Write([]byte("end\n"))
		}
	}()
	// Request logs come from many handler goroutines.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _ = fmt.Fprintf(proxy, "GET /%02d (200)\n", i)
		}(i)
	}
	wg.Wait()

	for _, line := range strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n") {
		if strings.HasPrefix(line, "GET /") {
			if !strings.HasSuffix(line, "(200)") || strings.Contains(line, "child") {
				t.Fatalf("proxy line was split: %q", line)
			}
			continue
		}
		if !strings.HasPrefix(line, "dev | child-") || !strings.HasSuffix(line, "-end") {
			t.Fatalf("unexpected line: %q", line)
		}
	}
}
