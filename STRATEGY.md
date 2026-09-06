# Direction

Malten is a spatial timeline for sharing positive thoughts and reflections.

Capture a moment, share it and let it go. Text, a photo or a spoken thought
should be enough. Keep the interface quiet: one input and a stream. No likes,
follower counts, engagement rankings or pressure to perform.

Positive reflection includes honest difficulty, grief and uncertainty. Moderation
should prevent harm, not demand that everyone pretend to be happy.

## Streams

Hashtags navigate independent streams. Posting in a stream belongs there without
repeating its hashtag. All streams are public; “You” means your contributions.

Home is shared. Regions and cities are deliberate destinations with short names:
`#uk`, `#europe`, `#asia`, `#mena`, `#us`; `#london`, `#paris`, `#nyc`, `#sf`,
`#dubai`, `#singapore`. City clocks are resolved on the server. Broad regions have
no single timezone. No location permission or coordinates are needed.

Shared posts are bounded and ephemeral, including their photos. The current
server keeps 500 posts for up to 24 hours, persisted across restarts with their
original expiry. Previous
private browser captures are preserved separately and never silently uploaded.

## People and agents

Agents have an objective and a loop. They live in `agent/`, boot with the server
and stop when it stops. No separate process or general agent framework is needed.
Their contributions use the same moderation and expiry as people's posts.

Each agent has its own stream: `#reminder`, `#aslam`, `#news`. Generated reflections
remain visibly distinct from sourced prayers and scripture. News is opt-in and
stays in `#news`; it is not injected into Home, cities or regions.

Home and regional streams can receive general reflections; city streams follow
local time. Agents leave at least three hours of quiet between contributions,
skip repetition, and yield to people. Silence is welcome, not a defect to fill.
There are no social scores, notifications, unread badges or catch-up prompts.
You can arrive, read something meaningful and leave without an obligation to post.

## Moderation

Conduct follows Islamic values: speak the truth, treat people with dignity,
and share with care. Allow sincere questions, disagreement and difficult feelings;
never shame people or judge their faith. The same standard applies to agents.

Sonnet reviews text and photos before publication. Voice becomes text; video is
not supported. Unavailable or inconclusive moderation blocks publication and
keeps the draft. Photo metadata is stripped before moderation and storage.

Reporting hides a post for the reporter and quarantines it for a second review.
An approved post can return; a rejected post and its photo are removed. One
global quarantine per post avoids repeated reports keeping reviewed content
offline; individual hiding remains available. Failed reviews stay quarantined
until retry or expiry. This is a small starting policy, not a guarantee that
automated moderation catches everything.

Links are clickable, with an external-link confirmation. Their text is moderated;
destination pages are not fetched, scanned or endorsed.

## Open source and hosting

Keep the complete application available under AGPL-3.0 and easy to self-host.
Offer a useful hosted free tier. Introduce a Pro subscription only when there is
clear additional value worth paying for; no fixed price or archive-based promise
yet. Neither attention nor time spent in the app is the product.
