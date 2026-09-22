package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// realtimeGrantPayload is the JSON/plain shape of a minted grant.
type realtimeGrantPayload struct {
	Grant     string `json:"grant"`
	ExpiresAt string `json:"expires_at"`
}

// RealtimeGrantCmd mints a single-use realtime grant.
type RealtimeGrantCmd struct{}

// Run executes the grant command.
func (c *RealtimeGrantCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	grant, err := api.MintRealtimeGrant(ctx, client)
	if err != nil {
		return fmt.Errorf("mint realtime grant: %w", err)
	}

	expiresAt := ""
	if !grant.ExpiresAt.IsZero() {
		expiresAt = grant.ExpiresAt.UTC().Format(time.RFC3339)
	}

	writer := output.WriterFromContext(ctx)
	_, _ = fmt.Fprintf(writer.Err, "warning: this grant is single-use and expires at %s; treat it as a secret\n", expiresAt)

	payload := realtimeGrantPayload{Grant: grant.Grant, ExpiresAt: expiresAt}
	return output.Detail(ctx, payload, []any{payload.Grant, payload.ExpiresAt}, []output.Field{
		output.FAlways("grant", payload.Grant),
		output.FAlways("expires_at", payload.ExpiresAt),
	})
}
