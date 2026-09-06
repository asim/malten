package reflection

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/asim/malten/agent"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestReflectionUsesSearchToolsAndValidatesSources(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test")
	old := http.DefaultClient
	defer func() { http.DefaultClient = old }()
	modelCalls, searches := 0, 0
	sourceFailure := false
	http.DefaultClient = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		var answer any
		switch r.URL.Host {
		case "reminder.dev":
			if sourceFailure {
				return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable"))}, nil
			}
			var request struct {
				Q         string
				Summarise bool
			}
			json.NewDecoder(r.Body).Decode(&request)
			if r.Method != "POST" || request.Q != "patience" || request.Summarise {
				t.Fatal("search should retrieve original texts without another AI summary")
			}
			searches++
			answer = map[string]any{"references": []any{map[string]any{"text": "Fixture source text", "metadata": map[string]string{"source": "quran", "chapter": "2", "verse": "153"}}}}
		case "aslam.org":
			if sourceFailure {
				return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable"))}, nil
			}
			if r.URL.Query().Get("q") != "patience" {
				t.Fatal("wrong search")
			}
			searches++
			answer = map[string]any{"results": []any{map[string]string{"Title": "Fixture reference", "URL": "/quran/2/153", "Kind": "quran", "Content": "Fixture excerpt"}}}
		case "api.anthropic.com":
			modelCalls++
			var body struct {
				System   string
				Tools    []agent.Tool
				Messages []struct{ Content json.RawMessage }
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if len(body.Tools) != 4 || !strings.Contains(body.System, "Islamic") || !strings.Contains(body.System, "UNTRUSTED") {
				t.Fatal("missing tools or grounding")
			}
			if modelCalls == 1 {
				answer = map[string]any{"stop_reason": "tool_use", "content": []any{
					map[string]any{"type": "tool_use", "id": "r", "name": "reminder_search", "input": map[string]string{"query": "patience"}},
					map[string]any{"type": "tool_use", "id": "a", "name": "aslam_search", "input": map[string]string{"query": "patience"}},
				}}
			} else {
				if len(body.Messages) != 3 {
					t.Fatal("missing tool continuation")
				}
				var results []struct{ Content string }
				json.Unmarshal(body.Messages[2].Content, &results)
				if len(results) != 2 {
					t.Fatal("not all tool calls resolved")
				}
				if sourceFailure {
					raw, _ := json.Marshal(map[string]any{"stop_reason": "end_turn", "content": []any{map[string]string{"type": "text", "text": `{"summary":"Thinking about patience.","context":[]}`}}})
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
				}
				var sources []agent.Source
				json.Unmarshal([]byte(results[0].Content), &sources)
				if len(sources) != 1 || sources[0].Text != "Fixture source text" {
					t.Fatal("source lost")
				}
				text, _ := json.Marshal(map[string]any{"summary": "Thinking about patience.", "context": []any{map[string]any{"text": "A generated connection.", "sources": []string{sources[0].ID}}}})
				answer = map[string]any{"stop_reason": "end_turn", "content": []any{map[string]string{"type": "text", "text": string(text)}}}
			}
		default:
			t.Fatalf("unexpected source: %s", r.URL.Host)
		}
		raw, _ := json.Marshal(answer)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})}
	out, err := Summarise(context.Background(), []agent.Observation{{ID: "one", Stream: "k7m2p9x4r6", Text: "Thinking about patience", At: time.Now()}}, nil)
	if err != nil || modelCalls != 2 || searches != 2 {
		t.Fatalf("%+v %v", out, err)
	}
	if out.Context[0].Sources[0].URL != "https://reminder.dev/quran/2#153" || out.Context[0].Sources[0].Text != "" {
		t.Fatal("wrong attribution or exposed tool transcript")
	}
	if _, err = parse(`{"summary":"x","context":[{"text":"x","sources":["invented"]}]}`, nil); err == nil {
		t.Fatal("accepted invented citation")
	}
	sourceFailure = true
	modelCalls = 0
	fallback, err := Summarise(context.Background(), []agent.Observation{{ID: "one", Text: "Patience"}}, nil)
	if err != nil || len(fallback.Context) != 0 || len(fallback.Unavailable) != 2 {
		t.Fatalf("source failure should be explicit without invented context: %+v %v", fallback, err)
	}

}
