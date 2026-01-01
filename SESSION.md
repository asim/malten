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

## Current Issues

### Card ordering
- Cards display newest at top
- User wants chronological (oldest top, newest bottom)?
- Or context always at top, separate from cards?

### Bus data intermittent
- Sometimes shows "no buses" even when TfL has data
- Restart fixes it - something with in-memory state getting stale
- Need to investigate quadtree query or entity loading

## Recent Commits
```
8d8b258 Rename placeinfo to place
73829c2 Remove unused commands
f606548 Remove slash commands - spatial-first
dcb8e3a Display loaded messages as cards with timestamps
dde1896 Channels within streams: private session messages
6e5c195 Persist cards to localStorage, HTTP commands
72d94e6 Place info command
4e61bb0 Compute prayer display at query time
```

## Key Files
- `spatial/` - quadtree, agents, live data fetching
- `command/` - nearby.go, reminder.go (thin query layer)
- `server/` - HTTP/WebSocket, streams, channels
- `client/web/` - PWA

## Next
1. Fix card ordering (decide on layout)
2. Investigate bus data reliability
3. Push to origin
