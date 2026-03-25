# Implementation Phases — PublicTransportTracker

## Why Order Matters

Each phase builds on the previous one. The ordering is not arbitrary — it follows the **data dependency chain**: you can't filter GPS data you haven't ingested, you can't broadcast updates you haven't filtered, and you can't animate vehicles you haven't received. The phases also prioritize **core differentiators** (Kalman filter, route snapping, 60fps animation) over infrastructure concerns (CI/CD, monitoring).

---

## Phase 1: Foundation — GTFS Static Data

**Priority: Highest**
**Estimated effort: 1 week**

### What

- Project scaffolding (`go.mod`, directory structure, Docker Compose with PostGIS)
- PostgreSQL/PostGIS schema for GTFS static data (routes, stops, shapes, trips, stop_times, calendar)
- GTFS ZIP parser that reads CSV files and loads them into the database
- Basic REST API for querying routes, stops, and shapes
- Database migrations setup

### Why This Comes First

**Everything downstream depends on static data.** The route snapper needs shape geometries to snap GPS points to. The delay calculator needs scheduled stop_times to compare against real-time data. The frontend needs route paths to draw on the map. Without GTFS static data loaded and queryable, no other component can function.

This phase also validates the core data model. If the schema is wrong, fixing it later means migrating data and updating every component that reads from it.

### Key Files

```
cmd/tracker/main.go
internal/gtfs/importer.go
internal/gtfs/models.go
internal/store/postgres.go
internal/store/migrations/
internal/api/server.go
internal/api/handlers.go
docker-compose.yml
```

### Exit Criteria

- `go run ./cmd/tracker import --gtfs path/to/gtfs.zip` loads all GTFS tables into PostGIS
- `GET /api/routes` returns routes with basic metadata
- `GET /api/routes/:id/shape` returns GeoJSON LineString for a route shape
- `GET /api/stops?bbox=...` returns stops within a bounding box
- PostGIS spatial indexes are in place and performant

---

## Phase 2: Real-Time Pipeline — The Core Engine

**Priority: Critical Path**
**Estimated effort: 1.5–2 weeks**

### What

- GTFS-RT poller: HTTP client that fetches VehiclePositions and TripUpdates protobuf feeds on a configurable interval
- Protobuf decoder: Unmarshal GTFS-RT protobuf into Go structs
- Vehicle state manager: Concurrent map holding per-vehicle state (position, velocity, heading, filter state, trip assignment)
- Kalman filter: Per-vehicle extended Kalman filter (state: lat, lon, velocity, heading) that smooths noisy GPS
- Route snapper: Given a filtered position and a trip's shape, snap to the nearest point on the LineString
- Go channel pipeline: poller → decoder → state manager (filter + snap)

### Why This Comes Second

**This is the core value proposition.** A transit tracker without real-time data is just a static map. The Kalman filter and route snapper are what differentiate this from "just plot raw GPS dots on a map" — they produce smooth, believable vehicle motion.

This phase is also the most algorithmically complex. The Kalman filter requires careful tuning of process noise and measurement noise covariances. Route snapping needs efficient PostGIS queries or in-memory spatial indexing. Getting this right early means the rest of the system works with high-quality data.

### Why Before WebSocket/Frontend

It's tempting to build the frontend first to "see something." But without the filtering pipeline, you'd see jerky, noisy GPS dots jumping between updates — which would require reworking the frontend interpolation logic once filtered data arrives. Building the pipeline first means the frontend always works with clean data.

### Key Files

```
internal/realtime/poller.go
internal/realtime/decoder.go
internal/vehicle/state.go
internal/vehicle/kalman.go
internal/vehicle/snapper.go
```

### Exit Criteria

- Poller fetches GTFS-RT feed and decodes vehicle positions successfully
- Kalman filter smooths raw GPS positions (visible improvement in logged position traces)
- Route snapper constrains vehicles to their assigned route shapes
- Vehicle state manager holds 5,000+ vehicles concurrently without contention
- Pipeline processes 2,000 updates/second with <10ms end-to-end latency
- Unit tests for Kalman filter with known input/output pairs

---

## Phase 3: Client Delivery — WebSocket and API

**Priority: High**
**Estimated effort: 1 week**

### What

- WebSocket hub: fan-out pattern that broadcasts vehicle state updates to all connected clients
- Delta compression: Only send changed fields (position moved, delay changed) rather than full vehicle state every time
- MessagePack encoding for WebSocket binary frames
- REST snapshot endpoint: `GET /api/vehicles` returns current state of all vehicles (for initial page load)
- Connection lifecycle: upgrade, heartbeat/ping-pong, clean disconnect, backpressure handling

### Why This Comes Third

**This is the bridge between backend and frontend.** Phases 1–2 produce clean, filtered vehicle data flowing through Go channels. Phase 3 makes that data available to browser clients. Without this, the data stays trapped in the server process.

### Why After the Pipeline, Before the Frontend

The WebSocket protocol design (message format, delta encoding, update frequency) must be stable before the frontend is built against it. If the message format changes, both server and client code need updating. Getting the protocol right in Phase 3 means the frontend in Phase 4 can build against a stable contract.

### Key Files

```
internal/broadcast/hub.go
internal/broadcast/encoder.go
internal/api/ws_handler.go
```

### Exit Criteria

- WebSocket endpoint at `ws://localhost:8080/ws` accepts connections
- Connected clients receive MessagePack-encoded vehicle position updates
- Delta compression reduces bandwidth by >50% compared to full state
- Hub handles 1,000+ concurrent connections without dropping messages
- Graceful degradation under backpressure (slow clients get dropped, not blocking)
- `GET /api/vehicles` returns current snapshot for initial state hydration

---

## Phase 4: Frontend — The User-Facing Application

**Priority: High**
**Estimated effort: 1.5–2 weeks**

### What

- Svelte 5 app scaffolding (Vite, TypeScript, project structure)
- MapLibre GL JS base map with tile source configuration
- deck.gl `IconLayer` for vehicle rendering (oriented icons per vehicle type)
- WebSocket client: connect, receive MessagePack updates, maintain client-side vehicle state
- Client-side interpolation engine: animate vehicles between server updates using velocity and heading estimates for 60fps
- Modality filter: toggle bus/tram/train/metro/ferry visibility
- Vehicle detail panel: tap a vehicle to see route, direction, delay, next stops
- Delay color coding: green (on time), yellow (1–5 min late), red (5+ min late)
- Responsive layout: works on desktop and mobile browsers

### Why This Comes Fourth

**Depends on all previous phases.** The frontend needs:
- Phase 1: Route shapes to draw on the map, stop locations for the detail panel
- Phase 2: Filtered vehicle data for smooth animation (raw GPS would look terrible)
- Phase 3: WebSocket endpoint to receive real-time updates

Building the frontend without these produces a demo that needs to be reworked once real data flows. Building it last means every feature works against production-quality data from day one.

### Why the Interpolation Engine Is the Hardest Part

The server sends updates every 3–15 seconds (depending on the GTFS-RT feed). The screen refreshes at 60fps (every 16.67ms). The interpolation engine must:

1. Receive a new server position for a vehicle
2. Calculate the delta from the vehicle's current rendered position
3. Smoothly animate the vehicle along the route shape to the new position
4. Handle edge cases: vehicle jumps (GPS correction), vehicle stops (at a bus stop), vehicle disappears (trip ended)
5. Predict forward motion when the next server update is late

This is the most algorithmically challenging frontend component and the most visible to users. It deserves dedicated focus.

### Key Files

```
frontend/src/App.svelte
frontend/src/lib/map/Map.svelte
frontend/src/lib/map/VehicleLayer.svelte
frontend/src/lib/stores/vehicles.svelte.ts
frontend/src/lib/interpolation/engine.ts
frontend/src/lib/ws/client.ts
frontend/src/lib/components/VehicleDetail.svelte
frontend/src/lib/components/ModalityFilter.svelte
```

### Exit Criteria

- Map displays with vector tiles, centered on the agency's coverage area
- Vehicle icons appear and move smoothly along routes
- Animation is 60fps with no visible jank during position updates
- Modality filter toggles vehicle types on/off
- Tapping a vehicle shows route, direction, delay, and next stops
- Vehicles are color-coded by delay status
- Works on Chrome, Firefox, Safari (desktop and mobile)

---

## Phase 5: DevOps — Production Readiness

**Priority: Medium**
**Estimated effort: 0.5–1 week**

### What

- Dockerfile for the Go binary (multi-stage build)
- Docker Compose with PostgreSQL/PostGIS, Go app, and frontend static serve (or embedded in Go binary)
- Health check endpoint (`/healthz`) that verifies DB connectivity and feed freshness
- Structured logging (JSON format with trace IDs)
- Prometheus metrics: vehicle count, WebSocket connection count, feed poll latency, Kalman filter processing time
- Graceful shutdown: drain WebSocket connections, finish in-flight updates, close DB connections
- CI pipeline: lint, test, build, Docker image push
- Basic alerting: feed stopped updating, WebSocket connection count dropped to zero

### Why This Comes Fifth

**Observability and deployment tooling don't create user value, but they protect it.** The first four phases build the product. This phase ensures you can deploy it reliably, know when it's broken, and debug issues without SSH-ing into the server.

It comes after the product is functional because monitoring a broken system is useless, but running a working system without monitoring is dangerous only once users depend on it.

### Why Not Earlier

Some teams front-load CI/CD and monitoring. For a 1-person MVP, this slows velocity without proportionate benefit. It's faster to run `go test` and `docker compose up` locally during development. Once the system works end-to-end, then invest in the operational shell around it.

### Exit Criteria

- `docker compose up` starts the full system from scratch
- `/healthz` returns 200 when healthy, 503 when degraded
- Structured logs include timestamp, level, component, and relevant IDs
- Prometheus metrics are exposed at `/metrics`
- CI pipeline passes on every push

---

## Future Phases (Post-MVP)

These are ordered by estimated user value, but the actual order should be driven by user feedback after MVP launch.

### Phase 6: Multi-Agency Support

- Configuration-driven agency list (feed URLs, poll intervals, bounding boxes)
- Per-agency GTFS import scheduling
- Agency selector in the frontend
- Aggregate view (all agencies on one map)

### Phase 7: Service Alerts

- Ingest GTFS-RT ServiceAlerts feed
- Display route-level alerts (detours, cancellations, delays)
- Banner notifications in the frontend

### Phase 8: Historical Analytics

- Persist vehicle positions to a time-series store (TimescaleDB or ClickHouse)
- Route performance dashboards (average delay by route, time of day, day of week)
- This is where Option D (event-driven) architecture starts to make sense

### Phase 9: Occupancy Data

- Ingest OccupancyStatus from GTFS-RT (where available)
- Display crowding indicators on vehicles
- Historical occupancy patterns

### Phase 10: Advanced Features

- Arrival predictions using ML (beyond GTFS-RT's own predictions)
- Push notifications ("bus 42 is 5 minutes away")
- Mobile native apps (if web app usage warrants it)
- API for third-party developers

---

## Phase Dependency Graph

```
Phase 1: Foundation (GTFS Static)
    │
    ▼
Phase 2: Real-Time Pipeline (Kalman + Snap)
    │
    ▼
Phase 3: Client Delivery (WebSocket + API)
    │
    ▼
Phase 4: Frontend (Map + Animation)
    │
    ▼
Phase 5: DevOps (Docker + CI + Monitoring)
    │
    ▼
Phase 6+: Multi-agency, Alerts, Analytics...
```

Each phase strictly depends on the one above it. There is no parallelism between phases for a single developer. With 2 developers, Phase 4 (frontend) can start partway through Phase 3 once the WebSocket protocol is defined.

---

## Summary

| Phase | Focus | Effort | Why This Order |
|---|---|---|---|
| **1. Foundation** | GTFS static data + schema | 1 week | Everything depends on route/stop/shape data |
| **2. Pipeline** | Kalman filter + route snap | 1.5–2 weeks | Core value prop; produces clean data for all downstream |
| **3. Delivery** | WebSocket + delta compression | 1 week | Bridges backend pipeline to frontend |
| **4. Frontend** | Map + interpolation + UX | 1.5–2 weeks | Depends on phases 1–3; most visible to users |
| **5. DevOps** | Docker + CI + monitoring | 0.5–1 week | Protects the product, not needed to build it |
| **Total MVP** | | **~5–7 weeks** | |
