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
	styles := newDiffStyles(ctx)
	for i, item := range result.Entries {
		if _, err := output.Fprintf(ctx, "%s\n", styles.statusLine(item.Status, item.Path)); err != nil {
			return err
		}
		if item.Diff == "" {
			continue
		}
		if _, err := output.Fprintf(ctx, "%s", styles.colorize(item.Diff)); err != nil {
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

// diffStyles colors themes diff output following git conventions: additions
// green, deletions red, hunk headers cyan, file headers bold.
type diffStyles struct {
	enabled bool
	green   termenv.Style
	red     termenv.Style
	cyan    termenv.Style
	yellow  termenv.Style
	bold    termenv.Style
}

func newDiffStyles(ctx context.Context) diffStyles {
	writer := output.WriterFromContext(ctx)
	if writer == nil || !writer.UseColor() {
		return diffStyles{}
	}
	profile := termenv.EnvColorProfile()
	if writer.Color == "always" {
		profile = termenv.TrueColor
	}
	if profile == termenv.Ascii {
		return diffStyles{}
	}
	return diffStyles{
		enabled: true,
		green:   profile.String().Foreground(profile.Color("2")),
		red:     profile.String().Foreground(profile.Color("1")),
		cyan:    profile.String().Foreground(profile.Color("6")),
		yellow:  profile.String().Foreground(profile.Color("3")),
		bold:    profile.String().Bold(),
	}
}

func (s diffStyles) statusLine(status, path string) string {
	if !s.enabled {
		return status + " " + path
	}
	styledStatus := s.yellow.Styled(status)
	if status == "missing" {
		styledStatus = s.red.Styled(status)
	}
	return styledStatus + " " + s.bold.Styled(path)
}

func (s diffStyles) colorize(diff string) string {
	if !s.enabled {
		return diff
	}
	lines := strings.Split(diff, "\n")
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
			lines[i] = s.bold.Styled(line)
		case strings.HasPrefix(line, "@@"):
			lines[i] = s.cyan.Styled(line)
		case strings.HasPrefix(line, "+"):
			lines[i] = s.green.Styled(line)
		case strings.HasPrefix(line, "-"):
			lines[i] = s.red.Styled(line)
		}
	}
	return strings.Join(lines, "\n")
}
