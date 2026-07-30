package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// JobsRunCmd schedules a cloud job.
type JobsRunCmd struct {
	Job         string   `help:"Job identifier"`
	File        string   `help:"Read job params JSON from file (use - for stdin)"`
	Wait        bool     `help:"After scheduling, tail the job's cloud-code logs until interrupted"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. foo=bar, retry:=true)"`
}

// Run executes the run command.
func (c *JobsRunCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "execute job"); err != nil {
		return err
	}

	job, assignments, err := c.resolveJobAndAssignments()
	if err != nil {
		return err
	}

	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}
	if err := requireScopes(ctx, client, []string{"write_cloudcode"}, "Example: nimbu auth scopes"); err != nil {
		return err
	}

	body, err := readJSONBodyInput(c.File, assignments)
	if err != nil {
		if !errors.Is(err, errNoJSONInput) {
			return err
		}
		body = map[string]any{}
	}

	appKey := ""
	if c.Wait {
		appKey, err = resolveJobAppKey(ctx, client, job)
		if err != nil {
			return err
		}
	}

	// The server hands the request body to the job as its params; wrapping it
	// in a {"params": ...} envelope would leak the envelope into the job.
	scheduledAt := appsLogsNow()
	path := "/jobs/" + url.PathEscape(job)
	var result api.JobRunResult
	if err := client.Post(ctx, path, body, &result); err != nil {
		var apiErr *api.Error
		if errors.As(err, &apiErr) && apiErr.IsNotFound() {
			return fmt.Errorf("schedule job: %w; job %q is not registered on the server (run 'nimbu jobs list' to see registered jobs; a just-pushed file may register late, push it again to refresh the registry)", err, job)
		}
		return hintJSONAssignments(fmt.Errorf("schedule job: %w", err), assignments)
	}

	mode := output.FromContext(ctx)
	switch {
	case mode.JSON:
		if err := output.JSON(ctx, result); err != nil {
			return err
		}
	case mode.Plain:
		if err := output.Plain(ctx, result.JID); err != nil {
			return err
		}
	default:
		if err := output.JSON(ctx, result); err != nil {
			return err
		}
	}

	if c.Wait {
		return c.waitForLogs(ctx, client, appKey, job, scheduledAt)
	}
	return nil
}

// resolveJobAndAssignments validates --job; the flag stays optional in the CLI
// grammar so a missing or positional job name gets a teaching error instead of
// kong's bare "missing flags: --job=STRING".
func (c *JobsRunCmd) resolveJobAndAssignments() (string, []string, error) {
	job := strings.TrimSpace(c.Job)
	if job == "" {
		if len(c.Assignments) > 0 && !strings.Contains(c.Assignments[0], "=") {
			return "", nil, fmt.Errorf("pass the job name as a flag: nimbu jobs run --job %s [assignments]", c.Assignments[0])
		}
		return "", nil, fmt.Errorf("job name required: nimbu jobs run --job <job> [assignments]")
	}
	return job, c.Assignments, nil
}

// waitForLogs tails the job's cloud-code logs from the moment it was scheduled.
func (c *JobsRunCmd) waitForLogs(ctx context.Context, client *api.Client, appKey, job string, scheduledAt time.Time) error {
	logsCmd := &AppsLogsCmd{}
	return logsCmd.tail(ctx, client, appKey, api.AppLogOptions{
		Since: formatAppLogEpoch(scheduledAt.Add(-time.Second)),
		Job:   job,
		Limit: appsLogsDefaultLimit,
	})
}
