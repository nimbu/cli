package cmd

import (
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
		"price":       float64(29.9),
		"published":   true,
		"meta":        map[string]any{"a": float64(1)},
		"title":       "hello",
		"attrs":       map[string]any{"color": "red"},
	}

	if !reflect.DeepEqual(body, want) {
		t.Fatalf("unexpected body:\n got: %#v\nwant: %#v", body, want)
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

	want := []string{"values.nl-be=Achternaam", "values.fr=Nom", "key=activate.label.lastname", "values.id=Nama"}
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
