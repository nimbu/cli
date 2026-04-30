package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// RedirectsDeleteCmd deletes a redirect.
type RedirectsDeleteCmd struct {
	Redirect string `required:"" help:"Redirect ID"`
}

// Run executes the delete command.
func (c *RedirectsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "delete redirect"); err != nil {
		return err
	}

	if err := requireForce(flags, "redirect "+c.Redirect); err != nil {
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

	path := "/redirects/" + url.PathEscape(c.Redirect)
	if err := client.Delete(ctx, path, nil); err != nil {
		return fmt.Errorf("delete redirect: %w", err)
	}

	return output.Print(ctx, output.SuccessPayload("redirect deleted"), []any{c.Redirect, "deleted"}, func() error {
		_, err := output.Fprintf(ctx, "Deleted redirect: %s\n", c.Redirect)
		return err
	})
}
