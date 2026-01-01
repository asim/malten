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

### Streams with Channels
```
Stream: gcpsxb (Hampton area)
├── (public) - spatial events everyone sees
├── @session123 - my questions/responses (private)
```
- Empty channel = public
- @session = addressed to that session

### Commands (thin layer that queries agents)
- **nearby** - "cafes nearby", spatial search
- **place** - "what time does X close", specific place info
- **reminder** - Islamic content

Everything else falls through to AI.

## UI Layout

```
[Context box]     <- ALWAYS at top, live view of NOW
[Messages area]   <- Cards below, reverse chronological (newest top)
  - newest card
  - older card
```

### Card Structure
Question and answer should be ONE card:
```
[Card]
  Q: cafes nearby
  A: NEARBY CAFES (cached)
     • Sun Deck Cafe...
     • Chino's Cafe...
```
Not two separate cards. Response belongs with its question.

## Current Issues

### ✅ Card structure (FIXED)
- Question and answer now combined into single card
- Question appears at top, answer below
- Uses pending card pattern - shows "..." while waiting

### ✅ Bus data intermittent (FIXED)
- Root cause: when fresh cache existed, skipping fetch but still calling ExtendTTL
- This kept stale data alive indefinitely
- Fix: nil = skipped (don't extend TTL), empty slice = API returned nothing (extend TTL)

## Recent Commits
```
62c540d Fix: cards reverse chronological (newest top)
8d8b258 Rename placeinfo to place
73829c2 Remove unused commands
f606548 Remove slash commands - spatial-first
dde1896 Channels within streams: private session messages
```

## Key Files
- `spatial/` - quadtree, agents, live data fetching
- `command/` - nearby.go, reminder.go
- `server/` - HTTP/WebSocket, streams, channels
- `client/web/` - PWA

## Next
1. Combine question + answer into single card
2. Investigate bus data reliability
