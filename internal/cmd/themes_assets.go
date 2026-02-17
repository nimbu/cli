package cmd

// ThemeAssetsCmd manages theme assets.
type ThemeAssetsCmd struct {
	List   ThemeAssetsListCmd   `cmd:"" help:"List assets"`
	Get    ThemeAssetsGetCmd    `cmd:"" help:"Get an asset"`
	Create ThemeAssetsCreateCmd `cmd:"" help:"Create or update an asset"`
	Delete ThemeAssetsDeleteCmd `cmd:"" help:"Delete an asset"`
}
