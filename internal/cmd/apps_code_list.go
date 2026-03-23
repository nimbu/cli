package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// AppsCodeListCmd lists app code files.
type AppsCodeListCmd struct {
	QueryFlags `embed:""`
	App        string `arg:"" help:"Application ID"`
}

// Run executes the list command.
func (c *AppsCodeListCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	path := "/apps/" + url.PathEscape(c.App) + "/code"
	files, err := api.List[api.AppCodeFile](ctx, client, path)
	if err != nil {
		return fmt.Errorf("list app code files: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, files)
	}

	plainFields := []string{"name", "url"}
	tableFields := []string{"name", "url", "updated_at"}
	tableHeaders := []string{"NAME", "URL", "UPDATED"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, files, listOutputFields(&c.QueryFlags, plainFields))
	}

	fields, headers := listOutputColumns(&c.QueryFlags, tableFields, tableHeaders)
	return output.WriteTable(ctx, files, fields, headers)
}
