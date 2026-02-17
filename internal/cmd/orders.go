package cmd

// OrdersCmd manages orders.
type OrdersCmd struct {
	List   OrdersListCmd   `cmd:"" help:"List orders"`
	Get    OrdersGetCmd    `cmd:"" help:"Get order by ID or number"`
	Update OrdersUpdateCmd `cmd:"" help:"Update order (status changes)"`
	Count  OrdersCountCmd  `cmd:"" help:"Count orders"`
}
