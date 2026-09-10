package cmd

// RolesCmd manages roles.
type RolesCmd struct {
	List      RolesListCmd      `cmd:"" help:"List roles"`
	Get       RolesGetCmd       `cmd:"" help:"Get role details"`
	Create    RolesCreateCmd    `cmd:"" help:"Create role from JSON"`
	Update    RolesUpdateCmd    `cmd:"" help:"Update a role. customers/children/parents arrays replace the whole relation; __op envelopes are passed through verbatim"`
	Delete    RolesDeleteCmd    `cmd:"" help:"Delete role"`
	Copy      RolesCopyCmd      `cmd:"" help:"Copy roles between sites"`
	Customers RolesCustomersCmd `cmd:"" help:"Add, remove, or replace customers on a role"`
}
