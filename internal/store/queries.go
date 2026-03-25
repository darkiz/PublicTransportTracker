package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/darkiz/publictransporttracker/internal/gtfs"
)

// GeoJSONFeature represents a single GeoJSON feature.
type GeoJSONFeature struct {
	Type       string          `json:"type"`
	Properties json.RawMessage `json:"properties"`
	Geometry   json.RawMessage `json:"geometry"`
}

// GeoJSONCollection represents a GeoJSON FeatureCollection.
type GeoJSONCollection struct {
	Type     string           `json:"type"`
	Features []GeoJSONFeature `json:"features"`
}

// ListRoutes returns all routes, optionally filtered by route_type.
func (s *Store) ListRoutes(ctx context.Context, routeType *int) ([]gtfs.Route, error) {
	query := `SELECT route_id, COALESCE(agency_id,''), short_name, long_name, route_type,
	                 COALESCE(color,''), COALESCE(text_color,''), sort_order
	          FROM routes`
	args := []any{}
	if routeType != nil {
		query += " WHERE route_type = $1"
		args = append(args, *routeType)
	}
	query += " ORDER BY sort_order, short_name"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list routes: %w", err)
	}
	defer rows.Close()

	var routes []gtfs.Route
	for rows.Next() {
		var r gtfs.Route
		if err := rows.Scan(&r.ID, &r.AgencyID, &r.ShortName, &r.LongName, &r.Type,
			&r.Color, &r.TextColor, &r.SortOrder); err != nil {
			return nil, fmt.Errorf("scan route: %w", err)
		}
		routes = append(routes, r)
	}
	return routes, rows.Err()
}

// GetRoute returns a single route by ID.
func (s *Store) GetRoute(ctx context.Context, routeID string) (*gtfs.Route, error) {
	var r gtfs.Route
	err := s.db.QueryRowContext(ctx,
		`SELECT route_id, COALESCE(agency_id,''), short_name, long_name, route_type,
		        COALESCE(color,''), COALESCE(text_color,''), sort_order
		 FROM routes WHERE route_id = $1`, routeID,
	).Scan(&r.ID, &r.AgencyID, &r.ShortName, &r.LongName, &r.Type, &r.Color, &r.TextColor, &r.SortOrder)
	if err != nil {
		return nil, fmt.Errorf("get route %s: %w", routeID, err)
	}
	return &r, nil
}

// GetRouteShape returns the shape geometry for a route as GeoJSON.
func (s *Store) GetRouteShape(ctx context.Context, routeID string) (json.RawMessage, error) {
	var geojson string
	err := s.db.QueryRowContext(ctx,
		`SELECT ST_AsGeoJSON(s.geom)
		 FROM shapes s
		 JOIN trips t ON t.shape_id = s.shape_id
		 WHERE t.route_id = $1
		 LIMIT 1`, routeID,
	).Scan(&geojson)
	if err != nil {
		return nil, fmt.Errorf("get shape for route %s: %w", routeID, err)
	}
	return json.RawMessage(geojson), nil
}

// GetAllRouteShapes returns all shapes as a GeoJSON FeatureCollection, with route metadata.
func (s *Store) GetAllRouteShapes(ctx context.Context) (*GeoJSONCollection, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT ON (r.route_id)
		        r.route_id, r.short_name, r.long_name, r.route_type,
		        COALESCE(r.color, '000000'), ST_AsGeoJSON(s.geom)
		 FROM routes r
		 JOIN trips t ON t.route_id = r.route_id
		 JOIN shapes s ON s.shape_id = t.shape_id
		 ORDER BY r.route_id`)
	if err != nil {
		return nil, fmt.Errorf("get all route shapes: %w", err)
	}
	defer rows.Close()

	fc := &GeoJSONCollection{Type: "FeatureCollection"}
	for rows.Next() {
		var routeID, shortName, longName, color, geomJSON string
		var routeType int
		if err := rows.Scan(&routeID, &shortName, &longName, &routeType, &color, &geomJSON); err != nil {
			return nil, fmt.Errorf("scan route shape: %w", err)
		}
		props, _ := json.Marshal(map[string]any{
			"route_id":   routeID,
			"short_name": shortName,
			"long_name":  longName,
			"route_type": routeType,
			"modality":   gtfs.RouteTypeName(routeType),
			"color":      "#" + color,
		})
		fc.Features = append(fc.Features, GeoJSONFeature{
			Type:       "Feature",
			Properties: props,
			Geometry:   json.RawMessage(geomJSON),
		})
	}
	return fc, rows.Err()
}

// ListStopsByBBox returns stops within a bounding box as GeoJSON.
func (s *Store) ListStopsByBBox(ctx context.Context, minLon, minLat, maxLon, maxLat float64) (*GeoJSONCollection, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT stop_id, stop_name, COALESCE(stop_code, ''), location_type,
		        ST_AsGeoJSON(geom)
		 FROM stops
		 WHERE geom && ST_MakeEnvelope($1, $2, $3, $4, 4326)
		 ORDER BY stop_name`, minLon, minLat, maxLon, maxLat)
	if err != nil {
		return nil, fmt.Errorf("list stops by bbox: %w", err)
	}
	defer rows.Close()

	fc := &GeoJSONCollection{Type: "FeatureCollection"}
	for rows.Next() {
		var id, name, code, geomJSON string
		var locType int
		if err := rows.Scan(&id, &name, &code, &locType, &geomJSON); err != nil {
			return nil, fmt.Errorf("scan stop: %w", err)
		}
		props, _ := json.Marshal(map[string]any{
			"stop_id":       id,
			"stop_name":     name,
			"stop_code":     code,
			"location_type": locType,
		})
		fc.Features = append(fc.Features, GeoJSONFeature{
			Type:       "Feature",
			Properties: props,
			Geometry:   json.RawMessage(geomJSON),
		})
	}
	return fc, rows.Err()
}

// GetTripStopTimes returns ordered stop times for a trip.
func (s *Store) GetTripStopTimes(ctx context.Context, tripID string) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT st.stop_sequence, st.arrival_time, st.departure_time,
		        s.stop_id, s.stop_name, ST_Y(s.geom) as lat, ST_X(s.geom) as lon
		 FROM stop_times st
		 JOIN stops s ON s.stop_id = st.stop_id
		 WHERE st.trip_id = $1
		 ORDER BY st.stop_sequence`, tripID)
	if err != nil {
		return nil, fmt.Errorf("get stop times for trip %s: %w", tripID, err)
	}
	defer rows.Close()

	var result []map[string]any
	for rows.Next() {
		var seq int
		var arrival, departure, stopID, stopName string
		var lat, lon float64
		if err := rows.Scan(&seq, &arrival, &departure, &stopID, &stopName, &lat, &lon); err != nil {
			return nil, fmt.Errorf("scan stop time: %w", err)
		}
		result = append(result, map[string]any{
			"stop_sequence":  seq,
			"arrival_time":   arrival,
			"departure_time": departure,
			"stop_id":        stopID,
			"stop_name":      stopName,
			"lat":            lat,
			"lon":            lon,
		})
	}
	return result, rows.Err()
}
