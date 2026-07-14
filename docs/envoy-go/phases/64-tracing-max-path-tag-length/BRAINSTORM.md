# Phase 64 Brainstorm — tracing `max_path_tag_length` (the FIFTEENTH Observability-family row; a bounded NUMERIC truncation knob on the ALREADY-emitted `http.url` span attribute — lifts the `internal/tracing/config.go:112-114` `max_path_tag_length is unsupported` reject and truncates the `:path` portion of the `http.url` tag to N bytes [default 256], on BOTH the OTLP and Zipkin exporters; +0 stats / +0 packages / +0 modules; anticipated ONE new fixture)

> **Stage:** BRAINSTORM (lifecycle-state 0 → 1). Docs-only; no `.go` changes at this stage. Fresh worktree off master, branch `phase-64-tracing-max-path-tag-length`, per `feedback_git_worktrees`.
>
> **Loop re-open (AUTONOMOUS — no human pick):** phase 63 (`tracing-custom-tags-environment`) landed COMPLETE (row 63 `done`, ADR-0284; the Observability family STAYS OPEN). Per the **STANDING DIRECTIVE (human, 2026-07-12)** the loop runs AUTONOMOUSLY until the termination sentinel fires; the sentinel was re-checked MECHANICALLY at the phase-63 IMPL and does NOT fire (check (1) silent — every row `done` — but check (2) prints THREE live "candidates:" sentences [HTTP/3, xDS, Observability] and check (3) prints THREE never-opened families [gRPC, Runtime, WASM], each independently blocking `stop`). No banked mid-lifecycle split legs remain (no `in-progress` ROADMAP rows). So the roller SELF-PICKED the next subject (§2.1): the **smallest cleanly-differential-provable candidate** from a full source read against the current master tip — the tracing **`max_path_tag_length`** knob (a bounded numeric truncation on the ALREADY-emitted, ALREADY-cross-side-asserted `http.url` span attribute) — over the declined larger/harder-to-prove alternatives (recorded §2.1). No human pause; no `stop` file.
>
> **Baselines re-verified against master tip `e0864177` (the router-refresh tip; includes the phase-63 IMPL squash `54f52628`):** stat surface **1201** · fixtures **108** (`ls -d test/fixtures/[0-9]*/ | wc -l`; tail `0106-tracing-custom-tags-environment`; the count includes the lettered `0007a`/`0007b` sub-fixtures — a `^[0-9]{4}-` grep under-counts) · fuzzers **55** (`grep -rh '^func Fuzz' --include='*.go' . | wc -l`) · BackendKind tail **38** (`H2GoawayResponder`) · DECISIONS tail **ADR-0284** (next-free **ADR-0285**) · new Go packages **0** · go.mod modules **2** (`quic-go v0.54.1` direct + `qpack v0.5.1` indirect). Counts are UNCHANGED at a BRAINSTORM (docs-only). All `file:line` citations below were RE-DERIVED from source this session (`feedback_brief_citations_not_evidence`) — see §11.

---

## 1. Mission and scope confirmation (64 — a NUMERIC knob on an existing tag, NOT a new tag source)

### 1.1 What phase 64 delivers as a self-contained whole (a truncated `http.url` span attribute)

The HCM `tracing` parse today emits the `http.url` built-in span attribute UNTRUNCATED — the full request URL — and STRICT-REJECTS the `max_path_tag_length` knob outright:

```go
// internal/tracing/config.go:112-114 (re-derived against master e0864177)
if t.GetMaxPathTagLength() != nil {
    return nil, fmt.Errorf("tracing: max_path_tag_length is unsupported")
}
```

```go
// internal/tracing/span.go:70 — http.url is ALWAYS emitted (a built-in)
KV{Key: "http.url", Str: in.URL},
// internal/filter/hcm/accesslog_emit.go:40 — built as scheme://host/path-and-query, UNTRUNCATED
url := scheme + "://" + r.Host + r.URL.RequestURI()
```

Phase 64 **lifts that one reject** and HONORS `max_path_tag_length`: the reference caps the length of the `:path` portion of the `http.url` span tag (Envoy `Http::Utility::buildOriginalUri(headers, max_path_tag_length)` — anticipated, probed at SPEC as D-MPTL-TARGET/D-MPTL-REFTRUNC). An operator configuring `tracing: { max_path_tag_length: { value: 16 } }` on the HCM gets an `http.url` span attribute whose PATH is truncated to 16 bytes — on **both** the OTLP and Zipkin exporters (the single URL-construction seam feeds `Span.Attrs`, §2.3). Critically, the reference applies a DEFAULT cap of **256** when the field is ABSENT (anticipated — D-MPTL-DEFAULT), so today's envoy-go UNTRUNCATED emission ALREADY DIVERGES from the reference for any request path longer than 256 bytes — a latent gap this row closes. Existing fixtures all use short paths (< 256), so honoring the default is byte-stable for the whole corpus (§2.6).

The delivery is a complete, testable slice: a bounded numeric knob, resolved at parse, applied at URL-construction time, cleanly differential-provable via a long-path GET against a small explicit cap.

### 1.2 What phase 64 does NOT deliver (forward to §8)

NO new `custom_tag` source (`metadata` STAYS the SOLE loud `custom_tags` strict-reject departure — untouched, orthogonal). NO other tracing follow-on (`spawn_upstream_span` / `http_service` / force-trace / `verbose` — each larger, §2.1/§8). NO OTLP-metrics stats sink. NO new provider, transport, or stat. The four built-in span attributes currently emitted EMPTY (`upstream_cluster`/`node_id`/`zone`/`peer.address`, the `reference_tracing_upstream_cluster_framework_gap` framework gap) are UNTOUCHED — `max_path_tag_length` acts ONLY on `http.url` (a fully-populated built-in), NOT on those un-plumbed fields, so it does NOT touch that gap (do NOT conflate — §11). The access-log record's `Path` field (`accesslog_emit.go:63`) is a SEPARATE concern and is UNTOUCHED — the truncation is scoped to the tracing `http.url` span attribute only.

### 1.3 Phase-done as the FIFTEENTH Observability-family row landing (family STAYS OPEN)

Row 64 is the FIFTEENTH Observability-family row and the FIRST tracing NUMERIC-knob row (the prior tracing rows — phase 46 provider/OTLP/Zipkin, phase 59/62/63 the three `custom_tag` sources — all added TAGS or providers; this adds a truncation cap on an existing tag). After phase 64 phase-done the family STAYS OPEN — the deferred candidates in §8 remain (OTLP-metrics sink + `metadata` custom-tag type + `spawn_upstream_span`/`http_service`/force-trace/`verbose`), so the sentinel check-(2) still prints ⇒ the loop continues.

### 1.4 ADR-0045 split readiness — anticipated a SINGLE FLAT ROW (escape-valve armable) *(self-answered; SPEC confirms)*

Anticipated a SINGLE FLAT ROW (~8–12 tasks: the parse arm lifting the reject + the default-256 resolution + the truncation helper + its call-site wiring at the three URL-build sites + config unit tests + a truncation unit test + the fuzz seed + the fixture + the doc/BEHAVIOR_CONTRACT edits + verify + ADR-0285), comfortably under the ADR-0045 `~15` ceiling. There is no second subsystem to strand: the parse (the numeric field + default) and the application (the path truncation at URL construction) both sit on the SAME landed tracing engine. The escape valve is documented ARMABLE and re-armed only if the SPEC's task count surprises upward (e.g. if proving the DEFAULT-256 truncation demands a separate large-path fixture leg — D-MPTL-DEFAULT-PROOF, §2.8).

### 1.5 Seed-stub alignment + package placement — ALL edits in EXISTING files/packages, ZERO new packages

- Production `max_path_tag_length` parse arm (lift the reject; read the `UInt32Value`; default 256 when absent): `internal/tracing/config.go` `NewConfig` (the reject at `:112-114`).
- Parsed-config home: a NEW `tracing.TracingConfig.MaxPathTagLength uint32` field (`config.go:25-40`) — the resolved cap (default 256; an explicit 0 preserved — D-MPTL-ZERO). Symbol GREP-confirmed collision-free this session (the only `MaxPathTagLength` hit is the PROTO field `tr.MaxPathTagLength` in the existing reject test `config_test.go:340` — a different struct; §11).
- Truncation application: a NEW small helper (anticipated `internal/tracing`) that builds `scheme://host + truncate(path, cap)`, called at the THREE `accesslog_emit.go` URL-construction sites (`:40` H1 / H2 `:~101` / H3 `:~163`) replacing the inline `url := scheme + "://" + r.Host + r.URL.RequestURI()`. The cap comes from `f.tracingConfig.MaxPathTagLength` (already reachable at all three sites — they read `f.tracingConfig.CustomTags`). D-MPTL-TRUNC-LOCATION (§2.5).
- Fuzz SEED: `internal/filter/hcm/fuzz_test.go` `FuzzHCMConfigParse` (existing) — a NEW `max_path_tag_length` seed (present incl. explicit 0), NOT a new fuzzer (§2.9).
- Fixture: anticipated ONE new `test/fixtures/NNNN-tracing-max-path-tag-length` dir (RE-DERIVE the next-free number at IMPL — `0106` is the current tail; `0107` anticipated).
- Docs: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (tracing section) + ROADMAP/STATE/DECISIONS.

ZERO new packages. ZERO new modules.

### 1.6 No prebrainstorm-notes branch

No off-master prebrainstorm-notes branch exists for this subject. `max_path_tag_length` is a recorded deferred candidate — named EXPLICITLY in the phase-63 BRAINSTORM §8 (`"caps the http.url tag length; still rejected; orthogonal. Carries forward."`) — not a stashed WIP.

### 1.7 Phase 64's relationship to the existing seams (a parse-reject lift + a URL-construction truncation — a DIFFERENT shape from the custom_tag rows)

Unlike phases 59/62/63 (which extended the `custom_tags` engine — the `parseCustomTags` dispatch, the `[]CustomTagSpec` model, the `ResolveCustomTags` seam), phase 64 touches NEITHER the custom-tag parse NOR the resolution seam. It sits one layer out: the `http.url` BUILT-IN attribute, whose value is constructed inline at the three `accesslog_emit.go` span-emit sites BEFORE `BuildServerSpan`. The genuinely NEW work is narrow: (a) parse one `UInt32Value` with a default-256 fallback into a new scalar field; (b) apply a byte-truncation to the `:path` portion at URL construction. The central design decisions are the reference's truncation TARGET + DEFAULT (D-MPTL-TARGET / D-MPTL-DEFAULT, §2.4) and WHERE envoy-go applies the truncation (D-MPTL-TRUNC-LOCATION, §2.5). Everything else — `BuildServerSpan`, `Span.Attrs`, `upsertAttr`, `ResolveCustomTags`, both exporters, the custom-tag machinery — is UNCHANGED.

**Contrast with phases 62/63:** those rows proudly had ZERO call-site change (the phase-62 resolution seam was already threaded). Phase 64 DOES touch the three URL-construction sites — but each becomes a single helper call replacing a single inline concatenation (the truncation is inherently at URL-build time, which lives at those sites). This is a small, mechanical, DRY change, NOT a signature or seam change. Framed honestly so the SPEC/PLAN does not mistake it for a zero-touch row.

---

## 2. Design decisions

### 2.1 Row + subject confirmation: the Observability family continues with tracing `max_path_tag_length` *(SELF-PICKED per the standing directive → phase 64 row registered)*

The FIRST decision, made AUTONOMOUSLY (no human pick) per the 2026-07-12 standing directive. Picked as the **smallest cleanly-differential-provable candidate** from a full source read of the tracing reject surface (`config.go` `NewConfig` + `parseOTel` + `parseZipkin`) AND the span-emit path (`span.go` / `accesslog_emit.go`) this session (§11). Row 64 registers `in-progress` AT this BRAINSTORM commit per the ROADMAP §Schema invariant.

**Why `max_path_tag_length` is the defensible pick:** (1) it acts on `http.url` — an ALREADY-EMITTED built-in (`span.go:70`) that is ALREADY ASSERTED cross-side and PASSES in the landed OTLP tracing differentials (`0087`/`0102`/`0105`/`0106` — the tag's format already matches the reference for short paths), so the ONLY new variable is the truncation, making it cleanly differential-provable with a single long-path fixture; (2) it needs NO new infrastructure — no dynamic-metadata path (`metadata`), no second span (`spawn_upstream_span`), no new transport (`http_service`), no per-stage annotation engine (`verbose`), no new sink (OTLP-metrics); it is a bounded numeric truncation, the smallest possible tracing increment; (3) it additionally CLOSES a latent default-256 divergence (envoy-go emits untruncated today, §1.1); (4) it is a recorded deferred candidate (phase-63 BRAINSTORM §8). The ONE genuine subtlety is honoring the DEFAULT (256) so the no-config path matches the reference — byte-stable for the existing corpus (§2.6).

**Rejected alternatives (recorded per the standing directive; each SIZED against source this session):**
- **`custom_tags` `metadata` type** — reads a value from dynamic/route/cluster/host METADATA (`CustomTag_Metadata{kind, metadata_key, default_value}`). envoy-go has no dynamic-metadata lookup path AND no MetadataKey path-traversal; dynamic (REQUEST) metadata would resolve EMPTY (no filter sets it), so it is not even cleanly differential-provable without first building a metadata subsystem. LARGE + hard-to-prove — the LAST and biggest of the four `custom_tag` sources. Deferred; STAYS rejected loudly (the SOLE remaining `custom_tags` departure).
- **`spawn_upstream_span`** — a second (upstream CLIENT) span with its own timing at the router/upstream seam; envoy-go emits ONE ingress SERVER span (`span.go` `Name: "ingress"`, `Kind: SERVER`). Medium-large; touches the span model. Deferred.
- **`http_service` (OTLP HTTP transport)** — a new HTTP exporter transport alongside `envoy_grpc` (`parseOTel` rejects it, `config.go:228-230`). Medium. Deferred.
- **`verbose`** — per-filter-stage span log annotations (`config.go:109-111` reject); needs an annotation-capture engine. Large. Deferred.
- **OTel `sampler` / `resource_detectors`** (`parseOTel` rejects, `config.go:231-236`) — a pluggable-sampler extension (parallels the existing Percent sampling) or a Resource-attribute detection path; each a new extension dispatch. Medium. Deferred.
- **force-trace (`x-envoy-force-trace`)** — needs an internal-request / edge-sanitization concept envoy-go lacks entirely. Deferred.
- **OTLP-metrics stats sink** — a full new gRPC `stats_sinks[]` consumer. The largest remaining Observability follow-on. Deferred.
- **Opening a new family** (HTTP/3 / xDS / Operational-tooling OPEN; gRPC / Runtime / WASM never-opened) — the standing directive says smallest-defensible-first, and the Observability tracing tail STILL holds a cheap candidate (`max_path_tag_length`), so smallest-first keeps us on the landed engine. Deferred.

### 2.2 Scope: `max_path_tag_length` ONLY; NO other tracing knob touched *(self-answered; the incremental-knob precedent)*

Phase 64 lifts EXACTLY ONE reject (`config.go:112-114`). The sibling rejected knobs STAY rejected loudly with their existing distinct substrings: `verbose` (`config.go:110`), `spawn_upstream_span` (`config.go:120`), `custom_tags metadata` (`config.go:207`), and the `parseOTel`/`parseZipkin` rejects (`http_service`/`resource_detectors`/`sampler`/`google_grpc`; non-`HTTP_JSON` collector version; `split_spans_for_request`). This mirrors the project's landed incremental posture (the one-knob-per-row cadence of phases 58–63). A `max_path_tag_length`-only slice is a complete, useful, deterministic capability (bounding trace-backend tag size), and the SPEC probe confirms the reference truncates identically (D-MPTL-REFTRUNC).

### 2.3 The span-emit seam is UNCHANGED: truncate the `:path` at URL CONSTRUCTION, feeding the existing `http.url` built-in — ONE path, TWO providers *(self-answered; the KV seam is landed)*

`http.url` is emitted by `BuildServerSpan` from `SpanInputs.URL` (`span.go:70`), which BOTH exporters consume (OTLP `toProto`; Zipkin `zipkin.go` reads `Attrs`). The truncation is applied to the `:path` BEFORE the URL string is built at the three `accesslog_emit.go` sites — so `BuildServerSpan`, `Span.Attrs`, `upsertAttr`, and both exporters are UNTOUCHED; only the STRING handed in via `SpanInputs.URL` becomes the truncated form. NO exporter-specific code. A custom `http.url` tag (a `literal`/`request_header`/`environment` custom_tag whose key is `http.url`) STILL upsert-OVERRIDES the built-in via `upsertAttr` (`reference_tracing_custom_tag_override_builtin`) — the truncation acts on the built-in value, and a colliding custom tag replaces it wholesale, UNCHANGED (an edge the SPEC notes but does not need a new arm for).

### 2.4 Reference truncation semantics — TARGET + DEFAULT + ZERO (proto + Envoy source) *(self-answered SHAPE; SPEC probes to PIN)*

`HttpConnectionManager.Tracing.max_path_tag_length` is a `google.protobuf.UInt32Value`. The anticipated reference behavior (Envoy `Http::Utility::buildOriginalUri(request_headers, max_path_length)` → `absl::StrCat(proto, "://", host, path.substr(0, max_path_length))`):
- **D-MPTL-TARGET** — the truncation applies to the `:path` value (path + query = envoy-go's `r.URL.RequestURI()`) — NOT to the whole `scheme://host/path` URL. The `scheme://host` prefix is added AFTER truncation, so it is never truncated. Anticipated: truncate the path portion only. PROBE to CONFIRM (a long path with a short host — assert the host survives and only the path is cut).
- **D-MPTL-DEFAULT** — when `max_path_tag_length` is ABSENT, the reference caps at **256** (`Tracing::DefaultMaxPathTagLength`, anticipated), NOT unlimited. This is the load-bearing decision: honoring it makes the no-config path match the reference (fixing the latent >256 divergence) while staying byte-stable for the < 256 existing corpus (§2.6). PROBE with a > 256 path and NO explicit knob.
- **D-MPTL-ZERO** — an EXPLICIT `max_path_tag_length: 0` (present-with-value-0) truncates the path to 0 bytes → `http.url` = `scheme://host` (Envoy `PROTOBUF_GET_WRAPPED_OR_DEFAULT` returns the explicit 0, NOT the 256 default). Anticipated: explicit 0 = empty path, NOT "unlimited". PROBE (the phase-63 explicit-0 sensitivity precedent — an explicit zero is a valid value, not "absent").
- **D-MPTL-TRUNCUNIT** — Envoy `substr` truncates by BYTES; Go `path[:n]` slices by BYTES. Anticipated BYTE parity; an ASCII-path fixture sidesteps the rune-vs-byte edge (D-MPTL-TRUNCUNIT probed only for confirmation; the fixture uses ASCII).
- **D-MPTL-QUERY** — the truncated `:path` INCLUDES the query string (the `:path` pseudo-header is path+query; `r.URL.RequestURI()` is path+query). Anticipated: query included, truncation cuts across the `?` boundary if the path alone is under the cap. PROBE (a path+query longer than the cap where the cut lands inside the query).

The SPEC live-probes each arm against `envoyproxy/envoy:contrib-v1.37.2` (fresh container per arm, `reference_probe_fresh_container_per_arm`), observing the `http.url` attribute via `test/helpers/otlptrace`.

### 2.5 Truncation LOCATION — a shared `tracing` helper at the three URL-build sites *(D-MPTL-TRUNC-LOCATION; SPEC pins)*

The `http.url` value is built inline at three sites (`accesslog_emit.go` H1 `:40` / H2 / H3), each as `scheme + "://" + r.Host + r.URL.RequestURI()`. Two shapes:
- **Option A (a shared helper) — LEAN:** a small exported `tracing.BuildHTTPURL(scheme, host, path string, maxPathTagLen uint32) string` (or similar) that truncates `path` to `maxPathTagLen` bytes and concatenates. Each site becomes one call reading `f.tracingConfig.MaxPathTagLength`. **PRO:** the truncation logic lives in ONE unit-testable place; DRY across H1/H2/H3; no `SpanInputs`/`BuildServerSpan` change. **CON:** a (mechanical) three-site edit.
- **Option B (truncate in `BuildServerSpan`):** thread the raw scheme/host/path + the cap through `SpanInputs`, truncate inside `BuildServerSpan`. **CON:** `BuildServerSpan` currently receives a pre-built `URL` string; re-deriving the path boundary from a full URL is fragile; enlarges `SpanInputs` + the signature. REJECTED-leaning.

**The decision** is D-MPTL-TRUNC-LOCATION: LEAN Option A (the shared helper). The SPEC pins the exact helper name (GREP-collision-checked, `reference_spec_drafted_identifier_collision_check`) and signature, and RE-DERIVES the three call-site line numbers.

### 2.6 Byte-stability of the DEFAULT-256 behavior change *(self-answered; the existing corpus is short-path)*

Honoring the default cap of 256 (D-MPTL-DEFAULT) is a behavior change to the no-config tracing path — but a BYTE-STABLE one for the existing differential corpus: every current OTLP/Zipkin tracing fixture (`0087`/`0088`/`0102`/`0105`/`0106`) drives short request paths (well under 256 bytes), so truncation at 256 is a no-op and their captured `http.url` is unchanged. The full 108-dir differential is anticipated byte-stable except the new `0107`. The SPEC re-confirms by running the full suite at the IMPL (the standard six-gate + N-dir gate). If ANY existing tracing fixture were found to carry a > 256 path (none anticipated), that would be a PRE-EXISTING latent divergence surfaced — recorded, not masked.

### 2.7 The reject narrows; envoy-go and the reference now AGREE on `max_path_tag_length` *(self-answered; ADR-0080)*

The `max_path_tag_length` reject is LIFTED (the departure NARROWS — envoy-go now HONORS the knob, matching the reference). The sibling tracing rejects (`verbose`/`spawn_upstream_span`/`custom_tags metadata`/`http_service`/`resource_detectors`/`sampler`/`google_grpc`/non-JSON-Zipkin/`split_spans_for_request`) STAY loud with their distinct substrings (`reference_strict_reject_sibling_typeurl_gap` — lifting one reject is an explicit per-arm change, not a fall-through). NO NEW reject arm is added (the knob is a scalar with a default — an absent field is valid, an explicit 0 is valid; there is no structural-invalid case beyond what PGV already enforces on a `UInt32Value`, which has no `min`/`max` constraint on this field — SPEC confirms D-MPTL-REJECT).

### 2.8 Fixture posture: anticipated ONE new fixture (OTLP, explicit small cap); default/zero/Zipkin unit-tested *(self-answered direction; SPEC pins D-MPTL-FIXTURE / D-MPTL-DEFAULT-PROOF)*

`http.url` is an OBSERVABLE span attribute already asserted cross-side, so the truncation IS cleanly differential-provable. A NEW `test/fixtures/NNNN-tracing-max-path-tag-length` dir (OTLP-provider) configures `max_path_tag_length` to a small EXPLICIT value and drives a GET with a request path LONGER than that cap, asserting the `http.url` attribute is truncated IDENTICALLY cross-side via the `test/helpers/otlptrace` receiver. Per the dispatch constraint (`reference_differential_fixture_dispatch_constraint` — one dir = one runner branch; do NOT mutate `0087`/`0088` baselines or the `0102`/`0105`/`0106` custom-tag fixtures), it is a NEW dir. Cloned from `0106` (the OTLP-provider template).

- **D-MPTL-FIXTURE** — the explicit-cap fixture (a long path, a small `max_path_tag_length`, cross-side `http.url` equality) is the ANCHOR proof. Cross-side VALUE-equality is achievable (unlike the phase-63 env-value case) because the truncated path is deterministic from the request — NO harness env-injection needed. Break-prove the assertion is live (`reference_differential_break_protocol_count1` — `-count=1`; `-run 'TestDifferential/<NNNN>-tracing-max-path-tag-length'`, NEVER bare — `reference_differential_run_selector`).
- **D-MPTL-DEFAULT-PROOF** — proving the DEFAULT-256 truncation (§2.4) cross-side needs a > 256-byte path. The SPEC weighs (a) a SECOND fixture with no explicit knob + a 257+-byte path vs (b) a UNIT test on the truncation helper for the default + relying on the explicit-cap fixture for the differential wire-parity. LEAN (b) — a unit test for the default (deterministic) + ONE differential fixture for the explicit cap — unless the SPEC judges the default path needs its own wire proof (then it consumes the ADR-0045 arm as a second fixture, §1.4).
- **D-MPTL-ZERO / D-MPTL-QUERY** — the explicit-0 (empty path) + the query-inclusion edges (§2.4) are anticipated proven by UNIT tests on the truncation helper (deterministic), not separate fixtures.

The Zipkin encoder path is anticipated UNIT-tested (the phase-59/62/63 precedent — one OTLP fixture + a Zipkin unit test asserting the truncated URL tag), NOT a second fixture. Anticipated: fixtures **108 → 109** — SPEC pins (and re-derives the next-free number; `0106` is the current tail, `0107` anticipated). **Harness note:** the OTLP fixture drives H1/H2 over TCP — NOT the H3/QUIC path — so `reference_differential_http_expectations_tcp_only` does not bite; the `http.url` assertion lives in the `otlptrace` receiver.

### 2.9 Fuzz posture: a SEED to the EXISTING `FuzzHCMConfigParse` — NO new fuzzer *(self-answered; count stays 55 → SPEC confirms D-MPTL-FUZZSEED)*

The tracing config parse is reached via `NewConfig` off the HCM proto, fuzzed by `FuzzHCMConfigParse` (`internal/filter/hcm/fuzz_test.go`) — the phase-59/62/63 host for the tracing parse. The new `max_path_tag_length` parse arm is exercised by ADDING a seed (a config with `max_path_tag_length` present, incl. an explicit 0) to `FuzzHCMConfigParse` — NOT a new fuzzer. Fuzzer count STAYS **55** (`reference_fuzzer_count_docs_drift`: reconcile the documented running total against actual `^func Fuzz` before AND after — the count must NOT move). SPEC confirms D-MPTL-FUZZSEED.

### 2.10 Stat surface hypothesis: +0 *(self-answered; a truncated attribute registers no stat)*

Truncating a span attribute is a wire-value change, not a stat registration. The HCM tracing counters + the tracer counters are UNCHANGED. Anticipated stat surface **1201 (+0)**, UNCHANGED.

---

## 3. Framework-survey result — a parse arm + a URL-construction truncation; ZERO new packages/modules (64 anticipated)

### 3.1 Framework: a reject lift + a scalar field + a truncation helper (no new interface, no new seam)

Phase 64 introduces NOTHING structurally new to the tracing engine: no new seam, no new interface, no `customTagKind`, no `ResolveCustomTags` change, no `BuildServerSpan` change. It adds one scalar config field (`MaxPathTagLength uint32`), one small truncation helper, and rewires three inline URL concatenations to call it. `parseCustomTags`, the `[]CustomTagSpec` model, and both exporters are UNCHANGED.

### 3.2 NEW packages: NONE

All edits land in `internal/tracing` (`config.go` parse + a truncation helper — a new file or an addition to `span.go`, both existing package) + `internal/filter/hcm` (the three `accesslog_emit.go` URL-build sites + the fuzz seed) + `test/fixtures` + `docs/`. ZERO new packages.

### 3.3 go.mod modules: NONE

`GetMaxPathTagLength()` returns a `*wrapperspb.UInt32Value`, already reachable via the resolved `github.com/envoyproxy/go-control-plane/envoy v1.32.4` HCM proto (already imported as `hcmv3`). No new module import. `go mod tidy -diff` anticipated EMPTY (modules STAY **2**).

### 3.4 REUSES

- **phase 46** the tracing engine: `TracingConfig`, `NewConfig` (the parse home + the reject roster), `BuildServerSpan`/`Span.Attrs []KV` (the built-in `http.url` attribute), both exporters, the `0087`/`0088` fixtures + the `test/helpers/otlptrace` receiver, the `FuzzHCMConfigParse` seed host.
- **phase 59/62/63** the `0102`/`0105`/`0106` OTLP custom-tag fixtures as templates (clone `0106` — the OTLP-provider shape), and the incremental-reject-lift precedent (flip one reject to a parse arm; siblings stay loud).
- **the three `accesslog_emit.go` span-emit sites** (`:40`/H2/H3) — the URL-construction points, minimally rewired to the truncation helper (§2.5).

---

## 4. Bootstrap-level applicability — a PER-LISTENER HCM tracing sub-field (NOT bootstrap `stats_sinks[]`)

`max_path_tag_length` is a PER-LISTENER HCM `tracing` sub-field, parsed by `NewConfig` from `HttpConnectionManager.tracing.max_path_tag_length` when the HCM filter is built (the phase-46/59 home). No bootstrap change; the lift lands INSIDE `NewConfig`. The fixture configures `max_path_tag_length` on the listener's HCM tracing block.

---

## 5. Stat surface hypothesis — +0 (64)

### 5.1 Stat names (SPEC confirms)

NONE. Truncating a span attribute registers no stat.

### 5.2 envoy-go-strict departure flags

The `max_path_tag_length` reject is LIFTED (the departure NARROWS — envoy-go now HONORS the knob). No new stat, no new flag; a parse+emit behavior change recorded in BEHAVIOR_CONTRACT. `metadata` STAYS the SOLE `custom_tags` departure; the other tracing knob rejects (`verbose`/`spawn_upstream_span`/`http_service`/etc.) STAY.

### 5.3 Anticipated surface arithmetic

Stat surface **1201 → 1201 (+0)**.

---

## 6. Edit-site enumeration — RE-DERIVED this session (SPEC re-derives + pins D-MPTL-TARGET / D-MPTL-DEFAULT / D-MPTL-TRUNC-LOCATION / D-MPTL-FIXTURE)

Each `file:line` RE-DERIVED against master `e0864177` this session (`feedback_brief_citations_not_evidence`); the SPEC re-derives again.

**Production — `internal/tracing/config.go`:**
1. **The `max_path_tag_length` parse arm** — replace the reject (`config.go:112-114`) with: read `t.GetMaxPathTagLength()`; if nil → default 256; else → `.GetValue()` (explicit 0 preserved). Store into a new `TracingConfig.MaxPathTagLength uint32`. The other rejects (`verbose`/`spawn_upstream_span`/custom_tags/provider) UNCHANGED. Apply to BOTH `parseOTel` and `parseZipkin` results (threaded like the sampling knobs — the truncation is provider-neutral). [EDIT]
2. **`TracingConfig`** (`config.go:25-40`) — a new `MaxPathTagLength uint32` field (collision-free; §11). [EDIT]

**Production — `internal/tracing` (helper):**
3. **A truncation helper** (`BuildHTTPURL(scheme, host, path string, maxPathTagLen uint32) string`, or similar — name GREP-checked at SPEC) — truncate `path` to `maxPathTagLen` bytes, return `scheme + "://" + host + path`. New function in `span.go` or a new small file. D-MPTL-TRUNC-LOCATION. [ADD]

**Production — `internal/filter/hcm/accesslog_emit.go`:**
4. **The THREE URL-construction sites** (`:40` H1 / H2 `:~101` / H3 `:~163`) — replace the inline `url := scheme + "://" + r.Host + r.URL.RequestURI()` with a call to the helper, passing `f.tracingConfig.MaxPathTagLength`. RE-DERIVE the exact lines at SPEC. [EDIT ×3]

**Test:**
5. **`internal/tracing/config_test.go`** — flip the `max_path_tag_length` REJECT test (`:340`, `tr.MaxPathTagLength = wrapperspb.UInt32(128)`) to an ACCEPT asserting `cfg.MaxPathTagLength == 128`; add an ABSENT→256 default test + an explicit-0 test (D-MPTL-ZERO). [EDIT + ADD]
6. **A truncation-helper unit test** (`internal/tracing/span_test.go` or a new `url_test.go`) — path under cap (unchanged), path over cap (truncated to N bytes), explicit 0 (empty path → `scheme://host`), query-inclusion (D-MPTL-QUERY), byte boundary (D-MPTL-TRUNCUNIT, ASCII). [ADD]
7. **`internal/filter/hcm/fuzz_test.go`** `FuzzHCMConfigParse` — a `max_path_tag_length` SEED (present incl. explicit 0). [ADD — no new fuzzer]

**Fixture:**
8. **`test/fixtures/NNNN-tracing-max-path-tag-length`** (new; `0107` anticipated) — an OTLP-provider listener with `max_path_tag_length` set to a small explicit value; drive a long-path GET; assert the truncated `http.url` cross-side. [ADD]

**BEHAVIOR_CONTRACT (`docs/envoy-go/BEHAVIOR_CONTRACT.md`):**
9. **the tracing section** — flip `max_path_tag_length` from "rejected (envoy-go-strict)" to "consumed (caps the `http.url` `:path` to N bytes, default 256, explicit 0 = empty path; applied on both exporters)"; the sibling knob rejects STAY. Note the default-256 behavior change (§2.6). SPEC RE-DERIVES the exact line(s). [EDIT]

**ROADMAP / STATE / DECISIONS:**
10. **ROADMAP** — row 64 `in-progress` at this BRAINSTORM (§Schema); the Observability family prose gains a "phase 64 CHARTERED and BRAINSTORMED" sentence. The LIVE deferred sentence drops `max_path_tag_length` (it was never IN the sentence — it is a §8-recorded candidate; the sentence's `custom_tags (metadata)`/OTLP/etc. content is UNCHANGED), so check-(2) still prints EXACTLY ONE live Observability match with UNCHANGED content (`reference_sentinel_deferred_sentence_live_vs_historical` — no narrow needed at this row). [BRAINSTORM: row + prose]
11. **STATE.md** — active-phase header flips to phase 64 BRAINSTORM (this stage). [EDIT]
12. **DECISIONS.md** — ADR-0285 §Context drafts at the SPEC, §Decision/§Consequences at the IMPL (ADR-0044). NOT at this BRAINSTORM. [SPEC/IMPL]

SPEC pins **D-MPTL-DOCSHAPE** (this full edit-site roster, RE-DERIVED) + **D-MPTL-TARGET/D-MPTL-DEFAULT/D-MPTL-ZERO** (§2.4) + **D-MPTL-TRUNC-LOCATION** (§2.5) + **D-MPTL-FIXTURE/D-MPTL-DEFAULT-PROOF** (§2.8).

---

## 7. Anticipated ADRs — 1 at the phase-64 IMPL: ADR-0285 (tracing `max_path_tag_length`)

ADR-0285 (tracing `max_path_tag_length` — lifting the one reject; the `UInt32Value` parse with a default-256 fallback [D-MPTL-DEFAULT] + explicit-0 preservation [D-MPTL-ZERO]; the `:path`-only byte-truncation applied at URL construction on both exporters [D-MPTL-TARGET / D-MPTL-TRUNC-LOCATION]; the byte-stable default-256 behavior change [§2.6]). §Context drafted at the SPEC (provenance: the phase-46 tracing engine + the phase-63 §8 deferred-candidate record + the `http.url` built-in), §Decision/§Consequences at the IMPL per ADR-0044. No separate seam ADR (the phase-59/62/63 precedent — a single row-scoped ADR). Next-free after: **ADR-0286**.

---

## 8. Deferred items

- **`custom_tags` `metadata` type** — a dynamic/route/cluster/host-metadata value as a span tag (needs a metadata-lookup + MetadataKey-traversal path, §2.1). The LAST `custom_tag` source type; the SOLE remaining `custom_tags` reject. Carries forward.
- **`spawn_upstream_span`** — a second (upstream CLIENT) span. Carries forward.
- **`http_service`** — an OTLP HTTP exporter transport. Carries forward.
- **`verbose`** — per-filter-stage span log annotations. Carries forward.
- **OTel `sampler` / `resource_detectors`** — a pluggable sampler / Resource-attribute detection extension. Carries forward.
- **force-trace (`x-envoy-force-trace`)** — needs internal-request detection + edge sanitization. Carries forward.
- **`OTLP-metrics` stats sink** — the largest remaining Observability sink follow-on. Carries forward.
- **The 4 EMPTY built-in span attributes** (`upstream_cluster`/`node_id`/`zone`/`peer.address`, `reference_tracing_upstream_cluster_framework_gap`) — a framework-surgery deferral, UNtouched here (`max_path_tag_length` acts on the fully-populated `http.url`, NOT those un-plumbed fields — do NOT conflate, §1.2/§11). Carries forward.
- **The DEFAULT-256 differential wire-proof** — if D-MPTL-DEFAULT-PROOF lands as a unit test (LEAN, §2.8), a > 256-path cross-side fixture proving the default remains a possible follow-on. Carries forward (low value).

After row 64 the Observability family STAYS OPEN (OTLP-metrics + `metadata` + the other tracing follow-ons remain) ⇒ the sentinel check-(2) STILL prints ⇒ the loop continues.

---

## 9. Cross-references against prior phases' deferred-items lists — pickup

Phase 64 PICKS UP tracing `max_path_tag_length` — recorded EXPLICITLY in the phase-63 BRAINSTORM §8 (`"caps the http.url tag length; still rejected; orthogonal. Carries forward."`) and implicit in the phase-59/62 tracing follow-on rosters. It is NOT in the ROADMAP Observability family's LIVE deferred SENTENCE (which names `custom_tags (metadata)`/OTLP-metrics/`spawn_upstream_span`/`http_service`/force-trace) — `max_path_tag_length` is a §8-tier candidate, so this row does NOT narrow the live sentence (its content is UNCHANGED). **Sentinel maintenance (at the IMPL):** the live Observability "candidates:" sentence content is UNCHANGED by this row, so re-run the check-(2) grep only to CONFIRM it still prints EXACTLY ONE live Observability match with the same content (`reference_sentinel_deferred_sentence_live_vs_historical`).

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution

- **D-MPTL-REFTRUNC** — CONFIRM the reference OTLP (and Zipkin) tracer truncates the `http.url` tag at `max_path_tag_length` (anticipated YES via `buildOriginalUri`, applied by the generic `HttpTracerUtility` for all tracers). The central provability probe. ONE fresh-container probe against `envoyproxy/envoy:contrib-v1.37.2` with an explicit small cap + a long path, observed via `test/helpers/otlptrace` (`reference_docker_probe_bridge_network`/`reference_host_gateway_ip_docker_desktop` for reachability). If the probe shows NO truncation (recall wrong), the SPEC RE-SCOPES. §1.1/§2.4.
- **D-MPTL-TARGET** — WHAT is truncated: the `:path` (path+query) portion BEFORE the `scheme://host` prefix (anticipated), NOT the whole URL. Probe a long path with a short host. §2.4.
- **D-MPTL-DEFAULT** — the ABSENT default: 256 (truncate — anticipated) vs unlimited. Probe a > 256 path with no explicit knob. The load-bearing behavior-change decision. §2.4/§2.6.
- **D-MPTL-ZERO** — an EXPLICIT `max_path_tag_length: 0`: truncate path to empty (`scheme://host` — anticipated) vs "unlimited". Probe. §2.4.
- **D-MPTL-TRUNCUNIT** — bytes vs runes for the substring (anticipated BYTES; ASCII fixture sidesteps). Probe for confirmation. §2.4.
- **D-MPTL-QUERY** — is the query string included in the truncated `:path` (anticipated YES — `:path` is path+query). Probe with a cut landing inside the query. §2.4.
- **D-MPTL-TRUNC-LOCATION** — a shared `tracing` helper at the three URL-build sites (Option A, LEAN) vs threading path+cap through `SpanInputs` into `BuildServerSpan` (Option B). Pin the helper name (GREP-collision-checked, `reference_spec_drafted_identifier_collision_check`) + signature + the three RE-DERIVED call-site lines. §2.5.
- **D-MPTL-FIXTURE** — ONE new OTLP fixture (`0107` anticipated; RE-DERIVE): a long path + a small explicit cap, cross-side `http.url` VALUE-equality (deterministic — no harness env-injection). New dir (`reference_differential_fixture_dispatch_constraint`); break-prove live (`reference_differential_break_protocol_count1`/`reference_differential_run_selector`). Fixtures **108 → 109**. §2.8.
- **D-MPTL-DEFAULT-PROOF** — prove the default-256 truncation via a UNIT test (LEAN) vs a second > 256-path fixture. §2.8.
- **D-MPTL-REJECT** — CONFIRM no new structural reject is needed (a `UInt32Value` with no PGV min/max on this field; absent + explicit-0 both valid). RE-DERIVE against the project-pinned go-control-plane. §2.7.
- **D-MPTL-FUZZSEED** — a SEED to the EXISTING `FuzzHCMConfigParse` (NOT a new fuzzer); fuzzer count STAYS 55 (`reference_fuzzer_count_docs_drift` — reconcile before AND after). §2.9.
- **D-MPTL-SPLIT** — the ADR-0045 disposition (SINGLE FLAT ROW anticipated, ~8–12 tasks; escape-valve armable only if D-MPTL-DEFAULT-PROOF balloons into a second fixture leg). §1.4.

---

## 11. Prior-phase lessons applied

- **`feedback_brief_citations_not_evidence`** — EVERY `file:line` here (the `config.go:112-114` reject, `span.go:70` `http.url`, the `accesslog_emit.go:40` URL construction, the `config_test.go:340` reject test) is RE-DERIVED from source at the SPEC. This session RE-DERIVED them live against master `e0864177` — notably CONFIRMING `http.url` is a fully-populated built-in (NOT one of the 4 EMPTY framework-gap attributes) and that the URL is built inline at the three span-emit sites (so the truncation lands there, NOT in `BuildServerSpan`).
- **`reference_spec_drafted_identifier_collision_check`** — the new `MaxPathTagLength` field name + the truncation-helper name are GREP-checked in `internal/tracing` before the PLAN adopts them. This session confirmed `MaxPathTagLength` is collision-free (the only hit is the PROTO field `tr.MaxPathTagLength` in the `config_test.go:340` reject test — a different struct). The SPEC re-checks the helper name.
- **`reference_tracing_upstream_cluster_framework_gap`** — `http.url` is a POPULATED built-in, NOT one of the 4 EMPTY attributes (`upstream_cluster`/`node_id`/`zone`/`peer.address`); the truncation acts on a real value — INDEPENDENT of that gap (do NOT conflate). §1.2/§8.
- **`reference_tracing_custom_tag_override_builtin`** — a custom_tag keyed `http.url` STILL upsert-overrides the (now-truncated) built-in via `upsertAttr`; the truncation and the override compose cleanly (no new arm). §2.3.
- **`reference_probe_fresh_container_per_arm`** + **`reference_envoy_contrib_image_tagging`** — each SPEC probe arm (D-MPTL-REFTRUNC / D-MPTL-TARGET / D-MPTL-DEFAULT / D-MPTL-ZERO / D-MPTL-QUERY / D-MPTL-TRUNCUNIT) runs on a FRESH container against `envoyproxy/envoy:contrib-v1.37.2`. §10.
- **`reference_docker_probe_bridge_network`** + **`reference_host_gateway_ip_docker_desktop`** — the OTLP span probes need a shared bridge network + a reachable receiver; verify the span decode ACTUALLY ran (not a vacuous empty capture). §10.
- **`reference_differential_fixture_dispatch_constraint`** — a new fixture dir per runner branch; do NOT mutate `0087`/`0088` (baselines) or `0102`/`0105`/`0106` (the custom-tag fixtures). §2.8.
- **`reference_differential_break_protocol_count1`** + **`reference_differential_run_selector`** — the `0107` assertion break-proof uses `-count=1` and `-run 'TestDifferential/0107-tracing-max-path-tag-length'` (NEVER bare `-run '0107'`, which matches zero subtests → vacuous green). §2.8.
- **`reference_differential_http_expectations_tcp_only`** — the OTLP fixture is H1/H2-over-TCP (NOT H3/QUIC), so the `http.url` assertion lives in the `otlptrace` receiver, not `HTTPExpectations`. §2.8.
- **`reference_fuzzer_count_docs_drift`** — a SEED, not a fuzzer; reconcile the documented running total (55) against actual `^func Fuzz` before AND after — the count must NOT move. §2.9.
- **`reference_sentinel_deferred_sentence_live_vs_historical`** — this row does NOT narrow the live Observability sentence (`max_path_tag_length` is a §8-tier candidate, not IN the sentence); re-run the check-(2) grep at the IMPL only to CONFIRM it still prints EXACTLY ONE live Observability match with UNCHANGED content. §9.
- **`reference_strict_reject_sibling_typeurl_gap`** / **ADR-0080** — the sibling tracing rejects (`verbose`/`spawn_upstream_span`/`metadata`/`http_service`/etc.) keep their DISTINCT substrings; lifting `max_path_tag_length` is an explicit per-arm change (not a fall-through). §2.7.
- **`reference_fatalf_makes_assertions_unreachable`** — the config/truncation unit tests assert each independent property with `Errorf` (not `Fatalf`), so a truncation failure does not mask the default/zero/query assertions. §6.

---

## 12. Section closeout

**Settled:** subject (tracing `max_path_tag_length`, SELF-PICKED per the standing directive as the smallest CLEANLY-DIFFERENTIAL-PROVABLE remaining candidate — a bounded numeric truncation on the ALREADY-emitted, ALREADY-cross-side-asserted `http.url` built-in — over `metadata` [needs a dynamic-metadata subsystem envoy-go lacks + not cleanly provable] and the larger declined alternatives, §2.1); scope (`max_path_tag_length` lifted; the sibling tracing knob rejects STAY loud with distinct substrings, §2.2/§2.7); the span-emit seam (UNCHANGED — truncate the `:path` at URL construction, feeding the existing `http.url` built-in; both exporters covered, §2.3); the reference semantics (truncate the `:path` portion to N bytes; default 256; explicit 0 = empty path; query included — all anticipated, SPEC-probed, §2.4); the truncation LOCATION (a shared `tracing` helper at the three URL-build sites — a small mechanical 3-site edit, NOT a zero-touch row, §2.5); the byte-stable default-256 behavior change (short-path corpus unaffected, §2.6); fixture posture (ONE new OTLP fixture — a long path + a small explicit cap, cross-side VALUE-equality, no harness surgery; default/zero/query/Zipkin unit-tested, §2.8); fuzz posture (a SEED to `FuzzHCMConfigParse`, no new fuzzer, §2.9); stat surface (+0, §2.10); envelope (SINGLE FLAT ROW anticipated, ~8–12 tasks — ADR-0285, §1.4). The novel production code is just the `UInt32Value`-with-default-256 parse arm + the `:path` byte-truncation helper + its three call-site wirings; the row's ONE genuinely novel test-side piece is the long-path cross-side fixture (D-MPTL-FIXTURE — deterministic, no env-injection).

**Anticipated moves at the phase-64 IMPL (docs-only now):** the `max_path_tag_length` parse arm (lift `config.go:112-114`) + the `MaxPathTagLength uint32` field + the truncation helper + its three call-site wirings + config/truncation unit tests + a `FuzzHCMConfigParse` seed + the new OTLP fixture + the BEHAVIOR_CONTRACT tracing edit + ADR-0285. Counts: stat surface **1201 (+0)** · fixtures **108 → 109** · fuzzers **55 (+0, seed only)** · BackendKind **38 (+0)** · DECISIONS tail **ADR-0285** (next-free **ADR-0286**) · new Go packages **0** · new go.mod modules **0**.

**Counts UNCHANGED at this BRAINSTORM (docs-only; re-verified against master tip `e0864177`):** stat surface **1201** · fixtures **108** · fuzzers **55** · BackendKind **38** · DECISIONS tail **ADR-0284** (next-free **ADR-0285**) · go.mod modules **2**. Row 64 registers `in-progress` at this BRAINSTORM commit per the §Schema invariant.

**Next → the phase-64 SPEC** (the D-MPTL-* live-probe arms against `envoyproxy/envoy:contrib-v1.37.2` — D-MPTL-REFTRUNC / D-MPTL-TARGET / D-MPTL-DEFAULT / D-MPTL-ZERO / D-MPTL-QUERY / D-MPTL-TRUNCUNIT; re-derive every §6 edit site + the truncation-helper name collision-check + the three `accesslog_emit.go` call-site lines; pin D-MPTL-TRUNC-LOCATION + D-MPTL-FIXTURE + D-MPTL-DEFAULT-PROOF; draft ADR-0285 §Context).
