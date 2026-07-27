package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/muesli/termenv"

	"github.com/nimbu/cli/internal/output"
	"github.com/nimbu/cli/internal/themes"
)

// ThemeDiffCmd compares local liquid files with the remote theme.
type ThemeDiffCmd struct {
	Theme   string `help:"Override theme from nimbu.yml"`
	Content bool   `help:"Show line-by-line changes for each file, like git diff"`
}

// Run executes the diff command.
func (c *ThemeDiffCmd) Run(ctx context.Context, flags *RootFlags) error {
	projectRoot, projectCfg, warnings, err := resolveThemeProjectConfig()
	if err != nil {
		return err
	}
	for _, warning := range warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
	}

	resolved, err := themes.ResolveConfig(projectRoot, projectCfg, c.Theme)
	if err != nil {
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

	result, err := themes.RunDiffWithOptions(ctx, client, resolved, themes.DiffOptions{Content: c.Content})
	if err != nil {
		return err
	}
	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, result)
	}
	if mode.Plain {
		for _, item := range result.Entries {
			if _, err := output.Fprintf(ctx, "%s\t%s\n", item.Status, item.Path); err != nil {
				return err
			}
			if item.Diff != "" {
				if _, err := output.Fprintf(ctx, "%s", item.Diff); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if len(result.Entries) == 0 {
		if _, err := output.Fprintln(ctx, "no differences found"); err != nil {
			return err
		}
		return nil
	}
	colorize := newDiffColorizer(ctx)
	for i, item := range result.Entries {
		if _, err := output.Fprintf(ctx, "%s %s\n", item.Status, item.Path); err != nil {
			return err
		}
		if item.Diff == "" {
			continue
		}
		if _, err := output.Fprintf(ctx, "%s", colorize(item.Diff)); err != nil {
			return err
		}
		if i < len(result.Entries)-1 {
			if _, err := output.Fprintln(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

// newDiffColorizer returns a function that colors unified diff text following
// git conventions: additions green, deletions red, hunk headers cyan.
func newDiffColorizer(ctx context.Context) func(string) string {
	writer := output.WriterFromContext(ctx)
	if writer == nil || !writer.UseColor() {
		return func(s string) string { return s }
	}
	profile := termenv.EnvColorProfile()
	if writer.Color == "always" {
		profile = termenv.TrueColor
	}
	if profile == termenv.Ascii {
		return func(s string) string { return s }
	}
	green := termenv.String().Foreground(profile.Color("2"))
	red := termenv.String().Foreground(profile.Color("1"))
	cyan := termenv.String().Foreground(profile.Color("6"))
	bold := termenv.String().Bold()
	return func(diff string) string {
		lines := strings.Split(diff, "\n")
		for i, line := range lines {
			switch {
			case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
				lines[i] = bold.Styled(line)
			case strings.HasPrefix(line, "@@"):
				lines[i] = cyan.Styled(line)
			case strings.HasPrefix(line, "+"):
				lines[i] = green.Styled(line)
			case strings.HasPrefix(line, "-"):
				lines[i] = red.Styled(line)
			}
		}
		return strings.Join(lines, "\n")
	}
}
