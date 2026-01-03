package spatial

import (
	"log"
	"sync"
	"time"
)

// APIRateLimiter serializes external API calls to prevent rate limiting
// All external API calls should go through this to coordinate across goroutines
type APIRateLimiter struct {
	mu           sync.Mutex
	lastCall     map[string]time.Time
	minInterval  time.Duration
	globalLock   sync.Mutex // Ensures only one API call at a time
}

var (
	// Global rate limiter for all external APIs
	apiLimiter = &APIRateLimiter{
		lastCall:    make(map[string]time.Time),
		minInterval: 2 * time.Second, // At least 2 seconds between any API calls
	}
)

// RateLimitedCall executes fn after waiting for rate limit
// apiName is used to track per-API timing (e.g. "tfl", "osm", "weather")
// Returns error from fn, or nil if skipped due to recent call
func RateLimitedCall(apiName string, fn func() error) error {
	apiLimiter.mu.Lock()
	
	// Check if we called this API recently
	if last, ok := apiLimiter.lastCall[apiName]; ok {
		elapsed := time.Since(last)
		if elapsed < apiLimiter.minInterval {
			wait := apiLimiter.minInterval - elapsed
			apiLimiter.mu.Unlock()
			log.Printf("[ratelimit] %s: waiting %.1fs", apiName, wait.Seconds())
			time.Sleep(wait)
			apiLimiter.mu.Lock()
		}
	}
	
	// Mark this API as being called
	apiLimiter.lastCall[apiName] = time.Now()
	apiLimiter.mu.Unlock()
	
	// Serialize all API calls globally (only one at a time)
	apiLimiter.globalLock.Lock()
	defer apiLimiter.globalLock.Unlock()
	
	return fn()
}

// TfLRateLimitedCall is a convenience wrapper for TfL API calls
func TfLRateLimitedCall(fn func() error) error {
	return RateLimitedCall("tfl", fn)
}

// OSMRateLimitedCall is a convenience wrapper for OSM Overpass API calls  
func OSMRateLimitedCall(fn func() error) error {
	return RateLimitedCall("osm", fn)
}
