# Malten

Ephemeral message streaming

## Overview

Malten is an ephemeral messaging service. It contains solely streams of text and nothing more. 
Streams have an idle lifetime of 1024 seconds. Each stream supports 1024 messages as a FIFO and 
1024 characters per message. Streams can be discovered through exploration or via the API.

## Design

Malten keeps everything in memory, nothing is ever written to disk or a database. This is to ensure privacy and security. We 
do not want to persist data and ideally also want to encrypt messages on the client side. Streams are aged out based on TTL 
and each stream limits the number of messages and char length to ensure we can comfortably run malten in memory on most devices. 

## Roadmap

- [ ] Presence - User presence status
- [ ] Secure Pipes - Stream between two users
- [ ] Decentralisation - Interconnect multiple Malten servers
- [x] EventSource support - Real time messaging directly to the browser
- [ ] Client side encryption - AES-256 on the user side so we never see the data
- [x] Configurable stream TTL - Per stream configuration to increase or decrease for DMs or Notes

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

Below is a high level overview of the API

### Streams

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

### Messages

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
