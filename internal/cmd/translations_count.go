package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// TranslationsCountCmd gets translation count.
type TranslationsCountCmd struct {
	CountQueryFlags `embed:""`
}

// Run executes the count command.
func (c *TranslationsCountCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	opts, err := countRequestOptions(&c.CountQueryFlags, false)
	if err != nil {
		return fmt.Errorf("count translations: %w", err)
	}
	count, err := api.Count(ctx, client, "/translations/count", opts...)
	if err != nil {
		return fmt.Errorf("count translations: %w", err)
	}

	return output.Print(ctx, output.CountPayload(count), []any{count}, func() error {
		_, err := output.Fprintf(ctx, "Translations: %d\n", count)
		return err
	})
}
