package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/asim/malten/internal/llm"
)

// hunt.go makes a short scavenger hunt for wherever someone is standing, for
// going out with children. It's the same agent and the same live data as the
// rest of the app — the difference is who it's written for.
//
// The hard part isn't generating a list, it's not lying to a six-year-old. A
// hunt that names a statue which isn't there ends the walk badly, so the model
// is given the real named places nearby and told to invent nothing: anything not
// on that list has to be something you'd find on almost any street.

const huntSystem = `You are making a small treasure hunt for a child to do outdoors right now, where they are standing.

Rules:
- Give exactly five things to find. Each one must be findable on foot, from a pavement or path, within a few hundred metres.
- You will be given the real named places nearby. You may use those by name. Anything else must be something found on almost any street or park in Britain — a postbox, a bench, a drain cover with writing on it, a bird with a forked tail, a door that isn't the usual colour, moss growing on something. NEVER invent a named building, statue, shop or landmark that isn't in the list you were given: a child looking for something that isn't there is worse than a dull hunt.
- Write for the child's age, given below. At six that means short sentences, ordinary words, nothing to read off a sign, and nothing needing a phone.
- Vary them: something to spot, something to count, something to touch, something to listen for, something that needs looking up or down.
- Take the weather and the time of day into account. Do not send anyone across a busy road, into water, or anywhere that needs a ticket.

Reply with JSON only, no markdown fence, in exactly this shape:
{"items":[{"what":"…","hint":"…"}]}
"what" is the thing to find, at most about eight words. "hint" is one short line telling them where to look or what to notice — it may be empty. Five items, no more, no less.`

type huntRequest struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
	Age int     `json:"age"`
}

type huntItem struct {
	What string `json:"what"`
	Hint string `json:"hint,omitempty"`
}

func (s *Server) handleHunt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST only"})
		return
	}
	if !s.askEnabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "hunts aren't configured"})
		return
	}
	var req huntRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})
		return
	}
	if req.Age < 2 || req.Age > 16 {
		req.Age = 6
	}

	snap := s.snapshot(r.Context(), req.Lat, req.Lng)
	if !snap.InGB {
		writeJSON(w, http.StatusOK, map[string]any{"items": []huntItem{}})
		return
	}

	var msg strings.Builder
	msg.WriteString("Here's what's really around them right now:\n")
	msg.WriteString(snapshotText(snap))
	// The named places are the only specifics the model is allowed to use.
	if pois, err := fetchPOIs(req.Lat, req.Lng, 500); err == nil && len(pois) > 0 {
		if len(pois) > 12 {
			pois = pois[:12]
		}
		msg.WriteString("Named places within a few minutes' walk (the only ones you may name):\n")
		for _, p := range pois {
			fmt.Fprintf(&msg, "- %s (%s, %dm away)\n", p.Name, p.Kind, int(metresBetween(req.Lat, req.Lng, p.Lat, p.Lng)))
		}
	} else {
		msg.WriteString("No named places are mapped nearby, so use only things found on any street.\n")
	}
	fmt.Fprintf(&msg, "\nThe child is %d years old. Make the hunt.\n", req.Age)

	var b strings.Builder
	if err := s.llm.Run(r.Context(), huntSystem, msg.String(), nil, func(ev llm.Event) {
		if ev.Type == "text" {
			b.WriteString(ev.Text)
		}
	}); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []huntItem{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": parseHunt(b.String())})
}

// parseHunt pulls the item list out of the model's reply. It tolerates a
// markdown fence or a stray sentence around the JSON, and drops anything
// malformed rather than showing a child an empty line.
func parseHunt(text string) []huntItem {
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return []huntItem{}
	}
	var out struct {
		Items []huntItem `json:"items"`
	}
	if err := json.Unmarshal([]byte(text[start:end+1]), &out); err != nil {
		return []huntItem{}
	}
	items := make([]huntItem, 0, len(out.Items))
	for _, it := range out.Items {
		it.What = clip(strings.TrimSpace(it.What), 90)
		it.Hint = clip(strings.TrimSpace(it.Hint), 140)
		if it.What == "" {
			continue
		}
		items = append(items, it)
		if len(items) == 5 {
			break
		}
	}
	return items
}
