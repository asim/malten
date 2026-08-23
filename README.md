# Malten

**A spatial companion for exploring Great Britain — a live map you move through,
built on Ordnance Survey.**

Malten is a single, self-contained Go binary that serves a map-first web app (and
installable PWA). You drop **finds** as you explore, see **live transport** moving
around you, search anywhere in Britain, point your camera to see what's nearby,
and get quiet, concrete nudges from an **ambient agent** — "here's something worth
walking to right now."

The map substrate is the **Ordnance Survey** — the OS Maps API for tiles, the OS
National Grid for addressing, OS Names for search. The moat is the experience and
the spatial memory, not the map data.

---

## What it does

- **A timeline front door.** The app opens on a live feed — date, place, weather,
  and what's around you (nearest station and next trains, buses moving, nearby
  cafés/parks, a nudge from the agent). It reads forwards: oldest at the top, now
  at the bottom, growing into a trail as you move. It survives a reload, and the
  map is a button away.
- **Live OS map.** Ordnance Survey tiles, centred on you. If the operator sets a
  shared OS key the server proxies tiles so visitors need no key of their own.
- **Finds.** Tap the map (or a place) to drop a note anchored to its OS National
  Grid reference. Finds live in your browser — the server never sees them.
- **Nationwide live transport.**
  - **Rail** — live departure boards for ~2,600 National Rail stations (Darwin).
  - **Buses** — live vehicle positions moving nearby (Bus Open Data Service).
  - **London** — stop-level bus/tram/tube arrivals (Transport for London).
- **Place search & reverse geocoding** across Great Britain (OS Names gazetteer).
- **"Around you".** On open, a live snapshot of your surroundings — nearest place,
  nearest station and its next trains, buses moving nearby.
- **A spatial agent ("Ask Malten").** No chat window. It works in the background
  and offers one concrete thing to go and do, with a loop — *something else* or
  *I did it → next* — and a composer at the foot of the timeline lets you ask
  about where you are ("when's the next train to Leeds?", "is the Lion Gate Café
  open?"). The answer streams in as an entry in the feed, using live transport,
  places (including OSM opening hours) and weather as tools. It's a conversation
  with a memory of your day: follow-ups carry the thread, and the trail travels
  with the question, so it knows where you've been, what you passed and what it
  already suggested. All still there after a reload.
- **Look around (camera).** Point your phone; POIs, stations and your finds are
  tagged in view by real compass bearing. Tap one to lock a navigation beacon
  with a live-updating arrow and distance.
- **New ground.** The National Grid divides Britain into 1 km squares; the ones
  you've stood in are the map of where you've actually been. Walk into one for
  the first time and the feed says so. No points, no streaks, nobody to compete
  with — and the agent knows which squares near you you've never set foot in, so
  it can send you somewhere genuinely new.
- **Nudges (opt-in).** An hourly loop that usually says nothing. It notifies only
  when there's a specific, time-bound reason — somewhere new a short walk away,
  the last of the light, a train that gets you back — at most once a day, never
  at night.
- **Out of coverage?** Visitors outside Britain can **request their city**, so
  demand tells us where to expand.

## How it works

- **One Go binary.** The UI, a vendored copy of Leaflet, and the rail station
  dataset are all embedded (`//go:embed`). No CDN, no cgo.
- **No external Go dependencies.** `go.mod` has no `require` block. The Anthropic
  client, the National Rail (SOAP) client, and everything else are hand-rolled
  over `net/http`.
- **Stateless and anonymous.** The server stores nothing about users. Your finds,
  your timeline (including the conversation) and, in per-user mode, your OS key
  live in the browser — which is why each question carries its own history and
  trail. The timeline is stored as a versioned log of typed events, so it can be
  replayed as context rather than merely redrawn. Live-data endpoints
  proxy public feeds and hold no user content. There are two opt-in exceptions:
  the out-of-coverage waitlist, and — if you switch nudges on — a subscriber
  record holding your push endpoint, a coarse location (a 1 km grid square) and
  the squares you've explored, so a notification can be composed while the app is
  closed. Turning nudges off deletes it.
- **Great Britain only** (for now) — the OS National Grid and OS Maps API cover
  England, Scotland and Wales.

## Quick start

```bash
go build ./...        # build everything
go test ./...         # unit tests (grid math, transport clients, agent loop, …)
go run ./cmd/malten   # start on :8080
```

Open <http://localhost:8080>. With no keys configured you'll be asked for a free
**OS Data Hub** key (from [osdatahub.os.uk](https://osdatahub.os.uk)) to load the
map; everything else lights up as you add the keys below.

## Configuration

Every server-side secret is read the same way: an **environment variable**, or
failing that a **file of the same name next to the binary** (how the deploy
provisions them from CI secrets). Each feature switches on only when its key is
present; the UI hides features whose key is absent (via `/api/health`).

| Variable | File | Enables | Notes |
|---|---|---|---|
| `OS_API_KEY` | `os_api_key` | Server-side tile proxy **and** place search | Optional. Without it, each visitor enters their own OS key in the UI (client-side only). How deep the map zooms depends on the plan of the key's OS Data Hub project — the OpenData plan stops short of the detailed levels and answers 403 there (the server logs this once); the app discovers the limit and scales the deepest real level up rather than showing blank tiles. |
| `ANTHROPIC_API_KEY` | `anthropic_key` | The ambient agent | Optionally set `MALTEN_MODEL` (default `claude-opus-4-8`). |
| `NRE_LDBWS_TOKEN` | `nre_ldbws_token` | Live rail departures | Free "Live Departure Board (LDBWS) - Public" consumer key from the [Rail Data Marketplace](https://raildata.org.uk). |
| `BODS_API_KEY` | `bods_api_key` | Nationwide live buses | Free key from the [Bus Open Data Service](https://www.bus-data.dft.gov.uk). |
| `TFL_APP_KEY` | — | Higher TfL rate limit | Optional; TfL works keyless. |
| `MALTEN_ADMIN_TOKEN` | `admin_token` | Viewing the waitlist over HTTP | Without it the waitlist is captured but not viewable via the API. |
| `MALTEN_OVERPASS_URL` | — | Override the Overpass instance for POIs | Default `https://overpass-api.de/api/interpreter`. |
| `MALTEN_DATA` | — | Waitlist file path | Default `interest.jsonl`. |
| `VAPID_PRIVATE_KEY` | `vapid_key` | Opt-in push nudges (with `ANTHROPIC_API_KEY`) | Generated and saved on first run if absent. Optionally set `MALTEN_PUSH_SUBJECT` (contact URL sent to push services) and `MALTEN_PUSH_DATA` (subscriber file, default `push.json`). |
| `MALTEN_ADDR` | — | Listen address | Default `:8080`. |

The **OS key stays the user's** in per-user mode — entered in the browser and sent
straight to the OS, never through the server. A shared `OS_API_KEY` is the one
exception: a server-side secret used only by the tile proxy, never sent to the
browser. The Anthropic, rail and bus keys are always server-side and never reach
the browser.

### The waitlist

Out-of-coverage "request your city" submissions are appended to `MALTEN_DATA`
(default `interest.jsonl`). View them with the admin token:

```
GET /api/interest?token=<MALTEN_ADMIN_TOKEN>
```

## HTTP endpoints

Pure helpers and public-data proxies (none store user content):

| Endpoint | Purpose |
|---|---|
| `/api/gridref` | WGS84 lat/lng → OS National Grid reference |
| `/api/tiles/{z}/{x}/{y}.png` | OS Maps tile proxy (when `OS_API_KEY` is set) |
| `/api/search`, `/api/nearest` | Place search & reverse geocoding (OS Names) |
| `/api/stations`, `/api/departures` | Nearest rail stations; live departure boards |
| `/api/stops`, `/api/arrivals` | London stops; live arrivals (TfL) |
| `/api/buses` | Live bus positions in a bounding box (BODS) |
| `/api/poi` | Nearby named POIs (OpenStreetMap Overpass, cached by grid cell) |
| `/api/around` | One live "around me" snapshot across every feed |
| `/api/suggest` | The ambient agent's single nudge (SSE-free; one call) |
| `/api/ask` | The fuller bounded tool-use agent loop (SSE) |
| `/api/push/subscribe`, `/api/push/unsubscribe` | Opt in/out of nudges (unsubscribing deletes the record) |
| `/api/interest` | Out-of-coverage waitlist (POST to add; GET with token to read) |
| `/api/health` | Status + which features are enabled |

## Repository map

```
cmd/malten          server binary (embeds the UI + Leaflet)
internal/osgrid     WGS84 <-> OS National Grid reference (Helmert + Transverse Mercator), tested
internal/llm        dependency-free Anthropic Messages API client (HTTP + SSE, bounded tool loop), tested
internal/nrail      National Rail: embedded station dataset (ODbL) + Darwin OpenLDBWS SOAP client, tested
internal/bods       Bus Open Data Service SIRI-VM client (live bus positions in a bbox), tested
internal/push       Web Push: VAPID (RFC 8292) + payload encryption (RFC 8291), dependency-free, tested
internal/server     stateless HTTP handlers + embedded web UI
internal/server/web UI: page templates, app.js (client store), vendored Leaflet, PWA assets
```

## Attribution

- **Ordnance Survey** — "Contains OS data © Crown copyright and database rights."
  Kept visible on the map, as OS licensing requires.
- **National Rail** station coordinates — a vendored dataset under the **Open
  Database License (ODbL)**.
- **OpenStreetMap** — POIs via the Overpass API, © OpenStreetMap contributors
  (ODbL).

## License

[AGPL-3.0](LICENSE).
