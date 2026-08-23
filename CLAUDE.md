# CLAUDE.md

Guidance for AI agents working in this repository. Kept in-repo because it
carries engineering invariants, not product docs.

## What this is

Malten is a **spatial-exploration** app for Great Britain: a map you move
through, where you drop "finds" (a note anchored to its Ordnance Survey National
Grid reference) as you explore. It ships as a single Go binary with the UI — and
a vendored copy of Leaflet — embedded.

Ordnance Survey (OS) is the authoritative substrate: the map tiles come from the
**OS Maps API**. Malten is the experience layer on top. The moat is the
experience and the spatial memory, not the map data.

**The server is stateless and anonymous.** It stores nothing about users. Your
finds live in the browser (`web/app.js`, localStorage). Map tiles come from the
OS Maps API one of two ways: if the operator sets a shared `OS_API_KEY`, the
server proxies tiles (`/api/tiles`, `internal/server/tiles.go`) so visitors need
no key of their own and the key stays server-side; otherwise each visitor enters
their own **OS Data Hub key** in the UI and the client fetches tiles *directly*
from the OS with it — the server never sees that per-user key.

Beyond serving the UI, the server does two things, and **neither stores user
content**:

- **Proxies public live data.** `/api/stops` and `/api/arrivals` proxy the
  Transport for London Unified API (free, keyless) so the map can show live
  buses/tube (London only) — proxied server-side because TfL blocks the default
  Go User-Agent and an optional `TFL_APP_KEY` raises the rate limit
  (`internal/server/tfl.go`). `/api/stations` (nearest National Rail stations,
  keyless, from a vendored dataset) and `/api/departures` (live train boards via
  National Rail's Darwin OpenLDBWS SOAP service) add **nationwide rail**; the
  boards need a free `NRE_LDBWS_TOKEN` on the server (`internal/nrail`).
  `/api/buses` adds **nationwide live buses** from the Bus Open Data Service's
  SIRI-VM feed (vehicle positions in a bounding box; free `BODS_API_KEY`,
  `internal/bods`) — SIRI-VM XML, not GTFS-RT protobuf, to stay dependency-free.
- **Runs a spatial agent.** Two ways in, neither a chat window. Ambiently:
  `/api/suggest` (`internal/server/suggest.go`) runs the agent silently over the
  live "around me" snapshot and returns ONE concrete nudge, surfaced in the
  timeline and the "Around you" card. On demand: a composer at the foot of the
  timeline posts to `/api/ask` (SSE) — the bounded tool-use loop, with the
  live-data calls above plus `/api/gridref` exposed as tools — and the streamed
  answer lands as an entry in the feed, so the timeline stays the surface. It's a
  conversation with a memory of the day: the browser replays the exchange so far
  (`history`) **and the trail** (`trail` — where you've been, what was around you
  there, what was already suggested) with each question, because the server
  remembers nothing between turns. Both are enabled only when
  `ANTHROPIC_API_KEY` is set; the question, history, trail and location are used
  per-turn and forgotten.

- **Place search & reverse geocoding** (`/api/search`, `/api/nearest`,
  `internal/server/search.go`). Proxies the **OS Names** gazetteer with the
  shared server OS key, so it's enabled exactly when the tile proxy is. OS Names
  returns British National Grid only, so every result is projected back to WGS84
  with `osgrid.ToWGS84` before it leaves the server. Also exposed to the agent
  (`find_place`, `whats_here`).

- **Nearby points of interest** (`/api/poi`, `internal/server/poi.go`). Proxies
  the OpenStreetMap **Overpass** API (free, keyless) for named features near a
  point — pubs, parks, landmarks, viewpoints — with their opening hours, website
  and phone where OSM has them. It's what the camera "look around" mode tags in
  view, and what lets the agent answer "is that café open?" (`nearby_places`)
  rather than deflect. Override the instance with `MALTEN_OVERPASS_URL`. OSM is
  ODbL — the UI attributes it.

- **A live "around me" snapshot** (`/api/around`, `internal/server/around.go`).
  Fans out across every feed above concurrently and folds them into one compact
  view (nearest place, nearest stations + next trains, buses moving nearby,
  London stops, and current **weather** from Open-Meteo — free/keyless,
  `internal/server/weather.go`). It powers the **timeline** — the app's front
  door, an append-only log of what happened: arriving somewhere, what's around,
  a nudge, a question, an answer — each appended in the order it happened and
  never rewritten, so a conversation and the trail don't disturb each other. A
  fix means *arriving somewhere*: one location watch drives the map (which
  follows you) and the feed, and a fix is appended when you've covered ~400m at
  walking pace or stopped after travelling — driving past somewhere at 60mph
  isn't being there. Standing still adds nothing. The map is behind a button. The agent has the same snapshot (`around_me`).
  Composes existing feeds only — no new keys.

- **Nudges you outdoors** (`/api/push/*`, `internal/server/nudge.go`,
  `internal/push`). The only part of Malten that reaches out rather than waiting
  to be opened: an hourly loop over opted-in devices that **usually decides to
  say nothing**. The agent is asked whether there's a specific, time-bound reason
  to interrupt — somewhere they've never stood, the last of the light, a train
  back — and returns nothing if not. At most one a day, never outside 08:00–20:00
  local. Web Push is hand-rolled (VAPID/RFC 8292 + RFC 8291 aes128gcm), so no SDK;
  the payload is encrypted for the subscriber's browser and the push service
  relays ciphertext it can't read. Needs `ANTHROPIC_API_KEY` plus a VAPID key
  (`VAPID_PRIVATE_KEY`/`vapid_key`, generated on first run if absent).

`/api/gridref` (WGS84 lat/lng → National Grid reference, plus the 1 km square and
its eight neighbours) is still a pure helper. There are **two** opt-in exceptions
to statelessness, and no more: the out-of-coverage **waitlist**
(`/api/interest`) and the **nudge subscriber list** (`push.json`) — a
notification can't be composed by a browser that isn't running, so that list
holds the push subscription, a coarse location (the centre of a 1 km grid
square), the squares explored and the last few nudges sent. It's opt-in,
disclosed in the UI, and unsubscribing deletes the record.

## Repository map

```
cmd/malten          server binary (embeds the UI + Leaflet)
internal/osgrid     WGS84 <-> OS National Grid reference (Helmert + Transverse Mercator, both directions), tested
internal/llm        dependency-free Anthropic Messages API client (HTTP+SSE, bounded tool loop), tested
internal/nrail      National Rail: embedded station dataset (ODbL) + Darwin OpenLDBWS SOAP client, tested
internal/bods       Bus Open Data Service SIRI-VM client (live bus positions in a bbox), tested
internal/push       Web Push, hand-rolled: VAPID (RFC 8292) + payload encryption (RFC 8291 aes128gcm), tested
internal/server     stateless HTTP handlers + embedded web UI (gridref, tfl + rail + bus proxies, ask agent, nudge loop, waitlist)
internal/server/web UI: page-map.html, app.js (client store), vendored leaflet.js/.css, PWA assets
```

No external Go dependencies: `go.mod` has no `require` block. Leaflet is vendored
as a static asset (`web/leaflet.js`, `web/leaflet.css`); the LLM and Darwin
(SOAP) clients are hand-rolled over `net/http` for the same reason (don't pull in
an SDK). The rail station coordinates are a vendored, embedded CSV
(`internal/nrail/stations.csv`, ODbL — attribute it).

## Build, test, run

```bash
go build ./...      # build everything
go test ./...       # unit tests (osgrid grid-reference math)
go run ./cmd/malten # start the server on :8080
```

The map needs a free **OS Data Hub** API key (osdatahub.os.uk). Either the
operator sets it once as `OS_API_KEY` (the server proxies tiles; visitors need
nothing), or each visitor enters their own in the UI. TfL live transport and
nearby-station lookup work with no key.
Optional server-side keys enable more: `ANTHROPIC_API_KEY` (and optionally
`MALTEN_MODEL`, default `claude-opus-4-8`) turns on **Ask Malten**;
`NRE_LDBWS_TOKEN` turns on **live rail departures**; `BODS_API_KEY` turns on
**nationwide live buses**. The UI hides each feature when its key is absent (via
`/api/health`). `go test ./...` must stay
green — the osgrid tests validate the projection against OS's own worked example
(Caister) to ~0.2 m, and `internal/llm` tests the tool loop against a stub SSE
server (no network, no key).

## Conventions & invariants (do not break these)

- **The server stores nothing about users.** Finds, the OS key and the timeline
  (place fixes and the whole conversation) live client-side and never touch the
  server; the live-data proxy and the agent hold no user content, and Ask
  Malten's questions/history/locations are used per-turn and forgotten. The
  browser is the only memory the conversation has — that's why history and the
  trail travel with each request. Keep the stored timeline **structural** (typed
  events, not markup): it's read back to feed the agent, not just to redraw.
  `web/app.js` documents the schema and versions it (`v`); bump the version when
  the shape changes rather than migrating in place. The only disk write is the opt-in waitlist. Don't add
  server-side persistence of user content — that's the privacy model.
- **The per-user OS key stays the user's.** When entered in the browser it's
  sent straight to the OS APIs — never route that per-user key through, log, or
  store it server-side. The one exception is an operator-supplied shared
  `OS_API_KEY`: that is a server-side secret (like the others) used only by the
  tile proxy, and it too never reaches the browser.
- **The Anthropic key stays the server's.** `ANTHROPIC_API_KEY` (for Ask Malten)
  is the one server-side secret; it never reaches the browser. Keep the LLM
  client dependency-free (`net/http` + SSE) — don't add the SDK.
- **Rail and bus keys stay the server's.** `NRE_LDBWS_TOKEN` (Darwin OpenLDBWS)
  and `BODS_API_KEY` (Bus Open Data Service) are server-side secrets like the
  Anthropic key; never send them to the browser.
- **Attribute OS data.** OS licensing requires visible attribution ("Contains OS
  data © Crown copyright and database rights <year>"); keep it on the map. The
  vendored rail station dataset is ODbL — keep its attribution too.
- **The reward is the place, never a score.** "New ground" (the 1 km grid
  squares you've stood in, `Malten.getSquares`) is a record, not a game: no
  points, badges, streaks or leaderboards, and the prompts say so explicitly.
  Extrinsic rewards are exactly what made check-in apps hollow. Keep it a fact
  about where someone has been.
- **Great Britain only.** The OS National Grid and OS Maps API cover England,
  Scotland and Wales. `osgrid.FromWGS84` returns ok=false outside GB; handle it.
- **Don't hard-code the deepest zoom.** How far the OS Maps API zooms depends on
  the plan behind the key: the OpenData plan answers 403 on the detailed levels,
  Premium serves them. The UI learns the limit from repeated tile errors, sets
  Leaflet's `maxNativeZoom` and scales up, so an over-zoomed map goes soft rather
  than blank (and the server logs the 403 once, since nothing else tells the
  operator). Keep that adaptive — a constant would break one plan or the other.
- **Single binary, no external runtime deps.** Keep the UI and Leaflet embedded
  (`//go:embed`); no cgo, no CDN. Vendor new assets, don't link them remotely.
- **Grid math is tested, both ways.** `internal/osgrid` converts WGS84↔BNG;
  `FromWGS84` (forward) and `ToWGS84`/`enToAiry` (inverse) are both validated
  against OS's Caister worked example, plus round-trips across GB and the
  region-letter tests. Keep them passing. The inverse is what lets OS-sourced
  BNG coordinates (OS Names) land on the WGS84 map.

## Known limitations / future work

- Raster basemap only (OS Maps API). Vector tiles, 3D building extrusion (OS
  building heights), and terrain (OS Terrain) are the obvious next steps.
- Finds are anchored to a grid reference + lat/lng, not yet to a persistent OS
  feature id (TOID/UPRN) — that needs the OS Features/NGD API and a keyed tier.
- Client-held data is per-device (localStorage); no cross-device sync.
- License is AGPL-3.0.
```
