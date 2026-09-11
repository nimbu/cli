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

func TestGetPageDraft(t *testing.T) {
	var path, rawQ string
	raw := `{
		"id":"draft1",
		"page_id":"pg1",
		"future_page_id":"pg1",
		"reserved_fullpath":"about",
		"content":{"title":"Draft Title","page_items":[{"slug":"Blokken","type":"canvas"}]},
		"updated_at":"2026-09-10T15:00:00.000Z",
		"extra":true
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		rawQ = r.URL.RawQuery
		_, _ = w.Write([]byte(raw))
	}))
	defer srv.Close()

	got, err := New(srv.URL, "tok").GetPageDraft(context.Background(), "pg1", DraftOptions{ContentLocale: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/pages/pg1/draft" {
		t.Fatalf("path = %s", path)
	}
	if rawQ != "content_locale=en" {
		t.Fatalf("query = %q", rawQ)
	}
	if got.ID != "draft1" || got.PageID != "pg1" || got.ReservedFullpath != "about" {
		t.Fatalf("draft = %#v", got)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var round, want any
	if err := json.Unmarshal(encoded, &round); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(raw), &want); err != nil {
		t.Fatal(err)
	}
	if !JSONEqual(round, want) {
		t.Fatalf("lossless json failed: %s", encoded)
	}
}

func TestGetPageDraftNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found","code":101}`))
	}))
	defer srv.Close()

	_, err := New(srv.URL, "").GetPageDraft(context.Background(), "pg1", DraftOptions{})
	var apiErr *Error
	if !errors.As(err, &apiErr) || !apiErr.IsNotFound() {
		t.Fatalf("err = %v", err)
	}
	if apiErr.Message != "Not Found" || apiErr.Code != "101" {
		t.Fatalf("status/code = %d %q %q", apiErr.StatusCode, apiErr.Code, apiErr.Message)
	}
}

func TestPostPageDraft(t *testing.T) {
	var method, path, rawQ string
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		rawQ = r.URL.RawQuery
		body, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"id":"draft1","page_id":"pg1","content":{"title":"Saved"},"updated_at":"2026-09-10T15:00:00.000Z"}`))
	}))
	defer srv.Close()

	got, err := New(srv.URL, "").PostPageDraft(context.Background(), "pg1", map[string]any{"title": "Saved"}, DraftOptions{ContentLocale: "nl"})
	if err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPost || path != "/pages/pg1/draft" {
		t.Fatalf("%s %s", method, path)
	}
	if rawQ != "content_locale=nl" {
		t.Fatalf("query = %q", rawQ)
	}
	if string(body) != `{"title":"Saved"}` {
		t.Fatalf("body = %s", body)
	}
	if got.ID != "draft1" {
		t.Fatalf("draft = %#v", got)
	}
}

func TestPostPageDraftBatch(t *testing.T) {
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
			"results":[{"index":0,"status":"ok","path":"/title"}],
			"draft":{"id":"draft1","page_id":"pg1","updated_at":"2026-09-10T15:00:00.000Z","content":{"title":"Hi"}}
		}`))
	}))
	defer srv.Close()

	got, err := New(srv.URL, "").PostPageDraftBatch(context.Background(), "pg1", []BatchOperation{
		{Op: "set", Path: "/title", Value: "Hi"},
	}, DraftBatchOptions{ContentLocale: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPost {
		t.Fatalf("method = %s", method)
	}
	if path != "/pages/pg1/draft/batch" {
		t.Fatalf("path = %s", path)
	}
	if rawQ != "content_locale=en" {
		t.Fatalf("query = %q", rawQ)
	}
	if ifMatch != "" {
		t.Fatalf("If-Match = %q, want empty", ifMatch)
	}
	if string(body) != `{"operations":[{"op":"set","path":"/title","value":"Hi"}]}` {
		t.Fatalf("body = %s", body)
	}
	if len(got.Results) != 1 || got.Results[0].Status != "ok" {
		t.Fatalf("results = %#v", got.Results)
	}
	var draft map[string]any
	if err := json.Unmarshal(got.Draft, &draft); err != nil || draft["id"] != "draft1" {
		t.Fatalf("draft = %s", got.Draft)
	}
}

func TestPostPageDraftBatchDoesNotSendAtomicOrInclude(t *testing.T) {
	var rawQ string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQ = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"results":[],"draft":{}}`))
	}))
	defer srv.Close()

	_, err := New(srv.URL, "").PostPageDraftBatch(context.Background(), "pg1", nil, DraftBatchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if rawQ != "" {
		t.Fatalf("query = %q, want empty (no atomic/include)", rawQ)
	}
}

func TestPublishPageDraft(t *testing.T) {
	var body []byte
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		body, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"id":"pg1","fullpath":"about","updated_at":"2026-09-10T16:00:00.000Z","title":"Live"}`))
	}))
	defer srv.Close()

	page, err := New(srv.URL, "").PublishPageDraft(context.Background(), "pg1", true)
	if err != nil {
		t.Fatal(err)
	}
	if path != "/pages/pg1/draft/publish" {
		t.Fatalf("path = %s", path)
	}
	if string(body) != `{"confirm":true}` {
		t.Fatalf("body = %s", body)
	}
	if page["id"] != "pg1" || page["fullpath"] != "about" {
		t.Fatalf("page = %#v", page)
	}
}

func TestPublishPageDraftConflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":"draft_base_changed","message":"Live page changed after this draft was based on it; confirm publish to replace live content."}`))
	}))
	defer srv.Close()

	_, err := New(srv.URL, "").PublishPageDraft(context.Background(), "pg1", false)
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err type %T: %v", err, err)
	}
	if apiErr.StatusCode != 409 || apiErr.Code != "draft_base_changed" {
		t.Fatalf("status/code = %d %q", apiErr.StatusCode, apiErr.Code)
	}
}

func TestDeletePageDraft(t *testing.T) {
	var method, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := New(srv.URL, "").DeletePageDraft(context.Background(), "pg1"); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodDelete || path != "/pages/pg1/draft" {
		t.Fatalf("%s %s", method, path)
	}
}

func TestCreatePageDraftPreviewToken(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"token":"jwt-token","preview_url":"/zz-cli-test/surgical?preview=jwt-token"}`))
	}))
	defer srv.Close()

	got, err := New(srv.URL, "").CreatePageDraftPreviewToken(context.Background(), "pg1")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/pages/pg1/draft/preview_token" {
		t.Fatalf("path = %s", path)
	}
	if got.Token != "jwt-token" || got.PreviewURL != "/zz-cli-test/surgical?preview=jwt-token" {
		t.Fatalf("token = %#v", got)
	}
}

func TestGetPageDraftForbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Page drafts are not enabled"}`))
	}))
	defer srv.Close()

	_, err := New(srv.URL, "").GetPageDraft(context.Background(), "pg1", DraftOptions{})
	var apiErr *Error
	if !errors.As(err, &apiErr) || !apiErr.IsForbidden() {
		t.Fatalf("err = %v", err)
	}
	if apiErr.Message != "Page drafts are not enabled" {
		t.Fatalf("message = %q", apiErr.Message)
	}
}
