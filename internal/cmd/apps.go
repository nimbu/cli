package cmd

// AppsCmd manages OAuth apps.
type AppsCmd struct {
	List   AppsListCmd   `cmd:"" help:"List apps"`
	Get    AppsGetCmd    `cmd:"" help:"Get app details"`
	Config AppsConfigCmd `cmd:"" help:"Add an app to local project config"`
	Push   AppsPushCmd   `cmd:"" help:"Push configured local cloud-code files"`
	Code   AppsCodeCmd   `cmd:"" help:"Manage app cloud code files"`
}
