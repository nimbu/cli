package api

import (
	"context"
	"net/http"
	"time"
)

// RealtimeGrant is a single-use, short-lived credential for the realtime socket.
// The grant value is a secret: never log it or include it in error messages.
type RealtimeGrant struct {
	Grant     string    `json:"grant"`
	ExpiresAt time.Time `json:"expires_at"`
}

// MintRealtimeGrant requests a fresh realtime grant for the client's site.
func MintRealtimeGrant(ctx context.Context, c *Client) (*RealtimeGrant, error) {
	// A grant is a read credential, not a site mutation, so --readonly must
	// not block it.
	c = c.WithReadonly(false)
	var grant RealtimeGrant
	err := c.Request(ctx, http.MethodPost, "/realtime/grants", map[string]any{}, &grant,
		WithRedactedResponseLog(),
		WithOperationClass(OperationMutate),
		WithIdempotent(false),
	)
	if err != nil {
		return nil, err
	}
	return &grant, nil
}
