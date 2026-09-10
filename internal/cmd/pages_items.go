package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nimbu/cli/internal/output"
)

// PagesItemsCmd gets a resolved items subtree.
type PagesItemsCmd struct {
	Page string `required:"" help:"Page fullpath or id"`
	Path string `required:"" help:"Human or raw path under /items"`
}

// Run executes pages items.
func (c *PagesItemsCmd) Run(ctx context.Context) error {
	session, err := openSurgicalPage(ctx, nil, c.Page, "")
	if err != nil {
		return err
	}
	resolved, err := session.resolve(c.Path)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(resolved.RawPath, "/items/") {
		return fmt.Errorf("path %q is a page field; use pages get", c.Path)
	}

	subtree, err := session.client.GetPageItems(ctx, session.pageID, resolved.RawPath)
	if err != nil {
		return fmt.Errorf("get page items: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, subtree)
	}
	if _, err := output.Fprintf(ctx, "Path:      %s\n", subtree.Path); err != nil {
		return err
	}
	if _, err := output.Fprintf(ctx, "Type:      %s\n", subtree.Type); err != nil {
		return err
	}
	if _, err := output.Fprintf(ctx, "Position:  %d\n", subtree.Position); err != nil {
		return err
	}
	if _, err := output.Fprintf(ctx, "Siblings:  %d\n", subtree.SiblingsCount); err != nil {
		return err
	}
	if len(subtree.Data) == 0 {
		return nil
	}
	var data any
	if err := json.Unmarshal(subtree.Data, &data); err != nil {
		_, err := output.Fprintln(ctx, string(subtree.Data))
		return err
	}
	if _, err := output.Fprintln(ctx); err != nil {
		return err
	}
	_, err = output.Fprintln(ctx, prettyJSON(data))
	return err
}
