package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func TestChannelEntriesUpdateLocaleSendsContentLocaleAndOnlyAssignedFields(t *testing.T) {
	var gotQuery string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user" {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		if r.Method != http.MethodPut || r.URL.Path != "/channels/c/entries/id" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(data, &gotBody); err != nil {
			t.Fatalf("decode body %q: %v", data, err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"id","title":"Hallo"}`))
	}))
	defer server.Close()

	ctx, _, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{JSON: true})
	cmd := &ChannelEntriesUpdateCmd{
		Channel:     "c",
		Entry:       "id",
		Locale:      "en",
		Assignments: []string{"title=Hallo"},
	}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("update entry: %v", err)
	}

	assertContentLocaleQuery(t, gotQuery)
	if len(gotBody) != 1 || gotBody["title"] != "Hallo" {
		t.Fatalf("body = %#v, want only title=Hallo", gotBody)
	}
}

func TestChannelEntriesUpdateFileSendsContentLocaleAndFileBody(t *testing.T) {
	file := filepath.Join(t.TempDir(), "entry.json")
	if err := os.WriteFile(file, []byte(`{"title":"Hallo"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var gotQuery string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user" {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		if r.Method != http.MethodPut || r.URL.Path != "/channels/c/entries/id" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"id","title":"Hallo"}`))
	}))
	defer server.Close()

	ctx, _, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{JSON: true})
	cmd := &ChannelEntriesUpdateCmd{
		Channel: "c",
		Entry:   "id",
		Locale:  "en",
		File:    file,
	}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("update entry from file: %v", err)
	}

	assertContentLocaleQuery(t, gotQuery)
	if len(gotBody) != 1 || gotBody["title"] != "Hallo" {
		t.Fatalf("body = %#v, want only title=Hallo", gotBody)
	}
}

func TestChannelEntriesGetLocaleSendsContentLocaleAndKeepsRawJSON(t *testing.T) {
	const response = `{
		"id":"id",
		"slug":"hello",
		"title":"Hallo",
		"published":false,
		"position":0,
		"nullable":null,
		"large_integer":9007199254740993,
		"dynamic_field":"custom",
		"nested":{"keep":true}
	}`
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user" {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		if r.Method != http.MethodGet || r.URL.Path != "/channels/c/entries/id" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	ctx, out, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{JSON: true})
	cmd := &ChannelEntriesGetCmd{
		Channel: "c",
		Entry:   "id",
		Locale:  "en",
	}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("get entry: %v", err)
	}

	assertContentLocaleQuery(t, gotQuery)

	decoder := json.NewDecoder(bytes.NewReader(out.Bytes()))
	decoder.UseNumber()
	var got map[string]any
	if err := decoder.Decode(&got); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if got["title"] != "Hallo" || got["dynamic_field"] != "custom" {
		t.Fatalf("dropped or reshaped fields: %#v", got)
	}
	if value, exists := got["nullable"]; !exists || value != nil {
		t.Fatalf("null field not preserved: %#v", got)
	}
	if got["published"] != false || got["position"] != json.Number("0") {
		t.Fatalf("false/zero fields not preserved: %#v", got)
	}
	if got["large_integer"] != json.Number("9007199254740993") {
		t.Fatalf("large integer not preserved: %#v", got["large_integer"])
	}
	nested, _ := got["nested"].(map[string]any)
	if nested["keep"] != true {
		t.Fatalf("nested field not preserved: %#v", got["nested"])
	}
}

func assertContentLocaleQuery(t *testing.T, raw string) {
	t.Helper()
	query, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("parse query %q: %v", raw, err)
	}
	if query.Get("content_locale") != "en" {
		t.Errorf("content_locale = %q, want en", query.Get("content_locale"))
	}
	if query.Get("locale") != "" {
		t.Errorf("legacy locale = %q, want empty", query.Get("locale"))
	}
}

func TestChannelEntriesLocaleFlagHelpExplainsPerLocaleReads(t *testing.T) {
	parser, _, err := newParser()
	if err != nil {
		t.Fatalf("new parser: %v", err)
	}

	want := map[string][]string{
		"nimbu channels entries get": {
			"per-locale",
			"translations map",
			"non-localized",
		},
		"nimbu channels entries update": {
			"per-locale",
			"translations map",
			"non-localized",
		},
	}

	found := map[string]string{}
	for _, command := range buildCommandContract(parser.Model).Commands {
		needles, ok := want[command.Path]
		if !ok {
			continue
		}
		for _, flag := range command.Flags {
			if flag.Name != "locale" {
				continue
			}
			help := strings.ToLower(flag.Help)
			found[command.Path] = flag.Help
			for _, needle := range needles {
				if !strings.Contains(help, needle) {
					t.Errorf("%s --locale help = %q, want %q", command.Path, flag.Help, needle)
				}
			}
		}
	}
	for path := range want {
		if _, ok := found[path]; !ok {
			t.Errorf("%s --locale missing from command contract", path)
		}
	}
}
