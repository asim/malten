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
		if stream != "london" || keys[0] != "2026-06-01T08+0100" {
			t.Fatalf("bad stream or local key: %s %v", stream, keys)
		}
		recent = append(recent, Post{Text: text, Created: now.UnixMilli()})
		return nil
	}, source)
	live.Observe("london")
	live.Observe("city")
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
	recent := []Post{{Text: "old", Created: now.Add(-4 * time.Hour).UnixMilli()}}
	calls, posts := 0, 0
	sources := []Source{
		func(context.Context, time.Time) (Post, error) { calls++; return Post{Text: "old", Name: "Test"}, nil },
		func(context.Context, time.Time) (Post, error) { calls++; return Post{}, errors.New("offline") },
	}
	live := NewLive(func(string) []Post { return recent }, func(context.Context, string, string, string, string, ...string) error { posts++; return nil }, sources...)
	live.Observe("home")
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

func TestCityClocksAndSharedRegions(t *testing.T) {
	now := time.Date(2026, 6, 1, 7, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		stream string
		hour   int
	}{{"london", 8}, {"paris", 9}, {"nyc", 3}, {"sf", 0}, {"dubai", 11}, {"singapore", 15}, {"mena", -1}, {"home", -1}} {
		t.Run(tc.stream, func(t *testing.T) {
			called := false
			source := func(_ context.Context, local time.Time) (Post, error) {
				called = true
				if tc.hour < 0 {
					if !local.IsZero() {
						t.Fatal("broad stream assigned a local hour")
					}
				} else if local.Hour() != tc.hour {
					t.Fatalf("local hour %d", local.Hour())
				}
				return Post{Text: "reflection", Name: "Aslam"}, nil
			}
			live := NewLive(func(string) []Post { return nil }, func(context.Context, string, string, string, string, ...string) error { return nil }, source)
			live.Observe(tc.stream)
			live.check(context.Background(), now)
			if !called {
				t.Fatal("city or region was not served")
			}
		})
	}
	live := NewLive(func(string) []Post { return []Post{{Created: now.Add(-2 * time.Hour).UnixMilli()}} }, nil, func(context.Context, time.Time) (Post, error) { t.Fatal("filled a quiet hour"); return Post{}, nil })
	live.Observe("london")
	live.check(context.Background(), now)
	live.Observe("news")
	live.Observe("aslam")
	if len(live.zones) != 1 {
		t.Fatal("dedicated agent streams included in reflection loop")
	}
}
