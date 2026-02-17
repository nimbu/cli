package cmd

// ProductsCmd manages products.
type ProductsCmd struct {
	List   ProductsListCmd   `cmd:"" help:"List products"`
	Get    ProductsGetCmd    `cmd:"" help:"Get product by ID or slug"`
	Create ProductsCreateCmd `cmd:"" help:"Create a product"`
	Update ProductsUpdateCmd `cmd:"" help:"Update a product"`
	Delete ProductsDeleteCmd `cmd:"" help:"Delete a product"`
	Count  ProductsCountCmd  `cmd:"" help:"Count products"`
}
