package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type testMu struct {
	receipt      Receipt
	checkouts    []string
	failCheckout bool
	failStart    bool
	asks         []string
	history      []Message
}

func (m *testMu) Available() bool { return true }
func (m *testMu) Checkout(_ context.Context, ref string, count int) (Receipt, error) {
	m.checkouts = append(m.checkouts, ref)
	if m.receipt.ID == "" {
		m.receipt = Receipt{ID: "mu-receipt", Reference: ref, Participants: count, Amount: 300 * count, Currency: "gbp", Minutes: 60, Status: "pending", CheckoutURL: "https://micro.mu/pay/mu-receipt"}
	}
	if m.failCheckout {
		return Receipt{}, errors.New("connection lost after checkout creation")
	}
	return m.receipt, nil
}
func (m *testMu) Payment(_ context.Context, id string) (Receipt, error) {
	if id != m.receipt.ID {
		return Receipt{}, errors.New("unknown payment")
	}
	return m.receipt, nil
}
func (m *testMu) StartSession(ctx context.Context, id string) (Receipt, error) {
	if m.receipt.Status == "paid" {
		m.receipt.Status = "active"
		m.receipt.EndsAt = time.Now().Add(time.Hour).Unix()
	}
	if m.failStart {
		return Receipt{}, errors.New("connection lost after Start")
	}
	return m.Payment(ctx, id)
}
func (m *testMu) AskSession(_ context.Context, id string, question Message, history []Message) (string, error) {
	if id != m.receipt.ID || m.receipt.Status != "active" {
		return "", errors.New("wrong paid session")
	}
	m.asks = append(m.asks, question.Text)
	m.history = slices.Clone(history)
	return "A private reply.", nil
}

type client struct {
	cookie  *http.Cookie
	handler http.Handler
}

func browser(t *testing.T, h http.Handler) client {
	t.Helper()
	r := httptest.NewRequest("GET", "https://malten.me/session", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	cookies := w.Result().Cookies()
	if w.Code != 200 || len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].Path != "/session" {
		t.Fatal("missing protected guest cookie", w.Code)
	}
	return client{cookies[0], h}
}
func (c client) request(path string, in *input) *httptest.ResponseRecorder {
	var b bytes.Buffer
	method := "GET"
	if in != nil {
		method = "POST"
		json.NewEncoder(&b).Encode(in)
	}
	r := httptest.NewRequest(method, "https://malten.me/session/"+path, &b)
	r.AddCookie(c.cookie)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c.handler.ServeHTTP(w, r)
	return w
}
func (c client) want(t *testing.T, path string, in *input, code int) view {
	t.Helper()
	w := c.request(path, in)
	if w.Code != code {
		t.Fatalf("%s got %d, want %d: %s", path, w.Code, code, w.Body.String())
	}
	if code != 200 {
		return view{}
	}
	var v view
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestGuestBooking(t *testing.T) {
	dir := t.TempDir()
	m := new(testMu)
	open := func() *Store {
		s, err := Open(dir, m)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	s := open()
	h := s.Handler()
	host, guest, stranger := browser(t, h), browser(t, h), browser(t, h)
	ref := uuid.NewString()
	in := &input{Room: ref, Participants: 3}
	m.failCheckout = true
	host.want(t, "book", in, 502)
	s = open()
	h = s.Handler()
	host.handler, guest.handler, stranger.handler = h, h, h
	m.failCheckout = false
	v := host.want(t, "book", in, 200)
	if v.Amount != 900 || v.Minutes != 60 || v.Checkout == "" || len(v.Invites) != 0 || !v.Host {
		t.Fatalf("wrong quote: %+v", v)
	}
	if len(m.checkouts) != 2 || m.checkouts[0] != m.checkouts[1] {
		t.Fatal("retry changed checkout reference")
	}
	host.want(t, "book", in, 200)
	if reopened := host.want(t, "book", &input{Room: uuid.NewString(), Participants: 3}, 200); reopened.Room != ref {
		t.Fatal("a second tab created a duplicate checkout")
	}
	if len(m.checkouts) != 2 {
		t.Fatal("duplicate checkout")
	}
	host.want(t, "book", &input{Room: ref, Participants: 2}, 409)
	stranger.want(t, "book", in, 404)
	stranger.want(t, "return", &input{Payment: "mu-receipt"}, 404)
	v = host.want(t, "return?status=paid", &input{Payment: "mu-receipt"}, 200)
	if v.Status != "pending" {
		t.Fatal("return unlocked the room")
	}
	host.want(t, "start", in, 409)
	host.want(t, "post", &input{Room: ref, Text: "early"}, 409)
	m.receipt.Status = "paid"
	v = host.want(t, "refresh", in, 200)
	if len(v.Invites) != 2 || v.Invites[0] == "" || v.Invites[0] == v.Invites[1] {
		t.Fatal("need a distinct invite per additional place")
	}
	first, second := v.Invites[0], v.Invites[1]
	guest.want(t, "state?room="+ref, nil, 404)
	guest.want(t, "claim", &input{Room: ref, Invite: "incorrect"}, 409)
	claim := &input{Room: ref, Invite: first}
	v = guest.want(t, "claim", claim, 200)
	if v.Host || v.Invites != nil || v.Checkout != "" || v.Status != "paid" {
		t.Fatal("guest received host capabilities")
	}
	guest.want(t, "claim", claim, 200)
	guest.want(t, "start", in, 403)
	guest.want(t, "refresh", in, 403)
	stranger.want(t, "claim", claim, 409)
	m.failStart = true
	host.want(t, "start", in, 502)
	ends := m.receipt.EndsAt
	s = open()
	h = s.Handler()
	host.handler, guest.handler, stranger.handler = h, h, h
	m.failStart = false
	s.reconcile(context.Background())
	if recovered := guest.want(t, "state?room="+ref, nil, 200); recovered.Status != "active" || recovered.EndsAt != ends {
		t.Fatal("guest could not recover while host was away")
	}
	v = host.want(t, "start", in, 200)
	if v.EndsAt != ends || v.Invites[0] != "" || v.Invites[1] != second {
		t.Fatal("restart lost end time or claimed seat")
	}
	if host.want(t, "start", in, 200).EndsAt != ends {
		t.Fatal("second Start changed the clock")
	}
	firstPost := guest.want(t, "post", &input{Room: ref, Text: "A shared reflection."}, 200).Messages[0]
	if firstPost.ID == "" {
		t.Fatal("private message has no ID")
	}
	host.want(t, "post", &input{Room: ref, Text: "missing parent", Ask: true, Parent: uuid.NewString()}, 409)
	const question = "What did the guest say? @Malten"
	v = host.want(t, "post", &input{Room: ref, Text: question, Parent: firstPost.ID}, 200)
	if len(v.Messages) != 3 || v.Messages[0].Author != "Guest 1" || v.Messages[1].Text != question || v.Messages[2].Author != "Malten" || len(m.asks) != 1 || m.asks[0] != "What did the guest say?" || len(m.history) != 1 {
		t.Fatal("wrong shared paid turn", v.Messages, m.asks)
	}
	if v.Messages[1].Parent != firstPost.ID || v.Messages[1].ID == "" || v.Messages[2].Parent != v.Messages[1].ID {
		t.Fatal("incorrect private reply chain", v.Messages)
	}
	if guest.want(t, "state?room="+ref, nil, 200).EndsAt != ends {
		t.Fatal("guest clock differs")
	}
	stranger.want(t, "post", &input{Room: ref, Text: "outside", Ask: true}, 404)
	f, err := os.ReadFile(filepath.Join(dir, ref+".json"))
	if err != nil || bytes.Contains(f, []byte(host.cookie.Value)) || bytes.Contains(f, []byte(first)) {
		t.Fatal("stored a browser credential or a used invite", err)
	}
	s = open()
	h = s.Handler()
	guest.handler, host.handler, stranger.handler = h, h, h
	if messages := guest.want(t, "state?room="+ref, nil, 200).Messages; !slices.Equal(messages, v.Messages) {
		t.Fatal("restart changed private messages or their links", messages)
	}
	late := stranger.want(t, "claim", &input{Room: ref, Invite: second}, 200)
	if late.EndsAt != ends || len(late.Messages) != 3 {
		t.Fatal("late arrival started a new session")
	}
	s.now = func() time.Time { return time.Unix(ends, 0) }
	v = guest.want(t, "state?room="+ref, nil, 200)
	if v.Status != "expired" || len(v.Messages) != 0 {
		t.Fatal("expired chat visible")
	}
	guest.want(t, "post", &input{Room: ref, Text: "late", Ask: true}, 409)
	host.want(t, "start", in, 409)
	if len(m.asks) != 1 {
		t.Fatal("expired prompt reached Mu")
	}
	if err := s.prune(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ref+".json")); !os.IsNotExist(err) {
		t.Fatal("expired room still on disk", err)
	}
	host.want(t, "state?room="+ref, nil, 404)
}

func TestAIAllowancePerBookedPlace(t *testing.T) {
	dir := t.TempDir()
	m := &testMu{}
	s, err := Open(dir, m)
	if err != nil {
		t.Fatal(err)
	}
	host, guest := browser(t, s.Handler()), browser(t, s.Handler())
	ref := uuid.NewString()
	booking := &input{Room: ref, Participants: 2}
	host.want(t, "book", booking, 200)
	m.receipt.Status = "paid"
	paid := host.want(t, "refresh", booking, 200)
	guest.want(t, "claim", &input{Room: ref, Invite: paid.Invites[0]}, 200)
	host.want(t, "start", booking, 200)
	ask := &input{Room: ref, Text: "A question", Ask: true}
	for range 10 {
		host.want(t, "post", ask, 200)
	}
	s, err = Open(dir, m)
	if err != nil {
		t.Fatal(err)
	}
	host.handler, guest.handler = s.Handler(), s.Handler()
	host.want(t, "post", ask, 429)
	host.want(t, "post", &input{Room: ref, Text: "Plain reflection"}, 200)
	r := s.rooms[ref]
	r.op.Lock()
	guest.want(t, "post", ask, 429)
	r.op.Unlock()
	guest.want(t, "post", ask, 200)
	s.now = func() time.Time { return time.Now().Add(11 * time.Minute) }
	host.want(t, "post", ask, 200)
	if len(m.asks) != 12 {
		t.Fatal("unexpected paid Mu calls", len(m.asks))
	}
}

func TestOneClaimWinsAndForeignOriginsCannotPost(t *testing.T) {
	m := new(testMu)
	s, err := Open(t.TempDir(), m)
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	host := browser(t, h)
	ref := uuid.NewString()
	in := &input{Room: ref, Participants: 2}
	host.want(t, "book", in, 200)
	m.receipt.Status = "paid"
	token := host.want(t, "refresh", in, 200).Invites[0]
	claim := &input{Room: ref, Invite: token}
	clients := []client{browser(t, h), browser(t, h)}
	got := make(chan int, 2)
	var wg sync.WaitGroup
	for _, c := range clients {
		wg.Add(1)
		go func() { defer wg.Done(); got <- c.request("claim", claim).Code }()
	}
	wg.Wait()
	first, second := <-got, <-got
	if (first != 200 || second != 409) && (first != 409 || second != 200) {
		t.Fatal(first, second)
	}
	r := httptest.NewRequest("POST", "https://malten.me/session/start", strings.NewReader(`{"room":"`+ref+`"}`))
	r.Header.Set("Origin", "https://elsewhere.example")
	r.Header.Set("Content-Type", "application/json")
	r.AddCookie(host.cookie)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 || w.Header().Get("Access-Control-Allow-Origin") != "" || m.receipt.Status != "paid" {
		t.Fatal("cross-origin action accepted")
	}
}

func TestPendingReconciliationAndExpiredRestore(t *testing.T) {
	m := new(testMu)
	dir := t.TempDir()
	s, err := Open(dir, m)
	if err != nil {
		t.Fatal(err)
	}
	host := browser(t, s.Handler())
	in := &input{Room: uuid.NewString(), Participants: 1}
	host.want(t, "book", in, 200)
	s.now = func() time.Time { return time.Now().Add(48 * time.Hour) }
	m.receipt.Status = "paid"
	s.reconcile(context.Background())
	if err := s.prune(); err != nil {
		t.Fatal(err)
	}
	v := host.want(t, "state?room="+in.Room, nil, 200)
	if v.Status != "paid" || len(v.Invites) != 0 {
		t.Fatal("lost unstarted solo booking")
	}
	m.receipt.Status, m.receipt.EndsAt = "active", time.Now().Add(-time.Hour).Unix()
	host.want(t, "refresh", in, 200)
	if _, err := Open(dir, m); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, in.Room+".json")); !os.IsNotExist(err) {
		t.Fatal("startup retained expired booking")
	}
}

func TestSaveFailureDoesNotConsumeSeat(t *testing.T) {
	m := new(testMu)
	dir := t.TempDir()
	s, err := Open(dir, m)
	if err != nil {
		t.Fatal(err)
	}
	host, guest := browser(t, s.Handler()), browser(t, s.Handler())
	in := &input{Room: uuid.NewString(), Participants: 2}
	host.want(t, "book", in, 200)
	m.receipt.Status = "paid"
	invite := host.want(t, "refresh", in, 200).Invites[0]
	s.dir = filepath.Join(dir, "missing")
	guest.want(t, "claim", &input{Room: in.Room, Invite: invite}, 502)
	s.dir = dir
	if host.want(t, "state?room="+in.Room, nil, 200).Invites[0] != invite {
		t.Fatal("failed persistence consumed a place")
	}
	guest.want(t, "claim", &input{Room: in.Room, Invite: invite}, 200)
}

func TestLatePaymentRecoversOnlyForOriginalHost(t *testing.T) {
	m := new(testMu)
	s, err := Open(t.TempDir(), m)
	if err != nil {
		t.Fatal(err)
	}
	host, other := browser(t, s.Handler()), browser(t, s.Handler())
	in := &input{Room: uuid.NewString(), Participants: 2}
	host.want(t, "book", in, 200)
	s.now = func() time.Time { return time.Now().Add(48 * time.Hour) }
	m.receipt.Status = "expired"
	s.reconcile(context.Background())
	if err := s.prune(); err != nil {
		t.Fatal(err)
	}
	host.want(t, "state?room="+in.Room, nil, 404)
	m.receipt.Status = "paid"
	returned := &input{Payment: m.receipt.ID}
	other.want(t, "return", returned, 404)
	v := host.want(t, "return", returned, 200)
	if !v.Host || v.Room != in.Room || len(v.Invites) != 1 || v.Status != "paid" {
		t.Fatal("failed to recover paid guest booking", v)
	}
	if host.want(t, "return", returned, 200).Invites[0] != v.Invites[0] {
		t.Fatal("receipt replay reissued a seat")
	}
}

func TestLegacyRoomMessageLinks(t *testing.T) {
	dir := t.TempDir()
	ref := uuid.NewString()
	b := booking{Reference: ref, Host: "host", Participants: 1, Created: time.Now().Unix(), Receipt: Receipt{ID: "paid-1", Status: "active", EndsAt: time.Now().Add(time.Hour).Unix()},
		Messages: []Message{{Author: "Host", Text: "First"}, {Author: "Host", Text: "Second"}}}
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ref+".json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	var parent string
	for range 2 {
		s, err := Open(dir, new(testMu))
		if err != nil {
			t.Fatal(err)
		}
		msg := s.rooms[ref].Messages
		if msg[0].ID == "" || msg[0].ID == msg[1].ID || (parent != "" && msg[0].ID != parent) || msg[0].Parent != "" {
			t.Fatal("invalid links in pre-existing room", msg)
		}
		parent = msg[0].ID
	}
}
