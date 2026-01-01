# Malten Development Context

## What is Malten
Spatial AI for the real world. Foursquare if built in the AI era.

## Core Primitives - NEVER CHANGE THESE

### Three Building Blocks
1. **Streams** (`/streams`) - message channels, real-time updates
2. **Agents** (`/agents`) - spatial indexers, maintain the world view
3. **Commands** (`/commands`) - actions, everything is a command

### Quadtree - The Spatial Index
- Source of truth for all spatial data
- Where we map and store everything
- Real-time view of the world
- File: `spatial.json`

### Cards = Stream Messages
- Cards are just messages in the stream
- Standard format for real-time updates
- Everything is a message on the stream
- Persisted in localStorage (24hr) for personal timeline

## Architecture

### Quadtree Entities
- **EntityPlace** - POIs from OSM (indexed by agents)
- **EntityAgent** - area indexers (5km radius)
- **EntityPerson** - users by session token
- **EntityWeather** - weather + rain forecast (10min TTL)
- **EntityPrayer** - prayer times (1hr TTL)
- **EntityArrival** - transport arrivals (2min TTL)

### Agents
Agents are spatial indexers (not anthropomorphized):
- Created when user pings a new area
- Index POIs from OSM (cafes, stations, bus stops, etc)
- Update live data every 30s (weather, prayer, transport)
- Store everything in quadtree with TTL
- Build the world view in real time

### Commands
Everything is a command. Commands self-register:
```go
Register(&Command{
    Name:    "walk",
    Handler: handleWalk,
    Match:   matchWalk,  // natural language detection
})
```
Server just routes to `command.Dispatch(ctx)`.

### Data Flow
```
User pings location
  → Agent created if needed
  → Agent indexes area (POIs, transport)
  → Agent updates live data every 30s
  → All stored in quadtree

Context query
  → Quadtree lookup (fast, no API calls)
  → Build context string
  → Client detects changes → creates card
  → Card = message on stream
```

## Files
- `spatial/` - quadtree, entities, agents, live data
- `command/` - all commands (walk, nearby, price, etc)
- `server/` - thin HTTP handlers, WebSocket, streams
- `client/web/` - PWA frontend

## User Context
- Muslim. Prayer times important. No anthropomorphizing.
- Engineer. Brevity. Correct terminology.
- Building Mu (blog, chat, news, video, mail) - Malten is spatial.

## Key Principles
1. Streams, Agents, Commands - the three primitives
2. Quadtree is source of truth
3. Cards are stream messages
4. Agents build the world view
5. Context from quadtree, not API calls
6. Proactive > reactive

## Don'ts
- Don't change the three primitives
- Don't anthropomorphize agents
- Don't make server thick - commands handle logic
- Don't call APIs if quadtree has data
- Don't over-engineer
