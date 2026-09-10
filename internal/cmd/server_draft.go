package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/devproxy"
)

func enableServerDraftPreviews(ctx context.Context, client *api.Client, pages []string, proxy *devproxy.Server) error {
	if len(pages) == 0 || proxy == nil {
		return nil
	}
	tokens, logs, err := draftPreviewBindings(ctx, client, pages)
	if err != nil {
		return err
	}
	proxy.SetPreview(devproxy.NewPreviewInjector(tokens))
	for _, line := range logs {
		_, _ = fmt.Fprintln(os.Stderr, line)
	}
	return nil
}

func draftPreviewBindings(ctx context.Context, client *api.Client, pages []string) (map[string]string, []string, error) {
	tokens := map[string]string{}
	var logs []string
	for _, page := range pages {
		page = strings.TrimSpace(page)
		if page == "" {
			continue
		}
		doc, err := api.GetPageDocument(ctx, client, page)
		if err != nil {
			return nil, nil, fmt.Errorf("get page %s for --draft: %w", page, err)
		}
		pageID := stringAny(doc["id"])
		if pageID == "" {
			return nil, nil, fmt.Errorf("page %s is missing id", page)
		}
		token, err := client.CreatePageDraftPreviewToken(ctx, pageID)
		if err != nil {
			return nil, nil, draftAPIError(err, api.PageDocumentFullpath(doc))
		}
		fullpath := previewFullpath(api.PageDocumentFullpath(doc), page)
		tokens[fullpath] = token.Token
		for _, alt := range translationFullpaths(doc["translations"]) {
			tokens[alt] = token.Token
		}
		logs = append(logs, fmt.Sprintf("draft preview enabled for %s (token expires in 24h)", fullpath))
	}
	return tokens, logs, nil
}

func previewFullpath(fullpath, fallback string) string {
	if fullpath == "" {
		fullpath = fallback
	}
	fullpath = strings.TrimSpace(fullpath)
	if fullpath == "" {
		return ""
	}
	if !strings.HasPrefix(fullpath, "/") {
		fullpath = "/" + fullpath
	}
	return strings.TrimSuffix(fullpath, "/")
}

func translationFullpaths(raw any) []string {
	var out []string
	switch typed := raw.(type) {
	case map[string]any:
		for _, value := range typed {
			out = append(out, translationFullpath(value)...)
		}
	case []any:
		for _, value := range typed {
			out = append(out, translationFullpath(value)...)
		}
	}
	return out
}

func translationFullpath(raw any) []string {
	object, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	fullpath := stringAny(object["fullpath"])
	if fullpath == "" {
		return nil
	}
	return []string{previewFullpath(fullpath, "")}
}
