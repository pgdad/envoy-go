# Phase 60.2 PROGRESS — `xds-sds-tls-cert-apply` (IMPL)

> **Scaffold produced at the phase-60.2 PLAN stage** (docs-only). The IMPL executes `PLAN-60.2.md` task-by-task, subagent-driven (`feedback_execution_style`), in worktree `.worktrees/phase-60.2-impl`, branch `phase-60-xds-sds-tls-cert-apply-impl`, off master. This is the TLS-APPLY leg of the confirmed 60.1/60.2 split (SPEC §3.0); ANCHORS **ADR-0280** (§Context re-uses the SPEC-60 §13 frame; §Decision/§Consequences land at this IMPL per ADR-0044). **Row 60 flips `in-progress` → `done` at THIS six-gate** — both legs now landed (60.1 substrate + 60.2 apply), ADR-0106 / `reference_roadmap_split_phase_row_done`. The xDS FAMILY STAYS OPEN.

## Baseline counts (verify at IMPL start against the master tip; `git fetch` first)

| metric | baseline | anticipated exit |
|---|---|---|
| stat surface | 1201 | **1201** (+0 static; +5 DYNAMIC `sds.<secret>.*` materialize at boot under the differential's SDS config — RE-VERIFY the count convention; the differential asserts the 5 named counters exist) |
| fixtures | 104 (tail `0102-tracing-custom-tags-literal`) | **105** (`0103-xds-sds-server-cert`) |
| fuzzers | 55 | **55** (+0 — `ParseSDSConfig` operates on typed protos, not untrusted wire; the wire boundary was 60.1's `FuzzDiscoveryResponseParse`) |
| BackendKind tail | 38 (`H2GoawayResponder`) | **38** (+0 — the SDS server is DIALED by the proxies, a driver-owned test helper, not a BackendKind) |
| DECISIONS tail | ADR-0278 (next-to-land ADR-0280; ADR-0279 = HTTP/3 phase 61) | **ADR-0280** (anchored at this IMPL; next-free **ADR-0281**) |
| new Go packages | 0 | **+0** (60.1 built `internal/xds` + `internal/xds/xdsgrpc` + `test/helpers/sdsserver`; 60.2 only ADDS `ParseSDSConfig` to `internal/xds` + `NewSDSProvider` to `internal/boot`) |
| new go.mod modules | 0 | **+0** (`go mod tidy -diff` EMPTY) |
| ROADMAP row 60 | `in-progress` | **`done`** (both legs landed, ADR-0106) |

## Cycle guard (LOAD-BEARING — ADR-0278; re-check at Tasks 2/3/5/9)

`internal/xds` proper imports NEITHER `grpcclient` NOR `cluster` NOR `tls`. At 60.2 `internal/tls` imports `internal/xds` ONLY for the `xds.SecretProvider` interface type (acyclic because `xds` is grpcclient/cluster/tls-free). The provider VALUE is built in `internal/boot`/`main.go` via `xdsgrpc.NewOpener(grpcclient.NewSDSClient(dialer, cluster))` and threaded down as a value. Verify: `go list -deps ./internal/tls | grep -E 'internal/(grpcclient|cluster|boot|listener)$'` prints nothing; `go list -deps ./internal/xds | grep -E 'internal/(tls|grpcclient|cluster|listener|boot)$'` prints nothing.

## 60.2 design pins (settled in PLAN-60.2 §"Design pins settled here")

- **CONFIG-SEAM:** thread a pre-built `xds.SecretProvider` value. `boot.NewSDSProvider(dialer, bs, baseDir, registry)` pre-scans the bootstrap listeners for the (single, MVP) downstream SDS-bound TLS context, enforces the node boot-requirement (arm 7), and builds the provider via `xdsgrpc.NewOpener(grpcclient.NewSDSClient(dialer, cluster))`. Threaded `Construct → NewManagerWithBaseDirAndAllowH2C → buildListenerRuntimeWithCtx → NewDownstreamConfig(ts, baseDir, provider)`. (Mirrors `boot.NewTracingProvider`.)
- **REJECT-HOME:** arms 1–4/8/9 in `xds.ParseSDSConfig` (`xds: sds:` prefix); arm 5 unchanged in `tls/config.go`; arm 6 = the existing string gated on `side != "downstream"`; arm 7 in `boot.NewSDSProvider`; arm 10 already in the 60.1 applier.
- **MVP CAP:** one `SdsSecretConfig` total (`ParseSDSConfig` rejects `len > 1`; `NewSDSProvider` rejects a second distinct SDS-bound context).
- **SERVED-LEAF ASSERTION:** the idiomatic `DriveReference`/`DriveSubject` + Step-7 `CompareBytes` cross-side capture (no new asserter/runner change); observable = `serial=<hex>\nsan=<sorted DNSNames>`; proven live by a two-different-certs break.
- **INITIAL-FETCH:** BLOCK-then-BOOT-FAIL departure — the lift PROPAGATES the provider's timeout/mgmt-down error → boot fails (proven by a `config_test.go` unit test, NOT a differential dir).
- **VALIDATE:** nil SDS provider (validate does not dial/fetch).

## Task checklist (mirrors PLAN-60.2)

- [ ] **Task 1** — PROGRESS scaffold + baselines + the 60.2 design pins. (folded into the PLAN commit)
- [ ] **Task 2** — `xds.ParseSDSConfig` (arms 1–4, 8, 9 + V3 + one-config MVP cap). [TDD]
- [ ] **Task 3** — `tls/config.go` one-arm downstream-gated lift + `provider` param; arms 1–6 + timeout-propagation + nil-provider reject. [TDD]
- [ ] **Task 4** — thread `sdsProvider` through the listener manager; `nil` at all existing callers. [mechanical + regression]
- [ ] **Task 5** — `boot.NewSDSProvider` (pre-scan + node arm 7 + provider build) + `Construct` param + main/validate wiring. [TDD]
- [ ] **Task 6** — `sdsserver.NewAtAddr` (0.0.0.0 bind) + `helpers.TLSServedLeaf`. [TDD]
- [ ] **Task 7** — `0103-xds-sds-server-cert` differential (driver-owned SDS server per side; served-leaf cross-side via `CompareBytes`); assertion proven live. [fixture]
- [ ] **Task 8** — BEHAVIOR_CONTRACT xDS/SDS section. [docs]
- [ ] **Task 9** — ADR-0280 + STATE + ROADMAP row 60 `done` + xDS deferred sentence + sentinel re-check + six-gate verify. [docs + verify]

## Six-gate (recorded at Task 9 — RUN in the worktree `.worktrees/phase-60.2-impl`)

```
(to be filled at the IMPL Task 9 completion)
gofmt -l .                                               → (expect clean)
golangci-lint run ./...                                  → (expect exit 0)
go vet ./...                                             → (expect exit 0)
go build ./...                                           → (expect ok)
go mod tidy -diff                                        → (expect EMPTY)
go test ./... -count=1                                   → (expect PASS)
go test ./test/differential/ -count=1                    → (expect PASS; byte-stable except 0103)
go list -deps ./internal/tls | grep -E 'internal/(grpcclient|cluster|boot|listener)$'   → (expect empty; TLS-CLEAN)
go list -deps ./internal/xds | grep -E 'internal/(tls|grpcclient|cluster|listener|boot)$' → (expect empty; XDS-CLEAN)
grep -rh '^func Fuzz' --include='*.go' . | wc -l         → 55
ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l         → 105
```

## Break evidence (recorded at Task 7 Step 6)

```
(to be filled — the two-different-certs served-leaf CompareBytes liveness break, -count=1 -run 'TestDifferential/0103-xds-sds-server-cert')
```
