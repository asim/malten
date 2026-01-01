## QUICK START

```bash
systemctl status malten  # running on :9090
curl -s "http://localhost:9090/ping" -X POST -d "lat=51.417&lon=-0.362" | jq .
```

## Architecture

### Primitives
- **Streams** - text representation of geo space, consciousness, where messages flow
- **Agents** - background processes that index/maintain world (weather, TfL, places)  
- **Commands** - interface to query agents (nearby, place, reminder)
- **Database** - quadtree spatial index (spatial.json)
- **Events** - replayable log (events.jsonl)

### Spatial Context
What you see based on where you are:
- 📍 Location (street, postcode)
- Weather, temperature
- 🕌 Prayer times
- 🚧 Traffic disruptions (TfL API - within 5km)
- 🚏 Bus/train arrivals (nearest stop with actual arrivals)
- ☕🍽️💊 Nearby places

### Commands
- **nearby** - "cafes nearby", spatial search
- **place** - "what time does X close", specific place info  
- **reminder** - Islamic content (Quran/Hadith)

## UI Layout

```
[Context box]     <- ALWAYS at top, live view of NOW
[Messages area]   <- Cards below, reverse chronological (newest top)
  - newest card
  - older card
```

### Q+A Cards
Question and answer combined into single card:
```
[Card]
  cafes nearby           <- question at top
  ─────────────────
  📍 NEARBY CAFES        <- answer below
  • Sun Deck Cafe...
```

## Recent Fixes (This Session)

### ✅ Q+A Cards
- Question and answer now combined into single card
- Shows "..." while waiting for response
- Purple left border for Q+A cards

### ✅ Bus Data Reliability
- Fixed TTL extension when cache exists (was extending stale data)
- Check both directions of bus stops (N/S)
- Increased search radius to 500m
- Skip stops with no arrivals, find next with actual arrivals
- Fallback to TfL API when cache empty

### ✅ Traffic Disruptions
- Added TfL Road Disruption API
- Shows serious/moderate disruptions within 5km
- 🚧 for roadworks, 🚨 for accidents

### ✅ Quadtree Optimization
- KNearest now collects all candidates first
- Deduplicates via map, sorts once at end
- Much faster for spatial queries

## Recent Commits
```
5b48075 Add traffic disruptions to spatial context (TfL API)
7b9fa12 Remove price command - not spatial
a17c392 Fix: increase bus search radius to 500m for both cache and fallback
c9ef222 Fix: check both directions of bus stops
e536d8c Fix: increase fallback bus stop search radius
abd838e Fix: skip stops with no arrivals
5d35394 Fix: don't extend TTL when skipping fetch
2f32f56 Q+A combined into single card
```

## Key Files
- `spatial/` - quadtree, agents, live data fetching
- `command/` - nearby.go, reminder.go
- `server/` - HTTP/WebSocket, streams, channels
- `client/web/` - PWA

## Quadtree
Separate repo: `/home/exedev/quadtree`
- Uses Haversine for distance
- WGS-84 ellipsoid for Earth radius
- Optimized KNearest (94c8fa7)
