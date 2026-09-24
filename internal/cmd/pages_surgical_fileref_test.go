package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

const (
	fileRefSiteShort = "oa8td8r"
	fileRefUploadID  = "507f1f77bcf86cd799439014"
	fileRefCDNURL    = "https://cdn.nimbu.io/s/oa8td8r/assets/1778666070043/dot.png"
)

// serveSameSiteUpload answers the site short id and upload lookups that turn
// a same-site CDN URL into a nimbu:// FileRef.
func serveSameSiteUpload(w http.ResponseWriter, r *http.Request) bool {
	switch r.URL.Path {
	case "/themes":
		_, _ = w.Write([]byte(`[{"id":"theme1","site_short_id":"` + fileRefSiteShort + `"}]`))
	case "/uploads":
		_, _ = w.Write([]byte(`[{"id":"` + fileRefUploadID + `","url":"` + fileRefCDNURL + `"}]`))
	default:
		return false
	}
	return true
}

func insertOpValueItems(t *testing.T, srvState *surgicalServer) map[string]any {
	t.Helper()
	op := batchOpAt(t, srvState, 0)
	if op["op"] != "insert" {
		t.Fatalf("op = %#v", op)
	}
	items, ok := op["value"].(map[string]any)["items"].(map[string]any)
	if !ok {
		t.Fatalf("insert value = %#v", op["value"])
	}
	return items
}

func TestPagesBatchInsertExpandsFileItems(t *testing.T) {
	t.Run("same-site upload url becomes a FileRef on a draft batch", func(t *testing.T) {
		srvState := &surgicalServer{extraFn: serveSameSiteUpload}
		ops := `[{"op":"insert","path":"Blokken","value":{"slug":"hero_stage","items":{
			"Title":"Hi",
			"Image":{"attachment_url":"` + fileRefCDNURL + `"}
		}}}]`
		if _, err := runBatchFile(t, srvState, ops, &PagesBatchCmd{Draft: true}); err != nil {
			t.Fatalf("run: %v", err)
		}
		if srvState.draftPosts != 1 || srvState.posts != 0 {
			t.Fatalf("draftPosts=%d posts=%d", srvState.draftPosts, srvState.posts)
		}
		items := insertOpValueItems(t, srvState)
		image, _ := items["Image"].(map[string]any)
		want := "nimbu://" + fileRefSiteShort + "/uploads/" + fileRefUploadID
		if image["__type"] != "FileRef" || image["source"] != want || len(image) != 2 {
			t.Fatalf("Image = %#v, want FileRef %s", items["Image"], want)
		}
		if items["Title"] != "Hi" {
			t.Fatalf("Title = %#v", items["Title"])
		}
	})

	t.Run("foreign url is inlined and non-file items stay untouched", func(t *testing.T) {
		asset := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("png-bytes"))
		}))
		t.Cleanup(asset.Close)
		srvState := &surgicalServer{}
		// Ref is a reference field: a file-like payload there must not be expanded.
		ops := `[{"op":"insert","path":"Blokken","value":{"slug":"hero_stage","items":{
			"Title":"Hi",
			"Enabled":true,
			"Ref":{"url":"https://example.invalid/not-a-file"},
			"Image":{"attachment_url":"` + asset.URL + `/dot.png"}
		}}}]`
		if _, err := runBatchFile(t, srvState, ops, &PagesBatchCmd{}); err != nil {
			t.Fatalf("run: %v", err)
		}
		items := insertOpValueItems(t, srvState)
		image, _ := items["Image"].(map[string]any)
		if image["data"] == nil || image["filename"] != "dot.png" || image["content_type"] != "image/png" {
			t.Fatalf("Image = %#v, want inline data payload", items["Image"])
		}
		if _, ok := image["attachment_url"]; ok {
			t.Fatalf("Image still carries attachment_url: %#v", image)
		}
		if items["Title"] != "Hi" || items["Enabled"] != true {
			t.Fatalf("scalar items changed: %#v", items)
		}
		ref, _ := items["Ref"].(map[string]any)
		if ref["url"] != "https://example.invalid/not-a-file" || len(ref) != 1 {
			t.Fatalf("Ref = %#v, want untouched", items["Ref"])
		}
		if len(srvState.lastBody["operations"].([]any)) != 1 {
			t.Fatalf("file items must not be split into follow-up ops: %#v", srvState.lastBody)
		}
	})
}

func draftBatchFailure(opCode, opMessage string) func(http.ResponseWriter, *http.Request, int) {
	return func(w http.ResponseWriter, _ *http.Request, _ int) {
		path := "/items/Blokken/repeatables/" + surgicalBlockID + "/items/Image"
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{
			"message":"Draft batch failed: operation 0 (` + path + `): ` + opMessage + `",
			"code":"operation_failed",
			"results":[{"index":0,"status":"error","path":"` + path + `","error":{"code":"` + opCode + `","message":"` + opMessage + `"}}]
		}`))
	}
}

// jsonErrorEnvelope renders the --json error envelope emitCommandError prints.
func jsonErrorEnvelope(t *testing.T, err error) map[string]any {
	t.Helper()
	data, marshalErr := json.Marshal(errorEnvelope{Status: "error", Error: classifyError(err)})
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	var envelope map[string]any
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope["error"].(map[string]any)
}

func assertDraftOpResult(t *testing.T, envErr map[string]any, opCode, opMessage string) {
	t.Helper()
	wantMessage := "Draft batch failed: operation 0 (/items/Blokken/repeatables/" + surgicalBlockID + "/items/Image): " + opMessage
	if envErr["message"] != wantMessage {
		t.Fatalf("message = %#v, want %q", envErr["message"], wantMessage)
	}
	if envErr["http_status"] != float64(422) {
		t.Fatalf("http_status = %#v", envErr["http_status"])
	}
	details, _ := envErr["details"].(map[string]any)
	results, _ := details["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("details.results = %#v", envErr["details"])
	}
	result := results[0].(map[string]any)
	if result["index"] != float64(0) || result["status"] != "error" ||
		result["path"] != "/items/Blokken/repeatables/"+surgicalBlockID+"/items/Image" {
		t.Fatalf("result = %#v", result)
	}
	opErr, _ := result["error"].(map[string]any)
	if opErr["code"] != opCode || opErr["message"] != opMessage {
		t.Fatalf("result.error = %#v", result["error"])
	}
	if hint, _ := envErr["hint"].(string); strings.Contains(hint, "not supported on drafts") {
		t.Fatalf("stale draft FileRef hint: %q", hint)
	}
}

func TestDraftFileRefErrorsKeepResultsInJSONEnvelope(t *testing.T) {
	fileRef := `{"__type":"FileRef","source":"nimbu://` + fileRefSiteShort + `/uploads/` + fileRefUploadID + `"}`

	t.Run("pages set --draft with an op error keeps details.results", func(t *testing.T) {
		srvState := &surgicalServer{draftBatchFn: draftBatchFailure("invalid_file_format", "Invalid file format")}
		srv := srvState.start(t)
		defer srv.Close()
		ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
		cmd := &PagesSetCmd{Page: "about", Path: "Blokken[0].Image", File: writePageFile(t, fileRef), Draft: true}
		err := cmd.Run(ctx, &RootFlags{Site: "demo"})
		if err == nil {
			t.Fatal("expected draft batch failure")
		}
		envErr := jsonErrorEnvelope(t, err)
		if envErr["code"] != string(errorRequestValidation) {
			t.Fatalf("code = %#v", envErr["code"])
		}
		assertDraftOpResult(t, envErr, "invalid_file_format", "Invalid file format")
	})

	t.Run("pages draft batch with an unauthorized FileRef is a permission error", func(t *testing.T) {
		srvState := &surgicalServer{draftBatchFn: draftBatchFailure("unauthorized", "Not authorized to copy this file")}
		ops := `[{"op":"set","path":"Blokken[0].Image","value":` + fileRef + `}]`
		srv := srvState.start(t)
		defer srv.Close()
		ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
		cmd := &PagesDraftBatchCmd{Page: "about", File: writePageFile(t, ops)}
		err := cmd.Run(ctx, &RootFlags{Site: "demo"})
		if err == nil {
			t.Fatal("expected draft batch failure")
		}
		envErr := jsonErrorEnvelope(t, err)
		if envErr["code"] != string(errorAuthForbidden) || envErr["exit_code"] != float64(ExitAuthz) {
			t.Fatalf("code=%#v exit=%#v", envErr["code"], envErr["exit_code"])
		}
		assertDraftOpResult(t, envErr, "unauthorized", "Not authorized to copy this file")
	})

	t.Run("pages draft batch with a missing source upload is resource.not_found", func(t *testing.T) {
		srvState := &surgicalServer{draftBatchFn: draftBatchFailure("not_found", "Source file is missing from storage")}
		ops := `[{"op":"set","path":"Blokken[0].Image","value":` + fileRef + `}]`
		srv := srvState.start(t)
		defer srv.Close()
		ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
		cmd := &PagesDraftBatchCmd{Page: "about", File: writePageFile(t, ops)}
		err := cmd.Run(ctx, &RootFlags{Site: "demo"})
		if err == nil {
			t.Fatal("expected draft batch failure")
		}
		envErr := jsonErrorEnvelope(t, err)
		if envErr["code"] != string(errorNotFound) || envErr["exit_code"] != float64(ExitNotFound) {
			t.Fatalf("code=%#v exit=%#v", envErr["code"], envErr["exit_code"])
		}
		if !strings.Contains(envErr["message"].(string), "Source file is missing from storage") {
			t.Fatalf("message = %#v", envErr["message"])
		}
		assertDraftOpResult(t, envErr, "not_found", "Source file is missing from storage")
	})
}

func TestApplyFileRefOpErrorNeedsASingleFailingOp(t *testing.T) {
	failed := func(code string) api.BatchOpResult {
		return api.BatchOpResult{Status: "error", Error: &api.BatchOpError{Code: code, Message: code}}
	}
	desc := errorDescriptor{Code: errorRequestValidation, ExitCode: ExitValidation}
	applyFileRefOpError(&desc, []api.BatchOpResult{failed("not_found"), failed("path_not_found")})
	if desc.Code != errorRequestValidation || desc.ExitCode != ExitValidation {
		t.Fatalf("two failing ops were reclassified: %#v", desc)
	}
	applyFileRefOpError(&desc, []api.BatchOpResult{{Status: "ok"}, failed("path_not_found")})
	if desc.Code != errorRequestValidation {
		t.Fatalf("path_not_found was reclassified: %#v", desc)
	}
	applyFileRefOpError(&desc, []api.BatchOpResult{{Status: "ok"}, failed("not_found")})
	if desc.Code != errorNotFound || desc.ExitCode != ExitNotFound {
		t.Fatalf("single not_found = %#v", desc)
	}
}

func TestPagesBatchRetryOn412ReusesExpandedFiles(t *testing.T) {
	downloads := 0
	asset := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads++
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png-bytes"))
	}))
	t.Cleanup(asset.Close)
	var images []any
	srvState := &surgicalServer{}
	srvState.batchFn = func(w http.ResponseWriter, _ *http.Request, n int) {
		images = append(images, insertOpValueItems(t, srvState)["Image"])
		if n == 1 {
			w.WriteHeader(http.StatusPreconditionFailed)
			_, _ = w.Write([]byte(`{"message":"Precondition Failed","code":"precondition_failed","current_etag":"deadbeef"}`))
			return
		}
		_, _ = w.Write([]byte(`{
			"results":[{"index":0,"status":"ok","path":"/items/Blokken/repeatables","id":"newblock000000000000001"}],
			"etag":"newetag12",
			"updated_at":"2026-09-10T15:00:00.000Z",
			"page":` + surgicalPageJSON() + `
		}`))
	}
	ops := `[{"op":"insert","path":"Blokken","value":{"slug":"hero_stage","items":{"Image":{"attachment_url":"` + asset.URL + `/dot.png"}}}}]`
	if _, err := runBatchFile(t, srvState, ops, &PagesBatchCmd{}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if srvState.posts != 2 || downloads != 1 {
		t.Fatalf("posts=%d downloads=%d, want 2 posts and 1 download", srvState.posts, downloads)
	}
	for i, image := range images {
		if file, _ := image.(map[string]any); file["data"] == nil || file["filename"] != "dot.png" {
			t.Fatalf("attempt %d Image = %#v", i+1, image)
		}
	}
}
