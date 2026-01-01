## Last Session
- Added speech-to-text input (browser Web Speech API)
- Service worker for offline PWA support
- Fixed agent deduplication (find by name, not just location)
- Improved session continuity with SESSION.md

## Next Up
- Test speech recognition on mobile
- Consider adding visual feedback during speech recognition

## Key Decisions
- Speech uses browser Web Speech API (no server dependency)
- Auto-submits after speech ends
- Service worker uses cache-first for static, skips API calls
