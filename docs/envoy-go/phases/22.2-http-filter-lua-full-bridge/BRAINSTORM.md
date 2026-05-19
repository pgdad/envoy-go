# Phase 22.2 Brainstorm — `envoy.filters.http.lua` (full bridge surface delta)

**Sub-phase:** `22.2-http-filter-lua-full-bridge` (second sub-phase of parent 22 per parent BRAINSTORM Q2 PRE-SPLIT)
**Parent row:** `22-http-filter-lua` (status `in-progress` per ROADMAP; parent row stays in-progress until 22.3 phase-done per parent SPEC §1 closure pattern)
**Predecessor:** `22.1-http-filter-lua-vm-and-headers-bridge` (status `done` at 2026-05-18; landed VM + DefaultSourceCode + pragmatic-middle bridge; 17 HTTP filters wired; 102 stat names; 28 fuzzers; 28 differential fixture dirs; DECISIONS.md tail at ADR-0189; next-free ADR-0190)
**Successor:** `22.3-http-filter-lua-multi-script-and-per-route` (status `planned`; lands SourceCodes multi-script map + LuaPerRoute 3-arm override + ADR-0125 9th canonical AMENDMENT body)
**Authored at:** this BRAINSTORM commit (squash-merged to master per project memory `feedback_git_worktrees.md` + ADR-0005 §Decision 4)

This sub-phase BRAINSTORM is the per-sub-phase BRAINSTORM convention per ADR-0106 + matches the discipline shape of the parent 22 BRAINSTORM (`../22-http-filter-lua/BRAINSTORM.md` — 12 Qs across 12 §2 numbered sub-sections). Parent BRAINSTORM Q6 settled 22.1's bridge envelope as pragmatic-middle and forward-pointed the full bridge surface delta to 22.2; this BRAINSTORM settles every per-surface decision + the transverse framework / stat / fixture / D-hypothesis decisions for 22.2.

---

## 1. Mission and scope confirmation (22.2 only)

### 1.1 What 22.2 delivers as a self-contained whole

Phase 22.2 lands the FULL Envoy↔Lua bridge surface delta on top of 22.1's pragmatic-middle, taking parent BRAINSTORM Q1 envelope D to its conclusion. By 22.2 phase-done every upstream-parity bridge method is active, plus two intentional envoy-go-strict scope-expansions (dynamic-metadata pulled forward from the cross-phase deferral discipline; filter-state in-package at the lua filter ahead of any cross-filter primitive landing).

The 22.2 surface delta (8 bridge surfaces + transverse framework / stat / fixture decisions):

- **Body bridge** (Q1): `:body()` whole-buffer + `:bodyChunks()` chunked iterator + Lua coroutine yield/resume + zero-copy buffer-sharing with the ADR-0128 decode-side buffer primitive.
- **Trailers bridge** (Q2): `:trailers()` mirrors the 22.1 headers metatable exactly (8 mutation methods + `__pairs`) + lazy-available semantics (returns nil if no trailers received yet).
- **Metadata bridge** (Q3): `:metadata()` callable surface (returns empty at v1.32.4 binding-gap forward-pointer; meaningful per-route source activates at the v1.37.x binding bump per AMEND-12) + `:streamInfo():dynamicMetadata()` + `:streamInfo():dynamicTypedMetadata()` via NEW `internal/dynamicmetadata/` framework primitive. **Breaks the cross-phase dynamic-metadata deferral discipline** (phases 16/17/18/19/20 deferred — see §1.6).
- **Connection bridge** (Q4): `:connection():ssl()` full upstream cert/session surface (~12 methods: subject + SANs local/peer + validFrom + expirationPeer + sessionId + ciphersuiteId + tlsVersion + urlEncodedPemEncodedPeerCertificate + urlEncodedPemEncodedPeerCertificateChain + sha256PeerCertificateDigest); returns nil on plaintext.
- **httpCall bridge** (Q5): `:httpCall(cluster, headers, body, timeout_ms, asynchronous?)` per upstream signature — cluster-based dispatch reusing envoy-go cluster manager + LB + circuit-breaker; sync default + optional async flag; consumer-#1 of `internal/httpclient/` per ADR-0177 (triggers in-place AMEND on ADR-0177 for cluster-based dispatch path).
- **Crypto bridge** (Q6): 6 upstream methods (`:base64Escape()` + `:base64Decode()` + `:sha256()` + `:sha512()` + `:importPublicKey()` + `:verifySignature()`) — thin in-package wrappers over Go stdlib `crypto/*` + `encoding/base64`.
- **Filesystem + clock bridge** (Q7): `:fileBytes(path)` unrestricted FS read with 16 MiB cap (inherits 22.1 Task 11 cap pattern from `DataSource.Filename`) + `:timestamp(unit?)` wall-clock with millisecond default + optional `'milliseconds'`/`'microseconds'`/`'seconds'` arg.
- **streamInfo-full** (Q8): all 7 additional methods including `:filterState()` (filter-state primitive stays IN-PACKAGE per Q9 EXTRACT-NOW-only-when-trigger-fires; `internal/filterstate/` not extracted at 22.2).

Plus transverse decisions:

- **Framework extraction** (Q9): NEW `internal/dynamicmetadata/` primitive (multi-consumer story is clear — jwt_authn read + rbac read + ext_proc read/write all wanted it) + IN-PLACE AMEND `internal/httpclient/` API for cluster dispatch + NEW `internal/lua/` API extensions for coroutine yield/resume + body-bridge buffer seam (per Q10 strict scope = NEW ADR-0191, NOT in-place AMEND of ADR-0188).
- **ADR-0188 API-revision allowance scope** (Q10): strict — the ALLOWANCE remains scoped to consumer-#2 (future cluster-specifier / access-logger / string-matcher Lua); 22.2's consumer-#1-scope-expansion revisions land under NEW ADR-0191 instead of in-place AMENDING ADR-0188. Cleanest separation of lineage; expands ADR roster by 1 vs the AMEND alternative.
- **Stat surface delta** (Q11): pragmatic-middle ~5-6 NEW counters (102 → ~108): `httpcall_total` + `httpcall_failures` + `httpcall_timeouts` + `body_buffered_bytes_total` + `coroutine_yields_total`. All envoy-go-strict; 5-6 BEHAVIOR_CONTRACT.md departure records added at 22.2 IMPL atomic-landing Task.
- **Fixture-0027 strategy** (Q12): single mixed-mode `0027-http-lua-full-bridge` directory with both deterministic scenarios (cross-side `CompareBytes` byte-exact: body / trailers / metadata-empty / crypto / streamInfo-most / fileBytes) and non-deterministic scenarios (REFERENCE-LESS subject-only: httpCall / timestamp / filterState). 28 → 29 fixture directories.
- **D-hypothesis** (Q13): WEAK HOLD — 3 anticipated NEW ADRs (ADR-0190 + ADR-0191 + ADR-0192) land cleanly + in-place AMEND on ADR-0177 (no number consumed) + 0-1 escape-valve consumption at 22.2 IMPL (likely surfaces: R6 *LState-pool gate if body+coroutine pushes per-stream construction over 1ms threshold; OR new ADR for body-buffer-seam-with-ADR-0128 separation; OR new ADR for connection-SSL bridge integration with phase-03).
- **Scope shape** (Q14): stay single-phase 22.2; let PLAN-stage split-gate fire if needed. Avoids speculative split-at-BRAINSTORM with imperfect task estimates.

### 1.2 What 22.2 does NOT deliver (forward to 22.3 / future)

Items DEFERRED to 22.3 (per parent BRAINSTORM Q2 + Q7) or to future phases:

- **`Lua.SourceCodes` multi-script map activation** (22.3) — arm 4 PARSE-REJECT lifts at 22.3.
- **`LuaPerRoute` 3-arm oneof PARSE-LIFT + dispatch** (22.3) — arm 18 PARSE-REJECT lifts at 22.3; NEW 9th canonical per-route shape per ADR-0125 §(xiv) AMENDMENT body landing at 22.3 IMPL final Task; per-route 3-tier dispatch (listener-default → SourceCodes-named-script → per-route DataSource override) settled at 22.3 SPEC.
- **`:metadata()` per-route source activation** (future v1.32.4 → v1.37.x binding bump) — per AMEND-12 + parent SPEC §10 items 16-17. The bridge surface exists at 22.2 (callable; returns empty); the source-of-data flips on at the binding bump phase.
- **Cluster-specifier Lua + access-logger Lua + string-matcher Lua** (future cross-family phases) — consumers #2/3/4 for the `internal/lua/` framework primitive; each future phase BRAINSTORM revisits the API shape per ADR-0188's EXPLICIT API-REVISION ALLOWANCE clause (which remains scoped to consumer-#2 per Q10 strict scope; NOT consumed by 22.2's consumer-#1 scope-expansion).
- **Cross-filter `internal/filterstate/` primitive extraction** (future) — `:filterState()` lands IN-PACKAGE at 22.2 per Q8 + Q9; future phase that adds a second filter-state consumer extracts the primitive per the EXTRACT-NOW-only-when-trigger-fires lesson.
- **HMAC + SHA-1 + base64-URL crypto extensions** (future, possibly never) — Q6 chose full upstream parity (6 methods) over envoy-go-strict extension (10 methods); the extensions defer indefinitely.
- **`:connection()` `:remoteAddress()` + `:remoteIp()` + `:remotePort()`** (future) — Q4 scoped `:connection()` to the SSL accessor only per upstream Envoy v1.37.2; the per-connection address surface lives on `:streamInfo()` already (22.1 + 22.2 ships).

### 1.3 22.2's relationship to parent 22 BRAINSTORM Q1 envelope D + Q6 pragmatic-middle hand-off

Parent BRAINSTORM Q1 settled the ambition: envelope D = full upstream parity by phase-22 phase-done across the 3-way pre-split. Parent BRAINSTORM Q6 settled the within-22.1 cut: pragmatic-middle bridge surface (top-level hooks + headers + log + streamInfo-subset + respond). 22.2 takes Q6's hand-off and lands the FULL DELTA: every method that 22.1 deferred (raising Lua runtime error in 22.1 per the Q6 deferred-method disposition) becomes a real bridge method at 22.2.

Two intentional envoy-go-strict scope-expansions beyond bare upstream parity:

1. **Dynamic-metadata at 22.2** breaks the cross-phase deferral discipline (phases 16/17/18/19/20 deferred independently; each phase booked an "operator-visibility deferred to future" line in BEHAVIOR_CONTRACT.md). This is INTENTIONAL per Q3 — the deferral discipline was about IMPL not principle; 22.2's NEW `internal/dynamicmetadata/` primitive is positioned as the cross-filter-reusable home that absorbs prior phases' deferrals at next-consumer time. Future phase BRAINSTORMs that need dynamic-metadata access (the read side: jwt_authn / rbac; the write side: ext_proc; the read+write: future ext_authz-extension) reuse the primitive rather than each independently deferring again.
2. **Filter-state at 22.2 (in-package)** ships the bridge surface (`:streamInfo():filterState()`) at 22.2 even though the project has no cross-filter state primitive yet. The IN-PACKAGE landing is the EXTRACT-NOW-only-when-trigger-fires posture (per phase-21 lesson) — second consumer of filter-state will trigger the `internal/filterstate/` primitive extraction at that future phase.

### 1.4 ADR-0045 split-by-surface readiness — staying single-phase at BRAINSTORM (Q14)

Per Q14: 22.2 stays as one ROADMAP row at this BRAINSTORM commit. Estimated scope from Q1-Q13 is ~25-35 tasks / ~3000-5000 LoC, which is at-or-above the ADR-0045 split-gate (~25 tasks / ~1500 LoC). The PLAN session does the precise estimation against the gate; if it exceeds, 22.2 splits into 22.2.1 + 22.2.2 at PLAN time per the phase-09 → phase-11 + phase-13 split-at-PLAN precedent (ROADMAP + STATE update; BRAINSTORM not invalidated).

Pre-splitting at BRAINSTORM was rejected on rationale grounds: imperfect task estimates this early in the lifecycle would force a suboptimal split-axis; the parent BRAINSTORM's Q2 PRE-SPLIT had a clearer envelope-D-to-3-way mapping (VM + bridge + multi-script) than 22.2's per-surface decisions, which don't decompose along any obvious axis (per-stream-state vs out-of-band? body vs metadata vs connection? — no obvious clean cut). Better to let PLAN session see the real Task graph and decide.

### 1.5 Phase 22.1 IMPL inheritance state

22.2 inherits the following state from 22.1 IMPL (master tip `c986419` = `phase 22.1 IMPL follow-up: STATE.md SHA-fill (TBD → d30f131 post-squash)`):

- **17 HTTP filters wired** (`envoy.filters.http.lua` is the 15th §9 family-row; 22.2 does NOT add a §9 row — it extends the existing lua filter).
- **102 stat names** (3 new at 22.1: errors + executions + respond_calls; 22.2 anticipates +5-6 → ~108 per Q11).
- **28 fuzzers** (22.1 added `FuzzLuaConfigParse`; 22.2 anticipates +1-2: `FuzzLuaBodyBridge` + `FuzzLuaHTTPCallConfig` — see D7).
- **28 differential fixture directories** (22.1 added `0026-http-lua-headers-bridge`; 22.2 anticipates +1: `0027-http-lua-full-bridge` per Q12).
- **DECISIONS.md tail at ADR-0189** with full §Decision + §Consequences bodies; ADR-0125 §(xiv) AMENDMENT-anticipation paragraph still UNCHANGED (AMENDMENT body lands at 22.3 IMPL final Task per ADR-0044).
- **Next-free ADR-0190** carries forward UNCONSUMED from 22.1 IMPL (D-P10 R6 STANDS WEAK-default at `ns/op = 69865` ~70µs/stream — well under 1ms threshold; ADR-0190 NOT consumed). 22.2 anticipates ADR-0190 → ADR-0192 consumption per Q9 + Q10 (NEW `internal/dynamicmetadata/` + NEW `internal/lua/` extensions + NEW `internal/filter/http/lua/` package shape extensions) + conditional ADR-0193 for escape-valve.
- **`internal/lua/` framework primitive** anchored at ADR-0188 — 22.2 EXTENDS via NEW ADR-0191 per Q10 strict scope (coroutine yield/resume + body-bridge buffer seam); ADR-0188's API-REVISION ALLOWANCE clause STAYS scoped to consumer-#2.
- **`internal/filter/http/lua/` package shape** anchored at ADR-0189 — 22.2 EXTENDS via NEW ADR-0192 (body bridge + trailers bridge + metadata bridge + connection-SSL bridge + httpCall bridge + crypto in-package + fileBytes + timestamp + streamInfo-full + filterState in-package).
- **`internal/httpclient/` framework primitive** anchored at ADR-0177 (phase-20) — 22.2 EXTENDS via in-place AMENDMENT (per ADR-0044 in-place edit discipline; matches phase-17 → phase-18 ADR-0149 → ADR-0150 AMEND precedent) for cluster-based dispatch (the lua filter's httpCall needs cluster manager + LB integration that the phase-20 oauth2 consumer didn't trigger). No new ADR number consumed for the httpclient AMEND.
- **3 envoy-go-strict departure records** at 22.1 (stdlib-sandbox-strict + respond_calls counter + runtime-error log-message wording); 22.2 adds 5-6 more per Q11 (httpcall counters + body_buffered_bytes + coroutine_yields counter).
- **19-arm PARSE-REJECT roster** at `internal/filter/http/lua/` per ADR-0189 (extended from SPEC-time 18 by Task 11 fuzzer); 22.2 anticipates extensions (NEW arms for httpCall cluster-name resolution failure + body-size-cap + crypto-key-format-invalid per SPEC-time scrape).

### 1.6 Cross-phase dynamic-metadata deferral discipline — broken at 22.2 (intentional per Q3)

Phases 16 (rbac) / 17 (jwt_authn) / 18 (ext_authz) / 19 (ext_proc) / 20 (oauth2) each deferred dynamic-metadata access by their respective filters with BEHAVIOR_CONTRACT.md "operator-visibility deferred to future" notes. The deferral was about IMPL — each phase chose not to be the first to land a cross-filter state primitive — not about principle (operators DO want dynamic-metadata visibility from these filters).

22.2 lands `internal/dynamicmetadata/` as the cross-filter-reusable primitive (per Q9). The PRIMITIVE shape: per-stream `*Bucket` accessor + map keyed by `(filter_name string, key string) → google.protobuf.Value`; lookups + writes are O(1); cross-filter visibility is per-stream (no cross-stream cache). Consumed by `internal/filter/http/lua/` bridge as `:streamInfo():dynamicMetadata()` + `:streamInfo():dynamicTypedMetadata(filter_name)` Lua-side accessors.

Future phase BRAINSTORMs that need dynamic-metadata access from their respective filters (rbac read at a future phase; jwt_authn read; ext_authz extension at a future phase; new filter families) consume `internal/dynamicmetadata/` rather than defer. The deferred items at phases 16/17/18/19/20 stay deferred until their next-touchpoint phases (next §9 family-row that touches one of those filters), at which point the BEHAVIOR_CONTRACT.md note converts from "deferred" to "lifted via internal/dynamicmetadata".

ADR-0190 §Consequences body documents this cross-phase deferral-lift expectation. Each prior-phase BEHAVIOR_CONTRACT note remains AS-IS until the lift phase reconnects them.

---

## 2. Design decisions (per topic; each cites BRAINSTORM-style rationale + consequences anchor)

### 2.1 Body bridge surface: full upstream parity *(Q1 → ADR-0191 + ADR-0192)*

**Decision:** Land both `:body()` whole-buffer + `:bodyChunks()` chunked iterator on both `request_handle` and `response_handle`. Use Lua coroutine yield/resume for async semantics (script yields when body not yet buffered; Go-side resumes the coroutine when ADR-0128's decode-side buffer signals end-stream availability). Zero-copy buffer-sharing — the Lua string returned by `:body()` is backed by the ADR-0128 buffer's underlying byte slice (no copy at bridge time; immutable from Lua's perspective per Lua's string semantics). `:bodyChunks()` returns an iterator that walks the chunks list lazily — each `iter()` call returns the next chunk as a Lua string (also zero-copy backed by ADR-0128 chunks).

**Rationale:** Whole-buffer-only + coroutine (option b at Q1) was rejected on operator-pattern-coverage grounds — operators with large-body scripts (e.g., body-rewriting for media files) need streaming-style processing; the chunked iterator is non-trivial to add later. Whole-buffer-only + goroutine block + copy (option c at Q1) was rejected on 3 grounds: (i) departs from upstream Envoy's coroutine semantics — operators porting Lua scripts from upstream would hit subtle behavioral differences; (ii) forces always-buffer at the filter (since Lua dynamic dispatch can't be statically analyzed to predict :body() calls), wasting perf on body-passthrough scripts; (iii) extra alloc cost contradicts envelope D's perf-parity ambition.

The zero-copy buffer-sharing is the load-bearing perf decision — Lua scripts that read the entire request body would otherwise pay 2× memory cost (Go-side buffer + Lua-side copy) on every request, which scales poorly at the gigabyte-body upper end.

**Anticipated ADRs:** ADR-0191 §Decision documents the `internal/lua/` API extensions for coroutine yield/resume (new `VM.Yield(value) any` + `VM.Resume(args ...any) error` + bridge-method `WithCoroutineYield(handler YieldHandler)` option). ADR-0192 §Decision documents the body-bridge methods in-package shape (in-package at `internal/filter/http/lua/body.go`; consumes the new `internal/lua/` API surface; reuses ADR-0128 decode-side buffer primitives). The body-buffer seam (lifecycle guarantees between Lua's string GC + ADR-0128's buffer GC) lands inside ADR-0192 §Decision body (D3 carry-forward to SPEC).

### 2.2 Trailers bridge surface: mirror headers exactly + lazy-available *(Q2 → ADR-0192)*

**Decision:** `request_handle:trailers()` + `response_handle:trailers()` return trailer-map objects with the same 8-method mutation surface as 22.1's headers metatable (get + getAtIndex + getNumValues + add + append + remove + replace + `__pairs` iterator). Lazy-available semantics: returns nil if no trailers have been received yet (HTTP/1.1 chunked-trailers absent; HTTP/2 trailing-HEADERS frame absent); script checks for nil before iterating.

**Rationale:** Mirror-headers + auto-wait-after-body (option b at Q2) was rejected on idiomaticity grounds — Lua's nil-checking idiom is well-established; introducing implicit wait semantics surprises operators porting from upstream. Read-only subset (option c at Q2) was rejected on upstream-parity grounds — operators want to mutate response trailers for tracing-context injection patterns (e.g., `grpc-trailer-trace-id: <span_id>`); cutting mutation surface forces operators to add a separate filter for trailer manipulation.

Reusing 22.1's headers metatable shape is the load-bearing simplicity decision — the trailer bridge has identical semantics modulo the underlying map type; the bridge code is largely a copy of the headers bridge with the underlying map source swapped.

**Anticipated ADRs:** ADR-0192 §Decision documents the trailers bridge methods in-package shape (in-package at `internal/filter/http/lua/trailers.go`; shares the headers metatable factory at `internal/filter/http/lua/bridge.go` from 22.1).

### 2.3 Metadata bridge surface: typed_per_filter_config + dynamic-metadata both at 22.2 *(Q3 → ADR-0190 + ADR-0192)*

**Decision:** Two distinct bridge surfaces both land at 22.2:

- `request_handle:metadata()` + `response_handle:metadata()` return the typed_per_filter_config metadata for the lua filter at the current route. At v1.32.4 binding-gap (`LuaPerRoute.filter_context` is v1.37.2-only per AMEND-12 + parent SPEC §11.1.4), `:metadata()` returns an EMPTY table — the bridge surface is callable but has no source-of-data. Per-route metadata source activates at the future v1.32.4 → v1.37.x binding bump phase. **Importantly: 22.2 does NOT pull LuaPerRoute parsing forward from 22.3** — the bridge returns empty regardless of any per-route `typed_per_filter_config` because the proto field that would carry the metadata doesn't exist in v1.32.4 bindings.
- `request_handle:streamInfo():dynamicMetadata()` + `:dynamicTypedMetadata(filter_name)` return the dynamic-metadata accessor (read + write across filter chain). NEW `internal/dynamicmetadata/` framework primitive lands at 22.2 per Q9 — per-stream `*Bucket` keyed by `(filter_name string, key string) → google.protobuf.Value`. **Breaks the cross-phase dynamic-metadata deferral discipline** (phases 16/17/18/19/20 deferred independently — see §1.6 for the discipline-lift expectation).

**Rationale:** Defer both (option a at Q3) was rejected on envelope D commitment grounds — operators expect `:metadata()` + dynamic-metadata access at the full-bridge phase; deferring one or both pushes the deliverable out indefinitely. Land `:metadata()` as nil-stub at 22.2 (option b at Q3) was rejected on stub-API hygiene grounds — operators calling a nil-returning function get confused signal; an empty-table return at least mirrors the post-binding-bump shape. The chosen option (option c at Q3) keeps the deferral-discipline-break confined to dynamic-metadata (which has clear cross-filter consumer story) and treats `:metadata()` per-route source as a separate v1.37.x binding-bump concern.

The cross-phase deferral-break is the load-bearing strategic decision — it commits the project to landing `internal/dynamicmetadata/` as a real primitive (not a deferral note) and positions the primitive as the canonical home for cross-filter state read/write going forward.

**Anticipated ADRs:** ADR-0190 §Decision documents the NEW `internal/dynamicmetadata/` framework primitive shape — per-stream `*Bucket` constructor + `Bucket.Get(filter_name, key) (proto.Value, bool)` + `Bucket.Set(filter_name, key, proto.Value)` + per-stream lifecycle (created at filter-chain entry; destroyed at OnDestroy) + cross-filter visibility scoping rules. ADR-0190 §Consequences body documents the cross-phase deferral-lift expectation. ADR-0192 §Decision documents the lua-side bridge methods consuming the primitive (`:streamInfo():dynamicMetadata()` + `:dynamicTypedMetadata()` + `:metadata()` empty-table-at-binding-gap).

### 2.4 Connection bridge surface: full upstream `:connection():ssl()` cert surface *(Q4 → ADR-0192)*

**Decision:** `request_handle:connection()` + `response_handle:connection()` return a connection wrapper with a single primary method `:ssl()`. `:ssl()` returns nil on plaintext connections; on TLS, returns an ssl wrapper with the full upstream Envoy v1.37.2 method set (~12 methods): subject + sanLocalCertificate + sanPeerCertificate + subjectLocalCertificate + subjectPeerCertificate + validFromPeerCertificate + expirationPeerCertificate + sessionId + ciphersuiteId + tlsVersion + urlEncodedPemEncodedPeerCertificate + urlEncodedPemEncodedPeerCertificateChain + sha256PeerCertificateDigest. Integrates with phase-03 TLS primitives.

**Rationale:** Pragmatic-middle SSL subset (option b at Q4) was rejected on envelope D commitment grounds — operators implementing mTLS-based authorization need the full cert surface (sanPeerCertificate for SAN-list matching; sha256PeerCertificateDigest for cert-pinning); cutting to 5 methods forces operators back to non-Lua filter solutions. Defer entirely (option c at Q4) was rejected on the same grounds.

Phase-03 TLS integration is the load-bearing implementation seam — the per-stream connection wrapper sources cert data from the phase-03 `*tls.ConnectionState` (Go stdlib) attached to the per-stream context; the wrapper marshals cert fields into Lua-friendly strings per upstream's wire-format conventions (PEM-encoded; URL-encoded; ISO-8601 timestamps).

**Anticipated ADRs:** ADR-0192 §Decision documents the in-package connection-SSL bridge shape — `internal/filter/http/lua/connection.go` + `internal/filter/http/lua/ssl.go`; per-method cert-data extraction from `*tls.ConnectionState`; wire-format conventions (PEM-encoded with `urlEncodedPem` URL-encoded variant; ISO-8601 timestamps for validFrom/expiration; hex-encoded sha256 digest). The phase-03 TLS integration seam is in-package (no new framework primitive for SSL wrapping; the existing phase-03 ConnectionState exposure is sufficient).

### 2.5 httpCall bridge surface: full upstream parity *(Q5 → ADR-0192 + in-place AMEND ADR-0177)*

**Decision:** `request_handle:httpCall(cluster, headers, body, timeout_ms, asynchronous?)` per upstream signature. `cluster` is a string referencing a configured envoy-go cluster (reuses cluster manager + LB + circuit-breaker + observability). Sync default (script's coroutine yields; Go-side dispatches via `internal/httpclient/`; coroutine resumes with `response_headers, response_body` when the response arrives). Optional `asynchronous = true` flag for fire-and-forget (returns nil immediately; no response retrieval). Timeout enforced via Go context cancellation; cluster's circuit-breaker semantics apply.

**Rationale:** Sync-only + cluster-based (option b at Q5) was rejected on upstream-parity grounds — async dispatch is operationally valuable for tracing-emit / metrics-emit / audit-log patterns where the script doesn't need the response. URL-based + sync-only (option c at Q5) was rejected on cluster-manager-bypass grounds — losing LB/circuit-breaker/observability is a substantial regression vs upstream.

The cluster-based dispatch is the load-bearing integration decision — envoy-go's cluster manager (phase-01 static config + phase-05 H2 + future phase XDS) provides the routing primitives; the lua httpCall bridge delegates to cluster manager rather than reimplementing routing.

**Anticipated ADRs:** ADR-0192 §Decision documents the in-package httpCall bridge shape — `internal/filter/http/lua/httpcall.go`; sync vs async dispatch paths; coroutine yield/resume integration via `internal/lua/` extensions (ADR-0191); stat-counter wiring (httpcall_total + httpcall_failures + httpcall_timeouts). ADR-0177 IN-PLACE AMENDMENT body at the same 22.2 IMPL atomic-landing Task per ADR-0044 in-place edit discipline — documents the cluster-based dispatch extension (`internal/httpclient/` API gains `ClusterDispatch(ctx, cluster, request) (*http.Response, error)` consumer-#1-triggered method; matches phase-17 → phase-18 ADR-0149 → ADR-0150 AMEND precedent). No new ADR number consumed for the httpclient AMEND.

### 2.6 Crypto bridge surface: 6 upstream methods, in-package *(Q6 → ADR-0192)*

**Decision:** All 6 upstream Envoy crypto methods at 22.2: `:base64Escape(s)` + `:base64Decode(s)` + `:sha256(s)` + `:sha512(s)` + `:importPublicKey(pem)` + `:verifySignature(key, signature, data, algorithm)`. All in-package at `internal/filter/http/lua/crypto.go`; thin wrappers over Go stdlib `crypto/sha256`, `crypto/sha512`, `encoding/base64`, `crypto/x509`. `:importPublicKey()` returns an opaque public-key userdata (Lua-side cannot inspect; Lua-side can pass to `:verifySignature()`). `:verifySignature()` returns bool — true if signature verifies against key + data per algorithm; false otherwise.

**Rationale:** Pragmatic subset (option b at Q6) was rejected on envelope D commitment grounds — `:importPublicKey()` + `:verifySignature()` are operationally important for HMAC-token validation patterns + webhook-signature verification patterns; deferring them pushes the deliverable out indefinitely. Full upstream + envoy-go-strict extensions (option c at Q6) was rejected on BEHAVIOR_CONTRACT departure-record cost grounds — each envoy-go-strict extension requires a departure record; 4 extra extensions (HMAC pair + sha1 + base64URL) would each need a separate record; cumulative cost outweighs the operator value of extensions that operators can implement in-Lua atop the 6 upstream methods (e.g., HMAC via base64Escape(sha256(key .. data))).

The in-package landing is per Q9 — no framework primitive extraction; the wrappers are trivial Go-stdlib calls; no consumer-#2 anticipated.

**Anticipated ADRs:** ADR-0192 §Decision documents the in-package crypto bridge shape — file at `internal/filter/http/lua/crypto.go`; per-method Go-stdlib wrapper signatures; `:importPublicKey()` userdata type discipline (per gopher-lua's `*lua.LUserData` patterns); `:verifySignature()` algorithm-name → Go-API mapping table (e.g., `"SHA256"` → `crypto.SHA256`; `"SHA512"` → `crypto.SHA512`).

### 2.7 fileBytes + timestamp bridge surface: full upstream parity *(Q7 → ADR-0192)*

**Decision:** `:fileBytes(path)` allows arbitrary filesystem paths + 16 MiB size cap (inherits 22.1 Task 11 cap pattern from `DataSource.Filename` — `parseRejectDataSourceFilenameTooLargeFmt` wording reused for the runtime-error message); reads on every call (no implicit cache; operators cache at script init if needed via `local cached_bytes = handle:fileBytes("/path/to/file")` at the top of the script). `:timestamp(unit?)` returns wall-clock time as a Lua number; default unit is `'milliseconds'`; optional arg `'milliseconds'` | `'microseconds'` | `'seconds'`. Sources from Go's `time.Now()`.

**Rationale:** Hardened envoy-go-strict (option b at Q7) was rejected on operator-flow-disruption grounds — config-declared `Lua.allowed_file_paths` allow-list would force operators to anticipate every `:fileBytes()` call site at config-author time; envoy-go-strict departure record for the allow-list discipline is also a substantial cost; the operator-supplied Lua script already runs in a trust-domain (operator-trusted code) so unrestricted FS access is acceptable. Defer both (option c at Q7) was rejected on envelope D commitment grounds.

Non-deterministic `:timestamp()` interacts with fixture-0027 strategy (Q12) — scenarios using `:timestamp()` go in the REFERENCE-LESS subject-only slot of the mixed-mode fixture-0027 (per parent SPEC §8.6 fixture-0027 anticipation).

**Anticipated ADRs:** ADR-0192 §Decision documents the in-package fileBytes + timestamp bridge shape — file at `internal/filter/http/lua/misc.go`; 16 MiB cap via `io.LimitReader(f, maxFileBytesScriptSize+1)` (mirrors 22.1's `resolveDataSourceFilename` cap pattern); timestamp via `time.Now().UnixMilli()` / `UnixMicro()` / `Unix()` per unit arg.

### 2.8 streamInfo-full bridge surface: 7 additional methods + filter-state in-package *(Q8 → ADR-0192)*

**Decision:** 22.2 adds 7 streamInfo methods on top of 22.1's 4-method subset (`:protocol()` + `:routeName()` + `:downstreamLocalAddress()` + `:downstreamDirectRemoteAddress()`): `:upstreamHost()` + `:upstreamCluster()` + `:dynamicMetadata()` + `:dynamicTypedMetadata(filter_name)` + `:requestedServerName()` + `:filterState()` + `:downstreamSslConnection()`. Total 11-method streamInfo surface at 22.2 phase-done.

- `:upstreamHost()` returns upstream host string from per-stream upstream-selection state; nil if upstream not yet selected.
- `:upstreamCluster()` returns upstream cluster name from per-stream upstream-selection state; nil if upstream not yet selected.
- `:dynamicMetadata()` returns the dynamic-metadata bucket accessor (per Q3 + Q9 NEW `internal/dynamicmetadata/` primitive).
- `:dynamicTypedMetadata(filter_name)` returns typed-metadata accessor for filter_name.
- `:requestedServerName()` returns TLS SNI value; nil on plaintext.
- `:filterState()` returns filter-state accessor — IN-PACKAGE at `internal/filter/http/lua/filterstate.go` per Q9 EXTRACT-NOW-only-when-trigger-fires; per-stream string-keyed map sourced from a NEW per-stream context field added at 22.2 (added to the per-stream context shared across filters; no framework primitive at 22.2; second consumer triggers `internal/filterstate/` extraction).
- `:downstreamSslConnection()` returns the ssl wrapper (same wrapper type as Q4's `:connection():ssl()`; semantically redundant; matches upstream).

**Rationale:** Subset minus filterState (option b at Q8) was rejected on envelope D commitment grounds — operators porting upstream Lua scripts that touch filter-state would hit immediate Lua runtime errors; the IN-PACKAGE landing absorbs the consumer-#1-only cost without committing to a primitive extraction. Subset minus filterState + minus downstreamSslConnection (option c at Q8) was rejected on the same grounds + the redundancy of `:downstreamSslConnection()` vs `:connection():ssl()` mirrors upstream's intentional convenience-overload (operators may prefer `streamInfo` over `connection` for path-symmetry reasons).

**Anticipated ADRs:** ADR-0192 §Decision documents the streamInfo-full bridge shape — extensions to `internal/filter/http/lua/streaminfo.go` from 22.1; per-method source mapping (upstream-selection state for upstreamHost/Cluster; `internal/dynamicmetadata/` for the metadata accessors; phase-03 TLS for requestedServerName + downstreamSslConnection; in-package per-stream filter-state map for filterState). Filter-state in-package shape: `map[string]any` per-stream + bridge methods `:filterState():get(name) any` + `:filterState():set(name, value)` + Lua-table marshaling via gopher-lua's `LValue` conversion.

### 2.9 Framework primitive extraction posture: pragmatic-middle *(Q9 → ADR-0190 + ADR-0191 + in-place AMEND ADR-0177 + IN-PACKAGE for the rest)*

**Decision:** Extract NEW `internal/dynamicmetadata/` framework primitive at 22.2 (clear multi-consumer story per the cross-phase deferral break — see §1.6). Revise `internal/lua/` API for coroutine yield/resume + body-bridge buffer seam via NEW ADR-0191 (per Q10 strict scope). Revise `internal/httpclient/` API for cluster-based dispatch via in-place AMENDMENT on ADR-0177 (per ADR-0044). Keep IN-PACKAGE: filter-state primitive (no second consumer anticipated; EXTRACT-NOW-only-when-trigger-fires per phase-21 lesson); body-buffer seam (in-package consumer of the new `internal/lua/` API; no second consumer anticipated); crypto wrappers (trivial Go-stdlib); connection-SSL wrapper (in-package consumer of phase-03 TLS state).

**Rationale:** Aggressive EXTRACT-NOW (option b at Q9) was rejected on filter-state-second-consumer-uncertainty grounds — extracting `internal/filterstate/` at first consumer would force the primitive API shape on speculative future consumers without empirical validation; the phase-21 lesson (extracted `internal/clock/` was DECLINED in favor of inline Clock per the same uncertainty) applies. Conservative IN-PACKAGE (option c at Q9) was rejected on dynamic-metadata-cross-phase-discipline grounds — the cross-phase deferral break (§1.6) requires the primitive to actually exist (not just be a future-pointer); keeping dynamic-metadata in-package would maroon prior phases' deferred surfaces.

The pragmatic-middle is the load-bearing extraction-posture decision — 1 NEW primitive at 22.2 (dynamicmetadata) keeps the framework footprint minimal while honoring the cross-phase commitment; the `internal/lua/` extensions and `internal/httpclient/` AMENDMENT are AS-NEEDED revisions (not new primitives).

**Anticipated ADRs:** ADR-0190 §Decision body documents `internal/dynamicmetadata/` primitive (see §2.3 for shape). ADR-0191 §Decision body documents `internal/lua/` API extensions (coroutine yield/resume + body-bridge buffer seam; see §2.1 for shape). ADR-0177 in-place AMENDMENT body documents cluster-based dispatch extension (see §2.5 for shape).

### 2.10 ADR-0188 API-revision allowance scope: strict — NEW ADR-0191 for 22.2 extensions *(Q10 → ADR-0191)*

**Decision:** ADR-0188's EXPLICIT API-REVISION ALLOWANCE clause stays scoped to consumer-#2 (future cluster-specifier Lua / access-logger Lua / string-matcher Lua per ADR-0188 §Decision §3). 22.2 is consumer-#1-scope-expanded (still HTTP filter Lua); the scope-expansion API revisions (coroutine yield/resume + body-bridge buffer seam) land under NEW ADR-0191 instead of an in-place AMENDMENT on ADR-0188. ADR-0188 stays UNCHANGED at 22.2 IMPL.

**Rationale:** Apply allowance to scope-expansion + in-place AMEND (option a at Q10) was rejected on lineage-separation grounds — over-loading ADR-0188 with consumer-#1-expansion AMENDMENTs would dilute the consumer-#2 ALLOWANCE's empirical-validation rationale (ADR-0188's allowance is a FUTURE-USE allowance triggered by a different consumer's empirical validation; mixing it with same-consumer scope-expansion confuses the lineage). No API revisions (option c at Q10) was rejected on duplication grounds — building coroutine + body-bridge surfaces outside `internal/lua/` would duplicate primitive-like code at the lua filter; the second `internal/lua/` consumer at the future consumer-#2 phase would then face TWO API surfaces to validate.

The strict-scope decision is the load-bearing ADR-lineage decision — keeps each ADR's scope tightly bound to a single semantic event (ADR-0188 = primitive landing at consumer-#1; ADR-0191 = primitive extensions at consumer-#1-scope-expansion; future ADR for consumer-#2 = primitive extensions per the ADR-0188 ALLOWANCE).

**Anticipated ADRs:** ADR-0191 anchored at 22.2 IMPL atomic-landing Task — title: "`internal/lua/` 22.2 API extensions for coroutine yield/resume + body-bridge buffer seam at HTTP filter Lua consumer-#1 scope-expansion". ADR-0191 §Context block authored at 22.2 SPEC commit per ADR-0044 §Context-draft discipline; §Decision + §Consequences bodies anchored at 22.2 IMPL atomic-landing Task.

### 2.11 Stat surface hypothesis: pragmatic-middle ~5-6 counters (102 → ~108) *(Q11 → ADR-0192 + BEHAVIOR_CONTRACT departure records)*

**Decision:** 5-6 NEW envoy-go-strict counters under the existing `lua.<config_stat_prefix>.<stat>` prefix template (from 22.1; HCM-rooted per AMEND-2): `httpcall_total` + `httpcall_failures` + `httpcall_timeouts` + `body_buffered_bytes_total` + `coroutine_yields_total`. Project stat count 102 → ~108 (delta +5 or +6 if a counter is split). BEHAVIOR_CONTRACT.md gets 5-6 NEW envoy-go-strict departure records (each counter has its own record; matches 22.1's 3-record pattern for errors + executions + respond_calls but with the cohort focused on httpCall + body + coroutine).

**Rationale:** Maximalist (option b at Q11) was rejected on departure-record-overhead grounds — 10 envoy-go-strict counters = 10 departure records = ~150 LoC of BEHAVIOR_CONTRACT.md per record × 10 = ~1500 LoC for stat documentation alone; dynamicMetadata_reads/writes + filterState_reads/writes + crypto_invocations are mostly debug-info counters that operators rarely consult at production. Minimal (option c at Q11) was rejected on operator-observability grounds — body_buffered_bytes_total is operationally important for capacity planning (large-body operators need to know aggregate buffer pressure); coroutine_yields_total is operationally important for perf debugging (yield-heavy scripts indicate inefficient body-streaming patterns).

The pragmatic-middle is the load-bearing stat-discipline decision — focuses operator visibility on the operationally-load-bearing surfaces (httpCall outbound observability + body-buffer capacity planning + coroutine perf debugging) while avoiding per-bridge-method micro-counters that operators rarely consult.

**Anticipated ADRs:** ADR-0192 §Decision body documents the 22.2 stat surface roster + the envoy-go-strict departure justifications + the BEHAVIOR_CONTRACT departure-record discipline. The 5-6 NEW departure records anchor at the 22.2 IMPL atomic-landing Task per ADR-0052 (matches 22.1's 3-record pattern). Stat-prefix template UNCHANGED from 22.1 (`http.<HCM_stat_prefix>.lua.<config_stat_prefix>.<stat>` per AMEND-2).

### 2.12 Differential fixture strategy: single mixed-mode fixture-0027 *(Q12 → ADR-0192)*

**Decision:** ONE fixture directory `test/fixtures/0027-http-lua-full-bridge/` with both deterministic scenarios (cross-side `CompareBytes` byte-exact: body / trailers / metadata-empty / crypto / streamInfo-most / fileBytes / connection-ssl) and non-deterministic scenarios (REFERENCE-LESS subject-only: httpCall / timestamp / filterState). Matches fixture-0026 multi-scenario precedent (scenarios (a)-(f) cross-side + scenario (g) BootRejectFixture in one fixture dir). 28 → 29 fixture directories at 22.2 phase-done.

**Rationale:** Two-fixture split (option b at Q12) was rejected on fixture-directory-count grounds — splitting 0027a + 0027b adds 1 more fixture directory than necessary; the fixture-0007a/b CORS precedent was an exceptional case driven by different upstream Envoy CORS-filter binding states between scenarios (a) and (b), not a multi-scenario architectural pattern. Full REFERENCE-LESS subject-only (option c at Q12) was rejected on envelope D empirical-verification grounds — many 22.2 surfaces ARE deterministic on the wire (body / trailers / metadata-empty / crypto / streamInfo-most / fileBytes); skipping cross-side comparison loses the empirical envelope D verification for those surfaces.

The single mixed-mode is the load-bearing fixture-discipline decision — matches the 22.1 fixture-0026 precedent (one directory holding heterogeneous scenarios) and the parent SPEC §8.6 anticipation; introduces ONE new fixture directory at 22.2 (vs 2 at option b).

**Anticipated ADRs:** ADR-0192 §Decision body documents the fixture-0027 scenario taxonomy (likely 10-14 scenarios: 7-9 deterministic cross-side + 3-5 non-deterministic REFERENCE-LESS) + the per-scenario script file roster + the cross-side vs subject-only per-scenario discipline. Fixture-0027 reuses the 22.1 `BackendKind=HTTPLua` + `scripts/` subdirectory pattern (no new BackendKind). Fixture-0027 may introduce NEW driver helpers for the REFERENCE-LESS subject-only scenarios (e.g., `RunSubjectOnlyHTTPLua` helper analogous to 22.1's `BootRejectFixture`) — settled at 22.2 SPEC §13-R review.

### 2.13 D-hypothesis prediction: WEAK HOLD *(Q13 → 0-1 escape-valve at 22.2 IMPL)*

**Decision:** BRAINSTORM-time prediction for 22.2 IMPL: WEAK HOLD — 3 anticipated NEW ADRs (ADR-0190 + ADR-0191 + ADR-0192) land cleanly + in-place AMEND on ADR-0177 (no number consumed) + 0-1 escape-valve consumption (ADR-0193 = conditional). The most likely escape-valve surfaces at 22.2 IMPL: (a) **R6 *LState-pool gate** — body + coroutine extensions may push per-stream construction cost over the 1ms threshold (22.1 baseline ~70µs; body + coroutine + filter-state in-package likely add ~30-50% — still well under 1ms but close enough to warrant remeasurement at 22.2 IMPL benchmark Task); (b) **body-buffer-seam-with-ADR-0128 separation** — the body-bridge buffer seam may surface implementation-time complexity warranting its own ADR (split from ADR-0192 in-package); (c) **connection-SSL bridge integration with phase-03** — TLS state extraction may surface phase-03 ADR amendments warranting a separate ADR; (d) **httpCall coroutine integration with cluster manager** — async semantics may surface cluster-manager interface refinements warranting a separate ADR.

**Rationale:** STRONG HOLD (option b at Q13) was rejected on first-of-kind-extension-risk grounds — 22.2 is the first phase to extend a NEW framework primitive (`internal/lua/` from 22.1); the API-revision risk surface (per Q10 strict scope = NEW ADR-0191 rather than ADR-0188 AMEND) is non-trivial and may surface IMPL-time refinements. BREAK (option c at Q13) was rejected on over-pessimism grounds — the 3 anticipated NEW ADRs cover the major design decisions cleanly; 4-5 NEW would require pre-identifying which sub-surfaces split, which the BRAINSTORM doesn't have enough impl evidence to do.

The WEAK HOLD is the load-bearing D-hypothesis decision — matches 22.1's WEAK HOLD prediction (which held with 0 escape-valve consumption; R6 STANDS WEAK-default at ~70µs); allows for one escape-valve consumption to absorb IMPL-time discoveries without forcing BRAINSTORM-time pre-resolution.

**Anticipated ADRs:** WEAK-HOLD prediction means the BRAINSTORM-anticipated ADR roster for 22.2 IMPL is: ADR-0190 (NEW `internal/dynamicmetadata/`) + ADR-0191 (NEW `internal/lua/` 22.2 extensions) + ADR-0192 (NEW `internal/filter/http/lua/` 22.2 package shape extensions) + in-place AMEND on ADR-0177 (no number consumed) + conditional ADR-0193 (escape-valve consumption — fires only at one of the surfaces above). Next-free ADR after 22.2 IMPL: ADR-0193 (if escape-valve not consumed) or ADR-0194 (if escape-valve consumed).

### 2.14 Scope shape: single-phase at BRAINSTORM *(Q14 → no split at this BRAINSTORM commit)*

**Decision:** 22.2 stays as one ROADMAP row at this BRAINSTORM commit. The PLAN session does the precise estimation against the ADR-0045 split-gate (~25 tasks / ~1500 LoC); if exceeds, 22.2 splits into 22.2.1 + 22.2.2 at PLAN time per the phase-09 → phase-11 + phase-13 split-at-PLAN precedent. BRAINSTORM artefacts (this file) are NOT invalidated by a future PLAN-time split; the SPEC + PLAN sessions inherit the BRAINSTORM design intent.

**Rationale:** Pre-split at BRAINSTORM (option b at Q14) was rejected on imperfect-estimate grounds — task estimates this early in the lifecycle would force a suboptimal split-axis; 22.2's per-surface decisions don't decompose along any obvious clean axis (no per-stream-state-vs-out-of-band cut, no body-vs-metadata-vs-connection cut). Letting PLAN session see the real Task graph (with TDD-ordered Steps, per-Task subagent dispatch outlines, parallelization opportunities) and decide is the more disciplined approach.

The single-phase-at-BRAINSTORM decision is the load-bearing scope-shape decision — preserves the cold-start prompt's single-phase 22.2 framing; defers split-axis selection to the PLAN session where empirical task estimates are available.

**Anticipated ADRs:** None at BRAINSTORM time. If 22.2 splits at PLAN, an ADR documenting the PLAN-time split (matches phase-13 ADR-0099 precedent? — confirm at PLAN time) may anchor.

---

## 3. Framework-survey result — 1 NEW package-level primitive at 22.2 + 1 in-place AMEND ADR-0177 + 2 NEW ADRs for extensions + 4 IN-PACKAGE surfaces

Phase 22.2 introduces 1 NEW package-level framework primitive at this sub-phase (`internal/dynamicmetadata/` per Q9 + Q3) + 1 in-place AMEND on `internal/httpclient/` (ADR-0177 per Q5 + Q9) + 2 NEW ADRs for primitive-level extensions (ADR-0191 for `internal/lua/` API extensions per Q10 strict scope + ADR-0192 for `internal/filter/http/lua/` package shape extensions per Q1-Q8) + 4 in-package surfaces (filter-state + body-buffer seam + connection-SSL + crypto + fileBytes + timestamp — all consumers of existing primitives without warranting their own).

### 3.1 NEW: `internal/dynamicmetadata/` framework primitive *(per Q9 EXTRACT-NOW; anchored at ADR-0190; lands at 22.2)*

**Decision:** Extract the cross-filter dynamic-metadata accessor as a NEW `internal/dynamicmetadata/` framework primitive at 22.2. Package boundary: `internal/dynamicmetadata/` hosts the per-stream `*Bucket` constructor + accessor API; consumers (lua filter; future jwt_authn / rbac / ext_proc / ext_authz extensions) consume the API via their per-stream context. Per-stream lifecycle: `*Bucket` created at filter-chain entry (HCM per-stream context); destroyed at OnDestroy.

The primitive's API shape (provisional; 22.2 SPEC settles):
- `dynamicmetadata.NewBucket() *Bucket` — construct a per-stream metadata bucket
- `Bucket.Get(filter_name string, key string) (*structpb.Value, bool)` — read a key
- `Bucket.Set(filter_name string, key string, value *structpb.Value)` — write a key
- `Bucket.Snapshot() map[string]map[string]*structpb.Value` — snapshot for read-only iteration (for the lua bridge's typed metadata accessor)
- Lifecycle: per-stream; thread-safe per-filter (filters run sequentially within a stream goroutine in envoy-go per ADR-0033; no cross-filter concurrency)

The primitive serves as the canonical home for cross-filter state read/write going forward — see §1.6 for the cross-phase deferral-lift expectation.

### 3.2 EXTENSION (NEW ADR-0191): `internal/lua/` API for coroutine yield/resume + body-bridge buffer seam *(per Q10 strict scope; anchored at ADR-0191; lands at 22.2)*

**Decision:** Extend `internal/lua/` API at 22.2 with coroutine yield/resume support + body-bridge buffer seam interface. Per Q10 strict scope, the extensions land under NEW ADR-0191 instead of in-place AMEND on ADR-0188 — the ADR-0188 API-REVISION ALLOWANCE stays scoped to consumer-#2.

The extension API shape (provisional; 22.2 SPEC settles):
- `VM.Yield(value any) (any, error)` — yields the current Lua coroutine; returns when resumed
- `VM.Resume(args ...any) error` — resumes a yielded coroutine with args (called by Go-side scheduling)
- `WithCoroutineYieldHandler(handler YieldHandler) VMOption` — VM option for coroutine yield-handler registration
- `BodyBuffer interface { Bytes() []byte; Chunks() [][]byte }` — body-buffer seam interface (consumed by lua bridge's `:body()` + `:bodyChunks()` methods; implemented by phase-13 ADR-0128 buffer)

Body-buffer seam is the integration point between `internal/lua/` API + ADR-0128 — the lua bridge layer wraps the ADR-0128 buffer in a `BodyBuffer` interface implementation; gopher-lua's string constructors back the Lua-side strings with the buffer's byte slices (zero-copy).

### 3.3 IN-PLACE AMEND: `internal/httpclient/` (ADR-0177) for cluster-based dispatch *(per Q5 + Q9; in-place AMEND on ADR-0177; lands at 22.2)*

**Decision:** Extend `internal/httpclient/` API at 22.2 with cluster-based dispatch via in-place AMENDMENT on ADR-0177 per ADR-0044 in-place edit discipline (matches phase-17 → phase-18 ADR-0149 → ADR-0150 AMEND precedent). The phase-20 oauth2 consumer used URL-based dispatch; the lua httpCall consumer needs cluster-based dispatch (resolving cluster name → upstream selection via cluster manager + LB).

The extension API shape (provisional; 22.2 SPEC settles):
- `Client.ClusterDispatch(ctx context.Context, cluster string, request *http.Request) (*http.Response, error)` — cluster-based dispatch path
- Integrates with `internal/cluster/` cluster manager for cluster name resolution
- Integrates with cluster's circuit-breaker + LB + observability primitives
- Timeout via context cancellation; cluster's timeout config applies if context timeout absent

No new ADR number consumed for the httpclient AMEND (per ADR-0044 in-place edit discipline — AMENDMENT body lands at the same 22.2 IMPL atomic-landing Task as ADR-0190 + ADR-0191 + ADR-0192).

### 3.4 IN-PACKAGE surfaces: filter-state + body-buffer seam + connection-SSL + crypto + fileBytes + timestamp

Per Q9 + Q8 + Q4 + Q6 + Q7: filter-state, body-buffer seam (consumer-side), connection-SSL wrapper, crypto wrappers, and fileBytes + timestamp helpers all land IN-PACKAGE at `internal/filter/http/lua/` without warranting their own framework primitive extraction. Rationale per surface in §2 above.

---

## 4. Per-route shape — UNCHANGED (`LuaPerRoute` parsing stays at 22.3)

22.2 does NOT touch the per-route shape; ADR-0125 §(xiv) AMENDMENT-anticipation paragraph stays UNCHANGED at 22.2 (anchored at parent SPEC commit `41ccee7`; AMENDMENT body lands at 22.3 IMPL final Task per ADR-0044). `LuaPerRoute` PARSE-LIFT happens at 22.3 paired with the 9th canonical AMENDMENT.

The `:metadata()` bridge surface at 22.2 returns empty regardless of any future `LuaPerRoute` parsing — because v1.32.4 binding-gap (`LuaPerRoute.filter_context` is v1.37.2-only) means there's NO source-of-metadata-data to plumb through, even with full LuaPerRoute parsing. The bridge surface activates with real data at the future v1.32.4 → v1.37.x binding bump phase (per AMEND-12 + parent SPEC §10 items 16-17).

---

## 5. Stat surface hypothesis (102 → ~108)

### 5.1 22.2 stat-surface roster (5-6 counters; project 102 → ~108)

| Counter | Source | Rationale |
|---|---|---|
| `lua.<prefix>.httpcall_total` | `:httpCall()` invocation count | Outbound-call budget + observability |
| `lua.<prefix>.httpcall_failures` | `:httpCall()` response 4xx/5xx + transport errors | Error rate observability |
| `lua.<prefix>.httpcall_timeouts` | `:httpCall()` timeout firings | Timeout-rate observability (distinct from generic failures) |
| `lua.<prefix>.body_buffered_bytes_total` | `:body()` + `:bodyChunks()` byte counts | Body-buffer capacity planning |
| `lua.<prefix>.coroutine_yields_total` | `:body()` + `:httpCall()` coroutine yield events | Perf debugging (yield-heavy = inefficient body-streaming) |

5 counters confirmed; +1 candidate `lua.<prefix>.dynmd_writes_total` (dynamic-metadata write count) deferred to 22.2 SPEC for OK/NOK decision (omitted from pragmatic-middle per Q11 unless SPEC scrape surfaces operator-value signal).

### 5.2 Stat-prefix template — UNCHANGED from 22.1

Template: `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.<stat>` per AMEND-2 + 22.1 SPEC §7.2. Empty `Lua.stat_prefix` produces literal consecutive-dot names per phase-14 compressor precedent. UNCHANGED at 22.2.

### 5.3 Project stat count delta

| Phase | Stat count | Delta |
|---|---|---|
| Phase 21 phase-done | 99 | — |
| Phase 22.1 phase-done | 102 | +3 (errors + executions + respond_calls) |
| Phase 22.2 phase-done | ~108 | +5 or +6 (httpcall trio + body_buffered_bytes + coroutine_yields [+ optional dynmd_writes]) |

### 5.4 envoy-go-strict departure rationale + BEHAVIOR_CONTRACT.md departure-record discipline

All 5-6 NEW counters are envoy-go-strict (upstream Envoy Lua filter has only 2 counters: errors + executions per `ALL_LUA_FILTER_STATS` macro in `lua_filter.h:23-24` per parent SPEC §11.5.1). Each NEW counter requires a BEHAVIOR_CONTRACT.md envoy-go-strict departure record per the project's discipline (matches 22.1's 3-record pattern for errors-unchanged-from-upstream + executions-unchanged + respond_calls-envoy-go-strict).

5-6 NEW envoy-go-strict departure records land at 22.2 IMPL atomic-landing Task per ADR-0052 atomic landing (matches 22.1's 3-record pattern; same Task as the BEHAVIOR_CONTRACT.md bundle for the bridge surface delta + the ADR landings).

---

## 6. Differential fixture strategy — single mixed-mode fixture-0027 *(per Q12)*

### 6.1 Fixture-0027 shape

ONE fixture directory `test/fixtures/0027-http-lua-full-bridge/` at 22.2 phase-done. Both deterministic + non-deterministic scenarios in one directory; per-scenario cross-side vs REFERENCE-LESS subject-only discipline.

### 6.2 Scenario taxonomy (provisional; 22.2 SPEC settles)

| Scenario | Surface | Determinism | Fixture mode |
|---|---|---|---|
| (a) body whole-buffer | `:body()` | deterministic | cross-side `CompareBytes` |
| (b) body chunks iterator | `:bodyChunks()` | deterministic | cross-side `CompareBytes` |
| (c) trailers add+remove | `:trailers()` mutation | deterministic | cross-side `CompareBytes` |
| (d) metadata empty-table at binding-gap | `:metadata()` | deterministic | cross-side `CompareBytes` |
| (e) dynamic-metadata read+write | `:streamInfo():dynamicMetadata()` | deterministic | cross-side `CompareBytes` |
| (f) connection-ssl SAN extraction | `:connection():ssl():sanPeerCertificate()` | deterministic (matching certs both sides) | cross-side `CompareBytes` |
| (g) crypto sha256 + base64 | `:sha256()` + `:base64Escape()` | deterministic | cross-side `CompareBytes` |
| (h) fileBytes read | `:fileBytes()` | deterministic (fixed file content) | cross-side `CompareBytes` |
| (i) streamInfo upstreamHost + upstreamCluster | `:streamInfo():upstreamHost()` + `:upstreamCluster()` | deterministic | cross-side `CompareBytes` |
| (j) httpCall sync upstream cluster call | `:httpCall(cluster, ..., async=nil)` | non-deterministic (async wire-shape) | REFERENCE-LESS subject-only |
| (k) httpCall async fire-and-forget | `:httpCall(cluster, ..., async=true)` | non-deterministic | REFERENCE-LESS subject-only |
| (l) timestamp wall-clock | `:timestamp('milliseconds')` | non-deterministic | REFERENCE-LESS subject-only |
| (m) filterState cross-filter set+get | `:streamInfo():filterState():set()` + `:get()` | mixed-determinism | REFERENCE-LESS subject-only |

13 scenarios provisional; ~9 deterministic cross-side + ~4 non-deterministic REFERENCE-LESS. 22.2 SPEC scrubs the roster + adds boot-reject scenarios (e.g., NEW PARSE-REJECT arms for cluster-name-resolution-failure + body-size-cap + crypto-key-format-invalid).

### 6.3 Backend reuse + listener topology

Reuses 22.1's `BackendKind=HTTPLua` (no new BackendKind) + `scripts/` subdirectory pattern. Listener topology likely multi-listener (one listener per scenario) per the 22.1 fixture-0026 pattern + parent SPEC §8.4 recommended structure; 22.2 SPEC scrubs the listener-topology decision.

### 6.4 Cross-side discipline for cross-side scenarios

Cross-side scenarios use the existing `CompareBytes` runner step 7 (no new harness infrastructure). The REFERENCE-LESS subject-only scenarios use either an existing helper (e.g., 22.1's `BootRejectFixture` substring-match path) or a NEW driver helper (e.g., `RunSubjectOnlyHTTPLua` analogous to `BootRejectFixture`); 22.2 SPEC §13-R review decides which.

---

## 7. Anticipated ADRs — ADR-0190 + ADR-0191 + ADR-0192 + in-place AMEND ADR-0177 + conditional ADR-0193 *(per Q9 + Q10 + Q13)*

### 7.1 ADR-0190 — NEW `internal/dynamicmetadata/` framework primitive

Per §2.3 + §3.1. Per-stream cross-filter dynamic-metadata accessor. Lands at 22.2 IMPL atomic-landing Task; §Context block authored at 22.2 SPEC commit per ADR-0044 §Context-draft discipline.

**Lands-in:** 22.2 IMPL atomic-landing Task (likely Task ≤25-35 depending on PLAN-time scope).
**Title (provisional):** "NEW `internal/dynamicmetadata/` framework primitive — per-stream `*Bucket` accessor for cross-filter dynamic-metadata read+write at first co-consumer (HTTP Lua filter 22.2) per phase-22 BRAINSTORM Q3 cross-phase-deferral-break + Q9 EXTRACT-NOW"

### 7.2 ADR-0191 — NEW `internal/lua/` 22.2 API extensions for coroutine + body-bridge buffer seam

Per §2.1 + §3.2 + Q10 strict scope. NEW ADR (NOT in-place AMEND on ADR-0188) for `internal/lua/` extensions at consumer-#1-scope-expansion. ADR-0188's API-REVISION ALLOWANCE stays scoped to consumer-#2.

**Lands-in:** 22.2 IMPL atomic-landing Task (same Task as ADR-0190 + ADR-0192).
**Title (provisional):** "`internal/lua/` 22.2 API extensions for coroutine yield/resume + body-bridge buffer seam at HTTP filter Lua consumer-#1 scope-expansion"

### 7.3 ADR-0192 — NEW `internal/filter/http/lua/` 22.2 package shape extensions

Per §2.1-§2.8 + §2.11 + §2.12. NEW ADR documenting the full 22.2 bridge surface delta + filter-state in-package + crypto in-package + connection-SSL in-package + fixture-0027 disposition.

**Lands-in:** 22.2 IMPL atomic-landing Task (same Task as ADR-0190 + ADR-0191).
**Title (provisional):** "`internal/filter/http/lua/` 22.2 package shape extensions — body + trailers + metadata + connection-SSL + httpCall + crypto + fileBytes + timestamp + streamInfo-full + filter-state in-package bridge methods + 5-6 envoy-go-strict departure records + fixture-0027 mixed-mode discipline"

### 7.4 IN-PLACE AMEND on ADR-0177 — `internal/httpclient/` cluster-based dispatch

Per §2.5 + §3.3. In-place AMENDMENT body on ADR-0177 §Decision at 22.2 IMPL atomic-landing Task per ADR-0044 in-place edit discipline. No new ADR number consumed.

### 7.5 Conditional ADR-0193 — escape-valve slot per WEAK HOLD prediction *(per Q13)*

Per §2.13. Fires only at one of the §2.13 surfaces (R6 *LState-pool gate OR body-buffer-seam-with-ADR-0128 separation OR connection-SSL bridge integration with phase-03 OR httpCall coroutine integration with cluster manager). If R6 STANDS WEAK-default again at 22.2 IMPL benchmark Task + no other unanticipated landings, ADR-0193 stays UNCONSUMED.

### 7.6 D-hypothesis prediction summary

| Hypothesis | Anticipated NEW ADRs | In-place AMEND | Conditional | Net consumption |
|---|---|---|---|---|
| WEAK HOLD (chosen) | 3 (ADR-0190/0191/0192) | 1 (ADR-0177) | 0-1 (ADR-0193) | 3-4 NEW |
| STRONG HOLD (rejected) | 3 (ADR-0190/0191/0192) | 1 (ADR-0177) | 0 | 3 NEW |
| BREAK (rejected) | 4-5 | 1 (ADR-0177) | 1-2 | 5-7 NEW |

Next-free ADR after 22.2 BRAINSTORM commit: UNCHANGED (ADR-0190 stays next-free per ADR-0044 §Context-draft discipline; NO ADR consumption at BRAINSTORM commit). ADR consumption happens at the 22.2 IMPL atomic-landing Task.

---

## 8. Deferred items (~6-10 items; forward to 22.3 / future / cross-phase)

1. **`Lua.SourceCodes` multi-script map activation** — 22.3 IMPL Tasks per ADR-0106 + parent BRAINSTORM Q2 + parent SPEC §3.2.
2. **`LuaPerRoute` 3-arm oneof PARSE-LIFT + dispatch + 9th canonical ADR** — 22.3 IMPL Tasks; ADR-0125 §(xiv) AMENDMENT body at 22.3 IMPL final Task.
3. **Per-route 3-tier dispatch (listener-default → SourceCodes-named-script → per-route DataSource override)** — 22.3 SPEC settles dispatch semantics.
4. **`:metadata()` per-route source activation** — future v1.32.4 → v1.37.x binding bump per AMEND-12 + parent SPEC §10 items 16-17. The `:metadata()` bridge surface activates with real data at the binding-bump phase.
5. **Cluster-specifier Lua + access-logger Lua + string-matcher Lua** — future cross-family phases; consumers #2/3/4 for `internal/lua/`; each future phase BRAINSTORM revisits the API shape per ADR-0188's EXPLICIT API-REVISION ALLOWANCE clause (NOT consumed by 22.2's consumer-#1 scope-expansion per Q10 strict scope).
6. **`internal/filterstate/` framework primitive extraction** — future; `:filterState()` lands IN-PACKAGE at 22.2 per Q8 + Q9; future phase that adds a second filter-state consumer extracts the primitive.
7. **HMAC + SHA-1 + base64-URL crypto extensions** — future (possibly never); Q6 chose full upstream parity (6 methods) over envoy-go-strict extension (10 methods).
8. **`:connection()` non-SSL methods (remoteAddress / remoteIp / remotePort)** — future; Q4 scoped `:connection()` to SSL accessor per upstream Envoy v1.37.2; the per-connection address surface lives on `:streamInfo()`.
9. **Cross-phase dynamic-metadata deferral lifts** — future phases that consume dynamic-metadata reuse `internal/dynamicmetadata/` from 22.2 per §1.6; each prior-phase BEHAVIOR_CONTRACT.md "deferred" note converts to "lifted via `internal/dynamicmetadata`" at the lift-phase's next-touchpoint.
10. **gopher-lua-vs-LuaJIT body / trailers / coroutine observable divergences** — 22.2 SPEC §11 empirical-pin scrape (mirrors 22.1 SPEC §11 scrape pattern); divergences likely surface at IMPL.

---

## 9. Cross-references against parent BRAINSTORM Q-decisions + 22.1 IMPL forward-pointers — closure pickup

### 9.1 Parent BRAINSTORM Q-decisions inherited

| Parent Q | Decision | 22.2 disposition |
|---|---|---|
| Q1 (envelope D) | Full upstream parity by phase-22 phase-done | 22.2 takes envelope D to its conclusion (per §1.1) |
| Q2 (3-way pre-split) | 22.1 + 22.2 + 22.3 | 22.2 is the second sub-phase; 22.3 unchanged (per §4) |
| Q3 (gopher-lua v1.1.2) | Pure-Go Lua 5.1 | UNCHANGED at 22.2 (no library bump) |
| Q4 (EXTRACT-NOW `internal/lua/`) | NEW framework primitive at 22.1 first-consumer + EXPLICIT API-REVISION ALLOWANCE for consumer-#2 | 22.2 EXTENDS via NEW ADR-0191 per Q10 strict scope (ALLOWANCE stays scoped to consumer-#2; per §2.10) |
| Q5 (4-arm DataSource) | 4 arms (Filename + InlineBytes + InlineString + EnvironmentVariable) + WatchedDirectory PARSE-REJECT | UNCHANGED at 22.2 (no DataSource extensions; `:fileBytes()` is a runtime call, not a DataSource arm) |
| Q6 (pragmatic-middle bridge at 22.1) | Pragmatic-middle 22.1; full delta at 22.2 | 22.2 lands the full delta per §2.1-§2.8 |
| Q7 (NEW 9th canonical per-route) | LuaPerRoute 3-arm hybrid | UNCHANGED at 22.2 (lands at 22.3) |
| Q8 (3-counter stat surface at 22.1) | errors + executions + respond_calls | 22.2 adds 5-6 envoy-go-strict counters (per §5.1) |
| Q9 (full cross-side fixture at 22.1) | Full cross-side byte-exact | 22.2 uses mixed-mode fixture-0027 per Q12 (per §6.1) |
| Q10 (WEAK HOLD escape-valve at 22.1) | 2 anticipated + 0-1 escape-valve | 22.2 uses WEAK HOLD again per Q13 (per §2.13) |
| Q11 (long-prefix slug naming) | `22.2-http-filter-lua-full-bridge` | UNCHANGED |
| Q12 (pre-create all sub-phase dirs) | All 4 dirs created at parent BRAINSTORM | 22.2 directory exists (placeholder README → THIS BRAINSTORM at this commit) |

### 9.2 22.1 IMPL forward-pointers picked up

Per BEHAVIOR_CONTRACT.md `### Phase 22.1 forward-pointer notes` + 22.1 SPEC §15.2 acceptance items 19-24:

- **`*LState`-pool design (ADR-0190 escape-valve)** — 22.1 D-P10 R6 STANDS WEAK-default at ~70µs/stream. 22.2 carries forward as the R6 *LState-pool gate at the 22.2 IMPL benchmark Task (per §2.13 escape-valve hypothesis (a)). Note: ADR-0190 at 22.1 was reserved as escape-valve slot; at 22.2 it consumes for the NEW `internal/dynamicmetadata/` primitive per Q9 + §2.3 — the escape-valve slot moves to ADR-0193 conditional per §7.5.
- **AMEND-9 gopher-lua-vs-LuaJIT divergence catalogue** — 22.2 SPEC §11 empirical-pin scrape covers body / trailers / coroutine / crypto wire-output divergences. 22.2 SPEC anticipates an `internal/lua/FormatNumber(v) string` helper landing per AMEND-9 forward-pointer (for the `tostring(float)` + `string.format("%d", float)` divergences that 22.2's body bridge may hit on wire-output of body-rewriting scripts).
- **22.2 BRAINSTORM scope hand-off bullets** — THIS BRAINSTORM is the hand-off destination; all bullets settled in §2.

### 9.3 22.3 BRAINSTORM scope hand-off (NEW forward-pointers from 22.2)

For the future 22.3 BRAINSTORM session per parent BRAINSTORM Q2 + Q7:

- **22.2 BREAKS the cross-phase dynamic-metadata deferral discipline** per §1.6 — 22.3 BRAINSTORM should NOT defer dynamic-metadata anew (the primitive exists at 22.2 phase-done; 22.3's per-route surfaces inherit the cross-filter visibility).
- **`internal/lua/` 22.2 extensions (ADR-0191)** are available at 22.3 — coroutine yield/resume + body-bridge buffer seam usable by 22.3's per-route surfaces if needed (likely not needed since per-route is mostly dispatch-table mutation, not body-touching).
- **5-6 NEW envoy-go-strict departure records** at BEHAVIOR_CONTRACT.md from 22.2 — 22.3 BRAINSTORM should align any new per-route stat surface with the 22.2 envoy-go-strict pattern.
- **Fixture-0027 mixed-mode pattern** at 22.2 — 22.3 BRAINSTORM may use the same pattern at fixture-0028 for per-route multi-tier dispatch scenarios.
- **`:metadata()` empty-table at binding-gap** at 22.2 — 22.3 LuaPerRoute PARSE-LIFT does NOT activate `:metadata()` data (v1.32.4 binding-gap remains until v1.37.x bump phase).

---

## 10. BRAINSTORM-time open questions for 22.2 SPEC-time resolution (D1-D7)

### D1 (per §2.3 + AMEND-12): `:metadata()` empty-table vs nil disposition at v1.32.4 binding-gap

Upstream Envoy `:metadata()` returns nil when no per-route typed config metadata is available. envoy-go's bridge could return either nil OR an empty Lua table; gopher-lua-vs-LuaJIT nil-vs-empty-table edge cases (e.g., `next(x)` returns different values) may surface. 22.2 SPEC §11 empirical scrape resolves: pick the disposition that matches upstream Envoy's gopher-lua-equivalent semantics for downstream-script compat.

### D2 (per §2.1 + §3.2): coroutine yield/resume capture mechanism

gopher-lua has native coroutine support via `lua.LState.Yield()` + `lua.LState.Resume()`. envoy-go's bridge can either use these directly (in-Lua-coroutine yield) OR wrap them in a Go-side scheduling layer (Go-channel-based yield + resume). 22.2 SPEC §11 empirical scrape decides: gopher-lua native coroutine semantics + scheduling-layer-shape that integrates with the per-stream goroutine dispatch model.

### D3 (per §2.1 + §3.2): body-buffer zero-copy lifetime guarantees

Lua's string GC + ADR-0128's buffer GC interplay: when a Lua script holds a string returned by `:body()`, the underlying ADR-0128 buffer must NOT be freed until the Lua string is GC'd. 22.2 SPEC settles: either (a) per-stream lifetime — buffer freed at OnDestroy after all Lua strings are released; (b) explicit copy at bridge time for safety-by-default; (c) gopher-lua-string-wrapper-GC interception via `*lua.LUserData` for Lua-side GC notification.

### D4 (per §2.8 + §3.4): `:filterState()` shape — typed (matches upstream) vs string-keyed Lua-table-only (envoy-go-strict cut)

Upstream Envoy `:filterState()` supports typed objects (proto3 messages stored as filter state); envoy-go's IN-PACKAGE filter-state at 22.2 may cut to string-keyed Lua-table-only (operators serialize objects to JSON/string for cross-filter passing). 22.2 SPEC §11 empirical scrape decides: gopher-lua-LUserData typed support vs string-keyed simplicity.

### D5 (per §2.4 + §6.2 scenario (f)): cross-side byte-exactness scope for `:connection():ssl()`

Cross-side byte-exact requires matching TLS certs on both reference Envoy + envoy-go sides (same SAN list + same expiration + same issuer + etc.). Operationally complex to set up — both sides must have identical cert files. 22.2 SPEC §11 empirical scrape decides: either (a) full cert-matching cross-side (high setup cost); (b) cert-fingerprint-only cross-side (compare sha256 digests only; allow other fields to differ); (c) drop fixture-0027 scenario (f) cross-side to REFERENCE-LESS subject-only (lose envelope D verification for SSL methods).

### D6 (per §2.5 + §6.2 scenarios (j)+(k)): httpCall async-flag semantics

Upstream's `asynchronous=true` flag is fire-and-forget — script returns nil immediately; HTTP call is dispatched and response is discarded. envoy-go's implementation could either: (a) match upstream exactly (no caller-suspension; goroutine-dispatch + discard); (b) caller-suspends-until-dispatch-confirmed (returns when the request is on the wire, not when the response arrives — gives operators dispatch-confirmation). 22.2 SPEC §11 empirical scrape decides per upstream Envoy v1.37.2 wire semantics.

### D7 (per parent BRAINSTORM §3.7 + 22.1 D5 closure): 28th + 29th project-wide fuzzer claim verification

22.2 anticipates 1-2 new fuzzers: `FuzzLuaBodyBridge` (fuzzes body-bridge against gopher-lua's coroutine state machine for panics) + `FuzzLuaHTTPCallConfig` (fuzzes httpCall config parameters at PARSE time). 22.2 SPEC §11.4 verifies the project-wide fuzzer count post-22.2 = 28 + N (where N = 1 or 2) via `grep -h '^func Fuzz' | sort -u | wc -l`. 22.2 IMPL Task 11-equivalent fuzzer task pins the count.

---

## 11. Phase-21 + parent-22 BRAINSTORM lessons applied

- **Phase-21 EXTRACT-NOW-only-when-trigger-fires lesson** — applied at Q9 + §3.4: filter-state stays IN-PACKAGE at 22.2 (no second consumer yet); body-buffer seam stays IN-PACKAGE; crypto stays IN-PACKAGE; connection-SSL stays IN-PACKAGE. Only `internal/dynamicmetadata/` extracts (clear multi-consumer story from §1.6 cross-phase deferral discipline).
- **Phase-21 STRONG-HOLD-too-aggressive-for-first-of-kind lesson** — applied at Q13: WEAK HOLD chosen (matches 22.1's WEAK HOLD precedent; first-of-kind primitive extension at consumer-#1-scope-expansion).
- **Parent 22 BRAINSTORM Q4 EXPLICIT API-REVISION ALLOWANCE** — applied at Q10: ALLOWANCE stays scoped to consumer-#2; 22.2 consumer-#1-scope-expansion gets NEW ADR-0191 instead of in-place AMEND on ADR-0188.
- **Parent 22 BRAINSTORM Q10 WEAK HOLD ZERO-slot buffer post-22.1-IMPL** — applied at §1.5 + §2.13: 22.1 IMPL phase-done re-evaluation surfaces ADR-0190 unconsumed (R6 STANDS); 22.2 BRAINSTORM consumes ADR-0190 for dynamicmetadata primitive per Q9; the escape-valve slot moves to ADR-0193 conditional.
- **Parent 22 BRAINSTORM Q6 deferred-method Lua-runtime-error disposition** — applied at §1.2: 22.2 deferred items (LuaPerRoute / SourceCodes / metadata-source-at-binding-gap) keep raising Lua runtime errors until their respective landing phases.
- **Phase-17 → Phase-18 ADR-0149 → ADR-0150 AMEND precedent** — applied at §2.5 + §3.3: in-place AMENDMENT on ADR-0177 for httpclient cluster-based dispatch (no new ADR number consumed).
- **Phase-09 → Phase-11 + Phase-13 split-at-PLAN precedent** — applied at Q14: 22.2 stays single-phase at BRAINSTORM; PLAN session decides if split per ADR-0045 gate.
- **22.1 SPEC §11 empirical-pin pattern** — applied at §10 D1-D7: 22.2 SPEC §11 will do similar empirical scrapes for the 22.2-specific surfaces (gopher-lua coroutine semantics + body-bridge-buffer lifetime + connection-SSL wire-format + httpCall async semantics + fuzzer count verification).
- **22.1 Task 11 fuzzer cap discipline** — applied at §2.7: `:fileBytes()` inherits the 16 MiB cap pattern from 22.1's `DataSource.Filename` cap (defense vs `/dev/full`-class infinite-read OOM-kill).

---

## 12. Section closeout

Phase 22.2 BRAINSTORM settles 14 Q-decisions across body / trailers / metadata / connection / httpCall / crypto / fileBytes + timestamp / streamInfo-full / extraction-posture / ADR-0188-allowance / stat / fixture / D-hypothesis / scope-shape. Outcomes:

- Maximal envelope D scope per parent BRAINSTORM Q1: every upstream-parity bridge method active by 22.2 phase-done.
- 2 intentional envoy-go-strict scope-expansions: dynamic-metadata pulled forward from cross-phase deferral discipline (NEW `internal/dynamicmetadata/` primitive); filter-state in-package at lua filter ahead of any cross-filter primitive extraction.
- 1 NEW framework primitive (`internal/dynamicmetadata/`) + 2 NEW ADRs for primitive-level extensions (ADR-0191 `internal/lua/` API + ADR-0192 `internal/filter/http/lua/` package shape) + 1 in-place AMEND on ADR-0177 (httpclient cluster dispatch) + 1 conditional ADR-0193 (escape-valve slot).
- 5-6 NEW envoy-go-strict counters (102 → ~108) + 5-6 BEHAVIOR_CONTRACT.md departure records.
- 1 NEW differential fixture (`0027-http-lua-full-bridge`) in single mixed-mode pattern (28 → 29 fixture directories).
- 1-2 NEW project-wide fuzzers (`FuzzLuaBodyBridge` + `FuzzLuaHTTPCallConfig`; 28 → 29-30).
- 7 D-questions carry forward to 22.2 SPEC.
- 22.3 scope UNCHANGED (LuaPerRoute parsing + SourceCodes activation + 9th canonical ADR + per-route 3-tier dispatch); 22.3 BRAINSTORM inherits 22.2's cross-phase deferral-break + the `internal/lua/` 22.2 extensions.

**Next-skill:** `superpowers:brainstorming` (scoped to 22.2 SPEC per SKILL_ROUTING state-1 entry "Phase in ROADMAP, directory does not exist → create + superpowers:brainstorming scoped to THIS phase → output: SPEC.md") — the next session authors `docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/SPEC.md`.

**Squash-merge handoff:** this BRAINSTORM session lands all 22.2 BRAINSTORM artefacts (NEW BRAINSTORM.md + STATE.md lifecycle 0→1 update + ROADMAP row 22.2 planned→in-progress update) atomically via one squash-merge commit per project memory `feedback_git_worktrees.md` + ADR-0005 §Decision 4. Post-squash SHA-fill follow-up commit per the phase-09..21 convention.

**No ADR consumption at this BRAINSTORM commit.** ADR-0190 stays next-free; ADR-0188 + ADR-0189 + ADR-0125 §(xiv) UNCHANGED. ADR consumption happens at 22.2 IMPL atomic-landing Task per Q9 + Q10 + Q13.
