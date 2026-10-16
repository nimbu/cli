package cmd

// ThemeLayoutsCmd manages theme layouts.
type ThemeLayoutsCmd struct {
	List   ThemeLayoutsListCmd   `cmd:"" help:"List layouts"`
	Get    ThemeLayoutsGetCmd    `cmd:"" help:"Get a layout"`
	Create ThemeLayoutsCreateCmd `cmd:"" help:"Create or update a layout (--force overrides conflicts)"`
	Delete ThemeLayoutsDeleteCmd `cmd:"" help:"Delete a layout (requires --force)"`
}
