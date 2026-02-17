package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// UploadsListCmd lists uploads.
type UploadsListCmd struct{}

// Run executes the list command.
func (c *UploadsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	uploads, err := api.List[api.Upload](ctx, client, "/uploads")
	if err != nil {
		return fmt.Errorf("list uploads: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, uploads)
	}

	if mode.Plain {
		for _, u := range uploads {
			if err := output.Plain(ctx, u.ID, u.Name, u.URL, u.Size); err != nil {
				return err
			}
		}
		return nil
	}

	fields := []string{"id", "name", "size", "mime_type"}
	headers := []string{"ID", "NAME", "SIZE", "MIME TYPE"}
	return output.WriteTable(ctx, uploads, fields, headers)
}
