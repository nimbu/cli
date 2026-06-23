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
					"attachment_url":  "https://cdn.example.test/new",
					"url":             "https://example.test/old",
					"public_url":      "https://example.test/public",
					"permanent_url":   "https://example.test/permanent",
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
	if file["__type"] != "File" {
		t.Fatalf("expected __type File, got %#v", file["__type"])
	}
	if file["filename"] != "hero.txt" {
		t.Fatalf("unexpected filename: %#v", file["filename"])
	}
	if _, ok := file["url"]; ok {
		t.Fatalf("url should be removed, got %#v", file["url"])
	}
	if _, ok := file["public_url"]; ok {
		t.Fatalf("public_url should be removed, got %#v", file["public_url"])
	}
	if _, ok := file["permanent_url"]; ok {
		t.Fatalf("permanent_url should be removed, got %#v", file["permanent_url"])
	}
	if _, ok := file["attachment_url"]; ok {
		t.Fatalf("attachment_url should be removed when attachment_path wins")
	}
	if _, ok := file["attachment_path"]; ok {
		t.Fatalf("attachment_path should be removed")
	}
}

func TestExpandPageAttachmentPathsRewritesAttachmentURL(t *testing.T) {
	doc := PageDocument{
		"items": map[string]any{
			"hero": map[string]any{
				"file": map[string]any{
					"attachment_url":  "https://cdn.example.test/hero.jpg",
					"attachment_path": "",
				},
			},
		},
	}

	if err := ExpandPageAttachmentPaths(doc); err != nil {
		t.Fatalf("expand attachment_url: %v", err)
	}

	file := doc["items"].(map[string]any)["hero"].(map[string]any)["file"].(map[string]any)
	if file["__type"] != "FileRef" {
		t.Fatalf("expected __type FileRef, got %#v", file["__type"])
	}
	if file["source"] != "https://cdn.example.test/hero.jpg" {
		t.Fatalf("unexpected source: %#v", file["source"])
	}
	if _, ok := file["attachment_url"]; ok {
		t.Fatalf("attachment_url should be removed")
	}
	if _, ok := file["attachment_path"]; ok {
		t.Fatalf("attachment_path should be removed")
	}
}

func TestExpandPageAttachmentPathsAllowsDirectFileRefSource(t *testing.T) {
	doc := PageDocument{
		"items": map[string]any{
			"hero": map[string]any{
				"file": map[string]any{
					"__type":        "FileRef",
					"source":        "https://cdn.example.test/hero.jpg",
					"url":           "https://cdn.example.test/read-only-url.jpg",
					"public_url":    "https://cdn.example.test/read-only-public.jpg",
					"permanent_url": "https://cdn.example.test/read-only-permanent.jpg",
				},
			},
		},
	}

	if err := ExpandPageAttachmentPaths(doc); err != nil {
		t.Fatalf("direct FileRef source should be accepted: %v", err)
	}

	stats := PageStats(doc)
	if stats.AttachmentCount != 1 {
		t.Fatalf("FileRef source should count as attachment, got %d", stats.AttachmentCount)
	}
	file := doc["items"].(map[string]any)["hero"].(map[string]any)["file"].(map[string]any)
	if _, ok := file["url"]; ok {
		t.Fatalf("read-only url should be removed from direct FileRef payload")
	}
	if _, ok := file["public_url"]; ok {
		t.Fatalf("read-only public_url should be removed from direct FileRef payload")
	}
	if _, ok := file["permanent_url"]; ok {
		t.Fatalf("read-only permanent_url should be removed from direct FileRef payload")
	}
}

func TestExpandPageAttachmentPathsMarksRawAttachmentAsFile(t *testing.T) {
	doc := PageDocument{
		"items": map[string]any{
			"hero": map[string]any{
				"file": map[string]any{
					"attachment":    base64.StdEncoding.EncodeToString([]byte("hello")),
					"filename":      "hero.txt",
					"url":           "https://cdn.example.test/read-only-url.jpg",
					"public_url":    "https://cdn.example.test/read-only-public.jpg",
					"permanent_url": "https://cdn.example.test/read-only-permanent.jpg",
				},
			},
		},
	}

	if err := ExpandPageAttachmentPaths(doc); err != nil {
		t.Fatalf("raw attachment should be accepted: %v", err)
	}

	file := doc["items"].(map[string]any)["hero"].(map[string]any)["file"].(map[string]any)
	if file["__type"] != "File" {
		t.Fatalf("expected raw attachment to be marked as File, got %#v", file["__type"])
	}
	if _, ok := file["url"]; ok {
		t.Fatalf("read-only url should be removed from raw attachment payload")
	}
	if _, ok := file["public_url"]; ok {
		t.Fatalf("read-only public_url should be removed from raw attachment payload")
	}
	if _, ok := file["permanent_url"]; ok {
		t.Fatalf("read-only permanent_url should be removed from raw attachment payload")
	}
}

func TestExpandPageAttachmentPathsErrorsOnEmptyFile(t *testing.T) {
	doc := PageDocument{
		"items": map[string]any{
			"hero": map[string]any{
				"file": map[string]any{
					"filename": "hero.jpg",
				},
			},
		},
	}

	err := ExpandPageAttachmentPaths(doc)
	if err == nil || !strings.Contains(err.Error(), "refusing to write an empty file") {
		t.Fatalf("expected empty-file error, got %v", err)
	}
}

func TestExpandPageAttachmentPathsErrorsOnURLOnlyFile(t *testing.T) {
	doc := PageDocument{
		"items": map[string]any{
			"hero": map[string]any{
				"file": map[string]any{
					"url": "https://cdn.example.test/hero.jpg",
				},
			},
		},
	}

	err := ExpandPageAttachmentPaths(doc)
	if err == nil || !strings.Contains(err.Error(), "refusing to write an empty file") {
		t.Fatalf("expected url-only file error, got %v", err)
	}
}

func TestExpandPageAttachmentPathsCanDropURLOnlyFile(t *testing.T) {
	doc := PageDocument{
		"items": map[string]any{
			"hero": map[string]any{
				"type": "file",
				"file": map[string]any{
					"url": "https://cdn.example.test/hero.jpg",
				},
			},
		},
	}

	err := ExpandPageAttachmentPathsWithOptions(doc, PageAttachmentExpansionOptions{DropReadOnlyFileURL: true})
	if err != nil {
		t.Fatalf("drop url-only file should not error: %v", err)
	}
	hero := doc["items"].(map[string]any)["hero"].(map[string]any)
	if _, ok := hero["file"]; ok {
		t.Fatalf("url-only file should be dropped from write payload, got %#v", hero["file"])
	}
}

func TestExpandPageAttachmentPathsCanAllowEmptyFile(t *testing.T) {
	doc := PageDocument{
		"items": map[string]any{
			"hero": map[string]any{
				"file": map[string]any{
					"filename": "hero.jpg",
				},
			},
		},
	}

	err := ExpandPageAttachmentPathsWithOptions(doc, PageAttachmentExpansionOptions{AllowEmptyFile: true})
	if err != nil {
		t.Fatalf("allow empty file should skip guard: %v", err)
	}
}

func TestNormalizePageDocumentForWrite(t *testing.T) {
	doc := PageDocument{
		"id":          "p1",
		"created_at":  "2020-01-01",
		"updated_at":  "2020-01-02",
		"creator_id":  "u1",
		"updater_id":  "u2",
		"parent":      "about",
		"parent_path": "about",
		"title":       "About",
		"items":       map[string]any{},
	}

	NormalizePageDocumentForWrite(doc)

	for _, key := range []string{"id", "created_at", "updated_at", "creator_id", "updater_id", "parent_path"} {
		if _, ok := doc[key]; ok {
			t.Fatalf("expected %q to be removed", key)
		}
	}
	if doc["parent"] != "about" {
		t.Fatalf("parent should be preserved as writable fullpath, got %#v", doc["parent"])
	}
	if doc["title"] != "About" {
		t.Fatalf("title should be preserved, got %#v", doc["title"])
	}
	if _, ok := doc["items"]; !ok {
		t.Fatalf("items should be preserved")
	}
}

func TestPatchPageDocumentReplaceParam(t *testing.T) {
	tests := []struct {
		name        string
		opts        []RequestOption
		wantReplace bool
	}{
		{name: "default merges without replace", wantReplace: false},
		{name: "with replace adds replace=1", opts: []RequestOption{WithReplace(true)}, wantReplace: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotRawQuery string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotRawQuery = r.URL.RawQuery
				_, _ = w.Write([]byte(`{"id":"p1"}`))
			}))
			defer srv.Close()

			_, err := PatchPageDocument(context.Background(), New(srv.URL, ""), "about", PageDocument{"title": "X"}, tt.opts...)
			if err != nil {
				t.Fatalf("patch page: %v", err)
			}

			hasReplace := strings.Contains(gotRawQuery, "replace=1")
			if hasReplace != tt.wantReplace {
				t.Fatalf("replace=1 present=%v, want %v (raw query %q)", hasReplace, tt.wantReplace, gotRawQuery)
			}
		})
	}
}

func TestPageShape(t *testing.T) {
	doc := PageDocument{
		"items": map[string]any{
			"intro": map[string]any{
				"type": "string",
			},
			"blocks": map[string]any{
				"type": "canvas",
				"repeatables": []any{
					map[string]any{
						"slug": "text_block",
						"items": map[string]any{
							"body": map[string]any{"type": "text"},
						},
					},
				},
			},
		},
	}

	shape, ok := PageShape(doc).(map[string]any)
	if !ok {
		t.Fatalf("expected map shape, got %T", PageShape(doc))
	}

	intro := shape["intro"].(map[string]any)
	if intro["type"] != "string" {
		t.Fatalf("unexpected intro type: %#v", intro["type"])
	}
	if _, ok := intro["repeatables"]; ok {
		t.Fatalf("non-canvas editable should have no repeatables: %#v", intro)
	}

	blocks := shape["blocks"].(map[string]any)
	reps := blocks["repeatables"].([]any)
	if len(reps) != 1 {
		t.Fatalf("expected 1 repeatable, got %d", len(reps))
	}
	rep := reps[0].(map[string]any)
	if rep["slug"] != "text_block" {
		t.Fatalf("unexpected slug: %#v", rep["slug"])
	}
	nested := rep["items"].(map[string]any)["body"].(map[string]any)
	if nested["type"] != "text" {
		t.Fatalf("unexpected nested type: %#v", nested["type"])
	}
}

func TestPageCanvasRepeatableCounts(t *testing.T) {
	doc := PageDocument{
		"items": map[string]any{
			"intro": map[string]any{"type": "string"},
			"blocks": map[string]any{
				"type": "canvas",
				"repeatables": []any{
					map[string]any{"slug": "a"},
					map[string]any{"slug": "b"},
				},
			},
		},
	}

	counts := PageCanvasRepeatableCounts(doc)
	if counts["blocks"] != 2 {
		t.Fatalf("expected 2 repeatables for blocks, got %d", counts["blocks"])
	}
	if _, ok := counts["intro"]; ok {
		t.Fatalf("non-canvas editable should be omitted: %#v", counts)
	}
}

func TestPageCanvasRepeatableCountsIncludesNestedCanvasPaths(t *testing.T) {
	doc := PageDocument{
		"items": map[string]any{
			"blocks": map[string]any{
				"type": "canvas",
				"repeatables": []any{
					map[string]any{
						"slug": "section",
						"items": map[string]any{
							"gallery": map[string]any{
								"type": "canvas",
								"repeatables": []any{
									map[string]any{"slug": "image"},
								},
							},
						},
					},
				},
			},
		},
	}

	counts := PageCanvasRepeatableCounts(doc)
	if counts["blocks"] != 1 {
		t.Fatalf("top-level canvas count = %d, want 1", counts["blocks"])
	}
	if counts["blocks.gallery"] != 1 {
		t.Fatalf("nested canvas count = %d, want 1 (counts %#v)", counts["blocks.gallery"], counts)
	}
}

func TestPageCanvasRepeatableInstanceCountsIncludesNestedIndexes(t *testing.T) {
	doc := PageDocument{
		"items": map[string]any{
			"blocks": map[string]any{
				"type": "canvas",
				"repeatables": []any{
					map[string]any{
						"slug": "section",
						"items": map[string]any{
							"gallery": map[string]any{
								"type": "canvas",
								"repeatables": []any{
									map[string]any{"slug": "image"},
									map[string]any{"slug": "image"},
								},
							},
						},
					},
					map[string]any{
						"slug": "section",
						"items": map[string]any{
							"gallery": map[string]any{
								"type":        "canvas",
								"repeatables": []any{map[string]any{"slug": "image"}},
							},
						},
					},
				},
			},
		},
	}

	counts := PageCanvasRepeatableInstanceCounts(doc)
	if counts["blocks"] != 2 {
		t.Fatalf("top-level canvas count = %d, want 2", counts["blocks"])
	}
	if counts["blocks[0].gallery"] != 2 {
		t.Fatalf("first nested canvas count = %d, want 2 (counts %#v)", counts["blocks[0].gallery"], counts)
	}
	if counts["blocks[1].gallery"] != 1 {
		t.Fatalf("second nested canvas count = %d, want 1 (counts %#v)", counts["blocks[1].gallery"], counts)
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
	count, warnings, err := DownloadPageAssets(context.Background(), New(srv.URL, ""), doc, dir)
	if err != nil {
		t.Fatalf("download assets: %v", err)
	}
	if len(warnings) > 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
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
	count, warnings, err := DownloadPageAssets(context.Background(), New(srv.URL, ""), doc, dir)
	if err != nil {
		t.Fatalf("download assets: %v", err)
	}
	if len(warnings) > 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
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
	count, warnings, err := DownloadPageAssets(context.Background(), client, doc, dir)
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 downloads, got %d", count)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(warnings), warnings)
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
