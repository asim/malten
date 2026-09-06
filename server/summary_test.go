package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
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
			if kind == "valid" {
				if w.Code != 200 || !called || len(s.stream.posts) != 2 || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatalf("%d %s", w.Code, w.Body)
				}
				if again := summaryRequest(s, stream, ids); again.Code != 429 {
					t.Fatal("missing separate summary limit")
				}
				r := httptest.NewRequest("POST", "/api/posts", nil)
				if !s.stream.allow(r) {
					t.Fatal("summary consumed posting allowance")
				}
			} else if w.Code != 409 || kind != "deleted during generation" && called {
				t.Fatalf("invalid input reached model: %d %v", w.Code, called)
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
