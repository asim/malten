package server

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/asim/malten/internal/llm"
	"github.com/asim/malten/internal/nrail"
	"github.com/asim/malten/internal/osgrid"
)

// around.go answers one question — "what's around me right now?" — by fanning
// out across every live feed we already have (place, rail, bus, London stops)
// and folding the results into a single compact snapshot. It's what makes the
// app feel alive on open, and it's exposed to the agent as `around_me` too.
// No new keys: it only composes feeds already configured on the server.

type aroundStation struct {
	Name  string            `json:"name"`
	CRS   string            `json:"crs"`
	Miles float64           `json:"miles"`
	Next  []nrail.Departure `json:"next,omitempty"`
}

type aroundSnapshot struct {
	Place       *Place             `json:"place,omitempty"`
	GridRef     string             `json:"grid_ref,omitempty"`
	Square      string             `json:"square,omitempty"`     // the 1 km grid square you're standing in
	Neighbours  []osgrid.Neighbour `json:"neighbours,omitempty"` // the eight around it, for "new ground"
	InGB        bool               `json:"in_gb"`
	Weather     *Weather           `json:"weather,omitempty"`
	BusesNearby int                `json:"buses_nearby"`
	StopsNearby int                `json:"stops_nearby"` // London (TfL) bus/tram/tube stops
	Stations    []aroundStation    `json:"stations,omitempty"`
}

// snapshot builds the "around me" view for a point, calling upstreams
// concurrently and returning whatever's available within a short budget.
func (s *Server) snapshot(ctx context.Context, lat, lng float64) aroundSnapshot {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	var snap aroundSnapshot
	if ref, ok := osgrid.FromWGS84(lat, lng); ok {
		snap.InGB, snap.GridRef, snap.Square = true, ref.GridRef, ref.Square
		snap.Neighbours = osgrid.Neighbours(lat, lng)
	}

	var (
		wg             sync.WaitGroup
		place          *Place
		stations       []aroundStation
		wx             *Weather
		busesN, stopsN int
	)

	wg.Add(1)
	go func() { defer wg.Done(); wx = fetchWeather(ctx, lat, lng) }()

	if s.searchEnabled() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if p, err := s.nearestPlace(lat, lng); err == nil {
				place = p
			}
		}()
	}

	// Nearest stations (instant, from the embedded dataset). Fetch a live board
	// only for the closest, to bound latency.
	wg.Add(1)
	go func() {
		defer wg.Done()
		near := nrail.Nearest(lat, lng, 2)
		out := make([]aroundStation, 0, len(near))
		for i, st := range near {
			as := aroundStation{Name: st.Name, CRS: st.CRS, Miles: round1(st.Miles)}
			if s.railEnabled() && i == 0 {
				if b, err := s.darwin.Departures(ctx, st.CRS, 3); err == nil {
					if len(b.Departures) > 3 {
						b.Departures = b.Departures[:3]
					}
					as.Next = b.Departures
				}
			}
			out = append(out, as)
		}
		stations = out
	}()

	if s.busesEnabled() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if buses, err := s.bods.Vehicles(ctx, lng-boxHalf, lat-boxHalf, lng+boxHalf, lat+boxHalf, 250); err == nil {
				busesN = len(buses)
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		if stops, err := nearbyStops(lat, lng, "500"); err == nil {
			stopsN = len(stops)
		}
	}()

	wg.Wait()
	snap.Place, snap.Stations, snap.Weather, snap.BusesNearby, snap.StopsNearby = place, stations, wx, busesN, stopsN
	return snap
}

func (s *Server) handleAround(w http.ResponseWriter, r *http.Request) {
	lat, err1 := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lng, err2 := strconv.ParseFloat(r.URL.Query().Get("lng"), 64)
	if err1 != nil || err2 != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "lat and lng are required"})
		return
	}
	writeJSON(w, http.StatusOK, s.snapshot(r.Context(), lat, lng))
}

// aroundTool lets the agent pull the same live snapshot in one call.
func (s *Server) aroundTool(req askRequest) []llm.Tool {
	return []llm.Tool{{
		Name:        "around_me",
		Description: "A one-call live snapshot of what's around a point right now: nearest named place, nearest rail stations with their next departures, how many buses are moving nearby, and how many London stops are nearby. Omit lat/lng to use the user's current location. Good for 'what's around me' or suggesting something to go do.",
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
			b, _ := json.Marshal(s.snapshot(ctx, lat, lng))
			return string(b), nil
		},
	}}
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }
