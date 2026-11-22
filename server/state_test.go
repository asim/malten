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
