package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

func TestDomainsGetResolvesByDomainName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/domains":
			_, _ = w.Write([]byte(`[{"id":"d1","domain":"shop.example.com","primary":true}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, out, _ := newAdminWorkflowTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &DomainsGetCmd{Domain: "shop.example.com"}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run domains get: %v", err)
	}

	if !strings.Contains(out.String(), `"domain": "shop.example.com"`) {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestDomainsGetResolvesByDomainNameCaseInsensitive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/domains":
			_, _ = w.Write([]byte(`[{"id":"d1","domain":"shop.example.com","primary":true}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, out, _ := newAdminWorkflowTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &DomainsGetCmd{Domain: "Shop.Example.com"}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run domains get: %v", err)
	}

	if !strings.Contains(out.String(), `"domain": "shop.example.com"`) {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestDomainsUpdatePostsInlineAssignments(t *testing.T) {
	var gotPath string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/domains":
			_, _ = w.Write([]byte(`[{"id":"d1","domain":"shop.example.com"}]`))
		case "/domains/d1":
			gotPath = r.URL.Path
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"d1","domain":"shop.example.com","primary":false}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, _, _ := newAdminWorkflowTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &DomainsUpdateCmd{
		Domain:      "shop.example.com",
		Assignments: []string{"default_locale=nl", "redirect_domain=www.example.com"},
	}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run domains update: %v", err)
	}

	if gotPath != "/domains/d1" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if gotBody["default_locale"] != "nl" || gotBody["redirect_domain"] != "www.example.com" {
		t.Fatalf("unexpected body: %#v", gotBody)
	}
}

func TestDomainsMakePrimaryRequiresForce(t *testing.T) {
	ctx, _, _ := newAdminWorkflowTestContext(t, "https://api.example.test", output.Mode{})
	cmd := &DomainsMakePrimaryCmd{Domain: "shop.example.com"}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected force error, got %v", err)
	}
}

func TestDomainsUpdateRejectsUnknownFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/domains":
			_, _ = w.Write([]byte(`[{"id":"d1","domain":"shop.example.com"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, _, _ := newAdminWorkflowTestContext(t, srv.URL, output.Mode{})
	cmd := &DomainsUpdateCmd{
		Domain:      "shop.example.com",
		Assignments: []string{"default_locle=nl"},
	}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), `unknown field "default_locle"`) {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestSendersVerifyOwnershipResolvesByDomainName(t *testing.T) {
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/senders":
			_, _ = w.Write([]byte(`[{"id":"s1","domain":"mail.example.com","status":"pending"}]`))
		case "/senders/s1/verify_ownership":
			gotPath = r.URL.Path
			_, _ = w.Write([]byte(`{"id":"s1","domain":"mail.example.com","ownership_verified":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, _, _ := newAdminWorkflowTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &SendersVerifyOwnershipCmd{Sender: "mail.example.com"}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run senders verify-ownership: %v", err)
	}

	if gotPath != "/senders/s1/verify_ownership" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
}

func TestSendersGetJSONPreservesDNSRecordMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/senders":
			_, _ = w.Write([]byte(`[{
				"id":"s1",
				"domain":"mail.example.com",
				"status":"pending",
				"dns_records":[{"type":"TXT","name":"mail.example.com","value":"v=spf1","ttl":3600,"priority":10,"description":"SPF","required":true}]
			}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, out, _ := newAdminWorkflowTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &SendersGetCmd{Sender: "mail.example.com"}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run senders get: %v", err)
	}

	body := out.String()
	for _, needle := range []string{`"ttl": 3600`, `"priority": 10`, `"description": "SPF"`, `"required": true`} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected output to contain %s, got %s", needle, body)
		}
	}
}

func TestOrdersPayPostsActionEndpoint(t *testing.T) {
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		_, _ = w.Write([]byte(`{"status":"ok","state":"completed","paid":true}`))
	}))
	defer srv.Close()

	ctx, _, _ := newAdminWorkflowTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &OrdersPayCmd{Order: "1001"}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run orders pay: %v", err)
	}

	if gotPath != "/orders/1001/payment" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
}

func TestCustomersResetPasswordPostsActionEndpoint(t *testing.T) {
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	ctx, _, _ := newAdminWorkflowTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &CustomersResetPasswordCmd{Customer: "ana@example.com"}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run customers reset-password: %v", err)
	}

	if gotPath != "/customers/ana@example.com/reset_password" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
}

func TestChannelsEmptyRequiresExactConfirmation(t *testing.T) {
	ctx, _, _ := newAdminWorkflowTestContext(t, "https://api.example.test", output.Mode{})
	cmd := &ChannelsEmptyCmd{Channel: "news", Confirm: "other"}
	err := cmd.Run(ctx, &RootFlags{Site: "demo", Force: true})
	if err == nil || !strings.Contains(err.Error(), "--confirm") {
		t.Fatalf("expected confirm error, got %v", err)
	}
}

func TestChannelsEmptyPostsResolvedSlug(t *testing.T) {
	var gotPath string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/channels/507f1f77bcf86cd799439011":
			_, _ = w.Write([]byte(`{"id":"507f1f77bcf86cd799439011","slug":"news","name":"News"}`))
		case "/channels/507f1f77bcf86cd799439011/empty":
			gotPath = r.URL.Path
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			_, _ = w.Write([]byte(`{"status":"ok","message":"Empty channel job scheduled"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, _, _ := newAdminWorkflowTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &ChannelsEmptyCmd{
		Channel: "507f1f77bcf86cd799439011",
		Confirm: "news",
	}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo", Force: true}); err != nil {
		t.Fatalf("run channels empty: %v", err)
	}

	if gotPath != "/channels/507f1f77bcf86cd799439011/empty" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if gotBody["confirm"] != "news" {
		t.Fatalf("unexpected body: %#v", gotBody)
	}
}

func TestChannelsEmptyFallsBackToHexSlugWhenLookupMisses(t *testing.T) {
	slug := "507f1f77bcf86cd799439011"
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/channels/" + slug:
			if r.Method == http.MethodGet {
				http.NotFound(w, r)
				return
			}
		case "/channels/" + slug + "/empty":
			gotPath = r.URL.Path
			_, _ = w.Write([]byte(`{"status":"ok","message":"Empty channel job scheduled"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, _, _ := newAdminWorkflowTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &ChannelsEmptyCmd{Channel: slug, Confirm: slug}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo", Force: true}); err != nil {
		t.Fatalf("run channels empty: %v", err)
	}

	if gotPath != "/channels/"+slug+"/empty" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
}

func TestDomainsListPassesStandardListOptions(t *testing.T) {
	var gotQuery string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	ctx, _, _ := newAdminWorkflowTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &DomainsListCmd{All: true, QueryFlags: QueryFlags{Sort: "domain:desc", Fields: "id,domain"}}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run domains list: %v", err)
	}

	if !strings.Contains(gotQuery, "sort=-domain") || !strings.Contains(gotQuery, "fields=id%2Cdomain") {
		t.Fatalf("expected query options to be forwarded, got %q", gotQuery)
	}
}

func TestSendersListPassesStandardListOptions(t *testing.T) {
	var gotQuery string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	ctx, _, _ := newAdminWorkflowTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &SendersListCmd{All: true, QueryFlags: QueryFlags{Sort: "status:asc", Fields: "id,domain"}}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run senders list: %v", err)
	}

	if !strings.Contains(gotQuery, "sort=status") || !strings.Contains(gotQuery, "fields=id%2Cdomain") {
		t.Fatalf("expected query options to be forwarded, got %q", gotQuery)
	}
}

func newAdminWorkflowTestContext(t *testing.T, apiURL string, mode output.Mode) (context.Context, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	t.Setenv("NIMBU_TOKEN", "test-token")

	flags := &RootFlags{
		APIURL:  apiURL,
		Site:    "demo",
		Timeout: 2 * time.Second,
	}
	cfg := config.Defaults()
	cfg.DefaultSite = "demo"

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}

	ctx := context.Background()
	ctx = context.WithValue(ctx, rootFlagsKey{}, flags)
	ctx = context.WithValue(ctx, configKey{}, &cfg)
	ctx = output.WithMode(ctx, mode)
	ctx = output.WithWriter(ctx, &output.Writer{Out: out, Err: errOut, Mode: mode, NoTTY: true})
	return ctx, out, errOut
}

func TestAdminWorkflowContextUsesBuffers(t *testing.T) {
	ctx, out, errOut := newAdminWorkflowTestContext(t, "https://api.example.test", output.Mode{})
	if output.WriterFromContext(ctx) == nil {
		t.Fatal("expected writer in context")
	}
	if out == nil || errOut == nil {
		t.Fatal("expected buffers")
	}
}

func TestAdminWorkflowContextWriterIsReadable(t *testing.T) {
	_, out, _ := newAdminWorkflowTestContext(t, "https://api.example.test", output.Mode{})
	if _, err := io.WriteString(out, "ok"); err != nil {
		t.Fatalf("write output: %v", err)
	}
	if out.String() != "ok" {
		t.Fatalf("unexpected output buffer: %q", out.String())
	}
}
