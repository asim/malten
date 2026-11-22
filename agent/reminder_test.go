package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDailyReminder(t *testing.T) {
	const date = "2026-11-22"
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/daily/"+date {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"verse":"Reference\n\nPassage","links":{"verse":"/quran/1#1"}}`))
	}))
	defer api.Close()
	m, err := fetchReminder(context.Background(), api.Client(), api.URL, date)
	if err != nil {
		t.Fatal(err)
	}
	if m.Stream != "_" || m.Type != "message" || m.Id != "reminder-"+date || m.Text != "Reference\n\nPassage" || m.SourceURL != "https://reminder.dev/quran/1#1" {
		t.Fatalf("unexpected reminder: %+v", m)
	}
	if _, err := fetchReminder(context.Background(), api.Client(), api.URL, "missing"); err == nil {
		t.Fatal("missing date produced a reminder")
	}
}
