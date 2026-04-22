package migrate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
)

const localizedCassetteDir = "../testdata/nimbu_api/localized_project_approaches"

func TestLocalizedProjectApproachesCassetteCopyUpdatesStaleEN(t *testing.T) {
	targetEN := readCassetteEntries(t, "target_entries_en_stale.json")
	putDefault := false
	putEN := false

	srv := localizedCassetteServer(t, &targetEN, &putDefault, &putEN)
	defer srv.Close()

	fromClient := api.New(srv.URL, "").WithSite("source")
	toClient := api.New(srv.URL, "").WithSite("target")
	result, err := CopyChannelEntries(context.Background(), fromClient, toClient,
		ChannelRef{SiteRef: SiteRef{BaseURL: srv.URL, Site: "source"}, Channel: "project_approaches"},
		ChannelRef{SiteRef: SiteRef{BaseURL: srv.URL, Site: "target"}, Channel: "project_approaches"},
		RecordCopyOptions{Upsert: "slug"},
	)
	if err != nil {
		t.Fatalf("copy channel entries from cassette: %v", err)
	}
	if !putEN {
		t.Fatal("expected EN content-locale update")
	}
	if putDefault {
		t.Fatal("did not expect default-locale update when only EN fields differ")
	}
	if len(result.Items) != 1 || result.Items[0].Action != "update" {
		t.Fatalf("expected default entry update because localized EN differed, got %#v", result.Items)
	}
	if got := stringValue(targetEN[0]["title"]); got != "Exploration" {
		t.Fatalf("expected EN title updated from cassette, got %q", got)
	}
	if warnings := localizedWarnings(result.Warnings); len(warnings) != 0 {
		t.Fatalf("expected localized validation to pass after update, got %v", warnings)
	}
}

func TestLocalizedProjectApproachesCassetteValidationReportsStaleEN(t *testing.T) {
	targetEN := readCassetteEntries(t, "target_entries_en_stale.json")
	putDefault := false
	putEN := false

	srv := localizedCassetteServer(t, &targetEN, &putDefault, &putEN)
	defer srv.Close()

	fromClient := api.New(srv.URL, "").WithSite("source")
	toClient := api.New(srv.URL, "").WithSite("target")
	var channels []api.ChannelDetail
	if err := json.Unmarshal(readCassette(t, "channels.json"), &channels); err != nil {
		t.Fatalf("decode channels cassette: %v", err)
	}
	channelMap := map[string]api.ChannelDetail{"project_approaches": channels[0]}

	warnings := ValidateEntries(context.Background(), fromClient, toClient,
		map[string]map[string]string{"project_approaches": {"fixture-source-entry-start": "fixture-target-entry-start"}},
		channelMap,
	)
	want := `channel=project_approaches entry=Exploration locale=en field=title: mismatch (source="Exploration", target="Exploratie")`
	if !containsString(warnings, want) {
		t.Fatalf("expected localized cassette warning %q, got %v", want, warnings)
	}
}

func TestLocalizedProjectApproachesCassetteContainsNoMongoIDs(t *testing.T) {
	entries, err := os.ReadDir(localizedCassetteDir)
	if err != nil {
		t.Fatalf("read cassette dir: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data := readCassette(t, entry.Name())
		var value any
		if err := json.Unmarshal(data, &value); err != nil {
			t.Fatalf("decode %s: %v", entry.Name(), err)
		}
		assertNoMongoIDs(t, entry.Name(), value)
	}
}

func localizedCassetteServer(t *testing.T, targetEN *[]map[string]any, putDefault, putEN *bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		site := r.Header.Get("X-Nimbu-Site")
		switch r.URL.Path {
		case "/sites/source/settings":
			writeCassette(t, w, "source_settings.json")
		case "/sites/target/settings":
			writeCassette(t, w, "target_settings.json")
		case "/channels":
			writeCassette(t, w, "channels.json")
		case "/channels/project_approaches/entries":
			if r.Method != http.MethodGet {
				http.NotFound(w, r)
				return
			}
			locale := r.URL.Query().Get("content_locale")
			switch {
			case site == "source" && locale == "en":
				writeCassette(t, w, "source_entries_en.json")
			case site == "source":
				writeCassette(t, w, "source_entries_nl.json")
			case site == "target" && locale == "en":
				writeJSON(t, w, *targetEN)
			case site == "target":
				writeCassette(t, w, "target_entries_nl_match.json")
			default:
				http.NotFound(w, r)
			}
		case "/channels/project_approaches/entries/fixture-target-entry-start":
			if r.Method != http.MethodPut || site != "target" {
				http.NotFound(w, r)
				return
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode update body: %v", err)
			}
			if r.URL.Query().Get("content_locale") == "en" {
				*putEN = true
				updated := cloneTestRecord((*targetEN)[0])
				for key, value := range body {
					updated[key] = value
				}
				*targetEN = []map[string]any{updated}
				writeJSON(t, w, updated)
				return
			}
			*putDefault = true
			writeCassetteFirstEntry(t, w, "target_entries_nl_match.json")
		default:
			http.NotFound(w, r)
		}
	}))
}

func readCassetteEntries(t *testing.T, name string) []map[string]any {
	t.Helper()
	var entries []map[string]any
	if err := json.Unmarshal(readCassette(t, name), &entries); err != nil {
		t.Fatalf("decode cassette %s: %v", name, err)
	}
	return entries
}

func readCassette(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(localizedCassetteDir + "/" + name)
	if err != nil {
		t.Fatalf("read cassette %s: %v", name, err)
	}
	return data
}

func writeCassette(t *testing.T, w http.ResponseWriter, name string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(readCassette(t, name))
}

func writeCassetteFirstEntry(t *testing.T, w http.ResponseWriter, name string) {
	t.Helper()
	entries := readCassetteEntries(t, name)
	if len(entries) == 0 {
		t.Fatalf("cassette %s is empty", name)
	}
	writeJSON(t, w, entries[0])
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode JSON response: %v", err)
	}
}

func localizedWarnings(warnings []string) []string {
	var out []string
	for _, warning := range warnings {
		if strings.Contains(warning, " locale=") {
			out = append(out, warning)
		}
	}
	return out
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func assertNoMongoIDs(t *testing.T, path string, value any) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			assertNoMongoIDs(t, path+"."+key, nested)
		}
	case []any:
		for _, nested := range typed {
			assertNoMongoIDs(t, path+"[]", nested)
		}
	case string:
		if len(typed) == 24 && isLowerHex(typed) {
			t.Fatalf("cassette contains Mongo-like ID at %s: %s", path, typed)
		}
	}
}

func isLowerHex(value string) bool {
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
