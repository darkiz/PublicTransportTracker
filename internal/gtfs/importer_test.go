package gtfs

import (
	"testing"
)

func TestColIndex(t *testing.T) {
	header := []string{"stop_id", "stop_name", "stop_lat", "stop_lon"}

	tests := []struct {
		name string
		want int
	}{
		{"stop_id", 0},
		{"stop_lat", 2},
		{"stop_lon", 3},
		{"nonexistent", -1},
	}

	for _, tt := range tests {
		got := colIndex(header, tt.name)
		if got != tt.want {
			t.Errorf("colIndex(%q) = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestGetField(t *testing.T) {
	header := []string{"stop_id", "stop_name", "stop_lat"}
	record := []string{"S001", "Central Station", "59.3293"}

	tests := []struct {
		name string
		want string
	}{
		{"stop_id", "S001"},
		{"stop_name", "Central Station"},
		{"stop_lat", "59.3293"},
		{"nonexistent", ""},
	}

	for _, tt := range tests {
		got := getField(header, record, tt.name)
		if got != tt.want {
			t.Errorf("getField(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestGetFieldWithBOM(t *testing.T) {
	header := []string{"\xef\xbb\xbfstop_id", "stop_name"}
	// Strip BOM like readCSV does.
	header[0] = header[0][3:] // simulate BOM stripping

	record := []string{"S001", "Test"}
	got := getField(header, record, "stop_id")
	if got != "S001" {
		t.Errorf("getField with BOM = %q, want %q", got, "S001")
	}
}

func TestRouteTypeName(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "tram"},
		{1, "metro"},
		{2, "rail"},
		{3, "bus"},
		{4, "ferry"},
		{7, "funicular"},
		{99, "other"},
	}

	for _, tt := range tests {
		got := RouteTypeName(tt.input)
		if got != tt.want {
			t.Errorf("RouteTypeName(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
