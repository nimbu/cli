package cmd

import (
	"context"
	"fmt"
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

func printLine(ctx context.Context, format string, args ...any) error {
	_, err := fmt.Fprintf(output.WriterFromContext(ctx).Out, format, args...)
	return err
}

func mergeTopLevel(dst map[string]any, src map[string]any) {
	for key, value := range src {
		dst[key] = value
	}
}
