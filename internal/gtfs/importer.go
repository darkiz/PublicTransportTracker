package gtfs

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Importer loads GTFS static data from a ZIP file into PostgreSQL/PostGIS.
type Importer struct {
	db *sql.DB
}

// NewImporter creates a new GTFS importer.
func NewImporter(db *sql.DB) *Importer {
	return &Importer{db: db}
}

// Import reads a GTFS ZIP file and loads all data into the database.
// It uses a transaction so either everything succeeds or nothing changes.
func (imp *Importer) Import(ctx context.Context, zipPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip %s: %w", zipPath, err)
	}
	defer r.Close()

	// Index files by name for easy lookup.
	files := make(map[string]*zip.File)
	for _, f := range r.File {
		files[f.Name] = f
	}

	tx, err := imp.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Clear existing data in reverse dependency order.
	for _, table := range []string{"stop_times", "calendar_dates", "calendar", "trips", "shapes", "stops", "routes", "agencies"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return fmt.Errorf("truncate %s: %w", table, err)
		}
	}

	// Import in dependency order.
	importers := []struct {
		file string
		fn   func(context.Context, *sql.Tx, *zip.File) (int, error)
		req  bool
	}{
		{"agency.txt", imp.importAgencies, true},
		{"routes.txt", imp.importRoutes, true},
		{"stops.txt", imp.importStops, true},
		{"shapes.txt", imp.importShapes, false},
		{"trips.txt", imp.importTrips, true},
		{"stop_times.txt", imp.importStopTimes, true},
		{"calendar.txt", imp.importCalendar, false},
		{"calendar_dates.txt", imp.importCalendarDates, false},
	}

	for _, imp := range importers {
		f, ok := files[imp.file]
		if !ok {
			if imp.req {
				return fmt.Errorf("required file %s not found in ZIP", imp.file)
			}
			slog.Info("optional file not found, skipping", "file", imp.file)
			continue
		}
		count, err := imp.fn(ctx, tx, f)
		if err != nil {
			return fmt.Errorf("import %s: %w", imp.file, err)
		}
		slog.Info("imported file", "file", imp.file, "records", count)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	slog.Info("GTFS import completed successfully")
	return nil
}

// readCSV opens a zip file entry and returns a csv.Reader with header row consumed.
// Returns the header fields and the reader.
func readCSV(f *zip.File) ([]string, *csv.Reader, io.ReadCloser, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, nil, nil, err
	}

	reader := csv.NewReader(rc)
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	header, err := reader.Read()
	if err != nil {
		rc.Close()
		return nil, nil, nil, fmt.Errorf("read header: %w", err)
	}

	// Strip BOM if present.
	if len(header) > 0 {
		header[0] = strings.TrimPrefix(header[0], "\xef\xbb\xbf")
	}

	return header, reader, rc, nil
}

// colIndex returns the index of a column name in the header, or -1 if not found.
func colIndex(header []string, name string) int {
	for i, h := range header {
		if strings.TrimSpace(h) == name {
			return i
		}
	}
	return -1
}

// getField returns the field value by column name, or empty string if not found.
func getField(header []string, record []string, name string) string {
	idx := colIndex(header, name)
	if idx < 0 || idx >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[idx])
}

func (imp *Importer) importAgencies(ctx context.Context, tx *sql.Tx, f *zip.File) (int, error) {
	header, reader, rc, err := readCSV(f)
	if err != nil {
		return 0, err
	}
	defer rc.Close()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO agencies (agency_id, name, url, timezone, lang, phone) VALUES ($1,$2,$3,$4,$5,$6)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	count := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, fmt.Errorf("row %d: %w", count+1, err)
		}

		id := getField(header, record, "agency_id")
		if id == "" {
			id = "default"
		}

		_, err = stmt.ExecContext(ctx, id,
			getField(header, record, "agency_name"),
			getField(header, record, "agency_url"),
			getField(header, record, "agency_timezone"),
			getField(header, record, "agency_lang"),
			getField(header, record, "agency_phone"),
		)
		if err != nil {
			return count, fmt.Errorf("insert agency %s: %w", id, err)
		}
		count++
	}
	return count, nil
}

func (imp *Importer) importRoutes(ctx context.Context, tx *sql.Tx, f *zip.File) (int, error) {
	header, reader, rc, err := readCSV(f)
	if err != nil {
		return 0, err
	}
	defer rc.Close()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO routes (route_id, agency_id, short_name, long_name, route_type, color, text_color, sort_order)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	count := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, fmt.Errorf("row %d: %w", count+1, err)
		}

		agencyID := getField(header, record, "agency_id")
		if agencyID == "" {
			agencyID = "default"
		}

		routeType, _ := strconv.Atoi(getField(header, record, "route_type"))
		sortOrder, _ := strconv.Atoi(getField(header, record, "route_sort_order"))

		_, err = stmt.ExecContext(ctx,
			getField(header, record, "route_id"),
			agencyID,
			getField(header, record, "route_short_name"),
			getField(header, record, "route_long_name"),
			routeType,
			getField(header, record, "route_color"),
			getField(header, record, "route_text_color"),
			sortOrder,
		)
		if err != nil {
			return count, fmt.Errorf("insert route: %w", err)
		}
		count++
	}
	return count, nil
}

func (imp *Importer) importStops(ctx context.Context, tx *sql.Tx, f *zip.File) (int, error) {
	header, reader, rc, err := readCSV(f)
	if err != nil {
		return 0, err
	}
	defer rc.Close()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO stops (stop_id, stop_code, stop_name, location_type, parent_station, geom)
		 VALUES ($1,$2,$3,$4,NULLIF($5,''),ST_SetSRID(ST_MakePoint($6,$7),4326))`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	count := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, fmt.Errorf("row %d: %w", count+1, err)
		}

		lat, _ := strconv.ParseFloat(getField(header, record, "stop_lat"), 64)
		lon, _ := strconv.ParseFloat(getField(header, record, "stop_lon"), 64)
		locType, _ := strconv.Atoi(getField(header, record, "location_type"))

		_, err = stmt.ExecContext(ctx,
			getField(header, record, "stop_id"),
			getField(header, record, "stop_code"),
			getField(header, record, "stop_name"),
			locType,
			getField(header, record, "parent_station"),
			lon, lat, // PostGIS uses (lon, lat) order
		)
		if err != nil {
			return count, fmt.Errorf("insert stop: %w", err)
		}
		count++
	}
	return count, nil
}

func (imp *Importer) importShapes(ctx context.Context, tx *sql.Tx, f *zip.File) (int, error) {
	header, reader, rc, err := readCSV(f)
	if err != nil {
		return 0, err
	}
	defer rc.Close()

	// Collect all points grouped by shape_id, then insert as LineStrings.
	type point struct {
		lon, lat float64
		seq      int
	}
	shapes := make(map[string][]point)

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("read shape point: %w", err)
		}

		shapeID := getField(header, record, "shape_id")
		lat, _ := strconv.ParseFloat(getField(header, record, "shape_pt_lat"), 64)
		lon, _ := strconv.ParseFloat(getField(header, record, "shape_pt_lon"), 64)
		seq, _ := strconv.Atoi(getField(header, record, "shape_pt_sequence"))

		shapes[shapeID] = append(shapes[shapeID], point{lon: lon, lat: lat, seq: seq})
	}

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO shapes (shape_id, geom) VALUES ($1, ST_SetSRID(ST_GeomFromText($2), 4326))`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	count := 0
	for shapeID, points := range shapes {
		// Sort by sequence.
		sort.Slice(points, func(i, j int) bool { return points[i].seq < points[j].seq })

		if len(points) < 2 {
			continue
		}

		// Build WKT LineString.
		var b strings.Builder
		b.WriteString("LINESTRING(")
		for i, p := range points {
			if i > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, "%f %f", p.lon, p.lat)
		}
		b.WriteString(")")

		if _, err := stmt.ExecContext(ctx, shapeID, b.String()); err != nil {
			return count, fmt.Errorf("insert shape %s: %w", shapeID, err)
		}
		count++
	}
	return count, nil
}

func (imp *Importer) importTrips(ctx context.Context, tx *sql.Tx, f *zip.File) (int, error) {
	header, reader, rc, err := readCSV(f)
	if err != nil {
		return 0, err
	}
	defer rc.Close()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO trips (trip_id, route_id, service_id, headsign, short_name, direction_id, shape_id)
		 VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,''))`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	count := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, fmt.Errorf("row %d: %w", count+1, err)
		}

		dirID, _ := strconv.Atoi(getField(header, record, "direction_id"))

		_, err = stmt.ExecContext(ctx,
			getField(header, record, "trip_id"),
			getField(header, record, "route_id"),
			getField(header, record, "service_id"),
			getField(header, record, "trip_headsign"),
			getField(header, record, "trip_short_name"),
			dirID,
			getField(header, record, "shape_id"),
		)
		if err != nil {
			return count, fmt.Errorf("insert trip: %w", err)
		}
		count++
	}
	return count, nil
}

func (imp *Importer) importStopTimes(ctx context.Context, tx *sql.Tx, f *zip.File) (int, error) {
	header, reader, rc, err := readCSV(f)
	if err != nil {
		return 0, err
	}
	defer rc.Close()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO stop_times (trip_id, stop_id, stop_sequence, arrival_time, departure_time, pickup_type, drop_off_type)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	count := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, fmt.Errorf("row %d: %w", count+1, err)
		}

		seq, _ := strconv.Atoi(getField(header, record, "stop_sequence"))
		pickup, _ := strconv.Atoi(getField(header, record, "pickup_type"))
		dropoff, _ := strconv.Atoi(getField(header, record, "drop_off_type"))

		_, err = stmt.ExecContext(ctx,
			getField(header, record, "trip_id"),
			getField(header, record, "stop_id"),
			seq,
			getField(header, record, "arrival_time"),
			getField(header, record, "departure_time"),
			pickup,
			dropoff,
		)
		if err != nil {
			return count, fmt.Errorf("insert stop_time: %w", err)
		}
		count++
	}
	return count, nil
}

func (imp *Importer) importCalendar(ctx context.Context, tx *sql.Tx, f *zip.File) (int, error) {
	header, reader, rc, err := readCSV(f)
	if err != nil {
		return 0, err
	}
	defer rc.Close()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO calendar (service_id, monday, tuesday, wednesday, thursday, friday, saturday, sunday, start_date, end_date)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	count := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, fmt.Errorf("row %d: %w", count+1, err)
		}

		startDate, _ := time.Parse("20060102", getField(header, record, "start_date"))
		endDate, _ := time.Parse("20060102", getField(header, record, "end_date"))

		_, err = stmt.ExecContext(ctx,
			getField(header, record, "service_id"),
			getField(header, record, "monday") == "1",
			getField(header, record, "tuesday") == "1",
			getField(header, record, "wednesday") == "1",
			getField(header, record, "thursday") == "1",
			getField(header, record, "friday") == "1",
			getField(header, record, "saturday") == "1",
			getField(header, record, "sunday") == "1",
			startDate,
			endDate,
		)
		if err != nil {
			return count, fmt.Errorf("insert calendar: %w", err)
		}
		count++
	}
	return count, nil
}

func (imp *Importer) importCalendarDates(ctx context.Context, tx *sql.Tx, f *zip.File) (int, error) {
	header, reader, rc, err := readCSV(f)
	if err != nil {
		return 0, err
	}
	defer rc.Close()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO calendar_dates (service_id, date, exception_type) VALUES ($1,$2,$3)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	count := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, fmt.Errorf("row %d: %w", count+1, err)
		}

		date, _ := time.Parse("20060102", getField(header, record, "date"))
		exType, _ := strconv.Atoi(getField(header, record, "exception_type"))

		_, err = stmt.ExecContext(ctx,
			getField(header, record, "service_id"),
			date,
			exType,
		)
		if err != nil {
			return count, fmt.Errorf("insert calendar_date: %w", err)
		}
		count++
	}
	return count, nil
}
