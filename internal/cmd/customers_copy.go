package cmd

import (
	"context"

	"github.com/nimbu/cli/internal/migrate"
	"github.com/nimbu/cli/internal/output"
)

// CustomersCopyCmd copies customers between sites.
type CustomersCopyCmd struct {
	From           string `help:"Source site" required:"" name:"from"`
	To             string `help:"Target site" required:"" name:"to"`
	FromHost       string `help:"Source API base URL or host" name:"from-host"`
	ToHost         string `help:"Target API base URL or host" name:"to-host"`
	Query          string `help:"Raw query string to append to the source customer list"`
	Where          string `help:"Where expression for source customer selection"`
	PerPage        int    `help:"Items per page" name:"per-page"`
	Upsert         string `help:"Comma-separated upsert fields" default:"email"`
	PasswordLength int    `help:"Generated password length for newly created customers" name:"password-length" default:"12"`
	AllowErrors    bool   `name:"allow-errors" help:"Continue on item-level validation errors"`
}

// Run executes customers copy.
func (c *CustomersCopyCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "copy customers"); err != nil {
		return err
	}
	fromRef, err := parseSiteRefForCommand(ctx, c.From, c.FromHost)
	if err != nil {
		return err
	}
	toRef, err := parseSiteRefForCommand(ctx, c.To, c.ToHost)
	if err != nil {
		return err
	}
	fromClient, err := GetAPIClientWithBaseURL(ctx, fromRef.BaseURL, fromRef.Site)
	if err != nil {
		return err
	}
	toClient, err := GetAPIClientWithBaseURL(ctx, toRef.BaseURL, toRef.Site)
	if err != nil {
		return err
	}
	result, err := migrate.CopyCustomers(ctx, fromClient, toClient, fromRef, toRef, migrate.RecordCopyOptions{
		AllowErrors:    c.AllowErrors,
		PasswordLength: c.PasswordLength,
		PerPage:        c.PerPage,
		Query:          c.Query,
		Upsert:         c.Upsert,
		Where:          c.Where,
	})
	if err != nil {
		return err
	}
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}
	for _, item := range result.Items {
		if mode.Plain {
			if err := printLine(ctx, "%s\t%s\t%s\t%s\n", item.Action, item.Resource, item.Identifier, item.TargetID); err != nil {
				return err
			}
			continue
		}
		if err := printLine(ctx, "%s %s\n", item.Action, item.Identifier); err != nil {
			return err
		}
	}
	for _, warning := range result.Warnings {
		if err := printLine(ctx, "warning: %s\n", warning); err != nil {
			return err
		}
	}
	return nil
}
