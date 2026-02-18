package api

import (
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
)

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestShouldRetryStatus(t *testing.T) {
	tests := []struct {
		name   string
		status int
		meta   operationMeta
		want   bool
	}{
		{"read_500", http.StatusInternalServerError, operationMeta{Class: OperationRead}, true},
		{"read_429", http.StatusTooManyRequests, operationMeta{Class: OperationRead}, true},
		{"mutate_non_idempotent_500", http.StatusInternalServerError, operationMeta{Class: OperationMutate, Idempotent: false}, false},
		{"mutate_idempotent_500", http.StatusInternalServerError, operationMeta{Class: OperationMutate, Idempotent: true}, true},
		{"destructive_500", http.StatusInternalServerError, operationMeta{Class: OperationDestructive}, false},
		{"destructive_429", http.StatusTooManyRequests, operationMeta{Class: OperationDestructive}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRetryStatus(tt.status, tt.meta); got != tt.want {
				t.Fatalf("shouldRetryStatus(%d, %#v) = %v, want %v", tt.status, tt.meta, got, tt.want)
			}
		})
	}
}

func TestShouldRetryError(t *testing.T) {
	netTimeout := net.Error(timeoutErr{})
	nonNet := errors.New("boom")

	if !shouldRetryError(netTimeout, operationMeta{Class: OperationRead}) {
		t.Fatal("expected read requests to retry timeout errors")
	}
	if shouldRetryError(netTimeout, operationMeta{Class: OperationMutate, Idempotent: false}) {
		t.Fatal("did not expect non-idempotent mutate to retry timeout errors")
	}
	if !shouldRetryError(netTimeout, operationMeta{Class: OperationMutate, Idempotent: true}) {
		t.Fatal("expected idempotent mutate to retry timeout errors")
	}
	if shouldRetryError(nonNet, operationMeta{Class: OperationDestructive}) {
		t.Fatal("did not expect destructive operations to retry generic errors")
	}
}

func TestCloneRequestForAttemptRequiresRewindableBody(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://example.com", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	req.GetBody = nil

	_, err = cloneRequestForAttempt(req, 1)
	if err == nil {
		t.Fatal("expected non-rewindable body error")
	}
}
