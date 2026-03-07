package api

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizePageFullpath(t *testing.T) {
	if got := NormalizePageFullpath(" /about/team "); got != "about/team" {
		t.Fatalf("unexpected fullpath normalization: %q", got)
	}
}

func TestPageStatsCountsNestedEditablesAndAttachments(t *testing.T) {
	doc := PageDocument{
		"items": map[string]any{
			"hero": map[string]any{
				"file": map[string]any{"url": "https://example.test/a.jpg"},
			},
			"gallery": map[string]any{
				"repeatables": []any{
					map[string]any{
						"items": map[string]any{
							"photo": map[string]any{
								"file": map[string]any{"attachment_path": "/tmp/image.jpg"},
							},
						},
					},
				},
			},
		},
	}

	stats := PageStats(doc)
	if stats.EditableCount != 3 {
		t.Fatalf("expected 3 editables, got %d", stats.EditableCount)
	}
	if stats.AttachmentCount != 2 {
		t.Fatalf("expected 2 attachments, got %d", stats.AttachmentCount)
	}
}

func TestExpandPageAttachmentPaths(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "hero.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	doc := PageDocument{
		"items": map[string]any{
			"hero": map[string]any{
				"file": map[string]any{
					"attachment_path": filePath,
					"url":             "https://example.test/old",
				},
			},
		},
	}

	if err := ExpandPageAttachmentPaths(doc); err != nil {
		t.Fatalf("expand attachment_path: %v", err)
	}

	file := doc["items"].(map[string]any)["hero"].(map[string]any)["file"].(map[string]any)
	if file["attachment"] != base64.StdEncoding.EncodeToString([]byte("hello")) {
		t.Fatalf("unexpected attachment payload: %#v", file["attachment"])
	}
	if file["filename"] != "hero.txt" {
		t.Fatalf("unexpected filename: %#v", file["filename"])
	}
	if _, ok := file["url"]; ok {
		t.Fatalf("url should be removed, got %#v", file["url"])
	}
	if _, ok := file["attachment_path"]; ok {
		t.Fatalf("attachment_path should be removed")
	}
}

func TestDownloadPageAssets(t *testing.T) {
	assetBase := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pages/about":
			_, _ = w.Write([]byte(`{"id":"p1","fullpath":"about","items":{"hero":{"file":{"url":"` + assetBase + `/assets/hero.jpg","filename":"hero.jpg"}}}}`))
		case "/assets/hero.jpg":
			_, _ = w.Write([]byte("asset-bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	assetBase = srv.URL

	doc, err := GetPageDocument(context.Background(), New(srv.URL, ""), "about")
	if err != nil {
		t.Fatalf("get page document: %v", err)
	}

	dir := t.TempDir()
	count, err := DownloadPageAssets(context.Background(), New(srv.URL, ""), doc, dir)
	if err != nil {
		t.Fatalf("download assets: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 download, got %d", count)
	}

	file := doc["items"].(map[string]any)["hero"].(map[string]any)["file"].(map[string]any)
	attachmentPath, ok := file["attachment_path"].(string)
	if !ok || attachmentPath == "" {
		t.Fatalf("expected attachment_path in file object, got %#v", file)
	}
	data, err := os.ReadFile(attachmentPath)
	if err != nil {
		t.Fatalf("read downloaded asset: %v", err)
	}
	if strings.TrimSpace(string(data)) != "asset-bytes" {
		t.Fatalf("unexpected asset contents: %q", string(data))
	}
	if _, ok := file["url"]; ok {
		t.Fatalf("url should be removed after download")
	}
}

func TestDownloadPageAssetsKeepsDistinctAttachmentPaths(t *testing.T) {
	assetBase := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/assets/hero-a":
			_, _ = w.Write([]byte("first"))
		case "/assets/hero-b":
			_, _ = w.Write([]byte("second"))
		case "/assets/hero-c":
			_, _ = w.Write([]byte("third"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	assetBase = srv.URL

	doc := PageDocument{
		"items": map[string]any{
			"hero": map[string]any{
				"file": map[string]any{
					"url":      assetBase + "/assets/hero-a",
					"filename": "hero.jpg",
				},
			},
			"gallery": map[string]any{
				"repeatables": []any{
					map[string]any{
						"items": map[string]any{
							"image_one": map[string]any{
								"file": map[string]any{
									"url":      assetBase + "/assets/hero-b",
									"filename": "hero.jpg",
								},
							},
							"image_two": map[string]any{
								"file": map[string]any{
									"url":      assetBase + "/assets/hero-c",
									"filename": "hero-2.jpg",
								},
							},
						},
					},
				},
			},
		},
	}

	dir := t.TempDir()
	count, err := DownloadPageAssets(context.Background(), New(srv.URL, ""), doc, dir)
	if err != nil {
		t.Fatalf("download assets: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 downloads, got %d", count)
	}

	files := []map[string]any{
		doc["items"].(map[string]any)["hero"].(map[string]any)["file"].(map[string]any),
		doc["items"].(map[string]any)["gallery"].(map[string]any)["repeatables"].([]any)[0].(map[string]any)["items"].(map[string]any)["image_one"].(map[string]any)["file"].(map[string]any),
		doc["items"].(map[string]any)["gallery"].(map[string]any)["repeatables"].([]any)[0].(map[string]any)["items"].(map[string]any)["image_two"].(map[string]any)["file"].(map[string]any),
	}

	seenPaths := map[string]string{}
	seenContents := map[string]struct{}{}
	for _, file := range files {
		attachmentPath, ok := file["attachment_path"].(string)
		if !ok || attachmentPath == "" {
			t.Fatalf("expected attachment_path in file object, got %#v", file)
		}
		base := filepath.Base(attachmentPath)
		if prev, exists := seenPaths[base]; exists {
			t.Fatalf("attachment basename collision: %s and %s both used %s", prev, attachmentPath, base)
		}
		data, err := os.ReadFile(attachmentPath)
		if err != nil {
			t.Fatalf("read downloaded asset %q: %v", attachmentPath, err)
		}
		seenPaths[base] = attachmentPath
		seenContents[string(data)] = struct{}{}
	}

	if len(seenPaths) != 3 {
		t.Fatalf("expected 3 distinct attachment paths, got %d", len(seenPaths))
	}
	for _, want := range []string{"first", "second", "third"} {
		if _, ok := seenContents[want]; !ok {
			t.Fatalf("missing attachment content %q in %#v", want, seenContents)
		}
	}
	if _, ok := seenPaths["hero-2.jpg"]; !ok {
		t.Fatalf("expected one reserved suffixed filename, got %#v", seenPaths)
	}
}

func TestDownloadPageAssetsRemovesPartialFileOnCopyError(t *testing.T) {
	doc := PageDocument{
		"items": map[string]any{
			"hero": map[string]any{
				"file": map[string]any{
					"url":      "https://example.test/assets/hero.jpg",
					"filename": "hero.jpg",
				},
			},
		},
	}

	client := New("https://example.test", "")
	client.HTTPClient = &http.Client{
		Transport: pageRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode:    http.StatusOK,
				ContentLength: 5,
				Header:        make(http.Header),
				Body: &failingPageReadCloser{
					data: []byte("abc"),
					err:  errors.New("stream broke"),
				},
				Request: req,
			}, nil
		}),
	}

	dir := t.TempDir()
	_, err := DownloadPageAssets(context.Background(), client, doc, dir)
	if err == nil {
		t.Fatal("expected download error")
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatalf("read temp dir: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no leftover files, got %d", len(entries))
	}
}

type pageRoundTripperFunc func(*http.Request) (*http.Response, error)

func (fn pageRoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type failingPageReadCloser struct {
	data []byte
	err  error
	read bool
}

func (r *failingPageReadCloser) Read(p []byte) (int, error) {
	if !r.read {
		r.read = true
		n := copy(p, r.data)
		return n, nil
	}
	return 0, r.err
}

func (r *failingPageReadCloser) Close() error {
	return nil
}
