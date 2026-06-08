# Phase 31 — `network-filter-kafka-broker`

**Status:** SPEC authored (lifecycle-state 1 → 2 at this commit; ready for the phase-31 PLAN authoring per `superpowers:writing-plans`). PLAN / PROGRESS artifacts still pending; land when this phase enters subsequent lifecycle states.

**What this phase is.** Phase 31 is the **FIFTH §9 Network-filters-family row** (a flat top-level row per ADR-0106; ROADMAP row 31, depends-on 30). It lands `envoy.filters.network.kafka_broker` (proto `envoy.extensions.filters.network.kafka_broker.v3.KafkaBroker`, in the `/contrib v1.32.4` module) — a passive both-direction Kafka observability sniffer in a NEW `internal/filter/network/kafkabroker/` package (the 9th network-filter built-in; consumer #3 of the ADR-0221 WriteFilter seam after zookeeper_proxy/mongo_proxy). It is the project's **FIRST `github.com/envoyproxy/go-control-plane/contrib` consumer** — phase 30 (the contrib-image pin-refresh to `envoyproxy/envoy:contrib-v1.37.2`, ADR-0227) was its standalone infra prerequisite. It is framework-ZERO-touch (pure consumer of the as-built §9 machinery) and adds ONE go.mod dep (`/contrib v1.32.4`, with its first consumer).

**The envelope (BRAINSTORM Q1).** Decode the Kafka request/response **HEADER ONLY** (generic across all API keys — request header `api_key`/`api_version`/`correlation_id`/`client_id`/tagged-fields; response header `correlation_id`/tagged-fields; a 4-byte INT32 length prefix on both) and emit FULL per-API-key `request.<msg>_request` / `response.<msg>_response` counter parity in BOTH directions under `kafka.<stat_prefix>.`, recovering the api_key on the response side via a `correlation_id → (api_key, api_version)` per-connection map (the mongo-29.2 correlation precedent under an ADR-0223-style mutex), plus the four fixed `request.unknown`/`request.failure`/`response.unknown`/`response.failure`. The four active features (`force_response_rewrite`, `id_based_broker_address_rewrite_spec`, `api_keys_allowed`, `api_keys_denied`) are PARSE-ACCEPTED, behavior-DEFERRED.

**The six empirical amendments (this SPEC session, D31-1..D31-8 executed live).** The §11 scrape against the contrib reference image + go-control-plane `/contrib` v1.32.4 + upstream v1.37.2 contrib source CONFIRMED the two blocking pins and REFINED/REFUTED several BRAINSTORM hypotheses (SPEC §1.1):

- **AMEND-K1** — the per-key stat segment is the `_request`/`_response`-SUFFIXED message-name snake-case (`request.produce_request`, `request.api_versions_request`), NOT the bare api-key name.
- **AMEND-K2** — the Prometheus exposition is FULLY INLINED with EMPTY labels (`envoy_kafka_kprobe_request_api_versions_request{} 1`); **REFUTES** the BRAINSTORM's label-hoist hypothesis. The `kafka.` `name.go` arm is the simplest INLINE arm (the zookeeper shape, not the mongo label-hoist shape).
- **AMEND-K3** — the roster is EAGER + **176 counters** (86 request + 86 response per-key + 4 fixed; api_keys 71/72 telemetry-excluded; 86 response-duration histograms DEFERRED per ADR-0060). Stat surface **360 → 536** — the project's largest single-phase stat jump.
- **AMEND-K4** — `unknown` is keyed on (api_key, api_version) (unknown VERSION of a known key also → unknown); `failure` is partly header-reproducible (malformed header; response unregistered-correlation_id) so `response.failure` earns a differential arm.
- **AMEND-K5** — header-version is a `flexibleVersions(api_key)` predicate (tagged fields iff `api_version ∈ flexibleVersions`), with a SPECIAL CASE: ApiVersions(18) response header suppresses tagged fields. Kafka source 3.9.1.
- **AMEND-K6** — the only genuinely-new go.mod dep is `/contrib v1.32.4` (kafka_broker/v3 is self-contained; `cncf/xds`/`protoc-gen-validate`/`vtprotobuf`/`/envoy` already in the closure); the @type carries `extensions.`.
- **AMEND-K7** — the `IsValidName` charset guard is satisfied BY CONSTRUCTION (the api-key names are table-bounded, not arbitrary wire strings; only `stat_prefix` needs the config-boundary guard).

**Split (BRAINSTORM Q-split, D31-8 confirmed).** SINGLE unsplit §9 row (~805–1200 production LoC / ~13–18 tasks — header-only kafka is much leaner than mongo: no BSON parser, trivial response header, no fault-delay/async-halt seam; the two static tables are DATA decoded with `encoding/binary` only). UNDER the ADR-0045 gate. The pre-authorized 31.1-request / 31.2-response split axis STAYS UNCONSUMED (the mongo-29.1 precedent); the PLAN re-checks the gate.

**Masters (read first, in order):**

1. [`./BRAINSTORM.md`](./BRAINSTORM.md) — **the phase-31 charter.** The Q0/Q1/Q-split decisions (§2); the framework-survey (§3 — zero seam touch); the stat-surface hypothesis (§5); the fixture envelope (§6); the anticipated ADR-0228 (§7); the deferred items (§8); the §10 D31-1..D31-8 empirical pins this SPEC executes.
2. [`./SPEC.md`](./SPEC.md) — **the authoritative phase SPEC.** The empirical-finding-driven scope (§1.1, AMEND-K1..K7); the single-row split disposition (§3.0); the package + framework reuse (§3); the proto roster + PGV arms (§5/§6); the stat surface (§7 — the 176-counter eager roster + the `kafka.` INLINE prom arm); the differential taxonomy (§8 — `0053`/`0054` + the new BackendKind); the ~13–18-task PLAN spine (§10); the D31-1..D31-8 empirical-pin block (§11); the api-key + flexibleVersions tables (Appendices B/C).
3. [`../../ENVOY_TARGET.md`](../../ENVOY_TARGET.md) — the current pin (`envoyproxy/envoy:contrib-v1.37.2` @ `sha256:7edd5b0f…`, ADR-0227) — the reference image the cross-side differential boots.
4. `docs/envoy-go/DECISIONS.md` — ADR-0228 §Context (this phase's ADR; §Decision/§Consequences body at the 31 IMPL per ADR-0044) + ADR-0106 (§9 flat-row discipline) + ADR-0045 (split-gate) + ADR-0221 (WriteFilter seam) + ADR-0223 (per-connection mutex) + ADR-0060 (histograms deferred) + ADR-0227 (the contrib pin) + ADR-0044 (ADR §-body timing).
5. `internal/filter/network/mongoproxy/` (the §9-sniffer package-shape precedent for `kafkabroker`) + `internal/stats/name.go` (the tag-extractor arm pattern for the new `kafka.` arm).

**Scope at phase 31** (per the SPEC + ROADMAP row 31):

- (a) The **`internal/filter/network/kafkabroker/` package** — TypeURL via `proto.MessageName`; the 5-field config parse (+ PGV arms); the Kafka primitive decoder + the request/response header decoders; the static `api_key → message-name` (86 keys) + `flexibleVersions(api_key)` tables; the `correlation_id → (api_key, api_version)` per-connection map; the EAGER 176-counter roster.
- (b) **Registration** as the 9th built-in + the `kafka_broker/v3` blank-import (the `/contrib` path) in `bootstrap.go`.
- (c) The **`/contrib v1.32.4` go.mod dep** (the project's first, with its first consumer).
- (d) The **`kafka.` INLINE Prometheus arm** in `internal/stats/name.go`.
- (e) **Differential fixtures** `0053-kafka-requests` (cross-side, multi-arm) + `0054-kafka-boot-reject`; a NEW BackendKind (the correlation-id-echoing TCP Kafka responder, anticipated value 31); the 40th fuzzer `FuzzKafkaDecode`.
- (f) **ADR-0228** + the BEHAVIOR_CONTRACT 31 bundle (stat surface 360 → 536) + STATE/ROADMAP advance + the six-gate at IMPL.

**Counts (advance at IMPL):** fixtures 54 → 56; fuzzers 39 → 40; stat surface 360 → 536 (+176); BackendKind tail 30 → 31; DECISIONS.md tail ADR-0227 → ADR-0228 (next-free ADR-0229). ZERO framework-seam extension.

**Predecessor:** Phase 30 CLOSED (the contrib-image pin-refresh; ADR-0227; the differential reference is now `envoyproxy/envoy:contrib-v1.37.2`).

**Successor:** the remaining §9 candidates {redis, thrift} (both terminal-proxy surfaces — command routing + upstream pooling — each its own future brainstorm). After phase 31 phase-done the §9 candidate count drops 3 → 2.
