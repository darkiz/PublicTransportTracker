# Project Goals — PublicTransportTracker

## Vision

Build a real-time public transit vehicle tracker that displays buses, trains, trams, and metro vehicles moving smoothly on an interactive map. Users open a web app, see vehicles gliding along their routes in real time, and can tap any vehicle to see delay information, route details, and next stops.

The system ingests standardized GTFS and GTFS-RT data feeds, applies GPS smoothing algorithms, and delivers sub-second updates to thousands of simultaneous users via WebSocket.

## MVP Scope

The MVP targets a single transit agency and proves the core technical thesis: **smooth 60fps vehicle animation from 3–15 second server update intervals**.

### In Scope (MVP)

| Feature | Description |
|---|---|
| **GTFS Static Import** | Parse routes, stops, shapes, trips, stop_times, calendar from a GTFS ZIP feed. Store in PostGIS. |
| **GTFS-RT Ingestion** | Poll a single agency's GTFS-RT VehiclePositions and TripUpdates feeds at their recommended interval. |
| **Kalman Filtering** | Apply a per-vehicle Kalman filter to smooth noisy GPS positions and estimate velocity/heading. |
| **Route Snapping** | Snap filtered positions to the nearest point on the vehicle's assigned route shape. |
| **WebSocket Delivery** | Broadcast vehicle state updates to connected clients via WebSocket with delta compression. |
| **Interactive Map** | Svelte 5 web app with MapLibre GL JS base map and deck.gl vehicle layer. |
| **Client-Side Interpolation** | Animate vehicles between server updates using estimated velocity/heading for 60fps visual smoothness. |
| **Modality Filter** | Toggle visibility of vehicle types: bus, tram, train, metro, ferry. |
| **Vehicle Detail Panel** | Tap a vehicle to see: route name, direction, delay (early/late/on-time), next stops with predicted times. |
| **Delay Indicators** | Color-code vehicles by delay status (green = on time, yellow = slight delay, red = significant delay). |

### Out of Scope (MVP)

These are explicitly deferred — not forgotten, but not needed to prove the core product:

- **Multi-agency support** — The architecture supports it, but MVP targets one agency to reduce complexity.
- **Trip planning / routing** — This is a viewer, not a journey planner. Tools like Google Maps already do this well.
- **User accounts / authentication** — The app is public and read-only. No login needed.
- **Historical analytics** — "How late was this route last month?" is a different product.
- **Predictive ML** — Arrival prediction beyond GTFS-RT's own TripUpdate predictions.
- **Push notifications** — "Alert me when bus 42 is 5 minutes away."
- **Mobile native apps** — The web app works on mobile browsers. Native apps come later if demand warrants.
- **Kubernetes / multi-region deployment** — A single VPS handles the MVP workload. Container orchestration is premature.
- **Service alerts display** — GTFS-RT ServiceAlerts (detours, cancellations) are Phase 6+.
- **Occupancy / crowding data** — Requires OccupancyStatus in GTFS-RT, which few agencies provide.

## MVP Exit Criteria

The MVP is "done" when a user can:

1. Open the web app in a browser (desktop or mobile).
2. See a map centered on the transit agency's coverage area.
3. See vehicle icons moving smoothly along their routes in real time.
4. Filter vehicles by type (bus, train, tram, etc.).
5. Tap a vehicle and see its route, direction, delay status, and upcoming stops.
6. Experience smooth 60fps animation even though the server sends updates every 3–15 seconds.
7. The system handles at least 5,000 vehicles and 1,000 concurrent WebSocket clients without degradation.

## Target Performance Metrics

| Metric | Target |
|---|---|
| Vehicle count supported | 5,000+ |
| Concurrent WebSocket clients | 10,000+ |
| Server update → client display latency | < 500ms |
| Client-side animation frame rate | 60fps |
| GTFS-RT poll frequency | Agency-recommended (typically 10–30s) |
| Server memory usage | < 1GB for full vehicle state |
| Cold start time | < 10 seconds |

## Design Principles

1. **Simplicity over sophistication** — A modular monolith that works beats a microservice architecture that's half-built. Ship first, optimize later.
2. **Data quality matters** — The Kalman filter and route snapping are the core differentiators. Invest time here, not in infrastructure plumbing.
3. **Client does the animation** — The server sends state snapshots; the client interpolates between them. This keeps server bandwidth manageable and animation smooth.
4. **Standards-based** — GTFS/GTFS-RT are the universal transit data formats. Building on standards means any agency can be added.
5. **Observable** — Structured logging, health checks, and metrics from day one. A real-time system you can't debug is a real-time system you can't trust.
