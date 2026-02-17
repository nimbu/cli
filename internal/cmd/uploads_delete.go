package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// UploadsDeleteCmd deletes an upload.
type UploadsDeleteCmd struct {
	ID string `arg:"" help:"Upload ID"`
}

// Run executes the delete command.
func (c *UploadsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "delete upload"); err != nil {
		return err
	}

	if err := requireForce(flags, "upload "+c.ID); err != nil {
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

	path := "/uploads/" + url.PathEscape(c.ID)
	if err := client.Delete(ctx, path, nil); err != nil {
		return fmt.Errorf("delete upload: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, output.SuccessPayload(fmt.Sprintf("deleted %s", c.ID)))
	}

	if mode.Plain {
		return output.Plain(ctx, c.ID, "deleted")
	}

	fmt.Printf("Deleted: %s\n", c.ID)
	return nil
}
