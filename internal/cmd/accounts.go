package cmd

// AccountsCmd manages accounts.
type AccountsCmd struct {
	List  AccountsListCmd  `cmd:"" help:"List accounts"`
	Count AccountsCountCmd `cmd:"" help:"Count accounts"`
}
