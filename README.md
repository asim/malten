# Malten

Spatial AI for the real world. Context-aware of what's around you.

## What It Does

Open the app → instantly see:
- 📍 Where you are (street, postcode)
- ⛅ Weather + rain forecast  
- 🕌 Current prayer time, next prayer
- 🚏 Live bus/train arrivals with countdown
- ☕ Nearby cafes, restaurants, pharmacies, shops

Move → context updates automatically (adaptive: 5s driving, 10s walking, 30s stationary).

Ask anything → AI with spatial context answers.

## The Vision

The world is filled with signs (ayat) of the Creator. Malten surfaces these reminders subtly:
- Prayer times ground you in the rhythm of the day
- Names of Allah at each prayer - The Provider at Dhuhr, The Light at Maghrib
- Verses that speak to the moment - "By the morning sunlight" at Duha

Utility and purpose, together.

## Architecture

### The Spacetime Model

```
events.jsonl     = the ledger of spacetime (facts about the world)
spatial.json     = materialized present (quadtree, rebuildable)
stream/websocket = real-time propagation of events
localStorage     = your worldline (private timeline)
```

### Agents

Agents operate invisibly in the background, like angels maintaining the world:
- One agent per area (geohash-based)
- Fetch weather, transport, places
- Store in spatial index
- Accumulate observations for awareness system

### Regional Couriers

Couriers walk between agents to map streets:
- Cluster agents by proximity (50km)
- One courier per cluster
- Routes stored as street geometry
- Enables the map view

### Awareness System

Agents observe changes. LLM filters noise:
- 🌧️ Rain starting soon
- ⚠️ Transport disruption
- 🕌 Prayer approaching

## Regional Support

| Region     | Transport | Weather | Prayer | POIs | Streets |
|------------|-----------|---------|--------|------|--------|
| London     | ✓ TfL     | ✓       | ✓      | ✓    | ✓      |
| Cardiff    | TODO      | ✓       | ✓      | ✓    | ✓      |
| Dublin     | TODO      | ✓       | ✓      | ✓    | ✓      |
| Manchester | TODO      | ✓       | ✓      | ✓    | TODO   |
| USA        | TODO      | ✓       | ✓      | ✓    | Partial |
| Global     | -         | ✓       | ✓      | ✓    | Via courier |

Transport APIs are region-specific. Weather, prayer, and POIs work globally.

## Commands

| Command | Description |
|---------|-------------|
| `/ping` | Update location, get context |
| `/nearby <type>` | Find nearby places |
| `/directions <place>` | Walking directions |
| `/weather` | Current weather |
| `/bus` | Bus times |
| `/prayer` | Prayer times |
| `/reminder` | Daily verse and Name of Allah |
| `/agents` | Agent status |
| `/couriers` | Regional courier status |
| `/system` | System stats |
| `/map` | Link to map view |

## Data Sources

- **Location**: OpenStreetMap Nominatim
- **Weather**: Open-Meteo
- **Prayer times**: Aladhan
- **Transport**: TfL (London), more regions TODO
- **Places**: OpenStreetMap + Foursquare fallback
- **Reminders**: reminder.dev

## Development

```bash
# Build
go build -o malten .

# Run
FANAR_API_KEY=xxx FANAR_API_URL=https://api.fanar.qa/v1 ./malten

# Test
curl -X POST localhost:9090/commands -d 'prompt=/ping&lat=51.45&lon=-0.35'
```

## Key Files

| File | Purpose |
|------|--------|
| `claude.md` | Full development context |
| `events.jsonl` | Event log (don't delete!) |
| `spatial.json` | Spatial index |
| `courier_state.json` | Courier state |
| `regional_couriers.json` | Regional courier state |
| `push_subscriptions.json` | Push notification subscriptions |

## License

AGPL-3.0
