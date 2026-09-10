package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
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

	n := newTestFileRefNormalizer(server.URL, siteShort)
	n.HTTPClient = failingHTTPClient(t)
	got, warning, err := n.NormalizeURL(context.Background(), cdnURL)
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

func TestNormalizeFileRefURLOtherSiteCDNDownloadsInline(t *testing.T) {
	t.Parallel()

	const (
		otherURL = "https://cdn.nimbu.io/s/othersite/uploads/abc/file.png"
		body     = "other-site-bytes"
	)

	files := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/s/othersite/uploads/abc/file.png" {
			t.Errorf("unexpected download path %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(files.Close)

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("other-site CDN must not look up uploads, got %s", r.URL.Path)
		http.NotFound(w, r)
	}))
	t.Cleanup(apiServer.Close)

	n := newTestFileRefNormalizer(apiServer.URL, "oa8td8r")
	n.HTTPClient = hostRewriteClient(t, files.URL)
	got, warning, err := n.NormalizeURL(context.Background(), otherURL)
	if err != nil {
		t.Fatalf("NormalizeURL: %v", err)
	}
	assertInlineFile(t, got, []byte(body), "file.png")
	if !strings.Contains(warning, otherURL) || !strings.Contains(warning, "is not an upload of this site; downloading it and storing a copy") {
		t.Fatalf("warning = %q", warning)
	}
}

func TestNormalizeFileRefURLLookupMissDownloadsInline(t *testing.T) {
	t.Parallel()

	const (
		cdnURL = "https://cdn.nimbu.io/s/oa8td8r/uploads/missing/file.png"
		body   = "lookup-miss-bytes"
	)

	files := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(files.Close)

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/uploads" {
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]Upload{})
	}))
	t.Cleanup(apiServer.Close)

	n := newTestFileRefNormalizer(apiServer.URL, "oa8td8r")
	n.HTTPClient = hostRewriteClient(t, files.URL)
	got, warning, err := n.NormalizeURL(context.Background(), cdnURL)
	if err != nil {
		t.Fatalf("NormalizeURL: %v", err)
	}
	assertInlineFile(t, got, []byte(body), "file.png")
	if !strings.Contains(warning, cdnURL) || !strings.Contains(warning, "is not an upload of this site; downloading it and storing a copy") {
		t.Fatalf("warning = %q", warning)
	}
}

func TestNormalizeFileRefURLNonCDNDownloadsInline(t *testing.T) {
	t.Parallel()

	const body = "favicon-bytes"
	files := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pics/dot.png" {
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(files.Close)

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("non-CDN URL must not look up uploads, got %s", r.URL.Path)
		http.NotFound(w, r)
	}))
	t.Cleanup(apiServer.Close)

	remote := files.URL + "/pics/dot.png"
	got, warning, err := newTestFileRefNormalizer(apiServer.URL, "oa8td8r").NormalizeURL(context.Background(), remote)
	if err != nil {
		t.Fatalf("NormalizeURL: %v", err)
	}
	assertInlineFile(t, got, []byte(body), "dot.png")
	if !strings.Contains(warning, remote) || !strings.Contains(warning, "is not an upload of this site; downloading it and storing a copy") {
		t.Fatalf("warning = %q", warning)
	}
}

func TestNormalizeFileRefURLNotFound(t *testing.T) {
	t.Parallel()

	files := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	}))
	t.Cleanup(files.Close)

	remote := files.URL + "/gone.png"
	_, _, err := newTestFileRefNormalizer("http://127.0.0.1:1", "oa8td8r").NormalizeURL(context.Background(), remote)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), remote) || !strings.Contains(err.Error(), "404") {
		t.Fatalf("error = %v", err)
	}
}

func TestNormalizeFileRefURLOversize(t *testing.T) {
	t.Parallel()

	files := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(make([]byte, maxInlineFileBytes+1))
	}))
	t.Cleanup(files.Close)

	remote := files.URL + "/huge.bin"
	_, _, err := newTestFileRefNormalizer("http://127.0.0.1:1", "oa8td8r").NormalizeURL(context.Background(), remote)
	if err == nil {
		t.Fatal("expected oversize error")
	}
	if !strings.Contains(err.Error(), remote) {
		t.Fatalf("error = %v", err)
	}
}

func TestNormalizeFileRefURLUsesContentDispositionFilename(t *testing.T) {
	t.Parallel()

	files := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="logo.png"`)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("logo-bytes"))
	}))
	t.Cleanup(files.Close)

	remote := files.URL + "/download"
	got, _, err := newTestFileRefNormalizer("http://127.0.0.1:1", "").NormalizeURL(context.Background(), remote)
	if err != nil {
		t.Fatalf("NormalizeURL: %v", err)
	}
	assertInlineFile(t, got, []byte("logo-bytes"), "logo.png")
}

func newTestFileRefNormalizer(baseURL, siteShortID string) *FileRefNormalizer {
	return &FileRefNormalizer{
		Client:      New(baseURL, "token"),
		SiteShortID: siteShortID,
	}
}

func assertInlineFile(t *testing.T, got map[string]any, wantBytes []byte, filename string) {
	t.Helper()
	if got["__type"] != "File" {
		t.Fatalf("__type = %#v, want File", got["__type"])
	}
	encoded, _ := got["attachment"].(string)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("attachment: %v", err)
	}
	if string(decoded) != string(wantBytes) {
		t.Fatalf("attachment bytes = %q, want %q", decoded, wantBytes)
	}
	if got["filename"] != filename {
		t.Fatalf("filename = %#v, want %q", got["filename"], filename)
	}
	if _, ok := got["source"]; ok {
		t.Fatalf("source should be omitted on inline File: %#v", got)
	}
	if _, ok := got["url"]; ok {
		t.Fatalf("url should be omitted on inline File: %#v", got)
	}
}

func failingHTTPClient(t *testing.T) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Errorf("same-site hit must not download, got %s", req.URL)
		return nil, fmt.Errorf("unexpected download of %s", req.URL)
	})}
}

func hostRewriteClient(t *testing.T, target string) *http.Client {
	t.Helper()
	dest, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			clone := req.Clone(req.Context())
			clone.URL.Scheme = dest.Scheme
			clone.URL.Host = dest.Host
			clone.Host = dest.Host
			clone.RequestURI = ""
			return http.DefaultTransport.RoundTrip(clone)
		}),
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
