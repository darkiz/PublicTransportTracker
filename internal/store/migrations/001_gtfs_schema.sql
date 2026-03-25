-- GTFS Static Data Schema with PostGIS
-- Migration 001: Initial GTFS tables

CREATE EXTENSION IF NOT EXISTS postgis;

-- Agencies
CREATE TABLE IF NOT EXISTS agencies (
    agency_id   TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    url         TEXT NOT NULL,
    timezone    TEXT NOT NULL,
    lang        TEXT,
    phone       TEXT
);

-- Routes
CREATE TABLE IF NOT EXISTS routes (
    route_id    TEXT PRIMARY KEY,
    agency_id   TEXT REFERENCES agencies(agency_id),
    short_name  TEXT,
    long_name   TEXT,
    route_type  INTEGER NOT NULL,
    color       TEXT,
    text_color  TEXT,
    sort_order  INTEGER DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_routes_agency ON routes(agency_id);
CREATE INDEX IF NOT EXISTS idx_routes_type ON routes(route_type);

-- Stops
CREATE TABLE IF NOT EXISTS stops (
    stop_id        TEXT PRIMARY KEY,
    stop_code      TEXT,
    stop_name      TEXT NOT NULL,
    location_type  INTEGER DEFAULT 0,
    parent_station TEXT REFERENCES stops(stop_id),
    geom           GEOMETRY(Point, 4326) NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_stops_geom ON stops USING GIST(geom);
CREATE INDEX IF NOT EXISTS idx_stops_parent ON stops(parent_station);

-- Shapes (stored as full LineStrings per shape_id)
CREATE TABLE IF NOT EXISTS shapes (
    shape_id TEXT PRIMARY KEY,
    geom     GEOMETRY(LineString, 4326) NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_shapes_geom ON shapes USING GIST(geom);

-- Trips
CREATE TABLE IF NOT EXISTS trips (
    trip_id      TEXT PRIMARY KEY,
    route_id     TEXT NOT NULL REFERENCES routes(route_id),
    service_id   TEXT NOT NULL,
    headsign     TEXT,
    short_name   TEXT,
    direction_id INTEGER DEFAULT 0,
    shape_id     TEXT REFERENCES shapes(shape_id)
);

CREATE INDEX IF NOT EXISTS idx_trips_route ON trips(route_id);
CREATE INDEX IF NOT EXISTS idx_trips_service ON trips(service_id);
CREATE INDEX IF NOT EXISTS idx_trips_shape ON trips(shape_id);

-- Stop Times
CREATE TABLE IF NOT EXISTS stop_times (
    trip_id        TEXT NOT NULL REFERENCES trips(trip_id),
    stop_id        TEXT NOT NULL REFERENCES stops(stop_id),
    stop_sequence  INTEGER NOT NULL,
    arrival_time   TEXT NOT NULL,
    departure_time TEXT NOT NULL,
    pickup_type    INTEGER DEFAULT 0,
    drop_off_type  INTEGER DEFAULT 0,
    PRIMARY KEY (trip_id, stop_sequence)
);

CREATE INDEX IF NOT EXISTS idx_stop_times_stop ON stop_times(stop_id);
CREATE INDEX IF NOT EXISTS idx_stop_times_trip ON stop_times(trip_id);

-- Calendar
CREATE TABLE IF NOT EXISTS calendar (
    service_id TEXT PRIMARY KEY,
    monday     BOOLEAN NOT NULL,
    tuesday    BOOLEAN NOT NULL,
    wednesday  BOOLEAN NOT NULL,
    thursday   BOOLEAN NOT NULL,
    friday     BOOLEAN NOT NULL,
    saturday   BOOLEAN NOT NULL,
    sunday     BOOLEAN NOT NULL,
    start_date DATE NOT NULL,
    end_date   DATE NOT NULL
);

-- Calendar Dates (exceptions)
CREATE TABLE IF NOT EXISTS calendar_dates (
    service_id     TEXT NOT NULL,
    date           DATE NOT NULL,
    exception_type INTEGER NOT NULL,
    PRIMARY KEY (service_id, date)
);

CREATE INDEX IF NOT EXISTS idx_calendar_dates_service ON calendar_dates(service_id);
