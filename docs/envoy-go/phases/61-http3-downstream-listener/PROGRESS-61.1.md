# Phase 61.1 PROGRESS — `http3-quic-substrate` (IMPL)

> **Scaffold produced at the phase-61.1 PLAN stage** (docs-only). The IMPL executes `PLAN-61.1.md` task-by-task, subagent-driven (`feedback_execution_style`), in worktree `.worktrees/phase-61.1-impl`, branch `phase-61-http3-quic-substrate-impl`, off master. This is the FIRST leg of the confirmed 61.1/61.2/61.3 split (SPEC-61 §3.0); ANCHORS **ADR-0279** (§Context re-uses the SPEC-61 §13 frame; §Decision/§Consequences land at this IMPL per ADR-0044). **Row 61 STAYS `in-progress` at this six-gate** — it flips `done` only when ALL THREE legs land (ADR-0106 / `reference_roadmap_split_phase_row_done`). The HTTP/3 FAMILY STAYS OPEN.

## Baseline counts (verify at IMPL start against the master tip; `git fetch` first)

| metric | baseline | anticipated exit |
|---|---|---|
| stat surface | 1201 | **1201** (+0 — the QUIC listener REUSES `registerListenerMetrics`; `downstream_cx_total`/`downstream_cx_active` are per-bound-address DYNAMIC names, not new SURFACE registrations) |
| fixtures | 105 (tail `0103-xds-sds-server-cert`) | **105** (+0 — no fixture in 61.1; the cross-side H3-GET `0102` is leg 61.3) |
| fuzzers | 55 | **55** (+0 — quic-go owns H3 framing + QPACK; the only hand-rolled parse is the bootstrap config, reachable from the existing listener parse — no new `func Fuzz`) |
| BackendKind tail | 38 (`H2GoawayResponder`) | **38** (+0 — 61.1 has no differential/backend) |
| DECISIONS tail | ADR-0280 (ADR-0279 RESERVED for HTTP/3, lands here) | **ADR-0279** (authored at this IMPL, out of numeric order after ADR-0280 per the reservation ledger; next-free **ADR-0281**) |
| new production Go packages | 0 | **+0** (the QUIC accept path folds into `internal/listener` as `quic.go`; it needs the private `listenerRuntime`/`chainInfo` fields, so a separate package would force exporting internals) |
| new go.mod modules | 0 | **+1** (`github.com/quic-go/quic-go v0.54.1` — the FIRST external module + transitive `quic-go/qpack`, `golang.org/x/crypto`/`x/mod`/`x/tools`, bumps to `x/net`/`x/sys`/`x/sync`) |
| ROADMAP row 61 | `in-progress` | **`in-progress`** (NO flip — 61.2 + 61.3 pending) |

## Import hygiene (LOAD-BEARING — re-check at Tasks 3/6/9)

quic-go (`github.com/quic-go/quic-go`) is imported ONLY in `internal/listener/quic.go`. `internal/tls` imports the go-control-plane quic transport PROTO (`quicv3`), NEVER quic-go. Verify: `go list -deps ./internal/tls | grep -i quic-go` prints NOTHING (expect the `TLS-NO-QUICGO` echo). The `internal/tls → internal/xds` interface-only edge (60.2, ADR-0278/0280) is unaffected — 61.1 adds no new cross-package edge beyond `internal/listener → quic-go` (external) and `internal/tls → quicv3` (proto).

## 61.1 design pins (settled in PLAN-61.1 §"Design pins settled here")

- **KIND DISCRIMINANT:** `l.GetUdpListenerConfig().GetQuicOptions() != nil` → a per-listener `kind listenerKind` (`kindTCP` zero-value / `kindQUIC`) on `listenerRuntime`, set in `buildListenerRuntimeWithCtx`.
- **QUIC TLS DECODE:** a NEW `internal/tls.NewQUICDownstreamConfig(ts, baseDir)` unwraps `QuicDownstreamTransport` → the inner `DownstreamTlsContext` → the SHARED `commonTLSContextToConfig` (ALPN `h3` + cert + empty-cert mandatory-TLS error reused). QUIC carries no SDS in 61.1 → provider `nil`.
- **ACCEPT PATH:** a same-package `internal/listener/quic.go` (`startQUIC`/`quicAcceptLoop`/`serveQUICConnection`/`quicTLSConfig`) — methods on `*listenerRuntime` reusing the private fields; quic-go confined here. +0 packages.
- **QUIC CLOSE:** two new `*listenerRuntime` fields — `udpConn *net.UDPConn` + `quicCloser io.Closer` (the `*quic.Listener`); `Start`-unwind + `Stop` close BOTH (quic-go's `Listener.Close` does not close the packet conn); `Listeners()` reports the QUIC bound addr.
- **HANDSHAKE-ONLY:** `serveQUICConnection` Inc's the cx counters + `CloseWithError(0, "")` — NO H3 decode/dispatch (61.2). Proof: a local quic-go client asserting ALPN `h3` + TLS 1.3.
- **MANDATORY TLS:** a QUIC listener with no transport socket → config-parity reject `quic listener requires a transport_socket (mandatory TLS)` (SPEC §11 arm reject-C).
- **STRICT-REJECT ROSTER (ADR-0080):** `enable_early_data`, `quic_options.{proof_source_config, connection_id_generator_config, reject_new_connections, server_preferred_address_config}`, runtime-disabled `enabled` → distinct substrings. Accept-and-ignore: `idle_timeout`, `crypto_handshake_timeout`, `downstream_socket_config`, nested core knobs.
- **NO HTTP3-on-non-QUIC parity reject in 61.1** — it needs the `codec_type HTTP3` accept (still rejected at `hcm/config.go:240` until 61.2), so it is a 61.2 concern.

## Module decision (D-H3-QUICLIB, PINNED — SPEC §2.4/§4.1/§11)

`github.com/quic-go/quic-go v0.54.1` EXACTLY — the LAST release keeping the project's `go 1.23.0` directive (v0.55.0 → `go 1.24`; v0.60.0 → `go 1.25`). Re-confirmed at PLAN time: v0.54.1's own `go.mod` declares `go 1.23`. Interop with reference Envoy `contrib-v1.37.2` H3 PROVEN (SPEC §11: `HTTP/3.0`, 200, ALPN `h3`, TLS 1.3). Do NOT bump the project `go` directive; if `go mod tidy` changes it, the wrong quic-go was pulled.

## Task checklist (mirrors PLAN-61.1)

- [ ] **Task 1** — PROGRESS scaffold + baselines + the 61.1 design pins. (folded into the PLAN commit)
- [ ] **Task 2** — lift the `transport_protocol "quic"` filter-chain-match reject + flip the reject test + a positive test. [TDD]
- [ ] **Task 3** — `internal/tls.NewQUICDownstreamConfig` (unwrap the QUIC transport socket, reuse `commonTLSContextToConfig`, ALPN h3, mandatory-TLS error). [TDD]
- [ ] **Task 4** — the `listenerKind` discriminant + the QUIC transport-socket branch + the mandatory-TLS config-parity reject in `buildListenerRuntimeWithCtx`. [TDD]
- [ ] **Task 5** — add `github.com/quic-go/quic-go v0.54.1` (the FIRST external module) + `go mod tidy` + a compile-gate smoke test. [module + TDD]
- [ ] **Task 6** — the UDP/QUIC listen path (`Start` kind-branch + `quicAcceptLoop`/`serveQUICConnection` handshake substrate in `quic.go` + `Stop`/`Listeners` kind-awareness) + the subject-side handshake integration test (local quic-go client, ALPN h3 + TLS 1.3). [TDD + -race]
- [ ] **Task 7** — the QUIC strict-reject roster (0-RTT + `quic_options` tuning sub-fields, ADR-0080 distinct substrings). [TDD]
- [ ] **Task 8** — BEHAVIOR_CONTRACT HTTP/3 QUIC-substrate section. [docs]
- [ ] **Task 9** — ADR-0279 §Decision/§Consequences + STATE + ROADMAP row-61-stays-in-progress verify + sentinel re-check + six-gate + router roll to the 61.2 PLAN. [docs + verify]

## Six-gate (recorded at Task 9 — RUN in the worktree `.worktrees/phase-61.1-impl`)

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
(expect MODTIDY_CLEAN — tidy despite the new module; the module MODIFIES go.mod/go.sum
 in the commit diff, but a proper `go get`+`go mod tidy` leaves the tree tidy)

$ go list -deps ./internal/tls | grep -i quic-go || echo TLS-NO-QUICGO
(expect TLS-NO-QUICGO — quic-go confined to internal/listener)

$ grep -rh '^func Fuzz' --include='*.go' . | wc -l
(expect 55 — +0)

$ ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l
(expect 105 — +0)

$ grep -c 'quic-go/quic-go v0.54.1' go.mod
(expect 1 — the first external module)

$ go test ./internal/tls/ ./internal/listener/ -count=1
(expect ok, ok)

$ go test ./internal/tls/ ./internal/listener/ -race -count=1
(expect race-clean — the QUIC accept goroutine + Stop nil-writes)
```

**FULL non-differential suite** (`go test $(go list ./... | grep -v '/test/differential') -count=1`) **+ the full 105-dir differential** (byte-stable — no new fixture, no TCP-path change): DELEGATED to the controller on the frozen squash HEAD.

## Break evidence (recorded at Task 6 Step 7 / Task 7 Step 4)

The 61.1 proof is a SUBJECT-SIDE integration test (no differential, so no cross-side `CompareBytes` break protocol). Liveness of the handshake test is proven by the assertions biting: a deliberate break (e.g. drop `"h3"` from the listener's `alpn_protocols`, or set the client `NextProtos` to `["h2"]`) makes `NegotiatedProtocol != "h3"` fire. Each strict-reject arm is proven RED-first (Task 7 Step 2). Record the confirmed break output here at IMPL.
