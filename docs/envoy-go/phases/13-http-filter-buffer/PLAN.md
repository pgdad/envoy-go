# Phase 13 — HTTP filter `envoy.filters.http.buffer` (`internal/filter/http/buffer/`, differential fixture `0015-http-buffer`, `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.buffer` extension) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per project memory user preference) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land `envoy.filters.http.buffer` — Envoy v1.37.2's canonical "buffer entire request body before forwarding upstream" filter — as the SIXTH production HTTP filter in envoy-go, with byte-equivalent wire outcomes against reference Envoy on every observable axis (status / body / headers / counter increments / 100-Continue addendum / `maybeAddContentLength` chunked → fixed-CL conversion at the upstream boundary), under the 07.1 framework, with ZERO new framework primitives.

**Architecture:** New `internal/filter/http/buffer/` package owning the filter implementation; decoder-only `HTTPFilter` value (mirrors phase 12 csrf ADR-0120); body-counting algorithm STREAMING-CAP-ONLY per SPEC §11.6 (NO Content-Length fast-fail); `DecodeHeaders` returns `StopIteration` on bodied + non-disabled requests; `DecodeData` accumulates via `DataStopIterationAndBuffer`, fires `SendLocalReply(413, "Payload Too Large", connClose)` + `DataStopIterationNoBuffer` on overflow, invokes `maybeAddContentLength` on terminal `endStream=true` to mirror `buffer_filter.cc:91-97` (chunked → fixed-CL conversion observable at upstream); per-route `BufferPerRoute` proto carries a `oneof override` with `disabled: true` shortcut OR `buffer.max_request_bytes` wholesale override (5th canonical per-route discipline per ADR-0125); envoy-go-only `max_request_bytes ≤ 1 MiB` parse-time validation (per ADR-0126) keeps the buffer-filter cap inside envoy-go's framework safety net (ADR-0076 `filterBufferLimitBytes = 1 << 20`); ZERO new stat-table entries — buffer-overflow observable on envoy-go side via the existing in-table `downstream_rq_4xx` HCM counter (per phase 06.1 Rule SN4); Envoy-only `downstream_rq_too_large` + `downstream_rq_completed` filtered out via the existing twin-series-discipline allow-list.

**Tech Stack:** Go 1.26.2; `go-control-plane` v1.32.4 module (proto pin per ADR-0008); `protojson.Unmarshal` for `BufferPerRoute` oneof discipline; reference Envoy `envoyproxy/envoy:v1.37.2` SHA `c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (per ADR-0008 + ENVOY_TARGET.md); golangci-lint 1.64.8 (ADR-0009 pin); Docker for differential harness; HTTP/1.1 plaintext fixture (no H2 differential coverage in phase 13).

---

## Scope check — why phase 13 ships as one row (not split)

Net change estimate (mirroring the 06.1 / 06.2 / 07.1 / 07.2 / 08.1 / 08.2 / 09 / 10 / 11 / 12 PLAN's component-table convention):

- `internal/filter/http/buffer/doc.go` ~25
- `internal/filter/http/buffer/buffer.go` ~280–330 (filter + factory + `compiledConfig` + `compiledPerRoute` + `parsePerRoute` + `DecodeHeaders` + `DecodeData` + `maybeAddContentLength` + `DecodeTrailers` + helpers)
- `internal/filter/http/buffer/buffer_test.go` ~400–500 (6 test groups per SPEC §14.1)
- `internal/filter/http/buffer/fuzz_test.go` ~60 (17th fuzzer in repo)
- `cmd/envoy-go/main.go` one new `httpReg.Register(buffer.TypeURL, buffer.New)` line + matching import ~+3
- `test/fixtures/0015-http-buffer/` (NEW directory) — `envoy.yaml` ~70 + `envoy-go.yaml` ~70 + `expectations.yaml` ~30 + `README.md` ~50 + `driver/driver.go` ~180 + `backends/backend.go` ~50 = ~450
- `test/differential/fixture/fixture.go` new `BackendKind` enum value (`HTTPBuffer BackendKind = 12`) + doc-comment ~+15
- `test/differential/runner_test.go` blank-import addition + new `startHTTPBufferBackend` spawn helper + switch case ~+25
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` per SPEC §13 patches — §13.1 `### envoy.filters.http.buffer` subsection ~70 + §13.2 29-name table preamble note ~5 + §13.3 equivalence-matrix row ~3 + §13.4 `### Phase 13 forward-pointer notes` subsection ~25 = ~+103
- `docs/envoy-go/DECISIONS.md` (3 ADRs ADR-0125, ADR-0126, ADR-0127 v2) ~+200
- `docs/envoy-go/ROADMAP.md` row `13` `in-progress → done` flip + (UNCHANGED) §9 family heading at line 56 ~+1 net
- `docs/envoy-go/STATE.md` advance per `BOOTSTRAP_PROMPT.md` §5 lifecycle ~rewrite-in-place
- `docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md` (NEW; lifecycle artefact) ~450
- `docs/envoy-go/phases/13-http-filter-buffer/REVIEW.md` (NEW; lifecycle artefact) ~150

**Production code: ~280–330 LoC (filter impl in `buffer.go`) + ~3 LoC main.go = ~283–333 LoC production + ~460 LoC tests (400-500 unit + 60 fuzzer) + ~450 LoC fixture YAML/Go + ~334 LoC docs ≈ ~1530-1580 LoC total** (production-only ~283-333 LoC, well below the ADR-0045 ~1500 LoC threshold). Both ADR-0045 thresholds — ~25 tasks AND ~1500 LoC of production code — are well under (production ~283-333 LoC; task count below is **13**, comfortably under the 25 limit). The SPEC's anticipated 3-ADR cluster (ADR-0125, ADR-0126, ADR-0127 v2) lands across 13 tasks per the table at `## ADRs introduced by this plan` below; no task lands more than 2 ADRs simultaneously. SPEC §1.3 (per BRAINSTORM Decision 9 + ADR-0106) settled the family-expansion shape as flat top-level rows; phase 13 is a SINGLE coherent row, no parent-and-sub-phases split. STATE.md `next-skill-scope` projected ~10–14 tasks; this PLAN lands at **13 tasks** (mid-bound — driven by buffer's body-counting state machine which is structurally heavier than csrf at the algorithm level (DecodeData accumulation + maybeAddContentLength) but lighter at the proto-surface level (1 listener field vs csrf's 3) and at the stat-surface level (0 filter-specific counters vs csrf's 3) and at the ADR roster (3 vs csrf's 5)).

The natural ADR-0045 release-valve split per BRAINSTORM §1.4 / SPEC §1.4 would be `13.1 = listener-level filter MVP (Tasks 1–7)` and `13.2 = per-route disabled-OR-override TPFC + per-route fixture scenarios (Tasks 8–13)`; SPEC §1.4 explicitly rejects the split since both halves stay well under the LoC threshold and the per-route discipline is a small extension of the listener-level work (data-only + shared-vacuous stats — no new stateful resources). PLAN concurs and ships single-row.

---

## File structure (decomposition decisions locked in here)

| File | Status | Responsibility |
|---|---|---|
| `internal/filter/http/buffer/doc.go` | NEW | Package doc enumerating: (a) the typed_config surface (`Buffer` proto with 1-actively-consumed (`max_request_bytes` UInt32Value REQUIRED with envoy-go-own validation: non-nil + value > 0 + value ≤ 1048576 / 1 MiB per ADR-0126); 0-deferred at the listener level — `Buffer` proto has only the one top-level field; the per-route `BufferPerRoute` proto carries a separate `oneof override` with `disabled` boolean OR nested `buffer` message — both cases honored); (b) the public API surface (`TypeURL` const, `New` HTTPFilterFactory); (c) the iteration-protocol coverage (`DecodeHeaders` Continue allow path on header-only / per-route-disabled; `DecodeHeaders` StopIteration on bodied + non-disabled per ADR-0127 v2; `DecodeData` accumulation via `DataStopIterationAndBuffer` + `DataContinue` on terminal-fits + `DataStopIterationNoBuffer` on overflow; `SendLocalReply(413, "Payload Too Large", connClose)` reuse from phase 09 fault precedent + ADR-0076 framework synthesis; no async-resume; no encode-side state — `HTTPFilter` value sets `Encoder: nil` per planner-time decision 5 + phase 12 csrf ADR-0120 precedent); (d) the per-route discipline (per ADR-0073 wholesale-override + ADR-0125's 5th canonical "disabled-OR-override" sum-type shape — separate `BufferPerRoute` proto from listener-level `Buffer`; per-route TPFC entry → independent `*compiledPerRoute{disabled, maxOverride}` value; SHARED-vacuous stats — phase 13 emits no filter-specific counters per SPEC §1.1 amendment 5); (e) the body-counting algorithm — STREAMING-CAP ONLY per SPEC §11.6 + ADR-0127 v2 (NO Content-Length fast-fail; mirrors `buffer_filter.cc:50-67`); + the `maybeAddContentLength` mirror per `buffer_filter.cc:91-97` (chunked → fixed-CL conversion observable at upstream); (f) the cap-layering rationale — buffer's `accumulated > effectiveMax` check fires INSIDE the framework's `filterBufferLimitBytes = 1 << 20` cap (per `internal/filter/http/chain.go:19`) because `effectiveMax ≤ 1 MiB` invariant per ADR-0126's parse-time validation; (g) the cross-cutting ADR anchors (ADR-0125 / ADR-0126 / ADR-0127 v2). Mirrors `internal/filter/http/csrf/doc.go`-style brevity (~25 LoC precedent). Per SPEC §4.1. |
| `internal/filter/http/buffer/buffer.go` | NEW | Filter implementation — single-file orchestration per planner-time decision 6. **Public surface (per SPEC §6.1):** `TypeURL` string constant (`"type.googleapis.com/envoy.extensions.filters.http.buffer.v3.Buffer"`); `New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error)` factory matching `envoyhttp.HTTPFilterFactory`. **Unexported types (per SPEC §6.2):** `compiledConfig` struct (1 field per §6.2: `maxRequestBytes uint32` — validated 0 < value ≤ 1048576); `compiledPerRoute` struct (2 fields per §6.2: `disabled bool` — exclusive with maxOverride; true → filter wholly inactive; `maxOverride *uint32` — exclusive with disabled; pointer to discriminate from "unset"); `filter` struct (5 fields: `config *compiledConfig` (closure-captured listener-level reference) + `dcb envoyhttp.DecoderFilterCallbacks` + `effectiveMax uint32` (per-stream resolved cap) + `accumulated uint32` (per-stream byte counter) + `passthrough bool` (per-stream disabled flag) + `headersRef envoyhttp.RequestHeaderMap` (held headers reference for `maybeAddContentLength`)). **Helpers:** `parsePerRoute(perRoute proto.Message) (*compiledPerRoute, error)` (oneof discipline per §6.3 + §11.3 — handles `*BufferPerRoute_Disabled`, `*BufferPerRoute_Buffer`, nil PGV-required mirror, defensively `disabled: false` PGV bool.const mirror); `(f *filter) resolveEffective(headers envoyhttp.RequestHeaderMap) (effectiveMax uint32, disabled bool)` (calls `f.dcb.RequestRouteConfig().Resolve("envoy.filters.http.buffer", routeIdx)`; nil → listener fallback; type-assert to `*compiledPerRoute`; disabled=true → `(0, true)`; maxOverride!=nil → `(*maxOverride, false)`); `(f *filter) maybeAddContentLength()` (mirrors `buffer_filter.cc:91-97` per §6.5 — if `headersRef != nil` AND `headersRef.Get("content-length") == ""` → `headersRef.Set("content-length", fmt.Sprintf("%d", accumulated))` AND `headersRef.Remove("transfer-encoding")` per §11.8-CL empirical evidence). **DecodeHeaders body** (per SPEC §6.4 + §11.6 + §11.10): step 1 `endStream=true` → `HeadersContinue` (header-only fast-path; mirrors `buffer_filter.cc:54-56`; no per-route resolve; no state touch); step 2 resolve `(effectiveMax, disabled)` via `resolveEffective(headers)`; step 3 `disabled=true` → set `f.passthrough = true`; return `HeadersContinue` (mirrors `buffer_filter.cc:60-62`); step 4 bodied + non-disabled → `f.effectiveMax = effectiveMax`; `f.headersRef = headers`; return `HeadersStopIteration` (mirrors `buffer_filter.cc:67`); **NO Content-Length inspection per §11.6** (the streaming-cap path handles all overflow cases). **DecodeData body** (per SPEC §6.5 + §11.2 + §11.7 + §11.9): step 1 `f.passthrough=true` → `DataContinue` (filter never returns `DataStopIterationAndBuffer` on this path; framework safety-net cap never engages on disabled routes); step 2 `f.accumulated += uint32(data.Len())`; step 3 `f.accumulated > f.effectiveMax` (strict `>` per §11.2) → `f.dcb.SendLocalReply(413, []byte("Payload Too Large"), envoyhttp.OrderedHeaders{{Name: "Connection", Value: "close"}})`; return `DataStopIterationNoBuffer` (discards partial buffer); step 4 `endStream=true` (terminal chunk fits) → invoke `maybeAddContentLength()`; return `DataContinue`; step 5 in-flight chunk → `DataStopIterationAndBuffer` (accumulate; framework holds bytes per ADR-0076 §Decision (b)). **DecodeTrailers body** (per §6.6): defensively invoke `maybeAddContentLength()`; return `TrailersContinue`. **Pass-through methods (per §6.7):** `SetDecoderCallbacks(cb)` stores `f.dcb = cb`; `OnDestroy()` no-op. **NO encode-side methods** — the `HTTPFilter` value returned by the factory sets `Decoder: f, Encoder: nil` per planner-time decision 5. Per SPEC §6.1–§6.8. ~280-330 LoC. |
| `internal/filter/http/buffer/buffer_test.go` | NEW | Unit tests per SPEC §14.1 covering 6 test groups: **Group 1 — `New` factory PGV-mirror (per §6.1 + §11.1):** `TestNew_NilTC`, `TestNew_MalformedTC`, `TestNew_MaxRequestBytesNil_RejectAtParseTime`, `TestNew_MaxRequestBytesZero_RejectAtParseTime`, `TestNew_MaxRequestBytesOverCap_RejectAtParseTime` (`5MiB`, `2MiB`, `1MiB+1`), `TestNew_MaxRequestBytesBoundary_Accepted` (`1`, `1MiB` exact, `64KiB` mid-range), envoy-go-own error wording per planner-time decision 4 (D3 settlement). **Group 2 — `parsePerRoute` PGV-mirror discipline (per §6.3 + §11.3):** `TestParsePerRoute_Disabled_Parses`, `TestParsePerRoute_BufferOverride_Parses` (`maxOverride=&65536`), `TestParsePerRoute_BufferOverride_Zero_Rejects`, `TestParsePerRoute_BufferOverride_OverCap_Rejects` (5MiB), `TestParsePerRoute_OneofUnset_Rejects` (PGV-required mirror; "override oneof is required"), `TestParsePerRoute_DisabledFalse_Rejects` (PGV bool.const mirror; "disabled must be true"; structurally unreachable post-decode but defensively covered), `TestParsePerRoute_WrongType_Rejects` (Go-side type assertion guard). **Group 3 — `DecodeHeaders` (per §6.4 + §11.6 + §11.10):** `TestDecodeHeaders_HeaderOnlyEndStream_Continue` (parametrized over `GET`, `HEAD`, `OPTIONS`, plus a bodied `POST` with `endStream=true` on headers), `TestDecodeHeaders_PerRouteDisabled_Continue_PassthroughSet`, `TestDecodeHeaders_BodiedNonDisabled_StopIteration_EffectiveMaxStored` (listener fallback path), `TestDecodeHeaders_BodiedPerRouteOverride_StopIteration_OverrideMaxStored`, `TestDecodeHeaders_DoesNotInspectContentLength` (§11.6 — even with `Content-Length: 99GB` header on bodied request, the filter returns `StopIteration` rather than `SendLocalReply`; no fast-fail). **Group 4 — `DecodeData` accumulation + cap predicate (per §6.5 + §11.2 + §11.9):** `TestDecodeData_PassthroughFlag_DataContinue` (per chunk regardless of accumulation), `TestDecodeData_SingleChunkFits_EndStream_DataContinue` (`accumulated < effectiveMax`, terminal), `TestDecodeData_SingleChunkExactCap_EndStream_DataContinue` (`accumulated == effectiveMax`; predicate is `>` strict not `>=` — verifies §11.2 boundary), `TestDecodeData_SingleChunkOverflow_413_StopIterationNoBuffer` (`accumulated > effectiveMax`; verifies SendLocalReply args byte-exact: status 413, body `Payload Too Large` 17 bytes, `Connection: close` header), `TestDecodeData_MultiChunkBelowCap_StopIterationAndBuffer_TerminalContinue` (chunks A, B with A+B<cap, terminal `endStream=true` releases via DataContinue), `TestDecodeData_MultiChunkOverflowMidStream_413` (chunk N pushes accumulated past cap), `TestDecodeData_EmptyTerminalChunk_DataContinue` (`endStream=true`, `len(data)==0` — exercises §11.11 empty-body POST path). **Group 5 — `maybeAddContentLength` mirror (per §6.5 + §11.8-CL):** `TestMaybeAddContentLength_NoOriginalCL_InjectsCL_DropsTransferEncoding`, `TestMaybeAddContentLength_OriginalCLPresent_NoOp`, `TestMaybeAddContentLength_HeadersRefNil_NoOp` (e.g., header-only or disabled paths where headersRef wasn't stored), `TestMaybeAddContentLength_Idempotent` (called twice → same result; no double-injection). **Group 6 — Per-route integration (per §5.6 + §11.4):** `TestPerRoute_ListenerFallback_AppliesWhenPerRouteNil`, `TestPerRoute_OverrideSmaller_FiresAtSmallerCap`, `TestPerRoute_OverrideLarger_FiresAtLargerCap` (within 1 MiB; verifies override truly overrides listener-level), `TestPerRoute_DisabledBypassesCap` (passthrough flag set; no `DataStopIterationAndBuffer` ever returned even with arbitrarily large body; framework safety-net structurally never engages), `TestPerRoute_ResolveCalledOncePerStream` (spy on callback; verifies per-stream not per-chunk resolve). ~400-500 LoC total. |
| `internal/filter/http/buffer/fuzz_test.go` | NEW | `FuzzBufferConfigParse` — fuzzes arbitrary byte sequences as the `tc *anypb.Any` parameter to `New`. Asserts: `New` returns either `(factory, nil)` OR `(nil, error)`; never panics; never returns `(nil, nil)`. Per ADR-0018's "every parser/codec/filter ships a fuzzer" + the buffer filter's `New` factory is a parser. ~60 LoC; 30s budget per ADR-0018; **seventeenth fuzzer overall** (post-12's sixteenth `FuzzCsrfPolicyConfigParse`). Seed corpus: `max_request_bytes = 1024` (small valid), `= 1048576` (max valid), `= 0` (rejected by validation), `= 5242880` (rejected by ≤ 1 MiB validation), empty bytes (Unmarshal failure), malformed proto bytes (Unmarshal failure). |
| `cmd/envoy-go/main.go` | MODIFIED | NEW one-line `httpReg.Register(buffer.TypeURL, buffer.New)` registration inserted IMMEDIATELY AFTER the existing `httpReg.Register(router.TypeURL, router.New)` line at `cmd/envoy-go/main.go:115` (and BEFORE the existing `httpReg.Register(cors.TypeURL, cors.New)` line at `cmd/envoy-go/main.go:116`). Plus the matching `import "github.com/esalaine/envoy-go/internal/filter/http/buffer"` alphabetically among the existing filter-package imports (currently `cmd/envoy-go/main.go:28-34`: `cors, csrf, envoygotest, fault, header_mutation, localratelimit, router` → `buffer, cors, csrf, envoygotest, fault, header_mutation, localratelimit, router`). Per the BRAINSTORM Decision 2's "router-first-then-alphabetical" stylistic discipline (codified at phase-09 brainstorm time + reaffirmed at phases 10 + 11 + 12), the resulting block reads: `httpReg.Register(router.TypeURL, router.New); httpReg.Register(buffer.TypeURL, buffer.New); httpReg.Register(cors.TypeURL, cors.New); httpReg.Register(csrf.TypeURL, csrf.New); httpReg.Register(envoygotest.TypeURL, envoygotest.New); httpReg.Register(fault.TypeURL, fault.New); httpReg.Register(header_mutation.TypeURL, header_mutation.New); httpReg.Register(localratelimit.TypeURL, localratelimit.New); header_mutation.RegisterPerRouteValidator(httpReg); httpReg.Freeze()`. **No other wiring changes** — buffer is HTTP-only, no listener/cluster/drain manager threading; no per-route-validator registration call (buffer's per-route TPFC parsing happens at HCM-build via `BuildPerRouteConfig`'s generic `UnmarshalNew`, and the filter applies its PGV-mirror validation in `parsePerRoute` for per-route entries — same discipline as csrf phase 12). ~+3 LoC delta (1 import line + 1 register line). Per SPEC §4.3. |
| `test/fixtures/0015-http-buffer/` | NEW DIRECTORY | Fixture root carrying `envoy.yaml`, `envoy-go.yaml`, `expectations.yaml`, `README.md`, `driver/driver.go`, `backends/backend.go` per SPEC §7. The runner-side blank-import lives at `test/differential/runner_test.go` per the existing 0010 / 0011 / 0012 / 0013 / 0014 convention. |
| `test/fixtures/0015-http-buffer/envoy.yaml` | NEW | Reference Envoy bootstrap (admin port resolved at boot by the runner; **ONE listener `l_main` per planner-time decision 7** — single listener with three routes (`/route-disabled` per-route TPFC `disabled: true`; `/route-tighter` per-route TPFC `buffer: {max_request_bytes: 131072}` (128 KiB); `/` default uses listener-level Buffer); cluster `c_backend` STRICT_DNS pointing at the harness backend via `host.docker.internal` per ADR-0010). Listener-level `Buffer`: `max_request_bytes: 1048576` (1 MiB; the envoy-go cap per ADR-0126). http_filters chain on the listener: `[envoy.filters.http.buffer, envoy.filters.http.router]`. ~70 LoC. Per SPEC §7.2. |
| `test/fixtures/0015-http-buffer/envoy-go.yaml` | NEW | Subject envoy-go bootstrap. Identical to `envoy.yaml` modulo cluster type (STATIC instead of STRICT_DNS) + admin/listener port values resolved at boot by the runner. Both `max_request_bytes: 1048576` listener-level + `max_request_bytes: 131072` per-route-tighter values are PRESENT in envoy-go.yaml (Envoy and envoy-go both accept these values; the divergence-window is at `max_request_bytes > 1 MiB` which the fixture does NOT exercise per scope rationale at SPEC §2.1.1). ~70 LoC. Per SPEC §7.2. |
| `test/fixtures/0015-http-buffer/expectations.yaml` | NEW | Prose narrative of the per-scenario equivalence claims (per ADR-0019 — expectations.yaml is prose, not machine-evaluated; the runner enforces via the driver's per-scenario assertions). Documents per SPEC §7.1: scenario 1 (body fits within listener cap, `POST /` 1 KiB) → 200 + backend body passthrough; counter delta `downstream_rq_total +1`, `downstream_rq_2xx +1`; scenario 2 (streaming overflow with CL-known body, `POST /` 2 MiB) → 100-Continue + 413 + body byte-exact `Payload Too Large` (17 bytes, no LF) + 4-header set lowercase wire-form (`content-length: 17`, `content-type: text/plain`, `date: <allow-listed>`, `server: envoy`) + `Connection: close`; counter delta `downstream_rq_total +1`, `downstream_rq_4xx +1`; scenario 3 (chunked overflow against per-route tighter cap, `POST /route-tighter` chunked ~200 KiB) → 413 (NO 100-Continue with chunked) + same 4-header set + `Payload Too Large`; counter delta `downstream_rq_total +1`, `downstream_rq_4xx +1`; scenario 4 (per-route disabled bypasses cap, `POST /route-disabled` 2 MiB) → 200 + backend echo (filter wholly inactive on disabled route per §11.4); counter delta `downstream_rq_total +1`, `downstream_rq_2xx +1`; scenario 5 (per-route tighter override fires CL-known, `POST /route-tighter` 200 KiB) → 100-Continue + 413 + same 4-header set + `Payload Too Large`; counter delta `downstream_rq_total +1`, `downstream_rq_4xx +1`; scenario 6 (chunked-passthrough Content-Length injection cross-cutting, `POST /` chunked 10 KiB body) → 200 + backend echo; **backend asserts inbound request carries `Content-Length: 10240` (NOT chunked encoding) per §11.8-CL `maybeAddContentLength` mirror — load-bearing CL-injection assertion**; counter delta `downstream_rq_total +1`, `downstream_rq_2xx +1`. Final counter snapshot after the 6 requests (envoy-go side): `downstream_rq_total=6`, `downstream_rq_2xx=3`, `downstream_rq_4xx=3`. Envoy-only `downstream_rq_too_large` (+3) and `downstream_rq_completed` (+6) filtered out via the existing twin-series-discipline allow-list (per BEHAVIOR_CONTRACT.md `### Twin-series filter discipline` + phase 06.1 SPEC §11.5). Prometheus form: `envoy_http_downstream_rq_total{envoy_http_conn_manager_prefix="ingress_buffer"}`, `envoy_http_downstream_rq_xx{envoy_response_code_class="2",envoy_http_conn_manager_prefix="ingress_buffer"}`, `envoy_http_downstream_rq_xx{envoy_response_code_class="4",envoy_http_conn_manager_prefix="ingress_buffer"}`. **No timing tolerances** (buffer is purely synchronous; no analog to phase 11 fixture 0013 ±10ms refill-boundary). **100-Continue addendum** per SPEC §11.8: scenarios 2 + 5 emit `HTTP/1.1 100 Continue` BEFORE 413 (curl auto-injects `Expect: 100-continue` for `--data-binary @<file>`); scenarios 3 + 6 do NOT (chunked path bypasses); driver treats "first non-1xx response is 413 (or 200)" rather than "first response is 413". Cross-refs SPEC §7.1 + §13.1 + ADR-0125 + ADR-0126 + ADR-0127 v2. ~30 LoC. Per SPEC §4.3. |
| `test/fixtures/0015-http-buffer/README.md` | NEW | Fixture overview + per-scenario equivalence-claim narrative + 6-scenario list (per SPEC §7.1) + single-listener bootstrap discipline (per planner-time decision 7: all 6 scenarios run against the single listener `l_main` with three routes — no per-scenario teardown) + Envoy-deviation note (none — buffer is a normal HTTP filter; no SIGTERM/drain divergence) + the `max_request_bytes ≤ 1 MiB` envoy-go-only parse-time validation note (SPEC §2.1.1 + ADR-0126; reference Envoy accepts arbitrary `UInt32Value`, envoy-go rejects values > 1048576) + the `maybeAddContentLength` chunked → fixed-CL conversion note (SPEC §11.8-CL + ADR-0127 v2; observable at backend boundary) + the per-route disabled-OR-override 5th canonical discipline note (SPEC §1.3 + ADR-0125) + the per-route shared-vacuous stats note (no filter-specific counters; mirrors phase 12 csrf shared-stats discipline modulo the absence-of-counters difference) + planner-time-decision cross-references. ~50 LoC. Per SPEC §4.3. |
| `test/fixtures/0015-http-buffer/driver/driver.go` | NEW | Go driver implementing the SPEC §7.1 + §7.2 6-scenario sequential orchestration via the single-listener topology per planner-time decision 7. **Driver shape:** `package driver`; `init()` calls `fixture.RegisterFixture("0015-http-buffer", &bufferDriver{})`; `BackendCount() int` returns 1; `BackendKind() fixture.BackendKind` returns `fixture.HTTPBuffer` (the new enum value added in Task 7); implements the SINGLE-listener fixture interface (`fixture.Driver` per the fault / cors / header_mutation / csrf precedent — NOT the `MultiListenerDriver` introduced by phase 07.2 + used by phase 11). `ReferenceBootstrap` / `SubjectConfig` templates `envoy.yaml` / `envoy-go.yaml` substituting the listener-port placeholder + backend port; the bootstrap is rendered ONCE. `DriveReference` / `DriveSubject` issue ALL SIX scenarios in ONE call: scenario 1 (POST `/` body=1 KiB CL-known); scenario 2 (POST `/` body=2 MiB CL-known with `Expect: 100-continue`); scenario 3 (POST `/route-tighter` body=~200 KiB chunked via `req.TransferEncoding = []string{"chunked"}` per planner-time decision 8); scenario 4 (POST `/route-disabled` body=2 MiB CL-known); scenario 5 (POST `/route-tighter` body=200 KiB CL-known with `Expect: 100-continue`); scenario 6 (POST `/` body=10 KiB chunked) — backend assertion of inbound `Content-Length: 10240` via JSON-echo + driver-side parse per planner-time decision 9 (D4 settlement option (a)). Per-probe captures status + body + headers (rejection path) + post-scenario comparison via `CompareBytes`; final `/stats/prometheus` scrape captures the in-table HCM counters AND the tag-extracted Prometheus label `envoy_http_conn_manager_prefix="ingress_buffer"` for differential-equivalence assertion. **Go stdlib net/http transparent 100-Continue handling** (per planner-time decision 10): `http.Client.Do` strips 1xx interim responses from the returned `*http.Response`; the driver's status-line assertion compares the FINAL response (200 or 413). **No timing tolerances** — all scenarios run in microseconds; counter scrape is post-hoc via the driver's standard `/stats/prometheus` capture. ~180 LoC. Per SPEC §7.4. |
| `test/fixtures/0015-http-buffer/backends/backend.go` | NEW | Minimal Go HTTP backend bound to a runner-allocated port. Echoes inbound request headers as JSON in the response body — needed for fixture scenario 6's CL-injection assertion (the driver parses the JSON and asserts the `content-length` field equals `10240`). Mirrors the Python `BaseHTTPRequestHandler` echo at SPEC §11.5 empirical pin verbatim, in Go. Single endpoint `/` accepts any method; reads inbound request URL + method + headers; writes response with `Content-Type: application/json` and body `{"method": "POST", "path": "/", "headers": {"host": "...", "content-length": "10240", ...}}` (key/value pairs of inbound canonical headers — the python backend's lowercase-keyed shape is mirrored). Status 200; `Content-Length` set to the JSON body's byte length. Accepts a `--port` flag for the runner-allocated port; `package main` for `go run` invocation by the runner's spawn helper. ~50 LoC (heavier than phase 12's ~30 because of the JSON-echo discipline). Per SPEC §7.4 + planner-time decision 9. |
| `test/differential/fixture/fixture.go` | MODIFIED | New `BackendKind` enum value `HTTPBuffer BackendKind = 12` after the existing `HTTPCsrf BackendKind = 11` (introduced by phase 12). Doc-comment notes: "HTTPBuffer is an out-of-process HTTP/1.1 backend: the runner spawns `test/fixtures/0015-http-buffer/backends/backend.go` on the pre-allocated port. The backend echoes the inbound request method + path + headers as a JSON object in the response body (load-bearing for fixture scenario 6's `Content-Length: 10240` assertion at the backend boundary per the `maybeAddContentLength` mirror per §11.8-CL); status 200, Content-Type: application/json. No TLS. Introduced by fixture 0015-http-buffer (phase 13 Task 7). Because the backend is a subprocess, the runner's in-process accept counter is NOT incremented." ~+15 LoC delta. |
| `test/differential/runner_test.go` | MODIFIED | (a) Add blank-import `_ "github.com/esalaine/envoy-go/test/fixtures/0015-http-buffer/driver"` (insert in alphabetical order, after the `0014-http-csrf` blank-import). (b) Extend the `kind` switch in `runFixture` with a new case `fixture.HTTPBuffer` mirroring the `HTTPCsrf` block: spawn via `startHTTPBufferBackend`. (c) Add new spawn helper `startHTTPBufferBackend(ctx, repoRoot, port int) (*exec.Cmd, error)` mirroring `startHTTPCsrfBackend` from phase 12: `exec.CommandContext(ctx, "go", "run", "./test/fixtures/0015-http-buffer/backends", "--port", fmt.Sprintf("%d", port))` + Setpgid process-group + Stdout/Stderr to os.Stderr + Start. ~+25 LoC delta total. Per SPEC §4.3. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFIED | Per SPEC §13 verbatim Markdown patches: (a) NEW `### envoy.filters.http.buffer` subsection inserted under existing `## HTTP filter chain` umbrella AFTER the `### envoy.filters.http.csrf` subsection at line 1093 landed by phase 12 (per §13.1; ~70 LoC); (b) `## Stat-name mapping ### 29-name table` STAYS UNCHANGED — no new rows; per §13.2 a preamble note appended after the existing table noting phase 13 contributes ZERO new entries (~5 LoC; **NO new tag-extractor preamble** since buffer reuses the existing `envoy_http_conn_manager_prefix` HCM-namespace SN2 extractor; **NO new SN flattening rule**); (c) `## Equivalence Matrix` new buffer-filter row (per §13.3; ~3 LoC); (d) NEW `### Phase 13 forward-pointer notes` subsection appended to existing `## Forward-pointer notes` section per §13.4 (~25 LoC) — covers the 2-item deferral list (`max_request_bytes > 1 MiB` envoy-go-only PARSE-time rejection per ADR-0126; `per_connection_buffer_limit_bytes` / `per_request_buffer_limit_bytes` silent-ignored per ADR-0076) + the no-new-tag-extractor note + the per-route shared-vacuous stats note (no filter-specific counters; consistency framing for the 5th canonical per-route discipline) + the body-counting algorithm divergence-from-Envoy note (envoy-go does its own counting + 413 emission; reference Envoy delegates to HCM via `setBufferLimit + StopIteration`; WIRE OUTCOMES byte-equivalent; only `maybeAddContentLength` semantics observable upstream as deliberate mirror). ADR-0052 in-place edit authorisation carries forward. ~+103 LoC total. |
| `docs/envoy-go/DECISIONS.md` | MODIFIED | Append three new ADRs ADR-0125, ADR-0126, ADR-0127 v2 per SPEC §8 (incrementally per task; each ADR's first-use commit anchors the addition per ADR-0044 ADR-on-impl convention). The 7-section ADR-0001 template applies to each (Status / Date / Doctrine / Lands-in-task / Context / Decision / Alternatives considered / Consequences). **NO inline supersessions / amendments** for phase 13 — UNLIKE phases 10 + 11 which each amended ADR-0073 (phase 10 ADR-0110 multi-tier; phase 11 ADR-0117 stateful-per-route), phase 13 inherits ADR-0073 verbatim with NO third amendment (buffer's per-route is data-only AND wholesale-override; falls within the existing wholesale-override semantics; ADR-0125 captures the disabled-OR-override sum-type within the existing discipline). UNLIKE phase 11 which extended ADR-0061 with Rule SN9, phase 13 does NOT extend ADR-0061 (buffer reuses the existing SN2 rule for HCM-namespace stats — buffer emits no filter-specific counters at all). UNLIKE the original BRAINSTORM §7's anticipated 4 ADRs, phase 13 ships only 3 (ADR-0128 retired per BRAINSTORM §12.7 + SPEC §1.1 amendment 5 — phase 13 contributes zero new stat-table entries; reference Envoy emits no `envoy_http_buffer_*` counter family per §11.5 empirical pin). ADR-0127 is published as **v2** because BRAINSTORM §2.6 hypothesized a Content-Length fast-fail clause that §11.6 empirical pin refuted; the v2 numbering preserves the "named-decision-revision" discipline introduced at phase 12 (ADR-0123 v2 was phase 12's analog though it didn't ship as v2 — phase 13 makes the v2 explicit because the algorithm shape changed materially between brainstorm and SPEC). ~+200 LoC total (3 ADRs; no amendment paragraphs). |
| `docs/envoy-go/ROADMAP.md` | MODIFIED | Row `13` `in-progress → done` flip AT the phase-done commit. The §9 HTTP filters family heading at row 56 stays UNCHANGED (headings are not rows; their state is implicit; per ADR-0106). No new row authored for the next §9 family-child; future family-expansion brainstorms cold-start from the §9 heading + just-shipped phase 13 artefacts (per ADR-0106 no-sibling-stub discipline). Row 13's summary text may need a minor in-place sharpening at the phase-done commit to reflect the SPEC's §1.1 amendment 5 (the row currently summarizes the BRAINSTORM-time pre-amendment scope: "29→31 names" stat-table extension + 4 ADRs; the actual landing is "stays at 29 names" + 3 ADRs); this sharpening is a clean-up, NOT a scope change. |
| `docs/envoy-go/STATE.md` | MODIFIED | Advance through lifecycle-states 3 (PLAN drafting — this PLAN landing flips state 2 → 3 in the orchestrating session's STATE.md edit), 4 (PLAN execution — Tasks 1–11 land production code + fixture; STATE stays at 4), 5 (verification — Task 12 lands BEHAVIOR_CONTRACT/ADRs/six-gate verification; STATE flips 4 → 5), 6 (review — Task 13 REVIEW.md per requesting-code-review skill; STATE flips 5 → 6 then to `awaiting next planning`); `next-skill: superpowers:brainstorming` against §9's family list for the next family-child; `active-phase: <next-family-row-id>` resolved by the next session's planner. |
| `docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md` | NEW | Append-only log; one entry per task; verbatim command outputs. Mirrors phase-04..12 PROGRESS.md structure. The preamble enumerates the three anticipated ADRs ADR-0125, ADR-0126, ADR-0127 v2 + the per-task ADR anchor table + the planner-time deferred-decisions resolution (the 11 items below — D1–D4 from SPEC §12 plus 7 PLAN-emerging items). |
| `docs/envoy-go/phases/13-http-filter-buffer/REVIEW.md` | NEW | End-of-phase review per the 06.1 / 06.2 / 07.1 / 07.2 / 08.1 / 08.2 / 09 / 10 / 11 / 12 cadence; populates per the requesting-code-review skill. Phase 13 has NO parent row (it is a top-level §9 family-child per ADR-0106), so the REVIEW closes only row 13. |

---

## Planner-time deferred-decision resolution (settles SPEC §12 + this PLAN's planner-time-emerged decisions)

The planner is required by SPEC §12 to settle the SPEC's four deferred decisions before implementation; this PLAN settles all four plus seven that emerged at PLAN-drafting time (items 5–11 below). The eleven resolutions are recorded in `PROGRESS.md`'s preamble (Task 1) and reproduced in summary form here so the implementer at each task can act without re-deriving them:

1. **D1 — Filter-callback wiring hook = `SetDecoderCallbacks(cb)` per the cors + fault + header_mutation + localratelimit + csrf precedents; encode side ABSENT.** Per SPEC §12 D1 + survey of existing patterns: `internal/filter/http/cors/cors.go:55` defines `func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }`; `internal/filter/http/fault/fault.go`, `header_mutation/header_mutation.go`, `localratelimit/local_ratelimit.go`, and `csrf/csrf.go` all follow the same pattern. The framework's per-stream state machine (per `internal/filter/http/chain.go`) calls `SetDecoderCallbacks` once per stream as part of the chain construction; the filter stores the callback reference for later use during `DecodeHeaders` (for both `SendLocalReply` and `RequestRouteConfig`) AND `DecodeData` (for `SendLocalReply` on overflow). **Phase 13 does NOT implement `SetEncoderCallbacks` or any encode-side methods** — the `HTTPFilter` value returned by the factory closure sets `Decoder: f, Encoder: nil` per planner-time decision 5. This mirrors phase 12 csrf which set the precedent for decoder-only `HTTPFilter` value (cors + local_ratelimit + fault + header_mutation all set both `Decoder: f, Encoder: f` even when only one side is needed, for chain-of-conformance). Phase 13 inherits phase 12's departure since: (a) buffer has zero encode-side responsibilities (the rejection path uses `SendLocalReply` which enters the encode chain at `filter[len-1]` per ADR-0075 — buffer's filter does NOT participate in that encode iteration; the chain framework handles the localReply chain entry); (b) saving the encode-side struct method implementations + callback field reduces the filter's surface area and makes the read-only-after-`New` invariant more obvious. *Anchored: SPEC §12 D1; cors.go:55; fault.go; header_mutation.go; localratelimit/local_ratelimit.go; csrf/csrf.go; types.go HTTPFilter struct allowing `Encoder: nil` per ADR-0071.*

2. **D2 — `buffer.go` file split = SINGLE-FILE `buffer.go` (no `count.go` or `perroute.go` split).** Per SPEC §12 D2 + §4.1 PLAN-author option. The body-counting helpers (`maybeAddContentLength`, plus the inline accumulation logic in `DecodeData`) total ~30 LoC; the per-route helpers (`parsePerRoute`, `resolveEffective`) total ~50 LoC; both are tightly coupled to the surrounding filter type definitions + `DecodeHeaders` / `DecodeData` body discipline. Splitting into sibling `count.go` or `perroute.go` would: (a) save the reader from scrolling within `buffer.go` (~280-330 LoC), (b) make the helpers' reusability visible (they could in principle be reused by future body-counting filters or future per-route disabled-OR-override filters). However, neither benefit applies to phase 13: (i) ~280-330 LoC stays under the project's general 200-300 LoC mental-model threshold (similar to fault.go ~430 LoC + cors.go ~250 LoC + csrf.go ~280 LoC which all stay single-file); (ii) no future filter is anticipated to reuse buffer's body-counting helpers — if/when such a filter lands, the helpers can be promoted to a shared package then. The single-file approach mirrors csrf.go (which keeps everything in one file at ~280 LoC because the filter is a single integrated unit with no separable primitive — same applies to buffer; the body-counting state machine is intrinsic to the filter's `DecodeData` body, NOT a separable token-bucket-like primitive). DIVERGES from phase 11's `local_ratelimit.go` + `bucket.go` split because the `tokenBucket` was a separable primitive; buffer's body-counting is not. *Anchored: SPEC §12 D2; project code-quality discipline; csrf single-file precedent.*

3. **D3 — Filter-internal validation error message wording = envoy-go's own clear-text wording (option (b)) per phase 11 ADR-0115 + phase 12 ADR-0121 precedent.** Per SPEC §12 D3 + §6.1. Phase 13 emits its own clear-text error messages from the `New` factory + `parsePerRoute` rather than mirroring Envoy's PGV envelope verbatim (option (a)). Specifically, the four error wordings per SPEC §6.1 + §6.3 are: `"buffer: invalid typed_config: %w"` (Any unmarshal failure; %w wraps proto error); `"buffer: max_request_bytes is required"` (cfg.GetMaxRequestBytes() == nil); `"buffer: max_request_bytes must be > 0"` (zero value); `"buffer: max_request_bytes (%d) exceeds envoy-go cap of 1048576 bytes"` (over cap; %d formats the offending value). Per-route variants prepend `"buffer per-route: "` instead of `"buffer: "`. The PGV-required mirror error is `"buffer per-route: override oneof is required (neither disabled nor buffer set)"`; the bool.const mirror error is `"buffer per-route: disabled must be true (PGV bool.const violation)"`; the wrong-type assertion error is `"buffer per-route: expected *BufferPerRoute, got %T"`. Phase 11's ADR-0115 set the precedent — used option (b) for the 50ms `fill_interval` validation with the verbatim Envoy wire-equivalence note as a deliberate exception. Phase 12 ADR-0121 inverted that exception by choosing (b) for csrf's proto-shape PGV check (no canonical Envoy-mirror equivalent). Phase 13 has no analogous boot-log byte-equivalence claim (the differential fixture asserts request-time wire shape, NOT boot-log byte equivalence); Envoy's PGV envelope wording (`BufferValidationError.MaxRequestBytes: value is required`) is descriptive but tied to PGV machinery envoy-go does not host. envoy-go's own wording is operator-friendlier (no opaque PGV-envelope prefix) and consistent with phase 11 + 12 patterns. *Anchored: SPEC §12 D3; phase 11 ADR-0115 + phase 12 ADR-0121 envoy-go-own-wording precedent.*

4. **D4 — Backend-side Content-Length assertion mechanism = OPTION (a) JSON-echo (the existing fixture-pattern is a per-fixture backend; phase 13's backend echoes inbound headers as JSON in the response body; the driver parses the JSON and asserts the `content-length` field).** Per SPEC §12 D4 + §7.4. The python `BaseHTTPRequestHandler` echo used at SPEC §11.5 empirical pin already prints inbound request headers as JSON in the body (e.g., `{"method": "POST", "path": "/", "headers": {"host": "...", "content-length": "65536", ...}}`) — verifying the §11.8-CL mirror against this evidence is what gave SPEC §11.8 its empirical anchor. Phase 13 mirrors the Python backend in Go for the fixture's host-side backend at `test/fixtures/0015-http-buffer/backends/backend.go` (per the existing fixture pattern phase 11 + 12 used; the backend is a per-fixture subprocess spawned by the runner, NOT a host-side helper at `test/helpers/echobackend/`). The driver parses the JSON response body and asserts `headers["content-length"] == "10240"` for fixture scenario 6 (chunked → CL-injection passthrough). Option (b) — a custom `x-asserted-content-length` echo header — was rejected as over-engineering (the JSON body contains all inbound headers; no custom header needed). Option (c) — a per-fixture assertion hook that fails the fixture if inbound CL ≠ 10240 — was rejected because the assertion belongs in the driver (where it can be compared against both Envoy AND envoy-go's responses for differential equivalence), NOT in the backend (where it would only see one side). The JSON-echo backend serves both Envoy and envoy-go indiscriminately; the driver's assertion runs against both sides' parsed responses. *Anchored: SPEC §12 D4 + §7.4 + §11.8-CL evidence; python backend pattern at SPEC §11.5; phase 11 + 12 fixture-backend precedent.*

5. **PLAN-emerging — `HTTPFilter` value shape = `Decoder: f, Encoder: nil` (decoder-only).** Per the `internal/filter/http/types.go:75-78` `HTTPFilter` struct definition: `type HTTPFilter struct { Name string; Decoder StreamDecoderFilter; Encoder StreamEncoderFilter }` — comment "nil for encoder-only filters" / "nil for decoder-only filters" makes both nilable. The chain framework's `RunDecodeHeaders` / `RunEncodeHeaders` iterators dispatch only on non-nil sides per ADR-0071. Buffer has no encode-side state and no encode-side responsibilities; setting `Encoder: nil` (a) reduces struct surface area, (b) saves implementing the `StreamEncoderFilter` method set on `*filter` (no `EncodeHeaders` / `EncodeData` / `EncodeTrailers` / `SetEncoderCallbacks` duplication), (c) makes the decoder-only nature self-documenting. Phase 12 csrf set the §9-family-row precedent for structurally decoder-only `HTTPFilter` value; phase 13 inherits it. *Anchored: types.go HTTPFilter (lines 75-78); ADR-0071 iteration protocol; SPEC §6.7; phase 12 csrf ADR-0120 precedent.*

6. **PLAN-emerging — Fixture topology = SINGLE LISTENER `l_main` with THREE ROUTES (`/`, `/route-disabled`, `/route-tighter`).** Per SPEC §7.2. Phase 13's 6 scenarios split across the same listener: scenarios 1, 2, 6 against `/` (listener-level Buffer 1 MiB cap); scenario 3, 5 against `/route-tighter` (per-route override 128 KiB cap); scenario 4 against `/route-disabled` (per-route `disabled: true`). All scenarios run against the SAME listener with the same listener-level config; only routes 2 and 3 carry per-route TPFC. UNLIKE phase 11's 4-listener topology (which was driven by per-scenario distinct bucket parameters), phase 13's scenarios all share the listener-level `max_request_bytes: 1048576` config; the only varying inputs are the request path (driving per-route resolution) + body size + Transfer-Encoding (CL-known vs chunked). The single-listener topology fits the existing `fixture.Driver` contract (NOT `MultiListenerDriver`) — same as fault 0011 + cors 0007a/0007b + header_mutation 0012 + csrf 0014. Saves driver complexity (no per-scenario port allocation; no `DriveSubjectMulti` / `DriveReferenceMulti` orchestration). *Anchored: SPEC §7.2 (the SPEC's bootstrap fragment shows a single listener with three routes); phase 09 / 10 / 12 single-listener precedents (0011 / 0012 / 0014); phase 11 deviated for per-scenario bucket distinctness which is not phase 13's case.*

7. **PLAN-emerging — BackendKind enum value = `HTTPBuffer BackendKind = 12`** (continues existing naming convention; next value after phase 12's `HTTPCsrf BackendKind = 11` at `test/differential/fixture/fixture.go:218`). Doc-comment matches the format used for `HTTPCsrf`. *Anchored: phase 12 PLAN planner-time decision 9 precedent; existing enum at `test/differential/fixture/fixture.go:129-218`.*

8. **PLAN-emerging — Chunked-body construction in driver = `req.TransferEncoding = []string{"chunked"}` + `bytes.NewReader(data)` body via Go stdlib net/http.** Scenarios 3 and 6 require the driver to issue chunked POST requests. Go's net/http library: by default, `http.NewRequest(method, url, body)` with a `*bytes.Reader` body sets `Content-Length` from the reader's size and uses identity transfer encoding. To force chunked encoding, the driver MUST set `req.TransferEncoding = []string{"chunked"}` BEFORE calling `client.Do(req)`. This is the documented Go stdlib idiom (per `https://pkg.go.dev/net/http#Request.TransferEncoding` — the chunked transfer encoding is requested explicitly). Alternative: `io.Pipe` with goroutine-based incremental writes (more complex; matches phase 11's slow-stream pattern) — unnecessary for phase 13 since the bodies are tractably small (200 KiB for scenario 3; 10 KiB for scenario 6) and don't need streaming pacing. The driver constructs the body bytes upfront via `bytes.NewReader(make([]byte, 200*1024))` (or 10*1024 for scenario 6), sets `req.TransferEncoding = []string{"chunked"}`, and lets the stdlib write the chunked framing on the wire. Reference Envoy receives the chunked body and accumulates it in the buffer filter's per-stream state per the `DataStopIterationAndBuffer` discipline; envoy-go's filter does the same via `f.accumulated += uint32(data.Len())` in `DecodeData`. *Anchored: SPEC §7.4 driver shape; Go stdlib `net/http.Request.TransferEncoding` semantics; reference Envoy §11.9 P9.B chunked-passthrough probe.*

9. **PLAN-emerging — Backend echoes inbound headers as JSON in response body (mirroring SPEC §11.5 python backend).** Per planner-time decision 4 (D4 settlement) elaboration. The phase 13 backend at `test/fixtures/0015-http-buffer/backends/backend.go` is a Go HTTP/1.1 server that, for any inbound request, writes a JSON body containing `{"method": "<method>", "path": "<path>", "headers": {"<lowercase-key>": "<value>", ...}}`. The header keys are lowercased to match Envoy's wire-form (per ADR-0072 + phase 04 lowercase header discipline); the values are the header values as received. Status 200; Content-Type: application/json; Content-Length set to the JSON body's byte length. The driver parses the JSON via `encoding/json` and asserts `parsed.Headers["content-length"] == "10240"` for scenario 6. Backends for scenarios 1 + 4 also receive a JSON body (any non-overflow scenario reaches the backend); the driver does not assert on those scenarios' headers (only on counter deltas + status equivalence). *Anchored: SPEC §11.5 python `BaseHTTPRequestHandler` echo; planner-time decision 4 (D4); D8.*

10. **PLAN-emerging — Go stdlib transparent 100-Continue handling.** Per SPEC §7.3 (100-Continue prefix on scenarios 2 + 5; absent on 3 + 6). Go's `http.Client.Do(req)` strips 1xx interim responses from the returned `*http.Response` — when the server emits `HTTP/1.1 100 Continue` followed by `HTTP/1.1 413 Payload Too Large`, the client sees ONLY the 413 in the returned response. The driver's status-line assertion (`resp.StatusCode == 413`) compares the FINAL response, not the 100-Continue prefix. This matches reference Envoy's HCM/H1-codec behavior on the wire (the 100-Continue is HCM/H1-codec discipline, NOT buffer-filter discipline) AND envoy-go's HCM/H1-codec behavior (already shipped in phase 04 — `internal/filter/hcm/h1.go` handles `Expect: 100-continue` per the standard H1 server discipline). The driver's wire-shape assertion treats "first non-1xx response is 413 (or 200)" rather than "first response is 413"; the Go stdlib client transparently absorbs the 1xx. **No driver-level code is needed to handle 100-Continue.** The only requirement is that the driver explicitly attaches `Expect: 100-continue` to scenarios 2 + 5 (CL-known with body size > some-threshold) so reference Envoy's HCM emits the prefix; on scenarios 3 + 6 (chunked), no `Expect:` header → no 100-Continue. *Anchored: SPEC §7.3 + §11.8 100-Continue addendum; Go stdlib `http.Client` 1xx handling.*

11. **PLAN-emerging — `effectiveMax` + `accumulated` field types = `uint32`.** Per SPEC §6.2's `compiledConfig.maxRequestBytes uint32` + the parse-time invariant `value ≤ 1048576`. The per-stream `accumulated` counter on `*filter` is also `uint32` since it is bounded by the same cap (overflow at 4 GiB is impossible — the cap is 1 MiB ≪ 4 GiB; the request body cannot reach 4 GiB without first hitting the framework's `filterBufferLimitBytes = 1 << 20` safety net or the buffer filter's own `effectiveMax > effectiveMax` overflow check). Casting `data.Len()` (an `int`) to `uint32` is safe within the request-body domain. Alternative: `uint64` for safety against future cap-promotion (when ADR-0076 is amended to per-stream tunability and `max_request_bytes` could exceed 4 GiB). PLAN chooses `uint32` for symmetry with `compiledConfig.maxRequestBytes` — when the future cap-promotion phase amends ADR-0126's parse-time check, the field type can be widened in lockstep with `compiledConfig.maxRequestBytes`. *Anchored: SPEC §6.2 (`maxRequestBytes uint32`); ADR-0126 parse-time invariant.*

These eleven decisions are reproduced verbatim in `docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md` Preamble (Task 1) so any subsequent reader has the full context without re-reading this PLAN.

---

## ADRs introduced by this plan

The three ADRs anticipated by SPEC §8 (ADR-0125, ADR-0126, ADR-0127 v2). Each ADR's "Lands-in-task" anchor is fixed below per ADR-0044 ADR-on-impl convention; the implementer at the named task appends the ADR to `DECISIONS.md` per the ADR-0001 7-section template (Status / Date / Doctrine / Lands-in-task / Context / Decision / Alternatives considered / Consequences). The three ADRs land in topical-vs-commit-time-permuted order per the 07.1 / 07.2 / 08.1 / 08.2 / 09 / 10 / 11 / 12 PLAN convention; the per-task appendix records the ordering chosen by the implementer.

| ADR | Title | Lands-in-task |
|---|---|---|
| ADR-0125 | `internal/filter/http/buffer/` package shape (TypeURL + New + filter struct + decoder-only `HTTPFilter` value with `Encoder: nil`; **single-token directory matching cors / fault / csrf precedent** — no underscore needed since the proto type-name is already a single token) + extension-registry registration line + boot-time `httpReg.Register(buffer.TypeURL, buffer.New)` ordering (`router → buffer → cors → csrf → ...`) + per-route disabled-OR-override discipline (5th canonical per-route shape; references ADR-0073 + ADR-0117 + ADR-0124 explicitly; per-route stats SHARED-vacuous since phase 13 emits no filter-specific counters per SPEC §1.1 amendment 5) | Task 2 (`internal/filter/http/buffer/{doc.go,buffer.go}` package skeleton + `parsePerRoute` first lands; the boot registration code lands in Task 6 but ADR-0125 anchors at Task 2 because that's the first-use site that justifies the package shape per ADR-0044). |
| ADR-0126 | `compiledConfig` shape + 1-consumed/0-deferred field decomposition (`max_request_bytes` UInt32Value REQUIRED; envoy-go-own validation: non-nil + value > 0 + value ≤ 1048576 / 1 MiB) + parse-time `max_request_bytes ≤ 1 MiB` validation (envoy-go-only divergence vs reference Envoy v1.37.2 which accepts arbitrary `UInt32Value` per §11.1 empirical pin) + cap-layering rationale (buffer's check fires INSIDE the framework cap; ADR-0076's `filterBufferLimitBytes = 1 << 20` stays armed as safety net; structurally unreachable in MVP because `effectiveMax ≤ 1 MiB` invariant) + explicit forward-pointer to the future cap-promotion phase (compression's natural amender per ADR-0076 §Consequences (d)) + PGV-mirror filter-internal validation discipline at `New` time (envoy-go-own-wording errors per planner-time decision 3 — phase 11 ADR-0115 + phase 12 ADR-0121 precedent) | Task 2 (`compiledConfig` + `New` factory + parse-time PGV-mirror validation + `parsePerRoute` first lands). |
| ADR-0127 v2 | Body-counting + 413-trigger algorithm — STREAMING-CAP ONLY (NO Content-Length fast-fail per §11.6 — BRAINSTORM §2.6 hypothesized fast-fail; v2 retires it after empirical refutation) + `DecodeHeaders` returns `StopIteration` on bodied + non-disabled requests (mirrors `buffer_filter.cc:67`) + `DecodeData` accumulation via `DataStopIterationAndBuffer` per chunk + `DataStopIterationNoBuffer` on overflow + cap predicate `>` strict (per §11.2 — `accumulated > effectiveMax`, NOT `>=`) + `maybeAddContentLength` mirror per `buffer_filter.cc:91-97` + reuse of framework `SendLocalReply` 413 wire shape (per ADR-0076 §Decision (b) byte-equivalence; CONFIRMED at §11.7 + §11.8 empirical pins) + 100-Continue addendum (HCM/H1-codec emits independent of buffer filter on `Expect: 100-continue` requests; chunked path bypasses) + `maybeAddContentLength` chunked → fixed-CL conversion observable at upstream boundary (per §11.8-CL empirical evidence) | Task 3 (`DecodeHeaders` body — header-only fast-path + per-route disabled passthrough + bodied StopIteration first lands; this is the structurally-novel part of the algorithm); ADR-0127 v2 anchors at Task 3 with the algorithm's gateway entry point; Task 4 completes the body-counting mechanics in `DecodeData` + `maybeAddContentLength` without a new ADR. |

The implementer at each task drafts the ADR body following the ADR-0001 template; the per-task acceptance bullet "ADR-XXXX appears in DECISIONS.md with full Context/Decision/Consequences sections" enforces compliance.

**Inline supersessions / amendments anticipated** (cross-references only; **NO in-place ADR edits required** — this is a notable simplification consistent with phase 12; UNLIKE phases 10 + 11 which each amended ADR-0073):

- **ADR-0073** (typed_per_filter_config 3-tier merge — most-specific override) — UNCHANGED in phase 13. Phase 13's per-route is data-only AND most-specific-override; the wholesale-override discipline applies as-is. Phase 11's ADR-0117 amendment paragraph (stateful per-route extension) and phase 10's ADR-0110 amendment (multi-tier evaluation) and phase 12's ADR-0124 (data-only with shared stats) all stay landed and unused by phase 13. Cross-reference recorded in ADR-0125 §Decision noting the canonical 5-shape table: phase 07.1 cors (data-only override / shared stats / ADR-0073); phase 10 header_mutation (multi-tier / shared stats / ADR-0110); phase 11 local_ratelimit (stateful / INDEPENDENT stats / ADR-0117); phase 12 csrf (data-only / SHARED stats / ADR-0124); **phase 13 buffer (disabled-OR-override sum-type / SHARED-vacuous stats / ADR-0125 — first time per-route is a structural sum type; first time per-route stats are vacuously shared because no filter-specific counters exist).** NO in-place edit of ADR-0073.
- **ADR-0040** (out-of-scope deferrals format) — UNCHANGED in phase 13. The 2-item deferral list (per SPEC §2.1.1 / §2.1.2) is captured INLINE at BEHAVIOR_CONTRACT §13.4 (the `### Phase 13 forward-pointer notes` subsection). NO new deferral ADRs are authored at phase 13 (mirrors phase 10 / phase 11 SPEC §8.1 collapse precedent — silent-ignore + parse-time-rejection are framework patterns, deferral lists are documentation artefacts).
- **ADR-0061** (stats Registry + SN1–SN9 rules) — UNCHANGED in phase 13. NO new SN flattening rule. Buffer reuses the existing SN2 rule (HCM-namespace `http.<HCM stat_prefix>.<rest>` → `envoy_http_<rest>` + label `envoy_http_conn_manager_prefix=<HCM stat_prefix>`); however phase 13 emits NO filter-specific counters (per SPEC §1.1 amendment 5 + §11.5 empirical pin — reference Envoy emits no `envoy_http_buffer_*` counter family at all), so no SN-rule application is even needed. UNLIKE phase 11 which extended SN with Rule SN9 for the filter-specific `envoy_local_http_ratelimit_prefix` extractor via ADR-0118. Cross-reference recorded in ADR-0125 §Decision ("phase 13 demonstrates the family-row pattern carries through filters that emit ZERO filter-specific stats; the per-route discipline retains the SHARED-stats invariant from ADR-0124 with the special case that 'shared' is structurally vacuous when there is nothing to share"). NO in-place edit.
- **ADR-0072** (HTTPRegistry threaded constructor map + factory typed_config validation contract) — UNCHANGED in phase 13 (the existing `Register` + `Freeze` discipline carries through). Cross-reference recorded in ADR-0125 §Consequences. NO in-place edit.
- **ADR-0074** (filter set: cors + envoy_go_test) — purely additive expansion recorded in ADR-0125 §Consequences. The filter set extends from {cors, csrf, envoy_go_test, router, fault, header_mutation, local_ratelimit} to {buffer, cors, csrf, envoy_go_test, router, fault, header_mutation, local_ratelimit}. NO in-place edit of ADR-0074.
- **ADR-0075** (HCM dispatch — wire-write path for SendLocalReply) — UNCHANGED in phase 13 (the existing 413 wire-write path carries through verbatim per §11.7 + §11.8 empirical pins). Cross-reference recorded in ADR-0127 v2 §Consequences. NO in-place edit.
- **ADR-0076** (framework body-buffer cap — `filterBufferLimitBytes = 1 << 20` + 17-byte `localReply413Body` synthesis) — UNCHANGED in phase 13. ADR-0126 §Decision notes the cap-layering rationale ("buffer's `accumulated > effectiveMax` check fires INSIDE the framework cap because `effectiveMax ≤ 1 MiB` invariant"); ADR-0127 v2 §Decision notes the wire-shape reuse ("the 413 emitted by the buffer filter on overflow is byte-equivalent to the 413 the framework synthesizes on `DataStopIterationAndBuffer` overflow per ADR-0076 §Decision (b) — same status, same body, same 4-header set, same `Connection: close`"). NO in-place edit of ADR-0076. The future cap-promotion phase is the natural amender per ADR-0076 §Consequences (d).
- **ADR-0100** (FactoryCtx framework extension — Stats + StatPrefix) — UNCHANGED in phase 13. Buffer's `New` factory CONSUMES ZERO `ctx.Stats` + `ctx.StatPrefix` since no filter-specific counters are registered (per SPEC §1.1 amendment 5 — buffer-filter overflow is observable on the existing in-table HCM `downstream_rq_4xx` counter, NOT via filter-specific counters). ADR-0125 §Consequences notes the no-stats-consumption pattern. NO in-place edit.
- **ADR-0101** (runtimeConfig shape + parser pattern) — extended cross-reference recorded in ADR-0126 §Consequences. The buffer `compiledConfig` mirrors fault/csrf structurally (1 field vs fault's 8 + csrf's 2 — the SMALLEST `compiledConfig` shape so far in §9 family rows; closure-capture + parse-at-New + read-only-shared-after-New discipline applies as-is). NO in-place edit of ADR-0101.
- **ADR-0102** (terminal-replace + StopIteration localReplyDone gate) — VERBATIM REUSE in phase 13; no change. ADR-0127 v2 §Consequences notes that the request-side terminal-replace primitive carries through unchanged for the rejection path (same primitive used by phase 09 fault abort + phase 11 local_ratelimit + phase 12 csrf — buffer is the FIFTH consumer). NO in-place edit.
- **ADR-0117** + **ADR-0120** + **ADR-0124** (the prior 4 canonical per-route disciplines) — UNCHANGED in phase 13. ADR-0125 §Decision references all three explicitly in the canonical 5-shape table (cited above). NO in-place edits.

These eleven cross-references land at the task that anchors each affected ADR (ADR-0125 + ADR-0126 at Task 2; ADR-0127 v2 at Task 3). **NO in-place edit of any pre-existing ADR is required by phase 13** — this is a notable simplification matching phase 12, divergent from phases 10 + 11 (each of which amended ADR-0073).

---

## Execution preconditions

Before Task 1, the implementer cold-starts and verifies. **Worktree spawn discipline:** the impl session is expected to run on a fresh worktree branched off the PLAN tip per ADR-0003 + the per-phase-worktree convention (per the user's persistent preference for git worktrees recorded in `~/.claude/projects/-home-esa-git-envoy-go/memory/MEMORY.md`). The expected sequence (executed by the orchestrating session BEFORE invoking the impl session, OR by the impl session itself at cold-start if it's running standalone) is:

```bash
# From the master worktree (or any non-conflicting worktree):
git worktree add /home/esa/git/envoy-go/.worktrees/phase-13-http-filter-buffer-impl \
                 -b phase-13-http-filter-buffer-impl <PLAN-tip-SHA>
cd /home/esa/git/envoy-go/.worktrees/phase-13-http-filter-buffer-impl
```

where `<PLAN-tip-SHA>` is the master tip after the PLAN.md commit + its SHA-fill follow-up (filled by the orchestrating session that landed the PLAN).

The 16 preconditions verified at Task 1 cold-start:

1. **Worktree branch.** `git rev-parse --abbrev-ref HEAD` returns `phase-13-http-filter-buffer-impl` (the impl-stage worktree). If a SPEC-stage or PLAN-stage worktree is the only branch present, branch a fresh impl worktree from master HEAD per ADR-0003: `git worktree add .worktrees/phase-13-http-filter-buffer-impl -b phase-13-http-filter-buffer-impl master` then `cd` into it.
2. **Master tail.** `git log --oneline master | head -10` shows the PLAN.md commit (this plan) and its SHA-fill follow-up at the head, with the SPEC.md commit `f5d38fa` and its SHA-fill follow-up `6e39444` immediately before, then the BRAINSTORM.md commits `37d4dfa` (advisory-rec follow-up) + `812d234` (STATE.md update) + `3915338` (§12 amendment) + `6cf412e` (initial brainstorm), then phase 12 REVIEW at `a782fc9`. If not, the cold-start environment is stale; resync via `git fetch origin master && git pull --ff-only`.
3. **Toolchain.** `go version` reports `go1.23.0` or newer. `golangci-lint version` reports `1.64.8` (ADR-0009 pin). `docker version` reports both client + server (the differential harness needs Docker).
4. **DECISIONS.md tail.** `grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1` returns `124`. If it returns a higher number, another phase has landed concurrently; re-verify the next-free numbers (ADR-0125 + ADR-0126 + ADR-0127 may need bumping per ADR-0004).
5. **SPEC SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/13-http-filter-buffer/SPEC.md` returns `f5d38fa` (the SPEC commit) or descendant. If it returns a different SHA, the SPEC has been amended; re-read SPEC and re-verify §11 empirical pins are still valid.
6. **Pristine tree.** `git status --porcelain` returns empty. If not, commit or stash the uncommitted state before starting.
7. **Pre-existing fixtures green at `-short` budget.** `go test -count=1 -short ./...` returns clean.
8. **Pre-existing differential suite green.** `go test -count=1 ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005|Test.*0006|Test.*0007a|Test.*0007b|Test.*0008|Test.*0009|Test.*0010|Test.*0011|Test.*0012|Test.*0013|Test.*0014'` returns every fixture PASS. The 15 pre-existing fixtures (0000–0014) are the regression baseline.
9. **Pre-existing fuzzers run clean at 30s.** The 16 fuzzers from phases 02–12 run clean. Phase 13 adds the seventeenth (`FuzzBufferConfigParse` per Task 5).
10. **Reference Envoy image present.** `docker pull envoyproxy/envoy:v1.37.2` returns success; `docker image inspect envoyproxy/envoy:v1.37.2` returns the SHA `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 pin).
11. **`envoy.extensions.filters.http.buffer.v3` proto package present in module closure.** `go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3 Buffer | head -5` returns the `Buffer` proto type's exported fields without an `import path failed` error; `go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3 BufferPerRoute | head -5` returns the `BufferPerRoute` proto. If any `go doc` fails, the go-control-plane module needs `go mod download` (or `go mod tidy` if a version bump is needed; the SPEC reports the module is already in the closure at master `a782fc9` so a tidy should not be needed — phase 12 csrf added the `envoy.extensions.filters.http.csrf.v3` import which is in the same go-control-plane module).
12. **Pre-existing `internal/filter/http/buffer/` directory does NOT exist.** `test ! -d internal/filter/http/buffer && echo "ok: buffer absent"` returns success. If non-empty, the package has been added by a concurrent phase — investigate before proceeding.
13. **Pre-existing `fixture.HTTPBuffer` does NOT exist.** `grep -nE 'HTTPBuffer' test/differential/fixture/fixture.go` returns 0 matches. If 1+, investigate.
14. **CONFORMANCE_PINS.md UNCHANGED.** `git diff master -- docs/envoy-go/CONFORMANCE_PINS.md` reports zero changes (D-3.7).
15. **Pre-existing `cmd/envoy-go/main.go` registers exactly the SEVEN filters expected at master `a782fc9`** — `grep -nE 'httpReg.Register' cmd/envoy-go/main.go` returns 7 matches: `router`, `cors`, `csrf`, `envoygotest`, `fault`, `header_mutation`, `localratelimit`. If 8+, another filter has been added concurrently; re-verify the registration ordering before adding the buffer line.
16. **Pre-existing `BEHAVIOR_CONTRACT.md` carries the phase-12 `### envoy.filters.http.csrf` subsection at line 1093** — `grep -n '^### envoy.filters.http.csrf' docs/envoy-go/BEHAVIOR_CONTRACT.md` returns line `1093`. If 0 matches or different line, the file has drifted; re-read SPEC §13.1 to re-anchor the new buffer subsection insertion point.

If all 16 preconditions pass, proceed to Task 1.

---

## Task 1: Execution-precondition check + PROGRESS.md preamble

**Files:**
- Create: `docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md`

This task verifies the `## Execution preconditions` block above and creates PROGRESS.md so subsequent tasks have an append target. Per ADR-0044 ADR-on-impl convention, the three ADRs ADR-0125 / ADR-0126 / ADR-0127 v2 are NOT all landed at Task 1 — each ADR lands at the task that anchors its first-use commit (per the table above). Task 1 lands NO ADR; the PROGRESS preamble simply ANTICIPATES the three ADRs and records the planner-time decisions resolution.

**Precondition:** worktree exists at `phase-13-http-filter-buffer-impl`; branch base is master tip after the PLAN.md SHA-fill follow-up; all 16 preconditions above report green.
**Artifact:** `docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md` (new file).
**Acceptance:** all 16 preconditions report green; PROGRESS.md preamble entry committed; `git log -1 --format=%H -- docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md` returns the Task 1 commit's SHA.

- [ ] **Step 1: Verify each precondition**

Run, in the worktree root:

```bash
git rev-parse --abbrev-ref HEAD                                       # expect: phase-13-http-filter-buffer-impl
git log --oneline master | head -10                                   # expect: PLAN SHA-fill, PLAN, SPEC SHA-fill (6e39444), SPEC (f5d38fa), BRAINSTORM commits, phase-12 REVIEW (a782fc9)
docker version                                                        # expect: client + server reported
go version                                                            # expect: go1.23+
golangci-lint version                                                 # expect: 1.64.8
go test -count=1 -short ./...                                         # expect: every package PASS
go test -count=1 ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005|Test.*0006|Test.*0007a|Test.*0007b|Test.*0008|Test.*0009|Test.*0010|Test.*0011|Test.*0012|Test.*0013|Test.*0014' -v
                                                                       # expect: every fixture PASS (15 fixtures)
grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1
                                                                       # expect: 124
git log -1 --format=%H -- docs/envoy-go/phases/13-http-filter-buffer/SPEC.md
                                                                       # expect: f5d38fa... or descendant
git status --porcelain                                                # expect: empty
test ! -d internal/filter/http/buffer && echo "ok: buffer absent"
go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3 Buffer | head -5
                                                                       # expect: type Buffer struct { ... }
go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3 BufferPerRoute | head -5
                                                                       # expect: type BufferPerRoute struct { ... }
grep -cE 'HTTPBuffer' test/differential/fixture/fixture.go            # expect: 0
docker pull envoyproxy/envoy:v1.37.2                                  # expect: pull success
git diff master -- docs/envoy-go/CONFORMANCE_PINS.md                  # expect: empty
grep -cE 'httpReg.Register' cmd/envoy-go/main.go                      # expect: 7
grep -n '^### envoy.filters.http.csrf' docs/envoy-go/BEHAVIOR_CONTRACT.md
                                                                       # expect: 1093:### envoy.filters.http.csrf
```

If any line fails, stop and follow the precondition's "if fails" guidance.

- [ ] **Step 2: Author `PROGRESS.md` preamble**

Create `docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md` with the following structure:

````markdown
# Phase 13 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-04..12 PROGRESS.md structure.

## Preamble — execution preconditions

(Verbatim 16-precondition output captured during Task 1; all 16 green.)

## Preamble — anticipated ADRs (per ADR-0044 ADR-on-impl convention; SPEC §8)

The three ADRs anticipated by SPEC §8 (ADR-0125, ADR-0126, ADR-0127 v2). Each lands at the task that anchors its first-use commit per the PLAN.md "ADRs introduced by this plan" table:

- **ADR-0125** `internal/filter/http/buffer/` package shape (single-token directory matching cors / fault / csrf precedent + extension-registry registration ordering + decoder-only `HTTPFilter` value with `Encoder: nil`) + per-route disabled-OR-override discipline (5th canonical per-route shape) — Task 2
- **ADR-0126** `compiledConfig` shape + 1-consumed/0-deferred field decomposition (`max_request_bytes`) + parse-time `max_request_bytes ≤ 1 MiB` validation (envoy-go-only divergence) + cap-layering rationale + PGV-mirror filter-internal validation discipline at `New` time — Task 2
- **ADR-0127 v2** Body-counting + 413-trigger algorithm — STREAMING-CAP ONLY + `DecodeHeaders` StopIteration on bodied + non-disabled requests + `DecodeData` accumulation + `DataStopIterationNoBuffer` on overflow + cap predicate `>` strict + `maybeAddContentLength` mirror + reuse of framework `SendLocalReply` 413 wire shape + 100-Continue addendum — Task 3

## Preamble — planner-time deferred-decision resolution (per PLAN §"Planner-time deferred-decision resolution")

The eleven planner-time deferred decisions reproduced verbatim from PLAN.md so this PROGRESS.md is self-contained for any task-N reader:

1. **D1 — Filter-callback wiring hook = `SetDecoderCallbacks(cb)`; encode side ABSENT** (decoder-only filter; HTTPFilter struct sets Decoder: f, Encoder: nil — mirrors phase 12 csrf precedent).
2. **D2 — `buffer.go` file split = SINGLE-FILE** (no `count.go` or `perroute.go`; ~280-330 LoC stays under mental-model threshold; mirrors csrf single-file precedent).
3. **D3 — Filter-internal validation error message wording = envoy-go's own clear-text wording** (option (b); `buffer: max_request_bytes is required` etc.; phase 11 ADR-0115 + phase 12 ADR-0121 precedent).
4. **D4 — Backend-side Content-Length assertion mechanism = OPTION (a) JSON-echo** (backend echoes inbound headers as JSON in response body; driver parses and asserts `headers["content-length"] == "10240"` for fixture scenario 6).
5. **PLAN-emerging — `HTTPFilter` value shape = `Decoder: f, Encoder: nil`** (decoder-only; saves implementing StreamEncoderFilter method set; mirrors phase 12 csrf ADR-0120 precedent).
6. **PLAN-emerging — Fixture topology = SINGLE LISTENER `l_main` with THREE ROUTES** (`/` default + `/route-disabled` + `/route-tighter`; fits existing `fixture.Driver` contract; saves driver complexity vs phase 11's 4-listener topology).
7. **PLAN-emerging — BackendKind enum value = `HTTPBuffer BackendKind = 12`** (continues existing naming convention; next value after `HTTPCsrf BackendKind = 11`).
8. **PLAN-emerging — Chunked-body construction in driver = `req.TransferEncoding = []string{"chunked"}` + `bytes.NewReader(data)`** (Go stdlib net/http idiom; no io.Pipe needed since bodies are tractably small ≤200 KiB).
9. **PLAN-emerging — Backend echoes inbound headers as JSON** (mirrors SPEC §11.5 python `BaseHTTPRequestHandler` echo; lowercase header keys per Envoy wire-form discipline).
10. **PLAN-emerging — Go stdlib transparent 100-Continue handling** (`http.Client.Do` strips 1xx interim responses; driver compares final response only; no driver-level 100-Continue code needed).
11. **PLAN-emerging — `effectiveMax` + `accumulated` field types = `uint32`** (matches `compiledConfig.maxRequestBytes uint32`; future cap-promotion phase widens in lockstep).

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** `<SHA>` — `phase 13: PROGRESS preamble + planner-time decision resolution`
**Notes:** Created PROGRESS.md; verified all 16 preconditions per PLAN §"Execution preconditions"; phase-13 SPEC + PLAN confirmed present in HEAD; SPEC at f5d38fa; ADR tail at 0124 (next-free 0125); `internal/filter/http/buffer/` absent (Task 2 lands); `fixture.HTTPBuffer` absent (Task 7 lands). No ADR landed in Task 1 (ADR-0044 ADR-on-impl convention; ADRs land at first-use commit per PLAN's ADR table).

**Outputs:** (16 verbatim command outputs captured.)
````

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 13: PROGRESS preamble + planner-time decision resolution

Authors PROGRESS.md per ADR-0044 ADR-on-impl convention; verifies all 16
preconditions per PLAN §"Execution preconditions"; preamble enumerates
the 3 anticipated ADRs (ADR-0125, ADR-0126, ADR-0127 v2) + the 11
planner-time decisions resolution (D1-D4 from SPEC §12 + 7 PLAN-emerging
items). No ADR landed in Task 1; ADRs land at first-use commit per
PLAN's "ADRs introduced by this plan" table.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Verify:

```bash
git log -1 --format=%H -- docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md
                                                                       # expect: the just-committed SHA
git status --porcelain                                                 # expect: empty
```

---

## Task 2: `internal/filter/http/buffer/` package — `doc.go` + `buffer.go` skeleton (TypeURL, types, compiledConfig + compiledPerRoute + parsePerRoute + New factory PGV-mirror) + `buffer_test.go` Group 1 + Group 2 tests [ADR-0125, ADR-0126]

**Files:**
- Create: `internal/filter/http/buffer/doc.go`
- Create: `internal/filter/http/buffer/buffer.go`
- Create: `internal/filter/http/buffer/buffer_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0125, ADR-0126)
- Modify: `docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md` (append Task 2 entry)

This task lands the package skeleton + `New` factory PGV-mirror + `parsePerRoute` per SPEC §6.1-§6.3. Per TDD discipline: write Group 1 + Group 2 tests FIRST; verify they FAIL (no package exists); then land doc.go + buffer.go skeleton. ADR-0125 + ADR-0126 land at this commit per ADR-0044 ADR-on-impl convention.

**Precondition:** Task 1 commit on HEAD; pristine tree; `internal/filter/http/buffer/` does not exist.
**Artifacts:** doc.go, buffer.go (skeleton with `compiledConfig`, `compiledPerRoute`, `filter`, `TypeURL`, `New`, `parsePerRoute`, stubs for `DecodeHeaders` / `DecodeData` / `DecodeTrailers` / `SetDecoderCallbacks` / `OnDestroy`), buffer_test.go (Group 1 + 2), 2 new ADRs in DECISIONS.md, Task 2 PROGRESS entry.
**Acceptance:** `go build ./internal/filter/http/buffer/...` clean; `go vet ./internal/filter/http/buffer/...` clean; `golangci-lint run ./internal/filter/http/buffer/...` clean; `go test -race -count=1 -v ./internal/filter/http/buffer/` shows Group 1 + Group 2 tests PASS; ADR-0125 + ADR-0126 appear in DECISIONS.md with full Context/Decision/Consequences sections; Task 2 entry appended to PROGRESS.md.

- [ ] **Step 1: Write the failing tests (Group 1 + Group 2)**

Create `internal/filter/http/buffer/buffer_test.go` with 13 tests across 2 groups (Group 1 = 7 tests; Group 2 = 6 tests). The skeleton:

```go
package buffer

import (
	"errors"
	"strings"
	"testing"

	bufferv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// --- Group 1: New factory PGV-mirror ---

func TestNew_NilTC(t *testing.T) {
	_, err := New(nil, envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatal("expected error on nil typed_config")
	}
	if !strings.Contains(err.Error(), "buffer:") {
		t.Errorf("error wording missing 'buffer:' prefix: %v", err)
	}
}

func TestNew_MalformedTC(t *testing.T) {
	any := &anypb.Any{TypeUrl: TypeURL, Value: []byte("not-a-valid-proto")}
	_, err := New(any, envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatal("expected error on malformed typed_config")
	}
}

func TestNew_MaxRequestBytesNil_RejectAtParseTime(t *testing.T) {
	cfg := &bufferv3.Buffer{} // MaxRequestBytes nil
	any := mustMarshalAny(t, cfg)
	_, err := New(any, envoyhttp.FactoryCtx{})
	if err == nil || !strings.Contains(err.Error(), "max_request_bytes is required") {
		t.Errorf("expected 'max_request_bytes is required' error, got: %v", err)
	}
}

func TestNew_MaxRequestBytesZero_RejectAtParseTime(t *testing.T) {
	cfg := &bufferv3.Buffer{MaxRequestBytes: wrapperspb.UInt32(0)}
	any := mustMarshalAny(t, cfg)
	_, err := New(any, envoyhttp.FactoryCtx{})
	if err == nil || !strings.Contains(err.Error(), "must be > 0") {
		t.Errorf("expected 'must be > 0' error, got: %v", err)
	}
}

func TestNew_MaxRequestBytesOverCap_RejectAtParseTime(t *testing.T) {
	cases := []uint32{1048577, 2 * 1024 * 1024, 5 * 1024 * 1024}
	for _, v := range cases {
		v := v
		t.Run("", func(t *testing.T) {
			cfg := &bufferv3.Buffer{MaxRequestBytes: wrapperspb.UInt32(v)}
			any := mustMarshalAny(t, cfg)
			_, err := New(any, envoyhttp.FactoryCtx{})
			if err == nil || !strings.Contains(err.Error(), "exceeds envoy-go cap of 1048576 bytes") {
				t.Errorf("expected over-cap error for v=%d, got: %v", v, err)
			}
		})
	}
}

func TestNew_MaxRequestBytesBoundary_Accepted(t *testing.T) {
	cases := []uint32{1, 65536, 1048576}
	for _, v := range cases {
		v := v
		t.Run("", func(t *testing.T) {
			cfg := &bufferv3.Buffer{MaxRequestBytes: wrapperspb.UInt32(v)}
			any := mustMarshalAny(t, cfg)
			factory, err := New(any, envoyhttp.FactoryCtx{})
			if err != nil {
				t.Fatalf("expected accept for v=%d, got error: %v", v, err)
			}
			if factory == nil {
				t.Fatal("expected non-nil factory")
			}
		})
	}
}

func TestNew_HappyPath_Round(t *testing.T) {
	cfg := &bufferv3.Buffer{MaxRequestBytes: wrapperspb.UInt32(1024)}
	any := mustMarshalAny(t, cfg)
	factory, err := New(any, envoyhttp.FactoryCtx{})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	hf := factory()
	if hf.Decoder == nil || hf.Encoder != nil {
		t.Errorf("expected decoder-only HTTPFilter (Decoder!=nil, Encoder==nil), got %+v", hf)
	}
}

// --- Group 2: parsePerRoute PGV-mirror discipline ---

func TestParsePerRoute_Disabled_Parses(t *testing.T) {
	pr := &bufferv3.BufferPerRoute{Override: &bufferv3.BufferPerRoute_Disabled{Disabled: true}}
	cpr, err := parsePerRoute(pr)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if !cpr.disabled || cpr.maxOverride != nil {
		t.Errorf("expected disabled=true, maxOverride=nil; got %+v", cpr)
	}
}

func TestParsePerRoute_BufferOverride_Parses(t *testing.T) {
	pr := &bufferv3.BufferPerRoute{Override: &bufferv3.BufferPerRoute_Buffer{Buffer: &bufferv3.Buffer{MaxRequestBytes: wrapperspb.UInt32(65536)}}}
	cpr, err := parsePerRoute(pr)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if cpr.disabled || cpr.maxOverride == nil || *cpr.maxOverride != 65536 {
		t.Errorf("expected disabled=false, maxOverride=&65536; got %+v", cpr)
	}
}

func TestParsePerRoute_BufferOverride_Zero_Rejects(t *testing.T) {
	pr := &bufferv3.BufferPerRoute{Override: &bufferv3.BufferPerRoute_Buffer{Buffer: &bufferv3.Buffer{MaxRequestBytes: wrapperspb.UInt32(0)}}}
	_, err := parsePerRoute(pr)
	if err == nil || !strings.Contains(err.Error(), "must be > 0") {
		t.Errorf("expected zero-rejection, got: %v", err)
	}
}

func TestParsePerRoute_BufferOverride_OverCap_Rejects(t *testing.T) {
	pr := &bufferv3.BufferPerRoute{Override: &bufferv3.BufferPerRoute_Buffer{Buffer: &bufferv3.Buffer{MaxRequestBytes: wrapperspb.UInt32(5 * 1024 * 1024)}}}
	_, err := parsePerRoute(pr)
	if err == nil || !strings.Contains(err.Error(), "exceeds envoy-go cap") {
		t.Errorf("expected over-cap rejection, got: %v", err)
	}
}

func TestParsePerRoute_OneofUnset_Rejects(t *testing.T) {
	pr := &bufferv3.BufferPerRoute{} // Override nil
	_, err := parsePerRoute(pr)
	if err == nil || !strings.Contains(err.Error(), "override oneof is required") {
		t.Errorf("expected oneof-required rejection, got: %v", err)
	}
}

func TestParsePerRoute_DisabledFalse_Rejects(t *testing.T) {
	pr := &bufferv3.BufferPerRoute{Override: &bufferv3.BufferPerRoute_Disabled{Disabled: false}}
	_, err := parsePerRoute(pr)
	if err == nil || !strings.Contains(err.Error(), "disabled must be true") {
		t.Errorf("expected disabled-bool.const rejection, got: %v", err)
	}
}

// --- Helpers ---

func mustMarshalAny(t *testing.T, m proto.Message) *anypb.Any {
	t.Helper()
	a, err := anypb.New(m)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}
```

- [ ] **Step 2: Run tests; verify compile failure (package does not exist yet)**

```bash
go test -race -count=1 ./internal/filter/http/buffer/
# expect: build error — "package buffer is not in std" / undefined: TypeURL, New, parsePerRoute, etc.
```

- [ ] **Step 3: Author `doc.go`**

Create `internal/filter/http/buffer/doc.go` (~25 LoC) per the file-structure responsibility row. Mirror `internal/filter/http/csrf/doc.go` brevity. Doc comment enumerates: package surface (TypeURL + New); typed_config decomposition (1-actively-consumed `max_request_bytes`; 0-deferred at listener-level; per-route `BufferPerRoute` oneof); decoder-only `HTTPFilter` value (`Encoder: nil`); body-counting algorithm STREAMING-CAP ONLY per ADR-0127 v2; `maybeAddContentLength` mirror; per-route disabled-OR-override 5th canonical discipline per ADR-0125; SHARED-vacuous stats (no filter-specific counters); cross-cutting ADR anchors (ADR-0125 / ADR-0126 / ADR-0127 v2).

- [ ] **Step 4: Author `buffer.go` skeleton**

Create `internal/filter/http/buffer/buffer.go` (~150-180 LoC at this stage; the body-counting body lands in Tasks 3-4) per SPEC §6.1-§6.3 + §6.7. Includes:

```go
package buffer

import (
	"fmt"

	bufferv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.buffer.v3.Buffer"

const filterName = "envoy.filters.http.buffer"

const cap1MiB uint32 = 1 << 20 // 1048576; mirrors internal/filter/http/chain.go:19 filterBufferLimitBytes

type compiledConfig struct {
	maxRequestBytes uint32
}

type compiledPerRoute struct {
	disabled    bool
	maxOverride *uint32
}

type filter struct {
	config       *compiledConfig
	dcb          envoyhttp.DecoderFilterCallbacks
	effectiveMax uint32
	accumulated  uint32
	passthrough  bool
	headersRef   envoyhttp.RequestHeaderMap
}

func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	if tc == nil {
		return nil, fmt.Errorf("buffer: invalid typed_config: nil")
	}
	cfg := &bufferv3.Buffer{}
	if err := tc.UnmarshalTo(cfg); err != nil {
		return nil, fmt.Errorf("buffer: invalid typed_config: %w", err)
	}
	if cfg.GetMaxRequestBytes() == nil {
		return nil, fmt.Errorf("buffer: max_request_bytes is required")
	}
	v := cfg.GetMaxRequestBytes().GetValue()
	if v == 0 {
		return nil, fmt.Errorf("buffer: max_request_bytes must be > 0")
	}
	if v > cap1MiB {
		return nil, fmt.Errorf("buffer: max_request_bytes (%d) exceeds envoy-go cap of %d bytes", v, cap1MiB)
	}
	cc := &compiledConfig{maxRequestBytes: v}
	return func() envoyhttp.HTTPFilter {
		return envoyhttp.HTTPFilter{
			Name:     filterName,
			Decoder:  &filter{config: cc},
			Encoder:  nil,
			PerRoute: parsePerRoute,
		}
	}, nil
}

func parsePerRoute(perRoute proto.Message) (*compiledPerRoute, error) {
	cfg, ok := perRoute.(*bufferv3.BufferPerRoute)
	if !ok {
		return nil, fmt.Errorf("buffer per-route: expected *BufferPerRoute, got %T", perRoute)
	}
	switch override := cfg.GetOverride().(type) {
	case *bufferv3.BufferPerRoute_Disabled:
		if !override.Disabled {
			return nil, fmt.Errorf("buffer per-route: disabled must be true (PGV bool.const violation)")
		}
		return &compiledPerRoute{disabled: true}, nil
	case *bufferv3.BufferPerRoute_Buffer:
		if v := override.Buffer.GetMaxRequestBytes(); v == nil {
			return nil, fmt.Errorf("buffer per-route: max_request_bytes is required")
		} else if v.GetValue() == 0 {
			return nil, fmt.Errorf("buffer per-route: max_request_bytes must be > 0")
		} else if v.GetValue() > cap1MiB {
			return nil, fmt.Errorf("buffer per-route: max_request_bytes (%d) exceeds envoy-go cap of %d bytes", v.GetValue(), cap1MiB)
		} else {
			n := v.GetValue()
			return &compiledPerRoute{maxOverride: &n}, nil
		}
	case nil:
		return nil, fmt.Errorf("buffer per-route: override oneof is required (neither disabled nor buffer set)")
	default:
		return nil, fmt.Errorf("buffer per-route: unknown override case %T", override)
	}
}

// --- Filter method skeletons (bodies land in Tasks 3-4) ---

func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) {
	f.dcb = cb
}

func (f *filter) DecodeHeaders(headers envoyhttp.RequestHeaderMap, endStream bool) envoyhttp.FilterHeadersStatus {
	// Skeleton — body lands in Task 3.
	return envoyhttp.HeadersContinue
}

func (f *filter) DecodeData(data envoyhttp.BufferInstance, endStream bool) envoyhttp.FilterDataStatus {
	// Skeleton — body lands in Task 4.
	return envoyhttp.DataContinue
}

func (f *filter) DecodeTrailers(trailers envoyhttp.RequestTrailerMap) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

func (f *filter) OnDestroy() {}
```

NOTE on the actual `envoyhttp` interface names (`RequestHeaderMap`, `BufferInstance`, `RequestTrailerMap`, `DecoderFilterCallbacks`, `FilterHeadersStatus` enum names like `HeadersContinue`/`HeadersStopIteration`, `FilterDataStatus` enum names like `DataContinue`/`DataStopIterationAndBuffer`/`DataStopIterationNoBuffer`, `FilterTrailersStatus` enum like `TrailersContinue`, `OrderedHeaders`): these match `internal/filter/http/types.go` definitions per phase 12 csrf precedent. The implementer should consult `internal/filter/http/csrf/csrf.go` for verbatim symbol usage and adjust any name divergences (e.g., if the framework uses `Continue` instead of `HeadersContinue`, follow the existing precedent).

- [ ] **Step 5: Run tests; verify Group 1 + 2 PASS**

```bash
go build ./internal/filter/http/buffer/...
go vet ./internal/filter/http/buffer/...
golangci-lint run ./internal/filter/http/buffer/...
go test -race -count=1 -v ./internal/filter/http/buffer/
# expect: TestNew_NilTC PASS, TestNew_MalformedTC PASS, TestNew_MaxRequestBytesNil_RejectAtParseTime PASS,
# TestNew_MaxRequestBytesZero_RejectAtParseTime PASS, TestNew_MaxRequestBytesOverCap_RejectAtParseTime PASS (×3 subtests),
# TestNew_MaxRequestBytesBoundary_Accepted PASS (×3 subtests), TestNew_HappyPath_Round PASS,
# TestParsePerRoute_Disabled_Parses PASS, TestParsePerRoute_BufferOverride_Parses PASS,
# TestParsePerRoute_BufferOverride_Zero_Rejects PASS, TestParsePerRoute_BufferOverride_OverCap_Rejects PASS,
# TestParsePerRoute_OneofUnset_Rejects PASS, TestParsePerRoute_DisabledFalse_Rejects PASS
```

- [ ] **Step 6: Append ADR-0125 + ADR-0126 to DECISIONS.md**

Append both ADRs to `docs/envoy-go/DECISIONS.md` per the ADR-0001 7-section template. Each ADR's body details:

**ADR-0125: `internal/filter/http/buffer/` package shape — single-token directory + decoder-only `HTTPFilter` value + per-route disabled-OR-override 5th canonical discipline**

§Status: Accepted
§Date: 2026-05-09 (PROGRESS commit date)
§Doctrine: Phase-13 §9 family-row. ADR-0044 ADR-on-impl convention.
§Lands-in-task: Task 2 (this commit; package skeleton + parsePerRoute first lands).
§Context: Phase 13 lands `envoy.filters.http.buffer` as the 6th §9 production HTTP filter. The package shape decision (directory naming + file split + filter-struct + factory signature) builds on the cors / fault / header_mutation / localratelimit / csrf precedents. Per BRAINSTORM §1.1 + §2.1 + §2.5: directory `buffer/` (single-token; matches cors/fault/csrf precedent — no underscore needed since proto type-name is single token); decoder-only HTTPFilter value (mirrors phase 12 csrf ADR-0120; `Encoder: nil` saves StreamEncoderFilter method set); per-route TPFC carries `disabled: true` shortcut OR `buffer.max_request_bytes` wholesale override (5th canonical per-route discipline alongside ADR-0073 / ADR-0110 / ADR-0117 / ADR-0124).
§Decision:
  (i) Package directory + Go-package identifier are both `buffer` (single token; matches cors/fault/csrf precedent).
  (ii) Files: `doc.go`, `buffer.go`, `buffer_test.go`, `fuzz_test.go`. Single-file `buffer.go` per planner-time decision 6 (no `count.go` or `perroute.go` split).
  (iii) Public surface: `TypeURL` const + `New` HTTPFilterFactory.
  (iv) Decoder-only `HTTPFilter` value: `Decoder: f, Encoder: nil` (mirrors phase 12 csrf precedent; first-use in §9 family was phase 12).
  (v) Boot registration ordering: `router → buffer → cors → csrf → envoygotest → fault → header_mutation → localratelimit` (alphabetical-after-router per ADR-0100 §2.2 stylistic discipline).
  (vi) Per-route discipline: 5th canonical shape "disabled-OR-override" — `BufferPerRoute` proto carries `oneof override` with `disabled: true` (filter wholly inactive on route) OR `buffer: {max_request_bytes: ...}` (wholesale override of listener cap). `parsePerRoute` enforces oneof semantics + PGV-mirror constraints; "both fields set" rejection happens at JSON-decoder per §11.3 empirical pin (NOT PGV; rejection wording: `'buffer' has already been set (either directly or as part of a oneof)`).
  (vii) Per-route stats SHARED-vacuous with listener-level: phase 13 emits no filter-specific counters per SPEC §1.1 amendment 5; the SHARED-stats invariant from ADR-0124 carries through with the special case that "shared" is structurally vacuous when there is nothing to share.
§Alternatives considered: (a) split `buffer.go` into `count.go` + `perroute.go` — rejected per planner-time decision 6 (~280-330 LoC stays under mental-model threshold); (b) `Decoder: f, Encoder: f` symmetric — rejected per planner-time decision 5 (decoder-only is structurally honest); (c) per-route INDEPENDENT stats (mirroring phase 11 ADR-0117) — rejected because phase 13 emits no filter-specific counters at all per SPEC §11.5 empirical pin (reference Envoy emits no `envoy_http_buffer_*` counter family).
§Consequences:
  - Filter set extends from 7 → 8: `{buffer, cors, csrf, envoygotest, fault, header_mutation, localratelimit, router}`. ADR-0074 (filter set) absorbs the 8th member additively; no in-place edit.
  - Canonical 5-shape per-route table extends ADR-0073 + ADR-0117 + ADR-0124 with the disabled-OR-override sum-type; future per-route disciplines for additional §9 family rows can pick from the now 5-shape catalog or codify a new shape.
  - The "disabled-OR-override" sum-type is the FIRST per-route discipline to use a structural sum type (oneof) rather than a flat replace-all-fields wholesale override; the discriminator is the proto oneof case rather than a runtime type assertion. This sets a precedent for future per-route disciplines that need a "shortcut" alongside an "override" form.

**ADR-0126: `compiledConfig` shape + parse-time `max_request_bytes ≤ 1 MiB` validation + cap-layering rationale + PGV-mirror filter-internal validation discipline**

§Status: Accepted
§Date: 2026-05-09
§Doctrine: Phase-13 §9 family-row. ADR-0044 ADR-on-impl convention.
§Lands-in-task: Task 2.
§Context: Phase 13 consumes the only top-level field on the parent `Buffer` proto: `max_request_bytes` (UInt32Value, REQUIRED). Reference Envoy v1.37.2 accepts arbitrary `UInt32Value` up to ~4 GiB at parse time (confirmed at SPEC §11.1 empirical pin). envoy-go's framework safety net is the hardcoded `filterBufferLimitBytes = 1 << 20` (per `internal/filter/http/chain.go:19` + ADR-0076 §Decision (b)) which fires when `DataStopIterationAndBuffer` exceeds 1 MiB. Without an envoy-go-side parse-time ceiling, a config with `max_request_bytes > 1 MiB` would trip the framework's 17-byte 413 path BEFORE the buffer filter's own wire shape could fire — divergence-window. Per BRAINSTORM §2.3 + Q3 + §2.4: phase 13 imposes an envoy-go-only `max_request_bytes ≤ 1 MiB` parse-time validation that closes this divergence-window.
§Decision:
  (i) `compiledConfig` shape: 1 field — `maxRequestBytes uint32` (validated 0 < value ≤ 1048576). NO `*filterStats` field (phase 13 emits no filter-specific counters). NO additional state.
  (ii) Parse-time validation: `New` rejects nil `MaxRequestBytes`, rejects zero value, rejects values > 1048576 (1 MiB). Same validation applies to `BufferPerRoute.buffer.max_request_bytes` via `parsePerRoute`.
  (iii) Cap-layering rationale: buffer's own `accumulated > effectiveMax` check fires INSIDE the framework's `filterBufferLimitBytes = 1 << 20` cap because `effectiveMax ≤ 1 MiB` invariant per (ii). The framework cap stays armed as a safety net but is structurally unreachable in MVP. When the future cap-promotion phase amends ADR-0076's `filterBufferLimitBytes` to per-stream tunability (the `per_connection_buffer_limit_bytes` / `per_request_buffer_limit_bytes` knobs become honored rather than silent-ignored), this ADR amends in-place to remove the parse-time ≤ 1 MiB validation; `max_request_bytes` becomes operationally equivalent to reference Envoy.
  (iv) PGV-mirror error wording: envoy-go-own clear-text per planner-time decision 3 + phase 11 ADR-0115 + phase 12 ADR-0121 precedent. Specifically: `"buffer: max_request_bytes is required"`, `"buffer: max_request_bytes must be > 0"`, `"buffer: max_request_bytes (%d) exceeds envoy-go cap of 1048576 bytes"`. Per-route variants prepend `"buffer per-route: "`. NOT verbatim-Envoy-PGV-envelope mirroring (option (a) rejected — no canonical Envoy boot-log byte-equivalence target for proto-shape PGV checks; phase 11's 50ms `fill_interval` exception was a numeric-bound check with a canonical Envoy server.cc wording; phase 13's proto-shape check has no analogous mirror target).
§Alternatives considered:
  (a) Match-time-truncate-to-1-MiB: silent and surprises operator at runtime (no boot signal); rejected per BRAINSTORM §2.3 in favor of parse-time rejection.
  (b) No cap-layering: defer to framework cap entirely; rejected because the framework cap's wire shape is the framework's own 17-byte 413 path, NOT the buffer filter's per-spec response — divergence-window opens.
  (c) Match Envoy's PGV envelope wording verbatim: rejected per planner-time decision 3 (no boot-log byte-equivalence target; envoy-go-own wording is operator-friendlier).
§Consequences:
  - Operators with existing configs targeting `max_request_bytes > 1 MiB` against reference Envoy MUST adjust the value to load on envoy-go. Documented at BEHAVIOR_CONTRACT §13.4 `### Phase 13 forward-pointer notes`.
  - Future cap-promotion phase (compression's natural amender per ADR-0076 §Consequences (d)) amends THIS ADR in-place to remove the ≤ 1 MiB ceiling once the framework cap becomes per-stream tunable; `compiledConfig` field type may also widen from `uint32` to `uint64`.
  - The `compiledConfig` shape is the SMALLEST `compiledConfig` so far in §9 family rows (1 field vs csrf's 2 vs fault's 8). The minimal shape is the structurally honest one — buffer has no other state to carry.

- [ ] **Step 7: Append Task 2 entry to PROGRESS.md**

Append a Task 2 section to PROGRESS.md with **Commits / Notes / Outputs** subsections per the phase 12 PROGRESS template. Verbatim go test outputs go in Outputs.

- [ ] **Step 8: Commit**

```bash
git add internal/filter/http/buffer/ docs/envoy-go/DECISIONS.md docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 13: buffer package skeleton + New factory PGV-mirror + parsePerRoute oneof discipline [ADR-0125, ADR-0126]

Lands internal/filter/http/buffer/{doc.go,buffer.go,buffer_test.go} per
SPEC §6.1-§6.3. New factory PGV-mirrors max_request_bytes (non-nil + > 0
+ ≤ 1 MiB per ADR-0126); parsePerRoute enforces BufferPerRoute oneof
discipline (Disabled / Buffer / nil PGV-required mirror / disabled:false
PGV bool.const mirror per §11.3 empirical pin). Decoder-only HTTPFilter
value (Encoder: nil) per planner-time decision 5 + phase 12 csrf
precedent. Body-counting bodies for DecodeHeaders + DecodeData land in
Tasks 3-4. ADR-0125 (package shape + 5th canonical per-route discipline)
+ ADR-0126 (compiledConfig + parse-time validation + cap-layering) land
at this commit per ADR-0044 ADR-on-impl convention.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Verify:

```bash
git log -1 --format=%H -- internal/filter/http/buffer/buffer.go      # expect: just-committed SHA
grep -nE '^## ADR-0125|^## ADR-0126' docs/envoy-go/DECISIONS.md       # expect: 2 matches
go test -race -count=1 ./internal/filter/http/buffer/                 # expect: 13 tests PASS
```

---

## Task 3: `DecodeHeaders` body — header-only fast-path + per-route disabled passthrough + bodied StopIteration + Group 3 tests [ADR-0127 v2]

**Files:**
- Modify: `internal/filter/http/buffer/buffer.go` (DecodeHeaders body + resolveEffective helper)
- Modify: `internal/filter/http/buffer/buffer_test.go` (append Group 3 tests)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0127 v2)
- Modify: `docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md`

This task lands the `DecodeHeaders` body per SPEC §6.4 + §11.6 + §11.10 + the `resolveEffective` helper. ADR-0127 v2 anchors at this commit because `DecodeHeaders`'s `StopIteration` on bodied + non-disabled requests is the algorithm's gateway entry point. Task 4 completes the body-counting mechanics in `DecodeData` + `maybeAddContentLength` without a new ADR.

**Precondition:** Task 2 commit on HEAD; Group 1 + 2 PASS; pristine tree.
**Artifacts:** Updated buffer.go (DecodeHeaders body + resolveEffective), Group 3 tests, ADR-0127 v2 in DECISIONS.md, PROGRESS Task 3 entry.
**Acceptance:** `go test -race -count=1 -v ./internal/filter/http/buffer/` shows Groups 1-3 PASS; ADR-0127 v2 appears in DECISIONS.md.

- [ ] **Step 1: Append Group 3 tests**

Append to `internal/filter/http/buffer/buffer_test.go`:

```go
// --- Group 3: DecodeHeaders ---

func TestDecodeHeaders_HeaderOnlyEndStream_Continue(t *testing.T) {
	cases := []string{"GET", "HEAD", "OPTIONS", "POST"} // POST with endStream=true on headers (rare but legal)
	for _, method := range cases {
		method := method
		t.Run(method, func(t *testing.T) {
			f := freshFilter(t, 1024)
			headers := newHeaders(map[string]string{":method": method})
			status := f.DecodeHeaders(headers, true) // endStream=true on headers
			if status != envoyhttp.HeadersContinue {
				t.Errorf("expected HeadersContinue on header-only %s; got %v", method, status)
			}
			if f.passthrough || f.headersRef != nil || f.effectiveMax != 0 {
				t.Errorf("expected zero state touch on header-only path; got passthrough=%v, headersRef=%v, effectiveMax=%d", f.passthrough, f.headersRef, f.effectiveMax)
			}
		})
	}
}

func TestDecodeHeaders_PerRouteDisabled_Continue_PassthroughSet(t *testing.T) {
	f := freshFilter(t, 1024)
	cb := newFakeCallbacks()
	cb.perRoute = &compiledPerRoute{disabled: true}
	f.dcb = cb
	headers := newHeaders(map[string]string{":method": "POST"})
	status := f.DecodeHeaders(headers, false)
	if status != envoyhttp.HeadersContinue {
		t.Errorf("expected HeadersContinue on per-route disabled; got %v", status)
	}
	if !f.passthrough {
		t.Error("expected passthrough flag set")
	}
}

func TestDecodeHeaders_BodiedNonDisabled_StopIteration_EffectiveMaxStored(t *testing.T) {
	f := freshFilter(t, 1024)
	cb := newFakeCallbacks() // perRoute nil → listener fallback
	f.dcb = cb
	headers := newHeaders(map[string]string{":method": "POST", "content-length": "512"})
	status := f.DecodeHeaders(headers, false)
	if status != envoyhttp.HeadersStopIteration {
		t.Errorf("expected HeadersStopIteration on bodied + non-disabled; got %v", status)
	}
	if f.passthrough {
		t.Error("expected passthrough flag NOT set")
	}
	if f.effectiveMax != 1024 {
		t.Errorf("expected effectiveMax=1024 (listener fallback); got %d", f.effectiveMax)
	}
	if f.headersRef == nil {
		t.Error("expected headersRef stored")
	}
}

func TestDecodeHeaders_BodiedPerRouteOverride_StopIteration_OverrideMaxStored(t *testing.T) {
	f := freshFilter(t, 1024)
	override := uint32(256)
	cb := newFakeCallbacks()
	cb.perRoute = &compiledPerRoute{maxOverride: &override}
	f.dcb = cb
	headers := newHeaders(map[string]string{":method": "POST", "content-length": "512"})
	status := f.DecodeHeaders(headers, false)
	if status != envoyhttp.HeadersStopIteration {
		t.Errorf("expected HeadersStopIteration on bodied + override; got %v", status)
	}
	if f.effectiveMax != 256 {
		t.Errorf("expected effectiveMax=256 (override wins); got %d", f.effectiveMax)
	}
}

func TestDecodeHeaders_DoesNotInspectContentLength(t *testing.T) {
	// §11.6 — even with absurd CL header, DecodeHeaders does NOT fast-fail.
	f := freshFilter(t, 1024)
	cb := newFakeCallbacks()
	f.dcb = cb
	headers := newHeaders(map[string]string{":method": "POST", "content-length": "99999999999"})
	status := f.DecodeHeaders(headers, false)
	if status != envoyhttp.HeadersStopIteration {
		t.Errorf("expected HeadersStopIteration (no CL fast-fail); got %v", status)
	}
	// SendLocalReply MUST NOT have been invoked.
	if cb.localReplyCount != 0 {
		t.Errorf("expected zero SendLocalReply calls in DecodeHeaders; got %d", cb.localReplyCount)
	}
}

// --- Helpers (extends test file) ---

func freshFilter(t *testing.T, maxBytes uint32) *filter {
	t.Helper()
	return &filter{config: &compiledConfig{maxRequestBytes: maxBytes}}
}

// newHeaders + newFakeCallbacks: see test helpers below; mirror phase 12 csrf_test.go fakeCallbacks pattern.
// fakeCallbacks records perRoute pointer + tracks SendLocalReply invocations; implementer adapts the exact
// envoyhttp.DecoderFilterCallbacks interface per existing precedent at internal/filter/http/csrf/csrf_test.go.
```

NOTE: the `newHeaders` + `newFakeCallbacks` helpers should mirror phase 12 csrf's `csrf_test.go` test-fakes (around `localReplyArgs` + `fakeCallbacks` per phase 12 PROGRESS Task 3). Implementer adapts per the existing test-helper precedent.

- [ ] **Step 2: Run tests; verify Group 3 FAILS (DecodeHeaders skeleton returns Continue everywhere)**

```bash
go test -race -count=1 -v ./internal/filter/http/buffer/
# expect: Groups 1+2 PASS; Group 3 — TestDecodeHeaders_HeaderOnlyEndStream_Continue PASS (skeleton happens to satisfy);
# TestDecodeHeaders_PerRouteDisabled_Continue_PassthroughSet FAIL (passthrough flag not set);
# TestDecodeHeaders_BodiedNonDisabled_StopIteration_EffectiveMaxStored FAIL (Continue vs StopIteration);
# TestDecodeHeaders_BodiedPerRouteOverride_StopIteration_OverrideMaxStored FAIL;
# TestDecodeHeaders_DoesNotInspectContentLength FAIL (Continue vs StopIteration; this one passes the no-localReply assertion vacuously since skeleton has no localReply either)
```

- [ ] **Step 3: Implement `DecodeHeaders` body + `resolveEffective` helper**

Replace the skeleton `DecodeHeaders` body in `buffer.go`:

```go
func (f *filter) DecodeHeaders(headers envoyhttp.RequestHeaderMap, endStream bool) envoyhttp.FilterHeadersStatus {
	// Step 1: Header-only fast-path (mirrors buffer_filter.cc:54-56).
	if endStream {
		return envoyhttp.HeadersContinue
	}
	// Step 2: Resolve effectiveMax + disabled.
	effectiveMax, disabled := f.resolveEffective(headers)
	// Step 3: Per-route disabled bypass (mirrors buffer_filter.cc:60-62).
	if disabled {
		f.passthrough = true
		return envoyhttp.HeadersContinue
	}
	// Step 4: Bodied + non-disabled — hold headers; signal StopIteration (mirrors buffer_filter.cc:67).
	f.effectiveMax = effectiveMax
	f.headersRef = headers
	return envoyhttp.HeadersStopIteration
}

func (f *filter) resolveEffective(headers envoyhttp.RequestHeaderMap) (effectiveMax uint32, disabled bool) {
	// Look up the per-route compiledPerRoute via the framework's RequestRouteConfig. Listener fallback applies if nil.
	if f.dcb == nil {
		return f.config.maxRequestBytes, false
	}
	rc := f.dcb.RequestRouteConfig()
	if rc == nil {
		return f.config.maxRequestBytes, false
	}
	resolved := rc.Resolve(filterName, /* routeIdx */ 0) // routeIdx convention per phase 12 csrf
	if resolved == nil {
		return f.config.maxRequestBytes, false
	}
	cpr, ok := resolved.(*compiledPerRoute)
	if !ok {
		return f.config.maxRequestBytes, false
	}
	if cpr.disabled {
		return 0, true
	}
	if cpr.maxOverride != nil {
		return *cpr.maxOverride, false
	}
	return f.config.maxRequestBytes, false
}
```

NOTE: the exact `RequestRouteConfig().Resolve(...)` call signature should match `internal/filter/http/csrf/csrf.go` precedent — adapt the argument list (filter name string + routeIdx) to whatever the framework's existing convention is per phase 12 csrf landing.

- [ ] **Step 4: Run tests; verify Groups 1-3 PASS**

```bash
go vet ./internal/filter/http/buffer/...
golangci-lint run ./internal/filter/http/buffer/...
go test -race -count=1 -v ./internal/filter/http/buffer/
# expect: all Group 1, 2, 3 tests PASS
```

- [ ] **Step 5: Append ADR-0127 v2 to DECISIONS.md**

Append per the ADR-0001 7-section template:

**ADR-0127 v2: Body-counting + 413-trigger algorithm — STREAMING-CAP-ONLY + `maybeAddContentLength` mirror + reuse of framework `SendLocalReply` 413 wire shape + 100-Continue addendum**

§Status: Accepted. (v1 was the BRAINSTORM-time hypothesis with Content-Length fast-fail; v2 retires that clause after empirical refutation at SPEC §11.6.)
§Date: 2026-05-09
§Doctrine: Phase-13 §9 family-row. ADR-0044 ADR-on-impl convention.
§Lands-in-task: Task 3 (DecodeHeaders gateway lands first; Task 4 completes DecodeData + maybeAddContentLength without a new ADR).
§Context: Phase 13's filter implements a per-stream body-cap with 413 emission on overflow. Reference Envoy v1.37.2's `buffer_filter.cc:50-67` does NOT inspect `Content-Length` in `decodeHeaders`; the cap fires only after data accumulates past the limit (confirmed at SPEC §11.6 — a request with `Content-Length: 6291456` and zero body bytes does NOT 413, the connection times out). The v2 algorithm is STREAMING-CAP ONLY: `DecodeHeaders` returns `StopIteration` on bodied + non-disabled requests; `DecodeData` accumulates via `DataStopIterationAndBuffer` per chunk; on `accumulated > effectiveMax` the filter calls `SendLocalReply(413, "Payload Too Large", connClose)` + returns `DataStopIterationNoBuffer`; on terminal `endStream=true` the filter invokes `maybeAddContentLength` (mirrors `buffer_filter.cc:91-97`) before returning `DataContinue`. The cap predicate is `>` strict (per SPEC §11.2 — body=1 MiB exact does NOT trip; body=1 MiB+1 byte → 413). The 413 wire shape is byte-equivalent to ADR-0076's framework synthesis (status 413, body `Payload Too Large` 17 bytes ASCII no LF, 4-header lowercase wire-form, `Connection: close`). The 100-Continue prefix on CL-known overflow probes is HCM/H1-codec discipline (NOT buffer-filter discipline) and is emitted independently of the buffer filter.
§Decision:
  (i) `DecodeHeaders` discipline: header-only `endStream=true` → `Continue` (mirrors `buffer_filter.cc:54-56`); per-route `disabled=true` → set `passthrough` flag + `Continue` (mirrors `buffer_filter.cc:60-62`); bodied + non-disabled → store `effectiveMax` + `headersRef`; return `StopIteration` (mirrors `buffer_filter.cc:67`). NO `Content-Length` inspection — per §11.6.
  (ii) `DecodeData` discipline: passthrough flag → `DataContinue` (filter never returns `DataStopIterationAndBuffer` on disabled-route path; framework safety-net cap never engages on disabled routes); accumulate via `f.accumulated += uint32(data.Len())`; cap predicate `>` strict (per §11.2); `accumulated > effectiveMax` → `SendLocalReply(413, "Payload Too Large", connClose)` + `DataStopIterationNoBuffer`; terminal `endStream=true` (within cap) → invoke `maybeAddContentLength`; return `DataContinue`; in-flight chunk → `DataStopIterationAndBuffer` (framework holds bytes per ADR-0076 §Decision (b)).
  (iii) `maybeAddContentLength` mirror: if `headersRef != nil` AND original request had no `Content-Length` → set `Content-Length: <accumulated>` on held headers + drop `Transfer-Encoding: chunked`. Mirrors `buffer_filter.cc:91-97` + observable at upstream boundary per SPEC §11.8-CL empirical evidence.
  (iv) 413 wire shape: byte-equivalent to ADR-0076's framework path — `SendLocalReply(413, []byte("Payload Too Large"), OrderedHeaders{{Name: "Connection", Value: "close"}})`. The body literal is the verbatim 17-byte ASCII `Payload Too Large` (no trailing newline; same as the framework constant `localReply413Body` at `internal/filter/http/chain.go:25`). 4-header lowercase wire-form: `content-length: 17`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`. Plus the user-supplied `Connection: close`.
  (v) 100-Continue addendum: HCM/H1-codec emits `HTTP/1.1 100 Continue` BEFORE the eventual 413 when the request includes `Expect: 100-continue` (which curl auto-injects for `--data-binary @<file>`). The buffer filter does NOT need to emit 100-Continue itself; the HCM does (already shipped in phase 04). Chunked encoding bypasses 100-Continue (curl does not auto-inject).
§Alternatives considered:
  (a) v1 BRAINSTORM hypothesis with Content-Length fast-fail in `DecodeHeaders`: REFUTED by §11.6 empirical pin; reference Envoy does not fast-fail on CL. v1 retired.
  (b) Delegate body-counting to HCM via a `setBufferLimit`-equivalent framework primitive: rejected per BRAINSTORM §2.4 + ADR-0076 §Consequences (d) — the framework has no per-stream cap-override primitive; introducing one is the future cap-promotion phase's responsibility. The deliberate divergence is recorded here with a forward-pointer to that phase.
  (c) Cap predicate `>=` (off-by-one): REFUTED by §11.2 empirical pin; reference Envoy uses strict `>`. v1 already used `>` correctly; preserved verbatim in v2.
  (d) Distinct buffer-filter 413 wire shape: rejected per BRAINSTORM §2.8 + §11.7 + §11.8 — reference Envoy reuses the stock 413 path; reusing the framework's 413 wire shape preserves byte-equivalence with no risk of divergence-window.
§Consequences:
  - The framework's `filterBufferLimitBytes = 1 << 20` cap stays armed as a safety net. With ADR-0126's parse-time `effectiveMax ≤ 1 MiB` invariant, the framework cap is structurally unreachable in MVP.
  - `maybeAddContentLength` is the ONLY observable upstream-side divergence between envoy-go's filter and reference Envoy's; the wire-side 413 + counter increments are byte-equivalent on every probe.
  - Phase 13 is the FIRST §9 family-row to use `DataStopIterationAndBuffer` outside the framework's own ADR-0076 path. The chain framework's accumulation discipline carries through unchanged; buffer's filter just returns the status-code instead of letting the framework's overflow path fire.
  - Future cap-promotion phase (compression's natural amender per ADR-0076 §Consequences (d)) may revisit (ii) — if the framework gains a per-stream cap-override primitive, buffer can delegate body-counting to HCM and emit only `StopIteration`; the wire outcomes stay the same. ADR-0127 v3 would record that landing.

- [ ] **Step 6: Append Task 3 entry to PROGRESS.md**

- [ ] **Step 7: Commit**

```bash
git add internal/filter/http/buffer/ docs/envoy-go/DECISIONS.md docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 13: buffer DecodeHeaders body — header-only fast-path + per-route disabled passthrough + bodied StopIteration [ADR-0127 v2]

Lands DecodeHeaders body + resolveEffective helper per SPEC §6.4 +
§11.6 + §11.10. Header-only endStream=true → Continue (mirrors
buffer_filter.cc:54-56); per-route disabled → set passthrough + Continue
(mirrors buffer_filter.cc:60-62); bodied + non-disabled → store
effectiveMax + headersRef + StopIteration (mirrors buffer_filter.cc:67).
NO Content-Length inspection per §11.6 empirical refutation of
BRAINSTORM §2.6 fast-fail hypothesis. Group 3 tests (5 cases) PASS.
ADR-0127 v2 (body-counting algorithm STREAMING-CAP-ONLY +
maybeAddContentLength mirror + 413 wire shape reuse + 100-Continue
addendum) lands at this commit per ADR-0044 ADR-on-impl convention; v2
numbering reflects the post-empirical-pin retirement of v1's CL
fast-fail clause.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Verify:

```bash
grep -nE '^## ADR-0127' docs/envoy-go/DECISIONS.md   # expect: 1 match (v2)
go test -race -count=1 ./internal/filter/http/buffer/ # expect: Groups 1-3 PASS
```

---

## Task 4: `DecodeData` body + `maybeAddContentLength` mirror + `DecodeTrailers` body + Group 4 + Group 5 + Group 6 unit tests

**Files:**
- Modify: `internal/filter/http/buffer/buffer.go` (DecodeData body + maybeAddContentLength + DecodeTrailers body)
- Modify: `internal/filter/http/buffer/buffer_test.go` (append Group 4 + 5 + 6)
- Modify: `docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md`

This task completes the body-counting algorithm + per-route integration validation. NO new ADR (ADR-0127 v2 already anchored at Task 3 covers DecodeData + maybeAddContentLength). Group 4 + 5 + 6 tests round out the unit-test coverage per SPEC §14.1.

**Precondition:** Task 3 commit on HEAD; Groups 1-3 PASS; pristine tree.
**Artifacts:** Updated buffer.go (DecodeData + maybeAddContentLength + DecodeTrailers bodies), Groups 4 + 5 + 6 tests, PROGRESS Task 4 entry.
**Acceptance:** All 6 unit-test groups PASS; race-test clean; `go vet` + `golangci-lint` clean.

- [ ] **Step 1: Append Group 4 + 5 + 6 tests**

Append to `buffer_test.go`:

```go
// --- Group 4: DecodeData accumulation + cap predicate ---

func TestDecodeData_PassthroughFlag_DataContinue(t *testing.T) {
	f := freshFilter(t, 1024)
	f.passthrough = true
	for i := 0; i < 5; i++ {
		status := f.DecodeData(newBuffer(make([]byte, 4096)), false)
		if status != envoyhttp.DataContinue {
			t.Errorf("expected DataContinue per chunk on passthrough; got %v", status)
		}
	}
}

func TestDecodeData_SingleChunkFits_EndStream_DataContinue(t *testing.T) {
	f := freshFilter(t, 1024)
	f.effectiveMax = 1024
	f.headersRef = newHeaders(map[string]string{"content-length": "512"})
	status := f.DecodeData(newBuffer(make([]byte, 512)), true) // endStream=true; fits
	if status != envoyhttp.DataContinue {
		t.Errorf("expected DataContinue on single fit chunk; got %v", status)
	}
}

func TestDecodeData_SingleChunkExactCap_EndStream_DataContinue(t *testing.T) {
	// §11.2 — predicate is `>` strict; accumulated == effectiveMax must NOT trip.
	f := freshFilter(t, 1024)
	f.effectiveMax = 1024
	f.headersRef = newHeaders(map[string]string{"content-length": "1024"})
	status := f.DecodeData(newBuffer(make([]byte, 1024)), true)
	if status != envoyhttp.DataContinue {
		t.Errorf("expected DataContinue on exact-cap fit; got %v", status)
	}
}

func TestDecodeData_SingleChunkOverflow_413_StopIterationNoBuffer(t *testing.T) {
	f := freshFilter(t, 1024)
	f.effectiveMax = 1024
	f.headersRef = newHeaders(map[string]string{"content-length": "2048"})
	cb := newFakeCallbacks()
	f.dcb = cb
	status := f.DecodeData(newBuffer(make([]byte, 2048)), false)
	if status != envoyhttp.DataStopIterationNoBuffer {
		t.Errorf("expected DataStopIterationNoBuffer on overflow; got %v", status)
	}
	if cb.localReplyCount != 1 {
		t.Fatalf("expected 1 SendLocalReply call; got %d", cb.localReplyCount)
	}
	if cb.localReplyArgs.status != 413 {
		t.Errorf("expected status 413; got %d", cb.localReplyArgs.status)
	}
	if string(cb.localReplyArgs.body) != "Payload Too Large" {
		t.Errorf("expected body 'Payload Too Large'; got %q", cb.localReplyArgs.body)
	}
	if !cb.localReplyArgs.hasConnectionClose() {
		t.Error("expected Connection: close header")
	}
}

func TestDecodeData_MultiChunkBelowCap_StopIterationAndBuffer_TerminalContinue(t *testing.T) {
	f := freshFilter(t, 1024)
	f.effectiveMax = 1024
	f.headersRef = newHeaders(map[string]string{"content-length": "512"})
	// Chunks A=200, B=200, terminal C=112; total=512 < cap.
	if got := f.DecodeData(newBuffer(make([]byte, 200)), false); got != envoyhttp.DataStopIterationAndBuffer {
		t.Errorf("expected DataStopIterationAndBuffer on chunk A; got %v", got)
	}
	if got := f.DecodeData(newBuffer(make([]byte, 200)), false); got != envoyhttp.DataStopIterationAndBuffer {
		t.Errorf("expected DataStopIterationAndBuffer on chunk B; got %v", got)
	}
	if got := f.DecodeData(newBuffer(make([]byte, 112)), true); got != envoyhttp.DataContinue {
		t.Errorf("expected DataContinue on terminal chunk; got %v", got)
	}
	if f.accumulated != 512 {
		t.Errorf("expected accumulated=512; got %d", f.accumulated)
	}
}

func TestDecodeData_MultiChunkOverflowMidStream_413(t *testing.T) {
	f := freshFilter(t, 1024)
	f.effectiveMax = 1024
	f.headersRef = newHeaders(map[string]string{"content-length": "2048"})
	cb := newFakeCallbacks()
	f.dcb = cb
	if got := f.DecodeData(newBuffer(make([]byte, 800)), false); got != envoyhttp.DataStopIterationAndBuffer {
		t.Errorf("expected DataStopIterationAndBuffer on chunk 1 (under cap); got %v", got)
	}
	// Second chunk pushes accumulated past cap.
	if got := f.DecodeData(newBuffer(make([]byte, 400)), false); got != envoyhttp.DataStopIterationNoBuffer {
		t.Errorf("expected DataStopIterationNoBuffer on overflow; got %v", got)
	}
	if cb.localReplyCount != 1 {
		t.Errorf("expected 1 SendLocalReply call; got %d", cb.localReplyCount)
	}
}

func TestDecodeData_EmptyTerminalChunk_DataContinue(t *testing.T) {
	// §11.11 empty-body POST disposition.
	f := freshFilter(t, 1024)
	f.effectiveMax = 1024
	f.headersRef = newHeaders(map[string]string{"content-length": "0"})
	status := f.DecodeData(newBuffer(nil), true)
	if status != envoyhttp.DataContinue {
		t.Errorf("expected DataContinue on empty terminal; got %v", status)
	}
}

// --- Group 5: maybeAddContentLength mirror ---

func TestMaybeAddContentLength_NoOriginalCL_InjectsCL_DropsTransferEncoding(t *testing.T) {
	f := freshFilter(t, 1024)
	f.headersRef = newHeaders(map[string]string{"transfer-encoding": "chunked"})
	f.accumulated = 10240
	f.maybeAddContentLength()
	if got := f.headersRef.Get("content-length"); got != "10240" {
		t.Errorf("expected content-length=10240; got %q", got)
	}
	if got := f.headersRef.Get("transfer-encoding"); got != "" {
		t.Errorf("expected transfer-encoding dropped; got %q", got)
	}
}

func TestMaybeAddContentLength_OriginalCLPresent_NoOp(t *testing.T) {
	f := freshFilter(t, 1024)
	f.headersRef = newHeaders(map[string]string{"content-length": "512"})
	f.accumulated = 512
	f.maybeAddContentLength()
	if got := f.headersRef.Get("content-length"); got != "512" {
		t.Errorf("expected content-length unchanged at 512; got %q", got)
	}
}

func TestMaybeAddContentLength_HeadersRefNil_NoOp(t *testing.T) {
	f := freshFilter(t, 1024)
	f.headersRef = nil // disabled or header-only paths leave headersRef unset
	f.accumulated = 1024
	f.maybeAddContentLength() // must not panic
}

func TestMaybeAddContentLength_Idempotent(t *testing.T) {
	f := freshFilter(t, 1024)
	f.headersRef = newHeaders(map[string]string{"transfer-encoding": "chunked"})
	f.accumulated = 10240
	f.maybeAddContentLength()
	f.maybeAddContentLength() // second call: original-CL is now present (just-injected); no double-injection.
	if got := f.headersRef.Get("content-length"); got != "10240" {
		t.Errorf("expected content-length=10240 idempotent; got %q", got)
	}
}

// --- Group 6: Per-route integration ---

func TestPerRoute_ListenerFallback_AppliesWhenPerRouteNil(t *testing.T) {
	f := freshFilter(t, 1024)
	cb := newFakeCallbacks() // perRoute nil
	f.dcb = cb
	headers := newHeaders(map[string]string{":method": "POST", "content-length": "512"})
	f.DecodeHeaders(headers, false)
	if f.effectiveMax != 1024 {
		t.Errorf("expected listener fallback effectiveMax=1024; got %d", f.effectiveMax)
	}
}

func TestPerRoute_OverrideSmaller_FiresAtSmallerCap(t *testing.T) {
	f := freshFilter(t, 1024)
	override := uint32(256)
	cb := newFakeCallbacks()
	cb.perRoute = &compiledPerRoute{maxOverride: &override}
	f.dcb = cb
	f.DecodeHeaders(newHeaders(map[string]string{":method": "POST", "content-length": "512"}), false)
	// Now drive 300 bytes past the 256-byte cap.
	status := f.DecodeData(newBuffer(make([]byte, 300)), false)
	if status != envoyhttp.DataStopIterationNoBuffer {
		t.Errorf("expected 413 at 300 bytes vs 256 cap; got %v", status)
	}
}

func TestPerRoute_OverrideLarger_FiresAtLargerCap(t *testing.T) {
	f := freshFilter(t, 256)
	override := uint32(1024)
	cb := newFakeCallbacks()
	cb.perRoute = &compiledPerRoute{maxOverride: &override}
	f.dcb = cb
	f.DecodeHeaders(newHeaders(map[string]string{":method": "POST", "content-length": "768"}), false)
	// Listener cap 256 would have fired at 256; override raises to 1024 → 768 fits.
	status := f.DecodeData(newBuffer(make([]byte, 768)), true)
	if status != envoyhttp.DataContinue {
		t.Errorf("expected DataContinue at 768 vs 1024 override cap; got %v", status)
	}
}

func TestPerRoute_DisabledBypassesCap(t *testing.T) {
	f := freshFilter(t, 1024)
	cb := newFakeCallbacks()
	cb.perRoute = &compiledPerRoute{disabled: true}
	f.dcb = cb
	f.DecodeHeaders(newHeaders(map[string]string{":method": "POST"}), false)
	if !f.passthrough {
		t.Fatal("expected passthrough flag set")
	}
	// Drive 8 MiB through DecodeData; passthrough returns DataContinue per chunk.
	for i := 0; i < 8; i++ {
		status := f.DecodeData(newBuffer(make([]byte, 1024*1024)), false)
		if status != envoyhttp.DataContinue {
			t.Errorf("expected DataContinue per MiB on disabled-route; got %v at chunk %d", status, i)
		}
	}
	if cb.localReplyCount != 0 {
		t.Errorf("expected zero SendLocalReply on disabled-route; got %d", cb.localReplyCount)
	}
}

func TestPerRoute_ResolveCalledOncePerStream(t *testing.T) {
	f := freshFilter(t, 1024)
	cb := newFakeCallbacks()
	f.dcb = cb
	headers := newHeaders(map[string]string{":method": "POST", "content-length": "512"})
	f.DecodeHeaders(headers, false)
	f.DecodeData(newBuffer(make([]byte, 100)), false)
	f.DecodeData(newBuffer(make([]byte, 100)), false)
	f.DecodeData(newBuffer(make([]byte, 312)), true)
	if cb.resolveCount != 1 {
		t.Errorf("expected exactly 1 RequestRouteConfig.Resolve call; got %d", cb.resolveCount)
	}
}
```

NOTE: helpers (`newBuffer`, `localReplyArgs.hasConnectionClose`, `fakeCallbacks.resolveCount`, `fakeCallbacks.perRoute`) extend the test-helper set from Task 3 + phase 12 csrf precedent. `newBuffer` constructs a `envoyhttp.BufferInstance` from a `[]byte` (the framework's interface; consult `internal/filter/http/csrf/csrf_test.go` for the precedent).

- [ ] **Step 2: Run tests; verify Groups 4+5+6 FAIL (DecodeData + maybeAddContentLength skeletons)**

```bash
go test -race -count=1 -v ./internal/filter/http/buffer/
# expect: Groups 1+2+3 PASS; Group 4+5+6 FAIL (DecodeData skeleton returns DataContinue everywhere; maybeAddContentLength does not exist; resolve-count tracking unimplemented)
```

- [ ] **Step 3: Implement `DecodeData` body + `maybeAddContentLength` + `DecodeTrailers`**

Replace the `DecodeData` skeleton + add `maybeAddContentLength` + `DecodeTrailers` body in `buffer.go`:

```go
func (f *filter) DecodeData(data envoyhttp.BufferInstance, endStream bool) envoyhttp.FilterDataStatus {
	// Step 1: Per-route disabled bypass.
	if f.passthrough {
		return envoyhttp.DataContinue
	}
	// Step 2: Accumulate.
	f.accumulated += uint32(data.Len())
	// Step 3: Mid-stream overflow — emit 413 + discard partial buffer.
	if f.accumulated > f.effectiveMax {
		f.dcb.SendLocalReply(413, []byte("Payload Too Large"), envoyhttp.OrderedHeaders{
			{Name: "Connection", Value: "close"},
		})
		return envoyhttp.DataStopIterationNoBuffer
	}
	// Step 4: Terminal chunk fits — invoke maybeAddContentLength; release held headers + body.
	if endStream {
		f.maybeAddContentLength()
		return envoyhttp.DataContinue
	}
	// Step 5: In-flight chunk — accumulate; framework holds bytes per ADR-0076 §Decision (b).
	return envoyhttp.DataStopIterationAndBuffer
}

func (f *filter) maybeAddContentLength() {
	// Mirrors buffer_filter.cc:91-97 verbatim.
	if f.headersRef != nil && f.headersRef.Get("content-length") == "" {
		f.headersRef.Set("content-length", fmt.Sprintf("%d", f.accumulated))
		// Per §11.8 empirical evidence: Envoy ALSO drops Transfer-Encoding: chunked.
		f.headersRef.Remove("transfer-encoding")
	}
}

func (f *filter) DecodeTrailers(trailers envoyhttp.RequestTrailerMap) envoyhttp.FilterTrailersStatus {
	// Defensive: invoke maybeAddContentLength in case end-stream arrived via trailers, not via terminal endStream=true on data.
	f.maybeAddContentLength()
	return envoyhttp.TrailersContinue
}
```

NOTE: the exact `envoyhttp.OrderedHeaders` shape + `SendLocalReply` argument list should mirror phase 12 csrf precedent at `internal/filter/http/csrf/csrf.go`'s reject-path. If the framework's signature differs (e.g., positional vs. struct args), adapt accordingly.

- [ ] **Step 4: Run tests; verify all 6 groups PASS**

```bash
go vet ./internal/filter/http/buffer/...
golangci-lint run ./internal/filter/http/buffer/...
go test -race -count=1 -v ./internal/filter/http/buffer/
# expect: all Groups 1-6 PASS (~26 test functions across the 6 groups, with subtests bringing leaf count to ~30-35)
```

- [ ] **Step 5: Append Task 4 entry to PROGRESS.md**

- [ ] **Step 6: Commit**

```bash
git add internal/filter/http/buffer/ docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 13: buffer DecodeData body + maybeAddContentLength mirror + DecodeTrailers + Groups 4-6 tests

Completes the body-counting algorithm per ADR-0127 v2 (already anchored
at Task 3): DecodeData accumulates via DataStopIterationAndBuffer,
fires 413 + DataStopIterationNoBuffer on accumulated > effectiveMax
(strict > predicate per §11.2), invokes maybeAddContentLength on
terminal endStream=true. maybeAddContentLength mirrors buffer_filter.cc
:91-97 — sets Content-Length: <accumulated> + drops Transfer-Encoding:
chunked when original request had no Content-Length (chunked → fixed-CL
conversion observable at upstream per §11.8-CL). DecodeTrailers
defensively invokes maybeAddContentLength. Groups 4 (7 tests), 5 (4
tests), 6 (5 tests) all PASS under -race -count=1.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Verify:

```bash
go test -race -count=1 ./internal/filter/http/buffer/    # expect: all 6 groups PASS
```

---

## Task 5: `FuzzBufferConfigParse` fuzzer

**Files:**
- Create: `internal/filter/http/buffer/fuzz_test.go`
- Modify: `docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md`

The 17th fuzzer per ADR-0018's "every parser/codec/filter ships a fuzzer" discipline. Mirrors phase 12 csrf's `FuzzCsrfPolicyConfigParse` shape.

**Precondition:** Task 4 commit on HEAD; pristine tree; all unit tests PASS.
**Artifacts:** `fuzz_test.go` (~60 LoC), PROGRESS Task 5 entry.
**Acceptance:** `go test -fuzz=FuzzBufferConfigParse -fuzztime=30s ./internal/filter/http/buffer/` runs clean (no panics, no `(nil, nil)`).

- [ ] **Step 1: Author `fuzz_test.go`**

```go
package buffer

import (
	"testing"

	bufferv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// FuzzBufferConfigParse fuzzes arbitrary byte sequences as the typed_config
// payload to New. Asserts: New returns either (factory, nil) OR (nil, error);
// never panics; never returns (nil, nil). Per ADR-0018 + SPEC §14.3.
//
// 17th fuzzer in the repo (post-12's 16th FuzzCsrfPolicyConfigParse).
func FuzzBufferConfigParse(f *testing.F) {
	// Seed corpus (well-formed + intentionally-rejected + malformed).
	for _, v := range []uint32{1, 1024, 1048576, 0, 5242880} {
		bytes, _ := proto.Marshal(&bufferv3.Buffer{MaxRequestBytes: wrapperspb.UInt32(v)})
		f.Add(bytes)
	}
	f.Add([]byte{})       // empty bytes: Unmarshal failure
	f.Add([]byte{0xff})   // single garbage byte
	f.Add([]byte("not-a-valid-proto"))

	f.Fuzz(func(t *testing.T, raw []byte) {
		any := &anypb.Any{TypeUrl: TypeURL, Value: raw}
		factory, err := New(any, envoyhttp.FactoryCtx{})
		if factory == nil && err == nil {
			t.Fatalf("New returned (nil, nil) — invariant violation")
		}
		if factory != nil && err != nil {
			t.Fatalf("New returned (factory, error) — invariant violation: %v", err)
		}
	})
}
```

- [ ] **Step 2: Run a brief fuzz pass to verify no immediate crashers**

```bash
go test -fuzz=FuzzBufferConfigParse -fuzztime=10s ./internal/filter/http/buffer/
# expect: clean exit; no crashers
```

- [ ] **Step 3: Append Task 5 entry to PROGRESS.md**

- [ ] **Step 4: Commit**

```bash
git add internal/filter/http/buffer/fuzz_test.go docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 13: FuzzBufferConfigParse — 17th fuzzer in repo

Adds the New-factory parser fuzzer per ADR-0018 + SPEC §14.3 + the per-
filter-phase precedent (cors / fault / header_mutation / localratelimit
/ csrf each ship one). 5 well-formed/intentionally-rejected seeds + 3
malformed-bytes seeds. Asserts the (factory, nil) ⊕ (nil, error)
invariant; no panics; no (nil, nil).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: `cmd/envoy-go/main.go` register `buffer.New` under `buffer.TypeURL`

**Files:**
- Modify: `cmd/envoy-go/main.go` (1 import line + 1 register line)
- Modify: `docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md`

Boot-time registration — the eighth `httpReg.Register` call, alphabetical-after-router insertion (between `router` at line 115 and `cors` at line 116).

**Precondition:** Task 5 commit on HEAD; pristine tree.
**Artifacts:** main.go updated, PROGRESS Task 6 entry.
**Acceptance:** `go build ./cmd/envoy-go/...` clean; `grep -cE 'httpReg.Register' cmd/envoy-go/main.go` returns 8.

- [ ] **Step 1: Edit `cmd/envoy-go/main.go`**

Add the import alphabetically:

```go
"github.com/esalaine/envoy-go/internal/filter/http/buffer"
```

Add the registration immediately after the `router` line (which is at `cmd/envoy-go/main.go:115`):

```go
httpReg.Register(router.TypeURL, router.New)
httpReg.Register(buffer.TypeURL, buffer.New)
httpReg.Register(cors.TypeURL, cors.New)
// ... existing csrf/envoygotest/fault/header_mutation/localratelimit lines unchanged ...
```

- [ ] **Step 2: Build the binary**

```bash
go build ./cmd/envoy-go/...
go vet ./cmd/envoy-go/...
golangci-lint run ./cmd/envoy-go/...
grep -cE 'httpReg.Register' cmd/envoy-go/main.go    # expect: 8
```

- [ ] **Step 3: Smoke test by booting envoy-go with a minimal buffer-using bootstrap**

(Optional smoke; the differential fixture in Tasks 7-11 is the load-bearing verification.) The smoke is to boot `envoy-go --config-path test/fixtures/0015-http-buffer/envoy-go.yaml` once Task 11 lands; for Task 6 we verify only that the binary builds.

- [ ] **Step 4: Append Task 6 entry to PROGRESS.md**

- [ ] **Step 5: Commit**

```bash
git add cmd/envoy-go/main.go docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 13: cmd/envoy-go register buffer.New under buffer.TypeURL

Eighth httpReg.Register call; alphabetical-after-router insertion
between router (line 115) and cors (line 116) per ADR-0125 boot-
ordering discipline. Filter set extends from {cors, csrf, envoygotest,
fault, header_mutation, localratelimit, router} to add `buffer` (8
filters total).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Fixture infrastructure — `BackendKind=HTTPBuffer` enum + runner spawn helper + driver stub

**Files:**
- Modify: `test/differential/fixture/fixture.go` (new `HTTPBuffer BackendKind = 12` enum value)
- Modify: `test/differential/runner_test.go` (blank-import + spawn helper + switch case)
- Create: `test/fixtures/0015-http-buffer/driver/driver.go` (stub that registers `0015-http-buffer` with `BackendKind=HTTPBuffer`; full body lands in Task 11)
- Modify: `docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md`

This task lands the runner-side scaffolding so that Tasks 8-11 can populate the fixture iteratively without breaking the runner's existing `runFixture` switch. The driver stub registers itself with the runner but returns no scenarios yet (Task 11 fills the body).

**Precondition:** Task 6 commit on HEAD; pristine tree.
**Artifacts:** fixture.go enum extension, runner_test.go updated, driver/driver.go stub, PROGRESS Task 7 entry.
**Acceptance:** `go build ./test/...` clean; `go test -count=1 -short ./test/differential/` green (no fixture 0015 test running yet because the stub returns no scenarios).

- [ ] **Step 1: Extend `BackendKind` enum**

In `test/differential/fixture/fixture.go`, add after `HTTPCsrf BackendKind = 11`:

```go
// HTTPBuffer is an out-of-process HTTP/1.1 backend: the runner spawns
// test/fixtures/0015-http-buffer/backends/backend.go on the pre-allocated
// port. The backend echoes the inbound request method + path + headers as a
// JSON object in the response body — load-bearing for fixture scenario 6's
// `Content-Length: 10240` assertion at the backend boundary per the
// `maybeAddContentLength` mirror per phase 13 SPEC §11.8-CL. Status 200;
// Content-Type: application/json. No TLS. Introduced by fixture
// 0015-http-buffer (phase 13 Task 7). Because the backend is a subprocess,
// the runner's in-process accept counter is NOT incremented.
HTTPBuffer BackendKind = 12
```

- [ ] **Step 2: Add `startHTTPBufferBackend` spawn helper + switch case in `runner_test.go`**

Mirror the `startHTTPCsrfBackend` pattern from phase 12. Add:

```go
import _ "github.com/esalaine/envoy-go/test/fixtures/0015-http-buffer/driver"

// (in the kind switch:)
case fixture.HTTPBuffer:
    cmd, err := startHTTPBufferBackend(ctx, repoRoot, port)
    // ...

func startHTTPBufferBackend(ctx context.Context, repoRoot string, port int) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "go", "run", "./test/fixtures/0015-http-buffer/backends", "--port", fmt.Sprintf("%d", port))
	cmd.Dir = repoRoot
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd, cmd.Start()
}
```

(Adapt argument types + Setpgid usage per the existing phase 12 helper; the implementer should verbatim-mirror phase 12's helper, substituting the fixture path.)

- [ ] **Step 3: Author `test/fixtures/0015-http-buffer/driver/driver.go` stub**

```go
// Package driver registers the 0015-http-buffer differential fixture.
//
// The full driver body lands in Task 11; this Task-7 stub registers the fixture
// with the runner so the BackendKind plumbing can be wired without breaking
// the runner's switch. The stub returns no scenarios; the runner's per-fixture
// test will SKIP until Task 11 populates the body.
package driver

import (
	"github.com/esalaine/envoy-go/test/differential/fixture"
)

type bufferDriver struct{}

func (d *bufferDriver) BackendCount() int                    { return 1 }
func (d *bufferDriver) BackendKind() fixture.BackendKind     { return fixture.HTTPBuffer }
// ... other Driver interface methods stub out per Task 11 ...

func init() {
	fixture.RegisterFixture("0015-http-buffer", &bufferDriver{})
}
```

NOTE: the exact `fixture.Driver` interface is established by phase 07.1+ precedent; the Task-7 stub may need to satisfy multiple methods (`ReferenceBootstrap`, `SubjectConfig`, `DriveReference`, `DriveSubject`) returning empty/zero values. The implementer should make the stub minimally compile + register but skip at runtime; Task 11 fills the bodies. The PLAN's intent is that Task 7 = "infrastructure only, no scenarios yet"; Task 11 = "scenarios + assertions land".

- [ ] **Step 4: Build + smoke**

```bash
go build ./test/...
go test -count=1 -short ./test/differential/    # expect: all existing fixtures green; 0015 either SKIP or not yet runnable
grep -cE 'HTTPBuffer' test/differential/fixture/fixture.go    # expect: ≥1 (the enum + comment refs)
```

- [ ] **Step 5: Append Task 7 entry to PROGRESS.md**

- [ ] **Step 6: Commit**

```bash
git add test/differential/ test/fixtures/0015-http-buffer/driver/ docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 13: fixture 0015 infrastructure — BackendKind=HTTPBuffer + runner spawn helper + driver stub

Lands runner-side scaffolding for the 0015-http-buffer differential
fixture. fixture.HTTPBuffer BackendKind = 12; startHTTPBufferBackend
spawn helper mirrors startHTTPCsrfBackend; runner_test.go blank-imports
the new driver. Driver stub registers the fixture but returns no
scenarios — Tasks 8-11 populate the backend, bootstraps, expectations,
and driver body iteratively.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Fixture 0015 — `backends/backend.go` (Go HTTP backend echoing inbound headers as JSON)

**Files:**
- Create: `test/fixtures/0015-http-buffer/backends/backend.go`
- Modify: `docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md`

A Go HTTP/1.1 backend that echoes inbound request method + path + headers as a JSON object (mirrors SPEC §11.5 python `BaseHTTPRequestHandler` echo). Load-bearing for fixture scenario 6's `Content-Length: 10240` assertion at the backend boundary per the `maybeAddContentLength` mirror per §11.8-CL.

**Precondition:** Task 7 commit on HEAD; pristine tree.
**Artifacts:** backends/backend.go (~50 LoC), PROGRESS Task 8 entry.
**Acceptance:** `go run ./test/fixtures/0015-http-buffer/backends --port 18190` smoke; `curl -s -H "X-Test: foo" 127.0.0.1:18190/anypath` returns valid JSON with `headers["x-test"] == "foo"`.

- [ ] **Step 1: Author `backends/backend.go`**

```go
// Backend for fixture 0015-http-buffer. Echoes inbound request method + path + headers as JSON.
// Mirrors the python BaseHTTPRequestHandler echo at SPEC §11.5 empirical pin verbatim, in Go.
// Load-bearing for fixture scenario 6's Content-Length: 10240 assertion at the backend boundary
// per the maybeAddContentLength mirror per phase 13 SPEC §11.8-CL.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
)

func main() {
	port := flag.Int("port", 0, "listen port")
	flag.Parse()
	if *port == 0 {
		log.Fatal("--port required")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Read body to consume it (drains chunked + CL bodies; no echo of body bytes — only headers matter for fixture).
		buf := make([]byte, 4096)
		for {
			n, err := r.Body.Read(buf)
			if n == 0 || err != nil {
				break
			}
		}
		// Lowercase canonical header keys per Envoy wire-form discipline.
		hdrs := make(map[string]string, len(r.Header))
		for k, vs := range r.Header {
			if len(vs) > 0 {
				hdrs[strings.ToLower(k)] = vs[0]
			}
		}
		resp := map[string]any{
			"method":  r.Method,
			"path":    r.URL.Path,
			"headers": hdrs,
		}
		body, err := json.Marshal(resp)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		w.WriteHeader(200)
		_, _ = w.Write(body)
	})
	addr := fmt.Sprintf(":%d", *port)
	log.Printf("0015-http-buffer backend listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 2: Smoke test**

```bash
go run ./test/fixtures/0015-http-buffer/backends --port 18190 &
PID=$!
sleep 1
curl -s -H "X-Test: foo" -d "@-" -H "Content-Type: text/plain" http://127.0.0.1:18190/somepath </dev/null
kill $PID
# expect: JSON body with "method":"POST", "path":"/somepath", "headers":{"x-test":"foo", "content-type":"text/plain", ...}
```

- [ ] **Step 3: Append Task 8 entry to PROGRESS.md**

- [ ] **Step 4: Commit**

```bash
git add test/fixtures/0015-http-buffer/backends/ docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 13: fixture 0015 backend — Go HTTP echo serving inbound headers as JSON

Mirrors SPEC §11.5 python BaseHTTPRequestHandler echo verbatim in Go.
Lowercase canonical header keys per Envoy wire-form discipline. Status
200; Content-Type: application/json. Load-bearing for fixture scenario
6's Content-Length: 10240 assertion at the backend boundary per the
maybeAddContentLength mirror per §11.8-CL.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Fixture 0015 — `envoy.yaml` + `envoy-go.yaml` bootstraps (single-listener topology per planner-time decision 6)

**Files:**
- Create: `test/fixtures/0015-http-buffer/envoy.yaml`
- Create: `test/fixtures/0015-http-buffer/envoy-go.yaml`
- Modify: `docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md`

Reference Envoy + envoy-go bootstraps. Single listener `l_main` with three routes (`/`, `/route-disabled`, `/route-tighter`) per SPEC §7.2 + planner-time decision 6.

**Precondition:** Task 8 commit on HEAD; pristine tree.
**Artifacts:** Both YAML files (~70 LoC each), PROGRESS Task 9 entry.
**Acceptance:** Both bootstraps validate via `envoy --mode validate` (with templated ports substituted); `cmd/envoy-go` boots with envoy-go.yaml without error.

- [ ] **Step 1: Author `test/fixtures/0015-http-buffer/envoy.yaml`**

Reference Envoy bootstrap with:
- `admin: address: socket_address: { address: 127.0.0.1, port_value: <ADMIN_PORT> }`
- One listener `l_main` at `127.0.0.1:<LISTENER_PORT>`, HTTP/1.1 plaintext.
- HCM filter chain: `http_filters: [envoy.filters.http.buffer (max_request_bytes: 1048576), envoy.filters.http.router]`.
- One virtual_host `vh_main`, three routes:
  - `/route-disabled` — TPFC `envoy.filters.http.buffer: BufferPerRoute{disabled: true}`
  - `/route-tighter` — TPFC `envoy.filters.http.buffer: BufferPerRoute{buffer: {max_request_bytes: 131072}}`
  - `/` (default) — listener-level Buffer applies (no TPFC)
- One cluster `c_backend`, STRICT_DNS to `host.docker.internal:<BACKEND_PORT>` per ADR-0010.
- Templated placeholders: `<ADMIN_PORT>`, `<LISTENER_PORT>`, `<BACKEND_PORT>` (resolved by the runner at boot).

(~70 LoC mirroring 0014-http-csrf/envoy.yaml structure.)

- [ ] **Step 2: Author `test/fixtures/0015-http-buffer/envoy-go.yaml`**

Identical to envoy.yaml modulo:
- Cluster type: STATIC instead of STRICT_DNS (envoy-go's existing fixture pattern per ADR-0010).
- Same templated port placeholders.
- Same listener config + same per-route TPFC entries.

(~70 LoC.)

- [ ] **Step 3: Validate**

```bash
# Substitute placeholders for a smoke validation:
sed 's/<ADMIN_PORT>/19990/; s/<LISTENER_PORT>/11399/; s/<BACKEND_PORT>/18190/' test/fixtures/0015-http-buffer/envoy.yaml > /tmp/p13-validate.yaml
docker run --rm -v /tmp/p13-validate.yaml:/etc/envoy/envoy.yaml:ro envoyproxy/envoy:v1.37.2 --mode validate -c /etc/envoy/envoy.yaml
# expect: configuration '/etc/envoy/envoy.yaml' OK
```

```bash
# Smoke envoy-go bootstrap:
sed 's/<ADMIN_PORT>/19991/; s/<LISTENER_PORT>/11400/; s/<BACKEND_PORT>/18190/' test/fixtures/0015-http-buffer/envoy-go.yaml > /tmp/p13-go-validate.yaml
go run ./cmd/envoy-go --config-path /tmp/p13-go-validate.yaml &
PID=$!
sleep 2
kill $PID
# expect: clean exit (no panic, no parse error)
```

- [ ] **Step 4: Append Task 9 entry to PROGRESS.md**

- [ ] **Step 5: Commit**

```bash
git add test/fixtures/0015-http-buffer/envoy.yaml test/fixtures/0015-http-buffer/envoy-go.yaml docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 13: fixture 0015 bootstraps — envoy.yaml + envoy-go.yaml (single-listener, three routes)

Single listener l_main with three routes per planner-time decision 6:
/route-disabled (per-route TPFC disabled: true), /route-tighter (per-
route TPFC buffer.max_request_bytes: 131072), / (listener-level Buffer
max_request_bytes: 1048576). Reference Envoy uses STRICT_DNS cluster;
envoy-go uses STATIC per ADR-0010 fixture convention. Both bootstraps
validate cleanly.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Fixture 0015 — `expectations.yaml` + `README.md` (narrative-only documentation per ADR-0019)

**Files:**
- Create: `test/fixtures/0015-http-buffer/expectations.yaml`
- Create: `test/fixtures/0015-http-buffer/README.md`
- Modify: `docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md`

Prose narratives — `expectations.yaml` is documentation per ADR-0019 (NOT machine-evaluated; the driver enforces the assertions). `README.md` is the fixture overview.

**Precondition:** Task 9 commit on HEAD; pristine tree.
**Artifacts:** expectations.yaml (~30 LoC), README.md (~50 LoC), PROGRESS Task 10 entry.
**Acceptance:** Both files committed; structural review confirms each scenario's expected status, body, header set, counter delta is documented.

- [ ] **Step 1: Author `expectations.yaml`**

YAML-formatted prose. For each of the 6 scenarios per SPEC §7.1, document: scenario number, request line, expected response status, expected body bytes, expected header set, expected counter delta on the envoy-go side. Include the final counter snapshot. Cross-refs to SPEC sections + ADRs.

- [ ] **Step 2: Author `README.md`**

Markdown. Sections:
- Fixture overview + purpose.
- Topology (single listener, 3 routes).
- 6-scenario list with brief one-line summaries.
- `max_request_bytes ≤ 1 MiB` envoy-go-only divergence note (SPEC §2.1.1 + ADR-0126).
- `maybeAddContentLength` chunked → fixed-CL conversion note (SPEC §11.8-CL + ADR-0127 v2).
- Per-route disabled-OR-override 5th canonical discipline note (SPEC §1.3 + ADR-0125).
- 100-Continue addendum note (SPEC §11.8 + planner-time decision 10).
- Planner-time-decision cross-references (D1-D11).

- [ ] **Step 3: Append Task 10 entry to PROGRESS.md**

- [ ] **Step 4: Commit**

```bash
git add test/fixtures/0015-http-buffer/expectations.yaml test/fixtures/0015-http-buffer/README.md docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 13: fixture 0015 documentation — expectations.yaml + README.md

Prose narrative of the 6-scenario equivalence claims (per ADR-0019 —
expectations.yaml is documentation, not machine-evaluated; driver
enforces). README covers topology + max_request_bytes ≤ 1 MiB envoy-go
divergence + maybeAddContentLength chunked → fixed-CL conversion + per-
route disabled-OR-override 5th canonical discipline + 100-Continue
addendum + planner-time-decision cross-references.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: Fixture 0015 — `driver/driver.go` (single-listener 6-scenario sequential orchestration)

**Files:**
- Modify: `test/fixtures/0015-http-buffer/driver/driver.go` (replace stub from Task 7 with full body)
- Modify: `docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md`

The driver issues all 6 scenarios sequentially against both Envoy and envoy-go, asserts per-scenario equivalence, and scrapes `/stats/prometheus` for differential counter-delta comparison. Per planner-time decision 8 (chunked-body construction), decision 9 (backend JSON-echo + driver-side parse for scenario 6 CL-injection), decision 10 (Go stdlib transparent 100-Continue handling).

**Precondition:** Task 10 commit on HEAD; pristine tree; backend at Task 8 + bootstraps at Task 9 + expectations at Task 10 all present.
**Artifacts:** driver/driver.go (~180 LoC), PROGRESS Task 11 entry.
**Acceptance:** `go test -count=1 -v ./test/differential/ -run 'Test.*0015'` returns PASS for `0015-http-buffer`; per-scenario equivalence holds; final counter deltas equal between Envoy and envoy-go (after twin-series-discipline filtering).

- [ ] **Step 1: Replace driver stub with full body**

The full driver implements:
1. `package driver`; `init()` registers via `fixture.RegisterFixture("0015-http-buffer", &bufferDriver{})`.
2. `BackendCount() int` returns 1; `BackendKind() fixture.BackendKind` returns `fixture.HTTPBuffer`.
3. `ReferenceBootstrap(ports fixture.Ports) string` substitutes templated ports into `envoy.yaml` content; returns the rendered bootstrap.
4. `SubjectConfig(ports fixture.Ports) string` substitutes templated ports into `envoy-go.yaml` content; returns the rendered config.
5. `DriveReference(ctx, listenerPort) fixture.WireOutcome` issues the 6 scenarios against Envoy; captures status + body + headers per scenario; returns the WireOutcome.
6. `DriveSubject(ctx, listenerPort) fixture.WireOutcome` does the same against envoy-go.
7. Scenario implementations:
   - **Scenario 1** (POST `/` 1 KiB CL-known): `body := bytes.NewReader(make([]byte, 1024))`; `req, _ := http.NewRequestWithContext(ctx, "POST", "http://127.0.0.1:port/", body)`; assert `resp.StatusCode == 200`.
   - **Scenario 2** (POST `/` 2 MiB CL-known with Expect: 100-continue): `body := bytes.NewReader(make([]byte, 2*1024*1024))`; `req.Header.Set("Expect", "100-continue")`; assert `resp.StatusCode == 413`; assert response body byte-equals `"Payload Too Large"`; assert `resp.Header.Get("Content-Length") == "17"`; assert `resp.Header.Get("Connection") == "close"`.
   - **Scenario 3** (POST `/route-tighter` ~200 KiB chunked): `body := bytes.NewReader(make([]byte, 200*1024))`; `req.TransferEncoding = []string{"chunked"}` per planner-time decision 8; assert `resp.StatusCode == 413` + same wire shape as scenario 2 modulo no 100-Continue.
   - **Scenario 4** (POST `/route-disabled` 2 MiB CL-known): `body := bytes.NewReader(make([]byte, 2*1024*1024))`; assert `resp.StatusCode == 200`; assert response body is JSON-parseable backend echo.
   - **Scenario 5** (POST `/route-tighter` 200 KiB CL-known with Expect: 100-continue): mirror scenario 2 against `/route-tighter`; assert 413.
   - **Scenario 6** (POST `/` 10 KiB chunked, CL-injection assertion): `body := bytes.NewReader(make([]byte, 10*1024))`; `req.TransferEncoding = []string{"chunked"}`; assert `resp.StatusCode == 200`; **parse JSON body via `encoding/json`; assert `parsed.Headers["content-length"] == "10240"`** per planner-time decision 9 (load-bearing for §11.8-CL `maybeAddContentLength` byte-equivalence).
8. Final stats scrape: `GET /stats/prometheus` from both proxies; capture HCM `downstream_rq_*` counters under the `envoy_http_conn_manager_prefix="ingress_buffer"` label; compute deltas; assert envoy-go side's `downstream_rq_total +6`, `downstream_rq_2xx +3`, `downstream_rq_4xx +3`. Filter out Envoy-only `downstream_rq_too_large` + `downstream_rq_completed` per the existing twin-series-discipline allow-list. **Allow-list mechanism (per phase 06.1 SPEC §11.5 + phase 11 + 12 fixture precedent):** the differential runner enforces the allow-list automatically — it constructs the per-fixture expected-counter set from envoy-go's emit (the names appearing in envoy-go's `/stats/prometheus` scrape that match the project's 29-name table) and compares ONLY those names against the Envoy-side scrape; counters present only on the Envoy side (like `downstream_rq_too_large` + `downstream_rq_completed`) are silently ignored as expected non-overlap. The driver does NOT need explicit filter-out code — passing the raw scrape through is sufficient. Implementer verifies this by inspecting phase 12 0014-http-csrf/driver/driver.go's stats-comparison block, which similarly relies on the runner's automatic allow-list rather than per-driver filtering.

(~180 LoC. Mirror phase 12 csrf 0014-http-csrf/driver/driver.go structure.)

- [ ] **Step 2: Run the fixture**

```bash
go test -count=1 -v ./test/differential/ -run 'Test.*0015' 
# expect: PASS for 0015-http-buffer (~3-5 seconds wallclock)
```

- [ ] **Step 3: Run the full differential suite (regression check)**

```bash
go test -count=1 ./test/differential/
# expect: all 16 fixtures (0000-0015) PASS; ~50-60 seconds wallclock
```

- [ ] **Step 4: Append Task 11 entry to PROGRESS.md**

- [ ] **Step 5: Commit**

```bash
git add test/fixtures/0015-http-buffer/driver/driver.go docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 13: fixture 0015 driver — 6-scenario sequential orchestration

Replaces the Task 7 driver stub with the full body. 6 scenarios per
SPEC §7.1: body-fits (CL-known), streaming-overflow (CL-known +
Expect: 100-continue), chunked-overflow at per-route tighter cap, per-
route disabled bypass, per-route tighter override fires, chunked-
passthrough Content-Length injection (cross-cutting). Scenario 6 parses
JSON backend echo + asserts headers["content-length"]=="10240" per
planner-time decision 9 — load-bearing for §11.8-CL maybeAddContent
Length byte-equivalence. Final counter delta on envoy-go side:
downstream_rq_total +6, downstream_rq_2xx +3, downstream_rq_4xx +3.
Envoy-only downstream_rq_too_large (+3) + downstream_rq_completed (+6)
filtered out via existing twin-series-discipline allow-list. All 16
differential fixtures (0000-0015) green at ~50-60s wallclock.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: BEHAVIOR_CONTRACT.md 4-edit bundle + ROADMAP row 13 in-progress→done + STATE.md advance + 6-gate phase-done verification

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (4 in-place edits per SPEC §13)
- Modify: `docs/envoy-go/ROADMAP.md` (row 13 in-progress → done; sharpen summary text)
- Modify: `docs/envoy-go/STATE.md` (advance lifecycle to phase-done)
- Modify: `docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md`

This task lands Gate F (BEHAVIOR_CONTRACT populated) + the ROADMAP flip + STATE.md advance + the verbatim 6-gate verification. NO new ADR (all 3 ADRs landed at Tasks 2 + 3).

**Precondition:** Task 11 commit on HEAD; pristine tree; all unit + fuzzer + differential fixtures green.
**Artifacts:** BEHAVIOR_CONTRACT.md updated, ROADMAP.md updated, STATE.md advanced, PROGRESS Task 12 entry.
**Acceptance:** All 6 gates verbatim-pass per `BOOTSTRAP_PROMPT.md` §7.5: A (build/vet/lint clean), B (race-test pass on 36 packages), C (h2spec 53/53), D (17 fuzzers green at 30s), E (16 differential fixtures green), F (BEHAVIOR_CONTRACT populated).

- [ ] **Step 1: Apply BEHAVIOR_CONTRACT.md 4-edit bundle per SPEC §13**

The 4 edits are (per the verbatim Markdown patches in SPEC §13.1-§13.4):
- (a) NEW `### envoy.filters.http.buffer` subsection inserted under `## HTTP filter chain` umbrella, AFTER the existing `### envoy.filters.http.csrf` subsection at line 1093 (per SPEC §13.1; ~70 LoC verbatim from SPEC §13.1's Markdown block).
- (b) `## Stat-name mapping ### 29-name table` preamble note appended after the existing table (per SPEC §13.2; ~5 LoC verbatim — "Phase 13 (buffer filter) note: ...").
- (c) NEW row appended to `## Equivalence Matrix` table (per SPEC §13.3; ~3 LoC verbatim).
- (d) NEW `### Phase 13 forward-pointer notes` subsection appended to `## Forward-pointer notes` section (per SPEC §13.4; ~25 LoC verbatim).

The Markdown content for each patch is documented verbatim in SPEC §13 — copy verbatim with no edits.

- [ ] **Step 2: Apply ROADMAP.md row 13 flip + summary sharpening**

Row 13's status changes `in-progress → done` with a date column populated. The summary text is sharpened to align with SPEC §1.1 amendment 5 — "stays at 29 names" (NOT BRAINSTORM-time pre-amendment "29→31 names") + "3 ADRs" (NOT BRAINSTORM-time "4 ADRs"). The implementer crafts a verbatim row replacement preserving all the load-bearing facts (1 listener-level field consumed; envoy-go-only ≤ 1 MiB parse-time validation per ADR-0126; STREAMING-CAP-ONLY body-counting per ADR-0127 v2 + §11.6; per-route disabled-OR-override 5th canonical discipline per ADR-0125; SHARED-vacuous stats; ZERO new stat-table entries; 6-scenario differential fixture; 17th fuzzer; 3 ADRs ADR-0125+ADR-0126+ADR-0127 v2).

- [ ] **Step 3: Apply STATE.md advance**

Update `active-phase`, `phase-directory`, `lifecycle-state`, `next-skill`, `next-skill-scope`, `last-commit`, `last-updated`. Set `lifecycle-state` to `awaiting next planning` (per phase 12 phase-done STATE.md precedent); set `next-skill` to `superpowers:brainstorming` against §9's family list for the next family-child; set `active-phase` to a placeholder pending the next session's selection (e.g., `<next-§9-family-row>` resolved by the next planner).

- [ ] **Step 4: Run all 6 gates verbatim**

```bash
# Gate A: build + vet + lint clean
go build ./...
go vet ./...
golangci-lint run ./...

# Gate B: race-test pass on all 36 packages
go test -race -count=1 ./...
# expect: every package PASS, no -race violations

# Gate C: h2spec 53/53 PASS (per phases 04+ precedent; phase 13 introduces no HTTP/2 stack changes — regression check)
make h2spec    # or whatever the project's h2spec entry-point is
# expect: 53/53 PASS

# Gate D: 17 fuzzers green at 30s budget
for fuzzer in $(go test -list 'Fuzz.*' ./internal/... 2>/dev/null | grep -E '^Fuzz'); do
    pkg=$(go test -list "$fuzzer" ./internal/... 2>/dev/null | grep -B1 "$fuzzer" | head -1)
    go test -fuzz="$fuzzer" -fuzztime=30s "$pkg"
done
# expect: all 17 fuzzers (16 prior + new FuzzBufferConfigParse) clean exit

# Gate E: 16 differential fixtures 0000-0015 PASS
go test -count=1 -v ./test/differential/
# expect: all 16 PASS; total wallclock ~50-60s

# Gate F: BEHAVIOR_CONTRACT.md populated (verified by file diff)
git diff master -- docs/envoy-go/BEHAVIOR_CONTRACT.md | head -100
# expect: 4 patches landed
```

- [ ] **Step 5: Append Task 12 entry to PROGRESS.md**

Capture verbatim outputs for each of the 6 gates.

- [ ] **Step 6: Commit**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/ROADMAP.md docs/envoy-go/STATE.md docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 13: phase-done — BEHAVIOR_CONTRACT 4-edit bundle + ROADMAP row 13 done + STATE.md advance + 6 gates green

Lands Gate F per SPEC §13: NEW ### envoy.filters.http.buffer subsection
under ## HTTP filter chain (~70 LoC); 29-name table preamble note (NO
new rows — phase 13 contributes ZERO new stat-table entries per §11.5 +
§1.1 amendment 5); NEW Equivalence Matrix row pointing at fixture 0015;
NEW ### Phase 13 forward-pointer notes subsection (2-item deferral list
+ no-new-tag-extractor + per-route shared-vacuous + body-counting
divergence-from-Envoy notes). ROADMAP row 13 in-progress → done with
sharpened summary aligning to SPEC §1.1 amendment 5 (3 ADRs, stays at
29 names). STATE.md advances lifecycle to awaiting next planning.

All 6 gates green:
  A — build / vet / lint clean
  B — race-test pass on 36 packages
  C — h2spec 53/53 PASS
  D — 17 fuzzers green at 30s budget (16 prior + FuzzBufferConfigParse)
  E — 16 differential fixtures 0000-0015 PASS (~50-60s wallclock)
  F — BEHAVIOR_CONTRACT populated

3 ADRs anchored: ADR-0125 (Task 2; package + 5th canonical per-route),
ADR-0126 (Task 2; compiledConfig + parse-time validation), ADR-0127 v2
(Task 3; body-counting algorithm + maybeAddContentLength + 413 wire
reuse + 100-Continue addendum).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Verify:

```bash
grep -n '^### envoy.filters.http.buffer' docs/envoy-go/BEHAVIOR_CONTRACT.md   # expect: 1 match (newly inserted)
grep -nE '^\| 13 \| http-filter-buffer .* \| done' docs/envoy-go/ROADMAP.md   # expect: 1 match
go test -count=1 ./test/differential/                                          # expect: all 16 PASS
```

---

## Task 13: REVIEW.md — end-of-phase review per `superpowers:requesting-code-review` skill

**Files:**
- Create: `docs/envoy-go/phases/13-http-filter-buffer/REVIEW.md`
- Modify: `docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md`

End-of-phase review per the 06.1 / 06.2 / 07.1 / 07.2 / 08.1 / 08.2 / 09 / 10 / 11 / 12 cadence. Phase 13 has NO parent row (it is a top-level §9 family-child per ADR-0106), so REVIEW closes only row 13.

**Precondition:** Task 12 commit on HEAD; pristine tree; phase 13 is functionally complete + 6 gates green.
**Artifacts:** REVIEW.md, PROGRESS Task 13 entry.
**Acceptance:** REVIEW landed; row 13 closed; phase 13 lifecycle complete.

- [ ] **Step 1: Invoke the requesting-code-review skill**

The skill drives the REVIEW.md authoring. Per phase 12 REVIEW.md precedent, the document covers:
- §1: Phase summary (the 8 SPEC §4.1 deliverables + 3 SPEC §4.2 fixture deliverables; what was added vs. modified vs. retired).
- §2: ADR roster — ADR-0125 / ADR-0126 / ADR-0127 v2 anchored at Tasks 2 + 3 per ADR-0044.
- §3: Empirical pins outcome — all 11 §11 pins resolved at SPEC drafting; no new divergences during impl.
- §4: Gate-by-gate evidence (verbatim from PROGRESS Task 12 outputs).
- §5: Acceptance checklist confirmation (per SPEC §15 + this PLAN).
- §6: Forward-pointer roster (the 2-item BEHAVIOR_CONTRACT §13.4 deferrals).
- §7: Phase-done lessons learned (e.g., the post-landing BRAINSTORM §12 amendment as a release-valve precedent for cases where empirical findings invalidate the original hypothesis at scale; the v2-numbered ADR convention for retired-clause supersessions; the "ZERO new stat-table entries" outcome as a structurally-thinnest §9 row data point).

- [ ] **Step 2: Author REVIEW.md per skill output**

(~150 LoC mirroring phase 12 REVIEW.md structure.)

- [ ] **Step 3: Append Task 13 entry to PROGRESS.md**

- [ ] **Step 4: Commit**

```bash
git add docs/envoy-go/phases/13-http-filter-buffer/REVIEW.md docs/envoy-go/phases/13-http-filter-buffer/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 13: REVIEW — end-of-phase retrospective + N-1 carry-forward

End-of-phase review per the 06.1..12 cadence. Phase 13 closed row 13.
3 ADRs anchored (ADR-0125 + ADR-0126 + ADR-0127 v2); 16 differential
fixtures green; 17 fuzzers green; h2spec 53/53; build/vet/lint/race-
test all clean. The post-landing BRAINSTORM §12 amendment (D-3.5 in-
place) was a release-valve precedent for cases where empirical findings
require a substantial design re-frame; the v2-numbered ADR convention
(ADR-0127 v2 — retired the BRAINSTORM-time CL fast-fail clause)
formalizes the supersession discipline. ZERO new stat-table entries —
the structurally-thinnest §9 row at the stat surface (after phase 12
csrf's 3-counter additive expansion); buffer demonstrates that some
filter families have no observable on the filter-namespace at all,
relying entirely on HCM-namespace counters for differential equivalence.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## End of phase 13 implementation plan

13 tasks total. Production code ~283-333 LoC + tests ~460 LoC + fixture ~450 LoC + docs ~334 LoC ≈ ~1530-1580 LoC across all bundles. 3 ADRs landed (ADR-0125 + ADR-0126 + ADR-0127 v2). 6 gates green at phase-done. 16 differential fixtures (0000-0015) PASS. 17 fuzzers green at 30s budget. h2spec 53/53. Phase 13 row closed; §9 family heading at ROADMAP line 56 stays unchanged. Next §9 family-child is row 14 — selection deferred to the next BRAINSTORM session per ADR-0106 no-sibling-stub discipline.
