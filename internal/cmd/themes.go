package cmd

// ThemesCmd manages themes.
type ThemesCmd struct {
	List  ThemesListCmd `cmd:"" help:"List themes"`
	Get   ThemesGetCmd  `cmd:"" help:"Get theme details"`
	Files ThemeFilesCmd `cmd:"" help:"Manage theme files"`
}
