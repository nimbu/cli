package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadonlyClientAllowsReadRequests(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	client := New(srv.URL, "").WithReadonly(true)
	var result map[string]any
	if err := client.Get(context.Background(), "/channels", &result); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if hits != 1 {
		t.Fatalf("server hits = %d, want 1", hits)
	}
}

func TestReadonlyClientBlocksMutatingRequestsBeforeNetwork(t *testing.T) {
	mutatingMethods := []string{
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
	}

	for _, method := range mutatingMethods {
		t.Run(method, func(t *testing.T) {
			var hits int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits++
				w.WriteHeader(http.StatusNoContent)
			}))
			defer srv.Close()

			client := New(srv.URL, "").WithReadonly(true)
			err := client.Request(context.Background(), method, "/channels", map[string]any{"name": "x"}, nil)
			if err == nil {
				t.Fatal("expected readonly error")
			}
			var readonlyErr *ReadonlyError
			if !errors.As(err, &readonlyErr) {
				t.Fatalf("error = %T %v, want *ReadonlyError", err, err)
			}
			if readonlyErr.Method != method || readonlyErr.Path != "/channels" {
				t.Fatalf("readonly error = %#v, want method %s path /channels", readonlyErr, method)
			}
			if hits != 0 {
				t.Fatalf("server hits = %d, want 0", hits)
			}
		})
	}
}

func TestReadonlyClientBlocksRawMutatingRequestsBeforeNetwork(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := New(srv.URL, "").WithReadonly(true)
	resp, err := client.RawRequest(context.Background(), http.MethodPost, "/channels", map[string]any{"name": "x"})
	if err == nil {
		if resp != nil && resp.Body != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
		t.Fatal("expected readonly error")
	}
	var readonlyErr *ReadonlyError
	if !errors.As(err, &readonlyErr) {
		t.Fatalf("error = %T %v, want *ReadonlyError", err, err)
	}
	if hits != 0 {
		t.Fatalf("server hits = %d, want 0", hits)
	}
}

func TestReadonlyRequestIgnoresOperationClassOverride(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := New(srv.URL, "").WithReadonly(true)
	err := client.Post(context.Background(), "/channels", nil, nil, WithOperationClass(OperationRead))
	if err == nil {
		t.Fatal("expected readonly error")
	}
	if hits != 0 {
		t.Fatalf("server hits = %d, want 0", hits)
	}
}

func TestNonReadonlyClientAllowsMutatingRequests(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := New(srv.URL, "").WithReadonly(false)
	if err := client.Post(context.Background(), "/channels", map[string]any{"name": "x"}, nil); err != nil {
		t.Fatalf("Post: %v", err)
	}
	if hits != 1 {
		t.Fatalf("server hits = %d, want 1", hits)
	}
}
