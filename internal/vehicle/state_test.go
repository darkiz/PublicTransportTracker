package vehicle

import (
	"testing"
	"time"
)

// TestStateManagerImplementsInterface verifies the contract at compile time.
func TestStateManagerImplementsInterface(t *testing.T) {
	var _ StateManager = (*DefaultStateManager)(nil)
}

func newTestManager() *DefaultStateManager {
	snapper := NewInMemorySnapper()
	snapper.LoadShape("shape1", [][2]float64{
		{59.33, 18.06},
		{59.33, 18.08},
	})

	return NewDefaultStateManager(
		func() PositionFilter { return NewKalmanFilter() },
		snapper,
	)
}

func TestStateManager_UpdateAndGet(t *testing.T) {
	mgr := newTestManager()
	now := time.Now()

	mgr.Update(RawUpdate{
		VehicleID: "bus-1",
		TripID:    "trip-100",
		RouteID:   "route-A",
		Position: Position{
			Lat: 59.3305, Lon: 18.07, Speed: 10, Bearing: 90,
			Timestamp: now,
		},
	})

	state := mgr.Get("bus-1")
	if state == nil {
		t.Fatal("expected state for bus-1, got nil")
	}
	if state.VehicleID != "bus-1" {
		t.Errorf("expected vehicle_id bus-1, got %s", state.VehicleID)
	}
	if state.TripID != "trip-100" {
		t.Errorf("expected trip_id trip-100, got %s", state.TripID)
	}
	if state.RouteID != "route-A" {
		t.Errorf("expected route_id route-A, got %s", state.RouteID)
	}
	if state.RawPosition.Lat != 59.3305 {
		t.Errorf("expected raw lat 59.3305, got %f", state.RawPosition.Lat)
	}
}

func TestStateManager_GetUnknownReturnsNil(t *testing.T) {
	mgr := newTestManager()

	if mgr.Get("nonexistent") != nil {
		t.Error("expected nil for unknown vehicle")
	}
}

func TestStateManager_AllReturnsSnapshot(t *testing.T) {
	mgr := newTestManager()
	now := time.Now()

	for i, id := range []string{"v1", "v2", "v3"} {
		mgr.Update(RawUpdate{
			VehicleID: id,
			TripID:    "trip",
			RouteID:   "route",
			Position: Position{
				Lat: 59.33 + float64(i)*0.001, Lon: 18.07,
				Timestamp: now,
			},
		})
	}

	all := mgr.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 vehicles, got %d", len(all))
	}

	// Verify it's a snapshot (not a reference to internal state).
	all[0].VehicleID = "mutated"
	original := mgr.Get("v1")
	if original.VehicleID == "mutated" {
		t.Error("All() should return a snapshot, not a reference")
	}
}

func TestStateManager_UpdateReplacesOldState(t *testing.T) {
	mgr := newTestManager()
	now := time.Now()

	mgr.Update(RawUpdate{
		VehicleID: "bus-1", TripID: "trip-1", RouteID: "route-A",
		Position: Position{Lat: 59.33, Lon: 18.07, Timestamp: now},
	})

	mgr.Update(RawUpdate{
		VehicleID: "bus-1", TripID: "trip-2", RouteID: "route-B",
		Position: Position{Lat: 59.34, Lon: 18.08, Timestamp: now.Add(10 * time.Second)},
	})

	state := mgr.Get("bus-1")
	if state.TripID != "trip-2" {
		t.Errorf("expected updated trip_id trip-2, got %s", state.TripID)
	}
	if state.RouteID != "route-B" {
		t.Errorf("expected updated route_id route-B, got %s", state.RouteID)
	}

	// Should still only be 1 vehicle.
	if len(mgr.All()) != 1 {
		t.Errorf("expected 1 vehicle, got %d", len(mgr.All()))
	}
}

func TestStateManager_Prune(t *testing.T) {
	mgr := newTestManager()
	old := time.Now().Add(-10 * time.Minute)
	recent := time.Now()

	mgr.Update(RawUpdate{
		VehicleID: "old-bus", TripID: "t", RouteID: "r",
		Position: Position{Lat: 59.33, Lon: 18.07, Timestamp: old},
	})
	mgr.Update(RawUpdate{
		VehicleID: "new-bus", TripID: "t", RouteID: "r",
		Position: Position{Lat: 59.34, Lon: 18.08, Timestamp: recent},
	})

	// Prune vehicles older than 5 minutes ago.
	cutoff := time.Now().Add(-5 * time.Minute)
	pruned := mgr.Prune(cutoff)

	if pruned != 1 {
		t.Errorf("expected 1 pruned, got %d", pruned)
	}
	if mgr.Get("old-bus") != nil {
		t.Error("old-bus should have been pruned")
	}
	if mgr.Get("new-bus") == nil {
		t.Error("new-bus should still exist")
	}
}

func TestStateManager_FilterIsPerVehicle(t *testing.T) {
	mgr := newTestManager()
	now := time.Now()

	// Two vehicles updating independently — they should have separate filters.
	mgr.Update(RawUpdate{
		VehicleID: "v1", TripID: "t", RouteID: "r",
		Position: Position{Lat: 59.33, Lon: 18.07, Timestamp: now},
	})
	mgr.Update(RawUpdate{
		VehicleID: "v2", TripID: "t", RouteID: "r",
		Position: Position{Lat: 60.00, Lon: 19.00, Timestamp: now},
	})

	s1 := mgr.Get("v1")
	s2 := mgr.Get("v2")

	// Their positions should not have contaminated each other.
	if s1.Position.Lat > 60.0 || s2.Position.Lat < 59.5 {
		t.Error("vehicles should have independent filter state")
	}
}
