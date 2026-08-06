package llm

import "testing"

// unmatched returns the tool_use ids that have no tool_result in the very next
// message — the condition the Messages API rejects with a 400.
func unmatched(msgs []Message) []string {
	var bad []string
	for i, m := range msgs {
		answered := map[string]bool{}
		if i+1 < len(msgs) {
			for _, b := range msgs[i+1].Content {
				if b.Type == BlockToolResult {
					answered[b.ToolUseID] = true
				}
			}
		}
		for _, b := range m.Content {
			if b.Type == BlockToolUse && !answered[b.ID] {
				bad = append(bad, b.ID)
			}
		}
	}
	return bad
}

func TestBalanceToolResults(t *testing.T) {
	cases := []struct {
		name string
		in   []Message
	}{
		{
			name: "orphaned tool_use followed by assistant text (the escalation bug)",
			in: []Message{
				UserText("I want a human"),
				{Role: RoleAssistant, Content: []Block{ToolUse("t1", "escalate_to_human", []byte(`{}`))}},
				{Role: RoleAssistant, Content: []Block{Text("Escalated. You'll hear back soon.")}},
				UserText("Are you there?"),
			},
		},
		{
			name: "tool_use with no following message at all",
			in: []Message{
				UserText("refund please"),
				{Role: RoleAssistant, Content: []Block{ToolUse("t2", "issue_refund", []byte(`{}`))}},
			},
		},
		{
			name: "partially answered batch",
			in: []Message{
				UserText("do two things"),
				{Role: RoleAssistant, Content: []Block{
					ToolUse("a", "search", []byte(`{}`)),
					ToolUse("b", "account_lookup", []byte(`{}`)),
				}},
				{Role: RoleUser, Content: []Block{ToolResult("a", "ok", false)}}, // b missing
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := unmatched(balanceToolResults(tc.in)); len(got) != 0 {
				t.Fatalf("balanceToolResults left unmatched tool_use ids: %v", got)
			}
		})
	}
}

// A healthy transcript must be returned unchanged in shape.
func TestBalanceToolResultsLeavesValidTranscriptAlone(t *testing.T) {
	in := []Message{
		UserText("export data?"),
		{Role: RoleAssistant, Content: []Block{ToolUse("t1", "search", []byte(`{}`))}},
		{Role: RoleUser, Content: []Block{ToolResult("t1", "here you go", false)}},
		{Role: RoleAssistant, Content: []Block{Text("You can export from Settings.")}},
	}
	out := balanceToolResults(in)
	if len(out) != len(in) {
		t.Fatalf("valid transcript changed length: %d -> %d", len(in), len(out))
	}
	if got := unmatched(out); len(got) != 0 {
		t.Fatalf("unexpected unmatched ids: %v", got)
	}
}
