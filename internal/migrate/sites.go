package migrate

import (
	"context"
	"fmt"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/themes"
)

// SiteCopyOptions controls site copy orchestration.
type SiteCopyOptions struct {
	AllowErrors      bool
	ConflictResolver ExistingContentResolver
	CopyCustomers    bool
	DryRun           bool
	Include          []string
	Only             []string
	Recursive        bool
	Upsert           string
	Force            bool
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
	conflicts := newExistingContentDecider(opts.Force || opts.DryRun, opts.ConflictResolver)

	emitStageStart(ctx, "Channels")
	channelExistingAction := ExistingContentUpdate
	channelSourceCount, channelExistingCount, err := countExistingChannels(ctx, fromClient, toClient)
	if err != nil {
		return result, err
	}
	if channelExistingCount > 0 {
		channelExistingAction, err = conflicts.decide(ctx, ExistingContentPrompt{Type: "Channels", Source: fromRef.Site, Target: toRef.Site, SourceCount: channelSourceCount, ExistingCount: channelExistingCount})
		if err != nil {
			return result, err
		}
	}
	channelsResult, err := CopyAllChannelsWithOptions(ctx, fromClient, toClient, fromRef, toRef, ChannelCopyOptions{
		DryRun:          opts.DryRun,
		Existing:        channelExistingAction,
		ResolveExisting: opts.ConflictResolver,
	})
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
		emitStageWarning(ctx, "Uploads", w)
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
	customerConfigAction, err := customizationExistingAction(ctx, conflicts, CustomizationService{Kind: CustomizationCustomers}, toClient, fromRef, toRef, "Customer Config")
	if err != nil {
		return result, err
	}
	customerConfig, err := CopyCustomizationsWithOptions(ctx, CustomizationService{Kind: CustomizationCustomers}, fromClient, toClient, fromRef, toRef, CustomizationCopyOptions{
		DryRun:   opts.DryRun,
		Existing: customerConfigAction,
		Stage:    "Customer Config",
	})
	if err != nil {
		return result, err
	}
	result.CustomerConfig = customerConfig
	emitStageDone(ctx, "Customer Config", customizationCopySummary(customerConfig))

	emitStageStart(ctx, "Product Config")
	productConfigAction, err := customizationExistingAction(ctx, conflicts, CustomizationService{Kind: CustomizationProducts}, toClient, fromRef, toRef, "Product Config")
	if err != nil {
		return result, err
	}
	productConfig, err := CopyCustomizationsWithOptions(ctx, CustomizationService{Kind: CustomizationProducts}, fromClient, toClient, fromRef, toRef, CustomizationCopyOptions{
		DryRun:   opts.DryRun,
		Existing: productConfigAction,
		Stage:    "Product Config",
	})
	if err != nil {
		return result, err
	}
	result.ProductConfig = productConfig
	emitStageDone(ctx, "Product Config", customizationCopySummary(productConfig))

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
	} else if themeResult, err := themes.RunCopy(ctx, fromClient, themes.CopyRef{BaseURL: fromRef.BaseURL, Site: fromRef.Site, Theme: sourceTheme}, toClient, themes.CopyRef{BaseURL: toRef.BaseURL, Site: toRef.Site, Theme: targetTheme}, themes.CopyOptions{DryRun: opts.DryRun, Force: opts.Force, ContinueOnError: true}); err != nil {
		msg := fmt.Sprintf("%v", err)
		result.Warnings = append(result.Warnings, msg)
		emitStageSkip(ctx, "Theme", msg)
	} else {
		result.Theme = themeResult
		result.Warnings = append(result.Warnings, themeResult.Warnings...)
		emitStageDone(ctx, "Theme", fmt.Sprintf("%d copied, %d skipped", len(themeResult.Items), len(themeResult.Skipped)))
	}

	emitStageStart(ctx, "Pages")
	pagesResult, err := CopyPages(ctx, fromClient, toClient, fromRef, toRef, "*", media, opts.DryRun)
	if err != nil {
		return result, err
	}
	result.Pages = pagesResult
	emitStageDone(ctx, "Pages", fmt.Sprintf("%d synced", len(pagesResult.Items)))
	for _, w := range pagesResult.Warnings {
		emitStageWarning(ctx, "Pages", w)
	}
	result.Warnings = append(result.Warnings, pagesResult.Warnings...)

	emitStageStart(ctx, "Menus")
	menuExistingAction := ExistingContentUpdate
	menuSourceCount, menuExistingCount, err := countExistingMenus(ctx, fromClient, toClient, "*")
	if err != nil {
		return result, err
	}
	if menuExistingCount > 0 {
		menuExistingAction, err = conflicts.decide(ctx, ExistingContentPrompt{Type: "Menus", Source: fromRef.Site, Target: toRef.Site, SourceCount: menuSourceCount, ExistingCount: menuExistingCount})
		if err != nil {
			return result, err
		}
	}
	menusResult, err := CopyMenusWithOptions(ctx, fromClient, toClient, fromRef, toRef, "*", MenuCopyOptions{
		DryRun:          opts.DryRun,
		Existing:        menuExistingAction,
		Media:           media,
		ResolveExisting: opts.ConflictResolver,
	})
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
		emitStageWarning(ctx, "Translations", w)
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

func countExistingChannels(ctx context.Context, fromClient, toClient *api.Client) (int, int, error) {
	source, err := api.ListChannelDetails(ctx, fromClient)
	if err != nil {
		return 0, 0, err
	}
	target, err := api.ListChannelDetails(ctx, toClient)
	if err != nil {
		return 0, 0, err
	}
	targetBySlug := make(map[string]struct{}, len(target))
	for _, channel := range target {
		if channel.Slug != "" {
			targetBySlug[channel.Slug] = struct{}{}
		}
	}
	existing := 0
	for _, channel := range source {
		if _, ok := targetBySlug[channel.Slug]; ok && channel.Slug != "" {
			existing++
		}
	}
	return len(source), existing, nil
}

func customizationExistingAction(ctx context.Context, conflicts *existingContentDecider, service CustomizationService, toClient *api.Client, fromRef, toRef SiteRef, label string) (ExistingContentAction, error) {
	target, err := service.Load(ctx, toClient)
	if err != nil && !api.IsNotFound(err) {
		return "", err
	}
	if len(target) == 0 {
		return ExistingContentUpdate, nil
	}
	return conflicts.decide(ctx, ExistingContentPrompt{Type: label, Source: fromRef.Site, Target: toRef.Site})
}

func customizationCopySummary(result CustomizationCopyResult) string {
	if result.Action == "skip" {
		return "skipped"
	}
	return fmt.Sprintf("%d fields", result.FieldCount)
}

func countExistingMenus(ctx context.Context, fromClient, toClient *api.Client, query string) (int, int, error) {
	source, err := listMenuDocuments(ctx, fromClient, query)
	if err != nil {
		return 0, 0, err
	}
	target, err := listMenuDocuments(ctx, toClient, query)
	if err != nil {
		return 0, 0, err
	}
	targetBySlug := make(map[string]struct{}, len(target))
	for _, menu := range target {
		if slug := api.MenuDocumentSlug(menu); slug != "" {
			targetBySlug[slug] = struct{}{}
		}
	}
	existing := 0
	for _, menu := range source {
		slug := api.MenuDocumentSlug(menu)
		if _, ok := targetBySlug[slug]; ok && slug != "" {
			existing++
		}
	}
	return len(source), existing, nil
}
