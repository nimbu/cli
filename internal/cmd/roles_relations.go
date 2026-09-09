package cmd

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

var roleRelationFields = []string{"customers", "children", "parents"}

type roleRelationDiff struct {
	Field     string   `json:"field"`
	Before    int      `json:"before"`
	After     int      `json:"after"`
	IDs       []string `json:"ids,omitempty"`
	Projected bool     `json:"projected"`
}

func rolePath(id string) string {
	return "/roles/" + url.PathEscape(id)
}

func getRole(ctx context.Context, client *api.Client, id string) (api.Role, error) {
	var role api.Role
	if err := client.Get(ctx, rolePath(id), &role); err != nil {
		return api.Role{}, fmt.Errorf("get role: %w", err)
	}
	return role, nil
}

func roleRelationValues(role api.Role) map[string][]string {
	return map[string][]string{
		"customers": slices.Clone(role.Customers),
		"children":  slices.Clone(role.Children),
		"parents":   slices.Clone(role.Parents),
	}
}

func roleRelationDiffs(role api.Role, body map[string]any) ([]roleRelationDiff, error) {
	current := roleRelationValues(role)
	var diffs []roleRelationDiff
	for _, field := range roleRelationFields {
		raw, ok := body[field]
		if !ok {
			continue
		}
		next, err := projectRelationIDs(current[field], raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", field, err)
		}
		diffs = append(diffs, roleRelationDiff{
			Field:     field,
			Before:    len(current[field]),
			After:     len(next),
			IDs:       next,
			Projected: true,
		})
	}
	return diffs, nil
}

func projectRelationIDs(current []string, raw any) ([]string, error) {
	if raw == nil {
		return slices.Clone(current), nil
	}

	obj, ok := raw.(map[string]any)
	if !ok {
		return api.ParseRelationIDs(raw), nil
	}

	op, _ := obj["__op"].(string)
	switch strings.ToLower(strings.TrimSpace(op)) {
	case "", "set":
		return api.ParseRelationIDs(obj), nil
	case "addrelation", "addreference":
		return uniqueStrings(append(slices.Clone(current), api.ParseRelationIDs(obj)...)), nil
	case "removerelation", "removereference":
		return subtractIDs(current, api.ParseRelationIDs(obj)), nil
	case "batch":
		next := slices.Clone(current)
		ops, _ := obj["ops"].([]any)
		for i, step := range ops {
			var err error
			next, err = projectRelationIDs(next, step)
			if err != nil {
				return nil, fmt.Errorf("batch op %d: %w", i, err)
			}
		}
		return uniqueStrings(next), nil
	default:
		return nil, fmt.Errorf("unknown relation op %q", op)
	}
}

func subtractIDs(current, remove []string) []string {
	drop := make(map[string]struct{}, len(remove))
	for _, id := range remove {
		drop[id] = struct{}{}
	}
	out := make([]string, 0, len(current))
	for _, id := range current {
		if _, found := drop[id]; !found {
			out = append(out, id)
		}
	}
	return out
}

func relationShrinksOverHalf(before, after int) bool {
	if before <= 0 {
		return false
	}
	lost := before - after
	return lost*2 > before
}

func requireRelationShrinks(flags *RootFlags, roleID string, diffs []roleRelationDiff) error {
	for _, diff := range diffs {
		if !relationShrinksOverHalf(diff.Before, diff.After) {
			continue
		}
		target := fmt.Sprintf("more than half of the %s relation on role %s (%d → %d)", diff.Field, roleID, diff.Before, diff.After)
		if err := requireForce(flags, target); err != nil {
			return err
		}
	}
	return nil
}

func printRoleRelationDiffs(ctx context.Context, diffs []roleRelationDiff) error {
	for _, diff := range diffs {
		if _, err := output.Fprintf(ctx, "%s: %d → %d\n", diff.Field, diff.Before, diff.After); err != nil {
			return err
		}
	}
	return nil
}

func printRoleMemberCounts(ctx context.Context, role api.Role) error {
	_, err := output.Fprintf(ctx, "customers: %d, children: %d, parents: %d\n", len(role.Customers), len(role.Children), len(role.Parents))
	return err
}

func humanOutput(ctx context.Context) bool {
	mode := output.FromContext(ctx)
	return !mode.JSON && !mode.Plain
}
