package server

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDaySeedKeepsStreamsIndependent(t *testing.T) {
	t.Setenv("MALTEN_DAY", "true")
	s := New()
	now := time.Now().UnixMilli()
	s.stream.posts = []Post{
		{ID: "local", Stream: "near-test", Text: "local", Created: now},
		{ID: "old", Stream: "morning", Agent: "Aslam · adhkar", Created: now},
		{ID: "new", Stream: "morning", Agent: "Aslam · adhkar", Created: now},
		{ID: "human", Stream: "morning", Text: "human", Created: now},
		{ID: "hidden", Stream: "morning", Agent: "Aslam · adhkar", Created: now, hidden: true},
	}
	for _, tc := range []struct {
		path string
		want int
	}{
		{"/api/posts?stream=near-test&moment=morning", 2},
		{"/api/posts?stream=&moment=morning", 1},
		{"/api/posts?stream=other&moment=morning", 0},
		{"/api/posts?stream=morning", 3},
		{"/api/posts?stream=near-test&moment=anything", 1},
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
	t.Setenv("MALTEN_DAY", "false")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/posts?moment=morning", nil))
	if w.Body.String() != "[]\n" {
		t.Fatal("disabled day agent still seeded the feed")
	}
}
