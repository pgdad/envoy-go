# PLAN 63 — tracing `custom_tags` `environment` SOURCE arm Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Every task is TDD (`superpowers:test-driven-development`): red → green, with a `-count=1` liveness break where an assertion is load-bearing.

**Goal:** Lift the one `environment type unsupported` reject in `internal/tracing/config.go:195-196` and support the `environment` `CustomTag` type — the THIRD custom-tag source type (after phase-59 `literal` + phase-62 `request_header`), whose value is read from a named PROCESS ENVIRONMENT VARIABLE (process-STATIC, not per-request) — emitted as a `{tag, value}` STRING span attribute on BOTH the OTLP and Zipkin exporters, while `metadata` STAYS parse-rejected loudly (the SOLE remaining `custom_tags` departure) and an empty `environment.name` gains a NEW PGV-parity structural reject.

**Architecture:** D-ENV-RESOLVE-TIME = **Option B** (SPEC §1.1/§3.3 — PINNED by the §11 arm F/G live probes; do NOT re-open). The phase-62 `[]CustomTagSpec` config model + the per-request `ResolveCustomTags(specs, headerLookup) []KV` seam are REUSED unchanged: `customTagKind` gains a `kindEnvironment` value and `CustomTagSpec` gains an `EnvName` field; `parseCustomTags` gains an `environment` ACCEPT arm (empty-name PGV-parity reject) that stores a `kindEnvironment` spec through the EXISTING first-wins-dedup path (so an omitting env tag reserves its config-order key slot NATURALLY — arm F); `ResolveCustomTags` gains a `kindEnvironment` case that reads `os.LookupEnv` and OMITS iff the resolved value is empty (a present-empty var, an absent var with no/empty default — arm G), IGNORING `headerLookup`. The THREE `accesslog_emit.go` `BuildServerSpan` call sites (H1 `:55` / H2 `:116` / H3 `:177`) are UNCHANGED — the phase-62 seam already threads them. One new OTLP differential fixture (`0106`) proves the environment attribute cross-side by KEY-PRESENCE via `PATH` (value differs per side); the present/default/omit/present-empty/dedup value-resolution edges and the Zipkin path are deterministic UNIT tests.

**Tech Stack:** Go; `github.com/envoyproxy/go-control-plane/envoy/type/tracing/v3` (`CustomTag`, `CustomTag_Environment` — an ALREADY-resolved module: `config.go:11` imports `tracingv3`, `config_test.go:11` imports `typetracingv3`); `os.LookupEnv` (stdlib); the differential harness (`test/differential/fixture`, `test/helpers/otlptrace`); `go-fuzz`-style `testing.F` seed corpus.

## Global Constraints

- **Single stage this session context is the IMPL** — subagent-driven per `feedback_execution_style`/`feedback_subagent_autocommit_claudemd`/`feedback_subagents_no_push`: each task is a fresh subagent that commits LOCALLY only; the controller verifies each commit, cleans any leak files, squashes at stage-close, re-runs the suite on the frozen HEAD, and pushes.
- **Worktree discipline** (`feedback_git_worktrees`, `feedback_subagent_worktree_path_targeting`, `feedback_subagent_worktree_detach`): the IMPL runs in `.worktrees/phase-63-impl` (branch `phase-63-tracing-environment-custom-tag-impl`). Pin the canonical worktree root; subagents write worktree-relative paths; the controller verifies the MAIN checkout stays clean. On a deliberate break, restore with `git restore` only (no checkout-sha/amend) and re-verify the branch each task.
- **`next-prompt.txt` IS TRACKED** (`reference_next_prompt_tracked_despite_gitignore`) — edit it inside the stage worktree and fold into the squash; locate commits by SUBJECT (`git log --grep`), never by position.
- **ADR-0080** — every reject substring is DISTINCT. The `metadata` DEPARTURE reject + the NEW empty-`environment.name` PGV-parity reject are distinct from each other and from the phase-59/62 rejects.
- **ADR-0106** — row 63 flips `done` ONLY at this IMPL's six-gate (the SOLE leg; `reference_roadmap_split_phase_row_done`).
- **ADR-0044** — ADR-0284 §Decision/§Consequences land at this IMPL (SPEC §13 drafted §Context); DECISIONS tail **ADR-0283 → ADR-0284** (next-free ADR-0285).
- **ADR-0045** — a SINGLE FLAT ROW (9 tasks; escape-valve UNCONSUMED, well under the ~15 ceiling — the resolver seam + the three call-site threadings are ALREADY LANDED, so this row is SMALLER than phase 62's 9).
- **Per-task gates** (`feedback_pertask_gofmt_lint`): each code task runs `gofmt -l` + `golangci-lint run` on touched packages + `go build ./...` + the touched-package `go test` before its commit.
- **Anticipated counts at IMPL-DONE:** stat surface **1201 (+0)** · fixtures **107 → 108** (`0106`) · fuzzers **55 (+0, seed only)** · BackendKind **38 (+0)** · DECISIONS tail **ADR-0284** (next-free ADR-0285) · new Go packages **0** · new go.mod modules **0**.
- **D-ENV-EMPTYVAL (SPEC §11 arm G) — the omit-iff-resolved-value-empty rule.** A PRESENT-but-EMPTY env var (`os.LookupEnv` returns `("", true)`) → the resolved value is the empty env value (present-ness honored, the default NOT used) → the tag is OMITTED. An ABSENT var → the resolved value is the default → emitted iff non-empty. So the resolver MUST use `os.LookupEnv` (present-ness), NOT `os.Getenv`. This DIVERGES from the phase-62 `request_header` present-empty edge (which emits `""`) — a probe-justified difference; do NOT copy the request_header empty-handling into the environment arm.
- **`reference_fatalf_makes_assertions_unreachable`** — in the fixture driver and `resolve_test.go`, use `Errorf` per independent property/row so one failing case does not mask the rest.
- **`reference_deliberate_break_wrong_assertion`** — every liveness break must confirm WHICH assertion fired (subtest name / message), not merely that *a* failure occurred; add an isolating break where a break could abort earlier and mask the intended one.
- **`reference_differential_break_protocol_count1`** — every differential/liveness break runs with `-count=1` (go-test caching serves a stale PASS otherwise).
- **`reference_vacuous_break_receiver_normalizes`** — a break that changes both the emitted config value AND the expected value fires NOTHING. In the fixture, break the ASSERTION's expected KEY only (the yaml still emits `env_path`).
- **`reference_tracing_upstream_cluster_framework_gap`** — the four EMPTY-emitted built-ins (`upstream_cluster`/`node_id`/`zone`/`peer.address`) are a SEPARATE framework gap; an `environment` tag reads a PROCESS ENV VAR (fully available at the seam via `os.LookupEnv`), NOT those un-plumbed fields — do NOT conflate. UNassert those four VALUES cross-side (the `0102`/`0105` drivers already only assert KEY presence for `upstream_cluster`; the `0106` clone inherits that).
- **`reference_fuzzer_count_docs_drift`** — the seed must NOT move the fuzzer count off 55; reconcile actual `^func Fuzz` = 55 before AND after.
- **`reference_spec_drafted_identifier_collision_check`** — `kindEnvironment`/`EnvName` were RE-GREP-confirmed collision-free in `internal/tracing` against master tip `33d5ed4c` this PLAN session (no existing `kindEnvironment`/`EnvName`/`os` reference in the package).

---

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `internal/tracing/config.go` | ADD `kindEnvironment` const (`:47-49`) + `EnvName` field to `CustomTagSpec` (`:56-62`) | 1 |
| `internal/tracing/resolve.go` | ADD `import "os"` + a `case kindEnvironment:` (os.LookupEnv, omit-iff-resolved-empty, ignores headerLookup) to `ResolveCustomTags` (switch `:18`, after the `kindRequestHeader` case `:33`) | 1 |
| `internal/tracing/resolve_test.go` | ADD the `kindEnvironment` matrix (present / absent+default / absent+no-default / present-empty→omit) via `t.Setenv` | 1 |
| `internal/tracing/config.go` | `parseCustomTags` (`:169`): replace the `environment` reject (`:195-196`) with the ACCEPT arm + empty-name reject; append a `kindEnvironment` spec; dedup (`:202-205`) UNCHANGED | 2 |
| `internal/tracing/config_test.go` | reject table (`:441`): REMOVE the `environment` row (`:462-466`), ADD an `environment-empty-name` reject row; `metadata` row (`:467-471`) STAYS; ADD `TestNewConfigAcceptCustomTagEnvironment` + an environment first-wins dedup test | 2 |
| `internal/tracing/span_test.go` | NEW case: a resolved environment KV upserts over a colliding built-in (arm B); `BuildServerSpan` signature UNCHANGED (confirm no production edit) | 3 |
| `internal/tracing/zipkin_test.go` | NEW case: a resolved environment tag surfaces in the Zipkin `tags` map | 4 |
| `test/fixtures/0106-tracing-custom-tags-environment/` | new OTLP fixture (clone `0105`); `environment{name: PATH}`; plain GET; key-presence + value-non-empty span assertion cross-side | 5 |
| `test/differential/runner_test.go` | blank-import the `0106` driver (after the `0105` line `:132`) | 5 |
| `internal/filter/hcm/fuzz_test.go` | one `environment` custom_tags seed on the existing `FuzzHCMConfigParse` (after the phase-62 seed `:59`) | 6 |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | tracing `custom_tags` clause: `environment` → CONSUMED; add empty-name reject; narrow the departure to `metadata` | 7 |
| `docs/envoy-go/{DECISIONS,STATE,ROADMAP}.md`, `PROGRESS.md`, `next-prompt.txt` | ADR-0284 body; STATE header; ROADMAP row 63 → done + deferred-sentence narrow; PROGRESS close; router roll | 9 |

**RE-DERIVED edit-site roster (verified against master tip `33d5ed4c` this PLAN session, `feedback_brief_citations_not_evidence`):**
- `config.go:11` `tracingv3` import (already present) · `config.go:39` `CustomTags []CustomTagSpec` field · `config.go:42-44` `customTagKind uint8` · `config.go:46-49` the iota const block (`kindLiteral`/`kindRequestHeader`) · `config.go:51-62` `CustomTagSpec` struct (fields `Key`/`Kind`/`LiteralValue`/`HeaderName`/`DefaultValue`/`HasDefault`) · `config.go:169` `parseCustomTags` · `:177-179` empty-tag guard · `:181-194` literal + request_header accept arms · `:195-196` the `environment` reject (LIFTED) · `:197-198` `metadata` reject (UNCHANGED) · `:199-200` typeless default (UNCHANGED) · `:202-205` the `seen`/first-wins dedup block (UNCHANGED).
- `resolve.go` — currently `package tracing` with NO import block; `ResolveCustomTags` at `:12`, the `switch s.Kind` at `:18`, `case kindLiteral:` `:19-20`, `case kindRequestHeader:` `:21-33`, switch close `:34`. ADD `import "os"` + a `case kindEnvironment:` before `:34`.
- `accesslog_emit.go:55` (H1, `r *http.Request`) + `:116` (H2, `req h2.H2Request`) + `:177` (H3, `r *http.Request`) — the three `f.exporter.Export(tracing.BuildServerSpan(*traceDecision, in, tracing.ResolveCustomTags(f.tracingConfig.CustomTags, reqHeaderLookupH1(r)/reqHeaderLookupH2(req)), start, time.Now()))` sites — **UNCHANGED** (already thread the resolver; `reqHeaderLookupH1` `:218`, `reqHeaderLookupH2` `:228`).
- `span.go:12` `type KV struct{ Key; Str string }` · `span.go:64` `BuildServerSpan(d Decision, in SpanInputs, customTags []KV, start, end time.Time) *Span` · `span.go:121` `upsertAttr` — ALL UNCHANGED.
- Test helpers `config_test.go`: `typetracingv3` alias `:11`, `otelProvider` `:65`, `envoyGrpcOTel` `:79`, `zipkinProvider` `:134`, `customTagLiteral` helper func `:397`, `TestNewConfigRejectCustomTagArms` `:441` (rows: `empty-tag`, `empty-literal-value`, `request_header-empty-name` `:458-461`, `environment` `:462-466`, `metadata` `:467-471`, `typeless` `:472-476`), `TestNewConfigAcceptCustomTagRequestHeader` `:499`, `TestNewConfigCustomTagFirstWinsDedup` `:525`.
- `span_test.go`: `freshDecision` `:12`, `freshInputs` `:34` (`Method:"GET"`, `NodeID:""`, `Zone:""`), `attrStr` `:298`, `hasAttr` `:307`; existing custom-tag tests `TestSpanCustomTagsAppend` `:326`, `TestSpanCustomTagsUpsertOverridesBuiltin` `:344`, `TestSpanResolvedRequestHeaderUpsertsOverBuiltin` `:369` (unchanged).
- `zipkin.go`: `encodeZipkinSpans([]*Span, id128, shared bool)` `:78`; the built-in drop guard `if kv.Key == "node_id" || kv.Key == "zone"` `:89`; `tags[kv.Key] = kv.Str` `:92`.
- `zipkin_test.go`: `encoding/json` `:6`; the `b[1:len(b)-1]` single-span decode idiom `:116`; existing `TestZipkinEncodeCustomTagLiteral` `:539`, `TestZipkinEncodeResolvedRequestHeaderTag` `:573`.
- `fuzz_test.go:27` `FuzzHCMConfigParse`; `hcmv3` `:12` + `tracingv3` `:13` already imported; the phase-59 seed `:36-44`, the phase-62 `request_header` seed ends `:59`; `mkHCM` (in `config_test.go`, package `hcm`).
- Fixture clone source `test/fixtures/0105-tracing-custom-tags-request-header/` (driver: `fixtureName` `:93`, `refListenerPort 10105` `:98`, `wantServiceName "0105"` `:123`, `customTagKey "trace_user"` `:132`, `traceUserHeader`/`traceUserValue` `:133-134`, the `req.Header.Set(traceUserHeader, traceUserValue)` `:353`, `FIXTURE_0105_DUMP` `:410`, `assertCustomTag` `:564`, `spanAttrMap` `:579`, `mapKeys` `:639`; the `custom_tags` yaml block `envoy.yaml:58-62` / `envoy-go.yaml:57-61`; `service_name "0105"` `envoy.yaml:55` / `envoy-go.yaml:54`); runner registration `runner_test.go:132` (0105).

**⚠️ Proto oneof Go-type footgun (verified in `type/tracing/v3/custom_tag.pb.go` @ go-control-plane/envoy v1.32.4):** the `environment` oneof wrapper is `CustomTag_Environment_` (TRAILING underscore) wrapping the message `CustomTag_Environment`. Getters: `CustomTag.GetEnvironment() *CustomTag_Environment`, `CustomTag_Environment.GetName() string`, `CustomTag_Environment.GetDefaultValue() string`. (Contrast `CustomTag_RequestHeader` — NO trailing underscore — which wraps the differently-named `CustomTag_Header`.) The existing `config_test.go:464` environment reject row already uses `&typetracingv3.CustomTag_Environment_{Environment: &typetracingv3.CustomTag_Environment{Name: "E"}}` — copy that literal shape.

**⚠️ `EnvName`/`kindEnvironment` naming (RE-GREP-confirmed collision-free).** No existing `kindEnvironment`, `EnvName`, or `os`/`"os"` reference anywhere in `internal/tracing` at master tip `33d5ed4c`. `resolve.go` has NO import block yet, so Task 1 ADDS `import "os"`. The `DefaultValue` field is REUSED (same semantics: value when the source is absent); `kindEnvironment` does NOT consult `HasDefault` (it uses the omit-iff-resolved-empty rule, so `HasDefault` stays `kindRequestHeader`-only — documented on the struct).

**⚠️ Build-ordering (why T1 is additive-first, T2 is the parse arm).** Unlike phase 62, NO field-type change and NO call-site change is needed (`TracingConfig.CustomTags` is already `[]CustomTagSpec`; the resolver is already threaded). So T1 adds `kindEnvironment` + `EnvName` + the resolver `case kindEnvironment:` PURELY ADDITIVELY — the build stays green because `parseCustomTags` still rejects `environment`, so no `kindEnvironment` spec is ever constructed in production; only `resolve_test.go` drives the new case directly with hand-built specs. T2 then lifts the parse reject (the behavior change), after which an environment custom_tag flows through the already-threaded call sites into T1's resolver case. `span.go`/`BuildServerSpan` do NOT change (still `[]KV`), so Tasks 3/4 append tests without touching production.

---

## Task 1: Config model (`kindEnvironment` + `EnvName`) + `ResolveCustomTags` `kindEnvironment` case + resolver matrix

**Files:**
- Modify: `internal/tracing/config.go` (const block `:46-49`; `CustomTagSpec` struct `:51-62`)
- Modify: `internal/tracing/resolve.go` (ADD `import "os"`; ADD `case kindEnvironment:` after `:33`)
- Modify: `internal/tracing/resolve_test.go` (ADD the environment matrix)

**Interfaces:**
- Produces: `kindEnvironment` (a new `customTagKind` iota value); `CustomTagSpec.EnvName string` (a new field). `ResolveCustomTags(specs []CustomTagSpec, headerLookup func(string) ([]string, bool)) []KV` gains a `kindEnvironment` arm (signature UNCHANGED).
- Consumes: `os.LookupEnv` (stdlib, new import); `KV` (existing, `span.go:12`).

- [ ] **Step 1: Add the `kindEnvironment` const** — `config.go:46-49`, extend the iota block:

```go
const (
	kindLiteral customTagKind = iota
	kindRequestHeader
	kindEnvironment // reads a named process env var (os.LookupEnv), omit-on-empty-resolved
)
```

- [ ] **Step 2: Add the `EnvName` field** — `config.go:56-62`, extend the `CustomTagSpec` struct and update the doc comment (the `Kind` comment line `:58` and the `HasDefault` comment `:62`):

```go
type CustomTagSpec struct {
	Key          string        // the span-attribute key (CustomTag.tag)
	Kind         customTagKind // kindLiteral | kindRequestHeader | kindEnvironment
	LiteralValue string        // Kind==kindLiteral: the static value
	HeaderName   string        // Kind==kindRequestHeader: the header to read
	EnvName      string        // Kind==kindEnvironment: the process env var to read (os.LookupEnv)
	DefaultValue string        // Kind==kindRequestHeader | kindEnvironment: value when the source is absent
	HasDefault   bool          // Kind==kindRequestHeader only: DefaultValue != "" (kindEnvironment uses the omit-iff-resolved-empty rule, not HasDefault)
}
```

- [ ] **Step 3: Write the failing resolver matrix** in `internal/tracing/resolve_test.go` (append at end). Drives the environment source arm hermetically via `t.Setenv` (auto-restored; forbids `t.Parallel`). `Errorf` per subtest (`reference_fatalf_makes_assertions_unreachable`):

```go
// TestResolveCustomTagsEnvironment drives the kindEnvironment source arm: env
// present → the env value (the default is IGNORED); env absent + default → the
// default; env absent + no default → OMIT; env PRESENT-but-EMPTY → OMIT (SPEC-63 §11
// arm G — present-ness is honored via os.LookupEnv, the default is NOT used, and an
// empty resolved value is omitted). Together: OMIT iff the resolved value is empty.
// headerLookup is IGNORED by this arm (passed nil). t.Setenv gives hermetic env
// control. Errorf per subtest so one failing case does not mask the rest.
func TestResolveCustomTagsEnvironment(t *testing.T) {
	// A name that is not set in the test environment (absent cases).
	const absent = "ENVOY_GO_TEST_ABSENT_XYZ"

	t.Run("present-uses-env-value", func(t *testing.T) {
		t.Setenv("ENVOY_GO_TEST_PRESENT", "PRESENT-VAL")
		specs := []CustomTagSpec{{Key: "e", Kind: kindEnvironment, EnvName: "ENVOY_GO_TEST_PRESENT", DefaultValue: "def"}}
		got := ResolveCustomTags(specs, nil)
		if len(got) != 1 || got[0].Key != "e" || got[0].Str != "PRESENT-VAL" {
			t.Errorf("present: got %+v, want one {e, PRESENT-VAL} (env value, default ignored)", got)
		}
	})
	t.Run("absent-with-default", func(t *testing.T) {
		specs := []CustomTagSpec{{Key: "e", Kind: kindEnvironment, EnvName: absent, DefaultValue: "def-m"}}
		got := ResolveCustomTags(specs, nil)
		if len(got) != 1 || got[0].Str != "def-m" {
			t.Errorf("absent+default: got %+v, want one {e, def-m}", got)
		}
	})
	t.Run("absent-no-default-omits", func(t *testing.T) {
		specs := []CustomTagSpec{{Key: "e", Kind: kindEnvironment, EnvName: absent}}
		got := ResolveCustomTags(specs, nil)
		if len(got) != 0 {
			t.Errorf("absent+no-default: got %+v, want OMITTED (empty resolved value)", got)
		}
	})
	t.Run("present-empty-omits", func(t *testing.T) {
		t.Setenv("ENVOY_GO_TEST_EMPTY", "") // present, empty string
		specs := []CustomTagSpec{{Key: "e", Kind: kindEnvironment, EnvName: "ENVOY_GO_TEST_EMPTY", DefaultValue: "def-empty"}}
		got := ResolveCustomTags(specs, nil)
		if len(got) != 0 {
			t.Errorf("present-empty: got %+v, want OMITTED (present-empty ignores the default, arm G)", got)
		}
	})
}
```

- [ ] **Step 4: Run — expect FAIL** (the const/field exist from Steps 1–2 but the resolver has no `kindEnvironment` arm, so present/default emit nothing → the matrix fails on `present` and `absent-with-default`):

```
cd internal/tracing && go test -run 'TestResolveCustomTagsEnvironment' -count=1 -v .
```
Expected: FAIL — `present: got [], want one {e, PRESENT-VAL}` (+ `absent+default`).

- [ ] **Step 5: Implement the `kindEnvironment` resolver case** in `resolve.go`. First ADD the import (the file currently has no import block — insert between the `package tracing` line `:1` and the `ResolveCustomTags` doc comment `:3`):

```go
package tracing

import "os"
```

Then ADD the case to the `switch s.Kind` (`:18`), immediately AFTER the `kindRequestHeader` case's closing (before the switch's closing brace at `:34`):

```go
		case kindEnvironment:
			// The env is process-STATIC; os.LookupEnv reports present-ness so a
			// PRESENT-but-EMPTY var ("") is distinguished from an ABSENT one
			// (D-ENV-EMPTYVAL, SPEC §11 arm G). Resolved value = the env value if
			// present, else the DefaultValue. The tag is OMITTED iff the resolved
			// value is empty — a present-empty var, an absent var with no default,
			// and an absent var with an empty default all omit; only a NON-EMPTY
			// resolved value emits. headerLookup is IGNORED (an env tag needs no
			// request header). This DIVERGES from kindRequestHeader's present-empty
			// edge (which emits ""), a probe-justified difference (SPEC §3.3).
			v, present := os.LookupEnv(s.EnvName)
			if !present {
				v = s.DefaultValue
			}
			if v != "" {
				out = append(out, KV{Key: s.Key, Str: v})
			}
```

- [ ] **Step 6: Run — expect PASS** (build stays green — the new symbols are referenced only by the resolver arm + the new test; production `parseCustomTags` still rejects `environment`, so nothing else constructs a `kindEnvironment` spec):

```
cd internal/tracing && go test -run 'TestResolveCustomTags' -count=1 . && go build ./...
```
Expected: PASS.

- [ ] **Step 7: LIVENESS BREAKS (`-count=1`, confirm WHICH fires).** Prove three load-bearing arms are live:
  1. **present-uses-env-value (present-ness honored):** change `if !present { v = s.DefaultValue }` → `v = s.DefaultValue` (unconditional, ignoring the env value) and confirm `TestResolveCustomTagsEnvironment/present-uses-env-value` fires (`got [{e def}], want one {e, PRESENT-VAL}`). Restore.
  2. **present-empty-omits (arm G — the load-bearing D-ENV-EMPTYVAL):** replace the `os.LookupEnv`+present handling with `os.Getenv` semantics — `v := os.Getenv(s.EnvName); if v == "" { v = s.DefaultValue }` — and confirm ONLY `TestResolveCustomTagsEnvironment/present-empty-omits` fires (`got [{e def-empty}], want OMITTED` — Getenv can't tell present-empty from absent, so the default leaks). Restore.
  3. **omit-on-empty-resolved:** change `if v != "" { ... }` to always append (drop the guard) and confirm `TestResolveCustomTagsEnvironment/absent-no-default-omits` fires (`got [{e }], want OMITTED`). Restore.

```
cd internal/tracing && go test -run 'TestResolveCustomTagsEnvironment' -count=1 -v .
```

- [ ] **Step 8: Per-task gates + commit:**

```
gofmt -l internal/tracing/config.go internal/tracing/resolve.go internal/tracing/resolve_test.go   # expect no output
golangci-lint run ./internal/tracing/... && go vet ./internal/tracing/...
go build ./... && cd internal/tracing && go test -count=1 . && cd -
git add internal/tracing/config.go internal/tracing/resolve.go internal/tracing/resolve_test.go
git commit -m "phase 63 IMPL T1: kindEnvironment + EnvName + ResolveCustomTags environment case (os.LookupEnv, omit-iff-resolved-empty) + matrix"
```

---

## Task 2: `config.go` — lift the `environment` reject in `parseCustomTags` (accept + empty-name reject) + config tests

**Files:**
- Modify: `internal/tracing/config.go` (`parseCustomTags` `:195-196` — the `environment` case)
- Modify: `internal/tracing/config_test.go` (reject table `:441`; add accept + dedup tests)

**Interfaces:**
- Consumes: `kindEnvironment`/`CustomTagSpec.EnvName` (Task 1); `CustomTag_Environment` getters (`GetName`/`GetDefaultValue`).
- Produces: `parseCustomTags` now ACCEPTS an `environment` tag into a `kindEnvironment` spec (empty-name reject); the first-wins dedup at `:202-205` is UNCHANGED (the env spec appends through the existing path, reserving its slot — arm F).

- [ ] **Step 1: Update the reject table + add tests in `config_test.go`.**

**(a)** In `TestNewConfigRejectCustomTagArms` (`:441`), REPLACE the `environment` reject row (`:462-466`, currently `Name: "E"` → `wantSub: "environment type unsupported"`) with an `environment-empty-name` reject row (the `metadata` row `:467-471` and all others STAY unchanged):

```go
		{
			name:    "environment-empty-name",
			tag:     &typetracingv3.CustomTag{Tag: "e", Type: &typetracingv3.CustomTag_Environment_{Environment: &typetracingv3.CustomTag_Environment{Name: ""}}},
			wantSub: "environment tag \"e\" empty name",
		},
```

**(b)** ADD two new tests after `TestNewConfigCustomTagFirstWinsDedup` (end of file). The accept test mirrors `TestNewConfigAcceptCustomTagRequestHeader` (`:499`); the dedup test mirrors `TestNewConfigCustomTagFirstWinsDedup` (`:525`) but proves an omitting-env-FIRST tag reserves its config-order slot (arm F, parse half):

```go
// TestNewConfigAcceptCustomTagEnvironment: an environment custom tag parses into a
// CustomTagSpec carrying the env-var name + default. kindEnvironment does NOT set
// HasDefault (it uses the omit-iff-resolved-empty rule).
func TestNewConfigAcceptCustomTagEnvironment(t *testing.T) {
	tr := otelProvider(t, envoyGrpcOTel("c", "svc"))
	tr.CustomTags = []*typetracingv3.CustomTag{
		{Tag: "region", Type: &typetracingv3.CustomTag_Environment_{Environment: &typetracingv3.CustomTag_Environment{Name: "ENVOY_REGION", DefaultValue: "unknown"}}},
		{Tag: "bare", Type: &typetracingv3.CustomTag_Environment_{Environment: &typetracingv3.CustomTag_Environment{Name: "ENVOY_BARE"}}}, // no default
	}
	cfg, err := NewConfig(tr)
	if err != nil {
		t.Fatalf("NewConfig accept environment: unexpected err %v", err)
	}
	if len(cfg.CustomTags) != 2 {
		t.Fatalf("CustomTags len = %d, want 2", len(cfg.CustomTags))
	}
	if got := cfg.CustomTags[0]; got.Key != "region" || got.Kind != kindEnvironment ||
		got.EnvName != "ENVOY_REGION" || got.DefaultValue != "unknown" {
		t.Errorf("CustomTags[0] = %+v, want environment {region,ENVOY_REGION,unknown}", got)
	}
	if got := cfg.CustomTags[1]; got.Key != "bare" || got.Kind != kindEnvironment ||
		got.EnvName != "ENVOY_BARE" || got.DefaultValue != "" {
		t.Errorf("CustomTags[1] = %+v, want environment {bare,ENVOY_BARE,no-default}", got)
	}
}

// TestNewConfigCustomTagEnvironmentFirstWinsDedup: an environment tag placed FIRST in
// config order reserves its key slot at parse — a later same-key tag of ANY source
// type is dropped (SPEC-63 §11 arm F, parse half; the resolve-time omit of an unset
// env tag is TestResolveCustomTagsEnvironment). The stored spec is the FIRST (env).
func TestNewConfigCustomTagEnvironmentFirstWinsDedup(t *testing.T) {
	tr := otelProvider(t, envoyGrpcOTel("c", "svc"))
	tr.CustomTags = []*typetracingv3.CustomTag{
		{Tag: "dup", Type: &typetracingv3.CustomTag_Environment_{Environment: &typetracingv3.CustomTag_Environment{Name: "ENVOY_UNSET_XYZ"}}},
		customTagLiteral("dup", "LIT-VAL"),
	}
	cfg, err := NewConfig(tr)
	if err != nil {
		t.Fatalf("NewConfig env-dedup: unexpected err %v", err)
	}
	if len(cfg.CustomTags) != 1 {
		t.Fatalf("CustomTags len = %d, want 1 (first-wins dedup)", len(cfg.CustomTags))
	}
	if got := cfg.CustomTags[0]; got.Key != "dup" || got.Kind != kindEnvironment || got.EnvName != "ENVOY_UNSET_XYZ" {
		t.Errorf("CustomTags[0] = %+v, want the FIRST (environment ENVOY_UNSET_XYZ)", got)
	}
}
```

- [ ] **Step 2: Run the tests — expect FAIL** (the code still rejects `environment`, so the accept/dedup tests error and the reject table's `environment-empty-name` row expects the wrong substring):

```
cd internal/tracing && go test -run 'TestNewConfig.*CustomTag' -count=1 .
```
Expected: FAIL — `TestNewConfigAcceptCustomTagEnvironment` gets `err = environment type unsupported`; the `environment-empty-name` reject row gets the wrong substring.

- [ ] **Step 3: Lift the `environment` reject in `parseCustomTags`** — `config.go:195-196`, REPLACE the reject case:

```go
		case ct.GetEnvironment() != nil:
			return nil, fmt.Errorf("tracing: custom_tags environment type unsupported")
```

with the ACCEPT arm + empty-name PGV-parity reject (the `metadata` case `:197-198`, the typeless default `:199-200`, and the dedup block `:202-205` are UNCHANGED):

```go
		case ct.GetEnvironment() != nil:
			e := ct.GetEnvironment()
			if e.GetName() == "" {
				return nil, fmt.Errorf("tracing: custom_tags environment tag %q empty name", tag)
			}
			spec = CustomTagSpec{Key: tag, Kind: kindEnvironment, EnvName: e.GetName(), DefaultValue: e.GetDefaultValue()}
```

- [ ] **Step 4: Run tests — expect PASS:**

```
cd internal/tracing && go test -count=1 . && cd - && go build ./... && go test -count=1 ./internal/filter/hcm/...
```
Expected: PASS — the accept/dedup tests, the `environment-empty-name` reject, the unchanged `metadata`/typeless rows, and the already-threaded hcm call sites all green.

- [ ] **Step 5: LIVENESS BREAKS (`-count=1`, confirm WHICH fires).**
  1. **environment accept:** in the accept arm, corrupt the stored name — `EnvName: e.GetName()` → `EnvName: "WRONG"` — and confirm `TestNewConfigAcceptCustomTagEnvironment` fires on `CustomTags[0] = ... want ...ENVOY_REGION...`. Restore.
  2. **empty-name reject:** change the empty-name substring `"empty name"` → `"XXX"` and confirm ONLY `TestNewConfigRejectCustomTagArms/environment-empty-name` fires (its `wantSub`), no cross-firing. Restore.
  3. **first-wins dedup slot reservation (arm F, parse half):** remove the `if _, dup := seen[tag]; dup { continue }` guard (`:202-204`) so BOTH same-key tags append, and confirm `TestNewConfigCustomTagEnvironmentFirstWinsDedup` fires on `CustomTags len = 2, want 1`. Restore.

```
cd internal/tracing && go test -run 'TestNewConfig.*CustomTag' -count=1 -v .
```

- [ ] **Step 6: Per-task gates + commit:**

```
gofmt -l internal/tracing/config.go internal/tracing/config_test.go   # expect no output
golangci-lint run ./internal/tracing/... && go vet ./internal/tracing/...
go build ./... && go test -count=1 ./internal/tracing/... ./internal/filter/hcm/...
git add internal/tracing/config.go internal/tracing/config_test.go
git commit -m "phase 63 IMPL T2: parseCustomTags environment accept + empty-name reject; first-wins dedup reserves the env slot (arm F)"
```

---

## Task 3: `span.go`/`BuildServerSpan` — CONFIRM unchanged + environment upsert-over-built-in test (arm B)

**Files:**
- Modify: `internal/tracing/span_test.go` (add ONE test; `BuildServerSpan` signature UNCHANGED — no production edit)

**Interfaces:**
- Consumes: `BuildServerSpan(d, in, customTags []KV, start, end)` (UNCHANGED); `freshDecision`/`freshInputs`/`attrStr` (`span_test.go:12`/`:34`/`:298`).

- [ ] **Step 1: CONFIRM `BuildServerSpan`/`upsertAttr` need NO change.** The resolver produces unique keys, so `upsertAttr`'s only job is overriding a colliding built-in — the landed behavior. Read `span.go:64` (signature still `customTags []KV`) and `:121` (`upsertAttr`). No production edit. (If any signature drift is found, STOP and reconcile — the SPEC asserts NO change.)

- [ ] **Step 2: Write the failing test** in `span_test.go` (append at end). Mirrors `TestSpanResolvedRequestHeaderUpsertsOverBuiltin` (`:369`) but frames the input as a RESOLVED environment KV (the resolver would have produced it), asserting arm B (custom overrides built-in):

```go
// TestSpanResolvedEnvironmentUpsertsOverBuiltin: a resolved environment custom tag
// whose key collides with a built-in OVERRIDES it (exactly ONE attribute with that
// key, carrying the resolved env value) — arm B (SPEC-63 §11). The resolver hands
// BuildServerSpan a unique-keyed []KV; upsertAttr overrides the built-in.
func TestSpanResolvedEnvironmentUpsertsOverBuiltin(t *testing.T) {
	d := freshDecision()
	in := freshInputs() // built-in http.method == "GET"
	start := time.Now()
	end := start.Add(time.Millisecond)
	// A resolved environment tag: key collides with the built-in http.method.
	s := BuildServerSpan(d, in, []KV{{Key: "http.method", Str: "ENV-METHOD"}}, start, end)

	n := 0
	for _, kv := range s.Attrs {
		if kv.Key == "http.method" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("http.method attribute count = %d, want 1 (upsert, not append-duplicate)", n)
	}
	if v := attrStr(s.Attrs, "http.method"); v != "ENV-METHOD" {
		t.Errorf("http.method = %q, want ENV-METHOD (resolved environment overrides built-in)", v)
	}
}
```

- [ ] **Step 3: Run — expect PASS** (the phase-59 upsert already implements this):

```
cd internal/tracing && go test -run 'TestSpanResolvedEnvironmentUpsertsOverBuiltin' -count=1 -v .
```
Expected: PASS.

- [ ] **Step 4: LIVENESS BREAK (`-count=1`, confirm WHICH fires).** Temporarily make `upsertAttr` always append (delete the replace-in-place loop, leaving only `*attrs = append(*attrs, ct)`) and confirm THIS test fires on `count = 2, want 1`. Restore. (Isolates to the override property.)

```
cd internal/tracing && go test -run 'TestSpanResolvedEnvironmentUpsertsOverBuiltin' -count=1 -v .
```

- [ ] **Step 5: Gates + commit:**

```
gofmt -l internal/tracing/span_test.go && golangci-lint run ./internal/tracing/...
cd internal/tracing && go test -count=1 . && cd -
git add internal/tracing/span_test.go
git commit -m "phase 63 IMPL T3: span_test resolved-environment-upserts-over-builtin (arm B); BuildServerSpan unchanged"
```

---

## Task 4: Zipkin encoder unit test — a resolved environment tag surfaces in the `tags` map

**Files:**
- Modify: `internal/tracing/zipkin_test.go` (add one test; `encoding/json` already imported `:6`)

**Interfaces:**
- Consumes: `BuildServerSpan` (unchanged), `encodeZipkinSpans([]*Span, id128, shared bool)` (`zipkin.go:78`); `freshDecision`/`freshInputs` + the `b[1:len(b)-1]` single-span decode idiom (`zipkin_test.go:116`).

- [ ] **Step 1: Write the failing test** in `zipkin_test.go` (append at end; mirror `TestZipkinEncodeResolvedRequestHeaderTag` `:573`):

```go
// TestZipkinEncodeResolvedEnvironmentTag: a resolved environment custom tag surfaces
// in the Zipkin v2 `tags` map (the shared Attrs seam feeds both exporters);
// node_id/zone stay dropped by the encoder.
func TestZipkinEncodeResolvedEnvironmentTag(t *testing.T) {
	d := freshDecision()
	in := freshInputs()
	in.Authority = "127.0.0.1:10000"
	in.NodeID = "node-x"
	in.Zone = "zone-y"
	start := time.Unix(0, 1_000_000_000)
	end := start.Add(10 * time.Millisecond)
	// The resolver would have produced this KV from {tag: region, environment:{name: ENVOY_REGION}}.
	span := BuildServerSpan(d, in, []KV{{Key: "region", Str: "us-east-2"}}, start, end)

	b, err := encodeZipkinSpans([]*Span{span}, false, true)
	if err != nil {
		t.Fatalf("encodeZipkinSpans err = %v", err)
	}
	var got struct {
		Tags map[string]string `json:"tags"`
	}
	if err := json.Unmarshal(b[1:len(b)-1], &got); err != nil {
		t.Fatalf("decode span: %v (%s)", err, b)
	}
	if got.Tags["region"] != "us-east-2" {
		t.Errorf("tags[region] = %q, want us-east-2", got.Tags["region"])
	}
	if _, ok := got.Tags["node_id"]; ok {
		t.Errorf("tags[node_id] present, want dropped by the Zipkin encoder")
	}
	if _, ok := got.Tags["zone"]; ok {
		t.Errorf("tags[zone] present, want dropped by the Zipkin encoder")
	}
}
```

- [ ] **Step 2: Run — expect PASS** (the tag flows into `Attrs` via the unchanged upsert; `encodeZipkinSpans` builds `tags` from `Attrs`, dropping node_id/zone at `:89`):

```
cd internal/tracing && go test -run 'TestZipkinEncodeResolvedEnvironmentTag' -count=1 -v .
```
Expected: PASS.

- [ ] **Step 3: LIVENESS BREAK (`-count=1`).** Temporarily add `region` to the Zipkin encoder's built-in drop condition (`zipkin.go:89`, change `if kv.Key == "node_id" || kv.Key == "zone"` → `... || kv.Key == "region"`) and confirm the `tags[region]` assertion fires (`= "", want us-east-2`); restore.

```
cd internal/tracing && go test -run 'TestZipkinEncodeResolvedEnvironmentTag' -count=1 -v .
```

- [ ] **Step 4: Gates + commit:**

```
gofmt -l internal/tracing/zipkin_test.go && golangci-lint run ./internal/tracing/...
cd internal/tracing && go test -count=1 . && cd -
git add internal/tracing/zipkin_test.go
git commit -m "phase 63 IMPL T4: Zipkin encoder resolved-environment-tag unit test"
```

---

## Task 5: New OTLP fixture `0106-tracing-custom-tags-environment` (key-presence via `PATH`)

**Files:**
- Create: `test/fixtures/0106-tracing-custom-tags-environment/{envoy.yaml,envoy-go.yaml,expectations.yaml,README.md}`
- Create: `test/fixtures/0106-tracing-custom-tags-environment/driver/driver.go`
- Modify: `test/differential/runner_test.go` (blank-import after the `0105` line `:132`)

**Approach:** CLONE `0105-tracing-custom-tags-request-header` verbatim, then apply the enumerated edits. Do NOT mutate `0087`/`0088`/`0102`/`0105` (`reference_differential_fixture_dispatch_constraint` — one fixture dir = one runner branch). RE-DERIVE the next-free number at implementation: `ls -d test/fixtures/[0-9]*/ | tail -1` — expect `0105-tracing-custom-tags-request-header`, so `0106` is free.

**Fixture semantics (SPEC §8):** the `custom_tags` block reads `PATH` — an env var NATURALLY present + non-empty in BOTH the reference Docker container AND the subject Go subprocess (which inherits `os.Environ()`). The driver drives ONE plain GET (NO request-header manipulation — `environment` reads the process env) and asserts (by KEY, attribute order non-deterministic) that the captured span carries an attribute with key `env_path` **and a non-empty string value** on BOTH sides. The VALUE differs per side (container `PATH` ≠ subject `PATH`), so the assertion is key-present + value-non-empty, NOT value-equality (D-ENV-HARNESS value-injection deferred, SPEC §8). `BackendCount` ≥ 1 (`reference_differential_backendcount_min_one`).

- [ ] **Step 1: Clone the fixture dir:**

```
cp -r test/fixtures/0105-tracing-custom-tags-request-header test/fixtures/0106-tracing-custom-tags-environment
```

- [ ] **Step 2: Swap the `custom_tags` block in BOTH bootstrap templates.** In `envoy.yaml` (block `:58-62`) AND the mirror in `envoy-go.yaml` (`:57-61`), replace the request_header block with an environment tag (a NON-colliding key reading `PATH`), and change `service_name: "0105"` → `"0106"` (`envoy.yaml:55` / `envoy-go.yaml:54`):

```yaml
                  custom_tags:
                  - tag: env_path
                    environment:
                      name: PATH
```
(Verify indentation against each file's existing `custom_tags:` — it sits as a sibling of `provider:`/`random_sampling:` under `tracing:`. Update the `# phase 62 ...` header comments in both yamls to `# phase 63: the tracing block carries a custom_tags environment entry (tag "env_path", read from the PATH process env var) exercising the environment custom_tags source arm.`)

- [ ] **Step 3: Edit `driver/driver.go`** — the enumerated changes on the clone:
  1. Package doc (`:1-90`) + `fixtureName` const (`:93`) → `"0106-tracing-custom-tags-environment"`; reword the doc to describe the environment tag (key-presence cross-side via `PATH`; the driver drives a PLAIN GET — no request header).
  2. `refListenerPort` (`:98`) → `10106`.
  3. `wantServiceName` (`:123`) → `"0106"`.
  4. The custom-tag consts (`:132-134`) → the environment expectation. REPLACE the three consts (`customTagKey`/`traceUserHeader`/`traceUserValue`) with ONE:
  ```go
	// phase 63: the environment custom_tags entry baked into both bootstraps'
	// `tracing` block (sibling of `provider`) reads the PATH process env var —
	// present + non-empty in BOTH the reference container and the subject
	// subprocess. Asserted by KEY with a non-empty value on EVERY span (both
	// sides); the VALUE differs per side (container PATH != subject PATH), so
	// this is key-presence + value-non-empty, NOT value-equality (SPEC §8).
	customTagKey = "env_path"
  ```
  5. **Remove the header-send.** In `fireProbe`, DELETE the line `req.Header.Set(traceUserHeader, traceUserValue)` (`:353`) — this driver drives a PLAIN GET (the `extra` map still overlays the continuation `Traceparent`).
  6. Update `assertCustomTag` (`:564-576`) to assert key-present + value-non-empty (Errorf per property, `reference_fatalf_makes_assertions_unreachable`):
  ```go
	// assertCustomTag asserts the phase-63 environment custom tag on EVERY span,
	// cross-side by KEY (OTLP attribute order is non-deterministic). The tag reads
	// the PATH env var, present + non-empty on both sides, but with a DIFFERENT
	// value per side (container PATH != subject PATH) — so we assert key-present +
	// value-non-empty, NOT value-equality (SPEC §8). Errorf per property.
	func assertCustomTag(t fixture.TB, side string, spans []*tracepb.Span) {
		t.Helper()
		for i, sp := range spans {
			v, ok := spanAttrMap(sp)[customTagKey]
			if !ok {
				t.Errorf("%s span %d: missing custom tag key %q (present: %v)", side, i, customTagKey, mapKeys(spanAttrMap(sp)))
				continue
			}
			if got := v.GetStringValue(); got == "" {
				t.Errorf("%s span %d: %s = empty, want a non-empty resolved PATH value", side, i, customTagKey)
			}
		}
	}
  ```
  7. `FIXTURE_0105_DUMP` (`:410`) → `FIXTURE_0106_DUMP`.
  8. Grep the driver for any remaining `0105`/`trace_user`/`x-trace-user`/`traceUser`/`u-42` and reconcile (the `fixtureDir` comment near the interface assertions, the dump helper comment `:647`). Leave the type name `traceOTLPDriver` and the compile-time interface assertions as-is (a rename is cosmetic; packages are isolated).

- [ ] **Step 4: Update `expectations.yaml` + `README.md`** — reflect the new fixture name/purpose (the environment custom tag `env_path`, read from `PATH`, asserted cross-side by key-presence + value-non-empty; the present/default/omit/present-empty/dedup edges are the deterministic `resolve_test.go`/`config_test.go` unit tests, NOT this fixture). Keep the `0105` framing otherwise (the `upstream_cluster` framework-gap note stays; span count unchanged). Grep the cloned files for stray `0105`/`trace_user`/`request_header` and reconcile.

- [ ] **Step 5: Register + run the fixture.** Add the blank import to `runner_test.go` immediately AFTER the `0105` line (`:132`):

```go
	_ "github.com/pgdad/envoy-go/test/fixtures/0106-tracing-custom-tags-environment/driver"
```
Then (Docker required):

```
go test ./test/differential/ -run 'TestDifferential/0106-tracing-custom-tags-environment' -count=1 -v
```
Expected: PASS (both sides emit a non-empty `env_path`). NOTE the `-run` selector footgun (`reference_differential_run_selector`): use the FULL `TestDifferential/0106-tracing-custom-tags-environment` form, not a bare `0106` (which matches ZERO subtests → vacuous green).

- [ ] **Step 6: LIVENESS BREAK (`-count=1`, confirm WHICH fires).** Break the ASSERTION's expected KEY only (NOT the yaml — that would be vacuous, `reference_vacuous_break_receiver_normalizes`): temporarily change the const `customTagKey = "env_path"` → `"env_path_WRONG"`. The yaml still emits `env_path`, so the assert lookup misses → confirm BOTH sides' `assertCustomTag` fire with `missing custom tag key "env_path_WRONG"` (the custom-tag key-presence assertion, not another). Restore.

```
go test ./test/differential/ -run 'TestDifferential/0106-tracing-custom-tags-environment' -count=1 -v
```

- [ ] **Step 7: Gates + commit** (Docker required for the differential; if the subagent lacks Docker, the controller runs it at stage-close — note the deferral in the commit):

```
gofmt -l test/fixtures/0106-tracing-custom-tags-environment/driver/driver.go && golangci-lint run ./test/...
go build ./...
git add test/fixtures/0106-tracing-custom-tags-environment/ test/differential/runner_test.go
git commit -m "phase 63 IMPL T5: 0106-tracing-custom-tags-environment OTLP fixture (fixtures 107 -> 108)"
```

---

## Task 6: `FuzzHCMConfigParse` seed — one `environment` custom_tags seed (fuzzers stay 55)

**Files:**
- Modify: `internal/filter/hcm/fuzz_test.go` (add one seed after the phase-62 seed `:59`; `hcmv3`/`tracingv3` already imported `:12`/`:13`)

- [ ] **Step 1: Reconcile the fuzzer count BEFORE** (`reference_fuzzer_count_docs_drift`):

```
grep -rn '^func Fuzz' --include='*.go' . | wc -l    # expect 55
```

- [ ] **Step 2: Add the seed** in `FuzzHCMConfigParse`, after the phase-62 `withReqHeaderTags` seed's `f.Add` (`fuzz_test.go:59`):

```go
	// Phase 63: an environment custom_tags seed — one accepted `environment`
	// (name + default) + a mixed literal+environment config with a duplicate key
	// (exercises the environment accept arm + the first-wins dedup; the empty-name
	// reject boundary is a config_test unit test).
	withEnvTags := mkHCM(func(h *hcmv3.HttpConnectionManager) {
		h.Tracing = &hcmv3.HttpConnectionManager_Tracing{
			CustomTags: []*tracingv3.CustomTag{
				{Tag: "region", Type: &tracingv3.CustomTag_Environment_{Environment: &tracingv3.CustomTag_Environment{Name: "ENVOY_REGION", DefaultValue: "unknown"}}},
				{Tag: "dup", Type: &tracingv3.CustomTag_Literal_{Literal: &tracingv3.CustomTag_Literal{Value: "L"}}},
				{Tag: "dup", Type: &tracingv3.CustomTag_Environment_{Environment: &tracingv3.CustomTag_Environment{Name: "ENVOY_DUP"}}},
			},
		}
	})
	f.Add(withEnvTags.GetTypeUrl(), withEnvTags.GetValue())
```

- [ ] **Step 3: Run the fuzzer briefly + reconcile the count AFTER:**

```
cd internal/filter/hcm && go test -run 'FuzzHCMConfigParse' -count=1 . && go test -fuzz 'FuzzHCMConfigParse' -fuzztime 10s .
grep -rn '^func Fuzz' --include='*.go' . | wc -l    # expect 55 (a seed is NOT a new func Fuzz)
```
Expected: PASS; no panic; count STILL 55.

- [ ] **Step 4: Gates + commit:**

```
gofmt -l internal/filter/hcm/fuzz_test.go && golangci-lint run ./internal/filter/hcm/...
git add internal/filter/hcm/fuzz_test.go
git commit -m "phase 63 IMPL T6: FuzzHCMConfigParse environment custom_tags seed (fuzzers stay 55)"
```

---

## Task 7: `BEHAVIOR_CONTRACT.md` edits (tracing `custom_tags` clause)

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`

- [ ] **Step 1: RE-DERIVE the exact lines** — the phase-59/62 IMPLs wrote the tracing `custom_tags` clause (RE-DERIVE against the landed tree):

```
grep -n 'custom_tags\|environment\|request_header\|metadata\|STRICT-REJECT' docs/envoy-go/BEHAVIOR_CONTRACT.md
```

- [ ] **Step 2: Flip `environment` from STRICT-REJECT to CONSUMED** in the tracing `custom_tags` clause. Amend so:
  - the `literal` + `request_header` types STAY CONSUMED;
  - `environment` is now CONSUMED: "the named process env var's value is emitted as a `{tag, value}` STRING span attribute on both exporters; `default_value` on an absent env var; OMITTED when the RESOLVED value is empty — a present-empty var, or an absent var with no/empty default; resolved per-span via `os.LookupEnv`; FIRST-wins dedup on a duplicate tag key (incl. an omitting env tag reserving its config-order slot); OVERRIDES a colliding built-in";
  - `metadata` STAYS STRICT-REJECT — **the SOLE remaining `custom_tags` departure**;
  - ADD the empty-`environment.name` PGV-parity reject to the structural-reject list.

Also NARROW any Zipkin/deferred bullet that names `custom_tags (environment/metadata)` → `(metadata)`.

(Exact final wording RE-DERIVED and written against the landed lines — no line number is load-bearing here beyond the grep.)

- [ ] **Step 3: Commit** (docs-only; grep-confirm `environment` no longer reads as strict-rejected in the tracing clause):

```
git add docs/envoy-go/BEHAVIOR_CONTRACT.md
git commit -m "phase 63 IMPL T7: BEHAVIOR_CONTRACT custom_tags environment CONSUMED + empty-name reject; departure narrows to metadata"
```

---

## Task 8: Verify — six-gate + full 108-dir differential

**Files:** none (verification only).

- [ ] **Step 1: Six-gate on the frozen HEAD:**

```
gofmt -l internal/ test/ cmd/            # expect no output
golangci-lint run ./...
go vet ./...
go build ./...
go mod tidy -diff                        # expect EMPTY (CustomTag_Environment is an already-resolved module; re-check `git diff go.mod` per reference_new_subpackage_pulls_transitive_module — no NEW sub-package here, only a new field getter + the stdlib os import)
go test -race -count=1 ./internal/tracing/... ./internal/filter/hcm/...
```
The `-race` on BOTH `internal/tracing` and `internal/filter/hcm` is required (`reference_detrand_race_catches_protojson_value_substring`, `reference_full_suite_race_after_background_mutator`).

- [ ] **Step 2: Full differential — 108 dirs, byte-stable except `0106`** (Docker; reference `envoyproxy/envoy:contrib-v1.37.2`):

```
go test ./test/differential/ -count=1
```
Expected: `ok`, exit 0. The 107 pre-existing dirs stay byte-stable (no `environment` custom_tags in their configs; the `0102` literal + `0105` request_header fixtures stay green — those single unique-key tags resolve identically); `0106` is the only new dir. If a startup flake appears on an UNRELATED fixture (`subject ready: EOF`), isolate-re-run to discriminate (`reference_differential_fullsuite_startup_flake`).

- [ ] **Step 3: Record the evidence** (the six-gate output + the differential `ok ... exit 0`) in `PROGRESS.md`. No commit (verification task); findings feed Task 9's ADR-0284 §Consequences.

---

## Task 9: ADR-0284 body + STATE + ROADMAP + PROGRESS close + router roll

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0284)
- Modify: `docs/envoy-go/STATE.md` (active-phase header)
- Modify: `docs/envoy-go/ROADMAP.md` (row 63 → `done`; family prose + LIVE deferred-sentence narrow)
- Modify: `docs/envoy-go/phases/63-tracing-custom-tags-environment/PROGRESS.md` (close)
- Modify: `next-prompt.txt` (router roll to the NEXT decision)

- [ ] **Step 1: Append ADR-0284 §Decision/§Consequences** to `DECISIONS.md` (reproduce the SPEC §13 §Context draft, then add):
  - **§Decision:** the `environment` custom_tag type CONSUMED; `customTagKind` extended with `kindEnvironment` + `CustomTagSpec` with an `EnvName` field; the `environment` accept arm in `parseCustomTags` (empty-name PGV-parity reject) stores a `kindEnvironment` spec through the EXISTING first-wins-dedup path (reserving its slot naturally — arm F); a `kindEnvironment` case in `ResolveCustomTags` reads `os.LookupEnv` and OMITS iff the resolved value is empty (D-ENV-EMPTYVAL arm G — present-empty ⇒ omit; requires `os.LookupEnv` not `os.Getenv`), IGNORING `headerLookup`; **ZERO call-site change** (the phase-62 seam already threads H1/H2/H3); `BuildServerSpan`/`upsertAttr` UNCHANGED. D-ENV-RESOLVE-TIME = **Option B** (a parse-stored spec resolved per-span) over the BRAINSTORM-anticipated Option A (resolve-at-parse), decided by the §11 arm F (dedup-slot-reservation) + arm G (present-empty-omit) probes (ADR-0044 empirical refinement); the `kindEnvironment` extension FOLDED into ADR-0284 (no separate seam ADR — the phase-59/62 precedent). `metadata` STAYS a loud strict-reject DEPARTURE (the SOLE remaining `custom_tags` departure — the reference-accept of `environment` confirmed by §11 arms A–D/F/G); a NEW empty-`environment.name` PGV-parity reject.
  - **§Consequences:** the `0106` OTLP differential (key-presence cross-side via `PATH` + value-non-empty, `Errorf`-per-property, break-proven live); the present/default/omit/present-empty/dedup value-resolution edges + the precedence-upsert + the Zipkin path are deterministic UNIT tests (`resolve_test.go`/`config_test.go`/`span_test.go`/`zipkin_test.go`); a `FuzzHCMConfigParse` seed; +0 stats / +1 fixture (`0106`) / +0 fuzzers / +0 packages / +0 modules; the value-equality env-injection differential (D-ENV-HARNESS) DEFERRED; the four EMPTY-emitted built-ins (`reference_tracing_upstream_cluster_framework_gap`) UNTOUCHED. Record the six-gate + 108-dir result from Task 8. DECISIONS tail ADR-0283 → **ADR-0284** (next-free ADR-0285).

- [ ] **Step 2: Update `STATE.md`** active-phase header → `phase 63 (tracing-custom-tags-environment) IMPL done` (lifecycle 3 → done; row 63 → `done`; DECISIONS tail ADR-0283 → ADR-0284). Counts: fixtures 107 → 108; all others +0.

- [ ] **Step 3: Update `ROADMAP.md`** — flip row 63 `in-progress` → `done` (ADR-0106, the SOLE leg — `reference_roadmap_split_phase_row_done`). NARROW the LIVE Observability deferred sentence: `custom_tags (environment/metadata)` → `custom_tags (metadata)`. Then RE-RUN the sentinel check-(2) grep and confirm EXACTLY ONE live `candidates:` match for the Observability family (`reference_sentinel_deferred_sentence_live_vs_historical` — the live sentence uses "candidates:", not "candidates were:"):

```
grep -oE 'remaining deferred \(not-yet-chartered\) candidates:[^.]*\.' docs/envoy-go/ROADMAP.md
```
Expected: still THREE live "candidates:" sentences total (HTTP/3, xDS, Observability) — the Observability one now names `custom_tags (metadata)` (narrowed), not `(environment/metadata)`.

- [ ] **Step 4: Close `PROGRESS.md`** — mark all tasks `[x]`, record the liveness-break outcomes + the Task-8 verify evidence + the landed task commit shas + the exit counts (mirror the phase-62 PROGRESS close shape).

- [ ] **Step 5: Roll `next-prompt.txt`** to the NEXT router decision — a new BRAINSTORM (the roller self-picks the smallest defensible candidate per the 2026-07-12 standing directive; the sentinel does NOT fire — re-run the three checks mechanically and record they still print: (1) once row 63 flips `done`, check whether ANY row remains non-`done`; (2) three families still carry candidates AFTER the Observability narrow — HTTP/3, xDS, and Observability (now `custom_tags (metadata)`); (3) gRPC/Runtime/WASM never opened + Operational-tooling open). Update the STATUS block, the "What THIS session does" section, the counts (fixtures 108, DECISIONS tail ADR-0284), and the SENTINEL RE-CHECKED date.

- [ ] **Step 6: Commit** (docs + router are docs-only):

```
git add docs/envoy-go/DECISIONS.md docs/envoy-go/STATE.md docs/envoy-go/ROADMAP.md docs/envoy-go/phases/63-tracing-custom-tags-environment/PROGRESS.md next-prompt.txt
git commit -m "phase 63 (tracing-custom-tags-environment) IMPL: ADR-0284 + STATE/ROADMAP row 63 done + router roll"
```

- [ ] **Step 7: Controller stage-close** — squash all task commits into ONE `phase 63 (tracing-custom-tags-environment) IMPL: ...` commit on `master`, re-run the six-gate + 108-dir differential on the frozen HEAD, then PUSH (`feedback_push_to_origin`, `feedback_subagents_no_push` — only the controller pushes).

---

## Self-Review (checked against SPEC 63 this PLAN session)

**Spec coverage:** SPEC §3.1 parse arm (environment accept + empty-name reject; dedup unchanged) → Task 2. §3.2 config model (`kindEnvironment` + `EnvName`) → Task 1. §3.3 `ResolveCustomTags` `kindEnvironment` case (os.LookupEnv, omit-iff-resolved-empty, ignores headerLookup) → Task 1. §3.4 ZERO call-site change → confirmed in File Structure + Task 2 Step 4 (hcm tests green, no edit). §3.5 `BuildServerSpan`/`upsertAttr` unchanged → Task 3 (confirm + arm-B test). §6 reject roster (empty-name reject + metadata departure) → Task 2; fuzz seed → Task 6. §7 +0 stats → Task 8 gate. §8 fixture (`0106`, key-presence) → Task 5; Zipkin unit test → Task 4; the present/default/omit/present-empty/dedup edges → Task 1 (`resolve_test.go`) + Task 2 (config dedup test). §9 BEHAVIOR_CONTRACT → Task 7. §10 test plan → Tasks 1–6. §13 ADR → Task 9. No gap.

**Placeholder scan:** every code step carries real code; the only "clone-then-edit" is Task 5 (the `0105` driver), enumerated as explicit edits with the changed `assertCustomTag` reproduced in full. Task 7's exact BEHAVIOR_CONTRACT wording is deferred to a RE-DERIVE-then-write step (the lines drift between phases) — a documented docs-edit, not a code placeholder.

**Type consistency:** `CustomTagSpec{Key, Kind, LiteralValue, HeaderName, EnvName, DefaultValue, HasDefault}` + `customTagKind` (`kindLiteral`/`kindRequestHeader`/`kindEnvironment`) used identically in Tasks 1/2/6; `ResolveCustomTags(specs []CustomTagSpec, headerLookup func(string) ([]string, bool)) []KV` UNCHANGED signature, `kindEnvironment` arm added in Task 1; `parseCustomTags([]*tracingv3.CustomTag) ([]CustomTagSpec, error)` UNCHANGED signature, env accept arm in Task 2; `TracingConfig.CustomTags []CustomTagSpec` UNCHANGED (no field-type change); `BuildServerSpan(d, in, []KV, start, end)` UNCHANGED (Tasks 3/4 pass `[]KV` literals). The `environment` proto oneof wrapper (`CustomTag_Environment_` TRAILING underscore wrapping `CustomTag_Environment`, getters `GetName`/`GetDefaultValue`) used correctly in Tasks 2/6 and the config-test rows. `customTagKey = "env_path"` used consistently in Task 5's yaml + driver assert.

**Ordering / build-green:** Task 1 (types + resolver arm + resolve_test) is PURELY ADDITIVE — build green (parse still rejects environment, so no `kindEnvironment` spec is constructed in production; only resolve_test drives it). Task 2 lifts the parse reject (the behavior change) + adds config tests — the env spec then flows through the already-threaded call sites into Task 1's resolver arm; NO field-type or call-site change, so the build never breaks. Tasks 3/4 depend on nothing new (BuildServerSpan unchanged). Task 5 depends on Tasks 1–2 (the fixture exercises the full path). Task 6 independent. Task 8 gates the frozen tree. Task 9 lands docs + router roll. The stale `environment` reject row is explicitly removed and replaced with an `environment-empty-name` reject in Task 2 Step 1(a).
