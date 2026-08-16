# PLAN 89 — `h2-decode-filter-mutations`

**Base master `733f9830`. Stage branch `phase-89-plan`. Date 2026-08-16. Lifecycle-state 2 → 3.**

`ROADMAP.md` is **BYTE-UNTOUCHED** at this stage (proved by an EMPTY diff against master, §1). Row 89 stays `in-progress`; sentinel `want` stays **121**. `DECISIONS.md` and `BEHAVIOR_CONTRACT.md` are BYTE-UNTOUCHED too — the strict `^> **STATUS: PROPOSED` guard **STAYS ARMED at 1**; the phase-89 IMPL disarms it. This PLAN adds **no ADR**.

Method: five probe agents on five disjoint detached worktrees (`wt-89-a`…`wt-89-e`), disjoint port bands 47800-47879, private scratch each; **none committed**, each proved its tree clean. The controller ran the sentinel battery, the count battery, and **re-derived every load-bearing agent claim by execution** — which produced two further refutations the agents had not made, and confirmed one that corrects a STANDING METHOD NOTE.

---

## 0. THE HEADLINE: THIS PLAN REFUTES ITS OWN SPEC ON THREE DECISIONS, AND ONE OF THEM CHANGES THE ALGORITHM

| # | SPEC position | verdict at this tip |
|---|---|---|
| 1 | §13.2 item 3: duplicate-name collapse "to the first" is *"deliberate, but a wire-order divergence that needs a reference measurement before it is contracted"* | ⚠️ **MEASURED AND REFUTED.** Reference Envoy is an **ordered multimap**: a *replaced* name is **removed everywhere and ONE copy appended at the TAIL**; *untouched* duplicates stay at their original non-adjacent positions; nothing is ever comma-joined or collapsed to position 0. **Both prototypes built this stage rewrite at the first occurrence and therefore diverge.** §2.3 |
| 2 | §8.1: *"The exact substring the IMPL removes: `, H2 differential coverage.`"* with a **zero line delta** as the gate | ⚠️ **REFUTED AS A UNIQUE ANCHOR.** The substring occurs **THREE** times — `BEHAVIOR_CONTRACT.md:32` (`header_mutation`, this row's), `:33` (`local_ratelimit`), `:34` (`csrf`). A global edit closes two carve-outs this row does not own, **and the line delta is 0 either way**, so the SPEC's stated gate is blind to it. §7.1 |
| 3 | §4.4: `emitAccessLogH2` takes the H2Request by value at **five** call sites and scans `req.Headers` **twice** | ⚠️ **BOTH NUMBERS REFUTED. SIX call sites; THREE scans.** The third is `reqHeaderLookupH2` (`accesslog_emit.go:230`), feeding tracing `custom_tags` and `request_headers_to_log` — so the fix changes **those** outputs too. §2.5 |

And the row got **larger, not smaller**: the outbound-validation hazard the SPEC flagged as one item is **four** (§2.2), and against a conformant peer it is not a silent spec violation but a **client-visible 400 with ZERO backend delivery**.

---

## 1. SENTINEL — RUN MECHANICALLY AT THIS TIP, ACTUAL OUTPUT RECORDED

⚠️ **A PLAN edits no ROADMAP row. The binding proof is an EMPTY `ROADMAP.md` diff against master** — `git diff master -- docs/envoy-go/ROADMAP.md | wc -c` returned **0** at stage start and is re-asserted at close. `want` stays **121**; row 89 stays `in-progress` at `:151`.

Input **239 lines / 121 data rows**:

| check | ACTUAL output |
|---|---|
| (1) `want=121` | **`NOT DONE: row 89`** ALONE, denominator SILENT |
| (2) union form | **SIX** at `:199 :205 :211 :221 :227 :235` |
| (3) | **SILENT** |

⇒ the conjunction FAILS; **the sentinel does NOT fire; `stop` was NOT created** (`ls stop` → *No such file or directory*, verified at the git root).

**ALL FOUR NCs FIRED:**
- **row-62 doctoring** — `NC LANDED? [ in-progress ]` INSPECTED FIRST, then `NOT DONE: row 62` **AND** `NOT DONE: row 89`.
- **denominator** — `want=120` on the real file gave `NOT DONE: row 89` **plus** `GATE FAIL: examined 121 data rows, expected 120`.
- **check-(3) doctoring** — residual `gRPC-family row` **2 → 0** confirmed on the doctored copy FIRST, then `NEVER OPENED: gRPC` fired ALONE with WASM correctly silent.
- **check-(2) one-arm** — long **5** / short **1** / union **6**. A one-arm strip is NOT an NC for the union.

**Leak axes:** `-family row` **95 occurrences / 67 LINES** · `gRPC-family row` **2** · `Operational-tooling-family row` **3**. **Row 89 is WELL-FORMED: `fields=8`, control row 88 also `fields=8`.**

⚠️ Do **not** "fix" the six and do **not** forecast a decrease — history `0 → 1 → 3 → 4 → 5 → 6` across ~40 phases.

---

## 2. THE SEVEN DECISIONS THIS PLAN FREEZES

The SPEC froze **D-89-SHAPE / SITE / PSEUDO / PROOF / DOC / BODY / STAT**. Those are not re-opened. This PLAN freezes the seven that were left open or that execution moved.

### 2.1 **D-89-HOST: SKIP `host` alongside the `:` prefix.** (SPEC §5.4 / §17 item 4 — was NOT MEASURED)

Measured on **reference `envoyproxy/envoy:contrib-v1.37.2`**, downstream H2 → upstream H2, raw-framer backend:

| arm | upstream `:authority` | upstream regular `host` |
|---|---|---|
| passthrough | `orig.example` | **absent** |
| lua `replace("host","mutated.example")` | **`mutated.example`** | **absent** |
| lua `add("host","added.example")` | **`orig.example,added.example`** — comma-joined into the SAME entry | **absent** |
| lua `replace(":authority", …)` | `mutated-auth.example` | **absent** |
| inbound `:authority` **and** `host` | `auth.example` | **the regular `host` is DROPPED** |
| inbound `host` only, no `:authority` | `hostonly.example` — **PROMOTED** | **absent** |

⇒ **the reference NORMALIZES: `host` and `:authority` are one and the same entry.** A regular `host` field appeared on the H/2 upstream in **0 of 15** reference readings; the `add("host")` comma-join is the clincher — that is singleton/inline-header behaviour, not a second field.

Measured on the **subject at this tip**, with a positive control (`envoy_on_response` adds `x-lua-ran: yes`, so "the filter ran" is proven independently of whether the request mutation landed):
- ds H1 + lua `replace("host")` → up H1: upstream head **UNCHANGED** (`Host: orig.example`) while the response carries `X-Lua-Ran: yes`. **envoy-go's own H1 leg silently ignores a Lua `host` replace, and it is not because the filter didn't run.**
- ds H2 + lua `replace("host")` → up H2: byte-identical to passthrough, `x-lua-ran: yes` present. The row's defect, reproduced independently.

**DECISION: SKIP.** Three grounds, all measured:
1. **H1 parity — the charter's own target — says skip.** Skipping reproduces envoy-go's H1 behaviour exactly.
2. **Projecting emits a shape the reference produces in ZERO of 15 readings**, and it would be *self*-inconsistent: envoy-go's `:authority` comes from the frozen scalar, so a projected `host: mutated.example` would ride next to `:authority: orig.example` — **two contradictory authorities on one request**. Legal HTTP/2, but it changes vhost decisions at the peer.
3. The reference-exact fix is to make `host` an **alias of the authority scalar on BOTH legs** (write-back into `req.Host` / the H2 `:authority`). That is strictly larger than the delta reconciler and touches the H1 leg. **Out of scope at `want=121`.**

⚠️ **The residual divergence must be WRITTEN DOWN, not left implicit** (§7.2): a Lua `host` replace stays a silent no-op on H2 *and* H1. It **already exists at the tip on the H1 leg**, so this row neither creates nor widens it — but without the sentence the row's "decode mutations now reach the H/2 upstream" claim ships **over-broad**.

### 2.2 **D-89-VALIDATE: the outbound RFC 9113 §8.2.2 validation is IN, and it must be VALUE-AWARE.** (SPEC §17 item 3)

⚠️ **THE FIX INTRODUCES THIS HAZARD; IT DOES NOT INHERIT IT.** The phase-88 shape repeating.

Reachability, controller-re-derived. `internal/filter/http/header_mutation/header_mutation.go:177-182`, verbatim:
```go
func isProtectedHeader(name string) bool {
	if strings.HasPrefix(name, ":") { return true }
	return strings.EqualFold(name, "host")
}
```
**Exactly two rules.** `connection`, `transfer-encoding`, `keep-alive`, `proxy-connection`, `upgrade` and `te` are **NOT protected** — and the predicate gates both Append (`:153`) and Remove (`:140`), listener-level and per-route (`:201`). Configs carrying all of them boot: `-mode validate -allow-h2c` → **rc=0, `configuration OK`**.

Wire-measured against a **raw-framer** backend (mandatory — a `net/http`+`h2c` backend normalizes case, hides pseudo-headers and destroys order), with `n>0` asserted before interpretation, `ss -ltnp` asserted, and qualified symbol counts asserted (`hcm.reconcileH2DecodeDelta`: tip **0**, prototype **2**):

| arm | TIP | DELTA prototype |
|---|---|---|
| control, no mutations | `nfields=7` | **byte-identical to tip** — no regression on the no-mutation path |
| `connection: close` + `transfer-encoding: chunked` | `nfields=7`, neither reaches upstream — **this IS the phase-89 bug, and it is exactly why the hazard does not exist at the tip** | `nfields=9`, `[7] connection=close`, `[8] transfer-encoding=chunked` |
| `te: gzip` | `nfields=7` | `nfields=8`, `[7] te=gzip` — ⚠️ **A THIRD LEAK THE SPEC DID NOT RECORD** |
| counterfactual `nolower` binary | — | `[7] "X-Mixed-Case"`, `[8] "X-Upper"` — ⚠️ **UPPERCASE ON THE WIRE** |
| leak arm vs a **conformant** `h2c.NewHandler` peer | control 200 / backend count **1** | **`400`**, body `request header "Connection" is not valid in HTTP/2`, backend count **0** |

**Four findings, three of them new:**
1. **The leak SET is incomplete in the SPEC.** `te` with a non-`trailers` value also leaks — and **a name-only guard structurally cannot catch it**, because `te` is deliberately excluded from `isConnectionSpecificField` (it is value-conditional; the function's own doc says so). ⇒ **the outbound guard MUST be value-aware.**
2. **A FOURTH hazard the SPEC did not name: header-name CASE.** `header_mutation` canonicalizes config keys (`http.CanonicalHeaderKey` at `:145`/`:158`), so `connection` becomes `Connection`. And **nothing below the reconciler lowercases** — controller-confirmed at the emit site, `internal/filter/hcm/h2/client.go`: `headers = append(headers, req.Headers...)`, verbatim, no transformation. ⇒ **the reconciler's own `strings.ToLower` is load-bearing and owes its own break arm.**
3. **"Reachable" understates it.** Against a conformant peer this is a **client-visible 400 with zero backend delivery** — a production outage shape.
4. `keep-alive` / `proxy-connection` / `upgrade` were **config-validated but NOT wire-measured**; they are structurally identical to the two that were. **Stated as INFERRED, not measured.**

**DECISION — three parts:**
- **(a) EXPORT, do not duplicate.** `isConnectionSpecificField` (`h2/stream.go:370`) and `teTrailersValue` (`:355`) are unexported in package `h2`; `h2dispatch.go` is package `hcm`. The function's own doc comment calls itself *"the ONE source of truth for the RFC list … so a member can never be dropped from one copy and survive in the other."* **A duplicate would violate that invariant in the append-only contract.** Its only two callers today are both inside package `h2` (`stream.go:445`, `client.go:795`).
- **(b) The exported symbol is VALUE-AWARE:**
  ```go
  // IsIllegalH2RequestHeader reports whether a (name, value) pair may NOT appear
  // on an HTTP/2 request message per RFC 9113 §8.2.2. name MUST already be
  // lowercase (H/2 wire form).
  //
  // Exported so the OUTBOUND path — package hcm's decode-filter reconciler,
  // which re-emits filter-written header names onto the upstream HEADERS block
  // — can consult the SAME list buildRequest enforces on the inbound path,
  // without duplicating it.
  func IsIllegalH2RequestHeader(name, value string) bool {
  	if isConnectionSpecificField(name) {
  		return true
  	}
  	return name == "te" && value != teTrailersValue
  }
  ```
  Zero import churn — `h2dispatch.go` already imports the package unaliased.
- **(c) DROP, do not reject.** Rejecting turns a config-legal mutation into a client-facing 5xx. Dropping matches what the tip already does with these mutations (nothing reaches upstream), so it is the strictly smaller behavioural change.

**MEASURED cost of the validation: +22 lines across ONE additional file** (`+16` `h2/stream.go`, `+6` `h2dispatch.go` — two three-line guards), on top of the reconciler.

**Over-firing control, executed** (`reference_positive_arm_cannot_catch_overfiring`): a STACKED arm — `connection: close` + `x-benign: kept` + `te: trailers` + remove `user-agent` — yields illegal DROPPED, **legal `te: trailers` KEPT** (a name-only guard would have wrongly dropped it), benign KEPT, `user-agent` REMOVED, order preserved, **no duplication**. Arms re-run on the validated binary: leaks gone, the conformant-peer arm back to **200 with backend count 1**.

### 2.3 ⚠️ **D-89-DUP: the reference's rule is TAIL-APPEND, NOT first-position collapse. THE SPEC'S PROPOSED SEMANTICS ARE REFUTED.** (SPEC §17 item 7 — the SPEC asked for a measurement *or* a written deferral; it is MEASURED)

Input on every arm, exact wire order: `x-dup: one`, `x-mid: mid`, `x-dup: three`. Driven with a raw `x/net/http2` framer **client** so wire order is controlled exactly (a stock `net/http` client reorders and joins).

**REFERENCE, downstream H2 → upstream H2:**

| arm | upstream result |
|---|---|
| no filter (pure proxy) | `[04] x-dup=one`, `[05] x-mid=mid`, `[06] x-dup=three` — **both non-adjacent positions preserved; no comma-join, no collapse** |
| `header_mutation` append an **unrelated** name | `x-dup` still at `[04]` and `[06]`; the new field near the tail — **a mutation does not change collapse behaviour** |
| `header_mutation` APPEND to `x-dup` itself | originals kept at `[04]`/`[06]`; a **third** `x-dup` appended at `[09]` |
| **lua `replace("x-dup","REPL")`** | ⚠️ **both originals REMOVED; ONE `x-dup="REPL"` appended at the TAIL `[07]`. Position 0 is NOT preserved.** |
| lua `add("x-dup","ADDED")` | originals kept; third appended at the tail |

Same preservation measured on the H2→H1, H1→H2 and H1→H1 legs.

⇒ **the reference model is an ORDERED MULTIMAP. `replace` = remove-every-occurrence + append-one-at-TAIL. `add`/`append` = append-at-tail. Untouched names never move. It NEVER collapses to the first position.**

**SUBJECT at the tip:** the H2 leg's **passthrough ordering already matches the reference exactly** (`[04] x-dup=one`, `[05] x-mid=mid`, `[06] x-dup=three`), and the lua `replace` arm is byte-identical to passthrough with `x-lua-ran: yes` proving the filter ran — the row's defect again.

⚠️ **AND A TRAP FOR ANY "USE H1 AS THE ORDERING ANCHOR" INSTINCT:** the subject's **H1 leg groups duplicates adjacently and emits names SORTED** (`Host` / `User-Agent` / `X-Dup: one` / `X-Dup: three` / `X-Mid: mid`) — Go `net/http` map semantics. **The H1 leg is already lossy on exactly this input and is NOT a clean ordering parity anchor.**

**DECISION: adopt the reference's rule.** For every name whose value list the decode chain **added or changed**: remove **every** occurrence from the carrier, then append **one field per value at the TAIL**. For a **removed** name: remove every occurrence. **Untouched names keep their exact positions.**

Two arguments beyond reference fidelity:
- It makes the H2 leg **reference-exact on this input**, because the passthrough ordering already matches.
- ⚠️ **The repo's own helper already implements it.** `upsertH2Header` (`h2dispatch.go:675-683`) drops every case-insensitive match and then appends at the end — i.e. an upsert of an existing header **MOVES IT TO THE TAIL**. The reconciler must use the **same semantics**, differing only in that it builds a **FRESH** slice instead of `upsertH2Header`'s `fields[:0]` in-place compaction.

⚠️ **Both prototypes built this stage rewrite the changed name at its FIRST occurrence.** That is the one algorithmic correction the IMPL must carry over the measured prototypes, and it is why the +162 figure is a floor on the *behaviour* axis and not only on the *scope* axis.

**Deterministic append order for MULTIPLE added names is a NAMED DEFERRAL.** Map iteration is nondeterministic, so the added-name set must be **sorted by name** before appending. Whether the reference's multi-add order matches a sort is **NOT MEASURED**; a sort is deterministic and cross-side-stable, which is what the differential needs. Recorded, not claimed.

### 2.4 **D-89-CASE: emit lowercase, and gate it.** New at this stage; see §2.2 finding 2. The reconciler lowercases every name it emits; the break arm that removes the `strings.ToLower` must redden a named fixture arm, not only a unit row.

### 2.5 **D-89-SECOND-ORDER: the behaviour change is stated, not discovered.** (SPEC §4.4, both numbers refuted)

Controller-measured:
```
$ git grep -n 'emitAccessLogH2(' -- 'internal/**/*.go' | grep -v '_test\.go' | grep -v 'func (f \*Filter)'
h2dispatch.go:318  :401  :542  :609  :616  :645        COUNT: 6
```
**SIX call sites, not five** (the `:609`/`:616` pair straddles the `ReconcileOrderedHeaders` branch and reads as one by eye). And there are **THREE** `req.Headers` scans, not two:
- `accesslog_emit.go:99` — `x-client-trace-id`
- `accesslog_emit.go:265-267` — `h2UserAgent`, used at `:109` (span `UserAgent`) and `:132` (`accesslog.Record.UserAgent`)
- ⚠️ `accesslog_emit.go:230` — **`reqHeaderLookupH2`**, passed at `:118` (tracing `ResolveCustomTags`) and `:135` (`captureRecordHeaders`, i.e. **`request_headers_to_log`**)

⇒ **the fix changes access-log content, span content, `request_headers_to_log` output AND tracing `custom_tag` resolution.** That is arguably correct — those should reflect what was sent — but it is a behaviour change the PLAN **states** rather than discovers. It is contract-visible and belongs in §7.2.

### 2.6 **D-89-DOC-ANCHOR: the contract edit is anchored on the ROW, not the substring, and the gate is a RESIDUAL COUNT.** See §7.1.

### 2.7 **D-89-INVENTORY: the slice-only-writer inventory ships with a machine-checkable gate and a NAMED blind axis.** See §3.

---

## 3. THE SLICE-ONLY-WRITER INVENTORY — a NAMED DELIVERABLE (SPEC §17 item 2 / §14.1)

**FIVE production writers to the H2 request slice. ZERO of them writes the decode map.**

| # | file:line | enclosing func | writes | writes the map too? |
|---|---|---|---|---|
| 1 | `h2/stream.go:345` | `buildH2Request` | `out.Headers = append(out.Headers, h)` — every non-pseudo inbound field, in wire order | **NO** |
| 2 | `h2dispatch.go:438` | `(*chainDispatchAction).WriteH2` | `x-request-id` | **NO** |
| 3 | `h2dispatch.go:443` | same | `x-b3-traceid` / `x-b3-spanid` / `x-b3-parentspanid` / `x-b3-sampled` (Zipkin arm) | **NO** |
| 4 | `h2dispatch.go:448` | same | `traceparent` (OTel arm) | **NO** |
| 5 | `h2dispatch.go:450` | same | `tracestate` (OTel arm, non-empty only) | **NO** |

**The exact mirror — map writers on the H2 path, none of which writes the slice:** `h2dispatch.go:472` `:method` · `:484` `:authority` · `:498` `:path`. Guarded ONLY by an idempotence check plus a nil/empty-source check — **no config gate**. ⚠️ Note `:authority`/`:path` *do* carry source guards (`c.req.Host != ""`, `c.req.URL != nil`), so "unconditional" is true in practice but **not structurally** — which is precisely why the skip must be a **`:`-byte test on the key**, never a three-name enumeration.

Supporting facts, each measured: `SetH2Request` has exactly **one** production call site (`:457`) · the router **never writes** the slice (every `.Headers` reference in `internal/filter/http/router/*.go` is a read) · `*H2Request` appears **0** times in production `internal/`, so no pointer escape exists today.

The tracing-block measurement reproduced at this tip: the SPEC's literal `NR>=422 && NR<=456` gives `c.req` count **0**; the range re-derived **by symbol** as `422-455` (from `if c.f.tracingConfig != nil {` to its closing brace) gives **0** in both the line form and the occurrence form, against a whole-file positive control of **13**.

### 3.1 The gate, with its NC and its blind axis

```sh
git grep -nE '\.Headers[[:space:]]*=[^=]' -- 'internal/filter/hcm/*.go' 'internal/filter/hcm/h2/*.go' \
  | grep -v '_test\.go' | grep -v 'resp\.Headers' | wc -l
```
**Reads `5` at this tip** — non-zero, so the selector is not silently matching nothing (`reference_gate_selector_matched_nothing`), and the match **SET** is exactly the five rows above (verified by printing the list, not only the count).

**NEGATIVE CONTROL, executed on scratch copies — the tracked file was never patched:**

| arm | reading |
|---|---|
| clean tree | **5** |
| faithful scratch copy | **5** |
| copy + a fake `h2req.Headers = append(...)` slice-only writer | **6** ✅ FIRES |
| copy + a `func stampVendorHeader(r *h2.H2Request)` helper | **6** ✅ FIRES |
| copy + an **element in-place** writer `h2req.Headers[0].Value = …` | **6 (unchanged)** ⚠️ **BLIND** |

**Companion counter** (usable ONLY paired with the non-zero primary — a zero-reading gate is fail-unsafe on its own):
`grep -rnE '\*(h2\.)?H2Request' --include='*.go' internal/ | grep -v _test | wc -l` must stay **0**; NC moved it **0 → 1**.

⚠️ **NAMED BLIND AXES, stated rather than papered over:** (i) element-level in-place mutation (`.Headers[i].Value = …`) — the selector needs `=` immediately after `.Headers` and `[` intervenes; (ii) a writer added in a package other than `internal/filter/hcm/`. Neither exists today.

### 3.2 The IMPL adds the SIXTH writer, deliberately

The reconciler's `h2req.Headers = merged` is a sixth match. **The gate's expected reading at row-done is `6`, and the IMPL must state that** — a gate whose number silently moves is not a gate.

---

## 4. THE ORDERED IMPL TASKS (TDD; the RED census is observed BEFORE any production edit)

⚠️ **The tracing pin is IN the RED set** (SPEC §17 item 1) — but see §5.1: it is **split across two files**, and a plan that edits only the zipkin file touches half of it.

**T1 — RED census. Tests only; ZERO production bytes.**
Every T1 test compiles against the unmodified tip and asserts **behaviour through `WriteH2`**, so nothing references a symbol that does not yet exist and the RED is a clean assertion failure rather than a build break.
- T1a `internal/filter/hcm/h2dispatch_reconcile_test.go` (**NEW**): the `headerMutatingFilter` double + the count-and-set helpers + the reconciler table (§5.2).
- T1b the tracing pin, extended in **BOTH** `connection_test.go` and `tracing_zipkin_dispatch_test.go` (§5.1).
- T1c the three early-exit arms (§5.3).
- T1d the `SetH2Request` re-issue arm, placed in `hcm` (§5.4).
- **Observe and RECORD the RED census with denominators**, against the green baseline of §9.2. A liveness break needs a failing baseline (`reference_liveness_break_needs_failing_baseline`).

**T2 — production: export the validator.** `internal/filter/hcm/h2/stream.go` gains `IsIllegalH2RequestHeader` (§2.2b). Nothing else in that file moves.

**T3 — production: the reconciler.** `internal/filter/hcm/h2dispatch.go`:
1. a deep-copy snapshot of `c.req.Header` taken **immediately before** `chain.RunDecodeHeaders` (`:518`);
2. the delta apply placed **after** the `if hasBody { … RunDecodeData … }` block (`:551-555`) and **before** `rf.RunAction(ctx)` (`:563`) — after `RunDecodeData`, per SPEC §4.2's corrected three-exit enumeration;
3. a **re-issued** `rf.SetH2Request(h2req)`, with the existing call at `:457` **NOT moved** (SPEC §4.3: `RunAction`'s H2 arm has no guard);
4. a **FRESH** output slice — **never** the `fields[:0]` idiom;
5. `:`-prefixed keys and `host` skipped (§2.1);
6. names emitted **lowercase** (§2.4);
7. changed/added names removed-everywhere then appended at the **TAIL**, added names **sorted** (§2.3);
8. `h2.IsIllegalH2RequestHeader(name, value)` consulted per emitted value, **dropping** on true (§2.2c);
9. an early `return` leaving the carrier byte-stable when the delta is empty, so the no-filter path is unchanged.

**T4 — GREEN census** + the §3.1 gate re-read at **6** + the §3.2 statement.

**T5 — the `0004` differential extension** (§6).

**T6 — the break protocol, seven arms, seven distinct injection sites** (§6.3).

**T7 — docs** (§7): ADR-0311 completed (guard **1 → 0**), the scoped `BEHAVIOR_CONTRACT.md` edit, the ROADMAP row flip, `PROGRESS.md`, `STATE.md`, `STATE_HISTORY.md`.

**T8 — gates, run LAST against the FINAL tree** (§9.1).

⚠️ **ONE ATOMIC IMPL COMMIT** — TDD held *inside* it, both censuses observed first, no sub-phase rows minted (`want` stays 121). The phase-88 precedent.

---

## 5. THE UNIT ROSTER (SPEC §17 item 6)

### 5.1 ⚠️ The tracing pin is SPLIT ACROSS TWO FILES — SPEC §6.4 is wrong

```
$ git grep -n 'func TestWriteH2_Tracing' -- '*_test.go'
connection_test.go:855               TestWriteH2_Tracing_SampledInjects
connection_test.go:883               TestWriteH2_Tracing_Continued
tracing_zipkin_dispatch_test.go:163  TestWriteH2_TracingZipkin_SampledInjectsB3
tracing_zipkin_dispatch_test.go:210  TestWriteH2_TracingZipkin_Continued
```
`-run TestWriteH2_Tracing` denominator **4**, **4/4 PASS** at the clean tip and **4/4 PASS** on both prototypes. SPEC §6.4 names only `tracing_zipkin_dispatch_test.go`, which holds **2 of 4**.

⚠️ **What these four rows DO pin, precisely — do not overstate it and do not understate it.** The SPEC MEASURED them flipping **4 of 4 to FAIL** on the rejected full-rebuild shape, with a green baseline and the denominator asserted, so **they already catch a whole-map projection**. That is the row's largest hazard and it IS covered.

What they do **not** cover is narrower and still owed: their chains are **router-only** (`mkFilterForTable`, `connection_test.go:48-68`), so **no decode filter runs in any of them**. They therefore cannot show that the tracing writes survive *alongside* a filter mutation — the exact interaction this row introduces. **The IMPL owes a fifth row** in each file's idiom: the same setup with a two-entry chain `[headerMutatingFilter, router]`, asserting that `x-request-id` / `x-b3-*` / `traceparent` are **still present** on `captured` *and* that the filter's own mutation landed. Additive coverage over a pin that already works, not a replacement for it.

### 5.2 The reconciler table — and the assertion primitive that is NOT sufficient

The observation seam, verified end to end: `c.WriteH2(...)` → `rf.SetH2Action(c.action)` (`:407`) → tracing block (`:422-455`) → `rf.SetH2Request` (`:457`) → pseudo injections (`:471-499`) → `RunDecodeHeaders` (`:518`) → `RunDecodeData` (`:552`) → `rf.RunAction` (`:563`) → `f.h2Action(ctx, f.h2Req)` (`router.go:313`) → the closure writes `*captured = req`.
⇒ **`captured` IS `rf.h2Req` at RunAction time.** There is **no exported accessor** for `h2Req`; `captureH2Action` (`connection_test.go:845-853`) is the only mechanism and it is sufficient.
⚠️ Tests must build `&chainDispatchAction{f: …, action: captureH2Action(&captured), req: hreq, routeIdx: 0}` as a **struct literal** — `disp.Match(req)` builds the action internally and cannot be given a capture closure. `c.req` must be non-nil (`WriteH2` writes `c.req.Header[":method"]` at `:471`).

⚠️ **`h2HeaderValue` (`connection_test.go:726-735`) returns only the FIRST case-insensitive match.** It therefore **cannot** detect the `[a, c, c]` duplication the `fields[:0]` idiom produces, and **cannot** distinguish "removed" from "still present later in the slice". Any removal arm written with it alone is **vacuous**. **The IMPL must add a count-and-set helper over `captured.Headers`** — e.g. `h2HeaderValues(req, name) []string` returning every match in wire order, plus `h2HeaderNames(req) []string`. Rows assert the **SET and the ORDER**, not a first-match value.

⚠️ **No existing test double mutates the decode map.** All four in-package `DecodeHeaders` stubs (`orderRecordingFilter` `chain_dispatch_test.go:26-61`, `integrationRecordingFilter`, `tlsStateCapturingFilter`, `encodeSignalFilter`) are read-only. `orderRecordingFilter` is the smallest complete `HTTPFilter` shape to clone; a **new** `headerMutatingFilter` (add / remove / value-change / multi-value / no-op, table-driven, mutating in `DecodeHeaders` and optionally in `DecodeData`) must be written.

**Rows (each a `t.Errorf` per property, never `t.Fatalf` mid-table — `reference_fatalf_makes_assertions_unreachable`):**

| # | row | asserts |
|---|---|---|
| 1 | no-op passthrough | `captured.Headers` **byte-identical** to the tip's — the delta's early return |
| 2 | add | new field present, **at the TAIL**, lowercase; every pre-existing field unmoved |
| 3 | remove | **zero** occurrences (via the count helper, not `h2HeaderValue`) |
| 4 | value change | old value gone, new value present, **at the TAIL** (§2.3), exactly one occurrence |
| 5 | multi-value | one field **per value**, all at the tail, in the map's value order |
| 6 | **duplicate-name, changed** | both originals gone; **exactly one** field at the **TAIL**. ⚠️ The row that fails a first-occurrence implementation |
| 7 | **duplicate-name, untouched** | both occurrences **at their original non-adjacent positions** |
| 8 | pseudo skip | no `:`-prefixed field in `captured.Headers`; the scalars unchanged |
| 9 | `host` skip (§2.1) | a mutated `host` produces **no** regular `host` field and does not change `Authority` |
| 10 | case | a canonical-MIME map key emits **lowercase** on the carrier |
| 11 | §8.2.2 drop | `connection`, `transfer-encoding`, **`te: gzip`** dropped; **`te: trailers` KEPT** (the over-firing control) |
| 12 | tracing survival | see §5.1 |
| 13 | order preservation | `h2HeaderNames(captured)` equals the expected sequence exactly |

### 5.3 The three early exits — testability enumerated from `internal/filter/http/chain.go`

`RunDecodeHeaders` non-nil-error paths (SendLocalReply is **not** one — `:361`/`:375` return `(false, nil)`):

| # | line | trigger | error |
|---|---|---|---|
| 1 | `:381-383` | `StopIteration`, then `parkDecode` loses to `ctx.Done()` | `ctx.Err()` |
| 2 | `:385-386` | `TerminateStream` | `filter_http.ErrStreamTerminatedByFilter` (`chain.go:62`) |
| 3 | `:387-388` | unknown status | `fmt.Errorf("chain: filter %q returned unknown FilterHeadersStatus %d", …)` |

**Exactly ONE test in the repo drives any of them** — `chain_test.go:2941-2952` subtest `decode_headers` — and it is `package filter_http` calling `c.RunDecodeHeaders` **directly, never through `WriteH2`**. ⇒ **the SPEC's "CODE-READ, NOT EXECUTED" HOLDS for the `WriteH2` seam**, and the arm is owed.

**RECIPE (cheapest, non-parking):** a stub returning `filter_http.TerminateStream` from `DecodeHeaders`, installed ahead of the router in a two-entry chain, driven through the struct-literal `chainDispatchAction`. Assert `errors.Is(err, filter_http.ErrStreamTerminatedByFilter)` **and** `captured == h2.H2Request{}` (the action never ran, so no reconcile is owed).
⚠️ **Do NOT use the ctx-cancel path** — it needs a goroutine plus a deadline and cannot distinguish "parked" from "returned late" without `chain_test.go:3000-3022`'s scaffolding.

`LocalReplyDone` (`h2dispatch.go:530`): **ZERO unit coverage today**; the existing HCM cors test drives `f.dispatchRequest` (the **H1** seam). Clone `localReplyFilter` (`internal/filter/http/chain_test.go:317-337`).

`RunDecodeData` error (`h2dispatch.go:552`): four non-nil-error paths (`chain.go:456,461,465,470`; the 413 overflow at `:443` returns `(false, nil)` and is **not** one). ⚠️ **Reachable through `WriteH2` ONLY when `hasBody` is true** (`:515`, `:551`) — **the arm MUST set `h2req.Body` non-empty or it is SILENTLY VACUOUS.**

### 5.4 The `SetH2Request` arm — ⚠️ THE TEMPLATE SPEC §6.4 ASSUMES DOES NOT EXIST

```
$ grep -ow 'Filter' internal/filter/http/router/router_test.go | wc -l   =>  1   (line 140, a COMMENT)
```
**No test in the `router` package ever constructs a `router.Filter` or calls `RunAction`.** And the SPEC's census control is worse than reported:

```
$ git grep -n -w 'SetAction' -- '*_test.go'
actions_test.go:19   // (the Action closure HCM dispatch injects via *Filter.SetAction) and asserts
router_test.go:140   // dispatch injects via *Filter.SetAction) against a loopback echo backend.
$ (non-comment lines)  =>  NONE
```
⚠️ **The `SetAction = 2` "positive control" is VACUOUS — both hits are comment prose.** Repo-wide there are **zero non-comment test call sites** for `SetAction`, `SetRequest`, `SetH2Action`, `SetH2Request` **and** `RunAction`. The SPEC's conclusion ("neither setter has coverage") is right, but its control does not discriminate. *(The `SetRequest` trap itself reproduces: `-w` reads **0**, the substring form surfaces `SetRequestCtx`; the `-w` reading is the true one.)*

**DECISION: place the arm in `hcm`, not `router`.** It observes the real seam (`captureH2Action`), reuses landed machinery, and avoids minting the first `Filter`-constructing test in a package that has never had one. The arm asserts that a mutation made **after** the `:457` Set still reaches `RunAction` — i.e. that the **re-issue** happened. Its break arm is B1 (§6.3).

⚠️ **`RunAction` is IDEMPOTENT** (`router.go:306`, `if f.actionRan { return }`) — a second call is a silent no-op, so the re-issued **Set** must land before the single `RunAction`, and there is exactly one on the H2 path.

### 5.5 `buildH2Request` has ZERO coverage — a characterization pin, NOT part of the RED census

```
$ git grep -l -w 'buildH2Request' -- '*_test.go'   =>  0 files
```
SPEC §6.4 calls `h2/stream_test.go` "the only file covering `buildH2Request`/`buildRequest`"; it covers **`buildRequest` only**. The pseudo-header **exclusion** contract (`stream.go:343-346`), on which the whole `:`-skip rests, is pinned by nothing.
⚠️ *(Even the positive control has an identifier collision: `git grep -l -w 'buildRequest' -- '*_test.go'` reads **6**, but three are `internal/filter/network/kafkabroker/*` — a different package's function. The h2-side control is **3**. Same trap class as `SetRequestCtx`.)*
**The IMPL adds a table test in `h2/stream_test.go`'s `minHeaders()` idiom.** ⚠️ It **PASSES at the tip** — it is a characterization pin, and the IMPL must **not** report it inside the RED census.

---

## 6. THE DIFFERENTIAL RECIPE — extend `0004-h2-routing` IN PLACE (SPEC §17 item 5)

### 6.1 Baselines and the yardstick, controller-re-derived

```
$ git show --numstat 4eab3f72 -- test/fixtures/0004-h2-routing/
11 2 README.md · 47 0 backends/main.go · 90 3 driver/driver.go · 24 2 expectations.yaml
= +172 / -7 over 4 files
```
Confirmed at this tip: `http_filters` is **`envoy.filters.http.router` ALONE on BOTH sides** (`envoy.yaml:71-74`, `envoy-go.yaml:70-73`); `header_mutation` count **0**; **tracing count 0** on both YAMLs (⇒ SPEC §6.4 confirmed, the tracing pin **must** be a unit test); downstream TLS+ALPN `["h2","http/1.1"]`, upstream TLS+ALPN `["h2"]`; the three registration gates already satisfied, blank import at `runner_test.go:30`.

**`--allow-h2c` is NOT passed** — `harness.go:252` and `:606` are both `exec.CommandContext(ctx, bin, "-c", cfgPath)` and nothing else; zero `allow-h2c` hits in the file. ⇒ **a downstream-H2 arm MUST use TLS+ALPN. `0004` already does.**

**Round-trips = 31**, counted by call site × literal loop bound: `driver.go:282` ×9, `:292` ×9, `:309` ×9, `:340` ×2, `:383` ×1, `:396` ×1. `ADR-0057` (`DECISIONS.md:2056`) still says **27** and is STALE by 4. ⚠️ `README.md:36`'s "The first 27 requests are unchanged from phase 05.2" is **CORRECT as a prefix statement — do NOT fix it**. ⚠️ `expectations.yaml:10` already says **31**, so only ADR-0057 is stale. *(The SPEC brief's `README.md:34` cite is off by two; the correct anchor is `:36`. Cite by string.)*

### 6.2 ⚠️ THREE `0004` TRAPS THE IMPL MUST HONOUR

1. **Both YAMLs say "documentation only" (line 3 of each) BUT ARE THE LIVE TEMPLATES.** `readYAML` (`driver.go:106`) reads the committed file, strips the leading comment block, and `renderBootstrap` substitutes placeholders. There is no third template inside the driver. ⇒ **the `header_mutation` block must be added to BOTH YAMLs.**
2. ⚠️ **`renderBootstrap` does POSITIONAL, FIRST-OCCURRENCE replacement:** `driver.go:188`, `yaml = strings.Replace(yaml, "port_value: 0", "port_value: "+p, 1)` inside a loop over `ports`. **Inserting anything containing `port_value: 0` above an existing occurrence silently reassigns ports.** `driver_test.go`'s `TestRenderBootstrap_Subject`/`_Reference` pin that ordering and redden first — **name that as a free guard and run it early**.
3. **`expectations.yaml` is PROSE, NOT machine-evaluated** (its own line 3, per ADR-0019). Editing it changes no gate. ⚠️ It also carries a **stale clause**: `:3-7` claims enforcement via *"byte comparison + DistributionAsserter + **HTTPExpectations** passes"*, while `driver.go:498-504` states HTTPExpectations is **intentionally NOT implemented** (the runner's branch uses HTTP/1.1 `helpers.HTTPRoundTrip`). The row edits this file anyway and **should fix that clause**.

**Backend dispatch is not what a BackendKind census suggests:** `BackendKind() == fixture.HTTPSH2` and the runner spawns a **`go run` SUBPROCESS** (`runner_test.go:1814`, `startHTTPSH2Backend`, `BACKEND_IDX` in env, `Setpgid`). Because it is a subprocess it does not increment the runner's in-process accept counter — which is why `AssertDistribution` is driver-implemented from response bodies.

### 6.3 The arm roster, the backend surface, and the break protocol

**Backend surface (one new exact-match handler, `backends/main.go`).** Phase 88's precedent, verbatim at this tip: *"Exact-match patterns beat the `/api/v1/` subtree handler above, so no existing behavior moves."* The new handler is `/api/v1/reflect-headers` and emits **two** lines:
- a **SORTED** `name: value` block, copying `0012`'s landed ten-line pattern (`backends/backend.go:21-36`, controller-verified to call `sort.Strings(names)` *"so reference vs envoy-go body bytes compare equal"*);
- ⚠️ **plus** an `order=` line listing, **in received order**, only the names matching the row's fixed probe prefix `x-p89-`.

⚠️ **THE DECISION THIS FORCES, MADE EXPLICITLY:** copying `0012` verbatim makes the body **order-insensitive**, which retires the cross-side risk of tail-appending net-new headers — **but it also means the fixture cannot pin wire ORDER at all**, and §2.3 makes order load-bearing for the first time in this lineage. The **prefix-filtered** `order=` line restores order coverage while staying immune to each proxy's own header additions (`x-forwarded-for`, `x-request-id`, `x-envoy-*`), which would otherwise diverge cross-side for reasons that have nothing to do with this row.

**Arms (driver-side, appended after the existing 31 so the transcript prefix stays byte-identical — the phase-88 discipline):**

| # | arm | asserts | break arm | **injection site (NAMED)** |
|---|---|---|---|---|
| A1 | `header_mutation` **append** `x-p89-added` | the name appears in the sorted block and in `order=` at the tail | **B1: delete the re-issued `rf.SetH2Request`** (keep the reconcile) | the re-issue line in `h2dispatch.go`'s delta block |
| A2 | **remove** `x-p89-removed` (pre-seeded by the client) | absent from BOTH lines | **B2: swap the fresh slice for the `fields[:0]` idiom** | the output-slice allocation inside the delta helper |
| A3 | **value change** on `x-p89-changed` | new value only, one occurrence, tail position in `order=` | **B6: rewrite at the FIRST occurrence instead of the tail** | the changed-name emission branch |
| A4 | **duplicate-name untouched** — client sends `x-p89-dup` twice, non-adjacent | both occurrences at their original positions in `order=` | **B6** (same site, opposite direction) | as above |
| A5 | **pseudo skip** — no config; every request | no `:`-prefixed name in the sorted block | **B3: remove the `:`-skip** | the colon test in the snapshot helper |
| A6 | **case** — mutation with a canonical-MIME key | lowercase in the sorted block | **B4: remove `strings.ToLower`** | the `hpack.HeaderField{Name: …}` construction |
| A7 | **§8.2.2 drop** — append `connection`/`transfer-encoding`/`te: gzip`, plus `te: trailers` as the stacked control | the three illegal absent, `te: trailers` **present** | **B5: remove the `IsIllegalH2RequestHeader` guard** | the two guard lines in the emission loop |
| A8 | **body-path** — a filter mutating headers from `DecodeData` | the mutation lands | **B7: move the reconcile ABOVE `RunDecodeData`** | the delta block's position relative to `:551-555` |

⚠️ **Seven break arms, seven DISTINCT injection sites** (`reference_break_arm_injection_site_is_a_claim`). ⚠️ **Run one break per run, `-count=1`** — a fail-fast driver masks later RED arms (`reference_failfast_driver_masks_later_red_arms`, `reference_differential_break_protocol_count1`). ⚠️ **Confirm WHICH assertion fired**, not merely that the run went red (`reference_deliberate_break_wrong_assertion`) — and note that `0004` asserts **in-band** via `return nil, fmt.Errorf(...)`, so the first failing arm aborts the Drive.
⚠️ **A5 has no config lever** — it fires on every request, so its break arm must be checked against the whole roster, not one arm.

### 6.4 Count deltas — **+0 on every axis**

fixtures **121** · narrowed blank imports **121** · BackendKind tail **38** · new ports **none** · PKI **already present** · `0120` **stays UNCONSUMED**. Anticipated band, anchored on the phase-88 measurement into the same fixture: **~+145-200**.

---

## 7. DOCS — TEXT CONSTRAINED AT THIS PLAN

### 7.1 ⚠️ `BEHAVIOR_CONTRACT.md` — the substring is NOT a unique anchor, and the zero-line-delta gate is BLIND to the failure

```
$ grep -o ', H2 differential coverage\.' docs/envoy-go/BEHAVIOR_CONTRACT.md | wc -l   =>  3
   :32  | HTTP filter `envoy.filters.http.header_mutation` |   <- THIS ROW'S
   :33  | HTTP filter `envoy.filters.http.local_ratelimit` |   <- NOT this row's
   :34  | HTTP filter `envoy.filters.http.csrf` |              <- NOT this row's
```

**NC executed on scratch copies:**

| edit | residual occurrences | line delta | collateral |
|---|---|---|---|
| naive global `sed 's/, H2 differential coverage\.//'` | **0** | **0** | `local_ratelimit` and `csrf` carve-outs **silently closed** |
| scoped to the row | **2** | **0** | none; rows 33/34 intact |

⚠️ **`reference_compensating_defects_cancel_in_the_gate_metric`: the SPEC's stated gate (zero line delta) reads IDENTICAL for the correct edit and for the wrong one.**

**BINDING FOR THE IMPL:**
- anchor on the **ROW**, which IS unique: `grep -c '^| HTTP filter \`envoy.filters.http.header_mutation\` |'` ⇒ **1** (secondary unique anchor: `Differential gate fixture 0012-http-header-mutation` ⇒ **1**);
- the edit deletes `, H2 differential coverage.` from **that row only** and extends the asserted half to name the H2 arm, **in place, ZERO line delta** (5960 stays 5960), so no by-line citation shifts — phase 88 shifted these anchors +2 and phase 87 +1;
- **the gate is the RESIDUAL COUNT: it must read `2`, not `0`.** A reading of 0 is the defect, not the success. Assert rows `:33`/`:34` still carry the phrase.
- ⚠️ **If the IMPL instead ADDS a bullet anywhere, that IS line-adding and it must say so.**

### 7.2 The sentences the IMPL must ADD — because omission would ship an over-broad claim

Two behaviour statements, both measured this stage, neither currently in the contract:
1. **`host` / `:authority` stays a no-op on the decode side, on BOTH legs** (§2.1). Without it, "decode mutations now reach the H/2 upstream" reads as universal.
2. **Access logs, spans, `request_headers_to_log` and tracing `custom_tags` now observe filter-mutated request headers** (§2.5) — three `req.Headers` readers, six `emitAccessLogH2` call sites.

⚠️ Both are **line-adding**; the IMPL must state the delta and re-anchor any by-line cite at/below the insertion point.

### 7.3 The mirror sentence STAYS — SPEC §8.2 CONFIRMED, no edit

`BEHAVIOR_CONTRACT.md:829`, under `### Does not yet apply to`:
> `- H2 decode-side observation of the injected headers (the H2 inject mutates the upstream-forwarded header set only, not the decode-side map; H1 mutates `req.Header` which is both).`

Exactly true, for a structural reason: a reconciler runs strictly **after** `RunDecodeHeaders` returns, and "decode-side observation" is what filters see *during* that call — a post-hoc reconciliation cannot retroactively change what already-executed filters observed. The data flow is the **inverse** of this row's (map → slice, vs the tracing gap's missing slice → map). **No edit.**

### 7.4 `DECISIONS.md` — ADR-0311 completion at the IMPL, not here

The strict guard stays **ARMED at 1** (`:18212`) through this PLAN. The IMPL appends §Decision + §Consequences **IN PLACE after the retained italic footer**, no renumber, **no `---` separator** (`^---$` stays **216**), and flips the guard **1 → 0**. Next-free stays **ADR-0312** (TAIL-derived; `^## ADR-0312` ⇒ **0**; ⚠️ headings+1 **COLLIDES** at the ADR-0209 gap). This PLAN adds **no ADR**.

### 7.5 Documentary defects — carried, plus TWO NEW at this stage

⚠️ **NEW — a PHANTOM SYMBOL cited three times INSIDE the very block this row edits.** `h2dispatch.go:462`, `:479`, `:491` all cite `h2.parseHeadersForRequest`, one with a hard line number (`h2/stream.go:399`):
```
$ git grep -c '^func.*parseHeadersForRequest' -- 'internal/**/*.go'   =>  0
$ git grep -c '^func buildH2Request'          -- 'internal/**/*.go'   =>  1   (POSITIVE CONTROL)
```
The function does not exist anywhere in the tree. `reference_code_comment_not_evidence`. **Recorded, NOT fixed** (a doc row, not this charter) — but flagged loudly because the IMPL edits these exact lines and must not read them as a map of the code.

⚠️ **NEW — `expectations.yaml:3-7`'s HTTPExpectations clause is false** (§6.2 trap 3). The row edits that file; fixing the clause is in scope.

⚠️ **NEW (pre-existing, found in passing, NOT chartered)** — two subject `host`/`:authority` divergences measured against the reference:
- inbound `:authority` **and** `host` ⇒ envoy-go projects **both** onto the H/2 upstream (`host` verbatim at index `[04]`) while the reference **drops** the regular `host`;
- inbound `host` **only**, no `:authority` ⇒ envoy-go emits **`:authority = ""` (EMPTY)** plus `host`, while the reference **promotes** `host` into `:authority`.
Both are **independent of phase 89** and reachable at the tip with no filter at all. **Named, not fixed.**

**CARRIED unchanged:** the false *"per ADR-0071's filter API stability"* comment in `internal/filter/http/types.go` (⚠️ **the PLAN and IMPL write "rejected on measured blast radius — 33 production + 38 test files, ~101 mutation call sites, and an `OrderedHeaders` type carrying only `Get`/`ToHTTPHeader` so the mutation API must be written from scratch". NEVER "per ADR-0071"**) · `header_mutation` rejecting `remove` of a protected header where the reference config-accepts it · `RunAction`'s unguarded H2 arm · `ADR-0051` §2 · `ADR-0058`'s dead `routerActionH2.doH2` location · ROADMAP rows 119/131 malformed (ARM-A guard) · `STATE.md` §Project counts frozen at phase 76 · `harness_test.go` port inventory stale · `body.go` nolint inert · xDS cycle guard not automated · `wasm/doc.go:219` · ROADMAP's five `esalaine` cites · `rbac.go:50` token `F2` · root `PROGRESS.md`'s stray phase-32.1 doc · SPEC-86/PLAN-86's nonexistent `internal/xds/xdsgrpc/…` path · the "no stdlib net/http parsing" sentence (retained, ridden by ADR-0309) · `ADR-0057`'s "27 round-trips" (now 31) · the two riders citing ADR-0052 at a drifted `:1821` · window `:221`'s two closed candidates.

---

## 8. COST — RE-MEASURED AT THE PUBLISHING COMMIT (SPEC §17 item 9)

⚠️ `reference_cost_figure_measured_at_publishing_commit` went stale **twice** inside the phase-88 IMPL, the second time in the sentence correcting the first. All figures below are measured at **`733f9830`**, which is this PLAN's base; **this PLAN is docs-only (zero `.go`), so its own commit moves no production figure**, and the IMPL re-measures at ITS publishing commit regardless.

**THREE independent implementations of one instruction were built this stage. The spread IS the finding.**

| sample | numstat | comment | blank | brace-only | **substantive** |
|---|---|---|---|---|---|
| SPEC prototype @ `c1284a03` | **+162 / −0**, 1 file | 69 (42.6%) | 5 | 24 | **64** |
| agent D @ `733f9830` | **+158 / −0**, 1 file | 84 (53.2%) | 6 | 19 | **49** |
| agent B, reconciler only @ `733f9830` | **+121 / −0**, 1 file | — | — | — | — |
| agent B, reconciler **+ validation** | **+127 / −0** `h2dispatch.go`, **+16 / −0** `h2/stream.go` | — | — | — | — |

⚠️ **The near-agreement of +162 and +158 is PARTLY COINCIDENCE.** The totals differ by 4 while the **substantive** counts differ by **15** — a heavier comment ratio absorbing a leaner algorithm. **These are not two confirmations of one number.** ⇒ **quote the SPEC's +162 as the floor** (the highest of the three; a lower prototype is a weaker bound).

⚠️ **AND +162 IS STILL A FLOOR**, because **none of the three prototypes implements the tail-append rule of §2.3** — all three rewrite at the first occurrence, which §2.3 measured as a reference divergence. The remainder, enumerated:

1. **The §2.3 tail-append correction** — the one algorithmic change over every measured prototype.
2. **`host` skip** (§2.1) — one predicate, but it must be gated.
3. **The §8.2.2 validation: +22 MEASURED** (+16 `h2/stream.go` exported predicate, +6 `h2dispatch.go`).
4. **Deterministic sorted append** for multiple added names.
5. **Zero tests** in every prototype — the 13-row table, the fifth tracing row **in two files**, the three early-exit arms, the `SetH2Request` arm, the `buildH2Request` characterization pin, plus the count-and-set helpers and the `headerMutatingFilter` double.
6. **Zero differential lines** — the `0004` extension of §6.
7. **Zero doc lines** — ADR-0311 §Decision + §Consequences, the scoped contract edit, the two added contract sentences, the ROADMAP row flip, `PROGRESS.md`, `STATE.md`, `STATE_HISTORY.md`.
8. **Unbenchmarked runtime cost**: two full `http.Header` deep copies on **every** H2 request, including the no-filter path.
9. Decode-side **trailers** have no seam (`RunDecodeTrailers` is not called on this path) — out of charter, named.

**IMPL BANDS, stated as bands and labelled:**
- production **~+185-250** — the +162 floor, **plus the MEASURED +22 validation**, plus items 1/2/4, ESTIMATED;
- unit tests **~+250-450** — ESTIMATED, revised **up** from the SPEC's ~+150-350 because the roster grew (13 rows, a pin in **two** files, three early-exit arms, new helpers and a new double);
- differential **~+145-200** — anchored on phase 88's **MEASURED** `+172 / −7` into the same fixture.

---

## 9. GATES AND COUNTS

### 9.1 The IMPL's gates (T8, run LAST against the FINAL tree)

`go build ./...` · `go vet ./...` · `gofmt -l` on touched packages (**gate on OUTPUT — it never exits non-zero**) · `golangci-lint` (⚠️ **misspell runs in locale US — sweep British spellings in `.go` comments BEFORE the gate**) · `go test ./... -count=1` · `-race` on `internal/filter/hcm/...` · the **full** differential suite with **`INNER_EXIT` captured** (the wrapper exits 0 even when the binary aborts mid-suite) · the **ANCHORED** panic gate `^panic:|DATA RACE|SIGSEGV` (⚠️ an unanchored form false-fires 14× on a fully green log) · `go mod tidy -diff` EMPTY · stat-surface delta by the **same command on both sides** · **h2spec cited ONLY from the IMPL's own run**.

⚠️ **h2spec is STRUCTURALLY incapable of anchoring this row** (SPEC §11): its harness configures `envoy.filters.http.router` **alone** with `direct_response` routes and never goes upstream — no decode-mutating filter, no upstream request, no mutation to lose. Run it as a regression gate; **do not present a green reading as coverage.**

⚠️ **Assert the SYMBOL, QUALIFIED, not merely that the build succeeded** (`reference_symbol_assertion_needs_qualified_name`; a bare `writeHeaderBlock` read **9** on an unmodified tip at phase 88). Assert the **bound binary** with `ss -ltnp` before reading any probe result.

### 9.2 The GREEN BASELINE the RED census is measured against

```
$ go test ./internal/filter/hcm/... ./internal/filter/http/router/... -count=1
rc=0 · ok=3 · FAIL=0
```
`=== RUN` denominators: `hcm` **301** · `hcm/h2` **192** · `router` **122** · combined **615**.
`-run TestWriteH2_Tracing` denominator **4**, **4/4 PASS**.
Prototype gate readings (both agents, independently): build rc=0 · `gofmt -l` empty · vet rc=0 · golangci-lint rc=0 · `go test ./internal/filter/hcm/... ./internal/filter/http/...` rc=0 **ok=25 FAIL=0** · `-race` rc=0 · `go.mod`/`go.sum` diff EMPTY · stat surface **406 / 406, DELTA 0**.

⚠️ **A `-run` selector that matches nothing prints `[no tests to run]` and EXITS 0.** Every census in the IMPL must quote its **denominator**.

### 9.3 Counts re-derived MECHANICALLY at this tip (`733f9830`)

`ROADMAP.md` **239 / 121 rows**, row 89 `in-progress` at `:151` · `DECISIONS.md` **18228**, **310** headings, tail **ADR-0311** at `:18210` (§Context only, `PROPOSED`), next-free **ADR-0312**, `^---$` **216**, STATUS census **24**, **strict `PROPOSED` guard 1 — ARMED, and this PLAN leaves it armed** · `BEHAVIOR_CONTRACT.md` **5960** · `STATE.md` **64** · `STATE_HISTORY.md` **502** (archive labels **200**) · `BOOTSTRAP_PROMPT.md` **522** · phase dirs **130** · `REVIEW.md` **37** (standing departure, named not claimed) · fixtures **121** at `test/fixtures/` (⚠️ **not** `test/differential/fixtures/`, which returns a silent **0**), tail `0119-grpc-unary-trailers`, `0120` **free** · narrowed blank imports `^\t_ "[^"]*test/fixtures/` **121** (⚠️ the unnarrowed `^\t_ "` reads **123** and is REFUTED — two non-fixture imports) · fuzzers **55 / 48**, anticipated **+0** (the row consumes no new config field) · BackendKind tail **38** · stat surface `grep -ro --include='*.go' -e 'NewCounter(' -e 'NewGauge(' . | wc -l` ⇒ **406** (`NewCounter(` 327 + `NewGauge(` 79; `-rn` LINE form **404**), **DELTA 0 measured on the prototypes**.

**Anticipated at row-done, each +0:** stat surface · fuzzers · BackendKind tail · `go.mod` · config fields · fixture count · blank imports. **`want` moves 121 → 121** (a row flip, not a row add).

### 9.4 ⚠️ ONE STANDING METHOD NOTE CORRECTED BY EXECUTION

The carried note says the reference is unreachable because *"`--network host` did not share the host netns … and `-p` publishing was unreachable too."* **The second clause is REFUTED.** Controller-executed at this tip:
```
$ docker run -d --rm --name b89ctl-net -p 47899:80 nginx:alpine
$ curl -sS -m 6 -o /dev/null -w 'HTTP=%{http_code}\n' http://127.0.0.1:47899/
HTTP=200
$ ss -ltn | grep -c ':47899'   =>  1
```
**`-p` publishing WORKS.** The real mechanism is narrower and sharper: **`--network host` is broken here AND it silently IGNORES `-p`**, so a probe that combined them read as "publishing is unreachable". The host-gateway is **`192.168.65.2`** — a Docker-Desktop-style VM daemon, **not** a `172.17.x` bridge gateway, which is exactly why host networking fails. **Both `-p` publishing and `--add-host=host.docker.internal:host-gateway` are working recipes; only `--network host` is not.**

### 9.5 ⚠️ AN INSTRUMENT DEFECT ONE PROBE FOUND IN ITSELF — carry it forward

An H2 raw-framer backend that builds a **fresh `hpack.Decoder` per request** decodes correctly only for the first request on a connection: **the HPACK dynamic table is CONNECTION-scoped**, so every later request on a pooled connection yields `invalid indexed representation index NN` and a **truncated field list** — which reads exactly like "headers were lost". The probe caught it, fixed it, re-ran everything, and asserted `HPACK-ERR` count **0** across all reported logs, with a client→backend **direct** negative control reproducing `a=1,b=2,a=3` at indices 4/5/6 verbatim. **Any raw-framer backend the IMPL writes must hold ONE decoder per connection and assert a zero HPACK-error count.**

---

## 10. EXPLICITLY NOT MEASURED (stated, never inferred)

- **`keep-alive` / `proxy-connection` / `upgrade` on the wire.** Config-validated `configuration OK` and structurally identical to the two that were wire-measured — **INFERRED**.
- **The reference's ordering for MULTIPLE simultaneously-added names.** The sort is chosen for determinism, not for measured fidelity (§2.3).
- Whether the reference applies decode-side mutations over downstream H2 with **wasm** or **extproc** (Lua and `header_mutation` WERE measured, on both legs).
- The `wasm`/`lua`/`extproc` guest-boundary cost of the REJECTED slice-native shape.
- **Runtime cost** of two per-request `http.Header` deep copies — no benchmark was run.
- The `~+185-250` / `~+250-450` bands are **ESTIMATES**; the measured anchors are +162 / +158 / +121 (production), +22 (validation) and phase 88's `+172 / −7` (differential).

---

## 11. NEXT

**IMPL** — ONE atomic commit delivering §4's T1-T8: the RED census observed first with denominators (**the fifth tracing row in BOTH files inside it**) · the reconciler with the §2.3 **tail-append** rule, the `host` skip, explicit lowercasing and the exported value-aware §8.2.2 guard · the 13-row table plus the three early-exit arms and the `SetH2Request` arm in `hcm` · the `0004` extension with eight arms and seven break arms at seven distinct injection sites · **the scoped contract edit whose gate is a residual count of 2, not 0** · the two added contract sentences · ADR-0311 completed with the strict guard **1 → 0** · **row 89 flipped `done`** at `ROADMAP.md:151` with `want` **121 → 121** · the slice-only-writer gate re-read at **6** and stated · cost re-measured at the IMPL's OWN publishing commit · the sentinel and all four NCs run on **both sides** of the row flip.
