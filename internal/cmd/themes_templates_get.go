package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ThemeTemplatesGetCmd gets a template.
type ThemeTemplatesGetCmd struct {
	Theme string `required:"" help:"Theme ID"`
	Name  string `required:"" help:"Template name including extension"`
}

// Run executes the get command.
func (c *ThemeTemplatesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/themes/%s/templates/%s", url.PathEscape(c.Theme), url.PathEscape(c.Name))
	var tmpl api.ThemeResource
	if err := client.Get(ctx, path, &tmpl); err != nil {
		return fmt.Errorf("get template: %w", err)
	}

	if tmpl.Code != "" {
		return output.Print(ctx, tmpl, []any{tmpl.Code}, func() error {
			_, err := output.Fprintf(ctx, "%s\n", tmpl.Code)
			return err
		})
	}

	var created, updated string
	if !tmpl.CreatedAt.IsZero() {
		created = tmpl.CreatedAt.Format("2006-01-02 15:04:05")
	}
	if !tmpl.UpdatedAt.IsZero() {
		updated = tmpl.UpdatedAt.Format("2006-01-02 15:04:05")
	}

	return output.Detail(ctx, tmpl, []any{tmpl.ID, tmpl.Name, tmpl.UpdatedAt}, []output.Field{
		output.FAlways("ID", tmpl.ID),
		output.FAlways("Name", tmpl.Name),
		output.FAlways("URL", tmpl.URL),
		output.FAlways("Permalink", tmpl.Permalink),
		output.F("Created", created),
		output.F("Updated", updated),
	})
}
