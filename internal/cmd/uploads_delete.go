package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/output"
)

// UploadsDeleteCmd deletes an upload.
type UploadsDeleteCmd struct {
	ID string `arg:"" help:"Upload ID"`
}

// Run executes the delete command.
func (c *UploadsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if flags.Readonly {
		return fmt.Errorf("write operations disabled in readonly mode")
	}

	if !flags.Force {
		return fmt.Errorf("delete requires --force flag")
	}

	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	if err := client.Delete(ctx, "/uploads/"+c.ID, nil); err != nil {
		return fmt.Errorf("delete upload: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, output.SuccessPayload(fmt.Sprintf("deleted %s", c.ID)))
	}

	if mode.Plain {
		return output.Plain(ctx, "deleted", c.ID)
	}

	fmt.Printf("Deleted: %s\n", c.ID)
	return nil
}
