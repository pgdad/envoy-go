# Phase 60.1 PROGRESS — `xds-sds-stream-substrate` (IMPL)

> **Scaffold produced at the phase-60.1 PLAN stage** (docs-only). The IMPL executes `PLAN-60.1.md` task-by-task, subagent-driven (`feedback_execution_style`), in worktree `.worktrees/phase-60.1-impl`, branch `phase-60-xds-sds-stream-substrate-impl`, off master. This is the SUBSTRATE leg of the confirmed 60.1/60.2 split (SPEC §3.0); ANCHORS **ADR-0278** (§Context drafted at SPEC §13; §Decision/§Consequences land at this IMPL per ADR-0044). Row 60 STAYS `in-progress` — it flips `done` only once 60.2 also lands (ADR-0106, `reference_roadmap_split_phase_row_done`).

## Baseline counts (verify at IMPL start against the master tip; `git fetch` first)

| metric | baseline | anticipated exit |
|---|---|---|
| stat surface | 1201 | **1201** (+0 static; +5 DYNAMIC `sds.<secret>.*` per configured secret — proven by a unit +5 delta, materialized at boot only under a 60.2 SDS config) |
| fixtures | 104 (tail `0102-tracing-custom-tags-literal`) | **104** (+0 — the differential is 60.2) |
| fuzzers | 54 | **55** (`FuzzDiscoveryResponseParse`) |
| BackendKind tail | 38 (`H2GoawayResponder`) | **38** (+0 — the fake server is a driver-owned test helper, not a BackendKind) |
| DECISIONS tail | ADR-0277 (next-free ADR-0278) | **ADR-0278** (anchored at this IMPL) |
| new Go packages | 0 | **+1** prod (`internal/xds`) + **+1** test (`test/helpers/sdsserver`) |
| new go.mod modules | 0 | **+0** (go-control-plane v1.32.4 already carries `service/secret/v3` + `service/discovery/v3`) |

## D-XDS PLAN pins (settled in PLAN-60.1 §D-question resolutions)

- **CONFIG-SEAM:** ONE package `internal/xds`; `SecretProvider` HOMED there; `internal/xds` does NOT import `internal/tls` (the 60.2 `tls → xds` edge would else cycle) ⇒ `internal/xds` carries its own `dataSourceBytes`; the BIDI dial wrapper (`SDSClient`) is homed in `internal/grpcclient`.
- **STATS:** the 5-counter `sds.<secret>.*` subset is DYNAMIC (per-secret, `NewCounterIfAbsent` under an `IsValidName` guard); static no-SDS surface stays 1201 at 60.1.
- **FUZZER:** +1 `FuzzDiscoveryResponseParse` (54 → 55).
- **NODE / INITIAL-FETCH (60.1 slice):** the client POPULATES `DiscoveryRequest.node` from a given `Node{ID,Cluster}` (the node-required boot-reject is 60.2); `FetchInitialCertificate` blocks bounded by `initial_fetch_timeout` and returns a classified error on timeout/mgmt-down (the boot-FAIL DEPARTURE is 60.2).

## Task checklist (mirrors PLAN-60.1)

- [ ] **Task 1** — PROGRESS scaffold + baselines + the D-XDS PLAN-pin record.
- [ ] **Task 2** — `internal/xds` skeleton: package doc + `dataSourceBytes` + `secretTypeURL`.
- [ ] **Task 3** — `parseSecret` (Any→Secret→`*tls.Certificate`).
- [ ] **Task 4** — `fetchSecret` (SotW initial-request/recv/ACK-NACK over the `sdsStream` seam).
- [ ] **Task 5** — `sds.<secret>.*` 5-counter subset + `IsValidName` guard.
- [ ] **Task 6** — `grpcclient.SDSClient` BIDI wrapper.
- [ ] **Task 7** — `test/helpers/sdsserver` driver-owned fake SDS server.
- [ ] **Task 8** — `SecretProvider` + `Provider.FetchInitialCertificate` (blocking, `initial_fetch_timeout`).
- [ ] **Task 9** — `FuzzDiscoveryResponseParse` (54 → 55).
- [ ] **Task 10** — ADR-0278 body + STATE/ROADMAP/PROGRESS + the six-gate verify.

## Six-gate (recorded at Task 10)

```
gofmt -l internal/ test/ cmd/                → (pending)
golangci-lint run ./...                      → (pending)
go vet ./...                                 → (pending)
go build ./...                               → (pending)
go mod tidy -diff                            → (pending; expect EMPTY)
go test -race -count=1 ./internal/xds/... ./internal/grpcclient/... ./test/helpers/sdsserver/...   → (pending)
grep -rh '^func Fuzz' … | wc -l              → (pending; expect 55)
```
(NO full `test/differential` run at 60.1 — no new fixture, no production request-path change; that gate is 60.2.)

## Landed task commits

(recorded at each task close during the IMPL)

## Next

After the phase-60.1 IMPL lands → the **phase-60.2 PLAN** (`xds-sds-tls-cert-apply`: the `tls/config.go:153` one-arm lift + the config seam + the boot-fail-on-timeout DEPARTURE + the six strict-reject arms + the node-required boot-reject + the driver-owned-SDS differential; ANCHORS ADR-0280; row 60 flips `done` at that IMPL).
