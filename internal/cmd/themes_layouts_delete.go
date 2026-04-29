package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// ThemeLayoutsDeleteCmd deletes a layout.
type ThemeLayoutsDeleteCmd struct {
	Theme string `required:"" help:"Theme ID"`
	Name  string `required:"" help:"Layout name including extension"`
}

// Run executes the delete command.
func (c *ThemeLayoutsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "delete layout"); err != nil {
		return err
	}

	if err := requireForce(flags, "layout "+c.Name); err != nil {
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

	path := "/themes/" + url.PathEscape(c.Theme) + "/layouts/" + url.PathEscape(c.Name)
	if err := client.Delete(ctx, path, nil); err != nil {
		return fmt.Errorf("delete layout: %w", err)
	}

	return output.Print(ctx, output.SuccessPayload("deleted "+c.Name), []any{c.Name, "deleted"}, func() error {
		_, err := output.Fprintf(ctx, "Deleted: %s\n", c.Name)
		return err
	})
}
