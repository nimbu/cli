package cmd

// PagesCmd manages pages.
type PagesCmd struct {
	List   PagesListCmd   `cmd:"" help:"List pages"`
	Get    PagesGetCmd    `cmd:"" help:"Get page details"`
	Create PagesCreateCmd `cmd:"" help:"Create page from JSON"`
	Update PagesUpdateCmd `cmd:"" help:"Update page"`
	Delete PagesDeleteCmd `cmd:"" help:"Delete page"`
	Count  PagesCountCmd  `cmd:"" help:"Count pages"`
	Copy   PagesCopyCmd   `cmd:"" help:"Copy pages between sites"`
}
