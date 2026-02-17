package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ThemeFilesListCmd lists theme files.
type ThemeFilesListCmd struct {
	Theme   string `arg:"" help:"Theme ID"`
	All     bool   `help:"Fetch all pages"`
	Page    int    `help:"Page number" default:"1"`
	PerPage int    `help:"Items per page" default:"25"`
}

// Run executes the list command.
func (c *ThemeFilesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/themes/%s/files", url.PathEscape(c.Theme))
	opts, err := listRequestOptions(flags)
	if err != nil {
		return fmt.Errorf("list theme files: %w", err)
	}

	var files []api.ThemeFile

	if c.All {
		files, err = api.List[api.ThemeFile](ctx, client, path, opts...)
		if err != nil {
			return fmt.Errorf("list theme files: %w", err)
		}
	} else {
		paged, err := api.ListPage[api.ThemeFile](ctx, client, path, c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list theme files: %w", err)
		}
		files = paged.Data
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, files)
	}

	plainFields := []string{"path", "type", "size"}
	tableFields := []string{"path", "type", "size"}
	tableHeaders := []string{"PATH", "TYPE", "SIZE"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, files, listOutputFields(flags, plainFields))
	}

	fields, headers := listOutputColumns(flags, tableFields, tableHeaders)
	return output.WriteTable(ctx, files, fields, headers)
}
