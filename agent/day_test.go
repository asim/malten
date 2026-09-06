package agent

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestReadReminderPreservesCompleteAdhkar(t *testing.T) {
	data := `{"results":[
 {"Kind":"islamqa","Role":"daily dua","Content":"unrelated","URL":"/adhkar/a"},
 {"Kind":"adhkar","Role":"evening dhikr","Content":"wrong time","URL":"/adhkar/b"},
 {"Kind":"adhkar","Role":"daily dua","Content":"cut short...","URL":"/adhkar/c"},
 {"Kind":"adhkar","Role":"daily dua","Content":"unsafe link","URL":"//other.example/adhkar/d"},
 {"Kind":"adhkar","Role":"daily dua","Content":"All praise is to Allah.","Title":"Source title","URL":"/adhkar/e"}]}`
	got, err := readReminder(strings.NewReader(data), "daily dua", 3)
	want := "All praise is to Allah.\n\nSource title\nhttps://aslam.org/adhkar/e"
	if err != nil || got != want {
		t.Fatalf("%q, %v", got, err)
	}
	if _, err = readReminder(strings.NewReader(`{"results":[]}`), "daily dua", 0); err == nil {
		t.Fatal("empty search should not create a reminder")
	}
}

func TestDayStopsWithServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		Day(ctx, func(context.Context, string, string, string, string) error {
			t.Error("posted after cancellation")
			return nil
		})
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("day agent did not stop")
	}
}
