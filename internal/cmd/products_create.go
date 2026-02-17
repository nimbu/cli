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

// ProductsCreateCmd creates a product.
type ProductsCreateCmd struct {
	File string `help:"Read product JSON from file (use - for stdin)" type:"existingfile"`
}

// Run executes the create command.
func (c *ProductsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if flags.Readonly {
		return fmt.Errorf("cannot create product in readonly mode")
	}

	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	// Read input
	var input io.Reader
	if c.File == "-" || c.File == "" {
		input = os.Stdin
	} else {
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

	var p api.Product
	if err := client.Post(ctx, "/products", body, &p); err != nil {
		return fmt.Errorf("create product: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, p)
	}

	if mode.Plain {
		return output.Plain(ctx, p.ID, p.Slug, p.Name)
	}

	fmt.Printf("Created product: %s (%s)\n", p.Name, p.ID)
	return nil
}
