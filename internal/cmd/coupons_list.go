package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// CouponsListCmd lists coupons.
type CouponsListCmd struct {
	All     bool `help:"Fetch all pages"`
	Page    int  `help:"Page number" default:"1"`
	PerPage int  `help:"Items per page" default:"25"`
}

// Run executes the list command.
func (c *CouponsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	opts, err := listRequestOptions(flags)
	if err != nil {
		return fmt.Errorf("list coupons: %w", err)
	}

	var coupons []api.Coupon
	if c.All {
		coupons, err = api.List[api.Coupon](ctx, client, "/coupons", opts...)
		if err != nil {
			return fmt.Errorf("list coupons: %w", err)
		}
	} else {
		paged, err := api.ListPage[api.Coupon](ctx, client, "/coupons", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list coupons: %w", err)
		}
		coupons = paged.Data
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, coupons)
	}

	plainFields := []string{"id", "code", "name", "state", "coupon_type"}
	tableFields := []string{"id", "code", "name", "state", "coupon_type", "coupon_percentage", "coupon_amount"}
	tableHeaders := []string{"ID", "CODE", "NAME", "STATE", "TYPE", "%", "AMOUNT"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, coupons, listOutputFields(flags, plainFields))
	}

	fields, headers := listOutputColumns(flags, tableFields, tableHeaders)
	return output.WriteTable(ctx, coupons, fields, headers)
}
