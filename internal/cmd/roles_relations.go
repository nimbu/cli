package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

func isResponseDecodeError(err error) bool {
	var decodeErr *api.ResponseDecodeError
	return errors.As(err, &decodeErr)
}

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
		next, projected, err := projectRelationIDs(current[field], raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", field, err)
		}
		after := len(current[field])
		if projected {
			after = len(next)
		}
		diffs = append(diffs, roleRelationDiff{
			Field:     field,
			Before:    len(current[field]),
			After:     after,
			IDs:       next,
			Projected: projected,
		})
	}
	return diffs, nil
}

func projectRelationIDs(current []string, raw any) ([]string, bool, error) {
	if raw == nil {
		return slices.Clone(current), true, nil
	}

	obj, ok := raw.(map[string]any)
	if !ok {
		return api.ParseRelationIDs(raw), true, nil
	}

	op, _ := obj["__op"].(string)
	switch strings.ToLower(strings.TrimSpace(op)) {
	case "", "set":
		return api.ParseRelationIDs(obj), true, nil
	case "addrelation", "addreference":
		return uniqueStrings(append(slices.Clone(current), api.ParseRelationIDs(obj)...)), true, nil
	case "removerelation", "removereference":
		return subtractIDs(current, api.ParseRelationIDs(obj)), true, nil
	case "batch":
		next := slices.Clone(current)
		ops, _ := obj["ops"].([]any)
		for i, step := range ops {
			ids, projected, err := projectRelationIDs(next, step)
			if err != nil {
				return nil, false, fmt.Errorf("batch op %d: %w", i, err)
			}
			if !projected {
				return nil, false, nil
			}
			next = ids
		}
		return uniqueStrings(next), true, nil
	default:
		return nil, false, nil
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
		if !diff.Projected || !relationShrinksOverHalf(diff.Before, diff.After) {
			continue
		}
		action := fmt.Sprintf("replace more than half of the %s relation on role %s (%d → %d)", diff.Field, roleID, diff.Before, diff.After)
		if flags != nil && !flags.Force {
			return fmt.Errorf("use --force to %s", action)
		}
	}
	return nil
}

func printRoleRelationDiffs(ctx context.Context, diffs []roleRelationDiff) error {
	for _, diff := range diffs {
		line := fmt.Sprintf("%s: %d → %d\n", diff.Field, diff.Before, diff.After)
		if !diff.Projected {
			line = fmt.Sprintf("%s: %d → ? (__op passed through)\n", diff.Field, diff.Before)
		}
		if _, err := output.Fprintf(ctx, "%s", line); err != nil {
			return err
		}
	}
	return nil
}

func printRoleMemberCounts(ctx context.Context, role api.Role) error {
	_, err := output.Fprintf(ctx, "customers: %s, children: %s, parents: %s\n",
		relationMemberCount(role.Customers, role.CustomersExpanded),
		relationMemberCount(role.Children, role.ChildrenExpanded),
		relationMemberCount(role.Parents, role.ParentsExpanded),
	)
	return err
}

func relationMemberCount(ids api.RelationIDs, expanded bool) any {
	if !expanded {
		return "unknown"
	}
	return len(ids)
}

func requireExpandedReplacements(role api.Role, body map[string]any) error {
	expanded := map[string]bool{
		"customers": role.CustomersExpanded,
		"children":  role.ChildrenExpanded,
		"parents":   role.ParentsExpanded,
	}
	for _, field := range roleRelationFields {
		raw, ok := body[field]
		if !ok || !relationPayloadReplaces(raw) {
			continue
		}
		if expanded[field] {
			continue
		}
		id := role.ID
		if id == "" {
			id = role.Name
		}
		return fmt.Errorf("role %s %s relation was not expanded; refusing to replace members", id, field)
	}
	return nil
}

func relationPayloadReplaces(raw any) bool {
	obj, ok := raw.(map[string]any)
	if !ok {
		return true
	}
	op, _ := obj["__op"].(string)
	switch strings.ToLower(strings.TrimSpace(op)) {
	case "", "set":
		return true
	default:
		return false
	}
}

func humanOutput(ctx context.Context) bool {
	mode := output.FromContext(ctx)
	return !mode.JSON && !mode.Plain
}
