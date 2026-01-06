# Malten

A spatial timeline for the real world.

## What It Is

Malten is a timeline of where you are. Open it and see:

- 📍 Your location - street, area, postcode
- ⛅ Weather and what's coming
- 🚏 Live transport - buses, trains, countdowns
- ☕ What's nearby - cafes, shops, places
- 🕐 The rhythm of the day - sunrise, sunset, the hours

Move through the world and it updates. Take a photo of the snow falling. Note where you had coffee. Your timeline becomes a record of moments in places.

## Features

**Context-aware**: Updates based on where you are - walking, driving, stationary.

**Timeline**: Your history of places and moments. Persists locally. Private to you.

**Commands**: Ask for what you need.
- `/nearby cafe` - find coffee
- `/directions home` - walking route
- `/weather` - forecast
- `/bus` - next arrivals

**AI**: What's happening. "What's that building?" "Where can I get lunch?" "What time is sunset?"

**Notifications**: Get updates when backgrounded - transport times, weather changes.

**Map**: See the spatial index - places, agents, streets we've mapped.

## How It Works

### Agents

Invisible agents maintain different areas. They fetch weather, transport, places. They keep the world view fresh. You don't see them working, but they're there.

### The Spatial Index

Everything has a location. Places, weather, transport, moments. All indexed spatially. Query by where you are.

### Your Timeline

Stored locally on your device. Your path through space and time. Private. Exportable. Yours.

## Regional Coverage

| Region | Transport | Weather | Places |
|--------|-----------|---------|--------|
| London | ✓ TfL | ✓ | ✓ |
| UK | Partial | ✓ | ✓ |
| Ireland | Partial | ✓ | ✓ |
| Global | - | ✓ | ✓ |

Transport APIs are regional. Weather and places work everywhere.

## Privacy

- No ads
- No tracking
- No algorithms
- Timeline stored locally
- Location used only to show you context
- We don't sell your data because we don't collect it

## Business Model

Free to use. Considering:
- Pro subscription for cloud backup, sync across devices, extended history
- API access for developers
- No ads, ever

Still figuring this out. If you have thoughts, open an issue.

## Installation

### Prerequisites

- Go 1.21+
- Linux/macOS (Windows untested)

### Build

```bash
git clone https://github.com/asim/malten.git
cd malten
go build -o malten .
```

### Environment Variables

Copy `.env.example` to `.env` and configure:

```bash
cp .env.example .env
```

| Variable | Required | Description |
|----------|----------|-------------|
| `FANAR_API_KEY` | Yes* | AI API key from [fanar.ai](https://fanar.ai) |
| `FANAR_API_URL` | Yes* | AI API endpoint |
| `OPENAI_API_KEY` | Yes* | Alternative: OpenAI API key from [platform.openai.com](https://platform.openai.com) |
| `VAPID_PUBLIC_KEY` | Optional | Web push public key (if not set, push disabled) |
| `VAPID_PRIVATE_KEY` | Optional | Web push private key |
| `FOURSQUARE_API_KEY` | Optional | Places API fallback |
| `MU_API_TOKEN` | Optional | User authentication |

*Either Fanar or OpenAI required for AI features.

Generate VAPID keys (optional, for push notifications):
```bash
npx web-push generate-vapid-keys
```

### Run

```bash
# Development (serves from client/web/)
./malten -web=client/web

# Production (uses embedded files)
./malten
```

Default port: 9090. Access at http://localhost:9090

### Systemd (Production)

```bash
sudo cp malten.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable malten
sudo systemctl start malten
```

## Data Sources

| Data | Source | Notes |
|------|--------|-------|
| Weather | [Open-Meteo](https://open-meteo.com) | Free, model-based (may differ from BBC/Google by 1-2°C) |
| Transport | TfL API | London only, other regions TODO |
| Places | OpenStreetMap + Foursquare | OSM primary, Foursquare fallback |
| Prayer Times | Aladhan API | Calculation-based |
| Routing | OSRM | Walking directions |

## Related Projects

- [Reminder](https://reminder.dev) - Daily verses and reflections
- [Mu](https://mu.xyz) - A personal platform without ads

## License

GPL-3.0 - See [LICENSE](LICENSE)

---

*The world is full of signs for those who reflect.*
