package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// CouponsGetCmd gets a coupon by ID.
type CouponsGetCmd struct {
	Coupon string `required:"" help:"Coupon ID"`
}

// Run executes the get command.
func (c *CouponsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	var coupon api.Coupon
	path := "/coupons/" + url.PathEscape(c.Coupon)
	if err := client.Get(ctx, path, &coupon); err != nil {
		return fmt.Errorf("get coupon: %w", err)
	}

	return output.Detail(ctx, coupon, []any{coupon.ID, coupon.Code, coupon.Name, coupon.State, coupon.CouponType}, []output.Field{
		output.FAlways("ID", coupon.ID),
		output.FAlways("Code", coupon.Code),
		output.FAlways("Name", coupon.Name),
		output.F("Description", coupon.Description),
		output.FAlways("State", coupon.State),
		output.FAlways("Type", coupon.CouponType),
		output.FAlways("Percentage", fmt.Sprintf("%.2f", coupon.CouponPercentage)),
		output.FAlways("Amount", fmt.Sprintf("%.2f", coupon.CouponAmount)),
	})
}
