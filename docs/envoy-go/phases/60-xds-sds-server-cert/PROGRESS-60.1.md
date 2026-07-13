# Phase 60.1 PROGRESS — `xds-sds-stream-substrate` (IMPL)

> **Scaffold produced at the phase-60.1 PLAN stage** (docs-only). The IMPL executes `PLAN-60.1.md` task-by-task, subagent-driven (`feedback_execution_style`), in worktree `.worktrees/phase-60.1-impl`, branch `phase-60-xds-sds-stream-substrate-impl`, off master. This is the SUBSTRATE leg of the confirmed 60.1/60.2 split (SPEC §3.0); ANCHORS **ADR-0278** (§Context drafted at SPEC §13; §Decision/§Consequences land at this IMPL per ADR-0044). Row 60 STAYS `in-progress` — it flips `done` only once 60.2 also lands (ADR-0106, `reference_roadmap_split_phase_row_done`).

## Baseline counts (verify at IMPL start against the master tip; `git fetch` first)

| metric | baseline | anticipated exit |
|---|---|---|
| stat surface | 1201 | **1201** (+0 static; +5 DYNAMIC `sds.<secret>.*` per configured secret — proven by a unit +5 delta, materialized at boot only under a 60.2 SDS config) |
| fixtures | 104 (tail `0102-tracing-custom-tags-literal`) | **104** (+0 — the differential is 60.2) |
| fuzzers | 54 | **55** (`FuzzDiscoveryResponseParse`) — CONFIRMED (`grep -rh '^func Fuzz' --include='*.go' . | wc -l` = 55) |
| BackendKind tail | 38 (`H2GoawayResponder`) | **38** (+0 — the fake server is a driver-owned test helper, not a BackendKind) |
| DECISIONS tail | ADR-0277 (next-free ADR-0278) | **ADR-0278** (anchored at this IMPL; next-free **ADR-0279**) |
| new Go packages | 0 | **+2** prod (`internal/xds`, `internal/xds/xdsgrpc` — REVISED from the anticipated +1, see the EXIT NOTE below) + **+1** test (`test/helpers/sdsserver`) |
| new go.mod modules | 0 | **+0** (`go mod tidy -diff` EMPTY — go-control-plane v1.32.4 already carries `service/secret/v3` + `service/discovery/v3`) |

### EXIT NOTE — the xdsgrpc cycle-avoidance correction (Task 8)

The PLAN's single-package premise ("`internal/grpcclient` is tls-free, so its SDS adapter can live inside `internal/xds`") was FALSE: `internal/grpcclient → internal/cluster → internal/tls`. Housing the adapter in `internal/xds` would have made `internal/xds` itself transitively import `internal/tls`, cycling against the 60.2 `internal/tls → internal/xds` edge (`tls → xds → grpcclient → cluster → tls`). Task 8's same-task correction commit (`3ddf6eb4`) EXPORTED the `Stream`/`StreamOpener` seam interfaces from `internal/xds` and re-homed the grpcclient-carrying adapter to a NEW sibling package `internal/xds/xdsgrpc` (`Opener`/`NewOpener(*grpcclient.SDSClient) xds.StreamOpener`), imported only by the (not-yet-wired) 60.2 boot path. Verified via `go list -deps`: `internal/xds` → only `internal/stats` (in-module); `internal/xds/xdsgrpc` → `internal/tls`/`internal/cluster`/`internal/grpcclient`/`internal/xds`. `internal/tls → internal/xds` at 60.2 is therefore acyclic. This is recorded in ADR-0278 §Decision/§Consequences.

## D-XDS PLAN pins (settled in PLAN-60.1 §D-question resolutions)

- **CONFIG-SEAM:** ONE package `internal/xds`; `SecretProvider` HOMED there; `internal/xds` does NOT import `internal/tls` (the 60.2 `tls → xds` edge would else cycle) ⇒ `internal/xds` carries its own `dataSourceBytes`; the BIDI dial wrapper (`SDSClient`) is homed in `internal/grpcclient`.
- **STATS:** the 5-counter `sds.<secret>.*` subset is DYNAMIC (per-secret, `NewCounterIfAbsent` under an `IsValidName` guard); static no-SDS surface stays 1201 at 60.1.
- **FUZZER:** +1 `FuzzDiscoveryResponseParse` (54 → 55).
- **NODE / INITIAL-FETCH (60.1 slice):** the client POPULATES `DiscoveryRequest.node` from a given `Node{ID,Cluster}` (the node-required boot-reject is 60.2); `FetchInitialCertificate` blocks bounded by `initial_fetch_timeout` and returns a classified error on timeout/mgmt-down (the boot-FAIL DEPARTURE is 60.2).

## Task checklist (mirrors PLAN-60.1)

- [x] **Task 1** — PROGRESS scaffold + baselines + the D-XDS PLAN-pin record. (folded into the PLAN commit `62820c3c`)
- [x] **Task 2** — `internal/xds` skeleton: package doc + `dataSourceBytes` + `secretTypeURL`. (`8cc55f3f`)
- [x] **Task 3** — `parseSecret` (Any→Secret→`*tls.Certificate`). (`9ffa5561`)
- [x] **Task 4** — `fetchSecret` (SotW initial-request/recv/ACK-NACK over the `sdsStream` seam). (`e2a54475`)
- [x] **Task 5** — `sds.<secret>.*` 5-counter subset + `IsValidName` guard. (`56d87082`)
- [x] **Task 6** — `grpcclient.SDSClient` BIDI wrapper. (`ba994853`)
- [x] **Task 7** — `test/helpers/sdsserver` driver-owned fake SDS server. (`11f76a2c`)
- [x] **Task 8** — `SecretProvider` + `Provider.FetchInitialCertificate` (blocking, `initial_fetch_timeout`). (`ddcd3a06`, corrected by `3ddf6eb4` — the `internal/xds/xdsgrpc` cycle-avoidance split, see the EXIT NOTE above)
- [x] **Task 9** — `FuzzDiscoveryResponseParse` (54 → 55). (`f11d14d3`)
- [x] **Task 10** — ADR-0278 body + STATE/ROADMAP/PROGRESS + the six-gate verify. (this commit)

## Six-gate (recorded at Task 10 — RUN in the worktree `.worktrees/phase-60.1-impl`, 2026-07-13)

```
$ gofmt -l internal/ test/ cmd/
(no output)                                                                    → PASS

$ golangci-lint run ./...
(no output, exit 0)                                                            → PASS

$ go vet ./...
(no output, exit 0)                                                            → PASS

$ go build ./...
(no output, exit 0)                                                            → PASS

$ go mod tidy -diff
(no output, exit 0 — EMPTY)                                                    → PASS

$ go test -race -count=1 ./internal/xds/... ./internal/xds/xdsgrpc/... ./internal/grpcclient/... ./test/helpers/sdsserver/...
ok  	github.com/pgdad/envoy-go/internal/xds	1.226s
ok  	github.com/pgdad/envoy-go/internal/xds/xdsgrpc	1.020s
ok  	github.com/pgdad/envoy-go/internal/grpcclient	1.141s
ok  	github.com/pgdad/envoy-go/test/helpers/sdsserver	1.018s                → PASS (all ok)

$ grep -rh '^func Fuzz' --include='*.go' . | wc -l
55                                                                             → PASS (matches anticipated 55)
```
(NO full `test/differential` run at 60.1 — no new fixture, no production request-path change; `internal/xds` is not imported by any boot path yet; that gate is 60.2's. `go build ./...` [above] confirms the whole tree still compiles with the two new packages present.)

## Landed task commits

- Task 1 (scaffold): folded into the PLAN commit `62820c3c`
- Task 2: `8cc55f3f`
- Task 3: `9ffa5561`
- Task 4: `e2a54475`
- Task 5: `56d87082`
- Task 6: `ba994853`
- Task 7: `11f76a2c`
- Task 8: `ddcd3a06` (+ correction `3ddf6eb4`)
- Task 9: `f11d14d3`
- Task 10: this commit (ADR-0278 body + STATE/ROADMAP/PROGRESS + six-gate)

## Exit counts (as-built, this commit)

stat surface **1201** (+0 static; +5 DYNAMIC `sds.*` proven) · fixtures **104** (+0) · fuzzers **55** (54 → 55, `FuzzDiscoveryResponseParse`) · BackendKind tail **38** (+0) · DECISIONS tail **ADR-0278** (ADR-0277 → ADR-0278; next-free **ADR-0279**) · new Go packages **+2** prod (`internal/xds`, `internal/xds/xdsgrpc`) + **+1** test (`test/helpers/sdsserver`) · new go.mod modules **+0**. Row 60 STAYS `in-progress` (60.2 pending, ADR-0106); the xDS family STAYS OPEN.

## Next

After the phase-60.1 IMPL lands → the **phase-60.2 PLAN** (`xds-sds-tls-cert-apply`: the `tls/config.go:153` one-arm lift + the config seam + the boot-fail-on-timeout DEPARTURE + the six strict-reject arms + the node-required boot-reject + the driver-owned-SDS differential; ANCHORS ADR-0280; row 60 flips `done` at that IMPL).
