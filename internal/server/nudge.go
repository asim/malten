package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/asim/malten/internal/llm"
	"github.com/asim/malten/internal/osgrid"
	"github.com/asim/malten/internal/push"
)

// nudge.go is the only part of Malten that reaches out to you rather than
// waiting to be opened: an hourly loop over the people who asked for it, which
// usually decides to say nothing.
//
// It is the second — and last — deliberate exception to the stateless model
// (the waitlist is the first), because a notification cannot be composed by a
// browser that isn't running. What it keeps is the minimum needed to say
// something worth reading: the push subscription, roughly where you are (the
// centre of a 1 km grid square), which squares you've explored, and the last
// few nudges so it doesn't repeat itself. It's opt-in, it's disclosed in the UI,
// and unsubscribing deletes the record.
//
// The bar for sending is deliberately high. A loop that fires every hour and
// notifies every hour is spam; the agent is told to return nothing unless
// there's a genuine, time-bound reason to go outside right now.

const (
	nudgeTTL      = 3 * time.Hour  // if the phone's off, this stops being true
	nudgeGap      = 20 * time.Hour // at most one a day, per person
	nudgeFromHour = 8              // local time, inclusive
	nudgeToHour   = 20             // …and exclusive: never at night
	maxSquares    = 400            // cap on the explored-squares list we keep
	maxRecent     = 6              // recent nudges kept, to avoid repeating
)

// subscriber is one opted-in device.
type subscriber struct {
	ID       string            `json:"id"`
	Sub      push.Subscription `json:"sub"`
	Lat      float64           `json:"lat"` // centre of the 1 km square they were last in
	Lng      float64           `json:"lng"`
	Square   string            `json:"square"`
	Squares  []string          `json:"squares"`   // squares they've stood in
	TZOffset int               `json:"tz_offset"` // minutes, as JS getTimezoneOffset (west is positive)
	Recent   []string          `json:"recent"`    // the last few nudges sent
	LastSent time.Time         `json:"last_sent"`
	Created  time.Time         `json:"created"`
}

// localHour is the subscriber's own wall-clock hour, so nobody is woken at 3am.
func (s subscriber) localHour(now time.Time) int {
	return now.UTC().Add(-time.Duration(s.TZOffset) * time.Minute).Hour()
}

// subStore is the list of subscribers, kept in memory and mirrored to one JSON
// file so it survives a restart.
type subStore struct {
	mu   sync.Mutex
	path string
	list []*subscriber
}

func newSubStore(path string) *subStore {
	st := &subStore{path: path}
	b, err := os.ReadFile(path)
	if err != nil {
		return st
	}
	if err := json.Unmarshal(b, &st.list); err != nil {
		log.Printf("push: couldn't read %s: %v", path, err)
	}
	return st
}

// saveLocked writes the list out. Callers hold the lock.
func (st *subStore) saveLocked() {
	b, err := json.Marshal(st.list)
	if err != nil {
		return
	}
	tmp := st.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		log.Printf("push: couldn't save subscribers: %v", err)
		return
	}
	if err := os.Rename(tmp, st.path); err != nil {
		log.Printf("push: couldn't replace %s: %v", st.path, err)
	}
}

// upsert adds or updates a device by its endpoint (which is its identity).
func (st *subStore) upsert(in *subscriber) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, s := range st.list {
		if s.Sub.Endpoint == in.Sub.Endpoint {
			s.Sub, s.Lat, s.Lng, s.Square, s.Squares, s.TZOffset = in.Sub, in.Lat, in.Lng, in.Square, in.Squares, in.TZOffset
			st.saveLocked()
			return
		}
	}
	in.Created = time.Now()
	st.list = append(st.list, in)
	st.saveLocked()
}

func (st *subStore) remove(endpoint string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	out := st.list[:0]
	for _, s := range st.list {
		if s.Sub.Endpoint != endpoint {
			out = append(out, s)
		}
	}
	st.list = out
	st.saveLocked()
}

// snapshot copies the list so the loop can work without holding the lock.
func (st *subStore) snapshot() []subscriber {
	st.mu.Lock()
	defer st.mu.Unlock()
	out := make([]subscriber, 0, len(st.list))
	for _, s := range st.list {
		out = append(out, *s)
	}
	return out
}

// sent records that we nudged someone, and what with.
func (st *subStore) sent(endpoint, text string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, s := range st.list {
		if s.Sub.Endpoint != endpoint {
			continue
		}
		s.LastSent = time.Now()
		s.Recent = append(s.Recent, text)
		if len(s.Recent) > maxRecent {
			s.Recent = s.Recent[len(s.Recent)-maxRecent:]
		}
		st.saveLocked()
		return
	}
}

func (s *Server) pushEnabled() bool { return s.push != nil && s.subs != nil && s.askEnabled() }

// vapidPublic is the key the browser subscribes with, or "" when nudges are off.
func (s *Server) vapidPublic() string {
	if !s.pushEnabled() {
		return ""
	}
	return s.push.PublicKey()
}

// --- HTTP -------------------------------------------------------------------

type subscribeRequest struct {
	Sub      push.Subscription `json:"sub"`
	Lat      float64           `json:"lat"`
	Lng      float64           `json:"lng"`
	Squares  []string          `json:"squares"`
	TZOffset int               `json:"tz_offset"`
}

func (s *Server) handlePushSubscribe(w http.ResponseWriter, r *http.Request) {
	if !s.pushEnabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "nudges aren't configured"})
		return
	}
	var req subscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Sub.Endpoint == "" || req.Sub.Keys.Auth == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "a push subscription is required"})
		return
	}
	if !strings.HasPrefix(req.Sub.Endpoint, "https://") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "endpoint must be https"})
		return
	}
	if len(req.Squares) > maxSquares {
		req.Squares = req.Squares[len(req.Squares)-maxSquares:]
	}

	// Keep the location coarse on purpose: the centre of the 1 km grid square,
	// not the fix. It's enough to say "there's somewhere new north of you".
	sub := &subscriber{
		ID: fmt.Sprintf("%d", time.Now().UnixNano()), Sub: req.Sub,
		Lat: req.Lat, Lng: req.Lng, Squares: req.Squares, TZOffset: req.TZOffset,
	}
	if ref, ok := osgrid.FromWGS84(req.Lat, req.Lng); ok {
		sub.Square = ref.Square
	}
	s.subs.upsert(sub)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handlePushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	if s.subs == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	var req struct {
		Endpoint string `json:"endpoint"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Endpoint != "" {
		s.subs.remove(req.Endpoint) // forgetting is the whole of unsubscribing
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- the loop ---------------------------------------------------------------

const nudgeSystem = `You are Malten, a spatial guide for Great Britain. Someone has asked to be nudged outdoors occasionally. You are deciding whether there is a good enough reason to interrupt them right now, and if so, what to say.

Reply with NOTHING AT ALL — an empty response — unless there is a specific, concrete, time-bound reason to go outside in the next hour or two. Staying silent is the normal outcome and costs nothing; a weak notification costs their trust.

Reasons that are good enough: somewhere close by they have never set foot; weather that suits a particular walk right now; a train or bus that makes somewhere reachable and back; the last of the daylight. Reasons that are not: generic encouragement, exercise advice, anything you'd say on any day, anything you've said to them before.

If you do reply, write one or two short sentences, warm and specific, naming the place and the direction, as if nudging a friend out the door. No greeting, no markdown, no emoji, no exclamation marks. Never mention points, badges, streaks or scores — the reward is the place.`

// startNudges runs the hourly loop until ctx is done. Most hours it sends
// nothing at all.
func (s *Server) startNudges(ctx context.Context, every time.Duration) {
	if !s.pushEnabled() {
		return
	}
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.runNudges(ctx)
			}
		}
	}()
}

// runNudges walks the subscriber list once.
func (s *Server) runNudges(ctx context.Context) {
	now := time.Now()
	for _, sub := range s.subs.snapshot() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if !dueForNudge(sub, now) {
			continue
		}
		text, err := s.composeNudge(ctx, sub)
		if err != nil || strings.TrimSpace(text) == "" {
			continue // silence is the normal outcome
		}
		body, _ := json.Marshal(map[string]string{"title": "Malten", "body": text, "url": "/"})
		switch err := s.push.Send(sub.Sub, body, nudgeTTL); {
		case err == push.ErrGone:
			s.subs.remove(sub.Sub.Endpoint)
		case err != nil:
			log.Printf("push: send failed: %v", err)
		default:
			s.subs.sent(sub.Sub.Endpoint, text)
		}
	}
}

// dueForNudge applies the cheap rules before any model is asked: daylight-ish
// hours in their own timezone, and not too soon after the last one.
func dueForNudge(sub subscriber, now time.Time) bool {
	if sub.Lat == 0 && sub.Lng == 0 {
		return false
	}
	if h := sub.localHour(now); h < nudgeFromHour || h >= nudgeToHour {
		return false
	}
	return now.Sub(sub.LastSent) >= nudgeGap
}

// composeNudge asks the agent whether there's anything worth saying. An empty
// string means no.
func (s *Server) composeNudge(ctx context.Context, sub subscriber) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	snap := s.snapshot(ctx, sub.Lat, sub.Lng)
	if !snap.InGB {
		return "", nil
	}

	var msg strings.Builder
	msg.WriteString("Here's what's live around them right now:\n")
	msg.WriteString(snapshotText(snap))
	msg.WriteString(groundFor(sub, snap).text())
	fmt.Fprintf(&msg, "\nTheir local time is %s.\n", time.Now().UTC().Add(-time.Duration(sub.TZOffset)*time.Minute).Format("Monday 15:04"))
	if len(sub.Recent) > 0 {
		msg.WriteString("\nYou have already said these to them; saying anything like them again is worse than saying nothing:\n")
		for _, r := range sub.Recent {
			fmt.Fprintf(&msg, "- %s\n", r)
		}
	}
	msg.WriteString("\nIs there a reason to interrupt them right now? Reply with the nudge, or with nothing at all.\n")

	var b strings.Builder
	err := s.llm.Run(ctx, nudgeSystem, msg.String(), nil, func(ev llm.Event) {
		if ev.Type == "text" {
			b.WriteString(ev.Text)
		}
	})
	if err != nil {
		return "", err
	}
	out := strings.TrimSpace(b.String())
	// A model that means "nothing" sometimes says so in words.
	if len(out) < 12 || strings.EqualFold(out, "nothing") || strings.HasPrefix(strings.ToLower(out), "no reason") {
		return "", nil
	}
	return out, nil
}

// groundFor rebuilds the "new ground" context for a subscriber from what they
// last uploaded — the same shape the browser sends when the app is open.
func groundFor(sub subscriber, snap aroundSnapshot) *ground {
	been := make(map[string]bool, len(sub.Squares))
	for _, c := range sub.Squares {
		been[c] = true
	}
	g := &ground{Here: snap.Square, Visited: len(been)}
	for _, n := range snap.Neighbours {
		if !been[n.Square] {
			g.Unvisited = append(g.Unvisited, groundNext{Square: n.Square, Dir: n.Dir, Lat: n.Lat, Lng: n.Lng})
		}
	}
	// Vary which unexplored square gets suggested first, so a person who ignores
	// one nudge isn't handed the same square tomorrow.
	if len(g.Unvisited) > 1 {
		rand.Shuffle(len(g.Unvisited), func(i, j int) { g.Unvisited[i], g.Unvisited[j] = g.Unvisited[j], g.Unvisited[i] })
	}
	return g
}
