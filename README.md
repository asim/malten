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

**AI**: Ask anything. "What's that building?" "Where can I get lunch?" "What time is sunset?"

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

## Development

```bash
go build -o malten .
./malten
```

Requires:
- Go 1.21+
- API keys for weather, transport, AI (see .env.example)

## Related Projects

- [Reminder](https://reminder.dev) - Daily verses and reflections
- [Mu](https://mu.xyz) - A personal platform without ads

## License

AGPL-3.0

---

*The world is full of signs for those who reflect.*
