package cmd

// ThemesCmd manages themes.
type ThemesCmd struct {
	List      ThemesListCmd     `cmd:"" help:"List themes"`
	Get       ThemesGetCmd      `cmd:"" help:"Get theme details"`
	Pull      ThemePullCmd      `cmd:"" help:"Download managed remote theme files"`
	Diff      ThemeDiffCmd      `cmd:"" help:"Compare remote liquid theme files to local files"`
	Copy      ThemeCopyCmd      `cmd:"" help:"Copy a theme between sites/themes"`
	Push      ThemePushCmd      `cmd:"" help:"Upload managed local theme files"`
	Sync      ThemeSyncCmd      `cmd:"" help:"Upload and reconcile managed local theme files"`
	Layouts   ThemeLayoutsCmd   `cmd:"" help:"Manage layouts"`
	Templates ThemeTemplatesCmd `cmd:"" help:"Manage templates"`
	Snippets  ThemeSnippetsCmd  `cmd:"" help:"Manage snippets"`
	Assets    ThemeAssetsCmd    `cmd:"" help:"Manage assets"`
	Files     ThemeFilesCmd     `cmd:"" help:"Manage theme files (undocumented in API overview)"`
}
