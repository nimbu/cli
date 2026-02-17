package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/output"
)

// PagesDeleteCmd deletes a page.
type PagesDeleteCmd struct {
	Page string `arg:"" help:"Page ID or slug"`
}

// Run executes the delete command.
func (c *PagesDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if flags.Readonly {
		return fmt.Errorf("write operations disabled in readonly mode")
	}

	if !flags.Force {
		return fmt.Errorf("delete requires --force flag")
	}

	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	if err := client.Delete(ctx, "/pages/"+c.Page, nil); err != nil {
		return fmt.Errorf("delete page: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, output.SuccessPayload("page deleted"))
	}

	if mode.Plain {
		return output.Plain(ctx, "deleted", c.Page)
	}

	fmt.Printf("Deleted page %s\n", c.Page)
	return nil
}
