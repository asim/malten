package server

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestStoreKeepsNewestMessages(t *testing.T) {
	s := New()
	for i := 0; i < MaxMessages+2; i++ {
		m := NewMessage(fmt.Sprint(i), "notes")
		m.Created = int64(i + 1)
		s.Store(m)
	}
	got := s.Retrieve("", "notes", 1, 0, MaxMessages)
	if len(got) != MaxMessages || got[0].Text != "2" || got[len(got)-1].Text != fmt.Sprint(MaxMessages+1) {
		t.Fatalf("unexpected FIFO contents: %d messages", len(got))
	}
	if !s.streams["notes"].Private {
		t.Fatal("new message streams must default to private")
	}
	if len(s.Retrieve("", "other", 1, 0, 10)) != 0 {
		t.Fatal("messages leaked between streams")
	}
}

func TestRetrieveByIDAndDirection(t *testing.T) {
	s := New()
	for i := 1; i <= 3; i++ {
		s.Store(&Message{Id: fmt.Sprint(i), Stream: "notes", Created: int64(i)})
	}
	got := s.Retrieve("2", "notes", 1, 0, 10)
	if len(got) != 1 || got[0].Id != "2" {
		t.Fatalf("ID lookup: %v", got)
	}
	got = s.Retrieve("", "notes", -1, 3, 2)
	if len(got) != 2 || got[0].Id != "2" || got[1].Id != "1" {
		t.Fatalf("backward pagination: %v", got)
	}
	got = s.Retrieve("", "notes", 1, 2, 10)
	if len(got) != 1 || got[0].Id != "3" {
		t.Fatalf("forward pagination: %v", got)
	}
}

func TestBroadcastIsolation(t *testing.T) {
	s := New()
	a, b := NewObserver("a"), NewObserver("b")
	s.observers[a.Id], s.observers[b.Id] = a, b
	m := NewMessage("hello", "a")
	s.Broadcast(m)
	select {
	case got := <-a.Events:
		if got != m {
			t.Fatal("wrong message")
		}
	default:
		t.Fatal("missing message")
	}
	select {
	case <-b.Events:
		t.Fatal("message leaked to another stream")
	default:
	}
}

func TestCommandUsesStreamContextAndKeepsPrompt(t *testing.T) {
	old := Answer
	defer func() { Answer = old }()
	calls := make(chan []*Message, 1)
	Answer = func(_ context.Context, messages []*Message) (string, error) { calls <- messages; return "reply", nil }
	s := New()
	s.process(NewMessage("another stream", "other"))
	s.process(NewMessage("ordinary thought", "local"))
	select {
	case <-calls:
		t.Fatal("ordinary post triggered AI")
	default:
	}
	s.process(NewCommand("question", "local"))
	select {
	case history := <-calls:
		if len(history) != 2 || history[0].Text != "ordinary thought" || history[1].Text != "question" {
			t.Fatal("incorrect context")
		}
	case <-time.After(time.Second):
		t.Fatal("command did not invoke AI")
	}
	select {
	case reply := <-s.Events:
		if reply.Role != "assistant" || reply.Stream != "local" {
			t.Fatal("wrong reply stream")
		}
	case <-time.After(time.Second):
		t.Fatal("missing reply")
	}
	if len(s.Retrieve("", "local", 1, 0, 10)) != 2 {
		t.Fatal("prompt not stored")
	}
}
