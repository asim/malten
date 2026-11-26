package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
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
	if m.Stream != reminderStream || m.Type != "message" || m.Id != "reminder-"+date || m.Text != "Reference\n\nPassage" || m.Source != "https://reminder.dev/quran/1#1" {
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
	if m.Id != "reminder-"+date || m.Source != dayURL || m.Text != reflection || len(m.References) != 2 || m.References[0].Text != verse || m.References[0].URL != verseURL {
		t.Fatalf("reflection or its source changed: %+v", m)
	}

	s := server.New()
	if err := s.PublishPublic(m); err != nil {
		t.Fatal(err)
	}
	question := server.NewCommand("What does this mean?", reminderStream)
	for _, parent := range []string{"", m.Id} {
		question.Parent = parent
		req := streamRequest(s, question)
		raw, _ := json.Marshal(req.Context)
		if strings.Count(string(raw), reflection) != 1 || !strings.Contains(string(raw), "The verse text.") || !strings.Contains(string(raw), dayURL) {
			t.Fatalf("unexpected stream context: %s", raw)
		}
	}
	ctx := readerRequest(m, "", nil).Context.Messages
	if len(ctx) != 3 || ctx[0].Text != reflection || ctx[1].Text != dayURL || ctx[2].Author != "Readings from Reminder.dev" || !strings.Contains(ctx[2].Text, verseURL) {
		t.Fatal("the reader's explanation did not distinguish the reflection from its source", ctx)
	}
}

func TestOptionalPassageFitsReaderRequest(t *testing.T) {
	m := server.NewMessage(strings.Repeat("<", 4000), server.DefaultStream)
	m.Id, m.Source = "reminder-2026-11-26", "https://reminder.dev/daily/2026-11-26"
	m.References = []server.Reference{{Title: "Qur’an passage", Text: strings.Repeat("&", 1400)}, {Title: "Hadith", Text: strings.Repeat("<", 1400)}}
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
			for _, ref := range m.References {
				if ref.Title == "Qur’an passage" {
					verseURL = ref.URL
				}
			}
			if verseURL != tc.verseURL {
				t.Fatal("unexpected passage link", verseURL)
			}
		})
	}
}

func TestPublishOriginalDailyReflection(t *testing.T) {
	const date = "2026-11-26"
	const original = "You find a stranger’s key on the pavement. What would you hope someone would do if it were yours?"
	const summary = "Today's published reminder."
	const verse = "A Qur’an reference\n\nVerse text"
	const hadith = "Report on lost property\n\nHadith text"
	const name = "Al Muqsit\n\nThe Equitable One"
	var archiveCalls, muCalls int
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/daily/" + date:
			archiveCalls++
			json.NewEncoder(w).Encode(map[string]any{
				"verse": verse, "hadith": hadith, "name": name, "message": summary,
				"links": map[string]string{"verse": "/quran/71#25", "hadith": "/hadith/45#2427", "name": "/names/86"},
			})
		case "/agent/malten":
			muCalls++
			var req muRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Error(err)
			}
			if r.Header.Get("Authorization") != "Bearer test-token" || req.Thread != "" || req.Session != "" ||
				!strings.Contains(req.Prompt, "own experience") || len(req.Context.Messages) != 4 {
				t.Error("unexpected daily request", req)
			}
			for i, want := range []string{verse, hadith, name, summary} {
				if len(req.Context.Messages) <= i || req.Context.Messages[i].Text != want {
					t.Error("missing daily source", want)
				}
			}
			json.NewEncoder(w).Encode(muReply{Text: original, Thread: "editorial-turn"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()

	path := filepath.Join(t.TempDir(), "streams.json")
	newAgent := func() *Agent {
		s := server.New()
		if err := s.Restore(path); err != nil {
			t.Fatal(err)
		}
		return &Agent{server: s, client: api.Client(), endpoint: api.URL + "/agent/malten", token: "test-token"}
	}
	a := newAgent()
	// The home stream may already have today's source reminder after an upgrade.
	legacy := server.NewMessage("An earlier posting of the verse", server.DefaultStream)
	legacy.Id, legacy.Source = "reminder-"+date, reminderOrigin+"/quran/71#25"
	if err := a.server.Publish(legacy); err != nil {
		t.Fatal(err)
	}
	question := server.NewCommand("Someone’s personal question", server.DefaultStream)
	a.server.Publish(question)
	a.server.Reply(question, "Previous shared answer", muMaltenURL, "home-thread")
	for i := range 2 {
		if i > 0 {
			a = newAgent()
		}
		if err := a.publishDaily(context.Background(), api.Client(), api.URL, date); err != nil {
			t.Fatal(err)
		}
		sources := a.server.Retrieve("", reminderStream, 1, 0, 10)
		listing := a.server.List()[reminderStream]
		if len(sources) != 1 || sources[0].Text != summary || sources[0].Source != reminderOrigin+"/daily/"+date || listing == nil || listing.Private || listing.TTL != int64(server.StreamTTL) {
			t.Fatal("missing original reminder or public listing", sources, listing)
		}
		if old := a.server.Retrieve(legacy.Id, server.DefaultStream, 0, 0, 1); len(old) != 1 || old[0].Text != legacy.Text || old[0].Created != legacy.Created {
			t.Fatal("the previous home post lost its permalink or expiry")
		}
		posts := a.server.Retrieve("reflection-"+date, server.DefaultStream, 0, 0, 1)
		if len(posts) != 1 || posts[0].Source != muMaltenURL || posts[0].Text != original || len(posts[0].References) != 4 || a.server.Thread(server.DefaultStream) != "home-thread" {
			t.Fatal("wrong post, attribution, sources or conversation", posts)
		}
		got := posts[0]
		req := readerRequest(got, "Where did this come from?", nil)
		raw, _ := json.Marshal(req.Context)
		for _, want := range []string{"Malten’s daily reflection", "Verse text", "Hadith text", "The Equitable One", "/daily/" + date} {
			if !strings.Contains(string(raw), want) {
				t.Fatalf("missing %q from the reader’s sources: %s", want, raw)
			}
		}
		public := server.NewCommand("Which reading inspired this?", server.DefaultStream)
		public.Parent = got.Id
		raw, _ = json.Marshal(streamRequest(a.server, public).Context)
		if !strings.Contains(string(raw), "Hadith text") || !strings.Contains(string(raw), "The Equitable One") {
			t.Fatal("public reply lost the underlying readings", string(raw))
		}
	}
	if muCalls != 1 || archiveCalls != 1 {
		t.Fatal("a restart fetched or generated a second daily post", muCalls, archiveCalls)
	}
}

func TestMissingReflectionRetriesFromSavedReminder(t *testing.T) {
	const sourceText = "What does it mean to care for something you found?"
	const date = "2026-11-26"
	for _, tc := range []struct{ name, token, output string }{
		{"disabled", "", ""}, {"unavailable", "test", ""}, {"copied source", "test", sourceText}, {"oversize reply", "test", strings.Repeat("a", server.MaxMessageSize+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var archiveCalls, muCalls int
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/agent/malten" {
					muCalls++
					if tc.token == "" || muCalls > 1 {
						json.NewEncoder(w).Encode(muReply{Text: "Who has looked out for you?"})
						return
					}
					if tc.output == "" {
						http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
						return
					}
					json.NewEncoder(w).Encode(muReply{Text: tc.output})
					return
				}
				archiveCalls++
				json.NewEncoder(w).Encode(map[string]string{"message": sourceText, "hadith": "Hadith text"})
			}))
			defer api.Close()
			a := &Agent{server: server.New(), client: api.Client(), endpoint: api.URL + "/agent/malten", token: tc.token}
			path := filepath.Join(t.TempDir(), "streams.json")
			if err := a.server.Restore(path); err != nil {
				t.Fatal(err)
			}
			if err := a.publishDaily(context.Background(), api.Client(), api.URL, date); (err != nil) != (tc.token != "") {
				t.Fatal("unexpected first attempt result", err)
			}
			if posts := a.server.Retrieve("", server.DefaultStream, 1, 0, 10); len(posts) != 0 {
				t.Fatal("an unsuccessful generation filled the home stream", posts)
			}
			posts := a.server.Retrieve("", reminderStream, 1, 0, 10)
			if len(posts) != 1 || posts[0].Text != sourceText || posts[0].Source != reminderOrigin+"/daily/"+date || !slices.ContainsFunc(posts[0].References, func(r server.Reference) bool { return r.Text == "Hadith text" }) {
				t.Fatal("could not recover with the original source", posts)
			}
			before := muCalls
			if (tc.token == "" && before != 0) || (tc.token != "" && before != 1) {
				t.Fatal("unexpected initial calls", before)
			}
			a.server, a.token = server.New(), "enabled"
			if err := a.server.Restore(path); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if err := a.publishDaily(context.Background(), api.Client(), api.URL, date); err != nil {
					t.Fatal(err)
				}
			}
			home := a.server.Retrieve("", server.DefaultStream, 1, 0, 10)
			if len(home) != 1 || home[0].Id != "reflection-"+date || home[0].Source != muMaltenURL || muCalls != before+1 || archiveCalls != 1 {
				t.Fatal("did not fill only the missing reflection", home, muCalls, archiveCalls)
			}
		})
	}
}

func TestExistingHomeReflectionOnlyBackfillsReminder(t *testing.T) {
	const date = "2026-11-26"
	for _, prefix := range []string{"reflection-", "reminder-"} {
		t.Run(prefix, func(t *testing.T) {
			var archiveCalls int
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/daily/"+date {
					t.Error("unexpected generation for an existing original", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				archiveCalls++
				io.WriteString(w, `{"message":"The source reminder."}`)
			}))
			defer api.Close()
			a := &Agent{server: server.New(), client: api.Client(), endpoint: api.URL + "/agent/malten", token: "on"}
			m := server.NewMessage("An existing original.", server.DefaultStream)
			m.Id, m.Source = prefix+date, muMaltenURL
			if err := a.server.Publish(m); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if err := a.publishDaily(context.Background(), api.Client(), api.URL, date); err != nil {
					t.Fatal(err)
				}
			}
			home := a.server.Retrieve("", server.DefaultStream, 1, 0, 10)
			source := a.server.Retrieve("reminder-"+date, reminderStream, 0, 0, 1)
			if len(home) != 1 || home[0].Id != m.Id || home[0].Created != m.Created || len(source) != 1 || archiveCalls != 1 {
				t.Fatal("unexpected backfill", home, source, archiveCalls)
			}
		})
	}
}

func TestDailyReadingsFitMu(t *testing.T) {
	mu := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wire, _ := io.ReadAll(r.Body)
		var req muRequest
		if err := json.Unmarshal(wire, &req); err != nil {
			t.Error(err)
			return
		}
		size := 0
		for _, m := range req.Context.Messages {
			size += len(m.Author) + len(m.Text)
		}
		if len(req.Context.Messages) != 4 || len(wire) > muRequestBytes || size > 16_000 {
			t.Errorf("daily context outside Mu limits: %d sources, %d bytes, %d on the wire", len(req.Context.Messages), size, len(wire))
		}
		json.NewEncoder(w).Encode(muReply{Text: "What comes to mind?"})
	}))
	defer mu.Close()
	a := &Agent{client: mu.Client(), endpoint: mu.URL, token: "enabled"}
	for _, fragment := range []string{"a", "<"} {
		long := strings.Repeat(fragment, server.MaxMessageSize-10)
		m := server.NewMessage("Summary "+long, server.DefaultStream)
		for _, title := range []string{"Qur’an", "Hadith", "Name"} {
			m.References = append(m.References, server.Reference{Title: title, Text: title + long})
		}
		if _, err := a.writeDailyReflection(context.Background(), m); err != nil {
			t.Fatal(err)
		}
	}
}
