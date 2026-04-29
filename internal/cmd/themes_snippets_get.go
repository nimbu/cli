package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ThemeSnippetsGetCmd gets a snippet.
type ThemeSnippetsGetCmd struct {
	Theme string `required:"" help:"Theme ID"`
	Name  string `required:"" help:"Snippet name including extension"`
}

// Run executes the get command.
func (c *ThemeSnippetsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/themes/%s/snippets/%s", url.PathEscape(c.Theme), url.PathEscape(c.Name))
	var snippet api.ThemeResource
	if err := client.Get(ctx, path, &snippet); err != nil {
		return fmt.Errorf("get snippet: %w", err)
	}

	if snippet.Code != "" {
		return output.Print(ctx, snippet, []any{snippet.Code}, func() error {
			_, err := output.Fprintf(ctx, "%s\n", snippet.Code)
			return err
		})
	}

	var created, updated string
	if !snippet.CreatedAt.IsZero() {
		created = snippet.CreatedAt.Format("2006-01-02 15:04:05")
	}
	if !snippet.UpdatedAt.IsZero() {
		updated = snippet.UpdatedAt.Format("2006-01-02 15:04:05")
	}

	return output.Detail(ctx, snippet, []any{snippet.ID, snippet.Name, snippet.UpdatedAt}, []output.Field{
		output.FAlways("ID", snippet.ID),
		output.FAlways("Name", snippet.Name),
		output.FAlways("URL", snippet.URL),
		output.FAlways("Permalink", snippet.Permalink),
		output.F("Created", created),
		output.F("Updated", updated),
	})
}
