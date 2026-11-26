package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	a := &Agent{server: s, client: mu.Client(), endpoint: mu.URL + "/agent/malten", token: "test-token"}
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
			if msg[3].Parent != msg[1].Id || msg[4].Parent != msg[2].Id {
				t.Fatal("interleaved replies quoted the wrong question", msg)
			}
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

func TestPostedMentionGetsSharedReply(t *testing.T) {
	const post = "What is the true religion @malten"
	mu := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req muRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Prompt != "What is the true religion" {
			t.Errorf("Mu received %q, decode error %v", req.Prompt, err)
		}
		json.NewEncoder(w).Encode(muReply{Text: "A shared reply", Thread: "stream-thread"})
	}))
	defer mu.Close()
	s := server.New()
	old := server.Default
	server.Default = s
	defer func() { server.Default = old }()
	s.Commands = make(chan *server.Message, 1)
	reader := server.NewObserver(server.DefaultStream)
	reader.Events = make(chan *server.Message, 2)
	s.Observe(reader)
	defer close(reader.Kill)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a := &Agent{server: s, client: mu.Client(), endpoint: mu.URL, token: "test-token"}
	go a.runMu(ctx)
	form := url.Values{"message": {post}}
	req := httptest.NewRequest(http.MethodPost, "/messages", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	server.PostHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("post: %d %s", w.Code, w.Body.String())
	}
	for _, want := range []string{post, "A shared reply"} {
		select {
		case event := <-reader.Events:
			if event.Text != want || event.Stream != server.DefaultStream {
				t.Fatalf("unexpected stream event: %+v", event)
			}
		case <-ctx.Done():
			t.Fatalf("missing %q", want)
		}
	}
	history := s.Retrieve("", server.DefaultStream, 1, 0, 10)
	if len(history) != 2 || history[0].Text != post || history[1].Source != mu.URL || s.Thread(server.DefaultStream) != "stream-thread" {
		t.Fatal("question or reply missing on refresh", history)
	}
	if answer := history[1]; answer.Parent != history[0].Id || answer.Quote == nil || answer.Quote.Text != post {
		t.Fatal("answer did not quote the reader’s original wording", answer)
	}
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

func TestQuotedStreamContext(t *testing.T) {
	s := server.New()
	parent := server.NewMessage("The specific passage", "quiet")
	parent.Source = "https://reminder.dev/quran/1/1"
	s.Publish(parent)
	for range 8 {
		s.Publish(server.NewMessage("More recent reflection", "quiet"))
	}
	for _, stream := range []string{"quiet", "other"} {
		q := server.NewCommand("What does this mean?", stream)
		q.Parent = parent.Id
		context := streamRequest(s, q).Context.Messages
		data, _ := json.Marshal(context)
		if stream == "quiet" {
			if len(context) != 8 || context[1].Author != "Quoted reflection" || !strings.Contains(context[1].Text, parent.Source) || strings.Count(string(data), parent.Text) != 1 {
				t.Fatal("Mu did not receive the specific older reflection", context)
			}
		} else if strings.Contains(string(data), parent.Text) {
			t.Fatal("quoted text crossed streams", context)
		}
	}
}
