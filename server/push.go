package server

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

const pushFile = "push_subscriptions.json"

// PushSubscription represents a user's push subscription
type PushSubscription struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// PushUser tracks a user's push subscription and state
type PushUser struct {
	SessionID      string            `json:"session_id"`
	Subscription   *PushSubscription `json:"subscription"`
	Lat            float64           `json:"lat"`
	Lon            float64           `json:"lon"`
	LastPing       time.Time         `json:"last_ping"`
	LastPush       time.Time         `json:"last_push"`
	Timezone       *time.Location    `json:"-"`                      // Not persisted, recalculated from lon
	PushHistory    []PushHistoryItem `json:"push_history,omitempty"` // Recent push notifications
	BusNotify      bool              `json:"bus_notify"`             // Whether to send bus push notifications (default: false)
	DailyPushed    map[string]string `json:"daily_pushed,omitempty"` // notifyType -> date string (YYYY-MM-DD)
	PushedContent  map[string]int64  `json:"pushed_content,omitempty"` // content hash -> unix timestamp
}

// PushHistoryItem represents a sent push notification
type PushHistoryItem struct {
	Time  time.Time `json:"time"`
	Title string    `json:"title"`
	Body  string    `json:"body"`
}

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
			log.Printf("[push] Push notifications enabled, %d subscriptions loaded", len(pushManager.users))
		} else {
			log.Printf("[push] VAPID keys not configured, push disabled")
		}
	})
	return pushManager
}

// load reads subscriptions from disk
func (pm *PushManager) load() {
	data, err := os.ReadFile(pushFile)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[push] Failed to load subscriptions: %v", err)
		}
		return
	}

	var users []*PushUser
	if err := json.Unmarshal(data, &users); err != nil {
		log.Printf("[push] Failed to parse subscriptions: %v", err)
		return
	}

	for _, u := range users {
		// Recalculate timezone from longitude
		if u.Lon != 0 {
			offsetHours := int(u.Lon / 15)
			u.Timezone = time.FixedZone("local", offsetHours*3600)
		}
		pm.users[u.SessionID] = u
	}
}

// save writes subscriptions to disk
func (pm *PushManager) save() {
	pm.mu.RLock()
	users := make([]*PushUser, 0, len(pm.users))
	for _, u := range pm.users {
		users = append(users, u)
	}
	pm.mu.RUnlock()

	data, err := json.MarshalIndent(users, "", "  ")
	if err != nil {
		log.Printf("[push] Failed to marshal subscriptions: %v", err)
		return
	}

	if err := os.WriteFile(pushFile, data, 0644); err != nil {
		log.Printf("[push] Failed to save subscriptions: %v", err)
	}
}

// Subscribe adds or updates a push subscription for a session
func (pm *PushManager) Subscribe(sessionID string, sub *PushSubscription) {
	pm.mu.Lock()
	user, exists := pm.users[sessionID]
	if !exists {
		user = &PushUser{
			SessionID: sessionID,
			Timezone:  time.UTC, // Default, updated on ping
		}
		pm.users[sessionID] = user
	}
	user.Subscription = sub
	pm.mu.Unlock()

	pm.save()
	log.Printf("[push] Subscription added for session %s", sessionID[:8])
}

// Unsubscribe removes a push subscription
func (pm *PushManager) Unsubscribe(sessionID string) {
	pm.mu.Lock()
	delete(pm.users, sessionID)
	pm.mu.Unlock()

	pm.save()
	log.Printf("[push] Subscription removed for session %s", sessionID[:8])
}

// UpdateLocation updates user's last known location
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

// isQuietHours checks if it's between 10pm and 7am in user's timezone
func (pm *PushManager) isQuietHours(user *PushUser) bool {
	if user.Timezone == nil {
		return false
	}
	now := time.Now().In(user.Timezone)
	hour := now.Hour()
	return hour >= 22 || hour < 7
}

// canPush checks rate limits and quiet hours
func (pm *PushManager) canPush(user *PushUser) bool {
	// No subscription
	if user.Subscription == nil {
		return false
	}

	// Quiet hours (10pm-7am local time)
	if pm.isQuietHours(user) {
		return false
	}

	// Rate limit: max 1 push per 5 minutes
	if time.Since(user.LastPush) < 5*time.Minute {
		return false
	}

	// Don't push if user was active recently (they have the app open)
	if time.Since(user.LastPing) < 2*time.Minute {
		return false
	}

	return true
}

// PushNotification represents a push message
type PushNotification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Icon  string `json:"icon,omitempty"`
	Tag   string `json:"tag,omitempty"` // Replace previous with same tag
	Data  any    `json:"data,omitempty"`
}

// SendPush sends a push notification to a user
func (pm *PushManager) SendPush(sessionID string, notification *PushNotification) error {
	pm.mu.RLock()
	user, exists := pm.users[sessionID]
	pm.mu.RUnlock()

	if !exists || user.Subscription == nil {
		return nil
	}

	if !pm.canPush(user) {
		return nil
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
		TTL:             60, // 1 minute TTL for time-sensitive data
	})

	if err != nil {
		log.Printf("[push] Failed to send to %s: %v", sessionID[:8], err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusGone {
		// Subscription expired, remove it
		pm.Unsubscribe(sessionID)
		return nil
	}

	pm.mu.Lock()
	user.LastPush = time.Now()
	// Store in push history for timeline display
	user.PushHistory = append(user.PushHistory, PushHistoryItem{
		Time:  time.Now(),
		Title: notification.Title,
		Body:  notification.Body,
	})
	// Keep only last 20 push notifications
	if len(user.PushHistory) > 20 {
		user.PushHistory = user.PushHistory[len(user.PushHistory)-20:]
	}
	pm.mu.Unlock()

	pm.save() // Persist push history
	log.Printf("[push] Sent to %s: %s", sessionID[:8], notification.Title)
	return nil
}

// backgroundLoop checks for users who need push updates
func (pm *PushManager) backgroundLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		pm.checkAndPush()                // Bus/context updates for backgrounded users
		pm.checkScheduledNotifications() // Daily scheduled notifications
	}
}

// checkAndPush evaluates all users and sends relevant pushes
func (pm *PushManager) checkAndPush() {
	pm.mu.RLock()
	users := make([]*PushUser, 0, len(pm.users))
	for _, u := range pm.users {
		users = append(users, u)
	}
	pm.mu.RUnlock()

	for _, user := range users {
		if !pm.canPush(user) {
			continue
		}

		// User hasn't pinged in 2-30 minutes = probably backgrounded
		// Beyond 30 min, they've probably left the area
		sinceLastPing := time.Since(user.LastPing)
		if sinceLastPing < 2*time.Minute || sinceLastPing > 30*time.Minute {
			continue
		}

		// Get fresh context for their last known location
		notification := pm.buildNotification(user)
		if notification != nil {
			pm.SendPush(user.SessionID, notification)
		}
	}
}

// buildNotification creates a notification based on user's context
func (pm *PushManager) buildNotification(user *PushUser) *PushNotification {
	// Import cycle prevention: we can't import spatial here
	// Instead, we'll call a callback that's set by main.go
	if buildNotificationCallback == nil {
		return nil
	}
	return buildNotificationCallback(user.Lat, user.Lon, user.BusNotify)
}

// Callback for building notifications (set by main.go to avoid import cycle)
// Third param is whether bus notifications are enabled for this user
var buildNotificationCallback func(lat, lon float64, busNotify bool) *PushNotification

// SetNotificationBuilder sets the callback for building notifications
func SetNotificationBuilder(cb func(lat, lon float64, busNotify bool) *PushNotification) {
	buildNotificationCallback = cb
}

// GetVAPIDPublicKey returns the public key for client subscription
func (pm *PushManager) GetVAPIDPublicKey() string {
	return pm.vapidPublic
}

// HandleSubscribe handles POST /push/subscribe
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

// HandleUnsubscribe handles POST /push/unsubscribe
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

// HandleVAPIDKey handles GET /push/vapid-key
func HandleVAPIDKey(w http.ResponseWriter, r *http.Request) {
	pm := GetPushManager()
	if pm.vapidPublic == "" {
		JsonError(w, "Push not configured", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"key": pm.vapidPublic})
}

// HandlePushHistory handles GET /push/history - returns recent push notifications for timeline
func HandlePushHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID := getSessionToken(w, r)
	if sessionID == "" {
		JsonError(w, "No session", http.StatusUnauthorized)
		return
	}

	pm := GetPushManager()
	pm.mu.RLock()
	user, exists := pm.users[sessionID]
	pm.mu.RUnlock()

	var history []PushHistoryItem
	if exists && user.PushHistory != nil {
		history = user.PushHistory
		// Clear history after fetching (will be re-populated by new pushes)
		pm.ClearPushHistory(sessionID)
	} else {
		history = []PushHistoryItem{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"history": history,
	})
}

// ClearPushHistory clears push history after client has fetched it
func (pm *PushManager) ClearPushHistory(sessionID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if user, exists := pm.users[sessionID]; exists {
		user.PushHistory = nil
	}
}

// SetBusNotify sets bus notification preference for a session
func (pm *PushManager) SetBusNotify(sessionID string, enabled bool) {
	pm.mu.Lock()

	user, exists := pm.users[sessionID]
	if !exists {
		// Create a user entry even without push subscription
		// This allows tracking preferences before they enable notifications
		user = &PushUser{
			SessionID: sessionID,
		}
		pm.users[sessionID] = user
	}

	user.BusNotify = enabled
	pm.mu.Unlock() // Release lock before save (save acquires its own lock)

	pm.save()
	log.Printf("[push] Bus notifications %s for session %s", map[bool]string{true: "enabled", false: "disabled"}[enabled], sessionID[:8])
}

// GetBusNotify gets bus notification preference for a session
func (pm *PushManager) GetBusNotify(sessionID string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if user, exists := pm.users[sessionID]; exists {
		return user.BusNotify
	}
	return false // Default: bus notifications off
}

// Scheduled notification types
const (
	NotifyMorningWeather = "morning_weather"
	NotifyDuha           = "duha"
	NotifyPrayerSoon     = "prayer_soon"
)

// checkScheduledNotifications runs scheduled pushes based on user's local time
func (pm *PushManager) checkScheduledNotifications() {
	pm.mu.RLock()
	users := make([]*PushUser, 0, len(pm.users))
	for _, u := range pm.users {
		if u.Subscription != nil && u.Lat != 0 {
			users = append(users, u)
		}
	}
	pm.mu.RUnlock()

	for _, user := range users {
		// Skip if quiet hours or recently pushed
		if pm.isQuietHours(user) {
			continue
		}

		now := time.Now()
		if user.Timezone != nil {
			now = now.In(user.Timezone)
		}
		hour, minute := now.Hour(), now.Minute()

		// Morning weather: 7:00-7:05am
		if hour == 7 && minute < 5 {
			if pm.canPushType(user, NotifyMorningWeather) {
				pm.pushMorningWeather(user)
			}
		}

		// Ad-Duha: Handled by client-side prayer reminder system
		// Don't send via push to avoid duplicates

		// Prayer reminders: check if any prayer is 10 min away
		pm.checkPrayerReminder(user, now)
	}
}

// canPushType checks if we can push this type (max once per day per type)
func (pm *PushManager) canPushType(user *PushUser, notifyType string) bool {
	now := time.Now()
	if user.Timezone != nil {
		now = now.In(user.Timezone)
	}
	today := now.Format("2006-01-02")

	if user.DailyPushed == nil {
		return true
	}

	lastDate, exists := user.DailyPushed[notifyType]
	return !exists || lastDate != today
}

// markPushed marks a notification type as sent today
func (pm *PushManager) markPushed(user *PushUser, notifyType string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	now := time.Now()
	if user.Timezone != nil {
		now = now.In(user.Timezone)
	}
	today := now.Format("2006-01-02")

	if user.DailyPushed == nil {
		user.DailyPushed = make(map[string]string)
	}
	user.DailyPushed[notifyType] = today

	// Persist
	pm.save()
}

// pushMorningWeather sends morning weather notification
func (pm *PushManager) pushMorningWeather(user *PushUser) {
	if buildNotificationCallback == nil {
		return
	}

	ctx := buildWeatherNotification(user.Lat, user.Lon)
	if ctx != nil {
		pm.SendPush(user.SessionID, ctx)
		pm.markPushed(user, NotifyMorningWeather)
	}
}

// pushDuha sends Ad-Duha reminder
func (pm *PushManager) pushDuha(user *PushUser) {
	pm.SendPush(user.SessionID, &PushNotification{
		Title: "☀️ Ad-Duha",
		Body:  "By the morning sunlight, and the night when it falls still... (93:1-2)",
		Tag:   "duha",
	})
	pm.markPushed(user, NotifyDuha)
}

// checkPrayerReminder checks if any prayer is ~10 min away
func (pm *PushManager) checkPrayerReminder(user *PushUser, now time.Time) {
	if buildPrayerNotification == nil {
		return
	}

	notification := buildPrayerNotification(user.Lat, user.Lon, now)
	if notification != nil {
		pm.SendPush(user.SessionID, notification)
	}
}

// Callbacks for building notifications (set by main.go)
var buildWeatherNotification func(lat, lon float64) *PushNotification
var buildPrayerNotification func(lat, lon float64, now time.Time) *PushNotification

// SetWeatherNotificationBuilder sets the callback for weather notifications
func SetWeatherNotificationBuilder(cb func(lat, lon float64) *PushNotification) {
	buildWeatherNotification = cb
}

// SetPrayerNotificationBuilder sets the callback for prayer notifications
func SetPrayerNotificationBuilder(cb func(lat, lon float64, now time.Time) *PushNotification) {
	buildPrayerNotification = cb
}

// PushAwarenessToArea pushes awareness items to all users in an area
func (pm *PushManager) PushAwarenessToArea(lat, lon float64, items []struct{ Emoji, Message string }) {
	if pm == nil {
		return
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	for _, user := range pm.users {
		// Check if user is in this area (within 2km)
		if user.Lat == 0 && user.Lon == 0 {
			continue
		}

		dist := haversine(lat, lon, user.Lat, user.Lon)
		if dist > 2.0 { // > 2km
			continue
		}

		for _, item := range items {
			// Content-based dedupe: extract key info from message
			// e.g., "Rain at 9pm" -> hash "rain_21" (rain + hour)
			contentKey := extractContentKey(item.Emoji, item.Message)
			if pm.hasRecentlyPushed(user, contentKey) {
				log.Printf("[push] Skipping duplicate for %s: %s", user.SessionID[:8], contentKey)
				continue
			}
			
			// Mark as pushed BEFORE sending to prevent race with other agents
			pm.markContentPushed(user, contentKey)

			notification := &PushNotification{
				Title: item.Emoji + " Malten",
				Body:  item.Message,
			}
			// Send without releasing lock (simpler, awareness isn't time-critical)
			pm.sendPushSimple(user, notification)
		}
	}
	
	// Persist after batch
	go pm.save()
}

// sendPushSimple sends push without releasing lock (for batch operations)
func (pm *PushManager) sendPushSimple(user *PushUser, notification *PushNotification) {
	if user.Subscription == nil {
		return
	}

	sub := &webpush.Subscription{
		Endpoint: user.Subscription.Endpoint,
		Keys: webpush.Keys{
			P256dh: user.Subscription.Keys.P256dh,
			Auth:   user.Subscription.Keys.Auth,
		},
	}

	payload, _ := json.Marshal(notification)
	
	// Send in goroutine to not block
	go func() {
		resp, err := webpush.SendNotification(payload, sub, &webpush.Options{
			VAPIDPublicKey:  pm.vapidPublic,
			VAPIDPrivateKey: pm.vapidPrivate,
			Subscriber:      pm.subject,
			TTL:             3600,
		})
		if err != nil {
			log.Printf("[push] Error sending to %s: %v", user.SessionID[:8], err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			log.Printf("[push] Failed for %s: status %d", user.SessionID[:8], resp.StatusCode)
			return
		}

		bodyPreview := notification.Body
		if len(bodyPreview) > 50 {
			bodyPreview = bodyPreview[:50]
		}
		log.Printf("[push] Sent to %s: %s", user.SessionID[:8], bodyPreview)
	}()
}

// extractContentKey extracts a dedupe key from notification content
// Goal: "Rain at 9 PM" and "Rain expected at 21:00" should have same key
func extractContentKey(emoji, message string) string {
	lower := strings.ToLower(message)
	
	// Rain notifications: key is "rain_<hour>" or "rain_now"
	if emoji == "🌧️" || strings.Contains(lower, "rain") {
		if strings.Contains(lower, "now") || strings.Contains(lower, "currently") || strings.Contains(lower, "likely now") {
			return "rain_now"
		}
		// Extract hour from message
		// Patterns: "at 9 PM", "at 21:00", "at 9pm", "around 14:30"
		re := regexp.MustCompile(`(?:at |around )(\d{1,2})(?::\d{2})?\s*(?:PM|AM|pm|am)?`)
		if matches := re.FindStringSubmatch(message); len(matches) > 1 {
			hour := matches[1]
			// Normalize to 24h
			if strings.Contains(strings.ToLower(message), "pm") {
				if h, _ := strconv.Atoi(hour); h < 12 {
					hour = strconv.Itoa(h + 12)
				}
			}
			return "rain_" + hour
		}
		return "rain_general"
	}
	
	// Temperature warnings
	if emoji == "❄️" || emoji == "🌡️" {
		return "temp_warning"
	}
	
	// Default: use first 50 chars of message
	key := strings.ToLower(message)
	if len(key) > 50 {
		key = key[:50]
	}
	return emoji + "_" + key
}

// hasRecentlyPushed checks if content was pushed in last 6 hours
func (pm *PushManager) hasRecentlyPushed(user *PushUser, contentKey string) bool {
	if user.PushedContent == nil {
		return false
	}
	timestamp, exists := user.PushedContent[contentKey]
	if !exists {
		return false
	}
	// Content expires after 6 hours
	return time.Now().Unix()-timestamp < 6*60*60
}

// markContentPushed records that content was pushed
func (pm *PushManager) markContentPushed(user *PushUser, contentKey string) {
	if user.PushedContent == nil {
		user.PushedContent = make(map[string]int64)
	}
	user.PushedContent[contentKey] = time.Now().Unix()
	
	// Clean old entries (older than 24h)
	cutoff := time.Now().Unix() - 24*60*60
	for k, v := range user.PushedContent {
		if v < cutoff {
			delete(user.PushedContent, k)
		}
	}
}

// canPushTodayLocked checks if we can push this notification type today (caller holds lock)
func (pm *PushManager) canPushTodayLocked(user *PushUser, notifyType string) bool {
	now := time.Now()
	if user.Timezone != nil {
		now = now.In(user.Timezone)
	}
	today := now.Format("2006-01-02")

	if user.DailyPushed == nil {
		return true
	}

	lastDate, exists := user.DailyPushed[notifyType]
	return !exists || lastDate != today
}

// markPushedLocked marks a notification type as sent today (caller holds lock)
func (pm *PushManager) markPushedLocked(user *PushUser, notifyType string) {
	now := time.Now()
	if user.Timezone != nil {
		now = now.In(user.Timezone)
	}
	today := now.Format("2006-01-02")

	if user.DailyPushed == nil {
		user.DailyPushed = make(map[string]string)
	}
	user.DailyPushed[notifyType] = today
}

// sendPushLocked sends a push notification (caller holds lock, releases for network call)
func (pm *PushManager) sendPushLocked(user *PushUser, notification *PushNotification) bool {
	if user.Subscription == nil {
		return false
	}

	// Copy what we need before releasing lock
	sub := &webpush.Subscription{
		Endpoint: user.Subscription.Endpoint,
		Keys: webpush.Keys{
			P256dh: user.Subscription.Keys.P256dh,
			Auth:   user.Subscription.Keys.Auth,
		},
	}
	sessionID := user.SessionID

	// Release lock for network call
	pm.mu.Unlock()
	defer pm.mu.Lock()

	payload, _ := json.Marshal(notification)
	resp, err := webpush.SendNotification(payload, sub, &webpush.Options{
		VAPIDPublicKey:  pm.vapidPublic,
		VAPIDPrivateKey: pm.vapidPrivate,
		Subscriber:      pm.subject,
		TTL:             3600,
	})
	if err != nil {
		log.Printf("[push] Error sending to %s: %v", sessionID[:8], err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Printf("[push] Failed for %s: status %d", sessionID[:8], resp.StatusCode)
		return false
	}

	bodyPreview := notification.Body
	if len(bodyPreview) > 50 {
		bodyPreview = bodyPreview[:50]
	}
	log.Printf("[push] Sent to %s: %s", sessionID[:8], bodyPreview)
	return true
}
