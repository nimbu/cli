//go:build !windows

package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestReadJSONInputFromFIFO(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "body.json")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat fifo: %v", err)
	}
	if info.Size() != 0 {
		t.Fatalf("fifo Size() = %d, want 0 (the check that used to reject pipes)", info.Size())
	}
	if info.Mode().IsRegular() {
		t.Fatal("fifo reported as regular file")
	}

	errc := make(chan error, 1)
	go func() {
		f, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			errc <- err
			return
		}
		_, err = f.WriteString(`{"from":"fifo"}`)
		closeErr := f.Close()
		if err != nil {
			errc <- err
			return
		}
		errc <- closeErr
	}()

	done := make(chan struct{})
	var (
		body    map[string]any
		readErr error
	)
	go func() {
		defer close(done)
		body, readErr = readJSONInput(path)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out reading FIFO")
	}
	if readErr != nil {
		t.Fatalf("readJSONInput(fifo): %v", readErr)
	}
	if body["from"] != "fifo" {
		t.Fatalf("body = %#v", body)
	}
	if err := <-errc; err != nil {
		t.Fatalf("fifo writer: %v", err)
	}
}

func TestReadJSONInputEmptyFIFO(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}

	errc := make(chan error, 1)
	go func() {
		f, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			errc <- err
			return
		}
		errc <- f.Close()
	}()

	_, err := readJSONInput(path)
	if err == nil {
		t.Fatal("expected empty FIFO to be rejected")
	}
	if !errors.Is(err, errNoJSONInput) {
		t.Fatalf("error = %v, want no JSON input", err)
	}
	if err := <-errc; err != nil {
		t.Fatalf("fifo writer: %v", err)
	}
}
