package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func writeAppsPushProject(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	files["nimbu.yml"] = "site: demo\napps:\n  - id: storefront\n    name: storefront\n    dir: code\n"
	for name, body := range files {
		abs := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}

func appsPushServer(t *testing.T, remote string) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var writes []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/apps/storefront/code" {
			_, _ = w.Write([]byte(remote))
			return
		}
		mu.Lock()
		writes = append(writes, r.Method+" "+r.URL.Path)
		mu.Unlock()
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string{}, writes...)
	}
}

// Regression: a bare require('lib/mailer') must upload lib/mailer.js before
// main.js, even though the old alphabetical fallback put main.js first.
func TestAppsPushUploadsRootRelativeRequiresBeforeMain(t *testing.T) {
	root := writeAppsPushProject(t, map[string]string{
		"code/main.js":       "require('lib/mailer');\nrequire('templates');",
		"code/lib/mailer.js": "module.exports = {};",
		"code/templates.js":  "module.exports = {};",
	})
	srv, writes := appsPushServer(t, `[{"name":"main.js"}]`)
	ctx := newAppsTestContext(t, srv.URL)

	withTempCWD(t, root, func() {
		if err := (&AppsPushCmd{}).Run(ctx, &RootFlags{Site: "demo", APIURL: srv.URL}); err != nil {
			t.Fatalf("push: %v", err)
		}
	})
	want := []string{
		"POST /apps/storefront/code",
		"POST /apps/storefront/code",
		"PUT /apps/storefront/code/main.js",
	}
	if got := writes(); !reflect.DeepEqual(got, want) {
		t.Fatalf("writes = %v, want %v", got, want)
	}
}

func TestAppsPushOnlyRefusesUnuploadedDependencies(t *testing.T) {
	root := writeAppsPushProject(t, map[string]string{
		"code/main.js":       "require('lib/mailer');",
		"code/lib/mailer.js": "module.exports = {};",
	})
	srv, writes := appsPushServer(t, `[{"name":"main.js"}]`)
	ctx := newAppsTestContext(t, srv.URL)

	var err error
	withTempCWD(t, root, func() {
		err = (&AppsPushCmd{Files: []string{"code/main.js"}}).Run(ctx, &RootFlags{Site: "demo", APIURL: srv.URL})
	})
	if err == nil || !strings.Contains(err.Error(), "main.js requires lib/mailer.js") {
		t.Fatalf("expected missing dependency error, got %v", err)
	}
	if got := writes(); len(got) != 0 {
		t.Fatalf("expected no writes before failing, got %v", got)
	}
}
