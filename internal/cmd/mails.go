package cmd

// MailsCmd exposes legacy mail workflow aliases.
type MailsCmd struct {
	Pull NotificationsPullCmd `cmd:"" help:"Download notification templates"`
	Push NotificationsPushCmd `cmd:"" help:"Upload notification templates"`
}
