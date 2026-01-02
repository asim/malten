# Malten Session - Jan 2 2026

## What We Fixed

### Bus times disappearing
- Root cause: corrupted spatial.json (truncated mid-write)
- Fixed: atomic file writes (write to .tmp, then rename)
- Fixed: client won't replace good cached context with empty response

### Fetch on demand
- User never waits for agents - data fetched immediately if not cached
- Each category (cafe, restaurant, pharmacy, supermarket) checked individually
- Agents enrich in background after user is served

### Stream/Cards
- Stream TTL now 24 hours (was 17 min)
- Cards persist in localStorage, load on refresh
- Reverse chronological order (newest first)
- Date separators: "Yesterday", "Tuesday" etc
- Removed noisy bus stop "arrived at" cards

### News
- BBC UK headline as separate card in stream
- Includes link to article
- Cached 30 min on server, shown once per 30 min to user

### Prayer times
- Shows "Fajr ends 08:07" (sunrise time)
- Shows "Fajr ending 08:07" when within 15 min of sunrise

### Commands/Messages
- User message shows immediately (blue background)
- "..." loading indicator while waiting
- Response appears as new message
- Dedupes echoed input from WebSocket

### UI fixes
- Card padding so text doesn't overlap timestamp
- URLs in nearby results now clickable ("Open in Maps")
- Debug command: type "debug" to see stream ID, location, cache info

## Quadtree changes
- Reverted KNearest optimization (was breaking queries)
- Atomic file writes to prevent corruption

## Files changed
- spatial/live.go - fetch on demand, news, prayer display
- spatial/entity.go - EntityNews type
- spatial/db.go - load logging
- spatial/agent.go - serialize indexing, logging
- server/location.go - news in ping response
- server/server.go - 24hr TTL
- client/web/malten.js - cards, loading, user messages
- client/web/malten.css - card styling, date separators, loading
- quadtree/store.go - atomic writes
- README.md - updated for spatial AI focus

## Open issues
- None critical

## To continue
- All changes committed and pushed
- Server running on port 9090
