# Phase 31 PLAN — `kafka_broker` network filter (header-only decode + full per-API-key request/response counter parity)

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended, per `feedback_execution_style`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Subagents commit **LOCAL-ONLY** (`feedback_subagents_no_push`); the controller squash-merges + pushes at stage-close (`feedback_push_to_origin`). Work in the git worktree (`feedback_git_worktrees`). Each task runs `gofmt -l` + `golangci-lint run` on the touched packages before its commit (`feedback_pertask_gofmt_lint`). After a temporary deliberate break (Task 16 R4 liveness), use `go test -count=1` to defeat result caching (`reference_differential_break_protocol_count1`). A deliberate-break check that detaches HEAD in a worktree must restore with `git restore` (NOT `git checkout <sha>`) per `feedback_subagent_worktree_detach`.

**Goal:** Land the `internal/filter/network/kafkabroker/` package — TypeURL + 5-field `KafkaBroker` config parse (incl. the PGV arms + the `stat_prefix` `IsValidName` guard), an in-house `encoding/binary` Kafka primitive decoder (INT16 / INT32 / NULLABLE_STRING / UNSIGNED_VARINT tagged-fields) over a 4-byte INT32 length prefix, a request-header decoder + a response-header decoder, a static `api_key → (message-name, maxVersion)` table (86 keys, Kafka 3.9.1) + a static `flexibleVersions(api_key)` predicate, a per-connection `correlation_id → (api_key, api_version)` map under an ADR-0223 mutex, and an EAGERLY-created 176-counter fixed roster under `kafka.<stat_prefix>.` — wired as the 9th built-in with the project's FIRST `/contrib v1.32.4` dep, the `kafka.` INLINE Prometheus arm, the new correlation-id-echoing BackendKind, fixtures `0053-kafka-requests` (cross-side `StatsAsserter`) + `0054-kafka-boot-reject`, and the 40th fuzzer `FuzzKafkaDecode`.

**Architecture:** A NEW `internal/filter/network/kafkabroker/` package implements BOTH `ReadFilter` (request-header decode in `OnData`) and `WriteFilter` (response-header decode in `OnWrite`; one instance per connection — consumer #3 of the ADR-0221 conn-wrap seam, the `mongoproxy` both-directions shape). Decoders run over a PRIVATE copy of the chain bytes (the chain `Buffer` is observational, never drained/mutated — the `mongoproxy` `chainConsumed` high-water pattern); the filter is a pure copying sniffer that ALWAYS returns `Continue` and NEVER mutates/closes the connection (R1). The request side registers `correlation_id → (api_key, api_version)` into a per-connection map; the response side (on the write goroutine) recovers + erases it, so the cross-goroutine map is guarded by an ADR-0223 mutex. ZERO framework changes: `internal/filter/network/` (chain.go / readconn.go / writeconn.go / types.go), `internal/listener/manager.go`, `tcp_proxy`, HCM are all untouched (§4). Cross-side `StatsAsserter` counter parity is the load-bearing differential proof (the filter never mutates bytes — a body differential is intrinsically vacuous).

**Tech Stack:** Go 1.26.2 / golangci-lint 1.64.8 (ADR-0009); reference Envoy **`envoyproxy/envoy:contrib-v1.37.2`** (ADR-0227, the contrib variant of v1.37.2). go-control-plane `/envoy` v1.32.4 (ADR-0008) + the NEW `/contrib` v1.32.4 (the project's first `/contrib` dep — proto bindings only; decode is in-house). Reuses `internal/filter/network/` (26.1/26.2/27/28.1a/28.1b — consumed, not modified), `internal/stats/` (06.1; `NewCounterIfAbsent` + `IsValidName`), the ADR-0223 per-connection mutex pattern, the differential harness + `fixture.StatsAsserter`. ONE new third-party `go.mod` dep (`github.com/envoyproxy/go-control-plane/contrib v1.32.4`); ZERO framework-seam extensions.

---

## ADR-0045 split-gate FINAL re-check (at PLAN time, per SPEC §3.0 / §11.8)

The gate fires at `> ~25 tasks OR > ~1500 net-new production LoC`. This PLAN decomposes to **17 tasks** / **~805–1200 production LoC** (the SPEC §3.0 envelope, re-confirmed at PLAN time on the 26.x/29.1 accounting basis — fixture drivers + unit tests excluded):

| Unit | Production LoC | Tasks |
|---|---|---|
| `config.go` (5-field parse + PGV arms + the oneof + nested-rule arms + the `stat_prefix` IsValidName guard) | ~120–180 | 3 |
| `apikeys.go` (the `api_key → (root, maxVersion)` table, 86 entries + the `flexibleVersions` predicate — DATA, not branching) | ~180–260 | 4–5 |
| `stats.go` (the EAGER 176-counter roster + eager-create helper + inc accessors) | ~80–130 | 6 |
| `codec.go` (the Kafka primitive decoder + 4-byte length framing + request-header decode + response-header decode + private-buffer reassembly + the unknown/failure classification) | ~250–350 | 7–8 |
| `filter.go` + `kafkabroker.go` + `doc.go` (glue + the correlation map + the ADR-0223 mutex + factory) | ~140–220 | 9 |
| builtins + `bootstrap.go` + the `/contrib` dep + the `name.go` `kafka.` INLINE arm | ~55–95 | 2, 10–11 |
| The 40th fuzzer | ~60 | 15 |
| **Total (production basis)** | **~805–1200** | **17** |

Both axes under the gate (17 ≤ ~25 tasks; ~1200 ≤ ~1500 LoC) → **NO split. Phase 31 proceeds as ONE flat §9 row.** The pre-authorized **31.1-request / 31.2-response** split axis (SPEC §3.0) stays UNCONSUMED (the mongo-29.1 "pre-authorized split stands unconsumed" precedent). The two static tables are the only piece materially heavier than a typical flat row, already folded into the upper bound above (DATA decoded with `encoding/binary`, far leaner than mongo's BSON parser; the response header is trivial; NO fault-delay/async-halt seam). The fixture drivers (`0053` ~600–900 LoC + `0054` ~200 LoC) are excluded per the 26.x/27/28.x/29.1 accounting precedent.

## PLAN-time D-question dispositions (SPEC §12)

- **D-P3 (boot-reject fixture arms) — RESOLVED at PLAN: `0054-kafka-boot-reject` carries the `stat_prefix`-required arm ONLY; the api_keys-range + nested-rule reject arms are UNIT-TEST-ONLY.** This mirrors the mongo `0050`/zookeeper boot-reject precedent (the load-bearing required-field arm is the fixture; the deferred-feature PGV arms exercise behavior-deferred fields and stay unit-test-only — config-parse faithfulness without a fixture-dir cost). The api_keys-range / oneof-typed-nil / nested-rule arms are exhaustively covered by `config_test.go` byte-stable reject tests (Task 3).
- **D-P6 (the api-key + flexibleVersions tables — hand-transcribed vs generated) — RESOLVED at PLAN: hand-transcribed static data (SPEC Appendices B/C), guarded by byte-stable golden tests.** envoy-go has no code-gen pipeline (no Jinja2/Python `generator.py` port — YAGNI). `apikeys.go` carries the tables as Go literals; `TestApiKeyRoster_MatchesUpstream` + `TestHeaderVersion` pin them byte-stable against the SPEC appendices (the mongo `TestStatRoster_MatchesUpstreamMacro` golden-test precedent).
- **D-PLAN-1 (NEW, surfaced at PLAN: the `unknown`-keyed-on-`(api_key, api_version)` routing requires a per-api-key version-range) — RESOLVED: extend the static api-key table to carry `maxVersion` (the highest api_version Kafka 3.9.1 defines for that key); the decoder routes `api_version > maxVersion(api_key)` → `*.unknown` (an unknown api_key → `*.unknown` directly).** Upstream's resolver returns a `SentinelParser` when `(api_key, api_version)` has no generated parser — i.e. an unknown key OR an unknown VERSION of a known key (AMEND-K4 / §11.4). A header-only decoder reproduces the version dimension only with a supported-version-range table. `minVersion` is treated as `0` for all keys (the deprecated-low-version → unknown sub-case is NOT exercised by `0053` and is recorded as a coverage boundary in BEHAVIOR_CONTRACT — header-reproducible but low-differential-value; the `0053` unknown-version arm tests the HIGH side with `api_version = 0x7FFF`). `maxVersion` is transcribed from the Kafka 3.9.1 message JSON alongside the message-name in Appendix B and pinned by the same `TestApiKeyRoster_MatchesUpstream` golden test (Task 4).
- **D-PLAN-2 (decode-error recovery posture — NOT mongo-style lifetime sniffing-off) — RESOLVED: per-exception `failure` + decoder RESET + Continue, NOT a lifetime sniffing-off.** Upstream increments `*.failure` + calls `decoder->reset()` and keeps decoding (`broker/filter.cc:78-96`; §11.4) — it does NOT disable the decoder for the connection lifetime (contrast mongo's AMEND-B6 lifetime sniffing-off). envoy-go mirrors: a decode exception increments `request.failure`/`response.failure`, RESETS that direction's private reassembly buffer (resync to a fresh frame boundary), and returns `Continue` — the connection keeps being sniffed. The correlation map is NOT cleared on a decode reset (it spans frames). This is a documented difference from mongo, called out in the codec task (Task 7/8) and BEHAVIOR_CONTRACT.
- **D-PLAN-3 (which header fields the decoder actually consumes) — RESOLVED: the request decoder reads + VALIDATES `api_key`/`api_version`/`correlation_id`/`client_id`/tagged-fields (client_id + tagged-fields are validated, not just skipped — that is how the `request.failure` malformed-`client_id` arm is reproduced); the response decoder reads `correlation_id` ONLY (count-and-frame-skip — it recovers `(api_key, api_version)` from the map and never needs the response tagged-fields for counting, since each frame is length-delimited).** The request side MUST attempt `client_id` (NULLABLE_STRING) + tagged-fields decode so a malformed length THROWS → `request.failure` (§7.3 / §11.4). Correlation registration happens after the `(api_key, api_version, correlation_id)` triple is read, BEFORE the `client_id` attempt — so a request that registers its correlation then fails on `client_id` still leaves the registration (the upstream `expectResponse`-on-`onFailedParse` parity, §11.3). The response side needs only `correlation_id` (INT32) to recover the spec + count; the AMEND-K5 `responseUsesTaggedFieldsInHeader` special-case (ApiVersions 18) is therefore IMMATERIAL to envoy-go's count-and-frame-skip response path and is recorded as such (the predicate + special-case are still implemented + table-tested at Task 5 for the request side + completeness, and as the documented response-side no-op).
- **Task ordering (green-compiling dependency order; SPEC §10 permits merge/reorder).** The SPEC §10 spine lists the decoder (Tasks 6–7) before the roster (Task 8); but the codec INCREMENTS roster counters + LOOKS UP names from the api-key table, so this PLAN orders `dep+skeleton (2) → config (3) → api-key table (4) → flexibleVersions (5) → stats roster (6) → request decoder + correlation map (7) → response decoder + correlation consume (8) → filter glue (9) → 9th built-in (10) → name.go arm (11) → BackendKind (12) → 0053 (13) → 0054 (14) → fuzzer (15) → differential re-verify (16) → completion (17)`. Each task compiles + unit-tests green standalone (the 29.1 `config → tables → stats → decoder → glue` order). The SPEC's task-to-deliverable mapping is preserved; only the linear order changes.

---

## File Structure

**Created:**
- `internal/filter/network/kafkabroker/doc.go` — package doc (the `kafka_broker` both-direction sniffer; ADR-0228; consumer #3 of the ADR-0221 WriteFilter seam; the project's first `/contrib` consumer).
- `internal/filter/network/kafkabroker/kafkabroker.go` — `TypeURL` (via `proto.MessageName(&kafka_brokerv3.KafkaBroker{})`, NEVER hand-typed).
- `internal/filter/network/kafkabroker/kafkabroker_test.go` — the TypeURL pinning test.
- `internal/filter/network/kafkabroker/config.go` — `compiledConfig` + `parseConfig` (the 5-field parse + the PGV-mirror reject constants + the `broker_address_rewrite_spec` oneof + the nested-rule arms + the `stats.IsValidName(stat_prefix)` config-boundary guard).
- `internal/filter/network/kafkabroker/config_test.go` — parse / defaults / every PGV-mirror reject-arm (byte-stable) / the IsValidName guard tests.
- `internal/filter/network/kafkabroker/apikeys.go` — the static `api_key → (root, maxVersion)` table (86 entries, SPEC Appendix B) + `apiKeyRoster()` + `apiKeyName(key)` / `apiKeyMaxVersion(key)` lookups + the `flexibleVersions` predicate (`requestUsesTaggedFieldsInHeader`/`responseUsesTaggedFieldsInHeader` with the ApiVersions(18) response special-case, SPEC Appendix C).
- `internal/filter/network/kafkabroker/apikeys_test.go` — `TestApiKeyRoster_MatchesUpstream` (the 86-name + maxVersion golden) + `TestHeaderVersion` (the flexibleVersions predicate + the ApiVersions(18) special-case).
- `internal/filter/network/kafkabroker/codec.go` — the Kafka primitive decoder (INT16/INT32/NULLABLE_STRING/UNSIGNED_VARINT tagged-fields) + the 4-byte length-prefix framing + the per-connection `decoder` (request-header decode + response-header decode + private-buffer reassembly + the unknown/failure classification + the correlation map).
- `internal/filter/network/kafkabroker/codec_test.go` — per-primitive decode / partial-frame reassembly / multi-read no-double-count / malformed-length throw / request-header decode (incl. flexible/tagged-field arms) / response-header decode / unknown-key / unknown-version / malformed-client_id failure / correlation register-recover-erase-miss / the `-race` concurrent test.
- `internal/filter/network/kafkabroker/stats.go` — the EAGER 176-counter roster (`apiKeyRoster()`-driven) + `newKafkaStats` eager creation + the `incRequest`/`incResponse`/`incRequestUnknown`/… accessors.
- `internal/filter/network/kafkabroker/stats_test.go` — `TestStatRoster` (the 176-counter roster present-at-0; the 4 fixed; the 86×2 per-key; idempotency).
- `internal/filter/network/kafkabroker/filter.go` — `NewFactory(reg *stats.Registry)` + the both-directions `filter` glue (OnNewConnection / OnData / OnWrite / SetReadFilterCallbacks / SetWriteFilterCallbacks / OnDestroy) — pure-copying-sniffer (always `Continue`, never mutate/close).
- `internal/filter/network/kafkabroker/filter_test.go` — factory / both-directions injection / OnData-feeds-request-decoder / OnWrite-feeds-response-decoder / R1 chain-buffer-never-drained / always-Continue / OnDestroy tests.
- `internal/filter/network/kafkabroker/fuzz_test.go` — `FuzzKafkaDecode` (the 40th fuzzer).
- `test/fixtures/0053-kafka-requests/driver/driver.go` + `README.md` — the cross-side multi-arm `StatsAsserter` fixture (request/unknown-key/unknown-version/failure/response/unregistered-correlation arms).
- `test/fixtures/0054-kafka-boot-reject/driver/driver.go` + `README.md` — the symmetric boot-reject fixture (missing `stat_prefix`).

**Modified:**
- `go.mod` / `go.sum` — add `github.com/envoyproxy/go-control-plane/contrib v1.32.4` (Task 2; held by the `bootstrap.go` blank-import through `go mod tidy`).
- `internal/bootstrap/bootstrap.go` — the `kafka_broker/v3` `/contrib` blank-import (after the `mongo_proxy/v3` import at `bootstrap.go:103`).
- `internal/stats/name.go` — the `kafka.` INLINE arm (in the `default` branch, after the `mongo.` arm at `name.go:278-289`, before the default error at `name.go:290`).
- `internal/stats/name_test.go` — the `kafka.` flattening tests (request/response/unknown/failure shapes → `envoy_kafka_<sp>_<rest>{}` with EMPTY labels; non-matching still errors).
- `internal/filter/network/builtins/builtins.go` — the 9th registration (after `mongoproxy` at `builtins.go:74`); package doc `eight` → `nine`.
- `internal/filter/network/builtins/builtins_test.go` — the all-nine registration test + the kafka registration test + boot smoke.
- `test/differential/fixture/fixture.go` — the new `TCPKafkaResponder BackendKind = 31` (after `TCPMongoResponder = 30` at `fixture.go:530`).
- `test/differential/runner_test.go` — the `TCPKafkaResponder` respond-loop + the `0053`/`0054` driver blank-imports.
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` / `DECISIONS.md` / `STATE.md` / `ROADMAP.md` / `next-prompt.txt` — the completion bundle (Task 17).

**Untouched (pinned — the §4 zero-touch property; a regression gate):** `internal/filter/network/` (chain.go / readconn.go / writeconn.go / prefixconn.go / types.go / callbacks.go / terminal.go / registry.go — pure consumer), `internal/listener/manager.go`, `internal/filter/tcpproxy/`, `internal/filter/hcm/`, `internal/accesslog/`, `internal/stats/prom.go` (the `kafka.` arm needs no label sort — empty labels), `internal/stats/registry.go`, `internal/stats/gauge.go` (kafka has no gauge), `internal/filter/network/mongoproxy/` + `zookeeperproxy/` (precedent only).

---

## Task 1: First-action baselines/anchors gate (no code change)

**Files:** none modified — verification + re-pin gate at the IMPL-session tip; record in `PROGRESS.md` (created this task at the worktree root).

- [ ] **Step 1: Re-confirm the project counts at the IMPL-session tip**

Run (from repo root):
```bash
git log --oneline -1
# fixtures (canonical recipe):
ls -d test/fixtures/[0-9]* | wc -l            # expect 54; tail dir:
ls -d test/fixtures/[0-9]* | tail -1          # expect test/fixtures/0052-mongo-fault-delay
# fuzzers (canonical recipe — scoped to ./internal):
grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l   # expect 39
# DECISIONS.md tail + next-free (heading entries only):
grep -nE "^## ADR-0[0-9]+" docs/envoy-go/DECISIONS.md | tail -3       # tail heading = ADR-0228 → next-free ADR-0229
# BackendKind tail:
grep -nE "TCP.*Responder BackendKind = [0-9]+" test/differential/fixture/fixture.go | tail -1   # expect TCPMongoResponder = 30
```
Expected: fixtures **54** (tail `0052-mongo-fault-delay`); fuzzers **39**; DECISIONS.md tail heading **ADR-0228** (the §Context anchored at the phase-31 SPEC squash `6104036`; next-free **ADR-0229**); BackendKind tail **30** (`TCPMongoResponder`). Phase 31 lands `0053`+`0054` → 56, the 40th fuzzer (`FuzzKafkaDecode`), the ADR-0228 §Decision/§Consequences body IN PLACE (no new ADR number consumed — `DECISIONS.md:14624`), and `TCPKafkaResponder = 31`.

- [ ] **Step 2: Re-confirm the stat surface = 360**

The count STATE.md / BEHAVIOR_CONTRACT.md report is **360** (the BEHAVIOR_CONTRACT stat-table row count; do NOT invent a new recipe). Expected: **360**. Phase 31 lands +176 → **536** at Task 17 (the 86 response-duration histograms are DEFERRED per ADR-0060 and NOT counted).

- [ ] **Step 3: Re-confirm `proto.MessageName` (the TypeURL pin) + that `/contrib v1.32.4` resolves**

```bash
mkdir -p /tmp/d31probe && cd /tmp/d31probe
go mod init d31probe >/dev/null 2>&1
go get github.com/envoyproxy/go-control-plane/contrib@v1.32.4
cat > main.go <<'EOF'
package main

import (
	"fmt"

	kbv3 "github.com/envoyproxy/go-control-plane/contrib/envoy/extensions/filters/network/kafka_broker/v3"
	"google.golang.org/protobuf/proto"
)

func main() { fmt.Println(proto.MessageName(&kbv3.KafkaBroker{})) }
EOF
go run main.go   # expect: envoy.extensions.filters.network.kafka_broker.v3.KafkaBroker
cd - >/dev/null
```
Expected `proto.MessageName` = `envoy.extensions.filters.network.kafka_broker.v3.KafkaBroker` (the `extensions.` segment per `reference_network_filter_typeurl_extensions`) → `TypeURL` = `type.googleapis.com/` + that. Confirm the 5-field accessor set + the oneof + nested-rule accessors against the resolved `/contrib v1.32.4` in-tree:
```bash
KB=$(go env GOMODCACHE)/github.com/envoyproxy/go-control-plane/contrib@v1.32.4/envoy/extensions/filters/network/kafka_broker/v3/kafka_broker.pb.go
grep -nE "func \(x \*KafkaBroker\) Get|KafkaBroker_IdBasedBrokerAddressRewriteSpec|func \(x \*IdBasedBrokerRewriteRule\) Get" $KB
# expect: GetStatPrefix() string / GetForceResponseRewrite() bool /
#         GetIdBasedBrokerAddressRewriteSpec() *IdBasedBrokerRewriteSpec /
#         GetApiKeysAllowed() []uint32 / GetApiKeysDenied() []uint32 ;
#         the KafkaBroker_IdBasedBrokerAddressRewriteSpec oneof wrapper ;
#         IdBasedBrokerRewriteRule Get{Id,Host,Port}
```
Record the exact Go identifiers (incl. the oneof-wrapper type name) for Tasks 2–3.

- [ ] **Step 4: Re-pin the as-built anchors against the IMPL-session tip**

```bash
grep -n "reg.Register(mongoproxy.TypeURL" internal/filter/network/builtins/builtins.go   # the 8th reg — insert AFTER
grep -n "built-in network filters" internal/filter/network/builtins/builtins.go          # doc "eight" → "nine"
grep -n "mongo_proxy/v3" internal/bootstrap/bootstrap.go                                   # blank-import — insert AFTER
grep -n "has no recognized top-level segment" internal/stats/name.go                       # the default error (the kafka. arm goes BEFORE it)
grep -n "CutPrefix(internal, \"mongo.\")" internal/stats/name.go                            # the mongo. arm — insert the kafka. arm AFTER
grep -n "TCPMongoResponder BackendKind = 30" test/differential/fixture/fixture.go          # BackendKind tail
```
Confirm: the 8th registration at `builtins.go:74`; the doc-count phrase at `builtins.go:1`; the mongo blank-import at `bootstrap.go:103`; the `name.go` default error at `name.go:290` + the mongo arm at `name.go:278`; the BackendKind tail at `fixture.go:530`. Record line numbers for Tasks 2/10/11/12 (re-grep at edit time — line numbers drift).

- [ ] **Step 5: Create `PROGRESS.md`** at the worktree root with the count baselines above + a per-task log section. Commit:

```bash
git add PROGRESS.md
git commit -m "phase 31 IMPL Task 1: first-action baselines gate (fixtures 54, fuzzers 39, stats 360, ADR tail 0228, BackendKind tail 30; kafka TypeURL + /contrib v1.32.4 + field roster pinned)"
```

---

## Task 2: The `/contrib v1.32.4` dep + the `kafka_broker/v3` blank-import + the package skeleton + TypeURL

**Files:**
- Modify: `go.mod`, `go.sum`, `internal/bootstrap/bootstrap.go`
- Create: `internal/filter/network/kafkabroker/doc.go`, `internal/filter/network/kafkabroker/kafkabroker.go`
- Test: `internal/filter/network/kafkabroker/kafkabroker_test.go`

- [ ] **Step 1: Write the failing TypeURL pinning test**

```go
// kafkabroker_test.go
package kafkabroker

import "testing"

func TestTypeURL(t *testing.T) {
	const want = "type.googleapis.com/envoy.extensions.filters.network.kafka_broker.v3.KafkaBroker"
	if TypeURL != want {
		t.Fatalf("TypeURL = %q, want %q", TypeURL, want)
	}
}
```

- [ ] **Step 2: Run it — expect a COMPILE failure** (`TypeURL` undefined; the package + dep do not exist yet).

Run: `go test ./internal/filter/network/kafkabroker/ -run TestTypeURL`
Expected: build error (undefined `TypeURL`).

- [ ] **Step 3: Add the dep + the blank-import + the skeleton**

```bash
go get github.com/envoyproxy/go-control-plane/contrib@v1.32.4
```

`internal/bootstrap/bootstrap.go` — add after the `mongo_proxy/v3` import (`bootstrap.go:103`), mirroring the comment shape:
```go
// Phase-31 registers the kafka_broker network-filter extension proto (the
// project's FIRST /contrib import) so protojson round-trips bootstraps carrying
// filter_chains[].filters[].typed_config of that type. Registered transitively
// by the kafkabroker filter package too; the explicit blank-import here
// guarantees resolution in any bootstrap-parsing context (e.g. the differential
// harness) AND holds the new contrib module dep through `go mod tidy`. Per
// ADR-0016 amendment policy, documented in PROGRESS, not a new ADR.
_ "github.com/envoyproxy/go-control-plane/contrib/envoy/extensions/filters/network/kafka_broker/v3"
```

`internal/filter/network/kafkabroker/doc.go`:
```go
// Package kafkabroker implements the envoy.filters.network.kafka_broker network
// filter (ADR-0228): a passive both-direction Kafka observability sniffer that
// decodes the request/response HEADER ONLY and emits full per-API-key
// request.<msg>_request / response.<msg>_response counter parity under
// kafka.<stat_prefix>. It is the 9th built-in network filter, consumer #3 of the
// ADR-0221 WriteFilter seam, and the project's first /contrib consumer. It NEVER
// mutates the byte stream (a pure copying sniffer; always Continue).
package kafkabroker
```

`internal/filter/network/kafkabroker/kafkabroker.go`:
```go
package kafkabroker

import (
	kafka_brokerv3 "github.com/envoyproxy/go-control-plane/contrib/envoy/extensions/filters/network/kafka_broker/v3"
	"google.golang.org/protobuf/proto"
)

// TypeURL is the config @type for the kafka_broker filter. Pinned by TestTypeURL;
// NEVER hand-typed. Carries the `extensions.` segment per
// reference_network_filter_typeurl_extensions.
var TypeURL = "type.googleapis.com/" + string(proto.MessageName(&kafka_brokerv3.KafkaBroker{}))
```

- [ ] **Step 4: Run the test + `go mod tidy` survival check**

Run:
```bash
go test ./internal/filter/network/kafkabroker/ -run TestTypeURL   # expect PASS
go mod tidy && git diff --exit-code go.mod | grep -q contrib || \
  grep -q "go-control-plane/contrib v1.32.4" go.mod   # expect: contrib still a direct require
```
Expected: PASS; `go.mod` retains `github.com/envoyproxy/go-control-plane/contrib v1.32.4` as a direct require after tidy (held by the bootstrap blank-import + the kafkabroker.go import).

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/kafkabroker/ internal/bootstrap/
golangci-lint run ./internal/filter/network/kafkabroker/... ./internal/bootstrap/...
go build ./...
git add go.mod go.sum internal/bootstrap/bootstrap.go internal/filter/network/kafkabroker/
git commit -m "phase 31 IMPL Task 2: /contrib v1.32.4 dep + kafka_broker/v3 blank-import + kafkabroker skeleton + TypeURL"
```

---

## Task 3: Config parse — the 5-field `KafkaBroker` + PGV arms + the `IsValidName` guard

**Files:**
- Create: `internal/filter/network/kafkabroker/config.go`, `internal/filter/network/kafkabroker/config_test.go`

The proto (SPEC §5.2): `StatPrefix string` (PGV-required min 1 rune → boot-reject), `ForceResponseRewrite bool` (unconstrained), `IdBasedBrokerAddressRewriteSpec *IdBasedBrokerRewriteSpec` (sole member of the `broker_address_rewrite_spec` oneof; nested `Rules []*IdBasedBrokerRewriteRule{ Id uint32; Host string (min 1 rune); Port uint32 (≤65535) }`), `ApiKeysAllowed []uint32` (each ∈ [0,32767]), `ApiKeysDenied []uint32` (each ∈ [0,32767]). All four active features PARSE-ACCEPT, behavior-DEFERRED (§2). Reject wording is envoy-go's OWN ADR-0080 byte-stable constants (every reject mirrors an upstream PGV failure — NO departure-class rejects).

- [ ] **Step 1: Write the failing config tests** (parse-accept defaults; the IsValidName guard; every reject arm byte-stable)

```go
// config_test.go (representative arms — the IMPL writes the full set)
func TestParseConfig_Defaults(t *testing.T) {
	cfg, err := parseConfig(&kafka_brokerv3.KafkaBroker{StatPrefix: "kprobe"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.statPrefix != "kprobe" {
		t.Fatalf("statPrefix = %q, want kprobe", cfg.statPrefix)
	}
}

func TestParseConfig_StatPrefixRequired(t *testing.T) {
	_, err := parseConfig(&kafka_brokerv3.KafkaBroker{})
	if err == nil || err.Error() != errStatPrefixRequired {
		t.Fatalf("err = %v, want %q", err, errStatPrefixRequired)
	}
}

func TestParseConfig_StatPrefixInvalidChars(t *testing.T) {
	_, err := parseConfig(&kafka_brokerv3.KafkaBroker{StatPrefix: "bad name"}) // space → IsValidName false
	if err == nil || err.Error() != errStatPrefixInvalid {
		t.Fatalf("err = %v, want %q", err, errStatPrefixInvalid)
	}
}

func TestParseConfig_ApiKeyOutOfRange(t *testing.T) {
	_, err := parseConfig(&kafka_brokerv3.KafkaBroker{StatPrefix: "k", ApiKeysAllowed: []uint32{32768}})
	if err == nil || err.Error() != errApiKeyOutOfRange {
		t.Fatalf("err = %v, want %q", err, errApiKeyOutOfRange)
	}
}

func TestParseConfig_RewriteRuleHostRequired(t *testing.T) {
	spec := &kafka_brokerv3.IdBasedBrokerRewriteSpec{
		Rules: []*kafka_brokerv3.IdBasedBrokerRewriteRule{{Id: 1, Port: 9092}}, // Host empty
	}
	_, err := parseConfig(&kafka_brokerv3.KafkaBroker{
		StatPrefix:                      "k",
		IdBasedBrokerAddressRewriteSpec: spec,
	})
	if err == nil || err.Error() != errRewriteRuleHostRequired {
		t.Fatalf("err = %v, want %q", err, errRewriteRuleHostRequired)
	}
}
// + TestParseConfig_RewriteRulePortTooLarge (port 65536) → errRewriteRulePortTooLarge
// + TestParseConfig_OneofTypedNil (a non-nil oneof wrapper holding a nil *IdBasedBrokerRewriteSpec) → errRewriteSpecNil
// + TestParseConfig_ForceResponseRewriteAccepted (ForceResponseRewrite:true parses OK)
```

- [ ] **Step 2: Run — expect COMPILE failure** (`parseConfig`/`compiledConfig`/the error constants undefined).

Run: `go test ./internal/filter/network/kafkabroker/ -run TestParseConfig`
Expected: build error.

- [ ] **Step 3: Write `config.go`**

```go
package kafkabroker

import (
	"errors"

	kafka_brokerv3 "github.com/envoyproxy/go-control-plane/contrib/envoy/extensions/filters/network/kafka_broker/v3"
	"github.com/<module>/internal/stats"
)

const (
	errStatPrefixRequired       = "kafka_broker: stat_prefix is required"
	errStatPrefixInvalid        = "kafka_broker: stat_prefix contains characters invalid for a stat name"
	errApiKeyOutOfRange         = "kafka_broker: api_keys_allowed/api_keys_denied values must be in [0, 32767]"
	errRewriteSpecNil           = "kafka_broker: broker_address_rewrite_spec oneof value cannot be a typed-nil"
	errRewriteRuleHostRequired  = "kafka_broker: id_based_broker_address_rewrite_spec: rule host is required"
	errRewriteRulePortTooLarge  = "kafka_broker: id_based_broker_address_rewrite_spec: rule port must be <= 65535"
)

const apiKeyMax = 32767 // int16 max — the api_keys_allowed/denied PGV upper bound (§5.2)

type compiledConfig struct {
	statPrefix string
	// The four active features are parsed-and-remembered for faithfulness but
	// their BEHAVIOR is deferred (§2): force_response_rewrite, the rewrite spec,
	// api_keys_allowed/denied. They drive NO runtime branch in phase 31.
	forceResponseRewrite bool
	apiKeysAllowed       []uint32
	apiKeysDenied        []uint32
	rewriteSpec          *kafka_brokerv3.IdBasedBrokerRewriteSpec
	stats                *kafkaStats // set by the factory (Task 9)
}

func parseConfig(msg *kafka_brokerv3.KafkaBroker) (*compiledConfig, error) {
	sp := msg.GetStatPrefix()
	if sp == "" {
		return nil, errors.New(errStatPrefixRequired)
	}
	if !stats.IsValidName(sp) { // config-boundary charset guard (AMEND-K7; the rbac/mongo precedent)
		return nil, errors.New(errStatPrefixInvalid)
	}
	cfg := &compiledConfig{
		statPrefix:           sp,
		forceResponseRewrite: msg.GetForceResponseRewrite(),
		apiKeysAllowed:       msg.GetApiKeysAllowed(),
		apiKeysDenied:        msg.GetApiKeysDenied(),
	}
	for _, k := range append(append([]uint32{}, cfg.apiKeysAllowed...), cfg.apiKeysDenied...) {
		if k > apiKeyMax {
			return nil, errors.New(errApiKeyOutOfRange)
		}
	}
	if spec := msg.GetIdBasedBrokerAddressRewriteSpec(); spec != nil || hasRewriteOneof(msg) {
		if spec == nil { // a set oneof wrapper holding a nil message — the typed-nil arm
			return nil, errors.New(errRewriteSpecNil)
		}
		for _, r := range spec.GetRules() {
			if r.GetHost() == "" {
				return nil, errors.New(errRewriteRuleHostRequired)
			}
			if r.GetPort() > 65535 {
				return nil, errors.New(errRewriteRulePortTooLarge)
			}
		}
		cfg.rewriteSpec = spec
	}
	return cfg, nil
}

// hasRewriteOneof reports whether the broker_address_rewrite_spec oneof is SET
// (even to a typed-nil) — distinguishing "field absent" from "oneof set to nil".
// Implement via the generated oneof wrapper type-switch (Task-1 Step 3 pinned the
// KafkaBroker_IdBasedBrokerAddressRewriteSpec wrapper name).
func hasRewriteOneof(msg *kafka_brokerv3.KafkaBroker) bool {
	_, ok := msg.GetBrokerAddressRewriteSpec().(*kafka_brokerv3.KafkaBroker_IdBasedBrokerAddressRewriteSpec)
	return ok
}
```
(Replace `<module>` with the real module path — grep `go.mod` head. Confirm the oneof getter name `GetBrokerAddressRewriteSpec` + the wrapper `KafkaBroker_IdBasedBrokerAddressRewriteSpec` against the Task-1 pin; adjust if the generated names differ.)

- [ ] **Step 4: Run the tests** — `go test ./internal/filter/network/kafkabroker/ -run TestParseConfig -v` — expect all PASS.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/kafkabroker/ && golangci-lint run ./internal/filter/network/kafkabroker/...
git add internal/filter/network/kafkabroker/config.go internal/filter/network/kafkabroker/config_test.go
git commit -m "phase 31 IMPL Task 3: kafka_broker 5-field config parse + PGV reject arms + stat_prefix IsValidName guard"
```

---

## Task 4: The static `api_key → (root, maxVersion)` table + the byte-stable golden test

**Files:**
- Create: `internal/filter/network/kafkabroker/apikeys.go` (the table portion)
- Create: `internal/filter/network/kafkabroker/apikeys_test.go` (the `TestApiKeyRoster_MatchesUpstream` portion)

Transcribe the 86-entry table from **SPEC Appendix B** (the `api_key → message-name-snake` map; api_keys 71/72 EXCLUDED). Extend each entry with `maxVersion` (D-PLAN-1; the highest api_version Kafka 3.9.1 defines for that key — transcribe from the Kafka 3.9.1 message JSON / upstream `kafka_request_resolver.cc` version ceilings). The roster (the 86 `root` strings, in api_key order) drives both the stat roster (Task 6) and the counter naming (`request.<root>_request` / `response.<root>_response`).

- [ ] **Step 1: Write the failing golden test**

```go
// apikeys_test.go
func TestApiKeyRoster_MatchesUpstream(t *testing.T) {
	// The 86 roster roots in api_key order (SPEC Appendix B; 71/72 excluded).
	want := []string{
		"produce", "fetch", "list_offsets", "metadata", "leader_and_isr",
		"stop_replica", "update_metadata", "controlled_shutdown", "offset_commit",
		"offset_fetch", "find_coordinator", "join_group", "heartbeat", "leave_group",
		"sync_group", "describe_groups", "list_groups", "sasl_handshake", "api_versions",
		// ... transcribe ALL 86 from SPEC Appendix B (keys 0..70, 73..87) ...
		"read_share_group_state_summary", // key 87 (last)
	}
	got := apiKeyRoster()
	if len(got) != 86 {
		t.Fatalf("roster len = %d, want 86", len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("roster[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestApiKeyName(t *testing.T) {
	if n, ok := apiKeyName(0); !ok || n != "produce" {
		t.Fatalf("apiKeyName(0) = %q,%v want produce,true", n, ok)
	}
	if n, ok := apiKeyName(18); !ok || n != "api_versions" {
		t.Fatalf("apiKeyName(18) = %q,%v want api_versions,true", n, ok)
	}
	if _, ok := apiKeyName(71); ok { // telemetry-excluded → not in the table
		t.Fatal("apiKeyName(71) should be absent (telemetry-excluded)")
	}
	if _, ok := apiKeyName(9999); ok {
		t.Fatal("apiKeyName(9999) should be absent (unknown key)")
	}
}

func TestApiKeyMaxVersion(t *testing.T) {
	// A spot-pin against SPEC Appendix B maxVersion column (IMPL fills exact values).
	if v, ok := apiKeyMaxVersion(18); !ok || v < 3 { // ApiVersions supports at least v3 in 3.9.1
		t.Fatalf("apiKeyMaxVersion(18) = %d,%v", v, ok)
	}
}
```

- [ ] **Step 2: Run — expect COMPILE failure** (`apiKeyRoster`/`apiKeyName`/`apiKeyMaxVersion` undefined).

- [ ] **Step 3: Write the table in `apikeys.go`**

```go
package kafkabroker

// apiKeyEntry is one row of the static Kafka 3.9.1 api-key table (SPEC Appendix
// B + the D-PLAN-1 maxVersion). `root` is name_in_c_case() of the message name
// (the stat segment is request.<root>_request / response.<root>_response).
// `maxVersion` is the highest api_version Kafka 3.9.1 defines; a request whose
// api_version exceeds it routes to *.unknown (D-PLAN-1). api_keys 71/72
// (telemetry) are EXCLUDED.
type apiKeyEntry struct {
	key        int16
	root       string
	maxVersion int16
}

// apiKeyTable is in api_key order with 71/72 omitted (86 entries). Transcribed
// from SPEC Appendix B; pinned by TestApiKeyRoster_MatchesUpstream.
var apiKeyTable = []apiKeyEntry{
	{0, "produce", 11},      // maxVersion values are illustrative — IMPL transcribes
	{1, "fetch", 17},        //   the exact Kafka 3.9.1 ceilings.
	{2, "list_offsets", 9},
	{3, "metadata", 12},
	// ... all 86 ...
	{18, "api_versions", 4},
	// ... keys 19..70, 73..87 ...
}

var apiKeyByKey = func() map[int16]apiKeyEntry {
	m := make(map[int16]apiKeyEntry, len(apiKeyTable))
	for _, e := range apiKeyTable {
		m[e.key] = e
	}
	return m
}()

func apiKeyRoster() []string {
	roots := make([]string, len(apiKeyTable))
	for i, e := range apiKeyTable {
		roots[i] = e.root
	}
	return roots
}

func apiKeyName(key int16) (string, bool) {
	e, ok := apiKeyByKey[key]
	return e.root, ok
}

func apiKeyMaxVersion(key int16) (int16, bool) {
	e, ok := apiKeyByKey[key]
	return e.maxVersion, ok
}
```

- [ ] **Step 4: Run** `go test ./internal/filter/network/kafkabroker/ -run 'TestApiKey'` — expect PASS once the full 86-entry table is transcribed.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/kafkabroker/ && golangci-lint run ./internal/filter/network/kafkabroker/...
git add internal/filter/network/kafkabroker/apikeys.go internal/filter/network/kafkabroker/apikeys_test.go
git commit -m "phase 31 IMPL Task 4: static api_key->(root,maxVersion) table (86 entries, Appendix B) + byte-stable golden test"
```

---

## Task 5: The `flexibleVersions(api_key)` predicate + the ApiVersions(18) response special-case

**Files:**
- Modify: `internal/filter/network/kafkabroker/apikeys.go` (add the predicate)
- Modify: `internal/filter/network/kafkabroker/apikeys_test.go` (add `TestHeaderVersion`)

A request/response header uses the v2/flexible (tagged-fields) form iff `api_version ∈ flexibleVersions(api_key)` — encoded as the lowest-flexible-version `N+` rule (SPEC Appendix C). `responseUsesTaggedFieldsInHeader` is identical EXCEPT api_key 18 (ApiVersions) → ALWAYS false (AMEND-K5). Per D-PLAN-3 the response special-case is immaterial to envoy-go's count-and-frame-skip response path; it is implemented + table-tested here for the request side + completeness.

- [ ] **Step 1: Write the failing `TestHeaderVersion`**

```go
func TestHeaderVersion(t *testing.T) {
	cases := []struct {
		key, ver       int16
		reqFlex, respFlex bool
	}{
		{0, 8, false, false},  // produce flexible at 9+ → v8 not flexible
		{0, 9, true, true},    // produce v9 flexible
		{18, 3, true, false},  // ApiVersions req v3 flexible; RESPONSE header suppressed (AMEND-K5)
		{18, 2, false, false}, // ApiVersions v2 below flex floor
		{17, 0, false, false}, // sasl_handshake never flexible
		{47, 0, false, false}, // offset_delete never flexible
		{60, 0, true, true},   // describe_cluster flexible at 0+
	}
	for _, c := range cases {
		if got := requestUsesTaggedFieldsInHeader(c.key, c.ver); got != c.reqFlex {
			t.Errorf("request(%d,%d) = %v, want %v", c.key, c.ver, got, c.reqFlex)
		}
		if got := responseUsesTaggedFieldsInHeader(c.key, c.ver); got != c.respFlex {
			t.Errorf("response(%d,%d) = %v, want %v", c.key, c.ver, got, c.respFlex)
		}
	}
}
```

- [ ] **Step 2: Run — expect COMPILE failure** (predicates undefined).

- [ ] **Step 3: Add the predicate to `apikeys.go`**

```go
// flexibleSince maps api_key → the lowest api_version that uses the flexible
// (tagged-fields) header form; a key absent from the map (or sentinel -1) is
// NEVER flexible (SPEC Appendix C; Kafka 3.9.1). 71/72 excluded.
var flexibleSince = map[int16]int16{
	0: 9, 1: 12, 2: 6, 3: 9, 4: 4, 5: 2, 6: 6, 7: 3, 8: 8, 9: 6, 10: 3,
	11: 6, 12: 4, 13: 4, 14: 4, 15: 5, 16: 3, /* 17 never */ 18: 3, 19: 5,
	20: 4, 21: 2, 22: 2, 23: 4, 24: 3, 25: 3, 26: 3, 27: 1, 28: 3, 29: 2,
	// ... transcribe all flexible keys from SPEC Appendix C; omit 17 and 47 ...
	60: 0, // describe_cluster
	// ... 61..70, 73..87 ...
}

func requestUsesTaggedFieldsInHeader(key, ver int16) bool {
	since, ok := flexibleSince[key]
	return ok && ver >= since
}

func responseUsesTaggedFieldsInHeader(key, ver int16) bool {
	if key == 18 { // ApiVersions: response header suppresses tagged fields (AMEND-K5)
		return false
	}
	return requestUsesTaggedFieldsInHeader(key, ver)
}
```

- [ ] **Step 4: Run** `go test ./internal/filter/network/kafkabroker/ -run TestHeaderVersion` — expect PASS once Appendix C is transcribed.

- [ ] **Step 5: gofmt + lint + commit**

```bash
git add internal/filter/network/kafkabroker/apikeys.go internal/filter/network/kafkabroker/apikeys_test.go
git commit -m "phase 31 IMPL Task 5: flexibleVersions predicate + ApiVersions(18) response special-case (Appendix C) + table test"
```

---

## Task 6: The EAGER 176-counter roster

**Files:**
- Create: `internal/filter/network/kafkabroker/stats.go`, `internal/filter/network/kafkabroker/stats_test.go`

The roster (AMEND-K3; §7.2): 86 `request.<root>_request` + 86 `response.<root>_response` + 4 fixed (`request.unknown`/`request.failure`/`response.unknown`/`response.failure`) = **176**, all created EAGERLY at config parse under `kafka.<stat_prefix>.` (the mongo `newMongoStats` precedent; `NewCounterIfAbsent` idempotent across listeners sharing a `stat_prefix`). The 86 response-duration histograms are DEFERRED (ADR-0060) — NOT created.

- [ ] **Step 1: Write the failing `TestStatRoster`**

```go
func TestStatRoster(t *testing.T) {
	reg := stats.NewRegistry()
	ks := newKafkaStats(reg, "kprobe")
	if got := len(ks.counters); got != 176 {
		t.Fatalf("roster size = %d, want 176", got)
	}
	// present-at-0 (eager): a representative per-key + each fixed counter.
	for _, suf := range []string{
		"request.produce_request", "response.metadata_response",
		"request.api_versions_request", "response.api_versions_response",
		"request.unknown", "request.failure", "response.unknown", "response.failure",
	} {
		c, ok := ks.counters[suf]
		if !ok {
			t.Errorf("counter %q absent from eager roster", suf)
			continue
		}
		if c.Load() != 0 {
			t.Errorf("counter %q = %d at creation, want 0", suf, c.Load())
		}
	}
	// the full internal name carries the kafka.<sp>. scope:
	if name := reg.NameOf(ks.counters["request.produce_request"]); name != "kafka.kprobe.request.produce_request" {
		t.Errorf("internal name = %q", name) // adapt to the project's name-introspection API
	}
}

func TestStatRoster_Idempotent(t *testing.T) {
	reg := stats.NewRegistry()
	_ = newKafkaStats(reg, "kprobe")
	_ = newKafkaStats(reg, "kprobe") // a second listener sharing the prefix — no panic, same counters
}
```
(Adapt the internal-name introspection to whatever the `stats.Registry` exposes — mirror the mongo `stats_test.go`; if there is no `NameOf`, drop that sub-assertion and rely on the `name.go` arm test at Task 11.)

- [ ] **Step 2: Run — expect COMPILE failure** (`newKafkaStats`/`kafkaStats` undefined).

- [ ] **Step 3: Write `stats.go`**

```go
package kafkabroker

import (
	"fmt"

	"github.com/<module>/internal/stats"
)

type kafkaStats struct {
	prefix   string
	counters map[string]*stats.Counter // keyed by the suffix after kafka.<sp>.
}

var fixedSuffixes = []string{"request.unknown", "request.failure", "response.unknown", "response.failure"}

func newKafkaStats(reg *stats.Registry, statPrefix string) *kafkaStats {
	ks := &kafkaStats{
		prefix:   "kafka." + statPrefix + ".",
		counters: make(map[string]*stats.Counter, 176),
	}
	for _, root := range apiKeyRoster() {
		reqSuf := "request." + root + "_request"
		respSuf := "response." + root + "_response"
		ks.counters[reqSuf] = reg.NewCounterIfAbsent(ks.prefix + reqSuf)
		ks.counters[respSuf] = reg.NewCounterIfAbsent(ks.prefix + respSuf)
	}
	for _, suf := range fixedSuffixes {
		ks.counters[suf] = reg.NewCounterIfAbsent(ks.prefix + suf)
	}
	return ks
}

func (ks *kafkaStats) inc(suffix string) {
	c, ok := ks.counters[suffix]
	if !ok {
		panic(fmt.Sprintf("kafkabroker: unknown roster suffix %q", suffix))
	}
	c.Inc()
}

func (ks *kafkaStats) incRequest(root string)  { ks.inc("request." + root + "_request") }
func (ks *kafkaStats) incResponse(root string) { ks.inc("response." + root + "_response") }
func (ks *kafkaStats) incRequestUnknown()      { ks.inc("request.unknown") }
func (ks *kafkaStats) incRequestFailure()      { ks.inc("request.failure") }
func (ks *kafkaStats) incResponseUnknown()     { ks.inc("response.unknown") }
func (ks *kafkaStats) incResponseFailure()     { ks.inc("response.failure") }
```

- [ ] **Step 4: Run** `go test ./internal/filter/network/kafkabroker/ -run TestStatRoster` — expect PASS.

- [ ] **Step 5: gofmt + lint + commit**

```bash
git add internal/filter/network/kafkabroker/stats.go internal/filter/network/kafkabroker/stats_test.go
git commit -m "phase 31 IMPL Task 6: eager 176-counter roster (86x2 per-key + 4 fixed) under kafka.<stat_prefix>."
```

---

## Task 7: The Kafka primitive decoder + 4-byte framing + the request-header decoder + the correlation map

**Files:**
- Create: `internal/filter/network/kafkabroker/codec.go` (primitives + framing + the request path + the correlation map)
- Create: `internal/filter/network/kafkabroker/codec_test.go`

Primitives (`encoding/binary` big-endian — Kafka wire is BE): `INT16`, `INT32`, `NULLABLE_STRING` (INT16 length; -1 = null; a length < -1 or exceeding remaining bytes → THROW), `UNSIGNED_VARINT` (for tagged-fields count + each tagged field's tag/size — skip the tagged-field bytes). Framing: a 4-byte INT32 length prefix; partial frames WAIT for more (never an error). Request-header decode (in wire order, §11.3): `api_key` INT16 → `api_version` INT16 → `correlation_id` INT32 → **register `correlation_id → (api_key, api_version)`** → `client_id` NULLABLE_STRING (validate) → tagged-fields iff `requestUsesTaggedFieldsInHeader(api_key, api_version)` (validate). Classification: a decode exception → `request.failure` + reset (D-PLAN-2); else `api_key` unknown OR `api_version > maxVersion(api_key)` → `request.unknown`; else `request.<root>_request`. Per D-PLAN-2 a decode exception resets the direction's private buffer + Continue (NOT lifetime sniffing-off).

- [ ] **Step 1: Write the failing codec tests** (primitives + framing + request classification + correlation)

```go
// codec_test.go (representative — IMPL writes the full set)
func TestDecodeRequestHeader_KnownKey(t *testing.T) {
	reg := stats.NewRegistry()
	ks := newKafkaStats(reg, "k")
	d := newDecoder(ks)
	// ApiVersions(18) v0 (non-flexible): api_key=18, api_version=0, corr=7, client_id="c"
	frame := buildRequest(18, 0, 7, "c", nil)
	d.decodeOnData(frame, int64(len(frame)))
	if ks.counters["request.api_versions_request"].Load() != 1 {
		t.Fatal("api_versions_request not counted")
	}
	if spec, ok := d.lookupAndErase(7); !ok || spec.apiKey != 18 || spec.apiVersion != 0 {
		t.Fatalf("correlation not registered: %+v %v", spec, ok)
	}
}

func TestDecodeRequestHeader_UnknownKey(t *testing.T) { /* api_key 9999 → request.unknown +1 */ }
func TestDecodeRequestHeader_UnknownVersion(t *testing.T) { /* key 18 ver 0x7FFF → request.unknown +1 */ }
func TestDecodeRequestHeader_MalformedClientID(t *testing.T) { /* client_id length -5 → request.failure +1 */ }
func TestFraming_PartialThenComplete(t *testing.T) { /* feed first 3 bytes then the rest — counts once, no error */ }
func TestFraming_MultiReadNoDoubleCount(t *testing.T) { /* cumulative buffer + TotalAppended high-water — counts once */ }
func TestPrimitives_NullableStringNull(t *testing.T) { /* length -1 → nil, no throw */ }
```

- [ ] **Step 2: Run — expect COMPILE failure** (`newDecoder`/`decodeOnData`/`lookupAndErase`/`buildRequest` undefined).

- [ ] **Step 3: Write `codec.go`** (the request path + primitives + framing + the correlation map)

Key shapes (the IMPL fleshes out the primitive reader + the high-water reassembly, mirroring `mongoproxy/codec.go`):
```go
type respSpec struct{ apiKey, apiVersion int16 }

type decoder struct {
	stats         *kafkaStats
	chainConsumed int64  // high-water vs Buffer.TotalAppended() (D-S29.1-4 analog)
	readBuf       []byte // private request reassembly buffer
	writeBuf      []byte // private response reassembly buffer (Task 8)

	mu       sync.Mutex            // guards EXACTLY `expected` (ADR-0223)
	expected map[int32]respSpec    // correlation_id -> (api_key, api_version)
}

func newDecoder(ks *kafkaStats) *decoder {
	return &decoder{stats: ks, expected: make(map[int32]respSpec)}
}

// errKafkaDecode is the internal throw type; a decode helper returns it to signal
// a *.failure (D-PLAN-2). Truncation (need more bytes) is a DISTINCT sentinel
// (errShortBuffer) that means "wait", NOT failure.
var errShortBuffer = errors.New("short")

func (d *decoder) register(corr int32, key, ver int16) {
	d.mu.Lock()
	d.expected[corr] = respSpec{key, ver}
	d.mu.Unlock()
}

func (d *decoder) lookupAndErase(corr int32) (respSpec, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.expected[corr]
	if ok {
		delete(d.expected, corr)
	}
	return s, ok
}

// decodeOnData appends only the never-before-seen trailing bytes (high-water),
// then drains complete frames. Each complete frame decodes ONE request header.
func (d *decoder) decodeOnData(chain []byte, totalAppended int64) {
	d.readBuf = append(d.readBuf, newBytes(chain, totalAppended, &d.chainConsumed)...)
	for {
		frame, ok := d.nextFrame(&d.readBuf) // peels 4-byte length + body; ok=false → wait
		if !ok {
			return
		}
		d.decodeRequestFrame(frame)
	}
}

func (d *decoder) decodeRequestFrame(frame []byte) {
	r := newReader(frame)
	key, err := r.int16()
	if err != nil { d.stats.incRequestFailure(); return }
	ver, err := r.int16()
	if err != nil { d.stats.incRequestFailure(); return }
	corr, err := r.int32()
	if err != nil { d.stats.incRequestFailure(); return }
	d.register(corr, key, ver) // register BEFORE client_id (the onFailedParse parity)
	// validate client_id + tagged-fields so a malformed length → request.failure:
	if _, err := r.nullableString(); err != nil { d.stats.incRequestFailure(); return }
	if requestUsesTaggedFieldsInHeader(key, ver) {
		if err := r.skipTaggedFields(); err != nil { d.stats.incRequestFailure(); return }
	}
	// classify:
	root, ok := apiKeyName(key)
	if maxV, okv := apiKeyMaxVersion(key); !ok || !okv || ver > maxV {
		d.stats.incRequestUnknown()
		return
	}
	d.stats.incRequest(root)
}
```
The primitive `reader` (BE int16/int32/nullableString/unsignedVarint/skipTaggedFields) + `nextFrame` (4-byte length peel; truncation → wait) + `newBytes` (high-water trailing slice) are written + unit-tested here. On a malformed frame LENGTH (e.g. negative), reset `readBuf` to empty (resync) rather than spin — but a length simply larger than the buffer means WAIT (not failure), so distinguish carefully (the mongo `nextMessage` precedent).

- [ ] **Step 4: Run** `go test ./internal/filter/network/kafkabroker/ -run 'TestDecodeRequest|TestFraming|TestPrimitives' -v` — expect PASS.

- [ ] **Step 5: gofmt + lint + commit**

```bash
git add internal/filter/network/kafkabroker/codec.go internal/filter/network/kafkabroker/codec_test.go
git commit -m "phase 31 IMPL Task 7: Kafka primitive decoder + 4-byte framing + request-header decode + correlation map registration"
```

---

## Task 8: The response-header decoder + correlation recover/erase + the `-race` test

**Files:**
- Modify: `internal/filter/network/kafkabroker/codec.go` (add the response path)
- Modify: `internal/filter/network/kafkabroker/codec_test.go` (add response + `-race` tests)

Response-header decode (in `OnWrite`, on the WRITE goroutine; §11.3): read the 4-byte length prefix → `correlation_id` INT32 → recover `(api_key, api_version)` from the map (`lookupAndErase`) → MISS → `response.failure` (the unregistered-correlation arm, AMEND-K4); HIT → classify by the recovered key: unknown key/version → `response.unknown`; else `response.<root>_response`. Per D-PLAN-3 the response side needs ONLY `correlation_id` to count + the frame length to skip — it does NOT parse the response tagged-fields. A response decode exception (e.g. a truncated correlation_id that is NOT just "wait") → `response.failure` + reset.

- [ ] **Step 1: Write the failing response + concurrency tests**

```go
func TestDecodeResponse_Correlated(t *testing.T) {
	reg := stats.NewRegistry(); ks := newKafkaStats(reg, "k"); d := newDecoder(ks)
	req := buildRequest(3, 9, 42, "c", nil) // metadata(3) v9
	d.decodeOnData(req, int64(len(req)))
	resp := buildResponse(42, true) // correlation_id=42, flexible(metadata v9) → tagged-fields present
	d.decodeOnWrite(resp)
	if ks.counters["response.metadata_response"].Load() != 1 {
		t.Fatal("metadata_response not counted")
	}
}

func TestDecodeResponse_Unregistered(t *testing.T) {
	reg := stats.NewRegistry(); ks := newKafkaStats(reg, "k"); d := newDecoder(ks)
	resp := buildResponse(999, false) // never registered → response.failure
	d.decodeOnWrite(resp)
	if ks.counters["response.failure"].Load() != 1 {
		t.Fatal("response.failure not counted for unregistered correlation_id")
	}
}

func TestCorrelation_Race(t *testing.T) {
	// Run with -race: a request goroutine registering + a response goroutine
	// recovering on the SAME decoder, asserting no data race on `expected`.
	reg := stats.NewRegistry(); ks := newKafkaStats(reg, "k"); d := newDecoder(ks)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		corr := int32(i)
		wg.Add(2)
		go func() { defer wg.Done(); d.register(corr, 3, 9) }()
		go func() { defer wg.Done(); d.lookupAndErase(corr) }()
	}
	wg.Wait()
}
```

- [ ] **Step 2: Run — expect FAIL** (`decodeOnWrite`/`buildResponse` undefined; the response counters stay 0).

- [ ] **Step 3: Add the response path to `codec.go`**

```go
func (d *decoder) decodeOnWrite(chain []byte) {
	d.writeBuf = append(d.writeBuf, chain...) // response side has no high-water (28.2 asymmetry)
	for {
		frame, ok := d.nextFrame(&d.writeBuf)
		if !ok {
			return
		}
		d.decodeResponseFrame(frame)
	}
}

func (d *decoder) decodeResponseFrame(frame []byte) {
	r := newReader(frame)
	corr, err := r.int32()
	if err != nil { d.stats.incResponseFailure(); return }
	spec, ok := d.lookupAndErase(corr)
	if !ok { // unregistered correlation_id → response.failure (AMEND-K4 / getResponseSpec throw)
		d.stats.incResponseFailure()
		return
	}
	root, ok := apiKeyName(spec.apiKey)
	if maxV, okv := apiKeyMaxVersion(spec.apiKey); !ok || !okv || spec.apiVersion > maxV {
		d.stats.incResponseUnknown()
		return
	}
	d.stats.incResponse(root)
}
```

- [ ] **Step 4: Run** `go test ./internal/filter/network/kafkabroker/ -run 'TestDecodeResponse|TestCorrelation' -race -v` — expect PASS (incl. `-race` clean).

- [ ] **Step 5: gofmt + lint + commit**

```bash
git add internal/filter/network/kafkabroker/codec.go internal/filter/network/kafkabroker/codec_test.go
git commit -m "phase 31 IMPL Task 8: response-header decode + correlation recover/erase + unregistered->response.failure + -race test"
```

---

## Task 9: The ReadFilter/WriteFilter glue + the factory (pure copying sniffer)

**Files:**
- Create: `internal/filter/network/kafkabroker/filter.go`, `internal/filter/network/kafkabroker/filter_test.go`

The `filter` implements BOTH `network.ReadFilter` (OnNewConnection / OnData / SetReadFilterCallbacks) and `network.WriteFilter` (OnWrite / SetWriteFilterCallbacks) + OnDestroy. It is a pure copying sniffer: OnData feeds the request decoder, OnWrite feeds the response decoder, BOTH always return `network.Continue`, and it NEVER drains/mutates the chain buffer or closes the connection (R1). The filter implements `WriteFilter` even though it never mutates the write buffer — to qualify the chain for the 28.1b post-handoff read seam (`reference_network_chain_terminal_handoff_ends_ondata`). `NewFactory` mirrors `mongoproxy.NewFactory`: parse config → eager-create stats → return the per-connection filter constructor.

- [ ] **Step 1: Write the failing filter tests**

```go
func TestFactory_BothDirections(t *testing.T) {
	reg := stats.NewRegistry()
	mk := NewFactory(reg)
	tc, _ := anypb.New(&kafka_brokerv3.KafkaBroker{StatPrefix: "k"})
	inst, err := mk(tc, nil)
	if err != nil { t.Fatal(err) }
	f := inst().(interface{ network.ReadFilter; network.WriteFilter })
	_ = f // both interfaces satisfied (compile-time assertion)
}

func TestOnData_FeedsDecoder_NeverDrains(t *testing.T) {
	// build a filter, push a request frame through OnData, assert the request
	// counter incremented AND the chain Buffer was not drained (R1) AND Continue.
}

func TestOnWrite_FeedsResponseDecoder_AlwaysContinue(t *testing.T) { /* ... */ }
```

- [ ] **Step 2: Run — expect COMPILE failure** (`NewFactory`/`filter` undefined).

- [ ] **Step 3: Write `filter.go`**

```go
package kafkabroker

import (
	"fmt"

	"github.com/<module>/internal/filter/network"
	"github.com/<module>/internal/stats"
	"google.golang.org/protobuf/types/known/anypb"

	kafka_brokerv3 "github.com/envoyproxy/go-control-plane/contrib/envoy/extensions/filters/network/kafka_broker/v3"
)

func NewFactory(reg *stats.Registry) network.NetworkFilterFactory {
	return func(tc *anypb.Any, _ network.FactoryCtx) (network.FilterInstanceFactory, error) {
		var msg kafka_brokerv3.KafkaBroker
		if tc != nil && len(tc.GetValue()) > 0 {
			if err := tc.UnmarshalTo(&msg); err != nil {
				return nil, fmt.Errorf("kafka_broker: invalid typed_config: %w", err)
			}
		}
		cfg, err := parseConfig(&msg)
		if err != nil {
			return nil, err
		}
		cfg.stats = newKafkaStats(reg, cfg.statPrefix) // EAGER roster (present-at-0 from boot)
		return func() network.NetworkFilter {
			return &filter{cfg: cfg, dec: newDecoder(cfg.stats)}
		}, nil
	}
}

type filter struct {
	cfg *compiledConfig
	dec *decoder
}

func (f *filter) OnNewConnection() network.Status { return network.Continue }

func (f *filter) OnData(buf *network.Buffer, _ bool) network.Status {
	f.dec.decodeOnData(buf.Bytes(), buf.TotalAppended()) // observational; never drains
	return network.Continue
}

func (f *filter) OnWrite(buf *network.Buffer, _ bool) network.Status {
	f.dec.decodeOnWrite(buf.Bytes())
	return network.Continue
}

// SetReadFilterCallbacks / SetWriteFilterCallbacks / OnDestroy — mirror
// mongoproxy/filter.go (no callbacks needed; the sniffer never closes/drains).
```
(Confirm `network.NetworkFilterFactory` / `FilterInstanceFactory` / `NetworkFilter` / `Buffer.Bytes()` / `Buffer.TotalAppended()` signatures against `mongoproxy/filter.go` — copy the exact interface set incl. the `SetReadFilterCallbacks`/`SetWriteFilterCallbacks`/`OnDestroy` methods.)

- [ ] **Step 4: Run** `go test ./internal/filter/network/kafkabroker/ -v` — expect ALL PASS.

- [ ] **Step 5: gofmt + lint + commit**

```bash
git add internal/filter/network/kafkabroker/filter.go internal/filter/network/kafkabroker/filter_test.go
git commit -m "phase 31 IMPL Task 9: ReadFilter/WriteFilter glue + factory (pure copying sniffer; always Continue, never mutate/close)"
```

---

## Task 10: Registration as the 9th built-in + boot smoke

**Files:**
- Modify: `internal/filter/network/builtins/builtins.go` (the 9th registration + doc `eight` → `nine`)
- Modify: `internal/filter/network/builtins/builtins_test.go`
- Modify: `cmd/envoy-go/main.go` IF it lists the filters explicitly (grep first — ADR-0072 makes order behavior-neutral)

- [ ] **Step 1: Write the failing registration test**

```go
func TestRegisterBuiltins_IncludesKafkaBroker(t *testing.T) {
	reg := network.NewRegistry()
	RegisterBuiltins(reg, Deps{StatsRegistry: stats.NewRegistry()})
	if _, ok := reg.Lookup(kafkabroker.TypeURL); !ok {
		t.Fatal("kafka_broker not registered")
	}
}
// + update the all-N registration count test from 8 → 9.
```

- [ ] **Step 2: Run — expect FAIL** (kafka_broker not registered).

- [ ] **Step 3: Add the registration** in `builtins.go` after the `mongoproxy` line (`builtins.go:74`):
```go
// kafka_broker: the 9th built-in (31; ADR-0228; the first /contrib consumer).
reg.Register(kafkabroker.TypeURL, kafkabroker.NewFactory(deps.StatsRegistry))
```
Update the package doc `eight` → `nine` (`builtins.go:1`). Add the `kafkabroker` import. Grep `cmd/envoy-go/main.go` for an explicit filter list; if present, mirror.

- [ ] **Step 4: Run** `go test ./internal/filter/network/builtins/... -v && go build ./...` — expect PASS.

- [ ] **Step 5: gofmt + lint + commit**

```bash
git add internal/filter/network/builtins/ cmd/envoy-go/main.go
git commit -m "phase 31 IMPL Task 10: register kafka_broker as the 9th built-in network filter + boot smoke"
```

---

## Task 11: The `kafka.` INLINE Prometheus arm in `internal/stats/name.go`

**Files:**
- Modify: `internal/stats/name.go` (the `kafka.` arm — after the `mongo.` arm at `name.go:289`, before the default error at `name.go:290`)
- Modify: `internal/stats/name_test.go`

The arm (AMEND-K2; §7.4) is the simplest INLINE shape — `kafka.<sp>.<rest>` → `envoy_kafka_<sp>_<rest flattened>` with EMPTY labels (NO label hoist; the zookeeper INLINE no-label style, generalized to a `kafka.` ROOT prefix). This REFUTES the BRAINSTORM label-hoist hypothesis.

- [ ] **Step 1: Write the failing `name.go` tests**

```go
func TestFlatten_Kafka(t *testing.T) {
	cases := []struct{ in, base string }{
		{"kafka.kprobe.request.api_versions_request", "envoy_kafka_kprobe_request_api_versions_request"},
		{"kafka.kprobe.request.unknown", "envoy_kafka_kprobe_request_unknown"},
		{"kafka.kprobe.response.metadata_response", "envoy_kafka_kprobe_response_metadata_response"},
		{"kafka.kprobe.response.failure", "envoy_kafka_kprobe_response_failure"},
	}
	for _, c := range cases {
		base, labels, err := flattenName(c.in) // adapt to the real fn name
		if err != nil {
			t.Errorf("%q: unexpected err %v", c.in, err)
			continue
		}
		if base != c.base || len(labels) != 0 {
			t.Errorf("%q -> base=%q labels=%v, want base=%q empty", c.in, base, labels, c.base)
		}
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (the default error branch rejects `kafka.*`).

- [ ] **Step 3: Insert the arm** before `name.go:290` (the default error), after the `mongo.` arm:
```go
// Phase-31 kafka_broker INLINE-PREFIX detection (ADR-0228; AMEND-K2; the
// zookeeper .zookeeper. no-label INLINE precedent generalized to a kafka. ROOT
// prefix). Internal name kafka.<sp>.<rest> → envoy_kafka_<sp>_<rest flattened>
// with EMPTY labels (live-probed: prefix + direction + api-key are ALL inlined
// into the metric NAME; NO tag extraction — the OPPOSITE of the mongo label
// hoist). Needs no shape-validation guard (no dynamic VALUE is hoisted to a
// label; the whole name flattens). KEEP-IN-SYNC:
// internal/filter/network/kafkabroker/stats.go.
if rest, ok := strings.CutPrefix(internal, "kafka."); ok {
	base = "envoy_kafka_" + strings.ReplaceAll(rest, ".", "_")
	return base, labels, nil // labels stays empty
}
```

- [ ] **Step 4: Run** `go test ./internal/stats/ -run TestFlatten_Kafka -v` — expect PASS. Also run the full `./internal/stats/...` suite to confirm no existing golden regressed.

- [ ] **Step 5: gofmt + lint + commit**

```bash
git add internal/stats/name.go internal/stats/name_test.go
git commit -m "phase 31 IMPL Task 11: kafka. INLINE Prometheus arm (envoy_kafka_<sp>_<rest>{} empty labels; no hoist)"
```

---

## Task 12: The new BackendKind — the correlation-id-echoing TCP Kafka responder

**Files:**
- Modify: `test/differential/fixture/fixture.go` (the `TCPKafkaResponder BackendKind = 31`)
- Modify: `test/differential/runner_test.go` (the respond-loop)

A NEW BackendKind (anticipated value **31**; re-pin against the `fixture.go` tail at edit time). The responder reads each request frame's `correlation_id` (INT32 at offset 8 after the 4-byte length + INT16 api_key + INT16 api_version) and echoes it in a canned response frame (4-byte length + INT32 correlation_id + a minimal trailing body sufficient for the reference's response decoder to count). The exact response-frame shape — incl. whether a flexible response needs response-header tagged-fields for the REFERENCE to decode — is pinned at this task via a live re-probe against `contrib-v1.37.2` (D-P4). Contrast the silent `TCPSink` (28) / the correlated `TCPMongoResponder` (30).

- [ ] **Step 1: Add the enum value** in `fixture.go` after `TCPMongoResponder = 30` (`fixture.go:530`), with the documenting comment (mirror the `TCPMongoResponder` block):
```go
// TCPKafkaResponder is a Kafka-aware canned-response TCP backend (31 SPEC §8.3):
// for every complete request frame (4-byte BE INT32 length prefix) it reads the
// correlation_id (INT32 at offset 8 — after api_key INT16 + api_version INT16)
// and echoes it in a canned response frame (4-byte length + INT32 correlation_id
// + a minimal body) so the subject's + reference's response-side correlation has
// something to correlate against. An UNREGISTERED-correlation trigger (a marker
// correlation_id) makes the responder emit a response whose correlation_id was
// never sent as a request → response.failure on both sides. NEW BackendKind per
// reference_differential_fixture_dispatch_constraint; the TCPMongoResponder = 30
// precedent.
TCPKafkaResponder BackendKind = 31
```

- [ ] **Step 2: Write the respond-loop** in `runner_test.go` (mirror `mongoRespondLoop` at `runner_test.go:2087`):
```go
func kafkaRespondLoop(c net.Conn) {
	defer func() { _ = c.Close() }()
	for {
		var lenp [4]byte
		if _, err := io.ReadFull(c, lenp[:]); err != nil { return }
		n := int(binary.BigEndian.Uint32(lenp[:]))
		if n < 8 || n > 1<<20 { return }
		body := make([]byte, n)
		if _, err := io.ReadFull(c, body); err != nil { return }
		corr := binary.BigEndian.Uint32(body[4:8]) // after api_key(2)+api_version(2)
		// echo a minimal response: length-prefixed INT32 correlation_id (+ optional
		// minimal body; pinned at the D-P4 re-probe).
		resp := make([]byte, 4)
		binary.BigEndian.PutUint32(resp, corr)
		out := make([]byte, 4+len(resp))
		binary.BigEndian.PutUint32(out[:4], uint32(len(resp)))
		copy(out[4:], resp)
		_, _ = c.Write(out)
	}
}
```
Wire `TCPKafkaResponder` into the backend dispatch switch (the existing `TCPMongoResponder` case site).

- [ ] **Step 3: Run** `go build ./... && go vet ./test/...` — expect clean (the loop is exercised by the `0053` fixture in Task 13).

- [ ] **Step 4: gofmt + lint + commit**

```bash
git add test/differential/fixture/fixture.go test/differential/runner_test.go
git commit -m "phase 31 IMPL Task 12: TCPKafkaResponder BackendKind (31) — correlation-id-echoing Kafka responder"
```

---

## Task 13: `0053-kafka-requests` cross-side fixture + driver

**Files:**
- Create: `test/fixtures/0053-kafka-requests/driver/driver.go`, `test/fixtures/0053-kafka-requests/README.md`
- Modify: `test/differential/runner_test.go` (the `0053` driver blank-import)

Chain `[kafka_broker (stat_prefix: …), tcp_proxy]` on BOTH sides; backend `TCPKafkaResponder` (Task 12). The driver sends hand-crafted Kafka wire frames (the §11.3 layouts) and asserts cross-side counter parity via `fixture.StatsAsserter` (cross-side MUST use `StatsAsserter` per `reference_differential_asserter_dispatch`; `SubjectAsserter` would be a dead vacuous assertion). Drive ONE request per connection (or carefully sequence frames — the §11.2/D-P5 caveat: a hand-rolled multi-frame stream tripped the reference decoder after frame 1; the driver pins the exact per-connection framing). Verify decode actually ran (`tcp.<sp>.downstream_cx_rx_bytes_total > 0` and/or non-zero kafka counters) — if tcp_proxy cannot establish its upstream the reference closes the downstream before the kafka decoder runs.

Arms (mirror the `0051-mongo-responses` 6-arm `StatsAsserter` shape):
1. **request per-key** — a non-flexible header (e.g. Metadata key 3 at a non-flexible version) + a flexible/tagged-field header (e.g. ApiVersions key 18 or Produce key 0 at a flexible version) → the matching `request.<root>_request` +1 both sides (the flexible arm proves tagged-field framing decode).
2. **request unknown-key** — api_key 9999 → `request.unknown` +1 both sides.
3. **request unknown-version** — a known key (e.g. 18) at `api_version=0x7FFF` → `request.unknown` +1 both sides (D-PLAN-1).
4. **request failure** — a malformed `client_id` NULLABLE_STRING length → `request.failure` +1 both sides.
5. **response per-key** — the responder echoes a response whose `correlation_id` matches a prior request → `response.<root>_response` +1 both sides.
6. **response failure (unregistered correlation)** — a response whose `correlation_id` was never registered → `response.failure` +1 both sides.

- [ ] **Step 1: Write the driver** — implement `fixture.Driver` + `fixture.BackendKindAware` (`func (*kafkaRequestsDriver) BackendKind() fixture.BackendKind { return fixture.TCPKafkaResponder }`) + `fixture.StatsAsserter`. `AssertStats` scrapes `/stats/prometheus` from BOTH admin endpoints and asserts each `envoy_kafka_<sp>_<rest>{}` counter == its arm-accounting value (ABSENT reported distinctly from present-but-wrong — the `0051` discipline). Include the Kafka wire-frame builder helpers (`buildRequest`/`buildResponse` — shareable with the unit tests if exported via a small test helper, else duplicated in the driver package).

- [ ] **Step 2: Add the README** with the per-arm accounting table + the **R4 deliberate-break liveness proof** recorded in comments (the `0030`/`0051` discipline): temporarily break each asserted counter's production increment, run `go test -run <0053> -count=1` (defeat caching — `reference_differential_break_protocol_count1`), confirm the assertion FAILS, restore.

- [ ] **Step 3: Wire the blank-import** in `runner_test.go`; run the fixture both-sides:
```bash
go test ./test/differential/ -run 'TestDifferential/0053' -count=1 -v   # expect cross-side PASS
```

- [ ] **Step 4: Execute the R4 deliberate-break proof** for each asserted counter (break → `-count=1` FAIL observed → restore → PASS). Record the observed failures in the README.

- [ ] **Step 5: gofmt + lint + commit**

```bash
git add test/fixtures/0053-kafka-requests/ test/differential/runner_test.go
git commit -m "phase 31 IMPL Task 13: 0053-kafka-requests cross-side StatsAsserter fixture (6 arms) + R4 liveness proofs"
```

---

## Task 14: `0054-kafka-boot-reject` fixture

**Files:**
- Create: `test/fixtures/0054-kafka-boot-reject/driver/driver.go`, `test/fixtures/0054-kafka-boot-reject/README.md`
- Modify: `test/differential/runner_test.go` (the `0054` blank-import)

Missing `stat_prefix` → BOTH sides reject at boot (the §6.2 `kafka-broker-stat-prefix-required` arm; boot-stderr-substring parity — each side's reject wording is its own, the harness matches on each side's substring, NOT exact cross-impl string equality). Mirror `0050-mongo-boot-reject` (the symmetric boot-reject shape; SEPARATE dir per `reference_differential_fixture_dispatch_constraint`). The api_keys-range / nested-rule arms stay unit-test-only (D-P3).

- [ ] **Step 1: Write the driver** (boot-reject shape — the `0050` precedent: a `[kafka_broker {}, tcp_proxy]` chain with `stat_prefix` omitted; assert both the reference and the subject FAIL to boot with the expected stderr substring).

- [ ] **Step 2: Add the README** (the boot-reject discipline + the cross-reference to the unit-tested deferred-feature PGV arms).

- [ ] **Step 3: Run** `go test ./test/differential/ -run 'TestDifferential/0054' -count=1 -v` — expect both-sides-reject PASS.

- [ ] **Step 4: gofmt + lint + commit**

```bash
git add test/fixtures/0054-kafka-boot-reject/ test/differential/runner_test.go
git commit -m "phase 31 IMPL Task 14: 0054-kafka-boot-reject fixture (missing stat_prefix; both sides reject at boot)"
```

---

## Task 15: The 40th fuzzer `FuzzKafkaDecode`

**Files:**
- Create: `internal/filter/network/kafkabroker/fuzz_test.go`

Mirror `FuzzMongoDecode` (`mongoproxy/fuzz_test.go`): feed arbitrary bytes through BOTH the request (`decodeOnData`) and response (`decodeOnWrite`) decoders on one decoder; assert (1) no panic, (2) the input slice is NEVER mutated (R1, both directions), (3) both private buffers stay bounded on partial-frame input. Seed with a valid ApiVersions request, an unknown-key frame, a partial frame, an oversized length, a malformed-client_id frame, a correlated response, an unregistered response.

- [ ] **Step 1: Write `FuzzKafkaDecode`** (the no-panic + no-mutation + bounded-buffer invariants; the mongo fuzzer shape).

- [ ] **Step 2: Run** the seed corpus:
```bash
go test ./internal/filter/network/kafkabroker/ -run FuzzKafkaDecode   # seed corpus PASS
go test ./internal/filter/network/kafkabroker/ -fuzz FuzzKafkaDecode -fuzztime 30s   # no crash
```
Confirm the count: `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l` → **40**.

- [ ] **Step 3: gofmt + lint + commit**

```bash
git add internal/filter/network/kafkabroker/fuzz_test.go
git commit -m "phase 31 IMPL Task 15: 40th fuzzer FuzzKafkaDecode (both directions: no-panic + no-mutation + bounded-buffer)"
```

---

## Task 16: Full differential re-verify + the deliberate-break liveness proofs

**Files:** none (verification gate); record outputs in `PROGRESS.md`.

- [ ] **Step 1: Full differential suite** (the 54 prior dirs byte-exact back-compat + the 2 new):
```bash
go test ./test/differential/ -count=1 -v 2>&1 | tee /tmp/diff31.log
ls -d test/fixtures/[0-9]* | wc -l   # expect 56
```
Expected: 56/56 green (the 54 prior byte-exact + `0053` + `0054`). The 54 prior dirs staying byte-exact is the R1 passthrough-invariant proof (the sniffer never mutated bytes).

- [ ] **Step 2: Re-run the R4 liveness proofs** (`0053`, from Task 13) under `-count=1` and confirm each break is recorded in the `0053` README.

- [ ] **Step 3: Commit** the PROGRESS.md evidence block.
```bash
git add PROGRESS.md
git commit -m "phase 31 IMPL Task 16: full differential re-verify (56/56 byte-exact incl. 54 back-compat) + R4 liveness recorded"
```

---

## Task 17: Completion bundle (ADR-0052 atomic landing)

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/DECISIONS.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/next-prompt.txt`

- [ ] **Step 1: BEHAVIOR_CONTRACT.md** — add the `### envoy.filters.network.kafka_broker` subsection (§9): the header-only decode envelope; the `flexibleVersions` + ApiVersions(18) rule; the 176-counter eager roster under `kafka.<stat_prefix>.` (the `_request`/`_response`-suffixed naming); the `unknown`/`failure` semantics; the `correlation_id → (api_key, api_version)` correlation; the 9th built-in; the `kafka.` INLINE prom arm. Update the stat table **360 → 536** (+176). Record the coverage boundaries / departures: the 86 response-duration histograms unmirrored (ADR-0060); the four active features parse-accepted-behavior-deferred (the broker-address-rewrite write-mutation as a future surgery sub-phase); the "leftover-body-bytes → unknown" sub-case unmirrored; the deprecated-low-version → unknown sub-case unmirrored (D-PLAN-1 minVersion=0 boundary); the eager-vs-per-connection boot-window difference; the per-exception-reset (NOT lifetime-sniffing-off) decode posture (D-PLAN-2); the response-side tagged-fields no-op (D-PLAN-3); runtime-keys-at-defaults; api_keys 71/72 excluded (upstream parity).

- [ ] **Step 2: DECISIONS.md** — fill the ADR-0228 §Decision + §Consequences body IN PLACE (the §Context is already anchored at `DECISIONS.md:14624`; the placeholder note at `:14658` is replaced per ADR-0044). DECISIONS.md tail stays **ADR-0228**; next-free → **ADR-0229**. No new ADR number consumed.

- [ ] **Step 3: STATE.md + ROADMAP.md** — advance the counts: stat surface **360 → 536**; fixtures **54 → 56** (tail `0054-kafka-boot-reject`); fuzzers **39 → 40**; BackendKind tail **30 → 31**; DECISIONS tail ADR-0228 (next-free 0229). ROADMAP row 31 `in-progress → done` (a flat §9 row, NO parent rollup); the §9 candidate count drops 3 → 2 ({redis, thrift}). STATE lifecycle → phase-31 phase-done; next-skill the phase-32 brainstorm (or the §9 next-row brainstorm).

- [ ] **Step 4: Run the six gates LIVE** and quote each into PROGRESS.md (`superpowers:verification-before-completion` — evidence before assertions):
```bash
go build ./...
go vet ./...
golangci-lint run
go test ./... -race -short
go test ./test/differential/ -count=1            # 56/56 byte-exact
# h2spec 53/53 + proxy-wasm 10/10 re-run LIVE (asserted-unaffected — phase 31 touches no HTTP/h2/proxy-wasm path)
```
All six GREEN, outputs quoted into PROGRESS.md (run honestly — if a gate fails, fix before claiming done).

- [ ] **Step 5: Update `next-prompt.txt`** for the next phase cold-start, then commit the completion bundle:
```bash
git add docs/envoy-go/ PROGRESS.md
git commit -m "phase 31 IMPL Task 17: completion bundle — BEHAVIOR_CONTRACT 31 (360->536), ADR-0228 body, STATE/ROADMAP row 31 done, six gates green"
```

(The controller squash-merges the branch to master + pushes at stage-close per `feedback_push_to_origin`; subagents never push — `feedback_subagents_no_push`.)

---

## Acceptance checklist (SPEC §14.3 — the IMPL is DONE when all hold)

- [ ] The `kafkabroker` package lands: 5-field config parse (+ PGV arms + IsValidName guard) + the header-only decoder (request + response) + the api-key `(root, maxVersion)` + flexibleVersions tables + the correlation map + the eager 176-counter roster; the 9th built-in + the `/contrib` blank-import; the `kafka.` INLINE prom arm.
- [ ] `/contrib v1.32.4` added + survives `go mod tidy`; the @type pinned via `proto.MessageName`.
- [ ] `0053-kafka-requests` (request/unknown-key/unknown-version/failure/response/unregistered-correlation arms) + `0054-kafka-boot-reject` green cross-side; back-compat 54 dirs byte-exact; suite 54 → 56; the new BackendKind (31).
- [ ] Stat surface 360 → 536 (+176 eager counters; 86 response-duration histograms deferred); fuzzers 39 → 40; BackendKind tail 30 → 31; ADR-0228 body in place (DECISIONS tail ADR-0228, next-free 0229).
- [ ] BEHAVIOR_CONTRACT 31 bundle; STATE/ROADMAP row 31 `in-progress → done`; six gates GREEN LIVE quoted into PROGRESS.md.
- [ ] R1 (passthrough byte-exact) / R2 (roster parity) / R3 (correlation hand-off) / R4 (StatsAsserter liveness) / R5 (header-version fidelity) / R6 (dep+tidy) / R7 (Prometheus parity) / R8 (counts re-pinned) all ratified (SPEC §13).
