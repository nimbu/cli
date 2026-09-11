package cmd

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestDraftSnapshotToDocument(t *testing.T) {
	snapshot := map[string]any{
		"title":     "Draft About",
		"seo_title": "SEO",
		"page_items": []any{
			map[string]any{
				"slug":    "Theme",
				"type":    "select",
				"content": "Light",
			},
			map[string]any{
				"_id":  "item-blokken",
				"slug": "Blokken",
				"type": "canvas",
				"repeatables": []any{
					map[string]any{
						"_id":      "rep-draft-only",
						"slug":     "hero_stage",
						"position": 1,
						"page_items": []any{
							map[string]any{"slug": "Title", "type": "text", "content": "Draft Hero"},
							map[string]any{
								"slug": "Photos",
								"type": "canvas",
								"repeatables": []any{
									map[string]any{
										"id":       "photo-1",
										"slug":     "photo",
										"position": 2,
										"page_items": []any{
											map[string]any{"slug": "Image", "type": "file", "content": "https://cdn.example.test/a.jpg"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	doc, ok := draftSnapshotToDocument(snapshot)
	if !ok {
		t.Fatal("expected conversion to succeed")
	}
	if doc["title"] != "Draft About" || doc["seo_title"] != "SEO" {
		t.Fatalf("page fields = %#v", doc)
	}
	items, _ := doc["items"].(map[string]any)
	theme, _ := items["Theme"].(map[string]any)
	if theme["type"] != "select" || theme["content"] != "Light" {
		t.Fatalf("Theme = %#v", theme)
	}
	blokken, _ := items["Blokken"].(map[string]any)
	reps, _ := blokken["repeatables"].([]any)
	if len(reps) != 1 {
		t.Fatalf("repeatables = %#v", reps)
	}
	rep, _ := reps[0].(map[string]any)
	if rep["id"] != "rep-draft-only" || rep["slug"] != "hero_stage" || rep["position"] != 1 {
		t.Fatalf("repeatable = %#v", rep)
	}
	repItems, _ := rep["items"].(map[string]any)
	title, _ := repItems["Title"].(map[string]any)
	if title["content"] != "Draft Hero" {
		t.Fatalf("Title = %#v", title)
	}
	photos, _ := repItems["Photos"].(map[string]any)
	photoReps, _ := photos["repeatables"].([]any)
	photo, _ := photoReps[0].(map[string]any)
	if photo["id"] != "photo-1" || photo["position"] != 2 {
		t.Fatalf("nested repeatable = %#v", photo)
	}
	photoItems, _ := photo["items"].(map[string]any)
	image, _ := photoItems["Image"].(map[string]any)
	if image["type"] != "file" {
		t.Fatalf("Image = %#v", image)
	}
}

func TestDraftSnapshotToDocumentRejectsUnusableShape(t *testing.T) {
	if _, ok := draftSnapshotToDocument(nil); ok {
		t.Fatal("nil snapshot should fail")
	}
	if _, ok := draftSnapshotToDocument(map[string]any{"title": "x"}); ok {
		t.Fatal("missing page_items should fail")
	}
	if _, ok := draftSnapshotToDocument(map[string]any{"page_items": "nope"}); ok {
		t.Fatal("non-array page_items should fail")
	}
}

func TestDraftSnapshotToDocumentJSONFixtureRoundTripKeys(t *testing.T) {
	raw := `{
		"title":"Hi",
		"page_items":[
			{"_id":"a1","slug":"Intro","type":"text","content":"Hello"}
		]
	}`
	var snapshot map[string]any
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		t.Fatal(err)
	}
	doc, ok := draftSnapshotToDocument(snapshot)
	if !ok {
		t.Fatal("expected conversion")
	}
	items := doc["items"].(map[string]any)
	intro := items["Intro"].(map[string]any)
	if !reflect.DeepEqual(intro["content"], "Hello") {
		t.Fatalf("intro = %#v", intro)
	}
}
