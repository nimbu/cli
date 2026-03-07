package output

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
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
