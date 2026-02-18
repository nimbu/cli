package api

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"
	"time"
)

// RetryTransport wraps an http.RoundTripper with retry logic.
type RetryTransport struct {
	Transport  http.RoundTripper
	MaxRetries int
	BaseDelay  time.Duration
}

// NewRetryTransport creates a new RetryTransport.
func NewRetryTransport(transport http.RoundTripper) *RetryTransport {
	return &RetryTransport{
		Transport:  transport,
		MaxRetries: 3,
		BaseDelay:  500 * time.Millisecond,
	}
}

// RoundTrip implements http.RoundTripper with retry logic.
func (t *RetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error
	meta := operationMetaFromContext(req.Context())

	for attempt := 0; attempt <= t.MaxRetries; attempt++ {
		retryReq, retryErr := cloneRequestForAttempt(req, attempt)
		if retryErr != nil {
			if resp != nil {
				return resp, retryErr
			}
			return nil, retryErr
		}

		if attempt > 0 {
			delay := t.calculateDelay(attempt, resp)
			slog.Debug("retrying request",
				"attempt", attempt,
				"delay", delay,
				"url", req.URL.String(),
				"operation_class", meta.Class,
				"idempotent", meta.Idempotent,
			)
			select {
			case <-time.After(delay):
			case <-req.Context().Done():
				if resp != nil {
					return resp, req.Context().Err()
				}
				return nil, req.Context().Err()
			}
		}

		resp, err = t.Transport.RoundTrip(retryReq)
		if err != nil {
			if !shouldRetryError(err, meta) {
				return nil, err
			}
			continue
		}

		if !shouldRetryStatus(resp.StatusCode, meta) {
			return resp, nil
		}

		if attempt < t.MaxRetries {
			_ = resp.Body.Close()
		}
	}

	return resp, err
}

func cloneRequestForAttempt(req *http.Request, attempt int) (*http.Request, error) {
	if attempt == 0 {
		return req, nil
	}
	if req.Body == nil {
		return req.Clone(req.Context()), nil
	}
	if req.GetBody == nil {
		return nil, errors.New("cannot retry request with non-rewindable body")
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	cloned := req.Clone(req.Context())
	cloned.Body = body
	return cloned, nil
}

func shouldRetryError(err error, meta operationMeta) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	if meta.Class == OperationRead {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if meta.Class == OperationMutate {
			return meta.Idempotent && netErr.Timeout()
		}
		return false
	}

	if meta.Class == OperationMutate {
		return meta.Idempotent
	}

	return false
}

func shouldRetryStatus(status int, meta operationMeta) bool {
	if status == http.StatusRequestTimeout {
		if meta.Class == OperationRead {
			return true
		}
		return meta.Class == OperationMutate && meta.Idempotent
	}

	if status == http.StatusTooManyRequests {
		return true
	}

	if status < 500 {
		return false
	}

	switch meta.Class {
	case OperationRead:
		return true
	case OperationMutate:
		return meta.Idempotent
	case OperationDestructive:
		return false
	default:
		return false
	}
}

func (t *RetryTransport) calculateDelay(attempt int, resp *http.Response) time.Duration {
	// Check for Retry-After header
	if resp != nil {
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if seconds, err := strconv.Atoi(ra); err == nil {
				return time.Duration(seconds) * time.Second
			}
			if at, err := http.ParseTime(ra); err == nil {
				delay := time.Until(at)
				if delay > 0 {
					return delay
				}
			}
		}
	}

	// Exponential backoff with jitter
	delay := t.BaseDelay * time.Duration(math.Pow(2, float64(attempt-1)))
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}

	return delay
}
