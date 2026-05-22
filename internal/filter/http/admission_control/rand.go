package admission_control

// rand.go — in-package Rand interface seam per phase-23 SPEC §3.1 + AMEND-2
// + ADR-0194 §Decision (inline NOT framework primitive). Production wiring uses
// defaultRand (math/rand/v2 package-level Uint64). Tests use the test-scope-only
// fakeRand at rand_test.go (deterministic injected uint64).
//
// # Why inline (NOT a framework primitive)
//
// Per SPEC §3.1 + SPEC §2.5 + the phase-17 jwt_authn EXTRACT-NOW-only-when-
// trigger-fires lesson: consumer count at introduction time is 1 (the in-package
// controller only). The seam stays inline at
// internal/filter/http/admission_control/rand.go. No shared internal/rand/
// framework primitive is introduced.
//
// # Uint64, NOT Float64 (AMEND-2)
//
// The seam exposes Uint64() uint64, mirroring upstream Envoy's
// Random::RandomGenerator::random() which returns a uint64. The reject decision
// is the integer-modulo comparison
//
//	(accuracy * math.Max(P, 0.0)) > float64(r % accuracy)   with accuracy=10000
//
// per admission_control.cc:175-178. This is NOT a float comparison random() < P
// (the BRAINSTORM hypothesis refuted by AMEND-2). The integer-modulo decision
// lets the unit tests pin the exact admit/reject boundary deterministically via
// an injected fakeRand.
//
// # Surface
//
//   - Rand interface (Uint64() uint64).
//   - defaultRand production wiring wrapping math/rand/v2's package-level Uint64.
//
// The fakeRand test-scope implementation lives at rand_test.go in the same
// package; it is NOT compiled into the production binary (test-file-only).

import "math/rand/v2"

// Rand is the controller's seam over the rejection dice-roll. It mirrors
// upstream Envoy's Random::RandomGenerator::random() (uint64 per AMEND-2).
// The in-package interface lets the unit tests inject a deterministic draw to
// pin the exact reject/admit boundary without depending on a framework-wide
// primitive.
//
// Inline NOT framework primitive per SPEC §3.1 + SPEC §2.5 (consumer count 1
// at introduction time; phase-17 jwt_authn EXTRACT-NOW-only-when-trigger-fires
// lesson). When a second Rand-consumer filter materializes, evaluate extraction
// to an internal/rand/ framework primitive.
type Rand interface {
	// Uint64 returns a pseudo-random uint64. Used by the controller's
	// shouldReject method to compute the integer-modulo reject decision
	// (accuracy*P) > (r % accuracy) with accuracy=10000 per AMEND-2.
	Uint64() uint64
}

// defaultRand wraps math/rand/v2's package-level Uint64 for production wiring.
// The zero-value is usable (no state). Consumed at filter construction where
// the factory wires defaultRand{} as the Rand arg to the controller in the
// production HCM-build path.
type defaultRand struct{}

func (defaultRand) Uint64() uint64 { return rand.Uint64() }
