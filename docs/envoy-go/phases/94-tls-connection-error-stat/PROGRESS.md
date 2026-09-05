# Phase 94 — `tls-connection-error-stat` — PROGRESS (the IMPL)

Lifecycle-state **3 -> DONE**. Row 94 flips `in-progress` -> `done`.

**What landed.** `outcomeOther` — the fourth downstream-TLS handshake-outcome bucket, which previously incremented nothing — now increments `listener.<normalized-addr>.ssl.connection_error`, the FIFTH listener-scope TLS counter, gated by a **closed transport-exclusion predicate**. The named departure at `BEHAVIOR_CONTRACT.md:1971` is **CLOSED**. The TWENTY-FIFTH Observability-family row; the family **STAYS OPEN**.

⚠️ **Every figure below is from THIS session's own runs.** Nothing is inherited from the BRAINSTORM, SPEC or PLAN without re-derivation.

---

## 0. Sentinel — RUN MECHANICALLY, BEFORE the row-94 flip

ACTUAL output, recorded not predicted:

```
(1) NOT DONE: row 94                     <- ONE line, correct and expected while row 94 is open
(2) 204:remaining deferred (not-yet-chartered) candidates:
    210:remaining deferred (not-yet-chartered) candidates:
    216:remaining deferred (not-yet-chartered) candidates:
    226:remaining deferred (not-yet-chartered) candidates:
    232:remaining deferred (not-yet-chartered) candidates:
    240:deferred candidates:              <- SIX
(3) (silent)
```

Per-line md5 of the six windows, **trailing newline INCLUDED** (`sed -n 'Np' f | md5sum`) — the method is stated because the digest is method-sensitive and a `tr -d '\n'` form mismatches all six:

```
204 10d7807bf02d   210 4a92f7e62fc6   216 2a7eb298b9fd
226 4ad940205410   232 b2680e6f4fbf   240 6caa1c3ce0e7
```

All six **BYTE-IDENTICAL** to the SPEC's and the PLAN's record.

**All four NCs, plus the check-(2) positive control:**

| control | result |
|---|---|
| NC-A (row 62 doctored to `in-progress`) | **TWO** lines: `NOT DONE: row 62`, `NOT DONE: row 94` — NC landed, verified by inspecting `[ in-progress ]` before trusting it |
| NC-B (`want=125` on the real file) | **TWO** lines: `NOT DONE: row 94`, `GATE FAIL: examined 126 data rows, expected 125` |
| NC-C (check-3 NC, `gRPC-family row` neutralised) | **FIRED** — `NEVER OPENED: gRPC`, with **0** residual matches |
| NC-D (`-family row` with `--`) | occurrences **96**, lines **68** |
| check-(2) positive control | residual **0**, with the neutralisation itself verified at **6** substitutions |

⚠️ **THE CHECK-(2) POSITIVE CONTROL WAS BROKEN ON ITS FIRST RUN AND IS RECORDED AS SUCH.** The first form neutralised only `deferred candidates:` and left the five `remaining deferred (not-yet-chartered) candidates:` lines untouched, because the longer phrase does not CONTAIN the shorter one as a substring. It read **5**, not 0. The corrected form substitutes BOTH phrases and reads **0**, with the substitution count asserted at 6. `reference_gate_command_negative_control`: **the NC itself can be broken, and a positive control that reports a non-zero residual is indistinguishable from a real finding until you read the command.**

⇒ **THE SENTINEL DOES NOT FIRE.** `stop` was evaluated and **deliberately NOT created** — verified absent at the git root and in all four stage worktrees.

---

## 1. Execution shape

Subagent-driven per `feedback_execution_style`, **three streams on three private worktrees and branches**, each with its own scratch dir, **Docker serialized to exactly ONE stream at a time**:

| stream | branch | tasks | Docker |
|---|---|---|---|
| A | `phase-94-a` | T1-T8 — production counter, nil gate, helpText triple, predicate, Inc + counter table, roster, prose | no |
| B | `phase-94-b` | T9-T10 — PKI, both configs, the boot gate | **yes**, then released |
| C | `phase-94-c` | T11-T14 — driver, five drive arms, `AssertStats`, expectations/README | **yes** |
| controller | `phase-94-impl` | T15-T17, the gates, the close | h2spec only |

Every stream committed LOCALLY with explicit pathspecs; the controller merged and squashed. **No stream pushed.** Sibling `curl-world` containers (16 of them) were present throughout and **never touched**; every teardown was BY NAME.

---

## 2. ⚠️ WHAT THIS IMPL REFUTED BY EXECUTION — TWELVE CLAIMS, PLUS ONE SELF-INFLICTED DEFECT CAUGHT

The project discipline is that every stage refutes its predecessor by execution (92 IMPL twelve · 93 BRAINSTORM eleven · 93 SPEC eleven · 93 PLAN twelve · 93 IMPL nine · 94 BRAINSTORM eleven · 94 SPEC twelve · **94 PLAN sixteen**). This stage refutes **TWELVE**, and **THREE of them invalidated text this session had already written and committed** — corrected in place rather than left standing. **A THIRTEENTH entry (§2.13) is not a refutation of a predecessor but a defect this session INFLICTED ON ITSELF and caught**; it is recorded here because it is the most transferable thing the stage learned, and because concealing it would make the record less useful than the mistake made it.

### ⚠️ 2.1 — TWO PREDICATE TERMS ARE UNEXERCISED, NOT ONE (corrects the SPEC, the PLAN §3.1 and this row's own first ADR draft)

`SPEC.md` §3, `PLAN.md` §3.1 and the production comment landed verbatim from it all label **only** `net.ErrClosed` as *"DEFENSIVE and UNEXERCISED"*, and the PLAN states `context.DeadlineExceeded` *"is behaviour-matching, not defensive."* **MEASURED at the IMPL: neither is produced by any arm.**

**CONTROLLER-VERIFIED INDEPENDENTLY** rather than taken on the executing stream's word: `context.DeadlineExceeded` occurs in `internal/listener`'s tests at exactly one site, `manager_test.go:4420`, a hand-written value in `TestClassifyHandshakeErr` — which calls the classifier DIRECTLY and **never reaches the predicate**.

**THREE of the five terms ARE exercised**, each by a distinct arm, with the concrete dynamic types read back by instrumentation:

```
bad_version            -> *errors.errorString "tls: client offered only unsupported versions: [302]"
plaintext_http         -> tls.RecordHeaderError
garbage_bytes          -> tls.RecordHeaderError
partial_hello_then_FIN -> io.ErrUnexpectedEOF
zero_bytes_then_FIN    -> io.EOF
partial_then_RST       -> *net.OpError, matching syscall.ECONNRESET
```

⇒ **the production comment and ADR-0316 §Decision both corrected.** No term collapses onto another; each transport arm exercises a distinct member.

### ⚠️ 2.2 — THE `gofmt` STORY IS NOW REFUTED IN BOTH DIRECTIONS

The SPEC claimed the new `helpText` key would force a realignment of the map block. The PLAN refuted that (the key is **35** chars against the incumbent longest **38** — SHORTER) and asserted the realignment merely **MOVED** to `manager.go`, where `sslNoCertificate` would gain a space, calling that churn *"expected and correct"*.

**MEASURED: `gofmt` demands the OPPOSITE.** The new field's intervening doc comment **BREAKS the alignment run**, so each commented field is its own run and `gofmt` wants a **SINGLE** space on both. Following the PLAN literally made `gofmt -l internal/listener/` **PRINT `manager.go`**; reverting to a single space made it print nothing.

⇒ **The net churn on existing lines is ZERO, in BOTH files.** The row rewrites no existing line for alignment anywhere.

### ⚠️ 2.3 — PLAN §4.1's "STABLE ANCHOR" TABLE IS WRONG IN TWO OF ITS TWELVE ROWS

| row | PLAN's anchor | actual |
|---|---|---|
| `helpTextRoster` ssl entry | `"listener.<addr>.ssl.no_certificate"` | **no `<addr>` placeholder exists** — the real form is `{internal: "listener.0_0_0_0_10000.ssl.no_certificate"},` (`helptext_test.go:48`). Task 3 Step 3's code snippet is the wrong shape too |
| `wantNames` slice | `func TestListenerSSLHandshakeOutcomes` | **no such symbol** — it is `TestHelpText_ListenerSSLHandshakeOutcomes` |

⚠️ **A table of "drift-proof anchors" is ITSELF A CLAIM.** The PLAN introduced §4.1 precisely because line numbers drift — and then two of its twelve replacement anchors did not survive contact with the tree.

### ⚠️ 2.4 — PLAN TASK 7 STEP 5'S OWN GATE IS UNSATISFIABLE BY THE PLAN'S OWN MANDATED TEXT

The PLAN prescribes:

```sh
git grep -n -i 'four' -- 'internal/listener/manager.go'   # expect exactly 1 line: :414
```

That gate **cannot pass** on a tree built to the PLAN's own instructions:
- **Task 1 Step 4** dictates a field comment containing *"the fourth outcome"* and *"taxonomy stays FOUR"*;
- **Task 7 Step 1 sites 3 and 4** dictate stating *"the FOUR-outcomes / FIVE-counters asymmetry"*;
- **Task 4 Step 4**'s verbatim §3.1 predicate comment contains a **SECOND copy** of *"crypto/tls exports four error TYPES"* — the very sentence the PLAN protects at `:414`.

Measured sweep: **SIX** lines, every one justified (`:189 :193 :402 :409` count OUTCOMES; `:435 :487` count `crypto/tls` error TYPES). ⇒ **The correct gate is a JUSTIFICATION requirement — "no surviving `four` counts an `ssl.*` NAME or POINTER" — not a line count.** A count gate cannot express it.

### ⚠️ 2.5 — PLAN TASK 5's `=== RUN` DENOMINATOR IS 8, NOT 9

Steps 3 and 5 both predict `=== RUN` = **9**, glossed as *"2 parents + 6 subtests + stacked"*. **MEASURED: 8**, in RED, in GREEN, and in both NCs. The PLAN **double-counted the stacked test** — it is one of the "2 parents" AND the "+ stacked". Correct denominator: 1 parent + 6 subtests + 1 stacked top-level = **8**. ⚠️ This matters: an executor asserting `RUN == 9` would have read a correct run as a failure.

### ⚠️ 2.6 — TWO BEHAVIOR_CONTRACT SITES ARE ABSENT FROM `SPEC.md` §11'S PINNED EDIT MAP

The map names `:1967`, `:1971` (×2), `:1973`, `:1975`. Found by reading OUTWARD from it rather than trusting its completeness:

- **`:1042`** — the phase-65 **C3 coverage boundary**: *"RETIRED for FOUR fixed names (three by phase 74/ADR-0296, a fourth by phase 75/ADR-0297)"*. Phase 94 makes it **FIVE**.
- **`:5137`** — the **stat-surface ledger** is LIVE and maintained, tail `**Phase 92 — 1207 → 1208 (+1)**`. This row is +1 and owed an entry; it now carries `**Phase 94 — 1208 → 1209 (+1)**`.

A **THIRD** false proposition sits on `:1971` itself and the map does not name it either: *"the withheld fourth name is `ssl.connection_error`, and it is **STILL withheld**."*

⇒ **`reference_measured_prototype_is_a_lower_bound` fires an ELEVENTH consecutive row, again by under-enumerating SITES.**

### ⚠️ 2.7 — REPAIRING THE PHANTOM `B5` CITES FALSIFIES THE CONTRACT'S OWN DOCUMENTARY NOTE

`BEHAVIOR_CONTRACT.md:1971` carries a note stating that the dangling *"BEHAVIOR_CONTRACT B5/B6"* citations *"appear in landed production comments under `internal/listener/`"*. **PLAN Task 7 repoints exactly those two carriers**, so the note goes FALSE at this row's own close. Neither the SPEC's map nor PLAN T17 mentions it. The note is updated in the same commit that repairs them.

⚠️ **AND THE FIRST WORDING OF THAT UPDATE WAS ITSELF IMPRECISE, CAUGHT BY RE-READING T7 AS LANDED.** `manager.go:431` and `:1337` **RETAIN** the string `"BEHAVIOR_CONTRACT B5"` as a LABELLED historical note (*"the former cite was a PHANTOM"*), so a bare grep for that token **still matches** under `internal/listener/`. What no longer exists there is a citation that PURPORTS to resolve. Corrected to distinguish a retained LABEL from a live CITATION.

### 2.8 — ADR-0297's `:1855` POINTER IS LINE DRIFT, NOT A COMPETING REFERENT

`DECISIONS.md:17310` and `:17390` record the true referent as `BEHAVIOR_CONTRACT.md:1855` under a `:1849` heading. Measured at this tip **both lines are BLANK**; the subsection has moved **+116** to `:1965`/`:1971`. ⇒ a **stale-but-once-correct** pointer, not a wrong one. Append-only ⇒ **not edited**; recorded in ADR-0316 §Consequences (iv). This STRENGTHENS the PLAN's choice of `:1965`.

### ⚠️ 2.9 — THE PLAN'S T11 STEP 6 FIXTURE-SET GATE IS DEFECTIVE IN TWO INDEPENDENT WAYS

```sh
grep -o 'test/fixtures/[^/]*/driver' test/differential/runner_test.go | sed 's|test/fixtures/||; s|/driver||' | sort
```

1. **It matches PROSE, not imports.** Five fixtures (`0020`, `0021`, `0026`, `0032`, `0033`) are named in COMMENTS as well as in blank imports, so the extraction yields duplicates that `comm` mis-reports as **phantom "imports with no dir"**. Measured: the naive form yields **126** entries for **121** imports. The PLAN warns about the 97-`driver/` / 24-`inputs/` two-layout trap and **not** about comment contamination.
2. **Its `sed` delimiter is `|`**, which breaks the instant you add the `(driver|inputs)` alternation the PLAN itself demands — the expression errors out, the extraction reads **0**, and `comm` then reports **all 122 directories** as unimported. Measured live at this stage: `sed: -e expression #1, char 41: unknown option to 's'`.

⚠️ **Both failure modes read like a real finding.** The corrected, negative-controlled form anchors on the blank-import LINE:

```sh
extract () { grep -oE '^[[:space:]]*_ "github\.com/pgdad/envoy-go/test/fixtures/[^/]+/(driver|inputs)"$' "$1" \
  | sed -E 's#.*/test/fixtures/##; s#/(driver|inputs)"$##' | sort; }
```

**NC on the extractor itself** (one import renamed to `0119-XXXX`): it reports `0119-XXXX` as an import with no dir AND `0119-grpc-unary-trailers` as a dir with no import — so the gate is proven able to FIRE in both directions.

### 2.10 — `SPEC.md` §8 NC 5 IS RICHER STILL AT THE LANDED TIP

`PLAN.md` §0.16 corrected the SPEC from ONE test to **TWO**. Measured here: **two** against a slice-left-at-four tree, but **THREE** at this row's landed state, where the slice is also extended. The mutation is strictly richer than either predecessor recorded. The conclusion is unchanged.

### 2.11 — THE `sslLeafRoster` CONTROL CELL THE PLAN DID NOT ASK FOR

PLAN T6 Step 3 mandates one isolating NC. The executing stream ran the **control cell too** — the same NC with the roster reverted to FOUR — and it came back **fully GREEN** (`RC=0 RUN=2 FAIL=0`), against RED at five. ⇒ **the leak is invisible at four and caught at five, so the roster extension and nothing else is the guard.** That is the discriminating half `reference_passing_test_is_not_a_guard` demands, and it converts PLAN §0.1's argument from a reading into a measurement.

### 2.12 — THE REGISTERED SDS FLAKE RECURRED, AND IS NOT A REGRESSION

One full-repo sweep returned `RC=1 RUN=8720 ANCHORED_FAIL=5`, a single failure in `internal/boot`: `TestSDSEndToEnd_FetchFailure_BootFailsClosed/silent_SDS_server…` (`DeadlineExceeded` instead of the initial-fetch-timeout message). **That package is untouched by this row**, it passes in isolation, and the immediately following full run was `RC=0 RUN=8720 ANCHORED_FAIL=0`. This is the **registered** SDS dial-budget/timing flake. ⚠️ **A green rerun clears NOTHING** — it is recorded, not dismissed.


### ⚠️ 2.13 — THIS SESSION BROKE THE SENTINEL WITH ITS OWN NARROWING TEXT, AND CAUGHT IT BY RE-RUNNING RATHER THAN BY TRUSTING THE EDIT

`PLAN.md` §8 requires this row to NARROW window `:226` (item 2 of three — the uncounted non-certificate handshake-failure bucket this row consumed) and warns that **narrowing is not sentinel progress**, because check (2) keys on the window's opening PHRASE and not on the clause.

The first draft of that narrowing wrote the warning out in full **and quoted the match phrase verbatim to explain it**. Line `:226` then matched **TWICE**:

```
226:remaining deferred (not-yet-chartered) candidates:
226:remaining deferred (not-yet-chartered) candidates:
   check (2) = SEVEN                     <- BROKEN, by the sentence asserting it could not break
```

⚠️ **THE SENTENCE EXPLAINING THAT NARROWING CANNOT MOVE THE SENTINEL MOVED THE SENTINEL.** Reworded so the phrase is described rather than re-spelled; check (2) is back to **SIX**, with five of the six window md5s **byte-identical** to the inherited record and only `:226` changed — which is exactly the line this row deliberately narrowed.

**THE DURABLE RULE, new at this row: NEVER RE-SPELL A SENTINEL MATCH PHRASE INSIDE A SENTINEL WINDOW.** This is the same species as `reference_self_incrementing_positive_control` — a figure or token recorded in the file it measures is falsified by the act of recording it — but sharper, because here the *self-reference was inside the gate's own scan region* and the damage was to the termination sentinel itself. It was caught only because the check was **re-run after the edit**; a session that had reasoned *"narrowing cannot move check (2), so there is nothing to re-measure"* would have shipped a broken sentinel while quoting the rule that says it is safe.

---

## 3. Task-by-task evidence — every figure from this session's own runs

### T1 — the fifth name, spelling pins RED first
Anchors verified unique BEFORE editing: `prefix + "ssl.fail_verify_error",` = 1 hit each in `manager_test.go` / `quic_test.go`; `RegistersExactlyFourSSLNames` = **4** hits under `-- 'internal/listener/*.go'`, **0** after the pathspec-scoped `sed`. ⚠️ The historical occurrences in `75/PLAN.md`, `75/PROGRESS.md`, `DECISIONS.md`, `94/BRAINSTORM.md`, `94/SPEC.md` and `next-prompt.txt` were **NOT** renamed — an unscoped `sed` would have corrupted six landed documents.

```
RED:   RC=1 RUN=2 FAIL=5   both tests, `got` missing ssl.connection_error from `want`
GREEN: RC=0 RUN=2 FAIL=0   gofmt -l internal/listener/  -> EMPTY
```

The new name **PREPENDS** at index 0 in both the dotted and Prometheus projections — confirmed, and now recorded in the collation comments, which the SPEC's list never stated.

### T2 — the nil gate, landed THREE TASKS BEFORE the Inc
⚠️ **A DELIBERATE, STRICTER DEVIATION FROM `SPEC.md` §16 ITEM 7**, stated rather than made silently. The SPEC requires the fifth `GateMatchesInc` assertion in the SAME commit as the `Inc`. Landing the guard FIRST satisfies the invariant at **every** commit boundary, where landing them together satisfies it only at the boundary. **No commit in this sequence holds an unguarded `Inc`.**

```
NC (registration line neutralised to a comment; package still COMPILES):
  RC=1 RUN=4 FAIL=5
  tls_listener  RED at manager_test.go:2306
    "rt.sslConnectionError is NIL — Inc would panic the serveConnection goroutine"
  plaintext_listener PASS ; mixed_rejected_at_build PASS
RESTORED: RC=0 RUN=4 FAIL=0
```
`RUN=4` matches the PLAN exactly. Both stale `all FOUR counter fields` headers (PLAN §0.15, sitting OUTSIDE the SPEC's stated byte ranges) fixed here.

### T3 — `helpText` + `helpTextRoster` + `wantNames`, ONE commit
```
RED (roster + slice landed, helpText entry withheld): RC=1 RUN=146 FAIL=6, three tests
GREEN:                                               RC=0 RUN=146 FAIL=0
gofmt -l internal/stats/ -> EMPTY (hand-aligned; zero existing lines rewritten)
```

| NC | mutation | predicted | MEASURED |
|---|---|---|---|
| 4 | entry present, roster removed | RED, 1 test, `extra:` | ✅ `helptext_test.go:141 … extra: [envoy_listener_ssl_connection_error]` |
| 5 | roster present, entry removed | RED, **TWO** tests | ✅ two against a slice-at-four tree (`:139 missing:` + `:202 1 rendered HELP line(s) degraded`); ⚠️ **THREE** at the landed tip — richer than either predecessor recorded |
| 6 | both present, slice at FOUR | **GREEN** — the documented silent gap | ✅ `RC=0 RUN=146 FAIL=0`, reproduced deliberately |

### T4 — `isTransportHandshakeErr`, landed VERBATIM from PLAN §3.1
```
RED:   undefined: isTransportHandshakeErr   (the expected compile-error red)
GREEN: RC=0 RUN=8 FAIL=0                    (RUN=8 matches the PLAN exactly)
NC (return false, terms still referenced): RC=1 RUN=8 FAIL=7 — exactly the three transport arms RED
```
The table is built on **PRODUCTION-REPRESENTATIVE values from real server-side handshakes over a loopback TCP pair**, never hand-written strings, and `TestIsTransportHandshakeErr_ReadsIdentityNotText` ships PERMANENTLY: `errors.New("connection reset by peer")` is byte-identical in TEXT to `syscall.ECONNRESET.Error()` yet `errors.Is` is FALSE. If that test ever flips, the predicate has started reading text.

### T5 — ⚠️ THE LOAD-BEARING TASK: the predicate-gated Inc and the table that falsifies it
```
RED (before the Inc arm):  RC=1 RUN=8 FAIL=8
   three inclusion arms 0 want 1; stacked 0 want 1
   the three EXCLUSION arms pass VACUOUSLY — precisely why the stacked control exists
GREEN:                     RC=0 RUN=8 FAIL=0 ; full package RUN=148 FAIL=0
```

| NC | mutation | MEASURED |
|---|---|---|
| **1** | predicate removed (unconditional `Inc`) | **RED** — `partial_hello_then_FIN`, `zero_bytes_then_FIN`, `partial_then_RST` each read 1 want 0; **STACKED READS 4, WANT 1** |
| **1b** | `Inc` removed (`_ = rt.sslConnectionError`) | **RED** — all three inclusion arms read 0 want 1; **STACKED READS 0, WANT 1** |

Only the correct implementation reads 1. ⚠️ **The stacked control is what makes the exclusion arms observable at all** — a bare `== 0` cannot distinguish *"the predicate excluded it"* from *"nothing ran"*, and `assertSSLCrossProduct` cannot express a pure negative arm (it `t.Fatalf`s on zero `wantSuffixes`).

**Reachability control, BOTH forms** — because a green run is not evidence a site is exercised:
- log-marker enumeration (no fail-fast masking), full package `RUN=148 RC=0`: reachers are exactly the six new arms, the stacked control's four connections, **and `TestNewManager_LiveHandshake_ALPNNegotiationFailure_Aborts` with `transport=false`** — PLAN §0.2 CONFIRMED, the `Inc` fires there **unasserted**.
- `panic("REACHED")` form: `RC=1`, `panic: REACHED` at `manager.go:1335`.

The release barrier is **poll-the-gauge with NO SLEEPS**: `downstream_cx_total` to N, then `downstream_cx_active` back to 0 — sound because `serveConnection` defers `downstreamCxActive.Dec()` AFTER the outcome switch. `gaugeValue` was ADDED (it did not exist; `counterValue` is counter-only) using `Load() int64` — there is no `Value()` — and never registers a stat inside `Registry.Walk`, which would deadlock under the RLock.

### T6 — `sslLeafRoster` to five, and what it actually guards
```
ISOLATING NC (Inc added to the outcomeVerifyError arm ONLY):
  RC=1 RUN=2 FAIL=4
  …SSLFailVerifyErrorIncrements  RED at manager_test.go:4716
    "ssl.connection_error = 1, want 0 — only [fail_verify_error] may move on this arm"
  …NoCertIncrements              GREEN            <- the per-arm asymmetry IS the proof
CONTROL CELL (same NC, roster reverted to FOUR):
  RC=0 RUN=2 FAIL=0   FULLY GREEN                 <- invisible at four, caught at five
```
The PLAN cites `:4694`; at this tip the line is `:4716` — drift from T4/T5's inserts, which is exactly why §4.1 mandates text anchors over line numbers. The roster's doc comment now records what it does and does **not** guard.

### T7 / T8 — prose
Six `manager.go` blocks edited; both phantom `BEHAVIOR_CONTRACT B5` cites repointed at `### Downstream TLS handshake-outcome stats`, **re-verified by READING `BEHAVIOR_CONTRACT.md:1965`**, which itself states *"There is NO B-numbered step scheme anywhere in this file."* The protected non-site (*"crypto/tls exports four error TYPES"*) left alone. `quic_test.go` is now **ZERO** `four` hits. British-spelling sweep over every `.go` file touched: **NONE**.

**Final `four` sweep, every surviving hit JUSTIFIED** — this is the gate, not a line count (§2.4):

| file | hits | why each is legitimate |
|---|---|---|
| `manager.go` | `:189 :193 :402 :409` | count **OUTCOMES** (the taxonomy stays FOUR against FIVE counters) |
| | `:435 :487` | count **`crypto/tls` error TYPES** — the protected species |
| `manager_test.go` | `:2019` | historical narration: *"three then, four as of phase 75, five as of phase 94"* |
| | `:2295 :2304 :2362` | **ORDINALS** naming WHICH pointer (fourth = `no_certificate`, fifth = `connection_error`) |
| | `:3443 :3631 :3787 :3788` | count **network filters / built-ins**, not `ssl.*` |
| | `:4592` | counts `assertSSLCrossProduct` **CALL SITES** — re-derived at this tip: exactly 4 |
| | `:4901` | counts **`crypto/tls` error TYPES** |
| `quic_test.go` | — | **zero** |

**No surviving `four` counts an `ssl.*` NAME or POINTER.**

### T9 — the PKI: the tree's FIRST committed `clientAuth` leaf
No committed cert in this tree carried `clientAuth` — all four `0002-tls-tcp` leaves and `0119`'s leaf are serverAuth only, and `0018`'s client cert is `.gitignore`d and generated at `init()`. `0120` ships its own, following `0119`'s COMMITTED-PEM precedent and **rejecting `0018`'s generator**, which writes into the worktree at test time and would make `git status --porcelain` dirty after every differential run.

```
ca.pem      CA:TRUE, Digital Signature + Certificate Sign, 2026-01-01 -> 2046-01-01
server.pem  TLS Web Server Authentication
            SAN: DNS:localhost, DNS:l_conn_err.fixture.test, IP:127.0.0.1, IP:::1
client.pem  TLS Web Client Authentication
            SAN: URI:spiffe://example.com/0120-client
openssl verify -CAfile ca.pem  ->  server.pem: OK   client.pem: OK   (exit 0)
```
⚠️ **One deliberate SUPERSET departure, reported not hidden:** the PLAN enumerates the server SANs as `localhost` + `127.0.0.1`/`::1`; a second DNS SAN `l_conn_err.fixture.test` was added because every TLS fixture precedent in this tree dials with a fixture-distinct `ServerName`, and a driver following that idiom against a `localhost`-only leaf would fail for the wrong reason. Nothing was removed.

### T10 — the tree's FIRST `tls_params` YAML, BOOTED ON BOTH SIDES
⚠️ **This gated everything downstream and was run BEFORE any arm was written.** Re-derived at this tip: `git grep -n 'tls_params' -- '*.yaml' '*.yml'` returns **NO MATCHES** (rc=1) — `0120` is the first.

```
SUBJECT  ($SCRATCH/envoy-go -c … ; built with -o into scratch, and -c NOT --config-path):
   envoy-go listener l_conn_err ready on [::]:13121
   envoy-go ready
   process confirmed ALIVE after 10s (not a crash-then-exit)

REFERENCE (envoyproxy/envoy@sha256:7edd5b0f…453be8, digest re-verified against
           docs/envoy-go/ENVOY_TARGET.md:4 BEFORE any arm was trusted):
   [config] loading 1 listener(s)
   [main]   all clusters initialized. initializing init manager
   [config] all dependencies initialized. starting workers
   [main]   starting main dispatch loop
   grep -ic tls_params on the reference log -> 0
   the ONLY warning present is the unconditional global-downstream-connection-limit
   notice; it does NOT name tls_params

RETENTION (not mere tolerance):
   curl -s localhost:12901/config_dump | grep -c tls_minimum_protocol_version -> 2
   both render "tls_minimum_protocol_version": "TLSv1_2"
```

**Evidence beyond the PLAN's gate**, which forecloses two later wrong-reason failures — an `openssl s_client` mTLS probe with the committed `clientAuth` leaf against BOTH sides returned the echoed payload through `tcp_proxy`, proving the reference's `STRICT_DNS` + `host.docker.internal` cluster **actually reached the host backend** (i.e. not the silently-unreachable `STATIC`-by-bridge-IP failure mode); and a no-client-cert negative returned `tlsv13 alert certificate required … alert number 116` on **both** sides, proving `require_client_certificate: true` is live on both.

⚠️ **A PLAN-TEXT HAZARD, worked around and recorded:** the PLAN requires the pre-1.2 protocol-version enum values to be documented as forbidden while also requiring they never appear in the YAML. A first draft put the literal tokens in a `#` comment and `grep -n 'TLSv1_[01]'` then **MATCHED**, which would trip any later literal-text gate. Both comments were reworded to prose. Current state: `grep -n 'TLSv1_[01]'` over both files → **no matches**; the only protocol-version literal in either file is `TLSv1_2`.

### T15 — `0110` / `0111` prose: four files that went FALSE
All four sites said `ssl.connection_error` *"increments nothing — a named departure"*. Rewritten to record that phase 94 landed the name under a closed transport-exclusion predicate, and that **these two fixtures still observe it at 0 because every one of their arms is a certificate-path outcome** — so a pin there would document a vacuous `0 == 0`.

⚠️ **NO DRIVER CHANGE, and the reason was VERIFIED BY READING rather than assumed.** `0110/driver/driver.go:794-799` and `0111/driver/driver.go:774-778` iterate **CLOSED NAMED SUBSETS** (a `names` slice), so a fifth registered metric name cannot redden them. Adding the fifth name to those subsets would couple two unrelated fixtures to this row and break their **absolute** arm arithmetic.

⚠️ **`0108` IS DELIBERATELY OUT OF SCOPE.** Its two *"envoy-go emits NO `ssl.*` stats whatsoever"* confessions are **already false since phase 74** — PRE-EXISTING DRIFT, recorded as a deferred item, not this row's delta.

Both `expectations.yaml` files re-parsed with a real YAML parser after editing (the edits sit inside `#` comments, but that is a claim worth checking rather than assuming).

### T16 — ADR-0316 completed IN PLACE, house guard DISARMED
§Decision and §Consequences **APPENDED after the RETAINED italic footer**. No renumber, no `---`. Structural counts re-derived AFTER the edit, every one unchanged as required:

```
^---$      216 (STAYS)      ^## ADR-  315 (STAYS)      ^## 323 (STAYS — both new headings are ###)
tail       ## ADR-0316 (STAYS)        ^## ADR-0317 -> 0   =>  next-free STAYS ADR-0317
lines      18834 -> 18872
```
⚠️ **next-free is TAIL-derived, never from the heading count** — the id space is sparse with one gap at `ADR-0209`, so headings+1 reads **316**, an id already TAKEN.

**The guard, verified BY LINE AND BY ADR — never by the count alone:**
```
house strict   '^> \*\*STATUS: PROPOSED'    -> 0          <- DISARMED
ADR-0231 decoy '^\*\*Status:\*\* PROPOSED'  -> 1 at :14866
   backward heading search resolves :14866 to '## ADR-0231' at :14864
   — a DIFFERENT matcher entirely, BYTE-UNTOUCHED by this row
```
⚠️ **NO COUNT OF EITHER MATCHER IS WRITTEN INSIDE THE ADR BLOCK, DELIBERATELY** — that STATUS line was itself a hit of the strict form until this edit, so any figure it named would have been falsified by its own landing.

### T17 — the contract and the roadmap
**Twelve within-line edits across SEVEN `BEHAVIOR_CONTRACT.md` sites** (the five SPEC §11 pinned + the two §2.6 found). The file grows by exactly the **2** lines of the new ledger entry, `5987 -> 5989`; **every other edit is WITHIN-LINE**, because `:1042`, `:1967`, `:1971`, `:1973` and `:1975` are each one enormous paragraph line and a `sed`-style line replacement would destroy neighbouring propositions.

**The surgical edit verified afterwards, both directions:**
```
grep -c 'UNENUMERATED'                                  -> 1   (KEPT, as required)
grep -c 'still blocked on enumerating its membership'   -> 0   (DELETED, as required)
```
Those are **different propositions on the same line**, and only the second was to be removed.

**`ROADMAP.md:155`** — row 93's two rejection claims marked SUPERSEDED citing ADR-0316. **Both were FALSE WHEN WRITTEN**: the reference side was NOT unprobed (phase 77 probed it on 2026-07-26 and the result never left its BRAINSTORM; phase 94 re-probed it), and *"~4-6 production lines"* is refuted (35-55 measured, plus six prose blocks, four test files, four fixture-prose files and a new fixture). ⚠️ **A third half is a CATEGORY ERROR the PLAN did not name**: *"five `outcomeOther` test rows already waiting"* — `TestClassifyHandshakeErr` calls the classifier DIRECTLY and asserts the OUTCOME only, so it never reaches an increment site and could never have gated the counter. Those five rows are **byte-unchanged** by phase 94. **The stat-ledger half STANDS.**

**Row field counts under BOTH forms, before and after** — an unescaped `|` in a cell passes check (1) but breaks the field count:
```
row 93  naive NF=8   escape-aware NF=8      (UNCHANGED by the supersession edit)
row 94  naive NF=8   escape-aware NF=8      (7 pipe characters)
ROADMAP.md  244 lines  (UNCHANGED — the row-94 change is a FLIP, not an ADD)
```

### T11-T14 — fixture `0120-tls-connection-error`
**All three registration gates**, because a missing blank import is `t.Skipf`'d **SILENTLY GREEN** and no fixture-count gate exists anywhere in the tree: the directory (`driver/` layout), `RegisterFixture` from `init()` with the name STRING-EQUAL to the directory, and the blank import in `runner_test.go` — **the only file outside the fixture directory this row touches.** Both compile-time interface assertions are present, and they are MANDATORY: `AssertStats` dispatch is an unguarded type assertion with no `else`, so a signature typo makes `ok == false` and **the entire stats leg vanishes green.**

**Per-arm deltas, MEASURED with a before/after `/stats/prometheus` snapshot AROUND EACH ARM — reference and subject IDENTICAL on every row:**

| arm | `connection_error` | `handshake` |
|---|---|---|
| (v) valid + client cert, run FIRST | +0 | **+1** |
| (i) bad version (client `MaxVersion` TLS 1.1) | **+1** | +0 |
| (ii) plaintext HTTP to the TLS port | **+1** | +0 |
| (iii) garbage bytes | **+1** | +0 |
| (iv) **clean FIN — the DISCRIMINATING negative control** | **+0** | **+0** |

**Over-firing controls, both sides identical:** 3× clean-FIN → `connection_error` +0; 3× valid → `handshake` +3, `connection_error` +0; 2× bad-version → `connection_error` +2, `handshake` +0. ⚠️ **A positive arm cannot catch an over-firing counter**, which is why these were run.

Subject-side log confirms four distinct server errors reaching the classifier: `client offered only unsupported versions: [302 301]`, two × `first record does not look like a TLS handshake`, and a bare `EOF` — **the last SUPPRESSED by the predicate's `io.EOF` term**, which is precisely what arm (iv) exists to exercise.

⚠️ **`driveSide` returns a NON-NIL EMPTY `[]byte{}` on both sides, never the bytes it read**, because arm (iii) DIVERGES ON THE WIRE. Measured across all four combinations: **only arm (iii) on the REFERENCE** gets a fatal alert (`15 03 01 00 02 02 …`); arm (ii) on the reference and BOTH raw arms on the subject read `n=0 err=EOF`. A shared comment that claimed the alert for both arms was corrected by that measurement.

⚠️ **All discrimination lives in `AssertStats`**, keyed on the metric NAME with the label value deliberately unread — MEASURED, the reference renders `envoy_listener_address="0.0.0.0_10126"` where the subject renders `"___10126"`. `wantConnectionError = 3` and `wantHandshake = 1` are asserted as **exact equalities, not floors**: a floor could not distinguish a discriminating predicate from one that also fires on the clean-FIN arm (which would read 4). The **negative half** asserts both certificate counters stay 0 on both sides — without it, an implementation booking every failure under all three names would still pass the positive half.

**NCs 7-9:**

| NC | prediction | MEASURED |
|---|---|---|
| 7 (blank import commented out) | `t.Skipf`, silently green, exit 0 | ✅ `runner_test.go:201: no driver registered…` → `--- SKIP`, `RC=0` |
| 8 (`wantConnectionError + 1`) | RED on both sides | ✅ `RC=1`; **BOTH** sides report (`reference: … = 3, want 4` and `subject: … = 3, want 4`) ⇒ no `Fatalf` masking |
| 9 (rename `AssertStats`) | leg vanishes green, then the assertion catches it | ⚠️ **PLAN NOT EXECUTABLE AS WRITTEN — see below** |

⚠️ **NC 9 AS WRITTEN IS SELF-CONTRADICTORY.** With the compile-time assertion in place the rename is a **BUILD BREAK**, so it cannot also *"still pass"*. It was split:
- **9a** — renamed AND the assertion commented out, **with NC 8's deliberately-wrong pin left in place as a STACKED control**: `RC=0`, anchored FAIL **0**, `--- PASS`. **The fixture went green while carrying an assertion that must fail** — direct proof the leg VANISHED rather than merely passed. A plain green would not have distinguished those.
- **9b** — assertion restored, method still misnamed: build `RC=1`, `*connErrDriver does not implement fixture.StatsAsserter (missing method AssertStats)`. The guard fires.

---

## 4. The SIX-GATE POSTURE — ⚠️ NAMED, NOT CLAIMED

Every figure below is from this session's own runs on the final merged tree.

### (a) Differential suite — full
```
go test ./test/differential/ -count=1 -v
RC=0   RUN=139   anchored FAIL=0   PASS subtests=122   SKIP=0   PANIC=0
    --- PASS: TestDifferential/0120-tls-connection-error (1.77s)
```
**Fixture set reconciled BY NAME, BOTH directions** — a count alone cannot see a rename:
```
fixtures on disk 122 ; fixtures that PASSED 122
on disk but DID NOT RUN -> (empty)      RAN but not on disk -> (empty)
NC (0120 removed from the ran set)      -> reports 0120-tls-connection-error
```
The reconciliation is therefore **proven able to fire**, not merely observed empty.

⚠️ **THE FIRST ATTEMPT ABORTED THE TEST BINARY AND IS RECORDED, NOT DISCARDED.**
```
--- FAIL: TestDifferential/0081-grpc-access-log (0.10s)
panic: driver: start ALS receiver on 0.0.0.0:40065: listen tcp 0.0.0.0:40065:
       bind: address already in use [recovered, repanicked]
RC=1  RUN=96  anchored FAIL=4  PANIC=1   -> 0082-0120 NEVER RAN
```
This is the **registered driver-owned receiver port race**: `40065` sits inside `net.ipv4.ip_local_port_range` (32768-60999), so the driver's own receiver collides with an ephemeral allocation. It is **NOT a regression of this row** — the panic is in `0081`'s driver, which this row does not touch. ⚠️ **THE INSTRUCTIVE PART: `RUN=96` with `0120` ABSENT would read as a pass to any gate that checks only `RC` on a re-run, or that counts `--- PASS` lines without reconciling them against the fixtures on disk.** The by-name reconciliation above is what makes that undetectable failure detectable.

### (b) Non-Docker sweep — gated on `PIPESTATUS[0]` AND a set reconciliation
```
go list ./... | grep -vE '/test/differential$|/test/conformance/h2spec$'   -> 235 packages
RC=0   anchored FAIL=0   125 ok + 110 no-test-files = 235
package SET: wanted 235, reported 235; both comm directions EMPTY
```
Packages went **234 -> 235** — the `+1` is `0120-tls-connection-error/driver`, verified present in the list rather than inferred. ⚠️ **Both Docker drivers excluded**; `go test ./...` drives Docker in TWO places, not one.

### (c) h2spec conformance
```
RC=0   95 tests, 94 passed, 1 skipped, 0 failed
```

### (d) Fuzzers — reconciled against `^func Fuzz`
```
56 targets / 48 files    (+0 — this row adds no config field, so no parse arm)
```

### (e) The ANCHORED panic gate — ⚠️ AND IT IS PROVEN LIVE
```
grep -cE '^panic:|DATA RACE|SIGSEGV'  over g-a2.log, g-b2.log, g-c.log   ->  0, 0, 0
NC: the SAME gate over the ABORTED first differential log               ->  1   <- FIRES
```
⚠️ **A gate that reads 0 has not been shown to work.** Running it over a log known to contain a panic is what distinguishes *"nothing panicked"* from *"the gate is broken."*

### (f) `REVIEW.md`
**NOT PRODUCED — a STANDING DEPARTURE, NAMED NOT CLAIMED.** This project does not produce one; saying so is not the same as implying a review step ran.

### Per-package lint, gated on OUTPUT
```
gofmt -l internal/listener/ internal/stats/ test/fixtures/0120-tls-connection-error/ test/differential/   -> EMPTY
golangci-lint run ./internal/listener/... ./internal/stats/... ./test/fixtures/0120-.../...              -> EMPTY
```
⚠️ `gofmt -l` never exits non-zero, so the gate is on OUTPUT. `golangci-lint`'s misspell runs in locale **US**; the British-spelling sweep over every `.go` file touched found **none**.

### Known-flake register — ⚠️ A GREEN RUN CLEARS NOTHING
Live: the two SDS dial-budget flakes **plus `TestSDSEndToEnd_FetchFailure_BootFailsClosed`** (which **RECURRED once at this stage** — §2.12) · **the driver-owned receiver port race, which RECURRED at this stage and aborted a full differential run** (§4a) · `internal/httpclient` zero-value · the two 84.2-era flakes · the REFERENCE h2spec section-8 flip · `TestOutlierDetector_ConcurrentEjectExactlyOnce` · `0061-lb-ring-hash`'s σ-margin second occurrence, **which this project still owes an investigation** · `TestP83_StopPauseTimer_IsAuthoritativeAgainstAnEnteredClosure` (wasm, 1/400-trial; **did NOT recur here**). ⚠️ **`TestServerConn_TinyWindowDelivery` IS NOT A FLAKE** — phase 91 fixed a live production deadlock, and a recurrence would be a REGRESSION of row 91.

---

## 5. Sentinel — RE-RUN AFTER THE ROW-94 FLIP

⚠️ **NC SHAPES CHANGE ACROSS A ROW FLIP AND WERE RE-MEASURED ON BOTH SIDES OF IT, NEVER INHERITED.**

| | BEFORE the flip | AFTER the flip |
|---|---|---|
| check (1) | `NOT DONE: row 94` — ONE line | ⚠️ **SILENT** |
| check (2) | SIX | **SIX** (unchanged) |
| check (3) | SILENT | SILENT |
| NC-A (row 62 doctored) | **TWO** lines | ⚠️ **ONE** — `NOT DONE: row 62` |
| NC-B (`want=125`) | **TWO** lines | ⚠️ **ONE** — `GATE FAIL: examined 126 data rows, expected 125` |
| NC-C | fired, 0 residual | fired, 0 residual |
| NC-D | 96 / 68 | 96 / 68 |

Window md5s after the flip — **five BYTE-IDENTICAL to the inherited record**, and only `:226` changed, which is exactly the line this row deliberately narrowed:
```
204 10d7807bf02d   210 4a92f7e62fc6   216 2a7eb298b9fd
226 242e53c6f7a3   232 b2680e6f4fbf   240 6caa1c3ce0e7
      ^ narrowed by this row
```

Row field counts, **both forms, after the flip** — row 94 carries **7** pipe characters and stays NF=8 under BOTH, so it does **NOT** join the naive-malformed roster:
```
row 57  naive NF=13  escape-aware NF=9    (pre-existing; the two forms DISAGREE)
row 69  naive NF=10  escape-aware NF=10   (pre-existing; the two forms AGREE)
row 93  naive NF=8   escape-aware NF=8
row 94  naive NF=8   escape-aware NF=8    <- FLIPPED to `done`; ROADMAP.md STAYS 244 lines
```
⚠️ **THE FIRST DRAFT OF ROW 94'S SUMMARY READ NF=10 UNDER BOTH FORMS**, because the narrative contained two literal `|` characters (a `(driver|inputs)` alternation and a `sed` delimiter). Escaping them as `\|` fixed the escape-aware form but left naive at 10, which would have added row 94 as a **THIRD** naive-malformed row. Both were **reworded away entirely** instead. ⚠️ **COUNT THE FIELDS BEFORE INSTALLING — an unescaped pipe PASSES check (1) and silently breaks the field count.**

⇒ **THE SENTINEL DOES NOT FIRE.** `stop` was evaluated and **deliberately NOT created**; verified absent at the git root and in all four stage worktrees.

⚠️ **THE MARGIN IS NOW ONE.** Before this row the sentinel was blocked TWO independent ways — check (1) by an open row AND check (2) by six lines. **This IMPL spent the first.** Only check (2) blocks it now, and **narrowing a window does not reduce that count**: check (2) keys on the window's opening phrase, not on the clause. ⚠️ **DELETING THE LAST DEFERRED-CANDIDATE LINE WOULD END THE PROJECT — do not "tidy" one.**

---

## 6. Cost — MEASURED by `--numstat`, never by `--stat`

⚠️ **`git diff --stat` reports a SUM of additions and deletions, not additions** (`reference_git_diff_stat_is_sum_not_additions`). Every figure here is `--numstat`, against `master`, at the publishing tip.

```
     7      5  docs/envoy-go/BEHAVIOR_CONTRACT.md          <- 12 within-line edits, 7 sites; +2 net = the ledger entry
    39      1  docs/envoy-go/DECISIONS.md                  <- ADR-0316 §Decision + §Consequences + the STATUS disarm
     3      3  docs/envoy-go/ROADMAP.md                    <- row 93 supersession, window :226 narrowing, row 94 FLIP
   514      0  docs/envoy-go/phases/94-.../PROGRESS.md
    73     12  internal/listener/manager.go
   419     21  internal/listener/manager_test.go
    14     11  internal/listener/quic_test.go
     1      0  internal/stats/helptext_test.go
     8      4  internal/stats/name.go
     9      4  internal/stats/name_test.go
     1      0  test/differential/runner_test.go            <- the ONLY file outside the fixture dir
     8      3  0110/README.md          7  3  0110/expectations.yaml
     6      2  0111/README.md          6  3  0111/expectations.yaml
   262      0  0120/README.md        242  0  0120/expectations.yaml
   618      0  0120/driver/driver.go
    89      0  0120/envoy-go.yaml    103  0  0120/envoy.yaml
    45      0  0120/pki/  (5 files)
------
  2474     72  TOTAL          of which .go:  +1143 / -52
```

⚠️ **THE TABLE ABOVE WAS MEASURED BEFORE THE STAGE-CLOSE ARTIFACTS LANDED, AND IS THEREFORE A SUBSET.** `reference_cost_figure_measured_at_publishing_commit`: a cost figure must be measured at the **PUBLISHING** commit. Re-measured on the staged squash:

```
publishing commit:  28 files    deletions -181    .go  +1143 / -52
  close artifacts:  docs/envoy-go/STATE.md            9 /   9
                    docs/envoy-go/STATE_HISTORY.md    2 /   0
                    next-prompt.txt                  95 / 100
```
⚠️ **NO TOTAL-ADDITIONS FIGURE IS QUOTED HERE, DELIBERATELY, AND THE REASON IS THIS FILE.** `PROGRESS.md` is itself one of the 28 files in the commit, so any total written into it is falsified by the act of writing it — **and that happened, twice, before this paragraph was settled**: recording the total made the file longer, which moved the total. That is `reference_self_incrementing_positive_control` again, and it is the same shape as §2.13, where a sentence written inside a sentinel window moved the sentinel.

**THE STABLE FIGURES, which do not rot:** deletions are **-181**; `.go` is **+1143 / -52**; the row's subject-only cost, excluding the three close artifacts AND this file, is **+2474 / -72**. Anyone wanting the shipped total must take it from `git show --numstat` at the merge commit, where it is a fact about the commit rather than a claim inside one of its own files.

**Against the PLAN's ESTIMATE (§7), which labelled itself an estimate and a LOWER BOUND:** predicted `≈+1608 / -25` total with `≈+995` `.go`. **Measured `.go` `+1143 / -52` — 1.15× the estimate — and deletions `-181`, over SEVEN times the predicted `-25`.** The shipped addition total exceeds the estimate by more than half again, but is not quoted here for the reason given above. ⚠️ **`reference_measured_prototype_is_a_lower_bound` HOLDS AGAIN — an ELEVENTH consecutive row** — and the gap is concentrated exactly where the PLAN warned it would be: prose and enumeration, not code. `PROGRESS.md` alone (514 lines) is un-budgeted by the estimate, and `0120`'s `README.md` + `expectations.yaml` came in at **504** against the fixture's whole `≈+1150` anchor.

**Axis deltas, each MEASURED at this tip:** fixtures **121 -> 122** (tail `0120`) · packages **234 -> 235** · Go test packages `ok` **125** · stat surface **+1 NAME — a DELTA ONLY, with NO absolute quoted outside the ledger's own internal chain**, three contested absolutes being live in this tree at one tip (`reference_a_drift_correction_is_itself_a_claim`) · BackendKind tail **38 (+0)** · fuzzers **56 / 48 (+0)** · `go.mod` **67 require entries (+0)** · phase dirs **135 (+0)** · `^---$` **216 (+0)** · `^## ADR-` **315 (+0)** · ROADMAP data rows **126 (+0 — a FLIP, not an ADD)** · `ROADMAP.md` **244 lines (+0)**.

---

## 7. What this row did NOT do — banked, not chartered

- ⚠️ **A SEVENTH `outcomeOther` arm exists and is now silently counted.** ALPN negotiation failure reaches the Inc site today, and `TestNewManager_LiveHandshake_ALPNNegotiationFailure_Aborts` **now increments `ssl.connection_error` while asserting nothing about it.** It is absent from `SPEC.md` §2.2's six-row table. It is not added to the unit table here because it needs an ALPN-configured listener; against `startOneWayTLSListener` the handshake SUCCEEDS. **A cheap follow-on.**
- `0108`'s two *"envoy-go emits NO `ssl.*` stats whatsoever"* confessions — **false since phase 74**, pre-existing drift, outside this row's delta.
- `0118/driver/driver.go:31`'s falsified *"TLS/SDS band"* characterization (`0112`/`0113` carry ZERO `DownstreamTlsContext`).
- **No `len(helpText)` guard exists**, and `name.go` still carries two ungated prose counts.
- `manager.go`'s cite of `handshake_server.go:964, go1.26.5` — at the live toolchain (go1.26.7) that text is at `:970`. A stdlib cite that drifts with the toolchain; the tripwire itself is sound because the test builds the error from a LIVE handshake.
- **Both unexercised predicate members** (`net.ErrClosed`, `context.DeadlineExceeded`) and the synthetic `errors.New("connection reset by peer")` have no measured production producer — recorded as residuals, **not claimed impossible**.
- The other TEN fixed `ssl.*` names and the FOUR dynamic families (blocked on NAMING: the stat-name charset bans the hyphen, so an OpenSSL-form cipher name fails validation and panics at registration; `sigalgs` may be unimplementable, `crypto/tls.ConnectionState` exposing no signature-scheme field).
- **`0061-lb-ring-hash`'s σ-margin second occurrence** — a real follow-on the project still owes.
- ⚠️ **The driver-owned receiver port race is now TWICE-observed at close range** and cost a full 400-second differential run at this stage. Its band sits inside `net.ipv4.ip_local_port_range`. **Worth chartering.**
