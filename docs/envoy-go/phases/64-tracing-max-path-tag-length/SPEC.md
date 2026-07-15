# SPEC 64 — tracing `max_path_tag_length` (the FIFTEENTH Observability-family row; the FIRST tracing NUMERIC-knob row — lifts the `internal/tracing/config.go:112-114` `max_path_tag_length is unsupported` reject and HONORS the knob: truncate the `:path` portion of the ALREADY-emitted `http.url` built-in span attribute to N bytes [default 256; explicit 0 = empty path] on BOTH the OTLP and Zipkin exporters; anticipated ONE new OTLP fixture / +0 stats / +0 fuzzers / +0 packages / +0 modules — a SINGLE FLAT ROW, ADR-0285)

> **Stage:** SPEC (lifecycle-state 1 → 2). Docs-only; NO production `.go` changes at this stage. Fresh worktree `.worktrees/phase-64-spec`, branch `phase-64-tracing-max-path-tag-length-spec`, off master, per `feedback_git_worktrees`.
>
> **ANCHORS ADR-0285 §Context DRAFT** (§Decision/§Consequences land at the phase-64 IMPL per ADR-0044; DECISIONS tail STAYS **ADR-0284** at this SPEC).
>
> **Baselines re-verified against master tip `7d724423` (the phase-64 BRAINSTORM squash):** stat surface **1201** · fixtures **108** (`ls -d test/fixtures/[0-9]*/ | wc -l`; tail `0106-tracing-custom-tags-environment`) · fuzzers **55** (verified `55` actual `^func Fuzz`) · BackendKind tail **38** (`H2GoawayResponder`) · DECISIONS tail **ADR-0284** (next-free **ADR-0285**) · new Go packages **0** · go.mod modules **2** (`quic-go` direct + `qpack` indirect). Counts UNCHANGED at this SPEC (docs-only). Every `file:line` below was RE-DERIVED from source this session (`feedback_brief_citations_not_evidence`) — the roster is §12.

---

## 1. Purpose / Mission

Phase 64 lifts the one `max_path_tag_length` reject (`internal/tracing/config.go:112-114`) and HONORS the knob. Today envoy-go emits the `http.url` built-in span attribute UNTRUNCATED (`span.go:70`, built inline at the three `accesslog_emit.go` URL-construction sites as `scheme + "://" + host + pathAndQuery`) and STRICT-REJECTS `max_path_tag_length` outright. After phase 64, an operator configuring `tracing: { max_path_tag_length: { value: 16 } }` on the HCM gets an `http.url` span attribute whose **`:path` portion** (path+query) is byte-truncated to 16 bytes — the `scheme://host` prefix untouched — on **both** the OTLP and Zipkin exporters (the single URL string feeds the shared `SpanInputs.URL` → `Span.Attrs` seam consumed by both). Critically, when the field is **ABSENT** the reference caps at **256** (not unlimited), so today's UNTRUNCATED emission ALREADY DIVERGES for any request path longer than 256 bytes — a latent gap this row closes. The existing tracing corpus (`0087`/`0088`/`0102`/`0105`/`0106`) drives short paths (< 256), so honoring the default is byte-stable for the whole 108-dir differential (§3.6).

This is the FIRST tracing NUMERIC-knob row and a DIFFERENT shape from the phase-59/62/63 `custom_tag` rows: it touches NEITHER the `custom_tags` parse NOR the `ResolveCustomTags` seam — it sits one layer out on the `http.url` built-in's URL construction. The novel production code is narrow: (a) parse one `UInt32Value` with a default-256 fallback into a new `TracingConfig.MaxPathTagLength uint32` scalar; (b) a small byte-truncation helper applied at the three URL-build sites. `BuildServerSpan`, `Span.Attrs`, `upsertAttr`, `ResolveCustomTags`, both exporters, and the entire custom-tag machinery are UNCHANGED.

ADR-0285 §Context is DRAFTED here (§13); §Decision/§Consequences at the IMPL (ADR-0044). **All BRAINSTORM D-MPTL-* questions are DISPOSED at this SPEC** — the empirical arms via LIVE probes against `envoyproxy/envoy:contrib-v1.37.2` (§11, FRESH container per arm, `reference_probe_fresh_container_per_arm`):

- **D-MPTL-REFTRUNC** — CONFIRMED LIVE (§11 arm 0): the reference OTLP tracer truncates the `http.url` tag at `max_path_tag_length`. The row is cleanly differential-provable; NO re-scope. The truncation is applied by Envoy's generic `Tracing::HttpTracerUtility` (provider-neutral — `Http::Utility::buildOriginalUri(headers, max_path_length)` runs in the shared span-finalize path before any tracer-specific encoder), so it applies IDENTICALLY to Zipkin; envoy-go mirrors this provider-neutrally by truncating the single `http.url` string that feeds both exporters (§3.3).
- **D-MPTL-TARGET** — PINNED (§11 arm 0): the `:path` portion (path+query) is truncated; the `scheme://host` prefix is added AFTER and is NEVER truncated (a 27-byte path under an 11-byte host truncated to exactly 16 bytes of `:path`, host intact).
- **D-MPTL-DEFAULT** — PINNED (§11 arm 1): an ABSENT `max_path_tag_length` caps at **256** (a 307-byte path emitted as `http://h.io` + 256 path bytes = 267). The load-bearing behavior-change decision — honoring it fixes the latent > 256 divergence while staying byte-stable for the < 256 corpus (§3.6).
- **D-MPTL-ZERO** — PINNED (§11 arm 2): an EXPLICIT `max_path_tag_length: 0` truncates the path to EMPTY → `http.url = scheme://host` (NOT "unlimited"; the explicit 0 is preserved, the phase-46 explicit-0 sampling precedent).
- **D-MPTL-QUERY** — PINNED (§11 arm 3): the query string IS included in the truncated `:path`; a cut can land INSIDE the query (`/p?query=abcdefghijklmnop` at cap 16 → `/p?query=abcdefg`). `:path` = path+query = envoy-go's `r.URL.RequestURI()` (H1/H3) and `req.Path` (H2).
- **D-MPTL-TRUNCUNIT** — PINNED BYTES (§11): Envoy's `absl::string_view::substr` is byte-indexed; Go `s[:n]` slices bytes. Every observed count is byte-consistent (16 / 256 / 0). The fixture (§8) uses an ASCII path so byte==rune parity is guaranteed; the unit test asserts byte truncation (§10).
- **D-MPTL-TRUNC-LOCATION** — DECIDED (§3.3): **Option A** — a shared exported helper `tracing.BuildHTTPURL(scheme, host, pathAndQuery string, maxPathTagLen uint32) string` (GREP-collision-checked, §11 note), called at the three `accesslog_emit.go` URL-build sites, replacing the inline concatenation. Option B (threading the raw path+cap through `SpanInputs` into `BuildServerSpan`) is REJECTED (fragile URL-boundary re-derivation, `SpanInputs`/signature growth).
- **D-MPTL-REJECT** — PINNED (§6, source re-derivation): `max_path_tag_length` is a `google.protobuf.UInt32Value` with NO PGV numeric constraint on this field (`.pb.validate.go` does only the no-op embedded-wrapper validation) — absent + explicit-0 both valid; NO new structural reject. The sibling tracing rejects STAY loud with distinct substrings.
- **D-MPTL-FIXTURE** — DECIDED (§8): ONE new OTLP fixture `0107-tracing-max-path-tag-length` (fixtures **108 → 109**), a small explicit cap + a long path, cross-side `http.url` **VALUE-equality** (deterministic from the request — NO harness env-injection). New dir; break-proven live.
- **D-MPTL-DEFAULT-PROOF** — DECIDED (§8): the default-256 truncation is proven by a UNIT test on the helper (LEAN); NO second > 256-path fixture. The ADR-0045 escape-valve stays UNCONSUMED.
- **D-MPTL-FUZZSEED** — DECIDED (§6): a seed to the existing `FuzzHCMConfigParse`; fuzzer count STAYS **55**.
- **D-MPTL-SPLIT** — DECIDED (§3.0): a SINGLE FLAT ROW (~8–11 tasks), ADR-0045 escape-valve UNCONSUMED.
- **D-MPTL-DOCSHAPE** — the full RE-DERIVED edit-site roster (§12).

No PLAN-time empirical question remains; the PLAN is a mechanical TDD decomposition.

### 1.1 Every BRAINSTORM anticipation HELD (no ADR-0044 refinement flip this row)

Unlike phase 63 (whose probes FLIPPED the value-source decision from Option A to Option B) and phase 62 (which flipped last-wins to first-wins), the phase-64 probes CONFIRMED every anticipated semantic: reference-truncates (arm 0), `:path`-only target (arm 0), default 256 (arm 1), explicit-0 = empty (arm 2), query included (arm 3), bytes (all arms). The ONE decision the SPEC makes beyond the BRAINSTORM's lean is the concrete helper signature + the parse-arm shape (§3.1/§3.3) — mechanical, not empirical. This is the expected outcome for a knob whose reference behavior is a single well-understood `substr`; the probes serve as a POSITIVE confirmation (and a guard against the D-MPTL-REFTRUNC "no truncation ⇒ re-scope" failure mode, which did NOT fire).

---

## 2. Non-purposes (deferred; per BRAINSTORM §1.2 + §8)

NO new `custom_tag` source (`metadata` STAYS the SOLE loud `custom_tags` strict-reject departure — untouched, orthogonal, `config.go:206-207`). NO other tracing follow-on (`spawn_upstream_span`/`http_service`/force-trace/`verbose`/OTel `sampler`/`resource_detectors` — each larger, §8). NO OTLP-metrics stats sink. NO new provider, transport, or stat. The four built-in span attributes emitted EMPTY by envoy-go (`upstream_cluster`/`node_id`/`zone`/`peer.address`, the `reference_tracing_upstream_cluster_framework_gap` framework gap) are UNTOUCHED — `max_path_tag_length` acts ONLY on `http.url` (a fully-POPULATED built-in), NOT on those un-plumbed fields; do NOT conflate. The access-log record's `Path` field (`accesslog_emit.go:63`/`:186`) is a SEPARATE concern and is UNTOUCHED — the truncation is scoped to the tracing `http.url` span attribute only. The default-256 differential wire-proof (a > 256-path cross-side fixture) is WEIGHED and DEFERRED (§8) in favor of a unit test.

---

## 3. The change — lift the reject, a scalar cap field + a truncation helper at three URL-build sites (ADR-0285)

### 3.0 Split disposition — a SINGLE FLAT ROW; the ADR-0045 escape-valve UNCONSUMED

Anticipated **~8–11 tasks** (§10), UNDER the ADR-0045 `~15` ceiling. There is no second subsystem to strand: the parse (the `UInt32Value` + default-256) and the application (the `:path` byte-truncation at URL construction) both sit on the SAME landed tracing engine. The escape-valve is documented ARMABLE but UNCONSUMED — D-MPTL-DEFAULT-PROOF lands as a unit test (§8), so the default path does NOT spawn a second fixture leg.

### 3.1 The parse arm — lift the reject in `NewConfig` (`internal/tracing/config.go`)

Replace the reject at `config.go:112-114` with a resolve-to-scalar arm (the `verbose` reject at `:109-111`, the `parseCustomTags` call at `:115`, the `spawn_upstream_span` reject at `:119-121`, and the provider dispatch are UNCHANGED):

```go
// replaces config.go:112-114
maxPathTagLen := uint32(256) // the reference default when ABSENT (D-MPTL-DEFAULT)
if m := t.GetMaxPathTagLength(); m != nil {
	maxPathTagLen = m.GetValue() // explicit value; an explicit 0 is PRESERVED (D-MPTL-ZERO)
}
```

and set it on the parsed config AFTER the provider dispatch, mirroring the `cfg.CustomTags = customTags` assignment at `config.go:154` (so BOTH the OTLP and Zipkin arms carry it — the truncation is provider-neutral, §3.3):

```go
cfg.CustomTags = customTags
cfg.MaxPathTagLength = maxPathTagLen // NEW — set on whichever provider arm parseOTel/parseZipkin returned
```

This keeps `parseOTel`/`parseZipkin` signatures UNCHANGED (unlike the sampling knobs which ARE threaded as params — the cap is set post-dispatch on the returned `cfg`, the simpler shape since it needs no per-provider handling). An ABSENT field yields 256; an explicit 0 yields 0 (empty path); a positive value yields that cap.

### 3.2 The config model — `TracingConfig` gains a `MaxPathTagLength uint32` field

Add ONE scalar field to `TracingConfig` (`config.go:25-40`), documented as the resolved cap:

```go
type TracingConfig struct {
	ClientSampling  float64
	RandomSampling  float64
	OverallSampling float64
	// MaxPathTagLength is the resolved byte-cap on the http.url span attribute's
	// :path (path+query) portion: the reference default 256 when ABSENT, the
	// explicit value otherwise (an explicit 0 = empty path is PRESERVED). Always
	// set by NewConfig, so a configured-tracing Filter never sees the zero value
	// as a spurious 0-cap (D-MPTL-DEFAULT / D-MPTL-ZERO).
	MaxPathTagLength uint32 // NEW
	ServiceName      string
	ClusterName      string
	Provider         ProviderKind
	Zipkin           *ZipkinSettings
	CustomTags       []CustomTagSpec
}
```

`MaxPathTagLength` is a NEW symbol — GREP-confirmed collision-free in `internal/tracing` this session (the ONLY `MaxPathTagLength` occurrences are the PROTO getter `t.GetMaxPathTagLength()` at `config.go:112` and the proto field `tr.MaxPathTagLength` in the `config_test.go:340` reject test — a different struct; `reference_spec_drafted_identifier_collision_check`). The `BuildHTTPURL` helper name (§3.3) is likewise collision-free (no `BuildHTTPURL`/`buildHTTPURL` in the package). The exact field ordering is an IMPL detail; placing it near the sampling scalars is illustrative.

### 3.3 The truncation helper — a shared `tracing.BuildHTTPURL` (D-MPTL-TRUNC-LOCATION, Option A)

Add one small exported helper to `internal/tracing` (a new function in `span.go`, or a new `url.go` — IMPL choice):

```go
// BuildHTTPURL assembles the http.url span-attribute value scheme://host+pathAndQuery,
// byte-truncating pathAndQuery (the :path pseudo-header = path+query) to maxPathTagLen
// bytes FIRST — the reference max_path_tag_length semantics (D-MPTL-TARGET/-QUERY/
// -TRUNCUNIT, SPEC-64 §11). The scheme://host prefix is NEVER truncated. A cap of 0
// yields an empty path (scheme://host only, D-MPTL-ZERO).
func BuildHTTPURL(scheme, host, pathAndQuery string, maxPathTagLen uint32) string {
	if len(pathAndQuery) > int(maxPathTagLen) { // int() cast: cap ≤ math.MaxUint32, no overflow on 64-bit
		pathAndQuery = pathAndQuery[:maxPathTagLen]
	}
	return scheme + "://" + host + pathAndQuery
}
```

Compare via `len(pathAndQuery) > int(maxPathTagLen)` (NOT `uint32(len(...)) > maxPathTagLen`, which could wrap for a > 4 GiB path — not realistic, but the `int` cast is the safe form). Byte-slicing `pathAndQuery[:maxPathTagLen]` matches Envoy's byte-indexed `substr` (D-MPTL-TRUNCUNIT). The helper lives in ONE unit-testable place, is DRY across H1/H2/H3, and needs NO `SpanInputs`/`BuildServerSpan` change.

### 3.4 The call-site threading — the three `accesslog_emit.go` URL-build sites (RE-DERIVED)

Each site builds the URL inline; replace the concatenation with the helper, passing `f.tracingConfig.MaxPathTagLength` (the sites already dereference `f.tracingConfig.CustomTags` in the same block, so `f.tracingConfig` is a non-nil invariant when the span block runs — guarded by `f.exporter != nil`; and `MaxPathTagLength` is always set by `NewConfig`, §3.1, so never a spurious zero-value 0-cap):

- **H1 — `accesslog_emit.go:40`:** `url := scheme + "://" + r.Host + r.URL.RequestURI()`
  → `url := tracing.BuildHTTPURL(scheme, r.Host, r.URL.RequestURI(), f.tracingConfig.MaxPathTagLength)`
- **H2 — `accesslog_emit.go:93`:** `url := scheme + "://" + req.Authority + req.Path`
  → `url := tracing.BuildHTTPURL(scheme, req.Authority, req.Path, f.tracingConfig.MaxPathTagLength)`
- **H3 — `accesslog_emit.go:162`:** `url := scheme + "://" + r.Host + r.URL.RequestURI()`
  → `url := tracing.BuildHTTPURL(scheme, r.Host, r.URL.RequestURI(), f.tracingConfig.MaxPathTagLength)`

`r.URL.RequestURI()` (H1/H3) and `req.Path` (H2) are each the `:path` pseudo-header (path+query) — exactly the unit the reference truncates (§11 arm 3). This is a small, mechanical, DRY change — NOT a signature or seam change, NOT a zero-touch row (§1, honestly framed).

### 3.5 The span-emit seam is UNCHANGED — one path, two providers

`http.url` is emitted by `BuildServerSpan` from `SpanInputs.URL` (`span.go:70`), which BOTH exporters consume (OTLP `toProto`; Zipkin reads `Attrs`). The truncation is applied to the STRING handed in via `SpanInputs.URL` — so `BuildServerSpan`, `Span.Attrs`, `upsertAttr`, `ResolveCustomTags`, and both exporters are UNTOUCHED; NO exporter-specific code. A custom `http.url` tag (a `literal`/`request_header`/`environment` custom_tag whose key is `http.url`) STILL upsert-OVERRIDES the truncated built-in via `upsertAttr` (`reference_tracing_custom_tag_override_builtin`) — the truncation acts on the built-in value, and a colliding custom tag replaces it wholesale, UNCHANGED (an edge the SPEC notes but needs no new arm for).

### 3.6 Byte-stability of the DEFAULT-256 behavior change

Honoring the default cap of 256 (D-MPTL-DEFAULT) is a behavior change to the no-config tracing path — but a BYTE-STABLE one for the existing differential corpus: every current OTLP/Zipkin tracing fixture (`0087`/`0088`/`0102`/`0105`/`0106`) drives short request paths (well under 256 bytes), so truncation at 256 is a no-op and their captured `http.url` is unchanged. A tracing HCM with no `max_path_tag_length` parses to `MaxPathTagLength == 256`; a request with a < 256-byte path is byte-identical. The full 108-dir differential is anticipated byte-stable except the new `0107`. The IMPL re-confirms via the six-gate + N-dir gate. If ANY existing tracing fixture were found to carry a > 256-byte path (none anticipated), that would be a PRE-EXISTING latent divergence surfaced — recorded, not masked.

---

## 4. Framework primitives — 0 new packages, 0 new go.mod modules

All edits land in `internal/tracing` (`config.go` + the `BuildHTTPURL` helper, both existing package) + `internal/filter/hcm` (the three `accesslog_emit.go` URL-build sites + the fuzz seed) + `test/fixtures` + `docs/`. `t.GetMaxPathTagLength()` returns a `*wrapperspb.UInt32Value`, already reachable via the resolved `github.com/envoyproxy/go-control-plane/envoy v1.32.4` HCM proto (already imported as `hcmv3`). NO new package, NO new module, NO new interface. `go mod tidy -diff` anticipated EMPTY (modules STAY **2**).

---

## 5. Proto-field roster — `HttpConnectionManager.Tracing.max_path_tag_length` (RE-DERIVED @ go-control-plane/envoy v1.32.4)

| Field | Getter | Phase-64 disposition |
|---|---|---|
| `Tracing.max_path_tag_length` (7, `google.protobuf.UInt32Value`) | `GetMaxPathTagLength()` `config.go:112` | **ACCEPT (NEW)** ⇒ resolve to `uint32` (default 256; explicit 0 preserved) |
| `.GetValue()` on the wrapper | — | the explicit cap when present |
| `Tracing.verbose` (1, bool) | `GetVerbose()` | reject (unchanged, `config.go:109-110`) |
| `Tracing.custom_tags` (8) | `GetCustomTags()` | `literal`/`request_header`/`environment` accept; `metadata` reject (unchanged) |
| `Tracing.spawn_upstream_span` (9) | `GetSpawnUpstreamSpan()` | reject (unchanged, `config.go:119-120`) |

**Proto doc (authoritative default, `http_connection_manager.pb.go`).** `max_path_tag_length`: *"Maximum length of the request path to extract and include in the HttpUrl tag. Used to truncate lengthy request paths to meet the needs of a tracing backend. Default: 256"* — CONFIRMED LIVE by §11 arm 1 (absent → 256) and arm 2 (explicit 0 → empty). PGV: NO numeric rule on the field (only the no-op embedded-`UInt32Value` wrapper validation; §6).

---

## 6. PARSE-REJECT roster + fuzzer

**No new reject (D-MPTL-REJECT).** `max_path_tag_length` is a `UInt32Value` with NO PGV `min`/`max`/`const` constraint (`http_connection_manager.pb.validate.go` does only the no-op embedded-wrapper switch — RE-DERIVED this session, §11 note). An ABSENT field is valid (→ 256), an explicit 0 is valid (→ empty path); there is no structural-invalid case beyond what a well-formed `UInt32Value` already guarantees. The reject at `config.go:112-114` is REMOVED (the field now resolves). The sibling tracing rejects STAY loud with their DISTINCT ADR-0080 substrings (`reference_strict_reject_sibling_typeurl_gap` — lifting one reject is an explicit per-arm change, not a fall-through): `verbose` (`:110`), `spawn_upstream_span` (`:120`), `custom_tags metadata` (`:207`), and the `parseOTel`/`parseZipkin` rejects (`http_service` `:229` / `resource_detectors` `:232` / `sampler` `:235` / `google_grpc` `:240`; non-`HTTP_JSON` Zipkin `:268`; `split_spans_for_request` `:271`; empty clusters).

**Fuzzer (D-MPTL-FUZZSEED).** The tracing parse is reached by `FuzzHCMConfigParse` (`internal/filter/hcm/fuzz_test.go`) — the phase-59/62/63 host. Add ONE seed: a `Tracing` block with `max_path_tag_length` present (incl. an explicit 0), via the existing `mkHCM`/config helper. **This is a SEED, not a new `func Fuzz` — fuzzer count STAYS 55** (`reference_fuzzer_count_docs_drift`: reconcile actual `^func Fuzz` = 55 before AND after — the count must NOT move).

---

## 7. Stat surface — +0 (1201 → 1201)

Truncating a span attribute is a wire-value change, not a stat registration. The HCM tracing decision counters + the tracer counters are UNCHANGED. Stat surface **1201 (+0)**.

---

## 8. Differential fixture taxonomy — +1 (D-MPTL-FIXTURE: ONE new OTLP fixture, VALUE-equality)

Per the dispatch constraint (`reference_differential_fixture_dispatch_constraint` — one dir = one runner branch) the `0087`/`0088` baselines and the `0102`/`0105`/`0106` custom-tag fixtures are NOT mutated. **ONE new dir `test/fixtures/0107-tracing-max-path-tag-length`** (fixtures **108 → 109**; RE-DERIVE the next-free number at the IMPL — `0106` is the current tail, `0107` anticipated):

- Cloned from `0106-tracing-custom-tags-environment` (OTLP provider, `test/helpers/otlptrace` receiver, `host.docker.internal` STRICT_DNS per ADR-0010; drop the `custom_tags` block).
- The HCM `tracing` block: `max_path_tag_length: { value: N }` for a small explicit N (e.g. 16), a route that responds (backend or direct_response — clone `0106`'s backend route), and a driver that drives ONE GET with a request path **longer than N bytes** (ASCII, so byte==rune — D-MPTL-TRUNCUNIT).
- **VALUE-equality assertion.** The driver asserts (by KEY — attribute order is non-deterministic, §11) that the captured span's `http.url` attribute equals the SAME truncated value `http://<authority>/<first-N-bytes-of-path>` on BOTH the reference AND subject side. Unlike the phase-63 env-value case (which needed key-presence because container `PATH` ≠ subject `PATH`), the truncated path is DETERMINISTIC from the request, so cross-side VALUE-equality is achievable with NO harness env-injection. Assert each independent property with `Errorf`, NOT `Fatalf` (`reference_fatalf_makes_assertions_unreachable`). `BackendCount` ≥ 1 (`reference_differential_backendcount_min_one`).
- Prove the new assertion LIVE with a deliberate `-count=1` break (`reference_differential_break_protocol_count1`) using `-run 'TestDifferential/0107-tracing-max-path-tag-length'` (NEVER bare `-run '0107'`, which matches zero subtests → vacuous green — `reference_differential_run_selector`), confirming WHICH assertion fires (`reference_deliberate_break_wrong_assertion`).

**D-MPTL-DEFAULT-PROOF (default-256) — a UNIT test, not a second fixture.** Proving the default-256 truncation cross-side would need a > 256-byte-path fixture; instead the default is proven by a UNIT test on `BuildHTTPURL` (a > 256-byte path with the default 256 cap truncates to 256; deterministic) PLUS a `NewConfig` config test asserting an ABSENT `max_path_tag_length` resolves to `MaxPathTagLength == 256`. The explicit-cap `0107` fixture supplies the cross-side WIRE-parity proof; the default is a config+helper unit concern. The ADR-0045 escape-valve stays UNCONSUMED (§3.0).

**D-MPTL-ZERO / D-MPTL-QUERY — UNIT tests on the helper** (deterministic): explicit-0 → `scheme://host` (empty path); a path+query cut landing inside the query → the truncated prefix; a path under the cap → unchanged; the byte boundary (ASCII). Assert each with `Errorf`.

**The Zipkin encoder path** (a resolved truncated `http.url` surfacing in the Zipkin `tags` map) is a UNIT test (the phase-59/62/63 precedent — one OTLP fixture + a Zipkin unit test), NOT a second fixture — the truncation is provider-neutral (§3.3/§3.5), so the OTLP fixture proves the wire path and the Zipkin unit test proves the encoder carries the (already-truncated) URL. Fixtures **108 → 109**.

**Harness note:** the OTLP fixture drives H1/H2 over TCP — NOT the H3/QUIC path — so `reference_differential_http_expectations_tcp_only` does not bite; the `http.url` assertion lives in the `otlptrace` receiver, not `HTTPExpectations`.

---

## 9. Behavior-contract delta (`docs/envoy-go/BEHAVIOR_CONTRACT.md`; ADR-0285 atomic landing at the IMPL)

- The tracing clause (RE-DERIVE the exact lines at the IMPL) — flip `max_path_tag_length` from "STRICT-REJECT (envoy-go-strict)" to "CONSUMED (byte-truncates the `http.url` span attribute's `:path` (path+query) portion to N bytes; default 256 when absent; an explicit 0 = empty path (`scheme://host`); the `scheme://host` prefix is never truncated; applied on both the OTLP and Zipkin exporters)". Note the default-256 behavior change (§3.6). The sibling tracing knob rejects (`verbose`/`spawn_upstream_span`/`custom_tags metadata`/`http_service`/`resource_detectors`/`sampler`/etc.) STAY.

(Exact final wording RE-DERIVED and written at the IMPL.)

---

## 10. Test plan + per-task structure (~8–11 tasks; PLAN decomposes)

TDD (`superpowers:test-driven-development`); each task a red→green with a `-count=1` liveness break where an assertion is load-bearing. Anticipated tasks:

1. **Config model + parse arm** — add the `MaxPathTagLength uint32` field (§3.2); replace the `config.go:112-114` reject with the resolve arm + the post-dispatch `cfg.MaxPathTagLength = maxPathTagLen` (§3.1). `config_test.go`: MOVE the `max_path_tag_length` row (`:337-343`) OUT of the reject table into an ACCEPT asserting `cfg.MaxPathTagLength == 128`; ADD an ABSENT→256 default test + an explicit-0 test (D-MPTL-ZERO). `-count=1` break per new assertion.
2. **`BuildHTTPURL` helper + unit test** — the helper (§3.3) in `span.go`/`url.go`; `span_test.go`/`url_test.go`: path under cap (unchanged), path over cap (truncated to N bytes), explicit 0 (empty path → `scheme://host`), query-inclusion cut (D-MPTL-QUERY), a > 256 path with cap 256 (D-MPTL-DEFAULT-PROOF), byte boundary (ASCII). Each property `Errorf`.
3. **Call-site rewiring** — the three `accesslog_emit.go` URL-build sites (§3.4) call `BuildHTTPURL`; an HCM span test (or the existing tracing tests) asserts a truncated `http.url` reaches the span through the three sites.
4. **Zipkin encoder unit test** — a span with a truncated `http.url` encodes into the Zipkin `tags` map (mirror the phase-59/62/63 arm).
5. **New OTLP fixture `0107-tracing-max-path-tag-length`** — envoy.yaml, envoy-go.yaml, expectations.yaml, driver (long-path GET, cross-side `http.url` VALUE-equality on the truncated form), README (§8); the assertion proven live with a `-count=1` break.
6. **`FuzzHCMConfigParse` seed** — one `max_path_tag_length` seed (incl. explicit 0, §6); reconcile fuzzer count = 55.
7. **BEHAVIOR_CONTRACT edits** (§9).
8. **Verify** — six-gate (gofmt / golangci-lint / go vet / build / `go mod tidy -diff` / full package `-race` on `internal/tracing` + `internal/filter/hcm`) + the full 109-dir differential (byte-stable except `0107`).
9. **ADR-0285 body** (§Decision/§Consequences) + **STATE** + **ROADMAP** (row 64 `done` per ADR-0106; the LIVE Observability deferred sentence is UNCHANGED — `max_path_tag_length` was never IN it — so re-run the check-(2) grep only to CONFIRM EXACTLY ONE live match with unchanged content, `reference_sentinel_deferred_sentence_live_vs_historical`) + **router roll**.

(Tasks 1–3 are the TDD core; the PLAN may split/merge. Total ~8–11, single flat row.)

---

## 11. SPEC-time empirical-pin block (D-MPTL-* live probes — executed IN-SESSION 2026-07-14, `envoyproxy/envoy:contrib-v1.37.2`, FRESH container per arm)

Each arm ran a fresh `envoyproxy/envoy:contrib-v1.37.2` container (`--add-host host.docker.internal:host-gateway`; a host-bound `test/helpers/otlptrace` receiver on `0.0.0.0:<port>`; listener published to a host-allocated port), configured `max_path_tag_length` (or omitted it), drove ONE `GET <path>` with a SHORT authority (`Host: h.io`, 4 bytes), and captured the OTLP span's `http.url` attribute. Decode VERIFIED non-vacuous (span present, `http.url` non-empty) on every arm. The route was a `direct_response: {status: 200}` (no backend needed — the ingress SERVER span is created at decode, before routing). A warmup GET preceded each probe; its span is filtered by matching the probe path.

**Arm 0 (D-MPTL-REFTRUNC / D-MPTL-TARGET — cap=16, 27-byte path, 4-byte host).** Config: `max_path_tag_length: {value: 16}`; drove `GET /abcdefghijKLMNOPqrstuvwxyz` (path = 27 bytes). Captured:
```
http.url = "http://h.io/abcdefghijKLMNO"   (len=27 = 11 "http://h.io" + 16 :path)
```
⇒ **D-MPTL-REFTRUNC**: the reference OTLP tracer TRUNCATES `http.url` at `max_path_tag_length` (provability CONFIRMED — no re-scope). **D-MPTL-TARGET**: only the `:path` portion is truncated (to exactly 16 bytes); the `scheme://host` prefix (`http://h.io`, 11 bytes) SURVIVES intact and is NOT counted toward the cap.

**Arm 1 (D-MPTL-DEFAULT — ABSENT, 307-byte path).** Config: NO `max_path_tag_length`; drove `GET /probe/<300×'a'>` (path = 307 bytes). Captured:
```
http.url len = 267 = 11 "http://h.io" + 256 :path   ("http://h.io/probe/" + 249×'a')
```
⇒ **D-MPTL-DEFAULT**: an ABSENT `max_path_tag_length` caps the `:path` at **256** (NOT unlimited). The load-bearing behavior-change decision — envoy-go emits UNTRUNCATED today, so honoring 256 CLOSES a latent > 256 divergence (§3.6).

**Arm 2 (D-MPTL-ZERO — explicit 0).** Config: `max_path_tag_length: {value: 0}`; drove `GET /somepath?x=1`. Captured:
```
http.url = "http://h.io"   (len=11 — EMPTY :path)
```
⇒ **D-MPTL-ZERO**: an EXPLICIT `max_path_tag_length: 0` truncates the `:path` to EMPTY → `scheme://host` only. NOT "unlimited"; the explicit 0 is a valid value the parse must PRESERVE (mirrors the phase-46 explicit-0 sampling precedent — `PROTOBUF_GET_WRAPPED_OR_DEFAULT` returns the explicit 0, not the 256 default).

**Arm 3 (D-MPTL-QUERY — cap=16, path+query, cut inside the query).** Config: `max_path_tag_length: {value: 16}`; drove `GET /p?query=abcdefghijklmnop` (path+query = 25 bytes). Captured:
```
http.url = "http://h.io/p?query=abcdefg"   (:path = "/p?query=abcdefg", 16 bytes)
```
⇒ **D-MPTL-QUERY**: the `:path` unit INCLUDES the query string; the truncation cut landed INSIDE the query (`abcdefghijklmnop` → `abcdefg`). ⇒ envoy-go's `r.URL.RequestURI()` (H1/H3) and `req.Path` (H2) — each path+query — are the correct truncation unit.

**D-MPTL-TRUNCUNIT (bytes).** Every observed `:path` length is byte-exact (16 / 256 / 0). Envoy's `absl::string_view::substr` is byte-indexed; Go `s[:n]` slices bytes. The `0107` fixture uses an ASCII path (byte==rune), so no rune-edge is exercised; the helper unit test asserts byte truncation.

**PGV structural rule (D-MPTL-REJECT — RE-DERIVED @ `.../http_connection_manager/v3/http_connection_manager.pb.validate.go`, go-control-plane/envoy v1.32.4).** The `MaxPathTagLength` validation is ONLY the no-op embedded-`UInt32Value` wrapper switch (`ValidateAll`/`Validate` on the wrapper — which has NO rules); there is NO `min`/`max`/`const`/`gt`/`lt` on the field. ⇒ absent + explicit-0 + any positive value are ALL structurally valid; envoy-go adds NO new reject (§6).

**Helper-name collision check (`reference_spec_drafted_identifier_collision_check`).** `grep -rn 'BuildHTTPURL\|buildHTTPURL\|MaxPathTagLength' internal/tracing/` this session: no `BuildHTTPURL`/`buildHTTPURL` anywhere; `MaxPathTagLength` only as the proto getter (`config.go:112`) + the reject-test proto field (`config_test.go:340`). Both new symbols are collision-free.

*(Probe harness: a throwaway `probe64/` Go program reusing the `otlptrace` receiver + a `docker run` CLI loop, one fresh `--rm` container per arm; convergence polled BEFORE container stop (OTLP flush is async); DELETED after — NOT committed, this SPEC is docs-only.)*

---

## 12. Edit-site roster (D-MPTL-DOCSHAPE — RE-DERIVED against master `7d724423`)

**Production — `internal/tracing/config.go`:**
- `config.go:25-40` `TracingConfig` — ADD the `MaxPathTagLength uint32` field (§3.2). [EDIT]
- `config.go:112-114` `NewConfig` — replace the `max_path_tag_length` reject with the resolve arm (default 256 / explicit incl. 0); set `cfg.MaxPathTagLength` after the provider dispatch, alongside `cfg.CustomTags` at `:154` (§3.1). [EDIT]

**Production — `internal/tracing` (helper):**
- `span.go` (or a new `url.go`) — ADD `func BuildHTTPURL(scheme, host, pathAndQuery string, maxPathTagLen uint32) string` (§3.3). [ADD]

**Production — `internal/tracing/span.go`:**
- `BuildServerSpan` + `upsertAttr` — UNCHANGED (§3.5). [NO CHANGE — confirm]

**Production — `internal/filter/hcm/accesslog_emit.go`:**
- `:40` (H1) + `:93` (H2) + `:162` (H3) — replace the inline `url := scheme + "://" + host + pathAndQuery` with `tracing.BuildHTTPURL(scheme, host, pathAndQuery, f.tracingConfig.MaxPathTagLength)` (§3.4). [EDIT ×3]

**Test:**
- `internal/tracing/config_test.go` — MOVE the `max_path_tag_length` row (`:337-343`) from the reject table to an ACCEPT (`cfg.MaxPathTagLength == 128`); ADD ABSENT→256 + explicit-0 tests (§10 task 1). [EDIT/ADD]
- `internal/tracing/span_test.go` / a new `url_test.go` — the `BuildHTTPURL` matrix (under-cap / over-cap / explicit-0 / query-cut / > 256-default / byte-boundary; §10 task 2). [ADD]
- `internal/tracing/zipkin_test.go` — a truncated `http.url` surfaces in the Zipkin tags map (§10 task 4). [ADD/EDIT]
- `internal/filter/hcm/*_test.go` — a truncated `http.url` reaches the span through the three call sites (§10 task 3). [ADD/confirm]
- `internal/filter/hcm/fuzz_test.go` — a `max_path_tag_length` seed; no new fuzzer (§6). [ADD]

**Fixture:**
- `test/fixtures/0107-tracing-max-path-tag-length/` (new) — OTLP provider + `max_path_tag_length: {value: N}`; the driver drives a long-path GET; cross-side `http.url` VALUE-equality on the truncated form (§8). [ADD]

**Docs:**
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` (tracing clause, §9). [EDIT — IMPL]
- `docs/envoy-go/ROADMAP.md` row 64 → `done` + family prose (deferred sentence UNCHANGED). [EDIT — IMPL]
- `docs/envoy-go/STATE.md` (active-phase header). [EDIT — each stage]
- `docs/envoy-go/DECISIONS.md` — ADR-0285 §Context here (§13); §Decision/§Consequences at the IMPL. [ADD]

---

## 13. ADR continuity — the ADR-0285 §Context DRAFT (anchored here; full entry at the phase-64 IMPL)

**ADR-0285 §Context (draft).** Phase 46 established the HCM-native tracing engine (`TracingConfig`, `NewConfig`, `BuildServerSpan`/`Span.Attrs []KV`, the OTLP + Zipkin exporters), emitting the `http.url` built-in span attribute UNTRUNCATED and STRICT-REJECTING `max_path_tag_length` (`config.go:112-114`, envoy-go-strict ADR-0080). Phases 59/62/63 lifted the three `custom_tags` source rejects (`literal`/`request_header`/`environment`) on the `[]CustomTagSpec`/`ResolveCustomTags` seam. `max_path_tag_length` was carried as a deferred candidate (phase-63 BRAINSTORM §8). Phase 64 lifts that reject — the FIRST tracing NUMERIC-knob row — and HONORS the knob: byte-truncate the `:path` portion (path+query) of the `http.url` tag to N bytes. SPEC-64 live probes against `envoyproxy/envoy:contrib-v1.37.2` (§11, fresh container per arm) PINNED every anticipated semantic: the reference truncates `http.url` at `max_path_tag_length` (arm 0); ONLY the `:path` is truncated, the `scheme://host` prefix survives (arm 0); an ABSENT field caps at 256 (arm 1 — so envoy-go's untruncated emission ALREADY diverges for > 256 paths, a latent gap this row closes); an explicit 0 truncates the path to EMPTY (arm 2 — preserved, not "unlimited"); the query string is included in the truncated `:path` and a cut can land inside it (arm 3); truncation is byte-indexed (all arms). The reference applies the truncation in the generic `Tracing::HttpTracerUtility` (provider-neutral), so it applies identically to Zipkin. The truncation is a `google.protobuf.UInt32Value` with NO PGV numeric constraint (§11) — absent + explicit-0 both valid, so NO new structural reject. The design: add a `TracingConfig.MaxPathTagLength uint32` field (default 256 / explicit incl. 0, resolved in `NewConfig`, set post-dispatch alongside `CustomTags` so both provider arms carry it); add a shared `tracing.BuildHTTPURL(scheme, host, pathAndQuery, maxPathTagLen)` helper (D-MPTL-TRUNC-LOCATION Option A) that byte-truncates `pathAndQuery` FIRST then prepends `scheme://host`; call it at the three `accesslog_emit.go` URL-construction sites (H1 `:40` / H2 `:93` / H3 `:162`) replacing the inline concatenation. `BuildServerSpan`/`Span.Attrs`/`upsertAttr`/`ResolveCustomTags`/both exporters/the custom-tag machinery are UNCHANGED; a custom `http.url` tag STILL upsert-overrides the truncated built-in. The default-256 behavior change is byte-stable for the short-path corpus. A SINGLE FLAT ROW (ADR-0045 escape-valve unconsumed); the truncation helper is FOLDED into ADR-0285 (no separate seam ADR — the phase-59/62/63 precedent). +0 stats / +1 fixture (`0107`, explicit-cap cross-side `http.url` VALUE-equality; the default-256 wire-proof deferred to a unit test) / +0 fuzzers (a seed) / +0 packages / +0 modules. §Decision/§Consequences land at the phase-64 IMPL per ADR-0044. ANCHORS ADR-0285.

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

**Counts UNCHANGED at this SPEC (docs-only; re-verified against master tip `7d724423`):** stat surface **1201** · fixtures **108** · fuzzers **55** · BackendKind **38** · DECISIONS tail **ADR-0284** (next-free **ADR-0285**) · new Go packages **0** · go.mod modules **2**.

**Anticipated at the phase-64 IMPL:** stat surface **1201 (+0)** · fixtures **108 → 109** (`0107-tracing-max-path-tag-length`) · fuzzers **55 (+0, seed only)** · BackendKind **38 (+0)** · DECISIONS tail **ADR-0285** (next-free **ADR-0286**) · new Go packages **0** · new go.mod modules **0** · row 64 → `done`.

**ROADMAP/STATE at SPEC-DONE:** row 64 STAYS `in-progress` (a row flips `done` only at its IMPL six-gate, ADR-0106). The LIVE Observability deferred sentence is UNCHANGED (`max_path_tag_length` was never IN it — it is a §8-tier candidate — so this row does not narrow it; sentinel check-(2) STILL one live Observability match with unchanged content). STATE active-phase header flips to `phase 64 SPEC done` (NEXT = the phase-64 PLAN).

**Next → the phase-64 PLAN** (the TDD decomposition of §10 over this SPEC; every `file:line` RE-DERIVED against the master tip; ADR-0045 single-flat-row; PROGRESS scaffolded).
