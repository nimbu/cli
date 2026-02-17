package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/output"
)

// ChannelsFieldsCmd lists channel fields.
type ChannelsFieldsCmd struct {
	Channel string `arg:"" help:"Channel ID or slug"`
}

// Run executes the list fields command.
func (c *ChannelsFieldsCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	path := "/channels/" + url.PathEscape(c.Channel) + "/customizations"
	var fields []map[string]any
	if err := client.Get(ctx, path, &fields); err != nil {
		return fmt.Errorf("list channel fields: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, fields)
	}

	if mode.Plain {
		data, err := json.Marshal(fields)
		if err != nil {
			return fmt.Errorf("serialize channel fields: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	data, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return fmt.Errorf("serialize channel fields: %w", err)
	}
	fmt.Printf("%s\n", string(data))
	return nil
}
