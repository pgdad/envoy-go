# PLAN 72 — tracing `custom_tags` `metadata` type, `HOST` MetadataKind only — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL — `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Stage:** PLAN (lifecycle-state 2 → 3). Docs-only — ZERO production `.go`. Worktree `.worktrees/phase-72-plan`, branch `phase-72-tracing-custom-tags-metadata-host-plan`, tip **`2a82cc7b`** (the phase-72 SPEC squash — master), per `feedback_git_worktrees`.
>
> **Row 72 STAYS `in-progress`** — the IMPL flips it `done` at its six-gate (ADR-0106, the SOLE leg — a SINGLE FLAT ROW, §10). **ADR-0294's §Context is ALREADY DRAFTED** at the SPEC squash (`grep -n '^## ADR-0294' docs/envoy-go/DECISIONS.md`, STATUS: **PROPOSED**, §Context only, closing with a `*(§Decision + §Consequences land at the phase-72 IMPL.)*` hand-off line); the IMPL **COMPLETES ADR-0294 IN PLACE** by APPENDING `### Decision (landed at the phase-72 IMPL)` + `### Consequences` — it does NOT append a new ADR, does NOT renumber, and **must NOT create a second `### Decision` heading** (the SPEC deliberately left none — the ADR-0293 shape; SPEC §16 F1/F2). DECISIONS tail stays **ADR-0294**, next-free **ADR-0295** (`[RUN]`: `grep -c '^## ADR-0295' docs/envoy-go/DECISIONS.md` → 0). **This PLAN adds NO ADR content; DECISIONS is UNTOUCHED at the PLAN.**
>
> **Baselines RE-DERIVED at `2a82cc7b` (`[RUN]` in the worktree, NOT copied):** fixtures **117** (`ls -d test/fixtures/[0-9]*/ | wc -l`; numeric tail `0115-tracing-custom-tags-metadata-route` ⇒ next fixture `0116`; `test/fixtures/0116*` does not exist; `grep -rn '10116' test/ | wc -l` ⇒ **0**, port free) · fuzzers **55** (`grep -rn '^func Fuzz' --include='*.go' internal/ test/ | wc -l`; `internal/` 55 + `test/` 0) · BackendKind tail **38** (`H2GoawayResponder`, `test/differential/fixture/fixture.go:614`) · stat surface **1201** · DECISIONS tail **ADR-0294** (PROPOSED; next-free ADR-0295) · go.mod modules **2** (lineage figure — re-check `git diff go.mod` after tidy at T8).
>
> **Sentinel expectation:** check (1) prints `NOT DONE: row 72`; check (2) prints **3** via the full-phrase form ONLY — `grep -cE 'remaining deferred \(not-yet-chartered\) candidates:' docs/envoy-go/ROADMAP.md` (`reference_sentinel_deferred_sentence_live_vs_historical` — cite the command, never the adjective); check (3) prints `NEVER OPENED: gRPC/Runtime/WASM`. **All three RUN at THIS PLAN close** (the PLAN-close checklist in Global Constraints — NOT §Task 9, which is the IMPL's close, where check (1) instead goes SILENT because row 72 has flipped). **No deferred-sentence edit at ANY pre-IMPL stage of this row** (SPEC §12); the live Observability `candidates:` sentence rolls the `HOST` MetadataKind OUT — narrowing it to `CLUSTER`-only — at the IMPL row-done edit, NOT before (the phase-57 precedent).
>
> **⚠️ NO PARALLEL STREAM.** Master (`2a82cc7b`) IS the SPEC squash. `git diff --stat 0150b04c 2a82cc7b -- '*.go' go.mod go.sum test/` is **EMPTY** — the only delta over the SPEC's cited re-derivation tree is docs (`SPEC.md` + the ADR-0294 §Context append + STATE + the router). So the production tree is byte-identical to what the SPEC re-derived, and §1 is a full independent re-verification (which found **2 MINOR drifts and 5 new findings** — enumerated below) plus the structural decisions the SPEC delegated to the PLAN.
>
> **⚠️ RE-DERIVE, do not execute.** A PLAN is not evidence (a PLAN's cites drift; a SPEC's do too). Where this document cites, go look; where it claims control flow, walk the call graph; default to REFUTED (`feedback_brief_citations_not_evidence`, `reference_quoting_is_not_executing`). **Take every `file:line` from THIS document or from SPEC 72 — never from the phase-70/71 documents, whose cites are STALE** (SPEC §1.1: phase-71's dedup `:257-262` is `:275-279`; its `descend`/`structpbValueToString` `:100`/`:123` are `:128`/`:151`).

---

## 1. Re-derivation ledger — every SPEC §3/§9/§11 anchor re-opened at `2a82cc7b`

**All SPEC code anchors RE-DERIVED at `2a82cc7b` by TWO independent read-only re-derivation agents on disjoint remits** — agent A over `internal/tracing/{config.go,resolve.go,config_test.go,resolve_test.go}` + the go-control-plane `custom_tag.pb.go`/`metadata.pb.go`/`base.pb.go`/`struct.pb.go` proto shapes + the collision greps + an EXECUTED `structpb.NewStructValue(nil)` probe; agent B over `internal/cluster/{cluster.go,manager.go,subset.go,outlier_test.go}` + `internal/filter/hcm/{accesslog_emit.go,connection.go,h2dispatch.go,h3dispatch.go,fuzz_test.go}` + the emit test callers + the `0115`/`0064` fixture chassis + the counts.

**RESULT: 68 of 69 anchors EXACT-HOLD. ONE MINOR drift (`RD-LIT`, a count; not design-blocking). ZERO SPEC claims refuted. FIVE new findings — three of them UNNAMED EDIT-SITE OBLIGATIONS the SPEC does not scope.** The SPEC's cites are adopted verbatim except where `RD-LIT` corrects them. ⚠️ **A second "drift" (`RD-FUZZ`) was recorded in an earlier draft and REFUTED by this PLAN's own adversarial pass** — the SPEC's `fuzz_test.go:92-93` was right and the "correction" to `:93-94` was wrong; it is restored to EXACT-HOLD, and the episode is retained in the ledger as a standing caution that a PLAN's corrections are not evidence either. All four identifier collision greps are 0 at tip. `go build ./...` EXIT 0; the layering check `go list -deps ./internal/tracing | grep -E 'internal/(filter|cluster)'` is EMPTY.

### 1.1 The RD-* ledger

| # | Anchor / SPEC claim | RE-DERIVED at `2a82cc7b` | Where |
|---|---|---|---|
| **RD-EXACT** | SPEC §3/§9/§11 cite ~69 code anchors | **68 / 69 EXACT-HOLD.** Agent A: 21/21 items, 63/63 discrete `file:line` cites exact, ZERO line drift. Agent B: 47/48 exact. **ONE** MINOR drift total (**RD-LIT**) — RD-FUZZ was initially recorded as a second drift and adversarial verification REFUTED that, restoring it to EXACT-HOLD. The SPEC's cites are adopted verbatim apart from RD-LIT. | all |
| **RD-KIND** | SPEC §3.2: APPEND `kindMetadataHost` AFTER `kindMetadataRoute` ⇒ iota == 5 | **CONFIRMED.** `customTagKind` block `config.go:54-60` (`:54` `const (`, `:60` `)`) = `kindLiteral`(0,`:55`) / `kindRequestHeader`(1,`:56`) / `kindEnvironment`(2,`:57`) / `kindMetadata`(3,`:58`) / `kindMetadataRoute`(4,`:59`). `kindMetadataRoute` IS last ⇒ an APPENDED constant lands at **iota == 5**. `CustomTagSpec` `:67-77` has all 8 fields incl. `MetaNamespace` `:73` / `MetaPath` `:74` / `DefaultValue` `:75` / `HasDefault` `:76` ⇒ **NO new field.** **APPEND, never INSERT** before `kindMetadataRoute` — a renumber recompiles silently (SPEC §3.2: hygiene, not a live hazard). | T2 |
| **RD-REJECT** | SPEC §3.2/§3.6: REPLACE the HOST reject (`:267-268`) with an accept arm cloning ROUTE (`:248-264`) | **CONFIRMED — all six arms EXACT with verbatim strings.** `case ct.GetMetadata() != nil:` `:225`; `md := ct.GetMetadata()` `:226`; **`k := md.GetKind()` `:227`** (its own landed comment names the V1 SEVERE); unset-A `:229-230` `"kind required"`; REQUEST accept `:231-247`; **ROUTE accept `:248-264`** (the CLONE SOURCE — §1.2); CLUSTER reject `:265-266` `"cluster kind unsupported"`; **HOST reject `:267-268`** `case k.GetHost() != nil:` → `"tracing: custom_tags metadata tag %q host kind unsupported"` (the arm REPLACED); `default:` unset-B `:269-270` `"kind required"`; `:271` closes the inner switch. Empty-tag reject **`:201-203`** and first-wins dedup **`:275-279`** are structurally independent and UNCHANGED. All retained reject substrings ADR-0080-distinct. | T2 |
| **RD-NOIMPORT-CFG** | SPEC §3.2/§4: config.go adds ZERO imports | **CONFIRMED.** Import block `:7-16` = `fmt` · `tracev3` · `hcmv3` · `tracingv3` · `typev3` · `proto` · `anypb`. **`metadatav3` is NOT imported and is not needed** — `k` is INFERRED from `md.GetKind()` (never spelled), and the new arm calls `k.GetHost()` on that inferred `k`, exactly as the four landed arms do. (The `*metadatav3.MetadataKind` at `:227` is a COMMENT, not a type reference.) | T2 |
| **RD-PROTO** | SPEC §3.1: the kind-getters are on `*metadatav3.MetadataKind`; `md.GetHost()` does not compile | **CONFIRMED — the phase-70 V1 SEVERE re-established by exhaustive method listing.** Module `go-control-plane/envoy@v1.32.4`. `*CustomTag_Metadata` (`type/tracing/v3/custom_tag.pb.go:326-339`) has **exactly six methods**: `Reset` `:341`, `String` `:350`, `ProtoReflect` `:356`, **`GetKind()` `:373`, `GetMetadataKey()` `:380`, `GetDefaultValue()` `:387` — and NOTHING else. There is NO `GetHost()` on `*CustomTag_Metadata`.** On `*MetadataKind` (`type/metadata/v3/metadata.pb.go`): `GetRequest()` `:164` / `GetRoute()` `:171` / `GetCluster()` `:178` / **`GetHost()` `:185`**; `MetadataKind_Host_` wrapper struct at `:211`. `corev3.Metadata.FilterMetadata map[string]*structpb.Struct` `base.pb.go:819` (sibling `TypedFilterMetadata` `:826` — unaddressable by a `MetadataKey`); `GetFilterMetadata()` `:861-866` is **nil-receiver-safe** (`if x != nil` → else nil map), and a nil-map index safely yields a nil `*structpb.Struct`. `structpb.NewStructValue` `struct.pb.go:396-398` = `func(v *Struct) *Value`. | T1, T2 |
| **RD-RESOLVE-5TH** | SPEC §3.3: `ResolveCustomTags` grows a FIFTH nil-tolerant one-arg `hostMetaLookup` | **CONFIRMED — the 4-param signature is exact at `resolve.go:32`:** `func ResolveCustomTags(specs []CustomTagSpec, headerLookup func(string) ([]string, bool), metaLookup func(ns, key string) (*structpb.Value, bool), routeMetaLookup func(ns string) (*structpb.Value, bool)) []KV`. Phase 72 APPENDS a 5th `hostMetaLookup func(ns string) (*structpb.Value, bool)` — shaped identically to `routeMetaLookup` (ONE-arg). The 3rd `metaLookup` and 4th `routeMetaLookup` are UNTOUCHED. The doc block **`:12-31`** documents `routeMetaLookup` at **`:24-29`** — that is the exact paragraph the HOST clause mirrors, and it is a GENUINE edit site (correctly cited by SPEC §11). | T3 |
| **RD-HOST-ARM** | SPEC §3.3: a `kindMetadataHost` arm descending the FULL `MetaPath` (NOT `[1:]`) | **CONFIRMED — the clone source is `resolve.go:95-116`** (the `kindMetadataRoute` arm; `:117` closes the `switch`, `:118` the `for` — the SPEC §1.1 cosmetic correction to `:95-116` is itself CORRECT). Quoted verbatim at §1.2. The REQUEST arm is `:71-94` with its `[1:]` slice at **`:84`** — REQUEST-only, a Bucket-pre-keying artifact (`reference_route_metadata_resolve_full_metapath`). The HOST source, like ROUTE, yields the WHOLE namespace struct ⇒ **descend the FULL `MetaPath`**. | T3 |
| **RD-DESCEND / RD-SERIALIZE** | SPEC §3.3: both reusable VERBATIM | **CONFIRMED.** `descend` doc `:122-127`, body **`:128-142`** — `func descend(v *structpb.Value, segs []string) (*structpb.Value, bool)`; per-segment `cur.GetStructValue()`; nil struct / missing field / nil field → `(nil,false)`; empty `segs` → `(cur,true)`. **Kind-agnostic and source-shape-independent** ⇒ reused UNCHANGED. `structpbValueToString` doc `:144-150`, body **`:151-168`** — `Value_StringValue`→raw incl. `""`; `Value_NullValue`→`("",false)`; else `protojson.Marshal` + `json.Compact` (**`json.Compact` LOAD-BEARING** — strips detrand whitespace; `reference_detrand_race_catches_protojson_value_substring`). resolve.go imports `:3-10` incl. **`structpb` `:9`** ⇒ the new param type needs **NO new import**. | T3 |
| **RD-DEFAULT** | SPEC §3.4: the `request_header` rule (present-empty EMITS `""`) | **CONFIRMED** — the HOST arm clones the ROUTE arm's `if s.HasDefault { … } // else omit` tail, NOT the `kindEnvironment` omit-on-empty rule. `HasDefault = DefaultValue != ""` at parse (T2). Resolve-if-resolved (present-empty `""` → emits `""`); else `default_value` if non-empty; else omit. EXECUTED-pinned reference-side at P4. | T3 |
| **RD-ENDPOINT** | SPEC §3.5/S1: ADD `filterMetadata map[string]*structpb.Struct` + a nil-safe `MetaLookup` to `cluster.Endpoint` | **CONFIRMED — the struct is `cluster.go:43-75`.** Fields: `Host` `:44`, `Port` `:45`, **`Metadata map[string]SubsetValue` `:49`** (the phase-38 `envoy.lb` scalars-only projection, comment `:46-48`), `Locality` `:54`, `LocalityWeight` `:60`, `Priority` `:68`, unexported `addr` `:74` (comment `:70-73`). `Addr()` **`:81-86`** (returns the precomputed cache). Non-comparability doc comment **`:88-91`**. `IsZero()` **`:92`**. `PooledH1Conn.Endpoint()` **`:106`**. **cluster.go's import block `:3-22` contains NEITHER `corev3` NOR `structpb`** (`grep -n 'structpb\|corev3'` ⇒ 0) ⇒ **`structpb` is genuinely the row's ONE new import here.** | T1 |
| **RD-POPULATE** | SPEC §11: `manager.go:884` the populate; `:883` byte-unchanged | **CONFIRMED — and the SPEC §11 PHRASING is the correct one; do not paraphrase it.** `:883` `scalars, _ := ScalarsFromStruct(lbe.GetMetadata().GetFilterMetadata()["envoy.lb"])` — computes `scalars` ONLY. **`:884` `e := Endpoint{Host: …, Port: …, Metadata: scalars, Locality: loc, LocalityWeight: weight, Priority: priority}`** — the composite literal, where the `filterMetadata:` key is ADDED. `:885` `e.addr = e.Addr()` precompute; `:886` append. ⚠️ **The `Metadata:` FIELD is on `:884`, not `:883`** — so `:883` stays byte-unchanged and **`:884` MUST change**. `defaultSubset` **`manager.go:754`** stays byte-unchanged. | T1 |
| **RD-NORIPPLE** | SPEC §3.5: `Endpoint` is never a map key, never `==`-compared, never serialized; one non-empty production literal | **CONFIRMED on every leg, by grep.** `grep -rn 'json:"' internal/cluster/` ⇒ **0** (never serialized). `grep -rnE 'map\[(cluster\.)?Endpoint\]' --include='*.go'` ⇒ **0** (never a map key). ⚠️ **Scope the grep with `--include='*.go'`** — unscoped it returns 1 hit, `docs/envoy-go/phases/36-load-balancer-ring-hash/PLAN.md:405`, a DOC. `!=`/`==` `Endpoint{}` comparison text repo-wide ⇒ **exactly ONE hit, `cluster.go:91`, the DOC COMMENT** (the only `== ep`-shaped hit is `manager.go:872` `if ep == nil {`, a *proto* pointer, not a `cluster.Endpoint`). Non-empty KEYED `Endpoint{…}` in production ⇒ **exactly ONE, `manager.go:884`.** `Endpoint.Metadata` production consumers ⇒ **exactly TWO, `subset.go:163` + `:250`.** `ScalarsFromStruct` (`subset.go:263-285`, doc `:258-262`, drop set the `default:` arm **`:277-279`** covering `StructValue`/`ListValue`/`NullValue`/nil) has **exactly FIVE production callers** (`hcm/config.go:977`/`:993`/`:997`, `manager.go:754`/`:883`). All four BRAINSTORM §2.5 residual risks NEGATIVE. | T1, T8 |
| **RD-LIT** ⚠️ | SPEC §3.5: "all **167** test literals are keyed" | **DRIFT (MINOR, count) — NO figure reproduces; the CLAIM survives intact.** Two independent agents measured and DISAGREED with each other as well as with the SPEC (177 lines / 209 occurrences vs 177 lines / 177 occurrences, depending on the grep; plain `Endpoint{` gives 229 lines). ⇒ **state the claim, cite NO number** — 167, 177 and 209 are all unreproducible artifacts of a particular grep. **The load-bearing part is CONFIRMED TWO independent ways: there is NOT ONE positional/unkeyed `Endpoint` literal anywhere in tests** — (i) bare `Endpoint{` not followed by `Ident:` yields a single hit, `manager_test.go:1508`, which is a **format string** (`t.Errorf("Endpoint{%q, %d}.Addr() = …")`), not a literal; (ii) elided slice elements `[]Endpoint{{` not followed by `Ident:` ⇒ **zero hits**. ⇒ adding a field is **zero-positional-breakage**. **PLAN ACTION: state the claim, not the number** — "every test literal is keyed (verified by exhaustive positional-literal grep)". Do NOT copy 167. Also informational: the SPEC's "35 production zero-value `Endpoint{}`" over-counts by at least TWO — `cluster.go:91` is the doc comment and `subset.go:161` is a `map[string][]Endpoint{}` MAP literal ⇒ **≤33 real**. Immaterial to the conclusion. | T1 |
| **RD-SEAM** | SPEC §3.3/S2: the closure is built LOCALLY inside each emit helper from the in-scope `picked`; signatures + 18 callers BYTE-UNTOUCHED | **CONFIRMED — the row's central cost claim is structurally sound.** The three emit methods are **9-parameter** with **`picked cluster.Endpoint` as the 4th** in each: `emitAccessLog` **`:27`**, `emitAccessLogH2` **`:87`**, `emitAccessLogH3` **`:149`**. The three `ResolveCustomTags(` calls sit INSIDE those bodies at **`:57`/`:118`/`:179`**, all currently **4-arg** (`f.tracingConfig.CustomTags, reqHeaderLookup*, metaLookup, routeMetaLookup`). `picked` is a live in-scope local throughout each body — proven by its use BELOW the call site at `upstreamHostString(picked)` **`:72`/`:133`/`:194`** (the SOLE current consumer of `picked`, exhaustive-grep-confirmed; `upstreamHostString` declared `:277`). ⇒ appending `picked.MetaLookup` as a 5th argument needs **NO signature change and NO caller change**. Span gates: **`:30`** (H1), **`:89`** (H2), **`:151`** (H3), all byte-identical `if statusCode != 0 && f.exporter != nil && traceDecision != nil && traceDecision.Sample {`. `UpstreamCluster: ""` at `:51` ("not available at this seam"). **`accesslog_emit.go` ALREADY imports `structpb` at `:9`** (block `:3-16`; it also already imports `internal/cluster` at `:12`) ⇒ **NO new import** (the S2 premise-(a) correction CONFIRMED). | T4 |
| **RD-CALLERS** | SPEC §11: 18 emit callers, 5+6+7, BYTE-UNTOUCHED | **CONFIRMED — all 18 line numbers EXACT, with their `picked` arguments.** `connection.go` (5): `:330` `Endpoint{}` · `:464` `Endpoint{}` · `:597` `Endpoint{}` · `:699` `Endpoint{}` · **`:777` `picked`**. `h2dispatch.go` (6): `:313` `picked` (**provably always the zero literal** — the site is inside `if c.routeIdx < 0 {` `:308`, `picked` comes from `c.action(ctx, h2req)` `:309`, and the no-match action is `directResponseAction.asRouterActionH2()` returning `cluster.Endpoint{}` at **`actions.go:169`**) · `:396` `Endpoint{}` · `:530` `Endpoint{}` · **`:577`/`:584`/`:613` `picked`**. `h3dispatch.go` (7): `:130` `Endpoint{}` · `:210` `Endpoint{}` · `:280` `Endpoint{}` · `:341` `Endpoint{}` · **`:367`/`:373`/`:395` `picked`**. **These 18 sites are NOT edited by this row** — verify mechanically at T8. | T4, T8 |
| **RD-SPANCAP** | SPEC §11: 6 PRE-Decide + 12 span-capable (7 real-`picked` + 5 zero) | **CONFIRMED — re-derived independently.** `traceDecision` assignments: `connection.go:553` `traceDecision = &d` · `h2dispatch.go:449` `c.traceDecision = &d` · `h3dispatch.go:255` `traceDecision = &d`. **PRE-Decide (6, can NEVER emit a span):** `connection.go:330`/`:464`, `h2dispatch.go:313`/`:396`, `h3dispatch.go:130`/`:210` — the two h2 sites carry their OWN landed corroborating comments (`:306-307` "c.traceDecision is nil here (PRE-Decide … ) → no span for a 404 no-match"; `:393-394` same for the synthetic 500), which is what makes the claim survive the stricter reading that `c.traceDecision` is a FIELD (call-order, not line-order). **Span-capable (12):** **5 zero-endpoint** (`connection.go:597`/`:699`, `h2dispatch.go:530`, `h3dispatch.go:280`/`:341` — all post-Decode local replies) + **7 real-`picked`** (`connection.go:777`, `h2dispatch.go:577`/`:584`/`:613`, `h3dispatch.go:367`/`:373`/`:395`). ⚠️ **T4's zero-`picked` test targets the 5, NOT the raw 11-of-18** (which counts 6 sites that can never emit a span). | T4 |
| **RD-TESTCALLERS** | SPEC §10 T3: 32 real `resolve_test.go` callers (raw grep 33; `:106` a string literal) | **CONFIRMED EXACTLY.** `grep -c 'ResolveCustomTags(' internal/tracing/resolve_test.go` ⇒ **33**. **`:106` is a STRING LITERAL** inside `t.Errorf("ResolveCustomTags(nil, ...) = %+v, want nil", got)` (the real call is on `:105`) ⇒ **32 real callers**, at lines **34, 72, 97, 105, 124, 131, 138, 146, 188, 198, 207, 216, 244, 254, 268, 276, 284, 292, 304, 311, 340, 349, 358, 367, 403, 416, 432, 441, 449, 457, 469, 476** — all 4-arg today; each gains a trailing `nil`. (Argument distribution today, MEASURED over the 32: 3rd arg = 22 `nil` / 10 `ml`; 4th arg = 22 `nil` / 10 `rl`.) **Re-grep at the IMPL tip** — a rename surfaces as a build error. | T3 |
| **RD-EMITTESTS** | SPEC §1/§16: 29 emit test callers BYTE-UNTOUCHED | **CONFIRMED — and the consequence verified, not just asserted.** `accesslog_emit_test.go` **13** + `span_emit_test.go` **16** = **29**, zero of them comment lines. **All 29 pass exactly 9 positional arguments** matching the current 9-param signature (e.g. `accesslog_emit_test.go:29` `f.emitAccessLog(req, 200, 3, cluster.Endpoint{}, start, nil, nil, nil, nil)`; `span_emit_test.go:117` `f.emitAccessLog(req, 200, 100, cluster.Endpoint{}, start, nil, d, nil, nil)`). Since the row adds NO parameter, **these 29 call sites are genuinely untouched.** ⚠️ **PRECISION:** T4 ADDS new test FUNCTIONS to `span_emit_test.go`, so the FILE is an edit site — the claim is that the **29 existing callers** are byte-stable, not that the file is. Two grep false positives are NOT emit callers: `mongoproxy/filter_test.go:409` (a different method) and `router/router_test.go:290` (a comment). ⚠️ **A NAMED SPEC-ROSTER DEPARTURE:** SPEC §11 lists `accesslog_emit_test.go` as a test EDIT site. That is inherited from phases 70/71, where the emit ARITY changed and every caller had to be touched. **Here it is REFUTED** — the row adds no parameter, so its 13 callers need no edit, and this PLAN instead asserts the file **BYTE-UNTOUCHED** at T8's envelope audit and hash-checks it at T4 Step 4. Stated explicitly rather than silently inverted: a SPEC edit site is being turned into a GATE, which is the strongest possible form of the opposite claim. | T4, T8 |
| **RD-FUZZ** | SPEC §7/S6: ADD `meta_host_ok`; `meta_bad` STAYS on CLUSTER; narrow the stale `:92-93` comment | **EXACT-HOLD on every leg, INCLUDING the comment anchor.** `withMetaTags` **`fuzz_test.go:97-115`** (`f.Add` at `:116`): `meta_ok` REQUEST `:100-104` · `meta_route_ok` ROUTE `:105-109` · **`meta_bad` CLUSTER `:110-112`** (MetadataKey-less) ⇒ **NO repoint needed**, and **NO seed points at HOST** (`MetadataKind_Host_` absent from the file) ⇒ the arm this row removes is unexercised by any seed. Wrapper spellings to copy: `&metadatav3.MetadataKind{Kind: &metadatav3.MetadataKind_Host_{Host: &metadatav3.MetadataKind_Host{}}}`; `MetadataKey` + `MetadataKey_PathSegment` + `MetadataKey_PathSegment_Key` all already used. **`metadatav3` IS imported at `fuzz_test.go:13`** ⇒ zero new imports. **The stale sentence spans `:92-93` exactly as the SPEC says** — token `CLUSTER/HOST` on **`:92`**, close `departures, ADR-0080).` on **`:93`** (`grep -n 'CLUSTER/HOST' internal/filter/hcm/fuzz_test.go` ⇒ `92:`). ⚠️ **An earlier draft of this PLAN "corrected" this to `:93-94`; that correction was ITSELF WRONG and was caught by adversarial verification** — `:93-94` contains no `CLUSTER/HOST` token at all, so an `old_string` built on it would have missed the stale claim and garbled the (correct) dispatch-order sentence. **Use `:92-93`.** Count STAYS **55** (a seed is not `^func Fuzz`). | T5 |
| **RD-DISPATCH** | SPEC §7: the fuzz seed reaches the metadata arm before the provider check | **CONFIRMED.** `customTags, err := parseCustomTags(t.GetCustomTags())` at **`config.go:128`**; `return nil, fmt.Errorf("tracing: provider required")` at **`:138`**. **128 < 138** ⇒ a well-formed HCM tracing block reaches the metadata arm BEFORE the provider check. Dispatch-verify at T5 anyway (`reference_probe_must_discriminate`). | T5 |
| **RD-FIXTURE** | SPEC §8: clone the `0115` chassis; move the metadata block onto `lb_endpoints[0]`; flip both `kind:` to `host: {}` | **CONFIRMED — chassis re-derived in full.** `0115-tracing-custom-tags-metadata-route/` = exactly **5 files**: `driver/driver.go` (730 lines), `envoy.yaml` (136), `envoy-go.yaml` (130), `expectations.yaml` (134), `README.md` — **NO `scripts/`**. Route metadata block: `envoy.yaml:92-95` / `envoy-go.yaml:91-94`, byte-identical (`filter_metadata: envoy.test: route_k: v-route-0115`). The two `custom_tags`: `envoy.yaml:66-84` / `envoy-go.yaml:65-83` — `route_hit` (path `route_k`, `default_value: unused-default-0115`) + `route_default` (path `absent_k`, `default_value: fallback-0115`). **Clusters — the asymmetry CONFIRMED:** reference `envoy.yaml:105-116` `type: STRICT_DNS` + `dns_lookup_family: V4_ONLY` + `{{.BackendHost}}/{{.BackendPort}}`; subject `envoy-go.yaml:103-112` `type: STATIC` + `127.0.0.1`. **Both are ALREADY single-endpoint** (exactly one `- endpoint:` under `lb_endpoints:` each) ⇒ deterministic pick, no LB spread (`reference_round_robin_offset_randomized`). **The exact insertion point for `metadata:`** — a SIBLING of `endpoint:` at the same 16-space indent: `envoy.yaml` after `:116` (the `endpoint:` key is `:114`); `envoy-go.yaml` after `:112` (`endpoint:` at `:110`). All ports templated except the reference listener `refListenerPort = 10115` (`driver.go:93`). | T6 |
| **RD-DRIVER** | SPEC §8: cross-side EXACT key+value, key-based | **CONFIRMED — the accessor API is already key-based.** `spanAttrMap(sp *tracepb.Span) map[string]*commonpb.AnyValue` **`driver.go:576-582`** (a key→AnyValue MAP — inherently order-free, matching P2's "intra-tag order is internal"). `assertAttrPresent` **`:585-591`** (Fatalf on miss). `assertAttrString` **`:594-605`** (reads `GetStringValue()`, Fatalf on mismatch). `assertRouteTags(t, side, spans)` asserts per-span with **`t.Errorf` per tag** (`reference_fatalf_makes_assertions_unreachable`-compliant), invoked once per side at `AssertStats` **`:432-433`**. `BackendCount() → 1` **`:224`** (`reference_differential_backendcount_min_one`), `BackendKind() → fixture.HTTPFixedBody` **`:225`**, `SubjectListenerName() "l_test"` `:226`, self-registration in `init()` `:161-163`. Also present: `pollSpanCount` `:358` (poll-to-converge, NO sleep) + `scrapeFlatStats` for `tracing.opentelemetry.spans_sent/dropped`. `expectations.yaml` is prose-only (ADR-0019), tag assertions documented `:93-105`. | T6 |
| **RD-0064** ⚠️ | SPEC §8: `0064-lb-subset` already puts `lb_endpoints[].metadata` on a STRICT_DNS reference cluster vs a STATIC subject | **CONFIRMED on substance — with ONE structural correction the PLAN must carry.** ⚠️ **`0064-lb-subset` has NO `envoy.yaml`/`envoy-go.yaml`** — it is a **driver-GENERATED fixture with inline YAML string literals**; its files are `driver/driver.go`, `driver/driver_test.go`, `expectations.yaml`, `README.md`. **Any instruction to "open `0064-lb-subset/envoy.yaml`" will FAIL.** The evidence is at `driver.go:206-226` (reference: `type: STRICT_DNS`, `host.docker.internal`, `metadata: { filter_metadata: { "envoy.lb": { version: "v1" } } }` as a sibling of `endpoint:`) and `driver.go:264-283` (subject: `type: STATIC`, `127.0.0.1`, same metadata shape). ⚠️ **Second precision: the `0064` precedent uses the `envoy.lb` namespace ONLY.** It does NOT precedent-test an ARBITRARY namespace cross-side — namespace-generality rests on **P2's live probe**, not on this landed fixture. Do not over-cite `0064`. | T6 |
| **RD-REGISTER** | SPEC §8: a one-line blank-import registration | **CONFIRMED.** `test/differential/runner_test.go:142` `_ "github.com/pgdad/envoy-go/test/fixtures/0115-tracing-custom-tags-metadata-route/driver"`, preceded by `:141` (`0114`) and `:140` (`0113`) ⇒ **`0116`'s line goes at `:143`**, immediately after `:142`, in the same alphanumeric-ordered block. | T6 |
| **RD-IDENT** | SPEC §5: collision checks | **ALL FREE at `2a82cc7b`** (`--include='*.go' internal/ test/ validate/ cmd/`): `kindMetadataHost` ⇒ **0** · `hostMetaLookup` ⇒ **0** · `filterMetadata` ⇒ **0** *(stronger than the SPEC's "as an `Endpoint` field" — it is 0 repo-wide; `GetFilterMetadata` has a capital F and does not match)* · word-boundary `grep -rnE '(^\|[^A-Za-z])MetaLookup'` ⇒ **0**. ⚠️ The naive substring `grep MetaLookup` returns **51**, decomposing EXACTLY as 22 `routeMetaLookup` + 27 `RouteMetaLookup` + 2 `TestFilterChain_RouteMetaLookup` — **all Route-prefixed, zero genuine collisions** (`reference_spec_drafted_identifier_collision_check`). Re-run all four at the IMPL tip. | T1, T2, T3 |
| **RD-MOD** | SPEC §4: +0 go.mod modules | **CONFIRMED buildable.** The ONLY new import repo-wide is **`structpb` into `internal/cluster/cluster.go`** — `google.golang.org/protobuf` is an existing direct dependency already imported by `internal/tracing/resolve.go:9` and `internal/filter/hcm/accesslog_emit.go:9`. `config.go`, `resolve.go` and `accesslog_emit.go` add **NONE**. `go build ./...` EXIT 0 at tip. `go mod tidy -diff` anticipated EMPTY — re-check `git diff go.mod` after tidy at T8 (`reference_new_subpackage_pulls_transitive_module`). | T1, T8 |
| **RD-LAYERING** ⚠️ | SPEC §3.3/§4: `internal/tracing` stays filter-free AND cluster-free | **CONFIRMED CLEAN — and the JUSTIFICATION correction re-verified INDEPENDENTLY.** `go list -deps ./internal/tracing | grep -E 'envoy-go/internal/(filter\|cluster)'` ⇒ **EMPTY** (the package's only internal dep is `internal/stats`). **The closure is a self-imposed LAYERING rule, NOT a cycle guard** — `go list -deps ./internal/cluster | grep 'envoy-go/internal/tracing'` ⇒ **EMPTY**, so `internal/cluster` does not depend on `internal/tracing` and a direct import would NOT cycle. This is SPEC §16's V1 finding **M3**, re-executed here. ⚠️ **CARRY THIS TO T9:** the landed **ADR-0294 §Context still says the closure is "mandatory rather than stylistic, since `internal/tracing` must not import `internal/cluster` (the cycle guard)"** — a claim its own SPEC refuted. The IMPL must NOT propagate it into §Decision, and should correct it (§Task 9). | T8, T9 |
| **RD-RACEFLAKE** | SPEC §10/S7: a pre-existing `internal/cluster` `-race` flake | **CONFIRMED PRESENT.** `TestOutlierDetector_ConcurrentEjectExactlyOnce` at `internal/cluster/outlier_test.go:744`, spanning **`:744-768`**, flaky assertion at **`:766`** — `t.Errorf("ejections_enforced_total = %d, want exactly 1", got)`. Reproduced by the SPEC on the UNMODIFIED baseline (6 failures at `-count=400`). `internal/cluster` is newly touched by this row ⇒ its `-race` leg is newly load-bearing at T8. **Isolate-re-run; do NOT re-classify as a phase-72 regression** unless deterministic or the text differs. | T8 |
| **RD-BASELINE** | SPEC §15 counts | **ALL CONFIRMED at tip:** fixtures **117** (tail `0115-…`; `0116` absent; port `10116` ⇒ 0 hits) · fuzzers **55** · BackendKind tail **38** (`fixture.go:614` `H2GoawayResponder BackendKind = 38`) · DECISIONS tail **ADR-0294** PROPOSED (next-free ADR-0295, `grep -c` ⇒ 0) · `go build ./...` EXIT 0 · `go test ./internal/tracing/ -count=1` **ok** (so the two `/host` subtests are genuinely the ONLY pre-known red set for T2). | T8 |

### 1.2 The clone skeletons the SPEC delegated to the PLAN (each RE-DERIVED verbatim at `2a82cc7b`, not invented)

> ⚠️ **GOFMT TRAP (finding F5a).** SPEC §3.2's quoted HOST skeleton compresses the three guards onto single lines (`if mk.GetKey() == "" { return nil, fmt.Errorf(...) }`). **That is NOT gofmt-clean** — `gofmt` expands a single-line `if b { return … }` into a three-line block, so pasting the SPEC's form fires T2's `gofmt -l` gate. The forms below are the **byte-accurate landed shapes**, copied from the clone sources at tip. Use THESE.

- **The `parseCustomTags` HOST arm (T2, `config.go`)** — REPLACES the `:267-268` `case k.GetHost() != nil:` reject; clones the LANDED ROUTE accept (`:248-264`) verbatim except `Kind: kindMetadataHost`. Indentation is **three tabs** for the `case`, four for its body (nested `for` inside `switch` inside `for` inside `switch`):
  ```go
  			case k.GetHost() != nil:
  				mk := md.GetMetadataKey()
  				if mk.GetKey() == "" {
  					return nil, fmt.Errorf("tracing: custom_tags metadata tag %q empty namespace", tag)
  				}
  				if len(mk.GetPath()) == 0 {
  					return nil, fmt.Errorf("tracing: custom_tags metadata tag %q empty path", tag)
  				}
  				path := make([]string, 0, len(mk.GetPath()))
  				for _, seg := range mk.GetPath() {
  					if seg.GetKey() == "" {
  						return nil, fmt.Errorf("tracing: custom_tags metadata tag %q empty path segment", tag)
  					}
  					path = append(path, seg.GetKey())
  				}
  				dv := md.GetDefaultValue()
  				spec = CustomTagSpec{Key: tag, Kind: kindMetadataHost, MetaNamespace: mk.GetKey(), MetaPath: path, DefaultValue: dv, HasDefault: dv != ""}
  ```
  **IDENTICAL to the ROUTE arm except `Kind: kindMetadataHost`** — `MetaPath` carries the FULL path (no `[1:]`; that is a REQUEST-only RESOLVE-time artifact, not a parse one). The `k := md.GetKind()` bind (`:227`), unset-A (`:229-230`), the REQUEST accept (`:231-247`), the ROUTE accept (`:248-264`), CLUSTER (`:265-266`), `default:` unset-B (`:269-270`), the empty-tag reject (`:201-203`) and the first-wins dedup (`:275-279`) are ALL UNCHANGED. **⚠️ The four kind-getters live on `*metadatav3.MetadataKind` (returned by `md.GetKind()`), NOT on `*tracingv3.CustomTag_Metadata` — whose ONLY getters are `GetKind`/`GetMetadataKey`/`GetDefaultValue` (RD-PROTO, exhaustively listed). The arm branches on `k.GetHost()`, and `k` is already bound. config.go adds NO import (RD-NOIMPORT-CFG).**

- **The `kindMetadataHost` resolve arm (T3, `resolve.go`)** — clones the LANDED `kindMetadataRoute` arm (`:95-116`) verbatim except the case label, the lookup name and the comment. Indentation is **two tabs** for the `case`, three for the body:
  ```go
  		case kindMetadataHost:
  			// Mirror kindMetadataRoute exactly: the HOST source yields the WHOLE
  			// namespace struct (the selected upstream endpoint's
  			// lb_endpoints[].metadata.filter_metadata[ns]), so descend the FULL
  			// MetaPath (the [1:] slice is a REQUEST-only Bucket-pre-keying artifact).
  			// hostMetaLookup may be nil, and picked may be the ZERO Endpoint (the
  			// 5 span-capable local-reply sites) → default / omit.
  			var v *structpb.Value
  			var ok bool
  			if hostMetaLookup != nil {
  				v, ok = hostMetaLookup(s.MetaNamespace)
  			}
  			if ok {
  				v, ok = descend(v, s.MetaPath) // FULL path, NOT s.MetaPath[1:]
  			}
  			if ok {
  				if str, emit := structpbValueToString(v); emit {
  					out = append(out, KV{Key: s.Key, Str: str})
  					continue
  				}
  			}
  			if s.HasDefault {
  				out = append(out, KV{Key: s.Key, Str: s.DefaultValue})
  			} // else omit (append nothing)
  ```
  `descend`/`structpbValueToString` REUSED VERBATIM (RD-DESCEND/RD-SERIALIZE). **No new import in resolve.go** (`structpb` at `:9`).

- **The `ResolveCustomTags` signature growth (T3, `resolve.go:32`)** — a FIFTH param, appended after `routeMetaLookup`; the 3rd and 4th UNTOUCHED:
  ```go
  func ResolveCustomTags(
      specs []CustomTagSpec,
      headerLookup func(string) ([]string, bool),
      metaLookup func(ns, key string) (*structpb.Value, bool),      // REQUEST — unchanged, two-arg
      routeMetaLookup func(ns string) (*structpb.Value, bool),      // ROUTE   — unchanged, one-arg
      hostMetaLookup func(ns string) (*structpb.Value, bool),       // HOST    — NEW, one-arg, nil-tolerant
  ) []KV
  ```
  *(The landed declaration is a single line; match the landed formatting at the tip — this expansion is for readability only.)* The doc block `:12-31` gains a HOST clause mirroring the `routeMetaLookup` paragraph at `:24-29`.

- **The `Endpoint` field + `MetaLookup` accessor (T1, `internal/cluster/cluster.go`)** — the field is APPENDED to the struct (`:43-75`) beside `Metadata` `:49`, and the accessor placed beside `IsZero()` `:92`:
  ```go
  	// filterMetadata retains the endpoint's RAW per-namespace static metadata
  	// (lb_endpoints[].metadata.filter_metadata), ALIASING the already-parsed
  	// proto map — zero new allocation. Added phase 72 so a HOST-kind tracing
  	// custom_tag can address ANY namespace and walk a NESTED path; the phase-38
  	// Metadata projection above stays the envoy.lb scalars-only subset-LB
  	// dimension and is BYTE-UNCHANGED. NOT part of the dial identity: Addr()
  	// ignores it.
  	filterMetadata map[string]*structpb.Struct
  ```
  ```go
  // MetaLookup returns the endpoint's static metadata for the namespace ns,
  // wrapped as a structpb StructValue (or (nil,false) when the endpoint carries
  // no metadata / the namespace is absent). The HOST analog of the HCM chain's
  // RouteMetaLookup; threaded as a method value at the three tracing emit sites.
  // Safe on the ZERO Endpoint (nil map → (nil,false)).
  func (e Endpoint) MetaLookup(ns string) (*structpb.Value, bool) {
  	st := e.filterMetadata[ns]
  	if st == nil {
  		return nil, false
  	}
  	return structpb.NewStructValue(st), true
  }
  ```
  ADD the import `structpb "google.golang.org/protobuf/types/known/structpb"` to `cluster.go`'s block (`:3-22`) — re-derive the exact spelling/grouping at the tip; it is an EXISTING module (RD-MOD). **The nil guard is REQUIRED by the accessor's own `(nil,false)` contract** — but see F5b: it is NOT observably load-bearing through `ResolveCustomTags`, so T1's break must assert on the accessor's return, not on a span attribute.

- **The `manager.go:884` populate (T1)** — the `filterMetadata:` key is ADDED to the ONE non-empty production composite literal; `:883` and `:754` stay byte-unchanged:
  ```go
  			e := Endpoint{Host: sa.GetAddress(), Port: sa.GetPortValue(), Metadata: scalars, filterMetadata: lbe.GetMetadata().GetFilterMetadata(), Locality: loc, LocalityWeight: weight, Priority: priority}
  ```
  `GetMetadata()`/`GetFilterMetadata()` are both nil-receiver-safe (RD-PROTO) ⇒ a metadata-less endpoint yields a nil map, and `MetaLookup` returns `(nil,false)`. *(The literal measures ~197 chars; **`lll` is NOT among the enabled linters** — `.golangci.yml` enables only govet/errcheck/staticcheck/unused/ineffassign/gofmt/goimports/misspell/revive — so leave the line as one statement and do not reformat for a linter that never runs.)* ⚠️ **`misspell` IS enabled with `locale: US`** — keep the accessor's doc comment free of British spellings (`analog`, not `analogue`); the §1.2 skeleton is already misspell-clean, and a verbatim paste of an earlier draft failed `golangci-lint run ./internal/cluster/` on exactly this.

- **The `meta_host_ok` fuzz seed (T5, `fuzz_test.go`, purely ADDITIVE)** — added inside `withMetaTags` (`:97-115`) alongside `meta_ok`/`meta_route_ok`; **`meta_bad` STAYS on CLUSTER, no repoint**:
  ```go
  				{Tag: "meta_host_ok", Type: &tracingv3.CustomTag_Metadata_{Metadata: &tracingv3.CustomTag_Metadata{
  					Kind:         &metadatav3.MetadataKind{Kind: &metadatav3.MetadataKind_Host_{Host: &metadatav3.MetadataKind_Host{}}},
  					MetadataKey:  &metadatav3.MetadataKey{Key: "envoy.test", Path: []*metadatav3.MetadataKey_PathSegment{{Segment: &metadatav3.MetadataKey_PathSegment_Key{Key: "k"}}}},
  					DefaultValue: "fb",
  				}}},
  ```
  (`metadatav3` already imported at `:13`; `MetadataKind_Host_`/`MetadataKind_Host` verified present in the pinned dep at `metadata.pb.go:211`. A fuzz `f.Add` only needs to REACH the arm — accept/reject unit coverage is T2's.)

### 1.3 Findings this re-derivation surfaced that the SPEC does NOT carry

**Three are UNNAMED EDIT-SITE OBLIGATIONS** — the same defect class SPEC §7 correctly pins for `fuzz_test.go` but misses elsewhere (`reference_code_comment_not_evidence`). All are folded into the tasks below.

- **F1 (MODERATE) — two stale doc comments in `internal/tracing/config_test.go`.** Both `_RejectKinds` tests assert in prose that HOST rejects, and go FALSE the moment T2 lands:
  - **`:627-629`** — `// TestNewConfigRejectCustomTagMetadata_RejectKinds: CLUSTER/HOST and an unset kind // each boot-reject with an ADR-0080-distinct substring (envoy-go-strict DEPARTURE — // the reference accepts these kinds).`
  - **`:754-757`** — `// TestNewConfigRejectCustomTagMetadataRoute_RejectKinds: CLUSTER/HOST and an unset // kind still boot-reject with an ADR-0080-distinct substring …`
  ⇒ **T2 narrows BOTH to CLUSTER-only**, alongside deleting the two `/host` table rows.
- **F2 (MODERATE) — SEVEN stale comment lines, in FOUR groups, in PRODUCTION `internal/tracing/config.go`.** SPEC §11 scopes config.go to "(`:54-60` const, `:267-268` → accept)" only. Also stale after the lift: the `customTagKind` doc **`:48-51`** (enumerates the sources, stops at ROUTE); the `Kind` field comment **`:69`** (`kindLiteral | … | kindMetadataRoute`); four `CustomTagSpec` field comments **`:73`/`:74`/`:75`/`:76`** (`Kind==kindMetadata|kindMetadataRoute:`); and the `parseCustomTags` doc **`:178-183`**, whose **`:182`** reads `// metadata CLUSTER/HOST/unset kinds reject LOUDLY with distinct substrings` — **flatly false** after the lift. ⇒ **T2 updates all six.** `resolve.go`'s doc `:12-31` IS cited by the SPEC and gains a HOST clause at T3.
- **F3 (MINOR) — a THIRD, uncited unset-kind regression guard.** `config_test.go:460-464` (`metadata-unset-kind` inside `TestNewConfigRejectCustomTagArms`, `func` at `:434`) exercises a `CustomTag_Metadata{}` with no `Kind` at all and expects `"kind required"`. It must STAY reject; it is a free extra guard on the `:229-230` arm. ⇒ **T2 verifies it still passes; no edit.**
- **F4 (MINOR) — the ADR-0294 §Context carries a justification its own SPEC refuted.** See **RD-LAYERING**. ⇒ **T9 obligation.**
- **F5 (MINOR, two parts) — a gofmt trap and an over-stated guard justification.**
  - **(a)** SPEC §3.2's compressed skeleton is not gofmt-clean (§1.2 preamble). Use the multi-line forms above.
  - **(b) The `structpb.NewStructValue(nil)` non-nil fact is CONFIRMED by EXECUTION** (`NewStructValue(nil) == nil` ⇒ **false**; the wrapper's `GetStructValue()` is nil and `GetFields()` is empty). **But the nil guard is NOT observably load-bearing through `ResolveCustomTags`** — a guard-free `MetaLookup` returning `(NewStructValue(nil), true)` still lands on default/omit, because `descend`'s FIRST hop calls `cur.GetStructValue()`, gets nil and returns `(nil,false)` **without panicking** (EXECUTED), and `MetaPath` is guaranteed `len >= 1` by the parse-time validation at `config.go:236`/`:253`. The ONLY input that would reach `structpbValueToString` with the empty wrapper is an EMPTY `MetaPath`, which parse forbids. ⇒ **Keep the guard** (the accessor's `(nil,false)` contract requires it and it is the correct shape) **but T1's guard break MUST assert on `MetaLookup`'s own return value — a break routed through a span attribute would be VACUOUS** (`reference_vacuous_break_receiver_normalizes`, `reference_probe_must_discriminate`).

### 1.4 Adversarial-pass record — BOTH verifiers found real defects; every finding is FOLDED

**TWO independent verifiers ran against this draft in PRIVATE scratch before landing** (`reference_parallel_subagents_private_scratch`; the real repo left untouched, no worktrees registered), on disjoint remits. **Both found real defects. ZERO SEVERE; SEVEN MODERATE; TWELVE MINOR — all folded, and the folds are visible above at the point of use.** The single most valuable outcome: **V1 REFUTED one of this PLAN's own "drift corrections"** — a case of the document's corrections needing the same scepticism its cites do.

**V1 — code-claims re-derivation + by-execution.** V1 copied the tree to private scratch, **applied every §1.2 skeleton verbatim and BUILT the row**: `gofmt -l` SILENT · `go build ./...` 0 · `go vet ./...` 0 · `go mod tidy -diff` EMPTY (**+0 modules CONFIRMED**) · the layering check EMPTY · and **sha256 proof that `connection.go`, `h2dispatch.go`, `h3dispatch.go`, `accesslog_emit_test.go`, `chain.go` and `subset.go` are byte-identical to baseline — the row's central cost claim CONFIRMED BY HASH.** It also confirmed the predicted red set is **EXACTLY TWO** `/host` subtests (output printing `Kind:5` ⇒ iota==5), the F5a gofmt trap, the **F5b vacuity finding** (`descend(NewStructValue(nil), ["k"])` → `(nil,false)` without panicking, *identical* to the guarded path ⇒ a span-routed Break C would be vacuous), Break G's discrimination, the FULL-`MetaPath` equivalence, the T5 dispatch-verify (run ahead of time: `HOST ACCEPT arm reached for tag meta_host_ok`), F4's reality, `unsafe.Sizeof(Endpoint)` 104 → 112, and ~55 discrete cites. **ZERO SEVERE. THREE MODERATE:** (M1) **RD-FUZZ's "drift correction" was ITSELF WRONG** — `CLUSTER/HOST` is on `fuzz_test.go:92`, so the SPEC's `:92-93` was correct and the "corrected" `:93-94` contains no such token; an `old_string` built on it would have missed the stale claim and garbled the adjacent, correct dispatch-order sentence; (M2) **T4's entry state and red classification were both false** — after T3 the package does NOT compile (T3 grows the `ResolveCustomTags` arity without touching its three call sites), so the red is three `not enough arguments` compile errors, not the "value mismatch" predicted; the draft had conflated the *emit-signature* arity (genuinely unchanged — the row's real distinction from 70/71) with the *`ResolveCustomTags`* arity (which changes exactly as in 70/71); (M3) **the `MetaLookup` doc-comment skeleton FAILED `golangci-lint`** — `analogue` is flagged by `misspell` (`locale: US`, enabled), breaking T1 Step 6 and T8 gate 5 on a verbatim paste. Plus NINE MINOR (the `resolve_test.go` 4th-arg distribution, RD-LIT's replacement figures, an unscoped `map[Endpoint]` grep, the zero-value over-count, Break D not being expressible at the call site, Break K over-warning, Break F's backwards phrasing, a moot `lll` caveat, and a "test constructor" alternative that would have minted a third exported symbol — with `Cluster.PickEndpoint()` EXECUTED as the zero-new-symbol path). **Verdict: BUILDABLE AS WRITTEN** once M3's one-word fix lands.

**V2 — process, consistency, SPEC-coverage and stage-close mechanics.** V2 re-ran every mechanical check and confirmed: the envelope is docs-only with ZERO production `.go`; DECISIONS and ROADMAP UNTOUCHED; row 72 `in-progress`; **ADR-0294 is PROPOSED, §Context-only, with `grep -c '^### Decision'` ⇒ 0 inside the block** (so T9's APPEND instruction is sound and the SPEC's V2 F1/F2 fix held) and its §Context does carry the refuted "(the cycle guard)" wording (F4 real); all five counts re-derive; **every `117` is scoped current and every `118` scoped IMPL-post-state**; the three sentinel commands produce exactly the predicted output with no `stop` file; SPEC §10/§11 production coverage complete with **no `TestNoNewStat`-class guard manufactured or omitted**; task count 9; break letters A–Q unique and sequential; the break protocol fully bound; format faithful to phase 71. **ZERO SEVERE. FOUR MODERATE:** (F1) `PROGRESS.md` did not exist while five places cited it as a record channel and the stage envelope requires it; (F2) **§1.4/§1.5 asserted an adversarial record in the past tense while §1.5 was an empty placeholder — and self-certified immunity to exactly that defect**, the phase-69 cited-but-unwritten class recurring, aggravated by the self-certification (this section is the fold: the split is collapsed and populated from the real reports); (F3) the header's sentinel line pointed at "§Task 9" for the PLAN close, but §Task 9 is the **IMPL**'s close where check (1) instead goes SILENT — and consequently **the PLAN listed none of its own stage-close obligations** (a PLAN-close checklist is now in Global Constraints); (F4) the `accesslog_emit_test.go` SPEC-roster departure was silently inverted into a BYTE-UNTOUCHED gate rather than named. Plus THREE MINOR (a "six"-vs-seven stale-comment count, a `SPEC §13` miscite for the memory updates, and RD-IDENT's re-run obligation not assigned to any task step — now T1 Step 0). **Verdict: stage-close mechanics PASS with the required fixes, all applied.**

**What neither verifier could check, stated plainly:** the seven SPEC-time live-probe arms (P1–P7) were not re-run — the containers were cleaned up — so the reference behaviours behind B1–B4 and the `0116` design rest on the SPEC's own executed evidence. **Nobody has verified that the `0116` fixture actually produces a green cross-side assertion**; that is the IMPL's job (T6), and the non-`envoy.lb` namespace is genuinely new cross-side ground (RD-0064).

---

## Global Constraints

- **ONE stage per session.** This session: the PLAN only. No production `.go`. After it lands: roll to the phase-72 IMPL.
- **⚠️ THIS PLAN's OWN stage-close checklist** *(distinct from §Task 9, which closes the IMPL — every task T1–T9 below is an IMPL task).* At the PLAN close the controller: creates `PLAN.md` + **`PROGRESS.md`** (the only two files in the phase directory delta) · rolls **STATE §Current IN PLACE, lifecycle 2 → 3** (never prepend above §Current — the ADR-0288 rule), re-caps the §Recent lineage at five **and updates its PREAMBLE** (naming the correct newest + dropped bullet — the SPEC's V2 F3 defect) · rolls `next-prompt.txt` to the phase-72 **IMPL** (TRACKED despite .gitignore; edit in the stage worktree) · **re-runs the three sentinel checks MECHANICALLY** expecting `NOT DONE: row 72` / **3** / `NEVER OPENED: gRPC/Runtime/WASM` ⇒ does NOT fire, no `stop` · re-runs every count in the worktree · leaves **DECISIONS and ROADMAP UNTOUCHED** and **row 72 `in-progress`** · squash-pushes.
- **FIVE functionally-edited production files, ZERO new packages** (SPEC §4, §11): `internal/cluster/cluster.go` · `internal/cluster/manager.go` · `internal/tracing/config.go` · `internal/tracing/resolve.go` · `internal/filter/hcm/accesslog_emit.go`. **New exported symbols: `func (e cluster.Endpoint) MetaLookup(ns string) (*structpb.Value, bool)` + the `ResolveCustomTags` signature growth (a 5th param) + the unexported `kindMetadataHost` const and `Endpoint.filterMetadata` field.** `descend`/`structpbValueToString`/`CustomTagSpec`'s metadata fields are REUSED (landed at phases 70/71).
- **BYTE-UNTOUCHED (assert mechanically at T8):** `internal/filter/hcm/{connection.go,h2dispatch.go,h3dispatch.go}` (**all 18 emit callers**) · `internal/filter/http/chain.go` · `internal/cluster/subset.go` · `internal/xds` · `internal/tls` · `internal/boot` · `internal/listener` · `internal/bootstrap` · `validate/` · `internal/dynamicmetadata` · `internal/filter/http/{ratelimit,lua}`. **Also byte-stable: the 29 existing emit test callers** (13 `accesslog_emit_test.go` + 16 `span_emit_test.go`) — though `span_emit_test.go` IS an edit site for T4's NEW tests (RD-EMITTESTS precision).
- **⚠️ `internal/cluster` CANNOT be asserted BYTE-UNTOUCHED** (phases 70/71 could). **Pin the SHAPE instead:** exactly ONE added field + ONE added accessor + ONE added import in `cluster.go`, and exactly ONE changed line in `manager.go` (`:884`), with **`ScalarsFromStruct` (`subset.go:263-285`), `defaultSubset` (`manager.go:754`) and the `envoy.lb` projection (`manager.go:883`) byte-unchanged** (SPEC §3.5).
- **`HOST` MetadataKind ONLY.** `CLUSTER` (`:265-266`) and both unset-kind arms (`:229-230` sub-case A, `:269-270` sub-case B) reject LOUDLY with distinct substrings. The split (SPEC §3.6, P1-EXECUTED + `.validate.go`-cross-checked): sub-case **A** (a fully-absent `kind`) is a **DEPARTURE** — `CustomTag_Metadata.validate` carries NO `required` rule on `Kind`, so the reference ACCEPTS it; sub-case **B** (a present-but-empty `kind: {}` oneof) is **PARITY** — PGV-rejected. The `CLUSTER` reject is a real DEPARTURE (P1: the reference still BOOTS it). Empty `metadata_key.key` / empty `path` / empty segment reject are **PGV-PARITY**. *(Note: the C++ PGV text is `MetadataValidationError.Kind … is required`; the GO generated symbols are named differently — `MetadataKindValidationError` / `CustomTag_MetadataValidationError` — so do not grep for the C++ string in the Go tree.)*
- **The default rule is `request_header`, NOT `environment`** (RD-DEFAULT): a present-but-empty HOST metadata value emits `""` (does NOT fall to the default); absent + non-empty default → default; absent + empty/omitted default → omit. `HasDefault = DefaultValue != ""`.
- **The HOST resolve descends the FULL `MetaPath`** (RD-HOST-ARM) — NOT `[1:]`. The REQUEST `[1:]` (`resolve.go:84`) is a Bucket-pre-keying artifact; the HOST lookup returns the whole namespace struct.
- **The 5th arg APPENDS at the three `ResolveCustomTags` call sites ONLY** (RD-SEAM). The three emit SIGNATURES and all 18 emit callers are UNCHANGED — this is the row's central cost claim, verified mechanically at T8.
- **A cost this row ACCEPTS and records:** the `picked.MetaLookup` method value is evaluated unconditionally inside the sample block, before `ResolveCustomTags`'s `len(specs)==0` early return, so **every sampled span gains +1 alloc / +128 B even with no `custom_tags` configured** (MEASURED at the SPEC: all candidate closure shapes cost exactly 1 alloc / 128 B; the pointer-receiver variant is strictly worse at 2). It matches how `metaLookup`/`routeMetaLookup` are already threaded. The mitigation (a parse-time `hasHostTags bool` gate) is NAMED so a future profiling row need not re-derive it.
- **The closure is a LAYERING rule, NOT a cycle guard** (RD-LAYERING) — `internal/cluster` does not depend on `internal/tracing`, so a direct import would build clean. Keep the closure (it preserves the layering `go list -deps` asserts) but **never justify it as preventing a cycle**, and correct ADR-0294's §Context wording at T9 (F4).
- **Counts at the IMPL:** fixtures **117 → 118** (`0116-tracing-custom-tags-metadata-host`) · fuzzers **55 (+0)** — a purely ADDITIVE seed, `f.Add` is not `^func Fuzz` (`reference_fuzzer_count_docs_drift`) · stat surface **1201 (+0)** — a span attribute registers no stat; **NO `TestNoNewStat` guard obligation for tracing rows** (SPEC §7, the phase-71 V2 finding — do NOT manufacture a cited-but-unwritten one) · BackendKind **38 (+0)** — `0116` reuses `HTTPFixedBody` · go.mod **+0** (re-check `git diff go.mod` after tidy — RD-MOD) · ZERO new packages · DECISIONS tail stays **ADR-0294** (completed IN PLACE at the IMPL; next-free ADR-0295).
- **The pinned §9 wording lands MECHANICALLY** — B1–B4 are named obligations with the SPEC §9 replacement text; never silent rewrites, never paraphrases. They land at T7, atomically with the row-done edit; ADR-0294 completes at T9.
- **Per-task hygiene** (`feedback_pertask_gofmt_lint`): `gofmt -l` + `go vet` + `golangci-lint run` on every touched package.
- **Worktree discipline** (`feedback_git_worktrees` / `feedback_subagent_worktree_detach` / `feedback_subagent_worktree_path_targeting`): pin the canonical root; the controller verifies the MAIN checkout stays clean; deliberate breaks restore with **`git restore` only**; breaks run AFTER committing (`reference_break_protocol_commit_first`).
- **Subagents commit locally; the controller squash-pushes at stage-close** (`feedback_subagents_no_push`, `feedback_push_to_origin`). Locate commits by SUBJECT (`git log --grep 'phase 72'`), never by position.
- **Known pre-existing flakes — do NOT reflex-classify as phase-72 regressions:** the `internal/cluster` `-race` outlier flake (RD-RACEFLAKE, `outlier_test.go:766` — newly load-bearing this row; `reference_cluster_race_outlier_flake`); the full-suite startup flake (`reference_differential_fullsuite_startup_flake`); `0061` ring-hash spread and the SDS `init_fetch_timeout` dial-budget flake (one occurrence each).

### Break protocol (binding on every task)

- **A break must COMPILE** (`reference_plan_break_instructions_dont_compile`). Breaks flagged `[NOT pre-compiled — substitution rule applies]`: at IMPL time, if it does not compile, **substitute a compiling equivalent, REPORT the substitution, record the TRUE result**.
- **A break must DISCRIMINATE** (`reference_probe_must_discriminate`): before recording it as proof, ask what the OTHER hypothesis would have printed. **⚠️ F5b makes this concrete for T1** — a `MetaLookup` nil-guard break routed through a span attribute is **VACUOUS** (`descend` already returns `(nil,false)` without panicking); assert on the accessor's own return.
- **`-count=1` on EVERY differential break** (`reference_differential_break_protocol_count1`); caching serves a stale PASS.
- **Confirm WHICH assertion fired** (`reference_deliberate_break_wrong_assertion`) — and whether a second property's firing is ENTAILED by the first.
- **A break that does NOT fire is a FINDING** — record it honestly in PROGRESS; do not route around it.
- **Full selector only:** `-run 'TestDifferential/0116-tracing-custom-tags-metadata-host'` — never bare `0116` (`reference_differential_run_selector`).
- **`Errorf` per independent property; `Fatalf` only for broken preconditions** (`reference_fatalf_makes_assertions_unreachable`).
- **The HOST metadata must be SERVED-this-arm** — the span must carry the STATIC endpoint value, not a vacuous default; a **wrong-namespace control** that falls to the default proves the endpoint-metadata/namespace binding is load-bearing (the `0116` fixture has NO runtime writer — the source is served static config, cross-side-identical).

### Identifier roster (`reference_spec_drafted_identifier_collision_check`)

**Verified FREE repo-wide at `2a82cc7b` (`--include='*.go' internal/ test/ validate/ cmd/`, all 0 hits — RD-IDENT):** `kindMetadataHost` · `hostMetaLookup` · `filterMetadata` · word-boundary `MetaLookup`. ⚠️ **The naive substring `grep MetaLookup` returns 51 — ALL `RouteMetaLookup`-prefixed (22 + 27 + 2); use the word-boundary form.** **REUSED (landed, free to reference):** `descend` · `structpbValueToString` · `CustomTagSpec.MetaNamespace`/`MetaPath`/`DefaultValue`/`HasDefault` · `kindMetadata` · `kindMetadataRoute` · `ScalarsFromStruct` · `upstreamHostString`. **`metadatav3`** is the ESTABLISHED alias (ratelimit + `fuzz_test.go:13`). **Fixture:** `test/fixtures/0116-*` does not exist; reference port **10116** is free (0 hits under `test/`). **Any FURTHER name the IMPL coins** (fixture `package driver` helpers, the endpoint metadata namespace/key strings, any test name): grep first, record the check.

---

## File structure

```
internal/cluster/cluster.go                       [EDIT]  T1 (the filterMetadata field + the nil-safe MetaLookup accessor; ADD the structpb import — the row's ONE new import)
internal/cluster/manager.go                       [EDIT]  T1 (:884 ONLY — add filterMetadata: lbe.GetMetadata().GetFilterMetadata() to the literal; :883 + :754 BYTE-UNCHANGED)
internal/cluster/cluster_test.go                  [EDIT]  T1 (MetaLookup: present ns → wrapped struct; absent ns → (nil,false); ZERO Endpoint → (nil,false) no panic)
internal/cluster/manager_test.go                  [EDIT]  T1 (the populate is live; the envoy.lb projection + defaultSubset byte-unchanged guard)
internal/tracing/config.go                        [EDIT]  T2 (kindMetadataHost APPENDED at iota==5; the HOST accept arm replacing the :267-268 reject; the SEVEN stale comment lines (4 groups) F2; NO new import)
internal/tracing/config_test.go                   [EDIT]  T2 (the two /host rows :639/:765 flip reject→accept; the two stale doc comments F1; CLUSTER/unset rows STAY reject)
internal/tracing/resolve.go                       [EDIT]  T3 (the hostMetaLookup 5th param; the kindMetadataHost arm descending the FULL MetaPath; the :12-31 doc HOST clause; NO new import)
internal/tracing/resolve_test.go                  [EDIT]  T3 (HOST path-walk single/multi/unresolvable; the default/omit/present-empty matrix; nil-hostMetaLookup tolerance; 32 existing calls += trailing nil)
internal/filter/hcm/accesslog_emit.go             [EDIT]  T4 (:57/:118/:179 ONLY — append picked.MetaLookup as the 5th ResolveCustomTags arg; the 3 SIGNATURES UNCHANGED; structpb already imported :9)
internal/filter/hcm/span_emit_test.go             [EDIT]  T4 (NEW: a live HOST-metadata-span test + a first-class zero-picked test; the 16 existing callers BYTE-STABLE)
internal/filter/hcm/fuzz_test.go                  [EDIT]  T5 (withMetaTags += meta_host_ok; meta_bad STAYS CLUSTER; narrow the stale :92-93 comment; +0 func Fuzz)
test/fixtures/0116-tracing-custom-tags-metadata-host/  [ADD]  T6 (driver/, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md — NO scripts/), T6 (breaks)
test/differential/runner_test.go                  [EDIT]  T6 (ONE blank-import line at :143, after the 0115 line at :142)
docs/envoy-go/BEHAVIOR_CONTRACT.md                [EDIT]  T7 (B1 metadata → CONSUMED for REQUEST+ROUTE+HOST; B2 the pick-time departure; B3 the Zipkin/serialization boundaries; B4 the :741 deferred-field narrow)
docs/envoy-go/DECISIONS.md                        [EDIT]  T9 (ADR-0294 completed IN PLACE — APPEND ### Decision + ### Consequences; NO second §Decision heading; correct the "cycle guard" wording, F4)
internal/filter/hcm/{connection.go,h2dispatch.go,h3dispatch.go} · internal/filter/http/chain.go · internal/cluster/subset.go · internal/xds/** · internal/tls/** · internal/boot/** · internal/listener/** · internal/bootstrap/** · validate/** · internal/dynamicmetadata/** · internal/filter/http/{ratelimit,lua}/**  [BYTE-UNTOUCHED]
```

---

## Task 1 — `internal/cluster`: the `Endpoint.filterMetadata` field + the `MetaLookup` accessor + the `manager.go:884` populate

**Files:**
- Modify: `internal/cluster/cluster.go` (the `Endpoint` struct `:43-75`; the accessor beside `IsZero()` `:92`; the import block `:3-22`) · `internal/cluster/manager.go` (`:884` ONLY)
- Test: `internal/cluster/cluster_test.go`, `internal/cluster/manager_test.go`

**Interfaces:**
- Produces: the unexported `Endpoint.filterMetadata map[string]*structpb.Struct` field + the exported `func (e Endpoint) MetaLookup(ns string) (*structpb.Value, bool)`. Consumed by `accesslog_emit.go` (T4) as the method value `picked.MetaLookup`.
- Consumes: `lbe.GetMetadata().GetFilterMetadata()` (both nil-receiver-safe — RD-PROTO) and `structpb.NewStructValue` (`struct.pb.go:396-398`).
- **ONE new import** (`structpb`) — the row's only one, repo-wide.

**Entry state:** clean `2a82cc7b`-derived branch; `go test ./internal/cluster/ -count=1` green.

- [ ] **Step 0 — re-run the FOUR collision greps at the IMPL tip** (RD-IDENT; SPEC §5 requires the re-run "at the PLAN and again at the IMPL tip", and this is where it lands). All four must be **0**: `kindMetadataHost` · `hostMetaLookup` · `filterMetadata` · **word-boundary** `grep -rnE '(^|[^A-Za-z])MetaLookup' --include='*.go' internal/ test/ validate/ cmd/`. ⚠️ **The naive substring `grep MetaLookup` returns 51 — all `RouteMetaLookup`-prefixed; do not read that as a collision.** Record the four counts in PROGRESS. A non-zero result on any of them BLOCKS the task until the name is re-picked.

**Design (RE-DERIVED; §1.2 skeletons):** the field ALIASES the already-parsed proto map ⇒ **zero new allocation per endpoint**; `unsafe.Sizeof(Endpoint)` 104 → 112 B (MEASURED at the SPEC). It is retained BESIDE — never replacing — the phase-38 `envoy.lb` scalars-only `Metadata` projection, because P2 EXECUTED that **`envoy.lb` is NOT privileged: ANY `filter_metadata` namespace is addressable**, and a HOST tag must also walk NESTED paths (exactly the `StructValue` kind `ScalarsFromStruct` drops at `subset.go:277-279`).

- [ ] **Step 1 — write the failing tests (red-first).**
  1. `cluster_test.go` — `TestEndpoint_MetaLookup`: (a) an `Endpoint` whose `filterMetadata` has `{"ns": {Fields: {"k": stringValue("v")}}}` → `MetaLookup("ns")` returns `(non-nil, true)` and the wrapped value's `GetStructValue().GetFields()["k"].GetStringValue() == "v"`; (b) absent namespace → `(nil,false)`; (c) **the ZERO `Endpoint{}`** → `MetaLookup("ns")` returns `(nil,false)` **without panicking** (the S3 majority arm — 5 of the 12 span-capable emit sites carry the zero endpoint); (d) an endpoint whose `filterMetadata` maps `"ns"` to a nil `*structpb.Struct` → `(nil,false)` (the guard's own contract).
  2. `manager_test.go` — the populate is LIVE: build a cluster whose `lb_endpoints[0].metadata.filter_metadata` carries a NON-`envoy.lb` namespace with a NESTED value; assert the built `Endpoint.MetaLookup("<ns>")` resolves it. **This is the test that would fail if the populate were omitted or pointed at `["envoy.lb"]`.**
  3. `manager_test.go` — the **byte-unchanged guard**: assert the phase-38 projection still behaves exactly as before — an `envoy.lb` scalar still lands in `Endpoint.Metadata` as a `SubsetValue`, and a non-scalar `envoy.lb` key is still DROPPED from `Metadata` (while now being reachable via `MetaLookup`). Assert `defaultSubset` behavior is unchanged.

  Run `go test ./internal/cluster/ -count=1`. **Expected: FAIL** — `MetaLookup`/`filterMetadata` undefined (compile error). Record the verbatim red.

- [ ] **Step 2 — add the field + accessor + import** (§1.2 skeletons). APPEND the field to the `Endpoint` struct; place `MetaLookup` beside `IsZero()` (`:92`); add `structpb` to the import block (`:3-22` — re-derive exact grouping at the tip). **Do NOT touch `Addr()`, the `addr` cache, `IsZero()`, or the non-comparability comment.**

- [ ] **Step 3 — add the populate line.** `manager.go:884` ONLY — add `filterMetadata: lbe.GetMetadata().GetFilterMetadata()` to the composite literal. **`:883` (`ScalarsFromStruct`) and `:754` (`defaultSubset`) stay BYTE-UNCHANGED — verify with `git diff`.**

- [ ] **Step 4 — run the tests.** `go test ./internal/cluster/ -count=1`. **Expected: PASS** (all four `MetaLookup` cases + the live populate + the byte-unchanged guard; every pre-existing cluster test green — subset LB, maglev, ring_hash, health, locality, priority all untouched).

- [ ] **Step 5 — breaks (AFTER committing).**
  - **Break A [populate dropped]:** remove the `filterMetadata:` key from the `:884` literal → test 2 (the live populate) FIRES. `git restore`; re-green. *(Discriminates: proves the populate — not some other path — is what makes the namespace reachable.)*
  - **Break B [wrong source]:** populate from `lbe.GetMetadata().GetFilterMetadata()["envoy.lb"]`-derived data instead of the whole map (e.g. a single-entry map) → test 2's NON-`envoy.lb` namespace assertion FIRES. `git restore`; re-green. `[NOT pre-compiled — substitution rule applies]` *(Discriminates: this is the P2 finding made executable — it proves `envoy.lb` is genuinely not privileged.)*
  - **Break C [nil guard dropped]:** remove `if st == nil { return nil, false }` and `return structpb.NewStructValue(st), true` unconditionally → **tests 1(b)/1(c)/1(d) FIRE on the ACCESSOR'S OWN RETURN** (`MetaLookup` now returns `(non-nil, true)` for an absent namespace, since `NewStructValue(nil)` is non-nil — EXECUTED). `git restore`; re-green. `[NOT pre-compiled — substitution rule applies]` **⚠️ F5b: assert on `MetaLookup`'s return, NOT on a span attribute — a span-routed break would be VACUOUS** because `descend`'s first hop returns `(nil,false)` without panicking and `MetaPath` is min-1 by parse validation.
  - **Break D [projection disturbed]:** ⚠️ **Not expressible at the call site** — `ScalarsFromStruct` returns `(map[string]SubsetValue, []string)`, so the `:883` CALL cannot be made to keep struct values without editing `subset.go`. **Named substitution (use this, do not improvise):** temporarily edit `subset.go`'s `default:` arm (`:277-279`) to keep `Value_StructValue` instead of routing it to `nonScalar` → test 3's byte-unchanged guard FIRES. **`subset.go` is a declared BYTE-UNTOUCHED file — `git restore` it immediately and re-green, then re-assert it byte-identical.** `[substitution pre-named; report the TRUE result]`

- [ ] **Step 6 — hygiene + `-race` + commit.** `gofmt -l internal/cluster` silent · `go vet ./internal/cluster/` · `golangci-lint run ./internal/cluster/` · `go test ./internal/cluster/ -race -count=1` (⚠️ **RD-RACEFLAKE**: `TestOutlierDetector_ConcurrentEjectExactlyOnce` `outlier_test.go:766` is a PRE-EXISTING intermittent failure reproduced on the UNMODIFIED baseline — isolate-re-run; do NOT re-classify).

**Commit:** `cluster(phase 72 T1): Endpoint retains RAW per-namespace static metadata — a filterMetadata map[string]*structpb.Struct field ALIASING lbe.GetMetadata().GetFilterMetadata() (zero new allocation, sizeof 104→112 B) + a nil-safe MetaLookup(ns) (*structpb.Value, bool) accessor, populated at manager.go:884 BESIDE the BYTE-UNCHANGED phase-38 envoy.lb scalar projection (:883) and defaultSubset (:754); ANY filter_metadata namespace is addressable (P2), not just envoy.lb; ONE new import (structpb, existing module); no identity ripple — Endpoint is never a map key, never ==-compared, never serialized`

---

## Task 2 — `internal/tracing/config.go`: the `kindMetadataHost` const + the HOST accept arm

**Files:**
- Modify: `internal/tracing/config.go` (the `customTagKind` block `:54-60`; the `parseCustomTags` HOST `case k.GetHost() != nil:` `:267-268`; the SEVEN stale comment lines, 4 groups — F2)
- Test: `internal/tracing/config_test.go` (the two `/host` rows `:639`/`:765`; the two stale doc comments — F1)

**Interfaces:**
- Produces: `kindMetadataHost` const (**iota == 5**); the HOST accept path building `CustomTagSpec{Kind: kindMetadataHost, MetaNamespace, MetaPath (FULL), DefaultValue, HasDefault}`. Consumed by `resolve.go` (T3).
- Consumes: `md.GetKind()` (→ `*metadatav3.MetadataKind`, bound as `k` at `:227`) then **`k.GetHost()` on `k`, NOT on `md`** (RD-PROTO — `*CustomTag_Metadata` has exactly three getters and no `GetHost()`); `md.GetMetadataKey()`, `mk.GetKey()`, `mk.GetPath()`, `seg.GetKey()`, `md.GetDefaultValue()`. **NO new import.**

**Entry state:** T1 landed; `go test ./internal/tracing/ -count=1` green (verified at tip — the package is GREEN at baseline, so the two `/host` subtests are genuinely the only pre-known red set).

**⚠️ The pre-known red set is EXACTLY TWO subtests**, both `t.Fatalf("NewConfig(%s) err = nil, want reject; got %+v", tc.name, got)`:
- `TestNewConfigRejectCustomTagMetadata_RejectKinds/host` — row `:639`, assertion `:648`
- `TestNewConfigRejectCustomTagMetadataRoute_RejectKinds/host` — row `:765`, assertion `:774`

**The live regression guard — these MUST STAY reject:** `cluster` rows `:638`/`:764` (`"cluster kind unsupported"`), `unset-kind` rows `:640`/`:766` (`"kind required"`), and the THIRD uncited guard `metadata-unset-kind` at **`:460-464`** inside `TestNewConfigRejectCustomTagArms` (F3 — verify it still passes; no edit).

- [ ] **Step 1 — write the failing tests (red-first).** In `config_test.go`:
  1. **Convert** the two `/host` table rows (`:639`, `:765`) from reject-expectations into the ACCEPT case — i.e. remove them from the `_RejectKinds` tables and add a `Test…MetadataHost_Accept`: a `CustomTag{Tag:"t", Type:CustomTag_Metadata_{Metadata:{Kind:Host, MetadataKey:{Key:"ns", Path:[{Key:"a"},{Key:"b"}]}, DefaultValue:"d"}}}` parses to `CustomTagSpec{Key:"t", Kind:kindMetadataHost, MetaNamespace:"ns", MetaPath:["a","b"], DefaultValue:"d", HasDefault:true}`.
  2. `Test…MetadataHost_RejectStructural` — empty namespace (`empty namespace`) / empty path (`empty path`) / empty path segment (`empty path segment`) for a HOST-kind tag (PGV-PARITY).
  3. `Test…MetadataHost_HasDefaultFalseWhenEmpty` — a HOST tag with `DefaultValue:""` → `HasDefault == false`.
  4. `Test…MetadataHost_FirstWinsDedup` — two same-key tags (a HOST metadata + a later literal) → the FIRST wins.
  5. **Assert the const's VALUE** — `kindMetadataHost == 5` (pins the APPEND-not-INSERT rule mechanically; a silent renumber of `kindMetadataRoute` would otherwise fail nowhere).

  Run `go test ./internal/tracing/ -run 'MetadataHost' -count=1`. **Expected: FAIL** — `kindMetadataHost` undefined (compile error). Record the verbatim red. Then run the full package and record the two predicted `/host` failures.

- [ ] **Step 2 — add the const.** **APPEND `kindMetadataHost` to the `customTagKind` iota block AFTER `kindMetadataRoute`** (`:54-60` — a new line before the `:60` closing paren) ⇒ **iota == 5**. **NEVER INSERT before `kindMetadataRoute`** — a renumber recompiles silently and fails nowhere (SPEC §3.2: hygiene). No struct change (RD-KIND). No new import (RD-NOIMPORT-CFG).

- [ ] **Step 3 — replace the `:267-268` HOST reject** with the accept arm (§1.2 skeleton, **the gofmt-clean multi-line form**). **Branch on `k.GetHost()` — `k` is already bound at `:227`; `md.GetHost()` DOES NOT COMPILE (RD-PROTO).** Keep unset-A `:229-230`, the REQUEST accept `:231-247`, the ROUTE accept `:248-264`, CLUSTER `:265-266`, `default:` `:269-270`, the empty-tag reject `:201-203` and the first-wins dedup `:275-279` UNCHANGED.

- [ ] **Step 4 — narrow the SEVEN stale production comment lines, in FOUR groups (F2).** `:48-51` (the `customTagKind` doc — add the HOST source) · `:69` (the `Kind` field comment — add `| kindMetadataHost`) · `:73`/`:74`/`:75`/`:76` (the four `CustomTagSpec` field comments — add `kindMetadataHost` to each `Kind==…` list) · **`:178-183` (the `parseCustomTags` doc), especially `:182`** — `// metadata CLUSTER/HOST/unset kinds reject LOUDLY with distinct substrings` narrows to **CLUSTER/unset only**. (`reference_code_comment_not_evidence` — a landed comment carries the authority of proximity; leaving these makes the file contradict itself.)

- [ ] **Step 5 — narrow the TWO stale test doc comments (F1).** `config_test.go:627-629` and `:754-757` both assert "CLUSTER/**HOST** and an unset kind each boot-reject" → narrow BOTH to **CLUSTER-only**.

- [ ] **Step 6 — run the tests.** `go test ./internal/tracing/ -count=1`. **Expected: PASS** (the five new tests green; the `/cluster` + unset rows at `:638`/`:640`/`:764`/`:766` STILL reject; the third guard at `:460-464` still passes; the literal/request_header/environment/metadata-REQUEST/ROUTE arms untouched).

- [ ] **Step 7 — breaks (AFTER committing).**
  - **Break E [accept→reject]:** make the `k.GetHost()!=nil` arm `return nil, fmt.Errorf(...)` instead of building the spec → test 1 FIRES (accept expected, got error). `git restore`; re-green.
  - **Break F [kind confusion]:** swap the CLUSTER and HOST arms (accept CLUSTER, reject HOST) → **TWO assertions fire: the `/cluster` regression rows (`:638`/`:764`) FAIL** (they assert a reject and now get an accept) **AND test 1 fails** (it asserts an accept and now gets a reject) → **confirm WHICH fired, and that BOTH did** (they are independent properties, not an entailment). `git restore`; re-green. `[NOT pre-compiled — substitution rule applies]`
  - **Break G [silent renumber]:** INSERT `kindMetadataHost` BEFORE `kindMetadataRoute` (so HOST==4, ROUTE==5) → **test 5 (`kindMetadataHost == 5`) FIRES.** `git restore`; re-green. *(Discriminates: this is the only assertion that catches a renumber — every other test would stay green, which is exactly why the SPEC calls the append rule "hygiene, not a live hazard".)*
  - **Break H [HasDefault]:** set `HasDefault: true` unconditionally → test 3 FIRES. `git restore`; re-green.

- [ ] **Step 8 — hygiene + commit.** `gofmt -l internal/tracing` silent · `go vet ./internal/tracing/` · `golangci-lint run ./internal/tracing/`.

**Commit:** `tracing(phase 72 T2): custom_tags metadata HOST parse — kindMetadataHost APPENDED at iota==5 (never inserted — a renumber fails nowhere, pinned by an explicit ==5 assertion) + the HOST accept arm (cloning the ROUTE accept verbatim, MetaPath FULL, HasDefault = DefaultValue != "") replacing the :267-268 host-kind reject; CLUSTER + both unset-kind rejects UNCHANGED (envoy-go-strict DEPARTURE — the reference BOOTS them, P1); empty namespace/path/segment PGV-PARITY; branches on k.GetHost() on the bound k (md.GetHost() does not exist); NO new import; narrows 7 stale production comment lines (4 groups) + 2 stale test doc comments`

---

## Task 3 — `internal/tracing/resolve.go`: the `hostMetaLookup` 5th param + the `kindMetadataHost` arm

**Files:**
- Modify: `internal/tracing/resolve.go` (the `ResolveCustomTags` signature `:32`; the doc block `:12-31`; a new `kindMetadataHost` `case` after `:95-116`)
- Test: `internal/tracing/resolve_test.go`

**Interfaces:**
- Produces: `ResolveCustomTags(specs, headerLookup, metaLookup, routeMetaLookup, hostMetaLookup)` — the new nil-tolerant 5th param `hostMetaLookup func(ns string) (*structpb.Value, bool)` (ONE-arg, shaped like `routeMetaLookup`); the `kindMetadataHost` resolve arm.
- Consumes: `descend` (`:128`) + `structpbValueToString` (`:151`) **REUSED VERBATIM**; `structpb.Value`, `KV`.
- Reuses UNTOUCHED: the `kindLiteral`/`kindRequestHeader`/`kindEnvironment`/`kindMetadata`/`kindMetadataRoute` arms. **NO new import.**

**Entry state:** T1–T2 landed; `go test ./internal/tracing/ -count=1` green.

**Design (RE-DERIVED; §1.2 skeletons):** the `kindMetadataHost` arm clones the LANDED `kindMetadataRoute` arm (`:95-116`) — the `request_header` default rule (present-empty EMITS `""`, RD-DEFAULT), a ONE-arg lookup, and `descend(v, s.MetaPath)` on the **FULL** path (RD-HOST-ARM), NOT `s.MetaPath[1:]` (which is the REQUEST arm's Bucket-pre-keying artifact at `:84`).

- [ ] **Step 1 — write the failing tests (red-first).** The **32** existing `ResolveCustomTags(...)` calls (RD-TESTCALLERS: lines 34, 72, 97, 105, 124, 131, 138, 146, 188, 198, 207, 216, 244, 254, 268, 276, 284, 292, 304, 311, 340, 349, 358, 367, 403, 416, 432, 441, 449, 457, 469, 476 — **re-grep at tip; a raw grep returns 33, the extra is the STRING LITERAL at `:106`**) each gain a trailing `nil` (5th arg — no HOST tags). Add HOST tests driving a fake `hostMetaLookup func(ns) (*structpb.Value, bool)`:
  1. **path-walk single** — `MetaPath:["k"]`, `hostMetaLookup("ns")→(structVal{k:"v"},true)` → `KV{Key,"v"}`.
  2. **path-walk multi** — `MetaPath:["a","b"]`, `hostMetaLookup("ns")→(structVal{a:{b:"deep"}},true)` → `KV{Key,"deep"}`; and an unresolvable segment → default/omit.
  3. **the serialization table** — string→raw (`"x"`, unquoted); number→`42`/`3.14`; bool→`true`/`false`; struct→`{"a":"b"}`; list→`["x","y","z"]` (each `json.Compact`-compared — assert the EXACT compact string, `reference_detrand_race_catches_protojson_value_substring`); terminal `NullValue`→unresolvable (default/omit). *(A thin HOST-arm re-assert of the phase-70 serializer, not a re-derivation of the table.)*
  4. **the default matrix** — present-non-empty→emit; present-EMPTY (`structVal{k:""}`)→emit `""` (**NOT** the default); absent namespace→default (if `HasDefault`) else omit.
  5. **nil-`hostMetaLookup` tolerance** — a `kindMetadataHost` spec with `hostMetaLookup==nil` → default/omit, no panic.
  6. **HOST ≡ ROUTE on an identical source** — the same namespace struct + the same `MetaPath` through `routeMetaLookup` and through `hostMetaLookup` yield the SAME `KV` (pins that the clone is faithful and that the FULL-path rule is shared).

  Run `go test ./internal/tracing/ -run 'Resolve' -count=1`. **Expected: FAIL** — `ResolveCustomTags` arity mismatch. Record the verbatim red.

- [ ] **Step 2 — write the arm + grow the signature.** Add the `hostMetaLookup` 5th param (`:32`) and the `kindMetadataHost` `case` (§1.2). Add a HOST clause to the doc block `:12-31`, mirroring the `routeMetaLookup` paragraph at `:24-29`. `descend`/`structpbValueToString` reused VERBATIM — **NO new import** (`structpb` `:9`, `protojson`/`encoding/json`/`bytes` all already there). `MetaPath` is safe to pass whole (validated non-empty at parse — T2; `descend` tolerates any length).

- [ ] **Step 3 — run the tests.** `go test ./internal/tracing/ -count=1`. **Expected: PASS**.

- [ ] **Step 4 — breaks (AFTER committing).**
  - **Break I [wrong slice]:** descend `s.MetaPath[1:]` (the REQUEST slice) instead of the full `s.MetaPath` → tests 1/2's single/multi-segment assertions FIRE (a single-segment HOST path would resolve to the namespace struct ITSELF, not the field). `git restore`; re-green. *(Pins RD-HOST-ARM — the crux, and the exact break that reproduced at phase 71.)*
  - **Break J [default rule — clone the wrong arm]:** make the present-empty case OMIT (the `kindEnvironment` rule) instead of emitting `""` → test 4's present-empty assertion FIRES. `git restore`; re-green. *(Pins RD-DEFAULT — the `request_header` rule.)*
  - **Break K [nil-tolerance dropped]:** call `hostMetaLookup(...)` without the `!= nil` guard → test 5 FIRES. **This COMPILES** (`go build ./internal/tracing/` exit 0 — EXECUTED at the PLAN's adversarial pass); the nil func value panics at call time and test 5 catches it directly. **No substitution is needed** — an earlier draft over-warned here. `git restore`; re-green.

- [ ] **Step 5 — `-race`.** `go test ./internal/tracing/ -race -count=1` (the protojson value-substring path; `reference_detrand_race_catches_protojson_value_substring`).

- [ ] **Step 6 — hygiene + commit.** `gofmt -l internal/tracing` silent · `go vet` · `golangci-lint run ./internal/tracing/`.

**Commit:** `tracing(phase 72 T3): custom_tags metadata HOST resolve — ResolveCustomTags grows a nil-tolerant one-arg hostMetaLookup 5th param (the 3rd metaLookup + 4th routeMetaLookup UNTOUCHED) + a kindMetadataHost arm descending the FULL MetaPath (NOT [1:] — the REQUEST slice is a Bucket-pre-keying artifact); descend/structpbValueToString reused VERBATIM (the request_header default rule: present-empty EMITS ""); the :12-31 doc gains a HOST clause; NO new import; 32 resolve_test.go callers += trailing nil`

---

## Task 4 — `internal/filter/hcm/accesslog_emit.go`: thread `picked.MetaLookup` at the THREE call sites (signatures + 18 callers UNTOUCHED)

**Files:**
- Modify: `internal/filter/hcm/accesslog_emit.go` — **`:57`/`:118`/`:179` ONLY**
- Test: `internal/filter/hcm/span_emit_test.go` (NEW tests; the 16 existing callers byte-stable)

**Interfaces:**
- Produces: `picked.MetaLookup` (T1's method value) passed as the **5th** `ResolveCustomTags` argument at each of the three call sites.
- Consumes: the already-present `picked cluster.Endpoint` (the 4th parameter of all three emit methods, `:27`/`:87`/`:149`) and T3's 5th param.
- **NO new import** (`structpb` `:9`, `internal/cluster` `:12`, both already there).

**Entry state:** T1–T3 landed — and **`internal/filter/hcm` does NOT COMPILE** until Step 2. T3 grew `ResolveCustomTags` from 4 to 5 params but touched only `resolve.go`/`resolve_test.go`, leaving the three call sites 4-arg. **EXECUTED at exactly this state:**
```
$ go test ./internal/filter/hcm/ -count=1
internal/filter/hcm/accesslog_emit.go:57:153: not enough arguments in call to tracing.ResolveCustomTags
internal/filter/hcm/accesslog_emit.go:118:155: not enough arguments in call to tracing.ResolveCustomTags
internal/filter/hcm/accesslog_emit.go:179:153: not enough arguments in call to tracing.ResolveCustomTags
```
⚠️ **Do NOT conflate the two arities.** What makes phase 72 different from 70/71 is that the **emit-method signatures** do not change (so the 18 callers are untouched). The **`ResolveCustomTags` arity DOES change**, exactly as in 70/71 — so T4's red is an ordinary COMPILE error, not the value mismatch an earlier draft of this PLAN predicted.

**⚠️ THE ROW'S CENTRAL COST CLAIM — verify it mechanically, do not assume it.** The three emit SIGNATURES (`:27`/`:87`/`:149`) and **all 18 emit callers** stay BYTE-UNTOUCHED, because the closure is built LOCALLY from the in-scope `picked` rather than threaded as a parameter (RD-SEAM; `picked` is proven live below each call site by `upstreamHostString(picked)` at `:72`/`:133`/`:194`). The **29 existing emit test callers** (13 + 16, all 9-arg) are likewise untouched — though `span_emit_test.go` IS an edit site for the new tests.

- [ ] **Step 1 — write the failing tests (red-first).** In `span_emit_test.go` (clone the phase-70/71 metadata-span tests, swapping `DynamicMetadata`/`RouteMetadata` for a `cluster.Endpoint` carrying `filterMetadata`):
  1. **A live HOST-metadata-span test** — a `Filter` configured with a `kindMetadataHost` custom_tag + a `picked cluster.Endpoint` carrying `filterMetadata{"ns":{"k":"v"}}`. ⚠️ **`filterMetadata` is UNEXPORTED, so a `package hcm` test cannot build one by literal — and adding a test constructor would mint a THIRD new exported symbol, contradicting this row's own envelope.** Use the POPULATE PATH, which needs zero new symbols and was EXECUTED end-to-end at the PLAN's adversarial pass: `cluster.NewManager(bs, stats.NewRegistry())` → `Manager.Get("c_test")` → **`Cluster.PickEndpoint()`** (already exported, no dial). A non-`envoy.lb` namespace with a NESTED value round-trips through `manager.go:884` and out of `ep.MetaLookup(...)`, driven through `emitAccessLog` with a SAMPLING `traceDecision`, asserts the exported span carries the resolved `{tag,"v"}` attribute. **Repeat for `emitAccessLogH2` and `emitAccessLogH3`** — all three call sites must be proven live independently (a single-helper test would leave two of three unproven).
  2. **A first-class ZERO-`picked` test** — the SAME config with `picked = cluster.Endpoint{}` → the tag falls to `default_value` (and, with an empty default, is OMITTED entirely). **This is the 5-of-12-span-capable arm** (RD-SPANCAP: `connection.go:597`/`:699`, `h2dispatch.go:530`, `h3dispatch.go:280`/`:341` — all post-Decode local replies). ⚠️ **Do NOT describe this as "11 of 18"** — that raw figure counts 6 PRE-Decide sites that can never emit a span at all.

  Run `go test ./internal/filter/hcm/ -count=1`. **Expected: FAIL — a COMPILE error**, the three `not enough arguments in call to tracing.ResolveCustomTags` at `:57`/`:118`/`:179` (see Entry state). Record the verbatim red. ⚠️ **The new tests' resolved-value assertions are NOT what proves the thread is live at this point** — they cannot even run yet. Their liveness is proven by **Breaks L and M** after Step 2.

- [ ] **Step 2 — append the 5th argument** at `accesslog_emit.go:57`, `:118`, `:179`: `tracing.ResolveCustomTags(f.tracingConfig.CustomTags, reqHeaderLookup*(…), metaLookup, routeMetaLookup, picked.MetaLookup)`. **Do NOT touch the three signatures. Do NOT touch any of the 18 callers. Do NOT add an import.**

- [ ] **Step 3 — run the tests.** `go test ./internal/filter/hcm/ -count=1`. **Expected: PASS** (the three live HOST-span tests + the zero-`picked` test green; all 29 pre-existing emit callers green with NO edit).

- [ ] **Step 4 — verify the cost claim MECHANICALLY (not by eye).** `git diff --stat` must show `accesslog_emit.go` and `span_emit_test.go` ONLY within `internal/filter/hcm`. Confirm by hash that `connection.go`, `h2dispatch.go`, `h3dispatch.go` and `accesslog_emit_test.go` are byte-identical to the T3 tip (e.g. `git diff --exit-code HEAD~1 -- internal/filter/hcm/connection.go …`). Record the result in PROGRESS. **A failure here refutes the row's headline claim and must be reported, not worked around.**

- [ ] **Step 5 — breaks (AFTER committing).**
  - **Break L [5th arg dropped at ONE site]:** pass `nil` instead of `picked.MetaLookup` at `:57` (H1) only → the H1 live HOST-span test's resolved-value assertion FIRES while the H2/H3 tests stay green. `git restore`; re-green. *(Discriminates per-site — proves each of the three call sites is independently load-bearing, which a single combined test could not.)*
  - **Break M [wrong lookup]:** substitute a lookup that always returns `(nil,false)` → the resolved-value assertions FIRE and the zero-`picked` test STAYS green (it already expects default/omit) — **confirm WHICH fired.** `git restore`; re-green. `[NOT pre-compiled — substitution rule applies]`

- [ ] **Step 6 — hygiene + `-race` + commit.** `gofmt -l internal/filter/hcm` silent · `go vet` · `golangci-lint run ./internal/filter/hcm/` · `go test ./internal/filter/hcm/ -race -count=1` (the dispatch goroutines).

**Commit:** `hcm(phase 72 T4): thread picked.MetaLookup as the 5th ResolveCustomTags arg at accesslog_emit.go:57/:118/:179 — the HOST-kind metadata custom_tag now resolves the SELECTED UPSTREAM ENDPOINT's metadata onto the ingress span on both exporters. THE THREE EMIT SIGNATURES AND ALL 18 EMIT CALLERS ARE BYTE-UNTOUCHED (verified by hash) — the first row in this lineage to skip the 18-caller threading tax, since picked is already the 4th emit parameter; a first-class zero-picked test pins the 5 span-capable local-reply sites falling to default_value/omit`

---

## Task 5 — fuzz: a purely ADDITIVE `meta_host_ok` seed + the stale-comment narrow (+0 fuzzers)

**Files:**
- Modify: `internal/filter/hcm/fuzz_test.go` (the `withMetaTags` seed `:97-115`; the stale comment **`:92-93`**)

**Entry state:** T1–T4 landed. Fuzzer `FuzzHCMConfigParse`; seed `withMetaTags` (RD-FUZZ).

- [ ] **Step 1 — ADD the seed.** In `withMetaTags` (§1.2 skeleton), add a valid HOST-accept tag `meta_host_ok` alongside `meta_ok` (REQUEST, `:100-104`) and `meta_route_ok` (ROUTE, `:105-109`). **`meta_bad` STAYS on CLUSTER (`:110-112`) — NO repoint** (S6: no seed points at HOST, so the arm this row removes is unexercised, and CLUSTER remains a live reject-arm seed). `metadatav3` already imported (`:13`) ⇒ zero new imports. Re-derive the exact `MetadataKind_Host_`/`MetadataKind_Host`/`MetadataKey_PathSegment_Key` wrapper spellings at the tip.

- [ ] **Step 2 — narrow the stale doc comment (RD-FUZZ).** The sentence asserting **`CLUSTER/HOST stay envoy-go-strict departures, ADR-0080`** spans **`:92-93`** — the token `CLUSTER/HOST` is on **`:92`**, the close `departures, ADR-0080).` on **`:93`**. Narrow to **CLUSTER-only**, and **do NOT disturb the following sentence** (`:93-95`, the dispatch-order note `The custom_tags loop runs BEFORE the provider check …`), which is CORRECT and shares line `:93`. Re-grep the token at the tip before building the `old_string`.

- [ ] **Step 3 — dispatch-verify (the named trap — RD-DISPATCH / SPEC §7).** Confirm the seed REACHES the `parseCustomTags` HOST accept arm: `parseCustomTags` is called at `config.go:128`, the `"provider required"` reject at `:138` (**128 < 138**). As a one-off scratch check (NOT committed): temporarily print which arm the seed reaches, run `go test ./internal/filter/hcm/ -run FuzzHCMConfigParse -count=1 -v`, confirm the HOST **accept** for `meta_host_ok` and the cluster-kind **reject** for `meta_bad` — NOT an earlier typeURL/provider reject. Restore. **Record the ACTUAL result in PROGRESS** (`reference_probe_must_discriminate` — a check that would have printed the same thing either way proves nothing).

- [ ] **Step 4 — reconcile the count.** `grep -rn '^func Fuzz' --include='*.go' internal/ test/ | wc -l` → **55** BEFORE and AFTER (`reference_fuzzer_count_docs_drift` — an `f.Add` seed is +0 `func Fuzz`). A short active-fuzz smoke (`go test -run FuzzHCMConfigParse -fuzz FuzzHCMConfigParse -fuzztime 10s ./internal/filter/hcm/`) — no panic; **NO corpus artifacts committed**.

- [ ] **Step 5 — hygiene + commit.** `gofmt -l internal/filter/hcm` silent · `go vet` · `golangci-lint run ./internal/filter/hcm/`.

**Commit:** `hcm(phase 72 T5): fuzz — a PURELY ADDITIVE meta_host_ok seed in withMetaTags exercising the new HOST accept arm (meta_bad STAYS on CLUSTER — no repoint, since no seed pointed at HOST); dispatch-verified to reach the parseCustomTags metadata arm (config.go:128 precedes the provider check :138); narrows the stale :92-93 "CLUSTER/HOST stay envoy-go-strict departures" comment to CLUSTER-only; +0 fuzzers, 55→55`

---

## Task 6 — fixture `0116-tracing-custom-tags-metadata-host` (OTLP) + breaks (fixtures 117 → 118)

**Files:**
- Create: `test/fixtures/0116-tracing-custom-tags-metadata-host/{driver/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}` — **NO `scripts/`**
- Modify: `test/differential/runner_test.go` (ONE blank-import line at **`:143`**, immediately after the `0115` line at `:142`)

**Entry state:** T1–T5 landed. Next fixture `0116`; reference port **10116** free (0 hits under `test/`).

**Design (SPEC §8 — ONE OTLP fixture, NO Zipkin dir; RD-FIXTURE/RD-DRIVER — clone the `0115` chassis):**
- Both YAMLs clone `0115` (H1 listener → `HTTPFixedBody` backend, HCM OTLP tracing provider → the driver-owned `test/helpers/otlptrace.Server` receiver, `random_sampling:100`, ports templated except `refListenerPort = 10116`).
- **(1) MOVE the metadata block** off the route (`envoy.yaml:92-95` / `envoy-go.yaml:91-94`) **onto `clusters[c_backend].load_assignment.endpoints[0].lb_endpoints[0].metadata`** — a SIBLING of `endpoint:` at the same 16-space indent: insert after `envoy.yaml:116` (`endpoint:` at `:114`) and after `envoy-go.yaml:112` (`endpoint:` at `:110`). BOTH YAMLs byte-identical in the metadata block.
- **(2) FLIP both `custom_tags` `kind:`** from `route: {}` to `host: {}` (`envoy.yaml:66-84` / `envoy-go.yaml:65-83`).
- **(3) Nothing else** — **both clusters are ALREADY single-endpoint** (RD-FIXTURE), so the pick is deterministic and no LB spread enters the assertion (`reference_round_robin_offset_randomized`).
- **TWO HOST tags, asserted cross-side EXACT key+value, KEY-BASED** (P2: intra-tag order is internal; `spanAttrMap` `driver.go:576-582` is already a key→value map): `host_hit` (path → the static endpoint value) and `host_default` (absent path → `default_value`). Each an independent `t.Errorf` (`reference_fatalf_makes_assertions_unreachable`), plus the `0087` span-count baseline.
- ⚠️ **STRING VALUES ONLY (S4/P3).** A multi-key struct value is **NOT cross-side comparable** (the reference serializes struct keys in an ARBITRARY order; Go always sorts), scalar-vs-nested numbers use DIFFERENT reference renderers, and top-level scalar numbers render at ~6 significant digits on the reference vs full precision in Go. Also: **Envoy's YAML loader turns any non-integer scalar into a STRING**, so an `envoy.yaml` cannot even express a fractional metadata number.
- **Inherited cross-side asymmetry, precedented:** the reference `c_backend` is `type: STRICT_DNS` (host.docker.internal, `dns_lookup_family: V4_ONLY`, ADR-0010) while the subject's is `type: STATIC`, so the reference must propagate `lb_endpoints[].metadata` through DNS resolution. **Landed precedent: `0064-lb-subset`** already does exactly this and is green in the 117-dir suite. ⚠️ **`0064-lb-subset` has NO `envoy.yaml`/`envoy-go.yaml` — it is DRIVER-GENERATED with inline YAML at `driver/driver.go:206-226` (reference) and `:264-283` (subject); do not try to open YAML files there.** ⚠️ **And that precedent covers the `envoy.lb` namespace ONLY** — namespace-generality rests on **P2's live probe**, not on `0064`. If `0116` uses a non-`envoy.lb` namespace (it should, to exercise the P2 finding), that is NEW cross-side ground: **if the reference side does not resolve it, STOP and report** rather than silently switching to `envoy.lb`.

- [ ] **Step 1–3 — write driver/YAMLs/expectations.** Clone the `0115` chassis; move the metadata block onto `lb_endpoints[0]`; flip both `kind:` to `host: {}`; rename the tags/values; reuse the `0115` driver's `spanAttrMap`/`assertAttrString` accessors (`driver.go:576-605`) and its per-side `AssertStats` invocation shape (`:432-433`). `BackendCount() → 1`, `BackendKind() → fixture.HTTPFixedBody`, self-register in `init()`. Add the blank import at `runner_test.go:143`.
- [ ] **Step 4 — run.** `go test ./test/differential/ -run 'TestDifferential/0116-tracing-custom-tags-metadata-host' -count=1`. **Expected: PASS**; fixture count **118**. Confirm the endpoint metadata is **SERVED** (the span carries the static endpoint value, not a vacuous default — the `host_hit` value-equality assertion is the proof).
- [ ] **Step 5 — hygiene + commit.**

- [ ] **Step 6 — breaks (AFTER committing; `-count=1`, FULL selector, confirm WHICH fired).**
  - **Break N [endpoint value swap]:** change the static `lb_endpoints[0].metadata` value on ONE side → the `host_hit` value-equality assertion FIRES (the sides now differ). `git restore`; re-green.
  - **Break O [default drop]:** empty `host_default`'s `default_value` on both sides → the tag OMITS → the `host_default` presence assertion FIRES. `git restore`; re-green.
  - **Break P [wrong-namespace control]:** point `host_hit`'s `metadata_key.key` at a namespace ABSENT from the endpoint metadata (both sides) → `host_hit` falls to its (unused) default → the `host_hit == "<fixed>"` assertion FIRES. **This is the anti-vacuity control — it proves the endpoint-metadata/namespace binding is load-bearing rather than a default match that would pass regardless.** `git restore`; re-green. `[NOT pre-compiled — substitution rule applies]`
  - **Break Q [source relocation control, RECOMMENDED]:** move the metadata block BACK onto the route (where `0115` has it) while keeping `kind: host` → both sides fall to default → `host_hit` FIRES. *(Discriminates HOST from ROUTE: proves the fixture reads the ENDPOINT source, not a leftover route source.)* `git restore`; re-green. `[NOT pre-compiled — substitution rule applies]`

**Commit:** `differential(phase 72 T6): fixture 0116-tracing-custom-tags-metadata-host — the 0115 OTLP chassis with the metadata block RELOCATED from the route onto lb_endpoints[0].metadata (both YAMLs) + both custom_tags flipped to kind: host, asserted cross-side EXACT key+value key-based, STRING values only (the reference serializes multi-key structs in an ARBITRARY key order); single-endpoint cluster ⇒ deterministic pick, NO writer; breaks N/O/P/Q (fixtures 117→118, reference port 10116)`

---

## Task 7 — BEHAVIOR_CONTRACT delta (B1–B4) — pinned VERBATIM

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`

**Entry state:** T1–T6 landed. Docs-only. Anchor by SYMBOL / first-clause (re-locate the tracing `custom_tags` `metadata` subsection at the IMPL tip; the SPEC cites `:688` for the `custom_tags` paragraph and `:741` for the deferred-field list — **re-derive both line numbers**, they will have shifted if anything above them changed).

- [ ] **B1 — flip the `HOST` line** (SPEC §9 B1): the `metadata` type is CONSUMED for **`REQUEST` + `ROUTE` + `HOST`** (three of the four MetadataKinds), naming the HOST source as the **selected upstream endpoint's `lb_endpoints[].metadata.filter_metadata[ns]`** (namespace + `path` walk over the new `cluster.Endpoint.MetaLookup` accessor), serialized to a STRING (string→raw; number/bool→scalar; struct/list→compact JSON — the shared `google.protobuf.Value` rendering), emitted as a `{tag, value}` span attribute on BOTH exporters; the `request_header` default rule (`default_value` when absent/unresolvable and non-empty; a present-empty value emits `""`; unresolvable-with-empty-default omits). **The departure list narrows from `CLUSTER`/`HOST`/unset-A to `CLUSTER`/unset-A.**
- [ ] **B2 — a NEW departure clause** (SPEC §9 B2) — **write the scope EXACTLY; do not generalize:** a HOST-kind tag resolves at **UPSTREAM-SELECTION (load-balancer pick) time on the reference**, so it emits the picked endpoint's metadata **even when the upstream connect then FAILS**; envoy-go's emit seam carries the ZERO `Endpoint` on every failure sub-path and therefore falls to `default_value`/omit there. **EXECUTED-proven for connect-failure 503 only** (P6 arm C); **structurally identical on envoy-go's side for pool-overflow 503 but the reference is UNPROBED there**; circuit-breaker 503 is likely parity (no pick occurs on either side — the `TryAcquireRequest` gate precedes any pick) and **also unprobed**. **PARITY holds on the post-Decode local-reply class.** ⚠️ **The reference's other two no-host arms have NO envoy-go counterpart and must NOT be written up as parity:** a 404 no-match emits **NO SPAN AT ALL** in envoy-go (all three no-match sites are PRE-Decide with a nil `traceDecision`; a PRE-EXISTING documented departure), and a zero-endpoint cluster is **BOOT-REJECTED** (`manager.go:890`). Note the gap is **PRE-EXISTING** — it already leaves the access-log upstream-host field empty on the same paths (pinned by three landed unit tests and by NO differential fixture); phase 72 adds a fourth consumer of it, it does not create it.
- [ ] **B3 — a documented BOUNDARY** (pre-existing, discovered at the SPEC): the reference's **Zipkin** serializer DROPS empty-string tag values (and the empty `node_id`/`zone` built-ins) where OTLP emits `""`; plus the newly-quantified number-precision and **arbitrary-struct-key-order** serialization edges carried from ADR-0292. Equally affects the landed REQUEST/ROUTE kinds; NOT fixed here.
- [ ] **B4 — the deferred-field list** (SPEC §9 B4, `:741`): the tracing `metadata` boundary line narrows to **`CLUSTER`/unset-A**.
- [ ] **Verify UNCHANGED:** the `literal`/`request_header`/`environment`/`metadata`-REQUEST/`metadata`-ROUTE lines + `max_path_tag_length` + the 4-empty-built-in-span-attrs neighborhood (`reference_tracing_upstream_cluster_framework_gap` — do NOT conflate the `upstream_cluster` span-TAG framework gap with the HOST MetadataKind; they are different features).

**Commit:** `docs(phase 72 T7): BEHAVIOR_CONTRACT B1–B4 — flip the custom_tags metadata HOST line to CONSUMED (the selected endpoint's lb_endpoints[].metadata.filter_metadata path-walk → serialized string → {tag,value} on both exporters; the request_header default rule); a NEW named departure — the reference resolves a HOST tag at LB-PICK time so it emits the picked endpoint's metadata even on a failed upstream connect, where envoy-go carries the zero Endpoint (EXECUTED for connect-failure only; pool-overflow + circuit-breaker unprobed; PRE-EXISTING, scoped out); the Zipkin empty-drop + serialization boundaries; narrow the departures to CLUSTER/unset-A. Three of four MetadataKinds now consumed`

---

## Task 8 — VERIFY: the six-gate + layering check + full differential + `-race` + counts + envelope audit

Controller-run on the frozen pre-stage-close HEAD:

- [ ] 1. `gofmt -l internal/ test/ cmd/` — SILENT
- [ ] 2. `go vet ./...` — exit 0
- [ ] 3. `go build ./...` — exit 0
- [ ] 4. `go mod tidy -diff` EMPTY + `git diff --exit-code master -- go.mod go.sum` EMPTY (**+0 modules** — RD-MOD; the only new import repo-wide is `structpb` into `internal/cluster/cluster.go`, an existing module)
- [ ] 5. `golangci-lint run ./...` — exit 0
- [ ] 6. **FULL differential:** `go test ./test/differential/ -count=1` — all **118** dirs, exit 0. The 117 pre-existing dirs byte-stable. `reference_differential_fullsuite_startup_flake`: a `subject ready: EOF` on an UNRELATED fixture is a startup race — isolate-re-run.

**Plus:**
- [ ] **Layering check:** `go list -deps ./internal/tracing | grep -E 'envoy-go/internal/(filter|cluster)'` (**no `...`**) ⇒ **EMPTY** — `internal/tracing` stays filter-free AND cluster-free (the `picked.MetaLookup` closure is built on the HCM side). ⚠️ **This is a DISCIPLINE check, not a cycle check** — neither edge would cycle (RD-LAYERING, re-executed: `internal/cluster` does not depend on `internal/tracing`). `reference_xds_config_seam_transitive_cycle_guard` (TYPE-level).
- [ ] **`-race` on touched packages:** `go test ./internal/cluster/ ./internal/tracing/ ./internal/filter/hcm/ -race -count=1`. ⚠️ **`internal/cluster` is NEWLY load-bearing this row** — `TestOutlierDetector_ConcurrentEjectExactlyOnce` (`outlier_test.go:766`) is a PRE-EXISTING intermittent failure reproduced on the UNMODIFIED baseline (RD-RACEFLAKE). **Isolate-re-run; do NOT re-classify as a phase-72 regression** unless it reproduces deterministically or the failure text differs.
- [ ] **Counts MECHANICAL, never copied:** fixtures **118** (tail `0116-tracing-custom-tags-metadata-host`) · fuzzers **55** (`^func Fuzz`) · BackendKind **38** · DECISIONS tail **ADR-0294** · stat surface **1201** (+0 — a span attribute registers no stat; there is NO mechanical stat command, and **NO `TestNoNewStat` guard is owed for a tracing row** — do not manufacture one) · go.mod diff EMPTY.
- [ ] **Envelope audit — pin the SHAPE for `internal/cluster`, BYTE-UNTOUCHED for the rest.** `git diff master --stat` shows functional production = `internal/cluster/{cluster.go,manager.go}` + `internal/tracing/{config.go,resolve.go}` + `internal/filter/hcm/accesslog_emit.go` ONLY.
  - **`internal/cluster` SHAPE (cannot assert byte-untouched):** exactly ONE added field + ONE added accessor + ONE added import in `cluster.go`; exactly ONE changed line in `manager.go` (`:884`). **`subset.go` BYTE-UNTOUCHED**; `manager.go:883` (the `ScalarsFromStruct` projection) and `:754` (`defaultSubset`) **byte-unchanged** — verify with `git diff`.
  - **BYTE-UNTOUCHED (assert):** `internal/filter/hcm/{connection.go,h2dispatch.go,h3dispatch.go}` (**all 18 emit callers — the row's central cost claim**) · `internal/filter/http/chain.go` · `internal/filter/hcm/accesslog_emit_test.go` · `internal/xds` · `internal/tls` · `internal/boot` · `internal/listener` · `internal/bootstrap` · `validate/` · `internal/dynamicmetadata` · `internal/filter/http/{ratelimit,lua}`.
  - **New exported symbols ONLY:** `cluster.Endpoint.MetaLookup` + the `ResolveCustomTags` signature growth. ZERO new packages/modules/stats/BackendKinds; the `internal/xds` zero-new-symbol discipline UNTOUCHED.

*(No separate commit — T8's evidence lands in PROGRESS at T9.)*

---

## Task 9 — ADR-0294 completed IN PLACE + stage-close (controller-adjacent)

- [ ] **ADR-0294: COMPLETE IN PLACE** — **APPEND** `### Decision (landed at the phase-72 IMPL)` + `### Consequences` **after the existing `*(§Decision + §Consequences land at the phase-72 IMPL.)*` hand-off line**. Flip the STATUS banner to **COMPLETE**. **Do NOT append a new ADR; do NOT renumber; do NOT create a SECOND `### Decision` heading** — the SPEC deliberately left none (the ADR-0293 shape; SPEC §16 F1/F2). Tail stays ADR-0294; next-free ADR-0295 (`grep -c '^## ADR-0295'` → 0). §Decision records the landed mechanism (the `Endpoint.filterMetadata` field + `MetaLookup` accessor + the `manager.go:884` populate; `kindMetadataHost` at iota==5 + the HOST accept arm; the `hostMetaLookup` 5th param + the FULL-`MetaPath` resolve arm; `picked.MetaLookup` at the three emit call sites with signatures + 18 callers untouched; the `0116` fixture); §Consequences records the counts, the named departures, and the memory updates.
  - [ ] ⚠️ **CORRECT THE STALE JUSTIFICATION (F4 / RD-LAYERING).** The landed §Context asserts the closure is *"mandatory rather than stylistic, since `internal/tracing` must not import `internal/cluster` (the cycle guard)"* — **a claim SPEC §16's V1 finding M3 REFUTED and this PLAN re-executed** (`internal/cluster` does not depend on `internal/tracing`; a direct import builds clean). **Do NOT propagate it into §Decision.** State it correctly: the closure preserves a **self-imposed LAYERING rule** that the `go list -deps` gate asserts — a good reason; "the cycle guard forces it" is a false one. Fix the §Context sentence in place at the same edit.
- [ ] **ROADMAP row 72 → `done`** at the six-gate (ADR-0106, SOLE leg; `reference_roadmap_split_phase_row_done`). **NARROW the deferred sentence NOW (and ONLY now):** roll the **`HOST`** MetadataKind OUT of the live Observability `candidates:` sentence, leaving **`CLUSTER`**-only (the phase-57 precedent — SPEC §12; the sentence STAYS a live `candidates:` match afterward). Keep EXACTLY ONE live Observability `candidates:` match; HTTP/3 + xDS untouched (three total).
- [ ] **STATE.md:** edit §Current pointer **IN PLACE** (lifecycle 3 → DONE; row 72 `done`); demote to §Recent lineage **capped at five** — and **update the lineage PREAMBLE too** (naming the correct newest + dropped bullet; the ADR-0288 rule exists to keep this file grep-trustworthy, and a stale preamble was V2's F3 at the SPEC). Update counts (fixtures 118, DECISIONS ADR-0294 COMPLETE).
- [ ] **PROGRESS.md:** finalize — every break's ACTUAL firing assertion, the verbatim red-first records, the T4 cost-claim hash verification, the T5 dispatch-verify trap result, any break substitutions, and any break that did NOT fire.
- [ ] **Router roll** (`next-prompt.txt` — TRACKED despite .gitignore; edit in the stage worktree; locate commits by SUBJECT). Row 72 done ⇒ the sentinel's check (1) goes SILENT ⇒ the roller SELF-PICKS the next subject at the phase-73 BRAINSTORM (the 2026-07-12 standing directive) unless the sentinel fires (it does not: checks (2)+(3) still print). ⚠️ **The router's own status text currently repeats the refuted "cycle guard" justification — do not carry it forward.**
- [ ] **Sentinel re-run MECHANICALLY:** (1) goes silent when row 72 flips (every OTHER chartered row already `done`); (2) still prints **3** via the full-phrase command (the HOST narrowing does NOT drop the whole Observability sentence); (3) unchanged (`NEVER OPENED: gRPC/Runtime/WASM`) ⇒ does NOT fire; no `stop` file.
- [ ] **Memory updates owed (PLAN-surfaced — SPEC §13 is "Deferred items" and carries NO memory obligation):** (i) **`cluster.Endpoint.MetaLookup`** — the HOST analogue of phase-70's `DynamicMetadata()` and phase-71's `RouteMetaLookup()`; the FIRST endpoint-side metadata seam, and the first row to touch `internal/cluster` in this lineage (extends `reference_filterchain_dynamicmetadata_accessor`). (ii) **`envoy.lb` is NOT privileged** — ANY `filter_metadata` namespace is addressable by a `metadata_key`, so the phase-38 scalars-only projection is insufficient for metadata resolution (P2). (iii) **The reference resolves a HOST tag at LB-PICK time, not on upstream success** — with the pick-time `picked` propagation gap named as its own deferred candidate (B2). (iv) **`LocalityLbEndpoints.metadata` contributes NOTHING** to HOST-kind resolution (P7) — no departure owed for retaining none.
- [ ] **Squash-push by the controller** at stage-close.

**Commit (stage-close docs):** `phase 72 (tracing-custom-tags-metadata-host) IMPL: …` (controller composes at close).

---

## Self-review against SPEC-72

| SPEC obligation | Where |
|---|---|
| the `Endpoint.filterMetadata` field + nil-safe `MetaLookup` accessor + the `manager.go:884` populate, projection byte-unchanged (§3.5, S1) | T1 |
| the `kindMetadataHost` const APPENDED at iota==5 (§3.2) | T2 |
| the HOST accept arm (cloning ROUTE, `MetaPath` FULL) replacing the `:267-268` reject; CLUSTER/unset rejects UNCHANGED (§3.2/§3.6/§6) | T2 |
| the `hostMetaLookup` 5th param + the `kindMetadataHost` resolve arm, the `request_header` default rule, the FULL-path descent (§3.3/§3.4) | T3 |
| `descend`/`structpbValueToString` REUSED VERBATIM; `internal/tracing` stays filter- AND cluster-free (§3.3, §4) | T3, T8 (layering check) |
| `picked.MetaLookup` at the three `ResolveCustomTags` call sites; the 3 signatures + 18 callers + 29 emit test callers BYTE-UNTOUCHED (§1, §3.3, §11) | T4, T8 |
| a first-class zero-`picked` test — the 5 span-capable zero-endpoint sites (§3.6, §11) | T4 |
| the purely-ADDITIVE `meta_host_ok` fuzz seed (NO `meta_bad` repoint) + dispatch-verify + the stale-comment narrow (§7, S6) | T5 |
| ONE OTLP fixture `0116`, two HOST custom_tags, cross-side EXACT key+value, **STRING values only**, NO writer (§8, S4) | T6 |
| BC B1–B4 pinned wording, with B2's scope kept EXACT (§9) | T7 |
| a SINGLE FLAT ROW of NINE tasks; ADR-0045 valve armable-but-unconsumed (§10) | §1, this table |
| six-gate + layering check + full-118-dir + `-race` (incl. the newly load-bearing `internal/cluster`) + counts + SHAPE-pinned envelope audit (§10 T8, §15) | T8 |
| +0 packages / +0 modules / +0 stats / +0 fuzzers / +0 BackendKinds (§4, §7) | T5, T8 |
| ADR-0294 completed IN PLACE, no new ADR, no second §Decision heading (§14) | T9 |
| Sentinel: narrow the sentence AT THE IMPL row-done, not before (§12) | T9 |
| Memory updates (PLAN-surfaced, not a SPEC obligation) | T9 |
| **+ PLAN-surfaced:** the 7 stale production comment lines (4 groups) + 2 stale test comments (F1/F2) | T2 |
| **+ PLAN-surfaced:** the ADR-0294 §Context "cycle guard" correction (F4) | T9 |

**Task count: 9** — matching SPEC §10's enumerated NINE (comfortably under the ADR-0045 ~15 ceiling). **ADR-0045 escape valve ARMABLE, UNCONSUMED — no split**: the one thing that could have forced one, the `internal/cluster` widening, turned up **zero** behavioral disturbance (RD-NORIPPLE — never a map key, never `==`-compared, never serialized, two `Metadata` consumers, one production literal, zero positional test literals). Sequencing: **T1** (cluster substrate, independent) → **T2** (parse) → **T3** (resolve, consumes T2's `kindMetadataHost`) → **T4** (wires T1's accessor + T3's 5th param at the three call sites) → **T5** (fuzz) → **T6** (fixture) → **T7** (BC) → **T8/T9** (close).

**⚠️ The IMPL's standing instruction: a PLAN is not evidence either.** **RE-DERIVE this document; do not execute it.** Where it cites, go look; where it claims control flow, walk the call graph; default to REFUTED. Start where this PLAN is most confident (all re-derived read-only at the PLAN tip, §1): the 68/69 EXACT-HOLD anchor set, the clone skeletons (§1.2, cloning the LANDED phase-71 ROUTE arms verbatim in their **gofmt-clean, misspell-clean** form), the `picked`-already-in-scope cost claim (RD-SEAM — CONFIRMED BY HASH at the PLAN's adversarial pass), the 18-caller roster with its span-capability partition (RD-CALLERS/RD-SPANCAP), and the ONE corrected anchor (**RD-LIT** — state the claim, **do not copy any of the three figures**: 167, 177 and 209 all fail to reproduce; the load-bearing "zero positional literals" is what was verified). ⚠️ **And note what adversarial verification did to an earlier draft of this very document: it REFUTED one of the PLAN's own "drift corrections" (RD-FUZZ) and caught a red-classification that was backwards (T4). A PLAN's corrections are not evidence either.**
