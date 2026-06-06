# Phase 30 Brainstorm — reference-image pin-refresh to `envoy-contrib` (infra prerequisite for the kafka_broker §9 row)

**Status:** brainstorm complete (lifecycle-state 0 → 1). This document captures the design decisions reached during the brainstorm session for phase 30 (`pin-refresh-envoy-contrib`), a **flat infrastructure row** (NOT a §9 Network-filters-family row). Phase 30 does exactly one thing: swap the SHA-pinned differential reference image from `envoyproxy/envoy:v1.37.2` → `envoyproxy/envoy-contrib:v1.37.2`, proving the contrib image is a behavioral **superset** (all 54 existing differential fixtures + the conformance gates stay green). It ships **no feature code and no new go.mod dependency** — it is a pure pin-refresh per `ENVOY_TARGET.md` doctrine **D-3.7**, executed because the next intended §9 subject (`kafka_broker`) is an Envoy **contrib** extension absent from the standard reference image.

The next session (lifecycle-state 1 → 2 for phase 30, skill `superpowers:writing-plans` scoped to SPEC authoring per the project's brainstorm→SPEC precedent) authors `docs/envoy-go/phases/30-pin-refresh-envoy-contrib/SPEC.md` based on this brainstorm — that SPEC executes the §6 empirical-pin obligations IN-SESSION (confirm the `envoyproxy/envoy-contrib:v1.37.2` tag exists + pull it + capture its `sha256:` digest + run the full 54-fixture suite against it) and anchors the ADR-0227 §Context draft.

**Brainstorm session:** worktree `.worktrees/phase-30-pin-refresh-envoy-contrib-brainstorm`, branch `phase-30-pin-refresh-envoy-contrib-brainstorm`, branched from master tip `30efcb1` (`next-prompt.txt: name the live SHA-fill tip 5710db0` — a docs-only commit). Substantive predecessor on master: `d671263` (the phase-29.3 IMPL squash — the async halt/resume seam + mongo fault-delay + access log + the parent-row-29 ROLLUP; ADR-0226).

**Brainstorm mode:** interactive with a live human. The user picked the subject + each major decision via a multi-question dialogue:

- **Q0 subject selection** — `kafka_broker` chosen from the 3 remaining §9 Network-filters candidates {redis / kafka_broker / thrift}. A binding/architecture probe executed DURING this brainstorm reframed the choice: `redis_proxy` and `thrift_proxy` are **terminal L7 routing proxies** (own upstream connection pool / prefix routing / command-splitting for redis; route-discovery + a nested `ThriftFilter` sub-chain + framed/unframed transports for thrift) — a fundamentally different architecture from the passive `[<filter>, tcp_proxy]` sniffer the framework built twice (zookeeper, mongo). `kafka_broker` is the closest fit to the sniffer SHAPE (decode-for-stats + a Metadata broker-address-rewrite passthrough, inserted before `tcp_proxy`) — but it is an Envoy **contrib** extension, which surfaced three blockers (§2.1).
- **Q-reconfirm** — after the three kafka_broker blockers were surfaced (new contrib go.mod dep; cross-side differential blocked behind a pin-refresh; enormous wire protocol), the user re-confirmed `kafka_broker anyway` — accept the departures, keep the cross-side differential, reduced wire envelope, multi-phase.
- **Q-pin-sequencing** — `Standalone phase first` chosen from {standalone phase first / fold as kafka sub-phase 0 / verify contrib parity first}. The mandatory contrib-image pin-refresh becomes **its own phase 30** (this phase); the kafka_broker filter work becomes a later **phase 31** (§9 row, pre-split into sub-phases). Cleanest per D-3.7 ("the pin is changed only via a dedicated phase that re-baselines the differential suite").
- **Q-this-cycle-scope** — `Phase 30 spec; phase 31 forward-pointer` chosen. This brainstorm fully designs phase 30 (the pin-refresh) and captures the phase-31 kafka_broker decomposition as a non-binding **forward-pointer** (§4) only; kafka_broker gets its own full brainstorm after phase 30 lands. Matches the brainstorming skill's "decompose, then brainstorm the first sub-project" rule.

Decisions that did not require live input are self-answered against `BOOTSTRAP_PROMPT.md`, `ROADMAP.md`, `ENVOY_TARGET.md`, `BEHAVIOR_CONTRACT.md`, prior ADRs (ADR-0001 .. ADR-0226), and the as-built differential harness (`test/differential/harness.go`). Empirical pins requiring evidence (the contrib image's existence + digest + 54-fixture parity) are enumerated in §6 and deferred to SPEC/IMPL time per the project precedent — the SHA itself is captured at IMPL via `docker pull` + `docker inspect` per D-3.7.

**Document shape:** adapted from `docs/envoy-go/phases/29-network-filter-mongo-proxy/BRAINSTORM.md`, reframed for an infrastructure pin-refresh (no sub-phases, no §9 membership, no new package, no feature LoC). Per D-3.4 (context isolation), every load-bearing fact cited here lives on disk in the named files; no "see prior conversation" references appear.

**Authored:** 2026-06-06.

---

## 1. Mission and scope confirmation (30 only)

ROADMAP row `30 | pin-refresh-envoy-contrib | 29 | in-progress | | …` (added by this brainstorm) is a **flat top-level infrastructure row** — NOT a §9 Network-filters-family row, and NOT a member of any feature family. It is registered `in-progress` with NO sub-phase list (phase 30 is not pre-split — §1.4). Its `depends-on` anchor is phase 29 (the last landed phase; substantive predecessor `d671263`).

Phase 30 is the project's **FIRST reference-image pin change** (ADR-0008 has been the unchanged pin since phase 02). It is the dedicated re-baselining phase mandated by `docs/envoy-go/ENVOY_TARGET.md` doctrine D-3.7: *"the pin is changed only via a dedicated phase that re-baselines the differential suite."* Its motivation is downstream: it unblocks every Envoy **contrib** network filter — `kafka_broker` first (phase 31, §4), with `sip_proxy` / `mysql_proxy` / `postgres_proxy` / `rocketmq_proxy` / `generic_proxy` latent in the same `go-control-plane/contrib@v1.32.4` module as future candidates.

### 1.1 What phase 30 delivers as a self-contained whole

1. The **reference image swap**: `docs/envoy-go/ENVOY_TARGET.md` `**Tag:**` → `envoyproxy/envoy-contrib:v1.37.2` + `**SHA256:**` → the new image digest (captured at IMPL) + refreshed release-notes URL + `Last verified` date. This single file is the ONLY runtime-load-bearing change — `test/differential/harness.go::parseEnvoyTarget` reads `**Tag:**`/`**SHA256:**` from it and `StartReferenceProxy` boots `pin.SHA256`, so editing this file repoints every reference-booting fixture (§3.1).
2. The **54-fixture re-baseline**: the full `ls -d test/fixtures/[0-9]* | wc -l` = 54 differential suite run byte-identical-PASS against the new image, plus the conformance gates (h2spec 53/53, proxy-wasm 10/10) re-run as a sanity gate (image-independent — §2.3). The **divergence policy** (D-3.7 step 3): any divergence is investigated → envoy-go fixed to match, OR recorded as a BEHAVIOR_CONTRACT extension with its own ADR. Expected divergences: **none** (contrib = the same upstream source + extra compiled-in extensions).
3. **ADR-0227** superseding ADR-0008 (the contrib-variant pin; same upstream version v1.37.2; the superset-parity evidence; the contrib filters it unblocks).
4. The **BEHAVIOR_CONTRACT.md** current-pin lines (2–3 occurrences naming the live SHA) updated to the new digest.
5. STATE/ROADMAP advance + next-prompt rewrite for the phase-30 SPEC cold-start (this BRAINSTORM commit), and at IMPL the standard six-gate + completion bundle.

### 1.2 What phase 30 does NOT deliver (forward to §4 + §5)

- **No kafka_broker code, no `/contrib` go.mod dep, no new filter package, no new stat, no new fixture, no new fuzzer, no new BackendKind.** Counts are unchanged across phase 30 (54 fixtures / 39 fuzzers / 360 stats / BackendKind tail 30). The `/contrib` dependency lands in phase 31.1 with its first consumer (an unused module dep cannot survive `go mod tidy` — §2.4).
- **No rewrite of historical SHA references.** The many `envoyproxy/envoy:v1.37.2` + old-digest mentions in prior phases' `PROGRESS.md`/`SPEC.md`/`PLAN.md` are durable records of what was true at those phases — they stay frozen (only the live `ENVOY_TARGET.md` + the current-pin `BEHAVIOR_CONTRACT.md` lines change).
- **No `harness.go`/`harness_test.go` code change.** The `v1.34.0` strings in `harness_test.go` are a *parser* unit-test sample (`TestParseEnvoyTarget_PullsTagAndDigest` feeds a hardcoded sample doc) — they have NO coupling to the live pin (the live pin is v1.37.2) and stay as-is (§3.2).

### 1.3 Not a family-row landing

Phase 30 lands no §9 family row; the family candidate roster `redis / kafka_broker / thrift` is UNCHANGED by phase 30. The kafka_broker §9 row is **phase 31** (§4), which depends on phase 30. After phase 30 the remaining §9 candidates are still `redis / kafka_broker / thrift` (kafka_broker now *unblocked* but not yet landed).

### 1.4 ADR-0045 split readiness — NO split (well under the gate)

Per ADR-0045 §6 the split-gate fires at `> ~25 tasks OR > ~1500 LoC`. Phase 30 ships **zero production LoC** and an anticipated **~4–6 task spine** (capture digest → flip `ENVOY_TARGET.md` → re-baseline 54 fixtures → ADR-0227 + contract-pin update → conformance sanity → completion bundle). No split; no pre-split sub-phases.

### 1.5 Package naming / seed-stub alignment

No new Go package. No seed-stub. No `internal/` change. The only code surface touched is configuration metadata (`docs/envoy-go/ENVOY_TARGET.md`) read by the test harness.

### 1.6 No prebrainstorm-notes branch

No `phase-30-*-prebrainstorm-notes` branch exists. Phase 30 starts cleanly from this BRAINSTORM.md.

---

## 2. Design decisions

### 2.1 Why phase 30 exists: the three kafka_broker blockers *(Q0 probe → Q-reconfirm)*

The Q0 binding/architecture probe found that `kafka_broker` — though the closest of the 3 remaining §9 candidates to the passive-sniffer SHAPE — uniquely breaks three invariants that every prior §9 phase (cors, sni_cluster, zookeeper, mongo) preserved:

1. **New go.mod dependency.** Reference Envoy v1.37.2's canonical config type is `envoy.extensions.filters.network.kafka_broker.v3.KafkaBroker` (verified via the v1.37.2 docs). Its Go binding lives in the **separate `github.com/envoyproxy/go-control-plane/contrib` module** (import `…/contrib/envoy/extensions/filters/network/kafka_broker/v3`, package `kafka_brokerv3`) — confirmed present at `contrib@v1.32.4`, the SAME version line as the project's existing `…/go-control-plane/envoy v1.32.4` dep. The project does NOT currently depend on `/contrib`. (The legacy `envoy.config.filter.network.kafka_broker.v2alpha1.KafkaBroker` IS already vendored in the `/envoy` submodule, but v1.37.2 will not accept it — the filter moved to contrib/v3; and using v2alpha1 would not rescue anything, since the *reference* side rejects kafka_broker regardless of which binding the subject parses — blocker 2.)
2. **The cross-side differential is blocked behind this pin-refresh.** `kafka_broker` is a **contrib-only** extension — it is NOT compiled into the SHA-pinned standard `envoyproxy/envoy:v1.37.2` image (ADR-0008 / `ENVOY_TARGET.md`). The cross-side `StatsAsserter` differential — the load-bearing proof for every §9 row — boots that standard image (`StartReferenceProxy` → `pin.SHA256`), which would reject a kafka_broker listener as an unknown extension. Running it requires the `envoyproxy/envoy-contrib:v1.37.2` image — exactly the swap phase 30 performs.
3. **Enormous wire protocol** (a phase-31 concern, not phase 30): dozens of Kafka API keys, each versioned v0..vN, with flexible/tagged-field request-header variants keyed on `(api_key, api_version)` — far larger than mongo's 7-opcode envelope. Mitigated at phase 31 by a reduced stats-at-default-config envelope (§4).

The user re-confirmed kafka_broker **with** the cross-side differential intact (not unit/golden-only), which makes the contrib-image pin-refresh a hard prerequisite — hence phase 30.

### 2.2 Target image + change surface *(Q-pin-sequencing → standalone phase first)*

**Decision:** swap to `envoyproxy/envoy-contrib:v1.37.2` — the **same upstream version** as the current pin, the contrib **variant**. The contrib image is built from the identical Envoy source with all contrib extensions additionally compiled in; the entrypoint binary is still `envoy`, so the harness container `Cmd` (`["envoy", "--config-yaml", …, "--concurrency", "1"]`) is unchanged. The digest is captured at IMPL via `docker pull envoyproxy/envoy-contrib:v1.37.2` + `docker inspect --format='{{index .RepoDigests 0}}'` per the `ENVOY_TARGET.md` refresh procedure.

**Rationale:** keeping the version fixed (only the variant changes) isolates the variable: any 54-fixture divergence is attributable to the core↔contrib build difference, not a version bump. This is the minimal, most-reviewable form of the prerequisite.

### 2.3 Re-baseline gate + divergence policy *(self-answered per D-3.7)*

**Decision:** the IMPL runs the full 54-fixture differential suite against the new image; byte-identical-PASS is the bar. Conformance (h2spec 53/53, proxy-wasm 10/10) is **image-independent** — both suites exercise the envoy-go subject, not the reference container — so they carry no correctness dependency on the swap; they are re-run only as a standing sanity gate. Any fixture divergence is investigated per D-3.7 step 3: either envoy-go is fixed to match the contrib image's behavior, or the divergence is recorded as a `BEHAVIOR_CONTRACT.md` extension with its own ADR.

**Rationale:** the re-baseline IS the proof that "contrib is a superset" — asserted by running, not assumed. The risk surface is narrow but real (e.g. a build-flag-dependent or version-string-bearing stat) and the gate is what catches it.

### 2.4 The `/contrib` go.mod dep is deferred to phase 31.1 *(self-answered — refinement of the Q-this-cycle framing)*

**Decision:** phase 30 adds NO go.mod dependency. `github.com/envoyproxy/go-control-plane/contrib v1.32.4` lands in **phase 31.1** alongside its first consumer (the code that parses the kafka_broker config + blank-imports the proto).

**Rationale:** an unused module dependency does not survive `go mod tidy` (Go removes deps with no importing package), so the dep cannot be added in phase 30 without a consumer. The clean boundary is: phase 30 = the reference *image* only; phase 31 = the subject-side *binding*. This is purely the image-pin prerequisite.

### 2.5 ADR-0227 supersedes ADR-0008 *(self-answered per D-3.7 step 5)*

**Decision:** a new **ADR-0227** records the pin change and supersedes ADR-0008 (the original v1.37.2 standard-image pin). §Context drafted at the SPEC; §Decision/§Consequences body at the IMPL per ADR-0044. Next-free ADR after phase 30 ≈ **ADR-0228**. The ADR-0209 escape-valve reserve stays unconsumed.

---

## 3. Change-surface inventory

### 3.1 The one runtime-load-bearing edit

`docs/envoy-go/ENVOY_TARGET.md` — `parseEnvoyTarget` (regex on `**Tag:**` + `**SHA256:**`) reads it; `StartReferenceProxy(ctx, pin, …)` uses `pin.SHA256` as the testcontainers image. Editing Tag + SHA256 repoints all reference-booting fixtures atomically. Also update the release-notes URL + `Last verified` date in the same file.

### 3.2 Code deliberately UNTOUCHED

`test/differential/harness.go` + `test/differential/harness_test.go` — the `EnvoyPin` struct comment and the `TestParseEnvoyTarget_*` unit tests use a hardcoded `envoyproxy/envoy:v1.34.0` / `sha256:abc123def456` **sample document** to test the *parser*, not the live pin. No behavioral coupling; left as-is to minimize churn. (Optional cosmetic refresh of the sample to a contrib example is a SPEC micro-decision — default: leave it.)

### 3.3 Docs touched

`docs/envoy-go/BEHAVIOR_CONTRACT.md` (the 2–3 current-pin lines naming the live SHA → new digest); `docs/envoy-go/DECISIONS.md` (ADR-0227); `docs/envoy-go/ROADMAP.md` (row 30); `docs/envoy-go/STATE.md`; `next-prompt.txt`; and the phase dir (`SPEC.md` at the next session, `PROGRESS.md` at IMPL).

### 3.4 Frozen (NOT rewritten)

Every historical `PROGRESS.md`/`SPEC.md`/`PLAN.md` mention of `envoyproxy/envoy:v1.37.2` + the old `c5e8a68e…` digest — durable per-phase records of what was true then.

---

## 4. Phase-31 forward-pointer — kafka_broker (`envoy.filters.network.kafka_broker`) *(non-binding; its own brainstorm later)*

Captured here so the decomposition is on record; phase 31 gets its own full BRAINSTORM → SPEC → PLAN → IMPL cycle after phase 30 lands.

- **Dependency:** `github.com/envoyproxy/go-control-plane/contrib v1.32.4` (lockstep with the existing `/envoy v1.32.4`), import `…/contrib/envoy/extensions/filters/network/kafka_broker/v3`, proto `envoy.extensions.filters.network.kafka_broker.v3.KafkaBroker` (verify the TypeURL via `proto.MessageName`, per `reference_network_filter_typeurl_extensions`; blank-import the proto in `bootstrap.go`). The v3 config carries 5 fields: `stat_prefix`, `force_response_rewrite`, `id_based_broker_address_rewrite_spec`, `api_keys_allowed`, `api_keys_denied`.
- **Envelope (reduced — stats at default config):** per-API-key `request.<type>` / `response.<type>` counters + `request.unknown` / `request.failure` + `response.unknown` / `response.failure`, under `kafka.<stat_prefix>.`. Response counters require a **correlation_id→api_key** per-connection map (the mongo 29.2 correlation precedent under the ADR-0223 per-connection mutex). `response.<type>_duration` **histograms deferred** project-wide (ADR-0060). `force_response_rewrite` + `id_based_broker_address_rewrite_spec` + `api_keys_allowed`/`api_keys_denied` are **parse-accepted, kept at defaults in fixtures, behavior deferred** (the mongo `header_delay` parse-accept-no-op precedent) — so the differential mirrors the default-config behavior exactly while the active features are later sub-phases / coverage boundaries.
- **Decode model:** Kafka wire = 4-byte length prefix + request header (`api_key` int16, `api_version` int16, `correlation_id` int32, `client_id` nullable-string [hdr v1+], tagged fields [hdr v2 flexible]); response = length prefix + `correlation_id` int32 + optional tagged fields. The data burden is the static **(api_key, api_version) → request/response header-version** table + the API-key name roster — pinned at SPEC via the live-probe precedent (`reference_docker_probe_bridge_network`). `request.failure`/`response.failure` semantics (malformed-payload detection) may require deeper body decode to match exactly — likely scoped by keeping fixtures well-formed and treating the failure path as a unit-tested coverage boundary (SPEC decides). Wire framing adopted verbatim from upstream (`reference_wire_format_both_sides_see_same_bytes`).
- **Seam:** response side via the ADR-0221 WriteFilter conn-wrap (the zookeeper/mongo `OnWrite` precedent); **seam-zero-touch** on the ADR-0226 async halt/resume seam (kafka_broker injects no delay → never-halting, byte-identical R1). ⚠️ The deferred broker-address-rewrite would *mutate* the write buffer — a **new framework capability** (every prior sniffer only OBSERVED bytes, never rewrote them); flag as the hard/surgery sub-phase if ever pursued.
- **Pre-split (ADR-0045, anticipated at the phase-31 BRAINSTORM):** **31.1** request side (length-prefix framing + request-header decode incl. the header-version table + API-key roster + `request.*` counters + the `kafka.` Prometheus tag-extractor arm + the 9th built-in + bootstrap blank-import + 5-field config parse + the `/contrib` dep); **31.2** response side + correlation (response-header decode + the correlation map + `response.*` counters); **31.x** (optional) active features (`api_keys_allowed`/`api_keys_denied` connection-close enforcement; the broker-address-rewrite write-mutation sub-phase). Histograms deferred project-wide.
- **Differential:** cross-side `StatsAsserter` (UNBLOCKED by phase 30) over hermetic synthesized-frame fixtures; a new BackendKind (a synthesized-response-frame TCP Kafka responder, anticipated **31**).

---

## 5. Anticipated ADRs — 1 ADR (ADR-0227)

Next-free ADR at master tip is **ADR-0227** (DECISIONS.md tail ADR-0226; the ADR-0209 escape-valve reserve stands unconsumed).

- **ADR-0227** *(30)* — reference-image pin-refresh to `envoyproxy/envoy-contrib:v1.37.2`; **supersedes ADR-0008**. Records: the contrib-variant choice (same upstream version); the motivation (unblock contrib network filters — kafka_broker first); the 54-fixture superset-parity evidence; the divergence outcome (expected: none).

§Context draft lands at the SPEC; §Decision/§Consequences body at the IMPL per ADR-0044.

---

## 6. BRAINSTORM-time open questions for SPEC/IMPL-time resolution (empirical pins)

The SPEC author executes these IN-SESSION (the digest capture lands at IMPL per D-3.7):

- **D30-1** *(SPEC/IMPL-BLOCKING)* — confirm `envoyproxy/envoy-contrib:v1.37.2` exists as a published tag, pulls, and capture its `sha256:` `RepoDigests` digest (the value that goes into `ENVOY_TARGET.md` `**SHA256:**`).
- **D30-2** — run the full 54-fixture differential suite against the contrib image; confirm byte-identical PASS. Investigate + classify any divergence per D-3.7 step 3 (envoy-go fix vs BEHAVIOR_CONTRACT extension + ADR).
- **D30-3** — confirm the conformance gates (h2spec 53/53, proxy-wasm 10/10) are unaffected (expected: trivially, they don't boot the reference image).
- **D30-4** — the exact `BEHAVIOR_CONTRACT.md` current-pin line set to update (vs the frozen historical references) — `grep` for the live digest, edit only the current-pin lines.
- **D30-5** — confirm the `harness_test.go` parser sample needs no change (leave-as-is default) and that no other code path hardcodes the live tag/digest.
- **D30-6** — the ADR-0227 supersession wording against ADR-0008 + the `ENVOY_TARGET.md` refresh-procedure step-by-step compliance record.

---

## 7. Prior-phase lessons applied

- **The pin is never changed ad-hoc** (`ENVOY_TARGET.md` D-3.7). Applied: phase 30 is the dedicated re-baselining phase; the change lands as a single reviewed phase with a green differential surface.
- **Wire-format / image pins are empirical, captured live** (`reference_docker_probe_bridge_network` — Docker Desktop netns lesson). Applied: the digest is captured via `docker pull` + `docker inspect` at IMPL, not guessed.
- **Differential liveness must be proven** (`reference_differential_asserter_dispatch`). Applied: the re-baseline is a real 54-fixture run, `-count=1` (`reference_differential_break_protocol_count1`) to defeat result caching.
- **Per-task gofmt + golangci-lint** (`feedback_pertask_gofmt_lint`) — N/A for the zero-production-LoC phase 30, but the six-gate (`go build`/`go vet`/`golangci-lint`/`go test -race -short`/differential/conformance) still runs at IMPL.
- **Subagents commit local-only** (`feedback_subagents_no_push`); **controller squash-merges + pushes at stage-close** (`feedback_push_to_origin`); **work in worktrees** (`feedback_git_worktrees`); **subagent-driven IMPL execution** (`feedback_execution_style`). Applied at every stage.

---

## 8. Counts at this BRAINSTORM (unchanged by phase 30)

- active differential fixtures **54** (tail `0052-mongo-fault-delay`); phase 30 adds none.
- fuzzers **39**; phase 30 adds none.
- stat surface **360**; phase 30 adds none.
- BackendKind tail **30** (`TCPMongoResponder`); phase 30 adds none.
- DECISIONS.md tail **ADR-0226**; phase 30 consumes **ADR-0227** (next-free after ≈ ADR-0228).
- reference pin: `envoyproxy/envoy:v1.37.2` @ `sha256:c5e8a68e…` (ADR-0008) → `envoyproxy/envoy-contrib:v1.37.2` @ `<captured-at-IMPL>` (ADR-0227, this phase).
- conformance h2spec 53/53 + proxy-wasm 10/10 (image-independent; re-run as sanity at the six-gate).
- Toolchain: Go 1.26.2; golangci-lint 1.64.8 (ADR-0009); go-control-plane `/envoy` v1.32.4 (ADR-0008) — `/contrib` v1.32.4 confirmed resolvable for phase 31.

---

## 9. Section closeout

This brainstorm settles: (Q0) the next §9 subject is `kafka_broker` — the closest of the 3 remaining candidates {redis / kafka_broker / thrift} to the passive-sniffer SHAPE, chosen over the terminal-routing-proxy alternatives; (Q-reconfirm) kafka_broker proceeds WITH the cross-side differential intact despite its three contrib blockers (new `/contrib` dep, contrib-only image, large protocol); (Q-pin-sequencing) the mandatory contrib-image pin-refresh is its OWN **phase 30** (this phase), the project's first reference-image change, per D-3.7; kafka_broker is the later **phase 31** (§9 row); (Q-this-cycle-scope) this cycle fully specs phase 30 and records the phase-31 kafka_broker decomposition as a forward-pointer (§4). Phase 30 is a pure pin-refresh: swap `envoyproxy/envoy:v1.37.2` → `envoyproxy/envoy-contrib:v1.37.2`, re-baseline all 54 fixtures green, supersede ADR-0008 with ADR-0227 — **zero production LoC, zero new go.mod deps, zero new package/stat/fixture/fuzzer/BackendKind**, NO split (~4–6 tasks). The `/contrib` dep defers to phase 31.1 (its first consumer).

The next session authors `docs/envoy-go/phases/30-pin-refresh-envoy-contrib/SPEC.md` (`superpowers:writing-plans` scoped to SPEC authoring), executing the §6 D30-1..D30-6 empirical pins, anchoring the ADR-0227 §Context draft. Per ADR-0106/the project precedent, row 30 registers `in-progress` (flat infra row, no sub-phases) at this BRAINSTORM-DONE commit.
