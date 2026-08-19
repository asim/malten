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
