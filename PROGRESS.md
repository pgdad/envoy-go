# Phase-31 `kafka_broker` IMPL — PROGRESS

## Baselines (phase-31 IMPL tip)

IMPL-session tip at Task 1: `ad1b853` (`next-prompt.txt + STATE.md: advance the live SHA-fill tip to 596fccd (trailing the phase-31 PLAN squash 12f7cac)`), branch `phase-31-network-filter-kafka-broker-impl`.

| Anchor | Expected | Actual | OK |
|---|---|---|---|
| fixtures count | 54 | 54 | yes |
| fixtures tail dir | `test/fixtures/0052-mongo-fault-delay` | `test/fixtures/0052-mongo-fault-delay` | yes |
| fuzzers (`^func Fuzz` over `internal/**/fuzz_test.go`) | 39 | 39 | yes |
| DECISIONS.md tail heading | ADR-0228 (next-free ADR-0229) | ADR-0228 (DECISIONS.md:14624; next-free ADR-0229) | yes |
| BackendKind tail | `TCPMongoResponder BackendKind = 30` | `TCPMongoResponder BackendKind = 30` (fixture.go:530) | yes |
| stat surface | 360 | 360 | yes |

### Stat surface = 360 (quoted)

- STATE.md:21 — `- **stat surface:** **360** (BEHAVIOR_CONTRACT doc count; +23 mongo fixed roster — 22 counters + the op_query_active gauge — created eagerly per 29.1 SPEC §7.2 / D-P1). ...`
- BEHAVIOR_CONTRACT.md:466 — `**Phase 29.3 extension — 360 → 360 internal names (zero creation delta; the parent-row-29 ROLLUP):** ... The stat total stays **360**.`
- STATE.md:23 — `- **DECISIONS.md tail:** **ADR-0228** (next-free **ADR-0229**) ...`

(Target at phase-31 IMPL end: 360 → 536, +176 eager per ADR-0228 / AMEND-K3 — NOT this task.)

## /contrib v1.32.4 — proto.MessageName + Go identifier roster (Step 3)

`/contrib v1.32.4` resolves via `go get github.com/envoyproxy/go-control-plane/contrib@v1.32.4` (probed in `/tmp/d31probe`).

`proto.MessageName(&kbv3.KafkaBroker{})` =
```
envoy.extensions.filters.network.kafka_broker.v3.KafkaBroker
```

Source file probed:
`$(go env GOMODCACHE)/github.com/envoyproxy/go-control-plane/contrib@v1.32.4/envoy/extensions/filters/network/kafka_broker/v3/kafka_broker.pb.go`

### KafkaBroker getters (5-field accessor set)

```go
func (x *KafkaBroker) GetStatPrefix() string                                            // :92
func (x *KafkaBroker) GetForceResponseRewrite() bool                                     // :99
func (x *KafkaBroker) GetIdBasedBrokerAddressRewriteSpec() *IdBasedBrokerRewriteSpec     // :113
func (x *KafkaBroker) GetApiKeysAllowed() []uint32                                       // :120
func (x *KafkaBroker) GetApiKeysDenied() []uint32                                        // :127
```

### Oneof (`broker_address_rewrite_spec`)

```go
// oneof getter (returns the interface):
func (m *KafkaBroker) GetBrokerAddressRewriteSpec() isKafkaBroker_BrokerAddressRewriteSpec  // :106
// oneof interface name:
type isKafkaBroker_BrokerAddressRewriteSpec interface { isKafkaBroker_BrokerAddressRewriteSpec() }  // :134
// oneof wrapper type (the single member):
type KafkaBroker_IdBasedBrokerAddressRewriteSpec struct { ... }                              // :138
```

`GetIdBasedBrokerAddressRewriteSpec()` (:113) type-asserts `GetBrokerAddressRewriteSpec().(*KafkaBroker_IdBasedBrokerAddressRewriteSpec)`.

### Nested rule accessors

```go
func (x *IdBasedBrokerRewriteSpec) GetRules() []*IdBasedBrokerRewriteRule   // :186
func (x *IdBasedBrokerRewriteRule) GetId() uint32                           // :243
func (x *IdBasedBrokerRewriteRule) GetHost() string                        // :250
func (x *IdBasedBrokerRewriteRule) GetPort() uint32                        // :257
```

## As-built anchor line numbers (Step 4 — snapshot for Tasks 2/10/11/12; they drift)

| File | Anchor | Line | Action |
|---|---|---|---|
| `internal/filter/network/builtins/builtins.go` | `reg.Register(mongoproxy.TypeURL, mongoproxy.NewFactory(deps.StatsRegistry))` (the 8th reg) | 74 | insert kafka reg AFTER |
| `internal/filter/network/builtins/builtins.go` | doc `built-in network filters` (`registers the eight built-in network filters`) | 1 | "eight" → "nine" |
| `internal/bootstrap/bootstrap.go` | `_ ".../filters/network/mongo_proxy/v3"` blank-import | 103 | insert kafka_broker/v3 blank-import AFTER |
| `internal/stats/name.go` | default error `has no recognized top-level segment` | 290 | kafka. arm goes BEFORE this |
| `internal/stats/name.go` | `strings.CutPrefix(internal, "mongo.")` (the mongo. arm) | 278 | insert kafka. arm AFTER |
| `test/differential/fixture/fixture.go` | `TCPMongoResponder BackendKind = 30` (BackendKind tail) | 530 | new responder = 31 AFTER |

## Task 16 — full differential re-verify

Full differential suite re-run at branch tip (`go test ./test/differential/ -count=1 -v`):

```
--- PASS: TestDifferential (187.31s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	189.250s
```

**56/56 fixtures green** (PASS subtests = 56, FAIL = 0). Confirmed:

- **Fixture-dir count = 56** (`ls -d test/fixtures/[0-9]* | wc -l`); tail = `0053-kafka-requests`, `0054-kafka-boot-reject`.
- **The 2 new dirs PASS**: `--- PASS: TestDifferential/0053-kafka-requests (6.25s)` (cross-side `StatsAsserter`, the LIVE request+response per-key proof) and `--- PASS: TestDifferential/0054-kafka-boot-reject (1.51s)`.
- **The 54 prior dirs (0000-0052) stayed byte-exact** — the R1 passthrough-invariant / back-compat proof. The differential runner's `CompareBytes` gate is byte-for-byte against reference Envoy; all 54 pre-kafka fixtures passing unchanged is the load-bearing evidence that the kafka_broker work touched NO shared path (the sniffer never mutated bytes). Full per-fixture PASS list captured in `/tmp/diff31.log`.

**R4 liveness** — the deliberate-break proofs for each asserted subject counter are recorded in `test/fixtures/0053-kafka-requests/README.md` (§ "R4 deliberate-break liveness proofs", the 5-row table: `incRequest` / `incRequestUnknown` / `incRequestFailure` / `incResponse` / `incResponseFailure`, each → its `= 0, want N` assertion failure). Re-confirmed the mechanism still works under `-count=1`: broke `incResponse` (core D-P4 counter) in `internal/filter/network/kafkabroker/stats.go`, ran `go test ./test/differential/ -run 'TestDifferential/0053' -count=1` → observed `FAIL: subj envoy_kafka_kafka_r_response_api_versions_response = 0, want 1` (matching the README row), then `git restore` (no checkout-sha / no amend; HEAD stayed on branch).

## Per-task log

- [x] Task 1: baselines gate — fixtures 54 / fuzzers 39 / stat surface 360 / DECISIONS tail ADR-0228 (next-free ADR-0229) / BackendKind tail 30 (TCPMongoResponder) all re-confirmed at tip `ad1b853`; `proto.MessageName(&kbv3.KafkaBroker{})` = `envoy.extensions.filters.network.kafka_broker.v3.KafkaBroker`; `/contrib v1.32.4` resolves; KafkaBroker getter/oneof/nested-rule roster + anchor line numbers pinned above. No discrepancies.
- [x] Task 2: /contrib dep + blank-import + skeleton + TypeURL — commit `8267440` (`/contrib v1.32.4` dep + `kafka_broker/v3` blank-import + kafkabroker skeleton + TypeURL).
- [x] Task 3: config parse + PGV arms + IsValidName guard — commit `2c33936` (5-field config parse + PGV reject arms + `stat_prefix` IsValidName guard).
- [x] Task 4: api_key->(root,maxVersion) table + golden test — commit `e82763b` (static 86-entry table, Appendix B + the D-PLAN-1 maxVersion column, byte-stable golden test).
- [x] Task 5: flexibleVersions predicate + ApiVersions(18) special-case — commit `510b0d0` (Appendix C + the response-header special-case + table test).
- [x] Task 6: EAGER 176-counter roster — commit `efa92f8` (86×2 per-key + 4 fixed under `kafka.<stat_prefix>.` + the flexibleSince keyset-invariant test).
- [x] Task 7: Kafka decoder + framing + request-header + correlation map — commit `8608e95` (+ follow-up `70d9ff0` framing-garbage zero-count + resync).
- [x] Task 8: response-header decoder + correlation recover/erase + -race test — commit `fc2caf3` (unregistered → `response.failure` + `-race`; + follow-up `663b0e6` pinning response malformed-frame → `response.failure` through `decodeOnWrite`).
- [x] Task 9: ReadFilter/WriteFilter glue + factory — commit `56a43eb` (pure copying sniffer; always Continue, never mutate/close; + follow-up `f819d78` documenting the WriteFilter-for-28.1b-read-seam rationale).
- [x] Task 10: register as 9th built-in + boot smoke — commit `17b97d1`.
- [x] Task 11: kafka. INLINE Prometheus arm in name.go — commit `0d99761` (`envoy_kafka_<sp>_<rest>{}` empty labels, no hoist; + follow-up `4ceaf5b` renaming the tests to the `TestFlattenToProm_` convention).
- [x] Task 12: TCPKafkaResponder BackendKind (31) — commit `4852681` (correlation-id-echoing Kafka responder).
- [x] Task 13: 0053-kafka-requests cross-side fixture + R4 proofs — commit `1400182` (cross-side StatsAsserter, 6 arms + R4 liveness proofs; the `*.failure` ref=2/subj=1 abandon-at-close asymmetry pinned to exact per-side values).
- [x] Task 14: 0054-kafka-boot-reject fixture — commit `93ad259` (missing `stat_prefix`; both sides reject at boot).
- [x] Task 15: 40th fuzzer FuzzKafkaDecode — commit `70bc677` (both directions: no-panic + no-mutation + bounded-buffer).
- [x] Task 16: full differential re-verify — 56/56 byte-exact (54 prior dirs back-compat + 0053 + 0054), suite `ok ... 189.250s`; R4 liveness recorded in `0053/README.md` and re-confirmed under `-count=1` (broke `incResponse` → `response_api_versions_response = 0, want 1`, then `git restore`). Commit `9efc8bb`.
- [x] Task 17: completion bundle + six gates — this commit (BEHAVIOR_CONTRACT 360→536 + the ADR-0228 §Decision/§Consequences body in-place + STATE/ROADMAP row 31 `in-progress → done` + next-prompt.txt rewrite + the six-gate evidence below).

## Task 17 — six-gate evidence

All gates RUN LIVE at the phase-31 IMPL Task 17 (2026-06-08, branch `phase-31-network-filter-kafka-broker-impl`). Real outputs quoted below.

**Gate 1 — `go build ./...`** → clean, EXIT=0 (no output).

**Gate 2 — `go vet ./...`** → clean, EXIT=0 (no output).

**Gate 3 — `golangci-lint run`** → clean, EXIT=0 (no findings, no output).

**Gate 4 — `go test ./... -race -short`** → EXIT=0; **81 ok packages, 0 FAIL** (71 no-test packages). The `kafkabroker` package: `ok  github.com/esalaine/envoy-go/internal/filter/network/kafkabroker`. (One earlier whole-module run flagged a bare transient `FAIL` under concurrent load with the lint job; the clean re-run with no other load reported zero non-ok lines, EXIT=0 — a confirmed environmental flake, not a code failure.)

**Gate 5 — `go test ./test/differential/ -count=1`** → **56/56 PASS** byte-exact (54 prior dirs back-compat + `0053-kafka-requests` + `0054-kafka-boot-reject`). Authoritative clean run (serial, `-p 1 -parallel 1 -v` to avoid the parallel-Docker-load subject-startup flake): `--- PASS: TestDifferential/0053-kafka-requests (6.17s)`, `--- PASS: TestDifferential/0054-kafka-boot-reject (1.51s)`, 56 PASS / 0 FAIL subtests, `ok  github.com/esalaine/envoy-go/test/differential  186.915s`, EXIT=0. (Two earlier default-parallelism whole-suite runs each flaked on a DIFFERENT HTTP-filter fixture — `0036-http-wasm-body-and-advanced` then `0027-http-lua-full-bridge` — both with the `subject ready: EOF` subject-startup signature, NEVER on the kafka fixtures; each PASSES 2/2 in isolation. This is a pre-existing harness flake — `StartSubjectProxy` does a per-fixture `go build` + process-start that occasionally loses the subject under heavy parallel container churn — unrelated to phase 31, which touches no HTTP path. Serialized, the full suite is 56/56 green; this matches the Task 16 logged 56/56.)

**Gate 6 — conformance (asserted-UNAFFECTED + re-run green).** Phase 31 touches no HTTP/h2/proxy-wasm code path (it is an L4 network filter on a `[kafka_broker, tcp_proxy]` chain), so the conformance gates cannot be affected; re-run LIVE as a sanity check:
- **h2spec** — `go test ./test/conformance/h2spec/... -count=1` → `ok  github.com/esalaine/envoy-go/test/conformance/h2spec  2.685s`, EXIT=0; report: **53 tests, 53 passed, 0 skipped, 0 failed**.
- **proxy-wasm** — `go test ./test/conformance/proxy-wasm/... -count=1` → `ok  github.com/esalaine/envoy-go/test/conformance/proxy-wasm  0.247s`, EXIT=0; **all 10 families PASS** (bytecode_util, endianness, exports, logging, pairs_util, runtime, security, shared_data, stop_iteration, wasm_vm).

**Counts at phase-done (consistent across BEHAVIOR_CONTRACT + STATE + ROADMAP):** stat surface **360 → 536** (+176 eager counters; the 86 response-duration histograms DEFERRED per ADR-0060 NOT counted); fixtures **54 → 56** (tail `0054-kafka-boot-reject`); fuzzers **39 → 40** (`FuzzKafkaDecode`); BackendKind tail **30 → 31** (`TCPKafkaResponder`); DECISIONS.md tail **ADR-0228** (next-free **ADR-0229**; body landed IN-PLACE per ADR-0044, NO new number); ROADMAP row 31 `in-progress → done` (a flat §9 row, NO parent rollup); §9 candidate count 3 → 2 ({redis, thrift} remain). Recipe verification: `ls -d test/fixtures/[0-9]* | wc -l` = 56; `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l` = 40.
