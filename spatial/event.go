// Package spatial implements the Malten spatial database.
//
// ARCHITECTURE (read ARCHITECTURE.md and claude.md "The Spacetime Model"):
//
//   events.jsonl  = cosmic ledger (facts about the world only)
//   spatial.json  = materialized quadtree (rebuildable from events)
//   localStorage  = user's private timeline (client-side)
//
// RULE: Never persist private data (channel != "") to events.jsonl.
// Private messages belong in the user's localStorage, not the cosmic ledger.
//
package spatial

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"sync"
	"time"
)

// Event represents a logged event
type Event struct {
	Timestamp time.Time              `json:"ts"`
	Type      string                 `json:"type"`
	ID        string                 `json:"id,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// EventLog handles append-only event logging
type EventLog struct {
	mu   sync.Mutex
	file *os.File
}

// NewEventLog creates a new event log
func NewEventLog(filename string) (*EventLog, error) {
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return &EventLog{file: f}, nil
}

// Log writes an event to the log
func (l *EventLog) Log(eventType, id string, data map[string]interface{}) {
	if l == nil || l.file == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	event := Event{
		Timestamp: time.Now(),
		Type:      eventType,
		ID:        id,
		Data:      data,
	}

	b, err := json.Marshal(event)
	if err != nil {
		return
	}

	l.file.Write(b)
	l.file.Write([]byte("\n"))
}

// LogPanic logs a panic with stack trace
func (l *EventLog) LogPanic(r interface{}) {
	l.Log("panic", "", map[string]interface{}{
		"error": fmt.Sprintf("%v", r),
		"stack": string(debug.Stack()),
	})
}

// Close closes the event log
func (l *EventLog) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	return l.file.Close()
}

// LogMessage logs a PUBLIC stream message event (channel="")
// NOTE: Per spacetime model, only use for public broadcasts (facts about the world)
// Private user messages (channel="@session") should NOT be logged here
// They belong in the user's localStorage (their worldline)
func LogMessage(stream, channel, text, msgID string) {
	// Only log public messages (empty channel)
	if channel != "" {
		return // Private message, don't persist to cosmic ledger
	}
	db := Get()
	if db == nil || db.eventLog == nil {
		return
	}
	db.eventLog.Log("message.created", msgID, map[string]interface{}{
		"stream": stream,
		"text":   text,
	})
}

// MessageEvent represents a PUBLIC message from the event log
type MessageEvent struct {
	ID        string
	Stream    string
	Text      string
	Timestamp time.Time
}

// ReplayMessages reads PUBLIC message events from the log
// Returns messages from the last 24 hours
// NOTE: Only replays public broadcasts (channel=""), not private messages
func ReplayMessages(filename string) ([]MessageEvent, error) {
	f, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No events file yet
		}
		return nil, err
	}
	defer f.Close()

	var messages []MessageEvent
	cutoff := time.Now().Add(-24 * time.Hour)
	scanner := bufio.NewScanner(f)
	
	// Increase scanner buffer for large lines
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}

		if event.Type != "message.created" {
			continue
		}

		if event.Timestamp.Before(cutoff) {
			continue
		}

		if event.Data == nil {
			continue
		}

		msg := MessageEvent{
			ID:        event.ID,
			Timestamp: event.Timestamp,
		}
		if s, ok := event.Data["stream"].(string); ok {
			msg.Stream = s
		}
		if t, ok := event.Data["text"].(string); ok {
			msg.Text = t
		}
		messages = append(messages, msg)
	}

	log.Printf("[events] Replayed %d public messages from last 24h", len(messages))
	return messages, scanner.Err()
}

// RecoverAndLog wraps a function with panic recovery
func RecoverAndLog(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			db := Get()
			if db != nil && db.eventLog != nil {
				db.eventLog.LogPanic(r)
			}
		}
	}()
	fn()
}
