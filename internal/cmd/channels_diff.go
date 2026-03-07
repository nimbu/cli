package cmd

import (
	"context"

	"github.com/nimbu/cli/internal/migrate"
	"github.com/nimbu/cli/internal/output"
)

// ChannelsDiffCmd diffs two channels between sites.
type ChannelsDiffCmd struct {
	From     string `help:"Source site/channel" required:"" name:"from"`
	To       string `help:"Target site/channel" required:"" name:"to"`
	FromHost string `help:"Source API base URL or host" name:"from-host"`
	ToHost   string `help:"Target API base URL or host" name:"to-host"`
}

// Run executes channel diff.
func (c *ChannelsDiffCmd) Run(ctx context.Context, flags *RootFlags) error {
	fromRef, err := parseChannelRefForCommand(ctx, c.From, c.FromHost)
	if err != nil {
		return err
	}
	toRef, err := parseChannelRefForCommand(ctx, c.To, c.ToHost)
	if err != nil {
		return err
	}
	fromClient, err := GetAPIClientWithBaseURL(ctx, fromRef.BaseURL, fromRef.Site)
	if err != nil {
		return err
	}
	toClient, err := GetAPIClientWithBaseURL(ctx, toRef.BaseURL, toRef.Site)
	if err != nil {
		return err
	}
	result, err := migrate.DiffChannel(ctx, fromClient, toClient, fromRef, toRef)
	if err != nil {
		return err
	}
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}

	plainLines := append(renderDiffChanges("channel:add", result.ChannelDiff.Added), renderDiffChanges("channel:remove", result.ChannelDiff.Removed)...)
	plainLines = append(plainLines, renderDiffChanges("channel:update", result.ChannelDiff.Updated)...)
	plainLines = append(plainLines, renderDiffChanges("field:add", result.FieldsDiff.Added)...)
	plainLines = append(plainLines, renderDiffChanges("field:remove", result.FieldsDiff.Removed)...)
	plainLines = append(plainLines, renderDiffChanges("field:update", result.FieldsDiff.Updated)...)

	humanLines := []string{"Channel attributes"}
	humanLines = append(humanLines, renderDiffChanges("  +", result.ChannelDiff.Added)...)
	humanLines = append(humanLines, renderDiffChanges("  -", result.ChannelDiff.Removed)...)
	humanLines = append(humanLines, renderDiffChanges("  ~", result.ChannelDiff.Updated)...)
	humanLines = append(humanLines, "Custom fields")
	humanLines = append(humanLines, renderDiffChanges("  +", result.FieldsDiff.Added)...)
	humanLines = append(humanLines, renderDiffChanges("  -", result.FieldsDiff.Removed)...)
	humanLines = append(humanLines, renderDiffChanges("  ~", result.FieldsDiff.Updated)...)
	if len(plainLines) == 0 {
		plainLines = []string{"equal\t$"}
		humanLines = []string{"There are no differences."}
	}
	return writeDiffSet(ctx, result, plainLines, humanLines)
}
