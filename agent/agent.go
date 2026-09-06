// Package agent contains server-owned loops with specific objectives.
package agent

// Stream describes discovery and an optional local-hour window for a feed seed.
// End may exceed 24 for a window crossing midnight. Zero hours means link only.
type Stream struct {
	Tag        string
	Start, End int
}
