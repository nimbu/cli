package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

func TestLocalizedContentJSONPreservesCompleteAPIResponse(t *testing.T) {
	const item = `{"id":"1","title":"Default","seo_title":"SEO","custom":{"flag":true},"items":[{"label":"child"}],"translations":{"nl":{"title":"Nederlands"}}}`

	tests := []struct {
		name     string
		method   string
		path     string
		response string
		run      func(context.Context, *RootFlags) error
	}{
		{"pages list", http.MethodGet, "/pages", `[` + item + `]`, func(ctx context.Context, flags *RootFlags) error {
			return (&PagesListCmd{All: true}).Run(ctx, flags)
		}},
		{"pages create", http.MethodPost, "/pages", item, func(ctx context.Context, flags *RootFlags) error {
			return (&PagesCreateCmd{Assignments: []string{"title=Default"}}).Run(ctx, flags)
		}},
		{"blogs list", http.MethodGet, "/blogs", `[` + item + `]`, func(ctx context.Context, flags *RootFlags) error {
			return (&BlogsListCmd{All: true}).Run(ctx, flags)
		}},
		{"blogs get", http.MethodGet, "/blogs/1", item, func(ctx context.Context, flags *RootFlags) error {
			return (&BlogsGetCmd{Blog: "1"}).Run(ctx, flags)
		}},
		{"blogs create", http.MethodPost, "/blogs", item, func(ctx context.Context, flags *RootFlags) error {
			return (&BlogsCreateCmd{Assignments: []string{"name=Default"}}).Run(ctx, flags)
		}},
		{"blogs update", http.MethodPut, "/blogs/1", item, func(ctx context.Context, flags *RootFlags) error {
			return (&BlogsUpdateCmd{Blog: "1", Assignments: []string{"name=Default"}}).Run(ctx, flags)
		}},
		{"blog posts list", http.MethodGet, "/blogs/news/articles", `[` + item + `]`, func(ctx context.Context, flags *RootFlags) error {
			return (&BlogPostsListCmd{Blog: "news", All: true}).Run(ctx, flags)
		}},
		{"blog posts get", http.MethodGet, "/blogs/news/articles/1", item, func(ctx context.Context, flags *RootFlags) error {
			return (&BlogPostsGetCmd{Blog: "news", Post: "1"}).Run(ctx, flags)
		}},
		{"blog posts create", http.MethodPost, "/blogs/news/articles", item, func(ctx context.Context, flags *RootFlags) error {
			return (&BlogPostsCreateCmd{Blog: "news", Assignments: []string{"title=Default"}}).Run(ctx, flags)
		}},
		{"blog posts update", http.MethodPut, "/blogs/news/articles/1", item, func(ctx context.Context, flags *RootFlags) error {
			return (&BlogPostsUpdateCmd{Blog: "news", Post: "1", Assignments: []string{"title=Default"}}).Run(ctx, flags)
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/user" {
					_, _ = w.Write([]byte(`{}`))
					return
				}
				if r.Method != tc.method || r.URL.Path != tc.path {
					t.Fatalf("request = %s %s, want %s %s", r.Method, r.URL.Path, tc.method, tc.path)
				}
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()

			ctx, out, _ := newContractTestContext(t, server.URL, output.Mode{JSON: true})
			flags := &RootFlags{Site: "demo"}
			if err := tc.run(ctx, flags); err != nil {
				t.Fatal(err)
			}

			var decoded any
			if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
				t.Fatalf("decode output: %v", err)
			}
			var row map[string]any
			if list, ok := decoded.([]any); ok {
				row = list[0].(map[string]any)
			} else {
				row = decoded.(map[string]any)
			}
			if row["seo_title"] != "SEO" || row["custom"].(map[string]any)["flag"] != true {
				t.Fatalf("lossy JSON output: %#v", row)
			}
			if row["translations"].(map[string]any)["nl"].(map[string]any)["title"] != "Nederlands" {
				t.Fatalf("translations missing: %#v", row)
			}
		})
	}
}

func TestPagesListHumanProjectsRequestedLocale(t *testing.T) {
	server := localizedListServer(t, "/pages", `[{"id":"1","fullpath":"home","title":"Default","template":"page","published":true,"translations":{"nl":{"title":"Nederlands"}}}]`)
	defer server.Close()

	ctx, out, _ := newContractTestContext(t, server.URL, output.Mode{})
	cmd := &PagesListCmd{QueryFlags: QueryFlags{Locale: "nl"}, All: true}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Nederlands") || strings.Contains(out.String(), "Default") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestProductAndChannelEntryPlainFieldsReadDynamicValues(t *testing.T) {
	tests := []struct {
		name string
		path string
		body string
		run  func(context.Context, *RootFlags) error
	}{
		{"product", "/products", `[{"id":"p1","custom_code":"limited"}]`, func(ctx context.Context, flags *RootFlags) error {
			return (&ProductsListCmd{QueryFlags: QueryFlags{Fields: "id,custom_code"}, All: true}).Run(ctx, flags)
		}},
		{"channel entry", "/channels/news/entries", `[{"id":"e1","custom_code":"featured"}]`, func(ctx context.Context, flags *RootFlags) error {
			return (&ChannelEntriesListCmd{Channel: "news", QueryFlags: QueryFlags{Fields: "id,custom_code"}, All: true}).Run(ctx, flags)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := localizedListServer(t, tc.path, tc.body)
			defer server.Close()
			ctx, out, _ := newContractTestContext(t, server.URL, output.Mode{Plain: true})
			if err := tc.run(ctx, &RootFlags{Site: "demo"}); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "limited") && !strings.Contains(out.String(), "featured") {
				t.Fatalf("output = %q", out.String())
			}
		})
	}
}

func TestTranslationsListJSONAppliesLocaleExpansionWhenFiltered(t *testing.T) {
	server := localizedListServer(t, "/translations", `[{"key":"greeting","values":{"en":"Hello","nl":"Hallo"}}]`)
	defer server.Close()
	ctx, out, _ := newContractTestContext(t, server.URL, output.Mode{JSON: true})
	cmd := &TranslationsListCmd{QueryFlags: QueryFlags{Locale: "nl"}, All: true}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0]["key"] != "greeting" || got[0]["locale"] != "nl" || got[0]["value"] != "Hallo" {
		t.Fatalf("output = %#v", got)
	}
}

func TestChannelEntriesGetSlugFallbackPreservesCompleteJSONDocument(t *testing.T) {
	const response = `[{
		"id":"e1",
		"slug":"hello",
		"published":false,
		"position":0,
		"nullable":null,
		"large_integer":9007199254740993,
		"dynamic_field":"custom",
		"translations":{"nl":{"dynamic_field":"aangepast"}}
	}]`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/channels/news/entries/hello":
			http.NotFound(w, r)
		case "/channels/news/entries":
			_, _ = w.Write([]byte(response))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx, out, _ := newContractTestContext(t, server.URL, output.Mode{JSON: true})
	cmd := &ChannelEntriesGetCmd{Channel: "news", Entry: "hello"}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatal(err)
	}

	decoder := json.NewDecoder(bytes.NewReader(out.Bytes()))
	decoder.UseNumber()
	var got map[string]any
	if err := decoder.Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got["published"] != false || got["position"] != json.Number("0") {
		t.Fatalf("false/zero fields not preserved: %#v", got)
	}
	if value, exists := got["nullable"]; !exists || value != nil {
		t.Fatalf("null field not preserved: %#v", got)
	}
	if got["large_integer"] != json.Number("9007199254740993") {
		t.Fatalf("large integer not preserved: %#v", got["large_integer"])
	}
	if got["dynamic_field"] != "custom" || got["translations"] == nil {
		t.Fatalf("dynamic/localized fields not preserved: %#v", got)
	}
}

func TestTranslationsListJSONFiltersMixedFlatAndValuesRecords(t *testing.T) {
	response := `[
		{"key":"flat.en","locale":"en","value":"Hello"},
		{"key":"flat.nl","locale":"nl_BE","value":"Hallo"},
		{"key":"mapped","values":{"en":"Mapped hello","nl-BE":"Mapped hallo"}}
	]`
	server := localizedListServer(t, "/translations", response)
	defer server.Close()

	ctx, out, _ := newContractTestContext(t, server.URL, output.Mode{JSON: true})
	cmd := &TranslationsListCmd{QueryFlags: QueryFlags{Locale: "nl-BE"}, All: true}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatal(err)
	}

	var got []api.Translation
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := []api.Translation{
		{Key: "flat.nl", Locale: "nl_BE", Value: "Hallo"},
		{Key: "mapped", Locale: "nl-BE", Value: "Mapped hallo"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("translations = %#v, want %#v", got, want)
	}
}

func TestBlogCommandsUseSlugAsDisplayHandle(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		run    func(context.Context, *RootFlags) error
	}{
		{"get", http.MethodGet, "/blogs/news", func(ctx context.Context, flags *RootFlags) error {
			return (&BlogsGetCmd{Blog: "news"}).Run(ctx, flags)
		}},
		{"create", http.MethodPost, "/blogs", func(ctx context.Context, flags *RootFlags) error {
			return (&BlogsCreateCmd{Assignments: []string{"name=News"}}).Run(ctx, flags)
		}},
		{"update", http.MethodPut, "/blogs/news", func(ctx context.Context, flags *RootFlags) error {
			return (&BlogsUpdateCmd{Blog: "news", Assignments: []string{"name=News"}}).Run(ctx, flags)
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tc.method || r.URL.Path != tc.path {
					t.Fatalf("request = %s %s, want %s %s", r.Method, r.URL.Path, tc.method, tc.path)
				}
				_, _ = w.Write([]byte(`{"id":"b1","slug":"news","name":"News"}`))
			}))
			defer server.Close()

			ctx, out, _ := newContractTestContext(t, server.URL, output.Mode{Plain: true})
			if err := tc.run(ctx, &RootFlags{Site: "demo"}); err != nil {
				t.Fatal(err)
			}
			if got := out.String(); got != "b1\tnews\tNews\n" {
				t.Fatalf("output = %q", got)
			}
		})
	}
}

func localizedListServer(t *testing.T, path, response string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user":
			_, _ = w.Write([]byte(`{}`))
		case path:
			_, _ = w.Write([]byte(response))
		default:
			http.NotFound(w, r)
		}
	}))
}
