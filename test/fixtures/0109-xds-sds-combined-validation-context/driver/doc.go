// Package driver implements the differential fixture driver for
// 0109-xds-sds-combined-validation-context — the phase-66 proof that under a
// downstream combined_validation_context (CVC) the SDS-delivered CA REPLACES
// the inline default_validation_context.trusted_ca; it does NOT union with it.
//
// This is a disciplined clone of 0108-xds-sds-validation-context. Same
// observable shape (a normalized two-arm accept/reject verdict), same in-memory
// PKI machinery, same per-side SDS receivers. The one semantic delta: the CA
// that 0108 NEVER delivered (CA_unserved) is here CONFIGURED as the inline
// default_validation_context.trusted_ca (CA_Y) — a real competitor. The proof
// is that a client chaining to CA_Y (the inline default) is still REJECTED,
// while a client chaining to CA_X (the SDS-served CA) is ACCEPTED: the served
// pool wins outright, refuting the pool-UNION design (Design C), which would
// accept BOTH.
package driver
