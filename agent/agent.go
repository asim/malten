// Package agent contains server-owned loops with specific objectives.
package agent

import "context"

// Stream describes discovery and an optional local-hour window for a feed seed.
// End may exceed 24 for a window crossing midnight. Zero hours means link only.
type Stream struct {
	Tag        string
	Start, End int
}

// Publish and PublishPhoto send agent posts through the server's moderation path.
type Publish func(context.Context, string, string, string, ...string) error
type PublishPhoto func(context.Context, string, string, string, string, ...string) error
