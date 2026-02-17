package cmd

// CustomersCmd manages customers.
type CustomersCmd struct {
	List   CustomersListCmd   `cmd:"" help:"List customers"`
	Get    CustomersGetCmd    `cmd:"" help:"Get customer by ID or email"`
	Create CustomersCreateCmd `cmd:"" help:"Create a customer"`
	Update CustomersUpdateCmd `cmd:"" help:"Update a customer"`
	Delete CustomersDeleteCmd `cmd:"" help:"Delete a customer"`
}
