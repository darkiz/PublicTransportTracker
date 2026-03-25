package realtime

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	pb "github.com/MobilityData/gtfs-realtime-bindings/golang/gtfs"
	"github.com/darkiz/publictransporttracker/internal/vehicle"
)

// ProtoDecoder decodes GTFS-RT protobuf feeds into domain types.
//
// Single Responsibility: only translates protobuf → domain types.
// It knows nothing about HTTP, filtering, or storage.
type ProtoDecoder struct{}

// DecodeVehiclePositions parses a GTFS-RT FeedMessage and extracts vehicle position updates.
func (d *ProtoDecoder) DecodeVehiclePositions(data []byte) ([]vehicle.RawUpdate, error) {
	feed := &pb.FeedMessage{}
	if err := proto.Unmarshal(data, feed); err != nil {
		return nil, fmt.Errorf("unmarshal vehicle positions: %w", err)
	}

	var updates []vehicle.RawUpdate
	for _, entity := range feed.GetEntity() {
		vp := entity.GetVehicle()
		if vp == nil || vp.GetPosition() == nil {
			continue
		}

		vehicleID := ""
		if v := vp.GetVehicle(); v != nil {
			vehicleID = v.GetId()
		}
		if vehicleID == "" {
			vehicleID = entity.GetId()
		}

		pos := vp.GetPosition()
		ts := time.Unix(int64(vp.GetTimestamp()), 0)
		if vp.GetTimestamp() == 0 {
			ts = time.Now()
		}

		updates = append(updates, vehicle.RawUpdate{
			VehicleID: vehicleID,
			TripID:    vp.GetTrip().GetTripId(),
			RouteID:   vp.GetTrip().GetRouteId(),
			Position: vehicle.Position{
				Lat:       float64(pos.GetLatitude()),
				Lon:       float64(pos.GetLongitude()),
				Bearing:   float64(pos.GetBearing()),
				Speed:     float64(pos.GetSpeed()),
				Timestamp: ts,
			},
		})
	}

	return updates, nil
}

// DecodeDelays parses a GTFS-RT TripUpdates feed and returns trip delays.
func (d *ProtoDecoder) DecodeDelays(data []byte) (map[string]int, error) {
	feed := &pb.FeedMessage{}
	if err := proto.Unmarshal(data, feed); err != nil {
		return nil, fmt.Errorf("unmarshal trip updates: %w", err)
	}

	delays := make(map[string]int)
	for _, entity := range feed.GetEntity() {
		tu := entity.GetTripUpdate()
		if tu == nil || tu.GetTrip() == nil {
			continue
		}

		tripID := tu.GetTrip().GetTripId()
		if tripID == "" {
			continue
		}

		// Use top-level delay if present. Otherwise, derive from stop time updates.
		if tu.Delay != nil {
			delays[tripID] = int(tu.GetDelay())
			continue
		}

		// Fall back to the last stop time update's arrival delay.
		stopUpdates := tu.GetStopTimeUpdate()
		if len(stopUpdates) > 0 {
			last := stopUpdates[len(stopUpdates)-1]
			if arr := last.GetArrival(); arr != nil {
				delays[tripID] = int(arr.GetDelay())
			}
		}
	}

	return delays, nil
}
