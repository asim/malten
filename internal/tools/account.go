package tools

import (
	"context"
	"encoding/json"

	"github.com/asim/malten/internal/llm"
	"github.com/asim/malten/internal/store"
)

// AccountLookup implements account_lookup(customer_id): subscription, recent
// orders and usage stats. Read-only.
type AccountLookup struct {
	Store *store.Store
}

func (t *AccountLookup) Def() llm.ToolDef {
	return llm.ToolDef{
		Name:        "account_lookup",
		Description: "Look up a customer's account: plan, status, usage stats and recent orders. Call this before taking any account-specific action.",
		Properties: map[string]any{
			"customer_id": map[string]any{"type": "string", "description": "The customer's id, e.g. CUST-1001"},
		},
		Required: []string{"customer_id"},
	}
}

func (t *AccountLookup) Destructive() bool { return false }

func (t *AccountLookup) Execute(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		CustomerID string `json:"customer_id"`
	}
	if err := decode(input, &in); err != nil {
		return Result{Content: err.Error(), IsError: true}, nil
	}
	if in.CustomerID == "" {
		return Result{Content: "customer_id is required", IsError: true}, nil
	}
	acct, err := t.Store.GetAccount(in.CustomerID)
	if err != nil {
		return Result{}, err
	}
	if acct == nil {
		return Result{Content: "No account found for customer_id " + in.CustomerID, IsError: true}, nil
	}
	out, err := json.Marshal(acct)
	if err != nil {
		return Result{}, err
	}
	return Result{Content: string(out)}, nil
}

var _ = store.Account{}
