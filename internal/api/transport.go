package api

import (
	"log/slog"
	"math"
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

	for attempt := 0; attempt <= t.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := t.calculateDelay(attempt, resp)
			slog.Debug("retrying request",
				"attempt", attempt,
				"delay", delay,
				"url", req.URL.String(),
			)
			time.Sleep(delay)
		}

		resp, err = t.Transport.RoundTrip(req)
		if err != nil {
			// Network error - retry
			continue
		}

		// Don't retry successful responses or client errors (4xx except 429)
		if resp.StatusCode < 500 && resp.StatusCode != 429 {
			return resp, nil
		}

		// Server error or rate limit - retry
		if attempt < t.MaxRetries {
			_ = resp.Body.Close()
		}
	}

	return resp, err
}

func (t *RetryTransport) calculateDelay(attempt int, resp *http.Response) time.Duration {
	// Check for Retry-After header
	if resp != nil {
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if seconds, err := strconv.Atoi(ra); err == nil {
				return time.Duration(seconds) * time.Second
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
