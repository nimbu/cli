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
