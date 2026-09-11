package pagepath

import (
	"errors"
	"testing"
)

func TestParse(t *testing.T) {
	title := Path{Field: "title", Raw: "/title"}
	index2 := Selector{Kind: SelIndex, Index: 2}
	idSel := Selector{Kind: SelID, ID: "000000000000000000000001"}
	slugSel := Selector{Kind: SelSlug, Slug: "hero_stage"}

	tests := []struct {
		name    string
		input   string
		want    Path
		wantErr string
	}{
		{name: "raw slash title", input: "/title", want: Path{Raw: "/title"}},
		{
			name:  "raw nested items path",
			input: "/items/Blokken/repeatables/000000000000000000000001/items/Title",
			want:  Path{Raw: "/items/Blokken/repeatables/000000000000000000000001/items/Title"},
		},
		{name: "page field title", input: "title", want: title},
		{name: "page field slug", input: "slug", want: Path{Field: "slug", Raw: "/slug"}},
		{name: "page field seo_title", input: "seo_title", want: Path{Field: "seo_title", Raw: "/seo_title"}},
		{name: "page field seo_description", input: "seo_description", want: Path{Field: "seo_description", Raw: "/seo_description"}},
		{name: "page field seo_keywords", input: "seo_keywords", want: Path{Field: "seo_keywords", Raw: "/seo_keywords"}},
		{name: "page field published", input: "published", want: Path{Field: "published", Raw: "/published"}},
		{name: "page field og_image", input: "og_image", want: Path{Field: "og_image", Raw: "/og_image"}},
		{name: "page field template", input: "template", want: Path{Field: "template", Raw: "/template"}},
		{name: "trimmed field", input: "  title  ", want: title},
		{name: "canvas only", input: "Blokken", want: Path{Segments: []Segment{{Name: "Blokken"}}}},
		{
			name:  "index selector",
			input: "Blokken[2]",
			want:  Path{Segments: []Segment{{Name: "Blokken", Selector: &index2}}},
		},
		{
			name:  "index then field",
			input: "Blokken[2].Title",
			want: Path{Segments: []Segment{
				{Name: "Blokken", Selector: &index2},
				{Name: "Title"},
			}},
		},
		{
			name:  "id selector",
			input: "Blokken[id=000000000000000000000001]",
			want:  Path{Segments: []Segment{{Name: "Blokken", Selector: &idSel}}},
		},
		{
			name:  "slug selector",
			input: "Blokken[slug=hero_stage]",
			want:  Path{Segments: []Segment{{Name: "Blokken", Selector: &slugSel}}},
		},
		{
			name:  "nested photos",
			input: "Blokken[id=000000000000000000000001].Photos[0].Image",
			want: Path{Segments: []Segment{
				{Name: "Blokken", Selector: &idSel},
				{Name: "Photos", Selector: &Selector{Kind: SelIndex, Index: 0}},
				{Name: "Image"},
			}},
		},
		{
			name:  "spaces dashes unquoted",
			input: "Left Button - Link",
			want:  Path{Segments: []Segment{{Name: "Left Button - Link"}}},
		},
		{
			name:  "en dash unquoted",
			input: "Korte uitleg \u2013 vraag",
			want:  Path{Segments: []Segment{{Name: "Korte uitleg \u2013 vraag"}}},
		},
		{
			name:  "parentheses unquoted",
			input: "Swoosh Bottom (hero)",
			want:  Path{Segments: []Segment{{Name: "Swoosh Bottom (hero)"}}},
		},
		{
			name:  "quoted name unnecessary spaces",
			input: `"Left Button - Link"`,
			want:  Path{Segments: []Segment{{Name: "Left Button - Link"}}},
		},
		{
			name:  "quoted name with dot",
			input: `"Foo.Bar".Title`,
			want: Path{Segments: []Segment{
				{Name: "Foo.Bar"},
				{Name: "Title"},
			}},
		},
		{
			name:  "quoted name with brackets",
			input: `"Foo[bar]"`,
			want:  Path{Segments: []Segment{{Name: "Foo[bar]"}}},
		},
		{
			name:  "escaped quote",
			input: `"Say \"hi\""`,
			want:  Path{Segments: []Segment{{Name: `Say "hi"`}}},
		},
		{
			name:  "slug containing dot inside brackets",
			input: "Blokken[slug=a.b]",
			want:  Path{Segments: []Segment{{Name: "Blokken", Selector: &Selector{Kind: SelSlug, Slug: "a.b"}}}},
		},
		{
			name:  "non hex id accepted at parse",
			input: "Blokken[id=abc]",
			want:  Path{Segments: []Segment{{Name: "Blokken", Selector: &Selector{Kind: SelID, ID: "abc"}}}},
		},
		{name: "empty", input: "", wantErr: `path "": empty path`},
		{name: "whitespace only", input: "   ", wantErr: `path "": empty path`},
		{name: "unbalanced quote", input: `"foo`, wantErr: `path "\"foo": unbalanced quotes`},
		{name: "unbalanced bracket", input: "Blokken[2", wantErr: `path "Blokken[2": unbalanced brackets`},
		{name: "bad selector", input: "Blokken[x]", wantErr: `path "Blokken[x]": invalid selector`},
		{name: "empty selector", input: "Blokken[]", wantErr: `path "Blokken[]": invalid selector`},
		{name: "empty id selector", input: "Blokken[id=]", wantErr: `path "Blokken[id=]": invalid selector`},
		{name: "empty slug selector", input: "Blokken[slug=]", wantErr: `path "Blokken[slug=]": invalid selector`},
		{name: "selector on page field", input: "title[0]", wantErr: `path "title[0]": selector not allowed on page field "title"`},
		{name: "negative index", input: "Blokken[-1]", wantErr: `path "Blokken[-1]": negative index`},
		{name: "trailing dot", input: "Blokken.", wantErr: `path "Blokken.": trailing '.'`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("Parse(%q) err=nil, want %q", tt.input, tt.wantErr)
				}
				var perr *ParseError
				if !errors.As(err, &perr) {
					t.Fatalf("Parse(%q) error type %T, want *ParseError", tt.input, err)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("Parse(%q) error = %q, want %q", tt.input, err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tt.input, err)
			}
			assertPathEqual(t, got, tt.want)
		})
	}
}

func TestParseRejectsBarePathWithoutLeadingSlashAsRaw(t *testing.T) {
	got, err := Parse("items/Blokken")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Raw != "" {
		t.Fatalf("bare items/... must not be treated as a raw API path, got Raw=%q", got.Raw)
	}
	if len(got.Segments) != 1 || got.Segments[0].Name != "items/Blokken" {
		t.Fatalf("expected a single unquoted segment, got %#v", got)
	}
}

func TestPathStringCanonical(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/title", "/title"},
		{"title", "title"},
		{"Blokken[2].Title", "Blokken[2].Title"},
		{`"Left Button - Link"`, "Left Button - Link"},
		{`"Foo.Bar"`, `"Foo.Bar"`},
		{`"Foo[bar]"`, `"Foo[bar]"`},
		{`"Say \"hi\""`, `"Say \"hi\""`},
		{"Blokken[id=000000000000000000000001].Photos[0].Image", "Blokken[id=000000000000000000000001].Photos[0].Image"},
		{"Blokken[slug=hero_stage]", "Blokken[slug=hero_stage]"},
		{"Korte uitleg \u2013 vraag", "Korte uitleg \u2013 vraag"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got := p.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPathRawForFields(t *testing.T) {
	p, err := Parse("title")
	if err != nil {
		t.Fatal(err)
	}
	if p.Raw != "/title" {
		t.Fatalf("Raw = %q, want /title", p.Raw)
	}
}

func assertPathEqual(t *testing.T, got, want Path) {
	t.Helper()
	if got.Raw != want.Raw || got.Field != want.Field || len(got.Segments) != len(want.Segments) {
		t.Fatalf("Path = %#v, want %#v", got, want)
	}
	for i := range want.Segments {
		gs, ws := got.Segments[i], want.Segments[i]
		if gs.Name != ws.Name {
			t.Fatalf("segment[%d].Name = %q, want %q", i, gs.Name, ws.Name)
		}
		if (gs.Selector == nil) != (ws.Selector == nil) {
			t.Fatalf("segment[%d].Selector = %#v, want %#v", i, gs.Selector, ws.Selector)
		}
		if gs.Selector != nil && *gs.Selector != *ws.Selector {
			t.Fatalf("segment[%d].Selector = %#v, want %#v", i, *gs.Selector, *ws.Selector)
		}
	}
}
