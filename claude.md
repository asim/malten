# Malten Development Context

## What is Malten
Spatial AI for the real world. Foursquare if built in the AI era.

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
