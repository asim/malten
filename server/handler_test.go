package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmptyStreamResponseIsArray(t *testing.T) {
	old := Default
	Default = New()
	defer func() { Default = old }()
	// An observer creates a stream before any messages have been posted.
	observer := NewObserver("empty")
	Default.Observe(observer)
	for _, query := range []string{"stream=missing", "stream=empty", "stream=empty&limit=0"} {
		w := httptest.NewRecorder()
		GetHandler(w, httptest.NewRequest("GET", "/messages?"+query, nil))
		if w.Code != 200 || strings.TrimSpace(w.Body.String()) != "[]" {
			t.Fatalf("%s: status %d, body %s", query, w.Code, w.Body.String())
		}
	}
}
