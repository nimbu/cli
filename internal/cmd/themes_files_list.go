package cmd

import (
	"context"
	"fmt"
	"net/url"
	pathpkg "path"
	"sort"
	"strings"
	"time"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
	"github.com/nimbu/cli/internal/themes"
)

// ThemeFilesListCmd lists theme files.
type ThemeFilesListCmd struct {
	QueryFlags `embed:""`
	Theme      string `arg:"" help:"Theme ID"`
	All        bool   `help:"Fetch all pages"`
	Page       int    `help:"Page number" default:"1"`
	PerPage    int    `help:"Items per page" default:"25"`
}

type themeFileListItem struct {
	Path      string    `json:"path"`
	Type      string    `json:"type"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// Run executes the list command.
func (c *ThemeFilesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	site, err := RequireSite(ctx, "")
	if err != nil {
		return err
	}

	client, err := GetAPIClientWithSite(ctx, site)
	if err != nil {
		return err
	}

	opts, err := listRequestOptions(&c.QueryFlags)
	if err != nil {
		return fmt.Errorf("list theme files: %w", err)
	}

	rows, err := fetchThemeFileListRows(ctx, client, c.Theme, opts)
	if err != nil {
		return fmt.Errorf("list theme files: %w", err)
	}

	if !c.All {
		rows = paginateThemeFileListRows(rows, c.Page, c.PerPage)
	}

	mode := output.FromContext(ctx)
	if mode.JSON {
		return output.JSON(ctx, rows)
	}

	plainFields := []string{"path", "type", "updated_at"}
	tableFields := []string{"path", "type", "updated_at"}
	tableHeaders := []string{"PATH", "TYPE", "UPDATED"}

	if mode.Plain {
		return output.PlainFromSlice(ctx, rows, listOutputFields(&c.QueryFlags, plainFields))
	}

	fields, headers := listOutputColumns(&c.QueryFlags, tableFields, tableHeaders)
	return output.WriteTable(ctx, rows, fields, headers)
}

func fetchThemeFileListRows(ctx context.Context, client *api.Client, theme string, opts []api.RequestOption) ([]themeFileListItem, error) {
	escapedTheme := url.PathEscape(theme)
	assets, err := api.List[api.ThemeResource](ctx, client, fmt.Sprintf("/themes/%s/assets", escapedTheme), opts...)
	if err != nil {
		return nil, err
	}
	layouts, err := api.List[api.ThemeResource](ctx, client, fmt.Sprintf("/themes/%s/layouts", escapedTheme), opts...)
	if err != nil {
		return nil, err
	}
	templates, err := api.List[api.ThemeResource](ctx, client, fmt.Sprintf("/themes/%s/templates", escapedTheme), opts...)
	if err != nil {
		return nil, err
	}
	snippets, err := api.List[api.ThemeResource](ctx, client, fmt.Sprintf("/themes/%s/snippets", escapedTheme), opts...)
	if err != nil {
		return nil, err
	}

	rows := make([]themeFileListItem, 0, len(assets)+len(layouts)+len(templates)+len(snippets))
	rows = append(rows, themeResourcesToFiles(assets, themes.KindAsset)...)
	rows = append(rows, themeResourcesToFiles(layouts, themes.KindLayout)...)
	rows = append(rows, themeResourcesToFiles(templates, themes.KindTemplate)...)
	rows = append(rows, themeResourcesToFiles(snippets, themes.KindSnippet)...)

	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Path == rows[j].Path {
			return rows[i].Type < rows[j].Type
		}
		return rows[i].Path < rows[j].Path
	})

	return rows, nil
}

func themeResourcesToFiles(resources []api.ThemeResource, kind themes.Kind) []themeFileListItem {
	rows := make([]themeFileListItem, 0, len(resources))
	for _, resource := range resources {
		rows = append(rows, themeFileListItem{
			Path:      themeResourcePath(kind, resource),
			Type:      string(kind),
			UpdatedAt: resource.UpdatedAt,
		})
	}
	return rows
}

func themeResourcePath(kind themes.Kind, resource api.ThemeResource) string {
	if kind == themes.KindAsset {
		p := strings.Trim(strings.TrimSpace(resource.Path), "/")
		if p != "" {
			return p
		}
	}

	folder := strings.Trim(strings.TrimSpace(resource.Folder), "/")
	name := strings.TrimSpace(resource.Name)
	if name != "" {
		if folder != "" {
			return themes.DisplayPath(kind, pathpkg.Join(folder, name))
		}
		return themes.DisplayPath(kind, name)
	}
	return themes.DisplayPath(kind, resource.ID)
}

func paginateThemeFileListRows(rows []themeFileListItem, page, perPage int) []themeFileListItem {
	if perPage <= 0 {
		return rows
	}
	if page <= 0 {
		page = 1
	}

	start := (page - 1) * perPage
	if start >= len(rows) {
		return []themeFileListItem{}
	}
	end := start + perPage
	if end > len(rows) {
		end = len(rows)
	}
	return rows[start:end]
}
