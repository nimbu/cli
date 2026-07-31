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
	Collection  string   `required:"" help:"Collection ID or slug"`
	Locale      string   `help:"Content locale for localized collection fields"`
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

	var document api.Document[api.Collection]
	path := "/collections/" + url.PathEscape(c.Collection)
	var opts []api.RequestOption
	if c.Locale != "" {
		opts = append(opts, api.WithContentLocale(c.Locale))
	}
	if err := client.Put(ctx, path, body, &document, opts...); err != nil {
		return fmt.Errorf("update collection: %w", err)
	}
	col := document.Value
	display, err := localizedDocumentMap(document, c.Locale)
	if err != nil {
		return fmt.Errorf("project collection locale: %w", err)
	}

	return output.Print(ctx, document, []any{display["id"], display["slug"], display["name"]}, func() error {
		_, err := output.Fprintf(ctx, "Updated collection: %v (%s)\n", display["name"], col.ID)
		return err
	})
}
