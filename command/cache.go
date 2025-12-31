package command

import (
	"sync"
	"time"
)

// CacheEntry holds a cached value with expiration
type CacheEntry struct {
	Value     string
	ExpiresAt time.Time
}

// Cache is a simple in-memory cache
type Cache struct {
	mu      sync.RWMutex
	entries map[string]CacheEntry
}

// GlobalCache is the shared cache instance
var GlobalCache = &Cache{
	entries: make(map[string]CacheEntry),
}

// Get retrieves a value from cache if not expired
func (c *Cache) Get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok {
		return "", false
	}
	if time.Now().After(entry.ExpiresAt) {
		return "", false
	}
	return entry.Value, true
}

// Set stores a value in cache with TTL
func (c *Cache) Set(key, value string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = CacheEntry{
		Value:     value,
		ExpiresAt: time.Now().Add(ttl),
	}
}
