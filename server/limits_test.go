package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPublicAIBudgetSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai-usage.json")
	s := New()
	if err := s.ConfigureAI(path, "2"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		finish, err := s.BeginAI("address-only-in-memory")
		if err != nil {
			t.Fatal(err)
		}
		finish()
	}
	raw, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(raw), "address") {
		t.Fatal("unexpected usage snapshot", string(raw), err)
	}
	s = New()
	if err := s.ConfigureAI(path, "2"); err != nil {
		t.Fatal(err)
	}
	_, err = s.BeginAI("another address")
	w := httptest.NewRecorder()
	if !WriteQuotaError(w, err) || w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatal("a different client or restart reset the daily allowance", err)
	}
	s.limits.total.Date = time.Now().UTC().AddDate(0, 0, -1).Format(time.DateOnly)
	finish, err := s.BeginAI("tomorrow")
	if err != nil {
		t.Fatal(err)
	}
	finish()
	if s.limits.total.Count != 1 {
		t.Fatal("daily allowance did not reset")
	}
	s.limits.path = filepath.Join(t.TempDir(), "missing", "usage")
	if _, err := s.BeginAI("unwritable"); err == nil || s.limits.total.Count != 1 {
		t.Fatal("admitted a call without recording its cost")
	}
	s = New()
	if err := s.ConfigureAI(path, "0"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginAI("paused"); !WriteQuotaError(httptest.NewRecorder(), err) {
		t.Fatal("zero daily allowance admitted a free call")
	}
}

func TestPublicAIConcurrencyAndWindow(t *testing.T) {
	s := New()
	var wg sync.WaitGroup
	started := make(chan func(), 12)
	for range 12 {
		wg.Go(func() {
			if done, err := s.BeginAI("same"); err == nil {
				started <- done
			}
		})
	}
	wg.Wait()
	close(started)
	if len(started) != 4 {
		t.Fatalf("%d outstanding calls", len(started))
	}
	for finish := range started {
		finish()
		finish()
	}
	finish, err := s.BeginAI("same")
	if err != nil {
		t.Fatal(err)
	}
	finish()
	if _, err := s.BeginAI("same"); err == nil {
		t.Fatal("sixth free question admitted")
	}
	now := time.Now().Add(11 * time.Minute)
	if err := s.limits.take("ai:same", 5, 10*time.Minute, now); err != nil {
		t.Fatal(err)
	}
}

func TestClientNetworkUsesNearestTrustedHop(t *testing.T) {
	for _, tc := range []struct{ peer, forwarded, want string }{
		{"192.0.2.1:4000", "203.0.113.10", "192.0.2.1"},
		{"127.0.0.1:4000", "203.0.113.99, 192.0.2.8", "192.0.2.8"},
		{"[::1]:4000", "2001:db8::a", "2001:db8::/64"},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tc.peer
		r.Header.Set("X-Forwarded-For", tc.forwarded)
		if got := ClientNetwork(r); got != tc.want {
			t.Fatalf("got %s, want %s", got, tc.want)
		}
	}
}

func TestMCPAndHTTPShareAIAllowance(t *testing.T) {
	s := New()
	s.Commands = make(chan *Message, 8)
	old := Default
	Default = s
	defer func() { Default = old }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	handler := MCPHandler(s)
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("X-Forwarded-For", "192.0.2.41")
		handler.ServeHTTP(w, r)
	}))
	defer web.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	c, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: web.URL, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	call := func() bool {
		res, err := c.CallTool(ctx, &mcp.CallToolParams{Name: "post_message", Arguments: map[string]any{"text": "@malten a question"}})
		if err != nil {
			t.Fatal(err)
		}
		return !res.IsError
	}
	if !call() {
		t.Fatal("first MCP call failed")
	}
	(<-s.Commands).FinishCommand()
	post := func(path, form string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", path, strings.NewReader(form))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.RemoteAddr = "192.0.2.41:80"
		w := httptest.NewRecorder()
		if path == "/commands" {
			PostCommandHandler(w, r)
		} else {
			PostHandler(w, r)
		}
		return w
	}
	for range 4 {
		if w := post("/messages", "message=%40malten+hello"); w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		(<-s.Commands).FinishCommand()
	}
	if call() || post("/commands", "prompt=hello").Code != 429 {
		t.Fatal("an alternate entry point bypassed the limit")
	}
	if w := post("/messages", "message=ordinary"); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	raw, _ := json.Marshal(s.Retrieve("", DefaultStream, 1, 0, 25))
	if len(s.Commands) != 0 || !strings.Contains(string(raw), "ordinary") {
		t.Fatal("limited request entered the queue or blocked a normal post")
	}
}
