package data

import (
	"path/filepath"
	"sync"
	"time"
)

// SubscriptionsFile manages push_subscriptions.json
type SubscriptionsFile struct {
	mu    sync.RWMutex
	Users map[string]*PushUser
}

// PushUser represents a push subscription
type PushUser struct {
	SessionID    string            `json:"session_id"`
	Subscription *PushSubscription `json:"subscription,omitempty"`
	Lat          float64           `json:"lat"`
	Lon          float64           `json:"lon"`
	LastPing     time.Time         `json:"last_ping"`
	LastPush     time.Time         `json:"last_push"`
	Timezone     *time.Location    `json:"-"`
	PushHistory  []PushHistoryItem `json:"push_history,omitempty"`
	BusNotify    bool              `json:"bus_notify"`
	DailyPushed  map[string]string `json:"daily_pushed,omitempty"`
}

// PushSubscription is the browser push subscription
type PushSubscription struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// PushHistoryItem is a sent push notification
type PushHistoryItem struct {
	Time  time.Time `json:"time"`
	Title string    `json:"title"`
	Body  string    `json:"body"`
}

// Load reads from push_subscriptions.json
func (s *SubscriptionsFile) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var users []*PushUser
	if err := loadJSON(filepath.Join(dataDir, "push_subscriptions.json"), &users); err != nil {
		return err
	}

	for _, u := range users {
		if u.Lon != 0 {
			offsetHours := int(u.Lon / 15)
			u.Timezone = time.FixedZone("local", offsetHours*3600)
		}
		s.Users[u.SessionID] = u
	}
	return nil
}

// Save writes to push_subscriptions.json
func (s *SubscriptionsFile) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]*PushUser, 0, len(s.Users))
	for _, u := range s.Users {
		users = append(users, u)
	}
	return saveJSON(filepath.Join(dataDir, "push_subscriptions.json"), users)
}

// GetUser returns a user by session ID
func (s *SubscriptionsFile) GetUser(sessionID string) *PushUser {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Users[sessionID]
}

// SetUser adds or updates a user
func (s *SubscriptionsFile) SetUser(user *PushUser) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Users[user.SessionID] = user
}

// GetAllUsers returns all users
func (s *SubscriptionsFile) GetAllUsers() []*PushUser {
	s.mu.RLock()
	defer s.mu.RUnlock()
	users := make([]*PushUser, 0, len(s.Users))
	for _, u := range s.Users {
		users = append(users, u)
	}
	return users
}
