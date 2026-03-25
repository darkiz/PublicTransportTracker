package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/darkiz/publictransporttracker/internal/broadcast"
	"github.com/darkiz/publictransporttracker/internal/store"
	"github.com/darkiz/publictransporttracker/internal/vehicle"
)

// Server is the HTTP/WebSocket API server.
type Server struct {
	store    *store.Store
	vehicles vehicle.StateManager
	hub      *broadcast.Hub
	mux      *http.ServeMux
	server   *http.Server
}

// New creates a new API server.
func New(s *store.Store, vehicles vehicle.StateManager, hub *broadcast.Hub, addr string) *Server {
	srv := &Server{
		store:    s,
		vehicles: vehicles,
		hub:      hub,
		mux:      http.NewServeMux(),
	}
	srv.routes()
	srv.server = &http.Server{
		Addr:         addr,
		Handler:      srv.mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	return srv
}

// routes registers all HTTP routes.
func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /api/routes", s.handleListRoutes)
	s.mux.HandleFunc("GET /api/routes/{routeID}", s.handleGetRoute)
	s.mux.HandleFunc("GET /api/routes/{routeID}/shape", s.handleGetRouteShape)
	s.mux.HandleFunc("GET /api/shapes", s.handleGetAllShapes)
	s.mux.HandleFunc("GET /api/stops", s.handleListStops)
	s.mux.HandleFunc("GET /api/trips/{tripID}/stop-times", s.handleGetTripStopTimes)
	s.mux.HandleFunc("GET /api/vehicles", s.handleListVehicles)
	s.mux.HandleFunc("/ws", s.handleWebSocket)
}

// Hub returns the broadcast hub for external use (e.g., broadcast loop).
func (s *Server) Hub() *broadcast.Hub {
	return s.hub
}

// Start begins listening for HTTP connections.
func (s *Server) Start() error {
	slog.Info("API server starting", "addr", s.server.Addr)
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}
