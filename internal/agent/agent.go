// Package agent implements the core loop: take a message from the user, decide
// which tools to call, execute the safe ones (validating any destructive ones
// through the policy layer), and produce a reply. The loop is bounded so it
// always terminates.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
	Steps     int      `json:"steps"`
	Actions   []Action `json:"actions,omitempty"`
}

// StreamEvent is emitted during HandleStream as the turn unfolds: either a piece
// of incremental assistant text, or the name of a tool about to run.
type StreamEvent struct {
	Delta string // incremental assistant text
	Tool  string // a tool that is starting to run
}

// Handle runs the agent loop for one user message within a session and returns
// the final reply. The session must already exist.
func (a *Agent) Handle(ctx context.Context, sessionID, userMessage string) (*Reply, error) {
	return a.run(ctx, sessionID, userMessage, nil)
}

// HandleStream is like Handle but streams the turn: emit is called with
// incremental assistant text and tool status as they happen. It still returns
// the final reply once the turn completes.
func (a *Agent) HandleStream(ctx context.Context, sessionID, userMessage string, emit func(StreamEvent)) (*Reply, error) {
	return a.run(ctx, sessionID, userMessage, emit)
}

func (a *Agent) run(ctx context.Context, sessionID, userMessage string, emit func(StreamEvent)) (*Reply, error) {
	if err := a.Store.TouchSession(sessionID); err != nil {
		return nil, err
	}

	history, err := a.Store.LoadMessages(sessionID)
	if err != nil {
		return nil, err
	}
	// The first message names the conversation for the sidebar.
	if len(history) == 0 {
		_ = a.Store.SetSessionTitle(sessionID, sessionTitle(userMessage))
	}

	userMsg := llm.UserText(userMessage)
	if err := a.Store.AppendMessage(sessionID, userMsg); err != nil {
		return nil, err
	}
	history = append(history, userMsg)

	reply := &Reply{SessionID: sessionID}

	// The user's open issues travel with every turn as memory across sessions.
	system := a.System
	if issues, err := a.Store.OpenIssues(8); err == nil && len(issues) > 0 {
		system += "\n\n" + issuesContext(issues)
	}

	for step := 0; step < a.MaxSteps; step++ {
		reply.Steps = step + 1

		req := llm.Request{System: system, Messages: history, Tools: a.Tools.Defs()}
		var resp *llm.Response
		if emit != nil {
			resp, err = a.Model.Stream(ctx, req, func(t string) { emit(StreamEvent{Delta: t}) })
		} else {
			resp, err = a.Model.Complete(ctx, req)
		}
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
			tool, ok := a.Tools.Get(call.Name)
			if !ok {
				res := "unknown tool: " + call.Name
				results = append(results, llm.ToolResult(call.ID, res, true))
				a.audit(sessionID, call, "n/a", "", res, true)
				reply.Actions = append(reply.Actions, Action{Tool: call.Name, Input: call.Input, Decision: "n/a", Result: res, IsError: true})
				continue
			}

			if emit != nil {
				emit(StreamEvent{Tool: call.Name})
			}

			decision := policy.Decision{Outcome: policy.Allow}
			if tool.Destructive() {
				decision, err = a.Policy.Validate(ctx, call.Name, call.Input)
				if err != nil {
					return nil, err
				}
			}
			if decision.Outcome == policy.Deny {
				res := "action not permitted: " + decision.Reason
				results = append(results, llm.ToolResult(call.ID, res, true))
				a.audit(sessionID, call, string(policy.Deny), decision.Reason, res, true)
				reply.Actions = append(reply.Actions, Action{Tool: call.Name, Input: call.Input, Decision: string(policy.Deny), Reason: decision.Reason, Result: res, IsError: true})
				continue
			}

			// Allowed: execute.
			ci := tools.CallInfo{SessionID: sessionID}
			out, err := tool.Execute(tools.WithCallInfo(ctx, ci), call.Input)
			if err != nil {
				return nil, fmt.Errorf("execute %s: %w", call.Name, err)
			}
			results = append(results, llm.ToolResult(call.ID, out.Content, out.IsError))
			a.audit(sessionID, call, string(policy.Allow), "", out.Content, out.IsError)
			reply.Actions = append(reply.Actions, Action{Tool: call.Name, Input: call.Input, Decision: string(policy.Allow), Result: out.Content, IsError: out.IsError})
		}

		// Feed the tool results back to the model as a user turn.
		toolMsg := llm.Message{Role: llm.RoleUser, Content: results}
		if err := a.Store.AppendMessage(sessionID, toolMsg); err != nil {
			return nil, err
		}
		history = append(history, toolMsg)
	}

	// Step budget exhausted: close the turn gently rather than looping.
	text := "I want to give this the attention it deserves, but I've gone in circles for a moment. Can we slow down — what feels most important to you right now?"
	assistant := llm.Message{Role: llm.RoleAssistant, Content: []llm.Block{llm.Text(text)}}
	if err := a.Store.AppendMessage(sessionID, assistant); err != nil {
		return nil, err
	}
	reply.Text = text
	return reply, nil
}

// issuesContext renders the user's open issues as background memory for a turn.
func issuesContext(issues []store.Issue) string {
	var b strings.Builder
	b.WriteString("What this person is currently working through (from earlier conversations), each with an id:\n")
	for _, iss := range issues {
		b.WriteString("- [" + iss.ID + "] " + iss.Title)
		if strings.TrimSpace(iss.Plan) != "" {
			b.WriteString(" — plan: " + iss.Plan)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// sessionTitle derives a short conversation title from the first message.
func sessionTitle(msg string) string {
	msg = strings.TrimSpace(strings.Join(strings.Fields(msg), " "))
	const max = 60
	if len(msg) > max {
		msg = strings.TrimSpace(msg[:max]) + "…"
	}
	if msg == "" {
		msg = "New conversation"
	}
	return msg
}

func (a *Agent) audit(sessionID string, call llm.Block, decision, reason, result string, isErr bool) {
	// Audit failures should not break the conversation; best-effort.
	_ = a.Store.RecordAudit(sessionID, call.Name, call.Input, decision, reason, result, isErr)
}

const systemPrompt = `You are Malten, a warm, steady companion built for neurodivergent people — those with ADHD, autism, and related ways of thinking. You help one person — the user — work with their own mind rather than against it.

Your purpose is to help them name what they're stuck on, understand it, think it through, and shape a next step small enough to actually start. You are a space to reflect and to externalise, not a service desk.

How to be:
- Listen first. Reflect back what you hear so they feel understood before offering anything. Ask one clear question at a time, not several.
- Be concise and direct. Avoid vague advice, long lists, and "just try harder". Say the concrete thing.
- Take neurodivergent experience as real and valid — executive dysfunction, task paralysis, time blindness, overwhelm, sensory overload, rejection sensitivity, masking fatigue, hyperfocus. Never frame these as laziness or a character flaw.
- Help them shrink tasks until the first step is almost too small to refuse, externalise what's swirling in their head, and work with their energy instead of an idealised routine.
- Validate feelings without judgement. Never minimise, rush, or lecture.

You will often see, in your context, a list of what this person is already working through (their open "issues", each with an id) from earlier conversations. Treat it as memory: keep it in mind for continuity and refer to it naturally when relevant, but don't recite it back unprompted.

Tools:
- search: look up simple, well-established strategies (breaking tasks down, grounding, managing overwhelm, sleep, reaching out) to ground your suggestions rather than inventing methods.
- create_issue: when the user names something new they want to keep working on, log it as an "issue" with a short title and, if you've shaped one together, a plan — so it lives outside their head and they can return to it. Only log real things they've agreed to, not every passing thought.
- update_issue: as they make progress, refine an existing issue's plan or mark it resolved (status "closed"). Reference it by the id shown in your context.

Boundaries and safety (important):
- You are not a therapist, doctor, diagnostician or crisis service, and you must not present yourself as one or diagnose anyone. For ongoing or serious struggles, gently encourage them to reach out to a GP, therapist, or someone they trust.
- If they express thoughts of suicide or self-harm, or seem to be in danger, respond with calm care, take it seriously, and encourage them to contact emergency services or a crisis line right now (for example, in the UK, Samaritans on 116 123; otherwise their local emergency number). Never give instructions that could cause harm, and don't try to be someone's only support in a crisis.`
