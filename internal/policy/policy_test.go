package policy

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/asim/malten/internal/store"
)

func newValidator(t *testing.T) *Validator {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return New(st)
}

func refundInput(order string, amount float64) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"order_id": order, "amount": amount})
	return b
}

func TestRefundPolicy(t *testing.T) {
	v := newValidator(t)
	ctx := context.Background()

	cases := []struct {
		name     string
		customer string
		order    string
		amount   float64
		want     Outcome
	}{
		{"small refund allowed", "CUST-1001", "ORD-5001", 49, Allow},
		{"over-limit refund escalates", "CUST-1001", "ORD-5002", 499, Escalate},
		{"wrong customer denied", "CUST-1002", "ORD-5001", 49, Deny},
		{"unknown order denied", "CUST-1001", "ORD-9999", 10, Deny},
		{"amount over total denied", "CUST-1001", "ORD-5001", 1000, Deny},
		{"zero amount denied", "CUST-1001", "ORD-5001", 0, Deny},
		{"no customer denied", "", "ORD-5001", 49, Deny},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := v.Validate(ctx, tc.customer, "issue_refund", refundInput(tc.order, tc.amount))
			if err != nil {
				t.Fatalf("validate: %v", err)
			}
			if d.Outcome != tc.want {
				t.Errorf("got %s (%s), want %s", d.Outcome, d.Reason, tc.want)
			}
		})
	}
}

func TestAlreadyRefundedDenied(t *testing.T) {
	v := newValidator(t)
	ctx := context.Background()
	if err := v.Store.MarkRefunded("ORD-5001", 49); err != nil {
		t.Fatal(err)
	}
	d, err := v.Validate(ctx, "CUST-1001", "issue_refund", refundInput("ORD-5001", 49))
	if err != nil {
		t.Fatal(err)
	}
	if d.Outcome != Deny {
		t.Errorf("re-refund should be denied, got %s", d.Outcome)
	}
}

func TestResetAndTicketPolicy(t *testing.T) {
	v := newValidator(t)
	ctx := context.Background()

	reset := func(sessionCust, inputCust string) Outcome {
		b, _ := json.Marshal(map[string]any{"customer_id": inputCust})
		d, _ := v.Validate(ctx, sessionCust, "reset_password", b)
		return d.Outcome
	}
	if got := reset("CUST-1001", "CUST-1001"); got != Allow {
		t.Errorf("reset own account: got %s want allow", got)
	}
	if got := reset("CUST-1001", "CUST-1002"); got != Deny {
		t.Errorf("reset different account: got %s want deny", got)
	}
	if got := reset("CUST-1001", "CUST-9999"); got != Deny {
		// session id wins; input mismatch denied before lookup
		t.Errorf("reset mismatched account: got %s want deny", got)
	}

	ticket := func(priority string) Outcome {
		b, _ := json.Marshal(map[string]any{"summary": "x", "priority": priority})
		d, _ := v.Validate(ctx, "CUST-1001", "create_ticket", b)
		return d.Outcome
	}
	if got := ticket("high"); got != Allow {
		t.Errorf("valid priority: got %s want allow", got)
	}
	if got := ticket("banana"); got != Deny {
		t.Errorf("invalid priority: got %s want deny", got)
	}
}
