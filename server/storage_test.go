package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/jpeg"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRestartPreservesCaptureAndModeration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stream.json")
	s, err := Open(path)
	if err != nil { t.Fatal(err) }
	token := strings.Repeat("ab", 32)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	identity := owner(req)
	var photo bytes.Buffer
	if err := jpeg.Encode(&photo, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); err != nil { t.Fatal(err) }
	p := Post{ID: "one", Stream: "park", Text: "reflection", Photo: "data:image/jpeg;base64,"+base64.StdEncoding.EncodeToString(photo.Bytes()), Created: time.Now().Add(-time.Hour).UnixMilli(), owner: identity, hidden: true, reviewed: true}
	s.stream.posts = []Post{p}
	if err := s.stream.save(); err != nil { t.Fatal(err) }
	restarted, err := Open(path)
	if err != nil { t.Fatal(err) }
	got := restarted.stream.posts[0]
	if got != p { t.Fatal("capture, original expiry or moderation state changed") }
	w := request(t, restarted, "GET", "/api/posts?stream=park", "", token)
	if strings.Contains(w.Body.String(), p.ID) { t.Fatal("quarantined post exposed") }
	w = request(t, restarted, "POST", "/api/posts/one/delete", "", token)
	if w.Code != 204 { t.Fatal("owner cannot delete after restart", w.Code) }
	again, err := Open(path)
	if err != nil || len(again.stream.posts) != 0 { t.Fatal("deleted post returned", err) }
	raw, _ := os.ReadFile(path)
	if bytes.Contains(raw, []byte("data:image")) { t.Fatal("deleted media retained") }
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 { t.Fatal("snapshot is not private") }
}

func TestRestartPurgesExpiredMedia(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stream.json")
	s, err := Open(path)
	if err != nil { t.Fatal(err) }
	s.stream.posts = []Post{{ID: "expired", Text: "old", Photo: "expired-media", Created: time.Now().Add(-lifetime).UnixMilli()}}
	if err := s.stream.save(); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), ".stream.json-abandoned"), []byte("partial media"), 0600); err != nil { t.Fatal(err) }
	restarted, err := Open(path)
	if err != nil || len(restarted.stream.posts) != 0 { t.Fatal("expired post restored", err) }
	raw, _ := os.ReadFile(path)
	if bytes.Contains(raw, []byte("expired-media")) { t.Fatal("expired media retained on disk") }
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".stream.json-*"))
	if len(matches) != 0 { t.Fatal("abandoned snapshot retained") }
}

func TestStorageFailureDoesNotAcknowledgeCapture(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stream.json")
	s, err := Open(path)
	if err != nil { t.Fatal(err) }
	s.stream.moderate = func(context.Context, Post) (bool, error) { return true, nil }
	s.stream.path = filepath.Join(t.TempDir(), "missing", "stream.json")
	if err := s.stream.publish(context.Background(), Post{Text: "draft"}); err == nil { t.Fatal("uncommitted capture acknowledged") }
	if len(s.stream.posts) != 0 { t.Fatal("failed capture exposed") }
	restarted, err := Open(path)
	if err != nil || len(restarted.stream.posts) != 0 { t.Fatal("failed capture restored", err) }
}

func TestCorruptSnapshotIsNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stream.json")
	broken := []byte("{incomplete")
	if err := os.WriteFile(path, broken, 0600); err != nil { t.Fatal(err) }
	if _, err := Open(path); err == nil { t.Fatal("silently ignored corrupt snapshot") }
	raw, _ := os.ReadFile(path)
	if !bytes.Equal(raw, broken) { t.Fatal("corrupt snapshot overwritten") }
}

func TestAcceptedPostAndReviewSurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stream.json")
	s, err := Open(path)
	if err != nil { t.Fatal(err) }
	s.stream.moderate = func(context.Context, Post) (bool, error) { return true, nil }
	w := request(t, s, "POST", "/api/posts", `{"text":"hello","stream":"x"}`, strings.Repeat("ab",32))
	if w.Code != 201 { t.Fatal(w.Code) }
	restarted, err := Open(path)
	if err != nil || len(restarted.stream.posts) != 1 { t.Fatal("acknowledged capture lost", err) }
	id := restarted.stream.posts[0].ID
	w = request(t, restarted, "POST", "/api/posts/"+id+"/report", "", strings.Repeat("cd",32))
	if w.Code != 204 { t.Fatal(w.Code) }
	restarted, err = Open(path)
	if err != nil || !restarted.stream.posts[0].hidden { t.Fatal("quarantine lost", err) }
	restarted.stream.moderate = func(context.Context, Post) (bool, error) { return false, nil }
	restarted.review(context.Background())
	restarted, err = Open(path)
	if err != nil || len(restarted.stream.posts) != 0 { t.Fatal("rejected capture restored", err) }
	var saved snapshot
	raw, _ := os.ReadFile(path)
	if err := json.Unmarshal(raw, &saved); err != nil || len(saved.Posts) != 0 { t.Fatal("rejected data on disk", err) }
}

func TestAgentDoesNotRepeatLiveCaptureAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stream.json")
	s, err := Open(path)
	if err != nil { t.Fatal(err) }
	allow := func(context.Context, Post) (bool, error) { return true, nil }
	s.stream.moderate = allow
	if err := s.PublishAgent(context.Background(), "reminder", "reflection", "Reminder"); err != nil { t.Fatal(err) }
	created := s.stream.posts[0].Created
	s, err = Open(path)
	if err != nil { t.Fatal(err) }
	s.stream.moderate = allow
	if err := s.PublishAgent(context.Background(), "reminder", "reflection", "Reminder"); err != nil { t.Fatal(err) }
	if len(s.stream.posts) != 1 || s.stream.posts[0].Created != created { t.Fatal("agent repeated or renewed live capture") }
}
