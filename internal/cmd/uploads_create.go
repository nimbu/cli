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
	Source      string   `optional:"" help:"Path to file to upload" name:"source"`
	File        string   `name:"file" short:"f" help:"Path to file to upload"`
	FileRef     string   `name:"file-ref" help:"Nimbu FileRef URI to copy (e.g. nimbu://site/uploads/id)"`
	Name        string   `help:"Override filename" short:"n"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. name=custom.jpg)"`
}

// Run executes the create command.
func (c *UploadsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "upload file"); err != nil {
		return err
	}

	sourceCount := 0
	for _, source := range []string{c.Source, c.File, c.FileRef} {
		if source != "" {
			sourceCount++
		}
	}
	if sourceCount > 1 {
		return fmt.Errorf("use only one of --source, --file, or --file-ref")
	}
	source := c.Source
	if source == "" {
		source = c.File
	}
	if source == "" && c.FileRef == "" {
		return fmt.Errorf("uploads create requires --source, --file, or --file-ref")
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
	if c.FileRef != "" && len(meta) > 0 {
		return fmt.Errorf("--name cannot be used with --file-ref")
	}

	if c.FileRef != "" {
		body := api.NewUploadCreateFileRefPayload(c.FileRef)
		var upload api.Upload
		if err := client.Post(ctx, "/uploads", body, &upload); err != nil {
			return fmt.Errorf("copy upload from FileRef: %w", err)
		}

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
		filename = filepath.Base(source)
	}

	task := output.ProgressFromContext(ctx).Transfer("upload "+filename, 0)
	content, err := os.ReadFile(source)
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
