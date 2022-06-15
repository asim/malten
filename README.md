# Malten

Anonymous ephemeral messaging

## Overview

Malten is an ephemeral messaging service. It contains solely streams of text and nothing else. 
Messages have a lifetime of 1024 seconds. Each stream supports 1024 messages and 1024 characters per message. There 
can only ever be 1024 streams at any given time. Streams can be discovered through exploration or listing via the API.

## Rationale

Most messaging services today store the messages on a server. Even when they are deleted, it's likely those messages are 
still stored somewhere or the data was at one point in time backed up. Services like WhatsApp and Signal might be secure or 
encrypted but still continue to persist data on the client. Many of the services are also run by giant tech corporations. 
We need a simple and secure self hostable alternative. 

## Design

Malten keeps everything in memory, nothing is ever written to disk or a database. This is to ensure privacy and security. We 
do not want to persist data and ideally also want to encrypt messages on the client side. Streams are maintained as an LRU 
to ensure once the 1024 stream cap is hit that we age out the oldest. Limits in streams, messages and char length ensure we can 
comfortably run malten in memory on most servers. 

## Roadmap

- [ ] Secure Pipes - Synchronous stream between 2 parties
- [ ] Decentralisation - Interconnect multiple Malten servers
- [ ] Websocket support - Real time messaging directly to the browser
- [ ] Client side encryption - AES-256 on the user side so we never see the data
- [ ] Configurable stream TTL - Per stream configuration to increase or decrease for DMs or Notes

## Usage

Use the Go toolchain to install

```
go get github.com/asim/malten
```

Or download the latest release binary

- https://github.com/asim/malten/releases/latest

Malten is a self executable, to run it simply do 

```
./malten
```

Browse to `localhost:9090` or host behind an nginx proxy

## API

To create a new stream

```
POST /streams
```

Params used

```
stream: string e.g stream=foo
private: bool e.g private=true
```

To retrieve a list of streams

```
GET /streams
```

To retrieve messages for a stream

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
