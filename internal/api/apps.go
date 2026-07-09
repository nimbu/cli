package api

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"time"
)

// AppLog represents one cloud-code log entry.
type AppLog struct {
	ID        string          `json:"id"`
	Time      float64         `json:"time"`
	Level     string          `json:"level"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
	Context   json.RawMessage `json:"context,omitempty"`
}

// AppLogOptions filters app cloud-code log requests.
type AppLogOptions struct {
	Since  string
	Before string
	Level  string
	Query  string
	Job    string
	Limit  int
}

// ListAppLogs fetches cloud-code logs for an app.
func ListAppLogs(ctx context.Context, client *Client, appKey string, opts AppLogOptions) ([]AppLog, error) {
	query := map[string]string{}
	if value := strings.TrimSpace(opts.Since); value != "" {
		query["since"] = value
	}
	if value := strings.TrimSpace(opts.Before); value != "" {
		query["before"] = value
	}
	if value := strings.TrimSpace(opts.Level); value != "" {
		query["level"] = value
	}
	if value := strings.TrimSpace(opts.Query); value != "" {
		query["q"] = value
	}
	if value := strings.TrimSpace(opts.Job); value != "" {
		query["job"] = value
	}
	if opts.Limit > 0 {
		query["limit"] = itoa(opts.Limit)
	}

	var reqOpts []RequestOption
	if len(query) > 0 {
		reqOpts = append(reqOpts, WithQuery(query))
	}

	var logs []AppLog
	path := "/apps/" + url.PathEscape(appKey) + "/logs"
	if err := client.Get(ctx, path, &logs, reqOpts...); err != nil {
		return nil, err
	}
	return logs, nil
}
