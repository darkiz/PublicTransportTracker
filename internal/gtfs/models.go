package gtfs

import "time"

// Agency represents a GTFS agency.txt record.
type Agency struct {
	ID       string `json:"agency_id"`
	Name     string `json:"agency_name"`
	URL      string `json:"agency_url"`
	Timezone string `json:"agency_timezone"`
	Lang     string `json:"agency_lang,omitempty"`
	Phone    string `json:"agency_phone,omitempty"`
}

// Route represents a GTFS routes.txt record.
type Route struct {
	ID        string `json:"route_id"`
	AgencyID  string `json:"agency_id"`
	ShortName string `json:"route_short_name"`
	LongName  string `json:"route_long_name"`
	Type      int    `json:"route_type"`
	Color     string `json:"route_color,omitempty"`
	TextColor string `json:"route_text_color,omitempty"`
	SortOrder int    `json:"sort_order,omitempty"`
}

// RouteTypeName returns a human-readable name for the GTFS route type.
func RouteTypeName(t int) string {
	switch t {
	case 0:
		return "tram"
	case 1:
		return "metro"
	case 2:
		return "rail"
	case 3:
		return "bus"
	case 4:
		return "ferry"
	case 5:
		return "cable_tram"
	case 6:
		return "aerial_lift"
	case 7:
		return "funicular"
	case 11:
		return "trolleybus"
	case 12:
		return "monorail"
	default:
		return "other"
	}
}

// Stop represents a GTFS stops.txt record.
type Stop struct {
	ID       string  `json:"stop_id"`
	Code     string  `json:"stop_code,omitempty"`
	Name     string  `json:"stop_name"`
	Lat      float64 `json:"stop_lat"`
	Lon      float64 `json:"stop_lon"`
	Type     int     `json:"location_type"`
	ParentID string  `json:"parent_station,omitempty"`
}

// Shape represents a single point in a GTFS shapes.txt record.
type ShapePoint struct {
	ShapeID  string  `json:"shape_id"`
	Lat      float64 `json:"shape_pt_lat"`
	Lon      float64 `json:"shape_pt_lon"`
	Sequence int     `json:"shape_pt_sequence"`
	DistTrav float64 `json:"shape_dist_traveled,omitempty"`
}

// Trip represents a GTFS trips.txt record.
type Trip struct {
	RouteID     string `json:"route_id"`
	ServiceID   string `json:"service_id"`
	TripID      string `json:"trip_id"`
	HeadsignText string `json:"trip_headsign,omitempty"`
	ShortName   string `json:"trip_short_name,omitempty"`
	DirectionID int    `json:"direction_id"`
	ShapeID     string `json:"shape_id,omitempty"`
}

// StopTime represents a GTFS stop_times.txt record.
type StopTime struct {
	TripID       string `json:"trip_id"`
	ArrivalTime  string `json:"arrival_time"`
	DepartureTime string `json:"departure_time"`
	StopID       string `json:"stop_id"`
	StopSequence int    `json:"stop_sequence"`
	PickupType   int    `json:"pickup_type"`
	DropOffType  int    `json:"drop_off_type"`
}

// Calendar represents a GTFS calendar.txt record.
type Calendar struct {
	ServiceID string    `json:"service_id"`
	Monday    bool      `json:"monday"`
	Tuesday   bool      `json:"tuesday"`
	Wednesday bool      `json:"wednesday"`
	Thursday  bool      `json:"thursday"`
	Friday    bool      `json:"friday"`
	Saturday  bool      `json:"saturday"`
	Sunday    bool      `json:"sunday"`
	StartDate time.Time `json:"start_date"`
	EndDate   time.Time `json:"end_date"`
}

// CalendarDate represents a GTFS calendar_dates.txt record.
type CalendarDate struct {
	ServiceID     string    `json:"service_id"`
	Date          time.Time `json:"date"`
	ExceptionType int       `json:"exception_type"` // 1=added, 2=removed
}
