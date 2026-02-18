package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/api"
)

type scopeMissingError struct {
	Required []string
	Sample   string
}

func (e *scopeMissingError) Error() string {
	if e == nil {
		return "missing required scopes"
	}
	if e.Sample != "" {
		return fmt.Sprintf("missing required scope(s): %v. %s", e.Required, e.Sample)
	}
	return fmt.Sprintf("missing required scope(s): %v", e.Required)
}

func requireScopes(ctx context.Context, client *api.Client, required []string, sample string) error {
	if len(required) == 0 {
		return nil
	}

	snapshot, err := fetchScopeSnapshot(ctx, client)
	if err != nil {
		slog.Debug("scope preflight skipped", "reason", err.Error())
		return nil
	}
	if snapshot.Status != scopeStatusKnown {
		return nil
	}

	available := make(map[string]struct{}, len(snapshot.Scopes))
	for _, scope := range snapshot.Scopes {
		available[scope] = struct{}{}
	}

	missing := make([]string, 0, len(required))
	for _, scope := range required {
		if !scopeSatisfied(scope, available) {
			missing = append(missing, scope)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return &scopeMissingError{Required: missing, Sample: sample}
}

func scopeSatisfied(required string, available map[string]struct{}) bool {
	if _, ok := available[required]; ok {
		return true
	}
	if strings.HasPrefix(required, "read_") {
		writeScope := "write_" + strings.TrimPrefix(required, "read_")
		if _, ok := available[writeScope]; ok {
			return true
		}
	}
	return false
}
