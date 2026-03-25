package broadcast

import (
	"encoding/json"
	"testing"

	"github.com/vmihailenco/msgpack/v5"
)

func TestDeltaEncoder_FirstCallSendsAll(t *testing.T) {
	enc := NewDeltaEncoder()

	updates := []VehicleUpdate{
		{VehicleID: "v1", Lat: 59.33, Lon: 18.07, Bearing: 90, Speed: 10},
		{VehicleID: "v2", Lat: 60.00, Lon: 19.00, Bearing: 180, Speed: 5},
	}

	delta := enc.Encode(updates)

	if len(delta) != 2 {
		t.Fatalf("first call should return all %d vehicles, got %d", len(updates), len(delta))
	}
	if delta[0].VehicleID != "v1" || delta[1].VehicleID != "v2" {
		t.Error("first call should preserve vehicle IDs")
	}
}

func TestDeltaEncoder_UnchangedVehiclesOmitted(t *testing.T) {
	enc := NewDeltaEncoder()

	updates := []VehicleUpdate{
		{VehicleID: "v1", Lat: 59.33, Lon: 18.07, Bearing: 90, Speed: 10, TripID: "t1", RouteID: "r1"},
		{VehicleID: "v2", Lat: 60.00, Lon: 19.00, Bearing: 180, Speed: 5, TripID: "t2", RouteID: "r2"},
	}

	enc.Encode(updates) // seed state

	// Second call with same data — nothing changed.
	delta := enc.Encode(updates)

	if len(delta) != 0 {
		t.Errorf("unchanged vehicles should produce empty delta, got %d", len(delta))
	}
}

func TestDeltaEncoder_ChangedPositionIncluded(t *testing.T) {
	enc := NewDeltaEncoder()

	enc.Encode([]VehicleUpdate{
		{VehicleID: "v1", Lat: 59.33, Lon: 18.07},
		{VehicleID: "v2", Lat: 60.00, Lon: 19.00},
	})

	// v1 moved, v2 unchanged.
	delta := enc.Encode([]VehicleUpdate{
		{VehicleID: "v1", Lat: 59.34, Lon: 18.08},
		{VehicleID: "v2", Lat: 60.00, Lon: 19.00},
	})

	if len(delta) != 1 {
		t.Fatalf("expected 1 changed vehicle, got %d", len(delta))
	}
	if delta[0].VehicleID != "v1" {
		t.Errorf("expected changed vehicle v1, got %s", delta[0].VehicleID)
	}
	if delta[0].Lat != 59.34 {
		t.Errorf("expected new lat 59.34, got %f", delta[0].Lat)
	}
}

func TestDeltaEncoder_RemovedVehicleMarked(t *testing.T) {
	enc := NewDeltaEncoder()

	enc.Encode([]VehicleUpdate{
		{VehicleID: "v1", Lat: 59.33, Lon: 18.07},
		{VehicleID: "v2", Lat: 60.00, Lon: 19.00},
	})

	// v2 disappeared.
	delta := enc.Encode([]VehicleUpdate{
		{VehicleID: "v1", Lat: 59.33, Lon: 18.07},
	})

	// Should contain v2 with Removed=true.
	var removed *VehicleUpdate
	for i := range delta {
		if delta[i].VehicleID == "v2" {
			removed = &delta[i]
		}
	}

	if removed == nil {
		t.Fatal("expected removed marker for v2")
	}
	if !removed.Removed {
		t.Error("expected Removed=true for v2")
	}
}

func TestDeltaEncoder_NewVehicleIncluded(t *testing.T) {
	enc := NewDeltaEncoder()

	enc.Encode([]VehicleUpdate{
		{VehicleID: "v1", Lat: 59.33, Lon: 18.07},
	})

	// v2 is new.
	delta := enc.Encode([]VehicleUpdate{
		{VehicleID: "v1", Lat: 59.33, Lon: 18.07},
		{VehicleID: "v2", Lat: 60.00, Lon: 19.00},
	})

	if len(delta) != 1 {
		t.Fatalf("expected 1 new vehicle, got %d", len(delta))
	}
	if delta[0].VehicleID != "v2" {
		t.Errorf("expected new vehicle v2, got %s", delta[0].VehicleID)
	}
}

func TestEnvelope_MsgpackSmallerThanJSON(t *testing.T) {
	env := Envelope{
		Type: "update",
		Vehicles: []VehicleUpdate{
			{VehicleID: "bus-1234", TripID: "trip-5678", RouteID: "route-A", Lat: 59.329345, Lon: 18.068573, Bearing: 127.5, Speed: 12.3},
			{VehicleID: "bus-5678", TripID: "trip-9012", RouteID: "route-B", Lat: 59.340123, Lon: 18.075432, Bearing: 45.2, Speed: 8.7},
		},
	}

	jsonData, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}

	msgpackData, err := msgpack.Marshal(env)
	if err != nil {
		t.Fatalf("msgpack marshal: %v", err)
	}

	t.Logf("JSON: %d bytes, MsgPack: %d bytes (%.0f%% smaller)",
		len(jsonData), len(msgpackData),
		(1-float64(len(msgpackData))/float64(len(jsonData)))*100)

	if len(msgpackData) >= len(jsonData) {
		t.Errorf("msgpack (%d) should be smaller than json (%d)", len(msgpackData), len(jsonData))
	}
}

func TestEnvelope_MsgpackRoundtrip(t *testing.T) {
	original := Envelope{
		Type: "update",
		Vehicles: []VehicleUpdate{
			{VehicleID: "v1", Lat: 59.33, Lon: 18.07, Bearing: 90, Speed: 10, DelaySeconds: 120},
		},
	}

	data, err := msgpack.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Envelope
	if err := msgpack.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Type != "update" {
		t.Errorf("type: got %s, want update", decoded.Type)
	}
	if len(decoded.Vehicles) != 1 {
		t.Fatalf("vehicles: got %d, want 1", len(decoded.Vehicles))
	}
	v := decoded.Vehicles[0]
	if v.VehicleID != "v1" || v.Lat != 59.33 || v.DelaySeconds != 120 {
		t.Errorf("vehicle mismatch: %+v", v)
	}
}
