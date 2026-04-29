package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ThemeLayoutsGetCmd gets a layout.
type ThemeLayoutsGetCmd struct {
	Theme string `required:"" help:"Theme ID"`
	Name  string `required:"" help:"Layout name including extension"`
}

// Run executes the get command.
func (c *ThemeLayoutsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/themes/%s/layouts/%s", url.PathEscape(c.Theme), url.PathEscape(c.Name))
	var layout api.ThemeResource
	if err := client.Get(ctx, path, &layout); err != nil {
		return fmt.Errorf("get layout: %w", err)
	}

	if layout.Code != "" {
		return output.Print(ctx, layout, []any{layout.Code}, func() error {
			_, err := output.Fprintf(ctx, "%s\n", layout.Code)
			return err
		})
	}

	var created, updated string
	if !layout.CreatedAt.IsZero() {
		created = layout.CreatedAt.Format("2006-01-02 15:04:05")
	}
	if !layout.UpdatedAt.IsZero() {
		updated = layout.UpdatedAt.Format("2006-01-02 15:04:05")
	}

	return output.Detail(ctx, layout, []any{layout.ID, layout.Name, layout.UpdatedAt}, []output.Field{
		output.FAlways("ID", layout.ID),
		output.FAlways("Name", layout.Name),
		output.FAlways("URL", layout.URL),
		output.FAlways("Permalink", layout.Permalink),
		output.F("Created", created),
		output.F("Updated", updated),
	})
}
