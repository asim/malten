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
// this is exactly what produced the "400 Bad Request" on the next turn.
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

// TestEscalationLeavesReplayableTranscript reproduces the bug where escalating
// dropped the tool_result for the escalating tool_use, corrupting the session
// so the *next* turn 400'd. Both escalation paths are covered: an explicit
// escalate_to_human request and a policy-escalated over-limit refund.
func TestEscalationLeavesReplayableTranscript(t *testing.T) {
	cases := []struct {
		name       string
		customerID string
		first      string
	}{
		{"explicit human request", "CUST-1001", "I want to speak to a human."},
		{"policy-escalated refund", "CUST-1001", "Please refund my annual plan upgrade ORD-5002."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ag, st, err := app.Build(llm.NewStub(), ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()

			const sid = "SESS-test"
			if err := st.CreateSession(sid, tc.customerID); err != nil {
				t.Fatal(err)
			}

			// Turn 1: triggers the escalation.
			r1, err := ag.Handle(context.Background(), sid, tc.customerID, tc.first)
			if err != nil {
				t.Fatalf("first turn: %v", err)
			}
			if !r1.Escalated {
				t.Fatalf("expected first turn to escalate, got %+v", r1)
			}
			assertBalancedTranscript(t, mustLoad(t, st, sid))

			// Turn 2: the follow-up that used to 400. It must succeed and the
			// transcript must remain replayable.
			if _, err := ag.Handle(context.Background(), sid, tc.customerID, "Are you still there?"); err != nil {
				t.Fatalf("follow-up turn after escalation: %v", err)
			}
			assertBalancedTranscript(t, mustLoad(t, st, sid))
		})
	}
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
