# SPEC 62 — tracing `custom_tags` `request_header` SOURCE arm (the THIRTEENTH Observability-family row; the SECOND `custom_tag` source type after the phase-59 `literal`; the FIRST PER-REQUEST-resolved custom tag — lifts the `internal/tracing/config.go:155-156` `request_header type unsupported` reject and reads a named DOWNSTREAM REQUEST header into a `{tag, value}` STRING span attribute [with `default_value` / omit-on-missing]; `environment`/`metadata` STAY parse-rejected loudly — an envoy-go-strict DEPARTURE, ADR-0080; anticipated ONE new OTLP fixture / +0 stats / +0 packages / +0 modules — a SINGLE FLAT ROW, ADR-0283)

> **Stage:** SPEC (lifecycle-state 1 → 2). Docs-only; NO production `.go` changes at this stage. Fresh worktree `.worktrees/phase-62-spec`, branch `phase-62-tracing-request-header-custom-tag-spec`, per `feedback_git_worktrees`.
>
> **ANCHORS ADR-0283 §Context DRAFT** (§Decision/§Consequences land at the phase-62 IMPL per ADR-0044; DECISIONS tail STAYS **ADR-0282** at this SPEC).
>
> **Baselines re-verified against master tip `664fb2dd` (the phase-62 BRAINSTORM squash):** stat surface **1201** · fixtures **106** (`ls -d test/fixtures/[0-9]*/ | wc -l`; tail `0104-http3-downstream-get`) · fuzzers **55** (verified `55` actual `^func Fuzz`) · BackendKind tail **38** (`H2GoawayResponder`) · DECISIONS tail **ADR-0282** (next-free **ADR-0283**) · new Go packages **0** · go.mod modules **2** (`quic-go` direct + `qpack` indirect). Counts UNCHANGED at this SPEC (docs-only). Every `file:line` below was RE-DERIVED from source this session (`feedback_brief_citations_not_evidence`) — the roster is §12.

---

## 1. Purpose / Mission

Phase 62 lifts the one `request_header` reject (`internal/tracing/config.go:155-156`) and supports the **`request_header`** `CustomTag` type: an operator configuring `custom_tags: [{tag: "user_id", request_header: {name: "x-user-id", default_value: "anon"}}]` on the HCM `tracing` block gets a `{key: "user_id", value: <the x-user-id header value>}` STRING attribute on every sampled ingress SERVER span — on **both** the OTLP and the Zipkin exporter (the resolved tag flows through the shared `Span.Attrs []KV` / `upsertAttr` seam landed at phase 59). This is the FIRST custom tag whose value is NOT static config — it is read per-request from a named DOWNSTREAM REQUEST header. The two remaining request-driven types (`environment`/`metadata`) STAY **PARSE-REJECTED loudly** with their existing distinct substrings (envoy-go-strict DEPARTURE — the reference SUPPORTS them, §11 arm A boots-and-emits). A NEW PGV-parity structural reject (empty `request_header.name`) mirrors the reference's boot-reject (both reject — NOT a departure; §11 arm E).

ADR-0283 §Context is DRAFTED here (§13); §Decision/§Consequences at the IMPL (ADR-0044). **All eleven BRAINSTORM D-RH-* questions are DISPOSED at this SPEC** — the empirical arms via LIVE probes against `envoyproxy/envoy:contrib-v1.37.2` (§11, fresh container per arm, `reference_probe_fresh_container_per_arm`):

- **D-RH-SCOPE** — DISPOSED: `request_header` added; `environment`/`metadata` reject loudly (§2, §6).
- **D-RH-WIRE** — PINNED (§11 arm A `t_present`): `{key: <tag>, value: {stringValue: <header value>}}` — a STRING attribute, upserted against the built-ins.
- **D-RH-MISSING** — PINNED (§11 arm A, three sub-arms): header present → the header value; header absent + `default_value` set → the default; header absent + `default_value` empty/unset → **the tag is OMITTED entirely** (a NEW semantic vs `literal`, which never omits).
- **D-RH-MULTIVALUE** — PINNED (§11 arm A `t_multi`): a header sent twice yields the **FIRST value** (`"MV-A"`), NOT a comma-join. envoy-go uses `lookup(name)[0]`.
- **D-RH-PRECEDENCE** — PINNED (§11 arms B/C/D), **REFINES the landed engine** (§1.1): (a) a `request_header` tag colliding with a BUILT-IN key OVERRIDES it (upsert, arm B — same as `literal`); (b) among CUSTOM tags with the SAME key, the **FIRST in config order WINS** (arms C/D) — the reference deduplicates `custom_tags` by key keeping the first occurrence, which CONTRADICTS the landed `upsertAttr` last-wins.
- **D-RH-CONFIGMODEL** — DECIDED (§3.2): the phase-59 static `TracingConfig.CustomTags []KV` becomes an ORDERED `[]CustomTagSpec`, deduplicated by tag key (FIRST-wins) at parse (matching the reference, §1.1).
- **D-RH-RESOLVE-SEAM** — DECIDED (§3.3): a new `ResolveCustomTags(specs []CustomTagSpec, headerLookup func(string) ([]string, bool)) []KV` in `internal/tracing`, threaded at the THREE `accesslog_emit.go` call sites (`:55`/`:116`/`:177`) with `reqHeaderLookupH1`/`reqHeaderLookupH2`; `BuildServerSpan` stays UNCHANGED (still `customTags []KV`, still upserts).
- **D-RH-REJECT** — PINNED (§6, §11 arm E + PGV re-derivation): `environment`/`metadata` reject substrings UNCHANGED; empty `request_header.name` → a PGV-parity structural reject (the reference boot-rejects, §11 arm E); the reference ACCEPTS `request_header` (arm A) ⇒ the departure genuinely narrows.
- **D-RH-FIXTURE** — DECIDED (§8): ONE new OTLP fixture `0105-tracing-custom-tags-request-header` (fixtures **106 → 107**); the default/omit/multi-value/precedence-dedup edges are UNIT tests; the Zipkin encoder path is a unit test (the phase-59 precedent).
- **D-RH-FUZZSEED** — DECIDED (§6): a seed to the existing `FuzzHCMConfigParse`; fuzzer count stays **55**.
- **D-RH-SPLIT** — DECIDED (§3.0): a SINGLE FLAT ROW (~11–13 tasks), ADR-0045 escape-valve UNCONSUMED.
- **D-RH-DOCSHAPE** — the full RE-DERIVED edit-site roster (§12).

No PLAN-time empirical question remains; the PLAN is a mechanical TDD decomposition.

### 1.1 Empirical-finding-driven refinement (per ADR-0044): custom-tag dedup is FIRST-wins

The BRAINSTORM (§2.5) anticipated an ordered `[]CustomTagSpec` that preserves config order "so cross-type same-key `upsertAttr` collisions stay reference-faithful" — but ASSUMED the landed last-wins `upsertAttr` was the reference rule. The SPEC-62 live probes (§11 arms C/D) PINNED the OPPOSITE for custom-vs-custom collisions: a `dup` tag configured as `[literal, request_header]` emits the **literal** value (arm C), and configured as `[request_header, literal]` emits the **request_header** value (arm D) — i.e. the **FIRST** custom tag with a given key wins, and later same-key custom tags are DROPPED. This matches Envoy's config-time behavior of storing `custom_tags` in a map keyed by tag name (emplace = insert-if-absent = first-wins). Custom tags STILL override built-ins (arm B, upsert-after-built-ins).

This is a material refinement, and it exposes a LATENT phase-59 divergence: the landed `BuildServerSpan` applies literal custom tags via `upsertAttr` (last-wins, `span.go:99-101`/`:121`), so TWO literal tags with the same key would emit the SECOND value — diverging from the reference's first-wins. Phase-59's fixture used a single unique-key literal, so the divergence was never exercised. Phase 62 CORRECTS it by moving the dedup to config-parse time (§3.2): `parseCustomTags` builds the ordered `[]CustomTagSpec` with **first-wins dedup by tag key** (a `seen` set). The resolved `[]KV` handed to `BuildServerSpan` then has UNIQUE keys, so `upsertAttr`'s only remaining job is overriding a colliding BUILT-IN (arm B) — `upsertAttr` and `BuildServerSpan` stay UNCHANGED. The common single-key case is byte-stable; the `0102` literal differential stays green (§3.6). This is the only anticipation the probes overturned; every other BRAINSTORM decision held.

---

## 2. Non-purposes (deferred; per BRAINSTORM §1.2 + §8)

NO `environment`/`metadata` custom-tag types (rejected loudly, §6). NO `spawn_upstream_span`/`http_service`/force-trace. NO `max_path_tag_length` (a distinct, still-rejected knob, `config.go:79-80`). NO OTLP-metrics stats sink. NO new provider, transport, or stat. The four built-in span attributes emitted EMPTY by envoy-go (`upstream_cluster`/`node_id`/`zone`/`peer.address`, the `reference_tracing_upstream_cluster_framework_gap` framework gap) are UNTOUCHED — a `request_header` tag reads a REQUEST HEADER (fully available at the seam via the existing lookups), NOT those un-plumbed upstream/node/zone/peer per-request fields; do NOT conflate. **Empty-header-VALUE edge (modeled, not probed):** the proto doc keys omission on the header NOT EXISTING (`custom_tag.pb.go:269-271`), and the existing lookups report existence via their `bool` return, so an EXISTING header with an empty value emits an empty-string tag (present, `""`) rather than the default; this edge is modeled from the proto semantics + the lookup contract, NOT live-probed (the fixture does not exercise it), and is recorded here as a documented modeling choice for the IMPL.

---

## 3. The change — lift the `request_header` reject, an ordered deduped `[]CustomTagSpec`, a per-request `ResolveCustomTags`, three call-site threadings (ADR-0283)

### 3.0 Split disposition — a SINGLE FLAT ROW; the ADR-0045 escape-valve UNCONSUMED

Anticipated **~11–13 tasks** (§10), under the ADR-0045 `~15` ceiling. There is no second subsystem to strand: the parse arm, the config-model reshape, the resolver, and the three call-site threadings all sit on the SAME tracing engine. The escape-valve is documented ARMABLE but UNCONSUMED — the single new OTLP fixture (§8) does not push the fixture leg into its own row.

### 3.1 The parse arm — lift the `request_header` reject in `parseCustomTags` (`internal/tracing/config.go`)

`parseCustomTags` (`config.go:139`) currently returns `([]KV, error)` and rejects `request_header` at `config.go:155-156`. Reshape it to return `([]CustomTagSpec, error)` (§3.2) and, per tag (after the empty-`tag` guard at `config.go:145-147`, unchanged):
- `ct.GetLiteral() != nil` → ACCEPT (unchanged reject of empty `literal.value`): append `CustomTagSpec{Key: tag, Kind: literal, LiteralValue: value}`.
- `ct.GetRequestHeader() != nil` → **ACCEPT (NEW)**: read `h := ct.GetRequestHeader()`; reject an empty `h.GetName()` as `tracing: custom_tags request_header tag %q empty name` (PGV-parity — `custom_tag.pb.validate.go:583`, §6); else append `CustomTagSpec{Key: tag, Kind: requestHeader, HeaderName: h.GetName(), DefaultValue: h.GetDefaultValue(), HasDefault: h.GetDefaultValue() != ""}`.
- `ct.GetEnvironment() != nil` → reject `tracing: custom_tags environment type unsupported` (DEPARTURE, unchanged).
- `ct.GetMetadata() != nil` → reject `tracing: custom_tags metadata type unsupported` (DEPARTURE, unchanged).
- else (typeless) → reject `tracing: custom_tags tag %q missing type` (unchanged).

**First-wins dedup by tag key (§1.1).** A `seen map[string]struct{}` skips appending a spec whose `Key` already appeared (the FIRST occurrence survives; a later same-key tag — of ANY source type — is dropped). This matches the reference's config-time map (§11 arms C/D). Dedup runs AFTER the per-tag structural validation (so a later duplicate-key tag with an invalid name still boot-rejects — parity with the reference's PGV, which validates every entry before building the map). Note on `HasDefault`: PGV places NO constraint on `default_value` (`custom_tag.pb.validate.go`, no `Header.DefaultValue` rule), so an empty `default_value` is valid and means "omit on missing header" (§3.3); `HasDefault` is derived as `default_value != ""` — the empty-string default is indistinguishable from an unset one on the wire and behaves identically (both ⇒ omit), so this derivation is exact.

### 3.2 The config model — `TracingConfig.CustomTags []CustomTagSpec` (D-RH-CONFIGMODEL)

Replace the phase-59 `TracingConfig.CustomTags []KV` field (`config.go:37`) with `CustomTags []CustomTagSpec` — an ordered, first-wins-deduped spec list. A NEW in-package type:

```go
// CustomTagSpec is one parsed HCM tracing custom_tag, resolved per-request by
// ResolveCustomTags. Kind selects the source: a static literal value, or a
// request-header lookup (with an optional default / omit-on-missing).
type customTagKind uint8
const ( customTagLiteral customTagKind = iota; customTagRequestHeader )

type CustomTagSpec struct {
    Key          string        // the span-attribute key (CustomTag.tag)
    Kind         customTagKind
    LiteralValue string        // Kind==literal: the static value
    HeaderName   string        // Kind==requestHeader: the header to read
    DefaultValue string        // Kind==requestHeader: value when the header is absent
    HasDefault   bool          // Kind==requestHeader: DefaultValue != "" (else omit on absent)
}
```

The switch-restructure that sets `cfg.CustomTags = customTags` after the provider dispatch (`config.go:88`/`:127`) is UNCHANGED in shape — only the field type changes. `parseOTel`/`parseZipkin` stay untouched (provider-neutral).

### 3.3 The per-request resolver — `ResolveCustomTags` (D-RH-RESOLVE-SEAM; `internal/tracing`)

A NEW exported function (anticipated `internal/tracing/resolve.go`):

```go
// ResolveCustomTags resolves the ordered (already first-wins-deduped) specs
// against a per-request header lookup into span attributes. Literal specs yield
// their static value; request_header specs yield the FIRST value of the named
// header (D-RH-MULTIVALUE), or the DefaultValue when the header is absent and a
// default was configured, or NOTHING when the header is absent and no default
// was set (omit-on-missing — D-RH-MISSING). headerLookup may be nil (no request
// headers available) ⇒ request_header specs use default / omit.
func ResolveCustomTags(specs []CustomTagSpec, headerLookup func(string) ([]string, bool)) []KV {
    if len(specs) == 0 { return nil }
    out := make([]KV, 0, len(specs))
    for _, s := range specs {
        switch s.Kind {
        case customTagLiteral:
            out = append(out, KV{Key: s.Key, Str: s.LiteralValue})
        case customTagRequestHeader:
            if headerLookup != nil {
                if vs, ok := headerLookup(s.HeaderName); ok && len(vs) > 0 {
                    out = append(out, KV{Key: s.Key, Str: vs[0]}) // FIRST value, arm A t_multi
                    continue
                }
            }
            if s.HasDefault {
                out = append(out, KV{Key: s.Key, Str: s.DefaultValue})
            } // else omit (append nothing) — arm A t_missnodef
        }
    }
    return out
}
```

The returned `[]KV` has unique keys (the specs were deduped at parse, §3.2). `BuildServerSpan` upserts each against the built-ins (§3.5). **Existence semantics:** `ok` (the lookup's `bool`) is TRUE when the header is present (even with an empty value), so an existing empty-valued header emits `KV{Key, ""}` (present) rather than the default (§2 modeled edge).

### 3.4 The call-site threading (`internal/filter/hcm/accesslog_emit.go`)

The THREE `BuildServerSpan` call sites resolve the specs against the request's header lookup:
- `:55` (H1, `emitAccessLog`, `r *http.Request`): `tracing.ResolveCustomTags(f.tracingConfig.CustomTags, reqHeaderLookupH1(r))`.
- `:116` (H2, `emitAccessLogH2`, `req h2.H2Request`): `... reqHeaderLookupH2(req)`.
- `:177` (H3, `emitAccessLogH3`, `r *http.Request`): `... reqHeaderLookupH1(r)` (H3 carries an `http.Request`, so it reuses the H1 lookup).

`reqHeaderLookupH1` (`accesslog_emit.go:218`) and `reqHeaderLookupH2` (`:228`) are the EXISTING case-insensitive `func(string) ([]string, bool)` lookups used by access-log header capture — REUSED unchanged. **Nil-safety (RE-DERIVED, unchanged from phase 59):** all three sites are guarded by `f.exporter != nil`, and `f.exporter != nil ⟹ f.tracingConfig != nil`, so `f.tracingConfig.CustomTags` is safe; the lookup constructors are pure (never nil for a live request). When `CustomTags` is empty the resolver returns `nil` and `BuildServerSpan`'s upsert loop is a no-op (byte-stable, §3.6).

### 3.5 Precedence — `BuildServerSpan` UNCHANGED (upsert vs built-in only)

`BuildServerSpan` (`span.go:64`) and `upsertAttr` (`span.go:121`) are UNCHANGED. The resolver hands it a unique-key `[]KV`; each is upserted after the 16 built-ins + optional `guid:x-client-trace-id`, so a custom tag whose key collides with a built-in OVERRIDES it (arm B) — the landed behavior. Custom-vs-custom collisions never reach `upsertAttr` (deduped at parse, §3.2), so its last-wins branch is only ever exercised against built-ins, where last-wins == "custom overrides built-in" == the reference (arm B). No change to `span.go`.

### 3.6 Byte-stability — no behavior change on the existing paths

A tracing HCM with no `custom_tags` parses to `CustomTags == nil`; the resolver returns `nil`; every existing span is byte-identical. A single literal custom tag (the `0102` fixture) parses to one `CustomTagSpec{Kind: literal}`, resolves to the same `KV{custom_env, prod-literal}` as today — the `0102` differential stays byte-stable. The 106-dir differential is unaffected except the new `0105` dir.

---

## 4. Framework primitives — 0 new packages, 0 new go.mod modules

All edits land in `internal/tracing` + `internal/filter/hcm` (both existing) + `test/fixtures` + `docs/`. The `CustomTag_Header` proto is reachable via the ALREADY-imported `github.com/envoyproxy/go-control-plane/envoy v1.32.4` module (`config.go` already imports `tracingv3`, phase 59). `ct.GetRequestHeader()` returns `*tracingv3.CustomTag_Header` with `GetName()`/`GetDefaultValue()` (§5). NO new package, NO new module, NO new interface. `go mod tidy -diff` anticipated EMPTY (modules STAY **2**).

---

## 5. Proto-field roster — `type.tracing.v3.CustomTag_Header` (RE-DERIVED @ go-control-plane/envoy v1.32.4, `type/tracing/v3/custom_tag.pb.go`)

| Field | Getter | Phase-62 disposition |
|---|---|---|
| `CustomTag.tag` (1, string) | `GetTag()` | the attribute KEY; empty ⇒ reject (unchanged) |
| `CustomTag.request_header` (4, `CustomTag_Header`) | `GetRequestHeader()` `:106` | **ACCEPT (NEW)** ⇒ per-request resolve |
| `CustomTag_Header.name` (1, string) | `GetName()` `:307` | the header to read; empty ⇒ reject (PGV-parity, `:583`) |
| `CustomTag_Header.default_value` (2, string) | `GetDefaultValue()` `:314` | value on absent header; empty ⇒ omit-on-missing |
| `CustomTag.environment` (3) | `GetEnvironment()` | reject (DEPARTURE, unchanged) |
| `CustomTag.metadata` (5) | `GetMetadata()` | reject (DEPARTURE, unchanged) |

---

## 6. PARSE-REJECT roster + fuzzer

**Reject taxonomy** (all ADR-0080-distinct substrings):

**Tier A — PGV-parity structural rejects (BOTH reject; NOT a departure).** envoy-go mirrors these explicitly (no PGV at runtime):
- empty `tag` → `tracing: custom_tags empty tag` (unchanged, ref PGV `:64`).
- empty `literal.value` → `tracing: custom_tags literal tag %q empty value` (unchanged, ref PGV `:358`).
- typeless tag → `tracing: custom_tags tag %q missing type` (unchanged).
- **empty `request_header.name` → `tracing: custom_tags request_header tag %q empty name` (NEW)** — ref PGV `custom_tag.pb.validate.go:583` (`Header.name` `min_len:1`), CONFIRMED LIVE (§11 arm E: `HeaderValidationError.Name: value length must be at least 1 characters`). (The `Header.name` pattern `^[^\x00\n\r]*$`, `:594`, is a stricter reference-only constraint; envoy-go's `min_len` mirror is the practical parity — the SPEC does not mirror the control-char pattern, a documented narrow-parity choice consistent with prior rows.)

**Tier B — envoy-go-strict DEPARTURES (the reference ACCEPTS — §11 arm A; envoy-go rejects):**
- `environment` → `tracing: custom_tags environment type unsupported` (unchanged).
- `metadata` → `tracing: custom_tags metadata type unsupported` (unchanged).

The `request_header` reject (`config.go:155-156`) is REMOVED (the arm now parses). §11 arm A confirms the reference ACCEPTS `request_header` (booted + emitted), so lifting it genuinely narrows the departure.

**Fuzzer (D-RH-FUZZSEED).** The custom_tags parse is reached by `FuzzHCMConfigParse` (`internal/filter/hcm/fuzz_test.go:25`) — the phase-59 host. Add ONE seed: a `Tracing` block with a `request_header` custom_tag (name + default) + a mixed literal+request_header config, via the existing `mkHCM` helper. **This is a SEED, not a new `func Fuzz` — fuzzer count STAYS 55** (`reference_fuzzer_count_docs_drift`: reconcile actual `^func Fuzz` = 55 before AND after).

---

## 7. Stat surface — +0 (1201 → 1201)

A span attribute is emitted on the wire, not registered as a stat. The 5 HCM tracing decision counters + the 2 tracer counters are UNCHANGED. Stat surface **1201 (+0)**.

---

## 8. Differential fixture taxonomy — +1 (D-RH-FIXTURE: ONE new OTLP fixture)

Per the dispatch constraint (`reference_differential_fixture_dispatch_constraint`) the `0087`/`0088`/`0102` fixtures are NOT mutated. **ONE new dir `test/fixtures/0105-tracing-custom-tags-request-header`** (fixtures **106 → 107**; RE-DERIVE the next-free number at the IMPL — `0104` is the current tail):

- Cloned from `0102-tracing-custom-tags-literal` (OTLP provider, `test/helpers/otlptrace` receiver, `host.docker.internal` STRICT_DNS per ADR-0010).
- The HCM `tracing` block: `custom_tags: [{tag: "trace_user", request_header: {name: "x-trace-user", default_value: "anon"}}]` — a **NON-colliding** key.
- The driver drives ONE request carrying `x-trace-user: <value>` and asserts (by KEY, attribute order non-deterministic — §11) that the captured span carries `{key: "trace_user", value: <value>}` on BOTH the reference AND subject side. Assert each independent property with `Errorf`, NOT `Fatalf` (`reference_fatalf_makes_assertions_unreachable`). `BackendCount` ≥ 1 (`reference_differential_backendcount_min_one`).
- The driver must SEND the header cross-side: the runner's HTTP client drives the request — confirm the driver sets `x-trace-user` on the driven request (the `0087`/`0102` drivers issue plain GETs; this driver adds one request header). This is the ONE new driver capability.
- Prove the new assertion LIVE with a deliberate `-count=1` break (`reference_differential_break_protocol_count1`), confirming WHICH assertion fires (`reference_deliberate_break_wrong_assertion`).

**The default / omit-on-missing / multi-value / precedence-dedup edges** are UNIT tests on `ResolveCustomTags` (§10 — deterministic, no differential). **The Zipkin encoder path** (a resolved request_header tag surfacing in the `tags` map) is a UNIT test (the phase-59 precedent — one OTLP fixture + a Zipkin unit test). This keeps the row a SINGLE FLAT ROW. Fixtures **106 → 107** (NOT 108). *(A second cross-side request for the default case was weighed and DEFERRED to the PLAN as an optional refinement — the present case is the load-bearing cross-side proof; the default/omit paths are fully covered by the deterministic unit tests.)*

---

## 9. Behavior-contract delta (`docs/envoy-go/BEHAVIOR_CONTRACT.md`; ADR-0283 atomic landing at the IMPL)

- The tracing `custom_tags` clause (RE-DERIVE the exact lines at the IMPL; the phase-59 IMPL wrote them ~686/739) — flip `request_header` from "STRICT-REJECT (envoy-go-strict departure)" to "CONSUMED (the named request header's FIRST value is emitted as a `{tag, value}` STRING span attribute on both exporters; `default_value` on an absent header; OMITTED when absent with no default; FIRST-wins dedup on a duplicate tag key; OVERRIDES a colliding built-in)"; `environment`/`metadata` STAY STRICT-REJECT; ADD the empty-`request_header.name` PGV-parity reject.

(Exact final wording RE-DERIVED and written at the IMPL.)

---

## 10. Test plan + per-task structure (~11–13 tasks; PLAN decomposes)

TDD (`superpowers:test-driven-development`); each task a red→green with a `-count=1` liveness break where an assertion is load-bearing. Anticipated tasks:

1. **Config model + parse arm** — the `CustomTagSpec` type + `customTagKind`; reshape `parseCustomTags` to `([]CustomTagSpec, error)` with the `request_header` accept arm + the empty-name reject + first-wins dedup (§3.1/§3.2); the `TracingConfig.CustomTags` field type change. `config_test.go`: accept a `request_header` tag (assert the parsed spec); reject empty-name + (unchanged) environment/metadata/empty-tag/typeless; dedup keeps the FIRST of two same-key tags. `-count=1` break per new reject / the dedup assertion.
2. **`ResolveCustomTags`** — the resolver (§3.3) in `resolve.go`. `resolve_test.go`: header present → first value; absent + default → default; absent + no default → OMIT; multi-value → first; nil lookup → default/omit; literal → static; a mixed deduped spec list resolves in order.
3. **`span.go`/`BuildServerSpan` call-site updates** — NO signature change; update the `span_test.go`/`zipkin_test.go` call sites only if the field-type change ripples (the tests pass `nil`/`[]KV` directly to `BuildServerSpan`, which is unchanged — likely NO edit; confirm). Add a `span_test.go` case: a resolved request_header `KV` upserts over a colliding built-in (arm B).
4. **Zipkin encoder unit test** — a span with a resolved request_header tag encodes into the Zipkin `tags` map.
5. **Call-site threading** — `accesslog_emit.go:55`/`:116`/`:177` call `ResolveCustomTags(..., reqHeaderLookupH1/H2(...))` (§3.4). Covered by the existing HCM span tests + a new test asserting a request-header tag reaches the span.
6. **New OTLP fixture `0105-tracing-custom-tags-request-header`** — envoy.yaml, envoy-go.yaml, expectations.yaml, driver (sends `x-trace-user`), README (§8); the request-header span assertion proven live.
7. **`FuzzHCMConfigParse` seed** — one request_header custom_tags seed (§6); reconcile fuzzer count = 55.
8. **BEHAVIOR_CONTRACT edits** (§9).
9. **Verify** — six-gate (gofmt / golangci-lint / go vet / build / `go mod tidy -diff` / full package `-race` on `internal/tracing` + `internal/filter/hcm`) + the full 107-dir differential (byte-stable except `0105`).
10. **ADR-0283 body** (§Decision/§Consequences) + **STATE** + **ROADMAP** (row 62 `done` per ADR-0106; the LIVE deferred sentence NARROWS `custom_tags (request_header/environment/metadata)` → `(environment/metadata)` — re-run the check-(2) grep, EXACTLY ONE live "candidates:" match, `reference_sentinel_deferred_sentence_live_vs_historical`) + **router roll**.

(Tasks 1–2 are the TDD core; the PLAN may split/merge. Total ~11–13, single flat row.)

---

## 11. SPEC-time empirical-pin block (D-RH-* live probes — executed IN-SESSION 2026-07-14, `envoyproxy/envoy:contrib-v1.37.2`, FRESH container per arm)

Each arm ran a fresh `envoyproxy/envoy:contrib-v1.37.2` container (`--add-host host.docker.internal:host-gateway`; a host-bound `test/helpers/otlptrace` receiver + a trivial host HTTP backend), drove ONE `GET /probe/path?q=1` with arm-specific request headers, and captured the OTLP span. Decode VERIFIED non-vacuous (span count = 1, built-ins present) on every boot arm. Arms C/D were re-run and reproduced identically.

**Arm A (D-RH-WIRE / D-RH-MISSING / D-RH-MULTIVALUE).** Config: four `request_header` tags — `t_present{name:x-present, default_value:def-present}`, `t_missdef{name:x-missing, default_value:def-missing}`, `t_missnodef{name:x-absent}` (no default), `t_multi{name:x-multi}`. Request headers sent: `x-present: PRESENT-VAL`, `x-multi: MV-A`, `x-multi: MV-B` (x-missing/x-absent NOT sent). Captured OTLP span attributes:
```
t_present   = STRING("PRESENT-VAL")   # header present  → the header value
t_missdef   = STRING("def-missing")   # header absent + default set → the default
t_missnodef = <OMITTED>               # header absent + no default → the tag is OMITTED
t_multi     = STRING("MV-A")          # header sent twice → the FIRST value (not "MV-A,MV-B")
```
⇒ D-RH-WIRE: `{key:<tag>, value:{stringValue:<header value>}}`. D-RH-MISSING: present→value / absent+default→default / absent+no-default→OMIT. D-RH-MULTIVALUE: FIRST value.

**Arm B (D-RH-PRECEDENCE — custom overrides built-in).** Config: `{tag: http.method, request_header:{name:x-ovr}}`; sent `x-ovr: OVERRIDE-METHOD`. Captured: `http.method = STRING("OVERRIDE-METHOD")` — the built-in `"GET"` ABSENT. ⇒ a `request_header` tag OVERRIDES a colliding built-in (upsert, same as `literal` — SPEC-59 §11 precedence-otlp).

**Arm C (D-RH-PRECEDENCE — custom-vs-custom, config order).** Config: `[{tag:dup, literal:{value:LIT-VAL}}, {tag:dup, request_header:{name:x-dup}}]`; sent `x-dup: HDR-VAL`. Captured: `dup = STRING("LIT-VAL")` — the FIRST (literal) tag won; the second (request_header) was DROPPED. (Re-run: identical.)

**Arm D (D-RH-PRECEDENCE — custom-vs-custom, reversed order).** Config: `[{tag:dup, request_header:{name:x-dup}}, {tag:dup, literal:{value:LIT-VAL}}]`; sent `x-dup: HDR-VAL`. Captured: `dup = STRING("HDR-VAL")` — the FIRST (request_header) tag won; the second (literal) was DROPPED. (Re-run: identical.)

⇒ Arms C/D: the reference deduplicates `custom_tags` by tag key keeping the **FIRST occurrence in config order** (Envoy's config-time map-emplace, insert-if-absent), regardless of source type. This CONTRADICTS the landed `upsertAttr` last-wins and drives the parse-time first-wins dedup (§1.1, §3.2).

**Arm E (D-RH-REJECT — empty `request_header.name` boot-rejects).** Config: `{tag: t_bad, request_header:{name:""}}`, run via `envoy --mode validate`. The reference REJECTED at config init:
```
CustomTagValidationError.RequestHeader: ... caused by
HeaderValidationError.Name: value length must be at least 1 characters
```
⇒ the reference PGV boot-rejects an empty `request_header.name`; envoy-go mirrors it as a Tier-A PGV-parity reject (§6). Arms A–D all BOOTED and emitted ⇒ the reference ACCEPTS `request_header` (the departure genuinely narrows).

**PGV structural rules (RE-DERIVED @ `type/tracing/v3/custom_tag.pb.validate.go`, go-control-plane/envoy v1.32.4).** `:583` `utf8.RuneCountInString(m.GetName()) < 1` (`Header.name` `min_len:1`); `:594` `_CustomTag_Header_Name_Pattern.MatchString` (`Header.name` pattern `^[^\x00\n\r]*$`). The phase-59 anchors are unchanged (`:64` tag, `:358` literal.value, `:245` typeless).

*(Probe harness: a throwaway `probe62/` Go program reusing the `otlptrace` receiver + a `docker run` CLI loop; DELETED after — NOT committed, this SPEC is docs-only.)*

---

## 12. Edit-site roster (D-RH-DOCSHAPE — RE-DERIVED against master `664fb2dd`)

**Production — `internal/tracing/config.go`:**
- `config.go:25-37` `TracingConfig` — change `CustomTags []KV` → `CustomTags []CustomTagSpec`; ADD the `CustomTagSpec`/`customTagKind` types (§3.2). [EDIT/ADD]
- `config.go:139-166` `parseCustomTags` — return `([]CustomTagSpec, error)`; the `request_header` accept arm + empty-name reject (`:155-156` lift); first-wins dedup (§3.1). [EDIT]
- `config.go:88`/`:127` — the switch-restructure sets `cfg.CustomTags` (type change only). [EDIT — minor]

**Production — `internal/tracing/resolve.go` (new file):**
- `ResolveCustomTags(specs []CustomTagSpec, headerLookup func(string) ([]string, bool)) []KV` (§3.3). [ADD]

**Production — `internal/tracing/span.go`:**
- `BuildServerSpan` (`:64`) + `upsertAttr` (`:121`) — UNCHANGED (§3.5). [NO CHANGE — confirm]

**Production — `internal/filter/hcm/accesslog_emit.go`:**
- `:55` (H1) + `:116` (H2) + `:177` (H3) — wrap `f.tracingConfig.CustomTags` in `tracing.ResolveCustomTags(..., reqHeaderLookupH1(r)/reqHeaderLookupH2(req))` (§3.4). [EDIT — 3 sites]

**Test:**
- `internal/tracing/config_test.go` — accept a request_header; reject empty-name + unchanged arms; dedup first-wins. [ADD]
- `internal/tracing/resolve_test.go` (new) — the resolver matrix (§10 task 2). [ADD]
- `internal/tracing/span_test.go` / `zipkin_test.go` — a resolved request_header KV upserts over a built-in; Zipkin tags map; update call sites only if the field-type change ripples. [ADD/EDIT]
- `internal/filter/hcm/fuzz_test.go` — a request_header custom_tags seed; no new fuzzer. [ADD]

**Fixture:**
- `test/fixtures/0105-tracing-custom-tags-request-header/` (new) — OTLP provider + a request_header custom tag; the driver sends `x-trace-user`; span assertion both sides (§8). [ADD]

**Docs:**
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` (tracing custom_tags clause, §9). [EDIT — IMPL]
- `docs/envoy-go/ROADMAP.md` row 62 → `done` + family prose + deferred-sentence narrow. [EDIT — IMPL]
- `docs/envoy-go/STATE.md` (active-phase header). [EDIT — each stage]
- `docs/envoy-go/DECISIONS.md` — ADR-0283 §Context here (§13); §Decision/§Consequences at the IMPL. [ADD]

---

## 13. ADR continuity — the ADR-0283 §Context DRAFT (anchored here; full entry at the phase-62 IMPL)

**ADR-0283 §Context (draft).** Phase 59 (ADR-0277) lifted the wholesale `custom_tags` reject for the `literal` type — a static `{tag, value}` span attribute on the shared `Span.Attrs []KV` seam (`span.go:54`), applied via an `upsertAttr` last-write-wins loop in `BuildServerSpan` (`span.go:64`/`:121`) — and PARSE-REJECTED the three request-driven types (`request_header`/`environment`/`metadata`) loudly. The ROADMAP Observability family's LIVE deferred sentence + ADR-0277 §Consequences named those three as the next tracing follow-ons. Phase 62 lifts the reject for `request_header` — the FIRST custom tag whose value is per-request (read from a named DOWNSTREAM REQUEST header). SPEC-62 live probes against `envoyproxy/envoy:contrib-v1.37.2` (§11, fresh container per arm) PINNED: the wire form (`{key:<tag>, value:{stringValue:<header value>}}`); the missing-header semantics (present→value / absent+default→default / absent+empty-default→OMIT — a NEW omit semantic vs `literal`); multi-value → FIRST value; and — REFINING the landed engine — that among custom tags with a colliding key the reference keeps the FIRST in config order (Envoy's config-time map, insert-if-absent), NOT the landed last-wins, while custom tags STILL override built-ins. The design: the phase-59 static `TracingConfig.CustomTags []KV` becomes an ORDERED `[]CustomTagSpec` deduped FIRST-wins at parse (`parseCustomTags`), plus a per-request `ResolveCustomTags(specs, headerLookup) []KV` (literal→static; request_header→first header value / default / omit) threaded at the THREE `accesslog_emit.go` `BuildServerSpan` call sites (H1/H2/H3) via the EXISTING `reqHeaderLookupH1`/`reqHeaderLookupH2` lookups; `BuildServerSpan`/`upsertAttr` UNCHANGED (they now only override built-ins, the resolver having produced unique keys). This CORRECTS a latent phase-59 divergence on duplicate literal keys (byte-stable for the single-key common case). `environment`/`metadata` STAY loud strict-reject departures (the reference-accept confirmed by §11 arm A); a NEW empty-`request_header.name` PGV-parity reject mirrors the reference's boot-reject (§11 arm E). A SINGLE FLAT ROW (ADR-0045 escape-valve unconsumed); the `CustomTagSpec` model + `ResolveCustomTags` seam are FOLDED into ADR-0283 (no separate seam ADR — the phase-59/58 precedent). +0 stats / +1 fixture (`0105`) / +0 fuzzers (a seed) / +0 packages / +0 modules. §Decision/§Consequences land at the phase-62 IMPL per ADR-0044. ANCHORS ADR-0283.

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

**Counts UNCHANGED at this SPEC (docs-only; re-verified against master tip `664fb2dd`):** stat surface **1201** · fixtures **106** · fuzzers **55** · BackendKind **38** · DECISIONS tail **ADR-0282** (next-free **ADR-0283**) · new Go packages **0** · go.mod modules **2**.

**Anticipated at the phase-62 IMPL:** stat surface **1201 (+0)** · fixtures **106 → 107** (`0105-tracing-custom-tags-request-header`) · fuzzers **55 (+0, seed only)** · BackendKind **38 (+0)** · DECISIONS tail **ADR-0283** (next-free **ADR-0284**) · new Go packages **0** · new go.mod modules **0**.

**ROADMAP/STATE at SPEC-DONE:** row 62 STAYS `in-progress` (a row flips `done` only at its IMPL six-gate, ADR-0106). The LIVE deferred sentence is UNCHANGED at this SPEC (`custom_tags` NARROWS only at the IMPL — sentinel check-(2) STILL one live match). STATE active-phase header flips to `phase 62 SPEC done` (NEXT = the phase-62 PLAN).

**Next → the phase-62 PLAN** (the TDD decomposition of §10 over this SPEC; every `file:line` RE-DERIVED against the master tip; ADR-0045 single-flat-row; PROGRESS scaffolded).
