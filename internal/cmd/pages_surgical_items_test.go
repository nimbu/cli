package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func itemsServer(t *testing.T, data string) (*httptest.Server, *string) {
	t.Helper()
	var itemsPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/pages/about":
			_, _ = w.Write([]byte(surgicalPageJSON()))
		case strings.HasPrefix(r.URL.Path, "/pages/"+surgicalPageID+"/items/"):
			itemsPath = r.URL.Path
			_, _ = w.Write([]byte(`{"path":"/items/Blokken/repeatables/` + surgicalBlockID + `/items/Title",` +
				`"parent_path":"/items/Blokken","position":1,"siblings_count":2,"type":"item","data":` + data + `}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &itemsPath
}

const itemsHTMLData = `{"type":"text","content":"<p>Hero &amp; co</p>",` +
	`"translations":{"nl":{"content":"<p>Held</p>"}}}`

func TestPagesItemsHumanOutputKeepsHTMLReadable(t *testing.T) {
	srv, _ := itemsServer(t, itemsHTMLData)
	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &PagesItemsCmd{Page: "about", Path: "Blokken[0].Title"}
	if err := cmd.Run(ctx); err != nil {
		t.Fatalf("run: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "<p>Hero &amp; co</p>") {
		t.Fatalf("html not readable: %q", got)
	}
	if strings.Contains(got, "\\u003c") || strings.Contains(got, "\\u0026") {
		t.Fatalf("output still escapes html: %q", got)
	}
}

func TestPagesItemsLocaleProjection(t *testing.T) {
	t.Run("human", func(t *testing.T) {
		srv, _ := itemsServer(t, itemsHTMLData)
		ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{})
		cmd := &PagesItemsCmd{Page: "about", Path: "Blokken[0].Title", Locale: "nl"}
		if err := cmd.Run(ctx); err != nil {
			t.Fatalf("run: %v", err)
		}
		got := out.String()
		if !strings.Contains(got, `"content": "<p>Held</p>"`) {
			t.Fatalf("locale not projected: %q", got)
		}
	})

	t.Run("json stays raw without --locale", func(t *testing.T) {
		srv, _ := itemsServer(t, itemsHTMLData)
		ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
		cmd := &PagesItemsCmd{Page: "about", Path: "Blokken[0].Title"}
		if err := cmd.Run(ctx); err != nil {
			t.Fatalf("run: %v", err)
		}
		var body map[string]any
		if err := json.Unmarshal(out.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		data := body["data"].(map[string]any)
		if data["content"] != "<p>Hero &amp; co</p>" {
			t.Fatalf("json data = %#v", data)
		}
	})

	t.Run("json projects with --locale", func(t *testing.T) {
		srv, _ := itemsServer(t, itemsHTMLData)
		ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
		cmd := &PagesItemsCmd{Page: "about", Path: "Blokken[0].Title", Locale: "nl"}
		if err := cmd.Run(ctx); err != nil {
			t.Fatalf("run: %v", err)
		}
		var body map[string]any
		if err := json.Unmarshal(out.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		data := body["data"].(map[string]any)
		if data["content"] != "<p>Held</p>" {
			t.Fatalf("json data = %#v", data)
		}
	})
}

func TestPagesItemsRawIndexPath(t *testing.T) {
	srv, itemsPath := itemsServer(t, itemsHTMLData)
	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	cmd := &PagesItemsCmd{Page: "about", Path: "/items/Blokken/repeatables/1/items/Quote"}
	if err := cmd.Run(ctx); err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "/pages/" + surgicalPageID + "/items/Blokken/repeatables/" + surgicalBlockID2 + "/items/Quote"
	if *itemsPath != want {
		t.Fatalf("items path = %s, want %s", *itemsPath, want)
	}
}
