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
	Version    string
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

// WithVersion returns a copy of the client with the version set.
func (c *Client) WithVersion(version string) *Client {
	clone := *c
	clone.Version = version
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
	var contentType string
	var contentLength int64 = -1
	var getBody func() (io.ReadCloser, error)
	if body != nil {
		switch custom := body.(type) {
		case RequestBody:
			bodyReader = custom.Reader
			contentType = custom.ContentType
			contentLength = custom.ContentLength
			getBody = custom.GetBody
		case *RequestBody:
			if custom != nil {
				bodyReader = custom.Reader
				contentType = custom.ContentType
				contentLength = custom.ContentLength
				getBody = custom.GetBody
			}
		default:
			data, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("encode body: %w", err)
			}
			bodyReader = bytes.NewReader(data)
			contentType = "application/json"
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	meta := operationMetaFromOptions(method, reqOpts)
	req = req.WithContext(withOperationMeta(req.Context(), meta))
	if contentLength >= 0 {
		req.ContentLength = contentLength
	}
	if getBody != nil {
		req.GetBody = getBody
	}

	// Set headers
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	} else if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Auth header
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	// Client version header (triggers rich serialization with __type metadata)
	if c.Version != "" {
		req.Header.Set("X-Nimbu-Client-Version", c.Version)
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

// DownloadURL fetches a file from a URL (absolute or relative to BaseURL).
// It appends raw=true to bypass CDN image optimization, resolves relative URLs,
// and attaches auth headers when the URL targets the API host.
func (c *Client) DownloadURL(ctx context.Context, rawURL string) (*http.Response, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, "", err
	}
	attachAuth := false
	if !parsed.IsAbs() {
		base, err := url.Parse(c.BaseURL)
		if err != nil {
			return nil, "", err
		}
		parsed = base.ResolveReference(parsed)
		attachAuth = true
	} else if base, err := url.Parse(c.BaseURL); err == nil && strings.EqualFold(base.Host, parsed.Host) {
		attachAuth = true
	}
	q := parsed.Query()
	q.Set("raw", "true")
	parsed.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, "", err
	}
	if attachAuth && c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if attachAuth && c.Site != "" {
		req.Header.Set("X-Nimbu-Site", c.Site)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	return resp, parsed.String(), nil
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
