package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func request(t *testing.T, s *Server, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}
func TestIndependentStreamsAndOwnership(t *testing.T) {
	s := New()
	s.stream.moderate = func(context.Context, Post) (bool, error) { return true, nil }
	token := strings.Repeat("ab", 32)
	w := request(t, s, "POST", "/api/posts", `{"stream":"x","text":"hello","agent":"forged","created_at":1}`, token)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var posts []Post
	w = request(t, s, "GET", "/api/posts?stream=x", "", token)
	if err := json.Unmarshal(w.Body.Bytes(), &posts); err != nil {
		t.Fatal(err)
	}
	if len(posts) != 1 || !posts[0].Mine || posts[0].Agent != "" || posts[0].Created == 1 {
		t.Fatalf("%+v", posts)
	}
	id := posts[0].ID
	w = request(t, s, "GET", "/api/posts", "", "")
	if w.Body.String() != "[]\n" {
		t.Fatal("stream leaked", w.Body.String())
	}
	w = request(t, s, "POST", "/api/posts/"+id+"/delete", "", strings.Repeat("cd", 32))
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	w = request(t, s, "POST", "/api/posts/"+id+"/delete", "", token)
	if w.Code != 204 || len(s.stream.posts) != 0 {
		t.Fatal("delete failed")
	}
}
func TestModerationFailsClosed(t *testing.T) {
	for _, err := range []error{nil, errors.New("unavailable")} {
		s := New()
		s.stream.moderate = func(context.Context, Post) (bool, error) { return false, err }
		if e := s.stream.publish(context.Background(), Post{Text: "test"}); e == nil || len(s.stream.posts) != 0 {
			t.Fatal("published rejected post")
		}
	}
}
func TestExpiryCapacityAndReport(t *testing.T) {
	s := New()
	s.stream.moderate = func(context.Context, Post) (bool, error) { return true, nil }
	for i := 0; i < capacity+1; i++ {
		if err := s.stream.publish(context.Background(), Post{Text: "hello"}); err != nil {
			t.Fatal(err)
		}
	}
	if len(s.stream.posts) != capacity {
		t.Fatal("unbounded")
	}
	id := s.stream.posts[0].ID
	w := request(t, s, "POST", "/api/posts/"+id+"/report", "", strings.Repeat("ab", 32))
	if w.Code != 204 || !s.stream.posts[0].hidden {
		t.Fatal("not quarantined")
	}
	w = request(t, s, "GET", "/api/posts", "", "")
	if strings.Contains(w.Body.String(), id) {
		t.Fatal("quarantined post visible")
	}
	s.stream.prune(time.Now().Add(lifetime))
	if len(s.stream.posts) != 0 {
		t.Fatal("not expired")
	}
}
func TestPhotoMetadataRemovedAndExpires(t *testing.T) {
	var original bytes.Buffer
	if err := jpeg.Encode(&original, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); err != nil {
		t.Fatal(err)
	}
	// JPEG APP1 metadata can contain EXIF/GPS. Ensure it is never stored or moderated.
	raw := original.Bytes()
	metadata := []byte("GPS-SECRET")
	withMetadata := append([]byte{0xff, 0xd8, 0xff, 0xe1, 0, byte(len(metadata) + 2)}, metadata...)
	withMetadata = append(withMetadata, raw[2:]...)
	s := New()
	s.stream.moderate = func(_ context.Context, p Post) (bool, error) {
		data, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(p.Photo, "data:image/jpeg;base64,"))
		if bytes.Contains(data, metadata) {
			t.Fatal("metadata reached moderation")
		}
		return true, nil
	}
	err := s.stream.publish(context.Background(), Post{Photo: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(withMetadata)})
	if err != nil {
		t.Fatal(err)
	}
	id := s.stream.posts[0].ID
	w := request(t, s, "GET", "/api/posts/"+id+"/photo", "", "")
	if w.Code != 200 || bytes.Contains(w.Body.Bytes(), metadata) {
		t.Fatal("metadata leaked")
	}
	s.stream.posts[0].hidden = true
	w = request(t, s, "GET", "/api/posts/"+id+"/photo", "", "")
	if w.Code != 404 {
		t.Fatal("quarantined image accessible")
	}
	s.stream.prune(time.Now().Add(lifetime))
	w = request(t, s, "GET", "/api/posts/"+id+"/photo", "", "")
	if w.Code != 404 {
		t.Fatal("expired image accessible")
	}
	if validPost(Post{Photo: "data:video/mp4;base64,AAAA"}) {
		t.Fatal("video accepted")
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestSonnetResponseValidation(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	old := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = old })
	for _, answer := range []string{"ALLOW", "BLOCK", "ALLOW maybe"} {
		http.DefaultClient = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			if req["model"] != "claude-sonnet-5" || r.Header.Get("x-api-key") != "test-key" {
				t.Fatal("incorrect model or credentials")
			}
			data, _ := json.Marshal(map[string]any{"content": []any{map[string]string{"type": "text", "text": answer}}})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data))}, nil
		})}
		ok, err := moderate(context.Background(), Post{Text: "hello"})
		if ok != (answer == "ALLOW") || (answer == "ALLOW maybe" && err == nil) {
			t.Fatal("invalid decision", answer, ok, err)
		}
	}
}

func TestReportReviewOutcomes(t *testing.T) {
	for _,decision := range []string{"allow", "block", "unavailable"} {
		t.Run(decision, func(t *testing.T) {
			s := New()
			s.stream.posts = []Post{{ID: "reported", Text: "reflection", Photo: "image", Created: time.Now().UnixMilli(), hidden: true}}
			s.stream.moderate = func(_ context.Context, p Post) (bool, error) {
				if !p.hidden {
					t.Fatal("review did not receive report context")
				}
				if decision == "unavailable" {
					return false, errors.New("offline")
				}
				return decision == "allow", nil
			}
			s.review(context.Background())
			switch decision {
			case "allow":
				if len(s.stream.posts) != 1 || s.stream.posts[0].hidden || !s.stream.posts[0].reviewed {
					t.Fatal("approved capture not restored")
				}
			case "block":
				if len(s.stream.posts) != 0 {
					t.Fatal("rejected capture and media not removed")
				}
			case "unavailable":
				if len(s.stream.posts) != 1 || !s.stream.posts[0].hidden {
					t.Fatal("failed review must remain quarantined")
				}
			}
		})
	}
}

func TestRateLimitDoesNotTrustPublicForwardedHeader(t *testing.T) {
	s := New()
	r := httptest.NewRequest("POST", "/api/posts", nil)
	r.RemoteAddr = "198.51.100.1:1234"
	r.Header.Set("X-Real-IP", "203.0.113.1")
	if !s.stream.allow(r) {
		t.Fatal("first request denied")
	}
	r.Header.Set("X-Real-IP", "203.0.113.2")
	if s.stream.allow(r) {
		t.Fatal("spoofed header bypassed rate limit")
	}
}
