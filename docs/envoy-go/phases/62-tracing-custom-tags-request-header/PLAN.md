# PLAN 62 — tracing `custom_tags` `request_header` SOURCE arm Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Every task is TDD (`superpowers:test-driven-development`): red → green, with a `-count=1` liveness break where an assertion is load-bearing.

**Goal:** Lift the one `request_header type unsupported` reject in `internal/tracing/config.go:155-156` and support the `request_header` `CustomTag` type — the FIRST custom tag whose value is resolved PER-REQUEST from a named DOWNSTREAM REQUEST header (with an optional `default_value` / omit-on-missing) — emitted as a `{tag, value}` STRING span attribute on BOTH the OTLP and Zipkin exporters, while `environment`/`metadata` STAY parse-rejected loudly and an empty `request_header.name` gains a NEW PGV-parity structural reject.

**Architecture:** The phase-59 static `TracingConfig.CustomTags []KV` field becomes an ORDERED `[]CustomTagSpec` — a spec list deduplicated by tag key FIRST-wins at parse (`parseCustomTags`), matching the reference's config-time map (SPEC §11 arms C/D). A NEW per-request `ResolveCustomTags(specs, headerLookup) []KV` (`internal/tracing/resolve.go`) resolves each spec against the request's header lookup: a `literal` spec yields its static value; a `request_header` spec yields the FIRST value of the named header, or the `DefaultValue` when the header is absent and a default was configured, or NOTHING when absent with no default. The resolved `[]KV` (now unique-keyed) is threaded at the THREE `accesslog_emit.go` `BuildServerSpan` call sites (H1 `:55` / H2 `:116` / H3 `:177`) via the EXISTING `reqHeaderLookupH1`/`reqHeaderLookupH2` lookups; `BuildServerSpan`/`upsertAttr` are UNCHANGED (they now only override built-ins, the resolver having produced unique keys). One new OTLP differential fixture (`0105`) proves the present-case attribute cross-side; the default/omit/multi-value/precedence-dedup edges and the Zipkin path are deterministic UNIT tests.

**Tech Stack:** Go; `github.com/envoyproxy/go-control-plane/envoy/type/tracing/v3` (`CustomTag`, `CustomTag_Header` — an ALREADY-resolved module: `config.go:12` imports `tracingv3`, `config_test.go:11` imports `typetracingv3`); the differential harness (`test/differential/fixture`, `test/helpers/otlptrace`); `go-fuzz`-style `testing.F` seed corpus.

## Global Constraints

- **Single stage this session context is the IMPL** — subagent-driven per `feedback_execution_style`/`feedback_subagent_autocommit_claudemd`/`feedback_subagents_no_push`: each task is a fresh subagent that commits LOCALLY only; the controller verifies each commit, cleans any leak files, squashes at stage-close, re-runs the suite on the frozen HEAD, and pushes.
- **Worktree discipline** (`feedback_git_worktrees`, `feedback_subagent_worktree_path_targeting`, `feedback_subagent_worktree_detach`): the IMPL runs in `.worktrees/phase-62-impl` (branch `phase-62-tracing-request-header-custom-tag-impl`). Pin the canonical worktree root; subagents write worktree-relative paths; the controller verifies the MAIN checkout stays clean. On a deliberate break, restore with `git restore` only (no checkout-sha/amend) and re-verify the branch each task.
- **`next-prompt.txt` IS TRACKED** (`reference_next_prompt_tracked_despite_gitignore`) — edit it inside the stage worktree and fold into the squash; locate commits by SUBJECT (`git log --grep`), never by position.
- **ADR-0080** — every reject substring is DISTINCT. The three unchanged rejects (`environment`/`metadata` DEPARTURES) + the NEW empty-`request_header.name` PGV-parity reject are all distinct from each other and from the phase-59 rejects.
- **ADR-0106** — row 62 flips `done` ONLY at this IMPL's six-gate (the SOLE leg; `reference_roadmap_split_phase_row_done`).
- **ADR-0044** — ADR-0283 §Decision/§Consequences land at this IMPL (SPEC §13 drafted §Context); DECISIONS tail **ADR-0282 → ADR-0283** (next-free ADR-0284).
- **ADR-0045** — a SINGLE FLAT ROW (9 tasks; escape-valve UNCONSUMED, well under the ~15 ceiling; the SPEC anticipated ~11–13 — this PLAN folds the resolver+types into T1 and the parse+field+call-site+config-tests atomic change into T2).
- **Per-task gates** (`feedback_pertask_gofmt_lint`): each code task runs `gofmt -l` + `golangci-lint run` on touched packages + `go build ./...` + the touched-package `go test` before its commit.
- **Anticipated counts at IMPL-DONE:** stat surface **1201 (+0)** · fixtures **106 → 107** (`0105`) · fuzzers **55 (+0, seed only)** · BackendKind **38 (+0)** · DECISIONS tail **ADR-0283** (next-free ADR-0284) · new Go packages **0** · new go.mod modules **0**.
- **The first-wins-dedup correction (SPEC §1.1).** Custom-vs-custom same-key collisions are resolved FIRST-wins at PARSE (a `seen` set in `parseCustomTags`), NOT last-wins at emit. This CORRECTS a latent phase-59 divergence (two same-key literal tags would have emitted the second value); the single-key common case is byte-stable and the `0102` literal differential stays green. `upsertAttr`'s last-wins branch now only ever fires against BUILT-INS (custom-overrides-built-in = arm B = the reference).
- **`reference_fatalf_makes_assertions_unreachable`** — in the fixture driver, use `Errorf` per independent property; in `resolve_test.go` use `Errorf` per matrix row so one failing case does not mask the rest.
- **`reference_deliberate_break_wrong_assertion`** — every liveness break must confirm WHICH assertion fired (subtest name / message), not merely that *a* failure occurred; add an isolating break where a break could abort earlier and mask the intended one.
- **`reference_differential_break_protocol_count1`** — every differential/liveness break runs with `-count=1` (go-test caching serves a stale PASS otherwise).
- **`reference_tracing_upstream_cluster_framework_gap`** — the four EMPTY-emitted built-ins (`upstream_cluster`/`node_id`/`zone`/`peer.address`) are a SEPARATE framework gap; a `request_header` tag reads a REQUEST HEADER (fully available at the seam), NOT those un-plumbed fields — do NOT conflate. UNassert those four VALUES cross-side (the `0102`/`0105` drivers already only assert KEY presence for `upstream_cluster`).

---

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `internal/tracing/config.go` | ADD `CustomTagSpec`/`customTagKind` types near `TracingConfig` | 1 |
| `internal/tracing/resolve.go` (new) | `ResolveCustomTags(specs []CustomTagSpec, headerLookup func(string) ([]string, bool)) []KV` | 1 |
| `internal/tracing/resolve_test.go` (new) | the resolver matrix (present / default / omit / multi-value / nil-lookup / literal / mixed-ordered) | 1 |
| `internal/tracing/config.go` | reshape `parseCustomTags` → `([]CustomTagSpec, error)` (request_header accept + empty-name reject + first-wins dedup); change `TracingConfig.CustomTags` field type; type-only `cfg.CustomTags` set | 2 |
| `internal/tracing/config_test.go` | accept-tests → spec shape; NEW request_header accept + empty-name reject + first-wins dedup; drop the now-stale `request_header`-reject row; keep environment/metadata/empty-tag/empty-value/typeless | 2 |
| `internal/filter/hcm/accesslog_emit.go` | thread `tracing.ResolveCustomTags(f.tracingConfig.CustomTags, reqHeaderLookupH1(r)/reqHeaderLookupH2(req))` at the 3 call sites (`:55` H1, `:116` H2, `:177` H3) | 2 |
| `internal/tracing/span_test.go` | NEW case: a resolved request_header KV upserts over a colliding built-in (arm B); `BuildServerSpan` signature UNCHANGED (confirm no call-site edit) | 3 |
| `internal/tracing/zipkin_test.go` | NEW case: a resolved request_header tag surfaces in the Zipkin `tags` map | 4 |
| `test/fixtures/0105-tracing-custom-tags-request-header/` | new OTLP fixture (clone `0102`); driver sends `x-trace-user`; present-case span attribute asserted cross-side by key | 5 |
| `test/differential/runner_test.go` | blank-import the `0105` driver (after the `0104` line `:131`) | 5 |
| `internal/filter/hcm/fuzz_test.go` | one `request_header` custom_tags seed on the existing `FuzzHCMConfigParse` | 6 |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | tracing `custom_tags` clause: `request_header` → CONSUMED; add empty-name reject | 7 |
| `docs/envoy-go/{DECISIONS,STATE,ROADMAP}.md`, `next-prompt.txt` | ADR-0283 body; STATE header; ROADMAP row 62 → done + deferred-sentence narrow; router roll | 9 |

**RE-DERIVED edit-site roster (verified against master tip `460f761e` this PLAN session, `feedback_brief_citations_not_evidence`):**
- `config.go:25-38` `TracingConfig` struct (the `CustomTags []KV` field is `:33-37`; the `//nolint:revive` is `:24`) · `config.go:7-16` import block (`tracingv3` alias at `:12`, already imported) · `config.go:88-91` the `parseCustomTags` call in `NewConfig` · `config.go:116-123` provider switch · `config.go:127` `cfg.CustomTags = customTags` · `config.go:139-166` `parseCustomTags` (the `request_header` reject is `:155-156`; empty-tag guard `:145-147`; literal arm `:149-154`; environment `:157-158`; metadata `:159-160`; typeless default `:161-162`).
- `resolve.go` — NEW file (does NOT exist yet).
- `span.go:64` `BuildServerSpan` signature (`customTags []KV` param already present, phase 59) · `span.go:99-101` the `upsertAttr` loop · `span.go:117-129` `upsertAttr` — ALL UNCHANGED.
- `accesslog_emit.go:55` (H1, `r *http.Request`) + `:116` (H2, `req h2.H2Request`) + `:177` (H3, `r *http.Request`) — the three `f.exporter.Export(tracing.BuildServerSpan(*traceDecision, in, f.tracingConfig.CustomTags, start, time.Now()))` sites · `reqHeaderLookupH1` `:218` · `reqHeaderLookupH2` `:228` (both `func(string) ([]string, bool)`, REUSED unchanged).
- Test helpers: `config_test.go` `typetracingv3` alias `:11`, `otelProvider` `:65`, `zipkinProvider` `:134`, `envoyGrpcOTel` `:79`, `customTagLiteral` `:397`, `TestNewConfigAcceptCustomTagLiteral` `:406`, `...Zipkin` `:423`, `TestNewConfigRejectCustomTagArms` `:440` (the `request_header` reject row is `:456-460`), `TestNewConfigRejectArms` typeless `custom_tags` row `:320-327` (STAYS GREEN — typeless → "missing type").
- `span_test.go`: `freshDecision` `:12`, `freshInputs` `:34` (`Method:"GET"`, `NodeID:""`, `Zone:""`), `attrStr` `:298`, `hasAttr` `:307`; existing phase-59 custom-tag tests `:331`/`:349` (unchanged); all `BuildServerSpan(d, in, nil|[]KV{...}, start, end)` call sites at `:60,151,178,193,207,283,331,349` — UNCHANGED (the signature does not change).
- `zipkin_test.go`: `encodeZipkinSpans([]*Span{span}, false, true)` idiom `:90`; existing phase-59 `custom_env` Zipkin test `:547`; `encoding/json` already imported.
- `fuzz_test.go:27` `FuzzHCMConfigParse`; `hcmv3` `:12` + `tracingv3` `:13` already imported; the phase-59 custom_tags seed `:36-44`; `mkHCM` (in `config_test.go`, package `hcm`).
- Fixture clone source `test/fixtures/0102-tracing-custom-tags-literal/` (driver 728 lines; the `custom_tags` yaml block is `envoy.yaml:55-58` / mirror in `envoy-go.yaml`; `wantServiceName` `driver.go:118`; `customTagKey/Value` `:127-128`; `fixtureName` `:88`; `refListenerPort 10102` `:93`; `FIXTURE_0102_DUMP` `:403`); runner registration `runner_test.go:129` (0102) / `:131` (0104).

**⚠️ Proto oneof Go-type footgun (verified in `type/tracing/v3/custom_tag.pb.go` @ go-control-plane/envoy v1.32.4):** the oneof wrappers are `CustomTag_Literal_`, `CustomTag_Environment_`, `CustomTag_Metadata_` (TRAILING underscore) but `CustomTag_RequestHeader` (NO trailing underscore — it wraps the differently-named message `CustomTag_Header`). Getters: `GetTag()`, `GetLiteral() *CustomTag_Literal`, `GetRequestHeader() *CustomTag_Header`, `GetEnvironment()`, `GetMetadata()`. `CustomTag_Header.GetName() string`, `CustomTag_Header.GetDefaultValue() string`.

**⚠️ Kind-const naming collision (RE-DERIVED — corrects the SPEC §3.2 draft).** SPEC §3.2 drafted the iota consts as `customTagLiteral`/`customTagRequestHeader`. But `config_test.go:397` ALREADY declares `func customTagLiteral(tag, value string) *typetracingv3.CustomTag` in `package tracing` (a phase-59 helper), so a `customTagLiteral` const is a DUPLICATE DECLARATION → compile error. This PLAN uses `kindLiteral`/`kindRequestHeader` instead (the `customTagKind` TYPE name does not collide and is kept). All task code below uses `kindLiteral`/`kindRequestHeader`.

**⚠️ The existing `TestNewConfigRejectCustomTagArms` `request_header` row (`config_test.go:456-460`)** asserts `wantSub: "request_header type unsupported"`. Under the new code request_header is ACCEPTED, so this row would go GREEN-but-wrong (its `err == nil` guard fails). Task 2 REMOVES this row and REPLACES it with an accept test + an empty-name reject test. Do NOT leave it.

**⚠️ Build-ordering (why T1 is the resolver, T2 is the atomic field change).** `TracingConfig.CustomTags` ALREADY exists as `[]KV` and is ALREADY referenced by the three `accesslog_emit.go` call sites and the two `config_test.go` accept tests. Changing its type to `[]CustomTagSpec` breaks all five references at once. So the field-type change, the `parseCustomTags` reshape, the three call-site threadings, and the config-test updates MUST land in ONE task (T2) to keep the build green — and that task consumes `ResolveCustomTags`, which is why the resolver + its types land FIRST in T1 (purely additive: nothing references them until T2). `span.go`/`BuildServerSpan` do NOT change (still `[]KV`), so `span_test.go`/`zipkin_test.go` call sites are untouched.

---

## Task 1: `resolve.go` — `CustomTagSpec`/`customTagKind` types + `ResolveCustomTags` resolver

**Files:**
- Modify: `internal/tracing/config.go` (ADD the two types near `TracingConfig`, after the struct at `:38`)
- Create: `internal/tracing/resolve.go`
- Create: `internal/tracing/resolve_test.go`

**Interfaces:**
- Produces: `CustomTagSpec` struct + `customTagKind` (`kindLiteral`/`kindRequestHeader` iota consts — NOT `customTagLiteral`: that name is ALREADY a `config_test.go:397` helper func in `package tracing`, so a `customTagLiteral` const would be a duplicate-declaration compile error; see the ⚠️ note below); `ResolveCustomTags(specs []CustomTagSpec, headerLookup func(string) ([]string, bool)) []KV`. `KV` is the existing `span.go:12` type.
- Consumes: nothing new (purely additive; `TracingConfig.CustomTags` stays `[]KV` this task).

- [ ] **Step 1: Add the two types to `config.go`** — insert AFTER the `TracingConfig` struct's closing brace (`config.go:38`), before the `ProviderKind` comment (`:40`):

```go
// customTagKind selects a CustomTagSpec's value source: a static literal, or a
// per-request lookup of a named downstream request header.
type customTagKind uint8

const (
	kindLiteral customTagKind = iota
	kindRequestHeader
)

// CustomTagSpec is one parsed HCM tracing custom_tag, resolved per-request by
// ResolveCustomTags. Kind selects the source: a static literal value, or a
// request-header lookup (with an optional default / omit-on-missing). The spec
// list on TracingConfig.CustomTags is ordered and first-wins-deduplicated by Key
// at parse (matching the reference's config-time map, SPEC-62 §11 arms C/D).
type CustomTagSpec struct {
	Key          string        // the span-attribute key (CustomTag.tag)
	Kind         customTagKind // kindLiteral | kindRequestHeader
	LiteralValue string        // Kind==kindLiteral: the static value
	HeaderName   string        // Kind==kindRequestHeader: the header to read
	DefaultValue string        // Kind==kindRequestHeader: value when the header is absent
	HasDefault   bool          // Kind==kindRequestHeader: DefaultValue != "" (else omit on absent)
}
```

- [ ] **Step 2: Write the failing resolver tests** in a NEW file `internal/tracing/resolve_test.go`:

```go
package tracing

import "testing"

// lookupFunc builds a header lookup from a name→values map (case-sensitive on the
// exact configured name; the production lookups are case-insensitive but the
// resolver is agnostic to that — it just calls the func it is handed).
func lookupFunc(m map[string][]string) func(string) ([]string, bool) {
	return func(name string) ([]string, bool) {
		v, ok := m[name]
		return v, ok
	}
}

// TestResolveCustomTagsMatrix drives every source/missing/multi/nil-lookup arm.
// Errorf per row so one failing case does not mask the rest
// (reference_fatalf_makes_assertions_unreachable).
func TestResolveCustomTagsMatrix(t *testing.T) {
	specs := []CustomTagSpec{
		{Key: "lit", Kind: kindLiteral, LiteralValue: "LIT-VAL"},
		{Key: "present", Kind: kindRequestHeader, HeaderName: "x-present", DefaultValue: "def-p", HasDefault: true},
		{Key: "missdef", Kind: kindRequestHeader, HeaderName: "x-missing", DefaultValue: "def-m", HasDefault: true},
		{Key: "missnodef", Kind: kindRequestHeader, HeaderName: "x-absent"}, // no default → omit
		{Key: "multi", Kind: kindRequestHeader, HeaderName: "x-multi"},
	}
	lookup := lookupFunc(map[string][]string{
		"x-present": {"PRESENT-VAL"},
		"x-multi":   {"MV-A", "MV-B"}, // multi-value → FIRST
	})
	got := ResolveCustomTags(specs, lookup)

	// Build a key→value map from the resolved KVs; assert presence + values by key
	// (omitted keys are simply absent).
	byKey := map[string]string{}
	for _, kv := range got {
		if _, dup := byKey[kv.Key]; dup {
			t.Errorf("duplicate resolved key %q", kv.Key)
		}
		byKey[kv.Key] = kv.Str
	}
	want := map[string]string{
		"lit":     "LIT-VAL",     // literal → static
		"present": "PRESENT-VAL", // header present → the header value (default ignored)
		"missdef": "def-m",       // header absent + default → the default
		"multi":   "MV-A",        // header sent twice → the FIRST value
	}
	for k, wv := range want {
		if gv, ok := byKey[k]; !ok || gv != wv {
			t.Errorf("resolved[%q] = %q (present=%v), want %q", k, gv, ok, wv)
		}
	}
	if _, ok := byKey["missnodef"]; ok {
		t.Errorf("resolved[missnodef] present, want OMITTED (header absent + no default)")
	}
	if len(got) != 4 {
		t.Errorf("len(resolved) = %d, want 4 (missnodef omitted)", len(got))
	}
}

// TestResolveCustomTagsNilLookup: a nil headerLookup (no request headers available)
// makes every request_header spec use its default / omit; literals are unaffected.
func TestResolveCustomTagsNilLookup(t *testing.T) {
	specs := []CustomTagSpec{
		{Key: "lit", Kind: kindLiteral, LiteralValue: "L"},
		{Key: "hdrdef", Kind: kindRequestHeader, HeaderName: "x", DefaultValue: "D", HasDefault: true},
		{Key: "hdrnodef", Kind: kindRequestHeader, HeaderName: "y"},
	}
	got := ResolveCustomTags(specs, nil)
	byKey := map[string]string{}
	for _, kv := range got {
		byKey[kv.Key] = kv.Str
	}
	if byKey["lit"] != "L" {
		t.Errorf("lit = %q, want L", byKey["lit"])
	}
	if byKey["hdrdef"] != "D" {
		t.Errorf("hdrdef = %q, want D (nil lookup → default)", byKey["hdrdef"])
	}
	if _, ok := byKey["hdrnodef"]; ok {
		t.Errorf("hdrnodef present, want OMITTED (nil lookup + no default)")
	}
}

// TestResolveCustomTagsEmptyPresentHeader: an EXISTING header with an empty value
// emits a present empty-string tag (NOT the default) — presence is the
// discriminator (SPEC §2 modeled edge; the lookup's bool is true for a
// present-but-empty header).
func TestResolveCustomTagsEmptyPresentHeader(t *testing.T) {
	specs := []CustomTagSpec{
		{Key: "e", Kind: kindRequestHeader, HeaderName: "x-empty", DefaultValue: "DEF", HasDefault: true},
	}
	lookup := lookupFunc(map[string][]string{"x-empty": {""}}) // present, empty value
	got := ResolveCustomTags(specs, lookup)
	if len(got) != 1 || got[0].Key != "e" || got[0].Str != "" {
		t.Errorf("resolved = %+v, want one {e, \"\"} (present empty, not the default)", got)
	}
}

// TestResolveCustomTagsEmpty: no specs → nil (byte-stable no-tags path).
func TestResolveCustomTagsEmpty(t *testing.T) {
	if got := ResolveCustomTags(nil, lookupFunc(nil)); got != nil {
		t.Errorf("ResolveCustomTags(nil, ...) = %+v, want nil", got)
	}
}
```

- [ ] **Step 3: Run — expect FAIL** (the types exist from Step 1 but `ResolveCustomTags` is undefined):

```
cd internal/tracing && go test -run 'TestResolveCustomTags' -count=1 .
```
Expected: FAIL — `undefined: ResolveCustomTags`.

- [ ] **Step 4: Implement `ResolveCustomTags`** in a NEW file `internal/tracing/resolve.go`:

```go
package tracing

// ResolveCustomTags resolves the ordered (already first-wins-deduped) specs
// against a per-request header lookup into span attributes. A literal spec yields
// its static value; a request_header spec yields the FIRST value of the named
// header (SPEC-62 §11 D-RH-MULTIVALUE), or the DefaultValue when the header is
// absent and a default was configured, or NOTHING when the header is absent and no
// default was set (omit-on-missing — D-RH-MISSING). headerLookup may be nil (no
// request headers available), in which case request_header specs use default /
// omit. The returned []KV has unique keys (the specs were deduped at parse), so
// BuildServerSpan's upsert only ever overrides a colliding BUILT-IN.
func ResolveCustomTags(specs []CustomTagSpec, headerLookup func(string) ([]string, bool)) []KV {
	if len(specs) == 0 {
		return nil
	}
	out := make([]KV, 0, len(specs))
	for _, s := range specs {
		switch s.Kind {
		case kindLiteral:
			out = append(out, KV{Key: s.Key, Str: s.LiteralValue})
		case kindRequestHeader:
			if headerLookup != nil {
				// The lookup's bool is TRUE for a present header even with an empty
				// value, so a present empty-valued header emits KV{Key, ""} (present),
				// NOT the default (SPEC §2 modeled edge).
				if vs, ok := headerLookup(s.HeaderName); ok && len(vs) > 0 {
					out = append(out, KV{Key: s.Key, Str: vs[0]}) // FIRST value
					continue
				}
			}
			if s.HasDefault {
				out = append(out, KV{Key: s.Key, Str: s.DefaultValue})
			} // else omit (append nothing)
		}
	}
	return out
}
```

- [ ] **Step 5: Run — expect PASS:**

```
cd internal/tracing && go test -run 'TestResolveCustomTags' -count=1 . && go build ./...
```
Expected: PASS (build stays green — the new symbols are only referenced by the new test).

- [ ] **Step 6: LIVENESS BREAKS (`-count=1`, confirm WHICH fires).** Prove three load-bearing arms are live:
  1. **First-value (multi-value):** change `vs[0]` → `vs[len(vs)-1]` and confirm `TestResolveCustomTagsMatrix` fires on `resolved["multi"] = "MV-B", want "MV-A"` (NOT another row). Restore.
  2. **Omit-on-missing:** change the `else omit` to always append `KV{Key: s.Key, Str: s.DefaultValue}` (drop the `if s.HasDefault`) and confirm `TestResolveCustomTagsMatrix` fires on `resolved[missnodef] present, want OMITTED` + `len == 5, want 4`. Restore.
  3. **Present-empty-not-default:** change the present branch guard from `ok && len(vs) > 0` to `ok && len(vs) > 0 && vs[0] != ""` and confirm `TestResolveCustomTagsEmptyPresentHeader` fires (`want one {e, ""}` — got the default). Restore.

```
cd internal/tracing && go test -run 'TestResolveCustomTags' -count=1 -v .
```

- [ ] **Step 7: Per-task gates + commit:**

```
gofmt -l internal/tracing/config.go internal/tracing/resolve.go internal/tracing/resolve_test.go   # expect no output
golangci-lint run ./internal/tracing/... && go vet ./internal/tracing/...
go build ./... && cd internal/tracing && go test -count=1 . && cd -
git add internal/tracing/config.go internal/tracing/resolve.go internal/tracing/resolve_test.go
git commit -m "phase 62 IMPL T1: CustomTagSpec/customTagKind types + ResolveCustomTags resolver + matrix test"
```

---

## Task 2: `config.go` — reshape `parseCustomTags` (request_header accept + empty-name reject + first-wins dedup) + field type + thread the 3 call sites

**Files:**
- Modify: `internal/tracing/config.go` (field `:33-37`; `parseCustomTags` `:139-166`; `cfg.CustomTags = customTags` `:127`)
- Modify: `internal/tracing/config_test.go` (accept tests `:406-436`; reject arms `:440-493`)
- Modify: `internal/filter/hcm/accesslog_emit.go` (`:55` H1, `:116` H2, `:177` H3)

**Interfaces:**
- Consumes: `CustomTagSpec`/`customTagKind` + `ResolveCustomTags` (Task 1); `reqHeaderLookupH1`/`reqHeaderLookupH2` (existing, `accesslog_emit.go:218`/`:228`).
- Produces: `parseCustomTags(tags []*tracingv3.CustomTag) ([]CustomTagSpec, error)` (reshaped); `TracingConfig.CustomTags []CustomTagSpec` (field type change).

- [ ] **Step 1: Change the field type** — `config.go:33-37`, replace `CustomTags []KV` (keep the comment intent, update wording):

```go
	// CustomTags are the parsed custom tags (provider-neutral), ORDERED and
	// first-wins-deduplicated by tag key at parse (matching the reference's
	// config-time map). ResolveCustomTags resolves them per-request into span
	// attributes appended by BuildServerSpan (each overriding a colliding built-in).
	// Empty/nil when the HCM tracing block configures no custom_tags — the
	// byte-stable no-tags path.
	CustomTags []CustomTagSpec
```

- [ ] **Step 2: Update the two accept tests + the reject arms** in `config_test.go`.

**(a)** `TestNewConfigAcceptCustomTagLiteral` (`:406`) — replace the final assertion block (`:413-418`) to the spec shape:

```go
	if len(cfg.CustomTags) != 1 {
		t.Fatalf("CustomTags len = %d, want 1", len(cfg.CustomTags))
	}
	if got := cfg.CustomTags[0]; got.Key != "env" || got.Kind != kindLiteral || got.LiteralValue != "prod" {
		t.Errorf("CustomTags[0] = %+v, want {Key:env Kind:literal LiteralValue:prod}", got)
	}
```

**(b)** `TestNewConfigAcceptCustomTagLiteralZipkin` (`:423`) — replace the final assertion (`:433-435`):

```go
	if len(cfg.CustomTags) != 1 || cfg.CustomTags[0].Key != "env" ||
		cfg.CustomTags[0].Kind != kindLiteral || cfg.CustomTags[0].LiteralValue != "prod" {
		t.Fatalf("CustomTags = %+v, want one literal {env,prod}", cfg.CustomTags)
	}
```

**(c)** In `TestNewConfigRejectCustomTagArms` (`:440`), DELETE the `request_header` reject row (`:456-460`) and CHANGE the `empty-name` reject in its place. Replace that one struct-literal row with:

```go
		{
			name:    "request_header-empty-name",
			tag:     &typetracingv3.CustomTag{Tag: "h", Type: &typetracingv3.CustomTag_RequestHeader{RequestHeader: &typetracingv3.CustomTag_Header{Name: ""}}},
			wantSub: "request_header tag \"h\" empty name",
		},
```

(The `environment`, `metadata`, `empty-tag`, `empty-literal-value`, `typeless` rows STAY unchanged.)

**(d)** ADD two new tests after `TestNewConfigRejectCustomTagArms` (end of file):

```go
// TestNewConfigAcceptCustomTagRequestHeader: a request_header custom tag parses
// into a CustomTagSpec carrying the header name + default (HasDefault derived from
// a non-empty default_value).
func TestNewConfigAcceptCustomTagRequestHeader(t *testing.T) {
	tr := otelProvider(t, envoyGrpcOTel("c", "svc"))
	tr.CustomTags = []*typetracingv3.CustomTag{
		{Tag: "user", Type: &typetracingv3.CustomTag_RequestHeader{RequestHeader: &typetracingv3.CustomTag_Header{Name: "x-user", DefaultValue: "anon"}}},
		{Tag: "bare", Type: &typetracingv3.CustomTag_RequestHeader{RequestHeader: &typetracingv3.CustomTag_Header{Name: "x-bare"}}}, // no default
	}
	cfg, err := NewConfig(tr)
	if err != nil {
		t.Fatalf("NewConfig accept request_header: unexpected err %v", err)
	}
	if len(cfg.CustomTags) != 2 {
		t.Fatalf("CustomTags len = %d, want 2", len(cfg.CustomTags))
	}
	if got := cfg.CustomTags[0]; got.Key != "user" || got.Kind != kindRequestHeader ||
		got.HeaderName != "x-user" || got.DefaultValue != "anon" || !got.HasDefault {
		t.Errorf("CustomTags[0] = %+v, want request_header {user,x-user,anon,HasDefault}", got)
	}
	if got := cfg.CustomTags[1]; got.Key != "bare" || got.Kind != kindRequestHeader ||
		got.HeaderName != "x-bare" || got.HasDefault {
		t.Errorf("CustomTags[1] = %+v, want request_header {bare,x-bare,no-default}", got)
	}
}

// TestNewConfigCustomTagFirstWinsDedup: two custom tags with the SAME key keep the
// FIRST in config order (Envoy's config-time map insert-if-absent, SPEC-62 §11
// arms C/D), regardless of source type.
func TestNewConfigCustomTagFirstWinsDedup(t *testing.T) {
	tr := otelProvider(t, envoyGrpcOTel("c", "svc"))
	tr.CustomTags = []*typetracingv3.CustomTag{
		customTagLiteral("dup", "LIT-VAL"),
		{Tag: "dup", Type: &typetracingv3.CustomTag_RequestHeader{RequestHeader: &typetracingv3.CustomTag_Header{Name: "x-dup"}}},
	}
	cfg, err := NewConfig(tr)
	if err != nil {
		t.Fatalf("NewConfig dedup: unexpected err %v", err)
	}
	if len(cfg.CustomTags) != 1 {
		t.Fatalf("CustomTags len = %d, want 1 (first-wins dedup)", len(cfg.CustomTags))
	}
	if got := cfg.CustomTags[0]; got.Key != "dup" || got.Kind != kindLiteral || got.LiteralValue != "LIT-VAL" {
		t.Errorf("CustomTags[0] = %+v, want the FIRST (literal LIT-VAL)", got)
	}
}
```

- [ ] **Step 3: Run the tests — expect FAIL** (field type / parse not yet reshaped; the code will not even compile until Step 4/5, so this is a compile-fail red):

```
cd internal/tracing && go test -run 'TestNewConfig.*CustomTag' -count=1 .
```
Expected: FAIL — build error (`cfg.CustomTags[0].Kind` undefined on `[]KV`; the accept test references spec fields) OR, once the field type changes, the request_header arm still rejects.

- [ ] **Step 4: Reshape `parseCustomTags`** — replace the whole function (`config.go:139-166`) with:

```go
// parseCustomTags converts the HCM tracing custom_tags into an ORDERED, first-wins-
// deduplicated []CustomTagSpec. Two source types are supported: `literal` (a static
// {tag, value} STRING attribute) and `request_header` (the FIRST value of a named
// downstream request header, resolved per-request by ResolveCustomTags, with an
// optional default / omit-on-missing). `environment`/`metadata` are STRICT-REJECTED
// loudly (envoy-go-strict DEPARTURE, ADR-0080 — the reference accepts them). PGV-
// parity structural rejects (empty tag / empty literal.value / empty
// request_header.name / typeless) mirror the reference boot-reject (both reject —
// NOT a departure). All substrings are ADR-0080-distinct. Dedup runs AFTER per-tag
// structural validation, so a later duplicate-key tag with an invalid name still
// boot-rejects (parity with the reference PGV, which validates every entry before
// building the map). First-wins: the FIRST tag with a given key survives; a later
// same-key tag of ANY source type is dropped (SPEC-62 §11 arms C/D).
func parseCustomTags(tags []*tracingv3.CustomTag) ([]CustomTagSpec, error) {
	if len(tags) == 0 {
		return nil, nil
	}
	out := make([]CustomTagSpec, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, ct := range tags {
		tag := ct.GetTag()
		if tag == "" {
			return nil, fmt.Errorf("tracing: custom_tags empty tag")
		}
		var spec CustomTagSpec
		switch {
		case ct.GetLiteral() != nil:
			v := ct.GetLiteral().GetValue()
			if v == "" {
				return nil, fmt.Errorf("tracing: custom_tags literal tag %q empty value", tag)
			}
			spec = CustomTagSpec{Key: tag, Kind: kindLiteral, LiteralValue: v}
		case ct.GetRequestHeader() != nil:
			h := ct.GetRequestHeader()
			if h.GetName() == "" {
				return nil, fmt.Errorf("tracing: custom_tags request_header tag %q empty name", tag)
			}
			dv := h.GetDefaultValue()
			spec = CustomTagSpec{Key: tag, Kind: kindRequestHeader, HeaderName: h.GetName(), DefaultValue: dv, HasDefault: dv != ""}
		case ct.GetEnvironment() != nil:
			return nil, fmt.Errorf("tracing: custom_tags environment type unsupported")
		case ct.GetMetadata() != nil:
			return nil, fmt.Errorf("tracing: custom_tags metadata type unsupported")
		default:
			return nil, fmt.Errorf("tracing: custom_tags tag %q missing type", tag)
		}
		if _, dup := seen[tag]; dup {
			continue // first-wins: drop a later same-key tag (validated above)
		}
		seen[tag] = struct{}{}
		out = append(out, spec)
	}
	return out, nil
}
```

- [ ] **Step 5: Thread the three call sites** in `accesslog_emit.go` so the build compiles with the new field type. Replace `f.tracingConfig.CustomTags` with the resolver call at each site:

`accesslog_emit.go:55` (H1, `r *http.Request`):
```go
		f.exporter.Export(tracing.BuildServerSpan(*traceDecision, in, tracing.ResolveCustomTags(f.tracingConfig.CustomTags, reqHeaderLookupH1(r)), start, time.Now()))
```

`accesslog_emit.go:116` (H2, `req h2.H2Request`):
```go
		f.exporter.Export(tracing.BuildServerSpan(*traceDecision, in, tracing.ResolveCustomTags(f.tracingConfig.CustomTags, reqHeaderLookupH2(req)), start, time.Now()))
```

`accesslog_emit.go:177` (H3, `r *http.Request` — reuses the H1 lookup):
```go
		f.exporter.Export(tracing.BuildServerSpan(*traceDecision, in, tracing.ResolveCustomTags(f.tracingConfig.CustomTags, reqHeaderLookupH1(r)), start, time.Now()))
```

(Nil-safety RE-DERIVED, unchanged from phase 59: all three sites are guarded by `f.exporter != nil`, and `f.exporter != nil ⟹ f.tracingConfig != nil`; the lookup constructors are pure. When `CustomTags` is empty the resolver returns `nil` and the upsert loop is a no-op — byte-stable.)

- [ ] **Step 6: Run tests — expect PASS:**

```
cd internal/tracing && go test -count=1 . && cd - && go build ./... && go test -count=1 ./internal/filter/hcm/...
```
Expected: PASS — all `internal/tracing` tests (incl. the reshaped accept/reject/dedup tests + the unchanged `TestNewConfigRejectArms` typeless `custom_tags` row) + a clean build (the three threaded call sites compile) + the hcm package tests green.

- [ ] **Step 7: LIVENESS BREAKS (`-count=1`, confirm WHICH fires).**
  1. **request_header accept:** in `parseCustomTags`, corrupt the request_header spec (e.g. `HeaderName: h.GetName()` → `HeaderName: "WRONG"`) and confirm `TestNewConfigAcceptCustomTagRequestHeader` fires on `CustomTags[0] = ... want ...x-user...`. Restore.
  2. **empty-name reject:** change the empty-name substring `"empty name"` → `"XXX"` and confirm ONLY `TestNewConfigRejectCustomTagArms/request_header-empty-name` fires (its `wantSub`), no cross-firing. Restore.
  3. **first-wins dedup:** change `continue` (drop-later) to break the dedup — e.g. remove the `if _, dup := seen[tag]; dup { continue }` guard so BOTH same-key tags append — and confirm `TestNewConfigCustomTagFirstWinsDedup` fires on `CustomTags len = 2, want 1`. Restore.

```
cd internal/tracing && go test -run 'TestNewConfig.*CustomTag' -count=1 -v .
```

- [ ] **Step 8: Per-task gates + commit:**

```
gofmt -l internal/tracing/config.go internal/tracing/config_test.go internal/filter/hcm/accesslog_emit.go   # expect no output
golangci-lint run ./internal/tracing/... ./internal/filter/hcm/... && go vet ./internal/tracing/... ./internal/filter/hcm/...
go build ./... && go test -count=1 ./internal/tracing/... ./internal/filter/hcm/...
git add internal/tracing/config.go internal/tracing/config_test.go internal/filter/hcm/accesslog_emit.go
git commit -m "phase 62 IMPL T2: parseCustomTags request_header accept + empty-name reject + first-wins dedup; []CustomTagSpec field; thread 3 call sites via ResolveCustomTags"
```

---

## Task 3: `span.go`/`BuildServerSpan` — CONFIRM unchanged + arm-B upsert-over-built-in test

**Files:**
- Modify: `internal/tracing/span_test.go` (add ONE test; `BuildServerSpan` signature UNCHANGED — no production edit)

**Interfaces:**
- Consumes: `BuildServerSpan(d, in, customTags []KV, start, end)` (UNCHANGED, phase 59); `freshDecision`/`freshInputs`/`attrStr` (`span_test.go:12`/`:34`/`:298`).

- [ ] **Step 1: CONFIRM `BuildServerSpan`/`upsertAttr` need NO change.** The resolver produces unique keys, so `upsertAttr`'s only job is overriding a colliding built-in — the landed behavior. Verify by reading `span.go:64` (signature still `customTags []KV`) and `:99-101`/`:117-129` (loop + helper). No edit. (This step is a read-confirm; if any signature drift is found, STOP and reconcile — the SPEC asserts NO change.)

- [ ] **Step 2: Write the failing test** in `span_test.go` (append at end). This mirrors the phase-59 `TestSpanCustomTagsUpsertOverridesBuiltin` (`:349`) but frames the input as a RESOLVED request_header KV (the resolver would have produced it), asserting arm B (custom overrides built-in):

```go
// TestSpanResolvedRequestHeaderUpsertsOverBuiltin: a resolved request_header custom
// tag whose key collides with a built-in OVERRIDES it (exactly ONE attribute with
// that key, carrying the resolved header value) — arm B (SPEC-62 §11). The resolver
// hands BuildServerSpan a unique-keyed []KV; upsertAttr overrides the built-in.
func TestSpanResolvedRequestHeaderUpsertsOverBuiltin(t *testing.T) {
	d := freshDecision()
	in := freshInputs() // built-in http.method == "GET"
	start := time.Now()
	end := start.Add(time.Millisecond)
	// A resolved request_header tag: key collides with the built-in http.method.
	s := BuildServerSpan(d, in, []KV{{Key: "http.method", Str: "OVERRIDE-METHOD"}}, start, end)

	n := 0
	for _, kv := range s.Attrs {
		if kv.Key == "http.method" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("http.method attribute count = %d, want 1 (upsert, not append-duplicate)", n)
	}
	if v := attrStr(s.Attrs, "http.method"); v != "OVERRIDE-METHOD" {
		t.Errorf("http.method = %q, want OVERRIDE-METHOD (resolved request_header overrides built-in)", v)
	}
}
```

- [ ] **Step 3: Run — expect PASS** (the phase-59 upsert already implements this):

```
cd internal/tracing && go test -run 'TestSpanResolvedRequestHeaderUpsertsOverBuiltin' -count=1 -v .
```
Expected: PASS.

- [ ] **Step 4: LIVENESS BREAK (`-count=1`, confirm WHICH fires).** Temporarily make `upsertAttr` always append (delete the replace-in-place loop, leaving only `*attrs = append(*attrs, ct)`) and confirm THIS test fires on `count = 2, want 1` (the append-duplicate divergence). Restore. (Isolates to the override property.)

```
cd internal/tracing && go test -run 'TestSpanResolvedRequestHeaderUpsertsOverBuiltin' -count=1 -v .
```

- [ ] **Step 5: Gates + commit:**

```
gofmt -l internal/tracing/span_test.go && golangci-lint run ./internal/tracing/...
cd internal/tracing && go test -count=1 . && cd -
git add internal/tracing/span_test.go
git commit -m "phase 62 IMPL T3: span_test resolved-request_header-upserts-over-builtin (arm B); BuildServerSpan unchanged"
```

---

## Task 4: Zipkin encoder unit test — a resolved request_header tag surfaces in the `tags` map

**Files:**
- Modify: `internal/tracing/zipkin_test.go` (add one test; `encoding/json` already imported)

**Interfaces:**
- Consumes: `BuildServerSpan` (unchanged), `encodeZipkinSpans([]*Span, id128, shared bool)` (`zipkin.go:78`); the existing `freshDecision`/`freshInputs` helpers + the `b[1:len(b)-1]` single-span decode idiom (`zipkin_test.go:90`).

- [ ] **Step 1: Write the failing test** in `zipkin_test.go` (append at end; mirror the phase-59 `custom_env` Zipkin test at `:547`):

```go
// TestZipkinEncodeResolvedRequestHeaderTag: a resolved request_header custom tag
// surfaces in the Zipkin v2 `tags` map (the shared Attrs seam feeds both exporters);
// node_id/zone stay dropped by the encoder.
func TestZipkinEncodeResolvedRequestHeaderTag(t *testing.T) {
	d := freshDecision()
	in := freshInputs()
	in.Authority = "127.0.0.1:10000"
	in.NodeID = "node-x"
	in.Zone = "zone-y"
	start := time.Unix(0, 1_000_000_000)
	end := start.Add(10 * time.Millisecond)
	// The resolver would have produced this KV from {tag: trace_user, request_header:{name: x-trace-user}}.
	span := BuildServerSpan(d, in, []KV{{Key: "trace_user", Str: "u-42"}}, start, end)

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
	if got.Tags["trace_user"] != "u-42" {
		t.Errorf("tags[trace_user] = %q, want u-42", got.Tags["trace_user"])
	}
	if _, ok := got.Tags["node_id"]; ok {
		t.Errorf("tags[node_id] present, want dropped by the Zipkin encoder")
	}
	if _, ok := got.Tags["zone"]; ok {
		t.Errorf("tags[zone] present, want dropped by the Zipkin encoder")
	}
}
```

- [ ] **Step 2: Run — expect PASS** (the tag flows into `Attrs` via the unchanged upsert; `encodeZipkinSpans` builds `tags` from `Attrs`, dropping node_id/zone):

```
cd internal/tracing && go test -run 'TestZipkinEncodeResolvedRequestHeaderTag' -count=1 -v .
```
Expected: PASS.

- [ ] **Step 3: LIVENESS BREAK (`-count=1`).** Temporarily add `trace_user` to the Zipkin encoder's built-in drop condition (`zipkin.go`, the `node_id`/`zone` drop guard — grep for `"node_id"` in `zipkin.go`) and confirm the `tags[trace_user]` assertion fires; restore.

```
cd internal/tracing && go test -run 'TestZipkinEncodeResolvedRequestHeaderTag' -count=1 -v .
```

- [ ] **Step 4: Gates + commit:**

```
gofmt -l internal/tracing/zipkin_test.go && golangci-lint run ./internal/tracing/...
cd internal/tracing && go test -count=1 . && cd -
git add internal/tracing/zipkin_test.go
git commit -m "phase 62 IMPL T4: Zipkin encoder resolved-request_header-tag unit test"
```

---

## Task 5: New OTLP fixture `0105-tracing-custom-tags-request-header`

**Files:**
- Create: `test/fixtures/0105-tracing-custom-tags-request-header/{envoy.yaml,envoy-go.yaml,expectations.yaml,README.md}`
- Create: `test/fixtures/0105-tracing-custom-tags-request-header/driver/driver.go`
- Modify: `test/differential/runner_test.go` (blank-import after the `0104` line `:131`)

**Approach:** CLONE `0102-tracing-custom-tags-literal` verbatim, then apply the enumerated edits. Do NOT mutate `0087`/`0088`/`0102` (`reference_differential_fixture_dispatch_constraint` — one fixture dir = one runner branch). RE-DERIVE the next-free number at implementation: `ls -d test/fixtures/[0-9]*/ | tail -1` — expect `0104-http3-downstream-get`, so `0105` is free.

- [ ] **Step 1: Clone the fixture dir:**

```
cp -r test/fixtures/0102-tracing-custom-tags-literal test/fixtures/0105-tracing-custom-tags-request-header
```

- [ ] **Step 2: Swap the `custom_tags` block in BOTH bootstrap templates.** In `envoy.yaml` (block at `:55-58`) AND the mirror in `envoy-go.yaml`, replace the literal block with a request_header tag (a NON-colliding key), and change `service_name: "0102"` → `"0105"`:

```yaml
                  custom_tags:
                  - tag: trace_user
                    request_header:
                      name: x-trace-user
                      default_value: anon
```
(Verify indentation against each file's existing `custom_tags:` — it sits as a sibling of `provider:`/`random_sampling:` under `tracing:`. The `service_name` appears at `envoy.yaml:52` and inside the OTel `typed_config` in `envoy-go.yaml`; change both.)

- [ ] **Step 3: Edit `driver/driver.go`** — the enumerated changes on the clone:
  1. Package doc (`:1-60`) + `fixtureName` const (`:88`) → `"0105-tracing-custom-tags-request-header"`; reword the doc to describe the request_header tag (present-case cross-side; the driver SENDS `x-trace-user`).
  2. `refListenerPort` (`:93`) → `10105`.
  3. `wantServiceName` (`:118`) → `"0105"`.
  4. The custom-tag consts (`:127-128`) → the request_header expectation:
  ```go
	// phase 62: the request_header custom_tags entry baked into both bootstraps'
	// `tracing` block (sibling of `provider`) — the FIRST value of x-trace-user is
	// asserted as an OTLP span attribute, by key, on EVERY exported span (both sides).
	customTagKey    = "trace_user"
	traceUserHeader = "x-trace-user"
	traceUserValue  = "u-42" // the value the driver SENDS on x-trace-user (present case)
  ```
  Remove the old `customTagValue = "prod-literal"` const.
  5. **Send the header on EVERY driven request.** In `fireProbe` (`:340`), after `req.Header.Set("User-Agent", probeUA)` (`:346`), add:
  ```go
	req.Header.Set(traceUserHeader, traceUserValue)
  ```
  (This is the ONE new driver capability — the `0102`/`0087` drivers issue plain GETs; this driver adds one request header. The `extra` map still overlays the continuation `Traceparent`.)
  6. Update `assertCustomTag` (`:554-566`) to assert the present-case resolved value:
  ```go
// assertCustomTag asserts the phase-62 request_header custom tag on EVERY span,
// cross-side by KEY (OTLP attribute order is non-deterministic — SPEC §11). The
// driver sends x-trace-user: <traceUserValue> on every request, so every span
// carries trace_user == traceUserValue (the present case). Errorf per property so
// one failure does not mask the rest.
func assertCustomTag(t fixture.TB, side string, spans []*tracepb.Span) {
	t.Helper()
	for i, sp := range spans {
		v, ok := spanAttrMap(sp)[customTagKey]
		if !ok {
			t.Errorf("%s span %d: missing custom tag key %q (present: %v)", side, i, customTagKey, mapKeys(spanAttrMap(sp)))
			continue
		}
		if got := v.GetStringValue(); got != traceUserValue {
			t.Errorf("%s span %d: %s = %q, want %q", side, i, customTagKey, got, traceUserValue)
		}
	}
}
  ```
  7. `FIXTURE_0102_DUMP` (`:403`) → `FIXTURE_0105_DUMP`.
  8. The `fixtureDir` comment (`:698`, `.../0102-...`) → `.../0105-tracing-custom-tags-request-header/driver/driver.go`.
  9. Leave the type name `traceOTLPDriver` (packages are isolated; a rename is cosmetic) and the compile-time interface assertions (`:724-728`) as-is.

- [ ] **Step 4: Update `expectations.yaml` + `README.md`** — reflect the new fixture name/purpose (the request_header custom tag `trace_user`, resolved from `x-trace-user`, asserted cross-side by key; the `default_value: anon` + omit edges are the deterministic `resolve_test.go` unit tests, NOT this fixture). Keep the `0102` framing otherwise (the `upstream_cluster` framework-gap note stays; span count unchanged — 12). Grep the cloned files for stray `0102`/`prod-literal`/`custom_env` and reconcile.

- [ ] **Step 5: Register + run the fixture.** Add the blank import to `runner_test.go` immediately AFTER the `0104` line (`:131`):

```go
	_ "github.com/pgdad/envoy-go/test/fixtures/0105-tracing-custom-tags-request-header/driver"
```
Then (Docker required):

```
go test ./test/differential/ -run 'TestDifferential/0105-tracing-custom-tags-request-header' -count=1 -v
```
Expected: PASS (both sides emit `trace_user=u-42`). NOTE the `-run` selector footgun (`reference_differential_run_selector`): use the FULL `TestDifferential/0105-tracing-custom-tags-request-header` form, not a bare `0105` (which matches ZERO subtests → vacuous green).

- [ ] **Step 6: LIVENESS BREAK (`-count=1`, confirm WHICH fires).** Temporarily set `traceUserValue = "WRONG"` (the driver keeps sending `x-trace-user: u-42` — wait, the SAME const feeds both the SEND and the ASSERT, so a value change is vacuous). Instead break ONLY the assertion: temporarily hard-code the compare to a wrong literal, e.g. change `got != traceUserValue` → `got != "WRONG-EXPECT"`, and confirm BOTH sides' `assertCustomTag` fire with `trace_user = "u-42", want "WRONG-EXPECT"` (the custom-tag assertion, not another). Restore. (`reference_vacuous_break_receiver_normalizes` — a break that changes both the sent and expected value fires NOTHING; break the EXPECTATION only.)

```
go test ./test/differential/ -run 'TestDifferential/0105-tracing-custom-tags-request-header' -count=1 -v
```

- [ ] **Step 7: Gates + commit** (Docker required for the differential; if the subagent lacks Docker, the controller runs it at stage-close — note the deferral in the commit):

```
gofmt -l test/fixtures/0105-tracing-custom-tags-request-header/driver/driver.go && golangci-lint run ./test/...
go build ./...
git add test/fixtures/0105-tracing-custom-tags-request-header/ test/differential/runner_test.go
git commit -m "phase 62 IMPL T5: 0105-tracing-custom-tags-request-header OTLP fixture (fixtures 106 -> 107)"
```

---

## Task 6: `FuzzHCMConfigParse` seed — one `request_header` custom_tags seed (fuzzers stay 55)

**Files:**
- Modify: `internal/filter/hcm/fuzz_test.go` (add one seed after the phase-59 seed `:44`; `hcmv3`/`tracingv3` already imported `:12`/`:13`)

- [ ] **Step 1: Reconcile the fuzzer count BEFORE** (`reference_fuzzer_count_docs_drift`):

```
grep -rn '^func Fuzz' --include='*.go' . | wc -l    # expect 55
```

- [ ] **Step 2: Add the seed** in `FuzzHCMConfigParse`, after the phase-59 `withCustomTags` seed (`fuzz_test.go:44`):

```go
	// Phase 62: a request_header custom_tags seed — one accepted `request_header`
	// (name + default) + a mixed literal+request_header config with a duplicate key
	// (exercises the accept arm, the empty-name reject boundary is a unit test, and
	// the first-wins dedup path).
	withReqHeaderTags := mkHCM(func(h *hcmv3.HttpConnectionManager) {
		h.Tracing = &hcmv3.HttpConnectionManager_Tracing{
			CustomTags: []*tracingv3.CustomTag{
				{Tag: "user", Type: &tracingv3.CustomTag_RequestHeader{RequestHeader: &tracingv3.CustomTag_Header{Name: "x-user", DefaultValue: "anon"}}},
				{Tag: "dup", Type: &tracingv3.CustomTag_Literal_{Literal: &tracingv3.CustomTag_Literal{Value: "L"}}},
				{Tag: "dup", Type: &tracingv3.CustomTag_RequestHeader{RequestHeader: &tracingv3.CustomTag_Header{Name: "x-dup"}}},
			},
		}
	})
	f.Add(withReqHeaderTags.GetTypeUrl(), withReqHeaderTags.GetValue())
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
git commit -m "phase 62 IMPL T6: FuzzHCMConfigParse request_header custom_tags seed (fuzzers stay 55)"
```

---

## Task 7: `BEHAVIOR_CONTRACT.md` edits (tracing `custom_tags` clause)

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`

- [ ] **Step 1: RE-DERIVE the exact lines** — the phase-59 IMPL wrote the tracing `custom_tags` clause (~686/739 per SPEC §9, RE-DERIVE against the landed tree):

```
grep -n 'custom_tags\|request_header\|literal.*type is CONSUMED' docs/envoy-go/BEHAVIOR_CONTRACT.md
```

- [ ] **Step 2: Flip `request_header` from STRICT-REJECT to CONSUMED** in the tracing `custom_tags` clause. Amend the phase-59 sentence so:
  - the `literal` type STAYS CONSUMED;
  - `request_header` is now CONSUMED: "the named request header's FIRST value is emitted as a `{tag, value}` STRING span attribute on both exporters; `default_value` on an absent header; OMITTED when absent with no default; FIRST-wins dedup on a duplicate tag key; OVERRIDES a colliding built-in";
  - `environment`/`metadata` STAY STRICT-REJECT (envoy-go-strict departures);
  - ADD the empty-`request_header.name` PGV-parity reject to the structural-reject list.

Also NARROW any Zipkin/deferred bullet that names `custom_tags (request_header/environment/metadata)` → `(environment/metadata)`.

(Exact final wording RE-DERIVED and written against the landed lines — no line number is load-bearing here beyond the grep.)

- [ ] **Step 3: Commit** (docs-only; grep-confirm `request_header` no longer reads as strict-rejected in the tracing clause):

```
git add docs/envoy-go/BEHAVIOR_CONTRACT.md
git commit -m "phase 62 IMPL T7: BEHAVIOR_CONTRACT custom_tags request_header CONSUMED + empty-name reject"
```

---

## Task 8: Verify — six-gate + full 107-dir differential

**Files:** none (verification only).

- [ ] **Step 1: Six-gate on the frozen HEAD:**

```
gofmt -l internal/ test/ cmd/            # expect no output
golangci-lint run ./...
go vet ./...
go build ./...
go mod tidy -diff                        # expect EMPTY (tracingv3/CustomTag_Header is an already-resolved module; re-check `git diff go.mod` per reference_new_subpackage_pulls_transitive_module — no NEW sub-package here, only a new field getter)
go test -race -count=1 ./internal/tracing/... ./internal/filter/hcm/...
```
The `-race` on BOTH `internal/tracing` and `internal/filter/hcm` is required (`reference_detrand_race_catches_protojson_value_substring`, `reference_full_suite_race_after_background_mutator`).

- [ ] **Step 2: Full differential — 107 dirs, byte-stable except `0105`** (Docker; reference `envoyproxy/envoy:contrib-v1.37.2`):

```
go test ./test/differential/ -count=1
```
Expected: `ok`, exit 0. The 106 pre-existing dirs stay byte-stable (no `request_header` custom_tags in their configs; the `0102` literal fixture stays green — a single unique-key literal resolves identically); `0105` is the only new dir. The first-wins-dedup correction is byte-stable for every existing single-key config. If a startup flake appears on an UNRELATED fixture (`subject ready: EOF`), isolate-re-run to discriminate (`reference_differential_fullsuite_startup_flake`).

- [ ] **Step 3: Record the evidence** (the six-gate output + the differential `ok ... exit 0`) in `PROGRESS.md`. No commit (verification task); findings feed Task 9's ADR-0283 §Consequences.

---

## Task 9: ADR-0283 body + STATE + ROADMAP + router roll

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0283)
- Modify: `docs/envoy-go/STATE.md` (active-phase header)
- Modify: `docs/envoy-go/ROADMAP.md` (row 62 → `done`; family prose + LIVE deferred-sentence narrow)
- Modify: `docs/envoy-go/phases/62-tracing-custom-tags-request-header/PROGRESS.md` (close)
- Modify: `next-prompt.txt` (router roll to the NEXT decision — a new BRAINSTORM)

- [ ] **Step 1: Append ADR-0283 §Decision/§Consequences** to `DECISIONS.md` (reproduce the SPEC §13 §Context draft, then add):
  - **§Decision:** the `request_header` custom_tag type CONSUMED; `parseCustomTags` reshaped to `([]CustomTagSpec, error)` with the request_header accept arm + empty-name PGV-parity reject + first-wins dedup by tag key; `TracingConfig.CustomTags` becomes `[]CustomTagSpec`; a per-request `ResolveCustomTags(specs, headerLookup) []KV` (literal→static; request_header→first header value / default / omit) threaded at the THREE `accesslog_emit.go` call sites (H1/H2/H3) via the EXISTING `reqHeaderLookupH1`/`reqHeaderLookupH2`; `BuildServerSpan`/`upsertAttr` UNCHANGED (they now only override built-ins, the resolver having produced unique keys); the first-wins dedup CORRECTS a latent phase-59 divergence (byte-stable single-key common case); `environment`/`metadata` STAY loud strict-reject DEPARTURES; the `CustomTagSpec` model + `ResolveCustomTags` seam FOLDED into ADR-0283 (no separate seam ADR — the phase-59/58 precedent).
  - **§Consequences:** the `0105` OTLP differential (present-case cross-side by key, `Errorf`-per-property, break-proven live); the default/omit/multi-value/present-empty edges + the precedence-dedup + the Zipkin path are deterministic UNIT tests; a `FuzzHCMConfigParse` seed; +0 stats / +1 fixture (`0105`) / +0 fuzzers / +0 packages / +0 modules; the four EMPTY-emitted built-ins (`reference_tracing_upstream_cluster_framework_gap`) UNTOUCHED. Record the six-gate + 107-dir result from Task 8. DECISIONS tail ADR-0282 → **ADR-0283** (next-free ADR-0284).

- [ ] **Step 2: Update `STATE.md`** active-phase header → `phase 62 (tracing-custom-tags-request-header) IMPL done` (lifecycle 3 → done; row 62 → `done`; DECISIONS tail ADR-0282 → ADR-0283). Counts: fixtures 106 → 107; all others +0.

- [ ] **Step 3: Update `ROADMAP.md`** — flip row 62 `in-progress` → `done` (ADR-0106, the SOLE leg — `reference_roadmap_split_phase_row_done`). NARROW the LIVE Observability deferred sentence: `custom_tags (request_header/environment/metadata)` → `custom_tags (environment/metadata)`. Then RE-RUN the sentinel check-(2) grep and confirm EXACTLY ONE live `candidates:` match for the Observability family (`reference_sentinel_deferred_sentence_live_vs_historical` — the live sentence uses "candidates:", not "candidates were:"):

```
grep -oE 'remaining deferred \(not-yet-chartered\) candidates:[^.]*\.' docs/envoy-go/ROADMAP.md
```
Expected: still THREE live "candidates:" sentences total (HTTP/3, xDS, Observability) — the Observability one now names `custom_tags (environment/metadata)` (narrowed), not the full triple.

- [ ] **Step 4: Close `PROGRESS.md`** — mark all tasks `[x]`, record the liveness-break outcomes + the Task-8 verify evidence + the landed task commit shas + the exit counts (mirror the phase-59 PROGRESS close shape).

- [ ] **Step 5: Roll `next-prompt.txt`** to the NEXT router decision — a new BRAINSTORM (the roller self-picks the smallest defensible candidate per the 2026-07-12 standing directive; the sentinel does NOT fire — re-run the three checks mechanically and record they still print: (1) all rows `done` after row 62 flips ⇒ check whether ANY row remains non-`done`; (2) three families still carry candidates; (3) gRPC/Runtime/WASM never opened + Operational-tooling open). Update the STATUS block, the "What THIS session does" section, the counts, and the SENTINEL RE-CHECKED date.

- [ ] **Step 6: Commit** (docs + router are docs-only):

```
git add docs/envoy-go/DECISIONS.md docs/envoy-go/STATE.md docs/envoy-go/ROADMAP.md docs/envoy-go/phases/62-tracing-custom-tags-request-header/PROGRESS.md next-prompt.txt
git commit -m "phase 62 (tracing-custom-tags-request-header) IMPL: ADR-0283 + STATE/ROADMAP row 62 done + router roll"
```

- [ ] **Step 7: Controller stage-close** — squash all task commits into ONE `phase 62 (tracing-custom-tags-request-header) IMPL: ...` commit on `master`, re-run the six-gate + 107-dir differential on the frozen HEAD, then PUSH (`feedback_push_to_origin`, `feedback_subagents_no_push` — only the controller pushes).

---

## Self-Review (checked against SPEC 62 this PLAN session)

**Spec coverage:** SPEC §3.1 parse arm (request_header accept + empty-name reject + first-wins dedup) → Task 2. §3.2 config model (`CustomTagSpec` + field type) → Task 1 (types) + Task 2 (field). §3.3 `ResolveCustomTags` → Task 1. §3.4 call-site threading (H1/H2/H3) → Task 2. §3.5 `BuildServerSpan` unchanged → Task 3 (confirm + arm-B test). §6 reject roster + fuzz seed → Task 2 (rejects) + Task 6 (seed). §7 +0 stats → Task 8 gate. §8 fixture (`0105`) → Task 5; Zipkin unit test → Task 4; the default/omit/multi/dedup edges → Task 1 (`resolve_test.go`) + Task 2 (dedup test). §9 BEHAVIOR_CONTRACT → Task 7. §10 test plan → Tasks 1–6. §13 ADR → Task 9. No gap.

**Placeholder scan:** every code step carries real code; the only "clone-then-edit" is Task 5 (the 728-line `0102` driver), enumerated as explicit edits with the changed assertion function reproduced in full. Task 7's exact BEHAVIOR_CONTRACT wording is deferred to a RE-DERIVE-then-write step (the lines drifted since phase 59) — this is a documented docs-edit, not a code placeholder.

**Type consistency:** `CustomTagSpec{Key, Kind, LiteralValue, HeaderName, DefaultValue, HasDefault}` + `customTagKind` (`kindLiteral`/`kindRequestHeader` — renamed off the SPEC §3.2 draft's `customTagLiteral`/`customTagRequestHeader` to avoid colliding with the existing `config_test.go:397` `customTagLiteral` helper func) used identically in Tasks 1/2/6; `ResolveCustomTags(specs []CustomTagSpec, headerLookup func(string) ([]string, bool)) []KV` used in Task 1 (def) and Task 2 (3 call sites); `parseCustomTags([]*tracingv3.CustomTag) ([]CustomTagSpec, error)`; `TracingConfig.CustomTags []CustomTagSpec`; `BuildServerSpan(d, in, []KV, start, end)` UNCHANGED (Tasks 3/4 pass `[]KV` literals). The proto oneof wrapper names (`CustomTag_RequestHeader` no trailing underscore wrapping `CustomTag_Header`) used correctly in Tasks 2/6 and the config-test rows.

**Ordering / build-green:** Task 1 (resolver + types) is purely additive — build green (nothing references the new symbols but the new test). Task 2 makes the atomic field-type change + parse reshape + 3 call-site threadings + config-test updates in ONE commit so the build never breaks (it consumes Task 1's resolver). Tasks 3/4 depend on nothing new (BuildServerSpan unchanged). Task 5 depends on Tasks 1–2 (the fixture exercises the full path). Task 6 independent. Task 8 gates the frozen tree. Task 9 lands docs + router roll. The `TestNewConfigRejectCustomTagArms/request_header` stale row is explicitly removed in Task 2 (flagged in the roster).
