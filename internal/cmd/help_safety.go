package cmd

import "reflect"

// confirmationCommands lists the commands that can stop on a confirmation
// (--force/--yes guards, y/N prompts, protected environments). Their default
// help shows --force, --yes and --dry-run under "Safety:".
// TestConfirmationCommandsRegistryMatchesSource keeps this list in step with
// the code and prints the entries to add.
var confirmationCommands = commandTypeSet(
	(*APIDeleteCmd)(nil),
	(*APIGetCmd)(nil),
	(*APILegacyCmd)(nil),
	(*APIPatchCmd)(nil),
	(*APIPostCmd)(nil),
	(*APIPutCmd)(nil),
	(*AnnouncementsDeleteCmd)(nil),
	(*AppsCodeDeleteCmd)(nil),
	(*AppsPushCmd)(nil),
	(*BlogPostsDeleteCmd)(nil),
	(*BlogsDeleteCmd)(nil),
	(*ChannelEntriesDeleteCmd)(nil),
	(*ChannelEntriesGalleryRemoveCmd)(nil),
	(*ChannelsDeleteCmd)(nil),
	(*ChannelsEmptyCmd)(nil),
	(*ChannelsFieldsDeleteCmd)(nil),
	(*ChannelsFieldsReplaceCmd)(nil),
	(*CollectionsDeleteCmd)(nil),
	(*CouponsDeleteCmd)(nil),
	(*CustomersConfigCopyCmd)(nil),
	(*CustomersDeleteCmd)(nil),
	(*DomainsDeleteCmd)(nil),
	(*DomainsMakePrimaryCmd)(nil),
	(*MenusCopyCmd)(nil),
	(*MenusDeleteCmd)(nil),
	(*NotificationsDeleteCmd)(nil),
	(*PagesDeleteBlockCmd)(nil),
	(*PagesDeleteCmd)(nil),
	(*PagesDraftDiscardCmd)(nil),
	(*ProductAttachmentsDeleteCmd)(nil),
	(*ProductAttachmentsDownloadCmd)(nil),
	(*ProductsConfigCopyCmd)(nil),
	(*ProductsDeleteCmd)(nil),
	(*RedirectsDeleteCmd)(nil),
	(*ReleaseCmd)(nil),
	(*RolesCustomersAddCmd)(nil),
	(*RolesCustomersRemoveCmd)(nil),
	(*RolesCustomersSetCmd)(nil),
	(*RolesDeleteCmd)(nil),
	(*RolesUpdateCmd)(nil),
	(*SchemaApplyCmd)(nil),
	(*SettingsConsentConfigReplaceCmd)(nil),
	(*SettingsConsentDeleteCmd)(nil),
	(*ShippingRatesDeleteCmd)(nil),
	(*SitesCopyCmd)(nil),
	(*ThemeAssetsCreateCmd)(nil),
	(*ThemeAssetsDeleteCmd)(nil),
	(*ThemeCopyCmd)(nil),
	(*ThemeFilesDeleteCmd)(nil),
	(*ThemeFilesPutCmd)(nil),
	(*ThemeLayoutsCreateCmd)(nil),
	(*ThemeLayoutsDeleteCmd)(nil),
	(*ThemePushCmd)(nil),
	(*ThemeSnippetsCreateCmd)(nil),
	(*ThemeSnippetsDeleteCmd)(nil),
	(*ThemeSyncCmd)(nil),
	(*ThemeTemplatesCreateCmd)(nil),
	(*ThemeTemplatesDeleteCmd)(nil),
	(*TranslationsDeleteCmd)(nil),
	(*UploadsDeleteCmd)(nil),
	(*UploadsDownloadCmd)(nil),
	(*WebhooksDeleteCmd)(nil),
)

func commandTypeSet(cmds ...any) map[reflect.Type]bool {
	out := make(map[reflect.Type]bool, len(cmds))
	for _, cmd := range cmds {
		out[reflect.TypeOf(cmd).Elem()] = true
	}
	return out
}
