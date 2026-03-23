package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nimbu/cli/internal/migrate"
	"github.com/nimbu/cli/internal/output"
)

func parseSiteRefForCommand(ctx context.Context, raw, hostOverride string) (migrate.SiteRef, error) {
	flags := ctx.Value(rootFlagsKey{}).(*RootFlags)
	defaultSite, err := RequireSite(ctx, "")
	if err != nil && strings.TrimSpace(raw) == "" {
		return migrate.SiteRef{}, err
	}
	return migrate.ParseSiteRef(raw, hostOverride, defaultSite, flags.APIURL)
}

func parseChannelRefForCommand(ctx context.Context, raw, hostOverride string) (migrate.ChannelRef, error) {
	flags := ctx.Value(rootFlagsKey{}).(*RootFlags)
	defaultSite := ""
	if site, err := RequireSite(ctx, ""); err == nil {
		defaultSite = site
	}
	return migrate.ParseChannelRef(raw, hostOverride, defaultSite, flags.APIURL)
}

func parseSinceFlag(raw string) (string, error) {
	return migrate.ParseSinceValue(raw, time.Now())
}

func writeDiffSet(ctx context.Context, diff any, plainLines []string, humanLines []string) error {
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, diff)
	}
	if mode.Plain {
		for _, line := range plainLines {
			if _, err := output.Fprintf(ctx, "%s\n", line); err != nil {
				return err
			}
		}
		return nil
	}
	for _, line := range humanLines {
		if _, err := output.Fprintf(ctx, "%s\n", line); err != nil {
			return err
		}
	}
	return nil
}

func renderDiffChanges(prefix string, changes []migrate.DiffChange) []string {
	lines := make([]string, 0, len(changes))
	for _, change := range changes {
		line := fmt.Sprintf("%s\t%s", prefix, change.Path)
		if change.Kind == "updated" {
			line = fmt.Sprintf("%s\t%s\t%v\t%v", prefix, change.Path, change.From, change.To)
		}
		lines = append(lines, line)
	}
	return lines
}

func splitCSV(raw string) []string {
	parts := strings.Split(strings.TrimSpace(raw), ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
