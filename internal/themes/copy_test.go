package themes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nimbu/cli/internal/api"
)

func TestRunCopyTransfersLiquidAndAssets(t *testing.T) {
	var uploads []string
	sourceURL := ""
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/themes/source":
			_, _ = w.Write([]byte(`{
				"snippets":[{"name":"header.liquid"}],
				"assets":[{"path":"/images/logo.png","public_url":"` + sourceURL + `/downloads/logo.png"}]
			}`))
		case "/themes/source/snippets/header.liquid":
			_, _ = w.Write([]byte(`{"name":"header.liquid","code":"{{ header }}"}`))
		case "/themes/source/assets/images/logo.png":
			_, _ = w.Write([]byte(`{"path":"/images/logo.png","public_url":"` + sourceURL + `/downloads/logo.png"}`))
		case "/downloads/logo.png":
			_, _ = w.Write([]byte("logo-bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer source.Close()
	sourceURL = source.URL

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode upload: %v", err)
		}
		uploads = append(uploads, r.URL.Path+":"+body["name"].(string))
		_, _ = w.Write([]byte(`{}`))
	}))
	defer target.Close()

	result, err := RunCopy(
		context.Background(),
		api.New(source.URL, "token"),
		CopyRef{BaseURL: source.URL, Site: "from", Theme: "source"},
		api.New(target.URL, "token"),
		CopyRef{BaseURL: target.URL, Site: "to", Theme: "target"},
		CopyOptions{},
	)
	if err != nil {
		t.Fatalf("run copy: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("item count = %d", len(result.Items))
	}
	if uploads[0] != "/themes/target/assets:images/logo.png" || uploads[1] != "/themes/target/snippets:header.liquid" {
		t.Fatalf("unexpected uploads: %#v", uploads)
	}
}
