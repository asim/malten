# Malten

Spatial AI for the real world. Your context-aware assistant that knows where you are.

## What It Does

Open the app → instantly see:
- 📍 Where you are (street, postcode)
- ⛅ Weather + rain forecast
- 🕌 Current prayer time, next prayer
- 🚏 Live bus/train arrivals with countdown
- ☕ Nearby cafes, restaurants, pharmacies, shops

Move → it updates automatically. No searching, no typing. Just awareness.

## How It Works

**Instant data**: When you arrive somewhere, Malten fetches what you need immediately - weather, transport, places. No waiting.

**Smart caching**: Data is cached spatially. Move 500m? Get fresh local data. Stay put? Use cache.

**Background agents**: After serving you instantly, agents continue indexing the area - more places, more detail. You never wait for them.

## Try It

```bash
go install github.com/asim/malten@latest
malten
```

Open `localhost:9090`, enable location.

### AI Integration (optional)

```bash
# For natural language queries
OPENAI_API_KEY=xxx ./malten
# or
FANAR_API_KEY=xxx FANAR_API_URL=https://api.fanar.qa/v1 ./malten
```

## Data Sources

- **Location**: OpenStreetMap Nominatim
- **Weather**: Open-Meteo
- **Prayer times**: Aladhan
- **Transport**: TfL (London buses, tubes, trains)
- **Places**: OpenStreetMap via Overpass
- **Traffic**: TfL disruptions

## Architecture

```
User at location
  → Check cache (instant)
  → Cache miss? Fetch now, cache, return
  → Background: agent enriches area
  → Next request: cache hit (instant)
```

No waiting. Cache-first. Fetch on demand. Enrich in background.

## License

MIT
