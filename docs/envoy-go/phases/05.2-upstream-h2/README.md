# Phase 05.2 — Upstream HTTP/2 + fixture 0004 (placeholder)

This directory was created by the planner-time split of phase 05 (see `DECISIONS.md` ADR-0045 and `BOOTSTRAP_PROMPT.md` §6.2). It awaits a SPEC.md drafted by `superpowers:brainstorming` per ADR-0004.

**Master design document:** `docs/envoy-go/phases/05-http-2/SPEC.md` (commit `612cdea`). The phase-05 SPEC remains the authoritative design for both 05.1 and 05.2; sub-phase SPECs carve coherent slices of its §4 deliverables.

**Sub-phase 05.2 scope** (per ADR-0045 + phase-05 SPEC §11.1 split-by-surface recommendation):

- `internal/filter/hcm/h2/client.go` — from-scratch `ClientConn` + `RoundTrip` for the upstream H2 leg (per phase-05 SPEC §4.1 #4 and §5.3).
- `internal/cluster/dial_h2.go` — `Cluster.DialH2(ctx) (*h2.ClientConn, error)`; ALPN-h2 confirmation; "not a TLS conn" diagnostic.
- `internal/cluster/cluster.go` — `Cluster.UseH2() bool` accessor.
- `internal/cluster/manager.go` — parses `typed_extension_protocol_options["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]`, peeks at `explicit_http_config.http2_protocol_options` discriminator, validates TLS + alpn `h2` on the cluster's `transport_socket` (per phase-05 SPEC §5.8).
- Blank import for `_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"` in `internal/cluster/cluster.go` (and possibly `internal/bootstrap/bootstrap.go` per phase-05 SPEC §4.2 — the planner verifies whether the bootstrap-side import is also needed for protojson round-trip in tests).
- `internal/filter/hcm/actions.go` — `routerActionH2` variant alongside the phase-04 `routerAction`. Build-time choice driven by the matched cluster's `UseH2()`.
- `test/fixtures/0004-h2-routing/` — NEW fixture: `envoy.yaml`, `envoy-go.yaml`, `expectations.yaml`, `README.md`, `pki/gen/main.go` + generated `pki/*.pem`, `driver/driver.go` + `driver_test.go`, `backends/main.go`. 27-request workload (9 × `/health` direct_response 200, 9 × `/api/v1/<n>` router-action, 9 × `/missing/<n>` direct_response 404). Per-side `[3,3,3]` distribution. Full HTTPS h2 between proxy and upstream backends.
- `test/helpers/h2.go` + `h2_test.go` — driver-side `H2RoundTrip` helper.
- `test/differential/runner_test.go` — blank-import for `test/fixtures/0004-h2-routing/driver`; optional `H2Expectations` extension on `Driver` interface (per phase-05 SPEC §4.3 + §10 #3 — planner picks final shape).
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` `## HTTP/2` subsection: EXTENDED from 05.1's scaffold with the upstream-H2 + fixture-0004 differential rules, header allow-list extensions for H2 pseudo-headers (`:status`, `:method`, `:path`, `:scheme`, `:authority`), the closure of ADR-0035's H2 leg.

**ADRs anticipated in 05.2** (sequential numbering after 05.1's tail; planner verifies the next-free at write time): ADR-R (per-request fresh upstream H2 dial — mirror of ADR-0039), ADR-W (closes ADR-0035 H2 leg; H1+TLS upstream gap remains), ADR-Y (trailers observed but not forwarded). Phase-05 SPEC §4.4 also lists ADR-X (phase-04 REVIEW carry-forward) but per ADR-0045 that ADR lands in 05.1 because it does not touch upstream-H2 surface; the planner of 05.2 may revisit if any 05.2-specific carry-forward arises.

**Depends on:** 05.1 (the H2 codec sub-package's server-side surface must exist before the client-side and upstream router action can be built on top of it; both live in `internal/filter/hcm/h2/`).

**OUT of 05.2:** anything 05.1 already shipped (the codec server surface, ALPN dispatch, h2spec, `--allow-h2c`, `CONFORMANCE_PINS.md`, the BEHAVIOR_CONTRACT scaffold).

**Differential surface at end of 05.2:** NEW fixture `0004-h2-routing` differentially green (gate a) — `:status` per request equivalence, decoded body equivalence on the 9 `/health` direct-response requests, per-side `[3,3,3]` per-cluster RR distribution, 404 status equivalence on the 9 `/missing` requests. Pre-existing fixtures (0000–0003) remain green (gate b). Conformance (gate c) still green. Gates (d) fuzz, (e) build/lint/test, (f) review apply normally.

---

When `superpowers:brainstorming` next runs against this directory, it produces `SPEC.md` here that authoritatively defines 05.2's scope (this README is illustrative; the SPEC supersedes it). Brainstorming for 05.2 should not start until 05.1 reaches `done` (the H2 codec server-side internals are 05.1's deliverable and 05.2's foundation).
