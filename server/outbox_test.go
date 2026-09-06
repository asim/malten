package server

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfflineRetrySurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stream.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	allow := func(context.Context, Post) (bool, error) { calls++; return true, nil }
	s.stream.moderate = allow
	post := func(token string) int {
		r := httptest.NewRequest("POST", "/api/posts", strings.NewReader(`{"stream":"park","text":"a moment"}`))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", "saved-capture")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w.Code
	}
	author := strings.Repeat("ab", 32)
	if post(author) != 201 {
		t.Fatal("initial send failed")
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.stream.moderate = allow
	if post(author) != 201 || len(s.stream.posts) != 1 || calls != 1 {
		t.Fatal("retry repeated saved post or moderation")
	}
	if post(strings.Repeat("cd", 32)) != 201 || len(s.stream.posts) != 2 {
		t.Fatal("send identifiers must be scoped to their author")
	}
}

func TestModerationRejectionIsNotRetryable(t *testing.T) {
	s := New()
	s.stream.moderate = func(context.Context, Post) (bool, error) { return false, nil }
	w := request(t, s, "POST", "/api/posts", `{"text":"blocked"}`, strings.Repeat("ab", 32))
	if w.Code != 422 || len(s.stream.posts) != 0 {
		t.Fatalf("rejection: %d", w.Code)
	}
}
