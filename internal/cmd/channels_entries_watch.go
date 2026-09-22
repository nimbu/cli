package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/realtime"
)

// ChannelEntriesWatchCmd streams live channel entry events over the realtime socket.
type ChannelEntriesWatchCmd struct {
	Channel  string        `required:"" help:"Channel ID or slug" group:"Query"`
	Where    string        `help:"Filter expression (same syntax as --filters where=...)" group:"Query"`
	Filters  []string      `aliases:"filter" help:"Filter entries by key=value or key.op=value, repeatable" group:"Query"`
	Once     bool          `help:"Exit after the first event"`
	For      time.Duration `name:"for" help:"Stop watching after this duration, e.g. 30s"`
	Host     string        `help:"Override the site host used for the websocket (e.g. localhost:3000 for dev)"`
	Insecure bool          `help:"Skip TLS verification for the websocket (dev only)"`
}

// Run executes the watch command.
func (c *ChannelEntriesWatchCmd) Run(ctx context.Context, flags *RootFlags) error {
	flags = rootFlagsFromContext(ctx, flags)

	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	query, err := c.buildQuery()
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}
	if err := requireScopes(ctx, client, []string{"read_channels"}, "Example: nimbu auth scopes"); err != nil {
		return err
	}

	host := c.resolveSiteHost(ctx, client, flags, site)
	if host == "" {
		return newDetailedError(
			errors.New("could not resolve the site host for the realtime socket; pass --host"),
			errorUsageInvalid, ExitUsage, nil)
	}

	printer := newWatchPrinter(ctx, c.Channel, c.Filters, c.Where)

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	printer.OnStatus(fmt.Sprintf("watching %s on %s (Ctrl-C to stop)", c.Channel, host))

	reason, watchErr := realtime.Watch(ctx, realtime.WatchOptions{
		Identifier: realtime.NewIdentifier(c.Channel, query),
		MintGrant: func(ctx context.Context) (string, error) {
			grant, err := api.MintRealtimeGrant(ctx, client)
			if err != nil {
				return "", err
			}
			return grant.Grant, nil
		},
		Dial: func(ctx context.Context, grant string) (realtime.Conn, error) {
			// realtime.Dial builds its own HTTP client: the API client's
			// request timeout and retry transport would break a long-lived socket.
			return realtime.Dial(ctx, host, grant, realtime.DialOptions{Insecure: c.Insecure})
		},
		Handler: printer,
		Once:    c.Once,
		Timeout: c.For,
	})

	return watchExitError(reason, watchErr)
}

// buildQuery turns the repeatable filter flags into a validated live query.
func (c *ChannelEntriesWatchCmd) buildQuery() (map[string]string, error) {
	query := map[string]string{}
	for _, raw := range c.Filters {
		key, value, err := parseFilter(raw)
		if err != nil {
			return nil, newDetailedError(fmt.Errorf("invalid --filters: %w", err), errorUsageInvalid, ExitUsage, nil)
		}
		query[key] = value
	}

	if where := strings.TrimSpace(c.Where); where != "" {
		if existing, ok := query["where"]; ok && existing != where {
			return nil, newDetailedError(
				errors.New("--where conflicts with --filters where=...; pass only one"),
				errorUsageInvalid, ExitUsage, nil)
		}
		query["where"] = where
	}

	if err := realtime.ValidateQuery(query); err != nil {
		return nil, newDetailedError(err, errorUsageInvalid, ExitUsage, nil)
	}
	return query, nil
}

// resolveSiteHost finds the public host serving the site's websocket endpoint.
func (c *ChannelEntriesWatchCmd) resolveSiteHost(ctx context.Context, client *api.Client, flags *RootFlags, site string) string {
	if host := normalizeWatchHost(c.Host); host != "" {
		return host
	}

	var resolved api.Site
	if err := client.Get(ctx, "/sites/"+url.PathEscape(site), &resolved); err == nil {
		if domain := normalizeWatchHost(resolved.Domain); domain != "" {
			return domain
		}
		if host := siteHostFromAPI(flags.APIURL, resolved.Subdomain); host != "" {
			return host
		}
	}
	return siteHostFromAPI(flags.APIURL, site)
}

// watchExitError maps a watch outcome onto the CLI exit-code contract.
func watchExitError(reason realtime.ExitReason, err error) error {
	switch reason {
	case realtime.ExitInterrupted, realtime.ExitTimeout, realtime.ExitOnce:
		return nil
	}
	if err == nil {
		return nil
	}

	var subErr *realtime.SubscriptionError
	if errors.As(err, &subErr) && subErr.Fatal() {
		return newDetailedError(
			fmt.Errorf("watch: the server rejected this live query (%s)", subErr.Code),
			errorUsageInvalid, ExitUsage, nil)
	}

	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		return MapAPIError(fmt.Errorf("watch: %w", err))
	}

	// Transport failures after the retry budget stay a plain failure (exit 1)
	// rather than the retryable network class.
	return &ExitError{Code: ExitGeneral, Err: fmt.Errorf("watch: %w", err)}
}

// normalizeWatchHost trims a user-supplied host down to scheme + host[:port].
// A ws:// or http:// prefix is kept: realtime.Dial reads it to pick a plaintext
// socket, which is how a local dev server is reached. wss:// and https:// are
// dropped because wss is already the default.
func normalizeWatchHost(raw string) string {
	host := strings.TrimSpace(raw)
	if host == "" {
		return ""
	}
	scheme := ""
	for _, prefix := range []string{"wss://", "ws://", "https://", "http://"} {
		if !strings.HasPrefix(host, prefix) {
			continue
		}
		host = strings.TrimPrefix(host, prefix)
		// wss is the default, so only the plaintext dev schemes are carried on.
		if prefix == "ws://" || prefix == "http://" {
			scheme = prefix
		}
		break
	}
	if idx := strings.IndexAny(host, "/?#"); idx >= 0 {
		host = host[:idx]
	}
	if host == "" {
		return ""
	}
	return scheme + host
}
