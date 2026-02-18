package cmd

// AuthCmd handles authentication commands.
type AuthCmd struct {
	Login   AuthLoginCmd   `cmd:"" help:"Log in to Nimbu"`
	Logout  AuthLogoutCmd  `cmd:"" help:"Log out and remove stored credentials"`
	Status  AuthStatusCmd  `cmd:"" help:"Show authentication status"`
	Whoami  AuthWhoamiCmd  `cmd:"" help:"Show current authenticated user"`
	Scopes  AuthScopesCmd  `cmd:"" help:"Show active token scopes"`
	Token   AuthTokenCmd   `cmd:"" help:"Print access token for scripts"`
	Keyring AuthKeyringCmd `cmd:"" help:"Manage keyring backend"`
}
