package vehicle

import (
	"sync"
	"time"
)

// vehicleEntry holds the per-vehicle filter and current state.
type vehicleEntry struct {
	filter PositionFilter
	state  State
}

// DefaultStateManager is the production implementation of StateManager.
//
// Single Responsibility: orchestrates filter → snap → store for each update.
// Open/Closed: new filter types can be injected via PositionFilterFactory.
// Dependency Inversion: depends on PositionFilter and RouteSnapper interfaces.
type DefaultStateManager struct {
	mu            sync.RWMutex
	vehicles      map[string]*vehicleEntry
	filterFactory PositionFilterFactory
	snapper       RouteSnapper
}

// NewDefaultStateManager creates a StateManager with the given dependencies.
func NewDefaultStateManager(filterFactory PositionFilterFactory, snapper RouteSnapper) *DefaultStateManager {
	return &DefaultStateManager{
		vehicles:      make(map[string]*vehicleEntry),
		filterFactory: filterFactory,
		snapper:       snapper,
	}
}

// Update processes a raw position through the filter → snap pipeline.
func (m *DefaultStateManager) Update(update RawUpdate) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.vehicles[update.VehicleID]
	if !ok {
		entry = &vehicleEntry{
			filter: m.filterFactory(),
		}
		m.vehicles[update.VehicleID] = entry
	}

	// Pipeline: raw → filter → snap.
	filtered := entry.filter.Filter(update.Position)
	snapped := m.snapper.Snap(filtered, shapeIDForUpdate(update))

	entry.state = State{
		VehicleID:   update.VehicleID,
		TripID:      update.TripID,
		RouteID:     update.RouteID,
		Position:    snapped,
		RawPosition: update.Position,
		UpdatedAt:   update.Position.Timestamp,
	}
}

// Get returns the current state of a vehicle, or nil if unknown.
func (m *DefaultStateManager) Get(vehicleID string) *State {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.vehicles[vehicleID]
	if !ok {
		return nil
	}
	s := entry.state
	return &s
}

// All returns a snapshot of all vehicle states.
func (m *DefaultStateManager) All() []State {
	m.mu.RLock()
	defer m.mu.RUnlock()

	states := make([]State, 0, len(m.vehicles))
	for _, entry := range m.vehicles {
		states = append(states, entry.state)
	}
	return states
}

// Prune removes vehicles not updated since the given cutoff time.
// Returns the number of vehicles removed.
func (m *DefaultStateManager) Prune(olderThan time.Time) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for id, entry := range m.vehicles {
		if entry.state.UpdatedAt.Before(olderThan) {
			delete(m.vehicles, id)
			count++
		}
	}
	return count
}

// shapeIDForUpdate derives the shape ID to use for snapping. For now, this
// uses the route ID as a lookup key. In the future, this could look up the
// trip's specific shape_id from the database.
func shapeIDForUpdate(update RawUpdate) string {
	// TODO: look up trip -> shape_id from GTFS static data.
	// For now, pass the trip ID — the snapper will return original position
	// if no shape is loaded for this ID.
	return update.TripID
}
