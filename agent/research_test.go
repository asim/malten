package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInvestigatorChoosesContextThenRefinesSearch(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test")
	memory, err := Open(filepath.Join(t.TempDir(), "agents.json"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, record := range []Record{
		{ID: "source", Kind: "source", At: now.Add(-time.Hour), Data: json.RawMessage(`{"text":"retained source"}`)},
		{ID: "event", Kind: "moderation", At: now, Data: json.RawMessage(`{"private":"DO NOT EXPOSE"}`)},
		{ID: "decision", Kind: "decision", At: now, Summary: "DO NOT EXPOSE generated context"},
	} {
		if err := memory.Write("reminder", record); err != nil {
			t.Fatal(err)
		}
	}
	before, _ := json.Marshal(memory.Read("reminder", now))
	old := http.DefaultClient
	defer func() { http.DefaultClient = old }()
	calls, searches := 0, 0
	source := NewSource("Retrieved text", "https://reminder.dev/quran/2#153", "fixture text", false)
	http.DefaultClient = &http.Client{Transport: modelTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		raw, _ := io.ReadAll(req.Body)
		if strings.Contains(string(raw), "DO NOT EXPOSE") {
			t.Error("non-source memory leaked")
		}
		var output any
		if calls < 3 {
			name := "read_context"
			input := map[string]string{}
			if calls == 2 {
				name = "search"
				input["query"] = "patience"
				if !strings.Contains(string(raw), "retained source") {
					t.Error("agent did not receive context before refining")
				}
			}
			output = map[string]any{"stop_reason": "tool_use", "content": []any{map[string]any{"type": "tool_use", "id": "lookup", "name": name, "input": input}}}
		} else {
			answer, _ := json.Marshal(map[string]any{"answer": "A finding from the targeted search.", "sources": []string{source.ID}, "uncertainty": "Application remains a matter for reflection."})
			output = map[string]any{"stop_reason": "end_turn", "content": []any{map[string]string{"type": "text", "text": string(answer)}}}
		}
		body, _ := json.Marshal(output)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})}
	r := Researcher{Name: "reminder", Objective: "Find primary evidence.", Sources: func(record Record) []Source {
		if record.ID != "source" {
			t.Error("read wrong context record")
		}
		return []Source{NewSource("Retained", "https://reminder.dev", string(record.Data), false)}
	}, Search: func(_ context.Context, q string) ([]Source, error) {
		searches++
		if q != "patience" {
			t.Error("wrong query")
		}
		return []Source{source}, nil
	}}
	f, err := r.Investigate(context.Background(), "What is established about patience?", memory)
	if err != nil || calls != 3 || searches != 1 || len(f.Sources) != 1 || f.Sources[0].ID != source.ID {
		t.Fatalf("wrong investigation: %+v %v", f, err)
	}
	after, _ := json.Marshal(memory.Read("reminder", now))
	if string(before) != string(after) {
		t.Fatal("private investigation changed shared memory")
	}
}

func TestInvestigatorRejectsUngroundedAnswers(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test")
	old := http.DefaultClient
	defer func() { http.DefaultClient = old }()
	for _, answer := range []string{`{"answer":"an unsupported claim","sources":[],"uncertainty":""}`, `{"answer":"a claim","sources":["invented"],"uncertainty":""}`} {
		http.DefaultClient = &http.Client{Transport: modelTransport(func(*http.Request) (*http.Response, error) {
			raw, _ := json.Marshal(map[string]any{"stop_reason": "end_turn", "content": []any{map[string]string{"type": "text", "text": answer}}})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
		})}
		if _, err := (Researcher{Name: "reminder"}).Investigate(context.Background(), "A question", nil); err == nil {
			t.Fatal("accepted unsupported finding")
		}
	}
}
