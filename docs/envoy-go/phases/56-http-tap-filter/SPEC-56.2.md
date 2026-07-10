# Phase 56.2 SPEC — the HTTP tap filter, bodies leg (`envoy.filters.http.tap`): body capture on the proven headers spine — `Message.Body` populated from `DecodeData`/`EncodeData`, the per-direction `max_buffered_rx_bytes`/`max_buffered_tx_bytes` caps, `Body.truncated`, and the now-DIVERGENT `JSON_BODY_AS_BYTES`-vs-`JSON_BODY_AS_STRING` render — the SECOND and FINAL leg of the NINTH Observability-family row (ANCHORS ADR-0274; flips row 56 `done` at its IMPL)

> **Lifecycle stage:** SPEC (lifecycle-state 1 → 2). Docs-only, worktree `.worktrees/phase-56.2-spec`, branch `phase-56.2-http-tap-filter-spec`. NO production code. Row 56 STAYS `in-progress` (flips `done` only at the 56.2 IMPL six-gate — `reference_roadmap_split_phase_row_done` + ADR-0106; no parent rollup).
>
> **The 56.1 headers leg is LANDED (`e954961f`) and is the FOUNDATION.** This leg adds ONLY the body-capture concern on top of a proven spine — no new package, no new seam, no new go.mod module, no new stat, no new BackendKind, no new fuzzer. The scope was chartered at the phase-56 BRAINSTORM *(Q6, by-concern split)* and re-affirmed at SPEC-56.1 §2. **ADR-0045's split gate is CONSUMED; 56.2 MUST NOT absorb 56.1 spillover.**
>
> **The §11 body-behavior pins were EXECUTED IN-SESSION 2026-07-10** against `envoyproxy/envoy:contrib-v1.37.2` (ADR-0227) on a Docker **bridge** network with `--concurrency 1`, a **FRESH probe container per arm** (`feedback_probe_fresh_container_per_arm`), a `mccutchen/go-httpbin` backend (`/range/N` = N deterministic UTF-8 bytes; `/bytes/N` = N random bytes; `/anything` echoes the POST body), and a `curlimages/curl` sidecar driver — exactly the SPEC-56.1 §11 method. **All body pins carry their verbatim reference output (§11).** They resolve every open body question — the truncation boundary, the default cap, the `truncated` render, the AS_BYTES base64 form, the non-UTF8 AS_STRING coverage boundary — and surface ONE new IMPL trap the charter never anticipated (§1.1 AMEND-TAP-BODY2-UTF8).

---

## 1. Purpose / Mission

Extend the landed tap filter (`internal/filter/http/tap`) to capture request and response **bodies**. 56.1 shipped a headers-only buffered-trace lifecycle over a deliberately body-free `GET → 204` stream (AMEND-TAP-BODY forced that shape so `body` was never rendered). 56.2 makes the two inert data hooks (`DecodeData`/`EncodeData`, currently `return DataContinue` pass-throughs, `tap.go:60-65`) BUFFER body bytes into the pending trace, honoring the per-direction byte caps, and populates `data/tap/v3.HttpBufferedTrace_Message.body` (f2) with a `data/tap/v3.Body`. The `JSON_BODY_AS_BYTES`-vs-`JSON_BODY_AS_STRING` `format` — parsed-and-accepted-but-INDISTINGUISHABLE at 56.1 (`config.go:113-115` accepts both, the body is never rendered) — now DIVERGES: `_AS_STRING` (=1) renders `{"as_string": "<utf8>"}`, `_AS_BYTES` (=0, the proto default) renders `{"as_bytes": "<base64>"}`. It touches ONE package (`internal/filter/http/tap`), adds the `0100-http-tap-bodies` differential, and flips row 56 `done` at its IMPL. Byte-identical when no tap filter is configured; the emit lifecycle, predicate engine, sink, `:status` synthesis, and reject roster are all UNCHANGED from 56.1.

This is the FINAL leg of phase 56. At its IMPL the Observability family's ninth row is complete (the family STAYS OPEN — its deferred-candidate list is non-empty).

### 1.1 Empirical-finding-driven scope — the `AMEND-TAP-BODY2-*` block (per ADR-0044)

Six body findings, ordered by consequence. **Five are PROBE-DERIVED** — AMEND-TAP-BODY2-{DEFAULTCAP, BOUNDARY, TRUNCFLAG, BYTES, UTF8} — each carrying its verbatim reference output in §11. **One is SOURCE-DERIVED** — AMEND-TAP-BODY2-CHUNKS — re-derived from the landed HCM data plumbing (no §11 subsection: it corrects the charter's reading of the framework, not the reference's behavior).

- **AMEND-TAP-BODY2-DEFAULTCAP (the unset cap is 1024, not "unbounded").** `OutputConfig.max_buffered_rx_bytes`/`max_buffered_tx_bytes` are `*wrapperspb.UInt32Value` (nil when unset). When unset the reference caps capture at **exactly 1024 bytes** (probe `tx-default-big`: a 2000-byte upstream body → `as_string` length **1024**, `truncated: true`). ⇒ envoy-go must apply a **1024-byte default** when the wrapper is nil, NOT capture unboundedly. A present wrapper (including value **0**) uses its value.
- **AMEND-TAP-BODY2-BOUNDARY (truncation is strict `>`, never `>=`).** `truncated` is set ONLY when the body length **exceeds** the cap. Measured three ways on a 10-byte body: cap **9** → `"abcdefghi"`, `truncated: true`; cap **10** (== body length) → `"abcdefghij"`, `truncated: **false**`; cap **11** → full, `truncated: false`. ⇒ the IMPL compares `capturedLen + len(chunk) > cap` (strict greater-than); a body EXACTLY at the cap is NOT truncated. This is the single most break-prone off-by-one of the leg (§8.4 break (c) targets it with a body-length-== -cap arm).
- **AMEND-TAP-BODY2-TRUNCFLAG (`truncated` is ALWAYS emitted, including `false`).** A full, non-truncated body renders `{"as_string": "abcdefghij", "truncated": false}` — the `truncated` bool (f3, `common.pb.go:42`) is emitted at its default `false` because the pinned marshal is `EmitDefaultValues` (AMEND-TAP-JSON, unchanged). This CONFIRMS the 56.1 marshal-option pin extends to the body case: a `Body` message renders `truncated` unconditionally. (SPEC-56.1 §11.9's abbreviated `{"as_bytes": "…"}` dropped the `truncated: false` for brevity; the full probe shows it present — §11.2.)
- **AMEND-TAP-BODY2-BYTES (`as_bytes` is standard base64 WITH padding — Go-native).** `JSON_BODY_AS_BYTES` renders the body as `{"as_bytes": "<StdEncoding base64 + padding>"}` — probe `tx-bytes-full`: `"abcdefghij"` → `"YWJjZGVmZ2hpag=="` (10 bytes → 16 chars incl. `==` padding), which is exactly `google.golang.org/protobuf`'s `bytes`-field rendering. ⇒ AS_BYTES needs no special handling: set the `Body_AsBytes` oneof arm to the captured `[]byte`, and `protojson` produces the reference form.
- **AMEND-TAP-BODY2-UTF8 (LOAD-BEARING NEW TRAP — non-UTF8 + AS_STRING is a coverage boundary Go cannot faithfully cross).** `JSON_BODY_AS_STRING` puts the raw bytes into the proto `string` field (`as_string`, f2). The reference's C++ JSON serializer tolerates invalid UTF-8 by MANGLING it (probe `tx-nonutf8`: 8 random bytes rendered `"as_string": " FH F "` — 6 chars, lossy, NOT the original bytes). **Go's `protojson.Marshal` REJECTS invalid UTF-8 in a proto3 `string` field and returns an error** — and the landed sink swallows a marshal error (`OnDestroy` does `_ = f.cfg.sink.write(...)`, `trace.go:65`), so a non-UTF8 AS_STRING body would silently drop the WHOLE trace. ⇒ TWO consequences: (1) the `0100` differential MUST drive UTF-8-safe bodies (`/range`-style ASCII) and UNassert non-UTF8 AS_STRING; (2) the IMPL must DECIDE how to handle a non-UTF8 body under AS_STRING (a §12 PLAN question: recommend documenting it as an explicit coverage boundary — envoy-go's AS_STRING assumes text bodies, per the proto's own intent — and NOT silently dropping the trace; the safest concrete choice is `strings.ToValidUTF8`-sanitize before marshal, a documented DEPARTURE from the reference's specific mangling, or emit-as-`as_bytes` fallback). AS_BYTES is always faithful and is the proto default, so the trap is AS_STRING-only.
- **AMEND-TAP-BODY2-CHUNKS (SOURCE — decode-side bodies arrive in CHUNKS; the filter must self-accumulate).** Re-derived from the landed HCM plumbing (data-hook seam analysis, §3.2): on the H1 **decode** path the request body is streamed to the filter chain in arbitrary **≤32 KiB chunks** (`connection.go:629-656`), with `endStream` riding the final chunk (or a synthetic `(nil, true)` call). The **encode** path (both H1/H2) and the **H2 decode** path deliver the whole body in exactly ONE `endStream=true` call (the router did `io.ReadAll(resp.Body)` before the encode chain runs, `router.go:625`; H2 buffers the whole request before dispatch, `h2dispatch.go:496-540`). ⇒ tap MUST accumulate `DecodeData` chunks into a filter-owned buffer across calls (the landed `buffer` filter is the precedent — `buffer/buffer.go:65-243`: a filter-owned field updated every call, finalized at `endStream`, always returning `DataContinue`), NOT assume a single whole-body call. A bodyless `GET` / zero-length response never invokes the data hook at all (the `hasBody` gate, `connection.go:561` / `h2dispatch.go:505`) — so `body` is naturally omitted, matching AMEND-TAP-BODY.

### 1.2 ADR continuity + D-disposition at SPEC commit

DECISIONS.md tail stays **ADR-0273** at this SPEC (docs-only; no ADR body lands until IMPL per ADR-0044). §13 anchors the **ADR-0274 §Context DRAFT** only (the SOLE anticipated ADR for this leg; §Decision/§Consequences land at the 56.2 IMPL). The SPEC resolves the body-behavior empirical questions (§11) and raises FOUR PLAN-level questions (§12): the non-UTF8 AS_STRING disposition (AMEND-TAP-BODY2-UTF8); whether the differential covers AS_BYTES cross-side or unit-tests it; whether a >32 KiB multi-chunk arm is differential or unit; and whether an optional predicate-false early-out skips body buffering.

---

## 2. Non-purposes (what 56.2 STILL defers — the 56.1 deferrals, minus bodies)

56.2 discharges ONLY the body deferral from SPEC-56.1 §2. Everything else stays deferred, unchanged:

- **The `streaming_grpc` sink** (the reference ABORTS on it — AMEND-TAP-REJECTS); **`streaming_admin`/`buffered_admin`/`admin_config`** (blocked on `internal/admin` chunked/flush semantics); **`custom_sink`**.
- **`streaming: true` / `HttpStreamedTraceSegment`** — a different `TraceWrapper` oneof arm; still boot-rejected. (Body STREAMING — chunked `HttpStreamedTraceSegment` bodies — is NOT this leg; 56.2 is BUFFERED bodies only, `streaming: false`.)
- **The `PROTO_BINARY`/`PROTO_BINARY_LENGTH_DELIMITED`/`PROTO_TEXT` formats** — still rejected (§6). 56.2 lifts only the AS_BYTES-vs-AS_STRING JSON distinction; the three PROTO formats remain a DEPARTURE reject.
- **The 2 `generic_body` match arms** (a body-content predicate — the reference boots and matches; envoy-go still rejects at compile, `matchpredicate` §3.3) and the 2 **trailer** match arms + `Message.trailers` CONTENT (Q1 framework gap, unchanged). NOTE: `generic_body` matching (predicate ON the body) is DISTINCT from body CAPTURE (this leg); 56.2 captures bodies for the TRACE without adding body-content MATCHING.
- **`tap_enabled` (f3)**, **`record_headers_received_time` (f2)** — still rejected. **`record_downstream_connection` (f3)** — honored as landed at 56.1 (`trace.go:79-84`); unchanged.
- **The tap TRANSPORT SOCKET**; **migrating `rbac`/`ratelimit`/`oauth2` onto `internal/headermatch`** (Q5 debt).
- **Non-UTF8 AS_STRING faithful reproduction** (AMEND-TAP-BODY2-UTF8) — a coverage boundary, not a deferral: Go's protojson cannot reproduce the reference's lossy C++ mangling, so it is UNasserted, not scheduled.

No change to any landed filter, the HTTP-filter dispatch layer, the callback surface, the predicate engine, the sink's file/marshal machinery, or the trailer seams (framework-zero-touch). The ONLY production package touched is `internal/filter/http/tap`.

---

## 3. The body-capture design (ADR-0274)

Body capture is a per-stream accumulation on the existing `*tapFilter` value — no new type, no new interface, no new seam. The one shared `*tapFilter` (installed in both `HTTPFilter.Decoder`/`.Encoder`, `config.go:38-46`) already owns `reqHdrs`/`respHdrs`; 56.2 adds body buffers alongside.

### 3.1 The config parse — store `format` + resolved caps (`config.go`)

`parseConfig` (`config.go:51`) currently accepts both JSON formats and DISCARDS the choice (`config.go:113-115`). 56.2:
- **Store the `format`** on `config` (a new field, e.g. `bodyAsString bool` derived from `s.GetFormat() == taptapv3.OutputSink_JSON_BODY_AS_STRING`). The two PROTO branches (`config.go:116-119`) still REJECT; the `default` (`:120-121`) still rejects. No roster change (§6).
- **Resolve the two caps at parse time** from `OutputConfig.max_buffered_rx_bytes` (f2) / `max_buffered_tx_bytes` (f3): `nil` → **1024** (AMEND-TAP-BODY2-DEFAULTCAP); a present `*wrapperspb.UInt32Value` → `GetValue()` (including **0**). Store both `uint32` (or `int`) on `config`. These are read-only per-stream, shared like the rest of `config`.
- No new counter, no new sink field, no new reject arm. The `format` and caps are the only additions to the parsed config.

### 3.2 The data-hook seam — accumulate chunks, honor the cap (`tap.go`)

Replace the two inert hooks (`tap.go:60-65`). Per AMEND-TAP-BODY2-CHUNKS, decode-side data arrives in chunks; encode-side in one call. A UNIFORM accumulation handles both:

- `*tapFilter` gains four fields: `reqBody []byte`, `reqTrunc bool`, `respBody []byte`, `respTrunc bool` (nil buffers until the first chunk).
- `DecodeData(data []byte, endStream bool)`: append `data` to `reqBody` up to `cfg.maxRx`; if `len(reqBody) + len(data) > maxRx` (strict `>`, AMEND-TAP-BODY2-BOUNDARY), append only the prefix that fits and set `reqTrunc = true`, then stop appending further bytes this and subsequent calls. Return `DataContinue` ALWAYS (tap is a read-only observer — NEVER `DataStopIterationAndBuffer`, NEVER `OverwriteBody`; it must not perturb the body flowing to the upstream). `endStream` is not needed to finalize (the buffer IS the state); accept it for signature conformance.
- `EncodeData(data []byte, endStream bool)`: symmetric, into `respBody`/`respTrunc` up to `cfg.maxTx`.
- **The cap-0 case** (AMEND-TAP-BODY2 / §11.3 `tx-zero0`): `maxTx == 0` and a non-empty chunk arrives → append nothing, set `respTrunc = true` → an EMPTY-but-truncated body. This is DISTINCT from a genuinely bodyless stream (the hook never fires → buffer stays nil → `body` omitted). The IMPL must track "did the hook fire at all" separately from "is the buffer empty": a fired-hook-with-empty-buffer-and-truncated emits `{as_string: "", truncated: true}`; a never-fired hook emits no `body`. RECOMMEND a `sawReqBody`/`sawRespBody` bool set on the first `DecodeData`/`EncodeData` invocation (mirrors the `sawReq`/`sawResp` header flags, `tap.go:25-26`).

**Chunk accumulation is filter-owned, like `buffer/buffer.go:65-243`, NOT via a callback buffer accessor** — `DecoderFilterCallbacks` has NO `BufferDecodedBody` (only `EncoderFilterCallbacks.BufferEncodedBody()` exists, `callbacks.go:350`), so tap keeps its own `[]byte` regardless of direction. This is a SHAPE precedent only; tap keeps bytes, `buffer` keeps only a count.

### 3.3 The `Body` assembly in `buildTrace` (`trace.go`)

`buildTrace` (`trace.go:68`) builds `Request`/`Response` `Message`s with `Headers` only. 56.2 adds `Body` to each Message when a body was captured:

- Add a helper `bodyProto(buf []byte, trunc bool, asString bool) *datatapv3.Body`:
  - `asString` → `&datatapv3.Body{BodyType: &datatapv3.Body_AsString{AsString: string(buf)}, Truncated: trunc}`;
  - else → `&datatapv3.Body{BodyType: &datatapv3.Body_AsBytes{AsBytes: buf}, Truncated: trunc}`.
  - `EmitDefaultValues` renders `truncated` unconditionally (AMEND-TAP-BODY2-TRUNCFLAG), so a full body gets `"truncated": false` — reference-identical.
- In `buildTrace`: set `bt.Request.Body = bodyProto(reqBody, reqTrunc, asString)` IFF `sawReqBody` (the hook fired), else leave nil (omitted). Symmetric for `Response`. A never-fired hook (bodyless GET / zero-length / predicate-false-and-early-out) leaves `Body` nil ⇒ omitted (AMEND-TAP-BODY).
- Trailers stay nil (§3.8 of SPEC-56.1, unchanged — the trailers boundary is orthogonal to bodies).

### 3.4 The AS_STRING UTF-8 constraint (AMEND-TAP-BODY2-UTF8)

`string(buf)` for a non-UTF8 `buf` produces an invalid-UTF-8 Go string; `marshalOpts.Marshal` then ERRORS (Go protojson rejects invalid UTF-8 in a `string` field) and the sink swallows it (`trace.go:65`), dropping the trace. The IMPL MUST handle this (§12 D-TAP-BODY-UTF8): the differential avoids it (UTF-8 bodies only), but a hostile/binary upstream body under an AS_STRING config must not silently drop every trace. RECOMMENDATION (a PLAN pin): sanitize with `strings.ToValidUTF8(string(buf), "�")` before constructing `Body_AsString` — a DOCUMENTED DEPARTURE from the reference's specific (unreproducible) mangling, but lossless-of-the-trace and deterministic. AS_BYTES is unaffected (base64 of arbitrary bytes always marshals). Record the boundary in `BEHAVIOR_CONTRACT` (§9).

### 3.5 What does NOT change (the 56.1 spine, verbatim)

The predicate engine (`internal/matchpredicate`), `internal/headermatch`, the emit lifecycle (`OnDestroy` resolve-then-emit, `trace.go:47-66`), the ONE-SHARED-VALUE constraint, the `:status` synthesis on a COPY (`tap.go:47-57`), the header rendering (lowercase/sort, `trace.go:21-35`), the `filePerTapSink` (file-per-stream, the pinned `marshalOpts` + trailing `\n`, the process-local trace-id, `sink.go`), the `rq_tapped` counter, the `builtins.go`/`boot.go` wiring, and the full §6 reject roster are ALL UNCHANGED. 56.2 is purely additive within `DecodeData`/`EncodeData`/`buildTrace`/`parseConfig`.

---

## 4. Framework primitives — 0 new packages, 0 new seams, 0 new go.mod deps, 0 new stats

| Primitive | Disposition |
|---|---|
| `internal/filter/http/tap` (config.go, tap.go, trace.go, sink.go) | **MODIFIED** (the only production package touched) |
| `StreamDecoderFilter.DecodeData` / `StreamEncoderFilter.EncodeData` | REUSED unchanged (`types.go:56`, `:67`) — the hooks made live |
| `FilterDataStatus` (`DataContinue` etc.) | REUSED (`types.go:26-37`); tap returns `DataContinue` always |
| `internal/matchpredicate`, `internal/headermatch` | UNCHANGED (no body-content matching this leg) |
| the `filePerTapSink` + `marshalOpts` | UNCHANGED (both JSON formats already `.json`; body is a proto-field change) |
| `buffer` filter (`buffer/buffer.go:65-243`) | SHAPE precedent for chunk accumulation, NOT reused |
| `EncoderFilterCallbacks.BufferEncodedBody` | NOT used (tap keeps its own buffer both directions) |
| new package / new seam / new go.mod module / new stat / new BackendKind / new fuzzer | **NONE** |

---

## 5. Proto-field roster consumed at 56.2 (fully qualified; all in the pinned `go-control-plane/envoy v1.32.4`)

Verified against the pinned module (`…/envoy@v1.32.4/`), not a citation:

| Message (package) | Field | # | Type | Disposition |
|---|---|---|---|---|
| `config/tap/v3.OutputConfig` | `max_buffered_rx_bytes` | 2 | `*wrapperspb.UInt32Value` | **CONSUMED** (nil → 1024; else value incl. 0) — `common.pb.go:544` |
| `OutputConfig` | `max_buffered_tx_bytes` | 3 | `*wrapperspb.UInt32Value` | **CONSUMED** — `common.pb.go:549` |
| `config/tap/v3.OutputSink` | `format` | 1 | `OutputSink_Format` | **CONSUMED** — AS_STRING(1)/AS_BYTES(0) now DIVERGE; PROTO_* still REJECT |
| `OutputSink_Format` enum | `JSON_BODY_AS_BYTES` / `JSON_BODY_AS_STRING` | 0 / 1 | — | AS_BYTES=default; both ACCEPT — `common.pb.go:43/51` |
| `data/tap/v3.HttpBufferedTrace_Message` | `body` | 2 | `*Body` | **POPULATED** iff the data hook fired — `http.pb.go:257` |
| `data/tap/v3.Body` | `as_bytes` | 1 | `[]byte` (oneof) | **POPULATED** under AS_BYTES — `common.pb.go:112` |
| `Body` | `as_string` | 2 | `string` (oneof) | **POPULATED** under AS_STRING (UTF-8 constraint §3.4) — `common.pb.go:119` |
| `Body` | `truncated` | 3 | `bool` | **POPULATED** (always emitted via EmitDefaultValues) — `common.pb.go:42` |

Everything else in the 56.1 roster (SPEC-56.1 §5) is unchanged.

---

## 6. PARSE-REJECT roster (ADR-0080) — UNCHANGED from 56.1 §6

The full PARITY-vs-DEPARTURE roster (SPEC-56.1 §6) stands verbatim. 56.2 changes NO reject arm:
- `PROTO_BINARY`/`PROTO_BINARY_LENGTH_DELIMITED`/`PROTO_TEXT` — still **REJECT** (DEPARTURE; the reference boots and honors them). 56.2 lifts only the AS_BYTES-vs-AS_STRING *render* divergence, both of which were already ACCEPTED at 56.1.
- `streaming: true`, the 2 trailer arms, the 2 generic_body arms, `admin_config`, `tap_enabled`, `match_config`, non-`file_per_tap` sinks, `sinks != 1` — all unchanged.
- **No new reject arm** is added for bodies: `max_buffered_rx_bytes`/`max_buffered_tx_bytes` are CONSUMED for any `uint32` value (0 is valid — empty-but-truncated). The reject roster remains UNIT-tested (`reference_differential_fixture_dispatch_constraint` — a boot-reject cannot share the `0100` cross-side dir).

---

## 7. Stat surface — 1201 → 1201 (+0)

No new stat. `http.<stat_prefix>.tap.rq_tapped` (the sole tap counter) already increments on the MATCH DECISION at `OnDestroy` (`trace.go:59-61`), independent of body capture. Body capture adds no counter (the reference exposes no body-specific tap stat — CONFIRMED: the §11 probes' `/stats` grep for `tap` yields only `rq_tapped` in every body arm). Surface stays **1201**; zero new SN rule.

---

## 8. Differential fixture (+1: `0100-http-tap-bodies`) — fixtures 101 → 102

### 8.1 The design — POST-echo, one config, three body-size arms *(AMEND-TAP-BODY2-BOUNDARY)*

One HCM + one tap filter per side. `match: any_match: true` (every stream taps — bodies are the subject, not the predicate); sink `file_per_tap{path_prefix: <per-side temp dir>/out}` (reuse `HostMount{Dir: true}`, the 56.1 D-TAP-DIRMOUNT surgery); `format: JSON_BODY_AS_STRING`; `max_buffered_rx_bytes: C` and `max_buffered_tx_bytes: C` for a chosen cap **C** (e.g. 20). Backend = **`HTTPEchoBody` (BackendKind 6, REUSE — tail stays 38)**: it echoes the request body back as the response body (`fixture.go:165-175`), so ONE POST populates BOTH `request.body` (rx) and `response.body` (tx) with the same known UTF-8 content. The driver issues three POSTs through the one config:
- **full arm** — body length **< C** (e.g. 10 bytes `"0123456789"`) → both bodies `{as_string: "0123456789", truncated: false}`.
- **boundary arm** — body length **== C** (e.g. 20 bytes) → both bodies FULL, `truncated: **false**` (strict-`>` boundary; this arm is what break (c) flips).
- **truncated arm** — body length **> C** (e.g. 40 bytes) → both bodies `{as_string: "<first C bytes>", truncated: true}`.

`BackendCount()` returns **≥1** (`reference_differential_backendcount_min_one`). All bodies are ASCII (UTF-8-safe — AMEND-TAP-BODY2-UTF8: never drive non-UTF8 under AS_STRING).

### 8.2 The glob-and-decode comparison *(reuse the 56.1 machinery)*

Glob `<path_prefix>*`, decode each file as a `data/tap/v3.TraceWrapper` (`reference_streaming_sink_differential_framing` — compare the decoded PAYLOAD, never filenames/framing). Cross-side assertions, each `t.Errorf`-per-property (`reference_fatalf_makes_assertions_unreachable`):
- trace COUNT == 3 (one per POST; `any_match` taps all).
- `http.<prefix>.tap.rq_tapped` == 3.
- per matching trace, `request.body.as_string` and `response.body.as_string` == the driven body (full arm) or its C-byte prefix (truncated arm) — **cross-side EXACT** (both proxies capture identical bytes; the reference always captures, AMEND-TAP-BODY).
- per trace, `request.body.truncated` / `response.body.truncated` == the expected bool (`false` for full+boundary, `true` for truncated) — a POSITIVE breakable assertion.
- the boundary-arm trace asserts `truncated == false` at body-length == C (the strict-`>` proof).
- `request.body` / `response.body` PRESENT (not absent) on every arm (the inverse of 0099's absent-assertion).

**UNasserted coverage boundaries:** non-UTF8 AS_STRING (AMEND-TAP-BODY2-UTF8 — never driven); the exact byte-count of a chunk boundary (H1 delivers ≤32KiB chunks — invisible to the payload comparison); header order (§3.6 of 56.1); filenames; the reference-only headers (unchanged from 0099 §8.2).

### 8.3 AS_BYTES coverage — UNIT-tested, not fixtured *(a §12 decision, RECOMMENDED)*

The `0100` differential drives AS_STRING (one `format` per config; the differential proves the cross-side body payload + truncation). **AS_BYTES render is proven by a UNIT test** in `internal/filter/http/tap`: `buildTrace` with a captured body + `bodyAsString=false` → `marshalOpts.Marshal` → assert `"as_bytes": "<base64>"` present, `"as_string"` ABSENT, and `truncated` rendered. This is faithful because AS_BYTES is `base64.StdEncoding` that Go protojson produces byte-identically to the reference (AMEND-TAP-BODY2-BYTES — probe-verified `YWJjZGVmZ2hpag==`), so a unit test is a complete proof and avoids a two-listener fixture. (ALTERNATIVE the PLAN may choose: a second listener in the `0100` config with AS_BYTES tap for a cross-side AS_BYTES proof — stronger but adds driver complexity for a Go-native-deterministic render; RECOMMEND the unit test.)

### 8.4 Deliberate breaks (≥4; each `-count=1`, `t.Errorf`-per-property isolated)

Per `reference_differential_break_protocol_count1`, `reference_deliberate_break_wrong_assertion`, `reference_fatalf_makes_assertions_unreachable`. Use `-run 'TestDifferential/0100-http-tap-bodies'`, NEVER bare `0100` (`reference_differential_run_selector`).

- **(a) Leave `DecodeData`/`EncodeData` inert (don't capture)** → `request.body`/`response.body`-PRESENT assertion fires (bodies absent on all arms). Proves the capture is LIVE.
- **(b) Ignore the cap (capture the full body always)** → the truncated arm's `truncated == true` AND its C-byte-prefix assertions fire (captures 40 bytes, `truncated: false`). Proves the cap is HONORED.
- **(c) Truncate at `>=` instead of `>`** → the **boundary arm** (body length == C) flips `truncated: false → true`, firing the boundary arm's **`truncated`-FLAG assertion** — and ONLY that assertion. The captured PAYLOAD is UNCHANGED: the prefix-length that fits is `cap − capturedLen` bytes (= the whole 20-byte body at `capturedLen=0`), a formula that does NOT depend on the `>`-vs-`>=` comparison, so the payload stays the correct 20 bytes; only the flag flips. The full/truncated arms stay green. Proves the strict-`>` boundary (AMEND-TAP-BODY2-BOUNDARY) is LIVE. *(This is the arm that JUSTIFIES the body-length-==-C boundary POST; without it, break (c) is vacuous. Per `reference_deliberate_break_wrong_assertion`, the IMPL must CONFIRM WHICH assertion fires when it re-performs this break — expect the `truncated`-flag check, NOT the payload check; do not assert the payload flips.)*
- **(d) Wrong oneof for AS_STRING (emit `as_bytes`)** → `request.body.as_string`/`response.body.as_string` decode to empty ⇒ the payload assertion fires. Proves the format→oneof mapping is LIVE. *(Isolating variant: if (d) also perturbs `truncated`, add a separate break; confirm WHICH fired per `reference_deliberate_break_wrong_assertion`.)*

Full-suite discipline unchanged: `subject ready: EOF` on an unrelated fixture is the known startup flake (`reference_differential_fullsuite_startup_flake`); Docker bridge network for the reference (`reference_docker_probe_bridge_network`); `-count=1` on every break (`reference_differential_break_protocol_count1`); watch for the `-race`-after-a-background-mutator full-package rule (`reference_full_suite_race_after_background_mutator`).

---

## 9. Behavior-contract delta (the 56.2 bundle; ADR-0052 atomic landing)

`BEHAVIOR_CONTRACT.md` gains, landed atomically at the 56.2 IMPL:
- the tap filter's body-capture model (accumulate `DecodeData`/`EncodeData` chunks into a per-direction buffer, honor the cap, populate `Message.body` at stream end on a match);
- the cap semantics: **default 1024 when `max_buffered_*` unset**; **strict-`>` truncation** (a body exactly at the cap is NOT truncated); **cap-0 + non-empty body → empty-but-truncated `body` present** (vs a genuinely bodyless stream → `body` omitted);
- the `format`→oneof render: `JSON_BODY_AS_STRING` → `as_string`, `JSON_BODY_AS_BYTES` → `as_bytes` (standard base64), with `truncated` ALWAYS emitted (EmitDefaultValues);
- the **non-UTF8 AS_STRING coverage boundary** (AMEND-TAP-BODY2-UTF8 — Go protojson cannot reproduce the reference's lossy mangling; envoy-go sanitizes/documents, never silently drops).

The 56.1 contract blocks (headers-only lifecycle, tri-state, reject roster, byte-exact JSON caveat) are UNCHANGED.

---

## 10. Per-task structure (~6-8 tasks; the PLAN decomposes — WELL under the ADR-0045 ceiling)

1. `config.go` — store `format` (as_string bool) + resolve both caps (nil → 1024; else value incl. 0) + unit tests (cap resolution, format storage; the PROTO_* rejects stay green).
2. `tap.go` — the `DecodeData`/`EncodeData` accumulation: append up to the cap, strict-`>` truncation, `sawReqBody`/`sawRespBody`, always `DataContinue` + unit tests (single-chunk, multi-chunk >32KiB accumulation, at/under/over the cap, cap-0).
3. `trace.go` — `bodyProto` + `buildTrace` wiring (Body iff hook fired; oneof from format; the UTF-8 sanitize decision §3.4) + unit tests (AS_STRING render, AS_BYTES base64 render §8.3, truncated true/false, empty-but-truncated, body-omitted-when-not-fired).
4. Fixture `0100-http-tap-bodies` config (both sides) + driver (three POSTs: full/boundary/truncated against `HTTPEchoBody`).
5. Differential assertions (§8.2) + the 4 deliberate breaks (§8.4), controller-reperformed with `-count=1`.
6. Docs bundle: `BEHAVIOR_CONTRACT` delta (§9) + **ADR-0274 §Decision/§Consequences** (bodies) + PROGRESS-56.2 + STATE + ROADMAP (flip row 56 `done` — `reference_roadmap_split_phase_row_done`) + README (56.2 done).

This is comfortably under the `~15`-task ADR-0045 ceiling — 56.2 is additive on a proven spine. The escape valve stays armed (SPEC-56.1 §3.0): if the PLAN's decomposition somehow exceeds `~15`, re-open ADR-0045 — but no re-split is anticipated.

---

## 11. SPEC-time empirical-pin block — executed IN-SESSION 2026-07-10 against `envoyproxy/envoy:contrib-v1.37.2`

**Harness:** `--concurrency 1`, Docker **bridge** network `tapbody-net`, a **FRESH probe container per arm** (`feedback_probe_fresh_container_per_arm`), backend `mccutchen/go-httpbin` (`/range/N` = N deterministic `abc…` UTF-8 bytes; `/bytes/N` = N random bytes; `/anything` echoes the POST body), driver a `curlimages/curl` sidecar. HCM `stat_prefix: tap_probe`; tap `match: any_match: true`, sink `file_per_tap`; the RAW tap file read from a host bind-mount. Ten arms; all resolved.

### Summary disposition table

| Pin | Disposition |
|---|---|
| DEFAULT cap (unset) | **RESOLVED** — 1024 bytes (2000B body → captured 1024, `truncated: true`) |
| Truncation boundary | **RESOLVED** — strict `>`: cap==len → `truncated: false`; cap<len → `true` |
| `truncated` render | **RESOLVED** — ALWAYS emitted (full body → `"truncated": false`) via EmitDefaultValues |
| AS_BYTES form | **RESOLVED** — standard base64 + padding (`YWJjZGVmZ2hpag==`) = Go-native |
| AS_STRING + non-UTF8 | **RESOLVED (coverage boundary)** — reference mangles lossily; Go protojson would ERROR |
| cap-0 + nonempty body | **RESOLVED** — `{as_string: "", truncated: true}` (distinct from omitted body) |
| request-body (rx) capture | **RESOLVED** — POST body captured; cap truncates symmetric to tx |
| data-hook chunk reality (SOURCE) | **RESOLVED** — H1 decode chunks ≤32KiB; encode + H2 one call; bodyless → hook never fires |

### 11.1 DEFAULTCAP + BOUNDARY (arms tx-default-big, tx-cap-under/eq/over)

`/range/10` (10-byte body `abcdefghij`), `format: JSON_BODY_AS_STRING`:
```
max_buffered_tx_bytes unset  -> "as_string":"abcdefghij" "truncated":false   (default cap 1024 > 10)
max_buffered_tx_bytes: 9     -> "as_string":"abcdefghi"  "truncated":true
max_buffered_tx_bytes: 10    -> "as_string":"abcdefghij" "truncated":false   <-- == cap, NOT truncated
max_buffered_tx_bytes: 11    -> "as_string":"abcdefghij" "truncated":false
```
`/range/2000` (2000-byte body), `max_buffered_tx_bytes` unset → `as_string` length **1024**, `truncated: true` ⇒ **default cap = 1024**.

### 11.2 TRUNCFLAG + BYTES (arms tx-full-str, tx-bytes-full)

Full body, AS_STRING (verbatim):
```
"body": { "as_string": "abcdefghij", "truncated": false }
```
Full body, AS_BYTES (verbatim):
```
"body": { "as_bytes": "YWJjZGVmZ2hpag==", "truncated": false }
```
`base64.StdEncoding("abcdefghij") == "YWJjZGVmZ2hpag=="` (padding present) — matches Go `protobuf` `bytes` rendering exactly. `truncated: false` is emitted in BOTH (EmitDefaultValues).

### 11.3 cap-0 (arm tx-zero0)

`/range/10`, `max_buffered_tx_bytes: 0` → `"body": { "as_string": "", "truncated": true }` — an EMPTY-but-truncated body (the field is PRESENT). Contrast a genuinely bodyless stream (bodyless GET / 204 → `body` OMITTED, SPEC-56.1 §11.9). ⇒ the IMPL must distinguish "hook fired, buffer empty, truncated" from "hook never fired".

### 11.4 UTF8 coverage boundary (arm tx-nonutf8)

`/bytes/8` (8 random bytes), `format: JSON_BODY_AS_STRING` → `"body": { "as_string": " FH F ", "truncated": false }` — 6 chars, LOSSY (not the 8 source bytes; the C++ serializer mangled the invalid UTF-8). Go's `protojson.Marshal` returns an error for invalid UTF-8 in a `string` field, so envoy-go cannot reproduce this ⇒ **the `0100` differential drives UTF-8-safe bodies only; non-UTF8 AS_STRING is UNasserted.** AS_BYTES is faithful for arbitrary bytes.

### 11.5 request-body capture (arms rx-full-str, rx-cap-under)

POST `HELLOBODY42` (11 bytes) to `/anything` (go-httpbin echoes), AS_STRING:
```
max_buffered_rx_bytes unset -> request.body {"as_string":"HELLOBODY42","truncated":false}
max_buffered_rx_bytes: 5    -> request.body {"as_string":"HELLO","truncated":true}
```
(response.body is go-httpbin's JSON echo — also captured; confirms both directions.)

### 11.6 data-hook chunk reality (SOURCE, AMEND-TAP-BODY2-CHUNKS)

Re-derived from the landed HCM plumbing (full citations in the session's data-hook-seam analysis): `DecodeData(data []byte, endStream bool)` / `EncodeData(...)` (`types.go:56/67`); `FilterDataStatus` = `DataContinue`/`DataStopIterationAndBuffer`/`DataStopIterationNoBuffer` (`types.go:26-37`). H1 decode streams the request body in ≤32KiB chunks (`connection.go:629-656`), endStream on the final chunk or a synthetic `(nil,true)`. H1 encode + H2 decode + H2 encode deliver one whole-body `endStream=true` call (`router.go:625` io.ReadAll before encode; `h2dispatch.go:496-540` full H2 request buffering). Bodyless GET / zero-length response → the `hasBody` gate (`connection.go:561` / `h2dispatch.go:505`) means the data hook NEVER fires. Precedent for filter-owned cross-chunk accumulation: `buffer/buffer.go:65-243` (a filter-owned field, finalize at endStream, always `DataContinue`). Compressor's `EncodeData` (`compressor.go:1029-1081`) relies on the one-whole-body encode guarantee and guards `if !endStream { return DataContinue }` — a shape tap should NOT copy on the DECODE side, which genuinely chunks.

---

## 12. PLAN / IMPL D-questions (not empirical pins)

- **D-TAP-BODY-UTF8.** The non-UTF8 AS_STRING disposition (AMEND-TAP-BODY2-UTF8, §3.4). RECOMMEND `strings.ToValidUTF8(…, "�")`-sanitize before `Body_AsString` (a documented DEPARTURE, lossless-of-the-trace) — NOT silently dropping the trace on a marshal error. The PLAN pins it; the `0100` fixture never exercises it.
- **D-TAP-BODY-ASBYTES-DIFF.** Whether AS_BYTES render is proven cross-side (a second AS_BYTES listener in `0100`) or by a unit test (§8.3). RECOMMEND the unit test (AS_BYTES is Go-native-deterministic base64; a second listener adds driver complexity for no additional signal).
- **D-TAP-BODY-CHUNKTEST.** Whether cross-chunk accumulation (>32KiB, multi-`DecodeData`) is proven differentially (an expensive large-body arm) or by a unit test feeding two `DecodeData` calls. RECOMMEND the unit test (a differential >32KiB body is slow and the payload comparison is chunk-agnostic anyway).
- **D-TAP-BODY-EARLYOUT.** Whether the IMPL skips body buffering when the predicate is ALREADY resolvably-False at `DecodeHeaders` (a memory optimization: a request-headers-only predicate that is False need not buffer the body). RECOMMEND deferring the optimization — buffer unconditionally up to the cap (the cap bounds memory) and discard on no-match at `OnDestroy`; the reference buffers per the config caps regardless, and an early-out adds a correctness-sensitive branch (the response arm may still flip a partially-Undetermined tree). If adopted, it must NOT change any observable trace (only memory).

---

## 13. ADR continuity — the ADR-0274 §Context DRAFT (anchored here; body at the IMPL per ADR-0044)

DECISIONS.md tail stays **ADR-0273** at this SPEC. next-free is **ADR-0274**. Recommend a SINGLE absorbing ADR-0274 for this leg (no separate seam ADR — the leg touches one package and adds no seam).

> **ADR-0274 — the HTTP tap filter, bodies leg (§Context DRAFT).**
>
> The Observability family's ninth row's SECOND and final leg picks up body capture *(Q6 by-concern split)*, on the headers spine landed at ADR-0273. 56.1 shipped a headers-only buffered trace over a deliberately body-free `GET → 204` stream (a zero-length body OMITS the `body` field, so the two accepted JSON formats were indistinguishable); 56.2 makes the two inert data hooks (`DecodeData`/`EncodeData`) buffer body bytes into the pending trace and populates `HttpBufferedTrace_Message.body`.
>
> A live probe against `envoyproxy/envoy:contrib-v1.37.2` (2026-07-10) pinned the body behavior on ten arms. The load-bearing findings: the per-direction cap `max_buffered_rx_bytes`/`max_buffered_tx_bytes` defaults to **1024 bytes** when the `UInt32Value` wrapper is unset (NOT unbounded); truncation is **strict `>`** (a body exactly at the cap is NOT truncated); the `truncated` bool is ALWAYS emitted (EmitDefaultValues renders `false` on a full body — the same marshal pin as the headers leg); AS_BYTES is standard base64 that Go's protojson reproduces byte-identically, while AS_STRING with non-UTF8 content is a coverage boundary Go cannot cross (the reference's C++ serializer mangles invalid UTF-8 lossily, whereas Go's protojson rejects it — so the differential drives UTF-8 bodies only and envoy-go sanitizes rather than silently dropping the trace). Source re-derivation of the data-hook seam established that decode-side request bodies arrive in ≤32KiB chunks (the filter must self-accumulate, like the `buffer` filter), whereas encode-side and H2-decode deliver one whole-body call, and a bodyless stream never invokes the hook (so `body` is naturally omitted). The leg touches ONE package (`internal/filter/http/tap`), reuses the `HTTPEchoBody` backend for a POST-echo `0100-http-tap-bodies` differential (full / at-cap-boundary / truncated arms, cross-side body-payload + `truncated`-flag equality), and adds no package, seam, go.mod module, stat, BackendKind, or fuzzer. It flips ROADMAP row 56 `done` at its IMPL six-gate (no parent rollup, ADR-0106).

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

| Surface | At SPEC (docs-only) | Anticipated at 56.2 IMPL |
|---|---|---|
| stat surface | 1201 | **1201** (+0; no new counter) |
| fixtures | 101 | **102** (`0100-http-tap-bodies`) |
| fuzzers | 53 | **53** (+0; body capture is non-recursive/bounded — no fuzzer earns its place) |
| BackendKind tail | 38 | **38** (+0; `HTTPEchoBody` REUSE) |
| DECISIONS tail | ADR-0273 | **ADR-0274** (next-free ADR-0275) |
| new Go packages | 0 | **0** (`internal/filter/http/tap` modified) |
| new go.mod modules | 0 | **0** (`go mod tidy -diff` anticipated EMPTY) |
| ROADMAP row 56 | in-progress | **done** (flips at the IMPL six-gate — `reference_roadmap_split_phase_row_done` + ADR-0106) |

Counts at THIS SPEC are UNCHANGED (docs-only). Row 56 STAYS `in-progress`. **Next → the phase-56.2 PLAN** (decompose §10 into a TDD spine; resolve the four §12 D-questions; the ADR-0045 split-gate stays CONSUMED — no re-split anticipated).
