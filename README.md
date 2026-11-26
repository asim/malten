# Malten

A network for reflection

## Overview

Malten is rooted in Islamic values and open to everyone. Share something from your day, ask a question, or read along.
The home stream carries a short reflection from Reminder.dev. Open the Qur’an passage if you’d like to read further.

By default, messages expire 24 hours after posting and idle streams expire after 24 hours. 
Each stream holds up to 1,024 messages of 4,096 UTF-8 bytes each (4 MiB of message text per stream).

Choose **Reply** on a reflection to quote it in the same stream or private session. Select the quote to open the original. The excerpt is available for as long as the original remains in the conversation.

Streams can be discovered through exploration or via the API.

## Private sessions

Select Malten’s name on a reply or an `@malten` mention to open [Malten’s profile](https://malten.me/@malten) and choose **Book a private session**.
You can also book from **Join** or visit [the booking page](https://malten.me/session).
Choose the number of places, including yours, to see the price and duration before paying through Mu.
Once payment is confirmed, send one invite to each guest. Guests can join the waiting room before you press **Start session**.
Everyone shares one end time and can post reflections or ask Malten. Return in the same browser to keep your place.
Clearing its cookies loses access to the booking. The private conversation expires when the session ends.

## Features

- [x] Presence - User presence status
- [ ] Secure Pipes - Stream between two users
- [x] Speech to text - Talk to malten and write speech
- [ ] Decentralisation - Interconnect multiple Malten servers
- [x] EventSource support - Real time messaging directly to the browser
- [ ] Client side encryption - AES-256 on the user side so we never see the data
- [x] Configurable stream TTL - Per stream configuration to increase or decrease for DMs or Notes

## Usage

[Install and deploy Malten](INSTALL.md).

### Agent

Set `MU_TOKEN` to enable a [Mu](https://micro.mu) agent. For a deployment running as the
`malten` user, put `MU_TOKEN=...` in `/home/malten/.env` and restart the service.

Mention `@malten` anywhere in a message, for example `@malten what is this?`
or `What does today’s passage mean, @malten?`. The original `/malten` shortcut
also works at the start. Mentioning Malten sends Mu a bounded excerpt of up to six preceding reflections
from that stream in Mu’s per-request `context.messages`. The visitor’s question
is sent as `prompt`. If you use **Reply** and mention `@malten`, Mu also receives the reflection you selected. Mu saves the question and answer in the thread. Everyone in the stream sees Malten’s answer with a linked quote of the question. Both appear
in stream history on refresh. The stream saves its Mu thread with the existing
recovery snapshot, so follow-ups can continue after a restart. The local link
expires after the stream TTL or 24 hours of agent inactivity, whichever is shorter.

To configure the hosted agent, open **Agents → Malten → Edit** in Mu. Give it
instructions for a shared reflection stream and select only the services suitable
for public replies. A starting instruction:

> You are Malten, the AI in a network for reflection rooted in Islamic values.
> Start with what the person shared and follow their lead on faith. Use everyday
> language. When someone asks about an Islamic teaching, explain unfamiliar terms
> and name the source. Everyone in the stream can read your replies to `@malten` mentions.

Choose **Ask Malten** beside a reflection or the Daily Reminder. Select **Explain this**
or type your own question. For example, you can ask what a phrase means, request a
translation, or ask for a source. The Daily Reminder links to the full entry on
Reminder.dev. When it includes a verse, you can also read its commentary (tafsir).

The answer appears in your own dialog. Malten sends the selected post, its source,
and your question to the configured Mu agent. Each request starts a separate Mu
conversation. For a follow-up, Malten includes an excerpt of the preceding exchange
from the open dialog. Closing the dialog clears it from the page. Mu keeps its own
conversation history. Use `@malten` in the stream composer to ask publicly.

## MCP

Connect an agent to `https://malten.me/mcp` using Streamable HTTP to list public streams, read or post reflections, and create streams. Pass a message ID as the optional `parent` argument to `post_message` to reply to it in the same stream.
