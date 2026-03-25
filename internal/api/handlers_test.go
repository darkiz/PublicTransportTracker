package api

import (
	"testing"
)

func TestSplitBBox(t *testing.T) {
	tests := []struct {
		input string
		want  int  // expected length, 0 means nil
		valid bool
	}{
		{"18.0,59.0,18.2,59.4", 4, true},
		{"-73.9,40.7,-73.8,40.8", 4, true},
		{"not,a,bbox,at,all", 0, false},
		{"1,2,3", 0, false},
		{"", 0, false},
	}

	for _, tt := range tests {
		got := splitBBox(tt.input)
		if tt.valid {
			if got == nil || len(got) != tt.want {
				t.Errorf("splitBBox(%q) = %v, want %d coords", tt.input, got, tt.want)
			}
		} else {
			if got != nil {
				t.Errorf("splitBBox(%q) = %v, want nil", tt.input, got)
			}
		}
	}
}

func TestSplit(t *testing.T) {
	got := split("a,b,c", ',')
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("split(\"a,b,c\", ',') = %v", got)
	}
}
