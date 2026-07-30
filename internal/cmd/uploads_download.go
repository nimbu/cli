package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
)

type UploadsDownloadCmd struct {
	ID     string `required:"" help:"Upload ID"`
	Output string `help:"Write the file to this path, or - for stdout" required:""`
}

func (c *UploadsDownloadCmd) Run(ctx context.Context, flags *RootFlags) error {
	client, err := auditedClient(ctx)
	if err != nil {
		return err
	}
	var upload api.Upload
	if err := client.Get(ctx, "/uploads/"+url.PathEscape(c.ID), &upload); err != nil {
		return fmt.Errorf("get upload: %w", err)
	}
	if upload.URL == "" {
		return fmt.Errorf("upload %s has no download URL", c.ID)
	}
	response, _, err := client.DownloadURL(ctx, upload.URL)
	if err != nil {
		return fmt.Errorf("download upload: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 400 {
		return fmt.Errorf("download upload: HTTP %d", response.StatusCode)
	}
	return writeDownloadResponse(ctx, response.Body, c.Output, flags != nil && flags.Force)
}
