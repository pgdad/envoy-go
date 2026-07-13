# Phase 61.2 PROGRESS — `http3-h3-codec-hcm` (IMPL)

> **Scaffold produced at the phase-61.2 PLAN stage** (docs-only). The IMPL executes `PLAN-61.2.md` task-by-task, subagent-driven (`feedback_execution_style`), in worktree `.worktrees/phase-61.2-impl`, branch `phase-61-http3-h3-codec-hcm-impl`, off master. This is the SECOND leg of the confirmed 61.1/61.2/61.3 split (SPEC-61 §3.0); ANCHORS **ADR-0281** (a NEW per-leg ADR for the codec-arm seam — §Context/§Decision/§Consequences all land at this IMPL per ADR-0044). **Row 61 STAYS `in-progress` at this six-gate** — it flips `done` only when ALL THREE legs land (ADR-0106 / `reference_roadmap_split_phase_row_done`). The HTTP/3 FAMILY STAYS OPEN.

## Baseline counts (verify at IMPL start against the master tip; `git fetch` first)

| metric | baseline | anticipated exit |
|---|---|---|
| stat surface | 1201 | **1201** (+0 RECOMMENDED — the H3 arm reuses the codec-agnostic `downstream_rq_<Nxx>`/`downstream_rq_completed` counters via `downstreamStatusClassCounter`; the reference's `downstream_{cx,rq}_http3_total`/`http3.*` DEFERRED to 61.3/robustness, asserted as a NAMED SUBSET; IMPL MAY pin +2 if trivial, `stats.IsValidName`-guarded) |
| fixtures | 105 (tail `0103-xds-sds-server-cert`) | **105** (+0 — the cross-side H3-GET `0102` is leg 61.3; 61.2 proves SUBJECT-side only) |
| fuzzers | 55 | **55** (+0 — quic-go owns H3 framing + QPACK; no new hand-rolled parse) |
| BackendKind tail | 38 (`H2GoawayResponder`) | **38** (+0 — the subject-side test reuses an existing responder / `direct_response`) |
| DECISIONS tail | ADR-0279 (61.1) | **ADR-0281** (a NEW per-leg codec-arm ADR, authored here; next-free **ADR-0282**) |
| new production Go packages | 0 | **+0** (the H3 arm folds into `hcm` as `h3dispatch.go` — quic-go IS the codec, so no `internal/filter/hcm/h3` package; refines SPEC §4/§12 as 61.1 folded `internal/listener/quic` into `quic.go`) |
| new go.mod modules | 0 | **+0** (quic-go v0.54.1 landed at 61.1) |
| ROADMAP row 61 | `in-progress` | **`in-progress`** (NO flip — 61.3 pending) |

## Import hygiene (LOAD-BEARING — re-check at Tasks 5/6/7/9)

quic-go (`github.com/quic-go/quic-go`, incl. the `http3` package) stays confined to `internal/listener/quic.go` — the ONLY production file importing it, UNCHANGED from 61.1 (61.2 adds the `http3` sub-package import THERE). The H3 dispatch arm (`internal/filter/hcm/h3dispatch.go`) speaks ONLY stdlib `net/http` — ZERO quic-go import. `internal/tls` stays quic-go-free (61.1 gate). Verify:
- `go list -deps ./internal/filter/hcm | grep -i quic-go || echo HCM-NO-QUICGO` → `HCM-NO-QUICGO`
- `go list -deps ./internal/tls | grep -i quic-go || echo TLS-NO-QUICGO` → `TLS-NO-QUICGO`
- `go list -deps ./internal/listener | grep -i quic-go` → prints the quic-go module (confined here)
- NO cycle: neither `internal/filter/hcm` nor `internal/filter/network` imports `internal/listener` (VERIFIED at PLAN time). The new `network.H3Terminal` interface (stdlib `net/http`-only) is asserted in `quic.go` (listener → network, an existing edge); the concrete `*hcm.Filter` is injected as a `network.NetworkFilterFactory` at boot (no listener → hcm edge).

## 61.2 design pins (settled in PLAN-61.2 §"Design pins settled here")

- **H3 SEAM = a stdlib-typed dispatch arm, NOT a codec package.** quic-go's `http3.Server` owns H3 framing + QPACK; the HCM arm is a per-request dispatch method (`ServeH3`→`runH3`) in `internal/filter/hcm/h3dispatch.go`, speaking only `net/http`. +0 packages (refines SPEC §4/§12).
- **BRIDGE = `network.H3Terminal` interface** (`TerminalFilter` + `ServeH3(w http.ResponseWriter, r *http.Request)`, stdlib-only). `*hcm.Filter` implements `ServeH3`; `internal/listener/quic.go` asserts the chain terminal to it and drives `http3.Server{Handler: http.HandlerFunc(h3t.ServeH3)}.ServeQUICConn(conn)`. Keeps quic-go confined; no listener→hcm edge; no cycle.
- **REQUEST SIDE reuses the H1 `router.Action` path** (`func(ctx, *http.Request) (ActionResponse, Endpoint, error)`, NOT `H2Action`) → ZERO `router.go` changes (target). `runH3` mirrors `dispatchRequest` (`connection.go:312`): `f.table.match(r)` → `entry.action.asRouterAction()` → build chain → seed (TLS from `r.TLS`) → `SetAction`/`SetRequest` → `RunDecodeHeaders(r.Header,…)` → `RunAction` → encode chain → `writeH3Reply`. `SetDownstreamProtocol("HTTP/3")`.
- **RESPONSE SIDE = `writeH3Reply` adapter** (`ActionResponse → w.Header()/WriteHeader/Write`) — NOT `writeH1Reply` (which emits the HTTP/1.1 wire format wrong for H3; quic-go owns H3 framing).
- **reject-B via `lc.IsQUIC`** — thread the listener `kind` `network.FactoryCtx.IsQUIC → hcm.ListenerCtx.IsQUIC`; `codec_type HTTP3` accepted iff QUIC, else config-parity reject `codec_type HTTP3 requires a QUIC (udp_listener_config) listener` (SPEC §11 arm reject-B — both sides reject). Lift the blanket `config.go:240` reject.
- **`emitAccessLogH3`** — thin third arm, `Protocol="HTTP/3"` (both access-log Record + span). The 61.3 differential VERIFIES the exact string cross-side.
- **STAT SURFACE +0 recommended** — reuse the codec-agnostic `downstream_rq_*` counters; DEFER `downstream_{cx,rq}_http3_total`.
- **serveQUICConnection ctx-honoring (M6-2 pickup)** — Task 7's serve path honors `ctx` (a canceled ctx closes the conn), addressing the 61.1 review's M6-2 (`serveQUICConnection` discarded ctx).
- **ADR-0281 = a NEW per-leg codec-arm ADR** (not folded into ADR-0279) — the codec arm is a distinct seam (the `H3Terminal` bridge + the quic-go-free HCM arm + the `http3.Server` wiring), per SPEC §3.0's PLAN/IMPL choice.

## SPEC correction recorded here (per `feedback_brief_citations_not_evidence`)

SPEC-61 §12 (edit-site roster) said: "`internal/filter/hcm/filter.go:112` (`Handle`) — ADD a `codecType == HTTP3` branch / `runH3`." **This is a re-derivation error.** `Handle(ctx, downstream net.Conn)` (`filter.go:112`) is the TCP-path entry — it is NEVER called on the QUIC path (QUIC has no `net.Conn`). The H3 entry is the NEW `ServeH3(w, r)` method invoked by quic-go's `http3.Server`, reached via the `network.H3Terminal` interface — NOT a `Handle` branch. The PLAN's Task 6 (interface) + Task 7 (wiring) implement the correct seam; `filter.go` gains only the `IsQUIC` bridge (Task 2), NOT a `Handle` H3 arm. The `runH3`/`ServeH3`/`writeH3Reply` functions live in the NEW `internal/filter/hcm/h3dispatch.go`, not in `filter.go`.

## 61.1 review-pickup disposition (from PLAN-61.1 / the 61.1 final review MINORS)

- **M6-2 (`serveQUICConnection` discards ctx)** — PICKED UP at Task 7: the `ServeQUICConn` serve path honors `ctx` (canceled ctx closes the conn to unblock the serve / Stop).
- **M6-1 (`quicAcceptLoop` no TCP-style backoff on Accept error)** — UNCHANGED by 61.2 (the accept loop is untouched; 61.2 replaces only the per-conn serve body). RE-DEFERRED to a QUIC-robustness row.
- **M-FB1 (QUIC transport-socket decode wired only into `filter_chains[]`, not `default_filter_chain`)** — the 61.2 `quicChain()` prefers `defaultChain` but the minimal slice uses `filter_chains[]`; a QUIC-listener-via-`default_filter_chain`-only config remains a documented boundary. RE-DEFERRED to 61.3/multi-chain.
- **M-FB2 (`quicTLSConfig`/`quicChain` map-iteration nondeterminism over `chainByName`)** — harmless for the single-chain slice; the deferral note in `quicChain()` must not overclaim determinism. RE-DEFERRED to 61.3 SNI-dispatch.

## Task checklist (mirrors PLAN-61.2)

- [ ] **Task 1** — PROGRESS scaffold + baselines + the 61.2 design pins + the SPEC correction + review-pickup disposition. (folded into the PLAN commit)
- [ ] **Task 2** — thread the listener `kind` (`IsQUIC`) through `network.FactoryCtx` → `hcm.ListenerCtx` (+ the manager sets it from `kind == kindQUIC`). [TDD]
- [ ] **Task 3** — lift the `codec_type HTTP3` reject (gated on `IsQUIC`) + add the HTTP3-on-non-QUIC config-parity reject + UPDATE `config_test.go:207-208`. [TDD]
- [ ] **Task 4** — `writeH3Reply` (ActionResponse → `http.ResponseWriter` adapter; pseudo-header skip). [TDD, `httptest.NewRecorder`]
- [ ] **Task 5** — the `runH3`/`ServeH3` dispatch arm (modeled on H1 `dispatchRequest`, reusing the H1 `Action` path) + `emitAccessLogH3` (`Protocol="HTTP/3"`). [TDD, `httptest` — NO quic-go]
- [ ] **Task 6** — the `network.H3Terminal` interface + `*hcm.Filter` satisfies it (`ServeH3` + a compile-assert). [TDD]
- [ ] **Task 7** — wire `serveQUICConnection` → `http3.Server{Handler}.ServeQUICConn` (find the chain terminal, assert `H3Terminal`, serve; ctx-honoring per M6-2) + `quicChain()`. [wiring]
- [ ] **Task 8** — the subject-side H3 GET→200 integration test (local quic-go `http3.Transport` client → routed `direct_response`). [TDD + -race]
- [ ] **Task 9** — BEHAVIOR_CONTRACT HTTP/3 serve extension + ADR-0281 §Context/§Decision/§Consequences + STATE + ROADMAP row-61-stays-in-progress verify + sentinel re-check + six-gate + router roll to the 61.3 PLAN. [docs + verify]

## Six-gate (recorded at Task 9 — RUN in the worktree `.worktrees/phase-61.2-impl`)

```
$ gofmt -l .
(expect empty — GOFMT_CLEAN)

$ golangci-lint run ./...
(expect empty, exit 0)

$ go vet ./...
(expect clean, exit 0)

$ go build ./... && echo BUILD_OK
(expect BUILD_OK)

$ go mod tidy -diff && echo MODTIDY_CLEAN
(expect MODTIDY_CLEAN — 61.2 adds NO module; quic-go landed at 61.1)

$ go list -deps ./internal/filter/hcm | grep -i quic-go || echo HCM-NO-QUICGO
(expect HCM-NO-QUICGO — the H3 arm is stdlib-only)

$ go list -deps ./internal/tls | grep -i quic-go || echo TLS-NO-QUICGO
(expect TLS-NO-QUICGO — the 61.1 gate holds)

$ go list -deps ./internal/listener | grep -i quic-go
(expect the quic-go module — confined to internal/listener/quic.go)

$ grep -rh '^func Fuzz' --include='*.go' . | wc -l
(expect 55 — +0)

$ ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l
(expect 105 — +0)

$ go test ./internal/filter/hcm/ ./internal/filter/network/ ./internal/listener/ -count=1
(expect ok, ok, ok)

$ go test ./internal/filter/hcm/ ./internal/filter/network/ ./internal/listener/ -race -count=1
(expect race-clean — the QUIC accept + http3.Server serve + ctx-cancel goroutines)
```

**FULL non-differential suite** (`go test $(go list ./... | grep -v '/test/differential') -count=1`) **+ the full 105-dir differential** (byte-stable — no new fixture, no TCP-path change; the QUIC serve path is reached only by a QUIC listener no fixture configures): DELEGATED to the controller on the frozen squash HEAD.

## Break evidence (recorded at Task 8 Step 5)

The 61.2 proof is a SUBJECT-SIDE integration test (no differential, so no cross-side `CompareBytes` break protocol). Liveness of the H3 GET→200 test is proven by temporarily reverting Task 7's serve body to the 61.1 handshake-only close (`-count=1` to defeat go-test caching, `reference_differential_break_protocol_count1`) and confirming WHICH assertion fires (dial-timeout / 404 / status≠200, `reference_deliberate_break_wrong_assertion`), then restoring. Record the confirmed break output here at IMPL.
