package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// JobsRunCmd schedules a cloud job.
type JobsRunCmd struct {
	Job         string   `arg:"" help:"Job identifier"`
	File        string   `help:"Read job params JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. foo=bar, retry:=true)"`
}

// Run executes the run command.
func (c *JobsRunCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "execute job"); err != nil {
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
	if err := requireScopes(ctx, client, []string{"write_cloudcode"}, "Example: nimbu-cli auth scopes"); err != nil {
		return err
	}

	body, err := readJSONBodyInput(c.File, c.Assignments)
	if err != nil {
		if !errors.Is(err, errNoJSONInput) {
			return err
		}
		body = map[string]any{}
	}

	requestBody := wrapParamsBody(body)

	path := "/jobs/" + url.PathEscape(c.Job)
	var result api.JobRunResult
	if err := client.Post(ctx, path, requestBody, &result); err != nil {
		return fmt.Errorf("schedule job: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}

	if mode.Plain {
		return output.Plain(ctx, result.JID)
	}

	return output.JSON(ctx, result)
}
