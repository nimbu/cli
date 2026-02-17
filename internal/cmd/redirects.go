package cmd

// RedirectsCmd manages redirects.
type RedirectsCmd struct {
	List   RedirectsListCmd   `cmd:"" help:"List redirects"`
	Get    RedirectsGetCmd    `cmd:"" help:"Get redirect details"`
	Create RedirectsCreateCmd `cmd:"" help:"Create redirect from JSON"`
	Update RedirectsUpdateCmd `cmd:"" help:"Update redirect"`
	Delete RedirectsDeleteCmd `cmd:"" help:"Delete redirect"`
}
