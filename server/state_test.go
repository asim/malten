package server

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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
	if restarted.store(message) {
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
		if m := <-client.Events; m.Source != source || m.Text != "A reply." {
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
