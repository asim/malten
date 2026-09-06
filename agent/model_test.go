package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type modelTransport func(*http.Request) (*http.Response, error)

func (t modelTransport) RoundTrip(r *http.Request) (*http.Response, error) { return t(r) }
func TestModelUsesObjectiveAndPhotoWithStrictDecision(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	old := http.DefaultClient
	defer func() { http.DefaultClient = old }()
	stop := "end_turn"
	http.DefaultClient = &http.Client{Transport: modelTransport(func(r *http.Request) (*http.Response, error) {
		var req struct {
			System   string
			Messages []struct{ Content []struct{ Type, Text string } }
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(req.System, "UNTRUSTED") || !strings.Contains(req.Messages[0].Content[0].Text, "Praise and gratitude") {
			t.Fatal("missing purpose or instruction boundary")
		}
		if len(req.Messages[0].Content) != 3 || req.Messages[0].Content[2].Type != "image" {
			t.Fatal("photo was not available for observation")
		}
		raw, _ := json.Marshal(map[string]any{"stop_reason": stop, "content": []any{map[string]string{"type": "text", "text": `{"Summary":"A quiet moment","Action":null,"Evidence":[]}`}}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})}
	view := View{Now: time.Now(), Objective: "Praise and gratitude", Observations: []Observation{{ID: "photo", Photo: "data:image/jpeg;base64,test"}}}
	d, err := Decide(context.Background(), view)
	if err != nil || d.Action != nil || d.Summary == "" {
		t.Fatalf("%+v %v", d, err)
	}
	stop = "max_tokens"
	if _, err := Decide(context.Background(), view); err == nil {
		t.Fatal("accepted incomplete decision")
	}
}

func TestToolRoundsAreBoundedAndErrorsAreData(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	old := http.DefaultClient
	defer func() { http.DefaultClient = old }()
	calls, rounds := 0, 0
	http.DefaultClient = &http.Client{Transport: modelTransport(func(r *http.Request) (*http.Response, error) {
		rounds++
		var req struct {
			Messages   []struct{ Content json.RawMessage }
			ToolChoice struct {
				Type string `json:"type"`
			} `json:"tool_choice"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if rounds > 1 {
			body := string(req.Messages[len(req.Messages)-1].Content)
			if !strings.Contains(body, `"is_error":true`) || strings.Contains(body, "sensitive upstream detail") {
				t.Fatal("tool failures must be bounded data, not exposed error details")
			}
		}
		if rounds == 3 && req.ToolChoice.Type != "none" {
			t.Fatal("tools still enabled after retrieval budget")
		}
		var content []any
		for _, id := range []string{"one", "two", "three", "four"} {
			content = append(content, map[string]any{"type": "tool_use", "id": id, "name": "search", "input": map[string]string{"query": "patience"}})
		}
		raw, _ := json.Marshal(map[string]any{"stop_reason": "tool_use", "content": content})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})}
	tool := Tool{Name: "search", InputSchema: json.RawMessage(`{"type":"object"}`), Call: func(context.Context, json.RawMessage) (json.RawMessage, error) {
		calls++
		return nil, fmt.Errorf("sensitive upstream detail")
	}}
	if _, err := CompleteWithTools(context.Background(), "test", "test", []Tool{tool}); err == nil {
		t.Fatal("accepted an unfinished tool loop")
	}
	if calls != 6 || rounds != 3 {
		t.Fatalf("unbounded calls: %d tools, %d model calls", calls, rounds)
	}
}
