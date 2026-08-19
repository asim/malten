package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/asim/malten/internal/llm"
	"github.com/asim/malten/internal/nrail"
)

// rail.go adds National Rail: nearby stations (keyless, from the embedded
// dataset) and live departure boards (Darwin OpenLDBWS, gated on a server-held
// token). Like TfL, these are also exposed to the "Ask Malten" agent as tools —
// so it can answer "when's the next train from here?" nationwide.

func (s *Server) railEnabled() bool { return s.darwin != nil }

// handleStations returns nearby National Rail stations for a lat/lng. Keyless:
// it only reads the embedded station dataset (no live call).
func (s *Server) handleStations(w http.ResponseWriter, r *http.Request) {
	lat, err1 := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lng, err2 := strconv.ParseFloat(r.URL.Query().Get("lng"), 64)
	if err1 != nil || err2 != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "lat and lng are required"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stations": nrail.Nearest(lat, lng, 8)})
}

// handleDepartures returns the live departure board for a station CRS code.
func (s *Server) handleDepartures(w http.ResponseWriter, r *http.Request) {
	if !s.railEnabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Live rail departures aren't configured on this server."})
		return
	}
	crs := r.URL.Query().Get("crs")
	if strings.TrimSpace(crs) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "crs is required"})
		return
	}
	board, err := s.darwin.Departures(r.Context(), crs, 10)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "couldn't reach the live rail service"})
		return
	}
	writeJSON(w, http.StatusOK, board)
}

// railTools exposes rail data to the agent. nearby_stations is always offered
// (it's just the dataset); train_departures only when Darwin is configured.
func (s *Server) railTools(req askRequest) []llm.Tool {
	tools := []llm.Tool{
		{
			Name: "nearby_stations",
			Description: "List the nearest National Rail stations to a point, with CRS code, name and distance in miles. " +
				"Omit lat/lng to use the user's current location. Covers all of Great Britain.",
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
					return "No location available. Ask the user for a place or to share their location.", nil
				}
				b, _ := json.Marshal(nrail.Nearest(lat, lng, 6))
				return string(b), nil
			},
		},
	}
	if s.railEnabled() {
		tools = append(tools, llm.Tool{
			Name: "train_departures",
			Description: "Live train departures from a National Rail station, soonest first, with destination, " +
				"scheduled time, expected time (or 'On time'/'Cancelled'), platform and operator. Takes a CRS code from nearby_stations.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"crs": map[string]any{"type": "string", "description": "the 3-letter CRS code from nearby_stations, e.g. KGX"},
				},
				"required": []any{"crs"},
			},
			Run: func(ctx context.Context, in json.RawMessage) (string, error) {
				var a struct {
					CRS string `json:"crs"`
				}
				_ = json.Unmarshal(in, &a)
				if strings.TrimSpace(a.CRS) == "" {
					return "A CRS code is required (get one from nearby_stations).", nil
				}
				board, err := s.darwin.Departures(ctx, a.CRS, 10)
				if err != nil {
					return "Live rail departures are unavailable right now.", nil
				}
				if len(board.Departures) == 0 {
					return fmt.Sprintf("No departures listed from %s right now.", board.Station), nil
				}
				b, _ := json.Marshal(board)
				return string(b), nil
			},
		})
	}
	return tools
}
