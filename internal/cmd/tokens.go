package cmd

// TokensCmd manages API tokens.
type TokensCmd struct {
	List   TokensListCmd   `cmd:"" help:"List API tokens"`
	Create TokensCreateCmd `cmd:"" help:"Create a new API token"`
	Revoke TokensRevokeCmd `cmd:"" help:"Revoke an API token"`
}
