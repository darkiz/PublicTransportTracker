package broadcast

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vmihailenco/msgpack/v5"
	"nhooyr.io/websocket"
)

func TestWSClient_ImplementsClientInterface(t *testing.T) {
	var _ Client = (*WSClient)(nil)
}

func TestWSClient_ReceivesBroadcasts(t *testing.T) {
	hub := NewHub()

	// Start a test HTTP server with the WebSocket handler.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		HandleWebSocket(w, r, hub)
	}))
	defer srv.Close()

	// Connect a WebSocket client.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + srv.URL[4:] // http -> ws
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Give the handler time to register the client.
	time.Sleep(50 * time.Millisecond)

	if hub.ClientCount() != 1 {
		t.Fatalf("expected 1 client registered, got %d", hub.ClientCount())
	}

	// Broadcast a message.
	hub.Broadcast(Envelope{
		Type: "update",
		Vehicles: []VehicleUpdate{
			{VehicleID: "bus-1", Lat: 59.33, Lon: 18.07, Bearing: 90, Speed: 10},
		},
	})

	// Read from the WebSocket.
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var env Envelope
	if err := msgpack.Unmarshal(data, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if env.Type != "update" {
		t.Errorf("expected type 'update', got %q", env.Type)
	}
	if len(env.Vehicles) != 1 || env.Vehicles[0].VehicleID != "bus-1" {
		t.Errorf("unexpected vehicles: %+v", env.Vehicles)
	}
}

func TestWSClient_DisconnectUnregisters(t *testing.T) {
	hub := NewHub()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		HandleWebSocket(w, r, hub)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + srv.URL[4:]
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if hub.ClientCount() != 1 {
		t.Fatalf("expected 1 client, got %d", hub.ClientCount())
	}

	// Close the connection.
	conn.Close(websocket.StatusNormalClosure, "bye")
	time.Sleep(100 * time.Millisecond)

	// The client should be unregistered (either eagerly or on next broadcast).
	hub.Broadcast(Envelope{Type: "ping"})
	time.Sleep(50 * time.Millisecond)

	if hub.ClientCount() != 0 {
		t.Errorf("expected 0 clients after disconnect, got %d", hub.ClientCount())
	}
}

func TestWSClient_MultipleClients(t *testing.T) {
	hub := NewHub()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		HandleWebSocket(w, r, hub)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + srv.URL[4:]

	conns := make([]*websocket.Conn, 5)
	for i := range conns {
		c, _, err := websocket.Dial(ctx, wsURL, nil)
		if err != nil {
			t.Fatalf("dial client %d: %v", i, err)
		}
		conns[i] = c
	}
	defer func() {
		for _, c := range conns {
			c.Close(websocket.StatusNormalClosure, "")
		}
	}()

	time.Sleep(100 * time.Millisecond)

	if hub.ClientCount() != 5 {
		t.Fatalf("expected 5 clients, got %d", hub.ClientCount())
	}

	// Broadcast and verify all receive.
	hub.Broadcast(Envelope{
		Type:     "update",
		Vehicles: []VehicleUpdate{{VehicleID: "v1", Lat: 1, Lon: 2}},
	})

	for i, c := range conns {
		_, data, err := c.Read(ctx)
		if err != nil {
			t.Fatalf("read client %d: %v", i, err)
		}
		var env Envelope
		if err := msgpack.Unmarshal(data, &env); err != nil {
			t.Fatalf("unmarshal client %d: %v", i, err)
		}
		if len(env.Vehicles) != 1 || env.Vehicles[0].VehicleID != "v1" {
			t.Errorf("client %d got unexpected data: %+v", i, env)
		}
	}
}
