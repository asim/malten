package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/asim/malten/internal/llm"
)

// suggest.go is the ambient agent: instead of a chat box, Malten works in the
// background and offers ONE concrete thing to go and do right now, from the live
// snapshot of your surroundings. It's a loop, not a conversation — the client
// can ask for a different idea (avoiding what it already saw) or the next thing
// to do after finishing one.

const suggestSystem = `You are Malten, a spatial guide for exploring Great Britain. Given a live snapshot of someone's surroundings right now, suggest ONE specific, appealing thing to do or place to go — ideally something that gets them outside and exploring nearby.

Keep it to one or two short sentences, warm and concrete. If a train or bus in the snapshot helps them reach somewhere good, mention it with its real time. Don't greet, don't offer a list of options, don't use markdown — just the single suggestion, as if nudging a friend out the door.`

type suggestRequest struct {
	Lat   float64  `json:"lat"`
	Lng   float64  `json:"lng"`
	Mode  string   `json:"mode"`  // "fresh" | "different" | "next"
	Last  string   `json:"last"`  // the suggestion just shown (for "next"/"different")
	Avoid []string `json:"avoid"` // recent suggestions to not repeat
}

func (s *Server) handleSuggest(w http.ResponseWriter, r *http.Request) {
	if !s.askEnabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "suggestions aren't configured"})
		return
	}
	var req suggestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})
		return
	}

	snap := s.snapshot(r.Context(), req.Lat, req.Lng)
	if !snap.InGB {
		writeJSON(w, http.StatusOK, map[string]string{"suggestion": ""})
		return
	}

	// Build the turn: the live snapshot, plus what the person wants next.
	var msg strings.Builder
	msg.WriteString("Here's what's live around me right now:\n")
	msg.WriteString(snapshotText(snap))

	if len(req.Avoid) > 6 {
		req.Avoid = req.Avoid[len(req.Avoid)-6:]
	}
	switch req.Mode {
	case "next":
		if strings.TrimSpace(req.Last) != "" {
			fmt.Fprintf(&msg, "\nI just did this: %q. Suggest a natural NEXT thing to do from there — build on it, ideally near where that leaves me.\n", req.Last)
		} else {
			msg.WriteString("\nSuggest the next thing to go and do.\n")
		}
	case "different":
		msg.WriteString("\nGive me a DIFFERENT idea — genuinely different in kind or direction from these, which I've already seen:\n")
		for _, a := range req.Avoid {
			fmt.Fprintf(&msg, "- %s\n", a)
		}
	default:
		if len(req.Avoid) > 0 {
			msg.WriteString("\nDon't repeat these, which I've already seen:\n")
			for _, a := range req.Avoid {
				fmt.Fprintf(&msg, "- %s\n", a)
			}
		}
		msg.WriteString("\nSuggest one thing to go and do.\n")
	}

	// Run the agent silently, no tools — the snapshot carries the live data, so
	// this is a single quick completion.
	var b strings.Builder
	err := s.llm.Run(r.Context(), suggestSystem, msg.String(), nil, func(ev llm.Event) {
		if ev.Type == "text" {
			b.WriteString(ev.Text)
		}
	})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"suggestion": ""})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"suggestion": strings.TrimSpace(b.String())})
}

// snapshotText renders an around snapshot as compact prose for the model.
func snapshotText(a aroundSnapshot) string {
	var b strings.Builder
	if a.Weather != nil {
		fmt.Fprintf(&b, "Weather: %s, %.0f°C, wind %.0f km/h.\n", a.Weather.Text, a.Weather.TempC, a.Weather.WindKph)
	}
	if a.Place != nil {
		fmt.Fprintf(&b, "Location: near %s", a.Place.Name)
		if a.Place.Type != "" {
			fmt.Fprintf(&b, " (%s)", a.Place.Type)
		}
		if a.Place.County != "" {
			fmt.Fprintf(&b, ", %s", a.Place.County)
		}
		b.WriteString(".\n")
	} else if a.GridRef != "" {
		fmt.Fprintf(&b, "Location: OS grid %s.\n", a.GridRef)
	}
	for _, st := range a.Stations {
		fmt.Fprintf(&b, "Rail station: %s (%.1f mi away)", st.Name, st.Miles)
		if len(st.Next) > 0 {
			parts := make([]string, 0, len(st.Next))
			for _, d := range st.Next {
				parts = append(parts, fmt.Sprintf("%s to %s (%s)", d.Scheduled, d.Destination, d.Expected))
			}
			fmt.Fprintf(&b, " — next departures: %s", strings.Join(parts, "; "))
		}
		b.WriteString(".\n")
	}
	if a.BusesNearby > 0 {
		fmt.Fprintf(&b, "%d buses are moving nearby.\n", a.BusesNearby)
	}
	if a.StopsNearby > 0 {
		fmt.Fprintf(&b, "%d London bus/tram/tube stops nearby.\n", a.StopsNearby)
	}
	return b.String()
}
