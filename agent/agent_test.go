package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/asim/malten/server"
)

func TestMuCommandThreads(t *testing.T) {
	type call struct{ Prompt, Thread string }
	calls := make(chan call, 3)
	release := make(chan struct{})
	mu := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/agent/malten" || r.Header.Get("Authorization") != "Bearer test-token" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("unexpected Mu request")
		}
		var request muRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if len(request.Context.Messages) == 0 || !strings.HasPrefix(request.Context.Messages[0].Text, "Malten is a shared network") {
			t.Error("missing app context")
		}
		in := call{request.Prompt, request.Thread}
		calls <- in
		if in.Prompt == "first" {
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		json.NewEncoder(w).Encode(muReply{Text: "Reply to " + in.Prompt, Thread: "thread-" + in.Prompt})
	}))
	defer mu.Close()
	s := server.New()
	s.Commands = make(chan *server.Message, 3)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan struct{})
	a := &Agent{server: s, client: mu.Client(), baseURL: mu.URL + "/agent", token: "test-token"}
	go func() { defer close(done); a.runMu(ctx) }()
	s.Publish(server.NewMessage("ordinary reflection", "a"))
	s.SubmitCommand(server.NewCommand("first", "a"))
	s.SubmitCommand(server.NewCommand("separate", "b"))
	s.SubmitCommand(server.NewCommand("again", "a"))
	got := map[call]bool{}
	for len(got) < 2 {
		select {
		case in := <-calls:
			got[in] = true
		case <-ctx.Done():
			t.Fatal("another stream was blocked")
		}
	}
	if !got[call{"first", ""}] || !got[call{"separate", ""}] {
		t.Fatalf("unexpected initial calls: %v", got)
	}
	close(release)
	select {
	case in := <-calls:
		if in != (call{"again", "thread-first"}) {
			t.Fatal(in)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	for {
		msg := s.Retrieve("", "a", 1, 0, 10)
		if len(msg) == 5 && msg[4].Text == "Reply to again" {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("reply missing")
		case <-time.After(time.Millisecond):
		}
	}
	cancel()
	<-done
}

func TestStreamPromptUsesOnlySameStream(t *testing.T) {
	s := server.New()
	passage := server.NewMessage("Reference\n\nVerse text", server.DefaultStream)
	passage.Id = "reminder-test"
	passage.Source = "https://reminder.dev/quran/1#1"
	s.Publish(passage)
	s.Publish(server.NewMessage("another visitor’s post", server.DefaultStream))
	for _, stream := range []string{server.DefaultStream, "quiet"} {
		req := streamRequest(s, server.NewCommand("what is this?", stream))
		data, _ := json.Marshal(req.Context)
		prompt := string(data)
		if strings.Contains(prompt, "Verse text") != (stream == server.DefaultStream) || strings.Contains(prompt, "another visitor’s post") != (stream == server.DefaultStream) || req.Prompt != "what is this?" {
			t.Fatal(prompt)
		}
	}
}

func TestStreamContextBudgetAndCutoff(t *testing.T) {
	s := server.New()
	for i := 0; i < 8; i++ {
		s.Publish(server.NewMessage(strings.Repeat("界", 1300), "quiet"))
	}
	s.Publish(server.NewMessage("closest reflection", "quiet"))
	q := server.NewCommand(strings.Repeat("a", server.MaxMessageSize), "quiet")
	s.Publish(server.NewMessage("later post", "quiet"))
	req := streamRequest(s, q)
	data, _ := json.Marshal(req.Context)
	prompt := string(data)
	if len(req.Context.Messages) != 7 || !strings.Contains(prompt, "closest reflection") || strings.Contains(prompt, "later post") || req.Prompt != q.Text {
		t.Fatal("incorrect context selection or budget")
	}
}

func TestTranslationUsesOnlySelectedMessage(t *testing.T) {
	s := server.New()
	const stream = "reflections"
	post := server.NewMessage("Բարև", stream)
	s.Publish(post)
	mention := server.NewCommand("a question for the stream", stream)
	s.Publish(mention)
	s.Reply(mention, "an earlier answer", "Malten", "stream-thread")

	mu := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/agent/translate" || r.Header.Get("Authorization") != "Bearer agent-token" {
			t.Error("translation did not use the hosted Translate agent")
		}
		var req muRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		if req.Thread != "" || !strings.Contains(req.Prompt, "en-GB") || len(req.Context.Messages) != 1 || req.Context.Messages[0].Text != post.Text {
			t.Errorf("unexpected translation request: %+v", req)
		}
		json.NewEncoder(w).Encode(muReply{Text: "Hello", Thread: "reader-thread"})
	}))
	defer mu.Close()
	a := &Agent{server: s, baseURL: mu.URL + "/agent", client: mu.Client(), token: "agent-token"}

	input := strings.NewReader(`{"stream":"reflections","id":"` + post.Id + `","to":"en-GB"}`)
	w := httptest.NewRecorder()
	a.Translate(w, httptest.NewRequest(http.MethodPost, "/agent/translate", input))
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"text":"Hello"}` {
		t.Fatalf("reader response: %d %s", w.Code, w.Body.String())
	}
	if s.Thread(stream) != "stream-thread" || len(s.Retrieve("", stream, 1, 0, 10)) != 3 {
		t.Fatal("a reader action changed the shared conversation")
	}
}
