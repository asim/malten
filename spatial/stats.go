package spatial

import (
	"fmt"
	"sync"
	"time"
)

// APIStats tracks statistics for an API endpoint
type APIStats struct {
	Name          string
	Calls         int64
	Successes     int64
	Errors        int64
	RateLimitHits int64
	LastCall      time.Time
	LastSuccess   time.Time
	LastError     time.Time
	LastErrorMsg  string
	ConsecErrors  int // Consecutive errors (for backoff)
}

// CacheStats tracks cache hit/miss statistics
type CacheStats struct {
	mu            sync.RWMutex
	LocationHits  int64
	LocationMiss  int64
	WeatherHits   int64
	WeatherMiss   int64
	PrayerHits    int64
	PrayerMiss    int64
	PingCount     int64
	PingTotalMs   int64 // Total milliseconds for all pings
}

var cacheStats = &CacheStats{}

// GetCacheStats returns global cache stats
func GetCacheStats() *CacheStats {
	return cacheStats
}

// RecordLocationHit records a location cache hit
func (c *CacheStats) RecordLocationHit() {
	c.mu.Lock()
	c.LocationHits++
	c.mu.Unlock()
}

// RecordLocationMiss records a location cache miss
func (c *CacheStats) RecordLocationMiss() {
	c.mu.Lock()
	c.LocationMiss++
	c.mu.Unlock()
}

// RecordPing records a ping timing
func (c *CacheStats) RecordPing(durationMs int64) {
	c.mu.Lock()
	c.PingCount++
	c.PingTotalMs += durationMs
	c.mu.Unlock()
}

// CacheSummary returns cache hit statistics
func (c *CacheStats) CacheSummary() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	avgPingMs := float64(0)
	if c.PingCount > 0 {
		avgPingMs = float64(c.PingTotalMs) / float64(c.PingCount)
	}
	
	locTotal := c.LocationHits + c.LocationMiss
	locHitRate := float64(0)
	if locTotal > 0 {
		locHitRate = float64(c.LocationHits) / float64(locTotal) * 100
	}
	
	return map[string]interface{}{
		"ping_count":       c.PingCount,
		"ping_avg_ms":      avgPingMs,
		"location_hits":    c.LocationHits,
		"location_misses":  c.LocationMiss,
		"location_hit_pct": locHitRate,
	}
}

// SystemStats tracks overall system statistics
type SystemStats struct {
	mu        sync.RWMutex
	APIs      map[string]*APIStats
	StartTime time.Time
}

var stats = &SystemStats{
	APIs:      make(map[string]*APIStats),
	StartTime: time.Now(),
}

// GetStats returns the global stats instance
func GetStats() *SystemStats {
	return stats
}

// getOrCreateAPI returns stats for an API, creating if needed (caller must hold lock)
func (s *SystemStats) getOrCreateAPI(name string) *APIStats {
	if api, ok := s.APIs[name]; ok {
		return api
	}
	api := &APIStats{Name: name}
	s.APIs[name] = api
	return api
}

// GetAPI returns stats for an API (public, takes lock)
func (s *SystemStats) GetAPI(name string) *APIStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getOrCreateAPI(name)
}

// RecordCall records an API call attempt
func (s *SystemStats) RecordCall(name string) {
	s.mu.Lock()
	api := s.getOrCreateAPI(name)
	api.Calls++
	api.LastCall = time.Now()
	s.mu.Unlock()
}

// RecordSuccess records a successful API call
func (s *SystemStats) RecordSuccess(name string) {
	s.mu.Lock()
	api := s.getOrCreateAPI(name)
	api.Successes++
	api.LastSuccess = time.Now()
	api.ConsecErrors = 0
	s.mu.Unlock()
}

// RecordError records an API error
func (s *SystemStats) RecordError(name string, err error) {
	s.mu.Lock()
	api := s.getOrCreateAPI(name)
	api.Errors++
	api.LastError = time.Now()
	api.LastErrorMsg = err.Error()
	api.ConsecErrors++
	s.mu.Unlock()
}

// RecordRateLimit records a rate limit hit
func (s *SystemStats) RecordRateLimit(name string) {
	s.mu.Lock()
	api := s.getOrCreateAPI(name)
	api.RateLimitHits++
	api.ConsecErrors++
	s.mu.Unlock()
}

// GetBackoffDuration returns how long to wait based on consecutive errors
func (s *SystemStats) GetBackoffDuration(name string) time.Duration {
	s.mu.RLock()
	api := s.APIs[name]
	var consecErrors int
	if api != nil {
		consecErrors = api.ConsecErrors
	}
	s.mu.RUnlock()

	if consecErrors == 0 {
		return 0
	}

	// Exponential backoff: 1s, 2s, 4s, 8s, 16s, 32s, max 60s
	backoff := time.Duration(1<<uint(consecErrors-1)) * time.Second
	if backoff > 60*time.Second {
		backoff = 60 * time.Second
	}
	return backoff
}

// Summary returns a formatted summary of all API stats
func (s *SystemStats) Summary() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	uptime := time.Since(s.StartTime)

	result := fmt.Sprintf("📊 System Stats (uptime: %s)\n\n", formatDuration(uptime))

	for name, api := range s.APIs {
		successRate := float64(0)
		if api.Calls > 0 {
			successRate = float64(api.Successes) / float64(api.Calls) * 100
		}

		result += fmt.Sprintf("**%s**\n", name)
		result += fmt.Sprintf("  Calls: %d (%.1f%% success)\n", api.Calls, successRate)

		if api.RateLimitHits > 0 {
			result += fmt.Sprintf("  Rate limits: %d\n", api.RateLimitHits)
		}
		if api.Errors > 0 {
			result += fmt.Sprintf("  Errors: %d", api.Errors)
			if api.ConsecErrors > 0 {
				result += fmt.Sprintf(" (%d consecutive)", api.ConsecErrors)
			}
			result += "\n"
		}
		if !api.LastSuccess.IsZero() {
			result += fmt.Sprintf("  Last success: %s\n", formatTimeAgo(api.LastSuccess))
		}
		if api.LastErrorMsg != "" {
			result += fmt.Sprintf("  Last error: %s\n", truncate(api.LastErrorMsg, 50))
		}
		result += "\n"
	}

	return result
}

func formatTimeAgo(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return "just now"
	} else if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return t.Format("Jan 2 15:04")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}
