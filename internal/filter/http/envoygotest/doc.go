// Package envoygotest implements the test-only `envoy.filters.http.envoy_go_test`
// HTTP filter — Phase 07.1 Task 19 / SPEC §4.1 + §7.3.
//
// # Purpose
//
// envoygotest is a structural-test probe filter that exercises every
// iteration-state shape the HTTP filter framework must support: pure
// pass-through, async-resume, body-buffering, decode-side SendLocalReply,
// encode-side header/body mutation, and trailers stop. It is NOT a real
// production filter: the entire configuration surface is gated on the
// per-request `x-envoy-go-test-mode` header and the per-route `count` config.
//
// The filter is registered at boot under the type URL
// `type.googleapis.com/envoy.filters.http.envoy_go_test.v0.EnvoyGoTest`
// (Task 20 wires the boot registration). Per-route config is
// `EnvoyGoTestPerRoute{count: <int32>}` carried via `typed_per_filter_config`.
// At Task 22 a structural fixture (`0007b-iteration-probe`) drives the eight
// modes through the full HCM dispatch path; the unit tests in `filter_test.go`
// exercise each mode in isolation through the chain framework directly.
//
// # Mode dispatch
//
// Eight modes are recognized on the per-request `x-envoy-go-test-mode`
// header (case-sensitive, exact match against the §7.3 enumeration). When the
// header is absent, the filter-level `mode_default` field from the unmarshaled
// `EnvoyGoTest` config supplies the mode. Unknown modes fall through to
// `continue` (pure pass-through) — defensive default for debugging fixtures
// that mistype the header value.
//
// The dispatch is an explicit `switch` per Decision §3.7 (no map): the Go
// compiler emits a jump table, the branch predictor handles the cold path,
// and the test author has a single anchor point for adding modes. The eight
// modes are:
//
//  1. continue                  — pure pass-through. DecodeHeaders/Data/
//                                Trailers and EncodeHeaders/Data/Trailers
//                                all return Continue.
//  2. stop-and-resume-headers   — DecodeHeaders returns StopIteration; a
//                                spawned goroutine calls dcb.ContinueDecoding
//                                after a 10ms sleep. Exercises the framework's
//                                async-resume channel + parkDecode loop.
//  3. stop-and-buffer-data      — DecodeData returns
//                                DataStopIterationAndBuffer; the chain's
//                                buffer-cap accumulator + park machinery
//                                exercise. A spawned goroutine resumes via
//                                dcb.ContinueDecoding after a 10ms sleep.
//  4. local-reply-decode        — DecodeHeaders calls
//                                dcb.SendLocalReply(418, "i am a teapot\n",
//                                nil) and returns StopIteration. Exercises
//                                the SendLocalReply path through the encode
//                                chain.
//  5. local-reply-decode-data   — DecodeData calls dcb.SendLocalReply with
//                                the same shape (418 / teapot body) and
//                                returns DataStopIterationNoBuffer.
//  6. modify-encode-headers     — DecodeHeaders returns Continue;
//                                EncodeHeaders mutates via headers.Set,
//                                adding `x-envoy-go-test-encoded: yes`.
//  7. modify-encode-data        — DecodeHeaders returns Continue; EncodeData
//                                replaces the body bytes with a probe-
//                                specific marker (`MODIFIED\n`).
//  8. stop-trailers             — DecodeTrailers returns
//                                TrailersStopIteration; a spawned goroutine
//                                resumes via dcb.ContinueDecoding after a
//                                10ms sleep.
//
// The 10ms sleep on the async-resume modes (2, 3, 8) is intentionally
// realistic: the framework's parkDecode select must hold the dispatch
// goroutine through a real wall-clock delay before the resume signal arrives,
// not just a synchronous send-then-receive race. Tests assert the resume
// fires within a generous timeout (250ms) to avoid CI flakes.
//
// # Per-route count echo
//
// On EncodeHeaders, the filter looks up its per-route config via
// `ecb.RequestRouteConfig()` — a `proto.Message` of type
// `*envoygotestpb.EnvoyGoTestPerRoute`. If a non-zero `count` value is
// configured, the filter sets `x-envoy-go-test-route-count: <count>` on the
// encode-side headers. This exercises the per-route config plumbing on the
// encode chain (decode-side per-route is exercised by the cors filter at
// Task 18).
//
// # Iteration-protocol coverage
//
// Each unit test in `filter_test.go` builds a minimal chain (envoygotest
// filter + a recording terminal) and asserts the per-mode behavior in
// isolation. Coverage matrix:
//
//   | Mode                       | Decode     | Encode    | Async-resume |
//   |----------------------------|------------|-----------|--------------|
//   | continue                   | Continue   | Continue  | no           |
//   | stop-and-resume-headers    | Stop+resume| Continue  | yes (10ms)   |
//   | stop-and-buffer-data       | Stop+buf   | Continue  | yes (10ms)   |
//   | local-reply-decode         | LocalReply | n/a       | no           |
//   | local-reply-decode-data    | LocalReply | n/a       | no           |
//   | modify-encode-headers      | Continue   | Mutate    | no           |
//   | modify-encode-data         | Continue   | Mutate    | no           |
//   | stop-trailers              | Stop+resume| Continue  | yes (10ms)   |
//
// At Task 22 the structural fixture wires all eight modes through HCM
// dispatch end-to-end and asserts the wire output via `0007b-iteration-probe`.
//
// # Safety: production-deploy guard
//
// envoygotest is intended for tests only. There is no production-time disable
// hook; the safeguard is operational (do not register the filter at boot in
// production) and convention (the type URL carries the `envoy_go_test` slug).
// Test fixtures pin the registration to the test-only HTTPRegistry; Task 20's
// `cmd/envoy-go/main.go` boot wiring registers it alongside cors + router in
// the default registry — this is a Phase 07.1 acceptable simplification.
// Future hardening (gate on a `--enable-test-filters` flag) is left for
// Phase 11+ when production-grade defenses become a roadmap concern.
package envoygotest
