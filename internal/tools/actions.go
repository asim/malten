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
