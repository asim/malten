package server

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"malten.ai/data"
)

// Type aliases - use data package types
type PushSubscription = data.PushSubscription
type PushUser = data.PushUser
type PushHistoryItem = data.PushHistoryItem

// PushManager handles web push notifications
type PushManager struct {
	mu           sync.RWMutex
	users        map[string]*PushUser // sessionID -> user
	vapidPublic  string
	vapidPrivate string
	subject      string
}

var pushManager *PushManager
var pushOnce sync.Once

// PushNotification represents a push message
type PushNotification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Icon  string `json:"icon,omitempty"`
	Image string `json:"image,omitempty"`
	Tag   string `json:"tag,omitempty"`
	Data  any    `json:"data,omitempty"`
}

// Callback for building morning context notification (set by main.go)
var buildMorningContext func(lat, lon float64) *PushNotification

// SetMorningContextBuilder sets the callback for morning notifications
func SetMorningContextBuilder(cb func(lat, lon float64) *PushNotification) {
	buildMorningContext = cb
}

// GetPushManager returns the singleton push manager
func GetPushManager() *PushManager {
	pushOnce.Do(func() {
		pushManager = &PushManager{
			users:        make(map[string]*PushUser),
			vapidPublic:  os.Getenv("VAPID_PUBLIC_KEY"),
			vapidPrivate: os.Getenv("VAPID_PRIVATE_KEY"),
			subject:      "mailto:push@malten.ai",
		}
		pushManager.load()
		if pushManager.vapidPublic != "" {
			go pushManager.backgroundLoop()
			log.Printf("[push] Enabled, %d subscriptions", len(pushManager.users))
		} else {
			log.Printf("[push] VAPID keys not configured, disabled")
		}
	})
	return pushManager
}

// load copies from data.Subscriptions to local map
func (pm *PushManager) load() {
	for _, u := range data.Subscriptions().GetAllUsers() {
		pm.users[u.SessionID] = u
	}
}

// save syncs to data.Subscriptions and saves
func (pm *PushManager) save() {
	pm.mu.RLock()
	for _, u := range pm.users {
		data.Subscriptions().SetUser(u)
	}
	pm.mu.RUnlock()
	data.SaveAll()
}

// Subscribe adds or updates a push subscription for a session
func (pm *PushManager) Subscribe(sessionID string, sub *PushSubscription) {
	pm.mu.Lock()
	user, exists := pm.users[sessionID]
	if !exists {
		user = &PushUser{
			SessionID: sessionID,
		}
		pm.users[sessionID] = user
	}
	user.Subscription = sub
	pm.mu.Unlock()
	pm.save()
	log.Printf("[push] Subscribed: %s", sessionID[:8])
}

// Unsubscribe removes a push subscription
func (pm *PushManager) Unsubscribe(sessionID string) {
	pm.mu.Lock()
	delete(pm.users, sessionID)
	pm.mu.Unlock()
	pm.save()
	log.Printf("[push] Unsubscribed: %s", sessionID[:8])
}

// UpdateLocation updates user's last known location and timezone
func (pm *PushManager) UpdateLocation(sessionID string, lat, lon float64) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	user, exists := pm.users[sessionID]
	if !exists {
		return
	}
	user.Lat = lat
	user.Lon = lon
	user.LastPing = time.Now()

	// Estimate timezone from longitude (rough: 15° per hour)
	offsetHours := int(lon / 15)
	user.Timezone = time.FixedZone("local", offsetHours*3600)
}

// backgroundLoop runs once per minute, checks for morning notification
func (pm *PushManager) backgroundLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		pm.checkMorningNotification()
	}
}

// checkMorningNotification sends context at 7am local time
func (pm *PushManager) checkMorningNotification() {
	if buildMorningContext == nil {
		return
	}

	pm.mu.RLock()
	users := make([]*PushUser, 0, len(pm.users))
	for _, u := range pm.users {
		if u.Subscription != nil && u.Lat != 0 {
			users = append(users, u)
		}
	}
	pm.mu.RUnlock()

	for _, user := range users {
		// Get user's local time
		now := time.Now()
		if user.Timezone != nil {
			now = now.In(user.Timezone)
		}
		hour, minute := now.Hour(), now.Minute()

		// 7:00-7:05am window - log when close to trigger time for debugging
		if hour == 6 && minute >= 55 {
			log.Printf("[push] User %s local time: %02d:%02d (approaching 7am window)", user.SessionID[:8], hour, minute)
		}
		if hour == 7 && minute < 5 {
			today := now.Format("2006-01-02")
			
			// Check if already sent today
			pm.mu.RLock()
			alreadySent := user.DailyPushed != nil && user.DailyPushed["morning"] == today
			pm.mu.RUnlock()
			
			if alreadySent {
				continue
			}

			// Build and send notification
			notification := buildMorningContext(user.Lat, user.Lon)
			if notification == nil {
				continue
			}

			if pm.sendPush(user, notification) {
				// Mark as sent
				pm.mu.Lock()
				if user.DailyPushed == nil {
					user.DailyPushed = make(map[string]string)
				}
				user.DailyPushed["morning"] = today
				pm.mu.Unlock()
				pm.save()
				log.Printf("[push] Morning context sent to %s", user.SessionID[:8])
			}
		}
	}
}

// sendPush sends a push notification to a user and stores it in history
func (pm *PushManager) sendPush(user *PushUser, notification *PushNotification) bool {
	if user.Subscription == nil {
		return false
	}

	payload, _ := json.Marshal(notification)

	resp, err := webpush.SendNotification(payload, &webpush.Subscription{
		Endpoint: user.Subscription.Endpoint,
		Keys: webpush.Keys{
			P256dh: user.Subscription.Keys.P256dh,
			Auth:   user.Subscription.Keys.Auth,
		},
	}, &webpush.Options{
		VAPIDPublicKey:  pm.vapidPublic,
		VAPIDPrivateKey: pm.vapidPrivate,
		Subscriber:      pm.subject,
		TTL:             3600,
	})

	if err != nil {
		log.Printf("[push] Error: %v", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusGone {
		// Subscription expired
		pm.Unsubscribe(user.SessionID)
		return false
	}

	if resp.StatusCode < 400 {
		// Store in history for retrieval when app opens
		pm.mu.Lock()
		user.PushHistory = append(user.PushHistory, PushHistoryItem{
			Time:  time.Now(),
			Title: notification.Title,
			Body:  notification.Body,
			Image: notification.Image,
		})
		// Keep only last 10 items
		if len(user.PushHistory) > 10 {
			user.PushHistory = user.PushHistory[len(user.PushHistory)-10:]
		}
		log.Printf("[push] Stored notification in history for %s, now have %d items", user.SessionID[:8], len(user.PushHistory))
		pm.mu.Unlock()
		return true
	}

	return false
}

// GetVAPIDPublicKey returns the public key for client subscription
func (pm *PushManager) GetVAPIDPublicKey() string {
	return pm.vapidPublic
}

// HTTP Handlers

func HandleSubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID := getSessionToken(w, r)
	if sessionID == "" {
		JsonError(w, "No session", http.StatusUnauthorized)
		return
	}

	var sub PushSubscription
	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
		JsonError(w, "Invalid subscription", http.StatusBadRequest)
		return
	}

	GetPushManager().Subscribe(sessionID, &sub)
	w.WriteHeader(http.StatusOK)
}

func HandleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID := getSessionToken(w, r)
	if sessionID == "" {
		JsonError(w, "No session", http.StatusUnauthorized)
		return
	}

	GetPushManager().Unsubscribe(sessionID)
	w.WriteHeader(http.StatusOK)
}

func HandleVAPIDKey(w http.ResponseWriter, r *http.Request) {
	pm := GetPushManager()
	if pm.vapidPublic == "" {
		JsonError(w, "Push not configured", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"key": pm.vapidPublic})
}

func HandlePushHistory(w http.ResponseWriter, r *http.Request) {
	sessionID := getSessionToken(w, r)
	if sessionID == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"history": []PushHistoryItem{}})
		return
	}

	pm := GetPushManager()
	pm.mu.Lock()
	user, exists := pm.users[sessionID]
	var history []PushHistoryItem
	if exists && len(user.PushHistory) > 0 {
		history = user.PushHistory
		user.PushHistory = nil // Clear after retrieval
	}
	pm.mu.Unlock()

	if len(history) > 0 {
		pm.save() // Persist the cleared history
	}

	w.Header().Set("Content-Type", "application/json")
	if history == nil {
		history = []PushHistoryItem{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"history": history})
}

// HandleTestMorningPush manually triggers the morning push for testing
func HandleTestMorningPush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pm := GetPushManager()
	if buildMorningContext == nil {
		JsonError(w, "Morning context builder not set", http.StatusServiceUnavailable)
		return
	}

	pm.mu.RLock()
	var user *PushUser
	for _, u := range pm.users {
		if u.Subscription != nil && u.Lat != 0 {
			user = u
			break
		}
	}
	pm.mu.RUnlock()

	if user == nil {
		JsonError(w, "No users with subscription and location", http.StatusNotFound)
		return
	}

	notification := buildMorningContext(user.Lat, user.Lon)
	if notification == nil {
		JsonError(w, "Failed to build notification", http.StatusInternalServerError)
		return
	}

	// Show notification even if send fails (subscription may be stale)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      pm.sendPush(user, notification),
		"title":        notification.Title,
		"body":         notification.Body,
		"image":        notification.Image,
		"user_session": user.SessionID[:8],
	})
	log.Printf("[push] Test morning push attempted for %s", user.SessionID[:8])
}
