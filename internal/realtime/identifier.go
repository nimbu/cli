// Package realtime implements the Nimbu realtime protocol: grant-authenticated
// ActionCable sessions, live-query validation and a reconnecting watch loop.
package realtime

import (
	"encoding/json"
	"fmt"
)

// ChannelName is the ActionCable channel serving live queries.
const ChannelName = "RealtimeApiChannel"

// ResourceChannelEntries is the realtime resource for channel entries.
const ResourceChannelEntries = "channel_entries"

// Identifier is the ActionCable subscription identifier payload.
type Identifier struct {
	Channel  string            `json:"channel"`
	Resource string            `json:"resource"`
	ParentID string            `json:"parent_id"`
	Query    map[string]string `json:"query,omitempty"`
}

// NewIdentifier builds a channel-entries identifier for a parent channel.
func NewIdentifier(parentID string, query map[string]string) Identifier {
	if len(query) == 0 {
		query = nil
	}
	return Identifier{
		Channel:  ChannelName,
		Resource: ResourceChannelEntries,
		ParentID: parentID,
		Query:    query,
	}
}

// String returns the canonical JSON encoding used as the ActionCable identifier.
func (i Identifier) String() (string, error) {
	data, err := json.Marshal(i)
	if err != nil {
		return "", fmt.Errorf("realtime: encode identifier: %w", err)
	}
	return string(data), nil
}
