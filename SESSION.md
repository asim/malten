## QUICK START - Read in 1 Minute

### What is Malten
Spatial AI. Open app → see world around you. Weather, prayer, buses, places. Moves with you.

### Five Primitives (NEVER CHANGE)
- **Streams** - geo areas as text channels
- **Agents** - index areas, build world view
- **Commands** - all actions
- **Database** - quadtree (`spatial.json`)
- **Events** - replayable log (`events.jsonl`)

### Key Files
- `spatial/` - quadtree, agents, live data
- `command/` - all commands (dispatch here, not server)
- `server/` - thin HTTP/WebSocket
- `client/web/` - PWA, localStorage for personal timeline

### Run
```bash
cd /home/exedev/malten
systemctl status malten  # should be running on :9090
journalctl -u malten -f  # logs
```

### Test
```bash
curl -s "http://localhost:9090/ping" -X POST -d "lat=51.417&lon=-0.362" | jq .
```

### Current State
- Agents index POIs from OSM, live data every 30s
- Context from quadtree (fast), falls back to API
- Messages persist 24hr in localStorage
- Cards = message format with timestamp

---

## START HERE - Bugs Fixed

### Bugs Fixed (2026-01-01 16:00)

1. ✅ **Location not updating** - Fixed: reduced maximumAge, enabled high accuracy
2. ✅ **Cards persist** - Cards stored in localStorage, survive refresh
3. ✅ **Local info clickable** - "3 cafes" etc. now link to /nearby
4. ✅ **Context unified** - Location, weather, prayer, bus all in context block
5. ✅ **Bus info showing** - Increased arrival query radius from 100m to 300m

### To Debug
```bash
# Check Hampton area in quadtree
curl -s "http://localhost:9090/ping" -X POST -d "lat=51.417&lon=-0.362" | jq .

# Check localStorage in browser console
JSON.parse(localStorage.getItem('malten_state'))

# Check agent logs
journalctl -u malten --since "5 minutes ago" | grep -i hampton
```

---

## The Model

### User Experience
Open app → see real world around you instantly:
- Weather, prayer times, area name
- Bus/train times, nearby places
- Move → updates automatically
- Timeline of messages
- Same area = same view (shared spatial reality)

### Five Primitives
| Primitive | Purpose |
|-----------|----------|
| **Streams** | Textual view of geo space. One per area. |
| **Agents** | Indexers. One per area. Build world view. |
| **Commands** | Actions. Everything is a command. |
| **Database** | Quadtree. Spatial index. World state. |
| **Events** | Replayable log. `events.jsonl` |

### Key Insight
- **Stream = Geo Area** - moving through space = moving through streams
- **Agent = Per Stream** - each area has an agent maintaining it
- **Messages = Events in Space** - what happens in that area
- **Message Formats** - card, map, list, text (presentation layer)

### Storage
- **Quadtree** (`spatial.json`) - spatial index, server-side, shared
- **localStorage** - personal timeline, client-side, 24hr, free
- **Server-side personal** - cross-device sync, paid feature (future)

---

## Last Session Summary - 2026-01-01

### Built
- Commands as core abstraction (dispatch in command pkg)
- Proactive messages (detect context changes)
- Message persistence (24hr localStorage)
- Message deduplication (prevent duplicates)
- Expanded agent indexing (trains, tubes, bus stops, more POIs)
- Quadtree-first lookups (cache before API)
- Status indicator, timestamps, postcodes

### Agent Indexes
Transport: stations, bus stops | Food: cafes, restaurants, pubs
Health: pharmacies, clinics | Services: banks, ATMs, post offices
Shopping: supermarkets, bakeries | Live: weather, prayer, arrivals (30s)

### Git
```
a832731 Document bugs
bc04e8e Document the model
094dc25 Messages are primitive, formats are presentation
ff27847 Deduplicate cards
396aa26 Expand agent indexing
```

---

## Next Up (after bugs)

1. **Geohash → Stream** - Auto-switch stream based on location
2. **Agent messages to stream** - Post events to geo stream, not just quadtree
3. **Map message format** - Spatial view
4. **Agent learning** - Predict, patterns
