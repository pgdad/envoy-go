# Phase 92 — `h2-response-header-validation` — IMPL PROGRESS

Working `PLAN.md` §14 **T1 -> T23** in order. One section per task, appended as it lands.
**Every figure below is ACTUAL measured output, never a forecast.** Where a measurement
DIVERGES from what an earlier stage claimed, the divergence is stated as a FINDING rather
than laundered.

- **Stage worktree:** `/home/esa/git/wt-92-impl`, branch `phase-92-impl`
- **Base:** master tip `7dfd7cb6`
- **Toolchain:** `go1.26.7 linux/amd64`

---

## SENTINEL — re-run MECHANICALLY at this tip BEFORE any work, per the standing method note

ACTUAL output, recorded not predicted:

- **(1)** `want=124` -> **`NOT DONE: row 92`** — ONE line.
- **(2)** **SIX**, at `:202 :208 :214 :224 :230 :238`.
- **(3)** **SILENT.**

⇒ **TWO checks block the sentinel. It does NOT fire. `stop` was EVALUATED and DELIBERATELY
NOT CREATED** (verified absent at the git root and in the stage worktree).

**All four NCs FIRED:**

| NC | ACTUAL |
|---|---|
| NC-A (doctor row 62) | `NC LANDED? [ in-progress ]` **inspected FIRST**, then **TWO** lines: `NOT DONE: row 62` + `NOT DONE: row 92` |
| NC-B (denominator `want=123`) | **TWO** lines: `NOT DONE: row 92` **AND** `GATE FAIL: examined 124 data rows, expected 123` |
| NC-C (check-3 control) | residual **2 -> 0** ⇒ `NEVER OPENED: gRPC` fired; the WASM control still reads **2** and stays correctly silent |
| NC-D (check-2 forms) | long **5** / short **1** / union **6** |

⚠️ **The pre-flip NC-A and NC-B shapes are BOTH two-line.** They are re-measured AFTER the
row-92 flip at T23, because the flip changes both shapes.

---

## T1 — pre-edit baselines. **ZERO production bytes.**

Run by a pure-measurement agent that committed nothing; tree proven clean at close
(`git status --porcelain` 0 bytes, `git diff --stat` 0 bytes, HEAD still `7dfd7cb6`).

| sub | figure | **ACTUAL** | PLAN expected | verdict |
|---|---|---|---|---|
| T1a | RC | **0** | 0 | MATCH |
| T1a | RUN (raw `=== RUN`) | **655** | 655 | MATCH |
| T1a | FAIL (**anchored** `^(FAIL\|--- FAIL)\|^ *--- FAIL`) | **0** | 0 | MATCH |
| T1a | run-name roster, `sort -u` | **655** | — | equals the raw count ⇒ `sort -u` dropped nothing |
| T1b | `./internal/cluster/...` RC / RUN / FAIL | **0 / 458 / 0** | — | green |
| T1b | `TestOutlierDetector_ConcurrentEjectExactlyOnce` | **`--- PASS`** | — | the known one-off flake did NOT fire |
| T1c | `gofmt -l` over the 10-file touch roster | **empty output** | clean | MATCH — **gated on OUTPUT**, not rc |
| T1d | `golangci-lint run` (3 package trees) | **empty output**, rc=0 | clean | MATCH — lint IS installed |
| T1e | `go vet` (3 package trees) | rc **0**, **empty** | clean | MATCH |
| T1f | differential `0004-h2-routing` RC | **0** | 0 | MATCH |
| T1f | literal `=== RUN   TestDifferential/0004-h2-routing` | **PRESENT (1 hit)** | must appear | **NOT VACUOUS** |
| T1f | `no tests to run` | **0** | 0 | selector genuinely matched |
| T1f | anchored panic gate `^panic:\|DATA RACE\|SIGSEGV` | **0** | 0 | MATCH |
| T1g | fuzz targets / FILES | **55 / 48** | 55 / 48 | MATCH (files cross-checked by two command forms) |
| T1g | fixtures (`ls -d test/fixtures/*/`) | **121** | 121 | MATCH |
| T1g | BackendKind **tail VALUE** | **38** (`H2GoawayResponder`, `fixture.go:614`) | 38 | MATCH |
| T1g | BackendKind **declared COUNT** | **39** (contiguous 0..38) | 39 | MATCH |

Decisive verbatim lines:

```
ok  .../internal/filter/hcm        0.017s
ok  .../internal/filter/hcm/h2     1.886s
ok  .../internal/filter/http/router 2.087s
ok  .../internal/cluster           4.263s
--- PASS: TestDifferential/0004-h2-routing (3.58s)
ok  .../test/differential          3.657s
```

**Method compliance recorded, because each rule has drawn blood in this lineage:** every
`go test` used `-v -count=1`; rc captured **without** a pipe (`out>log; rc=$?`), never as
`rc=$?` after one; every `grep -c` guarded with `|| true`; FAIL counts **anchored**;
`gofmt`/`golangci-lint` gated on **OUTPUT**, not rc.

**Docker:** T1f created and terminated **its own** container only. ⚠️ Foreign containers
observed and **left untouched**: `infallible_booth`, `crazy_kare`, `golink-ai`,
`quizzical_goldstine`, plus a **fifth** session-foreign container seen running
(`keen_spence` / later `priceless_cannon`) — recorded because the standing note names only
four, and the roster is therefore **not closed**.

All ten files on the T1c touch roster **EXIST** at this tip — no phantom path.

---

## RECON — the touch-site map, re-derived at the IMPL tip

A read-only agent re-derived every symbol, signature and line number the PLAN cites,
against the tree as it actually is. **NINE divergences from the PLAN/SPEC were found.**
They are stated as FINDINGS, not smoothed over — this stage's job is to refute its
predecessor by execution.

### ⚠️ F7 — THE PLAN'S "NO YAML CHANGE" IS FALSE AS WRITTEN, AND IT BLOCKED T2
PLAN §8/T2 names the new backend paths `/p92-keepalive`, `/p92-upgrade`, `/p92-proxyconn`
and states they *"fall through the existing `- match: { prefix: "/api" }`"*.
**They do not.** A TOP-LEVEL `/p92-*` path does not match prefix `/api`; it falls to
`- match: { prefix: "/" }` and is answered by a **404 `direct_response`** — it never reaches
the backend at all. The `/api` route to `c_h2_backend` is `envoy.yaml:157` / `envoy-go.yaml:155`;
the in-fixture p90 precedent correctly uses `/api/v1/reflect-headers/p90p|p90a|p90b`.
⇒ **The p92 paths MUST live under `/api`.** With that correction the "**ZERO YAML lines**"
claim becomes genuinely true rather than accidentally false.
⚠️ Had this not been caught, the anchor would have gone RED **for the wrong reason** — a 404
on both sides is not the divergence this row is about, and a cross-side-identical 404 could
even have read as GREEN. **This is a fail-unsafe defect, not a typo.**

### ⚠️ F2 — `validateResponseTrailers` HAS NO DOC COMMENT
`client.go:732-774` is the rule-set doc block that READS as `validateResponseTrailers`'s
documentation, but `:775` begins `// hasUppercaseHeaderChar reports whether…` with **no blank
line at 774/775**. So `:732-786` is ONE contiguous comment run, which godoc binds to
`func hasUppercaseHeaderChar` (`:779`). `func validateResponseTrailers` (`:788`) therefore has
**no doc comment at all**. Any instruction to "edit the `validateResponseTrailers` doc comment"
is under-specified: the block to edit is `:732-774`, and attaching it would require inserting a
blank line or moving it.

### ⚠️ F1 — `hasUppercaseHeaderChar` IS IN `client.go`, NOT `stream.go`
It is at `client.go:779`. No such symbol exists in `stream.go`. (`isConnectionSpecificField`
`stream.go:392` with its doc at `:379-391`, and `const teTrailersValue = "trailers"` at
`stream.go:377`, ARE where the PLAN says.)

### ⚠️ F3 — `startH2TrailerBackend` HAS **SIX** CALL SITES, NOT FIVE
`router_h2_trailers_test.go:201, 237, 567, 617, 788, 880`; `runH2TrailerBackend` has ONE (`:179`).
T9 therefore threads through **7 call expressions + 2 signatures**, not the PLAN's 5.

### ⚠️ F4 — THE AST-GOLDEN LINE ANCHORS ARE BOTH OFF BY A LITTLE
The doc enumeration is `:500-508` (PLAN said `:505-513`); `want := []int{0, 0, 502, 502, 503, 503, 503}`
is at `:526` (PLAN said `:528` — that line is the `t.Fatalf` that CONSUMES `want`, not its declaration).
The finding is not the offset itself but that **a by-line edit instruction aimed at `:528` would
have patched the wrong statement.**

### ⚠️ F5 — THE "17 -> 18 CENSUS" HAS NO 17 IN IT
`TestActionResponseLiterals_OnlySuccessSitePopulatesTrailers`'s census `t.Logf` (`:474`) prints
`len(lits)` — it **hard-codes no number**. There is no literal `17` in the source to become `18`.
The nearby numeric guards are `len(withTrailers) != 1` (`:482`) and `found != 2` (`:554`), and
neither is the census. The PLAN's *"expect that number to move"* is about a RUNTIME-PRINTED
value, not a source constant.

### ⚠️ F6 — THE PLAN NAMES THE WRONG FUZZ PRECEDENT
`fuzz_test.go` holds exactly **2** targets: `FuzzFrameStream` (`:24`) and `FuzzHPACKDecode` (`:96`).
PLAN §9 says the corpus should use *"the seed shape `FuzzFrameStream` already uses"* — but
`FuzzFrameStream` **never builds `[]hpack.HeaderField`**; it feeds raw bytes to a `ServerConn`
via `replayConn`. The actual precedent for decoding `[]byte` into header fields is
**`FuzzHPACKDecode`** (`newHPACKState(4096).encodeHeaders(...)` to seed, `decodeBlock(block, true)`
in the body).

### ⚠️ F8 — ROUTER TESTS CONFIGURE RETRY IN GO, NOT YAML, AND NEED AN EXTRA CALL
There is no YAML retry path in these tests. The seam is
`NewRetryPolicy(retryOn string, numRetries uint32, retriableCodes []uint32, base, max, perTry time.Duration)`
(`retry.go:58`), passed to `H2ClusterAction(c, nil, cluster.SubsetMatch{}, rp, nil)`.
⚠️ **`c.EnsureRetryStats()` MUST be called first** or every retry-counter assertion — including
the `== 0` ones — is **VACUOUS**. This is precisely the shape T13/NC6 exist to prevent.

### F9 — distribution anchors
`AssertDistribution` count gate at `driver.go:1060`; `want := [3]uint64{3,3,3}` at `:1071`.

### CONFIRMED EXACTLY AS THE PLAN SAID
`h2RxResetInc`/`h2TxResetInc` at `h2pool.go:201`/`:205` · `dialClientConnTCP` at
`trailers_validate_test.go:291` · `- match: { prefix: "/api" }` present on BOTH sides ·
the wrong 1xx claim at `client.go:673` · the quoted-field-name discipline at `:769-774` ·
`client.go` already imports all four of `errors`, `strconv`, `http2`, `hpack` (no new imports) ·
`onResponseHeaderBlock` `:609` with `cs.respHeaders = decoded` at `:635` and the trailer reject
mechanics at `:688-696` in the exact order the PLAN froze ·
`registerClusterMetrics` `manager.go:112`, `prefix :=` `:113`, `if c.useH2 {` `:194`,
rx/tx registrations `:200`/`:201` · `incCounter` `connpool.go:212` ·
`doH2ClusterAction` `router_h2.go:73` with the trailer sentinel arm ABOVE `EvictH2ConnOnError`.

---

## T2 + T3 — **THE RED ANCHOR.** Five shapes, each proven RED INDEPENDENTLY.

Commit `b9e1aa75`, plus the README de-rot at `eb28549e`. **ZERO production bytes** — the anchor
is recorded against the UNMODIFIED subject, which is what makes it a genuine anchor rather than
a build break (`reference_liveness_break_needs_failing_baseline`).

### ⚠️ T2's MEASUREMENT OBLIGATION DISCHARGED: **BOTH `te` SHAPES LEAK**
PLAN §8.1 left this UNMEASURED and forbade inferring it from `x/net/http2/server.go` — arm 7 being
the standing proof that a code-reading of that exact file gets it wrong. It was **MEASURED**: a
standalone raw-framer probe dialled the fixture backend **DIRECTLY, with no proxy in the path**
(port 21743, in-band below 32768) and dumped every response field on the wire:

```
=== /api/v1/p92-te-gzip   (5 response fields)   "te" = "gzip"
=== /api/v1/p92-te-empty  (5 response fields)   "te" = ""
=== /api/v1/9             (4 response fields)   <- NEGATIVE CONTROL, no illegal name
```
⚠️ **The `/api/v1/9` control is what makes this non-vacuous** — without it, "the field is absent"
and "the instrument cannot see the field" are indistinguishable.

⇒ Both kept as **permanent wire arms**. **THE DEPARTURE NARROWS FROM SIX UNIT-ONLY SHAPES TO FOUR**
(`connection`, which `x/net` deletes; `transfer-encoding`; an uppercase wire name; a duplicate
`content-length`). The differential covers **FIVE of NINE** shapes on the wire, not three.

### ⚠️ THE PLAN'S ROUTING CLAIM WAS FALSE AND IT WAS FAIL-UNSAFE
The five paths are `/api/v1/p92-{keepalive,upgrade,proxyconn,te-gzip,te-empty}` — **under `/api`**,
NOT the top-level `/p92-*` the PLAN specified. **ZERO YAML lines changed**, so the PLAN's cost claim
survives; its ROUTING claim did not.
**PROVEN TO ROUTE, not assumed:** temporary `log.Printf` instrumentation, run once, then `driver.go`
restored from a **sha256-verified** backup (`84b91d98…d86dba` identical before and after). Observed:
**subject 200 body `"p92-ok"`; reference 502 body `"upstream connect error or disconnect/reset before
headers. reset reason: protocol error"`. NEITHER side returned 404 on any of the five.**

### ⚠️ THE COMPARATOR IS FIRST-DIVERGENCE AND **DOES** MASK ARMS 2-5
`CompareBytes` (`diff.go:19`) stops at the first divergence, so the full-suite run proves ONLY the
`keepalive` arm. Reporting "it went red" would have been a claim about five shapes backed by evidence
for one (`reference_first_divergence_comparator_masks_arms`).
⇒ **Each remaining shape got its OWN differential run with a single-arm roster.** All four:
rc=1, RUN line present ×1, panic gate 0, anchored FAIL=5.

| arm | reference | subject |
|---|---|---|
| keepalive | `eepalive:status=502 illegal=<non` | `eepalive:status=200 illegal=keep` |
| upgrade | `-upgrade:status=502 illegal=<non` | `-upgrade:status=200 illegal=upgr` |
| proxyconn | `roxyconn:status=502 illegal=<non` | `roxyconn:status=200 illegal=prox` |
| te-gzip | `-te-gzip:status=502 illegal=<non` | `-te-gzip:status=200 illegal=te c` |
| te-empty | `te-empty:status=502 illegal=<non` | `te-empty:status=200 illegal=te c` |

Full run: rc=1, `=== RUN   TestDifferential/0004-h2-routing` **present (=1)**, panic gate **0**,
anchored FAIL **5**, and the ONLY assertion that fired was `runner_test.go:1289` (the byte compare).
**No `distribution:` line fired ⇒ `AssertDistribution` `[3,3,3]` still passes and `BackendCount()`
stays 3 — established BY BEHAVIOR, not by inspecting the constant.**

### COST — ACTUAL vs PLAN, a FINDING
```
 54   0  test/fixtures/0004-h2-routing/backends/main.go   (PLAN: +31/-0)
330   4  test/fixtures/0004-h2-routing/driver/driver.go   (PLAN: +162/-0)
=> +384 / -4                                              (PLAN: +193/-0)
```
**~2× the PLAN.** Drivers: the two extra `te` arms the measurement BOUGHT (unpriced by any earlier
stage, because nobody had measured that they leak), plus the p90-idiom doc discipline. The `-4` is
`driver.go`'s own 42 -> 47 request-count de-rot.

### FURTHER FINDINGS
1. ⚠️ **The reference rejects all five shapes IDENTICALLY** (502, `content-length` 87), so **status
   alone cannot discriminate the shapes.** The discriminator is the SUBJECT-side illegal-name SET —
   which is exactly why §8.2 requires the transcript to record the SET and one path per shape.
2. ⚠️ **`clfields=1` reads 1 on BOTH sides for every arm**, so it is an **INVARIANT THROUGH THE FIX,
   not a discriminator.** Recorded so it is never mistaken for a passing assertion.
   Convergence target after the fix: `status=502 illegal=<none> clfields=1`.
3. **The fixture README was made STALE by this very change** (42 -> 47 requests) and, the agent being
   single-file scoped, was flagged rather than silently fixed. Landed separately at `eb28549e`.
   ⚠️ **`driver.go:471`'s `total 42/side` was DELIBERATELY LEFT ALONE** — it is a RUNNING total at the
   phase-90 block and 9+9+9+1+1+1+1+8+3 = **42** is CORRECT there. A bare grep for the digits would
   have "fixed" a correct comment.

### HYGIENE
Exactly two `.go` files changed vs master, both under `test/fixtures/0004-h2-routing/`; the sibling
agent's `router_h2_trailers_test.go` was never opened. `git status --porcelain` EMPTY. gofmt no
output, `go vet` rc=0, backend build rc=0, golangci-lint no output.
**Docker:** the same four foreign containers before and after; none created or removed by this agent.
⚠️ **ONE SELF-CAUGHT INCIDENT, RECORDED RATHER THAN BURIED:** the first probe backgrounded an entire
`&&` chain, so `$!` named the SUBSHELL and left an orphan backend process. It was located with
`pgrep -laf` (**not** `pkill -f`, which matches the agent's own shell and kills the tool call with
exit 144), killed BY PID, and **the reported measurement is the clean re-run on a fresh port.**

---

## T9 — the h2 test backend gains a caller-supplied LEADING header block

Landed on a private worktree/branch (`phase-92-t9`, commit `a6c83d40`), merged into the stage
branch afterwards. **TEST-SIDE ONLY — one `_test.go` file, zero production bytes.**

### The design decision, and why it DEPARTS from the PLAN's two priced options
PLAN T9 offered **(a)** a FIFTH `h2TrailerBehavior` value **plus** a caller-supplied leading-header
slice threaded through both functions, or **(b)** a third standalone backend (~57 lines).

**Shipped: a REFINEMENT of (a) — the caller-supplied ordered slice, WITHOUT the new enum value.**
The leading block is **ORTHOGONAL to the frame sequence the enum selects**: `h2TrailerPeerReset`
short-circuits at `:100-105` BEFORE any leading block is written, while `h2TrailerNone` and
`h2TrailerEmit` both fall through to it. Threading the slice therefore gives **both surviving arms
the seam for free**; adding an enum value would have duplicated one arm's frame sequence purely to
carry the payload, and left the other arm without the seam.
**(b) rejected on measurement:** a standalone backend must re-implement the preface/SETTINGS
handshake AND the connection-scoped hpack encoder. The file's own banner justifies backend
separation because `runH2Backend` is shared by FOUR test files — that reasoning does **not**
transfer, because `runH2TrailerBackend` is used by **this file alone**.

**MEASURED COST: `47 added / 13 removed`, ONE file** — from `--numstat`.
⚠️ `--stat` would have read **60**, because it reports a SUM
(`reference_git_diff_stat_is_sum_not_additions`). ~29 of the 47 lines are documentation.

### New signatures, verbatim
```go
func runH2TrailerBackend(conn net.Conn, behavior h2TrailerBehavior, body []byte, trailers []hpack.HeaderField, leading []hpack.HeaderField)
func startH2TrailerBackend(t *testing.T, pki *h2BackendPKI, behavior h2TrailerBehavior, body []byte, trailers []hpack.HeaderField, leading []hpack.HeaderField) net.Listener
func h2DefaultLeadingBlock(body []byte) []hpack.HeaderField
```
⚠️ `leading` is appended **LAST deliberately**: it shares the type `[]hpack.HeaderField` with
`trailers`, so placing it BEFORE `trailers` would let a swapped call site **compile silently at all
six sites**. `lead == nil` (not `len(lead) == 0`) selects the default, so a non-nil EMPTY slice
stays expressible as a genuinely empty block.
⚠️ **Residual hazard, recorded not fixed:** the two type-identical slice parameters cannot be
order-checked by the compiler. Position mitigates it; any future THIRD slice parameter inherits it.

### ⚠️ THE SEAM IS PROVEN LIVE — WITH A DISCRIMINATING NEGATIVE CONTROL
A green build is not evidence a seam is live. Three temporary probe arms were driven and then deleted:
- **Wire arm** (raw framer, ONE hpack decoder per CONNECTION) decoded exactly: `:status 200`,
  `content-type text/plain`, `content-length 10`, **`X-Upper-Case yes`**, **`content-length 99`**
  ⇒ the seam carries an UPPERCASE name AND a DUPLICATE `content-length` **verbatim, in wire order**.
- **Client arm** through `doH2ClusterAction` saw all four non-pseudo fields verbatim, asserted by
  explicit counts (uppercase == 1, content-length == 2).
- **Default arm** proved `leading == nil` reproduces the pre-seam three-field block byte-for-byte.
- ⚠️ **NEGATIVE CONTROL:** forcing `if lead == nil` to `if true` reddened arms 1 and 2
  (`len(got)=3, want 5`; `len(Headers)=2, want 4`) while arm 3 correctly STAYED GREEN.
  **The probe discriminates the seam itself, not merely "the backend responds."**

### Gates
| gate | before | after |
|---|---|---|
| `go test -v -count=1 ./internal/filter/http/router/...` | RC=0, RUN=**122**, anchored FAIL=0 | RC=0, RUN=**122**, anchored FAIL=0 |
| `go build ./...` / `go vet` | — | rc 0 / rc 0, no output |
| `gofmt -l` / `golangci-lint` | — | both **empty output** |

`TestActionResponseLiterals_DoH2ClusterActionNonSuccessSites` PASSES and is absent from the diff
(`grep -c 'want := []int'` over the diff = **0**) — the AST golden is untouched, as required.

### FINDINGS
1. ⚠️ **The PLAN says FIVE `startH2TrailerBackend` call sites; there are SIX** — independently
   confirmed at the IMPL tip, matching recon's F3. Real threading cost: **7 call expressions + 2 signatures**.
2. ⚠️ **THE PLAN'S "FIFTH `h2TrailerBehavior` VALUE" MISCOUNTS THE ENUM ITSELF — there are exactly
   THREE values, so a new one would be the FOURTH.** That is the **SECOND wrong count in the same
   PLAN sentence**, and neither survived contact with the code.
3. The PLAN's new enum value is not merely unnecessary but **strictly worse** — see the design note.
4. ⚠️ **THE misspell LOCALE-US TRAP FIRED FOR REAL, NOT AS A HYPOTHETICAL** — `honoured` and
   `normalisation` failed `golangci-lint` on the first run and were corrected. The standing method
   note earned its place again.
5. **Confirms the row's premise for T12/T13:** the response LEADING block is entirely unvalidated at
   this base commit — the `content-length` reject at `client.go:808-810` is TRAILER-scoped and
   `stream.go:273-276` is REQUEST-scoped — so their anchors genuinely start RED.

---

## T4-T8 — PRODUCTION. The leading block is validated before it is retained.

Commit `aa4f36a9` on a private branch, merged at `89a95d91`. Every symbol assertion was
**pathspec-scoped** and used `grep -F` where a parenthesised receiver was involved
(`grep -E 'func (cc *ClientConn)'` returns a FAIL-UNSAFE ZERO — ERE reads the parens as a group).

| task | proof (`grep -nF`, pathspec-scoped) |
|---|---|
| **T4** sentinel | `802:var ErrMalformedResponseHeaders = errors.New("malformed response headers")` · `809:func malformedResponseHeadersError(streamID uint32, msg string) *Error {` |
| **T5a** validator | `933:func validateResponseHeaders(streamID uint32, fields []hpack.HeaderField) *Error {` |
| **T5b** stale doc | `stream.go:386` + `:391` — `isConnectionSpecificField`'s **EXHAUSTIVE caller list** now names `validateResponseHeaders` as the THIRD caller. Leaving it would have shipped a comment this row falsifies. |
| **T6** call site | `681:\t\tif verr := validateResponseHeaders(streamID, decoded); verr != nil {` — mechanics in the frozen order, `respHeadersSeen` **UNSET** on the reject path |
| **T7** router arm | `228:\t\tif errors.Is(err, h2.ErrMalformedResponseHeaders) {` — AFTER the evict, BEFORE the ctx-cancel check; import block **verified unchanged** |
| **T7** golden | `563:\twant := []int{0, 0, 502, 502, 502, 503, 503, 503}` + the doc enumeration at `:543` |
| **T8** counter | field `cluster.go:271` · boot registration `manager.go:208` inside `if c.useH2 {` · `h2pool.go:211 func (c *Cluster) h2RxMessagingErrorInc() { incCounter(c.http2RxMessagingError) }` · hook injection `dial_h2.go:82` · codec field `client.go:108`, option `:180`, fire site `:690` |

### ⚠️ PLAN §10.2 REPRODUCED EXACTLY — the AST golden PROVED itself a live guard
Both states were measured **deliberately**, because measuring only the green one would have left the
golden's liveness unproven:

**(a) production arm in, golden NOT yet edited — RC=1, RUN=655, anchored FAIL=4.** The AST golden was
the sole failing test:
```
router_h2_trailers_test.go:562: doH2ClusterAction non-Trailers ActionResponse Status set =
    [0 0 502 502 502 503 503 503] (n=8), want [0 0 502 502 503 503 503] (n=7)
--- FAIL: TestActionResponseLiterals_DoH2ClusterActionNonSuccessSites (0.00s)
```
**(b) golden edited — RC=0, RUN=655, anchored FAIL=0.**
**RUN=655 in BOTH states and at the T1 baseline** — T9 added no test, **verified rather than assumed**.

### The census `t.Logf`: 17 before, 18 after — and RECON F5 confirmed
It moved as the PLAN said it would, and it did **NOT** redden. But it **hard-codes no number**: it
prints `len(lits)`. ⇒ **the PLAN's "17 -> 18 constant" has nothing to edit.** Recorded so its silence is
never read as invariance.

### The counter hook — a NEW option, not a widened one
`func WithRxMessagingErrorHook(onRxMessagingError func()) ClientConnOption`.
`WithResetHooks` was MEASURED to have exactly **3** call sites (`dial_h2.go:81`, `client_test.go:1151`,
`:1198` — the other six grep hits are PROSE), two of them in a test file another task owns and neither
having anything to say about messaging errors. **New option = +1 call site, 0 edits to existing ones.**
The codec stays free of any `internal/stats` import.

**`IsValidName` RUN, not reasoned:** `IsValidName("cluster.c_h2s.http2.rx_messaging_error") = true`,
with six negative controls (space, hyphen, trailing dot, leading digit, empty, trailing newline) all
`false`. ⚠️ **No uppercase NC was used — it PASSES** (`a-zA-Z` appears in both character classes of
`^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$`), so a proposed uppercase NC would have been vacuous.
**The +1 stat delta is asserted STRUCTURALLY:** `NewCounter` call sites in `manager.go` **14 -> 15**.
⚠️ **No `406` was written anywhere** — it is not the stat surface (`grep -c '406'` on
`BEHAVIOR_CONTRACT.md` reads **0**).

### COST — ACTUAL `+218 / -8` across **EIGHT** files, vs the PLAN's `+174 / -1` across THREE
- Inside the PLAN's own three files: `client.go` +142, `router_h2.go` +35, golden +5/-2 = **+182/-2**.
  The `+8/-1` excess there is the mandated rationale comments, not code.
- **FIVE files the PLAN never priced:** four `internal/cluster/` files **+28/-1** (the hook wiring, which
  PLAN §4 explicitly EXCLUDED) and `stream.go` **+8/-5** (T5's second half, which PLAN §4 never names).
⚠️ **This is `reference_measured_prototype_is_a_lower_bound` firing for the THIRD time on this row, with
the SAME identified mechanism each time: UNDER-ENUMERATION OF *FILES*, NOT OF LINES.**

### FINDINGS
1. **The frozen design survived contact verbatim.** Every symbol PLAN §4.4 claimed was present at the
   claimed shape. The ONLY textual change was `BEHAVIOUR` -> `BEHAVIOR` (the locale-US misspell gate).
2. ⚠️ **RECON F2's repair was DELIBERATELY DECLINED, with reasoning.** Inserting a blank line does NOT
   fix the mis-bound doc block — it DETACHES the block and binds it to nothing. The real repair is a
   43-line block MOVE: uncharted, gratuitous merge surface against T16 in the same file, and it would
   make the numstat unreadable. ⚠️ **The defect was NOT EXTENDED** — the new function's doc comment binds
   correctly, with a blank line on both sides. **Recorded, not fixed.**
3. ⚠️ **AN UNOWNED GUARD WAS FOUND AND ASSIGNED: the new counter's REGISTRATION was pinned by NOTHING.**
   `manager_test.go`'s H2-stats subtests are a named-**SUBSET** roster, not an exact set, so all 458
   cluster tests stayed GREEN with the counter registered and completely unpinned. **This is exactly the
   `reference_passing_test_is_not_a_guard` shape**, caught by the implementing agent rather than by a
   later gate. Assigned to T13/T14 with both legs named — the must-EXIST leg on the H2 cluster and, more
   importantly, the **must-NOT-EXIST leg on the H1 cluster**, which is what proves the `useH2` gate
   actually gates and makes routing through `incCounter` load-bearing rather than decorative.
4. ⚠️ **BOTH the PLAN's `:528` AND RECON's `:526` were STALE at this agent's base** — T9 had shifted the
   golden by **+34** (`want` sat at `:560` pre-edit). **Every edit was anchored on LITERAL TEXT with a
   `count == 1` assertion, so the drift could not mis-target anything.** This is the third independent
   confirmation this row that line anchors drift and symbol/text anchors do not.
5. **The second `h2ConnFromDialed` path was checked and is NOT a gap.** `dial_h2.go:53` (`DialH2`) passes
   no options, so it books neither `tx_reset` nor `rx_messaging_error` — but it is documented in-tree as
   having had no production callers since phase 43.2a Task 7. `dialPooledH2To` is the one production path
   and it carries the hook. **Pre-existing shape, not a regression introduced here.**

---

## T16 — the in-code 1xx parity target was WRONG, and the correction is SUBSTANTIVE

Commit `56742245`. `internal/filter/hcm/h2/client.go`, **`+17 / -3`**.

The comment on the trailer-validation branch claimed *"the reference FORWARDS 1xx"*.
**Measured: contrib-v1.37.2 SWALLOWS an interim response** — it consumes the 1xx block and delivers
only the FINAL response downstream. ⇒ **the corrected parity target is DROP-AND-DELIVER, not forward.**

⚠️ **The PLAN required the accurate restatement, NOT the 1/1 word swap, and the reason is concrete:
the distinction changes what a future 1xx row would BUY.**
- **FORWARD** needs a downstream write path for the interim block.
- **DROP-AND-DELIVER** needs only that a 1xx leading block leave `respHeadersSeen` **UNSET**, so the real
  final HEADERS still lands in the leading-block branch.
The second is strictly cheaper, and **a row that designed against the old wording would have bought the
wrong one.** Swapping one word would have left that unsaid.

**Scope, re-derived at this tip with BOTH grep forms:** exactly **ONE** `.go` instance existed tree-wide
and it is now **ZERO**. The three surviving instances — `ROADMAP.md`, this phase's `SPEC.md`, and
`PLAN.md` — carry the string **ONLY TO REFUTE IT** and are deliberately **LEFT ALONE**.

⚠️ **THE PLAN'S ROSTER FOR THIS TASK WAS WRONG IN BOTH DIRECTIONS — a T16 finding.**
It named `next-prompt.txt`, which carries the string **ZERO** times at this tip (verified by **DIRECT**
access, because the harness `grep` is blind to that file), and it **OMITTED `PLAN.md` itself**, which
does carry it — the PLAN's roster could not see the file it was being written into.
**Neither error changed what T16 edits.** Recorded because a roster trusted as a closed enumeration is
exactly the shape `reference_verification_table_launders_wrong_cites` warns about.

⚠️ **The harness/GNU grep discrepancy did NOT manifest for this query** — both forms returned the same
four paths. That is **not** evidence the blindness is gone; it is explained by `next-prompt.txt` having
zero hits, so there was nothing for ugrep to hide. **No standing discrepancy figure is carried.**

Gates: `go build` rc=0 · `go vet` clean · `./internal/filter/hcm/...` **RC=0, RUN=533, anchored FAIL=0**
· `gofmt -l` and `golangci-lint` both **EMPTY OUTPUT** (gated on output, never on rc).

---

## T13 + T14 — the ROUTER DISPOSITION pins, the COUNTER pin, and the guard nobody owned

Commit `1846f9d2`. `+23` to `internal/cluster/manager_test.go`, `+310` for the new
`internal/filter/http/router/router_h2_response_headers_test.go`.
**Every NC reddened. No pin failed. No production defect found.**

The input shape rides **T9's seam**: a leading block carrying `content-length` **TWICE** — a shape that
is *inexpressible* without the seam, which is why T9 was a genuine prerequisite rather than a convenience.
⚠️ **`t.Errorf` per property, never `t.Fatalf`** — all three properties are checked in ONE run
(`reference_fatalf_makes_assertions_unreachable`).

### T13 — `TestRouterActionH2_MalformedResponseHeadersDisposition`
- **P1 — 502, not `Status: 0`.** `resp.Status == 502`, `err == nil`, body non-empty.
- **P2 — NO RETRY.** `NewRetryPolicy("connect-failure", 2, nil, 0, 0, 0)` via `H2ClusterAction`;
  **BACKEND-OBSERVED** attempts (`atomic.Int64` per accepted conn, new `startCountingH2TrailerBackend`)
  `== 1`, corroborated by `upstream_rq_retry == 0` / `upstream_rq_total == 1`.
- **P3 — EVICTION, STACKED.** `upstream_cx_http2_total` **DELTA** across a second request:
  malformed ⇒ **1**, legal ⇒ **0**. Baselines asserted REGISTERED first (`counterValue` returns `-1`
  when absent), so a delta computed against a missing counter cannot pass silently.

⚠️ **THE NO-RETRY PIN IS PROVEN NON-VACUOUS, WHICH WAS THE WHOLE RISK.** `c.EnsureRetryStats()` **IS**
called (test `:200`) — without it every retry-counter assertion, *including the `== 0` ones*, would have
been vacuous (RECON F8). And **NC6 REDDENED**: attempts **1 -> 3** — **exactly the cost the PLAN
measured** — with `upstream_rq_retry` **0 -> 2**. That is direct proof the counter is live and the green
`== 0` was a MEASUREMENT, not a no-op.

### T14 — `TestRouterActionH2_MalformedResponseHeadersIncsRxMessagingError`
Subject-side `cluster.c_h2_backend.http2.rx_messaging_error`, **DELTA from a baseline read, never the
absolute** (an absolute passes on a dirty registry and fails on a clean one). The baseline is asserted
`>= 0` **BEFORE** measuring — a registration assertion, because **`incCounter` SWALLOWS a nil**, so an
absent counter would otherwise pass the delta-0 control **for the wrong reason**. Legal-path control
asserts delta **0** and `Status == 200`.

### THE REGISTRATION GUARD — the guard that was pinned by NOTHING
⚠️ Line numbers **re-verified, and BOTH had drifted** from the estimates handed over: the H2 subtest is
`manager_test.go:1692-1746`, the H1 one `:1748-1781`.
- **Leg 1 (must EXIST):** `cl.http2RxMessagingError == nil` -> Error (`:1754`) plus
  `hasMetric("cluster.c_h2s.http2.rx_messaging_error")` (`:1757`).
- **Leg 2 (must NOT exist):** the name appended to the existing `c_h1s` roster (`:1786`) plus
  `cl.http2RxMessagingError != nil` (`:1801`). **The existing roster was NOT restructured.**
⚠️ **Leg 2 is the load-bearing one** — it is what proves the `useH2` gate actually gates, and what makes
routing the increment through `incCounter` load-bearing rather than decorative.

### THE NC CAMPAIGN — ALL REDDENED, EACH ISOLATED, ONE ARM PER RUN
| NC | the break | ACTUAL result |
|---|---|---|
| NC5 | `Underlying: ErrMalformedTrailers` in `client.go` | **RED — status 502 -> 0.** Reddened **on the ARM TAKEN**, not merely on the error value. P3a also flipped (the trailer arm precedes the evict) |
| NC6 | restore `localOrigin: true` on the new router arm | **RED — attempts 1 -> 3**, `upstream_rq_retry` 0 -> 2 |
| NC7 | drop `EvictH2ConnOnError` | **RED — only P3a** (delta 1 -> 0) |
| **NC7b** *(EXTRA, agent-added)* | evict on the SUCCESS path too | **RED — only P3b** (delta 0 -> 1); the malformed arm stayed green |
| NC8 | empty the `h2RxMessagingErrorInc` body | **RED — delta 1 -> 0** |
| NC9 | fire the hook on the accepted path too | **RED — only the legal-path control** (delta 0 -> 1) |
| NC-REG-A | delete the `NewCounter` line | **RED — both must-EXIST legs** (`:1754`, `:1757`) |
| NC-REG-B | hoist registration OUT of `if c.useH2 {` | **RED — both must-NOT-exist legs** (`:1786`, `:1801`); the H2 leg stayed green |

⚠️ **NC7b, NC9 and NC-REG-B each redden ONLY the control while the positive arm stays GREEN. That
asymmetry is the evidence that a stacked control does work no positive arm can do** — precisely
`reference_positive_arm_cannot_catch_overfiring`.

### VERIFICATION, post-restore, at the commit tip
`go build ./...` RC=0 · `go vet` RC=0, both silent · router **RUN 122 -> 129 (+7)**, RC=0, anchored
FAIL=0, **all 7 new `=== RUN` names verified PRESENT** · cluster **RUN 458 -> 458 (+0)** — the guard adds
assertions **inside existing subtests**, so it correctly adds no RUN rows · hcm+router
**RUN 655 -> 662 (+7)**, FAIL=0 · `gofmt -l` and `golangci-lint` **no output**.
**Every NC restore verified `sha256sum -c` ⇒ `OK` (7 checks).** `git status --porcelain` empty.
`git diff --name-only 89a95d91 -- internal/filter/hcm/ test/ docs/` **EMPTY** — the temporary `client.go`
NC patch was fully restored.

---

## T10-T12 + T20 — the unit tables, the wire liveness gate, the sentinel pair, and the fuzz target

Commit `b91a152d`. NEW `internal/filter/hcm/h2/response_headers_validate_test.go` (**+600**) and
`fuzz_test.go` (**+151**).

**Arm counts:** Table A **21** (9 reject · 4 order · 8 parity; **no `empty_block`**, deliberately — a
leading block with no fields is not a legal H/2 response and this validator does not enforce `:status`
presence, so such an arm would document a NON-DECISION as coverage) · Table B **7** · T12 **3** sub-tests
· T20 **11** seeds.
**RUN 655 -> 701 (delta 46 = 21+1 + 7+1 + 3+1 + 11+1).** Run-set diff: **0 removed, 46 added.**
`go build` rc=0 · `go vet` empty · `gofmt -l` empty · `golangci-lint` empty — all gated on OUTPUT.

### THE NC CAMPAIGN — every NC RUN, one arm per run, `-count=1`, all restores `sha256sum -c` ⇒ `OK`
⚠️ **The expected arm SET was stated BEFORE each run**, never a count, because a shared code path
defeats per-arm counts (`reference_shared_codepath_defeats_per_arm_counts`).

| NC | expected (stated first) | ACTUAL |
|---|---|---|
| NC1a delete the connection-specific leg | **6** arms (⚠️ PLAN said 5) | **identical 6** — connection, transfer_encoding, keep_alive, upgrade, proxy_connection, **connection_beats_te** |
| NC1b delete the `te` leg | **3** (⚠️ PLAN said 2) | **identical 3** — te_gzip, te_empty, **te_beats_dup_cl** |
| NC1c delete the uppercase leg | **3**, reddening by **nil** not by a message flip | **identical 3**, all *"= nil, want a rejection"* |
| NC1d delete the duplicate-CL count | **1** | **identical** — duplicate_content_length |
| NC2 uppercase leg -> LAST | ⚠️ **predicted VACUOUS** | **TEST_RC=0, EMPTY reddened set** |
| **NC2b** *(agent-ADDED)* reverse the FIELD loop | connection_beats_te, te_beats_dup_cl | **identical** |
| NC3 unconditional reject | all **8** parity arms | **all 8 RED** |
| NC4 call site neutered | **Table B RED, Table A FULLY GREEN** | **YES** — 6 Table-B reject arms RED, success_capture green, `--- PASS: TestValidateResponseHeaders_Table`, fuzz green |
| NC5 trailer error -> response sentinel | T12 sub-test **3** | **RED** |
| NC6 drop `upgrade` from the shared predicate | fuzz seed #5 | **RED**: `rejected=false, oracle rejected=true` |
| NC7 `te` written `!= "" && != trailers` | **te_empty only** | **exactly** te_empty ×3 tables + seed #9 |
| NC8 re-bar pseudo-headers | status_pseudo_passes | **RED** |
| NC9 peer RST code -> PROTOCOL_ERROR | T12 sub-test 1's CODE assertion | **RED** |

**No pin failed to redden.** ⚠️ **The NC4 asymmetry HELD: Table B red, Table A fully green.** That
asymmetry IS the assertion — it is what proves Table B is a CALL-SITE gate and that the A/B split
survived.

### ⚠️ FINDINGS — THREE MORE PLAN REFUTATIONS, ALL BY EXECUTION
1. ⚠️ **PLAN §7 NC1's leg->arm map is WRONG IN 3 OF ITS 4 ROWS.** Measured **6 / 3 / 3 / 1**; the PLAN
   says **5 / 2 / 1 / 1**. The ORDER arms are missing from its counts. Had the campaign gated on the
   PLAN's numbers it would have read three correct NCs as failures.
2. ⚠️ **PLAN §7 NC2 IS VACUOUS, AND THAT IS MEASURED, NOT ARGUED.** Moving `hasUppercaseHeaderChar` to
   LAST is a **behavioral NO-OP**: every sibling leg compares the name against an **all-lowercase
   literal**, so a name containing an uppercase character matches **none** of them at **any** position.
   ⇒ **`uppercase_beats_connection` and `uppercase_beats_content_length` CANNOT be reddened by any leg
   permutation.** They are LEG-PRESENCE guards, and against leg presence they add nothing beyond the
   `uppercase_name` reject arm. **They are RETAINED as executable documentation of the intended
   precedence and are now ANNOTATED IN THE SOURCE as not being order guards**, so their green is never
   misread as evidence that leg order is pinned.
   **The genuine order guards are `connection_beats_te` and `te_beats_dup_cl`, reddened by reversing the
   FIELD loop — NC2b, which the agent added because the PLAN's NC could not do the job.**
   ⚠️ The PLAN's claim that the two arms *"FLIP to a different message"* is also refuted: **they return
   nil.**
3. ⚠️ **PLAN §9's fuzz assertion 2 is UNDER-SPECIFIED, and the target caught the agent's own wrong
   implementation.** *"Quoted, in trailing position"* holds for **`*Error.Msg`**, NOT for `Error()`,
   which appends `": " + Underlying.Error()`. **9 of 11 seeds were RED until the assertion was
   re-anchored** — the fuzz target falsified its own author before it ever saw a mutation.
4. ⚠️ **NC3 HANGS the h2 package past 10 minutes** — pre-existing tests block on a `RoundTrip` that can
   no longer succeed once the validator rejects everything. **It must be run scoped, with `-timeout`.**
   Recorded so the next NC campaign does not read a hang as a hang-free pass.
5. **Process note the agent recorded against ITSELF:** its first commit message asserted the NC4/NC5
   outcomes **before running them** (they later matched). The commit was **AMENDED** with the measured
   campaign. Recorded because a prediction that happens to come true is still not a measurement.

### T20 — `FuzzValidateResponseHeaderBlock`
Landed in the **EXISTING** `fuzz_test.go` ⇒ **targets 55 -> 56, fuzz FILE count UNCHANGED at 48.**
(The PLAN required this be stated and measured either way; it was.)
30 s run: RC=0, **3,706,794 execs, 0 crashes**; seed run **11/11 PASS**.
⚠️ **THE ORACLE IS INDEPENDENTLY WRITTEN AND SHARES NO PREDICATE** — it spells the closed name set, the
byte-wise uppercase test and the `"trailers"` literal **LONGHAND**, calling neither
`isConnectionSpecificField`, nor `hasUppercaseHeaderChar`, nor `teTrailersValue`.
**NC6 PROVES the independence**: dropping `upgrade` from the shared predicate reddened seed #5
(`rejected=false, oracle rejected=true`). **A shared-predicate oracle would have stayed green** — it
would only have proven that a function equals itself.

---

## T15 — TRAILER INVARIANCE, as a RUN-SET DIFF (not a green run, not a count)

⚠️ **This is a NON-REGRESSION check and its gate is the `diff` of SORTED `=== RUN` NAME LISTS, which
must be EMPTY on the removed side.** A green run does not prove the set did not change, and **a renamed
or dropped test is invisible to a count.**

### Denominator 1 — the full two-package set
`go test -v -count=1 ./internal/filter/hcm/... ./internal/filter/http/router/...`
RC=**0** · raw RUN **708** · anchored FAIL **0** · `sort -u` **708** (equals raw ⇒ no duplicate names).

| | |
|---|---|
| **names REMOVED vs the T1a roster** | **0** — **THE GATE PASSES** |
| names ADDED | **53** |

655 + 53 = 708, and 53 = 46 (T10-T12/T20) + 7 (T13/T14), reconciling both agents' independently
reported deltas. Every added name belongs to this row:

```
22  TestValidateResponseHeaders_Table
12  FuzzValidateResponseHeaderBlock
 8  TestClientConn_RoundTrip_ResponseHeaderValidation_Wire
 4  TestRouterActionH2_MalformedResponseHeadersDisposition
 4  TestMalformedResponseHeaders_SentinelDiscriminatesTrailerReject
 3  TestRouterActionH2_MalformedResponseHeadersIncsRxMessagingError
```

### ⚠️ Denominator 2 — `-run 'Trailer'`, AND A TRAP THE CONTROLLER WALKED INTO AND CAUGHT
**FIRST ATTEMPT READ 10 NAMES "REMOVED" — AND THAT WAS THE INSTRUMENT, NOT A REGRESSION.**
The BEFORE set had been built with `grep -i 'trailer'` over the T1a roster, which is **NOT the same
selector as `go test -run 'Trailer'`**:
- `grep -i` matches **any substring anywhere**, including a lowercase `trailers` in a **SUBTEST** name;
- `go test -run 'Trailer'` matches **per path element** and is **CASE-SENSITIVE**, so a subtest only runs
  if its **TOP-LEVEL** name matches.

All 10 "removed" names were subtests such as `TestWriteH2Reply_FrameSequence/body_with_trailers` and
`TestBuildRequest_ConnectionSpecificFields/te_trailers_passes`, whose parents contain no `Trailer`.
⇒ **A grep-derived BEFORE set is not comparable to a `go test -run` AFTER set. Match the selector
SEMANTICS on both sides or the diff is meaningless.**

**Re-derived with matched semantics** (`awk -F'/' '$1 ~ /Trailer/'`, i.e. first path element,
case-sensitive):

| | |
|---|---|
| before | **67** |
| after | **71** |
| **REMOVED** | **0** — **THE GATE PASSES** |
| ADDED | **4**, all of them this row's `TestMalformedResponseHeaders_SentinelDiscriminatesTrailerReject` and its three sub-tests |

⚠️ **The Trailer-selector set GREW, and that is CORRECT, not a violation** — this row's sentinel test
carries `Trailer` in its own name by design (it discriminates *against* the trailer reject). A gate
demanding the `-run 'Trailer'` count be INVARIANT would have RED on a correct change.

### Denominator 3 — the five NAMED h2 trailer tests: ALL INVARIANT
| test | before | after | |
|---|---|---|---|
| `TestValidateResponseTrailers_Table` | 21 | 21 | INVARIANT |
| `TestClientConn_RoundTrip_TrailerValidation_Wire` | 8 | 8 | INVARIANT |
| `TestMalformedTrailers_SentinelDiscriminatesPeerReset` | 3 | 3 | INVARIANT |
| `TestActionResponseLiterals_DoH2ClusterActionNonSuccessSites` | 1 | 1 | INVARIANT |
| `TestActionResponseLiterals_OnlySuccessSitePopulatesTrailers` | 1 | 1 | INVARIANT |

Re-run with the selector asserted to have matched: hcm RC=0 **RUN=32**, router RC=0 **RUN=4**, and
**`no tests to run` reads 0 in BOTH** — a `-run` matching nothing prints that and EXITS 0.

⚠️ **A FIFTH-NAME CORRECTION:** the SPEC's roster names `TestMalformedTrailers_SentinelDiscriminates`,
which **does not exist**. The real symbol is **`TestMalformedTrailers_SentinelDiscriminatesPeerReset`**
(`trailers_validate_test.go:591`). ⚠️ **Queried under the SPEC's name the denominator reads `0` before
AND after — i.e. it would have reported "INVARIANT" while measuring NOTHING.** A `0 == 0` invariance is
the vacuous shape this row has now hit twice.

### The AST census, re-derived independently by the controller
```
router_h2_trailers_test.go:508: audited 18 ActionResponse composite literals across the router package
```
**18**, up from **17** — moved exactly as predicted, via `t.Logf`, and it correctly did **NOT** redden.
Its silence is therefore **not** evidence of invariance, which is why it is quoted here rather than relied on.

---

## T17-T19 — THE NEGATIVE-CONTROL CAMPAIGN, COMPLETE. **NC1-NC12, and not one failed to redden.**

The campaign was executed by the agents owning each pin, because an NC on a pin you did not write is an
NC on your reading of it. Discipline held throughout: **COMMIT FIRST** (`git checkout --` restores from
HEAD and wipes uncommitted work), `sha256sum` before, **ONE ARM PER RUN** (a fail-fast driver masks later
RED arms and the run's failure then reads as proof for all of them), `-count=1` on every run (the cache
serves a stale PASS), and `sha256sum -c` ⇒ **`OK`** after every restore.

**Restore verifications: 7 (T13/T14) + 11 (T21 resolution) + the unit campaign's full set — every one `OK`.**

- **T17 (NC1-NC4)** — the validator, its leg order, the parity arms and the CALL SITE. ⚠️ **The NC4
  asymmetry HELD: Table B RED, Table A FULLY GREEN.** That asymmetry *is* the assertion; had Table A also
  reddened, the A/B split would have been lost and Table B would not be a call-site gate.
- **T18 (NC5-NC9)** — the sentinel, the router arm, the retry classification, the eviction and the
  counter, plus an agent-added **NC7b**. ⚠️ **NC7b, NC9 and NC-REG-B each redden ONLY their CONTROL while
  the positive arm stays GREEN** — the thing a positive arm structurally cannot demonstrate.
- **T19 (NC10, NC12)** — below.
- **NC11 is T15**, and is a RUN-SET DIFF rather than a red run.

---

## T19 / NC12 — the `panic()` REACHABILITY CONTROL, **with its discriminating half**

⚠️ **MANDATORY: a green run is not evidence a site is live — it may be dead code, and grep cannot
separate assertion blindness from dead code.**

**Instrument: the in-package router tests, not the differential** — and the choice is reasoned, not
convenient. `TestRouterActionH2_MalformedResponseHeadersIncsRxMessagingError` already ships both halves
as sibling sub-tests calling `doH2ClusterAction` directly with **identical setup differing in exactly one
variable** (malformed vs well-formed leading block). The differential's subject is a separate subprocess
— a strictly worse instrument for a reachability question. Each half ran in its OWN run, because a panic
aborts the binary.

- **Malformed ⇒ PANICS.** rc=1, panic gate=1, `no tests to run`=0.
  `panic: PROBE-P92-ROUTER-ARM [recovered, repanicked]`, stack frame **`router_h2.go:229` inside
  `doH2ClusterAction`**, entered from the test. **The arm is live code.**
- **Legal ⇒ does NOT panic**, with the panic **verified still in place** (`grep -c` = 1). rc=0, panic
  gate=0, probe string absent, PASS — and the sub-test itself asserts `Status == 200`, so the success
  path genuinely ran. ⚠️ **This half is what excludes a fire-on-every-request panic**, which would
  otherwise have read as proof the arm is reached.

---

## T21 — DIFFERENTIAL GREEN, after the row's hardest finding

### ⚠️ THE FIRST T21 RUN WAS **RED**, AND NOT BECAUSE THE FIX FAILED
`status` and `illegal` converged on all five arms — **the row's charter held**. What diverged was the
transcript's `content-length` field ARITY: **reference 1, subject 0**, on all five arms, reproduced
identically on a second pristine run (**deterministic, not a flake**), with lines 1-12 of the transcript
byte-identical (no collateral regression).

⚠️ **THE MECHANISM IS A COMPENSATING-DEFECT UNMASKING** — `reference_compensating_defects_cancel_in_the_gate_metric`,
fired for real rather than cited. **Pre-fix:** subject forwarded upstream's 200 carrying
`content-length: 6` (arity **1**); reference answered 502 with `content-length: 87` (arity **1**).
**1 vs 1 — the two defects CANCELLED IN THE GATE METRIC.** The fix replaces the forwarded 200 with a
locally generated 502, and `h2LocalReplyHeaders()` emits **no Content-Length at all** ⇒ **0 vs 1**.
**The metric became a discriminator only once this row's defect was fixed.**

**Root cause, verified independently by the controller:** `h2LocalReplyHeaders()`
(`internal/filter/http/router/router_h2.go:294`) returns only Content-Type / Date / Server. It dates from
**phase 07.1 (`3dd7e129`)** and has **SEVEN call sites** (`retry.go:374` 504; `router_h2.go:80/:128/:138`
503; `:148`, `:231`, `:250` 502). ⚠️ **The H1 sibling `localReplyHeaders(bodyLen int)` DOES emit one, and
even takes a `bodyLen` the H2 version lacks.**

### THE RESOLUTION — neither of the two easy answers
- ⚠️ **NOT fixed.** Seven local-reply sites across the 502/503/504 H2 paths is unchartered and unpriced
  and needs its own behavior-contract treatment. **BANKED as a candidate with full evidence.**
- ⚠️ **NOT deleted.** Removing a failing assertion to reach green is the failure mode this project exists
  to prevent.
- **Instead:** the cross-side transcript carries **the row's contract only** (`status` + the illegal-name
  SET), applying **the fixture's OWN already-documented rule** that local-reply composition is an
  excluded cross-side axis — its 404 bodies are excluded for exactly this reason — and PLAN §8.2's own
  *"the content-length assertion, **IF ANY**"* wording. **And the departure is PINNED PER SIDE at its
  MEASURED values**, so it is documented and guarded rather than dropped.

### ⚠️ AND THE PIN'S FIRST PLACEMENT — WHICH THE CONTROLLER SPECIFIED — WAS ITSELF A DEFECT, CAUGHT BY MEASUREMENT
The controller's instruction put the per-side pin in `DriveReference` / `DriveSubject`. The agent built
it, committed it, **then measured it under NC10 — and it MASKED THE ROW'S OWN CHARTER GATE.** A Drive
error is `t.Fatalf`'d at runner step 5/6, **BEFORE `CompareBytes` at step 7**. With the production guard
reverted, the run reported **only** the arity pin; **the `status`/`illegal` divergence was never
reached.** This is `reference_liveness_barrier_upstream_of_gate`: an out-of-charter pin placed upstream
of the charter gate hides the regression it was meant to sit beside.
**Fixed by moving the pin into `AssertDistribution` (runner step 8, `t.Errorf`, BELOW the byte compare),
with `errors.Join` so the two RR rules and the two arity pins cannot mask each other.** Every NC10 run
now reports **BOTH** `runner_test.go:1289: differential mismatch` **AND** the arity pin in the same run.
Documented at the function and in the README with a do-not-move-it-back note.
⚠️ **The agent found this by RUNNING ITS OWN NEW PIN AGAINST NC10 rather than trusting the brief.**

### T21 FINAL — GREEN
rc **0** · `=== RUN   TestDifferential/0004-h2-routing` **PRESENT (=1)** · anchored FAIL **0** · panic
gate **0** · **SKIP 0** (the fixture RAN — the runner `t.Skipf`s an unregistered fixture at
`runner_test.go:200` and there is NO fixture-count gate in the tree, so this was asserted, not assumed).
Full transcripts `cmp` **IDENTICAL**, 420 B each (down from 475). The five converged lines, VERBATIM:
```
p92-keepalive:status=502 illegal=<none>
p92-upgrade:status=502 illegal=<none>
p92-proxyconn:status=502 illegal=<none>
p92-te-gzip:status=502 illegal=<none>
p92-te-empty:status=502 illegal=<none>
```

### The per-side pin, and BOTH its NC flips
`p92WantRefCLFields = 1` / `p92WantSubjCLFields = 0`, checked by `p92AssertCLFields(side, want, got []int)`.
**Arity only, never value** (the values legitimately differ — 87 against `bad502Body`). It asserts
`len(got) == len(p92Arms())` **first**, so an empty slice cannot pass vacuously, and it names **EVERY**
mismatching arm rather than the first.
- subject 0 -> 1: **rc=1**, `want 1 on every arm, got keepalive=0,upgrade=0,proxyconn=0,te-gzip=0,te-empty=0 (5 of 5 arms)`
- reference 1 -> 0: **rc=1**, `want 0 on every arm, got keepalive=1,upgrade=1,proxyconn=1,te-gzip=1,te-empty=1 (5 of 5 arms)`

### NC10 re-confirmed under the NEW transcript shape
⚠️ **`CompareBytes` (`diff.go:19`) is FIRST-DIVERGENCE and MASKS arms 2-5, so a single red run proves
only the FIRST arm.** Independence was established by reducing `p92Arms()` to a **SINGLE-ARM roster and
running once per shape — five runs**, the roster printed live from the file before each, with
`git checkout --` + `sha256sum -c` ⇒ `OK` after each. All five: rc=1, RUN line=1, anchored FAIL=5, panic
gate=0.

| arm | offset | reference | subject |
|---|---|---|---|
| keepalive | 246 | `502 illegal=<non` | `200 illegal=keep` |
| upgrade | 244 | `502 illegal=<non` | `200 illegal=upgr` |
| proxyconn | 246 | `502 illegal=<non` | `200 illegal=prox` |
| te-gzip | 244 | `502 illegal=<non` | `200 illegal=te.` |
| te-empty | 245 | `502 illegal=<non` | `200 illegal=te.` |

**The offsets REPRODUCE the pre-resolution record EXACTLY** (the divergence sits upstream of where
` clfields=` used to be). The `te-*` subject windows are now 31 B rather than 32 — end-of-transcript
truncation from the shorter line, **not** a behavior change.

⚠️ **A trap that had to be handled: because T21 was ALREADY red before the resolution, "NC10 went red"
proved nothing** — it reddened either way. The discriminator was divergence **CONTENT and OFFSET**:
guard present ⇒ offset 274 on `clfields`; guard removed ⇒ offsets 244-246 on `status`/`illegal`. A
counterfactual bonus confirmed the root cause independently: with the guard removed the subject's arity
returns to **1**, matching the reference.

---

## T22 — THE SIX-GATE POSTURE. **DEPARTURES NAMED; COMPLIANCE NOT CLAIMED.**

| # | gate | ACTUAL | verdict |
|---|---|---|---|
| **a** | `go test ./test/differential/ -count=1 -v` | EXIT=0 · RUN=**121** · PASS=**121** · SKIP=**0** · anchored FAIL=0 · panic gate=0 | **PASS** |
| **b** | `go test ./... -count=1` | ⚠️ **RED on first measurement** (see FINDING 1) — **after the fix: EXIT=0, anchored FAIL=0, panic gate=0** | **PASS (after fix)** |
| **c1** | h2spec, **cited ONLY from this row's own run** | **`95 tests, 94 passed, 1 skipped, 0 failed`**, `--- PASS (2.90s)`, re-confirmed identically on a second run | **PASS** |
| **c2** | grpc-conformance | not attempted | **DEFERRED — standing, in writing** |
| **c3** | proxy-wasm | `--- PASS`, **10 top-level families, all PASS** | **PASS (10 MEASURED)** |
| **d** | fuzzers | targets **55 -> 56**; **FILES UNCHANGED at 48** (the target landed in the EXISTING `fuzz_test.go`, taking it 2 -> 3 targets); 30 s run rc=0, **2,187,222 execs, 43 new interesting, 0 crashes** | **PASS** |
| **e** | anchored panic gate `^panic:\|DATA RACE\|SIGSEGV` on every launch | diffall **0** · full run 1 **1** (FINDING 2) · full run 2 **0** · isolated 0084 **0** · race **0** · final full **0** | **1 HIT, investigated** |
| **f** | `REVIEW.md` | absent | **DEPARTURE — named, not a pass** |

**Sweeps:** `gofmt -l internal/` and `test/` **empty** · `golangci-lint run ./...` **empty** · `go vet ./...` rc=0 **empty** · `go mod tidy -diff` rc=0 · `git diff --numstat <base> HEAD -- go.mod go.sum` **EMPTY ⇒ ZERO modules added** · **`-race`** on the three in-process packages rc=0, RUN **844**, FAIL 0, DATA RACE 0.
⚠️ **`-race` ON THE DIFFERENTIAL SUITE IS VACUOUS** — the differential subject is an **UNRACED SUBPROCESS**, so only the in-process packages were raced. Stated so the green is not over-read.
⚠️ **h2spec is MEASURED BLIND to burst-drain ordering.**
⚠️ **The proxy-wasm `/16` denominator was NOT reproducible** — the run does not emit it and no `16`-family constant exists in `test/conformance/proxy-wasm/*.go`. **This row therefore asserts "10 families measured green" and NOT "10/16".**

### RECONCILIATIONS — both EMPTY IN BOTH DIRECTIONS
- **Fixtures:** **121** directories vs **121** `=== RUN TestDifferential/` vs **121** PASS vs **0** SKIP, no duplicates; set difference by NAME empty both ways. **The `runner_test.go:200` skip fired ZERO times**, and `0004-h2-routing` is present and PASS. ⚠️ This matters because **there is no fixture-count gate anywhere in the tree**, so gate (a)'s failure mode is a SILENT PASS.
- **Packages:** `go list ./...` = **236**; final run **127 ok + 109 no-test-files + 0 FAIL = 236**. **No package silently failed to run.**

### ⚠️ FINDING 1 — GATE (b) WAS DETERMINISTICALLY RED, CAUSED BY THIS ROW'S OWN T21, AND THE SHALLOW SYMPTOM HID A WORSE DEFECT
`TestH2Driver_AssertDistribution/both_[3,3,3]` failed with
`p92 ref content-length arity: got 0 observations, want 5 (one per arm)`, reproduced **3×**.

T21 added `p92AssertCLFields` to `AssertDistribution`, whose FIRST check is the deliberate non-vacuity
guard `len(got) != len(arms)`. But `driver_test.go` builds a bare `&h2Driver{}` seeding only
`refBodyCnt`/`subjBodyCnt`, so the p92 slices were `len == 0` against 5 arms and **the guard fired on
every row**. `git show` confirms T21 touched `driver.go` + `README.md` and **never updated the test**.

⚠️ **THE RED ROW WAS THE LESSER HALF.** The five `wantErr: true` rows kept **PASSING** — but the arity
guard alone made `err != nil`, so **they would have passed even with the distribution rule entirely
removed.** A bare `err != nil` check cannot distinguish "failed for its own reason" from "failed for
someone else's". **Five of six rows were vacuous with respect to their stated purpose while looking
green** — `reference_passing_test_is_not_a_guard`, arising inside this row's own change.

**FIX, both halves:**
1. seed `d.refP92CL` / `d.subjP92CL` per row via `p92CLObs(v)`, **derived from `len(p92Arms())` rather
   than a literal 5**, so **adding an arm cannot silently make these rows vacuous again — which is
   precisely how the defect arose**;
2. add **`wantErrSubstr`** and assert **WHICH** property failed on every negative row;
3. add **FOUR NEW ROWS** covering the arity pins themselves in both directions (missing observations per
   side, wrong value per side), which nothing covered before. **RUN 6 -> 10.**

⚠️ **THE FIX WAS NEGATIVE-CONTROLLED, AND THE FIRST NC ATTEMPT WAS DISCARDED AS WORTHLESS.** Deleting the
distribution legs left `want` unused ⇒ a **BUILD BREAK, which proves nothing**
(`reference_config_counterfactual_is_not_implementation_counterfactual`). Re-run by NEUTRALISING the legs
(`if false && …`) so the package still compiles:
**exactly the THREE rows whose stated purpose is the distribution rule reddened** (`subj [4,3,2]`,
`ref [4,3,2]`, `both [9,0,0]`), while the two `count length mismatch` rows — which guard a DIFFERENT,
untouched early branch — correctly stayed GREEN, as did all four p92 rows.
**Before the fix those three would have PASSED under this same break.** Restore verified `sha256sum -c` ⇒ `OK`.

### FINDING 2 — the `0084-otlp` bind panic: NON-REPRODUCING, and **OUT-OF-BAND**
Run 1 only: `panic: driver: start OTLP receiver on 0.0.0.0:38801: bind: address already in use`.
⚠️ **It ABORTED THE TEST BINARY** — run 1 shows only **86** of 121 subtests, and every fixture after
`0084` never ran. **That is invisible to a bare `FAIL`/`ok` read**, which is why the fixture-set
reconciliation above is not ceremony.

**The band question was answered from SOURCE, not assumed:** `harness_test.go:293` reserves
**20000..31007**; `/proc/sys/net/ipv4/ip_local_port_range` = **32768 60999**. ⇒ **38801 is EPHEMERAL and
OUTSIDE the reserved band**, so this is an **out-of-band occurrence of the known driver-owned receiver
race, NOT an in-band recurrence** — an in-band recurrence would have been a FINDING rather than a flake.
Both runs are reported: run 1 PANIC/FAIL; run 2 PASS (121/121, panic 0); isolated `-run` PASS with the
RUN line asserted; standalone differential PASS. **Not reproducible ⇒ flake.**

⚠️ **A NAMEABLE CONTRIBUTING DEFECT, REPORTED AND DELIBERATELY NOT FIXED:** `allocateOTLPPort` probes
**`127.0.0.1:0` (loopback)** while `ensureServer` binds **`0.0.0.0:<port>` (wildcard)** — different
scopes, and **the exact trap `freeTCPPortBlock`'s own comment says it avoids** — plus a wide
allocate→bind gap and a hard `panic` instead of a retry. Out of this row's charter; **banked**.

### ⚠️ A STANDING METHOD NOTE IS CORRECTED BY MEASUREMENT
The two output-gated sweeps were **negative-controlled** with a deliberately broken untracked package
(deleted afterwards): `gofmt -l` named it, and `golangci-lint` reported 3 issues **and exited rc=1**.
⇒ **CORRECTION: this `golangci-lint` build DOES exit non-zero on findings.** The standing note says it
never does. **Gating on OUTPUT remains correct and strictly safer**, but the stated reason is wrong for
this build, and a future row must not rely on rc==0 meaning "no findings" in either direction without
measuring it.
