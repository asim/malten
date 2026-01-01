## The Model - READ THIS FIRST

### User Experience
Open app → see real world around you instantly:
- Weather, prayer times, area name
- Bus/train times, nearby places
- Move → updates automatically
- Timeline of events as messages
- Same area = same view (shared spatial reality)

### Five Primitives
| Primitive | Purpose |
|-----------|----------|
| **Streams** | Textual view of geo space. One per area. |
| **Agents** | Indexers. One per area. Build world view. |
| **Commands** | Actions. Everything is a command. |
| **Database** | Quadtree. Spatial index. World state. |
| **Events** | Replayable log. `events.jsonl` |

### Key Insight
- **Stream = Geo Area** - moving through space = moving through streams
- **Agent = Per Stream** - each area has an agent maintaining it
- **Messages = Events in Space** - what happens in that area

---

## Last Session - 2026-01-01 15:00 UTC

### What We Built
1. Commands as core abstraction
2. Proactive messages (detect changes, create events)
3. Message persistence (24hr localStorage)
4. Expanded agent indexing (trains, tubes, more POIs)
5. Quadtree-first lookups (no API if cached)

### Agent Indexes
- Transport: stations, bus stops
- Food: cafes, restaurants, pubs
- Health: pharmacies, clinics
- Services: banks, ATMs, post offices
- Shopping: supermarkets, bakeries

### Live Data (30s)
- Weather + rain forecast
- Prayer times
- Bus/tube/rail arrivals

### Open Question
If streams = geo areas, what about private/custom streams?
- Option 1: Remove - pure spatial
- Option 2: Keep with prefix (~private, @user)
- Option 3: Hybrid - default geo, can create others

### Git: 094dc25

### To Continue
1. Implement geohash → stream mapping
2. Auto-switch stream on move
3. Decide on private streams
4. Map message format
5. Agent learning/prediction

---

## Bugs to Fix (reported 2026-01-01)

### 1. Location not updating
- Shows Whitton when user is in Hampton
- Geolocation might not be triggering
- Or location update not calling setLocation properly

### 2. Cards disappearing
- Bus update and local info disappeared after some time
- localStorage might be getting cleared/overwritten
- Or deduplication logic too aggressive?

### 3. Local info not clickable
- Shops/places should have Map links
- Links might not be rendering

### 4. Context splitting
- Weather/prayer showing as separate message
- Location line missing
- Context string might be malformed

### 5. No bus info in Hampton
- Agent might not have indexed area
- Or no bus stops within 100m radius
- Check quadtree for Hampton area entities
