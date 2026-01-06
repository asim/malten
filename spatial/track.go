package spatial

import (
	"log"
	"math"
	"sync"
	"time"
)

// TrackPoint is a single GPS point in a user's journey
type TrackPoint struct {
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	Timestamp time.Time `json:"ts"`
}

// UserTrack holds the current journey for a session
type UserTrack struct {
	Points    []TrackPoint
	LastSave  time.Time
	SessionID string
}

var (
	userTracks   = make(map[string]*UserTrack) // session -> track
	userTracksMu sync.RWMutex
)

const (
	minPointDistance  = 20.0  // meters - don't record points closer than this
	minStreetLength   = 100.0 // meters - don't save streets shorter than this
	stationaryTimeout = 5 * time.Minute // save track after this much stillness
	maxTrackPoints    = 500   // max points before auto-save
)

// RecordLocation adds a GPS point to the user's current track
func RecordLocation(sessionID string, lat, lon float64) {
	userTracksMu.Lock()
	defer userTracksMu.Unlock()
	
	track, exists := userTracks[sessionID]
	if !exists {
		track = &UserTrack{
			Points:    make([]TrackPoint, 0),
			SessionID: sessionID,
		}
		userTracks[sessionID] = track
	}
	
	now := time.Now()
	
	// Check distance from last point
	if len(track.Points) > 0 {
		last := track.Points[len(track.Points)-1]
		dist := haversineMeters(last.Lat, last.Lon, lat, lon)
		
		// Skip if too close (standing still or GPS jitter)
		if dist < minPointDistance {
			// But check if we've been stationary long enough to save
			if now.Sub(last.Timestamp) > stationaryTimeout && len(track.Points) > 1 {
				// Save the track and start fresh
				go saveTrackAsStreet(track)
				track.Points = []TrackPoint{{Lat: lat, Lon: lon, Timestamp: now}}
				track.LastSave = now
			}
			return
		}
	}
	
	// Add the point
	track.Points = append(track.Points, TrackPoint{
		Lat:       lat,
		Lon:       lon,
		Timestamp: now,
	})
	
	// Auto-save if track is getting long
	if len(track.Points) >= maxTrackPoints {
		go saveTrackAsStreet(track)
		// Keep last point as start of new track
		lastPoint := track.Points[len(track.Points)-1]
		track.Points = []TrackPoint{lastPoint}
		track.LastSave = now
	}
}

// saveTrackAsStreet converts a user track to a street entity
func saveTrackAsStreet(track *UserTrack) {
	if len(track.Points) < 2 {
		return
	}
	
	// Calculate total length
	totalLength := 0.0
	for i := 1; i < len(track.Points); i++ {
		totalLength += haversineMeters(
			track.Points[i-1].Lat, track.Points[i-1].Lon,
			track.Points[i].Lat, track.Points[i].Lon,
		)
	}
	
	if totalLength < minStreetLength {
		log.Printf("[track] Skipping short track (%.0fm)", totalLength)
		return
	}
	
	// Simplify the track (reduce points while keeping shape)
	simplified := simplifyTrack(track.Points, 10.0) // 10m tolerance
	
	if len(simplified) < 2 {
		return
	}
	
	// Convert to street format [lon, lat]
	points := make([][]float64, len(simplified))
	for i, p := range simplified {
		points[i] = []float64{p.Lon, p.Lat}
	}
	
	// Get start/end for naming
	startLat, startLon := simplified[0].Lat, simplified[0].Lon
	endLat, endLon := simplified[len(simplified)-1].Lat, simplified[len(simplified)-1].Lon
	
	// Create street entity
	db := Get()
	
	// Check if similar street already exists
	existing := db.Query(startLat, startLon, 50, EntityStreet, 10)
	for _, e := range existing {
		sd := e.GetStreetData()
		if sd == nil || len(sd.Points) < 2 {
			continue
		}
		// Check if endpoints are similar
		eLat, eLon := sd.Points[len(sd.Points)-1][1], sd.Points[len(sd.Points)-1][0]
		if haversineMeters(endLat, endLon, eLat, eLon) < 50 {
			log.Printf("[track] Similar street already exists, skipping")
			return
		}
	}
	
	// Create the street
	streetData := &StreetData{
		Points: points,
		Length: totalLength,
		ToName: "user track",
	}
	
	entity := &Entity{
		ID:   GenerateID(EntityStreet, startLat, startLon, track.SessionID),
		Type: EntityStreet,
		Name: "User route",
		Lat:  startLat,
		Lon:  startLon,
		Data: streetData,
	}
	
	db.Insert(entity)
	log.Printf("[track] Saved user route: %.0fm, %d points", totalLength, len(simplified))
}

// simplifyTrack reduces points using Ramer-Douglas-Peucker algorithm
func simplifyTrack(points []TrackPoint, epsilon float64) []TrackPoint {
	if len(points) < 3 {
		return points
	}
	
	// Find point with max distance from line between start and end
	maxDist := 0.0
	maxIdx := 0
	
	start := points[0]
	end := points[len(points)-1]
	
	for i := 1; i < len(points)-1; i++ {
		dist := perpendicularDistance(points[i], start, end)
		if dist > maxDist {
			maxDist = dist
			maxIdx = i
		}
	}
	
	// If max distance > epsilon, recursively simplify
	if maxDist > epsilon {
		left := simplifyTrack(points[:maxIdx+1], epsilon)
		right := simplifyTrack(points[maxIdx:], epsilon)
		
		// Combine, avoiding duplicate at maxIdx
		result := make([]TrackPoint, 0, len(left)+len(right)-1)
		result = append(result, left[:len(left)-1]...)
		result = append(result, right...)
		return result
	}
	
	// All points within epsilon, just keep endpoints
	return []TrackPoint{start, end}
}

// perpendicularDistance calculates distance from point to line (in meters)
func perpendicularDistance(point, lineStart, lineEnd TrackPoint) float64 {
	// Convert to local meters (approximate)
	// Using start point as origin
	latScale := 111320.0 // meters per degree latitude
	lonScale := 111320.0 * math.Cos(lineStart.Lat*math.Pi/180)
	
	x := (point.Lon - lineStart.Lon) * lonScale
	y := (point.Lat - lineStart.Lat) * latScale
	
	x1 := 0.0
	y1 := 0.0
	x2 := (lineEnd.Lon - lineStart.Lon) * lonScale
	y2 := (lineEnd.Lat - lineStart.Lat) * latScale
	
	// Line length squared
	lineLenSq := x2*x2 + y2*y2
	if lineLenSq == 0 {
		return math.Sqrt(x*x + y*y)
	}
	
	// Project point onto line
	t := math.Max(0, math.Min(1, ((x-x1)*(x2-x1)+(y-y1)*(y2-y1))/lineLenSq))
	
	projX := x1 + t*(x2-x1)
	projY := y1 + t*(y2-y1)
	
	return math.Sqrt((x-projX)*(x-projX) + (y-projY)*(y-projY))
}

// haversineMeters calculates distance between two points in meters
func haversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000 // Earth radius in meters
	
	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	
	return R * c
}

// FlushTrack forces saving of a user's current track (e.g., on app close)
func FlushTrack(sessionID string) {
	userTracksMu.Lock()
	track, exists := userTracks[sessionID]
	if exists && len(track.Points) > 1 {
		go saveTrackAsStreet(track)
		delete(userTracks, sessionID)
	}
	userTracksMu.Unlock()
}
