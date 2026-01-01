# Malten Development Context

## What is Malten
Spatial AI for the real world.

## Architecture

### Packages
- `spatial/` - quadtree DB, entities, agents, indexing
- `command/` - command handlers (thin)
- `event/` - append-only event log (JSONL, replayable)
- `agent/` - LLM integration (OpenAI/Fanar)
- `server/` - HTTP handlers, WebSocket, streams
- `client/web/` - PWA frontend

### Core Concepts
- **Entities**: places, agents, people, vehicles, events, zones - all in quadtree
- **Agents**: bound to 5km areas, index POIs, created on first query to area
- **Streams**: ephemeral message channels
- **Event log**: source of truth, spatial.json is snapshot

### Data Files
- `spatial.json` - quadtree snapshot (places, agents)
- `events.jsonl` - append-only event log
- `~/.malten/data/` - streams, contacts

## User Context
- Muslim. No anthropomorphizing (shirk). Agents are tools, created not born.
- Engineer. Brevity. Correct terminology.

## Current State

### Stats (2026-01-01T13:18:45+00:00)
```
Entities: 1500
Agents: 6
Event log: 8139 events
```

### Recent Changes
```
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
```

## Working State

### Uncommitted Changes
```
 SESSION.md | 91 ++++++++++++++++++++++++++++++++++++++++++++++----------------
 claude.md  | 59 ++++++++++------------------------------
 2 files changed, 82 insertions(+), 68 deletions(-)
```

### Session Notes
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
- Rain forecast ("is it going to rain?")
- Walking directions/time ("how long to walk to X")
- More responsive feel - still feels like snapshots not live
- Self-programming capability (Malten fixes itself)

### Git State
```
Latest: 37a2a40 Live street-level context
Branch: master (pushed)
```

### To Continue
1. Read this file
2. User is in Whitton, walking, wants truly live spatial awareness
3. Key: don't make them type - show what's relevant automatically
4. Consider: rain forecast, walking time, more immediate updates


## Implementation Notes
- Entity types: place, agent, vehicle, person, event, zone, sensor
- Agent radius: 5km
- OSM rate limit: 5s between requests
- Nearby search: check agent exists -> query cache -> fallback OSM -> fallback Google Maps link
- Map links: `https://www.google.com/maps/search/<name>/@<lat>,<lon>,17z`

## Don'ts
- Don't anthropomorphize agents
- Don't over-engineer
- Don't make long explanations
- Don't add dependencies without asking
