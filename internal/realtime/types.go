// Package realtime handles GTFS-RT feed polling and decoding.
package realtime

import (
	"context"

	"github.com/darkiz/publictransporttracker/internal/vehicle"
)

// FeedConfig holds the configuration for a single GTFS-RT feed.
type FeedConfig struct {
	// URL is the GTFS-RT VehiclePositions feed URL.
	VehiclePositionsURL string
	// TripUpdatesURL is the GTFS-RT TripUpdates feed URL (optional).
	TripUpdatesURL string
	// PollInterval is how often to fetch the feed (e.g., 15s).
	PollIntervalSec int
}

// FeedFetcher retrieves raw bytes from a GTFS-RT feed endpoint.
// Abstracted for testability — tests inject a mock instead of making HTTP calls.
type FeedFetcher interface {
	Fetch(ctx context.Context, url string) ([]byte, error)
}

// FeedDecoder decodes raw GTFS-RT protobuf bytes into domain updates.
type FeedDecoder interface {
	DecodeVehiclePositions(data []byte) ([]vehicle.RawUpdate, error)
	DecodeDelays(data []byte) (map[string]int, error) // tripID -> delay seconds
}
