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

## Awareness System

Agents observe changes in each area. When something interesting happens, you get notified:

- 🌧️ Rain starting soon
- ⚠️ Transport disruption on your route  
- ☕ New cafe opened nearby

The system filters noise - you only see what matters.

## Push Notifications

Enable notifications to get updates when backgrounded:
- 🚌 Bus times when you're at a stop
- 🕌 Prayer reminders 10 min before
- ☀️ Morning weather at 7am

## Architecture

```
┌─────────────────────────────────────────────┐
│ Agents (per area, deterministic)            │
│ - Fetch weather, transport, places          │
│ - Accumulate observations                   │
└─────────────────────────────────────────────┘
                    │
                    ▼
┌─────────────────────────────────────────────┐
│ Awareness Filter (LLM, periodic)            │
│ - What's worth telling the user?            │
│ - Filter noise, surface what matters        │
└─────────────────────────────────────────────┘
                    │
                    ▼
┌─────────────────────────────────────────────┐
│ Delivery                                    │
│ - Timeline card if app open                 │
│ - Push notification if backgrounded         │
└─────────────────────────────────────────────┘
```

Data model:
- `events.jsonl` - append-only log of facts about the world
- `spatial.json` - quadtree spatial index (rebuildable from events)
- `localStorage` - your private timeline

## Try It

```bash
go build -o malten .
FANAR_API_KEY=xxx FANAR_API_URL=https://api.fanar.qa/v1 ./malten
```

Open `localhost:9090`, enable location.

## Commands

| Command | Description |
|---------|-------------|
| `/ping` | Update location, get context |
| `/nearby <type>` | Find nearby places |
| `/directions <place>` | Walking directions |
| `/weather` | Current weather |
| `/bus` | Bus times |
| `/prayer` | Prayer times |
| `/observe` | See pending observations |
| `/system` | System stats and health |

## Data Sources

- Location: OpenStreetMap Nominatim
- Weather: Open-Meteo
- Prayer times: Aladhan
- Transport: TfL (London)
- Places: OpenStreetMap + Foursquare

## Files

- `claude.md` - Full development context
- `ARCHITECTURE.md` - The spacetime model
- `events.jsonl` - Event log (don't delete)
- `spatial.json` - Spatial index

## License

AGPL-3.0
