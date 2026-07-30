package cmd

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// JobsListCmd lists the cloud jobs the server knows about, across all apps.
type JobsListCmd struct {
	App string `help:"Only jobs from this app (key or name)"`
}

type jobsListRow struct {
	Job     string `json:"job"`
	App     string `json:"app"`
	AppKey  string `json:"app_key"`
	Every   string `json:"every,omitempty"`
	Updated string `json:"updated_at,omitempty"`
}

func loadRegisteredJobs(ctx context.Context, client *api.Client, appFilter string) ([]jobsListRow, error) {
	appsList, err := api.List[api.App](ctx, client, "/apps")
	if err != nil {
		return nil, err
	}

	filter := strings.TrimSpace(appFilter)
	rows := make([]jobsListRow, 0)
	for _, listed := range appsList {
		if filter != "" && !strings.EqualFold(listed.Key, filter) && !strings.EqualFold(listed.Name, filter) {
			continue
		}
		// The /apps index omits the job registry; only the detail endpoint
		// includes it.
		var app api.App
		if err := client.Get(ctx, "/apps/"+url.PathEscape(listed.Key), &app); err != nil {
			return nil, fmt.Errorf("app %s: %w", listed.Key, err)
		}
		appKey := strings.TrimSpace(app.Key)
		if appKey == "" {
			appKey = strings.TrimSpace(listed.Key)
		}
		appLabel := strings.TrimSpace(app.Name)
		if appLabel == "" {
			appLabel = strings.TrimSpace(listed.Name)
		}
		if appLabel == "" {
			appLabel = appKey
		}
		for _, job := range app.Jobs {
			row := jobsListRow{Job: job.Name, App: appLabel, AppKey: appKey, Every: job.Every}
			if job.UpdatedAt != nil {
				row.Updated = job.UpdatedAt.Local().Format("2006-01-02 15:04:05")
			}
			rows = append(rows, row)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].App != rows[j].App {
			return rows[i].App < rows[j].App
		}
		return rows[i].Job < rows[j].Job
	})
	return rows, nil
}

func resolveJobAppKey(ctx context.Context, client *api.Client, job string) (string, error) {
	rows, err := loadRegisteredJobs(ctx, client, "")
	if err != nil {
		return "", fmt.Errorf("resolve app for job %q: %w", job, err)
	}

	var appKeys []string
	for _, row := range rows {
		if row.Job == job {
			appKeys = append(appKeys, row.AppKey)
		}
	}
	switch len(appKeys) {
	case 0:
		return "", fmt.Errorf("job %q is not registered on the server (run 'nimbu jobs list' to see registered jobs; a just-pushed file may register late, push it again to refresh the registry)", job)
	case 1:
		return appKeys[0], nil
	default:
		return "", fmt.Errorf("job %q is registered by multiple apps (%s); job names must be unique per site", job, strings.Join(appKeys, ", "))
	}
}

// Run executes the list command.
func (c *JobsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	rows, err := loadRegisteredJobs(ctx, client, c.App)
	if err != nil {
		return fmt.Errorf("list jobs: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, rows)
	}
	if mode.Plain {
		return output.PlainFromSlice(ctx, rows, []string{"job", "app", "every"})
	}
	if err := output.WriteTable(ctx, rows, []string{"job", "app", "every", "updated_at"}, []string{"JOB", "APP", "EVERY", "UPDATED"}); err != nil {
		return err
	}
	if len(rows) == 0 {
		_, err := output.Fprintf(ctx, "no jobs registered; a just-pushed file may register late, push it again to refresh the registry\n")
		return err
	}
	return nil
}
