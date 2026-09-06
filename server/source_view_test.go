package server

import (
	"encoding/json"
	"github.com/asim/malten/agent"
	"testing"
	"time"
)

func TestSourceShowsLatestApprovedUpdateWithoutHistory(t *testing.T) {
	s := New()
	s.AgentStreams = []agent.Stream{{Tag: "news"}}
	now := time.Now()
	s.stream.posts = []Post{
		{ID: "older", Stream: "news", Text: "older", Agent: "News", Created: now.Add(-4 * time.Hour).UnixMilli()},
		{ID: "current", Stream: "news", Text: "current", Agent: "News", Created: now.Add(-2 * time.Hour).UnixMilli()},
		{ID: "hidden", Stream: "news", Text: "hidden", Agent: "News", hidden: true, Created: now.Add(-90 * time.Minute).UnixMilli()},
		{ID: "human", Stream: "home", Text: "old human", Created: now.Add(-2 * time.Hour).UnixMilli()},
	}
	w := request(t, s, "GET", "/api/posts?stream=news", "", "")
	var posts []Post
	json.Unmarshal(w.Body.Bytes(), &posts)
	if len(posts) != 1 || posts[0].ID != "current" || posts[0].Created != s.stream.posts[1].Created {
		t.Fatalf("wrong source view: %s", w.Body.String())
	}
	w = request(t, s, "GET", "/api/posts?stream=home", "", "")
	json.Unmarshal(w.Body.Bytes(), &posts)
	if len(posts) != 0 {
		t.Fatal("changed human arrival window")
	}
	s.stream.posts[0].Created = now.Add(-26 * time.Hour).UnixMilli()
	s.stream.posts[1].Created = now.Add(-25 * time.Hour).UnixMilli()
	w = request(t, s, "GET", "/api/posts?stream=news", "", "")
	json.Unmarshal(w.Body.Bytes(), &posts)
	if len(posts) != 0 {
		t.Fatal("revived expired or hidden content")
	}
}
