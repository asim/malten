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

### Stats (2026-01-01T11:07:48+00:00)
```
Entities: 1171
Agents: 3
Event log: 844 events
```

### Recent Changes
```
e4b103b Update TODO: service worker done
80efb25 Add service worker for offline support
52a775a Find agents by name to prevent duplicates
595b127 Ignore data files
301108c Refactor: spatial package, event log, clean command package
6623232 Add spatial AI features: nearby places, location tracking, quadtree cache
f1887a8 Update assistant features: reminder formatting, links, commands without slash
7717238 Fix names of Allah display in search results
3d4d2cf Natural language reminder queries, linked references, fix hashtag parsing
c7ceaa4 Add caching (60s price, 5m reminder), /reminder and /quran search commands
```

## Working State

### Uncommitted Changes
```
 claude.md               | 30 +++++++++++-------------------
 scripts/save_context.sh | 32 ++++++++++++++++++++++++++++++--
 2 files changed, 41 insertions(+), 21 deletions(-)
```

### Session Notes
## Last Session
- Added service worker for offline PWA support
- Fixed agent deduplication (find by name, not just location)

## Next Up
- Test offline mode in browser
- Consider speech-to-text (browser API)

## Key Decisions
- Agents are identified by area name to prevent duplicates
- Service worker uses cache-first for static, skips API calls


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
