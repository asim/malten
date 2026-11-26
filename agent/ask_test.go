package agent

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/asim/malten/server"
)

func TestAskAboutSelectedReflection(t *testing.T) {
	s := server.New()
	post := server.NewMessage("A reflection to ask about", server.DefaultStream)
	s.Publish(post)
	daily := server.NewMessage("Passage reference\n\nQuoted verse.", server.DefaultStream)
	daily.Id, daily.Source = "reminder-today", "https://reminder.dev/quran/1#1"
	s.Publish(daily)
	mention := server.NewCommand("A public question", server.DefaultStream)
	s.Publish(mention)
	s.Reply(mention, "A public answer", "Malten", "shared-thread")

	requests := make(chan muRequest, 1)
	mu := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/agent/malten" || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("did not use the configured Mu agent")
		}
		var req muRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		requests <- req
		json.NewEncoder(w).Encode(muReply{Text: "**An answer**", Thread: "mu-thread"})
	}))
	defer mu.Close()
	a := &Agent{server: s, client: mu.Client(), endpoint: mu.URL + "/agent/malten", token: "test-token"}
	for _, tc := range []struct {
		name, question, wantPrompt string
		selected                   *server.Message
		previous                   *readerTurn
	}{
		{"default", "", "Explain this reflection in plain language.", post, nil},
		{"daily reminder", "What does this passage mean?", "What does this passage mean?", daily, nil},
		{"follow-up", "Can you give an example?", "Can you give an example?", post, &readerTurn{"First question", "First answer"}},
		{"further explanation", "", "Explain your previous answer in more detail.", post, &readerTurn{"First question", "First answer"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input, _ := json.Marshal(map[string]any{"stream": tc.selected.Stream, "id": tc.selected.Id, "question": tc.question, "previous": tc.previous})
			w := httptest.NewRecorder()
			a.Ask(w, httptest.NewRequest(http.MethodPost, "/ask", bytes.NewReader(input)))
			if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"text":"**An answer**"}` || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("response: %d %s", w.Code, w.Body.String())
			}
			req := <-requests
			ctx := req.Context.Messages
			if req.Thread != "" || !strings.HasSuffix(req.Prompt, tc.wantPrompt) || len(ctx) == 0 || ctx[0].Text != tc.selected.Text {
				t.Fatalf("incorrect selected context: %+v", req)
			}
			switch {
			case tc.previous != nil:
				if len(ctx) != 3 || ctx[1].Text != tc.previous.Question || ctx[2].Text != tc.previous.Answer {
					t.Fatalf("incorrect follow-up: %+v", ctx)
				}
			case tc.selected == daily:
				if len(ctx) != 2 || ctx[0].Author != "Daily Reminder" || ctx[1].Text != daily.Source {
					t.Fatalf("incorrect passage source: %+v", ctx)
				}
			default:
				if len(ctx) != 1 {
					t.Fatalf("unrelated messages included: %+v", ctx)
				}
			}
		})
	}
	if s.Thread(server.DefaultStream) != "shared-thread" || len(s.Retrieve("", server.DefaultStream, 1, 0, 10)) != 4 {
		t.Fatal("a reader question changed the shared conversation")
	}
}

func TestPrivateAskSharesThePublicAllowance(t *testing.T) {
	s := server.New()
	post := server.NewMessage("Discuss this", server.DefaultStream)
	if err := s.Publish(post); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		done, err := s.BeginAI("192.0.2.3")
		if err != nil {
			t.Fatal(err)
		}
		done()
	}
	a := &Agent{server: s, token: "test"}
	body, _ := json.Marshal(map[string]string{"id": post.Id})
	r := httptest.NewRequest("POST", "/ask", bytes.NewReader(body))
	r.RemoteAddr = "192.0.2.3:1234"
	w := httptest.NewRecorder()
	a.Ask(w, r)
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatal("reader reached Mu after exhausting the allowance", w.Code)
	}
}

func TestAskRequiresLiveReflectionInStream(t *testing.T) {
	s := server.New()
	post := server.NewMessage("A reflection", "gardens")
	s.Publish(post)
	expired := server.NewMessage("An old reflection", "gardens")
	expired.Created = time.Now().Add(-server.StreamTTL).UnixNano()
	s.Publish(expired)
	if err := s.Checkpoint(); err != nil {
		t.Fatal(err)
	}
	a := &Agent{server: s, token: "enabled"}
	for _, tc := range []struct {
		stream, id, question string
		code                 int
	}{
		{"elsewhere", post.Id, "", http.StatusNotFound},
		{"gardens", expired.Id, "", http.StatusNotFound},
		{"gardens", "", "", http.StatusBadRequest},
		{"gardens", post.Id, strings.Repeat("é", server.MaxMessageSize/2+1), http.StatusRequestEntityTooLarge},
	} {
		body, _ := json.Marshal(map[string]string{"stream": tc.stream, "id": tc.id, "question": tc.question})
		w := httptest.NewRecorder()
		a.Ask(w, httptest.NewRequest(http.MethodPost, "/ask", bytes.NewReader(body)))
		if w.Code != tc.code {
			t.Errorf("stream %q, id %q: got %d, want %d", tc.stream, tc.id, w.Code, tc.code)
		}
	}
}
