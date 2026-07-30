package migrate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
)

func TestCopyProductsWritesLocalizedFieldsSeparately(t *testing.T) {
	var defaultPayload, nlPayload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		site := r.Header.Get("X-Nimbu-Site")
		locale := r.URL.Query().Get("content_locale")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/products/customizations" && site == "source":
			_, _ = w.Write([]byte(`[
				{"name":"marketing_copy","type":"text","localized":true},
				{"name":"internal_code","type":"string","localized":false}
			]`))
		case r.Method == http.MethodGet && r.URL.Path == "/sites/source/settings" && site == "source":
			_, _ = w.Write([]byte(`{"default_locale":"en","locales":["en","nl","fr"]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/sites/target/settings" && site == "target":
			_, _ = w.Write([]byte(`{"default_locale":"en","locales":["en","nl"]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/products" && site == "source" && locale == "":
			_, _ = w.Write([]byte(`[{
				"id":"source-product","slug":"shirt","name":"Shirt","description":"Default description",
				"marketing_copy":"Default copy","internal_code":"ABC","price":25,
				"seo_title":"Default SEO","variant_name":"Size",
				"variants":[
					{"id":"source-small","sku":"S","label":"Small","translations":{"nl":{"label":"Klein"}}},
					{"id":"source-large","sku":"L","label":"Large"}
				],
				"options":[{"translations":{"nl":{"label":"Maat"}},"values":[{"translations":{"nl":{"label":"Klein"}}}]}],
				"translations":{"nl":{"name":"Hemd","marketing_copy":"Nederlandse tekst"}}
			}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/products" && site == "source" && locale == "nl":
			_, _ = w.Write([]byte(`[{
				"id":"source-product","slug":"shirt","name":"Hemd","description":"Nederlandse beschrijving",
				"marketing_copy":"Nederlandse tekst","internal_code":"ABC","price":25,
				"seo_title":"Nederlandse SEO","variant_name":"Maat",
				"variants":[
					{"id":"source-small","sku":"S","label":"Klein","price":25},
					{"id":"source-large","sku":"L","label":"Groot","price":25}
				]
			}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/products" && site == "target":
			_, _ = w.Write([]byte(`[{
				"id":"target-product","slug":"shirt","name":"Old","variants":[
					{"id":"target-large","sku":"L","label":"Old large"},
					{"id":"target-small","sku":"S","label":"Old small"}
				]
			}]`))
		case r.Method == http.MethodPut && r.URL.Path == "/products/target-product" && site == "target" && locale == "":
			if err := json.NewDecoder(r.Body).Decode(&defaultPayload); err != nil {
				t.Fatalf("decode default product payload: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"target-product","slug":"shirt"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/products/target-product" && site == "target" && locale == "nl":
			if err := json.NewDecoder(r.Body).Decode(&nlPayload); err != nil {
				t.Fatalf("decode localized product payload: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"target-product","slug":"shirt"}`))
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

	if defaultPayload["name"] != "Shirt" || defaultPayload["marketing_copy"] != "Default copy" {
		t.Fatalf("default product payload = %#v", defaultPayload)
	}
	if _, ok := defaultPayload["translations"]; ok {
		t.Fatalf("default product payload must stay separate from localized updates: %#v", defaultPayload)
	}
	if path := nestedTranslationsPath(defaultPayload, "product"); path != "" {
		t.Fatalf("default product payload contains nested translations at %s: %#v", path, defaultPayload)
	}
	if nlPayload["name"] != "Hemd" || nlPayload["description"] != "Nederlandse beschrijving" ||
		nlPayload["marketing_copy"] != "Nederlandse tekst" || nlPayload["seo_title"] != "Nederlandse SEO" ||
		nlPayload["slug"] != "shirt" {
		t.Fatalf("localized product fields lost: %#v", nlPayload)
	}
	if nlPayload["variant_name"] != "Maat" {
		t.Fatalf("localized variant_name = %#v", nlPayload["variant_name"])
	}
	variants := nlPayload["variants"].([]any)
	small := variants[0].(map[string]any)
	large := variants[1].(map[string]any)
	if small["id"] != "target-small" || small["label"] != "Klein" ||
		large["id"] != "target-large" || large["label"] != "Groot" {
		t.Fatalf("localized variants were not matched by SKU: %#v", variants)
	}
	assertPayloadOmits(t, nlPayload, "internal_code", "price", "status")
	if _, ok := small["price"]; ok {
		t.Fatalf("localized variant payload overwrites price: %#v", small)
	}
	if len(result.Items) != 1 || len(result.Items[0].Localized) != 1 ||
		result.Items[0].Localized[0].Locale != "nl" || result.Items[0].Localized[0].Action != "update" {
		t.Fatalf("localized result actions = %#v", result.Items)
	}
}

func TestCopyProductsMapsMismatchedDefaultLocales(t *testing.T) {
	var defaultPayload, enPayload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		site := r.Header.Get("X-Nimbu-Site")
		locale := r.URL.Query().Get("content_locale")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/products/customizations" && site == "source":
			_, _ = w.Write([]byte(`[{"name":"marketing_copy","type":"text","localized":true}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/sites/source/settings" && site == "source":
			_, _ = w.Write([]byte(`{"default_locale":"en","locales":["en","nl"]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/sites/target/settings" && site == "target":
			_, _ = w.Write([]byte(`{"default_locale":"nl","locales":["en","nl"]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/products" && site == "source" && locale == "":
			_, _ = w.Write([]byte(`[{"id":"source-product","slug":"shirt","name":"Shirt","description":"English","marketing_copy":"English copy","price":25}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/products" && site == "source" && locale == "nl":
			_, _ = w.Write([]byte(`[{"id":"source-product","slug":"hemd","name":"Hemd","description":"Nederlands","marketing_copy":"Nederlandse tekst","price":999}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/products" && site == "target" && locale == "":
			_, _ = w.Write([]byte(`[{"id":"target-product","slug":"hemd","name":"Oud","price":25}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/products" && site == "target" && locale == "en":
			_, _ = w.Write([]byte(`[{"id":"target-product","slug":"shirt","name":"Old","price":25}]`))
		case r.Method == http.MethodPut && r.URL.Path == "/products/target-product" && site == "target" && locale == "":
			if err := json.NewDecoder(r.Body).Decode(&defaultPayload); err != nil {
				t.Fatalf("decode target-default payload: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"target-product","slug":"hemd"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/products/target-product" && site == "target" && locale == "en":
			if err := json.NewDecoder(r.Body).Decode(&enPayload); err != nil {
				t.Fatalf("decode English localized payload: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"target-product","slug":"hemd"}`))
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
	if defaultPayload["name"] != "Hemd" || defaultPayload["slug"] != "hemd" ||
		defaultPayload["marketing_copy"] != "Nederlandse tekst" || defaultPayload["price"] != float64(25) {
		t.Fatalf("target-default payload did not overlay Dutch localized fields onto base data: %#v", defaultPayload)
	}
	if enPayload["name"] != "Shirt" || enPayload["slug"] != "shirt" ||
		enPayload["marketing_copy"] != "English copy" {
		t.Fatalf("source default was not copied as target localized content: %#v", enPayload)
	}
	if len(result.Items) != 1 || len(result.Items[0].Localized) != 1 || result.Items[0].Localized[0].Locale != "en" {
		t.Fatalf("localized actions = %#v", result.Items)
	}
}

func nestedTranslationsPath(value any, path string) string {
	switch typed := value.(type) {
	case map[string]any:
		if _, ok := typed["translations"]; ok {
			return path + ".translations"
		}
		for key, child := range typed {
			if found := nestedTranslationsPath(child, path+"."+key); found != "" {
				return found
			}
		}
	case []any:
		for i, child := range typed {
			if found := nestedTranslationsPath(child, fmt.Sprintf("%s[%d]", path, i)); found != "" {
				return found
			}
		}
	}
	return ""
}

func TestCopyProductsDryRunPlansLocalizedUpdatesWithoutWrites(t *testing.T) {
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
		case r.URL.Path == "/products/customizations" && site == "source":
			_, _ = w.Write([]byte(`[{"name":"marketing_copy","type":"text","localized":true}]`))
		case r.URL.Path == "/sites/source/settings" && site == "source":
			_, _ = w.Write([]byte(`{"default_locale":"en","locales":["en","nl"]}`))
		case r.URL.Path == "/sites/target/settings" && site == "target":
			_, _ = w.Write([]byte(`{"default_locale":"en","locales":["en","nl"]}`))
		case r.URL.Path == "/products" && site == "source" && locale == "":
			_, _ = w.Write([]byte(`[{"id":"source-product","slug":"shirt","name":"Shirt","marketing_copy":"Default"}]`))
		case r.URL.Path == "/products" && site == "source" && locale == "nl":
			_, _ = w.Write([]byte(`[{"id":"source-product","slug":"shirt","name":"Hemd","marketing_copy":"Nederlands"}]`))
		case r.URL.Path == "/products" && site == "target" && locale == "":
			_, _ = w.Write([]byte(`[{"id":"target-product","slug":"shirt","name":"Old"}]`))
		case r.URL.Path == "/products" && site == "target" && locale == "nl":
			_, _ = w.Write([]byte(`[{"id":"target-product","slug":"shirt","name":"Oud","marketing_copy":"Oud"}]`))
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
		t.Fatalf("dry-run performed %d writes", writes)
	}
	if len(result.Items) != 1 || result.Items[0].Action != "dry-run:update" ||
		len(result.Items[0].Localized) != 1 || result.Items[0].Localized[0].Action != "dry-run:update" {
		t.Fatalf("dry-run plan = %#v", result.Items)
	}
}

func TestCopyProductsAbortsWhenLocaleDiscoveryFails(t *testing.T) {
	var writes int
	srv := productWarningServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodGet {
			writes++
			_, _ = w.Write([]byte(`{}`))
			return true
		}
		if r.URL.Path == "/sites/target/settings" && r.Header.Get("X-Nimbu-Site") == "target" {
			http.Error(w, `{"message":"settings unavailable"}`, http.StatusServiceUnavailable)
			return true
		}
		return false
	})
	defer srv.Close()

	result, _, err := CopyProducts(
		context.Background(),
		api.New(srv.URL, "").WithSite("source"),
		api.New(srv.URL, "").WithSite("target"),
		SiteRef{Site: "source"},
		SiteRef{Site: "target"},
		ProductCopyOptions{AllowErrors: true},
	)
	if err == nil || !strings.Contains(err.Error(), "target site locales fetch failed") {
		t.Fatalf("CopyProducts error = %v", err)
	}
	if len(result.Items) != 0 || writes != 0 {
		t.Fatalf("product copy continued with unresolved locale settings: items=%#v writes=%d", result.Items, writes)
	}
	if len(result.Warnings) == 0 || !strings.Contains(result.Warnings[0], "target site locales fetch failed") {
		t.Fatalf("locale discovery warnings = %#v", result.Warnings)
	}
}

func TestCopyProductsRejectsInferredDefaultLocale(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/products/customizations":
			_, _ = w.Write([]byte(`[]`))
		case "/sites/source/settings":
			_, _ = w.Write([]byte(`{"locales":["en","nl"]}`))
		case "/sites/target/settings":
			_, _ = w.Write([]byte(`{"default_locale":"en","locales":["en","nl"]}`))
		default:
			t.Fatalf("unexpected request after ambiguous locale settings: %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	_, _, err := CopyProducts(
		context.Background(),
		api.New(srv.URL, "").WithSite("source"),
		api.New(srv.URL, "").WithSite("target"),
		SiteRef{Site: "source"},
		SiteRef{Site: "target"},
		ProductCopyOptions{},
	)
	if err == nil || !strings.Contains(err.Error(), "source site explicit default_locale is required") {
		t.Fatalf("CopyProducts error = %v", err)
	}
}

func TestCopyProductsAllowErrorsWarnsWhenLocalizedFetchFails(t *testing.T) {
	srv := productWarningServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/products" && r.Header.Get("X-Nimbu-Site") == "source" && r.URL.Query().Get("content_locale") == "nl" {
			http.Error(w, `{"message":"localized products unavailable"}`, http.StatusServiceUnavailable)
			return true
		}
		return false
	})
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
	if len(result.Warnings) == 0 || !strings.Contains(strings.Join(result.Warnings, "\n"), "source products locale=nl fetch failed") {
		t.Fatalf("localized fetch warnings = %#v", result.Warnings)
	}
}

func TestLocalizedProductVariantsSkipsAmbiguousSKUs(t *testing.T) {
	source := []any{
		map[string]any{"sku": "S", "label": "Small"},
		map[string]any{"sku": "", "label": "Blank"},
		map[string]any{"sku": "DUP", "label": "Duplicate one"},
		map[string]any{"sku": "DUP", "label": "Duplicate two"},
		map[string]any{"sku": "MISSING", "label": "Missing"},
		map[string]any{"sku": "TARGET-DUP", "label": "Target duplicate"},
	}
	target := []any{
		map[string]any{"id": "target-small", "sku": "S"},
		map[string]any{"id": "target-dup-1", "sku": "TARGET-DUP"},
		map[string]any{"id": "target-dup-2", "sku": "TARGET-DUP"},
	}

	variants, warnings := localizedProductVariants(source, target)
	if len(variants) != 1 {
		t.Fatalf("safe localized variants = %#v, want only SKU S", variants)
	}
	variant := variants[0].(map[string]any)
	if variant["id"] != "target-small" || variant["label"] != "Small" {
		t.Fatalf("safe localized variant = %#v", variant)
	}
	joined := strings.Join(warnings, "\n")
	for _, want := range []string{"blank SKU", `"DUP" is duplicated`, `"MISSING" is missing`, `"TARGET-DUP" is duplicated`} {
		if !strings.Contains(joined, want) {
			t.Errorf("warnings %q do not contain %q", joined, want)
		}
	}
}

func TestCopyLocalizedProductPreservesWarningsWhenUnsafeVariantsEmptyPayload(t *testing.T) {
	source := map[string]map[string]map[string]any{
		"nl": {
			"source-product": {
				"variants": []any{
					map[string]any{"sku": "", "label": "Klein"},
				},
			},
		},
	}

	items, warnings, err := copyLocalizedProduct(
		context.Background(),
		nil,
		nil,
		"source-product",
		"target-product",
		map[string]any{"variants": []any{}},
		schemaInfo{},
		[]string{"nl"},
		source,
		nil,
		false,
		ProductCopyOptions{DryRun: true},
	)
	if err != nil {
		t.Fatalf("copyLocalizedProduct error = %v", err)
	}
	if len(items) != 1 || items[0].Action != "skip:error" {
		t.Fatalf("localized empty-payload action = %#v", items)
	}
	if !strings.Contains(strings.Join(warnings, "\n"), "blank SKU") ||
		!strings.Contains(strings.Join(items[0].Warnings, "\n"), "blank SKU") {
		t.Fatalf("unsafe SKU warnings lost: result=%#v item=%#v", warnings, items)
	}
}

func TestCopyProductsAllowErrorsMarksLocalizedUpdateFailure(t *testing.T) {
	srv := productWarningServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPut && r.URL.Path == "/products/target-product" &&
			r.Header.Get("X-Nimbu-Site") == "target" && r.URL.Query().Get("content_locale") == "nl" {
			http.Error(w, `{"message":"localized update rejected"}`, http.StatusUnprocessableEntity)
			return true
		}
		return false
	})
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
	if len(result.Items) != 1 || len(result.Items[0].Localized) != 1 ||
		result.Items[0].Localized[0].Action != "skip:error" {
		t.Fatalf("localized failure action = %#v", result.Items)
	}
	if !strings.Contains(strings.Join(result.Warnings, "\n"), "localized update rejected") {
		t.Fatalf("localized failure warnings = %#v", result.Warnings)
	}
}

func TestCopyProductsFailsWhenTargetDefaultLocaleIsUnavailableOnSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/products/customizations":
			_, _ = w.Write([]byte(`[]`))
		case "/sites/source/settings":
			_, _ = w.Write([]byte(`{"default_locale":"en","locales":["en"]}`))
		case "/sites/target/settings":
			_, _ = w.Write([]byte(`{"default_locale":"nl","locales":["en","nl"]}`))
		default:
			t.Fatalf("unexpected request after locale validation: %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	_, _, err := CopyProducts(
		context.Background(),
		api.New(srv.URL, "").WithSite("source"),
		api.New(srv.URL, "").WithSite("target"),
		SiteRef{Site: "source"},
		SiteRef{Site: "target"},
		ProductCopyOptions{},
	)
	if err == nil || !strings.Contains(err.Error(), `target default locale "nl" is not available on source site`) {
		t.Fatalf("error = %v", err)
	}
}

func productWarningServer(t *testing.T, override func(http.ResponseWriter, *http.Request) bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if override(w, r) {
			return
		}
		site := r.Header.Get("X-Nimbu-Site")
		locale := r.URL.Query().Get("content_locale")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/products/customizations" && site == "source":
			_, _ = w.Write([]byte(`[{"name":"marketing_copy","type":"text","localized":true}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/sites/source/settings" && site == "source":
			_, _ = w.Write([]byte(`{"default_locale":"en","locales":["en","nl"]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/sites/target/settings" && site == "target":
			_, _ = w.Write([]byte(`{"default_locale":"en","locales":["en","nl"]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/products" && site == "source" && locale == "":
			_, _ = w.Write([]byte(`[{"id":"source-product","slug":"shirt","name":"Shirt","marketing_copy":"Default"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/products" && site == "source" && locale == "nl":
			_, _ = w.Write([]byte(`[{"id":"source-product","slug":"shirt","name":"Hemd","marketing_copy":"Nederlands"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/products" && site == "target":
			_, _ = w.Write([]byte(`[{"id":"target-product","slug":"shirt","name":"Old"}]`))
		case r.Method == http.MethodPut && r.URL.Path == "/products/target-product" && site == "target":
			_, _ = w.Write([]byte(`{"id":"target-product","slug":"shirt"}`))
		default:
			http.NotFound(w, r)
		}
	}))
}
