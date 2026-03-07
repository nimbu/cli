package cmd

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func TestNewMultipartFileBodyResetsProgressOnReplay(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "asset.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	ctx := output.WithWriter(context.Background(), &output.Writer{
		Out:   io.Discard,
		Err:   io.Discard,
		NoTTY: true,
	})
	progress := output.NewProgress(ctx)
	task := progress.Transfer("upload asset.txt", 0)

	body, err := newMultipartFileBody(path, "asset.txt", task)
	if err != nil {
		t.Fatalf("build multipart body: %v", err)
	}

	firstReader, ok := body.Reader.(io.ReadCloser)
	if !ok {
		t.Fatalf("expected read closer body, got %T", body.Reader)
	}
	defer func() { _ = firstReader.Close() }()
	if _, err := io.Copy(io.Discard, firstReader); err != nil {
		t.Fatalf("read initial body: %v", err)
	}
	if got := task.Current(); got != body.ContentLength {
		t.Fatalf("expected first pass progress %d, got %d", body.ContentLength, got)
	}

	replayReader, err := body.GetBody()
	if err != nil {
		t.Fatalf("rebuild body: %v", err)
	}
	defer func() { _ = replayReader.Close() }()
	if _, err := io.Copy(io.Discard, replayReader); err != nil {
		t.Fatalf("read replay body: %v", err)
	}
	if got := task.Current(); got != body.ContentLength {
		t.Fatalf("expected replay progress reset to %d, got %d", body.ContentLength, got)
	}
}
