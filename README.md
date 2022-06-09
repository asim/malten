# Malten

Secure ephemeral messaging

## Overview

Malten is a secure ephemeral messaging service. It contains solely streams of text and nothing else. 
Messages have a lifetime of 24 hours. Each stream supports 1000 messages and 512 characters per message. There 
can only ever be 1000 streams at any given time. Streams can be discovered through exploration or by listing them via the API.

## Rationale

Most messaging services today are storing the messages on a server. Even when they are deleted, it's likely those messages are 
still stored somewhere or the data was at one point in time backed up. Services like WhatsApp and Signal might be secure or 
encrypted but still continue to persist data. Many of the services are also run by giant tech corporations. We need a simple 
and secure self hostable alternative. 

## Design

Everything is stored in memory, nothing is ever written to disk. This is to ensure security of the service. We do not want to 
persist and ideally also want to encrypt messages client side in future. Streams are maintained as an LRU to ensure once the 
1000 stream cap is hit that we age out the oldest. Limits in streams, messages and char length ensure we can comfortably run 
malten in memory on most servers.

## Roadmap

- [ ] Websocket support
- [ ] Client side encryption
- [ ] Configurable stream TTL

## Usage

Use the Go toolchain to install

```
go get github.com/asim/malten
```

Or download the latest release binary

- https://github.com/asim/malten/releases/latest

Malten is a self executable, simply do `./malten`

Listens on port 9090

## API

To retrieve a list of streams

```
GET /streams
```

To retrieve messages

```
GET /messages
```

Params for messages

```
stream: string e.g stream=foo
limit: integer e.g limit=25
direction: integer e.g direction=1 or direction=-1
last: timestamp e.g last=1652035113591000000
```

An example retrieval

```
GET /messages?direction=1&limit=25&last=1652035113591000000&stream=foo
```

To create messages

```
POST /messages
```

Params for messages

```
stream: string e.g stream=foo
text: string e.g text=helloworld
```

An example

```
POST /messages?stream=foo&text=helloworld
```

## Web

Live server https://malten.com

## Testing 

For development http://localhost:9090
