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
	Coupon string `arg:"" help:"Coupon ID"`
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

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, coupon)
	}

	if mode.Plain {
		return output.Plain(ctx, coupon.ID, coupon.Code, coupon.Name, coupon.State, coupon.CouponType)
	}

	fmt.Printf("ID:          %s\n", coupon.ID)
	fmt.Printf("Code:        %s\n", coupon.Code)
	fmt.Printf("Name:        %s\n", coupon.Name)
	if coupon.Description != "" {
		fmt.Printf("Description: %s\n", coupon.Description)
	}
	fmt.Printf("State:       %s\n", coupon.State)
	fmt.Printf("Type:        %s\n", coupon.CouponType)
	fmt.Printf("Percentage:  %.2f\n", coupon.CouponPercentage)
	fmt.Printf("Amount:      %.2f\n", coupon.CouponAmount)

	return nil
}
