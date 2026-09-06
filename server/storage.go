package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Disk-only fields never appear in public API responses.
type storedPost struct {
	Post
	Key      string
	Owner    string
	Hidden   bool
	Reviewed bool
}

type snapshot struct {
	Version int
	Posts   []storedPost
}

// Open restores the stream before accepting requests. Invalid or inaccessible
// storage is an error, never a reason to silently start an empty stream.
func Open(path string) (*Server, error) {
	if path == "" {
		return nil, errors.New("empty stream storage path")
	}
	s := New()
	b := s.stream
	b.path = path
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err == nil {
		defer file.Close()
		var saved snapshot
		decoder := json.NewDecoder(io.LimitReader(file, 300*1024*1024))
		if err := decoder.Decode(&saved); err != nil {
			return nil, fmt.Errorf("read stream: %w", err)
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return nil, errors.New("invalid trailing stream data")
		}
		if saved.Version != 1 || len(saved.Posts) > capacity {
			return nil, errors.New("invalid stream snapshot")
		}
		now := time.Now()
		for _, record := range saved.Posts {
			p := record.Post
			if now.Sub(time.UnixMilli(p.Created)) >= lifetime {
				continue
			}
			if p.Created > now.UnixMilli() || p.ID == "" || !validPost(p) {
				return nil, errors.New("invalid stored capture")
			}
			p.owner, p.hidden, p.reviewed, p.key = record.Owner, record.Hidden, record.Reviewed, record.Key
			p.Mine = false
			b.posts = append(b.posts, p)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	// Remove incomplete snapshots left by a killed process. The committed file
	// remains authoritative, and is replaced below to purge expired media.
	stale, err := filepath.Glob(filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+"-*"))
	if err != nil {
		return nil, err
	}
	for _, name := range stale {
		if err := os.Remove(name); err != nil {
			return nil, err
		}
	}
	if err := b.save(); err != nil {
		return nil, err
	}
	return s, nil
}

// save runs under the store lock (or during startup). Successful mutations are
// saved before acknowledgement, rather than relying on a shutdown hook.
func (b *streamStore) save() error {
	if b.path == "" {
		return nil // Explicitly in-memory servers used by unit tests.
	}
	saved := snapshot{Version: 1, Posts: make([]storedPost, 0, len(b.posts))}
	for _, p := range b.posts {
		p.Mine = false
		saved.Posts = append(saved.Posts, storedPost{Post: p, Key: p.key, Owner: p.owner, Hidden: p.hidden, Reviewed: p.reviewed})
	}
	dir := filepath.Dir(b.path)
	file, err := os.CreateTemp(dir, "."+filepath.Base(b.path)+"-")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	defer file.Close()
	if err := json.NewEncoder(file).Encode(saved); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, b.path); err != nil {
		return err
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
