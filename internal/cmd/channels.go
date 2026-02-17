package cmd

// ChannelsCmd manages channels.
type ChannelsCmd struct {
	List    ChannelsListCmd   `cmd:"" help:"List channels"`
	Get     ChannelsGetCmd    `cmd:"" help:"Get channel details"`
	Entries ChannelEntriesCmd `cmd:"" help:"Manage channel entries"`
}
