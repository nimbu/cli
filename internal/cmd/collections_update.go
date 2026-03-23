package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// CollectionsUpdateCmd updates a collection.
type CollectionsUpdateCmd struct {
	Collection  string   `arg:"" help:"Collection ID or slug"`
	File        string   `help:"Read collection JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. name=Summer, slug=summer)"`
}

// Run executes the update command.
func (c *CollectionsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "update collection"); err != nil {
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

	body, err := readJSONBodyInput(c.File, c.Assignments)
	if err != nil {
		return err
	}

	var col api.Collection
	path := "/collections/" + url.PathEscape(c.Collection)
	if err := client.Put(ctx, path, body, &col); err != nil {
		return fmt.Errorf("update collection: %w", err)
	}

	return output.Print(ctx, col, []any{col.ID, col.Slug, col.Name}, func() error {
		_, err := output.Fprintf(ctx, "Updated collection: %s (%s)\n", col.Name, col.ID)
		return err
	})
}
