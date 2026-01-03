# Malten Architecture

## The Spacetime Model

This is the law. Read before changing anything.

```
events.jsonl     = cosmic ledger (facts about the world, append-only)
spatial.json     = materialized view (quadtree, rebuildable from events)
stream/websocket = real-time event propagation to observers
localStorage     = user's worldline (private, client-side only)
```

## What Goes Where

### events.jsonl (server persists)
- Entity CRUD (places, weather, transport, agents)
- Public broadcasts (channel="")
- System events

### localStorage (client persists)  
- User queries
- AI responses
- Conversations
- Timeline cards
- Location history

## Privacy Rule

```
if channel != "" {
    // PRIVATE - do not persist to events.jsonl
    // belongs in user's localStorage
}
```

## Code Enforcement

1. `spatial/event.go:LogMessage()` - guards against logging private messages
2. `server/server.go` - comments explain why user messages aren't persisted
3. `client/web/malten.js:initialLoad()` - comments explain localStorage is source of truth

## Key Features to Preserve

- **Adaptive ping**: 5s driving, 10s walking, 30s stationary
- **Check-in**: GPS correction for indoor (manual location override)
- **Directions**: OSRM routing with step-by-step
- **Prayer times**: Current/next prayer, Fajr ends before sunrise
- **Agent loop**: Background indexing every 30s
- **Foursquare fallback**: When OSM returns nothing
- **Supplementary data**: Cinema chains, etc.
- **Context JSON**: Structured response from /ping with places, weather, prayer

## On New Conversation

Read claude.md "The Spacetime Model (CANONICAL)" section.
