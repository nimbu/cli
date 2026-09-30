package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/channeltypes"
	"github.com/nimbu/cli/internal/output"
)

// ChannelsTypesCmd generates one TypeScript module with the types of all channels.
type ChannelsTypesCmd struct {
	Channel []string `help:"Only include these channels (slug or ID; repeatable or comma-separated)"`
	Output  string   `help:"Write the module to this path instead of stdout (- for stdout)" short:"o"`
}

// channelsTypesResult is the --json payload.
type channelsTypesResult struct {
	Site       string   `json:"site"`
	Channels   []string `json:"channels"`
	Path       string   `json:"path,omitempty"`
	TypeScript string   `json:"typescript,omitempty"`
}

// Run executes the types command.
func (c *ChannelsTypesCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}
	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}
	if err := requireScopes(ctx, client, []string{"read_channels"}, "Example: nimbu auth scopes"); err != nil {
		return err
	}

	channels, err := api.ListChannelDetails(ctx, client)
	if err != nil {
		return fmt.Errorf("list channels: %w", err)
	}
	channels, err = selectChannels(channels, c.Channel)
	if err != nil {
		return err
	}

	module := channeltypes.Module(channels, channeltypes.ModuleOptions{
		Site: site,
		// Only a module with every channel can make unknown slugs an error.
		StrictChannels: len(c.Channel) == 0,
	})
	result := channelsTypesResult{Site: site, Channels: channelSlugs(channels)}

	if c.Output == "" || c.Output == "-" {
		if output.IsJSON(ctx) {
			result.TypeScript = module
			return output.JSON(ctx, result)
		}
		// Human and --plain both get the module verbatim so it can be piped.
		_, err := output.Fprintf(ctx, "%s", module)
		return err
	}

	if dir := filepath.Dir(c.Output); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
	}
	if err := os.WriteFile(c.Output, []byte(module), 0o644); err != nil {
		return fmt.Errorf("write types: %w", err)
	}
	result.Path = c.Output
	return output.Print(ctx, result, []any{c.Output}, func() error {
		_, err := output.Fprintf(ctx, "Wrote types for %d channel(s) to %s\n", len(result.Channels), c.Output)
		return err
	})
}

// selectChannels keeps the channels named by slug or ID, or all channels when
// no filter is given. Unknown names are an error so typos do not silently
// produce an incomplete module.
func selectChannels(channels []api.ChannelDetail, filter []string) ([]api.ChannelDetail, error) {
	wanted := map[string]bool{}
	for _, value := range filter {
		if value = strings.TrimSpace(value); value != "" {
			wanted[value] = true
		}
	}
	if len(wanted) == 0 {
		return channels, nil
	}

	found := map[string]bool{}
	var selected []api.ChannelDetail
	for _, channel := range channels {
		matched := false
		for _, key := range []string{channel.Slug, channel.ID} {
			if key != "" && wanted[key] {
				found[key] = true
				matched = true
			}
		}
		if matched {
			selected = append(selected, channel)
		}
	}

	var missing []string
	for value := range wanted {
		if !found[value] {
			missing = append(missing, value)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("channel not found: %s", strings.Join(missing, ", "))
	}
	return selected, nil
}

func channelSlugs(channels []api.ChannelDetail) []string {
	slugs := make([]string, 0, len(channels))
	for _, channel := range channels {
		slugs = append(slugs, channel.Slug)
	}
	sort.Strings(slugs)
	return slugs
}
