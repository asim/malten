package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/asim/malten/internal/id"
	"github.com/asim/malten/internal/llm"
	"github.com/asim/malten/internal/store"
)

// CreateIssue implements create_issue(title, plan). An "issue" is something the
// user wants to keep working on; logging it lets them come back to it. It is
// not destructive — it only records what the user has chosen to track.
type CreateIssue struct {
	Store *store.Store
}

func (t *CreateIssue) Def() llm.ToolDef {
	return llm.ToolDef{
		Name:        "create_issue",
		Description: "Log something the user wants to keep working on as an issue, with a short title and an optional plan or first step you've shaped together. Use this only for real things the user has agreed to revisit — not every passing worry.",
		Properties: map[string]any{
			"title": map[string]any{"type": "string", "description": "A short, plain title for what they're working on"},
			"plan":  map[string]any{"type": "string", "description": "Optional: the plan or first step you've agreed on"},
		},
		Required: []string{"title"},
	}
}

func (t *CreateIssue) Destructive() bool { return false }

func (t *CreateIssue) Execute(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		Title string `json:"title"`
		Plan  string `json:"plan"`
	}
	if err := decode(input, &in); err != nil {
		return Result{Content: err.Error(), IsError: true}, nil
	}
	if strings.TrimSpace(in.Title) == "" {
		return Result{Content: "title is required", IsError: true}, nil
	}
	ci := CallInfoFrom(ctx)
	iss, err := t.Store.CreateIssue(id.New("ISS"), ci.SessionID, in.Title, in.Plan)
	if err != nil {
		return Result{}, err
	}
	return Result{Content: fmt.Sprintf("Logged issue %s: %q. The user can find it under Issues.", iss.ID, iss.Title)}, nil
}

// UpdateIssue implements update_issue(id, status, plan): refine an existing
// issue's plan or mark it resolved. Not destructive — it edits the user's own
// notes about what they're working on.
type UpdateIssue struct {
	Store *store.Store
}

func (t *UpdateIssue) Def() llm.ToolDef {
	return llm.ToolDef{
		Name:        "update_issue",
		Description: "Update an issue the user has been working on: refine its plan, or mark it resolved when they've made progress. Reference it by the id shown in your context. Use this to keep their issues current, not for every message.",
		Properties: map[string]any{
			"id":     map[string]any{"type": "string", "description": "The issue id, e.g. ISS-abc123"},
			"status": map[string]any{"type": "string", "enum": []string{"open", "closed"}, "description": "Set to 'closed' when the issue is resolved"},
			"plan":   map[string]any{"type": "string", "description": "A refined or updated plan / next step"},
		},
		Required: []string{"id"},
	}
}

func (t *UpdateIssue) Destructive() bool { return false }

func (t *UpdateIssue) Execute(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Plan   string `json:"plan"`
	}
	if err := decode(input, &in); err != nil {
		return Result{Content: err.Error(), IsError: true}, nil
	}
	if strings.TrimSpace(in.ID) == "" {
		return Result{Content: "id is required", IsError: true}, nil
	}
	if in.Status != "" && in.Status != "open" && in.Status != "closed" {
		return Result{Content: "status must be 'open' or 'closed'", IsError: true}, nil
	}
	iss, err := t.Store.UpdateIssue(in.ID, in.Plan, in.Status)
	if err != nil {
		return Result{}, err
	}
	if iss == nil {
		return Result{Content: "No issue found with id " + in.ID, IsError: true}, nil
	}
	return Result{Content: fmt.Sprintf("Updated issue %s (%s): %q.", iss.ID, iss.Status, iss.Title)}, nil
}
