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
	Product string `required:"" help:"Product ID or slug"`
	Locale  string `help:"Content locale for localized product fields"`
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

	var document api.Document[api.Product]
	path := "/products/" + url.PathEscape(c.Product)
	var opts []api.RequestOption
	if c.Locale != "" {
		opts = append(opts, api.WithContentLocale(c.Locale))
	}
	if err := client.Get(ctx, path, &document, opts...); err != nil {
		return fmt.Errorf("get product: %w", err)
	}
	p := document.Value
	display, err := localizedDocumentMap(document, c.Locale)
	if err != nil {
		return fmt.Errorf("project product locale: %w", err)
	}

	price := fmt.Sprintf("%.2f", p.Price)
	if p.Currency != "" {
		price = fmt.Sprintf("%.2f %s", p.Price, p.Currency)
	}

	fields := []output.Field{
		output.FAlways("ID", display["id"]),
		output.FAlways("Slug", display["slug"]),
		output.FAlways("Name", display["name"]),
		output.F("SKU", p.SKU),
		output.F("Description", display["description"]),
		output.F("Status", p.Status),
		output.FAlways("Price", price),
		output.FAlways("Stock", p.CurrentStock),
		output.FAlways("Digital", p.Digital),
		output.FAlways("Shipping", p.RequiresShipping),
	}
	if p.OnSale {
		fields = append(fields, output.FAlways("Sale Price", fmt.Sprintf("%.2f", p.OnSalePrice)))
	}
	if p.CreatedAt != nil && !p.CreatedAt.IsZero() {
		fields = append(fields, output.FAlways("Created", p.CreatedAt.Format("2006-01-02 15:04:05")))
	}
	if p.UpdatedAt != nil && !p.UpdatedAt.IsZero() {
		fields = append(fields, output.FAlways("Updated", p.UpdatedAt.Format("2006-01-02 15:04:05")))
	}

	return output.Detail(ctx, document, []any{display["id"], display["slug"], display["name"], p.SKU, p.Price, p.Status}, fields)
}
