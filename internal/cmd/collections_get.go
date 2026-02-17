package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// CollectionsGetCmd gets a collection by ID or slug.
type CollectionsGetCmd struct {
	Collection string `arg:"" help:"Collection ID or slug"`
}

// Run executes the get command.
func (c *CollectionsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	var col api.Collection
	path := "/collections/" + url.PathEscape(c.Collection)
	if err := client.Get(ctx, path, &col); err != nil {
		return fmt.Errorf("get collection: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, col)
	}

	if mode.Plain {
		return output.Plain(ctx, col.ID, col.Slug, col.Name, col.Status, col.Type)
	}

	fmt.Printf("ID:          %s\n", col.ID)
	fmt.Printf("Slug:        %s\n", col.Slug)
	fmt.Printf("Name:        %s\n", col.Name)
	if col.Description != "" {
		fmt.Printf("Description: %s\n", col.Description)
	}
	fmt.Printf("Status:      %s\n", col.Status)
	fmt.Printf("Type:        %s\n", col.Type)
	fmt.Printf("Products:    %d\n", col.ProductCount)
	if !col.CreatedAt.IsZero() {
		fmt.Printf("Created:     %s\n", col.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	if !col.UpdatedAt.IsZero() {
		fmt.Printf("Updated:     %s\n", col.UpdatedAt.Format("2006-01-02 15:04:05"))
	}

	return nil
}
