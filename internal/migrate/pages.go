package migrate

import (
	"context"
	"fmt"
	"os"
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
	From  SiteRef        `json:"from"`
	To    SiteRef        `json:"to"`
	Query string         `json:"query"`
	Items []PageCopyItem `json:"items,omitempty"`
}

// CopyPages copies pages matching query (`*`, prefix*, exact).
func CopyPages(ctx context.Context, fromClient, toClient *api.Client, fromRef, toRef SiteRef, query string, media *MediaRewritePlan) (PageCopyResult, error) {
	summaries, err := listPageSummaries(ctx, fromClient, query)
	if err != nil {
		return PageCopyResult{From: fromRef, To: toRef, Query: query}, err
	}
	docs := make([]api.PageDocument, 0, len(summaries))
	for _, summary := range summaries {
		doc, err := api.GetPageDocument(ctx, fromClient, summary.Fullpath)
		if err != nil {
			return PageCopyResult{From: fromRef, To: toRef, Query: query}, err
		}
		docs = append(docs, doc)
	}
	sort.SliceStable(docs, func(i, j int) bool {
		left := strings.Count(api.PageDocumentFullpath(docs[i]), "/")
		right := strings.Count(api.PageDocumentFullpath(docs[j]), "/")
		if left == right {
			return api.PageDocumentFullpath(docs[i]) < api.PageDocumentFullpath(docs[j])
		}
		return left < right
	})

	tempDir, err := os.MkdirTemp("", "nimbu-pages-copy-*")
	if err != nil {
		return PageCopyResult{From: fromRef, To: toRef, Query: query}, err
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	result := PageCopyResult{From: fromRef, To: toRef, Query: query}
	for _, doc := range docs {
		fullpath := api.PageDocumentFullpath(doc)
		downloadDir, err := os.MkdirTemp(tempDir, "page-*")
		if err != nil {
			return result, err
		}
		if _, err := api.DownloadPageAssets(ctx, fromClient, doc, downloadDir); err != nil {
			return result, fmt.Errorf("download page assets for %s: %w", fullpath, err)
		}
		if err := api.ExpandPageAttachmentPaths(doc); err != nil {
			return result, fmt.Errorf("prepare page %s: %w", fullpath, err)
		}
		sanitizePageDocument(doc)
		if media != nil {
			media.RewriteValue("pages."+fullpath, doc)
		}

		action := "create"
		_, err = api.GetPageDocument(ctx, toClient, fullpath)
		switch {
		case err == nil:
			action = "update"
			if _, err := api.PatchPageDocument(ctx, toClient, fullpath, doc); err != nil {
				return result, fmt.Errorf("update page %s: %w", fullpath, err)
			}
		case api.IsNotFound(err):
			var created api.PageDocument
			if err := toClient.Post(ctx, "/pages", doc, &created); err != nil {
				return result, fmt.Errorf("create page %s: %w", fullpath, err)
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
