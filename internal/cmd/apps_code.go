package cmd

// AppsCodeCmd manages app cloud code files.
type AppsCodeCmd struct {
	List   AppsCodeListCmd   `cmd:"" help:"List app code files"`
	Get    AppsCodeGetCmd    `cmd:"" help:"Get an app code file"`
	Create AppsCodeCreateCmd `cmd:"" help:"Create app code file from JSON"`
	Update AppsCodeUpdateCmd `cmd:"" help:"Update an app code file"`
	Delete AppsCodeDeleteCmd `cmd:"" help:"Delete an app code file (requires --force)"`
	Pull   AppsCodePullCmd   `cmd:"" help:"Pull remote app code files"`
}
