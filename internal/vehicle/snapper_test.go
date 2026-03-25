package vehicle

import (
	"math"
	"testing"
	"time"
)

// TestSnapperImplementsInterface verifies the contract at compile time.
func TestSnapperImplementsInterface(t *testing.T) {
	var _ RouteSnapper = (*InMemorySnapper)(nil)
}

func TestSnapper_SnapToSegment(t *testing.T) {
	snapper := NewInMemorySnapper()

	// A simple east-west line from (59.33, 18.06) to (59.33, 18.08).
	snapper.LoadShape("shape1", [][2]float64{
		{59.33, 18.06},
		{59.33, 18.08},
	})

	// Point slightly north of the line — should snap to the line.
	pos := Position{Lat: 59.3305, Lon: 18.07, Timestamp: time.Now()}
	snapped := snapper.Snap(pos, "shape1")

	// Snapped lat should be very close to 59.33 (on the line).
	if math.Abs(snapped.Lat-59.33) > 0.0005 {
		t.Errorf("expected snapped lat ~59.33, got %f", snapped.Lat)
	}
	// Snapped lon should be ~18.07 (perpendicular projection).
	if math.Abs(snapped.Lon-18.07) > 0.001 {
		t.Errorf("expected snapped lon ~18.07, got %f", snapped.Lon)
	}
}

func TestSnapper_SnapToClosestSegment(t *testing.T) {
	snapper := NewInMemorySnapper()

	// L-shaped route: east then north.
	snapper.LoadShape("shape2", [][2]float64{
		{59.33, 18.06},
		{59.33, 18.08}, // corner
		{59.35, 18.08},
	})

	// Point near the north segment.
	pos := Position{Lat: 59.34, Lon: 18.079, Timestamp: time.Now()}
	snapped := snapper.Snap(pos, "shape2")

	// Should snap to the vertical segment (lon ~18.08, lat ~59.34).
	if math.Abs(snapped.Lon-18.08) > 0.002 {
		t.Errorf("expected snapped lon ~18.08, got %f", snapped.Lon)
	}
	if math.Abs(snapped.Lat-59.34) > 0.002 {
		t.Errorf("expected snapped lat ~59.34, got %f", snapped.Lat)
	}
}

func TestSnapper_UnknownShapeReturnsOriginal(t *testing.T) {
	snapper := NewInMemorySnapper()

	pos := Position{Lat: 59.33, Lon: 18.07, Timestamp: time.Now()}
	snapped := snapper.Snap(pos, "nonexistent")

	if snapped.Lat != pos.Lat || snapped.Lon != pos.Lon {
		t.Error("unknown shape should return original position")
	}
}

func TestSnapper_SinglePointShapeReturnsOriginal(t *testing.T) {
	snapper := NewInMemorySnapper()
	snapper.LoadShape("one_point", [][2]float64{{59.33, 18.07}})

	pos := Position{Lat: 59.34, Lon: 18.08, Timestamp: time.Now()}
	snapped := snapper.Snap(pos, "one_point")

	// Can't snap to a single point — return original.
	if snapped.Lat != pos.Lat || snapped.Lon != pos.Lon {
		t.Error("single-point shape should return original position")
	}
}

func TestSnapper_SnapPreservesBearingAndSpeed(t *testing.T) {
	snapper := NewInMemorySnapper()
	snapper.LoadShape("shape3", [][2]float64{
		{59.33, 18.06},
		{59.33, 18.08},
	})

	pos := Position{Lat: 59.3305, Lon: 18.07, Speed: 15.0, Bearing: 90.0, Timestamp: time.Now()}
	snapped := snapper.Snap(pos, "shape3")

	if snapped.Speed != 15.0 {
		t.Errorf("snap should preserve speed, got %f", snapped.Speed)
	}
	if snapped.Timestamp != pos.Timestamp {
		t.Error("snap should preserve timestamp")
	}
}

func TestSnapper_SnapNearEndpoint(t *testing.T) {
	snapper := NewInMemorySnapper()
	snapper.LoadShape("shape4", [][2]float64{
		{59.33, 18.06},
		{59.33, 18.08},
	})

	// Point past the end of the segment.
	pos := Position{Lat: 59.33, Lon: 18.09, Timestamp: time.Now()}
	snapped := snapper.Snap(pos, "shape4")

	// Should clamp to the endpoint (59.33, 18.08).
	if math.Abs(snapped.Lon-18.08) > 0.001 {
		t.Errorf("expected snapped to endpoint lon ~18.08, got %f", snapped.Lon)
	}
}
