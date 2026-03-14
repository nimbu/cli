package migrate

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
)

func TestCopyUploadsReusesAndCreatesUploadsAndBuildsRewritePlan(t *testing.T) {
	var assetHits []string
	var uploadNames []string
	baseURL := ""

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/uploads" && r.Header.Get("X-Nimbu-Site") == "source":
			_, _ = w.Write([]byte(`[{"id":"src-reuse","name":"hero.jpg","url":"` + baseURL + `/cdn/hero.jpg?download=1","size":4},{"id":"src-new","name":"fresh.jpg","url":"` + baseURL + `/cdn/fresh.jpg","size":5},{"id":"src-skip","name":"missing.jpg","size":7},{"id":"src-amb","name":"dup.jpg","url":"` + baseURL + `/cdn/dup.jpg","size":3}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/uploads" && r.Header.Get("X-Nimbu-Site") == "target":
			_, _ = w.Write([]byte(`[{"id":"target-reuse","name":"hero.jpg","url":"https://cdn.target.test/reused-hero.jpg","size":4},{"id":"target-dup-1","name":"dup.jpg","url":"https://cdn.target.test/dup-1.jpg","size":3},{"id":"target-dup-2","name":"dup.jpg","url":"https://cdn.target.test/dup-2.jpg","size":3}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/sites/source":
			_, _ = w.Write([]byte(`{"id":"source","subdomain":"source","domain":"old-site.test"}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/cdn/"):
			assetHits = append(assetHits, r.URL.Path)
			switch r.URL.Path {
			case "/cdn/fresh.jpg":
				_, _ = w.Write([]byte("fresh"))
			case "/cdn/dup.jpg":
				_, _ = w.Write([]byte("dup"))
			case "/cdn/hero.jpg":
				_, _ = w.Write([]byte("hero"))
			default:
				http.NotFound(w, r)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/uploads" && r.Header.Get("X-Nimbu-Site") == "target":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatalf("parse multipart form: %v", err)
			}
			file, header, err := r.FormFile("file")
			if err != nil {
				t.Fatalf("read uploaded file: %v", err)
			}
			defer func() { _ = file.Close() }()
			body, err := io.ReadAll(file)
			if err != nil {
				t.Fatalf("read uploaded bytes: %v", err)
			}
			uploadNames = append(uploadNames, header.Filename+":"+string(body))
			var payload string
			switch header.Filename {
			case "fresh.jpg":
				payload = `{"id":"uploaded-fresh","name":"fresh.jpg","url":"https://cdn.target.test/fresh.jpg","size":5}`
			case "dup.jpg":
				payload = `{"id":"uploaded-dup","name":"dup.jpg","url":"https://cdn.target.test/dup.jpg","size":3}`
			default:
				t.Fatalf("unexpected upload filename: %s", header.Filename)
			}
			_, _ = w.Write([]byte(payload))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	baseURL = srv.URL

	fromClient := api.New(srv.URL, "").WithSite("source")
	toClient := api.New(srv.URL, "").WithSite("target")

	result, plan, err := CopyUploads(context.Background(), fromClient, toClient, SiteRef{Site: "source"}, SiteRef{Site: "target"}, false)
	if err != nil {
		t.Fatalf("copy uploads: %v", err)
	}
	if len(result.Items) != 4 {
		t.Fatalf("expected 4 upload items, got %d", len(result.Items))
	}
	bySourceID := map[string]UploadCopyItem{}
	for _, item := range result.Items {
		bySourceID[item.SourceID] = item
	}
	if item := bySourceID["src-reuse"]; item.Action != "reuse" || item.TargetID != "target-reuse" {
		t.Fatalf("unexpected reuse item: %#v", item)
	}
	if item := bySourceID["src-new"]; item.Action != "create" || item.TargetID != "uploaded-fresh" {
		t.Fatalf("unexpected create item: %#v", item)
	}
	if item := bySourceID["src-skip"]; item.Action != "skip" {
		t.Fatalf("unexpected skip item: %#v", item)
	}
	if item := bySourceID["src-amb"]; item.Action != "create" || item.TargetID != "uploaded-dup" {
		t.Fatalf("unexpected ambiguous-create item: %#v", item)
	}
	if len(result.Warnings) != 2 {
		t.Fatalf("expected 2 warnings, got %#v", result.Warnings)
	}
	if len(uploadNames) != 2 {
		t.Fatalf("expected 2 uploads to target, got %#v", uploadNames)
	}
	uploadedSet := strings.Join(sortedStrings(uploadNames), ",")
	if uploadedSet != "dup.jpg:dup,fresh.jpg:fresh" {
		t.Fatalf("unexpected uploaded payloads: %s", uploadedSet)
	}
	assetSet := strings.Join(sortedStrings(assetHits), ",")
	if assetSet != "/cdn/dup.jpg,/cdn/fresh.jpg" {
		t.Fatalf("unexpected asset downloads: %s", assetSet)
	}

	rewritten := plan.RewriteString("body.html", `<img src="https://old-site.test/cdn/hero.jpg?cache=9"> <img src="https://old-site.test/cdn/fresh.jpg"> <img src="https://old-site.test/cdn/dup.jpg">`)
	for _, want := range []string{"https://cdn.target.test/reused-hero.jpg", "https://cdn.target.test/fresh.jpg", "https://cdn.target.test/dup.jpg"} {
		if !strings.Contains(rewritten, want) {
			t.Fatalf("expected rewritten string to contain %q, got %s", want, rewritten)
		}
	}
}

func TestMediaRewritePlanWarnsForUnresolvedSourceHostURL(t *testing.T) {
	plan := NewMediaRewritePlan()
	plan.Add("https://media.source.test/uploads/hero.jpg", "https://cdn.target.test/hero.jpg")

	got := plan.RewriteString("content.html", `<img src="https://media.source.test/uploads/missing.jpg">`)
	if !strings.Contains(got, "https://media.source.test/uploads/missing.jpg") {
		t.Fatalf("unexpected rewrite: %s", got)
	}
	warnings := plan.Warnings()
	if len(warnings) != 1 {
		t.Fatalf("expected one warning, got %#v", warnings)
	}
	if !strings.Contains(warnings[0], "unresolved media URL") {
		t.Fatalf("unexpected warning: %s", warnings[0])
	}
}

func TestMediaRewritePlanDoesNotWarnForNonMediaURLOnSourceHost(t *testing.T) {
	plan := NewMediaRewritePlan()
	plan.Add("https://old-site.test/uploads/hero.jpg", "https://cdn.target.test/hero.jpg")

	got := plan.RewriteString("content.html", `<a href="https://old-site.test/contact">Contact</a>`)
	if got != `<a href="https://old-site.test/contact">Contact</a>` {
		t.Fatalf("unexpected rewrite: %s", got)
	}
	if warnings := plan.Warnings(); len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", warnings)
	}
}

func TestMediaRewritePlanKeepsBalancedClosingParenInURL(t *testing.T) {
	plan := NewMediaRewritePlan()
	plan.Add("https://media.source.test/uploads/image_(1).jpg", "https://cdn.target.test/image_(1).jpg")

	got := plan.RewriteString("content.html", `<img src="https://media.source.test/uploads/image_(1).jpg">`)
	if got != `<img src="https://cdn.target.test/image_(1).jpg">` {
		t.Fatalf("unexpected rewrite: %s", got)
	}
}

func TestCopyChannelEntriesRewritesKnownUploadURLsInStringFields(t *testing.T) {
	plan := NewMediaRewritePlan()
	plan.Add("https://media.source.test/uploads/hero.jpg", "https://cdn.target.test/hero.jpg")

	var created map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/channels":
			_, _ = w.Write([]byte(`[{"slug":"articles","customizations":[{"name":"content","type":"text"},{"name":"metadata","type":"string"}]}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/channels/articles/entries" && r.Header.Get("X-Nimbu-Site") == "source":
			_, _ = w.Write([]byte(`[{"id":"entry-1","slug":"hello","content":"<img src=\"https://media.source.test/uploads/hero.jpg\">","metadata":"{\"image\":\"https://media.source.test/uploads/hero.jpg\"}","note":"https://other.example.test/keep"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/channels/articles/entries" && r.Header.Get("X-Nimbu-Site") == "target":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/channels/articles/entries":
			if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
				t.Fatalf("decode created entry: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"created-entry"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	_, err := CopyChannelEntries(
		context.Background(),
		api.New(srv.URL, "").WithSite("source"),
		api.New(srv.URL, "").WithSite("target"),
		ChannelRef{SiteRef: SiteRef{Site: "source"}, Channel: "articles"},
		ChannelRef{SiteRef: SiteRef{Site: "target"}, Channel: "articles"},
		RecordCopyOptions{Media: plan},
	)
	if err != nil {
		t.Fatalf("copy channel entries: %v", err)
	}
	if created["content"] != `<img src="https://cdn.target.test/hero.jpg">` {
		t.Fatalf("unexpected content rewrite: %#v", created["content"])
	}
	if created["metadata"] != `{"image":"https://cdn.target.test/hero.jpg"}` {
		t.Fatalf("unexpected metadata rewrite: %#v", created["metadata"])
	}
	if created["note"] != "https://other.example.test/keep" {
		t.Fatalf("unexpected unrelated URL rewrite: %#v", created["note"])
	}
}

func TestCopyPagesRewritesTextContentAndPreservesFileAttachments(t *testing.T) {
	plan := NewMediaRewritePlan()
	plan.Add("https://media.source.test/uploads/hero.jpg", "https://cdn.target.test/hero.jpg")

	var created api.PageDocument
	baseURL := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/pages" && r.Header.Get("X-Nimbu-Site") == "source":
			_, _ = w.Write([]byte(`[{"fullpath":"about"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/pages/about" && r.Header.Get("X-Nimbu-Site") == "source":
			_, _ = w.Write([]byte(`{"fullpath":"about","items":{"hero":{"file":{"url":"` + baseURL + `/page-assets/hero.bin","filename":"hero.bin"}},"body":{"text":"<p><img src=\"https://media.source.test/uploads/hero.jpg\"></p>"}}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/page-assets/hero.bin":
			_, _ = w.Write([]byte("asset"))
		case r.Method == http.MethodGet && r.URL.Path == "/pages/about" && r.Header.Get("X-Nimbu-Site") == "target":
			http.NotFound(w, r)
		case r.Method == http.MethodPost && r.URL.Path == "/pages" && r.Header.Get("X-Nimbu-Site") == "target":
			if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
				t.Fatalf("decode created page: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"page-1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	baseURL = srv.URL

	_, err := CopyPages(
		context.Background(),
		api.New(srv.URL, "").WithSite("source"),
		api.New(srv.URL, "").WithSite("target"),
		SiteRef{Site: "source"},
		SiteRef{Site: "target"},
		"*",
		plan,
		false,
	)
	if err != nil {
		t.Fatalf("copy pages: %v", err)
	}

	body := created["items"].(map[string]any)["body"].(map[string]any)["text"]
	if body != `<p><img src="https://cdn.target.test/hero.jpg"></p>` {
		t.Fatalf("unexpected page text rewrite: %#v", body)
	}
	file := created["items"].(map[string]any)["hero"].(map[string]any)["file"].(map[string]any)
	if file["attachment"] != base64.StdEncoding.EncodeToString([]byte("asset")) {
		t.Fatalf("expected attachment payload, got %#v", file["attachment"])
	}
	if _, ok := file["url"]; ok {
		t.Fatalf("expected file url removed, got %#v", file["url"])
	}
}

func TestCopyMenusRewritesKnownUploadURLs(t *testing.T) {
	plan := NewMediaRewritePlan()
	plan.Add("https://media.source.test/uploads/hero.jpg", "https://cdn.target.test/hero.jpg")

	var created api.MenuDocument
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/menus" && r.Header.Get("X-Nimbu-Site") == "source":
			_, _ = w.Write([]byte(`[{"slug":"main","items":[{"title":"Download","url":"https://media.source.test/uploads/hero.jpg"}]}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/menus/main" && r.Header.Get("X-Nimbu-Site") == "target":
			http.NotFound(w, r)
		case r.Method == http.MethodPost && r.URL.Path == "/menus" && r.Header.Get("X-Nimbu-Site") == "target":
			if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
				t.Fatalf("decode created menu: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"menu-1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	_, err := CopyMenus(
		context.Background(),
		api.New(srv.URL, "").WithSite("source"),
		api.New(srv.URL, "").WithSite("target"),
		SiteRef{Site: "source"},
		SiteRef{Site: "target"},
		"*",
		true,
		plan,
		false,
	)
	if err != nil {
		t.Fatalf("copy menus: %v", err)
	}
	items := created["items"].([]any)
	if got := items[0].(map[string]any)["url"]; got != "https://cdn.target.test/hero.jpg" {
		t.Fatalf("unexpected menu url rewrite: %#v", got)
	}
}

func TestCopyTranslationsRewritesKnownUploadURLs(t *testing.T) {
	plan := NewMediaRewritePlan()
	plan.Add("https://media.source.test/uploads/hero.jpg", "https://cdn.target.test/hero.jpg")

	var created api.Translation
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/translations":
			_, _ = w.Write([]byte(`[{"key":"hero","value":"https://media.source.test/uploads/hero.jpg","values":{"nl":"<img src=\"https://media.source.test/uploads/hero.jpg\">"}}]`))
		case r.Method == http.MethodPost && r.URL.Path == "/translations":
			if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
				t.Fatalf("decode created translation: %v", err)
			}
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	_, err := CopyTranslations(
		context.Background(),
		api.New(srv.URL, "").WithSite("source"),
		api.New(srv.URL, "").WithSite("target"),
		SiteRef{Site: "source"},
		SiteRef{Site: "target"},
		TranslationCopyOptions{Query: "*", Media: plan},
	)
	if err != nil {
		t.Fatalf("copy translations: %v", err)
	}
	if created.Value != "https://cdn.target.test/hero.jpg" {
		t.Fatalf("unexpected translation value rewrite: %s", created.Value)
	}
	if created.Values["nl"] != `<img src="https://cdn.target.test/hero.jpg">` {
		t.Fatalf("unexpected localized translation rewrite: %s", created.Values["nl"])
	}
}

func TestNewUploadMultipartBody(t *testing.T) {
	body, err := newMultipartBytesBody([]byte("hello"), "asset.txt")
	if err != nil {
		t.Fatalf("build multipart body: %v", err)
	}
	data, err := io.ReadAll(body.Reader)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/uploads", strings.NewReader(string(data)))
	req.Header.Set("Content-Type", body.ContentType)
	if err := req.ParseMultipartForm(1 << 20); err != nil {
		t.Fatalf("parse multipart request: %v", err)
	}
	file, header, err := req.FormFile("file")
	if err != nil {
		t.Fatalf("read form file: %v", err)
	}
	defer func() { _ = file.Close() }()
	payload, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("read multipart payload: %v", err)
	}
	if header.Filename != "asset.txt" {
		t.Fatalf("unexpected filename: %s", header.Filename)
	}
	if string(payload) != "hello" {
		t.Fatalf("unexpected payload: %s", string(payload))
	}
}

func sortedStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}
