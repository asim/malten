package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewsLocalMorningAndDailyPublication(t *testing.T) {
	calls, posts := 0, 0
	n := NewNews(func(_ context.Context, stream, text, name string, keys ...string) error {
		posts++
		if stream != "europe-london" || name != "News · Micro" || len(keys) != 1 || keys[0] != "2026-06-01" {
			t.Fatalf("unexpected post: %s %v", stream, keys)
		}
		return nil
	})
	n.Observe("europe-london", "Europe/London")
	n.Observe("america-los-angeles", "America/Los_Angeles")
	n.Observe("city", "Europe/London")
	n.Observe("invalid", "Invalid")
	if len(n.zones) != 2 {
		t.Fatal("registered invalid timezone or unrelated hashtag")
	}
	n.headlines = func(context.Context) (string, error) { calls++; return "headlines", nil }
	now := time.Date(2026, 6, 1, 7, 15, 0, 0, time.UTC) // 08:15 London under DST.
	for stream, zone := range n.zones {
		zone.seen = now
		n.zones[stream] = zone
	}
	n.check(context.Background(), now)
	n.check(context.Background(), now.Add(time.Minute))
	if calls != 1 || posts != 1 {
		t.Fatalf("calls=%d posts=%d", calls, posts)
	}
	n.check(context.Background(), now.Add(2*time.Hour))
	if calls != 1 {
		t.Fatal("posted after the morning window")
	}
	n.check(context.Background(), now.Add(25*time.Hour))
	if len(n.zones) != 0 {
		t.Fatal("inactive streams retained")
	}
}

func TestNewsFailureAndCancellation(t *testing.T) {
	n := NewNews(func(context.Context, string, string, string, ...string) error { return errors.New("unavailable") })
	n.Observe("europe-london", "Europe/London")
	now := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC) // 08:00 London in winter.
	n.headlines = func(context.Context) (string, error) { return "headlines", nil }
	n.check(context.Background(), now)
	if n.zones["europe-london"].posted != "" {
		t.Fatal("failed publication marked complete")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	n.headlines = func(context.Context) (string, error) { t.Fatal("fetched after shutdown"); return "", nil }
	done := make(chan struct{})
	go func() { n.Run(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("agent did not stop")
	}
}

func TestReadHeadlines(t *testing.T) {
	payload := `{"items":[
 {"category":"Bad","title":"Unsafe","url":"javascript:alert(1)"},
 {"category":"World","title":"One","url":"https://example.com/one"},
 {"category":"World","title":"Duplicate topic","url":"https://example.com/two"},
 {"category":"Tech","title":"Two","url":"https://example.com/three"},
 {"category":"Finance","title":"Three","url":"https://example.com/four"},
 {"category":"Other","title":"Four","url":"https://example.com/five"}]}`
	raw, _ := json.Marshal(map[string]any{"result": map[string]any{"content": []any{map[string]string{"type": "text", "text": payload}}}})
	text, err := readHeadlines(strings.NewReader(string(raw)))
	if err != nil || !strings.Contains(text, "World · One") || strings.Contains(text, "Unsafe") || strings.Contains(text, "Duplicate topic") || strings.Contains(text, "Other ·") {
		t.Fatalf("%s: %v", text, err)
	}
	if len([]rune(text)) > 1200 {
		t.Fatal("post too large")
	}
	for _, raw := range []string{`{"error":{"code":-1}}`, `{"result":{"isError":true}}`, `{"result":{"content":[]}}`} {
		if _, err := readHeadlines(strings.NewReader(raw)); err == nil {
			t.Fatal("accepted failed or empty response")
		}
	}
}
