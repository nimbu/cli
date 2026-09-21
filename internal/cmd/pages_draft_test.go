package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
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
	for _, want := range []string{
		"ID:           " + surgicalPageID,
		"Fullpath:     about",
		"Title:        About draft",
		"Published:    true",
		"Editables:",
		"Attachments:",
		"Blokken:      3 blocks",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary missing %q in %q", want, got)
		}
	}

	ctxJSON, outJSON, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	if err := cmd.Run(ctxJSON, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("json: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(outJSON.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	// Document shape: same keys as `pages get --json`, never the draft envelope.
	if body["id"] != surgicalPageID || body["fullpath"] != "about" || body["title"] != "About draft" {
		t.Fatalf("document fields = %s", outJSON.Bytes())
	}
	if _, bad := body["content"]; bad {
		t.Fatalf("draft envelope leaked: %s", outJSON.Bytes())
	}
	items, ok := body["items"].(map[string]any)
	if !ok {
		t.Fatalf("items must be a canvas map: %s", outJSON.Bytes())
	}
	if _, bad := items["page_items"]; bad {
		t.Fatalf("page_items leaked into items: %s", outJSON.Bytes())
	}
	theme, _ := items["Theme"].(map[string]any)
	if theme["type"] != "select" || theme["content"] != "Light" {
		t.Fatalf("Theme = %#v", theme)
	}
	blokken, _ := items["Blokken"].(map[string]any)
	reps, _ := blokken["repeatables"].([]any)
	if blokken["type"] != "canvas" || len(reps) != 3 {
		t.Fatalf("Blokken = %#v", blokken)
	}
	first, _ := reps[0].(map[string]any)
	firstItems, _ := first["items"].(map[string]any)
	title, _ := firstItems["Title"].(map[string]any)
	if first["id"] != surgicalBlockID || title["content"] != "Hero" {
		t.Fatalf("first repeatable = %#v", first)
	}
	draftMeta, ok := body["draft"].(map[string]any)
	if !ok {
		t.Fatalf("draft metadata missing: %s", outJSON.Bytes())
	}
	for key, want := range map[string]any{
		"id":                surgicalDraftID,
		"page_id":           surgicalPageID,
		"future_page_id":    "",
		"reserved_fullpath": "about",
		"updated_at":        surgicalDraftUpdated,
		"items_source":      "draft",
	} {
		if draftMeta[key] != want {
			t.Fatalf("draft.%s = %#v, want %#v", key, draftMeta[key], want)
		}
	}
}

func TestPagesDraftGetRawKeepsEnvelope(t *testing.T) {
	srvState := &surgicalServer{}
	srv := srvState.start(t)
	defer srv.Close()

	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	if err := (&PagesDraftGetCmd{Page: "about", Raw: true}).Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(out.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["id"] != surgicalDraftID {
		t.Fatalf("raw json = %s", out.Bytes())
	}
	content, _ := body["content"].(map[string]any)
	if content["title"] != "About draft" {
		t.Fatalf("content = %#v", content)
	}

	ctxHuman, outHuman, _ := newContractTestContext(t, srv.URL, output.Mode{})
	if err := (&PagesDraftGetCmd{Page: "about", Raw: true}).Run(ctxHuman, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("human: %v", err)
	}
	if !strings.Contains(outHuman.String(), "title: About draft") || !strings.Contains(outHuman.String(), "2 page_items") {
		t.Fatalf("raw human = %q", outHuman.String())
	}

	ctxBoth, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	err := (&PagesDraftGetCmd{Page: "about", Raw: true, Shape: true}).Run(ctxBoth, &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), "--raw and --shape") {
		t.Fatalf("expected raw+shape rejection, got %v", err)
	}
}

// mirroredDraftPageJSON and mirroredDraftJSON describe the same content in the
// two server shapes: the page document (items map) and the draft snapshot
// (content.page_items).
func mirroredDraftPageJSON() string {
	return `{
		"id":"` + surgicalPageID + `",
		"fullpath":"about",
		"updated_at":"` + surgicalUpdatedAt + `",
		"title":"About",
		"published":true,
		"items":{
			"Theme":{"slug":"Theme","type":"select","content":"Light"},
			"Blokken":{
				"slug":"Blokken",
				"type":"canvas",
				"repeatables":[
					{"id":"` + surgicalBlockID + `","slug":"hero_stage","position":1,"items":{"Title":{"slug":"Title","type":"text","content":"Hero"}}},
					{"id":"` + surgicalBlockID2 + `","slug":"proof_strip","position":2,"items":{"Quote":{"slug":"Quote","type":"text","content":"Hi"}}}
				]
			}
		}
	}`
}

func mirroredDraftJSON() string {
	return `{
		"id":"` + surgicalDraftID + `",
		"page_id":"` + surgicalPageID + `",
		"reserved_fullpath":"about",
		"updated_at":"` + surgicalDraftUpdated + `",
		"content":{
			"title":"About",
			"page_items":[
				{"slug":"Theme","type":"select","content":"Light"},
				{"slug":"Blokken","type":"canvas","repeatables":[
					{"_id":"` + surgicalBlockID + `","slug":"hero_stage","position":1,"page_items":[
						{"slug":"Title","type":"text","content":"Hero"}
					]},
					{"_id":"` + surgicalBlockID2 + `","slug":"proof_strip","position":2,"page_items":[
						{"slug":"Quote","type":"text","content":"Hi"}
					]}
				]}
			]
		}
	}`
}

func TestPagesDraftGetShapeMatchesPagesGetShape(t *testing.T) {
	srvState := &surgicalServer{pageJSON: mirroredDraftPageJSON(), draftJSON: mirroredDraftJSON()}
	srv := srvState.start(t)
	defer srv.Close()

	ctxGet, outGet, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	if err := (&PagesGetCmd{Page: "about", Shape: true}).Run(ctxGet, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("pages get --shape: %v", err)
	}
	ctxDraft, outDraft, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	if err := (&PagesDraftGetCmd{Page: "about", Shape: true}).Run(ctxDraft, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("draft get --shape: %v", err)
	}

	var wantShape, gotShape map[string]any
	if err := json.Unmarshal(outGet.Bytes(), &wantShape); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(outDraft.Bytes(), &gotShape); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(wantShape, gotShape) {
		t.Fatalf("shape mismatch\npages get:  %s\ndraft get:  %s", outGet.Bytes(), outDraft.Bytes())
	}
	blokken, _ := gotShape["Blokken"].(map[string]any)
	if blokken["type"] != "canvas" {
		t.Fatalf("Blokken shape = %#v", blokken)
	}
	if reps, _ := blokken["repeatables"].([]any); len(reps) != 2 {
		t.Fatalf("repeatables = %#v", blokken["repeatables"])
	}

	ctxHuman, outHuman, _ := newContractTestContext(t, srv.URL, output.Mode{})
	if err := (&PagesDraftGetCmd{Page: "about", Shape: true}).Run(ctxHuman, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("human shape: %v", err)
	}
	if !strings.Contains(outHuman.String(), "Blokken (canvas)") || !strings.Contains(outHuman.String(), "- hero_stage [0]") {
		t.Fatalf("human shape = %q", outHuman.String())
	}
}

func TestPagesDraftGetJSONDocumentMatchesPagesGetDocument(t *testing.T) {
	srvState := &surgicalServer{pageJSON: mirroredDraftPageJSON(), draftJSON: mirroredDraftJSON()}
	srv := srvState.start(t)
	defer srv.Close()

	ctxGet, outGet, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	if err := (&PagesGetCmd{Page: "about"}).Run(ctxGet, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("pages get: %v", err)
	}
	ctxDraft, outDraft, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	if err := (&PagesDraftGetCmd{Page: "about"}).Run(ctxDraft, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("draft get: %v", err)
	}

	var want, got map[string]any
	if err := json.Unmarshal(outGet.Bytes(), &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(outDraft.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["draft"]; !ok {
		t.Fatalf("draft metadata missing: %s", outDraft.Bytes())
	}
	delete(got, "draft")
	if !reflect.DeepEqual(pruneNullContent(want), pruneNullContent(got)) {
		t.Fatalf("document mismatch\npages get:  %s\ndraft get:  %s", outGet.Bytes(), outDraft.Bytes())
	}
}

// pruneNullContent drops "content": null keys the snapshot converter adds for
// canvas editables, which the live document simply omits.
func pruneNullContent(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			if key == "content" && value == nil {
				continue
			}
			out[key] = pruneNullContent(value)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, value := range typed {
			out[i] = pruneNullContent(value)
		}
		return out
	default:
		return v
	}
}

func TestPagesDraftGetShapeWithoutDraftErrors(t *testing.T) {
	srvState := &surgicalServer{noDraft: true}
	srv := srvState.start(t)
	defer srv.Close()

	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	err := (&PagesDraftGetCmd{Page: "about", Shape: true}).Run(ctx, &RootFlags{Site: "demo"})
	if err == nil {
		t.Fatal("expected missing draft")
	}
	if classifyError(err).Code != errorNotFound {
		t.Fatalf("code = %s", classifyError(err).Code)
	}
	if !strings.Contains(err.Error(), "no draft for page about") {
		t.Fatalf("err = %v", err)
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
		if strings.Contains(err.Error(), "re-run with --confirm") || strings.Contains(err.Error(), "nimbu pages draft discard") {
			t.Fatalf("hint must not be embedded in the message: %v", err)
		}
		desc := classifyError(err)
		if desc.Code != errorConflict {
			t.Fatalf("code = %s", desc.Code)
		}
		if desc.Message != "Live page changed after this draft was based on it; confirm publish to replace live content." {
			t.Fatalf("message = %q", desc.Message)
		}
		if !strings.Contains(desc.Hint, "re-run with --confirm") || !strings.Contains(desc.Hint, "nimbu pages draft discard --page about --force") {
			t.Fatalf("hint = %q", desc.Hint)
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

// divergentLivePageJSON and divergentDraftJSON deliberately disagree: the draft
// changed a block title, added a translation and a file editable, and appended a
// third block that the live page does not have.
func divergentLivePageJSON() string {
	return `{
		"id":"` + surgicalPageID + `",
		"fullpath":"about",
		"updated_at":"` + surgicalUpdatedAt + `",
		"title":"About",
		"published":true,
		"items":{
			"Blokken":{
				"slug":"Blokken",
				"type":"canvas",
				"repeatables":[
					{"id":"` + surgicalBlockID + `","slug":"hero_stage","position":1,"items":{
						"Title":{"slug":"Title","type":"text","content":"Live hero"},
						"Image":{"slug":"Image","type":"file"}
					}},
					{"id":"` + surgicalBlockID2 + `","slug":"proof_strip","position":2,"items":{
						"Quote":{"slug":"Quote","type":"text","content":"Live quote"}
					}}
				]
			}
		}
	}`
}

func divergentDraftJSON() string {
	return `{
		"id":"` + surgicalDraftID + `",
		"page_id":"` + surgicalPageID + `",
		"reserved_fullpath":"about",
		"updated_at":"` + surgicalDraftUpdated + `",
		"content":{
			"title":"About draft",
			"page_items":[
				{"slug":"Blokken","type":"canvas","repeatables":[
					{"_id":"` + surgicalBlockID + `","slug":"hero_stage","position":1,"page_items":[
						{"_id":"item-title","slug":"Title","type":"text","content":"Draft hero",
						 "translations":[{"locale":"nl","content":"Draft hero"},{"locale":"en","content":"Draft hero EN"}]},
						{"_id":"item-image","slug":"Image","type":"file",
						 "source":"draft-shot.png","source_content_type":"image/png","source_size":4321,
						 "source_width":800,"source_height":600}
					]},
					{"_id":"` + surgicalBlockID2 + `","slug":"proof_strip","position":2,"page_items":[
						{"slug":"Quote","type":"text","content":"Draft quote"}
					]},
					{"_id":"` + surgicalDraftOnlyID + `","slug":"proof_strip","position":3,"page_items":[
						{"slug":"Quote","type":"text","content":"Draft only block"}
					]}
				]}
			]
		}
	}`
}

// TestPagesDraftGetShowsDraftContentNotLive is the mirror of
// TestPagesDraftGetJSONDocumentMatchesPagesGetDocument: when the draft and the
// live page disagree, the draft document must carry the draft's values only.
func TestPagesDraftGetShowsDraftContentNotLive(t *testing.T) {
	srvState := &surgicalServer{pageJSON: divergentLivePageJSON(), draftJSON: divergentDraftJSON()}
	srv := srvState.start(t)
	defer srv.Close()

	ctx, out, errOut := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	if err := (&PagesDraftGetCmd{Page: "about"}).Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("draft get: %v", err)
	}
	if strings.Contains(errOut.String(), "warning:") {
		t.Fatalf("unexpected warning: %q", errOut.String())
	}
	raw := out.String()
	for _, liveOnly := range []string{"Live hero", "Live quote"} {
		if strings.Contains(raw, liveOnly) {
			t.Fatalf("live-only value %q leaked into the draft document: %s", liveOnly, raw)
		}
	}

	var body map[string]any
	if err := json.Unmarshal(out.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	draftMeta, _ := body["draft"].(map[string]any)
	if draftMeta["items_source"] != "draft" {
		t.Fatalf("draft.items_source = %#v", draftMeta["items_source"])
	}
	items, _ := body["items"].(map[string]any)
	blokken, _ := items["Blokken"].(map[string]any)
	reps, _ := blokken["repeatables"].([]any)
	if len(reps) != 3 {
		t.Fatalf("draft-only block missing, repeatables = %#v", reps)
	}
	third, _ := reps[2].(map[string]any)
	thirdItems, _ := third["items"].(map[string]any)
	thirdQuote, _ := thirdItems["Quote"].(map[string]any)
	if third["id"] != surgicalDraftOnlyID || thirdQuote["content"] != "Draft only block" {
		t.Fatalf("draft-only block = %#v", third)
	}

	hero, _ := reps[0].(map[string]any)
	heroItems, _ := hero["items"].(map[string]any)
	title, _ := heroItems["Title"].(map[string]any)
	if title["content"] != "Draft hero" {
		t.Fatalf("Title = %#v", title)
	}
	if title["id"] != "item-title" || title["slug"] != "Title" {
		t.Fatalf("Title identity = %#v", title)
	}
	translations, _ := title["translations"].(map[string]any)
	en, _ := translations["en"].(map[string]any)
	if en["content"] != "Draft hero EN" {
		t.Fatalf("translations = %#v", translations)
	}
	if _, leaked := en["locale"]; leaked {
		t.Fatalf("locale key must not stay inside the translation map: %#v", en)
	}

	// A file editable must render its file object, not "(empty)".
	image, _ := heroItems["Image"].(map[string]any)
	file, _ := image["file"].(map[string]any)
	if file["filename"] != "draft-shot.png" || file["content_type"] != "image/png" {
		t.Fatalf("Image file = %#v", image)
	}
	if file["width"] != float64(800) || file["height"] != float64(600) || file["size"] != float64(4321) {
		t.Fatalf("Image file metadata = %#v", file)
	}
	if _, leaked := image["source"]; leaked {
		t.Fatalf("raw source column must not leak: %#v", image)
	}

	ctxHuman, outHuman, _ := newContractTestContext(t, srv.URL, output.Mode{})
	if err := (&PagesGetCmd{Page: "about", Draft: true, Outline: true}).Run(ctxHuman, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("pages get --draft --outline: %v", err)
	}
	if !strings.Contains(outHuman.String(), "draft-shot.png") {
		t.Fatalf("file editable should render its filename, got %q", outHuman.String())
	}
	if strings.Contains(outHuman.String(), "Live hero") {
		t.Fatalf("outline leaked live content: %q", outHuman.String())
	}
}

func TestPagesDraftGetWarnsWhenSnapshotCannotBeDecoded(t *testing.T) {
	broken := `{
		"id":"` + surgicalDraftID + `",
		"page_id":"` + surgicalPageID + `",
		"reserved_fullpath":"about",
		"updated_at":"` + surgicalDraftUpdated + `",
		"content":{"title":"About draft","page_items":"nope"}
	}`
	srvState := &surgicalServer{pageJSON: divergentLivePageJSON(), draftJSON: broken}
	srv := srvState.start(t)
	defer srv.Close()

	ctx, out, errOut := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	if err := (&PagesDraftGetCmd{Page: "about"}).Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("draft get: %v", err)
	}
	if !strings.Contains(errOut.String(), "warning: could not decode draft snapshot; showing live items") {
		t.Fatalf("stderr = %q", errOut.String())
	}
	var body map[string]any
	if err := json.Unmarshal(out.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	draftMeta, _ := body["draft"].(map[string]any)
	if draftMeta["items_source"] != "live" {
		t.Fatalf("draft.items_source = %#v (want live): %s", draftMeta["items_source"], out.Bytes())
	}
	// The live items are still shown, and the draft's page fields still win.
	if body["title"] != "About draft" {
		t.Fatalf("title = %#v", body["title"])
	}
	if !strings.Contains(out.String(), "Live hero") {
		t.Fatalf("live items should be the fallback: %s", out.Bytes())
	}
}
