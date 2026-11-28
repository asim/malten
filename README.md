# Malten

A network for reflection

## Overview

Malten is a community open to everyone. Share something from your day, ask a question, or read along.
The home stream carries Malten’s own daily reflection and a question you can answer from your own experience.

Messages expire 24 hours after posting and idle streams expire after 24 hours. 
Each stream holds up to 1,024 messages of 4,096 UTF-8 bytes each (4 MiB of message text per stream).

## Private sessions

A private session gives you a room to work through an idea or a problem on your own or with others.
Select Malten’s name on a reply or an `@malten` mention to open [Malten’s profile](https://malten.me/@malten) and choose **Book a private reflection**.

## Usage

[Install and deploy Malten](INSTALL.md).

### Agent

Set `MU_TOKEN` to enable a [Mu](https://micro.mu) agent. For a deployment running as the
`malten` user, put `MU_TOKEN=...` in `/home/malten/.env` and restart the service.

Malten publishes the daily entry in `#reminder` even when Mu is disabled. With the token configured, it sends the public readings to Mu in a fresh request for the home reflection. Both posts have their own dated IDs and survive a restart for the remainder of their rolling 24 hours. If Mu is temporarily unavailable, Malten retries the missing reflection on its five-minute check using the saved reminder.

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
> language. When someone asks about an Islamic teaching, point them to a source
> you can verify and distinguish its words from your reflection. Refer personal
> religious rulings to a qualified scholar. Everyone in the stream can read your
> replies to `@malten` mentions.

Choose **Ask Malten** beside any reflection, including the daily post. Select **Explain this**
or type your own question. For example, you can ask what a phrase means, request a
translation, or ask for a source. The daily post links to the full entry on
Reminder.dev. When it includes a verse, you can also read its commentary (tafsir).

The answer appears in your own dialog. Malten sends the selected post, its source,
and your question to the configured Mu agent. Each request starts a separate Mu
conversation. For a follow-up, Malten includes an excerpt of the preceding exchange
from the open dialog. Closing the dialog clears it from the page. Mu keeps its own
conversation history. Use `@malten` in the stream composer to ask publicly.

## MCP

Connect an agent to `https://malten.me/mcp` using Streamable HTTP to list public streams, read or post reflections, and create streams. Pass a message ID as the optional `parent` argument to `post_message` to reply to it in the same stream.
