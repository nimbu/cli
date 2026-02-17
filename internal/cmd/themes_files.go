package cmd

// ThemeFilesCmd manages theme files.
type ThemeFilesCmd struct {
	List   ThemeFilesListCmd   `cmd:"" help:"List theme files"`
	Get    ThemeFilesGetCmd    `cmd:"" help:"Get/download theme file content"`
	Put    ThemeFilesPutCmd    `cmd:"" help:"Upload/update theme file"`
	Delete ThemeFilesDeleteCmd `cmd:"" help:"Delete theme file"`
}
