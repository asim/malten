package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/asim/malten/internal/llm"
	"github.com/asim/malten/internal/osgrid"
)

// ask.go is "Ask Malten": a small spatial agent over the map. It answers
// questions about where you are and how to move through it, using live data as
// tools — nearby stops and live arrivals (TfL, London) and the OS National Grid.
//
// It uses an Anthropic API key held on the server (ANTHROPIC_API_KEY). No user
// content is stored: the question and your location are used for the turn and
// then forgotten, in keeping with the stateless model.

const askSystem = `You are "Ask Malten", a spatial guide inside Malten — a map-based exploration app for Great Britain built on Ordnance Survey maps.

You help people understand where they are and how to move through it right now: nearby public transport, when the next bus or train is due, and the lie of the land. Live transport data currently covers London only (Transport for London).

Guidance:
- Be concise and practical. Lead with the answer. Prefer short sentences and small lists.
- When asked about getting somewhere or what's nearby, use the tools to fetch live data rather than guessing. Use the user's current location (provided below) as the default point of reference.
- Give real minutes and line names from the tool results. Don't invent arrivals.
- If live transport returns nothing, say the area may be outside current coverage (London only for now).
- You can mention the OS National Grid reference for a spot when it's relevant.
- Never claim to store anything about the user. You don't.`

// askRequest is the browser's payload.
type askRequest struct {
	Message string  `json:"message"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	HasLoc  bool    `json:"has_loc"`
}

// enabled reports whether the agent is configured.
func (s *Server) askEnabled() bool { return s.llm != nil }

func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST only"})
		return
	}
	if !s.askEnabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Ask Malten isn't configured on this server."})
		return
	}
	var req askRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
		return
	}

	// Server-Sent Events stream back to the browser.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}
	send := func(ev llm.Event) {
		b, _ := json.Marshal(ev)
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}

	// Ground the model in the user's location.
	var ground strings.Builder
	if req.HasLoc {
		fmt.Fprintf(&ground, "The user's current location is latitude %.6f, longitude %.6f.", req.Lat, req.Lng)
		if ref, okRef := osgrid.FromWGS84(req.Lat, req.Lng); okRef {
			fmt.Fprintf(&ground, " That's OS National Grid %s, inside Great Britain.", ref.GridRef)
		} else {
			ground.WriteString(" That's outside Ordnance Survey coverage (Great Britain only).")
		}
	} else {
		ground.WriteString("The user's location is unknown; ask for it or a place name if you need one.")
	}
	system := askSystem + "\n\n" + ground.String()

	if err := s.llm.Run(r.Context(), system, req.Message, s.askTools(req), send); err != nil {
		send(llm.Event{Type: "error", Text: "Something went wrong reaching the guide."})
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// askTools wires the live-data functions in as model tools. The user's location
// is closed over as the default reference point for nearby_stops.
func (s *Server) askTools(req askRequest) []llm.Tool {
	return []llm.Tool{
		{
			Name: "nearby_stops",
			Description: "List public-transport stops (bus, tram, tube, rail) near a point, with their line names and stop ids. " +
				"Omit lat/lng to use the user's current location. London only for now.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"lat":      map[string]any{"type": "number", "description": "latitude; defaults to the user's location"},
					"lng":      map[string]any{"type": "number", "description": "longitude; defaults to the user's location"},
					"radius_m": map[string]any{"type": "integer", "description": "search radius in metres (default 500)"},
				},
			},
			Run: func(ctx context.Context, in json.RawMessage) (string, error) {
				var a struct {
					Lat    *float64 `json:"lat"`
					Lng    *float64 `json:"lng"`
					Radius int      `json:"radius_m"`
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
				radius := ""
				if a.Radius > 0 {
					radius = fmt.Sprintf("%d", a.Radius)
				}
				stops, err := nearbyStops(lat, lng, radius)
				if err != nil {
					return "Live transport is unavailable right now.", nil
				}
				if len(stops) == 0 {
					return "No stops found nearby (live transport covers London only for now).", nil
				}
				b, _ := json.Marshal(stops)
				return string(b), nil
			},
		},
		{
			Name:        "arrivals",
			Description: "Live arrivals for a stop id (from nearby_stops), soonest first, with line, destination and minutes.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"stop_id": map[string]any{"type": "string", "description": "the stop id returned by nearby_stops"},
				},
				"required": []any{"stop_id"},
			},
			Run: func(ctx context.Context, in json.RawMessage) (string, error) {
				var a struct {
					StopID string `json:"stop_id"`
				}
				_ = json.Unmarshal(in, &a)
				if strings.TrimSpace(a.StopID) == "" {
					return "A stop_id is required (get one from nearby_stops).", nil
				}
				arr, err := stopArrivals(a.StopID)
				if err != nil {
					return "Live arrivals are unavailable right now.", nil
				}
				if len(arr) == 0 {
					return "No arrivals due at that stop right now.", nil
				}
				b, _ := json.Marshal(arr)
				return string(b), nil
			},
		},
		{
			Name:        "grid_ref",
			Description: "Convert a latitude/longitude to an OS National Grid reference (Great Britain only).",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"lat": map[string]any{"type": "number"},
					"lng": map[string]any{"type": "number"},
				},
				"required": []any{"lat", "lng"},
			},
			Run: func(ctx context.Context, in json.RawMessage) (string, error) {
				var a struct{ Lat, Lng float64 }
				_ = json.Unmarshal(in, &a)
				ref, ok := osgrid.FromWGS84(a.Lat, a.Lng)
				if !ok {
					return "That point is outside Great Britain, so it has no OS National Grid reference.", nil
				}
				return fmt.Sprintf("%s (easting %d, northing %d)", ref.GridRef, ref.Easting, ref.Northing), nil
			},
		},
	}
}
