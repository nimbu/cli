package channeltypes

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestFieldType(t *testing.T) {
	options := func(names ...string) []api.SelectOption {
		out := make([]api.SelectOption, 0, len(names))
		for _, name := range names {
			out = append(out, api.SelectOption{Name: name})
		}
		return out
	}

	tests := []struct {
		name    string
		field   api.CustomField
		want    string
		imports []string
	}{
		{name: "string", field: api.CustomField{Type: "string"}, want: "string"},
		{name: "text", field: api.CustomField{Type: "text"}, want: "string"},
		{name: "email", field: api.CustomField{Type: "email"}, want: "string"},
		{name: "integer", field: api.CustomField{Type: "integer"}, want: "number"},
		{name: "float", field: api.CustomField{Type: "float"}, want: "number"},
		{name: "boolean", field: api.CustomField{Type: "boolean"}, want: "boolean"},
		{name: "calculated integer", field: api.CustomField{Type: "calculated", CalculationType: "integer"}, want: "number"},
		{name: "calculated float", field: api.CustomField{Type: "calculated", CalculationType: "float"}, want: "number"},
		{name: "calculated string", field: api.CustomField{Type: "calculated", CalculationType: "string"}, want: "string"},
		{name: "calculated unknown", field: api.CustomField{Type: "calculated"}, want: "string | number"},
		{name: "date", field: api.CustomField{Type: "date"}, want: "ISODate", imports: []string{"ISODate"}},
		{name: "date_time", field: api.CustomField{Type: "date_time"}, want: "DateTime", imports: []string{"DateTime"}},
		{name: "time", field: api.CustomField{Type: "time"}, want: "DateTime | ISODate", imports: []string{"DateTime", "ISODate"}},
		{
			name:    "select",
			field:   api.CustomField{Type: "select", SelectOptions: options("draft", "live")},
			want:    "Select<'draft' | 'live'>",
			imports: []string{"Select"},
		},
		{
			name:    "select escapes and dedupes",
			field:   api.CustomField{Type: "select", SelectOptions: options("it's", `a\b`, "it's", " ")},
			want:    `Select<'it\'s' | 'a\\b'>`,
			imports: []string{"Select"},
		},
		{name: "select without options", field: api.CustomField{Type: "select"}, want: "string"},
		{
			name:    "multi_select",
			field:   api.CustomField{Type: "multi_select", SelectOptions: options("a", "b")},
			want:    "MultiSelect<'a' | 'b'>",
			imports: []string{"MultiSelect"},
		},
		{name: "multi_select without options", field: api.CustomField{Type: "multi_select"}, want: "string[]"},
		{
			name:    "belongs_to known channel",
			field:   api.CustomField{Type: "belongs_to", Reference: "authors"},
			want:    "ReferenceTo<Authors>",
			imports: []string{"ReferenceTo"},
		},
		{
			name:    "belongs_to unknown channel",
			field:   api.CustomField{Type: "belongs_to", Reference: "elsewhere"},
			want:    "ReferenceTo",
			imports: []string{"ReferenceTo"},
		},
		{
			name:    "belongs_to_many known channel",
			field:   api.CustomField{Type: "belongs_to_many", Reference: "blog-posts"},
			want:    "ReferenceMany<BlogPosts>",
			imports: []string{"ReferenceMany"},
		},
		{
			name:    "belongs_to_many unknown channel",
			field:   api.CustomField{Type: "belongs_to_many"},
			want:    "ReferenceMany",
			imports: []string{"ReferenceMany"},
		},
		{name: "customer", field: api.CustomField{Type: "customer"}, want: "JSONField | string", imports: []string{"JSONField"}},
		{
			name:    "belongs_to native customer",
			field:   api.CustomField{Type: "belongs_to", Reference: "customers"},
			want:    "NimbuCustomer",
			imports: []string{"NimbuCustomer"},
		},
		{
			name:    "belongs_to_many native customers",
			field:   api.CustomField{Type: "belongs_to_many", Reference: "customers"},
			want:    "ReferenceMany",
			imports: []string{"ReferenceMany"},
		},
		{
			name:    "belongs_to native product",
			field:   api.CustomField{Type: "belongs_to", Reference: "products"},
			want:    "ReferenceTo",
			imports: []string{"ReferenceTo"},
		},
		{
			name:    "belongs_to native name shared with a channel",
			field:   api.CustomField{Type: "belongs_to", Reference: "orders"},
			want:    "ReferenceTo",
			imports: []string{"ReferenceTo"},
		},
		{name: "file", field: api.CustomField{Type: "file"}, want: "NimbuFile", imports: []string{"NimbuFile"}},
		{name: "gallery", field: api.CustomField{Type: "gallery"}, want: "NimbuGallery", imports: []string{"NimbuGallery"}},
		{name: "json", field: api.CustomField{Type: "json"}, want: "JSONField", imports: []string{"JSONField"}},
		{name: "geo", field: api.CustomField{Type: "geo", GeoType: "Point"}, want: "JSONField", imports: []string{"JSONField"}},
		{name: "unknown type", field: api.CustomField{Type: "hologram"}, want: "any"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRenderer([]api.ChannelDetail{{Slug: "authors"}, {Slug: "blog-posts"}, {Slug: "orders"}})
			if got := r.fieldType(tt.field); got != tt.want {
				t.Fatalf("fieldType() = %q, want %q", got, tt.want)
			}
			if len(r.imports) != len(tt.imports) {
				t.Fatalf("imports = %v, want %v", r.imports, tt.imports)
			}
			for _, name := range tt.imports {
				if !r.imports[name] {
					t.Fatalf("imports = %v, missing %q", r.imports, name)
				}
			}
		})
	}
}

func TestAssignTypeNames(t *testing.T) {
	got := assignTypeNames([]string{
		"blog-posts", "blog_posts", "blog posts", "products", "select", "date-time",
		"2020-archive", "---", "", "FAQ", "café-menu", "promise", "site",
	})
	want := map[string]string{
		"":             "Channel",
		"---":          fmt.Sprintf("Channel_%08x", slugHash("---")),
		"2020-archive": "Channel2020Archive",
		"FAQ":          "FAQ",
		"blog posts":   "BlogPosts",
		"blog-posts":   fmt.Sprintf("BlogPosts_%08x", slugHash("blog-posts")),
		"blog_posts":   "Blog_Posts",
		"café-menu":    "CaféMenu",
		"date-time":    "DateTimeChannel",
		"products":     "Products",
		"promise":      "PromiseChannel",
		"select":       "SelectChannel",
		"site":         "Site",
	}
	for slug, name := range want {
		if got[slug] != name {
			t.Errorf("name for %q = %q, want %q", slug, got[slug], name)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("got %d names, want %d: %v", len(got), len(want), got)
	}
}

// A collision loser's name comes from its own slug, so adding more colliding
// channels that sort after the winner renames nobody. (A new slug that sorts
// before the winner takes the plain name; that is the documented trade-off.)
func TestAssignTypeNamesLoserNamesAreStable(t *testing.T) {
	before := assignTypeNames([]string{"blog-posts", "blog_posts"})
	after := assignTypeNames([]string{"blog-posts", "blog_posts", "blog.posts", "blog~posts"})
	if before["blog_posts"] != "Blog_Posts" || after["blog_posts"] != "Blog_Posts" {
		t.Fatalf("blog_posts renamed: before %q, after %q", before["blog_posts"], after["blog_posts"])
	}
	if before["blog-posts"] != after["blog-posts"] {
		t.Fatalf("blog-posts renamed: before %q, after %q", before["blog-posts"], after["blog-posts"])
	}
	seen := map[string]string{}
	for slug, name := range after {
		if other, dup := seen[name]; dup {
			t.Fatalf("%q and %q both named %q", slug, other, name)
		}
		seen[name] = slug
	}
}

func TestMemberTypeNullability(t *testing.T) {
	tests := []struct {
		name     string
		field    api.CustomField
		optional bool
		want     string
	}{
		{name: "optional string", field: api.CustomField{Type: "string"}, optional: true, want: "string | null"},
		{name: "required string", field: api.CustomField{Type: "string", Required: true}, want: "string"},
		{
			name:     "conditionally required string",
			field:    api.CustomField{Type: "string", Required: true, RequiredExpression: "kind == 'event'"},
			optional: true,
			want:     "string | null",
		},
		{name: "blank expression is plain required", field: api.CustomField{Type: "string", Required: true, RequiredExpression: "  "}, want: "string"},
		{name: "optional boolean", field: api.CustomField{Type: "boolean"}, optional: true, want: "boolean"},
		{name: "optional unknown", field: api.CustomField{Type: "hologram"}, optional: true, want: "any"},
		{name: "optional customer", field: api.CustomField{Type: "customer"}, optional: true, want: "JSONField | string | null"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRenderer(nil)
			if got := r.memberType(tt.field); got != tt.want {
				t.Fatalf("memberType() = %q, want %q", got, tt.want)
			}
			if got := !isRequired(tt.field); got != tt.optional {
				t.Fatalf("optional = %v, want %v", got, tt.optional)
			}
		})
	}
}

func TestRequiredExpressionIsDocumented(t *testing.T) {
	got := Interface(api.ChannelDetail{Slug: "events", Name: "Events", Customizations: []api.CustomField{
		{Name: "venue", Type: "string", Required: true, RequiredExpression: "kind == 'live'"},
	}})
	if !strings.Contains(got, "  /** Required when `kind == 'live'`. */\n  venue?: string | null\n") {
		t.Fatalf("conditional required field not rendered as optional with a note:\n%s", got)
	}
}

func TestAmbiguousNativeReferenceIsDocumented(t *testing.T) {
	got := Module([]api.ChannelDetail{
		{Slug: "products", Name: "Products", Customizations: []api.CustomField{{Name: "title", Type: "string"}}},
		{Slug: "reviews", Name: "Reviews", Customizations: []api.CustomField{{Name: "product", Type: "belongs_to", Reference: "products"}}},
	}, ModuleOptions{})
	if !strings.Contains(got, "  /** Untyped: `products` is both a channel and a native Nimbu type. */\n  product?: ReferenceTo | null\n") {
		t.Fatalf("ambiguous reference not left untyped with a note:\n%s", got)
	}
}

// A channel named like a cloud typings export must still be what NimbuChannels
// points at, so the augmentation goes through prefixed local aliases.
func TestModuleAugmentationUsesAliases(t *testing.T) {
	got := Module([]api.ChannelDetail{
		{Slug: "site", Name: "Site", Customizations: []api.CustomField{{Name: "title", Type: "string"}}},
	}, ModuleOptions{})
	for _, want := range []string{
		"\nexport type Site = {\n",
		"\ntype __NimbuChannel_Site = Site\n",
		"    'site': __NimbuChannel_Site\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("module lacks %q:\n%s", want, got)
		}
	}
}

func TestQuoteStringEscapesLineSeparators(t *testing.T) {
	if got, want := quoteString("a\u2028b\u2029c\x01"), `'a\u2028b\u2029c\u0001'`; got != want {
		t.Fatalf("quoteString() = %q, want %q", got, want)
	}
}

func TestPropertyKeyQuotesInvalidIdentifiers(t *testing.T) {
	tests := map[string]string{
		"title":      "title",
		"_private":   "_private",
		"$meta":      "$meta",
		"field_2":    "field_2",
		"2fa":        "'2fa'",
		"with-dash":  "'with-dash'",
		"with space": "'with space'",
	}
	for name, want := range tests {
		if got := propertyKey(name); got != want {
			t.Errorf("propertyKey(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestModuleGolden(t *testing.T) {
	got := Module(loadFixture(t), ModuleOptions{Site: "demo", StrictChannels: true})
	assertGolden(t, "channels.d.ts", got)
}

func TestModuleIsDeterministic(t *testing.T) {
	channels := loadFixture(t)
	first := Module(channels, ModuleOptions{Site: "demo"})
	reversed := make([]api.ChannelDetail, len(channels))
	for i, channel := range channels {
		reversed[len(channels)-1-i] = channel
	}
	if second := Module(reversed, ModuleOptions{Site: "demo"}); second != first {
		t.Fatalf("module output depends on input order")
	}
}

func TestModuleWithoutHelperImportsStaysAModule(t *testing.T) {
	got := Module([]api.ChannelDetail{{Slug: "notes", Name: "Notes", Customizations: []api.CustomField{{Name: "body", Type: "text"}}}}, ModuleOptions{})
	if !strings.Contains(got, "\nexport {}\n") {
		t.Fatalf("expected export {} so the augmentation is not an ambient module:\n%s", got)
	}
	if strings.Contains(got, "import type") {
		t.Fatalf("unexpected import:\n%s", got)
	}
	if !strings.HasPrefix(got, "// Nimbu channel types.\n") {
		t.Fatalf("unexpected header without site:\n%s", got)
	}
}

func TestInterfaceGolden(t *testing.T) {
	var posts api.ChannelDetail
	for _, channel := range loadFixture(t) {
		if channel.Slug == "blog-posts" {
			posts = channel
		}
	}
	assertGolden(t, "blog-posts.ts", Interface(posts)+"\n")
}

func loadFixture(t *testing.T) []api.ChannelDetail {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "channels.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var channels []api.ChannelDetail
	if err := json.Unmarshal(data, &channels); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return channels
}

func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run go test ./internal/channeltypes -update): %v", path, err)
	}
	if got != string(want) {
		t.Fatalf("golden mismatch for %s (run go test ./internal/channeltypes -update)\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func TestModuleStrictChannelsOnlyWhenRequested(t *testing.T) {
	channels := loadFixture(t)
	if got := Module(channels, ModuleOptions{}); strings.Contains(got, "NimbuCloudOptions") {
		t.Fatalf("partial module must not opt in to strict channels:\n%s", got)
	}
	if got := Module(channels, ModuleOptions{StrictChannels: true}); !strings.Contains(got, "  interface NimbuCloudOptions {\n    strictChannels: true\n  }\n}\n") {
		t.Fatalf("strict module lacks NimbuCloudOptions:\n%s", got)
	}
}
