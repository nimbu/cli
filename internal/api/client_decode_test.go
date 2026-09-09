package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDoReturnsResponseDecodeErrorOnSuccessBodyMismatch(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"customers":{"__type":"Relation"}}`))
	}))
	t.Cleanup(server.Close)

	client := New(server.URL, "token")
	var result struct {
		Customers []string `json:"customers"`
	}
	err := client.Put(context.Background(), "/roles/bingo", map[string]any{"name": "bingo"}, &result)
	if err == nil {
		t.Fatal("expected decode error")
	}

	var decodeErr *ResponseDecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("error type = %T (%v)", err, err)
	}
	if decodeErr.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", decodeErr.StatusCode)
	}
	if !strings.Contains(string(decodeErr.Body), `"__type":"Relation"`) {
		t.Fatalf("body = %s", decodeErr.Body)
	}

	msg := err.Error()
	if !strings.Contains(msg, "request succeeded (HTTP 200) but the response could not be decoded:") {
		t.Fatalf("message = %q", msg)
	}
}
