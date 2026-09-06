package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAgentObservationsRespectVisibility(t *testing.T) {
	s := New()
	now := time.Now().UnixMilli()
	s.stream.posts = []Post{{ID: "human", Stream: "home", Text: "visible", Created: now, owner: "private owner"}, {ID: "hidden", Stream: "home", Text: "reported", Created: now, hidden: true}, {ID: "agent", Stream: "news", Text: "generated", Created: now, Agent: "News"}}
	s.stream.posts = append(s.stream.posts, Post{ID: "unlisted", Stream: strings.Repeat("ab", 16), Text: "a brain dump", Created: now})
	s.stream.posts = append(s.stream.posts, Post{ID: "short", Stream: "k7m2p9x4r6", Text: "another brain dump", Created: now})
	observations := s.AgentObservations()
	if len(observations) != 1 || observations[0].ID != "human" {
		t.Fatalf("observed inappropriate input: %+v", observations)
	}
	raw, _ := json.Marshal(observations)
	if strings.Contains(string(raw), "private owner") {
		t.Fatal("identity in agent context")
	}
	s.stream.posts = nil
	if len(s.AgentObservations()) != 0 {
		t.Fatal("deleted content remains visible")
	}
	for _, url := range []string{"/api/posts?stream=reminder", "/api/posts?stream=nature"} {
		r := httptest.NewRecorder()
		s.Handler().ServeHTTP(r, httptest.NewRequest("GET", url, nil))
		if strings.Contains(r.Body.String(), "source") || r.Code != 200 {
			t.Fatal("internal context served publicly")
		}
	}
}
