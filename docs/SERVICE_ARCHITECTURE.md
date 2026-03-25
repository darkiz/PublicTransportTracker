# Service Architecture — PublicTransportTracker

## Overview

This document compares four architectural approaches for the real-time transit tracking system, analyzes trade-offs for each, and provides a clear recommendation for the MVP stage.

### System Workload Profile

Before comparing options, it's essential to understand what the system actually does at runtime:

| Workload | Characteristic | Volume |
|---|---|---|
| GTFS static import | Batch job, runs daily/weekly | Minutes of CPU, infrequent |
| GTFS-RT polling | HTTP fetch every 10–30s per feed | ~2,000 decoded position updates/sec |
| Kalman filter + route snap | CPU math, stateful per vehicle | ~50 FLOPs per update, trivial |
| WebSocket fan-out | I/O heavy, broadcast pattern | 5,000 vehicles → 10,000 clients |

The **critical data path** is: HTTP poll → protobuf decode → Kalman filter → route snap → WebSocket broadcast. Latency on this path directly affects perceived real-time quality.

---

## Option A: Full Microservices

### Architecture

```
┌──────────────┐   ┌───────────────────┐   ┌────────────────┐   ┌──────────────┐
│ gtfs-importer│   │ realtime-ingester │──▶│ vehicle-state  │──▶│ api-gateway  │──▶ Clients
└──────┬───────┘   └────────┬──────────┘   └───────┬────────┘   └──────┬───────┘
       │                    │       NATS           │      NATS         │
       ▼                    ▼                      ▼                   ▼
   PostgreSQL           GTFS-RT              In-memory state      WebSocket
   + PostGIS            Feeds                + Redis cache         fan-out
```

**4 independent Go services** connected by NATS message broker.

### Service Responsibilities

| Service | Responsibility |
|---|---|
| **gtfs-importer** | Downloads GTFS ZIP, parses CSV, loads into PostgreSQL/PostGIS. Runs on schedule (daily). |
| **realtime-ingester** | Polls GTFS-RT feeds, decodes protobuf, publishes raw positions to NATS. |
| **vehicle-state** | Subscribes to raw positions, applies Kalman filter + route snapping, maintains vehicle state, publishes smoothed state to NATS. |
| **api-gateway** | Subscribes to smoothed state, manages WebSocket connections, handles REST API for static data queries. |

### Analysis

| Criterion | Assessment |
|---|---|
| **Complexity** | **High.** 4 binaries, each needing config, health checks, Dockerfile, graceful shutdown. NATS message schemas become contracts. |
| **Infrastructure** | **7+ processes:** 4 Go services + NATS + PostgreSQL + Redis. Docker Compose minimum, Kubernetes likely. |
| **Dev velocity (1 dev)** | **8–12 weeks to MVP.** Cross-service debugging requires distributed tracing. Schema changes propagate across services. |
| **Ops overhead** | **High.** Monitor 4 services + NATS + Postgres + Redis. Log correlation across services. Deployment ordering dependencies. |
| **Scalability** | **Excellent.** Each service scales independently. 10 api-gateways behind a load balancer while keeping 1 vehicle-state instance. |
| **Hot-path latency** | **Worst: +2–5ms.** Three NATS hops with serialize/deserialize at each boundary: ingester → NATS → vehicle-state → NATS → api-gateway. |

### Pros

- Independent scaling per service
- Independent deployment (update one service without touching others)
- Team parallelism (4 developers, each own a service)
- Fault isolation (ingester crash doesn't kill WebSocket connections)

### Cons

- Serialize/deserialize overhead at every boundary
- Distributed tracing needed for debugging
- NATS becomes a single point of failure (unless clustered)
- 3x more deployment configuration than a monolith
- Version compatibility between services must be maintained

### When to Choose This

When you have **4+ developers** who need to work and deploy independently, and the system is already proven and you're optimizing for **organizational scaling**, not technical MVP.

---

## Option B: Modular Monolith (Recommended)

### Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Single Go Binary                      │
│                                                          │
│  ┌─────────────┐  chan   ┌───────────────┐  chan   ┌───┐│
│  │internal/     │───────▶│internal/       │───────▶│   ││
│  │realtime      │        │vehicle         │        │api││──▶ Clients
│  │(poller)      │        │(kalman + snap) │        │   ││
│  └─────────────┘         └───────────────┘        └───┘│
│                                                          │
│  ┌─────────────┐                                        │
│  │internal/     │──▶ PostgreSQL + PostGIS                │
│  │gtfs          │                                        │
│  │(importer)    │                                        │
│  └─────────────┘                                        │
└─────────────────────────────────────────────────────────┘
```

**1 Go binary** with internal package boundaries. Communication via Go channels.

### Package Responsibilities

| Package | Responsibility |
|---|---|
| **`cmd/tracker/main.go`** | Entry point. Wires components, manages lifecycle, graceful shutdown. |
| **`internal/gtfs/`** | GTFS static import: parse ZIP, load into PostGIS. Route/stop/shape models. |
| **`internal/realtime/`** | GTFS-RT polling: HTTP fetch, protobuf decode, emit raw positions via channel. |
| **`internal/vehicle/`** | Vehicle state engine: Kalman filter, route snapping, state management. Receives raw positions, emits smoothed state. |
| **`internal/broadcast/`** | WebSocket hub: fan-out to clients, delta compression, connection management. |
| **`internal/api/`** | HTTP server: REST endpoints for static data, WebSocket upgrade handler. |
| **`internal/store/`** | PostgreSQL/PostGIS data access layer. |

### Analysis

| Criterion | Assessment |
|---|---|
| **Complexity** | **Lowest.** All communication is in-process. No serialization on the hot path. |
| **Infrastructure** | **2 processes:** 1 Go binary + PostgreSQL. No NATS, no Redis. |
| **Dev velocity (1 dev)** | **3–5 weeks to MVP.** One `go run .` starts everything. Step-through debugging with Delve across the entire pipeline. |
| **Ops overhead** | **Minimal.** 1 binary to deploy, 1 log stream, 1 health endpoint, 1 metrics endpoint. `pprof` profiling is trivial. |
| **Scalability** | **Sufficient.** A single Go process handles 5,000 vehicles + 10,000 WebSocket clients comfortably (proven by benchmarks). |
| **Hot-path latency** | **Best: ~0.** Channel operations are sub-microsecond. No serialization on the critical path. |

### The Scalability Concern — Addressed

The common objection to monoliths is "it won't scale." Here's why that doesn't apply:

- **5,000 vehicles × Kalman filter**: ~50 FLOPs per update × 2,000 updates/sec = 100,000 FLOPs/sec. A single core handles billions.
- **10,000 WebSocket connections**: Go benchmarks show ~37 bytes overhead per connection with `coder/websocket`. ~372MB RSS for 10,000 connections. A 4GB server has headroom.
- **Fan-out**: Batched delta updates at ~30Hz to 10,000 clients = ~300,000 write operations/sec. A single Go process handles this with a hub-and-spoke pattern.

**The workload fits a single process.** This isn't a philosophical opinion — it's arithmetic.

### Extraction Plan (When You Actually Need It)

If the system grows beyond one machine's WebSocket capacity (~50,000+ clients):

1. Add Redis Pub/Sub as a broadcast channel from the vehicle state engine.
2. Extract `internal/broadcast` + `internal/api` into a separate binary.
3. Run N instances of the API server behind a load balancer.
4. The state engine remains a single process (it's not the bottleneck).

**This is a 1–2 day refactor**, not a rewrite, because the package boundaries already exist.

### Pros

- Fastest time to MVP (2–3x faster than microservices)
- Zero architectural latency on the hot path
- Trivial debugging and profiling
- No external dependencies beyond PostgreSQL
- Package boundaries enforce modularity without network overhead
- Extraction to Option C is a small, well-defined refactor

### Cons

- Single process means single machine (but the workload fits)
- Requires discipline to maintain clean package boundaries
- All components restart together on crash (acceptable at MVP scale)
- Cannot independently scale individual components (not needed at MVP scale)

### When to Choose This

When you are **1–3 developers building an MVP**, the workload fits a single machine (this one does), and you value **development speed and debuggability** over theoretical future scaling.

---

## Option C: Hybrid (2–3 Services)

### Architecture

```
┌──────────────────────────────────┐         ┌──────────────────┐
│     State Engine Service          │  Redis  │  API Gateway(s)   │
│                                   │ Pub/Sub │                   │
│  realtime poller                 │────────▶│  WebSocket hub    │──▶ Clients
│  vehicle state (kalman + snap)   │         │  REST endpoints   │
│  gtfs importer                   │         │                   │
└──────────────┬───────────────────┘         └──────────────────┘
               │
               ▼
          PostgreSQL
          + PostGIS
```

**2–3 services** split at the natural scaling boundary: computation vs. fan-out.

### Service Responsibilities

| Service | Responsibility |
|---|---|
| **State Engine** | GTFS import, GTFS-RT polling, Kalman filtering, route snapping, vehicle state management. Publishes smoothed state to Redis Pub/Sub. |
| **API Gateway** (1–N instances) | Subscribes to state updates, manages WebSocket connections, serves REST API. Stateless — can be horizontally scaled. |
| **GTFS Importer** (optional, separate) | If import is heavy enough to warrant isolation. Can also be a scheduled job within the state engine. |

### Analysis

| Criterion | Assessment |
|---|---|
| **Complexity** | **Moderate.** One serialization boundary. Two Dockerfiles, two deployment configs. |
| **Infrastructure** | **4–5 processes:** 2–3 Go services + PostgreSQL + Redis. |
| **Dev velocity (1 dev)** | **5–7 weeks to MVP.** One message schema to maintain. Two services to debug. |
| **Ops overhead** | **Moderate.** Two services to monitor. One message boundary to debug. Redis health. |
| **Scalability** | **Good.** API gateways scale independently. Add more instances for more WebSocket capacity. |
| **Hot-path latency** | **Acceptable: +50–200μs.** One Redis Pub/Sub hop with serialization. |

### Pros

- Natural split at the real scaling boundary
- API gateway is stateless and horizontally scalable
- Moderate complexity increase over monolith
- Can handle 100k+ WebSocket clients by adding gateway instances

### Cons

- Redis Pub/Sub adds a dependency and a failure mode
- Still need to maintain a serialization schema between services
- 1.5–2x slower to build than a monolith
- Premature if you don't need 50k+ clients

### When to Choose This

When you **know from day one** that you need multiple WebSocket server instances (targeting 50,000+ clients) and want to build the split from the start rather than retrofitting.

---

## Option D: Event-Driven (NATS JetStream)

### Architecture

```
                    NATS JetStream
┌─────────┐    ┌──────────────────────┐    ┌──────────────┐
│ Ingester │──▶│  raw.positions       │──▶│ State Engine  │
└─────────┘    │  smoothed.positions  │──▶│              │
               │  trip.updates        │    └──────┬───────┘
               │  system.events       │           │
               └──────────┬───────────┘    ┌──────▼───────┐
                          │                │ API Gateway   │──▶ Clients
                          ▼                └──────────────┘
                   File/Memory
                   Persistence            PostgreSQL
                   (replay)               + PostGIS
```

**Services organized around persistent event streams.** All state changes are events. Services replay streams to rebuild state.

### Service Responsibilities

| Service | Responsibility |
|---|---|
| **Ingester** | Polls GTFS-RT, publishes raw position events to `raw.positions` stream. |
| **State Engine** | Consumes `raw.positions`, applies Kalman filter + snap, publishes to `smoothed.positions`. Can replay stream to rebuild state after restart. |
| **API Gateway** | Consumes `smoothed.positions`, delivers to WebSocket clients. |
| **GTFS Importer** | Publishes import events to `system.events`. Other services react to GTFS data updates. |

### Analysis

| Criterion | Assessment |
|---|---|
| **Complexity** | **Highest.** Event schema design, versioning, stream compaction, consumer groups, replay semantics — all required upfront. |
| **Infrastructure** | **5–6 processes:** 3–4 Go services + NATS JetStream (with persistent storage) + PostgreSQL. |
| **Dev velocity (1 dev)** | **10–14 weeks to MVP.** JetStream consumer model has a steep learning curve. Event versioning must be planned before writing business logic. |
| **Ops overhead** | **High.** Monitor stream disk usage, consumer lag, redelivery. Event schema evolution must be managed. |
| **Scalability** | **Theoretically excellent.** Adding new consumers to existing streams is trivial. Replay enables new services to catch up. |
| **Hot-path latency** | **Second worst: +1–5ms.** JetStream file persistence adds fsync latency. Memory streams sacrifice the persistence benefit. |

### Pros

- Full event replay (rebuild state from any point in time)
- Audit trail built in (every event is persisted)
- Easy to add new consumers (e.g., analytics, ML pipeline)
- Temporal queries ("what was vehicle X's position at time T?")
- Clean decoupling — services only know about streams, not each other

### Cons

- Massive upfront design cost for event schemas
- JetStream persistence adds latency on the hot path
- Event versioning is a hard, ongoing problem
- Replay of Kalman filter state is architecturally awkward (filter is inherently sequential)
- "What's the position NOW?" doesn't benefit from event sourcing
- 3–4x slower to MVP than a monolith

### When to Choose This

When you need **event replay, audit logging, or temporal queries** as core features. When building a **platform** where many different consumers process the same event streams (analytics, ML training, compliance). Not for an MVP focused on real-time display.

---

## Comparison Matrix

| Criterion | A: Full Micro | B: Monolith | C: Hybrid | D: Event-Driven |
|---|---|---|---|---|
| **Time to MVP (1 dev)** | 8–12 weeks | **3–5 weeks** | 5–7 weeks | 10–14 weeks |
| **Processes to run** | 7+ | **2** | 4–5 | 5–6 |
| **External dependencies** | NATS, Postgres, Redis | **Postgres only** | Postgres, Redis | NATS JetStream, Postgres |
| **Hot-path latency added** | 2–5ms | **~0** | 50–200μs | 1–5ms |
| **Debugging difficulty** | High | **Low** | Moderate | High |
| **Operational overhead** | High | **Low** | Moderate | High |
| **Scaling ceiling** | Unlimited | ~50k clients | ~100k+ clients | Unlimited |
| **Scaling effort when needed** | Already paid | 1–2 day refactor | Add instances | Already paid |
| **Risk of over-engineering** | High | **Low** | Moderate | Very High |

---

## Recommendation: Option B — Modular Monolith

**Option B is the clear winner for this project at the MVP stage.** The reasoning is grounded in three facts:

### 1. The workload fits a single process

This is not an opinion — it's math. 5,000 vehicles at 2,000 msg/sec with Kalman filtering is trivial CPU work. 10,000 WebSocket connections with batched delta updates is well within what a single Go process handles. A single $40/month VPS can run this entire system.

### 2. Development velocity is decisive for MVP

3–5 weeks vs. 8–12 weeks is not a minor difference. Every week spent on NATS schemas, distributed tracing, and cross-service debugging is a week not spent on the Kalman filter, route-snapping algorithm, or 60fps frontend animation — the features that users actually see and care about.

### 3. The extraction path is cheap and well-defined

The monolith's internal package boundaries mirror the microservice boundaries. When (if) you actually need to split, extracting `internal/broadcast` + `internal/api` into a separate service with Redis Pub/Sub is a 1–2 day refactor. You're not painting yourself into a corner — you're deferring a decision until you have data to inform it.

### The Anti-Pattern to Avoid

The single biggest risk for this project is **premature distribution**: building infrastructure for problems that don't exist yet. NATS, Redis, Kubernetes, service meshes, distributed tracing — these are all valuable tools, but only when the problems they solve are real. At MVP scale, they're pure overhead.

> "A distributed system is one where the failure of a computer you didn't even know existed can render your own computer unusable." — Leslie Lamport

Start simple. Measure. Split only when the numbers tell you to.
