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
	resolved, host, resolveErr := resolveEnvironmentReference(raw, hostOverride)
	if resolveErr != nil {
		return migrate.SiteRef{}, resolveErr
	}
	raw, hostOverride = resolved, host
	flags := ctx.Value(rootFlagsKey{}).(*RootFlags)
	defaultSite, err := RequireSite(ctx, "")
	if err != nil && strings.TrimSpace(raw) == "" {
		return migrate.SiteRef{}, err
	}
	return migrate.ParseSiteRef(raw, hostOverride, defaultSite, flags.APIURL)
}

func parseChannelRefForCommand(ctx context.Context, raw, hostOverride string) (migrate.ChannelRef, error) {
	if strings.Contains(strings.Trim(strings.TrimSpace(raw), "/"), "/") {
		resolved, host, resolveErr := resolveEnvironmentReference(raw, hostOverride)
		if resolveErr != nil {
			return migrate.ChannelRef{}, resolveErr
		}
		raw, hostOverride = resolved, host
	}
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

// renderDiffChanges renders diff changes as tab-separated lines for human
// output; --plain uses plainDiffChanges.
func renderDiffChanges(prefix string, changes []migrate.DiffChange) []string {
	return renderDiffLines(prefix, changes, func(s string) string { return s })
}

// plainDiffChanges is renderDiffChanges for --plain: each field goes through
// output.EscapeTSV, so a value holding a tab or newline (a field hint, a
// select option label) stays one escaped field instead of splitting the
// record.
func plainDiffChanges(prefix string, changes []migrate.DiffChange) []string {
	return renderDiffLines(prefix, changes, output.EscapeTSV)
}

func renderDiffLines(prefix string, changes []migrate.DiffChange, escape func(string) string) []string {
	lines := make([]string, 0, len(changes))
	for _, change := range changes {
		fields := []string{prefix, change.Path}
		if change.Kind == "updated" {
			fields = append(fields, fmt.Sprintf("%v", change.From), fmt.Sprintf("%v", change.To))
		}
		for i, field := range fields {
			fields[i] = escape(field)
		}
		lines = append(lines, strings.Join(fields, "\t"))
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
