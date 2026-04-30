package cmd

import (
	"context"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/api"
)

type completionRegistry struct {
	global map[string]completionKind
	paths  map[string]map[string]completionKind
}

func newCompletionRegistry() completionRegistry {
	registry := completionRegistry{
		global: map[string]completionKind{
			"site": completionKindSite,
		},
		paths: map[string]map[string]completionKind{},
	}

	for _, path := range [][]string{
		{"sites", "copy"},
		{"pages", "copy"},
		{"menus", "copy"},
		{"products", "copy"},
		{"collections", "copy"},
		{"customers", "copy"},
		{"roles", "copy"},
		{"redirects", "copy"},
		{"translations", "copy"},
		{"blogs", "copy"},
		{"notifications", "copy"},
		{"customers", "config", "copy"},
		{"customers", "config", "diff"},
		{"products", "config", "copy"},
		{"products", "config", "diff"},
	} {
		registry.register(path, completionKindSite, "from", "to")
	}
	registry.register([]string{"channels", "copy"}, completionKindChannelRef, "from", "to")
	registry.register([]string{"channels", "entries", "copy"}, completionKindChannelRef, "from", "to")
	registry.register([]string{"themes", "copy"}, completionKindThemeRef, "from", "to")
	for _, path := range [][]string{
		{"channels", "get"},
		{"channels", "info"},
		{"channels", "empty"},
		{"channels", "fields", "list"},
		{"channels", "fields", "add"},
		{"channels", "fields", "update"},
		{"channels", "fields", "delete"},
		{"channels", "fields", "apply"},
		{"channels", "fields", "replace"},
		{"channels", "fields", "diff"},
		{"channels", "entries", "list"},
		{"channels", "entries", "get"},
		{"channels", "entries", "create"},
		{"channels", "entries", "update"},
		{"channels", "entries", "delete"},
		{"channels", "entries", "count"},
	} {
		registry.register(path, completionKindChannel, "channel")
	}
	for _, path := range [][]string{
		{"themes", "get"},
		{"themes", "pull"},
		{"themes", "diff"},
		{"themes", "push"},
		{"themes", "sync"},
		{"themes", "layouts", "list"},
		{"themes", "layouts", "get"},
		{"themes", "layouts", "create"},
		{"themes", "layouts", "delete"},
		{"themes", "templates", "list"},
		{"themes", "templates", "get"},
		{"themes", "templates", "create"},
		{"themes", "templates", "delete"},
		{"themes", "snippets", "list"},
		{"themes", "snippets", "get"},
		{"themes", "snippets", "create"},
		{"themes", "snippets", "delete"},
		{"themes", "assets", "list"},
		{"themes", "assets", "get"},
		{"themes", "assets", "create"},
		{"themes", "assets", "delete"},
		{"themes", "files", "list"},
		{"themes", "files", "get"},
		{"themes", "files", "put"},
		{"themes", "files", "delete"},
	} {
		registry.register(path, completionKindTheme, "theme")
	}

	return registry
}

func (r completionRegistry) register(path []string, kind completionKind, flags ...string) {
	key := strings.Join(path, " ")
	if r.paths[key] == nil {
		r.paths[key] = map[string]completionKind{}
	}
	for _, flag := range flags {
		r.paths[key][flag] = kind
	}
}

func (r completionRegistry) Lookup(path []string, flag string) (completionKind, bool) {
	if kind, ok := r.global[flag]; ok {
		return kind, true
	}
	if flags, ok := r.paths[strings.Join(path, " ")]; ok {
		kind, ok := flags[flag]
		return kind, ok
	}
	return "", false
}

func fetchCompletionItems(ctx context.Context, client *api.Client, kind completionKind, prefix string, currentSite string) ([]completionItem, error) {
	switch kind {
	case completionKindSite:
		return fetchSiteCompletionItems(ctx, client, "")
	case completionKindChannel:
		if strings.TrimSpace(currentSite) == "" {
			return nil, nil
		}
		return fetchChannelCompletionItems(ctx, client.WithSite(currentSite), "")
	case completionKindChannelRef:
		site, hasResourcePrefix := completionRefSite(prefix)
		if !hasResourcePrefix {
			return fetchSiteCompletionItems(ctx, client, "/")
		}
		return fetchChannelCompletionItems(ctx, client.WithSite(site), site)
	case completionKindTheme:
		if strings.TrimSpace(currentSite) == "" {
			return nil, nil
		}
		return fetchThemeCompletionItems(ctx, client.WithSite(currentSite), "")
	case completionKindThemeRef:
		site, hasResourcePrefix := completionRefSite(prefix)
		if !hasResourcePrefix {
			return fetchSiteCompletionItems(ctx, client, "/")
		}
		return fetchThemeCompletionItems(ctx, client.WithSite(site), site)
	default:
		return nil, nil
	}
}

func fetchSiteCompletionItems(ctx context.Context, client *api.Client, suffix string) ([]completionItem, error) {
	sites, err := api.List[api.Site](ctx, client, "/sites")
	if err != nil {
		return nil, err
	}
	items := make([]completionItem, 0, len(sites))
	for _, site := range sites {
		value := site.Subdomain
		if value == "" {
			value = site.ID
		}
		if value == "" {
			continue
		}
		items = append(items, completionItem{
			Value:       value + suffix,
			Description: completionDescription(site.Name, site.ID),
		})
	}
	sortCompletionItems(items)
	return items, nil
}

func fetchChannelCompletionItems(ctx context.Context, client *api.Client, site string) ([]completionItem, error) {
	channels, err := api.List[api.ChannelSummary](ctx, client, "/channels")
	if err != nil {
		return nil, err
	}
	items := make([]completionItem, 0, len(channels))
	for _, channel := range channels {
		value := channel.Slug
		if value == "" {
			value = channel.ID
		}
		if value == "" {
			continue
		}
		if site != "" {
			value = site + "/" + value
		}
		items = append(items, completionItem{
			Value:       value,
			Description: completionDescription(channel.Name, channel.ID),
		})
	}
	sortCompletionItems(items)
	return items, nil
}

func fetchThemeCompletionItems(ctx context.Context, client *api.Client, site string) ([]completionItem, error) {
	themes, err := api.List[api.Theme](ctx, client, "/themes")
	if err != nil {
		return nil, err
	}
	items := make([]completionItem, 0, len(themes))
	for _, theme := range themes {
		value := theme.Short
		if value == "" {
			value = theme.ID
		}
		if value == "" {
			continue
		}
		if site != "" {
			value = site + "/" + value
		}
		items = append(items, completionItem{
			Value:       value,
			Description: completionDescription(theme.Name, theme.ID),
		})
	}
	sortCompletionItems(items)
	return items, nil
}

func completionDescription(name, id string) string {
	name = strings.TrimSpace(name)
	id = strings.TrimSpace(id)
	switch {
	case name != "" && id != "":
		return name + " (" + id + ")"
	case name != "":
		return name
	default:
		return id
	}
}

func sortCompletionItems(items []completionItem) {
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Value < items[j].Value
	})
}
