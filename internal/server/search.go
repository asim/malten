package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/asim/malten/internal/llm"
	"github.com/asim/malten/internal/osgrid"
)

// search.go is place search and reverse geocoding via the OS Names API — the
// gazetteer of Great Britain. It needs the shared server OS key (the same one
// the tile proxy uses), so it's enabled exactly when tiles are. OS Names returns
// British National Grid coordinates only, so every result is projected back to
// WGS84 with osgrid.ToWGS84 before it reaches the browser or the agent.

var osClient = &http.Client{Timeout: 12 * time.Second}

var errNoOSKey = errors.New("no OS key configured")

func (s *Server) searchEnabled() bool { return s.osKey != "" }

// osNamesGet calls an OS Names endpoint ("find" or "nearest") with the server key.
func (s *Server) osNamesGet(endpoint string, q url.Values) ([]byte, error) {
	if s.osKey == "" {
		return nil, errNoOSKey
	}
	q.Set("key", s.osKey)
	u := "https://api.os.uk/search/names/v1/" + endpoint + "?" + q.Encode()
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := osClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("os names status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// Place is a gazetteer result, projected to WGS84.
type Place struct {
	Name    string  `json:"name"`
	Type    string  `json:"type,omitempty"` // OS LOCAL_TYPE, e.g. "City", "Railway Station"
	County  string  `json:"county,omitempty"`
	Country string  `json:"country,omitempty"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	GridRef string  `json:"grid_ref,omitempty"`
}

// osNamesResult mirrors the fields we use from a GAZETTEER_ENTRY.
type osNamesResult struct {
	Entry struct {
		Name1     string  `json:"NAME1"`
		LocalType string  `json:"LOCAL_TYPE"`
		X         float64 `json:"GEOMETRY_X"` // British National Grid easting
		Y         float64 `json:"GEOMETRY_Y"` // British National Grid northing
		County    string  `json:"COUNTY_UNITARY"`
		Region    string  `json:"REGION"`
		Country   string  `json:"COUNTRY"`
	} `json:"GAZETTEER_ENTRY"`
}

// toPlace projects an OS Names entry to WGS84; ok is false if it has no usable
// coordinate.
func (r osNamesResult) toPlace() (Place, bool) {
	e := r.Entry
	if e.Name1 == "" || (e.X == 0 && e.Y == 0) {
		return Place{}, false
	}
	lat, lng, ok := osgrid.ToWGS84(e.X, e.Y)
	if !ok {
		return Place{}, false
	}
	p := Place{
		Name:    e.Name1,
		Type:    e.LocalType,
		County:  firstNonEmpty(e.County, e.Region),
		Country: e.Country,
		Lat:     round6(lat),
		Lng:     round6(lng),
	}
	if ref, ok := osgrid.FromWGS84(lat, lng); ok {
		p.GridRef = ref.GridRef
	}
	return p, true
}

// searchPlaces runs an OS Names "find" and returns up to max WGS84 places.
func (s *Server) searchPlaces(query string, max int) ([]Place, error) {
	if max <= 0 || max > 20 {
		max = 8
	}
	q := url.Values{}
	q.Set("query", query)
	q.Set("maxresults", strconv.Itoa(max))
	body, err := s.osNamesGet("find", q)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Results []osNamesResult `json:"results"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	out := make([]Place, 0, len(raw.Results))
	for _, r := range raw.Results {
		if p, ok := r.toPlace(); ok {
			out = append(out, p)
		}
	}
	return out, nil
}

// nearestPlace runs an OS Names "nearest" for a WGS84 point (reverse geocoding).
func (s *Server) nearestPlace(lat, lng float64) (*Place, error) {
	ref, ok := osgrid.FromWGS84(lat, lng)
	if !ok {
		return nil, fmt.Errorf("outside Great Britain")
	}
	q := url.Values{}
	q.Set("point", fmt.Sprintf("%d,%d", ref.Easting, ref.Northing))
	body, err := s.osNamesGet("nearest", q)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Results []osNamesResult `json:"results"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	for _, r := range raw.Results {
		if p, ok := r.toPlace(); ok {
			return &p, nil
		}
	}
	return nil, nil
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if !s.searchEnabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Search isn't configured on this server."})
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "q is required"})
		return
	}
	places, err := s.searchPlaces(query, 8)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "couldn't reach the place search"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": places})
}

func (s *Server) handleNearest(w http.ResponseWriter, r *http.Request) {
	if !s.searchEnabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Search isn't configured on this server."})
		return
	}
	lat, err1 := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lng, err2 := strconv.ParseFloat(r.URL.Query().Get("lng"), 64)
	if err1 != nil || err2 != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "lat and lng are required"})
		return
	}
	place, err := s.nearestPlace(lat, lng)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "couldn't reach the place search"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"nearest": place})
}

// searchTools exposes the gazetteer to the agent, when configured.
func (s *Server) searchTools(req askRequest) []llm.Tool {
	if !s.searchEnabled() {
		return nil
	}
	return []llm.Tool{
		{
			Name:        "find_place",
			Description: "Look up a place, street, station or landmark in Great Britain by name. Returns matches with type, county, latitude/longitude and OS grid reference. Use it to turn a place name into coordinates before using the transport tools.",
			Schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"query": map[string]any{"type": "string", "description": "the place name to search for"}},
				"required":   []any{"query"},
			},
			Run: func(ctx context.Context, in json.RawMessage) (string, error) {
				var a struct {
					Query string `json:"query"`
				}
				_ = json.Unmarshal(in, &a)
				if strings.TrimSpace(a.Query) == "" {
					return "A query is required.", nil
				}
				places, err := s.searchPlaces(a.Query, 6)
				if err != nil {
					return "Place search is unavailable right now.", nil
				}
				if len(places) == 0 {
					return "No matching place found in Great Britain.", nil
				}
				b, _ := json.Marshal(places)
				return string(b), nil
			},
		},
		{
			Name:        "whats_here",
			Description: "Reverse-geocode a point to the nearest named place in Great Britain. Omit lat/lng to use the user's current location.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"lat": map[string]any{"type": "number", "description": "latitude; defaults to the user's location"},
					"lng": map[string]any{"type": "number", "description": "longitude; defaults to the user's location"},
				},
			},
			Run: func(ctx context.Context, in json.RawMessage) (string, error) {
				var a struct {
					Lat *float64 `json:"lat"`
					Lng *float64 `json:"lng"`
				}
				_ = json.Unmarshal(in, &a)
				lat, lng := req.Lat, req.Lng
				if a.Lat != nil {
					lat = *a.Lat
				}
				if a.Lng != nil {
					lng = *a.Lng
				}
				if lat == 0 && lng == 0 {
					return "No location available.", nil
				}
				place, err := s.nearestPlace(lat, lng)
				if err != nil {
					return "Reverse geocoding is unavailable right now.", nil
				}
				if place == nil {
					return "Nothing named found nearby.", nil
				}
				b, _ := json.Marshal(place)
				return string(b), nil
			},
		},
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func round6(f float64) float64 {
	return float64(int64(f*1e6+0.5*sign(f))) / 1e6
}

func sign(f float64) float64 {
	if f < 0 {
		return -1
	}
	return 1
}
