// Package id generates short, human-readable, process-unique identifiers for
// sessions, tickets and tool calls. Ids are monotonic within a process which
// keeps test output stable and readable.
package id

import (
	"fmt"
	"sync/atomic"
)

var counter uint64

// New returns a new id with the given prefix, e.g. New("TCK") -> "TCK-000042".
func New(prefix string) string {
	n := atomic.AddUint64(&counter, 1)
	return fmt.Sprintf("%s-%06d", prefix, n)
}
