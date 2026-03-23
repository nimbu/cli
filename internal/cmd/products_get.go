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

	price := fmt.Sprintf("%.2f", p.Price)
	if p.Currency != "" {
		price = fmt.Sprintf("%.2f %s", p.Price, p.Currency)
	}

	fields := []output.Field{
		output.FAlways("ID", p.ID),
		output.FAlways("Slug", p.Slug),
		output.FAlways("Name", p.Name),
		output.F("SKU", p.SKU),
		output.F("Description", p.Description),
		output.F("Status", p.Status),
		output.FAlways("Price", price),
		output.FAlways("Stock", p.CurrentStock),
		output.FAlways("Digital", p.Digital),
		output.FAlways("Shipping", p.RequiresShipping),
	}
	if p.OnSale {
		fields = append(fields, output.FAlways("Sale Price", fmt.Sprintf("%.2f", p.OnSalePrice)))
	}
	if !p.CreatedAt.IsZero() {
		fields = append(fields, output.FAlways("Created", p.CreatedAt.Format("2006-01-02 15:04:05")))
	}
	if !p.UpdatedAt.IsZero() {
		fields = append(fields, output.FAlways("Updated", p.UpdatedAt.Format("2006-01-02 15:04:05")))
	}

	return output.Detail(ctx, p, []any{p.ID, p.Slug, p.Name, p.SKU, p.Price, p.Status}, fields)
}
