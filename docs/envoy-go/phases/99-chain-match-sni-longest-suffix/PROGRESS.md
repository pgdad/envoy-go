# Phase 99 (chain-match-sni-longest-suffix) — IMPL progress

## Task 1: Baseline, a proven-live panic gate, and PROGRESS.md

**Step 1 — selectors resolve** (`go list ./cmd/envoy-go/... ./internal/admin/... ./internal/boot/... ./internal/listener/... ./validate/...`):
RC=0, all 7 packages listed (no `[setup failed]`):
```
github.com/pgdad/envoy-go/cmd/envoy-go
github.com/pgdad/envoy-go/internal/admin
github.com/pgdad/envoy-go/internal/boot
github.com/pgdad/envoy-go/internal/listener
github.com/pgdad/envoy-go/internal/listener/listenerfilter
github.com/pgdad/envoy-go/internal/listener/listenerfilter/tls_inspector
github.com/pgdad/envoy-go/validate
```
`go version`: go1.27.1 linux/amd64.

**Step 2 — baseline recorded** to `$SCRATCH/base.txt` (base.roster to `$SCRATCH/base.roster`):

| measurement | expected | actual |
|---|---|---|
| RC (PIPESTATUS[0]) | — | 0 |
| `=== RUN` count | 398 | 398 |
| FAIL count (`^(FAIL\|--- FAIL)\|^ *--- FAIL`) | 0 | 0 |
| panic gate (`^panic:\|DATA RACE\|SIGSEGV`) | 0 | 0 |
| `base.roster` line count | — | 398 |

All figures match the plan's expected values exactly. No un-fixed regression is present in this run.

**Step 3 — panic gate proven live.** Inserted `panic("p99 gate probe")` as the first statement of
`breakTie` in `internal/listener/listenerfilter/chainmatch.go` (anchor A1, line 201-202).

- Selector-matches-nothing sanity check (`-run 'NoSuchTestXYZ123'`): RC=0, output
  `testing: warning: no tests to run` / `ok ... [no tests to run]` — confirms the selector resolves
  before any FAIL/panic is believed.
- During probe: `go test -count=1 -v -run 'TestSelectChainBreakTieFollowsPriorityOrder' ./internal/listener/listenerfilter/`
  → RC=1, panic gate count = **1** (≥ 1 as required). Traceback confirms the panic originates at
  `chainmatch.go:202` inside `breakTie`, called from `SelectChain` (chainmatch.go:104), called from
  `TestSelectChainBreakTieFollowsPriorityOrder` (chainmatch_test.go:164).
- Reverted the probe (removed the `panic(...)` line). `git diff --numstat -- internal/listener/listenerfilter/chainmatch.go`
  is EMPTY (byte-untouched after revert — the probe was never committed).
- After revert, same test rerun: RC=0, panic gate count = **0**.

**Step 4 — known flake check.** `TestEnvoyGoBinary_TwoListenerCutover` did not flake in this baseline
run: `--- PASS: TestEnvoyGoBinary_TwoListenerCutover (1.43s)`, no `bind 127.0.0.1:<port>` message present.
No port to record for this run.

**Step 5/6 — this file created and committed** (Task 1 commit, below).

---

## Task 2: Fold-in (a) — the compressor level assertion, RE-POINTED

**Step 1 — RED under ambient, PASS under pinned.**

`go version`: go1.27.1 linux/amd64.

`go test -count=1 -v -run 'TestEncodeData_LevelMapping_DifferentGzippedSizes' ./internal/filter/http/compressor/`:
- Ambient (go1.27.1): RC=1, `--- FAIL`, message `expected different compressed sizes for BestSpeed vs
  BestCompression on a non-repetitive input; both = 2121` — matches the plan's expected "both = 2121"
  exactly.
- `GOTOOLCHAIN=go1.26.2`: RC=0, `--- PASS`.

**Step 2 — Appendix G applied.** `git apply` against
`internal/filter/http/compressor/compressor_test.go`: clean apply (`git apply --check` OK). Result of
`git diff --numstat`: **`53  49  internal/filter/http/compressor/compressor_test.go`** — exactly matches
the plan's expected `53 49`, and only that one file was touched (confirmed via `git status --porcelain`).
Replaces `TestEncodeData_LevelMapping_DifferentGzippedSizes` with
`TestEncodeData_LevelMapping_LevelReachesEncoder`.

**Step 3 — green on both toolchains, full package.**

| toolchain | RC | `=== RUN` | FAIL |
|---|---|---|---|
| ambient go1.27.1 | 0 | 186 | 0 |
| pinned GOTOOLCHAIN=go1.26.2 | 0 | 186 | 0 |

Both match the plan's expected **186** RUN / 0 FAIL exactly.

**Step 4 — mutation matrix, both toolchains.**

M1 (`acquireWriter`'s fresh-writer branch: `gzip.NewWriterLevel(buf, g.level)` →
`gzip.NewWriterLevel(buf, gzip.DefaultCompression)`):

| toolchain | result |
|---|---|
| ambient go1.27.1 | FAIL: response 0 `XFL = 0; want 4` (BestSpeed) / `want 2` (BestCompression); response 1 same |
| pinned go1.26.2 | identical FAIL on responses 0 and 1 |

Matches expected: FAIL with XFL 0 (want 4 or 2) on responses 0 **and** 1 — the mutated fresh-writer path
poisons the pooled writer that response 1 later `Reset`s (the pool holds a writer already built at the
wrong level).

M2 (pooled branch changed to build a fresh writer at `gzip.DefaultCompression` instead of `Reset`ting the
pooled one — pooled writer discarded, `w.Reset(buf)` replaced by
`gzip.NewWriterLevel(buf, gzip.DefaultCompression)` when `ok`):

| toolchain | result |
|---|---|
| ambient go1.27.1 | FAIL: response 1 only, `XFL = 0; want 4` (BestSpeed) / `want 2` (BestCompression); response 0 passes |
| pinned go1.26.2 | identical: FAIL on response 1 only |

Matches expected: FAIL on response 1 only, both toolchains.

`compressor.go` reverted after each mutation; final `git diff --numstat -- internal/filter/http/compressor/compressor.go`
is EMPTY — byte-untouched. `git status --porcelain` shows only `compressor_test.go` modified (the
Appendix G patch), which is the intended, committed change.

**Step 5/6 — matrix recorded above; committed** (Task 2 commit, below).

---

## Task 3: Fold-in (b) — lint under the pinned toolchain, with a COMPILING planted control

**Step 1 — baseline.** `cd "$W" && GOTOOLCHAIN=go1.26.2 golangci-lint run ./...`: RC=0, no output.
Matches expected exactly.

**Step 2 — planted control that compiles.** Created `internal/listener/zz_planted.go` (exact content from
the brief). Same lint command: **RC=1**, and the output names **all three** required linters:
- `errcheck` — `Error return value of \`os.Remove\` is not checked`
- `revive` — `exported: exported function PlantedExported should have comment or be unexported`
- `ineffassign` — `ineffectual assignment to x`

No `typecheck`-only output — the control is NOT masked (router method note 88's failure mode did not
occur).

**Step 3 — delete + re-run.** Deleted `internal/listener/zz_planted.go`. Re-ran the same command: RC=0,
no output — matches expected. `git status --porcelain` shows nothing for this path both before creation
(untracked, unstaged) and after deletion; the planted file was never `git add`ed and never committed.

**CI check.** `.github/workflows/ci.yml`: `actions/setup-go@v6` pinned to `go-version: '1.23'` (three call
sites, lines 12-14, 37-39, 101-103); `golangci/golangci-lint-action@v6.5.2` with `version: v1.64.8` (the
golangci-lint binary pin). The brief names this "golangci-lint-action v1.64.8" — the action itself is
pinned at `v6.5.2`, and the *golangci-lint tool* it installs is pinned at `v1.64.8`; the substantive
figure (golangci-lint v1.64.8) matches. Since CI's ambient Go is 1.23 (not the go1.27.1 that breaks the
linter's export-data reader per SPEC.md §10), **CI needs no change** — confirmed.

**Step 4 — committed** (Task 3 commit, below).

---

## Task 4: The unit file — `chainmatch_sni_test.go`, RED at the un-fixed tip

**Step 1 — write + collision check.** Created `internal/listener/listenerfilter/chainmatch_sni_test.go`
verbatim from Appendix B (102 lines). `git grep -n --untracked 'func sniInputs\|func sniChain' --
internal/listener/listenerfilter/` (note: plain `git grep` misses untracked files — used `--untracked`)
lists only the new file, at lines 12 and 24. No collision.

**Step 2 — run at the un-fixed tip.**

```sh
go test -count=1 -v -run 'TestSelectChainSNILongestMatchedSuffix|TestSelectChainAmbiguousReturnsError' \
  ./internal/listener/listenerfilter/
```

RC=1.

**Step 3 — per-row score against §4's `tip` column.**

| row | tip (measured) | expected |
|---|---|---|
| `C0_long_declared_first_longer_suffix_wins` | FAIL | F |
| `C0_short_declared_first_longer_suffix_wins` | FAIL | F |
| `C0_matched_negative_only_short_matches` | PASS | P |
| `B_desc_three_levels_deepest_suffix_wins` | FAIL | F |
| `B_desc_middle_suffix_beats_shortest` | FAIL | F |
| `B_asc_three_levels_deepest_suffix_wins` | FAIL | F |
| `B_asc_middle_suffix_beats_shortest` | FAIL | F |
| `M1_rank_of_matched_pattern_not_whole_set_X_first` | FAIL | F |
| `M2_rank_of_matched_pattern_not_whole_set_Y_first` | FAIL | F |
| `M1_exact_member_of_mixed_set_still_wins_its_name` | PASS | P |
| `M3_longest_matching_member_not_longest_member_X2_first` | FAIL | F |
| `M4_longest_matching_member_not_longest_member_Y_first` | FAIL | F |
| `M3_deeper_member_of_mixed_set_wins` | FAIL | F |
| `M4_deeper_member_of_mixed_set_wins` | FAIL | F |
| `E_default_present_two_wildcards_resolve_not_default` | FAIL | F |
| `E_default_serves_only_no_match` | PASS | P |
| `R_exact_beats_wildcard_exact_first` | PASS | P |
| `R_exact_beats_wildcard_wildcard_first` | PASS | P |
| `O1_shared_matched_wildcard_is_ambiguous` | FAIL | F |
| `TestSelectChainAmbiguousReturnsError` (top-level) | PASS | P |

Counted **by NAME** (20 `--- (PASS|FAIL)` lines total: 19 subtests of
`TestSelectChainSNILongestMatchedSuffix` + the separate top-level `TestSelectChainAmbiguousReturnsError`):
**14 FAIL / 5 PASS** among the 19 rows, `TestSelectChainAmbiguousReturnsError` PASS — matches the brief's
Step 3 expectation exactly (13 precedence rows + O1). No panic-gate hit
(`^panic:|DATA RACE|SIGSEGV` — zero matches in the run log).

**Step 4 — gofmt / vet / lint.**
- `gofmt -l internal/listener/listenerfilter/chainmatch_sni_test.go internal/listener/quic_test.go`: no
  output (clean).
- `go vet ./internal/listener/...`: RC=0, no output.
- `GOTOOLCHAIN=go1.26.2 golangci-lint run ./internal/listener/...`: RC=0, no output — clean even with the
  RED test file present, as expected (a RED test result is not a lint finding).

Committed (Task 4 commit, below).

---

## Task 5: The QUIC wiring arm — RED at the un-fixed tip

**Step 1 — append + shape check.** Appended Appendix C verbatim to the end of
`internal/listener/quic_test.go`, immediately after anchor A8
(`TestQUICChainSelection_TLSConfigPrefersIndexedChainOverTLSDefaultSlot`). No import block touched.

`git diff -U0 -- internal/listener/quic_test.go`: exactly **one hunk**,
`@@ -1741,0 +1742,110 @@ func TestQUICChainSelection_TLSConfigPrefersIndexedChainOverTLSDefaultSlot(t *te` —
the file's actual line count at this tip (1741 lines pre-append) differs from the brief's predicted
`@@ -1739,3 +1739,113 @@` anchor by a few lines (110 inserted lines measured vs. 113 predicted; the file
grew from 1741 to 1851 lines), which the brief explicitly allows as "its re-derived equivalent at the file
end". **Zero `-` lines** — confirmed no existing line moved or was removed.

`chainmatch.go:293-302` literal check: the appended arm text (lines 1742-1851) contains **zero**
occurrences. The one occurrence in this file is pre-existing, at line 952 (`selectChainAny`'s doc
comment), outside the appended span — not introduced by this task.

**Step 2 — run at the tip.**

```sh
go test -count=1 -v -run 'TestQUICChainSelection_TwoWildcardsLongestMatchedSuffixWins' ./internal/listener/
```

RC=1.

| subtest | result | failing property | error text |
|---|---|---|---|
| `LONG_declared_first` | FAIL | PROPERTY 1 only | `Get "https://...": H3 error (0x0)`, body `""`, want `"LONG\n"` |
| `SHORT_declared_first` | FAIL | PROPERTY 1 only | `Get "https://...": H3 error (0x0)`, body `""`, want `"LONG\n"` |

Both subtests FAIL, exactly one `--- FAIL` line per subtest and exactly one `quic_test.go:1841:` (the
PROPERTY 1 `t.Errorf`) failure line per subtest — no line from the PROPERTY 2 assertion appears in either
run, so PROPERTY 2 (the `x.foo.test` matched negative → SHORT) PASSES in both, proving SHORT is live.
Matches the brief's Step 2 expectation exactly. No panic-gate hit.

**Step 3 — committed** (Task 5 commit, below).

---

## Task 6: Fixture `0124` — PKI

Wrote `test/fixtures/0124-listener-sni-longest-suffix/pki/gen/main.go` verbatim from Appendix E.2.

**Step 1 — generate.** `cd test/fixtures/0124-listener-sni-longest-suffix && go run ./pki/gen` → `ok: 3
PEMs written to pki`, rc 0.

**Step 2 — SANs.** `openssl x509 -in pki/server.pem -noout -ext subjectAltName`:

```
X509v3 Subject Alternative Name:
    DNS:*.foo.test, DNS:*.b.foo.test, DNS:*.c.b.foo.test, DNS:x.test, DNS:nomatch.example
```

All five names present, including `nomatch.example` (§0.7).

**Step 3 — determinism.** sha256 of all three PEMs before and after a second `go run ./pki/gen`:

| file | run 1 | run 2 |
|---|---|---|
| `pki/ca.pem` | `2e7be31c...d3b0c0` | identical |
| `pki/server.pem` | `ca1f697f...4e986` | identical |
| `pki/server.key.pem` | `a3bd899c...e30fc` | identical |

`diff` of the two sha256sum outputs was empty. Committed (`bd388034`).

---

## Task 7: Fixture `0124` — the driver

Wrote `test/fixtures/0124-listener-sni-longest-suffix/driver/driver.go` verbatim from Appendix E.1 (609
lines — matches the appendix's stated line count).

**Step 1 — port re-census** (§2.4), at the IMPL tip, after Task 6 landed:

```
git grep -lw -- 15124 -- test/ internal/ cmd/   → test/fixtures/0123-.../README.md, driver/driver.go (reserving prose only)
git grep -lw -- 15225 -- test/ internal/ cmd/   → zero files
git grep -lw -- 15226 -- test/ internal/ cmd/   → zero files
git grep -lw -- 15227 -- test/ internal/ cmd/   → zero files
ss -tan | grep -E ':(15124|15225|15226|15227)\b'  → zero sockets
ss -uan | grep -E ':(15124|15225|15226|15227)\b'  → zero sockets
```

Unchanged from the PLAN-stage census.

**Step 2 — driver written**, satisfying `fixture.Driver`, `fixture.MultiListenerDriver`,
`fixture.StatsAsserter`; `go build ./test/fixtures/0124-listener-sni-longest-suffix/...` rc 0 (the fixture
is not yet wired into the runner — that is Task 8's blank import).

**Step 3 — subject config validation.** `renderBootstrap` is unexported, so a throwaway `_test.go` was
written *inside* the driver package directory (never in `$SCRATCH`, since the function is unexported and
package-private), run once with `go test -run '^TestZZZDumpSubjectConfig$'` to dump the rendered YAML to
`$SCRATCH/rendered.yaml`, then deleted immediately (never committed — `git status` shows no trace).
Built `go build -o $SCRATCH/eg ./cmd/envoy-go/` (rc 0) and ran `$SCRATCH/eg -mode validate -c
$SCRATCH/rendered.yaml` → `configuration OK`, rc 0.

**Step 4 — gofmt / vet / lint.**
- `gofmt -l driver.go pki/gen/main.go`: no output (clean).
- `go vet ./test/fixtures/0124-listener-sni-longest-suffix/...`: rc 0, no output.
- `GOTOOLCHAIN=go1.26.2 golangci-lint run ./test/fixtures/0124-listener-sni-longest-suffix/...`: rc 0, no
  output.

Committed (`5cd6fcba`).

---

## Task 8: Fixture `0124` — `README.md`, `expectations.yaml`, and the FOUR registration gates

Wrote `README.md` (Appendix E.3) and `expectations.yaml` (Appendix E.4) verbatim, and added the blank
import `_ "github.com/pgdad/envoy-go/test/fixtures/0124-listener-sni-longest-suffix/driver"` to
`test/differential/runner_test.go` directly after the `0123` import (line 150 → new line 151, before the
`test/helpers` import).

**Step 2 — the four gates.** `RegisterFixture(fixtureName, &sniDriver{})` runs in the driver's `init()`
(gate 1); the blank import above satisfies gate 2; the registered string `0124-listener-sni-longest-suffix`
is byte-identical to the directory name `test/fixtures/0124-listener-sni-longest-suffix` (gate 3, checked
by eye — both are the literal `fixtureName` constant and the directory basename); the directory matches the
`NNNN-` shape (gate 4, no subtest — confirmed by `ls -d test/fixtures/0124-listener-sni-longest-suffix/`).

**Step 3 — set-difference score.**

```sh
extract() { /usr/bin/grep -oE '^[[:space:]]*_ "github\.com/pgdad/envoy-go/test/fixtures/[^/]+/(driver|inputs)"$' "$1" \
  | sed -E 's#.*/test/fixtures/##; s#/(driver|inputs)"$##' | sort; }
extract test/differential/runner_test.go > imports.txt   # 126 lines
ls -d test/fixtures/*/ | sed -E 's#test/fixtures/##; s#/$##' | sort > dirs.txt   # 126 lines
comm -23 imports.txt dirs.txt   # empty
comm -13 imports.txt dirs.txt   # empty
```

126 = 126, both `comm` directions empty — matches PLAN §2.3's post-`0124` prediction exactly.

**Step 4 — NC the extractor, in a scratch copy of `runner_test.go`.**

- **RENAME** (`0124-listener-sni-longest-suffix` → `0124-listener-sni-longest-suffix-RENAMED` in the import
  line only): `comm -23` (imports-only) surfaces the renamed entry, `comm -13` (dirs-only) surfaces the
  original name — **both directions fire**, one entry each. Matches the brief.
- **DELETION** (the `0124` import line removed entirely): `comm -23` (imports-only) is **empty**; `comm
  -13` (dirs-only) surfaces `0124-listener-sni-longest-suffix`. **Measured: only `comm -13` fires**, not
  `comm -23` as the brief's Step 4 states.

⚠️ **Disagreement with the brief, recorded per instructions.** The brief text reads "by DELETION (only
`comm -23` fires)". With the `comm -23 imports.txt dirs.txt; comm -13 imports.txt dirs.txt` argument order
used throughout Step 3 (imports first, dirs second) — the same order the brief's own Step 3 snippet
specifies — deleting an import removes it from `imports.txt` while `dirs.txt` still lists the directory, so
the surviving entry is unique to `dirs.txt` and therefore appears under `comm -13` (dirs-only), not `comm
-23` (imports-only). This follows directly from `comm`'s column semantics given that argument order and was
re-measured twice to rule out a transcription slip on this agent's part. The variable is most likely a
swapped mental model in the brief's own labeling (which direction "fires" was probably reasoned about with
the arguments in the opposite order, or a generic "one direction fires" was mislabeled with the wrong `comm`
flag) — not a defect in the extractor or the fixture. Both control shapes still discriminate correctly (a
rename is caught on both sides; a deletion is caught on the dirs-only side), so the gate itself is sound;
only the brief's flag label for the deletion arm is wrong.

Committed (`78f668b5`).

---

## Task 9: RECORD the un-fixed tip — every falsifier RED, and the roster SPENT

**Step 1 — fixture `0124` alone**, reference pinned by digest (`envoyproxy/envoy@sha256:7edd5b0f...453be8`,
verified against `docs/envoy-go/ENVOY_TARGET.md` lines 3-4 before the run; the harness pulls/runs it):

```sh
go test -count=1 -v -run 'TestDifferential/0124-listener-sni-longest-suffix' ./test/differential/
```

RC=1. `=== RUN` count: 2 (parent + the one registered subtest — the runner drives all four listeners
inside that one subtest via `MultiListenerDriver`). `--- SKIP` count: 0. `connection CLOSED without a
response|served body` count: 4. No panic-gate hit. No driver-owned receiver-port-race flake observed (no
retry was needed).

**Per-row table** (subject; the reference was green on every row, all nine `downstream_rq_total` values
matched `want` exactly):

| row | ref | subject (tip) |
|---|---|---|
| `l_long_first` a.b.foo.test | LONG | **CLOSED** (`tls handshake: EOF`) |
| `l_long_first` x.foo.test | SHORT | SHORT (green, single-candidate) |
| `l_short_first` a.b.foo.test | LONG | **CLOSED** (`tls handshake: EOF`) |
| `l_short_first` x.foo.test | SHORT | SHORT (green, single-candidate) |
| `l_mixed` a.b.foo.test | Y | **X** (served, wrong chain) |
| `l_mixed` q.foo.test | X | X (green, single-candidate) |
| `l_mixed` x.test | X | X (green, single-candidate) |
| `l_default` a.b.foo.test | LONG | **CLOSED** (`tls handshake: EOF`) |
| `l_default` nomatch.example | DEFAULT | DEFAULT (green, single-candidate) |

Subject-side listener logs recorded `listener "l_long_first"/"l_short_first"/"l_default": chain-match:
ambiguous filter_chain selection` at each CLOSE.

**Counter pins** (subject; `http.<prefix>.downstream_rq_total`, all present):

| prefix | want | subject (tip) |
|---|---|---|
| `lf_long` | 1 | **0** |
| `lf_short` | 1 | 1 |
| `sf_short` | 1 | 1 |
| `sf_long` | 1 | **0** |
| `mx_x` | 2 | **3** |
| `mx_y` | 1 | **0** |
| `df_long` | 1 | **0** |
| `df_short` | 0 | 0 |
| `df_default` | 1 | 1 |

Every reference counter matched `want`. This is exactly the brief's Step 1 expectation: subject RED on
exactly the four `a.b.foo.test` rows (three CLOSED, `l_mixed` serving X), `lf_long`/`sf_long`/`df_long` at 0
instead of 1, `mx_x`=3/`mx_y`=0 instead of 2/1, reference green on every row.

**Step 2 — five-selector suite.**

```sh
go test -count=1 -v ./cmd/envoy-go/... ./internal/admin/... ./internal/boot/... ./internal/listener/... ./validate/...
```

RC=1, **421** `=== RUN` (matches 398 base + 23 new exactly). No panic-gate hit.

Diffed the sorted `=== RUN` roster against Task 1's `$SCRATCH/base.roster`: **23 added, 0 removed** —
exactly the 20 `TestSelectChainSNILongestMatchedSuffix` rows (parent + 19 subtests) and 3
`TestQUICChainSelection_TwoWildcardsLongestMatchedSuffixWins` rows (parent + 2 subtests).

**RED roster** (18 `--- FAIL` lines among the 23 new names — matches Tasks 4-5's per-row records exactly):

- `TestQUICChainSelection_TwoWildcardsLongestMatchedSuffixWins` (parent) and both subtests
  (`LONG_declared_first`, `SHORT_declared_first`) — all 3 FAIL.
- `TestSelectChainSNILongestMatchedSuffix` (parent) FAILs; 14 of its 19 subtests FAIL:
  `B_asc_middle_suffix_beats_shortest`, `B_asc_three_levels_deepest_suffix_wins`,
  `B_desc_middle_suffix_beats_shortest`, `B_desc_three_levels_deepest_suffix_wins`,
  `C0_long_declared_first_longer_suffix_wins`, `C0_short_declared_first_longer_suffix_wins`,
  `E_default_present_two_wildcards_resolve_not_default`,
  `M1_rank_of_matched_pattern_not_whole_set_X_first`,
  `M2_rank_of_matched_pattern_not_whole_set_Y_first`, `M3_deeper_member_of_mixed_set_wins`,
  `M3_longest_matching_member_not_longest_member_X2_first`, `M4_deeper_member_of_mixed_set_wins`,
  `M4_longest_matching_member_not_longest_member_Y_first`, `O1_shared_matched_wildcard_is_ambiguous`.
- The remaining 5 new subtests PASS (structurally green at the tip, as designed):
  `C0_matched_negative_only_short_matches`, `E_default_serves_only_no_match`,
  `M1_exact_member_of_mixed_set_still_wins_its_name`, `R_exact_beats_wildcard_exact_first`,
  `R_exact_beats_wildcard_wildcard_first`.

This is byte-for-byte the union of Task 4's per-row table (14 FAIL / 5 PASS among the 19 unit subtests) and
Task 5's QUIC record (both subtests FAIL) — no new divergence appeared when run as part of the full
five-selector suite rather than the narrower `-run` selections Tasks 4/5/this-step-1 used.

**Step 3 — this section is that record.** Nothing after Task 10 (the production fix) can reproduce these
figures at the un-fixed tip; Task 10 changes `internal/listener/listenerfilter/chainmatch.go` and every
number above will move.

Committed together with this PROGRESS.md section (Tasks 6-9 combined; this task's own commit follows).

---

## Task 10: the production edit — `sniMatchedRank` and slot 2, plus the two cites it shifts

**Step 1.** Applied only the two CODE hunks of Appendix A to
`internal/listener/listenerfilter/chainmatch.go`: the `breakTie` slot-2 block (now calling
`sniMatchedRank(a.ServerNames, inputs.ServerName)` / `sniMatchedRank(b.ServerNames, inputs.ServerName)`,
with the added `la > lb` / `lb > la` longest-matched-suffix tie-break and its own inline comment), and the
function replacement (`sniSpecificityRank` deleted, `sniMatchedRank` added with its own doc comment,
matching predicate byte-identical to `sniMatchAny`'s:
`strings.HasPrefix(p, "*.") && strings.HasSuffix(sni, p[1:])`). The five comment-only hunks (`:24-28`,
`:54-58`, `:186-188`, `:196-200`, `:213`) were left untouched, for Task 12.

**Step 2 — re-derive the shift, fix the cites.** `/usr/bin/grep -n '^func alpnMatchAny'
internal/listener/listenerfilter/chainmatch.go` read `:299` after the code hunks — the `+6` shift PLAN.md
§0.5 predicted, confirmed measured, not assumed. Fixing the two live `quic_test.go` cites needed **two
different edits, not one sed**, per the dispatch's Ruling 1: line 952 reads
`// (chainmatch.go:293-302) is a genuine any-of intersection over it.` — the brief's
`s/chainmatch\.go:293-302/chainmatch.go:299-308/` matches it. Line 1033 reads
`// finds no intersection (chainmatch.go:131, 293-302) and filter_chains[0] is` — the substring
`chainmatch.go:293-302` does **not** appear contiguously (the `:131, ` infix breaks it), so that same sed
is silently a no-op on this line. Applied a line-scoped `1033s/293-302/299-308/` instead. Both lines
verified post-edit; `git diff --numstat -- internal/listener/quic_test.go` = **`2 2`**, matching the brief's
expectation exactly.

`git grep -n sniSpecificityRank` after the edit: **no `.go` hit.** Five `.md` hits remain (DECISIONS.md x2,
07.2 PLAN/PROGRESS/REVIEW, 98 BRAINSTORM, 99 BRAINSTORM/PLAN/SPEC), all historical/plan prose, as expected.

**Step 3 — unit file and QUIC arm.**

```sh
go build ./internal/listener/...                     # clean
go test -count=1 -v ./internal/listener/listenerfilter/...   # RC=0, all PASS incl. O1_shared_matched_wildcard_is_ambiguous
go test -count=1 -v -run TestQUICChainSelection ./internal/listener/...   # RC=0, 15 === RUN, 0 FAIL, 0 panic
```

Every row green, including all 19 `TestSelectChainSNILongestMatchedSuffix` subtests and both
`TestQUICChainSelection_TwoWildcardsLongestMatchedSuffixWins` subtests. `O1_shared_matched_wildcard_is_ambiguous`
passed, confirming `SelectChain` returns `ErrAmbiguousChainMatch` on that input. `gofmt -l` empty on both
touched files.

**Step 4.** Committed as `6b02dfdb`.

---

## Task 11: ALL ARMS GREEN — fixture, suite, race, lint

**Step 1 — fixture `0124`, three times.**

```sh
go test -count=1 -v -run 'TestDifferential/0124-listener-sni-longest-suffix' ./test/differential/
```

All three runs: `rc=0`, `=== RUN` count `2` (parent + subtest), `--- SKIP` count `0`, no panic-gate hit.
Run 1's per-row counters matched the PLAN §4 P2 table exactly: `l_long_first`/`l_short_first`/`l_default`
all served `LONG`, `l_mixed` served `Y` for the `mx_y` request as well as `X` for `mx_x` (subject
`lf_long=1, lf_short=1, sf_short=1, sf_long=1, mx_x=2, mx_y=1, df_long=1, df_short=0, df_default=1` — every
one matches `want`, and every one now matches the reference row-for-row, unlike Task 9's tip record).

**Step 2 — five-selector suite.**

```sh
go test -count=1 -v ./cmd/envoy-go/... ./internal/admin/... ./internal/boot/... ./internal/listener/... ./validate/...
```

`rc=0`, **421** `=== RUN`, **0** FAIL-matcher hits, **0** panic-gate hits — matches PLAN §4's "At full P2,
rc 0, 421, 0 RED" exactly.

**Step 3 — race.**

```sh
go test -race -count=1 ./internal/listener/...
```

`rc=0`; `internal/listener`, `internal/listener/listenerfilter`, `internal/listener/listenerfilter/tls_inspector`
all `ok`, no DATA RACE / panic / SIGSEGV.

**Step 4 — lint / format / deps.**

```sh
GOTOOLCHAIN=go1.26.2 golangci-lint run ./...   # rc=0, no output
gofmt -l .                                     # empty
go mod tidy -diff                              # rc=0, empty
git diff master -- go.mod go.sum               # empty
```

All clean; no toolchain/dep drift from this phase's work.

**Step 5.** This section is the record. Committed together with this task's commit (no code files —
"Files: none" per the brief).

---

## Task 12: the occurrence-set reconciliation in CODE — comments, under TWO gates

**Step 1 — apply both.** Applied Appendix D to `internal/listener/quic.go` (the `selectQUICChain` doc
comment, now naming both `ErrNoChainMatched` and `ErrAmbiguousChainMatch` as nil-return causes). Applied
four of Appendix A's five comment-only hunks to `chainmatch.go`: the `ServerNames` field comment (`:24-28`),
the `ErrAmbiguousChainMatch` doc comment (`:54-58`), and the `breakTie` doc paragraph (`:186-188` +
`:196-200`, hand-applied as one contiguous edit since they share unchanged context between them).

**⚠️ Deviation found, recorded, not bent:** the fifth hunk the dispatch's Ruling 2 assigns to Task 12 —
`:213`, the `// Slot 2 — ServerNames: SNI specificity...` → `// Slot 2 — ServerNames: rank of the MATCHED
pattern...` comment line — is **not** a separate hunk in Appendix A's actual diff. It is textually
interleaved inside the same `@@ -210,16 +210,22 @@` hunk as the slot-2 CODE change (the `sniSpecificityRank`
→ `sniMatchedRank` call-site rewrite with the `la`/`lb` tie-break), which Ruling 2 assigns to Task 10 ("slot
2 in breakTie"). Task 10 was applied by hand-matching Appendix A's post-image text against the live file
rather than a mechanical `git apply` of a pre-split hunk, and that comment line's new text was carried over
together with the code around it — so it landed in the Task 10 commit (`6b02dfdb`), not here.

Measured, not assumed: `git diff 64db8131 -- internal/listener/listenerfilter/chainmatch.go --numstat` =
**`42 39`** (confirmed matches Appendix A's whole-file total regardless of which task each line landed in).
`git diff 64db8131 HEAD -- chainmatch.go --numstat` (Task 10 alone, committed) = `27 24` — the extra `9`
added / `10` removed beyond the "code +18 -14" figure in PLAN.md §0.5 is exactly `sniMatchedRank`'s own doc
comment (explicitly Task 10's per the brief, "with its own doc comment") plus this one misplaced `:213`
line. The working-tree-vs-Task-11-commit diff for chainmatch.go (Task 12's actual delta) is `15 15` — four
hunks, not five — because the fifth already landed upstream of Task 11. This does not affect final
correctness (the `42 39` total is intact) and does not violate any Task 12 REQUIRED numeric gate (the brief
only requires rc 0 + a neutral count for chainmatch.go's gate, unlike quic.go's fixed `inspected 8 …
neutral +4 -4`).

**Step 2 — gate `quic.go`.**

```sh
gate 9bf2fc42 internal/listener/quic.go
# GATE: comment-only -- inspected 8 changed line(s), 0 violation(s); neutral +4 -4
# rc=0
```

Matches the brief's expectation exactly (RANGE = `9bf2fc42` = the Task 11 commit; measured against the
working tree, ahead of Task 12's own commit — see the note below).

**Step 3 — gate `chainmatch.go`, same RANGE.**

```sh
gate 9bf2fc42 internal/listener/listenerfilter/chainmatch.go
# GATE: comment-only -- inspected 30 changed line(s), 0 violation(s); neutral +15 -15
# rc=0
```

rc 0, neutral (`+15 -15`), as required. The `30`/`15`/`15` figures are lower than a hypothetical "all 5
hunks" run would show (`32`/`16`/`16`) for the reason recorded at Step 1 above: this commit's chainmatch.go
diff carries only four of Appendix A's five comment hunks, the fifth having already landed in Task 10's
commit. The gate still correctly reports the commit as comment-only and line-neutral, which is the
property it exists to check.

**Hazard question, answered for `chainmatch.go` (PLAN.md §6 asks explicitly, since phase-98's PLAN answered
it with no hits and this file needed its own check):**

```sh
/usr/bin/grep -n -- '"[^"]*//' internal/listener/listenerfilter/chainmatch.go   # rc=1, no output
```

The backtick form (`` `[^`]*// ``) is also rc 1, no output. The positive control — the same pattern over
the rest of the package (`*.go` in `internal/listener/listenerfilter/`) — hits
`chainmatch_test.go:39,118` and `fuzz_test.go:13-16` (string/backtick literals containing `//` before a
trailing Go comment). **A line-prefix matcher is therefore safe for `chainmatch.go`.**

**Step 4 — re-verify every live `chainmatch.go:<N>` cite in `quic_test.go`.**

```sh
git grep -nE 'chainmatch\.go:[0-9]+' -- internal/ cmd/ test/ | /usr/bin/grep -v '/phases/'
```

18 live cite lines, all in `quic_test.go` (444, 446, 491, 564, 666, 680, 721, 748, 798, 834, 846, 886, 940,
952, 998, 1033, 1088, 1323), citing target lines `87-92`, `119`, `125`, `128`, `131`, and (only at 952 and
1033) `299-308`. Read each cited line/range directly against the current file: `87-92` is the
`ErrNoChainMatched` fallback block; `119` is the `DestinationPort` `matches()` check; `125` is the
`ServerNames`/`sniMatchAny` check; `128` is `TransportProtocol`; `131` is `ApplicationProtocols`/
`alpnMatchAny`; `299-308` is `alpnMatchAny`'s full body. Every one still carries the text its cite names —
none of these lines sit at or below the Task 10 shift point except the two already fixed there.

**Step 5 — run the listener packages, commit.**

```sh
go test -count=1 -v ./internal/listener/...
```

rc=0, **266** `=== RUN`, 0 FAIL, 0 panic. Committed.

---

## Task 13: NC roster rows 1-5 — built as patches in a throwaway worktree, scored PER ARM by name

Isolation: `git -C /home/esa/git/envoy-go-wt-99-impl worktree add --detach /home/esa/git/envoy-go-wt-99-nc
363880b9` (Task-12 tip; `wt-99-impl` untouched throughout — a reviewer was reading it concurrently). Every
mutant applied there, scored, and reverted with `git checkout -- .`; `git status --porcelain` confirmed
clean after each row. Worktree removed clean at the end (`git worktree remove` with no force needed).

**Row 1 — `inv` (Appendix F.1, suffix-length compares reversed).** `go vet ./internal/listener/...` rc 0.

`go test -count=1 -v -run 'TestSelectChainSNILongestMatchedSuffix|TestSelectChainAmbiguousReturnsError'
./internal/listener/listenerfilter/`: rc 1. FAIL (13): `C0_long_declared_first_longer_suffix_wins`,
`C0_short_declared_first_longer_suffix_wins`, `B_desc_three_levels_deepest_suffix_wins`,
`B_desc_middle_suffix_beats_shortest`, `B_asc_three_levels_deepest_suffix_wins`,
`B_asc_middle_suffix_beats_shortest`, `M1_rank_of_matched_pattern_not_whole_set_X_first`,
`M2_rank_of_matched_pattern_not_whole_set_Y_first`,
`M3_longest_matching_member_not_longest_member_X2_first`,
`M4_longest_matching_member_not_longest_member_Y_first`, `M3_deeper_member_of_mixed_set_wins`,
`M4_deeper_member_of_mixed_set_wins`, `E_default_present_two_wildcards_resolve_not_default`. PASS (6):
`C0_matched_negative_only_short_matches`, `M1_exact_member_of_mixed_set_still_wins_its_name`,
`E_default_serves_only_no_match`, `R_exact_beats_wildcard_exact_first`,
`R_exact_beats_wildcard_wildcard_first`, `O1_shared_matched_wildcard_is_ambiguous`.
`TestSelectChainAmbiguousReturnsError` PASS.

`go test -count=1 -v -run 'TestQUICChainSelection_TwoWildcardsLongestMatchedSuffixWins'
./internal/listener/`: rc 1. FAIL both: `LONG_declared_first`, `SHORT_declared_first`.

Matches §7 row 1 exactly (all 13 precedence rows + QUIC both reddened; `R_*`, the 3 eligibility rows, O1,
and the ambiguous-error test stayed green).

**Fixture `0124`** (Docker was this session's alone; no pre-existing containers touched):
`go test -count=1 -v -run 'TestDifferential/0124-listener-sni-longest-suffix' ./test/differential/`: rc 1,
`=== RUN`=2, 0 SKIP. Subject: `l_long_first` a.b.foo.test served SHORT (want LONG, **reddened**);
`l_short_first` a.b.foo.test served SHORT (want LONG, **reddened**); `l_default` a.b.foo.test served SHORT
(want LONG, **reddened**); `l_mixed` a.b.foo.test served X (want Y, **reddened**). All four counter pairs
moved accordingly (`lf_long`=0/`lf_short`=2`, `sf_short`=2/`sf_long`=0, `mx_x`=3/`mx_y`=0,
`df_long`=0/`df_short`=1). Matches §7/§0.8: the three LONG rows **and** `l_mixed` reddened.

**Row 2 — `rank-mut` (Appendix F.2, rank compares reversed).** `go vet` rc 0.

Unit run: rc 1. FAIL (2 only): `R_exact_beats_wildcard_exact_first`, `R_exact_beats_wildcard_wildcard_first`.
Every other subtest PASS, including `M1_rank_of_matched_pattern_not_whole_set_X_first` and
`M2_rank_of_matched_pattern_not_whole_set_Y_first`. `TestSelectChainAmbiguousReturnsError` PASS.

QUIC run: rc 0, both subtests PASS.

Matches §7 row 2 exactly. No fixture run (row 2 is not in the fixture list).

**Row 3 — `P1` whole-set rank + matched length (Appendix F.3), applied to MASTER's `chainmatch.go`.**
`git -C $NC show master:internal/listener/listenerfilter/chainmatch.go >
internal/listener/listenerfilter/chainmatch.go && git -C $NC apply $SCRATCH/nc/p1.diff`. `go vet` rc 0.

Unit run: rc 1. FAIL (3): `M1_rank_of_matched_pattern_not_whole_set_X_first`,
`M2_rank_of_matched_pattern_not_whole_set_Y_first`, `O1_shared_matched_wildcard_is_ambiguous`. Every other
subtest PASS (including both `R_*` and all four `B_*`/`C0_*` rows). `TestSelectChainAmbiguousReturnsError`
PASS.

QUIC run: rc 0, both subtests PASS.

Fixture `0124`: rc 1, `=== RUN`=2, 0 SKIP. Subject: `l_long_first`, `l_short_first`, `l_default` all served
LONG correctly (green); only `l_mixed` a.b.foo.test served X (want Y, **reddened**); `mx_x`=3/`mx_y`=0.

Matches §7 row 3 exactly, including the SPEC-correction at §0.1 (O1 reddens under P1) and the §0.8-adjacent
fixture scope (only `l_mixed`, not the three LONG rows).

**Row 4 — `drop-len` (Appendix F.4, compiling form: `ra, _ :=`/`rb, _ :=`, both length `if` blocks
deleted).** `go vet` rc 0 (confirms §0.2's masked-control finding: the naive delete-only mutant from
`SPEC.md` §11 row 4 does NOT compile — this rewrite is the one that does).

Unit run: rc 1. Same 13-name FAIL set as row 1, byte-identical list (`C0_long/short`, `B_desc/asc`×4,
`M1/M2_rank`, `M3/M4_longest_matching`, `M3/M4_deeper`, `E_default_present`). Same 6-name PASS set as row 1.

QUIC run: rc 1. Both subtests FAIL — same as row 1.

Matches §7 row 4 exactly ("the same set as row 1, except fixture" — no fixture run for this row).

**Row 5 — `un-fix`, the Task 9 tip record (cited, not re-run in full).** Task 9's per-row table (this file,
line ~358) recorded the un-fixed tip: 14 of 19 unit subtests FAIL (the same 13 as row 1, **plus**
`O1_shared_matched_wildcard_is_ambiguous`), both QUIC subtests FAIL, and fixture `0124` reddened the three
LONG rows and `l_mixed` — the tip returns chain A for O1 instead of the ambiguous error. §7 row 5 names only
the O1 row as the row's distinguishing "must redden" entry (mechanism: whole-set rank, same axis as row 3's
O1 correction at §0.1); it carries no "must stay green" column, unlike rows 1-4.

**Cheap optional confirmation done:** checked out unpatched `master:chainmatch.go` (whole-set rank, no P1
hybrid) into the NC worktree, `go vet` rc 0, ran only
`-run 'TestSelectChainSNILongestMatchedSuffix/O1_shared_matched_wildcard_is_ambiguous'`: rc 1, **FAIL** —
confirms O1 reddens against master's original code exactly as Task 9 recorded (the tip returns chain A, not
`ErrAmbiguousChainMatch`). Reverted immediately after.

**Per-arm matrix (row × arm, MEASURED vs §7):**

| arm | row1 inv | row2 rank-mut | row3 P1 | row4 drop-len | row5 un-fix |
|---|---|---|---|---|---|
| `C0_long_declared_first_…` | FAIL | PASS | PASS | FAIL | FAIL |
| `C0_short_declared_first_…` | FAIL | PASS | PASS | FAIL | FAIL |
| `C0_matched_negative_…` | PASS | PASS | PASS | PASS | PASS |
| `B_desc_three_levels_…` | FAIL | PASS | PASS | FAIL | FAIL |
| `B_desc_middle_…` | FAIL | PASS | PASS | FAIL | FAIL |
| `B_asc_three_levels_…` | FAIL | PASS | PASS | FAIL | FAIL |
| `B_asc_middle_…` | FAIL | PASS | PASS | FAIL | FAIL |
| `M1_rank_of_matched_pattern_…` | FAIL | PASS | **FAIL** | FAIL | FAIL |
| `M2_rank_of_matched_pattern_…` | FAIL | PASS | **FAIL** | FAIL | FAIL |
| `M1_exact_member_…` | PASS | PASS | PASS | PASS | PASS |
| `M3_longest_matching_member_…X2_first` | FAIL | PASS | PASS | FAIL | FAIL |
| `M4_longest_matching_member_…Y_first` | FAIL | PASS | PASS | FAIL | FAIL |
| `M3_deeper_member_…` | FAIL | PASS | PASS | FAIL | FAIL |
| `M4_deeper_member_…` | FAIL | PASS | PASS | FAIL | FAIL |
| `E_default_present_…` | FAIL | PASS | PASS | FAIL | FAIL |
| `E_default_serves_only_…` | PASS | PASS | PASS | PASS | PASS |
| `R_exact_beats_wildcard_exact_first` | PASS | **FAIL** | PASS | PASS | PASS |
| `R_exact_beats_wildcard_wildcard_first` | PASS | **FAIL** | PASS | PASS | PASS |
| `O1_shared_matched_wildcard_is_ambiguous` | PASS | PASS | **FAIL** | PASS | **FAIL** |
| `TestSelectChainAmbiguousReturnsError` | PASS | PASS | PASS | PASS | (not re-run; cited unchanged) |
| QUIC `LONG_declared_first` | FAIL | PASS | PASS | FAIL | FAIL (cited) |
| QUIC `SHORT_declared_first` | FAIL | PASS | PASS | FAIL | FAIL (cited) |
| fixture `l_long_first`/`l_short_first`/`l_default` a.b | reddened | (n/a) | green | (n/a) | reddened (cited) |
| fixture `l_mixed` a.b | reddened | (n/a) | reddened | (n/a) | reddened (cited) |

Every measured row-by-row set agreed with §7's corrected table (including the two corrections at §0.1/§0.8)
— no variable found, nothing bent.


---

## Task 14: `ADR-0321` completed IN PLACE

Appended `### Decision (landed at the phase-99 IMPL)` (clauses 1-6: matched-pattern rank via
`sniMatchedRank`, suffix-length tiebreak, AMENDS ADR-0081 clause 4 and clause 5, NOTES ADR-0078 clause 9,
the three declared behaviour changes) and `### Consequences (landed at the phase-99 IMPL)` items (a)-(g)
after the retained italic footer. Flipped the status line in place to `> **STATUS: ACCEPTED`, reworded on
the ADR-0320 precedent. No `**Status:**` line, no `---`, no new `## ` heading, no renumber.

Measured (before → after):

| check | before | after |
|---|---|---|
| `sed -n '14866p' DECISIONS.md \| md5sum` (ADR-0231 decoy) | `929719b67c87aa16ac1e406fed7eba6b` | `929719b67c87aa16ac1e406fed7eba6b` |
| `^---$` count | 216 | 216 |
| `^## ADR-` count | 320 | 320 |
| last `^## ADR-` heading | ADR-0321 (:19361) | ADR-0321 (:19361) |
| strict `PROPOSED` guard | hit at :19363 (ADR-0321) | no hit (grep rc=1) |
| `git diff --numstat` | | `105 1` |

The empty guard is a disarmed guard, not a broken matcher: the same matcher run on
`git show HEAD:docs/envoy-go/DECISIONS.md` hits `:19363` (`> **STATUS: PROPOSED — §Context`), and the tail
ADR's status line read directly at `:19363` now begins `> **STATUS: ACCEPTED — §Con`.

---

## Task 15: `BEHAVIOR_CONTRACT.md` — the contract edits and the `+0` ledger entry

Lines located by literal text under `### Chain-match algorithm` (they sat at :4367 / :4368 as the brief
said):
- Step 1: the SNI clause of the `Tie-breakers within dimensions` bullet replaced with the matched-pattern
  rank + longest matching suffix wording (ADR-0321).
- Step 2: the `Final ties` bullet KEPT, with the per-connection sentence APPENDED (an add, per SPEC.md §0.9).
- Step 3: `**Phase 99 — +0, UNCHANGED (…)**` added directly after the phase-98 entry, in its form: nothing
  registered, renamed or retired; the close path increments no stat; the effect is a redistribution across
  existing per-chain counters; `no_filter_chain_match` again deliberately not added. No absolute quoted.
- Step 4: `:4359` (`listener_filters_timeout` "honored") untouched: `sed -n 4359p | md5sum` =
  `0230485b1ad3ddb13dcee599d7734071` before and after.

`git diff --numstat -- docs/envoy-go/BEHAVIOR_CONTRACT.md`: `4 2` (two replaced lines, one entry line, one
blank separator). File 5996 → 5998 lines.

---

## Task 16: `ROADMAP.md` — row 99 → `done`, under the field-count gate

Row located by ID (`awk -F'|' '/^\| *99 /{print NR}'` → `161`). Status `in-progress` → `done`; the
BRAINSTORM text kept and the SPEC/PLAN/IMPL summary appended to the cell. Before installing, on the
candidate line: `awk -F'|' '{print NF}'` = **8**, `sed 's/\\|//g' | awk -F'|' '{print NF}'` = **8**; zero
hits for `deferred candidates:`, `remaining deferred (not-yet-chartered) candidates:`, `-family row` and
`||`. `git diff --numstat -- docs/envoy-go/ROADMAP.md` = `1 1`, single hunk `@@ -161 +161 @@`.

Sentinel script: `$SCRATCH/t16-sentinel.sh` (check (1) awk verbatim; NC-A substitution inspected before
use; NC-B = check (1) at `want=130` on the real file; checks (2) and (3) verbatim; per-line md5 with the
trailing newline included).

**BEFORE the flip:**

```
== wc -l: 249
== check (1) want=131:
NOT DONE: row 99
== NC-A inspect:
NC LANDED? [ in-progress ]
== NC-A check (1) want=131 on nc.md:
NOT DONE: row 62
NOT DONE: row 99
== NC-B check (1) want=130:
NOT DONE: row 99
GATE FAIL: examined 131 data rows, expected 130
== check (2):
209:remaining deferred (not-yet-chartered) candidates:
215:remaining deferred (not-yet-chartered) candidates:
221:remaining deferred (not-yet-chartered) candidates:
231:remaining deferred (not-yet-chartered) candidates:
237:remaining deferred (not-yet-chartered) candidates:
245:deferred candidates:
== check (3):
== windows md5:
209 10d7807bf02d
215 4a92f7e62fc6
221 2a7eb298b9fd
231 242e53c6f7a3
237 b2680e6f4fbf
245 6caa1c3ce0e7
```

**AFTER the flip:**

```
== wc -l: 249
== check (1) want=131:
== NC-A inspect:
NC LANDED? [ in-progress ]
== NC-A check (1) want=131 on nc.md:
NOT DONE: row 62
== NC-B check (1) want=130:
GATE FAIL: examined 131 data rows, expected 130
== check (2):
209:remaining deferred (not-yet-chartered) candidates:
215:remaining deferred (not-yet-chartered) candidates:
221:remaining deferred (not-yet-chartered) candidates:
231:remaining deferred (not-yet-chartered) candidates:
237:remaining deferred (not-yet-chartered) candidates:
245:deferred candidates:
== check (3):
== windows md5:
209 10d7807bf02d
215 4a92f7e62fc6
221 2a7eb298b9fd
231 242e53c6f7a3
237 b2680e6f4fbf
245 6caa1c3ce0e7
```

Every shape changed as expected: check (1) ONE → SILENT; NC-A TWO → ONE (`NOT DONE: row 62`); NC-B TWO →
ONE (`GATE FAIL: examined 131 data rows, expected 130`). Check (2) SIX before and after at
`:209 :215 :221 :231 :237 :245`; check (3) SILENT before and after. The six windows' md5s are byte-identical
before and after and equal the PLAN §2.1 values. `ROADMAP.md` stays **249** lines; row 99 NF = 8 under
both forms after install.

---

## Task 17: The byte-untouched roster, the ARM roster, and the SIX-GATE sweep

Base ref used: `515856da` (== `master`, confirmed `git rev-parse master` == `git rev-parse 515856da`).
Worktree: `wt-phase-99-impl` at `948f00f3` (Task 16 tip).

### Step 1: Byte-untouched roster (`git diff 515856da --numstat -- <path>`, each expected EMPTY)

```
internal/listener/manager.go                              -> EMPTY
internal/listener/listenerfilter/types.go                  -> EMPTY
internal/listener/listenerfilter/tls_inspector/             -> EMPTY
internal/filter/http/compressor/compressor.go               -> EMPTY
go.mod                                                      -> EMPTY
go.sum                                                      -> EMPTY
.github/                                                    -> EMPTY
test/fixtures/ (git diff --numstat | grep -v '0124-')       -> EMPTY
```

All eight EMPTY. Roster holds.

### Step 2: ARM roster

`git diff 515856da -- internal/ test/ | /usr/bin/grep -E '^\+func (Test|Fuzz)' | sort`:

```
+func TestEncodeData_LevelMapping_LevelReachesEncoder(t *testing.T) {
+func TestQUICChainSelection_TwoWildcardsLongestMatchedSuffixWins(t *testing.T) {
+func TestSelectChainSNILongestMatchedSuffix(t *testing.T) {
```

`^-func (Test|Fuzz)` line: exactly one —
`-func TestEncodeData_LevelMapping_DifferentGzippedSizes(t *testing.T) {`

Both match the brief verbatim.

### Step 3: Gate (a) — full differential, `-count=1 -v`

`cd $W && go test -count=1 -v ./test/differential/ > $SCRATCH/gate_a.txt 2>&1` — 16:15:41 → 16:23:39 EDT
(≈478s / 7m58s). `RC=0`.

```
--- PASS: TestDifferential/*  count = 126
--- FAIL: TestDifferential/*  count = 0
--- SKIP: TestDifferential/*  count = 0
FAIL matcher (^(FAIL|--- FAIL)|^ *--- FAIL): no match
panic gate (^panic:|DATA RACE|SIGSEGV): no match
tail: PASS / ok  github.com/pgdad/envoy-go/test/differential 477.831s
```

Fixture-set reconciliation BY NAME, both `comm` directions, against `ls -d test/fixtures/*/`:
`comm -23` (in gate_a, not in fixtures) EMPTY; `comm -13` (in fixtures, not in gate_a) EMPTY. 126/126
exact match. No driver-owned receiver-port-bind abort observed on this run — single clean pass, no rerun
needed.

**Result: 126 PASS / 0 FAIL / 0 SKIP — matches expectation exactly.**

### Step 4: Gate (b) — non-Docker sweep, ambient toolchain

Ambient toolchain confirmed: `go version` → `go1.27.1 linux/amd64` (`/snap/bin/go`).

`go list ./... | /usr/bin/grep -vE '/test/differential$|/test/conformance/h2spec$'` → **241** packages
(not the brief's expected 240). `go test -count=1 $(cat $SCRATCH/pkgs_b.txt) > $SCRATCH/gate_b.txt 2>&1;
RC=${PIPESTATUS[0]}` → **RC=1**.

**Denominator disagreement (recorded, not bent):** base (`515856da`, checked directly in the `master`
checkout) has **239** packages under the same filter. `comm` between base and tip package lists shows
exactly two new packages, both under `0124`:
`test/fixtures/0124-listener-sni-longest-suffix/driver` and
`test/fixtures/0124-listener-sni-longest-suffix/pki/gen` — nothing removed. So the true rise is
**239 → 241 (+2)**, not the brief's **239 → 240 (+1)**. The variable: `0124` is a TLS fixture and follows
the same two-package shape already present for `0002`, `0004`, `0045` and `0121` (each has both a
`driver` package and a `pki/gen` package); the brief's expected figure counted only the driver as new and
missed the `pki/gen` helper package that TLS fixtures also register.

**Test failure:** one package FAILed —

```
--- FAIL: TestSDSEndToEnd_FetchFailure_BootFailsClosed (0.21s)
    --- FAIL: TestSDSEndToEnd_FetchFailure_BootFailsClosed/silent_SDS_server:_validation_context_fetch_times_out,_boot_fails (0.20s)
        boot_sds_e2e_test.go:551: boot error = "listener: \"l_tls_e2e\": filter_chains[0]: tls: downstream: SDS validation secret \"validation_ca\": xds: sds: recv response: rpc error: code = DeadlineExceeded desc = context deadline exceeded", want it to mention the initial-fetch timeout
FAIL
FAIL	github.com/pgdad/envoy-go/internal/boot	0.422s
```

This is the memory-indexed **SDS dial-budget** known flake. Set reconciliation on the full sweep: **124
`ok`** + **1 `FAIL`** (`internal/boot`) + **116 `[no test files]`** = **241**, matching `pkgs_b.txt`
exactly (every package in the SET accounted for as ok / FAIL / no-test-files).

Rerun `internal/boot`'s `TestSDSEndToEnd_FetchFailure_BootFailsClosed` alone, 3×
(`go test -count=1 -v -run TestSDSEndToEnd_FetchFailure_BootFailsClosed ./internal/boot/...`):

```
RUN 1: RC=0, PASS (0.21s / subtests 0.20s + 0.00s)
RUN 2: RC=0, PASS (0.21s / subtests 0.20s + 0.00s)
RUN 3: RC=0, PASS (0.21s / subtests 0.21s + 0.00s)
```

All 3 isolated reruns PASS. Per the evidence-discipline rule, a green rerun clears nothing: this is the
known SDS dial-budget flake reproducing under full-package concurrent load and passing in isolation, not
a code defect introduced by this IMPL (the failing test and the touched IMPL files do not intersect — SDS
boot-fail-closed vs. chain-match SNI longest-suffix).

**Result: gate (b) is RED on this run** (241 packages seen vs. 240 expected; 1 FAIL, not GREEN as the
plan calls for). The FAIL is the known SDS dial-budget flake (3/3 clean in isolated rerun); the package
count disagreement is a plan-arithmetic gap (0124 registers two new packages, not one), not a regression.

### Step 5: Gates (c)-(e)

**Gate (c), h2spec**, `go test -count=1 -v ./test/conformance/h2spec/` — RC=0, 2.978s. Summary line:
`95 tests, 94 passed, 1 skipped, 0 failed` — **exact match**.

**Gate (d), fuzzers.** `git grep -c '^func Fuzz' -- '*.go'`: at tip — **48 files / 56 targets**; at base
`515856da` — **48 files / 56 targets**. **+0 vs base — exact match.**

**Gate (e), panic gate + lint.**
Panic gate (`^panic:|DATA RACE|SIGSEGV`) over `gate_a.txt`, `gate_b.txt`, `gate_c.txt`: **0 matches in
all three** (grep rc=1 in each, i.e. no hits).
`GOTOOLCHAIN=go1.26.2 golangci-lint run ./...` → **RC=0, 0 lines of output** (`golangci-lint version` under
the same env confirms `v1.64.8 built with go1.26.2`). Exact match.

### Step 6: Gate (f)

`find docs/envoy-go/phases/99-chain-match-sni-longest-suffix -iname 'REVIEW.md'` → no hits. **No
`REVIEW.md` exists — standing departure, as noted in every prior task's report. Not claiming compliance.**

### Summary

| gate | expected | actual | verdict |
|---|---|---|---|
| byte-untouched roster (8 paths) | all EMPTY | all EMPTY | GREEN |
| ARM roster | 3 `+func`, 1 `-func`, verbatim | exact match | GREEN |
| (a) full differential | 126/0/0 | 126/0/0, fixture set matches BY NAME both directions, no port-race abort | GREEN |
| (b) non-Docker sweep | 240 pkgs, GREEN | sweep 1 RED (flake), sweep 2 GREEN | sweep 1 **RED** (flake), sweep 2 **GREEN** (DONE_WITH_CONCERNS) |
| (c) h2spec | 95/94/1/0 | 95/94/1/0 | GREEN |
| (d) fuzzers | 56/48, +0 | 56/48, +0 | GREEN |
| (e) panic gate + lint | 0 panics, lint rc 0 no output | 0 panics (a,b,c), lint rc 0 no output | GREEN |
| (f) REVIEW.md | absent (standing departure) | absent, confirmed | departure noted |

Overall: **DONE_WITH_CONCERNS.** Two findings, neither touching the phase-99 diff: (1) gate (b)'s package
denominator is 241 not 240 — an arithmetic gap in the plan's expectation, not a regression, since `0124`
following the established TLS-fixture two-package shape (driver + pki/gen) was foreseeable from precedent
but not accounted for; (2) `internal/boot`'s `TestSDSEndToEnd_FetchFailure_BootFailsClosed` FAILed once
under the full concurrent sweep and PASSed 3/3 in isolation — the memory-indexed SDS dial-budget flake,
unrelated to the SNI-longest-suffix chain-match code this phase touches. No code was changed to react to
either finding, per Task 17's charter (runs and records only).

---

## Task 17 addendum — gate (b) sweep 2 (controller)

Command: `go test -count=1 $(cat $SCRATCH/pkgs_b.txt) > $SCRATCH/gate_b2.txt 2>&1`, run under go1.27.1
(ambient). RC=0.

Verified directly from `$SCRATCH/gate_b2.txt`
(`/tmp/claude-1000/-home-esa-git-envoy-go/a8eda3e5-4746-48dc-bfbd-713f326de1a1/scratchpad/p99/gate_b2.txt`):
`wc -l` = 241 lines; `^ok` count = 125; `[no test files]` count = 116 (125 + 116 = 241, matching the line
count exactly); FAIL matcher (`^(FAIL|--- FAIL)|^ *--- FAIL`) = no match (grep rc=1); panic gate
(`^panic:|DATA RACE|SIGSEGV`) = no match (grep rc=1).

Rulings:
(a) The denominator is 241, not 240, because `0124` adds TWO packages
(`test/fixtures/0124-listener-sni-longest-suffix/driver` and
`test/fixtures/0124-listener-sni-longest-suffix/pki/gen`), not one — the same TLS-fixture two-package
shape already present for `0002`, `0004`, `0045` and `0121`.
(b) Gate (b) is recorded GREEN on sweep 2. Sweep 1's `TestSDSEndToEnd_FetchFailure_BootFailsClosed`
failure is a RECURRENCE of a router-listed flake that sweep 2's clean rerun does NOT clear FROM THE
RECORD: sweep 2 coming back green does not retroactively erase sweep 1's observed failure, so both
sweeps stand side by side rather than sweep 2 superseding sweep 1. `internal/boot` does depend on
`listenerfilter`, but the failing arm is an SDS fetch timeout at boot before any chain selection runs;
the chain-match SNI-longest-suffix code this phase touches is not on that failure's path.

Gate (b) verdict: **sweep 1 RED (flake), sweep 2 GREEN** — sweep 1 is not erased or superseded; both
sweeps are part of the record.

---

## Task 18: close-out — `STATE.md` rolled, archive +1, router rolled to a self-pick BRAINSTORM

Base: `31486188` (the final-review fix wave). Files: `STATE.md`, `STATE_HISTORY.md`, `next-prompt.txt`, this file.

**Sentinel, all four NCs and the check-(2) positive control**, commands verbatim from `next-prompt.txt`, `/usr/bin/grep`,
run in the publishing tree (ACTUAL output):

```
== (1)
== (2)
209:remaining deferred (not-yet-chartered) candidates:
215:remaining deferred (not-yet-chartered) candidates:
221:remaining deferred (not-yet-chartered) candidates:
231:remaining deferred (not-yet-chartered) candidates:
237:remaining deferred (not-yet-chartered) candidates:
245:deferred candidates:
== (3)
== NC-A
NC LANDED? [ in-progress ]
NOT DONE: row 62
== NC-B (want=130)
GATE FAIL: examined 131 data rows, expected 130
== NC-C
0
NEVER OPENED: gRPC   <- NC FIRED
== NC-D (occurrences / lines, with --)
96
68
== check-(2) positive control (residual / substitutions)
0
6
== md5 (trailing NL incl)
209 10d7807bf02d
215 4a92f7e62fc6
221 2a7eb298b9fd
231 242e53c6f7a3
237 b2680e6f4fbf
245 6caa1c3ce0e7
== malformed (escape-aware: line id NF)
119  57  9
131  69  10
== ROADMAP lines
249
== stop
ls: cannot access '/home/esa/git/envoy-go/stop': No such file or directory
ls: cannot access '/home/esa/git/envoy-go-wt-99-impl/stop': No such file or directory
```

⇒ the sentinel does NOT fire; `stop` NOT created. Shapes identical to the controller's post-flip run at Task 16.

**Eviction.** Histogram of §Current + §Recent before the roll (`grep -oE` over the `active-phase`/`prior active-phase`
labels, `uniq -c`): **3 × `2026-09-21`, 2 × `2026-09-20`, 1 × `2026-09-16`**. That includes the entry this close promoted
(the phase-99 PLAN). The tail is UNIQUE, so the date read picked the evictee alone: the phase-98 SPEC entry
(`2026-09-16`). LABEL-BOUND PAIR (`grep -cF -- '<label>'`, backticked label):

| label | `STATE.md` before -> after | `STATE_HISTORY.md` before -> after |
|---|---|---|
| evictee (phase-98 SPEC) | 1 -> 0 | 0 -> 1 |
| fabricated (`… SPECTRE done`) NC | 0 -> 0 | 0 -> 0 |
| positive control (phase-97 IMPL, archived) | 0 -> 0 | present -> present |

**Archive guard** (house anchored forms): strict `163 -> 163` (DELTA 0), parenthetical `79 -> 80`, loose `242 -> 243`;
`STATE_HISTORY.md` `584 -> 586` (`wc -l`), `git diff --numstat` `2 0`. `STATE.md` `66 -> 66`, `10 10`.

**Router roll.** `next-prompt.txt` now points at a phase-100 self-pick BRAINSTORM (state DONE -> 1). The spent
imperatives found by grep and resolved are listed in the task report. The figures moved: gate (b) `240` -> `241`
everywhere it was a live figure, fixtures 126, check (1) SILENT, NC-A/NC-B ONE, ADR-0321 ACCEPTED, and lint RUNS. The
toolchain fold-in is marked DISCHARGED once, in the six-gate bullet; its banked bullet is gone. The order-dependent fold
bullet is sharpened IN PLACE with the SNI-slot triple and the `ErrAmbiguousChainMatch` doc-comment caveat, so there is no
duplicate. New banked items: the uncovered `rank > 2` guard, and the SDS dial-budget flake recurrence. Method note 2 now
reads `99 IMPL eight`. Two stale figures were also corrected: method note 22's phase-94 path count (`29` -> `28` by
`git show --numstat --format= 0a985a35 | wc -l`) and the port-band note's `28` distinct `15xxx` literals (`38` now, `35`
at `515856da`). The memory-slug audit found 12 distinct slugs, all resolved.

**Line counts quoted in this commit** (`wc -l`, after the final edit): see the task report. The phase-99 docs are
`BRAINSTORM.md` 537, `SPEC.md` 697 and `PLAN.md` 2414. `DECISIONS.md` is 19487 (`^---$` 216, `^## ADR-` 320, bare `^## `
328, tail ADR-0321, next-free ADR-0322). `BEHAVIOR_CONTRACT.md` is 5998, `ROADMAP.md` 249, phase dirs 140, fixtures
126 = 126 (102 driver + 24 inputs).

## Task 19: squash, merge, push, worktree removal — done by the controller, not recorded here.
