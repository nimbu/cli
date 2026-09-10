package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

func TestPagesDraftGetHumanAndJSON(t *testing.T) {
	srvState := &surgicalServer{}
	srv := srvState.start(t)
	defer srv.Close()

	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &PagesDraftGetCmd{Page: "about"}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Draft "+surgicalDraftID+" for about, updated "+surgicalDraftUpdated) {
		t.Fatalf("human header = %q", got)
	}
	if !strings.Contains(got, "title: About draft") || !strings.Contains(got, "2 page_items") {
		t.Fatalf("summary = %q", got)
	}

	ctxJSON, outJSON, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	if err := cmd.Run(ctxJSON, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("json: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(outJSON.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["id"] != surgicalDraftID {
		t.Fatalf("json = %s", outJSON.Bytes())
	}
	content, _ := body["content"].(map[string]any)
	if content["title"] != "About draft" {
		t.Fatalf("content = %#v", content)
	}
}

func TestPagesDraftGetNotFound(t *testing.T) {
	srvState := &surgicalServer{noDraft: true}
	srv := srvState.start(t)
	defer srv.Close()

	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	err := (&PagesDraftGetCmd{Page: "about"}).Run(ctx, &RootFlags{Site: "demo"})
	if err == nil {
		t.Fatal("expected missing draft")
	}
	desc := classifyError(err)
	if desc.Code != errorNotFound {
		t.Fatalf("code = %s", desc.Code)
	}
	if !strings.Contains(err.Error(), "no draft for page about") || !strings.Contains(err.Error(), "nimbu pages set --draft") {
		t.Fatalf("err = %v", err)
	}
}

func TestPagesDraftGetDisabledHint(t *testing.T) {
	srvState := &surgicalServer{draftsOff: true}
	srv := srvState.start(t)
	defer srv.Close()

	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	err := (&PagesDraftGetCmd{Page: "about"}).Run(ctx, &RootFlags{Site: "demo"})
	if err == nil {
		t.Fatal("expected 403")
	}
	desc := classifyError(err)
	if desc.Code != errorAuthForbidden || !strings.Contains(desc.Hint, "PAGE_DRAFTS_DISABLED") {
		t.Fatalf("desc = %#v", desc)
	}
}

func TestPagesDraftSavePostsNormalizedBody(t *testing.T) {
	var posted map[string]any
	var path, rawQ string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/pages/about":
			_, _ = w.Write([]byte(surgicalPageJSON()))
		case r.Method == http.MethodPost && r.URL.Path == "/pages/"+surgicalPageID+"/draft":
			path = r.URL.Path
			rawQ = r.URL.RawQuery
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &posted); err != nil {
				t.Fatalf("decode: %v", err)
			}
			_, _ = w.Write([]byte(surgicalDraftJSON()))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	file := writePageFile(t, `{"id":"old","title":"Saved draft","items":{}}`)
	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesDraftSaveCmd{Page: "about", File: file, Locale: "en"}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if path != "/pages/"+surgicalPageID+"/draft" {
		t.Fatalf("path = %s", path)
	}
	if !strings.Contains(rawQ, "content_locale=en") {
		t.Fatalf("query = %q", rawQ)
	}
	if _, ok := posted["id"]; ok {
		t.Fatalf("id should be stripped: %#v", posted)
	}
	if posted["title"] != "Saved draft" {
		t.Fatalf("posted = %#v", posted)
	}
	var body map[string]any
	if err := json.Unmarshal(out.Bytes(), &body); err != nil || body["id"] != surgicalDraftID {
		t.Fatalf("json = %s", out.Bytes())
	}
}

func TestPagesDraftPublishConfirmAndConflictHint(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var body []byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/pages/about":
				_, _ = w.Write([]byte(surgicalPageJSON()))
			case r.Method == http.MethodPost && r.URL.Path == "/pages/"+surgicalPageID+"/draft/publish":
				body, _ = io.ReadAll(r.Body)
				_, _ = w.Write([]byte(`{"id":"` + surgicalPageID + `","fullpath":"about","updated_at":"2026-09-10T16:00:00.000Z"}`))
			default:
				http.NotFound(w, r)
			}
		}))
		defer srv.Close()

		ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{})
		cmd := &PagesDraftPublishCmd{Page: "about", Confirm: true}
		if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
			t.Fatalf("run: %v", err)
		}
		if string(body) != `{"confirm":true}` {
			t.Fatalf("body = %s", body)
		}
		if !strings.Contains(out.String(), "Published draft to about (page updated 2026-09-10T16:00:00.000Z)") {
			t.Fatalf("out = %q", out.String())
		}
	})

	t.Run("conflict", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/pages/about":
				_, _ = w.Write([]byte(surgicalPageJSON()))
			case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/draft/publish"):
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte(`{"code":"draft_base_changed","message":"Live page changed after this draft was based on it; confirm publish to replace live content."}`))
			default:
				http.NotFound(w, r)
			}
		}))
		defer srv.Close()

		ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
		err := (&PagesDraftPublishCmd{Page: "about"}).Run(ctx, &RootFlags{Site: "demo"})
		if err == nil {
			t.Fatal("expected 409")
		}
		if !strings.Contains(err.Error(), "Live page changed after this draft was based on it") {
			t.Fatalf("err = %v", err)
		}
		if !strings.Contains(err.Error(), "re-run with --confirm") || !strings.Contains(err.Error(), "nimbu pages draft discard --page about --force") {
			t.Fatalf("missing hint: %v", err)
		}
		desc := classifyError(err)
		if desc.Code != errorConflict {
			t.Fatalf("code = %s", desc.Code)
		}
	})

	t.Run("readonly", func(t *testing.T) {
		ctx, _, _ := newContractTestContext(t, "http://127.0.0.1:1", output.Mode{})
		err := (&PagesDraftPublishCmd{Page: "about"}).Run(ctx, &RootFlags{Readonly: true})
		if err == nil || !strings.Contains(err.Error(), "readonly") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestPagesDraftDiscardRequiresForce(t *testing.T) {
	var deleted bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/pages/about":
			_, _ = w.Write([]byte(surgicalPageJSON()))
		case r.Method == http.MethodDelete && r.URL.Path == "/pages/"+surgicalPageID+"/draft":
			deleted = true
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &PagesDraftDiscardCmd{Page: "about"}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected force, got %v", err)
	}
	if deleted {
		t.Fatal("deleted without force")
	}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo", Force: true}); err != nil {
		t.Fatalf("forced discard: %v", err)
	}
	if !deleted {
		t.Fatal("expected DELETE")
	}
}

func TestPagesDraftPreviewURLPrefixesPublicOrigin(t *testing.T) {
	var opened string
	openDraftPreview = func(target string) error {
		opened = target
		return nil
	}
	t.Cleanup(func() { openDraftPreview = openDraftPreviewDefault })

	page := strings.Replace(surgicalPageJSON(), `"fullpath":"about"`, `"fullpath":"about","public_url":"https://www.zenjoy.be/zz-cli-test/surgical"`, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/pages/about":
			_, _ = w.Write([]byte(page))
		case r.Method == http.MethodPost && r.URL.Path == "/pages/"+surgicalPageID+"/draft/preview_token":
			_, _ = w.Write([]byte(`{"token":"jwt-token","preview_url":"/zz-cli-test/surgical?preview=jwt-token"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &PagesDraftPreviewURLCmd{Page: "about", Open: true}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "https://www.zenjoy.be/zz-cli-test/surgical?preview=jwt-token"
	if strings.TrimSpace(out.String()) != want {
		t.Fatalf("url = %q", out.String())
	}
	if opened != want {
		t.Fatalf("opened = %q", opened)
	}

	ctxJSON, outJSON, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd.Open = false
	if err := cmd.Run(ctxJSON, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("json: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(outJSON.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["token"] != "jwt-token" || body["preview_url"] != want || body["expires_in"] != "24h" {
		t.Fatalf("json = %#v", body)
	}
}

func TestPagesDraftBatchAliasesDraftFlag(t *testing.T) {
	srvState := &surgicalServer{}
	srv := srvState.start(t)
	defer srv.Close()
	file := writePageFile(t, `{"operations":[{"op":"set","path":"title","value":"Hi"}]}`)
	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &PagesDraftBatchCmd{Page: "about", File: file}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if srvState.posts != 0 || srvState.draftPosts != 1 {
		t.Fatalf("live=%d draft=%d", srvState.posts, srvState.draftPosts)
	}
	if srvState.ifMatch[0] != "" {
		t.Fatalf("If-Match = %q", srvState.ifMatch[0])
	}
}

func TestDraftAPIErrorWrapper(t *testing.T) {
	err := draftAPIError(&api.Error{StatusCode: 403, Message: "Page drafts are not enabled"}, "about")
	desc := classifyError(err)
	if desc.Code != errorAuthForbidden || !strings.Contains(desc.Hint, "PAGE_DRAFTS_DISABLED") {
		t.Fatalf("desc = %#v", desc)
	}

	notFound := draftAPIError(&api.Error{StatusCode: 404, Message: "Not Found", Code: "101"}, "about")
	if classifyError(notFound).Code != errorNotFound {
		t.Fatalf("404 code = %s", classifyError(notFound).Code)
	}
	if !strings.Contains(notFound.Error(), "no draft for page about") {
		t.Fatalf("404 = %v", notFound)
	}
}
