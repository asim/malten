package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/asim/malten/internal/llm"
)

// buses.go adds nationwide live buses via BODS (SIRI-VM vehicle positions),
// gated on a server-held BODS_API_KEY. Like the other feeds it's exposed to the
// map and to the "Ask Malten" agent.

// boxHalf is the half-size (degrees) of the bounding box drawn around a point
// for a "buses near here" query — roughly a couple of miles across.
const boxHalf = 0.03

func (s *Server) busesEnabled() bool { return s.bods != nil }

// handleBuses returns live buses in a small box around a lat/lng.
func (s *Server) handleBuses(w http.ResponseWriter, r *http.Request) {
	if !s.busesEnabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Live buses aren't configured on this server."})
		return
	}
	lat, err1 := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lng, err2 := strconv.ParseFloat(r.URL.Query().Get("lng"), 64)
	if err1 != nil || err2 != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "lat and lng are required"})
		return
	}
	buses, err := s.bods.Vehicles(r.Context(), lng-boxHalf, lat-boxHalf, lng+boxHalf, lat+boxHalf, 250)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "couldn't reach the live bus service"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"buses": buses})
}

// busTools exposes live buses to the agent, when configured.
func (s *Server) busTools(req askRequest) []llm.Tool {
	if !s.busesEnabled() {
		return nil
	}
	return []llm.Tool{{
		Name: "live_buses",
		Description: "Live buses moving near a point right now (nationwide), with line, destination and operator. " +
			"Omit lat/lng to use the user's current location. These are vehicle positions, not stop-by-stop predictions.",
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
			buses, err := s.bods.Vehicles(ctx, lng-boxHalf, lat-boxHalf, lng+boxHalf, lat+boxHalf, 40)
			if err != nil {
				return "Live buses are unavailable right now.", nil
			}
			if len(buses) == 0 {
				return "No live buses reporting near here right now.", nil
			}
			b, _ := json.Marshal(buses)
			return string(b), nil
		},
	}}
}
