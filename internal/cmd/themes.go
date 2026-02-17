package cmd

// ThemesCmd manages themes.
type ThemesCmd struct {
	List      ThemesListCmd     `cmd:"" help:"List themes"`
	Get       ThemesGetCmd      `cmd:"" help:"Get theme details"`
	Layouts   ThemeLayoutsCmd   `cmd:"" help:"Manage layouts"`
	Templates ThemeTemplatesCmd `cmd:"" help:"Manage templates"`
	Snippets  ThemeSnippetsCmd  `cmd:"" help:"Manage snippets"`
	Assets    ThemeAssetsCmd    `cmd:"" help:"Manage assets"`
	Files     ThemeFilesCmd     `cmd:"" help:"Manage theme files (undocumented in API overview)"`
}
