package output

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

func TestProjectLocaleRecursivelyOverlaysTranslationsWithoutMutatingSource(t *testing.T) {
	source := map[string]any{
		"title": "Default",
		"translations": map[string]any{
			"nl": map[string]any{"title": "Nederlands", "seo_title": "SEO NL"},
		},
		"items": []any{
			map[string]any{
				"label": "Default child",
				"translations": map[string]any{
					"nl": map[string]any{"label": "Nederlands kind"},
				},
			},
		},
	}

	got, err := ProjectLocale(source, "nl")
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]any{
		"title":     "Nederlands",
		"seo_title": "SEO NL",
		"translations": map[string]any{
			"nl": map[string]any{"title": "Nederlands", "seo_title": "SEO NL"},
		},
		"items": []any{
			map[string]any{
				"label": "Nederlands kind",
				"translations": map[string]any{
					"nl": map[string]any{"label": "Nederlands kind"},
				},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projection = %#v, want %#v", got, want)
	}
	if source["title"] != "Default" {
		t.Fatalf("source mutated: %#v", source)
	}
}

func TestProjectLocaleKeepsFallbackWhenTranslationIsAbsent(t *testing.T) {
	source := map[string]any{
		"title": "Default",
		"translations": map[string]any{
			"fr": map[string]any{"title": "Français"},
		},
	}

	got, err := ProjectLocale(source, "nl")
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["title"] != "Default" {
		t.Fatalf("projection = %#v", got)
	}
}

func TestProjectLocalePreservesLargeJSONIntegers(t *testing.T) {
	source := map[string]any{
		"id": json.Number("9007199254740993"),
		"translations": map[string]any{
			"nl": map[string]any{"title": "Nederlands"},
		},
	}

	got, err := ProjectLocale(source, "nl")
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["id"] != json.Number("9007199254740993") {
		t.Fatalf("large integer = %#v", got.(map[string]any)["id"])
	}

	var out, errOut bytes.Buffer
	ctx := testContextWithMode(&out, &errOut, Mode{Plain: true})
	if err := PlainFromSlice(ctx, []map[string]any{got.(map[string]any)}, []string{"id"}); err != nil {
		t.Fatal(err)
	}
	if out.String() != "9007199254740993\n" {
		t.Fatalf("plain output = %q", out.String())
	}
}

func TestProjectLocaleMatchesCanonicalEquivalentLocaleKey(t *testing.T) {
	source := map[string]any{
		"title": "Default",
		"translations": map[string]any{
			"nl_BE": map[string]any{"title": "Belgisch Nederlands"},
		},
	}

	got, err := ProjectLocale(source, "nl-BE")
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["title"] != "Belgisch Nederlands" {
		t.Fatalf("projection = %#v", got)
	}
}

func TestProjectLocaleDoesNotGuessRegionalLocaleFromBaseLanguage(t *testing.T) {
	source := map[string]any{
		"title": "Default",
		"translations": map[string]any{
			"nl-BE": map[string]any{"title": "Belgisch Nederlands"},
			"nl-NL": map[string]any{"title": "Nederlands Nederlands"},
		},
	}

	for i := 0; i < 20; i++ {
		got, err := ProjectLocale(source, "nl")
		if err != nil {
			t.Fatal(err)
		}
		if got.(map[string]any)["title"] != "Default" {
			t.Fatalf("projection guessed regional locale: %#v", got)
		}
	}
}
