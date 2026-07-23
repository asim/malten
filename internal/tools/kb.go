package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/asim/malten/internal/llm"
	"github.com/asim/malten/internal/store"
)

// KBSearch implements kb_search(query, k): top-k chunks from the knowledge
// base. It is read-only and therefore not destructive.
type KBSearch struct {
	Store *store.Store
}

func (t *KBSearch) Def() llm.ToolDef {
	return llm.ToolDef{
		Name:        "kb_search",
		Description: "Search the product knowledge base and return the top matching articles. Call this to answer how-to and policy questions before replying from memory.",
		Properties: map[string]any{
			"query": map[string]any{"type": "string", "description": "Natural language search query"},
			"k":     map[string]any{"type": "integer", "description": "Number of results to return (default 3)"},
		},
		Required: []string{"query"},
	}
}

func (t *KBSearch) Destructive() bool { return false }

func (t *KBSearch) Execute(ctx context.Context, input json.RawMessage) (Result, error) {
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
