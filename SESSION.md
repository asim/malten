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
- **Streams** - geohash areas (invisible infrastructure)
- **Agents** - index areas, build world view
- **Commands** - actions (natural language, slash optional)
- **Database** - quadtree (`spatial.json`)
- **Events** - replayable log (`events.jsonl`)

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

### Seamless Spatial Experience
Stream/geohash is infrastructure, not UX. User never sees it.

- **Continuous view** - no jumps, no refreshes
- **Persistent timeline** - cards don't disappear when crossing boundaries
- **Smooth context updates** - weather, buses, places blend as you move
- **Silent stream switching** - WebSocket reconnects in background

The two views:
- **Context (top)** = live view of where you are NOW
- **Timeline (below)** = YOUR history, everywhere you've been

### Data Flow
```
Location → Geohash → Stream ID (invisible)
                  → Agent for area indexes data
                  → User queries spatial index
                  → Context displayed (no fetch)
```

---

## Key Files
- `spatial/` - quadtree, agents, geohash, live data
- `command/` - all commands (natural language + slash)
- `server/` - thin HTTP/WebSocket
- `client/web/` - PWA (served from disk via -web flag)

### Dev Mode
Server runs with `-web=/home/exedev/malten/client/web` - changes to JS/CSS are instant, no rebuild.

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

### Clickable places:
- All data embedded in context (no API calls on click)
- Single place shows name (Boots, Waitrose)
- Multiple shows count (8 places)
- Click expands to card with address, hours, phone, map link

### Spatial caching:
- Weather: 5km radius
- Prayer: 50km radius
- Transport: per stop, extended TTL if API fails
- Location: 500m radius, 1hr TTL

---

## Recent Commits
```
2488012 Remove stream UI - no more share/new/goto
18c1644 Add location change cards as you move
3c99a97 Seamless geohash streams - silent switching
28868a6 Document seamless spatial experience
e0b8b82 Agents index everything - zero API calls in GetLiveContext
ebcc88a Spatial caching for external APIs
e7c12aa Keep arrivals when TfL API returns empty
cfa84b1 Include business name in Maps URL
e40ff52 Map links on new line, underlined
6523def Embed all places data - zero API calls
```

---

## UI State
- Clean header: just logo
- Prompt: "Ask anything..." (natural language)
- No visible stream/hash
- No share/new/goto links
- Cards persist in localStorage (version 2)
