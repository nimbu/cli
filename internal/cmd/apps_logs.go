package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/apps"
	"github.com/nimbu/cli/internal/output"
)

const appsLogsDefaultLimit = 100

var (
	appsLogsNow          = time.Now
	appsLogsPollInterval = 2 * time.Second
)

// AppsLogsCmd reads app cloud-code logs.
type AppsLogsCmd struct {
	App   string `required:"" help:"Application local name or key"`
	Tail  bool   `help:"Poll for new logs until interrupted" short:"t"`
	Level string `help:"Minimum severity: debug, info, warn, error, fatal"`
	Query string `help:"Text search query" short:"q" name:"query"`
	Job   string `help:"Only logs from this background job name"`
	Since string `help:"Only logs after this value; raw epoch/ISO8601 or duration like 15m, 1h, 24h"`
	Limit int    `help:"Maximum logs to fetch" default:"100"`
}

// Run executes apps logs.
func (c *AppsLogsCmd) Run(ctx context.Context, flags *RootFlags) error {
	flags = rootFlagsFromContext(ctx, flags)
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}
	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}
	appKey, err := resolveAppsLogsAppKey(ctx, flags, site, c.App)
	if err != nil {
		return err
	}
	since, err := normalizeAppsLogsSince(c.Since, appsLogsNow())
	if err != nil {
		return err
	}

	opts := api.AppLogOptions{
		Since: since,
		Level: c.Level,
		Query: c.Query,
		Job:   c.Job,
		Limit: effectiveAppsLogsLimit(c.Limit),
	}
	if c.Tail {
		return c.tail(ctx, client, appKey, opts)
	}

	logs, err := api.ListAppLogs(ctx, client, appKey, opts)
	if err != nil {
		return fmt.Errorf("list app logs: %w", err)
	}
	return writeAppLogs(ctx, logs)
}

func (c *AppsLogsCmd) tail(ctx context.Context, client *api.Client, appKey string, opts api.AppLogOptions) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	logs, err := api.ListAppLogs(ctx, client, appKey, opts)
	if err != nil {
		if contextCanceled(ctx, err) {
			return nil
		}
		return fmt.Errorf("tail app logs: %w", err)
	}
	if opts.Since == "" {
		reverseAppLogs(logs)
	}
	if err := writeAppLogs(ctx, logs); err != nil {
		return err
	}

	since := opts.Since
	cursor := newestAppLogTime(logs)
	seenAtCursor := appLogIDsAtTime(logs, cursor)
	if cursor > 0 {
		since = formatAppLogSince(cursor)
	} else if since == "" {
		since = formatAppLogEpoch(appsLogsNow())
	}

	ticker := time.NewTicker(appsLogsPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			pollOpts := opts
			pollOpts.Since = since
			logs, err := api.ListAppLogs(ctx, client, appKey, pollOpts)
			if err != nil {
				if contextCanceled(ctx, err) {
					return nil
				}
				return fmt.Errorf("tail app logs: %w", err)
			}
			var printable []api.AppLog
			printable, cursor, seenAtCursor = appLogsAfterTailCursor(logs, cursor, seenAtCursor)
			if err := writeAppLogs(ctx, printable); err != nil {
				return err
			}
			if cursor > 0 {
				since = formatAppLogSince(cursor)
			}
		}
	}
}

func rootFlagsFromContext(ctx context.Context, flags *RootFlags) *RootFlags {
	if flags != nil {
		return flags
	}
	if fromCtx, ok := ctx.Value(rootFlagsKey{}).(*RootFlags); ok {
		return fromCtx
	}
	return &RootFlags{}
}

func resolveAppsLogsAppKey(ctx context.Context, flags *RootFlags, site string, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return "", fmt.Errorf("app required")
	}

	project, err := resolveProjectContext()
	if err != nil {
		return "", err
	}
	configuredApps := apps.VisibleApps(project.ProjectRoot, project.Config, currentAPIHost(flags), site)
	app, err := apps.ResolveApp(configuredApps, requested)
	if err == nil && strings.TrimSpace(app.ID) != "" {
		return strings.TrimSpace(app.ID), nil
	}
	return requested, nil
}

func effectiveAppsLogsLimit(limit int) int {
	if limit <= 0 {
		return appsLogsDefaultLimit
	}
	return limit
}

func normalizeAppsLogsSince(raw string, now time.Time) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	if duration, err := time.ParseDuration(value); err == nil {
		if duration <= 0 {
			return "", fmt.Errorf("invalid since duration %q", raw)
		}
		return formatAppLogEpoch(now.Add(-duration)), nil
	}
	return value, nil
}

func writeAppLogs(ctx context.Context, logs []api.AppLog) error {
	for _, log := range logs {
		if err := writeAppLog(ctx, log); err != nil {
			return err
		}
	}
	return nil
}

func writeAppLog(ctx context.Context, log api.AppLog) error {
	if output.FromContext(ctx).JSON {
		data, err := json.Marshal(log)
		if err != nil {
			return fmt.Errorf("encode app log: %w", err)
		}
		_, err = output.Fprintf(ctx, "%s\n", data)
		return err
	}

	timestamp := appLogTimestamp(log).Format("2006-01-02 15:04:05.000")
	level := strings.ToUpper(strings.TrimSpace(log.Level))
	paddedLevel := fmt.Sprintf("%-5s", level)
	paddedLevel = colorAppLogLevel(paddedLevel, level, output.WriterFromContext(ctx).UseColor())
	message := renderAppLogData(log.Data)
	_, err := output.Fprintf(ctx, "%s %s %s\n", timestamp, paddedLevel, message)
	return err
}

func renderAppLogData(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}

	var text string
	if err := json.Unmarshal(trimmed, &text); err == nil {
		return text
	}

	var compact bytes.Buffer
	if err := json.Compact(&compact, trimmed); err == nil {
		return compact.String()
	}
	return string(trimmed)
}

func appLogTimestamp(log api.AppLog) time.Time {
	if !log.CreatedAt.IsZero() {
		return log.CreatedAt.Local()
	}
	if log.Time != 0 {
		seconds, fraction := math.Modf(log.Time)
		return time.Unix(int64(seconds), int64(fraction*1e9)).Local()
	}
	return time.Time{}
}

func newestAppLogTime(logs []api.AppLog) float64 {
	var newest float64
	for _, log := range logs {
		if logTime := appLogCursorTime(log); logTime > newest {
			newest = logTime
		}
	}
	return newest
}

func appLogsAfterTailCursor(logs []api.AppLog, cursor float64, seenAtCursor map[string]struct{}) ([]api.AppLog, float64, map[string]struct{}) {
	newest := cursor
	printable := make([]api.AppLog, 0, len(logs))
	for _, log := range logs {
		logTime := appLogCursorTime(log)
		if logTime > newest {
			newest = logTime
		}
		id := strings.TrimSpace(log.ID)
		if cursor > 0 && logTime == cursor && id != "" {
			if _, seen := seenAtCursor[id]; seen {
				continue
			}
		}
		printable = append(printable, log)
	}

	nextSeenAtCursor := make(map[string]struct{})
	if newest == cursor {
		for id := range seenAtCursor {
			nextSeenAtCursor[id] = struct{}{}
		}
	}
	for _, log := range logs {
		id := strings.TrimSpace(log.ID)
		if id != "" && newest > 0 && appLogCursorTime(log) == newest {
			nextSeenAtCursor[id] = struct{}{}
		}
	}
	return printable, newest, nextSeenAtCursor
}

func appLogIDsAtTime(logs []api.AppLog, target float64) map[string]struct{} {
	ids := make(map[string]struct{})
	if target <= 0 {
		return ids
	}
	for _, log := range logs {
		id := strings.TrimSpace(log.ID)
		if id != "" && appLogCursorTime(log) == target {
			ids[id] = struct{}{}
		}
	}
	return ids
}

func appLogCursorTime(log api.AppLog) float64 {
	if log.Time != 0 {
		return log.Time
	}
	if !log.CreatedAt.IsZero() {
		return float64(log.CreatedAt.UnixNano()) / 1e9
	}
	return 0
}

func reverseAppLogs(logs []api.AppLog) {
	for i, j := 0, len(logs)-1; i < j; i, j = i+1, j-1 {
		logs[i], logs[j] = logs[j], logs[i]
	}
}

func formatAppLogEpoch(t time.Time) string {
	return strconv.FormatFloat(float64(t.UnixNano())/1e9, 'f', 3, 64)
}

func formatAppLogSince(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func colorAppLogLevel(paddedLevel, level string, useColor bool) string {
	if !useColor {
		return paddedLevel
	}
	switch level {
	case "ERROR", "FATAL":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#ef4444")).Bold(true).Render(paddedLevel)
	case "WARN":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#f59e0b")).Bold(true).Render(paddedLevel)
	default:
		return paddedLevel
	}
}

func contextCanceled(ctx context.Context, err error) bool {
	return ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
