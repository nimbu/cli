package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/output"
)

func readRichDocumentInput(file string) (map[string]any, error) {
	return readJSONInput(file)
}

func validateShallowInlineAssignments(resource string, assignments []string, allowed map[string]struct{}) error {
	if len(assignments) == 0 {
		return nil
	}

	allowedKeys := make([]string, 0, len(allowed))
	for key := range allowed {
		allowedKeys = append(allowedKeys, key)
	}
	sort.Strings(allowedKeys)

	for _, token := range assignments {
		path, _, _, err := splitInlineAssignment(token)
		if err != nil {
			return err
		}
		if strings.Contains(path, ".") {
			return fmt.Errorf("%s now uses a richer document contract; deep edit %q requires --file or stdin", resource, path)
		}
		if _, ok := allowed[path]; !ok {
			return fmt.Errorf("%s now uses a richer document contract; deep edit %q requires --file or stdin (allowed inline keys: %s)", resource, path, strings.Join(allowedKeys, ", "))
		}
	}

	return nil
}

func writeJSONToWriter(ctx context.Context, value any) error {
	w := output.WriterFromContext(ctx)
	enc := json.NewEncoder(w.Out)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func printLine(ctx context.Context, format string, args ...any) error {
	_, err := fmt.Fprintf(output.WriterFromContext(ctx).Out, format, args...)
	return err
}

func writeIndentedJSON(ctx context.Context, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(output.WriterFromContext(ctx).Out, "%s\n", data)
	return err
}

func mergeTopLevel(dst map[string]any, src map[string]any) {
	for key, value := range src {
		dst[key] = value
	}
}

func maybeRelativePath(base, target string) string {
	if base == "" || target == "" {
		return target
	}
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return target
	}
	return rel
}

func readAll(reader io.Reader) (string, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
