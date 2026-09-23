package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

const sameSiteUploadURL = "https://cdn.nimbu.io/s/acme/uploads/hero.jpg"

// serveSameSiteUpload makes sameSiteUploadURL an upload of the test site.
func serveSameSiteUpload(w http.ResponseWriter, r *http.Request) bool {
	switch r.URL.Path {
	case "/themes":
		_, _ = w.Write([]byte(`[{"id":"t1","name":"default","site_short_id":"acme"}]`))
	case "/uploads":
		_, _ = w.Write([]byte(`[{"id":"up1","name":"hero.jpg","url":"` + sameSiteUploadURL + `"}]`))
	default:
		return false
	}
	return true
}

func insertItemsAt(t *testing.T, srvState *surgicalServer, i int) map[string]any {
	t.Helper()
	value, ok := batchOpAt(t, srvState, i)["value"].(map[string]any)
	if !ok {
		t.Fatalf("insert value = %#v", batchOpAt(t, srvState, i)["value"])
	}
	items, ok := value["items"].(map[string]any)
	if !ok {
		t.Fatalf("insert items = %#v", value)
	}
	return items
}

func TestPagesBatchInsertExpandsFileItems(t *testing.T) {
	t.Run("same-site upload URL becomes a FileRef", func(t *testing.T) {
		srvState := &surgicalServer{otherFn: serveSameSiteUpload}
		if _, err := runBatchFile(t, srvState,
			`[{"op":"insert","path":"Blokken","value":{"slug":"hero_stage","items":{"Image":{"attachment_url":"`+sameSiteUploadURL+`"}}}}]`,
			&PagesBatchCmd{}); err != nil {
			t.Fatalf("run: %v", err)
		}
		image := insertItemsAt(t, srvState, 0)["Image"].(map[string]any)
		if image["__type"] != "FileRef" || image["source"] != "nimbu://acme/uploads/up1" {
			t.Fatalf("Image = %#v", image)
		}
	})

	t.Run("foreign URL is inlined as data", func(t *testing.T) {
		asset := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("png-bytes"))
		}))
		t.Cleanup(asset.Close)
		srvState := &surgicalServer{}
		if _, err := runBatchFile(t, srvState,
			`[{"op":"insert","path":"Blokken","value":{"slug":"hero_stage","items":{"Image":{"attachment_url":"`+asset.URL+`/dot.png"}}}}]`,
			&PagesBatchCmd{Draft: true}); err != nil {
			t.Fatalf("run: %v", err)
		}
		if srvState.draftPosts != 1 {
			t.Fatalf("draft posts = %d", srvState.draftPosts)
		}
		image := insertItemsAt(t, srvState, 0)["Image"].(map[string]any)
		if image["data"] == nil || image["filename"] != "dot.png" || image["content_type"] != "image/png" {
			t.Fatalf("Image = %#v", image)
		}
		if image["attachment_url"] != nil || image["__type"] != nil {
			t.Fatalf("Image should be {data,filename,content_type}, got %#v", image)
		}
	})

	t.Run("non-file items stay untouched", func(t *testing.T) {
		srvState := &surgicalServer{otherFn: serveSameSiteUpload}
		if _, err := runBatchFile(t, srvState,
			`[{"op":"insert","path":"Blokken","value":{"slug":"hero_stage","items":{
				"Title":{"url":"https://example.test/not-a-file"},
				"Enabled":true,
				"Image":{"attachment_url":"`+sameSiteUploadURL+`"}
			}}}]`,
			&PagesBatchCmd{}); err != nil {
			t.Fatalf("run: %v", err)
		}
		items := insertItemsAt(t, srvState, 0)
		title := items["Title"].(map[string]any)
		if len(title) != 1 || title["url"] != "https://example.test/not-a-file" {
			t.Fatalf("Title = %#v", title)
		}
		if items["Enabled"] != true {
			t.Fatalf("Enabled = %#v", items["Enabled"])
		}
		if len(srvState.lastBody["operations"].([]any)) != 1 {
			t.Fatalf("file items must not be split into extra set ops: %#v", srvState.lastBody["operations"])
		}
	})

	t.Run("schema failure sends items as written", func(t *testing.T) {
		srvState := &surgicalServer{schemaJSON: `{`}
		if _, err := runBatchFile(t, srvState,
			`[{"op":"insert","path":"/items/Blokken","value":{"slug":"hero_stage","items":{"Title":{"url":"https://example.test/x"}}}}]`,
			&PagesBatchCmd{}); err != nil {
			t.Fatalf("run: %v", err)
		}
		title := insertItemsAt(t, srvState, 0)["Title"].(map[string]any)
		if title["url"] != "https://example.test/x" {
			t.Fatalf("Title = %#v", title)
		}
	})

	t.Run("insert without file payloads skips the schema", func(t *testing.T) {
		srvState := &surgicalServer{schemaJSON: `{`}
		if _, err := runBatchFile(t, srvState,
			`[{"op":"insert","path":"/items/Blokken","value":{"slug":"proof_strip","items":{"Quote":"Hi"}}}]`,
			&PagesBatchCmd{}); err != nil {
			t.Fatalf("run: %v", err)
		}
		if got := insertItemsAt(t, srvState, 0)["Quote"]; got != "Hi" {
			t.Fatalf("Quote = %#v", got)
		}
	})
}

const draftBatchFailedBody = `{
	"message":"Draft batch failed: operation 0 (/items/Blokken/repeatables/` + surgicalBlockID + `/items/Image): %s",
	"code":"operation_failed",
	"results":[{"index":0,"status":"error","path":"/items/Blokken/repeatables/` + surgicalBlockID + `/items/Image","error":{"code":"%s","message":"%s"}}]
}`

func runFailingDraftSet(t *testing.T, code, message string) (errorDescriptor, error) {
	t.Helper()
	srvState := &surgicalServer{}
	srvState.draftBatchFn = func(w http.ResponseWriter, _ *http.Request, _ int) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = fmt.Fprintf(w, draftBatchFailedBody, message, code, message)
	}
	srv := srvState.start(t)
	defer srv.Close()
	file := writePageFile(t, `{"__type":"FileRef","source":"nimbu://other/uploads/1"}`)
	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesSetCmd{Page: "about", Path: "Blokken[0].Image", File: file, Draft: true}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil {
		t.Fatal("expected draft batch failure")
	}
	return classifyError(err), err
}

func TestPagesSetDraftFailureEnvelopeKeepsResults(t *testing.T) {
	desc, _ := runFailingDraftSet(t, "invalid_file_format", "Unsupported file value")
	if desc.Code != errorRequestValidation || desc.HTTPStatus != 422 {
		t.Fatalf("code=%s http=%d", desc.Code, desc.HTTPStatus)
	}
	if !strings.HasPrefix(desc.Message, "Draft batch failed: operation 0 (") || !strings.HasSuffix(desc.Message, "Unsupported file value") {
		t.Fatalf("message = %q", desc.Message)
	}

	data, err := json.Marshal(errorEnvelope{Status: "error", Error: desc})
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Error struct {
			Details struct {
				Results []api.BatchOpResult `json:"results"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	results := envelope.Error.Details.Results
	if len(results) != 1 || results[0].Error == nil {
		t.Fatalf("envelope results = %s", data)
	}
	got := results[0]
	if got.Index != 0 || !strings.HasSuffix(got.Path, "/items/Image") ||
		got.Error.Code != "invalid_file_format" || got.Error.Message != "Unsupported file value" {
		t.Fatalf("result = %#v", got)
	}
}

func TestPagesSetDraftUnauthorizedFileRefIsPermissionError(t *testing.T) {
	desc, err := runFailingDraftSet(t, "unauthorized", "Not authorized to read the source file")
	if desc.Code != errorAuthForbidden || desc.ExitCode != ExitAuthz || desc.HTTPStatus != 422 {
		t.Fatalf("code=%s exit=%d http=%d", desc.Code, desc.ExitCode, desc.HTTPStatus)
	}
	if !strings.HasPrefix(desc.Message, "Draft batch failed: operation 0 (") {
		t.Fatalf("message = %q", desc.Message)
	}
	if strings.Contains(desc.Hint, "draft") || !strings.Contains(desc.Hint, "cannot read") {
		t.Fatalf("hint = %q", desc.Hint)
	}
	results, ok := desc.Details["results"].([]api.BatchOpResult)
	if !ok || len(results) != 1 || results[0].Error == nil || results[0].Error.Code != "unauthorized" {
		t.Fatalf("details = %#v", desc.Details)
	}
	if batchResultsFromError(err) == nil {
		t.Fatal("wrapped error lost the API error")
	}
}

func TestPagesBatchUnauthorizedOpIsPermissionError(t *testing.T) {
	body := fmt.Sprintf(draftBatchFailedBody, "Not authorized", "unauthorized", "Not authorized")
	fail := func(w http.ResponseWriter, _ *http.Request, _ int) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(body))
	}
	ops := `[{"op":"set","path":"Blokken[0].Image","value":{"__type":"FileRef","source":"nimbu://other/uploads/1"}}]`
	for _, draft := range []bool{false, true} {
		srvState := &surgicalServer{batchFn: fail, draftBatchFn: fail}
		_, err := runBatchFile(t, srvState, ops, &PagesBatchCmd{Draft: draft})
		if err == nil {
			t.Fatalf("draft=%v: expected failure", draft)
		}
		desc := classifyError(err)
		if desc.Code != errorAuthForbidden || desc.HTTPStatus != 422 {
			t.Fatalf("draft=%v: code=%s http=%d", draft, desc.Code, desc.HTTPStatus)
		}
		if results, ok := desc.Details["results"].([]api.BatchOpResult); !ok || len(results) != 1 {
			t.Fatalf("draft=%v: details = %#v", draft, desc.Details)
		}
	}
}
