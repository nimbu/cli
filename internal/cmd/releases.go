package cmd

import (
	"context"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ReleasesCmd lists successful releases for the selected site.
type ReleasesCmd struct{}

func (c *ReleasesCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}
	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}
	records, err := api.List[releaseRecord](ctx, client, "/releases")
	if err != nil {
		return err
	}
	if output.FromContext(ctx).JSON {
		return output.JSON(ctx, records)
	}
	fields := []string{"created_at", "environment", "commit", "ref", "actor"}
	if output.FromContext(ctx).Plain {
		return output.PlainFromSlice(ctx, records, fields)
	}
	return output.WriteTable(ctx, records, fields, []string{"CREATED", "ENVIRONMENT", "COMMIT", "REF", "ACTOR"})
}
