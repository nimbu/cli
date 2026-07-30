package migrate

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
)

func TestCopyProductsPreparesLocalizedCustomFieldsBeforePut(t *testing.T) {
	var localizedPayload map[string]any
	var assetBase string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		site := r.Header.Get("X-Nimbu-Site")
		locale := r.URL.Query().Get("content_locale")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/products/customizations" && site == "source":
			_, _ = w.Write([]byte(`[
				{"name":"localized_file","type":"file","localized":true},
				{"name":"localized_gallery","type":"gallery","localized":true},
				{"name":"localized_color","type":"select","localized":true}
			]`))
		case r.Method == http.MethodGet && r.URL.Path == "/sites/source/settings" && site == "source":
			_, _ = w.Write([]byte(`{"default_locale":"en","locales":["en","nl"]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/sites/target/settings" && site == "target":
			_, _ = w.Write([]byte(`{"default_locale":"en","locales":["en","nl"]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/products" && site == "source" && locale == "":
			_, _ = w.Write([]byte(`[{"id":"source-product","slug":"shirt","name":"Shirt"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/products" && site == "source" && locale == "nl":
			_, _ = w.Write([]byte(`[{
				"id":"source-product","slug":"shirt","name":"Hemd",
				"localized_file":{"url":"` + assetBase + `/assets/file.bin","filename":"file.bin"},
				"localized_gallery":{"images":[{"id":"source-image","file":{"url":"` + assetBase + `/assets/gallery.bin","filename":"gallery.bin"}}]},
				"localized_color":{"value":"blue","label":"Blue"}
			}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/products" && site == "target" && locale == "":
			_, _ = w.Write([]byte(`[{"id":"target-product","slug":"shirt","name":"Old"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/products" && site == "target" && locale == "nl":
			_, _ = w.Write([]byte(`[{"id":"target-product","slug":"shirt","name":"Oud"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/assets/file.bin":
			_, _ = w.Write([]byte("localized file"))
		case r.Method == http.MethodGet && r.URL.Path == "/assets/gallery.bin":
			_, _ = w.Write([]byte("localized gallery"))
		case r.Method == http.MethodPut && r.URL.Path == "/products/target-product" && site == "target" && locale == "":
			_, _ = w.Write([]byte(`{"id":"target-product","slug":"shirt"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/products/target-product" && site == "target" && locale == "nl":
			if err := json.NewDecoder(r.Body).Decode(&localizedPayload); err != nil {
				t.Fatalf("decode localized payload: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"target-product","slug":"shirt"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	assetBase = srv.URL

	_, _, err := CopyProducts(
		context.Background(),
		api.New(srv.URL, "").WithSite("source"),
		api.New(srv.URL, "").WithSite("target"),
		SiteRef{Site: "source"},
		SiteRef{Site: "target"},
		ProductCopyOptions{},
	)
	if err != nil {
		t.Fatalf("CopyProducts error = %v", err)
	}

	file := localizedPayload["localized_file"].(map[string]any)
	if file["attachment"] != base64.StdEncoding.EncodeToString([]byte("localized file")) || file["__type"] != "File" {
		t.Fatalf("localized file was not embedded: %#v", file)
	}
	if _, ok := file["url"]; ok {
		t.Fatalf("localized file retained source URL: %#v", file)
	}
	gallery := localizedPayload["localized_gallery"].(map[string]any)
	image := gallery["images"].([]any)[0].(map[string]any)
	if _, ok := image["id"]; ok {
		t.Fatalf("localized gallery retained source image ID: %#v", image)
	}
	galleryFile := image["file"].(map[string]any)
	if galleryFile["attachment"] != base64.StdEncoding.EncodeToString([]byte("localized gallery")) {
		t.Fatalf("localized gallery file was not embedded: %#v", galleryFile)
	}
	if localizedPayload["localized_color"] != "blue" {
		t.Fatalf("localized select was not flattened: %#v", localizedPayload["localized_color"])
	}
}

func TestCopyProductsDryRunComparesPreparedLocalizedSubset(t *testing.T) {
	var writes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		site := r.Header.Get("X-Nimbu-Site")
		locale := r.URL.Query().Get("content_locale")
		if r.Method != http.MethodGet {
			writes++
			_, _ = w.Write([]byte(`{}`))
			return
		}
		switch {
		case r.URL.Path == "/products/customizations":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/sites/source/settings":
			_, _ = w.Write([]byte(`{"default_locale":"en","locales":["en","nl"]}`))
		case r.URL.Path == "/sites/target/settings":
			_, _ = w.Write([]byte(`{"default_locale":"en","locales":["en","nl"]}`))
		case r.URL.Path == "/products" && site == "source" && locale == "":
			_, _ = w.Write([]byte(`[{"id":"source-product","slug":"shirt","name":"Shirt","description":"Default"}]`))
		case r.URL.Path == "/products" && site == "source" && locale == "nl":
			_, _ = w.Write([]byte(`[{
				"id":"source-product","slug":"shirt","name":"Hemd",
				"description":"<img src=\"https://source.test/hero.jpg\">",
				"variants":[{"id":"source-small","sku":"S","label":"Klein"}]
			}]`))
		case r.URL.Path == "/products" && site == "target" && locale == "":
			_, _ = w.Write([]byte(`[{"id":"target-product","slug":"shirt","name":"Old","variants":[{"id":"target-small","sku":"S","label":"Small","price":25}]}]`))
		case r.URL.Path == "/products" && site == "target" && locale == "nl":
			_, _ = w.Write([]byte(`[{
				"id":"target-product","slug":"shirt","name":"Hemd",
				"description":"<img src=\"https://target.test/hero.jpg\">",
				"variants":[{"id":"target-small","sku":"S","label":"Klein","price":25}]
			}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	media := NewMediaRewritePlan()
	media.Add("https://source.test/hero.jpg", "https://target.test/hero.jpg")
	result, _, err := CopyProducts(
		context.Background(),
		api.New(srv.URL, "").WithSite("source"),
		api.New(srv.URL, "").WithSite("target"),
		SiteRef{Site: "source"},
		SiteRef{Site: "target"},
		ProductCopyOptions{DryRun: true, Media: media},
	)
	if err != nil {
		t.Fatalf("CopyProducts dry-run error = %v", err)
	}
	if writes != 0 {
		t.Fatalf("dry-run performed %d writes", writes)
	}
	if len(result.Items) != 1 || len(result.Items[0].Localized) != 1 ||
		result.Items[0].Localized[0].Action != "skip" {
		t.Fatalf("prepared localized dry-run actions = %#v", result.Items)
	}
}

func TestCopyProductsHydratesCreatedTargetBeforeLocalizedVariantWrite(t *testing.T) {
	var hydrated bool
	var localizedPayload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		site := r.Header.Get("X-Nimbu-Site")
		locale := r.URL.Query().Get("content_locale")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/products/customizations" && site == "source":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && r.URL.Path == "/sites/source/settings" && site == "source":
			_, _ = w.Write([]byte(`{"default_locale":"en","locales":["en","nl"]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/sites/target/settings" && site == "target":
			_, _ = w.Write([]byte(`{"default_locale":"en","locales":["en","nl"]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/products" && site == "source" && locale == "":
			_, _ = w.Write([]byte(`[{
				"id":"source-product","slug":"shirt","name":"Shirt",
				"variants":[{"id":"source-small","sku":"S","label":"Small"}]
			}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/products" && site == "source" && locale == "nl":
			_, _ = w.Write([]byte(`[{
				"id":"source-product","slug":"shirt","name":"Hemd",
				"variants":[{"id":"source-small","sku":"S","label":"Klein"}]
			}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/products" && site == "target":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/products" && site == "target":
			_, _ = w.Write([]byte(`{"id":"target-product"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/products/target-product" && site == "target":
			hydrated = true
			_, _ = w.Write([]byte(`{
				"id":"target-product","slug":"shirt","name":"Shirt",
				"variants":[{"id":"target-small","sku":"S","label":"Small","price":25}]
			}`))
		case r.Method == http.MethodPut && r.URL.Path == "/products/target-product" && site == "target" && locale == "nl":
			if err := json.NewDecoder(r.Body).Decode(&localizedPayload); err != nil {
				t.Fatalf("decode localized product: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"target-product"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	result, _, err := CopyProducts(
		context.Background(),
		api.New(srv.URL, "").WithSite("source"),
		api.New(srv.URL, "").WithSite("target"),
		SiteRef{Site: "source"},
		SiteRef{Site: "target"},
		ProductCopyOptions{},
	)
	if err != nil {
		t.Fatalf("CopyProducts error = %v", err)
	}
	if !hydrated {
		t.Fatal("created target product was not fetched before localized writes")
	}
	variant := localizedPayload["variants"].([]any)[0].(map[string]any)
	if variant["id"] != "target-small" || variant["label"] != "Klein" {
		t.Fatalf("localized created-product variant = %#v", variant)
	}
	if len(result.Items) != 1 || len(result.Items[0].Localized) != 1 ||
		result.Items[0].Localized[0].Action != "update" {
		t.Fatalf("created product actions = %#v", result.Items)
	}
}

func TestCopyProductsAllowErrorsSkipsLocalizedCreateWhenHydrationFails(t *testing.T) {
	var localizedWrites int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		site := r.Header.Get("X-Nimbu-Site")
		locale := r.URL.Query().Get("content_locale")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/products/customizations":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && (r.URL.Path == "/sites/source/settings" || r.URL.Path == "/sites/target/settings"):
			_, _ = w.Write([]byte(`{"default_locale":"en","locales":["en","nl"]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/products" && site == "source" && locale == "":
			_, _ = w.Write([]byte(`[{
				"id":"source-product","slug":"shirt","name":"Shirt",
				"variants":[{"id":"source-small","sku":"S","label":"Small"}]
			}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/products" && site == "source" && locale == "nl":
			_, _ = w.Write([]byte(`[{
				"id":"source-product","slug":"shirt","name":"Hemd",
				"variants":[{"id":"source-small","sku":"S","label":"Klein"}]
			}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/products" && site == "target":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/products" && site == "target":
			_, _ = w.Write([]byte(`{"id":"target-product"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/products/target-product" && site == "target":
			http.NotFound(w, r)
		case r.Method == http.MethodPut && locale != "":
			localizedWrites++
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	result, _, err := CopyProducts(
		context.Background(),
		api.New(srv.URL, "").WithSite("source"),
		api.New(srv.URL, "").WithSite("target"),
		SiteRef{Site: "source"},
		SiteRef{Site: "target"},
		ProductCopyOptions{AllowErrors: true},
	)
	if err != nil {
		t.Fatalf("CopyProducts AllowErrors error = %v", err)
	}
	if localizedWrites != 0 {
		t.Fatalf("localized writes = %d after failed target hydration", localizedWrites)
	}
	if len(result.Items) != 1 || len(result.Items[0].Localized) != 1 ||
		result.Items[0].Localized[0].Action != "skip:error" {
		t.Fatalf("failed hydration actions = %#v", result.Items)
	}
	if got := result.Warnings; len(got) == 0 {
		t.Fatal("failed hydration warning was not reported")
	}
}

func TestCopyProductsDryRunCreatePlansLocalizedVariantLabelsWithoutTargetIDs(t *testing.T) {
	var writes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		site := r.Header.Get("X-Nimbu-Site")
		locale := r.URL.Query().Get("content_locale")
		if r.Method != http.MethodGet {
			writes++
			_, _ = w.Write([]byte(`{}`))
			return
		}
		switch {
		case r.URL.Path == "/products/customizations":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/sites/source/settings" || r.URL.Path == "/sites/target/settings":
			_, _ = w.Write([]byte(`{"default_locale":"en","locales":["en","nl"]}`))
		case r.URL.Path == "/products" && site == "source" && locale == "":
			_, _ = w.Write([]byte(`[{
				"id":"source-product","slug":"shirt","name":"Shirt",
				"variants":[{"sku":"S","label":"Small"}]
			}]`))
		case r.URL.Path == "/products" && site == "source" && locale == "nl":
			_, _ = w.Write([]byte(`[{
				"id":"source-product",
				"variants":[{"sku":"S","label":"Klein"}]
			}]`))
		case r.URL.Path == "/products" && site == "target":
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	result, _, err := CopyProducts(
		context.Background(),
		api.New(srv.URL, "").WithSite("source"),
		api.New(srv.URL, "").WithSite("target"),
		SiteRef{Site: "source"},
		SiteRef{Site: "target"},
		ProductCopyOptions{DryRun: true},
	)
	if err != nil {
		t.Fatalf("CopyProducts dry-run error = %v", err)
	}
	if writes != 0 {
		t.Fatalf("dry-run create performed %d writes", writes)
	}
	if len(result.Items) != 1 || result.Items[0].Action != "dry-run:create" ||
		len(result.Items[0].Localized) != 1 || result.Items[0].Localized[0].Action != "dry-run:update" {
		t.Fatalf("dry-run create plan = %#v", result.Items)
	}
	if got := result.Items[0].Localized[0].Fields; len(got) != 1 || got[0] != "variants" {
		t.Fatalf("dry-run localized fields = %#v", got)
	}
	if strings.Contains(strings.Join(result.Warnings, "\n"), "target variant SKU") {
		t.Fatalf("dry-run create reported target-ID warning: %#v", result.Warnings)
	}
}
