package server

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestRestartAndExpiry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "streams.json")
	s := New()
	if err := s.Restore(path); err != nil {
		t.Fatal(err)
	}
	if err := s.New("quiet", "", true, 60); err != nil {
		t.Fatal(err)
	}
	message := NewMessage("still here", "quiet")
	s.store(message)
	old := NewMessage("expired reflection", "quiet")
	old.Created -= int64(time.Hour)
	s.store(old)
	if err := s.Checkpoint(); err != nil {
		t.Fatal(err)
	}
	restarted := New()
	if err := restarted.Restore(path); err != nil {
		t.Fatal(err)
	}
	if got := restarted.Retrieve("", "quiet", 1, 0, 25); len(got) != 1 || got[0].Created != message.Created || got[0].Id != message.Id {
		t.Fatal(got)
	}
	if restarted.store(message) != errDuplicate {
		t.Fatal("stored duplicate after restart")
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(data), "expired reflection") {
		t.Fatal(string(data), err)
	}
	// Simulate an idle stream expiring during downtime.
	data = []byte(strings.Replace(string(data), `"Updated":`+strconv.FormatInt(restarted.streams["quiet"].Updated, 10), `"Updated":1`, 1))
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	final := New()
	if err := final.Restore(path); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil || len(final.List()) != 0 || strings.TrimSpace(string(data)) != "{}" {
		t.Fatalf("retained expired stream: %s, %v", data, err)
	}
}

func TestSharedCommandAndReplySurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "streams.json")
	s := New()
	if err := s.Restore(path); err != nil {
		t.Fatal(err)
	}
	s.Commands = make(chan *Message, 1)
	// Two connected clients receive the same events as the persisted history.
	clients := []*Observer{NewObserver("quiet"), NewObserver("quiet")}
	for _, client := range clients {
		s.Observe(client)
		defer close(client.Kill)
	}
	q := NewCommand("What needs my attention?", "quiet")
	if !s.SubmitCommand(q) {
		t.Fatal("command refused")
	}
	for _, client := range clients {
		if m := <-client.Events; m.Id != q.Id || m.Type != "command" {
			t.Fatal(m)
		}
	}
	const source = "https://micro.mu/agent/malten"
	s.Reply(<-s.Commands, "A reply.", source, "owned-thread")
	for _, client := range clients {
		if m := <-client.Events; m.Source != source || m.Text != "A reply." || m.Parent != q.Id || m.Quote == nil || m.Quote.Text != "@malten "+q.Text {
			t.Fatal(m)
		}
	}
	if err := s.Checkpoint(); err != nil {
		t.Fatal(err)
	}
	restored := New()
	if err := restored.Restore(path); err != nil {
		t.Fatal(err)
	}
	history := restored.Retrieve("", "quiet", 1, 0, 25)
	if len(history) != 2 || history[0].Id != q.Id || history[1].Source != source || restored.Thread("quiet") != "owned-thread" || restored.Thread("other") != "" {
		t.Fatalf("incorrect restored conversation: %+v", history)
	}
	if history[1].Parent != q.Id || history[1].Quote == nil || history[1].Quote.Text != "@malten "+q.Text {
		t.Fatal("lost the question on restart", history[1])
	}
	if restored.List()["quiet"].Thread != "" {
		t.Fatal("private thread exposed in stream listing")
	}
	restored.streams["quiet"].ThreadExpires = 1
	if err := restored.Checkpoint(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(data), "owned-thread") {
		t.Fatal("expired continuation retained", err)
	}
}

func TestMessageTextLimit(t *testing.T) {
	for _, text := range []string{strings.Repeat("界", 2000), strings.Repeat("<", 6000), strings.Repeat("a", MaxMessageSize)} {
		reply := NewMessage(text, DefaultStream).Text
		if len(reply) > MaxMessageSize || !utf8.ValidString(reply) {
			t.Fatal("invalid reply boundary")
		}
		if len(text) <= MaxMessageSize && text != reply {
			t.Fatal("short reply changed")
		}
	}
}

func TestPublishPublicRespectsExistingVisibility(t *testing.T) {
	s := New()
	if err := s.New("unlisted", "", true, int(StreamTTL.Seconds())); err != nil {
		t.Fatal(err)
	}
	m := NewMessage("Public post", "unlisted")
	if err := s.PublishPublic(m); err == nil || !s.List()["unlisted"].Private || len(s.Retrieve(m.Id, "unlisted", 0, 0, 1)) != 0 {
		t.Fatal("published into an unlisted stream or changed its visibility")
	}
	m.Stream = "listed"
	if err := s.PublishPublic(m); err != nil {
		t.Fatal(err)
	}
	if st := s.List()["listed"]; st == nil || st.Private || st.TTL != int64(StreamTTL) {
		t.Fatal("public post is not discoverable with the daily TTL", st)
	}
}

func TestQuotesFollowTheOriginal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "streams.json")
	s := New()
	if err := s.Restore(path); err != nil {
		t.Fatal(err)
	}
	if err := s.New("quiet", "", true, 60); err != nil {
		t.Fatal(err)
	}
	parent := NewMessage(strings.Repeat("界", 200), "quiet")
	parent.Source = "https://reminder.dev/quran/1/1"
	parent.References = []Reference{{Title: "Reading", Text: "an excerpt that must expire"}}
	if err := s.store(parent); err != nil {
		t.Fatal(err)
	}
	child := NewMessage("My reflection", "quiet")
	child.Parent = parent.Id
	if err := s.store(child); err != nil {
		t.Fatal(err)
	}
	page := s.Retrieve("", "quiet", 1, 0, 1)
	if len(page) != 1 || page[0].Quote == nil {
		t.Fatal("parent outside the page was unavailable", page)
	}
	q := page[0].Quote
	if q.Source != parent.Source || q.Text != strings.Repeat("界", 179)+"…" || q.Expires != time.Unix(0, parent.Created+int64(time.Minute)).UnixMilli() {
		t.Fatal("invalid quote", q)
	}
	foreign := NewMessage("Other stream", "other")
	foreign.Parent = parent.Id
	if s.Publish(foreign) != errParentGone || s.List()["other"] != nil {
		t.Fatal("accepted a parent from a different stream")
	}
	if err := s.Checkpoint(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(data), `"Quote"`) || strings.Contains(string(data), `"Expires"`) || !strings.Contains(string(data), `"Parent":"`+parent.Id+`"`) {
		t.Fatal("snapshot should store only the link", string(data), err)
	}
	parent.Created -= int64(time.Minute)
	if len(s.Retrieve(parent.Id, "quiet", 0, 0, 1)) != 0 {
		t.Fatal("returned expired parent before cleanup")
	}
	if view := s.Retrieve(child.Id, "quiet", 0, 0, 1)[0]; view.Parent != parent.Id || view.Quote != nil {
		t.Fatal("returned expired excerpt before cleanup", view)
	}
	if err := s.Publish(child); err != nil {
		t.Fatal("retry of an existing reply changed after parent expiry", err)
	}
	late := NewMessage("Too late", "quiet")
	late.Parent = parent.Id
	if s.Publish(late) != errParentGone {
		t.Fatal("accepted reply to expired reflection")
	}
	if err := s.Checkpoint(); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil || strings.Contains(string(data), "界") || strings.Contains(string(data), parent.Source) || strings.Contains(string(data), parent.References[0].Text) {
		t.Fatal("original survived its expiry on disk", err)
	}
}

func TestEvictedParentDoesNotLeaveAQuote(t *testing.T) {
	s := New()
	parent := NewMessage("First", "quiet")
	s.store(parent)
	child := NewMessage("Second", "quiet")
	child.Parent = parent.Id
	s.store(child)
	for range MaxMessages - 1 {
		s.store(NewMessage("Later", "quiet"))
	}
	if view := s.Retrieve(child.Id, "quiet", 0, 0, 1)[0]; view.Parent != parent.Id || view.Quote != nil {
		t.Fatal("evicted parent left a quote", view)
	}
}
