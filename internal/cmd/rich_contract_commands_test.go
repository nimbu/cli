package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

func TestPagesUpdateRejectsDeepInlineAssignments(t *testing.T) {
	ctx, _, _ := newContractTestContext(t, "https://api.example.test", output.Mode{})
	cmd := &PagesUpdateCmd{
		Page:        "about",
		Assignments: []string{"items.hero.title=Hello"},
	}

	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "richer document contract") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPagesUpdateUsesPatchReplaceAndPreservesDocument(t *testing.T) {
	var gotMethod string
	var gotReplace string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pages/about":
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`{"id":"p1","fullpath":"about","title":"Old","items":{"hero":{"text":"Welcome"}}}`))
				return
			}
			gotMethod = r.Method
			gotReplace = r.URL.Query().Get("replace")
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode patch body: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"p1","fullpath":"about","title":"New","items":{"hero":{"text":"Welcome"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &PagesUpdateCmd{
		Page:        "about",
		Assignments: []string{"title=New"},
	}

	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run pages update: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Fatalf("expected PATCH, got %s", gotMethod)
	}
	if gotReplace != "1" {
		t.Fatalf("expected replace=1, got %q", gotReplace)
	}
	if gotBody["title"] != "New" {
		t.Fatalf("expected merged title, got %#v", gotBody["title"])
	}
	if _, ok := gotBody["items"]; !ok {
		t.Fatalf("expected existing document preserved, got %#v", gotBody)
	}
}

func TestPagesGetDownloadsAssetsAndReturnsJSONDocument(t *testing.T) {
	const assetBody = "page-asset"
	assetURL := "/assets/hero.jpg"
	assetBase := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pages/about":
			_, _ = w.Write([]byte(`{"id":"p1","fullpath":"about","items":{"hero":{"file":{"url":"` + assetBase + assetURL + `","filename":"hero.jpg"}}}}`))
		case assetURL:
			_, _ = w.Write([]byte(assetBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	assetBase = srv.URL

	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	dir := t.TempDir()
	cmd := &PagesGetCmd{
		Page:           "about",
		DownloadAssets: dir,
	}

	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run pages get: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(out.Bytes(), &body); err != nil {
		t.Fatalf("decode output json: %v", err)
	}
	items := body["items"].(map[string]any)
	hero := items["hero"].(map[string]any)
	file := hero["file"].(map[string]any)
	attachmentPath := file["attachment_path"].(string)
	if attachmentPath == "" {
		t.Fatalf("expected attachment_path in JSON output, got %#v", file)
	}
	data, err := os.ReadFile(attachmentPath)
	if err != nil {
		t.Fatalf("read downloaded asset: %v", err)
	}
	if string(data) != assetBody {
		t.Fatalf("unexpected asset data: %q", string(data))
	}
	if _, ok := file["url"]; ok {
		t.Fatalf("expected url removed after download")
	}
}

func TestMenusUpdateUsesPatchReplaceAndStripsTargetPage(t *testing.T) {
	var gotMethod string
	var gotReplace string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/menus/main":
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`{"id":"m1","slug":"main","handle":"main","name":"Main","items":[{"title":"Home","target_page":"home"}]}`))
				return
			}
			gotMethod = r.Method
			gotReplace = r.URL.Query().Get("replace")
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode patch body: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"m1","slug":"main","handle":"main","name":"Primary","items":[{"title":"Home"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, _, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &MenusUpdateCmd{
		Menu:        "main",
		Assignments: []string{"name=Primary"},
	}

	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run menus update: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Fatalf("expected PATCH, got %s", gotMethod)
	}
	if gotReplace != "1" {
		t.Fatalf("expected replace=1, got %q", gotReplace)
	}
	items := gotBody["items"].([]any)
	first := items[0].(map[string]any)
	if _, ok := first["target_page"]; ok {
		t.Fatalf("expected target_page stripped, got %#v", first)
	}
}

func TestChannelsGetHumanOutputIncludesDependencies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/channels/articles":
			_, _ = w.Write([]byte(`{
				"id":"c1",
				"slug":"articles",
				"name":"Articles",
				"description":"Article content",
				"acl":{"read":"public"},
				"customizations":[{"name":"author","label":"Author","type":"belongs_to","reference":"authors"}],
				"label_field":"title",
				"title_field":"title",
				"order_by":"published_at",
				"order_direction":"desc",
				"submittable":true
			}`))
		case "/channels":
			_, _ = w.Write([]byte(`[
				{"id":"c1","slug":"articles","name":"Articles","customizations":[{"name":"author","type":"belongs_to","reference":"authors"}]},
				{"id":"c2","slug":"authors","name":"Authors","customizations":[{"name":"featured_article","type":"belongs_to","reference":"articles"}]},
				{"id":"c3","slug":"topics","name":"Topics","customizations":[{"name":"owner","type":"belongs_to","reference":"authors"}]}
			]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &ChannelsGetCmd{Channel: "articles"}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run channels get: %v", err)
	}

	got := out.String()
	for _, needle := range []string{
		"Summary",
		"ACL",
		"Custom Fields (1)",
		"Dependency Summary",
		"direct deps:        authors",
		"circular:           true",
	} {
		if !strings.Contains(got, needle) {
			t.Fatalf("expected %q in output, got:\n%s", needle, got)
		}
	}
}

func TestChannelsFieldsHumanOutputShowsReadableSchemaDetails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/channels/articles":
			_, _ = w.Write([]byte(`{
				"id":"c1",
				"slug":"articles",
				"name":"Articles",
				"label_field":"title",
				"title_field":"title",
				"customizations":[
					{"name":"title","label":"Title","type":"string","required":true,"unique":true},
					{"name":"country","label":"Country","type":"select","localized":true,"select_options":[
						{"id":"opt_be","name":"Belgium","slug":"belgium","position":1},
						{"id":"opt_nl","name":"The Netherlands","slug":"netherlands","position":2}
					]},
					{"name":"author","label":"Author","type":"belongs_to","reference":"authors"},
					{"name":"location","label":"Location","type":"geo","geo_type":"point","hint":"Shown on map"}
				]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &ChannelsFieldsCmd{Channel: "articles"}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run channels fields: %v", err)
	}

	got := out.String()
	for _, needle := range []string{
		"articles  Articles",
		"title: title   label: title   fields: 4",
		"Core",
		"Choices",
		"Relations",
		"Advanced",
		"title",
		"string",
		"required",
		"unique",
		"country",
		"name",
		"slug",
		"id",
		"pos",
		"Belgium",
		"opt_be",
		"author",
		"-> authors",
		"location",
		"geo: point",
		"hint: Shown on map",
	} {
		if !strings.Contains(got, needle) {
			t.Fatalf("expected %q in output, got:\n%s", needle, got)
		}
	}
}

func TestChannelsFieldsHumanOutputUsesColorOnlyWhenEnabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/channels/articles":
			_, _ = w.Write([]byte(`{
				"id":"c1",
				"slug":"articles",
				"name":"Articles",
				"customizations":[
					{"name":"title","label":"Title","type":"string","required":true}
				]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	colorCtx, colorOut, _ := newContractTestContextWithWriter(t, srv.URL, output.Mode{}, &output.Writer{
		Out:   &bytes.Buffer{},
		Err:   &bytes.Buffer{},
		Mode:  output.Mode{},
		Color: "always",
		NoTTY: true,
	})
	colorCmd := &ChannelsFieldsCmd{Channel: "articles"}
	if err := colorCmd.Run(colorCtx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run channels fields with color: %v", err)
	}
	if !strings.Contains(colorOut.String(), "\x1b[") {
		t.Fatalf("expected ANSI color codes in output, got %q", colorOut.String())
	}

	plainCtx, plainOut, _ := newContractTestContext(t, srv.URL, output.Mode{})
	plainCmd := &ChannelsFieldsCmd{Channel: "articles"}
	if err := plainCmd.Run(plainCtx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run channels fields without color: %v", err)
	}
	if strings.Contains(plainOut.String(), "\x1b[") {
		t.Fatalf("did not expect ANSI color codes in output, got %q", plainOut.String())
	}
}

func TestChannelsFieldsPlainOutputUsesTSVRowsWithOptionsJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/channels/articles":
			_, _ = w.Write([]byte(`{
				"id":"c1",
				"slug":"articles",
				"name":"Articles",
				"customizations":[
					{"name":"country","label":"Country","type":"select","localized":true,"select_options":[
						{"id":"opt_be","name":"Belgium","slug":"belgium","position":1}
					]},
					{"name":"author","label":"Author","type":"belongs_to","required":true,"reference":"authors"}
				]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{Plain: true})
	cmd := &ChannelsFieldsCmd{Channel: "articles"}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run channels fields plain: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 plain rows, got %d: %q", len(lines), out.String())
	}
	if !strings.Contains(lines[0], "articles\tcountry\tCountry\tselect\tlocalized\t\t[{\"id\":\"opt_be\",\"name\":\"Belgium\",\"position\":1,\"slug\":\"belgium\"}]") {
		t.Fatalf("unexpected first plain row: %q", lines[0])
	}
	if !strings.Contains(lines[1], "articles\tauthor\tAuthor\tbelongs_to\trequired\tauthors\t[]") {
		t.Fatalf("unexpected second plain row: %q", lines[1])
	}
}

func TestCustomersFieldsHumanOutputUsesSharedSchemaRenderer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/customers/customizations":
			_, _ = w.Write([]byte(`[
				{"name":"email","label":"Email","type":"email","required":true,"unique":true},
				{"name":"segment","label":"Customer Segment","type":"select","select_options":[
					{"id":"seg_vip","name":"VIP Customers","slug":"vip-customers","position":1},
					{"id":"seg_repeat","name":"Repeat buyers","slug":"repeat-buyers","position":2}
				]},
				{"name":"account","label":"Account","type":"belongs_to","reference":"accounts"},
				{"name":"profile_location","label":"Profile Location","type":"geo","geo_type":"point","hint":"Used for proximity search"}
			]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{})
	cmd := &CustomersFieldsCmd{}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run customers fields: %v", err)
	}

	got := out.String()
	for _, needle := range []string{
		"customers",
		"fields: 4",
		"Core",
		"Choices",
		"Relations",
		"Advanced",
		"email",
		"required",
		"unique",
		"segment",
		"VIP Customers",
		"seg_vip",
		"account",
		"-> accounts",
		"profile_location",
		"geo: point",
		"hint: Used for proximity search",
	} {
		if !strings.Contains(got, needle) {
			t.Fatalf("expected %q in output, got:\n%s", needle, got)
		}
	}
}

func TestProductsFieldsPlainAliasMatchesTopLevelCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/products/customizations":
			_, _ = w.Write([]byte(`[
				{"name":"color","label":"Color","type":"select","select_options":[
					{"id":"clr_red","name":"Warm Red","slug":"warm-red","position":1}
				]},
				{"name":"brand","label":"Brand","type":"belongs_to","reference":"brands","required":true}
			]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("XDG_CONFIG_HOME", tempHome)
	t.Setenv("NIMBU_TOKEN", "test-token")

	topLevel := captureStdout(t, func() error {
		return execute([]string{"products", "fields", "--plain", "--site", "demo", "--apiurl", srv.URL})
	})
	alias := captureStdout(t, func() error {
		return execute([]string{"products", "config", "fields", "--plain", "--site", "demo", "--apiurl", srv.URL})
	})

	if topLevel != alias {
		t.Fatalf("expected alias output to match top-level command\nfields:\n%s\nalias:\n%s", topLevel, alias)
	}
	if !strings.Contains(topLevel, "products\tcolor\tColor\tselect\t\t\t[{\"id\":\"clr_red\",\"name\":\"Warm Red\",\"position\":1,\"slug\":\"warm-red\"}]") {
		t.Fatalf("unexpected top-level plain output: %q", topLevel)
	}
	if !strings.Contains(topLevel, "products\tbrand\tBrand\tbelongs_to\trequired\tbrands\t[]") {
		t.Fatalf("unexpected relation plain output: %q", topLevel)
	}
}

func newContractTestContext(t *testing.T, apiURL string, mode output.Mode) (context.Context, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	t.Setenv("NIMBU_TOKEN", "test-token")

	flags := &RootFlags{
		APIURL:  apiURL,
		Site:    "demo",
		Timeout: 2 * time.Second,
	}
	cfg := config.Defaults()
	cfg.DefaultSite = "demo"

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}

	ctx := context.Background()
	ctx = context.WithValue(ctx, rootFlagsKey{}, flags)
	ctx = context.WithValue(ctx, configKey{}, &cfg)
	ctx = output.WithMode(ctx, mode)
	ctx = output.WithWriter(ctx, &output.Writer{Out: out, Err: errOut, Mode: mode, NoTTY: true})
	return ctx, out, errOut
}

func newContractTestContextWithWriter(t *testing.T, apiURL string, mode output.Mode, writer *output.Writer) (context.Context, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	t.Setenv("NIMBU_TOKEN", "test-token")

	flags := &RootFlags{
		APIURL:  apiURL,
		Site:    "demo",
		Timeout: 2 * time.Second,
	}
	cfg := config.Defaults()
	cfg.DefaultSite = "demo"

	out, _ := writer.Out.(*bytes.Buffer)
	errOut, _ := writer.Err.(*bytes.Buffer)
	if out == nil {
		out = &bytes.Buffer{}
		writer.Out = out
	}
	if errOut == nil {
		errOut = &bytes.Buffer{}
		writer.Err = errOut
	}
	writer.Mode = mode

	ctx := context.Background()
	ctx = context.WithValue(ctx, rootFlagsKey{}, flags)
	ctx = context.WithValue(ctx, configKey{}, &cfg)
	ctx = output.WithMode(ctx, mode)
	ctx = output.WithWriter(ctx, writer)
	return ctx, out, errOut
}
