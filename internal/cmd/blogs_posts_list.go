package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// BlogPostsListCmd lists blog articles.
type BlogPostsListCmd struct {
	QueryFlags `embed:""`
	Blog       string `required:"" help:"Blog ID or handle"`
	All        bool   `help:"Fetch all pages"`
	Page       int    `help:"Page number" default:"1"`
	PerPage    int    `help:"Items per page" default:"25"`
}

// Run executes the list command.
func (c *BlogPostsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	path := "/blogs/" + url.PathEscape(c.Blog) + "/articles"
	opts, err := localizedContentListRequestOptions(&c.QueryFlags)
	if err != nil {
		return fmt.Errorf("list articles: %w", err)
	}

	var documents []api.Document[api.BlogPost]
	var meta listFooterMeta

	if c.All {
		documents, err = api.List[api.Document[api.BlogPost]](ctx, client, path, opts...)
		if err != nil {
			return fmt.Errorf("list articles: %w", err)
		}
		meta = allListFooterMeta(len(documents))
	} else {
		paged, err := api.ListPage[api.Document[api.BlogPost]](ctx, client, path, c.Page, c.PerPage, opts...)
		if err != nil {
			return fmt.Errorf("list articles: %w", err)
		}
		documents = paged.Data
		meta = newListFooterMeta(c.Page, c.PerPage, paged.Pagination, paged.Links, len(documents))
		meta.probeTotal(ctx, client, "/blogs/"+url.PathEscape(c.Blog)+"/articles/count", opts)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, documents)
	}
	posts, err := localizedDocumentMaps(documents, c.Locale)
	if err != nil {
		return fmt.Errorf("project article locale: %w", err)
	}

	plainFields := []string{"id", "slug", "title"}
	tableFields := []string{"id", "slug", "title", "status"}
	tableHeaders := []string{"ID", "SLUG", "TITLE", "STATUS"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, posts, listOutputFields(&c.QueryFlags, plainFields))
	}

	fields, headers := listOutputColumns(&c.QueryFlags, tableFields, tableHeaders)
	if err := output.WriteTable(ctx, posts, fields, headers); err != nil {
		return err
	}
	return writeListFooter(ctx, "articles", meta)
}
