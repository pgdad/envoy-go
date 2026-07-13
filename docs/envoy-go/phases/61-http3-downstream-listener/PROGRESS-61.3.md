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

- [x] **Task 1** — PROGRESS scaffold + baselines + the 61.3 design pins + the two RE-DERIVATION CORRECTIONS (0102→0104; HTTPExpectations TCP-only) + the carried deferred-maintenance dispositions. (folded into the PLAN commit)
- [x] **Task 2** — `test/helpers/h3.go` `H3RoundTrip` landed (pinned-UDP-dial quic-go `http3.Transport` client, TDD against a local in-process `http3.Server`).
- [x] **Task 3** — harness UDP surgery landed (`udpAddrs` + transport-aware `/udp` exposure + `ListenerUDPAddr`; both mapping starters delegate to a shared `startReferenceProxy`).
- [x] **Task 4** — reference contrib-Envoy H3 container de-risk PASSED (host→container UDP GET→200, NON-VACUOUS; see "Task 4 de-risk evidence" below).
- [x] **Task 5** — runner `ReferenceListenerIsUDP` marker + UDP-addr dispatch to `DriveReference` landed.
- [x] **Task 6** — `writeH3Reply` now synthesizes `server: envoy` + `content-length` (the 61.2-deferred H3 response fidelity; TDD via `httptest`, stdlib-only, production gate intact).
- [x] **Task 7** — the `0104-http3-downstream-get` cross-side fixture landed (driver + config templates for both sides + `AssertStats` named subset + blank-import registration).
- [x] **Task 8** — full cross-side run GREEN; all 4 load-bearing assertions (body `CompareBytes`, drive-hook status check, both `AssertStats` counters) proven live via deliberate `-count=1` breaks, each confirmed to fire the intended assertion and restored byte-identical; -race clean (see "Break evidence" below).
- [x] **Task 9** — BEHAVIOR_CONTRACT HTTP/3 cross-side-proven paragraph + ADR-0282 §Context/§Decision/§Consequences + **ROADMAP row 61 → `done`** + six-gate + sentinel re-check recorded (this commit). STATE/router roll owned by the controller.

## Six-gate (recorded at Task 9 — RUN in the worktree `.worktrees/phase-61.3-impl`)

```
$ gofmt -l .
(empty)                                                       → GOFMT_CLEAN

$ golangci-lint run (touched pkgs)
clean, exit 0

$ go vet ./...
clean

$ go build ./... && echo BUILD_OK
BUILD_OK

$ go mod tidy -diff && echo MODTIDY_CLEAN
MODTIDY_CLEAN (no module change in 61.3)

$ go list -deps ./internal/filter/hcm | grep -i quic-go || echo HCM-NO-QUICGO
(nothing)                                                     → HCM-NO-QUICGO

$ go list -deps ./internal/tls | grep -i quic-go || echo TLS-NO-QUICGO
(nothing)                                                     → TLS-NO-QUICGO

$ go list -deps ./test/helpers | grep -c quic-go
15  (TEST code — quic-go ALLOWED)

$ go list -deps ./internal/listener | grep -c quic-go
15  (production quic-go CONFINED to internal/listener)

$ grep -rh '^func Fuzz' --include='*.go' . | wc -l
55  (+0)

$ ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l
106  (+1: 0104)

$ go test ./test/helpers ./internal/filter/hcm ./internal/listener -count=1
ok

$ go test ./test/differential/ -run 'TestDifferential/0104-http3-downstream-get' -count=1 -v
PASS — cross-side H3 GET→200; non-vacuous decode, both sides downstream_rq_2xx >= 1
(see "Break evidence" below for the baseline log lines)

$ go test (touched pkgs + the 0104 fixture) -race -count=1  (run at Task 8)
race-clean
```

**FULL non-differential suite + the full 106-dir differential**: DELEGATED to the controller on the frozen squash HEAD (the 105 pre-existing dirs stay byte-stable — 61.3 changes no TCP path; the harness UDP surgery is gated on the `ReferenceListenerIsUDP` marker only `0104` sets; the writeH3Reply fidelity change affects only the H3 path only `0104` exercises).

## Task 4 de-risk evidence

SPEC-61 §8 flagged the host→container UDP-publishing direction as UNTESTED (unlike TCP port publishing, exercised by every prior fixture). Task 4 proved it directly on Docker Desktop, before the `0104` fixture was built on top of it: a `docker run` reference-Envoy container (`envoyproxy/envoy:contrib-v1.37.2`) with a QUIC listener config (mandatory TLS 1.3, ALPN `h3`, an ECDSA server cert) and its UDP listener port published (`-p <port>:<port>/udp`), driven by a host-side quic-go `http3.Transport` client:

- `GET /health` → **200**, body **`"OK\n"`**
- reference `/stats` after the request: **`http.ingress_http.downstream_rq_2xx = 1`**
- NO YAML iteration was needed to get the container's QUIC listener to accept — the config that worked at Task 4 is the same shape carried into the `0104` fixture's reference config template.
- The ECDSA certificate (matching the mandatory-TLS-1.3/QUIC transport-socket requirement) worked without additional cert-format iteration.

This satisfies the PLAN's Task-4-Step-3 requirement to record the de-risk result in PROGRESS. The de-risk PASSING cleared the fixture-build gate — Task 4's escalation path (a ReferenceLess subject-only `0104` fallback) was NOT needed.

## Break evidence (recorded at Task 8)

All commands run in `.worktrees/phase-61.3-impl` (branch `phase-61-http3-downstream-get-differential-impl`, HEAD `0cc869f6` throughout). Every break used `-run 'TestDifferential/0104-http3-downstream-get' -count=1` (the full subtest path — `reference_differential_run_selector`; `-count=1` defeats go-test's cached-PASS — `reference_differential_break_protocol_count1`).

**0. Baseline GREEN (non-vacuous):**
```
$ go test ./test/differential/ -run 'TestDifferential/0104-http3-downstream-get' -count=1 -v
...
2026/07/13 19:06:19 0104-http3-downstream-get: ref http.ingress_http.downstream_rq_2xx=1 subj http.ingress_http.downstream_rq_2xx=1
2026/07/13 19:06:19 0104-http3-downstream-get: ref http.ingress_http.downstream_rq_total=1 subj http.ingress_http.downstream_rq_total=1
--- PASS: TestDifferential (8.81s)
    --- PASS: TestDifferential/0104-http3-downstream-get (8.81s)
PASS
```
Both sides decoded live (`downstream_rq_2xx=1`, `downstream_rq_total=1` on BOTH ref and subj) — confirms the run is non-vacuous, not a green that never executed.

**1. `CompareBytes` (body) — broke the SUBJECT template's `direct_response` body** (`inline_string: "OK\n"` → `"BAD\n"`, line ~324; reference copy at ~254 untouched, so sides diverge):
```
runner_test.go:1269: differential mismatch:
    first divergence at offset 0
    ref [0..3]:  00000000  4f 4b 0a               |OK.|
    subj[0..4]:  00000000  42 41 44 0a            |BAD.|
--- FAIL: TestDifferential/0104-http3-downstream-get (1.98s)
```
WHICH FIRED: the `CompareBytes` byte-compare at `runner_test.go:1269` — not an earlier drive error (the drive hooks both returned 200 first: the log lines `... downstream_rq_2xx=1 ...downstream_rq_total=1` still printed, i.e. `AssertStats` ran too, proving CompareBytes is the sole failure and it fires before the test aborts). Restored `"OK\n"`; `git diff test/fixtures/0104-http3-downstream-get/driver/driver.go` → 0 lines (byte-identical). Re-ran `-count=1` → PASS.

**2. Drive-hook status assertion — broke the SUBJECT `direct_response` status** (`status: 200` → `500`, line ~323):
```
runner_test.go:1256: subj drive: H3 GET 127.0.0.1:20016: status 500, want 200
--- FAIL: TestDifferential/0104-http3-downstream-get (2.06s)
```
WHICH FIRED: the subject `DriveSubject`/`h3Driver.drive` `status != http.StatusOK` check (`driver.go:72-74`), surfaced by the runner at `runner_test.go:1256` as a Fatalf-style drive error — before `CompareBytes` or `AssertStats` ever ran (no stats log lines printed). Restored `status: 200`; `git diff` → 0 lines. Re-ran `-count=1` → PASS.

**3. `AssertStats` counter #1 (`downstream_rq_2xx`)** — renamed the asserted stat to a non-existent name (`http.ingress_http.downstream_rq_2xx` → `..._NOPE`, line ~121):
```
2026/07/13 19:07:15 0104-http3-downstream-get: ref http.ingress_http.downstream_rq_2xx_NOPE=0 subj http.ingress_http.downstream_rq_2xx_NOPE=0
    runner_test.go:1330: ref http.ingress_http.downstream_rq_2xx_NOPE = 0, want >=1
    runner_test.go:1330: subj http.ingress_http.downstream_rq_2xx_NOPE = 0, want >=1
2026/07/13 19:07:15 0104-http3-downstream-get: ref http.ingress_http.downstream_rq_total=1 subj http.ingress_http.downstream_rq_total=1
--- FAIL: TestDifferential/0104-http3-downstream-get (2.19s)
```
WHICH FIRED: both `t.Errorf` calls (ref AND subj) for the renamed `downstream_rq_2xx_NOPE` stat (value 0, since the stat doesn't exist) at `runner_test.go:1330` — isolated: the sibling `downstream_rq_total` check still logged its live value (=1 both sides) with NO error, proving the break targeted only the intended counter and the other assertion stayed live/unmasked. Restored the stat name; `git diff` → 0 lines. Re-ran `-count=1` → PASS.

**4. `AssertStats` counter #2 (`downstream_rq_total`)** — renamed the second asserted stat (`http.ingress_http.downstream_rq_total` → `..._NOPE`, line ~122):
```
2026/07/13 19:07:30 0104-http3-downstream-get: ref http.ingress_http.downstream_rq_2xx=1 subj http.ingress_http.downstream_rq_2xx=1
2026/07/13 19:07:30 0104-http3-downstream-get: ref http.ingress_http.downstream_rq_total_NOPE=0 subj http.ingress_http.downstream_rq_total_NOPE=0
    runner_test.go:1330: ref http.ingress_http.downstream_rq_total_NOPE = 0, want >=1
    runner_test.go:1330: subj http.ingress_http.downstream_rq_total_NOPE = 0, want >=1
--- FAIL: TestDifferential/0104-http3-downstream-get (1.99s)
```
WHICH FIRED: both `t.Errorf` calls for the renamed `downstream_rq_total_NOPE` stat — isolated: `downstream_rq_2xx` logged its live value (=1 both sides) with no error, confirming the break was scoped to the targeted counter alone. Restored; `git diff` → 0 lines. Re-ran `-count=1` → PASS.

**5. -race:**
```
$ go test ./test/helpers/ ./internal/filter/hcm/ ./internal/listener/ -race -count=1
ok  	github.com/pgdad/envoy-go/test/helpers	1.029s
ok  	github.com/pgdad/envoy-go/internal/filter/hcm	1.064s
ok  	github.com/pgdad/envoy-go/internal/listener	4.253s

$ go test ./test/differential/ -run 'TestDifferential/0104-http3-downstream-get' -race -count=1
ok  	github.com/pgdad/envoy-go/test/differential	3.023s
```
Both race-clean — the H3 client (`test/helpers.H3RoundTrip`) and QUIC serve goroutines (`internal/listener`, `internal/filter/hcm`) show no data races.

**Final restoration check:**
```
$ git diff 0cc869f6 -- test/fixtures/0104-http3-downstream-get/driver/driver.go
(empty — 0 lines)
```
`driver.go` is byte-identical to Task 7's commit `0cc869f6`; every break was fully restored. Final confirmatory run: `go test ./test/differential/ -run 'TestDifferential/0104-http3-downstream-get' -count=1 -v` → PASS (`downstream_rq_2xx=1`, `downstream_rq_total=1` both sides).

All 4 load-bearing assertions (body CompareBytes, drive-hook status check, both AssertStats counters) proven live via deliberate `-count=1` breaks, each confirmed to fire the INTENDED assertion (not masked by an earlier or sibling failure), and each restored byte-identical.

## Sentinel re-check (Task 9, mechanical — run POST-flip)

The three sentinel checks, run mechanically after flipping ROADMAP row 61 `in-progress` → `done`:

1. **No row prints `NOT DONE`** — row 61 clears (it now reads `done`); no other row regresses.
2. **The stop-condition does NOT fire because THREE live "candidates:" sentences remain**: HTTP/3 + QUIC (family STAYS OPEN — the deferred-candidate sentence at ROADMAP.md line 171 stays exactly one live match, verified via `grep -c 'remaining deferred (not-yet-chartered) candidates:.*upstream H3 cluster' docs/envoy-go/ROADMAP.md` → `1`), xDS, Observability.
3. **THREE never-opened families remain**: gRPC, Runtime, WASM.

Conclusion: the sentinel `stop` marker is **NOT created** — row 61's flip to `done` closes ONE row but does not clear the project-wide stop conditions (live deferred-candidate sentences + never-opened families both persist). This is the expected, correct outcome for a split-phase row close per `reference_roadmap_split_phase_row_done` (a per-leg flip, not a project-completion signal).
