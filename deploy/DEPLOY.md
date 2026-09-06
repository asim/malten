# Deployment

Malten runs as one Go binary behind the existing nginx server.

Pushes to main build and test ., upload the binary over SSH and restart
the existing systemd service. The deployment path remains /home/malten/malten.
No domain change or new service is required.

Configuration is read from /home/malten/.env by systemd; see
[env.example](env.example). Moderation reads ANTHROPIC_API_KEY or the existing
/home/malten/anthropic_key file. Keep secret files readable only by the service
user. Without a working key, reading works but publication is blocked.

Reminder and Aslam boot with the system and cancel when it stops; no agent
switches are needed. Reminder checks /api/latest at startup
and every six hours, skips duplicate reflections while the original post is still live,
and publishes into #reminder through the normal moderation path. It cancels with
the server. Aslam sources adhkar and nature photos for its day streams.
MALTEN_MODERATION_MODEL defaults to claude-sonnet-5.

Shared posts and photos are saved atomically to data/stream.json relative to the
working directory (/home/malten/data/stream.json with the existing systemd unit).
Set MALTEN_DATA to override the file path. The directory must be writable by the
service user. Deployments replace only the binary and leave this data intact.

The snapshot contains up to 500 posts and their photos (400 KB each), anonymous
ownership hashes, quarantine and review state. Each successful mutation is saved
before acknowledgement. Posts keep their original 24-hour expiry across restarts;
startup and periodic cleanup purge expired media. While the server is stopped,
cleanup resumes on the next start. The short-lived IP rate limits reset on restart.
No database dependency is needed. Run only one server process against this file.

The snapshot is private (0600), with atomic replacement and no rolling archive.
Unfinished temporary snapshots are removed at startup. Invalid or inaccessible
storage prevents startup rather than silently losing the stream. Do not include
this ephemeral data directory in long-lived backups. Older local captures remain
on the browser that created them.

The server limits concurrent moderation calls and rate-limits posting/reporting
by IP. Only loopback proxy requests can supply X-Real-IP; nginx must overwrite it.
HTTPS is needed for camera and microphone permissions. The existing
1 MB nginx upload limit accommodates the capped JSON photo uploads.

The browser uses its configured IANA timezone as its home stream: lowercase,
slashes and underscores become hyphens (Europe/London becomes europe-london).
Plus signs become -plus- for fixed-offset timezone names. The UTC offset is not
used, so daylight-saving changes do not change streams. No GPS or IP geolocation
is used. The readable stream name is sent, not coordinates.
Server-side JPEG
re-encoding removes photo metadata. Voice recognition may use the browser's
speech provider. Shared text and photos are sent to Anthropic for moderation.

The PWA shell works offline. Sharing requires a connection and never queues a
private capture for later automatic publication. Moving domains changes browser
storage and the installed PWA's origin; it does not require changing the server.
