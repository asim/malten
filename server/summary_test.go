package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/asim/malten/agent"
	"github.com/asim/malten/agent/reflection"
)

func summaryRequest(s *Server, stream string, ids []string) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(map[string]any{"stream": stream, "ids": ids})
	r := httptest.NewRequest("POST", "/api/summary", strings.NewReader(string(raw)))
	r.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 64))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func TestSummaryScopeAndInvalidation(t *testing.T) {
	for _, kind := range []string{"valid", "other stream", "hidden", "expired", "agent", "duplicate", "deleted during generation"} {
		t.Run(kind, func(t *testing.T) {
			s := New()
			s.stream.moderate = func(context.Context, Post) (bool, error) { return true, nil }
			stream := "k7m2p9x4r6"
			p := Post{ID: "one", Stream: stream, Text: "A quiet moment", Created: time.Now().UnixMilli()}
			switch kind {
			case "other stream":
				p.Stream = "elsewhere"
			case "hidden":
				p.hidden = true
			case "expired":
				p.Created = time.Now().Add(-25 * time.Hour).UnixMilli()
			case "agent":
				p.Agent = "Aslam"
			}
			s.stream.posts = []Post{p, {ID: "unrelated", Stream: "different", Text: "not part of this reflection", Created: time.Now().UnixMilli()}}
			called := false
			s.summarise = func(_ context.Context, captures []agent.Observation) (reflection.Result, error) {
				called = true
				if len(captures) != 1 || captures[0].ID != "one" || captures[0].Stream != stream {
					t.Fatal("mixed stream context")
				}
				if kind == "deleted during generation" {
					s.stream.Lock()
					s.stream.posts = nil
					s.stream.Unlock()
				}
				return reflection.Result{Summary: "Noticing a quiet moment."}, nil
			}
			ids := []string{"one"}
			if kind == "duplicate" {
				ids = append(ids, "one")
			}
			w := summaryRequest(s, stream, ids)
			if kind == "valid" || kind == "deleted during generation" {
				if w.Code != 202 || called {
					t.Fatalf("not queued: %d", w.Code)
				}
				if again := summaryRequest(s, stream, ids); again.Code != 202 {
					t.Fatal("retry must reuse pending job", again.Code)
				}
				s.startSummaries(context.Background())
				s.summaryWorkers.Wait()
				if !called {
					t.Fatal("job did not run")
				}
				if kind == "valid" {
					if len(s.stream.posts) != 3 || s.stream.posts[2].Summary != "done" {
						t.Fatal("summary not published", s.stream.posts)
					}
					if again := summaryRequest(s, stream, ids); again.Code != 202 || len(s.stream.posts) != 3 {
						t.Fatal("completed retry duplicated summary")
					}
				} else if len(s.stream.posts) != 0 {
					t.Fatal("deleted captures were published")
				}
			} else if w.Code != 409 || called {
				t.Fatalf("invalid input reached model: %d", w.Code)
			}

		})
	}
}

func TestSummaryRequiresIdentityAndBoundedInput(t *testing.T) {
	s := New()
	for _, tc := range []struct {
		method, body, token string
		want                int
	}{
		{"GET", "", "", 405}, {"POST", `{}`, "", 401},
		{"POST", `{"stream":"home","ids":[]}`, strings.Repeat("a", 64), 400},
		{"POST", `{"stream":"home","ids":["x"],"text":"client supplied"}`, strings.Repeat("a", 64), 400},
	} {
		r := httptest.NewRequest(tc.method, "/api/summary", strings.NewReader(tc.body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+tc.token)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("got %d, want %d", w.Code, tc.want)
		}
	}
}

func TestSummarySurvivesDisconnectAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stream.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.stream.posts = []Post{{ID: "one", Stream: "home", Text: "A long reflection", Created: time.Now().Add(-2 * time.Hour).UnixMilli()}}
	if w := summaryRequest(s, "home", []string{"one"}); w.Code != 202 {
		t.Fatal(w.Code)
	}
	// The HTTP request has ended. Restore the queued job without that connection.
	restarted, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	restarted.summarise = func(ctx context.Context, _ []agent.Observation) (reflection.Result, error) {
		if ctx.Err() != nil {
			t.Fatal(ctx.Err())
		}
		return reflection.Result{Summary: "A moment to reflect."}, nil
	}
	restarted.stream.moderate = func(context.Context, Post) (bool, error) { return true, nil }
	restarted.startSummaries(context.Background())
	restarted.summaryWorkers.Wait()
	again, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.stream.posts) != 2 || again.stream.posts[1].Summary != "done" {
		t.Fatal("result not durable")
	}
	again.stream.posts[1].Created = time.Now().Add(-2 * time.Hour).UnixMilli()
	w := request(t, again, "GET", "/api/posts?stream=home", "", strings.Repeat("a", 64))
	if !strings.Contains(w.Body.String(), "A moment to reflect.") {
		t.Fatal("returning reader lost summary")
	}
	if len(again.AgentObservations()) != 1 {
		t.Fatal("summary leaked into agent observations")
	}
	again.stream.posts[0].hidden = true
	again.stream.prune(time.Now())
	if len(again.stream.posts) != 1 {
		t.Fatal("summary retained reported source")
	}
}

func TestLongReflectionLimit(t *testing.T) {
	if !validPost(Post{Stream: "home", Text: strings.Repeat("a", 20000)}) {
		t.Fatal("long reflection rejected")
	}
	if validPost(Post{Stream: "home", Text: strings.Repeat("a", 20001)}) {
		t.Fatal("unbounded reflection")
	}
}

func TestSummaryFailureAndShutdown(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		s := New()
		s.stream.posts = []Post{{ID: "one", Stream: "home", Text: "reflection", Created: time.Now().UnixMilli()}}
		if w := summaryRequest(s, "home", []string{"one"}); w.Code != 202 {
			t.Fatal(w.Code)
		}
		ctx, cancel := context.WithCancel(context.Background())
		s.summarise = func(context.Context, []agent.Observation) (reflection.Result, error) {
			if shutdown {
				cancel()
			}
			return reflection.Result{Summary: "must not be exposed"}, nil
		}
		s.stream.moderate = func(context.Context, Post) (bool, error) { return false, nil }
		s.startSummaries(ctx)
		s.summaryWorkers.Wait()
		cancel()
		p := s.stream.posts[1]
		if strings.Contains(p.Text, "must not") {
			t.Fatal("unmoderated result exposed")
		}
		if shutdown {
			if p.Summary != "pending" || p.summaryRunning {
				t.Fatal("shutdown lost queued work")
			}
		} else {
			if p.Summary != "failed" {
				t.Fatal("failure not recorded")
			}
			s.stream.limits = map[string]time.Time{}
			if w := summaryRequest(s, "home", []string{"one"}); w.Code != 202 || len(s.stream.posts) != 2 || s.stream.posts[1].Summary != "pending" {
				t.Fatal("failed job cannot retry")
			}
		}
	}
}
