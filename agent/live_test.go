package agent

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLiveQuietCadenceAndFallback(t *testing.T) {
	now := time.Date(2026, 6, 1, 7, 10, 0, 0, time.UTC)
	recent := []Post{{Text: "human", Created: now.Add(-10 * time.Minute).UnixMilli()}}
	calls, posts := 0, 0
	source := func(context.Context, time.Time) (Post, error) { calls++; return Post{Text: "fresh", Name: "Test"}, nil }
	live := NewLive(func(string) []Post { return recent }, func(_ context.Context, stream, text, name, photo string, keys ...string) error {
		posts++
		if stream != "europe-london" || keys[0] != "2026-06-01T08+0100" {
			t.Fatalf("bad stream or local key: %s %v", stream, keys)
		}
		recent = append(recent, Post{Text: text, Created: now.UnixMilli()})
		return nil
	}, source)
	live.Observe("europe-london", "Europe/London")
	live.Observe("city", "Europe/London")
	if len(live.zones) != 1 {
		t.Fatal("registered unrelated stream")
	}
	live.check(context.Background(), now)
	if calls != 0 {
		t.Fatal("fetched while people were sharing")
	}
	recent = nil
	live.check(context.Background(), now)
	live.check(context.Background(), now.Add(time.Minute))
	if calls != 1 || posts != 1 {
		t.Fatalf("calls=%d posts=%d", calls, posts)
	}
}

func TestLiveSkipsRepeatsAndLimitsFailedAttempts(t *testing.T) {
	now := time.Now()
	recent := []Post{{Text: "old", Created: now.Add(-2 * time.Hour).UnixMilli()}}
	calls, posts := 0, 0
	sources := []Source{
		func(context.Context, time.Time) (Post, error) { calls++; return Post{Text: "old", Name: "Test"}, nil },
		func(context.Context, time.Time) (Post, error) { calls++; return Post{}, errors.New("offline") },
	}
	live := NewLive(func(string) []Post { return recent }, func(context.Context, string, string, string, string, ...string) error { posts++; return nil }, sources...)
	live.Observe("utc", "UTC")
	live.check(context.Background(), now)
	live.check(context.Background(), now.Add(time.Second))
	if calls != 2 || posts != 0 {
		t.Fatalf("calls=%d posts=%d", calls, posts)
	}
	live.sources = append(live.sources, func(context.Context, time.Time) (Post, error) { return Post{Text: "new", Name: "Test"}, nil })
	live.check(context.Background(), now.Add(time.Hour))
	if posts != 1 {
		t.Fatal("did not use a fresh alternative")
	}
	live.check(context.Background(), now.Add(25*time.Hour))
	if len(live.zones) != 0 {
		t.Fatal("retained inactive zone")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() { live.Run(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("did not stop")
	}
}
