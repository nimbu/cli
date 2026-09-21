package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func TestPagesDeleteBlockRequiresForce(t *testing.T) {
	srvState := &surgicalServer{}
	srv := srvState.start(t)
	defer srv.Close()
	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &PagesDeleteBlockCmd{Page: "about", Path: "Blokken[0]"}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected force error, got %v", err)
	}
	if srvState.posts != 0 {
		t.Fatal("delete without force should not post")
	}

	if err := cmd.Run(ctx, &RootFlags{Site: "demo", Force: true}); err != nil {
		t.Fatalf("forced delete: %v", err)
	}
	op := srvState.lastBody["operations"].([]any)[0].(map[string]any)
	if op["op"] != "delete" || op["path"] != "/items/Blokken/repeatables/"+surgicalBlockID {
		t.Fatalf("op = %#v", op)
	}
}

func TestPagesMoveNoopAndPosition(t *testing.T) {
	t.Run("noop", func(t *testing.T) {
		srvState := &surgicalServer{}
		srv := srvState.start(t)
		defer srv.Close()
		zero := 0
		ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{})
		cmd := &PagesMoveCmd{Page: "about", Path: "Blokken[0]", Position: &zero}
		if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
			t.Fatalf("run: %v", err)
		}
		if srvState.posts != 0 {
			t.Fatal("noop should not post")
		}
		if !strings.Contains(out.String(), "already at position 0") {
			t.Fatalf("output = %q", out.String())
		}
	})

	t.Run("position", func(t *testing.T) {
		srvState := &surgicalServer{}
		srv := srvState.start(t)
		defer srv.Close()
		one := 1
		ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
		cmd := &PagesMoveCmd{Page: "about", Path: "Blokken[0]", Position: &one}
		if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
			t.Fatalf("run: %v", err)
		}
		op := srvState.lastBody["operations"].([]any)[0].(map[string]any)
		if op["op"] != "move" || op["after"] != surgicalBlockID2 {
			t.Fatalf("op = %#v", op)
		}
	})
}

func TestPagesBatchMixedPathsAndLimit(t *testing.T) {
	t.Run("mixed", func(t *testing.T) {
		srvState := &surgicalServer{}
		srv := srvState.start(t)
		defer srv.Close()
		file := writePageFile(t, `{"operations":[
			{"op":"set","path":"title","value":"Hi"},
			{"op":"set","path":"/published","value":true}
		]}`)
		ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
		cmd := &PagesBatchCmd{Page: "about", File: file, Atomic: true}
		if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
			t.Fatalf("run: %v", err)
		}
		ops := srvState.lastBody["operations"].([]any)
		if ops[0].(map[string]any)["path"] != "/title" {
			t.Fatalf("human path not resolved: %#v", ops[0])
		}
		if ops[1].(map[string]any)["path"] != "/published" {
			t.Fatalf("raw path changed: %#v", ops[1])
		}
	})

	t.Run("too many", func(t *testing.T) {
		ops := make([]map[string]any, 11)
		for i := range ops {
			ops[i] = map[string]any{"op": "set", "path": "/title", "value": i}
		}
		raw, _ := json.Marshal(ops)
		file := writePageFile(t, string(raw))
		srvState := &surgicalServer{}
		srv := srvState.start(t)
		defer srv.Close()
		ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
		cmd := &PagesBatchCmd{Page: "about", File: file, Atomic: true}
		err := cmd.Run(ctx, &RootFlags{Site: "demo"})
		if err == nil || !strings.Contains(err.Error(), "10") {
			t.Fatalf("expected 10-op limit, got %v", err)
		}
		if srvState.posts != 0 {
			t.Fatal("oversize batch should not post")
		}
	})
}

func TestPagesItemsPathTranslation(t *testing.T) {
	var itemsPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/pages/about":
			_, _ = w.Write([]byte(surgicalPageJSON()))
		case strings.HasPrefix(r.URL.Path, "/pages/"+surgicalPageID+"/items/"):
			itemsPath = r.URL.Path
			_, _ = w.Write([]byte(`{"path":"/items/Blokken/repeatables/` + surgicalBlockID + `/items/Title","parent_path":"/items/Blokken","position":1,"siblings_count":2,"type":"item","data":{"type":"text","content":"Hero"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesItemsCmd{Page: "about", Path: "Blokken[0].Title"}
	if err := cmd.Run(ctx); err != nil {
		t.Fatalf("run: %v", err)
	}
	if itemsPath != "/pages/"+surgicalPageID+"/items/Blokken/repeatables/"+surgicalBlockID+"/items/Title" {
		t.Fatalf("items path = %s", itemsPath)
	}
	var body map[string]any
	if err := json.Unmarshal(out.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["type"] != "item" {
		t.Fatalf("body = %#v", body)
	}
}

func TestPagesSchemaHumanRendering(t *testing.T) {
	srvState := &surgicalServer{}
	srv := srvState.start(t)
	defer srv.Close()
	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &PagesSchemaCmd{Page: "about"}
	if err := cmd.Run(ctx); err != nil {
		t.Fatalf("run: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Template: home") || !strings.Contains(got, "hero_stage (Hero)") || !strings.Contains(got, "Title (text)") {
		t.Fatalf("schema output = %q", got)
	}
}

func TestPagesGetShapeShowsIndexIDAndSelectOptions(t *testing.T) {
	srvState := &surgicalServer{}
	srv := srvState.start(t)
	defer srv.Close()

	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &PagesGetCmd{Page: "about", Shape: true}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "- hero_stage [0] "+surgicalBlockID) {
		t.Fatalf("missing index/id: %q", got)
	}
	if !strings.Contains(got, "Theme (select: Light | Dark | Auto)") {
		t.Fatalf("missing select options: %q", got)
	}

	ctxJSON, outJSON, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	if err := cmd.Run(ctxJSON, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run json: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(outJSON.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	theme := body["Theme"].(map[string]any)
	opts := theme["options"].([]any)
	if len(opts) != 3 || opts[0] != "Light" {
		t.Fatalf("options = %#v", opts)
	}
	rep := body["Blokken"].(map[string]any)["repeatables"].([]any)[0].(map[string]any)
	if rep["id"] != surgicalBlockID || rep["position"] != float64(0) || rep["slug"] != "hero_stage" {
		t.Fatalf("repeatable = %#v", rep)
	}
}

func TestPagesVerbsAcceptRawIndexPaths(t *testing.T) {
	t.Run("set", func(t *testing.T) {
		srvState := &surgicalServer{}
		srv := srvState.start(t)
		defer srv.Close()
		ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
		cmd := &PagesSetCmd{Page: "about", Path: "/items/Blokken/repeatables/1/items/Quote", Value: "Hi"}
		if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
			t.Fatalf("run: %v", err)
		}
		op := srvState.lastBody["operations"].([]any)[0].(map[string]any)
		if op["path"] != "/items/Blokken/repeatables/"+surgicalBlockID2+"/items/Quote" {
			t.Fatalf("set path = %#v", op["path"])
		}
	})

	t.Run("move", func(t *testing.T) {
		srvState := &surgicalServer{}
		srv := srvState.start(t)
		defer srv.Close()
		one := 1
		ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
		cmd := &PagesMoveCmd{Page: "about", Path: "/items/Blokken/repeatables/0", Position: &one}
		if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
			t.Fatalf("run: %v", err)
		}
		op := srvState.lastBody["operations"].([]any)[0].(map[string]any)
		if op["path"] != "/items/Blokken/repeatables/"+surgicalBlockID || op["after"] != surgicalBlockID2 {
			t.Fatalf("move op = %#v", op)
		}
	})

	t.Run("delete-block", func(t *testing.T) {
		srvState := &surgicalServer{}
		srv := srvState.start(t)
		defer srv.Close()
		ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
		cmd := &PagesDeleteBlockCmd{Page: "about", Path: "/items/Blokken/repeatables/1"}
		if err := cmd.Run(ctx, &RootFlags{Site: "demo", Force: true}); err != nil {
			t.Fatalf("run: %v", err)
		}
		op := srvState.lastBody["operations"].([]any)[0].(map[string]any)
		if op["op"] != "delete" || op["path"] != "/items/Blokken/repeatables/"+surgicalBlockID2 {
			t.Fatalf("delete op = %#v", op)
		}
	})

	t.Run("out of range index", func(t *testing.T) {
		srvState := &surgicalServer{}
		srv := srvState.start(t)
		defer srv.Close()
		ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
		cmd := &PagesSetCmd{Page: "about", Path: "/items/Blokken/repeatables/11/items/Quote", Value: "Hi"}
		err := cmd.Run(ctx, &RootFlags{Site: "demo"})
		if err == nil || !strings.Contains(err.Error(), "index 11") {
			t.Fatalf("error = %v", err)
		}
		if srvState.posts != 0 {
			t.Fatal("bad index should not post")
		}
	})
}
