# Phase 102 — `stats-flush-interval-bounds` — SPEC

**Stage:** SPEC (lifecycle **1 -> 2**). Worktree `/home/esa/git/envoy-go-wt-phase-102-spec` off master
`fbd1b2a3`, branch `wt-phase-102-spec`. **Governs:** `BRAINSTORM.md` (613 lines), read as evidence and
re-derived before any of it was trusted. Written under the 2026-07-12 standing directive: **no human
consulted.**

**The decision in one paragraph.** `parseStatsSinks` reads `stats_flush_interval` from the Duration's
**fields** (`GetSeconds()`, `GetNanos()`), never from the saturating `AsDuration`, and applies the reference's
three checks **in the reference's order**:
1. a negative field gives **NEG**, `Invalid duration: Expected positive duration`;
2. seconds above `9223372035` or nanos above `999999999` give **OOR**, `Invalid duration: Duration out-of-range`;
3. a total outside `[1 ms, 300 s)` gives **RANGE**, `value must be inside range [1ms, 5m0s)`.

Absent still means **5 s**. Every message carries the `bootstrap: stats_flush_interval: ` prefix, then the
reference's phrase **byte for byte**, so a boot-reject fixture can share the phrase on both sides. The check is
**unconditional**: it does not depend on `stats_sinks`. The production edit is `bootstrap.go` **`9 2`** in code
(**`14 5`** with the two falsified comments reconciled), by `git diff --numstat`, built at this tip by a
separate agent (P2, Appendix A). Its predecessor's P1 floor was `7 2` with one message. The reference was
re-measured at this tip by digest:
- **84** validate runs: 14 values × {no sink, statsd sink} × 3 runs, each in a fresh container;
- **20** harness-equivalent boots.

Every class is **stable**, and the **RANGE/OOR boundary sits exactly between `9223372035.999999999s` and
`9223372036s`** (§0.2). P2 agrees with the reference on rc and on class for all 13 values the subject probe drove
(§3.3). **42 new test leaves** are named (Appendices B and C): **29 are RED at the tip**; all 42 are GREEN under
P2. The roster has **nine NC mutants, each scored per arm, none vacuous** (§7). Two boot-reject fixtures are
chartered:
- **`0127-stats-flush-interval-range-boot-reject`**: `0s`, RANGE, port `15127`;
- **`0128-stats-flush-interval-negative-boot-reject`**: `-0.5s`, NEG, port `15128`.

Both are sink-free (§6). The stat surface is **+0 NAMES** (§8.2).

---

## 0. What this stage refuted or sharpened — by execution

Every item comes from something that was run. **R-agent** is this stage's Docker-only reference agent (image by
digest, containers `p102spec-ref-*`, all torn down by name). **S-agent** is its Docker-free subject agent
(throwaway detached worktrees off `fbd1b2a3`, all removed).

### 0.1 🔴 THE REFERENCE HAS *THREE* PHRASES, NOT TWO CLASSES — AND THE SPEC MIRRORS ALL THREE

`BRAINSTORM.md` §0.1 and the router speak of **two** reject classes: range, and `Invalid duration: …`. R-agent
reads two DIFFERENT phrases under the `Invalid duration` prefix:
- `Invalid duration: Expected positive duration` for `-1s`, `-0.5s` and `-0.000000001s`;
- `Invalid duration: Duration out-of-range` for `9223372036s` and `315576000000s`.

Each is followed by `: goo.gle/debug<str|proto|only>`, then 2-4 trailing spaces, then a newline and a
`seconds: …` / `nanos: …` line. A two-message envoy-go (`Invalid duration` for both) would share a substring
with the reference, but it would let NC9's class collapse pass any fixture that pins only `Invalid duration`.
**Three messages, three phrases** (§2).

### 0.2 🔴 THE RANGE/OOR BOUNDARY IS AT `9223372036s` — THE SAME CEILING AS ROW 101

The BRAINSTORM ran nothing between `1000s` and `315576000000s`, so where RANGE stops and OOR starts was
unmeasured. R-agent, 3 runs × B0/B1 each:
- `9223372035s` gives RANGE;
- `9223372035.999999999s` gives RANGE;
- `9223372036s` gives OOR.

That is the reference's generic Duration ceiling, measured for `listener_filters_timeout` at phase 101
(ADR-0323 V3). Its order (NEG and OOR before PGV) is why `-1s` never reads RANGE. P1's single message put
`9223372036s` in RANGE (rc parity, class mismatch). P2's `secs > 9223372035` matches (§3.3).

### 0.3 NEGATIVE NANOS IS THE NEG CLASS ON THE REFERENCE — MEASURED, NOT ONLY SOURCE-READ

`-0.5s` (`{0, -500000000}`) and `-0.000000001s` (`{0, -1}`) read NEG on all 12 validate runs and on both `-0.5s`
boots. The BRAINSTORM never drove a negative-nanos value. P1's rule would have given them RANGE. **Why it
matters:** under NC6 (RANGE checked first), `-1s` stays NEG (seconds ≠ 0 skips the lower bound) while
`-0.5s` turns RANGE. So **`-0.5s`, not `-1s`, is the NEG fixture arm** (§6.2).

### 0.4 🔴 `BRAINSTORM.md` §3.3's "AN NC THAT DELETES ONLY THE LOWER BOUND MUST PANIC OR REJECT, NOT BOOT SILENTLY" IS FALSE — IT DEPENDS ON THE SINK, AND VALIDATE SAYS OK EITHER WAY

S-agent built NC1 (the lower bound deleted, the 300 s ceiling kept) and drove `0s` (`FlushInterval` 0):

| bootstrap | `-mode validate` | real boot |
|---|---|---|
| B1 (statsd sink) | rc 0, `configuration OK` | prints `envoy-go ready`, then **`panic: non-positive interval for NewTicker`** in `statssink.(*Flusher).Start`, exit 2, under 3 s |
| B0 (no sink) | rc 0 | **boots silently**, serves `222`, SIGTERM rc 0 |

**The mutant therefore both panics and boots silently, depending on the sink. Validate never sees it.** The
`> 0` guard's panic-protection role (`BRAINSTORM.md` §2.3) is real, but only on the sink path. NC1's falsifiers
are the unit arms, the binary validate arm, and a sink-free `0127`, which turns RED because the subject BOOTS.
A sink-ful fixture would see `ready`, then a panic, and its harness verdict is UNMEASURED (§6.4).

### 0.5 THE FLOOR MOVED: `7 2` -> `9 2` (code) / `14 5` (with comments), AND THE COMMENT OCCURRENCE SET IS FOUR SITES, NOT TWO

- P2's three messages cost two lines over P1's one (`9 2`).
- Reconciling the two FALSIFIED comments (`statsFlushIntervalDefault`, the `FlushInterval` field) costs `14 5`.
- The census (§4) adds two **half-falsified** comment sites `BRAINSTORM.md` §3.4 did not list. One is the
  `FlushInterval` field's second sentence, *"Parsed-and-stored even when no stats_sinks[] entry exists (inert)"*.
  The other is `parseStatsSinks`'s doc comment, *"stats_flush_interval is ALWAYS stored on
  result.FlushInterval, even with no stats_sinks[] (inert …)"*.
- P2 edits neither, so **`14 5` is still a FLOOR** (`reference_measured_prototype_is_a_lower_bound`, now its
  twenty-third consecutive row).

### 0.6 ONE EXISTING ARM IS NOT BLIND: `TestStatsSink_InertFlushInterval` REDDENS UNDER NC7

`BRAINSTORM.md` §0.5 is right that the existing suite cannot see the row (391 / 0 at the tip; S-agent
re-measured **391 / 0** at `fbd1b2a3`). It is not blind to everything the row could get wrong, though. NC7 gates
the check on `len(bs.GetStatsSinks()) > 0`, which skips the field entirely with no sink. The existing inert arm
then reads `got 5s, want 3s`. **Blind to the repair; not blind to a sink-gated repair** (§7).

### 0.7 THE LINT ZERO IS NOW PROVEN — AND THE FIRST PLANT REPRODUCED METHOD NOTE 88

- `GOTOOLCHAIN=go1.26.2 golangci-lint run ./internal/bootstrap/ ./cmd/envoy-go/` on P2 reads rc 0, empty.
- S-agent's FIRST planted control did not compile (`declared and not used`). Only `typecheck` fired, masking
  every linter.
- The second plant compiles (an ignored `os.Remove` error and a dead `x := 1; x = 2`). It fired `errcheck` and
  `ineffassign` ×2, rc 1, and was then deleted.
- `BRAINSTORM.md` §6's unproven P1 zero is superseded for P2.

### 0.8 HOW THE HARNESS SEES THE REFERENCE'S STDERR — THE SUBSTRING MUST NOT REACH THE TOKEN

`tryStartReferenceProxy` (`test/differential/harness.go`) runs `envoy --config-yaml <bs> --log-level warn
--concurrency 1`. It reads stdout and stderr together through `c.Logs()`, and `runBootRejectFixture` matches
with case-sensitive `strings.Contains`.
- At boot the error comes from `server.cc:453` (validate: `config_validation/server.cc:76`). It is printed
  **twice**, and all three phrases are byte-identical to validate.
- RANGE is preceded by a text dump of the bootstrap proto, so the token is never next to the phrase.
- NEG and OOR are followed by `: goo.gle/debug…`. **A shared substring that ran past `duration` would include
  a token that varies per run.** The two fixture substrings stop at the phrase (§6).

### 0.9 CONFIRMED, NOT REFUTED

- **The reference's `-1s` and `315576000000s` messages** (owed item 7): stable on 3 × 2 validate runs and 2
  boots each, apart from the token and the trailing spaces.
- **Every other class on the BRAINSTORM's 13 values.**
- **B0 = B1 on every value, on both sides.**
- **The tip blindness (391 / 0).**
- **The empty blast radius** of in-range values: every fixture and test literal sits inside `[1ms, 5m)` (§4).
- **The `315576000001s` decoder agreement:** reference `Unable to parse JSON as proto … duration out of range`,
  envoy-go `bootstrap: protojson: … google.protobuf.Duration value out of range`, unchanged under P2.
- **The `bootstrap: ` prefix `FuzzBootstrapLoad` requires**, read in `internal/bootstrap/fuzz_test.go` (no
  panic; any error starts with `bootstrap: `). P2's messages carry it.
- **How `Load` surfaces the error:** unwrapped; the binary prints `load config: <err>` at boot and the bare error
  in validate mode.

---

## 1. Scope, restated as a decision

**In:**
- the validity rule and its three messages (§2);
- the two FALSIFIED and two half-falsified `bootstrap.go` comments, and the half-falsified
  `BEHAVIOR_CONTRACT.md` phrase (§4);
- 42 new test leaves in two new test files (§5);
- fixtures `0127` and `0128` (§6);
- ADR-0324 (§8);
- a delta-only `+0` ledger entry (§8.2).

**Out, stated plainly (what the row does NOT buy):**
- **No byte-parity of whole messages.** The reference's messages carry a run-varying `goo.gle/debug…` token,
  trailing spaces, a proto dump (RANGE) and a `seconds:`/`nanos:` echo (NEG, OOR). envoy-go mirrors the
  **phrase**, not the line.
- **No sub-millisecond cadence parity** (`BRAINSTORM.md` §0.8, unobservable here) and no explanation of
  `0.0005s` at ~1 ms (§0.9). Both are moot for rejected values. The cadence of ACCEPTED `[1ms, 1.5ms]` values
  is unchanged and unpinned.
- **No `stats_flush_on_admin` change** (still strict-rejected, ADR-0080 / ADR-0262).
- **No stat name** (§8.2).
- **No listener-side candidate** of rows 100 or 101, and no fold-in of the exported `Pipeline.Run` overflow
  (ADR-0323 (h)). This row does not touch `Pipeline.Run`.
- **No nanos-overflow reference arm.** `nanos > 999999999` is reachable only from Go-built Durations. YAML and
  protojson normalise or refuse such input, so the reference's handling stays **source-read** (row 101's basis).
  It is not measured. Its OOR class is a SPEC choice consistent with that reading.

---

## 2. The validity rule — EXACT (owed item 1)

Input: `d := bs.GetStatsFlushInterval()` (`*durationpb.Duration`), read through `GetSeconds()` (`int64`) and
`GetNanos()` (`int32`). **Never `AsDuration()`**: it saturates, so `{MaxInt64, 0}` reads
`2562047h47m16.854775807s` and never flushes (`BRAINSTORM.md` §0.7).

| # | condition (evaluated in THIS order) | result | reference (R-agent, this tip) |
|---|---|---|---|
| F1 | `d == nil` | `FlushInterval = 5 s` | absent ⇒ OK, 5001 ms cadence (`BRAINSTORM.md` §2.1) |
| F2 | `secs < 0` **or** `nanos < 0` | **reject** `bootstrap: stats_flush_interval: Invalid duration: Expected positive duration` | `-1s`, `-0.5s`, `-0.000000001s` ⇒ NEG |
| F3 | `secs > 9223372035` **or** `nanos > 999999999` | **reject** `bootstrap: stats_flush_interval: Invalid duration: Duration out-of-range` | `9223372036s`, `315576000000s` ⇒ OOR; nanos half SOURCE-READ (§1) |
| F4 | `secs >= 300` **or** (`secs == 0` **and** `nanos < 1000000`) | **reject** `bootstrap: stats_flush_interval: value must be inside range [1ms, 5m0s)` | `0s`, `0.0005s`, `0.000999999s`, `300s`, `1000s`, `9223372035s`, `9223372035.999999999s` ⇒ RANGE |
| F5 | otherwise | `FlushInterval = time.Duration(secs)*time.Second + time.Duration(nanos)` — EXACT, no truncation | `0.001s`, `299.999999999s` ⇒ OK |

- **F4 also rejects `{0, 0}`.** The `> 0` guard's job (no `time.NewTicker(<=0)` panic) is therefore subsumed:
  every accepted value is ≥ 1 ms, so the flusher can never be built with a non-positive interval.
- **F5 cannot overflow.** F2-F4 leave `0 ≤ secs ≤ 299` and `0 ≤ nanos ≤ 999999999`, so the sum is below 300 s.
- **Every field the arithmetic reads is bounded** (method note 106): seconds by F2/F3/F4, nanos by F2/F3/F4.
  A Go-built `{1, -1}` and `{0, 1000000000}` each have their own arm (§5).
- **Why the order is load-bearing:** the reference runs its Duration validation before PGV. With RANGE first,
  `-0.5s` (seconds 0) would read RANGE and `9223372036s` would read RANGE. NC6 is the falsifier (§7).
- **Mixed sign `{1, -1}`:** not a well-formed protobuf Duration. It is rejected NEG, the same as the
  reference's `nanos < 0` test, source-read. Reachable only from Go.
- **The check is unconditional:** it does not depend on `len(bs.GetStatsSinks())` (`BRAINSTORM.md` §0.4; NC7
  is the falsifier). D-MS-FLUSH-INERT survives for **in-range** values only.
- **Wording:** `bootstrap: stats_flush_interval: ` then the reference phrase verbatim, including its
  capitalisation and the `5m0s)` spelling. No `seconds:` echo is appended; the reference prints that echo
  after the token, on a new line.
- **Accept edges are exact:** `0.001s` → 1 ms, `0.0015s` → 1.5 ms, `299.999999999s` → `299.999999999s`. Nothing
  rounds (there is no truncation rule here, unlike row 101).

---

## 3. The subject, MEASURED (owed items 1 and 3)

### 3.1 The shapes

Each was built in a throwaway worktree off `fbd1b2a3` and removed afterwards.

- **TIP**: master.
- **P2**: Appendix A, the chosen shape.
- **NC1-NC9**: on P2, applied by Appendix D's script, each marked `// NC<n>`. The marker was asserted ≥ 1
  before every run and 0 after reverting, and the file was `cmp`-identical to P2 at the end.

### 3.2 The unit matrix

Selector: `go test -count=1 -v ./internal/bootstrap/... ./cmd/envoy-go/... ./internal/statssink/...`, rc taken
from `PIPESTATUS[0]`.

| shape | `=== RUN` | FAIL lines (`^(FAIL\|--- FAIL)\|^ *--- FAIL`) | rc |
|---|---|---|---|
| TIP, existing suite | **391** | 0 | 0 |
| TIP + the 42 new leaves | **437** | 37 | 1 |
| P2 + the 42 new leaves | **437** | **0** | 0 (re-run on the final tree) |

437 = 391 + 42 leaves + 4 parent tests. The 37 FAIL lines are the 29 RED leaves plus their failing parents
and the package lines; they are not a leaf count. **Leaf verdicts (counted from S-agent's per-arm table):**

| group | leaves | TIP | P2 |
|---|---|---|---|
| `TestStatsFlushInterval_YAMLReject/<v>/<B0\|B1>` — 9 values × 2 | 18 | **18 RED** (`got nil error`) | 18 GREEN |
| `TestStatsFlushInterval_YAMLAccept/<v>/<B0\|B1>` — absent, `0.001s`, `0.0015s`, `0.5s`, `299.999999999s` × 2 | 10 | 10 GREEN | 10 GREEN |
| `TestStatsFlushInterval_GoBuilt` reject leaves — 9 | 9 | **9 RED** | 9 GREEN |
| `TestStatsFlushInterval_GoBuilt` accept leaves — `{0,1000000}`, `{299,999999999}` | 2 | 2 GREEN | 2 GREEN |
| `TestEnvoyGoBinary_ModeValidate_StatsFlushInterval/{0s,-1s}` | 2 | **2 RED** | 2 GREEN |
| `TestEnvoyGoBinary_ModeValidate_StatsFlushInterval/0.001s` | 1 | 1 GREEN | 1 GREEN |
| **total** | **42** | **29 RED / 13 GREEN** | **42 GREEN** |

The 13 GREEN-at-tip leaves are all ACCEPT arms and are GREEN **by design**: they are the regression gate.
The `absent/*` pair is the 5 s pin. Their falsifiers are NC7 and NC8 (§7) (method note 61).

### 3.3 The real binary — `-mode validate`, B0, TIP vs P2

| V | TIP | P2 (after the `bootstrap: stats_flush_interval: ` prefix) | reference class (§0) |
|---|---|---|---|
| absent | 0 OK | 0 OK | OK |
| `0s` | 0 OK | 1 `value must be inside range [1ms, 5m0s)` | RANGE ✅ |
| `-1s` | 0 OK | 1 `Invalid duration: Expected positive duration` | NEG ✅ |
| `-0.5s` | 0 OK | 1 `Invalid duration: Expected positive duration` | NEG ✅ |
| `0.0005s` | 0 OK | 1 RANGE | RANGE ✅ |
| `0.000999999s` | 0 OK | 1 RANGE | RANGE ✅ |
| `0.001s` | 0 OK | 0 OK | OK ✅ |
| `299.999999999s` | 0 OK | 0 OK | OK ✅ |
| `300s` | 0 OK | 1 RANGE | RANGE ✅ |
| `9223372035s` | 0 OK | 1 RANGE | RANGE ✅ |
| `9223372036s` | 0 OK | 1 `Invalid duration: Duration out-of-range` | OOR ✅ |
| `315576000000s` | 0 OK | 1 OOR | OOR ✅ |
| `315576000001s` | 1 protojson | 1 protojson (unchanged) | PARSE ✅ |

**Real boots under P2:**
- `0s` exits rc 1 in 8 ms (`load config: bootstrap: stats_flush_interval: value must be inside range [1ms, 5m0s)`).
- `-1s` exits rc 1 in 7 ms.
- `0.001s` and `299.999999999s` serve `222` with admin `/ready` 200, then drain on SIGTERM (captured PID) with
  rc 0.

### 3.4 Other gates run on P2 (not the six; a SPEC's measurement, not its compliance)

- `go vet ./internal/bootstrap/ ./cmd/envoy-go/`: rc 0.
- `gofmt -l`: empty output.
- Lint: rc 0, proven live (§0.7).
- `FuzzBootstrapLoad`, 20 s: 515,863 execs, no crash.
- `FuzzStatsSinkConfigParse`, 20 s: 522,395 execs, no crash.

---

## 4. Occurrence set of every falsified claim (owed item 6)

S-agent census: `git grep -n -i -E 'non-positive|parsed-and-inert|stats_flush_interval|D-MS-FLUSH' -- docs/
internal/ cmd/ test/ ':!docs/envoy-go/STATE_HISTORY.md' ':!docs/envoy-go/phases/'` gives **162** hits. Each
`DECISIONS.md` hit was resolved by backward `^## ADR-` search. Under `docs/envoy-go/phases/` there are **156**
hits in **38** files: closed documents, evidence, not edited. **Anchor on symbols and literals, never on these
line numbers** (method note 3).

| site (symbol / literal) | verdict | IMPL action |
|---|---|---|
| `bootstrap.go` `statsFlushIntervalDefault` comment — *"absent or non-positive"* | **FALSIFIED** | rewrite (Appendix A) |
| `bootstrap.go` `FlushInterval` field, 1st sentence — *"absent/non-positive"* | **FALSIFIED** | rewrite (Appendix A) |
| `bootstrap.go` `FlushInterval` field, 2nd sentence — *"Parsed-and-stored even when no stats_sinks[] entry exists (inert …)"* | **half-falsified** (true in range; an out-of-range bare value now rejects) | **lead with what survives**: add *"in-range"* or an equivalent clause (P2 does NOT do this — floor) |
| `bootstrap.go` `parseStatsSinks` doc comment — *"stats_flush_interval is ALWAYS stored … even with no stats_sinks[] (inert …)"* | **half-falsified** | same; also name the three reject classes and ADR-0324 |
| `BEHAVIOR_CONTRACT.md` *"a bare `stats_flush_interval` is parsed-and-inert"* (Stats-sinks section, *Byte-stable when unconfigured*) | **half-falsified** | rewrite to *"an in-range bare value is parsed-and-inert; an out-of-range one is a boot reject (ADR-0324)"* |
| `DECISIONS.md` ADR-0262 parse-arm bullet — *"`stats_flush_interval` defaults to 5s"* | **survives** (absent) — it never claims a non-positive fold | none; ADR-0324 AMENDS it |
| ADR-0064, ADR-0263, ADR-0264, ADR-0291 Flusher descriptions; ADR-0247's *"non-positive"* (an outlier threshold, unrelated) | survive | none |
| ADR-0323 *"does not touch `stats_flush_interval`"* / *"the next candidate"* | survive (historical, true when written) | none |
| `BEHAVIOR_CONTRACT.md` *"default 5s"* / *"every `stats_flush_interval`"* lines | survive | none |
| `cmd/envoy-go/main.go` D-MS-FLUSH-INERT (no goroutine without sinks) | survives | none |
| test literals `0.5s` (`main_test.go`), `2s`/`3s` (`bootstrap_test.go`, `statssink_fuzz_test.go` seeds), fixtures `0089`-`0101` `0.5s`, `0112`/`0113` `0.1s` | survive — all inside `[1ms, 5m)` | **none — do NOT tidy** (`BRAINSTORM.md` §3.5) |
| `ROADMAP.md` row 102 cell, `STATE.md` | phase narrative, rewritten at the IMPL flip | the IMPL's own |
| `SPEC-47.1.md` D-MS-FLUSH | survives; a closed document | none |

---

## 5. Unit-test design (owed item 2)

**Two NEW files, no edit to any existing test.** Appendix B is `internal/bootstrap/stats_flush_interval_test.go`
(150 lines) and Appendix C is `cmd/envoy-go/stats_flush_interval_test.go` (91 lines). Both are installed as
built, and both compile at the tip and under P2: they touch no new type, so method note 104 does not bite.

- **Reject arms assert exactly ONE class phrase, the absence of the other two, and the `bootstrap: ` prefix**
  (`assertFlushReject`; method notes 7g and 105). Every message names its property (method note 7e).
- **Every YAML arm runs on B0 (no sink) AND B1 (one statsd sink).** The sink-independence claim (§0.4 of the
  BRAINSTORM) is a pair of arms per value, not a comment.
- **Go-built arms call `parseStatsSinks` directly** for the shapes no config can carry: `{1,-1}`,
  `{0,1000000000}`, `{1,2147483647}`, `{MaxInt64,0}`. They also pin both sides of the seconds ceiling,
  `{9223372035,0}` RANGE and `{9223372036,0}` OOR.
- **Accept arms assert EXACT equality** (method note 7g), including the absent → 5 s pin.
- **The binary arm** copies `TestEnvoyGoBinary_ModeValidate`'s build/exec mechanics and asserts **exit code 1
  exactly** plus the phrase, and `configuration OK` for `0.001s`.
- ⚠️ **Hygiene for the PLAN:** the files use `b, c := b, c` loop copies. They are harmless, but redundant under
  the module's Go version. The PLAN may drop them; if it does, it re-runs the matrix.

---

## 6. Differential fixtures — TWO boot-reject directories (owed item 4)

### 6.1 Why two, and why not three

`runBootRejectFixture` asserts ONE substring per directory ("one dir = ONE runner branch",
`reference_differential_fixture_dispatch_constraint`). The NEG class can share a substring only because envoy-go
now mirrors the reference phrase. A cross-side gate is the only test that the mirror stays true against the
pinned reference, so **NEG earns its own directory**. **OOR does not:** its mechanism (a mirrored phrase in an
`Invalid duration` sibling) is the same one `0128` already exercises, it needs a ≥ 292-year value no operator
writes, and its unit arms (§5) pin the subject phrase against the measured reference phrase (§0.1). Recorded
as a choice, not an oversight.

### 6.2 Shape

Each is a symmetric `BootRejectFixture`, a copy of `0042-network-direct-response-boot-reject`'s shape:
- `README.md` plus `driver/driver.go`, and the one blank-import line in `test/differential/runner_test.go`;
- one listener `l_a` with an HCM `direct_response` `222`;
- a placeholder STATIC cluster `c_unused` at `127.0.0.1:1`;
- **no `stats_sinks`** (the B0 shape: the stronger, sink-independent claim, and the one NC1 and NC7 reach,
  §6.4);
- one top-level `stats_flush_interval`.

| dir | value | `ExpectedBootErrorSubstring()` | why this value |
|---|---|---|---|
| `0127-stats-flush-interval-range-boot-reject` | `0s` | `value must be inside range [1ms, 5m0s)` | the operator-visible silent fold (5 s at the tip); reddens NC1, NC7 |
| `0128-stats-flush-interval-negative-boot-reject` | `-0.5s` | `Invalid duration: Expected positive duration` | negative NANOS: reddens NC3, NC6, NC7 and NC9. `-1s` would miss NC6 (§0.3) |

Neither substring reaches the reference's `: goo.gle/debug…` token (§0.8). Neither contains `bootstrap:`, which
the reference never prints.

### 6.3 Ports — CENSUSED at `fbd1b2a3`

`git grep -c '\b15127\b' -- test/ internal/ cmd/` and the same for `15128` each read **0** files. They follow the
`15000 + <fixture index>` convention (`0126` → `15126`). **Re-census at the IMPL before landing.** A
boot-reject listener never binds, but the reference container still publishes the port.

### 6.4 RED at the tip, GREEN under P2, and the NC cells — PREDICTED, NOT MEASURED

**Nothing in §6 was built or run at this stage.** The PLAN builds both directories and measures every cell below
before writing it into a task (method note 89).

| shape | `0127` (`0s`, B0) | `0128` (`-0.5s`, B0) | mechanism |
|---|---|---|---|
| TIP | **RED**: the subject boots (`0s` folds to 5 s) | **RED**: the subject boots (`-0.5s` is `AsDuration` < 0, folds to 5 s) | the `> 0` fold |
| P2 | GREEN | GREEN | both sides reject with the shared phrase |
| NC1 (no lower bound) | **RED**: the subject boots silently (no sink ⇒ no ticker, §0.4) | GREEN | `{0,0}` passes F4 |
| NC3 (no NEG case) | GREEN | **RED**: subject prints RANGE | `{0,-5e8}` falls to F4 |
| NC6 (RANGE first) | GREEN | **RED**: subject prints RANGE | F4 fires before F2 |
| NC7 (sink-gated) | **RED**: boots | **RED**: boots | no sink ⇒ the block is skipped |
| NC9 (one message) | GREEN | **RED**: the substring is absent | class collapse |
| NC2, NC4, NC5, NC8 | GREEN | GREEN | neither value reaches the mutated clause |

⚠️ **A sink-ful variant would make NC1's verdict depend on how `tryStartSubjectProxy` treats an
`envoy-go ready` line followed by a panic. That is UNMEASURED, which is one more reason the fixtures stay
sink-free.**

### 6.5 Registration and cost

- **Registration:** four gates (`fixture.RegisterFixture` in `init()`, the blank import, byte-identity
  of dir name and registered string, and the `NNNN-` shape). All four fail as a SKIP or as nothing. **Score on
  the fixture set by name, both `comm` directions, never on the exit code.** The fixture set goes 128 -> **130**
  (104 `driver/` + 24 `inputs/` -> **106 + 24**).
- **Floor by shape:** `0042` is **289 lines in 2 files** (a file size, not a measured `numstat`). Two copies are
  ~580 plus two import lines. **Re-measure with `git diff --numstat` when built.**

---

## 7. Negative-control roster the PLAN inherits — neutralise, never revert

Applied to P2 by Appendix D, marker `// NC<n>`, asserted ≥ 1 before and 0 after. **Every row names its mechanism
before its result** (method note 7d). "Must redden" was traced through every arm, including the GREEN ones
(method note 76). **All nine were RUN by S-agent and every predicted RED set matched the measured set exactly,
except NC9, whose prediction over-counted by one (S-agent's own error, 4 Go-built OOR arms, not 5).**

| NC | mutation (on P2) | mechanism | measured RED leaves | must NOT redden (measured GREEN) |
|---|---|---|---|---|
| NC1 | drop `\|\| (secs == 0 && nanos < 1000000)` | sub-ms and zero values reach F5 | **8**: YAML `0s`, `0.0005s`, `0.000999999s` × B0/B1; GoBuilt `0s_999999ns`; binary `0s` | every NEG/OOR arm, `300s`, all accepts |
| NC2 | drop `\|\| nanos > 999999999` | Go-built nanos overflow reaches F4/F5; protojson normalises YAML, so no YAML arm can | **2**: GoBuilt `0s_1000000000ns`, `1s_2147483647ns` | every YAML arm (structural) |
| NC3 | delete the NEG case | `-1s` reaches F5 (negative interval — the `NewTicker` hazard); `-0.5s` falls to F4 (wrong class) | **7**: YAML `-1s`, `-0.5s` × 2; GoBuilt `1s_-1ns`, `-1s_0ns`; binary `-1s` | every RANGE/OOR arm |
| NC4 | `secs >= 300` → `secs > 300` | `300s` exactly is accepted | **3**: YAML `300s` × 2; GoBuilt `300s_0ns` | `1000s`, `299.999999999s` |
| NC5 | drop `secs > 9223372035 \|\|` | huge seconds fall to F4 (RANGE) | **6**: YAML `9223372036s`, `315576000000s` × 2; GoBuilt `MaxInt64s_0ns`, `9223372036s_0ns` | GoBuilt `9223372035s_0ns` (RANGE either way) |
| NC6 | order F4, F2, F3 | `-0.5s` (secs 0) and huge seconds hit F4 first; `-1s` does not (secs ≠ 0, < 300) | **8**: YAML `-0.5s`, `9223372036s`, `315576000000s` × 2; GoBuilt `MaxInt64s_0ns`, `9223372036s_0ns` | YAML `-1s` × 2, binary `-1s`, GoBuilt `-1s_0ns`, `1s_-1ns` |
| NC7 | gate the block on `len(bs.GetStatsSinks()) > 0` | no sink ⇒ field ignored ⇒ 5 s | **26 new** (9 B0 YAML rejects, 9 GoBuilt rejects, binary `0s`/`-1s`, 4 B0 non-absent accepts, 2 GoBuilt accepts) **+ the EXISTING `TestStatsSink_InertFlushInterval`** (§0.6) | every B1 arm, both `absent` |
| NC8 | `nanos < 1000000` → `nanos <= 1000000` | the 1 ms accept edge is rejected | **4**: YAML `0.001s` × 2; GoBuilt `0s_1000000ns`; binary `0.001s` | `0.0015s`, every reject |
| NC9 | all three cases return the RANGE message | class collapse | **15**: every NEG and OOR leaf (7 NEG + 8 OOR) | every RANGE leaf, every accept |

- **None is vacuous.** The roster has no NC shaped like row 101's NC5 (`AsDuration`): restoring `AsDuration`
  **is the tip**, and the tip column of §3.2 already scores it (29 RED).
- **NC1 and NC3 are the `NewTicker` hazards** (`BRAINSTORM.md` §3.3). At a real boot with a sink, NC1 panics
  after `ready` (§0.4). NC3's `-1s` would build a ticker with −1 s and panic the same way; this was not driven
  at a boot. State it; do not add an arm.
- **The fixture cells for these NCs (§6.4) are PREDICTED.** The PLAN measures them.

---

## 8. `ADR-0324` — §Context drafted HERE (owed item 5)

Drafted at the tail of `DECISIONS.md` in the house `> **STATUS: PROPOSED` block form (the ADR-0323 precedent):
heading, the status blockquote, `### Context`, and a RETAINED italic footer, with §Decision + §Consequences
APPENDED IN PLACE at the IMPL.
- It **AMENDS** ADR-0262's D-MS-FLUSH parse arm. *"defaults to 5s"* SURVIVES for an absent field; the bounds and
  the three rejects are new.
- It **NARROWS** D-MS-FLUSH-INERT to in-range values.
- It **NOTES** ADR-0323 §Consequences (g), which named this row next, and ADR-0323 V3, whose `9223372035`
  ceiling is the same reference bound measured on a different field.
- It **SUPERSEDES** nothing.
- This stage re-arms the house `PROPOSED` guard.

### 8.1 The guard, measured after the draft

The house form reads ONE hit, at the ADR-0324 status line. It is verified by line and by backward heading
search, never by the count (§9 gives the lines).

### 8.2 `BEHAVIOR_CONTRACT.md` ledger — **+0 NAMES**

The row adds no stat name: the reference has no flush-count stat (`BRAINSTORM.md` §2.1), and the subject's
reject fires before any stat registers. The IMPL appends a delta-only `+0, UNCHANGED` ledger entry in the
phase-96-through-101 form and quotes **no absolute**. It also rewrites the half-falsified *parsed-and-inert*
phrase per §4.

---

## 9. Counts, re-derived at THIS stage's own tip (the publishing commit)

Each figure comes from running its command in the tree that ships this file. They are re-run after the last
edit (method note 97).

- `DECISIONS.md` **19821 -> 19839** (`git diff --numstat`: **`18 0`**, the same as the phase-101 SPEC's `18 0`; the
  phase-100 SPEC's `22 0`). `^---$` **216** (UNMOVED). `^## ADR-` **322 -> 323**. Bare `^## ` **330 -> 331**.
  Tail **`## ADR-0324`**. `grep -c '^## ADR-0325'` gives **0**, so next-free is **ADR-0325**.
- **House guard `^> \*\*STATUS: PROPOSED`: RE-ARMED.** Its one hit is at `:19825`, which resolves by backward
  heading search to `:19823` `## ADR-0324`. **Decoy** `^\*\*Status:\*\* PROPOSED`: `:14866` resolves to `:14864`
  `## ADR-0231`, md5 of `:14866` `929719b67c87` (first 12 hex, `sed -n p | md5sum`), **BYTE-UNTOUCHED**.
- `ROADMAP.md` **252** / `want` **134** / row 102 `in-progress` at `:164`, **UNMOVED** (a SPEC edits no row).
- `BEHAVIOR_CONTRACT.md` **6002**, UNMOVED.
- Fixture set **128 = 128** (104 `driver/` + 24 `inputs/`), UNMOVED; `0127`/`0128` land at the IMPL.
- **Gate figures: NOT RUN (a SPEC's scope, not an omission).** They are carried from the phase-101 IMPL:
  - differential 128/0/0;
  - gate (b) 243 packages;
  - h2spec `95 tests, 94 passed, 1 skipped, 0 failed`;
  - fuzzers 56 targets / 48 files;
  - anchored panic gate 0;
  - `GOTOOLCHAIN=go1.26.2 golangci-lint run ./...` rc 0.

  The SPEC's own selector is `./internal/bootstrap/... ./cmd/envoy-go/... ./internal/statssink/...`: **391**
  at `fbd1b2a3`, **437** with the new leaves.
- **File scope: FIVE paths.** `DECISIONS.md`, `STATE.md`, `STATE_HISTORY.md` `2 0`, this `SPEC.md` and
  `next-prompt.txt`. That matches `7d18bcba` (phase 101) and `b6aa3ab3` (phase 100). No `.go`, no
  `BEHAVIOR_CONTRACT.md`, no `ROADMAP.md`.

---

## 10. Sentinel — RUN MECHANICALLY AT THIS STAGE's TIP, `/usr/bin/grep`

All commands were copied verbatim from `next-prompt.txt` and run at `fbd1b2a3` in the stage worktree. The SPEC
edits none of the files they read.

- (1) **ONE**: `NOT DONE: row 102` at `want=134`.
- (2) **SIX** at `:212 :218 :224 :234 :240 :248`.
- (3) **SILENT**.
- NC-A: the substitution was inspected first (`NC LANDED? [ in-progress ]`), then **TWO** (`NOT DONE: row 62`,
  `NOT DONE: row 102`).
- NC-B at `want=133`: **TWO** (`NOT DONE: row 102`, `GATE FAIL: examined 134 data rows, expected 133`).
- NC-C: residual **0**, `NEVER OPENED: gRPC   <- NC FIRED`.
- NC-D: **96 / 68** under `--`.
- Check-(2) positive control: residual **0**, **6** substitutions asserted.
- Escape-aware malformed set exactly **{57, 69}**, at `:119` (NF 9) and `:131` (NF 10).
- Row 102: NF **8** under both forms.
- Fixture set **128 = 128**, both `comm` directions EMPTY.
- Per-line md5 of the six windows, **trailing newline INCLUDED** (`sed -n 'Np' f | md5sum`, first 12 hex):
  `212 10d7807bf02d` · `218 4a92f7e62fc6` · `224 2a7eb298b9fd` · `234 242e53c6f7a3` · `240 b2680e6f4fbf` ·
  `248 6caa1c3ce0e7`. These are **byte-identical to the phase-102 BRAINSTORM close.**

⇒ **The sentinel does not fire. `stop` was evaluated and NOT created** (absent at the git root).

---

## 11. Probe hygiene, and what this stage banked

- **Worktrees:** the stage worktree is `/home/esa/git/envoy-go-wt-phase-102-spec` (branch `wt-phase-102-spec`,
  off `fbd1b2a3`). S-agent's throwaway detached worktrees were **created and removed**, verified by
  `git worktree list`.
- **Docker:** R-agent only, image by digest (Id and RepoDigest match `ENVOY_TARGET.md:3-4`). Each container
  `p102spec-ref-*` was removed by name right after its run, and `docker ps -a --filter name=p102spec-ref-`
  shows the header only. No network was created. Foreign `busy_brattain` and `golink-ai` were present and left
  alone.
- **Ports:** band `17100-17199`, censused first. `git grep -In '171[0-9][0-9]' -- test/ internal/ cmd/` gives
  two hits, both the same hex substring in `internal/tracing/decision_test.go` and neither a port. The
  word-bounded form gives **0**, and `ss -tan`/`ss -uan` are empty in the band. R-agent used `17100` (an unread
  statsd target), `17101` (admin) and `17102` (listener). S-agent used `17151`, `17152` and `17159`. Only
  TIME-WAIT was left. ⚠️ **This paragraph spells band numbers, so it is a hit in the next census.**
- **Scratch:** everything is under the session scratchpad (`ref/`, `subj/`), which **dies with the session**.
  Appendices A-D carry every artifact a later stage needs.
- **No production `.go` was written and none of the six gates was run.** That is a SPEC's scope. The one
  standing departure (no `REVIEW.md`, none of 93-101) is not this stage's to fix.
- **Banked (new at this stage):**
  - (a) **NC1's sink-dependent observable** (§0.4). Any future change near `Flusher.Start` should know that a
    zero interval panics only on the sink path, AFTER `ready`, and never at validate.
  - (b) **How the harness treats a subject that prints `ready` and then panics is UNMEASURED.** A sink-ful
    boot-reject fixture would depend on it.

---

## 12. What the PLAN owes

1. **Build and run every code block it embeds** (method note 89): Appendix A applied to the PLAN's own tip, plus
   the two half-falsified comment edits §4 adds (the floor moves); Appendices B and C installed as built; both
   fixture drivers.
2. **Measure every §6.4 cell before writing it**, both fixtures at the tip, under P2 and under NC1/NC3/NC6/NC7/NC9,
   with the fixture set asserted by name.
3. **Re-census `15127` / `15128`** and **re-derive the occurrence set** (§4) at the PLAN's tip.
4. **The NC roster (§7)** as appendix patches or Appendix D, each with its `NC<n>` marker. None is vacuous.
5. **The doc edits:** ADR-0324 §Decision + §Consequences, the `BEHAVIOR_CONTRACT.md` *parsed-and-inert*
   phrase, and the `+0` ledger entry.
6. **Record the un-fixed tip before the first production edit:** the 29 RED leaves named, the 13 GREEN accept
   leaves (by design), and `0127`/`0128` RED at subject boot.
7. **golangci-lint's misspell runs in locale US.** Sweep British spellings from new `.go` comments (this
   SPEC's prose uses `normalise`; the code must not).

### 12.1 Coverage of `BRAINSTORM.md` §10

| owed item | discharged at |
|---|---|
| 1 the exact validity rule, every field bounded; the message per class with the `bootstrap: ` prefix | §2, §0.1, §0.2, §0.3 |
| 2 the new RED arms: phrases, no-sink, binary `-mode validate`, Go-built | §3.2, §5, Appendices B-C |
| 3 the fixture: `0127` range, and whether a second class earns a directory | §6 (`0128` NEG yes; OOR no, §6.1) |
| 4 the NC roster with a named mechanism per row, incl. lower-bound-only and the nanos clause | §7 (NC1, NC2), §0.4 |
| 5 ADR-0324 §Context `PROPOSED`, amending ADR-0262 D-MS-FLUSH; `+0` names | §8, `DECISIONS.md` tail |
| 6 the occurrence set and what the row does NOT buy | §4, §1 |
| 7 re-measure the reference's `-1s` and `315576000000s` messages | §0.9 (stable, 3 × 2 + 2 boots each), §0.1 |

---

## Appendix A — P2, the exact diff (S-agent `P2.diff`, built at `fbd1b2a3`; the two half-falsified comments of §4 NOT included)

```diff
diff --git a/internal/bootstrap/bootstrap.go b/internal/bootstrap/bootstrap.go
index 2ea7189a..f9599748 100644
--- a/internal/bootstrap/bootstrap.go
+++ b/internal/bootstrap/bootstrap.go
@@ -224,7 +224,8 @@ const (
 	alsDefaultBufferSizeBytes uint32 = 16384
 
 	// statsFlushIntervalDefault is the stats_flush_interval default (5s) applied
-	// when the field is absent or non-positive (ADR-0262 / D-MS-FLUSH).
+	// when the field is absent (ADR-0262 / D-MS-FLUSH); a present value outside
+	// [1ms, 5m) is rejected at Load, as the reference does.
 	statsFlushIntervalDefault = 5 * time.Second
 )
 
@@ -531,8 +532,9 @@ type Bootstrap struct {
 	// OTLPMetricsSink is built in cmd/envoy-go/main.go after Load returns (and
 	// ONLY when this slice is non-empty).
 	OTLPSinkConfigs []OTLPSinkConfig
-	// FlushInterval is the stats_flush_interval (default 5s when absent/non-positive
-	// — D-MS-FLUSH). Parsed-and-stored even when no stats_sinks[] entry exists
+	// FlushInterval is the stats_flush_interval (default 5s when absent —
+	// D-MS-FLUSH; a present value outside [1ms, 5m) is rejected at Load).
+	// Parsed-and-stored even when no stats_sinks[] entry exists
 	// (inert in that case — D-MS-FLUSH-INERT); the Flusher uses it only when
 	// StatsSinkConfigs is non-empty.
 	FlushInterval time.Duration
@@ -797,9 +799,16 @@ func checkStaticLayerValues(fields map[string]*structpb.Value) error {
 func parseStatsSinks(bs *bootstrapv3.Bootstrap, result *Bootstrap) error {
 	result.FlushInterval = statsFlushIntervalDefault
 	if d := bs.GetStatsFlushInterval(); d != nil {
-		if v := d.AsDuration(); v > 0 {
-			result.FlushInterval = v
+		secs, nanos := d.GetSeconds(), d.GetNanos()
+		switch {
+		case secs < 0 || nanos < 0:
+			return fmt.Errorf("bootstrap: stats_flush_interval: Invalid duration: Expected positive duration")
+		case secs > 9223372035 || nanos > 999999999:
+			return fmt.Errorf("bootstrap: stats_flush_interval: Invalid duration: Duration out-of-range")
+		case secs >= 300 || (secs == 0 && nanos < 1000000):
+			return fmt.Errorf("bootstrap: stats_flush_interval: value must be inside range [1ms, 5m0s)")
 		}
+		result.FlushInterval = time.Duration(secs)*time.Second + time.Duration(nanos)
 	}
 	if bs.GetStatsFlushOnAdmin() {
 		return fmt.Errorf("bootstrap: stats_flush_on_admin is not supported (envoy-go ships only the periodic stats_flush_interval sink loop)")
```

## Appendix B — `internal/bootstrap/stats_flush_interval_test.go`, verbatim (S-agent; 41 leaves, 3 parent tests; compiles at the tip and under P2)

```go
package bootstrap

import (
	"math"
	"strings"
	"testing"
	"time"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	"google.golang.org/protobuf/types/known/durationpb"
)

// The three reference rejection classes for stats_flush_interval (phase 102).
const (
	flushNegPhrase   = "Invalid duration: Expected positive duration"
	flushOORPhrase   = "Invalid duration: Duration out-of-range"
	flushRangePhrase = "value must be inside range [1ms, 5m0s)"
)

var flushPhrases = []string{flushNegPhrase, flushOORPhrase, flushRangePhrase}

// flushStatsdSink is one statsd stats_sinks[] entry (bootstrap B1).
const flushStatsdSink = `stats_sinks:
  - name: envoy.stat_sinks.statsd
    typed_config:
      "@type": ` + statsdSinkType + `
      address:
        socket_address: {address: 127.0.0.1, port_value: 8125}
`

// flushBootstraps are B0 (no sink) and B1 (one statsd sink): the reference
// refuses an out-of-range value with or without a sink.
var flushBootstraps = []struct{ name, sinks string }{
	{"B0_no_sink", ""},
	{"B1_statsd_sink", flushStatsdSink},
}

// assertFlushReject asserts err carries exactly one class phrase (want) and
// the "bootstrap: " prefix FuzzBootstrapLoad requires.
func assertFlushReject(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("got nil error, want one containing %q", want)
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "bootstrap: ") {
		t.Errorf("error %q lacks the %q prefix", msg, "bootstrap: ")
	}
	if !strings.Contains(msg, want) {
		t.Errorf("error %q does not contain %q", msg, want)
	}
	for _, other := range flushPhrases {
		if other != want && strings.Contains(msg, other) {
			t.Errorf("error %q also contains %q (want a single cause)", msg, other)
		}
	}
}

func TestStatsFlushInterval_YAMLReject(t *testing.T) {
	cases := []struct {
		value, want string
	}{
		{"0s", flushRangePhrase},
		{"0.0005s", flushRangePhrase},
		{"0.000999999s", flushRangePhrase},
		{"300s", flushRangePhrase},
		{"1000s", flushRangePhrase},
		{"-1s", flushNegPhrase},
		{"-0.5s", flushNegPhrase},
		{"9223372036s", flushOORPhrase},
		{"315576000000s", flushOORPhrase},
	}
	for _, b := range flushBootstraps {
		for _, c := range cases {
			b, c := b, c
			t.Run(c.value+"/"+b.name, func(t *testing.T) {
				src := statsBootstrap("stats_flush_interval: " + c.value + "\n" + b.sinks)
				_, err := Load(strings.NewReader(src))
				assertFlushReject(t, err, c.want)
			})
		}
	}
}

func TestStatsFlushInterval_YAMLAccept(t *testing.T) {
	cases := []struct {
		name, line string
		want       time.Duration
	}{
		{"absent", "", 5 * time.Second},
		{"0.001s", "stats_flush_interval: 0.001s\n", time.Millisecond},
		{"0.0015s", "stats_flush_interval: 0.0015s\n", 1500 * time.Microsecond},
		{"0.5s", "stats_flush_interval: 0.5s\n", 500 * time.Millisecond},
		{"299.999999999s", "stats_flush_interval: 299.999999999s\n", 299*time.Second + 999999999},
	}
	for _, b := range flushBootstraps {
		for _, c := range cases {
			b, c := b, c
			t.Run(c.name+"/"+b.name, func(t *testing.T) {
				bs, err := Load(strings.NewReader(statsBootstrap(c.line + b.sinks)))
				if err != nil {
					t.Fatalf("Load: %v", err)
				}
				if bs.FlushInterval != c.want {
					t.Errorf("FlushInterval: got %v, want %v", bs.FlushInterval, c.want)
				}
			})
		}
	}
}

func TestStatsFlushInterval_GoBuilt(t *testing.T) {
	cases := []struct {
		name   string
		secs   int64
		nanos  int32
		reject string        // class phrase; "" means accept
		want   time.Duration // FlushInterval on accept
	}{
		{"0s_999999ns", 0, 999999, flushRangePhrase, 0},
		{"0s_1000000ns", 0, 1000000, "", time.Millisecond},
		{"299s_999999999ns", 299, 999999999, "", 299*time.Second + 999999999},
		{"300s_0ns", 300, 0, flushRangePhrase, 0},
		{"1s_-1ns", 1, -1, flushNegPhrase, 0},
		{"-1s_0ns", -1, 0, flushNegPhrase, 0},
		{"0s_1000000000ns", 0, 1000000000, flushOORPhrase, 0},
		{"1s_2147483647ns", 1, 2147483647, flushOORPhrase, 0},
		{"MaxInt64s_0ns", math.MaxInt64, 0, flushOORPhrase, 0},
		{"9223372035s_0ns", 9223372035, 0, flushRangePhrase, 0},
		{"9223372036s_0ns", 9223372036, 0, flushOORPhrase, 0},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			bs := &bootstrapv3.Bootstrap{StatsFlushInterval: &durationpb.Duration{Seconds: c.secs, Nanos: c.nanos}}
			result := &Bootstrap{}
			err := parseStatsSinks(bs, result)
			if c.reject != "" {
				assertFlushReject(t, err, c.reject)
				return
			}
			if err != nil {
				t.Fatalf("parseStatsSinks: %v", err)
			}
			if result.FlushInterval != c.want {
				t.Errorf("FlushInterval: got %v, want %v", result.FlushInterval, c.want)
			}
		})
	}
}
```

## Appendix C — `cmd/envoy-go/stats_flush_interval_test.go`, verbatim (S-agent; 3 leaves, 1 parent test)

```go
package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// flushValidateYAML is the TestEnvoyGoBinary_ModeValidate good config, with a
// stats_flush_interval line appended (phase 102).
const flushValidateYAML = `
node: { id: test-node, cluster: test-cluster }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 127.0.0.1, port_value: 0 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
  clusters:
    - name: c_echo
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 0 }
`

// TestEnvoyGoBinary_ModeValidate_StatsFlushInterval runs --mode validate on
// the built binary: 0s and -1s exit 1 naming their reference class; 0.001s
// (the lower bound) validates.
func TestEnvoyGoBinary_ModeValidate_StatsFlushInterval(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "envoy-go")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	cases := []struct {
		value, wantErr string // wantErr "" means exit 0 + "configuration OK"
	}{
		{"0s", "value must be inside range [1ms, 5m0s)"},
		{"-1s", "Invalid duration: Expected positive duration"},
		{"0.001s", ""},
	}
	for _, c := range cases {
		c := c
		t.Run(c.value, func(t *testing.T) {
			cfg := filepath.Join(t.TempDir(), "cfg.yaml")
			if err := os.WriteFile(cfg, []byte(flushValidateYAML+"stats_flush_interval: "+c.value+"\n"), 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			out, err := exec.CommandContext(ctx, bin, "-c", cfg, "--mode", "validate").CombinedOutput()
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("--mode validate failed: %v\n%s", err, out)
				}
				if !strings.Contains(string(out), "configuration OK") {
					t.Errorf("stdout = %q, want it to contain %q", out, "configuration OK")
				}
				return
			}
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
				t.Fatalf("got err=%v, want *exec.ExitError with exit code 1 (out=%s)", err, out)
			}
			if !strings.Contains(string(out), c.wantErr) {
				t.Errorf("output = %q, want it to contain %q", out, c.wantErr)
			}
		})
	}
}
```

## Appendix D — the NC applier, verbatim (S-agent `nc.py`; `python3 nc.py <n> internal/bootstrap/bootstrap.go` on a P2 tree; each mutant carries its `// NC<n>` marker)

```python
import sys
n=sys.argv[1]; p=sys.argv[2]
s=open(p).read()
NEG='''		case secs < 0 || nanos < 0:
			return fmt.Errorf("bootstrap: stats_flush_interval: Invalid duration: Expected positive duration")
'''
OOR='''		case secs > 9223372035 || nanos > 999999999:
			return fmt.Errorf("bootstrap: stats_flush_interval: Invalid duration: Duration out-of-range")
'''
RNG='''		case secs >= 300 || (secs == 0 && nanos < 1000000):
			return fmt.Errorf("bootstrap: stats_flush_interval: value must be inside range [1ms, 5m0s)")
'''
M={
'1':[('		case secs >= 300 || (secs == 0 && nanos < 1000000):\n','		case secs >= 300: // NC1\n')],
'2':[('		case secs > 9223372035 || nanos > 999999999:\n','		case secs > 9223372035: // NC2\n')],
'3':[(NEG,'		// NC3 NEG case deleted\n')],
'4':[('case secs >= 300 ||','case secs > 300 || // NC4\n\t\t\t')],
'5':[('		case secs > 9223372035 || nanos > 999999999:\n','		case nanos > 999999999: // NC5\n')],
'6':[(NEG+OOR+RNG, RNG.replace(':\n',': // NC6\n',1)+NEG+OOR)],
'7':[('	if d := bs.GetStatsFlushInterval(); d != nil {\n','	if d := bs.GetStatsFlushInterval(); d != nil && len(bs.GetStatsSinks()) > 0 { // NC7\n')],
'8':[('(secs == 0 && nanos < 1000000):\n','(secs == 0 && nanos <= 1000000): // NC8\n')],
'9':[('Invalid duration: Expected positive duration")\n','value must be inside range [1ms, 5m0s)") // NC9\n'),('Invalid duration: Duration out-of-range")\n','value must be inside range [1ms, 5m0s)") // NC9\n')],
}
for a,b in M[n]:
    assert s.count(a)==1,(n,a)
    s=s.replace(a,b)
open(p,'w').write(s)
```
