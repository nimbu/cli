package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/apps"
	"github.com/nimbu/cli/internal/output"
)

const appsLogsDefaultLimit = 100

var (
	appsLogsNow          = time.Now
	appsLogsPollInterval = 2 * time.Second

	// A tail rides out transient network and server errors with exponential
	// backoff and gives up after this many failed fetches in a row.
	appsLogsRetryBaseDelay       = 2 * time.Second
	appsLogsRetryMaxDelay        = 30 * time.Second
	appsLogsMaxConsecutiveErrors = 6
)

// appLogStop inspects each tailed log after it is printed; done ends the tail
// and err becomes its result.
type appLogStop func(api.AppLog) (done bool, err error)

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
		return c.tail(ctx, client, appKey, opts, nil)
	}

	logs, err := api.ListAppLogs(ctx, client, appKey, opts)
	if err != nil {
		return fmt.Errorf("list app logs: %w", err)
	}
	return writeAppLogs(ctx, logs)
}

// tail prints logs and polls for new ones until interrupted, or until stop
// reports done. Ctrl-C ends the tail through ctx: the root handler cancels it
// on the first signal (exit 0 below) and force-exits on a second one.
func (c *AppsLogsCmd) tail(ctx context.Context, client *api.Client, appKey string, opts api.AppLogOptions, stop appLogStop) error {
	logs, err := listAppLogsForTail(ctx, client, appKey, opts)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("tail app logs: %w", err)
	}
	if opts.Since == "" {
		reverseAppLogs(logs)
	}
	if done, err := writeTailedAppLogs(ctx, logs, stop); done {
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
			logs, err := listAppLogsForTail(ctx, client, appKey, pollOpts)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("tail app logs: %w", err)
			}
			var printable []api.AppLog
			printable, cursor, seenAtCursor = appLogsAfterTailCursor(logs, cursor, seenAtCursor)
			if done, err := writeTailedAppLogs(ctx, printable, stop); done {
				return err
			}
			if cursor > 0 {
				since = formatAppLogSince(cursor)
			}
		}
	}
}

// listAppLogsForTail fetches one page of logs, retrying transient failures
// with backoff so a network blip does not end a long tail. The HTTP transport
// already retries a few times within seconds; this covers longer outages.
//
// Only ctx itself ends the tail quietly: a request that hits the client
// timeout also matches context.DeadlineExceeded, but it is a retryable
// network failure, not an interrupt.
func listAppLogsForTail(ctx context.Context, client *api.Client, appKey string, opts api.AppLogOptions) ([]api.AppLog, error) {
	delay := appsLogsRetryBaseDelay
	for attempt := 1; ; attempt++ {
		logs, err := api.ListAppLogs(ctx, client, appKey, opts)
		if err == nil || ctx.Err() != nil || !transientAppLogsError(err) {
			return logs, err
		}
		if attempt >= appsLogsMaxConsecutiveErrors {
			return nil, fmt.Errorf("giving up after %d consecutive failures: %w", attempt, err)
		}
		_, _ = fmt.Fprintf(output.WriterFromContext(ctx).Err, "warning: fetching logs failed (attempt %d of %d), retrying in %s: %v\n", attempt, appsLogsMaxConsecutiveErrors, delay, err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
		if delay *= 2; delay > appsLogsRetryMaxDelay {
			delay = appsLogsRetryMaxDelay
		}
	}
}

// transientAppLogsError reports network failures, rate limits and 5xx
// responses, the same errors the JSON error envelope marks retryable. The
// client wraps transport failures in an api.Error without a status, which the
// envelope classifier cannot place; judge those by their cause. Log reads are
// always safe to repeat.
func transientAppLogsError(err error) bool {
	var apiErr *api.Error
	if errors.As(err, &apiErr) && apiErr.StatusCode == 0 && apiErr.Err != nil {
		err = apiErr.Err
	}
	return classifyError(err).Retryable
}

func writeTailedAppLogs(ctx context.Context, logs []api.AppLog, stop appLogStop) (bool, error) {
	for _, log := range logs {
		if err := writeAppLog(ctx, log); err != nil {
			return true, err
		}
		if stop == nil {
			continue
		}
		if done, err := stop(log); done {
			return true, err
		}
	}
	return false, nil
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
	paddedLevel = colorAppLogLevel(paddedLevel, level, output.WriterFromContext(ctx))
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

// colorAppLogLevel colors warning and error levels through the shared theme,
// downsampled to stdout's color profile; the level word carries the meaning.
func colorAppLogLevel(paddedLevel, level string, writer *output.Writer) string {
	theme := writer.OutTheme()
	switch level {
	case "ERROR", "FATAL":
		return theme.Error(paddedLevel)
	case "WARN":
		return theme.Warning(paddedLevel)
	default:
		return paddedLevel
	}
}
