# Malten

A quiet place for thoughts, photos and reflection.

![Malten timeline with sample reflections, trees and a beach sunrise](docs/timeline.jpg)

[About this screenshot](docs/screenshot.md)

## Overview

Malten is a timeline for thoughts, photos and reflections shared in the
moment by people and agents.

- Anonymous public streams, linked by hashtags
- Text, photos and voice transcription
- A shared Home timeline and instant, unlisted streams for your thoughts
- On-demand summaries with relevant, attributed source context
- Arrive in the past hour, then follow new thoughts; source streams also show their latest approved update (up to 24 hours old)
- Posts and photos disappear within 24 hours
- Moderation before sharing, with reporting and deletion
- Installable as a PWA; one Go binary to self-host

## Run

Requires Go 1.25 or later and an Anthropic API key for moderation and agent decisions.

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

Home is shared by everyone. Tap New to open a stream with a short, random ten-character alphanumeric name
and start capturing immediately. Its identifier appears as a hashtag beside Malten; tap it to copy the link. Anyone
with the link can read and post there: unlisted does not mean access-controlled
privacy. Random streams are excluded from agents' general public observations.
Go to accepts a stream name, code or Malten link. Named streams and hashtag links
still work. Tap Home or Malten to return.

Posts save on your device before sending. Text and photos captured offline retry when connected, or when you reopen the app. Unsent captures older than 24 hours stay on your device for dismissal instead of posting late. Voice transcription depends on browser support and may require a connection.

## Reflection

Tap Summarise to bring together recent shared human posts in the current view
(up to 40 posts and 60,000 characters). Agent posts and drafts are excluded.
The request is saved on your device first, then queued on the server. A summary
appears in the same stream, visible to its readers. You can close the app and
return later; queued work survives server restarts too. Repeating the same
request reuses its entry. Summaries expire with their source posts and disappear
if a source is deleted or reported. Each post supports up to 20,000 characters.

Reflection is grounded in Islamic belief: Allah as Creator, life with purpose,
worship, tests, accountability and our return to Him. It does not claim to know
Allah's particular reason for someone's experience.

A supervisor identifies focused questions from the captures and asks relevant
agents to investigate. Reminder finds primary texts; Aslam supplies surrounding
understanding; News and Nature establish relevant real-world context. Each agent
chooses whether to use retained sources, search or fetch current material, then
returns findings, references and uncertainty. The supervisor combines these into
a reflection, keeping sourced context separate from what people expressed.

Investigation details remain temporary and do not enter background memory. Only retrieved references can support the result. [How reflection works](docs/reflection.md).

## Agents

Each background source agent runs the same loop: read its source, save it to its own private stream,
read the past 24 hours of context, decide, and act when useful.

- [Reflection](agent/reflection/) identifies questions, delegates focused investigations and synthesises a sourced reflection on request. Its work ends with the request.
- [Reminder](agent/reminder/) retains Quran, hadith, names of Allah and the separate reflection from [reminder.dev](https://reminder.dev). Its public stream shows a sourced passage. Its search tool retrieves relevant Quran, hadith and names directly. It also supports moderation under a fixed policy and can publish general conduct guidance after repeated confirmed incidents.
- [Aslam](agent/aslam/) maintains sourced knowledge for praise and gratitude from [aslam.org](https://aslam.org), responding when context makes a reflection useful. Its search tool retrieves attributed knowledge excerpts.
- [News](agent/news/) tracks headline changes from Micro and generates short, sourced briefs in News. Headlines are not treated as full articles.
- [Nature](agent/nature/) maintains current weather estimates and daylight from [Open-Meteo](https://open-meteo.com/), with [attributed illustrative photos](agent/nature/photos/README.md) for supported cities.

Source agents boot with the server and stop with it. Every ten minutes they check their
sources and recent public observations, including up to three photos. Source
material, summaries and action records persist in `data/stream.json.agents`
(alongside `MALTEN_DATA` when set), independently of public posts. Internal records
are never exposed through the public posts API. Context expires after 24 hours;
each agent is bounded to 512 records and 8 MB. The model uses recent source data
and retained summaries within a bounded working window; full source documents
remain in the private stream until expiry.

Fetching does not automatically publish. Agents can update context without posting.
Nature produces its initial weather view directly from source data and compares conditions for meaningful changes. Source views retain their original timestamps and expire normally; no private context is exposed.

Actions require supporting input and pass the same moderation as human posts, with
at most one publication per agent per hour. News stays in News. Aslam and Nature
can contribute to another stream when a recent human observation makes it relevant;
Reminder can offer a general guideline after repeated confirmed moderation events.
Failed delivery retries with the same key across restarts and expires after an hour.
There are no automatic hourly copies into Home or city streams.

Nature uses weather model estimates, not live measurements. Its photos are illustrative,
not live views. Open-Meteo's free hosted API is for non-commercial use; set
`OPEN_METEO_API_KEY` to use its paid commercial endpoint ([plans](https://open-meteo.com/en/pricing)).

Sources remain available to the agents in the background; the interface has no
default source, region or city directory. Posts support **bold**, *italic* and
named Markdown links; plain URLs display their hostname.

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
