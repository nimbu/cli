package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

type channelFieldSection string

const (
	channelFieldSectionCore      channelFieldSection = "Core"
	channelFieldSectionChoices   channelFieldSection = "Choices"
	channelFieldSectionRelations channelFieldSection = "Relations"
	channelFieldSectionAdvanced  channelFieldSection = "Advanced"
)

type channelFieldPresenter struct {
	out   io.Writer
	theme output.Theme
}

type customFieldSchemaView struct {
	Key        string
	Name       string
	TitleField string
	LabelField string
	Fields     []api.CustomField
}

type channelFieldRow struct {
	key    string
	label  string
	kind   string
	target string
	flags  []string
}

func newChannelFieldPresenter(ctx context.Context) *channelFieldPresenter {
	writer := output.WriterFromContext(ctx)
	out := io.Writer(io.Discard)
	if writer.Out != nil {
		out = writer.Out
	}
	// The field table is content on stdout, so it follows stdout's color
	// decision and profile.
	return &channelFieldPresenter{out: out, theme: writer.OutTheme()}
}

func (p *channelFieldPresenter) Render(schema customFieldSchemaView) error {
	if _, err := fmt.Fprintln(p.out, p.headerLine(schema)); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(p.out, p.metaLine(schema)); err != nil {
		return err
	}

	sections := p.groupSections(schema.Fields)
	if len(sections) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(p.out); err != nil {
		return err
	}

	for index, section := range sections {
		if index > 0 {
			if _, err := fmt.Fprintln(p.out); err != nil {
				return err
			}
		}
		if err := p.renderSection(section); err != nil {
			return err
		}
	}
	return nil
}

func (p *channelFieldPresenter) headerLine(schema customFieldSchemaView) string {
	parts := []string{p.fieldKey(schema.Key)}
	if strings.TrimSpace(schema.Name) != "" {
		parts = append(parts, p.value(schema.Name))
	}
	return strings.Join(parts, "  ")
}

func (p *channelFieldPresenter) metaLine(schema customFieldSchemaView) string {
	segments := []string{}
	if schema.TitleField != "" {
		segments = append(segments, p.dim("title: "), p.value(schema.TitleField))
	}
	if schema.LabelField != "" {
		if len(segments) > 0 {
			segments = append(segments, p.dim("   "))
		}
		segments = append(segments, p.dim("label: "), p.value(schema.LabelField))
	}
	if len(segments) > 0 {
		segments = append(segments, p.dim("   "))
	}
	segments = append(segments, p.dim("fields: "), p.value(fmt.Sprintf("%d", len(schema.Fields))))
	return strings.Join(segments, "")
}

func (p *channelFieldPresenter) groupSections(fields []api.CustomField) []struct {
	name   channelFieldSection
	fields []api.CustomField
} {
	grouped := []struct {
		name   channelFieldSection
		fields []api.CustomField
	}{
		{name: channelFieldSectionCore},
		{name: channelFieldSectionChoices},
		{name: channelFieldSectionRelations},
		{name: channelFieldSectionAdvanced},
	}

	for _, field := range fields {
		switch p.classifyField(field) {
		case channelFieldSectionChoices:
			grouped[1].fields = append(grouped[1].fields, field)
		case channelFieldSectionRelations:
			grouped[2].fields = append(grouped[2].fields, field)
		case channelFieldSectionAdvanced:
			grouped[3].fields = append(grouped[3].fields, field)
		default:
			grouped[0].fields = append(grouped[0].fields, field)
		}
	}

	out := make([]struct {
		name   channelFieldSection
		fields []api.CustomField
	}, 0, len(grouped))
	for _, section := range grouped {
		if len(section.fields) == 0 {
			continue
		}
		out = append(out, section)
	}
	return out
}

func (p *channelFieldPresenter) classifyField(field api.CustomField) channelFieldSection {
	switch field.Type {
	case "select", "multi_select":
		return channelFieldSectionChoices
	case "belongs_to", "belongs_to_many":
		return channelFieldSectionRelations
	}
	if field.Hint != "" ||
		field.RequiredExpression != "" ||
		field.CalculatedExpression != "" ||
		field.CalculationType != "" ||
		field.GeoType != "" ||
		field.Encrypted ||
		field.PrivateStorage ||
		len(channelFieldInterestingExtra(field)) > 0 {
		return channelFieldSectionAdvanced
	}
	return channelFieldSectionCore
}

func (p *channelFieldPresenter) renderSection(section struct {
	name   channelFieldSection
	fields []api.CustomField
},
) error {
	if _, err := fmt.Fprintln(p.out, p.sectionHeader(string(section.name))); err != nil {
		return err
	}

	switch section.name {
	case channelFieldSectionChoices:
		return p.renderChoiceFields(section.fields)
	case channelFieldSectionRelations:
		return p.renderRelationFields(section.fields)
	case channelFieldSectionAdvanced:
		return p.renderAdvancedFields(section.fields)
	default:
		return p.renderCoreFields(section.fields)
	}
}

func (p *channelFieldPresenter) renderCoreFields(fields []api.CustomField) error {
	rows := make([]channelFieldRow, 0, len(fields))
	for _, field := range fields {
		rows = append(rows, channelFieldRow{
			key:   field.Name,
			label: channelFieldDisplayLabel(field),
			kind:  field.Type,
			flags: channelFieldFlagList(field),
		})
	}
	return p.renderAlignedRows(rows)
}

func (p *channelFieldPresenter) renderRelationFields(fields []api.CustomField) error {
	rows := make([]channelFieldRow, 0, len(fields))
	for _, field := range fields {
		rows = append(rows, channelFieldRow{
			key:    field.Name,
			label:  channelFieldDisplayLabel(field),
			kind:   field.Type,
			target: "-> " + field.Reference,
			flags:  channelFieldFlagList(field),
		})
	}
	return p.renderAlignedRowsWithDetails(rows, fields)
}

func (p *channelFieldPresenter) renderChoiceFields(fields []api.CustomField) error {
	rows := make([]channelFieldRow, 0, len(fields))
	for _, field := range fields {
		rows = append(rows, channelFieldRow{
			key:   field.Name,
			label: channelFieldDisplayLabel(field),
			kind:  field.Type,
			flags: channelFieldFlagList(field),
		})
	}
	return p.renderAlignedRowsWithDetails(rows, fields)
}

func (p *channelFieldPresenter) renderAdvancedFields(fields []api.CustomField) error {
	for index, field := range fields {
		row := channelFieldRow{
			key:   field.Name,
			label: channelFieldDisplayLabel(field),
			kind:  field.Type,
			flags: channelFieldFlagList(field),
		}
		if _, err := fmt.Fprintln(p.out, p.formatRow(row, ansi.StringWidth(row.key), ansi.StringWidth(row.label), ansi.StringWidth(row.kind), 0)); err != nil {
			return err
		}
		if err := p.renderExtraDetails(field, "    "); err != nil {
			return err
		}
		if index < len(fields)-1 {
			if _, err := fmt.Fprintln(p.out); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *channelFieldPresenter) renderAlignedRows(rows []channelFieldRow) error {
	if len(rows) == 0 {
		return nil
	}
	keyWidth, labelWidth, typeWidth, targetWidth := p.measureRows(rows)
	for _, row := range rows {
		if _, err := fmt.Fprintln(p.out, p.formatRow(row, keyWidth, labelWidth, typeWidth, targetWidth)); err != nil {
			return err
		}
	}
	return nil
}

func (p *channelFieldPresenter) renderAlignedRowsWithDetails(rows []channelFieldRow, fields []api.CustomField) error {
	if len(rows) == 0 {
		return nil
	}
	keyWidth, labelWidth, typeWidth, targetWidth := p.measureRows(rows)
	for index, row := range rows {
		if _, err := fmt.Fprintln(p.out, p.formatRow(row, keyWidth, labelWidth, typeWidth, targetWidth)); err != nil {
			return err
		}
		field := fields[index]
		if len(field.SelectOptions) > 0 {
			if _, err := fmt.Fprintf(p.out, "    %s\n", p.dim("options")); err != nil {
				return err
			}
			if err := p.renderOptionTable(field.SelectOptions); err != nil {
				return err
			}
		}
		if err := p.renderExtraDetails(field, "    "); err != nil {
			return err
		}
	}
	return nil
}

func (p *channelFieldPresenter) measureRows(rows []channelFieldRow) (int, int, int, int) {
	keyWidth := 0
	labelWidth := 0
	typeWidth := 0
	targetWidth := 0
	for _, row := range rows {
		keyWidth = max(keyWidth, ansi.StringWidth(row.key))
		labelWidth = max(labelWidth, ansi.StringWidth(row.label))
		typeWidth = max(typeWidth, ansi.StringWidth(row.kind))
		targetWidth = max(targetWidth, ansi.StringWidth(row.target))
	}
	return keyWidth, labelWidth, typeWidth, targetWidth
}

func (p *channelFieldPresenter) formatRow(row channelFieldRow, keyWidth, labelWidth, typeWidth, targetWidth int) string {
	parts := []string{
		"  " + p.fieldKey(p.padRight(row.key, keyWidth)),
		p.value(p.padRight(row.label, labelWidth)),
		p.fieldType(p.padRight(row.kind, typeWidth)),
	}
	if targetWidth > 0 {
		parts = append(parts, p.relationTarget(p.padRight(row.target, targetWidth)))
	}
	for _, flag := range row.flags {
		parts = append(parts, p.flag(flag))
	}
	return strings.Join(filterEmptyStrings(parts), "  ")
}

func (p *channelFieldPresenter) renderOptionTable(options []api.SelectOption) error {
	nameWidth := len("name")
	slugWidth := len("slug")
	idWidth := len("id")
	posWidth := len("pos")
	for _, option := range options {
		nameWidth = max(nameWidth, ansi.StringWidth(channelFieldOptionLabel(option)))
		slugWidth = max(slugWidth, ansi.StringWidth(option.Slug))
		idWidth = max(idWidth, ansi.StringWidth(option.ID))
		posWidth = max(posWidth, ansi.StringWidth(fmt.Sprintf("%d", option.Position)))
	}

	header := strings.Join([]string{
		"      " + p.dim(p.padRight("name", nameWidth)),
		p.dim(p.padRight("slug", slugWidth)),
		p.dim(p.padRight("id", idWidth)),
		p.dim(p.padRight("pos", posWidth)),
	}, "  ")
	if _, err := fmt.Fprintln(p.out, header); err != nil {
		return err
	}

	for _, option := range options {
		line := strings.Join([]string{
			"      " + p.value(p.padRight(channelFieldOptionLabel(option), nameWidth)),
			p.dim(p.padRight(option.Slug, slugWidth)),
			p.dim(p.padRight(option.ID, idWidth)),
			p.dim(p.padRight(fmt.Sprintf("%d", option.Position), posWidth)),
		}, "  ")
		if _, err := fmt.Fprintln(p.out, line); err != nil {
			return err
		}
	}
	return nil
}

func (p *channelFieldPresenter) renderExtraDetails(field api.CustomField, indent string) error {
	type detailLine struct {
		label string
		value string
	}
	lines := make([]detailLine, 0, 6)
	if field.GeoType != "" {
		lines = append(lines, detailLine{label: "geo", value: field.GeoType})
	}
	if field.Hint != "" {
		lines = append(lines, detailLine{label: "hint", value: field.Hint})
	}
	if field.RequiredExpression != "" {
		lines = append(lines, detailLine{label: "required when", value: field.RequiredExpression})
	}
	if field.CalculatedExpression != "" {
		lines = append(lines, detailLine{label: "calculated", value: field.CalculatedExpression})
	}
	if field.CalculationType != "" {
		lines = append(lines, detailLine{label: "calc type", value: field.CalculationType})
	}
	extra := channelFieldInterestingExtra(field)
	if value, ok := extra["text_formatting"].(string); ok && value != "" {
		lines = append(lines, detailLine{label: "text formatting", value: value})
		delete(extra, "text_formatting")
	}
	if value, ok := extra["auto_expand"].(bool); ok {
		lines = append(lines, detailLine{label: "auto expand", value: fmt.Sprintf("%v", value)})
		delete(extra, "auto_expand")
	}
	if len(extra) > 0 {
		raw, err := json.Marshal(extra)
		if err != nil {
			return fmt.Errorf("encode field extra metadata: %w", err)
		}
		lines = append(lines, detailLine{label: "extra", value: string(raw)})
	}

	for _, line := range lines {
		if _, err := fmt.Fprintf(p.out, "%s%s %s\n", indent, p.dim(line.label+":"), p.value(line.value)); err != nil {
			return err
		}
	}
	return nil
}

// Field styles come from the shared semantic theme. fieldKey and value keep
// the terminal's own foreground (near-white hex values disappeared on light
// backgrounds); flags are words, so color only reinforces them.
func (p *channelFieldPresenter) fieldKey(value string) string {
	return p.style(value, "", true)
}

func (p *channelFieldPresenter) value(value string) string {
	return value
}

func (p *channelFieldPresenter) fieldType(value string) string {
	return p.style(value, p.theme.Palette().Path, false)
}

func (p *channelFieldPresenter) relationTarget(value string) string {
	return p.style(value, p.theme.Palette().Success, false)
}

func (p *channelFieldPresenter) sectionHeader(value string) string {
	return p.style(value, p.theme.Palette().Accent, true)
}

func (p *channelFieldPresenter) flag(value string) string {
	palette := p.theme.Palette()
	color := palette.Muted
	switch value {
	case "required":
		color = palette.Warning
	case "unique", "localized", "encrypted", "private_storage":
		color = palette.Accent
	}
	return p.style(value, color, true)
}

func (p *channelFieldPresenter) dim(value string) string {
	return p.style(value, p.theme.Palette().Muted, false)
}

func (p *channelFieldPresenter) style(value string, color string, bold bool) string {
	if strings.TrimSpace(value) == "" {
		return value
	}
	return p.theme.Paint(value, color, bold)
}

// padRight pads to a display width: labels may hold accented letters, CJK or
// emoji, whose byte length is not their width on screen.
func (p *channelFieldPresenter) padRight(value string, width int) string {
	if gap := width - ansi.StringWidth(value); gap > 0 {
		return value + strings.Repeat(" ", gap)
	}
	return value
}

func channelFieldDisplayLabel(field api.CustomField) string {
	if strings.TrimSpace(field.Label) != "" {
		return field.Label
	}
	return field.Name
}

func channelFieldInterestingExtra(field api.CustomField) map[string]any {
	if len(field.Extra) == 0 {
		return nil
	}
	extra := make(map[string]any, len(field.Extra))
	for key, value := range field.Extra {
		switch key {
		case "created_at", "updated_at":
			continue
		default:
			extra[key] = value
		}
	}
	if len(extra) == 0 {
		return nil
	}
	return extra
}

func filterEmptyStrings(values []string) []string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			items = append(items, value)
		}
	}
	return items
}
