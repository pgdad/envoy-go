# Phase 19.2 — `ext-proc-body` (sibling SPEC stub)

> **This is a placeholder stub**, not the authoritative 19.2 SPEC. It is drafted at the phase-19 parent SPEC commit (the same commit that authored `docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/SPEC.md`) so the phases directory carries a forward-pointer for 19.2. **It is superseded by `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/SPEC.md`** — the full sub-phase SPEC drafted at 19.2's lifecycle-state 1 → 2, after 19.1 is `done`. Mirrors the `docs/envoy-go/phases/18.2-ext-authz-grpc/` stub-then-full-SPEC pattern.

**Phase id:** `19.2`
**Slug:** `19.2-http-filter-ext-proc-body`
**Status:** `planned` (ROADMAP row `19.2` added `planned` at the phase-19 parent SPEC commit; depends-on `19.1`)
**Parent:** `docs/envoy-go/phases/19-http-filter-ext-proc/SPEC.md` (parent master SPEC — the cross-cutting design §4, the full §5 13-pin empirical-pin block, the §6 amendment block, the §7 10-ADR anchor map).

---

## Scope (per parent SPEC §2)

Phase 19.2 lands `envoy.filters.http.ext_proc` body-stage participation — the body-mode extension landing — against the `internal/filter/http/extproc/` package that 19.1 establishes:

- **The NEW encode-side body-buffering framework primitive (ADR-0175)** — analogous to phase-13 ADR-0128 decode-side body-buffering. Buffers response body bytes across `EncodeData` calls until end_stream; exposes the buffered bytes via a new `EncoderFilterCallbacks` method (`BufferEncodedBody()` or similar — IMPL settles); releases the buffered-and-possibly-mutated body to the downstream wire-write path after the filter's response. Required because per parent §5.P11 REFUTED at SPEC time the existing encode-side `DataStopIterationAndBuffer` is park-only (no replay buffer) and `EncoderFilterCallbacks.OverwriteBody` (ADR-0131) is per-call replacement only (NOT buffer-and-hold). Cross-phase-reusable for any future filter needing encode-side body accumulation.
- **Body-mode activation in `compiledConfig`** — ADR-0168's §Decision is **amended at 19.2 IMPL** to replace the 19.1 PARSE-REJECT for `request_body_mode = BUFFERED` and `response_body_mode = BUFFERED` with the body-stage dispatch. STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED PARSE-REJECT continues envoy-go-strict permanently (out of envelope per Q2).
- **Body-stage dispatch + body_mutation discipline (ADR-0172 body-mode AMENDMENT)** — the request_body stage outbound call uses the existing phase-13 ADR-0128 decode-side body-buffering primitive; the response_body stage outbound call uses the NEW ADR-0175 encode-side body-buffering primitive. `CommonResponse.body_mutation` (`*BodyMutation` oneof: `body []byte` vs `clear_body bool` vs `streamed_response *StreamedBodyResponse` — last arm PARSE-REJECT since STREAMED is out of envelope) — body replacement-when-BUFFERED OR full-clear. The phase-14 ADR-0131 `OverwriteBody` primitive is NOT reused (it's per-call replacement, distinct from buffer-and-replace).
- **`CommonResponse.status = CONTINUE_AND_REPLACE` handling** — supersedes both `header_mutation` + `body_mutation` with combined-replacement semantics; honored only at the header stages with body-mode BUFFERED. ADR-0172 documents.
- **Multi-stage ImmediateResponse extension to body stages** — ImmediateResponse can now fire at request_body + response_body stages in addition to the request_headers + response_headers stages that 19.1 lands.
- **The 25th fuzzer `FuzzProcessingResponseMapping`** at the existing `extproc/fuzz_test.go`. Fuzzes arbitrary `*ProcessingResponse` protobytes → response-mapping → state-machine transition; corpus extends from the 19.1 baseline to cover body-stage CommonResponse variants + CONTINUE_AND_REPLACE permutations.
- **Differential fixture `0023-http-ext-proc-body`** (~4-6 body-stage scenarios) — gRPC body BUFFERED + body_mutation replace, gRPC body BUFFERED + clear_body, gRPC CONTINUE_AND_REPLACE, gRPC multi-stage immediate_response at response_body, gRPC response_body mutation. REUSES the 19.1 test-helper `test/helpers/extprocgrpc/` (same bidi-stream gRPC `Process` server; just adds new scripted response sequences for the body-stage scenarios).

**19.2-landing ADRs:** ADR-0175 (the encode-side body-buffering primitive — §Context anchored at the parent SPEC commit); ADR-0168 §Decision amendment (body-mode PARSE-REJECT lift); ADR-0171 body-mode-state-machine portion (`mode_override` interaction with body stages); ADR-0172 body-mode portion (body_mutation + CONTINUE_AND_REPLACE + body-stage ImmediateResponse). ADR-0044 escape-valve: the SPEC-time scrape closure at parent §5.P11 removes the most-likely escape-valve surface; other possible 19.2 IMPL surfaces: (i) the buffered-body-release-vs-stream-reset interaction with the framework's encode-chain primitive; (ii) the body_mutation_rules application to body bytes (if Envoy applies mutation_rules to body separately from headers — unlikely per the proto's mutation_rules doc which is header-specific).

## Phase-done rollup

Per parent SPEC §8: the 19.2 phase-done commit flips ROADMAP row `19.2` `in-progress → done` AND the parent row `19` `in-progress → done` IN ONE OPERATION (the commit-message body must name both transitions for grep-verifiability). 19.2's phase-done closes the parent row 19; the §9 HTTP filters family then has 12 rows landed; the next §9 family-row is numbered `20`.

## When 19.2 is drafted

After 19.1 is `done`. The 19.2 lifecycle-state 1 → 2 session authors `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/SPEC.md` (the full sub-phase SPEC, mirroring the 19.1 SPEC's 15-section structure scoped to the body-stage surface). Per the 08.2 + 18.2 precedent, 19.2 MAY run its own `superpowers:brainstorming`-scoped-to-SPEC session — the encode-side body-buffering framework-primitive-from-scratch lift may warrant a fresh brainstorm pass; the 19.2 SPEC session makes that call. This stub is superseded at that point.

## References

- Parent master SPEC: `docs/envoy-go/phases/19-http-filter-ext-proc/SPEC.md` (§2 scope table, §3 split rationale, §4 cross-cutting, §5 empirical pins, §6 amendments, §7 ADR map, §8 phase-done gate).
- 19.1 SPEC: `docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/SPEC.md` (the foundational filter scaffold 19.2 extends).
- BRAINSTORM: `docs/envoy-go/phases/19-http-filter-ext-proc/BRAINSTORM.md` (§1.4 split analysis, §3.3 conditional ADR-0175, §7 ADR roster).
- Sibling-stub precedent: `docs/envoy-go/phases/18.2-ext-authz-grpc/README.md` (stub-then-full-SPEC pattern).
