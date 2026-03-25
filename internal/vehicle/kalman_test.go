package vehicle

import (
	"math"
	"testing"
	"time"
)

// TestKalmanImplementsInterface verifies the contract at compile time.
func TestKalmanImplementsInterface(t *testing.T) {
	var _ PositionFilter = (*KalmanFilter)(nil)
}

func TestKalmanFilter_FirstMeasurement(t *testing.T) {
	kf := NewKalmanFilter()
	now := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)

	pos := kf.Filter(Position{
		Lat: 59.3293, Lon: 18.0686, Bearing: 90, Speed: 10,
		Timestamp: now,
	})

	// First measurement should be returned as-is (no prior state to filter against).
	if pos.Lat != 59.3293 || pos.Lon != 18.0686 {
		t.Errorf("first measurement should pass through unchanged, got lat=%f lon=%f", pos.Lat, pos.Lon)
	}
	if pos.Bearing != 90 || pos.Speed != 10 {
		t.Errorf("first measurement bearing/speed should pass through, got bearing=%f speed=%f", pos.Bearing, pos.Speed)
	}
}

func TestKalmanFilter_SmoothsNoise(t *testing.T) {
	kf := NewKalmanFilter()
	base := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)

	// Simulate a vehicle at roughly (59.33, 18.07) with GPS noise.
	// The true position is constant — a stopped vehicle.
	trueLat, trueLon := 59.3300, 18.0700
	noisyPositions := []Position{
		{Lat: 59.3303, Lon: 18.0698, Timestamp: base},
		{Lat: 59.3297, Lon: 18.0705, Timestamp: base.Add(5 * time.Second)},
		{Lat: 59.3305, Lon: 18.0695, Timestamp: base.Add(10 * time.Second)},
		{Lat: 59.3295, Lon: 18.0708, Timestamp: base.Add(15 * time.Second)},
		{Lat: 59.3302, Lon: 18.0701, Timestamp: base.Add(20 * time.Second)},
		{Lat: 59.3298, Lon: 18.0703, Timestamp: base.Add(25 * time.Second)},
		{Lat: 59.3301, Lon: 18.0699, Timestamp: base.Add(30 * time.Second)},
		{Lat: 59.3299, Lon: 18.0702, Timestamp: base.Add(35 * time.Second)},
	}

	var lastFiltered Position
	for _, noisy := range noisyPositions {
		lastFiltered = kf.Filter(noisy)
	}

	// After several measurements, the filtered position should be closer to
	// the true position than the average noise amplitude.
	latErr := math.Abs(lastFiltered.Lat - trueLat)
	lonErr := math.Abs(lastFiltered.Lon - trueLon)

	// GPS noise was ~0.0005-0.0008 degrees. Filter should reduce error.
	if latErr > 0.0005 {
		t.Errorf("filtered lat error too large: %f (filtered=%f, true=%f)", latErr, lastFiltered.Lat, trueLat)
	}
	if lonErr > 0.0005 {
		t.Errorf("filtered lon error too large: %f (filtered=%f, true=%f)", lonErr, lastFiltered.Lon, trueLon)
	}
}

func TestKalmanFilter_TracksMovingVehicle(t *testing.T) {
	kf := NewKalmanFilter()
	base := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)

	// Simulate a vehicle moving north: lat increases by ~0.0001/sec.
	var lastFiltered Position
	for i := 0; i < 20; i++ {
		trueLat := 59.3300 + float64(i)*0.0001
		noisy := Position{
			Lat:       trueLat + 0.00005*math.Sin(float64(i)), // small oscillating noise
			Lon:       18.0700,
			Speed:     11.0, // ~11 m/s northward
			Bearing:   0,    // north
			Timestamp: base.Add(time.Duration(i) * time.Second),
		}
		lastFiltered = kf.Filter(noisy)
	}

	// After 20 steps, true lat should be ~59.3319. Filtered should track it.
	expectedLat := 59.3300 + 19*0.0001
	latErr := math.Abs(lastFiltered.Lat - expectedLat)
	if latErr > 0.001 {
		t.Errorf("filter not tracking moving vehicle: err=%f (filtered=%f, expected=%f)", latErr, lastFiltered.Lat, expectedLat)
	}
}

func TestKalmanFilter_HandlesTimeGap(t *testing.T) {
	kf := NewKalmanFilter()
	base := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)

	// First measurement.
	kf.Filter(Position{Lat: 59.33, Lon: 18.07, Timestamp: base})

	// Large time gap (2 minutes) — vehicle likely moved. Filter should
	// trust the new measurement more (increase process noise).
	pos := kf.Filter(Position{
		Lat: 59.34, Lon: 18.08,
		Timestamp: base.Add(2 * time.Minute),
	})

	// With a large gap, the filter should move substantially toward the
	// new measurement, not cling to the old position.
	latDelta := math.Abs(pos.Lat - 59.34)
	if latDelta > 0.005 {
		t.Errorf("after time gap, filter should trust new measurement more: delta=%f", latDelta)
	}
}

func TestKalmanFilter_SpeedAndBearing(t *testing.T) {
	kf := NewKalmanFilter()
	base := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)

	kf.Filter(Position{Lat: 59.33, Lon: 18.07, Speed: 10, Bearing: 45, Timestamp: base})
	pos := kf.Filter(Position{Lat: 59.33, Lon: 18.07, Speed: 12, Bearing: 50, Timestamp: base.Add(5 * time.Second)})

	// Speed and bearing should be non-negative and in valid ranges.
	if pos.Speed < 0 {
		t.Errorf("filtered speed should not be negative: %f", pos.Speed)
	}
	if pos.Bearing < 0 || pos.Bearing >= 360 {
		t.Errorf("filtered bearing out of range [0,360): %f", pos.Bearing)
	}
}
