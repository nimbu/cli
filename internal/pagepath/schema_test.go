package pagepath

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSchemaUnmarshalFixture(t *testing.T) {
	schema := loadSchema(t)

	if schema.Template.ID != "0000000000000000000000bb" || schema.Template.Name != "home" {
		t.Fatalf("template = %#v", schema.Template)
	}
	if got := schema.BlockSlugs("Blokken"); !reflect.DeepEqual(got, []string{"hero_stage", "proof_strip", "case_cards"}) {
		t.Fatalf("BlockSlugs = %v", got)
	}
	if got := schema.BlockSlugs("missing"); len(got) != 0 {
		t.Fatalf("missing canvas slugs = %v", got)
	}

	top := schema.OptionsFor("", "", "Theme")
	if len(top) != 2 || top[0].Value != "dark" {
		t.Fatalf("top-level options = %#v", top)
	}
	nested := schema.OptionsFor("Blokken", "hero_stage", "Layout")
	if len(nested) != 2 || nested[1].Value != "narrow" {
		t.Fatalf("nested options = %#v", nested)
	}
	if got := schema.OptionsFor("Blokken", "hero_stage", "Title"); len(got) != 0 {
		t.Fatalf("unexpected options: %#v", got)
	}
	if len(schema.CurrentStructure) == 0 {
		t.Fatal("expected current_structure to be preserved")
	}
}

func TestSchemaUnmarshalEmptyObject(t *testing.T) {
	var schema Schema
	if err := json.Unmarshal([]byte(`{}`), &schema); err != nil {
		t.Fatalf("empty object: %v", err)
	}
	if schema.AvailableBlocks == nil {
		schema.AvailableBlocks = map[string][]BlockDef{}
	}
	if slugs := schema.BlockSlugs("Blokken"); len(slugs) != 0 {
		t.Fatalf("empty schema slugs = %v", slugs)
	}
	if opts := schema.OptionsFor("", "", "Theme"); len(opts) != 0 {
		t.Fatalf("empty schema options = %#v", opts)
	}
}

func TestSchemaUnmarshalToleratesArrayAvailableBlocks(t *testing.T) {
	var schema Schema
	if err := json.Unmarshal([]byte(`{"available_blocks":[],"select_options":[]}`), &schema); err != nil {
		t.Fatalf("array stubs: %v", err)
	}
	if len(schema.AvailableBlocks) != 0 {
		t.Fatalf("available_blocks = %#v", schema.AvailableBlocks)
	}
	if len(schema.SelectOptions) != 0 {
		t.Fatalf("select_options = %#v", schema.SelectOptions)
	}
}

func loadSchema(t *testing.T) *Schema {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema Schema
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	return &schema
}
