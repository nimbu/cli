package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// The Nimbu job runner (lib/nimbu/cloud_code/runners/jobs.rb) brackets every
// run with these lines, logged in the job's "background job" context:
//
//	Starting job '<name>' with params '<json>'
//	Job '<name>' completed successfully     (runJob returned)
//	Runtime Error: <message>                (ERROR; runJob raised, no completed line)
//
// An async throw or rejected promise logs its message at ERROR and still ends
// with the completed line, so any ERROR inside a run marks it failed.
// response.error() without a log line is invisible here.
const (
	jobLogContextType   = "background job"
	jobRuntimeErrorLead = "Runtime Error: "
)

// jobRunTracker follows the runs of one job through its log lines. Logs carry
// no run id, so it counts runs instead: it starts at the first "Starting job"
// line, ignores lines from runs already going before that, and is done once
// every run it saw start has finished.
type jobRunTracker struct {
	job           string
	startPrefix   string
	completedLine string

	active       int
	runs         int
	firstError   string
	runtimeError string
}

func newJobRunTracker(job string) *jobRunTracker {
	return &jobRunTracker{
		job:           job,
		startPrefix:   fmt.Sprintf("Starting job '%s' with params '", job),
		completedLine: fmt.Sprintf("Job '%s' completed successfully", job),
	}
}

func (t *jobRunTracker) observe(log api.AppLog) (bool, error) {
	if !t.ownsLog(log) {
		return false, nil
	}
	text := renderAppLogData(log.Data)
	isError := appLogIsError(log.Level)
	switch {
	case strings.HasPrefix(text, t.startPrefix):
		t.active++
		t.runs++
		return false, nil
	case t.active == 0:
		return false, nil
	case text == t.completedLine:
		t.active--
	case isError && strings.HasPrefix(text, jobRuntimeErrorLead):
		if t.runtimeError == "" {
			t.runtimeError = strings.TrimPrefix(text, jobRuntimeErrorLead)
		}
		t.active--
	case isError:
		if t.firstError == "" {
			t.firstError = text
		}
	}
	if t.active > 0 {
		return false, nil
	}
	return true, t.result()
}

func (t *jobRunTracker) done() bool {
	return t.runs > 0 && t.active == 0
}

func (t *jobRunTracker) ownsLog(log api.AppLog) bool {
	var source struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if len(log.Context) == 0 || json.Unmarshal(log.Context, &source) != nil {
		return false
	}
	return source.Type == jobLogContextType && source.Name == t.job
}

func (t *jobRunTracker) result() error {
	reason := t.runtimeError
	if reason == "" {
		reason = t.firstError
	}
	if reason == "" {
		return nil
	}
	return newDetailedError(fmt.Errorf("job %q failed: %s", t.job, reason), errorJobFailed, ExitGeneral,
		map[string]any{"job": t.job, "runs": t.runs})
}

func appLogIsError(level string) bool {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "ERROR", "FATAL":
		return true
	}
	return false
}

// waitForJob tails the job's cloud-code logs from the moment it was scheduled
// until the run finishes, the timeout passes, or the user interrupts.
func (c *JobsRunCmd) waitForJob(ctx context.Context, client *api.Client, site, appKey, job string, scheduledAt time.Time) error {
	waitCtx := ctx
	if c.WaitTimeout > 0 {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, c.WaitTimeout)
		defer cancel()
	}

	tracker := newJobRunTracker(job)
	err := (&AppsLogsCmd{}).tail(waitCtx, client, appKey, api.AppLogOptions{
		Since: formatAppLogEpoch(scheduledAt.Add(-time.Second)),
		Job:   job,
		Limit: appsLogsDefaultLimit,
	}, tracker.observe)

	if tracker.done() {
		if tracker.runs > 1 {
			_, _ = fmt.Fprintf(output.WriterFromContext(ctx).Err,
				"note: %d runs of job %q overlapped; logs carry no run id, so this result covers all of them\n", tracker.runs, job)
		}
		return err
	}
	if err != nil {
		return err
	}
	if ctx.Err() == nil && errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
		return newHintedError(fmt.Errorf("job %q did not finish within %s", job, c.WaitTimeout), errorJobWaitTimeout, ExitGeneral,
			fmt.Sprintf("the job may still be running; follow it with: nimbu apps logs --site %s --app %s --job %s --tail", site, appKey, job))
	}
	// Interrupted: the job keeps running server-side.
	return nil
}
