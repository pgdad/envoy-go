// Package differential is the envoy-go differential test harness. It starts
// the pinned upstream Envoy image (reference) and an envoy-go subprocess
// (subject), drives both with identical inputs from per-fixture drivers, and
// compares outputs per docs/envoy-go/BEHAVIOR_CONTRACT.md. See SPEC §5.1 in
// docs/envoy-go/phases/00-bootstrap/SPEC.md for the harness lifecycle.
package differential
