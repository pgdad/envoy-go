# Phase 29.1 PLAN — `mongo_proxy` wire+BSON decode + request side

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended, per `feedback_execution_style`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Subagents commit **LOCAL-ONLY** (`feedback_subagents_no_push`); the controller squash-merges + pushes at stage-close (`feedback_push_to_origin`). Work in the git worktree (`feedback_git_worktrees`). Each task runs `gofmt -l` + `golangci-lint run` on the touched packages before its commit (`feedback_pertask_gofmt_lint`). After a temporary deliberate break (Task 16 R4 liveness), use `go test -count=1` to defeat result caching (`reference_differential_break_protocol_count1`).

**Goal:** Land the `internal/filter/network/mongoproxy/` package's REQUEST side — TypeURL + 5-field config parse (incl. the FaultDelay PGV arms), the in-house little-endian BSON parser (the 14-type upstream subset), the MongoDB legacy wire decoder (the exactly-7-opcode envelope; the 5 request opcodes body-decoded), the 23-stat fixed roster created EAGERLY under `mongo.<stat_prefix>.`, request-side increments + the dynamic `cmd.*`/`collection.*`/callsite counter families, and the per-connection active-query list — wired as the 8th built-in with the `mongo.` four-rule Prometheus TAG-EXTRACTOR arm, proven by fixtures `0049-mongo-requests` (cross-side label-aware `StatsAsserter`) + `0050-mongo-boot-reject` and the 39th fuzzer.

**Architecture:** A NEW `internal/filter/network/mongoproxy/` package implements BOTH `ReadFilter` and `WriteFilter` (one instance per connection; consumer #2 of the ADR-0221 conn-wrap seam — the `zookeeperproxy` both-directions shape). The request-side wire decoder runs in `OnData` over a PRIVATE copy of the chain bytes (the chain `Buffer` is observational, never drained — the `zookeeperproxy/decoder.go` `chainConsumed` high-water pattern adapted verbatim); `OnWrite` is a no-op `Continue` stub at 29.1 (the response decoder is 29.2); decode errors set sniffing off for the connection LIFETIME (AMEND-B6). ZERO framework changes: `internal/filter/network/` (chain.go / readconn.go / writeconn.go / types.go), `internal/listener/manager.go`, `tcp_proxy`, and HCM are all untouched at 29.1 (§4). Cross-side `StatsAsserter` counter parity with label-aware Prometheus scrape mechanics is the load-bearing differential proof.

**Tech Stack:** Go 1.26.2 / golangci-lint 1.64.8 (ADR-0009); reference Envoy v1.37.2 (ADR-0008); go-control-plane v1.32.4 (ADR-0008). Reuses `internal/filter/network/` (26.1/26.2/27/28.1a/28.1b — consumed, not modified), `internal/stats/` (06.1; `NewCounterIfAbsent` + `NewGaugeIfAbsent`), the differential harness + `fixture.StatsAsserter` + the existing `TCPSink` BackendKind. ZERO new third-party `go.mod` dependencies (BSON decode is plain `encoding/binary` little-endian reads; the `$comment` callsite parse is stdlib `encoding/json`).

---

## ADR-0045 split-gate FINAL re-check (at PLAN time, per SPEC §10.1 + parent §3.0)

The gate fires at `> ~25 tasks OR > ~1500 net-new production LoC`. This PLAN decomposes to **17 tasks** / **~1070–1470 production LoC** (the SPEC §10.1 envelope, re-confirmed at PLAN time on the 26.x accounting basis — fixture drivers + unit tests excluded):

| Unit | Production LoC | Tasks |
|---|---|---|
| `config.go` (5-field parse + FaultDelay PGV + commands set + alias table) | ~130–180 | 2–3 |
| `bson.go` (14 types + doc walk + lookups) | ~250–350 | 4–5 |
| `stats.go` (roster + dynamic-name helpers) | ~100–150 | 6 |
| `codec.go` (framing + reassembly + 5 request-opcode decode + error path) | ~280–380 | 7–9 |
| `filter.go` + `mongoproxy.go` + `doc.go` (glue + active-query list + factory) | ~150–200 | 10 |
| builtins + `bootstrap.go` + the `name.go` four-rule arm | ~100–150 | 11–12 |
| The 39th fuzzer | ~60 | 13 |
| **Total (production basis)** | **~1070–1470** | **17** |

Both axes under the gate (17 ≤ ~25 tasks; ~1470 ≤ ~1500 LoC) → **NO split. 29.1 proceeds as ONE sub-phase.** The pre-authorized 29.1a/29.1b axis (SPEC §10.1) stays UNCONSUMED. The fixture drivers (`0049` ~700–950 LoC + `0050` ~200 LoC, the 875/202-LoC `0046`/`0047` precedents) are excluded per the 26.x/27/28.x accounting precedent. The four-rule `name.go` arm is the only piece materially larger than the parent estimate (AMEND-C1), already folded into the upper bound above.

## PLAN-time D-question dispositions (SPEC §12.2)

- **Task reorder — `stats.go` moves BEFORE `codec.go` (the green-compiling dependency order; SPEC §10 lead-in permits merge/split).** The SPEC §10 spine lists codec (Tasks 6–8) before stats (Task 9). The codec's per-opcode decode INCREMENTS roster + dynamic counters, so the roster must exist for codec tasks to compile + unit-test green standalone. This PLAN therefore orders `config (2–3) → bson (4–5) → stats (6) → codec (7–9) → filter glue (10) → integration (11–12) → fuzzer (13) → fixtures (14–16) → completion (17)` — the exact `config → stats → decoder → glue` dependency order the 28.1 PLAN used (28.1 Tasks 6→7→8→9–10→11). The SPEC's task-to-deliverable mapping is preserved; only the linear order changes.
- **D-S29.1-2 (BSON internal representation) — RESOLVED at PLAN: an ordered-element `bsonDoc`.** `bson.go` exposes a package-internal `type bsonDoc struct { elems []bsonElem }` with `bsonElem{name string; typ byte; val any}` (ordered as on the wire) + lookup helpers `first() (bsonElem, bool)` and `find(name string) (bsonElem, bool)`. Numeric `_id`-shape + `$maxTimeMS` checks unify Int32 (0x10, Go `int32`), Int64 (0x12, Go `int64`), Double (0x01, Go `float64`) via a helper `asInt64(v any) (int64, bool)`. No exported API (package-internal; extract-at-second-consumer per parent §4.2 YAGNI).
- **The active-query list lives ON THE DECODER (`dec.queries`), NOT on the `filter` struct (refines the SPEC §3.7 anticipated shape).** The SPEC §3.7 sketched `queries []activeQuery` on the `filter` struct, but the append happens during OP_QUERY decode INSIDE the codec, and the 29.2 gauge inc/dec + correlation also operate at decode time. Putting the list on the decoder (mirroring zookeeper's `requestsByXid`/`controlRequestsByXid` correlation-structures-on-the-decoder, `decoder.go:72-78`) keeps the codec unit-testable in isolation (feed bytes → assert `dec.queries`) and keeps Tasks 7–9 green BEFORE `filter.go` (Task 10) exists. The `filter` reaches the list via `f.dec.queries` (29.2's mutex + gauge sites attach to the decoder, the zookeeper `mu`/`decoder.go:62-70` precedent). This resolves the §3.7 representation question; the §3.7 `activeQuery` field set (requestID/collection/command/callsite/start) is unchanged.
- **D-S29.1-4 (chainConsumed adaptation) — the high-water mark is `dec.chainConsumed int64` against `Buffer.TotalAppended()`, adapted verbatim from `zookeeperproxy/decoder.go:40-48,99-104`.** The decoder receives the FULL chain-buffer slice + `buf.TotalAppended()` on every `OnData`; it appends only the trailing `(totalAppended − chainConsumed)` never-before-seen bytes to `readBuf`. The multi-read no-double-count unit test lands at Task 7.
- **D-S29.1-5 (multi-label Prometheus rendering) — RESOLVED at PLAN: the mongo `name.go` arm SORTS its own labels locally; `prom.go` is NOT touched.** `prom.go:90-113` `writeMetricLine` emits labels in slice order with NO sort, and Rule SN4 PREPENDS `envoy_response_code_class` ahead of the existing single label — so a global sort in `prom.go` would reorder existing two-label SN1–SN4 lines and break golden fixtures. Instead the four-rule mongo arm appends its 1–3 labels and then `sort.Slice`s them by `Key` before returning (the reference emits alphabetical key order — §11.2: `callsite`, `collection`, `prefix`). This is a localized change inside `name.go`, NOT a `prom.go` touch → `internal/stats/prom.go` stays byte-identical. A Task-12 unit test asserts the emitted three-label callsite line byte-matches the §11.2 form.
- **D-S29.1-6 (decoding_error single-increment discipline) — RESOLVED at PLAN: enforced IN THE DECODER.** The `sniffing bool` flag and the `decoding_error` counter are co-located on the decoder; `decoderError()` is a single method that checks-and-clears `sniffing` and increments at most once (the flag and the counter are mutated together). Tested at Task 7.
- **IMPL-owned D-questions left to their tasks:** D-S29.1-1 (PARSE-REJECT byte-stable wording — Task 3; anticipated prefix `mongo_proxy: `), D-S29.1-3 (wire-byte builder helper shape — Task 14; anticipated `bsonDoc(...)/opQuery(...)/opInsert(...)` builders in the driver package, shared with the future 29.2 `0051` driver).

---

## File Structure

**Created:**
- `internal/filter/network/mongoproxy/doc.go` — package doc (the mongo_proxy request side; ADR-0224; the 29.2/29.3 forward-pointers).
- `internal/filter/network/mongoproxy/mongoproxy.go` — `TypeURL` (via `proto.MessageName`).
- `internal/filter/network/mongoproxy/mongoproxy_test.go` — TypeURL pinning test.
- `internal/filter/network/mongoproxy/config.go` — `compiledConfig` + `parseConfig` (5-field parse + FaultDelay PGV validation + the commands remembered-set + the alias-normalization table + the PARSE-REJECT constants).
- `internal/filter/network/mongoproxy/config_test.go` — parse / defaults / commands-default-replace-remember / alias-normalization / every PGV-mirror reject-arm tests.
- `internal/filter/network/mongoproxy/bson.go` — the in-house BSON parser (the 14-type subset; ordered-element `bsonDoc`; throw-on-unknown; lookup helpers).
- `internal/filter/network/mongoproxy/bson_test.go` — each of the 14 types / throw-on-unknown (0x06/0x0D/0x13) / nested docs / truncation / string+cstring edge cases / lookup-helper tests.
- `internal/filter/network/mongoproxy/stats.go` — `rosterSuffixes()` (the 22-counter table) + `mongoStats` eager creation (22 counters + the gauge) + the dynamic cmd/collection/callsite name helpers.
- `internal/filter/network/mongoproxy/stats_test.go` — `TestStatRoster_MatchesUpstreamMacro` (R2 golden) + eager/idempotent/dynamic-name tests.
- `internal/filter/network/mongoproxy/codec.go` — the per-connection `decoder`: MsgHeader framing + private-buffer reassembly + the opcode dispatch + per-opcode request body decode + the `decoding_error`/sniffing-off path + the active-query list.
- `internal/filter/network/mongoproxy/codec_test.go` — framing / partial-frame reassembly / multi-read no-double-count / opcode dispatch / recognized-not-decoded Reply+CommandReply / OP_MSG→decoding_error / sniffing-off / per-opcode body decode / query-shape heuristics / collection+command extraction / $maxTimeMS+$comment / active-query-list population tests.
- `internal/filter/network/mongoproxy/filter.go` — `NewFactory(reg *stats.Registry)` + the both-directions `filter` glue (OnNewConnection / OnData / OnWrite stub / SetReadFilterCallbacks / SetWriteFilterCallbacks / OnDestroy).
- `internal/filter/network/mongoproxy/filter_test.go` — factory / both-directions injection / OnData-feeds-decoder / OnWrite-no-op / R3 chain-buffer-never-drained / OnDestroy tests.
- `internal/filter/network/mongoproxy/fuzz_test.go` — `FuzzMongoDecode` (the 39th fuzzer).
- `test/fixtures/0049-mongo-requests/driver/driver.go` + `README.md` — the cross-side 9-arm label-aware StatsAsserter fixture.
- `test/fixtures/0050-mongo-boot-reject/driver/driver.go` + `README.md` — the symmetric boot-reject fixture.

**Modified:**
- `internal/stats/name.go` — the `mongo.` FOUR-RULE TAG-EXTRACTOR arm (in the `default` branch, after the `.zookeeper.` arm at `name.go:255-262`, before the default error at `name.go:263`).
- `internal/stats/name_test.go` — the mongo flattening tests (fixed + cmd + collection + callsite shapes; deterministic sorted-label ordering; dot-free-prefix guard; non-matching still errors).
- `internal/filter/network/builtins/builtins.go` — the 8th registration (after `zookeeperproxy` at `builtins.go:68`); package doc `seven` → `eight`.
- `internal/filter/network/builtins/builtins_test.go` — the all-eight registration test + the mongo registration test + boot smoke.
- `internal/bootstrap/bootstrap.go` — the `mongo_proxy/v3` blank-import (after `zookeeper_proxy/v3` at `bootstrap.go:95`).
- `test/differential/runner_test.go` — the `0049`/`0050` driver blank-imports (the `TCPSink` backend arm already exists from 28.1 — no `fixture.go` change).
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` / `DECISIONS.md` / `STATE.md` / `ROADMAP.md` / `next-prompt.txt` — the completion bundle (Task 17).

**Untouched (pinned — the §4 zero-touch property; a regression gate):** `internal/filter/network/` (chain.go / readconn.go / writeconn.go / types.go / callbacks.go / terminal.go / registry.go), `internal/listener/manager.go`, `internal/filter/tcpproxy/`, `internal/filter/hcm/`, `internal/accesslog/`, `internal/stats/prom.go` (D-S29.1-5 — the mongo arm sorts its own labels), `internal/stats/registry.go`, `internal/stats/gauge.go`, `test/differential/fixture/fixture.go` (`TCPSink = 28` already exists from 28.1).

---

## Task 1: First-action baselines/anchors gate (no code change)

**Files:** none modified — verification + re-pin gate at the IMPL-session tip; record in `PROGRESS.md` (created this task at the worktree root).

- [ ] **Step 1: Re-confirm the project counts at the IMPL-session tip**

Run (from repo root):
```bash
git log --oneline -1
# fixtures (canonical recipe):
ls -d test/fixtures/[0-9]* | wc -l            # expect 50; tail dir:
ls -d test/fixtures/[0-9]* | tail -1          # expect test/fixtures/0048-zookeeper-responses
# fuzzers (canonical recipe — scoped to ./internal):
grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l   # expect 38
# DECISIONS.md tail + next-free:
grep -oE "ADR-0[0-9]+" docs/envoy-go/DECISIONS.md | sort -u | tail -3  # tail = ADR-0226 → next-free ADR-0227
```
Expected: fixtures **50** (tail `0048-zookeeper-responses`); fuzzers **38**; DECISIONS.md tail **ADR-0226** (next-free **ADR-0227**; the ADR-0224/0225/0226 §Context drafts landed at the parent SPEC — `DECISIONS.md:14421/:14440/:14459`). 29.1 lands `0049`+`0050` → 52, the 39th fuzzer, and the ADR-0224 §Decision/§Consequences body IN PLACE (no new ADR number consumed).

- [ ] **Step 2: Re-confirm the stat surface = 337**

Run the project's canonical stat-surface recipe (the count STATE.md / BEHAVIOR_CONTRACT.md report as **337** — the BEHAVIOR_CONTRACT stat-table row count; `BEHAVIOR_CONTRACT.md:466` "Phase 28.1 extension — 136 → 337 internal names"; do NOT invent a new recipe). Expected: **337**. 29.1 lands +23 → **360** at Task 17.

- [ ] **Step 3: Re-confirm `proto.MessageName` (the TypeURL pin) + the field roster**

```bash
cat > /tmp/mongo_tu.go <<'EOF'
package main

import (
	"fmt"

	mpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/mongo_proxy/v3"
	"google.golang.org/protobuf/proto"
)

func main() { fmt.Println(proto.MessageName(&mpv3.MongoProxy{})) }
EOF
go run /tmp/mongo_tu.go   # expect: envoy.extensions.filters.network.mongo_proxy.v3.MongoProxy
```
Expected `proto.MessageName` = `envoy.extensions.filters.network.mongo_proxy.v3.MongoProxy` (the `extensions.` segment per `reference_network_filter_typeurl_extensions`) → `TypeURL` = `type.googleapis.com/` + that. Confirm the 5-field accessor set + the FaultDelay oneof accessors against go-control-plane v1.32.4 in-tree (the §11.1 pin):
```bash
MP=$(go env GOMODCACHE)/github.com/envoyproxy/go-control-plane/envoy@v1.32.4/extensions/filters/network/mongo_proxy/v3/mongo_proxy.pb.go
grep -nE "func \(x \*MongoProxy\) Get" $MP
# expect: GetStatPrefix() string / GetAccessLog() string / GetDelay() *v3.FaultDelay /
#         GetEmitDynamicMetadata() bool / GetCommands() []string
FD=$(go env GOMODCACHE)/github.com/envoyproxy/go-control-plane/envoy@v1.32.4/extensions/filters/common/fault/v3/fault.pb.go
grep -nE "func \(x \*FaultDelay\) Get|FaultDelay_FixedDelay|FaultDelay_HeaderDelay_" $FD
# expect: GetFaultDelaySecifier() (the oneof; note the upstream-proto "Secifier" spelling) /
#         GetFixedDelay() *durationpb.Duration / GetHeaderDelay() *FaultDelay_HeaderDelay /
#         GetPercentage() *typev3.FractionalPercent
FP=$(go env GOMODCACHE)/github.com/envoyproxy/go-control-plane/envoy@v1.32.4/type/v3/percent.pb.go
grep -nE "FractionalPercent_DenominatorType = [0-9]+" $FP
# expect: HUNDRED=0, TEN_THOUSAND=1, MILLION=2 (the valid denominator set {0,1,2})
```
Record the exact Go identifiers (incl. the `FaultDelay_FixedDelay` / `FaultDelay_HeaderDelay_` oneof-wrapper type names and the `FaultDelaySecifier` typo) for Tasks 2–3.

- [ ] **Step 4: Create `PROGRESS.md`** at the worktree root with the count baselines above + a per-task log section. Commit:

```bash
git add PROGRESS.md
git commit -m "phase 29.1 IMPL Task 1: first-action baselines gate (fixtures 50, fuzzers 38, stats 337, ADR tail 0226; mongo TypeURL + field roster pinned)"
```

---

## Task 2: `mongoproxy` package skeleton + TypeURL + config parse (5 fields + commands set + alias table)

**Files:**
- Create: `internal/filter/network/mongoproxy/doc.go`
- Create: `internal/filter/network/mongoproxy/mongoproxy.go`
- Create: `internal/filter/network/mongoproxy/config.go`
- Test: `internal/filter/network/mongoproxy/mongoproxy_test.go`, `internal/filter/network/mongoproxy/config_test.go`

> **PLAN disposition (mirrors 28.1):** the SPEC §10 Task-2 bundle "skeleton + TypeURL + NewFactory + config parse" is SPLIT — Task 2 lands `doc.go` + `mongoproxy.go` (TypeURL only) + `config.go` (pure parse). `NewFactory` + the per-connection `filter` glue land at Task 10 (`filter.go`), after the roster (Task 6) and decoder (Tasks 7–9) it constructs exist. Each intermediate task compiles + tests green standalone.

- [ ] **Step 1: Write the failing TypeURL pinning test**

`mongoproxy_test.go`:
```go
package mongoproxy

import "testing"

func TestTypeURL_PinnedToUpstreamExtensionsName(t *testing.T) {
	const want = "type.googleapis.com/envoy.extensions.filters.network.mongo_proxy.v3.MongoProxy"
	if TypeURL != want {
		t.Fatalf("TypeURL = %q, want %q", TypeURL, want)
	}
}
```

- [ ] **Step 2: Run it — expect a COMPILE failure** (`undefined: TypeURL`)

Run: `go test ./internal/filter/network/mongoproxy/ -run TestTypeURL -v`
Expected: build error `undefined: TypeURL`.

- [ ] **Step 3: Write `doc.go` + `mongoproxy.go`**

`doc.go`:
```go
// Package mongoproxy implements the envoy.filters.network.mongo_proxy network
// filter (ADR-0224) — a passive MongoDB legacy-wire-protocol sniffer. At phase
// 29.1 it lands the REQUEST side: the 5-field config parse (incl. the FaultDelay
// PGV arms, parsed-but-consumed-at-29.3), the in-house little-endian BSON parser
// (the 14-type upstream subset), the wire decoder (the exactly-7-opcode envelope;
// the 5 request opcodes body-decoded; OP_REPLY/OP_COMMANDREPLY recognized but not
// decoded until 29.2), the 23-stat fixed roster created eagerly under
// mongo.<stat_prefix>., the dynamic cmd/collection/callsite counter families, and
// the per-connection active-query list (written here, consumed at 29.2).
//
// It is consumer #2 of the ADR-0221 network.WriteFilter seam: it implements BOTH
// ReadFilter and WriteFilter, one instance per connection. The 29.1 OnWrite is a
// pure no-op Continue stub (the response decoder + correlation + the gauge
// increments land at 29.2; the async halt/resume seam + fault delay + access log
// land at 29.3).
package mongoproxy
```

`mongoproxy.go`:
```go
package mongoproxy

import (
	mongo_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/mongo_proxy/v3"
	"google.golang.org/protobuf/proto"
)

// TypeURL is the canonical Any type URL for mongo_proxy's typed_config. Derived
// via proto.MessageName (NEVER a hand-typed docs string —
// reference_network_filter_typeurl_extensions; the zookeeperproxy.go:17
// precedent). Resolves to the parent §5.1 string (the extensions. segment).
var TypeURL = "type.googleapis.com/" + string(proto.MessageName(&mongo_proxyv3.MongoProxy{}))
```

- [ ] **Step 4: Run the TypeURL test — expect PASS**

Run: `go test ./internal/filter/network/mongoproxy/ -run TestTypeURL -v`
Expected: PASS.

- [ ] **Step 5: Write the failing config-parse tests**

`config_test.go` (the load-bearing arms — full set lands incrementally with Step 7; start with parse + commands + alias):
```go
package mongoproxy

import (
	"testing"

	mongo_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/mongo_proxy/v3"
)

func TestParseConfig_StatPrefixStored(t *testing.T) {
	cfg, err := parseConfig(&mongo_proxyv3.MongoProxy{StatPrefix: "mongo_a"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.statPrefix != "mongo_a" {
		t.Errorf("statPrefix = %q, want mongo_a", cfg.statPrefix)
	}
}

func TestParseConfig_CommandsDefault(t *testing.T) {
	// Empty list → the default {delete, insert, update} (AMEND-B7).
	cfg, err := parseConfig(&mongo_proxyv3.MongoProxy{StatPrefix: "p"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	for _, c := range []string{"delete", "insert", "update"} {
		if !cfg.commands[c] {
			t.Errorf("default commands missing %q", c)
		}
	}
	if cfg.commands["isMaster"] {
		t.Errorf("default commands must NOT contain isMaster")
	}
}

func TestParseConfig_CommandsExplicitReplacesDefault(t *testing.T) {
	cfg, err := parseConfig(&mongo_proxyv3.MongoProxy{StatPrefix: "p", Commands: []string{"isMaster"}})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if !cfg.commands["isMaster"] {
		t.Errorf("explicit commands missing isMaster")
	}
	if cfg.commands["delete"] {
		t.Errorf("explicit list must REPLACE the default (delete leaked in)")
	}
}

func TestNormalizeCommand_Aliases(t *testing.T) {
	cases := map[string]string{
		"collstats":     "collStats",
		"dbstats":       "dbStats",
		"findandmodify": "findAndModify",
		"getlasterror":  "getLastError",
		"ismaster":      "isMaster",
		"find":          "", // find clears → routed to the query path, never a cmd.* stat
		"isMaster":      "isMaster", // already canonical → unchanged
		"insert":        "insert",   // not an alias → unchanged
	}
	for in, want := range cases {
		if got := normalizeCommand(in); got != want {
			t.Errorf("normalizeCommand(%q) = %q, want %q", in, got, want)
		}
	}
}
```

- [ ] **Step 6: Run them — expect COMPILE failure** (`undefined: parseConfig` / `normalizeCommand`)

Run: `go test ./internal/filter/network/mongoproxy/ -run TestParseConfig -v`
Expected: build error.

- [ ] **Step 7: Write `config.go` (parse + commands + alias; the FaultDelay arms land at Task 3)**

`config.go`:
```go
package mongoproxy

import (
	"errors"
	"time"

	mongo_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/mongo_proxy/v3"
)

// PARSE-REJECT arms (ADR-0080 byte-stable; SPEC §6; D-S29.1-1). The error prefix
// for all mongoproxy arms is "mongo_proxy: " (mirrors zookeeper_proxy: —
// zookeeperproxy/config.go:148-155). These strings are byte-stable from this
// commit forward — DO NOT CHANGE. The stat_prefix arm is the load-bearing 0050
// fixture arm; the three FaultDelay arms are unit-test-only at 29.1 (their
// fixture disposition is parent D-P5, resolved at the 29.3 SPEC).
const (
	errStatPrefixRequired       = "mongo_proxy: stat_prefix is required"
	errDelaySpecifierRequired   = "mongo_proxy: delay: a delay type must be specified"
	errDelayFixedDelayTooSmall  = "mongo_proxy: delay: fixed_delay must be greater than 0s"
	errDelayDenominatorInvalid  = "mongo_proxy: delay: percentage denominator is not a defined value"
)

// defaultCommands is the AMEND-B7 default remembered-set when the proto commands
// list is empty: {delete, insert, update}. An explicit list REPLACES this.
var defaultCommands = []string{"delete", "insert", "update"}

// commandAliases normalizes DECODED wire command names before the remembered-set
// lookup (AMEND-B7 / parent §11.3; upstream utility.cc:21-37 normalizes at decode
// time, NOT the configured commands entries). "find" maps to "" — find is handled
// as a query (the collection path), never a cmd.* stat.
var commandAliases = map[string]string{
	"collstats":     "collStats",
	"dbstats":       "dbStats",
	"findandmodify": "findAndModify",
	"getlasterror":  "getLastError",
	"ismaster":      "isMaster",
	"find":          "",
}

// normalizeCommand applies commandAliases to a decoded wire command name. A name
// with no alias entry is returned unchanged. "find" → "" (query path).
func normalizeCommand(name string) string {
	if canon, ok := commandAliases[name]; ok {
		return canon
	}
	return name
}

// compiledConfig is the boot-parsed, per-listener-shared mongo_proxy config
// (ADR-0079 two-step factory). The roster (stats.go) attaches at factory time
// (NewFactory in filter.go); per-connection state lives on the decoder, never here.
type compiledConfig struct {
	statPrefix string

	// commands is the AMEND-B7 remembered-set (canonical names → membership).
	// Consumed at 29.1 by the codec's $cmd command-name lookup (§3.6).
	commands map[string]bool

	// Parsed at 29.1; CONSUMED later. accessLog (string path) → 29.3;
	// emitDynamicMetadata → 29.2; the fault-delay fields → 29.3. delayConfigured
	// records whether a (validated) delay block was present.
	accessLog           string
	emitDynamicMetadata bool
	delayConfigured     bool
	fixedDelay          time.Duration
	delayPercentNum     uint32
	delayPercentDenom   int32

	// stats is the eagerly-created 23-stat roster (22 counters + the gauge),
	// attached by NewFactory at boot (D-P1 eager creation). All per-connection
	// filter instances share this pointer; counter/gauge ops are atomic.
	stats *mongoStats
}

// parseConfig validates + compiles the 5-field proto (parent §5.2 roster).
// access_log + emit_dynamic_metadata are parse-and-store (consumed later — §3.3).
// Returns PARSE-REJECT errors on PGV-mirror violations (ADR-0080 byte-stable).
// Validation order: stat_prefix first, then the delay block (Task 3).
func parseConfig(msg *mongo_proxyv3.MongoProxy) (*compiledConfig, error) {
	if msg.GetStatPrefix() == "" {
		return nil, errors.New(errStatPrefixRequired)
	}
	cfg := &compiledConfig{
		statPrefix:          msg.GetStatPrefix(),
		accessLog:           msg.GetAccessLog(),
		emitDynamicMetadata: msg.GetEmitDynamicMetadata(),
		commands:            map[string]bool{},
	}
	cmds := msg.GetCommands()
	if len(cmds) == 0 {
		cmds = defaultCommands
	}
	for _, c := range cmds {
		cfg.commands[c] = true
	}
	if err := parseDelay(msg.GetDelay(), cfg); err != nil { // Task 3
		return nil, err
	}
	return cfg, nil
}
```

> Task 3 adds `parseDelay`; to keep Step 7 green, land a one-line stub `func parseDelay(_ *commonfaultv3.FaultDelay, _ *compiledConfig) error { return nil }` in `config.go` now and REPLACE it at Task 3. (Add the `commonfaultv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/common/fault/v3"` import with the stub.)

- [ ] **Step 8: Run the config tests — expect PASS**

Run: `go test ./internal/filter/network/mongoproxy/ -run 'TestParseConfig|TestNormalizeCommand|TestTypeURL' -v`
Expected: all PASS.

- [ ] **Step 9: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/mongoproxy/
golangci-lint run ./internal/filter/network/mongoproxy/...
go test ./internal/filter/network/mongoproxy/...
git add internal/filter/network/mongoproxy/ && git commit -m "phase 29.1 IMPL Task 2: mongoproxy skeleton + TypeURL + config parse (5 fields, commands remembered-set, alias normalization)"
```
Expected: `gofmt -l` prints nothing; lint clean; tests PASS.

---

## Task 3: FaultDelay PGV validation + PARSE-REJECT byte-stable constants

**Files:**
- Modify: `internal/filter/network/mongoproxy/config.go` (replace the `parseDelay` stub)
- Test: `internal/filter/network/mongoproxy/config_test.go`

- [ ] **Step 1: Write the failing FaultDelay + byte-stable reject tests**

Append to `config_test.go`:
```go
import (
	"google.golang.org/protobuf/types/known/durationpb"

	commonfaultv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/common/fault/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
)

func TestParseConfig_DelaySpecifierRequired(t *testing.T) {
	// delay: {} — the oneof absent → reject (AMEND-B9, PGV `required` mirror).
	_, err := parseConfig(&mongo_proxyv3.MongoProxy{StatPrefix: "p", Delay: &commonfaultv3.FaultDelay{}})
	if err == nil || err.Error() != errDelaySpecifierRequired {
		t.Fatalf("err = %v, want %q", err, errDelaySpecifierRequired)
	}
}

func TestParseConfig_FixedDelayTooSmall(t *testing.T) {
	d := &commonfaultv3.FaultDelay{
		FaultDelaySecifier: &commonfaultv3.FaultDelay_FixedDelay{FixedDelay: durationpb.New(0)},
	}
	_, err := parseConfig(&mongo_proxyv3.MongoProxy{StatPrefix: "p", Delay: d})
	if err == nil || err.Error() != errDelayFixedDelayTooSmall {
		t.Fatalf("err = %v, want %q", err, errDelayFixedDelayTooSmall)
	}
}

func TestParseConfig_FixedDelayValid(t *testing.T) {
	d := &commonfaultv3.FaultDelay{
		FaultDelaySecifier: &commonfaultv3.FaultDelay_FixedDelay{FixedDelay: durationpb.New(5 * time.Millisecond)},
		Percentage:         &typev3.FractionalPercent{Numerator: 50, Denominator: typev3.FractionalPercent_HUNDRED},
	}
	cfg, err := parseConfig(&mongo_proxyv3.MongoProxy{StatPrefix: "p", Delay: d})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if !cfg.delayConfigured || cfg.fixedDelay != 5*time.Millisecond {
		t.Errorf("delay not stored: configured=%v fixed=%v", cfg.delayConfigured, cfg.fixedDelay)
	}
}

func TestParseConfig_PercentageDenominatorInvalid(t *testing.T) {
	d := &commonfaultv3.FaultDelay{
		FaultDelaySecifier: &commonfaultv3.FaultDelay_FixedDelay{FixedDelay: durationpb.New(time.Millisecond)},
		Percentage:         &typev3.FractionalPercent{Numerator: 1, Denominator: 99}, // out-of-range enum
	}
	_, err := parseConfig(&mongo_proxyv3.MongoProxy{StatPrefix: "p", Delay: d})
	if err == nil || err.Error() != errDelayDenominatorInvalid {
		t.Fatalf("err = %v, want %q", err, errDelayDenominatorInvalid)
	}
}

func TestParseRejectConstants_ByteStable(t *testing.T) {
	// ADR-0080 byte-stable wording guard (D-S29.1-1). DO NOT update these to
	// match a code change — a mismatch means the production wording regressed.
	want := map[string]string{
		"stat_prefix":  "mongo_proxy: stat_prefix is required",
		"specifier":    "mongo_proxy: delay: a delay type must be specified",
		"fixed_delay":  "mongo_proxy: delay: fixed_delay must be greater than 0s",
		"denominator":  "mongo_proxy: delay: percentage denominator is not a defined value",
	}
	got := map[string]string{
		"stat_prefix": errStatPrefixRequired, "specifier": errDelaySpecifierRequired,
		"fixed_delay": errDelayFixedDelayTooSmall, "denominator": errDelayDenominatorInvalid,
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s arm = %q, want %q", k, got[k], w)
		}
	}
}
```

- [ ] **Step 2: Run them — expect FAIL** (the stub `parseDelay` returns nil → the delay tests fail; the byte-stable test passes already)

Run: `go test ./internal/filter/network/mongoproxy/ -run 'TestParseConfig_Delay|TestParseConfig_Fixed|TestParseConfig_Percentage' -v`
Expected: the delay-reject + delay-valid tests FAIL (no validation / not stored).

- [ ] **Step 3: Replace the `parseDelay` stub with the real validator**

In `config.go`, replace the stub:
```go
// validDenominators is the FractionalPercent.DenominatorType enum set {HUNDRED=0,
// TEN_THOUSAND=1, MILLION=2}. go-control-plane ships NO generated FractionalPercent
// Validate(); envoy-go rejects out-of-range values for parity (parent §5.3 note).
var validDenominators = map[int32]bool{0: true, 1: true, 2: true}

// parseDelay validates + stores the FaultDelay block (AMEND-B9). The oneof
// fault_delay_secifier is REQUIRED when delay is present; fixed_delay must be
// > 0s; header_delay is parse-accepted (no delay results at 29.3 — parent §5.3);
// the percentage (if present) must carry a defined denominator. Consumed at 29.3.
func parseDelay(d *commonfaultv3.FaultDelay, cfg *compiledConfig) error {
	if d == nil {
		return nil // no delay block → nothing to validate
	}
	cfg.delayConfigured = true
	switch s := d.GetFaultDelaySecifier().(type) {
	case *commonfaultv3.FaultDelay_FixedDelay:
		fixed := s.FixedDelay.AsDuration()
		if fixed <= 0 {
			return errors.New(errDelayFixedDelayTooSmall)
		}
		cfg.fixedDelay = fixed
	case *commonfaultv3.FaultDelay_HeaderDelay_:
		// header_delay: parse-accept; produces no delay at 29.3 (parent §5.3 / D-P5).
	default: // nil oneof (delay: {}) → the specifier-required reject.
		return errors.New(errDelaySpecifierRequired)
	}
	if p := d.GetPercentage(); p != nil {
		if !validDenominators[int32(p.GetDenominator())] {
			return errors.New(errDelayDenominatorInvalid)
		}
		cfg.delayPercentNum = p.GetNumerator()
		cfg.delayPercentDenom = int32(p.GetDenominator())
	}
	return nil
}
```
(The `commonfaultv3` import already arrived with the Task-2 stub; the stub line is now removed.)

- [ ] **Step 4: Run the delay + byte-stable tests — expect PASS**

Run: `go test ./internal/filter/network/mongoproxy/ -run 'TestParseConfig|TestParseReject' -v`
Expected: all PASS.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/mongoproxy/
golangci-lint run ./internal/filter/network/mongoproxy/...
go test ./internal/filter/network/mongoproxy/...
git add internal/filter/network/mongoproxy/ && git commit -m "phase 29.1 IMPL Task 3: FaultDelay PGV validation (oneof-required + gt-0s + denominator) + byte-stable PARSE-REJECT constants"
```
Expected: clean; tests PASS.

---

## Task 4: `bson.go` part 1 — reader primitives + document framing + the scalar types + throw-on-unknown

**Files:**
- Create: `internal/filter/network/mongoproxy/bson.go`
- Test: `internal/filter/network/mongoproxy/bson_test.go`

> Mirrors upstream `bson_impl.cc:386-520` exactly (parent §11.4 item 5 — `reference_wire_format_both_sides_see_same_bytes`). All multi-byte reads are little-endian. Part 1 lands the reader, the document frame walk, the scalar/fixed-width element types (Double 0x01, Boolean 0x08, Datetime 0x09, Null 0x0A, Int32 0x10, Timestamp 0x11, Int64 0x12), and the throw-on-unknown-type + underflow paths. Part 2 (Task 5) adds the variable-length + nested types + lookups.

- [ ] **Step 1: Write the failing part-1 tests**

`bson_test.go`:
```go
package mongoproxy

import (
	"encoding/binary"
	"testing"
)

// le helpers build little-endian wire bytes for tests (mirrored by the 0049
// driver's builders at Task 14).
func leI32(v int32) []byte { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, uint32(v)); return b }
func leI64(v int64) []byte { b := make([]byte, 8); binary.LittleEndian.PutUint64(b, uint64(v)); return b }

// doc builds a BSON document from raw element bytes (type+cstring-name+value...).
func doc(elems ...byte) []byte {
	body := append(elems, 0x00) // terminator
	total := int32(4 + len(body))
	return append(leI32(total), body...)
}

func cstr(s string) []byte { return append([]byte(s), 0x00) }

func TestBSON_Int32Element(t *testing.T) {
	// {"a": int32(7)}
	raw := doc(append(append([]byte{0x10}, cstr("a")...), leI32(7)...)...)
	d, err := parseBSON(raw)
	if err != nil {
		t.Fatalf("parseBSON: %v", err)
	}
	e, ok := d.first()
	if !ok || e.name != "a" || e.typ != 0x10 {
		t.Fatalf("first elem = %+v ok=%v", e, ok)
	}
	if e.val.(int32) != 7 {
		t.Errorf("val = %v, want 7", e.val)
	}
}

func TestBSON_ScalarTypes(t *testing.T) {
	// {"d": double, "b": bool, "n": null, "i64": int64}
	var elems []byte
	elems = append(elems, 0x01)
	elems = append(elems, cstr("d")...)
	dbl := make([]byte, 8)
	binary.LittleEndian.PutUint64(dbl, 0x3FF0000000000000) // 1.0
	elems = append(elems, dbl...)
	elems = append(elems, 0x08)
	elems = append(elems, cstr("b")...)
	elems = append(elems, 0x01) // true
	elems = append(elems, 0x0A)
	elems = append(elems, cstr("n")...)
	elems = append(elems, 0x12)
	elems = append(elems, cstr("i64")...)
	elems = append(elems, leI64(9000000000)...)
	d, err := parseBSON(doc(elems...))
	if err != nil {
		t.Fatalf("parseBSON: %v", err)
	}
	if len(d.elems) != 4 {
		t.Fatalf("got %d elems, want 4", len(d.elems))
	}
}

func TestBSON_UnknownTypeThrows(t *testing.T) {
	// 0x13 Decimal128 is NOT in the 14-type subset → error (upstream throw parity).
	raw := doc(append(append([]byte{0x13}, cstr("x")...), make([]byte, 16)...)...)
	if _, err := parseBSON(raw); err == nil {
		t.Fatalf("parseBSON accepted 0x13 Decimal128; want error")
	}
}

func TestBSON_UndefinedAndJSCodeThrow(t *testing.T) {
	for _, bad := range []byte{0x06, 0x0D} { // Undefined, JS code
		raw := doc(append([]byte{bad}, cstr("x")...)...)
		if _, err := parseBSON(raw); err == nil {
			t.Errorf("parseBSON accepted type 0x%02x; want error", bad)
		}
	}
}

func TestBSON_TruncatedUnderflow(t *testing.T) {
	// Declared docLength longer than the actual buffer → error.
	raw := doc(append(append([]byte{0x10}, cstr("a")...), leI32(7)...)...)
	if _, err := parseBSON(raw[:len(raw)-2]); err == nil {
		t.Fatalf("parseBSON accepted a truncated document; want error")
	}
}
```

- [ ] **Step 2: Run them — expect COMPILE failure** (`undefined: parseBSON`)

Run: `go test ./internal/filter/network/mongoproxy/ -run TestBSON -v`
Expected: build error.

- [ ] **Step 3: Write `bson.go` part 1**

`bson.go`:
```go
package mongoproxy

import (
	"encoding/binary"
	"errors"
	"math"
)

// errBSON wraps a BSON decode failure. The codec converts any BSON error into
// the decoding_error path (§3.5). Wording is internal-only (never byte-compared)
// — the differential proof is the decoding_error COUNT, not the message.
func errBSON(msg string) error { return errors.New("mongoproxy: bson: " + msg) }

// bsonElem is one document element in wire order. val holds the decoded Go value
// per type (Int32→int32, Int64/Datetime/Timestamp→int64, Double→float64,
// Boolean→bool, String/Symbol→string, ObjectId/Binary→[]byte, Document/Array→
// bsonDoc, Null→nil, Regex→[2]string). The codec reads only first()/find() +
// asInt64 (D-S29.1-2) — the full materialization mirrors upstream's eager parse.
type bsonElem struct {
	name string
	typ  byte
	val  any
}

// bsonDoc is a parsed BSON document (ordered elements). No exported API.
type bsonDoc struct {
	elems []bsonElem
}

func (d bsonDoc) first() (bsonElem, bool) {
	if len(d.elems) == 0 {
		return bsonElem{}, false
	}
	return d.elems[0], true
}

func (d bsonDoc) find(name string) (bsonElem, bool) {
	for _, e := range d.elems {
		if e.name == name {
			return e, true
		}
	}
	return bsonElem{}, false
}

// asInt64 unifies the numeric BSON types for the $maxTimeMS check (D-S29.1-2).
func asInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), true
	default:
		return 0, false
	}
}

// bsonReader is a little-endian cursor over a byte slice. Every read bounds-checks
// and returns an error on underflow (upstream throw parity).
type bsonReader struct {
	buf []byte
	pos int
}

func (r *bsonReader) readByte() (byte, error) {
	if r.pos+1 > len(r.buf) {
		return 0, errBSON("underflow: byte")
	}
	b := r.buf[r.pos]
	r.pos++
	return b, nil
}

func (r *bsonReader) readBytes(n int) ([]byte, error) {
	if n < 0 || r.pos+n > len(r.buf) {
		return nil, errBSON("underflow: bytes")
	}
	b := r.buf[r.pos : r.pos+n]
	r.pos += n
	return b, nil
}

func (r *bsonReader) readInt32() (int32, error) {
	b, err := r.readBytes(4)
	if err != nil {
		return 0, err
	}
	return int32(binary.LittleEndian.Uint32(b)), nil
}

func (r *bsonReader) readInt64() (int64, error) {
	b, err := r.readBytes(8)
	if err != nil {
		return 0, err
	}
	return int64(binary.LittleEndian.Uint64(b)), nil
}

// readCString reads a NUL-terminated string (the NUL is consumed, not returned).
func (r *bsonReader) readCString() (string, error) {
	for i := r.pos; i < len(r.buf); i++ {
		if r.buf[i] == 0x00 {
			s := string(r.buf[r.pos:i])
			r.pos = i + 1
			return s, nil
		}
	}
	return "", errBSON("unterminated cstring")
}

// parseBSON parses a single top-level BSON document from buf (the test entry
// point; production decode calls parseDocument directly so the codec can size a
// trailing returnFieldsSelector via remaining()). Trailing bytes after the
// document terminator are an error — the document length must account for the
// whole slice.
func parseBSON(buf []byte) (bsonDoc, error) {
	r := &bsonReader{buf: buf}
	d, err := parseDocument(r)
	if err != nil {
		return bsonDoc{}, err
	}
	if r.pos != len(buf) {
		return bsonDoc{}, errBSON("trailing bytes after document")
	}
	return d, nil
}

// parseDocument walks one document: int32 docLength (INCLUDES itself + the
// trailing 0x00) + elements + 0x00. Nested documents recurse (Task 5).
func parseDocument(r *bsonReader) (bsonDoc, error) {
	start := r.pos
	docLen, err := r.readInt32()
	if err != nil {
		return bsonDoc{}, err
	}
	if docLen < 5 { // 4 length bytes + at least the 1-byte terminator
		return bsonDoc{}, errBSON("document length too small")
	}
	end := start + int(docLen)
	if end > len(r.buf) {
		return bsonDoc{}, errBSON("document length exceeds buffer")
	}
	var d bsonDoc
	for {
		if r.pos >= end {
			return bsonDoc{}, errBSON("document not terminated")
		}
		t, err := r.readByte()
		if err != nil {
			return bsonDoc{}, err
		}
		if t == 0x00 { // terminator
			if r.pos != end {
				return bsonDoc{}, errBSON("document terminator misplaced")
			}
			return d, nil
		}
		name, err := r.readCString()
		if err != nil {
			return bsonDoc{}, err
		}
		val, err := parseElementValue(r, t)
		if err != nil {
			return bsonDoc{}, err
		}
		d.elems = append(d.elems, bsonElem{name: name, typ: t, val: val})
	}
}

// parseElementValue decodes one element value by type. Part 1 handles the
// fixed-width scalar types; Part 2 (Task 5) adds the variable-length + nested
// cases (String 0x02, Document 0x03, Array 0x04, Binary 0x05, ObjectId 0x07,
// Regex 0x0B, Symbol 0x0E). ANY other type byte → throw (upstream parity).
func parseElementValue(r *bsonReader, t byte) (any, error) {
	switch t {
	case 0x01: // Double
		b, err := r.readBytes(8)
		if err != nil {
			return nil, err
		}
		return math.Float64frombits(binary.LittleEndian.Uint64(b)), nil
	case 0x08: // Boolean
		b, err := r.readByte()
		if err != nil {
			return nil, err
		}
		return b != 0x00, nil
	case 0x09: // Datetime (int64 ms)
		return r.readInt64()
	case 0x0A: // Null (no bytes)
		return nil, nil
	case 0x10: // Int32
		return r.readInt32()
	case 0x11: // Timestamp (int64)
		return r.readInt64()
	case 0x12: // Int64
		return r.readInt64()
	default:
		// Part 2 (Task 5) inserts the variable-length cases ABOVE this default.
		return nil, errBSON("invalid BSON element type")
	}
}
```

- [ ] **Step 4: Run the part-1 tests — expect PASS**

Run: `go test ./internal/filter/network/mongoproxy/ -run TestBSON -v`
Expected: all PASS.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/mongoproxy/
golangci-lint run ./internal/filter/network/mongoproxy/...
git add internal/filter/network/mongoproxy/ && git commit -m "phase 29.1 IMPL Task 4: bson.go part 1 — reader primitives + document framing + scalar types + throw-on-unknown + underflow"
```

---

## Task 5: `bson.go` part 2 — variable-length + nested types + lookups

**Files:**
- Modify: `internal/filter/network/mongoproxy/bson.go` (insert the variable-length cases)
- Test: `internal/filter/network/mongoproxy/bson_test.go`

- [ ] **Step 1: Write the failing part-2 tests**

Append to `bson_test.go`:
```go
func bstr(s string) []byte { // BSON string: int32 len (incl trailing NUL) + bytes + NUL
	out := leI32(int32(len(s) + 1))
	out = append(out, []byte(s)...)
	return append(out, 0x00)
}

func TestBSON_StringElement(t *testing.T) {
	raw := doc(append(append([]byte{0x02}, cstr("s")...), bstr("hello")...)...)
	d, err := parseBSON(raw)
	if err != nil {
		t.Fatalf("parseBSON: %v", err)
	}
	e, _ := d.find("s")
	if e.val.(string) != "hello" {
		t.Errorf("val = %q, want hello", e.val)
	}
}

func TestBSON_NestedDocument(t *testing.T) {
	// {"_id": {"x": int32(1)}} — a Document-typed _id (the MultiGet shape).
	inner := doc(append(append([]byte{0x10}, cstr("x")...), leI32(1)...)...)
	raw := doc(append(append([]byte{0x03}, cstr("_id")...), inner...)...)
	d, err := parseBSON(raw)
	if err != nil {
		t.Fatalf("parseBSON: %v", err)
	}
	e, ok := d.find("_id")
	if !ok || e.typ != 0x03 {
		t.Fatalf("_id elem = %+v ok=%v", e, ok)
	}
	if _, isDoc := e.val.(bsonDoc); !isDoc {
		t.Errorf("nested _id val type = %T, want bsonDoc", e.val)
	}
}

func TestBSON_ObjectIdAndBinaryAndRegex(t *testing.T) {
	var elems []byte
	elems = append(elems, 0x07) // ObjectId (12 bytes)
	elems = append(elems, cstr("oid")...)
	elems = append(elems, make([]byte, 12)...)
	elems = append(elems, 0x05) // Binary: int32 len + subtype + bytes
	elems = append(elems, cstr("bin")...)
	elems = append(elems, leI32(3)...)
	elems = append(elems, 0x00) // subtype
	elems = append(elems, []byte{1, 2, 3}...)
	elems = append(elems, 0x0B) // Regex: 2 cstrings
	elems = append(elems, cstr("re")...)
	elems = append(elems, cstr("^a")...)
	elems = append(elems, cstr("i")...)
	d, err := parseBSON(doc(elems...))
	if err != nil {
		t.Fatalf("parseBSON: %v", err)
	}
	if len(d.elems) != 3 {
		t.Fatalf("got %d elems, want 3", len(d.elems))
	}
}

func TestBSON_AsInt64(t *testing.T) {
	for _, tc := range []struct {
		v    any
		want int64
		ok   bool
	}{{int32(5), 5, true}, {int64(9), 9, true}, {float64(3.0), 3, true}, {"x", 0, false}} {
		got, ok := asInt64(tc.v)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("asInt64(%v) = %d,%v want %d,%v", tc.v, got, ok, tc.want, tc.ok)
		}
	}
}
```

- [ ] **Step 2: Run them — expect FAIL** (`invalid BSON element type` for 0x02/0x03/0x05/0x07/0x0B; `TestBSON_AsInt64` passes already from Task 4)

Run: `go test ./internal/filter/network/mongoproxy/ -run TestBSON -v`
Expected: the string/nested/objectid tests FAIL.

- [ ] **Step 3: Insert the variable-length cases into `parseElementValue` (above the `default`)**

Add a `readString` reader method to `bson.go`:
```go
// readString reads a BSON string: int32 length (INCLUDES the trailing NUL) +
// bytes + NUL. The returned string excludes the NUL.
func (r *bsonReader) readString() (string, error) {
	n, err := r.readInt32()
	if err != nil {
		return "", err
	}
	if n < 1 {
		return "", errBSON("invalid string length")
	}
	b, err := r.readBytes(int(n))
	if err != nil {
		return "", err
	}
	return string(b[:n-1]), nil // strip the trailing NUL
}
```
Insert these cases into `parseElementValue` (before `default`):
```go
	case 0x02: // String
		return r.readString()
	case 0x0E: // Symbol (same wire shape as String)
		return r.readString()
	case 0x03: // Document (recurse)
		return parseDocument(r)
	case 0x04: // Array (recurse — same wire shape as Document)
		return parseDocument(r)
	case 0x05: // Binary: int32 len + subtype byte + bytes
		n, err := r.readInt32()
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, errBSON("invalid binary length")
		}
		if _, err := r.readByte(); err != nil { // subtype
			return nil, err
		}
		return r.readBytes(int(n))
	case 0x07: // ObjectId (12 bytes)
		return r.readBytes(12)
	case 0x0B: // Regex: two cstrings (pattern + options)
		pat, err := r.readCString()
		if err != nil {
			return nil, err
		}
		opt, err := r.readCString()
		if err != nil {
			return nil, err
		}
		return [2]string{pat, opt}, nil
```

- [ ] **Step 4: Run the BSON tests — expect PASS**

Run: `go test ./internal/filter/network/mongoproxy/ -run TestBSON -v`
Expected: all PASS.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/mongoproxy/
golangci-lint run ./internal/filter/network/mongoproxy/...
git add internal/filter/network/mongoproxy/ && git commit -m "phase 29.1 IMPL Task 5: bson.go part 2 — string/symbol/document/array/binary/objectid/regex + lookups + asInt64"
```

---

## Task 6: `stats.go` — the 23-stat eager roster + dynamic-name helpers

**Files:**
- Create: `internal/filter/network/mongoproxy/stats.go`
- Test: `internal/filter/network/mongoproxy/stats_test.go`

> **PLAN reorder note:** this is the SPEC §10 Task-9 deliverable, moved BEFORE the codec (Tasks 7–9) so the codec can increment a roster that already exists (the 28.1 `config → stats → decoder` order). D-P1: all 23 created EAGERLY at config parse.

- [ ] **Step 1: Write the failing roster + dynamic-name tests**

`stats_test.go`:
```go
package mongoproxy

import (
	"sort"
	"testing"

	"github.com/esalaine/envoy-go/internal/stats"
)

// goldenRoster is the EXACT 22 counter suffixes transcribed from upstream
// ALL_MONGO_PROXY_STATS (parent §7.2 / R2). delays_injected is PLURAL (AMEND-B3
// regression guard). DO NOT edit to match a code change.
var goldenRoster = []string{
	"cx_destroy_local_with_active_rq", "cx_destroy_remote_with_active_rq",
	"cx_drain_close", "decoding_error", "delays_injected",
	"op_command", "op_command_reply", "op_get_more", "op_insert", "op_kill_cursors",
	"op_query", "op_query_await_data", "op_query_exhaust", "op_query_multi_get",
	"op_query_no_cursor_timeout", "op_query_no_max_time", "op_query_scatter_get",
	"op_query_tailable_cursor", "op_reply", "op_reply_cursor_not_found",
	"op_reply_query_failure", "op_reply_valid_cursor",
}

func TestStatRoster_MatchesUpstreamMacro(t *testing.T) {
	got := append([]string(nil), rosterSuffixes()...)
	sort.Strings(got)
	want := append([]string(nil), goldenRoster...)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("roster has %d suffixes, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("suffix[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestStatRoster_EagerCreation(t *testing.T) {
	reg := stats.NewRegistry()
	ms := newMongoStats(reg, "mongo_a")
	// All 22 counters + the gauge exist under mongo.mongo_a. before any traffic,
	// at value 0 (the zk TestRosterStats_EagerCreation pattern — check the struct
	// map directly; the registry exposes no Lookup, only Walk).
	if len(ms.counters) != 22 {
		t.Fatalf("created %d counters, want 22", len(ms.counters))
	}
	for _, suf := range goldenRoster {
		c := ms.counters[suf]
		if c == nil {
			t.Errorf("counter %q not created eagerly", suf)
			continue
		}
		if c.Load() != 0 {
			t.Errorf("counter %q = %d at creation, want 0", suf, c.Load())
		}
	}
	if ms.opQueryActive == nil || ms.opQueryActive.Load() != 0 {
		t.Errorf("gauge op_query_active not created eagerly at 0")
	}
}

func TestStatRoster_IdempotentSharedPrefix(t *testing.T) {
	// Two listeners sharing a stat_prefix share counter instances (no panic) —
	// the zk TestRosterStats_IdempotentSharedPrefix precedent.
	reg := stats.NewRegistry()
	a := newMongoStats(reg, "mongo_a")
	b := newMongoStats(reg, "mongo_a")
	if a.counters["op_query"] != b.counters["op_query"] {
		t.Fatal("shared stat_prefix must share the same counter instances")
	}
}

func TestStatRoster_DynamicNames(t *testing.T) {
	reg := stats.NewRegistry()
	ms := newMongoStats(reg, "p")
	// The helpers register lazily; verify the registered NAME via Counter.Name()
	// (Metric.Name() — the zk dynamic-auth test pattern).
	cases := map[*stats.Counter]string{
		ms.cmdTotal("isMaster"):                          "mongo.p.cmd.isMaster.total",
		ms.collectionQuery("collection1", "scatter_get"): "mongo.p.collection.collection1.query.scatter_get",
		ms.callsiteQuery("collection1", "fixtureFn", "total"): "mongo.p.collection.collection1.callsite.fixtureFn.query.total",
	}
	for c, want := range cases {
		if c.Name() != want {
			t.Errorf("dynamic counter name = %q, want %q", c.Name(), want)
		}
	}
}
```

> **PLAN note:** the registry exposes NO `Lookup` accessor — only `Walk` (`registry.go:134`) + the `New*IfAbsent` constructors (`:157,191`). The tests above therefore inspect the `mongoStats` struct map directly + use `Counter.Load()` / `Counter.Name()` (the as-built `Metric` interface; the zk `stats_test.go:309-345` pattern). The production `stats.go` uses only `reg.NewCounterIfAbsent` / `reg.NewGaugeIfAbsent`.

- [ ] **Step 2: Run them — expect COMPILE failure** (`undefined: rosterSuffixes`, `newMongoStats`)

Run: `go test ./internal/filter/network/mongoproxy/ -run TestStatRoster -v`
Expected: build error.

- [ ] **Step 3: Write `stats.go`**

```go
package mongoproxy

import (
	"fmt"

	"github.com/esalaine/envoy-go/internal/stats"
)

// rosterSuffixes returns the EXACT 22 upstream-macro counter suffixes
// (parent §7.2; ALL_MONGO_PROXY_STATS; R2). delays_injected is PLURAL
// (AMEND-B3). The gauge op_query_active is created separately (newMongoStats).
// Construction order is not sorted; the R2 test sorts before comparing.
func rosterSuffixes() []string {
	return []string{
		"cx_destroy_local_with_active_rq", "cx_destroy_remote_with_active_rq",
		"cx_drain_close", "decoding_error", "delays_injected",
		"op_command", "op_command_reply", "op_get_more", "op_insert", "op_kill_cursors",
		"op_query", "op_query_await_data", "op_query_exhaust", "op_query_multi_get",
		"op_query_no_cursor_timeout", "op_query_no_max_time", "op_query_scatter_get",
		"op_query_tailable_cursor", "op_reply", "op_reply_cursor_not_found",
		"op_reply_query_failure", "op_reply_valid_cursor",
	}
}

// mongoStats holds the 23 eagerly-created fixed stats (22 counters + the
// op_query_active gauge; D-P1 creation parity — created once per distinct
// stat_prefix at config parse) plus the lazily-created dynamic counter families.
type mongoStats struct {
	prefix        string // "mongo.<stat_prefix>."
	reg           *stats.Registry
	counters      map[string]*stats.Counter // 22, keyed by suffix; all eager
	opQueryActive *stats.Gauge
}

// newMongoStats eagerly creates all 23 fixed stats under mongo.<statPrefix>. via
// NewCounterIfAbsent / NewGaugeIfAbsent — post-Freeze-permitted and idempotent
// across listeners sharing a stat_prefix (the zookeeper newRosterStats precedent;
// D-P1). The boot-window departure vs upstream's per-connection creation is a
// BEHAVIOR_CONTRACT record (§7.5), unobservable to the differential.
func newMongoStats(reg *stats.Registry, statPrefix string) *mongoStats {
	ms := &mongoStats{
		prefix:   "mongo." + statPrefix + ".",
		reg:      reg,
		counters: make(map[string]*stats.Counter, 22),
	}
	for _, suf := range rosterSuffixes() {
		ms.counters[suf] = reg.NewCounterIfAbsent(ms.prefix + suf)
	}
	ms.opQueryActive = reg.NewGaugeIfAbsent(ms.prefix + "op_query_active")
	return ms
}

// inc increments the fixed counter for suffix. Unknown suffix is a programming
// error → panic (the roster is closed; dynamic names go through the helpers).
func (ms *mongoStats) inc(suffix string) {
	c, ok := ms.counters[suffix]
	if !ok {
		panic(fmt.Sprintf("mongoproxy: unknown roster suffix %q", suffix))
	}
	c.Inc()
}

// cmdTotal returns mongo.<sp>.cmd.<cmd>.total (lazy; post-Freeze-permitted —
// the zookeeper auth.<scheme>_rq / rbac per-policy precedent). <cmd> is the
// remembered name or "unknown_command".
func (ms *mongoStats) cmdTotal(cmd string) *stats.Counter {
	return ms.reg.NewCounterIfAbsent(ms.prefix + "cmd." + cmd + ".total")
}

// collectionQuery returns mongo.<sp>.collection.<c>.query.<leaf> (leaf ∈
// total / scatter_get / multi_get).
func (ms *mongoStats) collectionQuery(c, leaf string) *stats.Counter {
	return ms.reg.NewCounterIfAbsent(ms.prefix + "collection." + c + ".query." + leaf)
}

// callsiteQuery returns mongo.<sp>.collection.<c>.callsite.<cs>.query.<leaf>
// (the AMEND-C3 double-count family — incremented IN ADDITION to collectionQuery).
func (ms *mongoStats) callsiteQuery(c, cs, leaf string) *stats.Counter {
	return ms.reg.NewCounterIfAbsent(ms.prefix + "collection." + c + ".callsite." + cs + ".query." + leaf)
}
```

- [ ] **Step 4: Run the roster tests — expect PASS**

Run: `go test ./internal/filter/network/mongoproxy/ -run TestStatRoster -v`
Expected: all PASS (incl. the 22-name golden + eager + dynamic).

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/mongoproxy/
golangci-lint run ./internal/filter/network/mongoproxy/...
git add internal/filter/network/mongoproxy/ && git commit -m "phase 29.1 IMPL Task 6: stats.go — 23-stat eager roster (22 counters + op_query_active gauge) + dynamic cmd/collection/callsite helpers"
```

---

## Task 7: `codec.go` part 1 — MsgHeader framing + reassembly + opcode dispatch + the decoding_error/sniffing-off path

**Files:**
- Create: `internal/filter/network/mongoproxy/codec.go`
- Test: `internal/filter/network/mongoproxy/codec_test.go`

> Mirrors upstream `codec_impl.cc:344-426` (parent §11.4). Part 1 lands the decoder struct + the chainConsumed high-water feed (D-S29.1-4) + MsgHeader framing + partial-frame reassembly + the 7-opcode dispatch (incl. Reply/CommandReply recognized-not-decoded + the OP_MSG/Update/Delete/Msg→decoding_error path) + the at-most-once decoding_error + sniffing-off discipline (D-S29.1-6). The per-opcode body decoders are STUBBED here (inc the primary op counter, return true) and fleshed out at Tasks 8–9.

- [ ] **Step 1: Write the failing part-1 tests**

`codec_test.go`:
```go
package mongoproxy

import (
	"testing"

	"github.com/esalaine/envoy-go/internal/stats"
)

// newTestDecoder wires a decoder over a fresh roster (stat_prefix "p").
func newTestDecoder(t *testing.T, commands ...string) (*decoder, *mongoStats) {
	t.Helper()
	reg := stats.NewRegistry()
	cfg := &compiledConfig{statPrefix: "p", commands: map[string]bool{}}
	if len(commands) == 0 {
		commands = defaultCommands
	}
	for _, c := range commands {
		cfg.commands[c] = true
	}
	ms := newMongoStats(reg, "p")
	cfg.stats = ms
	return newDecoder(cfg, ms), ms
}

// msg builds a full mongo wire message: 16-byte LE header + body. messageLength
// includes the header. (The 0049 driver builders at Task 14 generalize this.)
func msg(reqID, opCode int32, body []byte) []byte {
	total := int32(16 + len(body))
	out := append(leI32(total), leI32(reqID)...)
	out = append(out, leI32(0)...)       // responseTo
	out = append(out, leI32(opCode)...)  // opCode
	return append(out, body...)
}

func TestCodec_OpMsgIsDecodingError(t *testing.T) {
	d, ms := newTestDecoder(t)
	d.decodeOnData(msg(1, 2013, nil), int64(len(msg(1, 2013, nil)))) // OP_MSG
	if ms.counters["decoding_error"].Load() != 1 {
		t.Fatalf("decoding_error = %d, want 1", ms.counters["decoding_error"].Load())
	}
	if d.sniffing {
		t.Errorf("sniffing must be false after a decode error")
	}
}

func TestCodec_DecodingErrorAtMostOncePerConnection(t *testing.T) {
	d, ms := newTestDecoder(t)
	// First an OP_MSG (error), then a valid-looking second frame on the SAME
	// connection: sniffing is off → the second frame is dropped, NO 2nd error.
	frame1 := msg(1, 2013, nil)
	frame2 := msg(2, 2013, nil)
	both := append(frame1, frame2...)
	d.decodeOnData(both, int64(len(both)))
	if ms.counters["decoding_error"].Load() != 1 {
		t.Errorf("decoding_error = %d, want exactly 1 (AMEND-B6 / D-S29.1-6)", ms.counters["decoding_error"].Load())
	}
}

func TestCodec_ReplyAndCommandReplyRecognizedNotDecoded(t *testing.T) {
	d, ms := newTestDecoder(t)
	// Feed BOTH frames with the correct cumulative totalAppended so they actually
	// route through decodeMessage (a totalAppended of 0 would append nothing to
	// readBuf and the test would pass vacuously). Reply(1) + CommandReply(2011).
	full := append(msg(1, 1, []byte{0x01, 0x02, 0x03}), msg(2, 2011, []byte{0x04, 0x05})...)
	d.decodeOnData(full, int64(len(full)))
	// recognized-not-decoded: NO decoding_error, sniffing stays on, no counters,
	// and both frames are fully consumed from readBuf (proving dispatch ran).
	if ms.counters["decoding_error"].Load() != 0 {
		t.Errorf("Reply/CommandReply must not error; got %d", ms.counters["decoding_error"].Load())
	}
	if !d.sniffing {
		t.Errorf("sniffing must stay on after recognized-not-decoded opcodes")
	}
	if len(d.readBuf) != 0 {
		t.Errorf("both recognized frames must be consumed; readBuf has %d bytes left", len(d.readBuf))
	}
}

func TestCodec_PartialFrameReassembly(t *testing.T) {
	d, ms := newTestDecoder(t)
	full := msg(1, 2004, opQueryBody("db.c", 0, simpleQuery())) // a valid OP_QUERY
	// Feed the first 10 bytes (partial header) — nothing decoded yet.
	d.decodeOnData(full[:10], 10)
	if ms.counters["op_query"].Load() != 0 {
		t.Fatalf("op_query fired on a partial frame")
	}
	// Feed the rest — now the complete frame decodes exactly once.
	d.decodeOnData(full, int64(len(full)))
	if ms.counters["op_query"].Load() != 1 {
		t.Errorf("op_query = %d after full frame, want 1", ms.counters["op_query"].Load())
	}
}

func TestCodec_MultiReadNoDoubleCount(t *testing.T) {
	d, ms := newTestDecoder(t)
	full := msg(1, 2004, opQueryBody("db.c", 0, simpleQuery()))
	// Two OnData calls with the CUMULATIVE chain slice (the 28.1b re-base feed):
	// totalAppended advances; only the new tail bytes are appended to readBuf.
	d.decodeOnData(full, int64(len(full)))
	d.decodeOnData(full, int64(len(full))) // same totalAppended → no new bytes
	if ms.counters["op_query"].Load() != 1 {
		t.Errorf("op_query = %d, want 1 (no double-count across reads)", ms.counters["op_query"].Load())
	}
}
```

> `opQueryBody` / `simpleQuery` are defined at Task 8 (they build a real OP_QUERY body); for Task 7's partial/multi-read tests they need only produce a structurally-valid body that the Task-7 STUB `decodeQuery` accepts (the stub ignores the body). Land minimal versions in `codec_test.go` now and the real builders fold in at Task 8. Minimal:
> ```go
> func simpleQuery() []byte { return doc(append(append([]byte{0x10}, cstr("a")...), leI32(1)...)...) }
> func opQueryBody(fullColl string, flags int32, queryDoc []byte) []byte {
> 	out := append(leI32(flags), cstr(fullColl)...)
> 	out = append(out, leI32(0)...) // numberToSkip
> 	out = append(out, leI32(0)...) // numberToReturn
> 	return append(out, queryDoc...)
> }
> ```

- [ ] **Step 2: Run them — expect COMPILE failure** (`undefined: newDecoder` etc.)

Run: `go test ./internal/filter/network/mongoproxy/ -run TestCodec -v`
Expected: build error.

- [ ] **Step 3: Write `codec.go` part 1**

```go
package mongoproxy

import (
	"encoding/binary"
	"time"
)

// Wire opcodes (parent §11.4; codec.h:24-35 + the modern OP_MSG=2013). The
// dispatch decodes EXACTLY 7 (Reply, Query, GetMore, Insert, KillCursors,
// Command, CommandReply); everything else → decoding_error (AMEND-B5).
const (
	opReply        int32 = 1
	opUpdate       int32 = 2001
	opInsert       int32 = 2002
	opQuery        int32 = 2004
	opGetMore      int32 = 2005
	opDelete       int32 = 2006
	opKillCursors  int32 = 2007
	opCommand      int32 = 2010
	opCommandReply int32 = 2011
	opMsg          int32 = 2013
)

// activeQuery carries what 29.2's correlation + the dynamic reply-side stats
// need. Written at 29.1 on every decoded OP_QUERY; never read at 29.1 (R5).
type activeQuery struct {
	requestID  int32
	collection string
	command    string    // empty for non-$cmd queries
	callsite   string    // empty unless a $comment callsite was present
	start      time.Time // recorded at 29.1 (cheap; avoids a 29.2 struct revision)
}

// decoder is the per-connection request-side wire decoder. It owns its OWN
// readBuf (the chain Buffer is read, NEVER drained — R3). chainConsumed is the
// high-water mark against Buffer.TotalAppended() (D-S29.1-4; the
// zookeeperproxy/decoder.go:40-48,99-104 mechanism adapted verbatim). The
// active-query list lives HERE (not on the filter — PLAN disposition; the
// zookeeper requestsByXid-on-the-decoder precedent), so the codec is unit-testable
// in isolation. NO mutex at 29.1 (single-goroutine request path; the ADR-0223
// per-connection mutex arrives at 29.2 with the cross-goroutine OnWrite reader).
type decoder struct {
	cfg      *compiledConfig
	stats    *mongoStats
	chainConsumed int64
	readBuf  []byte
	sniffing bool // starts true; set false on the first decode error (lifetime)
	queries  []activeQuery
}

// newDecoder returns a fresh per-connection decoder (sniffing on).
func newDecoder(cfg *compiledConfig, ms *mongoStats) *decoder {
	return &decoder{cfg: cfg, stats: ms, sniffing: true}
}

// decodeOnData feeds the chain-buffer's NEW bytes (the trailing
// totalAppended−chainConsumed bytes) into readBuf and decodes every complete
// message. Once sniffing is off it only advances chainConsumed and drops bytes
// (AMEND-B6). It NEVER drains the chain buffer, never closes, never halts (R3).
func (d *decoder) decodeOnData(chainBytes []byte, totalAppended int64) {
	if !d.sniffing {
		d.chainConsumed = totalAppended
		d.readBuf = nil
		return
	}
	if newCount := totalAppended - d.chainConsumed; newCount > 0 {
		d.readBuf = append(d.readBuf, chainBytes[int64(len(chainBytes))-newCount:]...)
		d.chainConsumed = totalAppended
	}
	for {
		m, ok := d.nextMessage()
		if !ok {
			return // no complete frame buffered (or sniffing went off mid-loop)
		}
		if !d.decodeMessage(m) {
			return // decoding_error path already ran; sniffing now off
		}
	}
}

// nextMessage extracts one complete wire message from readBuf (header + body;
// messageLength INCLUDES the 16-byte header). Returns ok=false on a partial
// frame (wait for more — never an error). A malformed length (< 16) → decode error.
func (d *decoder) nextMessage() ([]byte, bool) {
	if len(d.readBuf) < 16 {
		return nil, false
	}
	msgLen := int32(binary.LittleEndian.Uint32(d.readBuf[0:4]))
	if msgLen < 16 {
		d.decoderError()
		return nil, false
	}
	if int64(len(d.readBuf)) < int64(msgLen) {
		return nil, false // partial frame — wait for more bytes
	}
	m := d.readBuf[:msgLen]
	d.readBuf = d.readBuf[msgLen:]
	return m, true
}

// decodeMessage parses the MsgHeader and dispatches by opcode (AMEND-B5).
// Returns false on a decode failure (the decoding_error path has already run).
func (d *decoder) decodeMessage(m []byte) bool {
	requestID := int32(binary.LittleEndian.Uint32(m[4:8]))
	opCode := int32(binary.LittleEndian.Uint32(m[12:16]))
	body := m[16:]
	switch opCode {
	case opQuery:
		return d.decodeQuery(requestID, body)
	case opInsert:
		return d.decodeInsert(body)
	case opGetMore:
		return d.decodeGetMore(body)
	case opKillCursors:
		return d.decodeKillCursors(body)
	case opCommand:
		return d.decodeCommand(body)
	case opReply, opCommandReply:
		// recognized-not-decoded at 29.1 (§1.2): valid envelope → NOT an error;
		// the frame is consumed; body decode + counters land at 29.2.
		return true
	default:
		// Msg(1000)/Update(2001)/Delete(2006)/OP_MSG(2013)/anything else → throw
		// (upstream "invalid mongo op N" parity).
		d.decoderError()
		return false
	}
}

// decoderError increments decoding_error AT MOST ONCE per connection and turns
// sniffing off for the connection lifetime (D-S29.1-6; the flag + counter are
// co-located + mutated together). The private buffer is released.
func (d *decoder) decoderError() {
	if !d.sniffing {
		return // at-most-once
	}
	d.stats.inc("decoding_error")
	d.sniffing = false
	d.readBuf = nil
}

// fail is the codec's error shorthand: take the decoding_error path, return false.
func (d *decoder) fail() bool { d.decoderError(); return false }

// --- STUB body decoders (Task 7) — replaced with full decode at Tasks 8–9. ---

func (d *decoder) decodeQuery(requestID int32, body []byte) bool { d.stats.inc("op_query"); return true }
func (d *decoder) decodeInsert(body []byte) bool                 { d.stats.inc("op_insert"); return true }
func (d *decoder) decodeGetMore(body []byte) bool                { d.stats.inc("op_get_more"); return true }
func (d *decoder) decodeKillCursors(body []byte) bool            { d.stats.inc("op_kill_cursors"); return true }
func (d *decoder) decodeCommand(body []byte) bool                { d.stats.inc("op_command"); return true }
```

> The stub signatures match the final ones (Tasks 8–9 only replace the bodies). `requestID`/`body` params are unused in the stubs — add `//nolint:unparam` or `_ = body` if the linter complains, OR accept the Task-8/9 fill-in lands within the same PR window so the unused-param window is brief; simplest is to keep the params named and let Tasks 8–9 use them (golangci's `unparam` tolerates this for unexported methods with a forthcoming consumer — verify at Step 4 lint).

- [ ] **Step 4: Run the part-1 tests + lint — expect PASS / clean**

Run: `go test ./internal/filter/network/mongoproxy/ -run TestCodec -v && golangci-lint run ./internal/filter/network/mongoproxy/...`
Expected: all PASS; lint clean (if `unparam` flags the stubs, the Task-8/9 fill-ins resolve it — note in PROGRESS and proceed; the six-gate at Task 17 is the binding lint gate).

- [ ] **Step 5: gofmt + commit**

```bash
gofmt -l internal/filter/network/mongoproxy/
git add internal/filter/network/mongoproxy/ && git commit -m "phase 29.1 IMPL Task 7: codec.go part 1 — MsgHeader framing + reassembly + 7-opcode dispatch + decoding_error/sniffing-off (Reply/CommandReply recognized-not-decoded)"
```

---

## Task 8: `codec.go` part 2 — OP_QUERY body decode (flags + collection + command + query-shape + maxTime + comment)

**Files:**
- Modify: `internal/filter/network/mongoproxy/codec.go` (replace the `decodeQuery` stub; add the shape/maxTime/callsite helpers)
- Modify: `internal/filter/network/mongoproxy/bson.go` (add `remaining()` + `import "strings"`/`"encoding/json"` land in codec.go)
- Test: `internal/filter/network/mongoproxy/codec_test.go`

- [ ] **Step 1: Write the failing OP_QUERY tests (the §8.1.3 arm expectations as unit tests)**

Append to `codec_test.go`:
```go
// helpers building richer query docs
func queryWithID(idType byte, idVal []byte) []byte {
	return doc(append(append([]byte{idType}, cstr("_id")...), idVal...)...)
}

func TestDecodeQuery_PlainScatterGet(t *testing.T) {
	// arm 1: db.collection1 {a:1}, no _id, no maxTime.
	d, ms := newTestDecoder(t)
	full := msg(1, 2004, opQueryBody("db.collection1", 0, simpleQuery()))
	d.decodeOnData(full, int64(len(full)))
	for _, suf := range []string{"op_query", "op_query_scatter_get", "op_query_no_max_time"} {
		if ms.counters[suf].Load() != 1 {
			t.Errorf("%s = %d, want 1", suf, ms.counters[suf].Load())
		}
	}
	if ms.collectionQuery("collection1", "total").Load() != 1 {
		t.Errorf("collection1.query.total != 1")
	}
	if ms.collectionQuery("collection1", "scatter_get").Load() != 1 {
		t.Errorf("collection1.query.scatter_get != 1")
	}
	if len(d.queries) != 1 || d.queries[0].collection != "collection1" {
		t.Errorf("active-query list not populated: %+v", d.queries)
	}
}

func TestDecodeQuery_PrimaryKeyScalarID(t *testing.T) {
	// arm 3a: {_id: 7} scalar → only query.total (no scatter/multi).
	d, ms := newTestDecoder(t)
	full := msg(1, 2004, opQueryBody("db.collection1", 0, queryWithID(0x10, leI32(7))))
	d.decodeOnData(full, int64(len(full)))
	if ms.collectionQuery("collection1", "total").Load() != 1 {
		t.Errorf("query.total != 1")
	}
	if ms.counters["op_query_scatter_get"].Load() != 0 || ms.counters["op_query_multi_get"].Load() != 0 {
		t.Errorf("scalar _id must be PrimaryKey (no scatter/multi)")
	}
}

func TestDecodeQuery_MultiGetDocumentID(t *testing.T) {
	// arm 3b: {_id: {x:1}} Document-typed → MultiGet.
	d, ms := newTestDecoder(t)
	inner := doc(append(append([]byte{0x10}, cstr("x")...), leI32(1)...)...)
	full := msg(1, 2004, opQueryBody("db.collection1", 0, queryWithID(0x03, inner)))
	d.decodeOnData(full, int64(len(full)))
	if ms.counters["op_query_multi_get"].Load() != 1 || ms.collectionQuery("collection1", "multi_get").Load() != 1 {
		t.Errorf("Document-typed _id must be MultiGet")
	}
}

func TestDecodeQuery_FlagCounters(t *testing.T) {
	// arm 3c: flags 0x02|0x10|0x20|0x40 → the four flag counters.
	d, ms := newTestDecoder(t)
	full := msg(1, 2004, opQueryBody("db.c", 0x02|0x10|0x20|0x40, simpleQuery()))
	d.decodeOnData(full, int64(len(full)))
	for _, suf := range []string{"op_query_tailable_cursor", "op_query_no_cursor_timeout", "op_query_await_data", "op_query_exhaust"} {
		if ms.counters[suf].Load() != 1 {
			t.Errorf("%s = %d, want 1", suf, ms.counters[suf].Load())
		}
	}
}

func TestDecodeQuery_CmdIsMasterAndUnknown(t *testing.T) {
	// arm 2: commands:[isMaster]. {isMaster:1} → cmd.isMaster.total; {foo:1} → unknown.
	d, ms := newTestDecoder(t, "isMaster")
	cmdDoc := func(name string) []byte { return doc(append(append([]byte{0x10}, cstr(name)...), leI32(1)...)...) }
	f1 := msg(1, 2004, opQueryBody("admin.$cmd", 0, cmdDoc("isMaster")))
	f2 := msg(2, 2004, opQueryBody("admin.$cmd", 0, cmdDoc("foo")))
	both := append(f1, f2...)
	d.decodeOnData(both, int64(len(both)))
	if ms.cmdTotal("isMaster").Load() != 1 {
		t.Errorf("cmd.isMaster.total != 1")
	}
	if ms.cmdTotal("unknown_command").Load() != 1 {
		t.Errorf("cmd.unknown_command.total != 1")
	}
	if ms.counters["op_query"].Load() != 2 {
		t.Errorf("op_query = %d, want 2", ms.counters["op_query"].Load())
	}
	if ms.counters["op_query_no_max_time"].Load() != 0 {
		t.Errorf("$cmd queries must NOT increment op_query_no_max_time (§11.2)")
	}
}

func TestDecodeQuery_CallsiteDoubleCount(t *testing.T) {
	// arm 5: {a:1, $comment:"{\"callingFunction\":\"fixtureFn\"}"} → callsite + plain.
	d, ms := newTestDecoder(t)
	var q []byte
	q = append(q, 0x10)
	q = append(q, cstr("a")...)
	q = append(q, leI32(1)...)
	q = append(q, 0x02)
	q = append(q, cstr("$comment")...)
	q = append(q, bstr(`{"callingFunction": "fixtureFn"}`)...)
	full := msg(1, 2004, opQueryBody("db.collection1", 0, doc(q...)))
	d.decodeOnData(full, int64(len(full)))
	if ms.collectionQuery("collection1", "total").Load() != 1 {
		t.Errorf("plain collection.query.total != 1")
	}
	if ms.callsiteQuery("collection1", "fixtureFn", "total").Load() != 1 {
		t.Errorf("callsite query.total != 1 (AMEND-C3 double-count)")
	}
	if ms.callsiteQuery("collection1", "fixtureFn", "scatter_get").Load() != 1 {
		t.Errorf("callsite query.scatter_get != 1")
	}
	if d.queries[0].callsite != "fixtureFn" {
		t.Errorf("active-query callsite = %q, want fixtureFn", d.queries[0].callsite)
	}
}

func TestDecodeQuery_NoDotCollectionIsError(t *testing.T) {
	d, ms := newTestDecoder(t)
	full := msg(1, 2004, opQueryBody("nodotcollection", 0, simpleQuery()))
	d.decodeOnData(full, int64(len(full)))
	if ms.counters["decoding_error"].Load() != 1 {
		t.Errorf("a fullCollectionName with no dot must be a decoding_error")
	}
}

func TestDecodeQuery_EmptyCmdDocIsError(t *testing.T) {
	d, ms := newTestDecoder(t)
	full := msg(1, 2004, opQueryBody("admin.$cmd", 0, doc())) // empty doc
	d.decodeOnData(full, int64(len(full)))
	if ms.counters["decoding_error"].Load() != 1 {
		t.Errorf("an empty $cmd document must be a decoding_error")
	}
}
```

- [ ] **Step 2: Run them — expect FAIL** (the stub `decodeQuery` only increments `op_query`)

Run: `go test ./internal/filter/network/mongoproxy/ -run TestDecodeQuery -v`
Expected: the shape/cmd/callsite/error tests FAIL.

- [ ] **Step 3: Add `remaining()` to `bson.go`, replace the `decodeQuery` stub in `codec.go`**

Add to `bson.go`:
```go
// remaining returns the unread byte count (used by the codec to detect an
// optional trailing returnFieldsSelector after the OP_QUERY query document).
func (r *bsonReader) remaining() int { return len(r.buf) - r.pos }
```

Replace the `decodeQuery` stub in `codec.go` (and add `"encoding/json"` + `"strings"` to the import block):
```go
// decodeQuery decodes an OP_QUERY body (parent §11.4): flags(int32) →
// fullCollectionName(cstring) → numberToSkip(int32) → numberToReturn(int32) →
// query(BSON doc) → OPTIONAL returnFieldsSelector(BSON doc, iff bytes remain).
func (d *decoder) decodeQuery(requestID int32, body []byte) bool {
	r := &bsonReader{buf: body}
	flags, err := r.readInt32()
	if err != nil {
		return d.fail()
	}
	fullColl, err := r.readCString()
	if err != nil {
		return d.fail()
	}
	if _, err := r.readInt32(); err != nil { // numberToSkip
		return d.fail()
	}
	if _, err := r.readInt32(); err != nil { // numberToReturn
		return d.fail()
	}
	queryDoc, err := parseDocument(r)
	if err != nil {
		return d.fail()
	}
	if r.remaining() > 0 { // optional returnFieldsSelector — parse to validate
		if _, err := parseDocument(r); err != nil {
			return d.fail()
		}
	}

	d.stats.inc("op_query")
	if flags&0x02 != 0 {
		d.stats.inc("op_query_tailable_cursor")
	}
	if flags&0x10 != 0 {
		d.stats.inc("op_query_no_cursor_timeout")
	}
	if flags&0x20 != 0 {
		d.stats.inc("op_query_await_data")
	}
	if flags&0x40 != 0 {
		d.stats.inc("op_query_exhaust")
	}

	dot := strings.IndexByte(fullColl, '.')
	if dot < 0 {
		return d.fail() // "invalid full collection name" parity
	}
	collection := fullColl[dot+1:]
	aq := activeQuery{requestID: requestID, collection: collection, start: time.Now()}

	if strings.Contains(fullColl, "$cmd") {
		cmdDoc := queryDoc
		if q, ok := queryDoc.find("$query"); ok {
			if nested, ok := q.val.(bsonDoc); ok {
				cmdDoc = nested
			}
		}
		first, ok := cmdDoc.first()
		if !ok {
			return d.fail() // empty $cmd doc → "invalid query command" parity
		}
		name := normalizeCommand(first.name)
		if name != "" {
			if !d.cfg.commands[name] {
				name = "unknown_command"
			}
			d.stats.cmdTotal(name).Inc()
			aq.command = name
			d.queries = append(d.queries, aq)
			return true
		}
		// name == "" → "find": route to the query path on the command document
		// (AMEND-B7; upstream utility.cc routes find to collection stats). The
		// exact find-collection extraction is an IMPL upstream-transcription
		// detail (no 29.1 fixture exercises find); the query-shape path below is
		// the faithful default.
		queryDoc = cmdDoc
	}

	// non-command query path
	leaves, opShape := queryShape(queryDoc)
	for _, leaf := range leaves {
		d.stats.collectionQuery(collection, leaf).Inc()
	}
	if opShape != "" {
		d.stats.inc(opShape)
	}
	if maxTimeLessThanOne(queryDoc) {
		d.stats.inc("op_query_no_max_time")
	}
	if cs := callsiteName(queryDoc); cs != "" {
		aq.callsite = cs
		for _, leaf := range leaves {
			d.stats.callsiteQuery(collection, cs, leaf).Inc()
		}
	}
	d.queries = append(d.queries, aq)
	return true
}

// queryShape classifies a non-command query (parent §11.3): no _id → ScatterGet;
// Document/Array _id → MultiGet; scalar _id → PrimaryKey. Returns the collection
// leaf set (always incl. "total") + the op_query_* shape counter ("" for
// PrimaryKey). The callsite family double-counts the SAME leaves (AMEND-C3).
func queryShape(queryDoc bsonDoc) (leaves []string, opCounter string) {
	leaves = []string{"total"}
	idElem, hasID := queryDoc.find("_id")
	switch {
	case !hasID:
		return append(leaves, "scatter_get"), "op_query_scatter_get"
	case idElem.typ == 0x03 || idElem.typ == 0x04: // Document or Array
		return append(leaves, "multi_get"), "op_query_multi_get"
	default:
		return leaves, "" // scalar _id → PrimaryKey: only total
	}
}

// maxTimeLessThanOne returns true when the query's $maxTimeMS (fallback
// maxTimeMS; Int32/Int64/Double) is < 1 — including absent (defaults to 0).
// Non-command queries with maxTime < 1 → op_query_no_max_time (§11.2).
func maxTimeLessThanOne(queryDoc bsonDoc) bool {
	for _, key := range []string{"$maxTimeMS", "maxTimeMS"} {
		if e, ok := queryDoc.find(key); ok {
			if v, ok := asInt64(e.val); ok {
				return v < 1
			}
		}
	}
	return true // absent → 0 → < 1
}

// callsiteName extracts the $comment callsite (AMEND-C3): $comment (String)
// parsed as JSON → field "callingFunction". Any parse failure → "" (no callsite).
func callsiteName(queryDoc bsonDoc) string {
	e, ok := queryDoc.find("$comment")
	if !ok {
		return ""
	}
	s, ok := e.val.(string)
	if !ok {
		return ""
	}
	var parsed struct {
		CallingFunction string `json:"callingFunction"`
	}
	if err := json.Unmarshal([]byte(s), &parsed); err != nil {
		return ""
	}
	return parsed.CallingFunction
}
```

- [ ] **Step 4: Run the OP_QUERY tests — expect PASS**

Run: `go test ./internal/filter/network/mongoproxy/ -run 'TestDecodeQuery|TestCodec' -v`
Expected: all PASS.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/mongoproxy/
golangci-lint run ./internal/filter/network/mongoproxy/...
git add internal/filter/network/mongoproxy/ && git commit -m "phase 29.1 IMPL Task 8: codec.go part 2 — OP_QUERY decode (flags, collection, $cmd command, query-shape, \$maxTimeMS, \$comment callsite double-count, active-query append)"
```

---

## Task 9: `codec.go` part 3 — OP_INSERT / OP_GET_MORE / OP_KILL_CURSORS / OP_COMMAND body decode

**Files:**
- Modify: `internal/filter/network/mongoproxy/codec.go` (replace the four remaining stubs)
- Test: `internal/filter/network/mongoproxy/codec_test.go`

> These four opcodes have only their primary op counter at 29.1 (no sub-counters; parent §7.2). Their body decode VALIDATES structure (a malformed body → decoding_error) and consumes bytes; none append to the active-query list (only OP_QUERY does — parent §11.4 item 7).

- [ ] **Step 1: Write the failing tests**

Append to `codec_test.go`:
```go
func TestDecodeInsert(t *testing.T) {
	d, ms := newTestDecoder(t)
	// flags(int32) + fullCollectionName(cstring) + 1 BSON doc
	body := append(leI32(0), cstr("db.c")...)
	body = append(body, simpleQuery()...)
	full := msg(1, 2002, body)
	d.decodeOnData(full, int64(len(full)))
	if ms.counters["op_insert"].Load() != 1 {
		t.Errorf("op_insert != 1")
	}
	if len(d.queries) != 0 {
		t.Errorf("OP_INSERT must NOT append to the active-query list")
	}
}

func TestDecodeGetMore(t *testing.T) {
	d, ms := newTestDecoder(t)
	// ZERO(int32) + fullCollectionName(cstring) + numberToReturn(int32) + cursorID(int64)
	body := append(leI32(0), cstr("db.c")...)
	body = append(body, leI32(10)...)
	body = append(body, leI64(12345)...)
	full := msg(1, 2005, body)
	d.decodeOnData(full, int64(len(full)))
	if ms.counters["op_get_more"].Load() != 1 {
		t.Errorf("op_get_more != 1")
	}
}

func TestDecodeKillCursors(t *testing.T) {
	d, ms := newTestDecoder(t)
	// ZERO(int32) + numberOfCursorIDs(int32) + cursorIDs(int64 each)
	body := append(leI32(0), leI32(2)...)
	body = append(body, leI64(1)...)
	body = append(body, leI64(2)...)
	full := msg(1, 2007, body)
	d.decodeOnData(full, int64(len(full)))
	if ms.counters["op_kill_cursors"].Load() != 1 {
		t.Errorf("op_kill_cursors != 1")
	}
}

func TestDecodeCommand(t *testing.T) {
	d, ms := newTestDecoder(t)
	// database(cstring) + commandName(cstring) + metadata(BSON) + commandArgs(BSON)
	body := append(cstr("admin"), cstr("ping")...)
	body = append(body, doc()...)          // metadata (empty)
	body = append(body, simpleQuery()...)  // commandArgs
	full := msg(1, 2010, body)
	d.decodeOnData(full, int64(len(full)))
	if ms.counters["op_command"].Load() != 1 {
		t.Errorf("op_command != 1")
	}
}

func TestDecodeInsert_MalformedIsError(t *testing.T) {
	d, ms := newTestDecoder(t)
	// flags + collection but a truncated BSON doc.
	body := append(leI32(0), cstr("db.c")...)
	body = append(body, leI32(99)...) // claims a 99-byte doc, none follows
	full := msg(1, 2002, body)
	d.decodeOnData(full, int64(len(full)))
	if ms.counters["decoding_error"].Load() != 1 {
		t.Errorf("a malformed OP_INSERT doc must be a decoding_error")
	}
}
```

- [ ] **Step 2: Run them — expect FAIL** (`TestDecodeInsert_MalformedIsError` fails: the stub never validates; `len(d.queries)` check fails if stub appended — it does not, so that part may pass)

Run: `go test ./internal/filter/network/mongoproxy/ -run 'TestDecodeInsert|TestDecodeGetMore|TestDecodeKillCursors|TestDecodeCommand' -v`
Expected: the malformed-insert test FAILs (no validation in the stub).

- [ ] **Step 3: Replace the four stubs in `codec.go`**

```go
// decodeInsert: flags(int32) → fullCollectionName(cstring) → 1..N BSON docs
// (loop to end of body). Validate-and-consume; op_insert.
func (d *decoder) decodeInsert(body []byte) bool {
	r := &bsonReader{buf: body}
	if _, err := r.readInt32(); err != nil { // flags
		return d.fail()
	}
	if _, err := r.readCString(); err != nil { // fullCollectionName
		return d.fail()
	}
	for r.remaining() > 0 {
		if _, err := parseDocument(r); err != nil {
			return d.fail()
		}
	}
	d.stats.inc("op_insert")
	return true
}

// decodeGetMore: ZERO(int32) → fullCollectionName(cstring) → numberToReturn(int32)
// → cursorID(int64). op_get_more.
func (d *decoder) decodeGetMore(body []byte) bool {
	r := &bsonReader{buf: body}
	if _, err := r.readInt32(); err != nil { // ZERO
		return d.fail()
	}
	if _, err := r.readCString(); err != nil { // fullCollectionName
		return d.fail()
	}
	if _, err := r.readInt32(); err != nil { // numberToReturn
		return d.fail()
	}
	if _, err := r.readInt64(); err != nil { // cursorID
		return d.fail()
	}
	d.stats.inc("op_get_more")
	return true
}

// decodeKillCursors: ZERO(int32) → numberOfCursorIDs(int32) → cursorIDs(int64…).
// op_kill_cursors.
func (d *decoder) decodeKillCursors(body []byte) bool {
	r := &bsonReader{buf: body}
	if _, err := r.readInt32(); err != nil { // ZERO
		return d.fail()
	}
	n, err := r.readInt32()
	if err != nil {
		return d.fail()
	}
	for i := int32(0); i < n; i++ {
		if _, err := r.readInt64(); err != nil {
			return d.fail()
		}
	}
	d.stats.inc("op_kill_cursors")
	return true
}

// decodeCommand: database(cstring) → commandName(cstring) → metadata(BSON) →
// commandArgs(BSON) → 0..N inputDocs(BSON, loop). op_command.
func (d *decoder) decodeCommand(body []byte) bool {
	r := &bsonReader{buf: body}
	if _, err := r.readCString(); err != nil { // database
		return d.fail()
	}
	if _, err := r.readCString(); err != nil { // commandName
		return d.fail()
	}
	if _, err := parseDocument(r); err != nil { // metadata
		return d.fail()
	}
	if _, err := parseDocument(r); err != nil { // commandArgs
		return d.fail()
	}
	for r.remaining() > 0 { // inputDocs
		if _, err := parseDocument(r); err != nil {
			return d.fail()
		}
	}
	d.stats.inc("op_command")
	return true
}
```

- [ ] **Step 4: Run the codec tests — expect PASS**

Run: `go test ./internal/filter/network/mongoproxy/ -run TestDecode -v && go test ./internal/filter/network/mongoproxy/...`
Expected: all PASS.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/mongoproxy/
golangci-lint run ./internal/filter/network/mongoproxy/...
git add internal/filter/network/mongoproxy/ && git commit -m "phase 29.1 IMPL Task 9: codec.go part 3 — OP_INSERT/OP_GET_MORE/OP_KILL_CURSORS/OP_COMMAND body decode + validate"
```

---

## Task 10: `filter.go` — `NewFactory` + the both-directions filter glue + active-query wiring

**Files:**
- Create: `internal/filter/network/mongoproxy/filter.go`
- Test: `internal/filter/network/mongoproxy/filter_test.go`

> Mirrors `zookeeperproxy.go:26-95` (the both-directions filter). `NewFactory` parses + validates ONCE at boot (ADR-0079) and creates the roster ONCE per distinct stat_prefix (D-P1 eager). The per-connection `*filter` holds the decoder; `OnWrite` is a pure no-op `Continue` stub (the §3.7 pin; the response decoder is 29.2).

- [ ] **Step 1: Write the failing filter tests**

`filter_test.go`:
```go
package mongoproxy

import (
	"testing"

	"github.com/esalaine/envoy-go/internal/filter/network"
	"github.com/esalaine/envoy-go/internal/stats"
	mongo_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/mongo_proxy/v3"
	"google.golang.org/protobuf/types/known/anypb"
)

func mustAny(t *testing.T, m *mongo_proxyv3.MongoProxy) *anypb.Any {
	t.Helper()
	a, err := anypb.New(m)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

func buildFilter(t *testing.T, m *mongo_proxyv3.MongoProxy) (*filter, *stats.Registry) {
	t.Helper()
	reg := stats.NewRegistry()
	instFactory, err := NewFactory(reg)(mustAny(t, m), network.FactoryCtx{})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	return instFactory().(*filter), reg
}

func TestFactory_BootRejectMissingStatPrefix(t *testing.T) {
	reg := stats.NewRegistry()
	_, err := NewFactory(reg)(mustAny(t, &mongo_proxyv3.MongoProxy{}), network.FactoryCtx{})
	if err == nil || err.Error() != errStatPrefixRequired {
		t.Fatalf("err = %v, want %q", err, errStatPrefixRequired)
	}
}

func TestFilter_ImplementsBothDirections(t *testing.T) {
	f, _ := buildFilter(t, &mongo_proxyv3.MongoProxy{StatPrefix: "p"})
	if _, ok := any(f).(network.ReadFilter); !ok {
		t.Error("filter must be a ReadFilter")
	}
	if _, ok := any(f).(network.WriteFilter); !ok {
		t.Error("filter must be a WriteFilter")
	}
}

func TestFilter_OnDataFeedsDecoder(t *testing.T) {
	f, reg := buildFilter(t, &mongo_proxyv3.MongoProxy{StatPrefix: "p"})
	var buf network.Buffer
	buf.Append(msg(1, 2004, opQueryBody("db.collection1", 0, simpleQuery())))
	if f.OnData(&buf, false) != network.Continue {
		t.Error("OnData must return Continue")
	}
	if reg.NewCounterIfAbsent("mongo.p.op_query").Load() != 1 {
		t.Errorf("OnData did not feed the decoder (op_query != 1)")
	}
}

func TestFilter_OnDataNeverDrainsChainBuffer(t *testing.T) {
	// R3: the chain Buffer is observational — never drained/mutated.
	f, _ := buildFilter(t, &mongo_proxyv3.MongoProxy{StatPrefix: "p"})
	var buf network.Buffer
	raw := msg(1, 2004, opQueryBody("db.c", 0, simpleQuery()))
	buf.Append(raw)
	before := buf.Len()
	f.OnData(&buf, false)
	if buf.Len() != before {
		t.Errorf("OnData drained the chain buffer: %d → %d", before, buf.Len())
	}
}

func TestFilter_OnWriteIsNoOp(t *testing.T) {
	f, reg := buildFilter(t, &mongo_proxyv3.MongoProxy{StatPrefix: "p"})
	var buf network.Buffer
	buf.Append(msg(1, 1, []byte{1, 2, 3})) // an OP_REPLY on the write side
	if f.OnWrite(&buf, false) != network.Continue {
		t.Error("OnWrite must return Continue")
	}
	// No write-side decode at 29.1: op_reply stays 0.
	if reg.NewCounterIfAbsent("mongo.p.op_reply").Load() != 0 {
		t.Errorf("OnWrite must be a pure no-op at 29.1 (op_reply must stay 0)")
	}
}

func TestFilter_OnDestroyReleasesDecoder(t *testing.T) {
	f, _ := buildFilter(t, &mongo_proxyv3.MongoProxy{StatPrefix: "p"})
	f.OnDestroy()
	if f.dec != nil {
		t.Error("OnDestroy must release the decoder")
	}
}
```

- [ ] **Step 2: Run them — expect COMPILE failure** (`undefined: NewFactory`, `filter`)

Run: `go test ./internal/filter/network/mongoproxy/ -run 'TestFactory|TestFilter' -v`
Expected: build error.

- [ ] **Step 3: Write `filter.go`**

```go
package mongoproxy

import (
	"fmt"

	mongo_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/mongo_proxy/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/filter/network"
	"github.com/esalaine/envoy-go/internal/stats"
)

// NewFactory returns the mongoproxy NetworkFilterFactory with the stats Registry
// closure-captured (the zookeeperproxy.go:26 precedent — network.FactoryCtx
// carries no stats registry). The factory parses + validates ONCE at boot
// (ADR-0079) and EAGERLY creates the 23-stat roster per distinct stat_prefix at
// parse (D-P1). The returned FilterInstanceFactory allocates a fresh *filter per
// connection, all sharing the boot-parsed *compiledConfig (incl. the roster).
func NewFactory(reg *stats.Registry) network.NetworkFilterFactory {
	return func(tc *anypb.Any, _ network.FactoryCtx) (network.FilterInstanceFactory, error) {
		var msg mongo_proxyv3.MongoProxy
		if tc != nil && len(tc.GetValue()) > 0 {
			if err := tc.UnmarshalTo(&msg); err != nil {
				return nil, fmt.Errorf("mongo_proxy: invalid typed_config: %w", err)
			}
		}
		cfg, err := parseConfig(&msg)
		if err != nil {
			return nil, err
		}
		cfg.stats = newMongoStats(reg, cfg.statPrefix)
		return func() network.NetworkFilter {
			return &filter{cfg: cfg, dec: newDecoder(cfg, cfg.stats)}
		}, nil
	}
}

// filter is the per-connection mongo_proxy filter. It implements BOTH
// network.ReadFilter and network.WriteFilter (one instance, both directions —
// consumer #2 of the ADR-0221 seam; the zookeeperproxy both-directions shape).
type filter struct {
	network.Marker
	cfg *compiledConfig // shared, boot-parsed (incl. the roster)
	dec *decoder        // per-connection (private readBuf + sniffing + chainConsumed + active-query list)
	cb  network.ReadFilterCallbacks
	wcb network.WriteFilterCallbacks
}

// OnNewConnection is a no-op Continue: an OnNewConnection StopIteration would set
// the chain's sticky connHalted flag and block all OnData
// (reference_network_read_filter_onnewconnection_halts).
func (f *filter) OnNewConnection() network.Status { return network.Continue }

// OnData feeds the decoder the chain-buffer's NEW bytes (the chainConsumed
// high-water mark against TotalAppended — D-S29.1-4) and ALWAYS returns Continue.
// It NEVER drains the chain buffer, never closes, never halts (R3; at 29.1
// mongoproxy has no halt path — fault delay is 29.3).
func (f *filter) OnData(buf *network.Buffer, _ bool) network.Status {
	f.dec.decodeOnData(buf.Bytes(), buf.TotalAppended())
	return network.Continue
}

// OnWrite is a PURE NO-OP at 29.1 (the 28.1 zookeeper OnWrite-stub pin verbatim):
// it does NOT buffer write-direction bytes (no response decoder to drain them →
// unbounded growth). The write-side private buffer is created WITH the response
// decoder at 29.2. The stub exists so the filter satisfies WriteFilter end-to-end
// (the 0049 traffic DOES flow through writeChainConn → OnWrite).
func (f *filter) OnWrite(_ *network.Buffer, _ bool) network.Status { return network.Continue }

// SetReadFilterCallbacks / SetWriteFilterCallbacks store both (the both-directions
// dual injection — chain.go injects each exactly once).
func (f *filter) SetReadFilterCallbacks(cb network.ReadFilterCallbacks)   { f.cb = cb }
func (f *filter) SetWriteFilterCallbacks(cb network.WriteFilterCallbacks) { f.wcb = cb }

// OnDestroy drops the per-connection decoder + its active-query list (they die
// with the connection). Called exactly once per filter instance (the 28.1a dedupe).
func (f *filter) OnDestroy() { f.dec = nil }
```

> The `cb`/`wcb` fields are stored-but-unread at 29.1 (29.2's correlation + 29.3's halt read them — the zookeeper precedent). If `golangci-lint`'s `unused`/`structcheck` flags them, add a `// stored for 29.2/29.3` comment + confirm the zookeeperproxy filter (which has the same stored-unused `cb`/`wcb`) passes lint as-built (it does — `zookeeperproxy.go:52-53`); the pattern is established.

- [ ] **Step 4: Run the filter tests — expect PASS**

Run: `go test ./internal/filter/network/mongoproxy/... -v`
Expected: all PASS (the full package: config + bson + stats + codec + filter).

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/mongoproxy/
golangci-lint run ./internal/filter/network/mongoproxy/...
git add internal/filter/network/mongoproxy/ && git commit -m "phase 29.1 IMPL Task 10: filter.go — NewFactory (eager roster) + both-directions filter glue (OnData feeds decoder; OnWrite no-op stub; R3 never-drains)"
```

---

## Task 11: The 8th built-in registration + `bootstrap.go` blank-import + boot smoke

**Files:**
- Modify: `internal/filter/network/builtins/builtins.go`
- Modify: `internal/filter/network/builtins/builtins_test.go`
- Modify: `internal/bootstrap/bootstrap.go`

- [ ] **Step 1: Write the failing registration + boot-smoke tests**

In `builtins_test.go`, add (and rename the all-seven test to all-eight):
```go
// TestRegisterBuiltins_RegistersMongoProxy proves mongo_proxy is wired as the
// 8th built-in network filter (29.1; ADR-0224). A non-nil StatsRegistry is
// supplied because mongo_proxy's factory eagerly creates the 23-stat roster.
func TestRegisterBuiltins_RegistersMongoProxy(t *testing.T) {
	reg := network.NewRegistry()
	RegisterBuiltins(reg, Deps{StatsRegistry: stats.NewRegistry()})
	reg.Freeze()
	if _, ok := reg.Lookup(mongoproxy.TypeURL); !ok {
		t.Fatal("mongo_proxy not registered as the 8th built-in")
	}
}

// TestMongoProxyBootSmoke is the boot-smoke for the [mongo_proxy, tcp_proxy]
// chain: a mongo_proxy filter resolves through the registry; parsing the config
// eagerly creates the 23 stats at 0; the instance satisfies BOTH directions.
func TestMongoProxyBootSmoke(t *testing.T) {
	sreg := stats.NewRegistry()
	reg := network.NewRegistry()
	RegisterBuiltins(reg, Deps{StatsRegistry: sreg})
	reg.Freeze()

	factory, ok := reg.Lookup(mongoproxy.TypeURL)
	if !ok {
		t.Fatal("mongo_proxy factory not found")
	}
	tc := mustAny(t, &mongo_proxyv3.MongoProxy{StatPrefix: "mongoboot"})
	instFactory, err := factory(tc, network.FactoryCtx{})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	inst := instFactory()
	if _, isRead := inst.(network.ReadFilter); !isRead {
		t.Fatal("mongo_proxy instance must be a ReadFilter")
	}
	if _, isWrite := inst.(network.WriteFilter); !isWrite {
		t.Fatal("mongo_proxy instance must be a WriteFilter")
	}
	for _, name := range []string{"mongo.mongoboot.op_query", "mongo.mongoboot.op_reply",
		"mongo.mongoboot.decoding_error", "mongo.mongoboot.delays_injected"} {
		if got := sreg.NewCounterIfAbsent(name).Load(); got != 0 {
			t.Errorf("counter %s = %d at boot, want 0", name, got)
		}
	}
	if got := sreg.NewGaugeIfAbsent("mongo.mongoboot.op_query_active").Load(); got != 0 {
		t.Errorf("gauge op_query_active = %d at boot, want 0", got)
	}
}
```
Add the imports `mongoproxy "github.com/esalaine/envoy-go/internal/filter/network/mongoproxy"` and `mongo_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/mongo_proxy/v3"` to `builtins_test.go`. Update the existing `TestRegisterBuiltinsRegistersAllSeven` → `...AllEight` (rename + add `mongoproxy.TypeURL` to its expected-TypeURL list).

> Confirm `mustAny` exists in `builtins_test.go` (the zookeeper boot-smoke uses it — `builtins_test.go`); if it is typed to a specific proto, generalize it to `proto.Message` or add a mongo-typed local helper.

- [ ] **Step 2: Run them — expect COMPILE/FAIL** (`mongoproxy` not imported / not registered)

Run: `go test ./internal/filter/network/builtins/ -run 'Mongo|AllEight' -v`
Expected: build error or FAIL.

- [ ] **Step 3: Register the 8th built-in + the bootstrap blank-import**

In `builtins.go`, add the import `"github.com/esalaine/envoy-go/internal/filter/network/mongoproxy"` and the registration after zookeeperproxy (`builtins.go:68`):
```go
	// mongo_proxy: the 8th built-in (29.1; ADR-0224). Stats-PRIMARY filter: the
	// registry is closure-captured (the zookeeper_proxy/rbac_network precedent —
	// FactoryCtx carries no stats registry). The second both-directions
	// (ReadFilter + WriteFilter) production filter (ADR-0221 consumer #2).
	reg.Register(mongoproxy.TypeURL, mongoproxy.NewFactory(deps.StatsRegistry))
```
Update the package doc comment (`builtins.go:1-2`): `seven built-in network filters (echo, …, zookeeper_proxy)` → `eight built-in network filters (echo, …, zookeeper_proxy, mongo_proxy)`. Update the `RegisterBuiltins` doc (`builtins.go:40-44`) likewise.

In `bootstrap.go`, add after the zookeeper_proxy blank-import (`bootstrap.go:95`):
```go
	// Phase-29.1 registers the mongo_proxy network-filter extension proto so
	// protojson round-trips bootstraps carrying
	// filter_chains[].filters[].typed_config of that type. Registered
	// transitively by the mongoproxy filter package too; the explicit
	// blank-import here guarantees resolution in any bootstrap-parsing context
	// (e.g. the differential harness). Per ADR-0016 amendment policy, documented
	// in PROGRESS, not a new ADR.
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/mongo_proxy/v3"
```

- [ ] **Step 4: Run the builtins tests + the full build — expect PASS**

Run: `go build ./... && go test ./internal/filter/network/builtins/ -v`
Expected: build clean; all PASS (incl. the renamed all-eight + the two mongo tests).

- [ ] **Step 5: Confirm the §4 zero-touch property — the full existing suite still builds + unit-tests green**

Run: `go test ./internal/... -count=1`
Expected: PASS (mongoproxy merely REGISTERED perturbs nothing; the differential suite is the Task-16/17 gate).

- [ ] **Step 6: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/builtins/ internal/bootstrap/
golangci-lint run ./internal/filter/network/builtins/... ./internal/bootstrap/...
git add internal/filter/network/builtins/ internal/bootstrap/ && git commit -m "phase 29.1 IMPL Task 11: mongo_proxy 8th built-in registration + bootstrap blank-import + boot smoke (23 stats eager at 0; both-directions)"
```

---

## Task 12: The `mongo.` four-rule `name.go` TAG-EXTRACTOR arm

**Files:**
- Modify: `internal/stats/name.go` (add the `mongo.` arm in the `default` branch, after the `.zookeeper.` arm at `name.go:255-262`, before the default error at `:263`)
- Test: `internal/stats/name_test.go`

> The load-bearing AMEND-C1 four-rule extractor (D-P2/D-P3). Mirrors upstream's four `addTokenized` rules (§11.2). Per D-S29.1-5 the arm SORTS its own labels locally (alphabetical key order — the reference's order) so the multi-label exposition is deterministic WITHOUT touching `prom.go`.

- [ ] **Step 1: Write the failing flattening tests**

Append to `name_test.go`:
```go
func TestFlattenToProm_MongoFixed(t *testing.T) {
	base, labels, err := flattenToProm("mongo.mongo_a.op_query")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	if base != "envoy_mongo_op_query" {
		t.Errorf("base = %q, want envoy_mongo_op_query", base)
	}
	if len(labels) != 1 || labels[0].Key != "envoy_mongo_prefix" || labels[0].Value != "mongo_a" {
		t.Errorf("labels = %+v, want [{envoy_mongo_prefix mongo_a}]", labels)
	}
}

func TestFlattenToProm_MongoCmd(t *testing.T) {
	base, labels, err := flattenToProm("mongo.mongoprobe.cmd.isMaster.total")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	if base != "envoy_mongo_cmd_total" {
		t.Errorf("base = %q, want envoy_mongo_cmd_total", base)
	}
	// sorted by key: cmd, prefix
	if !sameLabels(labels, []Label{{"envoy_mongo_cmd", "isMaster"}, {"envoy_mongo_prefix", "mongoprobe"}}) {
		t.Errorf("labels = %+v", labels)
	}
}

func TestFlattenToProm_MongoCollection(t *testing.T) {
	base, labels, err := flattenToProm("mongo.mongoprobe.collection.collection1.query.scatter_get")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	if base != "envoy_mongo_collection_query_scatter_get" {
		t.Errorf("base = %q", base)
	}
	if !sameLabels(labels, []Label{{"envoy_mongo_collection", "collection1"}, {"envoy_mongo_prefix", "mongoprobe"}}) {
		t.Errorf("labels = %+v", labels)
	}
}

func TestFlattenToProm_MongoCallsite(t *testing.T) {
	base, labels, err := flattenToProm("mongo.mongoprobe.collection.collection1.callsite.probeFn.query.total")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	if base != "envoy_mongo_collection_callsite_query_total" {
		t.Errorf("base = %q", base)
	}
	// three labels, sorted: callsite, collection, prefix (§11.2 verbatim order)
	want := []Label{
		{"envoy_mongo_callsite", "probeFn"},
		{"envoy_mongo_collection", "collection1"},
		{"envoy_mongo_prefix", "mongoprobe"},
	}
	if !sameLabels(labels, want) {
		t.Errorf("labels = %+v, want %+v", labels, want)
	}
}

func TestFlattenToProm_MongoGaugeAndGuards(t *testing.T) {
	// the gauge flattens like any fixed stat (TYPE is set by prom.go from Metric.Type)
	base, _, err := flattenToProm("mongo.p.op_query_active")
	if err != nil || base != "envoy_mongo_op_query_active" {
		t.Errorf("gauge base = %q err = %v", base, err)
	}
	// a dotted prefix must NOT match (the dot-free-prefix guard) → still errors
	if _, _, err := flattenToProm("mongo.a.b.cmd.x.total"); err == nil {
		// NOTE: "a.b" is not dot-free → the arm must reject; falls through to the
		// default error. (If the arm greedily matched it would mis-hoist.)
		t.Errorf("a dotted prefix must not match the mongo arm")
	}
}
```
Add a `sameLabels` helper to `name_test.go` if one does not already exist (compare as sets):
```go
func sameLabels(got, want []Label) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false // the mongo arm emits sorted order → positional compare is valid
		}
	}
	return true
}
```

> **PLAN note on the dot-free-prefix guard:** `mongo.a.b.cmd.x.total` — here the prefix segment after `mongo.` is `a`, and the remainder `b.cmd.x.total` does not match any of the three dynamic shapes, so the arm flattens it to `envoy_mongo_b_cmd_x_total{envoy_mongo_prefix="a"}` rather than erroring. The guard the test means is: the leading `mongo.` + the SINGLE dot-free prefix segment is consumed first (always one segment), THEN the remainder is shape-matched. Adjust the final sub-test to assert the actual fall-through flatten (a fixed-shape stat) rather than an error — the arm is intentionally permissive on the remainder (the wasm/zookeeper precedent). Pin the exact expectation when writing the production arm at Step 3.

- [ ] **Step 2: Run them — expect FAIL** (the mongo names hit the default error at `name.go:263`)

Run: `go test ./internal/stats/ -run TestFlattenToProm_Mongo -v`
Expected: FAIL with "no recognized top-level segment".

- [ ] **Step 3: Add the `mongo.` arm to `name.go`**

Add `"sort"` to the `name.go` import block. Insert AFTER the `.zookeeper.` arm (after `name.go:261`'s closing `}`), BEFORE the `return "", nil, fmt.Errorf(...)` default error:
```go
		// Phase-29.1 mongo_proxy TAG-EXTRACTOR (ADR-0224; parent AMEND-B2 + 29.1
		// AMEND-C1; the .rbac. ADR-0218 label-promotion precedent generalized to
		// MULTI-label). Mirrors upstream's four addTokenized rules
		// (well_known_names.cc — MONGO_PREFIX/MONGO_CMD/MONGO_COLLECTION/MONGO_CALLSITE,
		// §11.2). Shape (NOT allowlist) validation — dynamic cmd/collection/callsite
		// values make an allowlist impossible (the wasm/zookeeper permissive
		// precedent). Internal name mongo.<prefix>.<rest> →
		//   mongo.<sp>.<fixed>                                   → envoy_mongo_<fixed>      {prefix}
		//   mongo.<sp>.cmd.<cmd>.total                           → envoy_mongo_cmd_total    {prefix,cmd}
		//   mongo.<sp>.collection.<c>.query.<leaf>               → envoy_mongo_collection_query_<leaf>          {prefix,collection}
		//   mongo.<sp>.collection.<c>.callsite.<cs>.query.<leaf> → envoy_mongo_collection_callsite_query_<leaf> {prefix,collection,callsite}
		// Labels are emitted in SORTED key order (D-S29.1-5 — the reference's
		// alphabetical order; sorted HERE so prom.go is untouched).
		// KEEP-IN-SYNC: internal/filter/network/mongoproxy/stats.go (the name builders).
		if rest, ok := strings.CutPrefix(internal, "mongo."); ok {
			if idx := strings.IndexByte(rest, '.'); idx > 0 {
				prefix, tail := rest[:idx], rest[idx+1:]
				if !strings.ContainsRune(prefix, '.') {
					labels = append(labels, Label{Key: "envoy_mongo_prefix", Value: prefix})
					tail = hoistMongoDynamicSegments(tail, &labels)
					base = "envoy_mongo_" + strings.ReplaceAll(tail, ".", "_")
					sort.Slice(labels, func(i, j int) bool { return labels[i].Key < labels[j].Key })
					return base, labels, nil
				}
			}
		}
```
Add the helper (package-level in `name.go`):
```go
// hoistMongoDynamicSegments extracts the cmd/collection/callsite label tokens
// from a mongo post-prefix tail, appending them to *labels and returning the tail
// with the dynamic VALUE tokens removed (so the flattened base collapses distinct
// commands/collections/callsites onto one family — AMEND-C1). Mirrors the
// upstream addTokenized capture positions (§11.2):
//
//	cmd.<cmd>.<leaf...>                        → cmd.<leaf...>            + {cmd}
//	collection.<c>.callsite.<cs>.query.<leaf>  → collection.callsite.query.<leaf> + {collection,callsite}
//	collection.<c>.query.<leaf...>             → collection.query.<leaf...>        + {collection}
func hoistMongoDynamicSegments(tail string, labels *[]Label) string {
	if t, ok := strings.CutPrefix(tail, "cmd."); ok {
		// t = "<cmd>.<leaf...>"
		if idx := strings.IndexByte(t, '.'); idx > 0 {
			*labels = append(*labels, Label{Key: "envoy_mongo_cmd", Value: t[:idx]})
			return "cmd." + t[idx+1:]
		}
		return tail
	}
	if t, ok := strings.CutPrefix(tail, "collection."); ok {
		// t = "<c>.callsite.<cs>.query.<leaf>" OR "<c>.query.<leaf...>"
		idx := strings.IndexByte(t, '.')
		if idx <= 0 {
			return tail
		}
		coll := t[:idx]
		afterColl := t[idx+1:] // "callsite.<cs>.query.<leaf>" or "query.<leaf...>"
		*labels = append(*labels, Label{Key: "envoy_mongo_collection", Value: coll})
		if cs, ok := strings.CutPrefix(afterColl, "callsite."); ok {
			// cs = "<cs>.query.<leaf>"
			if j := strings.IndexByte(cs, '.'); j > 0 {
				*labels = append(*labels, Label{Key: "envoy_mongo_callsite", Value: cs[:j]})
				return "collection.callsite." + cs[j+1:] // "collection.callsite.query.<leaf>"
			}
		}
		return "collection." + afterColl // "collection.query.<leaf...>"
	}
	return tail
}
```

- [ ] **Step 4: Add the byte-exact emitted-line test (D-S29.1-5 proof) + run all**

Append to `name_test.go` (proves the three-label callsite line renders byte-identical to the §11.2 form via the real `WriteProm` path):
```go
func TestWriteProm_MongoCallsiteLineByteExact(t *testing.T) {
	reg := NewRegistry()
	reg.NewCounterIfAbsent("mongo.mongoprobe.collection.collection1.callsite.probeFn.query.scatter_get").Inc()
	var b strings.Builder
	if err := WriteProm(&b, reg); err != nil {
		t.Fatalf("WriteProm: %v", err)
	}
	want := `envoy_mongo_collection_callsite_query_scatter_get{envoy_mongo_callsite="probeFn",envoy_mongo_collection="collection1",envoy_mongo_prefix="mongoprobe"} 1`
	if !strings.Contains(b.String(), want) {
		t.Errorf("WriteProm output missing the §11.2 byte-exact line:\nwant: %s\ngot:\n%s", want, b.String())
	}
}
```

Run: `go test ./internal/stats/ -run 'TestFlattenToProm_Mongo|TestWriteProm_Mongo' -v && go test ./internal/stats/ -count=1`
Expected: all PASS; the FULL `internal/stats` suite stays green (the existing SN1–SN9 flatten tests unaffected — the mongo arm is additive in the `default` branch; `prom.go` untouched).

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/stats/
golangci-lint run ./internal/stats/...
git add internal/stats/ && git commit -m "phase 29.1 IMPL Task 12: name.go mongo. four-rule tag-extractor arm (prefix/cmd/collection/callsite; sorted-label order; prom.go untouched — D-S29.1-5)"
```

---

## Task 13: The 39th fuzzer `FuzzMongoDecode`

**Files:**
- Create: `internal/filter/network/mongoproxy/fuzz_test.go`

> The `FuzzZookeeperRequestDecode` (37th) precedent verbatim, adapted to the mongo decoder. Three safety invariants: (1) no panic; (2) the chain bytes are NEVER mutated (R3); (3) sniffing-off idempotence — once a decode error fires, further input decodes nothing and increments nothing (AMEND-B6); plus the bounded-readBuf property.

- [ ] **Step 1: Write the fuzzer**

`fuzz_test.go`:
```go
package mongoproxy

import (
	"bytes"
	"testing"

	"github.com/esalaine/envoy-go/internal/stats"
)

// FuzzMongoDecode is the 39th fuzzer (SPEC §15.1 Layer C). It feeds arbitrary
// bytes through the production decodeOnData entry point and asserts:
//  1. no panic (a panic fails the fuzz run);
//  2. the input slice (the chain-buffer stand-in) is NEVER mutated (R3);
//  3. sniffing-off idempotence — once decoding_error fires (sniffing=false),
//     a second feed decodes/increments NOTHING (decoding_error stays == 1) and
//     readBuf is released (AMEND-B6 / D-S29.1-6);
//  4. readBuf stays bounded (no unbounded growth on partial-frame input).
func FuzzMongoDecode(f *testing.F) {
	// Seed corpus: a valid OP_QUERY, an OP_MSG (→ decoding_error), a partial
	// header, an oversized messageLength, a garbage-BSON OP_QUERY, an OP_INSERT.
	f.Add(msgSeed(1, 2004, opQueryBody("db.c", 0, simpleQuery())))
	f.Add(msgSeed(1, 2013, nil))
	f.Add(msgSeed(1, 2004, opQueryBody("db.c", 0, simpleQuery()))[:10])
	f.Add(append(leI32(1<<20), make([]byte, 12)...))
	f.Add(msgSeed(1, 2004, opQueryBody("db.c", 0, []byte{0x05, 0x00, 0x00, 0x00, 0x13}))) // bad BSON type
	f.Add(msgSeed(1, 2002, append(leI32(0), append([]byte("db.c\x00"), simpleQuery()...)...)))

	f.Fuzz(func(t *testing.T, data []byte) {
		reg := stats.NewRegistry()
		cfg := &compiledConfig{statPrefix: "fuzz", commands: map[string]bool{"isMaster": true}}
		ms := newMongoStats(reg, "fuzz")
		cfg.stats = ms
		d := newDecoder(cfg, ms)

		orig := append([]byte(nil), data...)

		// Invariant 1: no panic (implicit).
		d.decodeOnData(data, int64(len(data)))
		errAfterFirst := ms.counters["decoding_error"].Load()
		sniffingAfterFirst := d.sniffing

		// Feed cumulatively a second time (the chain buffer accumulates).
		doubled := append(append([]byte(nil), data...), data...)
		d.decodeOnData(doubled, int64(len(doubled)))

		// Invariant 2: the input was never mutated (R3).
		if !bytes.Equal(data, orig) {
			t.Fatal("decodeOnData mutated the chain bytes")
		}

		// Invariant 3: once sniffing is off, decoding_error never increments again.
		if !sniffingAfterFirst && ms.counters["decoding_error"].Load() != errAfterFirst {
			t.Fatalf("decoding_error grew after sniffing-off: %d → %d",
				errAfterFirst, ms.counters["decoding_error"].Load())
		}

		// Invariant 4: readBuf is bounded — at most one partial frame. Once
		// sniffing is off readBuf is nil; otherwise it holds < one complete frame.
		if len(d.readBuf) > len(doubled)+16 {
			t.Fatalf("readBuf grew unboundedly: %d bytes", len(d.readBuf))
		}
	})
}

// msgSeed mirrors the codec_test msg() helper (a separate name avoids cross-file
// fuzz/seed coupling; identical layout). 16-byte LE header + body.
func msgSeed(reqID, opCode int32, body []byte) []byte { return msg(reqID, opCode, body) }
```

- [ ] **Step 2: Run the fuzzer briefly + the seed corpus**

Run: `go test ./internal/filter/network/mongoproxy/ -run FuzzMongoDecode -v` (runs the seed corpus as unit cases), then `go test ./internal/filter/network/mongoproxy/ -fuzz FuzzMongoDecode -fuzztime 30s`
Expected: seed PASS; 30s fuzz finds no crash.

- [ ] **Step 3: Confirm the fuzzer count is now 39**

Run: `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l`
Expected: **39**.

- [ ] **Step 4: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/mongoproxy/
golangci-lint run ./internal/filter/network/mongoproxy/...
git add internal/filter/network/mongoproxy/ && git commit -m "phase 29.1 IMPL Task 13: FuzzMongoDecode (39th fuzzer) — no-panic + chain-bytes-unmutated + sniffing-off idempotence + bounded readBuf"
```

---

## Task 14: `0049-mongo-requests` driver part 1 — bootstraps + wire-byte builders + MultiListener + TCPSink

**Files:**
- Create: `test/fixtures/0049-mongo-requests/driver/driver.go`
- Create: `test/fixtures/0049-mongo-requests/README.md`
- Modify: `test/differential/runner_test.go` (add the `0049` driver blank-import)

> The `0046-zookeeper-requests` driver (875 LoC) is the structural precedent: `MultiListenerDriver` (two listeners — `fixture.go:611`), `BackendKindAware` returning `fixture.TCPSink` (already exists, BackendKind 28 — NO `fixture.go` change), the dockerized reference + envoy-go subprocess. Part 1 lands the driver skeleton + the two bootstraps + the wire-byte/BSON builder helpers (D-S29.1-3, shared with the future 29.2 `0051` driver) + the MultiListener plumbing. The StatsAsserter + arms land at Tasks 15–16.

- [ ] **Step 1: Write the wire-byte/BSON builders** (the load-bearing reusable piece — D-S29.1-3)

In `driver.go`, the builders (a self-contained little-endian frame kit):
```go
// --- little-endian mongo wire builders (D-S29.1-3; shared with the 29.2 0051
// driver). These MIRROR the codec_test helpers but live in the driver package so
// the fixture is self-contained.) ---

func leI32(v int32) []byte { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, uint32(v)); return b }
func leI64(v int64) []byte { b := make([]byte, 8); binary.LittleEndian.PutUint64(b, uint64(v)); return b }
func cstr(s string) []byte { return append([]byte(s), 0x00) }

// bsonInt32 builds a {name: int32} element; bsonString a {name: string} element.
func bsonInt32(name string, v int32) []byte {
	return append(append([]byte{0x10}, cstr(name)...), leI32(v)...)
}
func bsonString(name, v string) []byte {
	out := append([]byte{0x02}, cstr(name)...)
	out = append(out, leI32(int32(len(v)+1))...)
	out = append(out, []byte(v)...)
	return append(out, 0x00)
}
func bsonDocElem(name string, inner []byte) []byte { // {name: <document>}
	return append(append([]byte{0x03}, cstr(name)...), inner...)
}

// bsonDoc wraps element bytes into a BSON document (int32 len incl self + 0x00).
func bsonDoc(elems ...byte) []byte {
	body := append(elems, 0x00)
	return append(leI32(int32(4+len(body))), body...)
}

// mongoMsg wraps a body in a 16-byte LE MsgHeader (messageLength incl header).
func mongoMsg(reqID, opCode int32, body []byte) []byte {
	out := append(leI32(int32(16+len(body))), leI32(reqID)...)
	out = append(out, leI32(0)...)      // responseTo
	out = append(out, leI32(opCode)...) // opCode
	return append(out, body...)
}

// opQuery builds a complete OP_QUERY message.
func opQuery(reqID int32, fullColl string, flags int32, queryDoc []byte) []byte {
	body := append(leI32(flags), cstr(fullColl)...)
	body = append(body, leI32(0)...) // numberToSkip
	body = append(body, leI32(0)...) // numberToReturn
	body = append(body, queryDoc...)
	return mongoMsg(reqID, 2004, body)
}

// opInsert/opGetMore/opKillCursors/opCommand — the other request opcodes.
func opInsert(reqID int32, fullColl string, docs ...[]byte) []byte {
	body := append(leI32(0), cstr(fullColl)...)
	for _, d := range docs {
		body = append(body, d...)
	}
	return mongoMsg(reqID, 2002, body)
}
func opGetMore(reqID int32, fullColl string, cursorID int64) []byte {
	body := append(leI32(0), cstr(fullColl)...)
	body = append(body, leI32(10)...)
	body = append(body, leI64(cursorID)...)
	return mongoMsg(reqID, 2005, body)
}
func opKillCursors(reqID int32, cursorIDs ...int64) []byte {
	body := append(leI32(0), leI32(int32(len(cursorIDs)))...)
	for _, c := range cursorIDs {
		body = append(body, leI64(c)...)
	}
	return mongoMsg(reqID, 2007, body)
}
func opCommand(reqID int32, db, cmd string) []byte {
	body := append(cstr(db), cstr(cmd)...)
	body = append(body, bsonDoc()...)            // metadata
	body = append(body, bsonDoc(bsonInt32(cmd, 1)...)...) // commandArgs
	return mongoMsg(reqID, 2010, body)
}
func opMsgFrame(reqID int32) []byte { return mongoMsg(reqID, 2013, nil) } // the unsupported-opcode arm
```

- [ ] **Step 2: Write the driver skeleton + the two bootstraps + MultiListener** (the `0046` precedent)

Implement `mongoRequestsDriver` satisfying `fixture.Driver` + `fixture.MultiListenerDriver` + `fixture.BackendKindAware` + `fixture.StatsAsserter` (the asserter body lands at Tasks 15–16). Two listeners per §8.1:
- `l_default` — `stat_prefix: mongo_a`, default `commands`.
- `l_commands` — `stat_prefix: mongo_b`, `commands: ["isMaster"]`.

Both route to ONE cluster → ONE `TCPSink` backend:
```go
func (d *mongoRequestsDriver) BackendCount() int                 { return 1 }
func (d *mongoRequestsDriver) BackendKind() fixture.BackendKind  { return fixture.TCPSink }
func (d *mongoRequestsDriver) SubjectListenerNames() []string    { return []string{"l_default", "l_commands"} }
func (d *mongoRequestsDriver) ReferenceListenerPorts() []int     { return []int{19140, 19141} }
```
The `ReferenceBootstrap` + `SubjectConfig` render `[mongo_proxy, tcp_proxy]` chains on both listeners (the `0046` template: `mongo_proxy` `typed_config` `@type: type.googleapis.com/envoy.extensions.filters.network.mongo_proxy.v3.MongoProxy`, `stat_prefix`, and on `l_commands` `commands: ["isMaster"]`). Reuse the `0046` admin-port + cluster scaffolding verbatim; register via `init()`:
```go
func init() { fixture.RegisterFixture("0049-mongo-requests", &mongoRequestsDriver{}) }
```

- [ ] **Step 3: Add the runner blank-import**

In `runner_test.go`, after the `0048` import:
```go
	_ "github.com/esalaine/envoy-go/test/fixtures/0049-mongo-requests/driver"
```

- [ ] **Step 4: Compile-check the driver package (Drive bodies stubbed to return nil,nil for now)**

Run: `go build ./test/... && go vet ./test/fixtures/0049-mongo-requests/...`
Expected: clean (the StatsAsserter + Drive arms are filled at Tasks 15–16; a no-op `DriveReferenceMulti`/`DriveSubjectMulti` that establishes the listeners compiles).

- [ ] **Step 5: gofmt + commit**

```bash
gofmt -l test/fixtures/0049-mongo-requests/ test/differential/
git add test/fixtures/0049-mongo-requests/ test/differential/runner_test.go && git commit -m "phase 29.1 IMPL Task 14: 0049 driver part 1 — bootstraps + LE wire/BSON builders (D-S29.1-3) + MultiListener + TCPSink wiring"
```

---

## Task 15: `0049` driver part 2 — the label-aware StatsAsserter + arms 1–5

**Files:**
- Modify: `test/fixtures/0049-mongo-requests/driver/driver.go`

> The novel piece is the LABEL-AWARE Prometheus scrape (§8.1.2): mongo's tag-extracted exposition (AMEND-B2 + C1) requires parsing `name{label="v",…}` lines into `(name + canonical sorted-label-set) → value` maps on BOTH sides. The `0043` rbac single-label driver is the nearest precedent; `0049` generalizes it to multi-label.

- [ ] **Step 1: Write the label-aware scrape helper** (the load-bearing new mechanism)

```go
// scrapeMongoStats issues GET /stats/prometheus and returns a map keyed by the
// CANONICAL form `name{k1="v1",k2="v2"}` with labels sorted by key, value parsed.
// Retains only envoy_mongo_* lines. This is the label-aware generalization of the
// 0043 single-label / 0046 flat mechanics (§8.1.2): the canonical key folds the
// metric NAME and the LABEL SET together, so a value lookup intrinsically asserts
// BOTH name-parity AND label-extraction parity (R7).
func scrapeMongoStats(adminAddr string) (map[string]int64, error) {
	body, err := httpGet("http://" + adminAddr + "/stats/prometheus")
	if err != nil {
		return nil, err
	}
	out := map[string]int64{}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.HasPrefix(line, "envoy_mongo_") {
			continue
		}
		// split "name{labels} value" — the value is the last space-separated field.
		sp := strings.LastIndexByte(line, ' ')
		if sp < 0 {
			continue
		}
		nameLabels, valStr := line[:sp], line[sp+1:]
		v, err := strconv.ParseInt(valStr, 10, 64)
		if err != nil {
			continue // skip non-integer (e.g. gauge float-form — not expected for mongo)
		}
		out[canonicalize(nameLabels)] = v
	}
	return out, nil
}

// canonicalize normalizes "name{k=\"v\",...}" to "name{k1=\"v1\",k2=\"v2\"}" with
// labels sorted by key (so ref/subj label-order differences cannot cause a miss).
func canonicalize(nameLabels string) string {
	open := strings.IndexByte(nameLabels, '{')
	if open < 0 {
		return nameLabels // no labels
	}
	name := nameLabels[:open]
	inner := strings.TrimSuffix(nameLabels[open+1:], "}")
	if inner == "" {
		return name
	}
	pairs := strings.Split(inner, ",")
	sort.Strings(pairs)
	return name + "{" + strings.Join(pairs, ",") + "}"
}
```
(`httpGet` is the project's existing admin-scrape helper used by `0046`/`0043` — reuse it; if it is unexported per-driver, copy the 5-line `net/http` GET the `0046` driver uses.)

- [ ] **Step 2: Write `AssertStats` arms 1–5 (the §8.1.3 expectations)** + the DriveMulti arms that send them

`DriveReferenceMulti` / `DriveSubjectMulti` both call a shared `driveArms(addrs)` that opens connections per arm (FRESH connections for error arms — none in 1–5) and writes the crafted bytes. Then `AssertStats` keys expectations by the canonical Prometheus form:
```go
expectations := []struct {
	key  string
	want int64
}{
	// arm 1 (l_default, mongo_a): plain scatter-get query on db.collection1.
	{`envoy_mongo_op_query{envoy_mongo_prefix="mongo_a"}`, /* cumulative — see table */ 0},
	{`envoy_mongo_op_query_scatter_get{envoy_mongo_prefix="mongo_a"}`, 0},
	{`envoy_mongo_collection_query_total{envoy_mongo_collection="collection1",envoy_mongo_prefix="mongo_a"}`, 0},
	{`envoy_mongo_collection_query_scatter_get{envoy_mongo_collection="collection1",envoy_mongo_prefix="mongo_a"}`, 0},
	// arm 2 (l_commands, mongo_b): cmd.isMaster + cmd.unknown_command.
	{`envoy_mongo_cmd_total{envoy_mongo_cmd="isMaster",envoy_mongo_prefix="mongo_b"}`, 1},
	{`envoy_mongo_cmd_total{envoy_mongo_cmd="unknown_command",envoy_mongo_prefix="mongo_b"}`, 1},
	// arm 5 (l_default): $comment callsite double-count.
	{`envoy_mongo_collection_callsite_query_total{envoy_mongo_callsite="fixtureFn",envoy_mongo_collection="collection1",envoy_mongo_prefix="mongo_a"}`, 1},
}
```
> **PLAN note (cumulative accounting — the `0046` discipline):** the per-prefix counters are CUMULATIVE over all arms sharing a listener. Author the exact `want` values in ONE authoritative arm-accounting table in the driver doc-comment (the `0046:600-640` precedent), filling the arm-1/3/5 contributions to `mongo_a`'s `op_query`/`scatter_get`/`multi_get`/`no_max_time`/`collection.*` — DO NOT leave the `0` placeholders above. Each value is asserted on BOTH sides via `scrapeMongoStats` lookups; an ABSENT key (name/label-shape failure) is reported distinctly from a present-but-wrong value (the `lookupZKCounter` presence-flag pattern).

- [ ] **Step 3: Run the fixture (arms 1–5 only) cross-side**

Run: `go test ./test/differential/ -run '0049' -count=1 -v` (requires Docker for the reference container per the harness; if Docker is unavailable in the IMPL session, record the SKIP honestly in PROGRESS and run at the Task-17 six-gate on a Docker host).
Expected: arms 1–5 PASS cross-side (ref ≡ subj).

- [ ] **Step 4: gofmt + commit**

```bash
gofmt -l test/fixtures/0049-mongo-requests/
git add test/fixtures/0049-mongo-requests/ && git commit -m "phase 29.1 IMPL Task 15: 0049 driver part 2 — label-aware StatsAsserter (canonical name+sorted-labels scrape; R7) + arms 1-5"
```

---

## Task 16: `0049` driver part 3 — arms 6–9 (error arms + exists-at-zero + deliberate-break) + `0050-mongo-boot-reject`

**Files:**
- Modify: `test/fixtures/0049-mongo-requests/driver/driver.go`, `README.md`
- Create: `test/fixtures/0050-mongo-boot-reject/driver/driver.go`, `README.md`
- Modify: `test/differential/runner_test.go` (add the `0050` blank-import)
- Modify: `PROGRESS.md` (record the R4 deliberate-break results)

- [ ] **Step 1: Add arms 6–8 to `0049`** (each error arm on a FRESH connection — AMEND-B6)

- **Arm 6 (unsupported opcode):** `opMsgFrame(...)` on `l_default` (FRESH conn) → `decoding_error` +1 both sides; a follow-up valid `opQuery(...)` on the SAME conn increments nothing (the sniffing-off proof — assert the post-arm-6 `op_query` does NOT advance for that connection's traffic). Passthrough proven via the backend's receive count > 0.
- **Arm 7 (garbage BSON):** an `opQuery(...)` whose query document has a bad element type `0x13` (FRESH conn) → `decoding_error` +1 both sides.
- **Arm 8 (exists-at-zero / creation + gauge-TYPE parity):** after arms 1–7 with ALL connections closed, assert (both prefixes): `op_reply`, `op_reply_cursor_not_found`, `op_reply_query_failure`, `op_reply_valid_cursor`, `op_command_reply`, `delays_injected`, `cx_drain_close` PRESENT and == 0; the `op_query_active` gauge PRESENT with `# TYPE envoy_mongo_op_query_active gauge` and == 0; `cx_destroy_local_with_active_rq` / `cx_destroy_remote_with_active_rq` PRESENT both sides, value NOT compared (AMEND-C2 — the reference increments them on every query-bearing connection close; envoy-go's increment lands at 29.2).

The gauge-TYPE assertion scrapes the raw `# TYPE …` line (not via `scrapeMongoStats`, which skips `#` lines) — add a small `scrapeTypeLine(addr, name)` helper.

- [ ] **Step 2: Record the R4 deliberate-break liveness proof (§8.1.3 arm 9)**

In `README.md` + `PROGRESS.md`, document the two recorded breaks (run locally, with `-count=1` per `reference_differential_break_protocol_count1`, then REVERT):
1. Temporarily assert `op_query == <wrong>` → MUST fail on both runner paths.
2. Temporarily disable the Task-12 `name.go` mongo arm → EVERY label-keyed lookup misses → every arm fails (the lookup-miss proves the §7.4 arm is load-bearing).
Quote the failing output into PROGRESS. (This mirrors the `0030`/28.1 R4 discipline — proving the assertions are LIVE, not vacuous; `reference_differential_asserter_dispatch`.)

- [ ] **Step 3: Write `0050-mongo-boot-reject`** (the `0047-zookeeper-boot-reject` precedent, 202 LoC)

A `[mongo_proxy, tcp_proxy]` chain whose mongo `typed_config` has NO `stat_prefix` → BOTH sides reject at boot. Implement `fixture.Driver` + `differential.BootRejectFixture`:
```go
func (d *mongoBootRejectDriver) BootRejectScript() string             { return "" }
func (d *mongoBootRejectDriver) ExpectedBootErrorSubstring() string   { return "stat_prefix" }
```
`"stat_prefix"` is present in BOTH the reference's PGV violation text AND envoy-go's `mongo_proxy: stat_prefix is required`. Symmetric mode; a minimal unused cluster satisfies the zero-cluster boot reject (`reference_network_filter_typeurl_extensions`). The AMEND-B9 delay arms stay unit-tested (Task 3), NOT fixture arms (parent D-P5). Register + add the runner blank-import:
```go
	_ "github.com/esalaine/envoy-go/test/fixtures/0050-mongo-boot-reject/driver"
```

- [ ] **Step 4: Run both fixtures cross-side + confirm the count**

Run: `go test ./test/differential/ -run '0049|0050' -count=1 -v`
Expected: both PASS (9 arms green on `0049`; `0050` boot-rejects symmetrically). Confirm `ls -d test/fixtures/[0-9]* | wc -l` → **52**.

- [ ] **Step 5: gofmt + commit**

```bash
gofmt -l test/fixtures/0049-mongo-requests/ test/fixtures/0050-mongo-boot-reject/ test/differential/
git add test/fixtures/0049-mongo-requests/ test/fixtures/0050-mongo-boot-reject/ test/differential/runner_test.go PROGRESS.md
git commit -m "phase 29.1 IMPL Task 16: 0049 arms 6-9 (error/exists-at-zero/gauge-TYPE/R4-break) + 0050-mongo-boot-reject — fixtures 50→52"
```

---

## Task 17: Completion bundle + the six-gate

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/DECISIONS.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `next-prompt.txt`, `PROGRESS.md`

- [ ] **Step 1: BEHAVIOR_CONTRACT 29.1 bundle (ADR-0052 atomic landing, §9/§14)**

Add a NEW `### envoy.filters.network.mongo_proxy` subsection (after the zookeeper_proxy subsection): the request-side decode semantics (§3.5); the 7-opcode envelope + OP_MSG-not-decoded (upstream parity, not a gap); the sniffing-off-on-error connection-lifetime semantics; the 23-stat roster + the EAGER creation posture + the boot-window departure (D-P1); the `mongo.<stat_prefix>.` scope; the Prometheus four-rule tag extraction (the §7.4 table); the dynamic cmd/collection/callsite counter families + the commands-remembering semantics + alias normalization; the runtime-keys-at-defaults departure (29.1 subset); the OP_REPLY/OP_COMMANDREPLY recognized-not-decoded 29.1 boundary (closed at 29.2); forward-pointers to 29.2/29.3. Update the stat table **337 → 360** (the 23 new rows). Confirm the canonical stat-surface recipe now reports **360**.

- [ ] **Step 2: ADR-0224 §Decision/§Consequences body in place (ADR-0044; no new ADR number)**

Fill the ADR-0224 §Decision + §Consequences bodies (the §Context draft landed at the parent SPEC, `DECISIONS.md:14421`). DECISIONS.md tail STAYS **ADR-0226**; next-free STAYS **ADR-0227**. The §3/§7/§8 design is the body's blueprint.

- [ ] **Step 3: STATE.md + ROADMAP advance**

- STATE.md: `active-phase → phase 29.1 IMPL done`; `next-skill → superpowers:writing-plans` (or the 29.2 SPEC per SKILL_ROUTING — confirm the lifecycle: the 29.1 IMPL closes → the 29.2 SPEC opens); counts: fixtures **52**, fuzzers **39**, stats **360**; DECISIONS tail **ADR-0226** (next-free **ADR-0227**); `last-commit` → the 29.1 IMPL squash SHA (filled post-merge).
- ROADMAP: sub-row 29.1 `in-progress → done`; **parent row 29 STAYS `in-progress`** (the ROLLUP is 29.3's); 29.2/29.3 STAY `planned`.

- [ ] **Step 4: Rewrite `next-prompt.txt`** for the **29.2-SPEC cold-start** (the response side + correlation + the gauge increments + dynamic metadata + fixture `0051` + the `TCPMongoResponder` BackendKind anticipated value 30; ADR-0225 body at 29.2). Mirror the current file's structure.

- [ ] **Step 5: THE SIX-GATE (§15.2; all outputs quoted into PROGRESS.md, run honestly)**

```bash
go build ./...                                            # gate 1
go vet ./...                                              # gate 2
golangci-lint run                                         # gate 3
go test ./... -race -short                                # gate 4
go test ./test/differential/ -count=1                     # gate 5: FULL 52-dir suite
                                                          #   incl. the 50-dir back-compat (R1 §4)
# gate 6: h2spec 53/53 + proxy-wasm 10/10 re-run (asserted-unaffected — 29.1
# touches no HTTP path); run the project's conformance recipes.
```
Expected: all green — `go build`/`vet`/`golangci-lint` clean; `-race -short` PASS; the FULL differential suite 52/52 byte-exact (the 50 pre-existing dirs unchanged — the §4 zero-touch proof); h2spec 53/53; proxy-wasm 10/10. Quote each gate's tail output into PROGRESS.md.

- [ ] **Step 6: Final commit (the controller squash-merges + pushes — `feedback_push_to_origin`; subagents do NOT push)**

```bash
git add docs/envoy-go/ next-prompt.txt PROGRESS.md
git commit -m "phase 29.1 IMPL Task 17: completion bundle — BEHAVIOR_CONTRACT 29.1 (337→360) + ADR-0224 body + STATE/ROADMAP (29.1 done; row 29 in-progress) + next-prompt 29.2-SPEC + six-gate green"
```

---

## Execution acceptance checklist (SPEC §15.3 — verify before stage-close)

1. The `mongoproxy` package lands per §3 (config parse + BSON + request-side codec + the 23-stat eager roster + dynamic-name helpers + the active-query list + the no-op OnWrite stub); the framework is UNTOUCHED (§4 — `internal/filter/network/`, `manager.go`, `tcp_proxy`, HCM, `accesslog`, `prom.go`, `registry.go`, `fixture.go` all unchanged).
2. The 8th built-in + `bootstrap.go` blank-import + the `mongo.` four-rule name.go arm land (§3.8/§7.4).
3. Fixtures `0049` + `0050` green (label-aware StatsAsserter; the `TCPSink` backend; the AMEND-C2 presence-only + AMEND-C3 double-count constraints honored); the 39th fuzzer lands; counts: fixtures 50→**52**, fuzzers 38→**39**, stats 337→**360** (R6).
4. ADR-0224 §Decision/§Consequences in place (DECISIONS.md tail STAYS ADR-0226; no new number); the BEHAVIOR_CONTRACT 29.1 bundle lands (§14).
5. Six gates green (§15.2); STATE.md advanced; ROADMAP sub-row 29.1 `in-progress → done`; parent row 29 STAYS `in-progress`; next-prompt.txt rewritten for the 29.2-SPEC cold-start.

**Ratified-pending (SPEC §13) honored:** R1 (50-dir back-compat byte-exact), R2 (`TestStatRoster_MatchesUpstreamMacro`), R3 (chain-buffer never drained — unit + fuzz), R4 (`0049` deliberate-break with `-count=1`), R5 (active-query list written-not-read; populated by OP_QUERY only), R6 (counts re-pinned at Task 1), R7 (label-aware Prometheus parity — intrinsic in the §8.1.2 scrape).
