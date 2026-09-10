package cmd

// PagesCmd manages pages.
type PagesCmd struct {
	List        PagesListCmd        `cmd:"" help:"List pages"`
	Get         PagesGetCmd         `cmd:"" help:"Get page details"`
	Create      PagesCreateCmd      `cmd:"" help:"Create page from JSON"`
	Update      PagesUpdateCmd      `cmd:"" help:"Update page"`
	Delete      PagesDeleteCmd      `cmd:"" help:"Delete page"`
	Set         PagesSetCmd         `cmd:"" help:"Set one page field or editable by path"`
	Insert      PagesInsertCmd      `cmd:"" help:"Insert a repeatable into a canvas"`
	DeleteBlock PagesDeleteBlockCmd `cmd:"" name:"delete-block" help:"Delete a repeatable block"`
	Move        PagesMoveCmd        `cmd:"" help:"Move a repeatable within its canvas"`
	Batch       PagesBatchCmd       `cmd:"" help:"Apply multiple page operations from a file"`
	Items       PagesItemsCmd       `cmd:"" help:"Get a resolved items subtree"`
	Schema      PagesSchemaCmd      `cmd:"" help:"Show the page template schema"`
	Count       PagesCountCmd       `cmd:"" help:"Count pages"`
	Copy        PagesCopyCmd        `cmd:"" help:"Copy pages between sites"`
	Versions    PageVersionsCmd     `cmd:"" help:"Manage page versions"`
}
