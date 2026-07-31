package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/nimbu/cli/internal/api"
)

var localeKeyRE = regexp.MustCompile(`(?i)^[a-z]{2,3}(?:-[a-z0-9]{1,8})*$`)

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
		value, err := decodeJSONAnyUseNumber([]byte(raw))
		if err != nil {
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

// hintJSONAssignments augments a 422 validation error when a plain string
// assignment (=) carries a value that parses as a JSON array or object; the
// user most likely meant the := JSON operator.
func hintJSONAssignments(err error, assignments []string) error {
	if err == nil {
		return err
	}
	var apiErr *api.Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 422 {
		return err
	}
	for _, token := range assignments {
		path, op, rhs, splitErr := splitInlineAssignment(token)
		if splitErr != nil || op != `=` {
			continue
		}
		raw := strings.TrimSpace(rhs)
		if raw == "" || (raw[0] != '[' && raw[0] != '{') {
			continue
		}
		if !json.Valid([]byte(raw)) {
			continue
		}
		return fmt.Errorf("%w (hint: the value of %q looks like JSON but was sent as a string; use %s:=... to send it as JSON)", err, path, path)
	}
	return err
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
		path, parsedValue, err := parseInlineAssignment(token)
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
			if !isValidLocaleKey(normalized) {
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

		if strings.EqualFold(path, "locale") {
			locale, ok := parsedValue.(string)
			if !ok {
				return nil, fmt.Errorf("locale assignment must be a string")
			}
			normalized := normalizeLocale(locale)
			if !isValidLocaleKey(normalized) {
				return nil, fmt.Errorf("invalid locale %q", locale)
			}
			if op == ":=" || op == ":=@" {
				encoded, err := json.Marshal(normalized)
				if err != nil {
					return nil, fmt.Errorf("encode locale: %w", err)
				}
				rewritten = append(rewritten, "locale:="+string(encoded))
			} else {
				rewritten = append(rewritten, "locale="+normalized)
			}
			continue
		}

		if strings.EqualFold(path, "values") && (op == ":=" || op == ":=@") {
			_, value, err := parseInlineAssignment(token)
			if err != nil {
				return nil, err
			}
			values, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("translation values must be a JSON object")
			}
			canonical := make(map[string]any, len(values))
			for locale, translation := range values {
				normalized := normalizeLocale(locale)
				if !isValidLocaleKey(normalized) {
					return nil, fmt.Errorf("invalid locale %q in values", locale)
				}
				localePath := "values." + normalized
				if prior, exists := seenLocalePaths[localePath]; exists {
					return nil, fmt.Errorf("duplicate locale assignment for %q (%s, %s)", localePath, prior, rawPath)
				}
				if _, exists := canonical[normalized]; exists {
					return nil, fmt.Errorf("duplicate locale assignment for %q after canonicalization", localePath)
				}
				seenLocalePaths[localePath] = rawPath
				canonical[normalized] = translation
			}
			encoded, err := json.Marshal(canonical)
			if err != nil {
				return nil, fmt.Errorf("encode translation values: %w", err)
			}
			rewritten = append(rewritten, "values:="+string(encoded))
			continue
		}

		if _, isReserved := reserved[strings.ToLower(path)]; isReserved {
			rewritten = append(rewritten, token)
			continue
		}

		normalized := normalizeLocale(path)
		if !isValidLocaleKey(normalized) {
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
	locale = strings.TrimSpace(locale)
	locale = strings.ReplaceAll(locale, "_", "-")
	parts := strings.Split(locale, "-")
	scriptSeen := false
	regionSeen := false
	extension := false
	for i, part := range parts {
		switch {
		case i == 0:
			parts[i] = strings.ToLower(part)
		case extension:
			parts[i] = strings.ToLower(part)
		case len(part) == 1:
			parts[i] = strings.ToLower(part)
			extension = true
		case !scriptSeen && !regionSeen && len(part) == 4 && isASCIIAlpha(part):
			parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
			scriptSeen = true
		case !regionSeen && len(part) == 2 && isASCIIAlpha(part):
			parts[i] = strings.ToUpper(part)
			regionSeen = true
		case !regionSeen && len(part) == 3 && isASCIIDigits(part):
			parts[i] = part
			regionSeen = true
		default:
			parts[i] = strings.ToLower(part)
		}
	}
	return strings.Join(parts, "-")
}

func isValidLocaleKey(locale string) bool {
	if locale == "" || !localeKeyRE.MatchString(locale) {
		return false
	}

	parts := strings.Split(locale, "-")
	seenSingletons := map[string]struct{}{}
	for index := 1; index < len(parts); index++ {
		part := strings.ToLower(parts[index])
		if len(part) != 1 {
			continue
		}
		if _, duplicate := seenSingletons[part]; duplicate {
			return false
		}
		seenSingletons[part] = struct{}{}
		if index+1 >= len(parts) {
			return false
		}
		if part == "x" {
			return true
		}
		if len(parts[index+1]) < 2 {
			return false
		}
	}
	return true
}

func isASCIIAlpha(value string) bool {
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') {
			return false
		}
	}
	return value != ""
}

func isASCIIDigits(value string) bool {
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return value != ""
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
	value, err := decodeJSONAnyUseNumber(data)
	if err != nil {
		return nil, fmt.Errorf("parse JSON file %q: %w", path, err)
	}
	return value, nil
}
