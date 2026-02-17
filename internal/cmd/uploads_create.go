package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// UploadsCreateCmd uploads a file.
type UploadsCreateCmd struct {
	File        string   `arg:"" help:"Path to file to upload"`
	Name        string   `help:"Override filename" short:"n"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. name=custom.jpg)"`
}

// Run executes the create command.
func (c *UploadsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "upload file"); err != nil {
		return err
	}

	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	// Open file
	f, err := os.Open(c.File)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = f.Close() }()

	meta := map[string]any{}
	if c.Name != "" {
		meta["name"] = c.Name
	}
	if len(c.Assignments) > 0 {
		inlineBody, err := parseInlineAssignments(c.Assignments)
		if err != nil {
			return err
		}
		meta, err = mergeJSONBodies(inlineBody, meta)
		if err != nil {
			return fmt.Errorf("merge inline assignments: %w", err)
		}
		for key := range meta {
			if key != "name" {
				return fmt.Errorf("uploads create only supports name=<value> inline assignment")
			}
		}
	}

	// Determine filename
	filename := ""
	if rawName, exists := meta["name"]; exists {
		name, ok := rawName.(string)
		if !ok {
			return fmt.Errorf("uploads create requires name to be a string")
		}
		filename = name
	}
	if filename == "" {
		filename = filepath.Base(c.File)
	}

	// Create multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return fmt.Errorf("create form file: %w", err)
	}

	if _, err := io.Copy(part, f); err != nil {
		return fmt.Errorf("copy file content: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("close multipart writer: %w", err)
	}

	// Build request manually for multipart upload
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.BaseURL+"/uploads", body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+client.Token)
	if client.Site != "" {
		req.Header.Set("X-Nimbu-Site", client.Site)
	}

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload file: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var upload api.Upload
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if err := json.Unmarshal(respBody, &upload); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, upload)
	}

	if mode.Plain {
		return output.Plain(ctx, upload.ID, upload.Name, upload.URL)
	}

	fmt.Printf("Uploaded: %s\n", upload.Name)
	fmt.Printf("ID:       %s\n", upload.ID)
	if upload.URL != "" {
		fmt.Printf("URL:      %s\n", upload.URL)
	}

	return nil
}
