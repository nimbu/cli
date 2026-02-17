package cmd

// AppsCodeCmd manages app cloud code files.
type AppsCodeCmd struct {
	List   AppsCodeListCmd   `cmd:"" help:"List app code files"`
	Create AppsCodeCreateCmd `cmd:"" help:"Create app code file from JSON"`
}
