package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/asim/malten/agent"
)

func TestThoughtsWindowAndDiscovery(t *testing.T) {
	s := New()
	now := time.Now().UnixMilli()
	s.AgentStreams = []agent.Stream{{Tag: "morning"}}
	s.stream.posts = []Post{
		{ID: "past", Stream: "zone", Text: "before arrival", Created: now - 7200000},
		{ID: "recent", Stream: "zone", Text: "recent", Created: now - 1000},
		{ID: "old-agent", Stream: "morning", Text: "old reminder", Agent: "Test", Created: now - 7200000},
		{ID: "hidden", Stream: "zone", Text: "reported", Created: now, hidden: true},
	}
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}
	w := get("/thoughts?stream=zone&seed=morning")
	var posts []Post
	if err := json.Unmarshal(w.Body.Bytes(), &posts); err != nil {
		t.Fatal(err)
	}
	if len(posts) != 1 || posts[0].ID != "recent" {
		t.Fatalf("arrival included history: %s", w.Body.String())
	}
	w = get("/thoughts?stream=zone&last=" + strconv.FormatInt(now-1000, 10))
	if w.Body.String() != "[]\n" {
		t.Fatal("cursor must be exclusive")
	}
	for _, last := range []string{"bad", "-1"} {
		if get("/thoughts?last="+last).Code != 400 {
			t.Fatal("accepted bad timestamp")
		}
	}
	w = get("/streams")
	var streams map[string]int64
	if err := json.Unmarshal(w.Body.Bytes(), &streams); err != nil {
		t.Fatal(err)
	}
	if streams["zone"] != now-1000 {
		t.Fatal("discovery includes a hidden thought")
	}
	s.stream.moderate = func(context.Context, Post) (bool, error) { return true, nil }
	w = request(t, s, "POST", "/thoughts", `{"stream":"zone","text":"new thought"}`, strings.Repeat("ab", 32))
	if w.Code != 201 {
		t.Fatalf("post: %d", w.Code)
	}
}

func TestAgentKeySurvivesRestartBeforeModeration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stream.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.stream.moderate = func(context.Context, Post) (bool, error) { return true, nil }
	if err = s.PublishAgent(context.Background(), "morning", "first wording", "Aslam", "2026-09-06"); err != nil {
		t.Fatal(err)
	}
	created := s.stream.posts[0].Created
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.stream.moderate = func(context.Context, Post) (bool, error) { t.Fatal("duplicate reached moderation"); return false, nil }
	if err = s.PublishAgent(context.Background(), "morning", "different wording", "Aslam", "2026-09-06"); err != nil {
		t.Fatal(err)
	}
	if len(s.stream.posts) != 1 || s.stream.posts[0].Created != created {
		t.Fatal("restart repeated a scheduled post")
	}
	raw, _ := json.Marshal(s.stream.posts[0])
	if strings.Contains(string(raw), "2026-09-06") {
		t.Fatal("internal key exposed publicly")
	}
}
