package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

// newAppsTestContextWithOut behaves like newAppsTestContext but exposes the
// stdout builder so tests can assert on rendered output.
func newAppsTestContextWithOut(t *testing.T, apiURL string) (context.Context, *strings.Builder) {
	t.Helper()
	t.Setenv("NIMBU_TOKEN", "test-token")

	flags := &RootFlags{APIURL: apiURL, Site: "demo"}
	cfg := config.Defaults()
	cfg.DefaultSite = "demo"

	out := &strings.Builder{}
	ctx := context.Background()
	ctx = context.WithValue(ctx, rootFlagsKey{}, flags)
	ctx = context.WithValue(ctx, configKey{}, &cfg)
	ctx = output.WithMode(ctx, output.Mode{})
	ctx = output.WithWriter(ctx, &output.Writer{Out: out, Err: &strings.Builder{}, Mode: output.Mode{}, NoTTY: true})
	return ctx, out
}

type capturedRequest struct {
	method string
	path   string
	body   map[string]any
}

func appsWriteServer(t *testing.T, response string) (*httptest.Server, *capturedRequest) {
	t.Helper()
	captured := &capturedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.method = r.Method
		captured.path = r.URL.Path
		data, _ := io.ReadAll(r.Body)
		if len(data) > 0 {
			_ = json.Unmarshal(data, &captured.body)
		}
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(srv.Close)
	return srv, captured
}

func TestAppsCreateInternalWithScopes(t *testing.T) {
	srv, captured := appsWriteServer(t, `{"key":"my-app","name":"My App","internal":true,"install_scopes":["read_channels","write_channels"],"callback_url":"https://api.nimbu.io/apps/my-app/callback"}`)
	ctx, out := newAppsTestContextWithOut(t, srv.URL)

	cmd := &AppsCreateCmd{
		Name:     "My App",
		Internal: true,
		Scopes:   []string{"read_channels,write_channels"},
	}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo", APIURL: srv.URL}); err != nil {
		t.Fatalf("run create: %v", err)
	}

	if captured.method != http.MethodPost || captured.path != "/apps" {
		t.Fatalf("request = %s %s, want POST /apps", captured.method, captured.path)
	}
	if captured.body["name"] != "My App" {
		t.Fatalf("name = %v", captured.body["name"])
	}
	if captured.body["internal"] != true {
		t.Fatalf("internal = %v, want true", captured.body["internal"])
	}
	if captured.body["install"] != true {
		t.Fatalf("install = %v, want true (default for internal)", captured.body["install"])
	}
	scopes, ok := captured.body["install_scopes"].([]any)
	if !ok || len(scopes) != 2 || scopes[0] != "read_channels" || scopes[1] != "write_channels" {
		t.Fatalf("install_scopes = %#v", captured.body["install_scopes"])
	}
	if _, exists := captured.body["callback_url"]; exists {
		t.Fatalf("callback_url should not be sent for internal app: %#v", captured.body)
	}
	if !strings.Contains(out.String(), "my-app") {
		t.Fatalf("output missing key: %q", out.String())
	}
}

func TestAppsCreateExternalWithCallback(t *testing.T) {
	srv, captured := appsWriteServer(t, `{"key":"ext-app","name":"Ext","internal":false,"callback_url":"https://example.test/callback"}`)
	ctx, _ := newAppsTestContextWithOut(t, srv.URL)

	cmd := &AppsCreateCmd{
		Name:        "Ext",
		Internal:    false,
		URL:         "https://example.test",
		CallbackURL: "https://example.test/callback",
	}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo", APIURL: srv.URL}); err != nil {
		t.Fatalf("run create: %v", err)
	}

	if captured.method != http.MethodPost || captured.path != "/apps" {
		t.Fatalf("request = %s %s", captured.method, captured.path)
	}
	if captured.body["internal"] != false {
		t.Fatalf("internal = %v, want false", captured.body["internal"])
	}
	if captured.body["install"] != false {
		t.Fatalf("install = %v, want false (default for external)", captured.body["install"])
	}
	if captured.body["callback_url"] != "https://example.test/callback" {
		t.Fatalf("callback_url = %v", captured.body["callback_url"])
	}
	if captured.body["url"] != "https://example.test" {
		t.Fatalf("url = %v", captured.body["url"])
	}
}

func TestAppsCreateInternalWithCallbackErrors(t *testing.T) {
	ctx, _ := newAppsTestContextWithOut(t, "https://api.example.test")
	cmd := &AppsCreateCmd{
		Name:        "My App",
		Internal:    true,
		Scopes:      []string{"read_channels"},
		CallbackURL: "https://example.test/callback",
	}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), "callback-url") {
		t.Fatalf("expected callback-url error, got %v", err)
	}
}

func TestAppsCreateInternalMissingScopesErrors(t *testing.T) {
	ctx, _ := newAppsTestContextWithOut(t, "https://api.example.test")
	cmd := &AppsCreateCmd{
		Name:     "My App",
		Internal: true,
	}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), "scopes") {
		t.Fatalf("expected missing scopes error, got %v", err)
	}
}

func TestAppsUpdatePartialFields(t *testing.T) {
	srv, captured := appsWriteServer(t, `{"key":"my-app","name":"Renamed","internal":true,"install_scopes":["read_channels"]}`)
	ctx, _ := newAppsTestContextWithOut(t, srv.URL)

	newName := "Renamed"
	cmd := &AppsUpdateCmd{
		App:  "my-app",
		Name: &newName,
	}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo", APIURL: srv.URL}); err != nil {
		t.Fatalf("run update: %v", err)
	}

	if captured.method != http.MethodPatch || captured.path != "/apps/my-app" {
		t.Fatalf("request = %s %s, want PATCH /apps/my-app", captured.method, captured.path)
	}
	if captured.body["name"] != "Renamed" {
		t.Fatalf("name = %v", captured.body["name"])
	}
	if len(captured.body) != 1 {
		t.Fatalf("only provided fields should be sent, got %#v", captured.body)
	}
}

func TestAppsUpdateNoFieldsErrors(t *testing.T) {
	ctx, _ := newAppsTestContextWithOut(t, "https://api.example.test")
	cmd := &AppsUpdateCmd{App: "my-app"}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), "no fields to update") {
		t.Fatalf("expected no-fields error, got %v", err)
	}
}
