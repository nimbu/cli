package cmd

// ChannelsCmd manages channels.
type ChannelsCmd struct {
	List    ChannelsListCmd   `cmd:"" help:"List channels"`
	Get     ChannelsGetCmd    `cmd:"" help:"Get channel details"`
	Create  ChannelsCreateCmd `cmd:"" help:"Create a channel from JSON or inline assignments"`
	Info    ChannelsInfoCmd   `cmd:"" help:"Show rich channel info and TypeScript output"`
	Copy    ChannelsCopyCmd   `cmd:"" help:"Copy channel configuration between sites"`
	Diff    ChannelsDiffCmd   `cmd:"" help:"Diff channel configuration between sites"`
	Empty   ChannelsEmptyCmd  `cmd:"" help:"Empty a channel after strict confirmation"`
	Delete  ChannelsDeleteCmd `cmd:"" help:"Delete a channel (requires --force)"`
	Entries ChannelEntriesCmd `cmd:"" help:"Manage channel entries"`
	Fields  ChannelsFieldsCmd `cmd:"" help:"Manage channel fields"`
}
