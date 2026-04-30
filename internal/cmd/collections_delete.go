package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// CollectionsDeleteCmd deletes a collection.
type CollectionsDeleteCmd struct {
	Collection string `required:"" help:"Collection ID or slug"`
}

// Run executes the delete command.
func (c *CollectionsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "delete collection"); err != nil {
		return err
	}

	if err := requireForce(flags, "collection "+c.Collection); err != nil {
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

	path := "/collections/" + url.PathEscape(c.Collection)
	if err := client.Delete(ctx, path, nil); err != nil {
		return fmt.Errorf("delete collection: %w", err)
	}

	return output.Print(ctx, output.SuccessPayload("collection deleted"), []any{c.Collection, "deleted"}, func() error {
		_, err := output.Fprintf(ctx, "Deleted collection: %s\n", c.Collection)
		return err
	})
}
