package server

import (
	"encoding/json"
	"github.com/asim/malten/agent"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAgentSeedKeepsStreamsIndependent(t *testing.T) {
	s := New()
	s.AgentStreams = []agent.Stream{{Tag: "morning"}}
	now := time.Now().UnixMilli()
	s.stream.posts = []Post{
		{ID: "local", Stream: "near-test", Text: "local", Created: now},
		{ID: "old", Stream: "morning", Agent: "Test agent", Created: now},
		{ID: "new", Stream: "morning", Agent: "Test agent", Created: now},
		{ID: "human", Stream: "morning", Text: "human", Created: now},
		{ID: "hidden", Stream: "morning", Agent: "Test agent", Created: now, hidden: true},
	}
	for _, tc := range []struct {
		path string
		want int
	}{
		{"/api/posts?stream=near-test&seed=morning", 2},
		{"/api/posts?stream=&seed=morning", 1},
		{"/api/posts?stream=other&seed=morning", 0},
		{"/api/posts?stream=morning", 3},
		{"/api/posts?stream=near-test&seed=anything", 1},
	} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		var posts []Post
		if err := json.Unmarshal(w.Body.Bytes(), &posts); err != nil {
			t.Fatal(err)
		}
		if len(posts) != tc.want {
			t.Fatalf("%s: %d posts, want %d", tc.path, len(posts), tc.want)
		}
		if tc.want == 2 && posts[1].ID != "new" {
			t.Fatal("did not select latest approved reminder")
		}
	}
	s.AgentStreams = nil
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/posts?seed=morning", nil))
	if w.Body.String() != "[]\n" {
		t.Fatal("unregistered agent still seeded the feed")
	}
}
