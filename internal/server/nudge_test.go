package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/asim/malten/internal/llm"
	"github.com/asim/malten/internal/push"
)

// stubAnthropic streams one text reply (or nothing at all) and records the
// prompt it was given.
func stubAnthropic(t *testing.T, reply string, seen *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			System   string `json:"system"`
			Messages []struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if seen != nil && len(req.Messages) > 0 && len(req.Messages[0].Content) > 0 {
			*seen = req.System + "\n" + req.Messages[0].Content[0].Text
		}
		w.Header().Set("Content-Type", "text/event-stream")
		lines := []string{`{"type":"content_block_start","index":0,"content_block":{"type":"text"}}`}
		if reply != "" {
			b, _ := json.Marshal(reply)
			lines = append(lines, fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":%s}}`, b))
		}
		lines = append(lines, `{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`, `{"type":"message_stop"}`)
		for _, l := range lines {
			_, _ = w.Write([]byte("data: " + l + "\n\n"))
		}
		w.(http.Flusher).Flush()
	}))
}

// offline points the snapshot's upstreams at a stub, so the nudge tests never
// touch the network (and don't wait on it).
func offline(t *testing.T) {
	t.Helper()
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(stub.Close)
	wOld, tOld, oOld := weatherAPI, tflAPI, overpassEndpoint
	weatherAPI, tflAPI, overpassEndpoint = stub.URL, stub.URL, stub.URL
	t.Cleanup(func() { weatherAPI, tflAPI, overpassEndpoint = wOld, tOld, oOld })
}

// testServer wires a server with the agent stubbed and the network stubbed out.
func testServer(t *testing.T, reply string, seen *string) *Server {
	t.Helper()
	offline(t)
	anthropic := stubAnthropic(t, reply, seen)
	t.Cleanup(anthropic.Close)

	c := llm.New("test-key", "")
	c.URL = anthropic.URL
	priv, _, err := push.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	sender, err := push.NewSender(priv, "mailto:test@example.com")
	if err != nil {
		t.Fatal(err)
	}
	return &Server{llm: c, push: sender, subs: newSubStore(filepath.Join(t.TempDir(), "push.json"))}
}

func TestNudgeSendsWhenThereIsAReason(t *testing.T) {
	var prompt string
	s := testServer(t, "The towpath north of you is a square you've never walked, and there's an hour of light left.", &prompt)

	var sent int
	pushSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent++
		w.WriteHeader(201)
	}))
	defer pushSrv.Close()
	var sub push.Subscription
	sub.Endpoint = pushSrv.URL + "/push/one"
	sub.Keys.P256dh = validP256dh(t)
	sub.Keys.Auth = "BTBZMqHH6r4Tts7J_aSIgg"

	// Standing at Hampton Court, having explored the square they're in but not
	// the ones around it. Local noon, so it's a reasonable hour.
	s.subs.upsert(&subscriber{
		Sub: sub, Lat: 51.4036, Lng: -0.3378,
		Squares:  []string{"TQ 15 68"},
		TZOffset: tzForNoon(),
	})

	s.runNudges(context.Background())

	if sent != 1 {
		t.Fatalf("sent %d notifications, want 1", sent)
	}
	// What the agent was told matters as much as that it was asked.
	for _, want := range []string{
		"TQ 15 68",       // where they are
		"never been in",  // the unexplored squares around them
		"NOTHING AT ALL", // the instruction to stay quiet by default
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("nudge prompt missing %q:\n%s", want, prompt)
		}
	}
	// A nudge that was sent is remembered, so the next one doesn't repeat it.
	list := s.subs.snapshot()
	if len(list) != 1 {
		t.Fatalf("subscribers = %d", len(list))
	}
	if len(list[0].Recent) != 1 || list[0].LastSent.IsZero() {
		t.Errorf("send not recorded: %+v", list[0])
	}

	// Straight after, they're not due again.
	if dueForNudge(list[0], time.Now()) {
		t.Errorf("nudged twice in a row")
	}
}

// Silence is the normal outcome: an empty reply must send nothing at all.
func TestNudgeStaysQuiet(t *testing.T) {
	s := testServer(t, "", nil)
	var sent int
	pushSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent++
		w.WriteHeader(201)
	}))
	defer pushSrv.Close()

	var sub push.Subscription
	sub.Endpoint = pushSrv.URL + "/push/quiet"
	sub.Keys.P256dh = validP256dh(t)
	sub.Keys.Auth = "BTBZMqHH6r4Tts7J_aSIgg"
	s.subs.upsert(&subscriber{Sub: sub, Lat: 51.4036, Lng: -0.3378, TZOffset: tzForNoon()})

	s.runNudges(context.Background())
	if sent != 0 {
		t.Errorf("sent %d notifications for an empty reply", sent)
	}
	if list := s.subs.snapshot(); !list[0].LastSent.IsZero() {
		t.Errorf("silence counted as a send")
	}
}

// The nudge window is the subscriber's own wall clock, so this has to hold at
// every hour of the day — including the hour the test suite happens to run in.
func TestNudgeWindowEveryHour(t *testing.T) {
	for h := 0; h < 24; h++ {
		now := time.Date(2026, 8, 23, h, 0, 0, 0, time.UTC)
		for local := 0; local < 24; local++ {
			sub := subscriber{Lat: 51.4, Lng: -0.3, TZOffset: (h - local) * 60}
			if got := sub.localHour(now); got != local {
				t.Fatalf("UTC %02d:00, offset %d → local %d, want %d", h, sub.TZOffset, got, local)
			}
			want := local >= nudgeFromHour && local < nudgeToHour
			if dueForNudge(sub, now) != want {
				t.Errorf("UTC %02d:00, local %02d:00: due = %v, want %v", h, local, !want, want)
			}
		}
	}
}

// Nobody gets woken at 3am, and nobody gets two in a day.
func TestDueForNudge(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC) // noon UTC
	base := subscriber{Lat: 51.4, Lng: -0.3}

	if !dueForNudge(base, now) {
		t.Errorf("midday UTC subscriber should be due")
	}
	// Same instant, a timezone where it's the middle of the night (UTC+13).
	night := base
	night.TZOffset = -13 * 60
	if dueForNudge(night, now) {
		t.Errorf("nudged at %d:00 local", night.localHour(now))
	}
	// Nudged an hour ago.
	recent := base
	recent.LastSent = now.Add(-time.Hour)
	if dueForNudge(recent, now) {
		t.Errorf("nudged again an hour later")
	}
	// Yesterday is fine.
	yesterday := base
	yesterday.LastSent = now.Add(-25 * time.Hour)
	if !dueForNudge(yesterday, now) {
		t.Errorf("not due a day later")
	}
	// No location, nothing to say.
	if dueForNudge(subscriber{}, now) {
		t.Errorf("nudged without a location")
	}
}

// Unsubscribing forgets the device — that's the whole of the privacy promise.
func TestUnsubscribeForgets(t *testing.T) {
	s := testServer(t, "", nil)
	var sub push.Subscription
	sub.Endpoint = "https://push.example.com/one"
	sub.Keys.P256dh = validP256dh(t)
	sub.Keys.Auth = "BTBZMqHH6r4Tts7J_aSIgg"
	s.subs.upsert(&subscriber{Sub: sub, Lat: 51.4, Lng: -0.3})

	rec := httptest.NewRecorder()
	s.handlePushUnsubscribe(rec, httptest.NewRequest(http.MethodPost, "/api/push/unsubscribe",
		strings.NewReader(`{"endpoint":"https://push.example.com/one"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if n := len(s.subs.snapshot()); n != 0 {
		t.Errorf("%d subscribers left after unsubscribe", n)
	}

	// And it survives a restart as empty, rather than resurrecting.
	if n := len(newSubStore(s.subs.path).snapshot()); n != 0 {
		t.Errorf("%d subscribers came back from disk", n)
	}
}

// tzForNoon is the offset (JS getTimezoneOffset semantics: west is positive)
// that puts the subscriber at midday local, whenever the test happens to run.
// localHour = UTC hour - offset/60, so offset = (UTC hour - 12) * 60.
func tzForNoon() int { return (time.Now().UTC().Hour() - 12) * 60 }

// validP256dh returns a well-formed subscriber public key.
func validP256dh(t *testing.T) string {
	t.Helper()
	_, pub, err := push.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return pub
}
