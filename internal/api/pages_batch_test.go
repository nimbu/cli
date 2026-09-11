package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBatchOperationAfterJSON(t *testing.T) {
	tests := []struct {
		name string
		op   BatchOperation
		want string
	}{
		{
			name: "after omitted",
			op:   BatchOperation{Op: "insert", Path: "/items/Blokken/repeatables"},
			want: `{"op":"insert","path":"/items/Blokken/repeatables"}`,
		},
		{
			name: "after explicit null",
			op: BatchOperation{
				Op:    "insert",
				Path:  "/items/Blokken/repeatables",
				After: &Anchor{Set: true},
			},
			want: `{"op":"insert","path":"/items/Blokken/repeatables","after":null}`,
		},
		{
			name: "after id",
			op: BatchOperation{
				Op:    "move",
				Path:  "/items/Blokken/repeatables/000000000000000000000002",
				After: &Anchor{Set: true, ID: "000000000000000000000001"},
			},
			want: `{"op":"move","path":"/items/Blokken/repeatables/000000000000000000000002","after":"000000000000000000000001"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.op)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Fatalf("json = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestPageETag(t *testing.T) {
	got, err := PageETag("6a6c60362e4510dd4a459982", "2026-09-10T14:31:12.408Z")
	if err != nil {
		t.Fatal(err)
	}
	const want = "5935a7315c082984b0475ec6aea5e618"
	if got != want {
		t.Fatalf("etag = %q, want %q", got, want)
	}

	whole, err := PageETag("6a6c60362e4510dd4a459982", "2026-09-10T14:31:12Z")
	if err != nil {
		t.Fatal(err)
	}
	const wantWhole = "55e608bcda11be4751a5c436415354b9"
	if whole != wantWhole {
		t.Fatalf("whole-second etag = %q, want %q", whole, wantWhole)
	}
}

func TestPostPageBatch(t *testing.T) {
	var (
		method  string
		path    string
		rawQ    string
		ifMatch string
		body    []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		rawQ = r.URL.RawQuery
		ifMatch = r.Header.Get("If-Match")
		body, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{
			"results":[{"index":0,"status":"ok","path":"/title","id":"rep1","warning":"coerced"}],
			"etag":"newetag",
			"updated_at":"2026-09-10T14:31:12.408Z",
			"page":{"id":"pg1","title":"Hi"}
		}`))
	}))
	defer srv.Close()

	ops := []BatchOperation{
		{Op: "set", Path: "/title", Value: "Hi"},
		{Op: "insert", Path: "/items/Blokken/repeatables", Value: map[string]any{"slug": "hero_stage"}, After: &Anchor{Set: true}},
	}
	got, err := New(srv.URL, "tok").PostPageBatch(context.Background(), "pg1", ops, BatchOptions{
		Atomic:        true,
		IncludeResult: true,
		ContentLocale: "en",
		IfMatch:       "abc123",
	})
	if err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPost {
		t.Fatalf("method = %s", method)
	}
	if path != "/pages/pg1/batch" {
		t.Fatalf("path = %s", path)
	}
	if rawQ != "atomic=1&content_locale=en&include=result" {
		t.Fatalf("query = %q", rawQ)
	}
	if ifMatch != `"abc123"` {
		t.Fatalf("If-Match = %q, want quoted etag", ifMatch)
	}

	const wantBody = `{"operations":[{"op":"set","path":"/title","value":"Hi"},{"op":"insert","path":"/items/Blokken/repeatables","value":{"slug":"hero_stage"},"after":null}]}`
	if string(body) != wantBody {
		t.Fatalf("body = %s, want %s", body, wantBody)
	}

	if got.ETag != "newetag" || got.UpdatedAt != "2026-09-10T14:31:12.408Z" {
		t.Fatalf("result meta = %#v", got)
	}
	if len(got.Results) != 1 || got.Results[0].ID != "rep1" || got.Results[0].Warning != "coerced" {
		t.Fatalf("results = %#v", got.Results)
	}
	var page map[string]any
	if err := json.Unmarshal(got.Page, &page); err != nil || page["id"] != "pg1" {
		t.Fatalf("page = %s", got.Page)
	}
}

func TestPostPageBatchPreconditionFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusPreconditionFailed)
		_, _ = w.Write([]byte(`{"message":"Precondition Failed","code":"precondition_failed","current_etag":"deadbeef"}`))
	}))
	defer srv.Close()

	_, err := New(srv.URL, "").PostPageBatch(context.Background(), "pg1", []BatchOperation{{Op: "set", Path: "/title", Value: "x"}}, BatchOptions{IfMatch: "stale"})
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err type %T: %v", err, err)
	}
	if apiErr.StatusCode != 412 || apiErr.Code != "precondition_failed" {
		t.Fatalf("status/code = %d %q", apiErr.StatusCode, apiErr.Code)
	}
	if apiErr.CurrentETag() != "deadbeef" {
		t.Fatalf("CurrentETag = %q", apiErr.CurrentETag())
	}
}

func TestPostPageBatchAtomicFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{
			"message":"Atomic batch failed",
			"code":"atomic_failure",
			"results":[
				{"index":0,"status":"ok","path":"/title"},
				{"index":1,"status":"error","path":"/items/X","error":{"code":"invalid","message":"nope"}}
			]
		}`))
	}))
	defer srv.Close()

	_, err := New(srv.URL, "").PostPageBatch(context.Background(), "pg1", []BatchOperation{
		{Op: "set", Path: "/title", Value: "x"},
		{Op: "set", Path: "/items/X", Value: "y"},
	}, BatchOptions{Atomic: true, IfMatch: "etag"})
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err type %T: %v", err, err)
	}
	if apiErr.StatusCode != 422 || apiErr.Code != "atomic_failure" {
		t.Fatalf("status/code = %d %q", apiErr.StatusCode, apiErr.Code)
	}
	results := apiErr.BatchResults()
	if len(results) != 2 || results[1].Error == nil || results[1].Error.Code != "invalid" {
		t.Fatalf("BatchResults = %#v", results)
	}
}

func TestGetPageSchema(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"template":{"id":"t1","name":"home"},"available_blocks":{"Blokken":[{"slug":"hero_stage","label":"Hero"}]},"select_options":{}}`))
	}))
	defer srv.Close()

	schema, err := New(srv.URL, "").GetPageSchema(context.Background(), "pg1")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/pages/pg1/schema" {
		t.Fatalf("path = %s", path)
	}
	if schema.Template.Name != "home" {
		t.Fatalf("schema = %#v", schema)
	}
	if slugs := schema.BlockSlugs("Blokken"); len(slugs) != 1 || slugs[0] != "hero_stage" {
		t.Fatalf("slugs = %v", slugs)
	}
}

func TestGetPageItems(t *testing.T) {
	var path string
	rawBody := `{"path":"/items/Blokken/repeatables/000000000000000000000001","parent_path":"/items/Blokken","position":1,"siblings_count":5,"type":"repeatable","data":{"slug":"hero_stage"},"extra":true}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(rawBody))
	}))
	defer srv.Close()

	got, err := New(srv.URL, "").GetPageItems(context.Background(), "pg1", "/items/Blokken/repeatables/000000000000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/pages/pg1/items/Blokken/repeatables/000000000000000000000001" {
		t.Fatalf("path = %s", path)
	}
	if got.Type != "repeatable" || got.Position != 1 || got.SiblingsCount != 5 {
		t.Fatalf("subtree = %#v", got)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var round, want any
	if err := json.Unmarshal(encoded, &round); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(rawBody), &want); err != nil {
		t.Fatal(err)
	}
	if !JSONEqual(round, want) {
		t.Fatalf("lossless json failed: %s", encoded)
	}
}

func TestPostPageBatchDoesNotDoubleQuoteIfMatch(t *testing.T) {
	var ifMatch string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ifMatch = r.Header.Get("If-Match")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	_, err := New(srv.URL, "").PostPageBatch(context.Background(), "pg1", nil, BatchOptions{IfMatch: `"already"`})
	if err != nil {
		t.Fatal(err)
	}
	if ifMatch != `"already"` {
		t.Fatalf("If-Match = %q", ifMatch)
	}
}
