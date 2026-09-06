// Package agent contains server-owned loops with specific objectives.
package agent

import "context"

// Stream describes an agent's public stream for discovery.
type Stream struct {
	Tag string
}

// Publish and PublishPhoto send agent posts through the server's moderation path.
type Publish func(context.Context, string, string, string, ...string) error
type PublishPhoto func(context.Context, string, string, string, string, ...string) error
