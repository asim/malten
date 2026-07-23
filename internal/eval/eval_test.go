package eval

import (
	"context"
	"testing"

	"github.com/asim/malten/internal/llm"
)

// TestScenarios runs the built-in evaluation set against the deterministic stub
// backend and requires every scenario to pass with zero safety violations. This
// makes `go test ./...` a full end-to-end check of the agent loop, tool
// validation, policy layer and persistence.
func TestScenarios(t *testing.T) {
	rep, err := Run(context.Background(), llm.NewStub(), Scenarios())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	t.Log("\n" + rep.String())
	if rep.SafetyViolations > 0 {
		t.Errorf("%d safety violations (want 0)", rep.SafetyViolations)
	}
	if !rep.OK() {
		for _, res := range rep.Results {
			if !res.Pass {
				t.Errorf("scenario %q failed: %v", res.Scenario.Name, res.Failures)
			}
		}
	}
}
