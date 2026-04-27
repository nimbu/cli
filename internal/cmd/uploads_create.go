package cmd

import (
	"context"
	"fmt"
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

	task := output.ProgressFromContext(ctx).Transfer("upload "+filename, 0)
	content, err := os.ReadFile(c.File)
	if err != nil {
		task.Fail(err)
		return err
	}
	task.SetTotal(int64(len(content)))
	task.Add(int64(len(content)))
	body := api.NewUploadCreatePayload(filename, content, "")
	var upload api.Upload
	if err := client.Post(ctx, "/uploads", body, &upload); err != nil {
		task.Fail(err)
		return fmt.Errorf("upload file: %w", err)
	}
	task.Done("done")

	return output.Print(ctx, upload, []any{upload.ID, upload.Name, upload.URL}, func() error {
		if _, err := output.Fprintf(ctx, "Uploaded: %s\n", upload.Name); err != nil {
			return err
		}
		if _, err := output.Fprintf(ctx, "ID:       %s\n", upload.ID); err != nil {
			return err
		}
		if upload.URL != "" {
			if _, err := output.Fprintf(ctx, "URL:      %s\n", upload.URL); err != nil {
				return err
			}
		}
		return nil
	})
}
