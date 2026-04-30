package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// UploadsGetCmd gets upload details.
type UploadsGetCmd struct {
	ID string `required:"" help:"Upload ID"`
}

// Run executes the get command.
func (c *UploadsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	var upload api.Upload
	path := "/uploads/" + url.PathEscape(c.ID)
	if err := client.Get(ctx, path, &upload); err != nil {
		return fmt.Errorf("get upload: %w", err)
	}

	var created string
	if !upload.CreatedAt.IsZero() {
		created = upload.CreatedAt.Format("2006-01-02 15:04:05")
	}

	return output.Detail(ctx, upload, []any{upload.ID, upload.Name, upload.URL, upload.Size, upload.MimeType}, []output.Field{
		output.FAlways("ID", upload.ID),
		output.FAlways("Name", upload.Name),
		output.F("URL", upload.URL),
		output.F("Size", upload.Size),
		output.F("MIME Type", upload.MimeType),
		output.F("Created", created),
	})
}
