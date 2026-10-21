package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func testPhoto(t *testing.T) string {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}

func TestPublicModerationGate(t *testing.T) {
	oldDefault, oldModerate := Default, Moderate
	defer func() { Default = oldDefault; Moderate = oldModerate }()
	photo := testPhoto(t)
	for _, tc := range []struct {
		name string
		err  error
		code int
	}{
		{"allowed", nil, 200}, {"rejected", ErrModerationRejected, 422}, {"unavailable", ErrModerationUnavailable, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			Default = New()
			Moderate = func(_ context.Context, m *Message) error {
				if m.Image != photo || m.Text != "" {
					t.Fatal("moderator did not receive photo-only post")
				}
				return tc.err
			}
			req := httptest.NewRequest("POST", "/messages", strings.NewReader(url.Values{"image": {photo}}.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()
			PostHandler(w, req)
			if w.Code != tc.code {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
			if got := len(Default.Events); (got == 1) != (tc.err == nil) {
				t.Fatalf("unexpected broadcast queue length %d", got)
			}
		})
	}
	Default = New()
	Moderate = func(context.Context, *Message) error { return ErrModerationRejected }
	req := httptest.NewRequest("POST", "/commands", strings.NewReader("prompt=blocked"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	PostCommandHandler(w, req)
	if w.Code != 422 || len(Default.Events) != 0 {
		t.Fatal("command bypassed moderation")
	}
}

func TestPhotoValidation(t *testing.T) {
	if err := validatePhoto(testPhoto(t)); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"data:image/svg+xml;base64,PHN2Zz4=", "data:image/png;base64,aGVsbG8=", "data:image/jpeg;base64," + strings.Repeat("A", 700000)} {
		if validatePhoto(value) == nil {
			t.Fatal("accepted invalid photo")
		}
	}
}

func TestInternalObserverPresence(t *testing.T) {
	s := New()
	internal, browser := NewObserver("_"), NewObserver("_")
	internal.Internal = true
	s.Observe(internal)
	s.Observe(browser)
	if s.List()["_"].Observers != 1 {
		t.Fatal("internal listener counted as person")
	}
	close(internal.Kill)
	close(browser.Kill)
	deadline := time.Now().Add(time.Second)
	for s.List()["_"].Observers != 0 {
		if time.Now().After(deadline) {
			t.Fatal("presence not cleaned up")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestPhotoStreamMemoryBound(t *testing.T) {
	s := New()
	for i := 0; i < 20; i++ {
		m := NewMessage("", "photos")
		m.Image = strings.Repeat("a", 700000)
		s.Store(m)
	}
	total := 0
	for _, m := range s.List()["photos"].Messages {
		total += len(m.Image)
	}
	if total > 8<<20 {
		t.Fatalf("photo budget exceeded: %d", total)
	}
}

var _ http.HandlerFunc = PostHandler
