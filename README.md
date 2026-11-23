# Malten

A network for reflection

## Overview

Malten is a network for daily reflection. Its made up of streams of text and nothing more. 
By default, messages expire 24 hours after posting and idle streams expire after 24 hours. 
Each stream holds up to 1,024 messages of 4,096 UTF-8 bytes each (4 MiB of message text per stream).

Streams can be discovered through exploration or via the API.

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

Mention `@malten` at the start of a message, for example `@malten what is this?`
or `@malten what does today’s passage mean?`. The original `/malten` shortcut also works. Mentioning Malten sends Mu a bounded excerpt of up to six preceding reflections
from that stream in Mu’s per-request `context.messages`. The visitor’s question
is sent as `prompt`. Mu saves the question and answer in the thread; an answer may quote an excerpt. Everyone in the stream sees the question and the reply from Malten. Both appear
in stream history on refresh. The stream saves its Mu thread with the existing
recovery snapshot, so follow-ups can continue after a restart. The local link
expires after the stream TTL or 24 hours of agent inactivity, whichever is shorter.

To configure the hosted agent, open **Agents → Malten → Edit** in Mu. Give it
instructions for a shared reflection stream and select only the services suitable
for public replies. A starting instruction:

> You are Malten, the guide to a network for reflection. Welcome newcomers,
> help people reflect on the daily Qur’an passage when asked, and keep your
> replies brief and grounded in the source. Everyone in the stream can read
> what you say.

## MCP

Connect an agent to `https://malten.me/mcp` using Streamable HTTP to list public streams, read or post reflections, and create streams.

Select **Translate** under a message to read it in your browser’s preferred
language. Malten sends the selected text to Mu using the server’s `MU_TOKEN`.
The translation appears only for the reader who requested it.
