// Package broadcast handles fan-out of vehicle state updates to WebSocket clients.
package broadcast

import (
	"github.com/darkiz/publictransporttracker/internal/vehicle"
)

// VehicleUpdate is the per-vehicle payload sent to clients.
// Only changed fields are non-zero in delta mode.
type VehicleUpdate struct {
	VehicleID    string  `msgpack:"id"              json:"id"`
	TripID       string  `msgpack:"trip,omitempty"   json:"trip,omitempty"`
	RouteID      string  `msgpack:"route,omitempty"  json:"route,omitempty"`
	Lat          float64 `msgpack:"lat"              json:"lat"`
	Lon          float64 `msgpack:"lon"              json:"lon"`
	Bearing      float64 `msgpack:"brg"              json:"brg"`
	Speed        float64 `msgpack:"spd"              json:"spd"`
	DelaySeconds int     `msgpack:"dly,omitempty"    json:"dly,omitempty"`
	Removed      bool    `msgpack:"rm,omitempty"     json:"rm,omitempty"`
}

// Envelope wraps a batch of updates sent over the WebSocket.
type Envelope struct {
	Type     string          `msgpack:"type"    json:"type"`
	Vehicles []VehicleUpdate `msgpack:"vehicles" json:"vehicles"`
}

// StateToUpdate converts a vehicle.State to a VehicleUpdate.
func StateToUpdate(s vehicle.State) VehicleUpdate {
	return VehicleUpdate{
		VehicleID:    s.VehicleID,
		TripID:       s.TripID,
		RouteID:      s.RouteID,
		Lat:          s.Position.Lat,
		Lon:          s.Position.Lon,
		Bearing:      s.Position.Bearing,
		Speed:        s.Position.Speed,
		DelaySeconds: s.DelaySeconds,
	}
}
