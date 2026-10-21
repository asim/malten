package agent

import "testing"

func TestCompletePreservesConversationOrder(t *testing.T) {
	req := complete("next thought", "notes", "listen kindly", Context{Prompt: "first thought", Reply: "heard"})
	want := []struct{ role, content string }{
		{"system", "listen kindly"}, {"user", "first thought"}, {"assistant", "heard"}, {"user", "next thought"},
	}
	if len(req.Messages) != len(want) {
		t.Fatalf("got %d messages", len(req.Messages))
	}
	for i, w := range want {
		if req.Messages[i].Role != w.role || req.Messages[i].Content != w.content {
			t.Fatalf("message %d: %+v", i, req.Messages[i])
		}
	}
	if req.User != "notes" {
		t.Fatal("missing stream identity")
	}
}
