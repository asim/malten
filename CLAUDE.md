# CLAUDE.md

Guidance for agents working in this repository. These are engineering and
product invariants, not marketing copy.

## What Malten is

Malten helps a person build a mental model of their world.

The primary interface is a spatial graph, not a geographic basemap, feed or chat
window. Thoughts and observations are nodes. Meaningful relationships are
edges. Place, time and movement are context attached to that network; they do
not dictate its visual coordinates.

The application should deepen human attention and agency. AI may eventually
index, associate, compress and resurface material in the background, but it
must not become the centre of the experience or make the person defer to it.

The core loop is capture, contextualise, connect, resurface and understand.

## Current application

The browser renders an open, pannable and zoomable SVG plane:

- each capture is a persistent node;
- selecting a node reveals its detail and connection count;
- “Continue from here” makes the next capture explicitly related;
- captures made within roughly 180 metres can acquire a dashed place edge;
- location works through browser geolocation and is not country-bound;
- location affects context and retrieval, never literal placement on a map.

The graph schema is stored under `malten_graph1` in `localStorage`:

```text
{
  v: 1,
  nodes: [{ id, text, created_at, location: { lat, lng, accuracy } | null, x, y }],
  edges: [{ id, from, to, kind: "related" | "place" }]
}
```

The server has no copy. Storage degrades to `sessionStorage` and then memory
when durable browser storage is unavailable.

## Architecture

```text
cmd/malten          server binary and embedded UI
internal/server     HTTP handlers and embedded web assets
internal/server/web mental-map graph, local browser store and PWA shell
internal/llm        dependency-free Anthropic Messages client
internal/osgrid     WGS84 and OS National Grid conversion
internal/nrail      National Rail data and LDBWS client
internal/bods       Bus Open Data Service client
internal/push       encrypted Web Push
```

The OS, transport, POI, question and push handlers are retained as dormant
capabilities while the graph interaction is developed. They are not part of the
primary interface. Do not reintroduce a map, transport dashboard, nearby-data
feed or chat surface merely because those endpoints exist.

## Invariants

- **Local first.** Do not add server-side persistence of user content without an
  explicit product decision and corresponding privacy design.
- **Structural data.** Store nodes, edges and context, never rendered markup.
- **The graph is not geography.** Physical coordinates are one input. They
  should not determine graph x/y coordinates or require a basemap.
- **Relationships should mean something.** Avoid connecting every capture into
  a chronological chain merely to make the graph look populated.
- **Silence is a feature.** Do not fill the interface with live data, nearby
  counts or model-generated suggestions.
- **No chat-first drift.** Direct questions may remain an escape hatch, but
  capture and the graph must remain primary.
- **Single binary.** Keep the UI embedded with `//go:embed`; no CDN or runtime
  asset dependency.
- **No external Go dependencies.** Preserve the standard-library-only server.
- **Keep dormant services safe.** Anthropic, OS, rail, bus and VAPID keys remain
  server-side. Public-data proxy responses must not store user content.

## Build and test

```bash
go build ./...
go test ./...
go run ./cmd/malten
```

Tests must require no network access or credentials.
