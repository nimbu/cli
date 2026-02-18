package cmd

import (
	"testing"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/auth"
)

func TestClassifyErrorScopeMissing(t *testing.T) {
	err := &scopeMissingError{Required: []string{"read_orders"}}
	desc := classifyError(err)

	if desc.Code != errorScopeMissing {
		t.Fatalf("unexpected code: %s", desc.Code)
	}
	if desc.ExitCode != ExitAuthz {
		t.Fatalf("unexpected exit code: %d", desc.ExitCode)
	}
}

func TestClassifyErrorNotLoggedIn(t *testing.T) {
	desc := classifyError(auth.ErrNoToken)
	if desc.Code != errorAuthNotLoggedIn {
		t.Fatalf("unexpected code: %s", desc.Code)
	}
	if desc.ExitCode != ExitAuth {
		t.Fatalf("unexpected exit code: %d", desc.ExitCode)
	}
}

func TestClassifyErrorRateLimit(t *testing.T) {
	err := &api.Error{StatusCode: 429, Message: "rate limit exceeded"}
	desc := classifyError(err)
	if desc.Code != errorRateLimited {
		t.Fatalf("unexpected code: %s", desc.Code)
	}
	if !desc.Retryable {
		t.Fatal("expected retryable=true")
	}
}
