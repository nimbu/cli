package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// UploadsGetCmd gets upload details.
type UploadsGetCmd struct {
	ID string `arg:"" help:"Upload ID"`
}

// Run executes the get command.
func (c *UploadsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	var upload api.Upload
	path := "/uploads/" + url.PathEscape(c.ID)
	if err := client.Get(ctx, path, &upload); err != nil {
		return fmt.Errorf("get upload: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, upload)
	}

	if mode.Plain {
		return output.Plain(ctx, upload.ID, upload.Name, upload.URL, upload.Size, upload.MimeType)
	}

	fmt.Printf("ID:        %s\n", upload.ID)
	fmt.Printf("Name:      %s\n", upload.Name)
	if upload.URL != "" {
		fmt.Printf("URL:       %s\n", upload.URL)
	}
	if upload.Size > 0 {
		fmt.Printf("Size:      %d\n", upload.Size)
	}
	if upload.MimeType != "" {
		fmt.Printf("MIME Type: %s\n", upload.MimeType)
	}
	if !upload.CreatedAt.IsZero() {
		fmt.Printf("Created:   %s\n", upload.CreatedAt.Format("2006-01-02 15:04:05"))
	}

	return nil
}
