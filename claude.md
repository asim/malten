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

A spatial timeline for the real world. Context-aware of where you are and what's around you.

### The Public Face

To the world, Malten is:
- A timeline of your movement through space
- Contextual awareness - weather, transport, places
- An alternative to algorithmic feeds
- Mindfulness through noticing the world
- No ads, no tracking, no manipulation

### The Internal Purpose

Internally, Malten is built to bring people back.

Not through preaching. Through pointing at the world and saying *look*.

The world is full of signs (ayat). The snow falling. The sunrise. The precision of everything. For those with eyes to see, these signs point to the Creator. For others, they're simply beautiful. Both are valid entry points.

Malten is the outermost circle of three tools:

| Tool | Audience | Method | The return |
|------|----------|--------|------------|
| **Reminder** | Muslims, practicing | Direct - Quran, Hadith, Names of Allah | Through revelation |
| **Mu** | Engineers, screen-addicts | Subtle - a verse card among the noise | Through interruption |
| **Malten** | Everyone, Western, English-speaking | Very subtle - reflection on the world itself | Through creation |

Reminder is for those already close. Mu is for those who might be open but are lost. Malten is for those who don't even know they're being called back.

### Design Principles

1. **Subtle, not preachy** - No Arabic terms in the UI for non-Muslim audiences. "The Provider" not "Ar-Razzaq". "Morning light" not "Ad-Duha". The meaning without the barrier.

2. **Universal accessibility** - Works for everyone. A Muslim sees prayer times and knows. A non-Muslim sees sunrise/sunset and finds it beautiful. Both are experiencing the same reality.

3. **Signs in the horizons and within themselves** (41:53) - The external world (weather, places, moments) and the internal reflection (your timeline, your path). Both paths to the same truth.

4. **The tool helps you see, it doesn't tell you what to see** - We surface the world. We don't interpret it for you. Those who want to see deeper will. Those who just want bus times get bus times.

5. **Utility and purpose together** - Every feature has practical value. The spiritual dimension is woven in, not bolted on.

### What This Means Practically

- Prayer times are presented as "rhythm of the day" - useful for anyone, meaningful for Muslims
- Weather includes sunrise/sunset prominently - natural markers of time
- The timeline is your "worldline" - a term that works spiritually and scientifically
- Reminders are optional and subtle - a verse card, not a popup
- Names of Allah appear contextually - "The Light" at sunset, "The Provider" at noon
- No forced religious content - it's there for those who look

### The Verse That Guides This

> "We will show them Our signs in the universe and within themselves until it becomes clear to them that this is the truth." (41:53)

The signs are already there. In the horizons (the world around you) and within yourself (your reflection on it). Malten just helps you notice.

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
- `spatial.json` - The spatial index. Contains all cached POIs, agents, streets, weather, prayer times, transport data
- `events.jsonl` - Event log for replay
- `data/*.json` - Supplementary data
- `backups/` - Automated backups (can regenerate, but saves time)

### Backups

Automated backups run every 6 hours via cron:

```bash
# Cron job
0 */6 * * * /home/exedev/malten/scripts/backup.sh

# Manual backup
/home/exedev/malten/scripts/backup.sh

# Restore from backup
gunzip -c backups/spatial_YYYYMMDD_HHMMSS.json.gz > spatial.json
sudo systemctl restart malten
```

| Setting | Value |
|---------|-------|
| Frequency | Every 6 hours |
| Retention | 7 days (28 backups) |
| Location | `/home/exedev/malten/backups/` |
| Files | `spatial_*.json.gz`, `events_*.jsonl.gz` |
| Compressed size | ~16MB per backup |

**Why backups matter:**
- Street indexing via OSRM is expensive (rate-limited API calls)
- 29 streets = ~30 OSRM calls at 2s each = 1 minute of API time
- POI indexing via OSM Overpass is also rate-limited
- Rebuilding from scratch would take hours

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

## Monetization Reality

### The Hard Truth

Monetization has never stuck. History:
- **Corporate sponsorship** (go-micro) - Paying for labour, not product
- **VC funding** (Micro) - Paying for potential upside, burned out
- **Paid APIs** (M3O) - Made sense technically, didn't scale
- **Crypto investments** - Currently paying the bills, not sustainable strategy

30 years of ad-supported "free" software trained consumers not to pay. But ads are surveillance. The model is broken.

### What Others Do

**Kagi** (private search):
- Launched on HackerNews
- Founder-funded initially
- Crowdfunding
- Now profitable
- Power users pay for privacy

**Spotify** - People pay for music streaming
**Instagram** - People won't pay for social feeds

Difference: Spotify provides content they can't get elsewhere. Instagram is "free" because you are the product.

### Malten's Position

We're closer to Kagi than Instagram:
- Utility tool, not social network
- Privacy-respecting
- Power user appeal (developers, privacy-conscious)
- Can charge for enhanced features

But also different:
- Spatial/local - harder to demonstrate value before use
- Competing with "free" Google Maps
- Muslim-friendly features are niche value-add

### Current Thinking

**Free Tier (generous)**
- All core features (context, directions, nearby, weather, transport)
- Local timeline storage
- Basic reminders
- Export/import

**Pro Tier (~£3/month or £25/year)**
- Cloud backup & sync across devices
- Extended timeline history
- Saved places sync
- Priority API responses
- Timeline sharing with family

**API Access (for developers)**
- Spatial index queries
- Contextual AI
- Pay-per-use or subscription

**NOT doing:**
- Ads (ever)
- Selling data
- "Promoted" places (feels like ads)
- Enterprise sales (hate it)

### The Honest Uncertainty

Will people pay £3/month for this? Unknown.

Kagi proves some people will pay for privacy and utility. But Kagi is search - used dozens of times daily. Malten might be opened once or twice.

Maybe the answer is:
1. Build something genuinely useful first
2. Let it spread organically
3. Offer Pro tier when there's demand
4. Trust that provision comes from elsewhere

The reminder.dev model is clear: "We do not ask for a reward. Our reward is with Allah."

Malten can't be that pure - it's a utility, not revelation. But it also doesn't need to be venture-scale. Sustainable is enough.

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

### Pricing
- £2.99/month or £24.99/year
- Family plan (5 devices) - £4.99/month
- One-time purchase option? (Maybe £50-75 lifetime)

### What We Don't Do
- Sell personal data
- Surveillance/tracking
- Ads
- Enterprise sales
- Compromise on privacy for revenue

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

## Regional Couriers (Street Mapping)

Multiple couriers operate in parallel to map streets in different regions.

### How It Works

1. **Clustering**: Agents within 50km are grouped into clusters
2. **One courier per cluster**: Each cluster gets its own courier
3. **Independent operation**: Couriers walk between agents in their region
4. **Street indexing**: Routes are stored as street geometry for the map

### Commands

- `/couriers` - Show status of all regional couriers
- `/couriers on` - Enable all regional couriers
- `/couriers off` - Pause all regional couriers
- `/courier` - Original single courier (backward compat)

### Files

- `spatial/courier_regional.go` - Regional courier manager
- `spatial/courier.go` - Original single courier
- `regional_couriers.json` - Persisted state

## Regional Data Sources

Agents are region-aware. Transport APIs only work in specific regions.

### Current Status

| Region     | Transport API | Status      | Weather | Prayer | POIs | Streets |
|------------|---------------|-------------|---------|--------|------|--------|
| London     | TfL           | ✓ Working   | ✓       | ✓      | ✓    | ✓ Active |
| Manchester | TfGM          | TODO        | ✓       | ✓      | ✓    | TODO |
| Edinburgh  | Edinburgh Trams| TODO       | ✓       | ✓      | ✓    | TODO |
| Cardiff    | Transport Wales| TODO       | ✓       | ✓      | ✓    | ✓ Active |
| Dublin     | Dublin Bus/Rail| TODO       | ✓       | ✓      | ✓    | ✓ Active |
| USA        | GTFS feeds    | TODO        | ✓       | ✓      | ✓    | Partial |
| Argentina  | BA transit    | TODO        | ✓       | ✓      | ✓    | TODO |
| India      | Various       | TODO        | ✓       | ✓      | ✓    | TODO |
| Global     | None          | -           | ✓       | ✓      | ✓    | Via courier |

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

### Cleanup Continued

**Removed unused code:**
- `spatial/vibe.go` - 230 lines of atmosphere inference (never used)
- `server/place.go` - PlaceHandler endpoint (never wired)
- `server/location.go` - HandlePingCommand, HandleNearbyCommand (replaced by command registry)
- `server/handler.go` - sendToSession, detectNearbyQuery, NewStreamHandler (dead code)

**Added:**
- `/status` command shows server memory, entity counts, uptime
- `/debug` shows both client state and server status

**Current codebase:**
- `malten.js`: 2,665 lines
- `malten.css`: 323 lines  
- Go server: ~8,900 lines across 34 files
- Memory: ~140 MB typical
- Entities: ~12,000 (places, weather, prayer, arrivals, agents)

### Endpoint Simplification

**Before:**
- `GET /commands` - static help text
- `GET /commands/meta` - command metadata JSON
- `/status` command - server status

**After:**
- `GET /commands` - command metadata JSON (merged)
- `GET /debug` - server memory, entities, uptime (internal endpoint)
- `/debug` client command - shows both client state + fetches server /debug

**Endpoints (current):**
| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/` | GET | Static files (PWA) |
| `/commands` | GET | Command metadata JSON |
| `/commands` | POST | Execute command |
| `/events` | GET | WebSocket for real-time updates |
| `/streams` | GET/POST | Stream management |
| `/messages` | GET | Stream message history |
| `/agents` | GET/POST/DELETE | Agent CRUD |
| `/debug` | GET | Server status (memory, entities, uptime) |
| `/push/*` | various | Push notification subscription |

**Removed:**
- `/commands/meta` - merged into GET /commands
- `/status` command - replaced by /debug endpoint
- `command/status.go` - deleted

### Current Stats
- JS: 2,670 lines
- CSS: 323 lines
- Go: ~9,100 lines across 33 files
- Memory: ~85 MB typical
- Entities: ~12,000

## Session: Jan 5, 2026 - Push History, Map View, Pro

### Push Notifications in Timeline
- Push notifications now stored in `PushHistory` on server
- New endpoint `GET /push/history` returns recent push notifications
- Client fetches push history when app becomes visible (visibilitychange)
- Push notifications appear in timeline with 🔔 emoji
- Styled with blue left border and light blue background

### Spatial Map View
- New `GET /map` endpoint returns all spatial data as JSON
- New `map.html` - canvas-based visualization of spatial index
- Features:
  - **Grid overlay** - 1km squares (adjusts with zoom: 100m, 500m, 1km, 5km)
  - **Scale bar** - shows actual distance (50m to 5km)
  - **Emoji POIs** - ☕ cafe, 🍴 restaurant, 🏪 shop, 🚏 transport, 🏥 health, 🎦 entertainment
  - **Agent markers** - 🤖 with labels and coverage circles
  - **User location** - blue dot with accuracy circle
  - **Center-on-user** button (◎) - gets GPS and centers map
  - **Pan/zoom** - mouse drag, scroll wheel, touch gestures
  - **Tooltips** - hover to see place/agent names
  - **Stats** - agents, total places, visible places, scale (m/px)
- Meters-per-pixel based zoom (1-500 m/px range)
- `/map` command shows map stats and link

### Pro Membership
- `/pro` command shows pricing information
- Free tier: all current features
- Pro tier: £2.99/month or £24.99/year
  - Cloud backup & sync
  - Saved places on all devices
  - Step history & stats
  - Timeline sharing
  - Priority support
- Family tier: £4.99/month for up to 5 people

### Agent Efficiency
- Added random jitter to agent loops (0-30s initial delay, 0-5s ongoing)
- Prevents all agents from updating simultaneously
- Rate limiting already in place: 2s between TfL calls
- Fresh cache check prevents redundant API calls

### Timeline Deduplication
- Improved from 60s to 5min window
- Checks all recent cards, not just the last one
- Prevents duplicate push notifications and repeated messages

### Entity Counting
- `CountByAgentID()` method added to DB
- `/agents` command now shows accurate place/stop counts per agent

### Files Changed
- `server/push.go` - Push history storage and retrieval
- `server/map.go` - New map data endpoint
- `client/web/map.html` - New canvas map view
- `client/web/malten.js` - Push history fetch, improved deduplication (v256)
- `client/web/malten.css` - Notification card styling (v27)
- `command/pro.go` - New Pro info command
- `command/map.go` - New map info command
- `command/agents.go` - Entity counting fix
- `spatial/db.go` - CountByAgentID method
- `spatial/agent.go` - Jittered agent loops
- `main.go` - New routes (/map, /push/history)

### Endpoints Added
| Endpoint | Method | Purpose |
|----------|--------|----------|
| `/map` | GET | Spatial data JSON for map view |
| `/push/history` | GET | Recent push notifications for timeline |

### Street Indexing via OSRM
- New `/streets` command triggers street indexing for an area
- Uses OSRM to fetch walking routes between agent location and nearby POIs
- Route geometry (GPS coordinates) stored as street entities
- Streets rendered as lines on the map
- Builds a street network over time without needing map tiles

**How it works:**
1. Agent identifies nearby POIs (200m+ away)
2. Fetches walking route from agent center to each POI via OSRM
3. Route geometry (array of [lon, lat] points) stored in spatial DB
4. Map renders streets as connected lines

**Files:**
- `spatial/streets.go` - Street fetching and indexing
- `spatial/entity.go` - EntityStreet type added
- `command/streets.go` - /streets command
- `server/map.go` - MapStreet struct and query
- `client/web/map.html` - Street rendering

**Rate limiting:**
- OSRM calls rate-limited (2s between calls)
- /streets indexes 5 routes immediately, rest in background
- Existing routes are skipped (deduplication by to_name)

**Cost:**
- Each street = 1 OSRM API call (~2s with rate limiting)
- 29 streets indexed so far
- Data is expensive to rebuild - backups are critical

## Session: Jan 5, 2026 - Movement & Check-in Fixes

### Issues Reported
1. **Initial street detection wrong** - First load showed wrong street (Broad Lane vs Gloucester Road)
2. **Speed detection slow** - Moving detected but mode (walking/driving) not immediately correct
3. **Street change not notified** - Changed streets but no notification
4. **No arrival notification** - Arrived at Sainsbury's, moved a lot, but no check-in prompt
5. **Phone sleeping** - Had to keep phone awake manually while driving

### Fixes Implemented

**Screen Wake Lock API**
- New `/wakelock on` and `/wakelock off` commands
- Uses Screen Wake Lock API to prevent phone from sleeping
- Auto-reacquires lock when app becomes visible (lock is released when page hidden)
- Wake lock object in `malten.js` with `acquire()`, `release()`, `reacquire()` methods

**Arrival Detection (new)**
- Previously only checked for "stuck GPS" (indoor drift)
- Now detects arrival: was moving (>5 m/s), now stopped (<2 m/s), near a POI (<100m)
- Sends arrival prompt: "📍 Arrived at Sainsbury's" with check-in suggestion
- `SetLocation()` now returns `LocationUpdate` struct with both check-in and arrival info
- `detectArrival()` function analyzes location history for speed changes

**Inline Location Fetch (first request)**
- Previously: first /ping had no location if not cached (waited for agent)
- Now: `GetContextData()` fetches location inline if cache is empty
- First response always has street name

### Files Changed
- `client/web/malten.js` - Wake lock API, v258
- `command/nearby.go` - `LocationUpdate` struct, `detectArrival()` function
- `server/handler.go` - Handle arrival prompts
- `server/location.go` - `sendArrivalPrompt()` function
- `spatial/context.go` - Inline location fetch when cache empty

### New Commands
- `/wakelock on` - Enable screen wake lock (prevents sleep)
- `/wakelock off` - Disable screen wake lock

### Detection Thresholds
```go
arrivalSpeedThreshold = 2.0   // m/s - below this is "stopped"
arrivalMovingThreshold = 5.0  // m/s - above this is "was moving"
arrivalPOIRadius = 100.0      // meters - look for POIs within this
locationHistorySize = 20      // Keep 20 points for ~60s of history
```

### GPS Accuracy-Based Street Filtering
- Street change notifications suppressed when:
  - GPS accuracy > 50m AND speed > 5 m/s (driving with poor GPS)
- This prevents false street notifications when GPS is bouncing while driving
- Accuracy sent from client with each ping
- Speed calculated from location history

### Reminder Deduplication Fix
- Daily and prayer reminders now mark as shown BEFORE async fetch
- Prevents duplicate reminders when app triggers multiple context updates quickly

## Session: Jan 5, 2026 - Event Model Refactor & Turn Detection

### Event Model Fixed
The core issue was treating location as an event rather than state. Now:

**State (visible in context card, NOT pushed to timeline):**
- 📍 Location/street name
- 🌡️ Temperature
- 🕌 Current prayer

**Events (pushed to timeline):**
- 🕌 Prayer time changed
- 🌧️ Rain warning
- 📍 Arrived at [POI] (from arrival detection)
- ↪️/↩️ Turned right/left (from turn detection)

No more duplicate "📍 Milton Road" notifications - location is always visible in context card.

### Turn Detection
New `turnTracker` in client detects significant direction changes:
- Accumulates heading changes from GPS movement
- Emits "↪️ Turned right (25m)" or "↩️ Turned left (30m)" when turn > 60°
- 30-second cooldown between turn events
- Uses `calculateBearingDegrees()` for raw heading

### Map Heading/Direction
Map view now shows:
- Blue direction arrow when heading known
- Direction cone when moving (longer cone = faster)
- Uses compass (deviceorientation API) when available
- Falls back to calculated heading from GPS movement

### Reminder Fixes
- `/reminder` command now returns JSON (was plain text)
- Surah titles map added: 93 → "The Morning Hours", 92 → "The Night", etc.
- Verse text is clickable, links to reminder.dev/quran/{number}
- Deduplication: mark as shown BEFORE async fetch

### Saved Places in Context
If within 50m of a saved place (e.g., "Home"), context shows:
- "📍 Home ⭐ (Milton Road, TW12 2LL)" instead of just street name

### Files Changed
- `spatial/changes.go` - Removed location push logic, simplified
- `server/map.html` - Added heading arrow, direction cone, deviceorientation
- `client/web/malten.js` - Turn tracker, bearing functions, saved place display
- `command/reminder.go` - Returns JSON, ReminderResponse struct

### Client JS Version: 269

## Session: Jan 5, 2026 - Checkpoint Before Agentic Refactor

### Current State Summary

**What Works:**
- Context card with location, weather, prayer, bus times, places
- Arrival detection (stop at POI → prompt to check-in)
- Turn detection (↪️ Turned right)
- Map view with heading/direction arrow
- Daily reminders with Name of Allah + description
- Prayer-time reminders (Duha, Fajr, etc.) with English titles
- Saved places shown in context ("📍 Home ⭐")
- Wake lock to prevent phone sleep (`/wakelock on`)
- Push notifications for background updates

**What "Agents" Currently Do (NOT agentic):**
- Dumb background loops every 30 seconds
- Fetch: weather, prayer times, transport arrivals
- Index: POIs from OSM (once on creation)
- No LLM, no reasoning, no decisions
- Just cron jobs with geographic assignment

**Architecture:**
- 33 agents covering Greater London + other cities
- Each agent has lat/lon center + radius
- `updateLiveData()` runs every 30s per agent
- `IndexAgent()` runs once to populate POIs
- All data goes into quadtree spatial index

### Next Session: Agentic Agents

Transform agents from dumb loops to LLM-powered decision makers:

**Agent Loop:**
1. **Observe** - What's the state of my area? What's changed?
2. **Think (LLM)** - Given observations, what should I do?
3. **Act** - Execute decided actions via tools
4. **Reflect** - Did actions achieve the goal?

**Tools to expose:**
- `fetch_transport(stop_id)` - Get arrivals for a stop
- `fetch_weather(lat, lon)` - Get weather  
- `index_pois(category, radius)` - Index places from OSM
- `notify_users(message)` - Push to users in area
- `query_spatial(type, radius)` - Query spatial index
- `set_poll_interval(resource, seconds)` - Adjust polling frequency

**Agent Prompt (draft):**
```
You are a spatial agent responsible for {name} ({lat}, {lon}).

Your job: keep the spatial index fresh and relevant for users in your area.

Current state:
- Weather: {status}
- Transport: {stop_count} stops, {stale_count} stale
- POIs: {poi_count} indexed
- Users: {active_count} in area

Recent events: {event_log}

Decide what actions to take. Be efficient - don't fetch what's already fresh.
```

**Considerations:**
- Cost: LLM calls every 30s × 33 agents = expensive
- Maybe: LLM decides only when something interesting happens
- Maybe: Tiered - most agents are dumb, promote to agentic when users present
- Maybe: One "supervisor" agent coordinates dumb worker agents

### Files to Change
- `spatial/agent.go` - Agent loop, tool execution
- `agent/agent.go` - LLM integration (already has Claude client)
- New: `spatial/tools.go` - Tool definitions
- New: `spatial/agent_prompt.go` - Prompt templates

### JS Version: 270
### Go Build: Working
### Server: Running on port 9090

## Session: Jan 5, 2026 - Agentic Agents

### The Change
Agents are now truly agentic. They use an LLM loop (OODA cycle) to decide what to do.

**Before:** Dumb 30s polling loop that always fetched weather, prayer, transport.

**After:** LLM observes state, decides what needs updating, executes tools, schedules next cycle.

### How It Works

1. **Observe** - Agent gathers current state:
   - Weather: fresh/STALE (>10 min)
   - Prayer: fresh/STALE (>1 hour)
   - Transport: fresh/STALE (>5 min)
   - Active users in area
   - Queued events

2. **Orient + Decide** - LLM processes observations, outputs JSON tool call:
   ```json
   {"tool": "fetch_weather", "args": {}}
   ```

3. **Act** - Tool executes, result fed back to LLM

4. **Loop** - LLM decides next action until `done` or `set_next_cycle`

### Tools Available

| Tool | Purpose |
|------|---------|
| `fetch_weather` | Get weather for agent's area |
| `fetch_prayer` | Get prayer times |
| `fetch_transport` | Get bus/tube/rail arrivals |
| `set_next_cycle` | Schedule when to wake up next (1-60 min) |
| `done` | Signal cycle complete |

### Rate Limiting

Fanar has strict rate limits. Added `LLMRateLimitedCall()` with 500ms minimum between LLM calls globally.

### Toggle

```bash
/agentic on    # Enable LLM-based processing
/agentic off   # Return to simple polling
/agentic       # Show status
```

### Efficiency Gains

- Agents with no users set next cycle to 30-60 min (not 30s)
- Only fetches STALE data (checks before calling)
- Self-scheduling based on conditions

### Files Changed

- `spatial/agentic.go` - Core OODA loop with JSON-based tool calling
- `spatial/ratelimit.go` - Added LLM rate limiter
- `spatial/agent.go` - Modified `agentLoop` to support agentic mode
- `command/agentic.go` - Toggle command

### Check-in Prompt Fix

Also fixed: if you're near a saved place (like Home), check-in prompts are suppressed. No more "Where are you?" when you're at home.

- `client/web/malten.js` - `addToTimeline()` checks `state.nearSavedPlace()` before showing check-in prompt

### JS Version: 271

### Observability & Stats

Added system stats tracking for API calls:

```bash
/system   # Shows memory, agents, entities, and API stats
```

**Output includes:**
- Memory usage (alloc, sys, GC cycles)
- Agent count and agentic mode status
- Entity counts (places, arrivals)
- Per-API stats:
  - Call count and success rate
  - Rate limit hits
  - Consecutive errors (for backoff)
  - Last success/error times

**Rate limiting with exponential backoff:**
- Tracks consecutive errors per API
- Backoff: 1s, 2s, 4s, 8s... up to 60s max
- Resets on successful call

**Files:**
- `spatial/stats.go` - Stats tracking
- `spatial/ratelimit.go` - Rate limiting with backoff
- `command/system.go` - /system command
- `agent/agent.go` - Added Fanar stats recording

### Architecture Decision

**Deterministic loops by default.** AgenticMode is off.

The simple polling loop does the job:
- Weather stale? Fetch.
- Prayer stale? Fetch.
- Transport stale? Fetch.

No LLM needed for this. LLM is for:
- User queries (Fanar)
- Complex decisions we don't want to hardcode

`/agentic on` enables LLM-based agent processing for experimentation.

### HTTP Client Wrapper (Jan 5, 2026)

All external HTTP calls now go through a unified client with:
- Rate limiting (2s between calls per API)
- Exponential backoff on errors
- Stats tracking
- Logging

**Usage:**
```go
// API-specific functions
resp, err := TfLGet(url)
resp, err := WeatherGet(url)
resp, err := PrayerGet(url)
resp, err := LocationGet(url)
resp, err := OSRMGet(url)
resp, err := OSMGet(url)
resp, err := NewsGet(url)

// Or generic
resp, err := External.Get("api-name", url)
```

**Files:**
- `spatial/http.go` - ExternalClient with rate limiting and stats
- `spatial/ratelimit.go` - Simplified, just LLM rate limiting
- `spatial/live.go` - Uses new API-specific functions
- `spatial/routing.go` - Uses OSRMGet, LocationGet
- `spatial/streets.go` - Uses OSRMGet

**Stats tracked per API:**
- Call count
- Success/error counts
- Rate limit hits
- Consecutive errors (for backoff calculation)
- Last success/error timestamps

## Awareness System

### The Vision

"Move → it updates automatically. No searching, no typing. Just awareness."

The app should proactively surface interesting things. Not just show data when you look - tell you things you didn't know to ask about.

### Architecture

```
┌─────────────────────────────────────────────┐
│ Deterministic Agent Loop (every 30s)        │
│ - Fetch weather, prayer, transport          │
│ - Index new places                          │
│ - Detect disruptions                        │
│ - Accumulate observations → ObservationLog  │
│ - No LLM, no cost                           │
└─────────────────────────────────────────────┘
                    │
                    ▼ (every 5-10 min, or on significant event)
┌─────────────────────────────────────────────┐
│ Awareness Filter (LLM)                      │
│                                             │
│ Input:                                      │
│ - Recent observations for area              │
│ - User context (location history, patterns) │
│ - Time of day, day of week                  │
│                                             │
│ Prompt:                                     │
│ "Given these observations and user context, │
│  what's worth telling the user? Be selective│
│  - only genuinely interesting/useful things"│
│                                             │
│ Output:                                     │
│ - List of awareness items to surface        │
│ - Or empty if nothing noteworthy            │
└─────────────────────────────────────────────┘
                    │
                    ▼
┌─────────────────────────────────────────────┐
│ Delivery                                    │
│ - App open? Timeline card                   │
│ - App backgrounded? Push notification       │
│ - Dedupe against recently surfaced items    │
└─────────────────────────────────────────────┘
```

### What Makes Something "Interesting"

**Worth surfacing:**
- Weather change that affects plans (rain starting, temp drop)
- Disruption on routes user frequents
- New place on streets user walks often
- Approaching prayer time (already doing this)
- Historic/notable place user is near for first time
- Unusual situation (bus much earlier/later than normal)

**NOT worth surfacing:**
- Normal weather
- Normal bus times
- Places user has passed many times
- Routine data refreshes

### Observation Types

```go
type Observation struct {
    Time      time.Time
    Type      string    // weather_change, new_place, disruption, etc.
    AgentID   string
    Data      map[string]interface{}
    Surfaced  bool      // Already told user?
}
```

Types:
- `weather_change` - significant weather shift
- `weather_warning` - rain, storm, extreme temp
- `new_place` - newly indexed POI
- `disruption` - transport disruption
- `arrival_anomaly` - bus much earlier/later than usual
- `prayer_approaching` - 10-15 min before prayer
- `notable_nearby` - historic site, mosque, etc. user hasn't seen

### User Context

To decide what's interesting, we need to know:
- **Location history** - where does user go regularly?
- **Time patterns** - when do they commute?
- **Places seen** - what have we already told them about?
- **Preferences** - do they care about cafes? mosques? history?

Currently in localStorage (client-side):
- `locationHistory` - last 20 points
- `savedPlaces` - manually saved
- `timeline` - what we've shown them

Need server-side (for awareness filter):
- Anonymized location patterns per session
- What's been surfaced to avoid repeats

### Cost Management

**Target:** 1 LLM call per area per 5-10 minutes, not per observation.

**Batching:** Accumulate observations, process in batch.

**Skip if nothing new:** Don't call LLM if observation log is empty or only routine items.

**Tiered:**
- Areas with active users: process every 5 min
- Areas without users: process every 30 min or skip

### Implementation Plan

1. **ObservationLog** - accumulator for agent observations
2. **AwarenessFilter** - LLM prompt to filter interesting items
3. **UserContext** - track patterns server-side (anonymized)
4. **SurfacedItems** - deduplication of what user has seen
5. **Delivery** - push to timeline or notification

### Files to Create

- `spatial/awareness.go` - ObservationLog, AwarenessFilter
- `spatial/patterns.go` - User pattern tracking
- `command/awareness.go` - /awareness command to see what's been observed

### Privacy

User patterns stored per session token, not user identity. No PII. Patterns are:
- Geohash frequency (which areas visited)
- Time-of-day patterns (when active)
- NOT: exact coordinates, names, personal data

### Implementation Complete (Jan 5, 2026)

The awareness system is now implemented:

**Files created/modified:**
- `spatial/awareness.go` - ObservationLog, ProcessAwareness, observation helpers
- `spatial/agent.go` - Added processAwarenessIfDue to agent loop, observation hooks
- `command/awareness.go` - /awareness and /test-awareness commands
- `server/push.go` - PushAwarenessToArea for pushing to users in area
- `main.go` - Wired up awareness push callback
- `README.md` - Updated to reflect current architecture

**How it works:**
1. Agents accumulate observations (weather warnings, disruptions, new places)
2. Every 5-10 min (if observations pending), LLM filters what's interesting
3. Interesting items pushed to users in that area (timeline or push notification)

**Commands:**
- `/awareness` - Show pending observations
- `/awareness process` - Manually trigger LLM filter
- `/test-awareness` - Add test observations for debugging

**Cost:** ~1 LLM call per area per 5-10 min when observations pending (not per observation)

---

## Session Checkpoint: Jan 5, 2026 - Awareness System Complete

### What We Built Today

1. **Agentic Mode (experimental)** - `/agentic on/off`
   - LLM-based agent decision making
   - JSON tool calling for Fanar compatibility
   - Disabled by default (deterministic is more efficient)

2. **Observability** - `/system`
   - API stats per endpoint (calls, success rate, errors)
   - Exponential backoff on failures
   - Memory and entity counts

3. **HTTP Client Wrapper** - `spatial/http.go`
   - All external calls through unified client
   - Rate limiting, stats, logging built-in
   - API-specific functions: `TfLGet()`, `WeatherGet()`, etc.

4. **Awareness System** - `/observe`, `/trigger`
   - Agents accumulate observations (weather warnings, disruptions, new places)
   - LLM filters what's interesting (every 5-10 min)
   - Push to users in area (timeline or notification)

5. **Check-in Fix**
   - Suppressed when near saved place (Home)

### Architecture Now Matches Vision

```
README says                          Reality
─────────────────────────────────────────────────────
"Open app → instantly see"           ✓ Context card
"Move → updates automatically"       ✓ Adaptive ping (5s/10s/30s)
"Ask anything → AI answers"          ✓ Fanar + tools
"Awareness system"                   ✓ ObservationLog + LLM filter
"Filter noise"                       ✓ LLM decides what's interesting
"Push notifications"                 ✓ Bus, prayer, weather, awareness
```

### Key Files Changed

| File | Purpose |
|------|---------|
| `spatial/awareness.go` | ObservationLog, ProcessAwareness |
| `spatial/agentic.go` | OODA loop (optional) |
| `spatial/http.go` | Unified HTTP client |
| `spatial/stats.go` | API statistics |
| `spatial/ratelimit.go` | Rate limiting + backoff |
| `spatial/agent.go` | Observation hooks, awareness processing |
| `command/observe.go` | /observe, /trigger commands |
| `command/system.go` | /system command |
| `command/agentic.go` | /agentic toggle |
| `server/push.go` | PushAwarenessToArea |
| `main.go` | Wired callbacks |
| `README.md` | Updated to match reality |

### Commands Added

| Command | Description |
|---------|-------------|
| `/observe` | Show pending observations |
| `/observe process` | Run awareness filter manually |
| `/trigger` | Add test observations |
| `/system` | System stats and API health |
| `/agentic on/off` | Toggle LLM-based agents |

### Cost Model

- **Deterministic loops**: Free (just API calls)
- **Awareness filter**: ~1 LLM call per area per 5-10 min when observations pending
- **User queries**: 1 LLM call per query (Fanar)

### What's Still TODO

1. **User patterns** - Track which routes/areas user frequents for smarter filtering
2. **Dedupe surfaced items** - Don't tell user same thing twice
3. **More observation types** - Prayer approaching, notable places nearby
4. **Timeline integration** - Awareness items should appear in timeline automatically

### JS Version: 273

---

## Session Checkpoint: Jan 5, 2026 - Bus Data Fix

### Problem
Bus times would intermittently disappear from context despite being cached.

### Root Causes Found

1. **Duplicate arrival entries** - Same stop had multiple entries with different IDs because:
   - ID was generated using user's coordinates instead of stop's coordinates
   - Name included dynamic arrival times, causing ID to change on each refresh

2. **Expired data accumulation** - Old arrivals weren't being cleaned up, cluttering the spatial index

### Fixes Applied

1. **ID generation fix** (`spatial/live.go`):
   ```go
   // Before (wrong - uses user location)
   ID: GenerateID(EntityArrival, lat, lon, stopID)
   
   // After (correct - uses stop location)  
   ID: GenerateID(EntityArrival, arr.Lat, arr.Lon, stopID)
   ```

2. **Name fix** - Don't include dynamic arrivals in entity name:
   ```go
   // Before
   Name: fmt.Sprintf("%s %s: %s", icon, stop.CommonName, arrivals)
   
   // After
   Name: fmt.Sprintf("%s %s", icon, stop.CommonName)
   ```

3. **Background cleanup** (`spatial/db.go`):
   - `StartBackgroundCleanup(interval)` - runs every 5 minutes
   - `cleanupExpiredArrivals()` - only removes expired arrivals, not places/agents
   - Started in `main.go` on server startup

4. **Manual cleanup command** (`command/cleanup.go`):
   - `/cleanup` - runs in background, cleans expired + duplicates from store

### Key Insight

The quadtree doesn't need TTL support. TTL is application-specific (arrivals expire in 5min, places never). The DB wrapper handles it via `ExpiresAt` field and `QueryWithMaxAge()`.

### Files Changed
- `spatial/live.go` - ID generation fix, name fix
- `spatial/db.go` - Added `StartBackgroundCleanup`, `cleanupExpiredArrivals`
- `command/cleanup.go` - Manual cleanup command (runs in background)
- `main.go` - Start background cleanup on server start

### Testing
```bash
curl -s -X POST 'http://localhost:9090/commands' -d 'prompt=/ping&lat=51.4179&lon=-0.3706' | jq -r '.html'
# Should show bus times consistently
```


## Session: Jan 5, 2026 - Bus Time Stale Fix

### Problem
Bus times weren't decreasing over time. If fetched at "11m, 12m, 15m", they would stay at those values even 5 minutes later.

### Root Cause
Arrivals were stored with `minutes` (a point-in-time calculation) instead of absolute arrival time. When read back, the stale minute values were displayed as-is.

### Fix Applied

1. **Changed busArrival struct** (`spatial/live.go`):
   ```go
   // Before
   type busArrival struct {
       Line        string `json:"line"`
       Destination string `json:"destination"`
       Minutes     int    `json:"minutes"`
   }
   
   // After
   type busArrival struct {
       Line        string    `json:"line"`
       Destination string    `json:"destination"`
       ArrivalTime time.Time `json:"arrival_time"` // Absolute time bus arrives
   }
   ```

2. **Added MinutesUntil() method**:
   ```go
   func (b busArrival) MinutesUntil() int {
       mins := int(time.Until(b.ArrivalTime).Minutes())
       if mins < 0 {
           return 0
       }
       return mins
   }
   ```

3. **Updated all display code** to use `MinutesUntil()` instead of `Minutes`

4. **Backward compatible JSON parsing** - reads both formats:
   ```go
   if arrTimeStr, ok := amap["arrival_time"].(string); ok {
       // New format - parse and calculate
   } else if minsFloat, ok := amap["minutes"].(float64); ok {
       // Legacy format - use as-is (will be stale)
   }
   ```

### Testing
```bash
# First query
curl -s -X POST 'http://localhost:9090/commands' -d 'prompt=/bus&lat=51.4179&lon=-0.3706'
# 🚏 Priory Road: 111 → Kingston in 4m

# After 60 seconds
# 🚏 Priory Road: 111 → Kingston in 3m  ← Time decreased!
```

### Files Changed
- `spatial/live.go` - busArrival struct, MinutesUntil(), all display code

### Note
After making this change, need to either:
1. Wait for old data to expire and be refreshed, OR
2. Run `/cleanup` to force removal of old data

## Session: Jan 5, 2026 - Bus Data Type Mismatch Bug

### Problem
Bus times would work after restart, then break after the first update cycle.

### Root Cause
**Type mismatch between JSON-loaded data and runtime-inserted data.**

When data is loaded from JSON (on restart):
- `arr.Data["arrivals"]` is `[]interface{}` (JSON deserializes arrays this way)

When data is inserted at runtime:
- `arr.Data["arrivals"]` was `[]busArrival` (Go struct slice)

The reading code did:
```go
arrData, _ := arr.Data["arrivals"].([]interface{})
```

This type assertion FAILS silently when the underlying type is `[]busArrival`, returning `nil, false`. So `arrData` was empty, and "No bus times available" was shown.

### Fix
Convert `[]busArrival` to `[]interface{}` before storing:

```go
func arrivalsToInterface(arrivals []busArrival) []interface{} {
    result := make([]interface{}, len(arrivals))
    for i, a := range arrivals {
        result[i] = map[string]interface{}{
            "line":         a.Line,
            "destination":  a.Destination,
            "arrival_time": a.ArrivalTime.Format(time.RFC3339Nano),
        }
    }
    return result
}
```

Now all storage paths use `arrivalsToInterface(arrivals)` so the type is consistent.

### Key Insight
In Go, you cannot type assert `interface{}` to `[]interface{}` if the underlying type is `[]SomeStruct`. The types are different at runtime. You CAN assert back to the original type (`[]SomeStruct`), but if you need a generic interface slice, you must convert element by element.

### Not a Quadtree Bug
The quadtree was working correctly. Insert succeeded, DebugFind found the point. The issue was that the Entity's Data contained the wrong type, so the display code couldn't read it.

### Files Changed
- `spatial/live.go` - Added `arrivalsToInterface()`, use it everywhere arrivals are stored

## Session Checkpoint: Jan 5, 2026 - Entity Type Refactor

### Problem Solved This Session
Bus times would work after restart, then break after updates. Root cause: **Go type assertion mismatch**.

- JSON deserializes `[]interface{}`
- Runtime code stored `[]busArrival`  
- `.([]interface{})` silently fails on `[]busArrival`

Fixed with `arrivalsToInterface()` conversion - but this is a band-aid.

### The Real Problem
`Entity.Data` is `map[string]interface{}` - a generic bag that:
- Has no type safety
- JSON round-trip changes types
- Forces ugly runtime assertions everywhere

### The Solution: Typed Entity Data
New architecture:

```go
type Entity struct {
    ID       string
    Type     string                 // "arrival", "weather", "place", etc
    Lat, Lon float64
    Metadata map[string]interface{} // expiry, timestamps, agent_id
    Data     interface{}            // *Arrival, *Weather, *Place
}

type Arrival struct {
    StopID   string
    StopName string
    Arrivals []BusArrival  // Properly typed!
}

type Weather struct {
    TempC       float64
    Code        int
    RainWarning string
}

type Place struct {
    Name     string
    Category string
    Tags     map[string]string
}

type Prayer struct {
    Timings map[string]string
    Current string
    Next    string
}

type Street struct {
    Points [][]float64
    Length float64
    ToName string
}
```

### Query Pattern
```go
for _, e := range entities {
    switch e.Type {
    case "arrival":
        arr := e.Data.(*Arrival)
        for _, bus := range arr.Arrivals {
            // bus.Line, bus.ArrivalTime - all typed
        }
    case "weather":
        w := e.Data.(*Weather)
        // w.TempC is float64, not interface{}
    }
}
```

### JSON Backward Compatibility
Custom UnmarshalJSON that:
1. Reads Type field first
2. Deserializes Data into correct struct based on Type
3. Falls back to map[string]interface{} for unknown types (old data)

```go
func (e *Entity) UnmarshalJSON(b []byte) error {
    var raw struct {
        ID       string                 `json:"id"`
        Type     string                 `json:"type"`
        Lat      float64                `json:"lat"`
        Lon      float64                `json:"lon"`
        Metadata map[string]interface{} `json:"metadata"`
        Data     json.RawMessage        `json:"data"`
        // Legacy fields for backward compat
        OldData  map[string]interface{} `json:"data,omitempty"`
    }
    if err := json.Unmarshal(b, &raw); err != nil {
        return err
    }
    
    e.ID, e.Type, e.Lat, e.Lon, e.Metadata = raw.ID, raw.Type, raw.Lat, raw.Lon, raw.Metadata
    
    switch raw.Type {
    case "arrival":
        var arr Arrival
        if err := json.Unmarshal(raw.Data, &arr); err != nil {
            // Fallback: try legacy format
            e.Data = raw.OldData
            return nil
        }
        e.Data = &arr
    case "weather":
        var w Weather
        json.Unmarshal(raw.Data, &w)
        e.Data = &w
    // ... other types
    default:
        // Unknown type - keep as map for backward compat
        var m map[string]interface{}
        json.Unmarshal(raw.Data, &m)
        e.Data = m
    }
    return nil
}
```

### Migration Strategy
1. New Entity struct with typed Data
2. Custom JSON marshal/unmarshal for backward compat
3. Update all code that reads Entity.Data to use type switch
4. Update all code that creates entities to use typed structs
5. Old data loads fine (falls back to map), new data is typed

### Files to Change
- `spatial/entity.go` - New typed structs, custom JSON handling
- `spatial/db.go` - Query helpers that return typed data
- `spatial/live.go` - Create typed Arrival, Weather, Prayer entities
- `spatial/agent.go` - Create typed Place entities
- `spatial/streets.go` - Create typed Street entities
- `spatial/context.go` - Read typed data
- `command/*.go` - Any code reading Entity.Data

### Next Session: Start Here
1. Define new typed structs in `spatial/entity.go`
2. Add custom UnmarshalJSON with backward compat
3. Update entity creation code (one type at a time, start with Arrival)
4. Update reading code
5. Test with existing spatial.json (must load old data)

### Also Fixed This Session
- Push notification duplicates: history now cleared after fetch
- `[][]float64` for streets: added `PointsToInterface()`
- Test files updated for new ArrivalTime field

## Session: Jan 5, 2026 - Entity Type Refactor

### What Was Done
Refactored `Entity.Data` from untyped `map[string]interface{}` to typed structs for type safety.

### New Typed Data Structures

```go
// In spatial/entity.go

type ArrivalData struct {
    StopID   string       `json:"stop_id"`
    StopName string       `json:"stop_name"`
    StopType string       `json:"stop_type"`
    Arrivals []BusArrival `json:"arrivals"`
}

type WeatherData struct {
    TempC        float64 `json:"temp_c"`
    WeatherCode  int     `json:"weather_code"`
    RainForecast string  `json:"rain_forecast"`
}

type PrayerData struct {
    Timings map[string]string `json:"timings"`
    Current string            `json:"current"`
    Next    string            `json:"next"`
}

type PlaceData struct {
    Category string            `json:"category"`
    Tags     map[string]string `json:"tags"`
    AgentID  string            `json:"agent_id"`
}

type AgentEntityData struct {
    Radius    float64    `json:"radius"`
    Status    string     `json:"status"`
    POICount  int        `json:"poi_count"`
    LastIndex *time.Time `json:"last_index,omitempty"`
    LastLive  *time.Time `json:"last_live,omitempty"`
}

type StreetData struct {
    Points [][]float64 `json:"points"`
    Length float64     `json:"length"`
    ToName string      `json:"to_name"`
}
```

### How to Access Data

```go
// Use typed accessor methods (preferred)
if arrData := entity.GetArrivalData(); arrData != nil {
    for _, bus := range arrData.Arrivals {
        mins := bus.MinutesUntil()
        // ...
    }
}

if wd := entity.GetWeatherData(); wd != nil {
    temp := wd.TempC
    rain := wd.RainForecast
}

// Accessor methods handle both typed and legacy map data
```

### Backward Compatibility

- Custom `UnmarshalJSON` on `Entity` type converts legacy JSON to typed structs
- Accessor methods (`GetArrivalData()`, etc.) check for typed data first, fall back to legacy map
- Existing `spatial.json` files load correctly
- No migration needed - old data works, new data uses typed structs

### Files Changed
- `spatial/entity.go` - Typed data structs, accessor methods, custom JSON unmarshaling
- `spatial/live.go` - Use typed data for weather, prayer, arrivals
- `spatial/context.go` - Use accessor methods for weather, places
- `spatial/agent.go` - Use typed AgentEntityData
- `spatial/db.go` - Use accessor methods in filters
- `spatial/streets.go` - Use typed StreetData
- `server/map.go` - Use accessor methods
- `server/agents.go` - Use typed AgentEntityData
- `command/agents.go` - Use accessor methods
- `command/directions.go` - Use accessor methods
- `command/nearby.go` - Use accessor methods
- `spatial/live_test.go` - Updated for new types

## Session: Jan 5, 2026 - Bus Notification Toggle

### Feature Added
`/bus on|off` command to toggle bus push notifications.

### How It Works
- **Default**: Bus push notifications are OFF
- **`/bus`**: Shows bus times (always works regardless of notification setting)
- **`/bus on`**: Enables bus push notifications when app is backgrounded
- **`/bus off`**: Disables bus push notifications
- **`/bus status`**: Shows current notification setting

Bus times still appear in the context card regardless of this setting.
Only push notifications when the app is in the background are affected.

### Implementation
- `PushUser.BusNotify` field tracks preference per session
- Preference persists to `push_subscriptions.json`
- `buildPushNotification` in main.go checks the flag before sending bus notifications
- Callback pattern used to avoid import cycles between command and server packages

### Files Changed
- `server/push.go` - Added BusNotify field, SetBusNotify/GetBusNotify methods
- `command/context_questions.go` - Extended /bus command to handle on/off/status
- `command/command.go` - Added callback variables for bus notification
- `main.go` - Updated notification builder signature, wired up callbacks

## Session: Jan 5, 2026 - Entity Refactor + Bus Toggle + UI Fixes

### Entity Type Refactor
Refactored `Entity.Data` from `map[string]interface{}` to typed structs:
- `ArrivalData`, `WeatherData`, `PrayerData`, `PlaceData`, `AgentEntityData`, `StreetData`
- Accessor methods (`GetArrivalData()`, etc.) handle both typed and legacy data
- Custom `UnmarshalJSON` for backward compatibility
- All existing `spatial.json` data loads correctly

### Bus Notification Toggle
- `/bus` - Shows bus times (unchanged)
- `/bus on` - Enables bus push notifications when backgrounded
- `/bus off` - Disables bus push notifications (default)
- `/bus status` - Shows current setting
- Bus times always show in context card; only push notifications affected

### Android Splash Screen
- Resized maskable icons to ~40% of canvas (200x200 in 512x512)
- Requires PWA reinstall to take effect

### Map Legend Mobile Fix
- Legend toggle button moved up (bottom: 60px) to avoid phone navigation bar cutoff
- Button made larger (18px font, more padding)
- z-index increased to 100
- Legend appears at bottom: 120px when shown
- Added no-cache headers to /map endpoint

### Files Changed
- `spatial/entity.go` - Typed data structs
- `spatial/live.go` - Use typed data
- `spatial/context.go`, `spatial/agent.go`, `spatial/db.go`, `spatial/streets.go` - Use accessor methods
- `server/map.go`, `server/map.html` - Legend UI fixes, cache headers
- `server/push.go` - BusNotify field and methods
- `server/agents.go` - Use typed AgentEntityData
- `command/context_questions.go` - `/bus on/off` handling
- `command/command.go` - Bus notification callbacks
- `command/agents.go`, `command/directions.go`, `command/nearby.go` - Use accessor methods
- `main.go` - Wire up bus callbacks, notification builder signature
- `client/web/icon-*-maskable.png` - Smaller icons for splash screen


## Session: Jan 5, 2026 - Duplicate Agent Fix

### Problem
Multiple agents with similar names for the same location:
- "Hampton" vs "Hampton, Greater London" (2.4m apart)
- "Norbiton" vs "Norbiton, Greater London"
- "Southwark" vs "Southwark, Greater London"
- "Kilmacud West" vs "Kilmacud West, County Dublin"
- "Area 51.42,-0.37" overlapping with Hampton

### Root Cause
1. Nominatim returns different names/coordinates on different days
2. 1km radius dedupe wasn't catching agents with slightly different coords
3. No name-based dedupe - "Hampton" and "Hampton, Greater London" weren't matched

### Fix Applied

**Data cleanup:**
- Merged 5 duplicate agents
- Transferred 731 places to surviving agents
- Reduced from 34 to 32 agents

**Code fix (`spatial/agent.go`):**
- Added base name comparison in `FindOrCreateAgentNamed`
- Strips region suffix (", Greater London") before comparing
- Logs when similar agent found

### Prevention
New agent creation now checks:
1. Existing agent within 1km radius
2. Existing agent with same base name (e.g., "Hampton" matches "Hampton, Greater London")


## Session: Jan 6, 2026 - Weather Fix

**Bug:** Temperature showing 0°C when actual is -3°C

**Root causes:**
1. Weather query radius (10km) didn't match fetch radius (5km)
2. Corrupted entity with no `ExpiresAt` was never filtered out
3. Legacy data parsing fallback was missing

**Fixes:**
- Query radius now 5km to match fetch
- Ephemeral entities (weather, arrivals, prayer) without ExpiresAt treated as expired
- Added legacy name parsing fallback for temp

---

## Known Gaps (TODO)

### 1. Awareness System Not End-to-End
- Agents accumulate observations ✅
- `/observe` shows pending observations ✅  
- LLM filter to decide what's interesting ✅
- **MISSING:** Automatic delivery to user's timeline/push
- Currently: must manually run `/observe process`

### 2. Agent Creation Feedback
- New agents created silently
- Should say "Spinning up agent in your area..."

### 3. Regional Transport APIs
- Only TfL (London) implemented
- TODO: TfGM (Manchester), TfW (Wales), National Rail, Dublin

### 4. ARCHITECTURE.md Missing
- README references it but file doesn't exist
- Should document the spacetime model

### 5. Step Counter
- Accelerometer access exists
- Not counting/displaying steps

### 6. Bus Data in Context JSON
- `"bus": null` in context JSON structure
- Bus info only in HTML string

### 7. Event Log Improvements
- No unique event IDs (evt_xxx)
- No checkpoint markers
- Rebuild from events not tested

---

## Session: Jan 5/6, 2026 - Performance Fixes, Async Commands, Courier

### Major Performance Fix: Batched Disk Writes
**Problem:** Every `db.Insert()` was writing the entire 13MB `spatial.json` to disk while holding the DB write lock. This blocked all queries, causing `/ping` to take 200-800ms.

**Solution:** Refactored `FileStore` in quadtree to batch writes:
- `Save()` marks data as dirty, schedules async save after 5s delay
- Background goroutine ensures save within 30s max
- `Close()` flushes immediately
- Persist happens without holding any locks (copies data first)

**Result:** `/ping` response time: 200-800ms → 9-10ms (20-80x faster)

**Files changed:**
- `~/quadtree/store.go` - Batched async FileStore

### Other Blocking Fixes
- `FindOrCreateAgent` now uses generic name immediately, geocodes async
- `ReverseGeocode` uses rate-limited HTTP client
- `Geocode` and OSM queries use rate-limited client
- Added `OSMPost()` for Overpass API queries

---

## Session: Jan 5/6, 2026 - Async Commands, Courier, Map Fixes

### Async Command Mode
Commands can now be executed asynchronously. Result comes via WebSocket instead of HTTP response.

**Usage:**
```bash
# Sync (default) - waits for result
curl -X POST '/commands' -d 'prompt=/courier'
# Returns: 🚴 Courier enabled!

# Async - returns immediately
curl -X POST '/commands' -d 'prompt=/courier&async=true'
# Returns: {"id":"cmd_abc123","status":"queued"}
# Result pushed via WebSocket with type="command_result"
```

**WebSocket message format:**
```json
{
  "Type": "command_result",
  "CommandID": "cmd_abc123",
  "Text": "🚴 Courier enabled!",
  "Stream": "~",
  "Channel": "@session_token"
}
```

**Client-side:**
- `sendAsyncCommand(prompt, callback)` - sends async and tracks pending
- `pendingAsyncCommands` - tracks commands waiting for results
- WebSocket handler processes `command_result` type messages

**Files:**
- `server/handler.go` - async mode handling
- `server/server.go` - `NewCommandResult()`, `CommandID` field on Message
- `client/web/malten.js` - `sendAsyncCommand()`, WebSocket handling (v275)

---

## Session: Jan 5/6, 2026 - Courier, Map Fixes

### Courier Agent
New system to connect areas by walking routes between them.

**Commands:**
- `/courier` - Show status
- `/courier on` - Enable courier
- `/courier off` - Pause courier

**How it works:**
1. Starts at Hampton
2. Picks nearest unconnected agent
3. Gets walking route via OSRM
4. Walks the route (~100m steps every 5s)
5. Indexes street geometry
6. Indexes POIs along route (async)
7. Arrives, picks next destination

**Files:**
- `spatial/courier.go` - Courier logic
- `command/courier.go` - Command interface

### Pinch Zoom Fix
Map now zooms around pinch center point instead of screen center.
- Mouse wheel zoom also centers on cursor
- Both use same offset adjustment formula

### Async POI Indexing
Courier's POI indexing is now fully async to avoid blocking when OSM is slow.

---

## Session: Jan 5/6, 2026 - Map Arrow & POI Visibility Fixes

### Issues Fixed

1. **POIs not visible when zoomed out past 12m/px**
   - Before: POIs only showed at <=12 m/px, completely disappeared otherwise
   - After: POIs show up to 50 m/px with gradual fade (full opacity at <=12, fades to 30% at 50)
   - Also reset globalAlpha before drawing to prevent inherited transparency

2. **Arrow 45° off to the right**
   - Root cause: compass heading (deviceorientation) was unreliable on the user's device
   - Fix: GPS-based heading now preferred when moving (speed > 1 m/s)
   - Compass heading only used when stationary
   - GPS heading requires minimum 3m movement to filter out noise
   - Added separate tracking: `gpsHeading`, `compassHeading`, `userHeading` (combined)

### Files Changed
- `server/map.html` (v5) - POI visibility at all zooms, GPS/compass heading priority

### Heading Priority Logic
```javascript
// Prefer GPS heading when moving, otherwise use compass
if (userSpeed > 1 && gpsHeading !== null) {
    userHeading = gpsHeading;
} else if (compassHeading !== null) {
    userHeading = compassHeading;
} else if (gpsHeading !== null) {
    userHeading = gpsHeading;
}
```

---

## Session: Jan 5, 2026 - Exploring Agents & Map Improvements

### Exploring Agents
Agents now move through space, mapping streets as they go:

**How it works:**
1. `/explore on` enables exploration mode
2. Each agent picks a target (nearby POI or random direction)
3. Gets walking route via OSRM
4. Stores street geometry in spatial index
5. Moves to destination
6. Indexes POIs along the route
7. Repeats

**Rate limiting:**
- Each agent can move every 10 seconds
- OSRM calls rate-limited to 2s globally
- OSM calls rate-limited to 5s globally

**Persistence:**
- Exploration state (home_lat, home_lon, total_steps, steps_today) saved to AgentEntityData
- Survives server restarts
- Agent position (lat, lon) also saved

**Commands:**
- `/explore on` - Enable exploration
- `/explore off` - Disable exploration
- `/explore status` - Show exploration stats

**Files:**
- `spatial/explorer.go` - Exploration logic
- `spatial/entity.go` - Added exploration fields to AgentEntityData
- `command/explore.go` - Explore command

### Map Improvements

**Visibility fixes:**
- Removed large agent coverage circles (were obscuring places)
- Added `ctx.fillStyle = '#000'` before drawing place emojis (were inheriting transparent blue)
- Places now fully visible and colorful

**Click handling:**
- Added drag distance check (only count as drag if moved >3px)
- Clicks on stationary mouseup now properly detected
- Increased hit area for places (radius + 10px)
- Popup shows place/agent details with Map and Directions links

**Live mode:**
- ▶️ button toggles 5-second auto-refresh
- 🔄 button for manual refresh
- Position preserved during refresh

### Current Stats
- 32 agents
- ~300 streets mapped
- ~4,500 places indexed
- Exploration state persists across restarts

## Session: Jan 6, 2026 - Regional Couriers & UI Polish

### Regional Couriers Implemented
- Multiple couriers operate in parallel across different regions
- Agents clustered by 50km proximity
- Each cluster gets its own courier
- Files: `spatial/courier_regional.go`, `command/regional_courier.go`
- Commands: `/couriers`, `/couriers on`, `/couriers off`
- State persisted to `regional_couriers.json`

### Map UI Improvements
- Agents hidden from map (work invisibly like angels)
- Category names formatted: "place_of_worship" → "Place of worship"
- Title changed from "Malten Spatial Index" to "Map"
- Buttons use proper symbols (↻, ‖) instead of emojis
- Compass heading updates when turning (redraws on >5° change)
- Popup centered on screen instead of near clicked point
- Less clutter when zoomed out

### Bug Fixes
- Push notification deduplication: `DailyPushed` map tracks which notification types sent today per user
- Timeline scrolls to bottom when app reopens (visibilitychange handler)
- Courier state persisted to survive restarts

### Names of Allah Links
- Now linked to reminder.dev/name/{number}
- Both daily reminders and prayer-specific reminders include name_number

### Camera/Photo Capture
- Camera button (📷) next to input field
- Photos added to timeline with location
- Stored in state.photos (up to 50)
- Tap thumbnail to view fullscreen

### Context Refresh Fix
- Immediate ping with cached location on app reopen
- Then GPS update if position changed >50m
- Refresh if context >30s old on visibility change
- Refresh if context >60s old on initial load

### Current Agent Status
- 32 agents globally
- 2 regional courier clusters:
  - London area (19 agents)
  - Cardiff/Castle area (3 agents)
- 10 isolated agents (single-agent areas, no courier)

### Files Changed This Session
- `spatial/courier_regional.go` - NEW: Regional courier manager
- `command/regional_courier.go` - NEW: /couriers command
- `server/map.html` - v11: UI polish, hide agents, compass fix
- `server/push.go` - DailyPushed tracking
- `spatial/courier.go` - State persistence
- `client/web/malten.js` - Camera, scroll fix, context refresh
- `client/web/malten.css` - Photo card styling
- `client/web/index.html` - Camera button
- `command/reminder.go` - name_number in response
- `spatial/reminder.go` - GetNameNumber() method
- `main.go` - Start regional courier loop
- `README.md` - Updated
- `claude.md` - Updated

### Known Issues
- Push notification timestamp mismatch: Ad-Duha shows "1 hour ago" in timeline but push received "1 minute ago"
  - Likely: push history fetch adds items with old timestamps
  - TODO: investigate fetchPushHistory timing

### JS Version: 282
### Map Version: v11

## Session: Jan 6, 2026 - Continued Fixes

### Push Notification Duplicate Fix
- Removed Ad-Duha from push notifications (was duplicating client-side prayer reminder)
- Client-side `checkPrayerReminder()` handles Duha, push system no longer sends it

### Map UI Fixes
- **Map links include place name**: URLs now use `maps/search/Name/@lat,lon` format for better Google Maps matching
- **Dynamic legend**: Only shows categories visible on screen, with counts, sorted by frequency
- Legend updates when toggled open

### Timeline Scroll Fix
- Changed `scrollToBottom()` to target `#messages-area` container directly
- Uses `scrollTop = scrollHeight` instead of `scrollIntoView`
- Increased delay to 300ms to ensure layout settles after context card expansion

### Files Changed
- `server/push.go` - Removed Duha push notification
- `server/map.html` - v14: Dynamic legend, map links with names
- `client/web/malten.js` - v283: Scroll fix

### Current State
- 32 agents globally
- 2 regional courier clusters active (London + Cardiff)
- Courier state persists across restarts
- Map shows places without agents (invisible like angels)
- Dynamic legend based on visible places

### JS Version: 283
### Map Version: v14

## Session: Jan 6, 2026 - Route Tracking, Zones, Sedentary Reminder

### Route Tracking from GPS (Phase 1)
Your actual GPS movement now gets captured as streets:

**How it works:**
- Every `/ping` calls `spatial.RecordLocation(session, lat, lon)`
- Points <20m apart skipped (GPS jitter)
- After 5 min stationary OR 500 points, track saved as street
- Douglas-Peucker algorithm simplifies (fewer points, same shape)
- Dedupes against existing streets
- Street has no user ID - just anonymous geometry

**Files:**
- `spatial/track.go` - TrackPoint, UserTrack, RecordLocation, saveTrackAsStreet, simplifyTrack
- `command/nearby.go` - Added RecordLocation call in handlePing

**Privacy model:**
- Server extracts street geometry from GPS pings
- Street stored anonymously (just coordinates, no user link)
- User's personal history stays in localStorage
- We're mapping streets, not tracking users

### `/zones` Command
Shows coverage analysis:

```
📊 **Coverage Analysis**

**Well connected:** Hampton (67), Whitton (60)
**Sparse:** Hounslow West (7)
**Dead zones** (gaps to fill):
• Hampton Hill ↔ Strawberry Hill (1.6km)
• Whitton ↔ St Margarets (1.8km)
```

**File:** `command/zones.go`

### Courier Improvements
- `/goto <lat,lon>` - Send courier to specific point
- `/route <lat,lon> <lat,lon> ...` - Multi-waypoint route
- `ManualTarget` flag prevents auto-switching mid-route
- Waypoint queue for sequential destinations
- Courier algorithm now picks FARTHEST isolated component (better coverage)

**Files:**
- `command/courier_goto.go` - /goto command
- `command/courier_route.go` - /route command with waypoints
- `spatial/courier_regional.go` - SendCourierTo, waypoint queue, improved pickRegionalDestination

### Sedentary Reminder
Nudges you to move if inactive for 1 hour:

> 🚶 You've been sitting for 60 minutes. Time for a walk?

**Triggers:**
- No steps detected (accelerometer) for 1 hour
- GPS position unchanged (>50m) for 1 hour

**Implementation:**
- `sedentaryReminder` object in malten.js
- Checks every 5 minutes
- Records movement on: step detected, GPS moved >50m
- Shows timeline message + browser notification if backgrounded

### Map Improvements
- Zoom centers on mouse/pinch point (`zoomAtPoint()` function)
- Double-tap to zoom on mobile
- Area names displayed (Hampton, Whitton, etc.) at 2-100 m/px zoom
- Version: v16

### Help & Markdown
- `/help` command lists all commands
- `/pro` command removed (not ready)
- Markdown rendering: `**bold**`, `` `code` ``, newlines
- `.card code` and `.card strong` CSS styling

### Commands Added This Session
| Command | Description |
|---------|-------------|
| `/help` | List all commands |
| `/zones` | Coverage analysis, dead zones |
| `/goto <coords>` | Send courier to point |
| `/route <coords> ...` | Multi-waypoint courier route |

### JS Version: 300
### Map Version: v16

### The Plan Going Forward

**What we have:**
1. User GPS pings → anonymous street geometry (automatic)
2. Courier explores in background (slow, respects rate limits)
3. `/zones` shows coverage gaps
4. `/route` for manual gap-filling when needed

**Future phases:**
- Heat map on /map showing street density
- Auto-bridge: courier finds waypoints through dead zones
- `/bridge <from> <to>` - suggest waypoints between areas

**Philosophy:** "Slow is smooth, smooth is fast"
- Courier maps slowly in background
- User's actual walks fill in their real routes
- No API hammering, organic growth
