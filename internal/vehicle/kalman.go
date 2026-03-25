package vehicle

import (
	"math"
	"time"
)

// KalmanFilter implements a 2D position Kalman filter for GPS smoothing.
//
// State vector: [lat, lon, vLat, vLon] (position + velocity).
// Measurement: [lat, lon, speed, bearing] from GTFS-RT.
//
// Single Responsibility: this struct only does position filtering.
// It knows nothing about routes, vehicles, or WebSockets.
type KalmanFilter struct {
	initialized bool
	lastTime    time.Time

	// State vector: [lat, lon, vLat, vLon].
	x [4]float64

	// Error covariance matrix (4x4, stored flat).
	p [16]float64

	// Process noise base (tunable).
	processNoise float64

	// Measurement noise (tunable).
	measurementNoise float64
}

// NewKalmanFilter creates a filter with sensible defaults for GPS vehicle tracking.
func NewKalmanFilter() *KalmanFilter {
	return &KalmanFilter{
		processNoise:     1e-5, // process noise per second (in degrees²/s)
		measurementNoise: 1e-4, // measurement noise (in degrees²)
	}
}

// Filter processes a raw GPS measurement and returns a smoothed position.
func (kf *KalmanFilter) Filter(m Position) Position {
	if !kf.initialized {
		kf.initialize(m)
		return m
	}

	dt := m.Timestamp.Sub(kf.lastTime).Seconds()
	if dt <= 0 {
		dt = 1.0
	}

	kf.predict(dt)
	kf.update(m)
	kf.lastTime = m.Timestamp

	return kf.position(m.Timestamp)
}

func (kf *KalmanFilter) initialize(m Position) {
	kf.x[0] = m.Lat
	kf.x[1] = m.Lon

	// Derive initial velocity from speed and bearing if available.
	if m.Speed > 0 {
		vLat, vLon := speedBearingToVelocity(m.Speed, m.Bearing, m.Lat)
		kf.x[2] = vLat
		kf.x[3] = vLon
	}

	// Initial covariance: moderate uncertainty.
	kf.p = diagMatrix(kf.measurementNoise, kf.measurementNoise, 1e-4, 1e-4)

	kf.initialized = true
	kf.lastTime = m.Timestamp
}

// predict applies the state transition (constant velocity model).
func (kf *KalmanFilter) predict(dt float64) {
	// State prediction: x_new = F * x
	// F = [[1, 0, dt, 0],
	//      [0, 1, 0, dt],
	//      [0, 0, 1,  0],
	//      [0, 0, 0,  1]]
	kf.x[0] += kf.x[2] * dt
	kf.x[1] += kf.x[3] * dt

	// Covariance prediction: P_new = F*P*F' + Q
	// Scale process noise with dt to handle time gaps.
	q := kf.processNoise * dt
	if dt > 30 {
		// Large gap: increase uncertainty significantly so the filter
		// trusts the next measurement more.
		q *= dt / 10.0
	}

	// Apply F*P*F' in-place (constant velocity transition).
	p := &kf.p
	// Row 0: lat depends on vLat
	p[0] += dt*(p[8]+p[2]) + dt*dt*p[10]
	p[1] += dt*(p[9]+p[3]) + dt*dt*p[11]
	p[2] += dt * p[10]
	p[3] += dt * p[11]
	// Row 1: lon depends on vLon
	p[4] += dt*(p[12]+p[6]) + dt*dt*p[14]
	p[5] += dt*(p[13]+p[7]) + dt*dt*p[15]
	p[6] += dt * p[14]
	p[7] += dt * p[15]
	// Row 2: vLat
	p[8] += dt * p[10]
	p[9] += dt * p[11]
	// Row 3: vLon
	p[12] += dt * p[14]
	p[13] += dt * p[15]

	// Add process noise.
	p[0] += q
	p[5] += q
	p[10] += q * 0.1
	p[15] += q * 0.1
}

// update applies the measurement correction step.
func (kf *KalmanFilter) update(m Position) {
	// Measurement model: we observe [lat, lon] directly.
	// H = [[1, 0, 0, 0],
	//      [0, 1, 0, 0]]
	r := kf.measurementNoise
	p := &kf.p

	// Innovation: y = z - H*x
	yLat := m.Lat - kf.x[0]
	yLon := m.Lon - kf.x[1]

	// Innovation covariance: S = H*P*H' + R
	s00 := p[0] + r
	s01 := p[1]
	s10 := p[4]
	s11 := p[5] + r

	// Invert 2x2 S matrix.
	det := s00*s11 - s01*s10
	if math.Abs(det) < 1e-20 {
		// Degenerate — skip update.
		return
	}
	invDet := 1.0 / det
	si00 := s11 * invDet
	si01 := -s01 * invDet
	si10 := -s10 * invDet
	si11 := s00 * invDet

	// Kalman gain: K = P * H' * S^-1 (4x2 matrix).
	// K[i][j] = P[i][0]*Si[0][j] + P[i][1]*Si[1][j]
	var k [8]float64
	for i := 0; i < 4; i++ {
		pi0 := p[i*4+0]
		pi1 := p[i*4+1]
		k[i*2+0] = pi0*si00 + pi1*si10
		k[i*2+1] = pi0*si01 + pi1*si11
	}

	// State update: x = x + K * y
	for i := 0; i < 4; i++ {
		kf.x[i] += k[i*2+0]*yLat + k[i*2+1]*yLon
	}

	// Covariance update: P = (I - K*H) * P
	// Store old P.
	var oldP [16]float64
	copy(oldP[:], p[:])

	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			// (I - K*H)[i][j] = delta(i,j) - K[i][0]*H[0][j] - K[i][1]*H[1][j]
			// H[0][j] = 1 if j==0, H[1][j] = 1 if j==1
			ikh := 0.0
			if j < 2 {
				ikh = k[i*2+j]
			}
			identity := 0.0
			if i == j {
				identity = 1.0
			}
			sum := 0.0
			for l := 0; l < 4; l++ {
				ikhl := 0.0
				if l < 2 {
					ikhl = k[i*2+l]
				}
				il := identity
				if l != i {
					il = 0
				}
				_ = il
				// (I-KH)[i][l] * oldP[l][j]
				ikhil := 0.0
				if i == l {
					ikhil = 1.0
				}
				ikhil -= ikhl
				sum += ikhil * oldP[l*4+j]
			}
			_ = ikh
			_ = identity
			p[i*4+j] = sum
		}
	}

	// Also incorporate speed/bearing as a soft velocity constraint.
	if m.Speed > 0 {
		vLat, vLon := speedBearingToVelocity(m.Speed, m.Bearing, m.Lat)
		// Blend toward measured velocity (simple exponential smoothing).
		alpha := 0.3
		kf.x[2] = kf.x[2]*(1-alpha) + vLat*alpha
		kf.x[3] = kf.x[3]*(1-alpha) + vLon*alpha
	}
}

// position extracts the current filtered position from state.
func (kf *KalmanFilter) position(ts time.Time) Position {
	speed, bearing := velocityToSpeedBearing(kf.x[2], kf.x[3], kf.x[0])

	// Normalize bearing to [0, 360).
	bearing = math.Mod(bearing+360, 360)
	if speed < 0 {
		speed = 0
	}

	return Position{
		Lat:       kf.x[0],
		Lon:       kf.x[1],
		Speed:     speed,
		Bearing:   bearing,
		Timestamp: ts,
	}
}

// speedBearingToVelocity converts speed (m/s) and bearing (degrees) to
// velocity components in degrees/second (lat, lon).
func speedBearingToVelocity(speed, bearing, lat float64) (vLat, vLon float64) {
	bearingRad := bearing * math.Pi / 180.0
	// 1 degree latitude ≈ 111,320 meters.
	// 1 degree longitude ≈ 111,320 * cos(lat) meters.
	metersPerDegreeLat := 111320.0
	metersPerDegreeLon := 111320.0 * math.Cos(lat*math.Pi/180.0)

	vLat = (speed * math.Cos(bearingRad)) / metersPerDegreeLat
	vLon = (speed * math.Sin(bearingRad)) / metersPerDegreeLon
	return
}

// velocityToSpeedBearing converts velocity components (degrees/sec) back to speed and bearing.
func velocityToSpeedBearing(vLat, vLon, lat float64) (speed, bearing float64) {
	metersPerDegreeLat := 111320.0
	metersPerDegreeLon := 111320.0 * math.Cos(lat*math.Pi/180.0)

	vLatM := vLat * metersPerDegreeLat
	vLonM := vLon * metersPerDegreeLon

	speed = math.Sqrt(vLatM*vLatM + vLonM*vLonM)
	bearing = math.Atan2(vLonM, vLatM) * 180.0 / math.Pi
	return
}

// diagMatrix creates a 4x4 diagonal matrix stored as flat [16]float64.
func diagMatrix(d0, d1, d2, d3 float64) [16]float64 {
	var m [16]float64
	m[0] = d0
	m[5] = d1
	m[10] = d2
	m[15] = d3
	return m
}
