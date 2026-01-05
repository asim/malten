package spatial

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// ExternalClient wraps http.Client with rate limiting, stats, and logging
type ExternalClient struct {
	client      *http.Client
	defaultAPI  string // Default API name for stats if not specified in request
}

// Global external client - use this for all external API calls
var External = &ExternalClient{
	client: &http.Client{
		Timeout: 30 * time.Second,
	},
	defaultAPI: "http",
}

// APIRequest wraps an HTTP request with API metadata
type APIRequest struct {
	*http.Request
	APIName string // For stats tracking (e.g., "tfl", "weather", "osm")
}

// NewRequest creates a new API request with tracking
func NewRequest(apiName, method, url string) (*APIRequest, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Malten/1.0")
	return &APIRequest{Request: req, APIName: apiName}, nil
}

// Get is a convenience method for GET requests
func (c *ExternalClient) Get(apiName, url string) (*http.Response, error) {
	req, err := NewRequest(apiName, "GET", url)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// Do executes the request with rate limiting and stats
func (c *ExternalClient) Do(req *APIRequest) (*http.Response, error) {
	apiName := req.APIName
	if apiName == "" {
		apiName = c.defaultAPI
	}
	
	stats := GetStats()
	
	// Check for backoff due to previous errors
	backoff := stats.GetBackoffDuration(apiName)
	if backoff > 0 {
		log.Printf("[http] %s: backing off %.1fs", apiName, backoff.Seconds())
		time.Sleep(backoff)
	}
	
	// Rate limit
	apiLimiter.mu.Lock()
	if last, ok := apiLimiter.lastCall[apiName]; ok {
		elapsed := time.Since(last)
		if elapsed < apiLimiter.minInterval {
			wait := apiLimiter.minInterval - elapsed
			apiLimiter.mu.Unlock()
			time.Sleep(wait)
			apiLimiter.mu.Lock()
		}
	}
	apiLimiter.lastCall[apiName] = time.Now()
	apiLimiter.mu.Unlock()
	
	// Record call attempt
	stats.RecordCall(apiName)
	start := time.Now()
	
	// Execute
	resp, err := c.client.Do(req.Request)
	duration := time.Since(start)
	
	// Log
	status := "err"
	if resp != nil {
		status = fmt.Sprintf("%d", resp.StatusCode)
	}
	log.Printf("[http] %s %s %s (%dms)", apiName, req.Method, truncateURL(req.URL.String()), duration.Milliseconds())
	
	// Handle errors
	if err != nil {
		stats.RecordError(apiName, err)
		return nil, err
	}
	
	// Handle HTTP errors
	if resp.StatusCode == 429 {
		stats.RecordRateLimit(apiName)
		err := fmt.Errorf("%s rate limited (429)", apiName)
		return resp, err
	}
	
	if resp.StatusCode >= 400 {
		stats.RecordError(apiName, fmt.Errorf("HTTP %d", resp.StatusCode))
		// Still return resp so caller can handle/read body
	} else {
		stats.RecordSuccess(apiName)
	}
	
	_ = status // used in log above
	return resp, nil
}

// GetJSON is a convenience method that returns body bytes
func (c *ExternalClient) GetJSON(apiName, url string) ([]byte, error) {
	resp, err := c.Get(apiName, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	
	return io.ReadAll(resp.Body)
}

func truncateURL(url string) string {
	if len(url) > 80 {
		return url[:77] + "..."
	}
	return url
}

// Convenience functions for specific APIs

// WeatherGet makes a weather API call
func WeatherGet(url string) (*http.Response, error) {
	return External.Get("weather", url)
}

// PrayerGet makes a prayer times API call
func PrayerGet(url string) (*http.Response, error) {
	return External.Get("prayer", url)
}

// LocationGet makes a location/geocoding API call
func LocationGet(url string) (*http.Response, error) {
	return External.Get("location", url)
}

// OSMGet makes an OpenStreetMap API call
func OSMGet(url string) (*http.Response, error) {
	return External.Get("osm", url)
}

// TfLGet makes a TfL API call
func TfLGet(url string) (*http.Response, error) {
	return External.Get("tfl", url)
}

// FoursquareGet makes a Foursquare API call
func FoursquareGet(url string) (*http.Response, error) {
	return External.Get("foursquare", url)
}

// NewsGet makes a news API call
func NewsGet(url string) (*http.Response, error) {
	return External.Get("news", url)
}

// OSRMGet makes an OSRM routing API call
func OSRMGet(url string) (*http.Response, error) {
	return External.Get("osrm", url)
}

// GenericGet makes a generic HTTP call with default tracking
func GenericGet(url string) (*http.Response, error) {
	return External.Get("http", url)
}
