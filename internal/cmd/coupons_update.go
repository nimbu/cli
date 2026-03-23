package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// CouponsUpdateCmd updates a coupon.
type CouponsUpdateCmd struct {
	Coupon      string   `arg:"" help:"Coupon ID"`
	File        string   `help:"Read coupon JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. name=Promo, code=SPRING)"`
}

// Run executes the update command.
func (c *CouponsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "update coupon"); err != nil {
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

	body, err := readJSONBodyInput(c.File, c.Assignments)
	if err != nil {
		return err
	}

	var coupon api.Coupon
	path := "/coupons/" + url.PathEscape(c.Coupon)
	if err := client.Put(ctx, path, body, &coupon); err != nil {
		return fmt.Errorf("update coupon: %w", err)
	}

	return output.Print(ctx, coupon, []any{coupon.ID, coupon.Code, coupon.Name}, func() error {
		_, err := output.Fprintf(ctx, "Updated coupon: %s (%s)\n", coupon.Code, coupon.ID)
		return err
	})
}
