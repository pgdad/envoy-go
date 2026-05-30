# Phase 26 — Network read-filter chain framework + `echo` / `direct_response` / `rbac_network` (parent master SPEC)

> **For agentic workers:** this is the PARENT SPEC for the phase-26 3-way pre-split (26.1 / 26.2 / 26.3). It is NOT directly executable. Per the phase-22 / phase-24 / phase-25 parent-row precedent, each sub-phase lands its own SPEC → PLAN → IMPL in dedicated sessions. This parent SPEC: (1) resolves the BRAINSTORM §10 D1–D8 empirical pins IN-SESSION against reference Envoy v1.37.2 + go-control-plane v1.32.4 (§11), (2) formalizes the 3-way split surface-mapping + per-sub-phase scope boundaries (§3), and (3) anchors the phase-26 ADR §Context drafts ADR-0213..ADR-0218 (§10). The next session, per BOOTSTRAP §5, authors the 26.1 SPEC (or PLAN — per the per-sub-phase precedent).

**Goal:** Bootstrap the L4 network read-filter chain framework (the network-layer analogue of phase-07.1's HTTP filter framework) and land the Network-filters family's first three filters — `echo`, `direct_response`, `rbac_network` — at full upstream parity, across a 3-way feature-progressive pre-split.

**Architecture:** A NEW `internal/filter/network/` package supplies the read-filter iteration protocol (`OnNewConnection`/`OnData` → `Continue`/`StopIteration`), `ReadFilterCallbacks` (connection accessor + `ContinueReading()` + `DynamicMetadata()`), per-connection read buffering on `StopIteration`, a per-connection runtime context, and a freeze-after-boot threaded-constructor registry (mirrors ADR-0072/0079). 26.2 migrates the two existing terminal filters (`tcp_proxy`, HCM) onto the new read-filter interface and retires the hardcoded `internal/listener/manager.go` registry. 26.3 extracts the phase-16 RBAC policy engine into a shared `internal/rbac/` package and adds `rbac_network` as its second consumer, reusing `internal/dynamicmetadata/` at connection scope for the shadow-rule metadata sink.

**Tech Stack:** Go 1.26.2; go-control-plane v1.32.4 proto bindings (ADR-0008); reference Envoy v1.37.2 (ADR-0008); `internal/matcher/` (phase-16); `internal/dynamicmetadata/` (phase-22.2). ZERO new third-party `go.mod` dependencies.

**Authored:** 2026-05-30. **Empirical-pin probe date:** 2026-05-30.

---

## 1. Mission summary

Phase 26 is the **FIRST concrete phase under `BOOTSTRAP_PROMPT.md` §9's Network filters family** — the first new §9 feature family opened since the HTTP filters family closed at phase 25.3. It bootstraps the missing L4 read-filter chain framework (at master tip the only network filters — `tcp_proxy` + HCM — are terminal-only, selected one-per-chain via a hardcoded `map[string]filterConstructor` in `internal/listener/manager.go:102` with a private `filterHandler { Handle(ctx, conn) }` interface — no iteration, no callbacks, no extensible registration), then lands `echo`, `direct_response`, and `rbac_network` across three feature-progressive sub-phases.

The design was settled at BRAINSTORM via a 6-question user dialogue (`docs/envoy-go/phases/26-network-filter-chain-and-rbac/BRAINSTORM.md` §2): Q1 chain framework + rbac_network; Q2 3-way pre-split; Q3 sub-phase mapping; Q4 read-filter-only (write deferred + API-revision allowance); Q5 extract shared `internal/rbac/`; Q6 full upstream parity. This SPEC does NOT re-litigate those decisions; it executes the empirical pins they deferred and formalizes the surface-mapping.

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The §11 D1–D8 scrape against Envoy v1.37.2 + go-control-plane v1.32.4 CONFIRMED most BRAINSTORM hypotheses and REFINED/REFUTED several. The load-bearing amendments to the BRAINSTORM design, each carried into the relevant §§ below:

- **AMEND-A1 (D1 — direct_response message name).** The direct_response config message is **`Config`**, not `DirectResponse`. FQN `envoy.extensions.filters.network.direct_response.v3.Config`; TypeURL `type.googleapis.com/envoy.extensions.filters.network.direct_response.v3.Config`. The BRAINSTORM's `…v3.DirectResponse` framing (BRAINSTORM §1.1.1, §3.1) is corrected throughout. Single field `response` (`envoy.config.core.v3.DataSource`). See §5.2.
- **AMEND-A2 (D1 — echo is an empty message with vacuous PGV).** `envoy.filters.network.echo.v3.Echo` has ZERO user fields and ships only a VACUOUS (zero-constraint) `echo.pb.validate.go` (a `Validate()`/`ValidateAll()` that validates nothing, since the message has no fields). There is no echo config to parse beyond the empty message; the 26.1 boot-reject parity fixture must therefore derive from `direct_response`'s `DataSource.specifier`-required PGV rule, NOT from echo. See §5.1 + §8.1.
- **AMEND-A3 (D2 — rbac stat roster + prefix CONFIRMED, no `logged`).** Exactly four static counters: `allowed`, `denied`, `shadow_allowed`, `shadow_denied` (NO `logged`). Prefix shape `<stat_prefix>.rbac.<counter>` (or `rbac.<counter>` when `stat_prefix` empty); `shadow_rules_stat_prefix` inserts a segment between `rbac.` and the two `shadow_*` counters only. `stat_prefix` is a **PGV-required** field (min 1 rune). Dynamic per-policy counters live under `…rbac.policy.<name>.*`. See §7.
- **AMEND-A4 (D3 — HTTP-only matchers SILENTLY never-match upstream; PARSE-REJECT is an envoy-go-strict DEPARTURE).** Upstream constructs HTTP-only matchers (`header`, `url_path`, `uri_template`) normally and evaluates them against `Http::StaticEmptyHeaders` at L4 — they silently never-match, with NO config-load rejection. envoy-go's chosen PARSE-REJECT of these arms in the network-rbac context is therefore a deliberate envoy-go-strict departure (recorded in BEHAVIOR_CONTRACT). See §6 + §9.
- **AMEND-A5 (D4 SPEC-BLOCKING — connection-metadata sink = REUSE `internal/dynamicmetadata/`, NOT a new package).** Upstream emits the shadow pair (`shadow_engine_result` ∈ {`allowed`,`denied`}, `shadow_effective_policy_id`) under namespace `envoy.filters.network.rbac` via `connection().streamInfo().setDynamicMetadata(...)` — a plain `(namespace→key→Value)` Struct. envoy-go's `internal/dynamicmetadata/.Bucket` is already a scope-agnostic `map[string]map[string]*structpb.Value` with a `Set(filterName, key, *structpb.Value)` signature that matches 1:1, is mutex-free (fits single-goroutine-per-connection), and carries zero HTTP-stream coupling in code. **Resolution: the 26.1 per-connection runtime context owns a `*dynamicmetadata.Bucket` (constructed at connection entry), and `ReadFilterCallbacks` exposes `DynamicMetadata() *dynamicmetadata.Bucket` from 26.1**, mirroring the HTTP decoder/encoder callbacks (`internal/filter/http/callbacks.go:285,476`). This shapes the 26.1 callbacks API so 26.3 needs NO post-hoc revision — which is exactly why D4 was SPEC-blocking. The genuinely-NEW primitive is the per-connection runtime context object, not a metadata package. See §4.3 + §10 (ADR-0217).
- **AMEND-A6 (D4 — HTTP rbac emits NO dynamic metadata today; "consumer #1 re-verification" is engine-correctness, not metadata).** `internal/filter/http/rbac/` does NOT import `internal/dynamicmetadata/` and emits no `shadow_engine_result`/`shadow_effective_policy_id` (grep-confirmed zero occurrences in-repo). The BRAINSTORM §2.5 framing of "HTTP rbac = consumer #1 re-verified by shadow metadata" is REFINED: the consumer-#1 re-verification at 26.3 is **engine-evaluation correctness** (the phase-16 HTTP-rbac differential fixtures stay byte-exact green after the engine moves to `internal/rbac/`), NOT preservation of metadata emission that does not exist. The connection-metadata sink is net-new behavior for `rbac_network` only. See §4.4 + §4.3.
- **AMEND-A7 (D3 — enforced denial uses connection-close + termination-detail, NOT dynamic metadata).** Upstream's `setDynamicMetadata` helper writes ONLY the shadow pair. Enforced (non-shadow) denial sets `streamInfo().setConnectionTerminationDetails("rbac_deny_close")` and closes the connection (`ConnectionCloseType::NoFlush`); it does NOT emit dynamic metadata. So at 26.3 the metadata sink emits the shadow pair; enforced deny = connection close. See §4.4 + §9.
- **AMEND-A8 (D3 — decision made in `OnData`, not `OnNewConnection`).** Upstream network-rbac `onNewConnection()` is a no-op returning `Continue`; the RBAC decision is made in `onData` (once on first data for the default `ONE_TIME_ON_FIRST_BYTE`; on every `onData` for `CONTINUOUS`). The 26.3 `rbac_network` filter mirrors this. See §4.4.
- **AMEND-A9 (D1 — `delay_deny` field exists → PARSE-REJECT at 26.3, envoy-go-strict departure).** The network rbac.v3.RBAC carries a `delay_deny` (`google.protobuf.Duration`, field 8) that delays the deny-close via an async timer + `readDisable`. The synchronous single-goroutine-per-connection read-filter model has no clean timer seam at 26.3; `delay_deny` is PARSE-REJECTed (envoy-go-strict departure, IMPL-time reconsideration note). See §6.3 + §12 (D-P3).
- **AMEND-A10 (D1 — `matcher`/`shadow_matcher` xDS-unified-matcher fields).** Network rbac.v3.RBAC carries `matcher`/`shadow_matcher` (`xds.type.matcher.v3.Matcher`, fields 6/7) in addition to `rules`/`shadow_rules` (`envoy.config.rbac.v3.RBAC`, fields 1/2). The HTTP rbac filter already supports the unified-matcher path via `internal/matcher/` (`internal/filter/http/rbac/rbac.go:446,824`). 26.3 supports BOTH the `rules` and the `matcher` paths (parity with the extracted engine), with HTTP-input leaf matchers PARSE-REJECTed in either path per AMEND-A4. See §5.3 + §6.
- **AMEND-A11 (D8 — engine extraction is mechanical; LoC envelope fits per sub-phase).** The phase-16 HTTP-rbac evaluator already evaluates against an abstract `evalContext` interface (`internal/filter/http/rbac/evaluator.go:60-121`) whose L4 accessors (`DestinationIP`/`DestinationPort`/`RequestedServerName`/`DirectRemoteIP`/`RemoteIP`) are ALREADY present and currently stubbed to nil/zero in the HTTP filter (`rbac.go:965-979`). The 26.3 extraction is therefore mechanical (move + re-export), and the L4 `rbac_network` filter merely supplies a non-stub `evalContext`. Sub-phase envelopes: 26.1 ~14-18 tasks / ~850-1020 LoC; 26.2 ~10-16 tasks / ~500-900 LoC; 26.3 ~16-22 tasks / ~800-1250 net-new LoC (+~1300 moved). All fit the ADR-0045 gate per sub-phase; the engine-extraction LoC-accounting caveat is pinned at §3.0 + §12 (D-P1). See §15.

### 1.2 ADR continuity + D-hypothesis disposition at SPEC commit

DECISIONS.md tail at master tip is **ADR-0212**; next-free **ADR-0213**. This SPEC anchors the phase-26 §Context drafts ADR-0213..ADR-0218 (six ADRs, locked clean at BRAINSTORM §7; §Decision/§Consequences bodies land at each sub-phase IMPL per ADR-0044). No ADR is consumed at SPEC time (a SPEC drafts §Context only). The ADR-0209 + ADR-0213-reserve escape valves carried from the §9 family STAND-UNCONSUMED. All eight D-pins are resolved at this session (§11); the remaining open items are PLAN/sub-phase-SPEC D-questions (§12), not empirical pins.

---

## 2. Scope — non-purposes + REUSES-not-consumed

### 2.1 Non-purposes (deferred; per BRAINSTORM §8)

- **Write-filter chain** (`onWrite` / `WriteFilter`) — absent at 26.1 per Q4. The `ReadFilterCallbacks` interface carries an explicit API-revision-allowance clause (ADR-0213) for a future write-filter addition. No near-term Network-family filter uses it.
- **L4 protocol proxies** (`redis`, `mongo`, `kafka_broker`, `thrift`, `zookeeper`) — each a large terminal protocol surface; each a future Network-family phase.
- **`sni_cluster`** — non-terminal cluster-override read filter; future phase (needs a connection-level cluster-override seam beyond rbac's needs).
- **`network-filter-wasm`** (`envoy.filters.network.wasm`) — WASM host family; consumes both the 26.1 read-filter chain + the phase-25 `internal/wasm/` primitive.
- **xDS / RTDS-driven dynamic RBAC policy** — PARSE-REJECT at config-load; rbac_network accepts only static inline policy at 26.3.
- **Cross-goroutine async read-filter resume** — beyond single-goroutine-per-connection; lands when a consumer needs it. 26.1 ships synchronous iteration with `StopIteration`/`ContinueReading` on the connection goroutine.
- **`delay_deny`** (network rbac.v3.RBAC field 8) — PARSE-REJECT at 26.3 per AMEND-A9.
- **`audit_logging_options`** (config.rbac.v3.RBAC field 3) — `[#not-implemented-hide:]` upstream; PARSE-REJECT/ignore at 26.3 per the upstream not-implemented status.

### 2.2 REUSE-by-absence: no per-route surface (D6 — §11.6)

Network filters in Envoy are configured per filter-chain on the listener, NOT per-route. A recursive grep for `PerRoute` across `envoy/extensions/filters/network/{echo,direct_response,rbac}/` returns ZERO matches (§11.6). There is no `typed_per_filter_config` override surface for any phase-26 filter. **The ADR-0125 canonical-per-route-shape roster is NOT touched by phase 26** — confirmed by absence (the simpler reason than phase-23/24.1: network filters are not route-scoped at all). No `RegisterPerRouteValidator` call is added for any phase-26 filter.

### 2.3 REUSES (not new primitives)

- `internal/matcher/` (phase-16) — the generic matcher primitive; its `MatchContext` interface already exposes `SourceIP()/DestinationIP()/DestinationPort()/RequestedServerName()` (`internal/matcher/matcher.go:38-64`) — exactly the L4 connection facts `rbac_network` needs. Consumed by the shared RBAC engine for the `matcher`-path (AMEND-A10).
- `internal/dynamicmetadata/` (phase-22.2) — REUSED at connection scope for the rbac_network shadow-metadata sink per AMEND-A5 (NOT a new package).
- `internal/listener/` (phases 02/07.2) — the listener manager + chain-match + listener-filter pipeline; 26.2 rewires its per-connection dispatch onto the new read-filter chain.
- `internal/filter/tcpproxy/` + `internal/filter/hcm/` (phases 02/04/05) — migrated onto the read-filter interface at 26.2 (no package move).
- `envoy.config.rbac.v3` + `envoy.extensions.filters.network.{echo,direct_response,rbac}.v3` bindings — already vendored via go-control-plane v1.32.4.
- The freeze-after-boot registry discipline (ADR-0059/0072/0079), the two-step factory pattern (ADR-0079), the iteration-status protocol (ADR-0038), the single-goroutine-per-connection model (ADR-0071 spirit; `internal/listener/manager.go` `serveConnection`), and the atomic-landing + six-gate discipline (ADR-0052).

---

## 3. Sub-phase scope summary

### 3.0 Split disposition — PRE-CONFIRMED at BRAINSTORM Q2; LoC-accounting caveat pinned here

The 3-way pre-split was settled at BRAINSTORM Q2 (feature-progressive axis; ROADMAP rows 26.1/26.2/26.3 already `planned`). No SPEC-time re-decision. The D8 envelope (§11.8 + §15) confirms each sub-phase fits the ADR-0045 gate (~25 tasks / ~1500 LoC). The single sizing caveat: **26.3's engine-extraction LoC accounting** — ~1300 LoC of the phase-16 evaluator MOVES to `internal/rbac/` (mechanical, AMEND-A11) plus ~800-1250 net-new LoC. If the ADR-0045 gate counts moved-LoC as churn, the 26.3 PLAN must confirm fit or split 26.3a (extraction + HTTP-rbac migration) / 26.3b (rbac_network + sink). Resolved as a 26.3-SPEC/PLAN D-question (§12 D-P1), NOT a parent-SPEC blocker — the net-new surface is well within gate.

### 3.1 Split surface-mapping table (per phase-22/25 §3.1 precedent)

| Surface element | 26.1 | 26.2 | 26.3 |
|---|---|---|---|
| NEW `internal/filter/network/` framework (iteration protocol + `ReadFilterCallbacks` + per-conn buffering + per-conn context + registry) | **lands** | — | — |
| Per-connection read-filter chain DISPATCH wired into `manager.go` (NEW path, alongside the existing terminal-filter path) | **lands (dual-dispatch)** | unifies + retires old path | — |
| `echo` (`envoy.filters.network.echo.v3.Echo`) | **lands** | — | — |
| `direct_response` (`…direct_response.v3.Config`) | **lands** | — | — |
| `cmd/envoy-go/main.go` network-registry boot-wiring | **lands** | extends (tcp_proxy/HCM registered) | — |
| `tcp_proxy` migrated onto read-filter interface | — | **lands** | — |
| HCM migrated onto read-filter interface | — | **lands** | — |
| Retire hardcoded `filterRegistry`/`filterHandler`/`filterConstructor`/`buildTerminalFilter` (`manager.go`) | — | **lands** | — |
| NEW shared `internal/rbac/` engine (extract phase-16 evaluator; abstract matchable-context) | — | — | **lands** |
| HTTP rbac migrated onto `internal/rbac/` (consumer #1; re-verified by phase-16 fixtures) | — | — | **lands** |
| Connection-scoped `dynamicmetadata.Bucket` sink + `ReadFilterCallbacks.DynamicMetadata()` STORAGE wired | API shaped at 26.1 | — | **storage + writes land** |
| `rbac_network` (`…network.rbac.v3.RBAC`) full-parity filter (consumer #2) | — | — | **lands** |
| rbac stat roster (`allowed`/`denied`/`shadow_allowed`/`shadow_denied`) | — | — | **lands** |
| Differential fixtures | +3 (echo, direct_response, boot-reject) | +0-1 (multi-read-filter chain) | +2 (rbac cross-side, rbac boot-reject) |
| New fuzzers | +1 (network config-parse) | +0 | +1 (rbac config-parse) |
| Anticipated ADRs | 0213, 0214 | 0215 | 0216, 0217, 0218 |

### 3.2 Per-sub-phase scope detail

**26.1 `network-filter-chain-framework-and-echo`** — NEW `internal/filter/network/`: the read-filter iteration protocol (`ReadFilter` with `OnNewConnection() Status` + `OnData(buf, endStream) Status` → `Continue`/`StopIteration`), `ReadFilterCallbacks` (connection accessor exposing `Write`/`Close`/local+remote addr/SNI/peer-cert + `ContinueReading()` + `DynamicMetadata() *dynamicmetadata.Bucket`), the per-connection read-filter chain runner (sequential dispatch + connection-level read buffering on `StopIteration` per §11.5 D5), the per-connection runtime context (owns the `*dynamicmetadata.Bucket`), and the freeze-after-boot threaded-constructor `*network.Registry` (mirrors `internal/listener/listenerfilter/registry.go`). Plus `echo` + `direct_response` as first consumers. The chain dispatch is wired into `manager.go` as a NEW path **alongside** the existing terminal-filter path: a chain whose `filters[0]` resolves in the new `*network.Registry` dispatches via the read-filter chain; a chain whose terminal filter is `tcp_proxy`/HCM keeps the existing `buildTerminalFilter` + `Handle` path UNTOUCHED. This dual-dispatch confines 26.1's blast radius to new code (BRAINSTORM §1.1.1(d)); 26.2 unifies it. Read-filter-only (write deferred + API-revision allowance, ADR-0213).

**26.2 `network-filter-registry-migration`** — migrate `tcp_proxy` (`internal/filter/tcpproxy/`) + HCM (`internal/filter/hcm/`) onto the new `ReadFilter` interface + register them in the `*network.Registry`; RETIRE the hardcoded `filterRegistry` map + `filterHandler`/`filterConstructor` types + `buildTerminalFilter` in `manager.go`; unify the per-connection dispatch so every chain resolves its read-filter chain through the threaded `*network.Registry`, terminating at `tcp_proxy` or HCM. Back-compat proven by the EXISTING differential fixtures (`0000-tcp-echo` + TLS-TCP + HCM) staying byte-exact green (the strongest migration proof; deliberate-break discipline). Optionally +1 multi-read-filter-chain fixture (`echo` preceding `tcp_proxy`). NO new operator-visible filter.

**26.3 `network-filter-rbac`** — extract the phase-16 RBAC principal/permission evaluation engine from `internal/filter/http/rbac/` into shared `internal/rbac/` (the `evalContext`/`permissionEvaluator`/`principalEvaluator` interfaces + the matcher bridge move largely intact per AMEND-A11); migrate HTTP rbac as consumer #1 (re-verified by its phase-16 differential fixtures staying green); add `rbac_network` as consumer #2 — a read filter that builds a non-stub L4 `evalContext` from connection facts and makes its RBAC decision in `OnData` per AMEND-A8, supporting BOTH `rules` and `matcher` paths (AMEND-A10) with full enforced + shadow parity. Land the connection-scoped `dynamicmetadata.Bucket` storage + the shadow-metadata writes (namespace `envoy.filters.network.rbac`, keys `shadow_engine_result`/`shadow_effective_policy_id` per §11.4 D4). Enforced deny = connection close (per AMEND-A7). Stat roster `<stat_prefix>.rbac.{allowed,denied,shadow_allowed,shadow_denied}` (§7). HTTP-only matchers PARSE-REJECT (AMEND-A4); `delay_deny` PARSE-REJECT (AMEND-A9). Parent row 26 flips `in-progress → done` ATOMICALLY with sub-row 26.3 per the 18/19/22/24/25 ROLLUP precedent.

---

## 4. Framework primitives — 1 NEW package (26.1) + 1 NEW package (26.3) + 1 REUSE-at-connection-scope (26.3)

### 4.1 NEW: `internal/filter/network/` read-filter chain framework (ADR-0213 + ADR-0214; lands at 26.1)

The L4 analogue of `internal/filter/http/`, deliberately mirroring it and the listener-filter pipeline. Anticipated layout (SPEC formalizes; 26.1 SPEC/PLAN finalizes file split):

- `types.go` — the `ReadFilter` interface + `Status` enum (`Continue` / `StopIteration` — the two-value enum confirmed at §11.5 D5; NO `StopIterationAndBuffer`/`StopAllIteration` variants, because L4 buffering is connection-level not filter-level) + the two-step factory types (`NetworkFilterFactory func(tc *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error)` + `FilterInstanceFactory func() ReadFilter`, mirroring `listenerfilter/types.go:91-96`).
- `callbacks.go` — `ReadFilterCallbacks`: a `Connection()` accessor (exposing `Write([]byte, endStream bool)`, `Close(closeType)`, `LocalAddr()/RemoteAddr() net.Addr`, `RequestedServerName() string`, `DownstreamPrincipals() []string`), `ContinueReading()`, and `DynamicMetadata() *dynamicmetadata.Bucket` (per AMEND-A5). The connection accessor surface is shaped to supply the L4 inputs the §11.3 D3 matcher subset needs (so 26.3 needs no callbacks revision).
- `chain.go` — the per-connection read-filter chain runner: sequential dispatch mirroring §11.5 D5 (`OnNewConnection` called eagerly per filter at connection accept before any data, in order, stopping on `StopIteration`; `OnData` called with the connection read buffer; on `StopIteration` the runner stops at the current filter and leaves undrained bytes in the connection read buffer; `ContinueReading()` resumes at the NEXT filter with currently-available buffered bytes — replaying the connection-level buffer per the upstream contract). The runner mirrors `listenerfilter/pipeline.go:32-59` (the closest existing sequential-dispatch precedent) + the per-connection-buffering discipline pinned at §11.5.
- `registry.go` — the freeze-after-boot threaded-constructor `*network.Registry`: `sync.RWMutex` + `byTypeURL map[string]NetworkFilterFactory` + `frozen atomic.Bool`; `Register` panics if frozen ("registry frozen: cannot register %q post-boot") or on duplicate; `Lookup` takes `RLock` (lock-free post-Freeze); `Freeze` idempotent; `KnownTypeURLs` for error messages. Byte-identical discipline to `internal/listener/listenerfilter/registry.go:19-58` + `internal/filter/http/registry.go:17-110` (ADR-0072/0079). NO package-global `init()`.
- The per-connection runtime context (the L4 analogue of the HTTP chain's per-stream `FilterChain` state) — owns a `*dynamicmetadata.Bucket` (constructed via `dynamicmetadata.NewBucket()` at connection entry, `Reset()`+nil at close), threaded into each filter's `ReadFilterCallbacks`. Unused by echo/direct_response at 26.1; consumed by rbac_network at 26.3.

Boot-wired in `cmd/envoy-go/main.go` exactly as the HTTP + listener-filter registries are (`main.go:129-183,198-200`): construct `*network.Registry` → `Register(echo.TypeURL, echo.New)` + `Register(directresponse.TypeURL, directresponse.New)` → `Freeze()` BEFORE manager construction → thread as a NEW argument into `listener.NewManagerWithBaseDirAndAllowH2C(...)`.

### 4.2 `echo` + `direct_response` (26.1 first consumers; per §11.7 D7)

- **`echo`** (`internal/filter/network/echo/`, TypeURL `type.googleapis.com/envoy.filters.network.echo.v3.Echo`) — empty config (AMEND-A2; parse the empty `Echo{}`, accept empty/absent body). `OnData(buf, endStream)` writes the received bytes back via `callbacks.Connection().Write(buf, endStream)`, fully consumes the buffer, returns `StopIteration` (mirrors `echo.cc`: `connection().write(data, end_stream)` + `ASSERT(0 == data.length())` + `FilterStatus::StopIteration`). No `OnNewConnection` override (default `Continue`).
- **`direct_response`** (`internal/filter/network/directresponse/`, TypeURL `type.googleapis.com/envoy.extensions.filters.network.direct_response.v3.Config`) — config field `response` (`DataSource`; §5.2). The logic is in `OnNewConnection` (NOT `OnData`, per §11.7 D7): write the configured response bytes with `endStream=true`, set response-code-details `DirectResponse`, close the connection with `FlushWrite` close semantics, return `StopIteration`. NO configurable delay in v1.37.2.

### 4.3 NEW: connection-scoped dynamic-metadata sink — REUSE `internal/dynamicmetadata/` (ADR-0217; storage lands at 26.3, API shaped at 26.1)

Per AMEND-A5 (the SPEC-BLOCKING D4 resolution). The connection-metadata sink is NOT a new package. The per-connection runtime context (§4.1) owns a `*dynamicmetadata.Bucket` (the existing scope-agnostic `map[string]map[string]*structpb.Value` with `Set(filterName, key, *structpb.Value)`, mutex-free — fits single-goroutine-per-connection). `ReadFilterCallbacks.DynamicMetadata()` returns it, mirroring `internal/filter/http/callbacks.go:285,476`. ADR-0217 anchors: (i) the connection-scoped reuse of `internal/dynamicmetadata/` (with an in-place `doc.go` generalization to "scope-agnostic; owner-determined lifetime — per-stream OR per-connection" per ADR-0044); (ii) the per-connection runtime context as the new owning primitive; (iii) the 26.1 callbacks-API shaping that makes the 26.3 storage drop in without revision. At 26.3, `rbac_network` writes the shadow pair (§11.4 D4) via `callbacks.DynamicMetadata().Set("envoy.filters.network.rbac", "shadow_engine_result", structpb.NewStringValue("allowed"|"denied"))` (+ `shadow_effective_policy_id` when non-empty). NOTE: envoy-go has no downstream consumer of connection-level dynamic metadata today (grep-confirmed; no access-log sink reads it) — so the shadow metadata is observable in differential fixtures only indirectly (via the `shadow_allowed`/`shadow_denied` stats, asserted by `StatsAsserter` per the asserter-dispatch memory) + directly by unit tests on the Bucket. The emission is still landed at 26.3 for upstream-parity + future-consumer readiness.

### 4.4 NEW: `internal/rbac/` shared RBAC policy engine (ADR-0216; lands at 26.3, extract-at-second-consumer)

Extract the phase-16 principal/permission evaluation engine from `internal/filter/http/rbac/` (`evaluator.go` 854 LoC + the compile/evaluate core of `rbac.go`) into shared `internal/rbac/`, refactored to evaluate against the abstract matchable-context interface that ALREADY exists there (`evalContext` at `evaluator.go:60-121`; `permissionEvaluator`/`principalEvaluator` at `:24-38`). Per AMEND-A11 the extraction is mechanical: the interfaces are already abstract, and the HTTP filter already stubs the L4 accessors (`DestinationIP`/`DestinationPort`/`RequestedServerName`/`DirectRemoteIP`/`RemoteIP` → nil/zero at `rbac.go:965-979`). Consumers:

- **Consumer #1 — HTTP rbac** (`internal/filter/http/rbac/`): migrated to import + drive `internal/rbac/`; its `evalContext` impl keeps the HTTP-request-derived accessors. Re-verified by the phase-16 HTTP-rbac differential fixtures staying byte-exact green (engine-correctness re-verification per AMEND-A6 — NOT metadata).
- **Consumer #2 — `rbac_network`** (`internal/filter/network/rbac/`): supplies a non-stub L4 `evalContext` (the IP/port/SNI/peer-cert accessors the HTTP filter stubs). Decision in `OnData` (AMEND-A8); enforced deny = connection close (AMEND-A7); shadow result → the §4.3 metadata sink + the `shadow_*` stats.

ADR-0216 carries the API-revision-allowance clause (phase-22.1 ADR-0188 / phase-25.1 ADR-0202 pattern), with the added discipline of re-verifying the LIVE first consumer. The `internal/rbac/` package consumes `internal/matcher/` for the `matcher`-path (AMEND-A10), reusing the `MatchContext` L4 accessors.

### 4.5 Framework-delta accretion shape

Phase 26 returns to framework-GROWTH: NEW `internal/filter/network/` (26.1) + NEW `internal/rbac/` (26.3), plus a connection-scoped REUSE of `internal/dynamicmetadata/`. This is the project's first new filter-category framework since phase-07.1 (`internal/filter/http/`) and its first SECOND-consumer-driven extraction with a LIVE first consumer to re-verify (contrast the phase-22.1/25.1 first-consumer extractions with speculative future consumers).

---

## 5. Proto-field roster (per §11.1 D1)

All rosters transcribed from go-control-plane v1.32.4 (§11 corpus). Go field / proto field / tag / Go type / default.

### 5.1 `envoy.filters.network.echo.v3.Echo` (echo config — 0 fields)

EMPTY message (`echo.pb.go:24-28`; zero user fields). Ships only a VACUOUS `echo.pb.validate.go` (zero-constraint `Validate()`/`ValidateAll()`, since there are no fields). 26.1 parses the empty `Echo{}`; an empty/absent `typed_config` body is accepted. There is no echo field to PARSE-REJECT.

### 5.2 `envoy.extensions.filters.network.direct_response.v3.Config` (1 field; per AMEND-A1)

| Go field | proto field | tag | Go type | default |
|---|---|---|---|---|
| `Response` | `response` | 1 | `*config.core.v3.DataSource` | `nil` |

PGV: `response` is NOT required at the `Config` level (`config.pb.validate.go:53-94` — a nil `response` PASSES). But the embedded `DataSource.specifier` oneof IS required if `response` is present (`base.pb.validate.go:2894-2898`): `Filename` (min 1 rune), `InlineBytes` (no constraint), `InlineString` (no constraint), `EnvironmentVariable` (min 1 rune). The 26.1 boot-reject parity fixture (§8.1) targets a `direct_response` config whose `response.specifier` is unset — the cleanest PGV-mirror boot-reject available in phase 26 (AMEND-A2).

### 5.3 `envoy.extensions.filters.network.rbac.v3.RBAC` (8 fields; per §11.1 + AMEND-A9/A10)

| Go field | proto field | tag | Go type | default | 26.3 disposition |
|---|---|---|---|---|---|
| `Rules` | `rules` | 1 | `*config.rbac.v3.RBAC` | `nil` | SUPPORT (engine `rules`-path) |
| `ShadowRules` | `shadow_rules` | 2 | `*config.rbac.v3.RBAC` | `nil` | SUPPORT (shadow) |
| `StatPrefix` | `stat_prefix` | 3 | `string` | `""` | **REQUIRED** (PGV min 1 rune, `rbac.pb.validate.go:178-187`) |
| `EnforcementType` | `enforcement_type` | 4 | `RBAC_EnforcementType` enum | `0` = `ONE_TIME_ON_FIRST_BYTE` | SUPPORT both `ONE_TIME_ON_FIRST_BYTE`(0) + `CONTINUOUS`(1) |
| `ShadowRulesStatPrefix` | `shadow_rules_stat_prefix` | 5 | `string` | `""` | SUPPORT (stat-prefix segment) |
| `Matcher` | `matcher` | 6 | `*xds.type.matcher.v3.Matcher` | `nil` | SUPPORT (engine `matcher`-path, AMEND-A10) |
| `ShadowMatcher` | `shadow_matcher` | 7 | `*xds.type.matcher.v3.Matcher` | `nil` | SUPPORT (shadow matcher) |
| `DelayDeny` | `delay_deny` | 8 | `*durationpb.Duration` | `nil` | **PARSE-REJECT** (AMEND-A9) |

`rules`/`matcher` are mutually-preferred (if both set, `rules` ignored upstream — `rbac.pb.go:90-99`); 26.3 mirrors. The `RBAC_EnforcementType` enum: `RBAC_ONE_TIME_ON_FIRST_BYTE=0`, `RBAC_CONTINUOUS=1` (`rbac.pb.go:29-51`).

### 5.4 Shared `envoy.config.rbac.v3` policy proto (the engine roster; per §11.1 D1.4)

- **`RBAC`**: `action` (`RBAC_Action` enum: `ALLOW=0`/`DENY=1`/`LOG=2`; PGV `defined_only`); `policies` (`map[string]*Policy`); `audit_logging_options` (`[#not-implemented-hide:]` — PARSE-REJECT/ignore per §2.1).
- **`Policy`**: `permissions[]` (PGV min 1 item); `principals[]` (PGV min 1 item); `condition` (CEL `*expr.v1alpha1.Expr` — see §6.3 disposition); `checked_condition` (`[#not-implemented-hide:]`).
- **`Permission`** oneof `rule` (14 variants; oneof required). L4-evaluable: `any`, `destination_ip`, `destination_port`, `destination_port_range`, `requested_server_name`, plus recursive `and_rules`/`or_rules`/`not_rule`. HTTP-only (PARSE-REJECT per AMEND-A4): `header`, `url_path`, `uri_template`. Context-dependent: `metadata`, `sourced_metadata`, `matcher` (extension).
- **`Principal`** oneof `identifier` (13 variants; oneof required). L4-evaluable: `any`, `authenticated` (+`principal_name` `StringMatcher` against mTLS peer-cert URI/DNS SAN + subject), `source_ip`, `direct_remote_ip`, `remote_ip`, plus recursive `and_ids`/`or_ids`/`not_id`. HTTP-only (PARSE-REJECT per AMEND-A4): `header`, `url_path`. Context-dependent: `metadata`, `sourced_metadata`, `filter_state`.

Full variant tables with tags + payload types + DEPRECATED markers are transcribed in §11.1.

---

## 6. PARSE-REJECT roster (per §11.1 D1 + §11.3 D3 + AMEND-A4/A9)

### 6.1 Wording discipline + arm-name convention

Per ADR-0080 byte-stable PARSE-REJECT discipline. Each arm is a named constant with byte-stable wording verified by a `TestParseRejectConstants_ByteStable` table at IMPL. The exact wording is finalized at each sub-phase IMPL (the rosters below are the SPEC-anticipated arms); boot-reject parity arms (those mirroring an upstream PGV failure) are distinguished from envoy-go-strict departure arms (those rejecting where upstream silently accepts/no-ops).

### 6.2 26.1 PARSE-REJECT arms (framework + echo + direct_response)

- `direct-response-response-required` / `direct-response-datasource-specifier-required` — boot-reject parity (mirrors the `DataSource.specifier` PGV rule, §5.2). The load-bearing 26.1 boot-reject fixture arm.
- Framework-level: unknown network-filter `typed_config` type_url not in the frozen `*network.Registry` → boot-reject (mirrors the existing unknown-filter behavior in `manager.go`).
- echo: no field-level reject (empty message, AMEND-A2).

### 6.3 26.3 PARSE-REJECT arms (rbac_network)

- `rbac-network-stat-prefix-required` — boot-reject parity (mirrors the `stat_prefix` min-1-rune PGV rule, §5.3).
- `rbac-network-delay-deny-unsupported` — envoy-go-strict departure (AMEND-A9; the synchronous read-filter model has no timer seam).
- `rbac-network-http-only-permission-<header|url-path|uri-template>` + `rbac-network-http-only-principal-<header|url-path>` — envoy-go-strict departures (AMEND-A4; upstream silently never-matches). Applies to BOTH the `rules`-path and the `matcher`-path leaves (AMEND-A10).
- `rbac-network-condition-cel-unsupported` — envoy-go-strict departure if the phase-16 engine does not already support the CEL `condition`/`checked_condition` fields (the 26.3 SPEC confirms against the phase-16 engine's existing disposition; phase-16 HTTP rbac's handling of `condition` is the precedent — if it PARSE-REJECTs CEL, network mirrors).
- `rbac-network-audit-logging-not-implemented` — ignore/reject `audit_logging_options` per its upstream `[#not-implemented-hide:]` status.

The exact arm set + wording is finalized at the 26.3 SPEC/IMPL against the phase-16 engine's existing PARSE-REJECT roster (the engine moves to `internal/rbac/`, so the network filter inherits the engine's leaf-level rejects; the network-specific arms above are additive).

---

## 7. Stat surface (per §11.2 D2 + AMEND-A3)

### 7.1 26.1 + 26.2 stat delta

`echo`: 0 built-in stats (upstream parity). `direct_response`: 0 built-in stats. 26.2 migration: 0 net-new (existing terminal-filter stats unchanged — back-compat). Project stat surface stays **132** across 26.1 + 26.2.

### 7.2 26.3 rbac stat roster (4 counters; per §11.2 D2)

Exactly four static counters (NO `logged`, AMEND-A3):

| Counter | Type | Parity | Name |
|---|---|---|---|
| `allowed` | counter | upstream-parity | `<stat_prefix>.rbac.allowed` |
| `denied` | counter | upstream-parity | `<stat_prefix>.rbac.denied` |
| `shadow_allowed` | counter | upstream-parity | `<stat_prefix>.rbac.[shadow_rules_stat_prefix]shadow_allowed` |
| `shadow_denied` | counter | upstream-parity | `<stat_prefix>.rbac.[shadow_rules_stat_prefix]shadow_denied` |

Prefix composition (§11.2): `stat_prefix` (PGV-required) is dot-joined before the literal `rbac.` segment; `shadow_rules_stat_prefix` (if set) inserts a segment between `rbac.` and the two `shadow_*` counters only (the enforced counters are unaffected). Dynamic per-policy counters (`…rbac.policy.<name>.{allowed,denied}`) are created lazily upstream; 26.3 mirrors the static four at minimum, with the per-policy dynamic counters as a SPEC-confirmed roster item finalized at the 26.3 SPEC (the HTTP rbac filter already implements per-policy counters via a `sync.Map` at `rbac.go:165-191` — the extracted engine carries this).

### 7.3 Project stat-count delta

132 → **136** at 26.3 phase-done (the four static rbac counters), per the upstream-parity posture. The per-policy dynamic counters are not counted in the static surface (they are config-dependent, lazily created). SPEC-confirmed; 26.3 SPEC finalizes whether any envoy-go-strict extension counter lands (none anticipated — the upstream roster is complete).

### 7.4 envoy-go-strict departure flags (anticipated; BEHAVIOR_CONTRACT at 26.3 IMPL)

- HTTP-only-matcher PARSE-REJECT (AMEND-A4) — upstream silently never-matches.
- `delay_deny` PARSE-REJECT (AMEND-A9).
- xDS/RTDS dynamic policy PARSE-REJECT (§2.1).
- Write-filter absent (Q4 / ADR-0213).

---

## 8. Differential fixture taxonomy (per §11.7 D7 + the §9 full-cross-side precedent + the fixture-dispatch + asserter-dispatch memories)

Full cross-side byte-exact against reference Envoy v1.37.2. Per the `reference_differential_fixture_dispatch_constraint` memory, cross-side and boot-reject fixtures are SEPARATE directories (one fixture dir = one runner branch). Per the `reference_differential_asserter_dispatch` memory, any subject-side-only assertion uses `StatsAsserter` (not `SubjectAsserter`) and must be proven live. Fixture numbering continues from `0039` (the 25.3 tail); exact numbers pinned at each sub-phase IMPL.

### 8.1 26.1 fixtures (+3)

- **echo cross-side** (`00XX-network-echo`): a listener with the `echo` network filter; client writes bytes, expects identical bytes echoed back; byte-exact vs reference Envoy. Terminal topology: `echo` is the terminal read filter.
- **direct_response cross-side** (`00XX-network-direct-response`): a listener with `direct_response` configured with an `inline_string` response; client connects, expects the static response + connection close (`FlushWrite`); byte-exact.
- **network-filter boot-reject** (`00XX-network-direct-response-boot-reject`): a `direct_response` config with an empty/missing `response.specifier` → both upstream + envoy-go reject at boot (PGV-mirror, §6.2). Boot-stderr substring parity.

### 8.2 26.2 fixtures (+0-1)

Back-compat proven by the EXISTING fixtures (`0000-tcp-echo` + the TLS-TCP fixtures + the HCM fixtures) staying byte-exact green after `tcp_proxy`/HCM migrate onto the read-filter interface — the strongest migration proof. Optionally +1 NEW multi-read-filter-chain fixture (`echo` preceding `tcp_proxy` in one chain) proving real-path iteration + `ContinueReading` resume on the load-bearing path.

### 8.3 26.3 fixtures (+2)

- **rbac_network cross-side** (`00XX-network-rbac`): allow (passthrough to a `tcp_proxy` terminal) + deny (connection close) + shadow (passthrough + `shadow_*` stat increments) scenarios. The allow/deny/shadow scenarios may share one cross-side dir (multiple bootstrap arms) or split; 26.3 SPEC pins. Subject-side stat assertions (`shadow_allowed`/`shadow_denied`/`allowed`/`denied`) use `StatsAsserter` and must be proven live (asserter-dispatch memory). L4 inputs exercised: `direct_remote_ip` principal + `destination_port` permission at minimum; an SNI (`requested_server_name`) + mTLS-peer-cert (`authenticated`) scenario if the fixture harness supports client certs.
- **rbac_network boot-reject** (`00XX-network-rbac-boot-reject`): a missing `stat_prefix` (PGV-mirror) OR an HTTP-only matcher arm (envoy-go-strict departure — boot-reject where upstream would ACCEPT, so this is a subject-side-only boot-reject, NOT cross-side; the dispatch-constraint memory requires it be its own dir, and it is documented as an envoy-go-strict-only reject not a parity reject).

### 8.4 Total fixture-dir count

Family total +5-6 (fixtures 41 → 46-47). Exact count + numbering pinned at each sub-phase IMPL. No new conformance harness (the L4 filters are validated by differential + existing back-compat fixtures; BRAINSTORM §6.5).

---

## 9. Behavior-contract delta (semantic; per ADR-0052 atomic landing)

BEHAVIOR_CONTRACT.md gains phase-26 content in three passes (one bundle per sub-phase IMPL final task per ADR-0052). Anticipated:

- **26.1 bundle**: NEW `### Network filter chain framework` subsection (iteration protocol + connection-level buffering + `ReadFilterCallbacks` + the read-filter-only scope + write-filter-deferral note); NEW `### envoy.filters.network.echo` + `### envoy.filters.network.direct_response` subsections; a forward-pointer notes subsection (26.2 migration, 26.3 rbac). envoy-go-strict departure record: write-filter absent.
- **26.2 bundle**: a structural note documenting the hardcoded-registry retirement + the tcp_proxy/HCM migration onto the read-filter interface (no operator-visible behavior change; back-compat via existing fixtures).
- **26.3 bundle**: NEW `### envoy.filters.network.rbac` subsection (full enforced + shadow parity; L4 principal/permission input surface; decision-in-OnData; enforced-deny = connection close; shadow = metadata + stats); stat-table 132 → 136 extension (the four `rbac` counters); envoy-go-strict departure records: HTTP-only-matcher PARSE-REJECT (AMEND-A4), `delay_deny` PARSE-REJECT (AMEND-A9), xDS dynamic-policy PARSE-REJECT; a note that connection-level dynamic metadata is emitted (namespace `envoy.filters.network.rbac`) but has no in-repo downstream consumer yet (AMEND-A5/A6).

---

## 10. ADR anchor map (2 NEW §Context drafts at THIS parent-SPEC commit: ADR-0213 + ADR-0214; +1 anticipated at 26.2 SPEC; +3 anticipated at 26.3 SPEC; ZERO ADR-0125 amendments per §2.2)

Per ADR-0044 (§Context at SPEC; §Decision/§Consequences at IMPL) + the BRAINSTORM §7 locked numbering + the phase-25 parent-SPEC precedent (the parent SPEC lands the FIRST sub-phase's ADR §Context drafts in DECISIONS.md; each later sub-phase's SPEC lands its own). At THIS parent-SPEC commit, ADR-0213 + ADR-0214 §Context drafts are appended to DECISIONS.md (the 26.1 ADRs); the 26.2 + 26.3 ADRs are provisional forward-pointers here and are drafted into DECISIONS.md at the 26.2 + 26.3 SPEC sessions respectively. No two sub-phases float onto the same number.

### 10.1 26.1 ADRs (DRAFTED into DECISIONS.md at this commit)

- **ADR-0213** — NEW `internal/filter/network/` read-filter chain framework: iteration-status protocol (`Continue`/`StopIteration`, two-value per §11.5), `ReadFilterCallbacks` (connection accessor + `ContinueReading()` + `DynamicMetadata()`), connection-level read buffering on `StopIteration`, single-goroutine-per-connection concurrency, the per-connection runtime context; read-filter-ONLY scope + write-filter deferral + API-revision-allowance clause. echo + direct_response fold in (no separate ADR). §Decision/§Consequences land at 26.1 IMPL.
- **ADR-0214** — the freeze-after-boot threaded-constructor `*network.Registry` (mirrors ADR-0072/0079) + the `cmd/envoy-go/main.go` boot-wiring + the 26.1 dual-dispatch (new chain path alongside the existing terminal-filter path) + the planned 26.2 hardcoded-registry retirement. §Decision/§Consequences land at 26.1 IMPL.

### 10.2 26.2 ADRs (provisional; drafted at 26.2 SPEC)

- **ADR-0215** (provisional) — the `tcp_proxy` + HCM migration onto the read-filter interface + the `manager.go` `filterRegistry`/`filterHandler`/`filterConstructor`/`buildTerminalFilter` retirement + the dispatch unification + the back-compat-via-existing-fixtures discipline.

### 10.3 26.3 ADRs (provisional; drafted at 26.3 SPEC)

- **ADR-0216** (provisional) — NEW `internal/rbac/` shared engine extraction (abstract matchable-context interface — the existing `evalContext`; consumer #1 HTTP-rbac migration + re-verification; consumer #2 rbac_network; API-revision-allowance clause; the mechanical-extraction finding AMEND-A11).
- **ADR-0217** (provisional) — the connection-level dynamic-metadata sink as a connection-scoped REUSE of `internal/dynamicmetadata/` (per AMEND-A5): the per-connection runtime context owning a `*dynamicmetadata.Bucket`, the `ReadFilterCallbacks.DynamicMetadata()` accessor (shaped at 26.1, storage at 26.3), and the `internal/dynamicmetadata/` doc generalization to scope-agnostic.
- **ADR-0218** (provisional) — the `rbac_network` filter: L4 principal/permission input surface (§5.4 + §11.3), HTTP-only-matcher PARSE-REJECT departure (AMEND-A4), the `<stat_prefix>.rbac.*` stat roster (§7.2), the full-parity enforced + shadow semantics (decision-in-OnData, enforced-deny = connection close, shadow = metadata + stats), `delay_deny` PARSE-REJECT (AMEND-A9), the `rules` + `matcher` dual-path support (AMEND-A10).

### 10.4 Next-free-ADR

At THIS parent-SPEC commit: ADR-0213 + ADR-0214 §Context drafts CONSUMED → DECISIONS.md tail advances to ADR-0214; **next-free ADR-0215** (consumed at the 26.2 SPEC). After phase-26 phase-done (all of 0213..0218 landed): anticipated next-free **ADR-0219**. The ADR-0209 escape-valve reserve carried from the §9 family STANDS-UNCONSUMED; the BRAINSTORM §7 "ADR-0213-reserve" framing is moot now that 0213 is the framework ADR (as the BRAINSTORM §7 locked numbering itself anticipated).

---

## 11. Empirical-pin block (D1–D8 resolved at this SPEC session)

Parallel-subagent-fan-out scrape executed during this SPEC session per ADR-0004's hard-gate. **Probe date: 2026-05-30.** The 8 pins span all three sub-phases; resolved once here; sub-phase SPECs reference this block.

**Reference source corpus:**

1. **go-control-plane v1.32.4 bindings** at `/home/esa/go/pkg/mod/github.com/envoyproxy/go-control-plane/envoy@v1.32.4/`: `extensions/filters/network/echo/v3/echo.pb.go`; `extensions/filters/network/direct_response/v3/{config.pb.go, config.pb.validate.go}`; `extensions/filters/network/rbac/v3/{rbac.pb.go, rbac.pb.validate.go}`; `config/rbac/v3/{rbac.pb.go, rbac.pb.validate.go}`; `config/core/v3/{base.pb.go, base.pb.validate.go}`.
2. **Upstream Envoy v1.37.2 source** via WebFetch against `raw.githubusercontent.com/envoyproxy/envoy/v1.37.2/`: `envoy/network/filter.h`; `source/common/network/{filter_manager_impl.cc, filter_manager_impl.h}`; `source/extensions/filters/network/echo/echo.cc`; `source/extensions/filters/network/direct_response/filter.cc`; `source/extensions/filters/network/rbac/{rbac_filter.cc, rbac_filter.h}`; `source/extensions/filters/common/rbac/{utility.h, utility.cc, engine_impl.cc, engine_impl.h, matchers.cc}`.
3. **envoy-go codebase** at master `d0c63d2`: `internal/listener/manager.go`; `internal/filter/http/{types,registry,callbacks,chain}.go`; `internal/filter/http/rbac/{evaluator,rbac}.go`; `internal/filter/{tcpproxy,hcm}/`; `internal/listener/listenerfilter/`; `internal/matcher/matcher.go`; `internal/dynamicmetadata/dynamicmetadata.go`; `internal/filterstate/`; `cmd/envoy-go/main.go`.

### Summary disposition table (8 pins)

| Pin | Topic | Disposition | AMEND |
|---|---|---|---|
| §11.1 | D1 — filter-config field rosters (echo / direct_response / network-rbac / config.rbac.v3) | CONFIRMS + REFINES (direct_response msg = `Config` not `DirectResponse`; echo empty + no validate.go; `stat_prefix` PGV-required; `delay_deny` + `matcher`/`shadow_matcher` fields present) | A1, A2, A9, A10 |
| §11.2 | D2 — rbac stat roster + prefix | CONFIRMS (4 counters `allowed`/`denied`/`shadow_allowed`/`shadow_denied`, NO `logged`; prefix `<stat_prefix>.rbac.<counter>`) | A3 |
| §11.3 | D3 — L4 matcher subset + HTTP-only behavior | REFINES (HTTP-only matchers SILENTLY never-match upstream → PARSE-REJECT is an envoy-go-strict DEPARTURE; decision in OnData; enforced-deny = connection close, not metadata) | A4, A7, A8 |
| §11.4 | D4 (SPEC-BLOCKING) — conn-metadata namespace/shape + sink primitive | RESOLVES (namespace `envoy.filters.network.rbac`, keys `shadow_engine_result`/`shadow_effective_policy_id`; sink = REUSE `internal/dynamicmetadata/` at connection scope, NOT a new package; callbacks API shaped at 26.1) | A5, A6 |
| §11.5 | D5 — read-filter iteration + buffering | CONFIRMS (two-value `Continue`/`StopIteration`; onNewConnection eager-at-accept; connection-level buffering; ContinueReading resumes at NEXT filter with live read buffer) | — |
| §11.6 | D6 — no per-route surface | CONFIRMS (zero `*PerRoute` messages; ADR-0125 untouched) | — |
| §11.7 | D7 — echo + direct_response semantics | CONFIRMS (echo OnData write-back + drain + StopIteration; direct_response OnNewConnection write + FlushWrite-close + StopIteration, no delay) | — |
| §11.8 | D8 — per-sub-phase LoC/task envelope | CONFIRMS fit (26.1 ~14-18t/~850-1020 LoC; 26.2 ~10-16t/~500-900; 26.3 ~16-22t/~800-1250 net + ~1300 moved); engine-extraction LoC-accounting caveat → §3.0 + D-P1 | A11 |

### 11.1 D1 — filter-config field rosters

**echo** — `envoy.filters.network.echo.v3.Echo` is EMPTY (`echo.pb.go:24-28`); no `.pb.validate.go`. **direct_response** — message `Config` (FQN `…direct_response.v3.Config`, `config.pb.go`); field `response` (`*config.core.v3.DataSource`, tag 1); `response` not required at `Config` level (`config.pb.validate.go:53-94`) but `DataSource.specifier` oneof required if present (`base.pb.validate.go:2894-2898`; `Filename`/`EnvironmentVariable` min 1 rune, `InlineBytes`/`InlineString` unconstrained). **network rbac** — `RBAC` (8 fields; §5.3 table); `stat_prefix` PGV-required min 1 rune (`rbac.pb.validate.go:178-187`); `EnforcementType` enum `ONE_TIME_ON_FIRST_BYTE=0`/`CONTINUOUS=1` (`rbac.pb.go:29-51`); `matcher`/`shadow_matcher` are `xds.type.matcher.v3.Matcher` (fields 6/7); `delay_deny` `Duration` (field 8). **config.rbac.v3** — `RBAC{action ALLOW=0/DENY=1/LOG=2, policies map, audit_logging_options [not-impl]}`; `Policy{permissions[] min1, principals[] min1, condition CEL, checked_condition [not-impl]}`; `Permission` 14-variant oneof (required); `Principal` 13-variant oneof (required). L4 vs HTTP variant classification per §5.4. (Note Go pkg-name collision: both network-rbac and config-rbac bindings are pkg `rbacv3` — import aliasing required at IMPL.)

### 11.2 D2 — rbac stat roster + prefix

Shared `Filters::Common::RBAC` stats macros (`utility.h`): `ENFORCE_RBAC_FILTER_STATS` = `allowed` + `denied`; `SHADOW_RBAC_FILTER_STATS` = `shadow_allowed` + `shadow_denied`. NO `logged`. The network filter calls `generateStats(proto_config.stat_prefix(), "", proto_config.shadow_rules_stat_prefix(), scope)` (`rbac_filter.cc`) — rules_prefix is empty `""`. `generateStats` (`utility.cc`): `base_prefix = statPrefixJoin(prefix, "rbac.")`; enforced counters use `base_prefix + rules_prefix` → `<stat_prefix>.rbac.{allowed,denied}`; shadow counters use `base_prefix + shadow_rules_stat_prefix` → `<stat_prefix>.rbac.[shadow_prefix]{shadow_allowed,shadow_denied}`. Empty `stat_prefix` → `rbac.<counter>`. Dynamic per-policy counters under `…rbac.policy.<name>.*`.

### 11.3 D3 — L4 matcher subset + HTTP-only behavior

The connection-only engine entry delegates to the HTTP overload with `Http::StaticEmptyHeaders` (`engine_impl.cc`): `handleAction(connection, *Http::StaticEmptyHeaders::get().request_headers, info, effective_policy_id)`. `HeaderMatcher::matches` reads only the (empty) headers (`matchers.cc`) → HTTP-only matchers (`header`, `url_path`, `uri_template`) SILENTLY never-match; upstream does NOT reject at config load (option a). L4 inputs read (`matchers.cc` `IPMatcher::extractIpAddress`): `destination_ip`=`downstreamAddressProvider().localAddress()`; `source_ip`=`connection.connectionInfoProvider().remoteAddress()`; `direct_remote_ip`=`directRemoteAddress()`; `remote_ip`=`downstreamAddressProvider().remoteAddress()`; `destination_port`=`localAddress()…ip()->port()`; `requested_server_name`=`connection.requestedServerName()`; `authenticated`=`connection.ssl()` URI/DNS SAN + subject. Decision timing (`rbac_filter.h`/`.cc`): `onNewConnection()` returns `Continue` (no-op); decision in `onData` — once on first data for `ONE_TIME` (default), every `onData` for `CONTINUOUS`. Enforced deny: `setConnectionTerminationDetails("rbac_deny_close")` + `connection().close(NoFlush)` (optional `delay_deny` timer + `readDisable` — PARSE-REJECTed by envoy-go per AMEND-A9).

### 11.4 D4 (SPEC-BLOCKING) — conn-metadata namespace/shape + sink primitive

Upstream (`rbac_filter.cc` `setDynamicMetadata`): writes to `callbacks_->connection().streamInfo().setDynamicMetadata(NetworkFilterNames::get().Rbac, metrics)` — namespace **`envoy.filters.network.rbac`**, CONNECTION-scoped. Keys (`engine_impl.h` `DynamicMetadataKeys`): `shadow_effective_policy_id` (written when non-empty), `shadow_engine_result` (string value `allowed`/`denied`). The helper writes ONLY the shadow pair (enforced denial = connection-termination-details + close, NOT metadata — AMEND-A7). envoy-go `internal/dynamicmetadata/.Bucket` (`dynamicmetadata.go:15-92`) is a scope-agnostic `map[string]map[string]*structpb.Value` (`Set(filterName, key, *structpb.Value)`, mutex-free, zero HTTP coupling in code; HTTP-stream binding lives only in the consumer `internal/filter/http/chain.go:314`). **Resolution (AMEND-A5):** REUSE it at connection scope — the per-connection runtime context owns a `*dynamicmetadata.Bucket`; `ReadFilterCallbacks.DynamicMetadata()` returns it (shaped at 26.1, storage at 26.3); no new package; the key-shape matches 1:1. HTTP rbac emits no metadata today (grep-confirmed — AMEND-A6), so this is net-new for rbac_network.

### 11.5 D5 — read-filter iteration + buffering

`envoy/network/filter.h` (NOTE: v1.37.2 path is `envoy/network/filter.h`, not `include/…`): `FilterStatus { Continue, StopIteration }` (two values only). `onNewConnection()` — "Filter chain iteration can be stopped if needed." `onData(Buffer&, bool end_stream)`. `continueReading()` — "the next filter will be called with all currently available data in the read buffer (it will also have onNewConnection() called on it if it was not previously called)." `filter_manager_impl.cc`: `initializeReadFilters()` calls `onNewConnection()` eagerly per filter at connection accept (before any data), in order, stopping on `StopIteration`. `onContinueReading()` is the loop: per filter, lazily call `onNewConnection()` if not initialized (StopIteration → return), then `onData` with the connection read buffer if `length>0 || end_stream` (StopIteration → return). `continueReading()` calls `onContinueReading(this, connection_)` — resumes at `std::next(filter->entry())` (the NEXT filter) with the live connection read buffer (replays currently-available bytes). `injectReadDataToFilterChain` wraps the caller's own buffer in a `FixedReadBufferSource` (propagated verbatim, not the connection buffer). Buffering is connection-level (undrained bytes stay in the connection read buffer), NOT per-filter — hence the two-value Status enum (no `StopIterationAndBuffer`).

### 11.6 D6 — no per-route surface

Recursive grep for `PerRoute` across `extensions/filters/network/{echo,direct_response,rbac}/` → ZERO matches. Network filters are chain-scoped, not route-scoped. ADR-0125 roster untouched (REUSE-by-absence).

### 11.7 D7 — echo + direct_response semantics

`echo.cc`: `onData` = `read_callbacks_->connection().write(data, end_stream)` + `ASSERT(0 == data.length())` (drains) + `return FilterStatus::StopIteration`; no `onNewConnection` override (default Continue). `direct_response/filter.cc`: logic in `onNewConnection` (no `onData`) — `Buffer::OwnedImpl data(response_); connection.write(data, true)` (end_stream=true) + set ResponseCodeDetails `DirectResponse` + `connection.close(ConnectionCloseType::FlushWrite)` + `return FilterStatus::StopIteration`; NO configurable delay in v1.37.2.

### 11.8 D8 — per-sub-phase LoC/task envelope

Grounded estimates from actual envoy-go LoC (the §11 corpus): **26.1** new `internal/filter/network/` framework (~380-450) + echo (~80) + directresponse (~120) + manager.go dual-dispatch rewire (~150-250) + main.go boot-wiring (~20) + fixtures + 1 fuzzer (~100) ≈ **850-1020 LoC, ~14-18 tasks**. **26.2** tcp_proxy adapt (~40-80) + HCM adapt (~60-120) + manager.go registry retirement + dispatch unification (~200-350, much overlapping 26.1's rewire) + test churn (~100-200) ≈ **500-900 LoC, ~10-16 tasks**. **26.3** `internal/rbac/` extraction (~1300 MOVED + ~150-300 new boundary) + HTTP-rbac migration (~100-200) + conn-metadata sink (~100-150) + `rbac_network` filter (~200-350) + stats (~100) + fixtures + 1 fuzzer (~150) ≈ **800-1250 net-new LoC (+~1300 moved), ~16-22 tasks**. All fit the ADR-0045 gate per sub-phase (net-new basis). Project fuzzer count = **34** at master tip — this REFUTES the BRAINSTORM §12 "fuzzers 35 → ~37-38" hypothesis (confirmed: `grep -rh "^func Fuzz" $(find . -name fuzz_test.go) | wc -l` = 34 across 28 files; 4 files carry >1 fuzzer — `hcm/h2`, `http/extproc`, `http/extauthz` have 2 each, `http/lua` has 4). The new network config-parse fuzzer at 26.1 is the **35th**; the rbac config-parse fuzzer at 26.3 is the **36th** → **36** at family-done.

---

## 12. SPEC-time D-questions for PLAN / sub-phase-SPEC resolution

- **D-P1 (26.3 engine-extraction LoC accounting).** Does the ADR-0045 split-gate count the ~1300 MOVED LoC as 26.3 churn? **Resolution at:** 26.3 SPEC/PLAN. **Anticipated:** net-new basis (moved code is mechanical relocation, re-verified by phase-16 fixtures) — 26.3 fits as one sub-phase; split 26.3a/26.3b only if the gate counts moved-LoC and the PLAN exceeds ~25 tasks.
- **D-P2 (26.1 dual-dispatch vs unified-at-26.1).** Does the chain dispatch rewire land fully at 26.1 (new path alongside old) or partly defer to 26.2? **Resolution at:** 26.1 SPEC. **Anticipated:** 26.1 lands the NEW chain path for echo/direct_response alongside the untouched terminal-filter path (BRAINSTORM §1.1.1(d)); 26.2 unifies + retires old. Confirms the §3.1 mapping.
- **D-P3 (`delay_deny` reconsideration).** Reconsider `delay_deny` PARSE-REJECT (AMEND-A9) if a clean timer seam emerges in the read-filter model. **Resolution at:** 26.3 IMPL. **Anticipated:** PARSE-REJECT stands (synchronous model, edge feature, out of BRAINSTORM enforced+shadow scope).
- **D-P4 (CEL `condition` disposition).** Does the phase-16 engine (moving to `internal/rbac/`) already support/reject the `condition`/`checked_condition` CEL fields? **Resolution at:** 26.3 SPEC against the phase-16 engine's existing roster. **Anticipated:** mirror the phase-16 HTTP-rbac disposition (the network filter inherits the engine's leaf-level handling).
- **D-P5 (per-policy dynamic counters).** Does 26.3 land the `…rbac.policy.<name>.*` dynamic per-policy counters (the HTTP engine has them via `sync.Map`)? **Resolution at:** 26.3 SPEC. **Anticipated:** yes (carried by the extracted engine; not counted in the static 132→136 surface).
- **D-P6 (PARSE-REJECT byte-stable wording).** Finalize the §6 arm wording + `TestParseRejectConstants_ByteStable` tables. **Resolution at:** each sub-phase IMPL.
- **D-P7 (boot-reject fixture arm finalization).** Confirm the §8.1 + §8.3 boot-reject arms are the cleanest parity candidates against upstream boot-stderr. **Resolution at:** each sub-phase IMPL (empirical-test the candidate arms).

---

## 13. RATIFIED-PENDING-IMPL items

- **R1 (26.1 framework mirror).** The `internal/filter/network/` registry + chain runner are near-verbatim structural copies of `internal/listener/listenerfilter/{registry,pipeline}.go` + `internal/filter/http/registry.go` (ADR-0072/0079). 26.1 IMPL verifies the freeze-after-boot + late-Register-panic + lock-free-post-freeze discipline via tests mirroring the listenerfilter registry tests.
- **R2 (26.1 callbacks API completeness for 26.3).** `ReadFilterCallbacks` exposes `DynamicMetadata() *dynamicmetadata.Bucket` + the full L4 connection accessor surface (§4.1) at 26.1, so 26.3 needs no callbacks revision. 26.1 IMPL verifies the accessor signatures against the §11.3 D3 L4 input list.
- **R3 (26.2 back-compat).** The EXISTING `0000-tcp-echo` + TLS-TCP + HCM differential fixtures stay byte-exact green after the tcp_proxy/HCM migration — the deliberate-break migration proof.
- **R4 (26.3 engine extraction re-verification).** The phase-16 HTTP-rbac differential fixtures stay byte-exact green after the engine moves to `internal/rbac/` (consumer #1 re-verification, engine-correctness — AMEND-A6).
- **R5 (26.3 metadata sink reuse).** `internal/dynamicmetadata/.Bucket` reused at connection scope (no new package); the doc generalized to scope-agnostic in-place (ADR-0044). 26.3 IMPL verifies the Bucket `Set("envoy.filters.network.rbac", "shadow_engine_result", …)` round-trips + the namespace/key strings match §11.4 byte-for-byte.
- **R6 (fuzzer count).** 26.1 IMPL first-task `grep -rh "^func Fuzz" $(find . -name fuzz_test.go) | wc -l` confirms **34** at master tip (per §11.8, refuting the BRAINSTORM's 35); the new network config-parse fuzzer is the **35th**; 26.3 adds the **36th**.
- **R7 (stat baseline re-confirm).** The 132 master-tip stat baseline (§7.3) is used as a hard parity number (132 → 136 at 26.3). The 26.3 SPEC re-confirms 132 with a concrete grep against the stats roster as a first-task gate (parallel to R6), before asserting the 132→136 delta.

---

## 14. Test surface

Per the §9 family precedent (unit + fuzz + differential + race). Per sub-phase:

- **26.1**: Layer A unit tests at `internal/filter/network/` (registry freeze/panic/lookup; chain iteration + StopIteration + ContinueReading resume + connection-level buffering; per-connection context + DynamicMetadata accessor) + `internal/filter/network/{echo,directresponse}/` (parse + OnData/OnNewConnection semantics); Layer C the 36th fuzzer `FuzzNetworkFilterConfigParse` (or per-filter fuzzers); Layer D the +3 differential fixtures (§8.1); Layer E race tests on the chain runner (single-goroutine-per-connection, but `-race` proves no data race in the registry/Bucket).
- **26.2**: unit tests for tcp_proxy/HCM under the read-filter interface; the EXISTING fixtures as the back-compat gate; optionally the multi-read-filter-chain fixture.
- **26.3**: unit tests at `internal/rbac/` (the extracted engine — principal/permission evaluation against an abstract context; the phase-16 evaluator tests move with the engine) + `internal/filter/network/rbac/` (L4 evalContext build; OnData decision; enforced-deny close; shadow metadata + stats; HTTP-only-matcher PARSE-REJECT; delay_deny PARSE-REJECT); the 37th fuzzer `FuzzNetworkRBACConfigParse`; the +2 differential fixtures (§8.3, StatsAsserter for the rbac counters); the phase-16 HTTP-rbac fixtures as the consumer-#1 re-verification gate.
- **Six-gate checklist** (per phase-22/24/25): `go build ./...` + `go vet ./...` + `golangci-lint run` + `go test -race -short ./...` + the differential suite for the phase's feature surface + (no new conformance suite; existing 10/10 proxy-wasm + 53/53 h2spec + 41→46-47 differential stay green).

---

## 15. Per-sub-phase split-gate confirmation (D8 → ADR-0045)

| Sub-phase | Net-new LoC | Moved LoC | Tasks | ADR-0045 gate (~25t / ~1500 LoC) | Verdict |
|---|---|---|---|---|---|
| 26.1 | ~850-1020 | 0 | ~14-18 | fits | ✅ |
| 26.2 | ~500-900 | 0 | ~10-16 | fits | ✅ |
| 26.3 | ~800-1250 | ~1300 | ~16-22 | fits (net-new basis; D-P1 caveat) | ✅ (with D-P1) |

Each sub-phase is independently shippable + delivers value: 26.1 ships two usable network filters + the reusable read-filter chain; 26.2 pays down the hardcoded-registry debt + proves the framework on the load-bearing path; 26.3 ships connection-level RBAC + the shared engine. The 3-way pre-split holds; no further splitting anticipated at parent-SPEC time (the 26.3 D-P1 caveat is a 26.3-SPEC/PLAN decision, not a parent-SPEC blocker).

---

## 16. Stage-close handoff

Per ADR-0004/0005 (autonomous adaptation): this SPEC is reviewed by the `spec-document-reviewer` subagent (≤3 iterations); on approval, STATE.md advances to lifecycle-state 2 with `next-skill = superpowers:writing-plans` scoped to the **26.1 SPEC** (per the per-sub-phase precedent — the next session authors the 26.1 sub-phase SPEC, not the 26.1 PLAN, mirroring phase-22.1/25.1 where each sub-phase landed its own SPEC). The parent SPEC is squash-merged to master + pushed; next-prompt.txt is rewritten for the 26.1-SPEC cold-start.
