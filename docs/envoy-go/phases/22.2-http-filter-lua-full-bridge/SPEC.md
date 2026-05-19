# Phase 22.2 SPEC — `envoy.filters.http.lua` (full Envoy↔Lua bridge surface delta)

> **Lifecycle state:** SPEC.md authored; ROADMAP row `22.2` stays `in-progress` (parent row `22` stays `in-progress` per ADR-0106 per-cell SPEC-done annotation; sub-rows `22.1` done, `22.3` planned) per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3. Successor session's skill is `superpowers:writing-plans` to author `PLAN.md` per the phase-09..21 + phase-18.1 + phase-19.1 + phase-22.1 sub-phase-SPEC → PLAN precedent. This SPEC is the authoritative input to the 22.2 PLAN.

**Parent:** `docs/envoy-go/phases/22-http-filter-lua/SPEC.md` (the parent master SPEC — §4 framework primitive sketch + §5 proto-field roster + §6 PARSE-REJECT roster + §7 stat surface template + §8 fixture taxonomy + §10 deferred items + §11 7-pin empirical-pin block + §13 RATIFIED-PENDING + §14 BEHAVIOR_CONTRACT edit bundle anticipation + §1.1 12-AMEND catalog).

**Predecessors:** `docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/BRAINSTORM.md` (14-Q dialogue + 12 §2 numbered sub-sections + framework-survey + 7 D-questions carry-forward + anticipated ADR roster + deferred items; authored at master tip `ac94a92` via squash `6ad3064`) + `docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/{SPEC.md, REVIEW.md}` (the predecessor sub-phase's SPEC + REVIEW — load-bearing precedents for structure + 22.1 IMPL inheritance state).

**Sub-phase scope (per parent SPEC §3.1 split surface-mapping + 22.2 BRAINSTORM §1.1):** 22.2 lands the FULL Envoy↔Lua bridge surface delta on top of 22.1's pragmatic-middle — every upstream-parity bridge method that 22.1 deferred via Lua-runtime-error becomes a real bridge method at 22.2. Plus two intentional envoy-go-strict scope-expansions:

- **Dynamic-metadata** pulled forward from the cross-phase deferral discipline (phases 16/17/18/19/20 deferred independently; 22.2 lifts via NEW `internal/dynamicmetadata/` primitive — per BRAINSTORM Q3 + §1.6).
- **Filter-state in-package** at the lua filter ahead of any cross-filter primitive landing (per BRAINSTORM Q8 + Q9 EXTRACT-NOW-only-when-trigger-fires; second consumer triggers `internal/filterstate/` extraction at a future phase).

22.2 surface delta (8 bridge surfaces + transverse decisions):

1. **Body bridge** (Q1) — `:body()` whole-buffer + `:bodyChunks()` chunked iterator + Lua coroutine yield/resume + zero-copy buffer-sharing seam with ADR-0128 decode-side primitive. Defensive copy at endStream per §11.3 D3 RECOMMENDATION (carries forward to PLAN for perf-benchmark validation).
2. **Trailers bridge** (Q2) — `:trailers()` mirrors the 22.1 headers metatable exactly (8 mutation methods + `__pairs` reusing 22.1's `installPairsShim` discipline); lazy-available (returns nil if no trailers received yet).
3. **Metadata bridge** (Q3) — `:metadata()` callable userdata wrapping empty metadata source at v1.32.4 binding-gap (per §11.6 D1 closure; NEVER nil per upstream `MetadataMapWrapper` pattern); `:streamInfo():dynamicMetadata()` + `:streamInfo():dynamicTypedMetadata()` via NEW `internal/dynamicmetadata/` framework primitive (ADR-0190). BREAKS cross-phase dynamic-metadata deferral discipline (per §1.6 + BRAINSTORM §1.6).
4. **Connection bridge** (Q4) — `:connection():ssl()` ~12-method cert/session surface (subject + SANs local/peer + validFromPeer + expirationPeer + sessionId + ciphersuiteId + tlsVersion + urlEncodedPemEncodedPeerCertificate + urlEncodedPemEncodedPeerCertificateChain + sha256PeerCertificateDigest); returns nil on plaintext; integrates with phase-03 TLS primitives via NEW `FilterChain.tlsConnectionState *tls.ConnectionState` field per §11.5.
5. **httpCall bridge** (Q5) — `:httpCall(cluster, headers, body, timeout_ms, asynchronous?)` cluster-based dispatch + sync-default + optional async flag; consumer-#1 of `internal/httpclient/` per ADR-0177 (triggers in-place AMEND on ADR-0177 for `ClusterDispatch` extension per §11.4). Async = pure fire-and-forget per §11.7 D6 closure.
6. **Crypto bridge** (Q6 + §11.2 AMEND-22.2-1) — 6 Lua-callable crypto methods in-package at `internal/filter/http/lua/crypto.go`: `:base64Escape` + `:base64Decode` + `:sha256` + `:sha512` + `:importPublicKey` + `:verifySignature`. `:base64Escape` is upstream-parity (Go `encoding/base64.StdEncoding`); the other 5 are RATIFIED-PENDING-PLAN re-scrape — Pin 2 found NO upstream stream-handle exposure; may exist on separate upstream wrappers OR be envoy-go-strict extensions (per §13-R7 follow-up).
7. **Filesystem + clock bridge** (Q7 + §11.2 AMEND-22.2-1) — `:fileBytes(path)` unrestricted FS + 16 MiB cap (inherits 22.1 Task 11 cap pattern from `DataSource.Filename`) + `:timestamp(unit?)` wall-clock with millisecond default. `:fileBytes` is RATIFIED-PENDING-PLAN re-scrape — Pin 2 found NO upstream stream-handle exposure (likely envoy-go-strict extension; final disposition per §13-R8 follow-up).
8. **streamInfo-full** (Q8) — 7 additional methods on top of 22.1's 4-method subset: `:upstreamHost()` + `:upstreamCluster()` + `:dynamicMetadata()` + `:dynamicTypedMetadata(filter_name)` + `:requestedServerName()` + `:filterState()` + `:downstreamSslConnection()`. Total 11-method streamInfo surface at 22.2 phase-done. `:filterState()` lands IN-PACKAGE per Q9 (string-keyed `map[string]any` per §11.8 D4 closure) with 2 envoy-go-strict divergences from upstream: `:set(name, value)` mutation exposed (upstream is read-only) + typed Lua-value marshaling at `:get(name)` (upstream returns serializeAsString strings). Per AMEND-22.2-4.

Plus transverse decisions:

- **Framework extraction** (Q9): NEW `internal/dynamicmetadata/` framework primitive (ADR-0190) + NEW `internal/lua/` 22.2 API extensions for coroutine yield/resume + body-bridge buffer seam (ADR-0191; strict scope per Q10) + IN-PLACE AMENDMENT on ADR-0177 for `internal/httpclient/` cluster-based dispatch path. 4 IN-PACKAGE: filter-state + body-buffer seam (consumer-side) + connection-SSL + crypto + fileBytes + timestamp.
- **Stat surface delta** (Q11): 5-6 NEW envoy-go-strict counters (102 → ~108): `httpcall_total` + `httpcall_failures` + `httpcall_timeouts` + `body_buffered_bytes_total` + `coroutine_yields_total`. Per §11.7 D6 closure, `httpcall_failures` + `httpcall_timeouts` are SYNC-ONLY (async fire-and-forget is invisible to script + filter-stats per upstream parity).
- **Fixture-0027 strategy** (Q12): single mixed-mode `0027-http-lua-full-bridge` directory with deterministic cross-side scenarios (body / trailers / metadata-empty / crypto / streamInfo-most / fileBytes / connection-ssl-cert-fingerprint per §11.5 D5 carry-forward) + non-deterministic REFERENCE-LESS scenarios (httpCall / timestamp / filterState).
- **D-hypothesis** (Q13): WEAK HOLD — 3 anticipated NEW ADRs (ADR-0190 + ADR-0191 + ADR-0192) + in-place AMEND on ADR-0177 + 0-1 conditional ADR-0193 escape-valve.
- **Scope shape** (Q14): single-phase at SPEC commit; PLAN-stage decides if split per ADR-0045 (~25-35 tasks / ~3000-5000 LoC estimate).

**22.3 (multi-script `SourceCodes` + per-route `LuaPerRoute` 3-arm oneof + NEW 9th canonical per-route shape ADR + ADR-0125 §(xiv) IN-PLACE AMENDMENT roster 8 → 9) is OUT OF SCOPE for 22.2.**

**ADR continuity:** Phase 22.1 IMPL closed at ADR-0189 §Decision + §Consequences body lands. **At THIS 22.2 SPEC commit: 3 NEW ADR §Context drafts anchor** (ADR-0190 + ADR-0191 + ADR-0192) per ADR-0044 §Context-draft discipline. §Decision + §Consequences bodies LAND at 22.2 IMPL atomic-landing Task per ADR-0044 in-place edit discipline. **In-place AMENDMENT body on ADR-0177** for `ClusterDispatch` extension also lands at 22.2 IMPL (no new ADR number consumed; matches phase-17 → phase-18 ADR-0149 → ADR-0150 AMEND precedent). **Conditional ADR-0193** (escape-valve slot per Q13 WEAK HOLD) — anchors AT 22.2 IMPL only if a §13-R surface fires. **Next-free ADR after THIS 22.2 SPEC commit: `ADR-0193`** (3 numbers consumed: ADR-0190 + ADR-0191 + ADR-0192 §Context drafts).

**Authored:** 2026-05-19.

**Base commit:** `ac94a92` (master tip at session entry; phase 22.2 BRAINSTORM SHA-fill follow-up; predecessor squash `6ad3064` = phase 22.2 BRAINSTORM atomic landing).

---

## 1. Purpose / Mission

Phase 22.2 lands the FULL Envoy↔Lua bridge surface delta on top of 22.1's pragmatic-middle, taking parent BRAINSTORM Q1 envelope D to its conclusion. By 22.2 phase-done every upstream-parity bridge method is active across 8 surface families (body / trailers / metadata / connection-SSL / httpCall / crypto / fileBytes+timestamp / streamInfo-full), plus two intentional envoy-go-strict scope-expansions (dynamic-metadata pulled forward + filter-state in-package).

The seven architectural primitives landing at 22.2:

1. **NEW `internal/dynamicmetadata/` framework primitive** — per-stream `*Bucket` accessor with `Get(filter_name, key) → (proto.Value, bool)` + `Set(filter_name, key, proto.Value)` + `Snapshot() map[string]map[string]proto.Value`. Per-stream lifecycle (created at filter-chain entry; destroyed at OnDestroy). Cross-filter visibility per-stream (no cross-stream cache). Anchored at ADR-0190. BREAKS the cross-phase dynamic-metadata deferral discipline (phases 16/17/18/19/20 deferred independently — see §1.6 for the discipline-lift expectation).

2. **NEW `internal/lua/` API extensions** (coroutine yield/resume + body-bridge buffer seam) — anchored at ADR-0191. Per Q10 strict scope: NEW ADR (NOT in-place AMEND on ADR-0188 — ADR-0188's API-REVISION ALLOWANCE clause STAYS scoped to consumer-#2). Coroutine extensions land per the §11.1 D2 closure (gopher-lua native `LState.NewThread()` + `LState.Yield()` returning `-1` sentinel + parent `LState.Resume()` from Go-side callback; 1 parent `*LState` per stream + 1 child `*LState` per phase invocation).

3. **NEW `internal/filter/http/lua/` 22.2 package shape extensions** — anchored at ADR-0192. Adds 7 NEW files (body.go + trailers.go + metadata.go + connection.go + httpcall.go + crypto.go + misc.go) + extends 3 existing 22.1 files (bridge.go + streaminfo.go + filterstate.go-new) + extends compiled_config.go (3 new PARSE-REJECT arms per §6). Per §11.8 D4 disposition, filter-state is in-package `map[string]any` + per-stream context field; `:filterState():get/set` marshaling via gopher-lua `LValue` conversion.

4. **IN-PLACE AMEND on ADR-0177** (`internal/httpclient/` cluster-based dispatch) — adds `(c *Client) ClusterDispatch(ctx, clusterName, request, clusterMgr *cluster.Manager) (*http.Response, error)` per §11.4 evidence. Resolves cluster name via `clusterMgr.Get(name)` + selects endpoint via `Cluster.PickEndpoint()` + rewrites `request.URL.Host` to endpoint addr + constructs temp `*http.Client` honoring cluster TLS config. Lua filter's `*compiledConfig` receives `*cluster.Manager` via NEW `FactoryCtx.ClusterManager` field paralleling existing `FactoryCtx.HTTPClient` threading.

5. **NEW `FilterChain.tlsConnectionState *tls.ConnectionState` field** + setter + getter symmetric to existing TLS-principals plumbing — extends ADR-0144 pattern (per §11.5 evidence). H1 (`internal/filter/hcm/connection.go`) + H2 (`internal/filter/hcm/h2dispatch.go`) both seed before `RunDecodeHeaders`. NEW `DecoderFilterCallbacks.DownstreamTLSConnectionState() *tls.ConnectionState` accessor + symmetric encoder side. Lua bridge wraps raw state into Lua userdata at `:connection():ssl()` invocation. No new ADR for the chain-side extension (lives inside ADR-0192 §Decision body per Q13 WEAK HOLD).

6. **Extension-registry registration UNCHANGED** at boot per ADR-0072; 22.1 already registered `lua.New` between `localratelimit` and `oauth2` per ADR-0100 §2.2. 22.2 adds NO new HCM-filter entries (still 17 HTTP filters wired; the lua filter just gains bridge surface).

7. **3-counter 22.1 stat surface EXTENDS to 8-9 counters at 22.2 phase-done** under the existing `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.<stat>` template (UNCHANGED from 22.1 AMEND-2). 5 NEW envoy-go-strict counters: `httpcall_total` + `httpcall_failures` + `httpcall_timeouts` + `body_buffered_bytes_total` + `coroutine_yields_total`. Optional 6th `dynmd_writes_total` deferred per §7 RECOMMENDATION (omitted from 22.2 pragmatic-middle unless 22.2 IMPL surfaces operator-value signal). Project stat-count delta 102 → 107 (or 108 with optional 6th).

After phase 22.2, the project has the FULL `envoy.filters.http.lua` bridge surface: every upstream-parity bridge method exposed; dynamic-metadata accessible via NEW `internal/dynamicmetadata/` primitive (cross-phase deferral-lift surface for future phases); coroutine yield/resume integrated with HCM's per-stream dispatch model; cluster-based httpCall via in-place AMENDMENT on `internal/httpclient/`; OBSERVABLE-OUTCOMES byte-equivalent to reference Envoy v1.37.2 on the deterministic fixture-0027 scenarios + REFERENCE-LESS-equivalent on the non-deterministic scenarios — modulo the 8-11 envoy-go-strict documented divergence-windows (stdlib-sandbox-strict from 22.1 + `respond_calls` from 22.1 + runtime-error log-message wording from 22.1 + httpcall_total/failures/timeouts + body_buffered_bytes_total + coroutine_yields_total + possible crypto/fileBytes envoy-go-strict records pending §13-R7+R8 PLAN-time scrape).

Phase 22.3 then activates the multi-script `SourceCodes` + the `LuaPerRoute` 3-arm oneof + the NEW 9th canonical per-route shape ADR + the ADR-0125 §(xiv) IN-PLACE AMENDMENT body against the same package surface.

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The 9 §11 empirical pins (executed at this SPEC session via parallel-subagent fan-out against gopher-lua master + upstream Envoy v1.37.2 + local envoy-go codebase per ADR-0004) generated the following **4 amendment-block entries** load-bearing for 22.2:

- **AMEND-22.2-1 (gopher-lua-vs-LuaJIT divergences for 22.2 bridge methods — EXTENDS parent SPEC AMEND-9):** Per §11.2 scrape. Body-byte fidelity CONFIRMED-IDENTICAL (gopher-lua `LString` = Go `string`; byte-safe; multi-MB tolerant). Trailers `pairs(udata)` CONFIRMED-IDENTICAL via 22.1's `installPairsShim` discipline (the 22.1 IMPL's pairs-shim makes gopher-lua honor `__pairs` metamethod on userdata — `bridge.go:305-345`; 22.2 inherits unchanged for trailers). Coroutine error propagation inherits 22.1 AMEND-9(c) `pcall` prefix divergence (no NEW BEHAVIOR_CONTRACT.md departure record needed; extend AMEND-9(c) wording to "applies equally to errors surfaced via coroutine resume from `:body()`/`:bodyChunks()`/`:httpCall()`"). `:base64Escape` wire-output is byte-identical to upstream (Go `encoding/base64.StdEncoding` matches `absl::Base64Escape` standard-padding output). The other 5 crypto methods + `:fileBytes` warrant separate AMEND-22.2-2.

- **AMEND-22.2-2 (crypto + fileBytes upstream-exposure scrape — PARTIAL-REFUTE BRAINSTORM Q6 + Q7 "full upstream parity" framing):** Per §11.2 scrape. Upstream Envoy v1.37.2 `StreamHandleWrapper` (`source/extensions/filters/http/lua/lua_filter.h:226-357` per `DECLARE_LUA_FUNCTION` grep) exposes ONLY `:base64Escape` on the stream-handle. `:sha256`, `:sha512`, `:base64Decode`, `:importPublicKey`, `:verifySignature`, `:fileBytes` are NOT on `StreamHandleWrapper`. PARTIAL-REFUTATION of BRAINSTORM Q6 + Q7 "full upstream parity" framing: these methods may exist on separate upstream wrappers at a different scope (script-global, or `PublicKeyWrapper` userdata-returning), OR they're envoy-go-strict extensions. **DISPOSITION at 22.2 SPEC commit:** the BRAINSTORM §2.6 + §2.7 in-package landing STANDS (envoy-go exposes them as request_handle/response_handle methods regardless of upstream's exposure scope — operationally equivalent for script authors). **§13-R7 RATIFIED-PENDING-PLAN:** 22.2 PLAN session does a targeted upstream re-scrape against `PublicKeyWrapper` + `CryptoUtility` + script-global helpers to confirm upstream-equivalence vs envoy-go-strict-extension classification. **§13-R8 RATIFIED-PENDING-PLAN:** same for `:fileBytes` — if confirmed absent from upstream, 22.2 IMPL adds an envoy-go-strict departure record at BEHAVIOR_CONTRACT.md (raising the 5-6 record bundle to 6-7+).

- **AMEND-22.2-3 (httpCall async-flag wire-shape — REFUTES BRAINSTORM Q5 anticipation of "fire-and-forget vs caller-suspends-until-dispatch-confirmed" ambiguity):** Per §11.7 scrape. Upstream `asynchronous=true` is PURE FIRE-AND-FORGET per `lua_filter.cc:400-416 doHttpCall`: callbacks parameter wired to `noopCallbacks()` global singleton (response fully discarded); no `lua_yield` (script keeps running synchronously); `return 0` (zero return values to script); test `HttpCallAsynchronous` (`lua_filter_test.cc:1232-1283`) confirms `decodeHeaders` returns `Continue` not `StopIteration`. Test header comment: *"Basic asynchronous, fire-and-forget HTTP request flow."* Async transport-failure is UNOBSERVABLE at the filter-stats layer (would only surface in destination cluster's upstream stats). **D6 CLOSURE:** envoy-go's `:httpCall(...asynchronous=true)` dispatches via `internal/httpclient/Client.ClusterDispatch` in a fire-and-forget goroutine; script gets 0 return values; no yield; response/error discarded. **Stat-counter implication:** `httpcall_total` increments on every dispatch (sync + async); `httpcall_failures` + `httpcall_timeouts` are SYNC-ONLY (async failures invisible per upstream parity — documented in the BEHAVIOR_CONTRACT.md departure records at 22.2 IMPL).

- **AMEND-22.2-4 (`:filterState()` shape — REFUTES BRAINSTORM Q8 implied `:set()` + REFUTES typed-userdata return shape):** Per §11.8 scrape. Upstream `FilterStateWrapper` (`wrappers.h`) exposes **ONLY `:get`** — read-only surface (no `:set`/`:add`/`:remove`); return shape is `lua_pushlstring()` from FilterState object's `serializeAsString()` (NOT typed userdata or Lua tables). Test evidence: `wrappers_test.cc` `correct_string_type` assertion on `UInt64AccessorImpl(12345)` returning Lua string `"12345"`. **D4 CLOSURE + 2 envoy-go-strict divergences:** envoy-go adopts Option B (string-keyed `map[string]any` per-stream + LValue marshaling) per BRAINSTORM §3.4 BUT exposes BOTH `:get` AND `:set(name, value)` (envoy-go-strict — upstream is read-only because C++ filters mutate FilterState directly; envoy-go has no Go-side mutation analog at 22.2) AND returns native Lua values at `:get(name)` (envoy-go-strict — upstream returns serializeAsString strings always). 2 NEW envoy-go-strict departure records at BEHAVIOR_CONTRACT.md `### envoy.filters.http.lua` 22.2 sub-section — bundle scales from 8 → **10 records** at 22.2 IMPL atomic landing (modulo §13-R7+R8 PLAN-time crypto+fileBytes-classification additions).

This 22.2 SPEC's §3-§14 incorporate all 4 amendments. AMEND-22.2-2 carries forward to PLAN via §13-R7 + §13-R8 (the crypto + fileBytes upstream-exposure-verification is RATIFIED-PENDING-PLAN-TIME).

### 1.2 ADR continuity + D-hypothesis at 22.2 SPEC commit

Phase 22.1 IMPL closed at ADR-0189 full body. **At THIS 22.2 SPEC commit: 3 NEW ADR §Context drafts anchor** per ADR-0044 §Context-draft discipline:

- **ADR-0190 §Context** — NEW `internal/dynamicmetadata/` framework primitive (per Q9 + Q3 cross-phase deferral break). §Context anchored at THIS SPEC commit; §Decision + §Consequences body lands at 22.2 IMPL atomic-landing Task per ADR-0044 in-place edit discipline.
- **ADR-0191 §Context** — NEW `internal/lua/` 22.2 API extensions for coroutine yield/resume + body-bridge buffer seam at HTTP filter Lua consumer-#1 scope-expansion (per Q10 strict scope). §Context anchored at THIS SPEC commit; §Decision + §Consequences body lands at 22.2 IMPL atomic-landing Task.
- **ADR-0192 §Context** — NEW `internal/filter/http/lua/` 22.2 package shape extensions — body + trailers + metadata + connection-SSL + httpCall + crypto + fileBytes + timestamp + streamInfo-full + filter-state in-package bridge methods + 5-6 envoy-go-strict departure records + fixture-0027 mixed-mode discipline. §Context anchored at THIS SPEC commit; §Decision + §Consequences body lands at 22.2 IMPL atomic-landing Task.

**Next-free ADR after THIS 22.2 SPEC commit: `ADR-0193`** (3 numbers consumed: ADR-0190 + ADR-0191 + ADR-0192). ADR-0044 escape-valve held in reserve at `ADR-0193` for the WEAK-HOLD conditional consumption surface per Q13.

**In-place AMENDMENT body on ADR-0177** (`internal/httpclient/` cluster-based dispatch) anchored AT 22.2 IMPL atomic-landing Task per ADR-0044 in-place edit discipline — matches phase-17 → phase-18 ADR-0149 → ADR-0150 AMEND precedent. **No new ADR number consumed** for the httpclient AMEND.

**D-hypothesis at 22.2 SPEC commit:** BRAINSTORM Q13 WEAK-HOLD predicted 3 anticipated NEW ADRs + in-place AMEND on ADR-0177 + 0-1 escape-valve consumption at 22.2 IMPL. This SPEC's empirical-pin scrape produces **two surfaces that could plausibly consume the escape-valve slot** but neither is anticipated to escalate from the §Decision-body-of-existing-ADR scope:

1. **§13-R6 *LState-pool gate** — 22.1 IMPL R6 STANDS WEAK-default at ~70µs/stream (per 22.1 REVIEW §5). 22.2 may re-evaluate against the body+coroutine+filter-state bridge surface (more bridge methods + more per-stream allocation). If 22.2 IMPL benchmarks surface > 1ms per-stream construction, ADR-0193 escape-valve consumes for `*LState` pool design.

2. **§13-R9 body-buffer-seam-with-ADR-0128 separation** — the body-bridge buffer seam may surface implementation-time complexity warranting its own ADR (split from ADR-0192 in-package). Lives inside ADR-0192 §Decision body unless it surfaces enough complexity at 22.2 IMPL to warrant separation.

**SPEC-time disposition:** WEAK HOLD STANDS (UNCHANGED from BRAINSTORM Q13). 3 anticipated ADRs (ADR-0190 + ADR-0191 + ADR-0192) land cleanly + in-place AMEND on ADR-0177 + 0-1 escape-valve slot consumption at ADR-0193. ZERO-or-ONE-slot buffer post-22.2-IMPL absorbs the consumption if it fires. The 22.3 BRAINSTORM re-evaluates buffer discipline.

---

## 2. Non-purposes

Phase 22.2 is the second sub-phase of the phase-22 BRAINSTORM-time 3-way pre-split. It does NOT extend the framework beyond the minimum needed to land the full bridge surface delta + the 3 NEW ADRs + the in-place AMEND on ADR-0177.

- **2.1 `Lua.SourceCodes` named-script map OUT OF SCOPE at 22.2 (PARSE-REJECT UNCHANGED from 22.1).** 22.3 activates per parent §3.1 + parent §6.2 arm 4.
- **2.2 `LuaPerRoute` 3-arm oneof OUT OF SCOPE at 22.2 (PARSE-REJECT UNCHANGED from 22.1).** 22.3 activates with NEW 9th canonical + ADR-0125 §(xiv) IN-PLACE AMENDMENT body.
- **2.3 `Lua.InlineCode` deprecated field OUT OF SCOPE (PARSE-REJECT UNCHANGED from 22.1).** Never re-enabled in envoy-go per envoy-go-strict deprecated-field-rejection discipline + parent AMEND-6.
- **2.4 `WatchedDirectory` DataSource sibling field OUT OF SCOPE (PARSE-REJECT UNCHANGED from 22.1).** Deferred to future Runtime/RTDS/hot-reload family phase per parent §2.1.
- **2.5 `Lua.clear_route_cache` (v1.37.2 field 5) NEVER-DEFERRED at 22.2.** Per parent AMEND-12 v1.32.4 binding-gap forward-pointer. The field is ABSENT from envoy-go's consumed v1.32.4 binding; activates at the v1.37.x binding bump phase.
- **2.6 `LuaPerRoute.filter_context` (v1.37.2 field 4) NEVER-DEFERRED at 22.2.** Same disposition; v1.32.4 binding-gap. The `:metadata()` bridge surface at 22.2 returns empty-userdata regardless (per §11.6 D1 + parent AMEND-12).
- **2.7 `*LState` pool at 22.2 NEVER-DEFERRED — escape-valve at §13-R6.** Per-stream `*LState` construction with shared `*Chunk` cache remains the WEAK-default per 22.1 R6 + §1.2 ADR-0193 escape-valve hypothesis (a).
- **2.8 Filter-state cross-stream cache OUT OF SCOPE.** Per-stream string-keyed `map[string]any` only; no cross-stream sharing (matches upstream parity — filter-state is per-stream).
- **2.9 `internal/filterstate/` framework primitive extraction OUT OF SCOPE.** Per BRAINSTORM Q8 + Q9 EXTRACT-NOW-only-when-trigger-fires. Filter-state lives IN-PACKAGE at `internal/filter/http/lua/filterstate.go` at 22.2. Second consumer of filter-state (future cross-filter passing) triggers `internal/filterstate/` extraction at that future phase.
- **2.10 HMAC + SHA-1 + base64-URL crypto extensions NEVER-DEFERRED.** BRAINSTORM Q6 chose 6-method upstream-equivalent envelope over 10-method envoy-go-strict extension. Future phases may add per operator-pattern demand.
- **2.11 `:connection()` non-SSL methods (`:remoteAddress` / `:remoteIp` / `:remotePort`) OUT OF SCOPE.** Upstream Envoy v1.37.2 `:connection()` is scoped to SSL accessor only; per-connection address surface lives on `:streamInfo():downstreamLocalAddress()` + `:downstreamDirectRemoteAddress()` (22.1 + 22.2 ships).
- **2.12 `:metadata()` per-route source activation OUT OF SCOPE at 22.2.** The bridge surface is callable (returns empty userdata); source-of-data flips on at v1.32.4 → v1.37.x binding bump phase per parent AMEND-12.
- **2.13 Cross-filter `internal/dynamicmetadata/` consumer adapter OUT OF SCOPE.** 22.2 lands the primitive + consumes it as consumer-#1 (the lua bridge). Phases 16/17/18/19/20's "operator-visibility deferred to future" BEHAVIOR_CONTRACT.md notes stay AS-IS until their respective next-touchpoint phases (lift-phase converts the note from "deferred" to "lifted via `internal/dynamicmetadata`").
- **2.14 `internal/httpclient/Client` URL-based `Do(*http.Request)` API UNCHANGED.** The existing API surface remains; 22.2 ADDS `ClusterDispatch(...)` as a NEW method (per the in-place AMENDMENT body on ADR-0177). Phase-20 oauth2 consumer continues consuming the URL-based path unchanged.
- **2.15 No `response_code_details` emission** — unchanged from phase-16..22.1; envoy-go's HCM does not surface `response_code_details` to local-reply callers (phase-04 scope). Documented divergence-window joint with prior §9 rows.
- **2.16 No filter-chain ordering surgery.** 22.2 extends bridge surface on the existing 22.1 lua filter entry; HCM filter-chain iteration protocol unchanged.
- **2.17 Framework REUSES NOT consumed beyond the named extensions.** ADR-0144 EXTENDED with new `FilterChain.tlsConnectionState` field (lives in ADR-0192 §Decision body; no new ADR for the chain-side extension per Q13 WEAK HOLD). ADR-0150 jwks NOT consumed. ADR-0151 jwt verifier NOT consumed. ADR-0178 `internal/sdsfile/` NOT consumed. ADR-0158 `internal/grpcclient/` NOT consumed. ADR-0165 + ADR-0174 NOT consumed. ADR-0186 `Clock` seam NOT consumed (the 22.2 `:timestamp()` uses `time.Now()` directly per BRAINSTORM §2.7 — non-deterministic by design; the Clock seam would force a synthetic-time injection complexity not warranted at 22.2).
- **2.18 MVP confirmations (positive consumption assertions for 22.2).** All 22.1 surfaces stay consumed (no regressions). 22.2 adds: full bridge surface delta (8 surface families); NEW `internal/dynamicmetadata/` primitive; NEW `internal/lua/` 22.2 API extensions; in-place AMENDMENT on ADR-0177; NEW `FilterChain.tlsConnectionState` field; 5 NEW envoy-go-strict counters; 1 NEW differential fixture `0027-http-lua-full-bridge`; 1-2 NEW project-wide fuzzers (`FuzzLuaBodyBridge` + `FuzzLuaHTTPCallConfig` — count finalized at PLAN per §13-R10).

---

## 3. Framework primitive (NEW `internal/dynamicmetadata/` + EXTEND `internal/lua/` + EXTEND `internal/filter/http/lua/` + IN-PLACE AMEND ADR-0177)

Per parent SPEC §4 + BRAINSTORM Q9 + AMEND-22.2-1..-3. 22.2 introduces ONE NEW framework primitive at the package level (`internal/dynamicmetadata/`) + extends two existing primitives (`internal/lua/` via NEW ADR-0191 + `internal/httpclient/` via in-place AMENDMENT on ADR-0177) + extends the `internal/filter/http/lua/` package via NEW ADR-0192. The chain-side `tlsConnectionState` extension lives inside ADR-0192 (no separate ADR per Q13 WEAK HOLD).

### 3.1 NEW `internal/dynamicmetadata/` framework primitive (ADR-0190; lands at 22.2 IMPL)

Per Q9 + §11 BRAINSTORM §1.6 cross-phase deferral-break. Package boundary: `internal/dynamicmetadata/` hosts the per-stream `*Bucket` accessor + map keyed by `(filter_name string, key string) → *structpb.Value` (or equivalent typed value). Consumers (lua filter at 22.2; future jwt_authn / rbac / ext_proc / ext_authz extensions) consume the API via their per-stream context.

Production signatures (lands at IMPL Task per 22.2 PLAN):

```go
// internal/dynamicmetadata/dynamicmetadata.go — Bucket type + accessor API

package dynamicmetadata

import structpb "google.golang.org/protobuf/types/known/structpb"

// Bucket is a per-stream cross-filter dynamic-metadata accessor.
// Lifecycle: created at filter-chain entry; destroyed at OnDestroy.
// Per-stream sequential per ADR-0033 (no cross-filter concurrency within
// a stream); NOT goroutine-safe across streams.
type Bucket struct {
    // unexported fields:
    // m map[string]map[string]*structpb.Value
}

// NewBucket constructs an empty per-stream metadata bucket.
func NewBucket() *Bucket

// Get returns the value at (filter_name, key), ok=false if absent.
// nil bucket tolerant: returns (nil, false) per ADR-0085 nil-tolerance.
func (b *Bucket) Get(filterName, key string) (*structpb.Value, bool)

// Set writes the value at (filter_name, key). Overwrites any prior value
// at the same coordinate. nil bucket tolerant: no-op.
func (b *Bucket) Set(filterName, key string, value *structpb.Value)

// Snapshot returns a copy of the bucket's contents for read-only iteration
// (consumed by Lua bridge's :dynamicTypedMetadata() typed-iteration access).
// Returns nil for nil bucket.
func (b *Bucket) Snapshot() map[string]map[string]*structpb.Value

// Reset clears all entries (consumed at OnDestroy).
func (b *Bucket) Reset()
```

**Lifecycle integration:** the FilterChain (`internal/filter/http/chain.go`) gains a new `dynamicMetadata *dynamicmetadata.Bucket` field. At chain construction (per-stream entry), the field is initialized via `dynamicmetadata.NewBucket()`. At OnDestroy, `chain.dynamicMetadata.Reset()` is called. The filter-callback API surface (`internal/filter/http/callbacks.go`) gains TWO new accessors:

- `DecoderFilterCallbacks.DynamicMetadata() *dynamicmetadata.Bucket` (returns the per-stream bucket)
- `EncoderFilterCallbacks.DynamicMetadata() *dynamicmetadata.Bucket` (returns same bucket — per-stream shared across decode + encode)

**File split** (3 production + 2 test files):

```
internal/dynamicmetadata/
  doc.go              # package overview + cross-phase deferral-lift rationale
                      # + API surface summary + ADR-0190 cross-reference
  dynamicmetadata.go  # Bucket type + NewBucket + Get + Set + Snapshot + Reset
  dynamicmetadata_test.go  # exhaustive table-driven tests including nil-tolerance
  bench_test.go       # microbenchmarks for Get/Set under concurrent-read (per-stream sequential)
```

### 3.2 EXTEND `internal/lua/` API for coroutine yield/resume + body-bridge buffer seam (NEW ADR-0191; lands at 22.2 IMPL)

Per Q10 strict scope + §11.1 D2 closure (gopher-lua native coroutines confirmed). NEW ADR (NOT in-place AMEND on ADR-0188 — the API-REVISION ALLOWANCE in ADR-0188 stays scoped to consumer-#2 per Q10). NEW production signatures appended to `internal/lua/vm.go`:

```go
// NewThread constructs a child *LState as a coroutine state, sharing globals
// with the parent. Per §11.1 D2 closure: gopher-lua native LState.NewThread()
// is the underlying mechanism. The returned *LState is the coroutine state;
// callers Resume it via the parent's Resume method. Cancel function from
// gopher-lua's NewThread MUST be invoked at coroutine cleanup.
func (vm *VM) NewThread() (*lua.LState, context.CancelFunc)

// Resume invokes parent.Resume(child, fn, args...) per gopher-lua semantics.
// Per §11.1 D2 closure: the parent *LState drives the child; on ResumeYield
// returns with the yield args, on ResumeError returns the API error, on
// ResumeOK returns the script's return values.
func (vm *VM) Resume(child *lua.LState, fn *lua.LFunction, args ...lua.LValue) (lua.ResumeState, error, []lua.LValue)

// YieldFromBridge is a Go-side helper for bridge LGFunction implementations.
// Pushes args onto the bridge LState and returns the gopher-lua yield sentinel
// (-1). The caller (an LGFunction) MUST return the result directly to gopher-lua
// per vm.go:200-210 switchToParentThread discipline (callGFunction sees gfnret<0
// and unwinds to the parent thread).
func YieldFromBridge(L *lua.LState, args ...lua.LValue) int

// BodyBuffer is the seam interface consumed by the lua bridge's :body() +
// :bodyChunks() methods. Implemented by the per-stream body-bridge wrapper
// at internal/filter/http/lua/body.go that consumes ADR-0128's decode-side
// buffer (HCM-level bodyBuf accumulation; defensive copy at endStream per
// §11.3 D3 recommendation).
type BodyBuffer interface {
    // Bytes returns the full accumulated body as one byte slice.
    // Returns nil if body not yet available; the slice MUST NOT be mutated
    // by consumers (treat as read-only).
    Bytes() []byte

    // Chunks returns the body as a sequence of per-DecodeData chunks.
    // Returns nil if body not yet available. Each inner slice is read-only.
    Chunks() [][]byte

    // EndStream reports whether the terminal endStream=true signal has fired.
    // Returns false until the body is fully accumulated.
    EndStream() bool
}
```

The `BodyBuffer` interface is the integration point between `internal/lua/` API + ADR-0128's HCM-level decode-side buffer accumulation. The lua bridge (at `internal/filter/http/lua/body.go`) constructs a concrete `*decodedBody` struct implementing `BodyBuffer` that wraps per-filter accumulated chunks (paralleling ext_authz's `f.body` + ext_proc's `f.decodeBodyBuf` patterns per §11.3 evidence). At endStream, the bridge's `:body()` method makes a defensive copy of the accumulated bytes into a Go string via `lua.LString(string(f.decodedBodyBytes))` — Lua owns the resulting Go string (immutable; safe across coroutine yield/resume per §11.3 D3 RECOMMENDED disposition).

`YieldFromBridge` is the bridge-method helper that codifies the gopher-lua VM unwinding discipline per §11.1 D2 closure — bridge LGFunctions that need to suspend the script (e.g., `:body()` when body not yet buffered) call `YieldFromBridge(L, lua.LNil)` and return its result; the gopher-lua VM's `callGFunction` (`vm.go:200-210`) sees the `-1` sentinel and calls `switchToParentThread`. The bridge then stashes the suspended `*LState` in a per-stream pending-map; the matching `vm.Resume(child, nil, lua.LString(bodyBytes))` is invoked from the Envoy decode-data callback when the body is available.

### 3.3 IN-PLACE AMEND on ADR-0177 — `internal/httpclient/` cluster-based dispatch (lands at 22.2 IMPL)

Per Q5 + Q9 + §11.4 evidence. In-place AMENDMENT body on ADR-0177 §Decision at 22.2 IMPL atomic-landing Task per ADR-0044 in-place edit discipline (matches phase-17 → phase-18 ADR-0149 → ADR-0150 AMEND precedent). No new ADR number consumed.

NEW production method appended to `internal/httpclient/httpclient.go`:

```go
// ClusterDispatch dispatches an HTTP request to an upstream endpoint selected
// from the named cluster via the cluster manager's load balancer.
//
// Resolves clusterName via clusterMgr.Get(name) (returns errClusterNotFound
// on lookup miss); selects endpoint via Cluster.PickEndpoint(); rewrites
// request.URL.Host to the endpoint's "host:port" form so the underlying
// http.Client dials the LB-selected endpoint.
//
// Honors per-cluster TLS via cluster.UpstreamTLSConfig() — constructs a
// temporary *http.Client with the cluster's TLS config + the receiver's
// Options timeout/retry. (This per-call construction is acceptable at the
// lua httpCall surface; phase-20 oauth2's shared-singleton lifecycle uses
// the URL-based Do() path unchanged.)
//
// Returns (*http.Response, error) per the existing Do() contract; the
// retry loop applies identically (status-driven retry on RetryOnStatus).
//
// Thread-safe: cluster manager is read-only at runtime; concurrent
// ClusterDispatch calls are safe.
func (c *Client) ClusterDispatch(ctx context.Context, clusterName string, request *http.Request, clusterMgr *cluster.Manager) (*http.Response, error)
```

The `*cluster.Manager` parameter is threaded explicitly to keep the httpclient package decoupled from cluster-manager singletons. The lua filter receives the `*cluster.Manager` reference at filter-construction time via a NEW `FactoryCtx.ClusterManager` field — paralleling the existing `FactoryCtx.HTTPClient` threading at `cmd/envoy-go/main.go` (per phase-20 oauth2 precedent). The lua filter's `*compiledConfig` captures both references; the `:httpCall` bridge closure consumes them.

### 3.4 EXTEND `internal/filter/http/lua/` 22.2 package shape extensions (NEW ADR-0192; lands at 22.2 IMPL)

Per Q1-Q8 + Q11 + Q12. NEW ADR documenting the full 22.2 bridge surface delta + filter-state in-package + crypto in-package + connection-SSL in-package + fixture-0027 mixed-mode discipline + 5-6 envoy-go-strict departure records. File split per §3.5 below.

### 3.5 `internal/filter/http/lua/` 22.2 file roster

Extends 22.1's 8 production + 5 test files. 22.2 adds 7 NEW production files + 7 NEW test files; extends 4 existing files (compiled_config.go + bridge.go + streaminfo.go + lua.go).

```
internal/filter/http/lua/  (22.2 — extends 22.1's roster)
  doc.go                      # EXTENDED: 22.2 BRAINSTORM Q1-Q14 decision summary +
                              # AMEND-22.2-1..-3 cross-references + D1-D7 cross-refs
  lua.go                      # EXTENDED: factory threads FactoryCtx.ClusterManager
                              # + FactoryCtx.HTTPClient into *compiledConfig
  compiled_config.go          # EXTENDED: +3 PARSE-REJECT arms (20 → 23 roster)
                              # per §6: httpcall-cluster-required + body-size-cap
                              # + crypto-key-format-invalid
  datasource.go               # UNCHANGED from 22.1 (no new DataSource arms)
  bridge.go                   # EXTENDED: trailers metatable factory (reuses 22.1's
                              # headers metatable shape per BRAINSTORM §2.2);
                              # __pairs/installPairsShim discipline UNCHANGED from 22.1
  decode_headers.go           # EXTENDED: registers full bridge surface on request_handle
                              # userdata at DecodeHeaders entry
  encode_headers.go           # EXTENDED: registers full bridge surface on response_handle
                              # userdata at EncodeHeaders entry
  body.go                     # NEW: :body() + :bodyChunks() bridge methods +
                              # decodedBody struct implementing BodyBuffer interface +
                              # per-filter f.decodedBodyBytes accumulation +
                              # endStream defensive-copy discipline per §11.3 D3
  trailers.go                 # NEW: :trailers() bridge method + 8 mutation methods
                              # reusing 22.1's headers metatable factory; lazy-available
                              # (nil if no trailers received)
  metadata.go                 # NEW: :metadata() bridge method returning callable
                              # empty userdata at v1.32.4 binding-gap per §11.6 D1
  connection.go               # NEW: :connection() bridge method + ssl wrapper
                              # construction from FilterChain.tlsConnectionState
  ssl.go                      # NEW: 12-method ssl wrapper (subject + SANs + valid*
                              # + sessionId + ciphersuiteId + tlsVersion + pem* + sha256*)
                              # consuming *tls.ConnectionState directly
  httpcall.go                 # NEW: :httpCall(cluster, headers, body, timeout, async?)
                              # bridge method + sync coroutine yield/resume via
                              # internal/lua YieldFromBridge + Client.ClusterDispatch
                              # + async fire-and-forget goroutine dispatch per §11.7 D6
  crypto.go                   # NEW: :base64Escape + :base64Decode + :sha256 + :sha512
                              # + :importPublicKey + :verifySignature bridge methods
                              # (thin wrappers over Go crypto/* + encoding/base64)
  misc.go                     # NEW: :fileBytes(path) + :timestamp(unit?) bridge methods
                              # (16 MiB cap via io.LimitReader per 22.1 Task 11 pattern)
  filterstate.go              # NEW: in-package filter-state per-stream string-keyed
                              # map[string]any + :filterState():get/set bridge methods
                              # per §11.8 D4 disposition
  streaminfo.go               # EXTENDED from 22.1's 4-method subset: 7 additional methods
                              # (upstreamHost + upstreamCluster + dynamicMetadata +
                              # dynamicTypedMetadata + requestedServerName + filterState +
                              # downstreamSslConnection)
  stats.go                    # EXTENDED: +5 envoy-go-strict counters (httpcall_total +
                              # httpcall_failures + httpcall_timeouts + body_buffered_bytes_total
                              # + coroutine_yields_total) per §7
  lua_test.go                 # UNCHANGED structure; extends scope to integration
  compiled_config_test.go     # EXTENDED: +3 new PARSE-REJECT arm table-driven tests
  datasource_test.go          # UNCHANGED
  bridge_test.go              # EXTENDED: trailers + connection + httpcall + crypto tests
  body_test.go                # NEW: :body + :bodyChunks + decodedBody + coroutine
                              # yield/resume + defensive-copy cross-run-determinism tests
  metadata_test.go            # NEW: :metadata + :streamInfo():dynamicMetadata tests
  ssl_test.go                 # NEW: ssl wrapper + 12 cert-method tests with mock
                              # *tls.ConnectionState fixtures
  httpcall_test.go            # NEW: :httpCall sync + async + cluster-resolution-failure
                              # + timeout + retry path tests with fakeClusterManager
  crypto_test.go              # NEW: 6 crypto bridge method tests with golden-byte vectors
  misc_test.go                # NEW: :fileBytes (16 MiB cap) + :timestamp tests
  filterstate_test.go         # NEW: :filterState get/set + Lua-table marshaling tests
                              # via gopher-lua LValue conversion
  fuzz_test.go                # EXTENDED: +1-2 fuzzers per §13-R10 (FuzzLuaBodyBridge +
                              # FuzzLuaHTTPCallConfig anticipated; final count at PLAN)
```

22.2 phase-done production-LoC estimate: ~3500-5000 (matches BRAINSTORM Q14 ~25-35-task envelope). PLAN session does precise estimation against ADR-0045 split-gate.

---

## 4. Framework primitive shapes — cross-references

The 4 framework-primitive shapes at 22.2 are anchored in §3 (above). Cross-reference table:

| Primitive | Surface | ADR anchor at SPEC | §Decision lands |
|---|---|---|---|
| NEW `internal/dynamicmetadata/` | per-stream Bucket accessor | ADR-0190 §Context (THIS commit) | ADR-0190 §Decision @ 22.2 IMPL |
| EXTEND `internal/lua/` (coroutine + BodyBuffer) | VM.NewThread/Resume + YieldFromBridge + BodyBuffer interface | ADR-0191 §Context (THIS commit) | ADR-0191 §Decision @ 22.2 IMPL |
| IN-PLACE AMEND ADR-0177 (`internal/httpclient/` cluster dispatch) | Client.ClusterDispatch method | (no new ADR; AMEND body) | AMENDMENT body @ 22.2 IMPL |
| EXTEND `internal/filter/http/lua/` (22.2 bridge package shape) | 7 NEW files + 4 EXTENDED files | ADR-0192 §Context (THIS commit) | ADR-0192 §Decision @ 22.2 IMPL |
| EXTEND ADR-0144 (FilterChain.tlsConnectionState field) | new field + setter + accessor | (no new ADR; inside ADR-0192) | inside ADR-0192 §Decision @ 22.2 IMPL |

### 4.1 Coroutine + body-buffer wiring discipline (per §11.1 D2 + §11.3 D3)

Per §11.1 D2 closure: the per-stream `*lua.LState` from 22.1 is the **parent** LState (script-bytecode owner). At each Envoy phase entry (DecodeHeaders firing `envoy_on_request`; EncodeHeaders firing `envoy_on_response`), the bridge calls `parent.NewThread()` (per gopher-lua `state.go:1614`) to mint a child `*lua.LState` as the coroutine state; then `parent.Resume(child, fn, reqHandle)` runs the operator hook inside the child. Bridge methods that await async events (`:body()`, `:httpCall()` sync, `:bodyChunks()` next-step) call `YieldFromBridge(L, lua.LNil)` from inside their LGFunction — returns the `-1` sentinel; gopher-lua's `callGFunction` (`vm.go:200-210`) unwinds via `switchToParentThread`. The Envoy data-callback that produces the awaited result (DecodeData with endStream=true; httpCall response arrival) invokes `parent.Resume(child, nil, results...)` to resume the suspended coroutine.

Per §11.3 D3 RECOMMENDATION: the body-buffer seam makes a defensive copy at endStream. The lua filter's per-stream `*filter` struct accumulates `decodedBodyBytes []byte` + `decodedBodyChunks [][]byte` across DecodeData calls (per ext_authz/ext_proc precedent). At endStream, the bridge's `:body()` returns `lua.LString(string(f.decodedBodyBytes))` — Lua owns the resulting Go string (immutable via Go string semantics; safe across coroutine yield/resume; safe across HCM dispatch goroutine lifetimes). The defensive copy cost is ≤1ms for typical sub-MB bodies — acceptable trade-off for GC safety. Final perf-validation per §13-R9.

---

## 5. Proto-field roster (cross-reference parent §5 + binding-gap forward-pointers)

22.2 consumes NO new proto fields beyond 22.1's consumption surface. The proto-field roster is UNCHANGED:

- `Lua.DefaultSourceCode` (field 3) — CONSUMED from 22.1 (UNCHANGED).
- `Lua.StatPrefix` (field 4) — CONSUMED from 22.1 (UNCHANGED; the 5 NEW counters extend the existing stat-prefix namespace).
- `Lua.SourceCodes` (field 2) — PARSE-REJECT (UNCHANGED from 22.1; deferred-to-22.3 wording).
- `Lua.InlineCode` (field 1, deprecated) — PARSE-REJECT (UNCHANGED from 22.1).
- `Lua.clear_route_cache` (field 5; v1.32.4 binding-gap) — UNCHANGED; activates at v1.37.x binding bump.
- `LuaPerRoute` (3-arm oneof) — PARSE-REJECT (UNCHANGED from 22.1; deferred-to-22.3).
- `LuaPerRoute.filter_context` (field 4; v1.32.4 binding-gap) — UNCHANGED; activates at v1.37.x binding bump.

The binding-gap forward-pointers continue to apply: `:metadata()` returns empty callable userdata regardless of future `LuaPerRoute` parsing because the source-of-data (filter_context) doesn't exist in v1.32.4 (per parent AMEND-12 + §11.6 D1 closure).

---

## 6. PARSE-REJECT roster extensions — 19-arm roster grows to 22 arms at 22.2

22.1 IMPL landed a 19-arm roster (18 from parent §6.2 + 1 from Task 11 fuzzer `arm 19` = `stat_prefix` invalid chars per `stats.IsValidName` regex). 22.2 anticipates 3 NEW arms surfacing at 22.2 IMPL atomic-landing Task:

| # | Arm | Wording (provisional; PLAN settles) | Surface |
|---|---|---|---|
| 20 | httpcall-cluster-name-required | `"lua: httpCall: cluster name must not be empty"` | `:httpCall("", ...)` runtime-reject (NOT PARSE-REJECT — runtime-rejected by gopher-lua via `luaL_error`; documented at BEHAVIOR_CONTRACT.md §13.x as a runtime-rejection) |
| 21 | body-size-cap-exceeded | `"lua: body: accumulated body exceeds maximum buffered size of %d bytes"` | Runtime-rejection at `:body()` invocation if `len(f.decodedBodyBytes) > maxBodyBufferedBytes` (16 MiB cap inherits 22.1 Task 11 cap pattern) |
| 22 | crypto-key-format-invalid | `"lua: %s: %w"` wrapping `crypto/x509.ParsePKIXPublicKey` error | Runtime-rejection at `:importPublicKey(pem)` if pem can't parse |

Per §11.2 AMEND-22.2-2: arms 20-22 are RUNTIME-REJECTS (Lua-runtime-error via `luaL_error`), NOT PARSE-REJECTs at config-load time. The 19-arm config-load PARSE-REJECT roster from 22.1 remains UNCHANGED at 22.2 config-load. 22.2 may surface additional config-load arms at Task-11-equivalent fuzzer time (per ADR-0018 fuzzer-must-never-panic discipline + 22.1 Task 11 +2-arm precedent); PLAN settles.

Cross-reference parent §6 + 22.1 SPEC §7 for the 22.1 19-arm roster verbatim.

---

## 7. Stat surface — 102 → ~107 at 22.2 IMPL (cross-reference parent §7 + 22.1 SPEC §8)

22.1 phase-done at 102 stat names (3 NEW: errors + executions + respond_calls). 22.2 adds 5 NEW envoy-go-strict counters → ~107.

### 7.1 22.2 stat-surface roster

| # | Internal name | Type | Source | Description |
|---|---|---|---|---|
| 1 | `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.httpcall_total` | counter | filter | Every `:httpCall()` invocation (sync + async) |
| 2 | `...lua.<prefix>.httpcall_failures` | counter | filter | Sync httpCall failures (4xx/5xx + transport errors). **SYNC-ONLY** per §11.7 D6 — async fire-and-forget failures invisible at filter-stats per upstream parity |
| 3 | `...lua.<prefix>.httpcall_timeouts` | counter | filter | Sync httpCall timeout firings. **SYNC-ONLY** per §11.7 D6 |
| 4 | `...lua.<prefix>.body_buffered_bytes_total` | counter | filter | Cumulative bytes accumulated in `decodedBodyBytes` across all streams (operational visibility for body-buffer capacity planning) |
| 5 | `...lua.<prefix>.coroutine_yields_total` | counter | filter | Cumulative coroutine yield events (from `:body()` + `:bodyChunks()` + `:httpCall()` sync). Operational visibility for perf debugging (yield-heavy = inefficient body-streaming patterns) |

Optional 6th counter `dynmd_writes_total` (dynamic-metadata write count) DEFERRED per BRAINSTORM Q11 pragmatic-middle — omitted unless 22.2 IMPL surfaces operator-value signal. SPEC author's call at 22.2 SPEC commit: omitted from the canonical roster; lands only if 22.2 IMPL fuzzer or fixture surface demands it. **Project stat-count delta: 102 → 107 (+5).**

### 7.2 Stat-prefix template UNCHANGED from 22.1

Template: `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.<stat>` per parent AMEND-2 + 22.1 SPEC §8 + ADR-0143 SN2-reuse. Empty `Lua.stat_prefix` produces literal consecutive-dot names mirroring phase-14 compressor empty-`<library>` precedent. UNCHANGED at 22.2.

### 7.3 envoy-go-strict departure rationale + BEHAVIOR_CONTRACT.md departure-record discipline

All 5 NEW counters are envoy-go-strict (upstream Envoy Lua filter has only 2 counters: `errors` + `executions` per `ALL_LUA_FILTER_STATS` macro at `lua_filter.h:23-24` per parent SPEC §11.5.1; per §11.7 D6 evidence). Each NEW counter requires a BEHAVIOR_CONTRACT.md envoy-go-strict departure record per the project's discipline (matches 22.1's 3-record pattern; raises bundle from 3 at 22.1 to 8 at 22.2 phase-done, modulo §13-R7+R8 PLAN-time crypto+fileBytes-record additions if AMEND-22.2-2 PARTIAL-REFUTATION confirms envoy-go-strict classification).

5 NEW envoy-go-strict departure records land at 22.2 IMPL atomic-landing Task per ADR-0052 atomic landing.

---

## 8. Differential fixture taxonomy — single mixed-mode `0027-http-lua-full-bridge` (per Q12)

### 8.1 Fixture-0027 shape

ONE fixture directory `test/fixtures/0027-http-lua-full-bridge/` at 22.2 phase-done. Both deterministic + non-deterministic scenarios in one directory; per-scenario cross-side vs REFERENCE-LESS-subject-only discipline. **28 → 29 fixture directories at 22.2 phase-done.**

### 8.2 Scenario taxonomy

| # | Scenario | Surface | Determinism | Fixture mode |
|---|---|---|---|---|
| (a) | body whole-buffer | `:body()` | deterministic | cross-side `CompareBytes` |
| (b) | body chunks iterator | `:bodyChunks()` | deterministic | cross-side `CompareBytes` |
| (c) | trailers add+remove | `:trailers()` 8-method mutation | deterministic | cross-side `CompareBytes` |
| (d) | metadata empty-userdata at binding-gap | `:metadata()` + `:get("k")` returns nil | deterministic | cross-side `CompareBytes` |
| (e) | dynamic-metadata read+write | `:streamInfo():dynamicMetadata()` | deterministic | cross-side `CompareBytes` |
| (f) | connection-ssl SAN extraction | `:connection():ssl():sanPeerCertificate()` | deterministic (cert-fingerprint-only per §11.5 D5 RECOMMENDED) | cross-side `CompareBytes` (fingerprint-only subset of ssl methods) |
| (g) | crypto sha256 + base64 | `:sha256(s)` + `:base64Escape(s)` | deterministic | cross-side `CompareBytes` |
| (h) | fileBytes read | `:fileBytes(path)` on fixed-content file | deterministic | cross-side `CompareBytes` (if §13-R8 PLAN scrape confirms upstream-equivalence) OR REFERENCE-LESS subject-only (if §13-R8 confirms envoy-go-strict) |
| (i) | streamInfo upstreamHost + upstreamCluster | `:streamInfo():upstreamHost()` + `:upstreamCluster()` | deterministic | cross-side `CompareBytes` |
| (j) | httpCall sync upstream cluster call | `:httpCall(cluster, ..., async=nil)` | non-deterministic (timing-dependent) | REFERENCE-LESS subject-only |
| (k) | httpCall async fire-and-forget | `:httpCall(cluster, ..., async=true)` returns 0 values | non-deterministic | REFERENCE-LESS subject-only |
| (l) | timestamp wall-clock | `:timestamp('milliseconds')` | non-deterministic | REFERENCE-LESS subject-only |
| (m) | filterState cross-stream set+get | `:streamInfo():filterState():set()` + `:get()` | mixed (intra-stream deterministic; cross-stream isolated) | REFERENCE-LESS subject-only |

13 scenarios at 22.2 SPEC commit; ~9-10 deterministic cross-side + ~3-4 non-deterministic REFERENCE-LESS. PLAN session may further refine.

### 8.3 Cross-side discipline (per §11.5 D5 carry-forward)

Cross-side scenarios use the existing `CompareBytes` runner step 7 (no new harness infrastructure). The REFERENCE-LESS subject-only scenarios use either the existing `BootRejectFixture` substring-match path (22.1) OR a NEW `RunSubjectOnlyHTTPLua` driver helper analogous to `BootRejectFixture` — PLAN settles per §13-R11.

Scenario (f) cross-side discipline carries forward to §12 D5: `:connection():ssl()` cross-side byte-exactness requires matching TLS certs on both reference + envoy-go sides (full cert subject + SANs + expiration + issuer). Three sub-options:

- **(f-A) full cert-matching cross-side** — both sides have identical cert files (operationally complex setup; ~150-300 LoC of fixture-cert plumbing).
- **(f-B) cert-fingerprint-only cross-side (RECOMMENDED at 22.2 SPEC)** — script extracts only `:sha256PeerCertificateDigest()` (or `:subject()` if pinned to a fixed cert); cross-side asserts the digest byte-exact. Lower operational complexity; sacrifices full cert-surface envelope-D verification for the cross-side gate.
- **(f-C) drop scenario (f) to REFERENCE-LESS subject-only** — lose envelope-D verification for SSL methods; only assert subject-side wire-output well-formed.

22.2 SPEC RECOMMENDED at scenario (f): **option (f-B) cert-fingerprint-only** — minimal cross-side cost while preserving SSL-bridge-method envelope-D verification. PLAN session ratifies + scripts the cert fixture.

### 8.4 Backend reuse + listener topology

Reuses 22.1's `BackendKind=HTTPLua` (no new BackendKind) + `scripts/` subdirectory pattern. Listener topology likely multi-listener (one listener per scenario) per the 22.1 fixture-0026 pattern + parent SPEC §8.4 recommended structure. PLAN scrubs the listener-topology decision.

### 8.5 22.3 fixture (forward-pointer)

22.3 fixture-0028 `0028-http-lua-multi-script-and-per-route`: cross-side byte-exact for deterministic multi-script + per-route scenarios per ADR-0125 9th canonical 3-arm hybrid. 22.3 BRAINSTORM/SPEC settles the exact scenario list.

---

## 9. Behavior-contract delta (cross-reference parent §9 + 22.1 SPEC §10 + AMEND-22.2-1..-3)

The phase-22.2 behavior-contract delta (vs 22.1 baseline; high-level semantic changes; verbatim Markdown patch lives at §14):

1. **Full Envoy↔Lua bridge surface delta active** — 8 NEW bridge surface families exposed (body / trailers / metadata / connection-SSL / httpCall / crypto / fileBytes+timestamp / streamInfo-full). Every upstream-parity bridge method that 22.1 deferred via Lua-runtime-error becomes a real method.

2. **Dynamic-metadata cross-phase deferral lifted** — NEW `internal/dynamicmetadata/` framework primitive makes cross-filter dynamic-metadata read+write available to all filters going forward. Phases 16/17/18/19/20's "operator-visibility deferred to future" BEHAVIOR_CONTRACT.md notes carry forward AS-IS until their respective next-touchpoint phases (lift-phases convert "deferred" to "lifted via `internal/dynamicmetadata`"). Recorded at BEHAVIOR_CONTRACT.md `### envoy.filters.http.lua` 22.2 sub-section + cross-phase reference paragraph.

3. **5 NEW envoy-go-strict counters** (per §7.3 + AMEND-22.2-3 D6 closure) — `httpcall_total` + `httpcall_failures` + `httpcall_timeouts` + `body_buffered_bytes_total` + `coroutine_yields_total`. Each documented as an envoy-go-strict departure record at BEHAVIOR_CONTRACT.md §13.6 departures section. `httpcall_failures` + `httpcall_timeouts` documented as SYNC-ONLY (async fire-and-forget failures invisible per upstream parity).

4. **`:httpCall` async fire-and-forget semantics** (per AMEND-22.2-3 D6 closure) — script gets 0 return values; no yield; response/error discarded; matches upstream `lua_filter.cc:400-416` exactly.

5. **`:metadata()` callable empty userdata at v1.32.4 binding-gap** (per §11.6 D1 closure) — bridge surface callable but returns empty userdata (`:get(k)` → nil; `pairs()` → 0 iters) until v1.37.x binding bump activates the source-of-data. Operators get a non-crashing callable interface regardless of binding state.

6. **Body-buffer defensive-copy discipline** (per §11.3 D3 RECOMMENDED) — `:body()` returns a Lua string backed by a defensive Go string copy of the accumulated body bytes. Lua owns the resulting string across coroutine yield/resume + HCM dispatch goroutine lifetimes. Stat-counter `body_buffered_bytes_total` tracks the cumulative copy volume. Documented at BEHAVIOR_CONTRACT.md `### Phase 22.2 lua body-buffer notes` paragraph.

7. **Crypto + fileBytes records — PENDING §13-R7+R8 PLAN-time scrape** (per AMEND-22.2-2). If PLAN's targeted upstream re-scrape confirms `:sha256`/`:sha512`/`:base64Decode`/`:importPublicKey`/`:verifySignature`/`:fileBytes` are envoy-go-strict extensions (NOT exposed in upstream v1.37.2 at any scope), 22.2 IMPL adds N NEW BEHAVIOR_CONTRACT.md departure records (max 6, min 0); total bundle scales 5 → 5-11 records. If PLAN re-scrape confirms upstream-equivalence (different exposure scope), bundle stays at 5.

8. **`:filterState()` envoy-go-strict divergences from upstream** (per AMEND-22.2-4 + §11.8 D4 closure) — 2 NEW envoy-go-strict departure records: `:filterState():set(name, value)` mutation surface exposed (upstream is strictly read-only; envoy-go diverges because it has no Go-side mutation analog at 22.2) + typed Lua-value marshaling at `:get(name)` (upstream always returns `serializeAsString()` Lua strings; envoy-go returns native typed Lua values per `LValue` conversion). Recorded at BEHAVIOR_CONTRACT.md §13.6 departures section.

8. **Cluster-based `:httpCall` dispatch** (per §11.4 + §3.3 in-place AMEND on ADR-0177) — operator scripts dispatch outbound HTTP calls via cluster name resolution + LB + cluster TLS config. No URL-based dispatch surface exposed to scripts (envoy-go-strict simplification — operator could theoretically build URL-based outbound calls externally, but the canonical surface is cluster-name based).

---

## 10. Deferred items + forward-pointers (cross-reference parent §10 + 22.2 BRAINSTORM §8)

The full envelope-D delivery completes across 22.1 + 22.2 + 22.3. Items DEFERRED to future phases (cross-phase boundaries) + items FORWARD-POINTED for future SPEC / IMPL resolution. Sourced from 22.2 BRAINSTORM §8 + parent §10:

1. **`Lua.SourceCodes` multi-script map activation** — 22.3 IMPL Tasks per ADR-0106 + parent BRAINSTORM Q2 + parent SPEC §3.2.
2. **`LuaPerRoute` 3-arm oneof PARSE-LIFT + dispatch + 9th canonical ADR** — 22.3 IMPL Tasks; ADR-0125 §(xiv) AMENDMENT body at 22.3 IMPL final Task.
3. **Per-route 3-tier dispatch (listener-default → SourceCodes-named-script → per-route DataSource override)** — 22.3 SPEC settles dispatch semantics.
4. **`:metadata()` per-route source activation** — future v1.32.4 → v1.37.x binding bump per parent AMEND-12 + §11.6 D1.
5. **Cluster-specifier Lua + access-logger Lua + string-matcher Lua** — future cross-family phases; consumers #2/3/4 for `internal/lua/`; each future phase BRAINSTORM revisits the API shape per ADR-0188's EXPLICIT API-REVISION ALLOWANCE clause (NOT consumed by 22.2's consumer-#1-scope-expansion per Q10 strict scope).
6. **`internal/filterstate/` framework primitive extraction** — future; `:filterState()` lands IN-PACKAGE at 22.2 per Q8 + Q9; future phase that adds a second filter-state consumer extracts the primitive.
7. **HMAC + SHA-1 + base64-URL crypto extensions** — future (possibly never); BRAINSTORM Q6 chose 6-method upstream-equivalent envelope over 10-method envoy-go-strict extension.
8. **`:connection()` non-SSL methods (remoteAddress / remoteIp / remotePort)** — future; BRAINSTORM Q4 scoped `:connection()` to SSL accessor per upstream Envoy v1.37.2; per-connection address surface lives on `:streamInfo()`.
9. **Cross-phase dynamic-metadata deferral lifts** — future phases that consume dynamic-metadata reuse `internal/dynamicmetadata/` from 22.2; each prior-phase BEHAVIOR_CONTRACT.md "deferred" note converts to "lifted via `internal/dynamicmetadata`" at the lift-phase's next-touchpoint.
10. **gopher-lua-vs-LuaJIT body / trailers / coroutine observable divergences** — extended at §11.2 (this SPEC); divergences likely surface at IMPL.
11. **Crypto + fileBytes upstream-exposure verification** — RATIFIED-PENDING-PLAN per §13-R7 + §13-R8 + AMEND-22.2-2.
12. **`*LState`-pool design (ADR-0193 escape-valve)** — 22.2 IMPL benchmark gates whether the escape-valve fires; carry-forward from 22.1 ADR-0190-escape-valve-now-ADR-0193 per §1.2.

---

## 11. SPEC-time empirical-pin block — 9 pins resolved IN-SESSION (per ADR-0004)

This block contains the verbatim parallel-subagent-fan-out scrape evidence executed during this 22.2 SPEC drafting session, per ADR-0004's hard-gate discipline. Mirrors phase-22.1 SPEC §11 + parent SPEC §11 structure. **Probe date: 2026-05-19.**

**Reference source corpus** (multi-axis verification per the phase-15..22.1 discipline):

1. **`github.com/yuin/gopher-lua` master** via WebFetch — `state.go`, `coroutine.go`, `coroutinelib.go`, `value.go`, `baselib.go`, `stringlib.go`, `vm.go`.
2. **Upstream Envoy v1.37.2 source** via WebFetch against `github.com/envoyproxy/envoy@v1.37.2/source/extensions/filters/http/lua/{lua_filter.cc, lua_filter.h, wrappers.cc, wrappers.h}` + `source/extensions/filters/common/lua/wrappers.cc` + `test/extensions/filters/http/lua/lua_filter_test.cc` + `api/envoy/extensions/filters/http/lua/v3/lua.proto`.
3. **Local envoy-go codebase** at `/home/esa/git/envoy-go` — `internal/httpclient/`, `internal/cluster/`, `internal/filter/hcm/`, `internal/filter/http/{callbacks.go, chain.go, extauthz/, extproc/, compressor/, lua/}`, `internal/lua/`, `internal/tls/`, `docs/envoy-go/DECISIONS.md` (ADR-0128 + ADR-0177 + ADR-0144).

### Summary disposition table (9 pins → 4 AMENDs + 5 D-closures + 2 carry-forwards)

| Pin | Topic | Disposition | AMEND/D-closure cross-ref |
|---|---|---|---|
| §11.1 | gopher-lua coroutine yield/resume + per-stream `*LState` interaction | CLOSES D2 → native LState.NewThread/Yield/Resume | D2 CLOSED |
| §11.2 | gopher-lua-vs-LuaJIT divergences across 22.2 bridge methods | EXTENDS AMEND-9; SURFACES AMEND-22.2-1 + AMEND-22.2-2 | AMEND-22.2-1 + AMEND-22.2-2 |
| §11.3 | ADR-0128 decode-side buffer primitive API for body-bridge | RECOMMENDS defensive copy + `BodyBuffer` interface | D3 RECOMMENDED + carry forward |
| §11.4 | phase-20 `internal/httpclient/` API for cluster-based dispatch | LANDS `ClusterDispatch` AMENDMENT signature | AMEND-on-ADR-0177 ratified |
| §11.5 | phase-03 TLS `*tls.ConnectionState` for connection-SSL bridge | EXTENDS ADR-0144 with new chain field + symmetric H1/H2 plumbing | D5 carry-forward (cross-side cert-topology) |
| §11.6 | upstream `:metadata()` semantics at v1.32.4 binding-gap | CLOSES D1 → callable empty userdata, NEVER nil | D1 CLOSED |
| §11.7 | upstream `:httpCall()` async-flag wire semantics | CLOSES D6 → PURE FIRE-AND-FORGET; SURFACES AMEND-22.2-3 | D6 CLOSED + AMEND-22.2-3 |
| §11.8 | upstream `:filterState()` shape + gopher-lua LUserData support | CLOSES D4 → string-keyed `map[string]any` with envoy-go-strict `:set` + typed-marshaling divergences; SURFACES AMEND-22.2-4 | D4 CLOSED + AMEND-22.2-4 |
| §11.9 | Project-wide fuzzer count verification (D7) | CLOSES D7 → 28 fuzzers post-22.1; 22.2 → 29-30 | D7 CLOSED |

### §11.1 gopher-lua coroutine yield/resume — D2 CLOSURE

Per parallel-subagent §9.1 report. Methodology: WebFetch scrape against `github.com/yuin/gopher-lua` master (`state.go`, `vm.go`, `coroutinelib.go`).

**§11.1.1 gopher-lua coroutine API surface:**
- `(*LState).NewThread() (*LState, context.CancelFunc)` (`state.go:1614`) — mints a child `*LState` sharing globals with the parent; the child IS the coroutine state.
- `(*LState).Resume(th *LState, fn *LFunction, args ...LValue) (ResumeState, error, []LValue)` (`state.go:2157`) — driven by parent LState; pushes args onto `th`, calls `threadRun(th)`. Returns `ResumeOK` / `ResumeYield` / `ResumeError`.
- `(*LState).Yield(values ...LValue) int` (`state.go:2217`) — pushes values; returns sentinel `-1`.
- `(*LState).Status(th *LState) string` (`state.go:2145`) — returns `"suspended"|"running"|"normal"|"dead"`.
- Lua-side stdlib: `coroutine.{create,resume,yield,running,status,wrap}` registered via `coFuncs` map (`coroutinelib.go:10-16`).

**§11.1.2 Yield mechanism for `:body()` — gopher-lua native:**
The critical mechanic is `vm.go:200-210` `callGFunction`:
```go
gfnret := frame.Fn.GFunction(L)
if gfnret < 0 {
    switchToParentThread(L, L.GetTop(), false, false)
    return true
}
```
A Go-side LGFunction yields by returning a negative value — exactly what `LState.Yield()` returns. So the bridge implements `:body()` as: stash the `*LState` in a per-stream pending-map; return `L.Yield(lua.LNil)` (returns -1 → switchToParentThread). The matching resume happens from the decode-data Envoy callback: `parent.Resume(th, nil, lua.LString(bodyBytes))`.

**§11.1.3 Per-stream LState lifecycle implication vs ADR-0188:** ADR-0188 says one `*LState` per stream (the parent). To use coroutines: parent LState = 1 per stream (compiled-bytecode owner; lives for the filter lifetime); child LState = 1 per phase invocation (cheap; shares `G`+`Env`). Both released on stream destroy; child's `context.CancelFunc` from `NewThread()` MUST be invoked to cancel the child's ctx-derived loop. ADR-0191's §Decision body documents the parent/child lifecycle separation.

**§11.1.4 Error propagation:** `threadRun` (`vm.go:272-293`) installs `defer recover()`. On panic in a yielded-then-resumed coroutine: parent's `Resume` returns `ResumeError` with `newApiError(ApiErrorRun, ret[0])`. The bridge MUST always pair every `Resume` with parent-side error handling.

**Option B (Go-side channel wrapper) REJECTED** — gopher-lua's VM is single-threaded over each `*LState`; blocking a Go bridge LGFunction on a channel would block the OS goroutine driving `threadRun`. Wasted complexity when `Yield(-1)`/`Resume` is built-in.

**D2 RESOLUTION:** Use **gopher-lua native** via `LState.NewThread()` + `LState.Yield()` (returning its `-1` sentinel from Go bridge LGFunction) + `LState.Resume()` from the Envoy data callback. Per-stream lifecycle = 1 parent `*LState` (script owner) + 1 child `*LState` per phase invocation (the coroutine). No Go-side channel scheduling wrapper. ADR-0191 §Decision body codifies.

### §11.2 gopher-lua-vs-LuaJIT divergences across 22.2 bridge methods — AMEND-22.2-1 + AMEND-22.2-2

Per parallel-subagent §9.2 report. Methodology: WebFetch scrape against gopher-lua master + upstream Envoy v1.37.2 `lua_filter.cc/.h` + `wrappers.cc`.

**§11.2.1 Body bytes — CONFIRMED-IDENTICAL.** gopher-lua `LString` is `type LString string` (`value.go:95`) backed by Go `string`. Go strings are immutable byte sequence — NUL-safe, length-prefixed, no UTF-8 validation. Identical wire semantics to LuaJIT 5.1 (Lua strings are length-prefixed `TString`). Upstream `BufferWrapper::luaSetBytes` uses `luaL_checklstring(state, index, &input_size)` — binary fidelity preserved. **No BEHAVIOR_CONTRACT departure required.** Multi-MB strings are fine modulo Go's GC.

**§11.2.2 Trailers `pairs(udata)` — CONFIRMED-IDENTICAL via 22.1 `installPairsShim` discipline.** Vanilla Lua 5.1 / gopher-lua's `basePairs` (`baselib.go:252`) calls `L.CheckTable(1)`, raising `"bad argument #1 to 'pairs' (table expected, got userdata)"`. Lua 5.1 does NOT honor `__pairs` metamethod. **22.1 IMPL's `installPairsShim`** at `internal/filter/http/lua/bridge.go:305-345` overrides the global `pairs` with a Lua-5.2-style version that honors `__pairs` on userdata (Go closure capturing the original `__builtin_pairs` for table fallback). 22.2's trailers bridge reuses this discipline UNCHANGED — `:trailers()` returns a userdata with `__pairs` alphabetical-snapshot metamethod (paralleling 22.1's headers metatable per §11.2 D7 closure). **No BEHAVIOR_CONTRACT departure.**

**§11.2.3 Coroutine error propagation — INHERITS AMEND-9(c) (no new record).** gopher-lua `LState.Resume` (`state.go:2157-2215`) wraps the error value as `newApiError(ApiErrorRun, ret[0])`; the formatted `Error()` prepends `"<source>:<line>: "` (matching `pcall` format per 22.1 AMEND-9(c)). LuaJIT's `lua_resume` returns `LUA_ERRRUN` with the chunkname-prefixed format. Per AMEND-9(c) (22.1) divergence pin: gopher-lua `[string "chunk"]:line: msg` vs LuaJIT `chunk:line: msg`. Coroutine yield/resume inherits the same divergence. **Extend AMEND-9(c) wording at 22.2 IMPL BEHAVIOR_CONTRACT.md edit:** "applies equally to errors surfaced via coroutine resume from `:body()`/`:bodyChunks()`/`:httpCall()`." No new departure record needed.

**§11.2.4 Crypto wire output — PARTIAL-IDENTICAL + PARTIAL-REFUTE (AMEND-22.2-2).** Upstream v1.37.2 `StreamHandleWrapper` exposes ONLY `:base64Escape` on the stream handle (`lua_filter.cc:204` → `static_luaBase64Escape`; impl at lines 762-768). `:base64Escape` uses `absl::Base64Escape(input)` — standard base64 with `=` padding (NOT URL-safe; NOT OpenSSL `BIO_f_base64` line-wrapped). Go's `encoding/base64.StdEncoding.EncodeToString` produces byte-identical output. **CONFIRMED-IDENTICAL for `:base64Escape`.**

`:sha256()`, `:sha512()`, `:base64Decode()`, `:importPublicKey()`, `:verifySignature()` are NOT on `StreamHandleWrapper` per `DECLARE_LUA_FUNCTION` grep at `lua_filter.h:226-357`. **PARTIAL-REFUTE BRAINSTORM Q6 framing** of "full upstream parity": the 5 additional crypto methods may exist on separate upstream wrappers (e.g., `PublicKeyWrapper` userdata for `:verifySignature`; or script-global crypto helpers exposed at script init) OR they're envoy-go-strict extensions. **§13-R7 RATIFIED-PENDING-PLAN re-scrape against `PublicKeyWrapper` + `CryptoUtility` + script-global helpers** to confirm classification.

**§11.2.5 `:fileBytes(path)` binary handling — NOT APPLICABLE in upstream v1.37.2 (AMEND-22.2-2).** No `fileBytes` / `luaFile*` method exists in upstream v1.37.2 Lua filter (grep returns empty across `lua_filter.cc`, `wrappers.cc`, `lua.h`). gopher-lua-side: `os.ReadFile` returned bytes round-trip via `LString(string(b))` preserve all bytes including NUL. **§13-R8 RATIFIED-PENDING-PLAN re-scrape** for upstream-exposure-verification. If confirmed absent, `:fileBytes` is envoy-go-strict extension; 22.2 IMPL adds 1 NEW BEHAVIOR_CONTRACT.md departure record.

**RESOLUTION:** 22.2 SPEC commits AMEND-22.2-1 (body / trailers / coroutine divergence catalogue extension to AMEND-9) + AMEND-22.2-2 (crypto + fileBytes PARTIAL-REFUTATION + §13-R7 + §13-R8 PLAN-time scrape RATIFIED-PENDING). The 22.2 BRAINSTORM §2.6 + §2.7 in-package landing STANDS at SPEC commit (envoy-go exposes the methods on request_handle/response_handle); upstream-equivalence vs envoy-go-strict-classification deferred to PLAN.

### §11.3 ADR-0128 decode-side buffer primitive API — D3 RECOMMENDATION + carry forward

Per parallel-subagent §9.3 report. Methodology: local code scrape against `internal/filter/hcm/connection.go` (ADR-0128 IMPL site) + ADR-0128 in `docs/envoy-go/DECISIONS.md:6010-6050` + ext_authz/ext_proc body-consumer patterns + ADR-0131 OverwriteBody primitive.

**§11.3.1 ADR-0128 architecture:** ADR-0128 anchors TWO primitives — (i) synthetic empty-terminal `RunDecodeData(ctx, nil, true)` at `connection.go:505-509` for chunked-body end-stream signaling; (ii) post-body Content-Length reconciliation at `connection.go:516-534`. The HCM-level body buffer `bodyBuf []byte` (`connection.go:483`) is a per-stream local variable in `dispatchRequest()`; allocated empty, accumulated via `append(bodyBuf, buf[:n]...)`, passed downstream to filters via `RunDecodeData(ctx, buf[:n], endStreamOnData)`; restored to `req.Body` after loop via `io.NopCloser(bytes.NewReader(bodyBuf))`. **Lifetime:** entire request scope (header → body → action → upstream). **Ownership:** HCM owns the bytes; consumers see a `[]byte` slice via the `data` arg to `DecodeData`.

**§11.3.2 Prior body-consumer patterns:** ext_authz (`internal/filter/http/extauthz/extauthz.go:1271-1323`) accumulates into per-filter `f.body []byte`; ext_proc (`internal/filter/http/extproc/extproc.go:1050-1061`) accumulates into per-filter `f.decodeBodyBuf []byte` via `bodyStageEntry()` helper. Both patterns: per-stream filter-struct-owned slice, survives async-resume. **22.2 lua bridge follows the same pattern**: `*filter.decodedBodyBytes []byte` + `*filter.decodedBodyChunks [][]byte` accumulating across `DecodeData` calls.

**§11.3.3 D3 GC interaction risk:** if a Lua filter calls `coroutine.yield()` from inside `:body()`, the Lua VM is suspended. When `ContinueDecoding()` resumes (e.g., from async resume goroutine), the dispatch goroutine wakes the VM. The HCM's `bodyBuf` is STILL valid in HCM scope at that moment, but gopher-lua's `LString` is `type LString string` — backed by an interned Go string in gopher-lua, NOT a direct pointer to the HCM's `bodyBuf` byte-slice. **Defensive copy** at endStream creates a new Go string via `lua.LString(string(f.decodedBodyBytes))` — Lua owns the resulting Go string (immutable; safe across coroutine yield/resume + HCM lifetimes).

**§11.3.4 Recommended `BodyBuffer` interface shape (per §3.2):**
```go
type BodyBuffer interface {
    Bytes() []byte    // full accumulated body; read-only slice
    Chunks() [][]byte // per-DecodeData chunks; each inner slice read-only
    EndStream() bool  // true iff terminal endStream=true fired
}
```
Implemented by the lua bridge's `*decodedBody` struct wrapping per-filter accumulation. Consumed by `:body()` + `:bodyChunks()` LGFunction bridge methods. Defensive copy at endStream per §11.3.3.

**D3 DISPOSITION:** RECOMMENDED — defensive copy at endStream; carry forward to PLAN for perf-benchmark validation (anticipated ≤1ms for typical sub-MB bodies; ≤100ms for 16 MiB-cap-saturated bodies — acceptable trade-off for GC safety). PLAN session may revise to zero-copy if benchmark shows defensive-copy overhead unacceptable for large-body operator patterns.

### §11.4 phase-20 `internal/httpclient/` API for cluster-based dispatch — AMEND-on-ADR-0177 ratified

Per parallel-subagent §9.4 report. Methodology: local code scrape against `internal/httpclient/httpclient.go` + `internal/cluster/manager.go` + `internal/cluster/cluster.go` + `internal/filter/http/oauth2/oauth_client.go`.

**§11.4.1 Current `internal/httpclient/` API surface** (`internal/httpclient/httpclient.go`, ~214 LoC):
- `Options{Timeout, RetryPolicy, TLSConfig *tls.Config}` (lines 49-63).
- `RetryPolicy{Attempts, PerAttemptDelay, RetryOnStatus []int}` (lines 78-91).
- `Client{*http.Client + Options}` (lines 98-101).
- `New(opts Options) *Client` (lines 110-133).
- `(*Client).Do(req *http.Request) (*http.Response, error)` (lines 155-202) — URL-based; status-driven retry.

**§11.4.2 Phase-20 oauth2 consumer pattern** (`internal/filter/http/oauth2/oauth_client.go:236-278` `postTokenEndpoint`): URL-based; `http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))` + `httpClient.Do(req)`. Zero-retry default. No cluster integration.

**§11.4.3 Cluster manager API** (`internal/cluster/manager.go:110-114` + `internal/cluster/cluster.go`):
- `(*Manager).Get(name string) (*Cluster, bool)` (manager.go:110).
- `(*Cluster).PickEndpoint() (Endpoint, error)` (cluster.go:158-162) — ROUND_ROBIN LB per phase-02.
- `(*Cluster).Dial(ctx) (net.Conn, Endpoint, error)` (cluster.go:185-207) — TCP dial + optional TLS handshake.
- `(*Cluster).ConnectTimeout() time.Duration`.
- `Endpoint{Host string, Port uint32; .Addr() string}` — `host:port` form.

**§11.4.4 Recommended AMENDMENT signature (per §3.3):**
```go
func (c *Client) ClusterDispatch(ctx context.Context, clusterName string, request *http.Request, clusterMgr *cluster.Manager) (*http.Response, error)
```
- Resolves clusterName via `clusterMgr.Get(name)` → errClusterNotFound on miss.
- Selects endpoint via `Cluster.PickEndpoint()`.
- Rewrites `request.URL.Host = ep.Addr()` so stdlib `http.Client.Do` dials the LB-selected endpoint.
- Honors per-cluster TLS via temporary `*http.Client` construction with cluster's `upstreamCfg *tls.Config` + receiver's `Options.Timeout/RetryPolicy`.
- Retry loop applies identically to `Do`.

**§11.4.5 Integration with lua filter:** the lua filter's `*compiledConfig` receives `*cluster.Manager` via NEW `FactoryCtx.ClusterManager` field — paralleling existing `FactoryCtx.HTTPClient` threading at `cmd/envoy-go/main.go`. The `:httpCall` bridge closure consumes both references. Per ADR-0044 in-place AMENDMENT discipline: the AMENDMENT body lands at 22.2 IMPL atomic-landing Task — same Task as ADR-0190 + ADR-0191 + ADR-0192 §Decision bodies. **No new ADR number consumed.**

**RESOLUTION:** AMENDMENT body shape RATIFIED at 22.2 SPEC commit per §3.3. PLAN session may refine signature parameters per implementation discovery.

### §11.5 phase-03 TLS `*tls.ConnectionState` for connection-SSL bridge — D5 carry-forward

Per parallel-subagent §9.5 report. Methodology: local code scrape against `internal/filter/hcm/connection.go` + `internal/filter/hcm/h2dispatch.go` + `internal/filter/http/callbacks.go` + `internal/filter/http/chain.go` + ADR-0144 in DECISIONS.md.

**§11.5.1 Per-stream TLS state plumbing:** captured at HCM dispatch time. H1 path (`connection.go:22-76 extractTLSPrincipals` + `:311 chain.SetTLSPrincipals(downstreamTLSPrincipals(downstream))`); H2 path (`h2dispatch.go` extracts ONCE at connection build time in `runH2`, caches on `h2Dispatcher.tlsPrincipals`, copies to each `chainDispatchAction.tlsPrincipals` at match-time before `chain.SetTLSPrincipals()` in `WriteH2:241`).

**§11.5.2 FilterCallbacks TLS accessors** (`internal/filter/http/callbacks.go:68-158`):
- `DownstreamPrincipal() []string` (line 84) — priority-ordered principal candidates from `PeerCertificates[0]`.
- `DownstreamTLSServerName() string` (line 118) — SNI.
- `DownstreamTLSPeerCertDER() []byte` (line 130) — raw DER bytes.
- `ListenerPrincipal() string` (line 157).

**§11.5.3 RECOMMENDED 22.2 seam — extend ADR-0144 pattern:**
1. Add `tlsConnectionState *tls.ConnectionState` field to `FilterChain` (`internal/filter/http/chain.go`).
2. Add setter `SetTLSConnectionState(state *tls.ConnectionState)` (wire-safe: set once BEFORE RunDecodeHeaders per ADR-0071).
3. Add `DecoderFilterCallbacks.DownstreamTLSConnectionState() *tls.ConnectionState` accessor + symmetric encoder side.
4. H1 + H2 plumbing mirrors existing TLS-principals seeding.
5. Lua bridge wraps raw state into Lua userdata at `:connection():ssl()` invocation; 12 ssl methods consume directly from the state.

**§11.5.4 Wire-format conventions for 12 ssl methods:**
- PEM-encoded certs via `encoding/pem.Encode` + DER bytes → PEM block.
- URL-encoded PEM via `url.QueryEscape` (matches upstream's `urlEncodedPem*` shape).
- ISO-8601 timestamps via `time.Time.Format(time.RFC3339)`.
- Hex digests via `fmt.Sprintf("%x", sha256.Sum256(cert.Raw))` (lowercase hex).
- TLS version strings: map `tls.ConnectionState.Version` uint16 → `"TLSv1.0"`/`"TLSv1.1"`/`"TLSv1.2"`/`"TLSv1.3"`.

**No new framework primitive for SSL wrapping** (the existing phase-03 `*tls.ConnectionState` exposure suffices). The chain-side `tlsConnectionState` extension lives inside ADR-0192 §Decision body per Q13 WEAK HOLD (no separate ADR).

**D5 CARRY-FORWARD:** the SPEC settles the SEAM ARCHITECTURE at §11.5; the FIXTURE cross-side cert-topology decision (scenario (f) full cert-matching vs cert-fingerprint-only vs drop-to-REFERENCE-LESS) carries forward to §12 D5 — RECOMMENDED option (f-B) cert-fingerprint-only per §8.3.

### §11.6 upstream `:metadata()` semantics at v1.32.4 binding-gap — D1 CLOSURE

Per parallel-subagent §9.6 report. Methodology: WebFetch scrape against upstream v1.37.2 `lua_filter.cc` + `wrappers.cc` + `lua_filter_test.cc`.

**§11.6.1 Upstream wire shape:** `StreamHandleWrapper::luaMetadata` (`lua_filter.cc:630-639`) ALWAYS returns a `MetadataMapWrapper` userdata — NEVER `nil`:
```cpp
if (metadata_wrapper_.get() != nullptr) {
    metadata_wrapper_.pushStack();
} else {
    metadata_wrapper_.reset(MetadataMapWrapper::create(state, callbacks_.metadata()), true);
}
return 1;
```
`getMetadata()` (`lua_filter.cc:109-128`) returns `Protobuf::Struct::default_instance()` (an empty `Struct`) when `route() == nullptr` OR when no `filter_metadata` entry matches. The empty Struct is wrapped in a real `MetadataMapWrapper` userdata exposing `get` + `__pairs`.

**§11.6.2 Test evidence:** `TEST_F(LuaHttpFilterTest, GetMetadataFromHandleNoRoute)` (line 2302) + `TEST_F(LuaHttpFilterTest, GetMetadataFromHandleNoLuaMetadata)` (line 2322) both call `request_handle:metadata():get("foo.bar")` and expect the chained call to succeed and return nil for the missing key. The wrapper itself is non-nil.

**§11.6.3 gopher-lua nil-vs-empty-table behavioral implications:** returning `nil` would BREAK upstream-equivalent scripts like `request_handle:metadata():get("foo.bar")` — they'd crash with `attempt to index a nil value`. Returning a callable empty wrapper preserves chained-call idioms. gopher-lua's `*lua.LUserData` supports `__index` (for methods) + custom `__pairs` (via `installPairsShim`). The user-friendly comparison `if metadata then ... end` enters the truthy branch for both `{}` and userdata — operator scripts depending on this idiom work identically.

**§11.6.4 LuaPerRoute.filter_context separation (per parent AMEND-12):** the v1.32.4 binding-gap concerns `LuaPerRoute.filter_context` (v1.37.2 field 4) — a separate Lua API `handle:filterContext()`, NOT `:metadata()`. `:metadata()` is wired to route filter_metadata (present in v1.32.4). The binding-gap doesn't ACTUALLY block `:metadata()` from having a source-of-data at v1.32.4 — but the 22.2 BRAINSTORM Q3 + parent AMEND-12 conservatively framed `:metadata()` as empty-at-binding-gap pending full per-route metadata plumbing.

**D1 RESOLUTION:** at v1.32.4 binding-gap, envoy-go's `request_handle:metadata()` returns a non-nil callable userdata wrapping an empty metadata source. `:get(any_key)` returns `lua.LNil`; `pairs()` yields zero iterations. **NEVER return `lua.LNil` from `:metadata()` itself.** Matches upstream v1.37.2 (`MetadataMapWrapper` always non-nil) + passes the script-shapes asserted in upstream `GetMetadataFromHandleNoRoute` + `GetMetadataFromHandleNoLuaMetadata` tests.

### §11.7 upstream `:httpCall()` async-flag wire semantics — D6 CLOSURE + AMEND-22.2-3

Per parallel-subagent §9.7 report. Methodology: WebFetch scrape against upstream v1.37.2 `lua_filter.cc` + `lua_filter.h` + `lua_filter_test.cc`.

**§11.7.1 Upstream `:httpCall()` signature** (`lua_filter.cc:368-398 luaHttpCall` + `:163-194 makeHttpCall`):
```
handle:httpCall(cluster, headers_table, body, timeout_ms, asynchronous?)
        -- OR --
handle:httpCall(cluster, headers_table, body, options_table)
```
Positional args:
1. `cluster` — string (required).
2. `headers` — table; must include `:path`, `:method`, `:authority` (`lua_filter.cc:180-184`).
3. `body` — optional string; `nil` allowed via `luaL_optlstring`.
4. `timeout_ms` — int ≥ 0, OR options table at this position.
5. `asynchronous` — optional boolean; default = false (sync).

**§11.7.2 Asynchronous=true wire shape — PURE FIRE-AND-FORGET** (`doHttpCall` `lua_filter.cc:400-416`):
```cpp
if (options.is_async_request_) {
    makeHttpCall(state, filter_, options.request_options_, noopCallbacks());
    return 0;
}
http_request_ = makeHttpCall(state, filter_, options.request_options_, *this);
if (http_request_ != nullptr) {
    state_ = State::HttpCall;
    ...
    return lua_yield(state, 0);
}
```
Async path: callbacks wired to `noopCallbacks()` global singleton (response fully discarded); no `state_ = State::HttpCall` transition (no reset/cancel tracking); `return 0` (no yield, no values returned to script); test `HttpCallAsynchronous` (`lua_filter_test.cc:1232-1283` header comment: *"Basic asynchronous, fire-and-forget HTTP request flow"*) confirms `decodeHeaders` returns `Continue` not `StopIteration`.

**§11.7.3 Synchronous (default) wire shape:** `lua_yield(state, 0)`; on resume `onSuccess` pushes 2 values (headers table + body string-or-nil). On transport failure, `onFailure` synthesizes a fake 503 with body `"upstream failure"`; script sees a normal response. **4xx/5xx and transport errors are NOT raised as Lua errors** (test `HttpCallFailure`:1592-1639 confirms `errors=0`; trace logs `:status 503` + `upstream failure`).

**§11.7.4 Upstream stat-counter implications:** `ALL_LUA_FILTER_STATS(COUNTER) COUNTER(errors) COUNTER(executions)` (`lua_filter.h:23`) — that's the entire set. Upstream emits only `lua.errors` + `lua.executions`. No `httpcall_total`/`httpcall_failures`/`httpcall_timeouts`. **envoy-go's 5 new counters are envoy-go-strict parity-plus enhancements** (per §7.1).

**D6 RESOLUTION:** envoy-go's `:httpCall(...asynchronous=true)` is PURE FIRE-AND-FORGET — dispatches via `internal/httpclient/Client.ClusterDispatch` in a fire-and-forget goroutine; script gets 0 return values; no yield; response/error discarded. **Stat-counter wiring:**
- `httpcall_total` increments on every dispatch (sync + async) — at the point `internal/httpclient/Client.ClusterDispatch` returns.
- `httpcall_failures` SYNC-ONLY (sync transport-failure / synthetic-503 path). Async failures invisible per upstream parity.
- `httpcall_timeouts` SYNC-ONLY. Async timeout fires noop callbacks → silently dropped.

Async failures are NOT countable at the filter-stats layer (parity-matched). Documented at BEHAVIOR_CONTRACT.md `### envoy.filters.http.lua` 22.2 sub-section + the SYNC-ONLY caveat on `httpcall_failures` + `httpcall_timeouts` departure records.

### §11.8 upstream `:filterState()` shape — D4 CLOSURE + AMEND-22.2-4

Per parallel-subagent §9.8 report. Methodology: WebFetch scrape against upstream v1.37.2 `source/extensions/filters/http/lua/wrappers.{cc,h}` + `test/extensions/filters/http/lua/wrappers_test.cc` + gopher-lua `value.go`/`state.go` for LUserData support.

**§11.8.1 Upstream wire shape (REFUTES BRAINSTORM Q8 `:set()` implication):** upstream `FilterStateWrapper` (`source/extensions/filters/http/lua/wrappers.h`) is a child wrapper hung off `StreamInfoWrapper` — exposure path is `request_handle:streamInfo():filterState():get(name [, field])` (matches BRAINSTORM Q8 exposure framing exactly). BUT the wrapper exports **exactly ONE method**:
```cpp
static ExportedFunctions exportedFunctions() { return {{"get", static_luaGet}}; }
```
**No `:set()`, `:add()`, `:remove()`.** Upstream's Lua `:filterState()` is **strictly READ-ONLY** — mutation happens in C++ filters at the FilterState object level; Lua scripts only observe. **REFUTES BRAINSTORM Q8 framing** that "`:set()` mutation surface is implied."

**§11.8.2 Upstream return shape (NOT typed userdata — flat scalars/strings):** `FilterStateWrapper::luaGet` (`wrappers.cc ~lines 413-455`) returns:
1. `:get(name)` → `lua_pushlstring()` from the FilterState object's `serializeAsString()` representation, OR `nil` if absent.
2. `:get(name, field)` → for objects implementing `hasFieldSupport()`, dispatches on the field-value variant: `absl::string_view` → `lua_pushlstring`; `int64_t` → `lua_pushnumber`; otherwise `nil`.
3. **No userdata, no proto-typed wrapper, no Lua table is ever pushed for filter-state values.** Typed proto objects collapse to their `serializeAsString()` representation at the top-level `:get(name)` call.

**§11.8.3 Test evidence** (`test/extensions/filters/http/lua/wrappers_test.cc`): all `GetFilterState*` cases assert Lua sees scalars. Decisive: `StringAccessorImpl("test_value")` → Lua string `"test_value"`; `UInt64AccessorImpl(12345)` → Lua string `"12345"` (asserted `correct_string_type`!); `BoolAccessorImpl(true)` → Lua string `"true"`. The `correct_string_type` assertion on a `UInt64Accessor` is the decisive evidence: even numeric FilterState objects collapse to **strings** at the top-level call. Numbers only reappear via typed field-access `:get(name, field)` on hasFieldSupport objects.

**§11.8.4 gopher-lua LUserData support:** gopher-lua fully supports the OOP-userdata pattern: `type LUserData struct { Value interface{}; Env *LTable; Metatable LValue }` (`value.go`); `LState.NewUserData()` + `LState.SetMetatable(ud, mt)` + `LState.NewFunction(LGFunction)` + `LState.SetField(mt, "__index", ...)` (`state.go`). Option A (typed userdata) is technically feasible — not a capability gap.

**D4 RESOLUTION + AMEND-22.2-4 (envoy-go-strict departure from upstream `:filterState()` shape):** envoy-go adopts **OPTION B with read+write surface — string-keyed `map[string]any` per-stream + `:get(name)` + `:set(name, value)` + Lua `LValue` marshaling (string→LString, float64/int64→LNumber, bool→LBool, `map[string]any`→LTable recursive)**. THREE intentional divergences from upstream:

1. **`:set(name, value)` exposed at Lua surface (envoy-go-strict).** Upstream is read-only because C++ filters mutate FilterState objects directly via the `StreamInfo::filterState()` accessor; envoy-go has no Go-side mutation path at 22.2 (no in-stream filter has a typed FilterState object hierarchy). Exposing `:set()` at the Lua surface is the most natural mutation seam at 22.2; matches BRAINSTORM Q8's expectation; warrants ONE envoy-go-strict departure record at BEHAVIOR_CONTRACT.md.
2. **Typed Lua-value marshaling at `:get()` (envoy-go-strict).** Upstream `:get(name)` always returns a Lua string (via `serializeAsString()` on the FilterState object). envoy-go's `:get(name)` returns native Lua values (string / number / bool / table) per gopher-lua `LValue` conversion — more convenient for operator scripts but observably different. Warrants ONE envoy-go-strict departure record.
3. **In-package `map[string]any` storage (envoy-go-strict but matches Q9 EXTRACT-NOW-only-when-trigger-fires).** Upstream uses a C++ FilterState object hierarchy with typed access; envoy-go's per-stream `map[string]any` is a simplified envelope. Per BRAINSTORM Q9: NO `internal/filterstate/` framework primitive at 22.2; second consumer triggers extraction at a future phase. NOT a separate departure record (this is a structural decision, not a wire-observable departure — the wire surface IS the Lua bridge methods).

**Implementation seam:** the lua filter's per-stream `*filter` struct gains a `filterState map[string]any` field; bridge methods `:get(name)` returns the value via `LValue` conversion; `:set(name, value)` writes via inverse conversion. PLAN session decides whether the `map[string]any` lives on `*filter` directly OR on a NEW per-stream-context field added to `FilterChain` (per BRAINSTORM §2.8 future-cross-filter-extraction-trigger). For 22.2 IMPL, per-`*filter` placement matches Q9 IN-PACKAGE discipline; future cross-filter consumer extracts to `FilterChain` then to `internal/filterstate/`.

**BEHAVIOR_CONTRACT.md impact:** AMEND-22.2-4 adds 2 NEW envoy-go-strict departure records to the 22.2 IMPL bundle (`:filterState():set()` exposed + `:filterState():get()` typed-marshaling) — bringing the §14 baseline bundle from 8 records to **10 records** at 22.2 IMPL atomic landing (modulo §13-R7+R8 PLAN-time crypto+fileBytes-classification additions; total bundle scale 5 → 10-16 records).

### §11.9 Project-wide fuzzer count verification — D7 CLOSURE

Per inline shell command at SPEC drafting:
```bash
$ find /home/esa/git/envoy-go -name 'fuzz_test.go' -not -path './.worktrees/*' -not -path './.claude/*' \
    | xargs grep -h '^func Fuzz' | sort -u | wc -l
28
```

Project-wide unique fuzzer count post-22.1 IMPL = **28** (in sorted alphabetical order, `FuzzLuaConfigParse` is #22; `FuzzTLSContextParse` is #28). 22.2 anticipates +1-2 NEW fuzzers (`FuzzLuaBodyBridge` + `FuzzLuaHTTPCallConfig` per BRAINSTORM §3.7). **D7 RESOLUTION:** 22.2 phase-done fuzzer count = **29 or 30** depending on PLAN's per-fuzzer task split. §13-R10 RATIFIED-PENDING-IMPL pins the exact number.

---

## 12. SPEC-time D-questions for PLAN-time / IMPL-time resolution

Per phase-22.1 SPEC §12 + phase-21+20+19+18 D-question precedent. SPEC-time D-questions surface unresolved decisions that the 22.2 SPEC author anchors for PLAN/IMPL-time resolution. This SPEC closes 5 of 7 D-questions at §11 (D1 + D2 + D4 + D6 + D7) and carries 2 forward (D3 + D5). Plus the 22.2-specific D-question surfacing from AMEND-22.2-2 (§13-R7 + §13-R8 PLAN-time crypto+fileBytes-exposure scrape).

### D3 (per §11.3): body-buffer zero-copy lifetime — final perf-validation at PLAN

**Question:** the §11.3 D3 RECOMMENDED disposition is defensive copy at endStream (`lua.LString(string(f.decodedBodyBytes))`). Validate perf-overhead at the body-bridge surface:
- (a) Defensive copy is acceptable: ≤1ms for typical sub-MB bodies; ≤100ms for 16 MiB-cap-saturated. SPEC RECOMMENDED.
- (b) Defensive copy is unacceptable for large-body operator patterns: revise to zero-copy via `*lua.LUserData` wrapping (gopher-lua's userdata can hold a pointer to the underlying Go slice without copy); requires lifecycle discipline (`SetMetatable + finalizer` to detect post-OnDestroy use).
- (c) Defensive copy + bounded streaming: read body in fixed-size chunks via `:bodyChunks()`; defensive-copy each chunk individually; cap aggregate memory.

**Resolution at:** 22.2 PLAN session (anticipated micro-benchmark task at body-bridge surface + the §13-R6 *LState-pool gate evaluation).

**Anticipated answer:** option (a) — defensive copy acceptable. RATIFIED-PENDING-PLAN.

### D5 (per §11.5 + §8.3): connection-SSL cross-side fixture-cert-topology

**Question:** fixture-0027 scenario (f) cross-side byte-exactness for `:connection():ssl()` requires matching TLS certs on both reference Envoy + envoy-go sides. Three options per §8.3:
- (f-A) full cert-matching cross-side — operationally complex; ~150-300 LoC of fixture-cert plumbing.
- (f-B) cert-fingerprint-only cross-side — SPEC RECOMMENDED at §8.3.
- (f-C) drop scenario (f) to REFERENCE-LESS subject-only — loses envelope-D verification for SSL methods.

**Resolution at:** 22.2 PLAN session.

**Anticipated answer:** option (f-B) cert-fingerprint-only. RATIFIED-PENDING-PLAN.

### D8 (per AMEND-22.2-2 + §13-R7 + §13-R8): crypto + fileBytes upstream-exposure-verification

**Question:** Pin 2's §11.2.4 + §11.2.5 scrape found `:sha256`/`:sha512`/`:base64Decode`/`:importPublicKey`/`:verifySignature`/`:fileBytes` are NOT on upstream Envoy v1.37.2 `StreamHandleWrapper`. Targeted re-scrape required: where are these methods exposed in upstream (separate wrappers? script-global helpers?) — OR are they envoy-go-strict extensions?

**Resolution at:** 22.2 PLAN session via targeted upstream re-scrape against `PublicKeyWrapper` + `CryptoUtility` + script-global helpers + grep-for-method-names in upstream Envoy source.

**Anticipated answer:** mixed — `:base64Decode` likely upstream-parity on separate wrapper; `:importPublicKey` + `:verifySignature` likely upstream-parity via `PublicKeyWrapper` userdata return; `:sha256`/`:sha512` likely upstream-parity at script-global; `:fileBytes` likely envoy-go-strict extension. Final classification + the BEHAVIOR_CONTRACT.md departure-record bundle scale (5 → 5-11 records) settled at PLAN.

### D1 + D2 + D4 + D6 + D7 — CLOSED IN-SESSION at this SPEC commit

Per §11. No further IMPL-time action required for these.

---

## 13. RATIFIED-PENDING items (cross-reference parent §13 + 22.1 SPEC §13 + sub-phase-specific)

Items the SPEC anchors as RATIFIED at SPEC commit but pending PLAN/IMPL-time confirmation against the actual envoy-go codebase state. The 22.2 SPEC anchors NEW R6 + R7 + R8 + R9 + R10 + R11 items in addition to inheriting the parent §13 + 22.1 §13 items.

### Wire-shape byte-confirmations + co-consumer validation

- **R5 (parent SPEC §13): ADR-0177 `internal/httpclient/` first co-consumer validation** — the 22.2 IMPL `:httpCall()` task lands the first co-consumer of phase-20's `internal/httpclient/` primitive (RATIFIES the phase-20 framework-primitive extraction discipline per ADR-0177 §Consequences forward-pointer). 22.2 closes R5 at IMPL.

### Sandbox + perf

- **R6: `*LState`-pool benchmark gate (escape-valve to ADR-0193)** — 22.1 IMPL R6 STANDS WEAK-default at ~70µs/stream (per 22.1 REVIEW §5; `ns/op = 69865`). 22.2 IMPL benchmark task measures per-stream `*LState` construction cost at the FULL bridge surface (body + coroutine + filter-state in-package). If `> 1ms`: ADR-0193 escape-valve consumed for `*LState` pool design. If `< 1ms`: WEAK-default carries forward; ADR-0193 stays unconsumed.

### Upstream-exposure verification (per AMEND-22.2-2)

- **R7: crypto methods `:sha256`/`:sha512`/`:base64Decode`/`:importPublicKey`/`:verifySignature` upstream-exposure scrape** — 22.2 PLAN session does targeted upstream re-scrape per D8. Outcome ratifies the BEHAVIOR_CONTRACT.md departure-record bundle scale (5 → 5-10 records) at 22.2 IMPL.
- **R8: `:fileBytes(path)` upstream-exposure scrape** — same PLAN-time scrape; outcome ratifies whether the 6th departure record lands (5 → 6 records if envoy-go-strict; 5 → 5 if upstream-parity).

### Body-bridge buffer seam

- **R9: body-buffer-seam-with-ADR-0128 separation (escape-valve to ADR-0193)** — body-bridge buffer seam may surface implementation-time complexity warranting its own ADR (split from ADR-0192 in-package). Lives inside ADR-0192 §Decision body unless 22.2 IMPL surfaces enough complexity to warrant ADR-0193 escape-valve.

### Fuzzer count + driver helpers

- **R10: 29th + 30th fuzzer count verification** — 22.2 IMPL Tasks for `FuzzLuaBodyBridge` + `FuzzLuaHTTPCallConfig` (anticipated; final count at PLAN). Project-wide grep at IMPL pins exact number; 22.2 SPEC §11.9 PRE-CONFIRMS baseline at 28.
- **R11: REFERENCE-LESS subject-only driver helper** — fixture-0027 non-deterministic scenarios (j-k-l-m) need a NEW driver helper (e.g., `RunSubjectOnlyHTTPLua` analogous to 22.1's `BootRejectFixture`) OR can reuse an existing pattern. 22.2 PLAN session decides; 22.2 IMPL Tasks 13-15-equivalent lands the harness extension.

### Wording-discipline

- **W (parent §13-W) wording-pinning at envoy-go boot-reject (CLOSED at 22.1 IMPL)** — UNCHANGED at 22.2.
- **W2 (NEW at 22.2 SPEC): byte-stable runtime-rejection wording for arms 20-22** — `:httpCall("", ...)` + `:body() size cap` + `:importPublicKey(invalid_pem)` runtime-error strings pinned at 22.2 IMPL fuzzer task (per ADR-0080 byte-stable wording + 22.1 Task 11 +2-arm precedent).

---

## 14. BEHAVIOR_CONTRACT.md edit bundle anticipation (cross-reference parent §14 + 22.1 SPEC §14)

Per ADR-0052 atomic landing + 22.1 SPEC §14 7-edit-bundle precedent. The 22.2 IMPL final-Task **10-12 edit bundle** (10 baseline including 2 NEW filter-state records from AMEND-22.2-4 + 0-2 conditional per §13-R7/R8 PLAN outcomes for crypto+fileBytes):

1. **EXTEND `### envoy.filters.http.lua` subsection** with 22.2 full bridge surface delta (body / trailers / metadata / connection-SSL / httpCall / crypto / fileBytes + timestamp / streamInfo-full / filter-state). Cross-reference 22.1 sub-section + carry forward-pointer to 22.3. ~150-200 lines.
2. **Stat-table 102 → 107 extension** under `## Stat surface` — 5 new rows (httpcall_total + httpcall_failures + httpcall_timeouts + body_buffered_bytes_total + coroutine_yields_total). Extension summary paragraph (`## Phase 22.2 extension — 102 → 107 internal names`).
3. **envoy-go-strict departure record #4: `httpcall_total` counter** (envoy-go-strict; operator outbound-call observability).
4. **envoy-go-strict departure record #5: `httpcall_failures` SYNC-ONLY counter** (envoy-go-strict; sync-only per §11.7 D6 upstream parity).
5. **envoy-go-strict departure record #6: `httpcall_timeouts` SYNC-ONLY counter** (envoy-go-strict; sync-only).
6. **envoy-go-strict departure record #7: `body_buffered_bytes_total` counter** (envoy-go-strict; body-buffer capacity-planning visibility).
7. **envoy-go-strict departure record #8: `coroutine_yields_total` counter** (envoy-go-strict; coroutine perf-debugging visibility).
8. **envoy-go-strict departure record #9: `:filterState():set(name, value)` mutation surface exposed** (per AMEND-22.2-4 + §11.8 D4 closure). Upstream is strictly read-only; envoy-go exposes `:set` because it has no Go-side mutation analog at 22.2.
9. **envoy-go-strict departure record #10: `:filterState():get(name)` typed Lua-value marshaling** (per AMEND-22.2-4 + §11.8 D4 closure). Upstream always returns `serializeAsString()` Lua strings; envoy-go returns native typed Lua values per `LValue` conversion.
10. **NEW `### Phase 22.2 forward-pointer notes` subsection**. Documents 22.3-anticipated additions (LuaPerRoute parsing + SourceCodes activation + ADR-0125 §(xiv) AMENDMENT body + 9th canonical per-route shape ADR). Cross-reference cross-phase dynamic-metadata deferral-lift state. ~50-80 lines.
11. **CONDITIONAL envoy-go-strict departure record #11..16** (0-6 records pending §13-R7+R8 PLAN-time crypto+fileBytes-classification outcome) — adds N records for each crypto method confirmed envoy-go-strict + 1 record for `:fileBytes` if envoy-go-strict.
12. **D8 disposition paragraph** documenting the crypto + fileBytes classification outcome from §13-R7+R8.

22.3 IMPL final-Task bundle anticipated (settled at 22.3 BRAINSTORM/SPEC).

---

## 15. 22.2 IMPL acceptance checklist (~25 items extending parent §16 + 22.1 SPEC §15.2)

The 22.2 IMPL Task that lands the full bridge surface + tests + fixture + ADR landings + STATE.md re-advance MUST satisfy ALL of:

**Items 1-18 from parent SPEC §16 (verbatim — see parent SPEC §16) extended to 22.2 surface:**

1. NEW `internal/dynamicmetadata/` package created per §3.1 + ADR-0190 §Decision body.
2. EXTEND `internal/lua/` per §3.2 (NewThread + Resume + YieldFromBridge + BodyBuffer interface) + ADR-0191 §Decision body.
3. EXTEND `internal/filter/http/lua/` per §3.5 (7 NEW files + 4 EXTENDED files) + ADR-0192 §Decision body.
4. IN-PLACE AMEND ADR-0177 with `ClusterDispatch` method per §3.3 + §11.4.
5. EXTEND `internal/filter/http/chain.go` + `internal/filter/http/callbacks.go` with `tlsConnectionState *tls.ConnectionState` field + setter + accessors (lives inside ADR-0192 §Decision body per §1.2 + §11.5).
6. EXTEND `internal/filter/hcm/connection.go` (H1) + `internal/filter/hcm/h2dispatch.go` (H2) with TLS-connection-state seeding symmetric to existing TLS-principals plumbing.
7. EXTEND `internal/filter/http/lua/compiled_config.go` with 3 NEW runtime-rejection arms (20-22 per §6) per 22.1 Task 11 cap pattern precedent.
8. EXTEND `internal/filter/http/lua/stats.go` with 5 NEW envoy-go-strict counters per §7.1.
9. ADR-0190 §Decision + §Consequences body landed in DECISIONS.md.
10. ADR-0191 §Decision + §Consequences body landed in DECISIONS.md.
11. ADR-0192 §Decision + §Consequences body landed in DECISIONS.md.
12. ADR-0177 in-place AMENDMENT body added to ADR-0177 §Decision body in DECISIONS.md (no new ADR number).
13. CONDITIONAL ADR-0193 §Context + §Decision + §Consequences body (only if §13-R6 or R9 escape-valve fires).
14. 29th or 30th project-wide fuzzer (`FuzzLuaBodyBridge` + optionally `FuzzLuaHTTPCallConfig`) at standard ADR-0018 baseline; must-never-panic verified.
15. Differential fixture `0027-http-lua-full-bridge` GREEN with ~9-10 deterministic cross-side scenarios + ~3-4 REFERENCE-LESS-subject-only scenarios per §8.2.
16. BEHAVIOR_CONTRACT.md 8-10 edit bundle landed atomically per ADR-0052 + §14.
17. Cross-phase dynamic-metadata deferral-lift expectation documented at BEHAVIOR_CONTRACT.md cross-phase reference paragraph.
18. STATE.md re-advance to `phase 22.2 IMPL done; awaiting 22.3 BRAINSTORM` + ROADMAP row 22.2 flipped `in-progress → done` per ADR-0106 per-cell IMPL-done annotation.

**Items 19-25 — 22.2 SPEC-specific extensions:**

19. **D1 + D2 + D4 + D6 + D7 closures recorded at §11.1, §11.6, §11.7, §11.8, §11.9** — ADR-0190 + ADR-0191 + ADR-0192 §Decision bodies cross-reference each closure paragraph.
20. **D3 + D5 closures at 22.2 PLAN** — PLAN session anchors the option (a) defensive-copy disposition for D3 + option (f-B) cert-fingerprint-only for D5; 22.2 IMPL Tasks ratify against benchmark + fixture-cert plumbing.
21. **D8 closure at 22.2 PLAN** — PLAN session does targeted upstream re-scrape per §13-R7+R8 + AMEND-22.2-2; outcome ratifies BEHAVIOR_CONTRACT.md departure-record bundle scale.
22. **R6 *LState-pool gate disposition at 22.2 IMPL benchmark task** — 22.2 IMPL benchmark measures per-stream construction at full bridge surface; conditional ADR-0193 fires only if `> 1ms` threshold per parent §13-R6 + D-P10 codification.
23. **R9 body-buffer-seam-with-ADR-0128 separation disposition** — 22.2 IMPL body-bridge task evaluates implementation complexity; conditional ADR-0193 fires only if complexity warrants split from ADR-0192.
24. **Per-task PROGRESS.md entry shape per phase-21 + phase-22.1 IMPL precedent** — N entries (matches PLAN-time task count) across all tasks; each entry quotes command outputs per `superpowers:verification-before-completion`.
25. **REVIEW.md authored at 22.2 IMPL phase-done** per `superpowers:requesting-code-review` per phase-21 + phase-22.1 IMPL precedent.

---

## 16. ADR §Context-draft anchors at 22.2 SPEC commit

Per ADR-0044 §Context-draft discipline. At THIS 22.2 SPEC commit, **3 NEW ADR §Context drafts anchor** with full ~300-500-LoC §Context blocks describing the SPEC-time decision context + the IMPL-time bodies that will land. §Decision + §Consequences bodies land at 22.2 IMPL atomic-landing Task per ADR-0044 in-place edit discipline.

### 16.1 ADR-0190 §Context draft

**Title (provisional):** "NEW `internal/dynamicmetadata/` framework primitive — per-stream `*Bucket` accessor for cross-filter dynamic-metadata read+write at first co-consumer (HTTP Lua filter 22.2) per phase-22 BRAINSTORM Q3 cross-phase-deferral-break + Q9 EXTRACT-NOW + 22.2 SPEC §3.1 production signatures + §1.6 cross-phase deferral-lift expectation."

**Lands-in:** 22.2 IMPL atomic-landing Task per PLAN.

**Title cross-reference:** Phase-22 parent SPEC §4 + 22.2 BRAINSTORM §1.6 + §2.3 + §3.1 + 22.2 SPEC §3.1 + §11.6.

### 16.2 ADR-0191 §Context draft

**Title (provisional):** "`internal/lua/` 22.2 API extensions for coroutine yield/resume + body-bridge buffer seam at HTTP filter Lua consumer-#1 scope-expansion per phase-22.2 BRAINSTORM Q1 + Q10 strict scope (NEW ADR not in-place AMEND on ADR-0188) + 22.2 SPEC §3.2 production signatures + §11.1 D2 closure (gopher-lua native LState.NewThread/Yield/Resume)."

**Lands-in:** 22.2 IMPL atomic-landing Task per PLAN.

**Title cross-reference:** Phase-22.1 ADR-0188 (paired predecessor primitive; API-REVISION ALLOWANCE clause STAYS scoped to consumer-#2) + 22.2 BRAINSTORM §2.1 + §3.2 + 22.2 SPEC §3.2 + §11.1.

### 16.3 ADR-0192 §Context draft

**Title (provisional):** "`internal/filter/http/lua/` 22.2 package shape extensions — body + trailers + metadata + connection-SSL + httpCall + crypto + fileBytes + timestamp + streamInfo-full + filter-state in-package bridge methods + 5 envoy-go-strict departure records + cross-phase dynamic-metadata deferral-lift via `internal/dynamicmetadata/` consumer-#1 + fixture-0027 mixed-mode discipline + `FilterChain.tlsConnectionState` field extension (lives inside this ADR per Q13 WEAK HOLD) per 22.2 BRAINSTORM §2.1-§2.13 + 22.2 SPEC §3.5 file roster + §8 fixture-0027 + §11 8-pin empirical-pin closures + AMEND-22.2-1..-3."

**Lands-in:** 22.2 IMPL atomic-landing Task per PLAN.

**Title cross-reference:** Phase-22.1 ADR-0189 (paired predecessor package-shape ADR; 22.2 extends) + ADR-0144 (TLS state plumbing pattern extended) + ADR-0128 (decode-side body-buffer integration via `BodyBuffer` interface seam from ADR-0191) + ADR-0125 §(xiv) (anticipation paragraph UNCHANGED at 22.2; AMENDMENT body lands at 22.3 IMPL) + 22.2 BRAINSTORM §2 + §3 + §4 + §6 + 22.2 SPEC §3 + §5 + §6 + §7 + §8 + §9 + §11.

---

## Appendix A — Cross-references to parent + 22.1 SPECs + BRAINSTORM

This 22.2 SPEC cross-references the following content (inherited verbatim; NOT duplicated here):

- **Parent SPEC §1** (envelope D + 3-way pre-split + 14-fact summary).
- **Parent SPEC §1.1** (12-AMEND catalog) — UNCHANGED at 22.2 SPEC.
- **Parent SPEC §3** (Sub-phase scope summary).
- **Parent SPEC §4** (Framework primitive sketch) — refined at 22.1 SPEC §3 (`internal/lua/` API) + extended at 22.2 SPEC §3 (NEW `internal/dynamicmetadata/` + `internal/lua/` extensions).
- **Parent SPEC §5** (Proto-field roster) — UNCHANGED at 22.2 (per §5).
- **Parent SPEC §6** (PARSE-REJECT roster) — 19-arm 22.1 roster extended with 3 RUNTIME-REJECT arms at 22.2 (per §6).
- **Parent SPEC §7** (Stat surface template) — 22.2 adds 5 counters (per §7).
- **Parent SPEC §8** (Fixture taxonomy) — 22.2 fixture-0027 specified (per §8).
- **Parent SPEC §10** (Deferred items) — extended at 22.2 §10.
- **Parent SPEC §11** (7-pin empirical-pin block) — UNCHANGED; 22.2 SPEC §11 8-pin block ADDS to the cumulative empirical-pin record.
- **Parent SPEC §13** (RATIFIED-PENDING items 6 + W) — R5 + R6 carry forward at 22.2; R7-R11 + W2 NEW at 22.2 (per §13).
- **22.1 SPEC §3.1-§3.5** (production `internal/lua/` API signatures + file split + sandbox roster + per-stream lifecycle + `internal/filter/http/lua/` file split) — INHERITED + EXTENDED at 22.2 SPEC §3.
- **22.1 SPEC §11.1-§11.2** (D5 + D7 closures) — UNCHANGED; 22.2 inherits the 28-fuzzer baseline + `__pairs` alphabetical-snapshot discipline.
- **22.1 SPEC §12** (D1 + D3 from parent §12-D1 + §12-D3) — CLOSED at 22.1 IMPL; UNCHANGED at 22.2.
- **22.1 SPEC §13-R6** (`*LState`-pool benchmark gate) — STANDS WEAK-default at 22.1 IMPL (`ns/op = 69865`); carries forward to 22.2 IMPL as escape-valve to ADR-0193.
- **22.1 SPEC §15.2** (24-item acceptance checklist) — UNCHANGED; 22.2 SPEC §15 25-item checklist EXTENDS the pattern.
- **22.2 BRAINSTORM §1-§12** (14-Q dialogue + framework-survey + per-route disposition + stat surface + fixture strategy + anticipated-ADR roster + deferred items + 7 D-questions carry-forward) — fully incorporated at 22.2 SPEC §1-§16.

---

## Appendix B — Phase 22.2 ADR landings summary

At THIS 22.2 SPEC commit: **3 NEW ADR §Context drafts anchor** (ADR-0190 + ADR-0191 + ADR-0192). DECISIONS.md tail advances from ADR-0189 → ADR-0190..0192 §Context. Next-free ADR advances from `ADR-0190` → `ADR-0193`.

At 22.2 IMPL atomic-landing Task per PLAN:

- **ADR-0190 §Decision + §Consequences body** — NEW `internal/dynamicmetadata/` framework primitive. Per §3.1 + §11 cross-references.
- **ADR-0191 §Decision + §Consequences body** — `internal/lua/` 22.2 API extensions. Per §3.2 + §11.1 D2 closure.
- **ADR-0192 §Decision + §Consequences body** — NEW `internal/filter/http/lua/` 22.2 package shape extensions. Per §3.5 + §6 + §7 + §8 + §11 + AMEND-22.2-1..-3.
- **ADR-0177 IN-PLACE AMENDMENT body** — `ClusterDispatch` method. Per §3.3 + §11.4. No new ADR number consumed.
- **CONDITIONAL ADR-0193 §Context + §Decision + §Consequences body** — only if §13-R6 *LState-pool gate fires OR §13-R9 body-buffer-seam-with-ADR-0128 separation fires. If unconsumed: carries forward to 22.3 BRAINSTORM as the 22.3 IMPL escape-valve slot.

At 22.3 IMPL final Task:

- **ADR-0125 §(xiv) IN-PLACE AMENDMENT body** — NEW 9th canonical per-route shape. AMENDMENT-anticipation paragraph anchored at parent SPEC commit STANDS UNCHANGED at 22.2 SPEC + 22.2 IMPL.

**Next-free ADR after 22.2 SPEC commit:** `ADR-0193`. After 22.2 IMPL: `ADR-0193` (if R6 + R9 both stand) or `ADR-0194` (if one escape-valve fires).
