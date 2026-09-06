package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Records are private agent stream entries, never served by the public API.
type Record struct {
	ID      string
	At      time.Time
	Kind    string
	Data    json.RawMessage `json:",omitempty"`
	Summary string          `json:",omitempty"`
	Action  *Action         `json:",omitempty"`
	Status  string          `json:",omitempty"`
}
type Memory struct {
	sync.Mutex
	path        string
	streams     map[string][]Record
	cycleErrors map[string]string
}

func Key(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }
func Open(path string) (*Memory, error) {
	m := &Memory{path: path, streams: map[string][]Record{}}
	if path == "" {
		return nil, errors.New("empty agent storage path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err == nil {
		defer f.Close()
		d := json.NewDecoder(io.LimitReader(f, 64<<20))
		if err = d.Decode(&m.streams); err != nil {
			return nil, err
		}
		if err = d.Decode(new(any)); err != io.EOF {
			return nil, errors.New("invalid agent stream data")
		}
		if m.streams == nil || len(m.streams) > 4 {
			return nil, errors.New("invalid agent streams")
		}
		for name, records := range m.streams {
			if !known(name) || len(records) > 512 {
				return nil, errors.New("invalid agent stream")
			}
			for _, r := range records {
				if r.ID == "" || r.At.IsZero() || r.At.After(time.Now()) || len(r.Data) > 128<<10 || (r.Status == "pending" && r.Action == nil) || len([]rune(r.Summary)) > 3000 {
					return nil, errors.New("invalid agent record")
				}
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	m.prune(time.Now())
	return m, m.save()
}
func known(s string) bool { return s == "reminder" || s == "aslam" || s == "news" || s == "nature" }
func (m *Memory) prune(now time.Time) {
	for name, rs := range m.streams {
		var keep []Record
		for _, r := range rs {
			if now.Sub(r.At) < 24*time.Hour {
				keep = append(keep, r)
			}
		}
		if len(keep) > 512 {
			keep = keep[len(keep)-512:]
		}
		size := 0
		start := len(keep)
		for i := len(keep) - 1; i >= 0; i-- {
			raw, _ := json.Marshal(keep[i])
			if size+len(raw) > 8<<20 {
				break
			}
			size += len(raw)
			start = i
		}
		m.streams[name] = keep[start:]
	}
}
func (m *Memory) Read(name string, now time.Time) []Record {
	m.Lock()
	defer m.Unlock()
	var out []Record
	for _, r := range m.streams[name] {
		if now.Sub(r.At) < 24*time.Hour {
			r.Data = append(json.RawMessage(nil), r.Data...)
			if r.Action != nil {
				a := *r.Action
				r.Action = &a
			}
			out = append(out, r)
		}
	}
	return out
}
func (m *Memory) Write(name string, r Record) error {
	m.Lock()
	defer m.Unlock()
	if !known(name) || r.ID == "" || len(r.Data) > 128<<10 {
		return errors.New("invalid agent record")
	}
	// Copy before mutation so a failed write cannot acknowledge unsaved progress.
	before := m.streams
	next := map[string][]Record{}
	for k, v := range before {
		next[k] = append([]Record(nil), v...)
	}
	m.streams = next
	m.prune(time.Now())
	found := false
	for i, old := range m.streams[name] {
		if old.ID == r.ID {
			r.At = old.At
			m.streams[name][i] = r
			found = true
			break
		}
	}
	if !found {
		m.streams[name] = append(m.streams[name], r)
	}
	m.prune(time.Now())
	if err := m.save(); err != nil {
		m.streams = before
		return err
	}
	return nil
}
func (m *Memory) save() error {
	f, err := os.CreateTemp(filepath.Dir(m.path), ".agents-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = json.NewEncoder(f).Encode(m.streams); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), m.path); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(m.path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// Expire purges disk context even during a source outage.
func (m *Memory) Expire(now time.Time) error {
	m.Lock()
	defer m.Unlock()
	before := map[string][]Record{}
	count := 0
	for k, v := range m.streams {
		before[k] = v
		count += len(v)
	}
	m.prune(now)
	after := 0
	for _, v := range m.streams {
		after += len(v)
	}
	if count == after {
		return nil
	}
	if err := m.save(); err != nil {
		m.streams = before
		return err
	}
	return nil
}

// Status reports pipeline health without exposing source data or summaries.
type Status struct {
	LastSource   time.Time `json:"last_source,omitempty"`
	LastDecision time.Time `json:"last_decision,omitempty"`
	LastAction   string    `json:"last_action,omitempty"`
	LastError    string    `json:"last_error,omitempty"`
}

func (m *Memory) RecordCycle(name string, err error) {
	m.Lock()
	defer m.Unlock()
	if m.cycleErrors == nil {
		m.cycleErrors = map[string]string{}
	}
	message := ""
	if err != nil {
		switch err.Error() {
		case "unsupported action", "summary too long", "invalid agent decision", "incomplete agent decision", "agent model unavailable", "weather source unavailable", "source unavailable", "source request failed", "invalid or oversized source", "moderation unavailable", "busy", "stale or incomplete weather", "unexpected weather timezone":
			message = err.Error()
		default:
			message = "cycle failed"
		}
	}
	m.cycleErrors[name] = message
}
func (m *Memory) Status() map[string]Status {
	m.Lock()
	defer m.Unlock()
	out := map[string]Status{}
	for _, name := range []string{"reminder", "aslam", "news", "nature"} {
		records := m.streams[name]
		state := Status{LastError: m.cycleErrors[name]}
		for _, r := range records {
			if time.Since(r.At) >= 24*time.Hour {
				continue
			}
			if r.Kind == "source" && r.At.After(state.LastSource) {
				state.LastSource = r.At
			}
			if r.Kind == "decision" && r.At.After(state.LastDecision) {
				state.LastDecision = r.At
			}
			if r.Action != nil {
				state.LastAction = r.Status
			}
		}
		out[name] = state
	}
	return out
}
