package server

import (
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
