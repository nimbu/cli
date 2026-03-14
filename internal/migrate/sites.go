package migrate

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/themes"
)

// SiteCopyOptions controls site copy orchestration.
type SiteCopyOptions struct {
	AllowErrors   bool
	CopyCustomers bool
	DryRun        bool
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
	DryRun         bool                    `json:"dry_run,omitempty"`
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
	result := SiteCopyResult{From: fromRef, To: toRef, DryRun: opts.DryRun}

	emitStageStart(ctx, "Channels")
	channelsResult, err := CopyAllChannels(ctx, fromClient, toClient, fromRef, toRef, opts.DryRun)
	if err != nil {
		return result, err
	}
	result.Channels = channelsResult
	emitStageDone(ctx, "Channels", fmt.Sprintf("%d synced", len(channelsResult.Items)))

	emitStageStart(ctx, "Uploads")
	uploadsResult, media, err := CopyUploads(ctx, fromClient, toClient, fromRef, toRef, opts.DryRun)
	if err != nil {
		return result, err
	}
	result.Uploads = uploadsResult
	result.Warnings = append(result.Warnings, uploadsResult.Warnings...)
	emitStageDone(ctx, "Uploads", fmt.Sprintf("%d files", len(uploadsResult.Items)))
	for _, w := range uploadsResult.Warnings {
		emitWarning(ctx, w)
	}

	recordOpts := RecordCopyOptions{
		AllowErrors:    opts.AllowErrors,
		CopyCustomers:  opts.CopyCustomers,
		DryRun:         opts.DryRun,
		Media:          media,
		Only:           opts.Only,
		PasswordLength: 12,
		Recursive:      opts.Recursive,
		Upsert:         opts.Upsert,
	}
	emitStageStart(ctx, "Channel Entries")
	if len(opts.Include) == 0 {
		emitStageSkip(ctx, "Channel Entries", "no channels specified")
	} else {
		entryResults, err := CopySiteEntries(ctx, fromClient, toClient, fromRef, toRef, opts.Include, recordOpts)
		if err != nil {
			return result, err
		}
		result.ChannelEntries = entryResults
		emitStageDone(ctx, "Channel Entries", fmt.Sprintf("%d channels", len(entryResults)))
	}

	emitStageStart(ctx, "Customer Config")
	customerConfig, err := CopyCustomizations(ctx, CustomizationService{Kind: CustomizationCustomers}, fromClient, toClient, fromRef, toRef, opts.DryRun)
	if err != nil {
		return result, err
	}
	result.CustomerConfig = customerConfig
	emitStageDone(ctx, "Customer Config", fmt.Sprintf("%d fields", customerConfig.FieldCount))

	emitStageStart(ctx, "Product Config")
	productConfig, err := CopyCustomizations(ctx, CustomizationService{Kind: CustomizationProducts}, fromClient, toClient, fromRef, toRef, opts.DryRun)
	if err != nil {
		return result, err
	}
	result.ProductConfig = productConfig
	emitStageDone(ctx, "Product Config", fmt.Sprintf("%d fields", productConfig.FieldCount))

	emitStageStart(ctx, "Roles")
	rolesResult, err := CopyRoles(ctx, fromClient, toClient, fromRef, toRef, opts.DryRun)
	if err != nil {
		return result, err
	}
	result.Roles = rolesResult
	emitStageDone(ctx, "Roles", fmt.Sprintf("%d synced", len(rolesResult.Items)))

	emitStageStart(ctx, "Products")
	productsResult, productMapping, err := CopyProducts(ctx, fromClient, toClient, fromRef, toRef, ProductCopyOptions{
		AllowErrors: opts.AllowErrors,
		DryRun:      opts.DryRun,
		Media:       media,
	})
	if err != nil {
		return result, err
	}
	result.Products = productsResult
	emitStageDone(ctx, "Products", fmt.Sprintf("%d synced", len(productsResult.Items)))

	emitStageStart(ctx, "Collections")
	collectionsResult, err := CopyCollections(ctx, fromClient, toClient, fromRef, toRef, CollectionCopyOptions{
		AllowErrors:    opts.AllowErrors,
		DryRun:         opts.DryRun,
		Media:          media,
		ProductMapping: productMapping,
	})
	if err != nil {
		return result, err
	}
	result.Collections = collectionsResult
	emitStageDone(ctx, "Collections", fmt.Sprintf("%d synced", len(collectionsResult.Items)))

	emitStageStart(ctx, "Theme")
	sourceTheme, err := activeThemeID(ctx, fromClient)
	if err != nil {
		msg := fmt.Sprintf("resolve source theme: %v", err)
		result.Warnings = append(result.Warnings, msg)
		emitStageSkip(ctx, "Theme", msg)
	} else if targetTheme, err := activeThemeID(ctx, toClient); err != nil {
		msg := fmt.Sprintf("resolve target theme: %v", err)
		result.Warnings = append(result.Warnings, msg)
		emitStageSkip(ctx, "Theme", msg)
	} else if themeResult, err := themes.RunCopy(ctx, fromClient, themes.CopyRef{BaseURL: fromRef.BaseURL, Site: fromRef.Site, Theme: sourceTheme}, toClient, themes.CopyRef{BaseURL: toRef.BaseURL, Site: toRef.Site, Theme: targetTheme}, themes.CopyOptions{DryRun: opts.DryRun, Force: opts.Force}); err != nil {
		msg := fmt.Sprintf("%v", err)
		result.Warnings = append(result.Warnings, msg)
		emitStageSkip(ctx, "Theme", msg)
	} else {
		result.Theme = themeResult
		emitStageDone(ctx, "Theme", fmt.Sprintf("%d assets", len(themeResult.Items)))
	}

	emitStageStart(ctx, "Pages")
	pagesResult, err := CopyPages(ctx, fromClient, toClient, fromRef, toRef, "*", media, opts.DryRun)
	if err != nil {
		return result, err
	}
	result.Pages = pagesResult
	emitStageDone(ctx, "Pages", fmt.Sprintf("%d synced", len(pagesResult.Items)))
	for _, w := range pagesResult.Warnings {
		emitWarning(ctx, w)
	}
	result.Warnings = append(result.Warnings, pagesResult.Warnings...)

	emitStageStart(ctx, "Menus")
	menusResult, err := CopyMenus(ctx, fromClient, toClient, fromRef, toRef, "*", opts.Force, media, opts.DryRun)
	if err != nil {
		return result, err
	}
	result.Menus = menusResult
	emitStageDone(ctx, "Menus", fmt.Sprintf("%d synced", len(menusResult.Items)))

	emitStageStart(ctx, "Blogs")
	blogsResult, err := CopyBlogs(ctx, fromClient, toClient, fromRef, toRef, "*", media, opts.DryRun)
	if err != nil {
		return result, err
	}
	result.Blogs = blogsResult
	emitStageDone(ctx, "Blogs", fmt.Sprintf("%d synced", len(blogsResult.Items)))

	emitStageStart(ctx, "Notifications")
	notificationsResult, err := CopyNotifications(ctx, fromClient, toClient, fromRef, toRef, "*", media, opts.DryRun)
	if err != nil {
		return result, err
	}
	result.Notifications = notificationsResult
	emitStageDone(ctx, "Notifications", fmt.Sprintf("%d synced", len(notificationsResult.Items)))

	emitStageStart(ctx, "Redirects")
	redirectsResult, err := CopyRedirects(ctx, fromClient, toClient, fromRef, toRef, opts.DryRun)
	if err != nil {
		return result, err
	}
	result.Redirects = redirectsResult
	emitStageDone(ctx, "Redirects", fmt.Sprintf("%d synced", len(redirectsResult.Items)))

	emitStageStart(ctx, "Translations")
	translationsResult, err := CopyTranslations(ctx, fromClient, toClient, fromRef, toRef, TranslationCopyOptions{DryRun: opts.DryRun, Query: "*", Media: media})
	if err != nil {
		return result, err
	}
	result.Translations = translationsResult
	emitStageDone(ctx, "Translations", fmt.Sprintf("%d synced", len(translationsResult.Items)))
	for _, w := range media.Warnings() {
		emitWarning(ctx, w)
	}
	result.Warnings = append(result.Warnings, media.Warnings()...)

	return result, nil
}

func activeThemeID(ctx context.Context, client *api.Client) (string, error) {
	items, err := api.List[api.Theme](ctx, client, "/themes")
	if err != nil {
		return "", err
	}
	for _, item := range items {
		if item.Active && item.ID != "" {
			return item.ID, nil
		}
	}
	if len(items) > 0 && items[0].ID != "" {
		return items[0].ID, nil
	}
	return "", fmt.Errorf("no themes found")
}
