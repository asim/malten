# Micro integration

Malten uses `MU_API_KEY` as a Bearer credential only at `https://micro.mu/mcp`.
The integration has a fixed read-only allowlist: `news_list`, `news_search`,
`places_geocode` and `weather_forecast`. It does not expose account mail, files,
calendar, tasks, messaging or publishing tools to the reflection model.

News can search for a relevant event rather than relying on the latest headlines.
Nature can resolve an explicitly named place and fetch current weather, preserving
provider dates and limitations. Existing context and source agents remain usable.
No GPS prompt or automatic location upload is introduced.

Reminder and Aslam continue to retrieve their texts directly. A live test of Mu's
`prayer_search` returned primary references alongside an AI-generated answer;
Malten's direct Reminder search disables that extra answer instead.

The existing Islamic foundation, content-focused writing, photo handling,
retrieved-source validation, moderation and durable summary jobs remain in Malten.
Tool results are untrusted data, never instructions. Source errors do not justify
invented context. Mu credentials are never forwarded on redirects or exposed in
browser code or error messages.

## Managed agent endpoint

Inspection of micro/mu at commit 06d60fd2b043a7a510169d7be72dc5ddbecab39d found
`POST /agent/malten` accepts `prompt` (or `text`) and an optional `thread`.
It returns text, thread, agent and flow identifiers. The handler has no image
field, silently truncates input to 8,000 bytes and records a conversation through
Ask. It offers no request-level ephemeral mode or source-reference response field.
Consequently this change does not route Malten captures through that endpoint:
doing so would lose photo support, truncate longer reflections and change where
conversations are retained. Micro needs an image-capable, bounded and explicit
retention contract before it can replace the full reflection path.

Live MCP tests in this session succeeded for source search, news list/search,
place lookup and weather. The connector credential worked after reauthentication.
The deployed Malten MU_API_KEY is not available in the local workspace, so the
managed HTTP agent was inspected in code, not live-tested with that credential.
