# PLAN 59 — tracing `custom_tags` (LITERAL tag type only) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Every task is TDD (`superpowers:test-driven-development`): red → green, with a `-count=1` liveness break where an assertion is load-bearing.

**Goal:** Lift the wholesale `custom_tags unsupported` reject in `internal/tracing/config.go` and support the `literal` `CustomTag` type — emitted as a static `{tag, value}` STRING span attribute on BOTH the OTLP and Zipkin exporters — while PARSE-REJECTING the other three types (`request_header`/`environment`/`metadata`) loudly plus three PGV-parity structural rejects.

**Architecture:** A literal-parse loop (`parseCustomTags`) replaces the wholesale reject in `NewConfig`; the parsed `[]KV` is stored on a new `TracingConfig.CustomTags` field (provider-neutral, set after the provider switch — `parseOTel`/`parseZipkin` untouched); `BuildServerSpan` gains a `customTags []KV` parameter and applies each tag by **UPSERT-by-key** (last-write-wins, matching the reference's OVERRIDE-on-collision semantics pinned by the SPEC-59 live probe); the two `accesslog_emit.go` call sites thread `f.tracingConfig.CustomTags`. One new OTLP differential fixture (`0102`) proves the attribute cross-side; the Zipkin path is a unit test.

**Tech Stack:** Go; `github.com/envoyproxy/go-control-plane/envoy/type/tracing/v3` (`CustomTag`, an ALREADY-resolved module — `config_test.go:9` already imports it); the differential harness (`test/differential/fixture`, `test/helpers/otlptrace`); `go-fuzz`-style `testing.F` seed corpus.

## Global Constraints

- **Single stage this session context is the IMPL** — subagent-driven per `feedback_execution_style`/`feedback_subagent_autocommit_claudemd`/`feedback_subagents_no_push`: each task is a fresh subagent that commits LOCALLY only; the controller verifies each commit, squashes at stage-close, re-runs the suite on the frozen HEAD, and pushes.
- **Worktree discipline** (`feedback_git_worktrees`, `feedback_subagent_worktree_path_targeting`): the IMPL runs in `.worktrees/phase-59-impl` (branch `phase-59-tracing-custom-tags-literal-impl`). Pin the canonical worktree root; subagents write worktree-relative paths; the controller verifies the MAIN checkout stays clean.
- **`next-prompt.txt` IS TRACKED** (`reference_next_prompt_tracked_despite_gitignore`) — edit it inside the stage worktree and fold into the squash; locate commits by SUBJECT (`git log --grep`), never by position.
- **ADR-0080** — every reject substring is DISTINCT (all six here are).
- **ADR-0106** — row 59 flips `done` ONLY at this IMPL's six-gate (the SOLE leg).
- **ADR-0044** — ADR-0277 §Decision/§Consequences land at this IMPL (SPEC §13 drafted §Context); DECISIONS tail **ADR-0276 → ADR-0277** (next-free ADR-0278).
- **ADR-0045** — a SINGLE FLAT ROW (8 tasks; escape-valve UNCONSUMED, well under the ~15 ceiling).
- **Per-task gates** (`feedback_pertask_gofmt_lint`): each code task runs `gofmt -l` + `golangci-lint run` on touched packages + `go build ./...` + the touched-package `go test` before its commit.
- **Anticipated counts at IMPL-DONE:** stat surface **1201 (+0)** · fixtures **103 → 104** (`0102`) · fuzzers **54 (+0, seed only)** · BackendKind **38 (+0)** · new Go packages **0** · new go.mod modules **0**.
- **`reference_fatalf_makes_assertions_unreachable`** — in the fixture driver, use `Errorf` per independent property (the built-in 0087 driver uses `Fatalf`; the NEW custom-tag assertion uses `Errorf`).
- **`reference_deliberate_break_wrong_assertion`** — every liveness break must confirm WHICH assertion fired (subtest name / message), not merely that *a* failure occurred.
- **`reference_differential_break_protocol_count1`** — every differential/liveness break runs with `-count=1` (go-test caching serves a stale PASS otherwise).

---

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `internal/tracing/config.go` | `CustomTags []KV` field on `TracingConfig`; `tracingv3` import; `parseCustomTags` helper (6 arms) replacing the wholesale reject; provider-switch restructure to set `CustomTags` | 1 |
| `internal/tracing/config_test.go` | accept-literal test (parsed `CustomTags`); 6 reject sub-tests with distinct substrings | 1 |
| `internal/tracing/span.go` | `customTags []KV` param on `BuildServerSpan`; `upsertAttr` helper; the upsert loop | 2 |
| `internal/tracing/span_test.go` | append case + upsert-override case; 6 call-site signature updates | 2 |
| `internal/filter/hcm/accesslog_emit.go` | thread `f.tracingConfig.CustomTags` at the 2 call sites (`:55` H1, `:116` H2) | 2 |
| `internal/tracing/zipkin_test.go` | literal-tag-in-`tags`-map encoder test; 1 call-site signature update | 2 (call site) + 3 (test) |
| `test/fixtures/0102-tracing-custom-tags-literal/` | new OTLP fixture (envoy.yaml, envoy-go.yaml, expectations.yaml, driver/driver.go, README.md) | 4 |
| `internal/filter/hcm/fuzz_test.go` | one `custom_tags` seed on the existing `FuzzHCMConfigParse` | 5 |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | `:686` strict-reject roster + `:739` Zipkin deferred bullet | 6 |
| `docs/envoy-go/{DECISIONS,STATE,ROADMAP}.md`, `next-prompt.txt` | ADR-0277 body; STATE header; ROADMAP row 59 → done + deferred-sentence narrow; router roll | 8 |

**RE-DERIVED edit-site roster (verified against master tip `36f012cf` this PLAN session, `feedback_brief_citations_not_evidence`):**
- `config.go:24-32` `TracingConfig` struct · `config.go:82-83` wholesale reject · `config.go:89` provider check · `config.go:108-115` provider switch · `config.go:7-15` import block.
- `span.go:64` `BuildServerSpan` signature · `span.go:65` capacity · `span.go:68-88` built-ins · `span.go:91-93` optional guid · `span.go:95-106` return.
- `accesslog_emit.go:55` (H1) + `:116` (H2) call sites; nil-safety invariant `f.exporter != nil ⟹ f.tracingConfig != nil` (`hcm/config.go:330-338` exporter set only inside `if tcfg != nil`; `:356` `tracingConfig: tcfg`).
- Test call sites: `span_test.go:60,151,178,193,207,283`; `zipkin_test.go:88`.
- `config_test.go`: existing `TestNewConfigRejectArms` `custom_tags` row at `:319-324` (NOTE below); helpers `otelProvider` `:63`, `envoyGrpcOTel` `:77`; existing `typetracingv3` import `:9`.
- `span_test.go`: helpers `attrStr` `:298`, `hasAttr` `:307`, `freshDecision`/`freshInputs`.
- `fuzz_test.go:25` `FuzzHCMConfigParse`, seeds `:26-29`; `mkHCM` at `config_test.go:44`.
- `BEHAVIOR_CONTRACT.md:686` (strict-reject roster) + `:739` (Zipkin "Does not yet apply to" bullet).
- `ROADMAP.md:121` (row 59) + `:181` (family prose + LIVE deferred sentence).
- `STATE.md:7` (active-phase header). `DECISIONS.md` tail ADR-0276 at `:16622`.

**⚠️ Proto oneof Go-type footgun (verified in `type/tracing/v3/custom_tag.pb.go` @ v1.32.4):** the oneof wrappers are `CustomTag_Literal_`, `CustomTag_Environment_`, `CustomTag_Metadata_` (TRAILING underscore) but `CustomTag_RequestHeader` (NO trailing underscore — it wraps the differently-named message `CustomTag_Header`). Getters: `GetTag()`, `GetLiteral() *CustomTag_Literal`, `GetRequestHeader() *CustomTag_Header`, `GetEnvironment()`, `GetMetadata()`, `CustomTag_Literal.GetValue() string`.

**⚠️ Existing `custom_tags` reject row (`config_test.go:319-324`):** it builds `tr.CustomTags = []*typetracingv3.CustomTag{{Tag: "x"}}` (non-empty tag, NO type) and the table loop asserts only `err != nil` (no substring). Under the new code `{Tag:"x"}` is TYPELESS → hits the `missing type` arm → STILL errors → this row STAYS GREEN unchanged. Leave it as-is (a valid typeless reject); the NEW dedicated tests in Task 1 carry the distinct-substring assertions. Do NOT delete it and do NOT rely on it to prove the new arms.

---

## Task 1: `config.go` — `CustomTags` field + `parseCustomTags` (6 arms) + provider-switch restructure

**Files:**
- Modify: `internal/tracing/config.go` (import block `:7-15`; struct `:24-32`; reject `:82-83`; switch `:108-115`)
- Test: `internal/tracing/config_test.go`

**Interfaces:**
- Produces: `TracingConfig.CustomTags []KV` (field); `parseCustomTags(tags []*tracingv3.CustomTag) ([]KV, error)` (package-private helper). `KV` is the existing `span.go:12` type — no new type.
- Consumes: `tracingv3.CustomTag` getters (see footgun box).

- [ ] **Step 1: Add the `tracingv3` import** to `config.go:7-15`:

```go
import (
	"fmt"

	tracev3 "github.com/envoyproxy/go-control-plane/envoy/config/trace/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	tracingv3 "github.com/envoyproxy/go-control-plane/envoy/type/tracing/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)
```

- [ ] **Step 2: Add the `CustomTags` field** to the `TracingConfig` struct (`config.go:24-32`), after `Zipkin`:

```go
	Zipkin          *ZipkinSettings // non-nil iff Provider == ProviderZipkin
	// CustomTags are the parsed `literal` custom tags (provider-neutral), appended
	// by BuildServerSpan as UPSERT-by-key span attributes (last-write-wins on a
	// built-in-key collision, matching the reference). Empty/nil when the HCM tracing
	// block configures no custom_tags — the byte-stable no-tags path.
	CustomTags []KV
```

- [ ] **Step 3: Write the failing accept + reject tests** in `config_test.go` (append at end of file). The accept test asserts on the parsed `CustomTags`; the reject table asserts each distinct substring.

```go
// customTagLiteral builds a *tracingv3.CustomTag of the `literal` type.
func customTagLiteral(tag, value string) *tracingv3.CustomTag {
	return &tracingv3.CustomTag{
		Tag:  tag,
		Type: &tracingv3.CustomTag_Literal_{Literal: &tracingv3.CustomTag_Literal{Value: value}},
	}
}

// TestNewConfigAcceptCustomTagLiteral: a literal custom tag parses into
// TracingConfig.CustomTags as a {Key,Str} KV on the OTel provider path.
func TestNewConfigAcceptCustomTagLiteral(t *testing.T) {
	tr := otelProvider(t, envoyGrpcOTel("c", "svc"))
	tr.CustomTags = []*tracingv3.CustomTag{customTagLiteral("env", "prod")}
	cfg, err := NewConfig(tr)
	if err != nil {
		t.Fatalf("NewConfig accept literal: unexpected err %v", err)
	}
	if len(cfg.CustomTags) != 1 {
		t.Fatalf("CustomTags len = %d, want 1", len(cfg.CustomTags))
	}
	if got := cfg.CustomTags[0]; got.Key != "env" || got.Str != "prod" || got.IsInt {
		t.Errorf("CustomTags[0] = %+v, want {Key:env Str:prod IsInt:false}", got)
	}
}

// TestNewConfigAcceptCustomTagLiteralZipkin: the same literal tag also parses on
// the Zipkin provider path (provider-neutral parse, set after the switch).
func TestNewConfigAcceptCustomTagLiteralZipkin(t *testing.T) {
	tr := zipkinProvider(t, &tracev3.ZipkinConfig{
		CollectorCluster:         "z",
		CollectorEndpointVersion: tracev3.ZipkinConfig_HTTP_JSON,
	})
	tr.CustomTags = []*tracingv3.CustomTag{customTagLiteral("env", "prod")}
	cfg, err := NewConfig(tr)
	if err != nil {
		t.Fatalf("NewConfig accept literal (zipkin): unexpected err %v", err)
	}
	if len(cfg.CustomTags) != 1 || cfg.CustomTags[0].Key != "env" || cfg.CustomTags[0].Str != "prod" {
		t.Fatalf("CustomTags = %+v, want one {env,prod}", cfg.CustomTags)
	}
}

// TestNewConfigRejectCustomTagArms: each unsupported / structurally-invalid
// custom tag rejects with its ADR-0080-distinct substring.
func TestNewConfigRejectCustomTagArms(t *testing.T) {
	tests := []struct {
		name    string
		tag     *tracingv3.CustomTag
		wantSub string
	}{
		{
			name:    "empty-tag",
			tag:     customTagLiteral("", "v"),
			wantSub: "custom_tags empty tag",
		},
		{
			name:    "empty-literal-value",
			tag:     customTagLiteral("env", ""),
			wantSub: "empty value",
		},
		{
			name:    "request_header",
			tag:     &tracingv3.CustomTag{Tag: "h", Type: &tracingv3.CustomTag_RequestHeader{RequestHeader: &tracingv3.CustomTag_Header{Name: "x-h"}}},
			wantSub: "request_header type unsupported",
		},
		{
			name:    "environment",
			tag:     &tracingv3.CustomTag{Tag: "e", Type: &tracingv3.CustomTag_Environment_{Environment: &tracingv3.CustomTag_Environment{Name: "E"}}},
			wantSub: "environment type unsupported",
		},
		{
			name:    "metadata",
			tag:     &tracingv3.CustomTag{Tag: "m", Type: &tracingv3.CustomTag_Metadata_{Metadata: &tracingv3.CustomTag_Metadata{}}},
			wantSub: "metadata type unsupported",
		},
		{
			name:    "typeless",
			tag:     &tracingv3.CustomTag{Tag: "t"}, // no Type oneof set
			wantSub: "missing type",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := otelProvider(t, envoyGrpcOTel("c", "svc"))
			tr.CustomTags = []*tracingv3.CustomTag{tc.tag}
			got, err := NewConfig(tr)
			if err == nil {
				t.Fatalf("NewConfig(%s) err = nil, want reject; got %+v", tc.name, got)
			}
			if got != nil {
				t.Fatalf("NewConfig(%s) returned non-nil config on reject: %+v", tc.name, got)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("NewConfig(%s) err = %q, want substring %q", tc.name, err.Error(), tc.wantSub)
			}
		})
	}
}
```

Add `"strings"` to the `config_test.go` import block if absent. The `tracingv3` alias must match the production import; the test file currently aliases it `typetracingv3` (`:9`) — either reuse `typetracingv3` throughout these new tests OR add a second alias. **Reuse the existing `typetracingv3` alias** in the test code above (replace `tracingv3.` with `typetracingv3.` in `config_test.go`) to avoid a duplicate import; keep `tracingv3` as the PRODUCTION alias in `config.go`.

- [ ] **Step 4: Run the tests — expect FAIL** (field/parse not yet implemented):

```
cd internal/tracing && go test -run 'TestNewConfig(Accept|Reject)CustomTag' -count=1 .
```
Expected: FAIL — accept tests see `custom_tags unsupported` error; reject tests see the wholesale message, not the distinct substrings.

- [ ] **Step 5: Implement `parseCustomTags`** — add after `NewConfig` (or before `parseOTel`) in `config.go`:

```go
// parseCustomTags converts the HCM tracing custom_tags into a provider-neutral
// []KV. Only the `literal` CustomTag type is supported (a static {tag, value}
// STRING span attribute); request_header/environment/metadata are STRICT-REJECTED
// loudly (envoy-go-strict DEPARTURE, ADR-0080 — the reference accepts them). Three
// PGV-parity structural rejects (empty tag / empty literal.value / typeless) mirror
// the reference boot-reject (both reject — NOT a departure). All six substrings are
// ADR-0080-distinct. The empty-tag check precedes the type dispatch (the reference
// PGV evaluates `tag` min_len before the required `type` oneof).
func parseCustomTags(tags []*tracingv3.CustomTag) ([]KV, error) {
	if len(tags) == 0 {
		return nil, nil
	}
	out := make([]KV, 0, len(tags))
	for _, ct := range tags {
		if ct.GetTag() == "" {
			return nil, fmt.Errorf("tracing: custom_tags empty tag")
		}
		switch {
		case ct.GetLiteral() != nil:
			v := ct.GetLiteral().GetValue()
			if v == "" {
				return nil, fmt.Errorf("tracing: custom_tags literal tag %q empty value", ct.GetTag())
			}
			out = append(out, KV{Key: ct.GetTag(), Str: v})
		case ct.GetRequestHeader() != nil:
			return nil, fmt.Errorf("tracing: custom_tags request_header type unsupported")
		case ct.GetEnvironment() != nil:
			return nil, fmt.Errorf("tracing: custom_tags environment type unsupported")
		case ct.GetMetadata() != nil:
			return nil, fmt.Errorf("tracing: custom_tags metadata type unsupported")
		default:
			return nil, fmt.Errorf("tracing: custom_tags tag %q missing type", ct.GetTag())
		}
	}
	return out, nil
}
```

- [ ] **Step 6: Wire it into `NewConfig`** — replace the wholesale reject (`config.go:82-83`):

```go
	customTags, err := parseCustomTags(t.GetCustomTags())
	if err != nil {
		return nil, err
	}
```
(This parses BEFORE the provider check `:89`, preserving the "reached regardless of provider" property the fuzz seed relies on — SPEC §6.)

Then restructure the provider switch (`config.go:108-115`) to capture the parsed config and set `CustomTags` before returning:

```go
	var cfg *TracingConfig
	switch tc.MessageName() {
	case otelTypeName:
		cfg, err = parseOTel(tc, clientSampling, randomSampling, overallSampling)
	case zipkinTypeName:
		cfg, err = parseZipkin(tc, clientSampling, randomSampling, overallSampling)
	default:
		return nil, fmt.Errorf("tracing: provider %s unsupported (only OpenTelemetry or Zipkin)", tc.GetTypeUrl())
	}
	if err != nil {
		return nil, err
	}
	cfg.CustomTags = customTags
	return cfg, nil
```
`parseOTel`/`parseZipkin` signatures are UNCHANGED. `err` is already declared by the `customTags, err :=` line above, so the switch uses `cfg, err =` (assignment).

- [ ] **Step 7: Run tests — expect PASS:**

```
cd internal/tracing && go test -run 'TestNewConfig' -count=1 . && go test -count=1 .
```
Expected: PASS (all NewConfig tests incl. the unchanged `TestNewConfigRejectArms` typeless `custom_tags` row).

- [ ] **Step 8: LIVENESS BREAKS (`-count=1`, confirm WHICH fires).** For EACH of the 6 reject arms, temporarily corrupt exactly that arm's substring (e.g. change `"request_header type unsupported"` → `"XXX"`) and confirm ONLY that subtest's `wantSub` assertion fires (subtest name `TestNewConfigRejectCustomTagArms/request_header`), then restore. Also break the accept: change `out = append(...)` to append a wrong `Str` and confirm `TestNewConfigAcceptCustomTagLiteral` fires. Restore all. Run e.g.:

```
cd internal/tracing && go test -run 'TestNewConfigRejectCustomTagArms/request_header' -count=1 -v .
```

- [ ] **Step 9: Per-task gates + commit:**

```
gofmt -l internal/tracing/config.go internal/tracing/config_test.go        # expect no output
golangci-lint run ./internal/tracing/... && go vet ./internal/tracing/...
go build ./... && cd internal/tracing && go test -count=1 . && cd -
git add internal/tracing/config.go internal/tracing/config_test.go
git commit -m "phase 59 IMPL T1: custom_tags literal parse + 6 reject arms + CustomTags field"
```

---

## Task 2: `span.go` — `customTags` param + `upsertAttr` + thread both call sites

**Files:**
- Modify: `internal/tracing/span.go` (`BuildServerSpan` `:64`, capacity `:65`, append the upsert loop; add `upsertAttr`)
- Modify: `internal/filter/hcm/accesslog_emit.go` (`:55` H1, `:116` H2)
- Modify: `internal/tracing/span_test.go` (6 call sites `:60,151,178,193,207,283`; 2 new tests)
- Modify: `internal/tracing/zipkin_test.go` (1 call site `:88`)

**Interfaces:**
- Consumes: `TracingConfig.CustomTags` (Task 1).
- Produces: `BuildServerSpan(d Decision, in SpanInputs, customTags []KV, start, end time.Time) *Span` (NEW signature); `upsertAttr(attrs *[]KV, ct KV)` (package-private).

- [ ] **Step 1: Update ALL call sites first** (so the build compiles once the signature changes). Test call sites — pass `nil` (custom tags irrelevant to those tests): `span_test.go:60,151,178,193,207,283` and `zipkin_test.go:88` each `BuildServerSpan(d, in, start, end)` → `BuildServerSpan(d, in, nil, start, end)`. Production call sites — thread `f.tracingConfig.CustomTags`:

`accesslog_emit.go:55` and `:116` (both identical form):
```go
	f.exporter.Export(tracing.BuildServerSpan(*traceDecision, in, f.tracingConfig.CustomTags, start, time.Now()))
```
(Nil-safe: both sites are guarded by `f.exporter != nil`, and `f.exporter != nil ⟹ f.tracingConfig != nil` per `hcm/config.go:330-338`/`:356`. An empty/nil `CustomTags` makes the upsert loop a no-op — byte-stable.)

- [ ] **Step 2: Write the failing span tests** in `span_test.go` (append at end):

```go
// TestSpanCustomTagsAppend: a NON-colliding literal custom tag appears in Attrs
// after the built-ins; the built-ins are untouched.
func TestSpanCustomTagsAppend(t *testing.T) {
	d := freshDecision()
	in := freshInputs()
	start := time.Now()
	end := start.Add(time.Millisecond)
	s := BuildServerSpan(d, in, []KV{{Key: "custom_env", Str: "prod-literal"}}, start, end)

	if v := attrStr(s.Attrs, "custom_env"); v != "prod-literal" {
		t.Errorf("custom_env = %q, want prod-literal", v)
	}
	if v := attrStr(s.Attrs, "http.method"); v != "GET" {
		t.Errorf("built-in http.method = %q, want GET (unperturbed)", v)
	}
}

// TestSpanCustomTagsUpsertOverridesBuiltin: a literal tag whose key collides with
// a built-in OVERRIDES it (last-write-wins) — exactly ONE attribute with that key,
// carrying the custom value (the reference OVERRIDE semantics, SPEC §11 precedence).
func TestSpanCustomTagsUpsertOverridesBuiltin(t *testing.T) {
	d := freshDecision()
	in := freshInputs() // built-in http.method == "GET"
	start := time.Now()
	end := start.Add(time.Millisecond)
	s := BuildServerSpan(d, in, []KV{{Key: "http.method", Str: "COLLIDE-VALUE"}}, start, end)

	n := 0
	for _, kv := range s.Attrs {
		if kv.Key == "http.method" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("http.method attribute count = %d, want 1 (upsert, not append-duplicate)", n)
	}
	if v := attrStr(s.Attrs, "http.method"); v != "COLLIDE-VALUE" {
		t.Errorf("http.method = %q, want COLLIDE-VALUE (override)", v)
	}
}
```
(`freshInputs().Method` is `"GET"` per `span_test.go` — confirm at implementation; if it differs, set `in.Method = "GET"` explicitly. `attrStr` is the existing `span_test.go:298` helper.)

- [ ] **Step 3: Run — expect FAIL** (signature already updated in Step 1, so the old build won't have the param; after Step 1 the tests compile but the upsert isn't implemented yet — the append test may pass trivially once the param exists but the OVERRIDE test fails because a naive spec has no loop). Precisely: after Step 1 the param exists but `BuildServerSpan` ignores `customTags`, so both new tests FAIL (custom_env absent; http.method count 1 but value still "GET"). Run:

```
cd internal/tracing && go test -run 'TestSpanCustomTags' -count=1 .
```
Expected: FAIL.

- [ ] **Step 4: Implement the param + upsert loop.** Change `BuildServerSpan` (`span.go:64-65`):

```go
func BuildServerSpan(d Decision, in SpanInputs, customTags []KV, start, end time.Time) *Span {
	attrs := make([]KV, 0, 17+len(customTags)) // 16 always-present + 1 optional + custom
```
Leave the 16 built-ins (`:68-88`) and the optional guid (`:91-93`) unchanged. Insert BEFORE the `return &Span{...}` (`:95`):

```go
	// Custom tags (literal). Upsert-by-key: a tag whose key collides with a built-in
	// OVERRIDES it (last-write-wins, matching the reference OTel tracer — SPEC-59 §11
	// arm precedence-otlp), NOT append-duplicate. The common non-colliding case is a
	// pure append. Cost is O(builtins) per tag — negligible (≤17 built-ins, few tags).
	for _, ct := range customTags {
		upsertAttr(&attrs, ct)
	}
```
Add the helper (after `BuildServerSpan`):

```go
// upsertAttr sets ct by key: replaces an existing attribute with the same key
// (last-write-wins), else appends. Keeps envoy-go's OTLP toProto (which emits every
// KV as a distinct wire attribute) consistent with the reference's single overridden
// attribute AND with the Zipkin encoder's tags map (which already last-write-wins).
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

- [ ] **Step 5: Run — expect PASS:**

```
cd internal/tracing && go test -count=1 . && cd - && go build ./...
```
Expected: PASS (all tracing tests, incl. the updated call sites) + clean build (the accesslog_emit.go threading compiles).

- [ ] **Step 6: LIVENESS BREAK (`-count=1`, confirm WHICH fires).** Temporarily change `upsertAttr` to ALWAYS append (`*attrs = append(*attrs, ct)` unconditionally, delete the loop) and confirm `TestSpanCustomTagsUpsertOverridesBuiltin` fires on `count = 2, want 1` (the append-duplicate divergence), NOT the append test. Restore.

```
cd internal/tracing && go test -run 'TestSpanCustomTagsUpsertOverridesBuiltin' -count=1 -v .
```

- [ ] **Step 7: Per-task gates + commit:**

```
gofmt -l internal/tracing/span.go internal/tracing/span_test.go internal/tracing/zipkin_test.go internal/filter/hcm/accesslog_emit.go
golangci-lint run ./internal/tracing/... ./internal/filter/hcm/... && go vet ./internal/tracing/... ./internal/filter/hcm/...
go build ./... && go test -count=1 ./internal/tracing/... ./internal/filter/hcm/...
git add internal/tracing/span.go internal/tracing/span_test.go internal/tracing/zipkin_test.go internal/filter/hcm/accesslog_emit.go
git commit -m "phase 59 IMPL T2: BuildServerSpan customTags upsert param + thread both accesslog_emit call sites"
```

---

## Task 3: Zipkin encoder unit test — literal tag surfaces in the `tags` map

**Files:**
- Modify: `internal/tracing/zipkin_test.go` (add one test; `encoding/json` already imported `:6`)

**Interfaces:**
- Consumes: `BuildServerSpan` (Task 2 signature), `encodeZipkinSpans` (`zipkin.go:78`).

- [ ] **Step 1: Write the failing test** in `zipkin_test.go` (append at end; reuse the existing `b[1:len(b)-1]` single-span decode idiom from `:96-116`):

```go
// TestZipkinEncodeCustomTagLiteral: a literal custom tag surfaces in the Zipkin v2
// `tags` map; node_id/zone stay dropped (the 14-tag roster is unchanged otherwise).
func TestZipkinEncodeCustomTagLiteral(t *testing.T) {
	d := freshDecision()
	in := freshInputs()
	in.Authority = "127.0.0.1:10000"
	in.NodeID = "node-x"
	in.Zone = "zone-y"
	start := time.Unix(0, 1_000_000_000)
	end := start.Add(10 * time.Millisecond)
	span := BuildServerSpan(d, in, []KV{{Key: "custom_env", Str: "prod-literal"}}, start, end)

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
	if got.Tags["custom_env"] != "prod-literal" {
		t.Errorf("tags[custom_env] = %q, want prod-literal", got.Tags["custom_env"])
	}
	if _, ok := got.Tags["node_id"]; ok {
		t.Errorf("tags[node_id] present, want dropped by the Zipkin encoder")
	}
	if _, ok := got.Tags["zone"]; ok {
		t.Errorf("tags[zone] present, want dropped by the Zipkin encoder")
	}
}
```

- [ ] **Step 2: Run — expect PASS** (the Task-2 upsert already flows the tag into `Attrs`; `encodeZipkinSpans` builds `tags` from `Attrs`, dropping node_id/zone at `zipkin.go:89`):

```
cd internal/tracing && go test -run 'TestZipkinEncodeCustomTagLiteral' -count=1 -v .
```
Expected: PASS.

- [ ] **Step 3: LIVENESS BREAK (`-count=1`).** Temporarily add `custom_env` to the `zipkin.go:89` drop condition (`if kv.Key == "node_id" || kv.Key == "zone" || kv.Key == "custom_env"`) and confirm the `tags[custom_env]` assertion fires; restore. (This proves the assertion is live, not vacuously green.)

- [ ] **Step 4: Gates + commit:**

```
gofmt -l internal/tracing/zipkin_test.go && golangci-lint run ./internal/tracing/...
cd internal/tracing && go test -count=1 . && cd -
git add internal/tracing/zipkin_test.go
git commit -m "phase 59 IMPL T3: Zipkin encoder literal-custom-tag unit test"
```

---

## Task 4: New OTLP fixture `0102-tracing-custom-tags-literal`

**Files:**
- Create: `test/fixtures/0102-tracing-custom-tags-literal/{envoy.yaml,envoy-go.yaml,expectations.yaml,README.md}`
- Create: `test/fixtures/0102-tracing-custom-tags-literal/driver/driver.go`

**Approach:** CLONE `0087-tracing-otlp` verbatim, then apply the enumerated edits below. Do NOT mutate `0087`/`0088` (`reference_differential_fixture_dispatch_constraint` — one fixture dir = one runner branch).

- [ ] **Step 1: Clone the fixture dir:**

```
cp -r test/fixtures/0087-tracing-otlp test/fixtures/0102-tracing-custom-tags-literal
```

- [ ] **Step 2: Add the custom tag to BOTH bootstrap templates.** In `envoy.yaml` AND `envoy-go.yaml`, inside the HCM `tracing:` block, add (a NON-colliding key so parity is clean — the collision/override case is the Task-2 unit test):

```yaml
        custom_tags:
        - tag: custom_env
          literal:
            value: prod-literal
```
Place it as a sibling of `provider:` under `tracing:`. (Verify indentation against each file's existing `tracing:` block — the two templates may differ in surrounding structure.)

- [ ] **Step 3: Edit `driver/driver.go`** — the enumerated changes on the clone:
  1. Package doc + `fixtureName` const → `"0102-tracing-custom-tags-literal"`.
  2. `refListenerPort` → `10102` (convention "100NN"; 0102 → 10102).
  3. `wantServiceName` → `"0102"` (and set `service_name: "0102"` in both yamls if the 0087 clone bakes `"0087"`).
  4. Rename the driver type `traceOTLPDriver` → `customTagDriver` (avoid a symbol clash across fixture packages is unnecessary — each fixture is its own package — but rename for clarity), OR keep the type name (packages are isolated; a rename is cosmetic). **Keep the type name** to minimize diff; only the consts change.
  5. Add the custom-tag constants near the other consts:
  ```go
  const (
  	customTagKey   = "custom_env"
  	customTagValue = "prod-literal"
  )
  ```
  6. Add an `Errorf`-based assertion function (mirrors `assertContinuationSpans` structure but uses `Errorf` per `reference_fatalf_makes_assertions_unreachable`):
  ```go
  // assertCustomTag asserts the phase-59 literal custom tag on EVERY span,
  // cross-side by KEY (OTLP attribute order is non-deterministic — SPEC §11).
  // Errorf per property so one failure does not mask the rest.
  func assertCustomTag(t fixture.TB, side string, spans []*tracepb.Span) {
  	t.Helper()
  	for i, sp := range spans {
  		v, ok := spanAttrMap(sp)[customTagKey]
  		if !ok {
  			t.Errorf("%s span %d: missing custom tag key %q (present: %v)", side, i, customTagKey, mapKeys(spanAttrMap(sp)))
  			continue
  		}
  		if got := v.GetStringValue(); got != customTagValue {
  			t.Errorf("%s span %d: %s = %q, want %q", side, i, customTagKey, got, customTagValue)
  		}
  	}
  }
  ```
  7. Call it for BOTH sides inside `AssertStats`, after the `assertServiceName` calls:
  ```go
  	assertCustomTag(t, "reference", d.refSpans)
  	assertCustomTag(t, "subject", d.subjSpans)
  ```

- [ ] **Step 4: Update `expectations.yaml` + `README.md`** — reflect the new fixture name/purpose (the literal custom-tag attribute asserted cross-side by key). Keep the `0087` framing otherwise (the framework-gap `upstream_cluster` note stays). Ensure `expectations.yaml` does not encode a stale `0087` span-count expectation that now differs (span count is unchanged — 12).

- [ ] **Step 5: Register + run the fixture.** Confirm the fixture package is imported by the differential runner's fixture registry (the `init()` `RegisterFixture` + a blank import in the runner's import aggregator — mirror how `0087` is wired; grep the runner for `0087-tracing-otlp` and add the `0102` sibling). Then:

```
go test ./test/differential/ -run 'TestDifferential/0102-tracing-custom-tags-literal' -count=1 -v
```
Expected: PASS (both sides emit the `custom_env=prod-literal` attribute).

- [ ] **Step 6: LIVENESS BREAK (`-count=1`, confirm WHICH fires).** Temporarily set `customTagValue = "WRONG"` and confirm BOTH sides' `assertCustomTag` fire with `custom_env = "prod-literal", want "WRONG"` (the custom-tag assertion, not another). Restore. (Isolates to the new assertion per `reference_deliberate_break_wrong_assertion`.)

```
go test ./test/differential/ -run 'TestDifferential/0102-tracing-custom-tags-literal' -count=1 -v
```

- [ ] **Step 7: Gates + commit** (Docker required for the differential; if the subagent lacks Docker, the controller runs the differential at stage-close — note the deferral in the commit):

```
gofmt -l test/fixtures/0102-tracing-custom-tags-literal/driver/driver.go && golangci-lint run ./test/...
go build ./...
git add test/fixtures/0102-tracing-custom-tags-literal/ <runner-registry-file>
git commit -m "phase 59 IMPL T4: 0102-tracing-custom-tags-literal OTLP fixture (fixtures 103 -> 104)"
```

---

## Task 5: `FuzzHCMConfigParse` seed — one `custom_tags` seed (fuzzers stay 54)

**Files:**
- Modify: `internal/filter/hcm/fuzz_test.go` (add imports `hcmv3`, `tracingv3`; one seed after `:29`)

- [ ] **Step 1: Reconcile the fuzzer count BEFORE** (`reference_fuzzer_count_docs_drift`):

```
grep -rn '^func Fuzz' --include='*.go' | wc -l    # expect 54
```

- [ ] **Step 2: Add the imports** to `fuzz_test.go`:

```go
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	tracingv3 "github.com/envoyproxy/go-control-plane/envoy/type/tracing/v3"
```

- [ ] **Step 3: Add the seed** in `FuzzHCMConfigParse`, after the three existing `f.Add` seeds (`:27-29`):

```go
	// Phase 59: a custom_tags seed — one accepted `literal` + one rejected
	// `request_header` type. The custom_tags loop runs BEFORE the provider check
	// (config.go), so this seed exercises both the accept-append and a reject arm.
	withCustomTags := mkHCM(func(h *hcmv3.HttpConnectionManager) {
		h.Tracing = &hcmv3.HttpConnectionManager_Tracing{
			CustomTags: []*tracingv3.CustomTag{
				{Tag: "env", Type: &tracingv3.CustomTag_Literal_{Literal: &tracingv3.CustomTag_Literal{Value: "prod"}}},
				{Tag: "hdr", Type: &tracingv3.CustomTag_RequestHeader{RequestHeader: &tracingv3.CustomTag_Header{Name: "x-req"}}},
			},
		}
	})
	f.Add(withCustomTags.GetTypeUrl(), withCustomTags.GetValue())
```

- [ ] **Step 4: Run the fuzzer briefly + reconcile the count AFTER:**

```
cd internal/filter/hcm && go test -run 'FuzzHCMConfigParse' -count=1 . && go test -fuzz 'FuzzHCMConfigParse' -fuzztime 10s .
grep -rn '^func Fuzz' --include='*.go' | wc -l    # expect 54 (a seed is NOT a new func Fuzz)
```
Expected: PASS; no panic; count STILL 54.

- [ ] **Step 5: Gates + commit:**

```
gofmt -l internal/filter/hcm/fuzz_test.go && golangci-lint run ./internal/filter/hcm/...
git add internal/filter/hcm/fuzz_test.go
git commit -m "phase 59 IMPL T5: FuzzHCMConfigParse custom_tags seed (fuzzers stay 54)"
```

---

## Task 6: `BEHAVIOR_CONTRACT.md` edits (`:686` + `:739`)

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`

- [ ] **Step 1: Edit line 686** (the STRICT-REJECT roster) — remove `custom_tags` from the wholesale list and add a clause. Replace the sentence ending `...an empty \`cluster_name\`, \`custom_tags\`, \`verbose\`, \`max_path_tag_length\`,...` so that `custom_tags` is REMOVED from that comma list, and append after the paragraph:

```markdown
The `custom_tags` field is now PARTIALLY consumed (phase 59): the `literal` `CustomTag` type is emitted as a static `{tag, value}` STRING span attribute on BOTH the OTLP and Zipkin exporters (UPSERT/last-write-wins on a built-in-key collision, matching the reference OVERRIDE semantics); `request_header`/`environment`/`metadata` STRICT-REJECT loudly with distinct substrings (envoy-go-strict DEPARTURE — the reference accepts them); empty-`tag` / empty-`literal.value` / typeless STRICT-REJECT (PGV-parity — the reference boot-rejects too).
```

- [ ] **Step 2: Edit line 739** (the Zipkin "Does not yet apply to" bullet) — NARROW the `custom_tags` mention:

```markdown
- `custom_tags` (the `literal` type is CONSUMED as of phase 59; `request_header`/`environment`/`metadata` remain STRICT-REJECTED), `spawn_upstream_span`, `http_service`, `resource_detectors`, `sampler` — STRICT-REJECTED at parse; deferred to future rows.
```

- [ ] **Step 3: Commit** (docs-only; no gate beyond a grep that `custom_tags` no longer appears in the wholesale reject comma-list):

```
git add docs/envoy-go/BEHAVIOR_CONTRACT.md
git commit -m "phase 59 IMPL T6: BEHAVIOR_CONTRACT custom_tags literal-consumed / narrow deferred bullet"
```

---

## Task 7: Verify — six-gate + full 104-dir differential

**Files:** none (verification only).

- [ ] **Step 1: Six-gate on the frozen HEAD:**

```
gofmt -l internal/ test/ cmd/            # expect no output
golangci-lint run ./...
go vet ./...
go build ./...
go mod tidy -diff                        # expect EMPTY (tracingv3 is an already-resolved module)
go test -race -count=1 ./internal/tracing/... ./internal/filter/hcm/...
```
The `-race` on BOTH `internal/tracing` and `internal/filter/hcm` is required (`reference_detrand_race_catches_protojson_value_substring`, `reference_full_suite_race_after_background_mutator`).

- [ ] **Step 2: Full differential — 104 dirs, byte-stable except `0102`** (Docker; the reference is `envoyproxy/envoy:contrib-v1.37.2`):

```
go test ./test/differential/ -count=1
```
Expected: `ok`, exit 0. The 103 pre-existing dirs stay byte-stable (no `custom_tags` in their configs ⇒ `CustomTags == nil` ⇒ the upsert loop is a no-op); `0102` is the only new dir. If a startup flake appears on an UNRELATED fixture (`subject ready: EOF`), isolate-re-run to discriminate (`reference_differential_fullsuite_startup_flake`).

- [ ] **Step 3: Record the evidence** (the six-gate output + the differential `ok ... exit 0`) in `PROGRESS.md`. No commit (verification task); findings feed Task 8's ADR-0277 §Consequences.

---

## Task 8: ADR-0277 body + STATE + ROADMAP + router roll

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0277)
- Modify: `docs/envoy-go/STATE.md` (`:7` active-phase header)
- Modify: `docs/envoy-go/ROADMAP.md` (`:121` row 59 → `done`; `:181` family prose + deferred-sentence narrow)
- Modify: `next-prompt.txt` (router roll to the NEXT router decision — a new BRAINSTORM)

- [ ] **Step 1: Append ADR-0277 §Decision/§Consequences** to `DECISIONS.md` (build on the SPEC §13 §Context draft — reproduce §Context, then add):

  - **§Decision:** literal-only `custom_tags`; `parseCustomTags` (6 arms) replaces the wholesale reject; `TracingConfig.CustomTags []KV` set provider-neutrally after the switch; `BuildServerSpan` gains `customTags []KV` applied by `upsertAttr` (last-write-wins, matching the reference OVERRIDE, consistent across OTLP `toProto` and the Zipkin `tags` map); three type-DEPARTURE rejects + three PGV-parity structural rejects, all ADR-0080-distinct; the `TracingConfig.CustomTags` field + the `BuildServerSpan customTags` param FOLDED into ADR-0277 (no separate seam ADR — the phase-58 precedent).
  - **§Consequences:** the `0102` OTLP differential (cross-side by key, `Errorf`-per-property, break-proven live); the Zipkin path a unit test; a `FuzzHCMConfigParse` seed; +0 stats / +1 fixture / +0 fuzzers / +0 packages / +0 modules; the four EMPTY-emitted built-ins (`upstream_cluster`/`node_id`/`zone`/`peer.address`, `reference_tracing_upstream_cluster_framework_gap`) UNTOUCHED. Record the six-gate + 104-dir result from Task 7.

- [ ] **Step 2: Update `STATE.md:7`** active-phase header → `phase 59 (tracing-custom-tags-literal) IMPL done` (lifecycle 2 → done; row 59 → `done`; DECISIONS tail ADR-0276 → ADR-0277). Counts: fixtures 103 → 104; all others +0.

- [ ] **Step 3: Update `ROADMAP.md`** — flip row 59 (`:121`) `in-progress` → `done` (ADR-0106, the SOLE leg — `reference_roadmap_split_phase_row_done`). NARROW the LIVE deferred sentence (`:181`): change `tracing \`custom_tags\`/...` to the NON-literal types only, e.g. `tracing custom_tags (\`request_header\`/\`environment\`/\`metadata\`)/\`spawn_upstream_span\`/\`http_service\`/force-trace`. Then RE-RUN the sentinel check-(2) grep and confirm EXACTLY ONE live `candidates:` match (`reference_sentinel_deferred_sentence_live_vs_historical`):

```
grep -oE 'remaining deferred \(not-yet-chartered\) candidates:[^.]*\.' docs/envoy-go/ROADMAP.md
```
Expected: one line, still naming the Observability family's remaining follow-ons (custom_tags now narrowed).

- [ ] **Step 4: Roll `next-prompt.txt`** to point at the NEXT router decision — a new BRAINSTORM (the roller self-picks the smallest defensible candidate per the 2026-07-12 standing directive; the sentinel does NOT fire — Observability + Operational-tooling stay open, five families never opened). Re-run the three sentinel checks mechanically and record they still print.

- [ ] **Step 5: Commit** (the ADR/STATE/ROADMAP/router are docs-only):

```
git add docs/envoy-go/DECISIONS.md docs/envoy-go/STATE.md docs/envoy-go/ROADMAP.md next-prompt.txt
git commit -m "phase 59 (tracing-custom-tags-literal) IMPL: ADR-0277 + STATE/ROADMAP row 59 done + router roll"
```

- [ ] **Step 6: Controller stage-close** — squash all task commits into ONE `phase 59 (tracing-custom-tags-literal) IMPL: ...` commit on `master`, re-run the six-gate + 104-dir differential on the frozen HEAD, then PUSH (`feedback_push_to_origin`, `feedback_subagents_no_push` — only the controller pushes).

---

## Self-Review (checked against SPEC 59 this PLAN session)

**Spec coverage:** SPEC §3.1 parse arms → Task 1. §3.2 field + threading → Task 1. §3.3 upsert → Task 2. §3.4 call-site threading → Task 2 (SPEC's task 4 folded, as SPEC §10 anticipated). §6 reject roster → Task 1; fuzz seed → Task 5. §7 +0 stats → Task 7 gate. §8 fixture → Task 4; Zipkin unit test → Task 3. §9 BEHAVIOR_CONTRACT → Task 6. §10 test plan → Tasks 1-4. §13 ADR → Task 8. No gap.

**Placeholder scan:** every code step carries real code; the only "clone-then-edit" is Task 4 (the 700-line 0087 driver), enumerated as explicit edits with the new assertion function reproduced in full.

**Type consistency:** `BuildServerSpan(d, in, customTags, start, end)` used identically in Tasks 2/3/4 and both `accesslog_emit.go` sites; `parseCustomTags([]*tracingv3.CustomTag) ([]KV, error)`, `upsertAttr(*[]KV, KV)`, `TracingConfig.CustomTags []KV`, `KV{Key,Str}` all consistent. The proto oneof wrapper names (`CustomTag_Literal_` / `CustomTag_RequestHeader` asymmetry) are used correctly in Tasks 1 and 5.

**Ordering:** Task 1 (field) precedes Task 2 (threading reads the field); Task 2 (signature) updates all 9 call sites atomically so the build never breaks; Task 3 depends on Task 2's tag flow; Task 4 depends on Tasks 1-2; Tasks 5-6 independent; Task 7 gates the frozen tree; Task 8 lands docs + router roll.
