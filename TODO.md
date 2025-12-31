# Malten TODO

## Current State

- [x] Basic ephemeral messaging
- [x] WebSocket real-time updates
- [x] Fanar/OpenAI AI integration
- [x] Slash commands (/help, /streams, /new, /goto)
- [x] PWA with proper icons
- [x] Stream switching via hash

## TODO

- [ ] Fix PWA splash screen icon sizing for Android
- [ ] Service worker for offline support
- [ ] Client-side encryption (AES-256)
- [ ] Secure pipes (stream between two users)
- [ ] Decentralisation (interconnect multiple servers)
- [ ] Speech to text
- [ ] WhatsApp integration (currently removed)

## Recent Changes

- Refactored agent to be self-contained (Prompt function)
- Removed presence indicator
- Fixed websocket reconnection on stream switch
- Default stream changed from `_` to `~`
- AI responds to @malten mentions (no /malten prefix needed)
- Added proper PWA icons (192x192, 512x512, maskable)

## Notes

- Streams expire after 1024 seconds idle
- Max 1024 messages per stream (FIFO)
- Max 1024 characters per message
