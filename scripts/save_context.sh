#!/bin/bash
# Save development context for continuity across sessions
# Run before restarting conversation or hitting token limits

set -e
cd /home/exedev/malten

# Trim old messages from current conversation, keep last 100
CONV_ID=$(sqlite3 "$HOME/.config/shelley/shelley.db" "SELECT conversation_id FROM conversations ORDER BY updated_at DESC LIMIT 1;")
if [ -n "$CONV_ID" ]; then
    MSG_COUNT=$(sqlite3 "$HOME/.config/shelley/shelley.db" "SELECT COUNT(*) FROM messages WHERE conversation_id='$CONV_ID';")
    if [ "$MSG_COUNT" -gt 200 ]; then
        KEEP=100
        sqlite3 "$HOME/.config/shelley/shelley.db" "DELETE FROM messages WHERE conversation_id='$CONV_ID' AND sequence_id NOT IN (SELECT sequence_id FROM messages WHERE conversation_id='$CONV_ID' ORDER BY sequence_id DESC LIMIT $KEEP);"
        echo "Trimmed conversation from $MSG_COUNT to $KEEP messages"
    fi
fi

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

# Add working state section - the key for continuity
echo "" >> claude.md
echo "## Working State" >> claude.md
echo "" >> claude.md

# Uncommitted changes show current work
if [ -n "$(git status --porcelain 2>/dev/null)" ]; then
  echo "### Uncommitted Changes" >> claude.md
  echo '```' >> claude.md
  git diff --stat HEAD 2>/dev/null | head -20 >> claude.md
  echo '```' >> claude.md
  echo "" >> claude.md
fi

# Session notes file for manual notes
if [ -f "SESSION.md" ]; then
  echo "### Session Notes" >> claude.md
  cat SESSION.md >> claude.md
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
