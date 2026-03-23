package cmd

// CustomersCmd manages customers.
type CustomersCmd struct {
	List               CustomersListCmd               `cmd:"" help:"List customers"`
	Get                CustomersGetCmd                `cmd:"" help:"Get customer by ID or email"`
	Create             CustomersCreateCmd             `cmd:"" help:"Create a customer"`
	Update             CustomersUpdateCmd             `cmd:"" help:"Update a customer"`
	Delete             CustomersDeleteCmd             `cmd:"" help:"Delete a customer"`
	Count              CustomersCountCmd              `cmd:"" help:"Count customers"`
	Copy               CustomersCopyCmd               `cmd:"" help:"Copy customers between sites"`
	Fields             CustomersFieldsCmd             `cmd:"" help:"Show customer field schema"`
	Config             CustomersConfigCmd             `cmd:"" help:"Copy or diff customer customizations"`
	ResetPassword      CustomersResetPasswordCmd      `cmd:"reset-password" help:"Send password reset instructions"`
	ResendConfirmation CustomersResendConfirmationCmd `cmd:"resend-confirmation" help:"Resend confirmation instructions"`
}
