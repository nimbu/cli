package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Error represents an API error.
type Error struct {
	StatusCode int               `json:"status_code"`
	Code       string            `json:"code,omitempty"`
	Message    string            `json:"message"`
	Details    map[string]any    `json:"details,omitempty"`
	Errors     []ValidationError `json:"errors,omitempty"`
	Err        error             `json:"-"`
}

// ValidationError represents a field-level validation error.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

func (e *Error) Error() string {
	if len(e.Errors) > 0 {
		msgs := make([]string, len(e.Errors))
		for i, ve := range e.Errors {
			msgs[i] = fmt.Sprintf("%s: %s", ve.Field, ve.Message)
		}
		return fmt.Sprintf("%s (%s)", e.Message, strings.Join(msgs, "; "))
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	return e.Err
}

// IsNotFound returns true if this is a 404 error.
func (e *Error) IsNotFound() bool {
	return e.StatusCode == 404
}

// IsUnauthorized returns true if this is a 401 error.
func (e *Error) IsUnauthorized() bool {
	return e.StatusCode == 401
}

// IsForbidden returns true if this is a 403 error.
func (e *Error) IsForbidden() bool {
	return e.StatusCode == 403
}

// IsRateLimit returns true if this is a 429 error.
func (e *Error) IsRateLimit() bool {
	return e.StatusCode == 429
}

// IsValidation returns true if this is a 422 validation error.
func (e *Error) IsValidation() bool {
	return e.StatusCode == 422
}

func parseError(statusCode int, body []byte) *Error {
	apiErr := &Error{StatusCode: statusCode}

	// Try to parse as JSON error
	var errResp struct {
		Error   string            `json:"error"`
		Message string            `json:"message"`
		Code    string            `json:"code"`
		Errors  []ValidationError `json:"errors"`
		Details map[string]any    `json:"details"`
	}

	if err := json.Unmarshal(body, &errResp); err == nil {
		if errResp.Error != "" {
			apiErr.Message = errResp.Error
		} else if errResp.Message != "" {
			apiErr.Message = errResp.Message
		}
		apiErr.Code = errResp.Code
		apiErr.Errors = errResp.Errors
		apiErr.Details = errResp.Details
	}

	// Fallback message based on status code
	if apiErr.Message == "" {
		switch statusCode {
		case 400:
			apiErr.Message = "bad request"
		case 401:
			apiErr.Message = "unauthorized"
		case 403:
			apiErr.Message = "forbidden"
		case 404:
			apiErr.Message = "not found"
		case 422:
			apiErr.Message = "validation error"
		case 429:
			apiErr.Message = "rate limit exceeded"
		case 500:
			apiErr.Message = "internal server error"
		case 502:
			apiErr.Message = "bad gateway"
		case 503:
			apiErr.Message = "service unavailable"
		default:
			apiErr.Message = fmt.Sprintf("HTTP %d", statusCode)
		}
	}

	return apiErr
}

// Common error checks
var (
	ErrNotFound     = errors.New("not found")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrRateLimit    = errors.New("rate limit exceeded")
)

// IsNotFound checks if any error in the chain is a 404.
func IsNotFound(err error) bool {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr.IsNotFound()
	}
	return errors.Is(err, ErrNotFound)
}

// IsUnauthorized checks if any error in the chain is a 401.
func IsUnauthorized(err error) bool {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr.IsUnauthorized()
	}
	return errors.Is(err, ErrUnauthorized)
}

// IsForbidden checks if any error in the chain is a 403.
func IsForbidden(err error) bool {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr.IsForbidden()
	}
	return errors.Is(err, ErrForbidden)
}

// IsRateLimit checks if any error in the chain is a 429.
func IsRateLimit(err error) bool {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr.IsRateLimit()
	}
	return errors.Is(err, ErrRateLimit)
}
