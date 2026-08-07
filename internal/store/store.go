// Package store holds the only thing the server keeps: a small, read-only
// self-help knowledge base, seeded at startup into an in-memory SQLite database.
//
// Malten deliberately persists nothing about users. Conversations, issues and
// the "what I'm working through" memory all live on the client and travel up
// with each request; the server assembles a prompt, calls the model, streams a
// reply, and forgets. There is no user data on disk to leak, scope or encrypt.
//
// The pure-Go modernc.org/sqlite driver keeps the whole app a single static
// binary with no cgo.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

// Store wraps an in-memory SQLite database holding the knowledge base.
type Store struct {
	db *sql.DB
}

// Issue is something the user is working through. It is a data-transfer type:
// the client owns the canonical copy, sends open issues up as memory, and the
// server returns any changes the agent made this turn. Nothing is stored here.
type Issue struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Plan      string    `json:"plan"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// Open opens an in-memory database, applies the schema and seeds the self-help
// library. The path argument is accepted for compatibility but a shared
// in-memory database is always used — the server never writes to disk.
func Open(_ string) (*Store, error) {
	// A shared-cache in-memory database kept alive by a single connection.
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	if err := s.seed(); err != nil {
		return nil, fmt.Errorf("seed: %w", err)
	}
	return s, nil
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }

// Ping verifies the database is reachable.
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

// KBCount returns the number of knowledge-base articles.
func (s *Store) KBCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM kb`).Scan(&n)
	return n, err
}

// seed inserts the self-help library on first run. The content is deliberately
// general, non-clinical, well-established technique — not medical advice.
func (s *Store) seed() error {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM kb`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	kb := []struct{ title, content string }{
		{"Starting when your brain won't", "Task initiation is genuinely hard for a lot of neurodivergent people — it isn't laziness. Shrink the task until the first step is almost too small to refuse: open the doc, write one bad sentence, put on your shoes. Starting is the hard part; momentum comes after action, not before it."},
		{"When everything feels like too much", "Overwhelm narrows everything into one grey wall. Get it out of your head and onto paper or the screen: dump every open loop as a list, then circle just one. You don't have to do the list — you only have to pick the next single thing. Rest counts as progress too."},
		{"Body doubling", "Doing a task alongside someone else — in the room, on a call, or even a video of someone working — can make it far easier to start and stay with. The other person isn't helping with the task; their presence just makes the task possible. It's a legitimate tool, not a crutch."},
		{"Grounding when you're overloaded", "For sensory or emotional overload, reduce input first: dim the light, leave the loud room, put on headphones. Then name five things you can see, four you can feel, three you can hear, two you can smell, one you can taste. It brings you back into the present without demanding words."},
		{"Rejection sensitivity", "That sudden, physical wave of shame after a slight or a mistake is real, and it passes faster than it feels like it will. Name it — 'this is rejection sensitivity, not the truth about me' — and wait before acting on it. Feelings are information, not instructions."},
		{"Sleep, energy and time", "Difficult feelings and executive function are almost always worse on poor sleep. Aim for roughly regular sleep and wake times and some daylight in the morning, and plan around your real energy rather than an idealised routine. Time blindness is common — external timers and alarms are fair game."},
		{"Reaching out for support", "Talking to another person helps more than we expect. For ongoing struggles, a GP or therapist can offer real, lasting support. If you ever feel unsafe or think about harming yourself, please contact emergency services or a crisis line straight away — in the UK you can call Samaritans free on 116 123, any time."},
	}
	for _, k := range kb {
		if _, err := s.db.Exec(`INSERT INTO kb(title,content) VALUES(?,?)`, k.title, k.content); err != nil {
			return err
		}
	}
	return nil
}

// SearchKB returns up to k self-help chunks matching query terms. It is a
// simple term-overlap search over title and content, ranked by match count.
func (s *Store) SearchKB(query string, k int) ([]struct{ Title, Content string }, error) {
	if k <= 0 {
		k = 3
	}
	terms := tokenize(query)
	rows, err := s.db.Query(`SELECT title,content FROM kb`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type scored struct {
		title, content string
		score          int
	}
	var all []scored
	for rows.Next() {
		var t, c string
		if err := rows.Scan(&t, &c); err != nil {
			return nil, err
		}
		hay := strings.ToLower(t + " " + c)
		score := 0
		for _, term := range terms {
			if strings.Contains(hay, term) {
				score++
			}
		}
		all = append(all, scored{t, c, score})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Stable insertion sort by score desc; small n.
	for i := 1; i < len(all); i++ {
		for j := i; j > 0 && all[j].score > all[j-1].score; j-- {
			all[j], all[j-1] = all[j-1], all[j]
		}
	}
	var out []struct{ Title, Content string }
	for _, sc := range all {
		if sc.score == 0 {
			continue
		}
		out = append(out, struct{ Title, Content string }{sc.title, sc.content})
		if len(out) >= k {
			break
		}
	}
	// If nothing matched, fall back to the first k docs so the agent still has
	// something to work with rather than an empty result.
	if len(out) == 0 {
		for _, sc := range all {
			out = append(out, struct{ Title, Content string }{sc.title, sc.content})
			if len(out) >= k {
				break
			}
		}
	}
	return out, nil
}

func tokenize(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	})
	stop := map[string]bool{"the": true, "a": true, "an": true, "to": true, "my": true, "i": true, "how": true, "do": true, "can": true, "of": true, "is": true, "for": true, "you": true}
	var out []string
	for _, f := range fields {
		if len(f) < 2 || stop[f] {
			continue
		}
		out = append(out, f)
	}
	return out
}
