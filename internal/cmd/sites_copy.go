package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nimbu/cli/internal/migrate"
	"github.com/nimbu/cli/internal/output"
)

// SitesCopyCmd copies major site resources between sites.
type SitesCopyCmd struct {
	From          string `help:"Source site" required:"" name:"from"`
	To            string `help:"Target site" required:"" name:"to"`
	FromHost      string `help:"Source API base URL or host" name:"from-host"`
	ToHost        string `help:"Target API base URL or host" name:"to-host"`
	EntryChannels string `help:"Comma-separated channels whose entries should also be copied" name:"entry-channels"`
	Only          string `help:"Comma-separated channel allowlist when using --recursive"`
	Recursive     bool   `help:"Recursively copy dependent channel entries"`
	Upsert        string `help:"Comma-separated upsert fields for entry-copy stage"`
	CopyCustomers bool   `name:"copy-customers" help:"Copy related customers when copying channel entries"`
	AllowErrors   bool   `name:"allow-errors" help:"Continue on item-level validation errors during record copy"`
	DryRun        bool   `name:"dry-run" help:"Show what would be copied without writing to target site"`
	SkipCloudCode bool   `name:"skip-cloud-code" help:"Skip copying cloud code app files"`
}

// Run executes sites copy.
func (c *SitesCopyCmd) Run(ctx context.Context, flags *RootFlags) error {
	if !c.DryRun {
		if err := requireWrite(flags, "copy site"); err != nil {
			return err
		}
	}
	fromRef, err := parseSiteRefForCommand(ctx, c.From, c.FromHost)
	if err != nil {
		return err
	}
	toRef, err := parseSiteRefForCommand(ctx, c.To, c.ToHost)
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

	mode := output.FromContext(ctx)

	// Wire timeline for human mode
	var tl *output.CopyTimeline
	if !mode.JSON && !mode.Plain {
		tl = output.NewCopyTimeline(ctx, c.DryRun)
		ctx = migrate.WithCopyObserver(ctx, tl)
		ctx = output.WithProgress(ctx, output.NewDisabledProgress())
		tl.Header(fromRef.Site, toRef.Site)
	}

	result, err := migrate.CopySite(ctx, fromClient, toClient, fromRef, toRef, migrate.SiteCopyOptions{
		AllowErrors:      c.AllowErrors,
		ConflictResolver: siteCopyConflictResolver(flags, tl),
		CopyCustomers:    c.CopyCustomers,
		DryRun:           c.DryRun,
		Force:            flags != nil && flags.Force,
		Include:          splitCSV(c.EntryChannels),
		Only:             splitCSV(c.Only),
		Recursive:        c.Recursive,
		SkipCloudCode:    c.SkipCloudCode,
		Upsert:           c.Upsert,
	})

	if tl != nil {
		if err == nil {
			tl.Footer()
		} else {
			tl.ErrorFooter(err.Error())
			err = &displayedError{err: err}
		}
		tl.Close()
	}

	if err != nil {
		return err
	}

	if mode.JSON {
		return output.JSON(ctx, result)
	}
	if mode.Plain {
		prefix := ""
		if result.DryRun {
			prefix = "[dry-run] "
		}
		if err := output.Plain(ctx, prefix+"uploads", len(result.Uploads.Items)); err != nil {
			return err
		}
		if err := output.Plain(ctx, prefix+"channels", len(result.Channels.Items)); err != nil {
			return err
		}
		if err := output.Plain(ctx, prefix+"channel_entries", len(result.ChannelEntries)); err != nil {
			return err
		}
		if err := output.Plain(ctx, prefix+"roles", len(result.Roles.Items)); err != nil {
			return err
		}
		if err := output.Plain(ctx, prefix+"products", len(result.Products.Items)); err != nil {
			return err
		}
		if err := output.Plain(ctx, prefix+"collections", len(result.Collections.Items)); err != nil {
			return err
		}
		if err := output.Plain(ctx, prefix+"shipping_rates", result.ShippingRates.Skipped); err != nil {
			return err
		}
		if err := output.Plain(ctx, prefix+"pages", len(result.Pages.Items)); err != nil {
			return err
		}
		if err := output.Plain(ctx, prefix+"consent", result.Consent.Action); err != nil {
			return err
		}
		if err := output.Plain(ctx, prefix+"menus", len(result.Menus.Items)); err != nil {
			return err
		}
		if err := output.Plain(ctx, prefix+"blogs", len(result.Blogs.Items)); err != nil {
			return err
		}
		if err := output.Plain(ctx, prefix+"notifications", len(result.Notifications.Items)); err != nil {
			return err
		}
		if err := output.Plain(ctx, prefix+"redirects", len(result.Redirects.Items)); err != nil {
			return err
		}
		if err := output.Plain(ctx, prefix+"translations", len(result.Translations.Items)); err != nil {
			return err
		}
		if err := output.Plain(ctx, prefix+"cloud_code", len(result.CloudCode.Items)); err != nil {
			return err
		}
		err := output.Plain(ctx, prefix+"warnings", len(result.Warnings))
		return err
	}

	// Human mode: timeline already rendered during execution
	return nil
}

func siteCopyConflictResolver(flags *RootFlags, tl *output.CopyTimeline) migrate.ExistingContentResolver {
	reader := bufio.NewReader(os.Stdin)
	return func(ctx context.Context, prompt migrate.ExistingContentPrompt) (migrate.ExistingContentDecision, error) {
		if flags != nil && flags.Force {
			return migrate.ExistingContentDecision{Action: migrate.ExistingContentUpdate, ApplyToAll: true}, nil
		}
		if reason := promptBlocker(flags); reason != "" {
			return migrate.ExistingContentDecision{}, confirmationRequiredError(
				fmt.Sprintf("refusing to update existing %s without --force (cannot prompt: %s)", strings.ToLower(prompt.Type), reason),
				"--force")
		}
		if tl != nil {
			tl.PreparePrompt()
		}
		w := output.WriterFromContext(ctx)
		errw := w.Err
		if errw == nil {
			errw = w.Out
		}
		if errw == nil {
			errw = os.Stderr
		}
		style := newSiteCopyPromptStyle(w)
		defer beginPrompt()()
		if prompt.Item != "" {
			return readSiteCopyItemConflictDecision(reader, errw, prompt, style)
		}
		return readSiteCopyConflictDecision(reader, errw, prompt, style)
	}
}

func readSiteCopyConflictDecision(reader *bufio.Reader, writer io.Writer, prompt migrate.ExistingContentPrompt, styles ...siteCopyPromptStyle) (migrate.ExistingContentDecision, error) {
	style := firstSiteCopyPromptStyle(styles)
	if _, err := fmt.Fprintf(writer, "\n%s %s\n", style.dim("◇"), style.bright(prompt.Type)); err != nil {
		return migrate.ExistingContentDecision{}, err
	}
	if _, err := fmt.Fprintf(writer, "  %s\n\n", style.dim(siteCopyConflictSummary(prompt))); err != nil {
		return migrate.ExistingContentDecision{}, err
	}
	if _, err := fmt.Fprintf(writer, "%s existing %s?\n", style.bright("Update"), strings.ToLower(prompt.Type)); err != nil {
		return migrate.ExistingContentDecision{}, err
	}
	if err := writeSiteCopyTypeHelp(writer, style); err != nil {
		return migrate.ExistingContentDecision{}, err
	}
	for {
		if _, err := fmt.Fprintf(writer, "\n%s ", style.dim("Choice [Y/r/n/a/s/q/?]:")); err != nil {
			return migrate.ExistingContentDecision{}, err
		}
		answer, err := reader.ReadString('\n')
		if err != nil && len(answer) == 0 {
			return migrate.ExistingContentDecision{}, err
		}
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "", "y", "yes":
			return migrate.ExistingContentDecision{Action: migrate.ExistingContentUpdate}, nil
		case "r", "review":
			return migrate.ExistingContentDecision{Action: migrate.ExistingContentReview}, nil
		case "n", "no":
			return migrate.ExistingContentDecision{Action: migrate.ExistingContentSkip}, nil
		case "a", "all", "yes-all", "yesall":
			return migrate.ExistingContentDecision{Action: migrate.ExistingContentUpdate, ApplyToAll: true}, nil
		case "s", "skip-all", "skipall":
			return migrate.ExistingContentDecision{Action: migrate.ExistingContentSkip, ApplyToAll: true}, nil
		case "q", "quit", "abort":
			return migrate.ExistingContentDecision{Action: migrate.ExistingContentAbort}, nil
		case "?":
			if err := writeSiteCopyTypeHelp(writer, style); err != nil {
				return migrate.ExistingContentDecision{}, err
			}
		default:
			_, _ = fmt.Fprintln(writer, style.dim("Please answer y, r, n, a, s, q, or ?."))
		}
	}
}

func readSiteCopyItemConflictDecision(reader *bufio.Reader, writer io.Writer, prompt migrate.ExistingContentPrompt, styles ...siteCopyPromptStyle) (migrate.ExistingContentDecision, error) {
	style := firstSiteCopyPromptStyle(styles)
	for {
		if _, err := fmt.Fprintf(writer, "%s %s already exists. Update it? %s ", style.dim(siteCopySingular(prompt.Type)), style.bright(prompt.Item), style.dim("[Y/n/a/s/q/?]:")); err != nil {
			return migrate.ExistingContentDecision{}, err
		}
		answer, err := reader.ReadString('\n')
		if err != nil && len(answer) == 0 {
			return migrate.ExistingContentDecision{}, err
		}
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "", "y", "yes":
			return migrate.ExistingContentDecision{Action: migrate.ExistingContentUpdate}, nil
		case "n", "no":
			return migrate.ExistingContentDecision{Action: migrate.ExistingContentSkip}, nil
		case "a", "all", "yes-all", "yesall":
			return migrate.ExistingContentDecision{Action: migrate.ExistingContentUpdate, ApplyToAll: true}, nil
		case "s", "skip-all", "skipall":
			return migrate.ExistingContentDecision{Action: migrate.ExistingContentSkip, ApplyToAll: true}, nil
		case "q", "quit", "abort":
			return migrate.ExistingContentDecision{Action: migrate.ExistingContentAbort}, nil
		case "?":
			_, _ = fmt.Fprintln(writer, style.dim("Enter updates this item. n skips this item. a updates all remaining items in this type. s skips all remaining items in this type. q aborts."))
		default:
			_, _ = fmt.Fprintln(writer, style.dim("Please answer y, n, a, s, q, or ?."))
		}
	}
}

func writeSiteCopyTypeHelp(writer io.Writer, style siteCopyPromptStyle) error {
	lines := []struct {
		key  string
		text string
	}{
		{"Enter", "update all existing items in this type"},
		{"r", "review items one by one"},
		{"n", "skip existing items in this type"},
		{"a", "update all remaining existing types"},
		{"s", "skip all remaining existing types"},
		{"q", "abort"},
	}
	for _, line := range lines {
		if _, err := fmt.Fprintf(writer, "  %s  %s\n", style.bright(line.key), style.dim(line.text)); err != nil {
			return err
		}
	}
	return nil
}

func siteCopyConflictSummary(prompt migrate.ExistingContentPrompt) string {
	if prompt.SourceCount > 0 || prompt.ExistingCount > 0 {
		return fmt.Sprintf("%d source %s · %d already exist on %s", prompt.SourceCount, strings.ToLower(prompt.Type), prompt.ExistingCount, prompt.Target)
	}
	return fmt.Sprintf("existing %s found on %s", strings.ToLower(prompt.Type), prompt.Target)
}

func siteCopySingular(kind string) string {
	kind = strings.TrimSpace(kind)
	switch kind {
	case "Channels":
		return "Channel"
	case "Menus":
		return "Menu"
	default:
		return strings.TrimSuffix(kind, "s")
	}
}

// siteCopyPromptStyle styles conflict prompts, which are written to stderr.
type siteCopyPromptStyle struct {
	theme output.Theme
}

func newSiteCopyPromptStyle(w *output.Writer) siteCopyPromptStyle {
	if w == nil {
		return siteCopyPromptStyle{}
	}
	return siteCopyPromptStyle{theme: w.ErrTheme()}
}

func firstSiteCopyPromptStyle(styles []siteCopyPromptStyle) siteCopyPromptStyle {
	if len(styles) > 0 {
		return styles[0]
	}
	return siteCopyPromptStyle{}
}

func (s siteCopyPromptStyle) bright(value string) string { return s.theme.Strong(value) }

func (s siteCopyPromptStyle) dim(value string) string { return s.theme.Muted(value) }
