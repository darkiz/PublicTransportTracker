package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/darkiz/publictransporttracker/internal/api"
	"github.com/darkiz/publictransporttracker/internal/broadcast"
	"github.com/darkiz/publictransporttracker/internal/gtfs"
	"github.com/darkiz/publictransporttracker/internal/realtime"
	"github.com/darkiz/publictransporttracker/internal/store"
	"github.com/darkiz/publictransporttracker/internal/vehicle"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	dsn := envOr("DATABASE_URL", "postgres://transit:transit@localhost:5432/transit?sslmode=disable")

	switch os.Args[1] {
	case "migrate":
		if err := runMigrate(dsn); err != nil {
			slog.Error("migration failed", "error", err)
			os.Exit(1)
		}
	case "import":
		importCmd := flag.NewFlagSet("import", flag.ExitOnError)
		gtfsPath := importCmd.String("gtfs", "", "path to GTFS ZIP file")
		importCmd.Parse(os.Args[2:])

		if *gtfsPath == "" {
			fmt.Fprintln(os.Stderr, "Usage: tracker import --gtfs <path/to/gtfs.zip>")
			os.Exit(1)
		}

		if err := runImport(dsn, *gtfsPath); err != nil {
			slog.Error("import failed", "error", err)
			os.Exit(1)
		}
	case "serve":
		serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
		addr := serveCmd.String("addr", ":8080", "HTTP listen address")
		feedURL := serveCmd.String("feed-url", "", "GTFS-RT VehiclePositions feed URL")
		pollInterval := serveCmd.Int("poll-interval", 15, "Feed poll interval in seconds")
		serveCmd.Parse(os.Args[2:])

		if err := runServe(dsn, *addr, *feedURL, *pollInterval); err != nil {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `Usage: tracker <command> [flags]

Commands:
  migrate                      Run database migrations
  import --gtfs <file>         Import GTFS static data from a ZIP file
  serve  [flags]               Start the API server

Serve flags:
  --addr <addr>                HTTP listen address (default :8080)
  --feed-url <url>             GTFS-RT VehiclePositions feed URL
  --poll-interval <seconds>    Feed poll interval (default 15)

Environment:
  DATABASE_URL  PostgreSQL connection string
                (default: postgres://transit:transit@localhost:5432/transit?sslmode=disable)`)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func runMigrate(dsn string) error {
	s, err := store.New(dsn)
	if err != nil {
		return err
	}
	defer s.Close()

	return s.Migrate(context.Background())
}

func runImport(dsn string, gtfsPath string) error {
	s, err := store.New(dsn)
	if err != nil {
		return err
	}
	defer s.Close()

	if err := s.Migrate(context.Background()); err != nil {
		return fmt.Errorf("auto-migrate: %w", err)
	}

	imp := gtfs.NewImporter(s.DB())
	return imp.Import(context.Background(), gtfsPath)
}

func runServe(dsn string, addr string, feedURL string, pollIntervalSec int) error {
	s, err := store.New(dsn)
	if err != nil {
		return err
	}
	defer s.Close()

	if err := s.Migrate(context.Background()); err != nil {
		return fmt.Errorf("auto-migrate: %w", err)
	}

	// Build the real-time pipeline.
	snapper := vehicle.NewInMemorySnapper()
	stateMgr := vehicle.NewDefaultStateManager(
		func() vehicle.PositionFilter { return vehicle.NewKalmanFilter() },
		snapper,
	)

	hub := broadcast.NewHub()
	srv := api.New(s, stateMgr, hub, addr)

	// Graceful shutdown on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start GTFS-RT poller if feed URL is configured.
	if feedURL != "" {
		fetcher := realtime.NewHTTPFetcher()
		decoder := &realtime.ProtoDecoder{}
		handler := func(updates []vehicle.RawUpdate) {
			for _, u := range updates {
				stateMgr.Update(u)
			}
		}
		poller := realtime.NewPoller(fetcher, decoder, handler)

		interval := time.Duration(pollIntervalSec) * time.Second
		go poller.Run(ctx, feedURL, interval)

		// Periodically prune stale vehicles.
		go func() {
			ticker := time.NewTicker(1 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					pruned := stateMgr.Prune(time.Now().Add(-5 * time.Minute))
					if pruned > 0 {
						slog.Info("pruned stale vehicles", "count", pruned)
					}
				}
			}
		}()

		slog.Info("real-time pipeline enabled", "feed_url", feedURL, "poll_interval", interval)
	} else {
		slog.Warn("no --feed-url specified, real-time pipeline disabled")
	}

	// Broadcast loop: periodically push delta-compressed state to WebSocket clients.
	go func() {
		deltaEnc := broadcast.NewDeltaEncoder()
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if hub.ClientCount() == 0 {
					continue
				}
				// Build current state and compute delta.
				all := stateMgr.All()
				updates := make([]broadcast.VehicleUpdate, len(all))
				for i, s := range all {
					updates[i] = broadcast.StateToUpdate(s)
				}
				delta := deltaEnc.Encode(updates)
				if len(delta) == 0 {
					continue
				}
				hub.Broadcast(broadcast.Envelope{
					Type:     "update",
					Vehicles: delta,
				})
			}
		}
	}()

	errCh := make(chan error, 1)
	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("shutting down server...")
		return srv.Shutdown(context.Background())
	}
}
