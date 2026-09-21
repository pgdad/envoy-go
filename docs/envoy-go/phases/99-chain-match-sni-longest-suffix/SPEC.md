# Phase 99 — `chain-match-sni-longest-suffix` — SPEC

**Stage:** SPEC (lifecycle **1 -> 2**). Worktree `/home/esa/git/envoy-go-wt-99-spec` off master `c9b9ecfc`,
branch `wt-phase-99-spec`. **Governs:** `BRAINSTORM.md` (537 lines) — read for evidence, re-derived before
trusted.

**The decision in one paragraph.** Among eligible chains that tie on the specificity bitmask, `breakTie`
slot 2 will rank each chain by **the `server_names` pattern that MATCHED the SNI** — exact beats any
wildcard, and between wildcards the **longest matching suffix** wins — instead of by the best pattern in
the chain's whole set. This is the rule the pinned reference follows, **measured at this stage on TCP
AND QUIC, in both declaration orders**, including two arms that separate it from the alternatives.
**The BRAINSTORM's `22 0` prototype (P1) is REFUTED as the repair**: it keeps whole-set RANK and would
still serve the wrong chain for a mixed exact+wildcard set (arm M1/M2), which the reference serves the
other way. The chosen shape (P2) is one production file, `internal/listener/listenerfilter/chainmatch.go`,
**`26 23`** by `git diff --numstat`. It was built, driven end to end on both transports, and reverted.
It agrees with the reference on every arm both sides ran. It adds, removes and renames **no stat name
(+0, confirmed)**. It **declares one served→closed change** on a config class that the reference
**refuses to boot** (§4.3). `ADR-0321` §Context is drafted and fixture `0124` is chartered.
The toolchain fold-in is chartered too (§10).

---

## 0. What this stage refuted — by execution

Every item below was produced by running something, almost all of it by this stage's four agents. **One
of them refuted a premise in the brief the controller gave it (§0.5)**, and **the controller's own first
draft of §11 was wrong** — it listed `M1/M2` as rank-mutant rows; under P2 both of M1's chains match
through a wildcard, so they tie on rank and LENGTH decides (corrected before publishing; method note 76).

### 0.1 🔴 THE BRAINSTORM's COST FLOOR `22 0` IS THE WRONG REPAIR — THE REFERENCE REFUTES IT ON ARM M1/M2

`BRAINSTORM.md` §6 prototyped P1: keep `sniSpecificityRank` over the chain's WHOLE pattern set, and
compare suffix length only when both chains rank 1. §0.11 inferred, unmeasured, that this would pick
X=`["x.test","*.foo.test"]` over Y=`["*.b.foo.test"]` for SNI `a.b.foo.test`. **Measured at this stage:**

| side | M1 (X declared first) | M2 (Y declared first) |
|---|---|---|
| reference (by digest `7edd5b0fd763…`) | **Y** | **Y** |
| subject @ `c9b9ecfc` | X | X |
| subject + P1 (`22 0`) | **X** — still wrong | **X** |
| subject + P2 (`26 23`, §4.1) | **Y** | **Y** |

The matched negatives were run on the same config, on both sides. `x.test` served X, and `q.foo.test`
served X through its wildcard. **Both chains are live**, so the reference's Y is a precedence answer,
not an ineligibility. ⇒ **method note 37 fired a fourth row running:** the prototype's shape encoded an
unmeasured parity answer, and it was the wrong one.

### 0.2 ⚠️ *"THE SHAPE QUESTION IS 'WHOLE-SET RANK + LENGTH' vs 'RANK OF THE MATCHED PATTERN'"* — HALF RIGHT

`BRAINSTORM.md` §3.1(a) framed the choice as whole-set vs matched on BOTH axes. P1's
`longestWildcardSuffix` already took the length **only from patterns that match**. So on the LENGTH axis
P1 was already matched-only, and it gets arm M3/M4 right: X2=`["*.foo.test","*.c.b.foo.test"]` vs Y,
SNI `a.b.foo.test` → Y on the reference, on P1 and on P2. **The divergence between P1 and P2 lives only
on the RANK axis** (exact vs wildcard inside one chain's set). M3/M4 is still chartered: it is the arm
that separates "longest MATCHING member" from "longest member". The unpatched tip CLOSES on it (§3.1).

### 0.3 🔴 P2 IS NOT PURELY ADDITIVE — IT CLOSES A SHAPE THE TIP USED TO SERVE (and the reference refuses to boot it)

The subject agent ran the case left over after the repair. It found configs that serve at the tip and
close under P2:

| config | tip `a.b.foo.test` / `q.foo.test` | P2 |
|---|---|---|
| O1 A=`["*.foo.test","q.test"]`, B=`["*.foo.test"]` | A / A | **CLOSED / CLOSED** |
| O2 = O1 + C=`["*.b.foo.test"]` | A / A | **CLOSED / CLOSED** (§0.4 masks C) |

At the tip, A wins because its WHOLE set ranks 0 through `q.test`. That is the M1 defect working in a
direction that happens to serve. Under P2 both chains match through the identical `*.foo.test`, so they tie
honestly and the connection closes. **The reference refuses this class outright.** Arm R1 used
X=`["x.test","*.b.foo.test"]` with Y=`["*.b.foo.test"]`, the O1 shape with one pattern shared across a
mixed set. It failed `--mode validate` rc=1 with `multiple filter chains with overlapping matching rules are defined`.
So neither the tip nor P2 matches the reference on O1. Both boot a config the reference rejects. **The
repair moves the subject from "serves an arbitrary chain" to "closes" on a config that should never
have booted.** That is declared in `ADR-0321` §Consequences, not hidden. Making it boot-reject is the
banked duplicate-matcher reject parity (`BRAINSTORM.md` §4.5), **not this row**.

### 0.4 ⚠️ NEW: THE PAIRWISE FOLD IN `SelectChain` IS ORDER-DEPENDENT — A TIE CAN MASK A STRICTLY BETTER LATER CHAIN

`SelectChain` folds eligible chains left to right and returns `ErrAmbiguousChainMatch` on the FIRST nil
`breakTie`. That can happen before a later chain that outranks both is ever compared. Subject arms,
all `-mode validate` rc=0 and booted:

| config | tip | P2 |
|---|---|---|
| F1 A{`application_protocols:[h2]`}, B{`[http/1.1]`}, C{`["*.foo.test"]`} — client offers both | **CLOSED** | **CLOSED** |
| F2 = F1 declared C, A, B | C | C |
| F3 C0's LONG, SHORT, then C{`destination_port`, `*.foo.test`} | **CLOSED** (a.b) | C |
| F4 = F3 declared C first | C | C |

C carries the `server_names` bit, which outranks the `application_protocols` bit, so it should win F1
outright. It is never reached. **This is independent of slot 2.** P2 cures F3 only because it resolves
the A-vs-B pair; F1 is an ALPN tie and P2 does not touch it. **Subject-side only; the reference was not
driven on F1.** ⇒ **BANKED, not folded in** (§4.2): the repair is a different function body with a
different arm set, and folding it would widen this row past its measured evidence.

### 0.5 ⚠️ *"`manager.go:chainSpecificityRank`"* DOES NOT EXIST — two production comments cite a deleted symbol

The controller's brief to the occurrence agent asked what uses `manager.go`'s `chainSpecificityRank`. The
agent refuted it: the symbol was **deleted in `ee45f35f` at phase 07.2**, as ADR-0078 clause 9 itself
records. `chainmatch.go:25-28` (*"Re-uses the existing internal/listener/manager.go:chainSpecificityRank
logic"*) and `chainmatch.go:304` (*"mirrors internal/listener/manager.go:chainSpecificityRank"*) are
**already stale at the tip**. P2 deletes the `:304` comment with the function it annotates, and the
`:25-28` field comment is on the §6 edit roster.

### 0.6 🔴 THE QUIC PATH HAS THE SAME DEFECT, AND THE REFERENCE's QUIC ORDERING IS THE SAME — owed item 2 DISCHARGED BY MEASUREMENT, NOT DEFERRAL

`BRAINSTORM.md` §2.3 measured TCP only. The reference was driven with a Go HTTP/3 client on the
`go.mod`-pinned quic-go, every response `proto=HTTP/3.0 status=222`. Both orders of C0 served **LONG** for
`a.b.foo.test` and **SHORT** for `x.foo.test`. Both orders of M1 served **Y**. **The subject at the tip,
same shapes over QUIC: `a.b.foo.test` → `H3 error (0x0)` plus the `ambiguous` log line, in both orders.**
Under P2 it serves LONG and Y. Under the inverted P2 it serves SHORT. **So the shared `SelectChain`
change is parity-correct on QUIC, and QUIC is measurably broken at the tip, not merely exposed.**

### 0.7 ⚠️ THE REFERENCE's QUIC NO-MATCH DOES NOT BOOK `no_filter_chain_match`

On TCP a no-match SNI closes and books `no_filter_chain_match` **0 -> 1**. On QUIC the same no-match
(client `context deadline exceeded`) left it at **0**. **Recorded, not explained.** It does not affect this
row, since the subject emits no such counter on either transport. It is a trap for any future QUIC
no-match pin.

### 0.8 ⚠️ THE OCCURRENCE SET GROWS — FIVE SITES THE BRAINSTORM's UNION DID NOT LIST

Every §3.3 floor hit was re-confirmed at the same line. Growth, each read in context:
`chainmatch.go:185-188` (the `breakTie` doc calls the nil return *"a NewManager-time config error"*),
`manager.go:1061-1064` (`findIdenticalChainSpecs` paraphrases the falsified clause 5),
`quic.go:181-183` (calls `selectQUICChain`'s nil return *"SelectChain's (nil, ErrNoChainMatched) branch"*,
which is incomplete since an ambiguous result also returns nil; **found by READING, no matcher hit it**),
`DECISIONS.md:3165` (ADR-0081 Alternative A says Envoy considers all dimensions together, which is false
per the BRAINSTORM's arm G3 and untouched by this row), and `DECISIONS.md:19184` (ADR-0319 (d): a QUIC
`server_names` winner with a different leaf is served under the Start-time certificate — **its REACH
widens** because two-wildcard QUIC shapes that closed will now select). §6 decides each one.

### 0.9 ⚠️ `BEHAVIOR_CONTRACT.md:4368` IS NOT FALSE AS WRITTEN — IT IS FALSE BY IMPLIED COMPLETENESS

The BRAINSTORM (§0.8) called all three normative sites false. `:4368` reads *"Final ties (chains
identical on all 8 dimensions) error at `NewManager`-build time."* That is **true** of structurally
identical chains, which `findIdenticalChainSpecs` does reject. What makes it misleading is its position
as the whole story: runtime ties exist and close connections. `chainmatch.go:54-58` and ADR-0081 clause
5 (*"NOT at per-connection dispatch time"*) are false outright. **The repair differs by site**: `:4368`
gains a sentence, and the other two are corrected (§6).

### 0.10 ⚠️ THE COMPRESSOR TEST WAS BLIND BEFORE IT WAS RED — the fold-in's re-point is strictly STRONGER, not a relaxation

The toolchain agent measured the gzip sizes of the test input (4096 B) under both toolchains. At
go1.26.2, **level 9 and the default produce the same 2124 bytes**, so a level-9 config silently forced
to the default would never have failed the test. A mutation that builds the POOLED writer at the
default level **PASSED the original test under go1.26.2**. The re-pointed assertion (the gzip header's
XFL byte, RFC 1952 §2.3.1) reddens under both mutations on both toolchains (§10). **The fold-in is
not "make the red go away".**

### 0.11 ⚠️ THE INVERTED PATCH CANNOT SCORE EVERY ARM — method note 76, measured

Two chartered arms (`R_exact_beats_wildcard_*`) are GREEN at the un-fixed tip, under P2 AND under the
inverted P2. That is correct, because exact beats wildcard by RANK and the inversion flips only
LENGTH. **A second mutant (the rank compare flipped) was built and reddens exactly those two.** Three
more arms are green everywhere because **only one chain is eligible**, so `breakTie` is never reached.
They are labelled as eligibility checks, not precedence evidence (§5.2).

---

## 1. Scope, restated as a decision

**IN:** slot 2 of `breakTie` ranks the MATCHED pattern (§4.1), on the shared selector, so TCP and QUIC
both change. The three normative sites of the false build-time claim, plus the growth sites of §0.8,
are reconciled (§6). There are unit arms scored per arm (§5), one QUIC wiring arm (§5.3), fixture
`0124` (§7), `ADR-0321` (§8), a `+0` ledger entry (§9), and the toolchain fold-in (§10).

**OUT, and stated in `ADR-0321` §Consequences:**
- nested-descent precedence (`BRAINSTORM.md` §0.12);
- SNI case-folding (§1.3 there);
- the duplicate/overlap boot reject (§0.3 here, §4.5 there);
- the order-dependent tie fold (§0.4, banked);
- a `no_filter_chain_match` counter;
- the `"*"` tier and partial-wildcard acceptance;
- per-connection certificate identity on QUIC.

**Family:** a Listener / chain-match MAINTENANCE row claiming **no family ordinal** (the 85-91 and 95-98
precedent).

---

## 2. The reference, MEASURED — controls FIRST, both transports

**Rig.** Image `envoyproxy/envoy@sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`,
run BY DIGEST after checking `ENVOY_TARGET.md:3-4` and `docker images --digests`. Containers were
`p99spec-ref-*`, one per config, `-p` published, and removed BY NAME. The ports were TCP
`16301`/`16302` and UDP `16311` plus admin `16312`, from band `16300-16349`, censused free. Every
chain's HCM had its own `stat_prefix` and answered `direct_response` status **222** with the chain name
as the body. TCP used `listener_filters: [tls_inspector]` and `curl -sS -k --resolve`. QUIC used
fixture `0122`'s reference listener shape with a QUIC transport socket on every chain. **Every arm
validated rc=0 and reached `/ready` LIVE, except R1 (a validate reject).** Every body was corroborated by
the chain's `http.<stat_prefix>.downstream_rq_total` moving by exactly one.

### 2.1 TCP

| arm | declared order | SNI | body | note |
|---|---|---|---|---|
| **C0** control | LONG `*.b.foo.test`, SHORT `*.foo.test` | a.b.foo.test / x.foo.test | **LONG** / SHORT | BRAINSTORM A1 reproduced |
| C0 | ″ | nomatch.example | curl rc=35 | `no_filter_chain_match` 0→1 |
| C0R | SHORT, LONG | ″ | **LONG** / SHORT | |
| **M1** | X `["x.test","*.foo.test"]`, Y `["*.b.foo.test"]` | a.b.foo.test / x.test / q.foo.test | **Y** / X / X | the RANK discriminator |
| **M2** | Y, X | ″ | **Y** / X / X | |
| **M3** | X2 `["*.foo.test","*.c.b.foo.test"]`, Y | a.b.foo.test / z.c.b.foo.test / q.foo.test | **Y** / X2 / X2 | the LENGTH discriminator |
| **M4** | Y, X2 | ″ | **Y** / X2 / X2 | |
| M5 | E1 `["a.b.foo.test"]`, W `["*.b.foo.test"]` | a.b.foo.test / y.b.foo.test | **E1** / W | exact beats wildcard |
| M5R | W, E1 | ″ | **E1** / W | |
| **R1** | X `["x.test","*.b.foo.test"]`, Y `["*.b.foo.test"]` | — | **validate rc=1** | `multiple filter chains with overlapping matching rules are defined` |

### 2.2 QUIC (owed item 2)

| arm | declared order | SNI | body (`HTTP/3.0`, 222) |
|---|---|---|---|
| **Q0** control | LONG, SHORT | a.b.foo.test / x.foo.test | **LONG** / SHORT |
| Q0 | ″ | nomatch.example | client deadline exceeded; `no_filter_chain_match` stayed **0** (§0.7) |
| Q0R | SHORT, LONG | a.b.foo.test / x.foo.test | **LONG** / SHORT |
| QM1 | X, Y | a.b.foo.test / x.test / q.foo.test | **Y** / X / X |
| QM2 | Y, X | ″ | **Y** / X / X |

### 2.3 The rule, as measured

The reference ranks **the pattern that matched**: exact first, then the longest matching `*.` suffix.
**Declaration order is ruled out** by all six reversal pairs (method note 70). **"The chain's best pattern"
is ruled out on both axes**: on RANK by M1/M2, where X's exact `x.test` did not help, and on LENGTH by
M3/M4, where X2's longer `*.c.b.foo.test` did not help. Every losing chain was shown live through each of
its own patterns.

### 2.4 Arms NOT run

- QUIC M3/M4 and M5. Q0 and QM1 already cover the rule on QUIC in both orders.
- The reference side of §0.4's F1-F4. The fold defect is banked, not chartered.
- The reference side of O1/O2 as literally written. R1 is the same class (one pattern shared across
  chains, one of them a mixed set) and rejects; O1 itself is **inferred** to reject.
- Which certificate was served, since all chains share one leaf. Also mixed-case SNI (banked row, and
  curl lowercases it).

---

## 3. The subject, MEASURED

**Rig.** `go build -o <scratch>` at `c9b9ecfc` and at each prototype, in throwaway detached worktrees
(all removed). Ports `16350-16399`, certs via `filename:`, killed by captured PID. **No Docker.**

### 3.1 TCP, end to end by served body

| arm | tip | P1 | **P2** | inverted P2 | reference |
|---|---|---|---|---|---|
| C0, both orders: a.b / x | **CLOSED** / SHORT | LONG / SHORT | **LONG / SHORT** | **SHORT** / SHORT | LONG / SHORT |
| M1, M2: a.b / x.test / q.foo | **X** / X / X | **X** / X / X | **Y / X / X** | X / X / X | Y / X / X |
| M3, M4: a.b / z.c.b / q.foo | **CLOSED / CLOSED** / X2 | Y / X2 / X2 | **Y / X2 / X2** | X2 / **Y** / X2 | Y / X2 / X2 |
| M5: a.b / y.b | E1 / W | E1 / W | **E1 / W** | E1 / W | E1 / W |
| E (C0 + default): a.b / nomatch | **CLOSED** / DEFAULT | LONG / DEFAULT | **LONG / DEFAULT** | **SHORT** / DEFAULT | LONG / DEFAULT |

**P2 agrees with the reference on every cell.** P1 disagrees on M1/M2 only. The inverted P2 serves
SHORT end to end, so the mutation is **live** (method note 50). Every config passed `-mode validate` rc=0
on the subject.

### 3.2 QUIC

| arm | tip | P1 | P2 | inverted P2 |
|---|---|---|---|---|
| C0, both orders: a.b / x | **`H3 error (0x0)`** + ambiguous log / SHORT | LONG / SHORT | **LONG / SHORT** | **SHORT** / SHORT |
| M1: a.b | X | X | **Y** | X |

### 3.3 The five-selector suite — BLIND in every direction (re-confirmed)

`go test -count=1 -v ./cmd/envoy-go/... ./internal/admin/... ./internal/boot/... ./internal/listener/... ./validate/...`:
**rc 0 / 398 `=== RUN` / 0 RED under the tip, P1, P2 and the inverted P2.** Method note 50: identical
greens under fix, un-fix and inversion mean the suite cannot see the slot. §5 exists to make it see.

---

## 4. The production edit — DECIDED

### 4.1 The shape (P2, `26 23`, one file)

In `breakTie` slot 2, replace the two `sniSpecificityRank(x.ServerNames)` calls with
`sniMatchedRank(x.ServerNames, inputs.ServerName)`, which returns `(rank, suffixLen)`. After the rank
compare, a longer `suffixLen` wins. Replace `sniSpecificityRank` with:

```go
// sniMatchedRank ranks the pattern of patterns that MATCHES sni (not the
// chain's whole pattern set). Lower rank = more specific:
//
//	0: an exact pattern equal to sni (suffix length 0)
//	1: a "*." suffix wildcard matching sni; the second result is the length
//	   of the LONGEST matching suffix (len of pattern minus "*"), so a
//	   longer suffix is more specific
//	2: the universal wildcard "*"
//	3: nothing matched (unreachable: matches() rejects it before breakTie)
func sniMatchedRank(patterns []string, sni string) (int, int) {
	rank, suffix := 3, 0
	for _, p := range patterns {
		switch {
		case p == sni:
			return 0, 0
		case p == "*":
			if rank > 2 {
				rank = 2
			}
		case strings.HasPrefix(p, "*.") && strings.HasSuffix(sni, p[1:]):
			rank = 1
			if n := len(p) - 1; n > suffix {
				suffix = n
			}
		}
	}
	return rank, suffix
}
```

⚠️ **It pins suffix LENGTH, not pattern length** (`BRAINSTORM.md` §3.1(b)). The two order identically. The
suffix is the value the reference's longest-match descends on, and it is the one a later case fold
composes with, since it compares lengths, never raw strings (`BRAINSTORM.md` §1.3). ⚠️ **The matching
predicate is byte-identical to `sniMatchAny`'s** (`HasPrefix(p,"*.") && HasSuffix(sni, p[1:])`), so rank
and eligibility cannot disagree about what matched. The PLAN must keep them identical, or share them.
**The figure `26 23` is a FLOOR** (`reference_measured_prototype_is_a_lower_bound`, twenty-one rows). The
§6 comment edits in the same file add to it.

### 4.2 Rejected shapes

| shape | why rejected |
|---|---|
| **P1** (`22 0`, whole-set rank + matched length) | wrong on M1/M2 against the reference (§0.1) |
| whole-set rank + whole-set longest pattern | wrong on M3/M4 by construction (X2's longest member does not match `a.b.foo.test`) |
| fix the pairwise fold too (§0.4) | a different defect in a different code path, with no reference arm measured; banked |
| boot-reject overlapping `server_names` (§0.3) | the banked duplicate-matcher parity row; needs its own both-sides roster |

### 4.3 The declared behaviour changes

1. **Two or more wildcard chains matching one SNI**, with no exact match among them: **closed → served by
   the longest suffix**, TCP and QUIC, with or without a default chain. This is the parity direction and
   the row's purpose.
2. **A mixed exact+wildcard chain vs a longer wildcard chain**, where the SNI matched only wildcards:
   **the mixed chain → the longer-suffix chain.** This is the parity direction, M1/M2. ⚠️ **It moves
   traffic on a config that served before this row**, which makes it the change most likely to surprise
   an operator.
3. **Two chains whose best MATCHED patterns are the same string** (O1/O2): **served by an arbitrary
   winner → CLOSED.** The reference rejects this class at validate (R1). Parity would be a boot reject,
   which is banked.

`ErrAmbiguousChainMatch` remains reachable through slot 2 **only** for (3). Its other runtime sources are
unchanged: equal-bitmask chains with overlapping ALPN or source-port sets, and §0.4's fold.

---

## 5. Unit-test design

### 5.1 `TestSelectChainSNILongestMatchedSuffix` — a NEW file, `internal/listener/listenerfilter/chainmatch_sni_test.go`

The test is table-driven and calls `SelectChain` directly with realistic post-`tls_inspector` inputs:
SNI, `TransportProtocol:"tls"`, ALPN offer `h2,http/1.1`, loopback addressing. None of the chains sets
those dimensions, so they don't affect eligibility. **Every row name states its property, and every
failure message names the row and the SNI.** Each row calls `Errorf`, never `Fatalf` (method note 7e).
A prototype of the file (83 lines, 18 rows) was written and scored by this stage's subject agent. The
PLAN adds the rows marked **NEW** below.

### 5.2 Per-arm matrix — MEASURED, not predicted (except the rows marked NEW)

| row | tip | P1 | P2 | inv P2 | rank-mutant | reading |
|---|---|---|---|---|---|---|
| `C0_long_declared_first_longer_suffix_wins` / `C0_short_…` | FAIL | PASS | PASS | FAIL | – | **evidence** |
| `B_desc_*` / `B_asc_*` (4 rows, deepest and middle) | FAIL | PASS | PASS | FAIL | – | **evidence** |
| `M1_rank_of_matched_pattern_not_whole_set_X_first` / `M2_…_Y_first` | FAIL | **FAIL** | PASS | FAIL | – | **evidence: the P1-vs-P2 discriminator** |
| `M3_longest_matching_member_…` / `M4_…` (a.b) | FAIL | PASS | PASS | FAIL | – | **evidence** |
| `M3_deeper_member_of_mixed_set_wins` / `M4_…` (z.c.b) | FAIL | PASS | PASS | FAIL | – | **evidence** |
| `E_default_present_two_wildcards_resolve_not_default` | FAIL | PASS | PASS | FAIL | – | **evidence** |
| `R_exact_beats_wildcard_exact_first` / `_wildcard_first` | PASS | PASS | PASS | PASS | **FAIL** | regression guard, live only under the rank-mutant (§0.11) |
| `C0_matched_negative_only_short_matches` | PASS | PASS | PASS | PASS | – | eligibility only; ONE chain eligible |
| `M1_exact_member_of_mixed_set_still_wins_its_name` | PASS | PASS | PASS | PASS | – | eligibility only |
| `E_default_serves_only_no_match` | PASS | PASS | PASS | PASS | – | eligibility only |
| **NEW** `O1_shared_matched_wildcard_is_ambiguous` (want `ErrAmbiguousChainMatch`) | *pred.* FAIL (tip returns A) | *pred.* FAIL | *pred.* PASS | *pred.* PASS | – | pins §4.3(3) so the declared change cannot drift silently |
| `TestSelectChainAmbiguousReturnsError` (unchanged, `chainmatch_test.go:173-186`) | PASS | PASS | PASS | PASS | PASS | survives: `TransportProtocol` only, slot 2 never engages (owed item 4) |

Measured totals over the 18-row prototype plus the two top-level RUN lines: **tip 17 RED, P1 6 RED
(M1/M2 pair plus their parents), P2 0, inverted P2 17.**

⚠️ **The NEW O1 row is predicted, not measured.** The PLAN must score it at the tip before landing P2,
and must not believe its colour until it has been run. ⚠️ **The rank-mutant is a mandatory NC-roster
row (§11)**, because the inversion is blind to the R rows.

### 5.3 One QUIC wiring arm — `quic_test.go`

`SelectChain` is shared, so §5.1 covers the rule. But the tip's QUIC failure (§3.2) is observed through
`selectQUICChain`, which runs **per connection** with SNI from the handshake and **also at Start with a
nil conn** (method note 27). The arm: a two-chain `*.b.foo.test` / `*.foo.test` QUIC runtime, driven
through the per-connection selector with SNI `a.b.foo.test`, must select LONG. It must be RED at the tip
(nil + ambiguous) and RED under the inverted P2. **The PLAN must read `quic_test.go`'s existing
per-connection harness first** (phase 97 built one) rather than invent a new one.

### 5.4 Deliberately NOT added

- No test for §0.4's fold, which is banked.
- No test for mixed case (banked row).
- `manager_test.go` is untouched. None of its existing SNI tests reaches slot 2 with two matching
  wildcards; the occurrence agent enumerated every one (`SNIHappy`, `Specificity`, `CatchAll`,
  `NoSNIMatch`, `ChainSelectionPropagation`, `TestUnifiedDispatchTLSWithSNI`, the `:3566` permutation
  arm). None of them can change colour under P2.

---

## 6. Occurrence set of every falsified claim (owed item 3)

**Method.** The BRAINSTORM's §3.3 union is inherited as a FLOOR. All its hits were re-confirmed at the
same lines. The four matchers were re-run case-insensitively under `git grep`, scoped to
`internal/ cmd/ validate/ test/ docs/envoy-go/{DECISIONS,BEHAVIOR_CONTRACT,ENVOY_TARGET}.md`, with
`phases/`, `STATE*.md` and `ROADMAP.md` excluded. Six growth matchers were added:
`build.time|build-time|NewManager-build`, `wildcard`, `server_names|server_name|ServerNames`,
`sub-ordering|tie-break|tiebreak`, `exact > suffix`, and `rank`. Every docs hit was resolved to its
enclosing ADR by backward `^## ` search. ⚠️ **One site (`quic.go:181-183`) was found by READING; no
matcher hits it** (method note 78's class). **The PLAN inherits these TABLES and uses matchers only to
look for further GROWTH** (method note 77).

### 6.1 Claim S1 — *"SNI ties are broken by the rank of the chain's pattern set (exact > suffix > universal)"*

| site | disposition |
|---|---|
| `chainmatch.go:25-28` (`ServerNames` field comment; also cites the deleted `manager.go:chainSpecificityRank`, §0.5) | **EDIT** (comment) |
| `chainmatch.go:196-197` (`breakTie` doc: *"ServerNames (slot 2, SNI rank)"*) | **EDIT** (comment) |
| `chainmatch.go:213` (slot-2 line comment) | **EDIT** (comment, part of §4.1) |
| `chainmatch.go:304-333` (`sniSpecificityRank` and its *"mirrors"* comment) | **REPLACED** by §4.1 |
| `DECISIONS.md:3152-3155` ADR-0081 clause 4 | **LEAVE, amended by ADR-0321** — a closed ADR is not rewritten; ADR-0321 §Decision names the clause it amends |
| `DECISIONS.md:3208` ADR-0078 clause 9 (*"LOGIC preserved verbatim as `sniSpecificityRank`"*) | **LEAVE, amended by ADR-0321** (same reason) |
| `DECISIONS.md:1024` ADR-0033 clause 9 | **LEAVE — HISTORICAL**, already superseded by ADR-0078 clause 9 |
| `BEHAVIOR_CONTRACT.md:4367` (*"SNI-specificity (exact > suffix > universal > catch-all per ADR-0033 clause 9 …)"*) | **EDIT** — the contract is live, not an ADR; it must state the matched-pattern + longest-suffix rule and cite ADR-0321 |

### 6.2 Claim S2 — *"an ambiguous selection is detected and rejected at `NewManager`-build time"*

| site | disposition |
|---|---|
| `chainmatch.go:54-58` (`ErrAmbiguousChainMatch` doc: *"pre-runs SelectChain on a sample input"*, which does not exist) | **EDIT** — the error is returned per connection; only structurally identical specs are caught at build |
| `chainmatch.go:185-188` (**growth**) | **EDIT** — the nil return is a per-connection outcome, not *"a NewManager-time config error"* |
| `DECISIONS.md:3157` ADR-0081 clause 5 (*"NOT at per-connection dispatch time"*) | **LEAVE, amended by ADR-0321** |
| `BEHAVIOR_CONTRACT.md:4368` | **EDIT — add, do not replace** (§0.9): keep the build-time sentence and append that non-identical ties are resolved per connection and close the connection when unresolved |
| `manager.go:1061-1064` (**growth**, paraphrases clause 5) | **LEAVE** — it describes what `findIdenticalChainSpecs` does, which stays true; the clause it paraphrases is amended at the ADR |
| `manager.go:727-731`, `:1080-1091`, `:1298-1299` | **LEAVE — STAYS TRUE** (scoped to structural duplicates and the abort path) |
| `quic.go:181-183` (**growth, found by reading**) | **EDIT** (comment) — name both nil sources; it is already incomplete at the tip |

### 6.3 Claims that STAY TRUE or are HISTORICAL — deliberately left

`chainmatch.go:235-236`, `:278-290` (`sniMatchAny`, where eligibility is untouched) · `fuzz_test.go:30,38-40`
· `manager_test.go:1336-1366, 1370-1405, 3218, 3417-3500, 3444, 3553-3596` · `quic_test.go:1129-1132, 1424`
· `DECISIONS.md:1034, 1040, 1042` (ADR-0033), `2881, 2893` (ADR-0077), `3087` (ADR-0080), `3144-3150`
(ADR-0081 clauses 1-3), `3173` (ADR-0081 consequence (b)), `3219` (ADR-0078 consequence (b)), `19233`,
`19242` (ADR-0320, accurate for phase 98) · `BEHAVIOR_CONTRACT.md:4347, 4364-4366, 4369, 4518`.
**Two left on purpose with a reason:** `DECISIONS.md:3165` (ADR-0081 Alternative A) is **already false**
per `BRAINSTORM.md` arm G3, but it belongs to the banked nested-descent row, and this row must not claim
it. `DECISIONS.md:19184` (ADR-0319 (d)) stays true, **but its reach widens**, and ADR-0321 §Consequences
says so.

### 6.4 Set-difference against a byte-untouched roster (method note 62)

The PLAN's byte-untouched roster **must not** list `chainmatch.go`, `quic.go` or `BEHAVIOR_CONTRACT.md`,
because all three are on §6.1/§6.2's EDIT list. `manager.go` **may** be byte-untouched: every hit in it
is LEAVE, and the row needs no code there. **That is the one intersection to check mechanically.**
⚠️ **The comment edits in `chainmatch.go` share a file with a CODE edit**, so the phase-98 comment-only diff
gate (`98/PLAN.md` §7) does not apply to that file as a whole. It **does** apply to `quic.go`, where the
row changes only a comment. Gate that file's edit as comment-only **and** line-count-neutral
(method note 80): `quic_test.go` carries line cites into production files, and the PLAN must census
which of them point into `quic.go` below `:181`.

---

## 7. Differential fixture `0124-listener-sni-longest-suffix` — CHARTERED (owed item 6)

### 7.1 Why ONE directory, FOUR TCP listeners

There is no existing gate: `server_names` appears in fixture `0002-tls-tcp` only, with exact names.
One fixture carries the reversal pair **as two listeners that differ only in declaration order**
(method note 70), plus the RANK discriminator and the default-chain case:

| listener | reference port | chains (declared order) | SNI → body pinned |
|---|---|---|---|
| `l_long_first` | **15124** | LONG `*.b.foo.test`, SHORT `*.foo.test` | a.b.foo.test → LONG · x.foo.test → SHORT |
| `l_short_first` | **15225** | SHORT, LONG | a.b.foo.test → LONG · x.foo.test → SHORT |
| `l_mixed` | **15226** | X `["x.test","*.foo.test"]`, Y `["*.b.foo.test"]` | a.b.foo.test → Y · q.foo.test → X · x.test → X |
| `l_default` | **15227** | LONG, SHORT + `default_filter_chain` DEFAULT | a.b.foo.test → LONG · nomatch.example → DEFAULT |

**QUIC is NOT in the fixture.** The harness cannot carry a mixed TCP+UDP listener set (the banked
`97/PLAN.md` §3 limitation), and QUIC parity is carried by §2.2's measurement and §5.3's arm. ⚠️ **M3/M4
is unit-only.** It discriminates nothing between P1 and P2 (§0.2), and a fifth listener costs more than
it buys.

### 7.2 Ports — CENSUSED at this tip, not inherited

`git grep -lw -- <port> -- test/ internal/ cmd/`: **`15124` → only `0123/README.md:293` and
`0123/driver/driver.go:115`** (the prose reserving it for this fixture). **`15225`, `15226`, `15227` → zero
files.** `ss -tan` → zero sockets on all four. The subject-side ports follow the `0123` multi-listener
derivation. **Re-census at the PLAN tip.**

### 7.3 Assertions — a NAMED SUBSET, values not presence

- **The served BODY per SNI per listener is the primary pin.** At the un-fixed tip, the `a.b.foo.test`
  rows of `l_long_first`, `l_short_first` and `l_default` read CLOSED, and `l_mixed`'s reads X. **Those
  four are the RED rows.** The matched negatives are green at the tip by construction (one chain
  eligible). They prove each losing chain is live and are **not** evidence of precedence (method note 61).
- **Per-chain `http.<stat_prefix>.downstream_rq_total` VALUES**, with distinct prefixes on every chain,
  pinned against the driven request count. **Pin values, not name presence**: both sides create the
  per-chain HCM scope at CONFIG time, so presence is satisfied at boot (method note 81). Prove the
  NAMES exist on both sides before pinning (method note 46).
- ⚠️ **No listener carries a no-match CLOSE.** `nomatch.example` on `l_default` is SERVED by the default
  chain. The reference books a TCP no-match close in `no_filter_chain_match` but **not** in
  `downstream_cx_total` (`BRAINSTORM.md` §0.14). Pin no `downstream_cx_total` across sides.
- **The driver sets SNI explicitly** on every request (method note 7h, the `0104` precedent). All SNIs
  are lowercase, since case is out of scope.

### 7.4 Registration, PKI, and cost

Four gates (method note 60): the directory shape `0124-…`, `fixture.RegisterFixture` in the driver
`init()`, the blank import in `test/differential/runner_test.go`, and byte-identity between the
directory name and the registered string. **Score them on the fixture-set set-difference, both `comm`
directions, never on the exit code.** At this tip the extractor reads **125 registered = 125 dirs**, with
both directions empty.

Commit PKI (`pki/` + `gen/main.go`, SANs `*.foo.test`, `*.b.foo.test`, `x.test`) and template
`inline_string:` for the reference, per `0121`. **Every chain needs a distinct `stat_prefix`** (the
row-78-class duplicate-registration panic, `BRAINSTORM.md` §2.1).

**Floor by SHAPE** (`git log --numstat`): `0045-sni-cluster` **849** (6 files, pki) · `0002-tls-tcp`
**1137** (17, pki) · `0121-listener-default-chain-tls` **1393** (9, pki). **Four listeners and four
chain pairs put it at the upper end, so budget ~1300-1500 added lines as a FLOOR.**

---

## 8. `ADR-0321` — §Context drafted HERE (owed item 7)

Drafted in `DECISIONS.md` under `## ADR-0321`, in the house block form (ADR-0294-0320). That form is
the `> **STATUS: PROPOSED` blockquote, a `### Context (drafted at the phase-99 SPEC)` of numbered
paragraphs, and a **retained** italic footer. There is no `**Status:**` line and no `---`. **This
re-arms the house guard.** At the IMPL, §Decision and §Consequences are appended after the footer, the
status flips to ACCEPTED in place, and nothing is renumbered.

**What §Decision must say (for the IMPL):**
1. It **amends ADR-0081 clause 4's `server_names` bullet** (the rank of the MATCHED pattern, then the
   longest matching suffix) and **clause 5** (non-identical ties are per-connection outcomes). It also
   **notes ADR-0078 clause 9** (the logic is no longer *"preserved verbatim"*). It supersedes nothing
   else.
2. It states §4.3's three behaviour changes, **(3) included**.

**What §Consequences must say:** the §1 OUT list, verbatim in substance (owed item 9), and ADR-0319 (d)'s
widened reach (§6.3).

---

## 9. `BEHAVIOR_CONTRACT.md` ledger — **+0 CONFIRMED**

The BRAINSTORM predicted `+0` names, and this stage **CONFIRMS** it by three measurements:
1. The close path (`manager.go:1363-1368` TCP, `selectQUICChain` QUIC) logs and closes, and increments
   **no** stat.
2. `git grep -inE 'ambiguous|no_filter_chain|nofilterchain' -- '*.go' ':!*_test.go'` finds **no stat
   name**, only comments, the error string, and `manager.go:731`'s message.
3. P2 touches no registry.

**The observable consequence is a redistribution across EXISTING per-chain counters.** The IMPL writes a
`**Phase 99 — +0, UNCHANGED (…)**` entry after the phase-98 entry at `BEHAVIOR_CONTRACT.md:5146`, **in the
phase-96/97/98 form**. It quotes **no absolute** (three mutually inconsistent absolutes are live) and
states that `no_filter_chain_match` is again deliberately not added.

---

## 10. The toolchain FOLD-IN — CHARTERED (owed item 8)

**Measured at this tip by this stage's agent, both toolchains** (ambient go1.27.1 at `/snap/go/11295`;
`go.mod` reads `go 1.23.0` with **no `toolchain` line**):

| gate | go1.27.1 | `GOTOOLCHAIN=go1.26.2` |
|---|---|---|
| `golangci-lint run ./...` (v1.64.8) | rc=1, **207** `(typecheck)`, root `sync/atomic … export data version 4` | **rc=0, zero findings** |
| same + planted file in `internal/listener/` | — | rc=1: **errcheck**, **revive** (`exported`), **ineffassign** — the gate is LIVE |
| `TestEncodeData_LevelMapping_DifferentGzippedSizes` | **FAIL** (`both = 2121`) | PASS |
| `internal/filter/http/compressor` package | rc=1, 184 RUN, **1** FAIL (only that test differs) | rc=0, 184 / 0 |

⚠️ **The planted file must COMPILE.** The agent's first control left a variable unused, and `typecheck`
masked every other linter. The control is valid only when the three target linters each fire.

**Mechanism (compressor).** go1.27.1 rewrote `compress/flate`: levels 1-6 now use a new fast encoder.
For the test's 4096-byte input, level 1 and level 9 both produce **2121** bytes.

**The charter:**
1. **Re-point the test** to `TestEncodeData_LevelMapping_LevelReachesEncoder`. It asserts the gzip header's
   **XFL byte** (offset 8: 4 at BestSpeed, 2 at BestCompression, 0 otherwise; RFC 1952), which
   `gzip.Writer` sets from its construction level identically in both toolchains. It runs two subtests,
   **each sending two responses through one config**, so the second takes the POOLED `Reset` path, and
   it keeps the round-trip check. **Test-only, `53 49` in `compressor_test.go`, no production change.**
   Measured: green on both toolchains (package **186 / 0** on each).
2. **RED-first, measured:** the original is RED under go1.27.1. The re-point is **RED under both
   plumbing mutations on both toolchains**. M1 builds the writer at the default level; M2 rebuilds the
   pooled writer at the default level, **which the original test PASSED under go1.26.2** (§0.10).
3. **The IMPL's lint gate runs as `GOTOOLCHAIN=go1.26.2 golangci-lint run ./...`**, with the compiling
   planted-violation control repeated and deleted. The command is recorded in `PROGRESS.md` and the
   router. **CI needs no change**: `.github/workflows/ci.yml` pins `setup-go` at `1.23` and
   `golangci-lint-action` v1.64.8, so CI never ran go1.27.1. **`go.mod` gains no `toolchain` line**: that
   would change the build for every consumer and is its own decision. A v2 linter move stays a separate
   ADR-gated re-baseline (`BRAINSTORM.md` §0.4).
4. Gate (b) then runs under the **ambient** toolchain and must be green, and the re-pointed test must
   also be green under go1.26.2.

---

## 11. Negative-control roster the PLAN inherits — neutralise, never revert

Each row names the mechanism that carries its mutation to a failure (method note 7d). ⚠️ **Score every
row PER ARM** (method notes 55, 61, 76).

| # | mutation | must redden | must NOT redden | mechanism |
|---|---|---|---|---|
| 1 | **inverted P2**: `la > lb` ↔ `lb > la` | every `C0_*`, `B_*`, `M1/M2_rank_*`, `M3/M4_*`, `E_default_present_*` row (MEASURED: 17 RED); `l_long_first`, `l_short_first`, `l_default` a.b rows in `0124`; the §5.3 QUIC arm | `R_*`, the three eligibility-only rows, `M1_exact_member_*`, `TestSelectChainAmbiguousReturnsError` | the length compare decides every same-rank wildcard pair |
| 2 | **rank-mutant**: `ra < rb` ↔ `rb < ra` in slot 2 | `R_exact_beats_wildcard_*` (both) — MEASURED | every wildcard-vs-wildcard row, **`M1/M2_rank_*` included**: under P2 both of M1's chains MATCH through a wildcard, so both rank 1 and LENGTH decides | only rows whose tied chains differ in MATCHED rank; after P2 that is exact-vs-wildcard alone |
| 3 | **whole-set rank** (restore `sniSpecificityRank`'s set scan for the rank only; this is P1) | `M1/M2_rank_*` (MEASURED: P1 reddens exactly this pair), `0124` `l_mixed` a.b row | everything else | the mixed set ranks 0 by `x.test`; **the only mutant that separates P1 from P2** |
| 4 | **drop the length compare** (tie ⇒ nil) | same set as row 1 | same set as row 1 | two same-rank wildcards become ambiguous again (the tip's behaviour) |
| 5 | **O1 row inverted expectation** | `O1_shared_matched_wildcard_is_ambiguous` under the tip | — | the tip returns A by whole-set rank |

⚠️ **Row 1 alone cannot score the `R_*` rows** (§0.11), **and row 1 reddens `M1/M2` for a LENGTH reason, so it cannot say whether the RANK half of P2 is present** — that is row 3's job. **Rows 2 and 3 exist to cover what row 1 cannot.**

---

## 12. Counts, re-derived at THIS stage's own tip

### 12.1 Moved by this stage

`DECISIONS.md` gains `## ADR-0321` §Context: `^## ADR-` **319 → 320**, bare `^## ` **327 → 328**, tail
**ADR-0321**, next-free **ADR-0322**, `^---$` **216 unmoved**. The line count is re-derived in the
publishing commit (§13). Phase dir `99-…` gains `SPEC.md`. `STATE.md` is rolled in place.
`STATE_HISTORY.md` gets **+2** raw lines (one archived entry), strict guard **DELTA 0**.

### 12.2 Not moved — and the scope was MEASURED

`ROADMAP.md` **249**, byte-untouched (a SPEC neither adds nor flips a row). `BEHAVIOR_CONTRACT.md`
**5996**, byte-untouched (the IMPL writes the ledger and the §6 edits). **No `.go` file is written.**
Fixtures stay at **125**. The SPEC scope is **five paths**, measured against `bd303d87` (phase 98),
`331c3524` (97) and `da6ea191` (96) by `git show --numstat`: `DECISIONS.md`, `STATE.md`,
`STATE_HISTORY.md` `2 0`, the new `SPEC.md`, and `next-prompt.txt`.

### 12.3 The split gate the PLAN must evaluate (BOOTSTRAP §6.1: ~25 tasks / ~1500 LoC)

These are floors: production `26 23` plus the `chainmatch.go`/`quic.go` comments; the unit file ~110;
the QUIC arm ~60-100; the compressor re-point `53 49`; fixture `0124` ~1300-1500. **Precedent:
phase 98's IMPL (`884e4b7c`) landed ~1846 added non-`PROGRESS.md` lines**, fixture included, and was not
split. **This row sits at the same order. The PLAN must evaluate the gate WITH A COMMAND over its own
task list, not by analogy.**

---

## 13. Sentinel — RUN MECHANICALLY AT THIS STAGE's TIP, `/usr/bin/grep`

This was run at the worktree tip before any edit and **re-run after every edit in the publishing tree**.
Both runs read identically, because a SPEC touches no `ROADMAP.md` byte:
(1) **ONE** — `NOT DONE: row 99` · (2) **SIX** at `:209 :215 :221 :231 :237 :245` · (3) **SILENT** · NC-A
(substitution inspected: `NC LANDED? [ in-progress ]`) **TWO** — `NOT DONE: row 62`, `NOT DONE: row 99`
· NC-B at `want=130` **TWO** — `NOT DONE: row 99`, `GATE FAIL: examined 131 data rows, expected 130` ·
NC-C **FIRED** (residual 0, `NEVER OPENED: gRPC   <- NC FIRED`) · NC-D **96 / 68** under `--` · check-(2)
positive control **residual 0, 6 substitutions asserted** · escape-aware malformed set **exactly {57, 69}**
at file lines **119** (NF 9) and **131** (NF 10) · row 99 NF **8**.

Per-line md5, **trailing newline INCLUDED** (`sed -n 'Np' f | md5sum`, first 12 hex): `209 10d7807bf02d` ·
`215 4a92f7e62fc6` · `221 2a7eb298b9fd` · `231 242e53c6f7a3` · `237 b2680e6f4fbf` · `245 6caa1c3ce0e7`.
These are **byte-identical to the phase-99 BRAINSTORM close.** ⇒ **The sentinel does NOT fire. `stop` was
evaluated and NOT created.**

---

## 14. Probe hygiene

There were four agents, **split on the Docker axis**. The reference agent alone used Docker (band
`16300-16349`, containers `p99spec-ref-*` removed BY NAME, no foreign container touched among the live
`cp-*`/`cpj-*` sessions). The subject agent used no Docker (band `16350-16399`, five throwaway worktrees,
all removed). The occurrence agent was read-only in the stage worktree. The toolchain agent used one
throwaway worktree, removed.

The band `16300-16399` was censused first: `git grep -In '163[0-9][0-9]' -- test/ internal/ cmd/` hits
only the literal `16384` (buffer and frame sizes), no port. `ss` read zero sockets. ⚠️ **This paragraph
spells the band numbers and is a hit in the next census** (method note 72). Afterwards,
`git status --porcelain --untracked-files=all` at the canonical root listed only the pre-existing
`.claude/`. **No production `.go` was written and none of the six gates was run.** That is a SPEC's
SCOPE, not an omission. The prototype diffs and the 83-line candidate test live only in the session
scratchpad.

---

## 15. What the PLAN owes

1. **Order the spine so the un-fixed tip is measured FIRST.** Land §5.1 (including the NEW O1 row),
   §5.3 and fixture `0124`, run them RED, and record **per arm** which rows are RED and which are
   structurally green. Then land §4.1 and re-run. **Score the O1 row before believing its predicted
   colour.**
2. **Inherit §6's TABLES** and re-anchor every site by LITERAL TEXT at the PLAN tip. Use matchers only
   for GROWTH (method note 77), and never shrink the set.
3. **Gate `quic.go`'s comment-only, line-count-neutral edit mechanically** (the `98/PLAN.md` §7 gate,
   NC'd). Census `quic_test.go`'s line cites into `quic.go` first.
4. **Build NC roster rows 1-5** (§11) as executable patches and prove each one can fire **before**
   believing its colour.
5. **Prove the per-chain counter NAMES exist on both sides** before `0124` pins their values, and pin
   values with a mirror arm.
6. **Evaluate BOOTSTRAP §6.1 with a command** over the task list (§12.3).
7. **Carry the fold-in (§10) as its own tasks**: the re-point RED-first under go1.27.1, the two plumbing
   mutations, and lint under `GOTOOLCHAIN=go1.26.2` with a compiling planted control.
8. **Carry §0.4 (the order-dependent fold) into the banked list** with its F1-F4 table. It must not
   leak into this row's tasks.
