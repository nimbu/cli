package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// ThemeAssetsDeleteCmd deletes an asset.
type ThemeAssetsDeleteCmd struct {
	Theme string `required:"" help:"Theme ID"`
	Path  string `required:"" help:"Asset path"`
}

// Run executes the delete command.
func (c *ThemeAssetsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "delete asset"); err != nil {
		return err
	}

	if err := requireForce(flags, "asset "+c.Path); err != nil {
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

	path := "/themes/" + url.PathEscape(c.Theme) + "/assets/" + url.PathEscape(c.Path)
	if err := client.Delete(ctx, path, nil); err != nil {
		return fmt.Errorf("delete asset: %w", err)
	}

	return output.Print(ctx, output.SuccessPayload("deleted "+c.Path), []any{c.Path, "deleted"}, func() error {
		_, err := output.Fprintf(ctx, "Deleted: %s\n", c.Path)
		return err
	})
}
