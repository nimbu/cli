package cmd

// ThemeTemplatesCmd manages theme templates.
type ThemeTemplatesCmd struct {
	List   ThemeTemplatesListCmd   `cmd:"" help:"List templates"`
	Get    ThemeTemplatesGetCmd    `cmd:"" help:"Get a template"`
	Create ThemeTemplatesCreateCmd `cmd:"" help:"Create or update a template"`
	Delete ThemeTemplatesDeleteCmd `cmd:"" help:"Delete a template"`
}
