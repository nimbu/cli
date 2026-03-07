package migrate

import (
	"context"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/themes"
)

// SiteCopyOptions controls site copy orchestration.
type SiteCopyOptions struct {
	AllowErrors   bool
	CopyCustomers bool
	Include       []string
	Only          []string
	Recursive     bool
	Upsert        string
	Force         bool
}

// SiteCopyResult groups stage outputs.
type SiteCopyResult struct {
	From           SiteRef                 `json:"from"`
	To             SiteRef                 `json:"to"`
	Channels       ChannelCopyResult       `json:"channels"`
	ChannelEntries []RecordCopyResult      `json:"channel_entries,omitempty"`
	CustomerConfig CustomizationCopyResult `json:"customer_config"`
	ProductConfig  CustomizationCopyResult `json:"product_config"`
	Theme          themes.CopyResult       `json:"theme"`
	Pages          PageCopyResult          `json:"pages"`
	Menus          MenuCopyResult          `json:"menus"`
	Translations   TranslationCopyResult   `json:"translations"`
}

// CopySite orchestrates a broad site migration.
func CopySite(ctx context.Context, fromClient, toClient *api.Client, fromRef, toRef SiteRef, opts SiteCopyOptions) (SiteCopyResult, error) {
	result := SiteCopyResult{From: fromRef, To: toRef}

	channelsResult, err := CopyAllChannels(ctx, fromClient, toClient, fromRef, toRef)
	if err != nil {
		return result, err
	}
	result.Channels = channelsResult

	recordOpts := RecordCopyOptions{
		AllowErrors:    opts.AllowErrors,
		CopyCustomers:  opts.CopyCustomers,
		Only:           opts.Only,
		PasswordLength: 12,
		Recursive:      opts.Recursive,
		Upsert:         opts.Upsert,
	}
	for _, channel := range opts.Include {
		recordResult, err := CopyChannelEntries(ctx, fromClient, toClient, ChannelRef{SiteRef: fromRef, Channel: channel}, ChannelRef{SiteRef: toRef, Channel: channel}, recordOpts)
		if err != nil {
			return result, err
		}
		result.ChannelEntries = append(result.ChannelEntries, recordResult)
	}

	customerConfig, err := CopyCustomizations(ctx, CustomizationService{Kind: CustomizationCustomers}, fromClient, toClient, fromRef, toRef)
	if err != nil {
		return result, err
	}
	result.CustomerConfig = customerConfig

	productConfig, err := CopyCustomizations(ctx, CustomizationService{Kind: CustomizationProducts}, fromClient, toClient, fromRef, toRef)
	if err != nil {
		return result, err
	}
	result.ProductConfig = productConfig

	sourceTheme, err := activeThemeName(ctx, fromClient)
	if err != nil {
		return result, err
	}
	targetTheme, err := activeThemeName(ctx, toClient)
	if err != nil {
		return result, err
	}
	themeResult, err := themes.RunCopy(ctx, fromClient, themes.CopyRef{BaseURL: fromRef.BaseURL, Site: fromRef.Site, Theme: sourceTheme}, toClient, themes.CopyRef{BaseURL: toRef.BaseURL, Site: toRef.Site, Theme: targetTheme}, themes.CopyOptions{Force: opts.Force})
	if err != nil {
		return result, err
	}
	result.Theme = themeResult

	pagesResult, err := CopyPages(ctx, fromClient, toClient, fromRef, toRef, "*")
	if err != nil {
		return result, err
	}
	result.Pages = pagesResult

	menusResult, err := CopyMenus(ctx, fromClient, toClient, fromRef, toRef, "*", opts.Force)
	if err != nil {
		return result, err
	}
	result.Menus = menusResult

	translationsResult, err := CopyTranslations(ctx, fromClient, toClient, fromRef, toRef, TranslationCopyOptions{Query: "*"})
	if err != nil {
		return result, err
	}
	result.Translations = translationsResult

	return result, nil
}

func activeThemeName(ctx context.Context, client *api.Client) (string, error) {
	items, err := api.List[api.Theme](ctx, client, "/themes")
	if err != nil {
		return "", err
	}
	for _, item := range items {
		if item.Active && item.Name != "" {
			return item.Name, nil
		}
	}
	if len(items) > 0 && items[0].Name != "" {
		return items[0].Name, nil
	}
	return "default-theme", nil
}
