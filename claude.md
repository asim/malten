# Malten Development Context

## URLs
- **Production**: https://malten.ai
- **Local dev**: http://localhost:9090
- **Test via curl**: `curl -s -X POST 'http://localhost:9090/commands' -d 'prompt=/ping&lat=51.4497&lon=-0.3575'`

## CRITICAL: Production Safety

### Before Making Changes
1. **Never change data structures** without version migration
2. **Never delete localStorage keys** - add new ones, deprecate old
3. **Test on browser** before saying "done" - use browser_take_screenshot
4. **Bump version numbers** - Run `./bump-js.sh` to bump JS version in both places
5. **Restart via systemctl** - not manual process killing

### Development Flow
```bash
# Edit code
vim client/web/malten.js

# JS changes: just bump version (dynamic loading enabled)
# Go changes: rebuild + restart
go build -o malten . && sudo systemctl restart malten

# Check status
sudo systemctl status malten
journalctl -u malten -f  # tail logs

# Test
curl -s -X POST 'http://localhost:9090/commands' -d 'prompt=/ping&lat=51.4179&lon=-0.3706'
```

### State Version Migration
When changing `state` structure in malten.js:
1. Increment `state.version` (currently 3)
2. Old data will be cleared on load (except lat/lon preserved)
3. Document what changed in this file

### Error Handling Principles
1. **Never lose cached data** - if API fails, extend TTL of existing data
2. **Log errors clearly** - include context (lat/lon, what failed)
3. **Graceful degradation** - show stale data with indicator rather than nothing

## What is Malten

A spatial timeline for the real world. Context-aware of where you are and what's around you.

### The Public Face

To the world, Malten is:
- A timeline of your movement through space
- Contextual awareness - weather, transport, places
- An alternative to algorithmic feeds
- Mindfulness through noticing the world
- No ads, no tracking, no manipulation

### Design Principles

1. **Subtle, not preachy** - No Arabic terms in the UI for non-Muslim audiences
2. **Universal accessibility** - Works for everyone
3. **The tool helps you see, it doesn't tell you what to see**
4. **Utility and purpose together** - Every feature has practical value

## Session Checkpoint: Jan 10, 2026

### What Was Done This Session

1. **Fixed push notification history not appearing in timeline**
   - `HandlePushHistory` was hardcoded to return empty array
   - `sendPush` wasn't storing notifications to `user.PushHistory`
   - Fixed both: notifications now stored on successful send, returned and cleared on fetch
   - Also fetch history on app cold start (not just on visibility change)

2. **Fixed broken street geometry (847 entries with 1 point)**
   - Buggy backfill on Jan 9 created street entries with only location, no geometry
   - These blocked proper geometry from being fetched later
   - Added `CleanupBrokenStreets()` function and `/cleanup-streets` command
   - Removed 847 broken entries, re-running backfill to get proper geometry

3. **Fixed initial load not refreshing context on PWA resume**
   - PWA resume fires `$(document).ready()` again, appearing as "initial load"
   - `getLocationAndContext()` was slower (15s timeout) and didn't fall back to cached location
   - Now uses `locationManager.update()` (5s timeout, cached fallback) when we have cached location
   - Only uses `getLocationAndContext()` for truly fresh starts needing permission prompts

### Files Changed
- `server/push.go` - Store notifications in history, implement history retrieval
- `spatial/streets_backfill.go` - Added `CleanupBrokenStreets()` function  
- `command/backfill.go` - Added `/cleanup-streets` command
- `server/map.go` - Increased street query limit from 500 to 2000
- `client/web/malten.js` - Fetch push history on cold start; use fast refresh on PWA resume

---

## Session Checkpoint: Jan 9, 2026

### What Was Done That Session

1. **Fixed deadlocks in push notification system**
   - `PushAwarenessToArea` was calling `saveAsync()` while holding `pm.mu.Lock()`, but `saveAsync()` internally tries to acquire `RLock()` → deadlock
   - `IndexAgent` global mutex caused all 34 agents to queue sequentially on startup (2-3 min each)
   - Fixed with `TryLock()` and skip-if-has-data logic

2. **Simplified push notifications drastically**
   - Removed: prayer-time notifications, duha, night star, bus notifications, weather warnings, awareness pushes
   - Kept: **One morning context push at 7am** with weather + prayer times
   - `server/push.go` reduced from ~800 lines to ~280 lines

3. **Stripped dead code (~1,300 lines removed)**
   - Deleted `spatial/awareness.go` (289 lines) - LLM filtering "what's interesting"
   - Deleted `spatial/agentic.go` (365 lines) - LLM-driven agent behavior (was disabled)
   - Deleted `spatial/explorer.go` (448 lines) - Street exploration (was disabled)
   - Deleted `command/agentic.go`, `command/explore.go`, `command/observe.go`
   - Total codebase: 15,823 → 14,552 lines

4. **Fixed AI context for queries**
   - "What's here" wasn't seeing location context
   - Added `updateUserContext()` call in `SetLocation()` so AI queries have context
   - Added "what's here", "what is here" to `isContextQuestion()` patterns

5. **Fixed /nearby command output format**
   - Now matches client place-link expansion format
   - Includes clickable Map and Directions links (HTML)
   - Uses same `map-link` and `directions-link` classes as client

6. **Fixed Wikimedia category for rain** - "Rain" had no files, changed to "Rainy_weather"

### Key Architectural Decisions

**LLM Usage (Simplified)**
- LLM is now ONLY used for chat assistant in `agent/agent.go`
- User asks questions, LLM answers using context + tools
- NO LLM for: filtering observations, background agent behavior, deciding "what's interesting"
- Philosophy: "The tool helps you see, it doesn't tell you what to see"

**Future LLM use cases (not implemented)**
- Detecting mapping gaps ("this area has no cafes indexed")
- Area summaries ("Whitton: residential area with good transport links")
- Things-to-do digests

### Files Changed

- `server/push.go` - Complete rewrite, simplified to morning-only
- `spatial/agent.go` - Removed awareness/agentic/explorer references
- `spatial/routing.go` - Added `DistanceMeters`, `GetWalkingRoute`, `queryOSMPOIsNearby`
- `spatial/stats.go` - Added `formatTimeAgo`, `truncate` helpers
- `spatial/nature.go` - Fixed rain category
- `command/nearby.go` - Format matches client place-link expansion with clickable links
- `command/system.go` - Removed agentic mode display
- `command/context_questions.go` - Removed bus notification toggle
- `command/command.go` - Removed bus notify callbacks
- `agent/agent.go` - Added "what's here" to context patterns
- `main.go` - Simplified push callback setup

### Commands Reference

| Command | Description |
|---------|-------------|
| /ping, /context | Get current context (weather, prayer, bus, places) |
| /nearby <type> | Find places nearby with Map/Directions links |
| /directions <dest> | Walking directions |
| /bus | Next bus times |
| /weather | Weather forecast |
| /reminder [type] | Islamic reminder |
| /nature [type] | Nature image (stars, moon, rain, etc) |
| /video <query> | Video search (mu.xyz) |
| /clear | Clear timeline (keeps photos) |
| /reset | Full reset + clear SW cache |

### Next Steps

- ✅ Test the nearby command output renders correctly with clickable links in browser (Done Jan 9)
- ✅ Monitor morning push notification at 7am (Logic verified - sends at 7:00-7:05am local time)
- Consider area summaries / things-to-do digests using LLM

## Testing Notes (Jan 9, 2026)

### Nearby Links Browser Test
- Map links: Work correctly, open map centered on place with highlight
- Directions links: Work correctly, trigger /directions command
- Display: Shows place name, address (if available), Map · Directions links

### Morning Push Test
- Test endpoint added: `POST /push/test-morning`
- Notification content verified:
  - Title: "🌅 Good morning"
  - Body: Weather + rain warning + prayer time (e.g., "🌧️ 3°C · 🌧️ Rain likely now (88%)\n🕌 Dhuhr 12:08")
  - Image: Wikimedia morning image
- Timing: 7:00-7:05am user local time (estimated from longitude)
- Debug logging: Logs when approaching 7am window (6:55+)
