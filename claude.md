# Malten Development Context

## What is Malten
Spatial AI for the real world. Foursquare if built in the AI era.

## Architecture

### Quadtree (spatial.json) - Source of Truth
Everything spatial lives here:
- **EntityPlace** - POIs from OSM (indexed by agents)
- **EntityAgent** - area indexers (5km radius, run loops)
- **EntityPerson** - users by session token
- **EntityWeather** - weather + rain forecast (10min TTL)
- **EntityPrayer** - prayer times (1hr TTL)
- **EntityArrival** - bus arrivals per stop (2min TTL)

### Agents
Agents are spatial indexers, not anthropomorphized entities:
- Created when user pings a new area
- Run continuous loop: live data every 30s, POI index once
- Store everything in quadtree with TTL
- Should do MORE: predict, learn patterns, proactive suggestions

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

### Client
- Context at top (live, from quadtree)
- Cards below (persisted 24hr in localStorage)
- Cards created on context changes (bus stop, rain, prayer)
- Personal timeline - your spatial memory

### Data Flow
```
User pings → Agent created if needed → Agent indexes area
                                     → Live data every 30s
Context query → Quadtree lookup (fast) → Build context string
             → Client detects changes → Creates card
             → Card persists in localStorage
```

## Files
- `spatial/` - quadtree, entities, agents, live data fetching
- `command/` - all commands (walk, nearby, price, reminder, etc)
- `server/` - thin HTTP handlers, WebSocket
- `client/web/` - PWA frontend

## Current Stats
```
Entities: ~1500
Agents: 6
Event log: 8139 events
```

## User Context
- Muslim. Prayer times important. No anthropomorphizing.
- Engineer. Brevity. Correct terminology.
- Building Mu (blog, chat, news, video, mail) - Malten is spatial component.

## Key Principles
- Quadtree is source of truth
- Agents index and maintain data
- Context from quadtree, not API calls
- Cards are personal (localStorage), streams are shared
- Proactive > reactive

## Don'ts
- Don't anthropomorphize agents
- Don't make server thick - commands handle logic
- Don't call APIs in GetLiveContext if quadtree has data
- Don't over-engineer
