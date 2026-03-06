package api

import "context"

// SimulatorPayload is sent to /simulator/render.
// Keep compatible with existing toolbelt format.
type SimulatorPayload struct {
	Simulator SimulatorRequest `json:"simulator"`
}

// SimulatorRequest contains the full render request context.
type SimulatorRequest struct {
	Code    string                  `json:"code"`
	Method  string                  `json:"method"`
	Path    string                  `json:"path"`
	Request SimulatorRequestContext `json:"request"`
	Version string                  `json:"version"`
}

// SimulatorRequestContext mirrors the Ruby-compatible request envelope.
type SimulatorRequestContext struct {
	Body    *string        `json:"body,omitempty"`
	Headers string         `json:"headers"`
	Host    string         `json:"host"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params"`
	Port    int            `json:"port"`
	Query   string         `json:"query"`
	RawBody *string        `json:"rawBody,omitempty"`
}

// SimulatorResponse is the simulator API response payload.
type SimulatorResponse struct {
	Body       string            `json:"body"`
	Encoding   string            `json:"encoding,omitempty"`
	Headers    map[string]string `json:"headers"`
	Status     int               `json:"status"`
	StatusCode int               `json:"statusCode,omitempty"`
}

// EffectiveStatus resolves alternate status field names.
func (r *SimulatorResponse) EffectiveStatus() int {
	if r == nil {
		return 0
	}
	if r.Status != 0 {
		return r.Status
	}
	if r.StatusCode != 0 {
		return r.StatusCode
	}
	return 0
}

// SimulatorRender calls the simulator rendering endpoint.
func (c *Client) SimulatorRender(ctx context.Context, payload SimulatorPayload) (*SimulatorResponse, error) {
	var out SimulatorResponse
	if err := c.Post(ctx, "/simulator/render", payload, &out); err != nil {
		return nil, err
	}

	if out.Status == 0 && out.StatusCode != 0 {
		out.Status = out.StatusCode
	}

	return &out, nil
}
