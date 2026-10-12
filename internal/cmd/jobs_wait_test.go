package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

const jobContext = `{"type":"background job","name":"reindex"}`

func jobAppLog(level, text string) api.AppLog {
	return api.AppLog{Level: level, Data: json.RawMessage(strconv.Quote(text)), Context: json.RawMessage(jobContext)}
}

func observeAll(t *testing.T, tracker *jobRunTracker, logs ...api.AppLog) (bool, error) {
	t.Helper()
	for i, log := range logs {
		done, err := tracker.observe(log)
		if done {
			if i != len(logs)-1 {
				t.Fatalf("tracker finished at log %d of %d", i+1, len(logs))
			}
			return true, err
		}
		if err != nil {
			t.Fatalf("observe returned error without finishing: %v", err)
		}
	}
	return false, nil
}

func TestJobRunTrackerSucceedsOnCompletedLine(t *testing.T) {
	tracker := newJobRunTracker("reindex")
	done, err := observeAll(t, tracker,
		jobAppLog("INFO", `Starting job 'reindex' with params '{"year":2026}'`),
		jobAppLog("INFO", "reindexed 12 products"),
		jobAppLog("WARN", "skipped 1 product"),
		jobAppLog("INFO", "Job 'reindex' completed successfully"),
	)
	if !done || err != nil {
		t.Fatalf("done=%v err=%v, want done without error", done, err)
	}
}

func TestJobRunTrackerFailsOnRuntimeError(t *testing.T) {
	tracker := newJobRunTracker("reindex")
	done, err := observeAll(t, tracker,
		jobAppLog("INFO", `Starting job 'reindex' with params '{}'`),
		jobAppLog("ERROR", "Runtime Error: ReferenceError: foo is not defined (line 3, within job)"),
	)
	if !done || err == nil {
		t.Fatalf("done=%v err=%v, want done with error", done, err)
	}
	want := `job "reindex" failed: ReferenceError: foo is not defined (line 3, within job)`
	if err.Error() != want {
		t.Fatalf("err = %q, want %q", err, want)
	}
	if desc := classifyError(err); desc.Code != errorJobFailed || desc.ExitCode != ExitGeneral {
		t.Fatalf("code=%s exit=%d, want %s exit %d", desc.Code, desc.ExitCode, errorJobFailed, ExitGeneral)
	}
}

// An async throw logs the error and the runner still logs "completed".
func TestJobRunTrackerFailsOnErrorBeforeCompletedLine(t *testing.T) {
	tracker := newJobRunTracker("reindex")
	done, err := observeAll(t, tracker,
		jobAppLog("INFO", `Starting job 'reindex' with params '{}'`),
		jobAppLog("ERROR", "sync rejected: 502"),
		jobAppLog("ERROR", "second error"),
		jobAppLog("INFO", "Job 'reindex' completed successfully"),
	)
	if !done || err == nil {
		t.Fatalf("done=%v err=%v, want done with error", done, err)
	}
	if want := `job "reindex" failed: sync rejected: 502`; err.Error() != want {
		t.Fatalf("err = %q, want %q", err, want)
	}
}

func TestJobRunTrackerPrefersRuntimeErrorMessage(t *testing.T) {
	tracker := newJobRunTracker("reindex")
	_, err := observeAll(t, tracker,
		jobAppLog("INFO", `Starting job 'reindex' with params '{}'`),
		jobAppLog("ERROR", "could not sync order 12"),
		jobAppLog("ERROR", "Runtime Error: TypeError: x is undefined (line 9, within job)"),
	)
	if want := `job "reindex" failed: TypeError: x is undefined (line 9, within job)`; err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
}

// A run that was already going when we scheduled ours can finish first; its
// lines come before our "Starting job" line and must not end the wait.
func TestJobRunTrackerIgnoresRunsStartedBeforeScheduling(t *testing.T) {
	tracker := newJobRunTracker("reindex")
	done, err := observeAll(t, tracker,
		jobAppLog("ERROR", "earlier run broke"),
		jobAppLog("INFO", "Job 'reindex' completed successfully"),
		jobAppLog("ERROR", "Runtime Error: earlier run crashed"),
		jobAppLog("INFO", `Starting job 'reindex' with params '{}'`),
		jobAppLog("INFO", "Job 'reindex' completed successfully"),
	)
	if !done || err != nil {
		t.Fatalf("done=%v err=%v, want done without error", done, err)
	}
}

func TestJobRunTrackerWaitsForOverlappingRuns(t *testing.T) {
	tracker := newJobRunTracker("reindex")
	done, err := observeAll(t, tracker,
		jobAppLog("INFO", `Starting job 'reindex' with params '{}'`),
		jobAppLog("INFO", `Starting job 'reindex' with params '{"other":true}'`),
		jobAppLog("INFO", "Job 'reindex' completed successfully"),
		jobAppLog("INFO", "Job 'reindex' completed successfully"),
	)
	if !done || err != nil {
		t.Fatalf("done=%v err=%v, want done without error", done, err)
	}
	if tracker.runs != 2 {
		t.Fatalf("runs = %d, want 2", tracker.runs)
	}
}

func TestJobRunTrackerIgnoresOtherSources(t *testing.T) {
	tracker := newJobRunTracker("reindex")
	other := jobAppLog("INFO", "Job 'reindex' completed successfully")
	other.Context = json.RawMessage(`{"type":"background job","name":"reindex_all"}`)
	function := jobAppLog("ERROR", "Runtime Error: boom")
	function.Context = json.RawMessage(`{"type":"function","name":"reindex"}`)
	noContext := jobAppLog("INFO", "Job 'reindex' completed successfully")
	noContext.Context = nil

	done, err := observeAll(t, tracker,
		jobAppLog("INFO", `Starting job 'reindex' with params '{}'`),
		other,
		function,
		noContext,
		jobAppLog("INFO", "Job 'reindex' completed successfully"),
	)
	if !done || err != nil {
		t.Fatalf("done=%v err=%v, want done without error", done, err)
	}
}

func TestJobsRunWaitExitsWhenJobCompletes(t *testing.T) {
	withFastAppsLogsPolling(t)
	server := jobsWaitServer(t, [][]string{
		{},
		{
			jobLogJSON("l1", 1, "INFO", `Starting job 'reindex' with params '{}'`),
			jobLogJSON("l2", 2, "INFO", "reindexed"),
		},
		{jobLogJSON("l3", 3, "INFO", "Job 'reindex' completed successfully")},
	})

	baseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx, stdout, _ := newAppsLogsTestContextFromBase(t, baseCtx, server.URL, output.Mode{})
	withTempCWD(t, t.TempDir(), func() {
		cmd := &JobsRunCmd{Job: "reindex", Wait: true}
		if err := cmd.Run(ctx, &RootFlags{Site: "demo", APIURL: server.URL}); err != nil {
			t.Fatalf("run job with wait: %v", err)
		}
	})
	if baseCtx.Err() != nil {
		t.Fatal("--wait kept tailing after the job completed")
	}
	if !strings.Contains(stdout.String(), "Job 'reindex' completed successfully") {
		t.Fatalf("stdout misses completion line: %q", stdout.String())
	}
}

func TestJobsRunWaitReturnsJobFailedError(t *testing.T) {
	withFastAppsLogsPolling(t)
	server := jobsWaitServer(t, [][]string{{
		jobLogJSON("l1", 1, "INFO", `Starting job 'reindex' with params '{}'`),
		jobLogJSON("l2", 2, "ERROR", "Runtime Error: boom (line 1, within job)"),
	}})

	baseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx, _, _ := newAppsLogsTestContextFromBase(t, baseCtx, server.URL, output.Mode{})
	var err error
	withTempCWD(t, t.TempDir(), func() {
		cmd := &JobsRunCmd{Job: "reindex", Wait: true}
		err = cmd.Run(ctx, &RootFlags{Site: "demo", APIURL: server.URL})
	})
	if err == nil || err.Error() != `job "reindex" failed: boom (line 1, within job)` {
		t.Fatalf("err = %v, want job failed error", err)
	}
}

func TestJobsRunWaitTimeoutStopsWaiting(t *testing.T) {
	withFastAppsLogsPolling(t)
	server := jobsWaitServer(t, [][]string{{
		jobLogJSON("l1", 1, "INFO", `Starting job 'reindex' with params '{}'`),
	}})

	baseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx, _, _ := newAppsLogsTestContextFromBase(t, baseCtx, server.URL, output.Mode{})
	var err error
	withTempCWD(t, t.TempDir(), func() {
		cmd := &JobsRunCmd{Job: "reindex", Wait: true, WaitTimeout: 50 * time.Millisecond}
		err = cmd.Run(ctx, &RootFlags{Site: "demo", APIURL: server.URL})
	})
	if baseCtx.Err() != nil {
		t.Fatal("--wait-timeout did not stop the wait")
	}
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	desc := classifyError(err)
	if desc.Code != errorJobWaitTimeout {
		t.Fatalf("code = %s, want %s (err: %v)", desc.Code, errorJobWaitTimeout, err)
	}
	if want := "nimbu apps logs --site demo --app storefront --job reindex --tail"; !strings.Contains(desc.Hint, want) {
		t.Fatalf("hint = %q, want it to contain %q", desc.Hint, want)
	}
}

func TestJobsRunWaitTimeoutRequiresWait(t *testing.T) {
	cmd := &JobsRunCmd{Job: "reindex", WaitTimeout: time.Minute}
	err := cmd.Run(context.Background(), &RootFlags{Site: "demo"})
	if err == nil || !strings.Contains(err.Error(), "--wait-timeout requires --wait") {
		t.Fatalf("err = %v, want --wait-timeout usage error", err)
	}
}

func withFastAppsLogsPolling(t *testing.T) {
	t.Helper()
	poll, base, maxDelay := appsLogsPollInterval, appsLogsRetryBaseDelay, appsLogsRetryMaxDelay
	appsLogsPollInterval = 10 * time.Millisecond
	appsLogsRetryBaseDelay = time.Millisecond
	appsLogsRetryMaxDelay = 2 * time.Millisecond
	t.Cleanup(func() {
		appsLogsPollInterval, appsLogsRetryBaseDelay, appsLogsRetryMaxDelay = poll, base, maxDelay
	})
}

func jobLogJSON(id string, second int, level, text string) string {
	data, _ := json.Marshal(text)
	return fmt.Sprintf(`{"id":%q,"time":%d.5,"level":%q,"data":%s,"context":%s}`, id, 1751716800+second, level, data, jobContext)
}

// jobsWaitServer schedules job "reindex" on app "storefront" and answers the
// log polls with batches in order, then with empty pages.
func jobsWaitServer(t *testing.T, batches [][]string) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user":
			_, _ = w.Write([]byte(`{}`))
		case "/apps":
			_, _ = w.Write([]byte(`[{"key":"storefront","name":"Storefront"}]`))
		case "/apps/storefront":
			_, _ = w.Write([]byte(`{"key":"storefront","name":"Storefront","jobs":[{"name":"reindex"}]}`))
		case "/jobs/reindex":
			_, _ = w.Write([]byte(`{"jid":"j1"}`))
		case "/apps/storefront/logs":
			mu.Lock()
			batch := []string{}
			if len(batches) > 0 {
				batch, batches = batches[0], batches[1:]
			}
			mu.Unlock()
			_, _ = w.Write([]byte("[" + strings.Join(batch, ",") + "]"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}
