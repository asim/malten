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

Send `/malten What needs my attention?` in a stream to ask Mu. Everyone in the stream sees the question and the reply from Malten. Both appear
in stream history on refresh. The stream saves its Mu thread with the existing
recovery snapshot, so follow-ups can continue after a restart. The local link
expires after the stream TTL or 24 hours of agent inactivity, whichever is shorter.

## MCP

Connect an agent to `https://malten.me/mcp` using Streamable HTTP to list public streams, read or post reflections, and create streams.
