package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseInlineAssignments(t *testing.T) {
	tmp := t.TempDir()
	jsonFile := filepath.Join(tmp, "meta.json")
	textFile := filepath.Join(tmp, "title.txt")
	if err := os.WriteFile(jsonFile, []byte(`{"a":1}`), 0o600); err != nil {
		t.Fatalf("write json file: %v", err)
	}
	if err := os.WriteFile(textFile, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write text file: %v", err)
	}

	body, err := parseInlineAssignments([]string{
		"name=Wine Box",
		"description=foo:=bar",
		"price:=29.9",
		"published:=true",
		"meta:=@" + jsonFile,
		"title=@" + textFile,
		"attrs.color=red",
	})
	if err != nil {
		t.Fatalf("parseInlineAssignments: %v", err)
	}

	want := map[string]any{
		"name":        "Wine Box",
		"description": "foo:=bar",
		"price":       json.Number("29.9"),
		"published":   true,
		"meta":        map[string]any{"a": json.Number("1")},
		"title":       "hello",
		"attrs":       map[string]any{"color": "red"},
	}

	if !reflect.DeepEqual(body, want) {
		t.Fatalf("unexpected body:\n got: %#v\nwant: %#v", body, want)
	}
}

func TestParseInlineAssignmentsPreservesExactJSONNumbers(t *testing.T) {
	body, err := parseInlineAssignments([]string{
		"large:=9007199254740993",
		"precise:=0.12345678901234567890",
	})
	if err != nil {
		t.Fatalf("parseInlineAssignments: %v", err)
	}
	if got := body["large"]; got != json.Number("9007199254740993") {
		t.Fatalf("large = %#v", got)
	}
	if got := body["precise"]; got != json.Number("0.12345678901234567890") {
		t.Fatalf("precise = %#v", got)
	}
}

func TestSplitInlineAssignmentUsesLeftMostOperator(t *testing.T) {
	path, op, rhs, err := splitInlineAssignment("description=foo:=bar")
	if err != nil {
		t.Fatalf("splitInlineAssignment: %v", err)
	}
	if path != "description" || op != "=" || rhs != "foo:=bar" {
		t.Fatalf("unexpected split: path=%q op=%q rhs=%q", path, op, rhs)
	}
}

func TestParseInlineAssignmentsConflicts(t *testing.T) {
	_, err := parseInlineAssignments([]string{"a=1", "a.b=2"})
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseInlineAssignmentsDuplicate(t *testing.T) {
	_, err := parseInlineAssignments([]string{"a=1", "a=2"})
	if err == nil {
		t.Fatal("expected duplicate error")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadJSONBodyInputModeConflict(t *testing.T) {
	_, err := readJSONBodyInput("payload.json", []string{"a=1"})
	if err == nil {
		t.Fatal("expected mode conflict error")
	}
	if !strings.Contains(err.Error(), "either --file or inline") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTranslationAssignmentsWithLocaleShorthand(t *testing.T) {
	got, err := translationAssignmentsWithLocaleShorthand([]string{"nl_BE=Achternaam", "values.fr=Nom", "key=activate.label.lastname", "id=Nama"})
	if err != nil {
		t.Fatalf("translationAssignmentsWithLocaleShorthand: %v", err)
	}

	want := []string{"values.nl-BE=Achternaam", "values.fr=Nom", "key=activate.label.lastname", "values.id=Nama"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected rewrite: %#v", got)
	}
}

func TestTranslationAssignmentsCanonicalizeScriptRegionAndLocaleValue(t *testing.T) {
	got, err := translationAssignmentsWithLocaleShorthand([]string{
		"values.zh_hant_tw=名稱",
		"locale= sr_latn_rs ",
	})
	if err != nil {
		t.Fatalf("translationAssignmentsWithLocaleShorthand: %v", err)
	}

	want := []string{"values.zh-Hant-TW=名稱", "locale=sr-Latn-RS"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected rewrite: %#v", got)
	}
}

func TestTranslationAssignmentsKeepExtensionAndPrivateUseSubtagsLowercase(t *testing.T) {
	got, err := translationAssignmentsWithLocaleShorthand([]string{
		"en_u_ca_gregory=Calendar",
		"en_x_us=Private",
	})
	if err != nil {
		t.Fatalf("translationAssignmentsWithLocaleShorthand: %v", err)
	}

	want := []string{"values.en-u-ca-gregory=Calendar", "values.en-x-us=Private"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected rewrite: %#v", got)
	}
}

func TestTranslationLocaleAssignmentCanonicalizesEveryOperator(t *testing.T) {
	rawPath := filepath.Join(t.TempDir(), "locale.txt")
	if err := os.WriteFile(rawPath, []byte(" nl_be "), 0o600); err != nil {
		t.Fatalf("write raw locale: %v", err)
	}
	jsonPath := filepath.Join(t.TempDir(), "locale.json")
	if err := os.WriteFile(jsonPath, []byte(`"zh_hant_tw"`), 0o600); err != nil {
		t.Fatalf("write JSON locale: %v", err)
	}

	got, err := translationAssignmentsWithLocaleShorthand([]string{
		"locale= nl_be ",
		`locale:="sr_latn_rs"`,
		"locale=@" + rawPath,
		"locale:=@" + jsonPath,
	})
	if err != nil {
		t.Fatalf("translationAssignmentsWithLocaleShorthand: %v", err)
	}

	want := []string{
		"locale=nl-BE",
		`locale:="sr-Latn-RS"`,
		"locale=nl-BE",
		`locale:="zh-Hant-TW"`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected rewrite:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestTranslationLocaleAssignmentRejectsNonStringJSON(t *testing.T) {
	jsonPath := filepath.Join(t.TempDir(), "locale.json")
	if err := os.WriteFile(jsonPath, []byte(`42`), 0o600); err != nil {
		t.Fatalf("write JSON locale: %v", err)
	}

	for _, assignment := range []string{`locale:=42`, "locale:=@" + jsonPath} {
		t.Run(assignment, func(t *testing.T) {
			_, err := translationAssignmentsWithLocaleShorthand([]string{assignment})
			if err == nil || !strings.Contains(err.Error(), "must be a string") {
				t.Fatalf("expected string validation error, got %v", err)
			}
		})
	}
}

func TestTranslationAssignmentsRejectIncompleteLocaleExtensions(t *testing.T) {
	for _, locale := range []string{"en-u", "en-x", "en-a-b"} {
		t.Run(locale, func(t *testing.T) {
			_, err := translationAssignmentsWithLocaleShorthand([]string{locale + "=value"})
			if err == nil || !strings.Contains(err.Error(), "invalid locale") {
				t.Fatalf("expected invalid locale error, got %v", err)
			}
		})
	}
}

func TestTranslationAssignmentsRejectDuplicateLocaleAfterCanonicalization(t *testing.T) {
	_, err := translationAssignmentsWithLocaleShorthand([]string{"nl_BE=Achternaam", "values.nl-BE=Naam"})
	if err == nil {
		t.Fatal("expected duplicate locale error")
	}
	if !strings.Contains(err.Error(), "duplicate locale") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTranslationAssignmentsCanonicalizeWholeValuesObjectKeys(t *testing.T) {
	got, err := translationAssignmentsWithLocaleShorthand([]string{
		`values:={"nl_BE":"Welkom","zh_hant_tw":"名稱"}`,
	})
	if err != nil {
		t.Fatalf("translationAssignmentsWithLocaleShorthand: %v", err)
	}

	want := []string{`values:={"nl-BE":"Welkom","zh-Hant-TW":"名稱"}`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected rewrite: %#v", got)
	}
}

func TestTranslationAssignmentsWithLocaleShorthandRejectsDuplicateLocale(t *testing.T) {
	_, err := translationAssignmentsWithLocaleShorthand([]string{"nl=Achternaam", "values.nl=Naam"})
	if err == nil {
		t.Fatal("expected duplicate locale error")
	}
	if !strings.Contains(err.Error(), "duplicate locale") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTranslationAssignmentsWithLocaleShorthandValidatesExplicitValuesLocale(t *testing.T) {
	_, err := translationAssignmentsWithLocaleShorthand([]string{"values.bad*=x"})
	if err == nil {
		t.Fatal("expected locale validation error")
	}
	if !strings.Contains(err.Error(), "invalid locale") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTranslationAssignmentsWithLocaleShorthandRejectsInvalidTopLevelKey(t *testing.T) {
	_, err := translationAssignmentsWithLocaleShorthand([]string{"title=Welkom"})
	if err == nil {
		t.Fatal("expected invalid locale key error")
	}
	if !strings.Contains(err.Error(), "invalid locale key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMergeJSONBodies(t *testing.T) {
	merged, err := mergeJSONBodies(
		map[string]any{"a": map[string]any{"x": 1}},
		map[string]any{"a": map[string]any{"y": 2}, "b": 3},
	)
	if err != nil {
		t.Fatalf("mergeJSONBodies: %v", err)
	}

	want := map[string]any{"a": map[string]any{"x": 1, "y": 2}, "b": 3}
	if !reflect.DeepEqual(merged, want) {
		t.Fatalf("unexpected merge: %#v", merged)
	}
}

func TestMergeJSONBodiesConflict(t *testing.T) {
	_, err := mergeJSONBodies(map[string]any{"a": 1}, map[string]any{"a": 2})
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if !strings.Contains(err.Error(), "conflicting value") {
		t.Fatalf("unexpected error: %v", err)
	}
}
