package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ChannelsCreateCmd creates a channel.
type ChannelsCreateCmd struct {
	File        string   `help:"Read channel JSON from file (use - for stdin)"`
	Assignments []string `arg:"" optional:"" help:"Inline assignments (e.g. name=Testimonials slug=testimonials title_field=author; customizations:=@fields.json)"`
}

// Run executes the create command.
func (c *ChannelsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "create channel"); err != nil {
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

	body, err := readJSONBodyInput(c.File, c.Assignments)
	if err != nil {
		return err
	}

	channel, err := api.CreateChannel(ctx, client, body)
	if err != nil {
		return fmt.Errorf("create channel: %w", err)
	}

	return output.Print(ctx, channel, []any{channel.ID, channel.Slug, channel.Name}, func() error {
		_, err := output.Fprintf(ctx, "Created channel %s\n", channel.Slug)
		return err
	})
}
