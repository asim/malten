// Package policy is the trust boundary between the model and destructive
// actions. Tool calls emitted by the LLM are never executed blindly: before any
// destructive tool runs, the agent asks the Validator for a decision. The
// Validator encodes business rules (an order must belong to the customer, a
// refund cannot exceed what was paid, refunds above a limit need a human) and
// returns Allow, Deny or Escalate.
//
// This is where "human-in-the-loop" lives: some actions are simply not the
// agent's to authorize, and the Validator says so.
package policy

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/asim/malten/internal/store"
)

// Outcome is the class of a policy decision.
type Outcome string

const (
	// Allow: the action is within policy and may execute.
	Allow Outcome = "allow"
	// Deny: the action is invalid and must not run (the model is told why).
	Deny Outcome = "deny"
	// Escalate: the action may be legitimate but requires human authority.
	Escalate Outcome = "escalate"
)

// Decision is the result of validating a tool call.
type Decision struct {
	Outcome Outcome
	Reason  string
}

func allow() Decision        { return Decision{Outcome: Allow} }
func deny(f string) Decision { return Decision{Outcome: Deny, Reason: f} }
func escalate(f string) Decision {
	return Decision{Outcome: Escalate, Reason: f}
}

// Validator applies business rules to destructive tool calls.
type Validator struct {
	Store *store.Store
	// RefundAutoApproveLimit is the largest refund the agent may issue without
	// human approval. Refunds above this are escalated.
	RefundAutoApproveLimit float64
}

// New returns a Validator with sensible defaults.
func New(s *store.Store) *Validator {
	return &Validator{Store: s, RefundAutoApproveLimit: 200}
}

// Validate checks a destructive tool call for the given customer. Read-only
// tools should not be passed here; they always Allow.
func (v *Validator) Validate(ctx context.Context, customerID, tool string, input json.RawMessage) (Decision, error) {
	switch tool {
	case "issue_refund":
		return v.validateRefund(customerID, input)
	case "reset_password":
		return v.validateReset(customerID, input)
	case "create_ticket":
		return v.validateTicket(input)
	default:
		// Unknown destructive tools are denied by default (fail closed).
		return deny("unrecognized action: " + tool), nil
	}
}

func (v *Validator) validateRefund(customerID string, input json.RawMessage) (Decision, error) {
	var in struct {
		OrderID string  `json:"order_id"`
		Amount  float64 `json:"amount"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return deny("refund input was not valid JSON"), nil
	}
	if customerID == "" {
		return deny("cannot issue a refund without a verified customer id"), nil
	}
	if in.OrderID == "" {
		return deny("an order id is required to issue a refund"), nil
	}
	order, err := v.Store.GetOrder(in.OrderID)
	if err != nil {
		return Decision{}, err
	}
	if order == nil {
		return deny(fmt.Sprintf("order %s does not exist", in.OrderID)), nil
	}
	if order.CustomerID != customerID {
		return deny(fmt.Sprintf("order %s does not belong to customer %s", in.OrderID, customerID)), nil
	}
	if order.Refunded {
		return deny(fmt.Sprintf("order %s has already been refunded", in.OrderID)), nil
	}
	if in.Amount <= 0 {
		return deny("refund amount must be positive"), nil
	}
	if in.Amount > order.Amount {
		return deny(fmt.Sprintf("refund amount $%.2f exceeds the order total $%.2f", in.Amount, order.Amount)), nil
	}
	if in.Amount > v.RefundAutoApproveLimit {
		return escalate(fmt.Sprintf("refund of $%.2f exceeds the $%.0f auto-approval limit and needs manager approval", in.Amount, v.RefundAutoApproveLimit)), nil
	}
	return allow(), nil
}

func (v *Validator) validateReset(customerID string, input json.RawMessage) (Decision, error) {
	var in struct {
		CustomerID string `json:"customer_id"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return deny("reset_password input was not valid JSON"), nil
	}
	target := in.CustomerID
	if target == "" {
		target = customerID
	}
	if target == "" {
		return deny("a customer id is required to reset a password"), nil
	}
	// The agent must operate on the verified session customer, not an arbitrary
	// one supplied by the model.
	if customerID != "" && in.CustomerID != "" && in.CustomerID != customerID {
		return deny("cannot reset a password for a different customer than the one in this session"), nil
	}
	acct, err := v.Store.GetAccount(target)
	if err != nil {
		return Decision{}, err
	}
	if acct == nil {
		return deny("no account found for " + target), nil
	}
	return allow(), nil
}

func (v *Validator) validateTicket(input json.RawMessage) (Decision, error) {
	var in struct {
		Summary  string `json:"summary"`
		Priority string `json:"priority"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return deny("create_ticket input was not valid JSON"), nil
	}
	switch in.Priority {
	case "low", "normal", "high", "urgent":
		return allow(), nil
	case "":
		return deny("ticket priority is required (low, normal, high or urgent)"), nil
	default:
		return deny("invalid ticket priority: " + in.Priority), nil
	}
}
