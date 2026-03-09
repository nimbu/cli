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
func CopyBlogs(ctx context.Context, fromClient, toClient *api.Client, fromRef, toRef SiteRef, query string, media *MediaRewritePlan) (BlogCopyResult, error) {
	result := BlogCopyResult{From: fromRef, To: toRef, Query: query}

	blogs, err := listBlogs(ctx, fromClient, query)
	if err != nil {
		return result, fmt.Errorf("list source blogs: %w", err)
	}

	for _, blog := range blogs {
		handle := blog.Handle
		if handle == "" {
			continue
		}

		blogPayload := map[string]any{
			"name": blog.Name,
			"slug": handle,
		}

		path := "/blogs/" + url.PathEscape(handle)
		var existing api.Blog
		err := toClient.Get(ctx, path, &existing)
		switch {
		case err == nil:
			if err := toClient.Put(ctx, path, blogPayload, &existing); err != nil {
				return result, fmt.Errorf("update blog %s: %w", handle, err)
			}
			result.Items = append(result.Items, BlogCopyItem{Blog: handle, Kind: "blog", Action: "update"})
		case api.IsNotFound(err):
			if err := toClient.Post(ctx, "/blogs", blogPayload, &existing); err != nil {
				return result, fmt.Errorf("create blog %s: %w", handle, err)
			}
			result.Items = append(result.Items, BlogCopyItem{Blog: handle, Kind: "blog", Action: "create"})
		default:
			return result, err
		}

		if err := copyBlogPosts(ctx, fromClient, toClient, handle, media, &result); err != nil {
			return result, err
		}
	}

	return result, nil
}

func copyBlogPosts(ctx context.Context, fromClient, toClient *api.Client, handle string, media *MediaRewritePlan, result *BlogCopyResult) error {
	basePath := "/blogs/" + url.PathEscape(handle) + "/articles"

	srcPosts, err := api.List[api.BlogPost](ctx, fromClient, basePath)
	if err != nil {
		return fmt.Errorf("list posts for blog %s: %w", handle, err)
	}

	dstPosts, err := api.List[api.BlogPost](ctx, toClient, basePath)
	if err != nil {
		return fmt.Errorf("list target posts for blog %s: %w", handle, err)
	}
	targetBySlug := make(map[string]api.BlogPost, len(dstPosts))
	for _, p := range dstPosts {
		if p.Slug != "" {
			targetBySlug[p.Slug] = p
		}
	}

	for _, post := range srcPosts {
		slug := post.Slug
		if slug == "" {
			continue
		}

		content := post.TextContent
		if media != nil {
			content = media.RewriteString("blogs."+handle+"."+slug+".text_content", content)
		}

		payload := map[string]any{
			"title":        post.Title,
			"slug":         slug,
			"text_content": content,
			"status":       post.Status,
		}

		if existing, ok := targetBySlug[slug]; ok {
			postPath := basePath + "/" + url.PathEscape(existing.ID)
			if err := toClient.Put(ctx, postPath, payload, nil); err != nil {
				return fmt.Errorf("update post %s/%s: %w", handle, slug, err)
			}
			result.Items = append(result.Items, BlogCopyItem{Blog: handle, Slug: slug, Kind: "post", Action: "update"})
		} else {
			if err := toClient.Post(ctx, basePath, payload, nil); err != nil {
				return fmt.Errorf("create post %s/%s: %w", handle, slug, err)
			}
			result.Items = append(result.Items, BlogCopyItem{Blog: handle, Slug: slug, Kind: "post", Action: "create"})
		}
	}

	return nil
}

func listBlogs(ctx context.Context, client *api.Client, query string) ([]api.Blog, error) {
	query = strings.TrimSpace(query)
	blogs, err := api.List[api.Blog](ctx, client, "/blogs")
	if err != nil {
		return nil, err
	}
	if query == "" || query == "*" {
		return blogs, nil
	}
	var filtered []api.Blog
	for _, b := range blogs {
		if b.Handle == query {
			filtered = append(filtered, b)
		}
	}
	return filtered, nil
}
