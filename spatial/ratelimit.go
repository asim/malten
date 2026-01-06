package spatial

import (
	"log"
	"sync"
	"time"
)

// APIRateLimiter serializes external API calls to prevent rate limiting
type APIRateLimiter struct {
	mu          sync.Mutex
	lastCall    map[string]time.Time
	minInterval time.Duration
}

var (
	apiLimiter = &APIRateLimiter{
		lastCall:    make(map[string]time.Time),
		minInterval: 2 * time.Second,
	}

	// Per-API minimum intervals (some APIs need longer)
	apiMinIntervals = map[string]time.Duration{
		"osm":      5 * time.Second, // Overpass API is strict
		"osrm":     2 * time.Second,
		"tfl":      2 * time.Second,
		"weather":  2 * time.Second,
		"location": 1 * time.Second, // Nominatim
	}
)

// LLM rate limiter (separate, longer interval for Fanar)
var llmLimiter = struct {
	mu          sync.Mutex
	lastCall    time.Time
	minInterval time.Duration
}{
	minInterval: 500 * time.Millisecond,
}

// LLMRateLimitedCall wraps LLM API calls with rate limiting and stats
func LLMRateLimitedCall(fn func() error) error {
	stats := GetStats()

	// Check for backoff
	backoff := stats.GetBackoffDuration("llm")
	if backoff > 0 {
		log.Printf("[ratelimit] llm: backing off %.1fs", backoff.Seconds())
		time.Sleep(backoff)
	}

	llmLimiter.mu.Lock()

	elapsed := time.Since(llmLimiter.lastCall)
	if elapsed < llmLimiter.minInterval {
		wait := llmLimiter.minInterval - elapsed
		llmLimiter.mu.Unlock()
		time.Sleep(wait)
		llmLimiter.mu.Lock()
	}

	llmLimiter.lastCall = time.Now()
	llmLimiter.mu.Unlock()

	stats.RecordCall("llm")

	err := fn()
	if err != nil {
		if isRateLimitError(err) {
			stats.RecordRateLimit("llm")
		} else {
			stats.RecordError("llm", err)
		}
		return err
	}

	stats.RecordSuccess("llm")
	return nil
}

// isRateLimitError checks if error is a rate limit (429)
func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "429") || contains(errStr, "rate limit") || contains(errStr, "Too Many Requests")
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
