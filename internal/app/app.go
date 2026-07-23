// Package app wires the pieces (store, tool registry, policy, agent) together.
// Both the HTTP server (cmd/malten) and the evaluation harness build their
// agent through Build so they exercise an identical configuration.
package app

import (
	"github.com/asim/malten/internal/agent"
	"github.com/asim/malten/internal/llm"
	"github.com/asim/malten/internal/policy"
	"github.com/asim/malten/internal/store"
	"github.com/asim/malten/internal/tools"
)

// Build opens the store at dbPath (use ":memory:" for ephemeral use), registers
// the full tool set and returns a ready agent. Adding a capability is a single
// Register call here.
func Build(model llm.LLM, dbPath string) (*agent.Agent, *store.Store, error) {
	st, err := store.Open(dbPath)
	if err != nil {
		return nil, nil, err
	}

	reg := tools.NewRegistry()
	// Read-only tools first, then actions, then escalation.
	reg.Register(&tools.KBSearch{Store: st})
	reg.Register(&tools.AccountLookup{Store: st})
	reg.Register(&tools.IssueRefund{Store: st})
	reg.Register(&tools.ResetPassword{Store: st})
	reg.Register(&tools.CreateTicket{Store: st})
	reg.Register(&tools.Escalate{})

	pol := policy.New(st)
	ag := agent.New(model, reg, pol, st)
	return ag, st, nil
}
