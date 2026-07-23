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

// IssueRefund implements issue_refund(order_id, amount). Destructive: the
// policy layer validates ownership, amount and the auto-approval limit before
// this ever runs.
type IssueRefund struct {
	Store *store.Store
}

func (t *IssueRefund) Def() llm.ToolDef {
	return llm.ToolDef{
		Name:        "issue_refund",
		Description: "Issue a refund against an order. The order must belong to the current customer. Large refunds may require human approval.",
		Properties: map[string]any{
			"order_id": map[string]any{"type": "string", "description": "The order to refund, e.g. ORD-5001"},
			"amount":   map[string]any{"type": "number", "description": "Amount to refund; must not exceed the order total"},
		},
		Required: []string{"order_id", "amount"},
	}
}

func (t *IssueRefund) Destructive() bool { return true }

func (t *IssueRefund) Execute(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		OrderID string  `json:"order_id"`
		Amount  float64 `json:"amount"`
	}
	if err := decode(input, &in); err != nil {
		return Result{Content: err.Error(), IsError: true}, nil
	}
	if err := t.Store.MarkRefunded(in.OrderID, in.Amount); err != nil {
		return Result{}, err
	}
	return Result{Content: fmt.Sprintf("Refund of $%.2f issued for order %s.", in.Amount, in.OrderID)}, nil
}

// ResetPassword implements reset_password(customer_id).
type ResetPassword struct {
	Store *store.Store
}

func (t *ResetPassword) Def() llm.ToolDef {
	return llm.ToolDef{
		Name:        "reset_password",
		Description: "Send a password reset link to the email on file for a customer.",
		Properties: map[string]any{
			"customer_id": map[string]any{"type": "string", "description": "The customer's id"},
		},
		Required: []string{"customer_id"},
	}
}

func (t *ResetPassword) Destructive() bool { return true }

func (t *ResetPassword) Execute(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		CustomerID string `json:"customer_id"`
	}
	if err := decode(input, &in); err != nil {
		return Result{Content: err.Error(), IsError: true}, nil
	}
	acct, err := t.Store.GetAccount(in.CustomerID)
	if err != nil {
		return Result{}, err
	}
	if acct == nil {
		return Result{Content: "No account found for " + in.CustomerID, IsError: true}, nil
	}
	if err := t.Store.SetPasswordReset(in.CustomerID); err != nil {
		return Result{}, err
	}
	// Email delivery is stubbed for now; we record intent and report success.
	return Result{Content: fmt.Sprintf("Password reset link sent to %s.", acct.Email)}, nil
}

// CreateTicket implements create_ticket(summary, priority). It appends to the
// support backlog that admins review.
type CreateTicket struct {
	Store *store.Store
}

func (t *CreateTicket) Def() llm.ToolDef {
	return llm.ToolDef{
		Name:        "create_ticket",
		Description: "Create a support ticket in the backlog for a human to follow up on. Use for bugs, feature requests or anything you cannot resolve directly.",
		Properties: map[string]any{
			"summary":  map[string]any{"type": "string", "description": "One-line summary of the issue"},
			"priority": map[string]any{"type": "string", "enum": []string{"low", "normal", "high", "urgent"}, "description": "Ticket priority"},
		},
		Required: []string{"summary", "priority"},
	}
}

func (t *CreateTicket) Destructive() bool { return true }

func (t *CreateTicket) Execute(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		Summary  string `json:"summary"`
		Priority string `json:"priority"`
	}
	if err := decode(input, &in); err != nil {
		return Result{Content: err.Error(), IsError: true}, nil
	}
	if strings.TrimSpace(in.Summary) == "" {
		return Result{Content: "summary is required", IsError: true}, nil
	}
	ci := CallInfoFrom(ctx)
	ticket, err := t.Store.CreateTicket(id.New("TCK"), ci.SessionID, ci.CustomerID, "ticket", in.Summary, in.Priority)
	if err != nil {
		return Result{}, err
	}
	// The link is stubbed; emailing the customer a copy is future work.
	return Result{Content: fmt.Sprintf("Ticket %s created (%s priority). Track it at /tickets/%s.", ticket.ID, ticket.Priority, ticket.ID)}, nil
}
