# Malten

Anonymous ephemeral messaging

## Overview

Malten is an ephemeral messaging app. It contains solely streams of text and nothing more. 
Streams have an idle lifetime of 1024 seconds. Each stream supports 1024 messages as a FIFO and 
1024 characters per message. Streams can be discovered through exploration or via the API.

## Features

- [x] Presence - User presence status
- [ ] Secure Pipes - Stream between two users
- [x] Speech to text - Talk to malten and write speech
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

### OpenAI

To enable conversations using [ChatGPT](https://openai.com/blog/chatgpt) specify the `OPENAI_API_KEY` environment variable

```
OPENAI_API_KEY=xxxx ./malten
```

To invoke ChatGPT use `/malten` as the prefix to your command

## WhatsApp

To enable usage through WhatsApp set the env var below and scan the QR code output on the command line. 

```
WHATSAPP_CLIENT=true
```

Malten has access to all your chats/groups. The next message sent in a chat/group will create a new stream 
observer. When you then prompt with `@malten`, it will send as a command to the server and it will be answered 
by the OpenAI agent if enabled.

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

### Events

Get server sent events for real time messages

```
var events = new EventSource("/events")
```

Params for events

```
stream: string e.g stream=foo
```

Event structure

```
Id string
Type string e.g message or event
Created int64 e.g Unix nano timestamp
Payload object
```

### Commands

Commands are sent to ChatGPT or other related systems that are listening

```
POST /commands
```

Send the params

```
stream: string e.g stream=foo
prompt: string e.g prompt=hello
```

The response to commands are seen within messages and events

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
message: string e.g message=helloworld
```

An example

```
POST /messages?stream=foo&message=helloworld
```
