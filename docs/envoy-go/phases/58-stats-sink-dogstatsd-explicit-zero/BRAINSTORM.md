# Phase 58 Brainstorm — `dog_statsd` explicit-`max_bytes_per_datagram: 0` parity fix (the ELEVENTH Observability-family row; the SMALLEST row yet — a ONE-arm parse-reject mirroring the phase-57 graphite reject VERBATIM modulo the sink name; closes the recorded phase-50/57 reference-parity gap; NO new fixture / NO new fuzzer / +0 stats / +0 packages / +0 modules)

> **Stage:** BRAINSTORM (lifecycle-state 0 → 1). Docs-only; no `.go` changes at this stage. Fresh worktree `.worktrees/phase-58-brainstorm`, branch `phase-58-stats-sink-dogstatsd-explicit-zero-brainstorm`, per `feedback_git_worktrees`.
>
> **Loop re-open (AUTONOMOUS — no human pick):** phase 57 (`stats-sink-graphite`) landed COMPLETE (row 57 `done`, ADR-0275). Per the **STANDING DIRECTIVE (human, 2026-07-12)** the loop runs AUTONOMOUSLY until the termination sentinel fires; the sentinel was re-checked MECHANICALLY at master `6d3a4330` and does NOT fire (check (1) silent — every row `done` — but checks (2) [Observability + Operational-tooling deferred lists] and (3) [five never-opened families] each still print, and each independently blocks `stop`). So the roller SELF-PICKED the next subject (§2.1): the **smallest defensible candidate** — the `dog_statsd` explicit-`max_bytes_per_datagram: 0` parity fix — over three declined alternatives (recorded §2.1). No human pause; no `stop` file.
>
> **Baselines re-verified against master tip `6d3a4330` (the phase-57 IMPL squash `2825ca90` + the router refresh `6d3a4330`):** stat surface **1201** · fixtures **103** (tail `0101-stats-sink-graphite`) · fuzzers **54** (tail `FuzzGraphiteStatsdSinkConfigParse`) · BackendKind tail **38** (`H2GoawayResponder`) · DECISIONS tail **ADR-0275** (next-free **ADR-0276**) · new Go packages **0** · new go.mod modules **0**. Counts are UNCHANGED at a BRAINSTORM (docs-only). All `file:line` citations below were RE-DERIVED from source this session (`feedback_brief_citations_not_evidence`) — see §11.

---

## 1. Mission and scope confirmation (58 — a ONE-arm parse-reject, mirroring phase 57)

### 1.1 What phase 58 delivers as a self-contained whole (a single reject arm closing a recorded parity gap)

A **single strict-reject arm** added to `parseDogStatsdSinkConfig` (`internal/bootstrap/bootstrap.go:712`) so that an **explicit** `max_bytes_per_datagram: 0` on a `dog_statsd` `stats_sinks[]` entry boots-rejects — mirroring the phase-57 graphite arm (`bootstrap.go:750-752`) VERBATIM modulo the sink name:

```go
if w := dsd.GetMaxBytesPerDatagram(); w != nil && w.GetValue() == 0 {
    return fmt.Errorf("bootstrap: stats_sinks[%d]: dog_statsd max_bytes_per_datagram must be greater than 0", idx)
}
```

This closes the recorded reference-parity gap: the reference's `DogStatsdSink` proto carries the IDENTICAL PGV `gt: 0` rule on `max_bytes_per_datagram` (`stats.pb.validate.go` — re-derive at SPEC, cited by SPEC-57 §11 A4b as `stats.pb.validate.go:1144-1150`) that graphite's phase-57 arm now enforces, but the LANDED phase-50 `parseDogStatsdSinkConfig` consumes an explicit 0 UNCHECKED (`dsd.GetMaxBytesPerDatagram().GetValue()` directly into the config, `bootstrap.go:724`). The fix is possible — and distinguishable from a legitimate ABSENT field — because `DogStatsdSink.max_bytes_per_datagram` is a `*wrapperspb.UInt64Value` (nil ⇔ absent; non-nil-with-value-0 ⇔ explicit 0), EXACTLY the property phase 57 relied on for graphite.

Nothing else changes in production behavior: an ABSENT `max_bytes_per_datagram` still parses to `0` = one-line-per-datagram (UNCHANGED); an explicit `>0` cap still batches (UNCHANGED); the delta/tag/UDP emit path is BYTE-untouched.

### 1.2 What phase 58 does NOT deliver (forward to §8)

No change to the graphite sink (its arm already exists — phase 57). No change to the plain `statsd` sink (its proto has NO `max_bytes_per_datagram` field at all — batching is a `DogStatsdSink`-only knob, confirmed at phase 50). No new sink, no new transport, no new stat, no new fixture, no new fuzzer, no new package/module. The remaining Observability deferred candidates (`OTLP-metrics` sink + tracing `custom_tags`/`spawn_upstream_span`/`http_service`/force-trace) are untouched and carry forward (§8); after row 58 the dog_statsd explicit-zero candidate rolls OUT of the live deferred sentence (§9, sentinel check-(2) re-run required).

### 1.3 Phase-done as the ELEVENTH Observability-family row landing (family STAYS OPEN)

Row 58 is the ELEVENTH Observability-family row. It is NOT a new `stats_sinks[]` consumer — it is a robustness/parity fix to the EXISTING dog_statsd consumer (the third, phase 49/50). After phase 58 phase-done the family STAYS OPEN — the deferred candidates in §8 remain (so the sentinel check-(2) still prints ⇒ the loop continues).

### 1.4 ADR-0045 split readiness — a SINGLE FLAT ROW, the SMALLEST yet (escape-valve unconsumed) *(self-answered)*

A SINGLE FLAT ROW. The anticipated task count is ~5–7 (§ below), FAR under the ADR-0045 `~15` ceiling — this is the smallest row the project has chartered. There is no second subsystem to strand; the graphite reject shape it mirrors landed THIS WEEK (phase 57). The escape valve is documented UNCONSUMED and re-armable only in the (very unlikely) event the SPEC's task count surprises upward.

### 1.5 Seed-stub alignment + package placement — ALL edits in EXISTING files, ZERO new files

- Production arm: `internal/bootstrap/bootstrap.go` `parseDogStatsdSinkConfig` (existing).
- Test: `internal/bootstrap/bootstrap_test.go` (existing) — CONVERT the existing accept-test into a reject arm (§6, the load-bearing non-additive edit).
- Fuzz SEED: `internal/bootstrap/dogstatsd_fuzz_test.go` (existing `FuzzDogStatsdSinkConfigParse`) — a NEW seed, NOT a new fuzzer.
- Docs: `internal/bootstrap/bootstrap.go` doc comments (three sites, §6) + `docs/envoy-go/BEHAVIOR_CONTRACT.md` (four sites, §6) + ROADMAP/STATE/DECISIONS.

ZERO new `.go` files, ZERO new packages, ZERO new modules.

### 1.6 No prebrainstorm-notes branch

No off-master prebrainstorm-notes branch exists for this subject (the phase-50 batching work is fully landed; this is a pure parity follow-on recorded IN the phase-57 SPEC, not a stashed WIP).

### 1.7 Phase 58's relationship to the existing seams (a ONE-line reject arm on an existing parser + doc reconciliation)

No new framework piece, no new seam, no reuse-of-a-core beyond the `*wrapperspb.UInt64Value` nil-vs-explicit-0 distinction the graphite arm already exploits. The ONLY novel production code is the four-line reject arm; everything else is doc/test reconciliation. This is the mirror image of the phase-57 reject arm (`bootstrap.go:750-752`) — the same PGV `gt: 0` semantic, applied to the sibling sink whose landed parse skipped it.

---

## 2. Design decisions

### 2.1 Row + subject confirmation: the Observability family continues with the dog_statsd explicit-zero parity fix *(SELF-PICKED per the standing directive → phase 58 row registered)*

The FIRST decision, made AUTONOMOUSLY (no human pick) per the 2026-07-12 standing directive. Picked as the **smallest defensible candidate**: a one-arm parse addition reusing a shape landed THIS WEEK (phase 57), closing a recorded reference-parity gap (SPEC-57 §2 explicitly recorded it as "a one-line parity fix candidate for a future robustness sweep"). Row 58 registers `in-progress` AT this BRAINSTORM commit per the ROADMAP §Schema invariant.

**Rejected alternatives (recorded per the standing directive):**
- **OTLP-metrics stats sink** — a full gRPC `stats_sinks[]` consumer (OpenTelemetry metrics export); a substantial new-sink row (new proto, new gRPC submit path), much larger than a one-arm fix. Deferred; remains the largest Observability sink follow-on.
- **Tracing follow-ons** (`custom_tags` / `spawn_upstream_span` / `http_service` / force-trace) — each taps the phase-46 tracing engine seams; larger than a parse fix and better batched together. Deferred.
- **Opening a new family** (xDS the most consequential — dynamic config) — a large charter; the standing directive says smallest-defensible-first, and the Observability tail still holds cheap candidates. Deferred; revisit when the Observability sink/parity tail is drained.

### 2.2 The fix shape: mirror the phase-57 graphite arm VERBATIM modulo the sink name *(self-answered; the template is landed)*

The graphite arm (`bootstrap.go:750-752`) is the exact template:
```go
if w := g.GetMaxBytesPerDatagram(); w != nil && w.GetValue() == 0 {
    return fmt.Errorf("bootstrap: stats_sinks[%d]: graphite_statsd max_bytes_per_datagram must be greater than 0", idx)
}
```
Phase 58 adds the same arm to `parseDogStatsdSinkConfig`, substituting `dsd` for `g` and `dog_statsd` for `graphite_statsd`. Placement: BEFORE the `append(result.DogStatsdSinkConfigs, …)` (currently `bootstrap.go:721`), so a rejected config never reaches the slice — the graphite ordering (`bootstrap.go:750` sits between the address parse and the append). The message substring `dog_statsd max_bytes_per_datagram must be greater than 0` is DISTINCT from graphite's `graphite_statsd …` substring (ADR-0080 anti-silent-divergence: each reject arm carries its own distinguishable substring). SPEC pins **D-DZ-REJECTMSG** (below).

### 2.3 The reject MESSAGE is envoy-go's OWN, NOT the reference's PGV string — behavior parity, not text parity *(self-answered; the phase-48/49/57 posture)*

A subtlety worth pinning at the BRAINSTORM so the SPEC probe is framed correctly: envoy-go's bootstrap reject messages are its OWN (`bootstrap: stats_sinks[%d]: dog_statsd max_bytes_per_datagram must be greater than 0`), NOT a copy of the reference's PGV wording (`Proto constraint validation failed (DogStatsdSinkValidationError.MaxBytesPerDatagram: value must be greater than 0)`). The reference probe (D-DZ-REJECTMSG) confirms only that the reference DOES boot-reject the explicit 0 (BEHAVIOR parity — both processes exit rather than accept), which SPEC-57 §11 A4b already strongly implies via the graphite arm's identical PGV rule; the anticipated reference wording is `DogStatsdSinkValidationError.MaxBytesPerDatagram: value must be greater than 0` (graphite's `GraphiteStatsdSinkValidationError.MaxBytesPerDatagram` with the sink name swapped). Anticipated: LOW risk — the reference's PGV rule for `DogStatsdSink.max_bytes_per_datagram` is re-derivable from the proto descriptor without even a live probe, but ONE fresh-container probe arm (`reference_probe_fresh_container_per_arm`) confirms it empirically.

### 2.4 The NON-additive edit: an existing ACCEPT test becomes a REJECT test *(self-answered; the load-bearing IMPL risk, §6)*

Unlike the phase-57 graphite arm (which was NEW code with NEW tests), this row touches a pre-existing test that asserts the OLD (buggy) behavior. `TestDogStatsdSink_AcceptMaxBytesPerDatagramZero` (`bootstrap_test.go:2550`) currently asserts an explicit `max_bytes_per_datagram: 0` is ACCEPTED and yields `MaxBytesPerDatagram == 0`. After the fix that config REJECTS. So this row is NOT purely additive: the accept-test must be CONVERTED (deleted + replaced by a reject arm, mirroring graphite's `explicit_max_bytes_per_datagram_zero` row in `TestGraphiteStatsdSink_Rejects`-equivalent, `bootstrap_test.go:2890-2899`). The sibling `TestDogStatsdSink_AcceptMaxBytesPerDatagramAbsent` (`bootstrap_test.go:2528`, ABSENT field ⇒ 0) STAYS accept — absent is still legal. This asymmetry (absent-accept vs explicit-0-reject) is the whole point of the wrapper type and MUST be preserved as two distinct test cases. SPEC pins **D-DZ-TESTROWS**.

### 2.5 Fixture posture: anticipated NO new fixture — a boot-reject is parse-time *(self-answered; the phase-49/50/57 reject-roster precedent → SPEC confirms D-DZ-FIXTURE)*

Boot-rejects are parse-time and were proven by SUBJECT unit tests + fuzz seeds in EVERY prior stats-sink row (phase 48/49/50/57 reject rosters are all unit-tested, NOT fixture-backed — a rejected bootstrap never reaches the differential runner, which needs a bootable config on both sides). Anticipated: NO new fixture (fixtures STAY **103**). SPEC confirms via D-DZ-FIXTURE; the escape hatch (a fixture proving the reference ALSO rejects) is unnecessary because the reference-reject is confirmed by the D-DZ-REJECTMSG probe, not a differential.

### 2.6 Fuzz posture: a SEED added to the EXISTING fuzzer — NO new fuzzer *(self-answered; the fuzzer count stays 54 → SPEC confirms D-DZ-FUZZSEED)*

The dog_statsd parse path already has `FuzzDogStatsdSinkConfigParse` (`dogstatsd_fuzz_test.go:11`). The new reject arm is exercised by adding an explicit-`max_bytes_per_datagram: 0` SEED to that fuzzer's corpus (mirroring the graphite fuzzer's own explicit-0 seed, `graphite_fuzz_test.go:63-70`) — NOT a new fuzzer. Fuzzer count STAYS **54** (`reference_fuzzer_count_docs_drift`: reconcile the documented running total against actual `^func Fuzz` before AND after — the count must not move). SPEC confirms D-DZ-FUZZSEED.

### 2.7 Stat surface hypothesis: +0 *(self-answered; a parse-reject registers no stat)*

A parse-reject registers nothing. Anticipated stat surface **1201 (+0)**, UNCHANGED. No `NoNewStat` guard is even implicated (the dog_statsd sink's registration path is untouched).

---

## 3. Framework-survey result — a ONE-line reject arm on an existing parser; ZERO new packages/modules/framework (58 anticipated)

### 3.1 Framework: NO new piece

No new interface, no new type, no new seam. `parseDogStatsdSinkConfig` gains four lines; every other symbol is pre-existing.

### 3.2 NEW packages: NONE

All edits land in `internal/bootstrap` (existing) + `docs/`. No new package.

### 3.3 go.mod modules: NONE

No new import (the `*wrapperspb.UInt64Value` type is already imported and consumed by the existing `dsd.GetMaxBytesPerDatagram().GetValue()`). `go mod tidy -diff` anticipated EMPTY.

### 3.4 REUSES

- **phase-57** the graphite reject arm (`bootstrap.go:750-752`) as the VERBATIM template + the graphite `explicit_max_bytes_per_datagram_zero` reject-test row + the graphite fuzzer's explicit-0 seed as the test/seed templates.
- **phase-50** the `DogStatsdSinkConfig.MaxBytesPerDatagram` field + `parseDogStatsdSinkConfig` (the parser being amended).
- **the `*wrapperspb.UInt64Value` wrapper semantics** (nil-absent vs explicit-0) — the property that makes the reject arm distinguishable, exactly as graphite relied on.

---

## 4. Bootstrap-level applicability — the `stats_sinks[]` surface (NOT per-listener)

The dog_statsd sink is a BOOTSTRAP-level `stats_sinks[]` entry (the phase-47/48/49 surface), NOT a per-listener filter. `parseStatsSinks` dispatches each entry by type URL; the dog_statsd case (`bootstrap.go:557`) calls `parseDogStatsdSinkConfig`, which is where the new reject arm lands. No dispatch change (the type URL and the case are unchanged — this is purely a validation added INSIDE the existing arm).

---

## 5. Stat surface hypothesis — +0 (58)

### 5.1 Stat names (SPEC confirms)

NONE. A parse-reject registers no stat.

### 5.2 envoy-go-strict departure flags

NONE new. This row REMOVES a departure (envoy-go previously ACCEPTED an explicit-0 the reference rejects — a silent divergence); after the fix envoy-go is CLOSER to the reference (both reject). The inherited hostname-accepting departure (statsd/dog_statsd `net.ResolveUDPAddr` accepts hostnames the reference rejects) is ORTHOGONAL and unchanged.

### 5.3 Anticipated surface arithmetic

Stat surface **1201 → 1201 (+0)**.

---

## 6. Edit-site enumeration — the load-bearing part of THIS row (SPEC pins D-DZ-DOCSHAPE + D-DZ-TESTROWS)

Because the production change is trivial, the ROW'S RISK is entirely in reconciling the DOCS and the NON-ADDITIVE test. The SPEC MUST enumerate every site; this BRAINSTORM lists the ones re-derived this session (each RE-DERIVED at SPEC per `feedback_brief_citations_not_evidence`):

**Production (`internal/bootstrap/bootstrap.go`):**
1. **The reject arm** — `parseDogStatsdSinkConfig`, before the append (currently `bootstrap.go:721`). [ADD]
2. **`DogStatsdSinkConfig` struct doc** (`bootstrap.go:318-330`) — line ~322 "`0 = one metric per datagram, the phase-49 default`" and the field comment line ~329 "`0 (absent or explicit) means "one metric per datagram"`" both say explicit-0 is legal. Flip to absent-only + explicit-0-rejected, mirroring the graphite field comment (`bootstrap.go:345`: "`0 (absent only — explicit 0 is parse-rejected)`"). [EDIT — makes the dog_statsd comment match graphite's]
3. **`parseDogStatsdSinkConfig` func doc** (`bootstrap.go:702-711`) — "`0 (absent or explicit) means one metric per datagram (phase-49 behavior, UNCHANGED)`" ⇒ split absent (accept, → one-per-datagram) from explicit-0 (reject); add the new-arm sentence mirroring the graphite func doc (`bootstrap.go:729-740`). [EDIT]
4. **`parseGraphiteStatsdSinkConfig` func doc NOTE** (`bootstrap.go:738-740`) — currently "`the landed dog_statsd parse does NOT enforce its identical PGV rule … a pre-existing phase-50 parity gap, deferred (SPEC-57 §2), NOT fixed here`" — becomes STALE once phase 58 fixes it. Update to "`… now enforced by phase 58 (ADR-0276)`" (or remove the "NOT fixed here" clause). [EDIT — historical-accuracy]

**Test (`internal/bootstrap/bootstrap_test.go`):**
5. **`TestDogStatsdSink_AcceptMaxBytesPerDatagramZero`** (`bootstrap_test.go:2550`) — CONVERT: delete this accept-test and add an `explicit_max_bytes_per_datagram_zero` reject row to `TestDogStatsdSink_Rejects` (`bootstrap_test.go:2458`), `errSubs: []string{"bootstrap:", "dog_statsd max_bytes_per_datagram must be greater than 0"}` (the graphite reject-row shape, `bootstrap_test.go:2890-2899`). [CONVERT — the NON-additive edit, §2.4]
6. **`TestDogStatsdSink_AcceptMaxBytesPerDatagramAbsent`** (`bootstrap_test.go:2528`) — STAYS (absent ⇒ accept ⇒ 0). Do NOT touch. [KEEP — proves the absent/explicit-0 asymmetry]
7. **`FuzzDogStatsdSinkConfigParse` seed** (`dogstatsd_fuzz_test.go`) — ADD an explicit-`max_bytes_per_datagram: 0` seed (graphite fuzzer precedent, `graphite_fuzz_test.go:63-70`). [ADD — no new fuzzer]

**BEHAVIOR_CONTRACT (`docs/envoy-go/BEHAVIOR_CONTRACT.md`):** the router noted the gap is named in TWO places; re-derived this session there are TWO gap-NAMING sites plus TWO correctness sites:
8. **dog_statsd Strict-rejects** (`BEHAVIOR_CONTRACT.md:785`) — currently lists only the missing-specifier + sibling-TypeURL rejects; ADD the explicit-`max_bytes_per_datagram: 0` reject arm. [EDIT — correctness]
9. **dog_statsd batching** (`BEHAVIOR_CONTRACT.md:787`) — "`An ABSENT max_bytes_per_datagram (or an explicit 0) continues to emit EXACTLY one line per datagram`" ⇒ split: absent ⇒ one-per-datagram; explicit-0 ⇒ boot-reject. [EDIT — correctness; this is the WRONG-after-fix sentence]
10. **graphite Strict-rejects NOTE** (`BEHAVIOR_CONTRACT.md:821`) — "`the landed phase-50 dog_statsd parse does NOT enforce it … recorded as a deferred candidate, NOT fixed by this row`" ⇒ flip to "`… now enforced by phase 58 (ADR-0276)`". [EDIT — gap-naming site #1]
11. **the sink-consumption summary** (`BEHAVIOR_CONTRACT.md:834`) — "`the dog_statsd sink's address/prefix/max_bytes_per_datagram are ALL consumed (phase 49 + phase 50) EXCEPT the explicit-max_bytes_per_datagram: 0 PGV reject, a recorded deferred parity candidate (phase 57 §2)`" ⇒ drop the EXCEPT clause (now consumed, including the explicit-zero reject, phase 58). [EDIT — gap-naming site #2]

**Historical — do NOT edit:** ADR-0275 §Consequences names the gap as it stood at phase 57 (a point-in-time record); leave it. SPEC-57 §2/§11 A4b likewise (historical). The phase-58 ADR-0276 will reference them as the gap's provenance.

**ROADMAP / STATE / DECISIONS:**
12. **ROADMAP** — row 58 `in-progress` at this BRAINSTORM (§Schema); the family prose gains a "phase 58 CHARTERED and BRAINSTORMED" sentence; the LIVE deferred-candidates sentence rolls the dog_statsd candidate OUT at the phase-58 IMPL (NOT now — re-run the sentinel check-(2) grep after that edit, `reference_sentinel_deferred_sentence_live_vs_historical`). [BRAINSTORM: row + prose; IMPL: deferred-list roll]
13. **STATE.md** — active-phase header flips to phase 58 BRAINSTORM (this stage). [EDIT]
14. **DECISIONS.md** — ADR-0276 §Context drafts at the SPEC, §Decision/§Consequences at the IMPL (ADR-0044). NOT at this BRAINSTORM. [SPEC/IMPL]

SPEC pins **D-DZ-DOCSHAPE** (this full edit-site roster, RE-DERIVED) + **D-DZ-TESTROWS** (the convert-not-add test shape).

---

## 7. Anticipated ADRs — 1 at the phase-58 IMPL: ADR-0276 (the dog_statsd explicit-zero parity fix)

ADR-0276 (the dog_statsd explicit-`max_bytes_per_datagram: 0` PGV-parity reject — a parse-arm-only robustness fix, no seam ADR since it mirrors the ADR-0275 graphite arm VERBATIM). §Context drafted at the SPEC (the gap's provenance: SPEC-57 §2/§11 A4b + ADR-0275 §Consequences), §Decision/§Consequences land at the IMPL per ADR-0044. Next-free after: **ADR-0277**.

---

## 8. Deferred items

- **`OTLP-metrics` stats sink** — an OTLP metrics `stats_sinks[]` consumer; carries forward (the largest remaining Observability sink follow-on).
- **Tracing `custom_tags`** — per-span custom tags on the phase-46 tracing engine; carries forward.
- **Tracing `spawn_upstream_span`** — a distinct upstream span; carries forward.
- **Tracing `http_service`** — an HTTP tracing backend; carries forward.
- **Tracing force-trace** — `x-envoy-force-trace` header handling; carries forward.
- **`stats_flush_on_admin`** — still rejected (`bootstrap.go:497-499`); orthogonal, carries forward.

After row 58 the dog_statsd explicit-zero candidate rolls OUT of the LIVE deferred sentence (at the IMPL); OTLP + the tracing quartet remain ⇒ the sentinel check-(2) STILL prints ⇒ the loop continues.

---

## 9. Cross-references against prior phases' deferred-items lists — pickup

Phase 58 PICKS UP the `dog_statsd` explicit-`max_bytes_per_datagram: 0` parity fix — recorded by SPEC-57 §2 as a "`NEW deferred candidate`" and carried into the ROADMAP Observability family's LIVE deferred-candidates sentence at the phase-57 IMPL. After phase 58 the remaining deferred candidates are: `OTLP-metrics` sink + the four tracing sub-features + `stats_flush_on_admin`. The family STAYS OPEN. **Sentinel maintenance (at the IMPL):** after rolling dog_statsd OUT of the deferred sentence, re-run the check-(2) grep — require EXACTLY ONE live "candidates:" match with the intended content (`reference_sentinel_deferred_sentence_live_vs_historical`).

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution

- **D-DZ-REJECTMSG** — the reference's exact boot-reject wording for a dog_statsd explicit-`max_bytes_per_datagram: 0` (anticipated `Proto constraint validation failed (DogStatsdSinkValidationError.MaxBytesPerDatagram: value must be greater than 0)`; ONE fresh-container probe arm, `reference_probe_fresh_container_per_arm`, against `envoyproxy/envoy:contrib-v1.37.2`, `reference_envoy_contrib_image_tagging`). Confirms BEHAVIOR parity (both reject); envoy-go's OWN message is the graphite-mirrored `dog_statsd max_bytes_per_datagram must be greater than 0` (§2.3). Also re-derive the `stats.pb.validate.go` line of the PGV `gt: 0` rule (SPEC-57 §11 A4b cited `1144-1150` — re-verify). §2.2/§2.3.
- **D-DZ-FIXTURE** — differential-provable? Anticipated NO new fixture (boot-rejects are parse-time; the phase-49/50/57 reject rosters were unit-test + fuzz-seed proven, NOT fixtures). §2.5.
- **D-DZ-TESTROWS** — the CONVERT-not-ADD test shape: delete `TestDogStatsdSink_AcceptMaxBytesPerDatagramZero`, add an `explicit_max_bytes_per_datagram_zero` row to `TestDogStatsdSink_Rejects`; KEEP `…Absent`. Prove the assertion is LIVE (a deliberate break: remove the new arm, confirm the new reject row FAILS — `reference_differential_break_protocol_count1` teaches `-count=1` to defeat go-test caching on a break). §2.4/§6.
- **D-DZ-FUZZSEED** — confirm a seed to the EXISTING `FuzzDogStatsdSinkConfigParse` (NOT a new fuzzer); fuzzer count STAYS 54 (`reference_fuzzer_count_docs_drift` — reconcile before AND after). §2.6.
- **D-DZ-DOCSHAPE** — the full edit-site roster (§6), RE-DERIVED against source at the SPEC (the three `bootstrap.go` comment sites + the four `BEHAVIOR_CONTRACT.md` sites); the two BEHAVIOR_CONTRACT gap-NAMING sites (785 correctness, 787 correctness, 821 + 834 gap-naming) PLUS the ADR-0275 §Consequences historical mention (do NOT edit). §6.

---

## 11. Prior-phase lessons applied

- **`feedback_brief_citations_not_evidence`** — EVERY `file:line` in this BRAINSTORM (the `bootstrap.go` arm/comment sites, the `bootstrap_test.go` test rows, the `BEHAVIOR_CONTRACT.md` edit sites, the `stats.pb.validate.go` PGV line) is to be RE-DERIVED from source at the SPEC, never verified against this document. (This session re-derived them live against master `6d3a4330`; the SPEC re-derives again.)
- **`reference_fuzzer_count_docs_drift`** — a SEED, not a fuzzer; reconcile the documented running total (54) against actual `^func Fuzz` before AND after — the count must NOT move. §2.6.
- **`reference_differential_break_protocol_count1`** + **`reference_deliberate_break_wrong_assertion`** — when proving the new reject arm is live, use `-count=1` (go-test caches a stale PASS after a deliberate break) and confirm the CORRECT assertion fires (the converted reject row, not an earlier abort). §2.4.
- **`reference_probe_fresh_container_per_arm`** + **`reference_envoy_contrib_image_tagging`** — the ONE SPEC probe arm runs on a FRESH container against `envoyproxy/envoy:contrib-v1.37.2`. §2.3.
- **`reference_sentinel_deferred_sentence_live_vs_historical`** — after the IMPL rolls dog_statsd OUT of the deferred sentence, re-run the check-(2) grep; EXACTLY ONE live "candidates:" match, correct content. §9.
- **`reference_strict_reject_sibling_typeurl_gap`** / **ADR-0080** — the new reject arm carries a DISTINCT message substring (`dog_statsd …` vs graphite's `graphite_statsd …`) so a future silent divergence surfaces. §2.2.
- **Phase-57 final-review deferred fold-ins (F-1/F-2/F-3, from the router):** this row does NOT touch `udp.go` or the `0101` driver, so F-1 (0101 driver counter-tag snapshot), F-2 (`udpWriter` doc names two sinks — now three), and F-3 (`0101` `HostGatewayIP` note) are OUT OF THIS ROW's blast radius. They remain fold-in candidates for the next `udp.go`/statssink-driver-touching row (the OTLP-metrics sink, most likely). Recorded here so they are not lost. §8.

---

## 12. Section closeout

**Settled:** subject (dog_statsd explicit-zero parity fix, SELF-PICKED per the standing directive over three declined alternatives, §2.1); fix shape (mirror the phase-57 graphite arm VERBATIM modulo the sink name, §2.2); message posture (envoy-go's OWN graphite-mirrored substring, behavior-parity-not-text-parity, §2.3); the NON-additive test edit (convert `…AcceptMaxBytesPerDatagramZero` → a reject row, §2.4/§6); fixture posture (NO new fixture, §2.5); fuzz posture (a SEED, no new fuzzer, §2.6); envelope (SINGLE FLAT ROW, the smallest yet — ADR-0276, §1.4). The ONLY novel production code is a four-line reject arm; the ROW'S RISK lives in the doc + non-additive-test reconciliation (§6), which the SPEC enumerates exhaustively.

**Anticipated moves at the phase-58 IMPL (docs-only now):** the reject arm in `parseDogStatsdSinkConfig` + three `bootstrap.go` doc-comment edits + the CONVERT of `TestDogStatsdSink_AcceptMaxBytesPerDatagramZero` into a `TestDogStatsdSink_Rejects` row (KEEP `…Absent`) + an explicit-0 SEED in `FuzzDogStatsdSinkConfigParse` + four `BEHAVIOR_CONTRACT.md` edits + ADR-0276 + the ROADMAP deferred-list roll. Counts: stat surface **1201 (+0)** · fixtures **103 (+0)** · fuzzers **54 (+0, seed only)** · BackendKind **38 (+0)** · DECISIONS tail **ADR-0276** (next-free **ADR-0277**) · new Go packages **0** · new go.mod modules **0**.

**Counts UNCHANGED at this BRAINSTORM (docs-only; re-verified against master tip `6d3a4330`):** stat surface **1201** · fixtures **103** · fuzzers **54** · BackendKind **38** · DECISIONS tail **ADR-0275** (next-free **ADR-0276**). Row 58 registers `in-progress` at this BRAINSTORM commit per the §Schema invariant.

**Next → the phase-58 SPEC** (the ONE live-probe D-DZ-REJECTMSG arm against `envoyproxy/envoy:contrib-v1.37.2`; re-derive every §6 edit site + the `stats.pb.validate.go` PGV line; draft ADR-0276 §Context).
