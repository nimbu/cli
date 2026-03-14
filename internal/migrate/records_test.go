package migrate

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type failingEntropyReader struct{}

func (failingEntropyReader) Read(_ []byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func TestDownloadBinaryRejectsOversizedContentLength(t *testing.T) {
	client := api.New("https://example.test", "")
	client.HTTPClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode:    http.StatusOK,
				ContentLength: maxRecordAttachmentBytes + 1,
				Body:          io.NopCloser(strings.NewReader("x")),
				Header:        make(http.Header),
				Request:       req,
			}, nil
		}),
	}

	_, err := downloadBinary(context.Background(), client, "https://example.test/file.bin")
	if err == nil {
		t.Fatal("expected oversized attachment error")
	}
	if !strings.Contains(err.Error(), "attachment exceeds") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDownloadBinaryRejectsOversizedUnknownLength(t *testing.T) {
	body := bytes.NewReader(bytes.Repeat([]byte("a"), int(maxRecordAttachmentBytes)+1))
	client := api.New("https://example.test", "")
	client.HTTPClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode:    http.StatusOK,
				ContentLength: -1,
				Body:          io.NopCloser(body),
				Header:        make(http.Header),
				Request:       req,
			}, nil
		}),
	}

	_, err := downloadBinary(context.Background(), client, "https://example.test/file.bin")
	if err == nil {
		t.Fatal("expected oversized attachment error")
	}
	if !strings.Contains(err.Error(), "attachment exceeds") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCountActions(t *testing.T) {
	items := []RecordCopyItem{
		{Action: "create"},
		{Action: "update"},
		{Action: "skip"},
		{Action: "create"},
		{Action: "skip"},
		{Action: "skip"},
	}
	synced, skipped := countActions(items)
	if synced != 3 {
		t.Fatalf("expected 3 synced, got %d", synced)
	}
	if skipped != 3 {
		t.Fatalf("expected 3 skipped, got %d", skipped)
	}
}

func TestCountActionsEmpty(t *testing.T) {
	synced, skipped := countActions(nil)
	if synced != 0 || skipped != 0 {
		t.Fatalf("expected 0/0, got %d/%d", synced, skipped)
	}
}

func TestContentEqualIdentical(t *testing.T) {
	info := schemaInfo{resource: "articles"}
	source := map[string]any{"title": "Hello", "body": "World", "slug": "hello"}
	target := map[string]any{"title": "Hello", "body": "World", "slug": "hello"}
	if !contentEqual(source, target, info) {
		t.Fatal("expected content equal for identical entries")
	}
}

func TestContentEqualDiffers(t *testing.T) {
	info := schemaInfo{resource: "articles"}
	source := map[string]any{"title": "Hello", "body": "World"}
	target := map[string]any{"title": "Hello", "body": "Changed"}
	if contentEqual(source, target, info) {
		t.Fatal("expected content not equal for differing entries")
	}
}

func TestContentEqualSkipsSystemAndComplexFields(t *testing.T) {
	info := schemaInfo{
		resource:        "articles",
		fileFields:      []api.CustomField{{Name: "document", Type: "file"}},
		referenceFields: []api.CustomField{{Name: "author", Type: "belongs_to"}},
	}
	source := map[string]any{
		"id": "src-1", "created_at": "2024-01-01",
		"title": "Hello", "document": map[string]any{"id": "f1"}, "author": map[string]any{"id": "a1"},
	}
	target := map[string]any{
		"id": "tgt-1", "created_at": "2025-01-01",
		"title": "Hello", "document": map[string]any{"id": "f2"}, "author": map[string]any{"id": "a2"},
	}
	if !contentEqual(source, target, info) {
		t.Fatal("expected content equal when only system/complex fields differ")
	}
}

func TestPreMatchEntriesSkipIdentical(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/channels/articles/entries" {
			_, _ = w.Write([]byte(`[{"id":"tgt-1","slug":"hello","title":"Hello"}]`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	copier := &recordCopier{
		toClient: api.New(srv.URL, ""),
		mapping:  map[string]map[string]string{},
		result:   &RecordCopyResult{},
	}
	source := []map[string]any{
		{"id": "src-1", "slug": "hello", "title": "Hello"},
	}
	info := schemaInfo{resource: "articles"}
	copier.preMatchEntries(context.Background(), "articles", "articles", source, info)

	if id, ok := copier.mapping["articles"]["src-1"]; !ok || id != "tgt-1" {
		t.Fatalf("expected mapping src-1→tgt-1, got %v", copier.mapping)
	}
	if len(copier.result.Items) != 1 || copier.result.Items[0].Action != "skip" {
		t.Fatalf("expected skip item, got %v", copier.result.Items)
	}
}

func TestPreMatchEntriesContentDiffers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/channels/articles/entries" {
			_, _ = w.Write([]byte(`[{"id":"tgt-1","slug":"hello","title":"Old Title"}]`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	copier := &recordCopier{
		toClient: api.New(srv.URL, ""),
		mapping:  map[string]map[string]string{},
		result:   &RecordCopyResult{},
	}
	source := []map[string]any{
		{"id": "src-1", "slug": "hello", "title": "New Title"},
	}
	info := schemaInfo{resource: "articles"}
	copier.preMatchEntries(context.Background(), "articles", "articles", source, info)

	// Should NOT be in mapping (content differs)
	if _, ok := copier.mapping["articles"]["src-1"]; ok {
		t.Fatal("expected no mapping for content-different entry")
	}
	// Should be in preMatched
	if id, ok := copier.lookupPreMatched("articles", "src-1"); !ok || id != "tgt-1" {
		t.Fatalf("expected preMatched src-1→tgt-1, got %v", id)
	}
	if len(copier.result.Items) != 0 {
		t.Fatalf("expected no items for content-different entry, got %v", copier.result.Items)
	}
}

func TestRemapReferencesPlainID(t *testing.T) {
	copier := &recordCopier{
		mapping: map[string]map[string]string{
			"authors": {"src-author": "tgt-author"},
		},
	}
	info := schemaInfo{
		resource: "articles",
		referenceFields: []api.CustomField{
			{Name: "author", Type: "belongs_to", Reference: "authors"},
		},
	}
	payload := map[string]any{
		"author": "src-author",
	}
	copier.remapReferences(payload, info, "test-entry")

	if payload["author"] != "tgt-author" {
		t.Fatalf("expected author=tgt-author, got %v", payload["author"])
	}
}

func TestRemapReferencesManyPlainIDs(t *testing.T) {
	copier := &recordCopier{
		mapping: map[string]map[string]string{
			"tags": {"src-t1": "tgt-t1", "src-t2": "tgt-t2"},
		},
	}
	info := schemaInfo{
		resource: "articles",
		referenceFields: []api.CustomField{
			{Name: "tags", Type: "belongs_to_many", Reference: "tags"},
		},
	}
	payload := map[string]any{
		"tags": []any{"src-t1", "src-t2"},
	}
	copier.remapReferences(payload, info, "test-entry")

	tags, ok := payload["tags"].([]string)
	if !ok {
		t.Fatalf("expected tags to be []string, got %T: %v", payload["tags"], payload["tags"])
	}
	if len(tags) != 2 || tags[0] != "tgt-t1" || tags[1] != "tgt-t2" {
		t.Fatalf("expected [tgt-t1, tgt-t2], got %v", tags)
	}
}

func TestRemapReferencesRichFormat(t *testing.T) {
	copier := &recordCopier{
		mapping: map[string]map[string]string{
			"authors": {"src-author": "tgt-author"},
			"tags":    {"src-t1": "tgt-t1"},
		},
	}
	info := schemaInfo{
		resource: "articles",
		referenceFields: []api.CustomField{
			{Name: "author", Type: "belongs_to", Reference: "authors"},
			{Name: "tags", Type: "belongs_to_many", Reference: "tags"},
		},
	}
	payload := map[string]any{
		"author": map[string]any{"__type": "Reference", "className": "authors", "id": "src-author"},
		"tags": map[string]any{
			"__type":    "Relation",
			"className": "tags",
			"objects":   []any{map[string]any{"id": "src-t1"}},
		},
	}
	copier.remapReferences(payload, info, "test-entry")

	if payload["author"] != "tgt-author" {
		t.Fatalf("expected author=tgt-author, got %v", payload["author"])
	}
	tags, ok := payload["tags"].([]string)
	if !ok {
		t.Fatalf("expected tags []string, got %T", payload["tags"])
	}
	if len(tags) != 1 || tags[0] != "tgt-t1" {
		t.Fatalf("expected [tgt-t1], got %v", tags)
	}
}

func TestRemapSelfRefsPlainID(t *testing.T) {
	copier := &recordCopier{
		mapping: map[string]map[string]string{
			"articles": {"src-related": "tgt-related"},
		},
	}
	selfRefs := []api.CustomField{
		{Name: "related", Type: "belongs_to", Reference: "articles"},
	}
	payload := map[string]any{
		"related": "src-related",
	}
	copier.remapPendingSelfRefs(payload, selfRefs)

	if payload["related"] != "tgt-related" {
		t.Fatalf("expected related=tgt-related, got %v", payload["related"])
	}
}

func TestRemapDeferredFieldsPlainID(t *testing.T) {
	copier := &recordCopier{
		mapping: map[string]map[string]string{
			"categories": {"src-cat": "tgt-cat"},
		},
	}
	refFields := []api.CustomField{
		{Name: "category", Type: "belongs_to", Reference: "categories"},
	}
	payload := map[string]any{
		"category": "src-cat",
	}
	copier.remapDeferredFields("articles", "tgt-entry", payload, refFields)

	if payload["category"] != "tgt-cat" {
		t.Fatalf("expected category=tgt-cat, got %v", payload["category"])
	}
}

func TestSelfRefsNotCorruptedByRemapReferences(t *testing.T) {
	copier := &recordCopier{
		mapping: map[string]map[string]string{
			"articles": {"src-related": "tgt-related"},
		},
	}
	info := schemaInfo{
		resource: "articles",
		referenceFields: []api.CustomField{
			{Name: "related", Type: "belongs_to", Reference: "articles"},
		},
		selfRefs: []api.CustomField{
			{Name: "related", Type: "belongs_to", Reference: "articles"},
		},
	}
	payload := map[string]any{
		"title":   "Test",
		"related": "src-related",
	}

	// Extract self-refs first (as the fixed code does)
	selfFields := extractSelfRefs(payload, info.selfRefs)
	copier.remapReferences(payload, info, "test")

	// Self-ref should be extracted with original source ID
	if selfFields["related"] != "src-related" {
		t.Fatalf("expected original source ID, got %v", selfFields["related"])
	}

	// Now remap self-refs (second pass)
	copier.remapPendingSelfRefs(selfFields, info.selfRefs)

	if selfFields["related"] != "tgt-related" {
		t.Fatalf("expected target ID after remap, got %v", selfFields["related"])
	}
}

func TestCopyCustomersFailsWhenPasswordEntropyFails(t *testing.T) {
	originalReader := passwordRandReader
	passwordRandReader = failingEntropyReader{}
	defer func() {
		passwordRandReader = originalReader
	}()

	var writes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/customers/customizations":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && r.URL.Path == "/customers" && r.Header.Get("X-Nimbu-Site") == "source":
			_, _ = w.Write([]byte(`[{"id":"cust_1","email":"hello@example.com"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/customers" && r.Header.Get("X-Nimbu-Site") == "target":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/customers":
			writes++
			http.Error(w, "unexpected write", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	fromClient := api.New(srv.URL, "").WithSite("source")
	toClient := api.New(srv.URL, "").WithSite("target")

	_, err := CopyCustomers(
		context.Background(),
		fromClient,
		toClient,
		SiteRef{Site: "source"},
		SiteRef{Site: "target"},
		RecordCopyOptions{},
	)
	if err == nil {
		t.Fatal("expected password generation error")
	}
	if !strings.Contains(err.Error(), "generate customer password") {
		t.Fatalf("unexpected error: %v", err)
	}
	if writes != 0 {
		t.Fatalf("expected no customer writes, got %d", writes)
	}
}

func TestRefFieldEmpty(t *testing.T) {
	// nil is empty
	if !refFieldEmpty(nil) {
		t.Fatal("nil should be empty")
	}
	// belongs_to with no id is empty
	if !refFieldEmpty(map[string]any{"__type": "Reference", "className": "tags"}) {
		t.Fatal("reference with no id should be empty")
	}
	// belongs_to with id is NOT empty
	if refFieldEmpty(map[string]any{"__type": "Reference", "className": "tags", "id": "abc"}) {
		t.Fatal("reference with id should not be empty")
	}
	// belongs_to_many with empty objects is empty
	if !refFieldEmpty(map[string]any{"__type": "Relation", "objects": []any{}}) {
		t.Fatal("relation with empty objects should be empty")
	}
	// belongs_to_many with objects is NOT empty
	if refFieldEmpty(map[string]any{"__type": "Relation", "objects": []any{map[string]any{"id": "x"}}}) {
		t.Fatal("relation with objects should not be empty")
	}
	// empty array is empty
	if !refFieldEmpty([]any{}) {
		t.Fatal("empty array should be empty")
	}
}

func TestContentEqualDetectsBrokenReferences(t *testing.T) {
	info := schemaInfo{
		referenceFields: []api.CustomField{
			{Name: "author", Type: "belongs_to", Reference: "authors"},
			{Name: "tags", Type: "belongs_to_many", Reference: "tags"},
		},
	}
	// Plain ID format (what API returns)
	source := map[string]any{
		"title":  "Hello",
		"author": "src1",
		"tags":   []any{"t1", "t2"},
	}
	target := map[string]any{
		"title":  "Hello",
		"author": nil,
		"tags":   []any{},
	}
	if contentEqual(source, target, info) {
		t.Fatal("expected not equal when source has refs but target is empty")
	}
	// Both nil → equal
	sourceEmpty := map[string]any{"title": "Hello", "author": nil, "tags": []any{}}
	if !contentEqual(sourceEmpty, target, info) {
		t.Fatal("expected equal when both refs are empty")
	}
}

func TestContentEqualSkipsShortID(t *testing.T) {
	info := schemaInfo{}
	source := map[string]any{"title": "Hello", "short_id": "abc123"}
	target := map[string]any{"title": "Hello", "short_id": "xyz789"}
	if !contentEqual(source, target, info) {
		t.Fatal("expected equal — short_id should be skipped")
	}
}
