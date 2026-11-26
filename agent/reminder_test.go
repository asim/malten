package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/asim/malten/server"
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
	if m.Stream != "_" || m.Type != "message" || m.Id != "reminder-"+date || m.Text != "Reference\n\nPassage" || m.Source != "https://reminder.dev/quran/1#1" {
		t.Fatalf("unexpected reminder: %+v", m)
	}
	if _, err := fetchReminder(context.Background(), api.Client(), api.URL, "missing"); err == nil {
		t.Fatal("missing date produced a reminder")
	}
}

func TestDailyReflectionAndItsSources(t *testing.T) {
	const date = "2026-11-26"
	const reflection = "Think about what it means to look after something someone has lost."
	const verse = "Passage reference\n\nThe verse text."
	const verseURL = "https://reminder.dev/quran/71#25"
	const dayURL = "https://reminder.dev/daily/" + date
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"message": reflection, "verse": verse, "links": map[string]string{"verse": "/quran/71#25"}})
	}))
	defer api.Close()
	m, err := fetchReminder(context.Background(), api.Client(), api.URL, date)
	if err != nil {
		t.Fatal(err)
	}
	if m.Id != "reminder-"+date || m.Source != dayURL || m.Text != reflection || m.Metadata == nil || m.Metadata.Description != verse || m.Metadata.Url != verseURL {
		t.Fatalf("reflection or its source changed: %+v", m)
	}

	s := server.New()
	if err := s.Publish(m); err != nil {
		t.Fatal(err)
	}
	question := server.NewCommand("What does this mean?", server.DefaultStream)
	for _, parent := range []string{"", m.Id} {
		question.Parent = parent
		req := streamRequest(s, question)
		raw, _ := json.Marshal(req.Context)
		if strings.Count(string(raw), reflection) != 1 || !strings.Contains(string(raw), "The verse text.") || !strings.Contains(string(raw), dayURL) {
			t.Fatalf("unexpected stream context: %s", raw)
		}
	}
	ctx := readerRequest(m, "", nil).Context.Messages
	if len(ctx) != 3 || ctx[0].Text != reflection || ctx[1].Text != dayURL || ctx[2].Author != "Qur’an passage" || !strings.Contains(ctx[2].Text, verseURL) {
		t.Fatal("the reader's explanation did not distinguish the reflection from its source", ctx)
	}
}

func TestOptionalPassageFitsReaderRequest(t *testing.T) {
	m := server.NewMessage(strings.Repeat("<", 4000), server.DefaultStream)
	m.Id, m.Source = "reminder-2026-11-26", "https://reminder.dev/daily/2026-11-26"
	m.Metadata = &server.Metadata{Description: strings.Repeat("&", 1400)}
	question := strings.Repeat("a", server.MaxMessageSize)
	req := readerRequest(m, question, nil)
	wire, _ := json.Marshal(req)
	if len(wire) > muRequestBytes || !strings.HasSuffix(req.Prompt, question) || req.Context.Messages[0].Text != m.Text {
		t.Fatal("optional passage displaced the question or reflection")
	}
}

func TestDailyReflectionWithMissingFields(t *testing.T) {
	for _, tc := range []struct {
		name, payload, text, verseURL string
		wantErr                       bool
	}{
		{"reflection only", `{"message":"Something to reflect on."}`, "Something to reflect on.", "", false},
		{"only a related link", `{"message":"Something to reflect on.","links":{"verse":"/quran/71#25"}}`, "Something to reflect on.", "https://reminder.dev/quran/71#25", false},
		{"no valid verse link", `{"message":"Reflection","verse":"Passage","links":{"verse":"https://example.org/quran/1"}}`, "Reflection", "", false},
		{"no content", `{"message":" ","verse":" "}`, "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, tc.payload) }))
			defer api.Close()
			m, err := fetchReminder(context.Background(), api.Client(), api.URL, "2026-11-26")
			if (err != nil) != tc.wantErr {
				t.Fatal(err)
			}
			if tc.wantErr {
				return
			}
			if m.Text != tc.text || m.Source != "https://reminder.dev/daily/2026-11-26" {
				t.Fatal(m)
			}
			var verseURL string
			if m.Metadata != nil {
				verseURL = m.Metadata.Url
			}
			if verseURL != tc.verseURL {
				t.Fatal("unexpected passage link", verseURL)
			}
		})
	}
}
