# Phase 31 SPEC — `kafka_broker` network filter (`envoy.filters.network.kafka_broker`): header-only decode + full per-API-key request/response counter parity (the project's first `/contrib` filter)

> **For agentic workers:** the NEXT lifecycle step is `superpowers:writing-plans` (PLAN authoring; SKILL_ROUTING state 2 → 3). This SPEC is the input to that PLAN. Steps are NOT checkboxes here — the PLAN decomposes §10 into bite-sized TDD tasks. This is a SINGLE flat §9 row (directly executable; SPEC → PLAN → IMPL), NOT a parent pre-split (contrast phase 29). The pre-authorized 31.1/31.2 split axis stays unconsumed (§3.0 / D31-8).

**Goal:** Land `envoy.filters.network.kafka_broker` — a passive both-direction Kafka observability sniffer that decodes the Kafka request/response **HEADER ONLY** (generic across all API keys) and emits FULL per-API-key `request.<msg>_request` / `response.<msg>_response` counter parity in both directions (recovering the api_key on the response side via a `correlation_id`→`(api_key, api_version)` per-connection map) under scope `kafka.<stat_prefix>.`, in ONE flat phase, framework-zero-touch, as the project's FIRST `github.com/envoyproxy/go-control-plane/contrib` consumer.

**Architecture:** A NEW `internal/filter/network/kafkabroker/` package implements BOTH `ReadFilter` (request-header decode in `OnData`) and `WriteFilter` (response-header decode in `OnWrite`) — consumer #3 of the ADR-0221 conn-wrap seam (after zookeeper_proxy and mongo_proxy). An in-house `encoding/binary` Kafka primitive decoder (INT16 / INT32 / NULLABLE_STRING / tagged-fields) over a 4-byte INT32 length prefix; a static `api_key → message-name` table (86 keys, Kafka 3.9.1) + a static `flexibleVersions(api_key)` predicate table (the `(api_key, api_version) → tagged-fields-in-header` rule); a per-connection `correlation_id → (api_key, api_version)` map under an ADR-0223-style mutex; and an EAGERLY-created 176-counter fixed roster under `kafka.<stat_prefix>.`. ZERO framework-seam extension (pure consumer of the as-built §9 machinery, incl. the 28.1b post-handoff read seam — the filter implements `WriteFilter` even where observation-only to qualify the chain). Cross-side `StatsAsserter` counter parity is the load-bearing differential proof (the filter never mutates bytes — a body differential is vacuous).

**Tech stack:** Go 1.26.2 / golangci-lint 1.64.8 (ADR-0009); reference Envoy **`envoyproxy/envoy:contrib-v1.37.2`** (ADR-0227, the contrib variant of v1.37.2); go-control-plane `/envoy` v1.32.4 + the NEW `/contrib` v1.32.4 (ADR-0008/ADR-0227). Reuses `internal/filter/network/` (26.1/26.2/27/28.1a/28.1b — ReadFilter + WriteFilter chains + the post-handoff read seam), `internal/stats/` (06.1 counters + `internal/stats/name.go` tag-extractor), the ADR-0223 per-connection mutex pattern, `internal/filter/tcpproxy/` (the terminal), the differential harness + `StatsAsserter`. ONE new go.mod dep: `github.com/envoyproxy/go-control-plane/contrib v1.32.4` (proto bindings only; decode is in-house). ZERO framework-seam extension.

**Authored:** 2026-06-07. **Empirical-pin probe date:** 2026-06-07.

---

## 1. Purpose / Mission

Phase 31 lands `kafka_broker` (`envoy.filters.network.kafka_broker`, proto `envoy.extensions.filters.network.kafka_broker.v3.KafkaBroker` in the `/contrib v1.32.4` module), the **FIFTH §9 Network-filters-family row** (after the phase-26 family-parent, the phase-27 `sni_cluster` flat row, the phase-28 `zookeeper_proxy` parent, the phase-29 `mongo_proxy` parent; the phase-30 pin-refresh was an infra row, not a family member). It is the family's third stats-PRIMARY both-direction sniffer and **consumer #3 of the ADR-0221 `network.WriteFilter` seam**. It is the project's **FIRST `/contrib`-module consumer** — phase 30 (the contrib-image pin-refresh, ADR-0227) was its standalone infra prerequisite, unblocking the cross-side differential.

This SPEC refines the phase-31 BRAINSTORM (`docs/envoy-go/phases/31-network-filter-kafka-broker/BRAINSTORM.md`, Q0/Q1/Q-split decisions) against the AS-BUILT §9 framework + the §10 D31-1..D31-8 empirical pins EXECUTED IN-SESSION (parallel-subagent fan-out) against the contrib reference Envoy (`envoyproxy/envoy:contrib-v1.37.2`) + go-control-plane `/contrib` v1.32.4 + upstream Envoy v1.37.2 contrib source (Kafka 3.9.1). It anchors the ADR-0228 §Context draft into DECISIONS.md (§Decision/§Consequences body lands at the IMPL per ADR-0044).

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The §11 D31-1..D31-8 scrape CONFIRMED the two SPEC-blocking hypotheses (D31-1 TypeURL + dep; D31-2 the filter exists in the contrib image and decodes live) and REFINED/REFUTED several BRAINSTORM hypotheses. The load-bearing amendments, each carried into the relevant §§ below:

- **AMEND-K1 (D31-2 — stat NAMING is `_request`/`_response`-SUFFIXED message-name segments, NOT bare api-key names; REFINES BRAINSTORM §2.7/§5).** The per-key counters are `kafka.<sp>.request.<msg_name_snake>_request` / `kafka.<sp>.response.<msg_name_snake>_response` — e.g. `request.produce_request`, `request.api_versions_request`, `response.metadata_response`. The segment is `name_in_c_case()` of the **full message name** (including the `Request`/`Response` suffix), NOT the bare api-key name (`produce`). Cross-confirmed LIVE (`envoy_kafka_kprobe_request_api_versions_request{} 1`) + SOURCE (`contrib/kafka/.../protocol/request_metrics_h.j2`). See §7.1.
- **AMEND-K2 (D31-2 — Prometheus exposition is FULLY INLINED, ZERO label hoist; REFUTES the BRAINSTORM's label-hoist hypothesis).** kafka_broker inlines prefix + direction + api-key into the metric NAME with an EMPTY label set: `envoy_kafka_kprobe_request_api_versions_request{} 1` (live-probed, two real lines quoted in §11.2). This is the exact OPPOSITE of mongo (`envoy_mongo_op_query{envoy_mongo_prefix="…"}` — label hoist). The correct `internal/stats/name.go` precedent is the **zookeeper `.zookeeper.` INLINE shape (no labels)**, generalized to a `kafka.` ROOT segment — NOT the mongo/rbac tag-extractor shape the BRAINSTORM hypothesized. The arm is the simplest possible: `CutPrefix(internal, "kafka.")` → `base = "envoy_kafka_" + strings.ReplaceAll(rest, ".", "_")`, **no labels**. See §7.4.
- **AMEND-K3 (D31-2 — the roster is EAGER + 176 fixed counters; REFINES the BRAINSTORM's "dynamic family / 360 + roster TBD").** Upstream creates **86 request + 86 response** per-key counters + **4 fixed** (`request.unknown` / `request.failure` / `response.unknown` / `response.failure`) **EAGERLY** (`POOL_COUNTER_PREFIX` over the full `KAFKA_*_METRICS` macro, in the `Rich*MetricsImpl` constructor — per filter-instance = per connection, pool-deduped → the full roster is present-at-0 after the first downstream connection). envoy-go mirrors **EAGER creation at config parse** (the mongo D-P1 / zookeeper-roster precedent), giving roster-present-at-0 parity. Project stat surface **360 → 536 (+176)** — the project's largest single-phase stat jump (a FIXED roster, NOT a lazy dynamic family). The **86 response-duration histograms** (`response.<msg>_response_duration`) are DEFERRED project-wide (ADR-0060) — a coverage boundary, not mirrored. **api_keys 71/72 (telemetry) are EXCLUDED** upstream → **86** per direction (not 88). See §7.
- **AMEND-K4 (D31-4 — `unknown`/`failure` semantics REFINED; `failure` is partly header-reproducible).** `*.unknown` is keyed on **(api_key, api_version)** — an unknown VERSION of a known api_key ALSO routes to `unknown` (the Sentinel parser path), not only an unknown api_key. `*.failure` = an `EnvoyException` thrown out of decode: a malformed header (e.g. a `client_id` NULLABLE_STRING with an invalid length) on the request side, OR the **response-side unregistered-`correlation_id` lookup-miss** (`getResponseSpec` throws). The response-side `failure` IS header-reproducible (send a response whose correlation_id was never registered) → `response.failure` is NOT purely a unit-test boundary; it earns a `0053` differential arm. Only the "leftover-body-bytes → unknown" sub-case needs body decode (the one true coverage boundary). See §7.3 / §11.4.
- **AMEND-K5 (D31-3 — header-version is a `flexibleVersions` predicate with an ApiVersions(18) response special-case).** There is NO numeric header-version enum; two generated boolean predicates `requestUsesTaggedFieldsInHeader(api_key, api_version)` / `responseUsesTaggedFieldsInHeader(api_key, api_version)` decide whether the v2/flexible (tagged-fields) header form applies: a header uses tagged fields iff `api_version ∈ flexibleVersions(api_key)`. SPECIAL CASE: **ApiVersions (api_key 18) RESPONSE header suppresses tagged fields even for flexible versions** (the request side follows the normal rule). Kafka source is **3.9.1**; api-key ceiling 87. See §11.3 / Appendix C.
- **AMEND-K6 (D31-1 — only `contrib v1.32.4` is a new dep; the @type carries `extensions.`).** `proto.MessageName(&kafka_brokerv3.KafkaBroker{})` = `envoy.extensions.filters.network.kafka_broker.v3.KafkaBroker` (run in-session) → `@type` = `type.googleapis.com/…kafka_broker.v3.KafkaBroker` (carries the `extensions.` segment per `reference_network_filter_typeurl_extensions`). The kafka_broker/v3 package is SELF-CONTAINED (no transitive `/envoy`); `cncf/xds`, `protoc-gen-validate`, `vtprotobuf`, and `/envoy` are ALREADY in the project's go.mod closure (verified). The SINGLE genuinely-new direct dep is `github.com/envoyproxy/go-control-plane/contrib v1.32.4`; it RESOLVES and SURVIVES `go mod tidy` with its first consumer (the config parse + the `kafka_broker/v3` blank-import in `bootstrap.go`). See §11.1.
- **AMEND-K7 (D31-5 — the `IsValidName` charset guard is satisfied BY CONSTRUCTION).** The per-key stat segment is looked up from the STATIC 86-entry api-key table (never a raw wire string) — so NO arbitrary wire-derived dynamic segment ever reaches `NewCounterIfAbsent`. The charset guard (`reference_dynamic_stat_name_charset_guard`) is trivially satisfied (table-bounded names); the only config-derived segment (`stat_prefix`) is validated at parse via `stats.IsValidName` at the config boundary (the mongo/rbac precedent). Contrast mongo, where arbitrary BSON collection/command names REQUIRED the runtime guard. The `FuzzKafkaDecode` no-panic fuzzer still asserts the decode never panics + never mutates the chain buffer. See §11.5.

### 1.2 ADR continuity + D-disposition at SPEC commit

DECISIONS.md tail at master tip is **ADR-0227** (the pin-refresh, superseding ADR-0008); next-free **ADR-0228**. This SPEC anchors the ADR-0228 §Context draft (§Decision/§Consequences body at the phase-31 IMPL per ADR-0044). No ADR number is consumed beyond the §Context draft. The ADR-0209 escape-valve reserve carried from the §9 family STANDS-UNCONSUMED. All eight D31 pins are RESOLVED this session (§11); the remaining open items are PLAN/IMPL D-questions (§12), not empirical pins.

---

## 2. Non-purposes (deferred; per BRAINSTORM §8 + the §11 amendments)

- **Per-API-key BODY/payload decode** — out of scope by Q1 (header-only). The per-API-key counters need only the header (the api_key IS in the request header; the response side recovers it by correlation). NOT a gap — upstream's default-config counter surface is fully mirrored from the header.
- **The four active features' BEHAVIOR** — `force_response_rewrite`, `id_based_broker_address_rewrite_spec`, `api_keys_allowed`, `api_keys_denied`: PARSE-ACCEPTED (the config parses without error), kept at DEFAULTS in every fixture, behavior DEFERRED (the mongo `header_delay` parse-accept-no-op precedent). Recorded as a BEHAVIOR_CONTRACT coverage boundary. What each WOULD do (D31-4 context, §11.7): `force_response_rewrite` forces the rewriting rewriter (full response decode→re-encode); `id_based_broker_address_rewrite_spec` MUTATES the response write buffer (swaps broker host/port by node_id for Metadata(3)/FindCoordinator(10)/DescribeCluster(60) responses); `api_keys_allowed`/`api_keys_denied` GATE/close the connection (`close(FlushWrite, "denied (API key)")`) on a disallowed request api_key.
- **The broker-address-rewrite WRITE-BUFFER MUTATION** (`id_based_broker_address_rewrite_spec` / `force_response_rewrite`) — a NEW framework capability never built (every prior §9 sniffer — echo, sni_cluster, zookeeper, mongo, kafka — only OBSERVES bytes). If ever pursued, a future framework-SURGERY sub-phase (the write-mutation seam, analogous to the 29.3 async-halt seam). Flagged here.
- **`api_keys_allowed`/`api_keys_denied` enforcement** (connection close on a denied api_key) — a future enforcement sub-phase.
- **`response.<msg>_response_duration` HISTOGRAMS (86)** — deferred per ADR-0060 (project-wide histogram deferral); BEHAVIOR_CONTRACT coverage-boundary record. (There are NO request-side duration histograms — duration is response-side only.)
- **The "leftover-body-bytes → unknown" sub-case** — the body-length-mismatch path that upstream routes to `*.unknown` requires body decode to reproduce; out of scope (header-only). The unrecognized-(api_key, api_version) → `unknown` path IS header-reproducible and IS exercised. Unit-tested / coverage-boundary record.
- **Runtime-key gating** — envoy-go has no runtime layer; the filter behaves at key defaults (envoy-go-strict departure; the Runtime + hot-restart family row is the future home).
- **Real-Kafka-broker integration fixtures** — out of scope; hermetic synthesized frames only (a real broker speaks dozens of api-key bodies + nondeterministic handshake/metadata traffic that breaks exact counter parity).
- **Per-route applicability** — network filters carry no `typed_per_filter_config` surface (the phase-26/27/29 confirmation; re-confirmed by absence — the KafkaBroker proto has no `*PerRoute` message, §11.1). The ADR-0125 roster is untouched.
- **The remaining protocol proxies** — `redis`, `thrift` — each its own future family phase, each needing its own brainstorm-time risk assessment (terminal-proxy seams: command routing + upstream pooling).

---

## 3. The `kafka_broker` filter package + framework reuse (ADR-0228)

### 3.0 Split disposition — D31-8 RESOLVED (single flat phase; the pre-authorized 31.1/31.2 split stays unconsumed)

ADR-0045 split-gate fires at `> ~25 tasks OR > ~1500 production LoC`. Phase-31 surface (re-estimated against the §11 findings):

| Unit | Anticipated production LoC |
|---|---|
| `kafkabroker` package: TypeURL + 5-field config parse (+ PGV arms) | ~120–180 |
| Kafka primitive decoder (INT16/INT32/NULLABLE_STRING/tagged-fields/compact-types) + the 4-byte length-prefix framing + request-header decoder + response-header decoder | ~250–350 |
| The static `api_key → message-name` table (86 entries) + the `flexibleVersions(api_key)` predicate table (the data burden — DATA, not branching logic) | ~180–260 |
| The EAGER 176-counter roster + the eager-create helper + the `inc(direction, name)` accessors | ~80–130 |
| Filter glue (ReadFilter `OnData` + WriteFilter `OnWrite`) + the per-connection `correlation_id → (api_key, api_version)` map + the ADR-0223 mutex | ~140–220 |
| `internal/stats/name.go` `kafka.` INLINE arm (AMEND-K2) | ~20–35 |
| 9th `builtins.RegisterBuiltins` registration + `bootstrap.go` blank-import (the `/contrib` path) | ~15–25 |
| Fixtures `0053`/`0054` drivers + the new BackendKind | ~600–900 |
| The 40th fuzzer `FuzzKafkaDecode` | ~60 |

Net production ~805–1200 LoC (the two static tables are DATA, decoded with `encoding/binary` only — much leaner than mongo's BSON parser; the response header is trivial; NO fault-delay/async-halt seam), ~13–18 tasks — both axes comfortably UNDER the gate. **Single flat phase 31 — no pre-split.** The pre-authorized **31.1-request / 31.2-response** split axis (feature-progressive: 31.1 = length-prefix framing + request-header decode + the two tables + the api-key roster + `request.*` counters + the `kafka.` prom arm + the 9th built-in + bootstrap blank-import + 5-field config parse + the `/contrib` dep; 31.2 = response-header decode + the correlation map + `response.*` counters) STAYS UNCONSUMED (the mongo-29.1 "pre-authorized split stands unconsumed" precedent). The PLAN re-checks the gate at PLAN time per ADR-0045.

### 3.1 ZERO framework-seam extension (pure consumer)

Phase 31 touches NO `internal/filter/network/` framework code. It is a pure consumer of the as-built §9 machinery:

- the **ReadFilter chain** (26.1) — request-header decode in `OnData`;
- the **WriteFilter chain** (28.1a, ADR-0221) — response-header decode in `OnWrite` (consumer #3 after zookeeper_proxy and mongo_proxy);
- the **28.1b post-handoff read seam** (`readChainConn`/`replayRead`) — the steady-state response decode runs post-handoff, so the filter MUST implement `WriteFilter` even where observation-only to qualify the chain for the read seam (`reference_network_chain_terminal_handoff_ends_ondata`);
- the **ADR-0223 per-connection mutex pattern** (zookeeper 28.2 / mongo 29.2) — the `correlation_id → (api_key, api_version)` map is written on the read goroutine (`OnData`) and read/erased on the write goroutine (`OnWrite`), so the cross-goroutine state is mutex-guarded (single-owner state stays lock-free).

**ZERO-touch on the ADR-0226 async halt/resume seam** — kafka_broker injects no delay → never-halting → byte-identical R1 (contrast mongo_proxy's fault-delay). The filter never mutates/drains the chain buffer, never closes the connection, always returns `Continue` from `OnData`/`OnWrite` (it is a pure copying sniffer — the mongo private-buffer model, §11.5).

### 3.2 NEW: `internal/filter/network/kafkabroker/` (Go package `kafkabroker`)

Single-token-joined per the `directresponse`/`snicluster`/`zookeeperproxy`/`mongoproxy` precedent. Implements BOTH `ReadFilter` and `WriteFilter` (one instance per connection). Anticipated file layout (the PLAN finalizes the split):

- `kafkabroker.go` — `TypeURL` (via `proto.MessageName(&kafka_brokerv3.KafkaBroker{})`, pinned by an IMPL Task-1 test, NEVER hand-typed) + `NewFactory`.
- `config.go` — the 5-field parse (+ PGV arms, §6) + `stats.IsValidName(stat_prefix)` guard at the config boundary.
- `codec.go` — the Kafka primitive decoder (INT16/INT32/NULLABLE_STRING/UNSIGNED_VARINT tagged-fields) + the 4-byte INT32 length-prefix framing + the request-header decoder + the response-header decoder + decoder-internal reassembly (private-buffer copy model, §11.5).
- `apikeys.go` — the static `api_key → message-name` table (86 entries, Appendix B) + the `flexibleVersions(api_key)` predicate (`requestUsesTaggedFieldsInHeader`/`responseUsesTaggedFieldsInHeader`, with the ApiVersions(18) response special-case, Appendix C).
- `stats.go` — the EAGER 176-counter roster + the eager-create helper + the `incRequest(name)`/`incResponse(name)`/`incRequestUnknown()`/… accessors.
- `filter.go` — the ReadFilter (`OnData`) / WriteFilter (`OnWrite`) glue + the per-connection `correlation_id → (api_key, api_version)` map + the ADR-0223 mutex.
- `doc.go` — the package doc.

The decoder, both tables, the correlation map, and the roster all live INSIDE the package (extract-at-second-consumer; YAGNI). NO new top-level package; NO framework change.

### 3.3 Registration as the 9th built-in + the `/contrib` blank-import

- `internal/filter/network/builtins/builtins.go` — the **9th** `RegisterBuiltins` entry: `reg.Register(kafkabroker.TypeURL, kafkabroker.NewFactory(deps.StatsRegistry))` (stats-PRIMARY filter; the registry is closure-captured, the zookeeper/mongo precedent — the network `FactoryCtx` carries no stats registry). Mirror the parallel registration in `cmd/envoy-go/main.go` if it lists them explicitly (IMPL Task-1 greps to confirm — ADR-0072 makes order behavior-neutral).
- `internal/bootstrap/bootstrap.go` — a blank-import of the proto descriptor at the **`/contrib`** path: `_ "github.com/envoyproxy/go-control-plane/contrib/envoy/extensions/filters/network/kafka_broker/v3"` (the FIRST `/contrib` import in the project; the mongo/zookeeper blank-import precedent, lines 95/103 of `bootstrap.go`). This is what holds the new `contrib v1.32.4` dep through `go mod tidy` (an unused module dep is removed).

### 3.4 The `/contrib v1.32.4` go.mod dep (the project's first; D31-1)

Add `github.com/envoyproxy/go-control-plane/contrib v1.32.4` (lockstep with the existing `/envoy v1.32.4`). The kafka_broker/v3 package is self-contained (no transitive `/envoy`); the other transitive deps (`cncf/xds`, `protoc-gen-validate`, `vtprotobuf`) are ALREADY in the closure (verified — §11.1). The decode is in-house (`encoding/binary`); the dep is proto bindings only (consistent with the existing `/envoy` proto-binding posture; NO Kafka client library). The dep lands WITH its first consumer (§3.2/§3.3) — an unused module dep cannot survive `go mod tidy` (the phase-30 §2.4 deferral, consumed here).

### 3.5 REUSES (not new primitives)

- `internal/filter/network/` (26.1/26.2/27/28.1a/28.1b) — ReadFilter + WriteFilter chains, `Buffer`, freeze-after-boot `*Registry`, `chainRuntime`, `readChainConn`/`writeChainConn`/`prefixConn`, the post-handoff read seam, `builtins.RegisterBuiltins` (UNTOUCHED — pure consumer).
- `internal/stats/` (06.1) — counters + `NewCounterIfAbsent` (eager-roster idempotent across listeners sharing a `stat_prefix` — the mongo `newMongoStats`/zookeeper `newRosterStats` precedent) + `IsValidName` (config-boundary guard); `internal/stats/name.go` (the NEW `kafka.` INLINE arm, AMEND-K2).
- The ADR-0223 per-connection mutex pattern (zookeeper 28.2 / mongo 29.2) — the `correlation_id → (api_key, api_version)` map.
- `internal/filter/network/mongoproxy/` (29) — the consumer-#2 package shape kafkabroker mirrors (two-step factory, decoder-internal private-buffer reassembly, the eager-roster `newMongoStats` shape, the ADR-0223 correlation mutex). The `mongoproxy/codec.go:467/493/507` `IsValidName`-at-codec-boundary pattern (here satisfied by construction — AMEND-K7).
- `internal/filter/tcpproxy/` (02/26.2/27) — the terminal in every fixture chain; untouched by 31.
- The differential harness + `fixture.StatsAsserter` (+ the fixture-dispatch + asserter-dispatch + `-count=1` break-protocol memory constraints) — now booting the contrib reference image (ADR-0227).
- `envoy.extensions.filters.network.kafka_broker.v3` proto bindings (go-control-plane `/contrib` v1.32.4 — the NEW dep, §3.4).
- The freeze-after-boot registry discipline (ADR-0072/0079), the two-step factory (ADR-0079), the iteration-status protocol (ADR-0038/0213), atomic landing + six-gate (ADR-0052), byte-stable PARSE-REJECT wording (ADR-0080).
- **NOT consumed:** the ADR-0219 upstream-cluster-override seam (kafka never overrides routing); the ADR-0226 async halt/resume seam (kafka never halts); the ADR-0217 dynamic-metadata Bucket (kafka emits no metadata); gauges (kafka has no gauge — contrast mongo's `op_query_active`); histograms (the 86 response-duration histograms deferred, ADR-0060).

---

## 4. Framework primitives — 0 framework-seam extensions + 1 NEW filter package + 1 new go.mod dep

Phase 31 adds NO framework delta (contrast mongo's 29.3 async-halt surgery). Its "newness" is entirely in the `kafkabroker` package + the `/contrib` dep + the cross-side differential against the contrib reference Envoy. The framework GROWTH story pauses: phase 31 demonstrates the as-built §9 framework (ReadFilter + WriteFilter chains, the post-handoff read seam, the per-connection mutex pattern, the eager-roster + tag-extractor machinery) is sufficient for a third both-direction protocol sniffer with ZERO seam churn — the defer-with-allowance / consume-at-consumer discipline at rest. The deferred broker-address-rewrite WOULD be a new framework capability (write-buffer mutation — every prior sniffer only OBSERVED), but it is deferred (§2 / §8 of the BRAINSTORM).

---

## 5. Proto-field roster (per §11.1 D31-1 + §11.7 D31-7)

All rosters transcribed from go-control-plane `/contrib` v1.32.4 (`contrib/envoy/extensions/filters/network/kafka_broker/v3/kafka_broker.pb.go` + `.pb.validate.go`); verified by `proto.MessageName` run in-session.

### 5.1 TypeURL

`proto.MessageName(&kafka_brokerv3.KafkaBroker{})` = `envoy.extensions.filters.network.kafka_broker.v3.KafkaBroker` → **`@type` = `type.googleapis.com/envoy.extensions.filters.network.kafka_broker.v3.KafkaBroker`** (the `extensions.` segment per `reference_network_filter_typeurl_extensions`; pinned by an IMPL Task-1 `proto.MessageName` test, NEVER the docs string). NOTE: the filter *extension/registration* name (the listener filter-chain `name`) is `envoy.filters.network.kafka_broker` — distinct from the config `@type` above.

### 5.2 `envoy.extensions.filters.network.kafka_broker.v3.KafkaBroker` (5 fields; `[#next-free-field: 6]`)

| Go field | proto field | tag | Go type | default / PGV | 31 disposition |
|---|---|---|---|---|---|
| `StatPrefix` | `stat_prefix` | 1 | `string` | **PGV-required min 1 rune** | REQUIRED → boot-reject (the `0054` fixture arm) |
| `ForceResponseRewrite` | `force_response_rewrite` | 2 | `bool` | false; no PGV | PARSE-ACCEPT, behavior-DEFERRED |
| `IdBasedBrokerAddressRewriteSpec` | `id_based_broker_address_rewrite_spec` | 3 | `*IdBasedBrokerRewriteSpec` (sole member of the `broker_address_rewrite_spec` **oneof**) | nil; oneof typed-nil reject + nested recurse | PARSE-ACCEPT, behavior-DEFERRED (the write-mutation feature) |
| `ApiKeysAllowed` | `api_keys_allowed` | 4 | `[]uint32` (repeated, packed) | empty; **each ∈ [0, 32767]** | PARSE-ACCEPT, behavior-DEFERRED |
| `ApiKeysDenied` | `api_keys_denied` | 5 | `[]uint32` (repeated, packed) | empty; **each ∈ [0, 32767]** | PARSE-ACCEPT, behavior-DEFERRED |

Nested messages (PARSE-ACCEPT, behavior-deferred): `IdBasedBrokerRewriteSpec{ Rules []*IdBasedBrokerRewriteRule }`; `IdBasedBrokerRewriteRule{ Id uint32; Host string (PGV-required min 1 rune); Port uint32 (PGV ≤ 65535) }`. No nested enums; no `*PerRoute` message; `NumMessages: 3`.

`id_based_broker_address_rewrite_spec` is the SOLE member of a oneof named `broker_address_rewrite_spec` (Go wrapper `KafkaBroker_IdBasedBrokerAddressRewriteSpec`) — not a plain optional field. This affects unmarshal + adds an `"oneof value cannot be a typed-nil"` PGV arm.

---

## 6. PARSE-REJECT roster (per §11.7 + ADR-0080)

### 6.1 Wording discipline

Per ADR-0080 byte-stable PARSE-REJECT discipline: each arm is a named constant with byte-stable wording verified by a table test at IMPL. Boot-reject PARITY arms (mirroring an upstream PGV failure) are distinguished from envoy-go-strict DEPARTURE arms. Phase 31 has NO departure-class rejects — every reject below mirrors an upstream PGV reject; the departures are all behavioral (active-features deferred; histograms deferred; runtime keys at defaults), recorded in BEHAVIOR_CONTRACT, never as rejects.

### 6.2 Boot-reject arms

- `kafka-broker-stat-prefix-required` — boot-reject PARITY (the `stat_prefix` PGV min-1-rune rule, §5.2). The load-bearing `0054` fixture arm. **Wording note (D31-7):** the reference C++ rejects with `Proto constraint validation failed (KafkaBrokerValidationError.StatPrefix: value length must be at least 1 characters)`; the Go PGV binding emits `value length must be at least 1 runes` (the C++/Go "characters" vs "runes" idiom difference). envoy-go's reject wording is its OWN ADR-0080 byte-stable constant; the boot-reject differential checks that BOTH sides REJECT at boot (a boot-stderr substring the harness can match on each side — the mongo `0050` / zookeeper boot-reject precedent), not exact-string-equality across implementations.
- `kafka-broker-api-key-out-of-range` — boot-reject PARITY (each `api_keys_allowed[i]` / `api_keys_denied[i]` must be ∈ [0, 32767] — int16 max; a value ≥ 32768 rejects). UNIT-TESTED (these arms exercise a behavior-deferred field; whether they gain a `0054` fixture arm or stay unit-test-only is D-P3 — anticipated unit-test-only, the mongo delay-arm precedent).
- `kafka-broker-rewrite-rule-host-required` / `kafka-broker-rewrite-rule-port-too-large` — boot-reject PARITY for the nested `IdBasedBrokerRewriteRule` (`host` min-1-rune; `port` ≤ 65535) + the `broker_address_rewrite_spec` oneof typed-nil arm. UNIT-TESTED (deferred-feature arms; the rule is parse-validated even though its behavior is deferred — config-parse faithfulness, the mongo FaultDelay-PGV precedent).
- Framework-level: unknown network-filter `typed_config` type_url → existing boot-reject (no new arm).
- `force_response_rewrite`: NOT a reject (bool, unconstrained — upstream parity).

---

## 7. Stat surface (per §11.2 D31-2 + AMEND-K1/K2/K3 + §11.4 D31-4)

### 7.1 Scope/naming — `kafka.<stat_prefix>.{request,response}.<msg>_request|_response` (AMEND-K1)

Upstream: `fmt::format("kafka.{}.request.", stat_prefix)` + `POOL_COUNTER_PREFIX` (`request_metrics_h.j2`); symmetric `kafka.{}.response.`. The per-key segment is `name_in_c_case()` of the FULL Kafka message name (incl. the `Request`/`Response` suffix). Emitted internal names:

- `kafka.<sp>.request.<msg>_request` — per api_key, e.g. `kafka.kprobe.request.produce_request`, `kafka.kprobe.request.api_versions_request`.
- `kafka.<sp>.response.<msg>_response` — per api_key, recovered via the correlation map.
- the 4 fixed: `kafka.<sp>.request.unknown`, `kafka.<sp>.request.failure`, `kafka.<sp>.response.unknown`, `kafka.<sp>.response.failure`.

envoy-go mirrors this internal naming exactly (the differential `StatsAsserter` + the Prometheus arm depend on it). The full 86-entry `api_key → message-name` table is Appendix B.

### 7.2 The EAGER 176-counter fixed roster (AMEND-K3)

| Family | Count | Created | Incremented | Notes |
|---|---|---|---|---|
| Request per-key: `request.<msg>_request` | 86 | eager (config parse) | `OnData` request decode | api_keys 0–87 minus 71/72 (telemetry-excluded) |
| Response per-key: `response.<msg>_response` | 86 | eager | `OnWrite` response decode (correlation-recovered) | identical api-key set to request |
| Fixed: `request.unknown` / `request.failure` / `response.unknown` / `response.failure` | 4 | eager | per §7.3 | |
| **Total counters** | **176** | | | |
| Response-duration histograms `response.<msg>_response_duration` | 86 | — | — | **DEFERRED (ADR-0060)** — coverage boundary, NOT created/mirrored |

**Creation timing (D-P1 analog):** upstream creates the full roster per-`Rich*MetricsImpl` (= per connection, pool-deduped), so the reference admin shows ZERO `kafka.*` until the first downstream connection (live-probed — §11.2). envoy-go's posture (resolved THIS SPEC): **EAGER creation at config parse** (freeze-after-boot-friendly; the mongo D-P1 / zookeeper-roster precedent — `NewCounterIfAbsent` idempotent across listeners sharing a `stat_prefix`), giving `kafka.<sp>.*` present-at-0 from boot. The boot-window difference (envoy-go exposes the roster at 0 from boot; reference exposes nothing until first connection) is a BEHAVIOR_CONTRACT departure that is UNOBSERVABLE to the differential because every fixture stat assertion runs post-first-connection (§8). This is the bounded-eager-roster choice (the mongo FIXED roster precedent) — the per-key counters are a FIXED roster from the static 86-key table, NOT a lazy unbounded dynamic family (contrast mongo's `cmd.*`/zookeeper's `auth.<scheme>_rq`).

### 7.3 `unknown` / `failure` semantics (AMEND-K4)

- `request.unknown` — a well-decoded request header whose `(api_key, api_version)` has no recognized parser (an unknown api_key OR an unknown VERSION of a known key). HEADER-reproducible (the `0053` unknown-key + unknown-version arms). [The upstream "leftover-body-bytes → unknown" sub-case needs body decode → coverage boundary.]
- `request.failure` — a decode EXCEPTION on the request bytes (e.g. a `client_id` NULLABLE_STRING with an invalid/out-of-range length). HEADER-reproducible (the `0053` malformed-header arm).
- `response.unknown` — the response-side Sentinel path (unrecognized recovered `(api_key, api_version)`).
- `response.failure` — a decode EXCEPTION on the response bytes, INCLUDING the **unregistered-`correlation_id` lookup-miss** (the recovered correlation_id was never registered by a request → `getResponseSpec` throws). HEADER-reproducible (the `0053` unregistered-correlation arm) — so `response.failure` is NOT purely a unit-test boundary.

### 7.4 Prometheus exposition — the `kafka.` INLINE arm (AMEND-K2)

Reference Envoy `contrib-v1.37.2` `/stats/prometheus` (probed live): kafka stats emit as **`envoy_kafka_<sp>_<direction>_<msg>_<request|response>{} <v>`** — the metric name is fully inlined (prefix + direction + api-key) and the label set is EMPTY. Real quoted lines:

```
# TYPE envoy_kafka_kprobe_request_api_versions_request counter
envoy_kafka_kprobe_request_api_versions_request{} 1
# TYPE envoy_kafka_kprobe_request_unknown counter
envoy_kafka_kprobe_request_unknown{} 1
```

envoy-go's `internal/stats/name.go` default branch errors on unrecognized prefixes, so a new arm is required — the **INLINE shape (no labels)**, the zookeeper `.zookeeper.` precedent generalized to a `kafka.` ROOT segment, NOT the mongo/rbac tag-extractor (label-hoist) shape the BRAINSTORM hypothesized:

```go
// kafka_broker (31; AMEND-K2): kafka.<sp>.<rest> → envoy_kafka_<sp>_<rest flattened>
// with EMPTY labels. Live-probed: the api-key + direction + prefix are ALL inlined
// into the metric name; NO tag extraction (contrast the mongo .rbac.-shape label hoist).
// KEEP-IN-SYNC: internal/filter/network/kafkabroker/stats.go.
if rest, ok := strings.CutPrefix(internal, "kafka."); ok {
    base = "envoy_kafka_" + strings.ReplaceAll(rest, ".", "_")
    return base, labels, nil // labels is empty
}
```

(Insert before the final `return "", nil, fmt.Errorf(...)` default in the unmatched-prefix block, `name.go:290`.) The arm needs NO shape-validation guard (no dynamic VALUE segment is hoisted to a label; the whole name flattens). The exact form is pinned by the §11.2 live probe.

### 7.5 Project stat-count delta

360 → **536** (+176; all 176 counters eager-created at config parse — §7.2). The 86 response-duration histograms are NOT counted (deferred, ADR-0060). No gauges. This is the project's largest single-phase stat jump (a bounded fixed roster).

### 7.6 envoy-go-strict departure flags (BEHAVIOR_CONTRACT at IMPL)

- The 86 `response.<msg>_response_duration` HISTOGRAMS unmirrored (ADR-0060).
- The four active features' behavior (parse-accepted, behavior-deferred); the broker-address-rewrite write-mutation flagged as a future surgery sub-phase.
- The "leftover-body-bytes → unknown" sub-case unmirrored (needs body decode; header-only).
- The boot-window eager-vs-per-connection creation difference (D-P1 analog; unobservable to the differential).
- Runtime-key gating unmirrored (no runtime layer → key defaults).
- api_keys 71/72 (telemetry) excluded — UPSTREAM PARITY (not a departure; upstream excludes them too).

---

## 8. Differential fixture taxonomy (+2)

Full cross-side against the contrib reference Envoy `envoyproxy/envoy:contrib-v1.37.2`. Per `reference_differential_fixture_dispatch_constraint`: cross-side and boot-reject fixtures are SEPARATE directories. Per `reference_differential_asserter_dispatch`: every subject-side stat assertion uses `fixture.StatsAsserter` and MUST be proven live via a deliberate-break with `-count=1` (`reference_differential_break_protocol_count1`). The body differential is intrinsically vacuous (bytes pass through unchanged on both sides) — the stat comparison IS the proof. Numbering continues from `0052` (master-tip tail `0052-mongo-fault-delay`); re-pinned at IMPL Task 1.

**Fixture-design caveats from the §11.2 live probe:** (i) the fixture backend MUST be a real listening TCP responder (if tcp_proxy cannot establish its upstream, the reference closes the downstream before the kafka decoder runs → zero decode, zero stats — verify decode ran via `tcp.<sp>.downstream_cx_rx_bytes_total > 0` and/or non-zero kafka counters); (ii) all stat assertions are post-first-connection (the eager-roster boot-window difference, §7.2); (iii) for the response-side arms the backend MUST echo the request's `correlation_id` in a well-formed response frame (the NEW BackendKind, §8.3); (iv) drive ONE request per connection (or carefully sequence frames) — the live probe found a hand-rolled multi-frame stream tripped after frame 1; the fixture driver pins the exact per-connection framing.

### 8.1 `0053-kafka-requests` (cross-side; multi-arm)

Chain `[kafka_broker (stat_prefix: …), tcp_proxy]` on BOTH sides; the fixture backend is the NEW correlation-id-echoing TCP Kafka responder (§8.3). The driver sends hand-crafted Kafka wire frames (4-byte INT32 length prefix + the §11.3 header layouts). Arms (the PLAN finalizes the exact roster):

1. **request per-key arms** — a spread of api_keys at a spread of header versions: a v0/v1 (non-flexible) header (e.g. Metadata key 3 at a non-flexible version), a flexible/tagged-field v2 header (e.g. ApiVersions key 18 at a flexible version, or Produce key 0 at a flexible version) → the matching `request.<msg>_request` +1 both sides + (for the flexible arm) proves tagged-field framing decode.
2. **request unknown-key arm** — a header with an unrecognized api_key (e.g. 9999) → `request.unknown` +1 both sides (live-confirmed: an api_key=9999 frame drove `request.unknown` to 1).
3. **request unknown-version arm** — a known api_key at an unrecognized api_version → `request.unknown` +1 both sides (AMEND-K4 — keyed on (api_key, api_version)).
4. **request failure arm** — a malformed header (e.g. a `client_id` NULLABLE_STRING with an invalid length) → `request.failure` +1 both sides.
5. **response per-key arms** — the backend echoes a response frame whose `correlation_id` matches a prior request → `response.<msg>_response` +1 both sides (correlation-recovered; the request side registered `correlation_id → (api_key, api_version)`).
6. **response failure (unregistered correlation) arm** — a response frame whose `correlation_id` was never registered by a request → `response.failure` +1 both sides (AMEND-K4).
7. **deliberate-break liveness proof** — recorded in driver comments + README per the `0030` lesson, run with `-count=1` (the cross-side `StatsAsserter` is the load-bearing proof — prove each asserted counter LIVE).

(Per the cross-side-XOR-boot-reject fixture-dispatch constraint, all cross-side arms share this ONE dir.)

### 8.2 `0054-kafka-boot-reject` (boot-reject; separate dir)

Missing `stat_prefix` → both sides reject at boot (the §6.2 `kafka-broker-stat-prefix-required` arm; boot-stderr-substring parity per §6.2). The api_keys-range arms (§6.2) are unit-tested; whether they gain a fixture arm here is D-P3 (anticipated unit-test-only — the mongo `0050`/zookeeper boot-reject precedent carries only the load-bearing required-field arm).

### 8.3 New BackendKind (anticipated value 31)

A NEW BackendKind — a synthesized **correlation-id-echoing TCP Kafka responder**: it reads each request frame's `correlation_id` (INT32 at offset 8 after the 4-byte length + INT16 api_key + INT16 api_version) and echoes it in a canned response frame (4-byte length + INT32 correlation_id + optional tagged-fields per the response-header-version rule + a minimal body sufficient for the reference to count the response) so the subject's response-side correlation has something to correlate AGAINST. Contrast the existing silent `TCPSink` (28) / the `TCPMongoResponder` (30). The exact response-frame shape (incl. whether the ApiVersions(18) response header suppresses tagged fields — AMEND-K5) is pinned at IMPL. BackendKind tail 30 → 31.

### 8.4 Total fixture-dir count + conformance

54 → **56** (+2: `0053` cross-side, `0054` boot-reject). No new conformance harness (matches 26/27/28/29). The h2spec 53/53 + proxy-wasm 10/10 gates re-run asserted-unaffected at the six-gate (image-independent; phase 31 touches no HTTP/h2/proxy-wasm path).

---

## 9. Behavior-contract delta (the 31 bundle; ADR-0052 atomic landing)

At IMPL final task, `docs/envoy-go/BEHAVIOR_CONTRACT.md` gains:

- A NEW `### envoy.filters.network.kafka_broker` subsection — proto `…kafka_broker.v3.KafkaBroker`; the header-only decode envelope (request header `api_key`/`api_version`/`correlation_id`/`client_id`/tagged-fields; response header `correlation_id`/tagged-fields; the 4-byte length prefix); the `(api_key, api_version) → tagged-fields` rule (the `flexibleVersions` predicate + the ApiVersions(18) response special-case); the 176-counter eager roster under `kafka.<stat_prefix>.` (the `_request`/`_response`-suffixed naming, AMEND-K1); the `unknown`/`failure` semantics (AMEND-K4); the `correlation_id → (api_key, api_version)` per-connection correlation; the 9th built-in; the `kafka.` INLINE Prometheus arm (AMEND-K2).
- The stat table 360 → 536 (+176).
- Coverage-boundary / departure records: the 86 response-duration histograms unmirrored (ADR-0060); the four active features parse-accepted-behavior-deferred (the broker-address-rewrite write-mutation as a future surgery sub-phase); the "leftover-body-bytes → unknown" sub-case unmirrored; the eager-vs-per-connection boot-window difference; runtime-keys-at-defaults; api_keys 71/72 excluded (upstream parity).

---

## 10. Per-task structure (~13–18 tasks; PLAN decomposes)

Indicative spine for the PLAN (TDD per task; per-task `gofmt -l` + `golangci-lint` on touched pkgs per `feedback_pertask_gofmt_lint`; subagents commit LOCAL-ONLY per `feedback_subagents_no_push`):

| # | Task | SPEC anchor |
|---|---|---|
| 1 | First-task baselines/anchors gate: re-confirm fuzzers **39** + fixtures **54** (tail `0052`) + stat surface **360** + DECISIONS.md tail **ADR-0227** (this SPEC drafts 0228) + BackendKind tail **30** via the canonical recipes; re-confirm `proto.MessageName(&kafka_brokerv3.KafkaBroker{})` + that `/contrib v1.32.4` resolves; re-pin the as-built anchors (`builtins.go` registration site, `bootstrap.go` blank-import block, `name.go:290` default branch, `mongoproxy/stats.go` eager-roster shape) against the IMPL-session tip | §11 / §3 |
| 2 | Add the `/contrib v1.32.4` dep + the `kafka_broker/v3` blank-import in `bootstrap.go` + `go mod tidy` keeps it (TDD: a `proto.MessageName` pinning test); the `kafkabroker` package skeleton + `TypeURL` | §3.3/§3.4/§5.1 |
| 3 | Config parse: the 5-field `KafkaBroker` + PGV arms (`stat_prefix` required → boot-reject; api_keys range; the oneof + nested rule arms) + `stats.IsValidName(stat_prefix)` config-boundary guard (TDD: all reject arms byte-stable) | §5/§6 |
| 4 | The static `api_key → message-name` table (86 entries) + a `TestApiKeyRoster_MatchesUpstream` byte-stable test (Appendix B) | §7.1 / Appendix B |
| 5 | The `flexibleVersions(api_key)` predicate (`requestUsesTaggedFieldsInHeader`/`responseUsesTaggedFieldsInHeader` + the ApiVersions(18) response special-case) + a table test (Appendix C) | §11.3 / Appendix C |
| 6 | The Kafka primitive decoder (INT16/INT32/NULLABLE_STRING/UNSIGNED_VARINT tagged-fields) + the 4-byte length-prefix framing + the request-header decoder (TDD per primitive + partial-frame reassembly + malformed-length throw) | §11.3 |
| 7 | The response-header decoder (`correlation_id` + optional tagged-fields) in `OnWrite` + the per-connection `correlation_id → (api_key, api_version)` map + the ADR-0223 mutex (TDD: register-on-request, recover-on-response, erase-on-hit, unregistered → failure; a `-race` concurrent test) | §3.1/§11.3/§11.4 |
| 8 | The EAGER 176-counter roster + the eager-create helper + the inc accessors + a `TestStatRoster` (TDD: roster present-at-0; the 4 fixed; the 86×2 per-key) | §7.2 |
| 9 | The ReadFilter (`OnData` request decode → `request.<msg>_request` / `request.unknown` / `request.failure`) + WriteFilter (`OnWrite` response decode → `response.<msg>_response` / `response.unknown` / `response.failure`) glue; pure-copying-sniffer (always Continue; never mutate/close) | §3.1/§7.3 |
| 10 | Registration as the 9th built-in (`builtins.RegisterBuiltins` + main.go parity) + boot smoke (TDD) | §3.3 |
| 11 | The `kafka.` INLINE Prometheus arm in `internal/stats/name.go` (TDD: `kafka.kprobe.request.api_versions_request` → `envoy_kafka_kprobe_request_api_versions_request{}`; no labels) | §7.4 |
| 12 | The new BackendKind (correlation-id-echoing TCP Kafka responder) | §8.3 |
| 13 | The `0053-kafka-requests` cross-side fixture (request/unknown/version/failure/response/unregistered-correlation arms) + driver | §8.1 |
| 14 | The `0054-kafka-boot-reject` fixture (missing `stat_prefix`) | §8.2 |
| 15 | The 40th fuzzer `FuzzKafkaDecode` (both directions: no-panic + no-chain-buffer-mutation) | §14 |
| 16 | Full differential re-verify (the 54 prior dirs byte-exact back-compat + the 2 new dirs green) + the deliberate-break liveness proofs (`-count=1`) | §8 |
| 17 | Completion bundle: BEHAVIOR_CONTRACT 31 subsection (360 → 536) + ADR-0228 §Decision/§Consequences body (ADR-0044 in-place) + STATE/ROADMAP row 31 `in-progress → done` + the six-gate evidence | §9 / §15 |

The PLAN re-checks the ADR-0045 gate; if it trips, consume the pre-authorized 31.1/31.2 split (§3.0).

---

## 11. SPEC-time empirical-pin block (D31-1..D31-8 — executed IN-SESSION 2026-06-07)

Parallel-subagent-fan-out scrape executed this SPEC session per ADR-0004's hard-gate. **Probe date: 2026-06-07.** **Reference source corpus:**

1. **The live `envoyproxy/envoy:contrib-v1.37.2` docker image** (id `7edd5b0f…`, present locally): a real boot of a `[kafka_broker (stat_prefix: kprobe), tcp_proxy]` listener on a docker BRIDGE network (`reference_docker_probe_bridge_network`) with a STRICT_DNS backend (`kafkabackend:9092`); admin `/stats` + `/stats/prometheus` scrapes pre- and post-connection; a hand-crafted ApiVersions(18) request frame + an unknown-api_key(9999) frame driven through the listener (decode CONFIRMED ran: `tcp.kprobe_tcp.downstream_cx_rx_bytes_total: 76`, `kafka.kprobe.request.api_versions_request: 1`, `kafka.kprobe.request.unknown: 1`); `--mode validate` boot-reject probe.
2. **go-control-plane `/contrib` v1.32.4 bindings** at `~/go/pkg/mod/github.com/envoyproxy/go-control-plane/contrib@v1.32.4/envoy/extensions/filters/network/kafka_broker/v3/`: `kafka_broker.pb.go` + `.pb.validate.go` + `_vtproto.pb.go`; `proto.MessageName` run in a throwaway `/tmp` module + `go mod tidy` survival check.
3. **Upstream Envoy v1.37.2 contrib source** via raw.githubusercontent.com at tag v1.37.2: `contrib/kafka/filters/network/source/` — `broker/filter.{h,cc}`, `kafka_request{,_parser}.h`, `kafka_response{,_parser}.{h,cc}`, `response_codec.{h,cc}`, `codec.h`, `protocol/{request,response}_metrics_h.j2`, `protocol/{request,response}_resolver_cc.j2`, `protocol/generator.py`, `request_handler.cc`, `rewriter.cc`, `filter_config.cc`; `bazel/repository_locations.bzl` (Kafka source 3.9.1).
4. **envoy-go codebase** at master tip `2f387b6`: `internal/filter/network/{builtins,mongoproxy,zookeeperproxy}`; `internal/stats/name.go`; `internal/bootstrap/bootstrap.go`; `go.mod`.

### Summary disposition table (8 pins)

| Pin | Topic | Disposition | AMEND |
|---|---|---|---|
| §11.1 | D31-1 (SPEC-BLOCKING) — TypeURL + `/contrib` dep | **CONFIRMED** (@type carries `extensions.`; contrib resolves + survives tidy) + REFINES (kafka_broker/v3 self-contained; only contrib is a new direct dep) | K6 |
| §11.2 | D31-2 (SPEC-BLOCKING) — stat roster + creation + prom form | **CONFIRMED** (filter ships + decodes live) + REFINES (`_request`/`_response`-suffixed names; eager 176-counter roster; api_keys 71/72 excluded) + REFUTES (NO label hoist — fully inlined) | K1, K2, K3 |
| §11.3 | D31-3 — wire framing + header-version | RESOLVES (request/response header layouts; the `flexibleVersions` predicate + the ApiVersions(18) response special-case; Kafka 3.9.1) | K5 |
| §11.4 | D31-4 — unknown/failure semantics | REFINES (`unknown` keyed on (api_key, api_version); `failure` partly header-reproducible incl. response unregistered-correlation) | K4 |
| §11.5 | D31-5 — `IsValidName` placement | RESOLVES (table-bounded names → guard satisfied by construction; config-boundary guard for `stat_prefix` only) | K7 |
| §11.6 | D31-6 — close-direction counters | RESOLVES: **NONE** (live + source — no `cx_destroy_*` analog; framework-zero-touch) | — |
| §11.7 | D31-7 — config PGV arms + boot-reject | RESOLVES (5 fields; `stat_prefix` required; api_keys [0,32767]; the oneof + nested-rule arms; `0054` include) | — |
| §11.8 | D31-8 — ADR-0045 single-row gate | RESOLVES (~805–1200 production LoC / ~13–18 tasks → single row; the 31.1/31.2 split stays unconsumed) | — |

### 11.1 D31-1 (SPEC-BLOCKING) — TypeURL + the `/contrib` dep: CONFIRMED

`proto.MessageName(&kafka_brokerv3.KafkaBroker{})` (run in a throwaway `/tmp/d31probe` module) = `envoy.extensions.filters.network.kafka_broker.v3.KafkaBroker` → `@type` = `type.googleapis.com/envoy.extensions.filters.network.kafka_broker.v3.KafkaBroker` (carries the `extensions.` segment — `reference_network_filter_typeurl_extensions` holds). The filter registration name is `envoy.filters.network.kafka_broker`. Generated files: `kafka_broker.pb.go` (package `kafka_brokerv3`), `kafka_broker.pb.validate.go`, `kafka_broker_vtproto.pb.go`.

`go get github.com/envoyproxy/go-control-plane/contrib@v1.32.4` resolved cleanly; after a consumer imports the kafka_broker/v3 package, `go mod tidy` KEEPS `github.com/envoyproxy/go-control-plane/contrib v1.32.4` as a direct require. **REFINEMENT (AMEND-K6):** the kafka_broker/v3 package is SELF-CONTAINED — it imports only `cncf/xds/go`, `protoc-gen-validate/validate`, `vtprotobuf/protohelpers`, and `google.golang.org/protobuf` (zero go-control-plane/envoy imports). All four are ALREADY in the envoy-go go.mod closure (verified: `cncf/xds`, `protoc-gen-validate`, `vtprotobuf`, `/envoy` all present). So the SINGLE genuinely-new direct dep is `contrib v1.32.4`. (Do not claim kafka_broker/v3 holds `/envoy` in place — it does not; `/envoy` stays because other phases consume it.) No nested enums; no `*PerRoute` message; `NumMessages: 3`.

### 11.2 D31-2 (SPEC-BLOCKING) — stat roster + creation timing + Prometheus form

**Live-decode achieved** (not just a boot roster): after an ApiVersions(18) request frame, `tcp.kprobe_tcp.downstream_cx_rx_bytes_total: 76` and `kafka.kprobe.request.api_versions_request: 1`; an api_key=9999 frame drove `kafka.kprobe.request.unknown: 1`.

- **Naming (AMEND-K1):** per-key counters are `kafka.<sp>.request.<msg_name_snake>_request` / `kafka.<sp>.response.<msg_name_snake>_response` (the segment is `name_in_c_case()` of the full message name, incl. the `Request`/`Response` suffix — source `protocol/generator.py:780` `name_in_c_case` over `spec['name']`). NOT the bare api-key name.
- **Roster (AMEND-K3):** 86 request + 86 response per-key counters + 4 fixed (`request.unknown`/`request.failure`/`response.unknown`/`response.failure`) + 86 response-duration HISTOGRAMS. api_keys 71/72 (telemetry) EXCLUDED (`generator.py:167` `if api_key not in [71, 72]`). Created EAGERLY (`POOL_COUNTER_PREFIX` over the full `KAFKA_*_METRICS` macro in the `Rich*MetricsImpl` constructor) — per filter-instance = per connection, pool-deduped (the boot `/stats | grep '^kafka'` was EMPTY pre-connection; the full roster materialized post-connection). envoy-go posture: eager-at-config-parse (§7.2).
- **Prometheus (AMEND-K2 — REFUTES label-hoist):** fully inlined, EMPTY labels:
  ```
  # TYPE envoy_kafka_kprobe_request_api_versions_request counter
  envoy_kafka_kprobe_request_api_versions_request{} 1
  # TYPE envoy_kafka_kprobe_request_unknown counter
  envoy_kafka_kprobe_request_unknown{} 1
  ```
  (The histograms emit standard Envoy duration buckets, also empty labels: `envoy_kafka_kprobe_response_<msg>_response_duration_bucket{le="…"}` — deferred per ADR-0060.)

**PROBE-HARNESS CAVEAT (recorded honestly):** the RESPONSE-side decode was NOT verified live — the dumb `nc` backend never echoed a valid Kafka response frame, so all `response.*` counters stayed 0. The response roster NAMES + form are pinned from the (lazily-materialized) live roster + the source (`response_metrics_h.j2`, symmetric with request). The response COUNTING behavior + the correlation_id matching are an IMPL re-probe obligation (stand up the §8.3 correlation-echoing backend). Also only ONE request api-key incremented live (a hand-rolled multi-frame stream tripped the decoder's per-connection state after frame 1); roster/naming pins are unaffected (all 86 names present regardless), but the IMPL should drive one clean connection per request to confirm a second distinct api-key counts (the `0053` driver does this).

### 11.3 D31-3 — wire framing + header layouts + the header-version rule

**Length prefix:** every request/response is preceded by a 4-byte INT32 length (`RequestStartParser`/`ResponseHeaderParser` read it into `remaining_*_size_`; partial frames wait for more — never an error).

**Request header** (`kafka_request.h:31-39`, in order): `api_key` INT16 → `api_version` INT16 → `correlation_id` INT32 → `client_id` NULLABLE_STRING (header v1+) → tagged_fields (header v2/flexible only, gated by `requestUsesTaggedFieldsInHeader(api_key, api_version)`).

**Response header** (`kafka_response.h:27-66`): `correlation_id` INT32 → tagged_fields (flexible only, gated by `responseUsesTaggedFieldsInHeader(api_key, api_version)`). The response header carries NO api_key/api_version.

**Correlation (the upstream precedent for envoy-go's map):** `ResponseDecoder` owns `expected_responses_ : std::map<int32_t, std::pair<int16_t,int16_t>>` (correlation_id → (api_key, api_version)). The `Forwarder` (request side) calls `expectResponse(correlation_id, api_key, api_version)` on BOTH `onMessage` (success) and `onFailedParse` (`broker/filter.cc:8-16`). `getResponseSpec(correlation_id)` (`kafka_response_parser.cc:60-72`) looks up + ERASES the entry; ABSENT → `throw EnvoyException("no response metadata registered for correlation_id …")` → `response.failure` (AMEND-K4). envoy-go mirrors: a per-connection map storing (api_key, api_version), registered on every request (success + decode-failure), recovered + erased on response, lookup-miss → `response.failure`, under the ADR-0223 mutex (request decode on the read goroutine; response decode on the write goroutine).

**Header-version rule (AMEND-K5):** no numeric enum; a header uses the v2/flexible (tagged-fields) form iff `api_version ∈ flexibleVersions(api_key)` (`requestUsesTaggedFieldsInHeader` generated from each message's JSON `flexibleVersions`). SPECIAL CASE: `responseUsesTaggedFieldsInHeader` SAME structure EXCEPT api_key 18 (ApiVersions) ALWAYS returns false (`kafka_response_resolver_cc.j2` `!= 18` guard; `kafka_response.h:18-21` — ApiVersions responses carry no tagged fields in the header despite being flexible). The full `flexibleVersions(api_key)` table (Kafka 3.9.1) is Appendix C. Kafka source 3.9.1 (`bazel/repository_locations.bzl`); api-key ceiling 87.

### 11.4 D31-4 — unknown / failure semantics

Two codepaths (`codec.h:103-138` `doParse`): a finished parser with `result.message_` → `onMessage`; else (`result.failure_data_`) → `onFailedParse`.

- **`unknown`** (`onUnknownRequest`/`onUnknownResponse`): the codec reached a Sentinel `parseFailure` result. The resolver `createParser(api_key, api_version)` returns a `SentinelParser` when (a) **(api_key, api_version) has no matching generated parser** (unknown key OR unknown version of a known key), or (b) a known parser left residual body bytes (length mismatch). Keyed on (api_key, api_version) — NOT just api_key. Header-reproducible for case (a); case (b) needs body decode (coverage boundary).
- **`failure`** (`onRequestException`/`onResponseException`): an `EnvoyException` thrown out of `onData`/`onWrite` (`broker/filter.cc:78-96`) → inc `failure` + `decoder->reset()` + StopIteration. Causes: a malformed request header (e.g. `client_id` NULLABLE_STRING with an invalid length — header-reproducible, no body needed); a body deserializer throw; or (response) the unregistered-`correlation_id` `getResponseSpec` throw (header-reproducible). So the distinction is "threw (`failure`) vs reached a clean Sentinel (`unknown`)", NOT "header vs body".

### 11.5 D31-5 — IsValidName guard placement

The per-key stat segment is looked up from the static 86-entry api-key table (never a raw wire string — the wire carries an int16 api_key; the NAME is table-derived), so NO arbitrary wire-derived dynamic segment reaches `NewCounterIfAbsent` → the charset guard (`reference_dynamic_stat_name_charset_guard`) is satisfied BY CONSTRUCTION (contrast mongo's arbitrary BSON collection/command names, which REQUIRED the runtime guard at `mongoproxy/codec.go:467/493/507`). The only config-derived segment is `stat_prefix` — guard it once at the config boundary with `stats.IsValidName` (the rbac `rbac.go:106`/mongo precedent). The `FuzzKafkaDecode` no-panic fuzzer asserts the decode path never panics + never mutates the chain buffer (defense-in-depth, even though no dynamic-name charset risk exists).

### 11.6 D31-6 — close-direction counters: NONE

Cross-confirmed live (`/stats | grep -i kafka` shows no `local`/`remote`/`destroy`/`cx_` decoder counter — the only `*_destroy_*` lines are generic `cluster.*.upstream_cx_destroy_*`) + source (the only stat-declaring code is `request_metrics_h.j2`/`response_metrics_h.j2`, exactly the per-key + unknown/failure counters + the response-duration histograms — no connection-lifecycle counters). kafka_broker stays framework-zero-touch w.r.t. close-direction (`reference_close_direction_framework_gap` does NOT bite here — no `cx_destroy_*` analog to defer).

### 11.7 D31-7 — config PGV arms + boot-reject

§5/§6 transcribe the full roster + arms. `stat_prefix` is PGV-required (min 1 rune) — the load-bearing boot-reject (live `--mode validate` confirmed both missing and empty `stat_prefix` reject identically: `Proto constraint validation failed (KafkaBrokerValidationError.StatPrefix: value length must be at least 1 characters)`; the Go binding emits `value length must be at least 1 runes`). `api_keys_allowed`/`api_keys_denied` each ∈ [0, 32767] (the `val < 0` arm is dead for uint32; the 32767 upper bound is live). The `broker_address_rewrite_spec` oneof emits a typed-nil arm; the nested `IdBasedBrokerRewriteRule` requires `host` (min 1 rune) and bounds `port` ≤ 65535. `force_response_rewrite` unconstrained. `0054-kafka-boot-reject` carries the `stat_prefix` arm; the others are unit-tested (D-P3).

### 11.8 D31-8 — ADR-0045 single-row gate

Net production ~805–1200 LoC (§3.0; the two static tables are DATA decoded with `encoding/binary` only — much leaner than mongo's BSON parser; the response header is trivial; no fault-delay/async-halt seam), ~13–18 tasks — both axes UNDER the gate (~25 tasks / ~1500 production LoC). **Single flat phase 31.** The pre-authorized 31.1/31.2 split stays UNCONSUMED. The PLAN re-checks.

---

## 12. SPEC-time D-questions for PLAN / IMPL resolution

- **D-P1 (eager-roster confirmation).** §7.2 resolves to eager-at-config-parse (mongo precedent). IMPL confirms the 176 counters present-at-0 + the boot-window departure is unobservable (all fixture assertions post-connection). **Resolution at:** IMPL Task 8.
- **D-P2 (Prometheus arm exactness).** The §7.4 INLINE arm is pinned from the live request-side probe; confirm the response-side + histogram lines flatten identically (the histograms are deferred but the `kafka.` arm must not choke on a `kafka.<sp>.response.<msg>_response_duration` name if one ever appears). **Resolution at:** IMPL Task 11 (anticipated: the inline arm handles all `kafka.*` uniformly).
- **D-P3 (boot-reject fixture arms).** Do the api_keys-range / nested-rule reject arms gain a `0054` fixture arm or stay unit-test-only? **Resolution at:** PLAN (anticipated: unit-test-only; `0054` carries the `stat_prefix` arm only — the mongo `0050`/zookeeper precedent).
- **D-P4 (response-side live re-probe).** §11.2 caveat: the response-side counting + correlation were not verified live. The `0053` correlation-echoing backend (§8.3) IS the live vehicle. **Resolution at:** IMPL Task 12/13 (the cross-side `StatsAsserter` proves response parity against the reference).
- **D-P5 (multi-request-per-connection framing).** §11.2 caveat: a hand-rolled multi-frame stream tripped after frame 1. The `0053` driver pins the exact per-connection framing (one request per connection, or correctly-sequenced length-prefixed frames). **Resolution at:** IMPL Task 13.
- **D-P6 (the api-key + flexibleVersions tables — hand-transcribed vs generated).** envoy-go has no code-gen pipeline; the tables are hand-transcribed static data (Appendices B/C) guarded by byte-stable tests. **Resolution at:** PLAN (anticipated: hand-transcribed; YAGNI — no Jinja2/Python gen).
- **D-P7 (fuzzer count recipe).** `FuzzKafkaDecode` is the 40th; re-pin the canonical recipe `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l` = 39 at the IMPL-session tip. **Resolution at:** IMPL Task 1/15.
- **D-P8 (BackendKind value).** The correlation-id-echoing responder is anticipated value 31; re-pin against the `fixture.go` enum tail (30 = `TCPMongoResponder`) at IMPL. **Resolution at:** IMPL Task 12.

---

## 13. RATIFIED-PENDING-IMPL items

- **R1 (passthrough invariant).** kafkabroker NEVER mutates/drains the chain buffer, never closes the connection, always returns `Continue` from `OnData`/`OnWrite` (a pure copying sniffer — the mongo private-buffer model). Ratified by ALL existing fixtures (0000..0052) staying byte-exact green + the `0053` passthrough-proven-byte-exact arms.
- **R2 (roster + scope parity).** The 176-counter roster under `kafka.<stat_prefix>.` matches upstream name-for-name (incl. the `_request`/`_response` suffixes + the 4 fixed + the 86-key set minus 71/72). Ratified by the `0053` post-connection roster assertion + a `TestStatRoster_MatchesUpstream` + `TestApiKeyRoster_MatchesUpstream` byte-stable test.
- **R3 (correlation hand-off).** The request-side `correlation_id → (api_key, api_version)` registration is consumed by the response side (recover + erase; unregistered → `response.failure`) under the ADR-0223 mutex. Ratified by the `0053` response + unregistered-correlation arms + a `-race` concurrent test.
- **R4 (StatsAsserter liveness).** Every `0053` stat assertion is proven live via a recorded deliberate-break run with `-count=1` (`reference_differential_asserter_dispatch` + `reference_differential_break_protocol_count1`).
- **R5 (header-version fidelity).** The `flexibleVersions` predicate + the ApiVersions(18) response special-case (AMEND-K5) reproduce upstream's tagged-fields-in-header decision byte-for-byte. Ratified by the `0053` flexible-header arm + the Appendix-C table test.
- **R6 (dep + tidy).** `/contrib v1.32.4` resolves + survives `go mod tidy` with the first consumer; the @type pinned via `proto.MessageName`. Ratified by the IMPL Task-2 pinning test + a clean `go mod tidy`.
- **R7 (Prometheus parity).** envoy-go's `/stats/prometheus` kafka lines match the reference's fully-inlined empty-label form `envoy_kafka_<sp>_<rest>{}`. Ratified by the `0053` StatsAsserter scrape.
- **R8 (counts re-pinned).** IMPL Task 1 re-pins: fuzzers 39 (→40), fixtures 54 (tail `0052`; →56), stat surface 360 (→536), DECISIONS.md tail ADR-0227 (next-free ADR-0228), BackendKind tail 30 (→31) — against the live IMPL-session tip.

---

## 14. Test surface + IMPL acceptance checklist

### 14.1 Test surface

- **Unit** at `internal/filter/network/kafkabroker/`: config parse (all PGV arms + the oneof + nested-rule arms + the `stat_prefix` IsValidName guard); the Kafka primitive decoder per type (INT16/INT32/NULLABLE_STRING/tagged-fields) + partial-frame reassembly + malformed-length throw; request-header + response-header decode incl. flexible/tagged-field arms; the `api_key → message-name` table (`TestApiKeyRoster_MatchesUpstream`); the `flexibleVersions` predicate + the ApiVersions(18) special-case (`TestHeaderVersion`); the correlation map (register/recover/erase/unregistered-miss + a `-race` concurrent test per the ADR-0223 precedent); the 176-counter roster (`TestStatRoster`); the `unknown`/`failure` semantics; sniffing-never-mutates-buffer. Plus `internal/stats/` (the `kafka.` INLINE arm). Per-task `gofmt -l` + `golangci-lint` on touched pkgs.
- **Fuzz** (Layer C): the 40th fuzzer `FuzzKafkaDecode` — random bytes through the request + response header decoders asserting no-panic + no-chain-buffer-mutation.
- **Differential** (Layer D): `0053-kafka-requests` (cross-side, multi-arm) + `0054-kafka-boot-reject` byte-exact; the full back-compat suite (54 prior dirs) byte-exact; suite 54 → 56. Every `0053` stat assertion proven live (`-count=1`).
- **Race** (Layer E): `-race -short` across touched packages (the correlation mutex is the load-bearing concurrent surface).

### 14.2 Six-gate checklist (phase-26/27/28/29 precedent)

`go build` / `go vet` / `golangci-lint run` clean; `go test ./... -race -short` green; the FULL differential suite byte-exact (56/56, incl. the back-compat dirs + `0053`/`0054`); h2spec 53/53 + proxy-wasm 10/10 re-run LIVE (asserted-unaffected — phase 31 touches no HTTP/h2/proxy-wasm path). All outputs quoted into PROGRESS.md (run honestly).

### 14.3 31 IMPL acceptance checklist

- [ ] The `kafkabroker` package lands: 5-field config parse (+ PGV arms) + the header-only decoder (request + response) + the api-key + flexibleVersions tables + the correlation map + the eager 176-counter roster; the 9th built-in + the `/contrib` blank-import; the `kafka.` INLINE prom arm.
- [ ] `/contrib v1.32.4` added + survives `go mod tidy`; the @type pinned via `proto.MessageName`.
- [ ] `0053-kafka-requests` (request/unknown/version/failure/response/unregistered-correlation arms) + `0054-kafka-boot-reject` green cross-side; back-compat 54 dirs green; suite 54 → 56; the new BackendKind (31).
- [ ] Stat surface 360 → 536 (+176 eager counters; 86 response-duration histograms deferred); fuzzers 39 → 40; BackendKind tail 30 → 31; ADRs +1 (0228 body in place; DECISIONS.md tail → ADR-0228; next-free → ADR-0229).
- [ ] BEHAVIOR_CONTRACT 31 bundle; STATE/ROADMAP row 31 `in-progress → done`; six gates GREEN LIVE quoted into PROGRESS.md.

---

## 15. Stage-close handoff

SPEC stage closes with the `spec-document-reviewer` loop (≤3 iterations) → STATE advance (`31 SPEC done`; next-skill `superpowers:writing-plans` for the PLAN) + ROADMAP row 31 stays `in-progress` (flips at IMPL phase-done) + the ADR-0228 §Context draft anchored in DECISIONS.md + commit + push (`feedback_push_to_origin`; the controller squash-merges + pushes at stage-close, subagents local-only per `feedback_subagents_no_push`). Next lifecycle step: **31 PLAN** (`superpowers:writing-plans` → `PLAN.md`; ADR-0045 split-gate re-check at PLAN time — the pre-authorized 31.1/31.2 split is the escape valve).

---

## Appendix A — Phase 31 ADR landing summary

- **ADR-0228** *(§Context drafted at this SPEC; §Decision/§Consequences body at the 31 IMPL per ADR-0044)* — the `kafka_broker` filter: TypeURL via `proto.MessageName` from the `/contrib` module (the `extensions.` segment); the header-only decode envelope (request header `api_key`/`api_version`/`correlation_id`/`client_id`/tagged-fields + response header `correlation_id`/tagged-fields; the 4-byte INT32 length prefix; the `flexibleVersions` tagged-fields-in-header rule + the ApiVersions(18) response special-case); the static `api_key → message-name` table (86 keys, Kafka 3.9.1, 71/72 excluded); the EAGER 176-counter roster under `kafka.<stat_prefix>.` (the `_request`/`_response`-suffixed naming) + the `request.unknown`/`request.failure`/`response.unknown`/`response.failure` semantics; the `correlation_id → (api_key, api_version)` per-connection correlation map under the ADR-0223 mutex (consumer #3 of the ADR-0221 WriteFilter seam); the project's FIRST `/contrib v1.32.4` go.mod dep (added with its first consumer); the `kafka.` INLINE Prometheus tag-extractor arm (no label hoist — REFUTES the BRAINSTORM hypothesis); the four active features parse-accepted-behavior-deferred (the broker-address-rewrite write-mutation flagged as a future framework-surgery sub-phase); the 9th built-in; the cross-side differential (`0053`/`0054`) UNBLOCKED by ADR-0227 + the new correlation-id-echoing BackendKind + the 40th fuzzer; ZERO framework-seam extension (pure consumer; ZERO-touch on the ADR-0226 async halt/resume seam — never-halting, byte-identical R1). Next-free after phase-31 phase-done ≈ **ADR-0229**.

---

## Appendix B — the `api_key → message-name` stat-segment table (Kafka 3.9.1; D31-2)

The per-key counter segment is `<message-name-snake>_request` / `<message-name-snake>_response`. api_keys 71 (GetTelemetrySubscriptions) and 72 (PushTelemetry) are EXCLUDED upstream (telemetry unsupported) → 86 keys. Hand-transcribed (D-P6); guarded by `TestApiKeyRoster_MatchesUpstream` at IMPL.

| key | segment root | key | segment root | key | segment root |
|---|---|---|---|---|---|
| 0 | produce | 30 | create_acls | 60 | describe_cluster |
| 1 | fetch | 31 | delete_acls | 61 | describe_producers |
| 2 | list_offsets | 32 | describe_configs | 62 | broker_registration |
| 3 | metadata | 33 | alter_configs | 63 | broker_heartbeat |
| 4 | leader_and_isr | 34 | alter_replica_log_dirs | 64 | unregister_broker |
| 5 | stop_replica | 35 | describe_log_dirs | 65 | describe_transactions |
| 6 | update_metadata | 36 | sasl_authenticate | 66 | list_transactions |
| 7 | controlled_shutdown | 37 | create_partitions | 67 | allocate_producer_ids |
| 8 | offset_commit | 38 | create_delegation_token | 68 | consumer_group_heartbeat |
| 9 | offset_fetch | 39 | renew_delegation_token | 69 | consumer_group_describe |
| 10 | find_coordinator | 40 | expire_delegation_token | 70 | controller_registration |
| 11 | join_group | 41 | describe_delegation_token | 71 | *(EXCLUDED)* |
| 12 | heartbeat | 42 | delete_groups | 72 | *(EXCLUDED)* |
| 13 | leave_group | 43 | elect_leaders | 73 | assign_replicas_to_dirs |
| 14 | sync_group | 44 | incremental_alter_configs | 74 | list_client_metrics_resources |
| 15 | describe_groups | 45 | alter_partition_reassignments | 75 | describe_topic_partitions |
| 16 | list_groups | 46 | list_partition_reassignments | 76 | share_group_heartbeat |
| 17 | sasl_handshake | 47 | offset_delete | 77 | share_group_describe |
| 18 | api_versions | 48 | describe_client_quotas | 78 | share_fetch |
| 19 | create_topics | 49 | alter_client_quotas | 79 | share_acknowledge |
| 20 | delete_topics | 50 | describe_user_scram_credentials | 80 | add_raft_voter |
| 21 | delete_records | 51 | alter_user_scram_credentials | 81 | remove_raft_voter |
| 22 | init_producer_id | 52 | vote | 82 | update_raft_voter |
| 23 | offset_for_leader_epoch | 53 | begin_quorum_epoch | 83 | initialize_share_group_state |
| 24 | add_partitions_to_txn | 54 | end_quorum_epoch | 84 | read_share_group_state |
| 25 | add_offsets_to_txn | 55 | describe_quorum | 85 | write_share_group_state |
| 26 | end_txn | 56 | alter_partition | 86 | delete_share_group_state |
| 27 | write_txn_markers | 57 | update_features | 87 | read_share_group_state_summary |
| 28 | txn_offset_commit | 58 | envelope | | |
| 29 | describe_acls | 59 | fetch_snapshot | | |

Full names: `kafka.<sp>.request.<root>_request` / `kafka.<sp>.response.<root>_response` (e.g. key 0 → `request.produce_request` / `response.produce_response`). Plus the 4 fixed `request.unknown`/`request.failure`/`response.unknown`/`response.failure`.

---

## Appendix C — the `flexibleVersions(api_key)` predicate table (Kafka 3.9.1; D31-3 / AMEND-K5)

A header uses the v2/flexible (tagged-fields) form iff `api_version ∈ flexibleVersions(api_key)`. Notation `N+` = versions ≥ N are flexible; `none` = never flexible. `responseUsesTaggedFieldsInHeader` is identical EXCEPT api_key 18 (ApiVersions) → ALWAYS false (its response header carries no tagged fields even at flexible versions). Hand-transcribed (D-P6); guarded by `TestHeaderVersion` at IMPL.

| key | flex | key | flex | key | flex | key | flex |
|---|---|---|---|---|---|---|---|
| 0 | 9+ | 22 | 2+ | 44 | 1+ | 66 | 0+ |
| 1 | 12+ | 23 | 4+ | 45 | 0+ | 67 | 0+ |
| 2 | 6+ | 24 | 3+ | 46 | 0+ | 68 | 0+ |
| 3 | 9+ | 25 | 3+ | 47 | none | 69 | 0+ |
| 4 | 4+ | 26 | 3+ | 48 | 1+ | 70 | 0+ |
| 5 | 2+ | 27 | 1+ | 49 | 1+ | 73 | 0+ |
| 6 | 6+ | 28 | 3+ | 50 | 0+ | 74 | 0+ |
| 7 | 3+ | 29 | 2+ | 51 | 0+ | 75 | 0+ |
| 8 | 8+ | 30 | 2+ | 52 | 0+ | 76 | 0+ |
| 9 | 6+ | 31 | 2+ | 53 | 1+ | 77 | 0+ |
| 10 | 3+ | 32 | 4+ | 54 | 1+ | 78 | 0+ |
| 11 | 6+ | 33 | 2+ | 55 | 0+ | 79 | 0+ |
| 12 | 4+ | 34 | 2+ | 56 | 0+ | 80 | 0+ |
| 13 | 4+ | 35 | 2+ | 57 | 0+ | 81 | 0+ |
| 14 | 4+ | 36 | 2+ | 58 | 0+ | 82 | 0+ |
| 15 | 5+ | 37 | 2+ | 59 | 0+ | 83 | 0+ |
| 16 | 3+ | 38 | 2+ | 60 | 0+ | 84 | 0+ |
| 17 | none | 39 | 2+ | 61 | 0+ | 85 | 0+ |
| 18 | 3+ (req); response header tagged-fields SUPPRESSED | 40 | 2+ | 62 | 0+ | 86 | 0+ |
| 19 | 5+ | 41 | 2+ | 63 | 0+ | 87 | 0+ |
| 20 | 4+ | 42 | 2+ | 64 | 0+ | | |
| 21 | 2+ | 43 | 2+ | 65 | 0+ | | |

(api_keys 17 SaslHandshake and 47 OffsetDelete are never flexible. The IMPL pins the exact per-version expansion from the Kafka 3.9.1 message JSON if a non-`N+` boundary is needed; the `N+` lowest-flexible-version form is the load-bearing rule.)
