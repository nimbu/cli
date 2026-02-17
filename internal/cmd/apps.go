package cmd

// AppsCmd manages OAuth apps.
type AppsCmd struct {
	List AppsListCmd `cmd:"" help:"List apps"`
	Get  AppsGetCmd  `cmd:"" help:"Get app details"`
	Code AppsCodeCmd `cmd:"" help:"Manage app cloud code files"`
}
