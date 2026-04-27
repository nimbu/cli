package themes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/observer"
)

type recordingThemeObserver struct {
	warnings []string
}

func (o *recordingThemeObserver) StageStart(string)                      {}
func (o *recordingThemeObserver) StageItem(string, string, int64, int64) {}
func (o *recordingThemeObserver) StageDone(string, string)               {}
func (o *recordingThemeObserver) StageSkip(string, string)               {}
func (o *recordingThemeObserver) SubStageDone(string, string, string)    {}
func (o *recordingThemeObserver) StageWarning(stage, msg string) {
	o.warnings = append(o.warnings, stage+": "+msg)
}

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
	if uploads[0] != "/themes/target/snippets:header.liquid" || uploads[1] != "/themes/target/assets:images/logo.png" {
		t.Fatalf("unexpected uploads: %#v", uploads)
	}
}

func TestRunCopyContinuesAfterResourceErrorWhenRequested(t *testing.T) {
	var uploads []string
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/themes/source":
			_, _ = w.Write([]byte(`{
				"templates":[
					{"name":"bad.liquid"},
					{"name":"good.liquid"}
				]
			}`))
		case "/themes/source/templates/bad.liquid":
			_, _ = w.Write([]byte(`{"name":"bad.liquid","code":"bad"}`))
		case "/themes/source/templates/good.liquid":
			_, _ = w.Write([]byte(`{"name":"good.liquid","code":"good"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer source.Close()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode upload: %v", err)
		}
		uploads = append(uploads, body["name"].(string))
		if body["name"] == "bad.liquid" {
			http.Error(w, "invalid template", http.StatusUnprocessableEntity)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer target.Close()

	obs := &recordingThemeObserver{}
	ctx := observer.WithCopyObserver(context.Background(), obs)
	result, err := RunCopy(
		ctx,
		api.New(source.URL, "token"),
		CopyRef{BaseURL: source.URL, Site: "from", Theme: "source"},
		api.New(target.URL, "token"),
		CopyRef{BaseURL: target.URL, Site: "to", Theme: "target"},
		CopyOptions{ContinueOnError: true},
	)
	if err != nil {
		t.Fatalf("run copy: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].RemoteName != "good.liquid" {
		t.Fatalf("unexpected copied items: %#v", result.Items)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].RemoteName != "bad.liquid" {
		t.Fatalf("unexpected skipped items: %#v", result.Skipped)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings count = %d, want 1: %#v", len(result.Warnings), result.Warnings)
	}
	if len(obs.warnings) != 1 || obs.warnings[0] != "Theme: "+result.Warnings[0] {
		t.Fatalf("stage warnings = %#v, want Theme-scoped warning %q", obs.warnings, result.Warnings[0])
	}
	if len(uploads) != 2 || uploads[0] != "bad.liquid" || uploads[1] != "good.liquid" {
		t.Fatalf("unexpected uploads: %#v", uploads)
	}
}

func TestRunCopyDryRunDoesNotFetchResourceContent(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/themes/source":
			_, _ = w.Write([]byte(`{
				"templates":[{"name":"page.liquid"}],
				"snippets":[{"name":"header.liquid"}],
				"assets":[{"path":"/images/logo.png"}]
			}`))
		case "/themes/source/templates/page.liquid", "/themes/source/snippets/header.liquid", "/themes/source/assets/images/logo.png":
			t.Fatalf("dry run unexpectedly fetched resource content at %s", r.URL.Path)
		default:
			http.NotFound(w, r)
		}
	}))
	defer source.Close()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("dry run unexpectedly wrote to target: %s %s", r.Method, r.URL.Path)
	}))
	defer target.Close()

	result, err := RunCopy(
		context.Background(),
		api.New(source.URL, "token"),
		CopyRef{BaseURL: source.URL, Site: "from", Theme: "source"},
		api.New(target.URL, "token"),
		CopyRef{BaseURL: target.URL, Site: "to", Theme: "target"},
		CopyOptions{DryRun: true},
	)
	if err != nil {
		t.Fatalf("run copy: %v", err)
	}
	if len(result.Items) != 3 {
		t.Fatalf("dry-run item count = %d, want 3", len(result.Items))
	}
}

func TestRunCopyUploadsLiquidCodeOnlyAndIgnoresTemplateItems(t *testing.T) {
	var captured []map[string]any
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/themes/source":
			_, _ = w.Write([]byte(`{
				"templates":[
					{
						"name":"page.liquid",
						"template_items":[
							{"slug":"title","type":"text"},
							{"slug":"old_title","type":"text","disabled":true}
						]
					}
				]
			}`))
		case "/themes/source/templates/page.liquid":
			_, _ = w.Write([]byte(`{"name":"page.liquid","code":"{{ title }}"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer source.Close()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode upload: %v", err)
		}
		captured = append(captured, body)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer target.Close()

	_, err := RunCopy(
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

	if len(captured) != 1 {
		t.Fatalf("captured uploads = %#v", captured)
	}
	if captured[0]["name"] != "page.liquid" || captured[0]["code"] != "{{ title }}" {
		t.Fatalf("unexpected code payload: %#v", captured[0])
	}
	if _, ok := captured[0]["template_items"]; ok {
		t.Fatalf("unexpected template_items payload: %#v", captured[0])
	}
}

func TestRunCopyOrdersLiquidByDependenciesAndAssetsLast(t *testing.T) {
	var uploads []string
	sourceURL := ""
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/themes/source":
			_, _ = w.Write([]byte(`{
				"assets":[{"path":"/images/logo.png","public_url":"` + sourceURL + `/downloads/logo.png"}],
				"layouts":[{"name":"default.liquid"}],
				"snippets":[
					{"name":"body.liquid"},
					{"name":"atoms/button.liquid"}
				],
				"templates":[{"name":"page.liquid"}]
			}`))
		case "/themes/source/snippets/atoms/button.liquid":
			_, _ = w.Write([]byte(`{"name":"atoms/button.liquid","code":"button"}`))
		case "/themes/source/snippets/body.liquid":
			_, _ = w.Write([]byte(`{"name":"body.liquid","code":"{% include \"atoms/button\" %}"}`))
		case "/themes/source/layouts/default.liquid":
			_, _ = w.Write([]byte(`{"name":"default.liquid","code":"layout"}`))
		case "/themes/source/templates/page.liquid":
			_, _ = w.Write([]byte(`{
				"name":"page.liquid",
				"code":"{% layout \"default.liquid\" %}{% include \"body\" %}"
			}`))
		case "/downloads/logo.png":
			_, _ = w.Write([]byte("logo-bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer source.Close()
	sourceURL = source.URL

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode upload: %v", err)
		}
		uploads = append(uploads, r.URL.Path+":"+body["name"].(string))
		_, _ = w.Write([]byte(`{}`))
	}))
	defer target.Close()

	_, err := RunCopy(
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

	want := []string{
		"/themes/target/snippets:atoms/button.liquid",
		"/themes/target/snippets:body.liquid",
		"/themes/target/layouts:default.liquid",
		"/themes/target/templates:page.liquid",
		"/themes/target/assets:images/logo.png",
	}
	if !reflect.DeepEqual(uploads, want) {
		t.Fatalf("uploads = %#v, want %#v", uploads, want)
	}
}
