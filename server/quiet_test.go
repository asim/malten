package server

import (
	"context"
	"testing"
	"time"
)

func TestQuietAgentYieldsToHumanDuringModeration(t *testing.T) {
	s := New()
	s.stream.moderate = func(context.Context, Post) (bool, error) {
		s.stream.Lock()
		s.stream.posts = append(s.stream.posts, Post{ID: "human", Stream: "zone", Text: "hello", Created: time.Now().UnixMilli()})
		s.stream.Unlock()
		return true, nil
	}
	if err := s.PublishQuietAgent(context.Background(), "zone", "reminder", "Agent", "", "hour"); err != nil {
		t.Fatal(err)
	}
	if len(s.stream.posts) != 1 || s.stream.posts[0].ID != "human" {
		t.Fatal("agent posted over new human activity")
	}
	s.stream.moderate = func(context.Context, Post) (bool, error) {
		t.Fatal("busy stream reached moderation")
		return false, nil
	}
	if err := s.PublishQuietAgent(context.Background(), "zone", "another", "Agent", "", "hour"); err != nil {
		t.Fatal(err)
	}
}
