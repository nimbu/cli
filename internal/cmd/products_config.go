package cmd

// ProductsConfigCmd manages product customizations.
type ProductsConfigCmd struct {
	Copy   ProductsConfigCopyCmd `cmd:"" help:"Copy product customizations between sites"`
	Diff   ProductsConfigDiffCmd `cmd:"" help:"Diff product customizations between sites"`
	Fields ProductsFieldsCmd     `cmd:"" help:"Alias for products fields" hidden:""`
}
