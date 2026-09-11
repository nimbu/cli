package devproxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPreviewInjectorApplyMatchesPageAndLocalePrefix(t *testing.T) {
	inj := NewPreviewInjector(map[string]string{
		"/zz-cli-test/surgical":    "tok-surgical",
		"/en/zz-cli-test/surgical": "tok-en",
	})

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "exact fullpath", path: "/zz-cli-test/surgical", want: "tok-surgical"},
		{name: "locale prefix on default fullpath", path: "/en/zz-cli-test/surgical", want: "tok-en"},
		{name: "inferred locale prefix", path: "/nl/zz-cli-test/surgical", want: "tok-surgical"},
		{name: "child path is not the page", path: "/zz-cli-test/surgical/child", want: ""},
		{name: "unrelated path", path: "/other", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+tt.path, nil)
			inj.Apply(req)
			got := req.URL.Query().Get("preview")
			if got != tt.want {
				t.Fatalf("preview = %q, want %q (rawquery=%q)", got, tt.want, req.URL.RawQuery)
			}
		})
	}
}

func TestPreviewInjectorPreservesExistingQueryAndSkipsIfPreviewSet(t *testing.T) {
	inj := NewPreviewInjector(map[string]string{"/about": "tok"})

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/about?utm=1", nil)
	inj.Apply(req)
	if req.URL.Query().Get("preview") != "tok" || req.URL.Query().Get("utm") != "1" {
		t.Fatalf("query = %q", req.URL.RawQuery)
	}

	existing := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/about?preview=already", nil)
	inj.Apply(existing)
	if existing.URL.Query().Get("preview") != "already" {
		t.Fatalf("existing preview overwritten: %q", existing.URL.RawQuery)
	}
}
