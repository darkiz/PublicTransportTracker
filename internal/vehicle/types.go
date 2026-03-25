// Package vehicle defines the core domain types and interfaces for real-time
// vehicle tracking. All components depend on these abstractions, not on each
// other (Dependency Inversion Principle).
package vehicle

import "time"

// Position represents a geographic position with metadata.
type Position struct {
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	Bearing   float64   `json:"bearing"`   // degrees from north, 0-360
	Speed     float64   `json:"speed"`     // meters per second
	Timestamp time.Time `json:"timestamp"`
}

// RawUpdate is the unprocessed position update from a GTFS-RT feed.
type RawUpdate struct {
	VehicleID string
	TripID    string
	RouteID   string
	Position  Position
}

// State holds the current known state of a single vehicle.
type State struct {
	VehicleID    string   `json:"vehicle_id"`
	TripID       string   `json:"trip_id"`
	RouteID      string   `json:"route_id"`
	Position     Position `json:"position"`       // filtered + snapped position
	RawPosition  Position `json:"raw_position"`   // original from feed
	DelaySeconds int      `json:"delay_seconds"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// PositionFilter smooths noisy GPS positions. Implementations must be safe
// for sequential use per vehicle (one filter instance per vehicle).
//
// Interface Segregation: callers only need this one method.
type PositionFilter interface {
	// Filter takes a raw GPS measurement and returns a smoothed position.
	// The filter maintains internal state between calls.
	Filter(measurement Position) Position
}

// PositionFilterFactory creates a new PositionFilter for a vehicle.
// This allows the StateManager to create filters without knowing the
// concrete type (Dependency Inversion).
type PositionFilterFactory func() PositionFilter

// RouteSnapper snaps a position to the nearest point on a route shape.
type RouteSnapper interface {
	// Snap projects the given position onto the route's shape geometry.
	// Returns the snapped position. If the route has no shape or snapping
	// fails, it returns the original position unchanged.
	Snap(pos Position, shapeID string) Position
}

// StateManager maintains the current state of all tracked vehicles.
type StateManager interface {
	// Update processes a raw position update through the filter and snapper
	// pipeline, then stores the resulting state.
	Update(update RawUpdate)

	// Get returns the current state of a single vehicle, or nil if unknown.
	Get(vehicleID string) *State

	// All returns a snapshot of all current vehicle states.
	All() []State

	// Prune removes vehicles that haven't been updated since the given time.
	Prune(olderThan time.Time) int
}
