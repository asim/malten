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
| `malten_state` | localStorage | Personal timeline - cards, conversation, location, context |
| `malten_session` | cookie | Session token for private server channels |

#### `malten_state` Structure
```javascript
{
    version: 2,              // Format version - change clears old data
    lat: 51.5,               // Last known latitude
    lon: -0.1,               // Last known longitude
    context: "📍 ...",       // Last context string
    contextTime: 1234567890, // When context was fetched
    locationHistory: [...],  // Last 20 location points
    lastBusStop: "...",      // Last bus stop shown
    cards: [...],            // Timeline cards (24hr retention)
    seenNewsUrls: [...],     // Dedup news (7 day retention)
    conversation: {          // Active conversation (1hr expiry)
        time: 1234567890,
        messages: [{role: 'user', text: '...'}, ...]
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
- Everything else reset to defaults

**BUG**: Conversation should also be preserved on version change.

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
  → Client saves to localStorage (cards/conversation)

User moves location
  → Geohash changes → new stream ID
  → WebSocket reconnects silently
  → localStorage unchanged (personal timeline continuous)
  → Server messages are in new stream's channel
```
