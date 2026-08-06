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

// The policy layer is a fail-closed boundary: with no destructive tools
// defined, any tool routed through Validate (i.e. one marked destructive) must
// be denied rather than run blindly.
func TestUnknownDestructiveDenied(t *testing.T) {
	v := newValidator(t)
	d, err := v.Validate(context.Background(), "some_future_action", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if d.Outcome != Deny {
		t.Errorf("unknown destructive action: got %s, want deny", d.Outcome)
	}
}
