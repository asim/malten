package server

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"net/http"
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
