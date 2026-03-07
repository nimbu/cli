package themes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/nimbu/cli/internal/api"
)

func TestRunPullMapsAssetsByLongestRemoteBasePrefix(t *testing.T) {
	root := t.TempDir()
	cfg := Config{
		ProjectRoot: root,
		Theme:       "demo",
		Roots: []RootSpec{
			{Kind: KindTemplate, LocalPath: "templates"},
			{Kind: KindAsset, LocalPath: "assets", RemoteBase: ""},
			{Kind: KindAsset, LocalPath: "images", RemoteBase: "images"},
		},
	}

	assetURL := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/themes/demo":
			_, _ = w.Write([]byte(`{
				"templates":[{"name":"page.liquid"}],
				"assets":[
					{"path":"/images/hero.png","public_url":"` + assetURL + `/downloads/hero.png"},
					{"path":"/docs/readme.txt","public_url":"` + assetURL + `/downloads/readme.txt"}
				]
			}`))
		case "/themes/demo/templates/page.liquid":
			_, _ = w.Write([]byte(`{"name":"page.liquid","code":"{{ content }}"}`))
		case "/themes/demo/assets/images/hero.png":
			_, _ = w.Write([]byte(`{"path":"/images/hero.png","public_url":"` + assetURL + `/downloads/hero.png"}`))
		case "/themes/demo/assets/docs/readme.txt":
			_, _ = w.Write([]byte(`{"path":"/docs/readme.txt","public_url":"` + assetURL + `/downloads/readme.txt"}`))
		case "/downloads/hero.png":
			_, _ = w.Write([]byte("hero-bytes"))
		case "/downloads/readme.txt":
			_, _ = w.Write([]byte("readme-bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	assetURL = srv.URL

	client := api.New(srv.URL, "token")
	result, err := RunPull(context.Background(), client, cfg, Options{})
	if err != nil {
		t.Fatalf("run pull: %v", err)
	}
	if len(result.Written) != 3 {
		t.Fatalf("written count = %d", len(result.Written))
	}

	heroPath := filepath.Join(root, "images", "hero.png")
	if data, err := os.ReadFile(heroPath); err != nil || string(data) != "hero-bytes" {
		t.Fatalf("unexpected hero file: %q err=%v", string(data), err)
	}
	readmePath := filepath.Join(root, "assets", "docs", "readme.txt")
	if data, err := os.ReadFile(readmePath); err != nil || string(data) != "readme-bytes" {
		t.Fatalf("unexpected readme file: %q err=%v", string(data), err)
	}
}
