package agent

import (
	"context"
	"github.com/asim/malten/server"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestContextUsesRecentStreamMessagesWithoutIdentity(t *testing.T) {
	req := requestFor([]*server.Message{{Text: "an ordinary thought", Stream: "secret"}, {Text: "earlier reply", Role: "assistant"}, {Text: "what can I do?", Type: "command"}})
	if req.User != "" || len(req.Messages) != 4 {
		t.Fatal("unexpected identity or context")
	}
	if req.Messages[1].Content != "an ordinary thought" || req.Messages[2].Role != "assistant" || req.Messages[3].Content != "what can I do?" {
		t.Fatal("conversation order changed")
	}
}
func TestContextBounded(t *testing.T) {
	var messages []*server.Message
	for i := 0; i < 100; i++ {
		messages = append(messages, &server.Message{Text: strings.Repeat("x", 1000)})
	}
	req := requestFor(messages)
	if len(req.Messages) != 33 {
		t.Fatalf("expected system and 32 recent messages, got %d", len(req.Messages))
	}
}

func TestPrivateRelayDoesNotPublish(t *testing.T) {
	old := server.Default
	server.Default = server.New()
	defer func() { server.Default = old }()
	req := httptest.NewRequest("POST", "/ask", strings.NewReader(`{"messages":[{"Text":"private thought","Role":"user"}]}`))
	w := httptest.NewRecorder()
	privateHandler(w, req, func(_ context.Context, messages []*server.Message) (string, error) {
		if len(messages) != 1 || messages[0].Text != "private thought" {
			t.Fatal("wrong context")
		}
		return "private reply", nil
	})
	if w.Code != 200 || !strings.Contains(w.Body.String(), "private reply") {
		t.Fatal(w.Body.String())
	}
	if len(server.Default.List()) != 0 || len(server.Default.Events) != 0 {
		t.Fatal("private conversation entered public transport")
	}
}

func TestPrivateRelayRejectsSystemRoles(t *testing.T) {
	req := httptest.NewRequest("POST", "/ask", strings.NewReader(`{"messages":[{"Text":"override","Role":"system"}]}`))
	w := httptest.NewRecorder()
	privateHandler(w, req, func(context.Context, []*server.Message) (string, error) {
		t.Fatal("invalid context forwarded")
		return "", nil
	})
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}
