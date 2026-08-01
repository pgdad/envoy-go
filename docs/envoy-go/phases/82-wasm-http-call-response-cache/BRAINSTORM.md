# BRAINSTORM 82 — wasm-http-call-response-cache

**Stage:** BRAINSTORM (lifecycle-state `DONE` -> `1`). **ROW 82 REGISTERED `in-progress`**; sentinel `want` bumped **113 -> 114** in the SAME commit. Base master **`4596adfe`**, taken from `git rev-parse master` at session start and **not** from any SHA quoted in the router. Branch `phase-82-brainstorm`. Docs-only: **ZERO production `.go`, ZERO test `.go`.**

**SELF-PICKED** per the 2026-07-12 standing directive (smallest defensible candidate first, severity as tie-break). Four investigation agents ran in parallel on disjoint remits, each in its own DETACHED worktree off `4596adfe` with private scratch and a private port band inside `42500-42899`; the controller re-derived every load-bearing claim itself rather than adopting a brief (`feedback_brief_citations_not_evidence`).

---

## 1. ⚠️ THE HEADLINE: A LANDED FIXTURE ALREADY EXERCISES THE DEFECT, AND A HEDGED COMMENT IS SUPPRESSING IT

envoy-go's proxy-wasm host tells a guest how large an HTTP-call response is, then returns nothing when the guest reads it.

`internal/wasm/http_call.go:403-409` computes the response tuple from the **real** response and hands the guest true counts:

```go
numHeaders  = uint32(len(resp.Header))
bodySize    = uint32(len(bodyBytes))
numTrailers = uint32(len(resp.Trailer))
```

`:411-417` then carries `// TODO Task 15: stash (resp.Header, bodyBytes, resp.Trailer) on the originating *StreamContext`. **That stash never landed.** A code comment is not evidence (`reference_code_comment_not_evidence`), so the consumer side was walked one layer up, and then one layer further:

- `internal/filter/http/wasm/abi_callbacks.go:178-191` — `headerMapForType` handles **only** types 0 and 1; its `default:` returns `(nil, false)`, covering `HttpCallResponseHeaders` (4) and `HttpCallResponseTrailers` (5).
- `abi_callbacks.go:200-204` — `GetHeaderMap` short-circuits on `!active || headers == nil` and returns `(nil, false)`; its own doc comment states *"the host wrapper converts `!ok` to `WasmResultNotFound` for the guest."*
- `abi_callbacks.go:647-661` — `GetBuffer`'s `default:` swallows `HttpCallResponseBody` (4) and returns `(nil, nil)`.

So the guest is told *"N headers, M body bytes"* and every read returns empty. This is a **silent wrong-answer divergence**, not a loud reject — the severity class phase 77 and phase 81 both used as their tie-break merit.

### ⚠️ AND THE CORPUS ALREADY CONTAINS THE FAILING-FIRST ANCHOR

`test/fixtures/0036-http-wasm-body-and-advanced/scripts/l_httpcall_success/src/lib.rs` — a **landed, vendored, 61-line** Rust guest — already does exactly the read that breaks:

```rust
fn on_http_call_response(&mut self, _: u32, _: usize, _: usize, _: usize) {
    let headers = self.get_http_call_response_headers();
    for (k, v) in headers.iter() {
        if k == ":status" { if let Ok(n) = v.parse::<u32>() { self.call_status = n; } }
    }
}
...
self.set_http_response_header("x-httpcall-status", Some(&self.call_status.to_string()));
```

`get_http_call_response_headers()` routes to `proxy_get_header_map_pairs(HttpCallResponseHeaders)` -> `NotFound` -> empty vec -> `call_status` stays **0** -> the **subject** emits `x-httpcall-status: 0`. The reference implements this fully and emits **`200`**.

⚠️ **THE LANDED DRIVER COMMENT STATES THE DIVERGENCE BACKWARDS, AND THAT COMMENT IS WHY THE SCENARIO IS NOT ASSERTED.** `test/fixtures/0036-http-wasm-body-and-advanced/inputs/driver.go:504-509`:

> `// (l) httpCall-success — SUBJECT-ONLY. The wasm guest issues an`
> `// httpCall to cluster_b at proxy_on_request_headers; the response`
> `// carries x-httpcall-status: but the value diverges cross-side`
> `// (subject may return 200 from cluster_b echo; reference V8 may`
> `// return 0 if its envoy ext doesn't recognize the cluster).`

The call graph says the **opposite**: subject `0`, reference `200`. The comment is hedged (*"may … may"*), unmeasured, and directionally wrong, and it is the stated justification for classifying scenario (l) **SUBJECT-ONLY** — i.e. for asserting only that two counters increment (`README.md:71`: `http_call_dispatched >= 1` + `http_call_response >= 1`) and never comparing the header cross-side. **A wrong comment is currently buying silence for a real divergence.** `reference_code_comment_not_evidence`, fired at full strength.

**Consequence for cost: the row needs NO new guest blob and NO new fixture.** The blob exists, is vendored, and already exercises the gap. Row 82 flips scenario (l) from SUBJECT-ONLY to cross-side and asserts the header — a failing-first anchor already sitting in the corpus.

---

## 2. THE PICK, AND THE REJECTED ALTERNATIVES

The standing directive orders candidates **smallest defensible first, severity as tie-break**. ⚠️ **Every cost below was re-derived at `4596adfe`** — `reference_deferred_candidate_cost_restale`. None is inherited.

| # | candidate | net `.go` LOWER BOUND | tasks | new fixture | moves sentinel? | disposition |
|---|---|---|---|---|---|---|
| 1 | **`wasm-http-call-response-cache`** | **~110-140** | **7-9** | **no** | **YES — check (3)** | **PICKED** |
| 2 | Observability `ssl.connection_error` bucket | ~230 (real 575-700) | 9-12 | no (extends `0110`) | no | rejected — larger, moves nothing |
| 3 | lua `statNameRegexLiteral` removal | ~40 (deletion) | 1 | no | no | rejected — not phase-shaped |
| 4 | matcher-tree `Action`-name boot walk (F2) | ~300-500 | 4-6 | no | no | rejected — mints a NEW departure |
| 5 | driver-owned receiver port race | ~730 (real **1800-2200**) | 6-8 | n/a | no | rejected — **band-crossing** |
| 6 | RBAC policy-name PROJECTION divergence | ~900 (real 900-1400) | 10-14 | yes | no | rejected — larger |
| 7 | `stats-name-empty-segment-guards` | ~2400 (real **3200-5400**) | **22-30** | yes | no | rejected — **crosses the split gate on BOTH axes** |
| 8 | dynamic `ssl` cipher/version stat family | LARGE | — | yes | no | rejected — **both blockers still live** |
| 9 | smallest gRPC row (`grpc_stats` / `grpc_web`) | ~250-300 / 600+ | 12-14 / 16-22+ | yes | yes (3) | rejected — off-charter / hard-blocked |

**Why #1 wins outright.** It is the smallest candidate that carries real severity; it is the only one whose failing-first anchor is already landed; it needs no new fixture, no new port, no new BackendKind, and no new package; and it is the **only** candidate in ~31 phases with a mechanical path to moving a sentinel check. #3 is smaller but a 1-task deletion is not a defensible *phase* — it folds into any later row. #5 and #7 are disqualified on size, not merit.

⚠️ **STATE WHAT THE ROW MOVES; DO NOT FORECAST.** Measured at this tip, not predicted:
- **Check (2) — this row narrows NOTHING.** The WASM family heading (`ROADMAP.md:219`) carries **no** deferred-candidate sentence at all, so there is nothing to consume, and `wasm-http-call-response-cache` appears on none of the five sentences. Check (2) stays **FIVE**. This is the thirty-second consecutive phase at which it does not go down.
- **Check (3) — this row DOES move it, from two failures to one.** Its summary carries the literal `WASM-family row` as a genuine **use**.

---

## 3. ⚠️ THE DRAFTING FOOTGUN THAT WOULD HAVE SILENTLY WASTED THE ROW

The gate is `grep -qi -- "$slug-family row"` with `slug=WASM`, i.e. it needs the literal **`WASM-family row`**. But the heading is named `### WASM host family`, and this lineage's row-summary convention is `<ORDINAL> [§9 ]<Family>-family row`. Writing the natural-reading **`WASM-host-family row`** does **not** match. Executed on two synthetic rows:

```
'THE FIRST WASM-host-family row'  => FAILS  <-- does NOT satisfy the gate
'THE FIRST WASM-family row'       => PASSES
```

**Row 82's summary uses `WASM-family row` exactly.** A successor editing that cell must preserve the string.

### 3.1 The leak check — armed, rehearsed, and PASSING

⚠️ **NEVER WRITE A SENTINEL MATCHER STRING INTO A FILE THE SENTINEL GREPS.** The two classes are opposite and must not be conflated:

- **`<slug>-family row` is the check-(3) PASS condition** — writing it as part of a genuine chartered row is a **USE**. (Writing it *without* a real row is the leak; that is exactly why phase 79's WASM *rider* was declined.)
- **`deferred candidates:` / `remaining deferred (not-yet-chartered) candidates:` are check-(2) FAIL conditions** — writing either into `ROADMAP.md`, even as a quotation, makes the sentinel strictly worse. **Row 82 writes neither.**

Rehearsed on the drafted cell before landing, with both arms negative-controlled, and the by-mention silencing **reproduced live**: injecting `WASM-family row` into a scratch ROADMAP copy made `WASM` vanish from check (3) while `gRPC` still printed.

---

## 4. SCOPE

**In scope.** Stash `(resp.Header, bodyBytes, resp.Trailer)` on the originating `*StreamContext` at `http_call.go:411`; wire `GetBuffer` case `HttpCallResponseBody`; wire `headerMapForType` cases `HttpCallResponseHeaders` / `HttpCallResponseTrailers`; clear the stash on stream/context teardown so it cannot leak across calls; flip fixture `0036` scenario (l) to cross-side and assert `x-httpcall-status`; correct the backwards driver comment.

**Out of scope, deferred by name.** The 9 `WasmResultUnimplemented` stubs at `internal/wasm/registration.go:877-882` (shared-queue ×4, outbound-gRPC ×5); `proxy_on_queue_ready` and the `proxy_on_grpc_*` guest callbacks (**0** hits repo-wide, NC: `proxy_on_http_call_response` => 15); the wasm **network** filter (no `envoy.filters.network.wasm`; the 5 TCP callbacks are absent); the 6 deferred cpp-host conformance families.

**Helpfully already built:** `internal/wasm/abi/body_bridge.go:104` **already** admits `WasmBufferTypeHttpCallResponseBody` in its accepted-type switch, so the ABI shim needs no change. The gap is purely stash + three dispatch arms.

### 4.1 Open questions for the SPEC

- **D-82-DIRECTION** — prove by execution which side emits which value. The call graph says subject `0` / reference `200`; the landed comment says the reverse. **The SPEC must settle it against `envoyproxy/envoy:contrib-v1.37.2`, not by reading.** A probe that cannot discriminate proves nothing (`reference_probe_must_discriminate`).
- **D-82-LIFETIME** — where the stash is cleared, and whether a second `dispatch_http_call` on the same stream must overwrite or queue. Bears on whether the row is 7 or 9 tasks.
- **D-82-TRAILERS** — whether response trailers are reachable at all. ⚠️ **A4 found `RunEncodeTrailers`/`RunDecodeTrailers` have ZERO non-test callers** (2 non-test *mentions*, both in `internal/filter/http/chain.go` — its own definition and doc comment). If `resp.Trailer` is populated by `net/http` independently of that seam this is fine; if not, the trailers arm may be structurally dead and must be scoped out rather than shipped vacuous (`reference_vacuous_break_modes`).
- **D-82-BREAK** — the deliberate-break arm. `0036`'s README §"Deliberate-break liveness verification" makes this mandatory for every StatsAsserter arm; a cross-side arm needs its own.

### 4.2 Anticipated surface

**+0 stats · +0 fixtures (120) · +0 fuzzers (55) · +0 BackendKind (tail 38) · +0 packages (73) · +0 go.mod modules · +0 new PUBLIC surface.** ADR-0304 anticipated (§Context at the SPEC per ADR-0044-**as-used** — ⚠️ ADR-0044 does not itself contain that discipline).

### 4.3 Cost and split posture

**Band ~110-350 net `.go`, budget ~250, 7-9 tasks. NEITHER §6.1 trigger fires** (`BOOTSTRAP_PROMPT.md:289` ~25 tasks, `:290` ~1500 LoC) — ~3x margin on tasks, ~4x on LoC. **NO SPLIT.**

⚠️ **THE FIGURE IS A LOWER BOUND, NOT AN ESTIMATE.** `reference_measured_prototype_is_a_lower_bound`: phase 81 built, ran and `numstat`-measured nine prototypes and still landed **3.07x** over them (725 -> 2229); phase 80 landed 1636 against ~640. At 3x this row lands ~330-420 — still comfortably inside §6.1. **The PLAN must state its own basis and not inherit this one.**

---

## 5. REFUTATION LEDGER — WHAT THIS STAGE FOUND BY EXECUTION

**Load-bearing:**

1. ⚠️ **The router's picking guidance is INVERTED on its own preferred candidate.** It calls the driver-owned receiver port race *"simultaneously small, unblocked, severity-bearing and on NO family's sentence"*. Three hold; **"small" does not.** Measured: the roster is **42 fixtures, not 14** (0 false positives, **28 false negatives**; controller re-derivation with coarser file-level matchers gives 45, same conclusion), across far more than the three claimed families, and the cheapest defensible fix is **~730 net `.go` LOWER BOUND** -> **1800-2200 realized**, worse than phase 81's crossing.
2. ⚠️ **`0036` scenario (l)'s SUBJECT-ONLY classification rests on a directionally WRONG comment** (§1). This is the pick's whole basis and it was found by walking the call graph, not by reading the TODO.
3. ⚠️ **The empty-segment successor is 3x the router's cost and CROSSES the split gate on BOTH axes.** Router: *"14 incumbents + row 81's 10 new, ~700-900 net"*. Measured: **24 guard sites** (14 + **10** call sites — F1 occupies two, `rbac.go:371` and `:377`), with `boot.go:286` an already-closed 25th; **~2400 net LOWER BOUND, 22-30 tasks**. Phase 81 spent **223 net/site** over 10 sites under the already-cheap posture.
4. ⚠️ **ADR-0132 §Decision (v)'s reference-parity premise is UNEVIDENCED.** `DECISIONS.md:6291` is at the cited line and does say *"MUST mirror this exact shape"* — but **no phase-14 probe ever set an empty `compressor_library.name`**: both set `name: text_optimized` (`SPEC.md:1399` *"same library name"*), so `SPEC.md:36`'s *"empirically confirmed via probeC"* is **false**, and the same corpus calls D5 a *"planner-time decision"*. Fixture `0016` sets a non-empty name on **both** sides, so there is **zero** cross-side enforcement. ⇒ **DISPOSABLE BY CARVE-OUT; superseding is not required to charter.**
5. ⚠️ **ADR-0132 is not the only blocker — `internal/filter/http/lua` carries a SECOND, independently ratified empty-segment shape.** `lua/stats.go:72`: *"AMEND-2 EXPLICITLY RATIFIES this literal consecutive-dot wire shape"*, 8 counters plus a double-empty `http..lua..` variant, all pinned and passing. **A carve-out must cover two packages, not one.**
6. ⚠️ **The check-(2) summary table in the router is wrong about `:211`.** Read at this tip, the Observability sentence names **FOUR** un-chartered items, not one: the dynamic `ssl` family, **the uncounted non-certificate handshake-failure bucket (`connection_error`)**, tracing `spawn_upstream_span`, and `http_service`/force-trace. The table listed only the first. `feedback_brief_citations_not_evidence`.

**Structural and numeric:**

7. **`### WASM host family` is at `ROADMAP.md:219`, not the router's `:218`**; `### gRPC family` is at `:193`. Lines 190/216/218 are **blank**. Phase 79's `:216`/`:190` are stale by +3. Use phrase anchors (`reference_stale_cite_recurs_fix_by_pattern`). ⚠️ **BOTH FIGURES ARE PRE-INSERT. This row's own append moves them to `:220` and `:194`**, and the check-(2) anchors from `:191 :201 :211 :217 :225` to **`:192 :202 :212 :218 :226`** — verified after landing. A line cite into `ROADMAP.md` is stale the moment any row is appended; that is finding 22, and it applies to this document too.
8. **`ROADMAP.md:76` does not decline a WASM row, and the previously-recorded framing was imprecise in a load-bearing way.** The literal phrase `FINAL row` does **not** appear at `:76` (control: the line is 11404 bytes). It reads *"the FINAL **§9 HTTP-filters-family** row"*. The family qualifier is the entire argument; dropping it is what made the sentence look like a WASM blocker. The actual decline is at `:141` and is scoped to a **rider**, both of whose reasons evaporate for a genuinely new row.
9. **"proxy-wasm all 10 families PASS" drops its denominator.** Re-run at this tip: `ok … 0.246s`, 18 subtests, **10/10 PASS, 0 SKIP** — the number is honest. But `test/conformance/proxy-wasm/README.md:14` says **"The 16-file cpp-host roster: 10 ported, 6 deferred"** — it is **10 of 16 (62.5%)**. `reference_sample_is_not_an_audit`.
10. **BOOTSTRAP §6.3 is at `:304`, not the router's `:306`.** §6.1 `:285`, §6.2 `:294`, §7.5 `:357`, gates `:360-365`, close `:367` all verified exact.
11. **The `STATE_HISTORY.md` tolerant-anchor count is 169, not the router's 168** — 161 naive plus exactly **8** parenthetical evictions, at 438 lines, NC firing at 170 on a doctored copy.
12. **Only ONE defective `freeTCPPort` survives, and it cannot abort the differential.** `test/conformance/h2spec/h2spec_test.go:219` carries both pre-`f2dd994a` defects but lives in a **different test binary** and uses `t.Fatalf`, not `panic`. The differential's own is fully hardened. The real surface is **38 hand-rolled inline allocators**, none named `freeTCPPort`.
13. **The port race splits into three classes with TWO different failure modes.** 36 Class-A1 (probe loopback, bind wildcard — `f2dd994a` defect (1), never propagated to drivers; reproduced live) · 2 Class-A2 · **4 Class-B** (`0066`/`0067`/`0068`/`0071`) whose `allocDeadPort` failure is **inverted and silent** and is **immune to the hold-the-listener fix**.
14. **`test/` contains ZERO `recover()`** (NC: 51 repo-wide), so a driver `panic` aborts the whole binary — demonstrated with two follow-on tests that never ran.
15. **ACCEPT-pins are landed at 8 of 9 phase-81 sources, not "every" source.** F2 (matcher-engine `Action.name`) has none — consistent with F2 having no boot guard. All 8 are executable assertions, not comments.
16. **There are THREE stale incumbent guard cites, not two.** `lua/compiled_config.go:272` (two dead cites) **and** `redisproxy/config.go:51`, all pointing at `manager.go:205` / `hcm/config.go:209`, which are a doc comment and a `// Get looks up a cluster by name.` respectively. Live sites: `internal/filter/hcm/config.go:267`, `internal/cluster/manager.go:419`. `internal/filter/network/hcm/` does not exist.
17. **The bare-prefix trio is equally holed on empty segments.** `parseConfig(stat_prefix="a..b")` returns `nil` on redis, thrift **and** kafka — they are stronger on charset but **not exempt** from the empty-segment retrofit.
18. **The empty-segment hole is POSITION-dependent, and the two phase-81 statements that read as contradictory are both true about different subjects.** As a *bare whole name* only interior `..` is accepted (`.policy`, `policy.`, `.`, `""` all false). At an *interior token position* **all five** shapes are accepted. Neither prior statement named its subject.
19. **Item (a)'s two blockers are both still live, and phase 81 does not move blocker 1.** `IsValidName("ECDHE-RSA-AES128-GCM-SHA256")` = **false** bare *and* assembled — a hyphen is outside the character class at **every** position, so the segment-position rule cannot help. Blocker 2 binds only TLS <= 1.2: `0x1302` spells `TLS_AES_256_GCM_SHA384` identically in both. ⇒ **still impossible, not possible-but-strict.**
20. **The Observability deferred sentence's "a FOUR-family dynamic surface blocked on NAMING" conflates three dispositions.** (`ROADMAP.md:211` pre-insert, **`:212` after this row lands.**) Only `ciphers` is charset-blocked, and only for TLS <= 1.2. `curves` (`X25519`) and TLS-1.3 `ciphers` pass; `versions` (`TLSv1.2`) passes charset but injects a spurious dot-**segment** — an SN3 tag-extraction problem, categorically different. **`sigalgs` was never observed emitted** under any of six live reference arms — its existence as a live surface is inherited, not measured.
21. **`BEHAVIOR_CONTRACT.md:1967` asserts a live blocker its own successor phase killed** — *"still blocked on enumerating its membership"* / *"the full membership is UNENUMERATED"*, contradicted by phase 75's own 18-Go + 16-reference arms and again by a fresh 6-arm probe. A **landed contract document** carrying a stale blocker; anyone costing from it alone gets the wrong answer. **Recorded, not fixed.**
22. **Chartering row 82 shifts 40 of 119 `ROADMAP.md:<line>` cites, and 35 are structurally UNREPAIRABLE** — they live in append-only phase records (66, 73-81) and `DECISIONS.md`. Only the 5 in `next-prompt.txt` can be fixed. ⚠️ **This is structural to EVERY row-adding phase and no prior document records it.** It silently includes the sentinel's own check-(2) anchors, which become `:192 :202 :212 :218 :226`.

**Findings about this stage's own instruments:**

23. **Two of the controller's own extractors returned EMPTY and were BROKEN, not zero** — the ADR STATUS census (`^\*\*STATUS:\*\*`; the real form is a `> **STATUS: ` blockquote — census **16, all COMPLETE**) and the retained-footer matcher (real form `*(§Decision + §Consequences land at the phase-N IMPL.)*` — count **9**, set exactly ADR-0294..0300, 0302, 0303). Caught only because the input denominator was measured alongside. `reference_empty_output_is_not_a_zero_result`.
24. ⚠️ **The controller's transcription of the ADR recurrence guard was a FALSE-POSITIVE GATE.** Matching `PROPOSED` anywhere in the STATUS line fired on **four COMPLETE ADRs** on a clean tree, because those blockquotes discuss the word in prose. Anchored on the STATUS **word** (`^> \*\*STATUS: PROPOSED`) it is silent on clean and fires exactly once on a doctored copy. **A guard must be re-verified in both directions after transcription, not just after authoring.**
25. **The archive-absence guard SELF-CLEARS — phase 81 §5.4's finding REPRODUCED at this stage.** After the eviction, `grep -c 'phase 80 (stats-sds-projection) PLAN done' STATE.md` reads **1**, not 0 — because the `last-updated` field that *records* the eviction names its target. The bullet-anchored form `^- \*\*prior active-phase:\*\* \`phase 80 …\`` reads **0** correctly, with a firing NC on the phase-80 IMPL bullet that legitimately remains (**1**). **Anchor the guard on the BULLET, never on the phase name.**
26. **A controller re-derivation arm silently matched NOTHING because `xargs` cannot exec the shell keyword `command`** — four arms read `0` against a 144-file input. Re-run with `git grep`, they read 45/45/37. **The `command grep` idiom that defeats the .gitignore-blind wrapper does not survive `xargs`.**

---

## 6. SENTINEL — RE-RUN MECHANICALLY AT THIS STAGE. IT DOES **NOT** FIRE

Input measured **229 lines / 113 data rows** BEFORE the edit, so an empty result cannot read as a zero result.

- **(1)** at `want=113`: **SILENT** — every row was `done` on arrival. ⚠️ **The one stage where silence is indistinguishable from a broken check, so the doctored-copy NC was MANDATORY and was run: row 62 flipped `in-progress` on a scratch copy ⇒ `NOT DONE: row 62`, with the NC VERIFIED TO HAVE LANDED by printing the doctored status field (`NC LANDED? [ in-progress ]`) before its result was trusted.** Denominator NC at `want=112` ⇒ `GATE FAIL: examined 113 data rows, expected 112`.
- **(2)** **FIVE — `:191 :201 :211 :217 :225` — UNCHANGED. THIS ROW NARROWS NOTHING, STATED AND NOT FORECAST. The THIRTY-SECOND consecutive phase at which it did not go down.** ⚠️ A one-arm strip is **not** an NC here: `sed 's/deferred candidates://'` moves the union **5 -> 4, not 5 -> 0**, re-confirmed at this tip.
- **(3)** `NEVER OPENED: gRPC`, `NEVER OPENED: WASM` on arrival. NC: an invented slug fires; the registered `Observability` correctly prints nothing. ⚠️ **AFTER row 82 lands, check (3) reports `NEVER OPENED: gRPC` ALONE.**
- ⇒ checks (2) and (3) both print, so **the sentinel does not fire**. `ls stop` ⇒ `No such file or directory`, and it was **not** created.

**Row well-formedness.** The disjunction gate reproduces exactly at this tip over 113 rows: **ARM-A** (escape-aware) catches lines 119 and 131 only; **ARM-B** (trailing piece) catches line 140 only; **naive `NF==8` flags 17** — 15 false positives, and it **misses** line 140 (compensating defects cancel). **Row 82 is written NF=8 with no literal `|` in its summary prose**, asserted mechanically before writing, and is flagged by neither arm.

---

## 7. COUNTS RE-DERIVED AT THIS TIP

fixtures **120** (faithful `^[0-9]{4}[a-z]?-`; naive `^[0-9]{4}-` gives **118**; numeric tail `0118-runtime-static-layer`, next-free `0119`) / fuzzers **55** / internal packages **73** / phase dirs **122** with **37** `REVIEW.md` (**85 without**) / `DECISIONS.md` **17796**, **302** `^## ADR-` headings, tail **ADR-0303 COMPLETE**, next-free **ADR-0304** (`grep -c '^## ADR-0304'` ⇒ 0; NC `^## ADR-0303` ⇒ 1), `^---$` **216**, STATUS census **16 all COMPLETE**, retained italic footers **9** / `ROADMAP.md` **229 -> 230 lines, 113 -> 114 data rows** / `BEHAVIOR_CONTRACT.md` **5870** / `STATE.md` **63** / `STATE_HISTORY.md` **438**, tolerant anchor **169** ⚠️ **not 168** / `ROADMAP.md:<line>` cites **119**, of which **40** shift and **35** are unrepairable.

⚠️ **CONTESTED, SO NO NUMBER IS CARRIED:** the next-free REFERENCE port (routers say `10119`; `STATE.md` §Project says `10450`) · the `STATE_HISTORY.md` archive-gap total · production `stats.IsValidName` **guard sites 25** vs a naive line-grep's **56** — these measure different things and must not be conflated.

**Verified toolchain fact for the SPEC:** Rust **1.96.0** with the `wasm32-wasip1` target is installed on this machine, and the guest build recipe (`0036/scripts/README.md:26-51`, `proxy-wasm-rust-sdk =0.2.4`, blobs vendored + committed) is reproducible. **CI carries no Rust toolchain** — blobs are built locally and vendored, which is the phase-25.2 precedent. **Row 82 needs no new blob**, but a successor that does can build one.

---

## 8. SIX-GATE (§7.5, `BOOTSTRAP_PROMPT.md` AT THE REPO ROOT — `:357`, `:360-365`, `:367` re-verified exact)

⚠️ **A docs-only BRAINSTORM owes (a)-(f) only in the posture a docs-only stage can have.** Stated as INAPPLICABLE rather than claimed green:

- **(a)/(b)** differential fixtures — **INAPPLICABLE**: zero `.go` touched, so no fixture can change state. Not run, and not claimed.
- **(c)** conformance — **INAPPLICABLE** for the row; proxy-wasm *was* re-run incidentally as evidence for §5 ledger 9 (**10/10 PASS, 0 SKIP**, denominator **10 of 16** stated).
- **(d)** fuzzers — **VACUOUS**: this row adds none (**55** repo-wide, unchanged). Said to be vacuous, not green.
- **(e)** lint/format/vet/modules — **INAPPLICABLE**: zero `.go` bytes changed; `golangci-lint` runs on Go packages, and `go.mod`/`go.sum` are untouched.
- **(f)** `REVIEW.md` — **ABSENT. A STANDING LINEAGE DEPARTURE, recorded rather than claimed as compliance**: **85 of 122** phase directories carry none (**37** do); the last authored was 25.3. `PROGRESS.md` has discharged it for the whole recent lineage.

---

## 9. HYGIENE

Fresh worktree off the CURRENT master tip `4596adfe` (`git rev-parse master`), branch `phase-82-brainstorm`. Four investigation agents in **DETACHED** worktrees off the same base, each with private scratch and a private port band inside `42500-42899` — clear of **both** reserved bands (`20000-31007` subject blocks, `11000-14999` backends) and the static fixture ports. Zero commits, zero branches and zero pushes by any agent; all four reported `git status --porcelain` = **0 lines**, controller-re-confirmed. Every throwaway `.go` probe was deleted and the tree proven clean. Six docker containers were created (`a2-probe-1..6`) and **all six removed BY NAME**; ⚠️ **no image or ancestor filter was ever used**, and no container this session did not create was touched (`reference_parallel_agents_shared_machine_namespaces`).

⚠️ **`Shell cwd was reset to /home/esa/git/envoy-go` FIRED LIVE at this stage**, as it has for twenty-nine consecutive sessions. Every git command used `git -C <abs-worktree-path>`; branch confirmed `phase-82-brainstorm`, never `master`, before any commit.

---

## 10. NEXT

**The phase-82 SPEC.** It must settle **D-82-DIRECTION by execution against `envoyproxy/envoy:contrib-v1.37.2`** — the call graph and the landed comment disagree, and the comment is the reason the scenario is unasserted, so reading is not enough. It must also settle **D-82-TRAILERS** before scoping the trailers arm, since `RunEncodeTrailers`/`RunDecodeTrailers` have zero non-test callers and a trailers assertion could ship vacuous. ADR-0304 §Context lands at the SPEC. ⚠️ **Budget ~3 differential launches (~20 min) per green pass** — the driver-owned receiver race is now measured at **42** fixtures and remains un-chartered.
