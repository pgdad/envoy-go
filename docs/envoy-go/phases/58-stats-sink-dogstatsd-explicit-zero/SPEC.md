# SPEC 58 — `dog_statsd` explicit-`max_bytes_per_datagram: 0` parity fix (`envoy.config.metrics.v3.DogStatsdSink` → a ONE-arm parse-reject mirroring the phase-57 graphite arm VERBATIM modulo the sink name; closes the phase-50/57 recorded reference-parity gap; NO new fixture / NO new fuzzer / +0 stats / +0 packages / +0 modules — a SINGLE FLAT ROW, ADR-0276)

> **Stage:** SPEC (lifecycle-state 1 → 2). Docs-only; NO production `.go` changes at a SPEC. Fresh worktree `.worktrees/phase-58-spec`, branch `phase-58-stats-sink-dogstatsd-explicit-zero-spec`, per `feedback_git_worktrees`. Row 58 STAYS `in-progress` (flips `done` only at the phase-58 IMPL six-gate, ADR-0106 — the SOLE leg).
>
> **ANCHORS ADR-0276 §Context DRAFT** (§Decision/§Consequences land at the IMPL per ADR-0044; DECISIONS tail STAYS **ADR-0275** at this SPEC).
>
> **Baselines re-verified against master tip `6628e17f` (the phase-58 BRAINSTORM squash):** stat surface **1201** · fixtures **103** (tail `0101-stats-sink-graphite`) · fuzzers **54** (verified `54` actual `^func Fuzz`, tail `FuzzGraphiteStatsdSinkConfigParse`) · BackendKind tail **38** (`H2GoawayResponder`) · DECISIONS tail **ADR-0275** (next-free **ADR-0276**) · new Go packages **0** · new go.mod modules **0**. Counts UNCHANGED at this SPEC (docs-only). Every `file:line` below was RE-DERIVED from source this session (`feedback_brief_citations_not_evidence`) — the roster is §12.

---

## 1. Purpose / Mission

Pin the behavior of a **single strict-reject arm** added to `parseDogStatsdSinkConfig` (`internal/bootstrap/bootstrap.go:712`) so that an **explicit** `max_bytes_per_datagram: 0` on a `dog_statsd` `stats_sinks[]` entry boot-rejects with envoy-go's own message `bootstrap: stats_sinks[%d]: dog_statsd max_bytes_per_datagram must be greater than 0` — mirroring the phase-57 graphite arm (`bootstrap.go:751`) VERBATIM modulo the sink name. This closes the reference-parity gap recorded by SPEC-57 §2: the reference's `DogStatsdSink` proto carries the IDENTICAL PGV `gt: 0` rule on `max_bytes_per_datagram` (`stats.pb.validate.go:1144-1157`, re-derived §11) that graphite's phase-57 arm enforces, but the landed phase-50 `parseDogStatsdSinkConfig` consumes an explicit 0 UNCHECKED (`dsd.GetMaxBytesPerDatagram().GetValue()` straight into the config, `bootstrap.go:724`). The fix is possible — and an explicit 0 is distinguishable from a legitimate ABSENT field — because `DogStatsdSink.max_bytes_per_datagram` is a `*wrapperspb.UInt64Value` (nil ⇔ absent; non-nil-with-value-0 ⇔ explicit 0), the same wrapper property phase 57 relied on for graphite.

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The BRAINSTORM anticipated the reference reject; the SPEC-time probe (§11, ONE arm + two isolating controls, fresh container per arm per `reference_probe_fresh_container_per_arm` against `envoyproxy/envoy:contrib-v1.37.2`) CONFIRMS it:

- **AMEND-DZ-REJECT-CONFIRMED (D-DZ-REJECTMSG).** The reference boot-rejects an explicit `max_bytes_per_datagram: 0` on a `DogStatsdSink` with `Proto constraint validation failed (DogStatsdSinkValidationError.MaxBytesPerDatagram: value must be greater than 0)` (exit 1) — EXACTLY graphite's A4b wording (SPEC-57 §11) with `Graphite`→`Dog`. Two controls ISOLATE the reject to the explicit zero: an ABSENT `max_bytes_per_datagram` validates OK (exit 0), and an explicit `512` validates OK (exit 0). ⇒ envoy-go mirrors the BEHAVIOR (both processes reject) with its OWN message substring (`dog_statsd max_bytes_per_datagram must be greater than 0`), NOT the reference's PGV text — the phase-48/49/57 own-message posture (envoy-go's bootstrap rejects are hand-authored, never copies of the reference's PGV wording). The PGV rule guards on `wrapper != nil` (`stats.pb.validate.go:1144`), so an absent field skips the check — which is precisely why envoy-go's arm is `w != nil && w.GetValue() == 0` and the absent-accept path is untouched.

### 1.2 ADR continuity + D-disposition at SPEC commit

ADR-0276 §Context is DRAFTED here (§13); §Decision/§Consequences at the IMPL (ADR-0044). All five BRAINSTORM D-DZ-* questions are DISPOSED at this SPEC: **D-DZ-REJECTMSG** PINNED (§11); **D-DZ-FIXTURE** DECIDED NO new fixture (§8); **D-DZ-TESTROWS** DECIDED convert-not-add (§10); **D-DZ-FUZZSEED** DECIDED a seed to the existing fuzzer, count stays 54 (§6); **D-DZ-DOCSHAPE** the full RE-DERIVED edit-site roster (§12). No PLAN-time empirical question remains; the PLAN is a mechanical decomposition.

---

## 2. Non-purposes (deferred; per BRAINSTORM §1.2 + §8)

No change to the graphite sink (its arm already exists — phase 57). No change to the plain `statsd` sink (its `StatsdSink` proto has NO `max_bytes_per_datagram` field — batching is a `DogStatsdSink`-only knob, confirmed at phase 50). No new sink, transport, stat, fixture, fuzzer, package, or module. No change to the dog_statsd EMIT path (delta/tag/UDP/batching) — this row touches ONLY the parse-time validation. The inherited hostname-accepting DEPARTURE (statsd/dog_statsd `net.ResolveUDPAddr` accepts hostnames the reference rejects) is orthogonal and unchanged. Untouched Observability deferred candidates: OTLP-metrics sink + tracing `custom_tags`/`spawn_upstream_span`/`http_service`/force-trace (they carry forward; the dog_statsd explicit-zero candidate rolls OUT of the ROADMAP live deferred sentence at the IMPL, §14). `stats_flush_on_admin` stays rejected (`bootstrap.go:497-499`); orthogonal.

---

## 3. The change — a single reject arm in `parseDogStatsdSinkConfig` (ADR-0276)

### 3.0 Split disposition — a SINGLE FLAT ROW; the ADR-0045 escape-valve UNCONSUMED

The smallest row the project has chartered: ~5-7 tasks (§10), margin large under the ADR-0045 `~15` ceiling. There is no second subsystem to strand; the graphite arm it mirrors landed at phase 57. Escape valve UNCONSUMED, re-armable only if the PLAN's task count surprises upward (it will not).

### 3.1 The reject arm (`internal/bootstrap/bootstrap.go`)

Added to `parseDogStatsdSinkConfig` **before** the `append(result.DogStatsdSinkConfigs, …)` (currently `bootstrap.go:721`), so a rejected config never reaches the slice — the exact placement of the graphite arm (`bootstrap.go:750`, between the address parse and the append):

```go
if w := dsd.GetMaxBytesPerDatagram(); w != nil && w.GetValue() == 0 {
    return fmt.Errorf("bootstrap: stats_sinks[%d]: dog_statsd max_bytes_per_datagram must be greater than 0", idx)
}
```

- The message substring `dog_statsd max_bytes_per_datagram must be greater than 0` is DISTINCT from graphite's `graphite_statsd max_bytes_per_datagram must be greater than 0` (ADR-0080 anti-silent-divergence — each reject arm carries its own distinguishable substring; the two differ only in the sink-name prefix, which is exactly the distinguisher).
- The `w != nil` guard preserves the absent-accept path VERBATIM: an absent field (nil wrapper) skips the arm and still parses to `MaxBytesPerDatagram == 0` = one-line-per-datagram (`bootstrap.go:724` unchanged, now reachable only for nil-or-non-zero wrappers). This mirrors the reference PGV's own `wrapper != nil` guard (`stats.pb.validate.go:1144`).
- For a `uint64`, `w.GetValue() == 0` and the reference's `<= 0` are equivalent (no negative values). envoy-go uses `== 0` to match the graphite arm byte-for-byte modulo the sink name.

### 3.2 Doc-comment reconciliation (the same file)

Three `bootstrap.go` doc comments currently assert the OLD (pre-fix) semantics and become stale; the IMPL corrects them in passing (full roster §12):
- the `DogStatsdSinkConfig` struct doc (`bootstrap.go:326`, the field comment "`0 (absent or explicit) means "one metric per datagram"`") — flip to absent-only, mirroring the graphite field comment (`bootstrap.go:342`+, "`0 (absent only — explicit 0 is parse-rejected)`");
- the `parseDogStatsdSinkConfig` func doc (`bootstrap.go:702-711`, "`0 (absent or explicit) means one metric per datagram (phase-49 behavior, UNCHANGED)`") — split absent (accept) from explicit-0 (reject);
- the `parseGraphiteStatsdSinkConfig` NOTE (`bootstrap.go:738-740`, "`the landed dog_statsd parse does NOT enforce its identical PGV rule … NOT fixed here`") — now FALSE; update to name phase 58 (ADR-0276).

### 3.3 Byte-stability — no behavior change beyond the new reject

A bootstrap that does NOT carry an explicit `max_bytes_per_datagram: 0` on a dog_statsd sink is parsed byte-identically to today. The reject arm fires only on the previously-accepted-now-rejected input. The full **103-dir** differential (none of which configures an explicit-0 dog_statsd cap) is the regression anchor and stays GREEN.

---

## 4. Framework primitives — 0 new packages, 0 new go.mod modules

No new interface/type/seam. `parseDogStatsdSinkConfig` gains four lines; every referenced symbol (`dsd.GetMaxBytesPerDatagram`, `*wrapperspb.UInt64Value`, `fmt.Errorf`) is already imported and consumed by the existing `bootstrap.go:724`. `go mod tidy -diff` anticipated EMPTY.

---

## 5. Proto-field roster (the `DogStatsdSink` surface after 58)

| Field | Type | Disposition |
|---|---|---|
| `dog_statsd_specifier.address` (oneof, sole arm) | `*corev3.Address` | CONSUMED (phase 49) — UDP endpoint via `parseUDPSinkAddressAndPrefix`; missing ⇒ reject (REFERENCE-PARITY, unchanged) |
| `prefix` (field 2) | `string` | CONSUMED (phase 49) — default `"envoy"` when empty |
| `max_bytes_per_datagram` (field 3) | `*wrapperspb.UInt64Value` | CONSUMED (phase 50 batching) — **NOW** explicit 0 ⇒ reject (REFERENCE-PARITY, phase 58, §11); nil ⇒ one-line-per-datagram (unchanged) |

After 58 the `DogStatsdSink` surface is FULLY consumed with the same explicit-zero reject as graphite — the two sinks reach parse-time parity.

---

## 6. PARSE-REJECT roster + fuzzer

**Reject arms (ADR-0080, each a DISTINCT message substring), `parseDogStatsdSinkConfig`:** (i) missing `dog_statsd_specifier` / nil `socket_address` — via `parseUDPSinkAddressAndPrefix` (existing, phase 49); (ii) unknown/sibling sink TypeURL — the shared four-sink dispatch default arm (existing, extended at phase 57); (iii) **NEW** explicit `max_bytes_per_datagram: 0` — non-nil wrapper with value 0 (REFERENCE-PARITY, §11; the wrapper type makes absent-vs-explicit-0 distinguishable). NOT reject arms: an absent `max_bytes_per_datagram` (accept ⇒ 0); a hostname `address` (the inherited, documented phase-48/49 DEPARTURE — envoy-go's `ResolveUDPAddr` accepts hostnames).

**Fuzzer (D-DZ-FUZZSEED).** NO new fuzzer. Add ONE explicit-`max_bytes_per_datagram: 0` SEED to the EXISTING `FuzzDogStatsdSinkConfigParse` (`internal/bootstrap/dogstatsd_fuzz_test.go`), mirroring the graphite fuzzer's own explicit-0 seed (`graphite_fuzz_test.go:63-70`). The running total is reconciled: actual `^func Fuzz` == documented == **54** BEFORE; it must be **54** AFTER (`reference_fuzzer_count_docs_drift` — re-run the count post-edit; a seed never changes it).

---

## 7. Stat surface — +0 (1201 → 1201)

A parse-reject registers no stat. The dog_statsd sink's registration path is untouched. No `NoNewStat` guard is implicated. Stat surface **1201 → 1201 (+0)**.

---

## 8. Differential fixture taxonomy — +0 (D-DZ-FIXTURE: NO new fixture)

A boot-reject is PARSE-TIME: a rejected bootstrap never reaches the differential runner (which needs a bootable config on BOTH sides). Every prior stats-sink reject arm (phase 48/49/50/57 rosters) was proven by SUBJECT unit tests + fuzz seeds, NOT fixture dirs (`reference_differential_fixture_dispatch_constraint`). The reference-reject is confirmed by the D-DZ-REJECTMSG probe (§11), not a differential. Fixtures **103 → 103 (+0)**. No new BackendKind (tail stays **38**).

---

## 9. Behavior-contract delta (the phase-58 bundle; ADR-0276 atomic landing)

Four `BEHAVIOR_CONTRACT.md` edits land WITH the IMPL (full roster §12): the dog_statsd Strict-rejects (`:785`, add the arm), the dog_statsd batching text (`:787`, split absent from explicit-0), the graphite NOTE (`:821`, flip "NOT fixed" → "fixed at phase 58"), and the consumption summary (`:834`, drop the "EXCEPT the explicit-`max_bytes_per_datagram: 0`" clause). ADR-0275 §Consequences + SPEC-57 §2/§11 A4b name the gap HISTORICALLY — leave them.

---

## 10. Test plan + per-task structure (~5-7 tasks; PLAN decomposes)

**D-DZ-TESTROWS — the CONVERT-not-ADD shape (the row's load-bearing edit).** `TestDogStatsdSink_AcceptMaxBytesPerDatagramZero` (`bootstrap_test.go:2550`) currently asserts an explicit `max_bytes_per_datagram: 0` is ACCEPTED (`len==1`, `MaxBytesPerDatagram==0`). After the fix that input REJECTS, so this row is NOT purely additive: **delete** that accept-test and **add** an `explicit_max_bytes_per_datagram_zero` row to `TestDogStatsdSink_Rejects` (`bootstrap_test.go:2458`) with `errSubs: []string{"bootstrap:", "dog_statsd max_bytes_per_datagram must be greater than 0"}` — the graphite reject-row shape (`bootstrap_test.go:2890-2899`). **KEEP** `TestDogStatsdSink_AcceptMaxBytesPerDatagramAbsent` (`bootstrap_test.go:2528`): absent ⇒ accept ⇒ 0 — the wrapper-type asymmetry (absent-accept vs explicit-0-reject) is the whole point and must remain a positive, breakable assertion. **Liveness:** prove the new reject arm is LIVE with a `-count=1` deliberate break (remove the arm; the new reject row must FAIL — `reference_differential_break_protocol_count1` defeats go-test caching; `reference_deliberate_break_wrong_assertion` — confirm the CONVERTED row fires, not an earlier abort).

**Anticipated tasks (PLAN pins exact):** T1 baselines/count reconciliation · T2 the reject arm + the three `bootstrap.go` doc-comment fixes · T3 the test convert (delete accept, add reject row, keep absent) + the `-count=1` liveness break · T4 the `FuzzDogStatsdSinkConfigParse` seed (count stays 54) · T5 the four `BEHAVIOR_CONTRACT.md` edits · T6 ADR-0276 body + STATE/ROADMAP (row-58-`done`, deferred-list roll, sentinel check-(2) re-run) + router roll · (optional T7 fold, e.g. the stale `:785` "three sinks" parenthetical, §12). ~5-7 tasks.

---

## 11. SPEC-time empirical-pin block (D-DZ-REJECTMSG — executed IN-SESSION 2026-07-12)

Three fresh-container arms via `docker run --rm … envoyproxy/envoy:contrib-v1.37.2 envoy --mode validate -c …` (each `--rm` is a fresh container per `reference_probe_fresh_container_per_arm`; identical base bootstrap — admin + a `dog_statsd` `stats_sinks[]` entry with a literal-IP UDP `address` + `prefix: envoy` + empty `static_resources` — varying ONLY the `max_bytes_per_datagram` field):

| Arm | `max_bytes_per_datagram` | Result | Verbatim tail |
|---|---|---|---|
| **A1** | explicit `0` | **REJECT, exit 1** | `: Proto constraint validation failed (DogStatsdSinkValidationError.MaxBytesPerDatagram: value must be greater than 0)` |
| **A2** (control) | ABSENT | **OK, exit 0** | `configuration '/cfg/A2_absent.yaml' OK` |
| **A3** (control) | explicit `512` | **OK, exit 0** | `configuration '/cfg/A3_valid512.yaml' OK` |

**Disposition.** A1 confirms the reference boot-rejects the explicit zero; A2/A3 ISOLATE the reject to the explicit-0 value (an absent field and a valid cap both pass). The reference wording is graphite's A4b (`GraphiteStatsdSinkValidationError.MaxBytesPerDatagram: value must be greater than 0`, SPEC-57 §11) with `Graphite`→`Dog`. **PGV source re-derived** (`feedback_brief_citations_not_evidence`): `github.com/envoyproxy/go-control-plane/envoy@v1.32.4/config/metrics/v3/stats.pb.validate.go:1144-1157` —
```go
if wrapper := m.GetMaxBytesPerDatagram(); wrapper != nil {
    if wrapper.GetValue() <= 0 {
        err := DogStatsdSinkValidationError{field: "MaxBytesPerDatagram", reason: "value must be greater than 0"}
        …
    }
}
```
SPEC-57 §11 A4b's `stats.pb.validate.go:1144-1150` cite is ACCURATE (the message literal is at `:1149`). envoy-go emits its OWN substring, not this PGV text (§1.1).

---

## 12. Edit-site roster (D-DZ-DOCSHAPE — RE-DERIVED against master `6628e17f`)

Because the production change is four lines, the row's RISK is the doc + non-additive-test reconciliation. Every site RE-DERIVED this session (the IMPL re-derives again):

**Production (`internal/bootstrap/bootstrap.go`):**
1. **The reject arm** — `parseDogStatsdSinkConfig`, before the append `:721`. [ADD, §3.1]
2. **`DogStatsdSinkConfig` struct doc** — `:326` (field comment "`0 (absent or explicit) means "one metric per datagram"`"). Flip to absent-only, mirroring the graphite field comment (`:342`+). [EDIT]
3. **`parseDogStatsdSinkConfig` func doc** — `:702-711` ("`0 (absent or explicit) …UNCHANGED`"). Split absent-accept from explicit-0-reject; add the new-arm sentence. [EDIT]
4. **`parseGraphiteStatsdSinkConfig` NOTE** — `:738-740` ("`… deferred (SPEC-57 §2), NOT fixed here`"). Update to "`… enforced by phase 58 (ADR-0276)`". [EDIT — historical accuracy]

**Test (`internal/bootstrap/bootstrap_test.go`):**
5. **`TestDogStatsdSink_AcceptMaxBytesPerDatagramZero`** — `:2550`. CONVERT: delete; add an `explicit_max_bytes_per_datagram_zero` reject row to `TestDogStatsdSink_Rejects` (`:2458`), `errSubs: []string{"bootstrap:", "dog_statsd max_bytes_per_datagram must be greater than 0"}`. [CONVERT — §10]
6. **`TestDogStatsdSink_AcceptMaxBytesPerDatagramAbsent`** — `:2528`. KEEP untouched. [KEEP]
7. **`FuzzDogStatsdSinkConfigParse` seed** — `dogstatsd_fuzz_test.go`. ADD an explicit-0 seed (graphite precedent `graphite_fuzz_test.go:63-70`). [ADD — no new fuzzer]

**BEHAVIOR_CONTRACT (`docs/envoy-go/BEHAVIOR_CONTRACT.md`):**
8. **dog_statsd Strict-rejects** — `:785`. ADD the explicit-`max_bytes_per_datagram: 0` reject arm. **OPTIONAL in-passing fold:** this line's "`(naming all three supported sinks)`" parenthetical is a latent phase-57 staleness — the sibling-reject message now names FOUR sinks; the IMPL may correct it while editing this line (a phase-57-precedent in-passing doc fix). [EDIT — correctness]
9. **dog_statsd batching** — `:787`. Split "`An ABSENT max_bytes_per_datagram (or an explicit 0) …`" into absent ⇒ one-per-datagram, explicit-0 ⇒ boot-reject. [EDIT — correctness; the WRONG-after-fix sentence]
10. **graphite Strict-rejects NOTE** — `:821` ("`… NOT fixed by this row`"). Flip to "`… now enforced by phase 58 (ADR-0276)`". [EDIT — gap-naming site #1]
11. **consumption summary** — `:834` ("`… ALL consumed (phase 49 + phase 50) EXCEPT the explicit-max_bytes_per_datagram: 0 PGV reject …`"). Drop the EXCEPT clause (now consumed incl. the explicit-zero reject, phase 58). [EDIT — gap-naming site #2]

**Historical — do NOT edit:** ADR-0275 §Consequences; SPEC-57 §2/§11 A4b (point-in-time records; ADR-0276 references them as the gap's provenance).

**ROADMAP / STATE / DECISIONS (at the IMPL):** row 58 `in-progress`→`done`; the family LIVE deferred sentence rolls the dog_statsd candidate OUT, then re-run the sentinel check-(2) grep (EXACTLY ONE live "candidates:" match — `reference_sentinel_deferred_sentence_live_vs_historical`); ADR-0276 §Decision/§Consequences land IN-PLACE (ADR-0044); STATE active-phase → IMPL done.

---

## 13. ADR continuity — the ADR-0276 §Context DRAFT (anchored here; full entry at the phase-58 IMPL)

**ADR-0276 §Context (draft).** Phase 57 landed the `graphite_statsd` stats sink (ADR-0275), whose parse arm enforces the PGV `gt: 0` rule on `max_bytes_per_datagram` by rejecting an explicit `0` (possible because the field is a `*wrapperspb.UInt64Value`, so nil-absent is distinguishable from explicit-zero). At that landing SPEC-57 §2 RECORDED a pre-existing parity gap: the reference's sibling `DogStatsdSink` proto carries the IDENTICAL PGV `gt: 0` rule (`stats.pb.validate.go:1144-1157`), but the landed phase-50 `parseDogStatsdSinkConfig` consumes an explicit `0` UNCHECKED (`bootstrap.go:724`) — a silent divergence where envoy-go ACCEPTS a config the reference boot-rejects. A SPEC-58 live probe against `envoyproxy/envoy:contrib-v1.37.2` (§11, fresh container per arm) confirmed the reference reject (`Proto constraint validation failed (DogStatsdSinkValidationError.MaxBytesPerDatagram: value must be greater than 0)`, exit 1) with two isolating controls (absent and `512` both validate OK). Phase 58 closes the gap with a single reject arm in `parseDogStatsdSinkConfig` mirroring the graphite arm VERBATIM modulo the sink name (envoy-go's own message substring `dog_statsd max_bytes_per_datagram must be greater than 0`, ADR-0080-distinct from graphite's), plus doc reconciliation across three `bootstrap.go` comments and four `BEHAVIOR_CONTRACT.md` sites, the CONVERSION of the pre-existing `TestDogStatsdSink_AcceptMaxBytesPerDatagramZero` accept-test into a `TestDogStatsdSink_Rejects` arm (the absent-accept sibling test preserved), and a seed to the existing `FuzzDogStatsdSinkConfigParse`. A SINGLE FLAT ROW (ADR-0045 escape-valve unconsumed); no seam ADR (the graphite reject shape is reused). +0 stats/fixtures/fuzzers/packages/modules. §Decision/§Consequences land at the phase-58 IMPL per ADR-0044. ANCHORS ADR-0276.

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

**Counts UNCHANGED at this SPEC (docs-only; re-verified against master tip `6628e17f`):** stat surface **1201** · fixtures **103** · fuzzers **54** · BackendKind tail **38** · DECISIONS tail **ADR-0275** (next-free **ADR-0276**) · new Go packages **0** · new go.mod modules **0**.

**Anticipated at the phase-58 IMPL:** the reject arm + 3 `bootstrap.go` doc edits + the test convert + the fuzz seed + 4 `BEHAVIOR_CONTRACT.md` edits + ADR-0276 body + the ROADMAP deferred-list roll (dog_statsd OUT, then re-run sentinel check-(2)); stat surface **1201 (+0)** · fixtures **103 (+0)** · fuzzers **54 (+0)** · BackendKind **38 (+0)** · DECISIONS tail **ADR-0276** (next-free ADR-0277) · new packages **0** · new modules **0**. Row 58 flips `in-progress` → `done` at the IMPL six-gate (the SOLE leg — NO parent rollup, ADR-0106). The Observability family STAYS OPEN (OTLP-metrics sink + the tracing quartet remain).

**Next → the phase-58 PLAN** (a mechanical ~5-7-task decomposition; no empirical questions remain).
