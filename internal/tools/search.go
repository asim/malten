package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/asim/malten/internal/llm"
	"github.com/asim/malten/internal/store"
)

// Search implements search(query, k): top-k chunks from the knowledge base.
// It is read-only and therefore not destructive.
type Search struct {
	Store *store.Store
}

func (t *Search) Def() llm.ToolDef {
	return llm.ToolDef{
		Name:        "search",
		Description: "Search a small library of simple, well-established self-help techniques (grounding, breathing, planning, sleep, reaching out) and return the best matches. Prefer grounding suggestions in these over inventing methods.",
		Properties: map[string]any{
			"query": map[string]any{"type": "string", "description": "Natural language search query"},
			"k":     map[string]any{"type": "integer", "description": "Number of results to return (default 3)"},
		},
		Required: []string{"query"},
	}
}

func (t *Search) Destructive() bool { return false }

func (t *Search) Execute(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		Query string `json:"query"`
		K     int    `json:"k"`
	}
	if err := decode(input, &in); err != nil {
		return Result{Content: err.Error(), IsError: true}, nil
	}
	if in.K == 0 {
		in.K = 3
	}
	docs, err := t.Store.SearchKB(in.Query, in.K)
	if err != nil {
		return Result{}, err
	}
	if len(docs) == 0 {
		return Result{Content: "No knowledge base articles matched."}, nil
	}
	var b strings.Builder
	for i, d := range docs {
		fmt.Fprintf(&b, "%d. %s\n%s\n", i+1, d.Title, d.Content)
	}
	return Result{Content: strings.TrimSpace(b.String())}, nil
}
