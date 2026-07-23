// Package id generates short, unguessable, collision-resistant identifiers for
// sessions, tickets and tool calls.
//
// IDs are random rather than sequential on purpose: they must not collide with
// rows already in the persisted SQLite database after a process restart (a
// monotonic counter resets to zero on restart and would reuse ids that already
// exist). 64 bits of randomness makes a collision negligible.
package id

import (
	"crypto/rand"
	"encoding/base32"
)

// lowercase, digit-safe base32 alphabet without padding — readable in URLs/logs.
var enc = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)

// New returns a new id with the given prefix, e.g. New("SESS") -> "SESS-k7q2m9f3xa4t8".
func New(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand.Read does not fail on a healthy system; if the OS RNG is
		// unavailable the process cannot safely mint identifiers.
		panic("id: reading random source: " + err.Error())
	}
	return prefix + "-" + enc.EncodeToString(b[:])
}
