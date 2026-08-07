// Package app wires the pieces (knowledge base, tool registry, policy, agent)
// together. Both the HTTP server (cmd/malten) and the evaluation harness build
// their agent through Build so they exercise an identical configuration.
package app

import (
	"github.com/asim/malten/internal/agent"
	"github.com/asim/malten/internal/llm"
	"github.com/asim/malten/internal/policy"
	"github.com/asim/malten/internal/store"
	"github.com/asim/malten/internal/tools"
)

// Build opens the in-memory knowledge base, registers the tool set and returns a
// ready (stateless) agent. Adding a capability is a single Register call here.
func Build(model llm.LLM, dbPath string) (*agent.Agent, *store.Store, error) {
	st, err := store.Open(dbPath)
	if err != nil {
		return nil, nil, err
	}

	reg := tools.NewRegistry()
	reg.Register(&tools.Search{Store: st})
	reg.Register(&tools.CreateIssue{})
	reg.Register(&tools.UpdateIssue{})

	pol := policy.New(st)
	ag := agent.New(model, reg, pol)
	return ag, st, nil
}
