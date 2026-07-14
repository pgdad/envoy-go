# SPEC 63 — tracing `custom_tags` `environment` SOURCE arm (the FOURTEENTH Observability-family row; the THIRD `custom_tag` source type after phase-59 `literal` + phase-62 `request_header`; a PROCESS-STATIC tag — lifts the `internal/tracing/config.go:195-196` `environment type unsupported` reject and reads a named PROCESS ENVIRONMENT VARIABLE into a `{tag, value}` STRING span attribute [with `default_value` / omit-on-missing / omit-on-empty]; `metadata` STAYS parse-rejected loudly — the SOLE remaining envoy-go-strict `custom_tags` DEPARTURE, ADR-0080; anticipated ONE new OTLP fixture / +0 stats / +0 packages / +0 modules — a SINGLE FLAT ROW, ADR-0284)

> **Stage:** SPEC (lifecycle-state 1 → 2). Docs-only; NO production `.go` changes at this stage. Fresh worktree `.worktrees/phase-63-spec`, branch `phase-63-tracing-environment-custom-tag-spec`, off master, per `feedback_git_worktrees`.
>
> **ANCHORS ADR-0284 §Context DRAFT** (§Decision/§Consequences land at the phase-63 IMPL per ADR-0044; DECISIONS tail STAYS **ADR-0283** at this SPEC).
>
> **Baselines re-verified against master tip `796f97e3` (the phase-63 BRAINSTORM squash):** stat surface **1201** · fixtures **107** (`ls -d test/fixtures/[0-9]*/ | wc -l`; tail `0105-tracing-custom-tags-request-header`) · fuzzers **55** (verified `55` actual `^func Fuzz`) · BackendKind tail **38** (`H2GoawayResponder`) · DECISIONS tail **ADR-0283** (next-free **ADR-0284**) · new Go packages **0** · go.mod modules **2** (`quic-go` direct + `qpack` indirect). Counts UNCHANGED at this SPEC (docs-only). Every `file:line` below was RE-DERIVED from source this session (`feedback_brief_citations_not_evidence`) — the roster is §12.

---

## 1. Purpose / Mission

Phase 63 lifts the one `environment` reject (`internal/tracing/config.go:195-196`) and supports the **`environment`** `CustomTag` type: an operator configuring `custom_tags: [{tag: "region", environment: {name: "ENVOY_REGION", default_value: "unknown"}}]` on the HCM `tracing` block gets a `{key: "region", value: <the ENVOY_REGION process env-var value>}` STRING attribute on every sampled ingress SERVER span — on **both** the OTLP and the Zipkin exporter (the resolved tag flows through the shared `Span.Attrs []KV` / `upsertAttr` seam landed at phase 59, threaded through the per-request `ResolveCustomTags` seam landed at phase 62). This is the THIRD custom-tag source type and — unlike `request_header` — its value is STATIC for the process lifetime (read from a named PROCESS ENVIRONMENT VARIABLE, not a per-request field). The last remaining source type (`metadata`) STAYS **PARSE-REJECTED loudly** with its existing distinct substring (envoy-go-strict DEPARTURE — the reference SUPPORTS it; §11 arms confirm the reference ACCEPTS `environment`, so lifting it genuinely NARROWS the departure to `metadata` alone). A NEW PGV-parity structural reject (empty `environment.name`) mirrors the reference's boot-reject (both reject — NOT a departure; §11 arm E).

ADR-0284 §Context is DRAFTED here (§13); §Decision/§Consequences at the IMPL (ADR-0044). **All BRAINSTORM D-ENV-* questions are DISPOSED at this SPEC** — the empirical arms via LIVE probes against `envoyproxy/envoy:contrib-v1.37.2` (§11, FRESH container per arm, `reference_probe_fresh_container_per_arm`):

- **D-ENV-SCOPE** — DISPOSED: `environment` added; `metadata` rejects loudly (the SOLE remaining `custom_tags` departure) (§2, §6).
- **D-ENV-WIRE** — PINNED (§11 arm A): `{key: <tag>, value: {stringValue: <env value>}}` — a STRING attribute, upserted against the built-ins (identical wire shape to `literal`/`request_header`).
- **D-ENV-MISSING** — PINNED (§11 arm A, three sub-arms): env present → the env value; env absent + `default_value` set → the default; env absent + `default_value` empty/unset → **the tag is OMITTED entirely** (the same three-arm shape phase 62 landed for `request_header`; the omit logic ALREADY exists).
- **D-ENV-EMPTYVAL** — PINNED (§11 arm G — a NEW finding the BRAINSTORM did not anticipate): an env var **present but set to the empty string** (`-e NAME=`) → **the tag is OMITTED** (NOT emitted as `""`, NOT the default). This unifies the missing/empty behavior into a single rule — **the tag is omitted iff its RESOLVED value is empty**, where `resolved = (env present ? envValue : defaultValue)` — and REQUIRES `os.LookupEnv` (present-ness), not `os.Getenv`, so present-empty (omit) is distinguished from absent-with-default (emit default). This DIVERGES from phase-62's request_header modeled empty-value edge (which emits `""`), a probe-justified difference recorded here.
- **D-ENV-PRECEDENCE** — PINNED (§11 arms B/C/D), IDENTICAL to the phase-62 findings: (a) an `environment` tag colliding with a BUILT-IN key OVERRIDES it (upsert, arm B); (b) among CUSTOM tags with the SAME key, the **FIRST in config order WINS** (arms C/D) — the reference's config-time map-emplace, regardless of source type.
- **D-ENV-DEDUP** — PINNED (§11 arm F — the load-bearing finding): an `environment` tag that RESOLVES TO NOTHING (absent + no default) but is FIRST in config order STILL **RESERVES its config-order key slot**, suppressing a later same-key tag (the reference dedups by key at config-LOAD, BEFORE/independent of value resolution). ⇒ the resolution MUST be modeled as two-phase (dedup-by-key at parse over stored specs; resolve/omit AFTER), which DECIDES D-ENV-RESOLVE-TIME toward Option B (§1.1, §3.3).
- **D-ENV-RESOLVE-TIME** — DECIDED (§1.1, §3.3): the CENTRAL design decision. The BRAINSTORM anticipated Option A (parse-time-static: `os.Getenv` at parse → a `kindLiteral` spec / omit). The D-ENV-DEDUP + D-ENV-EMPTYVAL probes **FLIP it to Option B** (a `kindEnvironment` spec stored at parse, resolved by `ResolveCustomTags` per-span via `os.LookupEnv`): storing the spec reserves the dedup slot NATURALLY through the existing append+`seen` path (arm F), and the omit-on-empty-resolved logic mirrors the landed `kindRequestHeader` arm. Option A would require a special-case slot-reservation control-flow wart to reproduce arm F, and would collapse the reference's two-phase (dedup-then-resolve) model into parse.
- **D-ENV-REJECT** — PINNED (§6, §11 arm E + PGV re-derivation): the `metadata` reject substring UNCHANGED; empty `environment.name` → a PGV-parity structural reject (the reference boot-rejects, §11 arm E); the reference ACCEPTS `environment` (arms A–D/F/G all booted + emitted) ⇒ the departure genuinely NARROWS to `metadata` alone.
- **D-ENV-FIXTURE** — DECIDED (§8): ONE new OTLP fixture `0106-tracing-custom-tags-environment` (fixtures **107 → 108**), KEY-PRESENCE strategy via a NATURALLY-PRESENT env var (`PATH`) — ZERO harness surgery, SINGLE FLAT ROW. Value-equality (env injected into BOTH sides) + the D-ENV-HARNESS injection surgery are WEIGHED and DEFERRED (§8). The value-resolution edges (present/default/omit/empty-omit/dedup) are UNIT tests; the Zipkin encoder path is a unit test (the phase-59/62 precedent).
- **D-ENV-HARNESS** — DEFERRED (§8): the value-equality env-injection surgery (testcontainers `ContainerRequest.Env` at `harness.go:124` + `cmd.Env` at `harness.go:252`, threaded per-fixture through the runner) is NOT one-line-per-starter — it needs a per-fixture env-declaration seam threaded through both starters. Deferred in favor of key-presence to keep the row a SINGLE FLAT ROW.
- **D-ENV-FUZZSEED** — DECIDED (§6): a seed to the existing `FuzzHCMConfigParse`; fuzzer count STAYS **55**.
- **D-ENV-SPLIT** — DECIDED (§3.0): a SINGLE FLAT ROW (~8–10 tasks), ADR-0045 escape-valve UNCONSUMED.
- **D-ENV-DOCSHAPE** — the full RE-DERIVED edit-site roster (§12).

No PLAN-time empirical question remains; the PLAN is a mechanical TDD decomposition.

### 1.1 Empirical-finding-driven refinement (per ADR-0044): the value-source decision FLIPS to Option B

The BRAINSTORM (§2.5) LEANED **Option A** (parse-time-static — `os.Getenv` at `parseCustomTags`, collapsing an `environment` tag to a `kindLiteral` spec or omitting it; ZERO new kind, ZERO `ResolveCustomTags` change) "UNLESS the D-ENV-DEDUP probe shows the omit-at-parse dedup interaction is cleaner under Option B's parse-time-reserved slot." **The SPEC-63 probes show exactly that**, on TWO fronts:

1. **D-ENV-DEDUP (§11 arm F).** An `environment` tag that resolves to NOTHING (env unset, no default), placed FIRST in config order before a same-key `literal`, still SUPPRESSES the literal — the captured span carries NO `dup` attribute at all. So the reference reserves the config-order key slot at config-LOAD (map-emplace, insert-if-absent), INDEPENDENT of whether the value later resolves to something. To reproduce this under Option A (resolve-at-parse), an omitting `environment` tag would have to record its key in `seen` WITHOUT appending a spec — a special-case reserve-and-`continue` branch that bypasses the normal dedup+append path (`config.go:202-206`). Under Option B (store a `kindEnvironment` spec, resolve later), the spec is appended through the EXISTING path, so the slot is reserved NATURALLY and the omit happens at resolve time.

2. **D-ENV-EMPTYVAL (§11 arm G).** An env var PRESENT but set to the empty string resolves to an empty value and the tag is OMITTED (not `""`, not the default). This means MORE cases omit than the BRAINSTORM modeled (not just absent-no-default, but also present-empty and empty-default), each of which — under Option A — would need the same reserve-and-`continue` wart to stay dedup-correct. Under Option B the resolver simply appends nothing for an empty resolved value.

Together these DECIDE **Option B**: a `kindEnvironment` `CustomTagSpec` stored at parse (dedup by key first-wins, reserving the slot), resolved per-span in `ResolveCustomTags` via `os.LookupEnv` with the unified **omit-iff-resolved-value-empty** rule. This mirrors the reference's two-phase (dedup-at-config-load, resolve-per-span) model exactly and reuses the structure of the landed `kindRequestHeader` arm; the redundant per-span `os.LookupEnv` is a cheap map read and observationally identical (the env is process-static). This is the ONE anticipation the probes overturned; every other BRAINSTORM decision held. (Cf. the phase-62 §1.1 refinement, which flipped the anticipated last-wins dedup to first-wins on the same custom-tag engine.)

---

## 2. Non-purposes (deferred; per BRAINSTORM §1.2 + §8)

NO `metadata` custom-tag type (rejected loudly, §6 — needs a dynamic-metadata lookup path envoy-go lacks; the LAST and biggest of the four `custom_tag` source types; after phase 63 it is the SOLE remaining `custom_tags` reject). NO `spawn_upstream_span`/`http_service`/force-trace. NO `max_path_tag_length` (a distinct, still-rejected knob, `config.go:110-111`). NO OTLP-metrics stats sink. NO new provider, transport, or stat. The four built-in span attributes emitted EMPTY by envoy-go (`upstream_cluster`/`node_id`/`zone`/`peer.address`, the `reference_tracing_upstream_cluster_framework_gap` framework gap) are UNTOUCHED — an `environment` tag reads a PROCESS ENV VAR (fully available at the seam via `os.LookupEnv`), NOT those un-plumbed upstream/node/zone/peer per-request fields; do NOT conflate. **Value-equality differential + the D-ENV-HARNESS env-injection surgery** are weighed and DEFERRED (§8) — the fixture proves cross-side EMISSION by key-presence; the value-resolution semantics are unit-tested.

---

## 3. The change — lift the `environment` reject, a `kindEnvironment` spec + resolver case, ZERO call-site change (ADR-0284)

### 3.0 Split disposition — a SINGLE FLAT ROW; the ADR-0045 escape-valve UNCONSUMED

Anticipated **~8–10 tasks** (§10), UNDER the ADR-0045 `~15` ceiling and SMALLER than phase 62's 9 — the per-request resolution seam + the three call-site threadings are ALREADY LANDED (phase 62), so this row adds only the `environment` parse arm + the `kindEnvironment` resolver case + config/resolve unit tests + a fuzz seed + one fixture + docs. There is no second subsystem to strand. The escape-valve is documented ARMABLE but UNCONSUMED — the key-presence fixture (§8) needs ZERO harness surgery, so the fixture leg does not become its own row (the D-ENV-HARNESS value-equality path, which COULD have, is deferred).

### 3.1 The parse arm — lift the `environment` reject in `parseCustomTags` (`internal/tracing/config.go`)

`parseCustomTags` (`config.go:169`) currently returns `([]CustomTagSpec, error)` (the phase-62 model) and rejects `environment` at `config.go:195-196`. Replace that reject arm with an ACCEPT arm (the `literal`/`request_header` arms, the empty-`tag` guard at `config.go:177-179`, and the first-wins dedup at `config.go:202-206` are UNCHANGED):

- `ct.GetEnvironment() != nil` → **ACCEPT (NEW)**: read `e := ct.GetEnvironment()`; reject an empty `e.GetName()` as `tracing: custom_tags environment tag %q empty name` (PGV-parity — `custom_tag.pb.validate.go:468`, §6); else append `CustomTagSpec{Key: tag, Kind: kindEnvironment, EnvName: e.GetName(), DefaultValue: e.GetDefaultValue()}`.
- `ct.GetMetadata() != nil` → reject `tracing: custom_tags metadata type unsupported` (DEPARTURE, unchanged).
- `literal` / `request_header` / empty-tag / typeless arms → UNCHANGED.

**First-wins dedup (§1.1 point 1, D-ENV-DEDUP).** The `seen map[string]struct{}` at `config.go:202-206` reserves the FIRST occurrence of each key regardless of source type; the stored `kindEnvironment` spec occupies its slot NATURALLY (arm F). Dedup runs AFTER per-tag structural validation, so a later same-key tag with an invalid name still boot-rejects (parity with the reference PGV). NO change to the dedup block — the `environment` accept arm just appends a `kindEnvironment` spec like the others.

### 3.2 The config model — `CustomTagSpec` gains `kindEnvironment` + an `EnvName` field (D-ENV-RESOLVE-TIME, Option B)

Extend the phase-62 `customTagKind` and `CustomTagSpec` (`config.go:42-63`):

```go
const (
	kindLiteral customTagKind = iota
	kindRequestHeader
	kindEnvironment // NEW — reads a named process env var (os.LookupEnv), omit-on-empty-resolved
)

type CustomTagSpec struct {
	Key          string
	Kind         customTagKind // kindLiteral | kindRequestHeader | kindEnvironment
	LiteralValue string        // Kind==kindLiteral
	HeaderName   string        // Kind==kindRequestHeader
	EnvName      string        // Kind==kindEnvironment: the env var to read   // NEW
	DefaultValue string        // Kind==kindRequestHeader | kindEnvironment: value when the source is absent
	HasDefault   bool          // Kind==kindRequestHeader only (kindEnvironment uses the omit-iff-empty rule, §3.3)
}
```

`kindEnvironment` and `EnvName` are NEW symbols — GREP-confirmed collision-free in `internal/tracing` this session (`reference_spec_drafted_identifier_collision_check`: no existing `kindEnvironment`/`EnvName` in the package; the only `environment`/`Environment` occurrences are the reject arm at `config.go:195-196` + the config_test row at `config_test.go:463-465`, both edited by this row). `DefaultValue` is REUSED (same semantics: value when the source is absent). `kindEnvironment` does NOT consult `HasDefault` — it uses the unified omit-iff-resolved-empty rule (§3.3), so `HasDefault` need not be set for an environment spec (the field stays `kindRequestHeader`-only; documented on the struct).

### 3.3 The per-request resolver — ADD a `kindEnvironment` case to `ResolveCustomTags` (D-ENV-RESOLVE-TIME; `internal/tracing/resolve.go`)

Add ONE case to the existing `ResolveCustomTags` switch (`resolve.go:18`), reading `os` (a NEW stdlib import in `resolve.go`):

```go
case kindEnvironment:
	// The env is process-STATIC; os.LookupEnv reports present-ness so a present-
	// but-EMPTY var ("") is distinguished from an ABSENT one (D-ENV-EMPTYVAL,
	// SPEC §11 arm G). Resolved value = env value if present, else the default.
	// The tag is OMITTED iff the resolved value is empty — a present-empty var,
	// an absent var with no default, and an absent var with an empty default all
	// omit (arms A/F/G); only a NON-EMPTY resolved value emits.
	v, present := os.LookupEnv(s.EnvName)
	if !present {
		v = s.DefaultValue
	}
	if v != "" {
		out = append(out, KV{Key: s.Key, Str: v})
	}
```

This reproduces every §11 environment arm: `e_present`→env value (arm A); `e_missdef`→default (arm A); `e_missnodef`→omit (arm A); `e_emptyset`→omit (arm G); and a first-wins environment spec that omits still reserves its slot because the SPEC is in the deduped list (arm F). `headerLookup` is IGNORED by this arm (an env tag needs no request header) — so the `ResolveCustomTags` signature is UNCHANGED and the three call sites (§3.4) need NO change. The `kindLiteral`/`kindRequestHeader` arms are UNCHANGED (the request_header present-empty edge STILL emits `""` per phase-62's documented model — the environment arm's omit-on-empty is a probe-justified DIVERGENCE from it, §1.1 point 2).

### 3.4 The call-site threading — UNCHANGED (D-ENV-RESOLVE-TIME payoff)

The THREE `accesslog_emit.go` `BuildServerSpan` call sites — `:55` (H1), `:116` (H2), `:177` (H3) — ALREADY call `tracing.ResolveCustomTags(f.tracingConfig.CustomTags, reqHeaderLookupH1(r)/reqHeaderLookupH2(req))` (landed at phase 62). A `kindEnvironment` spec flows through UNCHANGED — its resolver arm ignores the header lookup and reads `os.LookupEnv`. **NO call-site signature change, NO new lookup, NO `BuildServerSpan` change.** This is the concrete payoff of phase 62 having built the general resolution seam (§1 / BRAINSTORM §2.6).

### 3.5 Precedence — `BuildServerSpan` UNCHANGED (upsert vs built-in only)

`BuildServerSpan` (`span.go`) and `upsertAttr` are UNCHANGED. The resolver hands a unique-key `[]KV` (deduped at parse); each is upserted after the built-ins, so an `environment` tag whose key collides with a built-in OVERRIDES it (arm B — the landed behavior). Custom-vs-custom collisions never reach `upsertAttr` (deduped at parse), so its last-wins branch only ever overrides built-ins (== the reference, arms C/D). No change to `span.go`.

### 3.6 Byte-stability — no behavior change on the existing paths

A tracing HCM with no `custom_tags` parses to `CustomTags == nil`; the resolver returns `nil`; every existing span is byte-identical. A single `literal` custom tag (the `0102` fixture) or a single `request_header` tag (the `0105` fixture) parses/resolves EXACTLY as today — those differentials stay byte-stable. The 107-dir differential is unaffected except the new `0106` dir.

---

## 4. Framework primitives — 0 new packages, 0 new go.mod modules

All edits land in `internal/tracing` (`config.go` + `resolve.go`, both existing) + `internal/filter/hcm` (fuzz seed only) + `test/fixtures` + `docs/`. The `CustomTag_Environment` proto is reachable via the ALREADY-imported `github.com/envoyproxy/go-control-plane/envoy v1.32.4` module (`config.go` already imports `tracingv3`, phase 59). `ct.GetEnvironment()` returns `*tracingv3.CustomTag_Environment` with `GetName()`/`GetDefaultValue()` (§5). `os.LookupEnv` is stdlib. NO new package, NO new module, NO new interface. `go mod tidy -diff` anticipated EMPTY (modules STAY **2**).

---

## 5. Proto-field roster — `type.tracing.v3.CustomTag_Environment` (RE-DERIVED @ go-control-plane/envoy v1.32.4, `type/tracing/v3/custom_tag.pb.go` + `.pb.validate.go`)

| Field | Getter | Phase-63 disposition |
|---|---|---|
| `CustomTag.tag` (1, string) | `GetTag()` | the attribute KEY; empty ⇒ reject (unchanged) |
| `CustomTag.environment` (3, `CustomTag_Environment`) | `GetEnvironment()` `:99` | **ACCEPT (NEW)** ⇒ resolve via `os.LookupEnv` |
| `CustomTag_Environment.name` (1, string) | `GetName()` `:247` | the env var to read; empty ⇒ reject (PGV-parity, `.pb.validate.go:468`) |
| `CustomTag_Environment.default_value` (2, string) | `GetDefaultValue()` `:254` | value on absent env; UNCONSTRAINED (empty valid ⇒ omit-on-missing) |
| `CustomTag.literal` (2) | `GetLiteral()` | ACCEPT (phase 59, unchanged) |
| `CustomTag.request_header` (4) | `GetRequestHeader()` | ACCEPT (phase 62, unchanged) |
| `CustomTag.metadata` (5) | `GetMetadata()` | reject (DEPARTURE, unchanged — the SOLE remaining) |

**Proto doc (authoritative missing semantics, `custom_tag.pb.go:207-212`).** `name`: *"Environment variable name to obtain the value to populate the tag value."* `default_value`: *"When the environment variable is not found, the tag value will be populated with this default value if specified, otherwise no tag will be populated."* — the same three-arm shape as `request_header`, CONFIRMED + REFINED by the empty-value omit (§11 arm G).

---

## 6. PARSE-REJECT roster + fuzzer

**Reject taxonomy** (all ADR-0080-distinct substrings):

**Tier A — PGV-parity structural rejects (BOTH reject; NOT a departure).** envoy-go mirrors these explicitly (no PGV at runtime):
- empty `tag` → `tracing: custom_tags empty tag` (unchanged).
- empty `literal.value` → `tracing: custom_tags literal tag %q empty value` (unchanged).
- empty `request_header.name` → `tracing: custom_tags request_header tag %q empty name` (unchanged, phase 62).
- typeless tag → `tracing: custom_tags tag %q missing type` (unchanged).
- **empty `environment.name` → `tracing: custom_tags environment tag %q empty name` (NEW)** — ref PGV `custom_tag.pb.validate.go:468` (`Environment.name` `min_len:1`, `value length must be at least 1 runes`), CONFIRMED LIVE (§11 arm E: `EnvironmentValidationError.Name: value length must be at least 1 characters`). *(Unlike `Header.name`, `Environment.name` carries NO control-char pattern constraint in PGV — only `min_len:1` — so envoy-go's `min_len` mirror is EXACT parity, not a narrowing.)*

**Tier B — envoy-go-strict DEPARTURE (the reference ACCEPTS — §11 arms; envoy-go rejects):**
- `metadata` → `tracing: custom_tags metadata type unsupported` (unchanged) — **the SOLE remaining `custom_tags` departure after phase 63.**

The `environment` reject (`config.go:195-196`) is REMOVED (the arm now parses). §11 arms A–D/F/G confirm the reference ACCEPTS `environment` (booted + emitted), so lifting it genuinely narrows the departure.

**Fuzzer (D-ENV-FUZZSEED).** The custom_tags parse is reached by `FuzzHCMConfigParse` (`internal/filter/hcm/fuzz_test.go`) — the phase-59/62 host. Add ONE seed: a `Tracing` block with an `environment` custom_tag (name + default) + a mixed `literal`+`environment`+`request_header` config, via the existing `mkHCM` helper. **This is a SEED, not a new `func Fuzz` — fuzzer count STAYS 55** (`reference_fuzzer_count_docs_drift`: reconcile actual `^func Fuzz` = 55 before AND after).

---

## 7. Stat surface — +0 (1201 → 1201)

A span attribute is emitted on the wire, not registered as a stat. The HCM tracing decision counters + the tracer counters are UNCHANGED. Stat surface **1201 (+0)**.

---

## 8. Differential fixture taxonomy — +1 (D-ENV-FIXTURE: ONE new OTLP fixture, KEY-PRESENCE)

Per the dispatch constraint (`reference_differential_fixture_dispatch_constraint`) the `0087`/`0088`/`0102`/`0105` fixtures are NOT mutated. **ONE new dir `test/fixtures/0106-tracing-custom-tags-environment`** (fixtures **107 → 108**; RE-DERIVE the next-free number at the IMPL — `0105` is the current tail, `0106` anticipated):

- Cloned from `0105-tracing-custom-tags-request-header` (OTLP provider, `test/helpers/otlptrace` receiver, `host.docker.internal` STRICT_DNS per ADR-0010).
- The HCM `tracing` block: `custom_tags: [{tag: "env_path", environment: {name: "PATH"}}]` — a **NON-colliding** key reading `PATH`, an env var NATURALLY present + non-empty in BOTH the reference Docker container AND the subject Go subprocess (which inherits `os.Environ()`).
- **KEY-PRESENCE assertion (D-ENV-FIXTURE decision).** The driver drives ONE plain GET (no request-header manipulation needed — `environment` reads the process env) and asserts (by KEY, attribute order non-deterministic — §11) that the captured span carries an attribute with key `env_path` **and a non-empty string value** on BOTH the reference AND subject side. This proves cross-side that both proxies EMIT an `environment` custom tag resolved from a real process env var. The VALUE differs per side (container `PATH` ≠ subject `PATH`), so the assertion is key-present + value-non-empty, NOT value-equality (the phase-58/62 "present-case cross-side by key" precedent). Assert each independent property with `Errorf`, NOT `Fatalf` (`reference_fatalf_makes_assertions_unreachable`). `BackendCount` ≥ 1 (`reference_differential_backendcount_min_one`).
- Prove the new assertion LIVE with a deliberate `-count=1` break (`reference_differential_break_protocol_count1`), confirming WHICH assertion fires (`reference_deliberate_break_wrong_assertion`).

**Why key-presence, not value-equality (D-ENV-HARNESS deferred).** Value-equality (assert the SAME injected env value cross-side) would need a matching env var in BOTH the reference container (testcontainers `ContainerRequest.Env`, `harness.go:124`) AND the subject subprocess (`cmd.Env`, `harness.go:252`) — but neither starter is currently parameterized per-fixture, so it needs a per-fixture env-declaration seam threaded through the runner into both starters (a multi-site harness change, NOT one-line-per-starter as the BRAINSTORM hoped). To keep the row a SINGLE FLAT ROW (§3.0), the SPEC pins key-presence via a naturally-present env var (ZERO surgery) and DEFERS the value-equality/injection path (D-ENV-HARNESS). The value-resolution semantics — the interesting part (present→value / absent+default→default / absent→omit / present-empty→omit / dedup-slot-reservation) — are FULLY + DETERMINISTICALLY covered by UNIT tests on `ResolveCustomTags` (§10), needing no differential. **The Zipkin encoder path** (a resolved environment tag surfacing in the Zipkin `tags` map) is a UNIT test (the phase-59/62 precedent — one OTLP fixture + a Zipkin unit test). Fixtures **107 → 108** (NOT 109).

**Harness note:** the OTLP fixture drives H1/H2 over TCP — NOT the H3/QUIC path — so `reference_differential_http_expectations_tcp_only` does not bite; the span assertion lives in the `otlptrace` receiver, not `HTTPExpectations`.

---

## 9. Behavior-contract delta (`docs/envoy-go/BEHAVIOR_CONTRACT.md`; ADR-0284 atomic landing at the IMPL)

- The tracing `custom_tags` clause (RE-DERIVE the exact lines at the IMPL) — flip `environment` from "STRICT-REJECT (envoy-go-strict departure)" to "CONSUMED (the named process env var's value is emitted as a `{tag, value}` STRING span attribute on both exporters; `default_value` on an absent env var; OMITTED when the RESOLVED value is empty — a present-empty var, an absent var with no/empty default; resolved per-span via `os.LookupEnv`; FIRST-wins dedup on a duplicate tag key incl. an omitting env tag reserving its slot; OVERRIDES a colliding built-in)"; `metadata` STAYS STRICT-REJECT (the SOLE remaining `custom_tags` departure); ADD the empty-`environment.name` PGV-parity reject.

(Exact final wording RE-DERIVED and written at the IMPL.)

---

## 10. Test plan + per-task structure (~8–10 tasks; PLAN decomposes)

TDD (`superpowers:test-driven-development`); each task a red→green with a `-count=1` liveness break where an assertion is load-bearing. Anticipated tasks:

1. **Config model + parse arm** — add `kindEnvironment` + the `EnvName` field (§3.2); replace the `environment` reject with the ACCEPT arm + the empty-name reject (§3.1); the first-wins dedup is UNCHANGED (the env spec appends like the others). `config_test.go`: CHANGE the `environment` row (`config_test.go:463-465`) from `wantSub: "environment type unsupported"` to an ACCEPT asserting the parsed `kindEnvironment` spec (name + default); ADD an empty-`environment.name` reject row; the `metadata` row (`:468-470`) STAYS a reject; a first-wins dedup row with an environment spec (incl. the omit-first-then-literal slot-reservation, arm F). `-count=1` break per new reject / the dedup assertion.
2. **`ResolveCustomTags` `kindEnvironment` case** — the resolver arm (§3.3) in `resolve.go` (+ `os` import). `resolve_test.go`: env present → value; absent + default → default; absent + no default → omit; **present-empty → omit** (arm G, `t.Setenv` a var to `""`); a first-wins deduped mixed spec list resolves in order; upsert precedence vs a built-in (arm B). Use `t.Setenv` for hermetic env control.
3. **Zipkin encoder unit test** — a span with a resolved environment tag encodes into the Zipkin `tags` map (mirror the phase-59/62 arm).
4. **Call-site coverage** — NO `accesslog_emit.go` change (§3.4); a new HCM span test asserting an environment tag reaches the span through the already-threaded call sites (or confirm the existing tests cover it).
5. **New OTLP fixture `0106-tracing-custom-tags-environment`** — envoy.yaml, envoy-go.yaml, expectations.yaml, driver (plain GET, key-presence + value-non-empty assertion on `env_path`), README (§8); the environment span assertion proven live with a `-count=1` break.
6. **`FuzzHCMConfigParse` seed** — one environment custom_tags seed (§6); reconcile fuzzer count = 55.
7. **BEHAVIOR_CONTRACT edits** (§9).
8. **Verify** — six-gate (gofmt / golangci-lint / go vet / build / `go mod tidy -diff` / full package `-race` on `internal/tracing` + `internal/filter/hcm`) + the full 108-dir differential (byte-stable except `0106`).
9. **ADR-0284 body** (§Decision/§Consequences) + **STATE** + **ROADMAP** (row 63 `done` per ADR-0106; the LIVE deferred sentence NARROWS `custom_tags (environment/metadata)` → `(metadata)` — re-run the check-(2) grep, EXACTLY ONE live "candidates:" match, `reference_sentinel_deferred_sentence_live_vs_historical`) + **router roll**.

(Tasks 1–2 are the TDD core; the PLAN may split/merge. Total ~8–10, single flat row.)

---

## 11. SPEC-time empirical-pin block (D-ENV-* live probes — executed IN-SESSION 2026-07-14, `envoyproxy/envoy:contrib-v1.37.2`, FRESH container per arm)

Each arm ran a fresh `envoyproxy/envoy:contrib-v1.37.2` container (`--add-host host.docker.internal:host-gateway`; a host-bound `test/helpers/otlptrace` receiver + a trivial host HTTP backend), injected arm-specific env vars via `docker run -e`, drove ONE `GET /probe/path?q=1`, and captured the OTLP span. Decode VERIFIED non-vacuous (span count = 1, built-ins present) on every boot arm. Arm F + arm G were re-run and reproduced identically.

**Arm A (D-ENV-WIRE / D-ENV-MISSING).** Config: three `environment` tags — `e_present{name:ENVOY_PRESENT, default_value:def-present}`, `e_missdef{name:ENVOY_MISSING, default_value:def-missing}`, `e_missnodef{name:ENVOY_ABSENT}` (no default). Env injected: `ENVOY_PRESENT=PRESENT-VAL` (ENVOY_MISSING/ENVOY_ABSENT NOT set). Captured OTLP span attributes:
```
e_present   = STRING("PRESENT-VAL")   # env present            → the env value
e_missdef   = STRING("def-missing")   # env absent + default   → the default
e_missnodef = <OMITTED>               # env absent + no default → the tag is OMITTED
```
⇒ D-ENV-WIRE: `{key:<tag>, value:{stringValue:<env value>}}`. D-ENV-MISSING: present→value / absent+default→default / absent+no-default→OMIT.

**Arm B (D-ENV-PRECEDENCE — custom overrides built-in).** Config: `{tag: http.method, environment:{name:ENVOY_OVR}}`; injected `ENVOY_OVR=OVERRIDE-METHOD`. Captured: `http.method = STRING("OVERRIDE-METHOD")` — the built-in `"GET"` ABSENT. ⇒ an `environment` tag OVERRIDES a colliding built-in (upsert, same as `literal`/`request_header`).

**Arm C (D-ENV-PRECEDENCE — custom-vs-custom, config order).** Config: `[{tag:dup, literal:{value:LIT-VAL}}, {tag:dup, environment:{name:ENVOY_DUP}}]`; injected `ENVOY_DUP=ENV-VAL`. Captured: `dup = STRING("LIT-VAL")` — the FIRST (literal) tag won; the second (environment) DROPPED.

**Arm D (D-ENV-PRECEDENCE — custom-vs-custom, reversed order).** Config: `[{tag:dup, environment:{name:ENVOY_DUP}}, {tag:dup, literal:{value:LIT-VAL}}]`; injected `ENVOY_DUP=ENV-VAL`. Captured: `dup = STRING("ENV-VAL")` — the FIRST (environment) tag won; the second (literal) DROPPED.

⇒ Arms C/D: the reference deduplicates `custom_tags` by tag key keeping the **FIRST occurrence in config order** (Envoy's config-time map-emplace), regardless of source type — IDENTICAL to the phase-62 finding (§3.1/§3.5).

**Arm F (D-ENV-DEDUP — an omitting env tag RESERVES its config-order slot).** Config: `[{tag:dup, environment:{name:ENVOY_UNSET_XYZ}}, {tag:dup, literal:{value:LIT-VAL}}]`; `ENVOY_UNSET_XYZ` NOT set (⇒ the environment tag resolves to nothing). Captured: **NO `dup` attribute at all** (only the built-ins + `http.method=GET`). ⇒ the FIRST-in-order `environment` tag reserved the `dup` key slot at config-LOAD (suppressing the later `literal`), then resolved to nothing ⇒ `dup` OMITTED entirely. This is the LOAD-BEARING finding driving Option B (§1.1 point 1): the reference dedups by key BEFORE/independent of value resolution, so the resolution must be two-phase (dedup-at-parse over stored specs; resolve/omit after). (Re-run: identical.)

**Arm G (D-ENV-EMPTYVAL — present-but-empty env var omits; NEW finding).** Config: `{tag: e_emptyset, environment:{name:ENVOY_EMPTY, default_value:def-empty}}`; injected `ENVOY_EMPTY=` (present, empty string). Captured: **NO `e_emptyset` attribute** — NOT `STRING("")`, NOT the default `STRING("def-empty")`. ⇒ a PRESENT-but-EMPTY env var resolves to the empty env value (present-ness is honored — the default is NOT used), and an empty resolved value is OMITTED. Combined with arms A/F this yields the unified rule **omit iff the resolved value is empty**, where `resolved = (env present ? envValue : defaultValue)` — REQUIRING `os.LookupEnv` (present-ness) to distinguish present-empty (omit) from absent-with-default (emit default). This DIVERGES from phase-62's request_header modeled empty-value edge (emit `""`); recorded as a probe-justified difference (§1.1 point 2, §3.3). (Re-run: identical.)

**Arm E (D-ENV-REJECT — empty `environment.name` boot-rejects).** Config: `{tag: t_bad, environment:{name:""}}`, run via `envoy --mode validate`. The reference REJECTED at config init:
```
HttpConnectionManagerValidationError.Tracing ... caused by
CustomTagValidationError.Environment ... caused by
EnvironmentValidationError.Name: value length must be at least 1 characters
```
⇒ the reference PGV boot-rejects an empty `environment.name`; envoy-go mirrors it as a Tier-A PGV-parity reject (§6). Arms A–D/F/G all BOOTED and emitted ⇒ the reference ACCEPTS `environment` (the departure genuinely narrows to `metadata`).

**PGV structural rule (RE-DERIVED @ `type/tracing/v3/custom_tag.pb.validate.go`, go-control-plane/envoy v1.32.4).** `:468` `utf8.RuneCountInString(m.GetName()) < 1` (`Environment.name` `min_len:1`, `value length must be at least 1 runes`). NO `Environment.name` pattern constraint (contrast `Header.name` `:583`+`:594` which ALSO carries a `^[^\x00\n\r]*$` pattern). `default_value` UNCONSTRAINED (no PGV rule).

*(Probe harness: a throwaway `probe63/` Go program reusing the `otlptrace` receiver + a `docker run` CLI loop, one fresh `--rm` container per arm; convergence polled BEFORE container stop (OTLP flush is async); DELETED after — NOT committed, this SPEC is docs-only.)*

---

## 12. Edit-site roster (D-ENV-DOCSHAPE — RE-DERIVED against master `796f97e3`)

**Production — `internal/tracing/config.go`:**
- `config.go:42-63` `customTagKind` + `CustomTagSpec` — ADD `kindEnvironment` const + the `EnvName` field (§3.2). [EDIT/ADD]
- `config.go:169-209` `parseCustomTags` — replace the `environment` reject (`:195-196`) with the ACCEPT arm + empty-name reject; append a `kindEnvironment` spec; first-wins dedup UNCHANGED (§3.1). [EDIT]

**Production — `internal/tracing/resolve.go`:**
- `resolve.go:18` `ResolveCustomTags` switch — ADD a `case kindEnvironment:` (`os.LookupEnv`, omit-iff-resolved-empty, ignores `headerLookup`, §3.3); ADD the `os` import. [EDIT]

**Production — `internal/tracing/span.go`:**
- `BuildServerSpan` + `upsertAttr` — UNCHANGED (§3.5). [NO CHANGE — confirm]

**Production — `internal/filter/hcm/accesslog_emit.go`:**
- `:55` (H1) + `:116` (H2) + `:177` (H3) — UNCHANGED (already thread `ResolveCustomTags`; §3.4). [NO CHANGE]

**Test:**
- `internal/tracing/config_test.go` — CHANGE the `environment` row (`:463-465`) to an ACCEPT + assert the parsed spec; ADD an empty-`environment.name` reject; the `metadata` row (`:468-470`) STAYS; dedup first-wins incl. omit-first-then-literal (§3.1, arm F). [EDIT/ADD]
- `internal/tracing/resolve_test.go` — the `kindEnvironment` resolver matrix incl. present-empty→omit (§10 task 2), via `t.Setenv`. [ADD]
- `internal/tracing/span_test.go` / `zipkin_test.go` — a resolved environment `KV` upserts over a built-in; Zipkin tags map (§10 tasks 3–4). [ADD/EDIT]
- `internal/filter/hcm/fuzz_test.go` — an environment custom_tags seed; no new fuzzer (§6). [ADD]

**Fixture:**
- `test/fixtures/0106-tracing-custom-tags-environment/` (new) — OTLP provider + an environment custom tag (`name: PATH`); the driver drives a plain GET; key-presence + value-non-empty span assertion both sides (§8). [ADD]

**Docs:**
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` (tracing custom_tags clause, §9). [EDIT — IMPL]
- `docs/envoy-go/ROADMAP.md` row 63 → `done` + family prose + deferred-sentence narrow. [EDIT — IMPL]
- `docs/envoy-go/STATE.md` (active-phase header). [EDIT — each stage]
- `docs/envoy-go/DECISIONS.md` — ADR-0284 §Context here (§13); §Decision/§Consequences at the IMPL. [ADD]

---

## 13. ADR continuity — the ADR-0284 §Context DRAFT (anchored here; full entry at the phase-63 IMPL)

**ADR-0284 §Context (draft).** Phase 59 (ADR-0277) lifted the wholesale `custom_tags` reject for the `literal` type — a static `{tag, value}` span attribute on the shared `Span.Attrs []KV` seam, applied via `upsertAttr` in `BuildServerSpan`. Phase 62 (ADR-0283) lifted the `request_header` reject — the FIRST per-request custom tag — introducing the ordered first-wins-deduped `[]CustomTagSpec` config model (`parseCustomTags`) + the per-request `ResolveCustomTags(specs, headerLookup) []KV` seam threaded at the THREE `accesslog_emit.go` `BuildServerSpan` call sites (H1/H2/H3), and PARSE-REJECTED the two remaining types (`environment`/`metadata`) loudly. The ROADMAP Observability family's LIVE deferred sentence + ADR-0277/ADR-0283 §Consequences named `environment`/`metadata` as the next tracing follow-ons. Phase 63 lifts the reject for `environment` — the THIRD custom-tag source type, whose value is read from a named PROCESS ENVIRONMENT VARIABLE (process-STATIC, unlike the per-request `request_header`). SPEC-63 live probes against `envoyproxy/envoy:contrib-v1.37.2` (§11, fresh container per arm) PINNED: the wire form (`{key:<tag>, value:{stringValue:<env value>}}`, identical to `literal`/`request_header`); the missing semantics (present→value / absent+default→default / absent+empty-default→OMIT); and — a NEW finding — that a PRESENT-but-EMPTY env var also OMITS (the resolved value is empty), unifying the rule to **omit iff the resolved value is empty** and requiring `os.LookupEnv` (present-ness). Precedence is IDENTICAL to phase 62 (custom overrides built-in; custom-vs-custom first-in-config-order wins). Crucially, a probe (§11 arm F) showed that an `environment` tag that resolves to NOTHING but is FIRST in config order STILL reserves its config-order key slot (the reference dedups by key at config-LOAD, independent of value resolution) — which, together with the empty-value omit, DECIDED the value-source question (D-ENV-RESOLVE-TIME) toward Option B (a `kindEnvironment` spec stored at parse + resolved per-span in `ResolveCustomTags` via `os.LookupEnv`) over the BRAINSTORM-anticipated Option A (resolve-at-parse to a `kindLiteral`/omit): storing the spec reserves the dedup slot NATURALLY through the existing append path, mirrors the reference's two-phase (dedup-then-resolve) model, and reuses the landed `kindRequestHeader` structure, whereas Option A would need a special-case slot-reservation control-flow wart. The design: extend `customTagKind` with `kindEnvironment` + `CustomTagSpec` with an `EnvName` field; the `environment` accept arm in `parseCustomTags` (empty-name PGV-parity reject); a `kindEnvironment` case in `ResolveCustomTags` (ignores `headerLookup`); ZERO call-site change (the phase-62 seam is already threaded); `BuildServerSpan`/`upsertAttr` UNCHANGED. `metadata` STAYS a loud strict-reject departure (the SOLE remaining `custom_tags` departure; the reference-accept of `environment` confirmed by §11); a NEW empty-`environment.name` PGV-parity reject mirrors the reference's boot-reject (§11 arm E). A SINGLE FLAT ROW (ADR-0045 escape-valve unconsumed); the `kindEnvironment` extension is FOLDED into ADR-0284 (no separate seam ADR — the phase-59/62 precedent). +0 stats / +1 fixture (`0106`, key-presence via `PATH`; value-equality + the D-ENV-HARNESS env-injection deferred) / +0 fuzzers (a seed) / +0 packages / +0 modules. §Decision/§Consequences land at the phase-63 IMPL per ADR-0044. ANCHORS ADR-0284.

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

**Counts UNCHANGED at this SPEC (docs-only; re-verified against master tip `796f97e3`):** stat surface **1201** · fixtures **107** · fuzzers **55** · BackendKind **38** · DECISIONS tail **ADR-0283** (next-free **ADR-0284**) · new Go packages **0** · go.mod modules **2**.

**Anticipated at the phase-63 IMPL:** stat surface **1201 (+0)** · fixtures **107 → 108** (`0106-tracing-custom-tags-environment`) · fuzzers **55 (+0, seed only)** · BackendKind **38 (+0)** · DECISIONS tail **ADR-0284** (next-free **ADR-0285**) · new Go packages **0** · new go.mod modules **0** · row 63 → `done`.

**ROADMAP/STATE at SPEC-DONE:** row 63 STAYS `in-progress` (a row flips `done` only at its IMPL six-gate, ADR-0106). The LIVE deferred sentence is UNCHANGED at this SPEC (`custom_tags` NARROWS to `(metadata)` only at the IMPL — sentinel check-(2) STILL one live match). STATE active-phase header flips to `phase 63 SPEC done` (NEXT = the phase-63 PLAN).

**Next → the phase-63 PLAN** (the TDD decomposition of §10 over this SPEC; every `file:line` RE-DERIVED against the master tip; ADR-0045 single-flat-row; PROGRESS scaffolded).
