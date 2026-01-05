# Malten Development Context

## URLs
- **Production**: https://malten.ai
- **Local dev**: http://localhost:9090
- **Test via curl**: `curl -s -X POST 'http://localhost:9090/commands' -d 'prompt=/ping&lat=51.4497&lon=-0.3575'`

## CRITICAL: Production Safety

### Before Making Changes
1. **Never change data structures** without version migration
2. **Never delete localStorage keys** - add new ones, deprecate old
3. **Test on browser** before saying "done" - use browser_take_screenshot
4. **Bump version numbers** - Run `./bump-js.sh` to bump JS version in both places
5. **Restart via systemctl** - not manual process killing

### Development Flow
```bash
# Edit code
vim client/web/malten.js

# JS changes: just bump version (dynamic loading enabled)
# Go changes: rebuild + restart
go build -o malten . && sudo systemctl restart malten

# Check status
sudo systemctl status malten
journalctl -u malten -f  # tail logs

# Test
curl -s -X POST 'http://localhost:9090/commands' -d 'prompt=/ping&lat=51.4179&lon=-0.3706'
```

### State Version Migration
When changing `state` structure in malten.js:
1. Increment `state.version` (currently 3)
2. Old data will be cleared on load (except lat/lon preserved)
3. Document what changed in this file

### Error Handling Principles
1. **Never lose cached data** - if API fails, extend TTL of existing data
2. **Log errors clearly** - include context (lat/lon, what failed)
3. **Graceful degradation** - show stale data with indicator rather than nothing

## What is Malten

Spatial AI for the real world with contextual awareness. Foursquare if built in the AI era.

But internally, Malten is built with the morality and ethics of a Muslim. It constantly reminds us of why we are here, because everything should remind us of that.

The world is full of signs (ayat) of the Creator. The design. The complexity. The order. We could never make it. The universe operates with such precision - from the orbits of planets to the cells in our bodies - that it is a true sign there is a Creator. The One. The All-Powerful. Allah.

Malten surfaces these reminders subtly:
- Prayer times ground you in the rhythm of the day
- Names of Allah at each prayer - The Provider at Dhuhr, The Light at Maghrib
- Verses that speak to the moment - "By the morning sunlight" at Duha
- The natural world as sign - weather, sunrise, moon phases

This isn't preachy. It's woven into the fabric of the app. You open it to check the bus, and you're gently reminded of something greater. Utility and purpose, together.

**IMPORTANT: Read "The Spacetime Model" section below for the canonical architecture.**

## The Model

### User Experience
1. Open app → immediately see real world around you
   - Weather, temperature
   - Prayer times
   - Area name (street, postcode)
   - Bus/train times
   - Nearby places
2. Move → updates automatically
3. Events appear as messages in your timeline
4. Those in same area see the same spatial reality

### Core Primitives

| Primitive | Purpose |
|-----------|----------|
| **Streams** | Textual representation of geo space. One stream per area. |
| **Agents** | Spatial indexers. One agent per area/stream. Build world view. |
| **Commands** | Actions. Everything is a command. |
| **Database** | Quadtree spatial index. Real-time world state. |
| **Events** | Replayable log. Reference, replay, use elsewhere. |

### Streams = Geo Space
- Stream is the textual view of a geographic area
- Moving through space = moving through streams
- Same area = same stream = same spatial reality
- Messages on stream are events in that space

### Agents = Per Area
- One agent per area/stream
- Agent indexes and maintains that area's world view
- Fetches POIs, transport, weather, prayer times
- Stores in quadtree with TTL

### Database = Quadtree
- `spatial.json` - spatial index, source of truth
- All entities have lat/lon
- Query by location, radius, type
- Real-time world state

### Events = Replayable Log
- `events.jsonl` - append-only log
- Every action logged
- Can replay, reference, audit
- Source for other systems

## Message Formats

Messages are the primitive. Format is presentation:

| Format | Use |
|--------|-----|
| **Card** | Compact event (arrival, alert, prayer) |
| **Map** | Spatial view of area |
| **List** | Nearby places with details |
| **Text** | Plain response |

## HTTP Endpoints (Complete List)

| Endpoint | Method | Purpose |
|----------|--------|----------|
| `/` | GET | Static files (PWA) - served from disk in dev, embedded in prod |
| `/events` | GET | WebSocket for server→client messages |
| `/commands` | POST | All user input - commands and natural language |
| `/streams` | GET/POST | Stream management |
| `/messages` | GET | Stream message history |
| `/agents` | GET/POST/DELETE | Agent CRUD management |

**That's it.** No other endpoints. Everything goes through `/commands`.

### /streams Endpoint

```
GET /streams         → List all public streams
GET /streams?id=xxx  → Get single stream (TODO)
POST /streams        → Create new stream (stream, private, ttl params)
```

Stream types:
- **Geo streams** - geohash (e.g., `gcpsxz`) - created automatically on location
- **Named streams** - `~` (default), `~home`, `@family`, `#topic` - for private/shared spaces
- **Channels** - `@session_id` within a stream - your private view/conversation

Streams are created dynamically when someone connects. Geo streams dominate now, but named streams enable:
- Private spaces (your home stream follows you)
- Shared spaces (family, groups)
- Topic streams (communities, events)
- Building/venue streams (alternative to geo)

Future: stream navigation, hopping between streams while in one physical location.

## Commands (Complete List)

| Command | Description | Example |
|---------|-------------|----------|
| `/ping` | Update location, get JSON context | `/ping` (with lat/lon params) |
| `/nearby <type>` | Find nearby places | `/nearby cafe` |
| `/place <name>` | Get info about a place | `/place Starbucks` |
| `/directions <place>` | Walking directions | `/directions Waitrose` |
| `/checkin <place>` | Override GPS with manual location | `/checkin Costa Coffee` |
| `/agents` | Show all agents and status | `/agents` |
| `/weather` | Current weather | `/weather` |
| `/bus` | Next bus times | `/bus` |
| `/prayer` | Prayer times | `/prayer` |
| `/location` | Current location info | `/location` |
| `/summary` | Quick area summary | `/summary` |
| `/chat <query>` | Web-enhanced AI chat | `/chat what movies are showing` |
| `/web on/off` | Toggle web search | `/web on` |
| `/reminder` | Daily Islamic reminder | `/reminder` |

**Natural Language**: Anything not matching a command goes to AI with tool selection.

## Architecture

### Data Flow
```
User opens app at location
  → Geohash location → Stream ID
  → Join stream for that area
  → Agent for area builds world view
  → Context from quadtree (instant)
  → Display as messages

User moves
  → New geohash → New stream
  → Auto-switch stream
  → Agent for new area
  → New context
  → Timeline continuous (localStorage)
```

### Quadtree Entities
- **EntityPlace** - POIs from OSM
- **EntityAgent** - area indexers
- **EntityPerson** - users
- **EntityWeather** - weather (10min TTL)
- **EntityPrayer** - prayer times (1hr TTL)
- **EntityArrival** - transport (2min TTL)

### Agent Loop (per area)
```
Every 30s:
  - Fetch weather
  - Fetch prayer times
  - Fetch transport arrivals
  - Store in quadtree

Once (on creation):
  - Index POIs from OSM
  - Stations, stops, cafes, etc.
```

## Open Questions

### Private/Custom Streams
If streams = geo areas, what about:
- Private conversations?
- Topic streams?
- Group chats?

Options:
1. Remove goto/new stream - pure spatial
2. Keep for private/custom - prefix with `~` or `@`
3. Hybrid - default is geo, can create others

## Files
- `spatial/` - quadtree, entities, agents
- `command/` - all commands
- `server/` - HTTP, WebSocket, streams
- `client/web/` - PWA frontend
- `event/` - event log

## Client Timeline Functions

Your timeline is your worldline through spacetime. Everything flows through one path:

```javascript
addToTimeline(text, type)   // THE ONE way to add anything - saves + renders
loadTimeline()              // Load from localStorage on startup
renderTimelineItem(item)    // Render single item to DOM (internal)
getTimelineType(text)       // Determine type from emoji in text
```

**Item types:** `location`, `transport`, `weather`, `prayer`, `reminder`, `user`, `assistant`, `default`

**How it works:**
1. `addToTimeline()` dedupes, saves to `state.timeline`, prunes 24h, renders, scrolls
2. `loadTimeline()` called on startup, renders all persisted items
3. Everything persists across reloads - no more disappearing messages
4. User messages and AI responses both stored in `state.timeline` with type marker

**NEVER:**
- Render directly to DOM without saving
- Create new display functions
- Bypass `addToTimeline()`

## User Context
- Muslim. Prayer times important. No anthropomorphizing.
- Engineer. Brevity.

## Principles
1. Streams, Agents, Commands, Database, Events
2. Streams = geo space
3. Agents = per area
4. Everything is a message
5. Quadtree is source of truth
6. Proactive > reactive

## Don'ts
- Don't change the primitives
- Don't anthropomorphize agents
- Don't call APIs if quadtree has data

## Seamless Spatial Experience

### Core Principle
The stream/geohash is infrastructure, not UX. You don't "enter" a new stream - you're always in your continuous reality.

### What the User Sees
1. **Continuous view** - no jumps, no refreshes, no "loading new area"
2. **Persistent timeline** - cards don't disappear when you cross a boundary
3. **Smooth context updates** - weather, buses, places blend as you move
4. **No stream switching UI** - it happens in background, invisible

Like how your phone doesn't say "entering new cell tower coverage" - you're just connected.

### Implementation
- Geohash determines which agents are responsible
- Agents index into spatial DB
- User queries by lat/lon, gets seamless view
- Stream ID can change internally, but UI doesn't reset
- localStorage persists YOUR timeline across all locations

### The Two Views
- **Context box (top)** = live view of where you are NOW
- **Timeline (below)** = YOUR history, everywhere you've been

### What Geohash/Stream Actually Is
Like a database shard key - helps organize data and assign agents to areas. User never sees sharding. No page reloads. No "switching streams". Just smooth movement through space with your personal timeline following you.

## Keys and Storage

### Client-Side (Browser)

| Key | Type | Purpose |
|-----|------|----------|
| `malten_state` | localStorage | Personal timeline, location, context |
| `malten_session` | cookie | Session token for private server channels |

#### `malten_state` Structure (v3)
```javascript
{
    version: 3,              // Format version - change clears old data
    lat: 51.5,               // Last known latitude
    lon: -0.1,               // Last known longitude
    context: {               // JSON context from /ping (v3+)
        html: "📍 ...",    // Formatted display text
        location: {...},     // Structured location data
        weather: {...},      // Weather data
        prayer: {...},       // Prayer times
        places: {...}        // Places by category with lat/lon
    },
    contextTime: 1234567890, // When context was fetched
    locationHistory: [...],  // Last 20 location points
    timeline: [...],          // All messages (24hr retention) - user, assistant, system
                              // Each item: {text, type, time, lat, lon}
    checkedIn: null          // Manual location override {name, lat, lon, time}
}
```

#### Context JSON Structure (from /ping)
```javascript
{
    "html": "📍 Milton Road, TW12 2LL\n☀️ 3°C...",
    "location": {
        "name": "Milton Road, TW12 2LL",
        "postcode": "TW12 2LL",
        "lat": 51.4179,
        "lon": -0.3706
    },
    "weather": {
        "temp": 3,
        "condition": "☀️ 3°C",
        "icon": "",
        "rain_warning": ""
    },
    "prayer": {
        "current": "Asr",
        "next": "Maghrib",
        "next_time": "16:05",
        "display": "🕌 Asr now · Maghrib 16:05"
    },
    "bus": null,  // TODO: populate
    "places": {
        "cafe": [
            {"name": "Taste", "address": "70 Milton Road", "lat": 51.4169, "lon": -0.3708}
        ],
        "restaurant": [...],
        "pharmacy": [...],
        "supermarket": [...]
    }
}
```

**IMPORTANT**: This is YOUR personal timeline. Independent of stream/geohash. Survives location changes.

### Server-Side

| Key | Purpose |
|-----|----------|
| Stream ID | Geohash (6 chars) from location, e.g., `gcpsxb` |
| Channel | `@{session_token}` for private messages |
| Message | `{stream, channel, text}` - public if channel empty |

#### Channel Model
- **Public**: `channel: ""` - visible to all on stream
- **Private**: `channel: "@abc123"` - visible only to that session
- Questions/responses go to private channel
- Context updates are local (not stored on server)

### Version Migration
When `state.version` changes, old localStorage is cleared EXCEPT:
- `lat`, `lon` (location preserved)
- `savedPlaces` (preserved)
- `steps` (preserved)
- Everything else reset to defaults

**Version History:**
- v1: Initial
- v2: Added conversation persistence
- v3: Context changed from string to JSON object, timeline replaces cards+conversation (Jan 5 2026)

## Session Flow

```
Browser opens
  → Check cookie for malten_session
  → If none, server generates random token, sets cookie
  → Cookie sent with all requests
  → Server uses token for private channel: @{token}

User sends message
  → POST /commands with prompt, stream, lat, lon
  → Server stores to stream's @{session} channel
  → Response returned directly (HTTP)
  → Also broadcast to user's WebSocket (channel filtered)
  → Client saves to localStorage (timeline)

User moves location
  → Geohash changes → new stream ID
  → WebSocket reconnects silently
  → localStorage unchanged (personal timeline continuous)
  → Server messages are in new stream's channel
```

## Cache & TTL Strategy

### Entity TTLs
| Entity | TTL | Radius | Notes |
|--------|-----|--------|-------|
| Weather | 10min | 5km | Same weather for nearby area |
| Prayer | 1hr | 50km | City-wide, recalculated on display |
| Arrival | 5min | 500m | Bus/train times, stale-tolerant |
| Disruption | 10min | 10km | Traffic/roadworks |
| Location | 1hr | 500m | Reverse geocoded street names |
| Place | none | 500m | POIs from OSM, no expiry |

### Stale Data Handling
- `QueryWithMaxAge(lat, lon, radius, type, limit, maxAgeSecs)` - accepts stale data
- Arrivals allow 10min stale (600s) - better to show old bus times than nothing
- Stale arrivals show ⏳ indicator
- Background refresh triggered when stale data served

### Cache Consistency Rules
1. Query radius must match fetch radius (prayer times bug was 10km vs 50km mismatch)
2. Fetch functions return nil if fresh cache exists (to avoid duplicate inserts)
3. GetLiveContext queries cache first, fetches on-demand if empty
4. Agents refresh in background every 30s, but user queries never wait

### Recent Fixes (Jan 2026)
- Prayer times: Fixed radius mismatch (10km→50km in GetLiveContext)
- Disruptions: Added EntityDisruption type + caching (was fetch-every-time)
- Arrivals: Added stale tolerance via QueryWithMaxAge

## Session: Cinema/Foursquare Integration (Jan 2 2026)

### Problems Fixed
1. **Cinema not found** - Added `amenity=cinema` to OSM indexing
2. **"Kingston Curzon is fictional"** - AI hallucinated when no data. Added supplementary cinema data from Curzon API
3. **Foursquare integration** - Falls back to Foursquare Places API when OSM returns nothing
4. **"nearest" geocoded as Alabama** - Word "nearest" was being geocoded to a cemetery in Alabama. Fixed filler word list
5. **"bowling" geocoded as Scotland** - Search terms were being geocoded as locations. Fixed to not geocode search term itself

### New Files
- `spatial/websearch.go` - Foursquare Places API integration
- `spatial/supplementary.go` - Static cinema chain data (Curzon)
- `data/cinemas.json` - Curzon cinema locations
- `command/web.go` - `/web on|off|status` command

### Environment
- `FOURSQUARE_API_KEY` in `.env` - Service API key for Places API
- New endpoint: `places-api.foursquare.com` with Bearer auth

### Data Flow for POI Search
1. Check spatial DB cache (OSM data)
2. Check supplementary data (cinema chains)
3. Query OSM Overpass API
4. **Fallback**: Foursquare Places API
5. **Final fallback**: Google Maps link

### Fixed: Natural Language Place Queries (Jan 2 2026)
Problem: "bowling near me" was hijacked by `isContextQuestion` (matched "near me") → sent to AI with context → AI had no bowling data → hallucinated.

Fix: `isContextQuestion` now only returns true for "near me" if the place type is one we have in context (cafe, restaurant, pharmacy, supermarket, shop). Unknown types like bowling, arcade, spa fall through to LLM tool selection, which now includes the `nearby` tool.

Flow:
1. "bowling near me" → isContextQuestion=false (bowling not in contextPlaceTypes)
2. selectTool asks LLM → returns `{"tool": "nearby", "args": {"type": "bowling"}}`
3. executeTool("nearby") → NearbyWithLocation("bowling")
4. "bowling" not in ValidTypes → searchByName → Foursquare → real results

### Key Files Changed
- `spatial/agent.go` - Added cinema/theatre to indexed categories
- `command/nearby.go` - Added Foursquare fallback, fixed geocoding bugs, 5km radius for sparse POIs
- `agent/agent.go` - Added nearby tool to selection prompt
- `server/handler.go` - Store location on POST for AI tool usage

## CRITICAL: Data Preservation

### NEVER DELETE
- `spatial.json` - The spatial index. Contains all cached POIs, agents, weather, prayer times, transport data
- `events.jsonl` - Event log for replay
- `data/*.json` - Supplementary data

### Why This Matters
1. Agents constantly index in background - POIs, transport, weather
2. API calls are rate-limited and costly (OSM Overpass, TfL, weather APIs)
3. Deleting spatial.json loses hours/days of indexed data
4. Recovery requires replaying event log or re-indexing everything

### Error Handling for APIs
When external APIs fail (rate limit, timeout, error):
1. **Log clearly** with context
2. **Extend TTL** of existing cached data (don't wipe it)
3. **Return stale data** with indicator rather than nothing
4. **Retry in background** via agent loop

Example (TfL 429):
```go
if resp.StatusCode == 429 {
    log.Printf("[transport] Rate limited (429) for %s", stopType)
    return []*Entity{} // empty = extend TTL of existing data
}
```

### Query Strategy
- **Cache first** - always check spatial DB before external APIs
- **Agents index async** - user queries should never wait for indexing
- **Stale is better than nothing** - show old data with ⏳ indicator

### The Pattern
```
User query → Check cache → Return if found (even if stale)
                        → If empty, query API → Cache result → Return
                        → If API fails, return empty (don't error)
Agent loop  → Periodically refresh cache in background
            → If refresh fails, extend TTL of existing data
```

User queries are instant from cache. Agents keep cache fresh.

## Debugging

### Client Debug Command
Type `/debug` in the app to see:
- Stream ID
- Location
- Context cache status
- Card count
- State version
- JS version

### Server Logs
```bash
journalctl -u malten -f              # Tail logs
journalctl -u malten --since "5 min ago" | grep -i error
```

### Common Issues

**Blank screen on mobile:**
- Usually JS error from stale localStorage
- Bump `state.version` to clear old data
- Check browser console for errors

**Bus/transport data missing:**
- TfL rate limiting (429) - wait and retry
- Check: `journalctl -u malten | grep "429\|rate"`
- Arrivals cache empty: agents not running or API blocked

**Places not clickable:**
- Check `state.context.places` exists
- Verify JSON structure from `/ping` response

**Form not submitting:**
- Check if HTML has `<form>` with `onsubmit`
- Verify `submitCommand()` function exists


## TODO: Agent Auto-Creation for New Areas

### Problem
User opens app in Cardiff (or any new location). No agent exists for that area. Currently nothing happens - user sees nothing.

### Solution
1. On first request from a new area (no agent exists), automatically spawn an agent
2. Tell the user: "Looks like we need to spin up an agent in your area, hold on a second"
3. Agent starts indexing: weather, prayer, POIs, transport
4. User can continue sending commands while agent works (async processing)
5. Commands queue and process when agent is ready

### Agent Expansion (Future)
- Agents could use OSRM to simulate walking streets
- Expand coverage by "walking" to adjacent areas
- Index POIs along routes using OSM + Foursquare
- Mimic real-world exploration to build comprehensive coverage

### Implementation Notes
- Check for agent existence on ping/context request
- If no agent, create one and return progress message
- Agent creation must be logged to events.jsonl
- First response includes "spinning up agent" message
- Subsequent requests return data as it becomes available

## TODO: Progress/Status Messages

### Problem
Coding agents (Shelley, Claude, Copilot) show what they're doing. Malten is silent.

### Solution
Be more verbal about what's happening:
- "Spinning up an agent for your area..."
- "Fetching weather data..."
- "Indexing nearby places..."
- "Found 12 cafes, 5 restaurants nearby"

### Async Command Processing
- Like Shelley: user can type and send multiple commands
- Commands queue and process in order
- Each command gets a response when done
- No blocking - user can keep interacting

## TODO: /agents Command

### Purpose
Show agent status and activity.

### Output Should Include
- Total number of agents
- Agent locations (geohash or human-readable)
- Status: active/idle
- Last activity timestamp
- What they're currently doing (if active)
- Entity counts per agent (POIs indexed, etc.)

### Example Output
```
🤖 3 agents active

gcpsvz (Hampton) - idle 5m
  📍 142 places, 🚏 3 stops, ⏱️ last refresh 2m ago

gcpswb (Kingston) - active
  🔄 Indexing cafes... (23/50)

gcjsze (Cardiff) - new
  🆕 Just created, initial indexing...
```

## Event Log (events.jsonl)

**See "The Spacetime Model" section for the canonical architecture.**

events.jsonl is the cosmic ledger - append-only, immutable, source of truth for facts about the world.

### What Gets Logged
- Entity CRUD (places, agents, weather, transport, prayer times)
- Agent activity (indexing, refreshing)
- System events (startup, shutdown, errors)
- Public stream broadcasts (channel="")

### What Does NOT Get Logged
- User queries/commands (private - belongs in localStorage)
- AI responses to users (private - belongs in localStorage)
- Private conversations (private - belongs in localStorage)

### Event Format
```json
{"ts":"2026-01-03T16:13:54Z","type":"entity.created","id":"abc123","data":{"lat":51.4,"lon":-0.3,"type":"place","name":"Costa Coffee"}}
{"ts":"2026-01-03T16:13:55Z","type":"entity.updated","id":"abc123","data":{...}}
```

### Current State
- events.jsonl exists at `/home/exedev/malten/events.jsonl`
- 193K+ events logged
- Logs entity.created, entity.updated events
- spatial.json can be rebuilt from events.jsonl

### TODO
- Add unique event IDs (evt_xxx format instead of entity ID)
- Add checkpoint markers for faster replay
- Implement full rebuild from events (RebuildFromEvents function)
- Test recovery scenario

## Map Visualization (TODO)

### Endpoint Design
- `/map` - Single endpoint, GET serves HTML, POST for data queries
- `/map.js` - Optional separate JS if map code gets large

### File Organization Trade-offs

**Single file (malten.js includes map)**
- Pros: One file to read, shared utilities, simpler build
- Cons: Large file, more tokens when only working on map

**Separate files (malten.js + map.js)**
- Pros: Focused editing, smaller token load per task
- Cons: Two files to understand, potential duplication

### Canvas vs WebGL

**Canvas 2D**
- Pros: Simple API, easy to debug, works everywhere, good for <10k points
- Cons: CPU-bound, slow with many points, no 3D

**WebGL**
- Pros: GPU-accelerated, handles 100k+ points, smooth pan/zoom
- Cons: Complex shaders, harder to debug, more code, overkill for small datasets

**Recommendation**: Start with Canvas. Switch to WebGL only if performance becomes an issue with real usage data.

### What to Show
- Agents (🤖) with status colors (active/idle/indexing)
- POIs by type (☕🍽️💊🛒)
- User location trail (breadcrumbs)
- Coverage areas (agent radius circles)

### Data Source
- Query existing spatial DB via internal Go calls
- No need for separate API endpoints
- `/map` handler fetches and embeds data in response

### Quadtree Library (`$HOME/quadtree`)
Low-level spatial index:
- Persistent file store
- Insert, search, k-nearest
- Has UI package (for standalone server, not embedded in Malten)

Malten uses quadtree for:
- Spatial indexing of all entities
- Nearest neighbor queries  
- Radius searches

## Prayer Times Fix (Jan 3 2026)

### Issue
Fajr was shown as "ending at sunrise" but Fajr actually ends BEFORE sunrise - when you can distinguish the white thread from the black thread in the sky.

### Fix
- Fajr end time calculated as 10 minutes before sunrise
- Example: Sunrise 08:06 → Fajr ends 07:56
- Shows "Fajr ending 07:56" when within 15 min of end
- Shows "Fajr ends 07:56" when earlier in Fajr period

### Also Fixed
- After sunrise but before Dhuhr: no longer shows "Fajr" as current
- Shows only "Dhuhr 12:06" without any current prayer
- Prayer times now computed at query time from stored timings (not cached display string)

## TODO: Check-in / Location Correction

### Problem
Indoor GPS is inaccurate. User might be in Curzon Kingston but GPS says A308 road outside.

### Solution: User-assisted location
When GPS seems stuck or inaccurate, prompt user with nearby POIs to "check in":
- GPS accuracy > 50m, OR
- GPS position unchanged for > 5 min while app is active
- POIs exist within 200m

Show dismissible card: "📍 Where are you?"
- Curzon Kingston (50m)
- Bentall Centre (80m)  
- John Lewis (120m)
- "Somewhere else..."

Selection overrides GPS until GPS moves > 200m from checked-in location.

### Trigger Enhancement: Movement Detection
Use DeviceMotion API to detect walking while GPS is stuck:
- Accelerometer shows movement pattern
- But GPS hasn't changed
- → GPS is probably wrong indoors
- → Prompt for check-in

### Related Features
- Tap context bar location → show nearby POIs to correct
- Search history inference (searched "curzon" 30 min ago + nearby = probably there)
- Time-based inference (evening + cinema + stationary = watching film)

## TODO: Step Counter

### Purpose
Count steps using accelerometer data from DeviceMotion API.

### Notes
- DeviceMotion API available in PWA (may need HTTPS, user permission on iOS)
- Can detect walking vs stationary
- Useful for: fitness tracking, movement detection, GPS accuracy hinting
- Store daily step counts in localStorage
- Could show in context: "🚶 2,340 steps today"

### Implementation
- Use `window.addEventListener('devicemotion', handler)`
- Detect step pattern from acceleration.z peaks
- Threshold-based peak detection
- Debounce to avoid double-counting

## Check-in Feature (Jan 3 2026)

### Problem
Indoor GPS is inaccurate. User might be in Curzon Kingston but GPS says A308 road outside.

### Solution
User-assisted location via check-in command.

### Flow
1. Server tracks location history per session
2. If GPS position unchanged for 5+ minutes within 30m, it's "stuck"
3. Server pushes "Where are you?" message with nearby POIs to user's channel
4. User replies `/checkin <place name>` or taps an option
5. Server matches name to nearby POI, sets check-in
6. Context now uses check-in location instead of GPS
7. Check-in clears if GPS moves 200m+ away, or after 2 hours

### Implementation

**Server-side (command/nearby.go):**
- `Location` struct now has `History []LocationPoint` and `CheckedIn *CheckIn`
- `SetLocation()` tracks history, detects stuck GPS, returns shouldPrompt
- `SetCheckIn()`, `GetCheckIn()`, `ClearCheckIn()` manage check-in state
- `/checkin` command matches place name to nearby POIs

**Server-side (server/location.go):**
- `PingHandler` uses check-in location for context if set
- `sendCheckInPrompt()` broadcasts POI options to user's channel

**Client-side (malten.js):**
- `state.checkedIn` persists to localStorage
- Click handlers for `.checkin-option` and `.checkin-dismiss`
- Sends `/checkin <name>` command when user selects option

### Constants
- `gpsStuckThreshold = 30m` - all positions must be within this
- `gpsStuckDuration = 5min` - how long to observe before "stuck"
- `checkInPromptCooldown = 10min` - don't prompt more often than this
- `checkInExpiry = 2hr` - check-in auto-expires
- Clear check-in when GPS moves 200m+ from check-in location



## Architecture: Commands Not Endpoints

See "HTTP Endpoints" and "Commands" sections at top of this file for complete list.

### Principle
**Every action is a command.** Don't add HTTP endpoints for features. Don't add client-side logic.

### Adding New Features
Register a command in `command/` package:

```go
func init() {
    Register(&Command{
        Name:        "myfeature",
        Description: "Does something useful",
        Handler: func(ctx *Context, args []string) (string, error) {
            // Implementation
            return "result", nil
        },
    })
}
```


### Why
1. Single channel for all interaction
2. Everything is observable, replayable, logged  
3. Client stays simple - sends text, displays text
4. Server has all the intelligence

### Don'ts
- Don't add new HTTP endpoints for features
- Don't add client-side fetching/logic
- Don't split intelligence between client and server


## Agents as Primitive

Agents are a first-class primitive with their own endpoint.

```

### Agent Properties
- id, name, lat, lon
- status: active, paused, indexing, refreshing
- prompt: current instruction/objective
- radius: coverage area in meters
- poiCount: entities indexed
- lastIndex: last indexing time
- updatedAt: last activity

### Actions
- `refresh` - Re-fetch all data for area
- `pause` - Stop agent loop
- `resume` - Restart agent loop  
- `move` - Change agent location (with lat/lon)

### Why Endpoint vs Command
- Agents are infrastructure, need REST management
- Multiple agents, need to address by ID
- CRUD operations don't fit command model
- `/agents` command still works for quick status view

### /agents Endpoint (form params, like /streams)
```
GET    /agents              - List all agents
GET    /agents?id=xxx       - Get specific agent
POST   /agents              - Create agent (lat, lon, prompt)
POST   /agents?id=xxx       - Instruct agent (action, prompt, lat, lon)
DELETE /agents?id=xxx       - Kill agent
```

Actions: refresh, pause, resume, move

## Session Checkpoint (Jan 3 2026 - Afternoon)

### Key Decisions Made

**Architecture: Commands Not Endpoints**
- Every action is a command. Commands flow through `/commands`.
- Only infrastructure gets endpoints: `/streams`, `/agents`, `/events`, `/messages`
- Removed `/ping` and `/context` endpoints - now `/ping` and `/context` commands
- Client sends commands, server pushes responses via WebSocket
- Don't add new endpoints for features. Add commands.

**Agents are a Primitive**
- Agents get their own endpoint `/agents` because they need CRUD management
- GET/POST/DELETE with JSON responses
- Accepts form params OR JSON body (if Content-Type: application/json)
- ID in path `/agents/{id}` or query `?id=xxx`

**API Consistency**
- All endpoints use `r.ParseForm()` for params (form or query string)
- JSON body optional when Content-Type header set
- All errors return JSON: `{"error": "message"}`
- All responses have Content-Type: application/json

### Features Built This Session (Jan 3 2026 - Afternoon)

**Check-in (GPS correction)**
- Server tracks location history per session
- Detects "stuck GPS" (same position 5+ min within 30m)
- Pushes "Where are you?" with nearby POIs
- User replies `/checkin <place>` to override location
- Check-in expires after 2 hours or 200m movement

**Commands added:**
- `/ping` - Location update, returns context
- `/context` - Get current context
- `/checkin <place>` - Check in to location

### Things We Keep Reiterating

1. **Commands not endpoints** - Don't add HTTP endpoints for features
2. **Server has the intelligence** - Client is dumb, just displays
3. **Agents are tools, not anthropomorphized** - They compute, don't "think"
4. **Consistency** - All APIs behave the same way
5. **Stream is the interface** - Everything flows through the stream

### What's Not Working / TODO

1. **Place links in context** - Only visible when context card expanded (by design? or fix?)
2. **Check-in prompt UI** - Server pushes text but client needs to render clickable options
3. **Step counter** - Noted for future (accelerometer access exists)
4. **Agent actions** - refresh/pause/resume update status but don't actually control agent loop yet

### Next Session Focus

1. **Map visualization** - Show agents, POIs, user location on a map
2. **Agent loop control** - Make pause/resume/refresh actually work
3. **Check-in UI polish** - Render check-in prompts as clickable cards
4. **Context card UX** - Consider showing place links in collapsed summary

### Files Changed This Session
- `command/nearby.go` - Added ping, context, checkin commands, location history tracking
- `server/location.go` - Removed old ping/context handlers, added sendCheckInPrompt
- `server/agents.go` - New file, full CRUD for agents
- `server/handler.go` - Added JsonError helper, cleaned up error handling
- `client/web/malten.js` - Updated to use /commands for ping/context, added checkin state
- `spatial/db.go` - Added Delete method
- `main.go` - Removed /ping, /context, /place endpoints, added /agents

## Session: JSON Context Refactor (Jan 3 2026 - Evening)

### Summary
Major refactor to use structured JSON for context instead of text with embedded `{data}` format.

### Changes Made

**Server:**
- New `GetContextData()` returns structured JSON with html, location, weather, prayer, places
- Removed duplicate `GetLiveContext()` function
- Removed `/context` command - just use `/ping`
- Removed dead `ContextHandler` and `PingHandler` endpoints from location.go
- `/ping` and `/context` responses no longer broadcast to WebSocket (they're JSON, not messages)
- Fixed bus data disappearing: API errors (429) now return empty slice, triggering TTL extension
- `ExtendArrivalsTTL` now saves the extended expiry and logs what it does
- Extended TTL from 2 min to 5 min for more resilience
- Directions shows all steps (removed maxSteps=4 limit)

**Client:**
- State version bumped to 3 (clears old string context)
- `state.context` is now JSON object with `.html`, `.places`, etc.
- `displayContext()` handles both JSON and legacy string format
- `buildContextHtml()` creates clickable place links from structured data
- Place links use `data-category` attribute, look up places from `state.context.places`
- Map links now use name + coordinates: `/maps/search/Name/@lat,lon,17z`
- Directions link styled blue (CSS)
- `{enable_location}` converted in `buildContextHtml()`

**Files Changed:**
- `spatial/context.go` - New file with ContextData struct and GetContextJSON
- `spatial/live.go` - Removed GetLiveContext, fixed error handling in fetchTransportArrivals
- `spatial/db.go` - ExtendArrivalsTTL now saves and returns count
- `spatial/routing.go` - Removed maxSteps limit
- `command/nearby.go` - Removed /context command, updated updateUserContext
- `command/directions.go` - Added DirectionsTo for known coordinates
- `server/handler.go` - Don't broadcast /ping responses to WebSocket
- `server/location.go` - Removed dead ContextHandler/PingHandler, removed json import
- `client/web/malten.js` - JSON context handling, state v3
- `client/web/malten.css` - directions-link styling
- `claude.md` - Comprehensive documentation update

### Context JSON Structure
```json
{
  "html": "📍 Milton Road...",
  "location": {"name": "...", "postcode": "...", "lat": 51.4, "lon": -0.3},
  "weather": {"temp": 3, "condition": "☀️ 3°C"},
  "prayer": {"current": "Asr", "display": "🕌 Asr now · Maghrib 16:05"},
  "bus": null,
  "places": {
    "cafe": [{"name": "Taste", "address": "70 Milton Road", "lat": 51.41, "lon": -0.37}],
    "restaurant": [...],
    "pharmacy": [...],
    "supermarket": [...]
  }
}
```

### Lessons Learned
1. Don't change data structures without version migration
2. Test in browser before declaring done
3. Use systemctl not manual process killing
4. When APIs fail, extend TTL of existing data - don't wipe it
5. Keep form as form - changing to div broke submitCommand()

## The Spacetime Model (CANONICAL)

This is the foundational architecture. All implementations must follow this model.

### How the Universe Stores Information

The universe stores information through state changes - every interaction leaves a trace. The cosmic microwave background is the echo of the Big Bang, still readable 13.8 billion years later. Spacetime itself is the ledger.

If you could traverse time and space, you'd be reading from that ledger at different coordinates. The information isn't stored somewhere separate - it IS the fabric. The present moment is just your read position.

### Applying This to Malten

The stream IS spacetime. It's not a communication channel - it's the medium through which reality propagates. When an agent indexes a cafe, that's an event in spacetime. When weather changes, that's an event. When you move, that's an event. These are facts about the world at coordinates (lat, lon, time).

**events.jsonl** is the cosmic ledger. Append-only. Immutable. Everything that happens gets written.

**The quadtree (spatial.json)** is a materialized view - you could delete spatial.json and rebuild it entirely from events.jsonl.

**Your private experience is different.** You're an observer moving through spacetime. Your timeline is YOUR worldline - the path you trace through space and time. What you see at each point is determined by your coordinates, but your memory of the journey is yours alone. That's localStorage - your consciousness, your continuity.

When you ask "what's nearby", you're querying the universe at your current coordinates. The answer comes from the quadtree (materialized spacetime). When you have a conversation with the AI, that's YOUR experience - private, stored in your worldline (localStorage).

### The Architecture

```
events.jsonl     = the ledger of spacetime (append-only, immutable, source of truth)
spatial.json     = materialized present (quadtree, rebuilt from events)
stream/websocket = real-time propagation of events to observers at coordinates
localStorage     = your worldline (your private journey through spacetime)
```

### What Goes Where

| Event Type | Storage | Why |
|------------|---------|-----|
| Entity created/updated/deleted | events.jsonl → quadtree | Fact about the world |
| Agent indexed a place | events.jsonl | Fact about the world |
| Weather fetched | events.jsonl | Fact about the world |
| Bus arriving | events.jsonl | Fact about the world |
| Place details (POI) | events.jsonl → quadtree | Fact about the world |
| User position update | events.jsonl (anonymized) | Fact about the world |
| You asked a question | localStorage | Your private experience |
| AI responded to you | localStorage | Your private experience |
| Conversation messages | localStorage | Your private experience |

### Streams, Channels, and Privacy

**Stream** = spacetime coordinates (geohash-based). Events propagate through streams to observers at those coordinates.

**Channel** = conversation filter. Multiple conversations can happen in the same geohash without polluting each other. Like radio frequencies - same space, different channels.

- `channel: ""` (empty) = public broadcast, all observers see it
- `channel: "@session"` = private to that session  
- `channel: "#group"` = shared among group members (future)

**Channels are for conversations between people, not for storing facts about the world.** The weather doesn't need a channel - it's a fact. A chat between two people in the same cafe needs a channel so others don't see incoherent mixed conversation they're not part of.

### The User Experience

1. **Open app** → localStorage shows YOUR timeline (your worldline)
2. **WebSocket connects** → receive live events for your coordinates
3. **Events update your view** → weather changes, bus arrives, place indexed
4. **You ask a question** → stored in YOUR localStorage, response also stored there
5. **You move** → new coordinates, new stream, same localStorage (continuity)
6. **You refresh** → localStorage restores your timeline, WebSocket reconnects
7. **You scroll back** → see your journey through spacetime from localStorage

### Consistency Across Users

Two users at the same coordinates see the same world:
- Same POIs (from quadtree, rebuilt from events)
- Same weather (from quadtree)
- Same bus times (from quadtree)  
- Same agent activity (from stream)

They DON'T see each other's:
- Private conversations (localStorage)
- Personal queries (localStorage)
- AI responses (localStorage)

Unless they're in the same channel - then they see shared conversation.

### Persistence Model

**events.jsonl (eternal, source of truth)**
```json
{"ts":"...","type":"entity.created","id":"...","data":{"lat":...,"lon":...,"type":"place","name":"..."}}
{"ts":"...","type":"entity.updated","id":"...","data":{...}}
{"ts":"...","type":"weather.fetched","id":"...","data":{"lat":...,"lon":...,"temp":...}}
{"ts":"...","type":"agent.indexed","id":"...","data":{"geohash":"...","count":...}}
```

**spatial.json (materialized view, rebuildable from events)**
- Quadtree of current entities
- Can be deleted and rebuilt from events.jsonl
- Query by coordinates for instant lookups
- Contains: places, agents, weather, transport, prayer times, etc.

**localStorage (user's worldline, private)**
```javascript
{
    version: 3,
    lat: 51.5, lon: -0.1,           // Current position  
    context: {...},                  // Current view of surroundings
    timeline: [...],                 // All messages (user, assistant, system)
    locationHistory: [...],          // Your path through spacetime
    checkedIn: null                  // Manual location override
}
```

### Server Restart Behavior

On restart:
1. Load spatial.json (quadtree) - instant
2. If spatial.json missing/corrupt, rebuild from events.jsonl
3. Agents resume their loops
4. Users reconnect via WebSocket  
5. Users see their localStorage timeline (unaffected by restart)
6. Live updates resume
7. POIs, places, weather all preserved (from spatial.json/events.jsonl)

### Implementation Rules

**DO persist to events.jsonl:**
- Entity CRUD (places, agents, weather, transport, prayer times)
- Agent activity (indexing, refreshing)
- System events (startup, shutdown, errors)
- POI discoveries (new cafes, restaurants, etc.)

**DON'T persist to events.jsonl:**
- User queries/commands (private)
- AI responses (private)  
- Private conversations (private)
- Session-specific data (private)

**Stream broadcasts (real-time via WebSocket):**
- Public events (new place indexed, weather update, agent status)
- Channel-scoped messages (conversations between people)

**localStorage handles (client-side):**
- User's timeline (all messages - user, assistant, system)
- Location history
- Context cache
- Saved places
- All private user data

### Rebuilding from Events

The quadtree (spatial.json) must be rebuildable from events.jsonl:

```go
func RebuildFromEvents(eventFile string) (*DB, error) {
    // Read events.jsonl line by line
    // For each entity.created/updated event, insert into quadtree
    // For each entity.deleted event, remove from quadtree
    // Result: identical state to before crash/restart
}
```

This guarantees:
- No data loss on restart
- Consistent view of the world
- POIs, places, weather all survive
- Users experience continuity

## Paid Features (Roadmap)

### Free Tier
- Local-only storage (localStorage)
- Export/import backup (`/export`, `/import`)
- Step counter (24h history)
- Saved places (local only)
- All core features (context, directions, nearby, etc.)

### Paid Tier
- **Cloud backup** - encrypted, auto-sync across devices
- **Saved places sync** - Home/Work available everywhere
- **Step history** - unlimited, with stats/graphs
- **Priority API** - faster responses, no rate limits
- **Offline maps** - cached tiles for areas you frequent

### Implementation Options

**Option A: Cloud Backup (account-based)**
1. User accounts - email/password or passkey authentication
2. Encrypted storage - user blob encrypted client-side, stored on server
3. Payment - Stripe integration
4. Sync protocol - merge conflicts, last-write-wins or CRDT

**Option B: WhatsApp Web Model (no accounts)**
1. Phone is primary device, holds all data
2. Link other devices via QR code scan
3. Real-time sync via WebSocket (phone pushes to linked devices)
4. No cloud storage - data lives on phone only
5. Simpler, more private, no accounts needed

Option B is simpler and more privacy-friendly. Could offer Option A as premium for those who want cloud backup.

### Pricing Ideas
- £2.99/month or £24.99/year
- Family plan (5 devices)
- One-time purchase option?

## Business Model

### Revenue Streams

**1. Freemium Subscriptions**
- Free: core features (context, steps, directions, nearby)
- Paid (~£3/mo): sync, backup, history, timeline sharing, priority API

**2. Local Business Partnerships**
- Businesses pay for visibility in nearby results
- "Promoted" places shown contextually (not ads, just higher ranking)
- Check-in offers: "You're near Costa, here's 10% off"
- Aligns naturally with spatial context

**3. Timeline Sharing**
- Share live location with family/friends (safety use case)
- "See my day" - share your timeline publicly or with select people
- Free tier: share with 1 person
- Paid tier: share with groups, longer history

### What We Don't Do
- Sell personal data
- Surveillance/tracking without consent
- Intrusive ads

### Competitive Advantage
- Privacy-first (localStorage by default)
- AI-native (not bolted on)
- Spatial context (not just maps)
- Muslim-friendly (prayer times built-in, not an afterthought)

## Session: Adaptive Ping (Jan 3 2026)

### Street updates too slow when driving
- Old: Fixed 15s ping interval
- New: Adaptive ping based on speed
  - Driving (>10 m/s): 5s  
  - Walking (2-10 m/s): 10s
  - Stationary: 30s
- Uses haversine distance between pings to calculate speed

### /ping showing in timeline  
- Root cause: handler.go was broadcasting ALL user input, including /ping
- Fix: Skip storing /ping and /context to message channel
- They're system commands, not user messages

### Files Changed
- `server/handler.go` - Skip broadcasting /ping input
- `client/web/malten.js` - Adaptive ping (v62)

## Session: Scrolling & API Rate Limiting (Jan 3 2026)

### Issue 1: Commands not scrolling on mobile
- `/steps`, `/debug`, `/places` and other local commands didn't scroll to bottom
- Fix: `displaySystemMessage()` now always calls `scrollToBottom()` unless `skipScroll` param is true
- Removed redundant `scrollToBottom()` calls after `displaySystemMessage()` in `/places`
- JS version bumped to 67

### Issue 2: TfL API hammering
- Problem: 25 agents each calling TfL every 30 seconds = rate limited (429)
- Solution 1: Added `APIRateLimiter` in `spatial/ratelimit.go`
  - Serializes all external API calls through a global lock
  - Minimum 2 seconds between any TfL calls
  - All TfL calls now wrapped in `TfLRateLimitedCall()`
- Solution 2: Geographic filter - TfL only works for Greater London (51.2-51.7 lat, -0.6-0.3 lon)
  - Non-London agents no longer waste API calls on TfL

### Issue 3: Agents not recovering on restart
- Problem: `spatial.Get()` was lazy-loaded, agents weren't starting
- Fix: Call `spatial.Get()` explicitly in `main.go` at startup
- Added logging: "Recovering N agents" on startup

### Files Changed
- `client/web/malten.js` - `displaySystemMessage()` now auto-scrolls (v67)
- `client/web/index.html` - version bump
- `spatial/ratelimit.go` - New file: API rate limiter
- `spatial/live.go` - All TfL calls wrapped in rate limiter, London geo-filter added
- `spatial/agent.go` - Added recovery logging
- `main.go` - Explicit `spatial.Get()` call at startup

### Rate Limiter Usage
```go
// For TfL calls
err := TfLRateLimitedCall(func() error {
    resp, err = httpClient.Do(req)
    return err
})

// For OSM calls (future)
err := OSMRateLimitedCall(func() error {
    // ...
})

// For any external API
err := RateLimitedCall("api-name", func() error {
    // ...
})
```

## Regional Data Sources

Agents are now region-aware. See `spatial/regions.go` for full implementation.

### Current Status

| Region     | Transport API | Status      | Weather | Prayer | POIs |
|------------|---------------|-------------|---------|--------|------|
| London     | TfL           | ✓ Working   | ✓       | ✓      | ✓    |
| Manchester | TfGM          | TODO        | ✓       | ✓      | ✓    |
| Edinburgh  | Edinburgh Trams| TODO       | ✓       | ✓      | ✓    |
| Cardiff    | Transport Wales| TODO       | ✓       | ✓      | ✓    |
| Dublin     | Dublin Bus/Rail| TODO       | ✓       | ✓      | ✓    |
| Other UK   | National Rail | TODO        | ✓       | ✓      | ✓    |
| France     | SNCF          | TODO        | ✓       | ✓      | ✓    |
| USA        | Various       | TODO        | ✓       | ✓      | ✓    |
| Global     | None          | -           | ✓       | ✓      | ✓    |

### Transport APIs to Integrate

**UK:**
- TfGM (Manchester): https://api.tfgm.com/
- TfW (Wales): https://api.transport.wales/
- National Rail: https://opendata.nationalrail.co.uk/
- Edinburgh Trams: https://tfeapidocs.edinburgh.gov.uk/

**Ireland:**
- Irish Rail: https://api.irishrail.ie/realtime/
- Dublin Bus: https://data.dublinbus.ie/

**Europe:**
- SNCF (France): https://data.sncf.com/
- Deutsche Bahn: https://developers.deutschebahn.com/

### How It Works

1. `GetRegion(lat, lon)` returns the region for coordinates
2. Transport fetch functions check region before calling APIs
3. Non-London agents skip TfL calls entirely
4. All regions get weather, prayer times, and POIs from global APIs

### Adding a New Transport API

1. Add region to `regions.go` with bounding box
2. Create fetch function in `live.go` (e.g., `fetchTfGMArrivals`)
3. Update `updateLiveData` in `agent.go` to call based on region
4. Wrap in `RateLimitedCall("api-name", ...)` for rate limiting

## Purpose & Roadmap: Signs in the World

Malten isn't Foursquare with AI. It's a window to the signs (ayat) around you.

### The Vision

The world is filled with signs of Allah. Technology should help us see them, not distract from them. Every moment is purposeful. The app should remind you why you're here, subtly, while helping with the mundane.

### What We're Building

**Phase 1: The Reminder** (integrate reminder.dev) ✅ DONE

| Feature | When it appears | Source | Status |
|---------|-----------------|--------|--------|
| Daily verse | First open of the day | `/api/daily` → verse field | ✅ |
| Duha reminder | 10:00-11:30am | `/api/quran/93` (Ad-Duhaa) | ✅ |
| Name of Allah | With daily verse | `/api/daily` → name field | TODO |
| Hijri date | In context bar | `/api/daily` → hijri field | TODO |

Implementation:
- Daily reminder fetched from `/api/daily`, cached per day
- Duha reminder fetched from `/api/quran/93` during Duha time (10:00-11:30am)
- Subtle card style: light gray background, left border, just verse + reference
- State tracks `reminderDate` and `duhaReminderDate` to show each once per day
- `/reminder` command for manual access, `/reminder duha` for Duha specifically

Reminder.dev API:
- `/api/daily` - Today's verse, hadith, name of Allah, hijri date
- `/api/quran/{number}` - Full surah with all verses
- `/api/search` - Search verses (TODO: integrate)

**Phase 2: The Natural World**

| Feature | When it appears | Data source |
|---------|-----------------|-------------|
| Sun times | In context | Calculate from lat/lon |
| Moon phase | In context | Calculate or API |
| Weather as sign | With weather | Already have, reframe |

Implementation:
- Add sunrise/sunset to context: "☀️ Rises 07:52 · Sets 16:23"
- Add moon phase: "🌒 Waxing crescent"
- Weather already there, keep it factual

**Phase 3: The Built World**

| Feature | When it appears | Data source |
|---------|-----------------|-------------|
| Mosques nearby | In places | OSM amenity=place_of_worship + religion=muslim |
| Historic sites | On request | OSM historic=* |
| Age of places | With place info | OSM or Wikipedia |

Implementation:
- Already index mosques via OSM
- Add `/mosques` or `/nearby mosque` command
- Show mosque direction and distance
- Historic sites: query OSM for historic=* tag

**Phase 4: Journey Context**

| Feature | When it appears | Trigger |
|---------|-----------------|---------|  
| Route suggestions | When moving with direction | Speed + heading consistent |
| "On your way" | During journey | Detect destination from pattern |
| Mosque en route | During journey | If prayer time approaching |

Implementation:
- Detect journey: consistent movement >5 min
- Ask "Where are you headed?" or infer from direction
- Surface relevant stops: "Mosque 200m off route, Dhuhr in 20 min"

**Phase 5: Share with Family**

| Feature | How it works |
|---------|--------------|
| Share location | Generate link, family sees your position |
| Safety check-in | "I've arrived" notification |
| Group coordination | See each other on map |

Implementation:
- `/share` generates temporary link
- Link shows read-only map with your position
- Optional: WebSocket for live updates
- No accounts needed - link-based sharing

### What We're NOT Building

- Creepy pattern learning ("you usually...")
- Gamification (points, badges, streaks)
- Social features beyond family sharing
- Predictive suggestions without asking
- Anything that treats you as a data point

### Concrete Next Steps

1. **Today**: Integrate reminder.dev daily verse
   - Fetch on first open
   - Show verse card with Name of Allah
   - Add Hijri date to context

2. **Next**: Sun/moon in context
   - Sunrise/sunset times
   - Moon phase
   - Keep it factual, not preachy

3. **Then**: Mosques
   - Already in OSM index
   - `/mosque` command for nearest
   - Show in context if prayer approaching

### UI Treatment

The reminder should be:
- **Subtle** - not a popup, just a card in your timeline
- **First** - appears before the mundane context
- **Linked** - tap to read more on reminder.dev
- **Daily** - changes each day, not random

Example first-open experience:
```
📖 14 Rajab 1447

"So endure with beautiful patience."
— Al-Ma'arij 70:5

Al Baa'ith - The Resurrector
```

Then the normal context:
```
📍 Hampton, TW12 2LL
☀️ 3°C · Rises 07:52 · Sets 16:23
🌒 Waxing crescent
🕌 Asr now · Maghrib 16:05
```

### The Principle

Utility over creep. Signs over stats. Purpose over points.

## Session: Jan 4, 2026 - Reminders & Timeline Refactor

### Prayer-Time Reminders (DONE)
- Daily reminder on first open (from reminder.dev `/api/daily`)
- Prayer-time reminders triggered by `prayer.current` in context:
  - Fajr → Al-Fajr (89:1-4)
  - Duha → Ad-Duhaa (93:1-3) - shows 8am-12pm
  - Dhuhr → Ar-Razzaq (The Provider)
  - Asr → Al-Musawwir (The Shaper of Beauty)
  - Maghrib → An-Noor (The Light)
  - Isha → Al-Layl (92:1-4)
- `state.prayerReminders` tracks which shown today
- `/reminder [type]` command for manual access

### Timeline Refactor (DONE)
ONE path for everything:
```javascript
addToTimeline(text, type)   // Save + render - THE way to add anything
loadTimeline()              // Load from localStorage on startup
renderTimelineItem(item)    // Render single item to DOM
getTimelineType(text)       // Determine type from emoji
```

NEVER:
- Render directly to DOM without saving
- Create new display functions
- Bypass addToTimeline()

### Show Malten's Work (DONE)
- `📡 Acquiring location...` on first load
- `📍 Location` + `🤖 Agent status · X places nearby` on first context
- `🕐 Time · Location · Temp · Next Prayer` on every app reopen (showPresence)
- Agent info in `/ping` response: `{ id, status, poi_count, last_index }`

### Code Cleanup (DONE)
Removed ~200 lines:
- Old display functions (displaySystemMessage, displayQACard, etc.)
- Unused variables (streamUrl, maxChars, streams)
- Dead functions (loadMessages, loadStream, parseDate, etc.)

### Known Issue
Mobile PWA may cache old JS. Fix: Clear app cache or `/clear` command.

## Session: Jan 4, 2026 - Movement Feedback

### User Feedback
- App feels dead between updates
- No visual feedback on movement (unlike Google Maps)
- Updates too slow/sparse
- Need perceptual feedback that reflects reality
- Area knowledge not surfaced ("what do you know about Whitton?")

### Changes Made

**Movement Tracker (v72)**
- New `movementTracker` object tracks distance traveled since last context update
- Calculates speed to determine mode: walking, driving, stationary
- Heading calculation from location history (→N, →SE, etc.)
- Heartbeat messages every 60s while moving: "🚶 142m →NE"
- Movement cards styled subtle (no border, small text, no timestamp)

**Area Change Acknowledgment**
- When geohash changes (new stream), shows "📍 Entered [street name]"
- Movement tracker resets on area change

**Step Counter in Context**
- Shows daily step count in context summary: "🚶 2,340"
- Steps from accelerometer via stepDetector

**Improved Presence on Reopen**
- `showPresence()` now shows time since last update
- Includes step count if any today
- Shows even without cached context

**Fixed First-Open Flow**
- When permission is 'prompt', show "Acquiring..." not "Enable location"
- Enable button only shown when permission is denied
- New `showAcquiring()` function for clearer UX

### Files Changed
- `client/web/malten.js` - Movement tracker, presence, first-open flow (v72)
- `client/web/malten.css` - Movement card styling (v27)
- `client/web/index.html` - Version bumps
- `agent/agent.go` - Added ChatNoContext function (unused for now)

### What's Still TODO
- Area knowledge ("what do you know about Whitton?") - blocked by import cycle
- More frequent heartbeat when moving fast
- Visual indicator (pulsing dot?) when motion detected
- Map/network view for spatial visualization

## Session: Jan 4, 2026 - Simplification

### The Problem
Too much complexity scattered across client and server:
- Client had `detectChanges` with ad-hoc rules for filtering updates
- Multiple endpoints for similar things (`/events`, `/streams`, `/commands`)
- Deduplication logic that broke command output

### The Solution

**1. Client is dumb, server is smart**
- Removed `detectChanges` and all client-side filtering
- Server now tracks session context and pushes meaningful changes
- `addToTimeline()` is now truly simple - just save and render
- Client renders what it receives, nothing more

**2. Unified `/streams` endpoint**
```
GET  /streams              - list all streams
GET  /streams?stream=xxx   - WebSocket upgrade: stream events
POST /streams              - send command/message
```

Old endpoints still work for backwards compatibility:
- `/events` - still works (same as GET /streams with WebSocket)
- `/commands` - still works (same as POST /streams)

**3. Server-side change detection**
New `spatial/changes.go`:
- Tracks last context per session
- `DetectChanges()` compares old vs new context
- Only pushes when something meaningful changes:
  - Location (street level)
  - Prayer time change
  - Rain warning (new)
  - Temperature change (>3°)
- Messages pushed via WebSocket to user's channel

### Files Changed
- `client/web/malten.js` - Removed detectChanges, hasRecentCard, extractPrayer
- `spatial/changes.go` - New: server-side change detection
- `command/nearby.go` - handlePing uses GetContextWithChanges
- `command/command.go` - Added PushMessages to Context
- `server/handler.go` - Push change messages after command dispatch
- `main.go` - Unified /streams endpoint

### The Model Now

```
Client:
  - Sends commands to /streams (POST)
  - Receives messages via /streams (WebSocket)
  - Renders messages to timeline
  - Saves timeline to localStorage
  - That's it.

Server:
  - Receives commands
  - Tracks context per session
  - Decides what's worth pushing
  - Pushes meaningful changes via WebSocket
```

### What's Still TODO
- `/messages` endpoint could merge into `/streams?history=true`
- `/agents` stays separate (infrastructure CRUD)
- Consider removing old `/events` and `/commands` endpoints after client migration

## Push Notifications (Jan 4, 2026)

### Purpose
Background updates when app is backgrounded - bus times, prayer times, weather alerts.

### Flow
1. User enables notifications via button in context card
2. Browser prompts for permission
3. Subscription sent to `/push/subscribe`
4. Server tracks user's last known location
5. When user hasn't pinged in 2-30 min (backgrounded), server pushes updates
6. Push includes: bus times, prayer approaching, rain warnings

### Rate Limiting
- Max 1 push per 5 minutes per user
- Quiet hours: 10pm-7am (local time, estimated from longitude)
- No push if user was active in last 2 min (app is open)
- No push after 30 min of inactivity (user has left the area)

### Files
- `server/push.go` - Push manager, subscription handling, background loop
- `client/web/sw.js` - Service worker to receive push events
- `client/web/malten.js` - Push subscription UI (`subscribePush()`, `unsubscribePush()`)

### Endpoints
- `GET /push/vapid-key` - Get VAPID public key for subscription
- `POST /push/subscribe` - Save push subscription
- `POST /push/unsubscribe` - Remove push subscription

### Environment
- `VAPID_PUBLIC_KEY` - Public key for web push
- `VAPID_PRIVATE_KEY` - Private key for signing

### What Gets Pushed
1. **Bus times** (priority) - "🚌 Hampton Station: 111 → Kingston in 3m"
2. **Prayer approaching** - "🕌 Maghrib at 16:05"
3. **Rain warning** - "🌧️ Rain starting in 20 min"

### Callback Pattern
To avoid import cycles between server and spatial:
- `server.SetNotificationBuilder(func)` sets callback in main.go
- `command.UpdatePushLocation` is a var set by main.go
- This allows server to build notifications using spatial data

## Session: Push Notifications (Jan 4, 2026 - Evening)

### What Was Built
Complete web push notification system for background updates.

**Server-side (`server/push.go`):**
- `PushManager` singleton manages subscriptions
- Persists to `push_subscriptions.json` (survives restarts)
- Background loop every 1 minute:
  - Checks backgrounded users (2-30 min since last ping)
  - Checks scheduled notifications (morning weather, Duha, prayer reminders)
- Rate limiting: max 1 push per 5 min, quiet hours 10pm-7am

**Client-side:**
- Service worker (`sw.js`) receives push events
- `subscribePush()` / `unsubscribePush()` in malten.js
- Button in context card: "🔔 Enable notifications"
- State persisted via browser's push subscription (checked on load)

**Notification types:**
| Type | When | Content |
|------|------|---------|
| Bus times | Backgrounded 2-30 min | Fresh arrivals |
| Morning weather | 7am local | Weather + rain warning |
| Ad-Duha | 10am local | Surah 93:1-2 |
| Prayer reminder | 10 min before | "Dhuhr in 10 minutes" |

### Files Changed
- `server/push.go` - New: complete push system
- `client/web/sw.js` - New: service worker
- `client/web/malten.js` - Push subscription UI, removed SW kill code
- `client/web/malten.css` - Notification button styling
- `main.go` - Push routes, notification builders
- `command/nearby.go` - UpdatePushLocation callback
- `.env` - VAPID keys added

### Environment Variables
```
VAPID_PUBLIC_KEY=...
VAPID_PRIVATE_KEY=...
```

### Other Fixes
- Foreground refresh: stationary >1min refreshes (bus stop), moving >2min
- Timeline deduplication: same text within 60s skipped
- Check-ins persist in localStorage (already worked, verified)

## Session: Jan 5, 2026 - Timeline Refactor & Cleanup

### Timeline Storage Refactor
- **Renamed** `state.cards` → `state.timeline` 
- **Removed** `state.conversation` - was duplicate storage
- **Unified** all messages (user, assistant, system) in `state.timeline` with `type` field
- **Migration** handles old localStorage keys automatically

### Fixed: Old Messages Wrong Timestamps
**Problem:** User chats with AI, refreshes, messages show "6 hours ago"
**Cause:** `state.conversation` used single timestamp for all messages
**Fix:** Each message has its own timestamp. Server messages deduplicated by text match.

### Fixed: -0°C Display
**Problem:** Temperature showing `-0°C`
**Cause:** Old cached weather entities had `-0°C` in Name field
**Fix:** Context recalculates display string from raw `temp_c` with proper rounding

### Removed Dead Code
- `seenNewsUrls`, `hasSeenNews()`, `markNewsSeen()` - never used

### Memory Usage (typical)
- Process: ~150-200 MB
- spatial.json: ~11 MB on disk
- events.jsonl: ~80 MB, ~500K events (not loaded into memory)
- Quadtree expands in memory due to Go data structures

### Files Changed
- `client/web/malten.js` - timeline refactor, removed conversation
- `spatial/context.go` - recalculate weather display from raw temp
- `spatial/live.go` - proper temp rounding
- `claude.md` - updated docs
