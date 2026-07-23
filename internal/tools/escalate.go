package tools

import (
	"context"
	"encoding/json"

	"github.com/asim/malten/internal/llm"
)

// Escalate implements escalate_to_human(reason). It is terminal: the agent
// intercepts a call to this tool, records an escalation in the backlog and ends
// the loop. Execute here is a fallback that simply echoes the reason, but in
// normal operation the agent handles escalation before calling Execute so the
// escalation is tied to the session and customer.
type Escalate struct{}

func (t *Escalate) Def() llm.ToolDef {
	return llm.ToolDef{
		Name:        "escalate_to_human",
		Description: "Hand the conversation to a human agent. Use when a decision requires human authority, when the customer explicitly asks, or when you cannot safely resolve the request.",
		Properties: map[string]any{
			"reason": map[string]any{"type": "string", "description": "Why this needs a human"},
		},
		Required: []string{"reason"},
	}
}

func (t *Escalate) Destructive() bool { return false }

func (t *Escalate) Execute(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		Reason string `json:"reason"`
	}
	_ = decode(input, &in)
	return Result{Content: "Escalated to a human agent: " + in.Reason}, nil
}
