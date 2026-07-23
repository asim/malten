// Package tools defines the capabilities the support agent can invoke and a
// registry that makes the set extensible. Each tool declares its schema, says
// whether it is "destructive" (i.e. must pass policy validation before it
// runs), and knows how to execute itself against the backing store.
//
// The knowledge-base search, account lookup, refund, password reset, ticket
// creation and human-escalation capabilities from the spec are each a Tool.
// Adding a new capability is a matter of implementing Tool and registering it.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/asim/malten/internal/llm"
)

// Result is the outcome of executing a tool.
type Result struct {
	// Content is the text handed back to the model as the tool_result.
	Content string
	// IsError marks the result as a failure so the model can adapt.
	IsError bool
}

// Tool is a single capability the agent can call.
type Tool interface {
	// Def returns the model-facing definition (name, description, schema).
	Def() llm.ToolDef
	// Destructive reports whether calls must be validated by the policy layer
	// before execution. Read-only tools return false.
	Destructive() bool
	// Execute runs the tool with validated input.
	Execute(ctx context.Context, input json.RawMessage) (Result, error)
}

// Registry holds the available tools. It is safe for concurrent use.
type Registry struct {
	mu    sync.RWMutex
	order []string
	tools map[string]Tool
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{tools: map[string]Tool{}}
}

// Register adds a tool. A later registration with the same name replaces the
// earlier one, keeping the original ordering.
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := t.Def().Name
	if _, exists := r.tools[name]; !exists {
		r.order = append(r.order, name)
	}
	r.tools[name] = t
}

// Get returns the tool registered under name.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// Defs returns the definitions of all registered tools, in registration order.
func (r *Registry) Defs() []llm.ToolDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]llm.ToolDef, 0, len(r.order))
	for _, name := range r.order {
		defs = append(defs, r.tools[name].Def())
	}
	return defs
}

// decode is a small helper for tools to unmarshal their input with a clear
// error if the model produced malformed JSON.
func decode(input json.RawMessage, v any) error {
	if len(input) == 0 {
		return fmt.Errorf("missing tool input")
	}
	if err := json.Unmarshal(input, v); err != nil {
		return fmt.Errorf("invalid tool input: %w", err)
	}
	return nil
}
