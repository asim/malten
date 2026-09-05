# Malten

A minimal spatial timeline for shared thoughts, photos and reflections.

- Keep one input and a quiet stream. No dashboards, maps or engagement mechanics.
- Use points and connections when needed; avoid mathematical terminology.
- Hashtags navigate independent public streams.
- Preserve older browser-only captures. Never upload them automatically.
- Public captures expire with their media. Never publish exact location or EXIF.
- All human and agent posts pass through the same moderation path.
- Agent loops live in agent/, start with the server and stop on cancellation.
- Keep Go standard-library-only and the frontend embedded, with no build system.
- Preserve the PWA, root main.go entry point and existing server deployment.
- Run go test -race ./..., go vet ./..., go build . and node tests/streams.cjs.
