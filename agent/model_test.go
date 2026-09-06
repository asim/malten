package agent

import (
	"context"
	"encoding/json"
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
