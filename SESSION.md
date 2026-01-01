## QUICK START

### What is Malten
Spatial AI. Open app → see world around you. Weather, prayer, buses, places. Moves with you seamlessly.

### Run
```bash
systemctl status malten  # running on :9090
journalctl -u malten -f  # logs
```

### Test
```bash
curl -s "http://localhost:9090/ping" -X POST -d "lat=51.417&lon=-0.362" | jq .
```

---

## Architecture

### Five Primitives (NEVER CHANGE)
- **Streams** - geohash areas + channels within (invisible infrastructure)
- **Agents** - index areas, build world view
- **Commands** - actions (natural language, slash optional)
- **Database** - quadtree (`spatial.json`)
- **Events** - replayable log (`events.jsonl`)

### Streams with Channels
```
Stream: gcpsxb (Hampton area)
├── (public) - spatial events everyone sees
├── @session123 - my questions/responses (private)
├── @session456 - another user (private)
└── @groupname - group chat (shared secret)
```

- Stream = the space (geohash)
- Channel = who hears it within that space
- Empty channel = public (weather, buses)
- @session = addressed to that session (private)

Message struct:
```go
type Message struct {
    Stream  string  // "gcpsxb"
    Channel string  // "" = public, "@session" = addressed
    ...
}
```

### Agentic Model
Agents are the ONLY thing that fetches external APIs. Users only read from the index.

```
Agent Loop (background, every 30s):
  - Index location names (reverse geocode)
  - Index weather
  - Index prayer times
  - Index transport arrivals
  - Index POIs

User Request:
  - Read from spatial index
  - Zero API calls
  - Instant response
```

### Data Flow
```
Location → Geohash → Stream ID (invisible)
                  → Agent for area indexes data
                  → User queries spatial index
                  → Context displayed (no fetch)

Question → Stream:@session channel
        → Response to same channel
        → Persists until read/TTL
```

---

## Key Files
- `spatial/` - quadtree, agents, geohash, live data
- `command/` - all commands (natural language + slash)
- `server/` - HTTP/WebSocket, streams, channels
- `client/web/` - PWA (served from disk via -web flag)

---

## What's Working

### Context shows:
- 📍 Location (street, postcode)
- Weather + prayer on one line
- 🚏 Nearest bus stop with arrivals
- Places: clickable, expand inline with full details

### Cards created on:
- Location change (new street/area)
- Bus stop arrival
- Prayer time change
- Rain warning

### Questions & Responses:
- Ask "cafes nearby", "what time does X close", etc
- Response appears as card
- Both persist in stream (your @session channel)
- Reload page → messages load from stream
- Other users don't see your questions

### Spatial caching:
- Weather: 5km radius
- Prayer: 50km radius
- Transport: per stop, extended TTL if API fails
- Location: 500m radius, 1hr TTL

---

## Recent Commits
```
dde1896 Channels within streams: private session messages
72d94e6 Place info command: ask about specific places, responses as cards
6e5c195 Persist cards to localStorage, commands work via HTTP
4e61bb0 Compute prayer display at query time
6b550ef Change placeholder to 'What's happening?'
4b212fd Add Enable location button to welcome screen
```

---

## Open Questions

### Persistence across restarts
- Public spatial events: should persist (weather, buses - fine to store)
- Private @session messages: in-memory only (privacy)
- Gap: server restart loses pending responses before client reads
- Mitigation: client saves to localStorage immediately

### Location privacy
- Stream name = geohash = location
- Messages to @session don't expose location directly
- But: stream history could reveal where you were
- Consider: per-session stream isolation?
