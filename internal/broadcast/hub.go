package broadcast

import "sync"

// Client is the interface that any WebSocket connection wrapper must implement.
// Interface Segregation: only Send and Close — the hub doesn't know about
// WebSocket frames, encoding, or HTTP upgrades.
type Client interface {
	// Send delivers an envelope to the client. Returns false if the client
	// is dead and should be removed.
	Send(env Envelope) bool
	// Close terminates the connection.
	Close()
}

// Hub manages a set of connected clients and broadcasts updates to all of them.
//
// Single Responsibility: only fan-out. It doesn't encode, filter, or fetch.
// Thread-safe for concurrent Register/Unregister/Broadcast.
type Hub struct {
	mu      sync.RWMutex
	clients map[Client]struct{}
}

// NewHub creates an empty hub ready for client registration.
func NewHub() *Hub {
	return &Hub{
		clients: make(map[Client]struct{}),
	}
}

// Register adds a client to the broadcast set.
func (h *Hub) Register(c Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c] = struct{}{}
}

// Unregister removes a client from the broadcast set.
func (h *Hub) Unregister(c Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, c)
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Broadcast sends an envelope to all connected clients.
// Clients that fail to receive are removed automatically.
func (h *Hub) Broadcast(env Envelope) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for c := range h.clients {
		if !c.Send(env) {
			delete(h.clients, c)
		}
	}
}
