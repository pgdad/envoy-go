# Phase 27 SPEC — `sni_cluster` network filter (`envoy.filters.network.sni_cluster`) at full upstream parity + the first connection-scoped upstream-cluster-override seam

> **For agentic workers:** the NEXT lifecycle step is `superpowers:writing-plans` (PLAN authoring; SKILL_ROUTING state 2 → 3). This SPEC is the input to that PLAN. Steps are NOT checkboxes here — the PLAN decomposes §10 into bite-sized TDD tasks.

**Goal:** Land `sni_cluster` — a config-less L4 read-filter that publishes the TLS SNI verbatim as a per-connection upstream-cluster-override — at full upstream parity, in ONE flat phase, with the terminal `tcp_proxy` made per-connection-cluster-resolving.

**Architecture:** A NARROW typed per-connection cluster-override carried on the per-connection `chainRuntime` (NOT a general filter-state primitive — Q2/YAGNI). `sni_cluster` writes it in `OnNewConnection` via a `ReadFilterCallbacks` setter; the framework threads it to the terminal `tcp_proxy` (which retains its `*cluster.Manager` + boot-resolved default cluster) at `Handle` dispatch via the call `ctx`. No new package; no new third-party dependency.

**Tech stack:** Go 1.26.2 / golangci-lint 1.64.8 (ADR-0009); reference Envoy v1.37.2 (ADR-0008); go-control-plane v1.32.4 (ADR-0008). Reuses `internal/filter/network/` (26.1/26.2 chain framework + `TerminalFilter` seam) + `internal/filter/tcpproxy/` + `internal/cluster/`.

---

## 1. Purpose / Mission

Phase 27 lands `sni_cluster` (`envoy.filters.network.sni_cluster`, proto `envoy.extensions.filters.network.sni_cluster.v3.SniCluster` — an EMPTY message), the family's **fourth concrete filter** and the **SECOND §9 Network-filters-family row** (a flat top-level row per ADR-0106; phase 26 was the anomalous pre-split family-parent). It is the FIRST §9 row to add a non-terminal read-filter that *steers* a downstream terminal filter, and the FIRST to make `tcp_proxy` cluster-selection per-connection.

This SPEC refines the phase-27 BRAINSTORM (`docs/envoy-go/phases/27-network-filter-sni-cluster/BRAINSTORM.md`, 3 Q-decisions) against the AS-BUILT 26.1/26.2/26.3 framework and the §10 D27-1..D27-8 empirical pins executed IN-SESSION against Envoy v1.37.2 (per ADR-0004). It anchors the ADR-0219 + ADR-0220 §Context drafts into DECISIONS.md (§Decision/§Consequences bodies land at 27 IMPL per ADR-0044).

### 1.1 Empirical-finding-driven scope (the §10 pins, summarized; full evidence in §11)

All eight pins are RESOLVED in-session against the v1.37.2 source + the vendored go-control-plane v1.32.4 binding:

- **F-SNI (D27-2/D27-6):** Envoy's `sni_cluster.cc::onNewConnection()` reads `requestedServerName()`; when the SNI is non-empty it `setData`s a `TcpProxy::PerConnectionCluster` (key literal `"envoy.tcp_proxy.cluster"`) carrying the SNI **verbatim**, `StateType::Mutable`, `LifeSpan::Connection`; it returns `FilterStatus::Continue` (always), and `onData()` is a no-op `Continue`. **→ envoy-go: write the override in `OnNewConnection`, return `Continue` (no sticky halt — `reference_network_read_filter_onnewconnection_halts`); `OnData` pass-through `Continue`.**
- **F-RESOLVE (D27-5):** `tcp_proxy`'s `Config::getRegularRouteFromEntries` reads the `PerConnectionCluster` filter-state key FIRST and, when present, returns a route wrapping that name — fully overriding the configured `cluster:`/`default_route_`; when absent it falls through to the configured cluster. **→ envoy-go: override present → override cluster; absent/empty → configured default (byte-exact with today).**
- **F-NOROUTE (D27-4):** an unknown (override) cluster → `cluster_manager_.getThreadLocalCluster(name)` null → `config_->stats().downstream_cx_no_route_.inc()` + `NoClusterFound` response-flag + `onInitFailure(NoRoute)` → `connection().close(NoFlush)`. **→ envoy-go: close the downstream connection (zero bytes); the `tcp.<stat_prefix>.downstream_cx_no_route` counter is NOT mirrored (the entire `downstream_cx_*` family is a pre-existing gap — §7.2); stat surface stays 136.**
- **F-WEIGHTED:** Envoy bypasses `PerConnectionCluster` when `weighted_clusters` is configured (`getRouteFromEntries` takes the weighted path). envoy-go's `tcp_proxy` PARSE-REJECTS `weighted_clusters` (phase 02; `filter.go:66`), so envoy-go only ever has the single-cluster path where Envoy DOES honor the override — **aligned by construction.**
- **F-EMPTY-PROTO (D27-1):** `SniCluster` is a field-less message; `@type` = `type.googleapis.com/envoy.extensions.filters.network.sni_cluster.v3.SniCluster` (verified via `proto.MessageName`, carrying the `extensions.` segment — `reference_network_filter_typeurl_extensions`).
- **F-NO-PER-ROUTE (D27-7):** the proto carries no per-route message; network filters have no `typed_per_filter_config` surface (§5 / §4).

### 1.2 ADR continuity + D-hypothesis disposition at SPEC commit

Next-free ADR at master tip is **ADR-0219** (DECISIONS.md tail ADR-0218). This SPEC drafts the §Context for **ADR-0219** (the override seam + `tcp_proxy` per-connection resolution) and **ADR-0220** (the `sni_cluster` filter); next-free after phase-27 IMPL phase-done ≈ **ADR-0221**. The ADR-0209 escape-valve reserve carried from the §9 family STANDS-UNCONSUMED.

---

## 2. Non-purposes

- **A general connection-scoped filter-state primitive** — DEFERRED (Q2 / YAGNI / extract-at-second-consumer). The narrow typed override generalizes when a second override-publishing consumer appears (API-revision allowance — ADR-0213 lineage).
- **Reusing the ADR-0217 dynamic-metadata bucket for the override** — REJECTED (Q2). Envoy uses filter-state (control) NOT dynamic-metadata (observability) for `envoy.tcp_proxy.cluster`; conflating them diverges from upstream semantics while still needing the same terminal-threading work.
- **Dynamic routing beyond SNI-verbatim** — weighted-cluster / `cluster_header` / metadata-driven cluster selection are NOT part of `sni_cluster` (which uses the SNI string verbatim as the cluster name). Deferred.
- **The `tcp_proxy` `downstream_cx_*` stat family** — NOT introduced here (pre-existing gap; §7.2).
- **The remaining L4 protocol proxies** (`redis`/`mongo`/`kafka_broker`/`thrift`/`zookeeper`) — each its own future §9 phase.
- **A write-filter chain** (`onWrite`/`WriteFilter`) — still deferred (phase-26 Q4 API-revision allowance).

---

## 3. The connection-scoped cluster-override seam (ADR-0219)

### 3.0 Split disposition — D27-8 RESOLVED (single phase)

ADR-0045 split-gate fires at `> ~25 tasks OR > ~1500 LoC`. Phase-27 surface:

| Unit | Anticipated net-new LoC |
|---|---|
| `snicluster` filter package (config-less parse + `OnNewConnection` + no-op `OnData`/`OnDestroy` + registration) | ~80–160 |
| The override seam (`chainRuntime` field + `SetUpstreamCluster` on `ReadFilterCallbacks` + concrete impl + the `UpstreamClusterOverride` context accessor + `handleTerminal` threading) | ~40–100 |
| `tcp_proxy` per-connection resolution (retain manager + default; override-then-fallback at `Handle`; unknown→close) | ~60–140 |

Net-new ~180–400 LoC, ~9–14 tasks — both axes comfortably under the gate. **Single flat phase 27 — no pre-split.** The PLAN re-checks the gate at PLAN time per ADR-0045.

### 3.1 The narrow per-connection override field + the writer accessor (D27-3 RESOLVED)

**The field.** `chainRuntime` (`internal/filter/network/chain.go:127`) gains one field:

```go
// upstreamClusterOverride is the per-connection upstream-cluster-override a
// read filter (sni_cluster, 27) publishes; "" = no override. It is the NARROW
// typed stand-in for Envoy's PerConnectionCluster filter-state entry (key
// "envoy.tcp_proxy.cluster"; ADR-0219) — NOT a general filter-state primitive
// (Q2/YAGNI). handleTerminal threads it to the terminal filter via the call ctx.
upstreamClusterOverride string
```

Zero value `""` = no override → the terminal uses its configured cluster (byte-exact back-compat). `OnDestroy` need not reset it (the runtime is per-connection, discarded after dispatch); the IMPL MAY reset for hygiene but it is not load-bearing.

**The writer accessor — ON the `ReadFilterCallbacks` interface (NOT a type-asserted sink).** Add to `ReadFilterCallbacks` (`callbacks.go:16`):

```go
// SetUpstreamCluster publishes a per-connection upstream-cluster-override that
// the terminal filter (tcp_proxy) consumes to replace its configured cluster
// (ADR-0219). The NARROW typed stand-in for Envoy's
// connection().streamInfo().filterState()->setData("envoy.tcp_proxy.cluster", …)
// (Q2 — no general filter-state primitive). Set by sni_cluster (27) in
// OnNewConnection with the verbatim SNI; "" is a no-op (leaves the configured
// cluster). Last writer wins (Envoy's PerConnectionCluster is Mutable).
SetUpstreamCluster(name string)
```

The concrete `*callbacks` (`chain.go:321`) implements it: `func (c *callbacks) SetUpstreamCluster(name string) { c.rt.upstreamClusterOverride = name }`.

**Why on-interface, not the `SetResponseCodeDetails` type-assert pattern.** The override is an **essential** control channel (the entire function of `sni_cluster`), not a best-effort internal detail like the `rcd` sink (`chain.go:351`, deliberately OFF the interface per 26.1 D-P26.1-5b because "no operator-visible surface emits it"). It mirrors `DynamicMetadata()` (`callbacks.go:25`, an essential per-connection channel placed ON the interface). On-interface gives compile-time safety (a drifting impl is a build error, not a silent runtime no-op) and forces every test double to implement it (a type-assert sink would silently no-op against a mock that omits it — a footgun for the filter whose whole job is the write). The rejected type-assert alternative is recorded in ADR-0219.

### 3.2 The terminal-reader threading — via the call `ctx` (D27-3 RESOLVED)

The terminal filter's `Handle(ctx context.Context, downstream net.Conn)` signature is FIXED by the `TerminalFilter` interface (`terminal.go:42`) and SHARED by HCM — it must NOT change. The override is threaded out-of-band on the call `ctx`:

- A NEW unexported context key type + an exported accessor in `internal/filter/network/`:

```go
type upstreamClusterKey struct{}

// WithUpstreamClusterOverride returns ctx carrying override for the terminal
// filter to read via UpstreamClusterOverride. Internal to the framework's
// terminal handoff; not part of the public filter API.
func withUpstreamClusterOverride(ctx context.Context, override string) context.Context { … }

// UpstreamClusterOverride returns the per-connection upstream-cluster-override
// a read filter published (ADR-0219), or ("", false) if none. tcp_proxy reads
// it at Handle to override its configured cluster.
func UpstreamClusterOverride(ctx context.Context) (string, bool) { … }
```

- `handleTerminal` (`chain.go:209`) wraps the ctx ONLY when an override is set, before calling `Handle`:

```go
func (rt *chainRuntime) handleTerminal(ctx context.Context) {
    conn := rt.conn
    if rt.buf.Len() > 0 { /* prefixConn replay — unchanged (R-M) */ }
    if rt.upstreamClusterOverride != "" {
        ctx = withUpstreamClusterOverride(ctx, rt.upstreamClusterOverride)
    }
    rt.terminal.Handle(ctx, conn)
}
```

**Why `ctx`, not a signature change or a terminal-side callbacks accessor.** (a) `Handle`'s signature is shared by HCM — changing it ripples to HCM + the `TerminalFilter` interface for a feature only `tcp_proxy` consumes. (b) `tcp_proxy`'s terminal `*Filter` is a SHARED boot instance (`NewNetworkFactory` yields one `*Filter` per chain — `filter.go:81`), so it has no per-connection handle to hang an accessor on; the only per-connection inputs it gets are `ctx` + `conn`. (c) `context.Value` is Go's idiomatic per-call out-of-band channel and is naturally connection-scoped here. HCM ignores the override (an `sni_cluster`→HCM chain is not a meaningful upstream combination; HCM does its own routing) — correct and harmless. Recorded in ADR-0219.

### 3.3 `tcp_proxy` per-connection cluster resolution (ADR-0219; D27-4/D27-5)

`internal/filter/tcpproxy/filter.go` changes from boot-static to per-connection resolution while keeping the no-override path byte-exact:

- **`Filter` struct:** replace `cluster *cluster.Cluster` with `cm *cluster.Manager` + `defaultCluster *cluster.Cluster` (the boot-resolved configured cluster) — `statPrefix`/`dm` unchanged.
- **`NewFilter` / `NewNetworkFactory`:** UNCHANGED parse + boot-time resolution of the configured `cluster:` (the existing unknown-configured-cluster boot-reject `tcpproxy: cluster %q not found` stays byte-exact — `filter.go:62`); store `cm` + the resolved cluster as `defaultCluster`. `weighted_clusters` PARSE-REJECT unchanged (`filter.go:66`) — keeps envoy-go on the single-cluster path where the override always applies (F-WEIGHTED).
- **`Handle`:** before the Dial, resolve the effective cluster:

```go
eff := f.defaultCluster
if override, ok := network.UpstreamClusterOverride(ctx); ok && override != "" {
    c, found := f.cm.Get(override)
    if !found {
        // F-NOROUTE: unknown override cluster → close downstream, zero bytes.
        // (Envoy: downstream_cx_no_route + NoFlush close; the counter is NOT
        //  mirrored — §7.2. The deferred downstream.Close() yields zero-byte
        //  parity on the wire regardless of FIN-vs-RST.)
        log.Printf("tcpproxy: per-connection override cluster %q not found", override)
        return
    }
    eff = c
}
// … dial eff instead of f.cluster …
```

- **Precedence (F-RESOLVE):** override present & non-empty → override cluster (even if it equals the configured name → `cm.Get` resolves to the same cluster — no special case). Override absent/empty → `defaultCluster` → byte-exact with today. (The `OnNewConnection` `sni != ""` guard (§4.2) and the `Handle` `override != ""` guard are REDUNDANT-BY-DESIGN — `sni_cluster` only ever sets a non-empty override, but the `Handle` guard is defense-in-depth so an empty override can never route to a literal `""` cluster; the PLAN keeps both deliberately.)
- **Back-compat:** every existing `tcp_proxy` fixture (`0000`/`0001`/`0002` + the 26.x network fixtures) has no `sni_cluster` → no override on the ctx → `defaultCluster` → byte-exact green. These are the strongest regression proof of the per-connection-resolution change (deliberate-break discipline).

---

## 4. The `sni_cluster` filter (ADR-0220)

### 4.1 Package + parse + registration

NEW `internal/filter/network/snicluster/` (single-token sub-package, `echo`/`directresponse` precedent). Mirrors `echo` (config-less):

- **`TypeURL`** — derived/verified via `proto.MessageName(&snicluster.SniCluster{})`, NOT hand-typed (`reference_network_filter_typeurl_extensions`): `type.googleapis.com/envoy.extensions.filters.network.sni_cluster.v3.SniCluster` (F-EMPTY-PROTO; §11 D27-1).
- **`New(tc *anypb.Any, _ network.FactoryCtx) (network.FilterInstanceFactory, error)`** — config-less: accept an empty/absent `typed_config`; if `len(tc.GetValue()) > 0` `UnmarshalTo(&snicluster.SniCluster{})` (an empty message — any bytes that unmarshal are accepted; a malformed Any surfaces `sni_cluster: invalid typed_config: %w`, the `echo.New` shape). Returns `func() network.NetworkFilter { return &filter{} }`. Blank-import the proto where boot-wiring needs the descriptor registered (the differential bootstrap needs a cluster — the fixture supplies one).
- **Registration** — the **6th** `builtins.RegisterBuiltins` entry (`internal/filter/network/builtins/builtins.go`): `reg.Register(snicluster.TypeURL, snicluster.New)`. No `Deps` needed (config-less, no boot singletons — like `echo`/`direct_response`). Mirror the parallel registration in `cmd/envoy-go/main.go` if it lists them explicitly. Registration order does not affect runtime behavior (ADR-0072).

### 4.2 `OnNewConnection` SNI→override + `Continue` (F-SNI; D27-2/D27-6)

```go
type filter struct {
    network.Marker
    cb network.ReadFilterCallbacks
}

func (f *filter) OnNewConnection() network.Status {
    if sni := f.cb.Connection().RequestedServerName(); sni != "" {
        f.cb.SetUpstreamCluster(sni) // verbatim — no transform (F-SNI)
    }
    return network.Continue // MUST Continue — a StopIteration sets sticky connHalted
}

func (f *filter) OnData(*network.Buffer, bool) network.Status { return network.Continue } // pass-through
func (f *filter) SetReadFilterCallbacks(cb network.ReadFilterCallbacks) { f.cb = cb }
func (f *filter) OnDestroy() {}
```

- **Verbatim** SNI as the override (F-SNI — no lowercasing/trimming/truncation).
- **`Continue`, not `StopIteration`** — an `OnNewConnection` `StopIteration` would set the chain's sticky `connHalted` flag and block ALL `OnData` (`reference_network_read_filter_onnewconnection_halts`; `chain.go:233`). Envoy's filter also returns `Continue` (F-SNI) — so parity and the framework constraint coincide.
- **`OnData` returns `Continue`** (NOT `StopIteration`, NOT a drain) — `sni_cluster` does not inspect payload; it passes bytes through so the chain reaches the terminal (`echo` halts/drains; `sni_cluster` does not). This makes `[sni_cluster, tcp_proxy]` a **mixed read→terminal chain** (the second production mixed chain after 26.3 `rbac_network`): `sni_cluster` `Continue`s from `OnNewConnection`, `TerminalReady()` fires, `HandleTerminal` runs (the `prefixConn` replays any pre-terminal bytes — R-M). Empty/absent SNI → no override set → the terminal uses its configured cluster (fallback).

### 4.3 The mixed-chain path (R-M, second production consumer)

`[sni_cluster, tcp_proxy]`: `sni_cluster` (read) sets the override in `OnNewConnection` and `Continue`s → `serveNetworkChain`'s post-`OnNewConnection` `TerminalReady()` check (`manager.go:1046`) fires → `HandleTerminal` threads the override via ctx (§3.2) → `tcp_proxy.Handle` resolves it (§3.3). For TLS chains, the SNI is populated by the TLS handshake / tls_inspector (the `ConnFacts.ServerName` path, `manager.go:1027`), so `RequestedServerName()` returns it in `OnNewConnection`. No buffered prefix is expected (the override is set before any data), so `handleTerminal`'s `prefixConn` branch is typically not taken — but the path is unchanged and correct if bytes arrive first.

---

## 5. Proto-field roster

`envoy.extensions.filters.network.sni_cluster.v3.SniCluster` — **EMPTY message** (zero data fields; only a `previous_message_type` versioning annotation; F-EMPTY-PROTO / §11 D27-1). No field roster; no PGV constraints; no per-route message (F-NO-PER-ROUTE / D27-7). The `tcp_proxy` `TcpProxy` roster is UNCHANGED (phase 02) — phase 27 changes only `tcp_proxy`'s *resolution* of its already-parsed `cluster:` field, not the proto surface.

---

## 6. PARSE-REJECT roster

### 6.1 No new boot-reject arm — `sni_cluster` is config-less

The `SniCluster` proto has no fields → nothing to reject at boot. `sni_cluster.New` accepts empty/absent `typed_config` (mirrors `echo.New`); only a structurally-malformed `Any` body that fails `UnmarshalTo` surfaces `sni_cluster: invalid typed_config: %w` (the `echo`-shape defensive error). **No boot-reject differential fixture** (§8.2).

### 6.2 `tcp_proxy` boot-rejects UNCHANGED

The configured-cluster-not-found boot-reject (`tcpproxy: cluster %q not found`) and the `weighted_clusters` reject stay byte-exact (§3.3). Phase 27 adds NO new `tcp_proxy` boot-reject.

### 6.3 Back-compat as the regression gate

The no-override path (no `sni_cluster` in chain, OR empty SNI) MUST be byte-exact with master tip — proven by the existing `tcp_proxy` fixtures staying green (§3.3, §8.3).

---

## 7. Stat surface

### 7.1 `sni_cluster` emits no stats — surface stays 136 (+0)

Upstream `sni_cluster` has no stats (F-SNI — the filter only sets filter-state). envoy-go mirrors: NO built-in counters. Project stat surface stays **136**.

### 7.2 The unknown-override path does NOT add `downstream_cx_no_route` (D27-4 RESOLVED)

Envoy increments `tcp.<stat_prefix>.downstream_cx_no_route` on the unknown-cluster path (F-NOROUTE). envoy-go's `tcp_proxy` emits **no `downstream_cx_*` counters at all** today (verified: `internal/filter/tcpproxy/` has no stat emission). Adding a single `downstream_cx_no_route` only on the override-miss path would be an inconsistent partial of an absent family. **Decision: DEFER the whole `downstream_cx_*` family (+0); record `downstream_cx_no_route` as a known-unmirrored upstream counter — a pre-existing coverage boundary NOT introduced by phase 27.** The unknown-cluster behavior (downstream close / zero bytes) IS proven cross-side byte-exact (§8.1 arm 3); the stat absence is invisible to a response-body differential. The IMPL re-confirms the surface stays 136.

### 7.3 Stat baseline re-confirm

IMPL Task-1 re-confirms stat surface **136** (grep) at the IMPL-session tip.

---

## 8. Differential fixture taxonomy (+1)

### 8.1 The `0045-sni-cluster` cross-side fixture (3 arms)

ONE new cross-side multi-cluster TLS dir (next-free dir at master tip = `0045`; tail is `0044-network-rbac-boot-reject`; the PLAN re-pins the number against the IMPL-session tip). Multi-listener/multi-SNI TLS shape (template: `0002-tls-tcp` — `driver/`, `envoy.yaml`, `envoy-go.yaml`, `expectations.yaml`, `pki/`, `README.md`; clusters named verbatim after SNI values; distinct per-backend sentinels prove routing). All three arms cross-side byte-exact via the differential runner:

1. **Route arm** — SNI `foo.example.com` → `sni_cluster` sets override `foo.example.com` → `tcp_proxy` routes to cluster `foo.example.com` (distinct backend sentinel). Proves SNI→override→route.
2. **Fallback arm** — empty/absent SNI (or a non-SNI connection on a chain whose `tcp_proxy` has a configured cluster) → no override → `tcp_proxy`'s configured fallback cluster. Proves empty-SNI fallback (F-RESOLVE).
3. **Unknown-cluster-close arm** — SNI `unknown.example.com` naming a cluster that does NOT exist → override miss → downstream connection close, zero bytes (F-NOROUTE). Both subject and reference close with zero application bytes → byte-exact body comparison.

Per `reference_differential_fixture_dispatch_constraint` (one fixture dir = ONE runner branch — cross-side XOR boot-reject), all three arms are cross-side; there is NO boot-reject arm, so a SINGLE cross-side dir holds all three (multiple listeners/SNIs within the one dir). Per `reference_differential_asserter_dispatch`, any subject-side-only assertion uses `StatsAsserter` (NOT `SubjectAsserter`) and MUST be proven live — but the three arms are wire-byte-exact via the standard body comparison, so no subject-only assertion is required for the core proof (the IMPL confirms; if a `StatsAsserter` arm is added it must be proven non-vacuous per the memory).

### 8.2 No boot-reject fixture

`sni_cluster` is config-less — nothing to reject at boot (§6.1). No `00XX-sni-cluster-boot-reject` dir.

### 8.3 Back-compat fixtures (the regression gate)

The existing `tcp_proxy` fixtures (`0000-tcp-echo`/`0001-tcp-proxy-rr`/`0002-tls-tcp` + the 26.x network fixtures) stay byte-exact green under the per-connection-resolution change (§3.3) — the strongest proof the change is non-regressive.

### 8.4 Total fixture-dir count

Anticipated +1: fixtures **46 → 47** (one cross-side multi-arm dir). No conformance harness seeded (validated by the differential + back-compat fixtures).

---

## 9. Behavior-contract delta (the 27 bundle)

At IMPL Task (final), `docs/envoy-go/BEHAVIOR_CONTRACT.md` gains:

- A NEW `### envoy.filters.network.sni_cluster` subsection — proto `…sni_cluster.v3.SniCluster` (EMPTY); `OnNewConnection` reads SNI → publishes verbatim as the per-connection upstream-cluster-override → `Continue`; `OnData` pass-through; empty/absent SNI → no override; the 6th built-in; no stats; no per-route surface.
- A `tcp_proxy` per-connection-resolution amendment to the existing network-filter section — `tcp_proxy` resolves `override (PerConnectionCluster-equivalent) → cm.Get(override)` (unknown → downstream close, zero bytes) `else → configured cluster`; back-compat byte-exact when no override.
- An envoy-go-strict / coverage-boundary record: `tcp.<stat_prefix>.downstream_cx_no_route` (and the wider `downstream_cx_*` family) is a known-unmirrored upstream counter (§7.2); the narrow typed override is the envoy-go stand-in for Envoy's `envoy.tcp_proxy.cluster` filter-state key (no general filter-state primitive — Q2).

---

## 10. Per-task structure (~9–14 tasks; PLAN decomposes)

Indicative spine for the PLAN (TDD per task; per-task `gofmt -l` + `golangci-lint` on touched pkgs per `feedback_pertask_gofmt_lint`; subagents commit LOCAL-ONLY per `feedback_subagents_no_push`):

| # | Task | SPEC anchor |
|---|---|---|
| 1 | First-task baselines/anchors gate: re-confirm fuzzers **36** (the project's canonical fuzzer-accounting per STATE.md — NOT a raw `grep '^func Fuzz'`, which over-counts grouped fuzzers; +0 this phase) + fixtures **46** (tail 0044) + stat surface **136** + DECISIONS.md tail **ADR-0218** (this SPEC drafts 0219/0220) via grep; re-confirm `proto.MessageName(&snicluster.SniCluster{})` + the as-built line anchors (`chain.go:127` chainRuntime, `callbacks.go:16` ReadFilterCallbacks, `chain.go:209` handleTerminal, `filter.go:26/47/94` tcp_proxy struct/NewFilter/Handle) against the IMPL-session tip | §11 / §3 |
| 2 | The override field + `SetUpstreamCluster` on `ReadFilterCallbacks` + concrete `*callbacks` impl (TDD: a filter sets it; the runtime carries it) | §3.1 |
| 3 | The `UpstreamClusterOverride` context accessor + `handleTerminal` threading (TDD: handleTerminal wraps ctx iff override set; accessor round-trips; absent → (\"\", false)) | §3.2 |
| 4 | `tcp_proxy` per-connection resolution: retain `cm` + `defaultCluster`; `Handle` override-then-fallback + unknown→close (TDD: override routes; empty→default; unknown→zero-byte close; **back-compat unit tests** prove the no-override path is unchanged) | §3.3 |
| 5 | `internal/filter/network/snicluster/` package: config-less parse (`TypeURL` via `proto.MessageName`; accept empty/any) + `OnNewConnection` SNI→`SetUpstreamCluster`+`Continue` + no-op `OnData`/`OnDestroy` (TDD; prove the `SetUpstreamCluster` call is LIVE) | §4.1/§4.2 |
| 6 | Registration as the 6th built-in (`builtins.RegisterBuiltins` + main.go parity) + boot smoke (TDD) | §4.1 |
| 7 | The `0045-sni-cluster` cross-side TLS fixture (3 arms: route / fallback / unknown-close) + driver | §8.1 |
| 8 | Back-compat differential re-verify (the existing `tcp_proxy` fixtures stay byte-exact) + the new fixture green | §8.3 |
| 9 | Completion bundle: BEHAVIOR_CONTRACT 27 subsection + the `tcp_proxy` amendment + ADR-0219/0220 §Decision/§Consequences bodies (ADR-0044 in-place) + STATE/ROADMAP phase-done (row 27 `in-progress → done`) + the six-gate evidence | §9 / §15 |

Fuzzer (D27-8): **DEFER** — `sni_cluster`'s parse is byte-identical to `echo` (config-less, accept empty/any), and `echo` carries NO dedicated fuzzer (the network config-parse fuzzer `FuzzNetworkFilterConfigParse` lives in `directresponse`, which parses a real `DataSource` with file/env IO worth fuzzing). Fuzzers stay **36**. (If the PLAN/IMPL elects to add `FuzzSniClusterConfigParse` anyway it becomes the 37th — but the recommendation is DEFER.)

---

## 11. SPEC-time empirical-pin block (D27-1..D27-8 — executed IN-SESSION vs Envoy v1.37.2 + go-control-plane v1.32.4)

All pins resolved this SPEC session (ADR-0004). C++ evidence fetched from `github.com/envoyproxy/envoy` at tag **v1.37.2**; Go-binding evidence from the vendored go-control-plane v1.32.4.

- **D27-1 (proto + `@type`) — RESOLVED.** `envoy/extensions/filters/network/sni_cluster/v3/sni_cluster.pb.go`: `type SniCluster struct{ … }` has only `protoimpl` internal fields (zero data fields). `proto.MessageName(&snicluster.SniCluster{})` = `envoy.extensions.filters.network.sni_cluster.v3.SniCluster` (run in-session) → `@type` = `type.googleapis.com/envoy.extensions.filters.network.sni_cluster.v3.SniCluster` (the `extensions.` segment, per `reference_network_filter_typeurl_extensions`). The `api/.../sni_cluster.proto` `message SniCluster` carries only `previous_message_type` — empty.
- **D27-2 (SNI→cluster mechanism) — RESOLVED.** `source/extensions/filters/network/sni_cluster/sni_cluster.cc` `onNewConnection()`: `absl::string_view sni = read_callbacks_->connection().requestedServerName();` then `if (!sni.empty()) { read_callbacks_->connection().streamInfo().filterState()->setData(TcpProxy::PerConnectionCluster::key(), std::make_unique<TcpProxy::PerConnectionCluster>(sni), Mutable, LifeSpan::Connection); }`. Key literal `"envoy.tcp_proxy.cluster"` (`source/common/tcp_proxy/tcp_proxy.cc` `PerConnectionCluster::key()` → `CONSTRUCT_ON_FIRST_USE(std::string, "envoy.tcp_proxy.cluster")`). SNI stored verbatim (`PerConnectionCluster(absl::string_view cluster) : cluster_(cluster) {}`).
- **D27-3 (seam shape) — RESOLVED** (project-design, not Envoy): writer = `SetUpstreamCluster(name string)` ON the `ReadFilterCallbacks` interface (§3.1, on-interface rationale vs the `rcd` type-assert); reader-threading = `chainRuntime.upstreamClusterOverride` → `handleTerminal` wraps the call `ctx` → `tcp_proxy.Handle` reads `network.UpstreamClusterOverride(ctx)` (§3.2). `TerminalFilter.Handle` signature unchanged (no HCM ripple).
- **D27-4 (unknown cluster) — RESOLVED.** `tcp_proxy.cc` `establishUpstreamConnection()`: `getThreadLocalCluster(cluster_name)` null + no ODCDS → `config_->stats().downstream_cx_no_route_.inc();` + `setResponseFlag(NoClusterFound)` + `onInitFailure(NoRoute)`; `onInitFailure` → `connection().close(ConnectionCloseType::NoFlush, …TcpProxyInitializationFailure…)`. Stat defined by `ALL_TCP_PROXY_STATS(COUNTER…) COUNTER(downstream_cx_no_route)` (`tcp_proxy.h`), emitted on scope `tcp.<stat_prefix>` (`createScope(fmt::format("tcp.{}", config.stat_prefix()))`). → envoy-go: close downstream / zero bytes; counter NOT mirrored (§7.2).
- **D27-5 (precedence + fallback) — RESOLVED.** `tcp_proxy.cc` `Config::getRegularRouteFromEntries(connection)`: reads `filterState()->getDataReadOnly<PerConnectionCluster>(PerConnectionCluster::key())` FIRST; if non-null returns `SimpleRouteImpl(*this, per_connection_cluster->value())` (overrides configured cluster); else falls through to `default_route_` (built from `config.cluster()` at construction) — or `nullptr`. Override==configured resolves identically (same name → same cluster). Empty SNI → `sni_cluster` never sets the key → absent → fallback.
- **D27-6 (`onNewConnection` + `Continue`) — RESOLVED.** `sni_cluster.cc` `onNewConnection()` ends `return Network::FilterStatus::Continue;` (always — the empty-SNI case skips only the `setData`, not via early-return). `sni_cluster.h` `onData(Buffer&, bool) override { return Network::FilterStatus::Continue; }` (no-op). → satisfies the envoy-go `OnNewConnection`-must-Continue constraint naturally.
- **D27-7 (no per-route) — RESOLVED.** `sni_cluster.pb.go` has no per-route / `*PerRoute` message; network filters carry no `typed_per_filter_config` surface (consistent with the phase-26 §4 confirmation). The ADR-0125 canonical-per-route-shape roster is untouched.
- **D27-8 (envelope + fuzzer) — RESOLVED.** Single phase under ADR-0045 (§3.0: ~180–400 LoC / ~9–14 tasks). Fuzzer DEFERRED (echo-parity config-less parse; §10) — fuzzers stay 36.

### 11.1 As-built code-surface VERIFIED this SPEC session

- `internal/filter/network/chain.go` — `chainRuntime` struct (`:127`); `handleTerminal` (`:209`, prefixConn replay at `:211`); `callbacks` impl (`:321`); the `SetResponseCodeDetails` type-assert sink (`:351`, the rejected-pattern reference).
- `internal/filter/network/callbacks.go` — `ReadFilterCallbacks` (`:16`, with `DynamicMetadata()` the on-interface essential-channel precedent); `Connection.RequestedServerName()` (`:69`); `CloseType` (`:40`).
- `internal/filter/network/terminal.go` — `TerminalFilter.Handle(ctx, net.Conn)` (`:42`), the FIXED shared-with-HCM signature.
- `internal/filter/tcpproxy/filter.go` — `Filter{cluster,…}` (`:26`); `NewFilter` boot resolution + reject wording (`:47`); `NewNetworkFactory` shared instance (`:81`); `Handle` (`:94`).
- `internal/filter/network/builtins/builtins.go` — `RegisterBuiltins` (5 built-ins; `sni_cluster` = 6th); `echo.New`/`directresponse.New` the config-less template.
- `internal/listener/manager.go` — `serveNetworkChain` (`:1025`); the post-`OnNewConnection` `TerminalReady` mixed-chain handoff (`:1046`); `ConnFacts.ServerName` SNI plumbing (`:1027`).
- Counts at master tip `24b096c`: fuzzers **36**, fixtures **46** (tail `0044-network-rbac-boot-reject`), stat surface **136**, DECISIONS.md tail **ADR-0218** (next-free **ADR-0219**).

---

## 12. SPEC-time D-questions for PLAN / IMPL resolution

- **D27-S1** — exact `0045` fixture dir number (re-pin against the IMPL-session tip; +1 expected) + whether the fallback arm is best expressed as an empty-SNI TLS connection or a no-`sni_cluster` chain (PLAN picks the cleanest wire-deterministic shape). The 3 arms MAY be one or two listeners within the single dir; the dir count stays +1 regardless.
- **D27-S2** — whether `cmd/envoy-go/main.go` lists the network built-ins explicitly (requiring a parallel 6th-registration edit) or delegates wholly to `builtins.RegisterBuiltins` (IMPL Task-1 greps to confirm the single insertion point — ADR-0072 makes order behavior-neutral).
- **D27-S3** — the unknown-override-close on the terminal side: the deferred `downstream.Close()` (FIN) yields zero-byte body parity vs Envoy's `NoFlush` (RST). IMPL confirms the differential is byte-exact on bodies; if a test needs RST-vs-FIN distinction it is out of scope (the body differential does not observe it). NO `SO_LINGER` plumbing on the terminal `Handle` path (that framework `NoFlush` plumbing is for the read-filter close site, not the terminal-owned conn).
- **D27-S4** — fuzzer: confirm DEFER at IMPL (recommendation §10); fuzzers stay 36 unless IMPL elects to add `FuzzSniClusterConfigParse`.

---

## 13. RATIFIED-PENDING items

- **R-OVERRIDE** (the narrow override field + `SetUpstreamCluster` on-interface + the ctx threading + `tcp_proxy` per-connection resolution) — ADR-0219; ratified by the 3-arm cross-side fixture + the back-compat regression gate.
- **R-SNI** (the `sni_cluster` filter: config-less parse, `OnNewConnection` SNI→override→`Continue`, the 6th built-in) — ADR-0220; ratified by the route/fallback/unknown arms.
- **R-BACKCOMPAT** (no-override path byte-exact) — ratified by the existing `tcp_proxy` fixtures staying green.
- **R-MIXED-2** (`[sni_cluster, tcp_proxy]` is the SECOND production mixed read→terminal chain; the first config-less read filter that `Continue`s to a terminal) — exercised LIVE by the 0045 route/fallback arms.

---

## 14. BEHAVIOR_CONTRACT.md edit bundle (lands at IMPL final task)

See §9. Net: one NEW `### envoy.filters.network.sni_cluster` subsection + a `tcp_proxy` per-connection-resolution amendment + the `downstream_cx_*`-unmirrored coverage-boundary record + the narrow-override-vs-filter-state stand-in note.

---

## 15. Test surface + 27 IMPL acceptance checklist

### 15.1 Test surface

- Unit: the override field + `SetUpstreamCluster` round-trip; the `UpstreamClusterOverride` ctx accessor (present/absent); `handleTerminal` wraps-iff-set; `tcp_proxy` Handle override/fallback/unknown-close + **back-compat no-override**; `sni_cluster` parse (empty/any/malformed) + `OnNewConnection` SNI→override (live-assert) + empty-SNI no-op + `OnData` pass-through; registration smoke.
- Differential: `0045-sni-cluster` (3 arms) byte-exact; the existing `tcp_proxy` fixtures byte-exact (back-compat); the full suite 46 → 47.
- Byte-stable: the `sni_cluster:`/`tcpproxy:` error wording (no NEW reject const, but the existing `tcpproxy: cluster %q not found` stays byte-stable).

### 15.2 Six-gate checklist (phase-22/24/25/26.x precedent)

`go build` / `go vet` / `golangci-lint` clean; `go test ./... -race -short` green; the FULL differential suite byte-exact (47/47, incl. the back-compat `tcp_proxy` dirs + the new `0045`); h2spec 53/53 + proxy-wasm conformance 10/10 re-run LIVE (asserted-unaffected — phase 27 touches no HTTP/h2/proxy-wasm path — but re-confirmed since the harness is available). All outputs quoted into PROGRESS.md (run honestly).

### 15.3 27 IMPL acceptance checklist

- [ ] The override seam (field + on-interface `SetUpstreamCluster` + ctx accessor + `handleTerminal` threading) lands; `tcp_proxy` resolves per-connection (override/fallback/unknown-close); no-override byte-exact.
- [ ] `sni_cluster` lands config-less, `OnNewConnection` SNI→override→`Continue`, 6th built-in; `OnData` pass-through; empty-SNI no-op.
- [ ] `0045-sni-cluster` 3-arm cross-side fixture green; back-compat `tcp_proxy` fixtures green; suite 46 → 47.
- [ ] Stat surface 136 (+0); fuzzers 36 (+0 unless §10 elected); ADRs +2 (0219/0220 bodies in place; DECISIONS.md tail → ADR-0220; next-free → ADR-0221).
- [ ] BEHAVIOR_CONTRACT 27 bundle; STATE/ROADMAP row 27 `in-progress → done`; six gates GREEN LIVE quoted into PROGRESS.md.

---

## 16. Stage-close handoff

SPEC stage closes with the `spec-document-reviewer` loop (per the brainstorming/SPEC discipline) → STATE advance (`27 SPEC done`; next-skill `superpowers:writing-plans` for the PLAN) + ROADMAP row 27 stays `in-progress` (flips at IMPL phase-done) + commit + push (`feedback_push_to_origin`). Next lifecycle step: **27 PLAN** (`superpowers:writing-plans` → `PLAN.md`; ADR-0045 split-gate re-check at PLAN time).

---

## Appendix A — Phase 27 ADR landing summary

- **ADR-0219** — the connection-scoped upstream-cluster-override seam (the narrow typed `chainRuntime` field keyed by the upstream-canonical `envoy.tcp_proxy.cluster`; the `SetUpstreamCluster` on-interface writer; the `UpstreamClusterOverride` ctx-threaded reader) + the `tcp_proxy` per-connection cluster resolution (override-then-fallback; unknown→close; the `downstream_cx_no_route`-unmirrored decision; back-compat-via-existing-fixtures) + the no-general-primitive / API-revision-allowance clause. §Context drafted at this SPEC; §Decision/§Consequences at 27 IMPL (ADR-0044).
- **ADR-0220** — the NEW `sni_cluster` filter (config-less parse; `OnNewConnection` SNI-verbatim→override + `Continue`; no-op `OnData`/`OnDestroy`; 6th built-in; empty-SNI→fallback + unknown-cluster→close parity; the second production mixed read→terminal chain). §Context drafted at this SPEC; §Decision/§Consequences at 27 IMPL (ADR-0044).
