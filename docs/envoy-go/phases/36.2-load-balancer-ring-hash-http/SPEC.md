# Phase 36.2 SPEC — the HTTP route `hash_policy` producer plane (`RouteAction.hash_policy`): the SECOND producer feeding the ALREADY-LANDED `ring_hash` LB seam (ADR-0235) — the consistent-hash key computed per HTTP request from a per-route policy list (header `xxHash64` + connection-properties `source_ip`), folded `rotl64(prev,1)^new` with a `terminal` short-circuit, threaded via `cluster.WithHashKey`; the SECOND leg of the CONSUMED 36.1/36.2 by-plane split

> **For agentic workers:** the NEXT lifecycle step is `superpowers:writing-plans` (PLAN authoring; SKILL_ROUTING state 2 → 3) for the **36.2** sub-phase. This SPEC is the input to that PLAN. Steps are NOT checkboxes here — the PLAN decomposes §10 into bite-sized TDD tasks. 36.2 is a **PRODUCER-ONLY** leg: the `ringHashLB` policy, the ADR-0232/0235 seam (`Pick(hashKey, hasHash)` + `WithHashKey`/`hashKeyFrom`), the manager gate, the 3 `ring_hash_lb.*` gauges, and the exported `cluster.HashSourceIP`/`cluster.WithHashKey` helpers are ALL LANDED at 36.1 (master tip `e8e8c48`). 36.2 adds ONLY the HTTP-side key producer (the route `hash_policy` parse + per-request compute + the `WithHashKey` stuff at the router's dial sites) + its differential fixture. The ADR-0045 split is already CONSUMED at the parent SPEC §3.0 (D-RH7); this SPEC re-checks the gate for 36.2's own envelope (anticipated NO further split — ~150 prod LoC). Phase 36 keeps the Load-balancing family OPEN; **5** candidates remain after 36 {maglev, subset LB, locality-weighted LB, priority load balancing, panic thresholds}.

**Goal:** Land the HTTP router's `RouteAction.hash_policy` producer — the SECOND data plane feeding the ring_hash consistent-hash key (after the tcp_proxy `source_ip` plane at 36.1). Per matched route, if `RouteAction.hash_policy[]` is non-empty, the router computes a `uint64` hash key by folding each supported policy's contribution and threads it via the LANDED `cluster.WithHashKey(ctx, key)` seam into `AcquireH1(ctx)`/`Dial(ctx)`/`DialH2(ctx)` → the LANDED `ringHashLB.Pick`. Two specifiers are SUPPORTED (the two most self-contained, mirroring the tcp plane's `source_ip` precedent): **`header`** (`xxHash64` over the matched request-header value) and **`connection_properties.source_ip`** (byte-identical to the tcp plane — reuse `cluster.HashSourceIP`). Three specifiers **DEPARTURE-reject** at config-build (fail-fast): `cookie`, `query_parameter`, `filter_state` (the reference validate-ACCEPTS all three — D-RH2.5 — so envoy-go's reject is a RECORDED DEPARTURE, not a parity reject). The multi-policy fold is **`hash = rotl64(prev,1) ^ new`** with the FIRST contributor taken verbatim and a `terminal=true` policy short-circuiting once the accumulator is non-empty (D-RH2.2). The **exported `Cluster` surface stays BYTE-STABLE** — 36.2 adds NO new exported `cluster` symbol (it CONSUMES `WithHashKey`/`HashSourceIP` landed at 36.1); the producer lives entirely in `internal/filter/hcm` + `internal/filter/http/router`. ONE ADR: **ADR-0237** (the HTTP route `hash_policy` producer plane).

**Architecture:** A per-route hash-policy descriptor is PARSED + VALIDATED at config-build (the tcp_proxy `NewFilter` precedent, fail-fast): `internal/filter/hcm/config.go`'s `buildRouterAction` reads `r.GetHashPolicy()`, rejects the three unsupported specifiers + an empty `header_name`, and lowers the supported entries into a small router-package descriptor slice carried on `clusterRouteAction` (`actions.go`); the existing `asRouterAction()`/`asRouterActionH2()` bridges pass it to the EXTENDED `router.H1ClusterAction(c, hps)` / `router.H2ClusterAction(c, hps)` constructors. Per request, a shared helper `applyHashKey(ctx, req, remoteAddr) context.Context` folds the descriptor list into a key and returns `cluster.WithHashKey(ctx, key)` (or `ctx` untouched when no policy contributes); it is called at the FOUR per-request dial entrypoints (`doH1ClusterAction` [router.go:504], the legacy `routerAction.do` [router.go:641], `doH2ClusterAction` [router_h2.go:57], `routerActionH2.doH2` [router_h2.go:214]) before `AcquireH1`/`Dial`/`DialH2`. The header contribution is `xxHash64(value)` for the single-value case (the fixture path) — generalizing to the seed-chained `XXH64` fold over sorted values for the multi-value case (D-RH2.3, REFINES the parent SPEC's loose "xxHash64(sorted header values)"); the source_ip contribution reuses `cluster.HashSourceIP(remoteAddr)` VERBATIM (the tcp plane's bare-IP `xxHash64`). A route with no `hash_policy` leaves `ctx` untouched → the cluster's LB sees `hasHash==false` (ring_hash → random fallback; the non-hash policies unaffected) — byte-stable. NO new packages, NO new go.mod deps, NO new stat names, NO new fuzzer, NO new BackendKind.

**Tech stack:** Go 1.26.x / golangci-lint 1.64.8 (ADR-0009); reference Envoy **`envoyproxy/envoy:contrib-v1.37.2`** (ADR-0227, @ `sha256:7edd5b0f…`). go-control-plane **`/envoy` v1.32.4** (ADR-0008 — `RouteAction.HashPolicy` field 15 + `RouteAction_HashPolicy` [Header/Cookie/ConnectionProperties/QueryParameter/FilterState + `Terminal`] already in the pinned module; **ZERO new go.mod dep**, `go mod tidy -diff` EMPTY — D-RH1). Reuses `internal/cluster/` (the LANDED `WithHashKey`/`hashKeyFrom` ctx-carry + `HashSourceIP`/`xxHash64` + the `ringHashLB` + the manager gate), `internal/filter/hcm/` (the route-build + dispatch path), `internal/filter/http/router/` (the H1/H2 action closures), the differential harness (`DistributionAsserter` + `StatsAsserter` + the existing HTTP backends), upstream Envoy v1.37.2 source (`source/common/http/hash_policy.cc` + `source/common/common/hash.{h,cc}`) for the algorithm pins. ZERO new packages.

**Authored:** 2026-06-12. **Empirical-pin probe date:** 2026-06-12 (FRESH in-session — the proto roster against the actual v1.32.4 module + the upstream v1.37.2 source + a 7-variant live `--mode validate` matrix + a live HTTP header-hash affinity/stats run on the bridge network).

---

## 1. Purpose / Mission

Phase 36.2 is the SECOND leg of the CONSUMED 36.1/36.2 by-plane split (parent SPEC §3.0, D-RH7). 36.1 (LANDED, `e8e8c48`) built the consistent-hash LB end-to-end on the tcp plane: the `ringHashLB` Ketama policy (ADR-0236), the ADR-0235 seam extension (`Pick(hashKey, hasHash)` + the `context.Context`-carried key via `cluster.WithHashKey`), the manager two-layer gate, the 3 `ring_hash_lb.*` gauges, the hand-rolled `xxHash64`/`murmurHash2`, and the `tcp_proxy` `hash_policy: source_ip` producer + the `0061-lb-ring-hash` subject-side affinity differential. 36.2 adds the SECOND producer — the HTTP router's `RouteAction.hash_policy` — against that ALREADY-PROVEN seam, plus a NAT-transparent TRUE cross-side affinity differential (the HTTP header key survives Docker's source-IP NAT, unlike the tcp `source_ip` plane — AMEND-RH8 / `reference_differential_hash_key_cross_side_infeasible`).

This SPEC inherits the parent phase-36 BRAINSTORM (Q0/Q1/Q-seam/Q-split, both planes) and refines the parent SPEC's 36.2 design surface (§3.3 the HTTP plane, §8.2 the `0062` fixture, §11.6 D-RH6 the combine, §13 the ADR-0236 draft) against (1) the AS-BUILT 36.1 code at master tip `e8e8c48` (the LANDED seam + helpers the producer threads into) and (2) a FRESH in-session empirical-pin block (§11) executed 2026-06-12. No separate 36.2 BRAINSTORM session is warranted: the design space (the specifier subset, the combine, the fixture shape) was settled at the parent level; 36.2 is a producer wiring against a landed seam, and the only open questions are PLAN/IMPL placement details (§12) — not design forks.

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The §11 fresh pins (D-RH1..D-RH4, re-numbered for 36.2; relating back to the parent's D-RH1/D-RH3/D-RH4b/D-RH6) CONFIRMED the parent SPEC's 36.2 anticipations and **REFINED one** (the header multi-value fold). The load-bearing pins, each carried into the relevant §§ below:

- **AMEND-362-1 (D-RH1 — the v1.32.4 `RouteAction.HashPolicy` surface re-pinned against the ACTUAL module; ZERO new dep).** `RouteAction.HashPolicy []*RouteAction_HashPolicy` (proto field 15; `GetHashPolicy()` nil-safe). `RouteAction_HashPolicy` carries the oneof `PolicySpecifier` {`Header_`=field 1, `Cookie_`=field 2, `ConnectionProperties_`=field 3, `QueryParameter_`=field 5, `FilterState_`=field 6} + the scalar `Terminal bool`=field 4. Nested: `RouteAction_HashPolicy_Header` {`HeaderName string`=field 1 (PGV `min_len=1` + the `^[^\x00\n\r]*$` no-control-char regex), `RegexRewrite`=field 2}; `RouteAction_HashPolicy_ConnectionProperties` {`SourceIp bool`=field 1, NO PGV}; `Cookie`/`QueryParameter`/`FilterState` (each with a PGV `min_len=1` name/key). The oneof is PGV-REQUIRED (`value is required`) + a typed-nil guard. (Distinct from the tcp-plane `type.v3.HashPolicy` — SourceIp/FilterState only, no header/cookie — already consumed by tcp_proxy at 36.1.) `go build ./...` OK; `go mod tidy -diff` EMPTY — **ZERO new go.mod dep** (the `config/route/v3` package is already on the import graph). See §5 / §11.1.
- **AMEND-362-2 (D-RH2 — the HTTP `hash_policy` algorithm pinned from v1.37.2 source; combine + terminal + source_ip CONFIRMED, header fold REFINED).** (combine) `HashPolicyImpl::generateHash` (`hash_policy.cc:259-266`): `hash` starts as `absl::nullopt`; for each policy producing `new_hash`, `old = hash ? rotl64(hash,1) : 0; hash = old ^ new_hash` — the FIRST contributor is taken VERBATIM (`0 ^ new`, NOT rotated); only the 2nd+ contributors rotate. (terminal) `hash_policy.cc:270`: `if (hash_impl->terminal() && hash) break;` — checks the ACCUMULATOR (a terminal policy short-circuits once ANY policy so far has populated the accumulator, even if the terminal one itself contributed nothing). (skip) a policy returning `nullopt` (absent header, no source IP) is **skipped ENTIRELY** — it neither rotates nor XORs (`hash_policy.cc:262` `if (new_hash)`). (source_ip) `IpHashMethod` (`hash_policy.cc:152-156`): `xxHash64(downstream_ip->addressAsString())` seed 0 — the BARE client IP, NO port — **byte-identical to the tcp plane** → reuse `cluster.HashSourceIP` VERBATIM. (header — REFINED) `HeaderHashMethod` (`hash_policy.cc:43-77` + `hash.cc:7-12`): collect ALL values for the (lowercased) header name → optional per-value `regex_rewrite` → `std::sort` byte-wise → fold `seed = XXH64(value_i, seed)` (SEED-CHAINED, NO separator). For the SINGLE-value case (the fixture path + the overwhelming common case) this collapses to `xxHash64(value, 0)`; the parent SPEC's AMEND-RH3 "`xxHash64(sorted header values)`" was imprecise for multi-value (it is NOT a concatenation). Absent header / empty list → `nullopt` (skip). See §3 / §11.2.
- **AMEND-362-3 (D-RH3 — the live `--mode validate` matrix: cookie/query/filter_state ACCEPT upstream → envoy-go's reject is a RECORDED DEPARTURE; empty `header_name` PGV-rejects → parity).** The 7-variant live matrix (§11.3) against contrib-v1.37.2: route `hash_policy` with `header` / `connection_properties:{source_ip:true}` / `cookie` / `query_parameter` / `filter_state` / the two-policy `terminal` form ALL **VALIDATE-ACCEPT** (exit 0); an empty `header_name` **REJECTS** (`RouteActionValidationError.HashPolicy[0] … HeaderValidationError.HeaderName: value length must be at least 1 characters`). **Decision:** SUPPORT `header` + `connection_properties.source_ip`; **DEPARTURE-reject** `cookie`/`query_parameter`/`filter_state` (fail-fast at config-build — they alter the reference's pick, so silent acceptance would silently diverge; the thrift AMEND-T7 / least_request AMEND-L5 / tcp-plane-36.1 fail-fast lineage). Mirror the empty-`header_name` reject (a parity reject; the exact wording is a PLAN/IMPL D-question — the live C++ message says `at least 1 characters`, the go-binding PGV says `1 runes` — envoy-go hand-rolls and may pin either, §6/§12). See §6 / §11.3.
- **AMEND-362-4 (D-RH4 — the HTTP plane adds ZERO new stat NAMES; `upstream_rq_total` is cross-equal here [unlike the tcp plane]; affinity holds live).** A live bridge-network HTTP RING_HASH run (route `hash_policy: header x-hash`, 3 STRICT_DNS backends; decode proven — `upstream_cx_rx_bytes_total=3920`) confirmed: the 3 `ring_hash_lb.{size=1026,min=342,max=342}` gauges present (LANDED at 36.1); **`cluster.svc.upstream_rq_total > 0`** (the HTTP router increments it — UNLIKE the tcp plane where it stays 0, the 0059/0060/0061 boundary); and **ZERO** stat names matching `hash_policy` (`grep -ic hash_policy /stats` = 0) — the HTTP producer adds NO new stat NAMES beyond the 36.1 gauges. **Stat surface STAYS 1119.** Live affinity re-confirmed: each distinct `X-Hash` value pins to ONE backend (4/4 stable), values spread across all 3 (≥2) — re-corroborating the parent's D-RH4b property (the specific value→backend map is endpoint-address-dependent and differs per run — `reference_differential_hash_key_cross_side_infeasible`; the PINNED property is per-value affinity + spread, NOT host identity). See §7 / §8 / §11.4.

### 1.2 ADR continuity + D-disposition at SPEC commit

DECISIONS.md tail at master tip is **ADR-0236** (the ring_hash policy, ACCEPTED at 36.1); next-free **ADR-0237**. Per the family-row routing, the DECISIONS tail **STAYS ADR-0236 at this SPEC** (counts UNCHANGED at the SPEC). The **ADR-0237 §Context DRAFT** (the HTTP route `hash_policy` producer plane) is anchored in §13; the full DECISIONS.md entry (§Context + §Decision + §Consequences) lands at the 36.2 IMPL per ADR-0044. The four D-RH pins are RESOLVED this session (§11); the remaining open items are PLAN/IMPL D-questions (§12).

---

## 2. Non-purposes (deferred; per the parent BRAINSTORM §8 + AMEND-362-3)

- **The deferred HTTP hash-key specifiers** — `cookie` (incl. Set-Cookie GENERATION with name/path/ttl/attributes), `query_parameter`, `filter_state`. The reference validate-ACCEPTS all three (D-RH3); envoy-go DEPARTURE-rejects them at config-build (fail-fast). They need cookie-generation callbacks / query-string parsing / filter-state plumbing — out of envelope. A future row if/when needed.
- **The `header` `regex_rewrite` knob** — `RouteAction_HashPolicy_Header.regex_rewrite` (apply a regex substitution to each header value before hashing — `hash_policy.cc:64-70`). The MVP supports a bare `header_name` only; a configured `regex_rewrite` DEPARTURE-rejects (fail-fast — it alters the hashed value). The deferred regex-rewrite path (it needs the `type.matcher.v3.RegexMatchAndSubstitute` RE2 plumbing).
- **Multi-value-header byte-exactness as the fixture path** — the seed-chained `XXH64` fold over sorted values (D-RH2.3) is the FULL algorithm; the differential fixture uses a SINGLE-valued `X-Hash` header (the `xxHash64(value,0)` collapse — the cross-side-reproducible path). Whether the multi-value seed-chained fold is IMPLEMENTED at 36.2 or DEFERRED (single-value-only with a multi-value DEPARTURE-reject) is a PLAN D-question (§12; anticipated: implement the full fold — it is ~6 LoC over the single-value path and keeps the header method faithful).
- **`MAGLEV` + all other LB policies** — unchanged from the parent SPEC §2 (byte-stable-rejected; each a future family row). 36.2 touches NO LB-policy gate (the gate is the 36.1 manager's; 36.2 is a producer only).
- **Cross-side host IDENTITY assertions** — INFEASIBLE in the harness (AMEND-RH8 — the two sides build the ring over different endpoint address strings); the differential asserts per-side (here BOTH-side, since the header key is NAT-transparent) affinity + spread + cross-side byte-equivalence + cross-equal stats.
- **A new fuzzer / BackendKind / stat name** — NONE (§7/§8.3): the HTTP hash key derives from a parsed header value / source IP, not an untrusted wire frame (no decoder → a key-fold unit/property test suffices, NOT a `Fuzz*` corpus entry); `0062` reuses the existing HTTP backends; the plane adds no stat names (AMEND-362-4).

---

## 3. The HTTP route `hash_policy` producer (ADR-0237) — threading the LANDED seam

36.2 is a PRODUCER wiring: it adds NO LB policy, NO seam change, NO manager change. It parses a per-route policy list at config-build, computes a key per request, and threads it via the LANDED `cluster.WithHashKey` into the LANDED dial path.

### 3.1 The config-build parse + DEPARTURE-reject (the tcp_proxy `NewFilter` precedent)

`internal/filter/hcm/config.go` `buildRouterAction(r *routev3.RouteAction, …)` (the existing route-action builder) gains a `hash_policy` parse, mirroring the tcp_proxy `NewFilter` source_ip parse (`filter.go:69-82`):

```go
// 36.2: parse RouteAction.hash_policy. header + connection_properties.source_ip
// → a consistent-hash key producer; cookie/query_parameter/filter_state
// DEPARTURE-reject (the reference validate-accepts them — recorded departure;
// fail-fast). A configured regex_rewrite or an empty header_name rejects.
// No hash_policy → hps stays empty (the existing byte-stable behavior).
hps, err := parseRouteHashPolicies(r.GetHashPolicy())
if err != nil {
    return nil, err // "router: hash_policy …"
}
// hps carried on clusterRouteAction → router.H1ClusterAction(c, hps) / H2…(c, hps)
```

`parseRouteHashPolicies` lowers each `*routev3.RouteAction_HashPolicy` into a small descriptor (a router-package type, e.g. `router.HashPolicy{Kind hpKind; HeaderName string; Terminal bool}`), with:
- `GetHeader()` non-nil → `header` kind; `GetHeaderName()` empty → reject `value length must be at least 1 …` (parity, AMEND-362-3); a non-nil `GetRegexRewrite()` → DEPARTURE-reject (§2).
- `GetConnectionProperties().GetSourceIp()==true` → `source_ip` kind; `source_ip==false` connection_properties → DEPARTURE-reject (the upstream `HashPolicyImpl` ctor adds NO hash method for a false `source_ip` — a `connection_properties` with no `source_ip` is inert upstream, so reject as unsupported; the PLAN re-pins the exact ctor line if a precise citation is needed).
- `GetCookie()`/`GetQueryParameter()`/`GetFilterState()` non-nil → DEPARTURE-reject (`router: hash_policy specifier %T is not supported (only header, connection_properties.source_ip)`).
- `Terminal` carried through verbatim.

The descriptor placement (router package vs an hcm-internal type) + whether `parseRouteHashPolicies` lives in `hcm` or `router` is a PLAN D-question (§12; anticipated: a `router`-package parse helper called from `hcm/config.go`, since the per-request compute lives in `router` and the descriptor is router-owned — but the proto-read + fail-fast reject is at the hcm config-build boundary).

### 3.2 The per-request compute + the `rotl64`-XOR fold + `terminal` (D-RH2)

The router action closures gain a shared helper (the parent SPEC's "4 sites, one key-compute helper"):

```go
// applyHashKey folds the route's hash policies into a ring_hash key and returns
// ctx carrying it (cluster.WithHashKey); if no policy contributes, ctx is returned
// unchanged (→ the LB's no-hash fallback). Mirrors Envoy HashPolicyImpl::generateHash
// (SPEC §3.2 / AMEND-362-2): rotl64(prev,1)^new fold, first contributor verbatim,
// nullopt policies skipped, a terminal policy short-circuits once a hash exists.
// headerVal + remoteAddr are CODEC-AGNOSTIC inputs (see the H1/H2 note below) —
// the helper itself does not take a concrete request type.
func applyHashKey(ctx context.Context, hps []HashPolicy, headerVal func(name string) (string, bool), remoteAddr string) context.Context {
    var acc uint64
    var has bool
    for _, hp := range hps {
        nh, ok := hp.contribute(headerVal, remoteAddr) // header value / source_ip → (uint64, ok); ok==false → skip
        if ok {
            if has {
                acc = bits.RotateLeft64(acc, 1) ^ nh
            } else {
                acc = nh // first contributor verbatim
                has = true
            }
        }
        if hp.Terminal && has {
            break // hash_policy.cc:270 — checks the accumulator
        }
    }
    if !has {
        return ctx // no contribution → no key → ring_hash random fallback (byte-stable)
    }
    return cluster.WithHashKey(ctx, acc)
}
```

- **header contribution:** the matched request header's value → `cluster.xxHash64([]byte(value))` for the single-value case (the fixture path). The full multi-value seed-chained fold (`seed = XXH64(v_i, seed)` over sorted values; D-RH2.3) is a thin generalization — whether to implement it or single-value-only + reject is a PLAN D-question (§2/§12). **Note:** `xxHash64` is unexported in `internal/cluster`; the header path needs either a new exported `cluster.HashBytes([]byte) uint64` sibling to `HashSourceIP`, OR the header compute lives behind a new `cluster.HashHeaderValues(values []string) uint64` helper. The exact exported surface for the header hash is a PLAN D-question (§12; anticipated: a small additive `cluster.HashHeaderValues` mirroring `HashSourceIP` — keeping `xxHash64` unexported and the seed-chained fold inside `internal/cluster` where the digest lives). This is the ONE candidate new exported `cluster` symbol; it is additive (byte-stable surface).
- **source_ip contribution:** `cluster.HashSourceIP(remoteAddr)` VERBATIM (the LANDED helper — `xxHash64(bareIP)`; the tcp plane's exact compute, D-RH2). `ok==false` if `remoteAddr` has no IP.
- **The H1/H2 input asymmetry (D-S362-3):** the FOUR dial sites split across two codecs. The H1 closures receive `*http.Request` (a `req.Header.Get(name)` accessor + a populated `req.RemoteAddr`); the H2 closures (`doH2ClusterAction`, `routerActionH2.doH2`) receive `h2.H2Request` — which has NEITHER an `*http.Request`-style header accessor (its `Headers []hpack.HeaderField` must be scanned, fields already lowercased) NOR a `RemoteAddr` field. Therefore: (a) the header accessor is per-codec (an `*http.Request.Header.Get` shim on H1; an hpack-field scan on H2) — the helper takes a `headerVal func(name string)(string,bool)` closure, NOT a concrete request; (b) the source_ip `remoteAddr` on H2 MUST come from the ctx-carried downstream remote addr (the HCM h2 dispatch already calls `chain.SetDownstreamRemoteAddr(c.downstreamRemoteAddr)` at `h2dispatch.go:337`), since `H2Request` carries no remote addr — on H1 `req.RemoteAddr` MAY suffice, else the same ctx-carry (`connection.go:384`) is used uniformly. The PLAN pins the per-codec extraction shims + whether to ctx-carry the remote addr uniformly (anticipated: a uniform ctx-carried downstream remote addr for both codecs — H2 requires it, and uniformity is simpler than an H1-only `req.RemoteAddr` special case). Since the `0062` fixture keys on the HEADER specifier (the cross-side proof), the source_ip specifier on the HTTP plane is UNIT-tested only — but the header accessor split is LIVE on every `0062` request, so the per-codec `headerVal` shim is NOT deferrable.

`applyHashKey` is called at the FOUR per-request dial entrypoints, each rebinding `ctx` before the dial:
- `doH1ClusterAction` (`router.go:504`) — before `a.cluster.AcquireH1(ctx)` (`:509`).
- `routerAction.do` (`router.go:641`) — before `a.cluster.Dial(ctx)` (`:662`) — the legacy phase-04 H1 path.
- `doH2ClusterAction` (`router_h2.go:57`) — before `a.cluster.DialH2(ctx)` (`:62`).
- `routerActionH2.doH2` (`router_h2.go:214`) — before `r.cluster.DialH2(ctx)` (`:233`).

The `routerAction`/`routerActionH2` structs gain a `hashPolicies []HashPolicy` field; `H1ClusterAction(c, hps)` / `H2ClusterAction(c, hps)` set it (the EXTENDED constructors; their callers `clusterRouteAction.asRouterAction()`/`asRouterActionH2()` in `hcm/actions.go` pass the parsed descriptor). A route with empty `hashPolicies` → `applyHashKey` returns `ctx` unchanged → byte-stable (all prior HTTP fixtures unaffected; the non-hash LB policies see `hasHash==false`).

### 3.3 What 36.2 does NOT touch (the LANDED-at-36.1 surface)

- `internal/cluster/loadbalancer.go` (the `Pick(hashKey, hasHash)` seam) — UNCHANGED.
- `internal/cluster/cluster.go` (`WithHashKey`/`hashKeyFrom` + the `Dial`/`AcquireH1`/`PickEndpoint` threading) — CONSUMED unchanged (the `Dial`/`AcquireH1`/`DialH2` already extract the ctx key; 36.2 only STUFFS it upstream of them).
- `internal/cluster/ringhash.go` + `hash.go` (the ring + `xxHash64`/`murmurHash2`/`HashSourceIP`/`ipOnly`) — CONSUMED unchanged (the header path MAY add one additive exported `cluster.HashHeaderValues` helper — §3.2/§12 — but touches no existing symbol).
- `internal/cluster/manager.go` (the `case Cluster_RING_HASH` + the gate + the 3 gauges) — UNCHANGED.
- `internal/filter/tcpproxy/filter.go` (the source_ip producer) — UNCHANGED.

---

## 4. Framework primitives — 0 new packages + 0 new go.mod deps + (≤1 additive exported helper)

Phase 36.2 adds NO framework seam (it CONSUMES the ADR-0235 seam landed at 36.1). ZERO new packages (the producer lives in `internal/filter/hcm` + `internal/filter/http/router`; the optional header-hash helper in the existing `internal/cluster`). ZERO new go.mod deps (AMEND-362-1 — `config/route/v3` is already imported). ring_hash is not a filter — no builtins registration, no TypeURL factory, no bootstrap blank-import. The ONLY candidate new exported symbol is an additive `cluster.HashHeaderValues` (the header digest; §3.2/§12) — additive, no existing-symbol change (the exported `Cluster` surface stays byte-stable). The router action constructors `H1ClusterAction`/`H2ClusterAction` gain a parameter — these are INTERNAL (`internal/filter/http/router`), not a cross-module exported-surface concern, but their signature change ripples to `hcm/actions.go` (the two `asRouterAction*` bridges) + the router/hcm tests (a contained blast radius — §6.2).

---

## 5. Proto-field roster (per §11.1 D-RH1)

All from go-control-plane `/envoy` v1.32.4, verified in the module cache + `.pb.validate.go` this session (the ACTUAL pinned module, not a stray cache).

### 5.1 `RouteAction.HashPolicy` (field 15)

`RouteAction.HashPolicy []*RouteAction_HashPolicy` (proto field 15; `GetHashPolicy()` nil-safe → empty slice on absent). Each entry:

| Specifier (oneof `policy_specifier`) | proto # | Go getter | 36.2 disposition |
|---|---|---|---|
| `Header_` → `RouteAction_HashPolicy_Header` | 1 | `GetHeader()` | **SUPPORTED** (`xxHash64(value)`) |
| `Cookie_` → `…_Cookie` | 2 | `GetCookie()` | DEPARTURE-reject |
| `ConnectionProperties_` → `…_ConnectionProperties` | 3 | `GetConnectionProperties()` | **SUPPORTED iff `source_ip==true`** (reuse `HashSourceIP`) |
| `QueryParameter_` → `…_QueryParameter` | 5 | `GetQueryParameter()` | DEPARTURE-reject |
| `FilterState_` → `…_FilterState` | 6 | `GetFilterState()` | DEPARTURE-reject |
| `Terminal bool` (scalar, NOT oneof) | 4 | `GetTerminal()` | consumed (short-circuit) |

### 5.2 The supported nested messages

- `RouteAction_HashPolicy_Header` — `HeaderName string` (field 1; PGV `min_len=1` + `^[^\x00\n\r]*$` regex), `RegexRewrite *type.matcher.v3.RegexMatchAndSubstitute` (field 2; non-nil → DEPARTURE-reject, §2).
- `RouteAction_HashPolicy_ConnectionProperties` — `SourceIp bool` (field 1; NO PGV). `GetSourceIp()`.

### 5.3 PGV (the parity reject)

The oneof is PGV-REQUIRED (`value is required`) + a typed-nil guard. `header_name` PGV `min_len=1` (the live reject `value must be at least 1 characters`, AMEND-362-3) — envoy-go mirrors this (parity reject; wording a PLAN D-question, §6/§12). `source_ip` + `terminal` carry NO PGV. (envoy-go runs NO PGV — it hand-rolls the gate, the phase-34/36.1 precedent; the reject wording is a deliberate contract surface.)

---

## 6. PARSE-REJECT roster (per §11.3 + ADR-0080)

### 6.1 Wording discipline

Per ADR-0080: the `hash_policy` reject-text is the deliberate contract-surface change this phase. TWO families: (a) the unsupported-specifier rejects (cookie/query_parameter/filter_state + a configured regex_rewrite + a `source_ip==false` connection_properties) — DEPARTURE rejects (the reference validate-ACCEPTS; envoy-go fail-fasts); (b) the empty-`header_name` reject — a PARITY reject (the reference PGV-rejects). All verified by table tests at the 36.2 IMPL.

### 6.2 Reject arms (UNIT-TESTED; NO cross-side boot-reject dir)

The `hash_policy` parse rejects land UNIT-LEVEL in `hcm`/`router` config tests (the tcp-plane-36.1 / phase-34 config-reject precedent — config-parse rejects are unit-tested, not a fixture dir; the differential adds little — the reference accepts, so a cross-side boot-reject is asymmetric). Indicative wording (final pinned at the IMPL):

- **`router-hash-policy-unsupported-specifier`** — `cookie`/`query_parameter`/`filter_state`: `router: hash_policy specifier %T is not supported (only header, connection_properties.source_ip)` (the tcp-plane `tcpproxy: hash_policy specifier %T is not supported (only source_ip)` sibling).
- **`router-hash-policy-connprops-no-source-ip`** — a `connection_properties` with `source_ip==false`: rejected as unsupported (Envoy adds no hash impl — inert upstream).
- **`router-hash-policy-regex-rewrite`** — a `header` with a non-nil `regex_rewrite`: DEPARTURE-reject (§2; it alters the hashed value).
- **`router-hash-policy-empty-header-name`** — a `header` with empty `header_name`: PARITY reject mirroring the PGV `value … must be at least 1 …` (the exact `characters`-vs-`runes` wording a PLAN/IMPL D-question, §12).

**Blast radius** (the `H1ClusterAction`/`H2ClusterAction` signature change + the new parse): `internal/filter/http/router/{router.go,router_h2.go}` (the constructors + the 4 dial entrypoints + the action structs) + `internal/filter/hcm/{config.go,actions.go}` (the parse + the 2 `asRouterAction*` bridges) + their `*_test.go`. NO production reject string outside `hcm`/`router`; NO fixture pins the `hash_policy` reject text (the `0062` fixture is an ACCEPT path) → confirm `grep` blast radius at the IMPL first-task gate.

### 6.3 NON-reject dispositions (parity)

- A route with NO `hash_policy`: accepted, `ctx` untouched (the existing byte-stable behavior — all prior HTTP fixtures unaffected).
- A `header` `hash_policy` whose header is ABSENT at request time: the policy contributes nothing (`nullopt` skip, D-RH2) → if it was the only policy, no key → ring_hash random fallback. NOT a reject (a runtime absence, not a config error). Parity with Envoy.

---

## 7. Stat surface — ZERO new names (per §11.4 D-RH4 + AMEND-362-4)

- **ZERO new stat NAMES.** Stat surface **STAYS 1119** (the 36.1-LANDED baseline — re-confirmed via the canonical recipe at the 36.2 PLAN/IMPL Task 1, not re-counted at the SPEC). The load-bearing SPEC-time evidence is the live HTTP RING_HASH run's `/stats` name-set: the subset matching `hash_policy` is EMPTY; the only `hash`-matching names are the 3 `ring_hash_lb.*` gauges LANDED at 36.1. The HTTP producer reuses the existing `cluster.<name>.*` cx/rq roster + the 36.1 gauges — it adds NO name.
- **`cluster.<name>.upstream_rq_total` is CROSS-EQUAL on the HTTP plane** (the router calls `IncUpstreamRqTotal` — `router.go:507` — so both subject and reference increment it; UNLIKE the tcp plane where the subject stays 0, the 0059/0060/0061 boundary). This is the `0062` `StatsAsserter`'s strong cross-equal rq prong (in addition to the 36.1 cross-equal gauge/cx/membership prongs).
- The 3 `ring_hash_lb.*` gauges remain CROSS-SIDE-EXACT (keyed only on ring-config + host count; 1026/342/342 for a 3-equal-host default cluster) — the `0062` `StatsAsserter` reuses the 36.1 gauge prong.

---

## 8. Differential fixture — `0062-lb-ring-hash-http` (fixtures 63 → 64)

Per `reference_differential_fixture_dispatch_constraint`: ONE cross-side dir (no boot-reject dir — §6.2). Per `reference_differential_asserter_dispatch`: the stats prong uses `StatsAsserter` (cross-side path); the affinity/spread prong uses the runner's `DistributionAsserter` hook. Per `reference_differential_run_selector`: targeted runs use `-run 'TestDifferential/0062'` (NOT bare `0062`). Every assertion proven live via a deliberate-break with `-count=1` (`reference_differential_break_protocol_count1`). The affinity leg is DETERMINISTIC/EXACT (same header value → same backend always — NOT a σ-band); the spread COUNT carries a coarse `>= 2` threshold (`reference_differential_band_sigma_margin` governs RNG bands, which the affinity leg is not).

### 8.1 `0062-lb-ring-hash-http` (HTTP route header hash; TRUE cross-side affinity+spread)

An HTTP listener routing to a 3-endpoint RING_HASH cluster (`ring_hash_lb_config: {}` defaults) with a route-level `hash_policy: [{header: {header_name: "x-hash"}}]`. The driver sends requests with distinct `X-Hash` values; the header survives Docker's source-IP NAT UNTOUCHED → a SYMMETRIC cross-side proof (AMEND-RH8 — the HTTP plane is the TRUE cross-side differential the tcp plane could not be). Backends: existing HTTP backends (NO new BackendKind, tail STAYS 33).

**The workload + assertions:**
- **Affinity (BOTH sides, EXACT):** for each distinct `X-Hash` value, all K repeats hit ONE backend (per-value backend-set cardinality == 1) — asserted on BOTH subject and reference (the header value is NAT-transparent and each side's ring is internally deterministic). The attribution mechanism is a PLAN D-question (§12; per the 36.1 D-S36-4 precedent: either an identity-revealing backend, OR the aggregate-count modular invariant — binding K repeats per value forces per-backend counts to be multiples of K; the 36.1 `count % 16 == 0` shape on the subject, here applicable to BOTH sides since the key is NAT-transparent).
- **Spread (BOTH sides, count):** N distinct header values collectively cover `>= 2` backends per side.
- **Cross-side byte-equivalence** of the HTTP responses (the standard differential prong).
- **Cross-side `StatsAsserter`** (§7): the 3 `ring_hash_lb.{size=1026,min=342,max=342}` gauges cross-equal + `membership_total=3` + quiesced `upstream_cx_active=0` + **`upstream_rq_total` cross-equal** (the HTTP-plane rq increment — the new prong vs `0061`).
- **NOT asserted:** cross-side host IDENTITY (does `X-Hash: foo` hit the SAME backend index on both sides?) — the two sides' rings are built over different endpoint address strings (AMEND-RH8 / `reference_differential_hash_key_cross_side_infeasible`). The PINNED property is per-value affinity + spread + byte-equiv + cross-equal stats.

**Deliberate-break liveness (`-count=1`):** (i) scatter the key (make `applyHashKey` ignore the header / return `ctx` untouched → ring_hash random) → a value's K repeats spread across backends → the per-value affinity leg FAILS (BOTH sides); (ii) collapse the spread (force one backend) → the spread leg (`>= 2` distinct) FAILS; (iii) drop a `StatsAsserter` Inc / corrupt `upstream_rq_total` → the stats prong FAILS; (iv) corrupt a gauge value → the cross-equal `ring_hash_lb.*` prong FAILS. ≥20-run flake check (affinity is deterministic; spread `>= 2` over N≥4 values / 3 backends is overwhelmingly stable). Recorded in driver comments + README.

### 8.2 Total + no new BackendKind/fuzzer (family expectations)

Fixtures 63 → **64** (`0062-lb-ring-hash-http`, tail). BackendKind tail STAYS **33** (existing HTTP backends reused). Fuzzers STAY **42** — the HTTP hash key derives from a parsed header value / source IP, not a wire frame; a key-fold property test (random policy lists → deterministic key, terminal short-circuits, nullopt skips) is UNIT-level, NOT a `Fuzz*` corpus entry (the 36.1 no-fuzzer precedent). h2spec 53/53 + proxy-wasm 10/10 re-run asserted-unaffected at the six-gate — BUT NOTE 36.2 DOES touch the HTTP/h2 path (the router action closures + the H2 dial entrypoints), so the conformance gates are NOT zero-touch-by-construction as they were at 36.1; the PLAN's final task RE-RUNS or asserts h2spec/proxy-wasm with an explicit change-scope rationale (the producer only STUFFS a ctx key before the existing dial — the request/response wire path is byte-unchanged when no `hash_policy` is configured, which is every conformance config). The REAL guard is the full differential re-verify: all 63 prior dirs stay byte-exact through the constructor signature change (the action closures are behavior-neutral when `hashPolicies` is empty — structural byte-neutrality).

---

## 9. Behavior-contract delta (the 36.2 bundle; ADR-0052 atomic landing)

At the 36.2 IMPL final task, `docs/envoy-go/BEHAVIOR_CONTRACT.md`'s `### Load balancer — ring_hash (RING_HASH)` subsection (created at 36.1) gains the HTTP-plane details:

- The HTTP route `hash_policy` producer: the supported specifiers (`header` `xxHash64`, `connection_properties.source_ip` reusing the tcp compute), the `rotl64(prev,1)^new` multi-policy fold (first contributor verbatim, nullopt skip, `terminal` short-circuit on a non-empty accumulator), the single-value header collapse vs the multi-value seed-chained `XXH64` fold; the DEPARTURE-rejected specifiers (cookie/query_parameter/filter_state + regex_rewrite + source_ip-false connection_properties — the reference validate-accepts → recorded departures); the empty-`header_name` parity reject; the NAT-transparent TRUE cross-side affinity (vs the tcp plane's subject-side-only); the `upstream_rq_total` cross-equal on the HTTP plane (vs the tcp-plane 0).
- The stat-surface note: STAYS 1119 (the HTTP producer adds NO new stat names; the FIRST LB producer that reuses the prior plane's stat surface entirely).
- The deferred-specifier records + the NO-new-fuzzer/BackendKind family expectations.

---

## 10. Per-task structure (the 36.2 PLAN decomposes)

Indicative spine (TDD per task; per-task `gofmt -l` + `golangci-lint` on touched pkgs per `feedback_pertask_gofmt_lint`; subagents commit LOCAL-ONLY per `feedback_subagents_no_push`; work in a worktree per `feedback_git_worktrees`; subagent-driven per `feedback_execution_style`). The PLAN re-checks the ADR-0045 gate (anticipated NO split — ~150 prod LoC / ~6–8 tasks).

| # | Task | SPEC anchor |
|---|---|---|
| 1 | First-task baselines/anchors gate: re-confirm fixtures **63** (tail `0061`) + fuzzers **42** + stat surface **1119** + BackendKind tail **33** + DECISIONS tail **ADR-0236** via the canonical recipes; re-pin the as-built anchors (the LANDED `cluster.WithHashKey`/`HashSourceIP`/`xxHash64`; `hcm/config.go buildRouterAction`; `hcm/actions.go` `clusterRouteAction` + `asRouterAction`/`asRouterActionH2`; `router.go` `H1ClusterAction`/`routerAction`/`doH1ClusterAction:504`/`do:641`; `router_h2.go` `H2ClusterAction`/`routerActionH2`/`doH2ClusterAction:57`/`doH2:214`; the 4 dial sites; the downstream-remote-addr access for source_ip); PROGRESS.md created | §3 / §11 |
| 2 | The header-hash helper (D-RH2.3): a `cluster.HashHeaderValues(values []string) uint64` (the seed-chained `XXH64` fold over sorted values; single-value collapse to `xxHash64(value,0)`) — TDD against the published XXH64 vectors + the multi-value seed-chain + the live reference's observed header→key mapping. (OR keep header hashing in `router` via a new exported `cluster.HashBytes` — D-question §12.) | §3.2 / §11.2 |
| 3 | The config-build parse + DEPARTURE-reject: `parseRouteHashPolicies` (`hcm/config.go`/`router`) — header (empty-name reject; regex_rewrite reject) + source_ip (source_ip-false reject) accept; cookie/query/filter_state reject; `terminal` carried. TDD: the §6 reject matrix, byte-stable table tests. Thread the descriptor onto `clusterRouteAction` + extend the `asRouterAction*` bridges | §3.1 / §6 |
| 4 | The per-request compute + fold: `applyHashKey` (`rotl64`-XOR, first-verbatim, nullopt-skip, terminal short-circuit) + extend `H1ClusterAction(c,hps)`/`H2ClusterAction(c,hps)` + the `routerAction`/`routerActionH2` `hashPolicies` field + the 4 dial-site `ctx` rebinds + the downstream-remote-addr wiring for source_ip. TDD: deterministic key for a header; multi-policy fold; terminal; no-policy → ctx untouched; H1 + H2 paths | §3.2 |
| 5 | The `0062-lb-ring-hash-http` fixture: driver (distinct `X-Hash` values, K repeats each), the BOTH-side affinity(cardinality-1 / modular-invariant)+spread(`>=2`) `DistributionAsserter`, the cross-side `StatsAsserter` prong (the 3 gauges cross-equal + cx/membership/quiesced + `upstream_rq_total` cross-equal) | §8.1 |
| 6 | Deliberate-break liveness (`-count=1`): scatter-the-key (affinity fails both sides), collapse-spread (spread fails), drop/corrupt-Inc (stats fails), corrupt-gauge (gauge prong fails); ≥20-run flake check | §8.1 |
| 7 | Full differential re-verify (the 63 prior dirs byte-exact through the constructor signature change + `0062` green) + `-race -short` + **h2spec/proxy-wasm RE-RUN-or-assert with the HTTP-path-touch rationale** (§8.2 — NOT zero-touch-by-construction at 36.2); build/vet/gofmt/lint/tidy | §8.2 |
| 8 | Completion bundle (ADR-0052 atomic landing): BEHAVIOR_CONTRACT 36.2 addendum (§9) + the ADR-0237 §Context+§Decision+§Consequences (ADR-0044 in-place; tail → ADR-0237) + STATE/ROADMAP row 36.2 done (flat family row — NO parent rollup; the 36 family STAYS OPEN) + the six-gate evidence (surface 1119 unchanged, fixtures 64) | §9 / §13 |

---

## 11. SPEC-time empirical-pin block (D-RH1..D-RH4 — executed IN-SESSION 2026-06-12)

Parallel-subagent fan-out executed this SPEC session per ADR-0004's hard-gate. **Probe date: 2026-06-12.** Reference corpus: (1) go-control-plane `/envoy` **v1.32.4** bindings (the ACTUAL pinned module cache, `go list -m` confirmed) + `route_components.pb.{go,validate.go}`; `go build ./...` + `go mod tidy -diff` in the SPEC worktree; (2) upstream Envoy **v1.37.2** source via raw.githubusercontent.com (`source/common/http/hash_policy.{cc,h}` + `source/common/common/hash.{h,cc}`); (3) the live `envoyproxy/envoy:contrib-v1.37.2` docker image on a bridge network (`reference_docker_probe_bridge_network`) — a 7-variant route-`hash_policy` `--mode validate` matrix + a live HTTP header-hash affinity/spread run (`upstream_cx_rx_bytes_total=3920` — decode verified) + a `/stats` name-set scrape; (4) the envoy-go codebase at master tip `e8e8c48`.

### Summary disposition table (4 pins)

| Pin | Topic | Disposition |
|---|---|---|
| §11.1 | D-RH1 — the `RouteAction.HashPolicy` proto/PGV surface (actual v1.32.4) | **CONFIRMED** (field 15; oneof Header=1/Cookie=2/ConnProps=3/Query=5/FilterState=6 + Terminal=4; header_name PGV min_len=1; ZERO new dep) |
| §11.2 | D-RH2 — the HTTP `hash_policy` algorithm (v1.37.2 source) | **CONFIRMED w/ 1 REFINEMENT** (combine `rotl64(prev,1)^new` first-verbatim; terminal checks accumulator; nullopt-skip; source_ip = bare-IP `xxHash64` = tcp-identical; **header fold is SEED-CHAINED XXH64, not concatenation** — single-value collapses to `xxHash64(value,0)`) |
| §11.3 | D-RH3 — the live `--mode validate` matrix + the departure set | **RESOLVED** (header/source_ip/cookie/query/filter_state/terminal ALL validate-accept → cookie/query/filter_state are RECORDED DEPARTURES; empty header_name PGV-rejects → parity) |
| §11.4 | D-RH4 — the HTTP-plane stat delta + live affinity | **RESOLVED** (ZERO new stat names; `upstream_rq_total` cross-equal [unlike tcp]; per-value affinity + ≥2 spread live-confirmed; surface STAYS 1119) |

### 11.1 D-RH1 — the `RouteAction.HashPolicy` surface: CONFIRMED

`go list -m github.com/envoyproxy/go-control-plane/envoy` → `v1.32.4` (the module's own version line; carries Envoy v1.37.x APIs). `RouteAction.HashPolicy []*RouteAction_HashPolicy` (field 15; `GetHashPolicy()` nil-safe). `RouteAction_HashPolicy`: oneof `PolicySpecifier` {`Header_`=1, `Cookie_`=2, `ConnectionProperties_`=3, `QueryParameter_`=5, `FilterState_`=6} + scalar `Terminal bool`=4 (`GetTerminal()`). `RouteAction_HashPolicy_Header`: `HeaderName string`=1 (PGV `min_len=1` + regex `^[^\x00\n\r]*$`), `RegexRewrite`=2. `RouteAction_HashPolicy_ConnectionProperties`: `SourceIp bool`=1 (NO PGV). `Cookie`/`QueryParameter`/`FilterState` each carry a PGV `min_len=1` name/key. The oneof is PGV-REQUIRED (`value is required`) + typed-nil guard; `Terminal` unconstrained. The tcp-plane `type/v3.HashPolicy` has only SourceIp/FilterState (distinct type, smaller surface — already consumed by tcp_proxy at 36.1). `go build ./...` OK; `go mod tidy -diff` EMPTY — ZERO new dep (`config/route/v3` already imported).

### 11.2 D-RH2 — the HTTP `hash_policy` algorithm: CONFIRMED (1 refinement)

`source/common/http/hash_policy.cc`:
- **combine** (`:259-266`): `absl::optional<uint64_t> hash;` then per producing policy `old = hash ? ((hash<<1)|(hash>>63)) : 0; hash = old ^ new_hash` — `rotl64(acc,1)^new`; the FIRST contributor is `0 ^ new = new` (VERBATIM, not rotated — the accumulator starts as `nullopt`, NOT 0); only the 2nd+ rotate.
- **terminal** (`:270`): `if (hash_impl->terminal() && hash) break;` — checks the ACCUMULATOR `hash` (a terminal policy short-circuits once ANY prior/this policy populated the accumulator).
- **nullopt skip** (`:262`): `if (new_hash) { … }` — a policy returning `nullopt` (absent header, no source IP) is skipped ENTIRELY (no rotate, no XOR).
- **source_ip** (`IpHashMethod`, `:152-156`): `xxHash64(downstream_ip->addressAsString())` seed 0 — the BARE IP, NO port; returns `nullopt` on no-StreamInfo / no-remote / no-IP / empty — **byte-identical to the tcp plane** → reuse `cluster.HashSourceIP`.
- **header — REFINED** (`HeaderHashMethod`, `:43-77` + `hash.cc:7-12`): collect ALL values for the lowercased header name → optional per-value `regex_rewrite` (BEFORE sort) → `std::sort` byte-wise → fold `seed = XXH64(value_i, seed)` (SEED-CHAINED, NO separator). Single value → `XXH64(value, 0)` (the fixture path). Absent/empty header → `nullopt` (skip). The parent SPEC's AMEND-RH3 "`xxHash64(sorted header values)`" was imprecise for multi-value (it is NOT a concatenation — it is an output-as-next-seed chain). `HashUtil::xxHash64` default seed 0 (`hash.h:28`). Go: `bits.RotateLeft64` for the combine; a faithful XXH64 (the LANDED `cluster.xxHash64`) seeded-chained for multi-value headers; the single-value path is the LANDED `xxHash64([]byte(value))`.

### 11.3 D-RH3 — the live `--mode validate` matrix: RESOLVED

The 7-variant matrix (contrib-v1.37.2, RING_HASH cluster `svc`, route `hash_policy` varied):

| # | Variant | Verdict | Decisive fragment |
|---|---|---|---|
| a | `header: {header_name: "x-hash"}` | ACCEPT | exit 0 |
| b | `connection_properties: {source_ip: true}` | ACCEPT | exit 0 |
| c | `cookie: {name: "sess"}` | ACCEPT | exit 0 |
| d | `query_parameter: {name: "q"}` | ACCEPT | exit 0 |
| e | `filter_state: {key: "k"}` | ACCEPT | exit 0 |
| f | `header(x-hash) terminal:true` + `connection_properties:{source_ip:true}` | ACCEPT | exit 0 |
| g | `header: {header_name: ""}` | **REJECT** | `RouteActionValidationError.HashPolicy[0] … HeaderValidationError.HeaderName: value length must be at least 1 characters` |

**Decision:** SUPPORT header + connection_properties.source_ip; DEPARTURE-reject cookie/query_parameter/filter_state (the reference validate-ACCEPTS them — recorded departures, fail-fast). Mirror the empty-header_name PGV reject (parity). The departure set + the parity reject land UNIT-LEVEL (NO cross-side boot-reject dir).

### 11.4 D-RH4 — the HTTP-plane stat delta + live affinity: RESOLVED

Live bridge-network HTTP RING_HASH run (route `hash_policy: header x-hash`, 3 STRICT_DNS backends; decode verified `upstream_cx_rx_bytes_total=3920`, `downstream_rq_2xx=16`):
- The 3 `ring_hash_lb.{size=1026,min=342,max=342}` gauges present (LANDED at 36.1).
- `cluster.svc.upstream_rq_total=16` (>0) — the HTTP router increments rq_total (UNLIKE the tcp plane's 0) → the `0062` cross-equal rq prong.
- `grep -ic hash_policy /stats` = 0; the only `hash`-matching names are the 3 `ring_hash_lb.*` gauges → ZERO new stat NAMES; surface STAYS 1119.
- Live affinity: 4 distinct `X-Hash` values × 4 requests → each value pins to ONE backend (4/4), values spread across all 3 (≥2). Re-confirms the parent's D-RH4b PROPERTY (the specific value→backend map is endpoint-address-dependent and differs per run — `reference_differential_hash_key_cross_side_infeasible`; the PINNED property is per-value affinity + spread, NOT host identity).

---

## 12. PLAN / IMPL D-questions (not empirical pins; resolved at PLAN/IMPL)

- **D-S362-1** — the header-hash exported surface: a new additive `cluster.HashHeaderValues(values []string) uint64` (seed-chained fold, single-value collapse) vs a `cluster.HashBytes([]byte) uint64` + the router-side fold (anticipated: `HashHeaderValues` — keep the seed-chained digest inside `internal/cluster` where `xxHash64` lives; the ONE additive exported `cluster` symbol).
- **D-S362-2** — the parse placement: `parseRouteHashPolicies` in `hcm` (where the proto is read at config-build) vs a `router`-package parse helper called from `hcm/config.go` (anticipated: a `router`-package descriptor type + parse helper, called from `hcm/config.go buildRouterAction` for the fail-fast reject; the descriptor is router-owned since the per-request compute lives there).
- **D-S362-3** — the per-codec header accessor + the downstream-remote-addr access (§3.2 H1/H2 note): H1 closures carry `*http.Request` (`req.Header.Get` + `req.RemoteAddr`); H2 closures carry `h2.H2Request` (an hpack-field scan for the header; NO `RemoteAddr` field — so the source_ip remote addr MUST come from the ctx-carried `chain.SetDownstreamRemoteAddr` — `h2dispatch.go:337` on H2, `connection.go:384` on H1). The `applyHashKey` helper takes a `headerVal func(name string)(string,bool)` closure (NOT a concrete request type) + a `remoteAddr string`. Anticipated: per-codec `headerVal` shims (live on every `0062` request — NOT deferrable) + a UNIFORM ctx-carried downstream remote addr for both codecs (H2 requires it; uniformity beats an H1-only `req.RemoteAddr` special case). The source_ip specifier is unit-test-only (the `0062` fixture keys on the header).
- **D-S362-4** — the multi-value header fold: implement the full seed-chained `XXH64` fold (D-RH2.3) vs single-value-only + a multi-value DEPARTURE-reject (anticipated: implement the full fold — ~6 LoC over single-value, keeps the header method faithful; the fixture uses single-value).
- **D-S362-5** — the empty-`header_name` reject wording: the live C++ `value … must be at least 1 characters` vs the go-binding PGV `1 runes` (`utf8.RuneCountInString`) (anticipated: pin the go-binding `1 runes` form — envoy-go's other parity rejects mirror the go-binding PGV text, and the SPEC hand-rolls against the go module not the C++ binary; a PROGRESS note records the choice. No cross-side fixture pins it, so the exact text is unit-level discretion per ADR-0080).
- **D-S362-6** — the `0062` affinity attribution mechanism: an identity-revealing backend vs the aggregate-count modular invariant (the 36.1 D-S36-4 `count % K == 0` shape, here applicable to BOTH sides since the header key is NAT-transparent) (anticipated: reuse the 36.1 modular-invariant approach if no identity-revealing backend exists — NO new BackendKind).
- **D-S362-7** — the conformance-gate disposition: 36.2 touches the HTTP/h2 router path (NOT zero-touch like 36.1) → the PLAN's final task RE-RUNS h2spec/proxy-wasm OR asserts-unaffected with the explicit "ctx-stuff-only, wire-path byte-unchanged when no hash_policy configured" rationale (anticipated: assert-with-rationale if the heavy h2spec image setup is impractical, per the 36.1 Task-9 precedent — but with a STRONGER rationale obligation since the path is touched).
- ADR-0045 split-gate re-check at the 36.2 PLAN (anticipated NO split — ~150 prod LoC / ~6–8 tasks, well under the gate).

---

## 13. ADR continuity — the ADR-0237 §Context DRAFT (anchored here; full entry at the 36.2 IMPL)

Per the family-row routing, the DECISIONS.md tail **STAYS ADR-0236 at this SPEC**. The §Context draft is anchored HERE; the full ADR-0237 entry (§Context + §Decision + §Consequences, status PROPOSED → ACCEPTED) lands at the 36.2 IMPL per ADR-0044 (tail ADR-0236 → ADR-0237).

**ADR-0237 §Context DRAFT (the HTTP route `hash_policy` producer plane):** Phase 36.2 lands the HTTP router's `RouteAction.hash_policy` producer — the SECOND data plane feeding the ring_hash consistent-hash key (after the tcp_proxy `source_ip` plane at 36.1; both feed the ALREADY-LANDED ADR-0235 seam + ADR-0236 `ringHashLB`). It is a PRODUCER-ONLY leg: NO LB policy, NO seam change, NO manager change, NO new stat name, NO new go.mod dep, NO new package. The route's `hash_policy[]` (proto field 15; `RouteAction_HashPolicy` — Header/Cookie/ConnectionProperties/QueryParameter/FilterState + `Terminal`) is PARSED + VALIDATED at config-build (`hcm/config.go buildRouterAction`, the tcp_proxy `NewFilter` fail-fast precedent): `header` (the bare `header_name`) + `connection_properties.source_ip` are SUPPORTED; `cookie`/`query_parameter`/`filter_state` + a configured `regex_rewrite` + a `source_ip==false` connection_properties DEPARTURE-reject (the reference validate-ACCEPTS cookie/query/filter_state — D-RH3 — so the reject is a RECORDED DEPARTURE; an empty `header_name` PARITY-rejects mirroring the PGV `min_len=1`). The parsed descriptor rides `clusterRouteAction` → the EXTENDED `router.H1ClusterAction(c,hps)`/`H2ClusterAction(c,hps)`. Per request, a shared `applyHashKey` helper folds the policy list into a `uint64` key — `hash = rotl64(prev,1) ^ new` (the FIRST contributor verbatim; a `nullopt` policy [absent header / no source IP] skipped entirely; a `terminal=true` policy short-circuiting once the accumulator is non-empty — the exact upstream `HashPolicyImpl::generateHash` mirror, D-RH2) — where the header contribution is `xxHash64(value)` (single-value; the multi-value case is the seed-chained `XXH64` fold over sorted values, D-RH2.3 — REFINING the parent SPEC's imprecise "xxHash64(sorted header values)") and the source_ip contribution REUSES `cluster.HashSourceIP` VERBATIM (the tcp plane's bare-IP `xxHash64`). The folded key is threaded via the LANDED `cluster.WithHashKey(ctx, key)` at the FOUR router dial entrypoints (`doH1ClusterAction`/`routerAction.do`/`doH2ClusterAction`/`routerActionH2.doH2`, before `AcquireH1`/`Dial`/`DialH2`); a route with no `hash_policy` leaves `ctx` untouched (byte-stable — the LB sees `hasHash==false` → the ring_hash random fallback). The differential proof is `0062-lb-ring-hash-http` — a NAT-transparent TRUE cross-side affinity (the `X-Hash` header survives Docker's source-IP NAT, unlike the tcp `source_ip` plane — AMEND-RH8): per-value affinity (cardinality-1, BOTH sides) + spread (`>=2`) + cross-side byte-equivalence + a cross-side `StatsAsserter` (the 3 `ring_hash_lb.*` gauges cross-equal + `upstream_rq_total` cross-equal — the HTTP router increments rq, UNLIKE the tcp plane). Stat surface STAYS 1119 (ZERO new stat names — the FIRST LB producer reusing the prior plane's stat surface entirely). The ONE candidate additive exported symbol is `cluster.HashHeaderValues` (the header digest; the exported `Cluster` surface stays byte-stable). NO new fuzzer (no wire decode — a key-fold property test is unit-level) + NO new BackendKind (tail stays 33).

§Decision/§Consequences bodies land at the 36.2 IMPL per ADR-0044 (next-free **ADR-0237**).

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

At the SPEC-DONE commit (ALL counts UNCHANGED at the SPEC — they advance at the IMPL):

- stat surface **1119** (→ **1119** at the 36.2 IMPL — ZERO new stat names, AMEND-362-4).
- differential fixtures **63** (→ **64** at the 36.2 IMPL [`0062-lb-ring-hash-http`]; NO boot-reject dir — §6.2).
- fuzzers **42** (→ **42** — NO new fuzzer, deliberate, §8.2).
- BackendKind tail **33** (→ **33** — NO new BackendKind, deliberate, §8.2).
- DECISIONS.md tail **ADR-0236** (STAYS at this SPEC — the ADR-0237 §Context is a DRAFT in §13; the full entry lands at the 36.2 IMPL per ADR-0044; next-free **ADR-0237**).
- ROADMAP row 36 STAYS `in-progress` with its `36.1 done, 36.2 in-progress` split column (the split is CONSUMED — parent SPEC §3.0); the `36.2` leg flips at the 36.2 IMPL six-gate (a flat family row, NO parent rollup per ADR-0106); the Load-balancing family stays OPEN (5 candidates remain after 36).
- spec-document-reviewer gate applies at this SPEC.
- Next → the **36.2 PLAN** (`superpowers:writing-plans` — decompose §10's spine into bite-sized TDD tasks; the ADR-0045 gate re-check — anticipated NO split).
