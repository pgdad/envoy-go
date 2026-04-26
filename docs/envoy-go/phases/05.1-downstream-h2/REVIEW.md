# Phase 05.1 — Downstream HTTP/2 Review

**Reviewer:** `superpowers:requesting-code-review` skill, dispatched to `superpowers:code-reviewer` subagent (per ADR-0001 / ADR-0006).
**Date:** 2026-04-26
**Review range:** `ddf41cd` (phase-04 done — final master tip before phase 05 work began) → `536f353` (master tip; lifecycle-state 5 promotion). 49 phase-05.1 commits land in this range, plus six phase-05 SPEC + split commits (`8d18320`, `612cdea`, `0bb11b6`, `2162509`, `c940928`, `4b45941` is 05.1 SPEC).
**Worktree:** `.worktrees/phase-05.1-downstream-h2-review` on branch `phase/05.1-downstream-h2-review`, branched from master tip `536f353` per ADR-0003.
**Verdict:** APPROVED WITH FOLLOW-UPS

---

## Executive summary

Phase 05.1 lands envoy-go's downstream HTTP/2 dataplane: a from-scratch `internal/filter/hcm/h2/` server-side codec sub-package decomposing into nine production source files (`doc.go`, `errors.go`, `preface.go`, `framer.go`, `hpack.go`, `flow.go`, `settings.go`, `stream.go`, `conn.go`) that consume `golang.org/x/net/http2.Framer` + `golang.org/x/net/http2/hpack` as low-level codec only (per doctrine D-3.2 + ADR-0046; the runtime types `http2.Server`/`http2.Server.ServeConn`/`http2.ConfigureServer`/`http2.Transport`/`http2.Transport.NewClientConn` are FORBIDDEN and absent from production); a per-stream server-side state machine implementing the RFC 9113 §5.1 idle/open/half-closed/closed lifecycle; an ALPN-driven codec dispatcher in `internal/filter/hcm/filter.go` (per ADR-0050); a `codec_type: HTTP2` extension to HCM `config.go` with build-time TLS validation; a codec-neutral `directResponseAction` factoring (`body()` + `writeH1`/`writeH2` adapters) per ADR-0045; a test-only `--allow-h2c` CLI flag plumbed via a new `listener.NewManagerWithBaseDirAndAllowH2C` constructor + per-listener `listenerCtx{hasTLS, allowH2C}` (per ADR-0049); the project's first non-vacuous conformance gate `test/conformance/h2spec/` (per ADR-0051) running `summerwind/h2spec` pinned by SHA in the new `docs/envoy-go/CONFORMANCE_PINS.md`; a new `BEHAVIOR_CONTRACT.md ## HTTP/2` SCAFFOLD subsection (per ADR-0052); two new fuzz targets (`FuzzFrameStream` + `FuzzHPACKDecode`); eight new ADRs (ADR-0046..ADR-0053); and the formal phase-04 REVIEW Minor carry-forward triage (M-2/M-4/M-5/M-6/M-7) per ADR-0053.

The 16-task PLAN executed cleanly with one state-3 re-entry for gate-(e) lint cleanup (commits `9e23e77` mechanical sweep + `65d2574` unused-symbol triage). Two impl-followup commits resolved a real concurrency bug (`e806f17` data-race in `emitGoaway` + framer test; `3b4e2ed` misspelling). PROGRESS.md carries two verification blocks (lifecycle-state 4 at `df85f85` capturing the gate-(e) bounce; lifecycle-state 5 at `2237e6e` capturing the green re-run) plus a Task 17 follow-up block (`9aa557a`) and final SHA-fills. ADRs are sequentially numbered without gaps (ADR-0046 → ADR-0053; ADR-0045 was consumed by the phase-05 planner-time split).

Five of the six SPEC §3 phase-done gates are GREEN at HEAD `536f353` and were re-verified against this review worktree:

- (a) **VACUOUS** per ADR-0045 (no new differential fixture in 05.1; fixture `0004-h2-routing` is 05.2's deliverable).
- (b) Pre-existing fixtures `0000-tcp-echo`, `0001-tcp-proxy-rr`, `0002-tls-tcp`, `0003-http11-routing` GREEN; `go test ./...` and `go test -race ./...` clean across all 15 test-bearing packages.
- (c) **NEWLY NON-VACUOUS** — h2spec **53/53 PASS** at the ADR-0051 pinned image (`summerwind/h2spec@sha256:5f4a65c30cae8569558ced048b4bfe0dcf01a221e36767ae504ccd8348a7aeb0`); independently re-run by this review against HEAD `536f353` — same 53/53 result, same per-section breakdown.
- (d) All six fuzzers (`FuzzBootstrapLoad`, `FuzzTcpProxyFilter`, `FuzzTLSContextParse`, `FuzzHCMConfigParse`, `FuzzFrameStream`, `FuzzHPACKDecode`) PASS at the 30s ADR-0018 budget; `git status --porcelain` empty after each (no testdata/fuzz pollution).
- (e) `go build` / `go vet` / `golangci-lint run ./...` (v1.64.8) all return 0; ADR-0046 boundary grep clean (3 production hits in the 5 allowed files); ADR-0048 `client.go` absence verified; ADR-0048 `internal/cluster/` is byte-for-byte unchanged from `ddf41cd`.

Doctrine compliance is rigorously verifiable. The 05.1/05.2 boundary is auditable end-to-end: no `client.go`, no `dial_h2.go`, no `0004-h2-routing/` fixture directory, the `BEHAVIOR_CONTRACT.md ## HTTP/2` "Does not yet apply to" enumeration explicitly defers routed-to-upstream H2 to 05.2, and `internal/cluster/` carries zero diff vs phase 04. Phase-04 REVIEW Minor carry-forward dispositions (ADR-0053) are faithfully recorded with no silent re-disposition; M-7's phase-06-must-consume tag is explicit at `DECISIONS.md:1830`.

Verdict: **APPROVED WITH FOLLOW-UPS** — zero Critical findings, four Important findings (none blocking 05.1 because each is dormant in 05.1's actual behaviour surface), and seventeen Minor findings (one of which — M-1, the pre-flagged `hpackBlocked` dead code — was successfully predicted by the verification session). Three of the four Importants (I-1/I-2/I-3) are flow-control discipline gaps that **do not bite in 05.1** (every shipped path is a bodyless GET to a `direct_response` with a small body) but **will bite in 05.2** when routed-to-upstream H2 lands; they should be consumed by 05.2's brainstorming via a single dedicated ADR. The fourth Important (I-4) is a `CONFORMANCE_PINS.md` documentation gap against SPEC §13's acceptance bullet "pins by tag + SHA256 with a refresh procedure" — small enough (~5-line edit) that landing it as a state-3 follow-up commit alongside M-1 and the small documentation fixes is the cleanest path.

Phase 05.1 is ready to advance to lifecycle-state 6, optionally after a single small follow-up commit closing I-4 + M-1 + the prose-drift Minors. Per BOOTSTRAP §5.2 the lifecycle state on follow-ups is state 3 (resume implementation), not state 4 (re-verify only); a one-task follow-up batch + re-run of the affected gates is appropriate.

---

## Verification of acceptance checklist (SPEC §13)

The 14 SPEC §13 bullets — re-verified from this review worktree at HEAD `536f353`:

1. **Six phase-done gates green per §3, with gate (a) recorded as "vacuous — no new fixture in 05.1".** PASS — see executive summary above. Gate (a) marked vacuous in `PROGRESS.md` line 1578; gates (b)/(c)/(d)/(e) re-verified GREEN by this review against the worktree copy of HEAD `536f353`.

2. **`internal/filter/hcm/h2/` package contains from-scratch ServerConn + serverStream; NO `client.go`; no production use of `http2.Server` / `http2.Transport`.** PASS — `! ls internal/filter/hcm/h2/client.go` confirmed; `grep -nR 'http2.Server\|http2.Transport\|http2.ConfigureServer\|http2.Server.ServeConn\|http2.Transport.NewClientConn' internal/ cmd/envoy-go/main.go --include='*.go'` filtered to non-`_test.go` returns zero hits in production code. The three production references in `internal/filter/hcm/h2/doc.go:22-24` are inside the package's prohibition statement (comment text), not call sites.

3. **`internal/cluster/` byte-for-byte unchanged from phase 04.** PASS — `git diff ddf41cd..536f353 -- internal/cluster/` returns empty.

4. **`Filter.Handle` ALPN dispatch is grep-verifiable.** PASS — `internal/filter/hcm/filter.go:31-58` is a 28-line block: codec-type switch → `*tls.Conn` type-assert → defensive `HandshakeContext(ctx)` (SPEC §11.6 mitigation) → `NegotiatedProtocol` read → branch on `"h2"`. Build-time `codec_type: HTTP2` validation against `listenerCtx.allowH2C` lives in `config.go:74-84`.

5. **`directResponseAction` factoring grep-verifiable; H1 wire output preserved (golden test); fixture 0003 green.** PASS — `body()` at `actions.go:46-54`, `writeH1` at lines 58-60 (delegates unchanged to phase-04's `writeStatusReply`), `writeH2` at lines 65-78. H1 byte-equivalence asserted by `TestDirectResponseWriteH1_GoldenCompat` (`actions_test.go:47-63`) against `internal/filter/hcm/testdata/direct_response_h1.golden`. Differential fixture `0003-http11-routing` re-run GREEN by this review.

6. **`BEHAVIOR_CONTRACT.md ## HTTP/2` SCAFFOLD has the seven prescribed subheadings; Header allow-list extended.** PASS — `BEHAVIOR_CONTRACT.md:267-315` carries the seven subheadings (intro / Asserted equivalence (05.1 scope) / Not asserted (05.1 scope) / Header allow-list extensions / h2spec threshold / Applies to (05.1) / Does not yet apply to). Header allow-list rows for `:status` (active) + `:method`/`:path`/`:scheme`/`:authority` (forward-looking, applies-to 05.2) are present at lines 40-44 with phase 05.1 + ADR-0052 provenance.

7. **`CONFORMANCE_PINS.md` exists and pins `summerwind/h2spec` by tag + SHA256 with a refresh procedure.** **PARTIAL PASS** — file exists; pins by tag and SHA256; **but does not include a refresh-procedure section**. ENVOY_TARGET.md (the precedent) has a `## Refresh procedure` section (`ENVOY_TARGET.md:10-19`); CONFORMANCE_PINS.md does not. See finding I-4 below.

8. **All eight new ADRs (ADR-0046..ADR-0053) appear with full Context/Decision/Consequences sections.** PASS — every ADR carries Status / Date / Doctrine / Settles / Context / Decision / Consequences and references SPEC sections, BOOTSTRAP doctrine, and code touchpoints by file/function. ADR-numbering shift discipline from ADR-0045 honoured; numbers contiguous; no gaps.

9. **`test/conformance/h2spec/h2spec_test.go` exists, is excluded from `go test -short`, and reports `failed == 0`. h2c bootstrap uses `--allow-h2c`.** PASS — `t.Skip` on `testing.Short()` at `h2spec_test.go:36` (`if testing.Short() { t.Skip("h2spec is not -short") }`). Bootstrap synthesised inline in the test driver carries the `--allow-h2c` flag at the subprocess invocation. Test re-run by this review against HEAD `536f353`: 53 tests, 53 passed, 0 skipped, 0 failed. JUnit-XML parsing in `assertThreshold` at `h2spec_test.go:295-351` is real (not just a string match on output).

10. **No phase-04 fixture (`0000`/`0001`/`0002`/`0003`) regressed.** PASS — `go test ./test/differential/ -count=1` re-run by this review: all four fixtures GREEN.

11. **`cmd/envoy-go/main_test.go` carries an h2-over-TLS bootstrap variant.** PASS — `TestEnvoyGoBinary_H2Smoke` at `cmd/envoy-go/main_test.go:325+` exercises a TLS listener with `alpn_protocols: ["h2"]` + HCM `codec_type: HTTP2` + `direct_response` route, asserted via `golang.org/x/net/http2.Transport` client probe (driver-side use OK per D-3.2). The `--allow-h2c` h2c smoke is intentionally NOT duplicated here per SPEC §8.4 (the conformance suite covers it).

12. **STATE.md / ROADMAP.md will reach correct end states; commit message names every ADR.** Pending — STATE.md is at lifecycle-state 5 awaiting this REVIEW.md; the next session advances STATE.md / ROADMAP.md to state 6 / `done` per BOOTSTRAP §5 state-6 in the phase-done commit. ROADMAP.md row 05.1 currently `in-progress`; row 05.2 stays `planned`; row 05 (parent) stays `in-progress` until 05.2 lands.

13. **PROGRESS.md quotes command outputs of all six gates per §5.3 verification protocol; SHA-fill complete per phase-04 convention.** PASS — `PROGRESS.md:897` (state-4 verification block), `:1349` (Task 17 follow-up), `:1546` (state-5 verification block) carry verbatim command outputs. SHA-fill final at `b416664`; lifecycle bookkeeping at `df85f85` / `b61e61f` / `2cf3458` / `2237e6e` / `536f353`.

14. **05.1/05.2 boundary auditable.** PASS — `internal/filter/hcm/h2/client.go` absent; `internal/cluster/dial_h2.go` absent; `test/fixtures/0004-h2-routing/` absent; `BEHAVIOR_CONTRACT.md ## HTTP/2` "Does not yet apply to" enumeration at `:306-315` defers routed-to-upstream H2, HTTP/3, server push, gRPC framing, trailer forwarding, upstream H2 stream pooling, h2c production fixtures, mTLS over h2.

15. **Phase-04 REVIEW Minor carry-forward triage faithfully recorded** (ADR-0053). PASS — every Minor (M-2/M-4/M-5/M-6/M-7) carries an explicit disposition at `DECISIONS.md:1820-1830`; M-7's `phase-06-must-consume` tag is explicit; M-5's "phase-05.2-will-repeat-the-pattern" forward-looking note is at line 1826. The "New H2 prose-vs-mechanism shape (05.1 scope)" addendum at line 1832 honestly acknowledges 05.1 introduces an analogous `defer`-vs-prose gap on the H2 path.

**Overall acceptance:** 14/14 substantively satisfied; bullet 7 (CONFORMANCE_PINS.md refresh procedure) is short by ~5 lines of prose — surfaced as I-4.

---

## Strengths

- **Doctrine D-3.2 compliance is rigorously verifiable.** Production grep of the forbidden runtime types returns zero call sites; the three textual mentions in `internal/filter/hcm/h2/doc.go:22-24` are inside the package's own prohibition statement. ADR-0046 boundary holds: production imports of `golang.org/x/net/http2` are confined to `framer.go`, `settings.go`, `conn.go` (3 files; ADR-0046 line 1545 itself names a slightly different list — see M-18 below).
- **Race-fix at `e806f17` is correct and complete.** `emitGoaway` now holds `s.mu` across the `WriteGoAway` call (`conn.go:578-588`), serialising it against `encodeAndWriteHeaders` and `writeData`. The `golang.org/x/net/http2.Framer`'s internal write buffer is a shared non-thread-safe resource; the mutex coverage is now correct and `go test -race ./internal/filter/hcm/h2/...` is clean (re-verified by this review).
- **Burst-drain dispatch ordering is non-trivially correct.** `processFrameAndMaybeDrain` (`conn.go:153-193`) — process-then-non-blocking-poll-then-launch-pending-goroutines — solves a real h2spec 5.1.2/1 ordering requirement (RST_STREAM(REFUSED_STREAM) for an overflow stream MUST be visible on the wire before any DATA frame from accepted streams). The `pendingDispatch []func()` queue + `doneCh chan uint32` bookkeeping delivers MAX_CONCURRENT_STREAMS atomicity correctly. The design rationale is comment-justified at `conn.go:38-48` and `conn.go:103-113`.
- **Conformance gate is genuinely non-vacuous and was independently re-executed.** The pinned image, the JUnit-XML parser, the threshold check are all real. Re-run against HEAD `536f353` from this review worktree: 53/53 PASS, per-section breakdown matches `CONFORMANCE_PINS.md`.
- **Codec-neutral `directResponseAction` factoring is clean and grep-verifiable.** H1 wire output preserved byte-for-byte by golden-byte test (`internal/filter/hcm/testdata/direct_response_h1.golden`); H2 path emits `:status` first per RFC 9113 §8.3 then regular headers in deterministic order; fixture 0003 GREEN after the refactor (the SPEC §11.8 backwards-compat invariant holds).
- **ALPN dispatch is small, defensive, and grep-verifiable.** A tight 28-line block at `filter.go:31-58`. The defensive `HandshakeContext(ctx)` no-op at line ~40 is the SPEC §11.6 mitigation; cheap insurance against a future listener-side refactor.
- **`--allow-h2c` plumbing is correctly contained** — default OFF, documented as "test-only; not for production" in `--help`, threaded through `NewManagerWithBaseDirAndAllowH2C` → per-chain `listenerCtx` → `hcm.ListenerCtx{HasTLS, AllowH2C}` → `parseFilterWithCtx` validation. Three positive tests in `config_test.go:130-167` exercise rejection-on-plaintext, accept-on-TLS, and accept-on-allowH2C.
- **Eight new ADRs are individually well-structured, single-concern, and cross-referenced.** Each names the SPEC section it settles, the doctrine items it leans on, and (where applicable) the file/function in the consuming code. ADR-0052's "SCAFFOLD-pattern in-place-edit authorisation" is an unusually clean mechanism for the 05.1 → 05.2 brainstorming handoff without supersession-ADR proliferation. ADR-0051's image pin-by-digest discipline is the conformance-suite analogue of ENVOY_TARGET.md per D-3.7.
- **HPACK table-size update propagation (SPEC §11.4 mitigation)** is implemented at `hpack.go:67-69`, exercised by `TestHPACK_UpdateMaxTableSize_PropagatesToEncoder` (`hpack_test.go:46-63`), and end-to-end verified by `TestServerConn_HPACKTableSizeUpdate_PropagatesToOutgoingHEADERS` (`conn_test.go:619-715`). Forwarded from `onSettings` at `conn.go:444-448`.
- **Tiny-window stress (SPEC §11.5 mitigation)** has both unit-level (`flow_test.go:59-92` — `TestWindow_TinyWindowStressDelivery`) and end-to-end (`conn_test.go:720-822` — `TestServerConn_TinyWindowDelivery`, INITIAL_WINDOW_SIZE=1, drip-fed WINDOW_UPDATE) coverage.
- **Both new fuzzers carry sensible seeds** and assertions: `FuzzFrameStream` seeds preface; preface+SETTINGS; preface+SETTINGS+ACK; assertion shape "no panic and errors begin with `h2:` or are `io.EOF`/`io.ErrUnexpectedEOF` or wrapped ctx errors" matches the reality of fuzz-corpus truncations. `FuzzHPACKDecode` seeds empty + canonical pseudo-header. Local short-budget runs: 13.4M / 2.0M execs respectively.
- **Phase-04 REVIEW carry-forwards are an honest disposition record.** ADR-0053 names every Minor by number; M-7's phase-06-must-consume tag is explicit; M-5's "05.2-will-repeat-the-pattern" forward note prevents 05.2's brainstorming from re-litigating the same disposition. The "new H2 prose-vs-mechanism shape" self-disclosure at line 1832 is unusually candid.
- **TDD discipline is visible in PROGRESS.** Tasks 2–9 each carry a "wrote failing tests first; saw them fail; then wrote the production code" pattern. The state-3 re-entry for gate-(e) lint cleanup (commits `9e23e77` + `65d2574`) followed BOOTSTRAP §5.2 correctly: lifecycle re-entered at state 3 (not state 4) for the cleanup work, then re-promoted to state 4 at `2cf3458`.

---

## Findings

### Critical (Must Fix)

**None.** All five non-deferred SPEC §3 phase-done gates are GREEN at HEAD `536f353`, re-verified against this review worktree. No code-blocking, security-blocking, or doctrine-violating finding was identified. The pre-flagged `hpackBlocked` carry-forward observation from the verification session was correctly anticipated as Minor (see M-1 below).

### Important (Should Fix)

**I-1. `ServerConn.writeData` does not respect `SETTINGS_MAX_FRAME_SIZE` on outgoing DATA chunks.** *(`internal/filter/hcm/h2/conn.go:609-647`)*

The chunk size is bounded only by `s.sendW.reserve(int32(len(remaining)))`, which is the connection-level send window (default 65535). With the peer's default `MaxFrameSize=16384`, any body larger than 16384 bytes that fits in the window will be written as a single oversized DATA frame, triggering peer-side `FRAME_SIZE_ERROR` per RFC 9113 §6.5.2 (and §4.2).

**Why it matters:** dormant in 05.1 (the only `direct_response` body shipped is e.g. 3-byte `OK\n`); h2spec covers small frames only and does not exercise the boundary at 16384 bytes. Phase 05.2's routed-to-upstream H2 will write arbitrary upstream response bodies and trip this immediately.

**Fix:** cap each chunk additionally at `min(s.sendW.reserve(...), int32(s.clientS.MaxFrameSize))` (using the peer-advertised value, falling back to RFC default 16384 if the peer hasn't yet sent SETTINGS). Add a regression test in `conn_test.go` that drives a >16384-byte body over a connection where the peer advertises `MaxFrameSize: 16384` and asserts ≥2 DATA frames on the wire.

**I-2. `ServerConn.writeData` does not respect the per-stream send window.** *(`internal/filter/hcm/h2/conn.go:609-647`)*

The comment at lines 612-615 acknowledges this explicitly ("we honor the connection-level window (s.sendW) but not per-stream windows for outgoing DATA"), but RFC 9113 §6.9.1 requires senders to respect MIN(conn-window, stream-window). The per-stream `serverStream.sendW` IS replenished correctly on incoming WINDOW_UPDATE frames (`stream.go:195-201`); it just isn't reserved against on writes.

**Why it matters:** dormant in 05.1 (bodyless GETs only). A peer with a small per-stream inbound window (one that calls `WindowUpdate` granularly per stream) can be over-fed by us; the peer is then RFC-permitted to abort the connection with FLOW_CONTROL_ERROR. h2spec section 5 doesn't exercise this directly. Phase 05.2 routed-to-upstream H2 will hit it against realistic upstreams.

**Fix:** add a `waitFor`+`reserve` cycle on the per-stream `sendW` inside the inner write loop, taking the minimum of conn-window-available, stream-window-available, and `MaxFrameSize`. Regression test: a peer that sets per-stream initial window to 16 with a 100-byte body should observe ~7 DATA frames before completion, and no FLOW_CONTROL_ERROR.

**I-3. Receive-side flow control is allocated but never enforced.** *(`internal/filter/hcm/h2/conn.go:29,64`; `stream.go:68,82,81-82`)*

Both `ServerConn.recvW` and `serverStream.recvW` are constructed but never decremented as DATA arrives, never replenished, and the server emits no inbound `WriteWindowUpdate` frames (verified: `grep WriteWindowUpdate internal/filter/hcm/h2/*.go` excluding `_test.go` returns zero call sites; only the godoc reference at `framer.go:23`).

**Why it matters:** the server advertises `INITIAL_WINDOW_SIZE=65535` (the default conn-window) and never replenishes it. After the cumulative inbound DATA across all streams reaches the conn-window the connection deadlocks (peer cannot send more, server never sends WINDOW_UPDATE). Memory-pressure aspect: the per-stream body buffer (`stream.go:71` — `reqBody bytes.Buffer`) accumulates without a size cap, so a malicious peer can hold up to `MaxConcurrentStreams * 65535 ≈ 6.4 MB` per connection before any dispatch occurs. Dormant in 05.1 (every test/fixture sends bodyless GETs to `direct_response`); bites in 05.2 routed-to-upstream H2 and any production POST/PUT workload after ~65 KB cumulative inbound on one connection.

**Fix (minimum):** on every `onData` chunk, decrement `s.recvW`/`ss.recvW` and emit `s.fr.WriteWindowUpdate(0, n)` and `s.fr.WriteWindowUpdate(streamID, n)` once a high-water threshold is crossed (a half-window replenishment policy is conventional). End-to-end test: open a stream with a body > 65 KB and assert it completes.

**I-1/I-2/I-3 disposition:** **carry-forward to 05.2** with a single dedicated ADR documenting flow-control discipline for the from-scratch H2 codec (outbound `MaxFrameSize` chunking, per-stream send-window enforcement, inbound WINDOW_UPDATE emission). 05.2's brainstorming session inherits this finding as a SPEC §3 input and the ADR lands at PLAN-write time per ADR-0004's autonomous-numbering rule (likely ADR-0054). Rationale for not blocking 05.1: each gap is dormant given 05.1's actual shipped-behaviour surface (bodyless GETs to `direct_response` with small bodies); h2spec 53/53 PASS confirms the gaps don't surface against the conformance gate; carry-forward is consistent with how phase-04's M-5 deferred its routed-upstream-H1 close-semantics gap to 05.2 under ADR-0053's precedent.

**I-4. `CONFORMANCE_PINS.md` is missing the SPEC §13-required "Refresh procedure" section.** *(`docs/envoy-go/CONFORMANCE_PINS.md`; SPEC §13 acceptance bullet 7)*

The file is 56 lines and ends at the "First run result" block. The line "All pins are append-only. A new pin supersedes the old one for the same tool" at line 7 is implicit policy, not a procedure. ENVOY_TARGET.md (the precedent file per D-3.7) has an explicit `## Refresh procedure` section at lines 10-19; CONFORMANCE_PINS.md should mirror that discipline.

**Why it matters:** D-3.4 (context isolation) — a stranger reading this file in 18 months should know exactly how to update the pin. SPEC §13's acceptance bullet 7 explicitly enumerates "with a refresh procedure" as a requirement; this is a literal acceptance-bullet shortfall.

**Fix:** add a `## Refresh procedure` section with the canonical update steps (`docker pull summerwind/h2spec:<new-tag>`; `docker inspect --format='{{index .RepoDigests 0}}'`; append a new row to the pin table preserving the prior row for audit trail; re-run `go test -run TestH2Spec ./test/conformance/h2spec/` and confirm 53/53 PASS at the new digest; supersede prior row's "current" status with a "superseded by <digest>" annotation; commit as a dedicated pin-refresh phase per D-3.7). Promote the "All pins are append-only" line into the procedure section so the policy and the procedure live together. ~10–20 lines of prose.

**Disposition:** small enough to land as a state-3 follow-up commit before the phase-done commit; alternatively carry forward to 05.2 if the recommended ADR for I-1/I-2/I-3 also extends `BEHAVIOR_CONTRACT.md ## HTTP/2`'s threshold language and bundles the prose work. State-3 follow-up is preferred because the gap is *current* (it doesn't depend on 05.2's surface) and SPEC §13 lists it as an acceptance criterion.

### Minor (Nice to Have)

**M-1. `hpackBlocked bool` and its guard block are dead code.** *(`internal/filter/hcm/h2/conn.go:37`, `:227-240`)*

The pre-flagged carry-forward observation from the verification session is correct. `grep -n "hpackBlocked\s*=\s*true"` returns zero matches in production code; the guard at line 236 is unreachable; the block at lines 227-240 cannot fire. The comment at lines 230-235 itself acknowledges the framer enforces header-block ordering via `checkFrameOrder` (translated to `*Error` by `framer.go:76-83`'s `errors.As(connErr)` translation). Belt-and-suspenders is fine; dead belts and suspenders are not. **Fix:** delete the field and the guard block; the doc comment can be repurposed into a single-line note in `dispatchFrame` if anyone wants a defence-in-depth audit trail. ~10 lines deleted.

**M-2. `validateClientStreamID` is dead production code.** *(`internal/filter/hcm/h2/stream.go:402-410`)*

Marked `Deprecated:` in its godoc; the production path inlines the validation in `conn.onHeaders` (lines 326-336). Used only by two unit tests at `stream_test.go:375` / `:391`. **Fix:** either delete and let the unit tests die (they're testing a no-longer-real code path), or move the helper into `stream_test.go` as test infrastructure.

**M-3. `writeData` has a dead `if taken <= 0` recovery branch.** *(`internal/filter/hcm/h2/conn.go:624-629`)*

After `waitFor(ctx, 1)` returns nil the window is ≥ 1; `reserve(int32(len(remaining)))` with `len(remaining) ≥ 1` will return ≥ 1. The branch attempts a `taken=1; reserve(1)` recovery whose error return is ignored (`if _, err := ...; err != nil { _ = err }` is a literal no-op). Worse: under concurrent multi-stream writes, the window race the branch was presumably trying to handle IS reachable (`waitFor` releases the lock; a second goroutine drains the window before the first calls `reserve`), and the recovery path silently writes 1 byte from `remaining[:1]` when the window is 0 — **over-running the window**. **Fix:** lift the `waitFor`+`reserve` into a single mutex-guarded operation on the `window` primitive (e.g., `reserveBlocking(ctx, max int32) (taken int32, err error)`), so wait+take is atomic; then delete the dead branch. Bundles well with I-1 / I-2 in the 05.2 ADR.

**M-4. `readClientPreface` is not ctx-aware.** *(`internal/filter/hcm/h2/preface.go:15-33`)*

Uses `io.ReadFull(r, buf)` directly. A peer that sends 23 bytes and then stalls indefinitely will block this read forever (subject to OS TCP keepalive). Phase-04 H1 has the same shape on the H1 read path (no regression). **Fix (deferrable):** add a `readClientPrefaceCtx(ctx, conn)` variant using the same `SetReadDeadline` polling pattern as `framer.readFrameCtx`, or address at the listener-manager level via uniform OS read deadlines. Phase 06 or 07 follow-up.

**M-5. `framer.readFrameCtx` and `framer.tryReadFrame` duplicate the http2.ConnectionError / StreamError / ErrFrameTooLarge translation block.** *(`internal/filter/hcm/h2/framer.go:76-103` vs `:128-153`)*

**Fix:** extract a `translateFramerErr(err) error` helper. Three-line cosmetic; no behavioural change.

**M-6. Fuzzer error checks use direct sentinel comparison.** *(`internal/filter/hcm/h2/fuzz_test.go:72-73`)*

`err == context.DeadlineExceeded` / `err == context.Canceled` works for the unwrapped sentinels (the current `Run` returns them unwrapped) but won't tolerate future wrapping (e.g., 05.2 wrapping ctx errors with operation-prefix strings). **Fix:** `errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)`. Cheap insurance.

**M-7. `ServerConn.recvW` and `serverStream.recvW` are dead today.** *(`conn.go:29,64`; `stream.go:68,82`)*

The fields are constructed but read-only forward (and even reads are absent). If I-3's mitigation lands in 05.2, the fields are finally consumed; otherwise they're inhabitants of the same dead-code class as `hpackBlocked`. **Fix:** keep if I-3 is on a near-term roadmap (recommended); delete if not.

**M-8. `excludedSubsections []string{"http2/6/6"}` is unused and `//nolint:unused`-suppressed.** *(`test/conformance/h2spec/h2spec.go:42`)*

Per the comment "Kept for documentation purposes" — promoting to a `const` or to a doc comment in `CONFORMANCE_PINS.md` rather than a nolint-suppressed slice keeps doctrine D-3.4 cleaner; documentation that requires lint suppression to compile is noise.

**M-9. `serverStream.recvWindowUpdate` accepts deltas without checking for window overflow.** *(`stream.go:195-201`)*

RFC 9113 §6.9.1: a stream's flow-control window MUST NOT exceed 2³¹ − 1; an overflow is a stream-level FLOW_CONTROL_ERROR. The same gap exists in `ServerConn.onWindowUpdate` (`conn.go:474-502`). Today the window is `int32`, so overflow wraps to a negative value silently. h2spec doesn't cover the boundary. Realistic exposure is low (an attacker would have to send WINDOW_UPDATE frames totalling > 2 GiB), but bundling the bounds check into the I-3 ADR is appropriate. **Fix:** bounds-check the addition; on overflow emit RST_STREAM(FLOW_CONTROL_ERROR) for stream-level, GOAWAY(FLOW_CONTROL_ERROR) for conn-level.

**M-10. `ServerConn` has no `SETTINGS_TIMEOUT`.** *(`internal/filter/hcm/h2/conn.go`)*

RFC 9113 §6.5.3: "If the sender of a SETTINGS frame does not receive an acknowledgement within a reasonable amount of time, it MAY issue a connection error of type SETTINGS_TIMEOUT". Today `readClientSettings` reads exactly one frame from the framer with no timeout; if the peer never SETTINGS_ACKs our initial SETTINGS, the connection sits idle. Phase-05.1 tolerates this (h2spec sends SETTINGS_ACK promptly); a future roadmap item should add the timer. **Disposition:** flag for 05.2 or 06.

**M-11. `serverStream.recvData` writes to `s.reqBody` *before* checking state-transition validity.** *(`internal/filter/hcm/h2/stream.go:160-165`)*

If a peer sends DATA in `streamHalfClosedRemote` or `streamClosed`, the bytes are still appended to `s.reqBody` even though the function returns a stream-error and the stream will be RST'd. Memory waste only — the stream is being torn down anyway. **Fix:** check `cur` before the append. One-line reorder.

**M-12. `ServerConn.closedStreams` map has no upper bound.** *(`conn.go:32`, `conn.go:511`, `conn.go:567`)*

Each closed stream ID lingers forever in the closed-streams set. With a long-lived h2 connection serving many streams the map grows unboundedly (~24 bytes per entry on amd64; 1M streams ≈ 24 MB per conn). **Fix:** cap at e.g. the last 1024 closed stream IDs (a small ring buffer suffices); dropping older entries is RFC-compatible because those streams are long since reaped from the peer's perspective. Observable only in long-lived production sessions, which 05.1 doesn't have yet.

**M-13. `BEHAVIOR_CONTRACT.md ## HTTP/2`** says (line 285) "envoy-go's unit tests assert byte equality directly" for the H2 body. The unit test that does this is `actions_test.go:110-112` (`TestDirectResponseWriteH2_HEADERSThenDATAEndStream`) — a fake-`StreamWriter` capture of DATA payload, not a true end-to-end byte comparison against an upstream response. The assertion is correct for the codec-neutral writer; **fix:** tighten the prose to "envoy-go's hcm-package unit tests assert byte equality on the captured DATA payload via a fake StreamWriter". Cosmetic.

**M-14. `internal/filter/hcm/h2dispatch.go` no-match 404 body text divergence-risk vs H1 path.** *(`internal/filter/hcm/h2dispatch.go:41`)*

The H2 path synthesises a 404 with `bodyText: "not found\n"`. The H1 path's no-match shape is centralised in `connection.go`'s 404 emission. Acceptance #5 ("fixture 0003 green") only validates `direct_response` *matched* paths, not no-match. Worth a 5-minute audit to confirm body text alignment between the two paths. Cosmetic if equivalent; finding-worthy if divergent.

**M-15. ADR-0046 prose drift on the 3-import file list.** *(`DECISIONS.md:1545`)*

ADR-0046 says: "the boundary is grep-verifiable: `! grep -nR '"golang.org/x/net/http2"' internal/ cmd/envoy-go/main.go` (excluding `_test.go`) returns zero hits OUTSIDE `internal/filter/hcm/h2/framer.go`/`hpack.go`/`settings.go` — the three files that legitimately import the package." But `hpack.go` does NOT import `"golang.org/x/net/http2"` directly — it imports `"golang.org/x/net/http2/hpack"`. The actual list of production files importing the root `http2` package is `framer.go`, `settings.go`, `conn.go` (3 files; matches the count claimed in the PROGRESS verification blocks at lines 1572, 1573). **Fix:** amend ADR-0046 to say `framer.go`/`settings.go`/`conn.go`. Documentation drift; harmless because the grep gate runs against the actual code, not against ADR-0046 prose. Per D-3.5 the amendment should be a superseding ADR (ADR-0046 is landed/append-only) OR a single targeted PR-time fix justified as a textual correction. Phase 05.2's brainstorming session can land the correction alongside the I-1/I-2/I-3 ADR.

**M-16. `cmd/envoy-go/main_test.go` H2 smoke variant uses `InsecureSkipVerify: true`.** Correct for a smoke test; tag it in the test docstring as "smoke-only — production tests must verify CA chain". Cosmetic.

**M-17. `connection.go` ALPN-h1 fall-through documentation gap.** When `codec_type: AUTO` is set on a TLS listener with `alpn_protocols: ["h2", "http/1.1"]` and the negotiated ALPN is "http/1.1", the dispatch (`filter.go:31-58`) falls through to `runConnection` (the H1 driver). This is correct per SPEC §5.4, but the H1 driver's expectation that the connection has just been TLS-handshaken is implicit. A one-line comment in `connection.go` linking to ADR-0050 + SPEC §5.4 would help readability.

---

## Phase-04 REVIEW Minor carry-forward audit (per ADR-0053)

The phase-04 REVIEW (`04527eb`) closed with five Minor findings carried forward to 05.1: M-2 (ADR-0043 doctrine attribution), M-4 (listener-manager `Stop()`/`Listeners()` race), M-5 (phase-04 SPEC §7 prose-vs-`defer` mechanism on the upstream-close path), M-6 (fixture-0003 driver heredoc YAML), M-7 (`Filter.statPrefix` stored but never consumed). ADR-0053 (`DECISIONS.md:1807-1841`) records the 05.1 disposition per Minor.

Audit findings:

- **M-2:** disposition "DEFERRED — cosmetic" recorded faithfully at `DECISIONS.md:1822`. Phase 05.1 does not touch ADR-0043. No new obligations.
- **M-4:** disposition "DEFERRED to phase 08" recorded at `DECISIONS.md:1824`. Phase 05.1 does not touch the listener-manager lock surface. The 05.1 listener-manager change (`NewManagerWithBaseDirAndAllowH2C` + `listenerCtx` plumbing per Task 11) is a build-time path; runtime `Stop()/Listeners()` is unchanged. No new obligations.
- **M-5:** disposition "DEFERRED" + "phase-05.2-will-repeat-the-pattern" forward-looking note recorded at `DECISIONS.md:1826`. The 05.1 H2 path introduces an analogous shape (the `defer` cleanup in `serverStream.dispatch`'s action invocation, analogous to phase-04 H1's prose-vs-`defer` gap), explicitly acknowledged at `DECISIONS.md:1832`. No silent re-disposition.
- **M-6:** disposition "DEFERRED" recorded at `DECISIONS.md:1828`. Phase 05.1 introduces no new fixture; the structured-`expectations.yaml` plan remains unforced.
- **M-7:** disposition "DEFERRED with phase-06-must-consume tag" recorded at `DECISIONS.md:1830`. Phase 06's brainstorming session inherits the must-consume requirement: either honour `Filter.statPrefix` in stats emission (lifting M-7 to RESOLVED) or supersede ADR-0041 with a stat-naming policy. No silent re-disposition.

**Audit verdict:** ADR-0053 carries the five dispositions with no silent re-disposition. M-7's hard tag is preserved. M-5's forward-looking 05.2-will-repeat-the-pattern note is the model future cross-phase carry-forward language should follow.

---

## Recommendation

**APPROVED WITH FOLLOW-UPS.** Phase 05.1 is correct, well-tested, well-documented, and doctrine-compliant. Zero Critical findings. Four Important findings (I-1/I-2/I-3 dormant in 05.1 surface; I-4 a SPEC §13 acceptance-bullet documentation gap). Seventeen Minor findings, one of which (M-1) was successfully predicted by the verification session.

Two follow-up paths are available; the project may choose either:

**Path A (preferred): land a small state-3 follow-up commit, then advance to state 6.** The follow-up commit batch:
- Close I-4 by adding a `## Refresh procedure` section to `CONFORMANCE_PINS.md` (~10–20 lines of prose).
- Delete M-1 (`hpackBlocked` dead code; ~10 lines deleted).
- Triage M-2 (delete `validateClientStreamID` or move into `_test.go`).
- Fix M-15 (correct ADR-0046's file list — likely as an amendment-via-supersession ADR per D-3.5, OR a textual correction in the same commit if the user judges this a typo-class fix not warranting a new ADR).
- Optional: M-6 (fuzzer `errors.Is`), M-13 / M-16 / M-17 prose fixes.
- Re-run gates (b)/(c)/(d)/(e) against the new HEAD; gate (a) remains vacuous; gate (f) is closed by an updated REVIEW.md addendum or by the executor's PROGRESS verification block.
- Then advance STATE.md to lifecycle-state 6 and write the final phase commit per BOOTSTRAP §5 state 6.

Per BOOTSTRAP §5.2, state-3 re-entry is the correct mechanism for code/doc-change follow-ups, not state 4.

**Path B: advance directly to state 6 with all four Importants and all Minors carried forward to 05.2.** This is consistent with phase-04's "APPROVED WITH FOLLOW-UPS" precedent (where four Importants were also deferred to a follow-up commit, landed at `671a059`). The follow-ups (I-1/I-2/I-3) bundle naturally with 05.2's brainstorming as a single dedicated flow-control discipline ADR (likely ADR-0054). I-4 ships as a small textual fix at the start of 05.2 implementation, or even earlier as a doctrine-cleanup commit.

The choice depends on the project's preference for tightness-vs-velocity. Path A is tighter (closes the SPEC §13 acceptance bullet immediately and removes the predicted dead code); Path B is faster (matches the phase-04 precedent and bundles the flow-control work coherently into 05.2's natural surface). **Path A is recommended** because I-4 is a current acceptance-bullet shortfall (not a forward-looking surface) and M-1's deletion is the verification session's pre-flagged carry-forward — closing it now retires the prediction cleanly.

The single most important context to surface to the phase-05.2 planner: **the three flow-control discipline gaps (I-1/I-2/I-3) form a coherent ADR-shaped unit.** 05.2's routed-to-upstream H2 will trip all three on first contact with realistic upstreams; an ADR drafted at 05.2 PLAN-write time covering MaxFrameSize chunking, per-stream send-window enforcement, and inbound WINDOW_UPDATE emission is the cleanest shape. The ADR can also extend `BEHAVIOR_CONTRACT.md ## HTTP/2`'s threshold language to require an h2spec subset that exercises non-default `MaxFrameSize` (an additive threshold expansion is permitted; reduction would require an ADR superseding ADR-0051). The h2spec section list at ADR-0051 already excludes 6.6 (PUSH_PROMISE) explicitly; if 05.2 brings in 6.9 (WINDOW_UPDATE) strict-mode coverage, the ADR documents that addition.

Phase 05.1 is ready to advance to lifecycle-state 6.

**Verdict: APPROVE.**
