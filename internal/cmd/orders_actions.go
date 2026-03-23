package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

type OrdersPayCmd struct {
	Order string `arg:"" help:"Order ID or number"`
}

func (c *OrdersPayCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runOrderAction(ctx, flags, c.Order, "payment", map[string]any{}, "pay order")
}

type OrdersFinishCmd struct {
	Order  string `arg:"" help:"Order ID or number"`
	Notify bool   `help:"Send notification" default:"true"`
}

func (c *OrdersFinishCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runOrderAction(ctx, flags, c.Order, "finish", map[string]any{"notify": c.Notify}, "finish order")
}

type OrdersCancelCmd struct {
	Order        string `arg:"" help:"Order ID or number"`
	Notify       bool   `help:"Send notification" default:"true"`
	CancelReason string `name:"reason" help:"Cancel reason"`
}

func (c *OrdersCancelCmd) Run(ctx context.Context, flags *RootFlags) error {
	body := map[string]any{"notify": c.Notify}
	if c.CancelReason != "" {
		body["cancel_reason"] = c.CancelReason
	}
	return runOrderAction(ctx, flags, c.Order, "cancel", body, "cancel order")
}

type OrdersReopenCmd struct {
	Order string `arg:"" help:"Order ID or number"`
}

func (c *OrdersReopenCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runOrderAction(ctx, flags, c.Order, "reopen", map[string]any{}, "reopen order")
}

type OrdersArchiveCmd struct {
	Order string `arg:"" help:"Order ID or number"`
}

func (c *OrdersArchiveCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runOrderAction(ctx, flags, c.Order, "archive", map[string]any{}, "archive order")
}

func runOrderAction(ctx context.Context, flags *RootFlags, orderRef, action string, body map[string]any, label string) error {
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
	path := "/orders/" + url.PathEscape(orderRef) + "/" + action
	if err := client.Post(ctx, path, body, &result); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}
	if mode.Plain {
		return output.Plain(ctx, orderRef, result.Status, result.State)
	}
	fmt.Printf("%s: %s\n", label, orderRef)
	return nil
}
