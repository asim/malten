package agent_test

import (
	"context"
	"testing"

	"github.com/asim/malten/internal/agent"
	"github.com/asim/malten/internal/app"
	"github.com/asim/malten/internal/llm"
	"github.com/asim/malten/internal/store"
)

// TestCreateThenCloseAcrossTurns exercises the stateless flow: logging
// something returns an issue change for the client to save, and sending that
// issue back as memory lets a later turn close it — with no server state.
func TestCreateThenCloseAcrossTurns(t *testing.T) {
	ag, st, err := app.Build(llm.NewStub(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	r1, err := ag.Handle(ctx, agent.Turn{Message: "I keep avoiding my emails, can we make a plan?"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r1.IssueChanges) != 1 || r1.IssueChanges[0].Status != "open" || r1.IssueChanges[0].ID == "" {
		t.Fatalf("expected one new open issue change, got %+v", r1.IssueChanges)
	}
	iss := r1.IssueChanges[0]

	r2, err := ag.Handle(ctx, agent.Turn{
		Issues:  []store.Issue{iss},
		Message: "I actually did it — you can mark that done.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(r2.IssueChanges) != 1 || r2.IssueChanges[0].ID != iss.ID || r2.IssueChanges[0].Status != "closed" {
		t.Fatalf("expected the issue to be closed, got %+v", r2.IssueChanges)
	}
}

// TestStreamReconstructsText checks that the streamed deltas concatenate to the
// same final text a non-streaming turn produces.
func TestStreamReconstructsText(t *testing.T) {
	ag, st, err := app.Build(llm.NewStub(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	turn := agent.Turn{Message: "I feel anxious, how can I calm down?"}

	full, err := ag.Handle(ctx, turn)
	if err != nil {
		t.Fatal(err)
	}
	var acc string
	streamed, err := ag.HandleStream(ctx, turn, func(ev agent.StreamEvent) { acc += ev.Delta })
	if err != nil {
		t.Fatal(err)
	}
	if acc != full.Text || streamed.Text != full.Text {
		t.Fatalf("stream mismatch:\n deltas=%q\n stream=%q\n full=%q", acc, streamed.Text, full.Text)
	}
}
