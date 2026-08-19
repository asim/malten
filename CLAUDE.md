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
finds and your **OS Data Hub API key** live in the browser (`web/app.js`,
localStorage). Map tiles are fetched by the client *directly* from the OS APIs
with your key — the server never sees it.

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
- **Runs "Ask Malten", a spatial agent** (`/api/ask`, SSE). A small bounded
  tool-use loop over Anthropic's Messages API, with the live-data calls above (plus
  `/api/gridref`) exposed as tools. It's enabled only when `ANTHROPIC_API_KEY` is
  set on the server; the question and location are used for the turn and forgotten.

`/api/gridref` (WGS84 lat/lng → National Grid reference) is still a pure helper.
The one opt-in exception to statelessness is the out-of-coverage **waitlist**
(`/api/interest`, a local JSONL file); nothing else touches disk.

## Repository map

```
cmd/malten          server binary (embeds the UI + Leaflet)
internal/osgrid     WGS84 -> OS National Grid reference (Helmert + Transverse Mercator), tested
internal/llm        dependency-free Anthropic Messages API client (HTTP+SSE, bounded tool loop), tested
internal/nrail      National Rail: embedded station dataset (ODbL) + Darwin OpenLDBWS SOAP client, tested
internal/server     stateless HTTP handlers + embedded web UI (gridref, tfl + rail proxies, ask agent, waitlist)
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

The map needs a free **OS Data Hub** API key (osdatahub.os.uk), entered in the
UI — nothing else. TfL live transport and nearby-station lookup work with no key.
Optional server-side keys enable more: `ANTHROPIC_API_KEY` (and optionally
`MALTEN_MODEL`, default `claude-opus-4-8`) turns on **Ask Malten**;
`NRE_LDBWS_TOKEN` turns on **live rail departures**. The UI hides each feature
when its key is absent (via `/api/health`). `go test ./...` must stay
green — the osgrid tests validate the projection against OS's own worked example
(Caister) to ~0.2 m, and `internal/llm` tests the tool loop against a stub SSE
server (no network, no key).

## Conventions & invariants (do not break these)

- **The server stores nothing about users.** Finds and the OS key live client-
  side and never touch the server; the live-data proxy and the agent hold no
  user content, and Ask Malten's questions/locations are used per-turn and
  forgotten. The only disk write is the opt-in waitlist. Don't add server-side
  persistence of user content — that's the privacy model.
- **The OS key stays the user's.** It's entered in the browser and sent straight
  to the OS APIs. Never route it through, log, or store it server-side.
- **The Anthropic key stays the server's.** `ANTHROPIC_API_KEY` (for Ask Malten)
  is the one server-side secret; it never reaches the browser. Keep the LLM
  client dependency-free (`net/http` + SSE) — don't add the SDK.
- **The rail token stays the server's.** `NRE_LDBWS_TOKEN` (Darwin OpenLDBWS)
  is a server-side secret like the Anthropic key; never send it to the browser.
- **Attribute OS data.** OS licensing requires visible attribution ("Contains OS
  data © Crown copyright and database rights <year>"); keep it on the map. The
  vendored rail station dataset is ODbL — keep its attribution too.
- **Great Britain only.** The OS National Grid and OS Maps API cover England,
  Scotland and Wales. `osgrid.FromWGS84` returns ok=false outside GB; handle it.
- **Single binary, no external runtime deps.** Keep the UI and Leaflet embedded
  (`//go:embed`); no cgo, no CDN. Vendor new assets, don't link them remotely.
- **Grid math is tested.** Any change to `internal/osgrid` must keep the Caister
  worked-example test (and the region-letter tests) passing.

## Known limitations / future work

- Raster basemap only (OS Maps API). Vector tiles, 3D building extrusion (OS
  building heights), and terrain (OS Terrain) are the obvious next steps.
- Finds are anchored to a grid reference + lat/lng, not yet to a persistent OS
  feature id (TOID/UPRN) — that needs the OS Features/NGD API and a keyed tier.
- Client-held data is per-device (localStorage); no cross-device sync.
- License is AGPL-3.0.
```
