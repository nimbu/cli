package cmd

// SendersCmd manages email sender domains.
type SendersCmd struct {
	List            SendersListCmd            `cmd:"" help:"List sender domains"`
	Get             SendersGetCmd             `cmd:"" help:"Get sender domain details"`
	Create          SendersCreateCmd          `cmd:"" help:"Create a sender domain"`
	VerifyOwnership SendersVerifyOwnershipCmd `cmd:"verify-ownership" help:"Verify sender domain ownership"`
	Verify          SendersVerifyCmd          `cmd:"" help:"Verify sender domain DNS"`
}
