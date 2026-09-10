package pagepath

import (
	"bytes"
	"encoding/json"
)

// Schema mirrors GET /pages/{id}/schema.
type Schema struct {
	Template         Template
	AvailableBlocks  map[string][]BlockDef
	SelectOptions    map[string][]Option
	CurrentStructure json.RawMessage
	raw              json.RawMessage
}

// Template identifies the page template.
type Template struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// BlockDef is one available repeatable slug for a canvas.
type BlockDef struct {
	Slug            string                `json:"slug"`
	Label           string                `json:"label"`
	Fields          []FieldDef            `json:"fields"`
	AvailableBlocks map[string][]BlockDef `json:"available_blocks,omitempty"`
}

// FieldDef is one editable inside a block definition.
type FieldDef struct {
	Slug            string                `json:"slug"`
	Label           string                `json:"label"`
	Type            string                `json:"type"`
	Options         []Option              `json:"options"`
	Reference       string                `json:"reference"`
	AvailableBlocks map[string][]BlockDef `json:"available_blocks,omitempty"`
}

// Option is a select/radio choice.
type Option struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// MarshalJSON returns the original API body when it was captured.
func (s Schema) MarshalJSON() ([]byte, error) {
	if len(s.raw) > 0 {
		return bytes.Clone(s.raw), nil
	}
	type alias Schema
	return json.Marshal(alias(s))
}

// UnmarshalJSON accepts the real object-shaped schema and the empty/array stubs.
func (s *Schema) UnmarshalJSON(data []byte) error {
	s.raw = bytes.Clone(data)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if t, ok := raw["template"]; ok {
		if err := json.Unmarshal(t, &s.Template); err != nil {
			// template may be null on pages without one
			s.Template = Template{}
		}
	}
	s.AvailableBlocks = objectOrEmpty[map[string][]BlockDef](raw["available_blocks"])
	s.SelectOptions = objectOrEmpty[map[string][]Option](raw["select_options"])
	if cs, ok := raw["current_structure"]; ok {
		s.CurrentStructure = bytes.Clone(cs)
	}
	return nil
}

func objectOrEmpty[T any](raw json.RawMessage) T {
	var zero T
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return zero
	}
	var out T
	if err := json.Unmarshal(trimmed, &out); err != nil {
		return zero
	}
	return out
}

// Blocks returns available block definitions for a canvas, in schema order.
func (s *Schema) Blocks(canvas string) []BlockDef {
	if s == nil {
		return nil
	}
	return s.AvailableBlocks[canvas]
}

// BlockSlugs returns available repeatable slugs for a canvas, in schema order.
func (s *Schema) BlockSlugs(canvas string) []string {
	blocks := s.Blocks(canvas)
	out := make([]string, 0, len(blocks))
	for _, block := range blocks {
		out = append(out, block.Slug)
	}
	return out
}

// FieldType returns the schema type for a field inside a block, or "".
func (s *Schema) FieldType(canvas, slug, field string) string {
	for _, f := range s.fieldsFor(canvas, slug) {
		if f.Slug == field {
			return f.Type
		}
	}
	return ""
}

// OptionsFor returns select options for a field.
// Top-level fields use canvas="" and repeatableSlug="".
func (s *Schema) OptionsFor(canvas, repeatableSlug, field string) []Option {
	if s == nil || s.SelectOptions == nil {
		return nil
	}
	key := field
	if canvas != "" || repeatableSlug != "" {
		key = canvas + "." + repeatableSlug + "." + field
	}
	return s.SelectOptions[key]
}

func (s *Schema) fieldsFor(canvas, slug string) []FieldDef {
	if s == nil {
		return nil
	}
	if fields := fieldsInBlocks(s.AvailableBlocks[canvas], slug); fields != nil {
		return fields
	}
	for _, blocks := range s.AvailableBlocks {
		if fields := findFields(blocks, canvas, slug); fields != nil {
			return fields
		}
	}
	return nil
}

func fieldsInBlocks(blocks []BlockDef, slug string) []FieldDef {
	for _, block := range blocks {
		if block.Slug == slug {
			return block.Fields
		}
	}
	return nil
}

func findFields(blocks []BlockDef, canvas, slug string) []FieldDef {
	if fields := fieldsInBlocks(blocks, slug); fields != nil {
		return fields
	}
	for _, block := range blocks {
		if fields := fieldsInBlocks(block.AvailableBlocks[canvas], slug); fields != nil {
			return fields
		}
		for _, field := range block.Fields {
			if fields := fieldsInBlocks(field.AvailableBlocks[canvas], slug); fields != nil {
				return fields
			}
			if nested := findFields(blockDefs(field.AvailableBlocks), canvas, slug); nested != nil {
				return nested
			}
		}
		if nested := findFields(blockDefs(block.AvailableBlocks), canvas, slug); nested != nil {
			return nested
		}
	}
	return nil
}

func blockDefs(m map[string][]BlockDef) []BlockDef {
	var out []BlockDef
	for _, blocks := range m {
		out = append(out, blocks...)
	}
	return out
}
