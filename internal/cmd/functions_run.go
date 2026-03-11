package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// FunctionsRunCmd executes a cloud function.
type FunctionsRunCmd struct {
	Function    string   `arg:"" help:"Function identifier"`
	File        string   `help:"Read function params JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. foo=bar, amount:=2)"`
}

// Run executes the run command.
func (c *FunctionsRunCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "execute function"); err != nil {
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

	body, err := readJSONBodyInput(c.File, c.Assignments)
	if err != nil {
		if !errors.Is(err, errNoJSONInput) {
			return err
		}
		body = map[string]any{}
	}

	requestBody := wrapParamsBody(body)

	var result map[string]any
	path := "/functions/" + url.PathEscape(c.Function)
	if err := client.Post(ctx, path, requestBody, &result); err != nil {
		return fmt.Errorf("execute function: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}

	if mode.Plain {
		b, err := json.Marshal(result)
		if err != nil {
			return err
		}
		return output.Plain(ctx, string(b))
	}

	return output.JSON(ctx, result)
}
