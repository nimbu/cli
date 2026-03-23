package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// ThemeTemplatesDeleteCmd deletes a template.
type ThemeTemplatesDeleteCmd struct {
	Theme string `arg:"" help:"Theme ID"`
	Name  string `arg:"" help:"Template name including extension"`
}

// Run executes the delete command.
func (c *ThemeTemplatesDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "delete template"); err != nil {
		return err
	}

	if err := requireForce(flags, "template "+c.Name); err != nil {
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

	path := "/themes/" + url.PathEscape(c.Theme) + "/templates/" + url.PathEscape(c.Name)
	if err := client.Delete(ctx, path, nil); err != nil {
		return fmt.Errorf("delete template: %w", err)
	}

	return output.Print(ctx, output.SuccessPayload("deleted "+c.Name), []any{c.Name, "deleted"}, func() error {
		_, err := output.Fprintf(ctx, "Deleted: %s\n", c.Name)
		return err
	})
}
