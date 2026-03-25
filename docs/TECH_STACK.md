# Technology Stack — PublicTransportTracker

## Overview

Every technology choice below was made with three criteria: **performance for real-time workloads**, **development velocity for a small team**, and **operational simplicity at MVP scale**.

---

## Backend: Go

**Choice:** Go 1.22+

### Why Go

- **Goroutine-native concurrency.** The pipeline (poll → decode → filter → broadcast) maps directly to goroutines connected by channels. No callback hell, no async/await coloring, no thread pool tuning.
- **Single static binary.** `go build` produces one file. No runtime dependencies, no JVM, no `node_modules`. Deploy by copying a file.
- **Excellent WebSocket performance.** Go routinely handles 100k+ concurrent WebSocket connections. Benchmarks show ~37 bytes overhead per connection with `coder/websocket`. Our target of 10,000 clients is trivial.
- **Strong protobuf ecosystem.** GTFS-RT uses Protocol Buffers. Go's `google.golang.org/protobuf` is mature and fast.
- **Fast compilation.** Full rebuild in <5 seconds. No waiting for builds during development.
- **Predictable performance.** No JIT warmup, no GC pauses in the tens of milliseconds (Go's GC targets <1ms pause times as of Go 1.19+).

### Alternatives Considered

| Language | Why Not |
|---|---|
| **Rust** | Excellent performance but significantly slower development velocity. The borrow checker adds friction for prototyping. The concurrency model (async runtimes, pinning) is more complex than goroutines. Overkill for an I/O-bound workload. |
| **Node.js (TypeScript)** | Single-threaded event loop struggles with CPU-intensive Kalman filter math at scale. GC pauses less predictable. `worker_threads` add complexity. Ecosystem fragmentation (multiple WebSocket libraries with different trade-offs). |
| **Java / Kotlin** | JVM startup time (seconds vs milliseconds). Higher memory footprint (~100MB+ baseline). Heavier deployment artifact. Spring Boot adds unnecessary framework weight for a system that's mostly channels and goroutines. |
| **Python** | Too slow for the hot path. Even with asyncio, the GIL prevents true parallelism. Would need Cython/Rust extensions for the Kalman filter, defeating the purpose. |
| **C#/.NET** | Viable option with good async support, but smaller ecosystem for GTFS tooling. Less common in the transit/geo space. |

---

## Frontend Framework: Svelte 5

**Choice:** Svelte 5 with runes (`$state`, `$derived`, `$effect`)

### Why Svelte 5

- **No virtual DOM.** Svelte compiles to direct DOM manipulation. At 60fps with 5,000+ vehicle markers updating, we cannot afford React's reconciliation overhead.
- **Fine-grained reactivity via runes.** `$state` creates surgically precise reactive updates. When one vehicle's position changes, only that vehicle's DOM/canvas element updates — not the entire vehicle list.
- **Small bundle size.** Svelte's compiler output is typically 30–50% smaller than equivalent React bundles. Faster initial load on mobile connections.
- **Native stores.** Svelte's `$state` eliminates the need for Redux/Zustand/MobX. The vehicle state store is just a reactive Map.
- **Straightforward component model.** Less boilerplate than React (no `useEffect` dependency arrays, no `useMemo` optimization dance).

### Alternatives Considered

| Framework | Why Not |
|---|---|
| **React** | Virtual DOM reconciliation is a bottleneck at 60fps update rates with thousands of elements. `useMemo`/`useCallback` optimization is manual and error-prone. Larger bundle size. |
| **Vue 3** | Similar capability to Svelte with Composition API, but still uses a virtual DOM (lighter than React's, but present). Svelte's compile-time approach produces less runtime overhead. |
| **Solid.js** | Closest competitor — also no virtual DOM, fine-grained reactivity. Smaller ecosystem and community. Svelte has better tooling (SvelteKit, better IDE support). |
| **Vanilla JS** | Maximum control but enormous development cost. No component model, no reactivity, manual state management. Not practical for a full application. |

---

## Map Rendering: MapLibre GL JS

**Choice:** MapLibre GL JS v4+

### Why MapLibre

- **Open source, no API key.** Fork of Mapbox GL JS v1 under BSD license. No usage-based billing, no API tokens, no vendor lock-in.
- **WebGL vector tiles.** Client-side rendering of vector tiles enables smooth zooming, rotation, and 3D perspective. Raster tile libraries (Leaflet) look blurry at non-integer zoom levels.
- **Rich styling.** Full control over map appearance via style JSON. Can match any design system.
- **Large ecosystem.** Compatible with Mapbox-style tiles from many providers (MapTiler, Stadia Maps, OpenFreeMap, self-hosted with `tileserver-gl`).
- **Active development.** The MapLibre community is large and the project is well-funded by multiple sponsors.

### Alternatives Considered

| Library | Why Not |
|---|---|
| **Mapbox GL JS v2+** | Proprietary license since v2. Requires API token and incurs usage-based costs. Functionally similar to MapLibre for our needs. |
| **Leaflet** | No WebGL rendering. Canvas/SVG-based markers degrade significantly beyond ~1,000 markers. No smooth vector tile rendering. |
| **OpenLayers** | Capable but verbose API. Steeper learning curve. Less community momentum than MapLibre. |
| **Google Maps JS API** | Proprietary, usage-based pricing, limited styling control, no offline capability. |

---

## Vehicle Rendering Layer: deck.gl

**Choice:** deck.gl v9+ (`IconLayer` or `ScatterplotLayer`)

### Why deck.gl

- **GPU-accelerated rendering.** Renders 100,000+ points at 60fps using WebGL instanced rendering. Our 5,000 vehicles are trivial.
- **Attribute-level updates.** When a vehicle moves, only its position attribute is updated in the GPU buffer — not the entire layer. This is critical for smooth animation.
- **MapLibre integration.** deck.gl overlays natively on MapLibre via `@deck.gl/mapbox` (works with MapLibre's Mapbox-compatible API).
- **Built-in transitions.** `TransitionInterpolator` can handle position interpolation, though we'll likely implement custom interpolation for Kalman-predicted trajectories.
- **IconLayer.** Supports oriented vehicle icons (bus facing direction of travel), icon atlases for different vehicle types, and per-icon sizing/coloring.

### Why Not MapLibre Native Symbols

MapLibre's symbol layers use CPU-based collision detection and label placement. Beyond ~5,000 symbols, frame rate drops noticeably, especially with icon rotation. deck.gl bypasses this entirely with GPU instancing.

### Alternatives Considered

| Library | Why Not |
|---|---|
| **MapLibre native layers** | CPU-bound symbol placement. Performance ceiling too low for our vehicle count with rotation and per-frame updates. |
| **Three.js overlay** | Maximum control but requires building everything from scratch — projection, picking, layer management. Massive development cost for the same result deck.gl provides out of the box. |
| **Mapbox custom layers** | Raw WebGL in Mapbox/MapLibre custom layers. High effort, no abstractions. deck.gl already does this well. |

---

## Database: PostgreSQL + PostGIS

**Choice:** PostgreSQL 16+ with PostGIS 3.4+

### Why PostgreSQL + PostGIS

- **Spatial queries.** "Find all stops within this bounding box" and "find the nearest point on this route shape to this GPS coordinate" are core operations. PostGIS handles these with spatial indexes (GiST/SP-GiST).
- **GTFS data model fit.** GTFS is a relational dataset (routes have trips, trips have stop_times, stop_times reference stops). A relational database is the natural fit.
- **Mature GTFS ecosystem.** Tools like `gtfs-to-sql` and `gtfsdb` exist for PostgreSQL. The schema patterns are well-documented.
- **LineString geometry.** Route shapes are stored as `GEOGRAPHY(LineString, 4326)`. PostGIS provides `ST_LineLocatePoint`, `ST_LineInterpolatePoint`, and `ST_Distance` for route snapping.
- **General purpose.** Also serves REST API queries for stop/route information. One database for everything at MVP scale.

### Alternatives Considered

| Database | Why Not |
|---|---|
| **SQLite + SpatiaLite** | No concurrent write access. Single-writer limitation breaks when GTFS import runs while the app serves reads. Doesn't scale beyond a single process. Fine for CLI tools, wrong for a server. |
| **MongoDB** | Weaker spatial indexing (2dsphere is basic compared to PostGIS). Schema flexibility is unnecessary — GTFS has a fixed, well-defined schema. No foreign keys for data integrity. |
| **DuckDB** | Excellent for analytics but not designed for concurrent read/write server workloads. No PostGIS-level spatial operations. |
| **Redis (as primary store)** | No spatial indexing beyond basic geo commands. No relational queries. Persistence is an afterthought. Redis is a cache, not a database for structured GTFS data. |

---

## WebSocket Encoding: MessagePack

**Choice:** MessagePack for WebSocket binary frames

### Why MessagePack

- **~40% smaller than JSON.** For high-frequency vehicle position updates to 10,000 clients, bandwidth savings are significant. At 5,000 vehicles × 10,000 clients × updates every few seconds, every byte matters.
- **Fast encode/decode.** MessagePack libraries (Go: `vmihailenco/msgpack`, JS: `@msgpack/msgpack`) benchmark faster than JSON for structured data.
- **Schema-less.** Unlike Protocol Buffers, no `.proto` files needed for the client-facing protocol. The client just unpacks the binary data. Easier to iterate during development.
- **Binary WebSocket frames.** WebSocket supports binary frames natively. MessagePack fits naturally.

### Alternatives Considered

| Format | Why Not |
|---|---|
| **JSON** | ~40% larger payloads. Slower to parse. At scale, this is real bandwidth and CPU cost. |
| **Protocol Buffers** | More efficient than MessagePack but requires `.proto` schema files and code generation for the client. Adds build complexity for marginal size improvement over MessagePack. Better suited for service-to-service communication (which we don't have in the monolith). |
| **FlatBuffers** | Zero-copy deserialization is overkill for small vehicle update messages. Complex schema management. |
| **CBOR** | Similar to MessagePack but less popular, fewer battle-tested libraries. |

---

## Infrastructure: Docker Compose

**Choice:** Docker Compose for local development and single-server deployment

### Why Docker Compose

- **Single-command startup.** `docker compose up` starts PostgreSQL/PostGIS and the Go application. New contributors are productive in minutes.
- **Matches production.** At MVP scale, production is literally Docker Compose on a VPS. No gap between dev and prod environments.
- **No orchestration overhead.** Kubernetes is wildly overkill for 2 containers (app + database). Docker Compose is the right tool for the right scale.

### Why Not Kubernetes

Kubernetes solves problems we don't have at MVP:
- We don't need auto-scaling (one process handles the load).
- We don't need rolling deployments across multiple replicas (one replica is fine).
- We don't need service discovery (two containers, they know about each other).
- We don't need managed secrets or config maps (environment variables work).

Kubernetes would add ~2 weeks of setup and ongoing operational overhead for zero benefit at this scale. When the system grows beyond one server, Kubernetes becomes the right choice — but that's a future problem.

---

## Summary Table

| Layer | Technology | Key Reason |
|---|---|---|
| Backend language | Go 1.22+ | Goroutine concurrency, single binary, WebSocket performance |
| Frontend framework | Svelte 5 | No virtual DOM, fine-grained reactivity for 60fps |
| Map engine | MapLibre GL JS | Open source, WebGL vector tiles, no API key |
| Vehicle rendering | deck.gl | GPU-accelerated, 100k+ points at 60fps |
| Database | PostgreSQL + PostGIS | Spatial queries, relational GTFS fit |
| WebSocket encoding | MessagePack | 40% smaller than JSON, fast encode/decode |
| Infrastructure | Docker Compose | Single-command startup, right-sized for MVP |
| Version control | Git + GitHub | Standard, CI/CD ready |
