package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

type CustomersResetPasswordCmd struct {
	Customer string `required:"" help:"Customer ID or email"`
}

func (c *CustomersResetPasswordCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runCustomerAction(ctx, flags, c.Customer, "reset_password", "reset customer password")
}

type CustomersResendConfirmationCmd struct {
	Customer string `required:"" help:"Customer ID or email"`
}

func (c *CustomersResendConfirmationCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runCustomerAction(ctx, flags, c.Customer, "resend_confirmation", "resend customer confirmation")
}

func runCustomerAction(ctx context.Context, flags *RootFlags, customerRef, action, label string) error {
	if err := requireWrite(flags, label); err != nil {
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
	var result api.ActionStatus
	path := "/customers/" + url.PathEscape(customerRef) + "/" + action
	if err := client.Post(ctx, path, map[string]any{}, &result); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return output.Print(ctx, result, []any{customerRef, result.Status}, func() error {
		_, err := output.Fprintf(ctx, "%s: %s\n", label, customerRef)
		return err
	})
}
