package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// RedirectsDeleteCmd deletes a redirect.
type RedirectsDeleteCmd struct {
	Redirect string `arg:"" help:"Redirect ID"`
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

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, output.SuccessPayload("redirect deleted"))
	}

	if mode.Plain {
		return output.Plain(ctx, c.Redirect, "deleted")
	}

	fmt.Printf("Deleted redirect: %s\n", c.Redirect)
	return nil
}
