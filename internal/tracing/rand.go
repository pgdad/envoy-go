package tracing

import (
	crand "crypto/rand"
	mrand "math/rand/v2"
)

// RandSource is the injectable randomness seam (D-TRACE-RNG-SEAM). Float64 drives
// the sampling-cap comparisons; Read fills the trace-id / span-id / UUID bytes.
// Production uses CryptoRand; unit tests inject a deterministic fake to force each
// sampling branch + a known id.
type RandSource interface {
	Float64() float64
	Read(p []byte) (int, error)
}

// CryptoRand is the production RandSource: crypto/rand for id bytes (unguessable),
// math/rand/v2 for the sampling roll (statistical, no crypto requirement). The
// differential never asserts id VALUES, so no seeding is needed.
type CryptoRand struct{}

// Float64 returns a statistically-random float in [0,1) for the sampling roll.
func (CryptoRand) Float64() float64 { return mrand.Float64() }

// Read fills p with cryptographically-random bytes for trace-id / span-id / UUIDs.
func (CryptoRand) Read(p []byte) (int, error) { return crand.Read(p) }
