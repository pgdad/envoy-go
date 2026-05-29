# Phase 26 Brainstorm — Network read-filter chain framework + `rbac_network` (parent row; FIRST Network-filters-family phase)

**Status:** brainstorm complete (lifecycle-state 0 → 1). This document captures the design decisions reached during the brainstorm session for phase 26 (`network-filter-chain-and-rbac`), the **FIRST concrete phase under `BOOTSTRAP_PROMPT.md` §9's Network filters family** — the first new §9 feature family opened since the HTTP filters family closed at phase 25.3 (2026-05-29). Phase 26 bootstraps the L4 network read-filter chain framework (the network-layer analogue of phase 07.1's HTTP filter framework) and lands the family's first three concrete filters: `echo`, `direct_response`, and `rbac_network`.

The next session (lifecycle-state 1 → 2 for phase 26, skill `superpowers:writing-plans` per ADR-0005 scoped to **parent SPEC authoring** per the phase 22 + phase 24 + phase 25 parent-row precedent) authors `docs/envoy-go/phases/26-network-filter-chain-and-rbac/SPEC.md` based on this brainstorm — that parent SPEC is responsible for formalizing the 3-way split surface-mapping + executing the §10 empirical-pin obligations IN-SESSION against reference Envoy v1.37.2 per ADR-0004. The per-sub-phase SPEC sessions (26.1 / 26.2 / 26.3) follow the parent SPEC; each sub-phase's SPEC lands at its own dedicated session per the phase 22.1 / 25.1 precedent.

**Brainstorm session:** worktree `.worktrees/phase-26-network-filter-chain-and-rbac-brainstorm`, branch `phase-26-network-filter-chain-and-rbac-brainstorm`, branched from master tip `b0007a4` (`next-prompt.txt: repoint master-tip references to 3bda442 (actual HEAD)` — the §9-family-close docs-only repoint commit). Predecessors on master: `3bda442` + `6175672` (the §9-family-close next-prompt rewrite trail); `24c8fe1` (the 25.3 IMPL stage-close STATE/PROGRESS SHA-fill); substantive predecessor `57c7c4d` (the 25.3 IMPL squash that closed the §9 HTTP-filters family per the 18/19/22/24 ROLLUP precedent).

**Brainstorm mode:** interactive with a live human. The user picked the family/filter selection + each major design decision via a 6-question dialogue:

- **Q1 first-phase selection** — `Chain framework + rbac_network` chosen from {Chain framework + rbac_network / Chain framework + echo+direct_response / Terminal protocol proxy (redis) / Minimal registry + echo only}. Rationale: the network family has no read-filter chain framework at master tip; bootstrapping it first (the L4 analogue of HTTP phase 07.1) is the structurally correct opening move, exercised by the first real non-terminal filter.
- **Q2 phasing** — `3-way pre-split now` chosen from {2-way pre-split (framework / filter) / Single phase 26 + PLAN-time split-gate / 3-way pre-split now}.
- **Q3 sub-phase mapping** — `26.1 framework+echo/direct_response → 26.2 migrate tcp_proxy/HCM → 26.3 rbac` chosen from {that / framework+migrate-tcp_proxy first / framework+registry(echo) then migrate+direct_response}.
- **Q4 chain-protocol scope** — `Read-filter chain only (write deferred)` chosen from {Read-filter only / Read + write filter chain}.
- **Q5 rbac engine relationship** — `Extract shared internal/rbac/ engine (HTTP=consumer #1, network=#2)` chosen from {Extract shared engine / Fork separate evaluator / Hybrid share-input-agnostic-matchers-only}.
- **Q6 rbac scope envelope** — `Full upstream parity` chosen from {Full upstream parity / MVP enforce-only}.

The §9 family-entry is per ADR-0106 (a §9 family heading is a conceptual umbrella; the first phase under it registers the parent + sub rows when it enters `in-progress`). Decisions that did not require live input are self-answered against `BOOTSTRAP_PROMPT.md`, `MISSION.md`, `ROADMAP.md`, `BEHAVIOR_CONTRACT.md`, prior ADRs (ADR-0001 through ADR-0212), and the just-shipped §9 HTTP-filters-family artefacts. Two cross-cutting postures (differential-fixture strategy + built-in stat surface) are self-answered per the entrenched §9 precedent (full cross-side byte-exact; upstream-parity + envoy-go-strict extensions) and flagged as such in §2.6 + §2.7; the SPEC empirically pins both. Empirical pins requiring scrape evidence against Envoy v1.37.2 are enumerated in §10 and deferred to parent-SPEC-drafting time per the phase 09–25 precedent.

**Document shape:** mirrors `docs/envoy-go/phases/25-http-filter-wasm/BRAINSTORM.md` section-for-section, reframed for the network-filter-chain-framework scope + the 3-way pre-split + the parent-row design discipline. Phase 26 sits in a structurally important position: it is the **FIRST §9 Network-filters-family row**; it **opens the first new §9 feature family since the HTTP filters family**; it introduces the **NEW `internal/filter/network/` read-filter chain framework primitive** (the L4 analogue of the phase-07.1 `internal/filter/http/` framework — iteration protocol + callbacks + freeze-after-boot registry); it **migrates the two existing terminal network filters (`tcp_proxy` + HCM) onto the new chain** at 26.2, retiring the hardcoded `internal/listener/manager.go` filter registry; and it **extracts the phase-16 RBAC policy engine into a shared `internal/rbac/` primitive** at 26.3 (extract-at-second-consumer per the phase-22.1 `internal/lua` + phase-25.1 `internal/wasm` precedent). NO new third-party `go.mod` dependency (unlike phase-22 gopher-lua + phase-25 wazero) — the whole family-bootstrap is in-house. Per D-3.4 (context isolation), every load-bearing fact cited here lives on disk in the named files; no "see prior conversation" references appear.

**Authored:** 2026-05-29.

---

## 1. Mission and scope confirmation (26 only)

ROADMAP row `26 | network-filter-chain-and-rbac | 25 | in-progress | 26.1, 26.2, 26.3 | …` (added by this brainstorm, see §10 + the ROADMAP edit) is the parent row this brainstorm registers as `in-progress` with sub-phase list `26.1, 26.2, 26.3`. The three sub-rows `26.1 | network-filter-chain-framework-and-echo | 25 | planned | | …`, `26.2 | network-filter-registry-migration | 26.1 | planned | | …`, `26.3 | network-filter-rbac | 26.2 | planned | | …` are also registered by this brainstorm (per the long-prefix slug convention + the pre-create-all-directories convention inherited from the phase-22/25 precedent). Phase 26 is the FIRST concrete phase to enter the `BOOTSTRAP_PROMPT.md` §9 **Network filters family** heading (`ROADMAP.md` line 88 — `### Network filters family` — a conceptual umbrella, not a row, per ADR-0106). The phase 25.3 squash-merge commit `57c7c4d` (with SHA-fill at `24c8fe1`, the §9-family-close next-prompt rewrite at `6175672`, and the tip repoint at `3bda442`/`b0007a4`) is the parent row's `depends-on` anchor.

The Network filters family lists candidate filters at `ROADMAP.md` line 90: `redis, mongo, kafka_broker, thrift, zookeeper [scope TBD], echo, direct_response, sni_cluster, rbac network`. Phase 26 lands **3** of these (`echo`, `direct_response`, `rbac network`) plus the foundational chain framework + the registry migration. After phase 26 phase-done, **6** family candidates remain on the roster (`redis`, `mongo`, `kafka_broker`, `thrift`, `zookeeper`, `sni_cluster`), each to be brainstormed as its own future phase when it enters `in-progress`. The chosen branch + directory + Go-package identifiers are aligned per the existing convention: parent branch `phase-26-network-filter-chain-and-rbac-brainstorm`, parent directory `26-network-filter-chain-and-rbac/` (per ADR-0106 row-id-plus-slug convention).

Phase 26 is also: (i) the FIRST §9 row to **introduce an L4 read-filter chain abstraction** — at master tip the only "network filters" (`tcp_proxy` + HCM) are terminal, selected one-per-chain via a hardcoded `map[string]filterConstructor` in `internal/listener/manager.go` with a private `filterHandler { Handle(ctx, conn) }` interface and NO iteration, NO read/write-filter protocol, NO connection-level callbacks, NO extensible registration. Phase 26.1 establishes the missing chain framework. (ii) the FIRST §9 row to **bootstrap a brand-new filter category's framework** since phase 07.1 bootstrapped the HTTP filter framework — phase 26.1 is the L4 analogue, deliberately mirroring 07.1's design (iteration-status protocol per ADR-0038; freeze-after-boot threaded-constructor registry per ADR-0072/0079; two-step factory pattern per ADR-0079; single-goroutine-per-connection concurrency per the ADR-0071 spirit). (iii) the FIRST §9 row to **retire an existing hardcoded registry** — 26.2 migrates `tcp_proxy` + HCM onto the new extensible network-filter registry and removes the inline `filterRegistry` map from `manager.go`, with back-compat proven by the existing differential fixtures (the 0000-tcp-echo + TLS-TCP fixtures) staying byte-exact green. (iv) the THIRD §9 row to **extract a framework primitive at its Nth consumer** — 26.3 extracts the phase-16 RBAC policy engine (currently HTTP-filter-local in `internal/filter/http/rbac/evaluator.go` + `rbac.go`) into a shared `internal/rbac/` package, migrating HTTP rbac as consumer #1 and adding network rbac as consumer #2 (after the phase-22.1 `internal/lua` first-consumer extraction and the phase-25.1 `internal/wasm` first-consumer extraction, this is the project's first SECOND-consumer-driven extraction with a live first consumer to re-verify). (v) the FIRST §9 family-bootstrap to add **ZERO new third-party `go.mod` dependencies** — contrasts with phase-22 (gopher-lua) + phase-25 (wazero); the entire chain framework + the three filters + the shared RBAC engine are pure in-house Go.

### 1.1 What phase 26 delivers as a self-contained whole (envelope: framework + 3 filters, full upstream parity per filter)

Phase 26 lands the L4 network read-filter chain framework + three `envoy.filters.network.*` filters at full upstream parity, across THREE sub-phases (per Q2 + Q3):

1. **Sub-phase 26.1** (`network-filter-chain-framework-and-echo`) — delivers: (a) the NEW `internal/filter/network/` framework primitive — the read-filter iteration protocol (`OnNewConnection() Status` + `OnData(buffer, endStream) Status` returning `Continue` / `StopIteration`), the `ReadFilterCallbacks` interface (connection accessor + `ContinueReading()` resume), per-connection read-buffering when a filter returns `StopIteration` (the L4 analogue of the HTTP chain's body buffering), and the extensible **freeze-after-boot** network-filter registry (threaded-constructor map, no package-global `init()`, late-`Register` panics; mirrors ADR-0072 + ADR-0079); (b) the two trivial terminal filters `echo` (`envoy.filters.network.echo` — echoes received bytes back to the downstream connection) and `direct_response` (`envoy.filters.network.direct_response` — writes a static configured response then closes) as the first consumers of the chain framework; (c) the boot-time registration of the new registry in `cmd/envoy-go/main.go`, threaded into the listener manager's construction path; (d) `tcp_proxy` + HCM remain wired via their EXISTING `manager.go` path untouched at 26.1 (the migration is 26.2's job — this keeps 26.1's blast radius confined to NEW code); (e) the differential fixtures for echo + direct_response (full cross-side byte-exact per §2.6) + a network-filter boot-reject fixture; (f) the config-parse fuzzer(s) for the two filters; (g) the BEHAVIOR_CONTRACT.md 26.1-completion bundle landed atomically per ADR-0052 (NEW `### Network filter chain framework` + `### envoy.filters.network.echo` + `### envoy.filters.network.direct_response` subsections + any envoy-go-strict departure records + a forward-pointer notes subsection); (h) STATE.md re-advance + ROADMAP row 26.1 flipped `planned → done`.

2. **Sub-phase 26.2** (`network-filter-registry-migration`) — delivers: (a) migration of `tcp_proxy` (`internal/filter/tcpproxy/`) + HCM (`internal/filter/hcm/`) onto the new `internal/filter/network/` read-filter interface + extensible registry; (b) **retirement of the hardcoded `filterRegistry` map + `filterHandler`/`filterConstructor` types in `internal/listener/manager.go`** — the listener manager now resolves the terminal + any preceding read filters through the threaded `*network.Registry`; (c) the read-filter chain dispatch wired into the per-connection lifecycle in `manager.go` (after listener-filter pipeline + chain-match + TLS handshake, the selected chain's `filters[]` are dispatched in order through the read-filter iteration protocol, terminating at `tcp_proxy` or HCM); (d) back-compat proven by the EXISTING differential fixtures (0000-tcp-echo + the TLS-TCP fixtures + any HCM fixtures) staying byte-exact green — NO behavior change for the existing terminal filters; (e) optionally a NEW multi-read-filter chain fixture (e.g. `echo` preceding `tcp_proxy`, proving real-path iteration); (f) the ADR(s) anchoring the migration + the hardcoded-registry retirement + the back-compat discipline. NO new operator-visible filter at 26.2; this is the structural-debt-paydown sub-phase that proves the 26.1 framework on the load-bearing path.

3. **Sub-phase 26.3** (`network-filter-rbac`) — delivers: (a) `rbac_network` (`envoy.extensions.filters.network.rbac.v3.RBAC`) at full upstream parity — enforced rules + shadow rules + connection-level dynamic-metadata emission + the full stat surface; (b) the NEW shared `internal/rbac/` framework primitive — the phase-16 principal/permission evaluation engine extracted from `internal/filter/http/rbac/` and refactored to evaluate against an abstract "matchable context" interface (HTTP request context vs L4 connection context); the HTTP rbac filter migrates onto it as consumer #1 (re-verified by its phase-16 differential fixtures staying green); network rbac is consumer #2; (c) the L4 principal/permission input surface — `direct_remote_ip` / `remote_ip` / destination IP + port / `requested_server_name` (SNI) / downstream-cert principals (`authenticated.principal_name` from the peer cert SANs/subject); HTTP-only principal matchers (header / path / url_path / metadata-on-request) PARSE-REJECT in the L4 context (envoy-go-strict departure if upstream silently no-ops them — SPEC empirically pins); (d) the NEW connection-level **dynamic-metadata sink** primitive (the existing `internal/dynamicmetadata/` + `internal/filterstate/` primitives are HTTP-stream-scoped per ADR-0207; rbac_network's shadow-rule + enforced-denial metadata needs a connection-scoped sink — SPEC settles whether this extends an existing primitive or is a new `internal/connmetadata/`-style package); (e) the rbac stat surface (`allowed` / `denied` / `shadow_allowed` / `shadow_denied` + any envoy-go-strict extension); (f) the differential fixtures for rbac allow + deny + shadow scenarios (full cross-side byte-exact) + a boot-reject fixture; (g) the config-parse fuzzer; (h) the BEHAVIOR_CONTRACT.md 26.3-completion bundle + STATE/ROADMAP rollup (parent row 26 flips `in-progress → done` ATOMICALLY with sub-row 26.3 per the 18/19/22/24/25 ROLLUP precedent).

### 1.2 What phase 26 does NOT deliver (forward to §8)

See §8 for the explicit deferred-items list. Highlights:

- **Write-filter chain** (`onWrite` / `WriteFilter`) — PARSE-REJECT / absent at 26.1 per Q4 (no near-term Network-family filter uses it; the protocol proxies that do are deferred). The read-filter callbacks are shaped with an explicit API-revision allowance for a future write-filter addition (phase-22.1 / phase-25.1 API-revision-allowance precedent).
- **The L4 protocol proxies** — `redis`, `mongo`, `kafka_broker`, `thrift`, `zookeeper` — each a large terminal protocol surface; deferred to future Network-family phases (each its own brainstorm).
- **`sni_cluster`** (`envoy.filters.network.sni_cluster`) — a non-terminal read filter that overrides the upstream cluster from the SNI value; needs the 26.1 chain but is a separate future phase (it depends on a connection-level cluster-override seam that rbac does not need).
- **`network-filter-wasm`** (`envoy.filters.network.wasm`) — lives in the broader §9 WASM host family; consumes the phase-25 `internal/wasm/` primitive at the network layer once the 26.1 read-filter chain exists.
- **Async read-filter resumption across goroutines** beyond the single-goroutine-per-connection model — if a future filter needs cross-goroutine resume (the ADR-0071 amendment pattern for HTTP), that lands when a consumer needs it; 26.1 ships synchronous iteration with `StopIteration`/`ContinueReading` on the connection goroutine.
- **xDS / RTDS-driven dynamic RBAC policy** — PARSE-REJECT at config-load per the project's hot-fetch deferral discipline; rbac_network accepts only static inline policy at 26.3.

### 1.3 Phase-done as the FIRST Network-filters-family row landing

Phase 26 OPENS the Network filters family and lands its first 3 filters + the chain framework across 3 sub-phases. The remaining family candidate count drops from 9 to **6** post-phase-26 (`redis`, `mongo`, `kafka_broker`, `thrift`, `zookeeper`, `sni_cluster`). The §9 family-ordering after the Network filters family (per `BOOTSTRAP_PROMPT.md §9`) is Load balancing → Upstream robustness → HTTP/3 + QUIC → gRPC → xDS/dynamic config → Observability → Runtime + hot restart → WASM host family. The network read-filter chain framework landed here is the foundational primitive every future Network-family filter consumes.

### 1.4 ADR-0045 split-by-surface readiness — HIGH at BRAINSTORM; 3-way pre-split chosen per Q2

Per ADR-0045 §6, the split-gate fires when `PLAN.md > ~25 tasks OR > ~1500 LoC estimated`. Phase 26's full surface is anticipated to exceed BOTH gates substantially:

- The NEW `internal/filter/network/` chain framework (iteration protocol + `ReadFilterCallbacks` + per-connection buffering + extensible registry + the `cmd/envoy-go/main.go` boot-wiring) is anticipated at ~800–1400 LoC (smaller than the HTTP 07.1 framework because read-filter iteration is simpler than the decode/encode bidirectional HTTP chain — no trailers, no dual-direction cursor).
- The two trivial filters (echo + direct_response) are ~150–350 LoC combined.
- The 26.2 migration (re-shaping `tcp_proxy` + HCM onto the read-filter interface + rewiring `manager.go` + retiring the hardcoded registry) is ~400–800 LoC of churn (mostly refactor, not net-new).
- The 26.3 surface (`internal/rbac/` extraction + HTTP-rbac migration + the connection-metadata sink + the L4 rbac filter + its parity surface) is anticipated at ~1200–2200 LoC.
- Per-sub-phase task counts are anticipated at ~12–20 per sub-phase × 3 = ~36–55 tasks total.

Total anticipated LoC ~2500–4750, task count ~36–55 — both above the ADR-0045 split-gate. The user picked the 3-way pre-split at Q2. The split axes are natural and FEATURE-PROGRESSIVE (per Q3): 26.1 = framework + trivial filters; 26.2 = migration/refactor of the existing terminal filters; 26.3 = the real security filter + the shared-engine extraction. Each sub-phase is independently shippable + delivers value: 26.1 ships two usable network filters + the reusable chain; 26.2 pays down the hardcoded-registry debt + proves the framework on the real path; 26.3 ships connection-level RBAC + the shared engine. Each sub-phase is anticipated at ~12–20 tasks — fits cleanly under the ADR-0045 gate per sub-phase.

The 3-way pre-split at BRAINSTORM time is now the project's THIRD occurrence (phase-22 lua + phase-25 wasm were the first two). Per ADR-0106, the parent row registers as `in-progress` with sub-phases listed; each sub-row registers as `planned` until the corresponding sub-phase opens.

### 1.5 Seed-stub alignment + package naming

No seed-stub for network filters exists under `internal/filter/network/` (the directory does not exist at master tip). Phase 26.1 creates `internal/filter/network/` from scratch — the framework package (chain.go + types.go + callbacks.go + registry.go, mirroring the `internal/filter/http/` layout) plus the first filter sub-packages `internal/filter/network/echo/` + `internal/filter/network/directresponse/`. The Go-package identifier for the framework is `network` at `internal/filter/network/` (matches the `http` package at `internal/filter/http/`). Filter sub-packages: `echo`, `directresponse`, `rbac` (single token each). The shared RBAC engine is `rbac` at `internal/rbac/` (matches `internal/jwks/` + `internal/jwt/` + `internal/httpclient/` + `internal/lua/` + `internal/wasm/` + `internal/matcher/` precedent). The existing `tcp_proxy` (`internal/filter/tcpproxy/`) + HCM (`internal/filter/hcm/`) packages stay at their current paths but implement the new read-filter interface after 26.2 (no package move — minimizes churn).

### 1.6 No prebrainstorm-notes branch

No `phase-26-network-filter-chain-and-rbac-prebrainstorm-notes` branch exists. Phase 26 starts cleanly from this BRAINSTORM.md.

### 1.7 Phase 26's relationship to prior framework deltas + framework-delta accretion shape

Phase 26 returns to a framework-delta-GROWTH posture (after phase 25 added `internal/wasm/`). The prior framework-primitive lineage:

- Phase 07.1 — NEW `internal/filter/http/` HTTP filter chain framework (the structural model phase 26.1 mirrors at L4)
- Phase 16 rbac — NEW `internal/matcher/` (+ the HTTP-local RBAC engine that 26.3 now extracts)
- Phase 17 jwt_authn — NEW `internal/jwks/` + `internal/jwt/`
- Phase 18 ext_authz — NEW `internal/grpcclient/`
- Phase 20 oauth2 — NEW `internal/httpclient/` + `internal/sdsfile/`
- Phase 22.1 lua — NEW `internal/lua/` at first consumer (EXTRACT-NOW discipline)
- Phase 22.2 lua — NEW `internal/dynamicmetadata/` at first co-consumer
- Phase 25.1 wasm — NEW `internal/wasm/` at first consumer
- **Phase 26.1 — NEW `internal/filter/network/` read-filter chain framework** (first new filter-category framework since 07.1); **Phase 26.3 — NEW `internal/rbac/` shared engine (extract-at-SECOND-consumer)** + a NEW connection-level dynamic-metadata sink primitive.

The 26.3 `internal/rbac/` extraction is structurally distinct from the 22.1/25.1 first-consumer extractions: it extracts an engine that has a LIVE first consumer (the phase-16 HTTP rbac filter) which must be migrated + re-verified in the same sub-phase. This carries a different risk profile (migration regression risk, bounded by the phase-16 differential fixtures) than the speculative-future-consumer risk of the 22.1/25.1 extractions. The 26.3 ADR(s) anchor the abstraction shape WITH AN EXPLICIT API-REVISION ALLOWANCE clause (mirrors phase-22.1 ADR-0188 + phase-25.1 ADR-0202) in case a third consumer (e.g. a future RBAC-bearing filter) needs interface revision.

---

## 2. Design decisions (per topic; each cites BRAINSTORM-style rationale + consequences anchor)

The brainstorm dialogue settled 6 Q-decisions + 2 self-answered cross-cutting postures. Each is anchored here with rationale + the anticipated ADR or REUSE classification.

### 2.1 First-phase selection: chain framework + rbac_network *(Q1 → phase 26 opens the Network family)*

**Decision:** Open the Network filters family with phase 26 = the L4 read-filter chain framework + `rbac_network` as the first real non-terminal consumer (plus `echo` + `direct_response` as the trivial first consumers that exercise the framework at 26.1).

**Rationale:** The network family has no read-filter chain framework at master tip — `tcp_proxy` + HCM are terminal-only via a hardcoded registry. Most interesting network filters (`rbac`, `sni_cluster`, and even the protocol proxies in a real chain) assume a read-filter chain. Bootstrapping the framework first — the L4 analogue of HTTP phase 07.1 — is the structurally correct opening move, and `rbac_network` is the natural first non-terminal filter to prove it (security-relevant, reuses the phase-16 RBAC policy engine, default-aware). A terminal protocol proxy (redis) would front-load a large protocol surface while leaving the framework — and `sni_cluster`/`rbac` — blocked.

**Anticipated ADRs:** ADR-0213 (NEW `internal/filter/network/` read-filter chain framework + iteration protocol + callbacks; anchored at 26.1 IMPL); see §7 for the full anticipated roster.

### 2.2 Phasing: 3-way pre-split at BRAINSTORM time *(Q2+Q3 → ROADMAP rows 26 + 26.1 + 26.2 + 26.3 registered; FEATURE-PROGRESSIVE axis)*

**Decision:** Pre-split phase 26 into three sub-phases at BRAINSTORM time. Boundaries follow natural FEATURE-PROGRESSIVE axes: 26.1 = framework + echo + direct_response; 26.2 = migrate tcp_proxy/HCM onto the new registry/chain + retire the hardcoded registry; 26.3 = rbac_network + the shared `internal/rbac/` extraction + the connection-metadata sink. Each sub-phase independently shippable. ROADMAP registers parent row 26 `in-progress` + sub-rows 26.1 / 26.2 / 26.3 `planned` per ADR-0106.

**Rationale:** Full surface (framework + migration + a parity security filter) substantially exceeds the ADR-0045 split-gate (~2500–4750 LoC + ~36–55 tasks). The FEATURE-PROGRESSIVE split axes are unambiguous at BRAINSTORM design-decision time. Third project occurrence of the BRAINSTORM-time 3-way pre-split (phase-22 + phase-25 were the first two).

**Anticipated ADRs:** ROADMAP row registrations (4 new rows). No new ADR specifically for the split decision — ADR-0045 §6 + the phase-22/25 BRAINSTORM-time-split precedent cover it.

### 2.3 Chain-protocol scope: read-filter chain only; write-filter deferred *(Q4 → ADR-0213 anchor + API-revision-allowance clause)*

**Decision:** Build only the read-filter iteration protocol at 26.1 (`OnNewConnection` / `OnData` → `Continue` / `StopIteration` + `ReadFilterCallbacks` + per-connection read-buffering on `StopIteration`). The write-filter (`onWrite` / `WriteFilter`) interface is absent / PARSE-REJECTed until a consumer needs it. The `ReadFilterCallbacks` interface carries an explicit API-revision allowance for a future write-filter addition.

**Rationale:** Every filter on the near-term Network-family roster (`echo`, `direct_response`, `tcp_proxy`, `rbac`, `sni_cluster`) is a read filter; write filters (`onWrite`) appear only in a few protocol proxies that are deferred. Building unexercised write-filter plumbing would violate the project's deliberate-break / differential discipline (every surface should be exercised by a live fixture). YAGNI + smaller 26.1 surface.

**Anticipated ADRs:** ADR-0213 §Decision includes the read-filter-only scope + the write-filter deferral + the API-revision-allowance clause.

### 2.4 Chain framework shape: mirror the phase-07.1 HTTP framework discipline at L4 *(Q1 → ADR-0213 + ADR-0214)*

**Decision:** Mirror the established HTTP-filter framework design at L4: (i) an iteration-status protocol (`Continue` / `StopIteration`) per the ADR-0038 HTTP iteration precedent; (ii) a freeze-after-boot threaded-constructor registry (no package-global `init()`; late-`Register` panics) per ADR-0072 (HTTPRegistry) + ADR-0079 (listener-filter registry) + ADR-0059 (the stats-registry LBP-1 root); (iii) a two-step factory pattern (parse `typed_config` once at boot → per-connection instance factory) per ADR-0079; (iv) single-goroutine-per-connection concurrency (consistent with the existing `Handle(ctx, conn)` model + the ADR-0071 single-goroutine-per-stream spirit); (v) per-connection read buffering when a filter returns `StopIteration` (the L4 analogue of the HTTP chain's body buffering, capacity pinned by SPEC). The registry is allocated at boot in `cmd/envoy-go/main.go` + threaded into the listener-manager construction path.

**Rationale:** The HTTP framework is the project's proven filter-chain design; reusing its disciplines (freeze-after-boot, threaded constructor, two-step factory, iteration status) gives the network framework the same correctness + grep-discoverability properties for free, and keeps the two frameworks structurally parallel for future maintainers. Read-filter iteration is strictly simpler than the HTTP decode/encode chain (single direction, no trailers, no dual cursor).

**Anticipated ADRs:** ADR-0213 (iteration protocol + callbacks + buffering); ADR-0214 (the extensible registry + boot-wiring + the 26.2 hardcoded-registry retirement plan).

### 2.5 rbac engine relationship: extract shared `internal/rbac/` (HTTP = consumer #1, network = #2) *(Q5 → ADR-0216 + API-revision-allowance)*

**Decision:** Extract the phase-16 RBAC principal/permission evaluation engine (currently HTTP-filter-local in `internal/filter/http/rbac/evaluator.go` + `rbac.go`) into a shared `internal/rbac/` primitive evaluating against an abstract "matchable context" interface. Migrate the HTTP rbac filter onto it as consumer #1 (re-verified by its phase-16 differential fixtures); network rbac is consumer #2. The shared engine consumes the `envoy.config.rbac.v3.RBAC` policy proto for both. Carries an API-revision-allowance ADR (phase-22.1 / 25.1 pattern).

**Rationale:** Both the HTTP rbac and network rbac filters share the same `envoy.config.rbac.v3.RBAC` policy proto; only the input surface differs (HTTP request context: headers/path/method/auth vs L4 connection context: source/dest IP+port, SNI, peer-cert principals). Forking a second evaluator would duplicate the principal/permission/matcher evaluation logic (two engines that drift). Extracting follows the project's DRY + extract-at-Nth-consumer discipline (internal/lua, internal/httpclient, internal/grpcclient). The migration cost (touching the live phase-16 HTTP rbac filter) is bounded + de-risked by the phase-16 differential fixtures, which re-prove HTTP rbac byte-exact after the migration.

**Anticipated ADRs:** ADR-0216 (the `internal/rbac/` extraction + the abstract-context interface + the consumer #1 migration + the API-revision-allowance clause).

### 2.6 Differential-fixture strategy: full cross-side byte-exact *(self-answered per §9 precedent; SPEC pins counts)*

**Decision (self-answered per entrenched §9 precedent; flagged for SPEC confirmation):** Full cross-side byte-exact differential fixtures for all three filters. echo (echoes bytes), direct_response (static response + close), and rbac (allow → passthrough / deny → connection close / shadow → metadata-only) are all deterministic on the wire, so full cross-side byte-exact comparison against reference Envoy v1.37.2 is feasible. Per the `reference_differential_fixture_dispatch_constraint` project memory, cross-side and boot-reject fixtures are SEPARATE fixture directories (one fixture dir = one runner branch). Per the `reference_differential_asserter_dispatch` memory, any subject-side-only assertion must use `StatsAsserter` (not `SubjectAsserter`) and must be proven live.

**Rationale:** Matches the phase-09→25 full-cross-side discipline. The L4 filters' determinism removes the wazero-vs-V8-style divergence risk that complicated phase-25's fixtures. The 26.2 migration is validated by the EXISTING fixtures staying green (no new behavior), which is the strongest possible back-compat proof.

**Anticipated fixture envelope (SPEC pins exact counts):** 26.1 → echo cross-side dir + direct_response cross-side dir + a network-filter boot-reject dir (+3); 26.2 → existing fixtures prove back-compat, optionally +1 multi-read-filter-chain fixture (echo-before-tcp_proxy); 26.3 → rbac cross-side dir(s) (allow/deny/shadow) + a boot-reject dir (+2). Family total ≈ +5–6 (fixtures 41 → ~46–47).

### 2.7 Built-in stat surface: upstream-parity + envoy-go-strict extensions *(self-answered per §9 precedent; SPEC pins roster)*

**Decision (self-answered per entrenched §9 precedent; flagged for SPEC confirmation):** Upstream-parity built-in stats + any envoy-go-strict extensions, per the phase-09→25 precedent. `echo` + `direct_response` carry minimal-to-zero built-in stats (upstream echo has none; direct_response minimal). `rbac_network` carries the upstream parity roster `allowed` / `denied` / `shadow_allowed` / `shadow_denied` (SPEC empirically pins the exact names + prefix + any envoy-go-strict extension).

**Rationale:** Matches the established §9 stat-surface posture. SPEC empirically pins the network-rbac stat roster + prefix against Envoy v1.37.2.

**Anticipated stat delta (SPEC pins):** +0 at 26.1 (or minimal direct_response), +0 at 26.2, +4–6 at 26.3 (rbac roster). Project stat surface 132 → ~136–138.

### 2.8 No new third-party dependency *(self-answered → no go.mod delta)*

**Decision:** Phase 26 adds NO new third-party `go.mod` dependency. The chain framework, the three filters, the shared RBAC engine, and the connection-metadata sink are all pure in-house Go reusing existing primitives (`internal/matcher/`, the `envoy.config.rbac.v3` + `envoy.extensions.filters.network.*` proto bindings already vendored via go-control-plane).

**Rationale:** Contrasts with phase-22 (gopher-lua) + phase-25 (wazero). Nothing in the network-filter-chain + RBAC surface requires a VM or external library; the RBAC policy proto + matcher are already available. Keeps the family-bootstrap dependency-free.

---

## 3. Framework-survey result — 2 NEW package-level primitives + 0 NEW go.mod deps + REUSES

### 3.1 NEW: `internal/filter/network/` read-filter chain framework *(per Q1+Q4; anchored at ADR-0213 + ADR-0214; lands at 26.1)*

The L4 analogue of `internal/filter/http/`. Contents (anticipated; SPEC formalizes): `types.go` (the `ReadFilter` interface + `Status` enum `Continue`/`StopIteration`); `callbacks.go` (`ReadFilterCallbacks` — connection accessor + `ContinueReading()`); `chain.go` (the per-connection read-filter chain state machine — sequential dispatch + per-connection read buffering on `StopIteration`); `registry.go` (the freeze-after-boot threaded-constructor `*network.Registry` mirroring `*filter.HTTPRegistry` per ADR-0072). Boot-wired in `cmd/envoy-go/main.go` + threaded into the listener-manager construction path.

### 3.2 NEW: `internal/rbac/` shared RBAC policy engine *(per Q5; anchored at ADR-0216; lands at 26.3, extract-at-second-consumer)*

The phase-16 principal/permission evaluation engine extracted from `internal/filter/http/rbac/` and refactored to evaluate against an abstract matchable-context interface. HTTP rbac migrates as consumer #1; network rbac is consumer #2. Carries an API-revision-allowance clause. (SPEC settles the abstract-context interface shape + which phase-16 internals move vs stay.)

### 3.3 NEW (smaller): connection-level dynamic-metadata sink *(per Q6 full-parity; lands at 26.3)*

The existing `internal/dynamicmetadata/` (phase-22.2) + `internal/filterstate/` (phase-25.2) primitives are HTTP-stream-scoped per ADR-0207. rbac_network's shadow-rule + enforced-denial metadata needs a connection-scoped sink. SPEC settles whether this extends an existing primitive (a connection-scoped bucket alongside the per-stream ones) or is a new small package. The 26.1 `ReadFilterCallbacks` interface is shaped to accommodate it (the storage lands at 26.3 when rbac is its first consumer — YAGNI). **This is a SPEC-BLOCKING pin, not an IMPL-time one** (§10 D4): because the 26.1 callbacks interface must be shaped to accommodate the sink and ADR-0217 anchors it, the extend-vs-new-package decision must be settled at parent-SPEC time so the 26.1 callbacks API does not need post-hoc revision when 26.3 lands the storage.

### 3.4 REUSES

- `internal/matcher/` (phase 16) — the generic matcher primitive, consumed by the shared RBAC engine.
- `internal/listener/` (phases 02/07.2) — the listener manager + chain-match + listener-filter pipeline; 26.2 rewires its per-connection dispatch onto the new read-filter chain.
- `internal/filter/tcpproxy/` + `internal/filter/hcm/` (phases 02/04/05) — migrated onto the read-filter interface at 26.2 (no package move).
- `envoy.config.rbac.v3` + `envoy.extensions.filters.network.{echo,direct_response,rbac}.v3` proto bindings — already vendored via go-control-plane v1.32.4.
- The freeze-after-boot registry discipline (ADR-0059/0072/0079), the two-step factory pattern (ADR-0079), the iteration-status protocol (ADR-0038), and the atomic-landing + six-gate discipline (ADR-0052).

---

## 4. Per-route applicability — network filters have no `typed_per_filter_config` (SPEC confirms)

Network filters in Envoy are configured per filter-chain on the listener, NOT per-route (routes are an HTTP/HCM concept). There is therefore no `typed_per_filter_config` per-route override surface for `echo` / `direct_response` / `rbac_network` — the ADR-0125 canonical-per-route-shape roster is NOT touched by phase 26. SPEC confirms this by absence (the network-filter protos carry no per-route message) — analogous to the phase-23/24.1 REUSE-by-absence confirmation but for the simpler reason that network filters are not route-scoped at all.

---

## 5. Stat surface hypothesis

### 5.1 26.1 + 26.2 (BRAINSTORM hypothesis; SPEC-time empirical pin)

`echo`: 0 built-in stats (upstream parity). `direct_response`: 0–minimal. 26.2 migration: 0 net-new (existing terminal-filter stats unchanged). Project stat surface stays 132 across 26.1 + 26.2.

### 5.2 26.3 rbac counter roster (BRAINSTORM hypothesis; SPEC settles)

Upstream `envoy.extensions.filters.network.rbac.v3.RBAC` stats (anticipated): `allowed`, `denied`, `shadow_allowed`, `shadow_denied` (prefix per SPEC empirical pin). Possible envoy-go-strict extension counters per the §9 precedent (SPEC decides). Anticipated +4–6.

### 5.3 Project stat count delta

132 → ~136–138 at family-done (all at 26.3). SPEC pins.

### 5.4 envoy-go-strict departure flags (anticipated; SPEC + IMPL pin)

L4-inapplicable HTTP principal matchers (header/path/url_path) PARSE-REJECT in the network-rbac context (envoy-go-strict if upstream silently no-ops them); xDS/RTDS-driven dynamic policy PARSE-REJECT; write-filter absent. Each lands a BEHAVIOR_CONTRACT departure record at the relevant sub-phase IMPL.

---

## 6. Differential fixture envelope — anticipated five-to-six directories across 3 sub-phases

### 6.1 26.1 fixtures

`echo` cross-side dir + `direct_response` cross-side dir + a network-filter boot-reject dir (PGV-mirror reject for malformed echo/direct_response config where upstream also rejects). Anticipated +3.

### 6.2 26.2 fixtures

Back-compat proven by the EXISTING fixtures (0000-tcp-echo + TLS-TCP + HCM fixtures) staying byte-exact green — the strongest migration proof. Optionally +1 NEW multi-read-filter-chain fixture (e.g. `echo` preceding `tcp_proxy`) proving real-path iteration. Anticipated +0–1.

### 6.3 26.3 fixtures

`rbac_network` cross-side dir(s) — allow (passthrough) + deny (connection close) + shadow (metadata-only, no enforcement) scenarios — + a boot-reject dir (dangling/malformed policy). Per the fixture-dispatch-constraint memory, cross-side vs boot-reject are separate dirs; the allow/deny/shadow scenarios may share one cross-side dir or split. Anticipated +2.

### 6.4 Total fixture-dir count

Family total ≈ +5–6 (fixtures 41 → ~46–47). SPEC pins exact dir count + numbering.

### 6.5 No conformance harness

Unlike phase 25 (proxy-wasm conformance), phase 26 seeds no new conformance harness. The L4 filters are validated by the differential + the existing back-compat fixtures.

### 6.6 Listener topology

The differential fixtures place the network filters in a listener filter chain (the new read-filter chain), with `tcp_proxy` as the terminal filter for echo/rbac passthrough cases and direct_response as terminal for its own case. SPEC settles the exact bootstrap topology per fixture.

---

## 7. Anticipated ADRs — ~5–8 ADRs across the 3 sub-phases (ADR-0213 .. ~ADR-0219)

Next-free ADR at master tip is **ADR-0213** (DECISIONS.md tail ADR-0212).

The numbering below is a CLEAN non-overlapping anticipation (ADR-0213 .. ADR-0218 = 6 ADRs); the SPEC author locks the exact numbers per ADR-0044, but no two sub-phases float onto the same number — in particular the 26.2 migration ADR (ADR-0215) and the 26.3 `internal/rbac/` extraction ADR (ADR-0216) are distinct.

### 7.1 26.1 anticipated ADRs (ADR-0213 + ADR-0214)

- **ADR-0213** — NEW `internal/filter/network/` read-filter chain framework: iteration-status protocol (`Continue`/`StopIteration`), `ReadFilterCallbacks`, per-connection read buffering, single-goroutine-per-connection concurrency; read-filter-only scope + write-filter deferral + API-revision-allowance clause. The echo + direct_response filter packages + their fixture/stat discipline fold into ADR-0213/0214 (no separate ADR).
- **ADR-0214** — the extensible freeze-after-boot network-filter registry (threaded-constructor, mirrors ADR-0072/0079) + the boot-wiring in `cmd/envoy-go/main.go` + the planned 26.2 hardcoded-registry retirement.

### 7.2 26.2 anticipated ADRs (ADR-0215)

- **ADR-0215** — the `tcp_proxy` + HCM migration onto the read-filter interface + the `manager.go` hardcoded-registry retirement + the back-compat-via-existing-fixtures discipline.

### 7.3 26.3 anticipated ADRs (ADR-0216 .. ADR-0218)

- **ADR-0216** — NEW `internal/rbac/` shared engine extraction (abstract-context interface + consumer #1 HTTP-rbac migration + API-revision-allowance).
- **ADR-0217** — the connection-level dynamic-metadata sink primitive.
- **ADR-0218** — the `rbac_network` filter: L4 principal/permission input surface + HTTP-only-matcher PARSE-REJECT + the stat roster + the full-parity (enforced + shadow) semantics.

### 7.4 Next-free-ADR hypothesis

Anticipated family ADR span ADR-0213 .. ~ADR-0218 (≈6 ADRs); next-free after phase 26 phase-done ≈ **ADR-0219**. Exact count + numbering pinned at SPEC + per-sub-phase IMPL per ADR-0044 (§Context drafts at SPEC; §Decision/§Consequences bodies at IMPL).

---

## 8. Deferred items

- **Write-filter chain** (`onWrite` / `WriteFilter`) — deferred per Q4; API-revision allowance in the 26.1 callbacks.
- **L4 protocol proxies** — `redis`, `mongo`, `kafka_broker`, `thrift`, `zookeeper [scope TBD]` — each a future Network-family phase.
- **`sni_cluster`** — non-terminal cluster-override read filter; future Network-family phase (needs a connection-level cluster-override seam beyond what rbac needs).
- **`network-filter-wasm`** (`envoy.filters.network.wasm`) — WASM host family; consumes both the 26.1 read-filter chain + the phase-25 `internal/wasm/` primitive.
- **xDS / RTDS-driven dynamic RBAC policy** — PARSE-REJECT; Runtime/xDS family.
- **Cross-goroutine async read-filter resume** — beyond single-goroutine-per-connection; lands when a consumer needs it.
- **Per-route network-filter config** — does not exist in Envoy (network filters are chain-scoped, not route-scoped); §4.

---

## 9. Cross-references against prior phases' deferred-items lists — closure pickup

- Phase 25's deferred `network-filter-wasm` (the §9 WASM-host-family consumer #4 noted in the phase-25 BRAINSTORM §1.2 + STATE.md) is NOT picked up here — it is deferred to the WASM host family, but its eventual landing now has a prerequisite satisfied by phase 26.1 (the read-filter chain it will register on).
- The phase-02 tcp_proxy + phase-07.2 listener-chain-completion artefacts are the structural predecessors 26.2 builds on (the listener manager's per-connection dispatch path + chain-match + listener-filter pipeline). 26.2 rewires the post-handshake dispatch from the hardcoded single-`filterHandler` call into the new read-filter chain.
- The phase-16 RBAC engine + `internal/matcher/` are the direct reuse target of 26.3.

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution (empirical pins against Envoy v1.37.2 per ADR-0004)

The parent SPEC author executes these IN-SESSION (parallel-subagent fan-out per the phase-25 SPEC precedent) against Envoy v1.37.2 source + go-control-plane v1.32.4 bindings:

- **D1** — exact field rosters + defaults for `envoy.filters.network.echo` (likely empty config), `envoy.filters.network.direct_response.v3.DirectResponse` (the `response` DataSource field), and `envoy.extensions.filters.network.rbac.v3.RBAC` (rules / shadow_rules / stat_prefix / enforcement_type / shadow_rules_stat_prefix / matcher fields).
- **D2** — network-rbac stat roster + prefix (`allowed`/`denied`/`shadow_allowed`/`shadow_denied` confirmation + exact prefix shape).
- **D3** — the L4 principal/permission matcher subset that applies at the connection layer (which `envoy.config.rbac.v3.Principal` + `Permission` variants are evaluable with only connection context) + upstream behavior for HTTP-only matchers in a network-rbac config (silent no-op vs reject).
- **D4** *(SPEC-BLOCKING)* — connection-level dynamic-metadata namespace + shape emitted by upstream network-rbac (shadow-rule effective-policy-id key + the enforced-denial metadata) **AND** the connection-metadata-sink primitive shape (extend `internal/dynamicmetadata/` with a connection-scoped bucket vs a new small package — §3.3). Must be settled at parent-SPEC time because the 26.1 `ReadFilterCallbacks` interface is shaped to accommodate it and ADR-0217 anchors it; deferring to IMPL would risk a post-hoc 26.1 callbacks-API revision.
- **D5** — Envoy's network filter manager read-filter iteration + buffering semantics (onNewConnection/onData ordering, ContinueReading resume, StopIteration buffering behavior) — to mirror the iteration contract byte-faithfully.
- **D6** — confirmation that network filters carry no per-route / `typed_per_filter_config` surface (§4).
- **D7** — direct_response close semantics (does it half-close / full-close; any configurable delay) + echo's exact byte-pump semantics (does it terminate the chain).
- **D8** — the exact per-sub-phase task/LoC envelope (parent SPEC §3.0) confirming each sub-phase fits the ADR-0045 gate.

---

## 11. Prior-phase lessons applied

- **Differential catches integration bugs unit tests miss** (phase 25.3 REVIEW: 4 production bugs surfaced by the differential). Applied: full cross-side byte-exact fixtures at 26.1 + 26.3, and the 26.2 migration is gated on the EXISTING fixtures staying green — a deliberate-break-discipline migration.
- **Extract-at-Nth-consumer with API-revision allowance** (phase-22.1 internal/lua ADR-0188; phase-25.1 internal/wasm ADR-0202). Applied: the 26.3 `internal/rbac/` extraction carries the same API-revision-allowance clause, with the added discipline of re-verifying the LIVE first consumer (HTTP rbac) via its phase-16 fixtures.
- **Freeze-after-boot threaded-constructor registry** (ADR-0059/0072/0079). Applied: the 26.1 network-filter registry mirrors it exactly — no package-global `init()`, late-`Register` panics, lock-free post-Freeze lookup.
- **Fixture-dispatch constraints** (project memories `reference_differential_fixture_dispatch_constraint` + `reference_differential_asserter_dispatch`). Applied: cross-side vs boot-reject as separate dirs; any subject-side assertion uses `StatsAsserter` and is proven live.
- **Atomic landing + six gates + ROLLUP** (ADR-0052/0106). Applied: each sub-phase lands atomically with six-gate evidence; the parent row 26 flips `in-progress → done` ATOMICALLY with sub-row 26.3 at family-close per the 18/19/22/24/25 precedent.

---

## 12. Section closeout

This brainstorm settles: (Q1) phase 26 opens the Network filters family with the L4 read-filter chain framework + rbac_network; (Q2/Q3) 3-way FEATURE-PROGRESSIVE pre-split — 26.1 framework + echo + direct_response, 26.2 migrate tcp_proxy/HCM + retire the hardcoded registry, 26.3 rbac_network + shared `internal/rbac/` extraction + connection-metadata sink; (Q4) read-filter-only chain (write-filter deferred with API-revision allowance); (Q5) extract the shared RBAC engine (HTTP = consumer #1 migrated + re-verified, network = #2); (Q6) full upstream parity for rbac_network (enforced + shadow + connection dynamic-metadata). Self-answered per §9 precedent: full cross-side byte-exact fixtures; upstream-parity + envoy-go-strict stat surface; zero new third-party deps. Two NEW framework primitives (`internal/filter/network/` at 26.1; `internal/rbac/` at 26.3) + a small connection-metadata sink. Anticipated ~5–8 ADRs (ADR-0213 .. ~ADR-0218), fixtures 41 → ~46–47, stat surface 132 → ~136–138, fuzzers 35 → ~37–38.

The next session authors the parent SPEC (`superpowers:writing-plans` scoped to parent-SPEC authoring per the phase-22/24/25 parent-row precedent), executing the §10 D1–D8 empirical-pin obligations IN-SESSION against Envoy v1.37.2 per ADR-0004, anchoring the ADR-0213.. §Context drafts, and formalizing the 3-way split surface-mapping + per-sub-phase scope boundaries. Per ADR-0106, the parent row 26 registers `in-progress` with sub-phases listed at this BRAINSTORM-DONE commit; sub-rows 26.1 + 26.2 + 26.3 register `planned`.
