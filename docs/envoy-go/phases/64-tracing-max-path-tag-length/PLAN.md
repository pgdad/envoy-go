# PLAN 64 — tracing `max_path_tag_length` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Every task is TDD (`superpowers:test-driven-development`): red → green, with a `-count=1` liveness break where an assertion is load-bearing.

**Goal:** Lift the one `max_path_tag_length is unsupported` reject in `internal/tracing/config.go:112-114` and HONOR the knob — byte-truncate the `:path` (path+query) portion of the ALREADY-emitted `http.url` built-in span attribute to N bytes (default 256 when ABSENT; explicit 0 = empty path; the `scheme://host` prefix never truncated), on BOTH the OTLP and Zipkin exporters. The FIRST tracing NUMERIC-knob row (prior tracing rows added tags/providers; this caps an existing tag).

**Architecture:** A `TracingConfig.MaxPathTagLength uint32` scalar (resolved in `NewConfig`: default 256 / explicit incl. 0, set post-provider-dispatch alongside `cfg.CustomTags` so both provider arms carry it — `parseOTel`/`parseZipkin` signatures UNCHANGED) + a shared exported helper `tracing.BuildHTTPURL(scheme, host, pathAndQuery string, maxPathTagLen uint32) string` (byte-truncate `pathAndQuery` FIRST, then prepend `scheme://host`) called at the THREE `internal/filter/hcm/accesslog_emit.go` URL-construction sites (H1 `:40` / H2 `:93` / H3 `:162`) replacing the inline concatenation. `BuildServerSpan`/`Span.Attrs`/`upsertAttr`/`ResolveCustomTags`/both exporters/the entire custom-tag machinery are UNCHANGED — the truncation acts only on the single `http.url` string handed to `SpanInputs.URL`, which both exporters consume. One new OTLP differential fixture (`0107`) proves the truncation cross-side by `http.url` VALUE-equality on the truncated form; the default-256/explicit-0/query-cut/byte-boundary edges are deterministic UNIT tests on `BuildHTTPURL`.

**Tech Stack:** Go; `github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3` (`HttpConnectionManager_Tracing.GetMaxPathTagLength() *wrapperspb.UInt32Value` — an ALREADY-resolved module: `config.go:11` imports `hcmv3`); `google.golang.org/protobuf/types/known/wrapperspb` (test/fuzz construction); the differential harness (`test/differential/fixture`, `test/helpers/otlptrace`); `go-fuzz`-style `testing.F` seed corpus.

## Global Constraints

- **Single stage this session context is the IMPL** — subagent-driven per `feedback_execution_style`/`feedback_subagent_autocommit_claudemd`/`feedback_subagents_no_push`: each task is a fresh subagent that commits LOCALLY only; the controller verifies each commit, cleans any leak files, squashes at stage-close, re-runs the suite on the frozen HEAD, and pushes.
- **Worktree discipline** (`feedback_git_worktrees`, `feedback_subagent_worktree_path_targeting`, `feedback_subagent_worktree_detach`): the IMPL runs in `.worktrees/phase-64-impl` (branch `phase-64-tracing-max-path-tag-length-impl`). Pin the canonical worktree root; subagents write worktree-relative paths; the controller verifies the MAIN checkout stays clean. On a deliberate break, restore with `git restore` only (no checkout-sha/amend) and re-verify the branch each task.
- **`next-prompt.txt` IS TRACKED** (`reference_next_prompt_tracked_despite_gitignore`) — edit it inside the stage worktree and fold into the squash; locate commits by SUBJECT (`git log --grep`), never by position.
- **ADR-0080** — every reject substring stays DISTINCT. NO new reject is added this row (`max_path_tag_length` has no PGV numeric constraint — SPEC §6); the sibling tracing rejects (`verbose`/`spawn_upstream_span`/`custom_tags metadata`/`http_service`/`resource_detectors`/`sampler`/`google_grpc`/non-`HTTP_JSON` Zipkin/`split_spans_for_request`/empty clusters) STAY loud and unchanged.
- **ADR-0106** — row 64 flips `done` ONLY at this IMPL's six-gate (the SOLE leg; `reference_roadmap_split_phase_row_done`).
- **ADR-0044** — ADR-0285 §Decision/§Consequences land at this IMPL (SPEC §13 drafted §Context); DECISIONS tail **ADR-0284 → ADR-0285** (next-free ADR-0286).
- **ADR-0045** — a SINGLE FLAT ROW (9 tasks; escape-valve UNCONSUMED, well under the ~15 ceiling — the parse arm + the truncation helper both sit on the SAME landed tracing engine, no second subsystem to strand).
- **Per-task gates** (`feedback_pertask_gofmt_lint`): each code task runs `gofmt -l` + `golangci-lint run` on touched packages + `go build ./...` + the touched-package `go test` before its commit.
- **Anticipated counts at IMPL-DONE:** stat surface **1201 (+0)** · fixtures **108 → 109** (`0107`) · fuzzers **55 (+0, seed only)** · BackendKind **38 (+0)** · DECISIONS tail **ADR-0285** (next-free ADR-0286) · new Go packages **0** · new go.mod modules **0**.
- **⚠️ ZERO-VALUE CAP TRAP (load-bearing).** `MaxPathTagLength` is a `uint32` whose zero value is `0` = EMPTY-path truncation. Production NEVER hits this: `NewConfig` ALWAYS sets it (default 256). But any TEST that constructs a `tracing.TracingConfig{...}` LITERAL directly (bypassing `NewConfig`) gets `MaxPathTagLength == 0`, so after Task 3 rewires the call sites, `BuildHTTPURL(..., 0)` truncates the `:path` to EMPTY for those filters. The existing `span_emit_test.go` filters (`:101`/`:137`/`:154`/`:187`/`:209`/`:238`/`:277`/`:292`/`:323`/`:340`) construct bare literals but NONE assert `http.url` (they assert Name/Kind/TraceID/SpanID/count), so they STAY green. Any NEW test asserting `http.url` on a literal config MUST set `MaxPathTagLength` explicitly (Task 3 does). Do NOT "fix" this by defaulting the field in the struct — the default belongs in `NewConfig` (the reference semantics), and a struct-level default would mask a genuinely-configured explicit 0.
- **⚠️ `span_test.go:100` asserts `http.url` but stays green.** `internal/tracing/span_test.go:100-101` asserts `attrStr(s.Attrs, "http.url") == "http://h/p"` — but it calls `BuildServerSpan` DIRECTLY with a pre-built `SpanInputs.URL` (`freshInputs()` `:37` sets `URL: "http://h/p"`). Truncation is at the CALL SITE (`accesslog_emit.go`), NOT inside `BuildServerSpan`, so this test does NOT flow through `BuildHTTPURL` and is UNAFFECTED. Confirm it stays green at Task 3 (do NOT edit it).
- **`reference_fatalf_makes_assertions_unreachable`** — in the helper matrix, config tests, span-emit tests, and the fixture driver, use `Errorf` per independent property/row so one failing case does not mask the rest; reserve `Fatalf` for a broken precondition (e.g. `len(spans) != 1`).
- **`reference_deliberate_break_wrong_assertion`** — every liveness break must confirm WHICH assertion fired (subtest name / message), not merely that *a* failure occurred; add an isolating break where a break could abort earlier and mask the intended one.
- **`reference_differential_break_protocol_count1`** — every differential/liveness break runs with `-count=1` (go-test caching serves a stale PASS otherwise).
- **`reference_differential_run_selector`** — the `0107` differential ALWAYS uses the FULL `-run 'TestDifferential/0107-tracing-max-path-tag-length'` selector, NEVER a bare `-run '0107'` (which matches ZERO subtests → vacuous green).
- **`reference_vacuous_break_receiver_normalizes`** — in the `0107` fixture, break the ASSERTION's expected value ONLY (the yaml still emits the real truncated URL); a break that changes both the emitted config AND the expected value fires NOTHING.
- **`reference_differential_http_expectations_tcp_only`** — the `0107` fixture drives H1/H2 over TCP; assert `http.url` in the `otlptrace` receiver (via `AssertStats`), NOT in `HTTPExpectations`.
- **`reference_tracing_upstream_cluster_framework_gap`** — the four EMPTY-emitted built-ins (`upstream_cluster`/`node_id`/`zone`/`peer.address`) are a SEPARATE framework gap; `max_path_tag_length` acts ONLY on `http.url` (a fully-POPULATED built-in). Do NOT conflate; the `0107` clone inherits the `0106` drivers' key-presence-only assertion on those four.
- **`reference_tracing_custom_tag_override_builtin`** — a custom `http.url` tag (a `literal`/`request_header`/`environment` custom_tag keyed `http.url`) STILL upsert-OVERRIDES the truncated built-in via `upsertAttr` — the truncation acts on the built-in value, a colliding custom tag replaces it wholesale (UNCHANGED; the SPEC notes it, no new arm needed).
- **`reference_fuzzer_count_docs_drift`** — the seed must NOT move the fuzzer count off 55; reconcile actual `^func Fuzz` = 55 before AND after.
- **`reference_spec_drafted_identifier_collision_check`** — `BuildHTTPURL`/`buildHTTPURL` and `MaxPathTagLength` were RE-GREP-confirmed collision-free in `internal/tracing` this PLAN session against master tip `bb221ec5` (no existing `BuildHTTPURL`/`buildHTTPURL`; `MaxPathTagLength` appears ONLY as the proto getter `t.GetMaxPathTagLength()` `config.go:112` + the reject-test proto field `tr.MaxPathTagLength` `config_test.go:340` — both distinct from the new struct field).

---

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `internal/tracing/config.go` | ADD `TracingConfig.MaxPathTagLength uint32` field (`:25-40`); replace the `:112-114` reject with the resolve arm; set `cfg.MaxPathTagLength` post-dispatch alongside `cfg.CustomTags` (`:154`) | 1 |
| `internal/tracing/config_test.go` | REMOVE the `max_path_tag_length` reject row from `TestNewConfigRejectArms` (`:336-343`); ADD `TestNewConfigMaxPathTagLength` (explicit 128 / absent→256 / explicit-0-preserved) | 1 |
| `internal/tracing/url.go` (NEW) | ADD `func BuildHTTPURL(scheme, host, pathAndQuery string, maxPathTagLen uint32) string` (byte-truncate `pathAndQuery` first, then prepend `scheme://host`) | 2 |
| `internal/tracing/url_test.go` (NEW) | the `BuildHTTPURL` matrix (under-cap / over-cap / explicit-0 / query-cut / default-256 / byte-boundary) | 2 |
| `internal/filter/hcm/accesslog_emit.go` | rewire the THREE URL-build sites (H1 `:40` / H2 `:93` / H3 `:162`) to call `tracing.BuildHTTPURL(scheme, host, pathAndQuery, f.tracingConfig.MaxPathTagLength)` | 3 |
| `internal/filter/hcm/span_emit_test.go` | ADD a `spanAttr` helper + H1 + H2 truncation tests (via `newTracingFilter`/`fakeExporter` with `MaxPathTagLength: 16`) asserting a truncated `http.url` reaches the span through the call sites | 3 |
| `internal/tracing/zipkin_test.go` | ADD `TestZipkinEncodeTruncatedHTTPURL`: a truncated `http.url` surfaces verbatim in the Zipkin `tags` map | 4 |
| `test/fixtures/0107-tracing-max-path-tag-length/` | new OTLP fixture (clone `0106`); `max_path_tag_length: {value: 16}`; long-ASCII-path GET; cross-side `http.url` VALUE-equality on the truncated form | 5 |
| `test/differential/runner_test.go` | blank-import the `0107` driver (after the `0106` line `:133`) | 5 |
| `internal/filter/hcm/fuzz_test.go` | one `max_path_tag_length` seed on the existing `FuzzHCMConfigParse` (after the phase-63 seed `:74`); ADD the `wrapperspb` import | 6 |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | tracing clause: `max_path_tag_length` REJECT → CONSUMED (truncates `http.url` `:path`; default 256; explicit 0 = empty path; both exporters); sibling rejects STAY | 7 |
| `docs/envoy-go/{DECISIONS,STATE,ROADMAP}.md`, `PROGRESS.md`, `next-prompt.txt` | ADR-0285 body; STATE header; ROADMAP row 64 → done (deferred sentence UNCHANGED); PROGRESS close; router roll | 9 |

**RE-DERIVED edit-site roster (verified against master tip `bb221ec5` this PLAN session, `feedback_brief_citations_not_evidence`):**
- `config.go:11` `hcmv3` import (present) · `config.go:25-40` `TracingConfig` struct (fields `ClientSampling`/`RandomSampling`/`OverallSampling`/`ServiceName`/`ClusterName`/`Provider`/`Zipkin`/`CustomTags`) · `config.go:104` `NewConfig` · `:109-110` `verbose` reject (UNCHANGED) · `:112-114` the `max_path_tag_length` reject (LIFTED) · `:115` `parseCustomTags` call (UNCHANGED) · `:119-120` `spawn_upstream_span` reject (UNCHANGED) · `:143-150` the provider dispatch switch · `:154` `cfg.CustomTags = customTags` (the post-dispatch assignment site) · `:155` `return cfg, nil`.
- `internal/filter/hcm/accesslog_emit.go` — the emit methods `emitAccessLog` (H1, `r *http.Request`) `:25`, `emitAccessLogH2` (H2, `req h2.H2Request`) `:85`, and the H3 variant (`r *http.Request`) near `:149`; the three URL-build lines: H1 `:40` `url := scheme + "://" + r.Host + r.URL.RequestURI()`; H2 `:93` `url := scheme + "://" + req.Authority + req.Path`; H3 `:162` `url := scheme + "://" + r.Host + r.URL.RequestURI()`. Each site already dereferences `f.tracingConfig.CustomTags` in the same block (`f.exporter != nil` guards the block; `f.tracingConfig` is non-nil there) and `tracing` is already imported (`accesslog_emit.go:13`).
- `internal/filter/hcm/span_emit_test.go` — `fakeExporter` `:33` (`Export`/`captured`); `newTracingFilter(t, exp, cfg)` `:59`; `knownDecision()` `:86`; existing H1/H2 span-emit tests `:99`–`:358` (assert Name/Kind/TraceID/SpanID/count — NEVER `http.url`); imports `net/http` `:18`, `h2` `:26`, `cluster` `:25`, `tracing` `:28`, `time` `:21`.
- `internal/tracing/span.go` — `type KV struct{Key; Str; Int; IsInt}` `:12`; `SpanInputs.URL` `:23`; `BuildServerSpan(d, in, customTags []KV, start, end)` `:64`; `upsertAttr` `:121` — ALL UNCHANGED (confirm at Task 3).
- `internal/tracing/config_test.go` — `wrapperspb` import `:14`; `otelProvider` / `envoyGrpcOTel` helpers (used at `:339`); `TestNewConfigRejectArms` `:281` (asserts ONLY `err != nil` + `got == nil`, NO `wantSub`); the `max_path_tag_length` row `:336-343` (`tr.MaxPathTagLength = wrapperspb.UInt32(128)`); the table's other rows (`verbose` `:328-335`, `spawn_upstream_span` `:344-351`, `http_service`/`resource_detectors`/`sampler`/`nil-provider`) STAY.
- `internal/tracing/zipkin_test.go` — `encoding/json` `:6`; the `b[1:len(b)-1]` single-span decode idiom `:116`/`:556`/`:591`/`:626`; `freshDecision`/`freshInputs` (from `span_test.go:12`/`:34`); `encodeZipkinSpans([]*Span, id128, shared bool)` (`zipkin.go:78`); the built-in drop guard `if kv.Key == "node_id" || kv.Key == "zone"` (`zipkin.go:89`); existing `TestZipkinEncodeResolvedEnvironmentTag` `:608`.
- `internal/filter/hcm/fuzz_test.go` — `FuzzHCMConfigParse` `:27`; `hcmv3` `:12` + `tracingv3` `:13` imported; the phase-59 seed `:36-44`, phase-62 `:50-59`, phase-63 (`withEnvTags`) `:65-74`; `mkHCM` (in `config_test.go`, package `hcm`); NO `wrapperspb` import yet (Task 6 ADDS it).
- `test/fixtures/0106-tracing-custom-tags-environment/` clone source — driver consts `fixtureName` `:98`, `refListenerPort 10106` `:103`, `numPlain 8` `:107` / `numCont 4` `:110` / `numTotal` `:112`, `probePath "/trace"` `:115`, `probeHost "trace.example"` `:116`, `wantServiceName "0106"` `:128`, `customTagKey "env_path"` `:140`, `FIXTURE_0106_DUMP` `:415`; `fireProbe` `:352` (`req.Host = probeHost` `:357`); the per-span built-in assertions `:494-514` (`assertAttrString(...,"http.method","GET")` `:494`; `assertAttrPresent(...,"http.url")` `:495`); `assertCustomTag` `:569`; `spanAttrMap` `:584`; `mapKeys` `:644`; `assertAttrString` `:604`; the `custom_tags` yaml block in `envoy.yaml`/`envoy-go.yaml` (sibling of `provider`/`random_sampling` under `tracing:`); `runner_test.go:133` (the `0106` blank-import — the tail).

**⚠️ Build-ordering (why T1+T2 are independent additive pieces, T3 wires them).** T1 (parse) and T2 (the helper) are INDEPENDENT: T1 adds the `MaxPathTagLength` field + resolves it in `NewConfig` (the call sites still ignore it, so no behavior change yet); T2 adds `BuildHTTPURL` as a pure function with its own unit test (no caller yet). The build stays green after each because neither changes an existing call path. T3 wires the helper into the three call sites — the build never breaks because the helper is a no-op for short paths and `NewConfig` always sets the cap. Tasks may run T1/T2 in either order; T3 depends on BOTH.

**⚠️ `0107` `http.url` VALUE-equality — the authority-encoding risk + the fallback.** The `0106` driver asserts `http.url` by PRESENCE only (`:495`, comment "value varies by host/path encoding") — a defensive choice. SPEC-64 §8 asserts cross-side VALUE-equality IS achievable because the truncated path is deterministic AND the SPEC §11 arm-0 probe showed the reference echoes the `Host` header verbatim into `http.url` (`http://h.io/...`, no port, no normalization). Since `0107` sends a bare-hostname `Host: trace.example` (no port) and a query-less ASCII path, `scheme://host` is identical cross-side, so `http.url` VALUE-equality holds. **Primary assertion:** `assertAttrString(t, side, i, attrs, "http.url", wantTruncatedURL)` on every span, both sides. **Fallback (only if the run shows a cross-side authority divergence the probe didn't predict):** strip each side's `scheme://authority` prefix and assert the remaining `:path` portion is exactly `probePath[:maxPathTagLen]` per side (side-independent of authority encoding), PLUS cross-side path-portion equality. The IMPL subagent runs the differential (Step 5), and if VALUE-equality fails on an authority mismatch, switches to the fallback and RECORDS the decision in PROGRESS. Do NOT pre-emptively weaken to the fallback — lead with VALUE-equality (the SPEC's ask).

---

## Task 1: `config.go` — `TracingConfig.MaxPathTagLength` field + lift the reject (resolve arm) + config tests

**Files:**
- Modify: `internal/tracing/config.go` (`TracingConfig` `:25-40`; `NewConfig` reject `:112-114`; the post-dispatch assignment `:154`)
- Modify: `internal/tracing/config_test.go` (`TestNewConfigRejectArms` `:336-343`; add `TestNewConfigMaxPathTagLength`)

**Interfaces:**
- Produces: `TracingConfig.MaxPathTagLength uint32` (a new field, always set by `NewConfig`: default 256 / explicit incl. 0). Consumed by Task 3's call sites.
- Consumes: `t.GetMaxPathTagLength() *wrapperspb.UInt32Value` + `.GetValue() uint32` (the HCM proto getter, `config.go:112`).

- [ ] **Step 1: Write the failing config test** in `internal/tracing/config_test.go` (append at end of file). `Errorf` per subtest (`reference_fatalf_makes_assertions_unreachable`); `wrapperspb` is already imported `:14`:

```go
// TestNewConfigMaxPathTagLength: max_path_tag_length resolves to a uint32 cap on
// TracingConfig — an explicit value is preserved, an ABSENT field defaults to 256
// (the reference default, SPEC-64 §11 arm 1), and an explicit 0 is preserved (arm 2,
// NOT treated as "unlimited"). Errorf per case so one failure does not mask the rest.
func TestNewConfigMaxPathTagLength(t *testing.T) {
	t.Run("explicit", func(t *testing.T) {
		tr := otelProvider(t, envoyGrpcOTel("c", "svc"))
		tr.MaxPathTagLength = wrapperspb.UInt32(128)
		cfg, err := NewConfig(tr)
		if err != nil {
			t.Fatalf("NewConfig explicit: unexpected err %v", err)
		}
		if cfg.MaxPathTagLength != 128 {
			t.Errorf("MaxPathTagLength = %d, want 128 (explicit value)", cfg.MaxPathTagLength)
		}
	})
	t.Run("absent-defaults-256", func(t *testing.T) {
		tr := otelProvider(t, envoyGrpcOTel("c", "svc")) // no MaxPathTagLength set
		cfg, err := NewConfig(tr)
		if err != nil {
			t.Fatalf("NewConfig absent: unexpected err %v", err)
		}
		if cfg.MaxPathTagLength != 256 {
			t.Errorf("MaxPathTagLength = %d, want 256 (absent default)", cfg.MaxPathTagLength)
		}
	})
	t.Run("explicit-zero-preserved", func(t *testing.T) {
		tr := otelProvider(t, envoyGrpcOTel("c", "svc"))
		tr.MaxPathTagLength = wrapperspb.UInt32(0)
		cfg, err := NewConfig(tr)
		if err != nil {
			t.Fatalf("NewConfig explicit-0: unexpected err %v", err)
		}
		if cfg.MaxPathTagLength != 0 {
			t.Errorf("MaxPathTagLength = %d, want 0 (explicit 0 preserved, NOT 256)", cfg.MaxPathTagLength)
		}
	})
}
```

- [ ] **Step 2: Run — expect FAIL** (the code still rejects `max_path_tag_length`, and the field does not exist yet — a COMPILE error, then a reject error once the field is added):

```
cd internal/tracing && go test -run 'TestNewConfigMaxPathTagLength' -count=1 -v .
```
Expected: FAIL — compile error `cfg.MaxPathTagLength undefined` (before Step 3) OR `NewConfig explicit: unexpected err tracing: max_path_tag_length is unsupported` (before Step 4).

- [ ] **Step 3: Add the `MaxPathTagLength` field** to `TracingConfig` (`config.go:25-40`), immediately after `OverallSampling` with a doc comment:

```go
type TracingConfig struct {
	ClientSampling  float64
	RandomSampling  float64
	OverallSampling float64
	// MaxPathTagLength is the resolved byte-cap on the http.url span attribute's
	// :path (path+query) portion: the reference default 256 when ABSENT, the explicit
	// value otherwise (an explicit 0 = empty path is PRESERVED). ALWAYS set by
	// NewConfig, so a configured-tracing Filter never sees the zero value as a
	// spurious 0-cap (D-MPTL-DEFAULT / D-MPTL-ZERO, SPEC-64 §11).
	MaxPathTagLength uint32
	ServiceName      string
	ClusterName      string
	Provider         ProviderKind
	Zipkin           *ZipkinSettings // non-nil iff Provider == ProviderZipkin
	// CustomTags are the parsed custom tags (provider-neutral), ORDERED and
	// first-wins-deduplicated by tag key at parse (matching the reference's
	// config-time map). ResolveCustomTags resolves them per-request into span
	// attributes appended by BuildServerSpan (each overriding a colliding built-in).
	// Empty/nil when the HCM tracing block configures no custom_tags — the
	// byte-stable no-tags path.
	CustomTags []CustomTagSpec
}
```

- [ ] **Step 4: Lift the reject → the resolve arm** in `NewConfig` (`config.go:112-114`), REPLACE:

```go
	if t.GetMaxPathTagLength() != nil {
		return nil, fmt.Errorf("tracing: max_path_tag_length is unsupported")
	}
```

with:

```go
	maxPathTagLen := uint32(256) // the reference default when ABSENT (D-MPTL-DEFAULT, SPEC-64 §11 arm 1)
	if m := t.GetMaxPathTagLength(); m != nil {
		maxPathTagLen = m.GetValue() // explicit value; an explicit 0 is PRESERVED (D-MPTL-ZERO, arm 2)
	}
```

and set it on the parsed config AFTER the provider dispatch, alongside `cfg.CustomTags = customTags` (`config.go:154`):

```go
	cfg.CustomTags = customTags
	cfg.MaxPathTagLength = maxPathTagLen
	return cfg, nil
```

- [ ] **Step 5: Remove the `max_path_tag_length` reject row** from `TestNewConfigRejectArms` in `config_test.go` — DELETE the whole table entry `:336-343` (the `verbose` row `:328-335` and the `spawn_upstream_span` row `:344-351` STAY):

```go
		{
			name: "max_path_tag_length",
			mut: func(t *testing.T) *hcmv3.HttpConnectionManager_Tracing {
				tr := otelProvider(t, envoyGrpcOTel("c", "svc"))
				tr.MaxPathTagLength = wrapperspb.UInt32(128)
				return tr
			},
		},
```

- [ ] **Step 6: Run tests — expect PASS:**

```
cd internal/tracing && go test -run 'TestNewConfig' -count=1 . && cd - && go build ./...
```
Expected: PASS — the accept test (128/256/0), the reject table (now without the `max_path_tag_length` row) still rejecting `verbose`/`spawn_upstream_span`/etc., and the build green.

- [ ] **Step 7: LIVENESS BREAKS (`-count=1`, confirm WHICH fires).** Prove three load-bearing arms are live:
  1. **explicit value:** change `maxPathTagLen = m.GetValue()` → `maxPathTagLen = 999` and confirm ONLY `TestNewConfigMaxPathTagLength/explicit` fires (`got 999, want 128`). Restore.
  2. **absent default 256:** change `maxPathTagLen := uint32(256)` → `maxPathTagLen := uint32(0)` and confirm ONLY `TestNewConfigMaxPathTagLength/absent-defaults-256` fires (`got 0, want 256`) — the `explicit`/`explicit-zero` subtests still pass (they set `m != nil`). Restore.
  3. **explicit 0 preserved (NOT coerced to 256):** change the guard to `if m := t.GetMaxPathTagLength(); m != nil && m.GetValue() != 0 {` (treating explicit 0 as absent → 256) and confirm ONLY `TestNewConfigMaxPathTagLength/explicit-zero-preserved` fires (`got 256, want 0`). Restore.

```
cd internal/tracing && go test -run 'TestNewConfigMaxPathTagLength' -count=1 -v .
```

- [ ] **Step 8: Per-task gates + commit:**

```
gofmt -l internal/tracing/config.go internal/tracing/config_test.go   # expect no output
golangci-lint run ./internal/tracing/... && go vet ./internal/tracing/...
go build ./... && cd internal/tracing && go test -count=1 . && cd -
git add internal/tracing/config.go internal/tracing/config_test.go
git commit -m "phase 64 IMPL T1: TracingConfig.MaxPathTagLength + lift the max_path_tag_length reject (resolve: default 256 / explicit incl. 0) + config tests"
```

---

## Task 2: `url.go` — the `BuildHTTPURL` truncation helper + unit matrix

**Files:**
- Create: `internal/tracing/url.go`
- Create: `internal/tracing/url_test.go`

**Interfaces:**
- Produces: `func BuildHTTPURL(scheme, host, pathAndQuery string, maxPathTagLen uint32) string` — byte-truncates `pathAndQuery` to `maxPathTagLen` bytes FIRST, then returns `scheme + "://" + host + pathAndQuery`. Consumed by Task 3's call sites.
- Consumes: nothing (pure function; stdlib only).

- [ ] **Step 1: Write the failing unit matrix** in `internal/tracing/url_test.go` (`Errorf` per row, `reference_fatalf_makes_assertions_unreachable`):

```go
package tracing

import (
	"strings"
	"testing"
)

// TestBuildHTTPURL exercises the max_path_tag_length byte-truncation helper (SPEC-64
// §11): the :path (path+query) is byte-truncated to maxPathTagLen FIRST, then
// scheme://host is prepended (NEVER truncated). Cases: under-cap (unchanged),
// over-cap (truncated to N bytes, D-MPTL-TARGET), explicit-0 (empty path →
// scheme://host only, D-MPTL-ZERO), query-cut (a cut inside the query, D-MPTL-QUERY),
// and the exact byte boundary (== cap ⇒ unchanged, proving strict `>`; ASCII so
// byte==rune, D-MPTL-TRUNCUNIT). Errorf per row.
func TestBuildHTTPURL(t *testing.T) {
	const scheme, host = "http", "h.io"
	tests := []struct {
		name          string
		pathAndQuery  string
		maxPathTagLen uint32
		want          string
	}{
		{"under-cap-unchanged", "/short", 16, "http://h.io/short"},
		{"over-cap-truncated", "/abcdefghijKLMNOPqrstuvwxyz", 16, "http://h.io/abcdefghijKLMNO"},
		{"explicit-zero-empty-path", "/somepath?x=1", 0, "http://h.io"},
		{"query-cut-inside-query", "/p?query=abcdefghijklmnop", 16, "http://h.io/p?query=abcdefg"},
		{"exact-boundary-unchanged", "/exactly16bytes!", 16, "http://h.io/exactly16bytes!"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := BuildHTTPURL(scheme, host, tc.pathAndQuery, tc.maxPathTagLen); got != tc.want {
				t.Errorf("BuildHTTPURL(%q, cap %d) = %q, want %q", tc.pathAndQuery, tc.maxPathTagLen, got, tc.want)
			}
		})
	}

	// D-MPTL-DEFAULT-PROOF (SPEC §8): a > 256-byte path under the default 256 cap
	// truncates to exactly 256 :path bytes — proving the reference default is honored
	// WITHOUT a second differential fixture. (The 307-byte path mirrors §11 arm 1.)
	t.Run("default-256-truncation", func(t *testing.T) {
		longPath := "/probe/" + strings.Repeat("a", 300) // 7 + 300 = 307 bytes
		got := BuildHTTPURL(scheme, host, longPath, 256)
		want := "http://h.io" + longPath[:256]
		if got != want {
			t.Errorf("default-256: got %q (len %d), want the 256-byte-:path form (len %d)", got, len(got), len(want))
		}
		if wantLen := len("http://h.io") + 256; len(got) != wantLen {
			t.Errorf("default-256: len(http.url) = %d, want %d (11-byte prefix + 256 :path)", len(got), wantLen)
		}
	})
}
```

- [ ] **Step 2: Run — expect FAIL** (`BuildHTTPURL` undefined — compile error):

```
cd internal/tracing && go test -run 'TestBuildHTTPURL' -count=1 -v .
```
Expected: FAIL — `undefined: BuildHTTPURL`.

- [ ] **Step 3: Write the helper** in `internal/tracing/url.go`:

```go
package tracing

// BuildHTTPURL assembles the http.url span-attribute value scheme://host+pathAndQuery,
// byte-truncating pathAndQuery (the :path pseudo-header = path+query) to maxPathTagLen
// bytes FIRST — the reference max_path_tag_length semantics (D-MPTL-TARGET / -QUERY /
// -TRUNCUNIT, SPEC-64 §11). The scheme://host prefix is NEVER truncated. A cap of 0
// yields an empty path (scheme://host only, D-MPTL-ZERO). maxPathTagLen is the resolved
// cap from TracingConfig.MaxPathTagLength (default 256; ALWAYS set by NewConfig).
func BuildHTTPURL(scheme, host, pathAndQuery string, maxPathTagLen uint32) string {
	if len(pathAndQuery) > int(maxPathTagLen) { // int() cast: cap <= math.MaxUint32, no overflow on 64-bit
		pathAndQuery = pathAndQuery[:maxPathTagLen]
	}
	return scheme + "://" + host + pathAndQuery
}
```

- [ ] **Step 4: Run — expect PASS:**

```
cd internal/tracing && go test -run 'TestBuildHTTPURL' -count=1 -v . && go build ./...
```
Expected: PASS.

- [ ] **Step 5: LIVENESS BREAKS (`-count=1`, confirm WHICH fires).**
  1. **truncation applied:** change `pathAndQuery = pathAndQuery[:maxPathTagLen]` → delete the line (leave the `if` body empty / no truncation) and confirm `over-cap-truncated`, `explicit-zero-empty-path`, `query-cut-inside-query`, AND `default-256-truncation` fire (the `under-cap`/`exact-boundary` rows still pass — they were never over-cap). Restore.
  2. **strict `>` boundary:** change `len(pathAndQuery) > int(maxPathTagLen)` → `len(pathAndQuery) >= int(maxPathTagLen)` and confirm ONLY `exact-boundary-unchanged` fires (a 16-byte path at cap 16 would truncate to `pathAndQuery[:16]` = the same string, so actually NO change — SKIP this break; instead verify boundary by inspection: `>` means a path of EXACTLY the cap length is unchanged). *(Note: `>=` with `s[:len(s)]` is a no-op for the exact-boundary case, so this break is vacuous; rely on break #1 + the `exact-boundary` row's presence to pin the boundary. Do NOT fabricate a firing break here — `reference_vacuous_break_receiver_normalizes`.)*

```
cd internal/tracing && go test -run 'TestBuildHTTPURL' -count=1 -v .
```

- [ ] **Step 6: Per-task gates + commit:**

```
gofmt -l internal/tracing/url.go internal/tracing/url_test.go   # expect no output
golangci-lint run ./internal/tracing/... && go vet ./internal/tracing/...
go build ./... && cd internal/tracing && go test -count=1 . && cd -
git add internal/tracing/url.go internal/tracing/url_test.go
git commit -m "phase 64 IMPL T2: BuildHTTPURL byte-truncation helper (:path to N bytes, scheme://host preserved) + unit matrix"
```

---

## Task 3: `accesslog_emit.go` — rewire the three URL-build sites + span-emit truncation tests

**Files:**
- Modify: `internal/filter/hcm/accesslog_emit.go` (H1 `:40`, H2 `:93`, H3 `:162`)
- Modify: `internal/filter/hcm/span_emit_test.go` (add a `spanAttr` helper + H1 + H2 truncation tests)

**Interfaces:**
- Consumes: `tracing.BuildHTTPURL` (Task 2); `f.tracingConfig.MaxPathTagLength` (Task 1); `newTracingFilter`/`fakeExporter`/`knownDecision` (`span_emit_test.go:59`/`:33`/`:86`).
- Produces: nothing new (the three call sites now truncate `http.url`).

- [ ] **Step 1: Write the failing span-emit truncation tests** in `internal/filter/hcm/span_emit_test.go` (append at end). The config sets `MaxPathTagLength: 16` explicitly (the ZERO-VALUE CAP TRAP — a bare literal would truncate to empty). `Errorf` per property:

```go
// spanAttr returns the string value of the named span attribute (or "" if absent).
func spanAttr(s *tracing.Span, key string) string {
	for _, kv := range s.Attrs {
		if kv.Key == key {
			return kv.Str
		}
	}
	return ""
}

// TestSpanEmit_H1_MaxPathTagLengthTruncates: the H1 call site (accesslog_emit.go:40)
// byte-truncates the :path portion of http.url to TracingConfig.MaxPathTagLength (16),
// preserving the scheme://host prefix (SPEC-64 §3.4, mirrors §11 arm 0).
func TestSpanEmit_H1_MaxPathTagLengthTruncates(t *testing.T) {
	fe := &fakeExporter{}
	cfg := &tracing.TracingConfig{RandomSampling: 100, MaxPathTagLength: 16}
	f := newTracingFilter(t, fe, cfg)

	req, _ := http.NewRequest("GET", "http://example.com/abcdefghijKLMNOPqrstuvwxyz", nil)
	req.Proto = "HTTP/1.1"
	req.Host = "h.io"

	f.emitAccessLog(req, 200, 0, cluster.Endpoint{}, time.Now(), nil, knownDecision())

	spans := fe.captured()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if got := spanAttr(spans[0], "http.url"); got != "http://h.io/abcdefghijKLMNO" {
		t.Errorf("http.url = %q, want http://h.io/abcdefghijKLMNO (:path truncated to 16 bytes)", got)
	}
}

// TestSpanEmit_H2_MaxPathTagLengthTruncates: the H2 call site (accesslog_emit.go:93)
// mirror — truncates req.Path to MaxPathTagLength, scheme://authority preserved.
func TestSpanEmit_H2_MaxPathTagLengthTruncates(t *testing.T) {
	fe := &fakeExporter{}
	cfg := &tracing.TracingConfig{RandomSampling: 100, MaxPathTagLength: 16}
	f := newTracingFilter(t, fe, cfg)

	req := h2.H2Request{Method: "GET", Path: "/abcdefghijKLMNOPqrstuvwxyz", Scheme: "http", Authority: "h.io"}
	f.emitAccessLogH2(req, 200, 0, cluster.Endpoint{}, time.Now(), nil, knownDecision())

	spans := fe.captured()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if got := spanAttr(spans[0], "http.url"); got != "http://h.io/abcdefghijKLMNO" {
		t.Errorf("http.url = %q, want http://h.io/abcdefghijKLMNO", got)
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (the call sites still build `url` inline without truncation, so `http.url` is the full untruncated URL):

```
cd internal/filter/hcm && go test -run 'TestSpanEmit_H._MaxPathTagLengthTruncates' -count=1 -v .
```
Expected: FAIL — `http.url = "http://h.io/abcdefghijKLMNOPqrstuvwxyz", want http://h.io/abcdefghijKLMNO` (both H1 + H2).

- [ ] **Step 3: Rewire the three URL-build sites** in `internal/filter/hcm/accesslog_emit.go`:

**H1 — `:40`:** REPLACE

```go
		url := scheme + "://" + r.Host + r.URL.RequestURI()
```
with
```go
		url := tracing.BuildHTTPURL(scheme, r.Host, r.URL.RequestURI(), f.tracingConfig.MaxPathTagLength)
```

**H2 — `:93`:** REPLACE

```go
		url := scheme + "://" + req.Authority + req.Path
```
with
```go
		url := tracing.BuildHTTPURL(scheme, req.Authority, req.Path, f.tracingConfig.MaxPathTagLength)
```

**H3 — `:162`:** REPLACE

```go
		url := scheme + "://" + r.Host + r.URL.RequestURI()
```
with
```go
		url := tracing.BuildHTTPURL(scheme, r.Host, r.URL.RequestURI(), f.tracingConfig.MaxPathTagLength)
```

*(Note: H1 and H3 have identical text; make BOTH edits — the H1 site is inside `emitAccessLog` `:25`, the H3 site is inside the H3 variant near `:149`. Verify by line: `:40` and `:162`.)*

- [ ] **Step 4: Run — expect PASS** (and confirm `span_test.go:100` — the `BuildServerSpan`-direct `http.url` assertion — STAYS green, since truncation is at the call site not in `BuildServerSpan`):

```
go build ./... && cd internal/filter/hcm && go test -count=1 . && cd - && go test -count=1 ./internal/tracing/...
```
Expected: PASS — the two new truncation tests, ALL existing `span_emit_test.go` tests (they never assert `http.url`, so the zero-value-cap on their bare-literal configs is harmless), and `internal/tracing` (incl. `span_test.go:100`).

- [ ] **Step 5: LIVENESS BREAKS (`-count=1`, confirm WHICH fires).** Prove BOTH call-site rewirings are live by reverting each in isolation:
  1. **H1 site:** revert `:40` to `url := scheme + "://" + r.Host + r.URL.RequestURI()` and confirm ONLY `TestSpanEmit_H1_MaxPathTagLengthTruncates` fires (`got ...KLMNOPqrstuvwxyz, want ...KLMNO`); the H2 test still passes. Restore.
  2. **H2 site:** revert `:93` to `url := scheme + "://" + req.Authority + req.Path` and confirm ONLY `TestSpanEmit_H2_MaxPathTagLengthTruncates` fires. Restore. *(The H3 site `:162` has no unit test — it is identical to H1 and is covered end-to-end by the `0107` fixture's H1/H2 path plus the shared helper; do NOT fabricate an H3 unit break.)*

```
cd internal/filter/hcm && go test -run 'TestSpanEmit_H._MaxPathTagLengthTruncates' -count=1 -v .
```

- [ ] **Step 6: Per-task gates + commit:**

```
gofmt -l internal/filter/hcm/accesslog_emit.go internal/filter/hcm/span_emit_test.go   # expect no output
golangci-lint run ./internal/filter/hcm/... && go vet ./internal/filter/hcm/...
go build ./... && go test -count=1 ./internal/filter/hcm/... ./internal/tracing/...
git add internal/filter/hcm/accesslog_emit.go internal/filter/hcm/span_emit_test.go
git commit -m "phase 64 IMPL T3: rewire the 3 accesslog_emit URL-build sites (H1/H2/H3) through BuildHTTPURL + span-emit truncation tests"
```

---

## Task 4: Zipkin encoder unit test — a truncated `http.url` surfaces in the `tags` map

**Files:**
- Modify: `internal/tracing/zipkin_test.go` (add one test; `encoding/json` already imported `:6`)

**Interfaces:**
- Consumes: `BuildHTTPURL` (Task 2, provider-neutral truncation), `BuildServerSpan` (unchanged), `encodeZipkinSpans([]*Span, id128, shared bool)` (`zipkin.go:78`), `freshDecision`/`freshInputs` (`span_test.go:12`/`:34`), the `b[1:len(b)-1]` single-span decode idiom (`zipkin_test.go:116`).

- [ ] **Step 1: Write the failing test** in `internal/tracing/zipkin_test.go` (append at end; mirror `TestZipkinEncodeResolvedEnvironmentTag` `:608`):

```go
// TestZipkinEncodeTruncatedHTTPURL: a truncated http.url (built via BuildHTTPURL, the
// provider-neutral truncation) surfaces VERBATIM in the Zipkin v2 `tags` map — the
// Zipkin encoder carries the already-truncated URL (SPEC-64 §3.5/§8; truncation is at
// the call site, NOT in the encoder). node_id/zone stay dropped by the encoder.
func TestZipkinEncodeTruncatedHTTPURL(t *testing.T) {
	d := freshDecision()
	in := freshInputs()
	in.URL = BuildHTTPURL("http", "h.io", "/abcdefghijKLMNOPqrstuvwxyz", 16) // :path truncated to 16 bytes
	in.NodeID = "node-x"
	in.Zone = "zone-y"
	start := time.Unix(0, 1_000_000_000)
	end := start.Add(10 * time.Millisecond)
	span := BuildServerSpan(d, in, nil, start, end)

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
	if got.Tags["http.url"] != "http://h.io/abcdefghijKLMNO" {
		t.Errorf("tags[http.url] = %q, want http://h.io/abcdefghijKLMNO (truncated)", got.Tags["http.url"])
	}
	if _, ok := got.Tags["node_id"]; ok {
		t.Errorf("tags[node_id] present, want dropped by the Zipkin encoder")
	}
	if _, ok := got.Tags["zone"]; ok {
		t.Errorf("tags[zone] present, want dropped by the Zipkin encoder")
	}
}
```

- [ ] **Step 2: Run — expect PASS** (the truncated URL flows into `Attrs` via `BuildServerSpan`; `encodeZipkinSpans` builds `tags` from `Attrs`, dropping node_id/zone at `zipkin.go:89`):

```
cd internal/tracing && go test -run 'TestZipkinEncodeTruncatedHTTPURL' -count=1 -v .
```
Expected: PASS.

- [ ] **Step 3: LIVENESS BREAK (`-count=1`).** Temporarily add `http.url` to the Zipkin encoder's built-in drop condition (`zipkin.go:89`, change `if kv.Key == "node_id" || kv.Key == "zone"` → `... || kv.Key == "http.url"`) and confirm the `tags[http.url]` assertion fires (`= "", want http://h.io/abcdefghijKLMNO`); restore.

```
cd internal/tracing && go test -run 'TestZipkinEncodeTruncatedHTTPURL' -count=1 -v .
```

- [ ] **Step 4: Gates + commit:**

```
gofmt -l internal/tracing/zipkin_test.go && golangci-lint run ./internal/tracing/...
cd internal/tracing && go test -count=1 . && cd -
git add internal/tracing/zipkin_test.go
git commit -m "phase 64 IMPL T4: Zipkin encoder truncated-http.url unit test (provider-neutral truncation)"
```

---

## Task 5: New OTLP fixture `0107-tracing-max-path-tag-length` (cross-side `http.url` VALUE-equality)

**Files:**
- Create: `test/fixtures/0107-tracing-max-path-tag-length/{envoy.yaml,envoy-go.yaml,expectations.yaml,README.md}`
- Create: `test/fixtures/0107-tracing-max-path-tag-length/driver/driver.go`
- Modify: `test/differential/runner_test.go` (blank-import after the `0106` line `:133`)

**Approach:** CLONE `0106-tracing-custom-tags-environment` verbatim, then apply the enumerated edits. Do NOT mutate `0087`/`0088`/`0102`/`0105`/`0106` (`reference_differential_fixture_dispatch_constraint` — one fixture dir = one runner branch). RE-DERIVE the next-free number at implementation: `ls -d test/fixtures/[0-9]*/ | tail -1` — expect `0106-tracing-custom-tags-environment`, so `0107` is free.

**Fixture semantics (SPEC §8):** the HCM `tracing` block carries `max_path_tag_length: {value: 16}` (dropping the `0106` `custom_tags` block). The driver drives a GET with a LONG ASCII query-less path (> 16 bytes); the reference AND subject both truncate the `:path` to 16 bytes, so `http.url` is the SAME truncated value cross-side. Assert `http.url` VALUE-equality on every span (see the authority-encoding risk note in File Structure — lead with VALUE-equality, fall back to `:path`-portion equality only if the run shows an authority divergence). `BackendCount` ≥ 1 (`reference_differential_backendcount_min_one`).

- [ ] **Step 1: Clone the fixture dir:**

```
cp -r test/fixtures/0106-tracing-custom-tags-environment test/fixtures/0107-tracing-max-path-tag-length
```

- [ ] **Step 2: Swap the `custom_tags` block for `max_path_tag_length` in BOTH bootstrap templates.** In `envoy.yaml` AND `envoy-go.yaml`, REMOVE the `custom_tags:` block (the `env_path`/`environment`/`name: PATH` entry, sibling of `provider:`/`random_sampling:` under `tracing:`) and ADD, in its place (verify indentation against each file's existing siblings):

```yaml
                  max_path_tag_length:
                    value: 16
```

Also change `service_name: "0106"` → `service_name: "0107"` in both yamls, and update the `# phase 63 ...` header comment in both to:

```
# phase 64: the tracing block sets max_path_tag_length: 16 — the http.url span
# attribute's :path (path+query) is byte-truncated to 16 bytes (the scheme://host
# prefix preserved), exercising the max_path_tag_length numeric knob.
```

- [ ] **Step 3: Edit `driver/driver.go`** — the enumerated changes on the clone:
  1. Package doc (`:1`–`:~90`) + `fixtureName` const (`:98`) → `"0107-tracing-max-path-tag-length"`; reword the doc to describe the numeric knob (a long-ASCII-path GET; `http.url` truncated to 16 bytes; cross-side VALUE-equality on the truncated form; no request-header / no custom_tags).
  2. `refListenerPort` (`:103`) → `10107`.
  3. `wantServiceName` (`:128`) → `"0107"`.
  4. `probePath` (`:115`) → a LONG ASCII query-less path (> 16 bytes): `"/abcdefghijklmnopqrstuvwxyz0123456789"` (37 bytes). Keep `probeHost` (`:116`) = `"trace.example"`.
  5. REPLACE the `customTagKey` const (`:140`) with the truncation expectation:
  ```go
	// phase 64: max_path_tag_length in both bootstraps' `tracing` block caps the
	// http.url :path (path+query) at maxPathTagLen bytes. probePath is longer, so
	// the truncated http.url is deterministic AND identical cross-side (the
	// scheme://host prefix is echoed verbatim by both proxies — SPEC-64 §11 arm 0).
	maxPathTagLen    = 16
	wantTruncatedURL = "http://trace.example/abcdefghijklmno" // "http://"+probeHost + probePath[:16]
  ```
  *(Verify at IMPL: `probePath[:16]` == `/abcdefghijklmno` — a leading `/` + 15 chars = 16 bytes; and `wantTruncatedURL` == `"http://" + probeHost + probePath[:maxPathTagLen]`.)*
  6. Change the per-span `http.url` assertion (`:495`, currently `assertAttrPresent(t, side, i, attrs, "http.url") // value varies by host/path encoding`) to VALUE-equality:
  ```go
		assertAttrString(t, side, i, attrs, "http.url", wantTruncatedURL) // truncated :path, deterministic cross-side (SPEC-64 §8)
  ```
  7. REMOVE the `assertCustomTag` function (`:569-582`) AND its call site (grep the driver for `assertCustomTag(` and delete the call — the `0106` driver invokes it inside `AssertStats`). The `spanAttrMap`/`mapKeys` helpers may become unused after removing `assertCustomTag` — if `go vet`/`golangci-lint` flags them as unused, delete them too (or keep if still referenced by another assertion). `FIXTURE_0106_DUMP` (`:415`) → `FIXTURE_0107_DUMP`.
  8. Grep the driver for any remaining `0106`/`env_path`/`environment`/`PATH`/`custom` and reconcile (the package doc, the dump helper comment, the `# phase 63` references). Leave the type name `traceOTLPDriver` and the compile-time interface assertions as-is (a rename is cosmetic; packages are isolated).

- [ ] **Step 4: Update `expectations.yaml` + `README.md`** — reflect the new fixture name/purpose (the `max_path_tag_length: 16` knob; the `http.url` `:path` truncated to 16 bytes; cross-side VALUE-equality on the truncated form; the default-256/explicit-0/query-cut edges are the deterministic `url_test.go` unit tests, NOT this fixture). Keep the `0106` framing otherwise (the `upstream_cluster` framework-gap key-presence note stays; span count unchanged — `numPlain`/`numCont`/`numTotal` unchanged). Grep the cloned files for stray `0106`/`env_path`/`environment`/`custom_tags` and reconcile.

- [ ] **Step 5: Register + run the fixture.** Add the blank import to `runner_test.go` immediately AFTER the `0106` line (`:133`):

```go
	_ "github.com/pgdad/envoy-go/test/fixtures/0107-tracing-max-path-tag-length/driver"
```
Then (Docker required):

```
go test ./test/differential/ -run 'TestDifferential/0107-tracing-max-path-tag-length' -count=1 -v
```
Expected: PASS (both sides emit `http.url == "http://trace.example/abcdefghijklmno"`). **If it FAILS on a cross-side `http.url` authority mismatch** (the reference emitting a port/normalized authority the arm-0 probe did not predict), switch to the FALLBACK per the File-Structure risk note: assert the `:path` portion (after stripping each side's `scheme://authority`) equals `probePath[:maxPathTagLen]` per side + cross-side path-portion equality; RECORD the switch in PROGRESS. Use the FULL `-run 'TestDifferential/0107-tracing-max-path-tag-length'` selector (`reference_differential_run_selector`), NEVER a bare `0107`.

- [ ] **Step 6: LIVENESS BREAK (`-count=1`, confirm WHICH fires).** Break the ASSERTION's expected value ONLY (NOT the yaml — `reference_vacuous_break_receiver_normalizes`): temporarily change the const `wantTruncatedURL = "http://trace.example/abcdefghijklmno"` → `"http://trace.example/WRONGWRONGWRONG"`. The yaml still emits the real truncated URL, so the assert compares against the wrong value → confirm BOTH sides' `http.url` assertion fires with `http.url = "http://trace.example/abcdefghijklmno", want http://trace.example/WRONGWRONGWRONG` (the `http.url` value assertion, not another). Restore.

```
go test ./test/differential/ -run 'TestDifferential/0107-tracing-max-path-tag-length' -count=1 -v
```

- [ ] **Step 7: Gates + commit** (Docker required for the differential; if the subagent lacks Docker, the controller runs it at stage-close — note the deferral in the commit):

```
gofmt -l test/fixtures/0107-tracing-max-path-tag-length/driver/driver.go && golangci-lint run ./test/...
go build ./...
git add test/fixtures/0107-tracing-max-path-tag-length/ test/differential/runner_test.go
git commit -m "phase 64 IMPL T5: 0107-tracing-max-path-tag-length OTLP fixture (fixtures 108 -> 109; cross-side truncated http.url value-equality)"
```

---

## Task 6: `FuzzHCMConfigParse` seed — one `max_path_tag_length` seed (fuzzers stay 55)

**Files:**
- Modify: `internal/filter/hcm/fuzz_test.go` (add one seed after the phase-63 `withEnvTags` seed `:74`; ADD the `wrapperspb` import)

- [ ] **Step 1: Reconcile the fuzzer count BEFORE** (`reference_fuzzer_count_docs_drift`):

```
grep -rhoE '^func Fuzz[A-Za-z0-9]+' --include='*.go' . | sort -u | wc -l    # expect 55
```

- [ ] **Step 2: Add the `wrapperspb` import** to `fuzz_test.go` (`:3-19` import block), alongside the other `google.golang.org/protobuf/types/known/*` imports:

```go
	"google.golang.org/protobuf/types/known/wrapperspb"
```

- [ ] **Step 3: Add the seed** in `FuzzHCMConfigParse`, after the phase-63 `withEnvTags` `f.Add` (`fuzz_test.go:74`):

```go
	// Phase 64: a max_path_tag_length seed (incl. an explicit 0) exercises the tracing
	// numeric-knob resolve arm (the GetMaxPathTagLength resolve REPLACED the former
	// reject). The block sets no provider, so it errors at "provider required" AFTER
	// the resolve — the fuzz asserts no-panic + hcm:-prefixed.
	withMaxPathTag := mkHCM(func(h *hcmv3.HttpConnectionManager) {
		h.Tracing = &hcmv3.HttpConnectionManager_Tracing{
			MaxPathTagLength: wrapperspb.UInt32(0),
		}
	})
	f.Add(withMaxPathTag.GetTypeUrl(), withMaxPathTag.GetValue())
```

- [ ] **Step 4: Run the fuzz target briefly (seed corpus only) + reconcile the count AFTER:**

```
cd internal/filter/hcm && go test -run 'FuzzHCMConfigParse' -count=1 . && cd -
grep -rhoE '^func Fuzz[A-Za-z0-9]+' --include='*.go' . | sort -u | wc -l    # expect STILL 55 (a seed, not a new func)
```
Expected: PASS; count STILL 55.

- [ ] **Step 5: Gates + commit:**

```
gofmt -l internal/filter/hcm/fuzz_test.go && golangci-lint run ./internal/filter/hcm/... && go vet ./internal/filter/hcm/...
go build ./... && go test -count=1 ./internal/filter/hcm/...
git add internal/filter/hcm/fuzz_test.go
git commit -m "phase 64 IMPL T6: FuzzHCMConfigParse max_path_tag_length seed (fuzzers stay 55)"
```

---

## Task 7: `BEHAVIOR_CONTRACT.md` — flip `max_path_tag_length` REJECT → CONSUMED

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the tracing STRICT-REJECT clause `:686`; RE-DERIVE the exact lines at IMPL)

- [ ] **Step 1: RE-DERIVE the drifted lines** (`feedback_brief_citations_not_evidence`):

```
grep -n 'max_path_tag_length\|STRICT-REJECT set (ADR-0080)' docs/envoy-go/BEHAVIOR_CONTRACT.md
```

- [ ] **Step 2: Edit the STRICT-REJECT clause** (`:686`) — REMOVE `max_path_tag_length` from the strict-reject enumeration (currently `..., verbose, max_path_tag_length, a PRESENT spawn_upstream_span, ...` → `..., verbose, a PRESENT spawn_upstream_span, ...`), and ADD a CONSUMED clause for it (near the tracing ACCEPT enumeration). The new CONSUMED wording:

```
`max_path_tag_length` is CONSUMED (byte-truncates the `http.url` span attribute's `:path` (path+query) portion to N bytes; default 256 when absent; an explicit 0 = empty path (`scheme://host` only); the `scheme://host` prefix is never truncated; applied on both the OTLP and Zipkin exporters).
```

The sibling tracing knob rejects (`verbose`/`spawn_upstream_span`/`custom_tags metadata`/`http_service`/`resource_detectors`/`sampler`/`google_grpc`/non-`HTTP_JSON` Zipkin/`split_spans_for_request`/empty clusters) STAY.

- [ ] **Step 3: Verify no other `max_path_tag_length` reject reference remains** in the contract:

```
grep -n 'max_path_tag_length' docs/envoy-go/BEHAVIOR_CONTRACT.md   # every hit now describes CONSUMPTION, not rejection
```

- [ ] **Step 4: Commit** (docs-only, no gates):

```
git add docs/envoy-go/BEHAVIOR_CONTRACT.md
git commit -m "phase 64 IMPL T7: BEHAVIOR_CONTRACT max_path_tag_length REJECT -> CONSUMED (truncates http.url :path; both exporters)"
```

---

## Task 8: Verify — the six-gate + the full 109-dir differential

**Files:** none (a no-commit verification gate; controller-run on the frozen HEAD).

- [ ] **Step 1: The six-gate** (all must be GREEN):

```
gofmt -l internal/ test/ cmd/                                  # expect no output
go vet ./...                                                    # clean
go build ./...                                                 # clean
go mod tidy -diff && git diff --exit-code go.mod go.sum        # EMPTY (modules stay 2)
golangci-lint run ./...                                        # exit 0
go test -race -count=1 ./internal/tracing/... ./internal/filter/hcm/...   # all ok
```

- [ ] **Step 2: The full differential** (Docker required; ~6 min):

```
go test ./test/differential/ -count=1
```
Expected: `ok ... /test/differential`, EXIT 0, byte-stable except the new `0107` (the 108 pre-existing dirs byte-stable — the default-256 behavior change is a no-op for the short-path corpus, SPEC §3.6). If any pre-existing tracing fixture surfaces a > 256-byte path (none anticipated), RECORD it as a pre-existing latent divergence, do NOT mask it.

- [ ] **Step 3: Reconcile the exit counts** and record in PROGRESS (Task-8 evidence block): stat surface **1201** · fixtures **109** (`ls -d test/fixtures/[0-9]*/ | wc -l`) · fuzzers **55** · BackendKind **38** · `go mod tidy -diff` empty · DECISIONS tail still `## ADR-0284` (ADR-0285 body lands at T9).

---

## Task 9: ADR-0285 body + STATE + ROADMAP (row 64 `done`) + PROGRESS close + router roll

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0285 §Decision/§Consequences — SPEC §13 drafted §Context)
- Modify: `docs/envoy-go/STATE.md` (active-phase header → `phase 64 IMPL done`; NEXT = the roller's next self-pick)
- Modify: `docs/envoy-go/ROADMAP.md` (row 64 `in-progress` → `done`; the LIVE Observability deferred sentence UNCHANGED)
- Modify: `docs/envoy-go/phases/64-tracing-max-path-tag-length/PROGRESS.md` (close; fill the verify evidence + liveness log + landed commits)
- Modify: `next-prompt.txt` (roll the router to the NEXT phase's BRAINSTORM — the roller SELF-PICKS per the 2026-07-12 standing directive)

- [ ] **Step 1: ADR-0285 body.** Append `## ADR-0285 — tracing max_path_tag_length ...` to `DECISIONS.md` after ADR-0284 (`:16834`). §Context is the SPEC §13 draft (paste + tighten); ADD §Decision (the `MaxPathTagLength uint32` field + the `BuildHTTPURL` helper + the three call-site wirings; default 256 / explicit incl. 0; provider-neutral single-URL truncation; NO new reject; ADR-0045 single-flat-row, helper folded into ADR-0285 per the phase-59/62/63 precedent) and §Consequences (+0 stats / +1 fixture `0107` / +0 fuzzers-seed / +0 packages / +0 modules; the default-256 behavior change is byte-stable for the short-path corpus and CLOSES a latent > 256 divergence; a colliding custom `http.url` tag still upsert-overrides the truncated built-in).

- [ ] **Step 2: ROADMAP row 64 → `done`.** Flip the `in-progress` → `done` on row 64 (`:125`) and append the IMPL landing summary (ADR-0285; the parse arm + `BuildHTTPURL` + three call sites; +1 fixture `0107`; fuzzers/stats/packages/modules +0). The Observability family STAYS OPEN.

- [ ] **Step 3: Re-run the sentinel check-(2) grep + CONFIRM the LIVE Observability deferred sentence is UNCHANGED** (`reference_sentinel_deferred_sentence_live_vs_historical` — `max_path_tag_length` was never IN it, a §8-tier candidate, so this row does NOT narrow it):

```
grep -oE 'remaining deferred \(not-yet-chartered\) candidates:[^.]*\.' docs/envoy-go/ROADMAP.md
```
Expected: THREE live sentences (HTTP/3, xDS, Observability); the Observability one lists `custom_tags (metadata)` + OTLP-metrics + `spawn_upstream_span`/`http_service`/force-trace — UNCHANGED (no `max_path_tag_length` mention added or removed).

- [ ] **Step 4: STATE.md** — flip the active-phase header to `phase 64 (tracing-max-path-tag-length) IMPL done` with the landing summary + NEXT (the roller's self-pick for the next phase per the standing directive; sentinel does NOT fire — re-run the three mechanical checks to confirm).

- [ ] **Step 5: PROGRESS.md** — mark all 9 tasks `[x]`, fill the baseline block, the liveness-break log (every `-count=1` break + WHICH fired), the Task-8 verify evidence (six-gate + 109-dir differential, verbatim), and the landed task commits.

- [ ] **Step 6: Roll `next-prompt.txt`** to the NEXT phase's BRAINSTORM stage (the roller SELF-PICKS the smallest defensible next subject per the 2026-07-12 standing directive; record the pick + rejected alternatives at the BRAINSTORM, NOT here). Update the STATUS block, the "What THIS session does" section, the counts (fixtures 109, DECISIONS tail ADR-0285, next-free ADR-0286), and the sentinel re-check note.

- [ ] **Step 7: Commit** (docs-only; the controller squashes + pushes at stage-close per `feedback_subagents_no_push`):

```
git add docs/envoy-go/DECISIONS.md docs/envoy-go/STATE.md docs/envoy-go/ROADMAP.md docs/envoy-go/phases/64-tracing-max-path-tag-length/PROGRESS.md next-prompt.txt
git commit -m "phase 64 IMPL T9: ADR-0285 body + STATE + ROADMAP row 64 done + PROGRESS close + router roll"
```

---

## Self-Review (run against SPEC §10 / §12 with fresh eyes)

**Spec coverage** — every SPEC §10 task maps to a PLAN task: §10.1 (config model + parse arm) → T1; §10.2 (`BuildHTTPURL` + unit matrix) → T2; §10.3 (call-site rewiring + reach-the-span test) → T3; §10.4 (Zipkin encoder unit test) → T4; §10.5 (`0107` fixture) → T5; §10.6 (fuzz seed) → T6; §10.7 (BEHAVIOR_CONTRACT) → T7; §10.8 (verify) → T8; §10.9 (ADR/STATE/ROADMAP/router) → T9. SPEC §12 edit-site roster: `config.go:25-40`/`:112-114`/`:154` → T1; the `url.go` helper → T2; `accesslog_emit.go:40`/`:93`/`:162` → T3; `config_test.go:336-343` → T1; `url_test.go` → T2; `zipkin_test.go` → T4; `span_emit_test.go` (the reach-the-span test) → T3; `fuzz_test.go` → T6; `0107` fixture → T5; docs → T7/T9. `span.go`/`BuildServerSpan`/`upsertAttr` confirmed UNCHANGED (T3 Step 4).

**Placeholder scan** — no TBD/TODO; every code step shows the actual code; the T5 driver edits are enumerated with exact consts.

**Type consistency** — `BuildHTTPURL(scheme, host, pathAndQuery string, maxPathTagLen uint32) string` is IDENTICAL in T2 (definition), T3 (call sites), and T4 (Zipkin test). `TracingConfig.MaxPathTagLength uint32` is IDENTICAL in T1 (field), T3 (`f.tracingConfig.MaxPathTagLength`), and the test literals. `spanAttr(s *tracing.Span, key string) string` is defined once in T3.
