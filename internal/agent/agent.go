// Package agent implements the core loop: take a customer message and a
// customer id, decide which tools to call, execute the safe ones (validating
// destructive ones through the policy layer), and produce either a final reply
// or an escalation. The loop is bounded so it always terminates.
package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/asim/malten/internal/id"
	"github.com/asim/malten/internal/llm"
	"github.com/asim/malten/internal/policy"
	"github.com/asim/malten/internal/store"
	"github.com/asim/malten/internal/tools"
)

// DefaultMaxSteps bounds the number of model turns per user message so the loop
// always terminates even if the model keeps requesting tools.
const DefaultMaxSteps = 8

// Agent orchestrates the model, tools, policy and store.
type Agent struct {
	Model    llm.LLM
	Tools    *tools.Registry
	Policy   *policy.Validator
	Store    *store.Store
	MaxSteps int
	System   string
}

// New builds an Agent with default settings.
func New(model llm.LLM, reg *tools.Registry, pol *policy.Validator, st *store.Store) *Agent {
	return &Agent{
		Model:    model,
		Tools:    reg,
		Policy:   pol,
		Store:    st,
		MaxSteps: DefaultMaxSteps,
		System:   systemPrompt,
	}
}

// Action records a single tool execution for the response and observability.
type Action struct {
	Tool     string          `json:"tool"`
	Input    json.RawMessage `json:"input"`
	Decision string          `json:"decision"`
	Reason   string          `json:"reason,omitempty"`
	Result   string          `json:"result,omitempty"`
	IsError  bool            `json:"is_error,omitempty"`
}

// Reply is the outcome of handling one user message.
type Reply struct {
	SessionID string   `json:"session_id"`
	Text      string   `json:"text"`
	Escalated bool     `json:"escalated"`
	Steps     int      `json:"steps"`
	Actions   []Action `json:"actions,omitempty"`
}

// Handle runs the agent loop for one user message within a session. The session
// must already exist. customerID may be empty (unverified).
func (a *Agent) Handle(ctx context.Context, sessionID, customerID, userMessage string) (*Reply, error) {
	if err := a.Store.TouchSession(sessionID, customerID); err != nil {
		return nil, err
	}

	history, err := a.Store.LoadMessages(sessionID)
	if err != nil {
		return nil, err
	}

	userMsg := llm.UserText(userMessage)
	if err := a.Store.AppendMessage(sessionID, userMsg); err != nil {
		return nil, err
	}
	history = append(history, userMsg)

	reply := &Reply{SessionID: sessionID}
	system := a.System + "\n\n" + customerContext(customerID)

	for step := 0; step < a.MaxSteps; step++ {
		reply.Steps = step + 1

		resp, err := a.Model.Complete(ctx, llm.Request{
			System:   system,
			Messages: history,
			Tools:    a.Tools.Defs(),
		})
		if err != nil {
			return nil, fmt.Errorf("model completion: %w", err)
		}

		toolUses := resp.ToolUses()
		if len(toolUses) == 0 {
			// Final answer.
			text := resp.Text()
			assistant := llm.Message{Role: llm.RoleAssistant, Content: []llm.Block{llm.Text(text)}}
			if err := a.Store.AppendMessage(sessionID, assistant); err != nil {
				return nil, err
			}
			reply.Text = text
			return reply, nil
		}

		// Persist the assistant turn (which carries the tool_use blocks) before
		// executing tools, so the transcript is complete and replayable.
		assistant := llm.Message{Role: llm.RoleAssistant, Content: resp.Content}
		if err := a.Store.AppendMessage(sessionID, assistant); err != nil {
			return nil, err
		}
		history = append(history, assistant)

		var results []llm.Block
		for _, call := range toolUses {
			// Explicit escalation requested by the model. Terminal.
			if call.Name == "escalate_to_human" {
				return a.escalate(ctx, reply, sessionID, customerID, reasonFrom(call.Input), history)
			}

			tool, ok := a.Tools.Get(call.Name)
			if !ok {
				res := "unknown tool: " + call.Name
				results = append(results, llm.ToolResult(call.ID, res, true))
				a.audit(sessionID, customerID, call, "n/a", "", res, true)
				reply.Actions = append(reply.Actions, Action{Tool: call.Name, Input: call.Input, Decision: "n/a", Result: res, IsError: true})
				continue
			}

			decision := policy.Decision{Outcome: policy.Allow}
			if tool.Destructive() {
				decision, err = a.Policy.Validate(ctx, customerID, call.Name, call.Input)
				if err != nil {
					return nil, err
				}
			}

			switch decision.Outcome {
			case policy.Deny:
				res := "action not permitted: " + decision.Reason
				results = append(results, llm.ToolResult(call.ID, res, true))
				a.audit(sessionID, customerID, call, string(policy.Deny), decision.Reason, res, true)
				reply.Actions = append(reply.Actions, Action{Tool: call.Name, Input: call.Input, Decision: string(policy.Deny), Reason: decision.Reason, Result: res, IsError: true})
				continue

			case policy.Escalate:
				reply.Actions = append(reply.Actions, Action{Tool: call.Name, Input: call.Input, Decision: string(policy.Escalate), Reason: decision.Reason})
				a.audit(sessionID, customerID, call, string(policy.Escalate), decision.Reason, "", false)
				return a.escalate(ctx, reply, sessionID, customerID, decision.Reason, history)
			}

			// Allowed: execute.
			ci := tools.CallInfo{SessionID: sessionID, CustomerID: customerID}
			out, err := tool.Execute(tools.WithCallInfo(ctx, ci), call.Input)
			if err != nil {
				return nil, fmt.Errorf("execute %s: %w", call.Name, err)
			}
			results = append(results, llm.ToolResult(call.ID, out.Content, out.IsError))
			a.audit(sessionID, customerID, call, string(policy.Allow), "", out.Content, out.IsError)
			reply.Actions = append(reply.Actions, Action{Tool: call.Name, Input: call.Input, Decision: string(policy.Allow), Result: out.Content, IsError: out.IsError})
		}

		// Feed the tool results back to the model as a user turn.
		toolMsg := llm.Message{Role: llm.RoleUser, Content: results}
		if err := a.Store.AppendMessage(sessionID, toolMsg); err != nil {
			return nil, err
		}
		history = append(history, toolMsg)
	}

	// Step budget exhausted without a resolution: escalate rather than loop.
	return a.escalate(ctx, reply, sessionID, customerID, "the agent could not resolve the request within its step budget", history)
}

// escalate records an escalation in the backlog and returns a terminal reply.
func (a *Agent) escalate(ctx context.Context, reply *Reply, sessionID, customerID, reason string, history []llm.Message) (*Reply, error) {
	summary := "Escalation: " + reason
	if _, err := a.Store.CreateTicket(id.New("ESC"), sessionID, customerID, "escalation", summary, "high"); err != nil {
		return nil, err
	}
	text := "I've escalated this to a human support agent who can help further. " +
		"Reason: " + reason + ". You'll hear back shortly."
	assistant := llm.Message{Role: llm.RoleAssistant, Content: []llm.Block{llm.Text(text)}}
	if err := a.Store.AppendMessage(sessionID, assistant); err != nil {
		return nil, err
	}
	reply.Text = text
	reply.Escalated = true
	return reply, nil
}

func (a *Agent) audit(sessionID, customerID string, call llm.Block, decision, reason, result string, isErr bool) {
	// Audit failures should not break the conversation; best-effort.
	_ = a.Store.RecordAudit(sessionID, customerID, call.Name, call.Input, decision, reason, result, isErr)
}

func reasonFrom(input json.RawMessage) string {
	var in struct {
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(input, &in)
	if in.Reason == "" {
		return "human assistance requested"
	}
	return in.Reason
}

func customerContext(customerID string) string {
	if customerID == "" {
		return "The customer has not provided a customer_id yet. If you need account details or want to take an account action, ask them for their customer id first."
	}
	return "The current customer_id is " + customerID + " (provided by the client for this session). Use this id for account_lookup and account actions."
}

const systemPrompt = `You are Malten, a conversational customer-support agent for a SaaS product.

Your job: resolve the customer's issue, take an action on their behalf, or escalate to a human.

Tools available to you:
- search: search the product knowledge base for how-to and policy answers.
- account_lookup: fetch a customer's plan, usage and orders. Do this before any account-specific action.
- issue_refund, reset_password, create_ticket: take actions. These are validated against policy before they run; if a call is denied you will see the reason and should adapt or explain.
- escalate_to_human: hand off when a decision needs human authority or the customer asks for a person.

Guidelines:
- Prefer resolving the issue directly. Look up the account before acting on it.
- Never claim to have taken an action you did not take. Only report results you can see from tool results.
- If a refund or other action is denied or requires approval, explain clearly and, when appropriate, escalate.
- Be concise, warm and direct. When you have enough information to act, act.`
