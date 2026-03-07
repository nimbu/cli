package cmd

// ChannelEntriesCmd manages channel entries.
type ChannelEntriesCmd struct {
	List   ChannelEntriesListCmd   `cmd:"" help:"List channel entries"`
	Get    ChannelEntriesGetCmd    `cmd:"" help:"Get entry by ID or slug"`
	Create ChannelEntriesCreateCmd `cmd:"" help:"Create entry from JSON"`
	Update ChannelEntriesUpdateCmd `cmd:"" help:"Update entry"`
	Delete ChannelEntriesDeleteCmd `cmd:"" help:"Delete entry"`
	Count  ChannelEntriesCountCmd  `cmd:"" help:"Count entries"`
	Copy   ChannelEntriesCopyCmd   `cmd:"" help:"Copy channel entries between sites"`
}
