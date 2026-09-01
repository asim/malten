package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/asim/malten/internal/llm"
)

// The timeline composer reads /api/ask as a raw SSE stream and parses each
// `data:` line as an llm.Event. This pins that wire format: the field names the
// browser looks for, and the [DONE] sentinel it stops on.
func TestAskStreamsEvents(t *testing.T) {
	anthropic := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, l := range []string{
			`{"type":"content_block_start","index":0,"content_block":{"type":"text"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Two stops "}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"that way."}}`,
			`{"type":"content_block_stop","index":0}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
			`{"type":"message_stop"}`,
		} {
			_, _ = w.Write([]byte("data: " + l + "\n\n"))
		}
		w.(http.Flusher).Flush()
	}))
	defer anthropic.Close()

	c := llm.New("test-key", "")
	c.URL = anthropic.URL
	s := &Server{llm: c}

	req := httptest.NewRequest(http.MethodPost, "/api/ask",
		strings.NewReader(`{"message":"what's nearby?","lat":51.5,"lng":-0.12,"has_loc":true}`))
	rec := httptest.NewRecorder()
	s.handleAsk(rec, req)

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	// Reverse proxies must not buffer the stream, or the answer arrives all at once.
	if rec.Header().Get("X-Accel-Buffering") != "no" {
		t.Errorf("missing X-Accel-Buffering: no")
	}
	body := rec.Body.String()
	for _, want := range []string{
		`"Type":"text"`,      // the field name the client switches on
		`"Text":"Two stops `, // streamed token text
		"data: [DONE]",       // the sentinel that ends the client's loop
	} {
		if !strings.Contains(body, want) {
			t.Errorf("stream missing %q\n%s", want, body)
		}
	}
}

// The browser's timeline is the app's only memory, so it travels with the
// question. This checks it reaches the prompt in a form the model can use, and
// stays bounded.
func TestTrailText(t *testing.T) {
	req := askRequest{Trail: []askStop{
		{At: "13:40", Place: "Hampton Court", Saw: []string{"Lion Gate Café", "Hampton Court Palace"}, Suggested: []string{"Walk the maze before the light goes."}},
		{At: "14:05", Place: "Bushy Park", Saw: []string{"Pheasantry Café"}},
		{At: "14:30", Place: "", Saw: nil},
	}}
	got := req.trailText()
	for _, want := range []string{
		"13:40: Hampton Court",
		"nearby: Lion Gate Café, Hampton Court Palace",
		`you already suggested: "Walk the maze before the light goes."`,
		"14:05: Bushy Park",
		"somewhere unnamed",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("trail missing %q:\n%s", want, got)
		}
	}

	// No trail (a first visit) adds nothing to the prompt.
	if s := (askRequest{}).trailText(); s != "" {
		t.Errorf("empty trail rendered %q", s)
	}

	// Long trails are trimmed to the most recent stops.
	long := askRequest{}
	for i := 0; i < 30; i++ {
		long.Trail = append(long.Trail, askStop{At: "10:00", Place: fmt.Sprintf("Stop %d", i)})
	}
	got = long.trailText()
	if strings.Contains(got, "Stop 5") {
		t.Errorf("old stops not trimmed:\n%s", got)
	}
	if !strings.Contains(got, "Stop 29") {
		t.Errorf("newest stop dropped:\n%s", got)
	}
	if n := strings.Count(got, "\n- ") + 1; n > maxTrailStops+1 {
		t.Errorf("rendered %d stops, want at most %d", n, maxTrailStops)
	}
}

// A note is the person talking to themselves. It has to reach the model as
// context — and be bounded, like everything else the browser sends.
func TestNotesText(t *testing.T) {
	req := askRequest{Notes: []askNote{
		{Text: "That bakery on the corner opens at 7", Place: "Hampton Court", When: "Tue 08:12"},
		{Text: "  ", Place: "nowhere"},
		{Text: "Ask about the allotment waiting list"},
	}}
	got := req.notesText()
	for _, want := range []string{
		`"That bakery on the corner opens at 7" (at Hampton Court, Tue 08:12)`,
		`"Ask about the allotment waiting list"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("notes missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "nowhere") {
		t.Errorf("an empty note was rendered:\n%s", got)
	}
	if s := (askRequest{}).notesText(); s != "" {
		t.Errorf("no notes rendered %q", s)
	}

	// Only the most recent few travel.
	var many askRequest
	for i := 0; i < 20; i++ {
		many.Notes = append(many.Notes, askNote{Text: fmt.Sprintf("note %d", i)})
	}
	got = many.notesText()
	if strings.Contains(got, "note 3\"") {
		t.Errorf("old notes not trimmed:\n%s", got)
	}
	if !strings.Contains(got, "note 19") {
		t.Errorf("newest note dropped:\n%s", got)
	}
	if n := strings.Count(got, "\n- "); n > maxNotes {
		t.Errorf("rendered %d notes, want at most %d", n, maxNotes)
	}
}

// The whole conversation and the trail must reach the model in one turn.
func TestAskSendsHistoryAndTrail(t *testing.T) {
	var got struct {
		System   string `json:"system"`
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	anthropic := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "text/event-stream")
		for _, l := range []string{
			`{"type":"content_block_start","index":0,"content_block":{"type":"text"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
			`{"type":"message_stop"}`,
		} {
			_, _ = w.Write([]byte("data: " + l + "\n\n"))
		}
		w.(http.Flusher).Flush()
	}))
	defer anthropic.Close()

	c := llm.New("test-key", "")
	c.URL = anthropic.URL
	s := &Server{llm: c}

	body := `{"message":"anywhere new to go?",
	          "history":[{"role":"user","text":"is the café open?"},{"role":"assistant","text":"Until 17:00."}],
	          "trail":[{"at":"13:40","place":"Hampton Court","saw":["Lion Gate Café"],"suggested":["Walk the maze."]}],
	          "notes":[{"text":"Try the bakery on the corner","place":"Hampton Court","when":"Tue 08:12"}],
	          "lat":51.4036,"lng":-0.3378,"has_loc":true}`
	s.handleAsk(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/ask", strings.NewReader(body)))

	if len(got.Messages) != 3 {
		t.Fatalf("sent %d messages, want 3 (history + question)", len(got.Messages))
	}
	if got.Messages[2].Content[0].Text != "anywhere new to go?" {
		t.Errorf("last message = %q", got.Messages[2].Content[0].Text)
	}
	for _, want := range []string{
		"Hampton Court",                // the trail
		"Walk the maze.",               // …including what was already suggested
		"Try the bakery on the corner", // and the notes they left nearby
		"OS National Grid",             // the location grounding
		"The current local time is",    // and the clock
	} {
		if !strings.Contains(got.System, want) {
			t.Errorf("system prompt missing %q:\n%s", want, got.System)
		}
	}
}

// Without an Anthropic key the composer is hidden, but a POST must still fail
// politely rather than panic on a nil client.
func TestAskDisabled(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	s.handleAsk(rec, httptest.NewRequest(http.MethodPost, "/api/ask", strings.NewReader(`{"message":"hi"}`)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}
