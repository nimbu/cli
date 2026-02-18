package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/auth"
	"github.com/nimbu/cli/internal/output"
)

type canonicalErrorCode string

const (
	errorUsageInvalid      canonicalErrorCode = "cli.usage.invalid_args"
	errorValidationFailed  canonicalErrorCode = "cli.validation.failed"
	errorAuthNotLoggedIn   canonicalErrorCode = "auth.not_logged_in"
	errorAuthUnauthorized  canonicalErrorCode = "auth.unauthorized"
	errorAuthForbidden     canonicalErrorCode = "auth.forbidden"
	errorAuthTwoFactor     canonicalErrorCode = "auth.two_factor_required"
	errorScopeMissing      canonicalErrorCode = "auth.scope_missing"
	errorNotFound          canonicalErrorCode = "resource.not_found"
	errorConflict          canonicalErrorCode = "resource.conflict"
	errorRequestInvalid    canonicalErrorCode = "request.invalid"
	errorRequestValidation canonicalErrorCode = "request.validation"
	errorRateLimited       canonicalErrorCode = "rate_limit.exceeded"
	errorNetworkTimeout    canonicalErrorCode = "network.timeout"
	errorNetworkFailure    canonicalErrorCode = "network.failure"
	errorServerError       canonicalErrorCode = "server.error"
	errorUnknown           canonicalErrorCode = "internal.unknown"
)

type errorEnvelope struct {
	Status string          `json:"status"`
	Error  errorDescriptor `json:"error"`
}

type errorDescriptor struct {
	Code             canonicalErrorCode    `json:"code"`
	Message          string                `json:"message"`
	Hint             string                `json:"hint,omitempty"`
	ExitCode         int                   `json:"exit_code"`
	HTTPStatus       int                   `json:"http_status,omitempty"`
	Retryable        bool                  `json:"retryable"`
	Details          map[string]any        `json:"details,omitempty"`
	ValidationErrors []api.ValidationError `json:"validation_errors,omitempty"`
}

func emitCommandError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	desc := classifyError(err)

	mode := output.FromContext(ctx)
	if mode.JSON {
		payload := errorEnvelope{Status: "error", Error: desc}
		data, marshalErr := json.MarshalIndent(payload, "", "  ")
		if marshalErr != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
		} else {
			_, _ = fmt.Fprintln(os.Stderr, string(data))
		}
	} else {
		_, _ = fmt.Fprintln(os.Stderr, desc.Message)
		if desc.Hint != "" {
			_, _ = fmt.Fprintln(os.Stderr, desc.Hint)
		}
	}

	return &ExitError{Code: desc.ExitCode, Err: err}
}

func classifyError(err error) errorDescriptor {
	desc := errorDescriptor{
		Code:      errorUnknown,
		Message:   err.Error(),
		ExitCode:  ExitGeneral,
		Retryable: false,
	}

	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		desc.HTTPStatus = apiErr.StatusCode
		desc.Details = apiErr.Details
		desc.ValidationErrors = apiErr.Errors

		switch apiErr.StatusCode {
		case 400:
			desc.Code = errorRequestInvalid
			desc.ExitCode = ExitValidation
			desc.Hint = "check request parameters and payload fields"
		case 401:
			desc.Code = errorAuthUnauthorized
			desc.ExitCode = ExitAuth
			desc.Hint = "refresh credentials with `nimbu-cli auth login`"
		case 403:
			desc.ExitCode = ExitAuthz
			desc.Hint = scopeHint(apiErr)
			if isScopeError(apiErr) {
				desc.Code = errorScopeMissing
			} else {
				desc.Code = errorAuthForbidden
			}
		case 404:
			desc.Code = errorNotFound
			desc.ExitCode = ExitNotFound
		case 409:
			desc.Code = errorConflict
			desc.ExitCode = ExitValidation
		case 422:
			desc.Code = errorRequestValidation
			desc.ExitCode = ExitValidation
			desc.Hint = "inspect validation_errors for exact field failures"
		case 429:
			desc.Code = errorRateLimited
			desc.ExitCode = ExitRateLimit
			desc.Retryable = true
			desc.Hint = "retry later; honor Retry-After when present"
		default:
			if apiErr.StatusCode >= 500 {
				desc.Code = errorServerError
				desc.ExitCode = ExitGeneral
				desc.Retryable = true
				desc.Hint = "server-side failure; retry may succeed"
			}
		}
		if desc.Message == "" {
			desc.Message = apiErr.Error()
		}
		return desc
	}

	var scopeErr *scopeMissingError
	if errors.As(err, &scopeErr) {
		desc.Code = errorScopeMissing
		desc.ExitCode = ExitAuthz
		desc.Message = scopeErr.Error()
		desc.Hint = "create or use a token including the listed scopes"
		desc.Details = map[string]any{"required_scopes": scopeErr.Required}
		return desc
	}

	if errors.Is(err, auth.ErrNoToken) || strings.Contains(strings.ToLower(err.Error()), "not logged in") {
		desc.Code = errorAuthNotLoggedIn
		desc.ExitCode = ExitAuth
		desc.Message = "not logged in"
		desc.Hint = "run `nimbu-cli auth login`"
		return desc
	}

	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		desc.Code = errorUsageInvalid
		desc.ExitCode = exitErr.Code
		return desc
	}

	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "timeout") {
		desc.Code = errorNetworkTimeout
		desc.ExitCode = ExitNetwork
		desc.Retryable = true
		desc.Hint = "check network and retry"
		return desc
	}
	if strings.Contains(lower, "connection") || strings.Contains(lower, "dial") {
		desc.Code = errorNetworkFailure
		desc.ExitCode = ExitNetwork
		desc.Retryable = true
		desc.Hint = "check network connectivity and API URL"
		return desc
	}

	if strings.Contains(lower, "invalid") || strings.Contains(lower, "required") {
		desc.Code = errorValidationFailed
		desc.ExitCode = ExitValidation
	}

	return desc
}

func isScopeError(apiErr *api.Error) bool {
	if apiErr == nil {
		return false
	}
	text := strings.ToLower(apiErr.Message + " " + apiErr.Code)
	return strings.Contains(text, "scope")
}

func scopeHint(apiErr *api.Error) string {
	if apiErr == nil {
		return "token lacks required permissions for this endpoint"
	}
	if accepted, ok := apiErr.Details["accepted_scopes"]; ok {
		return fmt.Sprintf("missing required scope(s): %v. create/use a token with these scopes", accepted)
	}
	return "token lacks required permissions for this endpoint; compare X-OAuth-Scopes with X-Accepted-OAuth-Scopes"
}
