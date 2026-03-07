package cmd

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// PagesGetCmd gets page details.
type PagesGetCmd struct {
	DownloadAssets string `help:"Download page file editables into DIR and rewrite JSON to attachment_path refs"`
	Page           string `arg:"" help:"Page fullpath"`
}

// Run executes the get command.
func (c *PagesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	var opts []api.RequestOption
	if flags.Locale != "" {
		opts = append(opts, api.WithLocale(flags.Locale))
	}

	page, err := api.GetPageDocument(ctx, client, c.Page, opts...)
	if err != nil {
		return fmt.Errorf("get page: %w", err)
	}
	if c.DownloadAssets != "" {
		if _, err := api.DownloadPageAssets(ctx, client, page, c.DownloadAssets); err != nil {
			return fmt.Errorf("download page assets: %w", err)
		}
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, page)
	}

	stats := api.PageStats(page)
	if mode.Plain {
		return output.Plain(
			ctx,
			page["id"],
			api.PageDocumentFullpath(page),
			api.PageDocumentTitle(page),
			api.PageDocumentPublished(page),
		)
	}

	if err := printLine(ctx, "ID:           %v\n", page["id"]); err != nil {
		return err
	}
	if err := printLine(ctx, "Fullpath:     %s\n", api.PageDocumentFullpath(page)); err != nil {
		return err
	}
	if parent := api.PageDocumentParentPath(page); parent != "" {
		if err := printLine(ctx, "Parent path:  %s\n", parent); err != nil {
			return err
		}
	}
	if err := printLine(ctx, "Title:        %s\n", api.PageDocumentTitle(page)); err != nil {
		return err
	}
	if template := api.PageDocumentTemplate(page); template != "" {
		if err := printLine(ctx, "Template:     %s\n", template); err != nil {
			return err
		}
	}
	if err := printLine(ctx, "Published:    %v\n", api.PageDocumentPublished(page)); err != nil {
		return err
	}
	if locale := api.PageDocumentLocale(page); locale != "" {
		if err := printLine(ctx, "Locale:       %s\n", locale); err != nil {
			return err
		}
	}
	if err := printLine(ctx, "Editables:    %d\n", stats.EditableCount); err != nil {
		return err
	}
	if err := printLine(ctx, "Attachments:  %d\n", stats.AttachmentCount); err != nil {
		return err
	}
	if c.DownloadAssets != "" {
		if err := printLine(ctx, "Assets dir:   %s\n", c.DownloadAssets); err != nil {
			return err
		}
	}
	return nil
}
