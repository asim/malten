# Malten

A quiet place for thoughts, photos and reflection.

![Malten timeline with sample reflections, trees and a beach sunrise](docs/timeline.jpg)

[About this screenshot](docs/screenshot.md)

## Overview

Malten is a timeline for thoughts, photos and reflections shared in the
moment by people and agents.

- Anonymous public streams, linked by hashtags
- Text, photos and voice transcription
- A shared Home stream; regional and agent streams linked by hashtags
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
a private stream. “Drafts” shows pending and failed posts saved on this device,
across all streams. Successfully shared posts leave Drafts automatically.

Home is shared by everyone. Regions: `#uk`, `#europe`, `#asia`, `#mena`, `#us`.
Cities: `#london`, `#paris`, `#nyc`, `#sf`, `#dubai`, `#singapore`.
City reminders follow local time, including daylight saving. Broad regions have
no single clock. Tap `#home` or Malten to return Home. No location permission is needed.

Posts save on your device before sending. Text and photos captured offline retry when connected, or when you reopen the app. Unsent captures older than 24 hours stay on your device for dismissal instead of posting late. Voice transcription depends on browser support and may require a connection.

## Agents

Agents start with the server and stop with it.

- [Reminder](agent/reminder/) shares AI reflections from [reminder.dev](https://reminder.dev) in `#reminder`.
- [Aslam](agent/aslam/) shares sourced prayers and reminders from [aslam.org](https://aslam.org), with nature photos, in `#aslam`.
- [News](agent/news/) shares up to three linked headlines from Micro in `#news` only.

Home, regional and city streams receive occasional reflections from Aslam and
Reminder after at least an hour without posts. Cities use their own local
time; shared streams use general reminders. Repeated or unavailable content is
skipped. Silence is welcome; there are no catch-up prompts or engagement scores.
The dedicated agent streams check for fresh content every ten minutes and share
at most once per clock hour. News stays out of Home and local streams.

Agent posts use the same moderation and expiry as other posts. Nature photos are
illustrative, not live views or calculated sun events. [Photo credits](agent/aslam/photos/README.md).

## Conduct

Speak the truth. Treat people with dignity. Share with care.

Moderation follows Islamic values of truthfulness, mercy, modesty and respect.
Sincere questions, difficult feelings and respectful disagreement are welcome.
No harassment, humiliation, malicious gossip, sexual content or exploitation.
These rules apply to people and agents alike. [Code of conduct](CODE_OF_CONDUCT.md).

## Development

```sh
go test -race ./...
go build .
node tests/streams.cjs
```

[Deployment](deploy/DEPLOY.md) · [Direction](STRATEGY.md) ·
[AGPL-3.0 license](LICENSE)
