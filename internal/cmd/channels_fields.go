package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ChannelsFieldsCmd lists channel fields.
type ChannelsFieldsCmd struct {
	Channel string `arg:"" help:"Channel slug"`
}

// Run executes the list fields command.
func (c *ChannelsFieldsCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	detail, err := api.GetChannelDetail(ctx, client, c.Channel)
	if err != nil {
		return fmt.Errorf("get channel detail: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, detail.Customizations)
	}

	if mode.Plain {
		return writeSchemaFieldPlain(ctx, detail.Slug, detail.Customizations)
	}

	return writeSchemaFieldHuman(ctx, customFieldSchemaView{
		Key:        detail.Slug,
		Name:       detail.Name,
		TitleField: detail.TitleField,
		LabelField: detail.LabelField,
		Fields:     detail.Customizations,
	})
}

func writeSchemaFieldPlain(ctx context.Context, owner string, fields []api.CustomField) error {
	rows := make([][]any, 0, len(fields))
	for _, field := range fields {
		options := field.SelectOptions
		if options == nil {
			options = []api.SelectOption{}
		}
		optionsJSON, err := json.Marshal(options)
		if err != nil {
			return fmt.Errorf("encode select options: %w", err)
		}
		rows = append(rows, []any{
			owner,
			field.Name,
			field.Label,
			field.Type,
			channelFieldFlags(field),
			field.Reference,
			string(optionsJSON),
			field.Hint,
			field.RequiredExpression,
			field.CalculatedExpression,
			field.CalculationType,
			field.GeoType,
		})
	}
	return output.PlainRows(ctx, rows)
}

func writeSchemaFieldHuman(ctx context.Context, schema customFieldSchemaView) error {
	return newChannelFieldPresenter(ctx).Render(schema)
}

func channelFieldFlags(field api.CustomField) string {
	return strings.Join(channelFieldFlagList(field), ", ")
}

func channelFieldFlagList(field api.CustomField) []string {
	flags := make([]string, 0, 5)
	if field.Required {
		flags = append(flags, "required")
	}
	if field.Unique {
		flags = append(flags, "unique")
	}
	if field.Localized {
		flags = append(flags, "localized")
	}
	if field.Encrypted {
		flags = append(flags, "encrypted")
	}
	if field.PrivateStorage {
		flags = append(flags, "private_storage")
	}
	return flags
}

func channelFieldOptionLabel(option api.SelectOption) string {
	switch {
	case strings.TrimSpace(option.Name) != "":
		return option.Name
	case strings.TrimSpace(option.Slug) != "":
		return option.Slug
	case strings.TrimSpace(option.ID) != "":
		return option.ID
	default:
		return "<unnamed>"
	}
}
