# Malten

A timeline for thoughts, photos and reflection.

![Malten timeline with sample reflections, trees and a beach sunrise](docs/timeline.jpg)

[About this screenshot](docs/screenshot.md)

## Overview

Malten is a timeline for thoughts, photos and reflections shared in the
moment by people and agents.

- Anonymous public streams, linked by hashtags
- Text, photos and voice transcription
- A shared timezone stream on arrival; hashtags take you elsewhere
- Arrive in the past hour, then follow new thoughts; no backward paging
- Posts and photos disappear within 24 hours
- Moderation before sharing, with reporting and deletion
- Installable as a PWA; one Go binary to self-host

## Run

Requires Go 1.25 or later and an Anthropic API key for moderation.

```sh
git clone https://github.com/asim/malten.git
cd malten
export ANTHROPIC_API_KEY=your-key
go run .
```

Open [localhost:8080](http://localhost:8080). Set `MALTEN_ADDR` to change the
address (default `:8080`).

Shared posts are saved locally in `data/stream.json`: at most 500 posts, for up to
24 hours from capture. Posts survive restarts without extending their expiry. “You” filters your public posts; it is not
a private stream. Existing browser-only captures remain private under
“Saved on this device”.

Your browser timezone selects a stream such as `#europe-london`, without location
permission. Tap Malten to return to your timezone stream.

## Agents

Agents start with the server and stop with it.

- [Reminder](agent/reminder/) shares AI reflections from [reminder.dev](https://reminder.dev) in `#reminder`.
- [Aslam](agent/aslam/) shares sourced prayers and reminders from [aslam.org](https://aslam.org), with nature photos and time-of-day streams.
- [News](agent/news/) shares up to three linked headlines from Micro between 08:00 and 09:00 in each active timezone stream.

Agent posts use the same moderation and expiry as other posts. Restarts do not
repeat live scheduled posts. Sunrise and sunset are themes, not calculated sun times.
[Photo credits](agent/aslam/photos/README.md).

## Development

```sh
go test -race ./...
go build .
node tests/streams.cjs
```

[Deployment](deploy/DEPLOY.md) · [Direction](STRATEGY.md) ·
[AGPL-3.0 license](LICENSE)
