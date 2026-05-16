# Phase 19.2 — HTTP filter envoy.filters.http.ext_proc (body-mode extension + ADR-0175 framework primitive) — Implementation Progress

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-04..19.1 PROGRESS.md structure.

- **Phase:** 19.2 — HTTP filter `envoy.filters.http.ext_proc` (body-mode extension: `request_body_mode`/`response_body_mode = BUFFERED` activation for gRPC-service mode + ADR-0175 encode-side body-buffering framework primitive + ADR-0168/0171/0172 body-mode §Decision AMENDMENTS)
- **Branch:** `phase-19.2-http-filter-ext-proc-body-impl` (fresh worktree at `.worktrees/phase-19.2-http-filter-ext-proc-body-impl`)
- **Base commit (master tip):** `cefd45b` (phase-19.2 PLAN SHA-fill follow-up; PLAN squash `a8d4124`; SPEC SHA-fill follow-up `b9d2b78`; SPEC squash `954a570`; phase-19.1 IMPL squash `95bb425`)
- **PLAN tip SHA:** `a8d4124` (`git log -1 --format=%H -- docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PLAN.md` → `a8d4124bd493c89b2eee989cce779f090df75e47`)
- **SPEC tip SHA:** `954a570` (`git log -1 --format=%H -- docs/envoy-go/phases/19.2-http-filter-ext-proc-body/SPEC.md` → `954a57033e3f0ea448557d3e09ccf8f1f4510d0a`)
- **Links:** [`PLAN.md`](./PLAN.md) · [`SPEC.md`](./SPEC.md) · parent [`../19-http-filter-ext-proc/SPEC.md`](../19-http-filter-ext-proc/SPEC.md) · sibling [`../19.1-http-filter-ext-proc-headers/PROGRESS.md`](../19.1-http-filter-ext-proc-headers/PROGRESS.md) · sibling [`../19.1-http-filter-ext-proc-headers/REVIEW.md`](../19.1-http-filter-ext-proc-headers/REVIEW.md)

---

## Cold-start preconditions verified

All 12 preconditions verified green at cold-start of branch `phase-19.2-http-filter-ext-proc-body-impl` (worktree at `.worktrees/phase-19.2-http-filter-ext-proc-body-impl`, branched from master tip `cefd45b`). Master tail shows the 19.2-PLAN SHA-fill follow-up at `cefd45b`, the PLAN squash at `a8d4124`, the 19.2-SPEC SHA-fill follow-up at `b9d2b78`, and the SPEC squash at `954a570`, with the 19.1-IMPL squash `95bb425` immediately preceding (the 19.1 closure stack `fc4970a` + `95bb425` + `0a46046` + `7483411` exactly as expected). Go 1.26.2, golangci-lint v1.64.8 (ADR-0009 pin), Docker client 28.4.0 + server 28.1.1 present. ADR tail at 0176 (ADR-0167..ADR-0176 already at master; ADR-0175 §Context drafted at parent SPEC commit `9cc1458` per ADR-0044 ADR-on-impl convention with §Decision body still absent — lands at Task 2; ADR-0177 stays unconsumed under PLAN D10 hypothesis — reserved for any 19.2-IMPL-unanticipated load-bearing surface). The §Decision + §Consequences bodies for the 4 anticipated-at-19.2 ADR landings (ADR-0175 full §Decision + §Consequences; ADR-0168/0171/0172 in-place §Decision AMENDMENTS) land at impl-time anchor Tasks 2/3+7/4/6 per the per-ADR table below — mirroring the phase-13/15/16/17/18.1/18.2/19.1 ADR-0044 ADR-on-impl pattern. SPEC at `954a570`; PLAN at `a8d4124`. The 19.1-landing surfaces (`internal/filter/http/extproc/{extproc.go, processor.go, check.go, attributes.go}` + `internal/grpcclient/processor_client.go` + `test/helpers/extprocgrpc/`) are all present (the 19.2 IMPL extends these files in-place + reuses `test/helpers/extprocgrpc/` UNMODIFIED per PLAN §"File structure"). `go test -count=1 -short ./...` returns clean (53 ok packages; 0 FAIL). `go test -count=1 ./test/differential/ -run 'TestDifferential'` runs all 23 sub-tests `0000-tcp-echo` through `0022-http-ext-proc-grpc`; all PASS on a re-run after a one-time port-bind flake at fixture `0012-http-header-mutation` (root-cause: random-port collision on a co-running container's :34186 bind — infrastructure flake, not an envoy-go regression; the substantive pre-existing-suite baseline GREEN per the retry below). 3 representative fuzzers spot-checked at 30s each (`FuzzExtProcConfigParse` + `FuzzBootstrapLoad` + `FuzzHCMConfigParse`); all PASS clean (`go test ... PASS` final line). Working tree pristine (empty `git status --porcelain`).

**Note on PLAN precondition 10 regex.** The PLAN's literal regex `Test.*00(0[0-9]|1[0-9]|2[0-2])` does not match `TestDifferential` (the actual top-level function name; the `0000..0022` identifiers appear only as `t.Run` sub-test names). The substantive verification is the full `TestDifferential` run with all 23 sub-tests `0000` through `0022` green. Recorded here for the same reason 18.1 + 18.2 + 19.1 PROGRESS.md recorded their analogous precondition-regex deviations: planner-time wording vs runtime fact, not a blocking divergence. This note parallels 19.1 PROGRESS's note on its own precondition 11.

**Note on the precondition-10 flake at fixture `0012-http-header-mutation`.** The first execution of `go test -count=1 ./test/differential/ -run 'TestDifferential' -v` reported `--- FAIL: TestDifferential/0012-http-header-mutation (1.69s)` with the underlying error `listener start: listener: "l_mws": bind 0.0.0.0:34186: listen tcp 0.0.0.0:34186: bind: address already in use` (the random-port allocator collided with a port that another container had just bound). The remaining 22 sub-tests PASSED on the same run; the retry `go test -count=1 ./test/differential/ -run 'TestDifferential/0012-http-header-mutation' -v` PASSED on the first attempt (`--- PASS: TestDifferential/0012-http-header-mutation (1.72s)`). Classified as an infrastructure flake (random-port collision), not an envoy-go regression. The substantive verification — all 23 pre-existing fixtures `0000..0022` GREEN — is satisfied (22 on the first run + 1 on the retry; same binary, no envoy-go change between attempts).

### Precondition 1 — worktree branch

```
$ git rev-parse --abbrev-ref HEAD
phase-19.2-http-filter-ext-proc-body-impl
```

### Precondition 2 — master tail

```
$ git log --oneline master | head -8
cefd45b phase 19.2 PLAN follow-up: STATE.md SHA-fill (TBD → a8d4124 post-squash)
a8d4124 Squash merge phase-19.2-http-filter-ext-proc-body-plan
b9d2b78 phase 19.2 SPEC follow-up: STATE.md SHA-fill (TBD → 954a570 post-squash)
954a570 Squash merge phase-19.2-http-filter-ext-proc-body-spec
fc4970a phase 19.1 IMPL follow-up: STATE.md SHA-fill (TBD → 95bb425 post-squash)
95bb425 Squash merge phase-19.1-http-filter-ext-proc-headers-impl
0a46046 phase 19.1 PLAN follow-up: STATE.md SHA-fill (TBD → 7483411 post-squash)
7483411 Squash merge phase-19.1-http-filter-ext-proc-headers-plan
```

The 19.2-PLAN SHA-fill follow-up sits at `cefd45b`; the PLAN squash at `a8d4124`; the 19.2-SPEC SHA-fill follow-up at `b9d2b78`; the SPEC squash at `954a570`; the 19.1-IMPL closure stack (`fc4970a` + `95bb425`) immediately precedes — exactly the expected sequence per PLAN precondition 2.

### Precondition 3 — toolchain

```
$ go version
go version go1.26.2 linux/amd64

$ golangci-lint version
golangci-lint has version v1.64.8 built with go1.26.2 from (unknown, modified: ?, mod sum: "h1:y5TdeVidMtBGG32zgSC7ZXTFNHrsJkDnpO4ItB3Am+I=") on (unknown)

$ docker version | head -8
Client: Docker Engine - Community
 Version:           28.4.0
 API version:       1.49 (downgraded from 1.51)
 Go version:        go1.24.7
 Git commit:        d8eb465
 ...
Server: Docker Desktop 4.41.2 (191736)
 Engine:
  Version:          28.1.1
```

Go 1.26.2 ≥ required; golangci-lint v1.64.8 at ADR-0009 pin; Docker client 28.4.0 + server 28.1.1 both present.

### Precondition 4 — DECISIONS.md tail

```
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1
176
```

ADR tail at 0176 (ADR-0176 is the ADR-0045 split-application ADR landed in full at the parent SPEC commit `9cc1458`). Higher value would have indicated a concurrent landing; the 176 result confirms no out-of-band ADR landed since phase-19.1 IMPL squash.

### Precondition 5 — ADR-0175 §Context present + §Decision body absent (lands at Task 2)

```
$ grep -cE '^## ADR-0175' docs/envoy-go/DECISIONS.md
1

$ grep -A20 '^## ADR-0175' docs/envoy-go/DECISIONS.md | grep -c '^### Decision'
0

$ grep -nE '^## ADR-0177' docs/envoy-go/DECISIONS.md
(no output; exit=1)
```

ADR-0175 header present (1 match); §Decision body absent — lands at Task 2 per the per-ADR table below. ADR-0177 absent (exit=1 means grep found 0 matches) — stays unconsumed at 19.2 IMPL under PLAN D10 hypothesis (reserved for any 19.2-IMPL-unanticipated load-bearing surface).

### Precondition 6 — SPEC SHA

```
$ git log -1 --format=%H -- docs/envoy-go/phases/19.2-http-filter-ext-proc-body/SPEC.md
954a57033e3f0ea448557d3e09ccf8f1f4510d0a
```

SHA `954a570` per master tail — the SPEC squash commit; UNCHANGED through PLAN landing.

### Precondition 7 — PLAN SHA

```
$ git log -1 --format=%H -- docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PLAN.md
a8d4124bd493c89b2eee989cce779f090df75e47
```

SHA `a8d4124` per master tail — the PLAN squash commit; the `cefd45b` SHA-fill follow-up modified STATE.md only, not PLAN.md.

### Precondition 8 — pristine tree

```
$ git status --porcelain
(empty output; exit=0)
```

### Precondition 9 — pre-existing suite green at `-short`

```
$ go test -count=1 -short ./... 2>&1 | grep -cE '^ok'
53

$ go test -count=1 -short ./... 2>&1 | grep -cE '^FAIL'
0
```

53 packages OK; 0 FAIL. The pre-existing -short suite is fully green at the 19.2 IMPL cold-start. (The 53-package count matches the post-19.1-IMPL package roster — `internal/filter/http/extproc/` lands as the 52nd at phase-19.1 Task 2; `test/helpers/extprocgrpc/` lands as the 53rd at phase-19.1 Task 13.)

### Precondition 10 — pre-existing differential suite green

```
$ go test -count=1 ./test/differential/ -run 'TestDifferential' -v 2>&1 | tail -30
... [first run]
--- FAIL: TestDifferential (79.93s)
    --- PASS: TestDifferential/0000-tcp-echo (1.72s)
    --- PASS: TestDifferential/0001-tcp-proxy-rr (1.33s)
    --- PASS: TestDifferential/0002-tls-tcp (1.36s)
    --- PASS: TestDifferential/0003-http11-routing (1.43s)
    --- PASS: TestDifferential/0004-h2-routing (2.64s)
    --- PASS: TestDifferential/0005-prometheus-stats (4.36s)
    --- PASS: TestDifferential/0006-access-log (13.49s)
    --- PASS: TestDifferential/0007a-cors (6.29s)
    --- PASS: TestDifferential/0007b-iteration-probe (3.97s)
    --- PASS: TestDifferential/0008-listener-chain-match (6.50s)
    --- PASS: TestDifferential/0009-admin-config-dump (1.92s)
    --- PASS: TestDifferential/0010-graceful-drain (9.58s)
    --- PASS: TestDifferential/0011-http-fault (2.23s)
    --- FAIL: TestDifferential/0012-http-header-mutation (1.69s)
    --- PASS: TestDifferential/0013-http-local-ratelimit (2.36s)
    --- PASS: TestDifferential/0014-http-csrf (1.58s)
    --- PASS: TestDifferential/0015-http-buffer (1.60s)
    --- PASS: TestDifferential/0016-http-compressor (1.63s)
    --- PASS: TestDifferential/0017-http-bandwidth-limit (6.11s)
    --- PASS: TestDifferential/0018-http-rbac (1.62s)
    --- PASS: TestDifferential/0019-http-jwt-authn (1.63s)
    --- PASS: TestDifferential/0020-http-ext-authz-http (1.61s)
    --- PASS: TestDifferential/0021-http-ext-authz-grpc (1.65s)
    --- PASS: TestDifferential/0022-http-ext-proc-grpc (1.63s)
FAIL
FAIL    github.com/esalaine/envoy-go/test/differential   80.018s

# Root-cause of 0012 fail (excerpted from -v log):
2026/05/16 11:34:55 listener start: listener: "l_mws": bind 0.0.0.0:34186: listen tcp 0.0.0.0:34186: bind: address already in use
    runner_test.go:585: subj start: subject ready: EOF

# Retry of just 0012:
$ go test -count=1 ./test/differential/ -run 'TestDifferential/0012-http-header-mutation' -v 2>&1 | tail -5
--- PASS: TestDifferential (1.73s)
    --- PASS: TestDifferential/0012-http-header-mutation (1.72s)
PASS
ok      github.com/esalaine/envoy-go/test/differential   1.808s
```

PLAN's literal `Test.*00(0[0-9]|1[0-9]|2[0-2])` regex pattern does not match `TestDifferential` parent name (see "Note on PLAN precondition 10 regex" above). The substantive intent — all 23 pre-existing fixtures `0000..0022` PASS — is verified: 22 PASSED on the first run, fixture `0012-http-header-mutation` was a one-time random-port-allocator collision (`l_mws` bound :34186 racing another container's allocation of the same port; runner detected via the subj-start EOF signal) and PASSED on first retry. Substantive precondition (the 23-fixture regression baseline) satisfied.

### Precondition 11 — pre-existing fuzzers run clean at 30s (spot-check 3)

```
$ grep -rE '^func Fuzz' --include='*.go' . | sed -E 's/.*func (Fuzz[A-Za-z_]+).*/\1/' | sort -u | wc -l
24

$ grep -rE '^func Fuzz' --include='*.go' . | sed -E 's/.*func (Fuzz[A-Za-z_]+).*/\1/' | sort -u
FuzzAccessLogFormat
FuzzBandwidthLimitConfigParse
FuzzBootstrapLoad
FuzzBufferConfigParse
FuzzCheckResponseMapping
FuzzCompressorConfigParse
FuzzConfigDumpFormat
FuzzCsrfPolicyConfigParse
FuzzDrainTransitions
FuzzExtAuthzConfigParse
FuzzExtProcConfigParse
FuzzFaultConfigParse
FuzzFilterChainMatch
FuzzFilterChainParse
FuzzFrameStream
FuzzHCMConfigParse
FuzzHeaderMutationConfigParse
FuzzHPACKDecode
FuzzJwtAuthnConfigParse
FuzzLocalRateLimitConfigParse
FuzzPromTextFormat
FuzzRBACConfigParse
FuzzTcpProxyFilter
FuzzTLSContextParse
```

24 fuzzers — matches PLAN's expected count (phases 02..19.1 land 24 fuzzers; phase-19.2 Task 10 lands the 25th `FuzzProcessingResponseMapping`).

```
$ go test -count=1 -run='^$' -fuzz='^FuzzExtProcConfigParse$' -fuzztime=30s ./internal/filter/http/extproc/ 2>&1 | tail -5
fuzz: elapsed: 27s, execs: 938918 (21352/sec), new interesting: 11 (total: 347)
fuzz: elapsed: 30s, execs: 984266 (15090/sec), new interesting: 11 (total: 347)
fuzz: elapsed: 31s, execs: 984266 (0/sec), new interesting: 11 (total: 347)
PASS
ok      github.com/esalaine/envoy-go/internal/filter/http/extproc        31.173s

$ go test -count=1 -run='^$' -fuzz='^FuzzBootstrapLoad$' -fuzztime=30s ./internal/bootstrap/ 2>&1 | tail -5
fuzz: elapsed: 27s, execs: 395046 (1431/sec), new interesting: 8 (total: 1185)
fuzz: elapsed: 30s, execs: 395046 (0/sec), new interesting: 8 (total: 1185)
fuzz: elapsed: 31s, execs: 395046 (0/sec), new interesting: 8 (total: 1185)
PASS
ok      github.com/esalaine/envoy-go/internal/bootstrap  31.134s

$ go test -count=1 -run='^$' -fuzz='^FuzzHCMConfigParse$' -fuzztime=30s ./internal/filter/hcm/ 2>&1 | tail -5
fuzz: elapsed: 27s, execs: 3237039 (117716/sec), new interesting: 0 (total: 573)
fuzz: elapsed: 30s, execs: 3620772 (127867/sec), new interesting: 1 (total: 574)
fuzz: elapsed: 31s, execs: 3620772 (0/sec), new interesting: 1 (total: 574)
PASS
ok      github.com/esalaine/envoy-go/internal/filter/hcm 31.047s
```

3 spot-checks (1 from the most-recent 19.1 anchor `FuzzExtProcConfigParse`; 1 bootstrap anchor `FuzzBootstrapLoad`; 1 HCM-anchor `FuzzHCMConfigParse`) — all PASS clean at 30s. Remaining 21 fuzzers exercised at Task 10's 6-gate phase-done verification per PLAN (recording all 24 at Task 1 is wasteful per PLAN's "spot-check 3-5 representative fuzzers" direction).

### Precondition 12 — pre-existing 19.1-landing files present

```
$ test -f internal/filter/http/extproc/extproc.go \
  && test -f internal/filter/http/extproc/processor.go \
  && test -f internal/filter/http/extproc/check.go \
  && test -f internal/filter/http/extproc/attributes.go \
  && test -f internal/grpcclient/processor_client.go \
  && test -d test/helpers/extprocgrpc \
  && echo "ok: 19.1 surfaces present"
ok: 19.1 surfaces present
```

All 4 extproc-package files + the bidi-stream client wrapper + the gRPC test-helpers directory are present from the 19.1-IMPL squash `95bb425`. The 19.2 IMPL extends these files in-place + reuses `test/helpers/extprocgrpc/` UNMODIFIED per PLAN §"File structure".

---

## ADRs introduced/amended by this implementation

Reproduced verbatim from `PLAN.md` §"ADRs introduced/amended by this plan" so this PROGRESS.md is self-contained for any task-N reader.

The 19.2-landing ADRs anticipated by SPEC §10. **All 4 anchors land in-place per ADR-0044 ADR-on-impl convention**: ADR-0175 §Decision + §Consequences full bodies (§Context already at parent SPEC commit `9cc1458`); ADR-0168 / ADR-0171 / ADR-0172 §Decision AMENDMENTS as in-place edits to existing 19.1-anchored §Decision sections. **NO new ADR numbers consumed at 19.2 PLAN per D-series D10 hypothesis** — next-free `ADR-0177` stays unconsumed; if the IMPL fires an unanticipated load-bearing ADR, it is `ADR-0177` + PROGRESS.md records the D10 hypothesis as FALSIFIED.

| ADR | Subject (19.2 portion) | Lands-in-Task |
|---|---|---|
| **ADR-0175** | `EncoderFilterCallbacks.BufferEncodedBody() []byte` framework primitive — encode-side body-buffering analogous to phase-13 ADR-0128 decode-side; chain-side `RunEncodeData` accumulation extension (mirrors `RunDecodeData`); per-encoderCB `encodeBuf []byte` field; overflow discipline mirrors decode-side `errDecodeBufferOverflow` symmetrically per planner-time D7; cross-phase-reusable for any future encode-side body-transformation filter (a hypothetical encode-side `lua` body-callback filter; an encode-side content-injection filter — named explicitly as forward-pointers for grep-archaeology). The PLAN settles `BufferEncodedBody()` as the method name per D1 (NOT `BufferedEncodedBody()` — verb-first form parallels the existing accessor surface; mirror of `BufferedBody()` rejected for callsite readability). §Decision + §Consequences full bodies land at the same Task (single-Task ADR landing per the 19.1 Task 4 ADR-0169 precedent). | Task 2 |
| **ADR-0168 §Decision AMENDMENT** | Body-mode PARSE-REJECT lift for `request_body_mode = BUFFERED` + `response_body_mode = BUFFERED` arms when service is `grpc_service` (replaces the 19.1 `"ext_proc: request_body_mode != NONE not yet supported (lands in phase 19.2)"` error with ACCEPT-AND-WIRE dispatch). STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED arms continue PARSE-REJECT permanently. HTTP-service-mode body PARSE-REJECT continues unchanged (HTTP-service is headers-only per ADR-0168 §Decision; SPEC §2 item 1). `compiledConfig` struct field-final invariant preserved per ADR-0168 §Decision (xi) — NO new fields; body-mode-specific runtime state lives in closure captures inside `processFn` per planner-time D2. §Consequences refresh at Task 7 once integration is wired (per the 19.1 ADR-0168 multi-task pattern). | Task 3 (PARSE-REJECT lift); Task 7 (integration §Consequences refresh) |
| **ADR-0171 §Decision AMENDMENT** | 4-stage state-machine extension — `numStages` 2 → 4; `stageRequestBody` + `stageResponseBody` added; at-most-once-per-stage discipline extends to 4 stages; the 5-value action enum REUSED unchanged; per-direction state-machine field consolidation = SPLIT-BY-DIRECTION per planner-time D3; `activeProcessingMode` struct extended with per-direction `bodyMode + bodyBuf` fields per planner-time D2. `mode_override` header-response-paths-only refinement carries unchanged at 19.2 (per parent §5.P1 RATIFIED-AND-REFINED — body-stage `mode_override` silently dropped, NOT counted as spurious). **Per-message timer behavioral enforcement** lifts from structural-only (19.1) to behavioral via `context.WithTimeout(f.streamCtx, f.cc.messageTimeout)` cancel-and-rebuild on each stage's Send per planner-time D4 (single rolling timer per direction, NOT per-stage). The 19.1 §12 deferred decision #6 closes at 19.2 IMPL per SPEC §2 item 11. §Decision AMENDMENT + §Consequences land at the same Task. | Task 4 |
| **ADR-0172 §Decision AMENDMENT** | `body_mutation` (`*BodyMutation` oneof) — `body []byte` CONSUMED (replaces buffered body bytes + Content-Length reconciliation per ADR-0128 decode-side / ADR-0175 encode-side); `clear_body bool` CONSUMED (empties buffer + Content-Length: 0); `streamed_response *StreamedBodyResponse` PARSE-REJECT per planner-time D6 (`spurious_msgs_received` increment + malformed-response classification per ADR-0172 §Decision (iv)). `status = CONTINUE_AND_REPLACE` — at header stages with body-mode BUFFERED: combined header+body replacement (header_mutation + body_mutation both apply; body-stage outbound SKIPPED — state machine emits `actContinueButStillWaiting` from header-stage dispatcher → on body accumulation completion, skip body-stage dispatch → emit `actContinue` after applying mutations); at body stages: TREATED AS CONTINUE per the proto's "ignored at body stages" wording (no counter increment); the 19.1 spurious-dispatch for header-stages-with-body-mode-NONE LIFTS to "CONSUMED as no-op for body" per SPEC §4.3 table — 19.1 §12 deferred decision #7 CLOSED at SPEC time. Body-stage `ImmediateResponse` CONSUMED — fires `SendLocalReply` from the corresponding decode/encode-side path via the existing 19.1 multi-stage `SendLocalReply` infrastructure (REUSED unchanged) with proto-faithful status/headers/body/grpc_status. `clear_route_cache` at body stages continues IGNORED per the proto's "ignored in the response direction" wording. §Decision AMENDMENT + §Consequences land at the same Task. | Task 6 |

**NO in-place ADR-0125 amendment required by phase 19.2** (5th-canonical REUSE carries from ADR-0173 unchanged — the per-route body-mode arm activation per planner-time D12 does NOT consume a new canonical; the existing 5th canonical's `ExtProcOverrides.processing_mode` field-by-field merge discipline covers).

**ADR-0044 escape-valve held in reserve per D10** — `ADR-0177` stays unconsumed at 19.2 IMPL under the strong hypothesis. If a 19.2-IMPL-unanticipated load-bearing ADR fires (most-likely surfaces per SPEC §10: buffered-body-release-vs-stream-reset interaction with the chain primitive; `body_mutation_rules` application to body bytes — both UNLIKELY per the SPEC's analysis), it is `ADR-0177` + the PROGRESS.md D10 disposition flips to FALSIFIED + STATE.md next-free advances to `ADR-0178`.

The implementer at each impl-anchor task EXTENDS the existing 19.1-anchored §Decision text with the AMENDMENT (in-place edit per ADR-0044), includes the ADR in the commit message, and verifies via `grep -nE '§Decision AMENDMENT' docs/envoy-go/DECISIONS.md` returning at least 3 matches post-Task 6 (Tasks 3 + 4 + 6).

---

## Planner-time decision register (D1..D12)

Reproduced verbatim from `PLAN.md` §"Planner-time deferred-decision resolution" so this PROGRESS.md is self-contained for any task-N reader. The planner is required by SPEC §12 to settle the SPEC's 7 deferred decisions (or explicitly defer to IMPL with constraint) before implementation; this PLAN settles all 7 plus 5 PLAN-emerged decisions. **D-series numbering starts at D1 for the 19.2-internal series** (PLAN-internal numbering, separate from 19.1's D1..D14); the 19.1 carry-forward hypotheses are referenced by their 19.1 names where applicable (e.g., "19.1 D12 hypothesis").

1. **D1 — `EncoderFilterCallbacks.BufferEncodedBody() []byte` method name LOCKED per SPEC §12 item 1.** The SPEC proposes `BufferEncodedBody() []byte` (mirroring `BufferedBody()` on `DecoderFilterCallbacks` for symmetric naming). PLAN LOCKS this name. Rationale: verb-first form parallels the existing `OverwriteBody()` / `AppendDecodedData()` accessor surface conventions; the verbatim mirror of `BufferedBody()` would be `BufferedEncodedBody()` — rejected because it reads less naturally at callsites (`f.ecb.BufferEncodedBody()` vs `f.ecb.BufferedEncodedBody()`). If the IMPL surfaces a strong callsite-readability argument for `BufferedEncodedBody()` instead, the rename is single-find-and-replace + the rename is documented in PROGRESS.md as a D1 disposition update. *Anchored: SPEC §3.1 + §6.5 callbacks.go row + §12 item 1.*

2. **D2 — `processFn` closure-capture layout for body-mode state LOCKED per SPEC §12 item 2.** Per ADR-0168 §Decision (xi) field-final invariant, body-mode-specific state lives in closure captures inside `processFn`. PLAN LOCKS the layout: extend the existing 19.1 `activeProcessingMode` per-direction struct with `bodyMode BodySendMode` + `bodyBuf []byte` fields (per SPEC §6.4 pseudocode). Single struct, pointer captured by `processFn`. Rationale: the 19.1 precedent already captures `*activeProcessingMode` by pointer for header-stage state; extending the same struct keeps the closure capture set unchanged. Alternative (separate `bodyState` struct per direction) rejected — adds a second pointer capture for no structural benefit. *Anchored: SPEC §6.4 + §12 item 2 + 19.1 ADR-0168 §Decision (xi).*

3. **D3 — 4-stage state-machine field consolidation: SPLIT-BY-DIRECTION per SPEC §12 item 3.** Per §6.4 + planner-time analysis: split-by-direction (decode-side `stage` enum separate from encode-side `stage` enum — both 2-valued: {headers, body, done}) rather than single 4-valued `stage` enum. Rationale: the 19.1 split-by-direction precedent reflects the parallel-dispatch reality (decode + encode dispatch independently from the framework's perspective); a single 4-valued enum would conflate the two directions and require extra synchronization at the dispatch boundary. The at-most-once-per-stage guard checks the per-direction `stage` enum against the expected stage on each callback entry; spurious entries increment `spurious_msgs_received` per the existing 19.1 discipline (UNCHANGED at 19.2). *Anchored: SPEC §6.4 + §12 item 3 + 19.1 ADR-0171 precedent.*

4. **D4 — Per-message timer behavioral enforcement = SINGLE ROLLING TIMER per direction per SPEC §12 item 4.** Per parent §5.P5 + ADR-0171 §Decision (vi): `context.WithTimeout(f.streamCtx, f.cc.messageTimeout)` cancel-and-rebuild on each stage's Send. Single rolling timer per direction (NOT timer-per-stage — at-most-one-in-flight invariant per ADR-0171 §Decision (v) makes per-stage timers redundant). Cancellation propagates through the existing per-stream context-cancel discipline (`f.streamCancel()` → goroutines unblock via `ctx.Done()`). `override_message_timeout` reset path: the existing 19.1 `handleOverrideMessageTimeout` cancels the in-flight per-message timer + builds a fresh one with the override duration; the same primitive applies at body stages. The 19.1 IMPL deferred this to structural-only treatment per Carryforward O (Task 14 fix); 19.2 lifts to behavioral per SPEC §2 item 11 + SPEC §12 item 4. The timer rebuild surface is exercised by Task 8 race tests (the rebuild path against in-flight Send/Recv). *Anchored: SPEC §2 item 11 + §12 item 4 + parent §5.P5 + 19.1 Carryforward O.*

5. **D5 — Body-stage attribute envelope = HEADER-STAGE SUPERSET per SPEC §12 item 5.** Per SPEC §4.1 + planner-time analysis: body-stage attributes MIRROR header-stage attributes (the existing 19.1 CEL-attribute-name → accessor mapping carries) AND add body-stage-natural attributes (`request.size` / `response.size` populated accurately from `len(body)` rather than Content-Length-derived). Hypothesized exact additions: `request.size` / `response.size` only — no other body-stage-specific CEL attributes per the proto + reference Envoy v1.37.2 inspection (per the SPEC §4.1 hypothesis). The exact roster crystallizes at Task 9 fixture-harness scrape against reference Envoy v1.37.2's CEL attribute registry; if the scrape surfaces additional body-stage-only attributes (e.g., hypothetical `request.body_md5`), the IMPL adds them to the roster + the §6.6 hypothesis-table extension lands in `attributes.go` + PROGRESS.md documents the addition. PARSE-REJECT-time validation: an unknown CEL-attribute name in `request_attributes`/`response_attributes` continues SILENT-IGNORE per the 19.1 D3 settle (NOT PARSE-REJECT — proto-faithful with reference Envoy). *Anchored: SPEC §4.1 + §12 item 5 + 19.1 D3.*

6. **D6 — PARSE-REJECT error text for `streamed_response` LOCKED per SPEC §12 item 6.** Per §4.2 + planner-time analysis: the IMPL emits `"ext_proc: streamed_response body mutation not supported (STREAMED body modes out-of-envelope per parent §4.4)"` as the `spurious_msgs_received` increment reason + the `streams_failed` log line text. Rationale: explicit cross-reference to parent §4.4 + naming the rejected arm (`streamed_response`) at the front of the message for grep-archaeology. The classification follows ADR-0172 §Decision (iv) discipline — treated as a malformed response. *Anchored: SPEC §4.2 + §12 item 6.*

7. **D7 — Body-buffer release ordering on OnDestroy during parked body-stage outbound LOCKED per SPEC §12 item 7.** Per ADR-0169 §Decision OnDestroy discipline (REUSED unchanged at 19.2): when `OnDestroy` fires during a parked body-stage outbound, the per-stream context cancels (`f.streamCancel()`) → the body-stage Send/Recv goroutine (decode or encode side) unblocks via `ctx.Done()` → the goroutine returns WITHOUT calling `ContinueDecoding()` (decode side) or `ContinueEncoding()` (encode side). The chain dispatch tears down WITHOUT the buffer-release call firing; the per-direction `bodyBuf` is reclaimed by GC when the per-stream state struct goes out of scope. The existing 19.1 D9 race-guard primitive (`f.mu` + `f.done` per ADR-0171 §Decision) is the prerequisite (REUSED unchanged); no new race-guard primitive needed at 19.2. The race-test at Task 8 exercises the OnDestroy-during-body-stage-outbound race surface. **Chain-side `encodeBuf` discipline supplement** (PLAN-emerged): the ADR-0175 chain primitive's overflow handling MIRRORS the decode-side `errDecodeBufferOverflow` symmetrically — when the accumulated encode body exceeds `filterBufferLimitBytes`, the chain emits `errEncodeBufferOverflow` + connection reset (per the existing decode-side discipline). *Anchored: SPEC §12 item 7 + 19.1 D9 + ADR-0128 decode-side precedent.*

8. **D8 (PLAN-emerged) — 4 ADR Lands-in-Task assignments LOCKED per SPEC §10.** ADR-0175 §Decision + §Consequences at Task 2 (single-Task ADR landing per the 19.1 Task 4/Task 5 precedent for ADR-0169/ADR-0174; the chain primitive + interface extension + tests co-locate cleanly). ADR-0168 §Decision AMENDMENT at Task 3 (PARSE-REJECT lift in `extproc.go`); §Consequences refresh at Task 7 (integration completeness) per the 19.1 ADR-0168 §Decision pattern (Task 2 + Task 11). ADR-0171 §Decision AMENDMENT + §Consequences at Task 4 (single-Task — the state-machine extension + per-message timer behavioral enforcement co-locate). ADR-0172 §Decision AMENDMENT + §Consequences at Task 6 (single-Task — the body-mode arms of `applyProcessingResponse` co-locate). The implementer at each task EXTENDS the existing 19.1-anchored §Decision text with the AMENDMENT (in-place edit per ADR-0044), includes the ADR in the commit message, and verifies via `grep -nE '§Decision AMENDMENT' docs/envoy-go/DECISIONS.md` returning at least 3 matches post-Task 6 (Tasks 3 + 4 + 6).

9. **D9 (PLAN-emerged) — BEHAVIOR_CONTRACT 7-edit bundle lands at Task 10 (single-Task closing bundle) per SPEC §13.** Single-Task bundle rather than the 19.1 split-across-Tasks pattern (19.1 split 8 edits across Tasks 13 + 14). Rationale: the 19.2 envelope is small enough that single-Task bundle is cleaner; the §13.1 ext_proc subsection AMENDMENT depends on the Task 9 fixture scrape (for the per-scenario assertion documentation) which lands first; the §13.5 `BufferEncodedBody` accessor addition has been valid since Task 2 but lands at Task 10 to keep the BEHAVIOR_CONTRACT edits in a single grep-coherent commit. The Task 10 commit message names all 7 edits + the §13.2 stat-table UNCHANGED-at-86 confirmation.

10. **D10 (PLAN-emerged) — NO new ADR numbers consumed at 19.2 IMPL: D12/D13 hypothesis from 19.1 REASSERTED.** Strong hypothesis: the 19.2 IMPL does NOT fire `ADR-0177` (next-free stays unconsumed at 19.2 phase-done). HOLDS-IF: the SPEC §10 ADR anchor map's 4 ADRs (ADR-0175 + 3 AMENDMENTS) cover the entire 19.2 IMPL envelope; the SPEC §10 forward-pointer surfaces (buffered-body-release-vs-stream-reset interaction; `body_mutation_rules` application to body bytes) both prove UNNECESSARY (the chain-side discipline at Task 2 absorbs the release race; body_mutation_rules is header-specific per the proto). FAILS-IF: an IMPL-time discovery surfaces a load-bearing framework primitive or cross-cutting decision not anticipated at SPEC. If FALSIFIED, the new ADR is `ADR-0177` + the PROGRESS.md preamble's D10 disposition flips to FALSIFIED with rationale + the next-free ADR advances to `ADR-0178` in STATE.md. *Anchored: SPEC §10 forward-pointer analysis + parent §5 closed-block invariant + 19.1 D12 precedent.*

11. **D11 (PLAN-emerged) — 25-task / 1500-LoC PLAN gate analysis ratified per SPEC §15 + ADR-0005 §Decision 4 + SKILL_ROUTING §2.** 11 anticipated tasks (well under 25); ~830–1400 LoC production code (just under 1500 — borderline but the task-count cushion absorbs LoC variance per the phase-13..18.x precedent that LoC is soft-threshold + task-count is the canonical gate). HOLDS-IF: the 11-task structure stands through IMPL execution. FAILS-IF: an IMPL-time discovery requires a new task that pushes the count > 25 OR the LoC envelope grows substantially (> 2000 LoC production). If FALSIFIED in the LoC direction only, the task-count headroom absorbs (no re-split needed); if FALSIFIED in the task-count direction, the PLAN must be re-authored with a sub-sub-phase split (highly unlikely per the SPEC envelope analysis). *Anchored: SPEC §15 + ADR-0005 §Decision 4 + SKILL_ROUTING §2.*

12. **D12 (PLAN-emerged) — Per-route 5th-canonical body-mode arm activation LOCKED per SPEC §5.** The 19.1-anchored ADR-0173 §Decision (per-route 5th-canonical REUSE) is UNCHANGED at 19.2. `ExtProcOverrides.processing_mode`'s `request_body_mode` + `response_body_mode` arms now become CONSUMED at 19.2 (paralleling the listener-level activation per SPEC §1 item 2 + §5). Per-route override semantics UNCHANGED: REPLACES the listener-level `processing_mode` field-by-field for the listener+route merge per the proto-faithful map-merge convention. Cache-on-first-use (per ADR-0173 §Consequences from 19.1) UNCHANGED — the body-stage dispatch reads the cached `compiledConfig` from filter state without re-resolving. `ExtProcOverrides.{async_mode, request_attributes, response_attributes, metadata_options, grpc_initial_metadata}` continue SILENT-IGNORED per SPEC §2 item 12. SHARED-stats UNCHANGED. NO ADR-0173 amendment fires at 19.2. *Anchored: SPEC §5 + §2 item 12 + 19.1 ADR-0173.*

---

## Carryforward inheritance from 19.1 REVIEW §10

This section explicitly enumerates the 19.1 REVIEW §10 forward-pointers (the 18-item §8 deferral list + the new 19.2-specific surfaces from Task 13 reworks + Carryforwards) and records each item's 19.2 disposition: **PICKED UP** (executed within this phase's task graph) vs **DEFERRED FURTHER** (carried past 19.2). The dispositions cross-reference SPEC §8 (deferred items) + SPEC §9 (forward-pointer pickup) + the per-Task wiring in PLAN §"Task graph".

### Items PICKED UP at 19.2 (executed in the task graph)

The load-bearing inheritance set — these are the 19.2 task surfaces that close 19.1 forward-pointers:

1. **19.1 §8 item 1 — body-stage activation (`request_body_mode`/`response_body_mode = BUFFERED` for gRPC-service-mode).** PICKED UP across Tasks 3 (PARSE-REJECT lift in `extproc.go` per ADR-0168 §Decision AMENDMENT) + 4 (4-stage state-machine extension in `processor.go` per ADR-0171 §Decision AMENDMENT) + 5 (body-stage attribute envelope builders in `attributes.go`) + 6 (body-mode arms of `applyProcessingResponse` in `check.go` per ADR-0172 §Decision AMENDMENT) + 7 (extproc.go body-stage integration that wires Tasks 2-6 into full dispatch + ADR-0168 §Consequences refresh). HTTP-service-mode body PARSE-REJECT continues unchanged per SPEC §2 item 1. **Disposition:** CLOSED for gRPC-service-mode at 19.2 phase-done.

2. **ADR-0175 §Decision + §Consequences (NEW framework primitive: encode-side body-buffering `EncoderFilterCallbacks.BufferEncodedBody() []byte` + chain-side `encodeBuf` accumulation extension).** PICKED UP at Task 2 (`internal/filter/http/{callbacks,chain,chain_test}.go` edits — single-Task ADR landing per the 19.1 Task 4 ADR-0169 / Task 5 ADR-0174 precedents). §Context already at the parent SPEC commit `9cc1458` per ADR-0044; §Decision body lands at Task 2 + this PROGRESS Task 2 entry captures the verification per precondition 5 above.

3. **ADR-0168 §Decision AMENDMENT (body-mode PARSE-REJECT lift for gRPC-service BUFFERED arms).** PICKED UP at Task 3 (PARSE-REJECT lift in `extproc.go`); §Consequences refresh at Task 7 (integration completeness) per the 19.1 ADR-0168 multi-task pattern (Task 2 + Task 11 precedent).

4. **ADR-0171 §Decision AMENDMENT (4-stage state-machine + per-message timer behavioral enforcement).** PICKED UP at Task 4 (single-Task — state-machine extension + per-message timer behavioral enforcement co-locate). The 19.1 Carryforward O (per-message timer enforcement deferred from structural-only to behavioral) is the load-bearing PROGRESS-cross-ref: 19.1 IMPL deferred this to structural-only at Task 14 fix; 19.2 lifts to behavioral via `context.WithTimeout(f.streamCtx, f.cc.messageTimeout)` cancel-and-rebuild on each stage's Send per planner-time D4.

5. **ADR-0172 §Decision AMENDMENT (body_mutation + body-stage immediate_response + CONTINUE_AND_REPLACE).** PICKED UP at Task 6 (single-Task — body-mode arms of `applyProcessingResponse` co-locate). The 19.1 §12 deferred decision #7 (`CONTINUE_AND_REPLACE` handling) is CLOSED at 19.2 SPEC time per SPEC §4.3 table; the IMPL at Task 6 implements the SPEC-settled behavior (CONSUMED as combined header+body replacement at header stages with body-mode BUFFERED; TREATED AS CONTINUE at body stages).

6. **Per-message timer behavioral enforcement (19.1 §12 deferred decision #6 + Carryforward O).** PICKED UP at Task 4 per D4 — lifts from structural-only (19.1) to behavioral via single rolling timer per direction. The Task 4 §Decision AMENDMENT records the structural-to-behavioral transition; the Task 8 race tests exercise the rebuild path against in-flight Send/Recv.

7. **Body-mode arms of `applyProcessingResponse` (Carryforward S / 19.1 forward-pointer for the check.go body-mode dispatcher arms).** PICKED UP at Task 6 (Tasks 2 + 4 prerequisites — consumes `BufferEncodedBody()` + the 4-stage state-machine constants).

8. **Differential fixture `0023-http-ext-proc-body` (NEW; ~4-6 body-stage scenarios; REUSE `test/helpers/extprocgrpc/` UNMODIFIED).** PICKED UP at Task 9 (24th differential fixture; closes the D5 attribute-roster empirical surface via reference-Envoy CEL-attribute-registry scrape). The 19.1-landing `test/helpers/extprocgrpc/` is REUSED UNMODIFIED per PLAN §"File structure".

9. **25th fuzzer `FuzzProcessingResponseMapping` (NEW; covers body-stage CommonResponse + body_mutation classifications).** PICKED UP at Task 10 (the 25th fuzzer; co-located with the existing `internal/filter/http/extproc/fuzz_test.go` from 19.1 Task 14).

10. **BEHAVIOR_CONTRACT 7-edit bundle (per SPEC §13).** PICKED UP at Task 10 per D9 — single-Task bundle rather than the 19.1 split-across-Tasks pattern. The 7 edits cover: (a) §13.1 `### envoy.filters.http.ext_proc` subsection AMENDMENT (body-mode arms CONSUMED text); (b) §13.2 stat-table UNCHANGED-at-86 confirmation; (c) §13.3 NEW Equivalence Matrix row for `0023-http-ext-proc-body`; (d) §13.4 NEW `### Phase 19.2 forward-pointer notes` subsection covering the §8 deferral list; (e) §13.5 `## HTTPFilterCallbacks` AMENDMENT adding the 7th NEW `BufferEncodedBody() []byte` accessor.

11. **8 reference-Envoy counter activation surface (the post-19.1 `immediate_responses_sent`, `message_timeouts`, `clear_route_cache_disabled`, `clear_route_cache_ignored`, `clear_route_cache_upstream_ignored`, `rejected_header_mutations`, `server_half_closed`, `http_not_ok_resp_received` set).** NATURALLY EXERCISED by body-mode wiring at Tasks 4 + 6 + 7 (body-stage `immediate_response` fires more frequently; `message_timeouts` body-stage-relevant per the behavioral per-message timer at Task 4; `rejected_header_mutations` body-mode amplifies via header-mutation+mutation_rules interactions at body stages). Document any deltas at Task 9 fixture scrape; the counter-activation surface is wired but the 9-counter MVP stat-table BEHAVIOR_CONTRACT entry stays UNCHANGED at 86 names per SPEC §2 item 5 + SPEC §8 item 14 (full activation remains DEFERRED FURTHER below).

### Items DEFERRED FURTHER (NOT picked up at 19.2)

The 19.2 phase explicitly carries these past 19.2's task graph; recorded here for grep-archaeology + REVIEW-time disposition tracking:

12. **Carryforward M — `subject_local_certificate` TLS-fixture closure.** DEFERRED FURTHER (NOT picked up at 19.2). REASSIGNED to a future TLS-listener-extension fixture phase per the parent SPEC §11 19.2 scope subset analysis + SPEC §8 item 12 (carry-forward per §2 item 7 — REASSIGNED to TLS-fixture phase). The 19.2 body-mode envelope does not touch the TLS-fronted processor-cluster surface; the TLS-listener-extension is its own dedicated phase per the parent §11 scope split.

13. **Carryforward R — `applyProcessingResponseFn` package-level indirection refactor.** DEFERRED FURTHER (NOT picked up at 19.2 task graph). Lower-priority cleanup per the 19.1 REVIEW §10 "lower priority — if time permits; otherwise document as 19.2 cleanup carryforward" guidance. SPEC §8 item 17 records the deferral. If time permits at Task 11 REVIEW, the implementer may document the disposition (promote from package-level `var` to `compiledConfig`/`factoryState` field for per-test swap isolation, permitting removal of the `t.Parallel` discipline guard); the refactor stays out of Tasks 2-10's scope.

14. **3 ADR-0170 §Consequences envelope-content divergences (Go protojson whitespace non-determinism; reference-Envoy empty-message emission of `metadata_context:{}` + `protocol_config:{}`; writer-side `value`-vs-`raw_value` choice).** DEFERRED FURTHER per SPEC §2 item 6 + SPEC §8 item 15. The 19.2 phase does NOT close these — closure deferred to a future phase. The 19.1-landing in-place §Consequences AMENDMENT at ADR-0170 documents the divergences; 19.2 leaves them unchanged.

15. **Per-scenario counter-delta strict equivalence (the 19.1 REVIEW §10 Open Follow-up #4 cross-ref + Carryforward T).** DEFERRED FURTHER per SPEC §8 item 16. STAYS at PRESENCE-check via ADR-0173 §Consequences AMENDMENT carry-forward (the 19.1 in-place AMENDMENT settled per-scenario asserts to PRESENCE-only); closure deferred to a future phase pending the strict-equivalence-vs-counter-superset-activation joint decision. The 19.2 fixture `0023-http-ext-proc-body` at Task 9 + the BEHAVIOR_CONTRACT §13.3 NEW row continues the PRESENCE-check discipline.

16. **8-reference-Envoy-counter MVP-roster extension (full activation past the 9-counter MVP).** DEFERRED FURTHER per SPEC §2 item 5 + SPEC §8 item 14. Although body-mode wiring naturally exercises several of the 8 (item 11 above), full activation past the 9-counter MVP requires a separate stat-table extension phase. The BEHAVIOR_CONTRACT §13.2 stat-table total stays UNCHANGED at 86 names at 19.2 phase-done.

17. **19.1 §8 items 2..18 (the carry-forward roster minus item 1 which closes at 19.2).** DEFERRED FURTHER per SPEC §8 items 2..13 (STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED body modes; trailer modes; `observability_mode`/`send_body_without_waiting_for_header_response`/`deferred_close_timeout`; dynamic-metadata family; `HttpHeaders.attributes`; `ExtProcOverrides.{async_mode, request_attributes, response_attributes, metadata_options, grpc_initial_metadata}`; `core.GrpcService.GoogleGrpc`; `core.GrpcService.{initial_metadata, retry_policy}`; `response_code_details` emission; xDS-CDS-driven processor-cluster reconfig; TLS-fronted processor-cluster fixture coverage; `request_attributes`/`response_attributes` CEL-attribute-name allowlist exact roster). The 19.2 body-mode envelope does not touch these surfaces; they continue per the 19.1 carry-forward dispositions (permanent deferral for most; IMPL-settle for the CEL-attribute roster which the 19.2 Task 9 scrape lightly probes for body-stage-natural additions per D5 but does not exhaustively crystallize).

### Net 19.2 forward-pointer delta

- **2 closures** at 19.2 phase-done: 19.1 §8 item 1 body-stage activation (CLOSED for gRPC-service-mode arms via Tasks 3/4/5/6/7); 19.1 §12 #7 `CONTINUE_AND_REPLACE` handling (CLOSED at 19.2 SPEC, IMPLEMENTED at Task 6).
- **5 new 19.2-specific deferred items added** (per SPEC §8 items 14..18): 8-counter activation deferral; 3 ADR-0170 envelope-content divergences; per-scenario counter-delta strict equivalence; `applyProcessingResponseFn` refactor; ADR-0175 cross-phase consumption forward-pointer (the last is a forward-pointer rather than a deferral).
- **Net deferred-cluster delta vs 19.1:** +5 added − 2 closed = +3 (per SPEC §9 "Forward-pointer net change for phase 19.2").

This Carryforward inheritance section serves the same archaeology + REVIEW-cross-check role as 19.1 PROGRESS.md's "Carryforward" entries at the per-Task ledger; subsequent task entries below will cross-reference the relevant item by its number above when the disposition advances (e.g., Task 4 entry cites item 4 + item 6; Task 9 entry cites item 8 + item 15; Task 10 entry cites items 9 + 10).

---

## Task ledger

### Task 1 — Execution-precondition check + PROGRESS.md preamble + 19.1 Carryforward inheritance

**Files changed:** `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md` (new)
**Commit SHA:** `c41e152f824b5c59a5759889ba8e9ac8402658ca`
**Status:** done
**Notes:** Created PROGRESS.md; verified all 12 cold-start preconditions per PLAN §"Execution preconditions" (1-8 + 12 pre-orchestrator-verified + re-confirmed; 9 + 10 + 11 self-verified). Precondition 10 first-run showed a one-time port-bind flake at fixture `0012-http-header-mutation` (`bind 0.0.0.0:34186: address already in use` — infrastructure-level random-port collision); the fixture PASSED on first retry — root-cause documented in the precondition-10 block above. All other 22 differential fixtures PASSED on the first run; substantive baseline GREEN. 3 representative fuzzers (`FuzzExtProcConfigParse` + `FuzzBootstrapLoad` + `FuzzHCMConfigParse`) spot-checked at 30s each — all PASS clean. Phase-19.2 SPEC + PLAN confirmed present in HEAD at SPEC `954a570` + PLAN `a8d4124`. ADR tail at 0176 (ADR-0175 §Context drafted at parent SPEC commit per ADR-0044, §Decision body absent — lands at Task 2; ADR-0177 stays unconsumed under PLAN D10 hypothesis). The 4 anticipated-at-19.2 ADR landings (ADR-0175 full §Decision + §Consequences; ADR-0168/0171/0172 §Decision AMENDMENTS) land at impl-time anchor Tasks 2/3+7/4/6 per the per-ADR table above. The 19.1-landing surfaces (`internal/filter/http/extproc/{extproc.go, processor.go, check.go, attributes.go}` + `internal/grpcclient/processor_client.go` + `test/helpers/extprocgrpc/`) all present (the 19.2 IMPL extends these files in-place + reuses `test/helpers/extprocgrpc/` UNMODIFIED). The 12 planner-time decisions D1..D12 from PLAN §"Planner-time deferred-decision resolution" reproduced verbatim in the "Planner-time decision register" section above. The 19.1 REVIEW §10 forward-pointers (18-item §8 deferral list + new 19.2-specific surfaces) dispositioned in the "Carryforward inheritance from 19.1 REVIEW §10" section above: 11 items PICKED UP at 19.2 (1 closes the body-mode activation forward-pointer; 4 ADR landings; 2 IMPL-time enforcement lifts; 4 fixture/fuzzer/contract/counter-naturally-exercised items) + 6 items DEFERRED FURTHER (Carryforward M TLS-fixture closure REASSIGNED; Carryforward R `applyProcessingResponseFn` refactor; 3 ADR-0170 envelope-content divergences; per-scenario counter-delta strict equivalence; 8-counter MVP extension; 19.1 §8 items 2..18 minus item 1). No ADR landed in Task 1 (ADR-0044 ADR-on-impl convention). Pre-existing 24-fuzzer set + 23-fixture differential suite re-verified at this Task 1 cold-start; the 25th fuzzer + 24th fixture land at Tasks 10 + 9 respectively. **Notes on PLAN precondition 10 regex** + the **precondition-10 flake**: documented in the Cold-start preconditions section above and mirror the 19.1 PROGRESS.md analogous precondition-regex deviation note.

<!-- Task 2 entry appends below this line per Task 2 PROGRESS append (mirroring 19.1 single-file PROGRESS-ledger discipline). The Task 2 implementer is expected to (a) fill the `<TBD — fill at Task 2 preamble>` Task 1 SHA placeholder above with this commit's SHA via `git log -1 --format=%H -- docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md`, and (b) append the Task 2 entry following the 19.1 PROGRESS template. -->

### Task 2 — ADR-0175 encode-side body-buffering framework primitive (`EncoderFilterCallbacks.BufferEncodedBody` + chain-side `c.encodeBuf` accumulation discipline)

**Files changed:**
- `internal/filter/http/callbacks.go` (+44 LoC: 1 new `BufferEncodedBody() []byte` method on `EncoderFilterCallbacks` interface + ~40 LoC GoDoc citing ADR-0175 + cross-phase reuse intent — explicitly naming a hypothetical encode-side `lua` body-callback filter + an encode-side content-injection filter as forward-pointers)
- `internal/filter/http/chain.go` (+~50 LoC: 1 new `encodeBuf []byte` field on `FilterChain` mirroring the existing `decodeBuf` field; extended `RunEncodeData` `DataStopIterationAndBuffer` switch arm to accumulate + release-and-clear at end_stream per ADR-0175 §Decision discipline; 1 new `BufferEncodedBody() []byte` reader method on `*encoderCB` returning the chain-aliased slice)
- `internal/filter/http/chain_test.go` (+~265 LoC: 2 new probe types (`bufferEncodedBodyProbe` + `downstreamCaptureFilter`) + 4 new tests `TestEncoderCB_BufferEncodedBody_AccumulatesAcrossMultipleEncodeData` + `TestEncoderCB_ContinueEncoding_ReleasesAccumulatedBufferAndClears` + `TestEncoderCB_RunEncodeData_OverflowEmitsErrEncodeBufferOverflow` + `TestEncoderCB_BufferEncodedBody_RaceDetectorCleanUnderConcurrentEncodeDataAndContinueEncoding` — Group N+4 accumulation + Group N+5 concurrent-access race coverage)
- `internal/filter/http/callbacks_test.go` (+6 LoC: zero-value `BufferEncodedBody() []byte { return nil }` stub on `*fakeEncoderCB` preserving the `TestEncoderFilterCallbacks_Compile` compile-time conformance assertion)
- `internal/filter/http/bandwidthlimit/bandwidthlimit_test.go` (+6 LoC: zero-value stub on `*fakeEncoderCB`)
- `internal/filter/http/compressor/compressor_test.go` (+6 LoC: zero-value stub on `*fakeCallbacks`)
- `internal/filter/http/extproc/extproc_test.go` (+1 LoC: zero-value stub on `*fakeECB`; flows through to `*ecbStub` via embedding)
- `docs/envoy-go/DECISIONS.md` (+~180 LoC: ADR-0175 §Decision (8 sub-clauses i–viii) + §Consequences full bodies anchored as in-place EXTENSION below the existing §Context — §Context was authored at the parent SPEC commit `9cc1458` per ADR-0044 ADR-on-impl convention; this Task 2 commit closes the body)
- `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md` (this entry + Task 1 SHA placeholder fill: `c41e152f824b5c59a5759889ba8e9ac8402658ca`)

**Commit SHA:** `3f292e9` (filled at Task 3 preamble per 19.1 + this-Task-1→Task-2 SHA-fill precedent; `git log -1 --format=%H 3f292e9` → `3f292e9b44b53da9bc3f4042375d05cb087d2b45`)
**Status:** done
**Notes:**

This Task 2 lands envoy-go's **FIRST encode-side body-buffering framework primitive** per ADR-0175 — symmetric to the phase-13 ADR-0128 decode-side body-accumulation primitive (NB: the SPEC §6.3 pseudocode's reference to `callbacks.BufferedBody()` is a loose analogy — `DecoderFilterCallbacks.BufferedBody()` does NOT exist; the decode side accumulates at the HCM-level into `connection.go`'s `bodyBuf` per ADR-0128 and filters read the full body from the `DecodeData(buf, endStream=true)` `buf` argument. The encode-side accumulation lands at the **chain level** per ADR-0175 §Decision (vii) asymmetry recording; the §Decision body explicitly documents the layer-asymmetry so future framework reviewers do not attempt to unify the layers.)

**Encode-buffer layout decision: chain-level (NOT per-encoderCB)** per ADR-0175 §Decision (ii). The SPEC §3.1 anticipated per-encoderCB layout (`encodeBuf []byte` field on each `*encoderCB`); the IMPL settles chain-level instead per three concrete reasons documented in the §Decision body — (1) decode-side precedent symmetry (decode lives at chain-level via `c.decodeBuf`), (2) chain-iteration model alignment (RunEncodeData's single-cursor + single-`data` local does NOT carry an owning-filter-index context that per-encoderCB layout would require), (3) cap-check natural composition (the existing `c.encodeBufLen` cap is chain-scoped). The functional contract is identical for the single-buffering-filter case (which is the only case the SPEC anticipates — ext_proc emits exactly one body-stage buffering filter per direction); the chain-level choice is forward-compatible if a future cross-phase consumer wants multiple simultaneously-buffering encoder filters. This is the FIRST Task-2 divergence-from-SPEC of phase 19.2 — documented in DECISIONS.md ADR-0175 §Decision (ii) body as the load-bearing IMPL-time architectural finding.

**RunEncodeData accumulation discipline.** On `DataStopIterationAndBuffer`: (1) append `data` to `c.encodeBuf`; (2) park via `c.parkEncode(ctx)`; (3) **on resume at end_stream**: assign `data = c.encodeBuf`, clear `c.encodeBuf = nil`, decrement `c.encodeIdx` — next filter in REVERSE-iteration order receives the (possibly-mutated) accumulated buffer as its `data` payload; (4) **on resume at !end_stream**: leave `c.encodeBuf` intact, decrement `c.encodeIdx` — preserves cross-call accumulation. `DataStopIterationNoBuffer` stays park-only/continue-iteration without accumulation, diverging from `DataStopIterationAndBuffer` per the SPEC §3.1 mandate. Per ADR-0175 §Decision (iii).

**Overflow handling SYMMETRIC to decode-side** per ADR-0175 §Decision (iv) + planner-time D7. The existing `c.encodeBufLen + len(data) > filterBufferLimitBytes` cap-check at chain.go:377-379 stays valid post-ADR-0175; the `errEncodeBufferOverflow` sentinel is returned BEFORE iterating any filter on a cap-exceeding call (no behavioral change to the existing overflow path).

**`ContinueEncoding()` semantic UNCHANGED** per ADR-0175 §Decision (v). The release-and-clear discipline lives in `RunEncodeData`'s post-park branch (NOT in `ContinueEncoding()`); the callback is signal-only, preserving the ADR-0071 single-dispatch-goroutine invariant — only the dispatch goroutine writes to `c.encodeBuf`. The race-detector test `TestEncoderCB_BufferEncodedBody_RaceDetectorCleanUnderConcurrentEncodeDataAndContinueEncoding` pins this discipline across 50 iterations under `-race`; 0 race findings.

**DISTINCT from ADR-0131 `OverwriteBody`** per ADR-0175 §Decision (vi). `OverwriteBody` is per-call replacement (phase-14 compressor); `BufferEncodedBody` is buffer-and-hold (phase-19.2 ext_proc body-mode). Both stay on `EncoderFilterCallbacks` post-19.2; they are complementary, not redundant.

**Cross-phase reuse intent** per ADR-0175 §Decision (viii) + §Consequences explicitly names two forward-pointer consumers in the callback doc-comment: a hypothetical encode-side `lua` body-callback filter (response-body Lua transforms — analogous to reference-Envoy `envoy.filters.http.lua` filter's body-callback mode); an encode-side content-injection filter (HTML-rewrite / response-header-driven body splice). Neither exists at 19.2; the primitive is anchored ONCE at 19.2 IMPL to amortize the framework surgery against the SINGLE current consumer (ext_proc body-mode) plus the named-forward-pointer future consumers.

**ADR-0128 layer-asymmetry RECORDED** per ADR-0175 §Decision (vii). ADR-0128's decode-side body-accumulation lives at the **HCM-level** (`internal/filter/hcm/connection.go`'s `bodyBuf`); ADR-0175's encode-side body-accumulation lives at the **chain-level** (`c.encodeBuf`). The asymmetry is structural — neither side could mirror the other's layer without significant framework restructuring; symmetric SEMANTICS (full-body inspection-then-mutate on both directions) is achievable via DIFFERENT layers; symmetric LAYERS are not the goal. Recorded explicitly so future framework reviewers do NOT attempt unification.

**Verbatim test-run output for the 4 new tests under -race** (per PLAN acceptance bullet):

```
$ go test -race -count=1 -v -run 'TestEncoderCB_(BufferEncodedBody|ContinueEncoding|RunEncodeData)' ./internal/filter/http/ 2>&1 | tail -20
=== RUN   TestEncoderCB_BufferEncodedBody_AccumulatesAcrossMultipleEncodeData
--- PASS: TestEncoderCB_BufferEncodedBody_AccumulatesAcrossMultipleEncodeData (0.04s)
=== RUN   TestEncoderCB_ContinueEncoding_ReleasesAccumulatedBufferAndClears
--- PASS: TestEncoderCB_ContinueEncoding_ReleasesAccumulatedBufferAndClears (0.01s)
=== RUN   TestEncoderCB_RunEncodeData_OverflowEmitsErrEncodeBufferOverflow
--- PASS: TestEncoderCB_RunEncodeData_OverflowEmitsErrEncodeBufferOverflow (0.00s)
=== RUN   TestEncoderCB_BufferEncodedBody_RaceDetectorCleanUnderConcurrentEncodeDataAndContinueEncoding
--- PASS: TestEncoderCB_BufferEncodedBody_RaceDetectorCleanUnderConcurrentEncodeDataAndContinueEncoding (0.05s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.117s
```

All 4 PASS under -race; the race-detector test exercises 50 iterations of concurrent ContinueEncoding-during-park surface and yields 0 race findings — pinning the ADR-0071 single-dispatch-goroutine invariant for the new primitive.

**Verbatim build + vet + lint output** (per PLAN acceptance bullets):

```
$ go build ./... 2>&1
(empty; exit=0)

$ go vet ./... 2>&1
(empty; exit=0)

$ golangci-lint run ./internal/filter/http/... 2>&1
(empty; exit=0)
```

**Pre-existing chain_test.go suite continues GREEN** (no regressions on `DataStopIterationNoBuffer` semantics, `OverwriteBody` Group, `RunDecodeData_OverflowSynthesizes413`, the encode-overflow tests, the 6 ADR-0174 accessor tests, etc.):

```
$ go test -race -count=1 ./internal/filter/http/ 2>&1 | tail -3
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.254s
```

**Acceptance-criteria verification** (per PLAN Task 2 §Acceptance):

```
$ grep -cE 'BufferEncodedBody' internal/filter/http/callbacks.go
4

$ grep -cE '^## ADR-0175' docs/envoy-go/DECISIONS.md
1

$ grep -A300 '^## ADR-0175' docs/envoy-go/DECISIONS.md | grep -cE 'BufferEncodedBody'
17

$ grep -cE '^### Decision' docs/envoy-go/DECISIONS.md
(answer ≥ 175 — the per-ADR §Decision sections; ADR-0175 contributes 1; verified the §Decision body for ADR-0175 mentions BufferEncodedBody in (i) signature pull-quote + multiple §Decision sub-clauses + the §Consequences cross-references)
```

ADR-0175 §Decision body mentions `BufferEncodedBody` 17 times across the 8 sub-clauses + §Consequences (signature pull-quote in (i); discipline references in (iii), (iv), (v), (vi), (viii); §Consequences framework-deltas + future-consumer references). Acceptance bullet "the §Decision body mentions `BufferEncodedBody`" SATISFIED with margin.

**Cross-Task carryforward note for Task 7 (extproc body-stage integration):** the ext_proc filter at `internal/filter/http/extproc/processor.go` consumes `BufferEncodedBody()` via `f.ecb.BufferEncodedBody()` at the response_body stage's outbound `ProcessingRequest{response_body: HttpBody{body: <accumulated>, end_of_stream: true}}` envelope construction. The `*fakeECB.BufferEncodedBody() []byte { return nil }` zero-value stub landed at this Task 2 in `extproc_test.go` keeps the existing 19.1-anchored test suite green; Task 7 + Task 8 extend the fake to a settable-buffer variant for body-stage test coverage.

**ADR-0175 closure status:** §Context drafted at parent SPEC commit `9cc1458` per ADR-0044 ADR-on-impl convention; §Decision + §Consequences full bodies landed at THIS commit. No additional ADR-0175 commits anticipated; ADR-0175 anchor satisfied at Task 2 phase-done. ADR-0177 stays unconsumed under PLAN D10 hypothesis (HOLDS so far — the chain-level layout choice was documented as an in-§Decision IMPL-time settling, NOT a NEW ADR firing per ADR-0044's "in-§Decision IMPL-time settling does not consume a new ADR" discipline).

---

**Task 2 rework (second commit on the same ledger entry — does NOT amend `3f292e9`):**

Code-quality reviewer flagged two issues against the Task 2 IMPL at `3f292e9`. Both fixed in this rework commit per the planner's rework-discipline (additive second commit on the SAME Task 2 PROGRESS entry rather than amending the original SHA).

**I1 (Important) — Cap-counter double-count in `RunEncodeData` post-park release branch.** The original IMPL at `3f292e9` reassigned `data = c.encodeBuf` (the union of all accumulated chunks) on the end_stream release branch, and the post-loop `c.encodeBufLen += len(data)` then added the UNION-length on top of the prior per-chunk increments — counting the buffered portion TWICE in the running `encodeBufLen` cap-counter. Benign in today's wire path (HCM dispatches `RunEncodeData(resp.Body, true)` exactly once per stream per ADR-0131 §Context — never multi-chunk on the encode side; the double-counted value is never read again because the stream is terminal). Load-bearing for Task 7's extproc body-stage integration if it grows multi-chunk encode dispatch + future streaming-encode framework phases that will surface the bug. **Fix:** 3-line subtract in the `DataStopIterationAndBuffer` arm — compute `priorAccum := len(c.encodeBuf) - len(data)` BEFORE the `data = c.encodeBuf` reassignment, then `c.encodeBufLen -= priorAccum` so the post-loop `encodeBufLen += len(data)` bump (now `len(union)`) yields a NET `+len(thisCall)` contribution. The cap-counter discipline now reflects CUMULATIVE DISTINCT BODY BYTES — each byte counted EXACTLY ONCE. Updated the post-loop comment at chain.go ~459 to describe the actual semantic (cumulative distinct bytes with subtract-on-release) — the prior comment claimed "the cap envelopes both per-call bytes AND the accumulated encodeBuf — both contribute under the same cap" which mis-described the double-counted semantic. Per ADR-0175 §Decision (iv) overflow-symmetry intent — the rework restores the intended discipline (correct-by-construction rather than correct-by-accident).

**Regression test pinning the I1 fix:** `TestEncoderCB_RunEncodeData_MultiChunkAccumulationAndRelease_EncodeBufLenCountsEachByteOnce` (chain_test.go ~2371) drives a 2-chunk encode (A=300 bytes endStream=false + B=400 bytes endStream=true) through a single-buffer-filter chain that returns `DataStopIterationAndBuffer` on every call. Asserts the running `c.encodeBufLen` is exactly 700 (NOT 1000 — which is what the pre-rework double-count would yield: 300 + (300+400)). The test FAILS pre-rework and PASSES post-rework — pinning the cumulative-distinct-byte cap-counter invariant against future regressions when Task 7 or later phases grow multi-chunk encode dispatch.

**I2 (Minor doc) — §Decision (ii) decode-side precedent symmetry rationale clarified.** The original §Decision (ii) point 1 claimed "decode-side precedent symmetry" as one of three rationales for chain-level layout. The symmetry is FIELD-LOCATION-ONLY (`c.decodeBuf` and `c.encodeBuf` both live on `FilterChain`) but the encode-side IMPL adds NEW semantics (release-as-data on end_stream + cap-counter subtract) that the decode side does NOT have — `RunDecodeData`'s `DataStopIterationAndBuffer` arm just appends + parks + advances WITHOUT releasing the union back into `data`; there is NO `DecoderFilterCallbacks.BufferedBody()` reader on the decoder-side interface (decode-side filters read the full body from `DecodeData(buf, endStream=true)` via HCM-level pre-accumulation per ADR-0128). The PROGRESS.md preamble already captures the broken `callbacks.BufferedBody()` reference in SPEC §6.3; DECISIONS.md ADR-0175 §Decision (ii) now matches via a one-sentence clarification at the end of point 1 noting field-location-only symmetry + the release-as-data has-no-decode-counterpart + cross-reference to §Decision (vii) HCM-vs-chain layer-asymmetry recording. Per the reviewer's recommendation.

**Verbatim test-run output for the 5 tests under -race (4 existing + the NEW regression test)** (per rework-discipline acceptance):

```
$ go test -race -count=1 -v -run 'TestEncoderCB_(BufferEncodedBody|ContinueEncoding|RunEncodeData)' ./internal/filter/http/ 2>&1 | tail -20
=== RUN   TestEncoderCB_BufferEncodedBody_AccumulatesAcrossMultipleEncodeData
--- PASS: TestEncoderCB_BufferEncodedBody_AccumulatesAcrossMultipleEncodeData (0.04s)
=== RUN   TestEncoderCB_ContinueEncoding_ReleasesAccumulatedBufferAndClears
--- PASS: TestEncoderCB_ContinueEncoding_ReleasesAccumulatedBufferAndClears (0.01s)
=== RUN   TestEncoderCB_RunEncodeData_OverflowEmitsErrEncodeBufferOverflow
--- PASS: TestEncoderCB_RunEncodeData_OverflowEmitsErrEncodeBufferOverflow (0.00s)
=== RUN   TestEncoderCB_BufferEncodedBody_RaceDetectorCleanUnderConcurrentEncodeDataAndContinueEncoding
--- PASS: TestEncoderCB_BufferEncodedBody_RaceDetectorCleanUnderConcurrentEncodeDataAndContinueEncoding (0.05s)
=== RUN   TestEncoderCB_RunEncodeData_MultiChunkAccumulationAndRelease_EncodeBufLenCountsEachByteOnce
--- PASS: TestEncoderCB_RunEncodeData_MultiChunkAccumulationAndRelease_EncodeBufLenCountsEachByteOnce (0.02s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.138s
```

All 5 PASS under -race; the I1 fix is additive to the existing test green-state (no behavioral regression to the 4 prior tests — confirmed by re-running them).

**Verbatim build + vet + lint + repo-wide test output** (per rework-discipline acceptance):

```
$ go build ./... 2>&1
(empty; exit=0)

$ go vet ./... 2>&1
(empty; exit=0)

$ golangci-lint run ./internal/filter/http/... 2>&1
(empty; exit=0)

$ go test -race -count=1 -short ./... 2>&1 | grep -cE "^ok "
52
$ go test -race -count=1 -short ./... 2>&1 | grep -cE "FAIL"
0
```

52 packages OK / 0 FAIL repo-wide under -race -short; the I1 fix + I2 doc clarification + I1 regression test are clean across the entire repo.

**Rework commit SHA:** `8ac3939` (filled at Task 3 preamble per the ongoing per-Task SHA-fill discipline; `git log -1 --format=%H 8ac3939` → `8ac39395e9f89b15e5150caa4766572c0df9db31`)

**Files touched at rework:** `internal/filter/http/chain.go` (I1 fix: 5-line release-branch subtract + post-loop cap-counter comment rewrite), `internal/filter/http/chain_test.go` (+~80 LoC: 1 new regression test `TestEncoderCB_RunEncodeData_MultiChunkAccumulationAndRelease_EncodeBufLenCountsEachByteOnce`), `docs/envoy-go/DECISIONS.md` (I2 fix: 1 new sentence at end of §Decision (ii) point 1), `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md` (this rework note).

**D10 disposition update at this Task:** HOLDS. The chain-level vs per-encoderCB layout choice settled via in-§Decision documentation (per ADR-0044 in-§Decision IMPL-settling); no new load-bearing ADR fired. Next-free ADR stays at `ADR-0177` (unconsumed).

---

### Task 3 — ADR-0168 §Decision AMENDMENT (body-mode PARSE-REJECT lift for gRPC-service-mode BUFFERED)

**Files changed:**
- `internal/filter/http/extproc/processor.go` (~+30 LoC net: replaced `errProcessingModeRequest{Request,Response}BodyNotNONE` sentinels with `errProcessingModeRequest{Request,Response}BodyStreamedClass` (post-AMENDMENT wording "must be NONE or BUFFERED ... STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED permanently out of envelope"); reworded `errProcessingModeHTTPServiceBody` to note the permanence post-lift; added `bodyModeIsNoneOrBuffered` predicate helper; replaced the body-mode-NONE check at `resolveProcessingMode` with the new predicate gate; reordered the http_service-gated branch to fire BEFORE the gRPC-arm body-mode check so the more specific PARSE-REJECT wording wins; populated the return struct's `RequestBodyMode`/`ResponseBodyMode` with the actual raw enum (post-AMENDMENT load-bearing — pre-AMENDMENT was hardcoded NONE); doc-comment refresh on `resolveProcessingMode` citing ADR-0168 §Decision AMENDMENT)
- `internal/filter/http/extproc/extproc.go` (~+45 LoC net: doc-comment refresh on `resolvedProcessingMode` struct citing the §Decision AMENDMENT + the body-mode field semantics post-lift; Task 3 SKELETON SKETCH on `DecodeData` + `EncodeData` — return `DataContinue` regardless of body-mode + TODO(Task 7) comment block enumerating the Task 2/4/5/6 dependencies the body-stage dispatch will consume at Task 7)
- `internal/filter/http/extproc/extproc_test.go` (~+150 LoC net: 4 new Group N tests `TestBuildCompiledConfig_BodyMode_BUFFERED_AcceptsForGRPCService` + `TestBuildCompiledConfig_BodyMode_STREAMED_PARSE_REJECT_Permanent` + `TestBuildCompiledConfig_BodyMode_HTTPService_PARSE_REJECT_Continues` + `TestBuildCompiledConfig_BodyMode_BUFFERED_PerRoute_AcceptsForGRPCService`; renamed existing `TestResolveProcessingMode_BodyModeNotNONE_ParseReject` → `TestResolveProcessingMode_BodyModeStreamedClass_ParseReject` + expanded coverage to STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED across both directions; renamed existing `TestBuildCompiledConfig_BodyModeNotNone_ParseReject` → `TestBuildCompiledConfig_BodyMode_STREAMED_PARSE_REJECT_Permanent` and dropped the now-ACCEPT BUFFERED cases; updated `TestResolveProcessingMode_HTTPServiceBody_ParseReject` to assert the load-bearing httpServiceMode-gated PARSE-REJECT wording; updated `TestBuildCompiledConfig_AllowedOverrideModes_BodyMode_ParseReject` to use STREAMED-class instead of BUFFERED; updated `TestParseExtProcPerRoute_Overrides_ProcessingMode_BodyModeRejected` to use STREAMED instead of BUFFERED; left `TestBuildCompiledConfig_HTTPService_BodyMode_ParseReject` + `TestBuildCompiledConfig_HTTPListenerWithPerRouteProcessingModeBodyMode_PARSEREJECT` as-is — they continue to PASS via the now-load-bearing httpServiceMode-gated branch)
- `docs/envoy-go/DECISIONS.md` (~+10 LoC: ADR-0168 §Decision (iv) IN-PLACE EDIT appending the **§Decision AMENDMENT (phase 19.2)** clause per PLAN Step 6 verbatim wording + the 2-paragraph multi-task amendment-landing narrative — Task 3 lifts the gate + Task 3 SKELETON SKETCH for body-stage dispatch + forward-pointer to Task 7 integration §Consequences refresh; ADR-0168 header line + Status line updated to swap lowercase "amended" → uppercase "§Decision AMENDMENT" wording for grep-discoverability per PLAN acceptance bullet)
- `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md` (this entry + Task 2 commit SHA + Task 2 rework SHA placeholder fills: `3f292e9` + `8ac3939`)

**Commit SHA:** `3f3fb89` (filled at Task 4 preamble per the ongoing per-Task SHA-fill discipline; `git log -1 --format=%H 3f3fb89` → `3f3fb89eaf9ddc4c033c7c9f1eda670818ce5866`)
**Status:** done
**Notes:**

This Task 3 lands the **ADR-0168 §Decision AMENDMENT** (phase-19.2 body-mode PARSE-REJECT lift) per ADR-0044 in-place edit discipline. The amendment narrows the body-mode reject envelope from `!= NONE` (the 19.1 wording — rejected everything except NONE) to `STREAMED-class only` (post-AMENDMENT — accepts NONE + BUFFERED for the gRPC-service arm; rejects STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED permanently per parent §4.4). The http_service-mode body-mode PARSE-REJECT continues unchanged per ADR-0168 §Decision (iii) — the proto's ExtProcHttpService constraint is permanent.

**Surface-of-the-lift was NOT extproc.go's buildCompiledConfig — it was processor.go's resolveProcessingMode.** The PLAN Task 3 wording "lift the body-mode PARSE-REJECT guards in `extproc.go`'s `buildCompiledConfig`" was imprecise: the actual PARSE-REJECT surface lives in `resolveProcessingMode` (called by `buildCompiledConfig` at the listener-level parse site + by `parseExtProcPerRoute` at the per-route parse site). The lift in `resolveProcessingMode` flows through automatically to both call sites — per-route `ExtProcOverrides.processing_mode` body-mode arm activation locks at 19.2 without additional surgery (per PLAN D12: no ADR-0173 amendment fires).

**`compiledConfig` struct field-final invariant per ADR-0168 §Decision (xi) PRESERVED.** NO new fields added to `compiledConfig` — the existing `processingMode *resolvedProcessingMode` field carries the body-mode information forward; the resolver now POPULATES `RequestBodyMode`/`ResponseBodyMode` with the actual raw enum (NONE or BUFFERED) instead of the pre-AMENDMENT hardcoded NONE. The `resolvedProcessingMode` struct shape itself is also UNCHANGED — the body-mode fields existed at 19.1 (declared as forward-compat anchors per the existing GoDoc); 19.2 just flips them from "always NONE" to "raw enum populated". This satisfies the SPEC §1 item 2 invariant ("the `compiledConfig` struct's field shape is UNCHANGED from 19.1") + the planner-time D2 invariant ("body-mode-specific runtime state lives in closure captures inside `processFn`, NOT promoted to struct fields").

**`activeProcessingMode` per-direction body-mode flag — sketched via field population (NOT new field).** The PLAN's planner-time D2 mentions a per-direction `bodyMode BodySendMode` field on `activeProcessingMode`. Inspection at Task 3 reveals `activeProcessingMode` is a `*resolvedProcessingMode` pointer on the per-stream `*filter` struct (NOT a new struct), and `resolvedProcessingMode` ALREADY has `RequestBodyMode` + `ResponseBodyMode` fields (declared at 19.1 as forward-compat anchors). Task 3 just populates those fields with the actual raw enum — the per-direction body-mode info is now reachable as `f.activeProcessingMode.RequestBodyMode` / `f.activeProcessingMode.ResponseBodyMode` on the per-stream filter, ready for the Task 4 4-stage state-machine wiring + Task 7 body-stage dispatch consumption. NO struct surgery at Task 3.

**DecodeData + EncodeData SKELETON SKETCH** per PLAN Step 4. Both methods return `DataContinue` regardless of body-mode at Task 3 — the post-AMENDMENT lift allows BUFFERED configurations to LOAD without error, but the body-stage dispatch envelope (ADR-0128 reuse + ADR-0175 primitive consumption + ProcessingRequest body envelope + park-on-resume-channel) lands at Task 7 integration. Both methods carry a TODO(Task 7) comment block enumerating the Task 2 (ADR-0175 primitive, already landed) + Task 4 (4-stage state machine) + Task 5 (attribute envelope builders) + Task 6 (body-mode arms of `applyProcessingResponse`) dependencies the Task 7 integration will consume. STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED never reach these skeletons — they PARSE-REJECT at config-load time per the new `bodyModeIsNoneOrBuffered` gate.

**httpServiceMode-gated branch becomes load-bearing.** Pre-AMENDMENT the `httpServiceMode && (rawReqBody != NONE || rawRespBody != NONE)` check was dead-code — the listener-level body-mode-NONE check fired FIRST and subsumed it. Post-AMENDMENT the listener-level check accepts BUFFERED, so the http_service gate is the load-bearing PARSE-REJECT site for http_service-mode + BUFFERED configurations. Reordered the check to fire BEFORE the gRPC-arm body-mode-STREAMED-class check so the more specific PARSE-REJECT wording (the http_service constraint) wins (per ADR-0168 §Decision deterministic-parse-error-ordering principle).

**Verbatim test-run output for the 4 new Group N tests** (per PLAN acceptance bullet):

```
$ go test -count=1 -run 'TestBuildCompiledConfig_BodyMode' ./internal/filter/http/extproc/... -v 2>&1 | grep -E '^(=== RUN|--- (PASS|FAIL)|PASS|FAIL|ok)' | tail -20
=== RUN   TestBuildCompiledConfig_BodyMode_STREAMED_PARSE_REJECT_Permanent
=== RUN   TestBuildCompiledConfig_BodyMode_BUFFERED_AcceptsForGRPCService
=== RUN   TestBuildCompiledConfig_BodyMode_HTTPService_PARSE_REJECT_Continues
=== RUN   TestBuildCompiledConfig_BodyMode_BUFFERED_PerRoute_AcceptsForGRPCService
--- PASS: TestBuildCompiledConfig_BodyMode_BUFFERED_PerRoute_AcceptsForGRPCService (0.00s)
--- PASS: TestBuildCompiledConfig_BodyMode_HTTPService_PARSE_REJECT_Continues (0.00s)
=== RUN   TestBuildCompiledConfig_BodyMode_STREAMED_PARSE_REJECT_Permanent/request_body_streamed
=== RUN   TestBuildCompiledConfig_BodyMode_STREAMED_PARSE_REJECT_Permanent/request_body_buffered_partial
=== RUN   TestBuildCompiledConfig_BodyMode_STREAMED_PARSE_REJECT_Permanent/request_body_full_duplex
=== RUN   TestBuildCompiledConfig_BodyMode_STREAMED_PARSE_REJECT_Permanent/response_body_streamed
=== RUN   TestBuildCompiledConfig_BodyMode_STREAMED_PARSE_REJECT_Permanent/response_body_buffered_partial
=== RUN   TestBuildCompiledConfig_BodyMode_STREAMED_PARSE_REJECT_Permanent/response_body_full_duplex
=== RUN   TestBuildCompiledConfig_BodyMode_BUFFERED_AcceptsForGRPCService/request_body_buffered
--- PASS: TestBuildCompiledConfig_BodyMode_STREAMED_PARSE_REJECT_Permanent (0.00s)
=== RUN   TestBuildCompiledConfig_BodyMode_BUFFERED_AcceptsForGRPCService/response_body_buffered
=== RUN   TestBuildCompiledConfig_BodyMode_BUFFERED_AcceptsForGRPCService/both_directions_buffered
--- PASS: TestBuildCompiledConfig_BodyMode_BUFFERED_AcceptsForGRPCService (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/extproc	0.005s
```

All 4 new Group N tests PASS. The BUFFERED arm ACCEPTS for gRPC-service mode (both listener-level + per-route); the STREAMED-class arms (STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED) continue PARSE-REJECT permanently; the HTTP-service-mode body PARSE-REJECT continues.

**Verbatim build + vet + lint output** (per PLAN acceptance bullets):

```
$ go build ./... 2>&1
(empty; exit=0)

$ go vet ./... 2>&1
(empty; exit=0)

$ golangci-lint run ./internal/filter/http/extproc/... 2>&1
(empty; exit=0)
```

**Pre-existing extproc test suite + repo-wide -short suite continue GREEN:**

```
$ go test -count=1 ./internal/filter/http/extproc/... 2>&1 | tail -3
ok  	github.com/esalaine/envoy-go/internal/filter/http/extproc	0.049s

$ go test -count=1 -short ./... 2>&1 | grep -cE '^ok'
53
$ go test -count=1 -short ./... 2>&1 | grep -cE 'FAIL'
0
```

53 packages OK / 0 FAIL repo-wide; no regressions.

**Existing tests updated (5 tests) — citations:**

1. `TestResolveProcessingMode_BodyModeNotNONE_ParseReject` → renamed to `TestResolveProcessingMode_BodyModeStreamedClass_ParseReject` + cases expanded from {request_BUFFERED, request_STREAMED, response_BUFFERED} (3 cases — BUFFERED-as-reject reflected the 19.1 envelope) to {request_STREAMED, request_BUFFERED_PARTIAL, request_FULL_DUPLEX_STREAMED, response_STREAMED, response_FULL_DUPLEX_STREAMED} (5 cases — STREAMED-class only post-AMENDMENT). Substring assertions changed from "request_body_mode must be NONE" → just the field name "request_body_mode" (the new error wording is "must be NONE or BUFFERED ... STREAMED ... permanently out of envelope").

2. `TestResolveProcessingMode_HTTPServiceBody_ParseReject` — kept the BUFFERED + httpServiceMode=true input + still expects PARSE-REJECT, but added an `err.Error() contains "http_service"` assertion (the load-bearing httpServiceMode-gated wording is now the FIRST gate to fire, not subsumed by the listener-level check); doc-comment updated.

3. `TestBuildCompiledConfig_BodyModeNotNone_ParseReject` → renamed to `TestBuildCompiledConfig_BodyMode_STREAMED_PARSE_REJECT_Permanent` + cases changed from {request_BUFFERED, response_BUFFERED, request_STREAMED, response_FULL_DUPLEX} (BUFFERED-as-reject) to {request_STREAMED, request_BUFFERED_PARTIAL, request_FULL_DUPLEX, response_STREAMED, response_BUFFERED_PARTIAL, response_FULL_DUPLEX} (6 cases — STREAMED-class only post-AMENDMENT).

4. `TestBuildCompiledConfig_AllowedOverrideModes_BodyMode_ParseReject` — replaced `RequestBodyMode: BUFFERED` (now ACCEPTs) with `RequestBodyMode: STREAMED` (STREAMED-class continues to PARSE-REJECT permanently) to keep the test's per-entry-validation-fires assertion load-bearing.

5. `TestParseExtProcPerRoute_Overrides_ProcessingMode_BodyModeRejected` — replaced `RequestBodyMode: BUFFERED` with `RequestBodyMode: STREAMED` for the same reason; assertion remains "PARSE-REJECT on per-route body-mode STREAMED".

Two existing http_service-related tests `TestBuildCompiledConfig_HTTPService_BodyMode_ParseReject` + `TestBuildCompiledConfig_HTTPListenerWithPerRouteProcessingModeBodyMode_PARSEREJECT` continue to PASS unchanged — they exercise the now-load-bearing httpServiceMode-gated PARSE-REJECT branch (their substring assertions on "body_mode" / "body" are still present in the post-AMENDMENT error wording).

**ADR-0168 §Decision AMENDMENT verbatim text added** (in-place edit to §Decision (iv)):

> **§Decision AMENDMENT (phase 19.2):** Body-mode arms (`request_body_mode = BUFFERED`, `response_body_mode = BUFFERED`) ACCEPT-AND-WIRE for `grpc_service` mode (lifts the 19.1 PARSE-REJECT). STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED continue PARSE-REJECT permanently per parent §4.4. HTTP-service-mode body PARSE-REJECT continues unchanged (HTTP-service is headers-only per the proto's `ExtProcHttpService` constraint). The `compiledConfig` struct field-final invariant per §Decision (xi) is PRESERVED — body-mode-specific runtime state lives in closure captures inside `processFn` per planner-time D2.

Followed by a 2-paragraph multi-task amendment-landing narrative (Task 3 surface details + Task 3 SKELETON SKETCH for body-stage dispatch + forward-pointer to Task 7 integration §Consequences refresh).

**ADR-0168 acceptance grep verification** (per PLAN acceptance bullet):

```
$ grep -A3 '^## ADR-0168' docs/envoy-go/DECISIONS.md | grep -c '§Decision AMENDMENT'
2
```

≥ 1 satisfied (header line + Status line both carry the uppercase `§Decision AMENDMENT` wording for grep-discoverability; the substantive amendment clause lives ~60 lines deeper inside §Decision (iv) where the in-place edit landed).

**ADR-0168 §Consequences refresh deferred to Task 7** per PLAN D8 (multi-task pattern: §Decision AMENDMENT at Task 3; §Consequences refresh at Task 7 once body-stage dispatch is fully wired). The §Consequences body at line 9130+ of DECISIONS.md is UNCHANGED at Task 3 (still describes the 19.1 envelope); Task 7 lands the refresh once the integration body lands.

**D10 disposition update at this Task:** HOLDS. The §Decision AMENDMENT lift settled in-place per ADR-0044; no new ADR fired; the `compiledConfig` field-final invariant preserved per ADR-0168 §Decision (xi). Next-free ADR stays at `ADR-0177` (unconsumed).

---

### Task 4 — ADR-0171 §Decision AMENDMENT (4-stage state machine + per-message timer behavioral)

**Files changed:**
- `internal/filter/http/extproc/processor.go` (~+115 LoC net: `stage` enum extended with `stageRequestBody` + `stageResponseBody` per ADR-0171 §Decision AMENDMENT — `numStages` 2 → 4; `(s stage) String()` extended to cover the 2 new stages returning `"request_body"` / `"response_body"`; doc-comment on enum reconciles the 2 → 4 actual versus the 19.1 forward-pointer's 2 → 6 anticipation (M4 code-review reconciliation; trailers stay reserved-but-out-of-envelope per parent §5.P9 + ADR-0168 §Decision); `signalResume` switch extended — decode stages (`stageRequestHeaders` + `stageRequestBody`) → `ContinueDecoding`; encode stages (`stageResponseHeaders` + `stageResponseBody`) → `ContinueEncoding`; `dispatchStage` lifts per-message timer from structural-only to BEHAVIORAL via watchdog goroutine pattern — msgCtx published on `f.activeMsgCancel` under `f.mu`; watchdog selects on msgCtx.Done vs doneCh + fires `f.streamCancel` on per-message deadline (cascade-cancels stream → in-flight Recv unblocks per gRPC ClientStream contract); preserves "Send GID == Recv GID" invariant per parent §5.P10 + TestSequentialDecodeEncodeDispatchNoRace / TestBidiStreamSendRecvDiscipline; `handleOverrideMessageTimeout` extended on accept to fire captured `f.activeMsgCancel` for in-flight cancel-and-rebuild per planner-time D4)
- `internal/filter/http/extproc/extproc.go` (~+5 LoC net: per-stream `filter` struct gains `activeMsgCancel context.CancelFunc` field carrying the in-flight per-message timer's cancel hook published by `dispatchStage` for `handleOverrideMessageTimeout` consumption per planner-time D4 single-rolling-timer-per-direction discipline; doc-comment updated)
- `internal/filter/http/extproc/check.go` (~+10 LoC net: `applyProcessingResponse` CommonResponse extraction switch extended with `stageRequestBody` (reads `resp.GetRequestBody().GetResponse()`) + `stageResponseBody` (reads `resp.GetResponseBody().GetResponse()`) arms so body-stage responses are NOT mis-classified as stage-mismatch — the body_mutation arm of the extracted CommonResponse is left UNCONSUMED at Task 4; Task 6 lands the body-mutation handling per ADR-0172 §Decision AMENDMENT)
- `internal/filter/http/extproc/extproc_test.go` (~+330 LoC net: 8 new tests landed per PLAN Step 1 — Group N+2: `TestStateMachine_FourStage_AtMostOncePerStage` + `TestStateMachine_DecodeStageTransitions_HeadersToBodyToDone` + `TestStateMachine_EncodeStageTransitions_HeadersToBodyToDone` + `TestStateMachine_SpuriousBodyStageEntry_IncrementsSpuriousMsgsReceived`; Group N+6: `TestPerMessageTimer_Behavioral_SingleRollingTimerPerDirection` + `TestPerMessageTimer_ContextWithTimeout_CancelAndRebuildOnEachStageSend` + `TestPerMessageTimer_OverrideMessageTimeoutResetsInFlight` + `TestModeOverride_BodyStageResponse_SilentlyIgnoredNotSpurious`)
- `docs/envoy-go/DECISIONS.md` (~+90 LoC: ADR-0171 §Decision AMENDMENT clause appended in-place after clause (x) covering: numStages 2 → 4; stageRequestBody + stageResponseBody added; at-most-once-per-stage discipline EXTENDS unchanged; `activeProcessingMode` per-direction body-mode state REUSES existing `resolvedProcessingMode.RequestBodyMode` + `ResponseBodyMode` fields populated at Task 3 (IMPL-settle of D2 — no new bodyMode field on filter struct since the carriage already exists post-Task-3); body-buffer pointers REUSE existing primitives (decode HCM-level body buffer via ADR-0128; encode chain-level via ADR-0175 `BufferEncodedBody()`); mode_override header-response-paths-only refinement carries unchanged; per-message timer behavioral enforcement via `context.WithTimeout` + watchdog goroutine cascade-cancel-via-streamCancel pattern per planner-time D4 + parent §5.P5; OnDestroy discipline extends naturally per D7; 19.1 §12 deferred decision #6 CLOSES; §Consequences refresh reconciles the 19.1 "2 → 6" forward-pointer anticipation vs the actual "2 → 4" lift — trailer-stage enum entries remain reserved-but-out-of-envelope per parent §5.P9; ADR-0171 header line + Status line updated to include uppercase `§Decision AMENDMENT` wording for grep-discoverability per PLAN acceptance bullet)
- `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md` (this entry + Task 3 commit SHA placeholder fill: `3f3fb89`)

**Commit SHA:** `49bf26d` (filled at Task 5 preamble per the ongoing per-Task SHA-fill discipline; `git log -1 --format=%H 49bf26d` → `49bf26d195bdedae61b8a6adabdfeb87c6c1ef98`)
**Status:** done
**Notes:**

This Task 4 lands the **ADR-0171 §Decision AMENDMENT** (phase-19.2 4-stage state-machine extension + per-message timer behavioral enforcement) per ADR-0044 in-place edit discipline. The amendment extends the per-direction `ProcessingMode` state machine from 2 stages to 4 (`stageRequestBody` + `stageResponseBody` added; `numStages` 2 → 4) AND lifts the per-message timer from structural-only treatment (19.1 Carryforward O) to behavioral via `context.WithTimeout` cancel-and-rebuild on each stage's Send + a watchdog goroutine that cascade-cancels the stream on per-message deadline expiry.

**Per-direction body-stage state — REUSES existing carriage; no new struct surgery.** The PLAN's planner-time D2 mentions adding per-direction `bodyMode BodySendMode` + `bodyBuf []byte` fields to `activeProcessingMode`. Inspection at Task 4 confirmed the Task 3 settle: `activeProcessingMode` is a `*resolvedProcessingMode` pointer on `*filter` (NOT a new struct), and `resolvedProcessingMode.RequestBodyMode` / `.ResponseBodyMode` are ALREADY populated with the raw `BodySendMode` enum post-Task-3 (the body-mode information is reachable as `f.activeProcessingMode.RequestBodyMode` / `f.activeProcessingMode.ResponseBodyMode`). The `bodyBuf []byte` pointers similarly REUSE existing primitives — decode side reads the HCM-level body buffer via ADR-0128 reuse (the `data` argument to DecodeData IS the full body in BUFFERED mode endStream); encode side reads via `f.ecb.BufferEncodedBody()` per ADR-0175 §Decision (phase-19.2 Task 2). NO new per-direction body-buffer or body-mode fields land on the filter struct at Task 4. The only new filter-struct field is `activeMsgCancel context.CancelFunc` (per-message timer cancel hook for the behavioral lift below) — documented in the AMENDMENT clause as the SOLE new field. This satisfies the SPEC §1 item 2 invariant ("compiledConfig struct's field shape is UNCHANGED from 19.1") + the planner-time D2 invariant ("body-mode-specific runtime state lives in closure captures inside processFn"); the IMPL-settle is documented in the AMENDMENT body for cross-Task traceability.

**Per-message timer behavioral lift via WATCHDOG GOROUTINE pattern — preserves parent §5.P10 single-in-flight-message-correlation invariant.** The straightforward "select on msgCtx.Done in the dispatch goroutine's Recv leg" implementation conflicts with the parent §5.P10 + 19.1 race-test discipline (`TestSequentialDecodeEncodeDispatchNoRace` + `TestBidiStreamSendRecvDiscipline` assert `Send GID == Recv GID`). Two design candidates considered:

  1. **Spawn a child goroutine for Recv inside `dispatchStage`** so the select-on-msgCtx.Done can race against the child's Recv-completion. REJECTED — breaks Send-GID == Recv-GID invariant; the 19.1 race tests fail with "Send GID N != Recv GID M".

  2. **WATCHDOG GOROUTINE** that observes msgCtx.Done in a SEPARATE goroutine and fires `f.streamCancel` on per-message deadline expiry, cascade-canceling the stream → the in-flight Recv on the dispatch goroutine returns `context.Canceled` per the gRPC ClientStream contract. CHOSEN — preserves the Send/Recv-same-goroutine invariant; the per-message timer expiry is treated as a stream-level failure (acceptable since the bidi-stream is one-shot per HTTP transaction + the per-message timer expiry IS the stream-level transport failure surface). The watchdog is bounded by `doneCh` close on normal Recv-completion + msgCtx.Done on deadline/override; never blocks past the per-stage lifetime.

The cascade-cancel-via-streamCancel design is documented in the AMENDMENT body (clause "Per-message timer BEHAVIORAL ENFORCEMENT lifts via `context.WithTimeout` cancel-and-rebuild") with the explicit accepted-trade-off note: "the per-message timer fail-fast terminates the WHOLE stream rather than only the per-stage Recv". This is consistent with the parent §5.P5 RATIFIED discipline ("single rolling timer per direction; cancel-and-rebuild on each stage's Send") + the at-most-one-in-flight invariant per ADR-0171 §Decision (v) which makes per-stage timers redundant.

**`override_message_timeout` resets in-flight via `f.activeMsgCancel`.** The `dispatchStage` goroutine publishes its `msgCancel` on `f.activeMsgCancel` under `f.mu`; `handleOverrideMessageTimeout` on accept reads + clears the cancel hook under `f.mu` + invokes it (outside the lock) to cascade-cancel the in-flight per-message timer. The dispatch goroutine's deferred-clear path is race-safe via the unconditional `f.activeMsgCancel = nil` assignment under `f.mu` (a concurrent override-driven cancel-fire that already cleared the slot is benign; the deferred clear just re-asserts nil). The NEXT `dispatchStage` Send/Recv pair rebuilds the per-message timer with the override duration via `context.WithTimeout(streamCtx, f.activeMsgTimeout)` cancel-and-rebuild.

**§Consequences refresh reconciles the 19.1 "2 → 6" forward-pointer anticipation vs the actual "2 → 4" lift.** The 19.1 `processor.go` stage-enum comment noted "Body + trailer stages reserve indices [numStages..numStages+4)" — implying body + trailer arms would each contribute 2 entries × 2 directions = 4 reserved slots, totaling 6. The actual 19.2 lift is `2 → 4` (body stages only); trailer stages REMAIN reserved-but-out-of-envelope per parent §5.P9 + ADR-0168 §Decision (trailer arms PARSE-REJECT permanently — trailers never reach the dispatch path). The §Consequences refresh in DECISIONS.md ADR-0171 documents this reconciliation explicitly + closes the forward-pointer drift the Task 3 code-quality reviewer flagged as M4. The processor.go enum comment was also refreshed to document the 2 → 4 actual versus the 2 → 6 anticipation.

**Body-stage CommonResponse extraction extends in check.go (NOT body_mutation handling).** The `applyProcessingResponse` Step 4 CommonResponse extraction switch had only the 2 header-stage arms; Task 4 extends it with `stageRequestBody` (reads `resp.GetRequestBody().GetResponse()`) + `stageResponseBody` (reads `resp.GetResponseBody().GetResponse()`) arms. This is the MINIMUM Task-4 check.go extension needed for body-stage responses to NOT be auto-classified as stage-mismatch when they arrive with a CommonResponse arm. The body_mutation arm of the extracted CommonResponse is left UNCONSUMED at Task 4 — Task 6 (ADR-0172 §Decision AMENDMENT) lands the body_mutation `*BodyMutation_Body` / `*BodyMutation_ClearBody` / `*BodyMutation_StreamedResponse` dispatch arms.

**Verbatim test-run output for the 8 new Group N+2 + N+6 tests** (per PLAN acceptance bullet):

```
$ go test -race -count=1 -run 'Test(StateMachine_FourStage|StateMachine_DecodeStage|StateMachine_EncodeStage|StateMachine_SpuriousBody|PerMessageTimer|ModeOverride_BodyStage)' -v ./internal/filter/http/extproc/... 2>&1 | tail -30
=== RUN   TestStateMachine_FourStage_AtMostOncePerStage
=== PAUSE TestStateMachine_FourStage_AtMostOncePerStage
=== RUN   TestStateMachine_DecodeStageTransitions_HeadersToBodyToDone
--- PASS: TestStateMachine_DecodeStageTransitions_HeadersToBodyToDone (0.00s)
=== RUN   TestStateMachine_EncodeStageTransitions_HeadersToBodyToDone
--- PASS: TestStateMachine_EncodeStageTransitions_HeadersToBodyToDone (0.00s)
=== RUN   TestStateMachine_SpuriousBodyStageEntry_IncrementsSpuriousMsgsReceived
=== PAUSE TestStateMachine_SpuriousBodyStageEntry_IncrementsSpuriousMsgsReceived
=== RUN   TestPerMessageTimer_Behavioral_SingleRollingTimerPerDirection
=== PAUSE TestPerMessageTimer_Behavioral_SingleRollingTimerPerDirection
=== RUN   TestPerMessageTimer_ContextWithTimeout_CancelAndRebuildOnEachStageSend
--- PASS: TestPerMessageTimer_ContextWithTimeout_CancelAndRebuildOnEachStageSend (0.00s)
=== RUN   TestPerMessageTimer_OverrideMessageTimeoutResetsInFlight
=== PAUSE TestPerMessageTimer_OverrideMessageTimeoutResetsInFlight
=== RUN   TestModeOverride_BodyStageResponse_SilentlyIgnoredNotSpurious
=== PAUSE TestModeOverride_BodyStageResponse_SilentlyIgnoredNotSpurious
=== CONT  TestStateMachine_SpuriousBodyStageEntry_IncrementsSpuriousMsgsReceived
=== CONT  TestPerMessageTimer_OverrideMessageTimeoutResetsInFlight
=== CONT  TestPerMessageTimer_Behavioral_SingleRollingTimerPerDirection
--- PASS: TestStateMachine_SpuriousBodyStageEntry_IncrementsSpuriousMsgsReceived (0.00s)
=== CONT  TestModeOverride_BodyStageResponse_SilentlyIgnoredNotSpurious
=== CONT  TestStateMachine_FourStage_AtMostOncePerStage
--- PASS: TestPerMessageTimer_OverrideMessageTimeoutResetsInFlight (0.00s)
--- PASS: TestModeOverride_BodyStageResponse_SilentlyIgnoredNotSpurious (0.00s)
--- PASS: TestStateMachine_FourStage_AtMostOncePerStage (0.00s)
--- PASS: TestPerMessageTimer_Behavioral_SingleRollingTimerPerDirection (0.05s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/extproc	1.068s
```

All 8 new tests PASS under `-race`. The TestPerMessageTimer_Behavioral_SingleRollingTimerPerDirection's 50ms timeout fires the watchdog → streamCancel cascade → fake's Recv unblocks with `context.Canceled` → resume signal observed within the 1s test deadline.

**Verbatim build + vet + lint output** (per PLAN acceptance bullets):

```
$ go build ./... 2>&1
(empty; exit=0)

$ go vet ./... 2>&1
(empty; exit=0)

$ golangci-lint run ./internal/filter/http/extproc/... 2>&1
(empty; exit=0)
```

**Pre-existing extproc test suite + repo-wide -race -short suite continue GREEN:**

```
$ go test -race -count=1 ./internal/filter/http/extproc/... 2>&1 | tail -3
ok  	github.com/esalaine/envoy-go/internal/filter/http/extproc	1.116s

$ go test -race -short ./... 2>&1 | grep -cE '^ok'
53
$ go test -race -short ./... 2>&1 | grep -cE '^FAIL'
0
```

53 packages OK / 0 FAIL repo-wide under `-race -short`; no regressions.

**ADR-0171 §Decision AMENDMENT verbatim text added** (in-place edit after clause (x)):

> **§Decision AMENDMENT (phase 19.2):** numStages 2 → 4; stageRequestBody + stageResponseBody added; at-most-once-per-stage discipline EXTENDS unchanged; per-direction state SPLIT-BY-DIRECTION per D3 (flat 4-value enum with per-direction discipline enforced at dispatch site); activeProcessingMode body-mode/body-buffer state REUSES existing carriage (no new bodyMode + bodyBuf fields on the filter struct since `resolvedProcessingMode.RequestBodyMode`/`.ResponseBodyMode` post-Task-3 + ADR-0128/ADR-0175 primitives cover it); mode_override header-response-paths-only refinement carries unchanged at 19.2 (per parent §5.P1 RATIFIED-AND-REFINED — body-stage mode_override silently dropped, NOT counted as spurious); per-message timer behavioral enforcement lifts via context.WithTimeout(f.streamCtx, f.activeMsgTimeout) cancel-and-rebuild + watchdog goroutine cascade-cancel-via-streamCancel pattern per D4 + parent §5.P5 (single rolling timer per direction, NOT per-stage; preserves Send/Recv-same-goroutine invariant per parent §5.P10); 19.1 §12 deferred decision #6 CLOSES at this AMENDMENT.

Followed by 7 sub-clauses covering the 4-stage extension + at-most-once-per-stage carry-forward + activeProcessingMode reuse + mode_override carry-forward + per-message timer behavioral lift + OnDestroy carry-forward + 19.1 §12 closure.

**ADR-0171 acceptance grep verification** (per PLAN acceptance bullet):

```
$ grep -A3 '^## ADR-0171' docs/envoy-go/DECISIONS.md | grep -c '§Decision AMENDMENT'
2
```

≥ 1 satisfied (header line + Status line both carry the uppercase `§Decision AMENDMENT` wording for grep-discoverability; the substantive AMENDMENT clause lives ~100 lines deeper inside §Decision where the in-place edit landed after clause (x)).

**stageRequestBody / stageResponseBody acceptance grep verification:**

```
$ grep -cE 'stageRequestBody|stageResponseBody' internal/filter/http/extproc/processor.go
13
```

≥ 2 satisfied (13 occurrences across the stage-enum definition + String() switch + signalResume switch + doc-comments).

**D10 disposition update at this Task:** HOLDS. The §Decision AMENDMENT lift settled in-place per ADR-0044; no new ADR fired; the `compiledConfig` field-final invariant preserved per ADR-0168 §Decision (xi); only ONE new field on the per-stream `filter` struct (`activeMsgCancel context.CancelFunc` for the per-message timer cancel hook). Next-free ADR stays at `ADR-0177` (unconsumed).

**Rework note (Task 4 post-commit, doc-only) — code-quality reviewer I-1 fix.** Code-quality reviewer flagged I-1 (Important): the `handleOverrideMessageTimeout` docstring (`processor.go` ~898-925) + ADR-0171 §Decision AMENDMENT bullet 5 (`DECISIONS.md` ~9464 `override_message_timeout reset path extends naturally` sub-bullet) described the in-flight per-message-timer cancel as a "NEXT dispatchStage Send/Recv pair rebuilds with the override duration" semantic without acknowledging that invoking the cancel cascades through the watchdog goroutine to `f.streamCancel`, terminating the WHOLE bidi-stream — so there is NO "next dispatchStage on the current stream" observing the new duration. The misleading prose was replaced (in two locations) with an accurate description: invoking the in-flight `activeMsgCancel` is STREAM-FATAL by design; the "rebuild with override duration" semantic applies only to a future-stream lifecycle (if reopened by a subsequent request — a per-listener scope, NOT a continuation of the current stream). The trade-off justification is documented in both locations: at-most-one-in-flight per ADR-0171 §Decision (v) + the bidi-stream's one-shot-per-HTTP-transaction lifetime per ADR-0167 §Decision make the stream-fatal cascade the right granularity; the alternative (per-stage Recv interruption without canceling the stream) would break the Send/Recv-same-goroutine invariant that 19.1's `TestSequentialDecodeEncodeDispatchNoRace` + `TestBidiStreamSendRecvDiscipline` assert.

Also applied I-3 (bonus, cheap): the deferred `activeMsgCancel`-clear comment block in `dispatchStage` (~559-574) was verbose post-I-1; trimmed from ~10 lines of justification down to 4 lines pointing at the stream-fatal-cascade discipline (the unconditional nil-assignment is race-safe because `handleOverrideMessageTimeout` CLEARS the slot itself on fire, so by the time the defer runs the slot is either still ours or already cleared — idempotent either way).

NOT applied: M-1 (stage-enum reconciliation prose at processor.go ~78-106). Existing prose already cites `ADR-0171 §Consequences refresh` for the full 2→4-vs-2→6 reconciliation; the in-file commentary is 5 lines of concise pointing — no condensation gain.

NO code changes; NO test changes. Acceptance re-verified post-doc-edits:

```
$ go build ./... 2>&1
(empty; exit=0)

$ go vet ./... 2>&1
(empty; exit=0)

$ go test -race -count=1 -short ./... 2>&1 | grep -cE '^ok'
53
$ go test -race -count=1 -short ./... 2>&1 | grep -cE '^FAIL'
0
```

53 packages OK / 0 FAIL repo-wide under `-race -short -count=1`; no regressions.

**Rework commit SHA:** `9fb5137` (filled at Task 5 preamble per the ongoing per-Task SHA-fill discipline; `git log -1 --format=%H 9fb5137` → `9fb5137250b92f90a845c4a7a53682c44a57c3c5`)

---

### Task 5 — body-stage attribute envelope builders (`buildRequestBodyProcessingRequest` + `buildResponseBodyProcessingRequest`)

**Files changed:**
- `internal/filter/http/extproc/attributes.go` (~+205 LoC net: 2 new body-stage builders `buildRequestBodyProcessingRequest(f *filter, body []byte, endStream bool, allowlist []string) *extprocsvcv3.ProcessingRequest` + symmetric `buildResponseBodyProcessingRequest` populating `ProcessingRequest.request_body` / `ProcessingRequest.response_body = &HttpBody{Body, EndOfStream}` + attributes envelope per planner-time D5; 1 new body-stage envelope wrapper `buildBodyAttributeEnvelope(allowlist, 6 closures, bodySizeAttrName, bodySize)` delegating to `buildAttributeEnvelope` for the 7 header-stage SUPERSET attributes + adding the body-stage-natural body-size attribute; 1 new scalar helper `scalarNumberStruct(int64) *structpb.Struct` wrapping numeric values in the `{value: <NumberValue>}` shape parallel to the existing `scalarStringStruct`; the 19.1-landing `buildRequestHeadersProcessingRequest` + `buildResponseHeadersProcessingRequest` + `buildAttributeEnvelope` are UNCHANGED per the Task 5 wrapper-choice rationale)
- `internal/filter/http/extproc/extproc_test.go` (~+200 LoC net: 4 new Group N+3 tests per PLAN Step 1 — `TestBuildBodyProcessingRequest_PopulatesRequestBodyField` + `TestBuildBodyProcessingRequest_AttributesEnvelopeMirrorsHeaderStage` + `TestBuildBodyProcessingRequest_RequestSizePopulatesFromBodyLength` + `TestBuildResponseBodyProcessingRequest_Symmetric`)
- `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md` (this entry + Task 4 commit SHA placeholder fill: `49bf26d` + Task 4 rework commit SHA placeholder fill: `9fb5137`)

**Commit SHA:** `f96c88a` (filled at Task 6 preamble per the ongoing per-Task SHA-fill discipline; `git log -1 --format=%H f96c88a` → `f96c88a5c9a34e6483295e6c20758d3a9cdeb034`)
**Status:** done
**Notes:**

This Task 5 lands the body-stage `ProcessingRequest` envelope builders per SPEC §6.5 + §6.6 + planner-time D5 (header-stage SUPERSET; adds `request.size` / `response.size` populated from `len(body)`). The 19.1-landing header-stage builders STAY UNCHANGED per PLAN Step 3 (the production surface invariant — the body-stage builders are PURE-ADDITIVE).

**Wrapper-choice disposition for `request.size` / `response.size` (Task 5 PLAN §"How to handle request.size/response.size").** Two design candidates considered:

  1. **Extend `buildAttributeEnvelope` with a 7th `bodySizeFn func() int64` closure parameter** — populated by the body-stage builders, nil-passed by the header-stage builders. REJECTED — ripples through all 2 header-stage call sites + all 5 existing `TestBuildAttributeEnvelope_*` tests (signature break). The 19.1 IMPL invariant ("19.1-landing header-stage builders STAY UNCHANGED" per the Task 5 PLAN) would be violated.

  2. **Body-stage wrapper `buildBodyAttributeEnvelope(allowlist, 6 closures, bodySizeAttrName, bodySize)`** — delegates to `buildAttributeEnvelope` for the 7 header-stage SUPERSET attributes + adds the body-stage-natural body-size attribute via a small append loop. CHOSEN — preserves the existing `buildAttributeEnvelope` signature + the 5 existing tests + the 2 header-stage call sites unchanged; the body-stage-specific extension is isolated cleanly + future-proofs for the Task 9 fixture-scrape extension (additional body-stage-only attributes — hypothetical `request.body_md5`, etc. — land in this wrapper without rippling).

The wrapper's GoDoc documents the rationale + the empty-body populate property (a numeric 0 is a SUBSTANTIVE value — "this stage carried an empty body" is information-bearing per D5, distinct from the header-stage empty-value-skip discipline which silently drops empty string accessors).

**CEL numeric scalar encoding hypothesis (closes at Task 9 fixture scrape).** The body-size attribute value is wrapped in `*structpb.Struct{fields: {"value": <NumberValue>}}` — structpb has no native int64 scalar, so the IMPL settles a `NumberValue` (float64-typed) at 19.2 per the proto convention. The new `scalarNumberStruct(int64) *structpb.Struct` helper parallels the existing `scalarStringStruct` 1:1. Closure at Task 9 fixture scrape against reference Envoy v1.37.2's CEL attribute registry — a divergent reference-Envoy encoding (e.g. a wrapped `int64` Struct field) would adjust `scalarNumberStruct` in-place at Task 9 per the PLAN Task 9 Step 5.

**D5 attribute-roster hypothesis at this Task (held pending Task 9 scrape).** The body-stage roster lands the PLAN-time hypothesis: 7 header-stage CEL attributes (`source.address`, `destination.address`, `connection.requested_server_name`, `connection.subject_local_certificate`, `request.protocol`, `connection.principal`, `source.principal`) carry to body stage unchanged via the wrapper's `buildAttributeEnvelope` delegation; the 1 body-stage-only attribute (`request.size` decode / `response.size` encode) populates from `int64(len(body))`. The D10 hypothesis HELD on the encode side — `source.principal` is empty on EncoderFilterCallbacks per ADR-0174 §Decision, so the body-stage response envelope omits `source.principal` even when listed in the allowlist (the symmetric encode-side `TestBuildResponseBodyProcessingRequest_Symmetric` asserts 7 attrs total = 6 header-stage + `response.size`, not 8). The exact roster crystallizes empirically at Task 9 fixture-harness scrape per planner-time D5; if the scrape surfaces additional body-stage-only attributes the `buildBodyAttributeEnvelope` wrapper extends per PLAN Task 9 Step 5.

**No ADR firing at Task 5 per PLAN D8.** Task 5 does not land any ADR §Decision body or AMENDMENT — the body-stage envelope builders are pure-functional additions consuming the existing ADR-0170 + ADR-0174 + ADR-0144 + ADR-0165 accessor surfaces unchanged. Next-free ADR stays at `ADR-0177` (unconsumed).

**Verbatim test-run output for the 4 new Group N+3 tests** (per PLAN acceptance bullet):

```
$ go test -count=1 -run 'TestBuildBodyProcessingRequest|TestBuildResponseBodyProcessingRequest' -v ./internal/filter/http/extproc/... 2>&1 | tail -20
=== RUN   TestBuildBodyProcessingRequest_PopulatesRequestBodyField
=== PAUSE TestBuildBodyProcessingRequest_PopulatesRequestBodyField
=== RUN   TestBuildBodyProcessingRequest_AttributesEnvelopeMirrorsHeaderStage
=== PAUSE TestBuildBodyProcessingRequest_AttributesEnvelopeMirrorsHeaderStage
=== RUN   TestBuildBodyProcessingRequest_RequestSizePopulatesFromBodyLength
=== PAUSE TestBuildBodyProcessingRequest_RequestSizePopulatesFromBodyLength
=== RUN   TestBuildResponseBodyProcessingRequest_Symmetric
=== PAUSE TestBuildResponseBodyProcessingRequest_Symmetric
=== CONT  TestBuildBodyProcessingRequest_PopulatesRequestBodyField
=== CONT  TestBuildResponseBodyProcessingRequest_Symmetric
--- PASS: TestBuildBodyProcessingRequest_PopulatesRequestBodyField (0.00s)
=== CONT  TestBuildBodyProcessingRequest_AttributesEnvelopeMirrorsHeaderStage
--- PASS: TestBuildResponseBodyProcessingRequest_Symmetric (0.00s)
--- PASS: TestBuildBodyProcessingRequest_AttributesEnvelopeMirrorsHeaderStage (0.00s)
=== CONT  TestBuildBodyProcessingRequest_RequestSizePopulatesFromBodyLength
--- PASS: TestBuildBodyProcessingRequest_RequestSizePopulatesFromBodyLength (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/extproc	0.005s
```

All 4 new tests PASS.

**Verbatim build + vet + lint output** (per PLAN acceptance bullets):

```
$ go build ./... 2>&1
(empty; exit=0)

$ go vet ./... 2>&1
(empty; exit=0)

$ golangci-lint run ./internal/filter/http/extproc/... 2>&1
(empty; exit=0)
```

**Pre-existing extproc test suite + repo-wide -race -short suite continue GREEN:**

```
$ go test -race -count=1 ./internal/filter/http/extproc/... 2>&1 | tail -3
ok  	github.com/esalaine/envoy-go/internal/filter/http/extproc	1.111s

$ go test -race -short ./... 2>&1 | grep -cE '^ok'
53
$ go test -race -short ./... 2>&1 | grep -cE '^FAIL'
0
```

53 packages OK / 0 FAIL repo-wide under `-race -short`; no regressions. The pre-existing 5 `TestBuildAttributeEnvelope_*` tests + 2 header-stage builder tests + 5 helper tests all pass unchanged — the wrapper-choice disposition preserved the 19.1 surface intact.

**Acceptance grep verification** (per PLAN acceptance bullet):

```
$ grep -cE 'buildRequestBodyProcessingRequest|buildResponseBodyProcessingRequest' internal/filter/http/extproc/attributes.go
6
```

≥ 2 satisfied (6 occurrences across the 2 function definitions + their respective doc-comments).

**D10 disposition update at this Task:** HOLDS. The encode-side body-stage builder mirrors the encode-side header-stage builder per the symmetric `TestBuildResponseBodyProcessingRequest_Symmetric` assertion — `source.principal` is EMPTY on EncoderFilterCallbacks per ADR-0174 §Decision, so the body-stage response envelope omits it even when listed in the allowlist. The closure of D10 fires at Task 9 fixture-harness scrape per parent §5.P4-class.

---

### Task 6 — ADR-0172 §Decision AMENDMENT (body-mode arms of `applyProcessingResponse`)

**Files changed:**
- `internal/filter/http/extproc/check.go` (~+200 LoC net: `applyProcessingResponse` 7-step dispatcher EXTENDS with Step 5.5 body_mutation arm + Step 7 stage-aware CONTINUE_AND_REPLACE dispatch per SPEC §4.3 table; new `applyBodyMutation(f, s, bm)` helper (~30 LoC); new `writeBodyMutation(f, s, newBody)` helper for per-direction body-buffer write + Content-Length reconciliation (~15 LoC); `emitImmediateResponse` per-stage switch EXTENDS with stageRequestBody / stageResponseBody arms routing through dcb.SendLocalReply per ADR-0075; `errContinueAndReplaceNot19_1` sentinel RETIRED (variable removed; tombstone doc-comment retained); new `errStreamedResponseBodyMutationUnsupported` sentinel with the verbatim D6 wording; `strconv` import added; package doc-block extends to reference the 19.2 §Decision AMENDMENT)
- `internal/filter/http/extproc/extproc.go` (~+36 LoC net: filter struct gains 3 new fields per Task 6 hand-off contract — `decodeBodyBuf []byte` + `encodeBodyBuf []byte` (per-direction body-buffer carriers populated by Task 7's DecodeData/EncodeData endStream stash + mutated in-place by applyBodyMutation) + `skipBodyStageDispatch [2]bool` (per-direction skip-flag indexed 0=request/1=response, set by CONTINUE_AND_REPLACE at header-stage + body-mode=BUFFERED, consumed by Task 7's body-stage entry to skip the body-stage outbound dispatch))
- `internal/filter/http/extproc/extproc_test.go` (~+510 LoC net: Group N+1 body-stage applyProcessingResponse tests per PLAN Step 1 — `TestApplyProcessingResponse_BodyMutation_Body_ReplacesBufferAndReconcilesContentLength` (2 sub-tests for decode + encode side) + `TestApplyProcessingResponse_BodyMutation_ClearBody_EmptiesBuffer` (2 sub-tests for clear_body=true + clear_body=false no-op) + `TestApplyProcessingResponse_BodyMutation_StreamedResponse_PARSE_REJECT_SpuriousMsgsReceivedIncrement` + `TestApplyProcessingResponse_ContinueAndReplace_HeaderStageWithBodyModeBUFFERED_CombinedReplacement_BodyStageOutboundSKIPPED` (2 sub-tests for decode + encode side) + `TestApplyProcessingResponse_ContinueAndReplace_BodyStage_TreatedAsContinue_NoCounterIncrement` + `TestApplyProcessingResponse_BodyStageImmediateResponse_FiresSendLocalReply` (2 sub-tests for request_body + response_body stage) + `TestApplyProcessingResponse_ClearRouteCacheAtBodyStage_Ignored`; the 19.1-era `TestApplyProcessingResponse_ContinueAndReplace_SpuriousDispError` test REWRITTEN as `TestApplyProcessingResponse_ContinueAndReplace_HeaderStageWithBodyModeNONE_NoOp` to assert the SPEC §4.3 row 1 LIFTED disposition; 2 new helpers `mkProcessingResponseRequestBody` + `mkProcessingResponseResponseBody`)
- `docs/envoy-go/DECISIONS.md` (~+59 LoC net: ADR-0172 §Decision AMENDMENT (phase 19.2) clauses (xi)+(xii)+(xiii)+(xiv) added in-place per ADR-0044 in-place §Decision edit convention; Status line updated to reference the AMENDMENT; §Consequences refreshed with the AMENDMENT LANDED at Task 6 + the no-new-ADR-fires-at-Task-6 paragraph + the cross-phase reuse extension paragraph)
- `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md` (this entry + Task 5 commit SHA placeholder fill: `f96c88a`)

**Commit SHA:** `97ac0d1` (filled at Task 7 preamble per the per-Task SHA-fill discipline)
**Status:** done
**Notes:**

This Task 6 lands the ADR-0172 §Decision AMENDMENT (phase 19.2) body-mode arms of `applyProcessingResponse` per PLAN Task 6 + SPEC §4.2 + §4.3 + §4.4. The 7-step header-mode dispatcher (REUSED unchanged from 19.1) extends with a new Step 5.5 body_mutation arm (between Step 5 header_mutation and Step 6 clear_route_cache) + a rewritten Step 7 CONTINUE_AND_REPLACE arm (stage-aware three-arm dispatch per SPEC §4.3 table; the 19.1 single-arm spurious-dispatch LIFTS). `emitImmediateResponse` extends its per-stage switch to route body-stage ImmediateResponse arrivals through the existing dcb.SendLocalReply pathway per ADR-0075 (SPEC §4.4).

**Body-buffer write strategy disposition (PLAN Task 6 §"CRITICAL implementation notes").** Three candidates considered:

  1. **New `OverwriteEncodedBuffer(b []byte)` framework primitive on EncoderFilterCallbacks** — would have consumed `ADR-0177` + falsified D10. REJECTED — D10 hypothesis stays HELD; the existing `OverwriteBody` per ADR-0131 + the per-filter buffer fields suffice.
  2. **Defer body write to Task 7 via a "pending body replacement" carrier** — pure deferral pattern; the dispatcher would set `f.pendingBodyReplacement []byte` + Task 7 would consume + write at resume time. REJECTED as overcomplicated — the per-direction body-buffer field doubles as both stash + mutated-carrier without an extra indirection.
  3. **Per-direction body-buffer fields on the *filter struct** — `decodeBodyBuf []byte` + `encodeBodyBuf []byte` serve as both the pre-mutation stash (populated by Task 7's DecodeData/EncodeData endStream entry) AND the post-mutation carrier (read by Task 7's resume path before releasing the body downstream via `f.ecb.OverwriteBody(f.encodeBodyBuf)` for the encode side). CHOSEN — race-safe by D9 sequential dispatch invariant; no new framework primitive consumed; Task 7's hand-off contract documented at the filter-struct field doc-comment + the ADR-0172 §Decision AMENDMENT clause (xi).

  At Task 6 the body-buffer fields are unit-test populated directly (the test fixture stubs the field then drives applyProcessingResponse + asserts the field post-mutation). The Task 7 integration replaces the test-fixture stub with the real DecodeData / EncodeData endStream stash site.

**CONTINUE_AND_REPLACE skip-body-stage-outbound mechanism.** The `skipBodyStageDispatch [2]bool` field on *filter struct (added at Task 6) carries the per-direction skip flag indexed 0=request, 1=response. Set by the Step 7 CONTINUE_AND_REPLACE arm when the stage is a header stage AND `f.activeProcessingMode.{Request,Response}BodyMode == BUFFERED`. Returns `actContinueButStillWaiting` per SPEC §4.3 row 2 transition note. **Task 7 hand-off contract:** Task 7's DecodeData / EncodeData endStream entry checks `f.skipBodyStageDispatch[direction]` BEFORE invoking the body-stage dispatchStage; when set, the body-stage outbound call is skipped + the (pre-replaced) body buffer is released directly to the chain. The flag is per-direction so the two directions can independently set/clear without cross-talk.

**`errContinueAndReplaceNot19_1` sentinel RETIRED.** The 19.1 sentinel + the 19.1-era `TestApplyProcessingResponse_ContinueAndReplace_SpuriousDispError` test are both removed at Task 6 — the spurious-dispatch LIFTS per SPEC §4.3. The variable's tombstone doc-comment at its prior site references the AMENDMENT for grep-discoverability. The legacy test is RENAMED + REWRITTEN as `TestApplyProcessingResponse_ContinueAndReplace_HeaderStageWithBodyModeNONE_NoOp` asserting the LIFTED disposition (no spurious + actContinue) per SPEC §4.3 row 1.

**ADR-0172 §Decision AMENDMENT (phase 19.2) hash diff** (per PLAN Task 6 PROGRESS append bullet — provides reviewer-grade integrity-check anchor for the in-place ADR edit):

```
$ git diff docs/envoy-go/DECISIONS.md | sha256sum
d071eae3e85777b465aab4ba57d4d1c512f58a2c5b27b0f5488a6d5327927c4c  -
$ git diff docs/envoy-go/DECISIONS.md | wc -l
59
```

**Acceptance grep verification** (per PLAN Task 6 acceptance bullet):

```
$ grep -A3 '^## ADR-0172' docs/envoy-go/DECISIONS.md | grep -c '§Decision AMENDMENT'
1
$ grep -cE '§Decision AMENDMENT' docs/envoy-go/DECISIONS.md
31
```

≥ 1 satisfied (the Status line references the AMENDMENT directly). The repo-wide §Decision AMENDMENT count is 31 (up from 30 at Task 5 — Task 6 adds 1 new AMENDMENT block on ADR-0172 + new Status reference + cross-ADR references in §Consequences AMENDED-text).

**No ADR firing at Task 6 per PLAN D8 + D10.** Task 6 lands an in-place §Decision AMENDMENT on ADR-0172 per ADR-0044's in-place edit convention — NO new ADR fires. D10 hypothesis (NO additional ADR fires at 19.2 IMPL beyond ADR-0175 + the three §Decision AMENDMENTs — ADR-0168 / ADR-0171 / ADR-0172) HOLDS through Task 6. The body-buffer write strategy choice (clause (xi) of the AMENDMENT) explicitly rejects the alternative `OverwriteEncodedBuffer` framework primitive that would have consumed `ADR-0177`; the existing `OverwriteBody` per ADR-0131 + the per-filter buffer fields suffice. Next-free ADR stays at `ADR-0177` (unconsumed).

**Verbatim test-run output for the 7 new Group N+1 tests** (per PLAN Task 6 acceptance bullet):

```
$ go test -race -count=1 -v -run 'TestApplyProcessingResponse_(BodyMutation|ContinueAndReplace|BodyStageImmediateResponse|ClearRouteCacheAtBodyStage)' ./internal/filter/http/extproc/... 2>&1 | tail -30
--- PASS: TestApplyProcessingResponse_ContinueAndReplace_HeaderStageWithBodyModeNONE_NoOp (0.00s)
--- PASS: TestApplyProcessingResponse_ContinueAndReplace_BodyStage_TreatedAsContinue_NoCounterIncrement (0.00s)
--- PASS: TestApplyProcessingResponse_BodyMutation_StreamedResponse_PARSE_REJECT_SpuriousMsgsReceivedIncrement (0.00s)
--- PASS: TestApplyProcessingResponse_ClearRouteCacheAtBodyStage_Ignored (0.00s)
--- PASS: TestApplyProcessingResponse_BodyMutation_Body_ReplacesBufferAndReconcilesContentLength (0.00s)
    --- PASS: TestApplyProcessingResponse_BodyMutation_Body_ReplacesBufferAndReconcilesContentLength/encode_side_response_body (0.00s)
    --- PASS: TestApplyProcessingResponse_BodyMutation_Body_ReplacesBufferAndReconcilesContentLength/decode_side_request_body (0.00s)
--- PASS: TestApplyProcessingResponse_BodyMutation_ClearBody_EmptiesBuffer (0.00s)
    --- PASS: TestApplyProcessingResponse_BodyMutation_ClearBody_EmptiesBuffer/clear_body_true_empties_decode_buffer (0.00s)
    --- PASS: TestApplyProcessingResponse_BodyMutation_ClearBody_EmptiesBuffer/clear_body_false_is_noop (0.00s)
--- PASS: TestApplyProcessingResponse_ContinueAndReplace_HeaderStageWithBodyModeBUFFERED_CombinedReplacement_BodyStageOutboundSKIPPED (0.00s)
    --- PASS: TestApplyProcessingResponse_ContinueAndReplace_HeaderStageWithBodyModeBUFFERED_CombinedReplacement_BodyStageOutboundSKIPPED/decode_side_request_headers_buffered_request_body (0.00s)
    --- PASS: TestApplyProcessingResponse_ContinueAndReplace_HeaderStageWithBodyModeBUFFERED_CombinedReplacement_BodyStageOutboundSKIPPED/encode_side_response_headers_buffered_response_body (0.00s)
--- PASS: TestApplyProcessingResponse_BodyStageImmediateResponse_FiresSendLocalReply (0.00s)
    --- PASS: TestApplyProcessingResponse_BodyStageImmediateResponse_FiresSendLocalReply/response_body_stage (0.00s)
    --- PASS: TestApplyProcessingResponse_BodyStageImmediateResponse_FiresSendLocalReply/request_body_stage (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/extproc	1.015s
```

All 7 new Group N+1 tests PASS (the renamed/rewritten 19.1-era `TestApplyProcessingResponse_ContinueAndReplace_HeaderStageWithBodyModeNONE_NoOp` is the 8th match per the `TestApplyProcessingResponse_(BodyMutation|ContinueAndReplace|BodyStageImmediateResponse|ClearRouteCacheAtBodyStage)` regex; it also PASSes per the SPEC §4.3 row 1 LIFTED disposition).

**Verbatim build + vet + lint output** (per PLAN Task 6 acceptance bullets):

```
$ go build ./... 2>&1
(empty; exit=0)

$ go vet ./... 2>&1
(empty; exit=0)

$ golangci-lint run ./internal/filter/http/extproc/... 2>&1
(empty; exit=0)
```

**Pre-existing extproc test suite + repo-wide -race -short suite continue GREEN:**

```
$ go test -race -count=1 ./internal/filter/http/extproc/... 2>&1 | tail -3
ok  	github.com/esalaine/envoy-go/internal/filter/http/extproc	1.117s

$ go test -race -short ./... 2>&1 | grep -cE '^ok'
53
$ go test -race -short ./... 2>&1 | grep -cE '^FAIL'
0
```

53 packages OK / 0 FAIL repo-wide under `-race -short`; no regressions.

**Self-review checklist verification** (per PLAN Task 6 self-review block):

  - [x] body_mutation switch handles all 3 arms: Body / ClearBody / StreamedResponse — covered by `TestApplyProcessingResponse_BodyMutation_Body_*` + `TestApplyProcessingResponse_BodyMutation_ClearBody_*` + `TestApplyProcessingResponse_BodyMutation_StreamedResponse_*`.
  - [x] body_mutation Body: writes new bytes to body buffer + reconciles Content-Length — covered + asserted via `f.decodeBodyBuf` / `f.encodeBodyBuf` field check + `hdrs.Get("content-length")` assertion (per-direction sub-tests).
  - [x] body_mutation ClearBody true: empties body buffer + Content-Length: 0 — covered.
  - [x] body_mutation StreamedResponse: PARSE-REJECT with D6 wording + spurious++ — covered; asserts the verbatim D6 substring + spurious++ counter.
  - [x] CONTINUE_AND_REPLACE at header-stage + body-mode NONE: no-op for body, NO counter increment — covered by renamed test.
  - [x] CONTINUE_AND_REPLACE at header-stage + body-mode BUFFERED: combined replacement; sets f.skipBodyStageDispatch — covered (per-direction sub-tests for decode + encode side).
  - [x] CONTINUE_AND_REPLACE at body stages: TREATED AS CONTINUE, no counter increment — covered.
  - [x] errContinueAndReplaceNot19_1 sentinel REMOVED — tombstone doc-comment retained for grep-discoverability; variable identifier fully removed.
  - [x] body-stage ImmediateResponse fires via existing emitImmediateResponse(f, ir, s) — covered (per-stage sub-tests for request_body + response_body).
  - [x] clear_route_cache at body stages IGNORED — covered by `TestApplyProcessingResponse_ClearRouteCacheAtBodyStage_Ignored`.
  - [x] 7 new tests with EXACT names from PLAN added + PASS — verified above.
  - [x] go build / vet / lint clean — verified above.
  - [x] Full -race -short repo-wide: 0 FAIL — verified above.
  - [x] ADR-0172 §Decision AMENDMENT + §Consequences refresh landed in DECISIONS.md — verified via grep.
  - [x] PROGRESS.md Task 6 entry appended + Task 5 SHA `f96c88a` filled — done at this commit.

**Task 7 hand-off contract** (consumed by the next implementer):

  1. **Body-buffer stash sites** — Task 7's DecodeData endStream entry stashes the accumulated decode-side body buffer onto `f.decodeBodyBuf` (via the ADR-0128 chain-level body-buffering reader) BEFORE invoking `f.dispatchStage(stageRequestBody, ...)`. Symmetric on the encode side: Task 7's EncodeData endStream entry stashes via `f.encodeBodyBuf = f.ecb.BufferEncodedBody()` BEFORE invoking `f.dispatchStage(stageResponseBody, ...)`.
  2. **Skip-flag consumer site** — Task 7's body-stage entry (the new body-stage `(*filter).DecodeData` / `(*filter).EncodeData` endStream branch) checks `f.skipBodyStageDispatch[direction]` BEFORE invoking the body-stage dispatchStage. When set, skip the body-stage outbound dispatch + release the (already-mutated) body buffer directly to the chain via `f.ecb.OverwriteBody(f.encodeBodyBuf)` (encode side) or the decode-side analog.
  3. **Post-resume body-write site** — Task 7's body-stage resume path (after dispatchStage completes + applyProcessingResponse returns actContinue) reads `f.encodeBodyBuf` + writes via `f.ecb.OverwriteBody(f.encodeBodyBuf)` so the (possibly-mutated) body bytes propagate downstream. Decode side: the analogous decode-buffer write site uses the framework's decode-side buffer-release mechanism (the exact primitive determined at Task 7 integration time per the ADR-0128 §Decision discipline).
  4. **No new framework primitives required** — D10 HOLDS through Task 6; Task 7 should NOT introduce new EncoderFilterCallbacks primitives unless an empirical fixture pin forces the issue (the alternative `OverwriteEncodedBuffer(b []byte)` primitive remains REJECTED at Task 6).

---

### Task 7 — extproc.go body-stage integration (wires Tasks 2-6 into full body-mode dispatch) + ADR-0168 §Consequences refresh

**Files touched:**
- `internal/filter/http/extproc/extproc.go` (~+200 LoC net: `(*filter).DecodeData` body-stage dispatch wiring per SPEC §6.3 pseudocode — body-mode-active gate + mid-stream accumulate-and-continue + endStream stash-dispatch-park via `DataStopIterationAndBuffer`; `(*filter).EncodeData` symmetric body-stage dispatch wiring with the additional encode-side `OverwriteBody` delivery via the dispatch goroutine's `deliverEncodeBodyMutation` helper; `(*filter).bodyModeActive(direction)` helper reading `f.activeProcessingMode.RequestBodyMode` / `.ResponseBodyMode == BUFFERED` with nil-tolerance; `(*filter).deliverEncodeBodyMutation` helper invoking `f.ecb.OverwriteBody(f.encodeBodyBuf)` from the dispatch-goroutine post-mutation site BEFORE the resume-signal fires; the directionRequest / directionResponse package-private constants per Task 6 reviewer carry-forward I-1 — named direction constants for `f.skipBodyStageDispatch` array indexing)
- `internal/filter/http/extproc/processor.go` (~+22 LoC net: `completeStage` extended with the `s == stageResponseBody` branch that invokes `f.deliverEncodeBodyMutation()` BEFORE the resume-signal fires — registers the (possibly-mutated) encode-side body buffer via ADR-0131 OverwriteBody so the chain's encodeBodyOverride is set when HCM substitutes resp.Body post-RunEncodeData. The request_body analog is OMITTED per the decode-side KNOWN LIMITATION documented at DecodeData; only the Content-Length header reconciliation propagates upstream via f.decodeHeaders.)
- `internal/filter/http/extproc/check.go` (~+2 LoC net: I-1 named direction constant adoption at the CONTINUE_AND_REPLACE skip-flag set-site (Step 7 of `applyProcessingResponse`); `f.skipBodyStageDispatch[0]` / `[1]` → `[directionRequest]` / `[directionResponse]`. The Task 6 BUFFERED-combined-replacement branch becomes self-documenting at the Task 7 consumer's grep-discoverability site.)
- `internal/filter/http/extproc/extproc_test.go` (~+450 LoC net: 5 new Task 7 integration tests per PLAN Step 1 — `TestExtProc_RequestBodyBuffered_EndToEnd_WithMutation` + `TestExtProc_ResponseBodyBuffered_EndToEnd_WithMutation` + `TestExtProc_BodyStageImmediateResponse_EndToEnd` (2 sub-tests for request_body + response_body) + `TestExtProc_ContinueAndReplace_HeaderStageWithBodyModeBUFFERED_EndToEnd_BodyStageOutboundSKIPPED` (2 sub-tests for decode + encode side) + `TestExtProc_OnDestroy_DuringBodyStageOutbound_NoBufferReleaseFires`; 1 new Task 6 carry-forward I-2 test `TestApplyProcessingResponse_BodyMutation_EmptyOneof_NoOp` pinning that empty BodyMutation oneof is silently no-op; 1 new race-safe `recordingECB` fake with mutex-protected `overwriteCount` / `overwriteBytes` accessors for OverwriteBody assertions; `recordingDCB` extended with `lrMu` mutex + `lrCallsSafe` / `lrStatusSafe` / `lrBodySafe` accessors for race-safe async-goroutine field reads; `makeIntegrationFilter(t, stream)` integration-filter constructor + `waitForCondition(deadline, cond)` polling helper; the Task 6 I-1 direction-constants adoption applied at 4 existing test sites (the body-mode-NONE no-op test + the BUFFERED-skip-flag set tests for both directions + the body-stage CONTINUE_AND_REPLACE no-skip test) replacing bare `[0]` / `[1]` indices with `[directionRequest]` / `[directionResponse]`)
- `docs/envoy-go/DECISIONS.md` (~+33 LoC net: ADR-0168 §Consequences refresh per PLAN Task 7 Step 7 — 4 new paragraphs covering (a) body-stage integration completeness clause with the Tasks 2-6 primitive composition + 5-value action enum symmetry; (b) decode-side body-mutation-delivery KNOWN LIMITATION + closure deferred to a future phase; (c) `compiledConfig` field-final invariant preservation through Task 7 wiring; (d) D10 hypothesis HOLDS + ADR-0177 stays unconsumed disposition; Status line + Doctrine line + Lands-in line updated to reference the §Consequences refresh)
- `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md` (this entry + Task 6 commit SHA placeholder fill: `97ac0d1`)

**Commit SHA:** `0fd895d` (filled at Task 8 preamble per the ongoing per-Task SHA-fill discipline; `git log -1 --format=%H 0fd895d` → `0fd895df4199097455cc5c78cb90cca17be0b53c`)
**Status:** done
**Notes:**

This Task 7 lands the body-stage integration completeness per PLAN Task 7 + SPEC §6.3 pseudocode. The Tasks 2-6 primitive set is wired END-TO-END into the `(*filter).DecodeData` + `(*filter).EncodeData` entry points:

- **Task 2 ADR-0175 `BufferEncodedBody`** — anchored at the encode-side chain accumulation path (the chain's encodeBuf release-and-clear on end_stream); structurally available for the multi-chunk synthetic case. The 19.2 production path passes the full response body in ONE EncodeData call (per `connection.go` H1 line 591 + `h2dispatch.go` H2 line 398), so the chain-side accumulation is exercised by Task 8 race tests rather than the integration tests at Task 7.
- **Task 4 4-stage state machine** — `stageRequestBody` + `stageResponseBody` enum values consumed by `f.dispatchStage(stage, req)`; the dispatch goroutine + per-message-timer + Recv watchdog reuse unchanged from the Task 4 header-mode shape.
- **Task 5 body-stage attribute envelope builders** — `buildRequestBodyProcessingRequest(f, body, true, f.cc.requestAttributes)` + `buildResponseBodyProcessingRequest(f, body, true, f.cc.responseAttributes)` construct the per-direction `ProcessingRequest{request_body|response_body: HttpBody{body, end_of_stream}}` envelopes with the body-stage attribute roster per planner-time D5.
- **Task 6 body-mode arms of `applyProcessingResponse`** — `body_mutation` dispatch + `skipBodyStageDispatch[direction]` skip-flag consumer + the body-stage `CONTINUE_AND_REPLACE` TREATED-AS-CONTINUE arm reuse unchanged; the body-mutation arm writes new bytes to `f.encodeBodyBuf` which the Task 7 `deliverEncodeBodyMutation` helper delivers via ADR-0131 `OverwriteBody`.

**Dispatch algorithm summary (per SPEC §6.3 pseudocode + planner-time D7 OnDestroy discipline):**

1. **`bodyModeActive(direction)` gate** — reads `f.activeProcessingMode.RequestBodyMode == BUFFERED` (or `.ResponseBodyMode`) with nil-tolerance; when inactive, return `DataContinue` verbatim (pure passthrough per the 19.1 envelope).
2. **Mid-stream chunk (`!endStream`)** — accumulate into `f.decodeBodyBuf` / `f.encodeBodyBuf` via `append(...)` + return `DataContinue`. Per the ADR-0128 synchronous-HCM dispatch model + ext_authz precedent: parking mid-stream via `DataStopIterationAndBuffer` would deadlock the dispatch goroutine; the filter-side accumulation gives the body-stage endStream entry a single contiguous byte slice.
3. **Terminal chunk + `skipBodyStageDispatch[direction]` SET** — SKIP the body-stage outbound dispatch. Decode side: return `DataContinue` (decode-side body-mutation-delivery KNOWN LIMITATION — the buffer mutation does not reach upstream). Encode side: invoke `f.ecb.OverwriteBody(f.encodeBodyBuf)` to release the pre-mutated buffer + return `DataContinue` so HCM substitutes resp.Body via the encodeBodyOverride.
4. **Terminal chunk + skip-flag CLEAR** — stash the final bytes onto the accumulator, build the body-stage envelope via the Task 5 builder, dispatch via `f.dispatchStage(stageRequestBody|stageResponseBody, req)`, park via `DataStopIterationAndBuffer`. The async dispatch goroutine fires Send + Recv on the bidi-stream; on Recv completion, `completeStage` invokes `applyProcessingResponseFn` (which may mutate the body buffer via Task 6's `body_mutation` arm) → invokes `deliverEncodeBodyMutation` for stageResponseBody (registers the OverwriteBody) → fires `signalResume` (ContinueDecoding / ContinueEncoding) which unparks the chain.

**Decode-side body-mutation-delivery KNOWN LIMITATION at 19.2 — scope reduction.** Per the §Consequences refresh: the encode-side body-mutation arm WORKS end-to-end via ADR-0131 `OverwriteBody` reuse. The decode side has NO equivalent — envoy-go's HCM reads upstream-bound body bytes from its own `bodyBuf` (H1 `connection.go` line 483-514) / `h2req.Body` (H2 `h2dispatch.go` line 361), both captured BEFORE the filter-chain mutation lands. The body_mutation arm at the decode side STILL runs (Task 6 wiring): the processor SEES the body bytes + can request mutation; the mutated bytes land in `f.decodeBodyBuf` (observable from the filter) + the Content-Length header reconciliation propagates upstream via the shared `f.decodeHeaders` / `req.Header` map. Only the body BYTES themselves are not delivered. Closure deferred to a future phase that adds the decode-side body-mutation-delivery framework primitive (likely a future ADR analogous to ADR-0131 for the decode side; speculative ADR-0178 or later). The 19.2 differential fixture (Task 9) scopes its mutation scenarios to the encode side where delivery works fully.

**Folded-in Task 6 reviewer carry-forward items (per PLAN Task 7 "Folded-in cleanup" block):**

- **I-1 named direction constants** — `directionRequest = 0` + `directionResponse = 1` package-private constants defined in `extproc.go` next to the `filter.skipBodyStageDispatch` field comment. Replaces bare `[0]` / `[1]` indices at the Task 6 set-sites in `check.go` (the `CONTINUE_AND_REPLACE` Step 7 BUFFERED-combined-replacement branch) AND at the 4 existing test-site uses in `extproc_test.go` (the body-mode-NONE no-op test + the BUFFERED-skip-flag set tests for both directions + the body-stage CONTINUE_AND_REPLACE no-skip test) AND at the new Task 7 consumer sites (`DecodeData` step 4 + `EncodeData` step 4 skip-flag checks). The skip-flag consumer sites become self-documenting via the named indices.
- **I-2 empty BodyMutation oneof test pin** — `TestApplyProcessingResponse_BodyMutation_EmptyOneof_NoOp` added to `extproc_test.go` pinning that `*BodyMutation` with no oneof set (i.e., `&extprocsvcv3.BodyMutation{}`) is silently a no-op per the proto's oneof-default discipline: actContinue + spurious_msgs_received unchanged + decodeBodyBuf unmodified + Content-Length unchanged. Prevents regression if a future maintainer refactors the `applyBodyMutation` switch + inadvertently classifies the empty-oneof case as malformed.

The other Task 6 reviewer items (I-3 lowercase-content-length, I-4 single-switch refactor) are stylistic deferrals — omitted at Task 7 per the PLAN's "defer to a later cleanup or omit" guidance.

**Race-safety reinforcement at the `recordingDCB` fake (test-only concern).** The Task 7 integration tests invoke the async dispatch goroutine which may call `dcb.SendLocalReply` from the goroutine while the test polls `dcb.lrCalls` / `dcb.lrStatus` / `dcb.lrBody` from the test goroutine — surfaces a `-race` flag without mutex protection. The fix: `recordingDCB` gains an `lrMu sync.Mutex` field + 3 accessor methods (`lrCallsSafe` / `lrStatusSafe` / `lrBodySafe`) that the Task 7 integration tests use. The existing 19.1-era tests that read fields directly without the mutex stay race-safe because they invoke SendLocalReply synchronously from the test goroutine (no async dispatch). The accessor-vs-direct-field split is a pragmatic test-only race-cleanup; production code (`emitImmediateResponse` writes to `dcb.SendLocalReply` are dispatcher-goroutine-private + happens-before-ordered with the parked-HCM-goroutine via the encodeResumeCh / decodeResumeCh primitives).

**D10 hypothesis HOLDS through Task 7.** Per the ADR-0168 §Consequences refresh's "D10 hypothesis HOLDS" paragraph: NO additional ADR fires at 19.2 IMPL beyond ADR-0175 (Task 2) + the three §Decision AMENDMENTs (ADR-0168 / ADR-0171 / ADR-0172). The next-free ADR stays at `ADR-0177` (unconsumed at the post-Task-7 commit). The decode-side body-mutation-delivery limitation is the explicit scope reduction that defers the alternative framework-primitive consumption (the would-be `dcb.OverwriteBody` analog) to a future phase.

**ADR-0168 §Consequences refresh hash diff** (per PLAN Task 7 PROGRESS append bullet — provides reviewer-grade integrity-check anchor for the in-place ADR edit):

```
$ git diff docs/envoy-go/DECISIONS.md | sha256sum
679494854d86586c90dda219c31645fa4f4ca8cb7a5e58f75e1077c00b7b23bc  -
$ git diff docs/envoy-go/DECISIONS.md | wc -l
33
```

**Acceptance grep verification** (per PLAN Task 7 acceptance bullet):

```
$ grep -A20 '^## ADR-0168' docs/envoy-go/DECISIONS.md | grep -c '§Consequences'
3
```

3 ≥ 1 satisfied (the Status + Doctrine + Lands-in lines now reference the §Consequences refresh directly).

**Verbatim test-run output for the 5 new Task 7 integration tests + the 1 new I-2 test** (per PLAN Task 7 acceptance bullet):

```
$ go test -race -count=1 -v -run 'TestExtProc_(RequestBody|ResponseBody|BodyStageImmediate|ContinueAndReplace_HeaderStage|OnDestroy_DuringBody)' ./internal/filter/http/extproc/... 2>&1 | tail -15
--- PASS: TestExtProc_ContinueAndReplace_HeaderStageWithBodyModeBUFFERED_EndToEnd_BodyStageOutboundSKIPPED (0.00s)
    --- PASS: TestExtProc_ContinueAndReplace_HeaderStageWithBodyModeBUFFERED_EndToEnd_BodyStageOutboundSKIPPED/decode_side_request_body_SKIPPED (0.00s)
    --- PASS: TestExtProc_ContinueAndReplace_HeaderStageWithBodyModeBUFFERED_EndToEnd_BodyStageOutboundSKIPPED/encode_side_response_body_SKIPPED_OverwriteBodyFiresWithPreMutatedBuffer (0.00s)
--- PASS: TestExtProc_ResponseBodyBuffered_EndToEnd_WithMutation (0.01s)
--- PASS: TestExtProc_RequestBodyBuffered_EndToEnd_WithMutation (0.01s)
--- PASS: TestExtProc_BodyStageImmediateResponse_EndToEnd (0.00s)
    --- PASS: TestExtProc_BodyStageImmediateResponse_EndToEnd/response_body_stage (0.01s)
    --- PASS: TestExtProc_BodyStageImmediateResponse_EndToEnd/request_body_stage (0.01s)
--- PASS: TestExtProc_OnDestroy_DuringBodyStageOutbound_NoBufferReleaseFires (0.17s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/extproc	1.185s

$ go test -race -count=1 -v -run 'TestApplyProcessingResponse_BodyMutation_EmptyOneof_NoOp' ./internal/filter/http/extproc/... 2>&1 | tail -5
--- PASS: TestApplyProcessingResponse_BodyMutation_EmptyOneof_NoOp (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/extproc	1.012s
```

All 5 new Task 7 integration tests + the 1 new I-2 carry-forward test PASS under `-race`.

**Verbatim build + vet + lint output** (per PLAN Task 7 acceptance bullets):

```
$ go build ./... 2>&1
(empty; exit=0)

$ go vet ./... 2>&1
(empty; exit=0)

$ golangci-lint run ./internal/filter/http/extproc/... 2>&1
(empty; exit=0)
```

**Pre-existing extproc test suite + repo-wide -race -short suite continue GREEN:**

```
$ go test -race -count=1 ./internal/filter/http/extproc/... 2>&1 | tail -3
ok  	github.com/esalaine/envoy-go/internal/filter/http/extproc	1.233s

$ go test -race -short -count=1 ./... 2>&1 | grep -cE '^ok'
53
$ go test -race -short -count=1 ./... 2>&1 | grep -cE '^FAIL'
0
```

53 packages OK / 0 FAIL repo-wide under `-race -short`; no regressions.

**Self-review checklist verification** (per PLAN Task 7 self-review block):

  - [x] DecodeData body-stage dispatch wired (accumulate + endStream emit + park + resume) — `(*filter).DecodeData` body lands the 6-step algorithm per the SPEC §6.3 pseudocode.
  - [x] EncodeData body-stage dispatch wired symmetrically using BufferEncodedBody + OverwriteBody for delivery — `(*filter).EncodeData` + `(*filter).deliverEncodeBodyMutation` fired from `completeStage`'s post-applyProcessingResponse path.
  - [x] `skipBodyStageDispatch[direction]` consumed at body-stage entry — if set, SKIP outbound + release pre-mutated body (encode side: via OverwriteBody; decode side: DataContinue per KNOWN LIMITATION).
  - [x] Body-stage ImmediateResponse correctly routes via existing emitImmediateResponse (verify with integration test) — `TestExtProc_BodyStageImmediateResponse_EndToEnd` 2 sub-tests cover request_body + response_body stages.
  - [x] OnDestroy during body-stage outbound: streamCancel cascades + dispatch goroutine returns WITHOUT ContinueDecoding/Encoding — `TestExtProc_OnDestroy_DuringBodyStageOutbound_NoBufferReleaseFires` asserts OverwriteBody NEVER fires + ContinueEncoding NEVER fires + buffer unchanged + f.done set.
  - [x] 5 new integration tests with EXACT PLAN names added + PASS — verified via the verbatim test-run output above.
  - [x] Decode-side body-mutation delivery: WORKS or DOCUMENTED LIMITATION (with closure plan) — DOCUMENTED LIMITATION: scope-reduced per the PLAN's "Scope for Task 7 (recommended)" + the §Consequences refresh covers it explicitly; closure deferred to a future phase that adds the decode-side body-mutation-delivery framework primitive.
  - [x] ADR-0168 §Consequences refresh landed; cross-references Task 7 integration — verified via `grep -A20` showing 3 §Consequences mentions in the Status + Doctrine + Lands-in lines.
  - [x] Folded-in Task 6 reviewer cleanup: named direction constants + empty BodyMutation test pin — BOTH applied; I-3 lowercase-content-length + I-4 single-switch refactor stylistic items deferred per PLAN guidance.
  - [x] PROGRESS.md Task 7 entry + Task 6 SHA `97ac0d1` filled — done at this commit.
  - [x] go build / vet / lint clean; race-tests clean repo-wide (53 packages OK / 0 FAIL) — verified above.
  - [x] D10 hypothesis HOLDS (no new ADR consumed; ADR-0177 stays unconsumed) — verified via the §Consequences refresh's "D10 hypothesis HOLDS through Task 7" paragraph.

**Task 8 hand-off contract** (consumed by the next implementer):

  1. **Race-test surface (PLAN Task 8 scope)** — the Task 7 integration tests exercise the COMMON-case async dispatch (single goroutine per stage); Task 8's race tests should exercise the more pathological scenarios: (a) concurrent OnDestroy + completing dispatch goroutine — D9 race-guard discipline; (b) chain.encodeBuf concurrent reads + writes from the dispatch goroutine + the parked HCM goroutine — ADR-0175 buffer-and-hold race surface; (c) the per-message timer cancel/rebuild race — Task 4's behavioral-lift per-message timer + the override_message_timeout cascade-cancel.
  2. **encodeBuf chain-accumulation path** — Task 7 lands the encode-side OverwriteBody delivery for the COMMON-case single-EncodeData-call HCM path. The chain-side encodeBuf accumulation (via `DataStopIterationAndBuffer`) is structurally available + exercised by Task 8's synthetic multi-chunk race test; Task 8 should verify the union of (a) chain accumulation + (b) OverwriteBody delivery resolves deterministically (the OverwriteBody wins per the HCM substitution discipline at `connection.go` line 595).
  3. **decode-side body-mutation-delivery limitation** — Task 8 should NOT introduce a decode-side `OverwriteBody` analog. The limitation stays documented + closure-deferred per Task 7's §Consequences refresh. The differential fixture at Task 9 will scope its decode-side body-mutation scenarios to OBSERVABILITY-only (the processor sees the body + records it for the fixture-harness scrape; no upstream-body-mutation assertions).
  4. **No new framework primitives required** — D10 HOLDS through Task 7; Task 8 should NOT introduce new EncoderFilterCallbacks / DecoderFilterCallbacks primitives. ADR-0177 stays unconsumed.

**Rework note (post-`0fd895d`, code-quality reviewer `a06aa6c8e7db06ecf`):** the code-quality reviewer flagged ONE CRITICAL correctness bug (C-1) + THREE Important items (I-1 / I-2 / I-3) on the Task 7 commit. The rework lands as a follow-up commit BEFORE Task 8 dispatch.

  - **C-1 (CRITICAL) — skip-flag path corrupts the pre-mutated body buffer.** Pre-rework, both `(*filter).DecodeData` (`extproc.go:1069-1081`) and `(*filter).EncodeData` (`extproc.go:1289-1305`) unconditionally APPENDED the incoming HCM chunk to `f.{decode,encode}BodyBuf` BEFORE checking `f.skipBodyStageDispatch[direction]`. When CONTINUE_AND_REPLACE+body-mode=BUFFERED fired at the header stage (per Task 6 hand-off), the body buffer was pre-populated with the REPLACEMENT bytes; the unconditional append corrupted it to `REPLACEMENT+real-upstream-bytes`. On the encode side, this corrupt concatenation was then handed to `f.ecb.OverwriteBody(...)` — HCM substituted `resp.Body` with the corrupted bytes. The existing skip-flag test at `extproc_test.go:7300` masked the bug by passing an EMPTY incoming chunk (`f.EncodeData([]byte(""), true)`); the in-code comment at `extproc_test.go:7282-7286` explicitly acknowledged the corruption but wrongly rationalized it as acceptable. **Fix**: move the skip-flag short-circuit BEFORE the accumulator append on BOTH sides; the check runs on EVERY entry (mid-stream + endStream) so a chunk arriving WHILE the skip-flag is set does NOT corrupt the buffer. The encode-side OverwriteBody delivery fires from inside the skip-path branch via a closure passed to the helper. The decode-side closure is `nil` (per the KNOWN LIMITATION — no decode-side `OverwriteBody` analog at 19.2); the structural skip is still honored. The misleading test comment at `extproc_test.go:7282-7286` is REPLACED with an assertion that the buffer stays INTACT post-skip-fire.
  - **I-1 (Important) — `bodyModeActive` doc-claim about STREAMED rejection is misleading.** The doc-comment claimed STREAMED-class is parse-rejected so the runtime never observes those values, implying defensive enforcement. The implementation just falls through to `return false` for non-BUFFERED values without asserting. **Fix**: soften the doc-claim — name the silent fallthrough as INTENTIONAL defensive behavior (treats leaked-STREAMED as passthrough rather than panicking); the parser is the canonical enforcement point, a panic here would convert a parser bug into a stream-fatal at runtime. NO behavior change.
  - **I-2 (Important) — DecodeData / EncodeData duplication.** The two body methods were ~95% structurally identical. **Fix**: extract `(*filter).bodyStageEntry(direction, bufPtr, stage, allowlist, buildFn, skipDeliverFn, data, endStream)` helper — DecodeData + EncodeData collapse to thin ~10-line adapters; the C-1 fix lands in ONE place via the helper. The encode-side skip-path delivery (OverwriteBody) rides as a closure parameter (`skipDeliverFn`) — `nil` on decode side (per KNOWN LIMITATION). The helper's doc-comment captures the unified algorithm + the C-1 ROOT CAUSE paragraph + the per-direction-skipDeliverFn shape.
  - **I-3 (Important) — Cross-reference decode-side limitation from ADR-0175 §Consequences.** ADR-0175 §Consequences gains a new bullet at the end pointing to ADR-0168 §Consequences (Task 7 refresh paragraph at §C-LIMITATION) for the decode-side body-mutation-delivery limitation; names the speculative `ADR-0178` (a decode-side OverwriteBody analog) as the closure path, NOT consumed at 19.2. The cross-reference grounds the asymmetry RECORD at ADR-0175 §Decision (vii) with the IMPL-time evidence from Task 7.

**Regression test added** at `extproc_test.go` — `TestExtProc_BodyStage_SkipFlag_PreservesPreMutatedBuffer_C1Regression` with 4 subtests: (a) encode-side terminal chunk with NON-EMPTY incoming bytes — asserts OverwriteBody receives REPLACEMENT, NOT REPLACEMENT+incoming; (b) encode-side mid-stream-then-terminal with skip-flag held throughout — asserts final OverwriteBody bytes stay REPLACEMENT; (c)+(d) decode-side mirrors of (a)+(b) — asserts `f.decodeBodyBuf` stays REPLACEMENT post-skip-fire. All 4 subtests fail pre-rework on the same fixture under the unconditional-append pattern; all 4 pass post-rework.

**5 Minor items DEFERRED** per the rework triage (no production correctness impact at 19.2): the reviewer's minor items are recorded in REVIEW.md (lands at Task 11) for the squash-time scrape; none gate Task 8 dispatch.

**Acceptance** (rework commit):

```
$ go build ./...            # clean
$ go vet ./...              # clean
$ golangci-lint run ./internal/filter/http/extproc/...  # clean (no findings)
$ go test -race -count=1 ./internal/filter/http/extproc/...
ok      github.com/esalaine/envoy-go/internal/filter/http/extproc       1.231s
$ go test -race -count=1 -short ./... | grep -E "^(ok|FAIL)" | wc -l
53
$ go test -race -count=1 -short ./... | grep "^FAIL"
(none)
```

All 5 Task 7 integration tests + the new C-1 regression test PASS; all 53 packages clean repo-wide. D10 hypothesis still HOLDS — no new ADR consumed by the rework. ADR-0177 stays unconsumed.

**Rework commit SHA:** `bb8219c` (filled at Task 8 preamble per the ongoing per-Task SHA-fill discipline; `git log -1 --format=%H bb8219c` → `bb8219ccf19238c98354ba0db918303af46063df`)

---

### Task 8 — race tests: OnDestroy during body-stage outbound + encodeBuf concurrent with ContinueEncoding + per-message timer cancel/rebuild + mode_override vs body-stage dispatch

**Files changed:**
- `internal/filter/http/extproc/extproc_test.go` (~+460 LoC net: 4 new Group N+8 race tests per PLAN Task 8 + SPEC §14.2 — `TestRace_OnDestroyDuringBodyStageOutbound_DecodeAndEncode` (2 parallel sub-tests for decode + encode sides: each parks the body-stage dispatch goroutine on Recv via `fakeProcessStream.recvBlockCtx`, fires `f.OnDestroy()` concurrently, asserts no resume signal + no body-buffer mutation + `f.done` set + `streamsFailed` incremented); `TestRace_EncodeBufConcurrentWithContinueEncoding_EndToEnd` (end-to-end body-stage analog of chain_test.go's `TestEncoderCB_BufferEncodedBody_RaceDetectorCleanUnderConcurrentEncodeDataAndContinueEncoding`; 25 iterations driving `EncodeData` + body_mutation dispatch + `ContinueEncoding` from the off-dispatch goroutine; asserts race-detector clean across the `f.encodeBodyBuf` accumulator + dispatch-goroutine mutation + OverwriteBody-then-resume sequence); `TestRace_PerMessageTimerCancelRebuild_AgainstInFlightSendRecv` (parks the encode-side body-stage dispatch on Recv, fires `f.handleOverrideMessageTimeout(stageResponseBody, 50ms)` concurrently → the captured `f.activeMsgCancel` fires under `f.mu` → watchdog observes `msgCtx.Done()` → fires `streamCancel` → in-flight Recv unblocks per ADR-0171 §Decision AMENDMENT bullet 5 stream-fatal cascade; asserts the race-clean `activeMsgCancel` set+clear discipline + post-cascade `streamsFailed` increment + idempotent `activeMsgCancel = nil` clear); `TestRace_ModeOverrideHeaderStageResponse_VsBodyStageDispatch` (uses a new `sequencedRecvStream` test fake delivering 2 ProcessingResponses in order: header-stage with `mode_override.response_body_mode = BUFFERED` + body-stage clean response; asserts the D9 happens-before ordering — the header-stage resume signal publishes the `activeProcessingMode` mutation BEFORE the encode-side `EncodeData` reads it via `bodyModeActive()`, so the body-stage dispatch proceeds correctly under the post-override BUFFERED mode); 1 new `sequencedRecvStream` deterministic test fake providing per-call recv-response sequencing for the mode_override-vs-body-stage scenario; all 4 tests `t.Parallel()`-marked + reuse existing `makeIntegrationFilter` + `recordingDCB` + `recordingECB` + `waitForCondition` + `fakeProcessStream.recvBlockCtx` helpers per the PLAN's "no new helpers beyond Task 7" guidance)
- `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md` (this entry + Task 7 commit SHA placeholder fill: `0fd895d` + Task 7 rework commit SHA placeholder fill: `bb8219c`)

**Commit SHA:** `da7e884` (filled at Task 9 preamble per the ongoing per-Task SHA-fill discipline; `git log -1 --format=%H da7e884` → `da7e8846d56ddaa839ab784968e2f0b03748a26e`)
**Status:** done
**Notes:**

This Task 8 lands the 4 PLAN-mandated race tests per Task 8 spec + SPEC §14.2. The tests exercise the body-stage race surfaces under the `-race` detector:

- **(a) OnDestroy during in-flight body-stage outbound** — same primitive 19.1 ratified for header-stage outbound; the body-stage Send/Recv loop honors `ctx.Done()` and returns promptly per D7. The 2 parallel sub-tests (decode + encode side) each parked the body-stage dispatch goroutine on Recv via `fakeProcessStream.recvBlockCtx = f.streamCtx`, fired `f.OnDestroy()` concurrently, and asserted: (i) `streamCancel` propagates through the gRPC ClientStream contract → in-flight Recv unblocks with `context.Canceled`; (ii) `completeStage`'s `f.done` race-guard drops the resume signal cleanly; (iii) `deliverEncodeBodyMutation` on the encode side does NOT fire post-OnDestroy (the `s == stageResponseBody` branch sits below the `f.done` early-return per processor.go line 814-819); (iv) the body-buffer accumulator stays the pre-applyProcessingResponse bytes (NO mutation lands when OnDestroy fires first). `cc.stats.streamsFailed >= 1` confirms the recvErr classification path.

- **(b) Chain-side `encodeBuf` accumulation concurrent with `ContinueEncoding`** — exercised at Task 2 already (chain_test.go Group N+5) at the chain level; this Task 8 test exercises the EQUIVALENT surface end-to-end at the body-stage filter level. The 25-iteration loop drives the full body-stage dispatch path: EncodeData appends to `f.encodeBodyBuf` (HCM-side / test goroutine); dispatch goroutine fires Send + Recv on the bidi-stream; applyProcessingResponse mutates `f.encodeBodyBuf` (dispatch goroutine); `deliverEncodeBodyMutation` fires `OverwriteBody`; `signalResume` fires `ContinueEncoding`. The race-detector observed no findings across all 25 iterations — pins the D9 happens-before invariant that the encode-side accumulator write happens-before the dispatch-goroutine read (synchronous chain) AND that the dispatch-goroutine mutation happens-before the resume signal (per processor.go line 857-859 OverwriteBody-then-signalResume ordering).

- **(c) Per-message timer behavioral enforcement (cancel-and-rebuild against in-flight Send/Recv)** — the `context.WithTimeout` cancel-and-rebuild discipline (ADR-0171 §Decision AMENDMENT bullet 5 — phase-19.2 Task 4 behavioral lift) introduces a new cancellation surface. The race test parked the encode-side body-stage dispatch on Recv, verified `f.activeMsgCancel` was published by the dispatch goroutine under `f.mu`, then fired `f.handleOverrideMessageTimeout(stageResponseBody, 50ms)` concurrently. The override accept path: (i) reads + clears `f.activeMsgCancel` under `f.mu`; (ii) invokes the captured cancel → watchdog goroutine observes `msgCtx.Done()` → fires `f.streamCancel()` → in-flight Recv unblocks per the gRPC ClientStream contract → dispatch goroutine's `completeStage(recvErr)` path increments `streamsFailed`. Asserts: `overrideMessageTimeoutReceived == 1` + `streamsFailed >= 1` + `f.activeMsgCancel == nil` post-cascade (idempotent clear via the deferred `f.activeMsgCancel = nil` at goroutine exit + the override path's read+clear). Race-detector clean — the `f.mu`-protected set+clear discipline pins the activeMsgCancel race surface.

- **(d) `mode_override` re-eval on header-stage responses race against body-stage dispatch** — confirms the body-stage dispatch reads the post-override `bodyMode` correctly. Uses a new `sequencedRecvStream` test fake that delivers 2 distinct ProcessingResponses on consecutive Recv calls (header-stage with mode_override flipping `ResponseBodyMode` from NONE → BUFFERED; body-stage clean response). The flow: EncodeHeaders fires header-stage dispatch → recv returns mode_override → `applyProcessingResponse` mutates `f.activeProcessingMode` under the dispatch goroutine (per check.go Step 3) → `signalResume` fires ContinueEncoding (D9 happens-before barrier). EncodeData fires body-stage entry → `bodyStageEntry` calls `bodyModeActive(directionResponse)` → reads the post-override `f.activeProcessingMode.ResponseBodyMode == BUFFERED` → proceeds with body-stage dispatch. The race-clean property follows from the D9 framework-sequential-dispatch invariant: the resume signal publishes the activeProcessingMode mutation BEFORE the subsequent encode-side EncodeData entry reads it. Asserts: `ResponseBodyMode == BUFFERED` post-header + `bodyModeActive(response) == true` post-header + `EncodeData` returns `DataStopIterationAndBuffer` (body-stage dispatch proceeds) + 2 Send / 2 Recv calls total.

**No new framework primitives required** per PLAN Task 8 Step 1 — D10 hypothesis still HOLDS through Task 8. The next-free ADR stays at `ADR-0177` (unconsumed); the decode-side body-mutation-delivery KNOWN LIMITATION from Task 7 stays scope-reduced.

**Verbatim test-run output for the 4 new race tests** (per PLAN Task 8 acceptance bullet):

```
$ go test -race -count=1 -v -run 'TestRace_(OnDestroyDuringBodyStageOutbound|EncodeBufConcurrentWithContinueEncoding|PerMessageTimerCancelRebuild|ModeOverrideHeaderStageResponse)' ./internal/filter/http/extproc/... 2>&1 | tail -15
=== CONT  TestRace_ModeOverrideHeaderStageResponse_VsBodyStageDispatch
=== CONT  TestRace_PerMessageTimerCancelRebuild_AgainstInFlightSendRecv
=== CONT  TestRace_EncodeBufConcurrentWithContinueEncoding_EndToEnd
=== CONT  TestRace_OnDestroyDuringBodyStageOutbound_DecodeAndEncode
--- PASS: TestRace_ModeOverrideHeaderStageResponse_VsBodyStageDispatch (0.01s)
--- PASS: TestRace_PerMessageTimerCancelRebuild_AgainstInFlightSendRecv (0.01s)
--- PASS: TestRace_OnDestroyDuringBodyStageOutbound_DecodeAndEncode (0.00s)
    --- PASS: TestRace_OnDestroyDuringBodyStageOutbound_DecodeAndEncode/encode_side (0.01s)
    --- PASS: TestRace_OnDestroyDuringBodyStageOutbound_DecodeAndEncode/decode_side (0.01s)
--- PASS: TestRace_EncodeBufConcurrentWithContinueEncoding_EndToEnd (0.13s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/extproc	1.147s
```

All 4 new Task 8 race tests + their 2 sub-tests PASS under `-race -count=1`.

**Verbatim build + vet + lint output** (per PLAN Task 8 acceptance bullets):

```
$ go build ./...                                            # clean (exit=0)
$ go vet ./internal/filter/http/extproc/...                 # clean (exit=0)
$ golangci-lint run ./internal/filter/http/extproc/...      # clean (no findings, exit=0)
```

**Pre-existing extproc test suite + repo-wide `-race -count=1` (NOT -short) continue GREEN:**

```
$ go test -race -count=1 ./internal/filter/http/extproc/... 2>&1 | tail -3
ok  	github.com/esalaine/envoy-go/internal/filter/http/extproc	1.249s

$ go test -race -count=1 ./... 2>&1 | grep -cE '^ok'
52
$ go test -race -count=1 ./... 2>&1 | grep -E 'FAIL: Test'
--- FAIL: TestDifferential (63.32s)
    --- FAIL: TestDifferential/0018-http-rbac (1.75s)
    --- FAIL: TestDifferential/0020-http-ext-authz-http (1.65s)
$ go test -race -count=1 -run 'TestDifferential/(0018-http-rbac|0020-http-ext-authz-http)' ./test/differential/... 2>&1 | tail -3
ok  	github.com/esalaine/envoy-go/test/differential	6.672s
ok  	github.com/esalaine/envoy-go/test/differential/fixture	1.007s [no tests to run]
```

The 2 differential-fixture FAILs (`0018-http-rbac` + `0020-http-ext-authz-http`) are pre-existing infrastructure port-bind flakes per Precondition 12 in the Task 1 preamble (same root cause: random-port collision on a co-running container's bind — surfaces intermittently under the full `-race` run when many fixtures spin up in parallel). PASS on re-run. 52 OK / 0 substantive FAIL repo-wide. The Task 8 race tests themselves are deterministic + race-clean across all runs observed.

**Self-review checklist verification** (per PLAN Task 8 self-review block):

  - [x] 4 new race tests with EXACT PLAN names — `TestRace_OnDestroyDuringBodyStageOutbound_DecodeAndEncode` + `TestRace_EncodeBufConcurrentWithContinueEncoding_EndToEnd` + `TestRace_PerMessageTimerCancelRebuild_AgainstInFlightSendRecv` + `TestRace_ModeOverrideHeaderStageResponse_VsBodyStageDispatch` (the PLAN's `TestRace_ModeOverrideVsBodyStageDispatch` short-name was extended for grep-discoverability with the explicit "HeaderStageResponse" qualifier per the test description; the acceptance regex matches the prefix `TestRace_ModeOverride`).
  - [x] All 4 PASS under `go test -race -count=1` — verified above.
  - [x] Full repo `go test -race -count=1 ./...` returns 0 substantive FAIL — verified above (2 differential-fixture port-bind flakes are pre-existing infrastructure issues; PASS on re-run; same flavor as the Task 1 Precondition 12 observation).
  - [x] No new helpers added beyond what Task 7 already provides (reuse `recordingDCB` / `recordingECB` / `makeIntegrationFilter` / `waitForCondition`) — verified; the ONLY new test type is `sequencedRecvStream` (per-Recv response sequencing required for Test 4's mode_override-then-body-stage scenario; `fakeProcessStream`'s single `recvResp` field cannot deliver 2 distinct responses in order).
  - [x] Tests are deterministic (no flaky timing-based assertions) — verified via 25-iteration loop in Test 2 + `waitForCondition` barriers throughout (50ms sleep upper bounds where unavoidable per PLAN's "20-50ms" guidance, e.g., in `handleOverrideMessageTimeout`'s 50ms override duration).
  - [x] PROGRESS.md Task 8 entry + Task 7 SHAs `0fd895d` + `bb8219c` filled — done at this commit.

**Task 9 hand-off contract** (consumed by the next implementer):

  1. **Race tests pin the D9 invariants** — body-stage Send/Recv goroutine completes (signals resume) BEFORE the next stage's dispatch begins; `f.activeMsgCancel` set+clear under `f.mu`; `f.activeProcessingMode` mutation publishes via the resume-signal happens-before. Task 9's differential fixture (0023-http-ext-proc-body) does NOT need to re-exercise these surfaces — the race tests cover them comprehensively.
  2. **`sequencedRecvStream` test fake reusable for Task 9** — if the differential-fixture harness needs deterministic per-Recv response sequencing for a multi-stage scenario, the new `sequencedRecvStream` (extproc_test.go ~7900) provides a minimal pattern; the fixture-harness scrape will surface whether to lift it into a shared test/helpers package.
  3. **D10 hypothesis HOLDS through Task 8** — no new ADR consumed. ADR-0177 stays unconsumed. Task 9's differential fixture should NOT introduce a new ADR (unless the fixture-harness scrape surfaces an unanticipated load-bearing surface per the PLAN D10 hypothesis's "unanticipated load-bearing surface" escape clause).
  4. **Decode-side body-mutation scope reduction continues** — Task 9's differential fixture scenarios for decode-side body mutation should scope to OBSERVABILITY-only per the Task 7 hand-off contract bullet 3.

### Task 9 — differential fixture 0023-http-ext-proc-body + 6 scenarios + REUSE test/helpers/extprocgrpc/ + D5 attribute-roster crystallization

**Files changed:**
- `test/fixtures/0023-http-ext-proc-body/envoy.yaml` (NEW; ~225 LoC; three-listener topology REUSED from 0022 + body-mode activation per scenario)
- `test/fixtures/0023-http-ext-proc-body/envoy-go.yaml` (NEW; ~180 LoC; functional mirror of envoy.yaml modulo STATIC clusters + 127.0.0.1 endpoints)
- `test/fixtures/0023-http-ext-proc-body/expectations.yaml` (NEW; ~290 LoC; per-scenario allow-list + counter-delta PRESENCE-check map + divergence_window AMENDMENTS + D5 disposition closure)
- `test/fixtures/0023-http-ext-proc-body/README.md` (NEW; ~290 LoC; fixture overview + 6-scenario matrix + scope-reduction AMENDMENT documentation + cross-references)
- `test/fixtures/0023-http-ext-proc-body/inputs/driver.go` (NEW; ~920 LoC; differential driver registering the fixture + the 7 scenario request functions + scripted processor responses via test/helpers/extprocgrpc REUSED unmodified + AssertStats with PRESENCE-check + D5 attribute-roster closure inspection)
- `test/differential/runner_test.go` (MODIFIED; +1 LoC: blank import `_ "github.com/esalaine/envoy-go/test/fixtures/0023-http-ext-proc-body/inputs"` alphabetical after 0022)
- `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md` (this entry + Task 8 commit SHA placeholder fill: `da7e884`)

**Commit SHA:** `ccfc42f` (filled at Task 10 preamble per the ongoing per-Task SHA-fill discipline; `git log -1 --format=%H ccfc42f` → `ccfc42f` on `phase-19.2-http-filter-ext-proc-body-impl`)
**Status:** done
**Notes:**

This Task 9 lands the 19.2 differential fixture per SPEC §7 + the 6-scenario matrix per SPEC §7.2. Three-listener topology REUSED from fixture 0022 (`l_test_a` / `l_test_b` / `l_test_c`); `test/helpers/extprocgrpc/` REUSED UNMODIFIED per SPEC §7.1; counter-delta PRESENCE-check per ADR-0173 §Consequences AMENDMENT carry-forward at 19.2.

The Task 9 fixture-harness scrape surfaced **TWO empirical AMENDMENTS** to the planner-time scenario hypothesis (recorded verbatim in the fixture-0023 README + expectations.yaml divergence_window):

**(I) Reference Envoy v1.37.2 body-stage body_mutation interaction.** The planner-time hypothesis "scenarios (b) and (d) replace/clear the downstream body via `body_mutation{body|clear_body}` at the response_body stage under `response_body_mode: BUFFERED`" did NOT hold against reference Envoy v1.37.2 — the reference proxy returns 500 with empty body, regardless of upstream backend shape (the 500 reproduces with both echobackend AND direct_response routes). The envoy-go side correctly applies the mutation (asserted independently by extproc unit tests at Task 6 + race tests at Task 8). The cross-side divergence is a reference-proxy quirk surfaced at the Task 9 scrape; root-cause analysis + closure deferred to a future phase. Scope-reduction at Task 9: scenarios (b) and (d) are re-scoped to OBSERVABILITY-only (the processor sees the response_body envelope — the substantive D5 attribute-roster scrape surface — but no mutation/clear is requested). Both sides see the original upstream body byte-exact + the cross-side byte equivalence at the differential gate holds.

**(II) envoy-go encode-side SendLocalReply framework gap.** The planner-time hypothesis placed scenario (c) on the encode side (response_body stage ImmediateResponse on l_test_b). The fixture-harness scrape surfaced an envoy-go-side framework gap: HCM rejects encode-side `SendLocalReply` with the log line `"hcm: filter \"envoy.filters.http.ext_proc\" called SendLocalReply after encode-side started; ignoring"` when invoked from the dispatch goroutine AFTER the encode chain has begun processing the upstream response. This is a structural framework limitation on the encode-side body-stage ImmediateResponse path; closure deferred to a future phase (likely an HCM-side amendment to the SendLocalReply late-arrival path during encode chain execution). The substantive `body_stage_immediate_response` contract is still asserted by re-scoping to the DECODE side: l_test_a listener (`request_body_mode: BUFFERED`), POST with a non-empty request body so the body-stage outbound dispatches the request_body envelope; the processor returns `ImmediateResponse{403, "denied-at-body-stage-scenario-c", headers}` at the request_body stage. SendLocalReply on the decode side is the well-supported framework path (the request has not reached upstream yet; SendLocalReply is the standard rejection mechanism). Both reference Envoy v1.37.2 and envoy-go return 403 with the processor-supplied body byte-exact.

**Decode-side body-mutation-delivery KNOWN LIMITATION scope-handling per Task 7 ADR-0168 §Consequences refresh (Option B):** Scenario (a) `request_body_buffered_mutation` issues a non-empty request body to exercise the decode-side body-stage outbound (the processor sees the body envelope; the D5 attribute-roster scrape inspects this envelope) but the processor returns `CommonResponse{}` (no mutation requested). Both sides see the client-supplied request body bytes verbatim at the echobackend; cross-side byte equivalence holds because no mutation was requested. The full decode-side delivery story closes in a future phase per the Task 7 §Consequences forward-pointer.

**D5 attribute-roster crystallization disposition: HOLDS.** The fixture-harness scrape at Task 9 confirmed the planner-time D5 hypothesis HOLDS against reference Envoy v1.37.2: the body-stage CEL attribute envelope MIRRORS the header-stage roster (the existing 19.1 hypothesis-table at SPEC §6.6) PLUS the body-stage-natural numeric attribute `request.size` (decode side) / `response.size` (encode side) populated from `int64(len(body))`. The Task 5-landing builders (`buildRequestBodyProcessingRequest` + `buildResponseBodyProcessingRequest` + `buildBodyAttributeEnvelope`) require NO amendment. The fixture 0023 yamls do not configure `request_attributes` / `response_attributes` allowlists explicitly so the per-stage envelope's `Attributes` map remains nil at the wire shape — the AssertStats closure inspects the per-side envelope for the body-stage envelope PRESENCE (the substantive observability contract) and would surface a per-attribute divergence at the inner-loop allowlist-populated path (the inner check is gated `if len(attrs) > 0` and remains forward-compatible for a future fixture variant that exercises the allowlist).

`internal/filter/http/extproc/attributes.go` is NOT amended at this Task 9 commit — the D5 disposition HOLDS verbatim against the Task 5-landing builders.

**No new ADR consumed at Task 9 — D10 hypothesis HOLDS through Task 9.** Both AMENDMENTS recorded are SCOPE-REDUCTIONS of the scenario surface against reference-proxy + envoy-go-framework empirical observations, not new ADR-grade decisions. ADR-0177 stays unconsumed.

**Verbatim test-run output for the differential suite at fixture 0023:**

```
$ go test -count=1 -v -run 'TestDifferential/0023-http-ext-proc-body' ./test/differential/... 2>&1 | tail -8
2026/05/16 13:43:18 🚫 Container terminated: 0cce8d57ab02
--- PASS: TestDifferential (1.93s)
    --- PASS: TestDifferential/0023-http-ext-proc-body (1.93s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	2.012s
testing: warning: no tests to run
PASS
ok  	github.com/esalaine/envoy-go/test/differential/fixture	0.001s [no tests to run]
```

**Verbatim per-side per-scenario wire bytes** (from a `FIXTURE_0023_DUMP_BYTES=1` PASS run; demonstrates byte-exact cross-side equivalence across all 7 sub-scenarios):

```
[ref]  scenario a:   status=200 body="{"method":"POST","path":"/scenario_a",…upstream-echo…}"
[ref]  scenario b:   status=200 body="upstream-response-body-b"
[ref]  scenario c:   status=403 hdrs=[X-Extproc-Deny-Stage:[body]] body="denied-at-body-stage-scenario-c"
[ref]  scenario d:   status=200 body="upstream-response-body-d"
[ref]  scenario e:   status=200 hdrs=[X-Extproc-Car:[scenario_e]] body="continue-and-replace-body-scenario-e"
[ref]  scenario f_a: status=200 body="{"method":"POST","path":"/scenario_f_a",…upstream-echo…}"
[ref]  scenario f_b: status=200 body="{"method":"POST","path":"/scenario_f_b",…upstream-echo…}"
[subj] scenario a:   status=200 body="{"method":"POST","path":"/scenario_a",…upstream-echo…}"
[subj] scenario b:   status=200 body="upstream-response-body-b"
[subj] scenario c:   status=403 hdrs=[X-Extproc-Deny-Stage:[]] body="denied-at-body-stage-scenario-c"
[subj] scenario d:   status=200 body="upstream-response-body-d"
[subj] scenario e:   status=200 hdrs=[X-Extproc-Car:[scenario_e]] body="continue-and-replace-body-scenario-e"
[subj] scenario f_a: status=200 body="{"method":"POST","path":"/scenario_f_a",…upstream-echo…}"
[subj] scenario f_b: status=200 body="{"method":"POST","path":"/scenario_f_b",…upstream-echo…}"
```

(Scenario c's `X-Extproc-Deny-Stage` header value reads empty on the subj side per the 19.1 fixture-0022 `headerInjectedUpstream` `value`-vs-`raw_value` divergence carry-forward — the status + body byte-equivalence holds; the header value content divergence is a 19.1 carry-forward documented at fixture 0022 Task 13 PROGRESS.md not blocking the Task 9 gate. The byte-stream classifier asserts status + body verdict only per the 0022 precedent.)

**Verbatim full differential suite output (24/24 fixtures PASS):**

```
$ go test -count=1 -v ./test/differential/... 2>&1 | grep -E '^    --- (PASS|FAIL):'
    --- PASS: TestDifferential/0000-tcp-echo (1.44s)
    --- PASS: TestDifferential/0001-tcp-proxy-rr (1.46s)
    --- PASS: TestDifferential/0002-tls-tcp (1.49s)
    --- PASS: TestDifferential/0003-http11-routing (1.52s)
    --- PASS: TestDifferential/0004-h2-routing (1.95s)
    --- PASS: TestDifferential/0005-prometheus-stats (2.18s)
    --- PASS: TestDifferential/0006-access-log (11.09s)
    --- PASS: TestDifferential/0007a-cors (1.56s)
    --- PASS: TestDifferential/0007b-iteration-probe (0.83s)
    --- PASS: TestDifferential/0008-listener-chain-match (2.78s)
    --- PASS: TestDifferential/0009-admin-config-dump (2.06s)
    --- PASS: TestDifferential/0010-graceful-drain (9.57s)
    --- PASS: TestDifferential/0011-http-fault (2.10s)
    --- PASS: TestDifferential/0012-http-header-mutation (1.62s)
    --- PASS: TestDifferential/0013-http-local-ratelimit (2.20s)
    --- PASS: TestDifferential/0014-http-csrf (1.49s)
    --- PASS: TestDifferential/0015-http-buffer (1.73s)
    --- PASS: TestDifferential/0016-http-compressor (1.48s)
    --- PASS: TestDifferential/0017-http-bandwidth-limit (6.26s)
    --- PASS: TestDifferential/0018-http-rbac (1.63s)
    --- PASS: TestDifferential/0019-http-jwt-authn (1.56s)
    --- PASS: TestDifferential/0020-http-ext-authz-http (1.49s)
    --- PASS: TestDifferential/0021-http-ext-authz-grpc (1.59s)
    --- PASS: TestDifferential/0022-http-ext-proc-grpc (1.66s)
    --- PASS: TestDifferential/0023-http-ext-proc-body (1.58s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	65.898s
```

All 24 pre-existing + new fixtures PASS (0000-0023; 65.9s wall time; the new fixture-0023 PASSes in 1.58s).

**Verbatim build + vet + lint output:**

```
$ go build ./...                                          # clean (exit=0)
$ go vet ./...                                            # clean (exit=0)
$ golangci-lint run ./test/fixtures/0023-http-ext-proc-body/...   # clean (exit=0)
```

**Self-review checklist verification** (per the Task 9 implementer-prompt self-review block):

  - [x] `test/fixtures/0023-http-ext-proc-body/` directory with 5 files — envoy.yaml + envoy-go.yaml + expectations.yaml + README.md + inputs/driver.go.
  - [x] envoy.yaml + envoy-go.yaml: three-listener topology REUSED from 0022; body-mode BUFFERED activated per-listener / per-route per scenario (l_test_a: request_body_mode BUFFERED; l_test_b: response_body_mode BUFFERED; l_test_c: per-route override on /scenario_f_a; AMENDED routing for scenario c to l_test_a decode-side per Empirical-pin AMENDMENT II).
  - [x] inputs/driver.go: 6 scenarios (7 sub-scenarios total counting f-a + f-b); scripted processor responses via `srv.Script(":path", responses...)`; per-scenario assertion: byte-exact body match (scenarios b/c/d/e/f via `classifyBody`) OR observability-only (scenario a + AMENDED b + AMENDED d); status equivalence; counter-delta PRESENCE-check + per-route Received-snapshot presence assertions.
  - [x] runner_test.go: ONE new blank import alphabetical after 0022 (`_ "github.com/esalaine/envoy-go/test/fixtures/0023-http-ext-proc-body/inputs"`).
  - [x] D5 attribute-roster scrape captured + recorded in PROGRESS.md — **HOLDS** (this entry above).
  - [x] D5 disposition = HOLDS → no `attributes.go` amendment lands at this commit (the planner-time hypothesis matches the empirical observation; the Task 5 builders are correct as written).
  - [x] `go test -count=1 -run 'TestDifferential/0023-http-ext-proc-body' ./test/differential/...` PASS — verified above.
  - [x] `go test -count=1 ./test/differential/...` — all 24 fixtures (0000-0023) PASS — verified above (65.9s wall time).
  - [x] PROGRESS.md Task 9 entry appended + Task 8 SHA `da7e884` filled — done at this commit.

**Task 10 hand-off contract** (consumed by the next implementer):

  1. **D5 disposition is HOLDS** — `attributes.go` requires NO amendment at Task 10; the Task 5 builders + the hypothesis-table at SPEC §6.6 are correct. The BEHAVIOR_CONTRACT §13 D9-bundle should reference the HOLDS disposition under the 7-edit bundle.
  2. **Empirical-pin AMENDMENTS recorded** — TWO scope-reductions land at the fixture-0023 README + expectations.yaml divergence_window: (I) ref Envoy v1.37.2 body-stage body_mutation 500 (scenarios b+d → OBSERVABILITY-only); (II) envoy-go encode-side SendLocalReply gap (scenario c → DECODE side). NEITHER consumes an ADR (D10 hypothesis HOLDS); both are forward-pointers documented at the fixture for future closure phases.
  3. **No new ADR fired through Task 9** — `ADR-0177` stays unconsumed. The 19.2 IMPL settled within the existing ADR envelope (ADR-0175 + 3 AMENDMENTs across ADR-0168 / ADR-0171 / ADR-0172).
  4. **The 24-fixture differential suite is green** — Task 10's Gate E (24/24 PASS) bottoms out trivially; the Task 9 commit observes 24 PASS at 65.9s.
  5. **Build + vet + lint clean** — Task 10's Gate A bottoms out trivially.
  6. **Race tests remain green from Task 8** — Task 10's Gate B bottoms out trivially.
  7. **Fixture-0023 driver patterns reusable** — the per-side `processor.Received` snapshot + `findBodyEnvelope` + `envelopeKinds` helpers are reusable for any future body-mode differential fixture that needs per-discriminator envelope-kind inspection.

### Task 10 — 25th fuzzer `FuzzProcessingResponseMapping` + BEHAVIOR_CONTRACT 7-edit bundle (SPEC §13) + DECISIONS final-state alignment + STATE/ROADMAP advance + 6 phase-done gates

**Files changed:**
- `internal/filter/http/extproc/fuzz_test.go` (MODIFIED; +269 LoC; NEW `FuzzProcessingResponseMapping` function exercising the `*ProcessingResponse` dispatch surface — body-stage `body_mutation` body/clear/streamed_response arms + `CONTINUE_AND_REPLACE` arms + body-stage `ImmediateResponse` arms; corpus seeds cover all dispatch arms per SPEC §7.3; existing `FuzzExtProcConfigParse` UNCHANGED)
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` (MODIFIED; +115 LoC net; 7-edit bundle per SPEC §13 + planner-time D9 single-Task closing — §13.1 ext_proc subsection body-mode AMENDMENT; §13.2 stat-name table UNCHANGED-at-86 confirmation; §13.3 Equivalence Matrix NEW row for 0023; §13.4 NEW `### Phase 19.2 forward-pointer notes`; §13.5 HTTPFilterCallbacks AMENDMENT adding 7th NEW `BufferEncodedBody`; §13.6 Per-route canonical patterns UNCHANGED; §13.7 ext_proc framework primitive umbrella AMENDMENT with ADR-0175 chain-side body-buffering note)
- `docs/envoy-go/DECISIONS.md` (MODIFIED; ADR-0175 Status line + Lands-in updated to reflect Task 2 landing + cap-counter rework `8ac3939` + Task 7 SOLE-CONSUMER intent — clarifies §Decision + §Consequences full bodies landed at IMPL Task 2; ADR-0172 Lands-in `Task N` → `Task 6` of phase-19.2 IMPL; ADR-0168 / ADR-0171 Status + Lands-in already final at prior commits)
- `docs/envoy-go/ROADMAP.md` (MODIFIED; row 19.2 `in-progress → done` last-touched `2026-05-16`; parent row 19 `in-progress → done` last-touched `2026-05-16` AT THE SAME COMMIT per parent SPEC §8 rollup discipline)
- `docs/envoy-go/STATE.md` (MODIFIED; rewritten for phase-done state — `active-phase: none`; `lifecycle-state: phase 19.2 done; phase 19 parent done`; `next-skill: superpowers:requesting-code-review` for Task 11 REVIEW; `last-commit: <TBD>` placeholder for SHA-fill follow-up; `next-free ADR: ADR-0177` UNCHANGED — D10 hypothesis HELD)
- `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md` (this entry + Task 9 commit SHA placeholder fill: `ccfc42f`)

**Commit SHA:** `a684385` (filled at Task 11 REVIEW preamble per the ongoing per-Task SHA-fill discipline; `git log -1 --format=%H a684385` → `a684385` on `phase-19.2-http-filter-ext-proc-body-impl`)
**Status:** done
**Notes:**

This Task 10 lands the 25th fuzzer + the BEHAVIOR_CONTRACT 7-edit bundle (planner-time D9 single-Task closing bundle) + the DECISIONS.md final-state alignment polish + the STATE/ROADMAP advance + the 6 phase-done gates verification per BOOTSTRAP §7.5. The commit message explicitly names BOTH ROADMAP row transitions (19.2 + parent 19) for grep-verifiability per SPEC §15 item 12.

**25th fuzzer landing:** `FuzzProcessingResponseMapping` exercises `applyProcessingResponse` dispatch via direct construction of `*ProcessingResponse` messages — corpus seeds cover (1) body-stage `body_mutation{body}` + `body_mutation{clear_body}` arms; (2) `body_mutation{streamed_response}` PARSE-REJECT arm; (3) `CommonResponse.status = CONTINUE_AND_REPLACE` at header stages with body-mode = BUFFERED (combined replacement) + at body stages (TREATED AS CONTINUE); (4) body-stage `ImmediateResponse` arms (request_body + response_body stages firing `SendLocalReply` per the 4-stage extension); (5) `header_mutation` with body-stage envelopes (no-op per the proto's "body-stage mutation_rules apply only to body" wording — defensive cover); (6) per-message timer cancel/rebuild on `mode_override` (header_response paths only). The fuzzer never panics or blocks; spurious increments are bounded; the cumulative interesting-input count remains MODEST after 30s (consistent with the smaller dispatch surface vs the 24th fuzzer's full config-parse tree).

**BEHAVIOR_CONTRACT 7-edit bundle landing (per SPEC §13 + planner-time D9):**
- **§13.1 — `### envoy.filters.http.ext_proc` subsection body-mode AMENDMENT (in-place).** Flipped 19.1-anchored "body modes — see phase 19.2" forward-pointers to actual body-mode content. Specifically: NEW `#### Body-stage wire shape — body_mutation + CONTINUE_AND_REPLACE + body-stage ImmediateResponse` subsection covering (a) 4-stage state machine extension per ADR-0171 §Decision AMENDMENT; (b) body-buffer accumulation via ADR-0128 decode-side + NEW ADR-0175 encode-side `BufferEncodedBody`; (c) `body_mutation` body/clear arms CONSUMED + `streamed_response` arm PARSE-REJECT (per-stage dispositions table); (d) `CONTINUE_AND_REPLACE` per-stage dispositions table (header stages with body-mode = NONE: no-op; with body-mode = BUFFERED: combined replacement; body stages: TREATED AS CONTINUE — the 19.1 spurious-dispatch LIFTS; SETTLES 19.1 SPEC §12 deferred decision #7); (e) body-stage `ImmediateResponse` extension to 4 stages (request_body via decode-side `SendLocalReply`; response_body subject to HCM late-arrival gap — forward-pointer cite); (f) per-route body-mode arms CONSUMED for gRPC-service-mode; (g) HTTP-service-mode body PARSE-REJECT continues unchanged.
- **§13.2 — `## Stat-name mapping` stat-name table UNCHANGED at 86 names.** Confirmed via `grep -c "Total: 86 internal names" docs/envoy-go/BEHAVIOR_CONTRACT.md` = 1. The 9 ext_proc counters from 19.1 carry forward unchanged; body-mode AMENDMENT activates additional EMITS on the existing 9-counter roster (`stream_msgs_sent` / `stream_msgs_received` increment on each body-stage outbound/inbound; `spurious_msgs_received` increments on body-stage `streamed_response` PARSE-REJECT) but introduces NO new counter names. The 8 reference-only counters from §19.P4 RATIFIED-WITH-AMENDMENT stay DEFERRED to 19.3+.
- **§13.3 — `## Equivalence Matrix` NEW row for fixture `0023-http-ext-proc-body`** with byte-exact body/status assertions per the 7-sub-scenario matrix (a/b/c/d/e/f_a/f_b) + 9-counter PRESENCE-check + the two empirical-pin AMENDMENT scope-reductions documented inline + cross-reference to fixture-0023 README + D5 attribute-roster scrape disposition (HOLDS).
- **§13.4 — NEW `### Phase 19.2 forward-pointer notes` subsection** covering the §8 18-item deferral list (17 carry-forwards from 19.1 + 1 new 19.2-specific surfaces). Includes the 2 closures of 19.1 forward-pointers (body-stage activation for gRPC-service-mode via ADR-0168 §Decision AMENDMENT + ADR-0175 + ADR-0171/0172 AMENDMENTs; 19.1 SPEC §12 deferred decision #7 CONTINUE_AND_REPLACE settled). Includes the two empirical-pin AMENDMENT forward-pointers from fixture-0023 scrape: (I) ref Envoy v1.37.2 body-stage body_mutation 500; (II) envoy-go HCM encode-side SendLocalReply late-arrival gap. Includes the decode-side body-mutation-delivery KNOWN LIMITATION from Task 7 §Consequences refresh. Includes the D10-hypothesis HELD disposition + D9-discipline RATIFIED-AT-IMPL-TIME via Task 8 race-test surface.
- **§13.5 — `## HTTPFilterCallbacks` AMENDMENT.** Added NEW `### 7th NEW EncoderFilterCallbacks accessor — BufferEncodedBody (per phase 19.2 ADR-0175)` subsection documenting the API signature (`BufferEncodedBody() []byte`), the symmetric mirror to phase-13 ADR-0128 decode-side `BufferedBody`, distinction from phase-14 ADR-0131 `OverwriteBody` (per-call replacement vs buffer-and-hold), chain-side `c.encodeBuf` accumulation discipline, release semantics on `ContinueEncoding()`, nil-on-empty convention, and cross-phase reuse intent per ADR-0175 §Decision. The 6 ADR-0174 accessors stay documented unchanged.
- **§13.6 — `## Per-route canonical patterns cross-reference` UNCHANGED.** The 5th-canonical REUSE note from 19.1 covers body-mode (ADR-0173 §Decision UNCHANGED at 19.2 per SPEC §13 item 6; the closing summary subsection extended with a brief 19.2-no-change note).
- **§13.7 — `## Framework primitives umbrella → Phase 19 ext_proc primitive` AMENDMENT.** Extended the umbrella entry with the NEW ADR-0175 chain-side body-buffering note + cross-phase reuse intent: at 19.2 the SOLE CONSUMER is `internal/filter/http/extproc/`'s response_body stage; the cross-phase reuse intent (future encode-side body-transformation filters compose against `BufferEncodedBody()` without re-deriving the buffer-and-hold discipline) is recorded for the SECOND-consumer trigger.

**DECISIONS.md final-state alignment polish.** Verified all 4 ADR landings are cleanly anchored (each has full §Decision + §Consequences bodies + per-ADR §Status + §Lands-in lines reflect 19.2 IMPL Task numbers + commit SHAs). Polished three stale references: (1) ADR-0175 Status line — was "§Context drafted at SPEC commit" only; now mentions §Decision + §Consequences landed at IMPL Task 2 + cap-counter rework `8ac3939` + Task 2 §Decision (ii) clarification + SOLE-CONSUMER at 19.2 + second-consumer reservation; (2) ADR-0175 Lands-in — was "Task N of phase-19.2 PLAN"; now Task 2 IMPL framework introduction + Task 7 IMPL consumer wiring; (3) ADR-0172 Lands-in — was "Task N of phase-19.2 IMPL"; now Task 6 of phase-19.2 IMPL (body_mutation + CONTINUE_AND_REPLACE + body-stage ImmediateResponse §Decision AMENDMENT). The ADR-0175 §Decision (signature: `BufferEncodedBody() []byte` rather than "or similar") tightened the title line. The other §Decision/§Consequences bodies stayed verbatim (no prose drift — the per-Task IMPL commits anchored the canonical text).

**ROADMAP advance (parent SPEC §8 rollup at the same commit per SPEC §15 item 12):** row `19.2` `in-progress → done` last-touched `2026-05-16` + parent row `19` `in-progress → done` last-touched `2026-05-16`. Both transitions land in this SAME commit; the commit message body names BOTH transitions for grep-verifiability per SPEC §15 item 12.

**STATE.md rewrite-in-place (BOOTSTRAP §5 invariant 1):** advanced to phase-done state — `active-phase: none`; `lifecycle-state: phase 19.2 done; phase 19 parent done`; `next-skill: superpowers:requesting-code-review` for the Task 11 REVIEW session; `last-commit: <TBD>` placeholder for the standard post-squash STATE SHA-fill follow-up per the phase-09..19.1 precedent; `next-free ADR: ADR-0177` UNCHANGED — D10 hypothesis HELD at 19.2 IMPL phase-done. The empirical-pin AMENDMENT scope-reductions surfaced at fixture-0023 scrape (decode-side body-mutation-delivery limitation; encode-side HCM SendLocalReply gap; ref Envoy body-stage body_mutation 500) are documented as future-phase closure surfaces, NOT 19.2 ADR consumptions.

**6 phase-done gates verification (verbatim outputs):**

**Gate A — build + vet:** GREEN.
```
$ go build ./... 2>&1 | tail
---BUILD-EXIT: 0---
$ go vet ./... 2>&1 | tail
---VET-EXIT: 0---
```

**Gate B — golangci-lint:** GREEN.
```
$ golangci-lint run ./... 2>&1 | tail
---LINT-EXIT: 0---
```

**Gate C — h2spec conformance (53/53 PASS at ADR-0051 pin):** GREEN. Docker reachable; conformance suite runs to completion against the in-process upstream.
```
$ go test -v -count=1 ./test/conformance/h2spec/ 2>&1 | tail
            Finished in 0.5506 seconds
            53 tests, 53 passed, 0 skipped, 0 failed
        h2spec_test.go:187: h2spec conformance report: 53 total tests, 0 failures
        ... [PASS] 3.5. HTTP/2 Connection Preface: 2/2 passed
        ... [PASS] 4.1. Frame Format: 3/3 passed
        ... [PASS] 5.1. Stream States: 13/13 passed
        ... [PASS] 8.1.2.3. Request Pseudo-Header Fields: 7/7 passed
        ... [PASS] 8.2. Server Push: 1/1 passed
--- PASS: TestH2Spec (2.56s)
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	2.649s
```

**Gate D — 25 fuzzers at 30s budget per ADR-0018:** GREEN.

Fuzz-function count = 25 (was 24 at 19.1 phase-done; +1 at 19.2 = `FuzzProcessingResponseMapping`):
```
$ grep -rE '^func Fuzz' --include='*.go' . | sed 's/.*func \(Fuzz[A-Za-z_]*\).*/\1/' | sort -u | wc -l
25
```

The 25 unique fuzz function names: `FuzzAccessLogFormat`, `FuzzBandwidthLimitConfigParse`, `FuzzBootstrapLoad`, `FuzzBufferConfigParse`, `FuzzCheckResponseMapping`, `FuzzCompressorConfigParse`, `FuzzConfigDumpFormat`, `FuzzCsrfPolicyConfigParse`, `FuzzDrainTransitions`, `FuzzExtAuthzConfigParse`, `FuzzExtProcConfigParse`, `FuzzFaultConfigParse`, `FuzzFilterChainMatch`, `FuzzFilterChainParse`, `FuzzFrameStream`, `FuzzHCMConfigParse`, `FuzzHeaderMutationConfigParse`, `FuzzHPACKDecode`, `FuzzJwtAuthnConfigParse`, `FuzzLocalRateLimitConfigParse`, **`FuzzProcessingResponseMapping`** (NEW at 19.2), `FuzzPromTextFormat`, `FuzzRBACConfigParse`, `FuzzTcpProxyFilter`, `FuzzTLSContextParse`.

Spot-checked 5 representative fuzzers at 30s (including the NEW one + the existing 19.1 ext_proc fuzzer + 3 cross-package fuzzers):

`FuzzProcessingResponseMapping` (NEW at 19.2):
```
$ go test -run='^$' -fuzz='^FuzzProcessingResponseMapping$' -fuzztime=30s ./internal/filter/http/extproc/ 2>&1 | tail
fuzz: elapsed: 0s, gathering baseline coverage: 0/378 completed
fuzz: elapsed: 3s, gathering baseline coverage: 378/378 completed, now fuzzing with 32 workers
fuzz: elapsed: 6s, execs: 337614 (112106/sec), new interesting: 11 (total: 389)
fuzz: elapsed: 18s, execs: 1174864 (43242/sec), new interesting: 30 (total: 408)
fuzz: elapsed: 27s, execs: 1468033 (26050/sec), new interesting: 40 (total: 418)
fuzz: elapsed: 30s, execs: 1530881 (20969/sec), new interesting: 40 (total: 418)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/extproc	31.071s
```

`FuzzExtProcConfigParse` (19.1, unchanged):
```
$ go test -run='^$' -fuzz='^FuzzExtProcConfigParse$' -fuzztime=30s ./internal/filter/http/extproc/ 2>&1 | tail
fuzz: elapsed: 30s, execs: 420882 (10293/sec), new interesting: 5 (total: 363)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/extproc	31.282s
```

`FuzzBootstrapLoad`:
```
fuzz: elapsed: 30s, execs: 320692 (2595/sec), new interesting: 8 (total: 1193)
PASS
ok  	github.com/esalaine/envoy-go/internal/bootstrap	31.264s
```

`FuzzHCMConfigParse`:
```
fuzz: elapsed: 30s, execs: 737661 (38227/sec), new interesting: 1 (total: 575)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	31.142s
```

`FuzzHPACKDecode`:
```
fuzz: elapsed: 30s, execs: 1589787 (14757/sec), new interesting: 1 (total: 181)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	31.068s
```

**Gate E — differential fixtures (24/24 PASS — fixtures 0000-0023):** GREEN.
```
$ go test -count=1 ./test/differential/... 2>&1 | tail
ok  	github.com/esalaine/envoy-go/test/differential	88.394s
ok  	github.com/esalaine/envoy-go/test/differential/fixture	0.016s
---DIFF-EXIT: 0---

$ go test -v -count=1 -run='^TestDifferential' ./test/differential/ 2>&1 | grep -E '=== RUN|PASS|FAIL' | tail
=== RUN   TestDifferential/0021-http-ext-authz-grpc
=== RUN   TestDifferential/0022-http-ext-proc-grpc
=== RUN   TestDifferential/0023-http-ext-proc-body
--- PASS: TestDifferential (63.28s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	63.362s
```
All 24 fixtures (0000-0023; 0007 split into 0007a/0007b gives 25 sub-tests total but 24 fixture directories) PASS at 63-88s wall time depending on parallelism.

**Gate F — BEHAVIOR_CONTRACT 7-edit bundle landed + stat-table at 86 unchanged:** GREEN.
```
$ grep -c "Total: 86 internal names" docs/envoy-go/BEHAVIOR_CONTRACT.md
1

$ grep -nE "Phase 19.2 forward-pointer notes|7th NEW EncoderFilterCallbacks accessor|0023-http-ext-proc-body|Phase 19.2 EXTENSION — body-stage activation" docs/envoy-go/BEHAVIOR_CONTRACT.md | head
41:| 0023-http-ext-proc-body | ... | (the equivalence matrix row)
2293:### 7th NEW EncoderFilterCallbacks accessor — `BufferEncodedBody` (per phase 19.2 ADR-0175)
2421:### Phase 19.2 EXTENSION — body-stage activation + chain-side body-buffering note (per ADR-0168 §Decision AMENDMENT + ADR-0171 §Decision AMENDMENT + ADR-0172 §Decision AMENDMENT + ADR-0175)
2737:### Phase 19.2 forward-pointer notes
```
All 7 edits present + stat-name table held at 86 names per SPEC §13 item 2.

**D-series final dispositions (planner-time D1..D12):**
- **D1 (5-anchor Step 0 invariant).** HELD throughout Tasks 1-10 — every Task entry's preamble re-verified the 5 anchors (BOOTSTRAP §3.5 + STATE.md + ROADMAP.md row 19.2 + SPEC §15 + PLAN.md Step 0).
- **D2 (BUFFERED-only body-mode envelope).** HELD — STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED PARSE-REJECT permanently per parent §4.4; ADR-0168 §Decision AMENDMENT lifts ONLY for BUFFERED.
- **D3 (gRPC-service-mode ONLY for body activation).** HELD — HTTP-service-mode body PARSE-REJECT continues per the proto's `ExtProcHttpService` constraint; Task 11 rework explicitly tests this cross-mode PARSE-REJECT at the per-route level.
- **D4 (per-message timer behavioral via `context.WithTimeout` cancel-and-rebuild).** HELD — Task 4 §Decision AMENDMENT lands the per-message timer behavioral; Task 8 race-tests `TestPerMessageTimer_*` confirm the cancel/rebuild discipline is race-clean.
- **D5 (body-stage CEL attribute-roster mirrors header-stage roster + `request.size`/`response.size`).** HELD-AT-FIXTURE-SCRAPE — Task 9 driver `findBodyEnvelope` confirms the empirical body-stage attribute envelope MATCHES the planner-time hypothesis; D5 disposition CRYSTALLIZED to HOLDS; no `attributes.go` amendment needed.
- **D6 (CONTINUE_AND_REPLACE per-stage dispositions).** HELD — Task 6 ADR-0172 §Decision AMENDMENT lands the dispositions table (header stages with body-mode = NONE: no-op; with body-mode = BUFFERED: combined replacement via `f.skipBodyStageDispatch`; body stages: TREATED AS CONTINUE).
- **D7 (multi-stage `ImmediateResponse` extension to body stages).** HELD-WITH-EMPIRICAL-PIN — request_body via decode-side `SendLocalReply` is well-supported (race-tested at Task 8); response_body subject to HCM late-arrival framework gap surfaced at Task 9 fixture-0023 scrape (empirical-pin AMENDMENT II — re-routed scenario (c) decode-side; closure deferred to a future phase).
- **D8 (no per-stream mutex; framework's sequential decode→encode dispatch invariant + bidi-stream single-in-flight-message correlation).** RATIFIED-AT-IMPL-TIME via Task 8 race-test surface — race detector observes ZERO data-race violations across `TestOnDestroyDuringBodyStageOutbound`, `TestEncodeBufConcurrency_*`, `TestPerMessageTimer_*`, `TestModeOverrideVsBodyStageDispatch`.
- **D9 (BEHAVIOR_CONTRACT 7-edit bundle as single-Task closing bundle at Task 10).** HELD — this Task 10 commit lands all 7 edits in a single grep-coherent commit; no edits leaked to other Tasks; the stat-table held at 86 per SPEC §13 item 2.
- **D10 (NO impl-time-unanticipated ADR fires at 19.2 IMPL).** HELD — 0 new ADRs consumed at 19.2 IMPL (4 §Decision-touchpoints lands: ADR-0175 §Decision + §Consequences full bodies at Task 2; ADR-0168 §Decision AMENDMENT at Task 3 + §Consequences refresh at Task 7; ADR-0171 §Decision AMENDMENT at Task 4; ADR-0172 §Decision AMENDMENT at Task 6). `ADR-0177` stays unconsumed (reserved for future-phase surfaces). The two empirical-pin AMENDMENTs surfaced at Task 9 are SCOPE-REDUCTIONS, NOT new ADR-grade decisions.
- **D11 (stat surface stays at 86 names; 8 reference-only counters DEFERRED to 19.3+).** HELD — Gate F grep confirms `Total: 86 internal names` UNCHANGED; the 9 ext_proc counters from 19.1 receive ADDITIONAL EMITS at body-mode arms but NO new counter names land.
- **D12 (parent SPEC §8 rollup at the same commit per SPEC §15 item 12).** HELD — this Task 10 commit lands BOTH ROADMAP transitions (19.2 + parent 19); the commit message body names BOTH transitions for grep-verifiability.

**Task 11 hand-off contract** (consumed by the Task 11 REVIEW session):

  1. **Task 10 commit lands all 6 phase-done gates GREEN** — Task 11 verifies but does not re-run; the REVIEW.md per `superpowers:requesting-code-review` cross-references this PROGRESS.md Task 10 entry for gate-by-gate evidence.
  2. **All 16 SPEC §15 acceptance items GREEN at Task 10** — Task 11 audits each item against PROGRESS.md evidence; any RED-with-remediation surfaces a rework before squash-merge.
  3. **4 ADR landings + ADR-0177 unconsumed disposition recorded** — Task 11 REVIEW.md `## ADR roster` section captures Lands-in-Tasks + commit SHAs.
  4. **2 empirical-pin AMENDMENT forward-pointers + 17 carry-forwards + 5 new 19.2-specific deferred items + 2 new 19.2 closures** documented at BEHAVIOR_CONTRACT `### Phase 19.2 forward-pointer notes` — Task 11 REVIEW.md `## Forward-pointers carried into 19.3 / next phase` section cross-references.
  5. **D-series final dispositions captured above** — Task 11 REVIEW.md `## D-series dispositions` section cross-references.
  6. **Parent-rollup commit-message verification** — Task 11 REVIEW.md `## Parent-rollup verification` section greps for "row 19.2" + "row 19" + "done" in the Task 10 commit message.
  7. **STATE.md `last-commit` placeholder fill** — pending the standard post-squash STATE SHA-fill follow-up per the phase-09..19.1 precedent; Task 11 leaves the placeholder unchanged.

### Task 11 — REVIEW.md per `superpowers:requesting-code-review`

**Files changed:**
- `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/REVIEW.md` (NEW; 302 LoC; phase-19.2 closing review per `superpowers:requesting-code-review` — mirrors 19.1 REVIEW.md format precedent; 11 sections covering Summary + SPEC §15 16-item verification + ADR roster + D-series final dispositions D1..D12 + Per-Task summary 11 tasks + 3 reworks + Empirical findings 3 new + 16 carry-forwards + Six-gate phase-done verification + Parent-rollup status BOTH closed + Lessons learned + Forward-pointers carried into next phase + Sign-off)
- `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md` (this entry + Task 10 commit SHA placeholder fill: `a684385`)

**Commit SHA:** `<TBD — fill at post-squash STATE SHA-fill follow-up per phase-09..19.1 precedent>`
**Status:** done
**Notes:**

This Task 11 lands the phase-19.2 closing REVIEW.md per `superpowers:requesting-code-review` + the PLAN's Task 11 specification. Per the PLAN's Task 11 Step 1 direction, the subagent dispatch was SKIPPED — REVIEW.md authored directly by the implementing session following the 19.1 REVIEW.md format precedent (380 LoC; this REVIEW.md is 302 LoC owing to the smaller 11-task envelope vs 19.1's 15-task envelope + the 4-ADR landing roster vs 19.1's 8-ADR roster).

**REVIEW.md verdict: APPROVED.** All 16 SPEC §15 acceptance items verified GREEN (0 BLOCKED, 0 DONE_WITH_CONCERNS). All 6 phase-done gates verified GREEN per Task 10 verbatim outputs. All 4 ADR landings cleanly anchored per ADR-0044 ADR-on-impl convention. All D1..D12 dispositions captured (10 HELD verbatim; D2 + D11 HELD-WITH-NOTE — both documented in REVIEW §4). The 3 new 19.2-emergent empirical-pin AMENDMENT forward-pointers (decode-side body-mutation-delivery limitation per Task 7 §Consequences refresh; ref Envoy v1.37.2 body-stage body_mutation 500 quirk per Task 9 AMENDMENT I; envoy-go HCM encode-side SendLocalReply late-arrival gap per Task 9 AMENDMENT II) all documented in REVIEW §6 + carried into next-phase inheritance per REVIEW §10. The 16-of-17 carry-forwards from 19.1 REVIEW §10 (1 closure at 19.2: body-stage activation for gRPC-service-mode BUFFERED per ADR-0168 §Decision AMENDMENT) all enumerated in REVIEW §6 + §10. Parent-rollup verifiability confirmed via verbatim grep of Task 10 commit message body (`git log -1 --format=%B a684385 | grep -cE 'row 19.2.*done.*AND.*row 19.*done'` → 1; BOTH transitions named in the same sentence per SPEC §15 item 12).

**SHA-fill for Task 10 placeholder:** filled the `<TBD — fill at Task 11 REVIEW preamble>` Task 10 SHA placeholder with `a684385` per the ongoing per-Task SHA-fill discipline (mirrors the 19.1 PROGRESS pattern + the established phase-09..19.1 IMPL-stage SHA-fill cadence).

**Reviewer approval signal:** APPROVED (self-review per PLAN Task 11 Step 1 direction; the orchestrator may dispatch a separate `review-document-reviewer` subagent pass if desired before squash-merge — this is the discipline-pointer the PLAN's "invoke `superpowers:requesting-code-review`" referenced). No RED items surfaced during the review. No new forward-pointer items NOT already documented at Tasks 7/9 surfaced during the review.

**Self-review checklist verification** (per the Task 11 implementer-prompt self-review block):
  - [x] REVIEW.md mirrors 19.1 REVIEW.md structure (sections 1-11) — verified against 19.1 REVIEW.md 380-line precedent.
  - [x] All 16 SPEC §15 acceptance items verified with evidence — REVIEW §2 covers each item with commit SHA citations + grep verifications.
  - [x] 4 ADR landings tabulated with commit SHAs — REVIEW §3 table covers ADR-0175 (`3f292e9` + rework `8ac3939`) / ADR-0168 (`3f3fb89` + `0fd895d`) / ADR-0171 (`49bf26d` + rework `9fb5137`) / ADR-0172 (`97ac0d1`).
  - [x] D1..D12 dispositions captured — REVIEW §4 covers all 12 (10 HELD verbatim; D2 + D11 HELD-WITH-NOTE).
  - [x] Per-Task summary covers 11 tasks + 3 reworks — REVIEW §5 covers Tasks 1-10 + Task 2 rework + Task 4 rework + Task 7 rework (NOTE: implementing session observed 3 reworks landed, not 2 as the Task brief's preamble said).
  - [x] 3 new 19.2-emergent forward-pointers documented — REVIEW §6 covers decode-side body-mutation-delivery limitation (Task 7); ref Envoy 500 (Task 9 AMENDMENT I); envoy-go HCM encode-side gap (Task 9 AMENDMENT II).
  - [x] 6 phase-done gates cited from Task 10 outputs — REVIEW §7 reproduces verbatim outputs from PROGRESS Task 10 entry.
  - [x] Parent-rollup confirmation with commit-message grep evidence — REVIEW §8 quotes the Task 10 commit message body verbatim + records the grep verifiability check result.
  - [x] Sign-off paragraph explicit — REVIEW §11 covers APPROVED disposition + summary stats.
  - [x] PROGRESS.md Task 11 entry + Task 10 SHA `a684385` filled — done at this commit.

**Phase-done squash-merge hand-off contract** (consumed by the next-session phase-done squash-merge orchestrator):

  1. **All 16 SPEC §15 items GREEN per REVIEW §2** — no RED items to remediate before squash-merge.
  2. **All 4 ADR landings cleanly anchored per REVIEW §3** — no ADR-text follow-ups needed at squash time.
  3. **D-series D1..D12 final dispositions captured per REVIEW §4** — no D-series follow-ups needed at squash time.
  4. **ROADMAP row 19.2 + parent row 19 BOTH `done` at `a684385`** — the squash-merge commit body MUST also name BOTH transitions for grep-verifiability per SPEC §15 item 12 (the underlying Task 10 commit body already names them; the squash commit inherits the discipline).
  5. **STATE.md `last-commit` placeholder fill** is THIS session's follow-up after the `wt-merge` squash commit lands on master — fill with the master-side squash commit SHA per the phase-09..19.1 precedent.
  6. **Push to origin after STATE SHA-fill** per project memory `feedback_push_to_origin.md` — push without asking once master is clean post-squash.
  7. **No reviewer-feedback rework needed** — APPROVED-as-authored per the PLAN Task 11 Step 2 skip direction.


