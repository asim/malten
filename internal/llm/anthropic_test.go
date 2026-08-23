package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sse writes a minimal Messages API stream to w.
func sse(w http.ResponseWriter, lines ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, l := range lines {
		_, _ = w.Write([]byte("data: " + l + "\n\n"))
	}
	w.(http.Flusher).Flush()
}

// The conversation comes from a browser, so it can arrive in any shape. The
// Messages API won't accept just any shape, hence normalise.
func TestNormalise(t *testing.T) {
	got := normalise([]Turn{
		{Role: "assistant", Text: "dropped: nothing to answer"},
		{Role: "user", Text: "where am I?"},
		{Role: "", Text: "…and what's nearby?"}, // unknown role reads as user
		{Role: "assistant", Text: "Near Hampton Court."},
		{Role: "user", Text: "   "}, // empty turns vanish
		{Role: "user", Text: "is the café open?"},
	})
	if len(got) != 3 {
		t.Fatalf("got %d messages, want 3: %+v", len(got), got)
	}
	if got[0].Role != "user" || got[0].Content[0].Text != "where am I?\n\n…and what's nearby?" {
		t.Errorf("consecutive user turns not merged: %+v", got[0])
	}
	if got[1].Role != "assistant" || got[2].Role != "user" {
		t.Errorf("roles must alternate: %+v", got)
	}
	if got[2].Content[0].Text != "is the café open?" {
		t.Errorf("last turn = %q", got[2].Content[0].Text)
	}
	// A trailing assistant turn leaves nothing to answer, so it goes.
	if n := normalise([]Turn{{Role: "user", Text: "hi"}, {Role: "assistant", Text: "hello"}}); len(n) != 1 {
		t.Errorf("trailing assistant turn kept: %+v", n)
	}
	if n := normalise(nil); len(n) != 0 {
		t.Errorf("empty conversation = %+v", n)
	}
}

// TestRunTurnsSendsHistory checks the whole conversation reaches the API, so a
// follow-up like "how far is it?" has something to refer to.
func TestRunTurnsSendsHistory(t *testing.T) {
	var got request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		sse(w,
			`{"type":"content_block_start","index":0,"content_block":{"type":"text"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"300m north."}}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
			`{"type":"message_stop"}`,
		)
	}))
	defer srv.Close()

	c := New("test-key", "")
	c.URL = srv.URL
	var out strings.Builder
	err := c.RunTurns(context.Background(), "sys", []Turn{
		{Role: "user", Text: "is the café open?"},
		{Role: "assistant", Text: "Until 17:00."},
		{Role: "user", Text: "how far is it?"},
	}, nil, func(ev Event) {
		if ev.Type == "text" {
			out.WriteString(ev.Text)
		}
	})
	if err != nil {
		t.Fatalf("RunTurns: %v", err)
	}
	if len(got.Messages) != 3 {
		t.Fatalf("sent %d messages, want 3", len(got.Messages))
	}
	if got.Messages[1].Role != "assistant" || got.Messages[1].Content[0].Text != "Until 17:00." {
		t.Errorf("prior answer not replayed: %+v", got.Messages[1])
	}
	if out.String() != "300m north." {
		t.Errorf("streamed %q", out.String())
	}
}

// TestRunToolLoop runs a two-turn conversation: the first response asks for a
// tool, the second (after the tool result) streams a final answer. It verifies
// the loop runs the tool, feeds the result back, and streams text to emit.
func TestRunToolLoop(t *testing.T) {
	var turn int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Assert required headers on every request.
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("missing api key header")
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Errorf("missing anthropic-version header")
		}
		var req request
		_ = json.NewDecoder(r.Body).Decode(&req)

		if turn == 0 {
			turn++
			// First turn: emit a tool_use block for "ping".
			sse(w,
				`{"type":"message_start"}`,
				`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tu_1","name":"ping"}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"x\":"}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"42}"}}`,
				`{"type":"content_block_stop","index":0}`,
				`{"type":"message_delta","delta":{"stop_reason":"tool_use"}}`,
				`{"type":"message_stop"}`,
			)
			return
		}
		// Second turn: the last message must be a tool_result carrying "pong".
		last := req.Messages[len(req.Messages)-1]
		if last.Role != "user" || len(last.Content) == 0 || last.Content[0].Type != "tool_result" {
			t.Errorf("expected tool_result as last message, got %+v", last)
		}
		if !strings.Contains(last.Content[0].Content, "pong:42") {
			t.Errorf("tool result not fed back: %q", last.Content[0].Content)
		}
		sse(w,
			`{"type":"content_block_start","index":0,"content_block":{"type":"text"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"all "}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"good"}}`,
			`{"type":"content_block_stop","index":0}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
			`{"type":"message_stop"}`,
		)
	}))
	defer srv.Close()

	c := New("test-key", "")
	c.URL = srv.URL

	var ran bool
	tools := []Tool{{
		Name:   "ping",
		Schema: map[string]any{"type": "object"},
		Run: func(ctx context.Context, in json.RawMessage) (string, error) {
			ran = true
			var a struct {
				X int `json:"x"`
			}
			_ = json.Unmarshal(in, &a)
			return "pong:" + itoa(a.X), nil
		},
	}}

	var text, toolsSeen strings.Builder
	err := c.Run(context.Background(), "sys", "hi", tools, func(ev Event) {
		switch ev.Type {
		case "text":
			text.WriteString(ev.Text)
		case "tool":
			toolsSeen.WriteString(ev.Tool)
		}
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !ran {
		t.Errorf("tool was never run")
	}
	if got := text.String(); got != "all good" {
		t.Errorf("streamed text = %q, want %q", got, "all good")
	}
	if toolsSeen.String() != "ping" {
		t.Errorf("tool event = %q, want ping", toolsSeen.String())
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
