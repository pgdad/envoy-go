# Phase 05.1 — Downstream HTTP/2 (placeholder)

This directory was created by the planner-time split of phase 05 (see `DECISIONS.md` ADR-0045 and `BOOTSTRAP_PROMPT.md` §6.2). It awaits a SPEC.md drafted by `superpowers:brainstorming` per ADR-0004.

**Master design document:** `docs/envoy-go/phases/05-http-2/SPEC.md` (commit `612cdea`). The phase-05 SPEC remains the authoritative design for both 05.1 and 05.2; sub-phase SPECs carve coherent slices of its §4 deliverables.

**Sub-phase 05.1 scope** (per ADR-0045 + phase-05 SPEC §11.1 split-by-surface recommendation):

- Downstream HTTP/2 termination: full `internal/filter/hcm/h2/` SERVER-side surface — `errors.go`, `preface.go`, `framer.go`, `hpack.go`, `settings.go`, `flow.go`, `stream.go` (server-side per-stream state machine), `conn.go` (`ServerConn` + connection state machine).
- HCM ALPN dispatch in `internal/filter/hcm/filter.go`; `codec_type: HTTP2` permitted in `internal/filter/hcm/config.go`; `listenerCtx` plumbed from `internal/listener/manager.go` into `hcm.NewFilter`.
- Direct-response on H2: codec-neutral factoring of `directResponseAction.body()` + `writeH2` adapter (per phase-05 SPEC §5.5) so direct_response works end-to-end on the H2 path. **Required in 05.1** because the h2spec conformance gate exercises `direct_response` on the dedicated h2c listener.
- `cmd/envoy-go`: `--allow-h2c` test-only flag (per phase-05 SPEC §4.4 ADR-Z) bypassing the codec_type-vs-transport check; `cmd/envoy-go/main_test.go` h2 bootstrap variant smoke test.
- `test/conformance/h2spec/`: `h2spec.go` (threshold-section list) + `h2spec_test.go` (testcontainers-driven runner against a `--allow-h2c` h2c listener).
- `docs/envoy-go/CONFORMANCE_PINS.md` (NEW file): pins `summerwind/h2spec` by tag + SHA256.
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` `## HTTP/2` subsection: SCAFFOLDED with the codec/conformance scope (asserted: `:status` and decoded-body equivalence on direct_response 2xx via h2spec; not asserted: wire-byte framing). The upstream-H2 + fixture-0004 differential rules are added in 05.2.
- New fuzz targets: `internal/filter/hcm/h2.FuzzFrameStream`, `internal/filter/hcm/h2.FuzzHPACKDecode` (short-budget per ADR-0018).
- Phase-04 REVIEW Minor carry-forward triage (SPEC §12 + ADR-X) — landed in 05.1 since it touches no upstream-H2 surface.

**ADRs anticipated in 05.1** (per phase-05 SPEC §4.4, sequential numbering after ADR-0045 takes 0045 for the split): the planner verifies the next-free at write time. Anticipated: ADR-P (codec source), ADR-Q (conn manager from scratch), ADR-S (server settings), ADR-T (BEHAVIOR_CONTRACT scaffold), ADR-U (h2spec scope + threshold), ADR-V (ALPN dispatch wiring), ADR-Z (--allow-h2c), ADR-X (phase-04 REVIEW carry-forward triage).

**OUT of 05.1 (deferred to 05.2):** `internal/filter/hcm/h2/client.go` (`ClientConn`), `internal/cluster/dial_h2.go`, `Cluster.UseH2()` accessor, `internal/cluster/manager.go` `HttpProtocolOptions` parsing + validation, `internal/filter/hcm/actions.go` `routerActionH2` variant, blank import for `upstreams/http/v3` in `internal/cluster/cluster.go`, `test/fixtures/0004-h2-routing/` (full fixture incl. PKI gen + backends + driver + YAMLs + expectations + README), `test/helpers/h2.go`, `test/differential/runner_test.go` blank-import for fixture 0004, ADR-R (per-request fresh upstream H2), ADR-W (closes ADR-0035 H2 leg), ADR-Y (trailers).

**Differential surface at end of 05.1:** Pre-existing fixtures (`0000-tcp-echo`, `0001-tcp-proxy-rr`, `0002-tls-tcp`, `0003-http11-routing`) remain green with no behavioural regression (gate b). NO new differential fixture (gate a is vacuously green). NEW conformance gate (c) is non-vacuous for the first time in the project: `h2spec` runs against a `--allow-h2c` h2c listener and reports `failed == 0` over the threshold sections (3, 4, 5, 6 ex-6.6, 7, 8) per ADR-U. Gates (d) fuzz, (e) build/lint/test, (f) review apply normally.

---

When `superpowers:brainstorming` next runs against this directory, it produces `SPEC.md` here that authoritatively defines 05.1's scope (this README is illustrative; the SPEC supersedes it).
