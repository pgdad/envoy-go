# SPEC 59 — tracing `custom_tags` (LITERAL tag type only) (the TWELFTH Observability-family row; the FIRST tracing follow-on since phase 46; lifts the wholesale `custom_tags unsupported` reject at `internal/tracing/config.go:82-83` and emits a `literal` `CustomTag` as a static `{tag, value}` STRING span attribute on BOTH exporters via the shared `Span.Attrs []KV` seam; the other three types (`request_header`/`environment`/`metadata`) PARSE-REJECT loudly — an envoy-go-strict DEPARTURE, ADR-0080 — alongside three PGV-parity structural rejects; anticipated ONE new OTLP fixture / +0 stats / +0 packages / +0 modules — a SINGLE FLAT ROW, ADR-0277)

> **Stage:** SPEC (lifecycle-state 1 → 2). Docs-only; NO production `.go` changes at this stage. Fresh worktree `.worktrees/phase-59-spec`, branch `phase-59-tracing-custom-tags-literal-spec`, per `feedback_git_worktrees`.
>
> **ANCHORS ADR-0277 §Context DRAFT** (§Decision/§Consequences land at the phase-59 IMPL per ADR-0044; DECISIONS tail STAYS **ADR-0276** at this SPEC).
>
> **Baselines re-verified against master tip `9433d69a` (the phase-59 BRAINSTORM squash):** stat surface **1201** · fixtures **103** (tail `0101-stats-sink-graphite`) · fuzzers **54** (verified `54` actual `^func Fuzz`) · BackendKind tail **38** (`H2GoawayResponder`) · DECISIONS tail **ADR-0276** (next-free **ADR-0277**) · new Go packages **0** · new go.mod modules **0**. Counts UNCHANGED at this SPEC (docs-only). Every `file:line` below was RE-DERIVED from source this session (`feedback_brief_citations_not_evidence`) — the roster is §12.

---

## 1. Purpose / Mission

Phase 59 lifts the wholesale `custom_tags unsupported` reject (`internal/tracing/config.go:82-83`) and supports the **`literal`** `CustomTag` type: an operator configuring `custom_tags: [{tag: "env", literal: {value: "prod"}}]` on the HCM `tracing` block gets a `{key: "env", value: "prod"}` STRING attribute on every sampled ingress SERVER span — on **both** the OTLP and the Zipkin exporter (one append to the shared `Span.Attrs []KV` seam covers both). The three request-driven `CustomTag` types (`request_header`/`environment`/`metadata`) stay **PARSE-REJECTED loudly** with distinct substrings (envoy-go-strict DEPARTURE — the reference SUPPORTS them, §11 arm reject-accept). Three PGV-parity structural rejects (empty `tag`, empty `literal.value`, typeless tag) mirror the reference's boot-reject (both reject — NOT a departure).

ADR-0277 §Context is DRAFTED here (§13); §Decision/§Consequences at the IMPL (ADR-0044). **All ten BRAINSTORM D-CT-* questions are DISPOSED at this SPEC** — the four empirical arms via LIVE probes against `envoyproxy/envoy:contrib-v1.37.2` (§11, fresh container per arm, `reference_probe_fresh_container_per_arm`):

- **D-CT-SCOPE** — DISPOSED: literal-only; the other three types reject loudly (§2.2, §6).
- **D-CT-LITERAL-WIRE** — PINNED (§11 arm literal-otlp): `{key: <tag>, value: {stringValue: <literal.value>}}`, appended after the built-ins.
- **D-CT-ZIPKIN-WIRE** — PINNED (§11 arm zipkin): the tag appears in the Zipkin `tags` map as `"<tag>": "<value>"`.
- **D-CT-PRECEDENCE** — PINNED (§11 arm precedence-otlp) — **CONTRADICTS the BRAINSTORM anticipation**: the reference **OVERRIDES** a colliding built-in (last-write-wins by key), it does NOT emit a duplicate. Drives the `BuildServerSpan` **upsert-by-key** design (§3.3).
- **D-CT-CONFIG-SEAM** — DECIDED (§3): a new `CustomTags []KV` field on `TracingConfig`; parsed provider-neutrally in `NewConfig` and set on the returned config after the provider switch; threaded as a NEW `customTags []KV` parameter to `BuildServerSpan`.
- **D-CT-REJECT** — PINNED (§6, §11 arm reject-accept + PGV re-derivation): three type-unsupported departures + three PGV-parity structural rejects, each with a distinct substring.
- **D-CT-FIXTURE** — DECIDED (§8): ONE new OTLP fixture `0102-tracing-custom-tags-literal` (fixtures **103 → 104**); the Zipkin encoder path is covered by a unit test, NOT a second fixture (escape-valve unconsumed).
- **D-CT-FUZZSEED** — DECIDED (§6): a seed to the existing `FuzzHCMConfigParse`; fuzzer count stays **54**.
- **D-CT-SPLIT** — DECIDED (§3.0): a SINGLE FLAT ROW (~11 tasks), ADR-0045 escape-valve UNCONSUMED.
- **D-CT-DOCSHAPE** — the full RE-DERIVED edit-site roster (§12).

No PLAN-time empirical question remains; the PLAN is a mechanical TDD decomposition.

### 1.1 Empirical-finding-driven scope amendment (per ADR-0044)

The BRAINSTORM (§2.3, §10 D-CT-PRECEDENCE) anticipated **APPEND** on a built-in-key collision. The SPEC-59 live probe (§11 arm precedence-otlp) PINNED **OVERRIDE** instead: a `literal` tag whose `tag` equals a built-in attribute key REPLACES that built-in's value on the reference OTLP span (last-write-wins). This is a material amendment: envoy-go's OTLP `toProto` (`span.go:125-134`) emits every `KV` as a separate attribute, so a naive append would emit TWO attributes with the colliding key — a divergence. The SPEC therefore pins **upsert-by-key** append semantics in `BuildServerSpan` (§3.3), which also makes envoy-go's two exporters mutually consistent (the Zipkin encoder's `tags` map at `zipkin.go:87-94` already overrides last-write-wins). This is the only anticipation the probes overturned; every other BRAINSTORM decision held.

---

## 2. Non-purposes (deferred; per BRAINSTORM §1.2 + §8)

NO `request_header`/`environment`/`metadata` custom-tag types (rejected loudly, §6). NO `spawn_upstream_span`/`http_service`/force-trace. NO `max_path_tag_length` (a distinct, still-rejected knob, `config.go:79-80`). NO OTLP-metrics stats sink. NO new provider, transport, or stat. The four built-in span attributes emitted EMPTY by envoy-go (`upstream_cluster`/`node_id`/`zone`/`peer.address`, the `reference_tracing_upstream_cluster_framework_gap` framework gap) are UNTOUCHED — literal custom tags are static config values, INDEPENDENT of that per-request plumbing gap. (Note §11 arm literal-otlp shows the REFERENCE populates `upstream_cluster`/`peer.address` at emit time; envoy-go's differential remains UNasserted on those built-ins per the recorded gap — the custom-tag fixture asserts only the NEW literal attribute.)

### 2.1 The phase-57 deferred fold-ins F-1/F-2/F-3 remain OUT of scope

This row touches NEITHER `udp.go` NOR the `0101` stats-sink driver, so F-1/F-2/F-3 (BRAINSTORM §11) stay OPEN for the next `udp.go`/statssink-driver-touching row (OTLP-metrics). Recorded so they are not lost.

---

## 3. The change — a literal-parse + reject arms in `NewConfig`, a `CustomTags` field, an upsert append in `BuildServerSpan`, two call-site threadings (ADR-0277)

### 3.0 Split disposition — a SINGLE FLAT ROW; the ADR-0045 escape-valve UNCONSUMED

Anticipated **~11 tasks** (§10), comfortably under the ADR-0045 `~15` ceiling. There is no second subsystem to strand: the literal parse + the six reject arms live in the same `NewConfig`; the span-emit append is one function; both exporters share the `Attrs` seam. The escape-valve is documented ARMABLE but UNCONSUMED — the single new OTLP fixture (§8) does not push the fixture leg into its own row.

### 3.1 The parse arms — replace the wholesale reject in `NewConfig` (`internal/tracing/config.go`)

Replace `config.go:82-83` (the `if len(t.GetCustomTags()) > 0 { return … "custom_tags unsupported" }` wholesale reject) with a loop over `t.GetCustomTags()` that builds a provider-neutral `[]KV`. Per tag (a `*tracingv3.CustomTag` from `github.com/envoyproxy/go-control-plane/envoy/type/tracing/v3` — an EXISTING-module import, §4):

1. **Empty `tag`** (PGV-parity — `custom_tag.pb.validate.go:61`, `utf8.RuneCountInString(m.GetTag()) < 1`): reject `tracing: custom_tags empty tag`.
2. **Type dispatch** on the oneof (getters re-derived from `custom_tag.pb.go`, §5):
   - `ct.GetLiteral() != nil` (`:92`) → ACCEPT: reject an empty `literal.value` (PGV-parity — `custom_tag.pb.validate.go:355`, `min_len:1` on `CustomTag_Literal.value`) as `tracing: custom_tags literal tag %q empty value`, else append `KV{Key: ct.GetTag(), Str: ct.GetLiteral().GetValue()}`.
   - `ct.GetRequestHeader() != nil` (`:106`) → reject `tracing: custom_tags request_header type unsupported` (DEPARTURE).
   - `ct.GetEnvironment() != nil` (`:99`) → reject `tracing: custom_tags environment type unsupported` (DEPARTURE).
   - `ct.GetMetadata() != nil` (`:113`) → reject `tracing: custom_tags metadata type unsupported` (DEPARTURE).
   - else (typeless — `ct.GetType() == nil`, `:85`; PGV-parity — the `type` oneof is REQUIRED, `custom_tag.pb.validate.go:245`) → reject `tracing: custom_tags tag %q missing type`.

The six substrings are ALL ADR-0080-distinct. Ordering note: the empty-`tag` check runs BEFORE the type dispatch so a `{tag:"", literal:{...}}` rejects as empty-tag (the reference's PGV evaluates `tag` before the oneof, `custom_tag.pb.validate.go:61` precedes `:245`). Dispatch by the concrete getter (`GetLiteral()/GetRequestHeader()/...`), NOT by a `GetType()` type-switch, keeps the arm readable and mirrors the existing OTel/Zipkin provider dispatch style.

### 3.2 The config-home + threading (`config.go`)

- **Field** — add `CustomTags []KV` to `TracingConfig` (`config.go:24-32`). `KV` is defined in the SAME package (`span.go:12`, `package tracing`), so `[]KV` is a direct in-package reference — NO new type. Doc-comment the field (parsed literal custom tags, provider-neutral, appended by `BuildServerSpan`).
- **Provider-neutral threading** — the tags are parsed in `NewConfig` BEFORE the provider switch (`config.go:108-115`), so restructure the switch to capture the parsed config and set `CustomTags` before returning, WITHOUT touching the `parseOTel`/`parseZipkin` signatures:
  ```go
  var cfg *TracingConfig
  var perr error
  switch tc.MessageName() {
  case otelTypeName:
      cfg, perr = parseOTel(tc, clientSampling, randomSampling, overallSampling)
  case zipkinTypeName:
      cfg, perr = parseZipkin(tc, clientSampling, randomSampling, overallSampling)
  default:
      return nil, fmt.Errorf("tracing: provider %s unsupported (only OpenTelemetry or Zipkin)", tc.GetTypeUrl())
  }
  if perr != nil {
      return nil, perr
  }
  cfg.CustomTags = customTags
  return cfg, nil
  ```
  This leaves `parseOTel` (`config.go:120-153`) and `parseZipkin` (`config.go:159-195`) UNCHANGED — the tags land on both returns uniformly. (Alternative rejected: adding a `[]KV` param to each `parseX` — more churn, no benefit.)

### 3.3 The span-emit append — an UPSERT loop in `BuildServerSpan` (`internal/tracing/span.go`)

`BuildServerSpan` (`span.go:64`) gains a `customTags []KV` parameter; after the 16 built-ins (`span.go:68-88`) and the optional `guid:x-client-trace-id` (`span.go:91-93`), it applies each custom tag by **upsert** (last-write-wins by key — the reference OVERRIDE semantics, §11 arm precedence-otlp), NOT append:

```go
func BuildServerSpan(d Decision, in SpanInputs, customTags []KV, start, end time.Time) *Span {
    attrs := make([]KV, 0, 17+len(customTags))
    // ... 16 built-ins + optional guid:x-client-trace-id unchanged ...
    for _, ct := range customTags {
        upsertAttr(&attrs, ct)
    }
    // ... return &Span{... Attrs: attrs ...}
}

// upsertAttr sets ct by key: replaces an existing attribute with the same key
// (last-write-wins, matching the reference OTel tracer's OVERRIDE semantics on a
// custom_tag / built-in collision — SPEC-59 §11 arm precedence-otlp), else appends.
func upsertAttr(attrs *[]KV, ct KV) {
    for i := range *attrs {
        if (*attrs)[i].Key == ct.Key {
            (*attrs)[i] = ct
            return
        }
    }
    *attrs = append(*attrs, ct)
}
```

Rationale: the OTLP `toProto` (`span.go:125-134`) emits every `KV` as a separate wire attribute, so a plain append of a colliding literal tag would emit TWO attributes with that key — diverging from the reference's single overridden attribute. Upsert matches the reference on OTLP AND is consistent with the Zipkin encoder's `tags` map (`zipkin.go:87-94`, which already overrides last-write-wins). The common case (a NON-colliding key) is a pure append. The upsert cost is O(builtins) per custom tag — negligible (≤17 built-ins, typically 1–2 custom tags).

### 3.4 The call-site threading (`internal/filter/hcm/accesslog_emit.go`)

The two `BuildServerSpan` call sites — `accesslog_emit.go:55` (H1, `emitAccessLog`) and `:116` (H2, `emitAccessLogH2`) — pass `f.tracingConfig.CustomTags`:

```go
f.exporter.Export(tracing.BuildServerSpan(*traceDecision, in, f.tracingConfig.CustomTags, start, time.Now()))
```

**Nil-safety invariant (RE-DERIVED):** both call sites are inside `if statusCode != 0 && f.exporter != nil && … ` (`accesslog_emit.go:28`, `:87`), and `f.exporter` is set ONLY inside `if tcfg != nil { … exporter, err = provider.ExporterFor(tcfg) }` (`config.go:329-337`) with `f.tracingConfig = tcfg` (`config.go:356`). So `f.exporter != nil ⟹ f.tracingConfig != nil` — `f.tracingConfig.CustomTags` is safe with NO extra guard. (When `CustomTags` is empty/nil the upsert loop is a no-op — byte-stable with today's spans.)

### 3.5 Byte-stability — no behavior change on the no-custom-tags path

A tracing HCM with no `custom_tags` parses to `CustomTags == nil`; `BuildServerSpan`'s upsert loop iterates zero times; every existing span is byte-identical. The 103-dir differential (incl. `0087`/`0088`) stays byte-stable — the new fixture `0102` is the only dir exercising the arm.

---

## 4. Framework primitives — 0 new packages, 0 new go.mod modules

All edits land in `internal/tracing` + `internal/filter/hcm` (both existing) + `test/fixtures` + `docs/`. The `CustomTag` proto is reachable via the ALREADY-resolved `github.com/envoyproxy/go-control-plane/envoy v1.32.4` module at import path `github.com/envoyproxy/go-control-plane/envoy/type/tracing/v3` (verified: `go list -deps` resolves `.../envoy/type/tracing/v3` to `go-control-plane/envoy v1.32.4`). `config.go` gains ONE named import (`tracingv3 "…/envoy/type/tracing/v3"`) of that EXISTING module — `go mod tidy -diff` anticipated EMPTY. NO new package, NO new module, NO new interface.

---

## 5. Proto-field roster — `type.tracing.v3.CustomTag` (RE-DERIVED @ go-control-plane/envoy v1.32.4, `type/tracing/v3/custom_tag.pb.go`)

| Field | Getter (`custom_tag.pb.go`) | Phase-59 disposition |
|---|---|---|
| `tag` (1, string) | `GetTag()` `:78` | the attribute KEY; empty ⇒ reject (PGV-parity) |
| `type` oneof | `GetType() isCustomTag_Type` `:85` | REQUIRED (PGV `:245`); typeless ⇒ reject |
| `literal` (2, `CustomTag_Literal`) | `GetLiteral()` `:92`; `CustomTag_Literal.GetValue()` `:194` | ACCEPT ⇒ `KV{Key:tag, Str:value}`; empty value ⇒ reject (PGV-parity) |
| `environment` (3) | `GetEnvironment()` `:99` | reject (DEPARTURE) |
| `request_header` (4, `CustomTag_Header`) | `GetRequestHeader()` `:106` | reject (DEPARTURE) |
| `metadata` (5) | `GetMetadata()` `:113` | reject (DEPARTURE) |

---

## 6. PARSE-REJECT roster + fuzzer

**Two-tier reject taxonomy** (both tiers ADR-0080-distinct substrings):

**Tier A — PGV-parity structural rejects (BOTH the reference and envoy-go reject; NOT a departure).** envoy-go does NOT run PGV, so it mirrors these explicitly:
- empty `tag` → `tracing: custom_tags empty tag` (ref PGV `custom_tag.pb.validate.go:61`).
- empty `literal.value` → `tracing: custom_tags literal tag %q empty value` (ref PGV `:355`).
- typeless tag → `tracing: custom_tags tag %q missing type` (ref PGV `:245`, oneof required).

**Tier B — envoy-go-strict DEPARTURES (the reference ACCEPTS — §11 arm reject-accept; envoy-go rejects):**
- `request_header` → `tracing: custom_tags request_header type unsupported`.
- `environment` → `tracing: custom_tags environment type unsupported`.
- `metadata` → `tracing: custom_tags metadata type unsupported`.

**Fuzzer (D-CT-FUZZSEED).** The tracing config is parsed via `NewConfig` off the HCM proto; the HCM config parse path is fuzzed by `FuzzHCMConfigParse` (`internal/filter/hcm/fuzz_test.go:25`) — the tracing package has NO config fuzzer (its fuzzers are propagation/request-id only). The custom_tags parse is reached: `FuzzHCMConfigParse` → `NewFilterWithCtxAndSinksAndRegistry` → `BuildConfig` → `tracing.NewConfig(msg.GetTracing())` (`config.go:312`), and the custom_tags loop (`config.go:82`) runs BEFORE the provider check (`config.go:89`), so a custom_tags seed exercises the arms regardless of provider. **Add ONE seed** via the existing `mkHCM(modify func(*hcmv3.HttpConnectionManager))` helper (`config_test.go:44`): a `Tracing` block with `CustomTags` = one accepted `literal` + one rejected type (e.g. `request_header`), added with `f.Add(seed.GetTypeUrl(), seed.GetValue())` alongside the three existing seeds (`fuzz_test.go:27-29`). **This is a SEED, not a new `func Fuzz` — fuzzer count STAYS 54** (`reference_fuzzer_count_docs_drift`: reconcile actual `^func Fuzz` = 54 before AND after).

---

## 7. Stat surface — +0 (1201 → 1201)

A span attribute is emitted on the wire, not registered as a stat. The 5 HCM tracing decision counters + the 2 tracer counters (`spans_sent`/`spans_dropped`) are UNCHANGED. No new registration path. Stat surface **1201 (+0)**.

---

## 8. Differential fixture taxonomy — +1 (D-CT-FIXTURE: ONE new OTLP fixture)

A literal custom tag is an OBSERVABLE span attribute, so it IS differential-provable. Per the dispatch constraint (`reference_differential_fixture_dispatch_constraint` — one fixture dir = ONE runner branch; adding an assertion changes what a dir proves) the `0087`/`0088` pure baselines are NOT mutated. **ONE new dir `test/fixtures/0102-tracing-custom-tags-literal`** (fixtures **103 → 104**):

- Cloned from `0087-tracing-otlp` (OTLP provider, `test/helpers/otlptrace` receiver, `host.docker.internal` STRICT_DNS per ADR-0010).
- The HCM `tracing` block adds `custom_tags: [{tag: "custom_env", literal: {value: "prod-literal"}}]` — a **NON-colliding** key (clean parity on the new attribute; the OVERRIDE/collision case is a UNIT test, §10, not the differential).
- The driver asserts (via a `StatsAsserter`/span accessor, per the 0087 pattern) that the captured span carries `{key:"custom_env", value:"prod-literal"}` on BOTH the reference AND subject side. Assert by KEY (attribute order is non-deterministic — §11). Assert each independent property with `Errorf`, NOT `Fatalf` (`reference_fatalf_makes_assertions_unreachable`). `BackendCount` ≥ 1 (`reference_differential_backendcount_min_one`).
- Prove the new assertion LIVE with a deliberate `-count=1` break (`reference_differential_break_protocol_count1`), confirming WHICH assertion fires (`reference_deliberate_break_wrong_assertion`).

**The Zipkin encoder path** (a literal tag surfacing in the `tags` map, `zipkin.go:87-94`) is covered by a UNIT test (§10, `span_test.go`/`zipkin_test.go`), NOT a second fixture — the encoder is a pure function fully covered by unit test, and the cross-side Zipkin built-in parity is already proven by `0088`. This keeps the row a SINGLE FLAT ROW (escape-valve unconsumed). Fixtures **103 → 104** (NOT 105).

---

## 9. Behavior-contract delta (`docs/envoy-go/BEHAVIOR_CONTRACT.md`; ADR-0277 atomic landing at the IMPL)

- **Line 686** (the tracing STRICT-REJECT roster) — REMOVE `custom_tags` from the wholesale strict-reject list; ADD a clause: the `literal` `CustomTag` type is CONSUMED (emitted as a `{tag, value}` STRING span attribute on both OTLP and Zipkin, upsert/last-write-wins on a built-in-key collision matching the reference); `request_header`/`environment`/`metadata` STRICT-REJECT (envoy-go-strict DEPARTURE — the reference accepts); empty-`tag`/empty-`literal.value`/typeless STRICT-REJECT (PGV-parity — the reference boot-rejects too).
- **Line 739** (the Zipkin sub-section's deferred-list bullet) — NARROW the `custom_tags` mention: `literal` done; `request_header`/`environment`/`metadata` remain deferred.

(Exact final wording RE-DERIVED and written at the IMPL; both sites are in the tracing block ~672–742.)

---

## 10. Test plan + per-task structure (~11 tasks; PLAN decomposes)

TDD (`superpowers:test-driven-development`); each task a red→green with a `-count=1` liveness break where an assertion is load-bearing. Anticipated tasks:

1. **`config.go` parse arms + `CustomTags` field + provider-neutral threading** — the literal-parse + six reject arms replacing `config.go:82-83`; the `TracingConfig.CustomTags` field; the switch restructure (§3.1/§3.2). `config_test.go`: accept a literal (assert on the parsed `CustomTags`); reject each of the 6 arms with its distinct substring. `-count=1` break per new reject row (or an isolating break confirming WHICH fires).
2. **`span.go` upsert append + param** — the `customTags` parameter + `upsertAttr` (§3.3); `span_test.go`: (a) a literal tag appears in `Attrs` after the built-ins (append case); (b) a colliding tag OVERRIDES the built-in (upsert case — one KV, the override value). Update the 6 `span_test.go` call sites (`:60,:151,:178,:193,:207,:283`) + the `zipkin_test.go` call site (`:88`) to the new signature (pass `nil` where custom tags are irrelevant).
3. **Zipkin encoder unit test** — `zipkin_test.go` (or `span_test.go`): a span with a literal custom tag encodes into the Zipkin `tags` map (`"custom_env":"prod-literal"`), and node_id/zone stay dropped.
4. **Call-site threading** — `accesslog_emit.go:55`/`:116` pass `f.tracingConfig.CustomTags` (§3.4). Covered by the existing HCM span tests (no new assertion needed beyond signature).
5. **New OTLP fixture `0102-tracing-custom-tags-literal`** — envoy.yaml, envoy-go.yaml, expectations.yaml, driver, README (§8); the custom-tag span assertion proven live.
6. **`FuzzHCMConfigParse` seed** — one custom_tags seed (§6); reconcile fuzzer count = 54.
7. **BEHAVIOR_CONTRACT edits** — lines 686 + 739 (§9).
8. **Verify** — six-gate (gofmt / golangci-lint / go vet / build / `go mod tidy -diff` / full package `-race` on `internal/tracing` + `internal/filter/hcm`) + the full 104-dir differential (byte-stable except `0102`).
9. **ADR-0277 body** (§Decision/§Consequences) + **STATE** + **ROADMAP** (row 59 `done` per ADR-0106; the LIVE deferred sentence NARROWS `custom_tags` to the non-literal types — re-run the check-(2) grep, EXACTLY ONE live "candidates:" match, `reference_sentinel_deferred_sentence_live_vs_historical`) + **router roll**.

(Tasks 1–3 are the TDD core; the PLAN may split/merge — e.g. fold task 4 into task 2, or split task 1's accept vs reject. Total ~9–11, single flat row.)

---

## 11. SPEC-time empirical-pin block (D-CT-* live probes — executed IN-SESSION 2026-07-12, `envoyproxy/envoy:contrib-v1.37.2`, FRESH container per arm)

Each arm ran a fresh `envoyproxy/envoy:contrib-v1.37.2` container (`--add-host host.docker.internal:host-gateway`; host-bound `test/helpers/otlptrace` + `test/helpers/zipkincollector` receivers + an HTTP backend), drove ONE `GET /probe/path?q=1` request, and captured the span. Decode VERIFIED non-vacuous (span count = 1, built-ins present) on every arm.

**Arm literal-otlp (D-CT-LITERAL-WIRE).** Config: OTLP provider + `custom_tags: [{tag:"probe_literal", literal:{value:"probe-literal-value"}}]`. Captured OTLP span attribute (LAST in the list, after the 16 built-ins):
```json
{ "key": "probe_literal", "value": { "stringValue": "probe-literal-value" } }
```
⇒ key = the `tag` name; value = `literal.value` as a STRING `AnyValue`; APPENDED after the built-ins. Matches envoy-go's `KV{Key, Str}` → `toProto` `StringValue`.

**Arm precedence-otlp (D-CT-PRECEDENCE) — OVERRIDE, not append.** Config: `custom_tags: [{tag:"http.method", literal:{value:"COLLIDE-VALUE"}}]`. The captured span carried EXACTLY ONE `http.method` attribute = `"COLLIDE-VALUE"` — the built-in `"GET"` was ABSENT. ⇒ the reference OVERRIDES a built-in on a key collision (last-write-wins), NOT append-duplicate. Drives the `BuildServerSpan` upsert design (§1.1, §3.3).

**Arm zipkin (D-CT-ZIPKIN-WIRE).** Config: Zipkin provider + `custom_tags: [{tag:"probe_literal", literal:{value:"probe-zip-value"}}]`. The captured Zipkin v2 span `tags` map contained `"probe_literal": "probe-zip-value"` (string). `node_id`/`zone` absent from `tags` (the reference Zipkin drops them — matches envoy-go `zipkin.go:89`). ⇒ ONE append to `Attrs` surfaces on both exporters.

**Arm reject-accept (D-CT-REJECT — the departure is REAL).** Config: OTLP provider + `custom_tags:` `request_header`(`x-probe-hdr`) + `environment`(`PROBE_ENV_VAR`) + `metadata`(`envoy.probe`/`foo`). The reference BOOTED (`admin /ready ⇒ 200 LIVE`) and emitted all three on the span: `hdr_tag:"HDR-SENT-VALUE"` (the sent header), `env_tag:"env-default"` (env default — var unset), `meta_tag:"meta-default"` (metadata default — no dynamic metadata). ⇒ the reference ACCEPTS all three types; envoy-go's loud reject is a REAL strict-departure. (Emit order was meta/hdr/env vs config order hdr/env/meta ⇒ OTLP attribute order is non-deterministic ⇒ the fixture asserts by KEY, §8.)

**PGV structural rules (RE-DERIVED @ `type/tracing/v3/custom_tag.pb.validate.go`, go-control-plane/envoy v1.32.4).** `:61` `utf8.RuneCountInString(m.GetTag()) < 1` (tag `min_len:1`); `:245` `if !oneofTypePresent { "value is required" }` (the `type` oneof is REQUIRED); `:355` `utf8.RuneCountInString(m.GetValue()) < 1` (`CustomTag_Literal.value` `min_len:1`). ⇒ the reference boot-rejects empty-tag / typeless / empty-literal-value; envoy-go mirrors them as the Tier-A PGV-parity rejects (§6).

*(Probe harness: a throwaway `probe59/` Go program reusing the two receivers; deleted after — NOT committed, this SPEC is docs-only.)*

---

## 12. Edit-site roster (D-CT-DOCSHAPE — RE-DERIVED against master `9433d69a`)

**Production — `internal/tracing/config.go`:**
- `config.go:24-32` `TracingConfig` — ADD `CustomTags []KV` field. [EDIT]
- `config.go:82-83` — REPLACE the wholesale `custom_tags unsupported` reject with the parse loop + six reject arms (§3.1). [EDIT]
- `config.go:108-115` — restructure the provider switch to set `cfg.CustomTags = customTags` before returning (§3.2). [EDIT]
- import block (`config.go:7-15`) — ADD `tracingv3 "github.com/envoyproxy/go-control-plane/envoy/type/tracing/v3"` (existing module). [EDIT]

**Production — `internal/tracing/span.go`:**
- `span.go:64` `BuildServerSpan` — ADD `customTags []KV` param; ADD the upsert loop after the built-ins/guid; ADD `upsertAttr` helper (§3.3). [EDIT/ADD]

**Production — `internal/filter/hcm/accesslog_emit.go`:**
- `:55` (H1) + `:116` (H2) — pass `f.tracingConfig.CustomTags` (§3.4). [EDIT — 2 sites]

**Test:**
- `internal/tracing/config_test.go` — accept a literal; reject each of the 6 arms (distinct substrings). [ADD]
- `internal/tracing/span_test.go` — append + upsert-override cases; update call sites `:60,:151,:178,:193,:207,:283`. [ADD/EDIT]
- `internal/tracing/zipkin_test.go` — literal tag in the `tags` map; update call site `:88`. [ADD/EDIT]
- `internal/filter/hcm/fuzz_test.go` — a `custom_tags` seed (`:27`-adjacent); no new fuzzer. [ADD]

**Fixture:**
- `test/fixtures/0102-tracing-custom-tags-literal/` (new) — OTLP provider + a literal custom tag; span assertion both sides (§8). [ADD]

**Docs:**
- `docs/envoy-go/BEHAVIOR_CONTRACT.md:686` + `:739` (§9). [EDIT]
- `docs/envoy-go/ROADMAP.md:121` (row 59 → `done` at IMPL) + `:181` (family prose + deferred-sentence narrow at IMPL). [EDIT — IMPL]
- `docs/envoy-go/STATE.md:7` (active-phase header). [EDIT — each stage]
- `docs/envoy-go/DECISIONS.md` — ADR-0277 §Context here (§13); §Decision/§Consequences at the IMPL. [ADD]

---

## 13. ADR continuity — the ADR-0277 §Context DRAFT (anchored here; full entry at the phase-59 IMPL)

**ADR-0277 §Context (draft).** The phase-46 tracing engine (ADR-0260) parses the HCM `tracing` message via `NewConfig` and rejected `custom_tags` WHOLESALE (`internal/tracing/config.go:82-83`, `tracing: custom_tags unsupported`) — the reference SUPPORTS all four `CustomTag` types (`literal`/`request_header`/`environment`/`metadata`). The ROADMAP Observability family's LIVE deferred sentence enumerated `custom_tags` among the tracing follow-ons. Phase 59 lifts the reject for the FOUNDATIONAL `literal` type — a static, per-config `{tag, value}` needing NO per-request data, hence the smallest defensible slice — and emits it as a STRING span attribute on the shared `Span.Attrs []KV` seam (`span.go:54`), which BOTH the OTLP exporter (`span.go:112-134`) and the Zipkin encoder (`zipkin.go:87-94`) consume — so ONE append covers both providers. SPEC-59 live probes against `envoyproxy/envoy:contrib-v1.37.2` (§11, fresh container per arm) PINNED: the literal wire form (`{key:<tag>, value:{stringValue:<literal.value>}}`, appended after the built-ins); the Zipkin `tags`-map form; and — CONTRADICTING the BRAINSTORM's append anticipation — that a `literal` tag whose `tag` collides with a built-in key OVERRIDES it (last-write-wins), driving an UPSERT-by-key append in `BuildServerSpan` (matching the reference on OTLP and consistent with the Zipkin map). The three request-driven types (`request_header`/`environment`/`metadata`) STRICT-REJECT loudly with distinct substrings — a documented envoy-go-strict DEPARTURE (the reference-accept confirmed by the §11 reject-accept probe), the SAME posture as the landed OTel-transport / Zipkin-version rejects (ADR-0080). Three PGV-parity structural rejects (empty `tag` / empty `literal.value` / typeless — the reference PGV boot-rejects each, `custom_tag.pb.validate.go:61/:355/:245`) mirror the reference (both reject; NOT a departure). A SINGLE FLAT ROW (ADR-0045 escape-valve unconsumed); the `TracingConfig.CustomTags` field + the `BuildServerSpan customTags` parameter are a minor seam FOLDED into ADR-0277 (no separate seam ADR — the phase-58 precedent). +0 stats / +1 fixture (`0102`) / +0 fuzzers (a seed) / +0 packages / +0 modules. §Decision/§Consequences land at the phase-59 IMPL per ADR-0044. ANCHORS ADR-0277.

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

**Counts UNCHANGED at this SPEC (docs-only; re-verified against master tip `9433d69a`):** stat surface **1201** · fixtures **103** · fuzzers **54** · BackendKind **38** · DECISIONS tail **ADR-0276** (next-free **ADR-0277**) · new Go packages **0** · new go.mod modules **0**.

**Anticipated at the phase-59 IMPL:** stat surface **1201 (+0)** · fixtures **103 → 104** (`0102-tracing-custom-tags-literal`) · fuzzers **54 (+0, seed only)** · BackendKind **38 (+0)** · DECISIONS tail **ADR-0277** (next-free **ADR-0278**) · new Go packages **0** · new go.mod modules **0**.

**ROADMAP/STATE at SPEC-DONE:** row 59 STAYS `in-progress` (a row flips `done` only at its IMPL six-gate, ADR-0106). The LIVE deferred sentence is UNCHANGED at this SPEC (`custom_tags` NARROWS only at the IMPL — sentinel check-(2) STILL one live match). STATE active-phase header flips to `phase 59 SPEC done` (NEXT = the phase-59 PLAN).

**Next → the phase-59 PLAN** (the TDD decomposition of §10 over this SPEC; every `file:line` RE-DERIVED against the master tip; ADR-0045 single-flat-row; PROGRESS scaffolded).
