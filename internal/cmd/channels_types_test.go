package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

const channelsTypesFixture = `[
  {"id":"c2","slug":"posts","name":"Posts","customizations":[
    {"name":"title","type":"string","required":true},
    {"name":"author","type":"belongs_to","reference":"authors"}
  ]},
  {"id":"c1","slug":"authors","name":"Authors","customizations":[
    {"name":"name","type":"string","required":true}
  ]}
]`

func newChannelsTypesServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/channels" {
			_, _ = w.Write([]byte(channelsTypesFixture))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestChannelsTypesWritesModuleToStdout(t *testing.T) {
	srv := newChannelsTypesServer(t)
	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{})

	if err := (&ChannelsTypesCmd{}).Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run channels types: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		`// Nimbu channel types for site "demo".`,
		"import type { ReferenceTo } from 'nimbu-js-sdk'",
		"export type Authors = {",
		"  author?: ReferenceTo<Authors> | null\n",
		"type __NimbuChannel_Authors = Authors\ntype __NimbuChannel_Posts = Posts\n",
		"declare module 'nimbu-js-sdk/cloud' {",
		"    'authors': __NimbuChannel_Authors\n    'posts': __NimbuChannel_Posts\n",
		// Every channel is included, so unknown slugs become a type error.
		"  interface NimbuCloudOptions {\n    strictChannels: true\n  }\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestChannelsTypesPlainPrintsModuleVerbatim(t *testing.T) {
	srv := newChannelsTypesServer(t)
	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{Plain: true})

	if err := (&ChannelsTypesCmd{}).Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run channels types: %v", err)
	}
	if !strings.HasPrefix(out.String(), "// Nimbu channel types") || strings.HasSuffix(out.String(), "\n\n") {
		t.Fatalf("expected the module verbatim, got:\n%q", out.String())
	}
}

func TestChannelsTypesJSONIncludesModule(t *testing.T) {
	srv := newChannelsTypesServer(t)
	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})

	if err := (&ChannelsTypesCmd{Channel: []string{"authors"}}).Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run channels types: %v", err)
	}

	var got channelsTypesResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	if got.Site != "demo" || len(got.Channels) != 1 || got.Channels[0] != "authors" || got.Path != "" {
		t.Fatalf("unexpected payload: %#v", got)
	}
	if !strings.Contains(got.TypeScript, "export type Authors") || strings.Contains(got.TypeScript, "export type Posts") ||
		strings.Contains(got.TypeScript, "NimbuCloudOptions") {
		t.Fatalf("unexpected module:\n%s", got.TypeScript)
	}
}

func TestChannelsTypesWritesOutputFile(t *testing.T) {
	srv := newChannelsTypesServer(t)
	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{JSON: true})
	path := filepath.Join(t.TempDir(), "types", "nimbu-channels.ts")

	cmd := &ChannelsTypesCmd{Channel: []string{"posts"}, Output: path}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run channels types: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	// authors is filtered out, so the reference falls back to the untyped helper.
	if !strings.Contains(string(data), "  author?: ReferenceTo | null\n") {
		t.Fatalf("unexpected module:\n%s", data)
	}

	var got channelsTypesResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if got.Path != path || got.TypeScript != "" || len(got.Channels) != 1 {
		t.Fatalf("unexpected payload: %#v", got)
	}
}

func TestSelectChannels(t *testing.T) {
	channels := []api.ChannelDetail{{ID: "c1", Slug: "authors"}, {ID: "c2", Slug: "posts"}}

	all, err := selectChannels(channels, nil)
	if err != nil || len(all) != 2 {
		t.Fatalf("expected all channels, got %v (%v)", all, err)
	}

	byID, err := selectChannels(channels, []string{"c2", " authors "})
	if err != nil || len(byID) != 2 {
		t.Fatalf("expected slug and ID matches, got %v (%v)", byID, err)
	}

	_, err = selectChannels(channels, []string{"posts", "zeta", "alpha"})
	if err == nil || err.Error() != "channel not found: alpha, zeta" {
		t.Fatalf("expected sorted not-found error, got %v", err)
	}
}

func TestChannelsInfoTypeScriptRendersStandaloneType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/channels/posts":
			_, _ = w.Write([]byte(`{"id":"c2","slug":"posts","name":"Posts","customizations":[
				{"name":"title","type":"string","required":true},
				{"name":"author","type":"belongs_to","reference":"authors"},
				{"name":"owner","type":"customer"}
			]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/channels":
			_, _ = w.Write([]byte(channelsTypesFixture))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	ctx, out, _ := newContractTestContext(t, srv.URL, output.Mode{})

	if err := (&ChannelsInfoCmd{Channel: "posts", TypeScript: true}).Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("run channels info: %v", err)
	}
	want := "import type { JSONField, ReferenceTo } from 'nimbu-js-sdk'\n\n" +
		"/** Posts (channel `posts`) */\n" +
		"export type Posts = {\n" +
		"  title: string\n" +
		"  author?: ReferenceTo | null\n" +
		"  owner?: JSONField | string | null\n" +
		"}\n"
	if got := out.String(); got != want {
		t.Fatalf("unexpected output:\n%s\nwant:\n%s", got, want)
	}
}
