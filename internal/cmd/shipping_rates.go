package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ShippingRatesCmd manages shipping rates.
type ShippingRatesCmd struct {
	List   ShippingRatesListCmd   `cmd:"" help:"List shipping rates"`
	Get    ShippingRatesGetCmd    `cmd:"" help:"Get shipping rate details"`
	Create ShippingRatesCreateCmd `cmd:"" help:"Create a shipping rate"`
	Update ShippingRatesUpdateCmd `cmd:"" help:"Update a shipping rate"`
	Delete ShippingRatesDeleteCmd `cmd:"" help:"Delete a shipping rate"`
}

// ShippingRatesListCmd lists shipping rates without pagination.
type ShippingRatesListCmd struct {
	QueryFlags `embed:""`
}

// Run executes the list command.
func (c *ShippingRatesListCmd) Run(ctx context.Context, _ *RootFlags) error {
	client, err := shippingRatesClient(ctx)
	if err != nil {
		return err
	}
	opts, err := localizedContentListRequestOptions(&c.QueryFlags)
	if err != nil {
		return fmt.Errorf("list shipping rates: %w", err)
	}

	var documents []api.Document[api.ShippingRate]
	if err := client.Get(ctx, "/shipping_rates", &documents, opts...); err != nil {
		return fmt.Errorf("list shipping rates: %w", err)
	}
	if output.FromContext(ctx).JSON {
		return output.JSON(ctx, documents)
	}

	rates, err := localizedDocumentMaps(documents, c.Locale)
	if err != nil {
		return fmt.Errorf("project shipping rate locale: %w", err)
	}
	for i := range documents {
		applyShippingRateDisplayDefaults(rates[i], documents[i].Value)
	}
	plainFields := shippingRateOutputFields()
	if output.FromContext(ctx).Plain {
		return output.PlainFromSlice(ctx, rates, listOutputFields(&c.QueryFlags, plainFields))
	}
	fields, headers := listOutputColumns(
		&c.QueryFlags,
		plainFields,
		[]string{
			"ID", "NAME", "CRITERIA", "PRICE", "DEFAULT", "PICKUP", "REGION ID",
			"WEIGHT MIN", "WEIGHT MAX", "ORDER MIN", "ORDER MAX", "RESTRICTED", "ZIPCODES",
		},
	)
	return output.WriteTable(ctx, rates, fields, headers)
}

// ShippingRatesGetCmd gets a shipping rate by ID.
type ShippingRatesGetCmd struct {
	Rate   string `required:"" help:"Shipping rate ID"`
	Locale string `help:"Content locale for localized shipping rate fields"`
}

// Run executes the get command.
func (c *ShippingRatesGetCmd) Run(ctx context.Context, _ *RootFlags) error {
	client, err := shippingRatesClient(ctx)
	if err != nil {
		return err
	}

	var document api.Document[api.ShippingRate]
	if err := client.Get(
		ctx,
		"/shipping_rates/"+url.PathEscape(c.Rate),
		&document,
		shippingRateLocaleOptions(c.Locale)...,
	); err != nil {
		return fmt.Errorf("get shipping rate: %w", err)
	}
	return printShippingRateDetail(ctx, document, c.Locale)
}

// ShippingRatesCreateCmd creates a shipping rate.
type ShippingRatesCreateCmd struct {
	Locale      string   `help:"Content locale for localized shipping rate fields"`
	File        string   `help:"Read shipping rate JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. name=Express, price:=12.5)"`
}

// Run executes the create command.
func (c *ShippingRatesCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "create shipping rate"); err != nil {
		return err
	}
	body, err := shippingRateWriteBody(c.File, c.Assignments)
	if err != nil {
		return err
	}
	client, err := shippingRatesClient(ctx)
	if err != nil {
		return err
	}

	var document api.Document[api.ShippingRate]
	if err := client.Post(ctx, "/shipping_rates", body, &document, shippingRateLocaleOptions(c.Locale)...); err != nil {
		return fmt.Errorf("create shipping rate: %w", err)
	}
	return printShippingRateMutation(ctx, "Created", document, c.Locale)
}

// ShippingRatesUpdateCmd updates a shipping rate.
type ShippingRatesUpdateCmd struct {
	Rate        string   `required:"" help:"Shipping rate ID"`
	Locale      string   `help:"Content locale for localized shipping rate fields"`
	File        string   `help:"Read shipping rate JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. name=Express, price:=12.5)"`
}

// Run executes the update command.
func (c *ShippingRatesUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "update shipping rate"); err != nil {
		return err
	}
	body, err := shippingRateWriteBody(c.File, c.Assignments)
	if err != nil {
		return err
	}
	client, err := shippingRatesClient(ctx)
	if err != nil {
		return err
	}

	var document api.Document[api.ShippingRate]
	path := "/shipping_rates/" + url.PathEscape(c.Rate)
	// The public Rails shipping-rates member route explicitly accepts PATCH.
	if err := client.Patch(ctx, path, body, &document, shippingRateLocaleOptions(c.Locale)...); err != nil {
		return fmt.Errorf("update shipping rate: %w", err)
	}
	return printShippingRateMutation(ctx, "Updated", document, c.Locale)
}

// ShippingRatesDeleteCmd deletes a shipping rate.
type ShippingRatesDeleteCmd struct {
	Rate string `required:"" help:"Shipping rate ID"`
}

// Run executes the delete command.
func (c *ShippingRatesDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "delete shipping rate"); err != nil {
		return err
	}
	if err := requireForce(flags, "shipping rate "+c.Rate); err != nil {
		return err
	}
	client, err := shippingRatesClient(ctx)
	if err != nil {
		return err
	}
	if err := client.Delete(ctx, "/shipping_rates/"+url.PathEscape(c.Rate), nil); err != nil {
		return fmt.Errorf("delete shipping rate: %w", err)
	}
	return output.Print(ctx, output.SuccessPayload("shipping rate deleted"), []any{c.Rate, "deleted"}, func() error {
		_, err := output.Fprintf(ctx, "Deleted shipping rate: %s\n", c.Rate)
		return err
	})
}

func shippingRatesClient(ctx context.Context) (*api.Client, error) {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return nil, err
	}
	return GetAPIClientWithSite(ctx, site)
}

func shippingRateLocaleOptions(locale string) []api.RequestOption {
	if locale == "" {
		return nil
	}
	return []api.RequestOption{api.WithContentLocale(locale)}
}

func shippingRateWriteBody(file string, assignments []string) (map[string]any, error) {
	if file != "" && len(assignments) > 0 {
		return nil, fmt.Errorf("use either --file or inline assignments, not both")
	}

	var (
		body map[string]any
		err  error
	)
	if len(assignments) > 0 {
		body, err = parseInlineAssignments(assignments)
	} else {
		body, err = readJSONInputUseNumber(file)
	}
	if err != nil {
		return nil, err
	}
	if containsTranslationsPayload(body) {
		return nil, fmt.Errorf(
			"shipping rate payloads cannot include translations; issue a separate write for each translation using --locale",
		)
	}
	return body, nil
}

func containsTranslationsPayload(value any) bool {
	switch value := value.(type) {
	case map[string]any:
		for key, nested := range value {
			if key == "translations" || containsTranslationsPayload(nested) {
				return true
			}
		}
	case []any:
		for _, nested := range value {
			if containsTranslationsPayload(nested) {
				return true
			}
		}
	}
	return false
}

func printShippingRateDetail(ctx context.Context, document api.Document[api.ShippingRate], locale string) error {
	display, err := localizedDocumentMap(document, locale)
	if err != nil {
		return fmt.Errorf("project shipping rate locale: %w", err)
	}
	applyShippingRateDisplayDefaults(display, document.Value)
	return output.Detail(ctx, document, shippingRatePlainValues(display), []output.Field{
		output.FAlways("ID", display["id"]),
		output.FAlways("Name", display["name"]),
		output.FAlways("Criteria", display["criteria"]),
		output.FAlways("Price", display["price"]),
		output.FAlways("Default", display["default"]),
		output.FAlways("Pickup", display["pickup"]),
		output.FAlways("Region ID", display["region_id"]),
		output.FAlways("Weight Min", display["weight_min"]),
		output.FAlways("Weight Max", display["weight_max"]),
		output.FAlways("Order Min", display["order_min"]),
		output.FAlways("Order Max", display["order_max"]),
		output.FAlways("Restricted", display["restricted"]),
		output.FAlways("Zipcodes", display["zipcodes"]),
	})
}

func printShippingRateMutation(
	ctx context.Context,
	action string,
	document api.Document[api.ShippingRate],
	locale string,
) error {
	display, err := localizedDocumentMap(document, locale)
	if err != nil {
		return fmt.Errorf("project shipping rate locale: %w", err)
	}
	applyShippingRateDisplayDefaults(display, document.Value)
	return output.Print(ctx, document, shippingRatePlainValues(display), func() error {
		_, err := output.Fprintf(ctx, "%s shipping rate: %v (%v)\n", action, display["name"], display["id"])
		return err
	})
}

func applyShippingRateDisplayDefaults(display map[string]any, rate api.ShippingRate) {
	defaults := map[string]any{
		"id":         rate.ID,
		"name":       rate.Name,
		"criteria":   rate.Criteria,
		"price":      rate.Price,
		"default":    rate.Default,
		"pickup":     rate.Pickup,
		"region_id":  rate.RegionID,
		"weight_min": rate.WeightMin,
		"weight_max": rate.WeightMax,
		"order_min":  rate.OrderMin,
		"order_max":  rate.OrderMax,
		"restricted": rate.Restricted,
		"zipcodes":   rate.Zipcodes,
	}
	for field, value := range defaults {
		if _, exists := display[field]; !exists {
			display[field] = value
		}
	}
}

func shippingRatePlainValues(rate map[string]any) []any {
	values := make([]any, 0, len(shippingRateOutputFields()))
	for _, field := range shippingRateOutputFields() {
		values = append(values, rate[field])
	}
	return values
}

func shippingRateOutputFields() []string {
	return []string{
		"id", "name", "criteria", "price", "default", "pickup", "region_id",
		"weight_min", "weight_max", "order_min", "order_max", "restricted", "zipcodes",
	}
}
