# Malten

**Malten is a mental model for the world.**

Not a map of it. A model *for* it — because from where you're standing, the world
is the few hundred metres you're in, right now. We all occupy different points on
the map.

Malten opens on a **timeline**: where you are, what the weather's doing, what's
around you, and one concrete thing worth walking to. As you move it grows into a
trail of the day. The map is a button away, and deliberately not the front door —
the point isn't to look at a map, it's to end up knowing the place.

So the thing being built isn't the app's model of the world — it's **yours**.
Every feature is judged on one test: does it build your model, or replace it?
Turn-by-turn directions replace it, which is why there are none. Naming a place
and a direction builds it. The measure of success is that Malten leaves you
knowing where you are when your phone is dead.

It's a single self-contained Go binary serving a web app (and installable PWA),
with **no external Go dependencies** and **no database**. Your trail, your finds
and your photos live in your browser. The server holds none of it.

The substrate is the **Ordnance Survey** — the OS Maps API for tiles, the OS
National Grid for addressing, OS Names for search.

---

## What it does

### The timeline

An append-only log of what actually happened: arriving somewhere, what was
around, a nudge, a question, an answer, a photo of a find. Oldest at the top, now
at the bottom next to the composer. Nothing already on screen is ever rewritten,
so a conversation and the trail don't disturb each other, and a reload picks the
day back up where it left off.

A fix means **arriving somewhere** — you covered ~400m on foot, or stopped after
travelling. Driving past somewhere at 60mph isn't being there, and standing still
adds nothing.

### The agent

No chat window. It works two ways:

- **Ambiently** — one concrete thing to go and do, with a loop: *something else*,
  or *I did it → next*.
- **On demand** — a composer at the foot of the timeline. *"When's the next train
  to Leeds?"* *"Is the Lion Gate Café open?"* The answer streams into the feed
  using live transport, OSM opening hours and the weather as tools.

It's a conversation with a memory of your day: follow-ups carry the thread, and
the trail travels with each question — where you've been, what you passed, what
it already suggested — because the server remembers nothing between turns.

### Notes

One rule in the composer: what you type is a **note**, unless it ends in a
question mark — then it's a question. Both land in the same feed.

A note is kept with where and when you thought it. That's the whole point:
place is the strongest index a person has, and it's the one thing every notes
app throws away. Walk back within a couple of hundred metres and the note is
waiting for you — *"you noted this here"*. Notes near you also travel with a
question, so the agent can use what you told yourself, and then forgets them.

### New ground

The National Grid divides Britain into 1 km squares. The ones you've been in are
the map of where you've actually been; enter a new one and the feed says so. **No
points, no badges, no streaks, nobody to compete with** — extrinsic rewards are
what made check-in apps hollow. The pull is that the agent knows which squares
near you you've never been in, so it can send you somewhere genuinely new.

### Going out with children

- **A hunt.** Five things to find where you're actually standing — a red postbox,
  something older than Grandma, three different leaves, a bird making a noise.
  Written for the age you give it once, and built from the real places nearby: it
  may name those and nothing else, because a child hunting for a statue that
  isn't there ends the walk badly. Tick them off as you find them.
- **Photograph what you found.** The camera button in the composer saves a photo
  against the grid reference you were standing on, with a note if you want one.
  It's downscaled and stored in your browser's own IndexedDB — nothing is
  uploaded, and there's nowhere on the server it could go.

### Everything else

- **Live OS map** centred on you, following as you move.
- **Nationwide live transport** — rail departure boards for ~2,600 stations
  (Darwin), live bus positions (Bus Open Data Service), and stop-level bus, tram
  and tube arrivals in London (TfL).
- **Place search & reverse geocoding** across Great Britain (OS Names).
- **Look around (camera)** — point your phone and nearby places, stations and
  your finds are tagged in view by real compass bearing; tap one for a navigation
  beacon with a live-updating arrow and distance.
- **Nudges (opt-in)** — an hourly loop that usually says nothing, and notifies
  only when there's a specific, time-bound reason: somewhere new a short walk
  away, the last of the light, a train that gets you back. At most one a day,
  never at night.
- **Out of coverage?** Visitors outside Britain can **request their city**.

## How it works

- **One Go binary.** The UI, a vendored copy of Leaflet, and the rail station
  dataset are all embedded (`//go:embed`). No CDN, no cgo, no build step.
- **No external Go dependencies.** `go.mod` has no `require` block. The Anthropic
  client (HTTP + SSE), the National Rail SOAP client, the SIRI-VM bus feed and
  Web Push (VAPID + payload encryption) are all hand-rolled over `net/http`.
- **Stateless and anonymous.** The server stores nothing about users. Your finds,
  photos, explored squares and whole timeline — including the conversation — live
  in your browser. The timeline is a versioned log of *typed events*, not markup,
  so it can be replayed to the agent as context rather than merely redrawn.
- **Two opt-in exceptions, and no more.** The out-of-coverage waitlist, and — if
  you switch nudges on — a subscriber record holding your push endpoint, a coarse
  location (a 1 km grid square, not a fix) and which squares you've explored, so a
  notification can be composed while the app is closed. Turning nudges off deletes
  it.
- **Great Britain only** (for now) — the OS National Grid and OS Maps API cover
  England, Scotland and Wales.

## Quick start

```bash
go build ./...        # build everything
go test ./...         # grid math, transport clients, agent loop, web push, …
go run ./cmd/malten   # start on :8080
```

Open <http://localhost:8080>. The timeline works immediately. Opening the map
asks for a free **OS Data Hub** key (from [osdatahub.os.uk](https://osdatahub.os.uk))
unless the operator has configured one; everything else lights up as you add the
keys below.

## Configuration

Every server-side secret is read the same way: an **environment variable**, or
failing that a **file of the same name next to the binary** (how the deploy
provisions them from CI secrets). Each feature switches on only when its key is
present, and the UI hides features whose key is absent (via `/api/health`).

| Variable | File | Enables | Notes |
|---|---|---|---|
| `OS_API_KEY` | `os_api_key` | Server-side tile proxy **and** place search | Optional — without it each visitor enters their own OS key, which stays in their browser. How deep the map zooms depends on the plan behind the key: the OpenData plan answers 403 on the detailed levels (logged once), and the app discovers the limit and scales the deepest real level up rather than showing blank tiles. |
| `ANTHROPIC_API_KEY` | `anthropic_key` | The agent: nudges, questions, hunts | Optionally set `MALTEN_MODEL` (default `claude-opus-4-8`). |
| `NRE_LDBWS_TOKEN` | `nre_ldbws_token` | Live rail departures | Free "Live Departure Board (LDBWS) - Public" consumer key from the [Rail Data Marketplace](https://raildata.org.uk). Note this is *not* the Kafka/streaming credential. |
| `BODS_API_KEY` | `bods_api_key` | Nationwide live buses | Free key from the [Bus Open Data Service](https://www.bus-data.dft.gov.uk). |
| `VAPID_PRIVATE_KEY` | `vapid_key` | Opt-in push nudges (with `ANTHROPIC_API_KEY`) | Generated and saved on first run if absent. Also `MALTEN_PUSH_SUBJECT` (contact URL sent to push services) and `MALTEN_PUSH_DATA` (subscriber file, default `push.json`). |
| `TFL_APP_KEY` | — | Higher TfL rate limit | Optional; TfL works keyless. |
| `MALTEN_ADMIN_TOKEN` | `admin_token` | Reading the waitlist over HTTP | Without it the waitlist is captured but not readable via the API. |
| `MALTEN_OVERPASS_URL` | — | Override the Overpass instance | Default `https://overpass-api.de/api/interpreter`. |
| `MALTEN_DATA` | — | Waitlist file path | Default `interest.jsonl`. |
| `MALTEN_ADDR` | — | Listen address | Default `:8080`. |

**Where the keys live.** In per-user mode the OS key is entered in the browser
and sent straight to the OS — never through the server. A shared `OS_API_KEY` is
the one exception: a server-side secret used only by the tile proxy, never sent
to the browser. The Anthropic, rail, bus and VAPID keys are always server-side.

### The waitlist

Out-of-coverage "request your city" submissions are appended to `MALTEN_DATA`
(default `interest.jsonl`). Read them with the admin token:

```
GET /api/interest?token=<MALTEN_ADMIN_TOKEN>
```

## HTTP endpoints

Pure helpers and public-data proxies — none of them store user content:

| Endpoint | Purpose |
|---|---|
| `/api/gridref` | WGS84 lat/lng → OS National Grid reference, its 1 km square and the eight around it |
| `/api/tiles/{z}/{x}/{y}.png` | OS Maps tile proxy (when `OS_API_KEY` is set) |
| `/api/search`, `/api/nearest` | Place search & reverse geocoding (OS Names) |
| `/api/stations`, `/api/departures` | Nearest rail stations; live departure boards |
| `/api/stops`, `/api/arrivals` | London stops; live arrivals (TfL) |
| `/api/buses` | Live bus positions in a bounding box (BODS) |
| `/api/poi` | Nearby named places with opening hours (OSM Overpass, cached by grid cell) |
| `/api/around` | One live "around me" snapshot across every feed |
| `/api/suggest` | The agent's single nudge |
| `/api/hunt` | Five things for a child to find nearby, from real places |
| `/api/ask` | The bounded tool-use agent loop (SSE) |
| `/api/push/subscribe`, `/api/push/unsubscribe` | Opt in/out of nudges (unsubscribing deletes the record) |
| `/api/interest` | Out-of-coverage waitlist (POST to add; GET with token to read) |
| `/api/health` | Status, which features are enabled, and the VAPID public key |

## Repository map

```
cmd/malten          server binary (embeds the UI + Leaflet)
internal/osgrid     WGS84 <-> OS National Grid, both directions, plus 1 km squares — tested
internal/llm        Anthropic Messages API client: HTTP + SSE, bounded tool loop — tested
internal/nrail      National Rail: embedded station dataset (ODbL) + Darwin OpenLDBWS SOAP — tested
internal/bods       Bus Open Data Service SIRI-VM client (vehicle positions in a bbox) — tested
internal/push       Web Push: VAPID (RFC 8292) + payload encryption (RFC 8291 aes128gcm) — tested
internal/server     stateless HTTP handlers, the agent surfaces, the nudge loop
internal/server/web the UI: page template, client store, vendored Leaflet, PWA assets
```

The grid maths is validated against OS's own worked example (Caister) to ~0.2 m
in both directions, and the Web Push tests decrypt a message exactly as a browser
would. `go test ./...` needs no network and no keys.

## Attribution

- **Ordnance Survey** — "Contains OS data © Crown copyright and database rights."
  Kept visible on the map, as OS licensing requires.
- **National Rail** station coordinates — a vendored dataset under the **Open
  Database Licence (ODbL)**.
- **OpenStreetMap** — places via the Overpass API, © OpenStreetMap contributors
  (ODbL).
- **Open-Meteo** — weather, free and keyless.

## Licence

[AGPL-3.0](LICENSE).
