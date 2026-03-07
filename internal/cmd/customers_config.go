package cmd

// CustomersConfigCmd manages customer customizations.
type CustomersConfigCmd struct {
	Copy   CustomersConfigCopyCmd `cmd:"" help:"Copy customer customizations between sites"`
	Diff   CustomersConfigDiffCmd `cmd:"" help:"Diff customer customizations between sites"`
	Fields CustomersFieldsCmd     `cmd:"" help:"Alias for customers fields" hidden:""`
}
