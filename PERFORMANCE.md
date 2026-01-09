# Malten Performance Analysis

## Goals

- **Page load → first context visible**: < 100ms (from cache), < 500ms (network)
- **All read operations**: < 100ms (no external API calls)
- **Write operations (ping)**: < 500ms (may touch spatial cache)
- **No deadlocks**: All locks are short-held, no lock chains

## Client → Server Endpoints

| Endpoint | Method | Purpose | Target Latency |
|----------|--------|---------|----------------|
| `/` | GET | Static HTML (embedded) | < 50ms |
| `/malten.js` | GET | Static JS (embedded) | < 50ms |
| `/events` | GET/WS | WebSocket connection | < 100ms |
| `/commands` | POST | Execute command (ping, nearby, etc) | < 500ms |
| `/commands` | GET | Get command metadata | < 100ms |
| `/messages` | GET | Get server messages (rarely used) | < 100ms |
| `/push/*` | POST | Push subscription management | < 200ms |
| `/map` | GET | Map HTML page | < 100ms |
| `/agents` | GET | Agent info for map | < 100ms |
| `/debug` | GET | Server stats | < 100ms |

## Critical Path: Page Load

```
1. Browser requests / 
   → Serve embedded index.html (< 50ms)

2. Browser loads malten.js
   → Serve embedded JS (< 50ms)

3. $(document).ready():
   a) showCachedContext()     → Display localStorage state.context (0ms)
   b) loadTimeline()          → Display localStorage state.timeline (0ms)
   c) scrollToBottom()        → DOM operation (0ms)
   d) registerServiceWorker() → Background, non-blocking
   e) checkPushState()        → Async, non-blocking
   f) loadListeners()         → DOM setup (0ms)
   g) loadCommandMeta()       → GET /commands (async, non-blocking)
   h) loadMessages()          → GET /messages (async, non-blocking)
   i) fetchReminder()         → POST /commands /reminder (async, non-blocking)

4. If context stale (>60s) or no location:
   getLocationAndContext()
   → navigator.geolocation.getCurrentPosition (OS-level, ~1-5s)
   → POST /commands /ping (lat, lon)
   
5. Otherwise: startLocationWatch()
   → Background GPS updates
```

**Result**: Cached context displays in < 100ms. Fresh context in < 2s.

## Critical Path: /ping Command

```
Client: POST /commands { prompt: "/ping", lat: X, lon: Y, accuracy: A }

Server (PostCommandHandler):
1. Parse form values                              (< 1ms)
2. Get/create session token                       (< 1ms)
3. Parse lat/lon from POST                        (< 1ms)
4. command.SetLocation(token, lat, lon)           (< 10ms, in-memory)
5. command.Dispatch → handlePing:
   a) spatial.RecordLocation()                    (< 1ms, in-memory)
   b) UpdatePushLocation()                        (< 1ms, in-memory)
   c) spatial.GetContextWithChanges():
      - GetSessionContext() - RLock               (< 1ms)
      - GetContextData(lat, lon):                 (SEE BELOW)
      - SetSessionContext() - Lock                (< 1ms)
      - DetectChanges() - pure computation        (< 1ms)
6. Marshal JSON response                          (< 1ms)
7. Return HTTP response

GetContextData(lat, lon):
1. db.FindAgent() - RLock on entity store         (< 1ms)
2. db.GetNearestLocation() - RLock + rtree        (< 10ms)
   - If miss: fetchLocation() → Nominatim API    (200-500ms) ⚠️
3. db.Query(weather) - RLock + rtree              (< 10ms)
   - If miss: go fetchWeather() (background)
4. db.Query(prayer) - RLock + rtree               (< 10ms)
   - If miss: go fetchPrayerTimes() (background)
5. db.Query(arrivals) - RLock + rtree             (< 10ms)
   - If miss: no auto-fetch (relies on agent)
6. db.Query(places) - RLock + rtree               (< 10ms)
7. Build HTML string                              (< 1ms)
```

**Total for cache hit**: ~2-5ms ✅
**Total for cache miss (location)**: ~15-20ms (async, no blocking)

## External API Calls

| API | When Called | Cached For | Fallback |
|-----|-------------|------------|----------|
| Nominatim (reverse geocode) | New location point | Permanent (rtree) | Return empty |
| Open-Meteo (weather) | Agent startup, periodic | 1 hour | Show stale |
| TfL (bus arrivals) | Agent startup, periodic | 2 minutes | Show stale |
| Aladhan (prayer) | First request to area | 24 hours | Show stale |
| OSRM (directions) | /directions command | Not cached | Return error |
| Foursquare (places) | /nearby command (fallback) | Permanent | Show OSM only |

### Rule: No External Calls in Read Path

The `/ping` command should NEVER block on external APIs for users with cached data.

**FIXED**: `fetchLocation()` now runs async. Returns coordinates as fallback.
Next ping will show resolved name after background fetch completes.

## localStorage Keys

| Key | Purpose | Size |
|-----|---------|------|
| `malten_state` | Main state blob (JSON) | ~10-50KB |
| `malten_session` | Session token | 36 bytes |
| `malten_debug` | Debug mode flag | 4 bytes |

### state Object Structure

```javascript
{
  version: 3,                    // Migration trigger
  lat: 51.4179,                  // Last known latitude
  lon: -0.3706,                  // Last known longitude
  context: {                     // Last context response (JSON)
    html: "...",                 // Formatted display HTML
    location: { name, lat, lon },
    weather: { temp, condition, icon, rain_warning },
    prayer: { current, next, next_time, display },
    bus: { stop_name, distance, arrivals },
    places: { "cafe": [...], "pharmacy": [...] },
    agent: { id, status, poi_count }
  },
  contextTime: 1736500000000,    // When context was fetched (Date.now())
  contextExpanded: false,        // Is context card expanded?
  locationHistory: [...],        // Last 20 location points
  lastBusStop: "...",            // Last bus stop shown
  timeline: [...],               // Timeline entries (24h retention)
  checkedIn: null,               // Manual location override
  savedPlaces: {},               // Saved places (home, work, etc)
  steps: { count, date },        // Step counter
  reminderDate: "2025-01-09",    // Last reminder shown date
  prayerReminders: {},           // Prayer reminder tracking
  natureReminderDate: null,      // Nature reminder tracking
  photos: [...],                 // Captured photos (base64)
  startFrom: 0                   // Timeline pagination offset
}
```

## Server State (In-Memory)

| Store | Purpose | Access Pattern |
|-------|---------|----------------|
| `spatial.DB.entities` | R-tree of all entities | RWMutex, query by lat/lon |
| `server.streams` | WebSocket message streams | RWMutex |
| `server.observers` | Connected WebSocket clients | RWMutex |
| `command.locations` | Per-session location tracking | RWMutex |
| `command.userContexts` | Per-session AI context | RWMutex |
| `spatial.sessionContexts` | Per-session last context (for change detection) | RWMutex |

## Lock Analysis

All locks are acquired for short durations (<10ms). No nested locks between packages.

| Lock | Package | Held During |
|------|---------|-------------|
| `db.mu` | spatial | Entity read/write |
| `indexMu` | spatial | POI indexing (global, serializes API calls) |
| `locationsMu` | command | Session location read/write |
| `userContextsMu` | command | AI context read/write |
| `sessionContextsMu` | spatial | Change detection |
| `server.mtx` | server | Stream/observer management |

**No known deadlock scenarios** - locks are never held while acquiring another lock.

## Performance Recommendations

### Done
- [x] Cache location points permanently in rtree
- [x] Background fetch for weather, prayer
- [x] WebSocket for server→client push (no polling)
- [x] localStorage for timeline (local-first)
- [x] `fetchLocation()` async - returns coordinates immediately, fetches in background
- [x] Latency logging on /ping endpoint
- [x] Cache-hit metrics on /debug endpoint (`cache.ping_avg_ms`, `cache.location_hit_pct`)

### TODO
- [ ] Add client-side performance timing
- [ ] Consider IndexedDB for photos (localStorage has 5MB limit)

## Measured Performance (2026-01-09)

```
Ping (cache hit):    2-5ms
Ping (cache miss):   15-20ms (async location fetch)
Location lookup:     ~100µs (rtree)
Weather lookup:      ~150µs (rtree)
Bus arrivals lookup: ~150µs (rtree)
Places lookup:       ~400µs (rtree)
Total GetContextData: 2-3ms
```

Test with: `curl -s 'http://localhost:9090/debug' | jq '.cache'`
