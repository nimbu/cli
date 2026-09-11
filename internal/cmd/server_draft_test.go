package cmd

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

func TestDraftPreviewBindingsRegistersFullpathAndTranslations(t *testing.T) {
	var tokenPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && (r.URL.Path == "/pages/about" || r.URL.Path == "/pages/zz-cli-test/surgical"):
			_, _ = w.Write([]byte(`{
				"id":"` + surgicalPageID + `",
				"fullpath":"zz-cli-test/surgical",
				"translations":{
					"en":{"fullpath":"en/zz-cli-test/surgical"}
				}
			}`))
		case r.Method == http.MethodPost && r.URL.Path == "/pages/"+surgicalPageID+"/draft/preview_token":
			tokenPath = r.URL.Path
			_, _ = w.Write([]byte(`{"token":"jwt-token","preview_url":"/zz-cli-test/surgical?preview=jwt-token"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
	client := api.New(srv.URL, "tok")
	tokens, logs, err := draftPreviewBindings(ctx, client, []string{"about"})
	if err != nil {
		t.Fatal(err)
	}
	if tokenPath != "/pages/"+surgicalPageID+"/draft/preview_token" {
		t.Fatalf("token path = %s", tokenPath)
	}
	if tokens["/zz-cli-test/surgical"] != "jwt-token" || tokens["/en/zz-cli-test/surgical"] != "jwt-token" {
		t.Fatalf("tokens = %#v", tokens)
	}
	if len(logs) != 1 || !strings.Contains(logs[0], "draft preview enabled for /zz-cli-test/surgical (token expires in 24h)") {
		t.Fatalf("logs = %#v", logs)
	}
}

func TestDraftPreviewBindingsMapsDisabledDrafts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/pages/"):
			_, _ = w.Write([]byte(surgicalPageJSON()))
		default:
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"Page drafts are not enabled"}`))
		}
	}))
	defer srv.Close()

	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
	_, _, err := draftPreviewBindings(ctx, api.New(srv.URL, "tok"), []string{"about"})
	if err == nil {
		t.Fatal("expected drafts-disabled error")
	}
	desc := classifyError(err)
	if !strings.Contains(desc.Hint, "PAGE_DRAFTS_DISABLED") {
		t.Fatalf("err = %v hint = %q", err, desc.Hint)
	}
}

func TestDraftPreviewBindingsRequiresExistingDraft(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/pages/about":
			_, _ = w.Write([]byte(surgicalPageJSON()))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"message":"Not Found","code":101}`)
		}
	}))
	defer srv.Close()

	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
	_, _, err := draftPreviewBindings(ctx, api.New(srv.URL, "tok"), []string{"about"})
	if err == nil || !strings.Contains(err.Error(), "no draft for page about") {
		t.Fatalf("err = %v", err)
	}
}
