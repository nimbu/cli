package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

type SendersListCmd struct {
	QueryFlags `embed:""`
	All        bool `help:"Fetch all pages"`
	Page       int  `help:"Page number" default:"1"`
	PerPage    int  `help:"Items per page" default:"25"`
}

func (c *SendersListCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}
	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}
	opts, err := listRequestOptions(&c.QueryFlags)
	if err != nil {
		return fmt.Errorf("list senders: %w", err)
	}

	var senders []api.SenderDomain
	var meta listFooterMeta
	if c.All {
		senders, err = api.List[api.SenderDomain](ctx, client, "/senders", opts...)
		if err != nil {
			return fmt.Errorf("list senders: %w", err)
		}
		meta = allListFooterMeta(len(senders))
	} else {
		paged, err := api.ListPage[api.SenderDomain](ctx, client, "/senders", c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list senders: %w", err)
		}
		senders = paged.Data
		meta = newListFooterMeta(c.Page, c.PerPage, paged.Pagination, paged.Links, len(senders))
	}
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, senders)
	}
	plainFields := []string{"id", "domain", "provider", "status", "ownership_verified"}
	tableFields := []string{"id", "domain", "provider", "status", "ownership_verified", "verified_at"}
	tableHeaders := []string{"ID", "DOMAIN", "PROVIDER", "STATUS", "OWNERSHIP_VERIFIED", "VERIFIED_AT"}
	if mode.Plain {
		return output.PlainFromSlice(ctx, senders, listOutputFields(&c.QueryFlags, plainFields))
	}
	fields, headers := listOutputColumns(&c.QueryFlags, tableFields, tableHeaders)
	if err := output.WriteTable(ctx, senders, fields, headers); err != nil {
		return err
	}
	return writeListFooter(ctx, "senders", meta)
}

type SendersGetCmd struct {
	Sender string `arg:"" help:"Sender ID or domain"`
}

func (c *SendersGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}
	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}
	sender, err := resolveSenderRef(ctx, client, c.Sender)
	if err != nil {
		return fmt.Errorf("get sender: %w", err)
	}
	var verifiedAt string
	if sender.VerifiedAt != nil {
		verifiedAt = sender.VerifiedAt.Format("2006-01-02 15:04:05Z07:00")
	}
	return output.Detail(ctx, sender,
		[]any{sender.ID, sender.Domain, sender.Status},
		[]output.Field{
			output.FAlways("ID", sender.ID),
			output.FAlways("Domain", sender.Domain),
			output.FAlways("Provider", sender.Provider),
			output.FAlways("Status", sender.Status),
			output.FAlways("Ownership Verified", sender.OwnershipVerified),
			output.F("Verified At", verifiedAt),
			output.F("Last Error", sender.LastCheckError),
		},
	)
}

type SendersCreateCmd struct {
	Domain string `arg:"" help:"Sender domain"`
}

func (c *SendersCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "create sender domain"); err != nil {
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
	var sender api.SenderDomain
	if err := client.Post(ctx, "/senders", map[string]any{"domain": c.Domain}, &sender); err != nil {
		return fmt.Errorf("create sender: %w", err)
	}
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, sender)
	}
	if mode.Plain {
		return output.Plain(ctx, sender.ID, sender.Domain, sender.Status)
	}
	if _, err := output.Fprintf(ctx, "Created sender domain %s (%s)\n", sender.Domain, sender.ID); err != nil {
		return err
	}
	return nil
}

type SendersVerifyOwnershipCmd struct {
	Sender string `arg:"" help:"Sender ID or domain"`
}

func (c *SendersVerifyOwnershipCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "verify sender ownership"); err != nil {
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
	sender, err := resolveSenderRef(ctx, client, c.Sender)
	if err != nil {
		return fmt.Errorf("resolve sender: %w", err)
	}
	var verified api.SenderDomain
	path := "/senders/" + url.PathEscape(sender.ID) + "/verify_ownership"
	if err := client.Post(ctx, path, map[string]any{}, &verified); err != nil {
		return fmt.Errorf("verify sender ownership: %w", err)
	}
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, verified)
	}
	if mode.Plain {
		return output.Plain(ctx, verified.ID, verified.Domain, verified.OwnershipVerified)
	}
	if _, err := output.Fprintf(ctx, "Ownership verification requested for %s\n", verified.Domain); err != nil {
		return err
	}
	return nil
}

type SendersVerifyCmd struct {
	Sender string `arg:"" help:"Sender ID or domain"`
}

func (c *SendersVerifyCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "verify sender dns"); err != nil {
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
	sender, err := resolveSenderRef(ctx, client, c.Sender)
	if err != nil {
		return fmt.Errorf("resolve sender: %w", err)
	}
	var verified api.SenderDomain
	path := "/senders/" + url.PathEscape(sender.ID) + "/verify"
	if err := client.Post(ctx, path, map[string]any{}, &verified); err != nil {
		return fmt.Errorf("verify sender dns: %w", err)
	}
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, verified)
	}
	if mode.Plain {
		return output.Plain(ctx, verified.ID, verified.Domain, verified.Status)
	}
	if _, err := output.Fprintf(ctx, "DNS verification requested for %s\n", verified.Domain); err != nil {
		return err
	}
	return nil
}
