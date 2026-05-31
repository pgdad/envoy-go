# Phase 27 Brainstorm — `sni_cluster` network filter (`envoy.filters.network.sni_cluster`)

**Status:** brainstorm complete (lifecycle-state 0 → 1). This document captures the design decisions reached during the brainstorm session for phase 27 (`network-filter-sni-cluster`), the **SECOND §9 Network-filters-family phase** (the first new family-row after the phase-26 family-bootstrap parent `network-filter-chain-and-rbac` landed its chain framework + `echo` + `direct_response` + `rbac_network`). Phase 27 lands the family's fourth concrete filter, `sni_cluster`, and introduces the first **connection-scoped upstream-cluster-override seam** between a network read-filter and the terminal `tcp_proxy`.

The next session (lifecycle-state 1 → 2 for phase 27, skill `superpowers:writing-plans` per ADR-0005 scoped to **SPEC authoring** per the established single-phase precedent) authors `docs/envoy-go/phases/27-network-filter-sni-cluster/SPEC.md` based on this brainstorm — that SPEC executes the §10 empirical-pin obligations IN-SESSION against reference Envoy v1.37.2 per ADR-0004 and anchors the ADR-0219 + ADR-0220 §Context drafts.

**Brainstorm session:** worktree `.worktrees/phase-27-network-filter-sni-cluster-brainstorm`, branch `phase-27-network-filter-sni-cluster-brainstorm`, branched from master tip `ec66a87` (`next-prompt.txt: repoint master-tip reference to 7d1b7c6 (actual HEAD)` — the docs-only master-tip-repoint commit). Predecessors on master: `7d1b7c6` (the 26.3 IMPL `last-commit`-fill + next-phase next-prompt rewrite); substantive predecessor `72dc36d` (the phase-26.3 IMPL squash that closed parent phase 26 per the 18/19/22/24/25 ROLLUP precedent).

**Brainstorm mode:** interactive with a live human. The user picked the scope envelope + each major design decision via a 3-question dialogue:

- **Q1 scope / phasing** — `Full upstream parity, single phase` chosen from {Full parity, single phase / Full parity, pre-split / Minimal-reduced scope}. Rationale: `sni_cluster` is meatier than `echo`/`direct_response` (it needs one real framework seam — the cluster-override channel + a `tcp_proxy` per-connection resolution change) but materially lighter than `rbac_network` (~1 framework seam + 1 config-less filter + a contained `tcp_proxy` change). It fits cleanly under the ADR-0045 split-gate as one phase.
- **Q2 cluster-override seam** — `Narrow typed per-connection override` chosen from {Narrow typed override (recommended) / Connection-scoped filter-state primitive / Reuse the ADR-0217 dynamic-metadata bucket}. Rationale: a single per-connection cluster-override carried on the chain runtime — keyed by the upstream-canonical name `envoy.tcp_proxy.cluster` — threaded to the terminal filter, with NO general connection-scoped filter-state primitive built for one consumer. This is the project's entrenched extract-at-Nth-consumer / YAGNI discipline (the `internal/rbac/` engine was extracted only at its SECOND consumer in 26.3; `internal/dynamicmetadata/` storage landed only when rbac became its first connection-scoped writer). Reusing the dynamic-metadata bucket was rejected because Envoy deliberately uses **filter-state (control)** not **dynamic-metadata (observability)** for this override — conflating them would diverge from upstream semantics while still needing the same threading work.
- **Q3 differential fixture arms** — `All three arms cross-side proven` chosen from {All three arms (recommended) / Happy-path + fallback / Happy-path only}. Rationale: full parity is proven end-to-end by cross-side byte-exact comparison of (1) SNI→matching-cluster routing, (2) empty/absent-SNI→configured-cluster fallback, and (3) SNI→unknown-cluster→connection-close.

The §9 family-row registration is per ADR-0106 (a §9 family heading is a conceptual umbrella; each family-child is a FLAT top-level row numbered sequentially — phase 26 was anomalous as a pre-split family-parent; phase 27 is a single flat row, verified against the phase-23 flat-row brainstorm precedent which registered `in-progress` at its BRAINSTORM commit). Decisions that did not require live input are self-answered against `BOOTSTRAP_PROMPT.md`, `MISSION.md`, `ROADMAP.md`, `BEHAVIOR_CONTRACT.md`, and prior ADRs (ADR-0001 through ADR-0218). Two cross-cutting postures (differential-fixture strategy + built-in stat surface) are self-answered per the entrenched §9 precedent (full cross-side byte-exact; upstream-parity stat surface) and flagged in §2.4 + §2.5; the SPEC empirically pins both. Empirical pins requiring scrape evidence against Envoy v1.37.2 are enumerated in §10 and deferred to SPEC-drafting time per the phase 09–26 precedent.

**Document shape:** mirrors `docs/envoy-go/phases/26-network-filter-chain-and-rbac/BRAINSTORM.md` section-for-section, reframed for a single flat phase (no pre-split) with a much smaller surface. Per D-3.4 (context isolation), every load-bearing fact cited here lives on disk in the named files; no "see prior conversation" references appear.

**Authored:** 2026-05-31.

---

## 1. Mission and scope confirmation (27)

ROADMAP row `27 | network-filter-sni-cluster | 26 | in-progress | | …` (added by this brainstorm) is the flat top-level row this brainstorm registers as `in-progress`. Phase 27 is the SECOND concrete phase under the `BOOTSTRAP_PROMPT.md §9` **Network filters family** heading (`ROADMAP.md` `### Network filters family` — a conceptual umbrella, not a row, per ADR-0106). The parent phase-26 phase-done squash `72dc36d` (with `last-commit`-fill + next-prompt rewrite at `7d1b7c6` and the tip repoint at `ec66a87`) is this row's `depends-on` anchor.

The Network filters family lists candidate filters at the `### Network filters family` heading: `redis, mongo, kafka_broker, thrift, zookeeper [scope TBD], echo, direct_response, sni_cluster, rbac network`. Phase 26 landed 3 of these (`echo`, `direct_response`, `rbac network`) + the chain framework + the registry migration. Phase 27 lands **1** more (`sni_cluster`). After phase 27 phase-done, **5** family candidates remain on the roster (`redis`, `mongo`, `kafka_broker`, `thrift`, `zookeeper`), each to be brainstormed as its own future phase when it enters `in-progress`. The chosen branch + directory + Go-package identifiers are aligned per the existing convention: branch `phase-27-network-filter-sni-cluster-brainstorm`, directory `27-network-filter-sni-cluster/` (per ADR-0106 row-id-plus-slug convention), filter package `internal/filter/network/snicluster/` (single-token sub-package per the `echo`/`directresponse` precedent).

Phase 27 is also: (i) the FIRST §9 row to **add a non-terminal read-filter that influences a downstream terminal filter's behavior** — `echo`/`direct_response` are terminal; `rbac_network` is a read-filter that gates (allow/deny/shadow) but does not steer routing. `sni_cluster` is the first read-filter whose entire job is to **publish a control decision (the upstream cluster) that the terminal `tcp_proxy` consumes**. (ii) the FIRST §9 row to **make `tcp_proxy` cluster-selection per-connection** — at master tip `tcp_proxy` resolves its cluster ONCE at boot and stores the resolved `*cluster.Cluster` (`internal/filter/tcpproxy/filter.go`); phase 27 adds a per-connection override-then-fallback resolution path while keeping the no-override path byte-exact. (iii) a ZERO-new-`go.mod`-dependency phase (consistent with the in-house family-bootstrap).

### 1.1 What phase 27 delivers as a self-contained whole (envelope: 1 config-less filter + 1 framework seam, full upstream parity)

Phase 27 lands `sni_cluster` at full upstream parity, in one phase:

1. **The `sni_cluster` filter** (`internal/filter/network/snicluster/`) — `envoy.filters.network.sni_cluster` (type `…filters.network.sni_cluster.v3.SniCluster`, an **empty** proto message). A config-less read-filter following the `echo` template: embeds `network.Marker`, trivial parse (accept empty/any `typed_config`), `SetReadFilterCallbacks`. In `OnNewConnection` it reads `cb.Connection().RequestedServerName()`; if non-empty, it publishes that SNI string **verbatim as the per-connection upstream-cluster-override** and returns `Continue` (no sticky halt — per the `reference_network_read_filter_onnewconnection_halts` memory, `OnNewConnection` must `Continue`). `OnData` is a pass-through `Continue` no-op; `OnDestroy` is a no-op. Registered as the **6th built-in** in `internal/filter/network/builtins.RegisterBuiltins`.

2. **The connection-scoped cluster-override seam** (a small accretion to `internal/filter/network/` — NO new package) — a single per-connection cluster-override (a string) carried on the per-connection `chainRuntime`, keyed by the upstream-canonical name `envoy.tcp_proxy.cluster`. A WRITER accessor is exposed to read-filters via `ReadFilterCallbacks` (a setter); a READER path threads the override to the terminal `TerminalFilter` at dispatch time. The exact writer-API name + the terminal-reader threading mechanism are SPEC-pinned (§10 D-pins).

3. **`tcp_proxy` per-connection cluster resolution** — `internal/filter/tcpproxy/` is changed to retain the `*cluster.Manager` + the resolved default cluster (today it stores only the resolved cluster). At connection time it resolves `override present → cm.Get(override)` (close the downstream connection on miss — Envoy parity) `else → the configured default cluster`. **Back-compat:** no override set → identical to today → every existing `tcp_proxy` fixture (`0000-tcp-echo` / `0001-tcp-proxy-rr` / `0002-tls-tcp` + the 26.x network fixtures) stays byte-exact green (deliberate-break discipline — the existing fixtures are the strongest proof).

4. **The differential fixture(s)** — one NEW cross-side multi-cluster TLS fixture (anticipated `0045-sni-cluster`, multi-listener/multi-SNI shape ≈ the 26.3 `0043` fixture) proving all three arms cross-side byte-exact (§6). Clusters named verbatim after SNI values; distinct per-backend sentinels prove routing.

5. **The completion bundle** — the BEHAVIOR_CONTRACT.md 27 subsection (`### envoy.filters.network.sni_cluster` + the `tcp_proxy` per-connection-resolution amendment + any envoy-go-strict departure records), ADR-0219 + ADR-0220 §Decision/§Consequences bodies landed in place, STATE/ROADMAP phase-done advance (ROADMAP row 27 `in-progress → done`).

### 1.2 What phase 27 does NOT deliver (forward to §8)

See §8. Highlights: the remaining L4 protocol proxies (`redis`/`mongo`/`kafka_broker`/`thrift`/`zookeeper`) — each its own future phase; a GENERAL connection-scoped filter-state primitive (deferred until a second consumer needs it — Q2); weighted-cluster / `cluster_header` style dynamic routing beyond SNI-verbatim (not part of `sni_cluster`); write-filter chain (still deferred per the phase-26 Q4 API-revision allowance).

### 1.3 Phase-done as the SECOND Network-filters-family row landing

Phase 27 lands the family's 4th filter. The remaining family candidate count drops from 6 to **5** post-phase-27 (`redis`, `mongo`, `kafka_broker`, `thrift`, `zookeeper`). The connection-scoped cluster-override seam landed here is the first of the family's routing-control primitives.

### 1.4 ADR-0045 split-by-surface readiness — LOW; single phase per Q1

Per ADR-0045 §6, the split-gate fires when `PLAN.md > ~25 tasks OR > ~1500 LoC estimated`. Phase 27's full surface is well under both gates:

- The `sni_cluster` filter package (config-less parse + `OnNewConnection` SNI→override + no-op `OnData`/`OnDestroy` + registration) is anticipated at ~80–160 LoC.
- The connection-scoped cluster-override seam (the `chainRuntime` field + the `ReadFilterCallbacks` setter + the terminal-reader threading) is anticipated at ~40–100 LoC.
- The `tcp_proxy` per-connection-resolution change (retain manager + default; override-then-fallback at `Handle`; unknown-cluster→close) is anticipated at ~60–140 LoC of mostly-contained change.
- The new cross-side fixture + unit tests are test artifacts, not counted against the LoC gate.
- Anticipated task count ~10–16, net-new LoC ~180–400 — both comfortably under the ADR-0045 gate. Single phase confirmed at BRAINSTORM (the PLAN re-checks the gate at PLAN time per ADR-0045).

### 1.5 Seed-stub alignment + package naming

No seed-stub for `sni_cluster` exists. Phase 27 creates `internal/filter/network/snicluster/` from scratch (filter package, single-token `snicluster`, matching the `echo`/`directresponse` precedent). The `internal/filter/network/` framework package + `internal/filter/tcpproxy/` are modified in place (no package move — minimizes churn).

### 1.6 No prebrainstorm-notes branch

No `phase-27-network-filter-sni-cluster-prebrainstorm-notes` branch exists. Phase 27 starts cleanly from this BRAINSTORM.md.

### 1.7 Phase 27's relationship to prior framework deltas

Phase 27 is a SMALL framework accretion (the connection-scoped cluster-override seam) on top of the phase-26 `internal/filter/network/` chain framework + `TerminalFilter` seam — NOT a new package. It is the first time the framework threads a read-filter-published control value to the terminal filter, complementing the 26.3 connection-scoped dynamic-metadata WRITE (which was observability-only, with no in-repo reader). The override seam carries an implicit API-revision allowance (consistent with the phase-22.1/25.1/26.x pattern) should a second override-publishing filter (or a general connection-scoped filter-state need) appear.

---

## 2. Design decisions (per topic)

The brainstorm dialogue settled 3 Q-decisions + 2 self-answered cross-cutting postures.

### 2.1 Scope / phasing: full upstream parity, single phase *(Q1 → ROADMAP row 27 registered `in-progress`)*

**Decision:** Land `sni_cluster` at full upstream parity (SNI-verbatim→cluster override of `tcp_proxy`, connection lifespan, empty-SNI→fallback, unknown-cluster→close) as ONE flat phase 27. No pre-split.

**Rationale:** Surface (~180–400 net-new LoC, ~10–16 tasks) is well under the ADR-0045 split-gate. The seam + `tcp_proxy` change + the config-less filter are tightly coupled (the seam has no value without both ends) and best landed atomically.

**Anticipated ADRs:** ADR-0219 (the seam + `tcp_proxy` per-connection resolution) + ADR-0220 (the `sni_cluster` filter). See §7.

### 2.2 Cluster-override seam: narrow typed per-connection override *(Q2 → ADR-0219; NO new package)*

**Decision:** Carry a single per-connection cluster-override string on the `chainRuntime`, keyed by the upstream-canonical name `envoy.tcp_proxy.cluster`, exposed to read-filters via a `ReadFilterCallbacks` setter and threaded to the terminal `TerminalFilter` at dispatch time. NO general connection-scoped filter-state primitive. NO reuse of the dynamic-metadata bucket for control.

**Rationale:** Extract-at-Nth-consumer / YAGNI — the project does not build general primitives for one consumer (the `internal/rbac/` engine was extracted only at its second consumer; `internal/dynamicmetadata/` storage landed only at its first writer). Envoy uses filter-state (control), not dynamic-metadata (observability), for the `envoy.tcp_proxy.cluster` override; reusing the metadata bucket would diverge from upstream semantics while still requiring the same terminal-threading work. The narrow override is the minimal, byte-exact-back-compat-preserving shape; it can be generalized into a real connection-scoped filter-state primitive if/when a SECOND override-publishing consumer appears (API-revision allowance).

**Anticipated ADRs:** ADR-0219 §Decision includes the narrow-override shape + the canonical-key naming + the `tcp_proxy` per-connection resolution + the back-compat-via-existing-fixtures discipline + the no-general-primitive / API-revision-allowance clause.

### 2.3 Fixture arms: all three cross-side proven *(Q3 → §6)*

**Decision:** The new cross-side TLS fixture proves all three arms byte-exact against Envoy v1.37.2: (1) SNI matches an existing cluster → routed to that backend; (2) empty/absent SNI → falls back to `tcp_proxy`'s configured cluster; (3) SNI names an unknown cluster → connection close.

**Rationale:** Full parity proven end-to-end. Also organically cross-side-exercises SNI routing — an area the 26.3 `D-26.3-4` boundary left differential-unproven for rbac (a different filter, but the SNI plumbing is shared, so this strengthens confidence in the `RequestedServerName()` path).

### 2.4 Differential-fixture strategy: full cross-side byte-exact *(self-answered per §9 precedent; SPEC pins counts)*

**Decision (self-answered; flagged for SPEC confirmation):** Full cross-side byte-exact differential. SNI→cluster routing, fallback, and unknown-cluster-close are all deterministic on the wire. Per the `reference_differential_fixture_dispatch_constraint` memory, cross-side and boot-reject fixtures are SEPARATE dirs (one fixture dir = one runner branch) — phase 27 has NO boot-reject arm (the filter is config-less; nothing to reject at boot), so a single cross-side dir with multiple listener/SNI arms is anticipated. Per the `reference_differential_asserter_dispatch` memory, any subject-side-only assertion uses `StatsAsserter` (not `SubjectAsserter`) and must be proven live.

**Rationale:** Matches the phase-09→26 full-cross-side discipline.

### 2.5 Built-in stat surface: upstream-parity *(self-answered per §9 precedent; SPEC pins roster)*

**Decision (self-answered; flagged for SPEC confirmation):** `sni_cluster` emits NO built-in stats of its own (upstream parity — the filter has no stats). Project stat surface stays **136**. SPEC empirically pins whether the unknown-override-cluster path touches any EXISTING `tcp_proxy` no-route/cluster-not-found counter already in the surface (and whether that path adds a counter).

**Rationale:** Matches the established §9 stat-surface posture; the config-less filter has no roster to mirror.

### 2.6 No new third-party dependency *(self-answered → no go.mod delta)*

**Decision:** Phase 27 adds NO new third-party `go.mod` dependency. The filter + seam + `tcp_proxy` change are pure in-house Go reusing the `internal/filter/network/` framework + `internal/cluster/` manager + the already-vendored `envoy.extensions.filters.network.sni_cluster.v3` proto binding (go-control-plane v1.32.4).

---

## 3. Framework-survey result — 0 NEW packages + 1 small seam + 0 NEW go.mod deps + REUSES

### 3.1 NEW (small): the connection-scoped cluster-override seam *(per Q2; anchored at ADR-0219; lands in `internal/filter/network/`)*

A single per-connection cluster-override string on the `chainRuntime`, keyed by `envoy.tcp_proxy.cluster`. Writer: a `ReadFilterCallbacks` setter (name SPEC-pinned). Reader: threaded to `TerminalFilter` at dispatch (mechanism SPEC-pinned — candidates: a `chainRuntime`-owned value read at terminal-dispatch time, a per-connection `context.Value`, or a terminal-side callbacks accessor). NOT a new package — a small accretion to the existing framework.

### 3.2 MODIFIED: `tcp_proxy` per-connection cluster resolution *(per Q1 full-parity; anchored at ADR-0219)*

`internal/filter/tcpproxy/` retains the `*cluster.Manager` + the resolved default cluster; at `Handle` time it resolves `override → cm.Get(override)` (close on miss) else the default. Back-compat: no override → byte-exact with today.

### 3.3 REUSES

- `internal/filter/network/` (phase 26.1/26.2) — the read-filter chain framework + `ReadFilterCallbacks` + `Connection.RequestedServerName()` + the `TerminalFilter` seam + `builtins.RegisterBuiltins`.
- `internal/filter/tcpproxy/` (phase 02) — modified in place for per-connection resolution.
- `internal/cluster/` — the cluster manager (`cm.Get(name)`), now consulted per-connection for override resolution.
- `envoy.extensions.filters.network.sni_cluster.v3` proto binding — already vendored via go-control-plane v1.32.4.
- The freeze-after-boot registry discipline (ADR-0059/0072/0079), the two-step factory pattern (ADR-0079), the atomic-landing + six-gate discipline (ADR-0052), the `OnNewConnection`-must-Continue constraint (the `reference_network_read_filter_onnewconnection_halts` memory).

---

## 4. Per-route applicability — network filters have no `typed_per_filter_config` (SPEC confirms)

Network filters in Envoy are configured per filter-chain on the listener, NOT per-route. There is therefore no `typed_per_filter_config` per-route override surface for `sni_cluster` — the ADR-0125 canonical-per-route-shape roster is NOT touched by phase 27. SPEC confirms by absence (the `SniCluster` proto carries no per-route message), consistent with the phase-26 §4 confirmation.

---

## 5. Stat surface hypothesis

`sni_cluster`: 0 built-in stats (upstream parity — the filter has none). Project stat surface stays **136**. SPEC-time empirical pin: whether the unknown-override-cluster path touches an EXISTING `tcp_proxy` no-route / `cluster_not_found` counter already in the surface (and whether mirroring upstream requires adding one). If upstream emits a distinct counter on this path, it lands at IMPL with a BEHAVIOR_CONTRACT note (anticipated +0; SPEC settles).

---

## 6. Differential fixture envelope — one new cross-side multi-cluster TLS dir

### 6.1 The `0045-sni-cluster` cross-side fixture (anticipated)

A multi-cluster TLS fixture (multi-listener/multi-SNI shape ≈ the 26.3 `0043` fixture) proving all three arms byte-exact: (1) SNI `foo.example.com` → cluster `foo.example.com` routed (distinct backend sentinel); (2) empty/absent SNI → the `tcp_proxy` configured fallback cluster; (3) SNI naming an unknown cluster → connection close (zero bytes). Clusters named verbatim after SNI values. TLS required (SNI is populated by tls_inspector — fixture uses TLS like `0002-tls-tcp`). Cross-side byte-exact via the differential runner; any subject-side-only assertion uses `StatsAsserter` per the asserter-dispatch memory.

### 6.2 No boot-reject fixture

The `sni_cluster` proto is config-less — there is nothing to reject at boot. No boot-reject dir (unlike rbac's `0044`). The fixture-dispatch-constraint memory (one dir = one runner branch) is satisfied by a single cross-side dir.

### 6.3 Back-compat

Existing `tcp_proxy` fixtures (`0000`/`0001`/`0002` + 26.x) stay byte-exact green (no `sni_cluster` in chain → no override → static cluster) — the strongest proof the per-connection-resolution change is non-regressive.

### 6.4 Total fixture-dir count

Anticipated +1 (fixtures 46 → **47**). SPEC pins exact dir count + numbering (a single multi-arm cross-side dir is the working hypothesis; SPEC may split arms if a dispatch constraint forces it).

### 6.5 No conformance harness

Phase 27 seeds no new conformance harness; validated by the differential + the existing back-compat fixtures.

---

## 7. Anticipated ADRs — ~2 ADRs (ADR-0219 + ADR-0220)

Next-free ADR at master tip is **ADR-0219** (DECISIONS.md tail ADR-0218).

- **ADR-0219** — the connection-scoped cluster-override seam (the narrow typed per-connection override keyed by `envoy.tcp_proxy.cluster`; the `ReadFilterCallbacks` writer accessor; the terminal-reader threading) + the `tcp_proxy` per-connection cluster resolution (override-then-fallback; unknown-cluster→close; back-compat-via-existing-fixtures) + the no-general-primitive / API-revision-allowance clause.
- **ADR-0220** — the NEW `sni_cluster` filter: config-less parse; `OnNewConnection` SNI-verbatim→override + `Continue`; no-op `OnData`/`OnDestroy`; 6th built-in registration; the empty-SNI→fallback + unknown-cluster→close parity semantics.

### 7.1 Next-free-ADR hypothesis

Anticipated span ADR-0219 .. ADR-0220 (2 ADRs); next-free after phase 27 phase-done ≈ **ADR-0221**. Exact count + numbering pinned at SPEC + IMPL per ADR-0044 (§Context drafts at SPEC; §Decision/§Consequences bodies at IMPL). The ADR-0209 escape-valve reserve carried from the §9 family STANDS-UNCONSUMED.

---

## 8. Deferred items

- **A general connection-scoped filter-state primitive** — deferred per Q2 (YAGNI / extract-at-second-consumer); the narrow override generalizes when a second consumer appears (API-revision allowance).
- **L4 protocol proxies** — `redis`, `mongo`, `kafka_broker`, `thrift`, `zookeeper [scope TBD]` — each a future Network-family phase.
- **Dynamic routing beyond SNI-verbatim** — weighted-cluster / `cluster_header` / metadata-driven cluster selection in `tcp_proxy` are not part of `sni_cluster` (which uses the SNI string verbatim as the cluster name); deferred.
- **Write-filter chain** (`onWrite` / `WriteFilter`) — still deferred per the phase-26 Q4 API-revision allowance.
- **`network-filter-wasm`** — WASM host family; unaffected by phase 27.

---

## 9. Cross-references against prior phases' deferred-items lists — closure pickup

- The phase-26 BRAINSTORM §8 + STATE.md explicitly deferred `sni_cluster` as "a non-terminal read filter that overrides the upstream cluster from the SNI value; needs the 26.1 chain but is a separate future phase (it depends on a connection-level cluster-override seam that rbac does not need)." Phase 27 PICKS THIS UP — the 26.1 read-filter chain + the 26.2 `TerminalFilter` seam are the prerequisites now satisfied, and phase 27 builds exactly the connection-level cluster-override seam phase-26 named as the blocker.
- The 26.3 connection-scoped dynamic-metadata WRITE (ADR-0217) is the structural sibling of phase 27's override seam (both are per-connection read-filter-published values), but distinct: ADR-0217 is observability (no in-repo reader); phase 27's override is control (read by `tcp_proxy`). Q2 explicitly keeps them separate per upstream semantics.
- The `D-26.3-4` coverage boundary (SNI/`requested_server_name` arms unit-tested but not cross-side differential-proven for rbac) is partially de-risked by phase 27's cross-side SNI-routing fixture, which exercises the shared `RequestedServerName()` path end-to-end (though `D-26.3-4` remains formally open for the rbac filter specifically).

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution (empirical pins against Envoy v1.37.2 per ADR-0004)

The SPEC author executes these IN-SESSION against Envoy v1.37.2 source + go-control-plane v1.32.4 bindings:

- **D27-1** — confirm `envoy.extensions.filters.network.sni_cluster.v3.SniCluster` is an empty message (no fields) + its exact `@type` URL (verify via `proto.MessageName`, not the SPEC string, per the `reference_network_filter_typeurl_extensions` memory — network-filter type URLs carry the `extensions.` segment).
- **D27-2** — Envoy's exact SNI→cluster mechanism: confirm `sni_cluster` sets the connection-lifespan filter-state key `envoy.tcp_proxy.cluster` (`PerConnectionCluster`) with the SNI value VERBATIM as the cluster name (no transformation), and that `tcp_proxy` reads that key at upstream-connection time to OVERRIDE its configured cluster.
- **D27-3** *(seam-shaping)* — the writer-API name on `ReadFilterCallbacks` (e.g. `SetUpstreamCluster(name string)` vs a generic per-connection override accessor) + the terminal-reader threading mechanism (chainRuntime-owned value read at terminal-dispatch vs `context.Value` vs a terminal-side callbacks accessor). Settle so the seam is minimal yet faithful to the canonical-key contract.
- **D27-4** — `tcp_proxy` behavior when the override names an unknown / non-existent cluster: confirm Envoy closes the downstream connection (vs falling back to the configured cluster) + whether any stat (`downstream_cx_no_route` / `cluster_not_found` or similar) is touched on that path.
- **D27-5** — `tcp_proxy` behavior when the override names the SAME cluster as configured, and when SNI is empty/absent (confirm fallback to configured cluster) — the exact precedence + the empty-SNI no-op semantics.
- **D27-6** — confirm `sni_cluster` acts in `onNewConnection` (not `onData`) in upstream + returns `Continue` (no halt), so the envoy-go `OnNewConnection`-must-Continue constraint is naturally satisfied.
- **D27-7** — confirm network filters carry no per-route / `typed_per_filter_config` surface (§4).
- **D27-8** — the exact task/LoC envelope (SPEC §3.0) confirming the single-phase fit under the ADR-0045 gate; whether a config-less-parse fuzzer (`FuzzSniClusterConfigParse`, the 37th) is worth adding given the near-trivial parse (working hypothesis: low-value, recommend defer; SPEC decides — fuzzers stay 36 unless added).

---

## 11. Prior-phase lessons applied

- **Differential catches integration bugs unit tests miss** (phase 25.3 / 26.3 REVIEW). Applied: the full cross-side three-arm fixture + the back-compat-via-existing-fixtures gate on the `tcp_proxy` change.
- **`OnNewConnection` must `Continue`** (the `reference_network_read_filter_onnewconnection_halts` memory — an `OnNewConnection` `StopIteration` sets sticky `connHalted`). Applied: `sni_cluster` publishes the override in `OnNewConnection` and returns `Continue`.
- **Network-filter type URLs carry `extensions.`** (the `reference_network_filter_typeurl_extensions` memory). Applied: D27-1 verifies the `@type` via `proto.MessageName`; the proto is blank-imported in bootstrap; the differential bootstrap needs a cluster.
- **Extract-at-Nth-consumer with API-revision allowance** (phase-22.1/25.1/26.3). Applied: the override seam is the narrow form (no general filter-state primitive) with an API-revision allowance for a future second consumer (Q2).
- **Fixture-dispatch + asserter-dispatch constraints** (the `reference_differential_fixture_dispatch_constraint` + `reference_differential_asserter_dispatch` memories). Applied: a single cross-side dir (no boot-reject arm — config-less); any subject-side assertion uses `StatsAsserter` and is proven live.
- **Per-task gofmt + golangci-lint** (the `feedback_pertask_gofmt_lint` memory). Applied: carried into the PLAN's per-task gate.
- **Atomic landing + six gates** (ADR-0052). Applied: phase 27 lands atomically with six-gate evidence; ROADMAP row 27 flips `in-progress → done` at phase-done.

---

## 12. Section closeout

This brainstorm settles: (Q1) `sni_cluster` at full upstream parity as ONE flat phase 27 (no pre-split — well under the ADR-0045 gate); (Q2) a narrow typed per-connection cluster-override carried on the `chainRuntime`, keyed by the upstream-canonical `envoy.tcp_proxy.cluster`, written by `sni_cluster` via a `ReadFilterCallbacks` setter and read by the terminal `tcp_proxy` (no general connection-scoped filter-state primitive; no dynamic-metadata reuse; API-revision allowance); (Q3) all three behavior arms cross-side byte-exact proven. Self-answered per §9 precedent: full cross-side byte-exact differential; upstream-parity (zero) stat surface; zero new third-party deps. ZERO new packages (a small seam in `internal/filter/network/` + a contained `tcp_proxy` per-connection-resolution change + the config-less `internal/filter/network/snicluster/` filter). Anticipated ~2 ADRs (ADR-0219 seam + ADR-0220 filter), fixtures 46 → 47, stat surface 136 (+0), fuzzers 36 (→37 only if SPEC adds a low-value config-less-parse fuzzer).

The next session authors the SPEC (`superpowers:writing-plans` scoped to SPEC authoring per the single-phase precedent), executing the §10 D27-1..D27-8 empirical-pin obligations IN-SESSION against Envoy v1.37.2 per ADR-0004 and anchoring the ADR-0219 + ADR-0220 §Context drafts. Per ADR-0106, ROADMAP row 27 registers `in-progress` at this BRAINSTORM-DONE commit; it flips `in-progress → done` at phase-27 IMPL phase-done.
