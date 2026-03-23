package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// PagesUpdateCmd updates a page.
type PagesUpdateCmd struct {
	QueryFlags  `embed:""`
	Page        string   `arg:"" help:"Page fullpath"`
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

	var opts []api.RequestOption
	if c.Locale != "" {
		opts = append(opts, api.WithLocale(c.Locale))
	}

	var body api.PageDocument
	if len(c.Assignments) > 0 {
		if err := validateShallowInlineAssignments("pages update", c.Assignments, map[string]struct{}{
			"title":     {},
			"template":  {},
			"published": {},
			"locale":    {},
		}); err != nil {
			return err
		}

		body, err = api.GetPageDocument(ctx, client, c.Page, opts...)
		if err != nil {
			return fmt.Errorf("fetch current page: %w", err)
		}

		updates, err := readJSONBodyInput("", c.Assignments)
		if err != nil {
			return err
		}
		mergeTopLevel(body, updates)
	} else {
		rawBody, err := readRichDocumentInput(c.File)
		if err != nil {
			return err
		}
		body = api.PageDocument(rawBody)
	}

	if err := api.ExpandPageAttachmentPaths(body); err != nil {
		return err
	}

	page, err := api.PatchPageDocument(ctx, client, c.Page, body, opts...)
	if err != nil {
		return fmt.Errorf("update page: %w", err)
	}

	return output.Print(ctx, page, []any{page["id"]}, func() error {
		_, err := output.Fprintf(ctx, "Updated page %v\n", page["id"])
		return err
	})
}
