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

func TestCopyConsentConfigPreservesLocalizedMapsAndUnknownFields(t *testing.T) {
	var put map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		site := r.Header.Get("X-Nimbu-Site")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/settings/consent" && site == "source":
			_, _ = w.Write([]byte(`{
				"privacy_policy_kind":"url",
				"privacy_policy_url":{"en":"/privacy","nl":"/privacybeleid"},
				"purposes":[{"key":"analytics","description":{"en":"Analytics","nl":"Analyse"}}],
				"applications":[{"key":"chat","title":{"en":"Chat","nl":"Praat"},"description":{"en":"Help","nl":"Hulp"},"placeholder":{"en":"Ask","nl":"Vraag"}}],
				"cookies":[{"id":"cookie-1","name":"session","unknown":{"nested":true}}],
				"future_field":{"keep":"me"},
				"future_counter":9007199254740993
			}`))
		case r.Method == http.MethodPut && r.URL.Path == "/settings/consent" && site == "target":
			decoder := json.NewDecoder(r.Body)
			decoder.UseNumber()
			if err := decoder.Decode(&put); err != nil {
				t.Fatal(err)
			}
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	result, err := CopyConsentConfig(context.Background(),
		api.New(srv.URL, "").WithSite("source"),
		api.New(srv.URL, "").WithSite("target"),
		SiteRef{Site: "source"}, SiteRef{Site: "target"}, ConsentCopyOptions{})
	if err != nil {
		t.Fatalf("copy consent: %v", err)
	}
	if result.Action != "update" {
		t.Fatalf("action = %q", result.Action)
	}
	urls := put["privacy_policy_url"].(map[string]any)
	purpose := put["purposes"].([]any)[0].(map[string]any)
	app := put["applications"].([]any)[0].(map[string]any)
	if urls["nl"] != "/privacybeleid" ||
		purpose["description"].(map[string]any)["nl"] != "Analyse" ||
		app["title"].(map[string]any)["nl"] != "Praat" ||
		app["description"].(map[string]any)["nl"] != "Hulp" ||
		app["placeholder"].(map[string]any)["nl"] != "Vraag" ||
		put["future_field"].(map[string]any)["keep"] != "me" ||
		put["future_counter"] != json.Number("9007199254740993") {
		t.Fatalf("PUT body lost fields: %#v", put)
	}
}

func TestCopyConsentConfigRemapsPrivacyPolicyPageByFullpath(t *testing.T) {
	var put map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		site := r.Header.Get("X-Nimbu-Site")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/settings/consent" && site == "source":
			_, _ = w.Write([]byte(`{"privacy_policy_kind":"page","privacy_policy_page_id":"source-page","privacy_policy_url":{"nl":"/privacybeleid"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/pages" && site == "source":
			_, _ = w.Write([]byte(`[{"id":"source-page","fullpath":"legal/privacy"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/pages" && site == "target":
			_, _ = w.Write([]byte(`[{"id":"target-page","fullpath":"legal/privacy"}]`))
		case r.Method == http.MethodPut && r.URL.Path == "/settings/consent" && site == "target":
			_ = json.NewDecoder(r.Body).Decode(&put)
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	_, err := CopyConsentConfig(context.Background(),
		api.New(srv.URL, "").WithSite("source"),
		api.New(srv.URL, "").WithSite("target"),
		SiteRef{Site: "source"}, SiteRef{Site: "target"}, ConsentCopyOptions{})
	if err != nil {
		t.Fatalf("copy consent: %v", err)
	}
	if put["privacy_policy_page_id"] != "target-page" {
		t.Fatalf("privacy page ID = %#v, want target-page", put["privacy_policy_page_id"])
	}
}

func TestCopyConsentConfigAbortsBeforePutWhenPrivacyPageCannotBeResolved(t *testing.T) {
	tests := []struct {
		name       string
		config     string
		sourceList string
		targetList string
		dryRun     bool
		want       string
	}{
		{name: "page ID field missing", config: `{"privacy_policy_kind":"page"}`, want: "nonempty string privacy_policy_page_id"},
		{name: "page ID empty", config: `{"privacy_policy_kind":"page","privacy_policy_page_id":""}`, want: "nonempty string privacy_policy_page_id"},
		{name: "page ID not a string", config: `{"privacy_policy_kind":"page","privacy_policy_page_id":123}`, want: "nonempty string privacy_policy_page_id"},
		{name: "source ID missing", config: `{"privacy_policy_kind":"page","privacy_policy_page_id":"source-page"}`, sourceList: `[{"id":"other","fullpath":"legal/privacy"}]`, targetList: `[]`, want: "source privacy policy page ID"},
		{name: "target fullpath missing", config: `{"privacy_policy_kind":"page","privacy_policy_page_id":"source-page"}`, sourceList: `[{"id":"source-page","fullpath":"legal/privacy"}]`, targetList: `[]`, want: `target privacy policy page "legal/privacy"`},
		{name: "standalone dry-run still requires target", config: `{"privacy_policy_kind":"page","privacy_policy_page_id":"source-page"}`, sourceList: `[{"id":"source-page","fullpath":"legal/privacy"}]`, targetList: `[]`, dryRun: true, want: `target privacy policy page "legal/privacy"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var puts int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				site := r.Header.Get("X-Nimbu-Site")
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/settings/consent" && site == "source":
					_, _ = w.Write([]byte(tt.config))
				case r.Method == http.MethodGet && r.URL.Path == "/pages" && site == "source":
					_, _ = w.Write([]byte(tt.sourceList))
				case r.Method == http.MethodGet && r.URL.Path == "/pages" && site == "target":
					_, _ = w.Write([]byte(tt.targetList))
				case r.Method == http.MethodPut:
					puts++
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			_, err := CopyConsentConfig(context.Background(),
				api.New(srv.URL, "").WithSite("source"),
				api.New(srv.URL, "").WithSite("target"),
				SiteRef{Site: "source"}, SiteRef{Site: "target"}, ConsentCopyOptions{DryRun: tt.dryRun})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
			if puts != 0 {
				t.Fatalf("PUT calls = %d, want 0", puts)
			}
		})
	}
}

func TestCopyConsentConfigDryRunRemapsWithoutPut(t *testing.T) {
	var puts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		site := r.Header.Get("X-Nimbu-Site")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/settings/consent" && site == "source":
			_, _ = w.Write([]byte(`{"privacy_policy_kind":"page","privacy_policy_page_id":"source-page"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/pages" && site == "source":
			_, _ = w.Write([]byte(`[{"id":"source-page","fullpath":"privacy"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/pages" && site == "target":
			_, _ = w.Write([]byte(`[{"id":"target-page","fullpath":"privacy"}]`))
		case r.Method == http.MethodPut:
			puts++
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	result, err := CopyConsentConfig(context.Background(),
		api.New(srv.URL, "").WithSite("source"),
		api.New(srv.URL, "").WithSite("target"),
		SiteRef{Site: "source"}, SiteRef{Site: "target"}, ConsentCopyOptions{DryRun: true})
	if err != nil {
		t.Fatalf("dry-run copy consent: %v", err)
	}
	if result.Action != "dry-run:update" || puts != 0 {
		t.Fatalf("result = %#v, PUT calls = %d", result, puts)
	}
}

func TestCopySiteRunsConsentImmediatelyAfterPagesAndIncludesResult(t *testing.T) {
	var sequence []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		site := r.Header.Get("X-Nimbu-Site")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/pages" && site == "source":
			sequence = append(sequence, "pages")
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && r.URL.Path == "/settings/consent" && site == "source":
			sequence = append(sequence, "consent:get")
			_, _ = w.Write([]byte(`{"privacy_policy_kind":"url","privacy_policy_url":{"nl":"/privacybeleid"}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/settings/consent" && site == "target":
			sequence = append(sequence, "consent:put")
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodGet && r.URL.Path == "/menus" && site == "source":
			sequence = append(sequence, "menus")
			_, _ = w.Write([]byte(`[]`))
		default:
			writeEmptySiteCopyResponse(w, r)
		}
	}))
	defer srv.Close()

	result, err := CopySite(
		context.Background(),
		api.New(srv.URL, "").WithSite("source"),
		api.New(srv.URL, "").WithSite("target"),
		SiteRef{Site: "source"},
		SiteRef{Site: "target"},
		SiteCopyOptions{SkipCloudCode: true},
	)
	if err != nil {
		t.Fatalf("copy site: %v", err)
	}
	if result.Consent.Action != "update" {
		t.Fatalf("consent result = %#v", result.Consent)
	}
	if len(sequence) < 4 || strings.Join(sequence[:4], ",") != "pages,consent:get,consent:put,menus" {
		t.Fatalf("stage request order = %q, want pages,consent:get,consent:put,menus first", strings.Join(sequence, ","))
	}
}

func TestCopySiteDryRunAllowsConsentPagePlannedByPagesStage(t *testing.T) {
	var puts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		site := r.Header.Get("X-Nimbu-Site")
		switch {
		case r.Method == http.MethodPut:
			puts++
			http.Error(w, "dry-run must not write", http.StatusInternalServerError)
		case r.Method == http.MethodGet && r.URL.Path == "/pages" && site == "source":
			_, _ = w.Write([]byte(`[{"id":"source-page","fullpath":"legal/privacy"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/pages" && site == "target":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && r.URL.Path == "/pages/legal/privacy" && site == "source":
			_, _ = w.Write([]byte(`{"id":"source-page","fullpath":"legal/privacy","title":"Privacy"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/pages/legal/privacy" && site == "target":
			http.NotFound(w, r)
		case r.Method == http.MethodGet && r.URL.Path == "/settings/consent" && site == "source":
			_, _ = w.Write([]byte(`{"privacy_policy_kind":"page","privacy_policy_page_id":"source-page"}`))
		default:
			writeEmptySiteCopyResponse(w, r)
		}
	}))
	defer srv.Close()

	result, err := CopySite(
		context.Background(),
		api.New(srv.URL, "").WithSite("source"),
		api.New(srv.URL, "").WithSite("target"),
		SiteRef{Site: "source"},
		SiteRef{Site: "target"},
		SiteCopyOptions{DryRun: true, SkipCloudCode: true},
	)
	if err != nil {
		t.Fatalf("dry-run copy site: %v", err)
	}
	if result.Consent.Action != "dry-run:update" {
		t.Fatalf("consent result = %#v", result.Consent)
	}
	if len(result.Pages.Items) != 1 || result.Pages.Items[0].Action != "dry-run:create" {
		t.Fatalf("pages result = %#v", result.Pages)
	}
	if puts != 0 {
		t.Fatalf("PUT calls = %d, want 0", puts)
	}
}
