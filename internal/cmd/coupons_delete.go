package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// CouponsDeleteCmd deletes a coupon.
type CouponsDeleteCmd struct {
	Coupon string `arg:"" help:"Coupon ID"`
}

// Run executes the delete command.
func (c *CouponsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "delete coupon"); err != nil {
		return err
	}

	if err := requireForce(flags, "coupon "+c.Coupon); err != nil {
		return err
	}

	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	path := "/coupons/" + url.PathEscape(c.Coupon)
	if err := client.Delete(ctx, path, nil); err != nil {
		return fmt.Errorf("delete coupon: %w", err)
	}

	return output.Print(ctx, output.SuccessPayload("coupon deleted"), []any{c.Coupon, "deleted"}, func() error {
		_, err := output.Fprintf(ctx, "Deleted coupon: %s\n", c.Coupon)
		return err
	})
}
