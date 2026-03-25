package broadcast

import "math"

// DeltaEncoder tracks previous vehicle state and produces diffs.
//
// Single Responsibility: only computes deltas. Knows nothing about WebSocket
// frames, connections, or encoding formats.
type DeltaEncoder struct {
	prev map[string]VehicleUpdate
}

// NewDeltaEncoder creates a delta encoder with no previous state.
func NewDeltaEncoder() *DeltaEncoder {
	return &DeltaEncoder{
		prev: make(map[string]VehicleUpdate),
	}
}

// Encode compares the current vehicle list against the previous state and
// returns only the updates that changed, plus removal markers for vehicles
// that disappeared. On the first call, all vehicles are returned.
func (e *DeltaEncoder) Encode(current []VehicleUpdate) []VehicleUpdate {
	currentMap := make(map[string]VehicleUpdate, len(current))
	for _, v := range current {
		currentMap[v.VehicleID] = v
	}

	var delta []VehicleUpdate

	// Find changed or new vehicles.
	for _, v := range current {
		prev, existed := e.prev[v.VehicleID]
		if !existed || changed(prev, v) {
			delta = append(delta, v)
		}
	}

	// Find removed vehicles.
	for id := range e.prev {
		if _, exists := currentMap[id]; !exists {
			delta = append(delta, VehicleUpdate{
				VehicleID: id,
				Removed:   true,
			})
		}
	}

	// Update previous state.
	e.prev = currentMap

	return delta
}

// changed reports whether two updates differ meaningfully.
// Uses a small epsilon for floating point comparison.
func changed(a, b VehicleUpdate) bool {
	const eps = 1e-7
	if a.TripID != b.TripID || a.RouteID != b.RouteID {
		return true
	}
	if a.DelaySeconds != b.DelaySeconds || a.Removed != b.Removed {
		return true
	}
	if math.Abs(a.Lat-b.Lat) > eps || math.Abs(a.Lon-b.Lon) > eps {
		return true
	}
	if math.Abs(a.Bearing-b.Bearing) > eps || math.Abs(a.Speed-b.Speed) > eps {
		return true
	}
	return false
}
