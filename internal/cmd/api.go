package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// APICmd provides raw API access. Verb-first commands are canonical; Legacy
// preserves the original --method/--path interface.
type APICmd struct {
	Get    APIGetCmd    `cmd:"" aliases:"GET" help:"Send a GET request"`
	Post   APIPostCmd   `cmd:"" aliases:"POST" help:"Send a POST request"`
	Put    APIPutCmd    `cmd:"" aliases:"PUT" help:"Send a PUT request"`
	Patch  APIPatchCmd  `cmd:"" aliases:"PATCH" help:"Send a PATCH request"`
	Delete APIDeleteCmd `cmd:"" aliases:"DELETE" help:"Send a DELETE request (requires --force)"`
	Legacy APILegacyCmd `cmd:"" default:"withargs" hidden:""`
}

type APIRequestFlags struct {
	Data   string `help:"Request JSON: inline, @file, or - for stdin" short:"d"`
	File   string `help:"Read request JSON from file (legacy; use --data @file)" type:"existingfile"`
	All    bool   `help:"Fetch every page of a JSON array response"`
	Output string `help:"Write an exact binary response to FILE or - for stdout"`
}

type APIGetCmd struct {
	Path string `arg:"" required:"" help:"API path"`
	APIRequestFlags
}

func (c *APIGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runRawAPI(ctx, flags, http.MethodGet, c.Path, c.APIRequestFlags)
}

type APIPostCmd struct {
	Path string `arg:"" required:"" help:"API path"`
	APIRequestFlags
}

func (c *APIPostCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runRawAPI(ctx, flags, http.MethodPost, c.Path, c.APIRequestFlags)
}

type APIPutCmd struct {
	Path string `arg:"" required:"" help:"API path"`
	APIRequestFlags
}

func (c *APIPutCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runRawAPI(ctx, flags, http.MethodPut, c.Path, c.APIRequestFlags)
}

type APIPatchCmd struct {
	Path string `arg:"" required:"" help:"API path"`
	APIRequestFlags
}

func (c *APIPatchCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runRawAPI(ctx, flags, http.MethodPatch, c.Path, c.APIRequestFlags)
}

type APIDeleteCmd struct {
	Path string `arg:"" required:"" help:"API path"`
	APIRequestFlags
}

func (c *APIDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runRawAPI(ctx, flags, http.MethodDelete, c.Path, c.APIRequestFlags)
}

type APILegacyCmd struct {
	Method string `required:"" help:"HTTP method (GET, POST, PUT, PATCH, DELETE)"`
	Path   string `required:"" help:"API path (e.g., /channels)"`
	APIRequestFlags
}

func (c *APILegacyCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runRawAPI(ctx, flags, c.Method, c.Path, c.APIRequestFlags)
}

func runRawAPI(ctx context.Context, flags *RootFlags, rawMethod, rawPath string, request APIRequestFlags) error {
	method := strings.ToUpper(rawMethod)
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return fmt.Errorf("unsupported method: %s", method)
	}
	if method != http.MethodGet {
		if err := requireWrite(flags, fmt.Sprintf("use %s", method)); err != nil {
			return err
		}
	}
	if method == http.MethodDelete {
		if err := requireForce(flags, rawPath); err != nil {
			return err
		}
	}
	if request.All && method != http.MethodGet {
		return fmt.Errorf("--all is only valid for GET")
	}
	if request.All && request.Output != "" {
		return fmt.Errorf("--all cannot be combined with --output")
	}
	if request.Output != "" && output.FromContext(ctx).JSON {
		return fmt.Errorf("--output cannot be combined with --json")
	}

	site, _ := RequireSite(ctx, "")
	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}
	body, err := rawAPIRequestBody(request)
	if err != nil {
		return err
	}
	path := "/" + strings.TrimPrefix(rawPath, "/")

	if request.All {
		return outputRawAPIAll(ctx, client, path)
	}
	resp, err := client.RawRequest(ctx, method, path, body)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		responseBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("read error response: %w", readErr)
		}
		return decodeRawAPIError(resp.StatusCode, responseBody)
	}
	if request.Output != "" {
		return writeDownloadResponse(ctx, resp.Body, request.Output, flags != nil && flags.Force)
	}
	return outputRawAPIResponse(ctx, resp.Body)
}

func rawAPIRequestBody(request APIRequestFlags) (any, error) {
	if request.Data != "" && request.File != "" {
		return nil, fmt.Errorf("--data and --file are mutually exclusive")
	}
	source := request.Data
	if request.File != "" {
		source = "@" + request.File
	}
	if source == "" {
		return nil, nil
	}

	var data []byte
	var err error
	switch {
	case source == "-":
		data, err = io.ReadAll(os.Stdin)
	case strings.HasPrefix(source, "@"):
		data, err = os.ReadFile(strings.TrimPrefix(source, "@"))
	default:
		data = []byte(source)
	}
	if err != nil {
		return nil, fmt.Errorf("read request data: %w", err)
	}
	var body any
	if err := json.Unmarshal(data, &body); err != nil {
		return nil, fmt.Errorf("parse data: %w", err)
	}
	return body, nil
}

func outputRawAPIAll(ctx context.Context, client *api.Client, path string) error {
	documents, err := api.List[json.RawMessage](ctx, client, path)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	return output.JSON(ctx, documents)
}

func outputRawAPIResponse(ctx context.Context, body io.Reader) error {
	responseBody, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if len(responseBody) == 0 {
		return nil
	}
	var data any
	if json.Unmarshal(responseBody, &data) == nil {
		return output.JSON(ctx, data)
	}
	_, err = output.WriterFromContext(ctx).Out.Write(responseBody)
	return err
}

func writeDownloadResponse(ctx context.Context, reader io.Reader, destination string, force bool) error {
	if destination == "-" {
		writer := output.WriterFromContext(ctx)
		if writer.IsTTY() {
			return fmt.Errorf("refusing to write binary data to a terminal; use --output FILE")
		}
		_, err := io.Copy(writer.Out, reader)
		return err
	}
	if !force {
		if _, err := os.Stat(destination); err == nil {
			return fmt.Errorf("output file already exists: %s (use --force to overwrite)", destination)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect output file: %w", err)
		}
	}
	dir := filepath.Dir(destination)
	tmp, err := os.CreateTemp(dir, ".nimbu-download-*")
	if err != nil {
		return fmt.Errorf("create temporary output: %w", err)
	}
	tmpName := tmp.Name()
	keep := false
	defer func() {
		_ = tmp.Close()
		if !keep {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := io.Copy(tmp, reader); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close output: %w", err)
	}
	if err := os.Rename(tmpName, destination); err != nil {
		return fmt.Errorf("install output: %w", err)
	}
	keep = true
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
