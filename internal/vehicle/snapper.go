package vehicle

import (
	"math"
	"sync"
)

// InMemorySnapper snaps positions to pre-loaded route shape geometries using
// pure geometry (no database calls on the hot path).
//
// Single Responsibility: only does point-to-polyline projection.
// It stores shapes in memory for O(n) lookup per snap (n = points in shape).
type InMemorySnapper struct {
	mu     sync.RWMutex
	shapes map[string][][2]float64 // shapeID -> [[lat, lon], ...]
}

// NewInMemorySnapper creates an empty snapper. Load shapes before use.
func NewInMemorySnapper() *InMemorySnapper {
	return &InMemorySnapper{
		shapes: make(map[string][][2]float64),
	}
}

// LoadShape stores a shape's points for snapping. Points are [lat, lon] pairs.
func (s *InMemorySnapper) LoadShape(shapeID string, points [][2]float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shapes[shapeID] = points
}

// ShapeCount returns the number of loaded shapes.
func (s *InMemorySnapper) ShapeCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.shapes)
}

// Snap projects the position onto the nearest point on the shape's polyline.
// If the shape is unknown or has fewer than 2 points, returns the original position.
func (s *InMemorySnapper) Snap(pos Position, shapeID string) Position {
	s.mu.RLock()
	points, ok := s.shapes[shapeID]
	s.mu.RUnlock()

	if !ok || len(points) < 2 {
		return pos
	}

	bestLat, bestLon := pos.Lat, pos.Lon
	bestDist := math.Inf(1)

	for i := 0; i < len(points)-1; i++ {
		projLat, projLon, dist := projectPointToSegment(
			pos.Lat, pos.Lon,
			points[i][0], points[i][1],
			points[i+1][0], points[i+1][1],
		)
		if dist < bestDist {
			bestDist = dist
			bestLat = projLat
			bestLon = projLon
		}
	}

	return Position{
		Lat:       bestLat,
		Lon:       bestLon,
		Bearing:   pos.Bearing,
		Speed:     pos.Speed,
		Timestamp: pos.Timestamp,
	}
}

// projectPointToSegment finds the closest point on segment (ax,ay)-(bx,by) to point (px,py).
// Returns the projected point and the squared distance.
// Uses flat-earth approximation which is accurate enough for short segments.
func projectPointToSegment(pLat, pLon, aLat, aLon, bLat, bLon float64) (projLat, projLon, dist float64) {
	// Vector AB.
	abLat := bLat - aLat
	abLon := bLon - aLon

	// Vector AP.
	apLat := pLat - aLat
	apLon := pLon - aLon

	// Project AP onto AB: t = dot(AP, AB) / dot(AB, AB).
	abLenSq := abLat*abLat + abLon*abLon
	if abLenSq < 1e-20 {
		// Degenerate segment (A == B).
		return aLat, aLon, (apLat*apLat + apLon*apLon)
	}

	t := (apLat*abLat + apLon*abLon) / abLenSq

	// Clamp t to [0, 1] to stay on the segment.
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}

	projLat = aLat + t*abLat
	projLon = aLon + t*abLon

	dLat := pLat - projLat
	dLon := pLon - projLon
	dist = dLat*dLat + dLon*dLon

	return
}
