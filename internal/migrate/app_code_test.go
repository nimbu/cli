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

func TestCopyAppCodeCreatesUpdatesAndSkipsMissingTargetApps(t *testing.T) {
	var ops []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		site := r.Header.Get("X-Nimbu-Site")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/apps" && site == "source":
			_, _ = w.Write([]byte(`[
				{"key":"storefront","name":"Storefront"},
				{"key":"missing","name":"Missing"}
			]`))
		case r.Method == http.MethodGet && r.URL.Path == "/apps" && site == "target":
			_, _ = w.Write([]byte(`[{"key":"storefront","name":"Storefront"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/apps/storefront/code" && site == "source":
			_, _ = w.Write([]byte(`[
				{"name":"main.js","code":"source main"},
				{"name":"shared.js","code":"source shared"}
			]`))
		case r.Method == http.MethodGet && r.URL.Path == "/apps/storefront/code" && site == "target":
			_, _ = w.Write([]byte(`[{"name":"main.js","code":"target main"}]`))
		case r.Method == http.MethodPut && r.URL.Path == "/apps/storefront/code/main.js" && site == "target":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode put: %v", err)
			}
			ops = append(ops, "update:main.js:"+body["code"].(string))
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodPost && r.URL.Path == "/apps/storefront/code" && site == "target":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode post: %v", err)
			}
			ops = append(ops, "create:"+body["name"].(string)+":"+body["code"].(string))
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	result, err := CopyAppCode(
		context.Background(),
		api.New(srv.URL, "token").WithSite("source"),
		api.New(srv.URL, "token").WithSite("target"),
		SiteRef{Site: "source"},
		SiteRef{Site: "target"},
		AppCodeCopyOptions{},
	)
	if err != nil {
		t.Fatalf("copy app code: %v", err)
	}

	if got := strings.Join(ops, ","); got != "update:main.js:source main,create:shared.js:source shared" {
		t.Fatalf("ops = %q", got)
	}
	if len(result.Items) != 2 {
		t.Fatalf("items = %#v, want 2 copied files", result.Items)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].AppKey != "missing" {
		t.Fatalf("skipped = %#v, want missing app", result.Skipped)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "missing") {
		t.Fatalf("warnings = %#v, want missing-app warning", result.Warnings)
	}
}

func TestCopyAppCodeDryRunDoesNotWrite(t *testing.T) {
	var writes int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		site := r.Header.Get("X-Nimbu-Site")
		if r.Method == http.MethodPost || r.Method == http.MethodPut {
			writes++
			http.Error(w, "unexpected write", http.StatusInternalServerError)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/apps":
			_, _ = w.Write([]byte(`[{"key":"storefront","name":"Storefront"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/apps/storefront/code" && site == "source":
			_, _ = w.Write([]byte(`[{"name":"main.js","code":"source main"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/apps/storefront/code" && site == "target":
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	result, err := CopyAppCode(
		context.Background(),
		api.New(srv.URL, "token").WithSite("source"),
		api.New(srv.URL, "token").WithSite("target"),
		SiteRef{Site: "source"},
		SiteRef{Site: "target"},
		AppCodeCopyOptions{DryRun: true},
	)
	if err != nil {
		t.Fatalf("copy app code: %v", err)
	}
	if writes != 0 {
		t.Fatalf("writes = %d, want 0", writes)
	}
	if len(result.Items) != 1 || result.Items[0].Action != "dry-run:create" {
		t.Fatalf("items = %#v, want dry-run create", result.Items)
	}
}

func TestCopyAppCodeRejectsUnsafeRemoteNames(t *testing.T) {
	tests := []struct {
		name        string
		sourceFiles string
		targetFiles string
		wantContext string
	}{
		{
			name:        "source",
			sourceFiles: `[{"name":"nested/../secret.js","code":"leak"}]`,
			targetFiles: `[]`,
			wantContext: "source app code storefront",
		},
		{
			name:        "source absolute after trim",
			sourceFiles: `[{"name":" /secret.js","code":"leak"}]`,
			targetFiles: `[]`,
			wantContext: "source app code storefront",
		},
		{
			name:        "target",
			sourceFiles: `[{"name":"main.js","code":"main"}]`,
			targetFiles: `[{"name":"nested/../secret.js","code":"leak"}]`,
			wantContext: "target app code storefront",
		},
		{
			name:        "target backslash absolute",
			sourceFiles: `[{"name":"main.js","code":"main"}]`,
			targetFiles: `[{"name":"\\secret.js","code":"leak"}]`,
			wantContext: "target app code storefront",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var writes int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				site := r.Header.Get("X-Nimbu-Site")
				if r.Method == http.MethodPost || r.Method == http.MethodPut {
					writes++
					http.Error(w, "unexpected write", http.StatusInternalServerError)
					return
				}
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/apps":
					_, _ = w.Write([]byte(`[{"key":"storefront","name":"Storefront"}]`))
				case r.Method == http.MethodGet && r.URL.Path == "/apps/storefront/code" && site == "source":
					_, _ = w.Write([]byte(tt.sourceFiles))
				case r.Method == http.MethodGet && r.URL.Path == "/apps/storefront/code" && site == "target":
					_, _ = w.Write([]byte(tt.targetFiles))
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			_, err := CopyAppCode(
				context.Background(),
				api.New(srv.URL, "token").WithSite("source"),
				api.New(srv.URL, "token").WithSite("target"),
				SiteRef{Site: "source"},
				SiteRef{Site: "target"},
				AppCodeCopyOptions{},
			)
			if err == nil || !strings.Contains(err.Error(), tt.wantContext) || !strings.Contains(err.Error(), "unsafe app code file name") {
				t.Fatalf("expected unsafe filename error with context %q, got %v", tt.wantContext, err)
			}
			if writes != 0 {
				t.Fatalf("writes = %d, want 0", writes)
			}
		})
	}
}

func TestCopySiteIncludesCloudCodeByDefault(t *testing.T) {
	var appWrites []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		site := r.Header.Get("X-Nimbu-Site")
		if r.Method == http.MethodPost && r.URL.Path == "/apps/storefront/code" && site == "target" {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode app code post: %v", err)
			}
			appWrites = append(appWrites, body["name"].(string)+":"+body["code"].(string))
			_, _ = w.Write([]byte(`{}`))
			return
		}
		writeEmptySiteCopyResponse(w, r)
	}))
	defer srv.Close()

	result, err := CopySite(
		context.Background(),
		api.New(srv.URL, "token").WithSite("source"),
		api.New(srv.URL, "token").WithSite("target"),
		SiteRef{Site: "source"},
		SiteRef{Site: "target"},
		SiteCopyOptions{},
	)
	if err != nil {
		t.Fatalf("copy site: %v", err)
	}
	if got := strings.Join(appWrites, ","); got != "main.js:source main" {
		t.Fatalf("app writes = %q, want main.js:source main", got)
	}
	if len(result.CloudCode.Items) != 1 || result.CloudCode.Items[0].Name != "main.js" {
		t.Fatalf("cloud code result = %#v", result.CloudCode)
	}
}

func TestCopySiteCanSkipCloudCode(t *testing.T) {
	var appCalls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/apps") {
			appCalls++
			http.Error(w, "cloud code should be skipped", http.StatusInternalServerError)
			return
		}
		writeEmptySiteCopyResponse(w, r)
	}))
	defer srv.Close()

	_, err := CopySite(
		context.Background(),
		api.New(srv.URL, "token").WithSite("source"),
		api.New(srv.URL, "token").WithSite("target"),
		SiteRef{Site: "source"},
		SiteRef{Site: "target"},
		SiteCopyOptions{SkipCloudCode: true},
	)
	if err != nil {
		t.Fatalf("copy site: %v", err)
	}
	if appCalls != 0 {
		t.Fatalf("app calls = %d, want 0", appCalls)
	}
}

func writeEmptySiteCopyResponse(w http.ResponseWriter, r *http.Request) {
	site := r.Header.Get("X-Nimbu-Site")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/channels":
		_, _ = w.Write([]byte(`[]`))
	case r.Method == http.MethodGet && r.URL.Path == "/uploads":
		_, _ = w.Write([]byte(`[]`))
	case r.Method == http.MethodGet && r.URL.Path == "/customers/customizations":
		_, _ = w.Write([]byte(`[]`))
	case r.Method == http.MethodGet && r.URL.Path == "/products/customizations":
		_, _ = w.Write([]byte(`[]`))
	case r.Method == http.MethodPost && (r.URL.Path == "/customers/customizations" || r.URL.Path == "/products/customizations"):
		_, _ = w.Write([]byte(`[]`))
	case r.Method == http.MethodGet && r.URL.Path == "/roles":
		_, _ = w.Write([]byte(`[]`))
	case r.Method == http.MethodGet && r.URL.Path == "/products":
		_, _ = w.Write([]byte(`[]`))
	case r.Method == http.MethodGet && r.URL.Path == "/collections":
		_, _ = w.Write([]byte(`[]`))
	case r.Method == http.MethodGet && r.URL.Path == "/themes":
		_, _ = w.Write([]byte(`[{"id":"t1","name":"Storefront","active":true}]`))
	case r.Method == http.MethodGet && r.URL.Path == "/themes/t1":
		_, _ = w.Write([]byte(`{"assets":[],"layouts":[],"snippets":[],"templates":[]}`))
	case r.Method == http.MethodGet && r.URL.Path == "/pages":
		_, _ = w.Write([]byte(`[]`))
	case r.Method == http.MethodGet && r.URL.Path == "/menus":
		_, _ = w.Write([]byte(`[]`))
	case r.Method == http.MethodGet && r.URL.Path == "/blogs":
		_, _ = w.Write([]byte(`[]`))
	case r.Method == http.MethodGet && r.URL.Path == "/notifications":
		_, _ = w.Write([]byte(`[]`))
	case r.Method == http.MethodGet && r.URL.Path == "/redirects":
		_, _ = w.Write([]byte(`[]`))
	case r.Method == http.MethodGet && r.URL.Path == "/translations":
		_, _ = w.Write([]byte(`[]`))
	case r.Method == http.MethodGet && r.URL.Path == "/apps" && site == "source":
		_, _ = w.Write([]byte(`[{"key":"storefront","name":"Storefront"}]`))
	case r.Method == http.MethodGet && r.URL.Path == "/apps" && site == "target":
		_, _ = w.Write([]byte(`[{"key":"storefront","name":"Storefront"}]`))
	case r.Method == http.MethodGet && r.URL.Path == "/apps/storefront/code" && site == "source":
		_, _ = w.Write([]byte(`[{"name":"main.js","code":"source main"}]`))
	case r.Method == http.MethodGet && r.URL.Path == "/apps/storefront/code" && site == "target":
		_, _ = w.Write([]byte(`[]`))
	default:
		http.NotFound(w, r)
	}
}
