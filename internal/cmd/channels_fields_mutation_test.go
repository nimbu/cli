package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func TestChannelsFieldsAddPatchesOneNewField(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/channels/articles/customizations":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPatch && r.URL.Path == "/channels/articles":
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode patch body: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"c1","slug":"articles","customizations":[{"id":"f1","name":"title","type":"string"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &ChannelsFieldsAddCmd{
		Channel:     "articles",
		Name:        "title",
		Assignments: []string{"type=string", "label=Title"},
	}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run fields add: %v", err)
	}

	fields := gotBody["customizations"].([]any)
	if len(fields) != 1 {
		t.Fatalf("customizations = %#v", fields)
	}
	field := fields[0].(map[string]any)
	if field["name"] != "title" || field["type"] != "string" || field["label"] != "Title" {
		t.Fatalf("unexpected field payload: %#v", field)
	}
}

func TestChannelsFieldsAddRequiresNameFlag(t *testing.T) {
	ctx, _, _ := newContractTestContext(t, "https://api.example.test", output.Mode{})
	cmd := &ChannelsFieldsAddCmd{
		Channel:     "articles",
		Assignments: []string{"name=title", "type=string"},
	}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), "--name") {
		t.Fatalf("expected --name identity error, got %v", err)
	}
}

func TestChannelsFieldsUpdateResolvesFieldIdentityAndAllowsRenamePayload(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/channels/articles/customizations":
			_, _ = w.Write([]byte(`[{"id":"f1","name":"title","type":"string","label":"Old"}]`))
		case r.Method == http.MethodPatch && r.URL.Path == "/channels/articles":
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode patch body: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"c1","slug":"articles","customizations":[{"id":"f1","name":"headline","type":"string","label":"Headline"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &ChannelsFieldsUpdateCmd{
		Channel:     "articles",
		Field:       "title",
		Assignments: []string{"name=headline", "label=Headline"},
	}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run fields update: %v", err)
	}

	field := gotBody["customizations"].([]any)[0].(map[string]any)
	if field["id"] != "f1" || field["name"] != "headline" || field["label"] != "Headline" {
		t.Fatalf("unexpected field payload: %#v", field)
	}
}

func TestChannelsFieldsDeleteRequiresForceAndSendsDestroyMarker(t *testing.T) {
	ctx, _, _ := newContractTestContext(t, "https://api.example.test", output.Mode{})
	if err := (&ChannelsFieldsDeleteCmd{Channel: "articles", Field: "title"}).Run(ctx, &RootFlags{Site: "demo"}); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected --force error, got %v", err)
	}

	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/channels/articles/customizations":
			_, _ = w.Write([]byte(`[{"id":"f1","name":"title","type":"string"}]`))
		case r.Method == http.MethodPatch && r.URL.Path == "/channels/articles":
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode patch body: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"c1","slug":"articles","customizations":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, _, _ = newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &ChannelsFieldsDeleteCmd{Channel: "articles", Field: "title"}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo", Force: true}); err != nil {
		t.Fatalf("run fields delete: %v", err)
	}

	field := gotBody["customizations"].([]any)[0].(map[string]any)
	if field["id"] != "f1" || field["_destroy"] != true {
		t.Fatalf("unexpected delete payload: %#v", field)
	}
}

func TestChannelsFieldsReplaceUsesReplaceQuery(t *testing.T) {
	var gotReplace string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == "/channels/articles":
			gotReplace = r.URL.Query().Get("replace")
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode patch body: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"c1","slug":"articles","customizations":[{"id":"f1","name":"title","type":"string"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	file := writeFieldsFile(t, `[{"name":"title","type":"string"}]`)
	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &ChannelsFieldsReplaceCmd{Channel: "articles", File: file}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo", Force: true}); err != nil {
		t.Fatalf("run fields replace: %v", err)
	}

	if gotReplace != "1" {
		t.Fatalf("replace query = %q, want 1", gotReplace)
	}
	if len(gotBody["customizations"].([]any)) != 1 {
		t.Fatalf("unexpected body: %#v", gotBody)
	}
}

func TestChannelsFieldsApplyRejectsNullJSON(t *testing.T) {
	file := writeFieldsFile(t, `null`)
	ctx, _, _ := newContractTestContext(t, "https://api.example.test", output.Mode{})
	cmd := &ChannelsFieldsApplyCmd{Channel: "articles", File: file}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), "must be an array") {
		t.Fatalf("expected array validation error, got %v", err)
	}
}

func TestChannelsFieldsDiffComparesNormalizedFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/channels/articles/customizations" {
			_, _ = w.Write([]byte(`[{"id":"f1","name":"title","type":"string","label":"Old"}]`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	file := writeFieldsFile(t, `[{"id":"different","name":"title","type":"string","label":"New"}]`)
	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &ChannelsFieldsDiffCmd{Channel: "articles", File: file}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run fields diff: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(out.Bytes(), &body); err != nil {
		t.Fatalf("decode diff output: %v", err)
	}
	updated := body["updated"].([]any)
	if len(updated) != 1 || !strings.Contains(updated[0].(map[string]any)["path"].(string), "label") {
		t.Fatalf("unexpected diff output: %#v", body)
	}
}

func writeFieldsFile(t *testing.T, body string) string {
	t.Helper()
	path := t.TempDir() + "/fields.json"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write fields file: %v", err)
	}
	return path
}
