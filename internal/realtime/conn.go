package realtime

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/coder/websocket"
)

// Subprotocol is the ActionCable JSON subprotocol used by the realtime socket.
const Subprotocol = "actioncable-v1-json"

const readLimitBytes = 4 << 20

// Conn is the minimal websocket surface the session loop needs.
type Conn interface {
	Read(ctx context.Context) ([]byte, error)
	Write(ctx context.Context, b []byte) error
	Close(reason string) error
}

// DialOptions tunes the websocket handshake.
type DialOptions struct {
	Insecure   bool
	HTTPClient *http.Client
	Headers    http.Header
}

type wsConn struct {
	conn *websocket.Conn
}

func (c *wsConn) Read(ctx context.Context) ([]byte, error) {
	_, data, err := c.conn.Read(ctx)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (c *wsConn) Write(ctx context.Context, b []byte) error {
	return c.conn.Write(ctx, websocket.MessageText, b)
}

func (c *wsConn) Close(reason string) error {
	return c.conn.Close(websocket.StatusNormalClosure, truncateCloseReason(reason))
}

// Dial opens the realtime websocket for a site using a one-time grant.
// The grant is carried in the query string and is scrubbed from every error.
func Dial(ctx context.Context, siteHost, grant string, opts DialOptions) (Conn, error) {
	target, display, err := websocketURL(siteHost, grant)
	if err != nil {
		return nil, err
	}

	dialOpts := &websocket.DialOptions{
		Subprotocols: []string{Subprotocol},
		HTTPHeader:   opts.Headers,
		HTTPClient:   httpClientForDial(opts),
	}

	conn, resp, err := websocket.Dial(ctx, target, dialOpts)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, dialFailure(display, grant, err)
	}
	conn.SetReadLimit(readLimitBytes)
	return &wsConn{conn: conn}, nil
}

// httpClientForDial returns a client safe for a long-lived websocket: the
// request timeout of an API client would otherwise close the socket mid-stream.
func httpClientForDial(opts DialOptions) *http.Client {
	client := &http.Client{}
	if opts.HTTPClient != nil {
		clone := *opts.HTTPClient
		clone.Timeout = 0
		client = &clone
	}
	if opts.Insecure {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // dev-only escape hatch
		}
	}
	return client
}

func websocketURL(siteHost, grant string) (target, display string, err error) {
	host := strings.TrimSpace(siteHost)
	scheme := "wss"
	switch {
	case strings.HasPrefix(host, "ws://"):
		scheme, host = "ws", strings.TrimPrefix(host, "ws://")
	case strings.HasPrefix(host, "http://"):
		scheme, host = "ws", strings.TrimPrefix(host, "http://")
	case strings.HasPrefix(host, "wss://"):
		host = strings.TrimPrefix(host, "wss://")
	case strings.HasPrefix(host, "https://"):
		host = strings.TrimPrefix(host, "https://")
	}
	host = strings.Trim(host, "/")
	if host == "" {
		return "", "", fmt.Errorf("realtime: no site host to connect to")
	}

	u := url.URL{Scheme: scheme, Host: host, Path: "/ws"}
	display = u.String()
	u.RawQuery = url.Values{"grant": []string{grant}}.Encode()
	return u.String(), display, nil
}

type dialError struct {
	msg string
	err error
}

func (e *dialError) Error() string { return e.msg }

func (e *dialError) Unwrap() error { return e.err }

func dialFailure(display, grant string, err error) error {
	text := scrubGrant(err.Error(), grant)
	failure := &dialError{msg: fmt.Sprintf("realtime: dial %s: %s", display, text)}
	// Only keep the cause when it cannot leak the grant through Unwrap.
	if !containsGrant(err.Error(), grant) {
		failure.err = err
	}
	return failure
}

func containsGrant(text, grant string) bool {
	if grant == "" {
		return false
	}
	return strings.Contains(text, grant) || strings.Contains(text, url.QueryEscape(grant))
}

func scrubGrant(text, grant string) string {
	if grant == "" {
		return text
	}
	text = strings.ReplaceAll(text, grant, "<redacted>")
	return strings.ReplaceAll(text, url.QueryEscape(grant), "<redacted>")
}

func truncateCloseReason(reason string) string {
	const maxReason = 123
	if len(reason) <= maxReason {
		return reason
	}
	return reason[:maxReason]
}
