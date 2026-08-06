// Package policy is the trust boundary between the model and destructive
// actions. Before any tool the registry marks as destructive runs, the agent
// asks the Validator for a decision; read-only tools bypass it.
//
// Malten currently has no destructive tools — logging an issue and searching
// the library are both safe — so the Validator's job right now is simply to
// fail closed: an unrecognized "destructive" action is denied rather than run
// blindly. The seam is kept deliberately so adding a real destructive
// capability later means adding a case here, not special-casing the agent.
package policy

import (
	"context"
	"encoding/json"

	"github.com/asim/malten/internal/store"
)

// Outcome is the class of a policy decision.
type Outcome string

const (
	// Allow: the action is within policy and may execute.
	Allow Outcome = "allow"
	// Deny: the action is not permitted and must not run (the model is told why).
	Deny Outcome = "deny"
)

// Decision is the result of validating a tool call.
type Decision struct {
	Outcome Outcome
	Reason  string
}

func deny(f string) Decision { return Decision{Outcome: Deny, Reason: f} }

// Validator applies business rules to destructive tool calls.
type Validator struct {
	Store *store.Store
}

// New returns a Validator.
func New(s *store.Store) *Validator { return &Validator{Store: s} }

// Validate checks a destructive tool call. Read-only tools should not be passed
// here. With no destructive tools defined, the safe default is to deny.
func (v *Validator) Validate(ctx context.Context, tool string, input json.RawMessage) (Decision, error) {
	// Unknown destructive tools are denied by default (fail closed). Add a case
	// here when introducing a real destructive capability.
	return deny("unrecognized action: " + tool), nil
}
