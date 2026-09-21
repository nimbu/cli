package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/nimbu/cli/internal/output"
	"github.com/nimbu/cli/internal/realtime"
)

// watchPrinter renders realtime events for `channels entries watch`.
//
// In --json mode stdout carries nothing but raw server event envelopes, one per
// line; every control and status line goes to stderr.
type watchPrinter struct {
	ctx      context.Context
	writer   *output.Writer
	mode     output.Mode
	verbose  bool
	useColor bool
	channel  string
	filters  []string
	where    string
}

func newWatchPrinter(ctx context.Context, channel string, filters []string, where string) *watchPrinter {
	writer := output.WriterFromContext(ctx)
	flags := rootFlagsFromContext(ctx, nil)
	return &watchPrinter{
		ctx:      ctx,
		writer:   writer,
		mode:     output.FromContext(ctx),
		verbose:  flags.Verbose || flags.Debug,
		useColor: writer.UseColor(),
		channel:  channel,
		filters:  filters,
		where:    where,
	}
}

// OnStatus writes a human status line to stderr.
func (p *watchPrinter) OnStatus(msg string) {
	if p.mode.JSON || p.mode.Plain {
		if !p.verbose {
			return
		}
	}
	_, _ = fmt.Fprintln(p.writer.Err, msg)
}

// OnControl reports protocol control frames under --verbose only.
func (p *watchPrinter) OnControl(c realtime.Control) {
	if !p.verbose {
		return
	}
	detail := c.Type
	if c.Code != "" {
		detail += " " + c.Code
	}
	if c.Inactive {
		detail += " inactive"
	}
	_, _ = fmt.Fprintf(p.writer.Err, "verbose: control %s\n", detail)
}

// OnEvent writes one event to stdout in the active output mode.
func (p *watchPrinter) OnEvent(ev realtime.Event) {
	switch {
	case p.mode.JSON:
		_, _ = fmt.Fprintln(p.writer.Out, watchEventJSONLine(ev))
	case p.mode.Plain:
		_, _ = fmt.Fprintf(p.writer.Out, "%s\t%s\t%s\t%s\n",
			watchEventTime(ev).Format("15:04:05"), ev.Event, ev.ID, watchEventTitle(ev))
	default:
		_, _ = fmt.Fprintln(p.writer.Out, p.humanEventLine(ev))
	}
}

func (p *watchPrinter) humanEventLine(ev realtime.Event) string {
	stamp := watchEventTime(ev).Format("15:04:05")
	label := p.colorEvent(fmt.Sprintf("%-8s", ev.Event), ev.Event)

	if ev.Event == "resync" {
		return fmt.Sprintf("%s  %s  —  re-read the channel with: %s", stamp, label, p.resyncHint())
	}
	if title := watchEventTitle(ev); title != "" {
		return fmt.Sprintf("%s  %s  %s  %s", stamp, label, ev.ID, title)
	}
	return fmt.Sprintf("%s  %s  %s", stamp, label, ev.ID)
}

// resyncHint is the list command that re-reads the same slice of the channel.
func (p *watchPrinter) resyncHint() string {
	parts := []string{"nimbu channels entries list --channel " + p.channel}
	for _, filter := range p.filters {
		parts = append(parts, "--filters "+shellQuoteIfNeeded(filter))
	}
	if p.where != "" {
		parts = append(parts, "--where "+shellQuoteIfNeeded(p.where))
	}
	return strings.Join(parts, " ")
}

func (p *watchPrinter) colorEvent(padded, event string) string {
	if !p.useColor {
		return padded
	}
	style := lipgloss.NewStyle()
	switch event {
	case "added":
		style = style.Foreground(lipgloss.Color("#22c55e"))
	case "changed":
		style = style.Foreground(lipgloss.Color("#f59e0b"))
	case "removed":
		style = style.Foreground(lipgloss.Color("#ef4444"))
	case "resync":
		style = style.Faint(true)
	default:
		return padded
	}
	return style.Render(padded)
}

// watchEventJSONLine returns the server envelope as a single compact line.
func watchEventJSONLine(ev realtime.Event) string {
	raw := bytes.TrimSpace(ev.Raw)
	if len(raw) > 0 {
		var compact bytes.Buffer
		if err := json.Compact(&compact, raw); err == nil {
			return compact.String()
		}
		return string(raw)
	}
	encoded, err := json.Marshal(ev)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func watchEventTime(ev realtime.Event) time.Time {
	if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(ev.OccurredAt)); err == nil {
		return parsed.Local()
	}
	return time.Now()
}

// watchEventTitle mirrors the title fallback used by `channels entries list`.
func watchEventTitle(ev realtime.Event) string {
	if len(bytes.TrimSpace(ev.Object)) == 0 {
		return ""
	}
	var object map[string]any
	if err := json.Unmarshal(ev.Object, &object); err != nil {
		return ""
	}
	if title, _ := object["title"].(string); strings.TrimSpace(title) != "" {
		return title
	}
	if fields, ok := object["fields"].(map[string]any); ok {
		if title, _ := fields["title"].(string); strings.TrimSpace(title) != "" {
			return title
		}
	}
	for _, key := range []string{"title_field_value", "name", "slug"} {
		if value, _ := object[key].(string); strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func shellQuoteIfNeeded(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t'\"$`\\") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
