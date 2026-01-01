## Last Session
- Added service worker for offline PWA support
- Fixed agent deduplication (find by name, not just location)

## Next Up
- Test offline mode in browser
- Consider speech-to-text (browser API)

## Key Decisions
- Agents are identified by area name to prevent duplicates
- Service worker uses cache-first for static, skips API calls
