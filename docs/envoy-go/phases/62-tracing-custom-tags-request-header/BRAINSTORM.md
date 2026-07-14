# Phase 62 Brainstorm — tracing `custom_tags` `request_header` SOURCE arm (the THIRTEENTH Observability-family row; the SECOND `custom_tag` source type after the phase-59 `literal`; the FIRST PER-REQUEST-resolved custom tag — lifts the `internal/tracing/config.go:155-156` `request_header type unsupported` reject and reads a named request header into a span attribute [with `default_value` / omit-on-missing]; `environment`/`metadata` STAY parse-rejected loudly (envoy-go-strict, ADR-0080); +0 stats / +0 packages / +0 modules; anticipated ONE new fixture)

> **Stage:** BRAINSTORM (lifecycle-state 0 → 1). Docs-only; no `.go` changes at this stage. Fresh worktree `.worktrees/phase-62-brainstorm`, branch `phase-62-tracing-request-header-custom-tag-brainstorm`, off master, per `feedback_git_worktrees`.
>
> **Loop re-open (AUTONOMOUS — no human pick):** phase 61 (`http3-downstream-listener`, the 61.1/61.2/61.3 split) landed COMPLETE (row 61 `done`, ADR-0282; the HTTP/3 family STAYS OPEN). Per the **STANDING DIRECTIVE (human, 2026-07-12)** the loop runs AUTONOMOUSLY until the termination sentinel fires; the sentinel was re-checked MECHANICALLY at the phase-61.3 IMPL and does NOT fire (check (1) silent — every row `done` — but check (2) prints THREE live "candidates:" sentences [HTTP/3, xDS, Observability] and check (3) prints THREE never-opened families [gRPC, Runtime, WASM], each independently blocking `stop`). No banked mid-lifecycle split legs remain. So the roller SELF-PICKED the next subject (§2.1): the **smallest cleanly-differential-provable candidate** from the live Observability deferred sentence — the tracing `custom_tags` **`request_header`** source arm — over the declined larger/harder-to-prove alternatives (recorded §2.1). No human pause; no `stop` file.
>
> **Baselines re-verified against master tip `9f2494ed` (the phase-61.3 IMPL squash):** stat surface **1201** · fixtures **106** (`ls -d test/fixtures/[0-9]*/ | wc -l`; tail `0104-http3-downstream-get`; the count includes the lettered `0007a`/`0007b` sub-fixtures — a `^[0-9]{4}-` grep under-counts to 104) · fuzzers **55** (`grep -rh '^func Fuzz' --include='*.go' . | wc -l`) · BackendKind tail **38** (`H2GoawayResponder`) · DECISIONS tail **ADR-0282** (next-free **ADR-0283**) · new Go packages **0** · go.mod modules **2** (`quic-go v0.54.1` direct + `qpack v0.5.1` indirect, both confined to `internal/listener` prod deps + `test/helpers` test deps). Counts are UNCHANGED at a BRAINSTORM (docs-only). All `file:line` citations below were RE-DERIVED from source this session (`feedback_brief_citations_not_evidence`) — see §11.

---

## 1. Mission and scope confirmation (62 — the SECOND `custom_tag` source; the FIRST per-request one)

### 1.1 What phase 62 delivers as a self-contained whole (a request-header span attribute on the ingress span)

The HCM `tracing.custom_tags` parse today ACCEPTS the `literal` type (phase 59, ADR-0277) and STRICT-REJECTS the other three, `request_header` among them:

```go
// internal/tracing/config.go:155-156 (re-derived against master 9f2494ed)
case ct.GetRequestHeader() != nil:
    return nil, fmt.Errorf("tracing: custom_tags request_header type unsupported")
```

Phase 62 **lifts that one reject** and supports the **`request_header`** `CustomTag` type — a `{tag, header-name, default_value}` spec whose span-attribute VALUE is read from a named DOWNSTREAM REQUEST header at request time. The other two request-driven types (`environment`, `metadata`) STAY parse-rejected loudly, each with its own distinct substring (envoy-go-strict, ADR-0080). `request_header` is the natural next slice after `literal`: it is the FIRST custom tag whose value is NOT static config — it needs per-request header access — which is exactly what makes it the foundational per-request-resolution seam that `metadata` will later reuse (§2.1).

The delivery is a complete, testable slice: an operator configuring `custom_tags: [{tag: "user_id", request_header: {name: "x-user-id", default_value: "anon"}}]` on the HCM `tracing` message gets a `{key: "user_id", value: <the x-user-id header value>}` STRING attribute on every sampled ingress SERVER span — on **both** the OTLP and Zipkin exporters (the single `Span.Attrs []KV` seam covers both, §2.3) — with the reference's `default_value` / omit-on-missing semantics (§2.4).

### 1.2 What phase 62 does NOT deliver (forward to §8)

NO `environment` custom-tag type (STATIC-resolvable but drags a differential-harness env-var-injection wrinkle — §2.1; deferred, rejected loudly). NO `metadata` custom-tag type (needs a dynamic-metadata lookup path envoy-go lacks — §2.1/§8; deferred, rejected loudly). NO other tracing follow-on (`spawn_upstream_span` / `http_service` / force-trace — each larger, §2.1/§8). NO `max_path_tag_length` (still-rejected knob, orthogonal). NO OTLP-metrics stats sink. NO new provider, transport, or stat. The four built-in span attributes currently emitted EMPTY (`upstream_cluster`/`node_id`/`zone`/`peer.address`, the `reference_tracing_upstream_cluster_framework_gap` framework gap) are UNTOUCHED — a `request_header` tag reads a REQUEST HEADER (fully available at the span-build seam), NOT the un-plumbed upstream/node/zone/peer per-request fields, so it does NOT touch that gap (do NOT conflate — §11).

### 1.3 Phase-done as the THIRTEENTH Observability-family row landing (family STAYS OPEN)

Row 62 is the THIRTEENTH Observability-family row and the SECOND `custom_tag` source type. After phase 62 phase-done the family STAYS OPEN — the deferred candidates in §8 remain (OTLP-metrics sink + `environment`/`metadata` custom-tag types + `spawn_upstream_span`/`http_service`/force-trace), so the sentinel check-(2) still prints (NARROWED to `environment`/`metadata` for `custom_tags`) ⇒ the loop continues.

### 1.4 ADR-0045 split readiness — anticipated a SINGLE FLAT ROW (escape-valve armable) *(self-answered; SPEC confirms)*

Anticipated a SINGLE FLAT ROW (~10–14 tasks — the config-model change [§2.5] + the per-request resolution helper [§2.6] + the 3 call-site threadings [§2.6] + the `request_header` parse arm lift + config/resolve/span unit tests + the fuzz seed + the fixture + the doc/BEHAVIOR_CONTRACT edits + verify + ADR-0283), comfortably under the ADR-0045 `~15` ceiling. There is no second subsystem to strand: the parse-arm, the config-model change, and the per-request resolution all sit on the SAME tracing engine, resolved at the SAME three call sites. The escape valve is documented ARMABLE and re-armed only if the SPEC's task count surprises upward (e.g. if the config-model refactor [D-RH-CONFIGMODEL, §2.5] or a multi-value/omit scenario matrix pushes the fixture/test leg into its own row).

### 1.5 Seed-stub alignment + package placement — ALL edits in EXISTING files/packages, ZERO new packages

- Production `request_header` parse arm (lift the reject; parse `CustomTag_Header{name, default_value}` into a spec): `internal/tracing/config.go` `parseCustomTags` (existing, `config.go:139`).
- Parsed-config home: the `tracing.TracingConfig` custom-tag field (`config.go:33-37`, currently `CustomTags []KV`) — reshaped to an ordered spec list OR extended with a `request_header` spec slice (§2.5, D-RH-CONFIGMODEL).
- Per-request resolution: a NEW small helper in `internal/tracing` (anticipated `ResolveCustomTags(specs, headerLookup) []KV`, §2.6) — literal → static KV; request_header → header value / default / omit.
- Call-site threading: `internal/filter/hcm/accesslog_emit.go` — the **THREE** `BuildServerSpan` call sites (`:55` H1, `:116` H2, `:177` H3 — the H3 site added at phase 61.2; the phase-59 BRAINSTORM predated it, so this row RE-DERIVES all three, `feedback_brief_citations_not_evidence`). The per-request header lookups already exist: `reqHeaderLookupH1(r)` (`accesslog_emit.go:218`, serves H1 AND H3 — both carry `r *http.Request`) / `reqHeaderLookupH2(req)` (`accesslog_emit.go:228`, scans `req.Headers` case-insensitively).
- `BuildServerSpan` (`span.go:64`) is anticipated UNCHANGED — it already takes the resolved `customTags []KV` and applies `upsertAttr`; resolution happens at the call site (where the typed header lookup lives), keeping `BuildServerSpan` header-representation-agnostic (§2.6).
- Fuzz SEED: `internal/filter/hcm/fuzz_test.go` `FuzzHCMConfigParse` (existing, `:25`) — a NEW `request_header` seed, NOT a new fuzzer (§2.7).
- Fixture: anticipated ONE new `test/fixtures/NNNN-tracing-custom-tags-request-header` dir (RE-DERIVE the next-free number at IMPL — `0104` is the current tail; `0105` anticipated).
- Docs: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (tracing section) + ROADMAP/STATE/DECISIONS.

Possibly ONE new small `.go` file only if the SPEC decides the resolution helper + spec type warrant it (a `resolve.go` in `internal/tracing` is plausible; still ZERO new packages). ZERO new modules.

### 1.6 No prebrainstorm-notes branch

No off-master prebrainstorm-notes branch exists for this subject. `request_header` is a recorded deferred candidate in the ROADMAP Observability family's LIVE deferred sentence (`… tracing custom_tags (request_header/environment/metadata)/…`) and in ADR-0277 §Consequences — not a stashed WIP.

### 1.7 Phase 62's relationship to the existing seams (a parse-arm lift + a per-request resolution step on the phase-59 engine)

The phase-59 engine already exposes the two config/emit seams this row extends: (a) `parseCustomTags` (`config.go:139`), where the wholesale-then-literal parse lives and the `request_header` lift lands; and (b) `Span.Attrs []KV` + `BuildServerSpan(…, customTags []KV, …)` + `upsertAttr` (`span.go:54`/`:64`/`:121`), the provider-neutral attribute append (upsert-by-key, last-write-wins — `reference_tracing_custom_tag_override_builtin`) that BOTH exporters consume (OTLP `toProto` `span.go:134`; Zipkin `zipkin.go:87`). The ONE genuinely NEW seam is the **per-request resolution** step (§2.6): the landed engine resolves the custom tags at CONFIG time (literal is static → a fixed `[]KV`), but `request_header` MUST resolve at REQUEST time (against the incoming header lookup). This is the row's central design work (D-RH-CONFIGMODEL / D-RH-RESOLVE-SEAM, §2.5/§2.6).

---

## 2. Design decisions

### 2.1 Row + subject confirmation: the Observability family continues with tracing `custom_tags` `request_header` *(SELF-PICKED per the standing directive → phase 62 row registered)*

The FIRST decision, made AUTONOMOUSLY (no human pick) per the 2026-07-12 standing directive. Picked as the **smallest cleanly-differential-provable candidate** from the live Observability deferred sentence, after INVESTIGATING each candidate's size AND its differential-provability against source this session (§11). Row 62 registers `in-progress` AT this BRAINSTORM commit per the ROADMAP §Schema invariant.

**Why `request_header` is the defensible pick:** (1) it reuses the WHOLE landed custom_tag engine — the parse dispatch, the `Span.Attrs []KV` seam, `upsertAttr`, both exporters — adding exactly one parse arm + one per-request resolution step; (2) it is CLEANLY differential-provable with the EXISTING harness — the tag value is driven by simply SENDING (or not sending) a request header, no env-var/container surgery; (3) it builds the reusable per-request custom-tag resolution seam that the remaining request-driven type (`metadata`) will later reuse; (4) the reference behavior is fully specified by the proto doc comment (§2.4) and deterministic.

**Rejected alternatives (recorded per the standing directive; each SIZED against source this session):**
- **`environment` custom_tag** — *the closest sibling, and honestly FEWER prod LoC than `request_header`.* An env-var value (`CustomTag_Environment{name, default_value}`, identical shape to `Header`) is STATIC for the process lifetime, so it is resolvable ONCE at CONFIG-PARSE time (`os.Getenv(name)` → a fixed `KV`, or omit) — the same static path as `literal`, needing NO per-request threading and NO call-site changes. BUT its DIFFERENTIAL requires injecting a matching env var into BOTH the reference `envoyproxy/envoy:contrib-v1.37.2` Docker container (`docker -e`) AND the subject envoy-go subprocess (the harness does not thread per-fixture subprocess env today — a harness wrinkle) AND a second no-env scenario to prove omit-on-missing. That harness env-injection surgery is comparable to — plausibly exceeds — `request_header`'s per-request threading, while proving a NARROWER capability. `request_header` proves cleanly by a header on the request and builds the FOUNDATIONAL per-request seam. So `request_header` is smallest-CLEANLY-PROVABLE + most-foundational. `environment` is the immediate follow-on (its static-resolution path is even cheaper once the harness gains subprocess-env threading). Deferred; rejected loudly this row.
- **`metadata` custom_tag** — reads a value from dynamic/filter/route METADATA (`CustomTag_Metadata{kind, metadata_key, default_value}`). envoy-go has no dynamic-metadata lookup path (grep confirms no metadata-key resolver at the span seam); this drags in a whole metadata-plumbing subsystem. LARGE. Deferred; rejected loudly this row.
- **`spawn_upstream_span`** — a second (upstream CLIENT) span with its own timing at the router/upstream seam; envoy-go emits ONE ingress SERVER span (`span.go:107` `Name: "ingress"`, `Kind: SERVER`). Medium-large; touches the span model. Deferred (per phase-59 §2.1).
- **`http_service` (OTLP HTTP transport)** — a new HTTP exporter transport alongside `envoy_grpc`. Medium. Deferred (per phase-59 §2.1).
- **force-trace (`x-envoy-force-trace`)** — needs an internal-request / edge-sanitization concept envoy-go lacks entirely (per phase-59 §2.1). Deferred.
- **OTLP-metrics stats sink** — a full new gRPC `stats_sinks[]` consumer. The largest remaining Observability follow-on. Deferred.
- **Opening a new family** (HTTP/3 / xDS / Operational-tooling OPEN; gRPC / Runtime / WASM never-opened) — the standing directive says smallest-defensible-first, and the Observability `custom_tags` tail STILL holds a cheap candidate (`request_header`), so smallest-first keeps us on the landed engine. Deferred; revisit when the Observability tail's cheap candidates drain.

### 2.2 Scope: `request_header` type ONLY; `environment`/`metadata` STAY parse-rejected loudly *(self-answered; the incremental-arm precedent)*

The `CustomTag` proto (`envoy.type.tracing.v3.CustomTag`, go-control-plane/envoy v1.32.4) is `tag` (field 1, string) + a `type` oneof: `literal` (2) / `environment` (3) / `request_header` (4, `CustomTag_Header{name, default_value}`) / `metadata` (5, `CustomTag_Metadata{kind, metadata_key, default_value}`). Phase 62 ADDS support for `request_header`; `literal` STAYS supported (phase 59); `environment`/`metadata` STAY rejected loudly with their existing distinct substrings (§2.7). This mirrors the project's landed incremental-arm posture (the phase-59 literal-only slice; the OTel `envoy_grpc`-transport-only reject; the Zipkin `HTTP_JSON`-only reject). A `request_header`-added slice is a complete, useful, deterministic capability (per-request labels like a user/tenant/request-id header on every span), and the SPEC probe confirms the reference emits it identically (D-RH-WIRE).

### 2.3 The span-emit seam is UNCHANGED: append the RESOLVED tags to `Span.Attrs` via `upsertAttr` — ONE path, TWO providers *(self-answered; the KV seam is landed)*

`BuildServerSpan` (`span.go:64`) already takes `customTags []KV` and applies each via `upsertAttr` (`span.go:99-101`/`:121`) AFTER the 16 built-ins + optional `guid:x-client-trace-id` — upsert-by-key, last-write-wins (a custom tag whose key collides with a built-in OVERRIDES it; `reference_tracing_custom_tag_override_builtin`). BOTH exporters consume `Attrs` (OTLP `toProto` `span.go:134-169`; Zipkin `zipkin.go:87-93`). So a `request_header` tag, once RESOLVED to a `KV{Key: tag, Str: <header value>}`, flows through the EXISTING append/upsert path with NO exporter-specific code and NO `BuildServerSpan` change. The ONLY difference from `literal` is WHEN the `[]KV` is produced: `literal` is resolved at config time (static); `request_header` is resolved at REQUEST time (§2.6). Values are STRING-valued.

### 2.4 Missing-header + default semantics — the reference's authoritative behavior (proto doc) *(self-answered shape; SPEC probes to PIN)*

`CustomTag_Header{name, default_value}` (`custom_tag.pb.go:262-273`). The proto doc comment (`custom_tag.pb.go:269-271`, `default_value` field) is AUTHORITATIVE: *"When the header does not exist, the tag value will be populated with this default value if specified, otherwise no tag will be populated."* So the reference behavior is:
- **header present** → emit `{tag, <header value>}`.
- **header absent, `default_value` set** → emit `{tag, <default_value>}`.
- **header absent, `default_value` empty/unset** → **OMIT the tag entirely** (no attribute emitted).

The **omit-on-missing** case is a NEW semantic vs `literal` (which never omits — it always emits its static value). The per-request resolution helper (§2.6) must SKIP (not emit an empty KV) in that case. This is a KEY behavioral pin — the SPEC live-probes all three arms (D-RH-MISSING) against `envoyproxy/envoy:contrib-v1.37.2` (fresh container per arm, `reference_probe_fresh_container_per_arm`). **Multi-value header** (a header sent twice): which value the reference uses (FIRST value vs comma-joined) is UNSPECIFIED by the proto and MUST be probed — the header lookup returns `([]string, bool)` (all values), and the resolution picks per the probe (D-RH-MULTIVALUE, §2.6).

### 2.5 Config model: an ORDERED custom-tag spec list *(anticipated Option B; SPEC pins D-RH-CONFIGMODEL)*

The phase-59 config home is `TracingConfig.CustomTags []KV` (`config.go:33-37`) — a fully-RESOLVED static list (literal only). `request_header` cannot be pre-resolved (its value is per-request), so the config must carry the header NAME + `default_value` + tag key. Two shapes:

- **Option A (two slices):** keep `CustomTags []KV` (literal, static) + add `RequestHeaderTags []HeaderTagSpec{TagKey, HeaderName, DefaultValue, HasDefault}`. At each call site, resolve the header specs → `[]KV` and pass `append(literalKVs, resolvedHeaderKVs...)`. **CON:** loses ORIGINAL CONFIG ORDER between literal and request_header tags — with header tags always appended AFTER literal tags, a `request_header` tag ALWAYS wins an `upsertAttr` key-collision against a literal tag regardless of config order, which DIVERGES from the reference IF the reference preserves config order.
- **Option B (unified ordered spec) — ANTICIPATED:** replace `CustomTags []KV` with an ordered `[]CustomTagSpec`, each a discriminated union `{TagKey; Kind (literal|request_header); LiteralValue; HeaderName; DefaultValue; HasDefault}`, in CONFIG ORDER. At each call site, resolve the WHOLE ordered list against the header lookup (literal → static value; request_header → header/default/omit) into `[]KV` in config order, then pass to `BuildServerSpan` (which upserts in order). **PRO:** preserves config order → correct `upsertAttr` last-write-wins on a same-key cross-type collision, matching the reference (which iterates `custom_tags` in config order, each `apply()` setting the tag); extensible cleanly to `environment`/`metadata` later. Literal resolution at the call site is a trivial constant copy (negligible cost; tracing is sampled/low-frequency).

**The central design decision** is D-RH-CONFIGMODEL: adopt Option B (ordered spec) for order-correctness UNLESS the SPEC's probe (a `literal` + a same-key `request_header`, tested BOTH config orders — D-RH-PRECEDENCE) shows the reference does NOT preserve order (or that PGV forbids duplicate `tag` keys, which it does NOT — `CustomTag.tag` has only `min_len: 1`), in which case Option A's simpler two-slice form suffices. SPEC pins.

### 2.6 Per-request resolution seam + the three call-site threadings *(anticipated shape; SPEC pins D-RH-RESOLVE-SEAM)*

A NEW helper (anticipated `tracing.ResolveCustomTags(specs []CustomTagSpec, headerLookup func(string) ([]string, bool)) []KV`, in `internal/tracing`) resolves the ordered specs per-request: `literal` → `KV{TagKey, LiteralValue}`; `request_header` → look up `HeaderName`; if found → `KV{TagKey, <value per D-RH-MULTIVALUE>}`; else if `HasDefault` → `KV{TagKey, DefaultValue}`; else SKIP (omit-on-missing, §2.4). Called at the THREE `accesslog_emit.go` `BuildServerSpan` call sites:
- **`:55` (H1, `emitAccessLog`)** — `reqHeaderLookupH1(r)` (`accesslog_emit.go:218`).
- **`:116` (H2, `emitAccessLogH2`)** — `reqHeaderLookupH2(req)` (`accesslog_emit.go:228`).
- **`:177` (H3, `emitAccessLogH3`)** — `reqHeaderLookupH1(r)` (H3 carries `r *http.Request`, so it reuses the H1 lookup).

`BuildServerSpan` stays UNCHANGED (still `customTags []KV`) — resolution happens at the call site where the typed lookup lives, keeping `BuildServerSpan` header-representation-agnostic (H1/H3 `http.Header` vs H2 `[]h2.HeaderField` differ). When the config has NO custom tags the call sites pass `nil` (the existing nil-safe path — `f.exporter != nil ⟹ f.tracingConfig != nil`). The resolution-helper location + signature is D-RH-RESOLVE-SEAM; the SPEC RE-DERIVES the three call sites + the two lookups. A minor seam sub-note is anticipated FOLDED into ADR-0283 (no separate seam ADR — the phase-59 precedent).

### 2.7 The two remaining unsupported tag types STILL reject loudly with DISTINCT substrings *(self-answered; ADR-0080)*

`environment`/`metadata` STAY rejected loudly (the reference SUPPORTS them; envoy-go rejecting them is a documented envoy-go-strict DEPARTURE, ADR-0080 anti-silent-divergence), with their EXISTING distinct substrings (unchanged from phase 59):
- `tracing: custom_tags environment type unsupported` (`config.go:157-158`)
- `tracing: custom_tags metadata type unsupported` (`config.go:159-160`)
The `request_header` substring (`config.go:155-156`) is REMOVED (the arm now parses). The existing structural rejects STAY: empty `tag` (`config.go:145-147`), typeless (`config.go:161-162`). A NEW structural reject arm is anticipated for an EMPTY `request_header.name` — PGV `Header.name` `min_len: 1` + pattern `^[^\x00\n\r]*$` (`custom_tag.pb.validate.go:583-594`); this is a PGV-PARITY reject (the reference boot-rejects it too), like the empty-tag/empty-literal-value/typeless rejects — SPEC decides whether envoy-go rejects it explicitly or relies on the reference's PGV boot-reject (D-RH-REJECT). `Header.default_value` is UNCONSTRAINED (empty is valid — the omit-on-missing case). One probe arm confirms the reference ACCEPTS a `request_header` tag (so the phase-59 departure genuinely NARROWS).

### 2.8 Fixture posture: anticipated ONE new fixture (OTLP); Zipkin path unit-tested *(self-answered direction; SPEC confirms D-RH-FIXTURE)*

A `request_header` tag is an OBSERVABLE span attribute, so it IS differential-provable: a NEW `test/fixtures/NNNN-tracing-custom-tags-request-header` dir (OTLP-provider) configures a `request_header` custom_tag, sends a request WITH the header, and asserts the `{key, value}` attribute cross-side on the OTLP span via the `test/helpers/otlptrace` receiver. Per the differential dispatch constraint (`reference_differential_fixture_dispatch_constraint` — one dir = one runner branch; do NOT mutate the pure-baseline `0087`/`0088` or the phase-59 `0102` literal fixture), it is a NEW dir. The default / omit-on-missing arms (§2.4) are anticipated proven by UNIT tests on `ResolveCustomTags` (deterministic, no differential needed) + possibly ADDITIONAL requests within the one fixture (SPEC weighs whether the harness cleanly drives a header-present AND a header-absent request against the same span receiver — D-RH-FIXTURE). The Zipkin encoder path is anticipated UNIT-tested (the phase-59 precedent — one OTLP fixture + a Zipkin unit test, ADR-0277 §Consequences), NOT a second fixture. Anticipated: fixtures **106 → 107** — SPEC pins (and re-derives the next-free number; `0104` is the current tail, `0105` anticipated). **Harness note:** the OTLP fixture drives H1/H2 over TCP (the OTLP receiver + the request are TCP) — NOT the H3/QUIC path, so `reference_differential_http_expectations_tcp_only` does not bite (this is not an H3 fixture); the span assertion lives in the otlptrace receiver, not `HTTPExpectations`.

### 2.9 Fuzz posture: a SEED to the EXISTING `FuzzHCMConfigParse` — NO new fuzzer *(self-answered; count stays 55 → SPEC confirms D-RH-FUZZSEED)*

The tracing config parse is reached via `NewConfig` off the HCM proto, fuzzed by `FuzzHCMConfigParse` (`internal/filter/hcm/fuzz_test.go:25`) — the phase-59 host for the `custom_tags` parse. The new `request_header` parse arm is exercised by ADDING a `request_header` seed (a `{name, default_value}` tag + a mixed literal+request_header config) to `FuzzHCMConfigParse` — NOT a new fuzzer. Fuzzer count STAYS **55** (`reference_fuzzer_count_docs_drift`: reconcile the documented running total against actual `^func Fuzz` before AND after — the count must NOT move). SPEC confirms D-RH-FUZZSEED (and that the HCM fuzzer reaches the tracing-config `parseCustomTags` arm — already true at phase 59).

### 2.10 Stat surface hypothesis: +0 *(self-answered; a span attribute registers no stat)*

A span attribute is emitted on the wire, not registered as a stat. The HCM tracing counters + the tracer counters are UNCHANGED. Anticipated stat surface **1201 (+0)**, UNCHANGED. No new registration path.

---

## 3. Framework-survey result — a parse-arm lift + a per-request resolution step on the phase-59 engine; ZERO new packages/modules (62 anticipated)

### 3.1 Framework: a config-model change + a per-request resolution helper (no new interface)

The one genuinely new piece is the per-request resolution step (§2.6) — a helper function + a config-model reshape (`[]KV` → `[]CustomTagSpec`, §2.5). No new interface, no new package-level type beyond the `CustomTagSpec` (or `HeaderTagSpec`) struct on the existing `TracingConfig`. `BuildServerSpan` is anticipated UNCHANGED. Every other symbol is pre-existing.

### 3.2 NEW packages: NONE

All edits land in `internal/tracing` + `internal/filter/hcm` (both existing) + `test/fixtures` + `docs/`. A new `internal/tracing/resolve.go` FILE is plausible (the resolution helper) but ZERO new packages.

### 3.3 go.mod modules: NONE

The `CustomTag_Header` proto is already reachable via the resolved `github.com/envoyproxy/go-control-plane/envoy v1.32.4` module (the same module phase 59 already imports as `tracingv3`, `config.go:12`). `ct.GetRequestHeader()` returns `*<tracing/v3>.CustomTag_Header`. No new module import. `go mod tidy -diff` anticipated EMPTY (modules STAY **2**).

### 3.4 REUSES

- **phase-59** the whole custom_tag engine: `parseCustomTags` (the parse home + the reject roster), `TracingConfig.CustomTags` (the config home — reshaped), `BuildServerSpan`/`Span.Attrs []KV`/`upsertAttr` (the append/upsert seam, `reference_tracing_custom_tag_override_builtin`), both exporters (OTLP + Zipkin consume `Attrs`), the `0102` literal fixture as a template, and the `FuzzHCMConfigParse` seed host.
- **the access-log header-capture seam** — `reqHeaderLookupH1`/`reqHeaderLookupH2` (`accesslog_emit.go:218`/`:228`), the existing case-insensitive per-request header lookups (`func(string) ([]string, bool)`), REUSED for the custom-tag resolution (§2.6) — no new header-access code.
- **the incremental-reject precedent** — the `environment`/`metadata` rejects STAY as the template; the `request_header` arm flips from reject to parse.
- **phase-46** the tracing engine + the `0087`/`0088` fixtures + the `test/helpers/otlptrace` receiver.

---

## 4. Bootstrap-level applicability — a PER-LISTENER HCM filter config (NOT bootstrap `stats_sinks[]`)

`custom_tags` is a PER-LISTENER HCM `tracing` sub-field, parsed by `NewConfig`/`parseCustomTags` from `HttpConnectionManager.tracing.custom_tags` when the HCM filter is built (the phase-59 home). No bootstrap change; the `request_header` lift lands INSIDE `parseCustomTags`. The fixture configures a `request_header` custom_tag on the listener's HCM tracing block.

---

## 5. Stat surface hypothesis — +0 (62)

### 5.1 Stat names (SPEC confirms)

NONE. A span attribute registers no stat.

### 5.2 envoy-go-strict departure flags

The `request_header` reject is LIFTED (the departure NARROWS — the reference and envoy-go now AGREE on `request_header`). `environment`/`metadata` STAY a documented envoy-go-strict DEPARTURE (reject loudly, §2.7). No new stat, no new flag; a parse-behavior change recorded in BEHAVIOR_CONTRACT.

### 5.3 Anticipated surface arithmetic

Stat surface **1201 → 1201 (+0)**.

---

## 6. Edit-site enumeration — RE-DERIVED this session (SPEC re-derives + pins D-RH-CONFIGMODEL / D-RH-RESOLVE-SEAM / D-RH-DOCSHAPE)

Each `file:line` RE-DERIVED against master `9f2494ed` this session (`feedback_brief_citations_not_evidence`); the SPEC re-derives again.

**Production — `internal/tracing/config.go`:**
1. **The `request_header` parse arm** — replace the reject (`config.go:155-156`) with: read `ct.GetRequestHeader()` → `{name, default_value}`; reject an empty `name` (PGV-parity, or rely on the reference boot-reject — D-RH-REJECT); append a `request_header` spec to the parsed list. `environment`/`metadata`/empty-tag/typeless rejects UNCHANGED. [EDIT]
2. **`TracingConfig` custom-tag field** (`config.go:33-37`) — reshape `CustomTags []KV` to an ordered `[]CustomTagSpec` (Option B, §2.5) OR add a `request_header` spec slice (Option A). D-RH-CONFIGMODEL. [EDIT]
3. **`parseCustomTags` return + `NewConfig` threading** (`config.go:88`/`:127`) — carry the reshaped spec list onto `TracingConfig` (provider-neutral, before the provider dispatch, as today). [EDIT]

**Production — `internal/tracing/` (resolution helper):**
4. **`ResolveCustomTags`** (NEW, anticipated `resolve.go` or in `span.go`/`config.go`) — resolve the ordered specs against a per-request header lookup → `[]KV` (literal static; request_header header/default/omit — §2.4/§2.6). [ADD]

**Production — `internal/tracing/span.go`:**
5. **`BuildServerSpan`** (`span.go:64`) — anticipated UNCHANGED (still `customTags []KV`; resolution moved to the call site). [NO CHANGE anticipated — SPEC confirms]

**Production — `internal/filter/hcm/accesslog_emit.go`:**
6. **The THREE `BuildServerSpan` call sites** (`:55` H1, `:116` H2, `:177` H3) — replace `f.tracingConfig.CustomTags` with `tracing.ResolveCustomTags(f.tracingConfig.CustomTagSpecs, <lookup>)` — lookup = `reqHeaderLookupH1(r)` (H1/H3) / `reqHeaderLookupH2(req)` (H2). [EDIT — 3 sites]

**Test:**
7. **`internal/tracing/config_test.go`** — accept a `request_header` tag (name+default); still reject `environment`/`metadata`/empty-tag/typeless; empty-header-name structural reject (D-RH-REJECT). [ADD]
8. **`internal/tracing/resolve_test.go`** (new, or in `span_test.go`) — resolution: header present→value, absent+default→default, absent+empty-default→omit; multi-value per D-RH-MULTIVALUE; upsert precedence vs a built-in + config order across a cross-type same-key collision (D-RH-PRECEDENCE). [ADD]
9. **`internal/filter/hcm/fuzz_test.go`** `FuzzHCMConfigParse` — a `request_header` SEED (name+default + a mixed literal+header config). [ADD — no new fuzzer]

**Fixture:**
10. **`test/fixtures/NNNN-tracing-custom-tags-request-header`** (new; `0105` anticipated) — a `request_header` custom_tag on an OTLP-provider listener; send the header; assert the `{key, value}` attribute cross-side. [ADD]

**BEHAVIOR_CONTRACT (`docs/envoy-go/BEHAVIOR_CONTRACT.md`):**
11. **the tracing section** — flip `request_header` from "rejected (envoy-go-strict)" to "consumed (per-request header value, with `default_value` / omit-on-missing; emitted as a span attribute on both exporters)"; `environment`/`metadata` STAY "reject loudly". SPEC RE-DERIVES the exact line(s). [EDIT]

**ROADMAP / STATE / DECISIONS:**
12. **ROADMAP** — row 62 `in-progress` at this BRAINSTORM (§Schema); the family prose gains a "phase 62 CHARTERED and BRAINSTORMED" sentence. The LIVE deferred sentence NARROWS `custom_tags (request_header/environment/metadata)` → `(environment/metadata)` at the phase-62 IMPL (NOT now — re-run the sentinel check-(2) grep after that edit, `reference_sentinel_deferred_sentence_live_vs_historical`, keeping EXACTLY ONE live "candidates:" match). [BRAINSTORM: row + prose; IMPL: deferred-list narrow]
13. **STATE.md** — active-phase header flips to phase 62 BRAINSTORM (this stage). [EDIT]
14. **DECISIONS.md** — ADR-0283 §Context drafts at the SPEC, §Decision/§Consequences at the IMPL (ADR-0044). NOT at this BRAINSTORM. [SPEC/IMPL]

SPEC pins **D-RH-DOCSHAPE** (this full edit-site roster, RE-DERIVED) + **D-RH-CONFIGMODEL** (§2.5) + **D-RH-RESOLVE-SEAM** (§2.6).

---

## 7. Anticipated ADRs — 1 at the phase-62 IMPL: ADR-0283 (tracing `custom_tags` `request_header`)

ADR-0283 (tracing `custom_tags` `request_header` type — lifting the one reject, the per-request header→span-attribute resolution [with `default_value`/omit-on-missing], the config-model change to an ordered spec list, the `environment`/`metadata` strict-reject narrowing). §Context drafted at the SPEC (provenance: the phase-59 literal-only slice + ADR-0277 §Consequences naming the three non-literal types + the ROADMAP deferred sentence), §Decision/§Consequences at the IMPL per ADR-0044. The config-model change (§2.5) + the resolution seam (§2.6) are anticipated FOLDED into ADR-0283 (no separate seam ADR — the phase-59 precedent); the SPEC re-decides if it finds a genuine standalone seam. Next-free after: **ADR-0284**.

---

## 8. Deferred items

- **`custom_tags` `environment` type** — an env-var value as a span tag (STATIC-resolvable; the immediate follow-on once the harness gains subprocess-env threading, §2.1). Carries forward.
- **`custom_tags` `metadata` type** — a dynamic-metadata value as a span tag (needs a metadata-lookup path, §2.1). Carries forward.
- **`spawn_upstream_span`** — a second (upstream CLIENT) span. Carries forward.
- **`http_service`** — an OTLP HTTP exporter transport. Carries forward.
- **force-trace (`x-envoy-force-trace`)** — needs internal-request detection + edge sanitization. Carries forward.
- **`max_path_tag_length`** — caps the `http.url` tag length; still rejected; orthogonal. Carries forward.
- **`OTLP-metrics` stats sink** — the largest remaining Observability sink follow-on. Carries forward.
- **`stats_flush_on_admin`** — still rejected; orthogonal. Carries forward.
- **The 4 EMPTY built-in span attributes** (`upstream_cluster`/`node_id`/`zone`/`peer.address`, `reference_tracing_upstream_cluster_framework_gap`) — a framework-surgery deferral, UNtouched here (a `request_header` tag reads a header, NOT those un-plumbed fields — do NOT conflate, §1.2/§11). Carries forward.
- **Multi-value header join policy beyond the probed default** — if the reference's multi-value behavior (D-RH-MULTIVALUE) has edge cases (e.g. a configurable join), only the probed default is implemented this row; edges carry forward.

After row 62 the `custom_tags` candidate NARROWS (literal + request_header done; `environment`/`metadata` remain) in the LIVE deferred sentence (at the IMPL); OTLP + the other tracing follow-ons remain ⇒ the sentinel check-(2) STILL prints ⇒ the loop continues.

---

## 9. Cross-references against prior phases' deferred-items lists — pickup

Phase 62 PICKS UP tracing `custom_tags` `request_header` — recorded in the ROADMAP Observability family's LIVE deferred sentence (`OTLP-metrics stats sink + tracing custom_tags (request_header/environment/metadata)/spawn_upstream_span/http_service/force-trace`) and in ADR-0277 §Consequences (the three non-literal types). After phase 62 the remaining `custom_tags` candidates are `environment`/`metadata`; plus `spawn_upstream_span`/`http_service`/force-trace + OTLP-metrics + `max_path_tag_length` + `stats_flush_on_admin`. The family STAYS OPEN. **Sentinel maintenance (at the IMPL):** after NARROWING `custom_tags` in the deferred sentence, re-run the check-(2) grep — EXACTLY ONE live "candidates:" match with the intended content (`reference_sentinel_deferred_sentence_live_vs_historical`).

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution

- **D-RH-SCOPE** — confirm the `request_header`-added slice (`environment`/`metadata` STAY rejected loudly with their existing distinct substrings, §2.7). §2.2.
- **D-RH-WIRE** — how the reference emits a `request_header` custom tag on the OTLP span: key = the `tag` name, value = the header value as a STRING `AnyValue`, appended after the built-ins (upsert-by-key). ONE fresh-container probe against `envoyproxy/envoy:contrib-v1.37.2` with a configured `request_header` custom_tag + a request carrying the header, observed via `test/helpers/otlptrace` (`reference_docker_probe_bridge_network`/`reference_host_gateway_ip_docker_desktop` for reachability). §2.3.
- **D-RH-MISSING** — the reference's missing-header behavior: header present → value; absent + `default_value` set → default; absent + empty default → OMIT the tag (proto doc, §2.4). Probe ALL THREE arms (fresh container each, `reference_probe_fresh_container_per_arm`). §2.4.
- **D-RH-MULTIVALUE** — a header sent multiple times: does the reference use the FIRST value or a comma-joined string? Probe. The resolution picks per the probe. §2.4/§2.6.
- **D-RH-PRECEDENCE** — upsert-by-key vs a colliding built-in (e.g. `tag: "http.method"`) AND config order across a cross-type same-key collision (a `literal` + a same-key `request_header`, BOTH config orders). Pins Option A-vs-B (§2.5). Probe. §2.3/§2.5.
- **D-RH-CONFIGMODEL** — the ordered-spec model (Option B, §2.5) vs the two-slice model (Option A). The CENTRAL design decision. Anticipated Option B for order-correctness; D-RH-PRECEDENCE informs it. §2.5.
- **D-RH-RESOLVE-SEAM** — the resolution helper location + signature (`ResolveCustomTags(specs, headerLookup) []KV`), and the threading at the THREE `accesslog_emit.go` call sites (`:55`/`:116`/`:177`) with `reqHeaderLookupH1`/`reqHeaderLookupH2`. RE-DERIVE the call sites + lookups. Confirm `BuildServerSpan` stays unchanged. §2.6.
- **D-RH-REJECT** — `environment`/`metadata` reject substrings UNCHANGED; the empty-`request_header.name` structural reject (PGV `Header.name` `min_len: 1` + pattern, `custom_tag.pb.validate.go:583-594` — envoy-go's own reject vs reliance on the reference PGV boot-reject); `Header.default_value` unconstrained. Confirm (one probe arm) the reference ACCEPTS `request_header` (the departure genuinely narrows). §2.7.
- **D-RH-FIXTURE** — ONE new OTLP fixture (`0105` anticipated; RE-DERIVE the number); header-present cross-side; default/omit arms as unit tests (and/or additional requests — SPEC weighs the harness's multi-request drive against one span receiver); Zipkin path unit-tested (the phase-59 precedent). Fixtures **106 → 107**. New dir (`reference_differential_fixture_dispatch_constraint`). §2.8.
- **D-RH-FUZZSEED** — a SEED to the EXISTING `FuzzHCMConfigParse` (NOT a new fuzzer); confirm the HCM fuzzer reaches `parseCustomTags`; fuzzer count STAYS 55 (`reference_fuzzer_count_docs_drift` — reconcile before AND after). §2.9.
- **D-RH-SPLIT** — the ADR-0045 disposition (SINGLE FLAT ROW anticipated, ~10–14 tasks; escape-valve armable only if the config-model refactor or the scenario matrix surprises upward). §1.4.

---

## 11. Prior-phase lessons applied

- **`feedback_brief_citations_not_evidence`** — EVERY `file:line` here (the `config.go` reject/parse sites, `span.go` `BuildServerSpan`/`upsertAttr`, the `accesslog_emit.go` THREE call sites + the two lookups, the `custom_tag.pb.go`/`.pb.validate.go` proto lines) is to be RE-DERIVED from source at the SPEC. This session RE-DERIVED them live against master `9f2494ed` — notably catching that phase 61.2 added a THIRD call site (`emitAccessLogH3`, `:177`), which the phase-59 BRAINSTORM (two call sites) predated.
- **`reference_tracing_custom_tag_override_builtin`** — the upsert-by-key (last-write-wins) seam is ALREADY landed (`span.go:121`); `request_header` reuses it unchanged. Config ORDER now matters for cross-type same-key collisions (§2.5) — the config-model choice must preserve it (Option B).
- **`reference_fuzzer_count_docs_drift`** — a SEED, not a fuzzer; reconcile the documented running total (55) against actual `^func Fuzz` before AND after — the count must NOT move. §2.9.
- **`reference_probe_fresh_container_per_arm`** + **`reference_envoy_contrib_image_tagging`** — each SPEC probe arm (D-RH-WIRE / D-RH-MISSING ×3 / D-RH-MULTIVALUE / D-RH-PRECEDENCE / D-RH-REJECT) runs on a FRESH container against `envoyproxy/envoy:contrib-v1.37.2`. §10.
- **`reference_docker_probe_bridge_network`** + **`reference_host_gateway_ip_docker_desktop`** — the OTLP span probes need a shared bridge network + a reachable receiver; verify the span decode ACTUALLY ran (not a vacuous empty capture). §10.
- **`reference_differential_fixture_dispatch_constraint`** — a new fixture dir per runner branch; do NOT mutate `0087`/`0088` (baselines) or `0102` (the phase-59 literal fixture). §2.8.
- **`reference_differential_http_expectations_tcp_only`** — the OTLP fixture is H1/H2-over-TCP (NOT H3/QUIC), so `HTTPExpectations` is usable in principle; but the span assertion lives in the `otlptrace` receiver, not `HTTPExpectations` — the fixture asserts the SPAN attribute, not an HTTP status. §2.8.
- **`reference_tracing_upstream_cluster_framework_gap`** — the 4 EMPTY built-in span attributes are a KNOWN framework gap; a `request_header` tag reads a REQUEST HEADER (fully available at the seam), NOT those un-plumbed upstream/node/zone/peer fields — INDEPENDENT of that gap (do NOT conflate). §1.2/§8.
- **`reference_sentinel_deferred_sentence_live_vs_historical`** — after the IMPL NARROWS `custom_tags` in the deferred sentence, re-run the check-(2) grep; EXACTLY ONE live "candidates:" match, correct content. §9.
- **`reference_strict_reject_sibling_typeurl_gap`** / **ADR-0080** — the `environment`/`metadata` rejects keep DISTINCT substrings so a future silent divergence surfaces; lifting `request_header` out of the reject roster is an explicit per-arm change (not a fall-through). §2.7.
- **`reference_fatalf_makes_assertions_unreachable`** — the resolve/span/fixture tests assert each independent property with `Errorf` (not `Fatalf`), so a `request_header` failure does not mask the built-in / literal assertions. §6.

---

## 12. Section closeout

**Settled:** subject (tracing `custom_tags` `request_header`, SELF-PICKED per the standing directive as the smallest CLEANLY-DIFFERENTIAL-PROVABLE candidate over `environment` [fewer prod LoC but drags harness env-injection] and the larger declined alternatives, §2.1); scope (`request_header` added; `environment`/`metadata` reject loudly with distinct substrings — a NARROWED envoy-go-strict departure, §2.2/§2.7); the span-emit seam (UNCHANGED — resolve to `[]KV`, then the landed `upsertAttr` path covers OTLP + Zipkin, §2.3); the missing-header semantics (present→value / absent+default→default / absent+empty→OMIT, §2.4); the config model (an ORDERED spec list, Option B anticipated, for cross-type upsert-order correctness — the central design decision, D-RH-CONFIGMODEL, §2.5); the per-request resolution seam (a new `ResolveCustomTags` helper threaded at the THREE `accesslog_emit.go` call sites via `reqHeaderLookupH1`/`reqHeaderLookupH2`; `BuildServerSpan` unchanged, §2.6); fixture posture (ONE new OTLP fixture; default/omit unit-tested; Zipkin unit-tested, §2.8); fuzz posture (a SEED to `FuzzHCMConfigParse`, no new fuzzer, §2.9); stat surface (+0, §2.10); envelope (SINGLE FLAT ROW anticipated, ~10–14 tasks — ADR-0283, §1.4). The novel production code is the `request_header` parse arm + the config-model reshape + the per-request `ResolveCustomTags` helper; the row's differential value is the OBSERVABLE per-request header span attribute.

**Anticipated moves at the phase-62 IMPL (docs-only now):** the `request_header` parse arm (lift `config.go:155-156`) + the `TracingConfig` config-model change + the `ResolveCustomTags` helper + the THREE call-site threadings + config/resolve/span unit tests + a `FuzzHCMConfigParse` seed + the new OTLP fixture + the BEHAVIOR_CONTRACT tracing edit + ADR-0283 + the ROADMAP deferred-sentence narrow. Counts: stat surface **1201 (+0)** · fixtures **106 → 107** · fuzzers **55 (+0, seed only)** · BackendKind **38 (+0)** · DECISIONS tail **ADR-0283** (next-free **ADR-0284**) · new Go packages **0** · new go.mod modules **0**.

**Counts UNCHANGED at this BRAINSTORM (docs-only; re-verified against master tip `9f2494ed`):** stat surface **1201** · fixtures **106** · fuzzers **55** · BackendKind **38** · DECISIONS tail **ADR-0282** (next-free **ADR-0283**) · go.mod modules **2**. Row 62 registers `in-progress` at this BRAINSTORM commit per the §Schema invariant.

**Next → the phase-62 SPEC** (the D-RH-* live-probe arms against `envoyproxy/envoy:contrib-v1.37.2` — D-RH-WIRE / D-RH-MISSING ×3 / D-RH-MULTIVALUE / D-RH-PRECEDENCE / D-RH-REJECT; re-derive every §6 edit site + the `custom_tag.pb.validate.go` PGV lines; pin D-RH-CONFIGMODEL + D-RH-RESOLVE-SEAM + D-RH-FIXTURE; draft ADR-0283 §Context).
