package cmd

// MenusCmd manages navigation menus.
type MenusCmd struct {
	List   MenusListCmd   `cmd:"" help:"List menus"`
	Get    MenusGetCmd    `cmd:"" help:"Get menu details"`
	Create MenusCreateCmd `cmd:"" help:"Create menu from JSON"`
	Update MenusUpdateCmd `cmd:"" help:"Update menu"`
	Delete MenusDeleteCmd `cmd:"" help:"Delete menu"`
	Count  MenusCountCmd  `cmd:"" help:"Count menus"`
	Copy   MenusCopyCmd   `cmd:"" help:"Copy menus between sites"`
}
