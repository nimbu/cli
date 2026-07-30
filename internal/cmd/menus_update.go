package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// MenusUpdateCmd updates a menu.
type MenusUpdateCmd struct {
	Menu        string   `required:"" help:"Menu slug or handle"`
	File        string   `help:"Read menu JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. name=Main, handle=main)"`
}

// Run executes the update command.
func (c *MenusUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "update menu"); err != nil {
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

	slug := strings.TrimSpace(c.Menu)
	var body api.MenuDocument
	var submitted api.MenuDocumentStats
	fullDocument := false

	if len(c.Assignments) > 0 {
		if c.File != "" {
			return fmt.Errorf("use either --file or inline assignments, not both")
		}
		if err := validateShallowInlineAssignments("menus update", c.Assignments, map[string]struct{}{
			"name":   {},
			"handle": {},
		}); err != nil {
			return err
		}

		updates, err := readJSONBodyInput("", c.Assignments)
		if err != nil {
			return err
		}
		// Shallow metadata only — do not fetch/resend items or force replace.
		body = api.MenuDocument(updates)
	} else {
		rawBody, err := readRichDocumentInput(c.File)
		if err != nil {
			return err
		}
		body = api.MenuDocument(rawBody)
		submitted = api.MenuStats(body)
		api.NormalizeMenuDocumentForWrite(body)
		fullDocument = true
		if bodySlug := api.MenuDocumentSlug(body); bodySlug != "" {
			slug = bodySlug
		}
		current, err := api.GetMenuDocument(ctx, client, slug)
		if err != nil {
			return fmt.Errorf("read current menu before reconciliation: %w", err)
		}
		api.ReconcileMenuDocument(current, body)
	}

	menu, err := api.PatchMenuDocument(ctx, client, slug, body)
	if err != nil {
		return fmt.Errorf("update menu: %w", err)
	}

	if fullDocument {
		if err := verifyMenuNesting(ctx, client, submitted, menu); err != nil {
			return err
		}
	}

	return output.Print(ctx, menu, []any{menu["id"]}, func() error {
		_, err := output.Fprintf(ctx, "Updated menu %v\n", menu["id"])
		return err
	})
}
