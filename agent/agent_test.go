package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoopContextRestartAndRetry(t *testing.T) {
	now := time.Now()
	path := filepath.Join(t.TempDir(), "agents.json")
	memory, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	calls, publishes := 0, 0
	fail := true
	var key string
	l := Loop{Memory: memory, Agent: Agent{Name: "news", Objective: "Brief generation", Read: func(context.Context, time.Time) (json.RawMessage, error) {
		return json.RawMessage(`{"headline":"One","url":"https://example.com"}`), nil
	}}, Observe: func() []Observation { return nil }}
	l.Agent.Decide = func(_ context.Context, v View) (Decision, error) {
		calls++
		if len(v.Records) != 1 || v.Records[0].Kind != "source" || v.Objective != "Brief generation" {
			t.Fatalf("missing persisted context: %+v", v)
		}
		return Decision{Summary: "One new story", Action: &Action{Stream: "news", Text: "A headline-based brief"}, Evidence: []string{v.Records[0].ID}}, nil
	}
	l.Publish = func(_ context.Context, stream, text, name, photo string, keys ...string) error {
		publishes++
		if key != "" && key != keys[0] {
			t.Fatal("retry changed idempotency key")
		}
		key = keys[0]
		if fail {
			return errors.New("busy")
		}
		return nil
	}
	if l.Step(context.Background(), now) == nil {
		t.Fatal("expected publish failure")
	}
	restored, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	l.Memory = restored
	fail = false
	if err = l.Step(context.Background(), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || publishes != 2 {
		t.Fatalf("decisions %d, attempts %d", calls, publishes)
	}
	if err = l.Step(context.Background(), now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || publishes != 2 {
		t.Fatal("unchanged inputs repeated")
	}
	if err = l.Memory.Expire(now.Add(25 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "{\"news\":null}\n" && string(raw) != "{\"news\":[]}\n" {
		t.Fatalf("expired context persisted: %s", raw)
	}
}
func TestNoActionObservationsAndCancellation(t *testing.T) {
	now := time.Now()
	memory, _ := Open(filepath.Join(t.TempDir(), "agents.json"))
	var observations []Observation
	calls := 0
	l := Loop{Memory: memory, Agent: Agent{Name: "aslam", Read: func(context.Context, time.Time) (json.RawMessage, error) {
		return json.RawMessage(`{"source":"prayer"}`), nil
	}, Decide: func(_ context.Context, v View) (Decision, error) {
		calls++
		return Decision{Summary: "Context without publication"}, nil
	}}, Observe: func() []Observation { return observations }, Publish: func(context.Context, string, string, string, string, ...string) error {
		t.Fatal("unexpected publication")
		return nil
	}}
	if err := l.Step(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	observations = []Observation{{ID: "human", Stream: "home", Kind: "human", Text: "Grateful for the rain", At: now}}
	if err := l.Step(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("did not observe a new public input")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	l.Run(ctx)
	if calls != 2 {
		t.Fatal("read after cancellation")
	}
}
func TestDestinationsAndEvidence(t *testing.T) {
	now := time.Now()
	v := View{Now: now, Records: []Record{{ID: "source", Kind: "source"}}, Observations: []Observation{{ID: "human", Stream: "london", Kind: "human", At: now}}}
	d := Decision{Action: &Action{Stream: "london", Text: "A useful contribution"}, Evidence: []string{"source", "human"}}
	if validAction("news", v, d) {
		t.Fatal("news escaped own stream")
	}
	if !validAction("aslam", v, d) {
		t.Fatal("cannot act on public context")
	}
	d.Action.Stream = "unobserved"
	if validAction("aslam", v, d) {
		t.Fatal("seeded an unobserved stream")
	}
	d.Action.Stream = "aslam"
	d.Evidence = []string{"invented"}
	if validAction("aslam", v, d) {
		t.Fatal("invented evidence accepted")
	}
}
func TestCorruptStorageFails(t *testing.T) {
	p := filepath.Join(t.TempDir(), "agents.json")
	os.WriteFile(p, []byte(`{"news":`), 0600)
	if _, err := Open(p); err == nil {
		t.Fatal("discarded corrupted memory")
	}
}
func TestSourceFailureRetainsContext(t *testing.T) {
	now := time.Now()
	m, _ := Open(filepath.Join(t.TempDir(), "agents.json"))
	m.Write("news", Record{ID: "saved", At: now, Kind: "source", Data: json.RawMessage(`{"headline":"known"}`)})
	called := false
	l := Loop{Memory: m, Agent: Agent{Name: "news", Read: func(context.Context, time.Time) (json.RawMessage, error) { return nil, errors.New("offline") }, Decide: func(_ context.Context, v View) (Decision, error) {
		called = true
		return Decision{Summary: "Source unavailable; retained evidence"}, nil
	}}, Observe: func() []Observation { return nil }}
	if err := l.Step(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("lost retained source")
	}
}

func TestBlockedAttemptDoesNotConsumePublicationWindow(t *testing.T) {
	now := time.Now()
	m, _ := Open(filepath.Join(t.TempDir(), "agents.json"))
	m.Write("news", Record{ID: "blocked", Kind: "decision", At: now.Add(-10 * time.Minute), Action: &Action{Stream: "news", Text: "old attempt"}, Status: "blocked"})
	published := 0
	l := Loop{Memory: m, Agent: Agent{Name: "news", Read: func(context.Context, time.Time) (json.RawMessage, error) {
		return json.RawMessage(`{"headline":"new"}`), nil
	}, Decide: func(_ context.Context, v View) (Decision, error) {
		var id string
		for _, r := range v.Records {
			if r.Kind == "source" {
				id = r.ID
			}
		}
		return Decision{Action: &Action{Stream: "news", Text: "New brief"}, Evidence: []string{id}}, nil
	}}, Observe: func() []Observation { return nil }, Publish: func(context.Context, string, string, string, string, ...string) error { published++; return nil }}
	if err := l.Step(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if published != 1 {
		t.Fatal("rejected attempt suppressed a valid new contribution")
	}
	m.RecordCycle("news", errors.New("secret material"))
	status := m.Status()["news"]
	if status.LastError != "cycle failed" || status.LastAction != "sent" {
		t.Fatalf("unsafe or incorrect status: %+v", status)
	}
}
