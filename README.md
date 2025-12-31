# Malten

Anonymous ephemeral messaging

## Overview

Malten is an ephemeral messaging app. Streams of text that expire after 1024 seconds of idle time. 
Each stream supports 1024 messages (FIFO) with 1024 characters per message.

## Features

- Real-time messaging via WebSocket
- AI assistant (mention @malten in your message)
- Slash commands: `/help`, `/streams`, `/new`, `/goto <stream>`
- PWA support for mobile
- Configurable stream TTL

## Usage

```bash
go install github.com/asim/malten@latest
malten
```

Browse to `localhost:9090`

### AI Integration

To enable the AI assistant, set one of:

```bash
# Fanar (preferred)
FANAR_API_KEY=xxx FANAR_API_URL=https://api.fanar.qa/v1 ./malten

# Or OpenAI
OPENAI_API_KEY=xxx ./malten
```

Mention `@malten` or `malten` in your message to get a response.

## Commands

- `/help` - Show available commands
- `/streams` - List public streams  
- `/new [name]` - Create a new stream
- `/goto <stream>` - Switch to a stream (or use #stream in URL)

## API

### Streams

```
GET  /streams          - List public streams
POST /streams          - Create stream (stream=name, private=bool, ttl=seconds)
```

### Messages

```
GET  /messages         - Get messages (stream=x, limit=25, direction=1/-1, last=timestamp)
POST /messages         - Post message (stream=x, message=text)
```

### Commands

```
POST /commands         - Send command (stream=x, prompt=text)
```

### Events (WebSocket)

```
WS /events?stream=x    - Real-time message stream
```

## License

MIT
