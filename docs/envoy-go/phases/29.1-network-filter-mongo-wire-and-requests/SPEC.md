# Phase 29.1 SPEC — `mongo_proxy` wire+BSON decode + request side

> **For agentic workers:** this is the per-sub-phase SPEC for **phase 29.1** (`network-filter-mongo-wire-and-requests`), the FIRST sub-phase of the phase-29 BRAINSTORM-time 3-way pre-split (29.1 / 29.2 / 29.3). It is authored per the phase-22.1 / phase-25.1 / phase-26.1 / phase-28.1 per-sub-phase-SPEC precedent: the **parent SPEC** (`docs/envoy-go/phases/29-network-filter-mongo-proxy/SPEC.md`) already resolved the BRAINSTORM §10 D29-1..D29-12 empirical pins IN-SESSION against reference Envoy v1.37.2 + go-control-plane v1.32.4 (parent §11), formalized the 3-way split surface-mapping (parent §3), and anchored the ADR-0224 + ADR-0225 + ADR-0226 §Context drafts in DECISIONS.md. This 29.1 SPEC **INHERITS** the parent SPEC's §5 proto roster + §6 PARSE-REJECT roster + §7 stat surface + §8 fixture taxonomy + §11 empirical-pin block + §13 RATIFIED-PENDING items, and **refines per-Task-level surface only** — with ONE exception: the **D-P2 dynamic-stat live re-probe** (the parent §11.1 caveat), which THIS session executed against a live Envoy v1.37.2 with a working backend (§11.2). The next session, per BOOTSTRAP §5, authors the **29.1 PLAN** (bite-sized TDD tasks) from this SPEC.

**Goal:** Land the `internal/filter/network/mongoproxy/` package's REQUEST side — TypeURL + 5-field config parse (incl. the FaultDelay PGV arms), the in-house little-endian BSON parser (the 14-type upstream subset), the MongoDB legacy wire decoder (the EXACTLY-7-opcode envelope; the 5 request opcodes body-decoded), the 23-stat fixed roster created EAGERLY under `mongo.<stat_prefix>.`, request-side increments + the dynamic `cmd.*`/`collection.*`/callsite counter families, and the per-connection active-query list — wired as the 8th built-in with the `mongo.` Prometheus TAG-EXTRACTOR arm (four-rule label hoisting per the §11.2 re-probe), proven by fixtures `0049-mongo-requests` (cross-side `StatsAsserter`) + `0050-mongo-boot-reject` and the 39th fuzzer.

**Architecture:** A NEW `internal/filter/network/mongoproxy/` package implements BOTH `ReadFilter` and `WriteFilter` (one instance per connection; consumer #2 of the ADR-0221 conn-wrap seam): the request-side wire decoder runs in `OnData` over a PRIVATE copy of the chain bytes (the chain `Buffer` is observational, never drained — the zookeeper `chainConsumed` high-water pattern adapts); `OnWrite` is a no-op `Continue` stub at 29.1 (the response decoder is 29.2); decode errors set sniffing off for the connection LIFETIME (AMEND-B6). ZERO framework changes: `internal/filter/network/` (chain.go / readconn.go / types.go), `internal/listener/manager.go`, `tcp_proxy`, and HCM are all untouched at 29.1. Cross-side `StatsAsserter` counter parity (with label-aware Prometheus scrape mechanics — §8.1.2) is the load-bearing differential proof.

**Tech Stack:** Go 1.26.2; go-control-plane v1.32.4 proto bindings (ADR-0008); reference Envoy v1.37.2 (ADR-0008); `internal/stats/` (06.1; `NewCounterIfAbsent` + `NewGaugeIfAbsent`); `internal/filter/network/` (26.1/26.2/27/28.1a/28.1b — consumed, not modified); the differential harness + `fixture.StatsAsserter` + the existing `TCPSink` BackendKind. ZERO new third-party `go.mod` dependencies (BSON decode is plain `encoding/binary` little-endian reads; the `$comment` callsite parse is stdlib `encoding/json`).

**Authored:** 2026-06-03. **Empirical-pin probe date (inherited):** 2026-06-03 (parent SPEC §11). **D-P2 re-probe date:** 2026-06-03 (THIS SPEC session — §11.2). **Baseline-anchor re-pin date:** 2026-06-03 (this SPEC session, master tip `159e80c` — §11.1).

---

## 1. Purpose / Mission

Phase 29.1 delivers the mongo_proxy decode foundation + request side (parent §3.2 item "29.1"):

1. **The `mongoproxy` package, request side (ADR-0224)** — the `internal/filter/network/mongoproxy/` package: TypeURL + 5-field config parse (`stat_prefix` PGV-required → boot-reject; `delay` parsed + PGV-validated here [AMEND-B9 arms] but consumed at 29.3; `access_log` parsed here, consumed at 29.3; `emit_dynamic_metadata` parsed here, consumed at 29.2; `commands` with the default `{delete, insert, update}` + the builtin-remembering semantics per AMEND-B7), the **in-house BSON parser** (`bson.go` — the 14-type upstream subset, throw-on-unknown, full eager parse), the **wire decoder request side** (`codec.go` — 16-byte LE MsgHeader; the 7-opcode envelope with `decoding_error` on everything else incl. OP_MSG; body decode for the 5 request opcodes; private-buffer copy/sniff model; sniffing-off-on-error), request-side fixed increments (13 of the 23), the dynamic `cmd.*`/`collection.*`/callsite counter families, and the **per-connection active-query list** (written at 29.1, consumed at 29.2) + the `op_query_active` gauge CREATED (increments live at 29.2).

2. **The integration surface** — (a) registration as the **8th `builtins.RegisterBuiltins` built-in** + the `mongo_proxy/v3` blank-import in `internal/bootstrap/bootstrap.go`; (b) the **`mongo.` TAG-EXTRACTOR arm** in `internal/stats/name.go` — per the §11.2 re-probe a FOUR-RULE label-hoisting extractor (prefix + cmd + collection + callsite labels; AMEND-C1), the ADR-0218 shape generalized to multi-label; (c) fixtures **`0049-mongo-requests`** (cross-side `StatsAsserter`, label-aware scrape) + **`0050-mongo-boot-reject`**; (d) the **39th fuzzer** `FuzzMongoDecode`; (e) the ADR-0224 §Decision/§Consequences body, the BEHAVIOR_CONTRACT 29.1 bundle, and the STATE/ROADMAP advance (sub-row 29.1 `in-progress → done` at IMPL phase-done; parent row 29 STAYS `in-progress` — the ROLLUP is 29.3's).

After phase 29.1, the project has: a request-side-observable `mongo_proxy` with live cross-side stat parity (stat surface 337 → 360 — the full 23-stat roster created); the active-query list + parsed-but-unconsumed `delay`/`access_log`/`emit_dynamic_metadata` config that 29.2/29.3 consume; and a decode foundation (BSON + framing) the 29.2 response decoder extends. 29.2 then completes the round-trip (OP_REPLY/OP_COMMANDREPLY decode + correlation + the gauge increments); 29.3 lands the async halt/resume seam + fault delay + access log + the parent ROLLUP.

### 1.1 Parent AMENDs load-bearing for 29.1 (per parent SPEC §1.1)

- **AMEND-B1** (stat scope is `mongo.<stat_prefix>.<counter>` — literal `mongo.` root, prefix is the MIDDLE segment) — informs §7.1 + the §8.1 StatsAsserter expectations.
- **AMEND-B2** (Prometheus exposition is TAG-EXTRACTED — the `name.go` arm is the ADR-0218 shape, NOT ADR-0138 inline-prefix) — informs §7.4; REFINED by this SPEC's AMEND-C1 (the dynamic families hoist MORE labels than the parent anticipated).
- **AMEND-B3** (the fixed roster is EXACTLY 22 counters + 1 gauge; `delays_injected` PLURAL; zero macro histograms; surface 337 → 360) — informs §7.2/§7.3 + the R2 roster test.
- **AMEND-B4** (fixed-stat creation is per-CONNECTION upstream; reference admin shows nothing until the first downstream connection) — informs §7.2 creation posture (D-P1) + the §8.1 post-first-connection assertion discipline.
- **AMEND-B5** (the decode envelope is EXACTLY 7 opcodes; OP_MSG(2013)/Msg(1000)/Update(2001)/Delete(2006) → `decoding_error`) — informs §3.5 dispatch + the §8.1 OP_MSG arm.
- **AMEND-B6** (decode error → `sniffing_ = false` → decode STOPS for the connection LIFETIME; fresh connection per error fixture arm) — informs §3.5 + §3.7 + the §8.1 arm topology.
- **AMEND-B7** (`commands` = builtin-REMEMBERING; unlisted → `cmd.unknown_command.total`; default `{delete, insert, update}`; alias normalization incl. `ismaster`→`isMaster`, `find`→query-path) — informs §3.3 + §3.6 + the §8.1 arm-2 proof (D-P8).
- **AMEND-B9** (FaultDelay PGV: the `fault_delay_secifier` oneof REQUIRED; `fixed_delay` gt 0s; two boot-reject parity arms) — informs §3.3/§6.2 (parse + unit-arms at 29.1; fixture consumption at 29.3 per D-P5).

(AMEND-B8/B10/B11/B12/B13 are 29.2/29.3-scoped and not load-bearing here; they are listed in §2 Non-purposes where they bound this sub-phase.)

### 1.2 29.1-SPEC-additive contributions (what this document pins beyond the parent)

- **§11.2 D-P2 RESOLVED by live re-probe (the parent §11.1 caveat closed).** This SPEC session re-ran the live probe with a WORKING backend (docker bridge network; the prior failure was a Docker-Desktop `--network host` artifact). Decode ran (op_query 4, decoding_error 0, upstream_cx_total 4); the dynamic-stat admin names are confirmed; the Prometheus form is pinned live + cross-checked against upstream `well_known_names.cc` source. Three 29.1-additive amendments result:
  - **AMEND-C1 (the dynamic Prometheus form is FULLY label-hoisted — REFUTES the parent D-P2 anticipation).** The parent anticipated "the `envoy_mongo_prefix` label hoist applies uniformly; dynamic segments stay in the metric name". Live + source: cmd/collection/callsite values NEVER appear in the Prometheus metric name — they are 100% label-encoded (`envoy_mongo_cmd_total{envoy_mongo_cmd="isMaster", envoy_mongo_prefix="…"}`; `envoy_mongo_collection_query_total{envoy_mongo_collection="collection1", …}`; the callsite family carries THREE labels). The `name.go` arm is therefore a FOUR-RULE tag extractor (§7.4), not the single-label extractor the parent sketched.
  - **AMEND-C2 (`cx_destroy_*_with_active_rq` increments on the REFERENCE during 29.1 fixtures → excluded from 29.1 value assertions).** The probe observed `cx_destroy_local_with_active_rq: 4` — the reference increments it whenever a connection closes with an unanswered OP_QUERY outstanding, which is EVERY query-bearing `0049` arm (the backend never replies at 29.1). envoy-go's `cx_destroy_*` increment paths land at 29.2 → the `0049` assertions treat the pair as PRESENCE-ONLY (name exists post-first-connection, value NOT compared); value parity is 29.2's `0051`.
  - **AMEND-C3 (callsite queries DOUBLE-COUNT).** A `$comment`-callsite query increments BOTH the plain `collection.<c>.query.*` family AND the `collection.<c>.callsite.<cs>.query.*` family (probe: `collection1.query.total: 2` = plain arm + callsite arm). The §8.1 arm-5 expectations account for this.
- **The `0049` backend pin: reuse `TCPSink` (BackendKind 28); NO new BackendKind at 29.1 (AMEND-C4).** The parent §8.1 sketched "a canned-bytes TCP responder (… need not reply)". This SPEC HARDENS that into: the `0049` backend MUST be silent — the existing `TCPSink` (28.1's §1.2 pin, `fixture.go` BackendKind 28). Reason (the same divergence mechanism 28.1 §1.2 documented): any bytes the backend writes flow back through reference Envoy's mongo `onWrite` response decoder → ref-side `op_reply`/`decoding_error` increments that envoy-go's 29.1 no-op `OnWrite` stub never mirrors. The canned-bytes **mongo responder** BackendKind (anticipated `TCPMongoResponder = 30` — the next free value; the `TCPZKResponder = 29` / 28.2 precedent) is a **29.2-SPEC concern** (fixture `0051` needs correlated OP_REPLY bytes); 29.1 creates no new runner plumbing.
- **The StatsAsserter LABEL-AWARE scrape-mechanics pin (§8.1.2).** The as-built `0046` zookeeper mechanics compare flat (label-less) Prometheus names. Mongo's tag-extracted exposition (AMEND-B2 + C1) requires the `0049` driver to parse `name{label="v",…}` lines into (name + canonical label set) → value maps on BOTH sides. The `0043` rbac driver (single-label) is the nearest precedent; `0049` generalizes it to multi-label.
- **The OP_REPLY/OP_COMMANDREPLY 29.1 posture pin (§3.5).** The two response opcodes are part of the valid 7-opcode envelope (a frame bearing them is NOT a `decoding_error`) but their BODY decode + counters land at 29.2. At 29.1 the request-path decoder recognizes them, consumes the frame, and decodes/counts nothing. The `0049` corpus contains no response-direction opcodes on the request path (clients never send them — corpus constraint, §8.1).
- **Parent D-question resolutions owned by 29.1** (§12.1): **D-P1** (creation posture — RESOLVED: EAGER at config parse), **D-P2** (dynamic-stat shapes — RESOLVED by re-probe), **D-P3** (name.go validation posture — RESOLVED: shape-based four-rule extractor), **D-P8** (commands-list fixture arm — RESOLVED: yes, arm 2). The **D-P5 unit-arm half** (the two FaultDelay PGV rejects unit-tested at 29.1) is scheduled here; the fixture-arm half stays D-P5 for the 29.3 SPEC.

---

## 2. Non-purposes

Phase 29.1 does NOT extend any subsystem beyond the minimum needed to land the request side under ADR-0224.

- **2.1 Response decoding OUT OF SCOPE.** OP_REPLY/OP_COMMANDREPLY body decode (in `OnWrite`), requestID↔responseTo correlation, the `op_query_active` gauge INCREMENTS, `op_reply*`/`op_command_reply` increments, and `cx_destroy_*_with_active_rq` increments are all **29.2** (parent §3.2). The 29.1 `OnWrite` is a no-op `Continue` stub (§3.7 — the 28.1 zookeeper precedent verbatim: NOT a buffer-feeding stub). The active-query list is WRITTEN at 29.1 but never read (R5).
- **2.2 Fault-delay CONSUMPTION out of scope.** The `delay` field is PARSED + PGV-VALIDATED at 29.1 (§3.3 + §6.2 — the reject arms exist as code + unit tests) but CONSUMED at 29.3 (the async halt/resume seam + timer + `delays_injected`). mongoproxy's 29.1 `OnData` ALWAYS returns `Continue` — it never halts.
- **2.3 Access-log + dynamic-metadata CONSUMPTION out of scope.** `access_log` is parse-and-store (consumed at 29.3 per AMEND-B10's unit-test fallback); `emit_dynamic_metadata` is parse-and-store (consumed at 29.2 per AMEND-B11).
- **2.4 The framework is UNTOUCHED.** Zero changes to `internal/filter/network/` (chain.go / readconn.go / writeconn.go / types.go / callbacks.go / terminal.go / registry.go), `internal/listener/manager.go`, `tcp_proxy`, HCM, or `internal/accesslog/`. The async halt/resume seam is 29.3's (ADR-0226). mongoproxy at 29.1 is a pure consumer of the as-built ReadFilter + WriteFilter surfaces (the both-direction registration is what qualifies the chain for the 28.1b read seam per `reference_network_chain_terminal_handoff_ends_ondata`).
- **2.5 The dynamic HISTOGRAM families** (`cmd.<cmd>.reply_*`, `collection.<c>.query.reply_*`, callsite `reply_*`) — deferred per ADR-0060; the coverage-boundary record lands in the **29.2** bundle (where their counter siblings' response-side context lives), not 29.1's (parent §9).
- **2.6 OP_MSG / modern protocol decode** — upstream parity, not a gap (AMEND-B5): OP_MSG → `decoding_error`. The `0049` OP_MSG arm PROVES the parity.
- **2.7 Runtime-key gating unmirrored** (`mongo.proxy_enabled` etc.) — envoy-go has no runtime layer; behaves at key defaults. Recorded in the 29.1 BEHAVIOR_CONTRACT bundle as an envoy-go-strict departure (parent §7.5); the full record (incl. the fault keys) completes at 29.3.
- **2.8 No real-MongoDB-server fixtures; no OP_MSG corpus; no histograms; no per-route surface; no new conformance harness** — all per parent §2.
- **2.9 No retroactive zookeeper changes.** The zookeeperproxy package, its fixtures, and its name.go arm are untouched.

---

## 3. The `mongoproxy` package, request side (ADR-0224)

NEW Go package `internal/filter/network/mongoproxy/` (package `mongoproxy`, single-token-joined per the `directresponse`/`snicluster`/`zookeeperproxy` precedent). Implements BOTH `ReadFilter` and `WriteFilter` (one instance per connection — the zookeeperproxy both-directions shape, `zookeeperproxy.go:48-54`).

### 3.1 File split (lands at IMPL)

| File | Responsibility |
|---|---|
| `doc.go` | package doc — the mongo_proxy request side; ADR-0224 cross-refs; the 29.2/29.3 forward-pointers |
| `mongoproxy.go` | `TypeURL` (via `proto.MessageName` — §3.2) + `NewFactory(reg *stats.Registry)` + the `filter` struct glue (§3.7) |
| `config.go` | `compiledConfig` + `parseConfig` (5-field parse + PGV-mirror validation incl. the FaultDelay arms + the commands set with alias normalization) — §3.3 |
| `bson.go` | the in-house BSON parser (the 14-type subset; document/element walk; throw-on-unknown) — §3.4 |
| `codec.go` | MsgHeader framing + per-opcode request decode + the private-buffer reassembly + sniffing-off-on-error — §3.5 |
| `stats.go` | the 23-stat fixed roster (eager creation) + the dynamic-name helpers (cmd/collection/callsite) — §3.6 |
| `filter.go` | the ReadFilter/WriteFilter glue + the chain-buffer high-water tracking + the active-query list — §3.7 |
| `*_test.go` | per-file unit tests (§15.1) |
| `fuzz_test.go` | `FuzzMongoDecode` (the 39th fuzzer — §10 Task) |

(The parent §4.2 anticipated layout, refined: `filter.go` is split out of `mongoproxy.go` so the factory/TypeURL file stays small — the zookeeperproxy 95-LoC `zookeeperproxy.go` precedent.)

### 3.2 TypeURL + factory shape

```go
// TypeURL is derived via proto.MessageName, NEVER a hand-typed docs string
// (reference_network_filter_typeurl_extensions; the zookeeperproxy.go:17 precedent).
var TypeURL = "type.googleapis.com/" + string(proto.MessageName(&mongo_proxyv3.MongoProxy{}))

// NewFactory returns the mongoproxy NetworkFilterFactory with the stats
// registry closure-captured (the zookeeperproxy.go:26 / rbac precedent —
// network.FactoryCtx carries no stats registry).
func NewFactory(reg *stats.Registry) network.NetworkFilterFactory
```

Pinned: `proto.MessageName` resolves to `envoy.extensions.filters.network.mongo_proxy.v3.MongoProxy` (parent §5.1; the `extensions.` segment). The IMPL Task-1 pinning test asserts `TypeURL` ends in the parent §5.1 string — derivation by `proto.MessageName`, assertion against the empirically-pinned literal.

The factory parses + validates ONCE at boot (ADR-0079 two-step factory) and creates the 23-stat roster ONCE per distinct `stat_prefix` at parse time (§3.6 / D-P1 eager). The returned `FilterInstanceFactory` allocates a fresh `*filter` per connection, all sharing the boot-parsed `*compiledConfig` (incl. the shared roster stats — counter/gauge operations are atomic; the per-connection state is the decoder buffers + the active-query list, §3.5/§3.7).

### 3.3 Config parse (5 fields; the FaultDelay PGV recurse; the commands set)

Parses the FULL 5-field proto (parent §5.2 roster, inherited verbatim). Per-field 29.1 disposition:

| Field | 29.1 parse behavior |
|---|---|
| `stat_prefix` | REQUIRED (PGV min-1-rune mirror) → boot-reject `mongo_proxy: stat_prefix is required` (§6.1; the `0050` arm) |
| `access_log` | parse + store the path string; CONSUMED at 29.3 (any string accepted incl. empty = disabled — upstream parity, no reject) |
| `delay` | parse + PGV-validate when present: the `fault_delay_secifier` oneof REQUIRED (absent → reject); `fixed_delay` must be > 0s (else reject); `header_delay` parse-accept (no delay results — parent §5.3 / D-P5); `percentage` stored (FractionalPercent; out-of-range denominator → reject per parent §5.3 note). CONSUMED at 29.3 |
| `emit_dynamic_metadata` | parse + store the bool; CONSUMED at 29.2 |
| `commands` | parse into the REMEMBERED-COMMANDS set (AMEND-B7): empty list → the default `{delete, insert, update}`; an explicit list REPLACES the default; CONSUMED at 29.1 (§3.6 dynamic cmd naming) |

**Command-name alias normalization (AMEND-B7 / parent §11.3)** lands in `config.go` as a package-level table consumed by the decoder (§3.5): `collstats`→`collStats`, `dbstats`→`dbStats`, `findandmodify`→`findAndModify`, `getlasterror`→`getLastError`, `ismaster`→`isMaster`; `find` → cleared (handled as a query — the collection path, never a `cmd.*` stat). The normalization applies to DECODED wire command names before the remembered-set lookup, NOT to the configured `commands` list entries (upstream parity — `utility.cc:21-37` normalizes at decode time).

### 3.4 The in-house BSON parser (`bson.go`)

Mirrors upstream `bson_impl.cc` exactly (parent §11.4 item 5 — inherited verbatim; `reference_wire_format_both_sides_see_same_bytes`). All multi-byte reads are little-endian (`encoding/binary.LittleEndian`).

- **Document** = int32 docLength (INCLUDES itself + the trailing 0x00) + elements + 0x00 terminator. Nested documents recurse. Length-vs-content mismatch → error.
- **The 14 handled element types**: 0x01 Double, 0x02 String, 0x03 Document, 0x04 Array, 0x05 Binary, 0x07 ObjectId (12 bytes), 0x08 Boolean, 0x09 Datetime, 0x0A Null, 0x0B Regex (2 cstrings), 0x0E Symbol, 0x10 Int32, 0x11 Timestamp (int64), 0x12 Int64. **ANY other type byte** (incl. 0x06 Undefined, 0x0D JS code, 0x13 Decimal128) → error (`decoding_error` path) — upstream throw parity.
- **Strings** = int32 len (includes the trailing NUL) + bytes; **cstrings** = NUL-terminated. Truncation/underflow anywhere → error.
- **The parse is FULL/EAGER** (every element materialized; nested docs recursed) — upstream parity, and the decoder's field extraction (§3.5) needs first-element-key + `_id` type + `$comment`/`$maxTimeMS` lookups anyway.
- **API shape (anticipated; PLAN/IMPL finalizes — D-S29.1-2):** an internal `document` type with ordered elements (`[]element{name, typeByte, value}`) + lookup helpers (`first()`, `find(name)`); errors are plain Go errors that the codec converts into the `decoding_error` path. No exported API (package-internal; extract-at-second-consumer per the parent §4.2 YAGNI pin).

### 3.5 The wire decoder, request side (`codec.go`)

Mirrors the upstream decoder contract pinned at parent §11.4/§11.5. All multi-byte reads little-endian.

**Private-buffer copy model + reassembly (AMEND-B6 / parent §11.5).** The decoder owns its OWN `readBuf []byte`. Per `OnData`: append a COPY of the chain buffer's NEW bytes to `readBuf` (the chain `Buffer` is read, NEVER drained — passthrough is the chain runner's job), then loop: while `readBuf` holds a complete message (16-byte header readable AND `len(readBuf) >= messageLength`), decode it and consume it from `readBuf`; a trailing partial message stays for the next read (partial frames are NEVER an error — upstream parity). The "new bytes" tracking is the zookeeper `chainConsumed` high-water mark against `Buffer.TotalAppended()` (`zookeeperproxy/decoder.go:40-48` — the 28.1b §3.3 re-base basis), adapted verbatim (§3.7 / D-S29.1-4).

**MsgHeader** (parent §11.4): 16 bytes LE = messageLength (int32, INCLUDES itself) + requestID (int32) + responseTo (int32) + opCode (int32).

**The opcode dispatch (AMEND-B5):**

| Wire opcode | 29.1 treatment |
|---|---|
| Query (2004) | full body decode (§ below) + counters + active-query list append |
| Insert (2002) | body decode (flags + fullCollectionName + 1..N BSON docs) + `op_insert` |
| GetMore (2005) | body decode (ZERO + fullCollectionName + numberToReturn + cursorID) + `op_get_more` |
| KillCursors (2007) | body decode (ZERO + numberOfCursorIDs + cursorIDs) + `op_kill_cursors` |
| Command (2010) | body decode (database + commandName cstrings + metadata/commandArgs/inputDocs BSON) + `op_command` |
| Reply (1), CommandReply (2011) | **recognized-not-decoded at 29.1** (§1.2 pin): valid envelope → NOT a `decoding_error`; the frame is consumed; body decode + counters land at 29.2 |
| Msg (1000), Update (2001), Delete (2006), OP_MSG (2013), anything else | `decoding_error` path (upstream `EnvoyException("invalid mongo op N")` parity) |

**OP_QUERY body decode** (the load-bearing path; parent §11.4 item 4): flags (int32) → fullCollectionName (cstring) → numberToSkip (int32) → numberToReturn (int32) → query (BSON doc) → OPTIONAL returnFieldsSelector (BSON doc, iff body bytes remain). From the decoded fields:

- **Flag counters** (bit values per parent §11.4): TailableCursor 0x02 → `op_query_tailable_cursor`; NoCursorTimeout 0x10 → `op_query_no_cursor_timeout`; AwaitData 0x20 → `op_query_await_data`; Exhaust 0x40 → `op_query_exhaust`.
- **Collection extraction**: the substring of fullCollectionName after the FIRST `.` (no dot → `decoding_error` path — upstream `"invalid full collection name"` parity).
- **Command detection**: fullCollectionName contains `$cmd` → the command path: command name = the FIRST element key of the (possibly `$query`-nested) command document (empty doc → `decoding_error` path — `"invalid query command"` parity); alias normalization (§3.3) → the remembered-set lookup → `cmd.<name>.total` or `cmd.unknown_command.total` (§3.6). `find` commands route to the QUERY path (collection stats) per AMEND-B7. $cmd queries do NOT increment `op_query_no_max_time` (the §11.2 probe confirmation).
- **Query-shape heuristics** (non-command queries; parent §11.3): no `_id` field → ScatterGet (`op_query_scatter_get` + `collection.<c>.query.scatter_get`); `_id` of type Document/Array → MultiGet (`op_query_multi_get` + `.query.multi_get`); scalar `_id` → PrimaryKey (only `.query.total`). Every non-command query → `collection.<c>.query.total`.
- **`$maxTimeMS`** (fallback key `maxTimeMS`; Int32/Int64) → maxTime; non-command queries with maxTime < 1 → `op_query_no_max_time`.
- **`$comment`** (String) parsed as JSON → field `"callingFunction"` → the callsite name (any parse failure → empty → no callsite stats); a non-empty callsite → the `collection.<c>.callsite.<cs>.query.*` family IN ADDITION to the plain collection family (AMEND-C3 double-counting).
- **Every decoded OP_QUERY** → `op_query` +1 (incl. $cmd queries — the §11.2 probe: op_query counted 4 with 2 of them $cmd) + an active-query list append (§3.7).

**The `decoding_error` path (AMEND-B6).** Any decode error (unknown opcode, BSON error, collection-name error, empty command doc, buffer underflow inside a complete frame): `decoding_error` +1 (at most ONCE per connection — the increment and the flag-set are atomic) + **`sniffing = false`** — decode permanently stops for this connection; subsequent `OnData`/`OnWrite` calls update the high-water mark and drop the bytes without decoding (the private buffer is released). Passthrough unaffected; the connection is NEVER closed; `OnData` still returns `Continue`. (Structurally different from zookeeper's abandon-buffer-keep-decoding — pinned + unit-tested.)

### 3.6 The 23-stat roster + dynamic-name helpers (`stats.go`)

**Creation posture (D-P1 RESOLVED: EAGER at config parse).** ALL 23 fixed stats (22 counters + the `op_query_active` gauge) are created EAGERLY at config parse via `reg.NewCounterIfAbsent("mongo.<stat_prefix>." + suffix)` (`registry.go:157-171` — post-Freeze-permitted; idempotent across listeners sharing a prefix) + `reg.NewGaugeIfAbsent("mongo.<stat_prefix>.op_query_active")` (`registry.go:191-205`). Rationale (the zookeeper D-P5 precedent): (i) freeze-after-boot friendly; (ii) the response/fault counters exist-at-zero so the 29.2/29.3 increments cannot regress creation; (iii) the upstream per-connection creation difference (AMEND-B4) is UNOBSERVABLE to the differential because every `0049` assertion runs post-first-connection — recorded as the boot-window BEHAVIOR_CONTRACT departure (§7.5/§9).

**The roster table.** A `rosterSuffixes()` table (the `zookeeperproxy/stats.go:159-175` shape) producing the EXACT 22 counter suffixes of parent §7.2 (`cx_destroy_local_with_active_rq`, `cx_destroy_remote_with_active_rq`, `cx_drain_close`, `decoding_error`, `delays_injected`, `op_command`, `op_command_reply`, `op_get_more`, `op_insert`, `op_kill_cursors`, `op_query`, `op_query_await_data`, `op_query_exhaust`, `op_query_multi_get`, `op_query_no_cursor_timeout`, `op_query_no_max_time`, `op_query_scatter_get`, `op_query_tailable_cursor`, `op_reply`, `op_reply_cursor_not_found`, `op_reply_query_failure`, `op_reply_valid_cursor`) + the gauge. The byte-stable **`TestStatRoster_MatchesUpstreamMacro`** test pins all 23 names against a sorted golden list transcribed from `ALL_MONGO_PROXY_STATS` (the R2 ratification; `delays_injected` PLURAL is the regression guard).

**Dynamic-name helpers** (lazily created at decode time via `NewCounterIfAbsent` — post-Freeze-permitted is REQUIRED here; the zookeeper `auth.<scheme>_rq` / rbac per-policy precedent):

- `cmdTotal(cmd string)` → `mongo.<sp>.cmd.<cmd>.total` (`<cmd>` = the remembered name or `unknown_command`)
- `collectionQuery(c string, leaf string)` → `mongo.<sp>.collection.<c>.query.<leaf>` (leaf ∈ total / scatter_get / multi_get)
- `callsiteQuery(c, cs string, leaf string)` → `mongo.<sp>.collection.<c>.callsite.<cs>.query.<leaf>`

NOT counted in the static 360 surface (config/traffic-dependent — parent §7.3).

### 3.7 Filter glue + the active-query list (`filter.go`)

```go
type filter struct {
	network.Marker
	cfg     *compiledConfig // shared, boot-parsed (incl. the roster stats)
	dec     *decoder        // per-connection (private readBuf + sniffing flag + chainConsumed)
	queries []activeQuery   // per-connection active-query list (written 29.1; consumed 29.2)
	cb      network.ReadFilterCallbacks
	wcb     network.WriteFilterCallbacks
}

// activeQuery carries what 29.2's correlation + the dynamic reply-side stats need.
type activeQuery struct {
	requestID int32
	collection string
	command   string    // empty for non-$cmd queries
	callsite  string    // empty unless a $comment callsite was present
	start     time.Time // recorded at 29.1 (cheap; avoids a 29.2 struct revision)
}
```

- **`OnNewConnection() → network.Continue`** — no-op (the `reference_network_read_filter_onnewconnection_halts` constraint).
- **`OnData(buf, endStream) → network.Continue` ALWAYS** — feeds the decoder with the chain buffer's NEW bytes via the `chainConsumed` high-water mark against `buf.TotalAppended()` (the `zookeeperproxy/decoder.go:40-48` mechanism adapted verbatim — D-S29.1-4); never drains the chain buffer, never closes, never halts (R3). Note: at 29.1 mongoproxy has NO halt path at all (fault delay is 29.3) — `Continue` is unconditional.
- **`OnWrite(buf, endStream) → network.Continue` ALWAYS — a PURE NO-OP at 29.1** (the 28.1 zookeeper OnWrite-stub pin verbatim): it does NOT buffer write-direction bytes (no response decoder to drain them → unbounded growth on long-lived connections). The write-side private buffer is created WITH the response decoder at 29.2. The stub exists so the filter satisfies `WriteFilter` end-to-end (the `0049` traffic DOES flow through `writeChainConn` → `OnWrite`).
- **`SetReadFilterCallbacks` / `SetWriteFilterCallbacks`** — store both (the both-directions dual injection — `chain.go` injects each exactly once).
- **`OnDestroy`** — drops the per-connection decoder + the active-query list (they die with the connection). Called exactly once (the 28.1a once-per-instance dedupe).
- **The active-query list**: appended on every successfully decoded NON-COMMAND OP_QUERY *and* $cmd OP_QUERY (upstream: every decodeQuery constructs an `ActiveQuery`); NEVER read at 29.1 (R5). Only OP_QUERY appends (GetMore/Insert/KillCursors/Command do NOT — parent §11.4 item 7). NO mutex at 29.1: the list is written only on the read path (single goroutine — pre-handoff dispatch or post-handoff `replayRead`, never both concurrently); the ADR-0223 per-connection mutex arrives at 29.2 WITH the cross-goroutine reader (`OnWrite` on pump B). Pinned in a `filter.go` comment with a 29.2 forward-pointer.
- **The `op_query_active` gauge** is CREATED at 29.1 (§3.6) but `Inc`/`Dec` calls land at 29.2 (the increments are correlation-coupled). A `filter.go` comment marks the 29.2 inc site (the list-append) + dec sites (correlated reply / destroy).

### 3.8 The 8th built-in + bootstrap blank-import

- `internal/filter/network/builtins/builtins.go`: `reg.Register(mongoproxy.TypeURL, mongoproxy.NewFactory(deps.StatsRegistry))` — the 8th registration, after zookeeperproxy (`builtins.go:68`), mirroring its closure-capture shape; the doc-comment's "seven built-in network filters (…)" (line 1) is updated to "eight … (…, mongo_proxy)". Registration order is behavior-neutral (ADR-0072).
- `internal/bootstrap/bootstrap.go`: blank-import `_ ".../envoy/extensions/filters/network/mongo_proxy/v3"` (after the zookeeper_proxy import at `bootstrap.go:95` — required for `@type` Any resolution at config load; differential bootstraps also need ≥1 cluster per `reference_network_filter_typeurl_extensions`).

---

## 4. Framework touchpoints — NONE (a pinned property of this sub-phase)

29.1's production diff to `internal/filter/network/` (excluding the new `mongoproxy/` subpackage), `internal/listener/`, `internal/filter/tcpproxy/`, and `internal/http/` is **ZERO files**. The both-direction filter boots through the existing `case network.ReadFilter` classification arm (the 28.1 §3.6 pin — no `manager.go` change); the WriteFilter registration qualifies the chain for the 28.1b read seam intrinsically. This zero-touch property is itself a regression gate: the FULL 50-dir existing fixture suite must stay byte-exact green with mongoproxy merely REGISTERED (not configured) — proving registration alone perturbs nothing.

---

## 5. Proto-field roster (cross-reference parent §5)

INHERITED VERBATIM from parent §5.1 (TypeURLs) + §5.2 (the 5-field `MongoProxy` table with PGV + defaults) + §5.3 (`FaultDelay` oneof + `FractionalPercent`). No re-transcription here. The 29.1 IMPL Task-1 gate re-confirms `proto.MessageName` + the field roster against go-control-plane v1.32.4 in-tree (bindings verified present at `~/go/pkg/mod/.../envoy@v1.32.4/extensions/filters/network/mongo_proxy/v3/` — §11.1) before writing the parser, per the 26.x/27/28.x Task-1 precedent.

---

## 6. PARSE-REJECT roster (cross-reference parent §6)

Per ADR-0080 byte-stable PARSE-REJECT discipline: each arm is a named constant with byte-stable wording verified by a `TestParseRejectConstants_ByteStable` table test at IMPL. The error prefix for all mongoproxy arms is **`mongo_proxy: `** (mirrors `zookeeper_proxy: ` — `zookeeperproxy/config.go:148-155`; exact wording finalized at IMPL — D-S29.1-1). Phase 29 has NO departure-class rejects (parent §6.1) — every arm mirrors an upstream PGV failure.

### 6.1 The load-bearing 29.1 arm (fixture-proven)

- **`mongo-stat-prefix-required`** — missing/empty `stat_prefix` → boot-reject (PGV min-1-rune mirror). Anticipated wording: `mongo_proxy: stat_prefix is required`. The `0050` fixture arm (§8.2): BOTH sides reject at boot; common stderr substring `stat_prefix`.

### 6.2 The FaultDelay PGV-mirror arms (code + unit tests at 29.1; fixture disposition is 29.3's — parent D-P5)

Because the 29.1 config parse validates the FULL proto (§3.3), these arms' parse code + unit tests land at 29.1:

- `mongo-delay-specifier-required` — `delay: {}` (the oneof absent) → boot-reject (PGV `required` mirror — AMEND-B9). Anticipated wording: `mongo_proxy: delay: a delay type must be specified`.
- `mongo-delay-fixed-delay-too-small` — `delay: {fixed_delay: 0s}` → boot-reject (PGV `gt 0s` mirror — AMEND-B9). Anticipated wording: `mongo_proxy: delay: fixed_delay must be greater than 0s`.
- `mongo-delay-percentage-denominator-invalid` — an out-of-range `percentage.denominator` enum value → boot-reject (parent §5.3 note: go-control-plane ships no generated FractionalPercent `Validate()`; envoy-go rejects for parity). Anticipated wording: `mongo_proxy: delay: percentage denominator is not a defined value`.

Whether these gain `0050` fixture arms at 29.3 (when `delay` is consumed) or stay unit-test-only is parent D-P5, resolved at the 29.3 SPEC. At 29.1 the `0050` fixture carries the `stat_prefix` arm ONLY (the zookeeper `0047` precedent).

### 6.3 Framework-level arms (existing; no new wording)

- Unknown network-filter `typed_config` type_url → the existing unified reject; mongoproxy joins the registry, no new arm.
- `access_log` / `commands` / `emit_dynamic_metadata`: NOT rejects (parse-accept — §3.3).

---

## 7. Stat surface (cross-reference parent §7)

### 7.1 Scope shape — `mongo.<stat_prefix>.<counter>` (AMEND-B1, inherited)

envoy-go mirrors upstream's internal naming exactly (the `StatsAsserter` + the Prometheus arm depend on it). Internal registration name = `mongo.<stat_prefix>.<suffix>` for all 23 fixed stats + the dynamic `cmd.*`/`collection.*`/callsite names.

### 7.2 The roster + creation posture (D-P1 RESOLVED: EAGER)

Inherited from parent §7.2 (the 23-name table + the per-family created/incremented split); refined into the §3.6 implementation shape (the suffix table + `NewCounterIfAbsent`/`NewGaugeIfAbsent` eager creation + the R2 roster test). **D-P1 RESOLVED: all 23 created eagerly at config parse**; the boot-window visibility difference vs upstream's per-connection creation (AMEND-B4) is recorded as a BEHAVIOR_CONTRACT departure that is UNOBSERVABLE to the differential (every `0049` assertion is post-first-connection).

29.1 increment surface (13 of 23): `op_query`, the 7 `op_query_*` flag/shape counters, `op_insert`, `op_get_more`, `op_kill_cursors`, `op_command`, `decoding_error`. The remaining 9 counters + the gauge exist at zero until 29.2 (`op_reply*`×4, `op_command_reply`, `cx_destroy_*`×2, gauge) / 29.3 (`delays_injected`, `cx_drain_close`).

### 7.3 Project stat-count delta — 337 → **360** at 29.1

All +23 land at 29.1 (creation parity). 29.2/29.3 add ZERO new fixed names (increments only). The dynamic cmd/collection/callsite counters are excluded from the static count (config/traffic-dependent; the rbac/zookeeper precedent). The BEHAVIOR_CONTRACT stat table gains the 23 rows in the 29.1 bundle (§9).

### 7.4 Prometheus exposition — the `mongo.` FOUR-RULE TAG-EXTRACTOR arm (AMEND-B2 + AMEND-C1; D-P2/D-P3 RESOLVED)

**The pinned transformation (live-probed + source-pinned, §11.2).** Reference Envoy applies FOUR tag-extraction rules to mongo stats (upstream `well_known_names.cc:86-98,180-181` — `addTokenized` patterns, transcribed verbatim in §11.2):

| Internal name shape | Prometheus name | Labels |
|---|---|---|
| `mongo.<sp>.<fixed>` (the 23 fixed stats) | `envoy_mongo_<fixed flattened>` | `envoy_mongo_prefix="<sp>"` |
| `mongo.<sp>.cmd.<cmd>.total` | `envoy_mongo_cmd_total` | `envoy_mongo_prefix` + `envoy_mongo_cmd="<cmd>"` |
| `mongo.<sp>.collection.<c>.query.<leaf>` | `envoy_mongo_collection_query_<leaf>` | `envoy_mongo_prefix` + `envoy_mongo_collection="<c>"` |
| `mongo.<sp>.collection.<c>.callsite.<cs>.query.<leaf>` | `envoy_mongo_collection_callsite_query_<leaf>` | `envoy_mongo_prefix` + `envoy_mongo_collection="<c>"` + `envoy_mongo_callsite="<cs>"` |

The dynamic segment values NEVER appear in the metric name (AMEND-C1 — distinct commands/collections/callsites collapse onto the same `# TYPE` family, differentiated by labels). The gauge emits `# TYPE envoy_mongo_op_query_active gauge` (envoy-go's `prom.go:62-66` MetricGauge handling already produces this).

**The `name.go` arm (D-P3 RESOLVED: SHAPE-based four-rule extractor; no allowlist).** In `flattenToProm`'s dispatch (after the `.zookeeper.` arm at `name.go:243-262`, before the final unrecognized-prefix error at `name.go:263`): detect the leading literal `mongo.` + a dot-free `<prefix>` segment; hoist `envoy_mongo_prefix`; then match the post-prefix remainder against the three dynamic-family shapes (`cmd.<x>.…`, `collection.<x>.query.…`, `collection.<x>.callsite.<y>.query.…`) hoisting their labels; flatten what remains (dots → underscores) into `envoy_mongo_<rest>`. Anticipated shape (production code at IMPL):

```go
// Phase-29.1 mongo_proxy TAG-EXTRACTOR detection (ADR-0224; parent AMEND-B2 +
// 29.1 AMEND-C1; the .rbac. ADR-0218 label-promotion precedent generalized to
// multi-label). Mirrors upstream's four addTokenized rules
// (well_known_names.cc — MONGO_PREFIX/MONGO_CMD/MONGO_COLLECTION/MONGO_CALLSITE).
// KEEP-IN-SYNC: internal/filter/network/mongoproxy/stats.go (the name builders).
if rest, ok := strings.CutPrefix(internal, "mongo."); ok {
	if idx := strings.IndexByte(rest, '.'); idx > 0 {
		prefix, tail := rest[:idx], rest[idx+1:]
		labels = append(labels, Label{Key: "envoy_mongo_prefix", Value: prefix})
		// cmd.<cmd>.<leaf...>   → hoist envoy_mongo_cmd
		// collection.<c>.[callsite.<cs>.]query.<leaf...> → hoist collection [+ callsite]
		tail = hoistMongoDynamicSegments(tail, &labels)
		base = "envoy_mongo_" + strings.ReplaceAll(tail, ".", "_")
		return base, labels, nil
	}
}
```

Validation is the SHAPE (the zookeeper D-P8 / wasm permissive precedent): dynamic names make an allowlist impossible. Label ordering in the emitted exposition must be deterministic (sorted by key — matching the reference's alphabetical label order observed in §11.2) so the `0049` label-aware comparison is stable; if `prom.go`'s multi-label rendering needs a sort, that ~3-line touch is the ONLY `internal/stats/` change beyond `name.go` (D-S29.1-5 verifies at IMPL).

### 7.5 envoy-go-strict / envoy-go-lenient departure flags (BEHAVIOR_CONTRACT 29.1 bundle)

Inherited from parent §7.5, the 29.1-landing subset: the boot-window stat-creation difference (D-P1 eager vs upstream per-connection — unobservable to the differential); runtime-key gating unmirrored (sniffing always on — the `mongo.proxy_enabled` default; the fault/logging keys recorded fully at 29.3); the OP_REPLY/OP_COMMANDREPLY recognized-not-decoded 29.1 posture (§1.2 — a SUB-PHASE boundary, not a permanent departure; closed at 29.2). The histogram coverage boundary is 29.2's; the access-log + dynamic-metadata notes are 29.2/29.3's.

---

## 8. Differential fixture taxonomy (+2; cross-reference parent §8)

Per `reference_differential_fixture_dispatch_constraint`: cross-side and boot-reject fixtures are SEPARATE dirs. Per `reference_differential_asserter_dispatch`: subject-side stat assertions use `fixture.StatsAsserter` (`fixture.go:75-77`; cross-side-path-only dispatch) and MUST be proven live via a deliberate-break with `-count=1` (`reference_differential_break_protocol_count1`). Numbering continues from `0048` (the verified tail, §11.1): 29.1 lands **`0049` + `0050`** → 50 → **52** dirs.

**Fixture-design constraints (parent §8 caveats + the §11.2 probe findings):** (i) the backend MUST be listening (tcp_proxy upstream-connect failure → the reference never decodes — the parent §11.1 probe-failure lesson); (ii) ALL stat assertions are post-first-connection (AMEND-B4); (iii) arms that fire `decoding_error` use FRESH connections (AMEND-B6 — sniffing-off is connection-lifetime); (iv) the backend MUST be silent (AMEND-C4 — `TCPSink`); (v) `cx_destroy_*_with_active_rq` are PRESENCE-ONLY at 29.1 (AMEND-C2); (vi) no response-direction opcodes in the corpus (§1.2).

### 8.1 `0049-mongo-requests` (cross-side; the load-bearing fixture)

**Topology.** Chain `[mongo_proxy, tcp_proxy]` on BOTH sides (reference Envoy v1.37.2 docker + envoy-go subprocess). TWO listeners (the `0046` `MultiListenerDriver` precedent, `fixture.go:599-616`):

| Listener | mongo config | Purpose |
|---|---|---|
| `l_default` | `stat_prefix: mongo_a`; default `commands` (i.e. `{delete, insert, update}`) | arms 1, 3, 4, 5, 6, 7, 8 |
| `l_commands` | `stat_prefix: mongo_b`; `commands: ["isMaster"]` | arm 2 (the AMEND-B7 / D-P8 proof) |

Both listeners route to ONE cluster → ONE `TCPSink` backend (BackendKind 28 — reused, §1.2/AMEND-C4; the driver implements `BackendKindAware` returning `fixture.TCPSink`).

#### 8.1.1 Driver wire-byte crafting

The driver hand-crafts little-endian legacy wire bytes (parent §11.4 layouts — MsgHeader + per-opcode bodies + BSON docs) via small builder helpers (`bsonDoc(...)`, `opQuery(reqID, collection, flags, queryDoc)`, `opInsert(...)`, …) — readable + reusable by the 29.2 `0051` driver (D-S29.1-3). Each arm drives BOTH sides identically; each error arm uses a FRESH connection.

#### 8.1.2 StatsAsserter mechanics — LABEL-AWARE both-sides Prometheus scrape

The driver implements `fixture.StatsAsserter`. Both sides are scraped via `GET /stats/prometheus`; lines are parsed into **(metric name + canonicalized sorted-label-set) → value** maps (generalizing the `0043` rbac single-label + `0046` zookeeper flat mechanics). Expected entries are keyed by flattened name + labels per the §7.4 table — e.g. `envoy_mongo_cmd_total{envoy_mongo_cmd="isMaster",envoy_mongo_prefix="mongo_b"}`. Consequence: every value assertion intrinsically asserts BOTH the Prometheus name parity AND the label-extraction parity (R7) — if envoy-go's §7.4 arm hoisted different labels or produced a different flat name, the lookup would miss and the assertion would fail. The gauge's `# TYPE … gauge` line is asserted present on both sides (the gauge-TYPE parity proof — creation only; value increments are 29.2's).

#### 8.1.3 Arms (refines parent §8.1's 8 arms with the §11.2 probe expectations)

1. **plain query** (`l_default`, fresh conn): OP_QUERY to `db.collection1` with query `{a: 1}` (no `_id`, no maxTimeMS) → `op_query` +1, `op_query_scatter_get` +1, `op_query_no_max_time` +1, `collection.collection1.query.total` +1, `collection.collection1.query.scatter_get` +1 — both sides (the §11.2 probe-arm-(a) expectation verbatim).
2. **$cmd + commands-list semantics** (`l_commands`, two fresh conns — the D-P8 proof): (i) OP_QUERY to `admin.$cmd` with `{isMaster: 1}` → `cmd.isMaster.total` +1 (in the list); (ii) OP_QUERY to `admin.$cmd` with `{foo: 1}` → `cmd.unknown_command.total` +1 (not in the list). Both arms also `op_query` +1 each; NEITHER increments `op_query_no_max_time` (the $cmd exclusion — §11.2).
3. **query-shape variants** (`l_default`, fresh conns): `{_id: 7}` scalar → PrimaryKey (only `collection.<c>.query.total`, no scatter/multi counters); `{_id: {$in-doc}}` (a Document-typed `_id`) → MultiGet (`op_query_multi_get` + `.query.multi_get`); a query with flag bits 0x02|0x10|0x20|0x40 → the four `op_query_*` flag counters +1 each.
4. **other request opcodes** (`l_default`, one conn): OP_INSERT → `op_insert` +1; OP_GET_MORE → `op_get_more` +1; OP_KILL_CURSORS → `op_kill_cursors` +1; OP_COMMAND → `op_command` +1 — each both sides. (None of these create active-query entries → this connection's close does NOT bump the reference's `cx_destroy_*` — a deliberate corpus choice.)
5. **$comment callsite** (`l_default`, fresh conn): OP_QUERY to `db.collection1` with `{a: 1, $comment: "{\"callingFunction\": \"fixtureFn\"}"}` → `collection.collection1.callsite.fixtureFn.query.total` +1 AND `collection.collection1.query.total` +1 (AMEND-C3 double-count; the assertion covers both families + the three-label Prometheus form).
6. **unsupported-opcode arm** (`l_default`, FRESH conn): an OP_MSG (2013) frame → `decoding_error` +1 both sides; the bytes still pass through to the backend (passthrough proven via the backend's receive count); NO further decode on this connection (a follow-up valid OP_QUERY on the SAME conn increments nothing — the AMEND-B6 sniffing-off proof, asserted cross-side).
7. **garbage-BSON arm** (`l_default`, FRESH conn): a well-framed OP_QUERY whose BSON document is malformed (bad element type 0x13) → `decoding_error` +1 both sides (the BSON throw-parity proof).
8. **exists-at-zero / creation + Prometheus-TYPE parity** (both prefixes, asserted after arms 1–7 with all connections closed): `op_reply`, `op_reply_cursor_not_found`, `op_reply_query_failure`, `op_reply_valid_cursor`, `op_command_reply`, `delays_injected`, `cx_drain_close` all PRESENT and == 0 both sides; the `op_query_active` GAUGE present, `# TYPE … gauge`, and == 0 both sides (quiesced — all connections closed, so the reference's ActiveQuery teardown has run); `cx_destroy_local_with_active_rq`/`cx_destroy_remote_with_active_rq` PRESENT both sides, value NOT compared (AMEND-C2).
9. **deliberate-break liveness proof** (R4; the `0030` lesson): recorded in driver comments + README + PROGRESS.md at IMPL — e.g. temporarily asserting `op_query == 2` (when 1 is sent) MUST fail on both runner paths with `-count=1`; temporarily disabling the §7.4 name.go arm MUST fail every assertion (the lookup misses).

### 8.2 `0050-mongo-boot-reject` (boot-reject; symmetric)

The `0047-zookeeper-boot-reject` precedent (202 LoC; `ExpectedBootErrorSubstring "stat_prefix"`). A `[mongo_proxy, tcp_proxy]` chain whose mongo `typed_config` has NO `stat_prefix` → BOTH sides reject at boot (PGV mirror — §6.1). Driver implements `fixture.Driver` + `differential.BootRejectFixture` (`harness.go:340-352`): `BootRejectScript() ""`; `ExpectedBootErrorSubstring() "stat_prefix"` (present in the reference's PGV violation text AND in envoy-go's `mongo_proxy: stat_prefix is required`). Symmetric mode. A minimal unused cluster satisfies the zero-cluster boot reject. The AMEND-B9 delay arms are unit-tested at 29.1 (§6.2), NOT fixture arms here (parent D-P5).

### 8.3 Total fixture-dir count

50 → **52** at 29.1 phase-done (+2). The full 50-dir existing suite is the back-compat regression gate (§4) and re-runs green at the six-gate. No new conformance harness.

---

## 9. Behavior-contract delta (the 29.1 bundle; per ADR-0052 atomic landing)

ONE atomic bundle at the 29.1 IMPL final task:

- NEW `### envoy.filters.network.mongo_proxy` subsection (after the zookeeper_proxy subsection): request-side decode semantics (§3.5); the 7-opcode envelope + OP_MSG-not-decoded (upstream parity, not a gap); the sniffing-off-on-error connection-lifetime semantics; the 23-stat roster + the EAGER creation posture + the boot-window departure (D-P1); the `mongo.<stat_prefix>.` scope; the Prometheus four-rule tag extraction (§7.4 table); the dynamic cmd/collection/callsite counter families + the commands-remembering semantics + alias normalization; the runtime-keys-at-defaults departure (the 29.1 subset); the OP_REPLY/OP_COMMANDREPLY recognized-not-decoded 29.1 boundary (closed at 29.2); forward-pointers to the 29.2 (response/correlation/gauge/metadata) + 29.3 (fault-delay/access-log/drain) bundles.
- Stat table: 337 → **360** (the 23 new rows).

---

## 10. Per-task structure (~17 tasks; the SPEC-anticipated task spine)

The 29.1 PLAN authors the exact bite-sized TDD tasks (the PLAN may merge/split); this is the SPEC-anchored spine:

| # | Task | Lands |
|---|---|---|
| 1 | First-action baselines/anchors gate: re-pin fixtures **50** (tail `0048`) + fuzzers **38** + stat surface **337** + DECISIONS tail **ADR-0226** (next-free **ADR-0227**) + `proto.MessageName` TypeURL pinning test + the §11.1 as-built anchors, against the live IMPL-session tip | §11 / R6 |
| 2 | `mongoproxy` package skeleton + `TypeURL` + `NewFactory` + config parse (5 fields + the commands set + alias table) + parse unit tests | §3.2 / §3.3 |
| 3 | FaultDelay PGV validation (oneof-required + gt-0s + denominator arms) + PARSE-REJECT constants + `TestParseRejectConstants_ByteStable` | §3.3 / §6 |
| 4 | `bson.go` part 1: document/element framing + the 14-type table + throw-on-unknown + underflow errors | §3.4 |
| 5 | `bson.go` part 2: nested doc recursion + string/cstring/Regex/Binary/ObjectId parsing + lookup helpers + malformed-doc unit arms | §3.4 |
| 6 | `codec.go` part 1: MsgHeader framing + private-buffer reassembly + partial-frame handling + the opcode dispatch (incl. recognized-not-decoded Reply/CommandReply + the `decoding_error`/sniffing-off path) | §3.5 |
| 7 | `codec.go` part 2: OP_QUERY body decode (flags + collection + command detection + query-shape heuristics + $maxTimeMS/$comment extraction) | §3.5 |
| 8 | `codec.go` part 3: OP_INSERT / OP_GET_MORE / OP_KILL_CURSORS / OP_COMMAND body decode | §3.5 |
| 9 | `stats.go`: the 23-stat eager roster + `TestStatRoster_MatchesUpstreamMacro` + the dynamic-name helpers (cmd/collection/callsite) | §3.6 / R2 |
| 10 | `filter.go`: the both-directions `filter` struct + chainConsumed high-water tracking + OnData/OnWrite/OnDestroy + the active-query list + multi-read/partial-frame/sniffing-off unit tests | §3.7 |
| 11 | The 8th built-in registration + `bootstrap.go` blank-import + a boot smoke test (a `[mongo_proxy, tcp_proxy]` bootstrap boots; the 23 stats exist at 0) | §3.8 |
| 12 | The `mongo.` four-rule `name.go` tag-extractor arm + flattening/label unit tests (fixed + cmd + collection + callsite shapes; deterministic label ordering) | §7.4 |
| 13 | The 39th fuzzer `FuzzMongoDecode` (random bytes → decoder: no panic, no chain-buffer mutation, sniffing-off idempotence) | §15.1 Layer C |
| 14 | `0049` driver part 1: bootstraps + the wire-byte/BSON builder helpers + MultiListener + `TCPSink` wiring | §8.1.1 |
| 15 | `0049` driver part 2: the label-aware StatsAsserter + arms 1–5 | §8.1.2 / §8.1.3 |
| 16 | `0049` driver part 3: arms 6–9 (error arms + exists-at-zero + deliberate-break recording) — the fixture goes green cross-side; `0050-mongo-boot-reject` fixture | §8.1.3 / §8.2 / R4 |
| 17 | Completion bundle: BEHAVIOR_CONTRACT 29.1 bundle (§9) + ADR-0224 §Decision/§Consequences body in-place (ADR-0044) + STATE.md + ROADMAP sub-row 29.1 `in-progress → done` + next-prompt.txt + the six-gate (incl. the FULL 52-dir differential suite + the 50-dir back-compat gate) | §9 / §15.2 |

### 10.1 ADR-0045 split-gate — SPEC-level re-check (parent §15 row "29.1")

Production-LoC estimate against the §3/§7 refined surface (the 26.x accounting basis: production code; fixture drivers + unit tests EXCLUDED):

| Deliverable | Production LoC |
|---|---|
| `config.go` (5-field parse + FaultDelay PGV + commands set + alias table) | ~130–180 |
| `bson.go` (14 types + doc walk + lookups) | ~250–350 |
| `codec.go` (framing + reassembly + 5 request-opcode decode + error path) | ~280–380 |
| `stats.go` (roster + dynamic-name helpers) | ~100–150 |
| `filter.go` + `mongoproxy.go` + `doc.go` (glue + active-query list + factory) | ~150–200 |
| builtins + bootstrap.go + the `name.go` four-rule arm (larger than parent estimate per AMEND-C1) | ~100–150 |
| The 39th fuzzer | ~60 |
| **Total (production basis)** | **~1070–1470** |

**Verdict: fits as ONE sub-phase on the production-LoC basis** (under the ~1500 gate; the ~17-task spine is under the ~25-task gate) — the upper bound is tighter than the parent's ~920–1330 estimate because the AMEND-C1 four-rule extractor + the recognized-not-decoded dispatch grew the surface, but it does not trip the gate. The fixture drivers (~700–1000 LoC across `0049`+`0050`; the `0046`/`0047` precedents being 875+202) are excluded per the 26.x accounting precedent. **The 29.1 PLAN remains the FINAL gate-check** (parent §3.0): if the bite-sized TDD decomposition exceeds ~25 tasks, the pre-authorized split axis is **29.1a** (Tasks 1–11: the package + builtins/bootstrap + `0050`) / **29.1b** (Tasks 12–17: the name.go arm + fuzzer + `0049` + completion bundle) — chosen so 29.1a ships a bootable, unit-proven filter and 29.1b ships its differential proof.

---

## 11. SPEC-time empirical-pin block

The 29.1 SPEC does NOT re-execute the parent §11 D29-1..D29-12 pins (resolved once at the parent SPEC; inherited — §1.1) — EXCEPT the D-P2 dynamic-stat re-probe (§11.2), which the parent explicitly delegated to this session (parent §11.1 caveat + §12 D-P2).

### 11.1 D-S1 — master-tip baselines + as-built anchors VERIFIED at this SPEC session

Verified against master tip **`159e80c`** (the docs-only next-prompt repoint trailing the parent-SPEC squash `ab376d8` by +2) at this SPEC session. These are the source of the §10 Task-1 first-action gate; the IMPL Task-1 RE-RUNS them against the live IMPL-session tip.

- **Differential fixture-dir count = 50**; numbering tail = **`0048-zookeeper-responses`**. 29.1 lands `0049` + `0050` → **52**.
- **Fuzzer count = 38** (`grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l`). The 29.1 `FuzzMongoDecode` is the **39th**.
- **Stat surface = 337** (BEHAVIOR_CONTRACT stat table; `BEHAVIOR_CONTRACT.md:466` "Phase 28.1 extension — 136 → 337 internal names"). 29.1 lands +23 → **360**.
- **DECISIONS.md tail = ADR-0226** (`DECISIONS.md:14421` ADR-0224, `:14440` ADR-0225, `:14459` ADR-0226 — the three §Context drafts from the parent SPEC); **next-free ADR-0227**. 29.1 IMPL fills the ADR-0224 §Decision/§Consequences body IN PLACE (no new ADR number; ADR-0044).
- **go-control-plane v1.32.4 mongo bindings present**: `~/go/pkg/mod/github.com/envoyproxy/go-control-plane/envoy@v1.32.4/extensions/filters/network/mongo_proxy/v3/` (mongo_proxy.pb.go + .pb.validate.go + _vtproto.pb.go).
- **As-built anchors re-verified** (the §3/§7/§8 design anchors): `internal/stats/name.go:38-273` (`flattenToProm`; the `.rbac.` tag-extractor arm `:226-241`; the `.zookeeper.` arm `:243-262`; the wasm permissive arm `:88-122`; the default error `:263`); `internal/stats/registry.go:157-171` (`NewCounterIfAbsent`, post-Freeze-permitted) + `:191-205` (`NewGaugeIfAbsent`, post-Freeze-permitted); `internal/stats/gauge.go:32-56` (Gauge Inc/Dec/Load); `internal/stats/prom.go:62-66` (the `# TYPE` counter/gauge emission); `internal/filter/network/builtins/builtins.go:46-68` (7 registrations; zookeeperproxy at `:68`; doc-comment "seven built-in network filters" at `:1`); `internal/bootstrap/bootstrap.go:77,87,95` (the network-filter blank-imports; NO mongo_proxy import exists); `internal/filter/network/types.go:13-23` (Status) + `:29-48` (ReadFilter) + `:57-72` (WriteFilter); `internal/filter/network/zookeeperproxy/` (the consumer-#1 package shape: `zookeeperproxy.go:17` TypeURL, `:26-43` NewFactory, `:48-54` filter struct, `:65-95` OnData/OnWrite/OnDestroy; `config.go:148-155` PARSE-REJECT constants, `:168` parseConfig; `stats.go:159-175` rosterSuffixes, `:190-200` newRosterStats; `decoder.go:40-48` chainConsumed, `:62-70` the ADR-0223 mutex).
- **Differential-harness anchors**: `test/differential/fixture/fixture.go:15-52` (Driver), `:75-77` (StatsAsserter), the BackendKind roster `TCPEcho=0 … TCPSink=28, TCPZKResponder=29` (next-free **30**), `:522-527` (BackendKindAware), `:599-616` (MultiListenerDriver); `test/differential/harness.go:340-352` (BootRejectFixture); `test/fixtures/0046-zookeeper-requests/driver/driver.go` (875 LoC; AssertStats `:605-690`); `test/fixtures/0047-zookeeper-boot-reject/driver/driver.go` (202 LoC); `test/fixtures/0048-zookeeper-responses/driver/driver.go` (865 LoC; the TCPZKResponder consumer).

### 11.2 D-P2 — the dynamic-stat live re-probe (RESOLVED at this SPEC session)

**Probe date: 2026-06-03.** Live `envoyproxy/envoy:v1.37.2` docker boot with a `[mongo_proxy (stat_prefix: mongoprobe, commands: ["isMaster"]), tcp_proxy]` listener + a WORKING python TCP backend on a shared docker **bridge network** (cluster type STRICT_DNS → the backend container hostname). The parent §11.1 probe-harness failure is explained: Docker-Desktop's `--network host` shares the daemon-VM netns, not the host's — the bridge-network topology fixes it. Decode CONFIRMED ran: `tcp.tcp.downstream_cx_rx_bytes_total: 266` (the prior failure signal was 0), `cluster.backend.upstream_cx_total: 4`, `mongo.mongoprobe.op_query: 4`, `decoding_error: 0`.

**Probe arms** (each on a fresh connection, hand-crafted LE wire bytes): (a) OP_QUERY `db.collection1` `{a: 1}`; (b) OP_QUERY `admin.$cmd` `{isMaster: 1}`; (c) OP_QUERY `admin.$cmd` `{foo: 1}`; (d) OP_QUERY `db.collection1` `{a: 1, $comment: "{\"callingFunction\": \"probeFn\"}"}`.

**Admin /stats result (the dynamic-name pin — every line verbatim):**

```
mongo.mongoprobe.cmd.isMaster.total: 1
mongo.mongoprobe.cmd.unknown_command.total: 1
mongo.mongoprobe.collection.collection1.callsite.probeFn.query.scatter_get: 1
mongo.mongoprobe.collection.collection1.callsite.probeFn.query.total: 1
mongo.mongoprobe.collection.collection1.query.scatter_get: 2
mongo.mongoprobe.collection.collection1.query.total: 2
mongo.mongoprobe.op_query: 4
mongo.mongoprobe.op_query_no_max_time: 2
mongo.mongoprobe.op_query_scatter_get: 2
mongo.mongoprobe.cx_destroy_local_with_active_rq: 4
mongo.mongoprobe.decoding_error: 0
```

Findings folded into this SPEC: the dynamic admin names match the parent's anticipated shapes EXACTLY; `collection1.query.total = 2` (arms a + d — the AMEND-C3 callsite double-count); `op_query_no_max_time = 2` (arms a + d only — $cmd queries excluded); `cx_destroy_local_with_active_rq = 4` (every connection closed with an unanswered query — AMEND-C2).

**Prometheus /stats/prometheus result (the AMEND-C1 pin — verbatim):**

```
# TYPE envoy_mongo_cmd_total counter
envoy_mongo_cmd_total{envoy_mongo_cmd="isMaster",envoy_mongo_prefix="mongoprobe"} 1
envoy_mongo_cmd_total{envoy_mongo_cmd="unknown_command",envoy_mongo_prefix="mongoprobe"} 1
# TYPE envoy_mongo_collection_callsite_query_scatter_get counter
envoy_mongo_collection_callsite_query_scatter_get{envoy_mongo_callsite="probeFn",envoy_mongo_collection="collection1",envoy_mongo_prefix="mongoprobe"} 1
# TYPE envoy_mongo_collection_callsite_query_total counter
envoy_mongo_collection_callsite_query_total{envoy_mongo_callsite="probeFn",envoy_mongo_collection="collection1",envoy_mongo_prefix="mongoprobe"} 1
# TYPE envoy_mongo_collection_query_scatter_get counter
envoy_mongo_collection_query_scatter_get{envoy_mongo_collection="collection1",envoy_mongo_prefix="mongoprobe"} 2
# TYPE envoy_mongo_collection_query_total counter
envoy_mongo_collection_query_total{envoy_mongo_collection="collection1",envoy_mongo_prefix="mongoprobe"} 2
```

Fixed stats carry only `{envoy_mongo_prefix="mongoprobe"}`; `envoy_mongo_op_query_active` is the sole **gauge** TYPE; labels are emitted in alphabetical key order.

**Source cross-check (upstream v1.37.2 `well_known_names`):** `well_known_names.h:111-117` defines the four tag names `envoy.mongo_prefix` / `envoy.mongo_cmd` / `envoy.mongo_collection` / `envoy.mongo_callsite`; `well_known_names.cc` registers the four `addTokenized` extractors (`$` = captured label token, `*` = skipped token, `**` = skipped tail):

```
:86-87   addTokenized(MONGO_CALLSITE,   "mongo.*.collection.*.callsite.$.query.**");
:94-95   addTokenized(MONGO_COLLECTION, "mongo.*.collection.$.**.query.*");
:97-98   addTokenized(MONGO_CMD,        "mongo.*.cmd.$.**");
:180-181 addTokenized(MONGO_PREFIX,     "mongo.$.**");
```

The callsite + collection rules BOTH fire on a callsite stat (the collection rule's `**` spans `callsite.<cs>`), which is why callsite Prometheus lines carry all three labels. The live observation and the source rules agree exactly — the §7.4 transformation table is pinned by both.

---

## 12. SPEC-time D-questions — parent resolutions + 29.1-additive PLAN/IMPL questions

### 12.1 Parent D-questions RESOLVED at this SPEC

- **D-P1 (fixed-roster creation posture) — RESOLVED: EAGER at config parse** (§3.6/§7.2). The boot-window difference vs upstream's per-connection creation is a BEHAVIOR_CONTRACT departure, unobservable to the differential (post-first-connection assertions).
- **D-P2 (dynamic-stat live shapes) — RESOLVED by live re-probe** (§11.2). Admin names confirmed; Prometheus form = FULL label hoisting (AMEND-C1); the parent's "dynamic segments stay in the metric name" anticipation REFUTED.
- **D-P3 (name.go arm validation posture) — RESOLVED: SHAPE-based** four-rule extractor mirroring upstream's `addTokenized` rules; no allowlist (§7.4).
- **D-P8 (commands-list fixture arm) — RESOLVED: YES** — `0049` arm 2 exercises a non-default `commands` list + the `unknown_command` fallback on a dedicated listener (§8.1.3).
- **D-P5 (PARTIAL — the unit-arm half).** The two AMEND-B9 FaultDelay PGV reject arms (+ the denominator arm) land as code + unit tests at 29.1 (§6.2). The fixture-arm half (whether they gain `0050`-style fixture arms) stays D-P5, resolved at the 29.3 SPEC.

(Parent D-P4, D-P6, D-P7, D-P9, D-P10, D-P11, D-P12 are 29.2/29.3-owned and untouched here.)

### 12.2 29.1-additive D-questions for PLAN / IMPL resolution

- **D-S29.1-1 (PARSE-REJECT byte-stable wording).** Finalize the §6 arm wording + the `TestParseRejectConstants_ByteStable` table. **Resolution at:** IMPL Task 3. Anticipated prefix: `mongo_proxy: `.
- **D-S29.1-2 (BSON internal representation).** The `bson.go` document/element Go types (ordered `[]element` + lookup helpers anticipated) + how numeric types unify for the `_id`-shape and `$maxTimeMS` checks. **Resolution at:** PLAN / IMPL Tasks 4–5.
- **D-S29.1-3 (wire-byte builder helper shape).** The `0049` driver's frame/BSON builders (shared with the future `0051` driver). **Resolution at:** IMPL Task 14. Anticipated: `bsonDoc(...)/opQuery(...)/opInsert(...)` style builders in the driver package.
- **D-S29.1-4 (chainConsumed adaptation).** The high-water mark against `Buffer.TotalAppended()` (the `zookeeperproxy/decoder.go:40-48` / 28.1b §3.3 basis) adapted to mongoproxy — a field on the per-connection decoder; a multi-read unit test proves no double-count. **Resolution at:** IMPL Task 10.
- **D-S29.1-5 (multi-label Prometheus rendering).** Whether `internal/stats/prom.go`'s label rendering already emits multiple labels in sorted-key order (the reference's observed order), or needs a ~3-line sort. **Resolution at:** IMPL Task 12 (a unit test asserts the emitted line for a three-label callsite stat byte-matches the §11.2 form).
- **D-S29.1-6 (decoding_error single-increment discipline).** Whether the at-most-once-per-connection increment is enforced in the decoder (sniffing flag checked before inc) or the filter glue. **Resolution at:** IMPL Task 6. Anticipated: the decoder (the flag and the counter are co-located).

---

## 13. RATIFIED-PENDING items (cross-reference parent §13, scoped to 29.1)

- **R1 (back-compat).** The 50 existing fixture dirs stay byte-exact green at the 29.1 six-gate (mongoproxy registered but unconfigured in them — the §4 zero-touch property). (The parent R1's seam-back-compat half is 29.3's.)
- **R2 (roster + scope parity).** The 23-stat roster under `mongo.<stat_prefix>.` matches upstream name-for-name (incl. `delays_injected` plural). Ratified by `TestStatRoster_MatchesUpstreamMacro` + the `0049` arm-8 post-connection roster assertions.
- **R3 (passthrough invariant).** mongoproxy NEVER mutates/drains the chain buffer, never closes the connection, never returns StopIteration (at 29.1 — no fault delay exists yet). Decode errors → sniffing off + passthrough continues. Ratified by the `0049` arm-6/7 passthrough + connection-survival proofs + unit tests asserting the chain buffer is byte-identical before/after OnData.
- **R4 (StatsAsserter liveness).** Every `0049` stat assertion proven live via a recorded deliberate-break with `-count=1` (§8.1.3 arm 9).
- **R5 (correlation hand-off).** The active-query list is written-but-unread at 29.1 (the 28.1 correlation-structures precedent). At 29.1 a unit test asserts the list is populated by OP_QUERY decode (and NOT by Insert/GetMore/KillCursors/Command); ratified at 29.2 by the `0051` correlation arms.
- **R6 (counts).** IMPL Task 1 re-pins fuzzers 38→39, fixtures 50→52, stats 337→360, DECISIONS tail ADR-0226 (next-free ADR-0227) against the live IMPL-session tip (§11.1 recipes).
- **R7 (Prometheus parity).** envoy-go's `/stats/prometheus` mongo lines match the reference's tag-extracted shape (names + label sets + the gauge TYPE — the §7.4 table). Ratified intrinsically by the §8.1.2 label-aware both-sides-scrape mechanics.

---

## 14. BEHAVIOR_CONTRACT.md edit bundle

ONE atomic bundle at IMPL Task 17, per ADR-0052: the §9 enumerated edits (the mongo_proxy subsection + the 337→360 stat table delta + the departure/boundary records per §7.5 + the 29.2/29.3 forward-pointers).

---

## 15. Test surface + 29.1 IMPL acceptance checklist

### 15.1 Test surface (per parent §14, scoped to 29.1)

- **Layer A — mongoproxy unit tests**: config parse (all 5 fields; every §6 PGV-mirror arm incl. the three FaultDelay arms; the commands default/replace/remember semantics; alias normalization incl. `ismaster`→`isMaster` + `find`→query-path); BSON parse (each of the 14 types; throw-on-unknown-type incl. 0x06/0x0D/0x13; nested docs; malformed/truncated docs; string/cstring edge cases); wire decode (per-opcode body decode; partial-frame reassembly across reads; the recognized-not-decoded Reply/CommandReply posture; OP_MSG/Update/Delete/Msg → decoding_error; flag bits; query-shape heuristics; collection extraction + no-dot error; $cmd command-name extraction + empty-doc error; $maxTimeMS / $comment extraction incl. JSON-parse-failure fallback); sniffing-off-on-error (decode stops for connection lifetime; at-most-one decoding_error); the chainConsumed no-double-count multi-read test; the active-query list population (OP_QUERY only); the chain buffer never drained/mutated (R3).
- **Layer A — stats unit tests** (`internal/stats/`): the `mongo.` four-rule arm (fixed names; cmd names; collection names; callsite names — all four label shapes; deterministic label ordering; the dot-free-prefix guard; non-matching names still error).
- **Layer C — fuzz**: the 39th fuzzer `FuzzMongoDecode` (arbitrary bytes → decoder: no panic; chain buffer unmutated; sniffing-off idempotence — once an error fires, further input decodes nothing and increments nothing).
- **Layer D — differential**: `0049` (cross-side label-aware StatsAsserter; 9 arms) + `0050` (boot-reject) + the FULL 50-dir back-compat suite (R1) → 52/52 green.
- **Layer E — race**: `go test -race -short` across `internal/filter/network/...` + `internal/stats/` (the shared roster stats under concurrent connections).
- Per-task `gofmt -l` + `golangci-lint` on touched packages (`feedback_pertask_gofmt_lint`).

### 15.2 Six-gate checklist (per the 26/27/28 precedent)

`go build ./...` / `go vet ./...` / `golangci-lint run` / `go test ./... -race -short` / the FULL differential suite byte-exact (52 dirs incl. the 50-dir back-compat gate) / h2spec 53/53 + proxy-wasm 10/10 re-run (asserted-unaffected — phase 29.1 touches no HTTP path). All outputs quoted into PROGRESS.md (run honestly).

### 15.3 29.1 IMPL acceptance checklist

1. The `mongoproxy` package lands per §3 (config parse + BSON + the request-side codec + the 23-stat eager roster + dynamic-name helpers + the active-query list + the no-op OnWrite stub); the framework is untouched (§4).
2. The 8th built-in + `bootstrap.go` blank-import + the `mongo.` four-rule name.go arm land (§3.8/§7.4).
3. Fixtures `0049` + `0050` green (label-aware StatsAsserter; the `TCPSink` backend; the AMEND-C2/C3 assertion constraints honored); the 39th fuzzer lands; counts: fixtures 50→52, fuzzers 38→39, stats 337→360 (R6).
4. ADR-0224 §Decision/§Consequences body lands in place (DECISIONS.md tail STAYS ADR-0226; no new number consumed); the BEHAVIOR_CONTRACT 29.1 bundle lands (§14).
5. Six gates green (§15.2); STATE.md advanced; ROADMAP sub-row 29.1 `in-progress → done`; parent row 29 STAYS `in-progress` (the ROLLUP is 29.3's); next-prompt.txt rewritten for the 29.2-SPEC cold-start.

---

## 16. Stage-close handoff

Per ADR-0004/0005: this SPEC is reviewed by the `spec-document-reviewer` subagent (≤3 iterations); on approval, ROADMAP sub-row 29.1 flips **`planned → in-progress` AT THIS SPEC COMMIT** (ADR-0106 / the 26.x/28.x precedent); parent row 29 STAYS `in-progress`; 29.2/29.3 STAY `planned`. ALSO at this commit: the ROADMAP 29.1 row's stale "`.mongo.` Prometheus inline-prefix arm (ADR-0138 / `.zookeeper.` precedent)" text is corrected in place to the ADR-0218 tag-extractor shape (the AMEND-B2 in-place edit the parent SPEC §16 scheduled — the ADR-0044 discipline). STATE.md advances to lifecycle-state 2-for-29.1-PLAN with `next-skill = superpowers:writing-plans` scoped to the **29.1 PLAN** (`docs/envoy-go/phases/29.1-network-filter-mongo-wire-and-requests/PLAN.md`). The SPEC is squash-merged to master + pushed; next-prompt.txt is rewritten for the 29.1-PLAN cold-start. Per `feedback_execution_style` the 29.1 IMPL runs `superpowers:subagent-driven-development`; per `feedback_git_worktrees`/`feedback_subagents_no_push`/`feedback_push_to_origin` the established worktree/push discipline applies.

---

## Appendix A — Cross-references to parent SPEC

| 29.1 SPEC § | Parent SPEC § | Relationship |
|---|---|---|
| §1 Purpose | parent §1 + §3.2 (29.1 detail) | refines |
| §1.1 AMENDs | parent §1.1 (B1–B9) | inherits the 29.1-load-bearing subset |
| §1.2 Additive pins | — | NEW (AMEND-C1..C4; TCPSink; label-aware scrape; Reply posture; D-P resolutions) |
| §2 Non-purposes | parent §2 + §3.2 | refines (29.1-scoped) |
| §3 mongoproxy package | parent §4.2 + §11.3/§11.4/§11.5 | refines into file split + production shapes |
| §4 Framework touchpoints | parent §4.1 (the seam is 29.3's) | pins ZERO-touch at 29.1 |
| §5 Proto roster | parent §5 | INHERITS verbatim |
| §6 PARSE-REJECT | parent §6 | refines (29.1 arms; wording at IMPL) |
| §7 Stat surface | parent §7 | refines; resolves D-P1/D-P2/D-P3 |
| §8 Fixtures | parent §8.1/§8.2/§8.5 | refines (arms + TCPSink + label-aware scrape + AMEND-C2/C3 constraints) |
| §9 Behavior contract | parent §9 (29.1 bundle) | refines |
| §10 Tasks + split-gate | parent §11.11 + §15 (29.1 row) | NEW (task spine); gate re-check |
| §11 Empirical pins | parent §11 (D-S1 re-pin + the delegated D-P2 re-probe) | inherits; ADDS §11.2 |
| §12 D-questions | parent §12 | resolves D-P1/P2/P3/P8 (+ D-P5 unit half); adds D-S29.1-1..6 |
| §13 RATIFIED-PENDING | parent §13 (R1–R8) | scoped to 29.1 |

## Appendix B — Phase 29.1 ADR landing summary

- **ADR-0224** (the `mongo_proxy` filter, request side) — §Context drafted at the parent SPEC (`DECISIONS.md:14421`); §Decision + §Consequences bodies land at 29.1 IMPL Task 17 per ADR-0044. This SPEC's §3 + §7 + §8 are the body's blueprint: the package (§3.1–§3.8), the BSON parser (§3.4), the codec (§3.5), the eager roster + D-P1 (§3.6), the four-rule tag-extractor arm + D-P2/D-P3 + AMEND-C1 (§7.4), the fixtures + the TCPSink/label-aware/AMEND-C2/C3 pins (§8).
- DECISIONS.md tail STAYS **ADR-0226** at 29.1 phase-done (no new ADR number consumed); next-free **ADR-0227**. The ADR-0225 body lands at 29.2; the ADR-0226 body at 29.3.
