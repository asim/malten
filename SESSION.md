## Last Session - 2026-01-01 14:35 UTC

### What We Built
1. **Commands as core abstraction** - dispatch moved from server to command package
   - Commands self-register with Name, Handler, optional Match function
   - Server is thin - just routes to command.Dispatch()
   - Natural language matching in command package, not server

2. **Proactive cards** - client detects context changes, creates cards automatically
   - New bus stop approached → "🚏 Arrived at Whitton Station"
   - Rain forecast appears → "🌧️ Rain at 15:00"
   - Prayer time changes → "🕌 Asr now"

3. **Cards persist 24hr in localStorage** - personal timeline survives refresh
   - Stored with timestamp and location
   - Pruned on load (>24hr removed)
   - Rendered chronologically on page load

4. **Card styling** - timestamps in top-right, color-coded borders
   - Transport = blue, Weather = orange, Prayer = green, Location = red

5. **Status indicator** - pulsing blue dot with "Updating..." when fetching

6. **Postcodes** - shows "TW2 6JG" instead of obscure names like "Worton"

7. **Quadtree-first lookups** - bus arrivals from cache, not TfL API
   - Agent indexes arrivals every 30s
   - GetLiveContext queries quadtree first
   - Falls back to API only if no cached data

### Architecture

```
User pings location
  → Quadtree lookup for agent
  → If no agent, create one
  → Agent starts loop:
      - Live data every 30s (weather, prayer, buses)
      - POI index once (cafes, pharmacies, etc from OSM)
  → Context built from quadtree (fast, cached)
  → Client detects changes → creates cards
  → Cards persist in localStorage (24hr)
```

### Data in Quadtree (spatial.json)
- **EntityPlace** - POIs from OSM (cafes, pharmacies, etc)
- **EntityAgent** - area indexers, 5km radius
- **EntityPerson** - users by session
- **EntityWeather** - current weather + rain forecast (10min TTL)
- **EntityPrayer** - prayer times (1hr TTL)
- **EntityArrival** - bus arrivals per stop (2min TTL)

### What Agents Should Do More Of
- Index more POI categories
- Detect events (what's happening nearby today)
- Learn patterns (user at location X for 8hrs → probably work)
- Predict destinations (leaving work → probably going home)
- Proactive suggestions ("Rain in 30min, you're 20min from home")

### User's Vision
"I'm walking. I need information without typing:
- Weather, rain forecast
- Prayer times
- Bus stop arrivals, what street I'm on
- Sometimes: coffee shop nearby? How long to walk home?

This is Foursquare if built in the AI era - automatic, contextual, proactive."

### Files Changed This Session
- `command/command.go` - Dispatch with Context, natural language Match
- `command/walk.go` - walking directions with Match function
- `command/nearby.go` - nearby search with Match function
- `server/handler.go` - thin, just routes to command.Dispatch
- `spatial/live.go` - quadtree-first for arrivals, haversine, postcodes
- `client/web/malten.js` - cards, persistence, status indicator
- `client/web/malten.css` - card styling
- `client/web/index.html` - status div

### Git Log
```
0379518 Query quadtree for bus arrivals before TfL API
c86475a Persist cards in localStorage for 24 hours
fd61a8e Cards with timestamps, status indicator, postcodes
975a96a Refactor: commands as core abstraction, proactive cards
2a814d7 Add rain forecast and walking directions
```

### To Continue
1. Agents should be smarter - predict, suggest, learn patterns
2. Geohash streams - auto-switch based on location
3. More proactive - don't wait for queries
4. Events - what's happening nearby today
