package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

func TestAppsLogsResolvesConfiguredAppAndRendersText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apps/storefront/logs" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("limit") != "2" || query.Get("level") != "warn" || query.Get("q") != "checkout" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`[
			{"id":"2","time":1751716801.456,"level":"ERROR","data":"checkout failed","created_at":"2026-07-05T12:00:01.456Z","context":null},
			{"id":"1","time":1751716800.123,"level":"WARN","data":{"detail":"slow","count":2},"created_at":"2026-07-05T12:00:00.123Z","context":{"request_id":"req_1"}}
		]`))
	}))
	defer server.Close()

	ctx, stdout, _ := newAppsLogsTestContext(t, server.URL, output.Mode{})
	withTempCWD(t, t.TempDir(), func() {
		writeAppsProjectConfig(t, server.URL)

		cmd := &AppsLogsCmd{
			App:   "web",
			Limit: 2,
			Level: "warn",
			Query: "checkout",
		}
		if err := cmd.Run(ctx, &RootFlags{Site: "demo", APIURL: server.URL}); err != nil {
			t.Fatalf("run apps logs: %v", err)
		}
	})

	line1Time := localLogTime(t, "2026-07-05T12:00:01.456Z")
	line2Time := localLogTime(t, "2026-07-05T12:00:00.123Z")
	want := line1Time + " ERROR checkout failed\n" +
		line2Time + " WARN  {\"detail\":\"slow\",\"count\":2}\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestAppsLogsJSONWritesOneObjectPerLine(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apps/storefront/logs" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[
			{"id":"1","time":1751716800.123,"level":"INFO","data":"started","created_at":"2026-07-05T12:00:00.123Z","context":null},
			{"id":"2","time":1751716801.456,"level":"ERROR","data":{"message":"failed"},"created_at":"2026-07-05T12:00:01.456Z","context":null}
		]`))
	}))
	defer server.Close()

	ctx, stdout, _ := newAppsLogsTestContext(t, server.URL, output.Mode{JSON: true})
	cmd := &AppsLogsCmd{App: "storefront", Limit: 2}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo", APIURL: server.URL}); err != nil {
		t.Fatalf("run apps logs: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("line count = %d, output:\n%s", len(lines), stdout.String())
	}
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "[") {
			t.Fatalf("line %d is an array, want one object per line: %s", i, line)
		}
		var decoded map[string]any
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Fatalf("line %d is not JSON: %v", i, err)
		}
	}
}

func TestAppsLogsSinceDurationConvertsToEpoch(t *testing.T) {
	fixedNow := time.Date(2026, 7, 5, 12, 0, 0, 123_000_000, time.UTC)
	originalNow := appsLogsNow
	appsLogsNow = func() time.Time { return fixedNow }
	defer func() { appsLogsNow = originalNow }()

	wantSince := strconv.FormatFloat(float64(fixedNow.Add(-15*time.Minute).UnixNano())/1e9, 'f', 3, 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("since"); got != wantSince {
			t.Fatalf("since = %q, want %q; raw query %q", got, wantSince, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	ctx, _, _ := newAppsLogsTestContext(t, server.URL, output.Mode{})
	cmd := &AppsLogsCmd{App: "storefront", Since: "15m"}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo", APIURL: server.URL}); err != nil {
		t.Fatalf("run apps logs: %v", err)
	}
}

func TestAppsLogsTailPrintsInitialLogsOldestFirstThenDedupesBoundaryLogs(t *testing.T) {
	originalPollInterval := appsLogsPollInterval
	appsLogsPollInterval = 10 * time.Millisecond
	defer func() { appsLogsPollInterval = originalPollInterval }()

	baseCtx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		switch call {
		case 1:
			if got := r.URL.Query().Get("since"); got != "" {
				t.Fatalf("initial since = %q, want empty", got)
			}
			if got := r.URL.Query().Get("limit"); got != "2" {
				t.Fatalf("initial limit = %q, want 2", got)
			}
			_, _ = w.Write([]byte(`[
				{"id":"new","time":1751716801.456,"level":"ERROR","data":"new","created_at":"2026-07-05T12:00:01.456Z","context":null},
				{"id":"old","time":1751716800.123,"level":"INFO","data":"old","created_at":"2026-07-05T12:00:00.123Z","context":null}
			]`))
		case 2:
			if got := r.URL.Query().Get("since"); got != "1751716801.456" {
				t.Fatalf("poll since = %q", got)
			}
			_, _ = w.Write([]byte(`[
				{"id":"new","time":1751716801.456,"level":"ERROR","data":"new","created_at":"2026-07-05T12:00:01.456Z","context":null},
				{"id":"same-time","time":1751716801.456,"level":"INFO","data":"same time","created_at":"2026-07-05T12:00:01.456Z","context":null},
				{"id":"next","time":1751716802.789,"level":"WARN","data":"next","created_at":"2026-07-05T12:00:02.789Z","context":null}
			]`))
		case 3:
			if got := r.URL.Query().Get("since"); got != "1751716802.789" {
				t.Fatalf("second poll since = %q", got)
			}
			_, _ = w.Write([]byte(`[
				{"id":"next","time":1751716802.789,"level":"WARN","data":"next","created_at":"2026-07-05T12:00:02.789Z","context":null}
			]`))
			go func() {
				time.Sleep(5 * time.Millisecond)
				cancel()
			}()
		default:
			// The cancel above lands asynchronously; on coarse timers (Windows)
			// the poll loop can fire a few more times before it does.
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer server.Close()

	ctx, stdout, _ := newAppsLogsTestContextFromBase(t, baseCtx, server.URL, output.Mode{})
	cmd := &AppsLogsCmd{App: "storefront", Limit: 2, Tail: true}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo", APIURL: server.URL}); err != nil {
		t.Fatalf("run apps logs tail: %v", err)
	}

	oldTime := localLogTime(t, "2026-07-05T12:00:00.123Z")
	newTime := localLogTime(t, "2026-07-05T12:00:01.456Z")
	nextTime := localLogTime(t, "2026-07-05T12:00:02.789Z")
	want := oldTime + " INFO  old\n" +
		newTime + " ERROR new\n" +
		newTime + " INFO  same time\n" +
		nextTime + " WARN  next\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

func newAppsLogsTestContext(t *testing.T, apiURL string, mode output.Mode) (context.Context, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	return newAppsLogsTestContextFromBase(t, context.Background(), apiURL, mode)
}

func newAppsLogsTestContextFromBase(t *testing.T, base context.Context, apiURL string, mode output.Mode) (context.Context, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	t.Setenv("NIMBU_TOKEN", "test-token")

	flags := &RootFlags{APIURL: apiURL, Site: "demo"}
	cfg := config.Defaults()
	cfg.DefaultSite = "demo"
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	ctx := base
	ctx = context.WithValue(ctx, rootFlagsKey{}, flags)
	ctx = context.WithValue(ctx, configKey{}, &cfg)
	ctx = output.WithMode(ctx, mode)
	ctx = output.WithWriter(ctx, &output.Writer{Out: stdout, Err: stderr, Mode: mode, NoTTY: true, Color: "never"})
	return ctx, stdout, stderr
}

func writeAppsProjectConfig(t *testing.T, apiURL string) {
	t.Helper()
	host := strings.TrimPrefix(apiURL, "http://")
	host = strings.TrimPrefix(host, "https://")
	project := "site: demo\napps:\n  - id: storefront\n    name: web\n    dir: code\n    glob: \"**/*.js\"\n    host: " + host + "\n    site: demo\n"
	if err := os.WriteFile(filepath.Join(".", config.ProjectFileName), []byte(project), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}
}

func localLogTime(t *testing.T, raw string) string {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}
	return parsed.Local().Format("2006-01-02 15:04:05.000")
}
