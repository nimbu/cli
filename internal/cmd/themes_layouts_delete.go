package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// ThemeLayoutsDeleteCmd deletes a layout.
type ThemeLayoutsDeleteCmd struct {
	Theme string `arg:"" help:"Theme ID"`
	Name  string `arg:"" help:"Layout name including extension"`
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

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, output.SuccessPayload("deleted "+c.Name))
	}
	if mode.Plain {
		return output.Plain(ctx, c.Name, "deleted")
	}

	fmt.Printf("Deleted: %s\n", c.Name)
	return nil
}
