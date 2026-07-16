// Package driver implements the differential fixture driver for
// 0108-xds-sds-validation-context — the phase-65 proof that an SDS-delivered
// CertificateValidationContext is the ACTUAL downstream mTLS trust anchor.
//
// The observable is a normalized two-arm accept/reject verdict (PLAN-65 D4):
// a client cert chaining to the SDS-SERVED CA must be ACCEPTED, and one
// chaining to an UN-SERVED CA must be REJECTED — on both sides, byte-identically.
// The contrast is the proof; agreement alone is not (see structuralCheck).
package driver
