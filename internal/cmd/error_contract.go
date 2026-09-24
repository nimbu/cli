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
	"github.com/nimbu/cli/internal/pagepath"
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
	errorReadonly          canonicalErrorCode = "cli.readonly"
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

// displayedError wraps an error that has already been shown to the user
// (e.g. via the timeline ErrorFooter). emitCommandError skips the stderr
// print for these but still returns the correct exit code.
type displayedError struct {
	err error
}

func (e *displayedError) Error() string { return e.err.Error() }
func (e *displayedError) Unwrap() error { return e.err }

// detailedError injects JSON envelope details for a typed CLI failure.
type detailedError struct {
	err      error
	code     canonicalErrorCode
	exitCode int
	details  map[string]any
	hint     string
}

func (e *detailedError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *detailedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func newDetailedError(err error, code canonicalErrorCode, exitCode int, details map[string]any) error {
	return &detailedError{err: err, code: code, exitCode: exitCode, details: details}
}

func newHintedError(err error, code canonicalErrorCode, exitCode int, hint string) error {
	return &detailedError{err: err, code: code, exitCode: exitCode, hint: hint}
}

func pathResolveError(err error) error {
	var resolveErr *pagepath.ResolveError
	if errors.As(err, &resolveErr) {
		return newDetailedError(err, errorRequestInvalid, ExitUsage, map[string]any{
			"path":       resolveErr.Path,
			"candidates": resolveErr.Candidates,
		})
	}
	return err
}

func emitCommandError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	desc := classifyError(err)

	// If the error was already rendered (e.g. by timeline ErrorFooter),
	// skip the duplicate print but still return the exit code.
	var displayed *displayedError
	if errors.As(err, &displayed) {
		return &ExitError{Code: desc.ExitCode, Err: err}
	}

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
		if results := batchResultsFromError(err); len(results) > 0 {
			writeBatchResultLines(os.Stderr, nil, results)
		}
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

	var detailed *detailedError
	if errors.As(err, &detailed) {
		if detailed.code != "" {
			desc.Code = detailed.code
		} else {
			desc.Code = errorRequestInvalid
		}
		desc.Message = detailed.Error()
		desc.ExitCode = detailed.exitCode
		if desc.ExitCode == 0 {
			desc.ExitCode = ExitUsage
		}
		desc.Details = detailed.details
		desc.Hint = detailed.hint
		// A hint must not hide what the API said: keep its status, field
		// errors, and per-operation batch results.
		var apiErr *api.Error
		if errors.As(detailed.err, &apiErr) {
			desc.HTTPStatus = apiErr.StatusCode
			desc.ValidationErrors = apiErr.Errors
			if results := apiErr.BatchResults(); len(results) > 0 && desc.Details["results"] == nil {
				desc.Details = cloneDetails(desc.Details)
				if desc.Details == nil {
					desc.Details = map[string]any{}
				}
				desc.Details["results"] = results
			}
		}
		return desc
	}

	var resolveErr *pagepath.ResolveError
	if errors.As(err, &resolveErr) {
		desc.Code = errorRequestInvalid
		desc.ExitCode = ExitUsage
		desc.Message = resolveErr.Error()
		desc.Details = map[string]any{
			"path":       resolveErr.Path,
			"candidates": resolveErr.Candidates,
		}
		return desc
	}

	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		desc.HTTPStatus = apiErr.StatusCode
		desc.Details = cloneDetails(apiErr.Details)
		desc.ValidationErrors = apiErr.Errors
		if results := apiErr.BatchResults(); len(results) > 0 {
			if desc.Details == nil {
				desc.Details = map[string]any{}
			}
			desc.Details["results"] = results
		}

		switch apiErr.StatusCode {
		case 400:
			desc.Code = errorRequestInvalid
			desc.ExitCode = ExitValidation
			desc.Hint = "check request parameters and payload fields"
		case 401:
			desc.Code = errorAuthUnauthorized
			desc.ExitCode = ExitAuth
			desc.Hint = "refresh credentials with `nimbu auth login`"
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
		case 412:
			desc.Code = errorConflict
			desc.ExitCode = ExitValidation
			desc.Hint = "page changed since it was read; retry the command"
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
		if apiErr.StatusCode == 422 {
			applyFileRefOpError(&desc, apiErr.BatchResults())
		}
		if hint := overlayHintFrom(err); hint != "" {
			desc.Hint = hint
			desc.Code = errorRequestValidation
		}
		return desc
	}

	var readonlyErr *api.ReadonlyError
	if errors.As(err, &readonlyErr) {
		desc.Code = errorReadonly
		desc.ExitCode = ExitUsage
		desc.Message = readonlyErr.Error()
		desc.Hint = "unset NIMBU_READONLY or remove --readonly to allow write operations"
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
		desc.Hint = "run `nimbu auth login`"
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

func cloneDetails(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

// applyFileRefOpError reclassifies a batch that failed on a single FileRef
// copy. The server reports only FileRef copier failures with the bare
// "not_found" and "unauthorized" op codes; path misses use "path_not_found".
func applyFileRefOpError(desc *errorDescriptor, results []api.BatchOpResult) {
	var failed *api.BatchOpError
	for _, result := range results {
		if result.Error == nil {
			continue
		}
		if failed != nil {
			return
		}
		failed = result.Error
	}
	if failed == nil {
		return
	}
	switch failed.Code {
	case "not_found":
		desc.Code = errorNotFound
		desc.ExitCode = ExitNotFound
		desc.Hint = "the FileRef source does not exist; check the site and upload id, or re-upload the file and use the new upload URL"
	case "unauthorized":
		desc.Code = errorAuthForbidden
		desc.ExitCode = ExitAuthz
		desc.Hint = "this token may not copy from the FileRef source; use an upload from this site, or a user token with access to the source site"
	}
}

func batchResultsFromError(err error) []api.BatchOpResult {
	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		return apiErr.BatchResults()
	}
	return nil
}
