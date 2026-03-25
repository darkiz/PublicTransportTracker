package realtime

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	pb "github.com/MobilityData/gtfs-realtime-bindings/golang/gtfs"
	"github.com/darkiz/publictransporttracker/internal/vehicle"
)

// mockFetcher implements FeedFetcher for testing without HTTP.
type mockFetcher struct {
	data      []byte
	err       error
	callCount atomic.Int32
}

func (m *mockFetcher) Fetch(ctx context.Context, url string) ([]byte, error) {
	m.callCount.Add(1)
	return m.data, m.err
}

// mockHandler counts updates received from the poller.
type mockHandler struct {
	updates []vehicle.RawUpdate
	mu      chan struct{} // acts as lock for test assertions
}

func newMockHandler() *mockHandler {
	return &mockHandler{mu: make(chan struct{}, 1)}
}

func (h *mockHandler) HandleUpdates(updates []vehicle.RawUpdate) {
	h.mu <- struct{}{}
	h.updates = append(h.updates, updates...)
	<-h.mu
}

func TestPoller_FetchesAndDecodesOnTick(t *testing.T) {
	feed := buildVehicleFeed([]vehicleFixture{
		{id: "v1", tripID: "t1", routeID: "r1", lat: 59.33, lon: 18.07, timestamp: 1700000000},
	})
	data, _ := proto.Marshal(feed)

	fetcher := &mockFetcher{data: data}
	decoder := &ProtoDecoder{}
	handler := newMockHandler()

	poller := NewPoller(fetcher, decoder, handler.HandleUpdates)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Poll once manually.
	err := poller.PollOnce(ctx, "http://example.com/vehicle-positions")
	if err != nil {
		t.Fatalf("PollOnce: %v", err)
	}

	if fetcher.callCount.Load() != 1 {
		t.Errorf("expected 1 fetch call, got %d", fetcher.callCount.Load())
	}
	if len(handler.updates) != 1 {
		t.Errorf("expected 1 update delivered, got %d", len(handler.updates))
	}
	if handler.updates[0].VehicleID != "v1" {
		t.Errorf("expected vehicle v1, got %s", handler.updates[0].VehicleID)
	}
}

func TestPoller_HandlesEmptyFeed(t *testing.T) {
	feed := &pb.FeedMessage{
		Header: &pb.FeedHeader{GtfsRealtimeVersion: proto.String("2.0")},
	}
	data, _ := proto.Marshal(feed)

	fetcher := &mockFetcher{data: data}
	decoder := &ProtoDecoder{}
	handler := newMockHandler()

	poller := NewPoller(fetcher, decoder, handler.HandleUpdates)

	err := poller.PollOnce(context.Background(), "http://example.com/vp")
	if err != nil {
		t.Fatalf("PollOnce: %v", err)
	}

	// No updates should be delivered for empty feed.
	if len(handler.updates) != 0 {
		t.Errorf("expected 0 updates, got %d", len(handler.updates))
	}
}

func TestPoller_PropagatesFetchError(t *testing.T) {
	fetcher := &mockFetcher{err: context.DeadlineExceeded}
	decoder := &ProtoDecoder{}
	handler := newMockHandler()

	poller := NewPoller(fetcher, decoder, handler.HandleUpdates)

	err := poller.PollOnce(context.Background(), "http://example.com/vp")
	if err == nil {
		t.Error("expected error from fetch, got nil")
	}
}
