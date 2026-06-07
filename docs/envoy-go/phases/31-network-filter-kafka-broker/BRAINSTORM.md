# Phase 31 Brainstorm — `kafka_broker` (FIFTH §9 Network-filters-family row; the project's first `/contrib` filter)

**Status:** brainstorm complete (lifecycle-state 0 → 1). This document captures the design decisions reached during the brainstorm session for phase 31 (`network-filter-kafka-broker`), the **FIFTH §9 Network-filters-family row** (after the phase-26 family-parent, the phase-27 `sni_cluster` flat row, the phase-28 `zookeeper_proxy` parent row, and the phase-29 `mongo_proxy` parent row). Phase 31 lands `envoy.filters.network.kafka_broker` — a passive observability sniffer that decodes the Kafka request/response **HEADER ONLY** (generic across all API keys) in **both directions** and emits full per-API-key `request.<key>` / `response.<key>` counter parity (recovering the api_key on the response side via a `correlation_id`→`api_key` per-connection map), under `kafka.<stat_prefix>.`. It is the project's **FIRST consumer of the `github.com/envoyproxy/go-control-plane/contrib` module** — the dep lands here, with its first consumer, because an unused module dep cannot survive `go mod tidy`. Phase 30 (the contrib-image pin-refresh, ADR-0227) was its standalone infra prerequisite.

The next session (lifecycle-state 1 → 2 for phase 31, skill `superpowers:writing-plans` scoped to **SPEC authoring** per the project's brainstorm→SPEC precedent) authors `docs/envoy-go/phases/31-network-filter-kafka-broker/SPEC.md` based on this brainstorm — that SPEC executes the §10 empirical-pin obligations (D31-1..D31-8) IN-SESSION against the contrib reference Envoy (`envoyproxy/envoy:contrib-v1.37.2`, ADR-0227) via the live-probe precedent (`reference_docker_probe_bridge_network`) + go-control-plane `/contrib` v1.32.4 bindings, and anchors the ADR-0228 §Context draft.

**Brainstorm session:** worktree `.worktrees/phase-31-network-filter-kafka-broker-brainstorm`, branch `phase-31-network-filter-kafka-broker-brainstorm`. Substantive predecessor on master: the phase-30 IMPL squash `2d24f34` (the reference-image pin-refresh to `envoyproxy/envoy:contrib-v1.37.2` @ `sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`, the project's FIRST pin change since ADR-0008 — a ZERO-production-LoC re-baseline of all 54 fixtures byte-identical-PASS; ADR-0227 supersedes ADR-0008).

**Brainstorm mode:** interactive with a live human. The user picked the subject + each major design decision via a multi-question dialogue:

- **Q0 subject confirmation** — `kafka_broker` (`envoy.filters.network.kafka_broker`), the FIFTH §9 Network-filters row, already SELECTED at the phase-30 BRAINSTORM (chosen from the then-3 remaining §9 candidates {redis / kafka_broker / thrift}, over the terminal-routing-proxy redis/thrift). This brainstorm confirms it as the phase-31 row. After phase 31 the remaining §9 candidates are **{redis, thrift}**.
- **Q1 scope envelope** — `Header-only decode, full counter parity both directions` (over {header-only request side only / full per-API-key body decode}). Decode the Kafka request/response HEADER ONLY (generic across all API keys); deliver FULL per-API-key `request.<key>` + `response.<key>` counter parity in BOTH directions, recovering the api_key on the response side via a `correlation_id`→`api_key` per-connection map (the mongo-29.2 correlation precedent under an ADR-0223-style per-connection mutex). The four active features (`force_response_rewrite`, `id_based_broker_address_rewrite_spec`, `api_keys_allowed`, `api_keys_denied`) are PARSE-ACCEPTED, kept at defaults in fixtures, BEHAVIOR DEFERRED (the mongo `header_delay` parse-accept-no-op precedent). `request.failure`/`response.failure` are a UNIT-TESTED coverage boundary (fixtures stay well-formed). Histograms (`response.<key>_duration`) deferred project-wide (ADR-0060).
- **Q-split sizing** — `Single unsplit row + a pre-authorized 31.1/31.2 split axis` (over {pre-split into 31.1-request / 31.2-response now / single row no escape valve}). Phase 31 is anticipated as ONE flat §9 row (request + response + correlation together) — header-only kafka is much LEANER than mongo (NO BSON parser; the response header is trivial), likely UNDER the ADR-0045 ~1500-LoC / ~25-task gate. NAME a pre-authorized 31.1-request / 31.2-response split axis as the escape valve if the SPEC/PLAN actually trips the gate (the mongo-29.1 "pre-authorized split stands unconsumed" precedent). The phase-30 forward-pointer's optional `31.x` active-features sub-phase DROPS (active features fully deferred).

Decisions that did not require live input are self-answered against `BOOTSTRAP_PROMPT.md`, `ROADMAP.md`, `ENVOY_TARGET.md`, `BEHAVIOR_CONTRACT.md`, prior ADRs (ADR-0001 .. ADR-0227), and the as-built §9 framework (26.1/26.2/26.3/27/28/29). Empirical pins requiring evidence against the contrib reference Envoy are enumerated in §10 and deferred to SPEC-drafting time per the phase 09–30 precedent.

**Document shape:** mirrors `docs/envoy-go/phases/29-network-filter-mongo-proxy/BRAINSTORM.md` section-for-section (the most recent §9 protocol-sniffer precedent), reframed for the header-only kafka_broker scope + the single-unsplit-row sizing. Phase 31 sits in a structurally meaningful position: it is **consumer #3 of the ADR-0221 `network.WriteFilter` seam** (after zookeeper_proxy and mongo_proxy); it is **seam-ZERO-touch on the ADR-0226 async halt/resume seam** (kafka_broker injects no delay → never-halting → byte-identical R1); and it is the project's **first `/contrib`-module consumer**. Per D-3.4 (context isolation), every load-bearing fact cited here lives on disk in the named files; no "see prior conversation" references appear.

**Authored:** 2026-06-07.

---

## 1. Mission and scope confirmation (31 only)

ROADMAP row `31 | network-filter-kafka-broker | 30 | in-progress | | …` (added by this brainstorm) is a **flat top-level §9 Network-filters-family row** (per ADR-0106's family flat-row discipline — phase 31 is NOT pre-split; the pre-authorized 31.1/31.2 split axis is an escape valve, §1.4). Its `depends-on` anchor is phase 30 (the contrib-image pin-refresh, the prerequisite that unblocked the cross-side differential; substantive predecessor `2d24f34`).

The Network filters family candidate roster at `ROADMAP.md` (§ Feature Families → Network filters family) immediately BEFORE this brainstorm's registration commit was: `redis, kafka_broker, thrift` (echo, direct_response, sni_cluster, rbac_network, zookeeper — DONE via phases 26/27/28; mongo — DONE via phase 29; the phase-30 pin-refresh — an infra row, not a family member). Phase 31 lands **`kafka_broker`** (this commit updates the roster paragraph to mark kafka_broker IN-PROGRESS/landing). After phase 31 phase-done, **2** family candidates remain (`redis`, `thrift`). Branch/directory/Go-package identifiers: branch `phase-31-network-filter-kafka-broker-brainstorm`, directory `31-network-filter-kafka-broker/`, filter package `internal/filter/network/kafkabroker/` (Go package `kafkabroker`, single-token-joined per the `directresponse`/`snicluster`/`zookeeperproxy`/`mongoproxy` precedent).

Phase 31 is also: (i) **consumer #3 of the ADR-0221 `network.WriteFilter` seam** — the filter implements BOTH `ReadFilter` and `WriteFilter` (one instance, both directions), receiving upstream→downstream bytes via `OnWrite` exactly as zookeeper_proxy and mongo_proxy do; per project memory `reference_network_chain_terminal_handoff_ends_ondata`, implementing `WriteFilter` (even where a method is observation-only / a no-op) is what qualifies the chain for the 28.1b post-handoff read seam, which the steady-state response decode needs. (ii) the project's **FIRST `/contrib`-module consumer** — the canonical v3 config type `envoy.extensions.filters.network.kafka_broker.v3.KafkaBroker` lives in `github.com/envoyproxy/go-control-plane/contrib v1.32.4` (lockstep with the existing `/envoy v1.32.4`); the dep lands here with its first consumer. (iii) the FIRST §9 filter using a **`correlation_id`→`api_key` per-connection map** to recover the per-API-key identity on the response side (the response header carries only `correlation_id`, NOT the api_key) — the mongo-29.2 requestID↔responseTo correlation precedent, under an ADR-0223-style per-connection mutex. (iv) **seam-ZERO-touch on the ADR-0226 async halt/resume seam** — kafka_broker injects no delay, so it is never-halting and byte-identical on R1 (contrast mongo_proxy's fault-delay).

### 1.1 What phase 31 delivers as a self-contained whole (envelope: header-only decode + full counter parity per Q1)

Phase 31 lands `envoy.filters.network.kafka_broker` as a single §9 row:

1. The **`internal/filter/network/kafkabroker/` package** — TypeURL via `proto.MessageName` on `kafka_brokerv3.KafkaBroker` (the `extensions.` lesson, `reference_network_filter_typeurl_extensions`), config parse of the 5-field `KafkaBroker` proto (`stat_prefix` PGV-required → boot-reject; `force_response_rewrite`; `id_based_broker_address_rewrite_spec`; `api_keys_allowed`; `api_keys_denied` — all five parse-accepted; the four active features kept at defaults + behavior-deferred), the **Kafka request-header decoder** (4-byte length prefix + `api_key` int16 + `api_version` int16 + `correlation_id` int32 + `client_id` nullable-string [hdr v1+] + tagged fields [hdr v2 flexible]) and **response-header decoder** (length prefix + `correlation_id` int32 + optional tagged fields), the static **(api_key, api_version) → header-version** table + the API-key name roster (the data burden — pinned empirically at SPEC), and the per-connection `correlation_id`→`api_key` map.
2. **Counters** — `request.<key>` + `response.<key>` per API key + `request.unknown`/`request.failure` + `response.unknown`/`response.failure` under `kafka.<stat_prefix>.` (the exact roster + eager-vs-dynamic creation pinned empirically at SPEC). `request.unknown`/`response.unknown` (unrecognized api_key) are handled FROM THE HEADER; `request.failure`/`response.failure` (malformed-payload decode errors) require deeper body decode to match exactly → fixtures stay well-formed and the failure path is a UNIT-TESTED coverage boundary.
3. **Registration as the 9th network-filter built-in** (`builtins.RegisterBuiltins` single insertion) + the `kafka_broker/v3` proto blank-import in `internal/bootstrap/bootstrap.go`.
4. The **`kafka.` Prometheus tag-extractor arm** at `internal/stats/name.go` (the ADR-0138 / `.zookeeper.`/`.mongo.` tag-extractor precedent), with wire-derived dynamic stat segments guarded by `stats.IsValidName` before `NewCounterIfAbsent` (per `reference_dynamic_stat_name_charset_guard` — the no-panic fuzzer needs it).
5. **Differential fixtures** — `0053-kafka-requests` (cross-side; request + response + correlation arms) + possibly `0054-kafka-boot-reject` (the `stat_prefix`-required PARSE-REJECT); a NEW BackendKind (a synthesized correlation-id-echoing TCP Kafka responder); the **40th fuzzer** `FuzzKafkaDecode` (both directions).
6. The BEHAVIOR_CONTRACT 31 bundle + STATE/ROADMAP advance + the six-gate at IMPL.

### 1.2 What phase 31 does NOT deliver (forward to §8)

See §8. Highlights: per-API-key BODY/payload decode (header-only by Q1); the four active features' BEHAVIOR (`force_response_rewrite`, `id_based_broker_address_rewrite_spec`, `api_keys_allowed`, `api_keys_denied` — parse-accepted, behavior-deferred); the broker-address-rewrite WRITE-BUFFER MUTATION (a NEW framework capability never built — flagged as a future SURGERY sub-phase); `response.<key>_duration` histograms (ADR-0060); the `request.failure`/`response.failure` deep-body-decode path (unit-tested coverage boundary); the remaining protocol proxies (`redis`, `thrift`).

### 1.3 Phase-done as the FIFTH Network-filters-family row landing

After phase 31, the family candidate count drops 3 → **2** (`redis`, `thrift`). All three anticipated/landed consumers of the ADR-0221 WriteFilter seam (zookeeper_proxy, mongo_proxy, kafka_broker) will have landed. The remaining candidates are terminal-proxy-shaped (`redis`/`thrift` — command routing + upstream pooling, each needing framework seams of its own) — each needs its own brainstorm-time risk assessment.

### 1.4 ADR-0045 split readiness — single unsplit row + a pre-authorized 31.1/31.2 escape valve (per Q-split)

Per ADR-0045 §6, the split-gate fires at `> ~25 tasks OR > ~1500 LoC`. Header-only kafka_broker is anticipated to fit comfortably as ONE phase — much LEANER than mongo (phase 29's 3-way split):

- NO BSON parser (mongo's ~250–400 LoC) — Kafka primitives are int16/int32/nullable-string/tagged-fields decoded with `encoding/binary` only.
- The response header is TRIVIAL (`correlation_id` int32 + optional tagged fields) — contrast mongo's full OP_REPLY/OP_COMMANDREPLY body decode.
- The request header is generic across all API keys (no per-API-key body decode) — the data burden is the static `(api_key, api_version) → header-version` table + the api-key name roster (data, not branching decode logic).
- NO fault-delay / async halt seam (mongo's 29.3 ~400–600 LoC framework surgery) — kafka_broker is seam-zero-touch.

Anticipated ~600–1100 production LoC / ~12–18 tasks → **single unsplit row**. The pre-authorized **31.1-request / 31.2-response** split axis is NAMED as the escape valve if the SPEC or PLAN actually trips the gate (the mongo-29.1 "pre-authorized split stands unconsumed" precedent — the split axis is feature-progressive: 31.1 = length-prefix framing + request-header decode + the header-version table + api-key roster + `request.*` counters + the `kafka.` prom arm + the 9th built-in + bootstrap blank-import + 5-field config parse + the `/contrib` dep; 31.2 = response-header decode + the correlation map + `response.*` counters). The phase-30 forward-pointer's optional `31.x` active-features sub-phase DROPS (the four active features are fully deferred). The SPEC + each PLAN re-check the gate.

### 1.5 Seed-stub alignment + package naming

No seed-stub for kafka exists. Phase 31 creates `internal/filter/network/kafkabroker/` from scratch (package `kafkabroker`; matches the `directresponse`/`snicluster`/`zookeeperproxy`/`mongoproxy` single-token-joined convention). The Kafka header decoder + the static header-version table + the api-key name roster + the correlation map all live INSIDE the kafkabroker package (no new top-level package; YAGNI — extract-at-second-consumer). NO framework change (seam-zero-touch — §3).

### 1.6 No prebrainstorm-notes branch

No `phase-31-*-prebrainstorm-notes` branch exists. Phase 31 starts cleanly from this BRAINSTORM.md.

### 1.7 Phase 31's relationship to prior framework deltas

Phase 31 is a framework-ZERO-touch §9 row (contrast mongo's 29.3 framework surgery). Prior lineage (abridged; see 26/28/29 BRAINSTORM §1.7): 26.1 `internal/filter/network/` read-filter framework → 26.2 `TerminalFilter` seam → 27 connection-scoped upstream-cluster-override (ADR-0219) → 28.1a `network.WriteFilter` seam (ADR-0221) → 28.1b post-handoff read seam (`readChainConn`/`replayRead`) → 29.3 async halt/resume seam (ADR-0226). **Phase 31 adds NO framework delta** — it is a pure consumer of the as-built §9 framework (ReadFilter + WriteFilter chains, the post-handoff read seam, the per-connection mutex pattern, the `internal/stats/` counter + tag-extractor machinery). Its "newness" is entirely in the kafkabroker package + the `/contrib` dep + the cross-side differential against the contrib reference Envoy. The deferred broker-address-rewrite (a WRITE-BUFFER MUTATION) WOULD be a new framework capability (every prior sniffer only OBSERVED bytes), but it is deferred (§2.5 / §8).

---

## 2. Design decisions

### 2.1 Subject confirmation: `kafka_broker` *(Q0 → phase 31 row registered)*

**Decision:** Phase 31 = `envoy.filters.network.kafka_broker` (proto `envoy.extensions.filters.network.kafka_broker.v3.KafkaBroker`; binding in `github.com/envoyproxy/go-control-plane/contrib v1.32.4`, import `…/contrib/envoy/extensions/filters/network/kafka_broker/v3`, package `kafka_brokerv3`; the 5-field config `stat_prefix`/`force_response_rewrite`/`id_based_broker_address_rewrite_spec`/`api_keys_allowed`/`api_keys_denied` — confirmed present at `contrib@v1.32.4` per the phase-30 BRAINSTORM §2.1; TypeURL verified at SPEC via `proto.MessageName`, D31-1).

**Rationale:** The next §9 subject already SELECTED at the phase-30 BRAINSTORM — the closest of the remaining candidates to the passive-sniffer SHAPE (decode-for-stats inserted before `tcp_proxy`; contrast the terminal-routing-proxy redis/thrift, which need their own upstream-pool + prefix-routing seams). The three contrib blockers identified at phase 30 (contrib-only reference image; the `/contrib` go.mod dep; the enormous wire protocol) are now addressed: phase 30 (ADR-0227) flipped the differential reference image to the contrib variant (unblocking the cross-side `StatsAsserter`); the `/contrib` dep lands here with its first consumer; the wire-protocol burden is bounded by the header-only Q1 envelope.

**Anticipated ADRs:** ADR-0228 (the kafka_broker filter; see §7).

### 2.2 Scope envelope: header-only decode, full per-API-key request/response counter parity *(Q1 → single row; ADR-0228)*

**Decision:** Decode the Kafka request/response HEADER ONLY (generic across all API keys) — request header = `api_key` int16, `api_version` int16, `correlation_id` int32, `client_id` nullable-string [hdr v1+], tagged fields [hdr v2 flexible]; response header = `correlation_id` int32 + optional tagged fields; 4-byte length prefix on both. NO per-API-key body/payload decode. Deliver FULL per-API-key `request.<key>` + `response.<key>` counter parity in BOTH directions, recovering the api_key on the response side via a `correlation_id`→`api_key` per-connection map. `request.unknown`/`response.unknown` (unrecognized api_key) handled FROM THE HEADER. `request.failure`/`response.failure` (malformed-payload decode-error counters) require deeper body decode to match upstream exactly → fixtures stay WELL-FORMED, the failure path is a UNIT-TESTED COVERAGE BOUNDARY (the SPEC confirms the exact semantics). The four active features (`force_response_rewrite`, `id_based_broker_address_rewrite_spec`, `api_keys_allowed`, `api_keys_denied`) are PARSE-ACCEPTED, kept at defaults in fixtures, BEHAVIOR DEFERRED (the mongo `header_delay` parse-accept-no-op precedent — so the differential mirrors the default-config behavior exactly while the active features are coverage boundaries). The `response.<key>_duration` HISTOGRAM family is DEFERRED project-wide (ADR-0060).

**Rationale:** kafka_broker is a stats-PRIMARY filter — full per-API-key counter parity is its purpose, and the header carries everything those counters need (the api_key IS in the request header; the response side recovers it via correlation). Per-API-key body decode would require dozens of API-key × version structs (the §2.1 phase-30 blocker) for ZERO additional differential coverage at default config. The data burden collapses to the static `(api_key, api_version) → header-version` table + the api-key name roster — pinned EMPIRICALLY at SPEC via the live-probe precedent (`reference_docker_probe_bridge_network`). Wire framing is adopted VERBATIM from upstream (`reference_wire_format_both_sides_see_same_bytes` — both sides see the same bytes).

### 2.3 Response-side api_key recovery: a `correlation_id`→`api_key` per-connection map *(self-answered — the mongo-29.2 correlation precedent)*

**Decision:** The Kafka response header carries only `correlation_id` (int32), NOT the api_key — so the per-API-key `response.<key>` counter requires correlating the response back to its request. On request decode, record `correlation_id → api_key` in a per-connection map; on response decode, look up the `correlation_id` to recover the api_key (and the api_version, which keys the response header-version table — D31-3), then erase the entry. Uncorrelated responses (no matching request) charge `response.unknown` (or a SPEC-pinned fallback). The map is guarded by an ADR-0223-style per-connection `sync.Mutex` (the mongo-29.2 active-query-list precedent — request decode runs pre-handoff on the read goroutine; response decode runs post-handoff in `OnWrite` on the write goroutine; the map is shared across both).

**Rationale:** Direct transfer of the mongo-29.2 requestID↔responseTo correlation pattern (first-match-by-id, erase-on-hit), under the same per-connection mutex discipline. The exact map shape (value = api_key alone, or api_key+api_version, or the full header-version key) + residual-drain semantics on connection destroy are pinned at SPEC (D31-3).

### 2.4 Differential strategy: synthesized frames + cross-side StatsAsserter *(self-answered → fixture envelope §6; UNBLOCKED by phase 30)*

**Decision:** Hermetic fixtures with hand-crafted Kafka wire frames (4-byte length prefix + request/response headers; well-formed bodies sufficient for upstream to count them); chain `[kafka_broker, tcp_proxy]` on BOTH sides (the contrib reference Envoy `envoyproxy/envoy:contrib-v1.37.2` + envoy-go); a NEW BackendKind — a synthesized correlation-id-echoing TCP Kafka responder — replies with canned response frames echoing the request `correlation_id`; `StatsAsserter` compares the mirrored `request.<key>`/`response.<key>` counters cross-side; every assertion proven live via a deliberate-break with `-count=1` (the `reference_differential_asserter_dispatch` + `reference_differential_break_protocol_count1` disciplines). NO real Kafka broker.

**Rationale:** The filter is a passive sniffer — bytes pass through unchanged on both sides, so the stat comparison IS the proof (the zookeeper/mongo lesson, unchanged). The cross-side `StatsAsserter` is UNBLOCKED by phase 30 (the contrib image now booted by the differential harness includes kafka_broker — the standard image would have rejected a kafka_broker listener as an unknown extension). A real Kafka broker would add a heavy container dependency + nondeterministic broker handshake/metadata traffic that breaks exact counter parity. The api_key→name table + the response-side correlation make the synthesized responder slightly richer than mongo's (it must echo `correlation_id`) — hence the new BackendKind rather than the existing silent `TCPSink`.

### 2.5 Active features: parse-accepted, behavior-deferred *(per Q1 — the mongo header_delay precedent; SPEC pins the PGV arms)*

**Decision:** All four active features — `force_response_rewrite`, `id_based_broker_address_rewrite_spec`, `api_keys_allowed`, `api_keys_denied` — are PARSE-ACCEPTED (the config parses without error), kept at DEFAULTS in every fixture, and their BEHAVIOR is DEFERRED. The differential mirrors the default-config (passive-sniffer) behavior exactly. The PGV arms (e.g. `stat_prefix` required → boot-reject; any constraints on the four active fields) are pinned at SPEC (D31-7).

**Rationale:** the mongo `header_delay` parse-accept-no-op precedent — parse-accepting an unconsumed field keeps the config-parse surface faithful while the behavior lands later (or never, as a coverage boundary). ⚠️ **`id_based_broker_address_rewrite_spec` is special:** the broker-address-rewrite would MUTATE the write buffer — a NEW framework capability never built (every prior §9 sniffer — echo, sni_cluster, zookeeper, mongo — only OBSERVED bytes, never rewrote them). If ever pursued, this is a future framework-SURGERY sub-phase (the write-mutation seam), flagged in §8. `api_keys_allowed`/`api_keys_denied` would gate the connection (close on a denied api_key) — also a future enforcement sub-phase.

### 2.6 First `/contrib` go.mod dependency *(self-answered — the dep lands with its first consumer)*

**Decision:** Phase 31 adds `github.com/envoyproxy/go-control-plane/contrib v1.32.4` — the project's FIRST `/contrib` dep, lockstep with the existing `…/go-control-plane/envoy v1.32.4`. It lands WITH its first consumer (the kafkabroker config parse + the `kafka_broker/v3` proto blank-import in `bootstrap.go`) — an unused module dep cannot survive `go mod tidy` (the phase-30 §2.4 finding, deferring the dep from phase 30 to here). The decode itself (length-prefix framing + int16/int32/nullable-string/tagged-fields) is implemented in-house with `encoding/binary` only — no Kafka client library.

**Rationale:** the clean boundary set at phase 30: phase 30 = the reference *image* only; phase 31 = the subject-side *binding* + the first consumer. Matches the 26/27/28/29 zero-new-third-party-decode-dep posture (the `/contrib` dep is a proto-binding module, not a decode library — consistent with the existing `/envoy` proto-binding dep).

### 2.7 Stat surface: per-API-key request/response counters under `kafka.<stat_prefix>.` *(self-answered per §9 precedent; SPEC pins roster)*

**Decision:** Mirror upstream's `request.<key>` + `response.<key>` per-API-key counters + the four fixed `request.unknown`/`request.failure`/`response.unknown`/`response.failure` counters, under the `kafka.<stat_prefix>.` scope. A new `kafka.` Prometheus tag-extractor arm at `internal/stats/name.go` (the `.zookeeper.`/`.mongo.` precedent; the api-key segment is anticipated to be label-hoisted — the mongo D-P2 lesson that dynamic segments may be FULLY label-hoisted, not in the metric name — pinned at SPEC, D31-2). Wire-derived dynamic stat segments are guarded by `stats.IsValidName` before `NewCounterIfAbsent` (per `reference_dynamic_stat_name_charset_guard` — `NewCounterIfAbsent` PANICS on invalid names; the no-panic fuzzer needs the guard at the codec boundary). Exact roster (the api-key name table) + eager-vs-dynamic creation = empirical pin at SPEC (D31-2). The stat-count DELTA is `360 → 360 + roster` (roster TBD at SPEC).

**Rationale:** stats-PRIMARY filter — full per-API-key parity is the load-bearing surface. The api-key → name table is the data burden (D31-2), the `IsValidName` guard is mandatory at the codec boundary (the charset-guard memory recurs for every protocol decoder).

---

## 3. Framework-survey result — 0 framework-seam extensions + 1 NEW filter package + 1 new go.mod dep

### 3.1 ZERO framework-seam extensions *(self-answered — seam-zero-touch)*

Phase 31 touches NO `internal/filter/network/` framework code. It is a pure consumer of the as-built §9 machinery: the ReadFilter + WriteFilter chains (26.1/28.1a), the post-handoff read seam `readChainConn`/`replayRead` (28.1b — the steady-state response decode runs post-handoff, so the filter MUST implement `WriteFilter` even where observation-only to qualify the chain for the read seam, per `reference_network_chain_terminal_handoff_ends_ondata`), and the ADR-0223 per-connection mutex pattern (the correlation map). ZERO-touch on the ADR-0226 async halt/resume seam — kafka_broker injects no delay → never-halting → byte-identical R1.

### 3.2 NEW: `internal/filter/network/kafkabroker/` *(per Q0+Q1; ADR-0228)*

Config parse (5-field `KafkaBroker`; the four active features parse-accepted) + the Kafka request-header decoder + the response-header decoder + the static `(api_key, api_version) → header-version` table + the api-key name roster + the `correlation_id`→`api_key` per-connection map + the `request.*`/`response.*` counters. Implements BOTH `ReadFilter` (request decode in `OnData`) and `WriteFilter` (response decode in `OnWrite`) — consumer #3 of ADR-0221.

### 3.3 NEW go.mod dep: `github.com/envoyproxy/go-control-plane/contrib v1.32.4` *(per §2.6)*

The project's first `/contrib` dep, added with its first consumer (the `kafka_brokerv3.KafkaBroker` parse + the `kafka_broker/v3` bootstrap blank-import). The decode is in-house (`encoding/binary`); the dep is proto bindings only.

### 3.4 REUSES

- `internal/filter/network/` (26.1/26.2/27/28.1a/28.1b/29.3) — ReadFilter + WriteFilter chains, Buffer, registry, builtins seam, chainRuntime, `readChainConn`/`writeChainConn`, the post-handoff read seam (UNTOUCHED — pure consumer).
- `internal/stats/` (06.1) — counters + `NewCounterIfAbsent` dynamic-name convention + `IsValidName` (the charset guard); the `internal/stats/name.go` Prometheus tag-extractor arm pattern (ADR-0138; `.rbac.`/`.zookeeper.`/`.mongo.` precedents — the new `kafka.` arm).
- The ADR-0223 per-connection mutex pattern (zookeeper 28.2 / mongo 29.2) — the `correlation_id`→`api_key` map.
- `internal/filter/tcpproxy/` (02/26.2/27) — the terminal in every fixture chain; untouched by 31.
- The differential harness + `StatsAsserter` (+ the fixture-dispatch + asserter-dispatch + `-count=1` break-protocol memory constraints) — now booting the contrib reference image (ADR-0227).
- `envoy.extensions.filters.network.kafka_broker.v3` proto bindings (go-control-plane `/contrib` v1.32.4 — the NEW dep, §3.3).

---

## 4. Per-route applicability — none (network filters are not route-scoped)

Per the 26/29 BRAINSTORM §4 confirmation: network filters carry no `typed_per_filter_config` surface. Not applicable to phase 31.

---

## 5. Stat surface hypothesis

### 5.1 Request side (SPEC pins)

`request.<key>` per API key (a dynamic family keyed on the api-key name from the static roster — D31-2) + the fixed `request.unknown` (unrecognized api_key from the header) + `request.failure` (malformed-payload; unit-test coverage boundary, charged only on a SPEC-pinned decode-error condition). Anticipated +N differentially-mirrored names where N ≈ the size of the exercised api-key set + 2.

### 5.2 Response side + correlation (SPEC pins)

`response.<key>` per API key (recovered via the `correlation_id`→`api_key` map) + the fixed `response.unknown` (uncorrelated / unrecognized) + `response.failure` (unit-test coverage boundary). Anticipated +M mirrored names.

### 5.3 Project stat count delta

360 → **360 + roster** (the roster = the exercised api-key set across `request.*` + `response.*` + the four fixed `*.unknown`/`*.failure` counters; the EXACT roster + eager-vs-dynamic creation pinned at SPEC, D31-2 — by the dynamic-family BEHAVIOR_CONTRACT convention established for zookeeper's `auth.<scheme>_rq` / mongo's `cmd.<cmd>.total`). Any upstream counters NOT mirrored land as BEHAVIOR_CONTRACT coverage-boundary records.

### 5.4 envoy-go-strict departure flags (anticipated; SPEC + IMPL pin)

The `response.<key>_duration` HISTOGRAM family unmirrored (ADR-0060); the `request.failure`/`response.failure` deep-body-decode path (unit-tested coverage boundary — fixtures well-formed); the four active features' behavior (`force_response_rewrite`/`id_based_broker_address_rewrite_spec`/`api_keys_allowed`/`api_keys_denied` — parse-accepted, behavior-deferred); any upstream runtime-key gating (envoy-go has no runtime layer → key defaults; the Runtime family row stays the future home).

---

## 6. Differential fixture envelope — anticipated one-to-two directories

### 6.1 Fixtures (+1–2)

- **`0053-kafka-requests`** (cross-side): chain `[kafka_broker, tcp_proxy]` both sides; the driver sends hand-crafted Kafka request frames (a spread of api_keys at a spread of header versions — incl. a flexible/tagged-field v2-header arm + an unknown-api_key arm) + the NEW BackendKind (the correlation-id-echoing TCP Kafka responder) replies with canned response frames; `StatsAsserter` compares the mirrored `request.<key>` + `response.<key>` counters incl. the correlation-recovered response names + the `request.unknown`/`response.unknown` arms; one deliberate-break liveness proof recorded with `-count=1`. (Request + response + correlation arms can share this one cross-side dir — the cross-side-XOR-boot-reject fixture-dispatch constraint allows multiple cross-side arms in one dir.)
- **`0054-kafka-boot-reject`** (boot-reject; separate dir per the `reference_differential_fixture_dispatch_constraint` — cross-side XOR boot-reject per dir): missing `stat_prefix` → both sides reject at boot. (Include-or-fold pinned at SPEC, D31-7.)

### 6.2 Total

54 → **55–56** (55 if the boot-reject arm folds; 56 with a dedicated `0054`). SPEC pins exact numbering + arm rosters.

### 6.3 New BackendKind

A NEW BackendKind (anticipated value **31**) — a synthesized correlation-id-echoing TCP Kafka responder (it must read each request frame's `correlation_id` and echo it in a canned response frame so the subject's response-side correlation has something to correlate AGAINST). Contrast the existing silent `TCPSink` (28) / the `TCPMongoResponder` (30 — mongo's canned-OP_REPLY responder). The exact response-frame shape is pinned at SPEC.

### 6.4 No conformance harness

No new conformance harness (matches 26/27/28/29). The h2spec + proxy-wasm gates re-run asserted-unaffected at the six-gate (image-independent).

---

## 7. Anticipated ADRs — 1 ADR (ADR-0228)

Next-free ADR at master tip is **ADR-0228** (DECISIONS.md tail ADR-0227 — the phase-30 pin-refresh; the ADR-0209 escape-valve reserve stands unconsumed).

- **ADR-0228** *(31)* — the `kafka_broker` filter: the header-only decode envelope (request + response headers; the `(api_key, api_version) → header-version` table + the api-key name roster), the per-API-key `request.<key>`/`response.<key>` counter roster via the `correlation_id`→`api_key` correlation map, the project's first `/contrib` go.mod dep (added with its first consumer), the four active features parse-accepted-behavior-deferred (incl. the broker-address-rewrite write-mutation flagged as a future surgery sub-phase), the `kafka.` prom tag-extractor arm, the 9th built-in, and the cross-side differential (UNBLOCKED by ADR-0227).

§Context draft lands at the SPEC; §Decision/§Consequences body at the IMPL per ADR-0044. If the SPEC/PLAN trips the ADR-0045 gate (the pre-authorized 31.1/31.2 split), the ADR may re-allocate (e.g. one ADR per sub-phase → next-free shifts); anticipated default is 1 (next-free after phase 31 ≈ **ADR-0229**).

---

## 8. Deferred items

- **Per-API-key BODY/payload decode** — out of scope by Q1 (header-only); the per-API-key counters need only the header. Not a gap — upstream's default-config counter surface is fully mirrored from the header.
- **The four active features' BEHAVIOR** — `force_response_rewrite`, `id_based_broker_address_rewrite_spec`, `api_keys_allowed`, `api_keys_denied`: parse-accepted, kept at defaults in fixtures, behavior-deferred (the mongo `header_delay` precedent). Recorded as a BEHAVIOR_CONTRACT coverage boundary.
- **The broker-address-rewrite WRITE-BUFFER MUTATION** (`id_based_broker_address_rewrite_spec` / `force_response_rewrite`) — a NEW framework capability never built (every prior §9 sniffer only OBSERVED bytes). If ever pursued, a future framework-SURGERY sub-phase (the write-mutation seam). Flagged here.
- **`api_keys_allowed`/`api_keys_denied` enforcement** (connection-close on a denied api_key) — a future enforcement sub-phase.
- **`response.<key>_duration` histograms** — deferred per ADR-0060; coverage-boundary record.
- **The `request.failure`/`response.failure` deep-body-decode path** — fixtures stay well-formed; the malformed-payload failure path is a unit-tested coverage boundary (the exact upstream `*.failure` semantics may need deeper body decode to match — SPEC confirms, D31-4).
- **Runtime-key gating** — no runtime layer exists; the filter behaves at key defaults (envoy-go-strict departure; the Runtime + hot restart family row is the future home).
- **Real-Kafka-broker integration fixtures** — out of scope; hermetic synthesized frames only.
- **The remaining protocol proxies** — `redis`, `thrift` — each its own future family phase, each needing its own brainstorm-time risk assessment (terminal-proxy seams: command routing + upstream pooling).

---

## 9. Cross-references against prior phases' deferred-items lists — closure pickup

- **30 BRAINSTORM §4 (the phase-31 forward-pointer)** — **CONSUMED by phase 31.** The forward-pointer's decomposition is REFINED here per the brainstorm decisions: (a) the header-only envelope + full per-API-key both-direction counter parity is CONFIRMED; (b) the `/contrib v1.32.4` dep lands with its first consumer (CONFIRMED); (c) the forward-pointer's anticipated 31.1/31.2/31.x pre-split is REPLACED by a single unsplit row + a pre-authorized 31.1/31.2 escape valve, and the optional `31.x` active-features sub-phase DROPS (active features fully deferred); (d) the forward-pointer's anticipated BackendKind 31 (a synthesized-response-frame TCP Kafka responder) is CONFIRMED as the correlation-id-echoing responder.
- **30 §2.1 (the three kafka_broker blockers)** — blocker 1 (contrib-only reference image) CLEARED by phase 30 (ADR-0227); blocker 2 (the `/contrib` go.mod dep) CONSUMED here with the first consumer; blocker 3 (the enormous wire protocol) BOUNDED here by the header-only Q1 envelope.
- **29 BRAINSTORM §8 (the remaining protocol proxies)** — kafka_broker PICKED UP here; `redis`/`thrift` carried (terminal-proxy surfaces, each its own future brainstorm).
- **ADR-0221 §Consequences (the WriteFilter seam consumers)** — kafka_broker is consumer #3 (after zookeeper_proxy and mongo_proxy); seam UNCHANGED (pure consumer).
- **`tcp_proxy` `downstream_cx_*` family unmirrored** (phase-27 record) — not picked up here; carried.

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution (empirical pins against the contrib reference Envoy per ADR-0004/ADR-0227)

The SPEC author executes these IN-SESSION (parallel-subagent fan-out per the 25/26/27/28/29 SPEC precedent) against the contrib reference image (`envoyproxy/envoy:contrib-v1.37.2`, ADR-0227) + go-control-plane `/contrib` v1.32.4 bindings, using the live-probe precedent (`reference_docker_probe_bridge_network`):

- **D31-1** *(SPEC-BLOCKING for the dep + TypeURL)* — verify the TypeURL via `proto.MessageName` on `kafka_brokerv3.KafkaBroker` from the `/contrib` module (the `extensions.` lesson, `reference_network_filter_typeurl_extensions` — confirm `envoy.extensions.filters.network.kafka_broker.v3.KafkaBroker`, NOT the docs string); confirm `/contrib v1.32.4` resolves AND that `go mod tidy` KEEPS it with the first consumer (an unused dep is removed).
- **D31-2** *(SPEC-BLOCKING for the stat roster)* — live-probe the contrib reference Envoy (bridge-network probe per `reference_docker_probe_bridge_network`; STRICT_DNS backend hostname; verify decode actually ran via `downstream_cx_rx_bytes_total > 0`) for the EXACT stat roster: the api_key→name table, the `request.*`/`response.*` naming, eager-vs-dynamic creation, and the Prometheus label-hoist form (the mongo D-P2 lesson — dynamic segments may be FULLY label-hoisted, NOT in the metric name).
- **D31-3** — pin the `(api_key, api_version) → request/response header-version` table incl. flexible/tagged-field framing keyed on `(api_key, api_version)`; the request-header field layout (`api_key` int16 / `api_version` int16 / `correlation_id` int32 / `client_id` nullable-string [hdr v1+] / tagged fields [hdr v2 flexible]) + the response-header layout (`correlation_id` int32 + optional tagged fields); the 4-byte length prefix; enough to hand-craft the fixture frames + the correlation-map key. Per `reference_wire_format_both_sides_see_same_bytes`: adopt upstream's framing verbatim.
- **D31-4** — confirm the `request.failure`/`response.failure` coverage boundary (well-formed fixtures; the failure path unit-tested — what upstream counts as a request/response decode failure, and whether matching it exactly needs deeper body decode) + the `request.unknown`/`response.unknown` header-derived semantics (unrecognized api_key from the header; uncorrelated response).
- **D31-5** — the `stats.IsValidName` charset-guard placement at the codec boundary (guard the wire-derived dynamic api-key segment before `NewCounterIfAbsent`; the no-panic fuzzer crashes without it — `reference_dynamic_stat_name_charset_guard`).
- **D31-6** — confirm whether kafka_broker's response side needs any close-direction-keyed counters (the `reference_close_direction_framework_gap` lesson — the framework records close TYPE not DIRECTION; likely NOT in the kafka roster — confirm at the probe). If a close-direction-keyed counter IS in the roster, defer it to a framework-surgery boundary (the mongo 29.2→29.3 close-direction precedent).
- **D31-7** — the 5-field config PGV arms (`stat_prefix` required → boot-reject) + the parse-accept-defer arms for the four active features (`force_response_rewrite`/`id_based_broker_address_rewrite_spec`/`api_keys_allowed`/`api_keys_denied` — parse without error, kept at defaults); + the `0054-kafka-boot-reject` include-or-fold decision.
- **D31-8** — the ADR-0045 task/LoC envelope confirming phase 31 fits the gate as a single row (re-checked at PLAN); whether the pre-authorized 31.1/31.2 split is consumed.

---

## 11. Prior-phase lessons applied

- **TypeURL via `proto.MessageName`, never the docs string** (`reference_network_filter_typeurl_extensions`; the `extensions.` carrier). Applied: pinning-test at IMPL Task 1 on `kafka_brokerv3.KafkaBroker` from the `/contrib` module (D31-1).
- **Wire-format pins: both sides see the same bytes** (`reference_wire_format_both_sides_see_same_bytes`; the 28.2 watch-event lesson). Applied: ALL kafka framing/header-version layouts are empirical pins against the contrib reference Envoy (D31-3); "our frame format" is never a valid deviation.
- **Dynamic stat-name charset guard** (`reference_dynamic_stat_name_charset_guard`; `NewCounterIfAbsent` PANICS on invalid names). Applied: the wire-derived api-key segment is guarded by `stats.IsValidName` at the codec boundary (D31-5); the `FuzzKafkaDecode` no-panic fuzzer needs it.
- **Close-direction is a framework gap** (`reference_close_direction_framework_gap`; the framework records close TYPE not DIRECTION). Applied: confirm at the probe whether any kafka counter is close-direction-keyed (D31-6); if so, defer to a framework-surgery boundary rather than touch the framework here.
- **Docker probes need a bridge network** (`reference_docker_probe_bridge_network`; Docker Desktop netns). Applied: the D31-2/D31-3 live probes use a shared bridge network + STRICT_DNS backend hostname + verify decode ran via `downstream_cx_rx_bytes_total > 0`.
- **Observer filters must implement WriteFilter to get the read seam** (`reference_network_chain_terminal_handoff_ends_ondata`; the 28.1b post-handoff boundary). Applied: kafka_broker implements `WriteFilter` (response decode in `OnWrite`) so the steady-state response decode runs post-handoff via the read seam.
- **Differential break protocol needs `-count=1`** (`reference_differential_break_protocol_count1`; go-test result caching serves a stale PASS). Applied: every R4 deliberate-break in phase 31 runs with `-count=1`.
- **Differential asserter dispatch + liveness** (`reference_differential_asserter_dispatch`; cross-side fixtures use `StatsAsserter`, prove every assertion live). Applied: the cross-side `StatsAsserter` IS the load-bearing proof; every fixture records a deliberate-break.
- **Cross-side XOR boot-reject per fixture dir** (`reference_differential_fixture_dispatch_constraint`). Applied: `0053` cross-side; `0054` boot-reject; never mixed.
- **Per-task gofmt + golangci-lint** (`feedback_pertask_gofmt_lint`); **subagents commit local-only** (`feedback_subagents_no_push`); **controller squash-merges + pushes at stage-close** (`feedback_push_to_origin`); **work in worktrees** (`feedback_git_worktrees`); **subagent-driven IMPL execution** (`feedback_execution_style`). Applied at every IMPL.

---

## 12. Section closeout

This brainstorm settles: (Q0) phase 31 = `kafka_broker`, the FIFTH §9 Network-filters row — consumer #3 of the ADR-0221 WriteFilter seam, the project's first `/contrib` consumer; (Q1) HEADER-ONLY decode (request + response headers, generic across all API keys) + FULL per-API-key `request.<key>`/`response.<key>` counter parity in both directions via a `correlation_id`→`api_key` per-connection map; the four active features parse-accepted-behavior-deferred (the broker-address-rewrite write-mutation flagged as a future surgery sub-phase); `request.failure`/`response.failure` a unit-tested coverage boundary; histograms deferred (ADR-0060); (Q-split) a SINGLE unsplit §9 row (header-only kafka is much leaner than mongo — no BSON parser, trivial response header, no fault-delay seam) with a PRE-AUTHORIZED 31.1-request / 31.2-response split axis as the escape valve (the optional `31.x` active-features sub-phase DROPS). Self-answered per §9 precedent: the cross-side `StatsAsserter` strategy over hermetic synthesized frames (UNBLOCKED by phase 30's contrib pin, ADR-0227); the `/contrib v1.32.4` dep added with its first consumer; the `kafka.` prom tag-extractor arm + the `IsValidName` charset guard. ZERO framework-seam extensions (seam-zero-touch) + one NEW filter package (`kafkabroker`) + one new go.mod dep. Anticipated 1 ADR (ADR-0228), fixtures 54 → 55–56, stat surface 360 → 360 + roster (TBD at SPEC), fuzzers 39 → 40 (`FuzzKafkaDecode`), BackendKind 30 → 31 (the correlation-id-echoing Kafka responder). After phase 31 the §9 candidates {redis, thrift} remain.

The next session authors `docs/envoy-go/phases/31-network-filter-kafka-broker/SPEC.md` (`superpowers:writing-plans` scoped to SPEC authoring), executing the §10 D31-1..D31-8 empirical pins IN-SESSION against the contrib reference Envoy per ADR-0004/ADR-0227, anchoring the ADR-0228 §Context draft. Per ADR-0106, row 31 registers `in-progress` (flat §9 row, no sub-phases) at this BRAINSTORM-DONE commit.
