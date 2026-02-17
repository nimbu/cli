package cmd

// WebhooksCmd manages webhooks.
type WebhooksCmd struct {
	List   WebhooksListCmd   `cmd:"" help:"List webhooks"`
	Get    WebhooksGetCmd    `cmd:"" help:"Get webhook details"`
	Create WebhooksCreateCmd `cmd:"" help:"Create a webhook"`
	Update WebhooksUpdateCmd `cmd:"" help:"Update a webhook"`
	Delete WebhooksDeleteCmd `cmd:"" help:"Delete a webhook"`
}
