// Package store is the SQLite persistence layer. It owns sessions, the full
// message transcript, the seeded "product" data (accounts, orders, knowledge
// base), the support backlog (tickets/escalations) and the audit log.
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
	Sessions    int `json:"sessions"`
	Messages    int `json:"messages"`
	Tickets     int `json:"tickets"`
	Escalations int `json:"escalations"`
}

// Account is a customer record returned by account_lookup.
type Account struct {
	CustomerID    string  `json:"customer_id"`
	Name          string  `json:"name"`
	Email         string  `json:"email"`
	Plan          string  `json:"plan"`
	Status        string  `json:"status"`
	Seats         int     `json:"seats"`
	APICallsMonth int     `json:"api_calls_month"`
	Orders        []Order `json:"orders"`
}

// Order is a single purchase.
type Order struct {
	OrderID     string  `json:"order_id"`
	CustomerID  string  `json:"customer_id,omitempty"`
	Description string  `json:"description"`
	Amount      float64 `json:"amount"`
	Status      string  `json:"status"`
	Refunded    bool    `json:"refunded"`
}

// Ticket is a backlog item (support ticket or human escalation).
type Ticket struct {
	ID         string    `json:"id"`
	SessionID  string    `json:"session_id"`
	CustomerID string    `json:"customer_id"`
	Kind       string    `json:"kind"`
	Summary    string    `json:"summary"`
	Priority   string    `json:"priority"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

// Open opens (creating if needed) a SQLite database at path, applies the schema
// and seeds demo data if the database is empty. Use ":memory:" for tests.
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
		{`SELECT COUNT(*) FROM tickets`, &st.Tickets},
		{`SELECT COUNT(*) FROM tickets WHERE kind='escalation'`, &st.Escalations},
	} {
		if err := s.db.QueryRow(q.sql).Scan(q.dst); err != nil {
			return st, err
		}
	}
	return st, nil
}

func now() time.Time { return time.Now().UTC() }

// seed inserts demo product data on first run (idempotent via INSERT OR IGNORE).
func (s *Store) seed() error {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM accounts`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	accts := []Account{
		{CustomerID: "CUST-1001", Name: "Ada Lovelace", Email: "ada@example.com", Plan: "Pro", Status: "active", Seats: 5, APICallsMonth: 42000},
		{CustomerID: "CUST-1002", Name: "Alan Turing", Email: "alan@example.com", Plan: "Free", Status: "active", Seats: 1, APICallsMonth: 800},
	}
	for _, a := range accts {
		if _, err := s.db.Exec(`INSERT INTO accounts(customer_id,name,email,plan,status,seats,api_calls_month) VALUES(?,?,?,?,?,?,?)`,
			a.CustomerID, a.Name, a.Email, a.Plan, a.Status, a.Seats, a.APICallsMonth); err != nil {
			return err
		}
	}
	orders := []Order{
		{OrderID: "ORD-5001", CustomerID: "CUST-1001", Description: "Pro monthly subscription", Amount: 49.00, Status: "completed"},
		{OrderID: "ORD-5002", CustomerID: "CUST-1001", Description: "Annual plan upgrade", Amount: 499.00, Status: "completed"},
		{OrderID: "ORD-5003", CustomerID: "CUST-1002", Description: "Add-on seat", Amount: 19.00, Status: "completed"},
	}
	for _, o := range orders {
		if _, err := s.db.Exec(`INSERT INTO orders(order_id,customer_id,description,amount,status) VALUES(?,?,?,?,?)`,
			o.OrderID, o.CustomerID, o.Description, o.Amount, o.Status); err != nil {
			return err
		}
	}
	kb := []struct{ title, content string }{
		{"Exporting your data", "You can export your data from Settings -> Data -> Export. Exports are generated as CSV and emailed to you within 15 minutes."},
		{"Resetting your password", "Use the 'Forgot password' link on the login page, or ask support to send a reset link to the email on file."},
		{"Billing, charges and refunds", "Refunds for orders under $200 are processed automatically. Larger refunds require manager approval before they are issued."},
		{"Plans and pricing", "We offer Free, Pro ($49/mo) and Enterprise plans. You can upgrade or downgrade at any time from the billing page."},
		{"API rate limits", "Pro plans include 100k API calls per month. Exceeding the limit returns HTTP 429; contact us to raise your quota."},
		{"Cancelling your subscription", "You can cancel from Settings -> Billing -> Cancel. Access continues until the end of the current billing period."},
	}
	for _, k := range kb {
		if _, err := s.db.Exec(`INSERT INTO kb(title,content) VALUES(?,?)`, k.title, k.content); err != nil {
			return err
		}
	}
	return nil
}

// --- Sessions ---------------------------------------------------------------

// CreateSession inserts a new session with the given id and optional customer.
func (s *Store) CreateSession(id, customerID string) error {
	t := now()
	_, err := s.db.Exec(`INSERT INTO sessions(id,customer_id,created_at,updated_at) VALUES(?,?,?,?)`,
		id, nullable(customerID), t, t)
	return err
}

// SessionExists reports whether a session id is known.
func (s *Store) SessionExists(id string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id=?`, id).Scan(&n)
	return n > 0, err
}

// TouchSession updates updated_at and, if provided, the customer id.
func (s *Store) TouchSession(id, customerID string) error {
	if customerID != "" {
		_, err := s.db.Exec(`UPDATE sessions SET updated_at=?, customer_id=? WHERE id=?`, now(), customerID, id)
		return err
	}
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

// --- Accounts & orders ------------------------------------------------------

// GetAccount loads an account and its orders. Returns (nil, nil) if unknown.
func (s *Store) GetAccount(customerID string) (*Account, error) {
	var a Account
	err := s.db.QueryRow(`SELECT customer_id,name,email,plan,status,seats,api_calls_month FROM accounts WHERE customer_id=?`, customerID).
		Scan(&a.CustomerID, &a.Name, &a.Email, &a.Plan, &a.Status, &a.Seats, &a.APICallsMonth)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT order_id,description,amount,status,refunded FROM orders WHERE customer_id=? ORDER BY order_id`, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var o Order
		var refunded int
		if err := rows.Scan(&o.OrderID, &o.Description, &o.Amount, &o.Status, &refunded); err != nil {
			return nil, err
		}
		o.Refunded = refunded != 0
		a.Orders = append(a.Orders, o)
	}
	return &a, rows.Err()
}

// GetOrder loads a single order by id. Returns (nil, nil) if unknown.
func (s *Store) GetOrder(orderID string) (*Order, error) {
	var o Order
	var refunded int
	err := s.db.QueryRow(`SELECT order_id,customer_id,description,amount,status,refunded FROM orders WHERE order_id=?`, orderID).
		Scan(&o.OrderID, &o.CustomerID, &o.Description, &o.Amount, &o.Status, &refunded)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	o.Refunded = refunded != 0
	return &o, nil
}

// MarkRefunded records a refund against an order.
func (s *Store) MarkRefunded(orderID string, amount float64) error {
	_, err := s.db.Exec(`UPDATE orders SET refunded=1, refund_amount=?, status='refunded' WHERE order_id=?`, amount, orderID)
	return err
}

// SetPasswordReset stamps a password reset time on an account.
func (s *Store) SetPasswordReset(customerID string) error {
	_, err := s.db.Exec(`UPDATE accounts SET password_reset_at=? WHERE customer_id=?`, now(), customerID)
	return err
}

// --- Knowledge base ---------------------------------------------------------

// SearchKB returns up to k knowledge-base chunks matching query terms. It is a
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

// --- Tickets ----------------------------------------------------------------

// CreateTicket inserts a backlog item and returns it.
func (s *Store) CreateTicket(id, sessionID, customerID, kind, summary, priority string) (Ticket, error) {
	t := Ticket{
		ID: id, SessionID: sessionID, CustomerID: customerID, Kind: kind,
		Summary: summary, Priority: priority, Status: "open", CreatedAt: now(),
	}
	_, err := s.db.Exec(`INSERT INTO tickets(id,session_id,customer_id,kind,summary,priority,status,created_at) VALUES(?,?,?,?,?,?,?,?)`,
		t.ID, nullable(t.SessionID), nullable(t.CustomerID), t.Kind, t.Summary, t.Priority, t.Status, t.CreatedAt)
	return t, err
}

// ListTickets returns the backlog, newest first.
func (s *Store) ListTickets() ([]Ticket, error) {
	rows, err := s.db.Query(`SELECT id,COALESCE(session_id,''),COALESCE(customer_id,''),kind,summary,priority,status,created_at FROM tickets ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Ticket
	for rows.Next() {
		var t Ticket
		if err := rows.Scan(&t.ID, &t.SessionID, &t.CustomerID, &t.Kind, &t.Summary, &t.Priority, &t.Status, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// --- Audit ------------------------------------------------------------------

// AuditEntry is one row from the immutable audit log: a tool call, the policy
// decision made about it, and its outcome.
type AuditEntry struct {
	ID         int64     `json:"id"`
	SessionID  string    `json:"session_id"`
	CustomerID string    `json:"customer_id"`
	Tool       string    `json:"tool"`
	Input      string    `json:"input"`
	Decision   string    `json:"decision"`
	Reason     string    `json:"reason"`
	Result     string    `json:"result"`
	IsError    bool      `json:"is_error"`
	CreatedAt  time.Time `json:"created_at"`
}

// ListEscalatedActions returns audit entries where the policy escalated a
// destructive action for human approval (decision='escalate'), newest first.
// These are the "needs approval" items surfaced on the admin page.
func (s *Store) ListEscalatedActions() ([]AuditEntry, error) {
	rows, err := s.db.Query(`SELECT id,COALESCE(session_id,''),COALESCE(customer_id,''),tool,input,decision,COALESCE(reason,''),COALESCE(result,''),is_error,created_at FROM audit_log WHERE decision='escalate' ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var isErr int
		if err := rows.Scan(&e.ID, &e.SessionID, &e.CustomerID, &e.Tool, &e.Input, &e.Decision, &e.Reason, &e.Result, &isErr, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.IsError = isErr != 0
		out = append(out, e)
	}
	return out, rows.Err()
}

// RecordAudit appends an entry to the immutable audit log.
func (s *Store) RecordAudit(sessionID, customerID, tool string, input json.RawMessage, decision, reason, result string, isErr bool) error {
	_, err := s.db.Exec(`INSERT INTO audit_log(session_id,customer_id,tool,input,decision,reason,result,is_error,created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
		nullable(sessionID), nullable(customerID), tool, string(input), decision, reason, result, boolInt(isErr), now())
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
