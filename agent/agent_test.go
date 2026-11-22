package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/asim/malten/server"
)

func TestMuCommandThreads(t *testing.T) {
	type call struct{ Prompt, Thread string }
	calls := make(chan call, 3)
	mu := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/agent/malten" || r.Header.Get("Authorization") != "Bearer test-token" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("unexpected Mu request")
		}
		var in call
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Error(err)
		}
		calls <- in
		json.NewEncoder(w).Encode(muReply{Text: "Reply to " + in.Prompt, Thread: "thread-" + in.Prompt})
	}))
	defer mu.Close()
	s := server.New()
	s.Commands = make(chan *server.Message, 3)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); runMu(ctx, s, mu.Client(), mu.URL+"/agent/malten", "test-token") }()
	s.Publish(server.NewMessage("ordinary reflection", "a"))
	s.Commands <- server.NewCommand("first", "a")
	s.Commands <- server.NewCommand("separate", "b")
	s.Commands <- server.NewCommand("again", "a")
	for _, want := range []call{{"first", ""}, {"separate", ""}, {"again", "thread-first"}} {
		select {
		case got := <-calls:
			if got != want {
				t.Fatalf("got %+v, want %+v", got, want)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	for {
		msg := s.Retrieve("", "a", 1, 0, 10)
		if len(msg) == 3 && msg[2].Text == "Reply to again" {
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
