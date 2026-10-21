# Malten

Anonymous ephemeral messaging

## Overview

Malten is a small place to get thoughts out: keep them on your device, or share in anonymous ephemeral streams.

One stream interface serves both spaces. **Public** opens `/` and `/#stream`: anonymous, ephemeral server streams. **For you** opens `/private` and `/private#stream`: multiple streams held in IndexedDB on this device. New, Share, Go to, the composer and messages use the same page and JavaScript. Old `/streams.html#stream` links redirect to `/#stream`.

The stream backend remains ephemeral.
Streams have an idle lifetime of 1024 seconds. Each stream supports 1024 messages as a FIFO and 
1024 UTF-8 bytes per text message. Photo-heavy streams also have an 8 MiB content budget, evicting the oldest posts first. There is no stream directory. New public streams have random links; anyone who has the link can read them.

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

The default model for conversations is `gpt-6-astra` with `low` reasoning effort.

To invoke ChatGPT use `/malten` as the prefix to your command. The agent only replies to explicit commands; ordinary stream posts do not trigger an AI reply. Prompts appear in the stream immediately, with the AI response arriving separately.

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

To retrieve presence for a known stream (no directory or enumeration)

```
GET /streams?stream=foo
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
image: optional JPEG/PNG data URL (512 KiB decoded maximum; image-only posts allowed)
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

After the first online visit finishes caching the app, the personal timeline can open
and save text and photos offline. Opening the app offline offers a link to **For you**. A small service worker caches only the personal app shell;
it never caches stream API responses. Public streams still require a connection
and expire according to their TTL.

## On-device streams

- Use New to create a separate stream and Go to to return to one. Messages and drafts, including photos, are kept separately for each stream.
- Enter sends; Shift+Enter inserts a newline. Photo, Camera and Talk work in both spaces.
- Ordinary local messages make no stream or AI API requests. `/malten` explicitly sends the current stream's recent context to `/ask`, a stateless server relay to OpenAI. No stream ID or account identity is added to the model request. Message content itself can still identify you; this is not an anonymity network or on-device inference.
- The AI uses up to 40 recent messages, 32,000 text bytes and two photos. The question and answer remain in the local stream. Older entries remain stored locally but fall outside the model context window. An unsuccessful request leaves the question saved; Ask again is available in its message menu.
- In Public, `/malten` uses the recent, unexpired messages from that stream. The agent no longer keeps a second history that can outlive stream expiry. Ordinary posts never trigger replies.
- Share on a local stream asks before copying its messages to a new ephemeral public stream. It never publishes automatically when you switch spaces or invoke AI. Share on a public stream shares its existing link. Sharing does not turn copied `/malten` text into new commands.
- Menu → Export streams / Import streams backs up all local streams. Previous timeline exports remain supported; existing IDs are not overwritten.
- Existing personal timeline entries and drafts migrate into the initial local stream. No existing local content is deleted or uploaded.

Browser storage belongs to this device and origin. Clearing it removes your streams; export a backup before changing devices or domains. Local streams persist until deleted and work offline. They are not an encrypted vault. Browser dictation may use the browser provider's speech service. WhatsApp forwarding remains text-only.

### Browser regression checks

The UI uses plain JavaScript with no frontend build step. For browser tests only,
install Playwright (`npm install --no-save --package-lock=false playwright@1.62.1`),
then `npx playwright install chromium`. Build with `go build -o malten .` and run
`node scripts/test-timeline.cjs` with port 9090 free. The script uses isolated browser
profiles and never enables OpenAI or WhatsApp. `CHROMIUM_PATH` and `MALTEN_BINARY`
can select existing test binaries. The UI has no CDN dependency.
