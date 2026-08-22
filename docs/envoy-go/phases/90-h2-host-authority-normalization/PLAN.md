# PLAN 90 — `h2-host-authority-normalization`

> **For agentic workers:** REQUIRED SUB-SKILL — execute this with `superpowers:subagent-driven-development`
> (preferred) or `superpowers:executing-plans`, TDD per `superpowers:test-driven-development` on every
> task. Steps use checkbox (`- [ ]`) syntax for tracking. **ONE atomic IMPL commit** (the phase-88/89
> precedent): TDD is held *inside* it, both censuses observed first, no sub-phase rows minted.

**Stage:** lifecycle-state **2 -> 3**. Base master `c7fef29b`, branch `phase-90-plan`.
**Spec:** `docs/envoy-go/phases/90-h2-host-authority-normalization/SPEC.md` — the PLAN argues from it and
travels with it.
**Execution style:** subagent-driven per `feedback_execution_style` — three measurement agents on private
scratch, **each committing nothing and each proving its tree clean**; the controller re-derived every
load-bearing claim BY EXECUTION on its own instruments **and refuted one of the agents**.

**Docs-only stage.** ZERO production `.go`, ZERO test `.go`. `ROADMAP.md`, `BEHAVIOR_CONTRACT.md` and
`DECISIONS.md` are **BYTE-UNTOUCHED**; row 90 stays `in-progress`, `want` stays **122**, next-free stays
**`ADR-0313`**, and the strict `^> **STATUS: PROPOSED` guard **STAYS ARMED at 1** — the IMPL disarms it.

---

## 0. THE HEADLINE: THIS PLAN REFUTES ITS SPEC ON THE FIX'S SHAPE, ITS GUARD ARGUMENT, ITS COST AND ITS FIXTURE CONSTRAINT

Every stage's job is to refute its predecessor by execution. The SPEC refuted six BRAINSTORM claims. This
PLAN refutes **five** SPEC claims and **one of its own measurement agents**:

| # | claim | source | verdict |
|---|---|---|---|
| 1 | *"`buildRequest` … does `regular.Add` for `host`"* | ADR-0312 §Context ¶1 | ⚠️ **MISLEADING** — there is **no `host` branch and no `host` identifier** in `stream.go`; the IMPL **adds** a branch, it does not modify one — §2 |
| 2 | *"`buildH2Request` has demonstrated sensitivity; `buildRequest` has demonstrated zero"* | SPEC §9.3 | ⚠️ **REFUTED** — the asymmetry is by **PROPERTY, not SYMBOL**: dropping `host` from the carrier inside `buildH2Request` is **equally vacuous** — §5 |
| 3 | cost **+34/−0**, symbols `authoritySeen` 6 / `hostField` 7 | SPEC §9 | ⚠️ **REFUTED** — **+36/−0**, `hostField` **6**, and a **THIRD** required symbol `hostSeen` (**8**) the SPEC never names — §8 |
| 4 | *"raw-framer arms … must not hit `/api`"* | SPEC §8.2 | ⚠️ **REFUTED** — the constraint is the `/api/v1/<n>` **counting loop**, not the `/api` prefix; relaxing it makes **both YAMLs byte-untouched** — §3 |
| 5 | *"the BRAINSTORM's '69 packages ok' is a measurement artifact; the flake did not recur"* | SPEC §9.1 | ⚠️ **PARTIALLY REFUTED** — 69 came with `rc=1` and a **real** `--- FAIL`; and a **second** flake fired on **3 of 8** full-suite runs this stage — §9.2 |
| 6 | *"net/http lifts the authority out of `r.Header`, so the reflected block can never contain it"* | a **measurement agent**, not the SPEC | ⚠️ **REFUTED BY THE CONTROLLER** — SPEC §8.4's table is correct — §4 |

⚠️ **Refutations 2 and 3 both land on the SPEC's own §9, the block the row's entire guard argument rests
on.** The conclusion — *the row must bring its own guard* — survives; the **argument for it changes**, and
with it the shape of the unit roster.

---

**Stage:** lifecycle-state **2 -> 3**. Base master `c7fef29b`, branch `phase-90-plan`.
**Execution style:** subagent-driven per `feedback_execution_style` — three measurement agents on
disjoint scratch, each committing nothing and each proving its tree clean; the controller re-derived
every load-bearing claim BY EXECUTION on its own instruments.

**Docs-only stage.** ZERO production `.go`, ZERO test `.go`. `ROADMAP.md`, `BEHAVIOR_CONTRACT.md` and
`DECISIONS.md` are **BYTE-UNTOUCHED**; row 90 stays `in-progress`, `want` stays **122**, next-free stays
**`ADR-0313`**, and the strict `^> **STATUS: PROPOSED` guard **STAYS ARMED at 1** (the IMPL disarms it).

---

## 1. SENTINEL — RUN MECHANICALLY AT THIS TIP, ACTUAL OUTPUT RECORDED

`ROADMAP.md` is BYTE-UNTOUCHED this stage, so there is **ONE side, not two**.

| check | ACTUAL output at `c7fef29b` |
|---|---|
| (1) every row `done`, `want=122` | **`NOT DONE: row 90`** — the HEALTHY reading while the row is open; denominator asserted, no `GATE FAIL` |
| (2) deferred-candidate windows | **SIX** — `:200 :206 :212 :222 :228 :236` |
| (3) every WORK family opened | **SILENT** |

⇒ **TWO checks block the sentinel. `stop` was NOT created** — verified absent at the git root and in the
stage worktree.

**ALL FOUR NCs FIRED**, each inspected before being trusted:

- **NC-A** — `NC LANDED? [ in-progress ]` inspected FIRST, then check (1) on the doctored file ⇒
  **`NOT DONE: row 62` AND `NOT DONE: row 90`**. ⚠️ **BOTH lines are required while row 90 is open.** A
  reading of `row 62` alone would mean the row was LOST, not that the check is healthy.
- **NC-B** — `want=121` on the real file ⇒ **`GATE FAIL: examined 122 data rows, expected 121`**.
- **NC-C** — residual confirmed **2 -> 0** BEFORE the assertion ⇒ **`NEVER OPENED: gRPC`**, with WASM
  correctly silent (the NC is scoped to one slug).
- **NC-D** — the two check-(2) alternates run separately: long **5**, short **1**, union **6**. The short
  form is the Operational-tooling family the pre-phase-77 matcher was blind to.

**Leak axes at this tip:** `-family row` **95 occurrences / 67 LINES** (⚠️ `grep -c` counts LINES; both
figures are stated because a gate reading zero on both sides is not evidence of invariance, and the
leading `-` needs `--` or GNU grep reads it as a flag and prints `base=0 now=0 delta=0`) ·
`gRPC-family row` **2** · `Operational-tooling-family row` **3**.

**ARM-A malformed-row guard, ESCAPE-AWARE:** ids **57** (`NF=9`) and **69** (`NF=10`), at lines **119**
and **131**. ⚠️ The naive `awk -F'|'` form reads **17** at this tip — reproduced, and **NOT** a drift
signal. The recorded unit is awk `NF` (**8** for a well-formed row), not column count.

### 1.1 ⚠️ THE PROVENANCE GREP — the SPEC's discriminating form re-run, and it holds

The SPEC's §11.1 refutation of the BRAINSTORM is re-run at my tip with the DISCRIMINATING form
(`:authority` + backticked `` `host` ``/`` `Host` `` + `h2-host`):

```
window :200 -> 0    :206 -> 0    :212 -> 0    :222 -> 0    :228 -> 0    :236 -> 0
positive control on row 90's own line (:152) -> 22
fabricated-token NC across all six           -> 0 0 0 0 0 0
```

⇒ **The provenance genuinely is outside every sentinel window**, and the conclusion now rests on a
measurement that discriminates rather than on the loose form that reads **1** on window `:222`.

### 1.2 ⚠️ TWO WINDOW CANDIDATES ARE STALE — re-confirmed by execution, NOT inherited

Window `:222` still lists the H2 `//`-path routing bug and the `/stats/prometheus` projection gap.
Re-verified at my tip: `internal/filter/hcm/h2/stream.go:498` is `u, err := url.ParseRequestURI(path)`
(row 87's fix, landed), and rows **79**, **80**, **87** all read `done`. **A candidate's presence in a
window is not evidence it is still open**, and this PLAN does not narrow the window — that is a doc row,
not this charter.

---

## 2. ⚠️ REFUTATION 1 — THERE IS NO `host` BRANCH TO MODIFY, AND THAT MINTS A DECISION THE SPEC DID NOT ANTICIPATE

**ADR-0312 §Context ¶1 (drafted at the SPEC) reads:** *"`buildRequest` independently does `regular.Add`
for `host` and sets both `u.Host` and `http.Request.Host` from the same possibly-empty authority."*

⚠️ **THERE IS NO `host` BRANCH IN `stream.go`. THERE IS NO `host` IDENTIFIER IN `stream.go` AT ALL.**
Re-run by the controller on its own instrument, and independently by a measurement agent:

```
$ grep -in 'host' internal/filter/hcm/h2/stream.go
503:	u.Host = authority
507:		Host:       authority,
```

**Two occurrences in 516 lines, and BOTH are the WRITE of `authority` onto the `*http.Request`.** A
regular `host` field reaches `regular.Add(name, h.Value)` at **:472** — the single **generic**
non-pseudo path taken by *every* regular header — with **zero** special-casing. (`http.Header.Add` then
canonicalizes the map key to `Host`, which is why the RED census reads `req.Header Host = "h.example"`.)

The ADR sentence is *literally* true — `regular.Add` is indeed called, for `host` among everything else —
but it reads as though a `host`-specific site exists to be modified. **It does not.** The consequence is
concrete and changes the task decomposition:

⇒ **THE IMPL ADDS A BRANCH THAT DOES NOT EXIST, IT DOES NOT MODIFY ONE.** Every break arm targeting
`buildRequest` must name an injection site in **new** code, and the cost of the `buildRequest` half is
addition-only — which is exactly why the measured prototype is **`+34 / −0`** with a **zero** deletion
count. The `−0` was evidence of this all along and was not read that way.

### 2.1 ⚠️ **D-90-DUP: `:authority` HAS NO DUPLICATE GUARD, AND THIS ROW DOES NOT ADD ONE.** (NEW — the SPEC did not reach this)

Found while re-verifying the above. `buildRequest`'s pseudo-header switch guards **three** of its four
pseudo-headers against duplication and **not** the fourth:

```go
case ":method":  if methodSeen { return … "duplicate :method" }   // :426
case ":path":    if pathSeen   { return … "duplicate :path"   }   // :432
case ":scheme":  if schemeSeen { return … "duplicate :scheme" }   // :441
case ":authority":
	authority = h.Value                                            // :446-447 — NO GUARD, silent LAST-WINS
```

This matters because the frozen rule needs to know whether `:authority` was **PRESENT**, and the natural
implementation of that is a `…Seen` boolean — landing the new variable in a switch where every sibling
`…Seen` boolean *also* gates a `PROTOCOL_ERROR`. **An implementer following the local idiom would add the
reject, and that reject is an unpriced behaviour change outside this charter.**

**DECISION: `authoritySeen` tracks PRESENCE ONLY. Duplicate `:authority` KEEPS its current silent
last-wins behaviour.** Three grounds:

1. **Not in charter.** The charter is *"the upstream request carries exactly ONE authority"*. A duplicate
   `:authority` already yields exactly one — the rule is well-defined, merely unvalidated.
2. **The reference's behaviour there was NEVER MEASURED.** The SPEC's nine-arm partition (§5.1) varies
   authority *validity*; **no arm sent `:authority` twice.** Adding a reject would be parity with an
   unmeasured side — the `reference_config_counterfactual_is_not_implementation_counterfactual` species.
3. **It belongs to the deferred arm C**, which already owns the reject shape decision, the
   `isValidAuthority` predicate, and the stream-vs-connection scope question.

⚠️ **AND IT GETS A GUARD, because a silent regression here is exactly what an implementer would ship.**
The unit roster carries `D_dup_authority` (§5), which sends `:authority` twice and asserts **200-path,
last value wins, no error** — i.e. it pins the behaviour this row deliberately does **not** change.
Without it, "we didn't change duplicate handling" is a claim, not a measurement.

---

## 3. ⚠️ THE SECOND REFUTATION: THE FIXTURE CONSTRAINT THE SPEC HANDED DOWN IS OVER-BROAD, AND RELAXING IT REMOVES THE YAML EDIT ENTIRELY

**SPEC §8.2 and the stage brief both state:** *"raw-framer arms must open their own connections **and must not
hit `/api`**"*, because `AssertDistribution` demands exactly `[3,3,3]`.

⚠️ **The `/api` half is FALSE.** Executed at my tip:

```
$ grep -n 'counts\[' test/fixtures/0004-h2-routing/driver/driver.go
275:// Side-effect: per-call /api response-body parsing populates counts[idx]
308:		counts[idx]++
```

**`counts[idx]++` occurs at exactly ONE line — `:308` — inside the `for n := 0; n < 9; n++` loop over
`GET /api/v1/<n>` (`driver.go:293-309`).** The eight phase-89 arms already hit
`hmReflectBase = "/api/v1/reflect-headers/"` (`driver.go:476`) — **which is under `/api`** — through a
*different* loop at `:446-459` that never touches `counts`. Phase 89 landed green with `[3,3,3]` intact.

⇒ **THE REAL CONSTRAINT IS: DO NOT ADD REQUESTS TO THE `/api/v1/<n>` COUNTING LOOP.** Hitting `/api` is
fine and is already the established pattern.

**This is not a pedantic correction — it removes an entire class of work from the row.** The over-broad
reading forces a new route (and therefore a YAML edit on **both** sides, and therefore an update to
`TestRenderBootstrap_Subject`/`_Reference`, which assert over `renderBootstrap` output). The correct
reading does not, because the route table **already** has a documented fall-through for exactly this:

```yaml
# envoy-go.yaml:75-76 — the fixture's own words
#   entry here — `/api/v1/reflect-headers/a5` falls through to
#   the `prefix: "/api"` route below, which IS the …
```

Routes 2-8 are **exact** `path:` matches for `a1`-`a4`, `a6`-`a8`; `a5` has no entry and falls to
`- match: { prefix: "/api" }` (`:155`) → cluster `c_h2_backend` → the backend's
`/api/v1/reflect-headers/` **subtree** handler. **A phase-90 path of `/api/v1/reflect-headers/p90<N>`
takes exactly that fall-through.**

⇒ **D-90-YAML: `envoy-go.yaml` AND `envoy.yaml` ARE BYTE-UNTOUCHED.** No new route, no new port, no
placeholder change, and `driver_test.go`'s two `TestRenderBootstrap_*` tests need no update. **`+0` on
the config axis, and the row does not have to prove a route change is cross-side equivalent.**

### 3.1 ⚠️ A THIRD, SMALLER REFUTATION: `0004` HAS **SEVEN** `H2RoundTrip` CALL SITES, NOT NINE

The stage brief and SPEC §8.2 both say *"`0004`'s **9** `H2RoundTrip` call sites cannot express the arms."*
`git grep -c` reports `driver/driver.go:9` — **but `grep -c` counts matching LINES, and two of the nine are
PROSE**: `:433` (*"⚠️ NO ARM ASSERTS HEADER ORDER. helpers.H2RoundTrip sets client headers"*) and `:739`
(*"helpers.H2RoundTrip per Task 12. The runner's HTTPExpectations branch uses"*). **The real call count is
SEVEN**, at `:284 :294 :311 :342 :385 :398 :447`.

**The CONCLUSION is untouched** — seven inexpressible call sites are as inexpressible as nine — **but the
MEASUREMENT was wrong, and it is the same species as the `121`-vs-`122` blank-import lesson and the
`grep -c` counts-LINES trap this file's own §1 restates.** Recorded so the figure is not carried again.

---

## 4. ⚠️ A MEASUREMENT AGENT'S REFUTATION THAT DID **NOT** SURVIVE — recorded because the controller checked it

A measurement agent reported, as a decisive finding, that *"Go's `net/http` lifts the authority OUT of
`r.Header` into `r.Host`, so the sorted `r.Header` block the backend reflects can **never** contain it"* —
which, if true, would mean `0004`'s existing backend is blind to arm A and the row needs a much larger
backend change.

⚠️ **REFUTED BY THE CONTROLLER AT THE PINNED SOURCE.** `golang.org/x/net@v0.34.0/http2/server.go`:

```
2239	authority: f.PseudoValue("authority"),
2272	if rp.authority == "" {
2273		rp.authority = rp.header.Get("Host")
      	…
2341	delete(rp.header, "Trailer")          <- the ONLY delete
2367	Header:     rp.header,
2373	Host:       rp.authority,
```

```
$ grep -n 'Del("Host")' server.go   ->  rc=1, no output
```

**`Trailer` is the only key removed. A regular `host` field is canonicalized to `Host` and survives into
`req.Header` intact**, alongside `req.Host` carrying the `:authority`. ⇒ **SPEC §8.4's table is CORRECT
as written**, and its two load-bearing consequences both hold:

- **Arm A discriminates with a ZERO-line backend edit** — a regular `host` forwarded by the subject shows
  up as a `Host:` line in the already-sorted reflected block; the reference drops it, so the line is
  absent there.
- **Arm B needs `r.Host`** (~2 lines), because promotion moves a value into a field the reflected block
  never carries.

⚠️ **AND THE UNRECOVERABLE AXIS NOW HAS ITS MECHANISM NAMED, not just its existence asserted.** `:2272`'s
condition is `rp.authority == ""`, which is true **both** when `:authority` was ABSENT **and** when it was
PRESENT-AND-EMPTY. Both cases therefore take the `:2273` fallback and land identically as
`r.Host = <the host value>`, `r.Header["Host"] = [<the host value>]`. **The collision is a two-line
property of the pinned x/net server, not a vague limitation** — and no backend edit can undo it, because
the information is destroyed before any handler runs.

**METHOD NOTE, carried forward:** *the agent's claim was plausible, cited the right file, and was wrong.*
`feedback_brief_citations_not_evidence` applies to **subagent reports** exactly as it applies to stage
briefs — the controller re-derived it and must keep doing so.

---

## 5. ⚠️ THE GUARD ARGUMENT IS REBUILT: THE ROSTER IS ORGANISED BY **PROPERTY**, NOT BY SYMBOL

**SPEC §9.3 concludes:** *"`buildH2Request` has demonstrated sensitivity; `buildRequest` has demonstrated
zero. ⇒ the row's unit surface must cover `buildRequest` specifically."*

⚠️ **THE CONCLUSION IS RIGHT AND THE ARGUMENT IS WRONG.** Executed at this tip, in a probe worktree,
applied-and-reverted with `sha256sum -c` proving the restore:

| negative control | result |
|---|---|
| `authority = "NC2-BROKEN-AUTHORITY"` unconditionally in **`buildRequest`** | `rc=0`, **70 ok**, **0 FAIL**, **8201 `=== RUN`** — ⚠️ **SPEC CONFIRMED, whole tree blind** |
| `out.Authority = "NC3A-BROKEN-AUTHORITY"` unconditionally in **`buildH2Request`** | `rc=1` — `TestBuildH2Request_PseudoHeaderExclusion` + **exactly 4 subtests** — SPEC CONFIRMED |
| ⚠️ **`if h.Name == "host" { continue }`** unconditionally in **`buildH2Request`** — *the exact production behaviour arm A introduces* | ⚠️ **`rc=0`, BOTH packages `ok` — VACUOUS** |

⇒ **The asymmetry is NOT `buildH2Request` vs `buildRequest`. It is `H2Request.Authority`'s VALUE versus
everything else.** The only property pinned anywhere in the tree is the Authority *string*, by
`TestBuildH2Request_PseudoHeaderExclusion`'s `wantAuth`. **Carrier membership is unpinned in BOTH symbols.**

**A roster written to "cover `buildRequest` specifically" would therefore still ship a vacuous guard for
arm A**, whose whole content is a carrier deletion — inside `buildH2Request`, the symbol the SPEC believed
was already sensitive.

### 5.1 ⚠️ AND THE VACUITY IS PROVEN BLINDNESS, NOT DEAD CODE — a control the SPEC does not carry

A green negative control is worthless if the code never ran. Executed: `panic("NC2-REACH-buildRequest")`
as `buildRequest`'s first statement ⇒ `rc=1`, `panic: NC2-REACH-buildRequest`,
`FAIL github.com/pgdad/envoy-go/internal/filter/hcm/h2`. **`buildRequest` IS executed by the suite.**
⇒ the NC2 green is **assertion blindness**, measured. Reverted, `sha256sum -c` OK.
**Reproduce this control in the IMPL** — it is what makes the RED census's denominator mean anything.

### 5.2 The four properties the roster must pin, and which symbol owns each

| # | property | symbol | pinned today? |
|---|---|---|---|
| P1 | regular `host` **absent** from the upstream carrier `H2Request.Headers` | `buildH2Request` | ⚠️ **NO** — measured vacuous |
| P2 | `H2Request.Authority` promoted on `:authority` **ABSENT**, preserved (even empty) on **PRESENT** | `buildH2Request` | value pinned only for the non-promotion case |
| P3 | regular `host` **absent** from the decode map `req.Header` | `buildRequest` | ⚠️ **NO** |
| P4 | `req.Host` **and** `req.URL.Host` both carry the effective authority | `buildRequest` | ⚠️ **NO** — NC2 corrupted **both** and stayed green |

⚠️ **P4 IS TWO FIELDS, NOT ONE.** `buildRequest` sets `u.Host = authority` (`:503`) **and**
`Host: authority` (`:507`). A test asserting only `req.Host` misses half the surface — which is precisely
how NC2 corrupted both and went green.

### 5.3 The roster — `internal/filter/hcm/h2/authority_norm_test.go` (NEW), `package h2`

⚠️ **`package h2`, in-package** — both symbols are unexported. `stream_test.go` is already `package h2`,
so there is no new package-name decision. ⚠️ **`t.Errorf` per property, NEVER `t.Fatalf`**
(`reference_fatalf_makes_assertions_unreachable`) — the RED census needs *every* failing property to
print, and the existing `TestBuildH2Request_PseudoHeaderExclusion:879` already demonstrates the bug by
using `t.Fatalf` on a length mismatch.

**Precondition, stated so it is not re-derived:** H/2 field names are lowercase-only —
`buildRequest` rejects an uppercase name with `PROTOCOL_ERROR` before `buildH2Request` ever runs
(`dispatch` calls `buildRequest` at `:287` and returns early on error, then `buildH2Request` at `:306`
over the **same** `s.reqHeaders` slice). ⇒ **the suppression test is `h.Name == "host"` exactly. No
case-folding on this leg.** (`h2ReconcileSkipKey` uses `EqualFold` because it reads a *filter*-written Go
map, a different input — D-90-SKIP, §10.)

Helpers and the table:

```go
func hf(name, value string) hpack.HeaderField { return hpack.HeaderField{Name: name, Value: value} }

func carrierValues(hs []hpack.HeaderField, name string) []string {
	var out []string
	for _, h := range hs {
		if h.Name == name {
			out = append(out, h.Value)
		}
	}
	return out
}

func TestAuthorityNormalization(t *testing.T) {
	tests := []struct {
		name string
		in   []hpack.HeaderField
		// P1/P3: the regular `host` must be gone from BOTH outputs, always.
		// P2:    the effective authority.
		wantAuthority string
	}{
		{
			// POSITIVE CONTROL: must PASS on the UNPATCHED tip. If this ever
			// fails, the table is vacuously red and proves nothing.
			name:          "P_authority_only",
			in:            []hpack.HeaderField{hf(":method", "GET"), hf(":path", "/"), hf(":scheme", "https"), hf(":authority", "a.example")},
			wantAuthority: "a.example",
		},
		{
			// ARM A — both present. :authority WINS (it was present); the
			// regular host is suppressed from carrier AND decode map.
			name:          "A_both",
			in:            []hpack.HeaderField{hf(":method", "GET"), hf(":path", "/"), hf(":scheme", "https"), hf(":authority", "a.example"), hf("host", "h.example")},
			wantAuthority: "a.example",
		},
		{
			// ARM B — host only. :authority ABSENT => PROMOTE.
			name:          "B_host_only",
			in:            []hpack.HeaderField{hf(":method", "GET"), hf(":path", "/"), hf(":scheme", "https"), hf("host", "h.example")},
			wantAuthority: "h.example",
		},
		{
			// ARM C — :authority PRESENT-AND-EMPTY. D-90-SCOPE: PRESENT wins,
			// so the authority stays EMPTY. This is the shape §3(d) of the SPEC
			// says the deferral makes visible, pinned so it is never discovered.
			name:          "C_empty_authority",
			in:            []hpack.HeaderField{hf(":method", "GET"), hf(":path", "/"), hf(":scheme", "https"), hf(":authority", ""), hf("host", "h.example")},
			wantAuthority: "",
		},
		{
			// ARM E — FIRST OCCURRENCE WINS as the promotion source.
			name:          "E_dup_host_first_wins",
			in:            []hpack.HeaderField{hf(":method", "GET"), hf(":path", "/"), hf(":scheme", "https"), hf("host", "first.example"), hf("host", "second.example")},
			wantAuthority: "first.example",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			h2req := buildH2Request(tc.in, nil)

			// P2 — the effective authority.
			if h2req.Authority != tc.wantAuthority {
				t.Errorf("Authority = %q, want %q", h2req.Authority, tc.wantAuthority)
			}
			// P1 — regular host absent from the upstream carrier.
			if got := carrierValues(h2req.Headers, "host"); len(got) != 0 {
				t.Errorf("carrier carries host %v, want none", got)
			}

			req, err := buildRequest(tc.in, nil)
			if err != nil {
				t.Errorf("buildRequest: unexpected error %v", err)
				return
			}
			// P3 — regular host absent from the decode map. NOTE the CANONICAL
			// key: regular.Add() routes through textproto, so it lands under
			// "Host", never "host".
			if got := req.Header.Values("Host"); len(got) != 0 {
				t.Errorf("decode map carries Host %v, want none", got)
			}
			// P4 — BOTH fields, because buildRequest sets both and NC2 proved
			// corrupting both alone is invisible to the whole tree.
			if req.Host != tc.wantAuthority {
				t.Errorf("req.Host = %q, want %q", req.Host, tc.wantAuthority)
			}
			if req.URL.Host != tc.wantAuthority {
				t.Errorf("req.URL.Host = %q, want %q", req.URL.Host, tc.wantAuthority)
			}
		})
	}
}
```

### 5.4 ⚠️ THE ONE ARM THAT PINS WHAT THIS ROW DELIBERATELY DOES **NOT** CHANGE

Per **D-90-DUP** (§2.1), a duplicate `:authority` keeps its silent last-wins behaviour. That is a
*decision*, and an undefended decision is what an implementer regresses. Separate test, because it asserts
non-change rather than change:

```go
// D_dup_authority pins the behaviour D-90-DUP deliberately LEAVES ALONE.
// :method/:path/:scheme each reject a duplicate with PROTOCOL_ERROR; :authority
// does NOT, and this row does not add that reject — it belongs to the deferred
// arm-C row, which owns the reject shape decision. Without this arm, "we did not
// change duplicate handling" is a claim, not a measurement.
func TestAuthorityNormalization_DuplicateAuthorityUnchanged(t *testing.T) {
	in := []hpack.HeaderField{
		hf(":method", "GET"), hf(":path", "/"), hf(":scheme", "https"),
		hf(":authority", "first.example"), hf(":authority", "second.example"),
	}
	req, err := buildRequest(in, nil)
	if err != nil {
		t.Errorf("buildRequest: got error %v, want nil (duplicate :authority is NOT a reject at this row)", err)
		return
	}
	if req.Host != "second.example" {
		t.Errorf("req.Host = %q, want %q (last-wins, unchanged by row 90)", req.Host, "second.example")
	}
	if got := buildH2Request(in, nil).Authority; got != "second.example" {
		t.Errorf("Authority = %q, want %q (last-wins, unchanged by row 90)", got, "second.example")
	}
}
```

⚠️ **This arm PASSES on the unpatched tip and must STILL PASS after the fix. It is not part of the RED
census** — it is a stability pin, and §6's T1 records it in the GREEN column of the baseline so a reader
cannot mistake it for a arm that failed to redden.

### 5.5 The RED census — ALREADY OBSERVED. Reproduce it; do not invent a different one.

Executed against the **UNPATCHED** tip with the table above, `-count=1 -v`:

```
--- FAIL: TestAuthorityNormProbe (0.00s)
    --- PASS: TestAuthorityNormProbe/P_authority_only    <-- POSITIVE CONTROL: not vacuously failing
    --- FAIL: TestAuthorityNormProbe/A_both
        host on carrier = true, want false
        req.Header Host = "h.example", want absent
    --- FAIL: TestAuthorityNormProbe/B_host_only
        Authority = "", want "h.example"
        req.Host  = "", want "h.example"
    --- FAIL: TestAuthorityNormProbe/C_empty_authority
        host on carrier = true, want false
```

**5/5 PASS with the prototype installed** (`rc=0`), probe deleted, tree proven clean.
⚠️ **`E_dup_host_first_wins` and `D_dup_authority` are NEW at this PLAN and are NOT in that transcript** —
the IMPL must record its own census over the **full** roster and assert the denominator, not paste this one.

---

## 6. THE ORDERED IMPL TASKS (TDD; both censuses observed BEFORE any production edit)

**NINE tasks. The split gate is NOT tripped** — see §10.4.

- [ ] **T1 — RED census. Tests only; ZERO production bytes.**
  - T1a `internal/filter/hcm/h2/authority_norm_test.go` (**NEW**) — the §5.3 table (5 arms) + the §5.4
    stability pin. Every test compiles against the unmodified tip: it calls only `buildH2Request` and
    `buildRequest`, which both already exist, so **the RED is a clean assertion failure, never a build break.**
  - T1b **the reachability control of §5.1** — `panic("NC-REACH-buildRequest")`, observe `rc=1` + the panic
    line, revert with `sha256sum -c`. **Without it the census's green columns prove nothing.**
  - T1c **Observe and RECORD the census with denominators asserted** against the §10.2 green baseline:
    `rc`, `ok` count, anchored FAIL count, `=== RUN` count, and **which arms PASS** (`P_authority_only`
    and `D_dup_authority` must PASS; A/B/C/E must FAIL). ⚠️ Run with `-v` — **`go test` without `-v`
    prints zero `=== RUN`, and a census reading `RUN=0` beside `RC=0` is a VACUOUS GREEN.**

- [ ] **T2 — production: `buildH2Request`.** `internal/filter/hcm/h2/stream.go`, the switch at `:334-347`.
  Add a `case "host":` arm capturing the **first** occurrence and suppressing every occurrence from the
  carrier; track `:authority` presence with `authoritySeen`; after the loop, promote **only when
  `!authoritySeen`**. ⚠️ **This ADDS a branch — there is no `host` branch to modify (§2).**

- [ ] **T3 — production: `buildRequest`.** Same file, the regular-header loop ending at `regular.Add` (`:472`)
  and the request construction at `:502-507`. `continue` before `regular.Add` for `host`, latching the
  first value; `authoritySeen` at the `:authority` case (`:446-447`); promote into `authority` before
  `u.Host`/`Host` are set. ⚠️ **`authoritySeen` tracks PRESENCE ONLY — do NOT add the duplicate reject
  its three sibling `…Seen` booleans carry (D-90-DUP, §2.1).**

- [ ] **T4 — GREEN census.** The full §5.3 roster **and** §5.4 pin green; `gofmt -l` **output** empty
  (⚠️ it never exits non-zero — gate on OUTPUT); the **three** symbol assertions of §9.1; the
  slice-only-writer gate re-read at **6** with §10.3's statement.

- [ ] **T5 — the `0004` backend edit** (§7.2) — ~6 lines in `test/fixtures/0004-h2-routing/backends/main.go`.

- [ ] **T6 — the `0004` raw-framer arm block** (§7.3) — appended after `driver.go:459`, before `return`.

- [ ] **T7 — the break protocol, FOUR arms, FOUR DISTINCT injection sites** (§7.5).

- [ ] **T8 — docs** (§8): ADR-0312 completed (**guard 1 -> 0**), the `ROADMAP.md` row-90 text correction,
  the `BEHAVIOR_CONTRACT.md:2034` rider, the ROADMAP row flip to `done`, `PROGRESS.md`, `STATE.md`,
  `STATE_HISTORY.md`, `next-prompt.txt`.

- [ ] **T9 — gates, run LAST against the FINAL tree** (§10.1).

⚠️ **T2 and T3 are separate tasks deliberately.** They are separate *properties* (§5.2 P1/P2 vs P3/P4) in
separate functions, and §5's measurement says each is independently unpinned — a reviewer must be able to
reject one while approving the other.

---

## 7. THE DIFFERENTIAL RECIPE — extend `0004-h2-routing` IN PLACE

### 7.1 ⚠️ D-90-TARGET: THE ASSERTION AXIS, CHOSEN DELIBERATELY (SPEC §13 item 5)

The SPEC names two candidates and requires a deliberate choice.

| candidate | verdict |
|---|---|
| a **route** assertion | ⚠️ **FORBIDDEN.** SPEC §4 proves it cannot discriminate: `routeMatch` is literally `matches(path string) bool` and `git grep '\.Host' -- internal/filter/hcm/route.go` ⇒ **rc=1** against a positive control of **3** on `.Path`. Both YAMLs are `domains: ["*"]`, so no vhost selection exists to perturb either. |
| the **access-log / Zipkin-span** axis (§4.1) | ⚠️ **REJECTED, on a measurement the SPEC did not take.** `git grep -l 'access_log\|zipkin\|tracing'` over **each** H2-capable fixture ⇒ `0004` **rc=1**, `0079` **0**, `0080` **0**, `0119` **0**. **NOT ONE of the four H2-capable fixtures carries an access-log or tracing surface.** Buying this axis means minting that surface *and* a log-collection path — larger than the fix, and outside a `+0`-on-every-axis row. |
| ✅ the **`host`-presence + `r.Host`** axis | **CHOSEN.** Both halves are reachable from `0004`'s existing backend; §4 proves the `Host:` line survives into `r.Header`; `r.Host` costs ~6 lines. |

⚠️ **AND THE CHOICE IS BOUNDED, NOT SOLD AS COMPLETE.** The access-log/Zipkin flip **is** the row's most
user-visible consequence and this row does **not** assert it. It is registered as deferred follow-on **(v)**
in §11 rather than left as an implied gap.

### 7.2 D-90-BACKEND — the `r.Host` edit, with its exact insertion point

`test/fixtures/0004-h2-routing/backends/main.go`. `git grep 'r\.Host' -- test/fixtures/0004-h2-routing/`
⇒ **rc=1, zero hits.** The reflect handler is registered at `:119`; its sorted block closes at `:131` and
`w.Header().Set` follows at `:132`. **Insert between them:**

```go
		sort.Strings(names)
		var b strings.Builder
		for _, n := range names {
			for _, v := range r.Header[n] {
				fmt.Fprintf(&b, "%s: %s\n", n, v)
			}
		}
+		// Phase 90: the AUTHORITY the proxy forwarded. x/net's H/2 server puts
+		// :authority into r.Host (server.go:2373) and leaves a regular `host`
+		// field in r.Header as "Host" (only "Trailer" is deleted, :2341), so the
+		// sorted block above already shows arm A while r.Host shows arm B.
+		// Emitted AFTER the sort, deliberately: folding it into `names` would let
+		// a lexical sort move it and re-baseline every existing arm.
+		fmt.Fprintf(&b, "x-observed-authority: %s\n", r.Host)
		w.Header().Set("Content-Type", "text/plain")
```

⚠️ **PRESENT-AND-EMPTY IS DISTINGUISHABLE FROM ABSENT, and that is load-bearing for arm B.**
`parseReflectedHeaders` (`driver.go:613`) splits on the first `": "`. An empty `r.Host` emits
`x-observed-authority: ` — the separator is still present, so the key parses with value `""`.
⇒ `hmWantValues(got, "x-observed-authority", "")` and `hmWantAbsent(...)` are **different** assertions.

⚠️ **THE ONE AXIS THIS BUYS NOTHING FOR, restated with its mechanism (§4):** `:authority` **ABSENT** and
`:authority` **PRESENT-AND-EMPTY** both satisfy `rp.authority == ""` at `x/net .../server.go:2272` and
both take the `:2273` fallback to the `Host` header, landing **byte-identical** in `r.Host` *and*
`r.Header`. **No backend edit can recover it** — the information is destroyed before any handler runs.
Recovering it needs a raw-framer **backend**, i.e. `BackendKind = 39`. **THE ROW DOES NOT BUY THAT.**
The two in-scope arms are fully discriminated without it; arm C, which is where the distinction would
matter, is deferred.

**Transcript safety:** the reflected body never enters the differential byte stream (`driver.go:426-431`,
README `:88`), so this line cannot re-baseline anything.

### 7.3 The instrument — a raw framer, copied from `0119`, ONE FRESH CONNECTION PER ARM

`helpers.H2RoundTrip` is unusable, on **three** independently verified mechanisms:

1. **No `req.Host` surface** — the signature has no host parameter and the body never sets it; the URL is
   hard-built as `"https://"+addr+path` (`h2.go:63`).
2. **A `host` entry is SILENTLY DROPPED** — `x/net@v0.34.0/http2/transport.go:2162` `continue`s on
   `asciiEqualFold(k, "host")`. ⇒ **passing `{Name:"host"}` through this helper is a VACUOUS arm.**
3. **`:authority` cannot be set, emptied, or injected** — `:2102-2105` derives it from
   `req.Host`/`req.URL.Host`, `:2146` emits it unconditionally, and `validateHeaders` (`:2073-2087`)
   rejects a literal `":authority"` header name outright.

The instrument exists in-tree; all five cites **re-confirmed EXACT** at this tip:
`requestFields` pseudo-header list **355-358** · `http2.ClientPreface` **474** · `http2.NewFramer` **478** ·
`hpack.NewDecoder` **479** · `fr.WriteHeaders` **491** (`test/fixtures/0119-grpc-unary-trailers/driver/driver.go`).

⚠️ **ONE FRESH TLS CONNECTION PER ARM, and it is `0119`'s existing shape, not a new invention.**
`driveArm` dials at `:455`, creates its decoder at `:479`, uses `streamID = 1`, and `defer conn.Close()`
at `:466`. This simultaneously satisfies **three** constraints:
`reference_hpack_decoder_must_be_per_connection` (one decoder, one connection) · SPEC §8.5's
*"if arm C is ever added it must be LAST and must not be pipelined"* (there is no pipeline to poison) ·
and the arms' mutual independence.
⚠️ **`0004` is TLS+ALPN-h2, not h2c** — the arm must assert
`conn.ConnectionState().NegotiatedProtocol == "h2"` before writing the preface, exactly as `0119:468` does,
and reuse `d.loadTLSConfig()` (`driver.go:280`). `go.mod` **+0**: `golang.org/x/net` is already a direct
require (`go.mod:18`) and `http2` is already imported by five files under `test/`.

### 7.4 The arm roster — path, wire shape, and what each discriminates

**Path: `/api/v1/reflect-headers/p90<n>`.** ⚠️ **NO YAML EDIT** — routes 2-8 are *exact* matches for
`a1`-`a4`/`a6`-`a8`, so a `p90*` path falls through to `- match: { prefix: "/api" }` (`envoy-go.yaml:155`)
→ `c_h2_backend` → the backend's `/api/v1/reflect-headers/` **subtree** handler, exactly as `a5` already
does by design (§3). **`counts[idx]++` lives only at `driver.go:308`, inside the `/api/v1/<n>` loop, so
`AssertDistribution`'s `[3,3,3]` is untouched.**

| # | arm | wire (hand-built hpack fields) | asserts on the reflected body |
|---|---|---|---|
| **P90-P** | positive control | `:authority: p90.example` only | `x-observed-authority: p90.example` **and** `hmWantAbsent(got, "host")` — ⚠️ **must PASS on BOTH sides pre-fix**; if it ever fails the roster is vacuously red |
| **P90-A** | both authorities | `:authority: p90.example` **+** `host: p90host.example` | `hmWantAbsent(got, "host")` — pre-fix the **subject** emits `host: p90host.example` and the reference does not |
| **P90-B** | host only | `host: p90host.example`, **no** `:authority` | `x-observed-authority: p90host.example` **and** `hmWantAbsent(got, "host")` — pre-fix the subject reads `""` |
| **P90-E** | first-occurrence-wins | `host: first.example`, `host: second.example`, no `:authority` | `x-observed-authority: first.example` |

⚠️ **ARM C IS NOT IN THIS ROSTER.** It is deferred (§11 (i)), and SPEC §8.5 measured that adding it
pipelined kills P/A/B with **zero backend deliveries**. One-connection-per-arm removes the hazard, but the
arm still has nothing to assert: the reference emits **zero bytes** by default and neither reference stat
exists in the subject.

⚠️ **THE ARM BLOCK MUST NOT BE FAIL-FAST.** `0004`'s existing arms assert in-band with
`return nil, fmt.Errorf(...)` (`driver.go:449/452/456`), so **the first failing arm aborts the whole Drive
and every later arm is unreachable** (`reference_failfast_driver_masks_later_red_arms`). The phase-90 block
follows **`0119`'s discipline instead** (`driver.go:429-432`: *"It NEVER returns an error: failures are
recorded IN the transcript"*): each arm writes a normalized line into `out` and records failures as text.
The **cross-side byte-compare the runner already performs becomes the assertion**, all four arms always
run, and a break in one cannot mask another. Recorded line shape, one per arm:

```go
fmt.Fprintf(&out, "p90-%s:auth=%q host=%v\n", arm.name, gotAuthority, gotHostPresent)
```

⚠️ **These bytes MUST be identical cross-side once the fix lands** — they are, because every arm sends a
**fixed literal** authority (never the dial address, which differs between the container reference and the
in-process subject). ⚠️ **And they differ cross-side BEFORE the fix, which is the point** — that is the
RED signal, and it is a genuine differential, not a self-comparison.

### 7.5 The break protocol — FOUR arms, FOUR DISTINCT injection sites

⚠️ **One break per run, `-count=1`** (`reference_differential_break_protocol_count1` — the cache serves a
stale PASS). ⚠️ **Confirm WHICH assertion fired**, not merely that the run went red
(`reference_deliberate_break_wrong_assertion`).

| break | injection site (NAMED, and DISTINCT) | must redden |
|---|---|---|
| **B1** | delete the `case "host":` suppression arm in **`buildH2Request`** | P90-A and P90-B (`host` reappears on the carrier) |
| **B2** | delete the `!authoritySeen` guard, promoting **unconditionally** | P90-P (its `:authority` gets overwritten by the `host`) — ⚠️ **a stacked control: B1 cannot reach P90-P, so a break that reddens P90-P proves the guard, not the suppression** |
| **B3** | delete the `continue` before `regular.Add` in **`buildRequest`** | ⚠️ **NOTHING in the differential** — see below |
| **B4** | replace the first-occurrence latch with last-wins | P90-E only |

⚠️ **B3 IS THE SHARPEST ARM AND IT IS EXPECTED TO BE DIFFERENTIALLY SILENT.** `buildRequest`'s decode map
feeds the *filter chain*, not the upstream wire; `0004` runs an **empty** `header_mutation` plus `router`
(`envoy-go.yaml:165-178`), so nothing reads the map. **B3 must therefore redden the UNIT roster (§5.3 P3)
and not the fixture** — and the IMPL must **record that asymmetry as the observed result**, because a
break arm that reddens nothing anywhere is indistinguishable from a break arm that was never applied
(`reference_break_arm_injection_site_is_a_claim`). ⚠️ **Assert B3 landed by grepping the patched file, not
by observing a build.**

⚠️ **A FIFTH BREAK IS DELIBERATELY NOT WRITTEN.** There is no break that isolates `req.URL.Host` from
`req.Host` — both are set from the same `authority` local, so any injection moves both. **P4 is pinned by
two unit assertions, not by a break**, and §5.2 says so rather than letting a reader infer a break exists.

---

## 8. DOCS — TEXT CONSTRAINED AT THIS PLAN, LANDED AT THE IMPL

### 8.1 `ROADMAP.md` row 90 — the FALSE sentence, and the anchor to fix it by

Row 90 (`:152`) contains, verbatim:

> so **an empty authority is a ROUTE-MATCHING input, not merely a wire artifact**

⚠️ **FALSE**, on three independent measurements (SPEC §4, re-run at this tip in §7.1). **The IMPL replaces
it in the same commit that flips row 90 `done`.** Replacement text, fixed here so the IMPL does not
improvise:

> so an empty authority reaches **`http.Request.Host` and the access-log/tracing `Authority` field** —
> **NOT** the route table, whose `routeMatch` interface is `matches(path string) bool` and cannot see the
> request at all.

⚠️ **ANCHOR ON THE SENTENCE, NOT THE LINE** (`reference_brainstorm_adjective_acquires_adr_authority`: an
adjective that travels acquires authority by repetition). The gate is a **residual count**:
`grep -c 'ROUTE-MATCHING input' docs/envoy-go/ROADMAP.md` ⇒ **1 before, 0 after**, with
`grep -c 'routeMatch' docs/envoy-go/ROADMAP.md` as the positive control on the replacement landing.
⚠️ **Row 90 is ONE line — the edit must not change the row's field count.** Re-run the ARM-A escape-aware
guard afterwards: ids **{57, 69}** and **no third id**.

### 8.2 `BEHAVIOR_CONTRACT.md:2034` — a RIDER, not a rewrite

Found **by string, not by line** (`grep -n 'forwarded verbatim'` ⇒ **two** hits; **`:2033` is about
RESPONSE headers and is a decoy**). The `:authority` sentence is `:2034`:

> NEW (05.2): routed-to-upstream H2 request preservation — `:method`/`:path`/`:scheme`/`:authority`
> forwarded verbatim from downstream to upstream (witnessed by the in-process backend in fixture 0004's
> tests asserting on received pseudo-headers).

⚠️ **Promotion makes *"forwarded verbatim"* UNTRUE for `:authority`.** The bullet **already carries** a
phase-87 ADR-0309 rider about `:path` parsing — **that is the vehicle and the precedent.** The IMPL adds a
second rider to the **same bullet** rather than rewriting the sentence, so every by-line citation into the
HTTP/2 section stays valid. Rider text, fixed here:

> ⚠️ **RIDER (phase 90, ADR-0312): `:authority` is no longer forwarded verbatim in one shape.** When the
> downstream field list carries **no** `:authority`, a regular `host` is **promoted** into it; a regular
> `host` is **suppressed** from the upstream carrier in **every** shape. `:authority` **present** — even
> present-and-empty — is still forwarded verbatim. The **validity reject** the reference applies to an
> invalid authority is **NOT** implemented (deferred; see ADR-0312 §Consequences).

⚠️ **CITE BY STRING OR SYMBOL THROUGHOUT.** Every by-line cite at or below the `## HTTP/2` asserted list
has shifted **+2** (phase 89), **+2** (phase 88) and **+1** (phase 87).

### 8.3 `DECISIONS.md` — ADR-0312 completed at the IMPL, and **NOTHING at this PLAN**

`DECISIONS.md` is **BYTE-UNTOUCHED this stage**: **18297** lines, **311** `^## ADR-` headings, tail
**ADR-0312**, next-free **ADR-0313**, `^---$` **216**, strict `PROPOSED` guard **1**.

⚠️ **THE HEADING UNIT IS `^## ADR-`, AND THE CARRIED FIGURE OMITS ITS SCOPE.** `grep -c '^## '` reads
**319** at this tip — the extra **8** are `## Amendment (per phase …)` headings. **Both figures are right;
they measure different things.** Same species as the `121`-vs-`122` blank-import lesson. **State the scope
whenever the number is restated.**

At the IMPL: §Decision + §Consequences appended **IN PLACE** after the RETAINED italic footer, **no
renumber, NO `---` separator** (`^---$` stays **216**), and the strict guard **1 -> 0**. §Consequences must
carry the five deferred follow-ons of §11 **as named rows**, and must record **this PLAN's four
refutations of the SPEC** (§0) — including that ADR-0312 **§Context ¶1's own `regular.Add` wording** reads
as if a `host` branch exists.

### 8.4 Documentary defects — carried forward, plus THREE NEW at this stage

**NEW:** ⚠️ the SPEC's *"`0004`'s **9** `H2RoundTrip` call sites"* is a `grep -c` **LINE** count; the real
call count is **SEVEN** (§3.1) · ⚠️ SPEC §8.3's `discoverFixtures` cite `1462-1497` is off — the function
**ends at `:1499`** · ⚠️ the stage brief's phantom-symbol gate `git grep -c 'h2.parseHeadersForRequest'`
reads **1**, not 0 — it counts a **comment citation**; the *definition* selector
`^func.*parseHeadersForRequest` is the one that reads **0** (positive control `buildH2Request` ⇒ 1).
**Installing the brief's form as a gate would pin a number that is neither count.**

**Carried, still deliberately NOT fixed:** `0004-h2-routing/envoy-go.yaml:3` *"documentation only"* while
`driver.go:214` does `readYAML("envoy-go.yaml")` · the subject's `User-Agent: Go-http-client/1.1`
injection on H1 upstream requests where the reference injects none · ADR-0310 §Consequences (x)'s C3
*"deferred parity"* and (xi)'s *"~64 KiB band"* · `DECISIONS.md`'s `INNER_EXIT=0` at phase 87, a value
nothing in the tree emits · `internal/filter/http/types.go`'s false *"per ADR-0071"* comment · the H1→H2
502 cite and its unrecorded H3 leg · stale H1-trailers prose at `chain.go:477-489` and
`connection.go:562-568` · `header_mutation` rejecting `remove` of a protected header · `RunAction`'s
unguarded H2 arm · `ADR-0051` §2 · `ADR-0058`'s dead `routerActionH2.doH2` location · `ADR-0057`'s
"27 round-trips" (now **31**) · **`ROADMAP.md` rows 57/69 malformed (the ARM-A GUARD)** ·
**`STATE.md` §Project counts FROZEN at phase 76** · `harness_test.go` port inventory STALE · `body.go`
nolint INERT · the xDS cycle guard NOT AUTOMATED · `wasm/doc.go:219` two errors · ROADMAP cites
`esalaine` **five** times against a module path of `github.com/pgdad/envoy-go` · `rbac.go:50` token `F2` ·
root `PROGRESS.md` stray phase-32.1 doc · SPEC-86/PLAN-86 citing a nonexistent `internal/xds/xdsgrpc/…`
path · the TWO riders citing the ADR-0052 mandate at a DRIFTED `:1821` · **window `:222` carries TWO
CLOSED candidates** (§1.2).

---

## 9. COST — RE-MEASURED AT THIS PLAN'S OWN TIP, AND THE SPEC'S FIGURE IS REFUTED

`reference_cost_figure_measured_at_publishing_commit` has fired in **three** consecutive rows. Re-measured
at `c7fef29b` by a compiling prototype implementing the full D-90-SCOPE rule in **both** symbols, applied,
`gofmt`'d, measured, and reverted with `sha256sum -c` proving the restore:

```
git diff --numstat -- internal/filter/hcm/h2/stream.go   ->   36   0
gofmt -l   ->   EMPTY both before and after gofmt -w  (no realignment; the figure is already post-gofmt)
```

| form | cost | source |
|---|---|---|
| BRAINSTORM floor | +15 / −1 | phase-90 BRAINSTORM |
| comment-free minimum | +14 / −1 | phase-90 SPEC |
| SPEC's prototype | +34 / −0 | phase-90 SPEC |
| ⚠️ **this PLAN's prototype** | **+36 / −0** | measured at `c7fef29b` |

### 9.0 ⚠️ WHY `c7fef29b` **IS** THIS PLAN'S PUBLISHING COMMIT — the argument, not an assumption

`reference_cost_figure_measured_at_publishing_commit` demands the figure be taken at the commit that
publishes it, not at an earlier tip. **The figure above satisfies that STRUCTURALLY, and the argument is
stated rather than left implicit:** this stage is **docs-only — ZERO production `.go`, ZERO test `.go`**
(§10.5), so the production tree at this PLAN's publishing commit is **byte-identical** to `c7fef29b`,
against which the prototype was applied and reverted. The gate is
`git diff --stat c7fef29b -- '*.go'` ⇒ **EMPTY** at the publishing commit; if it is ever non-empty, the
figure is stale and must be re-measured before the commit lands.
⚠️ **The IMPL has no such shortcut** — it changes `.go` by construction, so it must re-measure at its
own publishing commit, and the lesson has now fired in **three** consecutive rows.

### 9.1 ⚠️ THE SYMBOL ASSERTION IS **THREE** SYMBOLS, NOT TWO — the SPEC's table is incomplete

```
authoritySeen   6      <- SPEC says 6.  CONFIRMED.
hostField       6      <- SPEC says 7.  ⚠️ REFUTED.
hostSeen        8      <- ⚠️ the SPEC NAMES NO SUCH SYMBOL.
```

**The third symbol is not padding — the rule cannot be written without it.** In `buildRequest`, a bare
`hostField != ""` test **cannot distinguish ABSENT from PRESENT-AND-EMPTY**, and it cannot express
first-occurrence-wins either. Both are explicit requirements of D-90-SCOPE. ⇒ **the IMPL's symbol gate is
a THREE-row table**, and a two-row table would leave the row's own promotion latch unasserted.

⚠️ **`reference_measured_prototype_is_a_lower_bound` FIRES A THIRD TIME, and the cause is again
UNDER-ENUMERATION, not estimation error** — each stage's figure rose because each stage found another
thing the rule requires. **Treat +36 as a floor.**

**IMPL bands** (production revised upward from the SPEC's): production **~+36-65** · unit **~+130-260**
(five arms + the stability pin + helpers) · differential **~+150-250** (raw framer ~125 + arm table ~60,
discounted against phase 89's MEASURED `+688/−24` into this same fixture because this row adds arms rather
than a reconciler) · backend **~+6**. **PRICED SEPARATELY AND NOT IN THE BAND:** arm C (**+32/−0** stream-
scoped or **+31/−0** over two files connection-scoped), H1-D (**+76/−1**), H1-B′, the H3 leg.

---

## 10. GATES AND COUNTS

### 10.1 The IMPL's gates (T9, run LAST against the FINAL tree)

The **six-gate posture: NAME DEPARTURES, DO NOT CLAIM COMPLIANCE.**

- **(a)/(b)** `go test ./...` + the differential suite, **121 fixtures**, gated on **`PIPESTATUS[0]`** and
  a **SET RECONCILIATION**. ⚠️ **`INNER_EXIT` DOES NOT EXIST** — the suite is a direct
  `go test ./test/differential/`, so the outer process IS the inner process.
  ⚠️ **`rc=$?` after a pipe returns the LAST command's status** — use `out=$(…); rc=$?` or `PIPESTATUS`.
- **(c)** h2spec **cite only from your own run** — and ⚠️ **it is STRUCTURALLY incapable of anchoring this
  row**: its harness configures `envoy.filters.http.router` ALONE with `direct_response` routes and never
  goes upstream, so no authority is ever forwarded. grpc-conformance deferred in writing; proxy-wasm 10/16.
- **(d)** fuzzers **55 / 48 files**, **+0** (this row consumes no new config field, so there is no parse
  arm to fuzz).
- **(e)** the **ANCHORED** panic gate `^panic:|DATA RACE|SIGSEGV` on every differential launch — an
  unanchored form false-fires 14 times on a fully green log.
- **(f)** no `REVIEW.md` — **the standing departure, NAMED**. `git ls-files | grep -c 'REVIEW\.md$'` ⇒
  **37** (a FILE count), newest at `phases/25.3-…`; no phase since 25.3 has written one.

⚠️ **THE FIXTURE INVOCATION.** `go test ./test/differential/ -run 'TestDifferential/0004-h2-routing' -count=1 -v`.
⚠️ **`0004`'s own README `:127-131` omits `-count=1` — DO NOT copy it verbatim**; the break protocol edits
the driver *and* backend packages, so a cache hit is live. ⚠️ **A `-run` selector matching nothing prints
`[no tests to run]` and EXITS 0** — use the full `TestDifferential/0004-h2-routing` form, never a bare
`0004`. `TestDifferential` is skipped under `-short` and calls `ensureDocker(t)`.

### 10.2 ⚠️ THE GREEN BASELINE — and TWO NAMED FLAKES THE GATE WILL OTHERWISE READ AS REGRESSIONS

`go test ./internal/... -count=1 -v` at the clean tip: **`rc=0`, 70 `ok`, 0 anchored FAIL, 8201 `=== RUN`,
anchored panic gate 0.**

⚠️ **THE SPEC'S *"the 69 was a measurement artifact and the flake did not recur"* IS PARTIALLY REFUTED.**
A 69 reading was reproduced **at this stage**, and it came with **`rc=1` and a real failure**:

```
--- FAIL: TestSDSEndToEnd_FetchFailure_BootFailsClosed   (internal/boot)      1 of 3 full-suite runs
--- FAIL: TestProvider_FetchInitialCertificate_Timeout   (internal/xds)       3 of 8 full-suite runs
```

Both pass **5/5 in isolation**. Both are load-dependent **SDS initial-fetch timeouts** — the standing
`SDS dial-budget` class in the index. ⚠️ **CAUSATION EXCLUDED MECHANICALLY, not by re-running:**
`go list -deps -test ./internal/xds/ | grep -c 'filter/hcm/h2'` ⇒ **0** — the `internal/xds` test binary
**does not link the patched package at all**, so the prototype cannot be its cause.

⇒ **69 IS NOT A COUNT DISCREPANCY; IT IS A RED RUN.** Anyone recording "69" without also recording `rc`
and the FAIL line has recorded a failure as a counting curiosity. **The IMPL's gate must name both tests,
or it will read a flake as a row-90 regression.**

### 10.3 ⚠️ A GATE HAZARD FOUND WHILE MEASURING — the FAIL counter itself

On `-v` output, **`grep -c 'FAIL'` reads 11 on a FULLY GREEN tree**: nine
`INFO wasm: FAIL_CLOSED/FAIL_OPEN …` log lines plus two lines of a **test name** containing `boot-FAILS`.
**Use the anchored form** `grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL'`, which reads **0** on the green
baseline. Same species as the anchored panic gate — and it is `reference_gate_command_negative_control`:
**the gate itself can be broken.**

**The slice-only-writer gate** reads **6** by the canonical form, verbatim:

```
internal/filter/hcm/h2/stream.go:345     out.Headers = append(out.Headers, h)          <- THIS ROW MODIFIES #1
internal/filter/hcm/h2dispatch.go:439/:444/:449/:451                                    upsertH2Header ×4
internal/filter/hcm/h2dispatch.go:597    reconcileH2DecodeDelta
```

⚠️ **THIS ROW MODIFIES WRITER #1 RATHER THAN ADDING A SEVENTH — the gate's expected reading at row-done is
STILL 6, and the IMPL must STATE that**, or a reader will take an unchanged gate as an unchanged file.
⚠️ **The companion `*H2Request` counter is VACUOUS as a control** — it reads **0**, exactly like a
deliberately fabricated selector, so it cannot discriminate. Use `git grep -c 'H2Request'` (**20** files)
as the live positive control instead. **NAMED BLIND AXES:** element-level in-place mutation
(`.Headers[i].Value = …`); any writer outside `internal/filter/hcm/`; and ⚠️ **the `resp\.Headers`
exclusion silently drops `h3dispatch.go` even though the pathspec covers it** — an H3 blindness acquired
as a side effect, named here rather than inherited.

### 10.4 ⚠️ THE SPLIT GATE — EVALUATED, NOT ASSUMED

BOOTSTRAP §5 state 2: *"if PLAN.md > ~25 tasks OR > ~1500 LoC estimated → split into 90.1, 90.2 …"*

- **Tasks: NINE** (§6) — **9 ≤ 25.**
- **LoC: ~+322 to ~+581** — production ~+36-65 · unit ~+130-260 · differential ~+150-250 · backend ~+6.
  **581 ≤ 1500**, with the upper bound **2.6× under** the gate.

⇒ **THE GATE IS NOT TRIPPED. NO SPLIT. `want` STAYS 122 and no sub-phase row is minted.** ⚠️ Note the
bound holds **because** arm C, H1-B′, H1-D and the H3 leg are deferred; **§11 must stay deferred for this
figure to remain true**, and an IMPL that absorbs any of them re-opens the gate.

### 10.5 Anticipated count deltas — **+0 ON EVERY AXIS**

fixtures **121** · narrowed blank imports **121** (⚠️ **the number is right only with its scope stated** —
repo-wide reads **122**, the extra being `0018-http-rbac/inputs/driver.go:105`, a `pki` sub-package import
that is **not** a fixture registration; the unnarrowed `grep -cP '^\t_ "'` reads **123** and is REFUTED) ·
BackendKind tail **38** · new ports **none** · `go.mod` **+0** · config fields **+0** · fuzzers **55/48** ·
stat surface **+0** · `0120` **STAYS UNCONSUMED** · **both `0004` YAMLs BYTE-UNTOUCHED** (§3) ·
phase dirs **131** — a PLAN adds no directory.

⚠️ **`want` (ROADMAP **122**) is a ROADMAP DATA-ROW count, entirely unrelated to the fixture count.
NEVER conflate them.**

⚠️ **THE THREE REGISTRATION GATES ARE ALL ALREADY SATISFIED — `0004` is extended IN PLACE.** Gate 1
(`discoverFixtures`, `runner_test.go:1462-1499`) already matches; gate 2 (blank import,
`runner_test.go:30`) already present; gate 3 (`BackendKind` + runner `case`) unchanged at
`HTTPSH2 = 2`. ⚠️ **Gate 2's failure mode is a SILENT PASS** — `runner_test.go:200` `t.Skipf`s an
unregistered fixture and **no fixture-count gate exists anywhere**
(`git grep -c 'len(fixture.DriverRegistry)\|fixtureCount' -- 'test/**/*.go'` ⇒ **rc=1**, reproduced). It
cannot bite this row, but it is why the IMPL must assert the fixture **ran**, not merely that the suite
was green.

---

## 11. THE DEFERRED FOLLOW-ONS — WRITTEN AS ROWS, NOT GESTURES (SPEC §13 item 8)

Each carries its charter, its measured spade-work, and **the decision it must OPEN with** — so the next
reader does not re-derive what is already measured, and does not mistake a priced deferral for an oversight.

**(i) Arm C — the authority VALIDITY reject.** *Charter:* reject an invalid downstream `:authority` (or
`host`) on the H/2 leg as the reference does. *Inherits, all measured:* the nine-arm partition; the rule is
**`isValidAuthority`, NEVER `!= ""`** — the reference tears down on whitespace-only, embedded-space and
userinfo authorities, **and on a perfectly valid `:authority` beside an EMPTY regular `host`**, so the two
fields are validated **INDEPENDENTLY**; two candidate sites with scopes and costs — **+32/−0** stream-scoped
in `buildRequest` (whose error reaches only `writeRSTStream`, so **STREAM scope, ALWAYS**) versus
**+31/−0 over TWO files** connection-scoped at `(*serverStream).recvHeaders` → the `hErr.Stream == 0`
discriminator, **which emits a GOAWAY the reference never sends**; the reaction is **CONFIG-DEPENDENT**
(the default emits **ZERO bytes** — no GOAWAY, no RST_STREAM, not even server SETTINGS, a clean FIN;
`stream_error_on_invalid_http_messaging: true` makes it a survivable `RST_STREAM(PROTOCOL_ERROR)`);
`http2.rx_messaging_error` is the only classifier surviving **both** postures, and ⚠️ **NEITHER reference
stat EXISTS in the subject** (`downstream_cx_protocol_error` resolves to `internal/filter/network/redisproxy`
ALONE), so stat parity needs a NEW stat and breaks a `+0` posture. ⚠️ **THAT ROW OPENS WITH A *SHAPE*
DECISION, NOT A COST DECISION** — neither available shape is byte-parity with the reference default.
⚠️ **And it now also owns D-90-DUP (§2.1)**: the duplicate-`:authority` reject this row declined to add.

**(ii) H1-B′ — HTTP/1.1 with NO `Host`.** *Measured:* reference **400**, subject **200** forwarding an
empty `Host:`. *In charter but a different codec:* the fix site is `connection.go::dispatchRequest`, a
different file from this row's mechanism. ⚠️ **NEW at the SPEC and absent from the BRAINSTORM entirely** —
the record's "H1-B" was **MIS-ATTRIBUTED**: HTTP/1.0 **with** a valid `Host` still returns **426**, so the
426 is the **HTTP/1.0 VERSION**, not the missing `Host` (stat-distinguishable too: 426 books
`downstream_cx_protocol_error`, the 400 books only `downstream_rq_4xx`). *Opens with:* nothing structural —
it drops into `TestH1Robustness_KnownDivergencesFromEnvoy` (`connection_robustness_test.go:143`) with zero
new machinery.

**(iii) H1-D — duplicate `Host`. A NAMED DEPARTURE, stated affirmatively.** The subject returns **400**;
the reference comma-coalesces in wire order and returns **200**. ⚠️ **envoy-go is the RFC 7230 §5.4-
conformant side, and parity would make it STOP rejecting something it correctly rejects.** There is nothing
in envoy-go to relax — the 400 originates in `$GOROOT/src/net/http/request.go:1139`
`"too many Host headers"`, a string with **zero** occurrences in this repo — so parity means **defeating
the stdlib parser** by buffering and rewriting the request head, measured at **+76/−1**, roughly **5× the
entire in-scope fix**. ⚠️ **That rewrite is a request-smuggling-surface change and the existing suite is
PROVABLY BLIND to it: all 12 `TestH1Robustness` rows pass untouched with it installed.** ⚠️ *Structural gap
to price if ever taken:* `connection_robustness_test.go` has a **conformant-rejections** group (`:85`) and a
**known-divergences** group (`:143`) and **NO "subject stricter" group** — H1-D fits neither.
**Precedent for naming rather than fixing: `BEHAVIOR_CONTRACT.md:698`'s existing *"Known departure (H1)"*.**

**(iv) The D-89-HOST residue — `h2ReconcileSkipKey`.** D-90-SKIP leaves the SKIP **untouched** on three
measured grounds (the reference's filter-write fold has **THREE** rules — `replace` ⇒ overwrite, `add` ⇒
**comma-join**, ⚠️ `remove` ⇒ **DELETES the authority entirely and forwards with NO authority at all, 200** —
and `reconcileH2DecodeDelta` has **no scalar write-back path** for any of them, returning only
`[]hpack.HeaderField` and never touching `h2req.Authority`; the reference's empty-authority reject is
**DECODE-ONLY**, so it cannot be extended to filters by a parity argument; and it is out of charter).
⚠️ **BUT D-89-HOST's GROUND 2 — *"two contradictory authorities on one request"* — IS RETIRED BY THIS ROW**,
because after promotion there is only ever one. *Opens with:* option (b) is **newly defensible at row-done**;
carry the replace/add/**remove** table so a later reader does not re-derive *"SKIP is still justified"* from
a rationale row 90 has already invalidated.

**(v) The H3 arm-A prediction — READ, NEVER MEASURED.** From the pinned `quic-go@v0.54.1`
`http3/headers.go`: `parseHeaders` (`:165`) already rejects an empty or absent `:authority` and `:209` sets
`http.Request.Host = hdr.Authority`, so **arm B CANNOT occur on H/3**; but a regular `host` falls through
the `default:` arm un-suppressed, so **arm A is READ-PREDICTED to reproduce there.** ⚠️ **ZERO PROBES WERE
RUN, at the SPEC or at this PLAN. Nothing here is a measurement.** H/3 has an **emptiness** reject but **no
validity** reject, so arm C's C3/C5/C12 shapes would be *accepted* there. *Opens with:* a probe, not a fix.

**(vi) The access-log / Zipkin-span assertion axis.** ⚠️ **NEW AT THIS PLAN.** §7.1 rejects it as this
row's differential target on a measurement the SPEC did not take — **not one of the four H2-capable
fixtures (`0004`, `0079`, `0080`, `0119`) carries an access-log or tracing surface**, so buying it means
minting that surface plus a log-collection path. But it **is** the row's most user-visible consequence:
`emitAccessLogH2` sources `Authority` from **`req.Authority`** (`accesslog_emit.go:116/:131`) while the H1
and H3 legs source it from `r.Host` (`:55/:70/:177/:192`), so **only the H/2 leg logs an empty authority
for a host-only request** — flipping `%REQ(:AUTHORITY)%` from `-` to the host
(`accesslog/format.go:53`) and the **Zipkin span NAME** from empty to the host (`zipkin.go:99`,
`Name: s.Authority`). *Opens with:* which fixture pays for the surface.

---

## 12. EXPLICITLY NOT MEASURED AT THIS PLAN — stated so it is never inferred

- **No differential fixture was built or executed at this stage.** The arm roster, the break protocol and
  the backend edit of §7 are **DESIGNED FROM READ CODE**, not from a run. The instrument, the paths, the
  route fall-through and the `[3,3,3]` scope were verified by reading and grepping; **no `0004` run of any
  kind was launched**, and **no Docker container was started**.
- **The reference side was NOT re-probed at this stage.** Every reference behaviour cited (the nine-arm
  validity partition, the three filter-write fold rules, the H1 status matrix, arm C's zero-byte teardown)
  is **INHERITED FROM THE SPEC'S RUNS**, not re-measured here.
- **`-race` was run on nothing.**
- **The H3 leg — ZERO probes**, at the SPEC and here (§11 (v)).
- **The encode/response direction and upstream-side authority handling.**
- **Downstream TLS/ALPN H/2 for the arms themselves** — the SPEC's arms all used **h2c prior knowledge**,
  while `0004` is **TLS+ALPN-h2**. §7.3 requires the ALPN assertion for exactly this reason: the instrument
  is being moved to a transport it was not exercised on.
- **The exact charset the reference accepts in an authority** — the nine arms partition it; they do not
  characterise the predicate.
- **Whether a conformant peer accepts the authority-less request the reference forwards after a filter
  `remove("host")`.**
- **Whether the reference emits a GOAWAY before closing on arm C** — a plain EOF was seen at the SPEC; no
  packet capture was taken, here or there.

---

## 13. NEXT

**BOOTSTRAP §5 state 3 ⇒ the phase-90 IMPL** (`superpowers:executing-plans` or
`superpowers:subagent-driven-development`), TDD per task, **ONE atomic commit**, `PROGRESS.md` appended per
task. The IMPL flips row 90 `done`, completes **ADR-0312** (guard **1 -> 0**), lands the `ROADMAP.md`
row-90 text correction (§8.1) and the `BEHAVIOR_CONTRACT.md:2034` rider (§8.2), and registers the **six**
follow-ons of §11 as named rows.

⚠️ **AND ITS FIRST JOB IS TO REFUTE THIS PLAN BY EXECUTION** — as this PLAN refuted the SPEC on five
counts and one measurement agent on a sixth, and as the SPEC refuted the BRAINSTORM on six.
