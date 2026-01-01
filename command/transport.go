package command

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/asim/malten/spatial"
)

// GetSpatialDB returns the spatial database
func GetSpatialDB() *spatial.DB {
	return spatial.Get()
}

const tflBaseURL = "https://api.tfl.gov.uk"

// TfL response types
type TfLStop struct {
	NaptanID   string    `json:"naptanId"`
	CommonName string    `json:"commonName"`
	Indicator  string    `json:"indicator"`
	Distance   float64   `json:"distance"`
	Lat        float64   `json:"lat"`
	Lon        float64   `json:"lon"`
	Lines      []TfLLine `json:"lines"`
	Modes      []string  `json:"modes"`
}

type TfLLine struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type TfLArrival struct {
	LineName        string `json:"lineName"`
	DestinationName string `json:"destinationName"`
	TimeToStation   int    `json:"timeToStation"` // seconds
	ExpectedArrival string `json:"expectedArrival"`
}

type TfLStopResponse struct {
	StopPoints []TfLStop `json:"stopPoints"`
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

func tflGet(url string, result interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Malten/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("TfL API error: %s", resp.Status)
	}

	return json.NewDecoder(resp.Body).Decode(result)
}

// GetNearbyStops finds bus/tram stops near a location
func GetNearbyStops(lat, lon float64, radius int) ([]TfLStop, error) {
	url := fmt.Sprintf("%s/StopPoint?lat=%f&lon=%f&stopTypes=NaptanPublicBusCoachTram&radius=%d",
		tflBaseURL, lat, lon, radius)

	var result TfLStopResponse
	if err := tflGet(url, &result); err != nil {
		return nil, err
	}

	return result.StopPoints, nil
}

// GetArrivals gets live arrivals for a stop
func GetArrivals(naptanID string) ([]TfLArrival, error) {
	url := fmt.Sprintf("%s/StopPoint/%s/Arrivals", tflBaseURL, naptanID)

	var arrivals []TfLArrival
	if err := tflGet(url, &arrivals); err != nil {
		return nil, err
	}

	// Sort by time
	sort.Slice(arrivals, func(i, j int) bool {
		return arrivals[i].TimeToStation < arrivals[j].TimeToStation
	})

	return arrivals, nil
}

// GetLocalContext returns a summary of what's happening nearby
// This is the "look around" view when you open Malten
func GetLocalContext(lat, lon float64) string {
	var parts []string

	// Get nearby bus stops and arrivals
	stops, err := GetNearbyStops(lat, lon, 300)
	if err == nil && len(stops) > 0 {
		// Group by stop name, get arrivals for closest of each
		seen := make(map[string]bool)
		var busInfo []string

		for _, stop := range stops {
			if seen[stop.CommonName] || len(busInfo) >= 2 {
				continue
			}
			seen[stop.CommonName] = true

			arrivals, err := GetArrivals(stop.NaptanID)
			if err != nil || len(arrivals) == 0 {
				continue
			}

			// Take first 2 arrivals
			var times []string
			for i, arr := range arrivals {
				if i >= 2 {
					break
				}
				mins := arr.TimeToStation / 60
				times = append(times, fmt.Sprintf("%s→%s %dm",
					arr.LineName, shortDest(arr.DestinationName), mins))
			}

			if len(times) > 0 {
				busInfo = append(busInfo, fmt.Sprintf("🚌 %s: %s",
					stop.CommonName, strings.Join(times, ", ")))
			}
		}

		if len(busInfo) > 0 {
			parts = append(parts, strings.Join(busInfo, "\n"))
		}
	}

	// Add nearby places summary from spatial DB
	placesSummary := getNearbyPlacesSummary(lat, lon)
	if placesSummary != "" {
		parts = append(parts, placesSummary)
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, "\n")
}

// getNearbyPlacesSummary returns a compact summary of nearby POIs
func getNearbyPlacesSummary(lat, lon float64) string {
	db := GetSpatialDB()
	if db == nil {
		return ""
	}

	// Get counts by category
	categories := []string{"cafe", "restaurant", "pharmacy", "supermarket"}
	var summary []string

	for _, cat := range categories {
		places := db.QueryPlaces(lat, lon, 500, cat, 10)
		if len(places) > 0 {
			icon := categoryIcon(cat)
			if len(places) == 1 {
				summary = append(summary, fmt.Sprintf("%s %s", icon, places[0].Name))
			} else {
				summary = append(summary, fmt.Sprintf("%s %d %ss", icon, len(places), cat))
			}
		}
	}

	if len(summary) == 0 {
		return ""
	}

	return strings.Join(summary, " · ")
}

func categoryIcon(cat string) string {
	switch cat {
	case "cafe":
		return "☕"
	case "restaurant":
		return "🍽️"
	case "pharmacy":
		return "💊"
	case "supermarket":
		return "🛒"
	case "bank":
		return "🏦"
	case "fuel":
		return "⛽"
	default:
		return "📍"
	}
}

// shortDest shortens destination names
func shortDest(dest string) string {
	// Remove common suffixes
	dest = strings.TrimSuffix(dest, " Bus Station")
	dest = strings.TrimSuffix(dest, " Station")
	// Take first word if long
	if len(dest) > 15 {
		if idx := strings.Index(dest, " "); idx > 0 {
			return dest[:idx]
		}
	}
	return dest
}

// HandleBusCommand handles /bus command
func HandleBusCommand(token string) string {
	loc := GetLocation(token)
	if loc == nil {
		return "📍 Location not available. Use /ping on"
	}

	stops, err := GetNearbyStops(loc.Lat, loc.Lon, 500)
	if err != nil {
		return "Error fetching bus stops"
	}

	if len(stops) == 0 {
		return "No bus stops found nearby"
	}

	var lines []string
	lines = append(lines, "🚌 NEARBY BUSES\n")

	seen := make(map[string]bool)
	for _, stop := range stops {
		if seen[stop.CommonName] || len(lines) > 6 {
			continue
		}
		seen[stop.CommonName] = true

		arrivals, err := GetArrivals(stop.NaptanID)
		if err != nil || len(arrivals) == 0 {
			continue
		}

		lines = append(lines, fmt.Sprintf("• %s", stop.CommonName))
		for i, arr := range arrivals {
			if i >= 3 {
				break
			}
			mins := arr.TimeToStation / 60
			lines = append(lines, fmt.Sprintf("  %s → %s (%d min)",
				arr.LineName, arr.DestinationName, mins))
		}
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}
