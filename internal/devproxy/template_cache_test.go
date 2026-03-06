package devproxy

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestTemplateCacheFallbackScanInvalidation(t *testing.T) {
	root := t.TempDir()
	templatesDir := filepath.Join(root, "templates")
	if err := os.MkdirAll(templatesDir, 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	file := filepath.Join(templatesDir, "index.liquid")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	cache := NewTemplateCache(root, false, 50*time.Millisecond, &Logger{})
	if err := cache.Start(); err != nil {
		t.Fatalf("start cache: %v", err)
	}
	defer func() { _ = cache.Stop() }()

	first, stale, err := cache.GetCompressed()
	if err != nil {
		t.Fatalf("get compressed first: %v", err)
	}
	if stale {
		t.Fatal("first payload cannot be stale")
	}

	if err := os.WriteFile(file, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("update template: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		second, stale2, err2 := cache.GetCompressed()
		if err2 != nil {
			t.Fatalf("get compressed second: %v", err2)
		}
		if stale2 {
			t.Fatal("stale payload unexpected in happy path")
		}
		if second != first {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cache did not invalidate after template change")
		}
		time.Sleep(60 * time.Millisecond)
	}
}

func TestTemplateCacheStopIsIdempotent(t *testing.T) {
	root := t.TempDir()
	templatesDir := filepath.Join(root, "templates")
	if err := os.MkdirAll(templatesDir, 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templatesDir, "index.liquid"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	cache := NewTemplateCache(root, false, 50*time.Millisecond, &Logger{})
	if err := cache.Start(); err != nil {
		t.Fatalf("start cache: %v", err)
	}

	if err := cache.Stop(); err != nil {
		t.Fatalf("first stop failed: %v", err)
	}
	if err := cache.Stop(); err != nil {
		t.Fatalf("second stop failed: %v", err)
	}
}

func TestTemplateCacheStaleFlagReturnedToConcurrentWaiters(t *testing.T) {
	root := t.TempDir()
	templatesDir := filepath.Join(root, "templates")
	if err := os.MkdirAll(templatesDir, 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	file := filepath.Join(templatesDir, "index.liquid")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	cache := NewTemplateCache(root, false, 2*time.Second, &Logger{})
	if err := cache.Start(); err != nil {
		t.Fatalf("start cache: %v", err)
	}
	defer func() { _ = cache.Stop() }()

	first, stale, err := cache.GetCompressed()
	if err != nil {
		t.Fatalf("first get compressed: %v", err)
	}
	if stale {
		t.Fatal("first cache read should not be stale")
	}

	if err := os.Chmod(file, 0o000); err != nil {
		t.Fatalf("chmod file: %v", err)
	}
	defer func() { _ = os.Chmod(file, 0o644) }()

	cache.markDirty("test")

	type result struct {
		code  string
		stale bool
		err   error
	}
	results := make([]result, 2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			code, staleFlag, callErr := cache.GetCompressed()
			results[idx] = result{code: code, stale: staleFlag, err: callErr}
		}(i)
	}
	wg.Wait()

	for i, r := range results {
		if r.err != nil {
			t.Fatalf("result %d unexpected error: %v", i, r.err)
		}
		if !r.stale {
			t.Fatalf("result %d expected stale=true", i)
		}
		if r.code != first {
			t.Fatalf("result %d stale payload mismatch", i)
		}
	}
}
