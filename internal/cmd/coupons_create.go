package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// CouponsCreateCmd creates a coupon.
type CouponsCreateCmd struct {
	File        string   `help:"Read coupon JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. name=Promo, code=SPRING)"`
}

// Run executes the create command.
func (c *CouponsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "create coupon"); err != nil {
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
	if err := client.Post(ctx, "/coupons", body, &coupon); err != nil {
		return fmt.Errorf("create coupon: %w", err)
	}

	return output.Print(ctx, coupon, []any{coupon.ID, coupon.Code, coupon.Name}, func() error {
		_, err := output.Fprintf(ctx, "Created coupon: %s (%s)\n", coupon.Code, coupon.ID)
		return err
	})
}
