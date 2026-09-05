# Malten

**Malten helps you build a mental model of your world.**

We experience life in fragments: a thought while walking, something noticed in a
place, an idea connected to an earlier conversation, or a pattern that only
becomes visible over time. Most software stores those fragments in separate
files, feeds and apps, stripped of the context in which they occurred.

Malten captures them as part of a living spatial network. Place, time, movement
and relationships provide structure. AI works quietly in the background to
connect and resurface what matters, but it is not the subject of the experience.
The aim is not to talk to a machine or recreate the world on a screen. It is to
help a person pay attention, think, remember and understand.

The name is an anagram of **mental**.

## The idea

Malten began in 2013 as anonymous, ephemeral message streams. A hashtag
represented an independent stream of thought, and discovery happened by linking
one stream to another. Messages could contain text, embedded video and slash
commands.

It was not intended to become another public social network. The underlying idea
was that fragments of thought could form a different kind of network: streams of
consciousness connected by context rather than gathered into profiles and
feeds.

The available primitives have changed. A phone can now contribute place, time,
movement, direction and other ambient signals. Models can find relationships
across fragments without requiring a person to organise everything manually.
What was once a network of linked message streams can become a private,
contextual model of the world as one person experiences it.

That does not mean a three-dimensional simulation of reality. Malten's model may
be understood as points and relationships:

- thoughts and observations
- places and moments
- movement and encounters
- people, projects and objects
- geographic, temporal, semantic and personal connections

A map is one possible projection of that network, but it is not the network
itself. The right view might instead be a trail, a nearby memory, a sequence, a
cluster or a connection that has just become relevant. Often the best interface
is no view at all.

## How Malten should feel

The person remains in the foreground. The software does not ask them to defer to
an agent, maintain a conversation with it, or spend more time looking at a
screen.

The basic loop is:

1. **Capture** — record a thought or observation with almost no friction.
2. **Contextualise** — quietly attach where, when and under what circumstances it
   occurred.
3. **Connect** — recognise relationships with other fragments.
4. **Resurface** — bring something back when its context becomes meaningful.
5. **Understand** — help the person form a clearer model of an idea, place,
   project, pattern or part of their life.

AI is infrastructure for indexing, association, compression and relevance. It
may assist directly when asked, but conversation is an escape hatch rather than
the product's centre.

This leads to a few principles:

- Human attention is the scarce resource Malten should protect.
- Capture should be easier than conversation.
- Context should be gathered where possible, not repeatedly entered.
- Connections matter more than feeds.
- Place is a powerful dimension, but not the product itself.
- The model belongs to the person and should deepen their agency.
- Malten should encourage attention to the world, not attention to Malten.
- Social or shared experiences may exist, but Malten is not public social media.

## The current experiment

The current application is a location-first exploration of this larger idea. It
uses walking, notes, movement and nearby places to test how software can help
someone become more aware of where they are and connect thoughts to the places
in which they occurred.

It opens on a **timeline** rather than a map. As you move, it becomes a trail of
the day: where you were, what was around you, what you noticed and what you
thought. The map is available as one view, deliberately not the front door.

This prototype is currently limited to Great Britain because it uses the
Ordnance Survey National Grid and OS Maps. That is an implementation constraint,
not the boundary of Malten.

### Timeline

The timeline is an append-only log of what happened: arriving somewhere, a note,
a question, an answer, a photo or a find. Oldest is at the top and now is at the
bottom beside the composer. Nothing already on screen is rewritten, and a reload
picks the day back up where it left off.

A location fix becomes an arrival after meaningful movement or a stop following
travel. Merely driving through somewhere does not mean you experienced it.

### Notes in context

What you type is treated as a note unless it ends in a question mark. Both notes
and questions land in the same timeline.

A note retains where and when it was made. Return near that location and Malten
can resurface it. Nearby notes can also become context for a question. Place is
one of the strongest human indexes for memory, and this prototype explores what
happens when a notes system stops discarding it.

### Ambient assistance

There is no separate chat window. Assistance works in two ways:

- **Ambiently** — Malten can occasionally surface something relevant to the
  present context.
- **On demand** — a question in the composer can use nearby places, live
  transport and weather as tools, with the response appearing in the timeline.

The long-term direction is not a more prominent agent. It is better judgement
about when to remain silent, when to connect two fragments and when to return
something useful to the person's attention.

### Spatial views and exploration

The current prototype includes several experiments built on the same underlying
context:

- **New ground** records which 1 km National Grid squares you have experienced.
  There are no points, badges, streaks or competition.
- **Look around** uses the camera and compass to place nearby locations and finds
  in their real direction.
- **Live map** provides a conventional geographic view when it is useful.
- **Going out with children** creates a local hunt from things that are actually
  nearby and lets a family photograph what they find.
- **Live transport** provides rail, bus, tram and tube context without turning
  Malten into a turn-by-turn navigation product.
- **Nudges** are opt-in and deliberately scarce: an hourly check that usually
  says nothing, with at most one notification a day and none at night.

These are product experiments, not a fixed feature checklist. Each should be
judged by the same question: does it help someone build their own model, or does
it replace that model?

## Privacy and architecture

Malten is a single self-contained Go binary serving an installable web app. It
has no external Go dependencies and no database.

The server is stateless and anonymous. The timeline, notes, finds, explored
squares and photos live in the browser. Photos are downscaled into IndexedDB and
are not uploaded. The timeline is a versioned log of typed events rather than
stored markup, allowing it to be replayed as context as well as redrawn.

There are two opt-in server-side exceptions:

- The out-of-coverage waitlist.
- Push subscribers, when nudges are enabled. A subscriber record contains the
  push endpoint, a coarse 1 km grid square and explored squares. Turning nudges
  off deletes it.

The UI and a vendored copy of Leaflet are embedded with `//go:embed`. The
Anthropic client, National Rail SOAP client, SIRI-VM bus feed and Web Push
implementation are built directly over the Go standard library.

## Quick start

```bash
go build ./...
go test ./...
go run ./cmd/malten
```

Open <http://localhost:8080>. The timeline works immediately. Opening the map
asks for a free **OS Data Hub** key from
[osdatahub.os.uk](https://osdatahub.os.uk) unless the operator has configured
one. Other features become available as their keys are added.

## Configuration

Server-side secrets are read from an environment variable or, failing that, a
file of the same name beside the binary. Features switch on only when their
configuration is present, and `/api/health` tells the UI what is available.

| Variable | File | Enables | Notes |
|---|---|---|---|
| `OS_API_KEY` | `os_api_key` | Tile proxy and place search | Optional. Without it each visitor can enter an OS key which remains in their browser. |
| `ANTHROPIC_API_KEY` | `anthropic_key` | Suggestions, questions and hunts | `MALTEN_MODEL` optionally selects the model. |
| `NRE_LDBWS_TOKEN` | `nre_ldbws_token` | Live rail departures | Uses the National Rail LDBWS public consumer key. |
| `BODS_API_KEY` | `bods_api_key` | Nationwide live buses | Free key from the Bus Open Data Service. |
| `VAPID_PRIVATE_KEY` | `vapid_key` | Opt-in push nudges | Generated and saved on first run if absent. |
| `TFL_APP_KEY` | — | Higher TfL rate limit | Optional; TfL works keyless. |
| `MALTEN_ADMIN_TOKEN` | `admin_token` | Reading the waitlist over HTTP | Without it submissions cannot be read through the API. |
| `MALTEN_OVERPASS_URL` | — | Overpass instance override | Defaults to `https://overpass-api.de/api/interpreter`. |
| `MALTEN_DATA` | — | Waitlist file path | Defaults to `interest.jsonl`. |
| `MALTEN_ADDR` | — | Listen address | Defaults to `:8080`. |

In per-user mode the OS key is sent directly from the browser to OS and never
passes through Malten's server. A shared `OS_API_KEY` is used only by the
server-side tile proxy. Anthropic, rail, bus and VAPID credentials remain
server-side.

### Waitlist

Out-of-coverage requests are appended to `MALTEN_DATA`. Read them with the
admin token:

```text
GET /api/interest?token=<MALTEN_ADMIN_TOKEN>
```

## HTTP endpoints

The HTTP API consists mainly of pure helpers and public-data proxies. It does not
store user content.

| Endpoint | Purpose |
|---|---|
| `/api/gridref` | Convert WGS84 coordinates to an OS National Grid reference and neighbouring squares |
| `/api/tiles/{z}/{x}/{y}.png` | OS Maps tile proxy |
| `/api/search`, `/api/nearest` | Place search and reverse geocoding |
| `/api/stations`, `/api/departures` | Nearby rail stations and live departures |
| `/api/stops`, `/api/arrivals` | London stops and live arrivals |
| `/api/buses` | Live bus positions in a bounding box |
| `/api/poi` | Nearby named places and opening hours |
| `/api/around` | A live snapshot of the surrounding context |
| `/api/suggest` | Ambient suggestion |
| `/api/hunt` | Five locally grounded things for a child to find |
| `/api/ask` | Bounded tool-use loop over server-sent events |
| `/api/push/subscribe`, `/api/push/unsubscribe` | Manage push nudges |
| `/api/interest` | Add to or read the out-of-coverage waitlist |
| `/api/health` | Feature availability and VAPID public key |

## Repository map

```text
cmd/malten          server binary and embedded web application
internal/osgrid     WGS84 and OS National Grid conversion
internal/llm        Anthropic Messages client and bounded tool loop
internal/nrail      National Rail station data and LDBWS client
internal/bods       Bus Open Data Service SIRI-VM client
internal/push       VAPID and encrypted Web Push
internal/server     stateless HTTP handlers and ambient suggestion loop
internal/server/web timeline UI, browser storage, Leaflet and PWA assets
```

Grid conversion is tested against Ordnance Survey's worked example to
approximately 0.2 metres in both directions. Web Push tests decrypt a message as
a browser would. `go test ./...` requires no network access or credentials.

## Data sources and attribution

- **Ordnance Survey** — “Contains OS data © Crown copyright and database
  rights.” Attribution remains visible on the map.
- **National Rail** — station coordinates from a vendored dataset under the Open
  Database Licence.
- **OpenStreetMap** — nearby places through Overpass, © OpenStreetMap
  contributors.
- **Open-Meteo** — weather data.

## Licence

[AGPL-3.0](LICENSE).
