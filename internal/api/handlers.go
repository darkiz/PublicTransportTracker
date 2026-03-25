package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Healthy(r.Context()); err != nil {
		slog.Error("health check failed", "error", err)
		http.Error(w, "unhealthy", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleListRoutes(w http.ResponseWriter, r *http.Request) {
	var routeType *int
	if rt := r.URL.Query().Get("type"); rt != "" {
		v, err := strconv.Atoi(rt)
		if err != nil {
			http.Error(w, "invalid type parameter", http.StatusBadRequest)
			return
		}
		routeType = &v
	}

	routes, err := s.store.ListRoutes(r.Context(), routeType)
	if err != nil {
		slog.Error("list routes failed", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, routes)
}

func (s *Server) handleGetRoute(w http.ResponseWriter, r *http.Request) {
	routeID := r.PathValue("routeID")
	route, err := s.store.GetRoute(r.Context(), routeID)
	if err != nil {
		slog.Error("get route failed", "error", err, "route_id", routeID)
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	writeJSON(w, route)
}

func (s *Server) handleGetRouteShape(w http.ResponseWriter, r *http.Request) {
	routeID := r.PathValue("routeID")
	geojson, err := s.store.GetRouteShape(r.Context(), routeID)
	if err != nil {
		slog.Error("get route shape failed", "error", err, "route_id", routeID)
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/geo+json")
	w.Write(geojson)
}

func (s *Server) handleGetAllShapes(w http.ResponseWriter, r *http.Request) {
	fc, err := s.store.GetAllRouteShapes(r.Context())
	if err != nil {
		slog.Error("get all shapes failed", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/geo+json")
	writeJSON(w, fc)
}

func (s *Server) handleListStops(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	bbox := q.Get("bbox")
	if bbox == "" {
		http.Error(w, "bbox parameter required (minLon,minLat,maxLon,maxLat)", http.StatusBadRequest)
		return
	}

	coords := splitBBox(bbox)
	if coords == nil {
		http.Error(w, "invalid bbox format, expected minLon,minLat,maxLon,maxLat", http.StatusBadRequest)
		return
	}

	fc, err := s.store.ListStopsByBBox(r.Context(), coords[0], coords[1], coords[2], coords[3])
	if err != nil {
		slog.Error("list stops failed", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/geo+json")
	writeJSON(w, fc)
}

func (s *Server) handleGetTripStopTimes(w http.ResponseWriter, r *http.Request) {
	tripID := r.PathValue("tripID")
	stopTimes, err := s.store.GetTripStopTimes(r.Context(), tripID)
	if err != nil {
		slog.Error("get trip stop times failed", "error", err, "trip_id", tripID)
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	writeJSON(w, stopTimes)
}

func (s *Server) handleListVehicles(w http.ResponseWriter, r *http.Request) {
	if s.vehicles == nil {
		writeJSON(w, []any{})
		return
	}

	writeJSON(w, s.vehicles.All())
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write json response failed", "error", err)
	}
}

func splitBBox(bbox string) []float64 {
	parts := make([]float64, 0, 4)
	for _, s := range split(bbox, ',') {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil
		}
		parts = append(parts, v)
	}
	if len(parts) != 4 {
		return nil
	}
	return parts
}

func split(s string, sep byte) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}
