package cmd

// WebhooksCmd manages webhooks.
type WebhooksCmd struct {
	List   WebhooksListCmd   `cmd:"" help:"List webhooks"`
	Get    WebhooksGetCmd    `cmd:"" help:"Get webhook details"`
	Delete WebhooksDeleteCmd `cmd:"" help:"Delete a webhook"`
}
