package cmd

// RolesCmd manages roles.
type RolesCmd struct {
	List   RolesListCmd   `cmd:"" help:"List roles"`
	Get    RolesGetCmd    `cmd:"" help:"Get role details"`
	Create RolesCreateCmd `cmd:"" help:"Create role from JSON"`
	Update RolesUpdateCmd `cmd:"" help:"Update role"`
	Delete RolesDeleteCmd `cmd:"" help:"Delete role"`
}
