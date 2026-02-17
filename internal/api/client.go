package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is the Nimbu API client.
type Client struct {
	BaseURL    string
	Token      string
	Site       string
	HTTPClient *http.Client
	Debug      bool
}

// New creates a new API client.
func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTPClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: NewRetryTransport(http.DefaultTransport),
		},
	}
}

// WithSite returns a copy of the client with the site set.
func (c *Client) WithSite(site string) *Client {
	clone := *c
	clone.Site = site
	return &clone
}

// WithTimeout returns a copy of the client with custom timeout.
func (c *Client) WithTimeout(timeout time.Duration) *Client {
	clone := *c
	clone.HTTPClient = &http.Client{
		Timeout:   timeout,
		Transport: c.HTTPClient.Transport,
	}
	return &clone
}

// WithDebug returns a copy of the client with debug logging enabled.
func (c *Client) WithDebug(debug bool) *Client {
	clone := *c
	clone.Debug = debug
	return &clone
}

// Request performs an HTTP request and decodes the JSON response.
func (c *Client) Request(ctx context.Context, method, path string, body any, result any, opts ...RequestOption) error {
	req, err := c.buildRequest(ctx, method, path, body, opts...)
	if err != nil {
		return err
	}

	return c.do(req, result)
}

// Get performs a GET request.
func (c *Client) Get(ctx context.Context, path string, result any, opts ...RequestOption) error {
	return c.Request(ctx, http.MethodGet, path, nil, result, opts...)
}

// Post performs a POST request.
func (c *Client) Post(ctx context.Context, path string, body any, result any, opts ...RequestOption) error {
	return c.Request(ctx, http.MethodPost, path, body, result, opts...)
}

// Put performs a PUT request.
func (c *Client) Put(ctx context.Context, path string, body any, result any, opts ...RequestOption) error {
	return c.Request(ctx, http.MethodPut, path, body, result, opts...)
}

// Patch performs a PATCH request.
func (c *Client) Patch(ctx context.Context, path string, body any, result any, opts ...RequestOption) error {
	return c.Request(ctx, http.MethodPatch, path, body, result, opts...)
}

// Delete performs a DELETE request.
func (c *Client) Delete(ctx context.Context, path string, result any, opts ...RequestOption) error {
	return c.Request(ctx, http.MethodDelete, path, nil, result, opts...)
}

func (c *Client) buildRequest(ctx context.Context, method, path string, body any, opts ...RequestOption) (*http.Request, error) {
	// Build URL
	u, err := url.Parse(c.BaseURL + path)
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}

	// Apply options
	reqOpts := &requestOptions{}
	for _, opt := range opts {
		opt(reqOpts)
	}

	// Add query params
	if len(reqOpts.Query) > 0 {
		q := u.Query()
		for k, v := range reqOpts.Query {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
	}

	// Build body
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	// Set headers
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Auth header
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	// Site header
	site := c.Site
	if reqOpts.Site != "" {
		site = reqOpts.Site
	}
	if site != "" {
		req.Header.Set("X-Nimbu-Site", site)
	}

	// Custom headers
	for k, v := range reqOpts.Headers {
		req.Header.Set(k, v)
	}

	return req, nil
}

func (c *Client) do(req *http.Request, result any) error {
	if c.Debug {
		slog.Debug("API request",
			"method", req.Method,
			"url", req.URL.String(),
		)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return &Error{
			StatusCode: 0,
			Message:    err.Error(),
			Err:        err,
		}
	}
	defer func() { _ = resp.Body.Close() }()

	// Read body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if c.Debug {
		slog.Debug("API response",
			"status", resp.StatusCode,
			"body", string(body),
		)
	}

	// Check for errors
	if resp.StatusCode >= 400 {
		return parseError(resp.StatusCode, body)
	}

	// Decode response
	if result != nil && len(body) > 0 {
		if err := json.Unmarshal(body, result); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}

	return nil
}

// RawRequest performs a request and returns the raw response.
func (c *Client) RawRequest(ctx context.Context, method, path string, body any, opts ...RequestOption) (*http.Response, error) {
	req, err := c.buildRequest(ctx, method, path, body, opts...)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, &Error{
			StatusCode: 0,
			Message:    err.Error(),
			Err:        err,
		}
	}

	return resp, nil
}
