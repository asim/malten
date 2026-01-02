# Session Notes - Jan 2, 2026

## Bugs to Fix

### 1. Context card not showing
- The `displayContext` function creates `#context-card` div but it may not be appearing
- Check if it's being inserted correctly before `#messages-container`
- May be a CSS issue or the element isn't being created

### 2. No response to messages
- WebSocket might not be connecting to correct stream
- Check `connectWebSocket()` - needs to connect to geohash stream based on location
- The stream changes when location changes but WS may not reconnect

### 3. Location changes not reflected
- User is driving, location changing
- `sendLocation()` should reconnect WS when stream changes
- `getLocationAndContext()` should also reconnect
- Context should update when location changes

## Key Files
- `/home/exedev/malten/client/web/malten.js` - main client code
- `/home/exedev/malten/client/web/malten.css` - styles  
- `/home/exedev/malten/client/web/index.html` - layout

## Recent Changes Made
1. Moved prompt to bottom of screen (chat-style layout)
2. Context card now created outside messages list, before `#messages-container`
3. Chat flows chronologically (oldest at top)
4. Removed mic button
5. Changed to "What's happening?" placeholder
6. Auto-scroll to bottom on new messages

## Debug Steps
1. Check browser console for errors
2. Check WebSocket connection - should connect to geohash stream
3. Check if `#context-card` element exists in DOM
4. Check if context is being fetched from `/ping` endpoint
5. Check server logs: `journalctl -u malten -f`

## Code Locations
- `displayContext()` - line ~649 in malten.js
- `connectWebSocket()` - line ~394
- `sendLocation()` - line ~930
- `getLocationAndContext()` - line ~954
- `insertCardByTimestamp()` - inserts new messages

## Server
- Running on port 9090
- Service: `sudo systemctl restart malten`
- Logs: `journalctl -u malten -f`
