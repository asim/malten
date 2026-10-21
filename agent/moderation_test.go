package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/asim/malten/server"
	"github.com/sashabaranov/go-openai"
)

func TestPublicModeratorDecisions(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		want          error
	}{
		{"allow", `{"allow":true,"reason":"allowed"}`, nil},
		{"reject", `{"allow":false,"reason":"abuse"}`, server.ErrModerationRejected},
		{"malformed", `not json`, server.ErrModerationUnavailable},
		{"missing decision", `{"reason":"allowed"}`, server.ErrModerationUnavailable},
		{"inconsistent", `{"allow":true,"reason":"abuse"}`, server.ErrModerationUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Messages []struct {
						Role    string
						Content json.RawMessage
					}
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				if len(req.Messages) != 2 || req.Messages[0].Role != "system" {
					t.Error("policy missing")
				}
				var parts []map[string]any
				if err := json.Unmarshal(req.Messages[1].Content, &parts); err != nil || len(parts) != 2 {
					t.Error("text and image not sent together")
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": tc.content}}}})
			}))
			defer api.Close()
			cfg := openai.DefaultConfig("test")
			cfg.BaseURL = api.URL
			moderate := newPublicModerator(openai.NewClientWithConfig(cfg), "")
			err := moderate(context.Background(), &server.Message{Text: "I am overwhelmed", Image: "data:image/png;base64,test"})
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestPublicModeratorFailsClosed(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "upstream down", 500) }))
	defer api.Close()
	cfg := openai.DefaultConfig("test")
	cfg.BaseURL = api.URL
	moderate := newPublicModerator(openai.NewClientWithConfig(cfg), "")
	if err := moderate(context.Background(), &server.Message{Text: "hello"}); !errors.Is(err, server.ErrModerationUnavailable) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := moderate(ctx, &server.Message{Text: "hello"}); !errors.Is(err, server.ErrModerationUnavailable) {
		t.Fatal(err)
	}
}
