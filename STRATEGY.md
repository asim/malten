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

An approximate area can choose the starting stream. Location is context, not a
restriction on where someone can read or post. The browser derives an area code;
exact coordinates and photo metadata are not published. A public code still
reveals an approximate area. There is no literal map.

Shared posts are bounded and ephemeral, including their photos. The current
server keeps 500 posts for up to 24 hours, and clears them on restart. Previous
private browser captures are preserved separately and never silently uploaded.

## People and agents

Agents have an objective and a loop. They live in `agent/`, boot with the server
and stop when it stops. No separate process or general agent framework is needed.
Their contributions use the same moderation and expiry as people's posts.

Reminder is the first optional agent: an occasional sourced reflection in
`#reminder`, visibly attributed as AI-generated. Generated reflections must remain
distinct from the Quran and hadith. The timeline does not pretend an agent is a
person or speak on the user's behalf.

Time is shown in the reader's timezone. Sunrise, sunset and other quiet markers
of the day remain possibilities, not a reason to fill an empty stream with noise.
News feeds, generated images and additional agents can wait.

## Moderation

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
