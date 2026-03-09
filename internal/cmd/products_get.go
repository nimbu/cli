package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ProductsGetCmd gets a product by ID or slug.
type ProductsGetCmd struct {
	Product string `arg:"" help:"Product ID or slug"`
}

// Run executes the get command.
func (c *ProductsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	var p api.Product
	path := "/products/" + url.PathEscape(c.Product)
	if err := client.Get(ctx, path, &p); err != nil {
		return fmt.Errorf("get product: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, p)
	}

	if mode.Plain {
		return output.Plain(ctx, p.ID, p.Slug, p.Name, p.SKU, p.Price, p.Status)
	}

	fmt.Printf("ID:          %s\n", p.ID)
	fmt.Printf("Slug:        %s\n", p.Slug)
	fmt.Printf("Name:        %s\n", p.Name)
	if p.SKU != "" {
		fmt.Printf("SKU:         %s\n", p.SKU)
	}
	if p.Description != "" {
		fmt.Printf("Description: %s\n", p.Description)
	}
	if p.Status != "" {
		fmt.Printf("Status:      %s\n", p.Status)
	}
	if p.Currency != "" {
		fmt.Printf("Price:       %.2f %s\n", p.Price, p.Currency)
	} else {
		fmt.Printf("Price:       %.2f\n", p.Price)
	}
	fmt.Printf("Stock:       %d\n", p.CurrentStock)
	fmt.Printf("Digital:     %v\n", p.Digital)
	fmt.Printf("Shipping:    %v\n", p.RequiresShipping)
	if p.OnSale {
		fmt.Printf("Sale Price:  %.2f\n", p.OnSalePrice)
	}
	if !p.CreatedAt.IsZero() {
		fmt.Printf("Created:     %s\n", p.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	if !p.UpdatedAt.IsZero() {
		fmt.Printf("Updated:     %s\n", p.UpdatedAt.Format("2006-01-02 15:04:05"))
	}

	return nil
}
