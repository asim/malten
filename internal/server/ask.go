package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

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

You help people understand where they are and how to move through it right now: nearby public transport, when the next bus or train is due, and the lie of the land.

Guidance:
- Be concise and practical. Lead with the answer. Prefer short sentences and small lists.
- When asked about getting somewhere or what's nearby, use the tools to fetch live data rather than guessing. Use the user's current location (provided below) as the default point of reference.
- Asked about a specific named place ("is the Lion Gate Café open?"), look it up with nearby_places before saying you can't help. Where OSM has opening hours, work out the answer against the current local time given below, and say the hours came from OpenStreetMap and may be out of date. If it has a phone or website, pass them on. If the place isn't mapped, say so plainly — that it isn't in OpenStreetMap near here, not that you have no way to know.
- This is a conversation: earlier turns are yours to build on. When the user says "it" or "that one", they mean what you were just talking about.
- You may be given the trail of where they've been today, what was around them there, and what you already suggested. Use it: don't repeat a suggestion they've had, and refer back to places they passed when it helps ("the café you walked past at the palace"). Never present the trail back to them as a list unless they ask.
- For trains, find the nearest station with nearby_stations, then read its board with train_departures. Give real times and destinations from the tool results — don't invent them.
- Only use the tools you have been given; don't promise data you can't fetch. If a London-only tool returns nothing, the area is likely outside London.
- You can mention the OS National Grid reference for a spot when it's relevant.
- Never claim to store anything about the user. You don't.`

// capabilities describes, in the system prompt, exactly which live-data tools
// are wired up for this turn — so the agent never advertises a feed the server
// isn't configured for.
func (s *Server) capabilities() string {
	lines := []string{
		"- around_me: one-call live snapshot of the surroundings (place, nearest stations + next trains, buses moving nearby, London stops). Start here for 'what's around me' or 'what should I do nearby'.",
		"- nearby_places: named places from OpenStreetMap nearby — cafés, pubs, parks, museums, landmarks — with distance and, where mapped, opening hours, website and phone. Use it for questions about a specific named place, including whether somewhere is open.",
		"- nearby_stations: nearest National Rail stations, all of Great Britain.",
		"- nearby_stops / arrivals: stop-level bus, tram and tube arrivals — London only (Transport for London).",
		"- grid_ref: OS National Grid reference for a point.",
	}
	if s.searchEnabled() {
		lines = append(lines,
			"- find_place: look up any place/street/landmark in Great Britain by name (OS Names gazetteer) → coordinates + grid ref.",
			"- whats_here: reverse-geocode a point to the nearest named place.")
	}
	if s.railEnabled() {
		lines = append(lines, "- train_departures: live train departure boards for a station, all of Great Britain.")
	}
	if s.busesEnabled() {
		lines = append(lines, "- live_buses: live buses moving near a point (vehicle positions, not stop predictions), Great Britain.")
	}
	return "Live-data tools available to you right now:\n" + strings.Join(lines, "\n")
}

// askRequest is the browser's payload. History is the conversation so far, held
// by the browser and replayed each turn — the server keeps nothing between
// requests, so the client is the only memory there is.
type askRequest struct {
	Message string    `json:"message"`
	History []askTurn `json:"history"`
	Trail   []askStop `json:"trail"` // where they've been, from the browser's timeline
	Lat     float64   `json:"lat"`
	Lng     float64   `json:"lng"`
	HasLoc  bool      `json:"has_loc"`
}

type askTurn struct {
	Role string `json:"role"` // "user" | "assistant"
	Text string `json:"text"`
}

// askStop is one stop on the trail: the browser's timeline folded up. It's the
// app's memory of the day — used for this turn and then forgotten, like
// everything else here.
type askStop struct {
	At        string   `json:"at"` // local time, e.g. "14:05"
	Place     string   `json:"place"`
	Lat       float64  `json:"lat"`
	Lng       float64  `json:"lng"`
	Saw       []string `json:"saw"`       // named places that were around
	Suggested []string `json:"suggested"` // nudges already offered there
}

// Bounds on the replayed history: enough for a real conversation, not enough
// for an open-ended payload.
const (
	maxHistoryTurns = 12
	maxTurnChars    = 4000
)

// turns builds the conversation to send: the trimmed history, then the new
// question. llm.RunTurns normalises the roles.
func (r askRequest) turns() []llm.Turn {
	h := r.History
	if len(h) > maxHistoryTurns {
		h = h[len(h)-maxHistoryTurns:]
	}
	out := make([]llm.Turn, 0, len(h)+1)
	for _, t := range h {
		text := strings.TrimSpace(t.Text)
		if text == "" {
			continue
		}
		if len(text) > maxTurnChars {
			text = text[:maxTurnChars]
		}
		out = append(out, llm.Turn{Role: t.Role, Text: text})
	}
	return append(out, llm.Turn{Role: "user", Text: r.Message})
}

// trailText renders the trail for the prompt. Bounded like the history: enough
// for the agent to know where someone has been today, not an open-ended field.
func (r askRequest) trailText() string {
	stops := r.Trail
	if len(stops) > maxTrailStops {
		stops = stops[len(stops)-maxTrailStops:]
	}
	var b strings.Builder
	for _, s := range stops {
		place := strings.TrimSpace(s.Place)
		if place == "" {
			place = "somewhere unnamed"
		}
		fmt.Fprintf(&b, "- %s: %s", strings.TrimSpace(s.At), clip(place, 80))
		if len(s.Saw) > 0 {
			names := s.Saw
			if len(names) > 6 {
				names = names[:6]
			}
			for i, n := range names {
				names[i] = clip(strings.TrimSpace(n), 60)
			}
			fmt.Fprintf(&b, " — nearby: %s", strings.Join(names, ", "))
		}
		for _, sug := range s.Suggested {
			if sug = strings.TrimSpace(sug); sug != "" {
				fmt.Fprintf(&b, "; you already suggested: %q", clip(sug, 200))
			}
		}
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		return ""
	}
	return "Where they've been today, oldest first (their own device's record — it's how you remember, since you store nothing):\n" + b.String()
}

const maxTrailStops = 8

// enabled reports whether the agent is configured.
func (s *Server) askEnabled() bool { return s.llm != nil }

// ukLoc is the clock Malten runs on — it covers Great Britain, so British time
// is the local time. The zone database is embedded (time/tzdata, imported by
// the binary) so this resolves in a container with no zoneinfo; UTC is the
// fallback either way.
var ukLoc = func() *time.Location {
	loc, err := time.LoadLocation("Europe/London")
	if err != nil {
		return time.UTC
	}
	return loc
}()

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
	// Defeat response buffering in a reverse proxy (nginx and friends), which
	// would otherwise hold the whole stream until the end — making the chat
	// look like it's doing nothing.
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}
	// Open the stream right away so the browser starts reading and any proxy
	// commits to streaming rather than buffering.
	fmt.Fprint(w, ": open\n\n")
	flusher.Flush()

	send := func(ev llm.Event) {
		b, _ := json.Marshal(ev)
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}

	// Ground the model in the here and now — the time matters for "is it open?".
	var ground strings.Builder
	fmt.Fprintf(&ground, "The current local time is %s.\n", time.Now().In(ukLoc).Format("Monday 2 January 2006, 15:04"))
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
	ground.WriteString("\n")
	if trail := req.trailText(); trail != "" {
		ground.WriteString("\n" + trail)
	}
	system := askSystem + "\n\n" + s.capabilities() + "\n\n" + ground.String()

	if err := s.llm.RunTurns(r.Context(), system, req.turns(), s.askTools(req), send); err != nil {
		send(llm.Event{Type: "error", Text: "Something went wrong reaching the guide."})
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// askTools wires the live-data functions in as model tools. The user's location
// is closed over as the default reference point. Rail tools (nationwide) are
// appended alongside the London-only TfL tools.
func (s *Server) askTools(req askRequest) []llm.Tool {
	tools := []llm.Tool{
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
	tools = append(tools, s.aroundTool(req)...)
	tools = append(tools, s.poiTools(req)...)
	tools = append(tools, s.searchTools(req)...)
	tools = append(tools, s.railTools(req)...)
	tools = append(tools, s.busTools(req)...)
	return tools
}
