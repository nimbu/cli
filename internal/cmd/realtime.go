package cmd

// RealtimeCmd groups realtime (websocket) helpers.
type RealtimeCmd struct {
	Grant RealtimeGrantCmd `cmd:"" help:"Mint a single-use realtime grant"`
}
