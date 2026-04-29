package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// APICmd provides raw API access.
type APICmd struct {
	Method string `required:"" help:"HTTP method (GET, POST, PUT, PATCH, DELETE)"`
	Path   string `required:"" help:"API path (e.g., /channels)"`
	Data   string `help:"Request body (JSON string)" short:"d"`
	File   string `help:"Read request body from file" type:"existingfile"`
}

// Run executes the raw API command.
func (c *APICmd) Run(ctx context.Context, flags *RootFlags) error {
	method := strings.ToUpper(c.Method)

	// Validate method
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		// OK
	default:
		return fmt.Errorf("unsupported method: %s", method)
	}

	// Check readonly for write methods
	if method != http.MethodGet {
		if err := requireWrite(flags, fmt.Sprintf("use %s", method)); err != nil {
			return err
		}
	}

	// Site is optional for API command - use global --site if set
	site, _ := RequireSite(ctx, "")
	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	// Build request body
	var body any
	if c.Data != "" {
		if err := json.Unmarshal([]byte(c.Data), &body); err != nil {
			return fmt.Errorf("parse data: %w", err)
		}
	} else if c.File != "" {
		f, err := os.Open(c.File)
		if err != nil {
			return fmt.Errorf("open file: %w", err)
		}
		defer func() { _ = f.Close() }()
		if err := json.NewDecoder(f).Decode(&body); err != nil {
			return fmt.Errorf("decode file: %w", err)
		}
	}

	// Ensure path starts with /
	path := c.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// Execute request
	resp, err := client.RawRequest(ctx, method, path, body)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	// Check for error status
	if resp.StatusCode >= 400 {
		return decodeRawAPIError(resp.StatusCode, respBody)
	}

	// Output response
	mode := output.FromContext(ctx)

	// If JSON mode or response is JSON, pretty print it
	if len(respBody) > 0 {
		var data any
		if err := json.Unmarshal(respBody, &data); err == nil {
			if mode.JSON || !mode.Plain {
				return output.JSON(ctx, data)
			}
		}
	}

	// Plain or non-JSON response
	if _, err := output.Fprintln(ctx, string(respBody)); err != nil {
		return err
	}
	return nil
}

func decodeRawAPIError(statusCode int, body []byte) error {
	var payload struct {
		Message string                `json:"message"`
		Error   string                `json:"error"`
		Code    string                `json:"code"`
		Errors  []api.ValidationError `json:"errors"`
		Details map[string]any        `json:"details"`
	}

	msg := strings.TrimSpace(string(body))
	if err := json.Unmarshal(body, &payload); err == nil {
		if payload.Message != "" {
			msg = payload.Message
		} else if payload.Error != "" {
			msg = payload.Error
		}
		if payload.Code == "" && statusCode == http.StatusNotFound {
			payload.Code = "object_not_found"
		}
		return &api.Error{
			StatusCode: statusCode,
			Code:       payload.Code,
			Message:    msg,
			Details:    payload.Details,
			Errors:     payload.Errors,
		}
	}

	if msg == "" {
		msg = fmt.Sprintf("HTTP %d", statusCode)
	}
	return &api.Error{StatusCode: statusCode, Message: msg}
}
