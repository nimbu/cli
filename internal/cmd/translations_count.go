package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// TranslationsCountCmd gets translation count.
type TranslationsCountCmd struct{}

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

	count, err := api.Count(ctx, client, "/translations/count")
	if err != nil {
		return fmt.Errorf("count translations: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, output.CountPayload(count))
	}

	if mode.Plain {
		return output.Plain(ctx, count)
	}

	fmt.Printf("Translations: %d\n", count)
	return nil
}
