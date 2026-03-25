package broadcast

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/vmihailenco/msgpack/v5"
	"nhooyr.io/websocket"
)

// WSClient wraps a nhooyr.io/websocket connection to implement the Client interface.
//
// Single Responsibility: only manages one WebSocket connection's send buffer.
// The hub calls Send(); the handler goroutine manages the lifecycle.
type WSClient struct {
	conn   *websocket.Conn
	sendCh chan []byte
	done   chan struct{}
	once   sync.Once
}

// newWSClient creates a WSClient with a buffered send channel.
func newWSClient(conn *websocket.Conn) *WSClient {
	return &WSClient{
		conn:   conn,
		sendCh: make(chan []byte, 64),
		done:   make(chan struct{}),
	}
}

// Send encodes the envelope as msgpack and queues it for writing.
// Returns false if the client is closed or the buffer is full.
func (c *WSClient) Send(env Envelope) bool {
	data, err := msgpack.Marshal(env)
	if err != nil {
		return false
	}

	select {
	case c.sendCh <- data:
		return true
	case <-c.done:
		return false
	default:
		// Buffer full — drop this message (backpressure).
		return true
	}
}

// Close terminates the connection and signals the write loop to stop.
func (c *WSClient) Close() {
	c.once.Do(func() {
		close(c.done)
		c.conn.Close(websocket.StatusNormalClosure, "")
	})
}

// writePump sends queued messages to the WebSocket connection.
// Exits when the done channel is closed or the connection errors.
func (c *WSClient) writePump(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		case data := <-c.sendCh:
			writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := c.conn.Write(writeCtx, websocket.MessageBinary, data)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

// readPump reads from the WebSocket to detect client disconnect.
// We don't expect client-to-server messages, but we need to consume
// the read side to get close/ping notifications.
func (c *WSClient) readPump(ctx context.Context) {
	for {
		_, _, err := c.conn.Read(ctx)
		if err != nil {
			return
		}
	}
}

// HandleWebSocket upgrades an HTTP connection to WebSocket, registers with
// the hub, and manages the connection lifecycle.
//
// This function blocks until the client disconnects.
func HandleWebSocket(w http.ResponseWriter, r *http.Request, hub *Hub) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // Allow any origin for development.
	})
	if err != nil {
		slog.Error("websocket accept failed", "error", err)
		return
	}

	client := newWSClient(conn)
	hub.Register(client)

	slog.Info("websocket client connected", "remote", r.RemoteAddr, "clients", hub.ClientCount())

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Run read and write pumps concurrently.
	go client.writePump(ctx)
	client.readPump(ctx) // blocks until disconnect

	// Clean up.
	client.Close()
	hub.Unregister(client)

	slog.Info("websocket client disconnected", "remote", r.RemoteAddr, "clients", hub.ClientCount())
}
