package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var localeKeyRE = regexp.MustCompile(`(?i)^[a-z]{2,3}(?:-[a-z0-9]{2,8})*$`)

func readJSONBodyInput(file string, assignments []string) (map[string]any, error) {
	if file != "" && len(assignments) > 0 {
		return nil, fmt.Errorf("use either --file or inline assignments, not both")
	}

	if len(assignments) > 0 {
		return parseInlineAssignments(assignments)
	}

	return readJSONInput(file)
}

func parseInlineAssignments(assignments []string) (map[string]any, error) {
	body := map[string]any{}

	for _, token := range assignments {
		path, value, err := parseInlineAssignment(token)
		if err != nil {
			return nil, err
		}
		if err := setJSONPathValue(body, path, value); err != nil {
			return nil, err
		}
	}

	return body, nil
}

func parseInlineAssignment(token string) (string, any, error) {
	path, op, rhs, err := splitInlineAssignment(token)
	if err != nil {
		return "", nil, err
	}

	switch op {
	case `:=@`:
		value, err := readJSONValueFromFile(strings.TrimSpace(rhs))
		if err != nil {
			return "", nil, err
		}
		return path, value, nil
	case `:=`:
		raw := strings.TrimSpace(rhs)
		if raw == "" {
			return "", nil, fmt.Errorf("invalid assignment %q", token)
		}
		var value any
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return "", nil, fmt.Errorf("parse JSON value for %q: %w", path, err)
		}
		return path, value, nil
	case `=@`:
		data, err := readRawValueFromFile(strings.TrimSpace(rhs))
		if err != nil {
			return "", nil, err
		}
		return path, data, nil
	case `=`:
		return path, rhs, nil
	default:
		return "", nil, fmt.Errorf("invalid assignment %q, expected key=value or key:=json", token)
	}
}

func setJSONPathValue(body map[string]any, path string, value any) error {
	parts := strings.Split(path, ".")
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return fmt.Errorf("invalid path %q", path)
		}
	}

	cursor := body
	for i := 0; i < len(parts)-1; i++ {
		segment := parts[i]
		existing, ok := cursor[segment]
		if !ok {
			next := map[string]any{}
			cursor[segment] = next
			cursor = next
			continue
		}

		next, ok := existing.(map[string]any)
		if !ok {
			return fmt.Errorf("assignment path conflict at %q", strings.Join(parts[:i+1], "."))
		}
		cursor = next
	}

	leaf := parts[len(parts)-1]
	if _, exists := cursor[leaf]; exists {
		return fmt.Errorf("duplicate assignment for %q", path)
	}
	cursor[leaf] = value
	return nil
}

func mergeJSONBodies(base, extra map[string]any) (map[string]any, error) {
	merged := map[string]any{}
	for k, v := range base {
		merged[k] = v
	}

	for k, v := range extra {
		existing, exists := merged[k]
		if !exists {
			merged[k] = v
			continue
		}

		existingMap, existingIsMap := existing.(map[string]any)
		vMap, vIsMap := v.(map[string]any)
		if existingIsMap && vIsMap {
			nested, err := mergeJSONBodies(existingMap, vMap)
			if err != nil {
				return nil, fmt.Errorf("merge %q: %w", k, err)
			}
			merged[k] = nested
			continue
		}

		return nil, fmt.Errorf("conflicting value for %q", k)
	}

	return merged, nil
}

func translationAssignmentsWithLocaleShorthand(assignments []string) ([]string, error) {
	reserved := map[string]struct{}{
		"key":    {},
		"value":  {},
		"values": {},
		"locale": {},
		"url":    {},
	}

	rewritten := make([]string, 0, len(assignments))
	seenLocalePaths := map[string]string{}

	for _, token := range assignments {
		path, _, err := parseInlineAssignment(token)
		if err != nil {
			return nil, err
		}

		rawPath, op, rhs, err := splitInlineAssignment(token)
		if err != nil {
			return nil, err
		}

		if strings.HasPrefix(path, "values.") {
			locale := strings.TrimPrefix(path, "values.")
			normalized := normalizeLocale(locale)
			if normalized == "" || !localeKeyRE.MatchString(normalized) {
				return nil, fmt.Errorf("invalid locale %q in %q", locale, rawPath)
			}
			key := "values." + normalized
			if prior, exists := seenLocalePaths[key]; exists {
				return nil, fmt.Errorf("duplicate locale assignment for %q (%s, %s)", key, prior, rawPath)
			}
			seenLocalePaths[key] = rawPath
			rewritten = append(rewritten, key+op+rhs)
			continue
		}

		if strings.Contains(path, ".") {
			rewritten = append(rewritten, token)
			continue
		}

		if _, isReserved := reserved[strings.ToLower(path)]; isReserved {
			rewritten = append(rewritten, token)
			continue
		}

		normalized := normalizeLocale(path)
		if !localeKeyRE.MatchString(normalized) {
			return nil, fmt.Errorf("invalid locale key %q; use key=<translation.key>, values.<locale>=..., or a locale like nl/en/fr", rawPath)
		}

		localePath := "values." + normalized
		if prior, exists := seenLocalePaths[localePath]; exists {
			return nil, fmt.Errorf("duplicate locale assignment for %q (%s, %s)", localePath, prior, rawPath)
		}
		seenLocalePaths[localePath] = rawPath
		rewritten = append(rewritten, localePath+op+rhs)
	}

	return rewritten, nil
}

func splitInlineAssignment(token string) (path string, op string, rhs string, err error) {
	bestIdx := -1
	bestOp := ""

	for _, candidate := range []string{`:=@`, `:=`, `=@`, `=`} {
		idx := strings.Index(token, candidate)
		if idx < 0 {
			continue
		}
		if bestIdx == -1 || idx < bestIdx || (idx == bestIdx && len(candidate) > len(bestOp)) {
			bestIdx = idx
			bestOp = candidate
		}
	}

	if bestIdx <= 0 {
		return "", "", "", fmt.Errorf("invalid assignment %q, expected key=value or key:=json", token)
	}

	path = strings.TrimSpace(token[:bestIdx])
	rhs = token[bestIdx+len(bestOp):]
	if path == "" || (bestOp != `=` && strings.TrimSpace(rhs) == "") {
		return "", "", "", fmt.Errorf("invalid assignment %q", token)
	}

	return path, bestOp, rhs, nil
}

func normalizeLocale(locale string) string {
	locale = strings.TrimSpace(strings.ToLower(locale))
	locale = strings.ReplaceAll(locale, "_", "-")
	return locale
}

func readRawValueFromFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file %q: %w", path, err)
	}
	if int64(len(data)) > maxJSONInputBytes {
		return "", fmt.Errorf("file %q exceeds %d bytes", path, maxJSONInputBytes)
	}
	return string(data), nil
}

func readJSONValueFromFile(path string) (any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file %q: %w", path, err)
	}
	if int64(len(data)) > maxJSONInputBytes {
		return nil, fmt.Errorf("file %q exceeds %d bytes", path, maxJSONInputBytes)
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("parse JSON file %q: %w", path, err)
	}
	return value, nil
}
