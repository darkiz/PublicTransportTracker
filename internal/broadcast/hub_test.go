package broadcast

import (
	"sync"
	"testing"
	"time"
)

// fakeClient collects messages for test assertions.
type fakeClient struct {
	mu       sync.Mutex
	messages []Envelope
	closed   bool
}

func (c *fakeClient) Send(env Envelope) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	c.messages = append(c.messages, env)
	return true
}

func (c *fakeClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
}

func (c *fakeClient) Messages() []Envelope {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make([]Envelope, len(c.messages))
	copy(cp, c.messages)
	return cp
}

func TestHub_RegisterAndBroadcast(t *testing.T) {
	hub := NewHub()

	c1 := &fakeClient{}
	c2 := &fakeClient{}
	hub.Register(c1)
	hub.Register(c2)

	if hub.ClientCount() != 2 {
		t.Fatalf("expected 2 clients, got %d", hub.ClientCount())
	}

	env := Envelope{
		Type: "update",
		Vehicles: []VehicleUpdate{
			{VehicleID: "bus-1", Lat: 59.33, Lon: 18.07},
		},
	}
	hub.Broadcast(env)

	// Both clients should have received the message.
	if msgs := c1.Messages(); len(msgs) != 1 {
		t.Errorf("c1 expected 1 message, got %d", len(msgs))
	}
	if msgs := c2.Messages(); len(msgs) != 1 {
		t.Errorf("c2 expected 1 message, got %d", len(msgs))
	}
}

func TestHub_UnregisterRemovesClient(t *testing.T) {
	hub := NewHub()

	c1 := &fakeClient{}
	c2 := &fakeClient{}
	hub.Register(c1)
	hub.Register(c2)
	hub.Unregister(c1)

	if hub.ClientCount() != 1 {
		t.Fatalf("expected 1 client after unregister, got %d", hub.ClientCount())
	}

	env := Envelope{Type: "update", Vehicles: []VehicleUpdate{{VehicleID: "v1"}}}
	hub.Broadcast(env)

	if msgs := c1.Messages(); len(msgs) != 0 {
		t.Errorf("unregistered c1 should get 0 messages, got %d", len(msgs))
	}
	if msgs := c2.Messages(); len(msgs) != 1 {
		t.Errorf("c2 expected 1 message, got %d", len(msgs))
	}
}

func TestHub_BroadcastDropsFailedClient(t *testing.T) {
	hub := NewHub()

	good := &fakeClient{}
	bad := &fakeClient{}
	bad.Close() // pre-close so Send returns false

	hub.Register(good)
	hub.Register(bad)

	env := Envelope{Type: "update", Vehicles: []VehicleUpdate{{VehicleID: "v1"}}}
	hub.Broadcast(env)

	// Bad client should be auto-removed.
	if hub.ClientCount() != 1 {
		t.Errorf("expected bad client removed, got %d clients", hub.ClientCount())
	}

	if msgs := good.Messages(); len(msgs) != 1 {
		t.Errorf("good client expected 1 message, got %d", len(msgs))
	}
}

func TestHub_BroadcastToEmpty(t *testing.T) {
	hub := NewHub()

	// Should not panic on empty hub.
	env := Envelope{Type: "update", Vehicles: []VehicleUpdate{{VehicleID: "v1"}}}
	hub.Broadcast(env)

	if hub.ClientCount() != 0 {
		t.Errorf("expected 0 clients, got %d", hub.ClientCount())
	}
}

func TestHub_ConcurrentRegisterBroadcast(t *testing.T) {
	hub := NewHub()

	var wg sync.WaitGroup
	clients := make([]*fakeClient, 100)

	// Concurrently register 100 clients.
	for i := range clients {
		clients[i] = &fakeClient{}
		wg.Add(1)
		go func(c *fakeClient) {
			defer wg.Done()
			hub.Register(c)
		}(clients[i])
	}
	wg.Wait()

	if hub.ClientCount() != 100 {
		t.Fatalf("expected 100 clients, got %d", hub.ClientCount())
	}

	// Broadcast while concurrently unregistering some.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			hub.Unregister(clients[i])
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			hub.Broadcast(Envelope{Type: "update", Vehicles: []VehicleUpdate{{VehicleID: "v1"}}})
			time.Sleep(time.Millisecond)
		}
	}()

	wg.Wait()

	// Should have ~50 clients remaining (no panics, no races).
	if hub.ClientCount() != 50 {
		t.Errorf("expected ~50 clients remaining, got %d", hub.ClientCount())
	}
}
