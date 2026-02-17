package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// TranslationsDeleteCmd deletes a translation.
type TranslationsDeleteCmd struct {
	Key string `arg:"" help:"Translation key"`
}

// Run executes the delete command.
func (c *TranslationsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "delete translation"); err != nil {
		return err
	}
	if err := requireForce(flags, "translation "+c.Key); err != nil {
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

	if err := client.Delete(ctx, "/translations/"+url.PathEscape(c.Key), nil); err != nil {
		return fmt.Errorf("delete translation: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, output.SuccessPayload(fmt.Sprintf("deleted %s", c.Key)))
	}

	if mode.Plain {
		return output.Plain(ctx, c.Key, "deleted")
	}

	fmt.Printf("Deleted translation: %s\n", c.Key)
	return nil
}
