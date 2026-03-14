package migrate

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/api"
)

// PageCopyItem describes one copied page.
type PageCopyItem struct {
	Fullpath string `json:"fullpath"`
	Action   string `json:"action"`
}

// PageCopyResult reports page copy results.
type PageCopyResult struct {
	From     SiteRef        `json:"from"`
	To       SiteRef        `json:"to"`
	Query    string         `json:"query"`
	Items    []PageCopyItem `json:"items,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
}

// CopyPages copies pages matching query (`*`, prefix*, exact).
func CopyPages(ctx context.Context, fromClient, toClient *api.Client, fromRef, toRef SiteRef, query string, media *MediaRewritePlan, dryRun bool) (PageCopyResult, error) {
	summaries, err := listPageSummaries(ctx, fromClient, query)
	if err != nil {
		return PageCopyResult{From: fromRef, To: toRef, Query: query}, err
	}
	docs := make([]api.PageDocument, 0, len(summaries))
	for _, summary := range summaries {
		doc, err := api.GetPageDocument(ctx, fromClient, summary.Fullpath, api.WithParam("x-cdn-expires", "600"))
		if err != nil {
			return PageCopyResult{From: fromRef, To: toRef, Query: query}, err
		}
		docs = append(docs, doc)
	}
	docs = topoSortPages(docs)

	copySet := make(map[string]struct{}, len(docs))
	for _, doc := range docs {
		copySet[api.PageDocumentFullpath(doc)] = struct{}{}
	}

	result := PageCopyResult{From: fromRef, To: toRef, Query: query}
	for i, doc := range docs {
		fullpath := api.PageDocumentFullpath(doc)
		parentPath := api.PageDocumentParentPath(doc)
		emitStageItem(ctx, "Pages", fullpath, int64(i+1), int64(len(docs)))

		embedWarnings := embedPageFiles(ctx, fromClient, doc)
		for _, w := range embedWarnings {
			result.Warnings = append(result.Warnings, fmt.Sprintf("page %s: %s", fullpath, w))
		}
		sanitizePageDocument(doc)
		if media != nil {
			media.RewriteValue("pages."+fullpath, doc)
		}

		// Set parent as fullpath string — the API resolves it by path.
		// Topo sort guarantees parents in the copy set were already created.
		if parentPath != "" {
			if _, inCopySet := copySet[parentPath]; !inCopySet {
				_, parentErr := api.GetPageDocument(ctx, toClient, parentPath)
				if parentErr != nil && api.IsNotFound(parentErr) {
					result.Warnings = append(result.Warnings,
						fmt.Sprintf("page %s: parent %q not in copy set and not found on target — skipped", fullpath, parentPath))
					result.Items = append(result.Items, PageCopyItem{Fullpath: fullpath, Action: "skip"})
					continue
				}
			}
			doc["parent"] = parentPath
		}

		action := "create"
		_, err = api.GetPageDocument(ctx, toClient, fullpath)
		switch {
		case err == nil:
			action = "update"
			if dryRun {
				action = "dry-run:" + action
			} else if _, err := api.PatchPageDocument(ctx, toClient, fullpath, doc); err != nil {
				return result, fmt.Errorf("update page %s: %w", fullpath, err)
			}
		case api.IsNotFound(err):
			if dryRun {
				action = "dry-run:" + action
			} else {
				var created api.PageDocument
				if err := toClient.Post(ctx, "/pages", doc, &created); err != nil {
					return result, fmt.Errorf("create page %s: %w", fullpath, err)
				}
			}
		default:
			return result, err
		}
		result.Items = append(result.Items, PageCopyItem{Fullpath: fullpath, Action: action})
	}
	return result, nil
}

func sanitizePageDocument(doc api.PageDocument) {
	delete(doc, "id")
	delete(doc, "created_at")
	delete(doc, "updated_at")
	delete(doc, "creator_id")
	delete(doc, "updater_id")
	delete(doc, "parent")
	delete(doc, "parent_path")
}

// topoSortPages sorts page documents so parents come before children.
// Uses parent_path to build dependency edges and visits parents first.
func topoSortPages(docs []api.PageDocument) []api.PageDocument {
	byFullpath := make(map[string]api.PageDocument, len(docs))
	for _, doc := range docs {
		byFullpath[api.PageDocumentFullpath(doc)] = doc
	}

	fullpaths := make([]string, 0, len(docs))
	for _, doc := range docs {
		fullpaths = append(fullpaths, api.PageDocumentFullpath(doc))
	}
	sort.Strings(fullpaths)

	visited := make(map[string]bool, len(docs))
	ordered := make([]api.PageDocument, 0, len(docs))

	var visit func(string)
	visit = func(fullpath string) {
		if visited[fullpath] {
			return
		}
		visited[fullpath] = true
		doc := byFullpath[fullpath]
		if parentPath := api.PageDocumentParentPath(doc); parentPath != "" {
			if _, inSet := byFullpath[parentPath]; inSet {
				visit(parentPath)
			}
		}
		ordered = append(ordered, doc)
	}

	for _, fp := range fullpaths {
		visit(fp)
	}
	return ordered
}

// embedPageFiles downloads and base64-encodes all file editables in a page document.
func embedPageFiles(ctx context.Context, client *api.Client, doc api.PageDocument) []string {
	var warnings []string
	_ = api.WalkPageEditables(doc, func(name string, editable map[string]any) error {
		file := api.PageEditableFile(editable)
		if file == nil {
			return nil
		}
		if err := embedFileFromClient(ctx, client, file); err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", name, err))
		}
		return nil
	})
	return warnings
}

func listPageSummaries(ctx context.Context, client *api.Client, query string) ([]api.PageSummary, error) {
	var opts []api.RequestOption
	switch {
	case query == "", query == "*":
	case strings.Contains(query, "*"):
		opts = append(opts, api.WithParam("fullpath.start", strings.TrimSuffix(query, "*")))
	default:
		opts = append(opts, api.WithParam("fullpath", strings.TrimPrefix(query, "/")))
	}
	pages, err := api.List[api.PageSummary](ctx, client, "/pages", opts...)
	if err != nil {
		return nil, err
	}
	for idx := range pages {
		pages[idx].Fullpath = api.NormalizePageFullpath(pages[idx].Fullpath)
	}
	return pages, nil
}
