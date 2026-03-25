package realtime

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/darkiz/publictransporttracker/internal/vehicle"
)

// UpdateHandler is a callback invoked when new vehicle updates are decoded.
// Dependency Inversion: the poller doesn't know who consumes the updates.
type UpdateHandler func(updates []vehicle.RawUpdate)

// Poller periodically fetches a GTFS-RT feed, decodes it, and delivers updates.
//
// Single Responsibility: only orchestrates fetch → decode → deliver.
// Open/Closed: fetcher, decoder, and handler are all injected interfaces.
type Poller struct {
	fetcher FeedFetcher
	decoder FeedDecoder
	handler UpdateHandler
}

// NewPoller creates a Poller with injected dependencies.
func NewPoller(fetcher FeedFetcher, decoder FeedDecoder, handler UpdateHandler) *Poller {
	return &Poller{
		fetcher: fetcher,
		decoder: decoder,
		handler: handler,
	}
}

// PollOnce fetches the feed once, decodes it, and delivers the updates.
// Exposed for testing and one-shot usage.
func (p *Poller) PollOnce(ctx context.Context, url string) error {
	data, err := p.fetcher.Fetch(ctx, url)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", url, err)
	}

	updates, err := p.decoder.DecodeVehiclePositions(data)
	if err != nil {
		return fmt.Errorf("decode %s: %w", url, err)
	}

	if len(updates) > 0 {
		p.handler(updates)
	}

	return nil
}

// Run starts the polling loop. It blocks until ctx is cancelled.
func (p *Poller) Run(ctx context.Context, url string, interval time.Duration) {
	slog.Info("poller starting", "url", url, "interval", interval)

	// Poll immediately on start, then on ticker.
	if err := p.PollOnce(ctx, url); err != nil {
		slog.Error("poll failed", "url", url, "error", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("poller stopping", "url", url)
			return
		case <-ticker.C:
			if err := p.PollOnce(ctx, url); err != nil {
				slog.Error("poll failed", "url", url, "error", err)
			}
		}
	}
}
