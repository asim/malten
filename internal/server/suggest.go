package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/asim/malten/internal/llm"
)

// suggest.go is the ambient agent: instead of a chat box, Malten works in the
// background and offers ONE concrete thing to go and do right now, from the live
// snapshot of your surroundings. No conversation, no typing — just a nudge.

const suggestSystem = `You are Malten, a spatial guide for exploring Great Britain. Given a live snapshot of someone's surroundings right now, suggest ONE specific, appealing thing to do or place to go — ideally something that gets them outside and exploring nearby.

Keep it to one or two short sentences, warm and concrete. If a train or bus in the snapshot helps them reach somewhere good, mention it with its real time. Don't greet, don't offer a list of options, don't use markdown — just the single suggestion, as if nudging a friend out the door.`

func (s *Server) handleSuggest(w http.ResponseWriter, r *http.Request) {
	if !s.askEnabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "suggestions aren't configured"})
		return
	}
	lat, err1 := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lng, err2 := strconv.ParseFloat(r.URL.Query().Get("lng"), 64)
	if err1 != nil || err2 != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "lat and lng are required"})
		return
	}
	snap := s.snapshot(r.Context(), lat, lng)
	if !snap.InGB {
		writeJSON(w, http.StatusOK, map[string]string{"suggestion": ""})
		return
	}

	// Run the agent silently, with no tools — the snapshot already carries the
	// live data, so this is a single, quick completion.
	var b strings.Builder
	err := s.llm.Run(r.Context(), suggestSystem,
		"Here's what's live around me right now:\n"+snapshotText(snap)+"\n\nSuggest one thing to go and do.",
		nil, func(ev llm.Event) {
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
