package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nimbu/cli/internal/output"
)

// PagesItemsCmd gets a resolved items subtree.
type PagesItemsCmd struct {
	Page   string `required:"" help:"Page fullpath or id"`
	Path   string `required:"" help:"Human or raw path under /items"`
	Locale string `help:"Content locale; overlays translations for display"`
}

// Run executes pages items.
func (c *PagesItemsCmd) Run(ctx context.Context) error {
	session, err := openSurgicalPage(ctx, nil, c.Page, c.Locale)
	if err != nil {
		return err
	}
	resolved, err := session.resolveUserPath(c.Path)
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
		if c.Locale == "" {
			return output.JSON(ctx, subtree)
		}
		projected, err := output.ProjectLocale(subtree, c.Locale)
		if err != nil {
			return fmt.Errorf("project items locale: %w", err)
		}
		return output.JSON(ctx, projected)
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
	data, err := decodePageItemsData(subtree.Data)
	if err != nil {
		_, err := output.Fprintln(ctx, string(subtree.Data))
		return err
	}
	if c.Locale != "" {
		projected, err := output.ProjectLocale(data, c.Locale)
		if err != nil {
			return fmt.Errorf("project items locale: %w", err)
		}
		data = projected
	}
	if _, err := output.Fprintln(ctx); err != nil {
		return err
	}
	_, err = output.Fprintln(ctx, rawJSONBlock(data))
	return err
}

func decodePageItemsData(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var data any
	if err := decoder.Decode(&data); err != nil {
		return nil, err
	}
	return data, nil
}

// rawJSONBlock renders v as indented JSON without HTML escaping, so that
// content editables stay readable: HTML tags and ampersands stay as typed.
func rawJSONBlock(v any) string {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(v); err != nil {
		return fmt.Sprint(v)
	}
	return strings.TrimRight(buf.String(), "\n")
}
