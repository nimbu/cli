package migrate

import (
	"testing"

	"github.com/nimbu/cli/internal/api"
)

func TestChannelPayloadStripsSourceIDs(t *testing.T) {
	payload := channelPayload(api.ChannelDetail{
		ID:   "channel-id",
		Slug: "articles",
		Name: "Articles",
		Customizations: []api.CustomField{
			{
				ID:   "field-id",
				Name: "status",
				Type: "select",
				SelectOptions: []api.SelectOption{
					{ID: "option-id", Name: "Draft"},
				},
			},
		},
	}, "articles")

	if _, ok := payload["id"]; ok {
		t.Fatal("expected top-level id removed")
	}
	fields, ok := payload["customizations"].([]map[string]any)
	if !ok || len(fields) != 1 {
		t.Fatalf("unexpected customizations payload: %#v", payload["customizations"])
	}
	if _, ok := fields[0]["id"]; ok {
		t.Fatal("expected field id removed")
	}
	options, ok := fields[0]["select_options"].([]any)
	if !ok || len(options) != 1 {
		t.Fatalf("unexpected select options payload: %#v", fields[0]["select_options"])
	}
	if option, ok := options[0].(map[string]any); !ok || option["id"] != nil {
		t.Fatalf("expected select option id removed, got %#v", options[0])
	}
}

func TestSanitizeMenuDocumentStripsNestedIDs(t *testing.T) {
	menu := api.MenuDocument{
		"id": "menu-id",
		"items": []any{
			map[string]any{
				"id":          "item-id",
				"title":       "Home",
				"target_page": "home",
				"children": []any{
					map[string]any{"id": "child-id", "title": "Child"},
				},
			},
		},
	}
	sanitizeMenuDocument(menu)

	if _, ok := menu["id"]; ok {
		t.Fatal("expected top-level menu id removed")
	}
	items := menu["items"].([]any)
	item := items[0].(map[string]any)
	if _, ok := item["id"]; ok {
		t.Fatal("expected nested item id removed")
	}
	if _, ok := item["target_page"]; ok {
		t.Fatal("expected target_page removed")
	}
	child := item["children"].([]any)[0].(map[string]any)
	if _, ok := child["id"]; ok {
		t.Fatal("expected child item id removed")
	}
}
