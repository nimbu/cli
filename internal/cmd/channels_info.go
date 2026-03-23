package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/migrate"
	"github.com/nimbu/cli/internal/output"
)

// ChannelsInfoCmd shows rich channel info.
type ChannelsInfoCmd struct {
	Channel    string `arg:"" help:"Channel slug or site/channel"`
	TypeScript bool   `name:"typescript" help:"Render a TypeScript interface instead of the rich summary"`
}

// Run executes channel info.
func (c *ChannelsInfoCmd) Run(ctx context.Context, flags *RootFlags) error {
	ref, err := parseChannelRefForCommand(ctx, c.Channel, "")
	if err != nil {
		return err
	}
	client, err := GetAPIClientWithBaseURL(ctx, ref.BaseURL, ref.Site)
	if err != nil {
		return err
	}
	info, err := migrate.ChannelInfo(ctx, client, ref)
	if err != nil {
		return err
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, info)
	}
	if c.TypeScript {
		_, err := output.Fprintf(ctx, "%s\n", info.TypeScript)
		return err
	}
	if mode.Plain {
		return output.Plain(ctx, info.Channel.ID, info.Channel.Slug, info.Channel.Name, len(info.Channel.Customizations))
	}

	if _, err := output.Fprintf(ctx, "Summary\n"); err != nil {
		return err
	}
	if _, err := output.Fprintf(ctx, "ID:               %s\n", info.Channel.ID); err != nil {
		return err
	}
	if _, err := output.Fprintf(ctx, "Slug:             %s\n", info.Channel.Slug); err != nil {
		return err
	}
	if _, err := output.Fprintf(ctx, "Name:             %s\n", info.Channel.Name); err != nil {
		return err
	}
	if info.Channel.Description != "" {
		if _, err := output.Fprintf(ctx, "Description:      %s\n", info.Channel.Description); err != nil {
			return err
		}
	}
	if _, err := output.Fprintf(ctx, "Fields:           %d\n", len(info.Channel.Customizations)); err != nil {
		return err
	}
	if _, err := output.Fprintf(ctx, "Direct deps:      %s\n", joinOrNone(info.Dependencies)); err != nil {
		return err
	}
	if _, err := output.Fprintf(ctx, "Direct dependants:%s%s\n", spacerForLabel("Direct dependants:"), joinOrNone(info.Dependants)); err != nil {
		return err
	}
	if _, err := output.Fprintf(ctx, "Circular:         %v\n", info.Circular); err != nil {
		return err
	}
	if _, err := output.Fprintf(ctx, "\nCustom Fields (%d)\n", len(info.Channel.Customizations)); err != nil {
		return err
	}
	for _, field := range info.Channel.Customizations {
		if _, err := output.Fprintf(ctx, "  - %s (%s)\n", field.Name, field.Type); err != nil {
			return err
		}
	}
	return nil
}

func spacerForLabel(label string) string {
	if len(label) >= 17 {
		return " "
	}
	return fmt.Sprintf("%*s", 17-len(label), "")
}
