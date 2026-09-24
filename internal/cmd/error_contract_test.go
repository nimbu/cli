package cmd

import (
	"testing"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/auth"
	"github.com/nimbu/cli/internal/pagepath"
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

func TestClassifyErrorReadonly(t *testing.T) {
	desc := classifyError(&api.ReadonlyError{Method: "POST", Path: "/channels"})
	if desc.Code != errorReadonly {
		t.Fatalf("unexpected code: %s", desc.Code)
	}
	if desc.ExitCode != ExitUsage {
		t.Fatalf("unexpected exit code: %d", desc.ExitCode)
	}
	if desc.Hint == "" {
		t.Fatal("expected readonly hint")
	}
}

func TestClassifyErrorResolveError(t *testing.T) {
	err := &pagepath.ResolveError{Path: "Blokken[9]", Candidates: []string{"[0] hero a"}}
	desc := classifyError(err)
	if desc.Code != errorRequestInvalid {
		t.Fatalf("code = %s", desc.Code)
	}
	if desc.ExitCode != ExitUsage {
		t.Fatalf("exit = %d", desc.ExitCode)
	}
	cands, ok := desc.Details["candidates"].([]string)
	if !ok || len(cands) != 1 {
		t.Fatalf("details = %#v", desc.Details)
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

func TestClassifyErrorHintedKeepsAPIFields(t *testing.T) {
	apiErr := &api.Error{
		StatusCode: 409,
		Code:       "draft_base_changed",
		Message:    "draft base changed",
		Errors:     []api.ValidationError{{Field: "base", Message: "is stale"}},
	}
	desc := classifyError(draftAPIError(apiErr, "about"))
	if desc.Code != errorConflict {
		t.Fatalf("code = %s", desc.Code)
	}
	if desc.Hint == "" {
		t.Fatal("expected hint")
	}
	if desc.HTTPStatus != 409 {
		t.Fatalf("http_status = %d", desc.HTTPStatus)
	}
	if len(desc.ValidationErrors) != 1 || desc.ValidationErrors[0].Field != "base" {
		t.Fatalf("validation_errors = %#v", desc.ValidationErrors)
	}
}
