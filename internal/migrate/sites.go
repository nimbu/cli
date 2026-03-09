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
	Uploads        UploadCopyResult        `json:"uploads"`
	Channels       ChannelCopyResult       `json:"channels"`
	ChannelEntries []RecordCopyResult      `json:"channel_entries,omitempty"`
	CustomerConfig CustomizationCopyResult `json:"customer_config"`
	ProductConfig  CustomizationCopyResult `json:"product_config"`
	Roles          RoleCopyResult          `json:"roles"`
	Products       ProductCopyResult       `json:"products"`
	Collections    CollectionCopyResult    `json:"collections"`
	Theme          themes.CopyResult       `json:"theme"`
	Pages          PageCopyResult          `json:"pages"`
	Menus          MenuCopyResult          `json:"menus"`
	Blogs          BlogCopyResult          `json:"blogs"`
	Notifications  NotificationCopyResult  `json:"notifications"`
	Redirects      RedirectCopyResult      `json:"redirects"`
	Translations   TranslationCopyResult   `json:"translations"`
	Warnings       []string                `json:"warnings,omitempty"`
}

// CopySite orchestrates a broad site migration.
func CopySite(ctx context.Context, fromClient, toClient *api.Client, fromRef, toRef SiteRef, opts SiteCopyOptions) (SiteCopyResult, error) {
	result := SiteCopyResult{From: fromRef, To: toRef}

	channelsResult, err := CopyAllChannels(ctx, fromClient, toClient, fromRef, toRef)
	if err != nil {
		return result, err
	}
	result.Channels = channelsResult

	uploadsResult, media, err := CopyUploads(ctx, fromClient, toClient, fromRef, toRef)
	if err != nil {
		return result, err
	}
	result.Uploads = uploadsResult
	result.Warnings = append(result.Warnings, uploadsResult.Warnings...)

	recordOpts := RecordCopyOptions{
		AllowErrors:    opts.AllowErrors,
		CopyCustomers:  opts.CopyCustomers,
		Media:          media,
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

	rolesResult, err := CopyRoles(ctx, fromClient, toClient, fromRef, toRef)
	if err != nil {
		return result, err
	}
	result.Roles = rolesResult

	productsResult, productMapping, err := CopyProducts(ctx, fromClient, toClient, fromRef, toRef, ProductCopyOptions{
		AllowErrors: opts.AllowErrors,
		Media:       media,
	})
	if err != nil {
		return result, err
	}
	result.Products = productsResult

	collectionsResult, err := CopyCollections(ctx, fromClient, toClient, fromRef, toRef, CollectionCopyOptions{
		AllowErrors:    opts.AllowErrors,
		Media:          media,
		ProductMapping: productMapping,
	})
	if err != nil {
		return result, err
	}
	result.Collections = collectionsResult

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

	pagesResult, err := CopyPages(ctx, fromClient, toClient, fromRef, toRef, "*", media)
	if err != nil {
		return result, err
	}
	result.Pages = pagesResult

	menusResult, err := CopyMenus(ctx, fromClient, toClient, fromRef, toRef, "*", opts.Force, media)
	if err != nil {
		return result, err
	}
	result.Menus = menusResult

	blogsResult, err := CopyBlogs(ctx, fromClient, toClient, fromRef, toRef, "*", media)
	if err != nil {
		return result, err
	}
	result.Blogs = blogsResult

	notificationsResult, err := CopyNotifications(ctx, fromClient, toClient, fromRef, toRef, "*", media)
	if err != nil {
		return result, err
	}
	result.Notifications = notificationsResult

	redirectsResult, err := CopyRedirects(ctx, fromClient, toClient, fromRef, toRef)
	if err != nil {
		return result, err
	}
	result.Redirects = redirectsResult

	translationsResult, err := CopyTranslations(ctx, fromClient, toClient, fromRef, toRef, TranslationCopyOptions{Query: "*", Media: media})
	if err != nil {
		return result, err
	}
	result.Translations = translationsResult
	result.Warnings = append(result.Warnings, media.Warnings()...)

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
