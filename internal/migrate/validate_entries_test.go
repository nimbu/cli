package migrate

import (
	"context"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
)

func TestExtractFileInfo(t *testing.T) {
	fi := extractFileInfo(map[string]any{
		"filename": "report.pdf",
		"checksum": "abc123",
		"url":      "https://cdn.example.com/report.pdf",
	})
	if fi.Filename != "report.pdf" {
		t.Fatalf("expected filename=report.pdf, got %q", fi.Filename)
	}
	if fi.Checksum != "abc123" {
		t.Fatalf("expected checksum=abc123, got %q", fi.Checksum)
	}
}

func TestExtractFileInfoNil(t *testing.T) {
	fi := extractFileInfo(nil)
	if fi.Filename != "" || fi.Checksum != "" {
		t.Fatalf("expected empty fileInfo, got %+v", fi)
	}
}

func TestCompareFileFieldMatch(t *testing.T) {
	source := map[string]any{"filename": "photo.jpg", "checksum": "abc"}
	target := map[string]any{"filename": "photo.jpg", "checksum": "abc"}
	if w := compareFileField("p", "img", source, target); w != "" {
		t.Fatalf("expected no warning, got %q", w)
	}
}

func TestCompareFileFieldChecksumMismatch(t *testing.T) {
	source := map[string]any{"filename": "photo.jpg", "checksum": "abc"}
	target := map[string]any{"filename": "photo.jpg", "checksum": "def"}
	w := compareFileField("p", "img", source, target)
	if !strings.Contains(w, "checksum mismatch") {
		t.Fatalf("expected checksum mismatch warning, got %q", w)
	}
}

func TestCompareFileFieldFilenameMismatch(t *testing.T) {
	source := map[string]any{"filename": "a.jpg", "checksum": "abc"}
	target := map[string]any{"filename": "b.jpg", "checksum": "abc"}
	w := compareFileField("p", "img", source, target)
	if !strings.Contains(w, "filename mismatch") {
		t.Fatalf("expected filename mismatch warning, got %q", w)
	}
}

func TestCompareFileFieldMissing(t *testing.T) {
	source := map[string]any{"filename": "a.jpg"}
	w := compareFileField("p", "img", source, nil)
	if !strings.Contains(w, "file missing on target") {
		t.Fatalf("expected missing warning, got %q", w)
	}
}

func TestExtractRefSlug(t *testing.T) {
	slug := extractRefSlug(map[string]any{
		"__type":    "Reference",
		"className": "authors",
		"id":        "507f",
		"slug":      "john-doe",
	})
	if slug != "john-doe" {
		t.Fatalf("expected john-doe, got %q", slug)
	}
}

func TestCompareRefFieldMatch(t *testing.T) {
	source := map[string]any{"slug": "john-doe"}
	target := map[string]any{"slug": "john-doe"}
	if w := compareRefField("p", "author", source, target); w != "" {
		t.Fatalf("expected no warning, got %q", w)
	}
}

func TestCompareRefFieldSlugMismatch(t *testing.T) {
	source := map[string]any{"slug": "john-doe"}
	target := map[string]any{"slug": "jane-doe"}
	w := compareRefField("p", "author", source, target)
	if !strings.Contains(w, "slug mismatch") {
		t.Fatalf("expected slug mismatch, got %q", w)
	}
}

func TestCompareRefFieldMissing(t *testing.T) {
	source := map[string]any{"slug": "john-doe"}
	w := compareRefField("p", "author", source, nil)
	if !strings.Contains(w, "reference missing on target") {
		t.Fatalf("expected missing warning, got %q", w)
	}
}

func TestExtractRelationSlugs(t *testing.T) {
	raw := map[string]any{
		"__type":    "Relation",
		"className": "tags",
		"objects": []any{
			map[string]any{"slug": "beta"},
			map[string]any{"slug": "alpha"},
		},
	}
	slugs := extractRelationSlugs(raw)
	if len(slugs) != 2 || slugs[0] != "alpha" || slugs[1] != "beta" {
		t.Fatalf("expected sorted [alpha, beta], got %v", slugs)
	}
}

func TestCompareRelationFieldSameOrderDifferent(t *testing.T) {
	// Same slugs, different order → no warning (sorted comparison)
	source := map[string]any{
		"objects": []any{
			map[string]any{"slug": "b"},
			map[string]any{"slug": "a"},
		},
	}
	target := map[string]any{
		"objects": []any{
			map[string]any{"slug": "a"},
			map[string]any{"slug": "b"},
		},
	}
	if w := compareRelationField("p", "tags", source, target); w != "" {
		t.Fatalf("expected no warning for reordered slugs, got %q", w)
	}
}

func TestCompareRelationFieldMismatch(t *testing.T) {
	source := map[string]any{
		"objects": []any{map[string]any{"slug": "a"}},
	}
	target := map[string]any{
		"objects": []any{map[string]any{"slug": "b"}},
	}
	w := compareRelationField("p", "tags", source, target)
	if !strings.Contains(w, "relation slugs mismatch") {
		t.Fatalf("expected mismatch warning, got %q", w)
	}
}

func TestNormalizeSelectValue(t *testing.T) {
	// Object form: {value: "draft"}
	if v := normalizeSelectValue(map[string]any{"value": "draft"}); v != "draft" {
		t.Fatalf("expected draft, got %v", v)
	}
	// Already plain string
	if v := normalizeSelectValue("published"); v != "published" {
		t.Fatalf("expected published, got %v", v)
	}
}

func TestNormalizeMultiSelectValues(t *testing.T) {
	// Object form: {values: [...]}
	vals := normalizeMultiSelectValues(map[string]any{
		"values": []any{"c", "a", "b"},
	})
	if len(vals) != 3 || vals[0] != "a" || vals[1] != "b" || vals[2] != "c" {
		t.Fatalf("expected sorted [a,b,c], got %v", vals)
	}

	// Array form
	vals = normalizeMultiSelectValues([]any{"z", "a"})
	if len(vals) != 2 || vals[0] != "a" || vals[1] != "z" {
		t.Fatalf("expected sorted [a,z], got %v", vals)
	}
}

func TestCompareScalarFieldMatch(t *testing.T) {
	if w := compareScalarField("p", "title", "Hello", "Hello"); w != "" {
		t.Fatalf("expected no warning, got %q", w)
	}
}

func TestCompareScalarFieldTrimMatch(t *testing.T) {
	if w := compareScalarField("p", "title", "Hello ", " Hello"); w != "" {
		t.Fatalf("expected no warning for trimmed match, got %q", w)
	}
}

func TestCompareScalarFieldMismatch(t *testing.T) {
	w := compareScalarField("p", "title", "Hello", "World")
	if !strings.Contains(w, "value mismatch") {
		t.Fatalf("expected mismatch, got %q", w)
	}
}

func TestExtractGalleryImages(t *testing.T) {
	raw := []any{
		map[string]any{
			"position": float64(1),
			"caption":  "Second",
			"file":     map[string]any{"filename": "b.jpg", "checksum": "bbb"},
		},
		map[string]any{
			"position": float64(0),
			"caption":  "First",
			"file":     map[string]any{"filename": "a.jpg", "checksum": "aaa"},
		},
	}
	images := extractGalleryImages(raw)
	if len(images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(images))
	}
	// Sorted by position
	if images[0].Caption != "First" || images[1].Caption != "Second" {
		t.Fatalf("expected sorted by position, got %+v", images)
	}
	if images[0].File.Checksum != "aaa" {
		t.Fatalf("expected checksum aaa, got %q", images[0].File.Checksum)
	}
}

func TestCompareGalleryFieldMatch(t *testing.T) {
	gallery := []any{
		map[string]any{
			"position": float64(0),
			"caption":  "Hero",
			"file":     map[string]any{"filename": "hero.jpg", "checksum": "abc"},
		},
	}
	if w := compareGalleryField("p", "photos", gallery, gallery); w != "" {
		t.Fatalf("expected no warning, got %q", w)
	}
}

func TestCompareGalleryFieldCountMismatch(t *testing.T) {
	source := []any{
		map[string]any{"position": float64(0), "file": map[string]any{"filename": "a.jpg"}},
		map[string]any{"position": float64(1), "file": map[string]any{"filename": "b.jpg"}},
	}
	target := []any{
		map[string]any{"position": float64(0), "file": map[string]any{"filename": "a.jpg"}},
	}
	w := compareGalleryField("p", "photos", source, target)
	if !strings.Contains(w, "image count mismatch") {
		t.Fatalf("expected count mismatch, got %q", w)
	}
}

func TestValidateEntrySkipsSystemFields(t *testing.T) {
	source := map[string]any{
		"id":         "source-1",
		"_id":        "source-internal",
		"created_at": "2024-01-01",
		"updated_at": "2024-01-02",
		"title":      "Hello",
	}
	target := map[string]any{
		"id":         "target-1",
		"_id":        "target-internal",
		"created_at": "2025-01-01",
		"updated_at": "2025-01-02",
		"title":      "Hello",
	}
	info := schemaInfo{resource: "articles"}
	warnings := validateEntry("articles", "hello", source, target, info)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings (system fields skipped), got %v", warnings)
	}
}

func TestValidateChannelMissingTarget(t *testing.T) {
	idMap := map[string]string{"src-1": "tgt-1"}
	sourceIndex := map[string]map[string]any{
		"src-1": {"id": "src-1", "title": "Post"},
	}
	targetIndex := map[string]map[string]any{} // empty — target missing

	warnings := validateChannel("articles", idMap, sourceIndex, targetIndex, schemaInfo{resource: "articles"})
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "missing on target") {
		t.Fatalf("expected 'missing on target', got %q", warnings[0])
	}
}

func TestValidateChannelAllMatch(t *testing.T) {
	idMap := map[string]string{"src-1": "tgt-1"}
	entry := map[string]any{
		"title":     "Hello",
		"body":      "World",
		"published": true,
	}
	sourceIndex := map[string]map[string]any{"src-1": entry}
	targetIndex := map[string]map[string]any{"tgt-1": entry}

	warnings := validateChannel("articles", idMap, sourceIndex, targetIndex, schemaInfo{resource: "articles"})
	if len(warnings) != 0 {
		t.Fatalf("expected 0 warnings, got %v", warnings)
	}
}

func TestValidateLocalizedEntryReportsMismatch(t *testing.T) {
	info := schemaInfo{
		resource:        "project_approaches",
		localizedFields: []api.CustomField{{Name: "title", Type: "text", Localized: true}},
	}
	warnings := validateLocalizedEntry(
		"project_approaches",
		"start",
		"en",
		map[string]any{"title": "Exploration"},
		map[string]any{"title": "Exploratie"},
		info,
	)
	if len(warnings) != 1 {
		t.Fatalf("expected one warning, got %v", warnings)
	}
	want := `channel=project_approaches entry=start locale=en field=title: mismatch (source="Exploration", target="Exploratie")`
	if warnings[0] != want {
		t.Fatalf("unexpected warning:\nwant %q\n got %q", want, warnings[0])
	}
}

func TestValidateLocalizedEntryPassesWhenValuesMatch(t *testing.T) {
	info := schemaInfo{
		resource:        "project_approaches",
		localizedFields: []api.CustomField{{Name: "title", Type: "text", Localized: true}},
	}
	warnings := validateLocalizedEntry(
		"project_approaches",
		"start",
		"en",
		map[string]any{"title": "Exploration"},
		map[string]any{"title": "Exploration"},
		info,
	)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
}

func TestValidateEntryFileChecksumMismatch(t *testing.T) {
	info := schemaInfo{
		resource:   "articles",
		fileFields: []api.CustomField{{Name: "document", Type: "file"}},
	}
	source := map[string]any{
		"title":    "Post",
		"document": map[string]any{"filename": "doc.pdf", "checksum": "aaa"},
	}
	target := map[string]any{
		"title":    "Post",
		"document": map[string]any{"filename": "doc.pdf", "checksum": "bbb"},
	}
	warnings := validateEntry("articles", "post", source, target, info)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", warnings)
	}
}

func TestValidateEntryRefSlugMismatch(t *testing.T) {
	info := schemaInfo{
		resource: "articles",
		referenceFields: []api.CustomField{
			{Name: "author", Type: "belongs_to", Reference: "authors"},
		},
	}
	source := map[string]any{
		"title":  "Post",
		"author": map[string]any{"slug": "john-doe", "id": "s1"},
	}
	target := map[string]any{
		"title":  "Post",
		"author": map[string]any{"slug": "jane-doe", "id": "t1"},
	}
	warnings := validateEntry("articles", "post", source, target, info)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "slug mismatch") {
		t.Fatalf("expected slug mismatch, got %v", warnings)
	}
}

func TestBuildFieldTypeMap(t *testing.T) {
	info := schemaInfo{
		resource:        "articles",
		fileFields:      []api.CustomField{{Name: "document", Type: "file"}},
		galleryFields:   []api.CustomField{{Name: "photos", Type: "gallery"}},
		referenceFields: []api.CustomField{{Name: "author", Type: "belongs_to"}},
		selectFields:    []api.CustomField{{Name: "status", Type: "select"}},
		multiFields:     []api.CustomField{{Name: "tags", Type: "multi_select"}},
	}
	m := buildFieldTypeMap(info)
	if m["document"] != "file" {
		t.Fatalf("expected file, got %q", m["document"])
	}
	if m["photos"] != "gallery" {
		t.Fatalf("expected gallery, got %q", m["photos"])
	}
	if m["author"] != "belongs_to" {
		t.Fatalf("expected belongs_to, got %q", m["author"])
	}
	if m["status"] != "select" {
		t.Fatalf("expected select, got %q", m["status"])
	}
	if m["tags"] != "multi_select" {
		t.Fatalf("expected multi_select, got %q", m["tags"])
	}
}

func TestSortedKeys(t *testing.T) {
	m := map[string]map[string]string{
		"c": {}, "a": {}, "b": {},
	}
	keys := sortedKeys(m)
	if len(keys) != 3 || keys[0] != "a" || keys[1] != "b" || keys[2] != "c" {
		t.Fatalf("expected [a,b,c], got %v", keys)
	}
}

func TestValidateEntriesEmptyMapping(t *testing.T) {
	warnings := ValidateEntries(context.Background(), nil, nil, map[string]map[string]string{}, nil)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings for empty mapping, got %v", warnings)
	}
}
