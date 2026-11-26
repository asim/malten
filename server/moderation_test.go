package server

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPublicReportReviewAndRemoval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "streams.json")
	s := New()
	if err := s.Restore(path); err != nil {
		t.Fatal(err)
	}
	parent := NewMessage("a reported <comment>", DefaultStream)
	child := NewMessage("A reply", DefaultStream)
	child.Parent = parent.Id
	other := NewMessage("keep this", DefaultStream)
	private := NewMessage("unlisted conversation", "quiet")
	for _, m := range []*Message{parent, child, other, private} {
		if err := s.Publish(m); err != nil {
			t.Fatal(err)
		}
	}
	h := s.ModerationHandler("")
	send := func(method, path, body string, auth bool, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://malten.test"+path, strings.NewReader(body))
		if auth {
			r.SetBasicAuth("moderator", "test-secret")
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if path == "/moderation" {
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	report := func(m *Message) int {
		body, _ := json.Marshal(reportInput{m.Stream, m.Id, "Spam"})
		return send("POST", "/reports", string(body), false, "").Code
	}
	if report(parent) != 204 || send("GET", "/moderation", "", false, "").Code != 404 {
		t.Fatal("reports before moderator setup")
	}
	h = s.ModerationHandler("test-secret")
	if send("GET", "/moderation", "", false, "").Code != 401 {
		t.Fatal("unprotected moderator queue")
	}
	if report(parent) != 204 || report(private) != 404 || report(other) != 204 {
		t.Fatal("unexpected report admission")
	}
	if got := s.Retrieve(parent.Id, DefaultStream, 0, 0, 1); len(got) != 1 || !got[0].Reportable || got[0].Review != "" {
		t.Fatal("pending report changed public visibility")
	}
	if s.Retrieve(private.Id, private.Stream, 0, 0, 1)[0].Reportable {
		t.Fatal("unlisted post offered reporting")
	}
	s = New()
	if err := s.Restore(path); err != nil {
		t.Fatal(err)
	}
	h = s.ModerationHandler("test-secret")
	queue := send("GET", "/moderation", "", true, "").Body.String()
	if !strings.Contains(queue, "a reported &lt;comment&gt;") || strings.Contains(queue, private.Text) || len(s.reports()) != 2 {
		t.Fatal("incorrect review queue", queue)
	}
	action := func(m *Message, decision, origin string) int {
		return send("POST", "/moderation", url.Values{"stream": {m.Stream}, "id": {m.Id}, "action": {decision}}.Encode(), true, origin).Code
	}
	if action(parent, "remove", "https://elsewhere.example") != 403 {
		t.Fatal("accepted cross-origin moderation")
	}
	if action(other, "dismiss", "") != 303 || action(other, "dismiss", "") != 303 || report(other) != 204 || len(s.reports()) != 1 {
		t.Fatal("dismissed report reappeared")
	}
	observer := NewObserver(DefaultStream)
	s.observers[observer.Id] = observer
	if action(parent, "remove", "") != 303 {
		t.Fatal("could not remove reported post")
	}
	select {
	case ev := <-observer.Events:
		if ev.Text != "removed" || ev.Parent != parent.Id {
			t.Fatal(ev)
		}
	default:
		t.Fatal("connected readers missed the removal")
	}
	s.Broadcast(parent)
	if len(observer.Events) != 0 || !s.Removed(parent.Id, parent.Stream) {
		t.Fatal("a late broadcast published a removed post")
	}
	if action(parent, "remove", "") != 303 {
		t.Fatal("could not retry a removal")
	}
	if err := s.Publish(parent); err != nil {
		t.Fatal(err)
	}
	s.Reply(parent, "Late answer", "Malten", "unused")
	for _, d := range []int64{-1, 1} {
		last := int64(0)
		if d < 0 {
			last = time.Now().UnixNano()
		}
		rows := s.Retrieve("", DefaultStream, d, last, 25)
		if len(rows) != 2 {
			t.Fatal("removed post appeared in history", rows)
		}
	}
	if len(s.Retrieve(parent.Id, DefaultStream, 0, 0, 1)) != 0 || s.Retrieve(child.Id, DefaultStream, 0, 0, 1)[0].Quote != nil {
		t.Fatal("removed text or a quoted copy is visible")
	}
	raw, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(raw), "a reported") {
		t.Fatal("removed text survived on disk", err)
	}
	s = New()
	if err := s.Restore(path); err != nil {
		t.Fatal(err)
	}
	if len(s.reports()) != 0 || s.reviewRevision(DefaultStream) != 1 || len(s.Retrieve(parent.Id, DefaultStream, 0, 0, 1)) != 0 {
		t.Fatal("removal lost on restart")
	}
}

func TestPendingReportsExpireWithTheirPosts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "streams.json")
	s := New()
	if err := s.Restore(path); err != nil {
		t.Fatal(err)
	}
	m := NewMessage("temporary reported post", DefaultStream)
	if err := s.Publish(m); err != nil {
		t.Fatal(err)
	}
	if !s.review(reportInput{m.Stream, m.Id, "Spam"}, "report") {
		t.Fatal("report was not recorded")
	}
	if err := s.Checkpoint(); err != nil {
		t.Fatal(err)
	}
	s.mtx.Lock()
	s.streams[m.Stream].Messages[0].Created -= int64(StreamTTL)
	s.mtx.Unlock()
	if len(s.reports()) != 0 {
		t.Fatal("expired post appeared in the moderator queue")
	}
	if err := s.Checkpoint(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(raw), m.Text) || strings.Contains(string(raw), "Spam") {
		t.Fatal("expired report remained on disk", err)
	}
}
