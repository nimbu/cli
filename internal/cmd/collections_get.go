package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// CollectionsGetCmd gets a collection by ID or slug.
type CollectionsGetCmd struct {
	Collection string `arg:"" help:"Collection ID or slug"`
}

// Run executes the get command.
func (c *CollectionsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	var col api.Collection
	path := "/collections/" + url.PathEscape(c.Collection)
	if err := client.Get(ctx, path, &col); err != nil {
		return fmt.Errorf("get collection: %w", err)
	}

	var created, updated string
	if !col.CreatedAt.IsZero() {
		created = col.CreatedAt.Format("2006-01-02 15:04:05")
	}
	if !col.UpdatedAt.IsZero() {
		updated = col.UpdatedAt.Format("2006-01-02 15:04:05")
	}

	return output.Detail(ctx, col, []any{col.ID, col.Slug, col.Name, col.Status, col.Type}, []output.Field{
		output.FAlways("ID", col.ID),
		output.FAlways("Slug", col.Slug),
		output.FAlways("Name", col.Name),
		output.F("Description", col.Description),
		output.FAlways("Status", col.Status),
		output.FAlways("Type", col.Type),
		output.FAlways("Products", col.ProductCount),
		output.F("Created", created),
		output.F("Updated", updated),
	})
}
