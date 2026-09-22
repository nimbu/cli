package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/nimbu/cli/internal/output"
	"github.com/nimbu/cli/internal/realtime"
)

const watchTestGrant = "super-secret-grant-value"

func TestChannelEntriesWatchFlagSurface(t *testing.T) {
	parser, cli, err := newParser()
	if err != nil {
		t.Fatalf("newParser: %v", err)
	}

	args := []string{
		"channels", "entries", "watch",
		"--channel=blog",
		"--filters", "status=open",
		"--where", "published=true",
		"--once",
		"--for=30s",
		"--host=localhost:3000",
		"--insecure",
		"--timeout=5s",
		"--json",
	}
	kctx, err := parser.Parse(args)
	if err != nil {
		t.Fatalf("parse watch: %v", err)
	}
	if got := kctx.Command(); !strings.HasPrefix(got, "channels entries watch") {
		t.Fatalf("command = %q", got)
	}

	watch := cli.Channels.Entries.Watch
	if watch.Channel != "blog" {
		t.Fatalf("channel = %q", watch.Channel)
	}
	if len(watch.Filters) != 1 || watch.Filters[0] != "status=open" {
		t.Fatalf("filters = %#v", watch.Filters)
	}
	if watch.Where != "published=true" {
		t.Fatalf("where = %q", watch.Where)
	}
	if !watch.Once || !watch.Insecure {
		t.Fatalf("once = %v, insecure = %v", watch.Once, watch.Insecure)
	}
	if watch.For != 30*time.Second {
		t.Fatalf("--for = %v, want 30s", watch.For)
	}
	// --for must not shadow the global HTTP --timeout.
	if cli.Timeout != 5*time.Second {
		t.Fatalf("global --timeout = %v, want 5s", cli.Timeout)
	}
}

func TestChannelEntriesWatchAcceptsFilterAlias(t *testing.T) {
	parser, cli, err := newParser()
	if err != nil {
		t.Fatalf("newParser: %v", err)
	}
	if _, err := parser.Parse([]string{"channels", "entries", "watch", "--channel=blog", "--filter", "status=open"}); err != nil {
		t.Fatalf("parse --filter alias: %v", err)
	}
	if got := cli.Channels.Entries.Watch.Filters; len(got) != 1 || got[0] != "status=open" {
		t.Fatalf("filters = %#v", got)
	}
}

func TestChannelEntriesWatchRejectsUnsupportedQueries(t *testing.T) {
	tests := []struct {
		name    string
		cmd     ChannelEntriesWatchCmd
		message string
	}{
		{
			name:    "sort",
			cmd:     ChannelEntriesWatchCmd{Channel: "blog", Filters: []string{"sort=title"}},
			message: "watch does not support `sort`",
		},
		{
			name:    "regex",
			cmd:     ChannelEntriesWatchCmd{Channel: "blog", Filters: []string{"title.regex=foo"}},
			message: "`regex` is not supported on live queries",
		},
		{
			name:    "where conflict",
			cmd:     ChannelEntriesWatchCmd{Channel: "blog", Filters: []string{"where=a=1"}, Where: "b=2"},
			message: "--where conflicts with --filters",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, _, _ := newContractTestContext(t, "https://api.example.test", output.Mode{JSON: true})
			err := test.cmd.Run(ctx, &RootFlags{Site: "demo", APIURL: "https://api.example.test"})
			if err == nil {
				t.Fatal("expected a validation failure")
			}
			if !strings.Contains(err.Error(), test.message) {
				t.Fatalf("error = %q, want it to contain %q", err.Error(), test.message)
			}
			if code := classifyError(err).ExitCode; code != ExitUsage {
				t.Fatalf("exit code = %d, want %d", code, ExitUsage)
			}
		})
	}
}

func TestChannelEntriesWatchStreamsOneJSONEnvelope(t *testing.T) {
	event := `{"event_id":"ev-1","event":"added","resource":"channel_entries","parent_id":"blog","id":"e1","type":"channel_entries.created","object":{"id":"e1","title":"Hello"},"occurred_at":"2026-09-21T10:00:00Z","revision":7}`

	wsSrv := newWatchTestSocket(t, event)
	apiSrv := newWatchTestAPI(t)

	ctx, out, errOut := newContractTestContext(t, apiSrv.URL, output.Mode{JSON: true})
	cmd := &ChannelEntriesWatchCmd{Channel: "blog", Once: true, Host: wsSrv.URL, For: 5 * time.Second}

	if err := cmd.Run(ctx, &RootFlags{Site: "demo", APIURL: apiSrv.URL}); err != nil {
		t.Fatalf("run watch: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("stdout lines = %#v, want exactly one event envelope", lines)
	}

	var got, want map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("decode stdout line %q: %v", lines[0], err)
	}
	if err := json.Unmarshal([]byte(event), &want); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if got["event_id"] != want["event_id"] || got["type"] != want["type"] || got["revision"] != want["revision"] {
		t.Fatalf("stdout envelope = %#v, want %#v", got, want)
	}

	if strings.Contains(out.String(), watchTestGrant) || strings.Contains(errOut.String(), watchTestGrant) {
		t.Fatal("the realtime grant must never be printed")
	}
}

func TestChannelEntriesWatchHumanLineAndResyncHint(t *testing.T) {
	printer := &watchPrinter{
		writer:  &output.Writer{Out: &strings.Builder{}, Err: &strings.Builder{}, NoTTY: true},
		channel: "blog",
		filters: []string{"status=open"},
	}

	added := printer.humanEventLine(realtime.Event{
		Event:      "added",
		ID:         "e1",
		OccurredAt: "2026-09-21T10:00:00Z",
		Object:     json.RawMessage(`{"title":"Hello"}`),
	})
	if !strings.Contains(added, "added") || !strings.Contains(added, "e1") || !strings.Contains(added, "Hello") {
		t.Fatalf("added line = %q", added)
	}

	resync := printer.humanEventLine(realtime.Event{Event: "resync", OccurredAt: "2026-09-21T10:00:00Z"})
	if !strings.Contains(resync, "nimbu channels entries list --channel blog --filters status=open") {
		t.Fatalf("resync line = %q", resync)
	}
}

func TestWatchExitErrorMapsOutcomes(t *testing.T) {
	if err := watchExitError(realtime.ExitOnce, nil); err != nil {
		t.Fatalf("once = %v, want nil", err)
	}
	if err := watchExitError(realtime.ExitTimeout, nil); err != nil {
		t.Fatalf("timeout = %v, want nil", err)
	}
	if err := watchExitError(realtime.ExitInterrupted, nil); err != nil {
		t.Fatalf("interrupted = %v, want nil", err)
	}

	invalid := watchExitError(realtime.ExitFatal, &realtime.SubscriptionError{Code: "invalid_query"})
	if code := classifyError(invalid).ExitCode; code != ExitUsage {
		t.Fatalf("invalid_query exit code = %d, want %d", code, ExitUsage)
	}

	// A dial failure after the retry budget stays a plain failure, not the
	// retryable network class.
	transport := watchExitError(realtime.ExitFatal, &realtime.SubscriptionError{Code: "not_available"})
	if code := classifyError(transport).ExitCode; code != ExitGeneral {
		t.Fatalf("not_available exit code = %d, want %d", code, ExitGeneral)
	}
}

func TestNormalizeWatchHostKeepsDevScheme(t *testing.T) {
	tests := map[string]string{
		"":                       "",
		"  ":                     "",
		"example.com":            "example.com",
		"https://example.com/":   "example.com",
		"wss://example.com/ws":   "example.com",
		"http://localhost:3000":  "http://localhost:3000",
		"ws://localhost:3000/ws": "ws://localhost:3000",
	}
	for input, want := range tests {
		if got := normalizeWatchHost(input); got != want {
			t.Fatalf("normalizeWatchHost(%q) = %q, want %q", input, got, want)
		}
	}
}

// newWatchTestAPI serves the endpoints the watch command touches before dialing.
func newWatchTestAPI(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user":
			_, _ = w.Write([]byte(`{}`))
		case "/sites/demo":
			_, _ = w.Write([]byte(`{"id":"s1","subdomain":"demo"}`))
		case "/realtime/grants":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"grant":"` + watchTestGrant + `","expires_at":"2126-09-21T10:02:00Z"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newWatchTestSocket scripts an ActionCable server: welcome, confirm,
// subscribed, then one event envelope.
func newWatchTestSocket(t *testing.T, event string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws" {
			http.NotFound(w, r)
			return
		}
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols:       []string{realtime.Subprotocol},
			InsecureSkipVerify: true,
		})
		if err != nil {
			return
		}
		defer conn.CloseNow() //nolint:errcheck // test server teardown

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		write := func(payload string) bool {
			return conn.Write(ctx, websocket.MessageText, []byte(payload)) == nil
		}
		if !write(`{"type":"welcome"}`) {
			return
		}

		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var subscribe struct {
			Command    string `json:"command"`
			Identifier string `json:"identifier"`
		}
		if err := json.Unmarshal(data, &subscribe); err != nil || subscribe.Command != "subscribe" {
			return
		}
		identifier, err := json.Marshal(subscribe.Identifier)
		if err != nil {
			return
		}

		if !write(`{"type":"confirm_subscription","identifier":` + string(identifier) + `}`) {
			return
		}
		if !write(`{"identifier":` + string(identifier) + `,"message":{"type":"subscribed","subscription_id":"sub-1","resource":"channel_entries","firehose":false,"seq_at_create":1,"resync":false,"lease":{"expires_at":0,"renew_after":0}}}`) {
			return
		}
		if !write(`{"identifier":` + string(identifier) + `,"message":` + event + `}`) {
			return
		}
		<-ctx.Done()
	}))
	t.Cleanup(srv.Close)
	return srv
}
