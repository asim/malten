# Malten

A conversational AI assistant with real-time messaging

## Overview

Malten is an AI-powered assistant that combines ephemeral messaging with useful commands. Ask questions, get crypto prices, receive Islamic reminders, and more. Messages are organized into streams that expire after idle time.

## Features

- AI assistant powered by Fanar or OpenAI
- Real-time messaging via WebSocket
- Crypto price lookups (`price btc`, `eth price`)
- Islamic reminders with Quran, Hadith, and Names of Allah (`reminder`)
- Natural language queries (`what does islam say about patience`)
- Ephemeral streams with 1024 message limit
- PWA support for mobile

## Usage

```bash
go install github.com/asim/malten@latest
malten
```

Browse to `localhost:9090`

### AI Integration

Set one of:

```bash
# Fanar
FANAR_API_KEY=xxx FANAR_API_URL=https://api.fanar.qa/v1 ./malten

# OpenAI
OPENAI_API_KEY=xxx ./malten
```

## Commands

Commands work with or without the `/` prefix:

- `help` - Show available commands
- `new` - Create a new stream
- `goto <stream>` - Switch to a stream
- `price <coin>` - Get crypto price (btc, eth, sol, etc.)
- `reminder` - Daily Islamic reminder
- `reminder <query>` - Search Quran and Hadith

### Natural Language

You can also ask naturally:

- `btc price` or `price of eth`
- `what does the quran say about patience`
- `hadith about charity`

## API

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
