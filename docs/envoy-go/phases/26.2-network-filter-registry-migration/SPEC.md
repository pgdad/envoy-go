# Phase 26.2 SPEC — `tcp_proxy` + HCM migration onto `internal/filter/network/` + hardcoded-registry retirement + dispatch unification

> **For agentic workers:** this is the per-sub-phase SPEC for **phase 26.2** (`network-filter-registry-migration`), the SECOND sub-phase of the phase-26 BRAINSTORM-time 3-way pre-split (26.1 / 26.2 / 26.3). It is authored per the phase-22.1 / phase-25.1 / phase-26.1 per-sub-phase-SPEC precedent: the **parent SPEC** (`docs/envoy-go/phases/26-network-filter-chain-and-rbac/SPEC.md`) resolved the BRAINSTORM §10 D1–D8 empirical pins (parent §11), formalized the 3-way split surface-mapping (parent §3), and anchored the ADR-0213..0218 §Context drafts; the **26.1 SPEC/PLAN/IMPL** landed the NEW `internal/filter/network/` read-filter chain framework + `echo` + `direct_response` + the `manager.go` **dual-dispatch** (new read-filter chain path alongside the UNTOUCHED `tcp_proxy`/HCM terminal-filter path). This 26.2 SPEC **INHERITS** the parent SPEC's §5 proto roster + §6 PARSE-REJECT roster + §7 stat surface + §8 fixture taxonomy + §11 empirical-pin block + §12 D-questions + §13 RATIFIED-PENDING items, and **refines per-Task-level surface only**. It does NOT re-execute the parent empirical pins. It **drafts ADR-0215 §Context** into DECISIONS.md (the registry-migration ADR; §Decision/§Consequences land at 26.2 IMPL per ADR-0044). The next session, per BOOTSTRAP §5, authors the **26.2 PLAN** (bite-sized TDD tasks) from this SPEC.

**Goal:** Migrate the two existing terminal network filters — `tcp_proxy` (`internal/filter/tcpproxy/`) + HCM (`internal/filter/hcm/`) — onto the `internal/filter/network/` framework + the freeze-after-boot `*network.Registry` that 26.1 landed, RETIRE the hardcoded `internal/listener/manager.go` terminal-filter registry (`filterRegistry` / `filterConstructor` / `filterHandler` / `buildTerminalFilter`), and UNIFY the 26.1 dual-dispatch into a single registry-driven per-connection dispatch path — at byte-exact parity (R3 back-compat is paramount: the existing `tcp_proxy`/HCM differential fixtures + conformance + h2spec stay byte-exact green).

**Architecture:** 26.1 left the `manager.go` terminal-filter path UNTOUCHED alongside the new read-filter chain path (the dual-dispatch), with a transitional `network-filter-mixed-chain-unsupported` boot-reject explicitly "lifted at 26.2". 26.2 collapses the dual-dispatch by extending the framework with a **terminal-filter seam**: a NEW `network.TerminalFilter` interface (method `Handle(ctx, downstream net.Conn)` — byte-identical to the retired `filterHandler` interface, so `*tcpproxy.Filter` + `*hcm.Filter` satisfy it with ZERO method changes) sits alongside the existing `ReadFilter`. The `*network.Registry` becomes the single registry for ALL network filters (read + terminal); `tcp_proxy` + HCM gain `network.NetworkFilterFactory` adapters and register in it. Every filter chain — including pure-`tcp_proxy`/pure-HCM chains — now builds through the unified chain factory and dispatches through a single `serveConnection` branch. The mixed read+terminal chain restriction LIFTS (the dispatch supports `[read-filter*, terminal-filter?]`); the buffered-prefix handover seam is shaped + unit-tested so 26.3's `rbac_network` (the first read filter that `Continue`s to a terminal) drops in. This is a MIGRATION/refactor: NO new operator-visible feature; `tcp_proxy`/HCM internals (their `Handle` connection-takeover loops) are UNTOUCHED — only their construction + dispatch wiring moves.

**Tech Stack:** Go 1.26.2; go-control-plane v1.32.4 proto bindings (ADR-0008); reference Envoy v1.37.2 (ADR-0008). ZERO new third-party `go.mod` dependencies (the migration is entirely in-house re-wiring). REUSES the 26.1 `internal/filter/network/` framework + the existing `internal/filter/tcpproxy/` + `internal/filter/hcm/` packages (no package move).

**Authored:** 2026-05-30. **Empirical-pin probe date (inherited):** 2026-05-30 (parent SPEC §11).

---

## 1. Purpose / Mission

Phase 26.2 pays down the network-filter-dispatch debt 26.1 deliberately deferred. At 26.1's close the listener manager carries TWO parallel per-connection dispatch mechanisms:

1. **The OLD terminal-filter path** (phases 02/04/05) — a hardcoded `var filterRegistry = map[string]filterConstructor` (`manager.go:104`) mapping `tcpproxy.TypeURL` + `hcm.TypeURL` to inline constructor closures; a private `filterHandler interface { Handle(ctx, conn) }` (`manager.go:46`); a `buildTerminalFilter` resolver (`manager.go:612`) requiring exactly one filter per chain; a `chainInfo.filter filterHandler` field; and the `serveConnection` step-7 `selected.filter.Handle(ctx, dispatchConn)` call (`manager.go:1108`).
2. **The NEW read-filter chain path** (26.1) — the boot-populated, frozen `*network.Registry`; the `buildNetChainFactory` build-time pre-check (`manager.go:654`); the `chainInfo.netChainFactory func() []network.ReadFilter` field; the `serveReadFilterChain` read loop (`manager.go:1126`); and the `serveConnection` step-7 dual-branch (`manager.go:1105`).

The dual-dispatch was the right 26.1 move (it confined 26.1's blast radius to NEW code — `tcp_proxy`/HCM never enter the new path, so back-compat was intrinsic). But it leaves the project with two registries, two dispatch idioms, and a transitional `network-filter-mixed-chain-unsupported` boot-reject (§6.2 of the 26.1 SPEC; an envoy-go-strict 26.1-only reject explicitly documented as "lifted at 26.2"). 26.2 unifies them.

The migration rests on one architectural observation (the parent SPEC §3.2 D-question, RESOLVED at §3.1 below): **`tcp_proxy` and HCM are *terminal connection-owning* filters.** Each takes over the raw downstream `net.Conn` via a blocking `Handle(ctx, downstream net.Conn)` that runs to connection close — `tcp_proxy` pumps bytes bidirectionally to a freshly-dialed upstream cluster member (`tcpproxy/filter.go:69`); HCM runs the full HTTP/1 driver or HTTP/2 codec over the conn (`hcm/filter.go:66`). This is fundamentally NOT the read-filter `OnData(buf, endStream) Status` byte-inspection model that `echo`/`direct_response` use (they write *through* `ReadFilterCallbacks.Connection().Write` while the framework's read loop owns `conn.Read`). Upstream Envoy models `tcp_proxy`/HCM as terminal read filters that return `StopIteration` and drive the connection directly via the connection object; envoy-go's existing `Handle(ctx, conn)` IS that connection-takeover form. **The migration therefore adds a terminal-filter seam to the framework rather than rewriting `Handle` into incremental `OnData` consumers** — preserving `tcp_proxy`/HCM internals verbatim, which makes byte-exact parity (R3) intrinsic. ADR-0213's explicit API-revision-allowance clause anticipated exactly this framework growth.

After phase 26.2 the project has: ONE network-filter registry (`*network.Registry`, now carrying `echo`/`direct_response`/`tcp_proxy`/HCM); ONE per-connection dispatch path (`serveConnection` step-7 → the unified network-chain runner); the hardcoded `filterRegistry`/`filterConstructor`/`filterHandler`/`buildTerminalFilter` retired; and a chain model that expresses mixed read+terminal chains (`[read-filter*, terminal-filter?]`) — with `rbac_network` (26.3) as the first production consumer of a read filter that `Continue`s to a terminal. NO operator-visible behavior changes; the stat surface stays 132; the existing `tcp_proxy`/HCM differential fixtures (the deliberate-break migration proof) stay byte-exact green.

### 1.1 Empirical-finding-driven scope (per parent SPEC §1.1)

The 11 AMENDs (A1–A11) in the parent SPEC §1.1 are the empirical-finding-driven scope revisions for phase 26. The amendments load-bearing for **26.2**:

- **§11.5 D5** (the read-filter iteration protocol — two-value `Continue`/`StopIteration`; `onNewConnection` eager-at-accept; connection-level buffering; `ContinueReading` resume-at-next-filter) — the 26.1-landed semantics the unified dispatch retains UNCHANGED for read filters; the terminal-filter seam extends the chain model WITHOUT touching read-filter iteration. The upstream `filter_manager_impl.cc` model (where `tcp_proxy`/HCM ARE terminal read filters that `StopIteration` + take over the connection) is the conceptual basis for the terminal-filter seam (§3.1).
- **AMEND-A11** (the migration is mechanical; the LoC envelope fits — parent §11.8 D8: 26.2 ~500-900 LoC, ~10-16 tasks) — `tcp_proxy`/HCM `Handle` is UNTOUCHED; only their construction (a `network.NetworkFilterFactory` adapter) + the `manager.go` wiring move. The bulk of the LoC is the dispatch-unification + old-path retirement, much of which overlaps the 26.1 dual-dispatch rewire.

This 26.2 SPEC makes NO NEW substantive scope revisions vs the parent SPEC; the migration approach inherits the BRAINSTORM Q4 "read-filter-only + API-revision allowance" framing (the terminal-filter seam is that allowance firing). The 26.2 SPEC's ADDITIVE contributions:

- **D-S1 resolution at SPEC time** (per §11.1 below): the master-tip baselines (fuzzers 35; differential fixtures 44, numbering tail 0042; stat surface 132; DECISIONS.md tail ADR-0214 → ADR-0215 at THIS SPEC commit) VERIFIED via project-wide grep. Pins the ADR-0215 §Decision body + BEHAVIOR_CONTRACT.md at IMPL.
- **The parent §3.2 D-question RESOLVED** (§3.1): `tcp_proxy`/HCM migrate via a NEW `network.TerminalFilter` seam (connection-takeover), NOT an `OnData` rewrite. Documented with the rejected alternative + rationale.
- **Refined `internal/filter/network/` API extensions** (§3.2): the `TerminalFilter` interface; the generalized `FilterInstanceFactory` return type (`ReadFilter` → `NetworkFilter` marker); the extended `FactoryCtx` (per-chain build fields); the unified chain-runner support for `[read-filter*, terminal-filter?]` + the buffered-prefix handover seam.
- **The `manager.go` retirement + unification surface** (§3.3): exactly which types/funcs are deleted, which fields collapse, how the constructors + the existing test callers re-wire, and where canonical network-filter registration lives.

---

## 2. Non-purposes

Phase 26.2 is the migration/refactor sub-phase. It does NOT extend any subsystem beyond what the `tcp_proxy`/HCM migration + the dispatch unification + the hardcoded-registry retirement require, under the 1 NEW ADR (ADR-0215).

- **2.1 No `tcp_proxy`/HCM behavior change.** `tcpproxy.Filter.Handle` (`tcpproxy/filter.go:69`) + `hcm.Filter.Handle` (`hcm/filter.go:66`) — the L4 bidirectional pump + the HTTP-codec drive loops — are UNTOUCHED. The migration moves their *construction* (onto a `network.NetworkFilterFactory` adapter) + their *dispatch wiring* (through the unified path), NOT their connection-handling logic. Byte-exact parity is intrinsic precisely because no byte-handling code changes (R3).
- **2.2 No `OnData` rewrite of the terminal filters.** `tcp_proxy`/HCM are NOT re-architected into incremental `OnData(buf, endStream)` read filters (the rejected Alt B, §3.1). They remain connection-takeover filters via the NEW `network.TerminalFilter` seam.
- **2.3 No `rbac_network` / `internal/rbac/` / connection-metadata writes.** Those are 26.3 (parent §3.2 / §4.3 / §4.4). 26.2 SHAPES the mixed read→terminal chain capability (so `rbac_network` drops in) but lands NO RBAC code, NO `internal/rbac/` extraction, and NO `dynamicmetadata.Bucket` writes.
- **2.4 No write-filter chain.** Read-filter-only + the terminal-filter seam remain the scope (parent §2.1 / BRAINSTORM Q4). The `network.TerminalFilter` seam is NOT a write-filter; it is the connection-takeover form upstream's terminal read filters already use. ADR-0213's API-revision-allowance clause covers a future write-filter; 26.2 does not consume it.
- **2.5 No new operator-visible network filter.** 26.2 adds ZERO new filters. The four network filters after 26.2 are `echo` + `direct_response` (26.1) + `tcp_proxy` + HCM (migrated). `rbac_network` is 26.3.
- **2.6 No per-route surface.** Network filters are chain-scoped, not route-scoped (parent §2.2 / §11.6 D6). ADR-0125 untouched. (HCM's per-route HTTP routing is internal to the HCM terminal filter and unaffected by the L4-chain migration.)
- **2.7 No new stats.** The migration is stat-neutral: `tcp_proxy` (0 built-in L4 stats today) + HCM (its existing per-instance metrics, allocated unchanged by the HCM constructor) keep their exact stat registrations. The project stat surface stays **132** (parent §7.1; verify at §11.1 D-S1 + the six-gate). The framework adds no counters.
- **2.8 No new differential fixture at 26.2 (back-compat is the proof).** Migration correctness is proven by the EXISTING `tcp_proxy`/HCM differential fixtures (`0000-tcp-echo`, the TLS-TCP fixtures, the HCM/h2/wasm/lua fixtures) staying byte-exact green after the migration — the strongest deliberate-break migration proof (R3). The parent §8.2-anticipated "echo preceding tcp_proxy" multi-read-filter-chain fixture is DEFERRED to 26.3 (§8.2 explains: `echo` halts and never `Continue`s to a terminal, so no 26.2-shippable read filter exercises a real Continue→terminal differential path; `rbac_network`'s allow→`Continue`→`tcp_proxy` is the first such path). The mixed read→terminal *capability* is proven at 26.2 by unit tests (a synthetic always-`Continue` read filter + a recording terminal). Fixtures stay **44**.
- **2.9 No new conformance/h2spec harness.** The migration touches no HTTP/h2/proxy-wasm code path *internals* — HCM's `Handle` is unchanged. Conformance 10/10 + h2spec 53/53 are re-run as a back-compat gate (they exercise HCM through the migrated dispatch) and stay green (§15.2).
- **2.10 No fuzzer delta.** 26.2 adds no new config-parse surface (`tcp_proxy`/HCM parsing is unchanged; they already validate their `typed_config`). Fuzzer count stays **35** (the 36th is 26.3's rbac config-parse fuzzer). The existing tcp_proxy/HCM config-parse coverage carries over via the adapters.

---

## 3. Framework + dispatch changes (`internal/filter/network/` + `internal/listener/manager.go`)

Per parent SPEC §3.2 + §10.2 (ADR-0215) + the D-P2 26.1 resolution (the dual-dispatch landed fully at 26.1; 26.2 unifies). The 26.2 changes split into: the framework extension (§3.1 the terminal-filter seam; §3.2 the API-signature refinements), the `manager.go` retirement + unification (§3.3), the `tcp_proxy`/HCM adapters (§4), and the boot-wiring / registration-seam re-shape (§3.4).

### 3.1 The terminal-filter seam — RESOLVING the parent §3.2 D-question

**The question (parent SPEC §3.2 + §12; next-prompt 26.2 scope):** does `tcp_proxy` fit the read-filter `OnData` model, or does it need a terminal-filter extension to the framework?

**The resolution: a terminal-filter extension.** Two alternatives were weighed:

- **Alt A — terminal-filter seam (CHOSEN).** Add a NEW `network.TerminalFilter` interface whose single method `Handle(ctx context.Context, downstream net.Conn)` is byte-identical to the retired `manager.go` `filterHandler` interface. `*tcpproxy.Filter` + `*hcm.Filter` already implement `Handle(ctx, conn)` with that exact signature, so they satisfy `network.TerminalFilter` with ZERO method changes. The framework's chain model grows to express `[read-filter*, terminal-filter?]`; the unified dispatch runs the leading read filters' `OnNewConnection`/`OnData` iteration, and when control reaches a trailing terminal filter (or for a pure-terminal chain, immediately), hands the downstream conn to `Handle`. **Why chosen:** the terminal filters' connection-handling code is UNTOUCHED → byte-exact parity is intrinsic (R3 paramount); the seam mirrors upstream's model (terminal read filters that `StopIteration` + drive the connection); it is the minimal, mechanical migration the LoC envelope (parent §11.8) assumed; ADR-0213's API-revision-allowance clause anticipated it.
- **Alt B — rewrite `tcp_proxy`/HCM as `OnData` read filters (REJECTED).** `tcp_proxy` would dial upstream in `OnNewConnection`, spawn an upstream→downstream pump goroutine writing via `Connection().Write`, and forward downstream bytes to upstream in `OnData`; HCM would feed bytes incrementally into a re-architected HTTP codec. **Why rejected:** a massive, high-risk rewrite — HCM's H1 driver (`runConnection`) and H2 `ServerConn` both fully OWN a `net.Conn` and assume a blocking read loop; re-architecting them into incremental byte-feed consumers is a multi-phase effort with severe byte-exact-parity risk. It directly violates the migration-not-rewrite mandate + R3 paramountcy. (A future phase MAY revisit a true read-filter `tcp_proxy` if a use case needs L4 filters *between* a read filter and the proxy with shared buffering — but that is not 26.2's job and the API-revision allowance keeps the door open.)

**Chain shape after 26.2.** A network-filter chain is `[read-filter*, terminal-filter?]` — zero or more `ReadFilter`s (`echo`, `direct_response`, future `rbac_network`) optionally ending in exactly one `TerminalFilter` (`tcp_proxy` or HCM). Build-time validation (§6): a terminal filter may appear ONLY as the last filter; at most one terminal filter per chain; a chain with no terminal filter is valid (its read filters either halt — `echo` — or write+close — `direct_response`). Realistic 26.2 chains:

| Chain | Kind | Dispatch | Parity basis |
|---|---|---|---|
| `[echo]` | pure read (halts) | read-loop only (26.1 path, unchanged) | fixture 0040 |
| `[direct_response]` | pure read (writes+closes) | read-loop only (26.1 path, unchanged) | fixture 0041 |
| `[tcp_proxy]` | pure terminal | immediate `Handle(conn)` — byte-identical to today's `selected.filter.Handle` | existing tcp_proxy fixtures (R3) |
| `[hcm]` | pure terminal | immediate `Handle(conn)` — byte-identical to today | existing HCM/h2 fixtures (R3) |
| `[read*, terminal]` | mixed | read-loop until read filters `Continue`, then `Handle` over the buffered-prefix conn | unit test (synthetic Continue filter); first production consumer = `rbac_network` 26.3 |

The mixed `[read*, terminal]` capability is the LIFT of the `network-filter-mixed-chain-unsupported` 26.1-transitional reject. At 26.2 NO shippable read filter `Continue`s to a terminal (`echo` halts via `StopIteration` + never `ContinueReading`s; `direct_response` writes + closes) — so the mixed path is exercised by a unit test, not a differential fixture (§8.2). The buffered-prefix handover seam (§3.2) is shaped + unit-tested so 26.3's `rbac_network` (allow → `Continue` → `tcp_proxy`/HCM, where the bytes `rbac_network` already read off the socket must be replayed to the terminal) drops in without a framework revision.

### 3.2 `internal/filter/network/` API extensions (refined; land at IMPL)

Production signatures. Each design point is carried into the IMPL task that lands it (§10):

- **NEW `TerminalFilter` interface** (in `types.go` or a new `terminal.go`): `Handle(ctx context.Context, downstream net.Conn)`. Byte-identical to the retired `manager.go` `filterHandler`. `*tcpproxy.Filter` + `*hcm.Filter` satisfy it verbatim. A `TerminalFilter` does NOT implement `ReadFilter` (it has no `OnData` — it owns the raw conn). It owns the conn-close lifecycle exactly as the old `filterHandler.Handle` contract did (the unified dispatch does NOT close the conn for a terminal filter; `Handle`'s own `defer conn.Close()` runs — byte-identical to today).

  ```go
  // TerminalFilter is a network filter that takes over the downstream connection
  // at the END of the chain (tcp_proxy: L4 bidirectional pump to an upstream
  // cluster member; HCM: the HTTP/1 driver or HTTP/2 codec). Unlike a ReadFilter
  // (which inspects buffered bytes via OnData and writes through
  // ReadFilterCallbacks.Connection), a TerminalFilter owns the raw net.Conn and
  // runs a blocking serve loop to connection close. It mirrors upstream's
  // terminal read filters (tcp_proxy/HCM), which return StopIteration and drive
  // the connection directly. Handle's signature is byte-identical to the
  // phase-02 manager.go filterHandler interface this seam retires, so the
  // existing *tcpproxy.Filter + *hcm.Filter satisfy it with no method change.
  //
  //nolint:revive // ADR-0215 reserves the network.TerminalFilter name.
  type TerminalFilter interface {
      // Handle takes ownership of the downstream connection and runs to
      // completion. It owns the conn-close lifecycle (the unified dispatch does
      // NOT close the conn for a terminal filter).
      Handle(ctx context.Context, downstream net.Conn)
  }
  ```

- **Generalized `FilterInstanceFactory` return type** — `ReadFilter` → a `NetworkFilter` marker that both `ReadFilter` and `TerminalFilter` satisfy. The factory yields either kind; the chain builder/dispatch type-switches. This is a deliberate 26.1-API revision (ADR-0213 allowance): `echo`/`direct_response` factories now return instances typed as `NetworkFilter` (they still ARE `ReadFilter`s).

  ```go
  // NetworkFilter is the common interface a chain filter satisfies — either a
  // ReadFilter (OnData inspection model) or a TerminalFilter (connection-takeover
  // model). The chain builder classifies each; the dispatch type-switches.
  type NetworkFilter interface {
      networkFilter() // sealed marker; ReadFilter + TerminalFilter embed it
  }

  // FilterInstanceFactory allocates a network filter instance. For ReadFilters a
  // FRESH instance per accepted connection (per-conn state). For TerminalFilters
  // the SAME boot-built instance every call (conn state lives on Handle's stack;
  // tcp_proxy/HCM build once per chain and share across connections — preserving
  // today's chainInfo.filter shared-instance semantic).
  type FilterInstanceFactory func() NetworkFilter
  ```

  > **D-26.2-1 (sealed marker vs open type-switch):** the SPEC anchors a *sealed* `NetworkFilter` marker (an unexported `networkFilter()` method embedded into both `ReadFilter` and `TerminalFilter`) so a value cannot accidentally satisfy `NetworkFilter` without being one of the two kinds, and the dispatch type-switch is exhaustive. The PLAN/IMPL MAY instead use an open marker (empty interface) + a dispatch-time "neither kind" boot/dispatch error if the sealed-method churn on `ReadFilter` (a 26.1-shipped interface — adding an unexported method affects its out-of-package implementers, which are all in-repo — `echo` + `direct_response` live in `internal/filter/network/echo` + `.../directresponse`, OUTSIDE the `network` package, so they must embed the marker, captured in Task 2's "update echo/direct_response to satisfy the marker") proves friction. **Resolution at:** 26.2 PLAN/IMPL. **Anticipated:** sealed marker (exhaustive, type-safe; the only implementers are in-repo and updated in the same task).

- **Extended `FactoryCtx`** — gains the per-chain build fields the terminal-filter factories consume. CRITICAL design constraint: keep the `internal/filter/network` package IMPORT-LIGHT (it must NOT import `internal/cluster`, `internal/stats`, `internal/accesslog`, `internal/filter/http`, `internal/drain`, `internal/httpclient`, `internal/filter/hcm`, or `internal/filter/tcpproxy` — that would risk import cycles and couple the framework to every consumer). Therefore `FactoryCtx` carries ONLY primitives + the already-present `BaseDir` (the per-chain build context that varies per filter-chain); the heavy boot singletons (`*cluster.Manager`, `*drain.Manager`, `*stats.Registry`, `[]accesslog.Sink`, `*filter_http.HTTPRegistry`, `*httpclient.Client`) are **captured in the registration closures** (§3.4) — exactly as the retired `filterRegistry` closures received them as call-time params. The split:

  ```go
  // FactoryCtx carries the PER-CHAIN build context a NetworkFilterFactory needs.
  // Primitives + BaseDir only — the heavy boot singletons (cluster manager, stats
  // registry, access-log sinks, HTTP-filter registry, drain manager, http client)
  // are captured in the registration closures (cmd/envoy-go/main.go §3.4), keeping
  // this package free of cluster/stats/hcm imports.
  type FactoryCtx struct {
      BaseDir            string // 26.1: direct_response DataSource Filename resolution
      // Per-chain terminal-filter build context (26.2; consumed by the HCM
      // adapter — mirrors the retired manager.go listenerCtx). echo/direct_response
      // ignore these; tcp_proxy ignores all but is handed them uniformly.
      HasTLS             bool   // chain has a *stdtls.Config (hcm.ListenerCtx.HasTLS)
      AllowH2C           bool   // --allow-h2c (hcm.ListenerCtx.AllowH2C)
      ListenerPrincipal  string // per-chain leaf-cert principal (hcm.ListenerCtx.ListenerPrincipal)
      NodeServiceCluster string // bootstrap node.cluster (hcm.ListenerCtx.NodeServiceCluster)
  }
  ```

  > **D-26.2-2 (FactoryCtx field set):** the exact field set is finalized at IMPL against the HCM + tcp_proxy constructor signatures (`hcm.NewFilterWithCtxAndSinksAndRegistry` consumes `hcm.ListenerCtx{HasTLS, AllowH2C, ListenerPrincipal, HTTPClient, NodeServiceCluster}` + `cm`/`registry`/`accessLogSinks`/`httpRegistry`/`dm`; `tcpproxy.NewFilter` consumes `cm`/`dm`). `HTTPClient` is a global singleton (the shared `*httpclient.Client`) → captured in the closure, NOT a FactoryCtx field. `HasTLS`/`ListenerPrincipal` vary per chain → FactoryCtx. `AllowH2C`/`NodeServiceCluster` are global but cheap primitives → carried in FactoryCtx for uniformity (alternatively closure-captured). **Resolution at:** 26.2 PLAN. **Anticipated:** the four fields above.

- **Unified chain runner support for `[read-filter*, terminal-filter?]`** — the `ChainRuntime` (`chain.go`) gains: (a) acceptance of `[]NetworkFilter` (classified into the read-filter prefix + the optional trailing terminal); (b) a "read filters completed (all `Continue`d) → terminal reached" signal; (c) the **buffered-prefix handover**: when control reaches the terminal, the undrained bytes in the connection read `*Buffer` (bytes a preceding read filter inspected but did not consume) are handed to the terminal's `Handle` via a `net.Conn` wrapper that serves the buffered prefix BEFORE reading fresh from the socket. At 26.2 no read filter `Continue`s to a terminal (so the prefix is always empty for shippable chains), but the seam is shaped + unit-tested (synthetic always-`Continue` read filter that drains nothing → a recording terminal asserts it receives the buffered prefix then the live socket bytes). For a pure-terminal chain (`[tcp_proxy]`/`[hcm]`) there is NO read-filter prefix, so the terminal receives the raw dispatch conn with nothing pre-consumed — byte-identical to today.

  > **D-26.2-3 (buffered-prefix conn wrapper):** the handover wrapper is a small `net.Conn` adapter (`prefixConn`) that returns the buffered prefix bytes on the first `Read`(s), then delegates to the underlying conn; all other methods delegate. **Resolution at:** 26.2 PLAN/IMPL. **Anticipated:** a `prefixConn` in `chain.go` (or `terminal.go`); shaped at 26.2, first production-exercised by `rbac_network` at 26.3. The PLAN confirms whether to land it at 26.2 (unit-tested, for 26.3 readiness — mirrors how 26.1 shaped `DynamicMetadata` ahead of its 26.3 consumer) or defer to 26.3; **anticipated: land at 26.2** so the "mixed chains become expressible" claim is honest + proven.

- **`*network.Registry` is unchanged in mechanism** — still the freeze-after-boot threaded-constructor registry (ADR-0214). 26.2 only changes WHAT registers in it (now also `tcp_proxy` + HCM) and makes it INTRINSIC (no longer nil-optional — §3.4). `KnownTypeURLs` now lists all four built-in network filters (used by the unified unknown-type boot-reject error).

### 3.3 `manager.go` retirement + dispatch unification

The OLD terminal-filter path is DELETED; everything dispatches through the unified network-chain path. Specific changes:

- **DELETE** `filterHandler interface` (`manager.go:46`) — replaced by `network.TerminalFilter` (its `Handle` method moves to the framework). DELETE `filterConstructor` type (`manager.go:97`). DELETE `var filterRegistry` (`manager.go:104`). DELETE `buildTerminalFilter` (`manager.go:612`). DELETE the `listenerCtx` struct (`manager.go:61`) IF its fields fully migrate into `FactoryCtx` + the registration closures (the `httpClient` global goes to the closure; `hasTLS`/`allowH2C`/`listenerPrincipal`/`nodeServiceCluster` go to `FactoryCtx`) — **D-26.2-4** confirms at IMPL whether `listenerCtx` is fully retired or retained as a thin per-chain carrier.
- **`chainInfo`** (`manager.go:184`): DROP the `filter filterHandler` field. The `netChainFactory` field generalizes from `func() []network.ReadFilter` to `func() []network.NetworkFilter` (read + terminal). EVERY chain (including pure-`tcp_proxy`/pure-HCM) now carries a `netChainFactory`; the "exactly one of `filter`/`netChainFactory`" invariant collapses to "always `netChainFactory`".
- **`buildNetChainFactory`** (`manager.go:654`) becomes THE chain builder (rename → `buildNetworkChainFactory` or keep): it resolves EVERY filter in the chain against `netReg` (now containing all four built-ins), classifies read vs terminal, validates the `[read*, terminal?]` shape (§6: terminal-only-last, ≤1 terminal), invokes each `NetworkFilterFactory` once at boot with the per-chain `FactoryCtx`, and returns the `func() []network.NetworkFilter` closure. The old "filters[0] not in netReg → fall through to terminal path" branch is DELETED (there is no terminal path to fall through to); an unresolvable `type_url` is now a unified unknown-type boot-reject (§6.1). The `expected exactly one filter` constraint (old `buildTerminalFilter`) is REPLACED by the `[read*, terminal?]` shape validation.
- **`serveConnection` step-7** (`manager.go:1104-1109`): collapse the dual-branch to a SINGLE call — `rt.serveNetworkChain(ctx, dispatchConn, *selected)`. The `serveReadFilterChain` (`manager.go:1126`) generalizes into `serveNetworkChain`: build the `[]network.NetworkFilter`; if the chain is pure read filters (no terminal), run the existing 26.1 read loop (`OnNewConnection` + the `OnData` socket loop, unchanged); if the chain ends in a terminal, run the read-filter prefix then hand the (buffered-prefix) conn to `terminal.Handle(ctx, conn)`; for a pure-terminal chain, call `terminal.Handle(ctx, dispatchConn)` directly (byte-identical to today's `selected.filter.Handle`). The conn-close lifecycle: the read-loop path closes the conn as today (`serveReadFilterChain`'s `defer conn.Close()`); the terminal path lets `Handle` own the close (as the old terminal path did).
- **Constructor + caller re-wiring** — `netReg` becomes a REQUIRED (non-nil) argument: the unified path resolves EVERY chain through `netReg`, so a nil registry would reject every chain. The 26.1 nil-tolerance ("nil netReg → every chain takes the old terminal path") is REMOVED (there is no old path). The blast radius (the `NewManagerWithBaseDirAndAllowH2C` callers the 26.1 PROGRESS enumerated — `internal/admin/admin_helpers_test.go`, `internal/admin/listeners_test.go`, `internal/listener/manager_test.go`, `cmd/envoy-go/main_test.go`, + the thinner `NewManager`/`NewManagerWithBaseDir` ctors) must each obtain a populated `netReg`. **D-26.2-5** (§12) resolves the registration seam (§3.4).

### 3.4 Registration seam + boot-wiring re-shape

After 26.2, the canonical network-filter registration registers all FOUR built-ins (`echo`, `direct_response`, `tcp_proxy`, HCM) with their boot singletons captured. The 26.1 boot-wiring (`main.go`: `netReg.Register(echo.TypeURL, echo.New)` + `netReg.Register(directresponse.TypeURL, directresponse.New)` + `netReg.Freeze()`) extends to register the two terminal filters via closure-capturing adapters:

```go
// cmd/envoy-go/main.go (26.2): register all four network filters. The terminal
// filters capture the boot singletons (cm, dm, registry, sinks, httpReg,
// httpClient); the per-chain build context arrives via network.FactoryCtx at
// chain-build time (manager.go buildNetworkChainFactory).
netReg := network.NewRegistry()
netReg.Register(echo.TypeURL, echo.New)
netReg.Register(directresponse.TypeURL, directresponse.New)
netReg.Register(tcpproxy.TypeURL, tcpproxy.NewNetworkFactory(cm, drainMgr))
netReg.Register(hcm.TypeURL, hcm.NewNetworkFactory(cm, bs.Stats, sinks, httpReg, drainMgr, httpClient))
netReg.Freeze()
```

> **D-26.2-5 (registration seam location + test-caller wiring):** because `netReg` is now required by every manager constructor + test caller, the SPEC anchors a single registration helper that both production (`main.go`) and tests share, to avoid duplicating the four `Register` calls (and to keep the boot-singleton wiring in one place). **Options:** (a) a `RegisterBuiltins(reg *network.Registry, deps BuiltinDeps)` helper in a small wiring package (or in `cmd/envoy-go/`); (b) the thinner `NewManager`/`NewManagerWithBaseDir` constructors build + populate a default `netReg` internally from the singletons they already receive (the manager already imports `tcpproxy` + `hcm`); (c) a test-only helper that builds a `netReg` with nil/stub singletons for the dispatch-shape tests that don't need a live cluster. **Resolution at:** 26.2 PLAN. **Anticipated:** a shared `RegisterBuiltins` helper consumed by `main.go` + a test fixture for the callers; the exact package placement (avoiding import cycles — the helper imports `tcpproxy`/`hcm`/`echo`/`directresponse` + `network`, so it CANNOT live in `network` or `listener`; likely `cmd/envoy-go/` or a new tiny `internal/filter/network/builtins` wiring package) is a PLAN/IMPL decision. The naming `tcpproxy.NewNetworkFactory` / `hcm.NewNetworkFactory` (the adapter constructors, §4) is SPEC-anticipated; PLAN pins exact names.

The adapter constructors (`tcpproxy.NewNetworkFactory` / `hcm.NewNetworkFactory`) live in the `tcpproxy` / `hcm` packages (which now import `internal/filter/network` for `NetworkFilterFactory`/`FilterInstanceFactory`/`NetworkFilter`/`FactoryCtx` — a one-directional import; `network` does NOT import `tcpproxy`/`hcm`). Registration order does not affect runtime behavior (ADR-0214 freeze-after-boot, lock-free lookup).

---

## 4. Filter migrations (`tcp_proxy` + HCM adapters)

### 4.1 `tcp_proxy` (`internal/filter/tcpproxy/`)

- **`Handle` UNTOUCHED** (`filter.go:69`) — the L4 bidirectional pump (dial upstream via `cluster.Dial` → two `io.Copy` goroutines + `halfClose`) is byte-identical. `*tcpproxy.Filter` satisfies `network.TerminalFilter` (it already has `Handle(ctx context.Context, downstream net.Conn)`); the IMPL adds the `network.TerminalFilter` assertion (a compile-time `var _ network.TerminalFilter = (*Filter)(nil)`) + the sealed-marker `networkFilter()` method if D-26.2-1 lands the sealed marker.
- **NEW adapter constructor** `NewNetworkFactory(cm *cluster.Manager, dm *drain.Manager) network.NetworkFilterFactory` — a closure capturing `cm`/`dm`; returns a `NetworkFilterFactory` that, given the chain's `typed_config` Any + `FactoryCtx`, calls the existing `NewFilter(tc, cm, dm)` once at boot and returns a `FilterInstanceFactory` yielding that SAME shared `*Filter` instance per connection (terminal filters are conn-stateless; today's `chainInfo.filter` is likewise shared across connections — preserving the semantic). The existing `NewFilter` (`filter.go:40`) is UNCHANGED; the adapter wraps it.
- **PARSE-REJECT** — `tcp_proxy`'s existing config rejects (wrong type_url, unmarshal failure, missing/empty cluster, `weighted_clusters` unsupported, unknown cluster — `filter.go:41-63`) carry over verbatim through the adapter (the adapter surfaces `NewFilter`'s error as the `NetworkFilterFactory` error → boot-reject, wrapped with the chain error prefix). Byte-stable error wording (`tcpproxy: ...`) is preserved; the IMPL verifies via the existing tcp_proxy parse tests + a `TestParseRejectConstants_ByteStable`-style check that the error strings are unchanged post-migration.

### 4.2 HCM (`internal/filter/hcm/`)

- **`Handle` UNTOUCHED** (`filter.go:66`) — the ALPN-dispatch + H1-driver / H2-codec drive loops are byte-identical. `*hcm.Filter` satisfies `network.TerminalFilter` (it has `Handle(ctx, conn)`); the IMPL adds the compile-time assertion + sealed-marker method (per D-26.2-1).
- **NEW adapter constructor** `NewNetworkFactory(cm *cluster.Manager, registry *stats.Registry, sinks []accesslog.Sink, httpReg *filter_http.HTTPRegistry, dm *drain.Manager, httpClient *httpclient.Client) network.NetworkFilterFactory` — captures the boot singletons; returns a `NetworkFilterFactory` that, given the chain's `typed_config` + `FactoryCtx` (supplying the per-chain `HasTLS`/`AllowH2C`/`ListenerPrincipal`/`NodeServiceCluster`), calls the existing `NewFilterWithCtxAndSinksAndRegistry(tc, cm, hcm.ListenerCtx{...}, registry, sinks, httpReg, dm)` once per chain at boot and returns a `FilterInstanceFactory` yielding that shared `*Filter`. The existing constructor (`filter.go:45`) + `hcm.ListenerCtx` (`config.go:55`) are UNCHANGED; the adapter bridges `network.FactoryCtx` → `hcm.ListenerCtx`. (This bridge is exactly what the retired `filterRegistry` HCM closure did at `manager.go:112-132` — the migration MOVES that bridge from the manager into the adapter.)
- **Per-instance metrics** — the HCM constructor allocates its existing per-instance metrics from the `registry` (unchanged); the stat surface is unaffected (§2.7). The IMPL verifies the HCM stat registrations are byte-identical post-migration (the same `registry` flows through; only the call site moves).
- **PARSE-REJECT** — HCM's existing config rejects carry over verbatim through the adapter.

### 4.3 No package move

Per parent §2.3, `tcp_proxy` + HCM stay in `internal/filter/tcpproxy/` + `internal/filter/hcm/`. Only their construction adapters + the import of `internal/filter/network` are added. The `internal/filter/network` package does NOT import them (one-directional).

---

## 5. Proto-field roster (cross-reference parent §5)

INHERITED. `tcp_proxy` (`envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy`) + HCM (`…http_connection_manager.v3.HttpConnectionManager`) proto parsing is UNCHANGED — the existing parsers (`tcpproxy.NewFilter`, `hcm` config parse) run verbatim through the adapters. 26.2 adds NO new proto fields. The 26.1 echo + direct_response rosters (parent §5.1/§5.2) are unchanged. The 26.3 network-rbac roster (parent §5.3) is out of 26.2 scope. The 26.2 IMPL first-task re-confirms (R-S baselines) that the tcp_proxy + HCM type URLs (`tcpproxy.TypeURL`, `hcm.TypeURL`) are stable against go-control-plane v1.32.4 before wiring the registrations.

---

## 6. PARSE-REJECT roster (cross-reference parent §6 + the 26.1 §6.2 mixed-chain LIFT)

Per ADR-0080 byte-stable PARSE-REJECT discipline. The exact wording + the `TestParseRejectConstants_ByteStable` tables are finalized at 26.2 IMPL (D-26.2-6). The migration's reject-surface changes:

### 6.1 Unified unknown-type-url reject (REPLACES the dual-registry miss)

- `network-filter-unknown-type-url` — a `filter_chains[].filters[].typed_config.type_url` not in the frozen `*network.Registry` (which now contains all four built-ins) → boot-reject. The 26.1 dual-dispatch had TWO miss-paths (the `netReg` miss fell through to the `filterRegistry` miss at `buildTerminalFilter`); 26.2 collapses them to ONE. **Byte-stability concern (R-S):** the existing `buildTerminalFilter` unknown-type wording (`listener: %q: filter_chains[%d]: unknown filter type_url %q`, `manager.go:628`) is the wording operators + the existing fixtures may depend on. The IMPL MUST preserve the exact existing wording for the unknown-type case (the unified path emits the same string) — verified against any existing unknown-network-filter test/fixture. **D-26.2-6** finalizes.

### 6.2 NEW chain-shape rejects (the `[read*, terminal?]` validation)

- `network-filter-terminal-not-last` — a terminal filter (`tcp_proxy`/HCM) appearing at a non-final chain position (e.g. `[tcp_proxy, echo]`) → boot-reject (a terminal filter owns the connection; nothing can follow it). Upstream parity note: upstream requires the terminal filter to be last in the chain; this mirrors it. **NEW arm; wording at IMPL.**
- `network-filter-multiple-terminals` — more than one terminal filter in a chain (`[tcp_proxy, hcm]`) → boot-reject. **NEW arm; wording at IMPL.** (MAY be folded into `terminal-not-last` if the validation naturally subsumes it — D-26.2-6.)

### 6.3 LIFTED reject

- `network-filter-mixed-chain-unsupported` (the 26.1 §6.2 transitional envoy-go-strict reject) is **DELETED**. Mixed `[read-filter*, terminal-filter]` chains are now valid + expressible (§3.1). The BEHAVIOR_CONTRACT departure record for it is updated (the 26.1 entry noted "lifted at 26.2"; 26.2 removes the restriction + records the lift — §9). The old-path `expected exactly one filter` constraint (retired with `buildTerminalFilter`) is likewise replaced by the `[read*, terminal?]` shape validation — a chain may now carry multiple filters.

---

## 7. Stat surface (cross-reference parent §7.1)

The migration is stat-neutral. `tcp_proxy`: 0 built-in L4 stats (unchanged). HCM: its existing per-instance metrics, allocated by the HCM constructor from the same `*stats.Registry` (the call site moves into the adapter; the registrations are byte-identical). `echo`/`direct_response`: 0 stats (unchanged). The framework adds 0 counters. **Project stat surface stays 132** (parent §7.1/§7.3). The 26.2 IMPL first-task re-confirms the 132 master-tip baseline via the existing stat-roster grep (R7-analogue) and asserts the 132 → 132 no-delta as a six-gate item.

---

## 8. Differential fixture taxonomy (+0; back-compat is the proof; cross-reference parent §8.2)

### 8.1 Back-compat as the migration proof (R3)

The EXISTING `tcp_proxy` + HCM differential fixtures stay byte-exact green after the migration — the strongest deliberate-break migration proof. The migration leaves a fixture green ONLY if the dispatch genuinely re-wires correctly (a broken adapter/dispatch would fail byte-exactness). The R3 gate fixtures (verified-present at §11.1): `0000-tcp-echo` + `0002-tls-tcp` (tcp_proxy) + the HCM/h2/wasm/lua fixtures (HCM through the migrated dispatch). All differential fixtures (44) + conformance (10/10) + h2spec (53/53) stay green (§15.2).

### 8.2 The deferred mixed-chain fixture (parent §8.2 → 26.3)

The parent §8.2 anticipated an OPTIONAL "+1 multi-read-filter-chain fixture (`echo` preceding `tcp_proxy`)". This is DEFERRED to 26.3, for a load-bearing reason: **`echo` halts** — its `OnData` returns `StopIteration` and it never calls `ContinueReading` — so a chain `[echo, tcp_proxy]` would never advance to `tcp_proxy` (echo would echo bytes forever and the terminal would never run). There is no 26.2-shippable read filter that `Continue`s to a terminal. The FIRST read filter that does is `rbac_network` (26.3): on `allow` it returns `Continue`, advancing to the `tcp_proxy`/HCM terminal (`[rbac_network, tcp_proxy]` — the real mixed read→terminal differential path). So 26.2 proves the mixed-chain *capability* via a UNIT test (a synthetic always-`Continue` read filter + a recording terminal asserting the buffered-prefix handover), and 26.3 lands the differential fixture for the real consumer. **Fixtures stay 44 at 26.2.**

### 8.3 Total fixture-dir count

44 → **44** at 26.2 phase-done (+0). The chain-shape rejects (§6.2) + the unified unknown-type reject (§6.1) are unit-test-only boot-rejects (envoy-go-strict / re-confirming existing wording — not new cross-side parity fixtures; covered by `manager.go` build-path unit tests). No new conformance harness (§2.9).

---

## 9. Behavior-contract delta (per parent §9, the 26.2 bundle)

BEHAVIOR_CONTRACT.md gains the phase-26.2 content as ONE atomic bundle at the 26.2 IMPL final task (per ADR-0052). Anticipated edits (a structural/refactor note — no operator-visible behavior change):

- UPDATE the `## Network filters` `### Network filter chain framework` subsection: document the NEW `TerminalFilter` seam (connection-takeover model alongside the `ReadFilter` OnData model); the `[read-filter*, terminal-filter?]` chain shape; the unified single-dispatch path (the dual-dispatch retired); the `tcp_proxy`/HCM migration onto the registry (no behavior change). REMOVE the `network-filter-mixed-chain-unsupported` 26.1-transitional departure record (the restriction is LIFTED) — replace with a note that mixed read+terminal chains are now expressible (first consumer `rbac_network` 26.3) + the NEW chain-shape rejects (`terminal-not-last`, `multiple-terminals`).
- A structural note: the hardcoded `manager.go` `filterRegistry`/`filterConstructor`/`filterHandler`/`buildTerminalFilter` retirement; all four network filters now resolve through `*network.Registry`; `tcp_proxy`/HCM `Handle` UNCHANGED (back-compat via existing fixtures).
- Confirm the stat surface stays 132 + no new fixtures/fuzzers.
- A forward-pointer note: 26.3 adds `rbac_network` (the first mixed read→terminal consumer) + the shared `internal/rbac/` engine + the connection-metadata writes.

---

## 10. Per-task structure (~10-16 tasks; per parent §11.8 + §15)

The parent §11.8 D8 envelope for 26.2: `tcp_proxy` adapt (~40-80) + HCM adapt (~60-120) + `manager.go` registry retirement + dispatch unification (~200-350, much overlapping 26.1's rewire) + the terminal-filter seam + buffered-prefix handover (~120-200) + test churn (~100-200) ≈ **500-900 LoC, ~10-16 tasks** — fits the ADR-0045 gate (~25 tasks / ~1500 LoC), net-new basis, ~0 moved LoC (the HCM-bridge closure MOVES from manager into the adapter — mechanical). The 26.2 PLAN authors the exact bite-sized TDD tasks; the SPEC-anticipated task spine (the PLAN may merge/split):

| # | Task | Lands |
|---|---|---|
| 1 | First-task baselines: re-confirm fuzzers **35** + fixtures **44** (tail 0042) + stat surface **132** + DECISIONS.md tail **ADR-0215** (this SPEC drafts it) via grep; re-confirm `tcpproxy.TypeURL`/`hcm.TypeURL` + the `manager.go` line anchors (filterRegistry/buildTerminalFilter/serveConnection step-7/buildNetChainFactory) against the IMPL-session tip | §11 / §5 gates |
| 2 | NEW `network.TerminalFilter` interface + the `NetworkFilter` sealed marker (D-26.2-1) + generalize `FilterInstanceFactory` return type to `NetworkFilter`; update `echo`/`direct_response` to satisfy the marker; `types.go`/`terminal.go` + tests | §3.2 |
| 3 | Extend `network.FactoryCtx` with the per-chain build fields (HasTLS/AllowH2C/ListenerPrincipal/NodeServiceCluster) (D-26.2-2) + tests | §3.2 |
| 4 | Unified `ChainRuntime` support for `[read*, terminal?]` + the buffered-prefix handover `prefixConn` (D-26.2-3) + `chain_test.go` (synthetic always-Continue read filter → recording terminal asserts prefix+live bytes; pure-terminal immediate handoff; pure-read unchanged) | §3.2 / §3.1 / R-M |
| 5 | `tcpproxy.NewNetworkFactory` adapter (capture cm/dm; wrap `NewFilter`; shared-instance `FilterInstanceFactory`) + `var _ network.TerminalFilter` assertion + adapter tests (parse-reject pass-through byte-stable) | §4.1 |
| 6 | `hcm.NewNetworkFactory` adapter (capture singletons; bridge FactoryCtx→hcm.ListenerCtx; wrap `NewFilterWithCtxAndSinksAndRegistry`; shared-instance) + assertion + adapter tests (stat-registration parity; parse-reject pass-through) | §4.2 |
| 7 | `manager.go` retirement: DELETE `filterHandler`/`filterConstructor`/`filterRegistry`/`buildTerminalFilter`; collapse `chainInfo` (drop `filter`; generalize `netChainFactory` → `[]network.NetworkFilter`); resolve `listenerCtx` disposition (D-26.2-4) | §3.3 |
| 8 | `manager.go` unified chain builder: `buildNetworkChainFactory` resolves ALL filters against netReg + classifies + validates `[read*, terminal?]` shape (§6.2 rejects) + builds per-chain FactoryCtx; the unknown-type reject unifies (§6.1, byte-stable wording preserved) + build-path unit tests | §3.3 / §6 |
| 9 | `manager.go` `serveConnection` step-7 single branch → `serveNetworkChain` (pure-read read loop unchanged; terminal handoff; pure-terminal immediate `Handle`) + unit tests (echo/direct_response/tcp_proxy/hcm dispatch shapes; R3 old-path-equivalence) | §3.3 |
| 10 | Constructor + caller re-wiring: `netReg` required (drop nil-tolerance); the registration seam `RegisterBuiltins` (D-26.2-5) consumed by main.go + the thinner ctors + the admin/manager_test/main_test callers | §3.3 / §3.4 |
| 11 | Boot-wiring at `cmd/envoy-go/main.go`: register all four built-ins (echo/direct_response/tcp_proxy/hcm) via the seam + Freeze; live boot smoke (tcp_proxy + HCM through the unified dispatch) | §3.4 |
| 12 | BEHAVIOR_CONTRACT.md 26.2 bundle + ADR-0215 §Decision/§Consequences body landing (DECISIONS.md tail STAYS ADR-0215; no new number consumed at IMPL) + STATE.md re-advance + ROADMAP sub-row 26.2 stays in-progress→done at phase-done + six-gate verification (incl. the full differential R3 back-compat suite + conformance + h2spec) | §9 / §15 |

---

## 11. SPEC-time empirical-pin block (cross-reference parent §11 + the D-S1 sub-pin)

The 26.2 SPEC does NOT re-execute the parent §11 D1–D8 pins (resolved once at the parent SPEC; inherited here). The 26.2-additive empirical pin:

### 11.1 D-S1 — master-tip baselines VERIFIED at this SPEC commit

Verified via project-wide grep against master tip `f9e77ab` (the substantive 26.1-IMPL squash is `3ee1955`; the tip trails it by next-prompt repoint commits — no Go code changed since `3ee1955`, so no code-surface drift) at this SPEC session (the source of the §10 Task-1 first-action gate):

- **Fuzzer count = 35** (`git ls-files '*fuzz_test.go' | xargs grep -h "^func Fuzz" | wc -l` = 35; the 26.1 `FuzzNetworkFilterConfigParse` was the 35th). 26.2 adds 0 → stays **35**. (The 26.3 rbac config-parse fuzzer is the 36th.) NOTE: the parent SPEC §11.8/R6 pinned **34** — that was probed at the parent's pre-26.1 master tip (`d0c63d2`); the count is **35** now that 26.1's network fuzzer landed. No contradiction; 35 is the correct 26.2 baseline.
- **Differential fixture-dir count = 44**; **numbering tail = 0042** (26.1 landed 0040/0041/0042). 26.2 adds 0 → stays **44** (§8.3).
- **Stat surface = 132** (parent §7.3). 26.2 adds 0 → stays **132**.
- **DECISIONS.md tail = ADR-0214** at master tip; **next-free = ADR-0215**. THIS 26.2 SPEC commit DRAFTS the **ADR-0215 §Context** → DECISIONS.md tail advances to **ADR-0215**; next-free becomes **ADR-0216** (consumed at the 26.3 SPEC). The ADR-0215 §Decision/§Consequences bodies land at 26.2 IMPL (§12 Task 12) per ADR-0044 (no new number consumed at IMPL).
- **`manager.go` line anchors** (the §3.3 retirement targets; re-pinned at IMPL Task 1 against the IMPL-session tip): `filterHandler` @46; `filterConstructor` @97; `filterRegistry` @104; `chainInfo` @184; `buildTerminalFilter` @612; `buildNetChainFactory` @654; `serveConnection` @1045; step-7 dual-branch @1104-1109 (the `// (7) Dispatch:` comment @1104, the `if selected.netChainFactory != nil` branch @1105, the `selected.filter.Handle` call @1108); `serveReadFilterChain` @1126. The `NewManagerWithBaseDirAndAllowH2C` callers (re-wiring blast radius, Task 10): `internal/admin/admin_helpers_test.go`, `internal/admin/listeners_test.go`, `internal/listener/manager_test.go`, `cmd/envoy-go/main_test.go`, + `cmd/envoy-go/main.go`. (A `git grep` also hits `internal/drain/doc.go` — a doc-COMMENT reference, NOT a caller; do not chase it.)

The 26.2 IMPL Task-1 RE-RUNS these greps + line-anchor checks as a hard first-action gate (the master tip may advance between this SPEC commit and the IMPL session; the gate catches drift before the deltas are asserted).

---

## 12. SPEC-time D-questions for PLAN / IMPL resolution

Inherits the parent §12 D-questions (D-P1/D-P3/D-P4/D-P5 are 26.3 territory). The parent §3.2 terminal-fit D-question is RESOLVED at §3.1 (terminal-filter seam). 26.2-additive D-questions (each cross-referenced inline above):

- **D-26.2-1 (sealed vs open `NetworkFilter` marker).** §3.2. **Resolution at:** 26.2 PLAN/IMPL Task 2. **Anticipated:** sealed marker (in-package, type-safe, exhaustive dispatch).
- **D-26.2-2 (FactoryCtx field set).** §3.2. **Resolution at:** 26.2 PLAN Task 3. **Anticipated:** `BaseDir`(existing) + `HasTLS`/`AllowH2C`/`ListenerPrincipal`/`NodeServiceCluster`; heavy singletons closure-captured.
- **D-26.2-3 (buffered-prefix handover land at 26.2 vs defer to 26.3).** §3.2. **Resolution at:** 26.2 PLAN/IMPL Task 4. **Anticipated:** land at 26.2 (unit-tested via a synthetic always-Continue read filter), so the "mixed chains expressible" claim is proven; first production consumer = `rbac_network` 26.3.
- **D-26.2-4 (`listenerCtx` full-retire vs thin-retain).** §3.3. **Resolution at:** 26.2 IMPL Task 7. **Anticipated:** retire if all fields migrate cleanly to `FactoryCtx` + closures; retain as a thin per-chain carrier only if a residual manager-side use remains.
- **D-26.2-5 (registration seam location + test-caller wiring).** §3.3 / §3.4. **Resolution at:** 26.2 PLAN Task 10. **Anticipated:** a shared `RegisterBuiltins` helper (placement avoiding import cycles — likely `cmd/envoy-go/` or a tiny wiring package, NOT `network`/`listener`) + a test fixture; `netReg` becomes required (nil-tolerance dropped).
- **D-26.2-6 (PARSE-REJECT byte-stable wording + unknown-type wording preservation).** §6. **Resolution at:** 26.2 IMPL Task 8. **Anticipated:** preserve the existing `unknown filter type_url` wording byte-for-byte (R-S); finalize the NEW `terminal-not-last`/`multiple-terminals` arm wording + `TestParseRejectConstants_ByteStable`.
- **D-26.2-7 (import-cycle audit).** §3.4. **Resolution at:** 26.2 IMPL Task 5/6. **Anticipated:** `tcpproxy`/`hcm` import `network` (one-directional); `network` imports neither + no heavy packages (FactoryCtx is primitives-only); the `RegisterBuiltins` seam imports all filters + must live where no cycle forms. Verified by `go build ./...`.

---

## 13. RATIFIED-PENDING items (cross-reference parent §13 + sub-phase-specific)

- **R3 (back-compat — the migration proof; parent §13 R3).** The EXISTING `tcp_proxy` + HCM differential fixtures (`0000-tcp-echo`, `0002-tls-tcp`, the HCM/h2/wasm/lua fixtures) + conformance 10/10 + h2spec 53/53 stay byte-exact green after the migration. This is the LOAD-BEARING 26.2 gate (the deliberate-break migration proof). 26.2 IMPL runs the full differential suite + conformance + h2spec at the six-gate (NOT asserted-unaffected — the dispatch path genuinely changed for tcp_proxy/HCM, so these MUST be re-run live).
- **R-T (terminal-filter seam fidelity).** `*tcpproxy.Filter` + `*hcm.Filter` satisfy `network.TerminalFilter` with ZERO `Handle`-method changes (compile-time `var _ network.TerminalFilter` assertions); the connection-takeover loops are byte-identical. 26.2 IMPL verifies via the assertions + the R3 fixtures.
- **R-M (mixed read→terminal capability).** The `[read-filter*, terminal-filter]` chain + the buffered-prefix handover are proven at 26.2 by a unit test (synthetic always-`Continue` read filter draining nothing → recording terminal asserts it receives the buffered prefix THEN the live socket bytes) — the load-bearing capability test, since no 26.2 differential fixture exercises a Continue→terminal chain (§8.2). Readies `rbac_network` (26.3).
- **R-U (single dispatch path).** After 26.2 the `serveConnection` step-7 has ONE branch; `chainInfo` carries ONE filter field (`netChainFactory`); the `*network.Registry` is the SOLE network-filter registry; `filterRegistry`/`filterConstructor`/`filterHandler`/`buildTerminalFilter` are DELETED. 26.2 IMPL verifies via `git grep` (zero post-retirement references) + the build-path unit tests.
- **R-S (stat + fixture + fuzzer baselines + wording stability).** 26.2 IMPL Task-1 re-confirms fuzzers 35, fixtures 44, stat surface 132, DECISIONS.md tail ADR-0215; and the unknown-type-url reject wording is preserved byte-for-byte (§6.1).
- **R-A (stat-registration parity).** HCM's per-instance metrics are registered byte-identically post-migration (the same `*stats.Registry` flows through the adapter; only the call site moves). 26.2 IMPL verifies the stat surface stays 132 + the HCM metric names/types are unchanged.

---

## 14. BEHAVIOR_CONTRACT.md edit bundle (cross-reference parent §9 + §9 above)

ONE atomic bundle at 26.2 IMPL final task (§10 Task 12), per ADR-0052. The edits enumerated at §9: the framework-subsection update (TerminalFilter seam + `[read*, terminal?]` shape + unified dispatch); the mixed-chain-restriction LIFT (remove the 26.1-transitional departure record; add the chain-shape rejects); the hardcoded-registry-retirement structural note; the stat-surface-stays-132 + no-new-fixtures/fuzzers confirmation; the 26.3 forward-pointer. NO operator-visible behavior change is recorded (the migration is structural).

---

## 15. Test surface + 26.2 IMPL acceptance checklist

### 15.1 Test surface (per parent §14)

- **Layer A — unit tests** at `internal/filter/network/`: the `TerminalFilter` interface + `NetworkFilter` marker (compile-time + dispatch type-switch); the extended `FactoryCtx`; the unified `ChainRuntime` (`[read*, terminal?]` classification; the buffered-prefix handover via a synthetic always-`Continue` read filter + recording terminal — R-M; pure-terminal immediate handoff; pure-read 26.1 path unchanged). Plus `internal/filter/tcpproxy/` + `internal/filter/hcm/`: the `NewNetworkFactory` adapters (parse-reject pass-through byte-stable; shared-instance semantics; HCM FactoryCtx→ListenerCtx bridge + stat-registration parity).
- **Layer B — `manager.go` unified dispatch** unit tests: the `buildNetworkChainFactory` chain-shape validation (`[read*, terminal?]`; `terminal-not-last` + `multiple-terminals` rejects; unified unknown-type reject with preserved wording); `serveNetworkChain` dispatch shapes (echo/direct_response read loop; tcp_proxy/HCM immediate `Handle`; mixed handoff); the `netReg`-required re-wiring + `RegisterBuiltins` seam; zero post-retirement references to the deleted types (R-U).
- **Layer C — fuzz**: NO new fuzzer (§2.10); the existing 35 stay green; tcp_proxy/HCM config-parse coverage carries through the adapters.
- **Layer D — differential (the R3 back-compat gate)**: the FULL differential suite (44 fixtures) stays byte-exact green — especially `0000-tcp-echo` + `0002-tls-tcp` (tcp_proxy) + the HCM/h2/wasm/lua fixtures (HCM through the migrated dispatch) + the 26.1 0040/0041/0042. NO new fixture (§8). This is the load-bearing migration proof — run LIVE (not asserted-unaffected).
- **Layer E — race**: `go test -race -short ./internal/filter/network/... ./internal/listener/...` proves no data race in the unified dispatch (the registry is frozen + lock-free post-boot; the chain runner is single-goroutine-per-connection; the terminal-filter shared instances are conn-stateless).

### 15.2 Six-gate checklist (per phase-22/24/25/26.1 precedent)

`go build ./...` + `go vet ./...` + `golangci-lint run` + `go test -race -short ./...` + the FULL differential suite (44 fixtures, R3 back-compat — run live) + conformance 10/10 + h2spec 53/53 (re-run LIVE as the R3 gate — HCM dispatches through the migrated path, so these are NOT asserted-unaffected; they must pass live). Counts: fuzzers 35; fixtures 44; stat surface 132; DECISIONS.md tail ADR-0215.

### 15.3 26.2 IMPL acceptance checklist (parent §15 + sub-phase-specific)

1. `tcp_proxy` + HCM migrated onto `network.TerminalFilter` (Handle UNCHANGED; compile-time assertions) + registered in `*network.Registry` via the adapters (§4); R-T verified.
2. The hardcoded `manager.go` `filterHandler`/`filterConstructor`/`filterRegistry`/`buildTerminalFilter` RETIRED (zero post-retirement references — R-U); `chainInfo` collapsed to one filter field.
3. The dual-dispatch UNIFIED: `serveConnection` step-7 single branch; `buildNetworkChainFactory` the sole chain builder; the `*network.Registry` the sole network-filter registry (§3.3).
4. The `network-filter-mixed-chain-unsupported` 26.1-transitional reject LIFTED; mixed `[read*, terminal]` chains expressible (R-M unit-tested via the synthetic Continue filter + buffered-prefix handover); the NEW chain-shape rejects (`terminal-not-last`/`multiple-terminals`) land (§6.2).
5. `netReg` intrinsic (nil-tolerance dropped); the `RegisterBuiltins` seam consumed by main.go + the thinner ctors + the admin/manager_test/main_test callers (D-26.2-5); boot smoke green.
6. R3 back-compat: the FULL differential suite (44) + conformance (10/10) + h2spec (53/53) byte-exact green LIVE (the deliberate-break migration proof). Stat surface 132; fuzzers 35; fixtures 44 (all +0 — §2.7/§2.8/§2.10).
7. ADR-0215 §Decision/§Consequences bodies land (DECISIONS.md tail STAYS ADR-0215 — drafted at THIS SPEC; no new number consumed at IMPL); BEHAVIOR_CONTRACT.md 26.2 bundle lands (§14).
8. Six gates green (§15.2); STATE.md advanced to the 26.2 phase-done / 26.3-SPEC-awaiting state; ROADMAP sub-row 26.2 `in-progress → done`; parent row 26 STAYS `in-progress` (flips at 26.3 per the 18/19/22/24/25 ROLLUP precedent).

---

## Appendix A — Cross-references to parent SPEC

| 26.2 SPEC § | Parent SPEC § | Relationship |
|---|---|---|
| §1 Purpose | parent §1 + §3.2 (26.2 scope detail) | refines |
| §1.1 AMENDs | parent §1.1 (A11) + §11.5 | inherits the 26.2-relevant amendments |
| §2 Non-purposes | parent §2 + §3.2 | refines (26.2-scoped) |
| §3 Framework + dispatch | parent §3.2 (terminal-fit D-question RESOLVED) + §4.1 | refines + resolves |
| §4 Filter migrations | parent §3.2 + §2.3 (no package move) | refines |
| §5 Proto roster | parent §5 | INHERITS (unchanged — tcp_proxy/HCM parsing untouched) |
| §6 PARSE-REJECT | parent §6 + the 26.1 §6.2 mixed-chain LIFT | refines (lift + new chain-shape rejects) |
| §7 Stat surface | parent §7.1 | INHERITS (0 delta — stat-neutral migration) |
| §8 Fixtures | parent §8.2 (multi-chain fixture DEFERRED to 26.3) | refines (+0; back-compat proof) |
| §9 Behavior contract | parent §9 (26.2 bundle) | refines |
| §10 Tasks | parent §11.8 + §15 (26.2 row) | NEW (task spine) |
| §11 Empirical pins | parent §11 (D-S1 sub-pin only) | inherits; adds D-S1 baseline re-verify |
| §12 D-questions | parent §12 (terminal-fit resolved) | adds D-26.2-1..7 |
| §13 RATIFIED-PENDING | parent §13 (R3) | refines (R3/R-T/R-M/R-U/R-S/R-A scoped to 26.2) |

## Appendix B — Phase 26.2 ADR landing summary

- **ADR-0215** (`tcp_proxy` + HCM migration onto the read-filter framework + the `manager.go` hardcoded-registry retirement + the dispatch unification) — §Context DRAFTED into DECISIONS.md at THIS 26.2 SPEC commit (DECISIONS.md tail ADR-0214 → ADR-0215; next-free → ADR-0216); §Decision + §Consequences bodies land at 26.2 IMPL (§10 Task 12) per ADR-0044. Covers: the NEW `network.TerminalFilter` seam (connection-takeover model alongside the `ReadFilter` OnData model; the parent §3.2 terminal-fit D-question resolved in favor of the seam over an OnData rewrite — byte-exact parity over re-architecture); the generalized `FilterInstanceFactory`/`NetworkFilter` marker; the extended `FactoryCtx` (per-chain build fields; heavy singletons closure-captured); the unified `[read-filter*, terminal-filter?]` chain dispatch + the buffered-prefix handover; the `filterRegistry`/`filterConstructor`/`filterHandler`/`buildTerminalFilter` retirement; the `network-filter-mixed-chain-unsupported` LIFT + the NEW chain-shape rejects; the registration-seam re-shape (`netReg` intrinsic); the back-compat-via-existing-fixtures discipline (R3).
- DECISIONS.md tail = **ADR-0215** at 26.2 SPEC commit (this SPEC drafts the §Context); STAYS ADR-0215 at 26.2 phase-done (IMPL fills the §Decision/§Consequences bodies in place — no new number consumed). Next-free **ADR-0216** (consumed at the 26.3 SPEC for the `internal/rbac/` extraction — ADR-0216 + ADR-0217 connection-metadata sink + ADR-0218 `rbac_network`).
