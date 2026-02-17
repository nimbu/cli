package cmd

// WebhooksCmd manages webhooks.
type WebhooksCmd struct {
	List   WebhooksListCmd   `cmd:"" help:"List webhooks"`
	Get    WebhooksGetCmd    `cmd:"" help:"Get webhook details"`
	Create WebhooksCreateCmd `cmd:"" help:"Create a webhook (undocumented in API overview)"`
	Update WebhooksUpdateCmd `cmd:"" help:"Update a webhook (legacy/undocumented in API overview)"`
	Delete WebhooksDeleteCmd `cmd:"" help:"Delete a webhook"`
	Count  WebhooksCountCmd  `cmd:"" help:"Count webhooks (undocumented in API overview)"`
}
