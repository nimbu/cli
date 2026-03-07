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
	Menu        string   `arg:"" help:"Menu slug or handle"`
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

	var body api.MenuDocument
	if len(c.Assignments) > 0 {
		if err := validateShallowInlineAssignments("menus update", c.Assignments, map[string]struct{}{
			"name":   {},
			"handle": {},
		}); err != nil {
			return err
		}

		body, err = api.GetMenuDocument(ctx, client, c.Menu)
		if err != nil {
			return fmt.Errorf("fetch current menu: %w", err)
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
		body = api.MenuDocument(rawBody)
	}
	api.NormalizeMenuDocumentForWrite(body)

	slug := api.MenuDocumentSlug(body)
	if slug == "" {
		slug = strings.TrimSpace(c.Menu)
	}
	menu, err := api.PatchMenuDocument(ctx, client, slug, body)
	if err != nil {
		return fmt.Errorf("update menu: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, menu)
	}

	if mode.Plain {
		return output.Plain(ctx, menu["id"])
	}

	if err := printLine(ctx, "Updated menu %v\n", menu["id"]); err != nil {
		return err
	}
	return nil
}
