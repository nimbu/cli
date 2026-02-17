package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// CollectionsCreateCmd creates a collection.
type CollectionsCreateCmd struct {
	File        string   `help:"Read collection JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. name=Summer, slug=summer)"`
}

// Run executes the create command.
func (c *CollectionsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "create collection"); err != nil {
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
	if err := client.Post(ctx, "/collections", body, &col); err != nil {
		return fmt.Errorf("create collection: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, col)
	}

	if mode.Plain {
		return output.Plain(ctx, col.ID, col.Slug, col.Name)
	}

	fmt.Printf("Created collection: %s (%s)\n", col.Name, col.ID)
	return nil
}
