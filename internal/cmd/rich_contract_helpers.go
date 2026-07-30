package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/api"
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

func mergeTopLevel(dst map[string]any, src map[string]any) {
	for key, value := range src {
		dstMap, dstIsMap := dst[key].(map[string]any)
		srcMap, srcIsMap := value.(map[string]any)
		if dstIsMap && srcIsMap {
			mergeTopLevel(dstMap, srcMap)
			continue
		}
		dst[key] = value
	}
}

// verifyMenuNesting rejects writes whose response or verification read flattened
// a submitted nested tree.
func verifyMenuNesting(ctx context.Context, client *api.Client, submitted api.MenuDocumentStats, menu api.MenuDocument, opts ...api.RequestOption) error {
	if !submitted.HasItems {
		return nil
	}
	returned := api.MenuStats(menu)
	if api.MenuNestingLost(submitted, returned) || !api.MenuDocumentHasItems(menu) {
		if identifier := api.MenuDocumentSlug(menu); identifier != "" {
			if refetched, err := api.GetMenuDocument(ctx, client, identifier, opts...); err == nil {
				returned = api.MenuStats(refetched)
			}
		}
	}
	if !api.MenuNestingLost(submitted, returned) {
		return nil
	}
	return fmt.Errorf(
		"menu verification failed: server returned a different item tree (items %d/%d, max depth %d/%d)",
		returned.ItemCount, submitted.ItemCount, returned.MaxDepth, submitted.MaxDepth,
	)
}
