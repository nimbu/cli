package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

func TestParsePageThemeHint(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		kind   string
		item   string
		canvas string
		ok     bool
	}{
		{
			name: "invalid editable message",
			err:  &api.Error{StatusCode: 422, Message: "invalid editable Hero Title"},
			kind: "editable",
			item: "Hero Title",
			ok:   true,
		},
		{
			name:   "invalid slug message",
			err:    &api.Error{StatusCode: 422, Message: "invalid slug (hero_stage) for repeatable in canvas 'Blokken'"},
			kind:   "repeatable",
			item:   "hero_stage",
			canvas: "Blokken",
			ok:     true,
		},
		{
			name: "item not found",
			err:  &api.Error{StatusCode: 422, Message: "Item 'Hero Title' not found"},
			kind: "editable",
			item: "Hero Title",
			ok:   true,
		},
		{
			name: "item not found in repeatable",
			err:  &api.Error{StatusCode: 422, Message: "Item 'Title' not found in repeatable"},
			kind: "editable",
			item: "Title",
			ok:   true,
		},
		{
			name: "structured invalid_editable wins over message",
			err: &api.Error{
				StatusCode: 422,
				Code:       "invalid_editable",
				Message:    "invalid slug (ignored) for repeatable in canvas 'Nope'",
				Details:    map[string]any{"data": map[string]any{"name": "Intro"}},
			},
			kind: "editable",
			item: "Intro",
			ok:   true,
		},
		{
			name: "structured invalid_slug",
			err: &api.Error{
				StatusCode: 422,
				Code:       "invalid_slug",
				Details:    map[string]any{"data": map[string]any{"name": "hero_stage", "canvas": "Blokken"}},
			},
			kind:   "repeatable",
			item:   "hero_stage",
			canvas: "Blokken",
			ok:     true,
		},
		{
			name: "unrelated 422",
			err:  &api.Error{StatusCode: 422, Message: "title is required"},
			ok:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parsePageThemeHint(tt.err)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v (%#v)", ok, tt.ok, got)
			}
			if !ok {
				return
			}
			if got.Kind != tt.kind || got.Name != tt.item || got.Canvas != tt.canvas {
				t.Fatalf("got %#v, want kind=%s name=%s canvas=%s", got, tt.kind, tt.item, tt.canvas)
			}
		})
	}
}

func TestPageThemePushHintUsesLocalThemeFiles(t *testing.T) {
	root := t.TempDir()
	writeThemeProject(t, root, map[string]string{
		"nimbu.yml":             "theme: demo\n",
		"templates/page.liquid": "{% canvas 'Blokken' %}{% repeatable 'hero_stage' %}\n",
	})
	t.Chdir(root)

	err := &api.Error{StatusCode: 422, Message: "invalid slug (hero_stage) for repeatable in canvas 'Blokken'"}
	hint := pageThemePushHint(err, "page")
	want := "repeatable 'hero_stage' (canvas 'Blokken') is not in the pushed theme. Push the template first: nimbu themes push --only templates/page.liquid --dry-run"
	if hint != want {
		t.Fatalf("hint = %q, want %q", hint, want)
	}
}

func TestPageThemePushHintWithoutThemeProject(t *testing.T) {
	t.Chdir(t.TempDir())

	err := &api.Error{StatusCode: 422, Message: "invalid slug (hero_stage) for repeatable in canvas 'Blokken'"}
	hint := pageThemePushHint(err, "page")
	want := "repeatable 'hero_stage' (canvas 'Blokken') is not in the pushed theme. Push the theme that defines it first: nimbu themes push --only templates/page.liquid --dry-run"
	if hint != want {
		t.Fatalf("hint = %q, want %q", hint, want)
	}

	hint = pageThemePushHint(err, "")
	if !strings.Contains(hint, "nimbu themes push --only templates/<template>.liquid --dry-run") {
		t.Fatalf("placeholder hint = %q", hint)
	}
}

func TestPageThemePushHintFromBatchPathNotFound(t *testing.T) {
	err := &api.Error{
		StatusCode: 422,
		Message:    "Atomic batch failed",
		Code:       "atomic_failure",
		Raw: map[string]any{
			"results": []any{
				map[string]any{
					"index":  0,
					"status": "error",
					"path":   "/items/Blokken/items/Title",
					"error":  map[string]any{"code": "path_not_found", "message": "Item 'Title' not found in repeatable"},
				},
			},
		},
	}
	got, ok := parsePageThemeHint(err)
	if !ok || got.Kind != "editable" || got.Name != "Title" {
		t.Fatalf("got %#v ok=%v", got, ok)
	}

	t.Chdir(t.TempDir())
	hint := pageThemePushHint(err, "home")
	if !strings.Contains(hint, "editable 'Title' is not in the pushed theme") {
		t.Fatalf("hint = %q", hint)
	}
	if !strings.Contains(hint, "nimbu themes push --only templates/home.liquid --dry-run") {
		t.Fatalf("hint = %q", hint)
	}
}

func TestClassifyErrorOverlaysPageThemeHint(t *testing.T) {
	t.Chdir(t.TempDir())
	err := withPageThemeHint(&api.Error{
		StatusCode: 422,
		Message:    "invalid slug (hero_stage) for repeatable in canvas 'Blokken'",
	}, "page")
	desc := classifyError(err)
	if desc.Code != errorRequestValidation {
		t.Fatalf("code = %s", desc.Code)
	}
	if desc.HTTPStatus != 422 {
		t.Fatalf("http = %d", desc.HTTPStatus)
	}
	if !strings.Contains(desc.Hint, "repeatable 'hero_stage' (canvas 'Blokken')") {
		t.Fatalf("hint = %q", desc.Hint)
	}
}

func TestPagesUpdateSurfacesThemeHint(t *testing.T) {
	root := t.TempDir()
	writeThemeProject(t, root, map[string]string{
		"nimbu.yml":             "theme: demo\n",
		"templates/page.liquid": "{% repeatable 'hero_stage' %}\n",
	})
	t.Chdir(root)

	srv := newThemeHintPageServer(t, http.MethodPatch, `{"message":"invalid slug (hero_stage) for repeatable in canvas 'Blokken'"}`)
	defer srv.Close()

	file := writePageFile(t, `{"template":"page","items":{"Blokken":{"type":"canvas","repeatables":[{"slug":"hero_stage"}]}}}`)
	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	err := (&PagesUpdateCmd{Page: "about", File: file}).Run(ctx, &RootFlags{Site: "demo"})
	if err == nil {
		t.Fatal("expected 422")
	}
	desc := classifyError(err)
	if desc.Code != errorRequestValidation || !strings.Contains(desc.Hint, "Push the template first: nimbu themes push --only templates/page.liquid --dry-run") {
		t.Fatalf("code=%s hint=%q", desc.Code, desc.Hint)
	}
}

func TestPagesCreateSurfacesThemeHint(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := newThemeHintPageServer(t, http.MethodPost, `{"message":"invalid editable Intro"}`)
	defer srv.Close()

	file := writePageFile(t, `{"template":"home","title":"About"}`)
	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	err := (&PagesCreateCmd{File: file}).Run(ctx, &RootFlags{Site: "demo"})
	if err == nil {
		t.Fatal("expected 422")
	}
	desc := classifyError(err)
	if desc.Code != errorRequestValidation || !strings.Contains(desc.Hint, "editable 'Intro'") {
		t.Fatalf("code=%s hint=%q", desc.Code, desc.Hint)
	}
	if !strings.Contains(desc.Hint, "templates/home.liquid") {
		t.Fatalf("hint = %q", desc.Hint)
	}
}

func TestPagesSetBatchSurfacesThemeHint(t *testing.T) {
	t.Chdir(t.TempDir())
	srvState := &surgicalServer{}
	srvState.batchFn = func(w http.ResponseWriter, _ *http.Request, _ int) {
		w.WriteHeader(422)
		_, _ = w.Write([]byte(`{
			"message":"Atomic batch failed",
			"code":"atomic_failure",
			"results":[{"index":0,"status":"error","path":"/items/Title","error":{"code":"path_not_found","message":"Item 'Title' not found"}}]
		}`))
	}
	srv := srvState.start(t)
	defer srv.Close()

	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	err := (&PagesSetCmd{Page: "about", Path: "title", Value: "X"}).Run(ctx, &RootFlags{Site: "demo"})
	if err == nil {
		t.Fatal("expected 422")
	}
	desc := classifyError(err)
	if desc.Code != errorRequestValidation || !strings.Contains(desc.Hint, "editable 'Title'") {
		t.Fatalf("code=%s hint=%q", desc.Code, desc.Hint)
	}
}

func newThemeHintPageServer(t *testing.T, method, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"id":"p1","fullpath":"about","template":"page","items":{}}`))
			return
		}
		if r.Method == method {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(body))
			return
		}
		http.NotFound(w, r)
	}))
}
