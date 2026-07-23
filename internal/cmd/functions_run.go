package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/nimbu/cli/internal/output"
)

// FunctionsRunCmd executes a cloud function.
type FunctionsRunCmd struct {
	Function    string   `help:"Function identifier"`
	File        string   `help:"Read function params JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. foo=bar, amount:=2)"`
}

// resolveFunction validates --function; the flag stays optional in the CLI
// grammar so a missing or positional function name gets a teaching error
// instead of kong's bare "missing flags: --function=STRING".
func (c *FunctionsRunCmd) resolveFunction() (string, error) {
	function := strings.TrimSpace(c.Function)
	if function == "" {
		if len(c.Assignments) > 0 && !strings.Contains(c.Assignments[0], "=") {
			return "", fmt.Errorf("pass the function name as a flag: nimbu functions run --function %s [assignments]", c.Assignments[0])
		}
		return "", fmt.Errorf("function name required: nimbu functions run --function <function> [assignments]")
	}
	return function, nil
}

// Run executes the run command.
func (c *FunctionsRunCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "execute function"); err != nil {
		return err
	}

	function, err := c.resolveFunction()
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

	body, err := readJSONBodyInput(c.File, c.Assignments)
	if err != nil {
		if !errors.Is(err, errNoJSONInput) {
			return err
		}
		body = map[string]any{}
	}

	// The server hands the request body to the function as its params; wrapping
	// it in a {"params": ...} envelope would leak the envelope into the function.
	var result map[string]any
	path := "/functions/" + url.PathEscape(function)
	if err := client.Post(ctx, path, body, &result); err != nil {
		return hintJSONAssignments(fmt.Errorf("execute function: %w", err), c.Assignments)
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
