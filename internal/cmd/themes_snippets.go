package cmd

// ThemeSnippetsCmd manages theme snippets.
type ThemeSnippetsCmd struct {
	List   ThemeSnippetsListCmd   `cmd:"" help:"List snippets"`
	Get    ThemeSnippetsGetCmd    `cmd:"" help:"Get a snippet"`
	Create ThemeSnippetsCreateCmd `cmd:"" help:"Create or update a snippet"`
	Delete ThemeSnippetsDeleteCmd `cmd:"" help:"Delete a snippet"`
}
