# Malten

Spatial AI for the real world. Foursquare if built in the AI era.

Contextually aware of what's around you. Fully agentic - agents continuously index and maintain the world view for each area.

## What It Does

Open the app → instantly see:
- 📍 Where you are (street, postcode)
- ⛅ Weather + rain forecast  
- 🕌 Current prayer time, next prayer
- 🚏 Live bus/train arrivals with countdown
- ☕ Nearby cafes, restaurants, pharmacies, shops

Move → it updates automatically. No searching, no typing. Just awareness.

Ask anything → AI with spatial context answers.

**Push notifications**: Enable notifications and get updates even when backgrounded:
- 🚌 Bus times when you're at a stop
- 🕌 Prayer reminders 10 min before
- ☀️ Morning weather at 7am
- ☀️ Ad-Duha reminder at 10am

## The Model

See `ARCHITECTURE.md` and `claude.md` for the full spacetime model.

```
events.jsonl     = cosmic ledger (facts about the world, append-only)
spatial.json     = materialized quadtree (current state, rebuildable)
stream/websocket = real-time event propagation
localStorage     = your private timeline (your worldline)
```

**Primitives**: Streams, Agents, Commands, Database (quadtree), Events

**Privacy**: Your conversations stay in localStorage. Only facts about the world go to the server.

## How It Works

**Instant data**: When you arrive somewhere, Malten fetches what you need immediately - weather, transport, places. No waiting.

**Smart caching**: Data is cached spatially in a quadtree. Move 500m? Get fresh local data. Stay put? Use cache.

**Background agents**: Agents per area continuously index - more places, more detail. You never wait for them.

**Adaptive updates**: Moving fast (driving)? Updates every 5s. Walking? 10s. Stationary? 30s.

## Try It

```bash
go build -o malten .
./malten
```

Open `localhost:9090`, enable location.

### AI Integration

```bash
# Fanar (production)
FANAR_API_KEY=xxx FANAR_API_URL=https://api.fanar.qa/v1 ./malten

# OpenAI (fallback)
OPENAI_API_KEY=xxx ./malten
```

## Data Sources

- **Location**: OpenStreetMap Nominatim
- **Weather**: Open-Meteo
- **Prayer times**: Aladhan
- **Transport**: TfL (London buses, tubes, trains)
- **Places**: OpenStreetMap + Foursquare
- **Traffic**: TfL disruptions

## Architecture

```
User at location
  → Query quadtree (instant)
  → Cache miss? Fetch, store as event, return
  → Agent enriches area in background
  → events.jsonl = source of truth
  → spatial.json = rebuildable from events
```

No waiting. Cache-first. Fetch on demand. Enrich in background.

## Important Files

- `ARCHITECTURE.md` - The spacetime model (read first)
- `claude.md` - Full development context for AI assistants
- `events.jsonl` - Append-only event log (never delete)
- `spatial.json` - Quadtree state (rebuildable)

## License

AGPL-3.0
