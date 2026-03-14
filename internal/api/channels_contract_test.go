package api

import (
	"encoding/json"
	"testing"
)

func TestCustomFieldUnmarshalPreservesExtra(t *testing.T) {
	var field CustomField
	if err := json.Unmarshal([]byte(`{
		"id":"f1",
		"name":"destination",
		"label":"Destination",
		"type":"belongs_to",
		"reference":"destinations",
		"required":true,
		"custom_flag":"x"
	}`), &field); err != nil {
		t.Fatalf("unmarshal custom field: %v", err)
	}

	if !field.IsRelational() {
		t.Fatal("expected relational field")
	}
	if field.Reference != "destinations" {
		t.Fatalf("unexpected reference: %q", field.Reference)
	}
	if field.Extra["custom_flag"] != "x" {
		t.Fatalf("expected unknown field preserved, got %#v", field.Extra)
	}
}

func TestCustomFieldMarshalRoundTrip(t *testing.T) {
	input := `{
		"id":"f1",
		"name":"status",
		"label":"Status",
		"type":"select",
		"required":true,
		"position":3,
		"default_value":"draft"
	}`
	var field CustomField
	if err := json.Unmarshal([]byte(input), &field); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if field.Type != "select" {
		t.Fatalf("expected type=select, got %q", field.Type)
	}
	if field.Extra["position"] != float64(3) {
		t.Fatalf("expected Extra[position]=3, got %v", field.Extra["position"])
	}

	data, err := json.Marshal(field)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if raw["type"] != "select" {
		t.Fatalf("round-trip lost type: %v", raw["type"])
	}
	if raw["position"] != float64(3) {
		t.Fatalf("round-trip lost Extra position: %v", raw["position"])
	}
	if raw["default_value"] != "draft" {
		t.Fatalf("round-trip lost Extra default_value: %v", raw["default_value"])
	}
}

func TestBuildChannelDependencyGraph(t *testing.T) {
	channels := []ChannelDetail{
		{
			Slug: "articles",
			Customizations: []CustomField{
				{Name: "author", Type: "belongs_to", Reference: "authors"},
			},
		},
		{
			Slug: "authors",
			Customizations: []CustomField{
				{Name: "featured_article", Type: "belongs_to", Reference: "articles"},
			},
		},
		{
			Slug: "topics",
			Customizations: []CustomField{
				{Name: "owner", Type: "belongs_to", Reference: "authors"},
			},
		},
	}

	graph := BuildChannelDependencyGraph(channels)
	if got := graph.DirectDependencies("articles"); len(got) != 1 || got[0] != "authors" {
		t.Fatalf("unexpected direct dependencies: %#v", got)
	}
	if got := graph.DirectDependants("authors"); len(got) != 2 {
		t.Fatalf("unexpected direct dependants: %#v", got)
	}
	if got := graph.TransitiveDependencies("topics"); len(got) != 2 {
		t.Fatalf("unexpected transitive dependencies: %#v", got)
	}
	if !graph.HasCircularDependencies("articles") {
		t.Fatal("expected circular dependency detection for articles")
	}
	if graph.HasCircularDependencies("topics") {
		t.Fatal("did not expect topics to participate in a cycle")
	}
}
