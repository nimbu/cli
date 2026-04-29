package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/migrate"
	"github.com/nimbu/cli/internal/output"
)

// ChannelsFieldsCmd manages channel fields.
type ChannelsFieldsCmd struct {
	List    ChannelsFieldsListCmd    `cmd:"" help:"List channel fields"`
	Add     ChannelsFieldsAddCmd     `cmd:"" help:"Add a channel field"`
	Update  ChannelsFieldsUpdateCmd  `cmd:"" help:"Update a channel field"`
	Delete  ChannelsFieldsDeleteCmd  `cmd:"" help:"Delete a channel field"`
	Apply   ChannelsFieldsApplyCmd   `cmd:"" help:"Apply channel fields from JSON"`
	Replace ChannelsFieldsReplaceCmd `cmd:"" help:"Replace channel fields from JSON"`
	Diff    ChannelsFieldsDiffCmd    `cmd:"" help:"Diff channel fields against JSON"`
}

// ChannelsFieldsListCmd lists channel fields.
type ChannelsFieldsListCmd struct {
	Channel string `required:"" help:"Channel slug"`
}

// ChannelsFieldsAddCmd adds a channel field.
type ChannelsFieldsAddCmd struct {
	Channel     string   `required:"" help:"Channel slug"`
	Name        string   `required:"" help:"Field name"`
	Assignments []string `arg:"" optional:"" help:"Field attributes (e.g. type=string label=Title)"`
}

// ChannelsFieldsUpdateCmd updates a channel field.
type ChannelsFieldsUpdateCmd struct {
	Channel     string   `required:"" help:"Channel slug"`
	Field       string   `required:"" help:"Field ID or name"`
	Assignments []string `arg:"" optional:"" help:"Field attributes to update (e.g. label=Headline required:=true)"`
}

// ChannelsFieldsDeleteCmd deletes a channel field.
type ChannelsFieldsDeleteCmd struct {
	Channel string `required:"" help:"Channel slug"`
	Field   string `required:"" help:"Field ID or name"`
}

// ChannelsFieldsApplyCmd applies channel fields.
type ChannelsFieldsApplyCmd struct {
	Channel string `required:"" help:"Channel slug"`
	File    string `required:"" help:"Read field JSON array from file (use - for stdin)"`
}

// ChannelsFieldsReplaceCmd replaces channel fields.
type ChannelsFieldsReplaceCmd struct {
	Channel string `required:"" help:"Channel slug"`
	File    string `required:"" help:"Read field JSON array from file (use - for stdin)"`
}

// ChannelsFieldsDiffCmd diffs channel fields.
type ChannelsFieldsDiffCmd struct {
	Channel string `required:"" help:"Channel slug"`
	File    string `required:"" help:"Read field JSON array from file (use - for stdin)"`
}

// Run executes the list fields command.
func (c *ChannelsFieldsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	fields, err := api.GetChannelCustomizations(ctx, client, c.Channel)
	if err != nil {
		return fmt.Errorf("list channel fields: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, fields)
	}

	if mode.Plain {
		return writeSchemaFieldPlain(ctx, c.Channel, fields)
	}

	return writeSchemaFieldHuman(ctx, customFieldSchemaView{
		Key:    c.Channel,
		Fields: fields,
	})
}

func (c *ChannelsFieldsAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "add channel field"); err != nil {
		return err
	}
	client, err := channelFieldsClient(ctx)
	if err != nil {
		return err
	}
	field, err := fieldMapFromAssignments(c.Assignments)
	if err != nil {
		return err
	}
	if _, exists := field["name"]; exists {
		return fmt.Errorf("field identity must use --name, not trailing name=<value>")
	}
	name := strings.TrimSpace(c.Name)
	fieldType, _ := field["type"].(string)
	if name == "" || strings.TrimSpace(fieldType) == "" {
		return fmt.Errorf("fields add requires --name and type=<type>")
	}
	field["name"] = name
	existing, err := api.GetChannelCustomizations(ctx, client, c.Channel)
	if err != nil {
		return fmt.Errorf("list channel fields: %w", err)
	}
	if _, ok := findCustomField(existing, name); ok {
		return fmt.Errorf("field %q already exists on channel %s", name, c.Channel)
	}
	return patchAndPrintChannelFields(ctx, client, c.Channel, []map[string]any{field}, false)
}

func (c *ChannelsFieldsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "update channel field"); err != nil {
		return err
	}
	client, err := channelFieldsClient(ctx)
	if err != nil {
		return err
	}
	existing, err := api.GetChannelCustomizations(ctx, client, c.Channel)
	if err != nil {
		return fmt.Errorf("list channel fields: %w", err)
	}
	current, ok := findCustomField(existing, c.Field)
	if !ok {
		return fmt.Errorf("field %q not found on channel %s", c.Field, c.Channel)
	}
	field, err := fieldMapFromAssignments(c.Assignments)
	if err != nil {
		return err
	}
	if len(field) == 0 {
		return fmt.Errorf("fields update requires at least one assignment")
	}
	field["id"] = current.ID
	return patchAndPrintChannelFields(ctx, client, c.Channel, []map[string]any{field}, false)
}

func (c *ChannelsFieldsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "delete channel field"); err != nil {
		return err
	}
	if err := requireForce(flags, fmt.Sprintf("channel field %s", c.Field)); err != nil {
		return err
	}
	client, err := channelFieldsClient(ctx)
	if err != nil {
		return err
	}
	existing, err := api.GetChannelCustomizations(ctx, client, c.Channel)
	if err != nil {
		return fmt.Errorf("list channel fields: %w", err)
	}
	current, ok := findCustomField(existing, c.Field)
	if !ok {
		return fmt.Errorf("field %q not found on channel %s", c.Field, c.Channel)
	}
	if current.ID == "" {
		return fmt.Errorf("field %q has no API id and cannot be deleted safely", c.Field)
	}
	field := map[string]any{"id": current.ID, "_destroy": true}
	return patchAndPrintChannelFields(ctx, client, c.Channel, []map[string]any{field}, false)
}

func (c *ChannelsFieldsApplyCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "apply channel fields"); err != nil {
		return err
	}
	client, err := channelFieldsClient(ctx)
	if err != nil {
		return err
	}
	fields, err := readCustomFieldsArray(c.File)
	if err != nil {
		return err
	}
	return patchAndPrintChannelFields(ctx, client, c.Channel, fields, false)
}

func (c *ChannelsFieldsReplaceCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "replace channel fields"); err != nil {
		return err
	}
	if err := requireForce(flags, fmt.Sprintf("all fields on channel %s", c.Channel)); err != nil {
		return err
	}
	client, err := channelFieldsClient(ctx)
	if err != nil {
		return err
	}
	fields, err := readCustomFieldsArray(c.File)
	if err != nil {
		return err
	}
	return patchAndPrintChannelFields(ctx, client, c.Channel, fields, true)
}

func (c *ChannelsFieldsDiffCmd) Run(ctx context.Context, flags *RootFlags) error {
	client, err := channelFieldsClient(ctx)
	if err != nil {
		return err
	}
	current, err := api.GetChannelCustomizations(ctx, client, c.Channel)
	if err != nil {
		return fmt.Errorf("list channel fields: %w", err)
	}
	target, err := readCustomFieldsArray(c.File)
	if err != nil {
		return err
	}
	diff := migrate.DiffNormalized(migrate.NormalizeCustomizations(current), normalizeFieldMapsForDiff(target))
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, diff)
	}
	plainLines := append(renderDiffChanges("add", diff.Added), renderDiffChanges("remove", diff.Removed)...)
	plainLines = append(plainLines, renderDiffChanges("update", diff.Updated)...)
	humanLines := append([]string{}, renderDiffChanges("+", diff.Added)...)
	humanLines = append(humanLines, renderDiffChanges("-", diff.Removed)...)
	humanLines = append(humanLines, renderDiffChanges("~", diff.Updated)...)
	if len(plainLines) == 0 {
		plainLines = []string{"equal\t$"}
		humanLines = []string{"There are no differences."}
	}
	return writeDiffSet(ctx, diff, plainLines, humanLines)
}

func channelFieldsClient(ctx context.Context) (*api.Client, error) {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return nil, err
	}
	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func fieldMapFromAssignments(assignments []string) (map[string]any, error) {
	if len(assignments) == 0 {
		return map[string]any{}, nil
	}
	return parseInlineAssignments(assignments)
}

func patchAndPrintChannelFields(ctx context.Context, client *api.Client, channel string, fields []map[string]any, replace bool) error {
	detail, err := api.PatchChannelCustomizations(ctx, client, channel, fields, replace)
	if err != nil {
		return fmt.Errorf("update channel fields: %w", err)
	}
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, detail.Customizations)
	}
	if mode.Plain {
		return writeSchemaFieldPlain(ctx, channel, detail.Customizations)
	}
	_, err = output.Fprintf(ctx, "Updated fields for channel %s\n", channel)
	return err
}

func findCustomField(fields []api.CustomField, ref string) (api.CustomField, bool) {
	for _, field := range fields {
		if field.ID == ref || field.Name == ref {
			return field, true
		}
	}
	return api.CustomField{}, false
}

func readCustomFieldsArray(file string) ([]map[string]any, error) {
	var input io.Reader
	switch file {
	case "":
		return nil, fmt.Errorf("%w; use --file <path> or --file - with piped stdin", errNoJSONInput)
	case "-":
		input = os.Stdin
	default:
		f, err := os.Open(file)
		if err != nil {
			return nil, fmt.Errorf("open file: %w", err)
		}
		defer func() { _ = f.Close() }()
		input = f
	}
	data, err := io.ReadAll(io.LimitReader(input, maxJSONInputBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read input: %w", err)
	}
	if int64(len(data)) > maxJSONInputBytes {
		return nil, fmt.Errorf("input exceeds %d bytes", maxJSONInputBytes)
	}
	var fields []map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, fmt.Errorf("parse field JSON array: %w", err)
	}
	if fields == nil {
		return nil, fmt.Errorf("field JSON must be an array")
	}
	return fields, nil
}

func normalizeFieldMapsForDiff(fields []map[string]any) []map[string]any {
	normalized := make([]map[string]any, 0, len(fields))
	for _, field := range fields {
		item := cloneStringAnyMap(field)
		delete(item, "id")
		if options, ok := item["select_options"].([]any); ok {
			for _, option := range options {
				if optionMap, ok := option.(map[string]any); ok {
					delete(optionMap, "id")
				}
			}
		}
		normalized = append(normalized, item)
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		return fmt.Sprint(normalized[i]["name"]) < fmt.Sprint(normalized[j]["name"])
	})
	return normalized
}

func cloneStringAnyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
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
