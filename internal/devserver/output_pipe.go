package devserver

import (
	"io"
	"os"
	"sync"
	"time"
)

// outputDrainDelay bounds how long the child's remaining output is copied
// after it exited and its process group was reaped.
const outputDrainDelay = time.Second

// childOutput connects the child's stdout/stderr to writers that are not
// files. It owns the pipes instead of letting exec copy them, so Wait
// returns as soon as the child exits: the leftover process group can be
// reaped first, and only then is output drained.
type childOutput struct {
	readers []*os.File
	writers []*os.File
	dsts    []io.Writer
	copies  sync.WaitGroup
}

// attach returns what to hand the child for dst: the file itself, the
// fallback (inherit) when dst is nil, or the write end of a new pipe.
func (o *childOutput) attach(dst io.Writer, fallback *os.File) (io.Writer, error) {
	if dst == nil {
		return fallback, nil
	}
	if file, ok := dst.(*os.File); ok {
		return file, nil
	}
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	o.readers = append(o.readers, r)
	o.writers = append(o.writers, w)
	o.dsts = append(o.dsts, dst)
	return w, nil
}

// started closes the parent's write ends (the child holds its own copies)
// and starts copying.
func (o *childOutput) started() {
	for _, w := range o.writers {
		_ = w.Close()
	}
	for i, r := range o.readers {
		o.copies.Add(1)
		go func(r *os.File, dst io.Writer) {
			defer o.copies.Done()
			_, _ = io.Copy(dst, r)
		}(r, o.dsts[i])
	}
}

// abort releases every pipe when the child failed to start.
func (o *childOutput) abort() {
	for _, w := range o.writers {
		_ = w.Close()
	}
	for _, r := range o.readers {
		_ = r.Close()
	}
}

// drain waits for the copies to reach EOF, up to timeout, then closes the
// read ends so a writer that is still alive cannot block shutdown. It waits
// at most timeout once more for the copies to notice: closing does not
// interrupt a blocked read on Windows (synchronous pipe handles), and a copy
// can be stuck writing to a stopped terminal. A copy that is still stuck
// then is abandoned rather than holding up the child's exit.
func (o *childOutput) drain(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		o.copies.Wait()
		close(done)
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		o.closeReaders()
		return
	case <-timer.C:
	}
	o.closeReaders()
	timer.Reset(timeout)
	select {
	case <-done:
	case <-timer.C:
	}
}

func (o *childOutput) closeReaders() {
	for _, r := range o.readers {
		_ = r.Close()
	}
}
