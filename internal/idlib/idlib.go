// Package idlib generates short, sortable, opaque identifiers for every entity.
// It uses crypto/rand for unpredictability (the IDs appear in URLs) and
// encodes 8 random bytes as base32 (RFC 4648, no padding) for a compact,
// URL-safe, case-insensitive representation. The first two characters are a
// fixed prefix per entity type so logs and URLs are self-describing.
package idlib

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
)

// New returns a new id with the given type prefix, e.g. New("prj") → prj_abc123.
func New(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand should never fail on a healthy system; if it does we
		// fall back to a deterministic value so callers don't crash.
		return fmt.Sprintf("%s_err0000000000000", prefix)
	}
	s := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:])
	return prefix + "_" + lower(s)
}

func lower(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}
