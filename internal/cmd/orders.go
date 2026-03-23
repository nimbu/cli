package cmd

// OrdersCmd manages orders.
type OrdersCmd struct {
	List    OrdersListCmd    `cmd:"" help:"List orders"`
	Get     OrdersGetCmd     `cmd:"" help:"Get order by ID or number"`
	Update  OrdersUpdateCmd  `cmd:"" help:"Update order (status changes)"`
	Count   OrdersCountCmd   `cmd:"" help:"Count orders"`
	Pay     OrdersPayCmd     `cmd:"" help:"Record manual payment for an order"`
	Finish  OrdersFinishCmd  `cmd:"" help:"Finish a paid order"`
	Cancel  OrdersCancelCmd  `cmd:"" help:"Cancel a completed order"`
	Reopen  OrdersReopenCmd  `cmd:"" help:"Reopen a done or canceled order"`
	Archive OrdersArchiveCmd `cmd:"" help:"Archive a done or canceled order"`
}
