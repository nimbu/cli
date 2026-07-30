package migrate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
)

func TestCopyBlogsPreservesWritableBlogAndPostFields(t *testing.T) {
	var blogPayload, postPayload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/blogs" && r.Header.Get("X-Nimbu-Site") == "source":
			_, _ = w.Write([]byte(`[{
				"id":"source-blog","handle":"news","name":"News","description":"Long description",
				"seo_title":"News SEO","translations":{"nl":{"name":"Nieuws","description":"Beschrijving"}},
				"custom_writable":{"featured":true},"url":"https://source.test/blogs/news",
				"created_at":"2025-01-01T00:00:00Z","updated_at":"2025-01-02T00:00:00Z"
			}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/blogs/news" && r.Header.Get("X-Nimbu-Site") == "target":
			_, _ = w.Write([]byte(`{"id":"target-blog","handle":"news","name":"Old"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/blogs/news" && r.Header.Get("X-Nimbu-Site") == "target":
			if err := json.NewDecoder(r.Body).Decode(&blogPayload); err != nil {
				t.Fatalf("decode blog payload: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"target-blog","handle":"news"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/blogs/news/articles" && r.Header.Get("X-Nimbu-Site") == "source":
			_, _ = w.Write([]byte(`[{
				"id":"source-post","slug":"hello","title":"Hello","text_content":"<img src=\"https://source.test/uploads/hero.jpg\">",
				"text_excerpt":"Intro","seo_title":"Hello SEO","seo_description":"Search description",
				"translations":{"nl":{"title":"Hallo","text_content":"<img src=\"https://source.test/uploads/hero.jpg\">","seo_title":"Hallo SEO"}},
				"custom_writable":{"theme":"dark"},"url":"https://source.test/blogs/news/hello",
				"public_url":"https://site.test/blogs/news/hello","next_article":{"id":"other"},
				"created_at":"2025-01-01T00:00:00Z","updated_at":"2025-01-02T00:00:00Z"
			}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/blogs/news/articles" && r.Header.Get("X-Nimbu-Site") == "target":
			_, _ = w.Write([]byte(`[{"id":"target-post","slug":"hello","title":"Old"}]`))
		case r.Method == http.MethodPut && r.URL.Path == "/blogs/news/articles/target-post" && r.Header.Get("X-Nimbu-Site") == "target":
			if err := json.NewDecoder(r.Body).Decode(&postPayload); err != nil {
				t.Fatalf("decode post payload: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"target-post","slug":"hello"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	media := NewMediaRewritePlan()
	media.Add("https://source.test/uploads/hero.jpg", "https://target.test/uploads/hero.jpg")
	_, err := CopyBlogs(
		context.Background(),
		api.New(srv.URL, "").WithSite("source"),
		api.New(srv.URL, "").WithSite("target"),
		SiteRef{Site: "source"},
		SiteRef{Site: "target"},
		"*",
		media,
		false,
	)
	if err != nil {
		t.Fatalf("CopyBlogs error = %v", err)
	}

	if blogPayload["description"] != "Long description" || blogPayload["seo_title"] != "News SEO" {
		t.Fatalf("blog writable fields lost: %#v", blogPayload)
	}
	if got := blogPayload["translations"].(map[string]any)["nl"].(map[string]any)["name"]; got != "Nieuws" {
		t.Fatalf("blog translation = %#v", got)
	}
	if blogPayload["custom_writable"].(map[string]any)["featured"] != true {
		t.Fatalf("blog custom field lost: %#v", blogPayload)
	}
	assertPayloadOmits(t, blogPayload, "id", "created_at", "updated_at", "url")

	if postPayload["text_excerpt"] != "Intro" || postPayload["seo_description"] != "Search description" {
		t.Fatalf("post writable fields lost: %#v", postPayload)
	}
	if !strings.Contains(postPayload["text_content"].(string), "target.test/uploads/hero.jpg") {
		t.Fatalf("default post media was not rewritten: %#v", postPayload["text_content"])
	}
	nl := postPayload["translations"].(map[string]any)["nl"].(map[string]any)
	if nl["seo_title"] != "Hallo SEO" || !strings.Contains(nl["text_content"].(string), "target.test/uploads/hero.jpg") {
		t.Fatalf("localized post fields lost or not rewritten: %#v", nl)
	}
	if postPayload["custom_writable"].(map[string]any)["theme"] != "dark" {
		t.Fatalf("post custom field lost: %#v", postPayload)
	}
	assertPayloadOmits(t, postPayload, "id", "created_at", "updated_at", "url", "public_url", "next_article")
}

func assertPayloadOmits(t *testing.T, payload map[string]any, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if _, ok := payload[key]; ok {
			t.Errorf("payload unexpectedly contains %q: %#v", key, payload)
		}
	}
}
