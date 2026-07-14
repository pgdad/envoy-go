# Phase 63 Brainstorm — tracing `custom_tags` `environment` SOURCE arm (the FOURTEENTH Observability-family row; the THIRD `custom_tag` source type after phase-59 `literal` and phase-62 `request_header` — lifts the `internal/tracing/config.go:196` `environment type unsupported` reject and reads a named PROCESS ENVIRONMENT VARIABLE into a span attribute [with `default_value` / omit-on-missing]; `metadata` STAYS parse-rejected loudly (envoy-go-strict, ADR-0080); +0 stats / +0 packages / +0 modules; anticipated ONE new fixture)

> **Stage:** BRAINSTORM (lifecycle-state 0 → 1). Docs-only; no `.go` changes at this stage. Fresh worktree `.worktrees/phase-63-brainstorm`, branch `phase-63-tracing-environment-custom-tag-brainstorm`, off master, per `feedback_git_worktrees`.
>
> **Loop re-open (AUTONOMOUS — no human pick):** phase 62 (`tracing-custom-tags-request-header`) landed COMPLETE (row 62 `done`, ADR-0283; the Observability family STAYS OPEN). Per the **STANDING DIRECTIVE (human, 2026-07-12)** the loop runs AUTONOMOUSLY until the termination sentinel fires; the sentinel was re-checked MECHANICALLY at the phase-62 IMPL and does NOT fire (check (1) silent — every row `done` — but check (2) prints THREE live "candidates:" sentences [HTTP/3, xDS, Observability] and check (3) prints THREE never-opened families [gRPC, Runtime, WASM], each independently blocking `stop`). No banked mid-lifecycle split legs remain (no `in-progress` ROADMAP rows). So the roller SELF-PICKED the next subject (§2.1): the **smallest cleanly-differential-provable candidate** from the live Observability deferred sentence — the tracing `custom_tags` **`environment`** source arm — over the declined larger/harder-to-prove alternatives (recorded §2.1). No human pause; no `stop` file.
>
> **Baselines re-verified against master tip `d14caa96` (the phase-62 IMPL squash):** stat surface **1201** · fixtures **107** (`ls -d test/fixtures/[0-9]*/ | wc -l`; tail `0105-tracing-custom-tags-request-header`; the count includes the lettered `0007a`/`0007b` sub-fixtures — a `^[0-9]{4}-` grep under-counts) · fuzzers **55** (`grep -rh '^func Fuzz' --include='*.go' . | wc -l`) · BackendKind tail **38** (`H2GoawayResponder`) · DECISIONS tail **ADR-0283** (next-free **ADR-0284**) · new Go packages **0** · go.mod modules **2** (`quic-go v0.54.1` direct + `qpack v0.5.1` indirect, both confined to `internal/listener` prod deps + `test/helpers` test deps). Counts are UNCHANGED at a BRAINSTORM (docs-only). All `file:line` citations below were RE-DERIVED from source this session (`feedback_brief_citations_not_evidence`) — see §11.

---

## 1. Mission and scope confirmation (63 — the THIRD `custom_tag` source; a PROCESS-STATIC one)

### 1.1 What phase 63 delivers as a self-contained whole (an environment-variable span attribute on the ingress span)

The HCM `tracing.custom_tags` parse today ACCEPTS `literal` (phase 59, ADR-0277) and `request_header` (phase 62, ADR-0283) and STRICT-REJECTS the other two, `environment` among them:

```go
// internal/tracing/config.go:195-196 (re-derived against master d14caa96)
case ct.GetEnvironment() != nil:
    return nil, fmt.Errorf("tracing: custom_tags environment type unsupported")
```

Phase 63 **lifts that one reject** and supports the **`environment`** `CustomTag` type — a `{tag, env-var-name, default_value}` spec whose span-attribute VALUE is read from a named PROCESS ENVIRONMENT VARIABLE. The last remaining source type (`metadata`) STAYS parse-rejected loudly, with its own distinct substring (envoy-go-strict, ADR-0080). `environment` is the natural next slice after `request_header`: it is the THIRD of the four `CustomTag` source types (`literal` / `environment` / `request_header` / `metadata`) and — unlike `request_header` — its value is STATIC for the process lifetime, so it reuses the landed machinery with an even smaller surface (§1.7, §2.5, §2.6).

The delivery is a complete, testable slice: an operator configuring `custom_tags: [{tag: "region", environment: {name: "ENVOY_REGION", default_value: "unknown"}}]` on the HCM `tracing` message gets a `{key: "region", value: <the ENVOY_REGION env value>}` STRING attribute on every sampled ingress SERVER span — on **both** the OTLP and Zipkin exporters (the single `Span.Attrs []KV` seam covers both, §2.3) — with the reference's `default_value` / omit-on-missing semantics (§2.4).

### 1.2 What phase 63 does NOT deliver (forward to §8)

NO `metadata` custom-tag type (needs a dynamic-metadata lookup path envoy-go lacks — §2.1/§8; deferred, rejected loudly — the LAST `custom_tag` source type). NO other tracing follow-on (`spawn_upstream_span` / `http_service` / force-trace — each larger, §2.1/§8). NO `max_path_tag_length` (still-rejected knob, orthogonal). NO OTLP-metrics stats sink. NO new provider, transport, or stat. The four built-in span attributes currently emitted EMPTY (`upstream_cluster`/`node_id`/`zone`/`peer.address`, the `reference_tracing_upstream_cluster_framework_gap` framework gap) are UNTOUCHED — an `environment` tag reads a PROCESS ENV VAR (fully available at parse/emit time), NOT the un-plumbed upstream/node/zone/peer per-request fields, so it does NOT touch that gap (do NOT conflate — §11).

### 1.3 Phase-done as the FOURTEENTH Observability-family row landing (family STAYS OPEN)

Row 63 is the FOURTEENTH Observability-family row and the THIRD `custom_tag` source type. After phase 63 phase-done the family STAYS OPEN — the deferred candidates in §8 remain (OTLP-metrics sink + `metadata` custom-tag type + `spawn_upstream_span`/`http_service`/force-trace), so the sentinel check-(2) still prints (NARROWED to `metadata` for `custom_tags`) ⇒ the loop continues.

### 1.4 ADR-0045 split readiness — anticipated a SINGLE FLAT ROW (escape-valve armable) *(self-answered; SPEC confirms)*

Anticipated a SINGLE FLAT ROW (~7–11 tasks — SMALLER than phase 62's 9: the per-request resolution seam + the three call-site threadings are ALREADY LANDED [phase 62; §1.7], so this row adds only the `environment` parse arm + the value-resolution decision [D-ENV-RESOLVE-TIME, §2.5] + config/resolve unit tests + the fuzz seed + the fixture + the doc/BEHAVIOR_CONTRACT edits + verify + ADR-0284), comfortably under the ADR-0045 `~15` ceiling. There is no second subsystem to strand: the parse-arm and the value resolution both sit on the SAME landed tracing engine, resolved at the SAME (already-threaded) call sites. The escape valve is documented ARMABLE and re-armed only if the SPEC's task count surprises upward (e.g. if the harness env-injection surgery for a value-equality fixture [D-ENV-FIXTURE / D-ENV-HARNESS, §2.8] balloons into its own leg).

### 1.5 Seed-stub alignment + package placement — ALL edits in EXISTING files/packages, ZERO new packages

- Production `environment` parse arm (lift the reject; parse `CustomTag_Environment{name, default_value}` into a spec): `internal/tracing/config.go` `parseCustomTags` (existing, `config.go:169`; the reject at `:195-196`).
- Parsed-config home: the `tracing.TracingConfig.CustomTags []CustomTagSpec` field (`config.go:39`; the phase-62 ORDERED first-wins-deduped spec list) — extended with an `environment` kind (or its value resolved into a `kindLiteral` spec at parse; §2.5, D-ENV-RESOLVE-TIME).
- Value resolution: EITHER at parse (`os.Getenv(name)` → a static value, degenerating to the `literal` path — env is process-static; §2.5) OR a new `kindEnvironment` case inside the LANDED `ResolveCustomTags` (`resolve.go:13`; §2.6, D-ENV-RESOLVE-TIME).
- Call-site threading: **NONE** — the THREE `accesslog_emit.go` `BuildServerSpan` call sites (`:55` H1 / `:116` H2 / `:177` H3) ALREADY call `tracing.ResolveCustomTags(f.tracingConfig.CustomTags, <lookup>)` (landed at phase 62). An `environment` spec flows through UNCHANGED — its resolution ignores the header lookup (§2.6). This is the load-bearing simplification vs phase 62.
- Fuzz SEED: `internal/filter/hcm/fuzz_test.go` `FuzzHCMConfigParse` (existing) — a NEW `environment` seed, NOT a new fuzzer (§2.7).
- Fixture: anticipated ONE new `test/fixtures/NNNN-tracing-custom-tags-environment` dir (RE-DERIVE the next-free number at IMPL — `0105` is the current tail; `0106` anticipated).
- Docs: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (tracing section) + ROADMAP/STATE/DECISIONS.

ZERO new packages. ZERO new modules.

### 1.6 No prebrainstorm-notes branch

No off-master prebrainstorm-notes branch exists for this subject. `environment` is a recorded deferred candidate in the ROADMAP Observability family's LIVE deferred sentence (`… tracing custom_tags (environment/metadata)/…`) and in ADR-0277/ADR-0283 §Consequences — not a stashed WIP.

### 1.7 Phase 63's relationship to the existing seams (a parse-arm lift on the phase-62 engine — the resolution seam is ALREADY threaded)

Phase 62 already built and threaded the two seams this row needs: (a) `parseCustomTags` (`config.go:169`) now returns an ORDERED, first-wins-deduped `[]CustomTagSpec` (`config.go:56-62`, a discriminated `{Key; Kind; LiteralValue; HeaderName; DefaultValue; HasDefault}`), where the `environment` accept arm lands; and (b) the per-request `ResolveCustomTags(specs, headerLookup) []KV` helper (`resolve.go:13`), ALREADY CALLED at all three `accesslog_emit.go` `BuildServerSpan` call sites (`:55`/`:116`/`:177`). So — unlike phase 62, which had to INTRODUCE the resolution seam and thread three call sites — phase 63 adds only a parse arm + a value-source decision. The genuinely NEW design question is narrow: `environment`'s value is PROCESS-STATIC (an env var does not change per request), so it can be resolved ONCE at CONFIG-PARSE time (`os.Getenv` → a fixed value, the same static path as `literal`) rather than per-request. That is the row's central (and only) design decision — D-ENV-RESOLVE-TIME (§2.5). Everything else — the `Span.Attrs []KV` / `upsertAttr` append (`reference_tracing_custom_tag_override_builtin`), both exporters, the first-wins dedup, the empty-name PGV-parity reject — is the phase-62 landed machinery reused unchanged.

---

## 2. Design decisions

### 2.1 Row + subject confirmation: the Observability family continues with tracing `custom_tags` `environment` *(SELF-PICKED per the standing directive → phase 63 row registered)*

The FIRST decision, made AUTONOMOUSLY (no human pick) per the 2026-07-12 standing directive. Picked as the **smallest cleanly-differential-provable candidate** from the live Observability deferred sentence, after INVESTIGATING each candidate's size AND its differential-provability against source this session (§11). Row 63 registers `in-progress` AT this BRAINSTORM commit per the ROADMAP §Schema invariant.

**Why `environment` is the defensible pick:** (1) it reuses the WHOLE landed custom_tag engine — the parse dispatch, the first-wins-deduped `[]CustomTagSpec`, the ALREADY-THREADED `ResolveCustomTags` seam, `Span.Attrs []KV`, `upsertAttr`, both exporters — adding exactly one parse arm + one value-source decision, with ZERO call-site changes (§1.7); (2) it is the SMALLEST remaining `custom_tag` source (its value is PROCESS-STATIC, so it needs no per-request threading — even smaller than `request_header`); (3) the reference behavior is fully specified by the proto doc comment (§2.4), IDENTICAL to `request_header`'s missing semantics, and deterministic; (4) it is the immediate follow-on the phase-62 BRAINSTORM (§2.1) explicitly named. The ONE novelty is the differential HARNESS: proving the value cross-side needs a matching env var in BOTH the reference container AND the subject subprocess — a small, bounded env-injection surgery (or a header-58-style key-presence dodge, §2.8) — which is why it was ranked BEHIND `request_header` at phase 62 but is now the smallest remaining candidate.

**Rejected alternatives (recorded per the standing directive; each SIZED against source this session):**
- **`metadata` custom_tag** — reads a value from dynamic/filter/route METADATA (`CustomTag_Metadata{kind, metadata_key, default_value}`). envoy-go has no dynamic-metadata lookup path (grep confirms no metadata-key resolver at the span seam); this drags in a whole metadata-plumbing subsystem. LARGE — the LAST and biggest of the four `custom_tag` source types. Deferred; rejected loudly this row (the sole remaining reject after phase 63).
- **`spawn_upstream_span`** — a second (upstream CLIENT) span with its own timing at the router/upstream seam; envoy-go emits ONE ingress SERVER span (`span.go` `Name: "ingress"`, `Kind: SERVER`). Medium-large; touches the span model. Deferred (per phase-59/62 §2.1).
- **`http_service` (OTLP HTTP transport)** — a new HTTP exporter transport alongside `envoy_grpc`. Medium. Deferred (per phase-59/62 §2.1).
- **force-trace (`x-envoy-force-trace`)** — needs an internal-request / edge-sanitization concept envoy-go lacks entirely. Deferred.
- **OTLP-metrics stats sink** — a full new gRPC `stats_sinks[]` consumer. The largest remaining Observability follow-on. Deferred.
- **Opening a new family** (HTTP/3 / xDS / Operational-tooling OPEN; gRPC / Runtime / WASM never-opened) — the standing directive says smallest-defensible-first, and the Observability `custom_tags` tail STILL holds a cheap candidate (`environment`), so smallest-first keeps us on the landed engine. Deferred; revisit when the Observability tail's cheap candidates drain (after this row, only `metadata` + the larger follow-ons remain).

### 2.2 Scope: `environment` type ONLY; `metadata` STAYS parse-rejected loudly *(self-answered; the incremental-arm precedent)*

The `CustomTag` proto (`envoy.type.tracing.v3.CustomTag`) is `tag` (field 1, string) + a `type` oneof: `literal` (2) / `environment` (3, `CustomTag_Environment{name, default_value}`) / `request_header` (4) / `metadata` (5). Phase 63 ADDS support for `environment`; `literal` (phase 59) + `request_header` (phase 62) STAY supported; `metadata` STAYS rejected loudly with its existing distinct substring (§2.7). This mirrors the project's landed incremental-arm posture (the phase-59 literal-only slice; the phase-62 request_header add). An `environment`-added slice is a complete, useful, deterministic capability (process-level labels like a region/zone/build env var on every span), and the SPEC probe confirms the reference emits it identically (D-ENV-WIRE).

### 2.3 The span-emit seam is UNCHANGED: append the RESOLVED tags to `Span.Attrs` via `upsertAttr` — ONE path, TWO providers *(self-answered; the KV seam is landed)*

`BuildServerSpan` already takes `customTags []KV` and applies each via `upsertAttr` (`span.go`) AFTER the built-ins — upsert-by-key (a custom tag whose key collides with a built-in OVERRIDES it, `reference_tracing_custom_tag_override_builtin`), and `ResolveCustomTags` guarantees unique keys among custom tags (the phase-62 first-wins dedup). BOTH exporters consume `Attrs` (OTLP `toProto`; Zipkin `zipkin.go`). So an `environment` tag, once RESOLVED to a `KV{Key: tag, Str: <env value>}`, flows through the EXISTING append/upsert path with NO exporter-specific code and NO `BuildServerSpan` change. The ONLY question is WHEN the `[]KV` is produced (§2.5). Values are STRING-valued.

### 2.4 Missing-env-var + default semantics — the reference's authoritative behavior (proto doc) *(self-answered shape; SPEC probes to PIN)*

`CustomTag_Environment{name, default_value}` (`custom_tag.pb.go:226-236`). The proto doc comment (`custom_tag.pb.go:230-232`, `default_value` field) is AUTHORITATIVE and IDENTICAL in shape to `request_header`'s: *"When the environment variable is not found, the tag value will be populated with this default value if specified, otherwise no tag will be populated."* So the reference behavior is:
- **env var present** → emit `{tag, <env value>}`.
- **env var absent, `default_value` set** → emit `{tag, <default_value>}`.
- **env var absent, `default_value` empty/unset** → **OMIT the tag entirely** (no attribute emitted).

This is the SAME three-arm shape phase 62 landed for `request_header` (present / default / omit) — so the `ResolveCustomTags` omit-on-missing logic ALREADY EXISTS (`resolve.go`). The SPEC live-probes all three arms (D-ENV-MISSING) against `envoyproxy/envoy:contrib-v1.37.2` (fresh container per arm, `reference_probe_fresh_container_per_arm`) to CONFIRM parity (there is one open subtlety — whether the reference reads the env var at CONFIG-LOAD or at REQUEST time; both are observationally identical for a static env, but they differ in the OMIT-at-parse-vs-resolve interaction with first-wins dedup, D-ENV-DEDUP §2.5).

### 2.5 Value-source decision: PARSE-time-static vs a `kindEnvironment` case in `ResolveCustomTags` *(THE central design question — D-ENV-RESOLVE-TIME)*

An env var is STATIC for the process lifetime — Envoy resolves `environment` custom tags at CONFIG-LOAD time. Two envoy-go shapes:

- **Option A (parse-time-static) — LEAN:** in `parseCustomTags`, resolve `os.Getenv(name)` immediately: env present → emit a `CustomTagSpec{Key, Kind: kindLiteral, LiteralValue: <env value>}` (degenerate to the landed literal path); env absent + `default_value` set → `kindLiteral` with the default; env absent + empty default → **emit NO spec** (omit-at-parse). **PRO:** matches Envoy's config-load resolution; ZERO per-request cost; ZERO `ResolveCustomTags` change; ZERO new kind. **CON / open subtlety:** the omit-at-parse case interacts with the phase-62 first-wins dedup (`config.go:202-206`) — does an omitted `environment` tag still RESERVE its config-order key slot (blocking a later same-key tag), or does it vanish entirely (letting a later same-key tag win)? The reference resolves at config-load, so an omitted env tag likely still occupies its dedup slot (the key was seen in config order) — but this MUST be probed (D-ENV-DEDUP). If the reference reserves the slot, the parse-time resolution must record the key in `seen` even when it omits the value.
- **Option B (a `kindEnvironment` case in `ResolveCustomTags`):** add `kindEnvironment` to `customTagKind` (`config.go:44-49`); store `{Key, Kind: kindEnvironment, EnvName, DefaultValue, HasDefault}` at parse; resolve `os.Getenv` inside `ResolveCustomTags` (`resolve.go`) per-request (env doesn't change, so redundant but uniform with `request_header`). **PRO:** structural uniformity with the landed seam; the dedup slot is naturally reserved at parse (the spec exists in the list). **CON:** redundant per-request `os.Getenv`; a new kind for a value that never varies.

**The decision** is D-ENV-RESOLVE-TIME: LEAN Option A (parse-time-static, matching Envoy) for minimal surface UNLESS the D-ENV-DEDUP probe shows the omit-at-parse dedup interaction is cleaner under Option B's parse-time-reserved slot. The SPEC probes D-ENV-DEDUP (a `literal` + a same-key omitting-`environment`, BOTH config orders) and pins the choice. Either way the resolved `[]KV` is byte-identical for the common (single-key, env-present) case.

### 2.6 The resolution seam is ALREADY threaded — NO call-site changes *(self-answered; the phase-62 landing)*

The THREE `accesslog_emit.go` `BuildServerSpan` call sites (`:55` H1 / `:116` H2 / `:177` H3) ALREADY call `tracing.ResolveCustomTags(f.tracingConfig.CustomTags, reqHeaderLookupH1/H2(...))` (phase 62). An `environment` spec resolves WITHOUT consulting the header lookup (Option A: it is already a `kindLiteral` static value; Option B: the `kindEnvironment` case ignores `headerLookup` and reads `os.Getenv`). So `ResolveCustomTags`'s existing `headerLookup` parameter is simply unused by the environment arm — NO call-site signature change, NO new lookup, NO `BuildServerSpan` change. This is the concrete payoff of phase 62 having built the general resolution seam: `environment` is a pure additive parse arm.

### 2.7 The one remaining unsupported tag type STILL rejects loudly with a DISTINCT substring *(self-answered; ADR-0080)*

`metadata` STAYS rejected loudly (the reference SUPPORTS it; envoy-go rejecting it is a documented envoy-go-strict DEPARTURE, ADR-0080 anti-silent-divergence), with its EXISTING distinct substring (unchanged from phase 59/62):
- `tracing: custom_tags metadata type unsupported` (`config.go:197-198`)
The `environment` substring (`config.go:195-196`) is REMOVED (the arm now parses). The existing structural rejects STAY: empty `tag` (`config.go:177-179`), typeless (`config.go:199-200`), empty `literal.value` (`config.go:184-186`), empty `request_header.name` (`config.go:190-192`). A NEW structural reject arm is anticipated for an EMPTY `environment.name` — PGV `CustomTag_Environment.name` `min_len: 1` (CONFIRMED this session at go-control-plane/envoy v1.37.0 `custom_tag.pb.validate.go`: `value length must be at least 1 runes`, EXACT parity with `Header.name`); this is a PGV-PARITY reject (the reference boot-rejects it too), like the empty-tag/empty-name rejects — SPEC decides whether envoy-go rejects it explicitly (the phase-62 precedent added an explicit empty-`request_header.name` reject) or relies on the reference PGV boot-reject (D-ENV-REJECT), and RE-DERIVES the exact `.pb.validate.go` line against the PROJECT-pinned go-control-plane version (the project resolves `/envoy v1.32.4` — the v1.37.0 line read this session is only a shape confirmation). `Environment.default_value` is UNCONSTRAINED (empty is valid — the omit-on-missing case). One probe arm confirms the reference ACCEPTS an `environment` tag (so the phase-59/62 departure genuinely NARROWS to `metadata` only).

### 2.8 Fixture posture: anticipated ONE new fixture (OTLP); default/omit/Zipkin unit-tested *(self-answered direction; SPEC pins D-ENV-FIXTURE)*

An `environment` tag is an OBSERVABLE span attribute, so it IS differential-provable — but its value source (the process env) makes the cross-side fixture the row's ONE genuinely novel piece (§2.1). A NEW `test/fixtures/NNNN-tracing-custom-tags-environment` dir (OTLP-provider) configures an `environment` custom_tag and asserts the `{key, value}` attribute cross-side on the OTLP span via the `test/helpers/otlptrace` receiver. Per the differential dispatch constraint (`reference_differential_fixture_dispatch_constraint` — one dir = one runner branch; do NOT mutate `0087`/`0088` baselines or the `0102`/`0105` custom-tag fixtures), it is a NEW dir. Two fixture strategies (the SPEC weighs + probes):

- **Value-equality (strong) — anticipated:** inject a matching env var into BOTH sides — the reference container via testcontainers `ContainerRequest.Env` (or `HostConfig.Env`; native support, a small `harness.go` `startReferenceProxy` addition threaded from the fixture) AND the subject subprocess via `cmd.Env = append(os.Environ(), "NAME=value")` in `StartSubjectProxy` (`harness.go:234`, a one-line addition). Asserts the tag VALUE cross-side. A small, bounded TWO-STARTER env-injection surgery (D-ENV-HARNESS) — far smaller than the phase-61.3 UDP surgery.
- **Key-presence (cheap, no harness surgery):** use an env var NATURALLY present on both sides (e.g. `PATH`, always set in a Docker container AND in a Go subprocess inheriting `os.Environ()`), asserting the tag with that KEY appears on BOTH sides (value differs, so key-only — the phase-62 "present-case cross-side by key" precedent). NO env injection.

**D-ENV-FIXTURE** picks between them (probe-driven, weighing the harness cost against proof strength — LEAN value-equality if the injection is confirmed one-line-per-starter, else key-presence). The default / omit-on-missing / dedup arms (§2.4/§2.5) are anticipated proven by UNIT tests on `parseCustomTags` + `ResolveCustomTags` (deterministic, no differential needed — the phase-62 precedent). The Zipkin encoder path is anticipated UNIT-tested (the phase-59/62 precedent — one OTLP fixture + a Zipkin unit test), NOT a second fixture. Anticipated: fixtures **107 → 108** — SPEC pins (and re-derives the next-free number; `0105` is the current tail, `0106` anticipated). **Harness note:** the OTLP fixture drives H1/H2 over TCP — NOT the H3/QUIC path — so `reference_differential_http_expectations_tcp_only` does not bite; the span assertion lives in the `otlptrace` receiver, not `HTTPExpectations`.

### 2.9 Fuzz posture: a SEED to the EXISTING `FuzzHCMConfigParse` — NO new fuzzer *(self-answered; count stays 55 → SPEC confirms D-ENV-FUZZSEED)*

The tracing config parse is reached via `NewConfig` off the HCM proto, fuzzed by `FuzzHCMConfigParse` (`internal/filter/hcm/fuzz_test.go`) — the phase-59/62 host for the `custom_tags` parse. The new `environment` parse arm is exercised by ADDING an `environment` seed (a `{name, default_value}` tag + a mixed literal+environment+request_header config) to `FuzzHCMConfigParse` — NOT a new fuzzer. Fuzzer count STAYS **55** (`reference_fuzzer_count_docs_drift`: reconcile the documented running total against actual `^func Fuzz` before AND after — the count must NOT move). SPEC confirms D-ENV-FUZZSEED.

### 2.10 Stat surface hypothesis: +0 *(self-answered; a span attribute registers no stat)*

A span attribute is emitted on the wire, not registered as a stat. The HCM tracing counters + the tracer counters are UNCHANGED. Anticipated stat surface **1201 (+0)**, UNCHANGED. No new registration path.

---

## 3. Framework-survey result — a pure additive parse arm on the phase-62 engine; ZERO new packages/modules (63 anticipated)

### 3.1 Framework: a parse-arm lift + a value-source decision (no new interface, no new seam)

Unlike phase 62 (which introduced the `ResolveCustomTags` resolution seam + the ordered spec model), phase 63 introduces NOTHING structurally new: the seam, the ordered first-wins spec model, the three threaded call sites, and the omit-on-missing logic ALL EXIST. The row adds one parse arm + (at most) one `customTagKind` value (`kindEnvironment`, Option B) or ZERO new kind (Option A). No new interface, no new package-level type, no new seam. `BuildServerSpan`, `ResolveCustomTags`'s signature, and all three call sites are UNCHANGED.

### 3.2 NEW packages: NONE

All edits land in `internal/tracing` (`config.go` + possibly `resolve.go`, both existing) + `internal/filter/hcm` (fuzz seed only) + `test/fixtures` + `test/differential` (the env-injection harness addition, if value-equality) + `docs/`. ZERO new packages.

### 3.3 go.mod modules: NONE

`CustomTag_Environment` is already reachable via the resolved `github.com/envoyproxy/go-control-plane/envoy v1.32.4` module (the same module phase 59/62 import as `tracingv3`). `ct.GetEnvironment()` returns `*<tracing/v3>.CustomTag_Environment`. `os.Getenv` is stdlib. No new module import. `go mod tidy -diff` anticipated EMPTY (modules STAY **2**).

### 3.4 REUSES

- **phase-59 + phase-62** the whole custom_tag engine: `parseCustomTags` (the parse home + the reject roster + the first-wins dedup), `TracingConfig.CustomTags []CustomTagSpec` (the ordered spec home), `ResolveCustomTags` (the resolution seam — its omit-on-missing logic REUSED for env-var-absent), `BuildServerSpan`/`Span.Attrs []KV`/`upsertAttr` (the append/upsert seam, `reference_tracing_custom_tag_override_builtin`), both exporters, the `0102`/`0105` custom-tag fixtures as templates, and the `FuzzHCMConfigParse` seed host.
- **phase 62** the ALREADY-THREADED three call sites (`:55`/`:116`/`:177`) — ZERO change (§2.6).
- **the incremental-reject precedent** — the `metadata` reject STAYS as the template; the `environment` arm flips from reject to parse.
- **phase-46** the tracing engine + the `0087`/`0088` fixtures + the `test/helpers/otlptrace` receiver.

---

## 4. Bootstrap-level applicability — a PER-LISTENER HCM filter config (NOT bootstrap `stats_sinks[]`)

`custom_tags` is a PER-LISTENER HCM `tracing` sub-field, parsed by `NewConfig`/`parseCustomTags` from `HttpConnectionManager.tracing.custom_tags` when the HCM filter is built (the phase-59/62 home). No bootstrap change; the `environment` lift lands INSIDE `parseCustomTags`. The fixture configures an `environment` custom_tag on the listener's HCM tracing block. (The env-injection, if value-equality, is a HARNESS/process-env concern, NOT a bootstrap-config one — §2.8.)

---

## 5. Stat surface hypothesis — +0 (63)

### 5.1 Stat names (SPEC confirms)

NONE. A span attribute registers no stat.

### 5.2 envoy-go-strict departure flags

The `environment` reject is LIFTED (the departure NARROWS — the reference and envoy-go now AGREE on `environment`). `metadata` STAYS the SOLE documented envoy-go-strict `custom_tags` DEPARTURE (reject loudly, §2.7). No new stat, no new flag; a parse-behavior change recorded in BEHAVIOR_CONTRACT.

### 5.3 Anticipated surface arithmetic

Stat surface **1201 → 1201 (+0)**.

---

## 6. Edit-site enumeration — RE-DERIVED this session (SPEC re-derives + pins D-ENV-RESOLVE-TIME / D-ENV-FIXTURE / D-ENV-DOCSHAPE)

Each `file:line` RE-DERIVED against master `d14caa96` this session (`feedback_brief_citations_not_evidence`); the SPEC re-derives again.

**Production — `internal/tracing/config.go`:**
1. **The `environment` parse arm** — replace the reject (`config.go:195-196`) with: read `ct.GetEnvironment()` → `{name, default_value}`; reject an empty `name` (PGV-parity, or rely on the reference boot-reject — D-ENV-REJECT); EITHER resolve `os.Getenv(name)` → a `kindLiteral` spec / omit (Option A) OR append a `kindEnvironment` spec (Option B) — D-ENV-RESOLVE-TIME. `metadata`/empty-tag/typeless/literal-empty/header-empty rejects UNCHANGED. Preserve the first-wins dedup interaction (D-ENV-DEDUP). [EDIT]
2. **`customTagKind` + `CustomTagSpec`** (`config.go:44-62`) — Option B adds `kindEnvironment` + reuses `HeaderName`/`DefaultValue`/`HasDefault` (renamed conceptually to an env name, or an added `EnvName` field); Option A adds NOTHING (collapses to `kindLiteral`). D-ENV-RESOLVE-TIME. [EDIT — Option B only]

**Production — `internal/tracing/resolve.go`:**
3. **`ResolveCustomTags`** (`resolve.go:13`) — Option A: UNCHANGED (environment already a `kindLiteral`); Option B: ADD a `case kindEnvironment:` reading `os.Getenv(s.EnvName)` → value / default / omit (mirroring the `kindRequestHeader` arm, ignoring `headerLookup`). D-ENV-RESOLVE-TIME. [EDIT — Option B only]

**Production — `internal/filter/hcm/accesslog_emit.go`:**
4. **The THREE `BuildServerSpan` call sites** (`:55`/`:116`/`:177`) — UNCHANGED (already thread `ResolveCustomTags`; §2.6). [NO CHANGE]

**Test:**
5. **`internal/tracing/config_test.go`** — accept an `environment` tag (name+default); still reject `metadata`/empty-tag/typeless; empty-env-name structural reject (D-ENV-REJECT); the first-wins dedup with an environment spec + the omit-at-parse dedup interaction (D-ENV-DEDUP). [ADD]
6. **`internal/tracing/resolve_test.go`** — resolution: env present→value, absent+default→default, absent+empty-default→omit; upsert precedence vs a built-in + config order across a cross-type same-key collision (mirror the phase-62 arms). [ADD]
7. **`internal/filter/hcm/fuzz_test.go`** `FuzzHCMConfigParse` — an `environment` SEED (name+default + a mixed config). [ADD — no new fuzzer]

**Fixture + harness:**
8. **`test/fixtures/NNNN-tracing-custom-tags-environment`** (new; `0106` anticipated) — an `environment` custom_tag on an OTLP-provider listener; assert the `{key, value}` attribute cross-side. [ADD]
9. **`test/differential/harness.go`** (D-ENV-HARNESS, value-equality fixture ONLY) — thread an env-injection param into `startReferenceProxy` (testcontainers `ContainerRequest.Env`, `harness.go:124`) + `StartSubjectProxy` (`cmd.Env`, `harness.go:252`). SPEC weighs value-equality vs key-presence (D-ENV-FIXTURE) — this edit lands only under value-equality. [ADD — conditional]

**BEHAVIOR_CONTRACT (`docs/envoy-go/BEHAVIOR_CONTRACT.md`):**
10. **the tracing section** — flip `environment` from "rejected (envoy-go-strict)" to "consumed (process env-var value, with `default_value` / omit-on-missing; emitted as a span attribute on both exporters; resolved at config-parse per D-ENV-RESOLVE-TIME)"; `metadata` STAYS "reject loudly" (the SOLE remaining departure). SPEC RE-DERIVES the exact line(s). [EDIT]

**ROADMAP / STATE / DECISIONS:**
11. **ROADMAP** — row 63 `in-progress` at this BRAINSTORM (§Schema); the family prose gains a "phase 63 CHARTERED and BRAINSTORMED" sentence. The LIVE deferred sentence NARROWS `custom_tags (environment/metadata)` → `(metadata)` at the phase-63 IMPL (NOT now — re-run the sentinel check-(2) grep after that edit, `reference_sentinel_deferred_sentence_live_vs_historical`, keeping EXACTLY ONE live "candidates:" match). [BRAINSTORM: row + prose; IMPL: deferred-list narrow]
12. **STATE.md** — active-phase header flips to phase 63 BRAINSTORM (this stage). [EDIT]
13. **DECISIONS.md** — ADR-0284 §Context drafts at the SPEC, §Decision/§Consequences at the IMPL (ADR-0044). NOT at this BRAINSTORM. [SPEC/IMPL]

SPEC pins **D-ENV-DOCSHAPE** (this full edit-site roster, RE-DERIVED) + **D-ENV-RESOLVE-TIME** (§2.5) + **D-ENV-FIXTURE** (§2.8).

---

## 7. Anticipated ADRs — 1 at the phase-63 IMPL: ADR-0284 (tracing `custom_tags` `environment`)

ADR-0284 (tracing `custom_tags` `environment` type — lifting the one reject, the process-env-var→span-attribute resolution [with `default_value`/omit-on-missing], the parse-time-static-vs-`kindEnvironment` value-source decision [D-ENV-RESOLVE-TIME], the `metadata` strict-reject narrowing to the SOLE remaining `custom_tags` departure). §Context drafted at the SPEC (provenance: the phase-59 literal + phase-62 request_header slices + ADR-0277/ADR-0283 §Consequences naming the non-literal types + the ROADMAP deferred sentence), §Decision/§Consequences at the IMPL per ADR-0044. The value-source decision (§2.5) + the env-injection harness note (§2.8) are anticipated FOLDED into ADR-0284 (no separate seam ADR — the phase-59/62 precedent); the SPEC re-decides if it finds a genuine standalone seam. Next-free after: **ADR-0285**.

---

## 8. Deferred items

- **`custom_tags` `metadata` type** — a dynamic-metadata value as a span tag (needs a metadata-lookup path, §2.1). The LAST `custom_tag` source type. Carries forward — after row 63 it is the SOLE remaining `custom_tags` reject.
- **`spawn_upstream_span`** — a second (upstream CLIENT) span. Carries forward.
- **`http_service`** — an OTLP HTTP exporter transport. Carries forward.
- **force-trace (`x-envoy-force-trace`)** — needs internal-request detection + edge sanitization. Carries forward.
- **`max_path_tag_length`** — caps the `http.url` tag length; still rejected; orthogonal. Carries forward.
- **`OTLP-metrics` stats sink** — the largest remaining Observability sink follow-on. Carries forward.
- **`stats_flush_on_admin`** — still rejected; orthogonal. Carries forward.
- **The 4 EMPTY built-in span attributes** (`upstream_cluster`/`node_id`/`zone`/`peer.address`, `reference_tracing_upstream_cluster_framework_gap`) — a framework-surgery deferral, UNtouched here (an `environment` tag reads a process env var, NOT those un-plumbed fields — do NOT conflate, §1.2/§11). Carries forward.
- **Per-fixture env injection beyond the one probed arm** — if the value-equality fixture (D-ENV-FIXTURE) surfaces harness env-threading edges (e.g. multiple env vars, per-side divergent values), only the single-var arm is implemented this row; edges carry forward.

After row 63 the `custom_tags` candidate NARROWS (literal + request_header + environment done; `metadata` remains) in the LIVE deferred sentence (at the IMPL); OTLP + the other tracing follow-ons remain ⇒ the sentinel check-(2) STILL prints ⇒ the loop continues.

---

## 9. Cross-references against prior phases' deferred-items lists — pickup

Phase 63 PICKS UP tracing `custom_tags` `environment` — recorded in the ROADMAP Observability family's LIVE deferred sentence (`OTLP-metrics stats sink + tracing custom_tags (environment/metadata)/spawn_upstream_span/http_service/force-trace`) and in ADR-0277/ADR-0283 §Consequences (the non-literal types). After phase 63 the remaining `custom_tags` candidate is `metadata` (the SOLE remaining reject); plus `spawn_upstream_span`/`http_service`/force-trace + OTLP-metrics + `max_path_tag_length` + `stats_flush_on_admin`. The family STAYS OPEN. **Sentinel maintenance (at the IMPL):** after NARROWING `custom_tags` in the deferred sentence, re-run the check-(2) grep — EXACTLY ONE live "candidates:" match with the intended content (`reference_sentinel_deferred_sentence_live_vs_historical`).

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution

- **D-ENV-SCOPE** — confirm the `environment`-added slice (`metadata` STAYS rejected loudly with its existing distinct substring, §2.7). §2.2.
- **D-ENV-WIRE** — how the reference emits an `environment` custom tag on the OTLP span: key = the `tag` name, value = the env value as a STRING `AnyValue`, appended after the built-ins (upsert-by-key). ONE fresh-container probe against `envoyproxy/envoy:contrib-v1.37.2` with a configured `environment` custom_tag + the env var set on the container (`-e` / testcontainers `Env`), observed via `test/helpers/otlptrace` (`reference_docker_probe_bridge_network`/`reference_host_gateway_ip_docker_desktop` for reachability). §2.3.
- **D-ENV-MISSING** — the reference's missing-env-var behavior: env present → value; absent + `default_value` set → default; absent + empty default → OMIT the tag (proto doc, §2.4 — anticipated IDENTICAL to `request_header`). Probe ALL THREE arms (fresh container each, `reference_probe_fresh_container_per_arm`). §2.4.
- **D-ENV-RESOLVE-TIME** — parse-time-static (Option A, `os.Getenv` at parse → `kindLiteral`/omit; LEAN) vs a `kindEnvironment` case in `ResolveCustomTags` (Option B). The CENTRAL design decision. §2.5.
- **D-ENV-DEDUP** — the first-wins dedup interaction when an `environment` spec OMITS (absent + empty default): does the reference still RESERVE the config-order key slot (blocking a later same-key tag) or does the omitted tag vanish (letting a later same-key tag win)? Probe (a `literal` + a same-key omitting-`environment`, BOTH config orders). Informs Option A-vs-B (§2.5). §2.4/§2.5.
- **D-ENV-PRECEDENCE** — upsert-by-key vs a colliding built-in AND config order across a cross-type same-key collision (an `environment` + a same-key `literal`/`request_header`, BOTH config orders). Anticipated IDENTICAL to the phase-62 findings (custom-overrides-built-in; custom-vs-custom same-key FIRST-in-config-order wins). Probe to CONFIRM parity. §2.3.
- **D-ENV-REJECT** — `metadata` reject substring UNCHANGED; the empty-`environment.name` structural reject (PGV `Environment.name` `min_len: 1` — CONFIRMED parity with `Header.name` at v1.37.0 this session; RE-DERIVE the exact `.pb.validate.go` line against the PROJECT-pinned go-control-plane version — envoy-go's own explicit reject [the phase-62 precedent] vs reliance on the reference PGV boot-reject); `Environment.default_value` unconstrained. Confirm (one probe arm) the reference ACCEPTS `environment` (the departure narrows to `metadata` only). §2.7.
- **D-ENV-FIXTURE** — ONE new OTLP fixture (`0106` anticipated; RE-DERIVE the number); value-equality (env injected into BOTH sides — D-ENV-HARNESS) vs key-presence (a naturally-present env var like `PATH`, no harness surgery — the phase-62 "present-case cross-side by key" precedent). SPEC weighs the harness cost against proof strength (LEAN value-equality if injection is one-line-per-starter). Default/omit/dedup arms as unit tests; Zipkin path unit-tested (the phase-59/62 precedent). Fixtures **107 → 108**. New dir (`reference_differential_fixture_dispatch_constraint`). §2.8.
- **D-ENV-HARNESS** — IF value-equality (D-ENV-FIXTURE): the env-injection surgery — testcontainers `ContainerRequest.Env` for the reference (`harness.go:124`) + `cmd.Env` for the subject subprocess (`harness.go:252`), threaded from the fixture. RE-DERIVE the two starter sites; confirm testcontainers v0.27.0 honors `Env` (unlike its dropped `Mounts` path — see `StartReferenceProxyWithMounts`'s `HostConfig.Binds` workaround). §2.8.
- **D-ENV-FUZZSEED** — a SEED to the EXISTING `FuzzHCMConfigParse` (NOT a new fuzzer); fuzzer count STAYS 55 (`reference_fuzzer_count_docs_drift` — reconcile before AND after). §2.9.
- **D-ENV-SPLIT** — the ADR-0045 disposition (SINGLE FLAT ROW anticipated, ~7–11 tasks — SMALLER than phase 62; escape-valve armable only if the env-injection harness surgery surprises upward). §1.4.

---

## 11. Prior-phase lessons applied

- **`feedback_brief_citations_not_evidence`** — EVERY `file:line` here (the `config.go` reject/parse/dedup sites, `resolve.go` `ResolveCustomTags`, the `accesslog_emit.go` THREE call sites, the `custom_tag.pb.go`/`.pb.validate.go` proto lines, the `harness.go` starter sites) is to be RE-DERIVED from source at the SPEC. This session RE-DERIVED them live against master `d14caa96` — notably CONFIRMING the three call sites already thread `ResolveCustomTags` (phase 62), so this row needs ZERO call-site change (§2.6), and that `CustomTag_Environment.name` carries the SAME PGV `min_len:1` as `Header.name` (§2.7).
- **`reference_tracing_custom_tag_override_builtin`** — the upsert-by-key seam + the phase-62 first-wins custom-vs-custom dedup are ALREADY landed; `environment` reuses both unchanged. The omit-at-parse dedup-slot question (D-ENV-DEDUP) is the one env-specific nuance. §2.4/§2.5.
- **`reference_spec_drafted_identifier_collision_check`** — IF Option B adds `kindEnvironment` (or an `EnvName` field), GREP `internal/tracing` for a colliding symbol before the PLAN adopts it (the phase-62 `kindLiteral`/`kindRequestHeader` naming dodged a `config_test.go` helper collision — check the same package). §2.5.
- **`reference_fuzzer_count_docs_drift`** — a SEED, not a fuzzer; reconcile the documented running total (55) against actual `^func Fuzz` before AND after — the count must NOT move. §2.9.
- **`reference_probe_fresh_container_per_arm`** + **`reference_envoy_contrib_image_tagging`** — each SPEC probe arm (D-ENV-WIRE / D-ENV-MISSING ×3 / D-ENV-DEDUP / D-ENV-PRECEDENCE / D-ENV-REJECT) runs on a FRESH container against `envoyproxy/envoy:contrib-v1.37.2`, injecting the env var via `-e`/testcontainers `Env`. §10.
- **`reference_docker_probe_bridge_network`** + **`reference_host_gateway_ip_docker_desktop`** — the OTLP span probes need a shared bridge network + a reachable receiver; verify the span decode ACTUALLY ran (not a vacuous empty capture). §10.
- **`reference_differential_fixture_dispatch_constraint`** — a new fixture dir per runner branch; do NOT mutate `0087`/`0088` (baselines) or `0102`/`0105` (the phase-59/62 custom-tag fixtures). §2.8.
- **`reference_differential_http_expectations_tcp_only`** — the OTLP fixture is H1/H2-over-TCP (NOT H3/QUIC), so the span assertion lives in the `otlptrace` receiver, not `HTTPExpectations`. §2.8.
- **`reference_tracing_upstream_cluster_framework_gap`** — the 4 EMPTY built-in span attributes are a KNOWN framework gap; an `environment` tag reads a process env var (available at parse/emit), NOT those un-plumbed upstream/node/zone/peer fields — INDEPENDENT of that gap (do NOT conflate). §1.2/§8.
- **`reference_sentinel_deferred_sentence_live_vs_historical`** — after the IMPL NARROWS `custom_tags` in the deferred sentence, re-run the check-(2) grep; EXACTLY ONE live "candidates:" match, correct content. §9.
- **`reference_strict_reject_sibling_typeurl_gap`** / **ADR-0080** — the `metadata` reject keeps its DISTINCT substring so a future silent divergence surfaces; lifting `environment` out of the reject roster is an explicit per-arm change (not a fall-through). §2.7.
- **`reference_fatalf_makes_assertions_unreachable`** — the config/resolve/fixture tests assert each independent property with `Errorf` (not `Fatalf`), so an `environment` failure does not mask the built-in / literal / request_header assertions. §6.

---

## 12. Section closeout

**Settled:** subject (tracing `custom_tags` `environment`, SELF-PICKED per the standing directive as the smallest CLEANLY-DIFFERENTIAL-PROVABLE remaining candidate — the immediate follow-on the phase-62 BRAINSTORM named — over `metadata` [needs a dynamic-metadata lookup path envoy-go lacks] and the larger declined alternatives, §2.1); scope (`environment` added; `metadata` rejects loudly with its distinct substring — the SOLE remaining `custom_tags` envoy-go-strict departure, §2.2/§2.7); the span-emit seam (UNCHANGED — resolve to `[]KV`, then the landed `upsertAttr` path covers OTLP + Zipkin, §2.3); the missing-env semantics (present→value / absent+default→default / absent+empty→OMIT — IDENTICAL to `request_header`, the omit logic ALREADY landed, §2.4); the value-source decision (parse-time-static Option A anticipated — env is process-static — vs a `kindEnvironment` case; the central and only design decision, D-ENV-RESOLVE-TIME, §2.5); the resolution seam + call sites (ALREADY THREADED at phase 62 — ZERO change, §2.6); fixture posture (ONE new OTLP fixture — value-equality via a small two-starter env-injection OR key-presence via a naturally-present env var, D-ENV-FIXTURE; default/omit/dedup unit-tested; Zipkin unit-tested, §2.8); fuzz posture (a SEED to `FuzzHCMConfigParse`, no new fuzzer, §2.9); stat surface (+0, §2.10); envelope (SINGLE FLAT ROW anticipated, ~7–11 tasks — ADR-0284, §1.4). The novel production code is just the `environment` parse arm + the value-source resolution; the row's ONE genuinely novel test-side piece is the cross-side env-var fixture (D-ENV-FIXTURE / D-ENV-HARNESS).

**Anticipated moves at the phase-63 IMPL (docs-only now):** the `environment` parse arm (lift `config.go:195-196`) + the value-source resolution (Option A parse-time-static anticipated) + config/resolve unit tests + a `FuzzHCMConfigParse` seed + the new OTLP fixture (+ conditional env-injection harness addition) + the BEHAVIOR_CONTRACT tracing edit + ADR-0284 + the ROADMAP deferred-sentence narrow. Counts: stat surface **1201 (+0)** · fixtures **107 → 108** · fuzzers **55 (+0, seed only)** · BackendKind **38 (+0)** · DECISIONS tail **ADR-0284** (next-free **ADR-0285**) · new Go packages **0** · new go.mod modules **0**.

**Counts UNCHANGED at this BRAINSTORM (docs-only; re-verified against master tip `d14caa96`):** stat surface **1201** · fixtures **107** · fuzzers **55** · BackendKind **38** · DECISIONS tail **ADR-0283** (next-free **ADR-0284**) · go.mod modules **2**. Row 63 registers `in-progress` at this BRAINSTORM commit per the §Schema invariant.

**Next → the phase-63 SPEC** (the D-ENV-* live-probe arms against `envoyproxy/envoy:contrib-v1.37.2` — D-ENV-WIRE / D-ENV-MISSING ×3 / D-ENV-DEDUP / D-ENV-PRECEDENCE / D-ENV-REJECT; re-derive every §6 edit site + the `custom_tag.pb.validate.go` PGV line against the project-pinned version + the `harness.go` starter sites; pin D-ENV-RESOLVE-TIME + D-ENV-FIXTURE + D-ENV-HARNESS; draft ADR-0284 §Context).
