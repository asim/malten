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

The application opens on an empty spatial plane. Captured thoughts and
observations become nodes; relationships between them become edges. This is not
a diagram of the physical world. It is a space in which the person can gradually
form their own model.

The graph can be panned, zoomed and rearranged. Select a node to inspect it or
choose **Continue from here** before capturing another thought. That creates an
explicit relationship rather than connecting everything into an arbitrary
chronological chain.

Place remains useful context without becoming the interface. With permission,
the phone attaches location to a capture. Observations made near one another can
gain a subtle place relationship, and Malten can indicate which thoughts were
recorded near the person's current position. Geographic coordinates never
determine where nodes sit on the mental plane.

The current application deliberately contains no map, nearby-data feed,
transport dashboard or chat window. Existing server integrations remain in the
codebase as experiments that may later provide context in the background. Their
existence does not require them to occupy the interface.

The first version of the graph keeps connection-making partly deliberate. Later
work can explore ambient semantic connections and resurfacing, but the standard
is not how much a model can generate. It is whether Malten helps the person see a
relationship they can understand and act upon.

## Privacy and architecture

Malten is a single self-contained Go binary serving an installable web app. It
has no external Go dependencies and no database.

The server is stateless and anonymous. Nodes, edges and their context live in
the browser as structural data under `malten_graph1`. Existing notes and finds
from earlier versions are imported into the graph on first use. Browser storage
degrades to session storage and then memory rather than breaking the app.

The UI is embedded with `//go:embed`. The dormant Anthropic, Ordnance Survey,
National Rail, bus and Web Push integrations remain built directly over the Go
standard library.

## Quick start

```bash
go build ./...
go test ./...
go run ./cmd/malten
```

Open <http://localhost:8080>. The mental map works immediately and requires no
API key. Allowing browser location adds place context to new captures.

## Configuration

The current graph interface requires no server-side configuration. The retained
experimental endpoints read secrets from an environment variable or, failing
that, a file of the same name beside the binary.

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

Anthropic, OS, rail, bus and VAPID credentials remain server-side.

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
internal/server     stateless HTTP handlers and embedded web application
internal/server/web mental-map graph, local browser storage and PWA assets
```

Grid conversion is tested against Ordnance Survey's worked example to
approximately 0.2 metres in both directions. Web Push tests decrypt a message as
a browser would. `go test ./...` requires no network access or credentials.

## Data sources and attribution

- **Ordnance Survey** — retained grid, place-search and tile experiments.
- **National Rail** — station coordinates from a vendored dataset under the Open
  Database Licence.
- **OpenStreetMap** — nearby places through Overpass, © OpenStreetMap
  contributors.
- **Open-Meteo** — weather data.

## Licence

[AGPL-3.0](LICENSE).
