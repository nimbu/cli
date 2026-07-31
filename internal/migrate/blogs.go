package migrate

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/nimbu/cli/internal/api"
)

// BlogCopyItem describes one copied blog or post.
type BlogCopyItem struct {
	Blog   string `json:"blog"`
	Slug   string `json:"slug,omitempty"`
	Kind   string `json:"kind"`
	Action string `json:"action"`
}

// BlogCopyResult reports blog copy results.
type BlogCopyResult struct {
	From  SiteRef        `json:"from"`
	To    SiteRef        `json:"to"`
	Query string         `json:"query"`
	Items []BlogCopyItem `json:"items,omitempty"`
}

// CopyBlogs copies blogs and their posts between sites.
func CopyBlogs(ctx context.Context, fromClient, toClient *api.Client, fromRef, toRef SiteRef, query string, media *MediaRewritePlan, dryRun bool) (BlogCopyResult, error) {
	result := BlogCopyResult{From: fromRef, To: toRef, Query: query}

	sourceLocales, err := fetchSiteLocaleInfo(ctx, fromClient, fromRef.Site)
	if err != nil {
		return result, fmt.Errorf("get source site locales: %w", err)
	}
	targetLocales, err := fetchSiteLocaleInfo(ctx, toClient, toRef.Site)
	if err != nil {
		return result, fmt.Errorf("get target site locales: %w", err)
	}
	if !sourceLocales.ExplicitDefault || !targetLocales.ExplicitDefault {
		return result, fmt.Errorf("explicit default_locale is required on both sites for blog copy")
	}
	if !containsLocale(sourceLocales.Locales, targetLocales.DefaultLocale) {
		return result, fmt.Errorf(
			"target default locale %q is not available on source site; add it to the source site or align the default locales",
			targetLocales.DefaultLocale,
		)
	}

	blogs, err := listBlogs(ctx, fromClient, query)
	if err != nil {
		return result, fmt.Errorf("list source blogs: %w", err)
	}

	for i, blog := range blogs {
		sourceHandle := blogHandle(blog)
		emitStageItem(ctx, "Blogs", sourceHandle, int64(i+1), int64(len(blogs)))
		if sourceHandle == "" {
			continue
		}
		payload := blogWritePayload(blog)
		promoteTranslationToRoot(payload, sourceLocales.DefaultLocale, targetLocales.DefaultLocale)
		targetHandle := blogHandle(payload)
		if targetHandle == "" {
			continue
		}

		path := "/blogs/" + url.PathEscape(targetHandle)
		var existing map[string]any
		err := toClient.Get(ctx, path, &existing)
		switch {
		case err == nil:
			action := "update"
			if dryRun {
				action = "dry-run:" + action
			} else if err := toClient.Put(ctx, path, payload, &existing); err != nil {
				return result, fmt.Errorf("update blog %s: %w", targetHandle, err)
			}
			result.Items = append(result.Items, BlogCopyItem{Blog: targetHandle, Kind: "blog", Action: action})
		case api.IsNotFound(err):
			action := "create"
			if dryRun {
				action = "dry-run:" + action
			} else if err := toClient.Post(ctx, "/blogs", payload, &existing); err != nil {
				return result, fmt.Errorf("create blog %s: %w", targetHandle, err)
			}
			result.Items = append(result.Items, BlogCopyItem{Blog: targetHandle, Kind: "blog", Action: action})
		default:
			return result, err
		}

		if err := copyBlogPosts(
			ctx,
			fromClient,
			toClient,
			sourceHandle,
			targetHandle,
			sourceLocales.DefaultLocale,
			targetLocales.DefaultLocale,
			media,
			&result,
			dryRun,
		); err != nil {
			return result, err
		}
	}

	return result, nil
}

func copyBlogPosts(
	ctx context.Context,
	fromClient, toClient *api.Client,
	sourceHandle, targetHandle, sourceDefaultLocale, targetDefaultLocale string,
	media *MediaRewritePlan,
	result *BlogCopyResult,
	dryRun bool,
) error {
	sourceBasePath := "/blogs/" + url.PathEscape(sourceHandle) + "/articles"
	targetBasePath := "/blogs/" + url.PathEscape(targetHandle) + "/articles"

	srcPosts, err := api.List[map[string]any](ctx, fromClient, sourceBasePath)
	if err != nil {
		return fmt.Errorf("list posts for blog %s: %w", sourceHandle, err)
	}

	dstPosts, err := api.List[map[string]any](ctx, toClient, targetBasePath)
	if err != nil {
		if !dryRun {
			return fmt.Errorf("list target posts for blog %s: %w", targetHandle, err)
		}
		// In dry-run the blog may not exist on target yet; treat all posts as creates.
		dstPosts = nil
	}
	targetBySlug := make(map[string]map[string]any, len(dstPosts))
	for _, p := range dstPosts {
		if slug := stringValue(p["slug"]); slug != "" {
			targetBySlug[slug] = p
		}
	}

	for i, post := range srcPosts {
		sourceSlug := stringValue(post["slug"])
		emitStageItem(ctx, "Blogs", sourceHandle+"/"+sourceSlug, int64(i+1), int64(len(srcPosts)))
		if sourceSlug == "" {
			continue
		}

		payload := blogPostWritePayload(post)
		promoteTranslationToRoot(payload, sourceDefaultLocale, targetDefaultLocale)
		targetSlug := stringValue(payload["slug"])
		if targetSlug == "" {
			continue
		}
		if media != nil {
			media.RewriteValue("blogs."+sourceHandle+"."+sourceSlug, payload)
		}

		if existing, ok := targetBySlug[targetSlug]; ok {
			action := "update"
			if dryRun {
				action = "dry-run:" + action
			} else {
				postPath := targetBasePath + "/" + url.PathEscape(stringValue(existing["id"]))
				if err := toClient.Put(ctx, postPath, payload, nil); err != nil {
					return fmt.Errorf("update post %s/%s: %w", targetHandle, targetSlug, err)
				}
			}
			result.Items = append(result.Items, BlogCopyItem{Blog: targetHandle, Slug: targetSlug, Kind: "post", Action: action})
		} else {
			action := "create"
			if dryRun {
				action = "dry-run:" + action
			} else {
				if err := toClient.Post(ctx, targetBasePath, payload, nil); err != nil {
					return fmt.Errorf("create post %s/%s: %w", targetHandle, targetSlug, err)
				}
			}
			result.Items = append(result.Items, BlogCopyItem{Blog: targetHandle, Slug: targetSlug, Kind: "post", Action: action})
		}
	}

	return nil
}

func listBlogs(ctx context.Context, client *api.Client, query string) ([]map[string]any, error) {
	query = strings.TrimSpace(query)
	blogs, err := api.List[map[string]any](ctx, client, "/blogs")
	if err != nil {
		return nil, err
	}
	if query == "" || query == "*" {
		return blogs, nil
	}
	var filtered []map[string]any
	for _, b := range blogs {
		if blogHandle(b) == query {
			filtered = append(filtered, b)
		}
	}
	return filtered, nil
}

func blogHandle(blog map[string]any) string {
	if handle := stringValue(blog["handle"]); handle != "" {
		return handle
	}
	return stringValue(blog["slug"])
}

func blogWritePayload(blog map[string]any) map[string]any {
	payload := deepCopyMap(blog)
	stripBlogReadOnlyFields(payload)
	if stringValue(payload["slug"]) == "" {
		payload["slug"] = blogHandle(blog)
	}
	return payload
}

func blogPostWritePayload(post map[string]any) map[string]any {
	payload := deepCopyMap(post)
	stripBlogReadOnlyFields(payload)
	for _, key := range []string{"next_article", "previous_article", "slugified_tags"} {
		delete(payload, key)
	}
	return payload
}

func promoteTranslationToRoot(payload map[string]any, sourceLocale, targetLocale string) {
	if sourceLocale == targetLocale {
		return
	}
	translations, ok := payload["translations"].(map[string]any)
	if !ok {
		return
	}
	localized, ok := translations[targetLocale].(map[string]any)
	if !ok {
		return
	}
	sourceLocalized, _ := translations[sourceLocale].(map[string]any)
	if sourceLocalized == nil {
		sourceLocalized = map[string]any{}
		translations[sourceLocale] = sourceLocalized
	}
	for key, value := range localized {
		if sourceValue, exists := payload[key]; exists {
			sourceLocalized[key] = deepCopyValue(sourceValue)
		}
		payload[key] = deepCopyValue(value)
	}
}

func stripBlogReadOnlyFields(payload map[string]any) {
	stripSystemFields(payload)
	for _, key := range []string{"url", "public_url", "fullpath"} {
		delete(payload, key)
	}
}
