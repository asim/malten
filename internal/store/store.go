// Package store is the SQLite persistence layer. It owns sessions, the full
// message transcript, a small self-help knowledge base, the issues you're
// working through, and an audit log of tool calls.
//
// It uses the pure-Go modernc.org/sqlite driver so the whole application builds
// and ships as a single static binary with no cgo.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/asim/malten/internal/llm"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

// Store wraps a SQLite database.
type Store struct {
	db   *sql.DB
	path string
}

// Stats is a snapshot of row counts, used for startup diagnostics and health.
type Stats struct {
	Sessions int `json:"sessions"`
	Messages int `json:"messages"`
	Issues   int `json:"issues"`
}

// Issue is something the user is working through. The agent logs it, optionally
// with a plan, so it can be revisited.
type Issue struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Title     string    `json:"title"`
	Plan      string    `json:"plan"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// Open opens (creating if needed) a SQLite database at path, applies the schema
// and seeds the self-help library if it is empty. Use ":memory:" for tests.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// SQLite tolerates a single writer; cap connections to avoid "database is
	// locked" under concurrent HTTP requests.
	db.SetMaxOpenConns(1)
	s := &Store{db: db, path: path}
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

// Path returns the database path this store was opened with.
func (s *Store) Path() string { return s.path }

// Ping verifies the database is reachable.
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

// Stats returns current row counts.
func (s *Store) Stats() (Stats, error) {
	var st Stats
	for _, q := range []struct {
		sql string
		dst *int
	}{
		{`SELECT COUNT(*) FROM sessions`, &st.Sessions},
		{`SELECT COUNT(*) FROM messages`, &st.Messages},
		{`SELECT COUNT(*) FROM issues`, &st.Issues},
	} {
		if err := s.db.QueryRow(q.sql).Scan(q.dst); err != nil {
			return st, err
		}
	}
	return st, nil
}

func now() time.Time { return time.Now().UTC() }

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

// --- Sessions ---------------------------------------------------------------

// CreateSession inserts a new session with the given id.
func (s *Store) CreateSession(id string) error {
	t := now()
	_, err := s.db.Exec(`INSERT INTO sessions(id,created_at,updated_at) VALUES(?,?,?)`, id, t, t)
	return err
}

// SessionExists reports whether a session id is known.
func (s *Store) SessionExists(id string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id=?`, id).Scan(&n)
	return n > 0, err
}

// TouchSession updates updated_at.
func (s *Store) TouchSession(id string) error {
	_, err := s.db.Exec(`UPDATE sessions SET updated_at=? WHERE id=?`, now(), id)
	return err
}

// --- Messages ---------------------------------------------------------------

// AppendMessage persists one message of the transcript.
func (s *Store) AppendMessage(sessionID string, m llm.Message) error {
	content, err := json.Marshal(m.Content)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO messages(session_id,role,content,created_at) VALUES(?,?,?,?)`,
		sessionID, string(m.Role), string(content), now())
	return err
}

// LoadMessages returns the full transcript for a session in order.
func (s *Store) LoadMessages(sessionID string) ([]llm.Message, error) {
	rows, err := s.db.Query(`SELECT role,content FROM messages WHERE session_id=? ORDER BY id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []llm.Message
	for rows.Next() {
		var role, content string
		if err := rows.Scan(&role, &content); err != nil {
			return nil, err
		}
		var blocks []llm.Block
		if err := json.Unmarshal([]byte(content), &blocks); err != nil {
			return nil, err
		}
		out = append(out, llm.Message{Role: llm.Role(role), Content: blocks})
	}
	return out, rows.Err()
}

// --- Knowledge base ---------------------------------------------------------

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

// --- Issues -----------------------------------------------------------------

// CreateIssue inserts an issue and returns it.
func (s *Store) CreateIssue(id, sessionID, title, plan string) (Issue, error) {
	iss := Issue{
		ID: id, SessionID: sessionID, Title: title, Plan: plan,
		Status: "open", CreatedAt: now(),
	}
	_, err := s.db.Exec(`INSERT INTO issues(id,session_id,title,plan,status,created_at) VALUES(?,?,?,?,?,?)`,
		iss.ID, nullable(iss.SessionID), iss.Title, nullable(iss.Plan), iss.Status, iss.CreatedAt)
	return iss, err
}

// ListIssues returns the issues, newest first.
func (s *Store) ListIssues() ([]Issue, error) {
	rows, err := s.db.Query(`SELECT id,COALESCE(session_id,''),title,COALESCE(plan,''),status,created_at FROM issues ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Issue
	for rows.Next() {
		var iss Issue
		if err := rows.Scan(&iss.ID, &iss.SessionID, &iss.Title, &iss.Plan, &iss.Status, &iss.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, iss)
	}
	return out, rows.Err()
}

// --- Audit ------------------------------------------------------------------

// RecordAudit appends an entry to the immutable audit log.
func (s *Store) RecordAudit(sessionID, tool string, input json.RawMessage, decision, reason, result string, isErr bool) error {
	_, err := s.db.Exec(`INSERT INTO audit_log(session_id,tool,input,decision,reason,result,is_error,created_at) VALUES(?,?,?,?,?,?,?,?)`,
		nullable(sessionID), tool, string(input), decision, reason, result, boolInt(isErr), now())
	return err
}

// --- helpers ----------------------------------------------------------------

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func tokenize(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	})
	// Drop very short/common words to reduce noise.
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
