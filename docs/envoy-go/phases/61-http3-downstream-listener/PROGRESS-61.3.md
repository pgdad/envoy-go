# Phase 61.3 PROGRESS — `http3-downstream-get-differential` (IMPL)

> **Scaffold produced at the phase-61.3 PLAN stage** (docs-only). The IMPL executes `PLAN-61.3.md` task-by-task, subagent-driven (`feedback_execution_style`), in worktree `.worktrees/phase-61.3-impl`, branch `phase-61-http3-downstream-get-differential-impl`, off master. This is the THIRD and FINAL leg of the confirmed 61.1/61.2/61.3 split (SPEC-61 §3.0); ANCHORS **ADR-0282** (RECOMMENDED — a NEW per-leg ADR for the harness's first non-TCP transport; the IMPL MAY fold into ADR-0281 and record that). **Row 61 flips `in-progress` → `done` at this six-gate** — ALL THREE legs land here (ADR-0106 / `reference_roadmap_split_phase_row_done`). The HTTP/3 + QUIC FAMILY STAYS OPEN (the §8 deferred candidates remain).

## Baseline counts (verify at IMPL start against the master tip; `git fetch` first)

| metric | baseline (61.2 exit) | anticipated exit (61.3) |
|---|---|---|
| stat surface | 1201 | **1201** (+0 RECOMMENDED — the H3 path Inc's the codec-agnostic `downstream_rq_<Nxx>`/`downstream_rq_total` counters, which 61.3 asserts cross-side as a NAMED SUBSET; registers NO new counter. IMPL MAY pin +2 `downstream_{cx,rq}_http3_total` to match the reference's http3-specific surface, but the recommendation is +0 + the named-subset assertion, `reference_stats_sink_emits_used_only`) |
| fixtures | 105 (tail `0103-xds-sds-server-cert`) | **106** (+1 — `0104-http3-downstream-get`; NOT `0102` — see the CORRECTION below) |
| fuzzers | 55 | **55** (+0 — quic-go owns H3 framing + QPACK; no new hand-rolled parse) |
| BackendKind tail | 38 (`H2GoawayResponder`) | **38** (+0 — the `direct_response` fixture needs no `test/helpers` responder) |
| DECISIONS tail | ADR-0281 (61.2) | **ADR-0282** (a NEW per-leg harness-transport ADR, authored here; next-free **ADR-0283**) |
| new production Go packages | 0 | **+0** (the ONLY production change is the stdlib-only writeH3Reply fidelity pickup in the existing `hcm` package; `test/helpers/h3.go` is a NEW test-helper FILE in the existing `test/helpers` package, not a new package) |
| new go.mod modules | 0 | **+0** (quic-go v0.54.1 [direct, 61.1] + qpack v0.5.1 [indirect, 61.2] already landed; `H3RoundTrip`'s `http3.Transport` import adds no new module) |
| ROADMAP row 61 | `in-progress` | **`done`** (ALL three legs landed — the FLIP, ADR-0106) |

## ⚠️ RE-DERIVATION CORRECTIONS recorded here (per `feedback_brief_citations_not_evidence`)

1. **The fixture is `0104-http3-downstream-get`, NOT the SPEC/router `0102`.** SPEC-61 §8 + the router named it `0102`; that slot was FREE at the SPEC's master tip (`cbda648b`, fixtures 103, tail `0101-stats-sink-graphite`). Since then `0102-tracing-custom-tags-literal` (phase 59) + `0103-xds-sds-server-cert` (phase 60.2) LANDED — the tail is `0103`, next free is **`0104`**. RE-VERIFY at IMPL start (`ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | sort | tail -3`); take the next free if a parallel row raced past `0104`.
2. **`HTTPExpectations` is TCP-only and UNUSABLE for H3.** The runner enforces `HTTPExpectations` by re-driving each request with its OWN internal HTTP/1-over-TCP client (`test/differential/runner_test.go:1273-1291`, `refResp.StatusCode`/`subjResp.StatusCode`) — it cannot reach a QUIC/UDP listener. The `0104` driver does NOT implement `HTTPExpectations`; it asserts status inside the Drive hooks (`H3RoundTrip` → error on `status != 200`), body via the runner's `CompareBytes`, and the named-stat subset via `StatsAsserter.AssertStats`.
3. **The harness surgery touches TWO of the three container-starters, not three.** SPEC-61 §8 said "three container-starters"; `tryStartReferenceProxy` (`harness.go:422`, the boot-reject path) calls NO `MappedPort` and builds NO address map, so it needs NO UDP surgery. Only `StartReferenceProxy` (`:107`) + `StartReferenceProxyWithMounts` (`:170`) map ports.

## Import hygiene (LOAD-BEARING — re-check at Tasks 2/6/9)

quic-go stays confined to `internal/listener/quic.go` in PRODUCTION (the 61.1/61.2 gate). 61.3's `test/helpers/h3.go` imports quic-go's `http3.Transport` — but that is TEST code and does NOT touch production deps. The ONLY production change (Task 6, writeH3Reply `server`/`content-length` synthesis) stays stdlib-only `net/http`. Verify:
- `go list -deps ./internal/filter/hcm | grep -i quic-go || echo HCM-NO-QUICGO` → `HCM-NO-QUICGO` (production gate intact after Task 6)
- `go list -deps ./internal/tls | grep -i quic-go || echo TLS-NO-QUICGO` → `TLS-NO-QUICGO`
- `go list -deps ./internal/listener | grep -i quic-go` → prints the quic-go module (confined)
- `go list -deps ./test/helpers | grep -i quic-go` → prints quic-go (TEST code — ALLOWED, not a production breach)

## 61.3 design pins (settled in PLAN-61.3 §"Design pins settled here")

- **FIXTURE = `0104-http3-downstream-get`** (0102/0103 consumed; fixtures 105 → 106).
- **OBSERVABLE = status 200 + body `"OK\n"` (byte-stable via `CompareBytes`) + a NAMED downstream-stat subset (`AssertStats`)** — NOT full response-header byte-parity (the H3 wire is encrypted/QPACK — not raw-comparable; header ordering/casing/`date` vary), NOT `HTTPExpectations` (TCP-only). Status asserted in the Drive hooks (error on non-200); body via `CompareBytes`; stats via `AssertStats` (cross-side-live; NOT the dead `SubjectAsserter`, `reference_differential_asserter_dispatch`).
- **HARNESS SURGERY = transport-aware `/udp` exposure + `udpAddrs` + `ListenerUDPAddr` + a `ReferenceListenerIsUDP` runner marker.** Two mapping starters; the subject side needs NO Docker change (binds UDP directly, reports via the `\S+` ready-sentinel).
- **H3 CLIENT = `test/helpers/h3.go` `H3RoundTrip`** — a pinned-UDP-dial quic-go `http3.Transport` client (ignores Alt-Svc), one client drives BOTH sides. Test-code quic-go (production gate intact).
- **writeH3Reply FIDELITY = synthesize `server: envoy` + `content-length`** (the ADR-0281 §Consequences deferral); `date` NOT synthesized (timestamp; not byte-compared). Stdlib-only.
- **ACCESS-LOG/SPAN Protocol string = `"HTTP/3"`** — verified-by-inspection (matches the reference's `%PROTOCOL%`), UNasserted cross-side (deferred confirmation; non-blocking for row 61 → done).
- **ADR-0282 = a NEW per-leg ADR** (the harness's first non-TCP transport — a reusable seam for every future QUIC/UDP fixture); fold-into-ADR-0281 is the documented IMPL alternative.
- **Reference-container UDP-reachability de-risked FIRST (Task 4)** — host→container UDP publishing (SPEC-61 §8's untested direction; PROVEN by the SPEC-61 §11 probe on this machine) proven before the fixture is built on top; ESCALATE on failure (fallback: a ReferenceLess subject-only `0104`).

## 61.1/61.2 deferred-maintenance dispositions (carried; RE-DEFERRED — none exercised or worsened by `0104`)

- **61.1 M6-1** (`quicAcceptLoop` no TCP-style backoff on repeated `Accept` error) — untouched by 61.3; RE-DEFERRED to a QUIC-robustness row.
- **61.1 M-FB1** (QUIC transport-socket decode wired only into `filter_chains[]`, not `default_filter_chain`) — `0104` uses `filter_chains[]`; RE-DEFERRED to a multi-chain/SNI row.
- **61.1 M-FB2** (`quicTLSConfig`/`quicChain` map-iteration nondeterminism over `chainByName`) — harmless for the single-chain `0104`; RE-DEFERRED to an SNI-dispatch row.
- **61.2 T5-M1** (`runH3` skips the `downstream_rq_<Nxx>` Inc on the encode-error early-return — intentional `WriteH2` parity), **T5-M2** (POST-body test depth), **T5-B1** (`SetDownstreamLocalAddr` nil — quic-go doesn't populate `LocalAddrContextKey`), **T7-M1** (`quicChain`/`quicTLSConfig` divergence only under deferred multi-chain SNI) — none exercised by the single-chain GET `0104`; RE-DEFERRED unchanged. None is a security/resource/crash risk within the deferred scope.

## Task checklist (mirrors PLAN-61.3)

- [ ] **Task 1** — PROGRESS scaffold + baselines + the 61.3 design pins + the two RE-DERIVATION CORRECTIONS (0102→0104; HTTPExpectations TCP-only) + the carried deferred-maintenance dispositions. (folded into the PLAN commit)
- [ ] **Task 2** — `test/helpers/h3.go` `H3RoundTrip` (pinned-UDP-dial quic-go `http3.Transport` client). [TDD, local in-process `http3.Server`]
- [ ] **Task 3** — harness UDP exposure (`udpAddrs` + transport-aware `/udp` + `ListenerUDPAddr`; the two mapping starters). [TDD, harness-level]
- [ ] **Task 4** — reference contrib-Envoy H3 container de-risk (host→container UDP GET→200, NON-VACUOUS). [integration proof; ESCALATE on failure]
- [ ] **Task 5** — runner `ReferenceListenerIsUDP` marker + UDP-addr dispatch to `DriveReference`. [runner surgery]
- [ ] **Task 6** — writeH3Reply synthesizes `server: envoy` + `content-length` (the 61.2-deferred H3 response fidelity). [TDD, `httptest` — stdlib-only, production gate intact]
- [ ] **Task 7** — the `0104-http3-downstream-get` cross-side fixture (driver + config templates for BOTH sides + `AssertStats` named subset + blank-import registration). [fixture]
- [ ] **Task 8** — the full cross-side run GREEN + per-assertion liveness breaks (`CompareBytes` body, the status assertion, each `AssertStats` counter; `-count=1`; confirm WHICH fires) + -race. [verify]
- [ ] **Task 9** — BEHAVIOR_CONTRACT HTTP/3 cross-side-proven + ADR-0282 §Context/§Decision/§Consequences + **ROADMAP row 61 → `done`** + STATE + six-gate + sentinel re-check + router roll. [docs + verify]

## Six-gate (recorded at Task 9 — RUN in the worktree `.worktrees/phase-61.3-impl`)

```
$ gofmt -l .
(expect empty — GOFMT_CLEAN)

$ golangci-lint run ./...
(expect clean, exit 0)

$ go vet ./...
(expect clean)

$ go build ./... && echo BUILD_OK
(expect BUILD_OK)

$ go mod tidy -diff && echo MODTIDY_CLEAN
(expect MODTIDY_CLEAN — no module change in 61.3)

$ go list -deps ./internal/filter/hcm | grep -i quic-go || echo HCM-NO-QUICGO
(expect HCM-NO-QUICGO — the writeH3Reply fidelity pickup stays stdlib-only)

$ go list -deps ./internal/tls | grep -i quic-go || echo TLS-NO-QUICGO
(expect TLS-NO-QUICGO)

$ go list -deps ./test/helpers | grep -i quic-go
(expect the quic-go module — TEST code, ALLOWED)

$ grep -rh '^func Fuzz' --include='*.go' . | wc -l
(expect 55 — +0)

$ ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l
(expect 106 — +1: 0104)

$ go test ./test/helpers/ ./internal/filter/hcm/ ./internal/listener/ -count=1
(expect ok)

$ go test ./test/differential/ -run 'TestDifferential/0104-http3-downstream-get' -count=1 -v
(expect PASS — cross-side H3 GET→200; non-vacuous decode, both sides downstream_rq_2xx >= 1)
```

**FULL non-differential suite + the full 106-dir differential**: DELEGATED to the controller on the frozen squash HEAD (the 105 pre-existing dirs stay byte-stable — 61.3 changes no TCP path; the harness UDP surgery is gated on the `ReferenceListenerIsUDP` marker only `0104` sets; the writeH3Reply fidelity change affects only the H3 path only `0104` exercises).

## Break evidence (recorded at Task 8)

(To be filled at the IMPL: the `-count=1` deliberate break for EACH load-bearing assertion — the `CompareBytes` body-compare, the Drive-hook status assertion, each `AssertStats` counter — with WHICH assertion fired confirmed [`reference_deliberate_break_wrong_assertion`], and the restored-byte-identical `git diff` confirmation.)

## Sentinel re-check (Task 9, mechanical)

(To be filled at the IMPL: the three sentinel checks run MECHANICALLY. Expected — (1) row 61 now `done` so it no longer prints `NOT DONE: row 61`, but the sentinel does NOT fire because (2) three live "candidates:" sentences [HTTP/3 STAYS OPEN, xDS, Observability] + (3) three never-opened families [gRPC/Runtime/WASM] + Operational-tooling. The HTTP/3 deferred sentence STAYS exactly one live match, `reference_sentinel_deferred_sentence_live_vs_historical`.)
