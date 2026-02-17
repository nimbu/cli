package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// PagesUpdateCmd updates a page.
type PagesUpdateCmd struct {
	Page        string   `arg:"" help:"Page ID or slug"`
	File        string   `help:"Read page JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. title=About, published:=true)"`
}

// Run executes the update command.
func (c *PagesUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "update page"); err != nil {
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

	var page api.Page
	path := "/pages/" + url.PathEscape(c.Page)
	if err := client.Put(ctx, path, body, &page); err != nil {
		return fmt.Errorf("update page: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, page)
	}

	if mode.Plain {
		return output.Plain(ctx, page.ID)
	}

	fmt.Printf("Updated page %s\n", page.ID)
	return nil
}
