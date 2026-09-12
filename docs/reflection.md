# Reflection

Malten helps people reflect on their experiences in light of Islamic truth.
Reminder provides access to primary texts. Aslam supports further understanding.
News and Nature provide relevant context about the world.

The supervisor's standing foundation recognises Allah as Creator, creation with
purpose, worship, life's tests, accountability and our return to Him. It also
requires humility: the system cannot know Allah's specific reason for an event
in someone's life. Difficult feelings remain valid subjects for reflection.

## One request, three stages

1. **Understand.** Summarise sends the IDs of up to 40 recent approved human posts (60,000 characters total)
   in the reader's current view. The server resolves them within that stream.
   The supervisor reads the text and up to three most recent photos, produces a faithful
   account of what was expressed, and identifies focused questions. Each question
   must refer to captures that prompted it. The stream address is omitted from
   model input.
2. **Investigate.** Up to four agents work concurrently, each on one question.
   They receive the generalised question rather than the full stream. Each
   chooses its own retrieval path and returns a short finding, supporting source
   references and uncertainty. A question about primary Islamic sources always
   goes to Reminder. The other agents are included only when relevant.
3. **Reflect.** The supervisor receives the findings, their supporting texts, the initial account
   and the same photos. Photo-only captures are described from visible details;
   an absent caption does not make them empty.
   It distinguishes original evidence from the investigators' interpretations,
   then produces a compact summary with up to two separately attributed
   reflections. Only source IDs actually retrieved and cited by investigators
   are accepted in the final result.

| Agent | Question it investigates | Available tools |
| --- | --- | --- |
| Reminder | What do the Quran, hadith and names of Allah establish here? | Retained primary texts, indexed search, latest source fetch |
| Aslam | What further Islamic explanation helps understanding? | Retained knowledge, indexed search, latest source fetch |
| News | What do the available headlines establish about this event? | Retained headlines, fresh headlines, Mu news search when configured |
| Nature | What are the relevant weather and daylight conditions? | Retained conditions, fresh conditions for supported cities, Mu place lookup and forecast when configured |

Reminder search disables the API's generated answer and retrieves texts directly.
Aslam search supplies attributed excerpts, which may not settle a question.
News cannot infer article details from headlines. Nature cannot guess the user's
location or extrapolate beyond supported places and times. An agent may report
that its sources do not answer the question.

## Context and boundaries

Mu integration and managed-agent constraints are documented in [micro.md](micro.md).
Only live server captures enter summary jobs; locally remembered expired posts are
never uploaded automatically for generation.

The existing source agents still run every ten minutes and retain 24 hours of
source data. On-demand investigations can read their source records, but not
moderation events, other people's observations or previous generated answers.
Search queries, questions, findings and final reflections are not saved into
those background streams. Fresh request-time retrieval is also temporary.

The supervisor has no search tools. Each investigator has only its own read-only
tools, with at most two retrieval rounds and six calls. Planning has a 20-second
budget, concurrent investigations 50 seconds each, and synthesis 30 seconds.
Two summary jobs can run simultaneously, with at most 20 pending entries. Their
rate limit is separate from posting; retries reuse the same saved entry before
rate limiting. The existing server lifecycle runs the jobs, with a 150-second
budget including moderation. No extra process or service is required.

Failures from individual investigators are isolated. The supervisor can use the
remaining findings while retaining uncertainty. If none provide evidence, only
the faithful account of what was expressed is returned, without added source
context. Model-generated citations are checked against retrieved IDs; this checks
provenance, not whether an interpretation is correct.

The browser saves summary requests in its existing offline outbox. The server
persists a pending entry before returning HTTP 202; generation does not depend
on the HTTP connection. Pending entries resume after server restart. Completion
replaces the pending entry after moderation, and failures are recorded there.
Readers returning after more than an hour can still see summary entries within
their lifetime. Results are shared in the selected stream, not across streams.
Deleted, reported or expired source posts invalidate their summaries. Drafts
never enter generation. On-demand findings still do not enter background memory.

## Code

- `agent/reflection/reflection.go`: routing and synthesis, with validated plans.
- `agent/research.go`: the shared investigation loop and Islamic foundation.
- `agent/{reminder,aslam,micro,nature}/research.go`: focused objectives and tools.
- `server/summary.go`: stream isolation, request limits and expiry checks.

Summaries speak directly about ideas and visible scenes, without narrating what
"the user" supplied or addressing the reader.

The interface remains a stream with one Summarise action. The final result goes into the stream; internal investigation steps do not.
