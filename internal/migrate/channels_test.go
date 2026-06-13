package migrate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
)

// TestCopyAllChannelsCircularDependencies verifies that copying channels with
// mutual references to an empty target creates placeholders for missing
// dependencies first, so the target's reference validation passes.
func TestCopyAllChannelsCircularDependencies(t *testing.T) {
	fromSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/channels" {
			_, _ = w.Write([]byte(`[
				{"id":"1","slug":"alpha","name":"Alpha","customizations":[{"name":"rel","label":"Rel","type":"belongs_to_many","reference":"beta"}]},
				{"id":"2","slug":"beta","name":"Beta","customizations":[{"name":"rel","label":"Rel","type":"belongs_to_many","reference":"alpha"}]},
				{"id":"3","slug":"gamma","name":"Gamma","rss_enabled":true,"rss_title":"Feed","rss_description":"Desc","rss_title_field":"titel","rss_description_field":"summary","customizations":[{"name":"titel","label":"Titel","type":"string"},{"name":"summary","label":"Summary","type":"text"}]}
			]`))
			return
		}
		http.NotFound(w, r)
	}))
	defer fromSrv.Close()

	// Target mimics the platform: creating/updating a channel whose reference
	// fields point at non-existing channels fails validation.
	created := map[string]bool{}
	rssPatches := map[string]map[string]any{}
	validateRefs := func(w http.ResponseWriter, payload map[string]any) bool {
		raw, _ := payload["customizations"].([]any)
		for _, rawField := range raw {
			field, _ := rawField.(map[string]any)
			ref, _ := field["reference"].(string)
			if ref != "" && !created[ref] {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"message":"Following parameters are pointing to a non-existing object: :reference (` + ref + `)"}`))
				return false
			}
		}
		return true
	}
	toSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/channels/"):
			slug := strings.TrimPrefix(r.URL.Path, "/channels/")
			if !created[slug] {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(`{"id":"x","slug":"` + slug + `","name":"` + slug + `"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/channels":
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if !validateRefs(w, payload) {
				return
			}
			if enabled, _ := payload["rss_enabled"].(bool); enabled {
				// The platform requires rss_title etc. when enabling RSS; the
				// copy must defer RSS to a follow-up update instead.
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"message":"Validation Failed (rss_title: rss_title kan niet leeg zijn)"}`))
				return
			}
			slug, _ := payload["slug"].(string)
			created[slug] = true
			_, _ = w.Write([]byte(`{"id":"x","slug":"` + slug + `"}`))
		case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/channels/"):
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if !validateRefs(w, payload) {
				return
			}
			slug := strings.TrimPrefix(r.URL.Path, "/channels/")
			if enabled, _ := payload["rss_enabled"].(bool); enabled {
				rssPatches[slug] = payload
			}
			_, _ = w.Write([]byte(`{"id":"x","slug":"` + slug + `"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer toSrv.Close()

	result, err := CopyAllChannelsWithOptions(
		context.Background(),
		api.New(fromSrv.URL, ""),
		api.New(toSrv.URL, ""),
		SiteRef{BaseURL: fromSrv.URL, Site: "from"},
		SiteRef{BaseURL: toSrv.URL, Site: "to"},
		ChannelCopyOptions{Existing: ExistingContentUpdate},
	)
	if err != nil {
		t.Fatalf("CopyAllChannelsWithOptions error = %v", err)
	}
	if len(result.Items) != 3 {
		t.Fatalf("items = %d, want 3 (%v)", len(result.Items), result.Items)
	}
	if len(result.Placeholders) == 0 {
		t.Fatal("expected at least one circular-dependency placeholder")
	}
	if !created["alpha"] || !created["beta"] || !created["gamma"] {
		t.Fatalf("all channels should exist on target, got %v", created)
	}
	rss := rssPatches["gamma"]
	if rss == nil {
		t.Fatal("expected follow-up RSS update for gamma")
	}
	for key, want := range map[string]string{
		"rss_title":             "Feed",
		"rss_description":       "Desc",
		"rss_title_field":       "titel",
		"rss_description_field": "summary",
	} {
		if got, _ := rss[key].(string); got != want {
			t.Fatalf("rss patch %s = %q, want %q", key, got, want)
		}
	}
}
