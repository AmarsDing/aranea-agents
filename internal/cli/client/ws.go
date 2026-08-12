package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/safego"

	"github.com/gorilla/websocket"
)

// Envelope mirrors the server's wsUpstream / wsDownstream structure.
// Used for both sending and receiving.
type Envelope struct {
	Direction string         `json:"direction,omitempty"`
	Channel   string         `json:"channel,omitempty"`
	Type      string         `json:"type"`
	RequestID string         `json:"request_id,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
	// Envelope field carries downstream event envelopes from the server.
	Envelope map[string]any `json:"envelope,omitempty"`
}

// WSClient dials the Aranea WebSocket endpoint.
type WSClient struct {
	Base  string // e.g. "http://localhost:8800"
	Token string
	Debug bool
}

// WSConn is an active WebSocket connection.
type WSConn struct {
	conn      *websocket.Conn
	events    chan Envelope
	done      chan struct{}
	closeOnce sync.Once
	mu        sync.Mutex
}

// Dial opens a WS connection for the given sessionID.
// URL: ws(s)://{host}/v1/ws?session_id=<id>; auth uses Authorization: Bearer.
func (w *WSClient) Dial(ctx context.Context, sessionID string) (*WSConn, error) {
	wsBase := strings.Replace(w.Base, "http://", "ws://", 1)
	wsBase = strings.Replace(wsBase, "https://", "wss://", 1)

	u, err := url.Parse(wsBase + "/v1/ws")
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	q := u.Query()
	q.Set("session_id", sessionID)
	u.RawQuery = q.Encode()

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	headers := http.Header{}
	if w.Token != "" {
		headers.Set("Authorization", "Bearer "+w.Token)
	}
	conn, _, err := dialer.DialContext(ctx, u.String(), headers)
	if err != nil {
		return nil, fmt.Errorf("ws dial %s%s: %w", u.Host, u.Path, err)
	}

	wc := &WSConn{
		conn:   conn,
		events: make(chan Envelope, 128),
		done:   make(chan struct{}),
	}
	safego.Go(ctx, "cli.ws.readPump", wc.readPump)
	return wc, nil
}

func (c *WSConn) readPump() {
	defer func() {
		close(c.events)
		close(c.done)
	}()
	c.conn.SetReadLimit(1 << 22) // 4 MB
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var env Envelope
		if jsonErr := json.Unmarshal(msg, &env); jsonErr != nil {
			fmt.Fprintf(os.Stderr, "warning: dropping malformed WS message: %v\n", jsonErr)
			continue
		}
		select {
		case c.events <- env:
		default:
			// Drop if channel is full (slow consumer); warn so user knows events were lost.
			fmt.Fprintln(os.Stderr, "warning: WS event buffer full, dropping event (slow consumer)")
		}
	}
}

// Send writes an upstream envelope to the server.
func (c *WSConn) Send(ctx context.Context, msg Envelope) error {
	msg.Direction = "client_to_server"
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

// Events returns the read-only channel of incoming server envelopes.
func (c *WSConn) Events() <-chan Envelope {
	return c.events
}

// Done returns a channel that is closed when the connection terminates.
func (c *WSConn) Done() <-chan struct{} {
	return c.done
}

// Close terminates the connection.
func (c *WSConn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		_ = c.conn.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		)
		err = c.conn.Close()
	})
	return err
}
