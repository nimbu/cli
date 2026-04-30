package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alecthomas/kong"

	"github.com/nimbu/cli/internal/auth"
	"github.com/nimbu/cli/internal/config"
)

func TestResolveCompletionRequestFindsDynamicFlagAssignments(t *testing.T) {
	req, ok := resolveCompletionRequest([]string{"nimbu", "sites", "copy"}, "--from=dem")
	if !ok {
		t.Fatal("expected completion target")
	}
	if got, want := strings.Join(req.CommandPath, " "), "sites copy"; got != want {
		t.Fatalf("command path = %q, want %q", got, want)
	}
	if req.Flag != "from" || req.Prefix != "dem" || !req.Assignment {
		t.Fatalf("unexpected request: %#v", req)
	}
}

func TestResolveCompletionRequestFindsDynamicFlagSpaceValues(t *testing.T) {
	req, ok := resolveCompletionRequest([]string{"nb", "--color=never", "sites", "copy", "--from"}, "dem")
	if !ok {
		t.Fatal("expected completion target")
	}
	if got, want := strings.Join(req.CommandPath, " "), "sites copy"; got != want {
		t.Fatalf("command path = %q, want %q", got, want)
	}
	if req.Flag != "from" || req.Prefix != "dem" || req.Assignment {
		t.Fatalf("unexpected request: %#v", req)
	}
}

func TestResolveCompletionRequestKeepsCommandsAfterBooleanFlags(t *testing.T) {
	req, ok := resolveCompletionRequest([]string{"nimbu", "--no-input", "sites", "copy"}, "--to=pro")
	if !ok {
		t.Fatal("expected completion target")
	}
	if got, want := strings.Join(req.CommandPath, " "), "sites copy"; got != want {
		t.Fatalf("command path = %q, want %q", got, want)
	}
}

func TestResolveCompletionRequestSkipsCommandFlagsWithValues(t *testing.T) {
	req, ok := resolveCompletionRequest([]string{"nimbu", "customers", "copy", "--password-length", "12"}, "--from=de")
	if !ok {
		t.Fatal("expected completion target")
	}
	if got, want := strings.Join(req.CommandPath, " "), "customers copy"; got != want {
		t.Fatalf("command path = %q, want %q", got, want)
	}
}

func TestCompletionValueFlagsCoversKongValueFlags(t *testing.T) {
	parser, _, err := newParser()
	if err != nil {
		t.Fatalf("newParser: %v", err)
	}

	var missing []string
	walkCompletionFlagNodes(parser.Model.Node, func(flag *kong.Flag) {
		if flag.IsBool() || flag.IsCounter() {
			return
		}
		if !completionValueFlags[flag.Name] {
			missing = append(missing, flag.Name)
		}
	})

	if len(missing) > 0 {
		t.Fatalf("completionValueFlags missing value flags: %s", strings.Join(missing, ", "))
	}
}

func TestCompletionRegistryResolvesKinds(t *testing.T) {
	registry := newCompletionRegistry()

	tests := []struct {
		name string
		path []string
		flag string
		want completionKind
	}{
		{name: "global site", path: []string{"channels", "list"}, flag: "site", want: completionKindSite},
		{name: "site copy", path: []string{"sites", "copy"}, flag: "from", want: completionKindSite},
		{name: "channel copy", path: []string{"channels", "entries", "copy"}, flag: "to", want: completionKindChannelRef},
		{name: "channel flag", path: []string{"channels", "fields", "update"}, flag: "channel", want: completionKindChannel},
		{name: "theme flag", path: []string{"themes", "files", "get"}, flag: "theme", want: completionKindTheme},
		{name: "theme copy", path: []string{"themes", "copy"}, flag: "from", want: completionKindThemeRef},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := registry.Lookup(tt.path, tt.flag)
			if !ok {
				t.Fatalf("expected registry entry")
			}
			if got != tt.want {
				t.Fatalf("kind = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCompleteDynamicSitesReturnsSubdomainsAndDescriptions(t *testing.T) {
	ctx := completionTestContext(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sites" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("authorization = %q", got)
		}
		_, _ = w.Write([]byte(`[
			{"id":"site-1","subdomain":"demo-shop","name":"Demo Shop"},
			{"id":"site-2","subdomain":"prod-shop","name":"Production"}
		]`))
	}))
	t.Cleanup(server.Close)

	flags := ctx.Value(rootFlagsKey{}).(*RootFlags)
	flags.APIURL = server.URL

	items, err := completeDynamic(ctx, completionRequest{
		Shell:       "zsh",
		Current:     "--from=de",
		CommandPath: []string{"sites", "copy"},
		Flag:        "from",
		Prefix:      "de",
		Assignment:  true,
	})
	if err != nil {
		t.Fatalf("completeDynamic: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
	if items[0].Value != "demo-shop" || items[0].Description != "Demo Shop (site-1)" {
		t.Fatalf("unexpected item: %#v", items[0])
	}
}

func TestCompleteDynamicChannelFlagReturnsBareChannelValues(t *testing.T) {
	ctx := completionTestContext(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/channels" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Nimbu-Site"); got != "demo" {
			t.Fatalf("site header = %q", got)
		}
		_, _ = w.Write([]byte(`[
			{"id":"c1","slug":"blog","name":"Blog"},
			{"id":"c2","slug":"brands","name":"Brands"}
		]`))
	}))
	t.Cleanup(server.Close)

	flags := ctx.Value(rootFlagsKey{}).(*RootFlags)
	flags.APIURL = server.URL
	flags.Site = "demo"

	items, err := completeDynamic(ctx, completionRequest{
		Shell:       "bash",
		Current:     "--channel=b",
		CommandPath: []string{"channels", "entries", "list"},
		Flag:        "channel",
		Prefix:      "b",
		Assignment:  true,
	})
	if err != nil {
		t.Fatalf("completeDynamic: %v", err)
	}
	if got := completionValues(items); strings.Join(got, ",") != "blog,brands" {
		t.Fatalf("values = %#v", got)
	}
}

func TestCompleteDynamicUsesOriginalCommandSiteAndAPIURL(t *testing.T) {
	ctx := completionTestContext(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/channels" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Nimbu-Site"); got != "staging" {
			t.Fatalf("site header = %q", got)
		}
		_, _ = w.Write([]byte(`[{"id":"c1","slug":"blog","name":"Blog"}]`))
	}))
	t.Cleanup(server.Close)

	flags := ctx.Value(rootFlagsKey{}).(*RootFlags)
	flags.APIURL = "http://127.0.0.1:1"

	items, err := completeDynamic(ctx, completionRequest{
		Shell:       "bash",
		Current:     "--channel=b",
		Words:       []string{"nimbu", "--apiurl", server.URL, "--site=staging", "channels", "entries", "list"},
		CommandPath: []string{"channels", "entries", "list"},
		Flag:        "channel",
		Prefix:      "b",
		Assignment:  true,
	})
	if err != nil {
		t.Fatalf("completeDynamic: %v", err)
	}
	if got := completionValues(items); strings.Join(got, ",") != "blog" {
		t.Fatalf("values = %#v", got)
	}
}

func TestMergeCompletionCachePreservesExistingEntries(t *testing.T) {
	now := time.Unix(100, 0)
	base := completionCache{Entries: map[string]completionCacheEntry{
		"sites": {FetchedAt: now.Add(-time.Minute), Items: []completionItem{{Value: "demo"}}},
	}}
	incoming := completionCache{Entries: map[string]completionCacheEntry{
		"channels": {FetchedAt: now, Items: []completionItem{{Value: "blog"}}},
	}}

	got := mergeCompletionCacheAt(base, incoming, now)
	if len(got.Entries) != 2 {
		t.Fatalf("entry count = %d, want 2", len(got.Entries))
	}
	if got.Entries["sites"].Items[0].Value != "demo" {
		t.Fatalf("existing entry was not preserved: %#v", got.Entries["sites"])
	}
	if got.Entries["channels"].Items[0].Value != "blog" {
		t.Fatalf("incoming entry was not added: %#v", got.Entries["channels"])
	}
}

func TestMergeCompletionCachePrunesExpiredEntries(t *testing.T) {
	now := time.Unix(1000, 0)
	base := completionCache{Entries: map[string]completionCacheEntry{
		"fresh": {FetchedAt: now.Add(-completionCacheTTL + time.Second), Items: []completionItem{{Value: "demo"}}},
		"stale": {FetchedAt: now.Add(-completionCacheTTL - time.Second), Items: []completionItem{{Value: "old"}}},
	}}
	incoming := completionCache{Entries: map[string]completionCacheEntry{
		"incoming": {FetchedAt: now, Items: []completionItem{{Value: "blog"}}},
	}}

	got := mergeCompletionCacheAt(base, incoming, now)
	if _, ok := got.Entries["stale"]; ok {
		t.Fatalf("stale entry was not pruned: %#v", got.Entries)
	}
	if _, ok := got.Entries["fresh"]; !ok {
		t.Fatalf("fresh entry was pruned: %#v", got.Entries)
	}
	if _, ok := got.Entries["incoming"]; !ok {
		t.Fatalf("incoming entry missing: %#v", got.Entries)
	}
}

func TestCompletionDebugWritesLogWhenEnabled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "data"))
	t.Setenv("NIMBU_COMPLETION_DEBUG", "1")

	completionDebugf("test %s", "message")

	dir, err := config.DataDir()
	if err != nil {
		t.Fatalf("data dir: %v", err)
	}
	data, err := os.ReadFile(completionDebugLogPath(dir))
	if err != nil {
		t.Fatalf("read debug log: %v", err)
	}
	if !strings.Contains(string(data), "nimbu completion: test message") {
		t.Fatalf("debug log = %q", data)
	}
}

func TestCompletionDebugLogTruncatesWhenTooLarge(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "data"))
	t.Setenv("NIMBU_COMPLETION_DEBUG", "1")

	dir, err := config.DataDir()
	if err != nil {
		t.Fatalf("data dir: %v", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir data dir: %v", err)
	}
	path := completionDebugLogPath(dir)
	large := strings.Repeat("x", completionDebugLogMaxBytes+1)
	if err := os.WriteFile(path, []byte(large), 0o600); err != nil {
		t.Fatalf("seed debug log: %v", err)
	}

	completionDebugf("after truncate")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read debug log: %v", err)
	}
	if strings.Contains(string(data), large[:1024]) {
		t.Fatal("debug log was appended instead of truncated")
	}
	if !strings.Contains(string(data), "nimbu completion: after truncate") {
		t.Fatalf("debug log = %q", data)
	}
}

func TestCompleteDynamicChannelRefFlagReturnsSiteChannelValues(t *testing.T) {
	ctx := completionTestContext(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sites":
			_, _ = w.Write([]byte(`[{"id":"site-1","subdomain":"demo","name":"Demo"}]`))
		case "/channels":
			if got := r.Header.Get("X-Nimbu-Site"); got != "demo" {
				t.Fatalf("site header = %q", got)
			}
			_, _ = w.Write([]byte(`[{"id":"c1","slug":"blog","name":"Blog"}]`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	flags := ctx.Value(rootFlagsKey{}).(*RootFlags)
	flags.APIURL = server.URL

	siteItems, err := completeDynamic(ctx, completionRequest{
		Shell:       "bash",
		Current:     "--from=d",
		CommandPath: []string{"channels", "entries", "copy"},
		Flag:        "from",
		Prefix:      "d",
		Assignment:  true,
	})
	if err != nil {
		t.Fatalf("completeDynamic sites: %v", err)
	}
	if got := completionValues(siteItems); strings.Join(got, ",") != "demo/" {
		t.Fatalf("site values = %#v", got)
	}

	channelItems, err := completeDynamic(ctx, completionRequest{
		Shell:       "bash",
		Current:     "--from=demo/b",
		CommandPath: []string{"channels", "entries", "copy"},
		Flag:        "from",
		Prefix:      "demo/b",
		Assignment:  true,
	})
	if err != nil {
		t.Fatalf("completeDynamic channels: %v", err)
	}
	if got := completionValues(channelItems); strings.Join(got, ",") != "demo/blog" {
		t.Fatalf("channel values = %#v", got)
	}
}

func TestCompleteDynamicThemeFlagReturnsBareThemeValues(t *testing.T) {
	ctx := completionTestContext(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/themes" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Nimbu-Site"); got != "demo" {
			t.Fatalf("site header = %q", got)
		}
		_, _ = w.Write([]byte(`[
			{"id":"theme-1","short":"storefront","name":"Storefront"},
			{"id":"theme-2","short":"sale","name":"Sale"}
		]`))
	}))
	t.Cleanup(server.Close)

	flags := ctx.Value(rootFlagsKey{}).(*RootFlags)
	flags.APIURL = server.URL
	flags.Site = "demo"

	items, err := completeDynamic(ctx, completionRequest{
		Shell:       "bash",
		Current:     "--theme=s",
		CommandPath: []string{"themes", "get"},
		Flag:        "theme",
		Prefix:      "s",
		Assignment:  true,
	})
	if err != nil {
		t.Fatalf("completeDynamic: %v", err)
	}
	if got := completionValues(items); strings.Join(got, ",") != "sale,storefront" {
		t.Fatalf("values = %#v", got)
	}
}

func TestCompleteDynamicFallsBackToStaleCacheOnRefreshFailure(t *testing.T) {
	ctx := completionTestContext(t)
	flags := ctx.Value(rootFlagsKey{}).(*RootFlags)
	flags.APIURL = "http://127.0.0.1:1"

	cache := completionCache{
		Entries: map[string]completionCacheEntry{
			completionCacheKey("http://127.0.0.1:1", "test-token", completionKindSite, ""): {
				FetchedAt: time.Now().Add(-10 * time.Minute),
				Items: []completionItem{
					{Value: "cached-site", Description: "Cached Site (cached-id)"},
				},
			},
		},
	}
	writeTestCompletionCache(t, cache)

	items, err := completeDynamic(ctx, completionRequest{
		Shell:       "bash",
		Current:     "--site=ca",
		CommandPath: []string{"channels", "list"},
		Flag:        "site",
		Prefix:      "ca",
		Assignment:  true,
	})
	if err != nil {
		t.Fatalf("completeDynamic: %v", err)
	}
	if len(items) != 1 || items[0].Value != "cached-site" {
		t.Fatalf("items = %#v", items)
	}
}

func completionValues(items []completionItem) []string {
	values := make([]string, 0, len(items))
	for _, item := range items {
		values = append(values, item.Value)
	}
	return values
}

func walkCompletionFlagNodes(node *kong.Node, fn func(*kong.Flag)) {
	if node == nil {
		return
	}
	for _, flag := range node.Flags {
		fn(flag)
	}
	for _, child := range node.Children {
		walkCompletionFlagNodes(child, fn)
	}
	if node.DefaultCmd != nil {
		walkCompletionFlagNodes(node.DefaultCmd, fn)
	}
}

func TestCompleteDynamicMissingAuthIsSilent(t *testing.T) {
	ctx := completionTestContext(t)
	if err := resolverFromContext(ctx).DeleteStoredCredentials(); err != nil {
		t.Fatalf("delete stored credentials: %v", err)
	}

	items, err := completeDynamic(ctx, completionRequest{
		Shell:       "bash",
		Current:     "--site=",
		CommandPath: []string{"sites", "list"},
		Flag:        "site",
		Prefix:      "",
		Assignment:  true,
	})
	if err != nil {
		t.Fatalf("completeDynamic: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items = %#v", items)
	}
}

func TestCompleteDynamicCommandPrintsCandidatesOnly(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`[{"id":"site-1","subdomain":"demo","name":"Demo"}]`))
	}))
	t.Cleanup(server.Close)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	withFakeAuthStore(t, &fakeAuthStore{
		credential: auth.Credential{Token: "test-token", Email: "me@example.com"},
	})

	code, stdout, stderr := captureExecute(t, []string{
		"--apiurl", server.URL,
		"__complete",
		"--shell", "bash",
		"--current=--from=d",
		"--",
		"nimbu", "sites", "copy",
	})
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q", stderr)
	}
	if strings.TrimSpace(stdout) != "demo" {
		t.Fatalf("stdout = %q", stdout)
	}
	if calls.Load() != 1 {
		t.Fatalf("api calls = %d", calls.Load())
	}
}

func TestGeneratedCompletionsContainDynamicHooks(t *testing.T) {
	bash := captureStdout(t, func() error { return writeBashCompletion(nil) })
	zsh := captureStdout(t, func() error { return writeZshCompletion(nil) })
	fish := captureStdout(t, func() error { return writeFishCompletion(nil) })

	for name, out := range map[string]string{"bash": bash, "zsh": zsh, "fish": fish} {
		if !strings.Contains(out, "__complete") {
			t.Fatalf("%s completion missing dynamic hook", name)
		}
		if !strings.Contains(out, "--from") || !strings.Contains(out, "--to") || !strings.Contains(out, "--site") || !strings.Contains(out, "--channel") || !strings.Contains(out, "--theme") {
			t.Fatalf("%s completion missing dynamic flags", name)
		}
	}
}

func TestGeneratedCompletionsContainFlagNameHooks(t *testing.T) {
	bash := captureStdout(t, func() error { return writeBashCompletion(nil) })
	zsh := captureStdout(t, func() error { return writeZshCompletion(nil) })
	fish := captureStdout(t, func() error { return writeFishCompletion(nil) })

	for name, out := range map[string]string{"bash": bash, "zsh": zsh, "fish": fish} {
		if !strings.Contains(out, "--flag-names") {
			t.Fatalf("%s completion missing flag-name hook", name)
		}
	}
}

func TestGeneratedCompletionsAvoidSpacesAfterDynamicPrefixes(t *testing.T) {
	bash := captureStdout(t, func() error { return writeBashCompletion(nil) })
	zsh := captureStdout(t, func() error { return writeZshCompletion(nil) })

	if !strings.Contains(bash, "compopt -o nospace") {
		t.Fatal("bash completion should suppress spaces after dynamic prefixes")
	}
	if !strings.Contains(zsh, "compadd -S ''") {
		t.Fatal("zsh completion should suppress spaces after dynamic prefixes")
	}
}

func TestGeneratedCompletionsDoNotRepeatAppsCodeCommand(t *testing.T) {
	bash := captureStdout(t, func() error { return writeBashCompletion(nil) })
	zsh := captureStdout(t, func() error { return writeZshCompletion(nil) })
	fish := captureStdout(t, func() error { return writeFishCompletion(nil) })

	if !strings.Contains(bash, `[[ ${COMP_CWORD} -eq 3 && ${COMP_WORDS[1]} == "apps" ]]`) {
		t.Fatal("bash apps completions should only run at the apps command depth")
	}
	if !strings.Contains(zsh, `if (( CURRENT != 3 )); then`) || !strings.Contains(zsh, `"apps code"`) {
		t.Fatal("zsh apps code completions should be depth-gated")
	}
	if !strings.Contains(fish, "not __fish_seen_subcommand_from list get config push code") {
		t.Fatal("fish apps completions should stop after an app subcommand is selected")
	}
}

func TestCompleteFlagNamesForCommand(t *testing.T) {
	parser, _, err := newParser()
	if err != nil {
		t.Fatalf("newParser: %v", err)
	}

	items := completeFlagNames(parser, completionRequest{
		Current:     "--",
		Words:       []string{"nimbu", "channels", "copy"},
		CommandPath: []string{"channels", "copy"},
		Prefix:      "--",
	})
	values := completionValues(items)

	for _, want := range []string{"--from", "--to", "--site"} {
		if !containsString(values, want) {
			t.Fatalf("flag values missing %s: %#v", want, values)
		}
	}
}

func TestCompleteFlagNamesFiltersPrefix(t *testing.T) {
	parser, _, err := newParser()
	if err != nil {
		t.Fatalf("newParser: %v", err)
	}

	items := completeFlagNames(parser, completionRequest{
		Current:     "--fr",
		Words:       []string{"nimbu", "channels", "copy"},
		CommandPath: []string{"channels", "copy"},
		Prefix:      "--fr",
	})
	values := completionValues(items)

	if !containsString(values, "--from") {
		t.Fatalf("flag values missing --from: %#v", values)
	}
	if containsString(values, "--to") {
		t.Fatalf("flag values should be filtered by prefix: %#v", values)
	}
}

func TestInternalCompleteCommandPrintsFlagNames(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())

	code, stdout, stderr := captureExecute(t, []string{
		"__complete",
		"--shell", "bash",
		"--flag-names",
		"--current=--fr",
		"--",
		"nimbu", "channels", "copy",
	})
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q", stderr)
	}
	values := strings.Fields(stdout)
	if !containsString(values, "--from") || containsString(values, "--to") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func completionTestContext(t *testing.T) context.Context {
	t.Helper()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	withFakeAuthStore(t, &fakeAuthStore{
		credential: auth.Credential{Token: "test-token", Email: "me@example.com"},
	})

	ctx := context.Background()
	flags := &RootFlags{APIURL: "https://api.nimbu.io", Timeout: 30 * time.Second}
	ctx = context.WithValue(ctx, rootFlagsKey{}, flags)
	ctx = context.WithValue(ctx, authResolverKey{}, newAuthCredentialResolver("api.nimbu.io"))
	return ctx
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func writeTestCompletionCache(t *testing.T, cache completionCache) {
	t.Helper()
	path, err := completionCachePath()
	if err != nil {
		t.Fatalf("completionCachePath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	data, err := json.Marshal(cache)
	if err != nil {
		t.Fatalf("marshal cache: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write cache: %v", err)
	}
}

func TestCompleteDynamicIgnoresProviderErrorsWithoutCache(t *testing.T) {
	ctx := completionTestContext(t)
	flags := ctx.Value(rootFlagsKey{}).(*RootFlags)
	flags.APIURL = "http://127.0.0.1:1"

	items, err := completeDynamic(ctx, completionRequest{
		Shell:       "bash",
		Current:     "--site=",
		CommandPath: []string{"sites", "list"},
		Flag:        "site",
		Prefix:      "",
		Assignment:  true,
	})
	if err != nil {
		t.Fatalf("completeDynamic should be silent, got: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items = %#v", items)
	}
}
