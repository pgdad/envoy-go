// Package fault implements the envoy.filters.http.fault HTTP filter.
//
// Phase 09: real Envoy filter, wire-shape pinned by SPEC §11.1–§11.8 empirical
// scrapes of reference Envoy v1.37.2.
//
// Decode side (per SPEC §6.4):
//
//  1. Per-route 3-tier merge resolves the listener-level OR per-route
//     *runtimeConfig (wholesale-override per ADR-0073 + §11.7).
//  2. Headers-field exact-match gate: if non-empty, ALL listed (name, value)
//     pairs must match; header NAME match is case-insensitive (per HTTP/1.1
//     RFC 7230); header VALUE match is case-sensitive byte-equality
//     (StringMatcher.exact only — non-exact matchers silent-ignored).
//  3. Percentage rolls: delay + abort independently; 0% short-circuits to
//     false; 100% short-circuits to true; intermediate values consult the
//     per-instance *math/rand.Rand seeded by time.Now().UnixNano().
//  4. max_active_faults check: if > 0 AND *atomic.Int64 active >= cap,
//     increment fault.faults_overflow and SKIP (no fault injected; return
//     Continue).
//  5. Fault path: fire delay timer (delay-only or combined) OR fire abort
//     SendLocalReply (abort-only); return StopIteration.
//
// Async-resume mechanics (per ADR-0102): the delay path uses time.AfterFunc
// to schedule a callback that calls cb.ContinueDecoding() (delay-only) OR
// cb.SendLocalReply() (combined delay+abort) from the timer goroutine. The
// chain parks at StopIteration and resumes from the timer goroutine. OnDestroy
// calls f.delayTimer.Stop() to cancel the timer on request teardown.
//
// Abort terminal-replace (per ADR-0103): the abort path calls
// cb.SendLocalReply(http_status, "fault filter abort", OrderedHeaders{
// {Name: "Content-Type", Value: "text/plain"}}) and returns StopIteration.
// Body is byte-exact "fault filter abort" (18 bytes, NO trailing newline).
// The OrderedHeaders carrier overrides the chain's default content-type
// charset modifier; the framework appends date + server + content-length.
//
// max_active_faults concurrency cap (per ADR-0105): a closure-captured
// *atomic.Int64 counter (LBP-1 sixth application) is shared across all
// per-instance *filter values from the same factory. Hot path is lock-free.
// The markedActive per-instance bool is a sync.Once-equivalent guard ensuring
// exactly-one Inc/Dec balance under the OnDestroy-races-timer-callback case;
// race-clean by the single-goroutine-per-stream invariant per ADR-0071.
//
// Per-route policy: resolved via DecoderFilterCallbacks.RequestRouteConfig()
// which returns the merged *faultv3.HTTPFault from the perRouteConfig 3-tier
// merge (Route > VirtualHost > RouteConfiguration; ADR-0073). When non-nil,
// the per-route config WHOLESALE-replaces the listener-level config — a
// per-route HTTPFault that omits delay does NOT inherit the listener-level
// delay (empirically confirmed at SPEC §11.7).
//
// Encode side: no-op pass-through. Fault operates exclusively on the decode-
// headers phase.
//
// Stats (per ADR-0107): 5 stats registered at HCM-build time on the
// *stats.Registry from FactoryCtx — 4 counters (aborts_injected,
// delays_injected, faults_overflow, response_rl_injected — last permanently
// zero per route A) + 1 gauge (active_faults).
//
// Deferrals (per ADR-0104 + SPEC §2): header-driven fault path
// (x-envoy-fault-{delay,abort}-request[-percentage]) is silently ignored;
// coupled to delay.header_delay / abort.header_abort proto sub-messages
// (deferred together per §11.5 empirical pin major surprise; future small
// follow-up phase ~150 LoC lands the coupled pair). response_rate_limit,
// abort.grpc_status, upstream_cluster, downstream_nodes,
// disable_downstream_cluster_stats, all four runtime-key fields,
// filter_enabled / filter_enabled_runtime: silently ignored at fault-eval
// time. HeaderMatcher non-exact variants (regex, prefix, suffix, contains,
// present-only): silently ignored.
//
// References:
//   - SPEC §1–§16 (full contract)
//   - ADR-0100 (package shape + boot registration + FactoryCtx framework
//     extension)
//   - ADR-0101 (runtimeConfig shape + PGV mirror + percentage-roll determinism)
//   - ADR-0102 (delay async-resume + combined-path timer-callback decision)
//   - ADR-0103 (abort terminal-replace + body byte-exact + 4-header set)
//   - ADR-0104 (header-driven fault path DEFERRED)
//   - ADR-0105 (max_active_faults + LBP-1 sixth + markedActive guard)
//   - ADR-0107 (17→22-name stat extension + response_rl_injected route A)
package fault
