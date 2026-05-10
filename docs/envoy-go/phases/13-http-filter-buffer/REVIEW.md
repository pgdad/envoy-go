# Phase 13 — Code review (REVIEW.md)

**Phase id:** `13` (sixth §9 HTTP-filters family-row to land per ADR-0106; FIRST production filter to demonstrate the "disabled-OR-override sum-type per-route discipline" — 5th canonical per-route shape per ADR-0125; ZERO new stat-table entries; FIRST §9 row to touch the framework's body-buffering machinery)
**Slug:** `13-http-filter-buffer`
**Branch under review:** `phase-13-http-filter-buffer-impl`
**Range:** `3142555` (branch tip; Task 12 beads-removal follow-up) — 12 task commits + SHA-fill / PROGRESS-append / doc-fix / beads-revert follow-ups
**Parent ROADMAP row:** `13 http-filter-buffer` flipped `in-progress → done` at Task 12 commit `a05bb6f` (already landed prior to this REVIEW; row 13's status field reads `done` on the impl branch at HEAD).
**Reviewer method:** Inline authoring by the implementing session per the PLAN's Task 13 explicit allowance; inputs: SPEC §15 acceptance checklist + the branch diff + phase-12 REVIEW.md structural template + PROGRESS.md per-task entries + DECISIONS.md ADR-0125..ADR-0128.
**Six-gate state at HEAD:** all green per Task 12's verification sweep — outputs reproduced verbatim in §4 below.

This review covers the full phase 13 surface: `internal/filter/http/buffer/` package (`doc.go` + `buffer.go` + `buffer_test.go` + `fuzz_test.go`), framework deltas at `internal/filter/hcm/connection.go` (+34 LoC; synthetic empty-terminal RunDecodeData + post-body CL reconciliation per ADR-0128 NEW), `cmd/envoy-go/main.go` boot registration, differential fixture `0015-http-buffer` (6 scenarios, single-listener + three-route topology), `FuzzBufferConfigParse` (seventeenth fuzzer in repo), `BEHAVIOR_CONTRACT.md` §13 four-edit bundle (NEW buffer subsection ~72 LoC + 29-name table preamble note + Equivalence Matrix row + Phase 13 forward-pointer notes ~30 LoC), the four ADRs ADR-0125..ADR-0128, and the ROADMAP row 13 status flip + STATE.md advance.

This REVIEW closes phase 13's lifecycle (state 5 → 6) and is the final task before merge to master.

---

## 1. Phase summary

**APPROVED.**

All six phase-done gates are GREEN at HEAD `3142555` per the Task 12 verification sweep (§4 below). The implementation faithfully realizes the SPEC across all 12 PLAN tasks. The buffer filter is the SIXTH §9 HTTP-filters family-row to ship under ADR-0106 and the first to demonstrate the "disabled-OR-override sum-type" per-route discipline (ADR-0125 codifies this as the 5th canonical per-route shape), the first to interact with the framework's body-buffering machinery (ADR-0076), and the structurally-thinnest §9 row at the stat surface (ZERO new entries to the 29-name table).

The architectural centerpiece is the Continue/DataContinue body-counting algorithm (ADR-0127 v2, post-pivot). `DecodeHeaders` returns `Continue` on header-only requests and `StopIteration` on bodied + non-disabled requests; `DecodeData` accumulates with `DataStopIterationAndBuffer`, fires `SendLocalReply(413, "Payload Too Large", connClose)` + `DataStopIterationNoBuffer` on overflow, and on terminal `endStream=true` invokes `maybeAddContentLength` then `DataContinue`. This algorithm diverges from the original BRAINSTORM §2.6 proposal (which mirrored buffer_filter.cc's StopIteration → HCM-emits-413 delegation); the pivot to the envoy-go-native path (envoy-go emits the 413 itself) emerged from integration testing at Task 11 which surfaced a synchronous-HCM dispatch deadlock in the StopIteration path. ADR-0128 NEW documents the framework primitives that emerged from that constraint.

The differential fixture `0015-http-buffer` is the phase-closing non-vacuous evidence against reference Envoy v1.37.2: 6 scenarios on a single listener with three routes (`/` default + `/route-disabled` + `/route-tighter`), exercising header-only pass-through (scenario 1), under-cap body (scenario 2), exact-cap boundary (scenario 3), over-cap 413 overflow (scenario 4), per-route disabled pass-through (scenario 5), and chunked → fixed-CL Content-Length injection (scenario 6 backend-JSON assertion). All 6 PASS.

Four ADRs landed (ADR-0125 + ADR-0126 + ADR-0127 v2 + ADR-0128 NEW). Phase 13 entered planning with 3 anticipated ADRs (ADR-0128 was provisionally retired in BRAINSTORM §12.7); it landed with 4 — ADR-0128 was restored at Task 12 as a NEW ADR documenting the framework primitives that emerged from the Task 11 synchronous-HCM-dispatch pivot.

---

## 2. ADR roster

Each of the four ADRs ADR-0125..ADR-0128, evaluated for whether the §Decision body held up under implementation + fixture exercise:

**ADR-0125** (`internal/filter/http/buffer/` package shape — single-token directory + decoder-only `HTTPFilter` value with `Encoder: nil` + 5th canonical per-route discipline: disabled-OR-override sum-type with shared-vacuous stats): **VALIDATED.** Single-token directory `buffer/` aligns with `cors/`/`fault/`/`csrf/` precedent. Boot-registration ordering `router → buffer → cors → csrf → envoygotest → fault → header_mutation → localratelimit → ... → Freeze` is exactly as described. Decoder-only `HTTPFilter` value matches phase-12 csrf precedent. The 5th canonical per-route shape (disabled-OR-override sum-type) is a NEW shape that subsumes neither the data-only-override (ADR-0073) nor the stateful-INDEPENDENT-stats (ADR-0117) nor the data-only-SHARED-stats (ADR-0124) precedents — all four prior shapes remain valid precedents for future filters.

**ADR-0126** (`compiledConfig` shape + 1-consumed/0-deferred field decomposition (`max_request_bytes`) + parse-time `max_request_bytes ≤ 1 MiB` validation (envoy-go-only divergence) + cap-layering rationale): **VALIDATED.** The 1-field decomposition held throughout; parse-time validation rejects nil / zero / > 1048576 at `New` time. The `≤ 1 MiB` ceiling is the load-bearing envoy-go-only constraint (prevents the filter's own cap from exceeding the framework's `filterBufferLimitBytes = 1 << 20` safety net per ADR-0076). Per-route override `BufferPerRoute.buffer.max_request_bytes` subject to the same validation via `parsePerRoute`.

**ADR-0127 v2** (Body-counting + 413-trigger algorithm — STREAMING-CAP ONLY; Continue/DataContinue post-pivot; maybeAddContentLength mirror; 413 wire shape reuse; 100-Continue addendum RETRACTED): **VALIDATED with in-place §Decision update at Task 12.** The Task 3 ADR text carried the original StopIteration/DataStopIterationAndBuffer algorithm; Task 11 integration surfaced the synchronous-HCM-dispatch deadlock; Task 12 updated ADR-0127 v2 in-place (Context + Decision (i)/(ii)/(v) + Consequences) to reflect the Continue/DataContinue algorithm. Decision (v) — the 100-Continue addendum — was RETRACTED at Task 12 when phase-04 `connection.go:122` was found to categorically 417 all `Expect:` headers before reaching the filter chain; the retraction is tracked as a forward-pointer note in BEHAVIOR_CONTRACT.md §13.4.

**ADR-0128 NEW** (Framework primitives for chunked-body end-stream detection + CL reconciliation — synthetic empty-terminal `RunDecodeData` + post-body CL reconciliation in `internal/filter/hcm/connection.go`): **VALIDATED.** Provisionally retired in BRAINSTORM §12.7 (under the "no framework deltas" hypothesis); restored at Task 12 as a NEW ADR after the +34 LoC framework delta at Task 11 was confirmed load-bearing. The ADR documents the two primitives, the synchronous-HCM-dispatch constraint that necessitated them, and the SPEC §4 "no framework deltas" invariant that was retired by this delta.

---

## 3. Empirical pins outcome

All 11 SPEC §11 pins were resolved at SPEC drafting; no new divergences emerged during implementation. The most load-bearing pins for the eventual algorithm:

- **§11.6 (no Content-Length fast-fail in DecodeHeaders)** — LOAD-BEARING. Refuted the BRAINSTORM §2.6 fast-fail clause; drove the v1→v2 ADR-0127 transition. Implementation confirmed: `DecodeHeaders` contains no Content-Length inspection.
- **§11.8-CL (maybeAddContentLength chunked → fixed-CL injection)** — LOAD-BEARING. Scenario 6 fixture assertion (`Content-Length: 10240` at backend boundary) is the primary non-vacuous claim for this pin.
- **§11.5 (ZERO new stat-table entries; `downstream_rq_too_large` is Envoy-only)** — Minor prose correction at SPEC drafting (BRAINSTORM §12.5 had mis-cited the counter as in-table); the "ZERO new entries" claim survived intact.
- **§11.7 (413 wire shape: 4-header set + 17-byte body + Connection:close)** — Verified byte-equivalent in fixture scenario 4.
- **§11.3 (per-route `disabled:false` is defensively rejected at parse time)** — Verified by `parsePerRoute` PGV-mirror and unit tests Group 2.

All remaining §11 pins (§11.1 max_request_bytes validation, §11.2 cap predicate `>` not `>=`, §11.4 passthrough flag, §11.9 endStream=true terminal) confirmed as anticipated at SPEC drafting; no surprises during implementation.

---

## 4. Gate-by-gate evidence

Verbatim from PROGRESS.md Task 12 outputs. All 6 gates green:

**Gate A — build / vet / lint clean:**
```
$ go build ./...
(clean — no output)
$ go vet ./...
(clean — no output)
$ golangci-lint run ./...
(clean — no output)
Gate A: CLEAN
```

**Gate B — race-test pass on 36 packages:**
```
$ go test -race -count=1 ./...
ok  github.com/esalaine/envoy-go/internal/filter/http/buffer  1.043s
[... 35 other packages PASS; differential suite 47.730s ...]
All 36 packages PASS; no race violations.
```

**Gate C — h2spec 53/53 PASS:**
```
$ go test -count=1 -v ./test/conformance/h2spec/
        53 tests, 53 passed, 0 skipped, 0 failed
--- PASS: TestH2Spec (2.29s)
ok  github.com/esalaine/envoy-go/test/conformance/h2spec  2.352s
```

**Gate D — 17 fuzzers green at 30s budget:**
```
$ go test -fuzz=FuzzBufferConfigParse -fuzztime=30s -run=^$ ./internal/filter/http/buffer/
fuzz: elapsed: 30s, execs: 4429085 (38609/sec), new interesting: 13 (total: 157)
PASS  ok  github.com/esalaine/envoy-go/internal/filter/http/buffer  31.053s
All 17 fuzzers PASS at 30s budget (FuzzBufferConfigParse — 17th; 16 prior clean).
```

**Gate E — 16 differential fixtures 0000-0015 PASS:**
```
$ go test -count=1 -v ./test/differential/ -run TestDifferential
    --- PASS: TestDifferential/0015-http-buffer (1.47s)
    [... 15 prior fixtures 0000-0014 all PASS ...]
--- PASS: TestDifferential (44.98s)
ok  github.com/esalaine/envoy-go/test/differential  45.806s
```

**Gate F — BEHAVIOR_CONTRACT 4-edit bundle landed:**
```
$ grep -n "^### envoy.filters.http.buffer" docs/envoy-go/BEHAVIOR_CONTRACT.md
1150:### envoy.filters.http.buffer
$ grep -n "Phase 13 (buffer filter) note" docs/envoy-go/BEHAVIOR_CONTRACT.md
215:**Phase 13 (buffer filter) note:** ...
$ grep -n "HTTP filter.*envoy.filters.http.buffer" docs/envoy-go/BEHAVIOR_CONTRACT.md
35:| HTTP filter `envoy.filters.http.buffer` | 0015-http-buffer: ...
$ grep -n "Phase 13 forward-pointer notes" docs/envoy-go/BEHAVIOR_CONTRACT.md
1616:### Phase 13 forward-pointer notes
All 4 edits confirmed present.
```

---

## 5. Acceptance checklist

Per SPEC §15. All items checked; deviations explicitly noted.

- [x] `internal/filter/http/buffer/` package exists with `doc.go` + `buffer.go` + `buffer_test.go` + `fuzz_test.go`.
- [x] `cmd/envoy-go/main.go` registers `buffer.New` alphabetically (`router → buffer → cors → csrf → ...`).
- [x] `New` factory rejects nil `MaxRequestBytes`, value 0, value > 1048576; envoy-go-own error wording per ADR-0126 + D3.
- [x] `compiledConfig` shape: 1 actively-consumed field; no `*filterStats` (ZERO filter-specific counters).
- [x] `compiledPerRoute` shape: `disabled bool` + `maxOverride *uint32`; mutually exclusive at runtime per oneof discipline.
- [x] `parsePerRoute` handles all 4 cases: `*BufferPerRoute_Disabled`, `*BufferPerRoute_Buffer`, nil (PGV-required mirror), `disabled:false` defensive rejection.
- [x] `DecodeHeaders` body: header-only `Continue`; per-route disabled `Continue` + passthrough flag; bodied + non-disabled `StopIteration`; NO Content-Length inspection per §11.6.
- [x] `DecodeData` body: passthrough flag `DataContinue`; cap predicate `>` not `>=`; overflow `SendLocalReply(413, "Payload Too Large", connClose)` + `DataStopIterationNoBuffer`; terminal `endStream=true` invokes `maybeAddContentLength` then `DataContinue`; in-flight `DataStopIterationAndBuffer`.
- [x] `maybeAddContentLength` sets `Content-Length` AND drops `Transfer-Encoding: chunked`; mirrors `buffer_filter.cc:91-97`; §11.8-CL verified by fixture scenario 6.
- [x] `DecodeTrailers` invokes `maybeAddContentLength` defensively per §6.6.
- [x] Encoder methods absent; `OnDestroy` no-op; decoder-only `HTTPFilter` value per §6.7.
- [x] Per-route override semantics: disabled-OR-override sum-type; SHARED-vacuous stats (no filter-specific counters to share or split) per ADR-0125.
- [x] Stat surface: ZERO new entries to 29-name table; `downstream_rq_4xx` is the in-table observable; `downstream_rq_too_large` + `downstream_rq_completed` are Envoy-only and filtered out per twin-series discipline.
- [x] Rejection wire shape: `content-length: 17`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`; 17-byte `Payload Too Large` body, no LF; status 413; `Connection: close`; per §11.7 + §11.8; verified byte-equivalent in fixture scenario 4.
- [~] 100-Continue addendum: **RETRACTED.** SPEC §15 item listed "HCM/H1-codec emits `100 Continue` independent of buffer filter on `Expect: 100-continue`"; retracted at Task 12 when phase-04 `connection.go:122` was found to categorically 417 all `Expect:` headers. Deferral captured inline in BEHAVIOR_CONTRACT.md §13.4 per ADR-0040. This is the ONE §15 item that does not check green.
- [x] `maybeAddContentLength` byte-equivalent at backend boundary: fixture 0015 scenario 6 backend asserts `Content-Length: 10240` on chunked-passthrough request.
- [x] Differential fixture 0015 6-request matrix green per §7.1.
- [x] `FuzzBufferConfigParse` green at 30s budget (17 fuzzers total).
- [x] All 15 prior differential fixtures still green; 16 prior fuzzers still green; h2spec 53/53 still PASS.
- [x] `BEHAVIOR_CONTRACT.md` §13 4-edit bundle at phase-done commit.
- [x] `DECISIONS.md` carries 4 new ADRs (ADR-0125, ADR-0126, ADR-0127 v2, ADR-0128 NEW). ADR-0128 originally provisionally retired; restored at Task 12 for framework-primitive documentation.
- [x] REVIEW.md authored: THIS document.

---

## 6. Forward-pointer roster

Per BEHAVIOR_CONTRACT.md §13.4 "Phase 13 forward-pointer notes":

**(i) `max_request_bytes > 1 MiB` envoy-go-only parse-time rejection (ADR-0126):** Reference Envoy v1.37.2 accepts arbitrary `UInt32Value` at parse time; envoy-go rejects values > 1048576. This divergence is intentional (cap-layering under ADR-0076 `filterBufferLimitBytes = 1 << 20`); a future cap-promotion phase is the natural amender per ADR-0076 §Consequences (d).

**(ii) `per_connection_buffer_limit_bytes` / `per_request_buffer_limit_bytes` silent-ignored (ADR-0076):** Both fields are deferred per the existing ADR-0076 framework body-cap deferral. These fields would interact with the buffer filter's own cap in non-trivial ways; the cap-promotion phase is the correct amender.

**(iii) Phase-04 `Expect: 100-continue` handling — 417 instead of forwarding (ADR-0127 v2 §Decision (v) retraction):** `connection.go:122` categorically rejects all `Expect:` headers with 417 Expectation Failed before reaching the filter chain. The ADR-0127 v2 100-Continue addendum was authored on the assumption that the buffer filter would observe Expect-gated requests; the rejection at connection.go makes this moot. Fix in future phase-04 fix bundle; tracked inline at BEHAVIOR_CONTRACT.md §13.4.

---

## 7. Phase-done lessons learned

**Post-landing BRAINSTORM §12 amendment as a release-valve precedent (D-3.5 pattern).** The BRAINSTORM hypothesis (Content-Length fast-fail in `DecodeHeaders`, mirroring `buffer_filter.cc:67-72`) was empirically refuted at SPEC §11.6. The §12 in-place amendment retired the hypothesis cleanly and reframed the algorithm as STREAMING-CAP-ONLY; the SPEC was authored against the corrected hypothesis. This confirms the D-3.5 "amend brainstorm in-place when empirical findings invalidate the hypothesis" pattern is workable: the brainstorm is preserved verbatim as §§1-11 for historical context; §12 is the authoritative design input for the SPEC/PLAN sessions. The pattern avoids branching the design narrative across multiple documents.

**v2-numbered ADR convention for retired-clause supersessions.** ADR-0127 v2 (vs an implicit v1) makes the supersession explicit at authoring time. The Task 12 in-place update of v2 (rather than bumping to v3) is a workable refinement: in-place updates are appropriate for behavioral clarifications within the same phase (the algorithm was rewritten but the ADR's structural purpose — "body-counting algorithm" — stayed the same); v3+ bumps are appropriate for cross-phase supersessions where the original decision is fully closed and a new phase opens a distinct decision scope.

**"ZERO new stat-table entries" as the structurally-thinnest §9 row data point.** Phase 13 demonstrates that some §9 family-row filters have no observable on the filter-namespace at all, relying entirely on the existing HCM-namespace `downstream_rq_4xx` counter for differential equivalence (per ADR-0125 §Consequences (b)). This is the counterpoint to phase 12 csrf's 3-counter additive expansion and phase 11 local_ratelimit's SN9 rule addition. Future brainstorms that anticipate a "no new stat-surface" outcome should cite phase 13 as the precedent and plan accordingly (no stat-registration code; no `filterStats` struct; the `downstream_rq_4xx` in-table counter is the only stat-surface observable).

**Synchronous-HCM dispatch deadlock as a brainstorm-blind-spot.** The original ADR-0127 algorithm (Task 3 as authored) specified `StopIteration` in `DecodeHeaders` and `DataStopIterationAndBuffer` in `DecodeData`, mirroring `buffer_filter.cc:67` and relying on HCM to emit the 413. Integration testing at Task 11 surfaced a deadlock: envoy-go's synchronous-HCM-dispatch path does not support the "filter returns StopIteration; HCM buffers and re-dispatches" delegation because there is no re-dispatch mechanism in the synchronous path. The pivot to Continue/DataContinue (envoy-go emits the 413 itself via `SendLocalReply`) required +34 LoC of framework primitives at `connection.go` (synthetic empty-terminal `RunDecodeData` + post-body CL reconciliation) and an entirely rewritten §Decision block in ADR-0127 v2. **Lesson for future §9 family-row brainstorms:** filters that mirror reference Envoy algorithms using `StopIteration`/`DataStopIterationAndBuffer` semantics should explicitly verify that the chosen status codes do not conflict with envoy-go's synchronous-HCM constraint. ADR-0128 documents the framework primitives that emerged; ADR-0128 §Context is the canonical reference for "what the synchronous-HCM constraint means for body-consuming filters."

**Framework-delta scope creep.** Phase 13 entered with SPEC §4 "no framework deltas" claim (inherited from BRAINSTORM §12.7 which provisionally retired ADR-0128); it landed with +34 LoC of new HCM primitives in `connection.go`. The SPEC §4 claim was retroactively amended in-place per ADR-0052 at Task 12. **Lesson:** future brainstorms should hold a `## Framework deltas required?` checkpoint explicitly, particularly for filters that consume framework primitives outside the canonical chain-iteration set (body-accumulation, CL-reconciliation, chunked-body end-stream detection). The "no framework deltas" hypothesis should be treated as a working assumption to be verified at integration testing — not a guaranteed constraint.

**Beads-tracker false-start as a directive-clarity lesson.** Task 12 implementer initialized the beads issue tracker to track the phase-04 Expect-handling deferral (item (iii) above). The user subsequently rejected beads use in this project, requiring a force-push revert of the unauthorized master commit (`a628d69`) back to `63850f6`, beads removal, and relocation of the deferral to BEHAVIOR_CONTRACT.md §13.4 per ADR-0040 inline-deferral discipline. **Lesson:** out-of-scope deferrals should default to the inline `BEHAVIOR_CONTRACT.md §forward-pointer-notes` pattern per ADR-0040 unless the user explicitly authorizes an external tracker. The default is "no external tracker; inline deferrals only." Implementers should not initialize external tracking infrastructure (issue trackers, project boards, etc.) without explicit user direction.
