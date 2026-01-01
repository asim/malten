# Malten Architecture

Malten is a spatial AI platform.

## Core Concepts

### Streams
Ephemeral message channels. Messages expire after idle time. Users communicate in streams.
- Max 1024 messages per stream
- TTL: 1024 seconds idle
- Identified by hash (e.g., `#hampton`)

### Commands
Slash commands and natural language queries:
- `/nearby cafes` - Find nearby places
- `/price btc` - Crypto prices
- `/reminder` - Islamic reminders
- `cafes near me` - Natural language nearby query

### Agents
Entities bound to geographic areas. Stored in spatial DB as type "agent".

Agent structure:
```json
{
  "id": "uuid",
  "type": "agent",
  "name": "Hampton",
  "nodes": [{"id": "n0", "x": 51.4, "y": -0.37}],  // lat, lon
  "radius": 5000,  // meters
  "data": {
    "prompt": "You are a local guide for Hampton.",
    "status": "active",
    "stream": "hampton"  // optional associated stream
  }
}
```

Agents:
- Are created automatically when a user queries a new area
- Index POIs in their territory (cafes, mosques, shops, etc.)
- Persist across restarts (stored in spatial.json)
- Can have an associated stream for notifications
- Have a prompt for AI persona/context

### Spatial DB
Quadtree-based spatial index for all entities:
- Places (POIs from OSM)
- Agents (area managers)
- Events (time-bounded)
- Zones (regions)

File: `spatial.json`

## Package Structure

```
malten/
├── main.go           # HTTP server setup
├── agent/            # LLM integration (OpenAI/Fanar)
│   └── agent.go      # Tool selection, prompts, AI calls
├── command/          # Command implementations
│   ├── nearby.go     # Location search (OSM, cache, fallback)
│   ├── spatial.go    # Spatial DB, quadtree, entities
│   ├── price.go      # Crypto prices
│   ├── reminder.go   # Islamic reminders
│   └── ...           
├── server/           # HTTP handlers, WebSocket
│   ├── handler.go    # Command routing, AI dispatch
│   ├── location.go   # Location handling, nearby command
│   └── server.go     # Stream management, events
├── client/web/       # Frontend (embedded)
│   ├── index.html
│   ├── malten.js
│   └── malten.css
└── config/           # Configuration
```

## Data Flow

### Nearby Query
1. User: "cafes near me"
2. `detectNearbyQuery()` identifies place type + location
3. `HandleNearbyCommand()` resolves coordinates
4. Check if agent exists for area (5km radius)
5. If no agent, create one → starts background indexing
6. Query spatial DB for cached POIs
7. If cache miss, query OSM Overpass API
8. If OSM fails/empty, return Google Maps fallback link
9. Cache results in spatial DB
10. Return formatted results with map links

### Agent Lifecycle
1. Created when user queries uncovered area
2. Reverse geocodes to get area name (Hampton, Twickenham, etc.)
3. Background task indexes POI categories:
   - amenity=cafe, restaurant, pharmacy, hospital, bank, fuel
   - amenity=place_of_worship (mosques, churches)
   - shop=supermarket
   - tourism=hotel
4. Stores POIs in spatial DB with agent reference
5. Persists to spatial.json
6. Subsequent queries hit local index (fast)
7. Periodic re-index to stay fresh

## Spiritual Context

Malten is built with Islamic values:
- Privacy-respecting (no Google tracking by default)
- Mosque/halal-aware place search
- Islamic reminders (Quran, Hadith, Names of Allah)
- Crisis support with appropriate resources

Agents can be configured with prompts that reflect these values.

## API Endpoints

```
GET  /messages?stream=x&limit=25    Messages
POST /messages                       Post message
POST /commands                       Execute command
WS   /events?stream=x               Real-time events
POST /ping                          Update location
GET  /streams                       List streams
POST /streams                       Create stream
```

## Future: /agents Endpoint

```
GET  /agents                        List all agents
GET  /agents?lat=x&lon=y            Find agent for location
POST /agents                        Create agent manually
```
