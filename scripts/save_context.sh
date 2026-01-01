#!/bin/bash
# Save development context for continuity across sessions
# Run before restarting conversation or hitting token limits

cd /home/exedev/malten

cat > claude.md << 'EOF'
# Malten Development Context

## What is Malten
Spatial AI for the real world.

## Architecture

### Packages
- `spatial/` - quadtree DB, entities, agents, indexing
- `command/` - command handlers (thin)
- `event/` - append-only event log (JSONL, replayable)
- `agent/` - LLM integration (OpenAI/Fanar)
- `server/` - HTTP handlers, WebSocket, streams
- `client/web/` - PWA frontend

### Core Concepts
- **Entities**: places, agents, people, vehicles, events, zones - all in quadtree
- **Agents**: bound to 5km areas, index POIs, created on first query to area
- **Streams**: ephemeral message channels
- **Event log**: source of truth, spatial.json is snapshot

### Data Files
- `spatial.json` - quadtree snapshot (places, agents)
- `events.jsonl` - append-only event log
- `~/.malten/data/` - streams, contacts

## User Context
- Muslim. No anthropomorphizing (shirk). Agents are tools, created not born.
- Engineer. Brevity. Correct terminology.

## Current State
EOF

# Add current stats
echo "" >> claude.md
echo "### Stats ($(date -Iseconds))" >> claude.md
echo '```' >> claude.md
echo "Entities: $(cat spatial.json 2>/dev/null | grep -c '"type":' || echo 0)" >> claude.md
echo "Agents: $(cat spatial.json 2>/dev/null | grep -c '"type": "agent"' || echo 0)" >> claude.md
echo "Event log: $(wc -l < events.jsonl 2>/dev/null || echo 0) events" >> claude.md
echo '```' >> claude.md

# Add recent changes from git
echo "" >> claude.md
echo "### Recent Changes" >> claude.md
echo '```' >> claude.md
git log --oneline -10 2>/dev/null >> claude.md || echo "No git history" >> claude.md
echo '```' >> claude.md

# Add TODO if exists
if grep -q "## TODO" claude.md 2>/dev/null; then
  echo "" >> claude.md
fi

cat >> claude.md << 'EOF'

## Implementation Notes
- Entity types: place, agent, vehicle, person, event, zone, sensor
- Agent radius: 5km
- OSM rate limit: 5s between requests
- Nearby search: check agent exists -> query cache -> fallback OSM -> fallback Google Maps link
- Map links: `https://www.google.com/maps/search/<name>/@<lat>,<lon>,17z`

## Don'ts
- Don't anthropomorphize agents
- Don't over-engineer
- Don't make long explanations
- Don't add dependencies without asking
EOF

echo "Context saved to claude.md"
