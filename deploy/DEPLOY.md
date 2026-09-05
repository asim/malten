# Deployment

Malten runs as one Go binary behind the existing nginx server.

Pushes to main build and test ./cmd/malten, upload the binary over SSH and restart
the existing systemd service. The deployment path remains /home/malten/malten.
No domain change or new service is required.

Configuration is read from /home/malten/.env by systemd; see
[env.example](env.example). Moderation reads ANTHROPIC_API_KEY or the existing
/home/malten/anthropic_key file. Keep secret files readable only by the service
user. Without a working key, reading works but publication is blocked.

MALTEN_REMINDER=true enables the Reminder agent. It checks /api/latest at startup
and every six hours, skips duplicate reflections within that process lifetime,
and publishes into #reminder through the normal moderation path. It cancels with
the server. MALTEN_MODERATION_MODEL defaults to claude-sonnet-5.

Shared posts and photos are held only in memory: 500 posts maximum, 400 KB per
photo, expiring after 24 hours or earlier on restart/capacity eviction. A restart
clears reports and the short-lived rate-limit state too. No database is needed.
Older local captures remain on the browser that created them.

The server limits concurrent moderation calls and rate-limits posting/reporting
by IP. Only loopback proxy requests can supply X-Real-IP; nginx must overwrite it.
HTTPS is needed for camera, microphone and location permissions. The existing
1 MB nginx upload limit accommodates the capped JSON photo uploads.

The local stream uses 0.01-degree cells (about 1.1 km north–south, narrower
east–west depending on latitude). It is approximate, public and not proof of
presence. Exact coordinates are never sent by the UI. Server-side JPEG
re-encoding removes photo metadata. Voice recognition may use the browser's
speech provider. Shared text and photos are sent to Anthropic for moderation.

The PWA shell works offline. Sharing requires a connection and never queues a
private capture for later automatic publication. Moving domains changes browser
storage and the installed PWA's origin; it does not require changing the server.
