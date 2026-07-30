package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

type AppsCodeGetCmd struct {
	App      string `required:"" help:"Application ID"`
	Filename string `required:"" help:"Code filename"`
}

func (c *AppsCodeGetCmd) Run(ctx context.Context) error {
	client, err := auditedClient(ctx)
	if err != nil {
		return err
	}
	var file api.AppCodeFile
	path := "/apps/" + url.PathEscape(c.App) + "/code/" + url.PathEscape(c.Filename)
	if err := client.Get(ctx, path, &file); err != nil {
		return fmt.Errorf("get app code file: %w", err)
	}
	return output.Detail(ctx, file, []any{file.Name, file.URL}, []output.Field{
		output.FAlways("Name", file.Name), output.F("URL", file.URL), output.F("Code", file.Code),
	})
}

type AppsCodeUpdateCmd struct {
	App         string   `required:"" help:"Application ID"`
	Filename    string   `required:"" help:"Code filename"`
	File        string   `help:"Read JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments such as code=..."`
}

func (c *AppsCodeUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	path := "/apps/" + url.PathEscape(c.App) + "/code/" + url.PathEscape(c.Filename)
	return auditedWrite(ctx, flags, "PUT", path, c.File, c.Assignments)
}

type AppsCodeDeleteCmd struct {
	App      string `required:"" help:"Application ID"`
	Filename string `required:"" help:"Code filename"`
}

func (c *AppsCodeDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	return auditedDelete(ctx, flags, "/apps/"+url.PathEscape(c.App)+"/code/"+url.PathEscape(c.Filename))
}
