package reflection

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/asim/malten/agent"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(v any) *http.Response {
	raw, _ := json.Marshal(v)
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw)))}
}
func completed(v any) *http.Response {
	raw, _ := json.Marshal(v)
	return response(map[string]any{"stop_reason": "end_turn", "content": []any{map[string]string{"type": "text", "text": string(raw)}}})
}

func TestSupervisorDelegatesToInvestigatingAgents(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test")
	old := http.DefaultClient
	defer func() { http.DefaultClient = old }()
	for _, failure := range []bool{false, true} {
		var plans, investigations, syntheses, searches atomic.Int32
		http.DefaultClient = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host != "api.anthropic.com" {
				searches.Add(1)
				if failure {
					return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable"))}, nil
				}
				switch r.URL.Host {
				case "reminder.dev":
					var q struct {
						Q         string
						Summarise bool
					}
					json.NewDecoder(r.Body).Decode(&q)
					if r.Method != "POST" || q.Q != "patience" || q.Summarise {
						t.Error("wrong primary-source search")
					}
					return response(map[string]any{"references": []any{map[string]any{"text": "Fixture primary text", "metadata": map[string]string{"source": "quran", "chapter": "2", "verse": "153"}}}}), nil
				case "aslam.org":
					if r.URL.Query().Get("q") != "patience" {
						t.Error("wrong Aslam query")
					}
					return response(map[string]any{"results": []any{map[string]string{"Title": "Fixture explanation", "URL": "/quran/2/153", "Kind": "quran", "Content": "Fixture explanation excerpt"}}}), nil
				default:
					t.Errorf("unexpected tool host %s", r.URL.Host)
				}
			}
			var req struct {
				System   string
				Tools    []agent.Tool
				Messages []struct{ Content json.RawMessage }
			}
			if json.NewDecoder(r.Body).Decode(&req) != nil {
				t.Error("invalid request")
			}
			if !strings.Contains(req.System, agent.Foundation) || !strings.Contains(req.System, "UNTRUSTED") {
				t.Error("missing standing foundation or trust boundary")
			}
			if strings.Contains(string(req.Messages[0].Content), "k7m2p9x4r6") {
				t.Error("unlisted address entered model context")
			}
			if req.System == routing {
				plans.Add(1)
				if len(req.Tools) != 0 {
					t.Error("supervisor should delegate questions, not execute search tools")
				}
				return completed(plan{Summary: "Thinking about patience.", Questions: []question{{Agent: "reminder", Question: "What do the primary texts establish about patience?", Captures: []string{"one"}}, {Agent: "aslam", Question: "What surrounding explanation helps understand patience?", Captures: []string{"one"}}}}), nil
			}
			if req.System == synthesis {
				syntheses.Add(1)
				var content []struct{ Text string }
				json.Unmarshal(req.Messages[0].Content, &content)
				var input struct{ Findings []agent.Finding }
				json.Unmarshal([]byte(content[0].Text), &input)
				if len(req.Tools) != 0 || len(input.Findings) != 2 || input.Findings[0].Agent != "reminder" || input.Findings[1].Agent != "aslam" {
					t.Error("synthesis did not receive focused findings")
				}
				for _, f := range input.Findings {
					if f.Answer == "" || len(f.Sources) != 1 || f.Sources[0].Text == "" {
						t.Error("missing evidence behind generated findings")
					}
				}
				return completed(map[string]any{"summary": "Thinking about patience.", "context": []any{map[string]any{"text": "A grounded connection.", "sources": []string{input.Findings[0].Sources[0].ID}}}}), nil
			}
			investigations.Add(1)
			if len(req.Tools) != 3 {
				t.Error("investigator needs its own context, search and refresh capabilities")
			}
			if len(req.Messages) == 1 {
				return response(map[string]any{"stop_reason": "tool_use", "content": []any{map[string]any{"type": "tool_use", "id": "lookup", "name": "search", "input": map[string]string{"query": "patience"}}}}), nil
			}
			var results []struct{ Content string }
			json.Unmarshal(req.Messages[2].Content, &results)
			if failure {
				return completed(map[string]any{"answer": "", "sources": []string{}, "uncertainty": "Sources unavailable."}), nil
			}
			var sources []agent.Source
			json.Unmarshal([]byte(results[0].Content), &sources)
			if len(sources) != 1 {
				t.Error("missing tool sources")
				return completed(map[string]any{}), nil
			}
			return completed(map[string]any{"answer": "A focused finding from the retrieved material.", "sources": []string{sources[0].ID}, "uncertainty": "A brief excerpt does not settle every application."}), nil
		})}
		out, err := Summarise(context.Background(), []agent.Observation{{ID: "one", Stream: "k7m2p9x4r6", Text: "Thinking about patience", At: time.Now()}}, nil)
		if err != nil || plans.Load() != 1 || investigations.Load() != 4 || searches.Load() != 2 {
			t.Fatalf("unexpected execution: %+v %v", out, err)
		}
		if failure {
			if syntheses.Load() != 0 || len(out.Context) != 0 || len(out.Unavailable) != 2 {
				t.Fatalf("invented context after source failure: %+v", out)
			}
		} else {
			if syntheses.Load() != 1 || out.Context[0].Sources[0].URL != "https://reminder.dev/quran/2#153" || out.Context[0].Sources[0].Text != "" {
				t.Fatalf("incorrect result: %+v", out)
			}
		}
	}
}

func TestRoutingAndSynthesisRejectInventedEvidence(t *testing.T) {
	captures := []agent.Observation{{ID: "one"}}
	valid := plan{Summary: "A thought.", Questions: []question{{Agent: "reminder", Question: "What helps?", Captures: []string{"one"}}}}
	for _, kind := range []string{"unknown agent", "unrelated capture", "missing primary sources", "duplicate agent", "too many"} {
		p := valid
		p.Questions = append([]question(nil), valid.Questions...)
		switch kind {
		case "unknown agent":
			p.Questions[0].Agent = "web"
		case "unrelated capture":
			p.Questions[0].Captures = []string{"other"}
		case "missing primary sources":
			p.Questions[0].Agent = "news"
		case "duplicate agent":
			p.Questions = append(p.Questions, p.Questions[0])
		case "too many":
			for len(p.Questions) < 5 {
				p.Questions = append(p.Questions, p.Questions[0])
			}
		}
		raw, _ := json.Marshal(p)
		if _, err := parsePlan(string(raw), captures); err == nil {
			t.Errorf("accepted %s", kind)
		}
	}
	if _, err := parse(`{"summary":"x","context":[{"text":"x","sources":["invented"]}]}`, nil); err == nil {
		t.Fatal("accepted invented synthesis citation")
	}
}

func TestInvestigationsRunConcurrentlyAndPartialFailureSurvives(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test")
	old := http.DefaultClient
	defer func() { http.DefaultClient = old }()
	source := agent.NewSource("Retrieved explanation", "https://aslam.org/quran/2/153", "fixture evidence", true)
	http.DefaultClient = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		var req struct{ System string }
		json.NewDecoder(r.Body).Decode(&req)
		if req.System == routing {
			return completed(plan{Summary: "A reflection.", Questions: []question{{Agent: "reminder", Question: "Primary text?", Captures: []string{"one"}}, {Agent: "aslam", Question: "Explanation?", Captures: []string{"one"}}}}), nil
		}
		return completed(map[string]any{"summary": "A reflection.", "context": []any{map[string]any{"text": "A supported connection.", "sources": []string{source.ID}}}}), nil
	})}
	started := make(chan string, 2)
	release := make(chan struct{})
	finished := make(chan struct{})
	go func() { defer close(finished); <-started; <-started; close(release) }()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := summarise(ctx, []agent.Observation{{ID: "one"}}, func(ctx context.Context, q question) (agent.Finding, error) {
		started <- q.Agent
		select {
		case <-release:
		case <-ctx.Done():
			return agent.Finding{}, ctx.Err()
		}
		if q.Agent == "reminder" {
			return agent.Finding{}, fmt.Errorf("unavailable")
		}
		return agent.Finding{Answer: "A supported explanation.", Sources: []agent.Source{source}}, nil
	})
	if err != nil || len(out.Context) != 1 || len(out.Unavailable) != 1 || out.Unavailable[0] != "reminder" {
		t.Fatalf("partial failure discarded supported findings: %+v %v", out, err)
	}
	<-finished
}

// Exercise the actual multimodal wire format at both supervisor stages. Source
// investigation is stubbed so the test isolates the handoff that lost photos.
func TestPhotoOnlySummaryKeepsImagesAndContentVoice(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test")
	old := http.DefaultClient
	defer func() { http.DefaultClient = old }()
	for _, grounded := range []bool{true, false} {
		calls := 0
		http.DefaultClient = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
			var req struct {
				System   string
				Messages []struct {
					Content []struct {
						Type, Text string
						Source     struct{ Data string }
					}
				}
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			calls++
			if !strings.Contains(req.System, voice) {
				t.Fatal("missing content-directed voice")
			}
			images := []string{}
			for _, block := range req.Messages[0].Content {
				if block.Type == "image" {
					images = append(images, block.Source.Data)
				}
			}
			if strings.Join(images, ",") != "photo4,photo3,photo2" {
				t.Fatalf("lost latest images: %v", images)
			}
			if strings.Contains(req.Messages[0].Content[0].Text, "data:image") {
				t.Fatal("duplicated image bytes in text")
			}
			if calls == 1 {
				return completed(plan{Summary: "Sunlight across water.", Questions: []question{{Agent: "reminder", Question: "What do primary sources say about water?", Captures: []string{"four"}}}}), nil
			}
			if !strings.Contains(req.Messages[0].Content[0].Text, "Sunlight across water.") {
				t.Fatal("initial visual account lost")
			}
			return completed(map[string]any{"summary": "Sunlight across water.", "context": []any{}}), nil
		})}
		captures := []agent.Observation{}
		for i, id := range []string{"one", "two", "three", "four"} {
			captures = append(captures, agent.Observation{ID: id, Photo: fmt.Sprintf("data:image/jpeg;base64,photo%d", i+1)})
		}
		result, err := summarise(context.Background(), captures, func(context.Context, question) (agent.Finding, error) {
			f := agent.Finding{}
			if grounded {
				f.Sources = []agent.Source{agent.NewSource("Fixture", "https://reminder.dev/quran/21#30", "Fixture text", false)}
			}
			return f, nil
		})
		if err != nil || result.Summary != "Sunlight across water." {
			t.Fatalf("photo lost: %+v %v", result, err)
		}
		want := 1
		if grounded {
			want = 2
		}
		if calls != want {
			t.Fatalf("calls: %d", calls)
		}
		if !strings.HasPrefix(captures[0].Photo, "data:image") {
			t.Fatal("original capture modified")
		}
	}
}
