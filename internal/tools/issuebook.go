package tools

import (
	"sync"
	"time"

	"github.com/asim/malten/internal/id"
	"github.com/asim/malten/internal/store"
)

// IssueBook is a request-scoped, in-memory set of the user's issues. The server
// seeds it from the client-supplied issues (the user's memory, sent up with the
// request), the issue tools read and mutate it during the turn, and the server
// reads back Changes() to return to the client. Nothing is persisted here.
type IssueBook struct {
	mu      sync.Mutex
	byID    map[string]*store.Issue
	touched []string
	seen    map[string]bool
	now     time.Time
}

// NewIssueBook seeds a book from the client's issues.
func NewIssueBook(seed []store.Issue) *IssueBook {
	b := &IssueBook{byID: map[string]*store.Issue{}, seen: map[string]bool{}, now: time.Now().UTC()}
	for i := range seed {
		iss := seed[i]
		if iss.ID == "" || iss.Title == "" {
			continue
		}
		if iss.Status == "" {
			iss.Status = "open"
		}
		cp := iss
		b.byID[iss.ID] = &cp
	}
	return b
}

func (b *IssueBook) mark(id string) {
	if !b.seen[id] {
		b.seen[id] = true
		b.touched = append(b.touched, id)
	}
}

// Create adds a new open issue and returns it.
func (b *IssueBook) Create(title, plan string) store.Issue {
	b.mu.Lock()
	defer b.mu.Unlock()
	iss := store.Issue{ID: id.New("ISS"), Title: title, Plan: plan, Status: "open", CreatedAt: b.now}
	cp := iss
	b.byID[iss.ID] = &cp
	b.mark(iss.ID)
	return iss
}

// Update changes an issue's plan and/or status (empty strings leave a field
// unchanged). Returns the updated issue and whether the id was found.
func (b *IssueBook) Update(issueID, plan, status string) (*store.Issue, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	iss, ok := b.byID[issueID]
	if !ok {
		return nil, false
	}
	if plan != "" {
		iss.Plan = plan
	}
	if status != "" {
		iss.Status = status
	}
	b.mark(issueID)
	cp := *iss
	return &cp, true
}

// Changes returns the issues created or updated this turn, in the order first
// touched, so the client can merge them into its local store.
func (b *IssueBook) Changes() []store.Issue {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]store.Issue, 0, len(b.touched))
	for _, id := range b.touched {
		if iss, ok := b.byID[id]; ok {
			out = append(out, *iss)
		}
	}
	return out
}
