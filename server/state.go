package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// Restore runs before the server accepts traffic. Both startup and the running
// server use the same expiry rules and original nanosecond timestamps.
func (s *Server) Restore(path string) error {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	s.statePath = path
	now := time.Now().UnixNano()
	if err == nil {
		var streams map[string]*Stream
		if err := json.Unmarshal(data, &streams); err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		for name, st := range streams {
			if st == nil || st.Id != name || st.TTL <= 0 {
				return fmt.Errorf("invalid stream in %s", path)
			}
			for _, m := range st.Messages {
				if m == nil || m.Stream != name {
					return fmt.Errorf("invalid message in %s", path)
				}
			}
			st.Observers = 0
			if now-st.Updated >= int64(StreamTTL) {
				s.dirty = true
				continue
			}
			live := st.Messages[:0]
			for _, m := range st.Messages {
				if now-m.Created < int64(StreamTTL) {
					live = append(live, m)
				} else {
					s.dirty = true
				}
			}
			clear(st.Messages[len(live):])
			st.Messages = live
			s.streams[name] = st
			if st.TTL > int64(StreamTTL) {
				s.dirty = true
			}
		}
	}
	// A process killed during a write may have left an uncommitted snapshot.
	if err := os.Remove(path + ".tmp"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return s.Checkpoint()
}

func (s *Server) expire(now int64) {
	for id, meta := range s.metadata {
		if now-meta.Created >= int64(StreamTTL) {
			delete(s.metadata, id)
		}
	}
	for name, st := range s.streams {
		if st.Thread != "" && now >= st.ThreadExpires {
			st.Thread = ""
			st.ThreadExpires = 0
			s.dirty = true
		}
		if now-st.Updated >= st.TTL {
			delete(s.streams, name)
			s.dirty = true
			continue
		}
		live := st.Messages[:0]
		for _, m := range st.Messages {
			if now-m.Created < st.TTL {
				live = append(live, m)
			}
		}
		if len(live) != len(st.Messages) {
			clear(st.Messages[len(live):])
			st.Messages = live
			s.dirty = true
		}
	}
}

// Checkpoint atomically replaces the last snapshot only when stream state changes.
func (s *Server) Checkpoint() error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	now := time.Now().UnixNano()
	s.expire(now)
	if s.nextDiskExpiry != 0 && s.nextDiskExpiry <= now {
		s.dirty = true
	}
	if !s.dirty || s.statePath == "" {
		return nil
	}
	snapshot := make(map[string]*Stream, len(s.streams))
	var nextExpiry int64
	for name, st := range s.streams {
		copy := *st
		// Custom streams can live longer in memory. Recovery covers only the last day.
		if st.TTL > int64(StreamTTL) {
			deadline := st.Updated + int64(StreamTTL)
			if deadline <= now {
				continue
			}
			copy.Messages = nil
			for _, m := range st.Messages {
				expiry := m.Created + int64(StreamTTL)
				if expiry > now {
					copy.Messages = append(copy.Messages, m)
					deadline = min(deadline, expiry)
				}
			}
			if nextExpiry == 0 || deadline < nextExpiry {
				nextExpiry = deadline
			}
		}
		copy.Observers = 0
		snapshot[name] = &copy
	}
	if err := saveJSON(s.statePath, snapshot); err != nil {
		return err
	}
	s.nextDiskExpiry = nextExpiry
	s.dirty = false
	return nil
}

// The caller serializes writes to path.
func saveJSON(path string, data any) error {
	tmp := path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	err = json.NewEncoder(file).Encode(data)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(tmp, path)
}
