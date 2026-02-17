package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// MenusCreateCmd creates a menu.
type MenusCreateCmd struct {
	File string `help:"JSON file path (default: stdin)" type:"existingfile"`
}

// Run executes the create command.
func (c *MenusCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if flags.Readonly {
		return fmt.Errorf("write operations disabled in readonly mode")
	}

	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	var input io.Reader = os.Stdin
	if c.File != "" {
		f, err := os.Open(c.File)
		if err != nil {
			return fmt.Errorf("open file: %w", err)
		}
		defer func() { _ = f.Close() }()
		input = f
	}

	data, err := io.ReadAll(input)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		return fmt.Errorf("parse JSON: %w", err)
	}

	var menu api.Menu
	if err := client.Post(ctx, "/menus", body, &menu); err != nil {
		return fmt.Errorf("create menu: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, menu)
	}

	if mode.Plain {
		return output.Plain(ctx, menu.ID)
	}

	fmt.Printf("Created menu %s\n", menu.ID)
	return nil
}
