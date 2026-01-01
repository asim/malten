## Last Session - 2026-01-01 13:15 UTC

### What We Built Today
1. **Live spatial context** - weather, prayer times, bus arrivals from TfL API
2. **Agent continuous loops** - each agent indexes its territory, updates live data every 30s
3. **Street-level awareness** - reverse geocode shows "📍 Montrose Avenue, Whitton"
4. **Bus stop detection** - "🚏 At Whitton Station" when within 30m
5. **Instant context on ping** - server returns context with ping response
6. **User in quadtree** - session token maps to EntityPerson in spatial index
7. **Welcome message** - never empty screen, shows greeting + guidance
8. **State management** - consolidated into single `malten_state` localStorage object

### User's Vision (IMPORTANT - preserve this)
"I'm walking. I need information. It's always good to know:
- How cold it is, weather, if it's going to rain
- What time to pray
- If I'm passing a bus stop, what street I'm on
- Sometimes ask: coffee shop nearby? How long to walk to 309 Whitton Dene?

We can be smarter. Google will do this eventually but they haven't.
I don't have a contextually aware spatial AI.
Open an app and know what's around you without typing.

Mu has blog, chat, news, video, mail. Spatial/maps would be next.
Malten is spatial AI - standalone product/tool."

### Current Architecture
```
User pings location → stored as EntityPerson in quadtree
                   → agent created for area if new
                   → context returned immediately

Agent loop (per area):
  - Indexes POIs from OSM (once, takes minutes)
  - Updates live data every 30s (weather, prayer, buses)
  - Writes to spatial index with TTL

Context query:
  - Reverse geocode → street name
  - Nearest bus stop + arrivals
  - Weather + prayer from index
  - Places summary
```

### Files Changed This Session
- `spatial/live.go` - GetLiveContext, reverseGeocode, getNearestStopWithArrivals
- `spatial/agent.go` - agent loop runs live updates in parallel with POI indexing
- `command/nearby.go` - SetLocation creates agent, caches user context
- `server/location.go` - ping returns context, context uses session
- `client/web/malten.js` - state object, welcome message, 15s update interval

### What's Missing (User Feedback)
- Rain forecast ("is it going to rain?") - Open-Meteo has hourly forecast
- Walking directions/time ("how long to walk to X") - OSRM or Google Directions API
- More responsive feel - still feels like snapshots not live
- Self-programming capability (Malten fixes itself)
- Trains - National Rail API (needs registration)
- Events - what's happening nearby today

### Known Issues Fixed This Session
- Context disappeared on refresh → now persists in localStorage + persistent div
- Agents not indexing live data → fixed parallel execution
- New areas had no agent → SetLocation now creates agent on ping
- Empty screen on first visit → welcome message added
- Whitton had no buses → agent wasn't created, fixed

### Data Sources
- **TfL API** (free, no key) - buses, tubes, stops, arrivals
- **Open-Meteo** (free, no key) - weather, hourly forecast available
- **Aladhan** (free, no key) - prayer times
- **OSM/Nominatim** (free) - reverse geocode, POIs via Overpass
- **Google Maps links** - fallback for directions

### User Preferences (from claude.md)
- Muslim - prayer times important, no anthropomorphizing
- Engineer - brevity, correct terminology
- Wants spatial AI that works without typing
- Building Mu (blog, chat, news, video, mail) - Malten is spatial component

### Git State
```
d1fc1ad Save session state for conversation continuity
37a2a40 Live street-level context
4a48964 Fix: create agent on ping, run live updates in parallel
68ceac5 Show welcome message when no context available
7e106fa Server maintains user view - ping returns context
a1ecb87 Consolidate state management into single object
e2d8c9c Fix: always show cached context on load, store last location
49ee15c Agent as continuous loop with prompt, persistent context UI
7f70a6c Live data indexer - instant context from spatial index
daf1acb Instant context with caching
c3f503f Context-aware summary on page load
b28bd51 Auto-show local context on page load
fb4ba3b Add live bus arrivals via TfL API
e3d5347 Add speech-to-text input
80efb25 Add service worker for offline support
```

### To Continue
1. Read this file
2. User is in Whitton, walking, wants truly live spatial awareness
3. Key: don't make them type - show what's relevant automatically
4. Consider: rain forecast, walking time, more immediate updates
