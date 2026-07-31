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
	Collection string `required:"" help:"Collection ID or slug"`
	Locale     string `help:"Content locale for localized collection fields"`
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

	var document api.Document[api.Collection]
	path := "/collections/" + url.PathEscape(c.Collection)
	var opts []api.RequestOption
	if c.Locale != "" {
		opts = append(opts, api.WithContentLocale(c.Locale))
	}
	if err := client.Get(ctx, path, &document, opts...); err != nil {
		return fmt.Errorf("get collection: %w", err)
	}
	col := document.Value
	display, err := localizedDocumentMap(document, c.Locale)
	if err != nil {
		return fmt.Errorf("project collection locale: %w", err)
	}

	var created, updated string
	if col.CreatedAt != nil && !col.CreatedAt.IsZero() {
		created = col.CreatedAt.Format("2006-01-02 15:04:05")
	}
	if col.UpdatedAt != nil && !col.UpdatedAt.IsZero() {
		updated = col.UpdatedAt.Format("2006-01-02 15:04:05")
	}

	return output.Detail(ctx, document, []any{display["id"], display["slug"], display["name"], col.Status, col.Type}, []output.Field{
		output.FAlways("ID", display["id"]),
		output.FAlways("Slug", display["slug"]),
		output.FAlways("Name", display["name"]),
		output.F("Description", display["description"]),
		output.FAlways("Status", col.Status),
		output.FAlways("Type", col.Type),
		output.FAlways("Products", col.ProductCount),
		output.F("Created", created),
		output.F("Updated", updated),
	})
}
