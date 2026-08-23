package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"time"
)

// tfl.go proxies a little of the Transport for London Unified API so the map can
// show live transport in London. It fetches public data server-side (no user
// content stored); an optional TFL_APP_KEY raises the rate limit. These two
// endpoints — nearby stops and live arrivals — are also the shape the future
// "Ask Malten" agent will call as tools.

// tflAPI is the Transport for London Unified API; a var so tests can stub it.
var tflAPI = "https://api.tfl.gov.uk"

var tflClient = &http.Client{Timeout: 12 * time.Second}

func tflGet(path string, q url.Values) ([]byte, error) {
	if q == nil {
		q = url.Values{}
	}
	if k := os.Getenv("TFL_APP_KEY"); k != "" {
		q.Set("app_key", k)
	}
	u := tflAPI + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	// TfL blocks the default Go user-agent, so identify ourselves.
	req.Header.Set("User-Agent", "Malten/1.0 (+https://malten.ai)")
	resp, err := tflClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tfl status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// Stop is a nearby public-transport stop.
type Stop struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Lat   float64  `json:"lat"`
	Lon   float64  `json:"lon"`
	Lines []string `json:"lines"`
}

// Arrival is a live vehicle due at a stop.
type Arrival struct {
	Line        string `json:"line"`
	Destination string `json:"destination"`
	Mins        int    `json:"mins"`
}

// nearbyStops returns bus/tram/tube stops near a lat/lng. This is the shared
// core behind both /api/stops and the agent's `nearby_stops` tool.
func nearbyStops(lat, lng float64, radius string) ([]Stop, error) {
	if radius == "" {
		radius = "500"
	}
	q := url.Values{}
	q.Set("lat", strconv.FormatFloat(lat, 'f', 6, 64))
	q.Set("lon", strconv.FormatFloat(lng, 'f', 6, 64))
	q.Set("radius", radius)
	q.Set("stopTypes", "NaptanPublicBusCoachTram,NaptanMetroStation")

	body, err := tflGet("/StopPoint", q)
	if err != nil {
		return nil, err
	}
	var raw struct {
		StopPoints []struct {
			ID         string  `json:"id"`
			CommonName string  `json:"commonName"`
			Lat        float64 `json:"lat"`
			Lon        float64 `json:"lon"`
			Lines      []struct {
				Name string `json:"name"`
			} `json:"lines"`
		} `json:"stopPoints"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	out := make([]Stop, 0, len(raw.StopPoints))
	for _, sp := range raw.StopPoints {
		if sp.Lat == 0 && sp.Lon == 0 {
			continue
		}
		lines := make([]string, 0, len(sp.Lines))
		for _, l := range sp.Lines {
			lines = append(lines, l.Name)
		}
		out = append(out, Stop{ID: sp.ID, Name: sp.CommonName, Lat: sp.Lat, Lon: sp.Lon, Lines: lines})
	}
	return out, nil
}

// stopArrivals returns live arrivals for a stop id, soonest first (capped).
// Shared by /api/arrivals and the agent's `arrivals` tool.
func stopArrivals(id string) ([]Arrival, error) {
	body, err := tflGet("/StopPoint/"+url.PathEscape(id)+"/Arrivals", nil)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		LineName        string `json:"lineName"`
		DestinationName string `json:"destinationName"`
		TimeToStation   int    `json:"timeToStation"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	sort.Slice(raw, func(i, j int) bool { return raw[i].TimeToStation < raw[j].TimeToStation })
	out := make([]Arrival, 0, len(raw))
	for _, a := range raw {
		if len(out) >= 8 {
			break
		}
		out = append(out, Arrival{Line: a.LineName, Destination: a.DestinationName, Mins: (a.TimeToStation + 30) / 60})
	}
	return out, nil
}

// handleStops returns nearby bus/tram/tube stops for a lat/lng.
func (s *Server) handleStops(w http.ResponseWriter, r *http.Request) {
	lat, err1 := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lng, err2 := strconv.ParseFloat(r.URL.Query().Get("lng"), 64)
	if err1 != nil || err2 != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "lat and lng are required"})
		return
	}
	stops, err := nearbyStops(lat, lng, r.URL.Query().Get("radius"))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "couldn't reach the live transport service"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stops": stops})
}

// handleArrivals returns live arrivals for a stop id, soonest first.
func (s *Server) handleArrivals(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}
	arrivals, err := stopArrivals(id)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "couldn't reach the live transport service"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"arrivals": arrivals})
}
