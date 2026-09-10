package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeFileRefURLSameSiteCDNBecomesNimbuSource(t *testing.T) {
	t.Parallel()

	const (
		siteShort = "oa8td8r"
		uploadID  = "507f1f77bcf86cd799439014"
		cdnURL    = "https://cdn.nimbu.io/s/oa8td8r/assets/1778666070043/dot.png"
	)

	var sawUploads bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/uploads" {
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		sawUploads = true
		_ = json.NewEncoder(w).Encode([]Upload{{
			ID:  uploadID,
			URL: cdnURL,
		}})
	}))
	t.Cleanup(server.Close)

	got, warning, err := newTestFileRefNormalizer(server.URL, siteShort).NormalizeURL(context.Background(), cdnURL)
	if err != nil {
		t.Fatalf("NormalizeURL: %v", err)
	}
	if !sawUploads {
		t.Fatal("expected GET /uploads lookup")
	}
	if warning != "" {
		t.Fatalf("warning = %q, want empty", warning)
	}
	if got["__type"] != "FileRef" {
		t.Fatalf("__type = %#v", got["__type"])
	}
	wantSource := "nimbu://" + siteShort + "/uploads/" + uploadID
	if got["source"] != wantSource {
		t.Fatalf("source = %#v, want %q", got["source"], wantSource)
	}
	if _, ok := got["url"]; ok {
		t.Fatalf("url should be omitted on same-site hit: %#v", got)
	}
}

func TestNormalizeFileRefURLOtherSiteCDNPassthroughWarns(t *testing.T) {
	t.Parallel()

	const otherURL = "https://cdn.nimbu.io/s/othersite/uploads/abc/file.png"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("other-site CDN must not look up uploads, got %s", r.URL.Path)
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)

	got, warning, err := newTestFileRefNormalizer(server.URL, "oa8td8r").NormalizeURL(context.Background(), otherURL)
	if err != nil {
		t.Fatalf("NormalizeURL: %v", err)
	}
	assertFileRefSource(t, got, otherURL)
	if !strings.Contains(warning, otherURL) || !strings.Contains(warning, "is not an upload of this site; the server will copy it") {
		t.Fatalf("warning = %q", warning)
	}
}

func TestNormalizeFileRefURLLookupMissPassthroughWarns(t *testing.T) {
	t.Parallel()

	const cdnURL = "https://cdn.nimbu.io/s/oa8td8r/uploads/missing/file.png"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/uploads" {
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]Upload{})
	}))
	t.Cleanup(server.Close)

	got, warning, err := newTestFileRefNormalizer(server.URL, "oa8td8r").NormalizeURL(context.Background(), cdnURL)
	if err != nil {
		t.Fatalf("NormalizeURL: %v", err)
	}
	assertFileRefSource(t, got, cdnURL)
	if !strings.Contains(warning, cdnURL) || !strings.Contains(warning, "is not an upload of this site; the server will copy it") {
		t.Fatalf("warning = %q", warning)
	}
}

func TestNormalizeFileRefURLNonCDNPassthroughNoWarning(t *testing.T) {
	t.Parallel()

	const remote = "https://nimbu.io/favicon.ico"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("non-CDN URL must not look up uploads, got %s", r.URL.Path)
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)

	got, warning, err := newTestFileRefNormalizer(server.URL, "oa8td8r").NormalizeURL(context.Background(), remote)
	if err != nil {
		t.Fatalf("NormalizeURL: %v", err)
	}
	assertFileRefSource(t, got, remote)
	if warning != "" {
		t.Fatalf("warning = %q, want empty", warning)
	}
}

func newTestFileRefNormalizer(baseURL, siteShortID string) *FileRefNormalizer {
	return &FileRefNormalizer{
		Client:      New(baseURL, "token"),
		SiteShortID: siteShortID,
	}
}

func assertFileRefSource(t *testing.T, got map[string]any, want string) {
	t.Helper()
	if got["__type"] != "FileRef" {
		t.Fatalf("__type = %#v, want FileRef", got["__type"])
	}
	if got["source"] != want {
		t.Fatalf("source = %#v, want %q", got["source"], want)
	}
	if _, ok := got["url"]; ok {
		t.Fatalf("url should be omitted on FileRef source passthrough: %#v", got)
	}
}
