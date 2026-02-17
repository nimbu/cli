package cmd

// CollectionsCmd manages collections.
type CollectionsCmd struct {
	List   CollectionsListCmd   `cmd:"" help:"List collections"`
	Get    CollectionsGetCmd    `cmd:"" help:"Get collection details"`
	Create CollectionsCreateCmd `cmd:"" help:"Create collection from JSON"`
	Update CollectionsUpdateCmd `cmd:"" help:"Update collection"`
	Delete CollectionsDeleteCmd `cmd:"" help:"Delete collection"`
	Count  CollectionsCountCmd  `cmd:"" help:"Count collections"`
}
