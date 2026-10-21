package server

import (
	"fmt"
	"testing"
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
