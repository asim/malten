## Core Architecture - READ THIS FIRST

### Three Primitives (NEVER CHANGE)
1. **Streams** - `/streams` - message channels
2. **Agents** - `/agents` - spatial indexers  
3. **Commands** - `/commands` - actions

### Quadtree = Spatial Index
- `spatial.json` - source of truth
- Maps and stores everything
- Real-time world view

### Cards = Stream Messages
- Everything is a message on the stream
- Cards are the standard format
- Personal timeline in localStorage (24hr)

---

## Last Session - 2026-01-01 14:45 UTC

### What We Built
1. **Expanded agent indexing** - trains, tubes, bus stops, more POIs
2. **Transport arrivals** - buses, tubes, rail from TfL
3. **Card deduplication** - no duplicates within time window
4. **Card persistence** - 24hr in localStorage

### Agent Categories Now Indexed
- Transport: railway=station, highway=bus_stop, public_transport=station
- Food: cafe, restaurant, fast_food, pub, bar
- Health: pharmacy, hospital, clinic, dentist, doctors
- Services: bank, atm, post_office, fuel, parking
- Shopping: supermarket, convenience, bakery, butcher
- Other: place_of_worship, hotel, park, library

### Live Data (every 30s)
- Weather + rain forecast
- Prayer times
- Bus arrivals (NaptanPublicBusCoachTram)
- Tube arrivals (NaptanMetroStation)
- Rail arrivals (NaptanRailStation)

### Git Log
```
ff27847 Deduplicate cards
396aa26 Expand agent indexing: trains, tubes, more POIs
4687b59 Update docs for session continuity
0379518 Query quadtree for bus arrivals before TfL API
c86475a Persist cards in localStorage for 24 hours
fd61a8e Cards with timestamps, status indicator, postcodes
975a96a Refactor: commands as core abstraction
```

### To Continue
1. Agents should predict, learn patterns
2. Geohash streams - auto-switch by location
3. More proactive suggestions
4. Events - what's happening nearby
