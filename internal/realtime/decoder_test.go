package realtime

import (
	"testing"

	"google.golang.org/protobuf/proto"

	pb "github.com/MobilityData/gtfs-realtime-bindings/golang/gtfs"
)

// TestProtoDecoderImplementsInterface verifies compile-time contract.
func TestProtoDecoderImplementsInterface(t *testing.T) {
	var _ FeedDecoder = (*ProtoDecoder)(nil)
}

func TestDecodeVehiclePositions_Empty(t *testing.T) {
	dec := &ProtoDecoder{}

	feed := &pb.FeedMessage{
		Header: &pb.FeedHeader{
			GtfsRealtimeVersion: proto.String("2.0"),
		},
		Entity: []*pb.FeedEntity{},
	}

	data, err := proto.Marshal(feed)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	updates, err := dec.DecodeVehiclePositions(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(updates) != 0 {
		t.Errorf("expected 0 updates, got %d", len(updates))
	}
}

func TestDecodeVehiclePositions_SingleVehicle(t *testing.T) {
	dec := &ProtoDecoder{}

	feed := buildVehicleFeed([]vehicleFixture{
		{id: "bus-1", tripID: "trip-100", routeID: "route-A", lat: 59.33, lon: 18.07, bearing: 90, speed: 10, timestamp: 1700000000},
	})

	data, err := proto.Marshal(feed)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	updates, err := dec.DecodeVehiclePositions(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}

	u := updates[0]
	if u.VehicleID != "bus-1" {
		t.Errorf("expected vehicle_id bus-1, got %s", u.VehicleID)
	}
	if u.TripID != "trip-100" {
		t.Errorf("expected trip_id trip-100, got %s", u.TripID)
	}
	if u.RouteID != "route-A" {
		t.Errorf("expected route_id route-A, got %s", u.RouteID)
	}
	// Protobuf uses float32, so compare with tolerance for float32→float64 conversion.
	if diff := u.Position.Lat - 59.33; diff < -0.001 || diff > 0.001 {
		t.Errorf("expected lat ~59.33, got %f", u.Position.Lat)
	}
	if diff := u.Position.Lon - 18.07; diff < -0.001 || diff > 0.001 {
		t.Errorf("expected lon ~18.07, got %f", u.Position.Lon)
	}
	if diff := u.Position.Speed - 10; diff < -0.01 || diff > 0.01 {
		t.Errorf("expected speed ~10, got %f", u.Position.Speed)
	}
}

func TestDecodeVehiclePositions_MultipleVehicles(t *testing.T) {
	dec := &ProtoDecoder{}

	feed := buildVehicleFeed([]vehicleFixture{
		{id: "v1", tripID: "t1", routeID: "r1", lat: 59.33, lon: 18.07, timestamp: 1700000000},
		{id: "v2", tripID: "t2", routeID: "r2", lat: 60.00, lon: 19.00, timestamp: 1700000000},
		{id: "v3", tripID: "t3", routeID: "r3", lat: 57.70, lon: 11.97, timestamp: 1700000000},
	})

	data, err := proto.Marshal(feed)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	updates, err := dec.DecodeVehiclePositions(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(updates) != 3 {
		t.Fatalf("expected 3 updates, got %d", len(updates))
	}
}

func TestDecodeVehiclePositions_SkipsMissingPosition(t *testing.T) {
	dec := &ProtoDecoder{}

	// Entity with Vehicle but no Position.
	feed := &pb.FeedMessage{
		Header: &pb.FeedHeader{
			GtfsRealtimeVersion: proto.String("2.0"),
		},
		Entity: []*pb.FeedEntity{
			{
				Id: proto.String("e1"),
				Vehicle: &pb.VehiclePosition{
					Vehicle: &pb.VehicleDescriptor{Id: proto.String("v1")},
					// No Position field.
				},
			},
		},
	}

	data, err := proto.Marshal(feed)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	updates, err := dec.DecodeVehiclePositions(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(updates) != 0 {
		t.Errorf("expected 0 updates for missing position, got %d", len(updates))
	}
}

func TestDecodeVehiclePositions_InvalidProtobuf(t *testing.T) {
	dec := &ProtoDecoder{}

	_, err := dec.DecodeVehiclePositions([]byte("not a protobuf"))
	if err == nil {
		t.Error("expected error for invalid protobuf")
	}
}

func TestDecodeDelays(t *testing.T) {
	dec := &ProtoDecoder{}

	feed := &pb.FeedMessage{
		Header: &pb.FeedHeader{
			GtfsRealtimeVersion: proto.String("2.0"),
		},
		Entity: []*pb.FeedEntity{
			{
				Id: proto.String("e1"),
				TripUpdate: &pb.TripUpdate{
					Trip: &pb.TripDescriptor{TripId: proto.String("trip-100")},
					Delay: proto.Int32(120), // 2 minutes late
				},
			},
			{
				Id: proto.String("e2"),
				TripUpdate: &pb.TripUpdate{
					Trip: &pb.TripDescriptor{TripId: proto.String("trip-200")},
					Delay: proto.Int32(-30), // 30 seconds early
				},
			},
		},
	}

	data, err := proto.Marshal(feed)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	delays, err := dec.DecodeDelays(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if delays["trip-100"] != 120 {
		t.Errorf("expected delay 120 for trip-100, got %d", delays["trip-100"])
	}
	if delays["trip-200"] != -30 {
		t.Errorf("expected delay -30 for trip-200, got %d", delays["trip-200"])
	}
}

// --- test helpers ---

type vehicleFixture struct {
	id, tripID, routeID         string
	lat, lon, bearing, speed    float32
	timestamp                   uint64
}

func buildVehicleFeed(vehicles []vehicleFixture) *pb.FeedMessage {
	entities := make([]*pb.FeedEntity, len(vehicles))
	for i, v := range vehicles {
		entities[i] = &pb.FeedEntity{
			Id: proto.String(v.id),
			Vehicle: &pb.VehiclePosition{
				Vehicle: &pb.VehicleDescriptor{Id: proto.String(v.id)},
				Trip:    &pb.TripDescriptor{TripId: proto.String(v.tripID), RouteId: proto.String(v.routeID)},
				Position: &pb.Position{
					Latitude:  proto.Float32(v.lat),
					Longitude: proto.Float32(v.lon),
					Bearing:   proto.Float32(v.bearing),
					Speed:     proto.Float32(v.speed),
				},
				Timestamp: proto.Uint64(v.timestamp),
			},
		}
	}

	return &pb.FeedMessage{
		Header: &pb.FeedHeader{
			GtfsRealtimeVersion: proto.String("2.0"),
		},
		Entity: entities,
	}
}
