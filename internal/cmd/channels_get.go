package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// ChannelsGetCmd gets channel details.
type ChannelsGetCmd struct {
	Channel string `arg:"" help:"Channel slug"`
}

// Run executes the get command.
func (c *ChannelsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	ch, err := api.GetChannelDetail(ctx, client, c.Channel)
	if err != nil {
		return fmt.Errorf("get channel: %w", err)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, ch)
	}

	if mode.Plain {
		return output.Plain(ctx, ch.ID, ch.Slug, ch.Name, len(ch.Customizations))
	}

	allChannels, err := api.ListChannelDetails(ctx, client)
	if err != nil {
		return fmt.Errorf("list channels for dependency graph: %w", err)
	}
	graph := api.BuildChannelDependencyGraph(allChannels)

	if err := printLine(ctx, "Summary\n"); err != nil {
		return err
	}
	if err := printLine(ctx, "ID:               %s\n", ch.ID); err != nil {
		return err
	}
	if err := printLine(ctx, "Slug:             %s\n", ch.Slug); err != nil {
		return err
	}
	if err := printLine(ctx, "Name:             %s\n", ch.Name); err != nil {
		return err
	}
	if ch.Description != "" {
		if err := printLine(ctx, "Description:      %s\n", ch.Description); err != nil {
			return err
		}
	}
	if ch.LabelField != "" {
		if err := printLine(ctx, "Label field:      %s\n", ch.LabelField); err != nil {
			return err
		}
	}
	if ch.TitleField != "" {
		if err := printLine(ctx, "Title field:      %s\n", ch.TitleField); err != nil {
			return err
		}
	}
	if ch.OrderBy != "" {
		if err := printLine(ctx, "Order:            %s %s\n", ch.OrderBy, strings.TrimSpace(ch.OrderDirection)); err != nil {
			return err
		}
	}
	if err := printLine(ctx, "Submittable:      %v\n", ch.Submittable); err != nil {
		return err
	}
	if err := printLine(ctx, "RSS enabled:      %v\n", ch.RSSEnabled); err != nil {
		return err
	}
	if ch.EntriesURL != "" {
		if err := printLine(ctx, "Entries URL:      %s\n", ch.EntriesURL); err != nil {
			return err
		}
	}
	if ch.URL != "" {
		if err := printLine(ctx, "URL:              %s\n", ch.URL); err != nil {
			return err
		}
	}

	if err := printLine(ctx, "\nACL\n"); err != nil {
		return err
	}
	if len(ch.ACL) == 0 {
		if err := printLine(ctx, "  <empty>\n"); err != nil {
			return err
		}
	} else if data, err := json.MarshalIndent(ch.ACL, "  ", "  "); err == nil {
		if err := printLine(ctx, "  %s\n", string(data)); err != nil {
			return err
		}
	}

	if err := printLine(ctx, "\nCustom Fields (%d)\n", len(ch.Customizations)); err != nil {
		return err
	}
	for _, field := range ch.Customizations {
		line := fmt.Sprintf("  - %s (%s)", field.Name, field.Type)
		if field.Label != "" {
			line += " label=" + field.Label
		}
		if field.Reference != "" {
			line += " ref=" + field.Reference
		}
		if field.Required {
			line += " required"
		}
		if field.Unique {
			line += " unique"
		}
		if field.Localized {
			line += " localized"
		}
		if err := printLine(ctx, "%s\n", line); err != nil {
			return err
		}
	}

	if err := printLine(ctx, "\nDependency Summary\n"); err != nil {
		return err
	}
	if err := printLine(ctx, "  direct deps:        %s\n", joinOrNone(graph.DirectDependencies(ch.Slug))); err != nil {
		return err
	}
	if err := printLine(ctx, "  transitive deps:    %s\n", joinOrNone(graph.TransitiveDependencies(ch.Slug))); err != nil {
		return err
	}
	if err := printLine(ctx, "  direct dependants:  %s\n", joinOrNone(graph.DirectDependants(ch.Slug))); err != nil {
		return err
	}
	if err := printLine(ctx, "  transitive dependants: %s\n", joinOrNone(graph.TransitiveDependants(ch.Slug))); err != nil {
		return err
	}
	if err := printLine(ctx, "  circular:           %v\n", graph.HasCircularDependencies(ch.Slug)); err != nil {
		return err
	}

	return nil
}

func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "<none>"
	}
	return strings.Join(values, ", ")
}
