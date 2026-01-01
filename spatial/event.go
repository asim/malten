package spatial

import (
	"encoding/json"
	"fmt"
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
