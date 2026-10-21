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

Use Go 1.26 or newer to install

```
go install github.com/asim/malten@latest
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

## Development

Run `bash tests.sh` to verify modules, vet, run race tests, and test/build with
CGO disabled. Tests run locally without OpenAI credentials or WhatsApp login.
CI also builds release archives on each push and pull request.

WhatsApp uses pure Go SQLite, including in release binaries. Existing `malten.db`
files are retained; back up the database before upgrading, as WhatsApp's store
schema is migrated on startup.

## Releases

After merging and passing CI, push a semantic version tag (for example `v0.2.0`).
The release workflow reruns tests and publishes archives and SHA-256 checksums.
Archives target Linux (386, amd64, armv7, arm64), macOS (amd64, arm64), and
Windows (386, amd64, arm64). To validate packaging without publishing, run
`goreleaser release --snapshot --clean` with GoReleaser v2.

## Install as an app

Serve Malten over HTTPS (localhost also works for development). In Chrome or Edge,
use the address-bar install icon or the **Install** button when it appears. On
an iPhone or iPad, use Safari's Share menu and **Add to Home Screen**. The app
opens in its own window with the Malten icon.

Installation does not add offline storage: messages still live on the server and
expire according to the stream TTL. A service worker is not required for modern
browser installation; offline capture is not implemented yet.
