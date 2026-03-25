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

	"github.com/darkiz/publictransporttracker/internal/api"
	"github.com/darkiz/publictransporttracker/internal/gtfs"
	"github.com/darkiz/publictransporttracker/internal/store"
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
		serveCmd.Parse(os.Args[2:])

		if err := runServe(dsn, *addr); err != nil {
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
  migrate               Run database migrations
  import --gtfs <file>  Import GTFS static data from a ZIP file
  serve  --addr <addr>  Start the API server (default :8080)

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

	// Ensure schema exists.
	if err := s.Migrate(context.Background()); err != nil {
		return fmt.Errorf("auto-migrate: %w", err)
	}

	imp := gtfs.NewImporter(s.DB())
	return imp.Import(context.Background(), gtfsPath)
}

func runServe(dsn string, addr string) error {
	s, err := store.New(dsn)
	if err != nil {
		return err
	}
	defer s.Close()

	// Ensure schema exists.
	if err := s.Migrate(context.Background()); err != nil {
		return fmt.Errorf("auto-migrate: %w", err)
	}

	srv := api.New(s, addr)

	// Graceful shutdown on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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
