package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/darkiz/publictransporttracker/internal/store"
)

// Server is the HTTP/WebSocket API server.
type Server struct {
	store  *store.Store
	mux    *http.ServeMux
	server *http.Server
}

// New creates a new API server.
func New(s *store.Store, addr string) *Server {
	srv := &Server{
		store: s,
		mux:   http.NewServeMux(),
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
