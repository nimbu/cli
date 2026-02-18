package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// AuthScopesCmd shows active scopes for the current token.
type AuthScopesCmd struct{}

type scopeStatus string

const (
	scopeStatusKnown   scopeStatus = "known"
	scopeStatusUnknown scopeStatus = "unknown"
)

type scopeSnapshot struct {
	Status        scopeStatus `json:"status"`
	Scopes        []string    `json:"scopes"`
	Source        string      `json:"source"`
	CheckedAt     time.Time   `json:"checked_at"`
	UnknownReason string      `json:"unknown_reason,omitempty"`
}

// Run executes the scopes command.
func (c *AuthScopesCmd) Run(ctx context.Context, flags *RootFlags) error {
	client, err := GetAPIClient(ctx)
	if err != nil {
		return err
	}

	snapshot, err := fetchScopeSnapshot(ctx, client)
	if err != nil {
		return err
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, snapshot)
	}

	if mode.Plain {
		scopes := strings.Join(snapshot.Scopes, ",")
		return output.Plain(ctx, snapshot.Status, scopes, snapshot.Source, snapshot.UnknownReason)
	}

	fmt.Printf("Scope status: %s\n", snapshot.Status)
	fmt.Printf("Source: %s\n", snapshot.Source)
	fmt.Printf("Checked at: %s\n", snapshot.CheckedAt.UTC().Format(time.RFC3339))
	if snapshot.Status == scopeStatusKnown {
		fmt.Printf("Scopes: %s\n", strings.Join(snapshot.Scopes, ", "))
		return nil
	}
	fmt.Printf("Reason: %s\n", snapshot.UnknownReason)
	fmt.Println("No scope claims can be made.")
	return nil
}

func fetchScopeSnapshot(ctx context.Context, client *api.Client) (scopeSnapshot, error) {
	resp, err := client.RawRequest(ctx, http.MethodGet, "/user", nil)
	if err != nil {
		return scopeSnapshot{}, fmt.Errorf("fetch scopes: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	snapshot := scopeSnapshot{
		Status:    scopeStatusUnknown,
		Scopes:    []string{},
		Source:    "none",
		CheckedAt: time.Now().UTC(),
	}

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return snapshot, fmt.Errorf("fetch scopes: HTTP %d: %s", resp.StatusCode, string(body))
	}

	scopes := parseScopesHeader(resp.Header.Get("X-OAuth-Scopes"))
	if len(scopes) == 0 {
		snapshot.UnknownReason = "missing_x_oauth_scopes_header"
		return snapshot, nil
	}

	snapshot.Status = scopeStatusKnown
	snapshot.Scopes = scopes
	snapshot.Source = "x-oauth-scopes"
	return snapshot, nil
}

func parseScopesHeader(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}

	parts := strings.Split(raw, ",")
	seen := make(map[string]struct{}, len(parts))
	scopes := make([]string, 0, len(parts))
	for _, part := range parts {
		for _, token := range strings.Fields(strings.TrimSpace(part)) {
			if token == "" {
				continue
			}
			if _, ok := seen[token]; ok {
				continue
			}
			seen[token] = struct{}{}
			scopes = append(scopes, token)
		}
	}
	sort.Strings(scopes)
	return scopes
}
