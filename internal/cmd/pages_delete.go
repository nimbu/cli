package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// PagesDeleteCmd deletes a page.
type PagesDeleteCmd struct {
	Page string `arg:"" help:"Page ID or slug"`
}

// Run executes the delete command.
func (c *PagesDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "delete page"); err != nil {
		return err
	}

	if err := requireForce(flags, "page "+c.Page); err != nil {
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

	path := "/pages/" + url.PathEscape(c.Page)
	if err := client.Delete(ctx, path, nil); err != nil {
		return fmt.Errorf("delete page: %w", err)
	}

	return output.Print(ctx, output.SuccessPayload("page deleted"), []any{c.Page, "deleted"}, func() error {
		_, err := output.Fprintf(ctx, "Deleted page %s\n", c.Page)
		return err
	})
}
