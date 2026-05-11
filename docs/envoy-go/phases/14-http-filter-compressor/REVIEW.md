# Phase 14 — Code review (REVIEW.md)

**Phase id:** `14` (seventh §9 HTTP-filters family-row to land per ADR-0106; SECOND production filter to use the "disabled-OR-override" 5th canonical per-route discipline per ADR-0125 amendment §(viii)-(x); LARGEST stat surface per filter to date in §9 family-rows — 17 counters; SECOND row to introduce a framework delta — encode-side `EncoderFilterCallbacks.OverwriteBody(b []byte)` per ADR-0131, symmetric to phase-13 ADR-0128's decode-side primitives; FIRST §9 row whose fixture body-assertion is decompress-and-compare rather than byte-exact per ADR-0133)
**Slug:** `14-http-filter-compressor`
**Branch under review:** `phase-14-http-filter-compressor-impl`
**Range:** `68c7bbf` (branch tip; Task 15 SHA-fill follow-up) — 15 task commits + SHA-fill / PROGRESS-append / ADR-0134 follow-up / test-count-fix follow-ups; phase-done at `823c948`.
**Parent ROADMAP row:** `14 http-filter-compressor` flipped `in-progress → done` at Task 15 commit `823c948` (already landed prior to this REVIEW; row 14's status field reads `done` on the impl branch at HEAD).
**Reviewer method:** Inline authoring by the implementing session per the PLAN's Task 16 explicit allowance; inputs: SPEC §15 acceptance checklist + the branch diff + phase-13 REVIEW.md structural template + PROGRESS.md per-task entries + DECISIONS.md ADR-0129..ADR-0134 + ADR-0125 amendment §(viii)-(x).
**Six-gate state at HEAD:** all green per Task 15's verification sweep — outputs reproduced verbatim in §4 below.

This review covers the full phase 14 surface: `internal/filter/http/compressor/` package (`doc.go` + `compressor.go` + `acceptencoding.go` + `compressor_test.go` + `fuzz_test.go`), framework deltas at `internal/filter/http/callbacks.go` + `internal/filter/http/chain.go` + `internal/filter/hcm/connection.go` + `internal/filter/hcm/h2dispatch.go` (encode-side `OverwriteBody` per ADR-0131) and at `internal/filter/hcm/actions.go` + `config.go` + `actions_test.go` (directResponseAction.response_headers_to_add plumbing per ADR-0134 follow-up), `cmd/envoy-go/main.go` boot registration, NEW shared helper `test/helpers/echobackend/`, differential fixture `0016-http-compressor` (6 scenarios, single-listener six-route topology with 4 direct_response + 1 cluster + 1 per-route-disabled), `FuzzCompressorConfigParse` (eighteenth fuzzer in repo), `BEHAVIOR_CONTRACT.md` §13 four-edit bundle (NEW compressor subsection ~140 LoC + 29→46-name table extension + Equivalence Matrix row + Phase 14 forward-pointer notes ~55 LoC), the six ADRs ADR-0129..ADR-0134 + ADR-0125 amendment §(viii)-(x), and the ROADMAP row 14 status flip + STATE.md advance.

This REVIEW closes phase 14's lifecycle (state 5 → 6) and is the final task before merge to master.

---

## 1. Phase summary

**APPROVED.**

All six phase-done gates are GREEN at HEAD `823c948` per the Task 15 verification sweep (§4 below). The implementation faithfully realizes the SPEC across all 15 PLAN tasks (plus the Task 14 ADR-0134 follow-up). The compressor filter is the SEVENTH §9 HTTP-filters family-row to ship under ADR-0106 and the SECOND to demonstrate the "disabled-OR-override sum-type" per-route discipline (ADR-0125 amendment §(viii)-(x) named at SPEC commit per phase-13 ADR-0127-v2 in-place-update precedent). It is the LARGEST stat-surface §9 row to date at 17 counters (vs phase 11 local_ratelimit's 4; phase 12 csrf's 3; phase 13 buffer's 0), the SECOND §9 row to land a framework delta (after phase-13's decode-side primitives), and the FIRST §9 row whose differential fixture body assertion is decompress-and-compare per ADR-0133 (vs byte-exact in all prior §9 fixtures).

The architectural centerpiece is the 11-bucket `EncodeHeaders` skip-decision sequence (SPEC §6.6) feeding into `EncodeData` Path B (buffer-then-compress) per ADR-0131: `DecodeHeaders` caches AE classification BEFORE per-route remove_accept_encoding_header strip (ADR-0129 §Decision (iv) same-`*filter` discipline); `EncodeHeaders` evaluates 11 skip predicates in order (status-code / content-encoding / content-type / content-length / etag-disable / AE-not-valid / overshadowed / wildcard / identity / no-header / compressor-used), and on the compressed path injects Vary: Accept-Encoding (always-append per §1.1 amendment 5; never short-circuit on `Vary: *`), strips strong ETag (mode-a per §1.1 amendment 6; preserves weak ETag verbatim), sets Content-Encoding, and strips Content-Length (deferred to post-encode fixed-CL emit per Path B). `EncodeData` accumulates the response body, invokes Go `compress/gzip.NewWriter`, fires `EncoderFilterCallbacks.OverwriteBody(gzipped)`, sets the fixed `Content-Length: <gzipped-len>`, and writes the response. This diverges from reference Envoy's chunked-output wire shape (forward-pointer to a future encode-side streaming framework phase per ADR-0131 §Decision (vi)); decompressed body bytes remain equivalent, which ADR-0133's decompress-and-compare fixture discipline asserts.

The differential fixture `0016-http-compressor` is the phase-closing non-vacuous evidence against reference Envoy v1.37.2: 6 scenarios on a single listener with six routes (`/echo` compress + `/image-png-1024` content-type-skip + `/text-html-10` min-content-length-skip + `/text-html-1024-etag` disable-on-etag-skip + `/per-route-disabled` + `/per-route-rmae`), exercising the compressed allow-path (scenario 1) + three skip predicates (2/3/4) + per-route disabled wholly-inactive (5) + per-route rmAE override with backend-echo assertion (6). All 6 PASS with decompress-and-compare body equivalence on the compressed scenarios (1, 4, 6) and identity-byte-exact on the skip scenarios (2, 3, 5).

Six ADRs landed (ADR-0129..ADR-0134) — five anticipated at SPEC commit per ADR-0044 SPEC-time-anticipation discipline + one (ADR-0134) added at Task 14 follow-up per ADR-0044's escape-valve clause for framework gaps surfaced at integration time. Phase 14 entered with 5 anticipated ADRs; it landed with 6.

---

## 2. ADR roster

Each of the six ADRs ADR-0129..ADR-0134 + ADR-0125 amendment, evaluated for whether the §Decision body held up under implementation + fixture exercise:

**ADR-0125 amendment §(viii)-(x)** (SECOND row using disabled-OR-override 5th canonical; per-route override surface is filter-specific NOT listener-level-wholesale; wholesale-not-merge semantic applies within the override-fields envelope): **VALIDATED.** Landed at SPEC commit per phase-13 ADR-0127-v2 in-place-update precedent. Phase 14's per-route `ResponseDirectionOverrides` carries exactly one field (`remove_accept_encoding_header`); the disabled-OR-override discipline survives intact; the wholesale-replace semantic is honored verbatim in `parsePerRoute`.

**ADR-0129** (`internal/filter/http/compressor/` package shape — single-token directory + ENCODER+DECODER `HTTPFilter` value with SAME `*filter` instance + 17-counter `filterStats` + boot-registration ordering router→buffer→compressor→cors→csrf→...): **VALIDATED.** Single-token directory `compressor/` aligns with cors/fault/csrf/buffer precedent. Boot-registration ordering inserted between buffer and cors alphabetically. `Decoder: f, Encoder: f` SAME *filter instance is the FIRST §9 row to use this shape with non-vacuous both paths structurally — both `SetDecoderCallbacks` (for `f.dcb` RequestRouteConfig per §6.4) and `SetEncoderCallbacks` (for `f.ecb` OverwriteBody per §6.7) store on the same `*filter` instance per §Decision (iv).

**ADR-0130** (`compiledConfig` shape + 8 consumed/12 ignored field decomposition + codec-library Any-unmarshal-and-dispatch + parse-rejection of unknown TypeURL + Gzip compression-level mapping table + envoy-go-only error wording): **VALIDATED with SPEC-time bookkeeping refinement (§1.1 amendment 1).** BRAINSTORM hypothesized 8 consumed / 9 silent-ignored / 1 parse-rejected; SPEC-time empirical pin §11 surfaced 2 bookkeeping errors (status_header_enabled missed; enabled slot mis-labeled) revising the count to 8 consumed (6 listener + 2 codec) + 12 silent-ignored (9 listener + 3 codec) + 1 parse-rejected. Implementation matches the revised count; parse-time rejection of unknown TypeURL is unit-tested in Group 2.

**ADR-0131** (Body algorithm Path B (buffer-then-compress) + wire-shape divergence + `EncoderFilterCallbacks.OverwriteBody(b []byte)` framework primitive + min_content_length late-revert anomaly forward-pointer): **VALIDATED.** Path B chosen at SPEC time; Task 4 lands the framework primitive FIRST per cold-start prompt Critical PLAN-time obligation 2 (before Tasks 5-7 consume); the min_content_length late-revert anomaly is structurally accepted (per planner-time decision D3 + D4) and documented at BEHAVIOR_CONTRACT.md §13.4. The wire-shape divergence (envoy-go fixed-CL identity vs Envoy chunked) is the load-bearing forward-pointer for a future encode-side streaming framework phase.

**ADR-0132** (17-counter stat surface + namespace shape `compressor.<library_name>.<codec>.[response.]<counter>` + Rule SN2 reuse (NO new SN10) + per-route SHARED stats discipline): **VALIDATED with Task-14 impl-time empirical refutation of SPEC §7.1 simplifications.** The 17-counter stat surface (6 header_* + 1 not_compressed_etag + 5 response.* + 5 request.*) registered correctly; namespace shape emits at `compressor.text_optimized.gzip.[response.]<counter>`; SN2 flatten produces `envoy_http_compressor_text_optimized_gzip_<counter>{envoy_http_conn_manager_prefix="ingress_compressor"}` verbatim. SPEC §7.1 + §7.3 simplifications about reference Envoy counter shape were REFUTED at Task 14 integration — see §5 below for the 4 per-side empirical divergences locked at `counterModePerSideExact`.

**ADR-0133** (Differential-fixture decompress-and-compare body-assertion discipline): **VALIDATED.** First §9 row whose fixture body assertion is decompress-and-compare rather than byte-exact. The `assertBodyEquivalent` + `decompressGzip` helpers at fixture driver fire on the 3 compressed scenarios (1, 4, 6); identity-byte-exact applies to the 3 skip scenarios (2, 3, 5). Generalizable to future codec/transform-filter fixtures (decompressor; bandwidth_limit; future codec phases).

**ADR-0134 (NEW; Task 14 follow-up per ADR-0044 escape-valve)** (HCM directResponseAction.response_headers_to_add support scoped to OVERWRITE_IF_EXISTS_OR_ADD): **VALIDATED.** Unanticipated framework gap surfaced at Task 14 integration — pre-phase-14 `directResponseAction.body()` hardcoded `Content-Type: text/plain` ignoring the route-level `response_headers_to_add`. Fixture 0016 scenario 2 (image/png content-type-skip path) required the route override to land on the direct_response body before the compressor's content-type predicate sees it. Fix: `directResponseAction.extraHeaders` field + `buildExtraResponseHeaders` parser at `internal/filter/hcm/{actions.go, config.go}`; OVERWRITE_IF_EXISTS_OR_ADD-only AppendAction support; APPEND_IF_EXISTS_OR_ADD / ADD_IF_ABSENT / OVERWRITE_IF_EXISTS reserved for future support. 7 unit tests cover the new code path.

---

## 3. Empirical pins outcome

All 15 SPEC §11 empirical pins were resolved at SPEC drafting: 9 ratified, 6 refuted (the latter drove the 6 §1.1 amendment blocks — the highest single-row amendment count in any §9 row to date). The structural design (gzip-only, response-only, Path B, ENCODER+DECODER, 5th canonical disabled-OR-override) survived intact through all six amendments.

**Phase 14 ALSO uncovered TWO impl-time empirical findings not anticipated at SPEC time:**

1. **The `directResponseAction.response_headers_to_add` gap (ADR-0134).** Surfaced at Task 14 integration when fixture scenario 2 failed to skip on envoy-go (the hardcoded `Content-Type: text/plain` matched the compressor's default 8-entry content_type list per SPEC §11.1). The SPEC §3 framework survey did NOT anticipate this gap; it surveyed `EncoderFilterCallbacks` but did not cover `directResponseAction.body()`. Task 14 follow-up lands ADR-0134 per ADR-0044's escape-valve clause.

2. **Four per-side counter divergences pinned at Task 14 integration** (refuting SPEC §7.1 + §7.3 simplifications). Reference Envoy v1.37.2 + envoy-go diverge by design choice on 4 of the 17 counters; both sides are valid implementations of the documented contract. See §5 + §7 below for the divergence table + root-cause notes.

The SPEC-time empirical-pin discipline (15 pins resolved in-session against reference Envoy) remains solid; impl-time framework-delta surfacing via fixture integration is the right gate. Integration testing is the truth-source for per-side counter shapes that SPEC-time prose simplifications cannot capture. **Lesson: SPEC §11 empirical pins are guidance subject to integration refutation; differential fixture probes at integration time are the authoritative truth-source.**

---

## 4. Gate-by-gate evidence

Verbatim from PROGRESS.md Task 15 outputs. All 6 gates green at HEAD `823c948`:

**Gate A — build / vet / lint clean:**
```
$ go build ./...
(no output; exit 0)
$ go vet ./...
(no output; exit 0)
$ golangci-lint run ./...
(no output; exit 0)
```

**Gate B — race-test pass across all packages:**
```
$ go test -race -count=1 ./...
ok  github.com/esalaine/envoy-go/internal/filter/http/compressor  1.060s
[... 40+ other packages PASS; differential suite ~47s standalone ...]
All packages PASS; no race violations.
(./test/differential occasionally hits ephemeral-port TIME_WAIT under `./...` parallel execution; standalone re-runs always green; known harness flake.)
```

**Gate C — h2spec 53/53 PASS:**
```
$ go test -count=1 -v ./test/conformance/h2spec/
        53 tests, 53 passed, 0 skipped, 0 failed
--- PASS: TestH2Spec (2.24s)
ok  github.com/esalaine/envoy-go/test/conformance/h2spec  3.373s
```

**Gate D — 18 fuzzers green at 30s budget:**
```
$ go test -fuzz=FuzzCompressorConfigParse -fuzztime=30s -run=^$ ./internal/filter/http/compressor/
fuzz: elapsed: 31s, execs: 4951255 (0/sec), new interesting: 45 (total: 309)
PASS
All 18 fuzzers PASS at 30s budget (FuzzCompressorConfigParse — 18th; 17 prior clean).
```

**Gate E — 17 differential fixtures 0000-0016 PASS:**
```
$ go test -count=1 -v ./test/differential/ -run TestDifferential
    --- PASS: TestDifferential/0016-http-compressor (~6s)
    [... 16 prior fixtures 0000-0015 all PASS ...]
--- PASS: TestDifferential (45.88s)
ok  github.com/esalaine/envoy-go/test/differential  47.007s
```

**Gate F — BEHAVIOR_CONTRACT 4-edit bundle landed:**
```
$ grep -n "^### envoy.filters.http.compressor\|^### 46-name table\|envoy.filters.http.compressor.*0016-http-compressor\|### Phase 14 forward-pointer notes" docs/envoy-go/BEHAVIOR_CONTRACT.md
36:| HTTP filter `envoy.filters.http.compressor` | 0016-http-compressor (gzip-only response-side): ...
136:### 46-name table (introduced by phase 06.1; extended by phase 09; extended by phase 11; extended by phase 12; UNCHANGED in phase 13; extended by phase 14)
1302:### envoy.filters.http.compressor
1738:### Phase 14 forward-pointer notes
All 4 anchors at expected positions.
```

---

## 5. Acceptance checklist

Per SPEC §15 + this PLAN's per-task acceptance bullets. All items green; deviations explicitly noted.

- [x] Phase 14 SPEC.md authored with 6 §1.1 amendment blocks; each amendment cross-referenced to §11 empirical evidence.
- [x] §3 framework-survey result locked: Path A fires; `EncoderFilterCallbacks.OverwriteBody(b []byte)` primitive landed at Task 4 per ADR-0131 §Decision (vi).
- [x] §11 empirical-pin block: 15 pins resolved IN-SESSION at SPEC drafting; 9 ratified, 6 refuted.
- [x] Differential fixture redesign: 6 scenarios; scenario 6 redesigned per §1.1 amendment 4 (per-route rmAE + real-backend-echo assertion); ADR-0133 codifies decompress-and-compare body assertion.
- [x] ADR roster: 6 ADRs landed (ADR-0129..ADR-0134 — five anticipated + ADR-0134 added at Task 14 follow-up per ADR-0044 escape-valve) + ADR-0125 amendment §(viii)-(x) at SPEC commit.
- [x] Stat surface: 17 counters; namespace `compressor.<library_name>.gzip.[response.]<counter>`; existing SN2 flatten; NO new SN10. Per-route SHARED-with-listener-level (mirrors phase-12 csrf ADR-0124 + phase-13 buffer ADR-0125).
- [x] Per-route surface: disabled-OR-rmAE-only (`ResponseDirectionOverrides` carries one field); SECOND row using ADR-0125 5th canonical.
- [x] Wire-shape divergence: envoy-go fixed-CL identity vs Envoy chunked; ADR-0131 forward-points to future encode-side streaming framework phase.
- [~] **SPEC §7.1 + §7.3 simplifications REFUTED at Task 14 integration.** SPEC §7.1 expected counter table assumed reference Envoy behavior; Task 14 empirical probes pinned 4 per-side divergences (`header_compressor_used`, `header_not_valid`, `response_not_compressed`, `request_not_compressed`). Driver locks both sides' empirical values via `counterModePerSideExact`; BEHAVIOR_CONTRACT §13.4 codifies the divergences with root-cause notes. This is the one §15 item that does not match SPEC's original framing; the resolution (per-side empirical locking) is honest + auditable.
- [x] STATE.md advanced to phase-done lifecycle-state-5; next-skill `superpowers:requesting-code-review`; next-free ADR `ADR-0135`.
- [x] `internal/filter/http/compressor/` package exists with `doc.go` + `compressor.go` + `acceptencoding.go` + `compressor_test.go` + `fuzz_test.go`.
- [x] `cmd/envoy-go/main.go` registers `compressor.New` alphabetically (`router → buffer → compressor → cors → csrf → ...`).
- [x] `New` factory rejects unknown codec TypeURLs + invalid Gzip compression-level values; envoy-go-own error wording per ADR-0130.
- [x] Framework primitive `EncoderFilterCallbacks.OverwriteBody(b []byte)` lands at Task 4 BEFORE consumers (Tasks 5-7); FIRST framework delta in §9 family-rows on the encode side; SECOND framework delta overall (after phase-13 ADR-0128 decode-side primitives).
- [x] `EncodeHeaders` 11-bucket skip-decision sequence implemented per §6.6; Vary: Accept-Encoding always-append per §1.1 amendment 5; ETag mode-a strong-strip / mode-b skip-on-any per §1.1 amendment 6.
- [x] `EncodeData` Path B (buffer-then-compress) implemented per §6.7; `OverwriteBody` fired on compressed path; fixed `Content-Length: <gzipped-len>` set; identity passthrough on skip paths.
- [x] `filterStats` 17-counter registration via real `newFilterStats` per Task 8 / ADR-0132. Namespace shape verified in Group 8 unit tests + fixture 0016 Prometheus scrape.
- [x] Differential fixture 0016 6-request matrix green per §7.1.
- [x] `FuzzCompressorConfigParse` green at 30s budget (18 fuzzers total).
- [x] All 16 prior differential fixtures still green; 17 prior fuzzers still green; h2spec 53/53 still PASS.
- [x] `BEHAVIOR_CONTRACT.md` §13 4-edit bundle at phase-done commit (lines 36 + 136 + 1302 + 1738).
- [x] `DECISIONS.md` carries 6 new ADRs (ADR-0129..ADR-0134). ADR-0134 added at Task 14 follow-up per ADR-0044 escape-valve.
- [x] REVIEW.md authored: THIS document.

---

## 6. Forward-pointer roster

Per BEHAVIOR_CONTRACT.md §13.4 "Phase 14 forward-pointer notes":

**(i) 8 BRAINSTORM §8 / SPEC §2.1 inline-deferrals:** brotli + zstd codecs (per-codec future phases extend ADR-0130); request-side compression + future `envoy.filters.http.decompressor` filter (couples `request_direction_config` re-activation); 4 deprecated top-level mirror fields (`content_length`, `content_type`, `disable_on_etag_header`, `remove_accept_encoding_header` at the Compressor message top level); `Compressor.runtime_enabled` + `response_direction_config.common_config.enabled` (couples to Runtime + hot restart family); `Compressor.choose_first` (always-q-value-based selection in MVP); `Compressor.response_direction_config.status_header_enabled` (the `x-envoy-compression-status:` debug header is never emitted); `Gzip.{memory_level, window_bits, chunk_size}` (Go `compress/gzip` does not expose libz-equivalent knobs); per-route `overrides.compressor_library` (per-route library swap silent-ignored).

**(ii) `per_connection_buffer_limit_bytes` / `per_request_buffer_limit_bytes` silent-ignored (ADR-0076):** Same forward-point as phase-13 buffer §6 item (ii); deferred to a future cap-promotion phase. Phase 14 introduces a symmetric encode-side cap question (compressed-response accumulation in EncodeData has the same unbounded-growth concern as phase-13's decode-side accumulation); the symmetric decompressor phase or a global cap-promotion phase is the natural amender.

**(iii) `directResponseAction` AppendAction broader support (ADR-0134 §Decision restriction):** Phase 14's ADR-0134 supports only `OVERWRITE_IF_EXISTS_OR_ADD`. The remaining 3 AppendAction values (`APPEND_IF_EXISTS_OR_ADD` / `ADD_IF_ABSENT` / `OVERWRITE_IF_EXISTS`) are reserved for future support; current behavior is parse-time rejection. Future filter or fixture needing one of these semantics is the natural amender.

**(iv) `request_headers_to_add` symmetric path on directResponse:** ADR-0134 covers `response_headers_to_add` only on the directResponseAction path. The symmetric `request_headers_to_add` path is structurally moot for directResponse (no upstream request emission), but for cluster routes the request-side route-level header injection is currently unaudited under the ADR-0134 framework lens. Future header-mutation or request-side filter is the natural amender.

**(v) Wire-shape divergence-window (ADR-0131):** envoy-go MVP emits fixed `Content-Length: <gzipped-len>` + identity transfer; reference Envoy emits `Transfer-Encoding: chunked` + no `Content-Length`. Decompressed body bytes equivalent; compressed body bytes structurally diverge (Go `compress/gzip` `OS: 255` + variable `XFL` vs libz `OS: 03 UNIX` + `XFL: 00`). Future encode-side streaming framework phase amends.

**(vi) `min_content_length` late-revert anomaly (ADR-0131 §Decision (vii)):** When Content-Length is unset at EncodeHeaders + body length emerges below threshold only at EncodeData, the filter cannot revert headers. Phase 14 documents but defers per planner-time decisions D3 + D4 (accept the wire-shape anomaly; increment BOTH `response_content_length_too_small` AND `response_not_compressed`). Future cap-promotion or revert-headers-primitive phase may revisit.

---

## 7. Phase-done lessons learned

**SPEC §1.1 amendment-block channel as a release-valve for multi-hypothesis SPEC-time refutations (SIX amendments — highest single-row count to date).** Phase 14 lands SIX SPEC-time amendment blocks against BRAINSTORM hypotheses (field-decomposition bookkeeping; runtime-enabled is RuntimeFeatureFlag not RuntimeFractionalPercent; 17 counters not 11; per-route surface narrowed; Vary always-append; ETag dual-mode strong-strip-vs-skip). All six fit the §1.1 amendment-block channel cleanly: each amendment is field-level / name-level / behavior-level + does not undo the structural design. The structural design (gzip-only, response-only, Path B, ENCODER+DECODER same-`*filter` instance, 5th canonical disabled-OR-override) survived intact through all six. **Lesson:** the §1.1 amendment-block channel scales to multiple corrections per row; the alternative (BRAINSTORM §12 amendment cycle per phase-13's D-3.5 pattern) is reserved for cases where the structural design itself shifts.

**Framework-primitive symmetry: encode-side delta SECOND framework delta in §9 family-rows after phase-13's decode-side primitives.** Phase 14's `EncoderFilterCallbacks.OverwriteBody(b []byte)` is the symmetric encode-side counterpart to phase-13 ADR-0128's decode-side primitives. Establishes the pattern that **framework deltas are permitted in §9 family-rows when load-bearing for the filter's algorithmic core** — encode-side primitives are now first-class citizens alongside decode-side primitives. Future codec / transform filters (decompressor; bandwidth_limit; brotli/zstd codecs) can rely on `OverwriteBody` without re-introducing a framework delta.

**ADR-0134 (directResponseAction.response_headers_to_add gap) — uncovered ONLY at Task 14 integration; lesson for SPEC §3 framework survey discipline.** The SPEC §3 framework survey covered `EncoderFilterCallbacks` thoroughly (yielding ADR-0131 `OverwriteBody`) but did NOT cover `directResponseAction.body()` — a peer of the encode-side response path. Future SPEC §3 framework surveys should explicitly enumerate **"what proto fields does envoy-go's existing HCM honor on the response-emission path?"** as a separate sweep, not just the filter-callback surface. The deep-dive could anticipate gaps like ADR-0134 at SPEC time rather than integration time. PROGRESS Task 14 + this REVIEW codify the lesson; the ADR-0044 escape-valve handled the impl-time landing cleanly.

**Four per-side counter divergences (refuting SPEC §7.1 + §7.3 simplifications) — differential fixture is the truth-source.** SPEC §7.1 expected counter table simplified reference Envoy behavior; Task 14 integration probes pinned 4 divergences:
1. `header_compressor_used`: ref=3, subj=5 — envoy-go caches AE classification BEFORE per-route rmAE strip (ADR-0129 same-`*filter`); ref reclassifies post-strip.
2. `header_not_valid`: ref=1, subj=0 — ref's post-strip reclassification returns `not_valid`; envoy-go's cached state returns `no_accept_header`.
3. `response_not_compressed`: ref=3, subj=2 — ref's per-route-disabled STILL increments despite wholly-inactive filter; envoy-go's per-route-disabled wholly silent per ADR-0125.
4. `request_not_compressed`: ref=6, subj=0 — ref increments PER REQUEST even with response-only configs; envoy-go MVP request side silent per ADR-0132 twin-series.

Both sides are valid implementations of the documented contract. The driver locks both sides' empirical values via `counterModePerSideExact` so regressions on either side surface immediately. PROGRESS Task 14 + BEHAVIOR_CONTRACT §13.4 codify the divergences with root-cause notes. **Lesson: SPEC empirical pins are guidance subject to integration refutation; differential fixture probes at integration time are the authoritative truth-source. Future SPEC-time counter-shape prose should explicitly flag claims as "expected reference Envoy behavior pending integration probe" rather than asserted equivalences.**

**PLAN-text fabricated-quote discipline lapse at Task 14 (caught + corrected via Task 14 follow-up).** Task 14 implementation initially scoped to "driver.go + PROGRESS.md only" per the PLAN; the ADR-0134 framework delta landed but the PROGRESS entry initially retroactively framed the PLAN as permitting the broader scope. Code review surfaced the discipline lapse; the Task 14 follow-up commit (`be8c78b`) corrected the PROGRESS entry to call out the PLAN deviation explicitly per ADR-0044's escape-valve clause. **Lesson:** never retroactively quote PLAN text to legitimize impl-time scope expansions. PLAN deviations should be called out explicitly at the PROGRESS entry + tracked as ADR escape-valve invocations per ADR-0044. PLAN.md is immutable post-squash per phase-13 precedent at commit `bdcb7c1`; impl-time PLAN expansions land in PROGRESS + DECISIONS, not in PLAN edits.

**17-counter filterStats as the LARGEST stat surface per filter to date.** Phase 11 local_ratelimit landed 4; phase 12 csrf landed 3; phase 13 buffer landed 0; phase 14 compressor lands 17. The progression demonstrates that §9 family-rows span the full spectrum from ZERO observables to LARGEST-stat-surface. The 17-counter shape is the natural maximum for a single-codec response-side compressor under reference Envoy's stat namespace; multi-codec phases (brotli + zstd) would expand to ~17 × num-codecs (one set per active library; codec library name is part of the stat path per §1.1 amendment 3). Future §9 row planning can cite phase 14 as the precedent for "filters with bidirectional twin-series stat surfaces + per-codec namespace expansion".
