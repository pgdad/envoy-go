# Phase 89 — `h2-decode-filter-mutations` — BRAINSTORM

**Base master `125f0714`. Stage branch `phase-89-h2-decode-filter-mutations-brainstorm`. Date 2026-08-15.**

**SELF-PICKED** per the 2026-07-12 standing directive. Phase 88 is CLOSED, all 120 chartered rows are `done`, and **no banked mid-lifecycle work exists** — proven, not assumed: check (1) is SILENT at `want=120`, so no row is `in-progress`. (The incomplete artifact sets under `docs/envoy-go/phases/` — e.g. `05-http-2 [SPEC only]`, `06-observability-baseline [BRAINSTORM+SPEC]` — are historical parent/child SPLIT structure on CLOSED rows, not banked work.)

---

## 1. The pick, and why it is defensible as "smallest first"

**THE PICK: decode-side filter header mutations never reach the upstream request on the HTTP/2 downstream leg.**

The bar set by BRAINSTORM-88 §1 is not "fewest lines" but **the smallest candidate that is a PRODUCT DEFECT reproducible by execution**. Candidates with nothing to reproduce do not qualify at any size.

Ranked by cost among the candidates that clear that bar — every figure re-derived at THIS tip (§4):

| # | Candidate | prod `.go` est. | stat Δ | config fields | ADR supersession | test inversion |
|---|---|---|---|---|---|---|
| **1** | **this row** | **~+50-90** | **0** | **0** | **none** | **none** |
| 2 | `server_name` + `server_header_transformation` | ~+80-130 | 0 | +2 | — | — (but +1 BackendKind) |
| 3 | `max_request_headers_kb` + `http2.header_overflow` | ~+80-140 | **+1** | +1 | **ADR-0041** | **YES** |
| 4 | `access_log[].filter` (2-arm split) | ~+130-190 | 0 | — | — | — (but +1 fixture) |
| 5 | stream-scoped overflow via decode-and-discard drain | ~+130-220 | 0 | optional | — | **YES** |

It is the smallest of the five **and** it has the lightest gate footprint of any of them.

⚠️ **IT IS ALSO THE ONLY ONE THAT IS A DEFECT RATHER THAN A MISSING FEATURE.** The other four are un-implemented config surface — feature parity. Here the sibling code paths already do it correctly, and **not just one of them**:

| codec | snapshot call | decode-chain call | verdict |
|---|---|---|---|
| H1 | `internal/filter/hcm/connection.go:468` `rf.SetRequest(req)` | `:571` `RunDecodeHeaders(ctx, req.Header, …)` — **SAME object** | **CORRECT** |
| H3 | `internal/filter/hcm/h3dispatch.go:217` `rf.SetRequest(r)` | `:263` `RunDecodeHeaders(ctx, r.Header, …)` — **SAME object** | **CORRECT** |
| H2 | `internal/filter/hcm/h2dispatch.go:457` `rf.SetH2Request(h2req)` | `:518` `RunDecodeHeaders(ctx, c.req.Header, …)` — **DIFFERENT container** | **BROKEN** |

**TWO OF THREE CODECS ARE RIGHT.** That is not a design tradeoff; it is an oversight in one of three symmetric paths. ⚠️ **The H3 fact was produced at THIS stage** — it is carried by neither ADR-0310 nor any router.

**Provenance:** ADR-0310 §Consequences (x) (`DECISIONS.md:18206`), which files it as *"a pre-existing framework property, NOT a phase-88 defect"*. ⚠️ **That sentence is about PROVENANCE, not about whether it is a divergence.** §2 shows by execution that it is one.

---

## 2. The defect, REPRODUCED BY EXECUTION at `125f0714` — with a positive control on every arm and a negative control on the isolating arm

Subject built from tip into scratch (`go build -o $S/envoy-go ./cmd/envoy-go/`); reference `envoyproxy/envoy:contrib-v1.37.2`; client `curl 8.5.0` (nghttp2) with `--http2-prior-knowledge`. Filter fragment copied VERBATIM from fixture `0012-http-header-mutation/envoy-go.yaml` rather than invented:

```yaml
mutations:
  request_mutations:
    - append: { header: { key: "x-probe", value: "seen" }, append_action: OVERWRITE_IF_EXISTS_OR_ADD }
    - remove: "x-remove-me"
  response_mutations:
    - append: { header: { key: "x-resp-probe", value: "resp-seen" }, append_action: OVERWRITE_IF_EXISTS_OR_ADD }
```

### 2.1 Arm 1 — SUBJECT, H2 downstream, decode-side ADD — the header NEVER ARRIVES

Client sends `x-remove-me: please-remove` and `x-client: c1`. The backend **actually received**:

```
METHOD GET PATH /arm1 PROTO HTTP/2.0 HOST 127.0.0.1:47580
Accept: */*
User-Agent: curl/8.5.0
X-Client: c1
X-Remove-Me: please-remove
```

**No `X-Probe`.** ⚠️ **`X-Client` DID survive** — a wire header resident in the `[]hpack.HeaderField` slice — so upstream H2 header serialization itself is fine. The loss is specific to the mutation, not general.

### 2.2 Arm 2 — POSITIVE CONTROL, H1, identical chain — the header ARRIVES

```
METHOD GET PATH /arm2 PROTO HTTP/1.1 HOST 127.0.0.1:47581
Accept: */*
User-Agent: curl/8.5.0
X-Client: c1
X-Probe: seen
```

`X-Probe: seen` present and `X-Remove-Me` **absent**. The filter, the config and the backend are all correct ⇒ **Arm 1's absence is the codec path, not a broken probe.**

### 2.3 Arm 3 — POSITIVE CONTROL, H2, ENCODE side — the encode direction is FINE

Downstream response headers on the same H2 listener carried `x-resp-probe: resp-seen`. The encode side reconciles via `filter_http.ReconcileOrderedHeaders` (`h2dispatch.go:612`). **The defect is DECODE-direction only, and the charter cannot silently widen to the encode side.**

### 2.4 Arm 4 — SUBJECT, H2, decode-side REMOVE — the removal is IGNORED

Config carries `- remove: "x-remove-me"`; the backend received `X-Remove-Me: please-remove` (quoted in §2.1). **Both directions of mutation are lost, not just additions.**

### 2.5 Arms 5 / 5b — REFERENCE — real Envoy applies BOTH, over H1-upstream AND H2-upstream

Arm 5 (`H2 in → H1 out`, delivery verified: backend request count moved 1 → 2):

```
X-Client: c1
X-Envoy-Expected-Rq-Timeout-Ms: 15000
X-Forwarded-Proto: http
X-Probe: seen
X-Request-Id: b89677d4-1d52-49c9-8b4a-bd38f510c6e2
```

`X-Probe: seen` present, `X-Remove-Me` absent. Arm 5b re-ran the reference with `typed_extension_protocol_options → explicit_http_config → http2_protocol_options: {}` so the reference leg was `H2 in → H2 out`, matching the subject exactly: **still applied.** ⇒ **the divergence is NOT an upstream-protocol artifact.**

### 2.6 ⚠️ Arms 6 / 7 — THE ISOLATING ARM, AND ITS NEGATIVE CONTROL

Arm 3 proves the *chain* runs but not that the *decode* chain's mutation is real. So: an H2 listener with `[header_mutation(add x-probe), rbac(ALLOW iff header x-probe == seen), router]`.

- **Arm 6 → HTTP 200**, and the backend received:
  ```
  METHOD GET PATH /arm6 PROTO HTTP/2.0 HOST 127.0.0.1:47582
  Accept: */*
  User-Agent: curl/8.5.0
  ```
  **RBAC saw `x-probe` — so the mutation IS visible to later filters in the chain — while the backend saw NO `X-Probe`.**
- **Arm 7, negative control** — same shape but header_mutation adds `x-other` while rbac still demands `x-probe` → **HTTP 403 `RBAC: access denied`**. The policy discriminates, so Arm 6's 200 is **not vacuous**.

**This localizes the defect exactly: the mutation lands in the map the chain reads, and is dropped at the boundary where the upstream request is emitted.**

### 2.7 ⚠️ A PROBE ARTIFACT, RECORDED SO IT IS NOT MISTAKEN FOR THE DEFECT

The first attempt used a plain `net/http` (H1-only) backend and the H2 arm returned **502 / `hcm: h2: EOF`** with **ZERO backend requests**. Cause: envoy-go's H2 downstream leg goes upstream over H2 unconditionally — `doH2ClusterAction` → `a.cluster.AcquireH2Stream` (`internal/filter/http/router/router_h2.go:120`) with no consultation of `Cluster.UseH2()`. Switching the backend to `h2c.NewHandler` fixed it. **That 502 is a probe artifact, not this row's defect.**

⚠️ **But it is ALSO an observation worth carrying:** it is the MIRROR of a candidate the gRPC window already names — the H1-downstream → H2-upstream 502 at `connection.go:467`, which the window attributes to *"takes the H1 action closure without consulting `Cluster.UseH2()`"*. **Recorded as an observation cross-referenced to that existing candidate; NOT chartered here and NOT claimed as a new defect** — whether the reference honours cluster protocol options in the H2-downstream direction was not measured.

---

## 3. The mechanism, stated precisely — and a correction to the framing it arrives with

⚠️ **THE TEMPTING FRAMING IS VALUE-VS-POINTER, AND CITING IT AS THE CAUSE WOULD BE WRONG.** It is true that `router.go:234 SetRequest(r *http.Request)` stores a POINTER while `router.go:246 SetH2Request(r h2.H2Request)` stores a VALUE (`router.go:211`; `:311` even comments *"h2Req is a value type"*). But an `http.Header` is a **map**, so mutations would be visible through a copy too. **The value/pointer difference is a symptom, not the cause.**

**THE ACTUAL MECHANISM IS TWO INDEPENDENT CONTAINERS WITH NO WRITE-BACK**, both built from the same `[]hpack.HeaderField` source:

- `buildH2Request` — `internal/filter/hcm/h2/stream.go:331` → `H2Request.Headers []hpack.HeaderField`, an **ORDERED SLICE**, pseudo-headers excluded at `:344`.
- `buildRequest` — `internal/filter/hcm/h2/stream.go:389` → a `*http.Request` whose `.Header` is an `http.Header` **MAP** (`regular := http.Header{}`).

The decode chain mutates the MAP. Nothing projects the map back onto the SLICE.

Ordering read at this tip in `internal/filter/hcm/h2dispatch.go`:

```
:457  rf.SetH2Request(h2req)                                    <- snapshot taken HERE
:518  chain.RunDecodeHeaders(ctx, c.req.Header, endStreamOnHeaders)
:552  chain.RunDecodeData(ctx, h2req.Body, true)
:563  rf.RunAction(ctx)                                         <- consumes the :457 snapshot
```

⚠️ **THE NAIVE FIX IS A NO-OP.** Moving `:457` below `:518` changes nothing, because `h2req.Headers` is not the container the chain mutated. **The row owes a RECONCILER.** Three sharp edges the SPEC must price:

1. **PSEUDO-HEADER POLLUTION.** `h2dispatch.go:471` / `:485` / `:493` inject `:method` / `:authority` / `:path` into `c.req.Header` as raw map keys **for filter consumption**. Their own comment states `c.req.Header` is *"decode-side only"* with *"no wire-emit path"* — **a naive map→slice projection would break exactly that safety property** and put pseudo-headers on the upstream wire, an RFC 9113 §8.3 violation. `buildH2Request` already knows to skip `name[0] == ':'` (`:344`); the reconciler must too.
2. **CASE.** `http.Header` canonicalizes to `X-Foo`; HTTP/2 requires lowercase.
3. **ORDER.** The slice preserves wire order by construction; the map has none. A full rebuild reorders every header. The safe shape is a **DELTA** against a pre-decode snapshot. ⚠️ **PRECEDENT EXISTS AND IS ALREADY IN USE ON THE ENCODE SIDE:** `filter_http.ReconcileOrderedHeaders`, called at `h2dispatch.go:612`. **This row is the decode-side mirror of an already-solved problem.**

---

## 4. Rejected alternatives — EVERY COST RE-DERIVED AT THIS TIP

Per `reference_deferred_candidate_cost_restale`, a carried cost is stale by construction. Each was re-measured at `125f0714` by the controller.

| Candidate | Re-derived at THIS tip | Verdict |
|---|---|---|
| **`SETTINGS_MAX_HEADER_LIST_SIZE` advertisement** | envoy-go sends exactly SIX settings, identical both directions (`settings.go:52-59` server, `:72-79` client); `0x6` absent from both, and absent from all three read/apply switches (`settings.go:148-164`, `conn.go:687-724`, `client.go:393-408`). ~+15-25 lines. | **REJECTED** — a fold-in, and **semantically incoherent alone**: advertising a limit that `maxHeaderBlockSize` (`conn.go:56`) does not enforce ships a wire-level lie. ⚠️ Its reference side was **NOT measured** — chartering it would need that first. |
| **`max_request_headers_kb` + `http2.header_overflow`** | Reference band pinned at `BEHAVIOR_CONTRACT.md:2038`; envoy-go answers 200 where the reference resets, pinned BY SYMBOL by `TestServerConn_Continuation_AcceptsPastReferenceLimit` (`continuation_test.go:1165`). | **REJECTED** — **breaks stat-surface +0**, needs a config field, **supersedes ADR-0041's silently-ignored set**, and **INVERTS an existing passing test**. Not small. |
| **stream-scoped parity via decode-and-discard drain** | ADR-0310 filed it "cost un-enumerated"; the drain primitive turns out to exist free — `hpack.Decoder.SetEmitEnabled` (vendored `x/net@v0.34.0/http2/hpack/hpack.go:140`) advances the dynamic table while suppressing materialization. Still ~+130-220 over BOTH legs. | **REJECTED** — largest of the five; same test inversion. ⚠️ **Its "un-enumerated" cost is now partially enumerated — record that, do not re-inherit "unknown".** |
| **`server_name` / `server_header_transformation` / `via`** | Two real anchors, one under DEFAULT config (Envoy defaults `OVERWRITE`; envoy-go appends only if absent — `codec.go:100-104`, `h2dispatch.go:742`, `h3dispatch.go:53`). 8 call sites across two packages. | **REJECTED for now** — larger, **+1 BackendKind** (no existing BackendKind emits a `Server` response header), and cross-package blast radius into the deliberately-decoupled `router`. **The strongest runner-up.** |
| **`access_log[].filter`** | Confirmed silently discarded: `parseOneAccessLog` (`bootstrap.go:1135-1174`) never calls `al.GetFilter()`; `bootstrap.go:289`/`:461` already confess it. 13 oneof arms exact (`accesslog.pb.go:421-483`). | **REJECTED for now** — ~+130-190 with 11 strict-reject arms and a global sink-flattening hazard. Cleanly splittable to 2 arms; **the best second pick.** |
| **`/stats/prometheus` projection gap** | ⚠️ **STALE — CONSUMED.** Rows **79** (`stats-prometheus-projection`) and **80** (`stats-sds-projection`) are BOTH `done`. `ExtractTags` now carries landed arms for `runtime.`/`access_logs.`/`tracing.` (`name.go:133-143`, phase 79) and `sds.` hoisting `envoy_xds_resource_name` (`:144-165`, phase 80); `WriteProm` no longer bare-returns — it collects `skipped` and logs an aggregate line (`prom.go:65`, `:82`). | **REJECTED — NO RED ANCHOR REMAINS.** The residual is a ~15-line self-consistency guard with **no divergence from real Envoy at all**. |
| **`ssl.connection_error`** | Still absent as a landed stat name: 4 comment refs in `.go` (`quic_test.go:234`, `manager.go:373`/`:411`/`:1292`) + 2 in `BEHAVIOR_CONTRACT.md`. Carried floor +444 whole-`.go`. | **REJECTED** — lands a stat NAME (breaks +0) and the floor exceeds this row's. |
| **`test/conformance/grpc/`** | `test/conformance/` still contains ONLY `doc.go`, `h2spec/`, `proxy-wasm/`. Interop client at `BOOTSTRAP_PROMPT.md:350` still does not exist. | **REJECTED** — a new subsystem; most of it foreclosed by the measured buffering ceiling. |
| **stat-surface recount** | CONTESTED, measured here BY A STATED COMMAND: `grep -ro --include='*.go' -e 'NewCounter(' -e 'NewGauge(' .` ⇒ **406** occurrences (`NewCounter(` 327 + `NewGauge(` 79); `-rn` ⇒ **404** lines; `git grep -o` tracked-only ⇒ **406**; zero untracked `.go`. **The phase-88 IMPL's 405/403 does NOT reproduce on either axis.** | **REJECTED as a row** — nothing to reproduce by execution. ⚠️ **THE COMMAND BINDS, NOT THE NUMBER.** No doc prose absolute is corrected here. |
| **REVIEW.md restoration** | **37** of **129** phase dirs, both re-measured. | **REJECTED** — process-not-product; no defect ⇒ no red anchor. |
| **D-86-CONN `client.Close` gate** | `_ = client.Close()` live and UNGATED at `internal/boot/boot.go:248` inside `NewValidateSDSProvider` (`:240`); five tests call the constructor, none asserts the Close. ~10 lines. | **REJECTED as a row** — a fold-in. Carried forward. |
| **hygiene fold-ins** | `test/differential/harness_test.go` port-inventory comment still cites `10000..10447, 15000..15011, 18001..18007` — STALE. xDS cycle guard still not automated. | **REJECTED** — fold-ins, no red anchor. |
| **split header block on a concurrently-RESET stream** | ⚠️ **REFUTED BY CODE READING — SEE §10.3. NOT A DEFECT.** | **REJECTED — there is nothing to fix.** |
| **the ~59-item family-window inventory** | Windows re-read at `:198 :204 :210 :220 :226 :234`. | **REJECTED for now** — see §5. |

---

## 5. Family attribution — and why it is CLEANER than row 88's

**Chartered as a core-HCM / HTTP-2-dispatch MAINTENANCE row claiming NO family ordinal** (the row-85 / 86 / 87 / 88 precedent: a maintenance row repairs a landed deliverable and does not extend a charter).

⚠️ **MEASURED, NOT ASSERTED: NO SENTINEL WINDOW NAMES THIS CANDIDATE.** A per-line grep across all six windows for `h2dispatch|SetH2Request|decode-side filter mutation|decode filter mutation` returns **0** at every one of `:198 :204 :210 :220 :226 :234`. Its provenance is `DECISIONS.md:18206` (ADR-0310 §Consequences (x)) and `STATE.md:18`.

**Sentinel consequences, stated as obligations rather than forecasts:**

- Unlike row 88 — whose provenance sat INSIDE the `:204` window and therefore narrowed a window at row-done — **this row narrows NO window at close.**
- The ONLY sentinel-affecting edit in this stage is the **+1 data row**, inserted at `ROADMAP.md:151`, which shifts the six window ANCHORS `+1` **without changing their COUNT or their CONTENT**.
- ⚠️ **THAT IS A PREDICTION. IT IS MEASURED ON BOTH SIDES IN §8 AND WAS NOT FORECAST.**
- ⚠️ Do NOT let this section's adjectives acquire ADR authority (`reference_brainstorm_adjective_acquires_adr_authority`): the SPEC must grep for the SENTENCE it intends to change, not inherit this framing.

---

## 6. Anticipated ADR, counts, and the cost FLOOR

- **Anticipated ADR `ADR-0311`** — **TAIL-derived** (`grep -o '^## ADR-[0-9]*' … | tail -1` → `## ADR-0310`; `grep -c '^## ADR-0311'` → **0**). ⚠️ **Headings+1 COLLIDES at the ADR-0209 gap** (headings = **309**); never derive from the count. **A BRAINSTORM ADDS NO ADR:** next-free stays `ADR-0311` and the strict `^> **STATUS: PROPOSED` guard stays **0**. **The SPEC re-arms it 0 → 1.**
- `DECISIONS.md` **18208** lines · **309** headings · `^---$` **216** · STATUS census **23**, all COMPLETE. ⚠️ **A NEW ADR TAKES NO `---` SEPARATOR.** ⚠️ **CARRY NO WHOLE-FILE COUNT OF THE LOOSE `PROPOSED` MATCHER** — it reads **1** at `DECISIONS.md:14866`, an ADR-0231 (phase-33) entry in the older non-blockquote form. **Only the strict form is the guard.**
- **Counts re-derived MECHANICALLY at this tip, each anticipated at +0 unless noted:** stat-surface **DELTA 0** · fuzzers **55 / 48** +0 · BackendKind tail **38** +0 · `go.mod` +0 · config fields **+0** · differential fixtures **121 +0 ANTICIPATED BUT NOT DECIDED** — extend `0004-h2-routing` in place vs mint `0120` is a **SPEC decision**; `0120` stays UNCONSUMED at this stage.
- **COST: ESTIMATED ~+50-90 net production `.go`**, concentrated in `h2dispatch.go` plus one helper. ⚠️ **AN ESTIMATE, NOT A MEASUREMENT.** `reference_measured_prototype_is_a_lower_bound` fired on BOTH axes at the phase-88 IMPL (+284 production against a ~+190-240 band). **THE SPEC MUST ENUMERATE BY COMPILING PROTOTYPE.** The under-enumeration risk here is concrete and named: **§7 Q3** (whether a filter may legitimately mutate `:path`/`:authority`, which would route mutations to the H2Request SCALAR fields rather than to `.Headers`) is the most likely source of hidden cost.

---

## 7. What the SPEC owes

1. **Q1 — THE RECONCILER SHAPE.** Delta-against-snapshot vs full rebuild vs teaching the decode chain to operate on the ordered slice directly. **DECIDE BY COMPILING PROTOTYPE and price all three.** Can `filter_http.ReconcileOrderedHeaders` (`h2dispatch.go:612`) be reused verbatim, or does the decode direction need its own? **ANSWER BY COMPILING IT, not by reading it.**
2. **Q2 — WHERE THE CALL MOVES.** Below `:518` only, or below `:552` (after `RunDecodeData`)? Re-check the two early-exits that bypass `RunAction`: the `RunDecodeHeaders` error return at `:522` and the `LocalReplyDone()` return at `:530`.
3. **Q3 — PSEUDO-HEADER SAFETY.** The injected keys at `:471`/`:485`/`:493` must not reach the wire. **Can a filter LEGITIMATELY mutate `:path` or `:authority`?** Envoy allows both. If yes, the reconciler must route those back to the H2Request **SCALAR fields**, not to `.Headers`. ⚠️ **This is the row's most likely hidden cost.**
4. **Q4 — THE PROOF SHAPE**, with the corpus facts MEASURED here so the SPEC need not re-derive them:
   - `codec_type` census across `test/fixtures/`: **HTTP1 270 · AUTO 6 · HTTP3 3 · HTTP2 ZERO.** Downstream H2 is reached ONLY via `codec_type: AUTO` + listener ALPN `["h2","http/1.1"]` (`0004-h2-routing/envoy-go.yaml:36` + `:48`). ⚠️ A fixture written with `codec_type: HTTP2` would be a shape the corpus has never used — the §2 probe needed `--allow-h2c` for plaintext H2 (`internal/filter/hcm/config.go:239`).
   - `header_mutation` is configured in **`0012-http-header-mutation` ONLY**, and that fixture is `codec_type: HTTP1` at BOTH listeners (`:14`, `:117`).
   - ⇒ **NO fixture exercises any decode-mutating filter over downstream H2. That is the mechanical reason this defect survived 88 phases.**
   - Most `http2_protocol_options` hits are CLUSTER-side (upstream h2) — do NOT read that grep as downstream-H2 coverage.
   - In-place `0004` extension vs a new `0120`: `0004` is TLS+h2 only, so an H1 positive-control arm needs a home. **Price both.**
5. **Q5 — THE ENCODE-SIDE BOUND.** §2.3 shows encode-side mutations DO propagate on H2. Re-confirm at the SPEC's own tip so the charter is bounded to the decode direction and cannot silently widen.
6. **Q6 — THE CONTRACT.** ⚠️ **CITE BY STRING, never by line** — the phase-88 bullets shifted every by-line cite at/below the `## HTTP/2` list by +2.
   - (a) The `envoy.filters.http.header_mutation` equivalence row (`BEHAVIOR_CONTRACT.md:32` at this tip) asserts *"Per-request equivalence on post-mutation request headers (visible at upstream backend)"* and then ends: **"NOT asserted: … `query_parameter_mutations` (deferred — ADR-0112), H2 differential coverage."** ⇒ **THERE IS NO FALSE SENTENCE TO CORRECT. There is an HONEST CARVE-OUT** — and that carve-out is exactly why the defect survived. The row's documentary deliverable is to **CLOSE** it. ⚠️ **Do NOT report this as a false-contract row.**
   - (b) `BEHAVIOR_CONTRACT.md:829` already records the MIRROR asymmetry under `### Does not yet apply to`: *"H2 decode-side observation of the injected headers (the H2 inject mutates the upstream-forwarded header set only, not the decode-side map; H1 mutates `req.Header` which is both)."* That is the TRACING-INJECT direction — slice mutated, map not — the exact inverse of this row's defect. **The SPEC must decide whether the reconciler makes (b) stale and say so IN WRITING**, rather than leaving two half-true sentences on the record.
7. **Q7 — BODY SCOPE.** `:552` passes `h2req.Body` — the same frozen value. **Is `RunDecodeData`'s body mutation ALSO dropped? Probe it.** If yes, say in writing whether it is in or out of charter.
8. **Q8 — H3, ALREADY ANSWERED HERE, RE-CONFIRM AT YOURS.** `h3dispatch.go:217` `rf.SetRequest(r)` + `:263` decode on the SAME object ⇒ **H3 does NOT carry the defect.** Keep H3 explicitly OUT of charter.
9. **Q9 — h2spec is NOT available as a red anchor** (ADR-0307, re-confirmed at the phase-88 IMPL: `95 tests, 94 passed, 1 skipped, 0 failed` identically at tip and fix). Cite only from the SPEC's OWN run.

---

## 8. Sentinel — RUN MECHANICALLY, BOTH SIDES, ACTUAL OUTPUT RECORDED

See `PROGRESS.md` §Sentinel for the recorded both-sides output. Summary: **the sentinel does NOT fire on either side; `stop` was NOT created** (verified absent at the git root). `want` moves **120 → 121 in the SAME commit** that registers row 89 — the phase-84/85/86/87/88 precedent — and the **single executable `want=` site is `next-prompt.txt:17`** (measured: every other `want=120` occurrence is historical prose in append-only phase docs, plus STATE.md's narrative line 18; `phases/77-runtime-static-layer/PLAN.md:69` carries a historical `want=109` and is NOT touched).

---

## 9. Probe hygiene

Four worktrees this stage — `wt-probe` (read-only, three sizing agents), `wt-repro` (the reproduction), `wt-89` (the stage branch), plus canonical `master`. **No agent committed; the controller squashes.** Private scratch per agent (`a1`-`a6`). Ports banded **47560-47599** (sizing 47560-47579, reproduction 47580-47599), clear of the static fixture ports (10000-19172), the subject block (20000-31007), the backend band (11000-14999) and the known receiver-race ports 35097 / 35323 / 42039. Docker containers created BY NAME with `--rm` (`repro-a6-ref`, `repro-a6-ref2`) and torn down BY NAME; `docker ps` empty afterwards. **No tracked file was patched at any point** — every config, binary and log lived in private scratch; `git status --porcelain` and `git diff --stat` both EMPTY in every probe worktree. ⚠️ **A sibling `envoy-rust` session was ACTIVE throughout and was CHECKED, NOT BLAMED**; zero containers were running at stage start, so none were torn down.

---

## 10. Findings this stage produced that the next stage must not re-learn

1. ⚠️ **THE PROBE'S FIRST BACKEND WAS THE WRONG PROTOCOL AND IT LOOKED LIKE THE DEFECT.** A plain `net/http` backend gave **502 / `hcm: h2: EOF` with ZERO backend requests** — a shape trivially mistakable for "the mutation was dropped". It was the upstream leg failing to connect at all. **`reference_iocopy_self_splice_echo_backend` and the "verify delivery ran" rule both apply: assert the backend RECEIVED something before interpreting what it received.**
2. ⚠️ **AN ENCODE-SIDE CONTROL DOES NOT PROVE THE DECODE CHAIN RAN.** Arm 3 (encode-side header present) is consistent with a decode chain that never executed. Only the Arm 6 / Arm 7 rbac pair — a later filter *observing* the mutation, plus a negative control proving the policy discriminates — establishes that the mutation exists and is dropped at the emit boundary. **A positive control on the wrong axis is not a control** (`topic_probe_discipline`).
3. ⚠️ **"GENUINELY UNPROBED" IS NOT "PROBABLY BROKEN" — one of ADR-0310's carried classes is REFUTED.** The *split header block on a concurrently-RESET stream* is **NOT a defect**: `checkFrameOrder` in the pinned `x/net@v0.34.0/http2/frame.go:543` makes any non-CONTINUATION frame (RST_STREAM included) a connection PROTOCOL_ERROR while a header block is open, and `AllowIllegalReads` is never set (`framer.go:114-121`), so the interleaving is unreachable through `ReadFrame`. The LOCAL-reset case is already handled deliberately — `client.go:604-615` decodes-and-discards with a comment saying why, and `conn.go:440` decodes BEFORE the stream-state checks at `:445-503`. **No reset path clears `hdrAccum` at any of its 8 sites.** ⚠️ **A confirming unit test would be near-worthless** — it would need `AllowIllegalReads` and would then test a wire sequence no peer can produce. **Record it as REFUTED rather than carrying it forward.**
4. ⚠️ **TWO SENTINEL-WINDOW CANDIDATES ARE STALE — the windows are not self-cleaning.** Window `:220` still lists *"a pre-existing H2 `//`-path routing bug — `h2/stream.go:440` uses `url.Parse`"*, which **row 87 CLOSED** (`internal/filter/hcm/h2/stream.go:478` now calls `url.ParseRequestURI`), and still lists the `/stats/prometheus` projection gap, which **rows 79 and 80 CONSUMED**. **A candidate's presence in a window is NOT evidence it is still open — re-derive at your tip before costing it.** ⚠️ Editing the windows is a sentinel-affecting edit; neither was touched here.
5. ⚠️ **THE DEFERRED INVENTORY IS ~59, NOT ~42.** The routers' carried "~42" is the **sentence-visible** subset — the count of items inside the six live `candidates:` sentences (43 by name-level count, 42 collapsing `/runtime` + `POST /runtime_modify` into the single clause the sentence writes). Window `:220` names **16 further un-chartered items OUTSIDE its live sentence**. **Cite ~42 only as "sentence-visible", never as the inventory.**
6. ⚠️ **THREE ROUTER FIGURES REFUTED AT THIS TIP** (all controller-measured, and `125f0714` touched ONLY `next-prompt.txt` with `numstat 1 0`, so none of these is tip drift): `DECISIONS.md` is **18208** lines, not 18206 · the contested stat-surface reads **406 occurrences / 404 lines** by the stated command, and the phase-88 IMPL's **405/403 does not reproduce on either axis** · the router's blank-import gate `grep -cP '^\t_ "'` on `test/differential/runner_test.go` reads **123**, not 121 — the two extras are non-fixture imports at `:157` (`internal/filter/http/lua`) and `:168` (`internal/filter/http/ratelimit`); **only the narrowed form `^\t_ "[^"]*test/fixtures/` reads 121**, matching the 121 fixture dirs 1:1. **The narrowed form is the gate; the router's form over-counts.**
7. ⚠️ **`SETTINGS_MAX_HEADER_LIST_SIZE` MUST NOT BE VERIFIED BY A TOKEN GREP.** A whole-file grep of that token is self-falsifying — `conn.go:46` and `continuation_test.go:557` both spell it in PROSE while the parameter is absent from every send site and every apply switch. **Verify by SYMBOL ABSENCE across the three read sites, not by token count.**
