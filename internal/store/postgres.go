package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"

	_ "github.com/lib/pq"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store provides access to the PostgreSQL/PostGIS database.
type Store struct {
	db *sql.DB
}

// New creates a new Store and verifies the connection.
func New(dsn string) (*Store, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying *sql.DB for use in transactions.
func (s *Store) DB() *sql.DB {
	return s.db
}

// Migrate runs all SQL migration files in order.
func (s *Store) Migrate(ctx context.Context) error {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		slog.Info("running migration", "file", entry.Name())
		if _, err := s.db.ExecContext(ctx, string(data)); err != nil {
			return fmt.Errorf("execute migration %s: %w", entry.Name(), err)
		}
	}

	return nil
}

// Healthy checks if the database connection is alive and PostGIS is available.
func (s *Store) Healthy(ctx context.Context) error {
	var version string
	err := s.db.QueryRowContext(ctx, "SELECT PostGIS_Version()").Scan(&version)
	if err != nil {
		return fmt.Errorf("postgis health check: %w", err)
	}
	return nil
}
