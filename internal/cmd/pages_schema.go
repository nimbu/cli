package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/output"
	"github.com/nimbu/cli/internal/pagepath"
)

// PagesSchemaCmd shows the page template schema.
type PagesSchemaCmd struct {
	Page string `required:"" help:"Page fullpath or id"`
}

// Run executes pages schema.
func (c *PagesSchemaCmd) Run(ctx context.Context) error {
	session, err := openSurgicalPage(ctx, nil, c.Page, "")
	if err != nil {
		return err
	}
	schema, err := session.ensureSchema()
	if err != nil {
		return err
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, schema)
	}
	return printPageSchema(ctx, schema)
}

func printPageSchema(ctx context.Context, schema *pagepath.Schema) error {
	if schema.Template.Name != "" {
		if _, err := output.Fprintf(ctx, "Template: %s\n", schema.Template.Name); err != nil {
			return err
		}
	}
	names := make([]string, 0, len(schema.AvailableBlocks))
	for name := range schema.AvailableBlocks {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, canvas := range names {
		if _, err := output.Fprintf(ctx, "\n%s\n", canvas); err != nil {
			return err
		}
		for _, block := range schema.Blocks(canvas) {
			label := block.Slug
			if block.Label != "" {
				label = block.Slug + " (" + block.Label + ")"
			}
			if _, err := output.Fprintf(ctx, "  %s\n", label); err != nil {
				return err
			}
			for _, field := range block.Fields {
				if _, err := output.Fprintf(ctx, "    %s\n", formatSchemaField(field, canvas, block.Slug, schema)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func formatSchemaField(field pagepath.FieldDef, canvas, slug string, schema *pagepath.Schema) string {
	typ := field.Type
	opts := schema.OptionsFor(canvas, slug, field.Slug)
	if len(opts) == 0 && len(field.Options) > 0 {
		opts = field.Options
	}
	if len(opts) > 0 {
		labels := make([]string, 0, len(opts))
		for _, opt := range opts {
			label := opt.Label
			if label == "" {
				label = opt.Value
			}
			labels = append(labels, label)
		}
		typ = typ + ": " + strings.Join(labels, "|")
	}
	return fmt.Sprintf("%s (%s)", field.Slug, typ)
}
