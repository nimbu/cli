package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nimbu/cli/internal/output"
)

func jobsTestServer(t *testing.T, status int, response string) (*httptest.Server, *capturedRequest) {
	t.Helper()
	captured := &capturedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user" {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		captured.method = r.Method
		captured.path = r.URL.Path
		data, _ := io.ReadAll(r.Body)
		captured.body = nil
		if len(data) > 0 {
			_ = json.Unmarshal(data, &captured.body)
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
		}
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(srv.Close)
	return srv, captured
}

func TestJobsRunPositionalJobFlattensParams(t *testing.T) {
	srv, captured := jobsTestServer(t, http.StatusOK, `{"jid":"j1"}`)
	ctx, out := newAppsTestContextWithOut(t, srv.URL)

	cmd := &JobsRunCmd{Job: "recalculateActivitiesAmounts", Assignments: []string{"jaar:=2026"}}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo", APIURL: srv.URL}); err != nil {
		t.Fatalf("run job: %v", err)
	}

	if captured.method != http.MethodPost || captured.path != "/jobs/recalculateActivitiesAmounts" {
		t.Fatalf("request = %s %s, want POST /jobs/recalculateActivitiesAmounts", captured.method, captured.path)
	}
	if captured.body["jaar"] != float64(2026) {
		t.Fatalf("jaar = %v, want 2026", captured.body["jaar"])
	}
	if _, exists := captured.body["params"]; exists {
		t.Fatalf("body should not wrap params in an envelope: %#v", captured.body)
	}
	if !strings.Contains(out.String(), "j1") {
		t.Fatalf("output missing jid: %q", out.String())
	}
}

func TestJobsRunPositionalJobNameSuggestsFlag(t *testing.T) {
	ctx, _ := newAppsTestContextWithOut(t, "https://api.example.test")

	cmd := &JobsRunCmd{Assignments: []string{"recalculateActivitiesAmounts", "jaar:=2026"}}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), "--job recalculateActivitiesAmounts") {
		t.Fatalf("expected flag suggestion, got %v", err)
	}
}

func TestJobsRunMissingJobErrors(t *testing.T) {
	ctx, _ := newAppsTestContextWithOut(t, "https://api.example.test")

	cmd := &JobsRunCmd{Assignments: []string{"foo=bar"}}
	err := cmd.Run(ctx, &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), "--job <job>") {
		t.Fatalf("expected job-name usage error, got %v", err)
	}
}

func TestJobsRunNotFoundHintsJobsList(t *testing.T) {
	srv, _ := jobsTestServer(t, http.StatusNotFound, `{"message":"Not Found"}`)
	ctx, _ := newAppsTestContextWithOut(t, srv.URL)

	cmd := &JobsRunCmd{Job: "missingJob"}
	err := cmd.Run(ctx, &RootFlags{Site: "demo", APIURL: srv.URL})
	if err == nil || !strings.Contains(err.Error(), "nimbu jobs list") {
		t.Fatalf("expected jobs list hint, got %v", err)
	}
}

func TestJobsRunValidationHintsJSONAssignment(t *testing.T) {
	srv, _ := jobsTestServer(t, http.StatusUnprocessableEntity, `{"message":"Validation Failed"}`)
	ctx, _ := newAppsTestContextWithOut(t, srv.URL)

	cmd := &JobsRunCmd{Job: "myJob", Assignments: []string{`activiteiten=["a","b"]`}}
	err := cmd.Run(ctx, &RootFlags{Site: "demo", APIURL: srv.URL})
	if err == nil || !strings.Contains(err.Error(), "activiteiten:=") {
		t.Fatalf("expected := hint, got %v", err)
	}
}

func TestJobsRunWaitResolvesOwningAppOutsideProject(t *testing.T) {
	originalPollInterval := appsLogsPollInterval
	appsLogsPollInterval = time.Hour
	defer func() { appsLogsPollInterval = originalPollInterval }()

	baseCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var requestsMu sync.Mutex
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			requestsMu.Lock()
			requests = append(requests, r.Method+" "+r.URL.Path)
			requestsMu.Unlock()
		}

		switch r.URL.Path {
		case "/user":
			_, _ = w.Write([]byte(`{}`))
		case "/apps":
			_, _ = w.Write([]byte(`[{"key":"storefront","name":"Storefront"}]`))
		case "/apps/storefront":
			_, _ = w.Write([]byte(`{"key":"storefront","name":"Storefront","jobs":[{"name":"reindex"}]}`))
		case "/jobs/reindex":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode job body: %v", err)
			}
			if body["year"] != float64(2026) {
				t.Fatalf("year = %#v, want 2026", body["year"])
			}
			_, _ = w.Write([]byte(`{"jid":"j1"}`))
		case "/apps/storefront/logs":
			if got := r.URL.Query().Get("job"); got != "reindex" {
				t.Fatalf("job log filter = %q, want reindex", got)
			}
			_, _ = w.Write([]byte(`[]`))
			go func() {
				time.Sleep(10 * time.Millisecond)
				cancel()
			}()
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx, _, _ := newAppsLogsTestContextFromBase(t, baseCtx, server.URL, output.Mode{})
	withTempCWD(t, t.TempDir(), func() {
		cmd := &JobsRunCmd{Job: "reindex", Wait: true, Assignments: []string{"year:=2026"}}
		if err := cmd.Run(ctx, &RootFlags{Site: "demo", APIURL: server.URL}); err != nil {
			t.Fatalf("run job with wait outside project: %v", err)
		}
	})

	requestsMu.Lock()
	got := append([]string(nil), requests...)
	requestsMu.Unlock()
	want := []string{
		"GET /apps",
		"GET /apps/storefront",
		"POST /jobs/reindex",
		"GET /apps/storefront/logs",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("request order = %#v, want %#v", got, want)
	}
}

func TestJobsRunWaitDoesNotScheduleWhenJobIsUnregistered(t *testing.T) {
	var scheduled atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user":
			_, _ = w.Write([]byte(`{}`))
		case "/apps":
			_, _ = w.Write([]byte(`[{"key":"storefront","name":"Storefront"}]`))
		case "/apps/storefront":
			_, _ = w.Write([]byte(`{"key":"storefront","name":"Storefront","jobs":[{"name":"another_job"}]}`))
		case "/jobs/missing":
			scheduled.Add(1)
			_, _ = w.Write([]byte(`{"jid":"unexpected"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx, _ := newAppsTestContextWithOut(t, server.URL)
	cmd := &JobsRunCmd{Job: "missing", Wait: true}
	err := cmd.Run(ctx, &RootFlags{Site: "demo", APIURL: server.URL})
	if err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("expected unregistered job error, got %v", err)
	}
	if got := scheduled.Load(); got != 0 {
		t.Fatalf("scheduled requests = %d, want 0", got)
	}
}

func TestJobsRunWaitDoesNotScheduleAmbiguousJob(t *testing.T) {
	var scheduled atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user":
			_, _ = w.Write([]byte(`{}`))
		case "/apps":
			_, _ = w.Write([]byte(`[
				{"key":"storefront","name":"Storefront"},
				{"key":"backoffice","name":"Backoffice"}
			]`))
		case "/apps/storefront":
			_, _ = w.Write([]byte(`{"key":"storefront","jobs":[{"name":"reindex"}]}`))
		case "/apps/backoffice":
			_, _ = w.Write([]byte(`{"key":"backoffice","jobs":[{"name":"reindex"}]}`))
		case "/jobs/reindex":
			scheduled.Add(1)
			_, _ = w.Write([]byte(`{"jid":"unexpected"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx, _ := newAppsTestContextWithOut(t, server.URL)
	cmd := &JobsRunCmd{Job: "reindex", Wait: true}
	err := cmd.Run(ctx, &RootFlags{Site: "demo", APIURL: server.URL})
	if err == nil || !strings.Contains(err.Error(), "multiple apps") {
		t.Fatalf("expected duplicate registration error, got %v", err)
	}
	if got := scheduled.Load(); got != 0 {
		t.Fatalf("scheduled requests = %d, want 0", got)
	}
}

func TestJobsListFlattensAppJobs(t *testing.T) {
	// The /apps index omits the job registry; only the detail endpoint has it.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/apps":
			_, _ = w.Write([]byte(`[
				{"key":"app1","name":"App One"},
				{"key":"app2","name":"App Two"}
			]`))
		case "/apps/app1":
			_, _ = w.Write([]byte(`{"key":"app1","name":"App One","jobs":[{"name":"zulu"},{"name":"alpha","every":"1h"}]}`))
		case "/apps/app2":
			_, _ = w.Write([]byte(`{"key":"app2","name":"App Two","jobs":[{"name":"beta"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	ctx, out := newAppsTestContextWithOut(t, srv.URL)

	cmd := &JobsListCmd{}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo", APIURL: srv.URL}); err != nil {
		t.Fatalf("list jobs: %v", err)
	}

	text := out.String()
	for _, want := range []string{"alpha", "zulu", "beta", "App One", "App Two", "1h"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q: %q", want, text)
		}
	}
	if strings.Index(text, "alpha") > strings.Index(text, "zulu") {
		t.Fatalf("jobs not sorted within app: %q", text)
	}
}

func TestJobsListFiltersByApp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/apps":
			_, _ = w.Write([]byte(`[
				{"key":"app1","name":"App One"},
				{"key":"app2","name":"App Two"}
			]`))
		case "/apps/app2":
			_, _ = w.Write([]byte(`{"key":"app2","name":"App Two","jobs":[{"name":"beta"}]}`))
		default:
			// app1 is filtered out, so its detail must not be fetched.
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	ctx, out := newAppsTestContextWithOut(t, srv.URL)

	cmd := &JobsListCmd{App: "App Two"}
	if err := cmd.Run(ctx, &RootFlags{Site: "demo", APIURL: srv.URL}); err != nil {
		t.Fatalf("list jobs: %v", err)
	}

	text := out.String()
	if strings.Contains(text, "alpha") || !strings.Contains(text, "beta") {
		t.Fatalf("filter not applied: %q", text)
	}
}
