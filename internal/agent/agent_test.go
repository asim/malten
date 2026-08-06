package agent_test

import (
	"context"
	"testing"

	"github.com/asim/malten/internal/app"
	"github.com/asim/malten/internal/llm"
)

// assertBalancedTranscript checks the invariant the Anthropic Messages API
// enforces: every tool_use block must be answered by a tool_result (with a
// matching id) in the immediately following message. A transcript that breaks
// this produces a "400 Bad Request" on the next turn.
func assertBalancedTranscript(t *testing.T, msgs []llm.Message) {
	t.Helper()
	for i, m := range msgs {
		var wantIDs []string
		for _, b := range m.Content {
			if b.Type == llm.BlockToolUse {
				wantIDs = append(wantIDs, b.ID)
			}
		}
		if len(wantIDs) == 0 {
			continue
		}
		if i+1 >= len(msgs) {
			t.Fatalf("message %d has tool_use blocks but no following message to answer them", i)
		}
		got := map[string]bool{}
		for _, b := range msgs[i+1].Content {
			if b.Type == llm.BlockToolResult {
				got[b.ToolUseID] = true
			}
		}
		for _, id := range wantIDs {
			if !got[id] {
				t.Fatalf("tool_use %q in message %d has no matching tool_result in message %d", id, i, i+1)
			}
		}
	}
}

// TestToolTurnLeavesReplayableTranscript runs a turn that calls a tool
// (create_issue) followed by a plain follow-up, and asserts the stored
// transcript stays balanced so it replays cleanly on later turns.
func TestToolTurnLeavesReplayableTranscript(t *testing.T) {
	ag, st, err := app.Build(llm.NewStub(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	const sid = "SESS-test"
	if err := st.CreateSession(sid); err != nil {
		t.Fatal(err)
	}

	if _, err := ag.Handle(context.Background(), sid, "I keep procrastinating on my thesis. Can we make a plan for it?"); err != nil {
		t.Fatalf("first turn: %v", err)
	}
	assertBalancedTranscript(t, mustLoad(t, st, sid))

	if _, err := ag.Handle(context.Background(), sid, "Thanks, that helps."); err != nil {
		t.Fatalf("follow-up turn: %v", err)
	}
	assertBalancedTranscript(t, mustLoad(t, st, sid))
}

func mustLoad(t *testing.T, st interface {
	LoadMessages(string) ([]llm.Message, error)
}, sid string) []llm.Message {
	t.Helper()
	msgs, err := st.LoadMessages(sid)
	if err != nil {
		t.Fatal(err)
	}
	return msgs
}
