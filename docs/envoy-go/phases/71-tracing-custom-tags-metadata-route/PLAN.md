# PLAN 71 — tracing `custom_tags` `metadata` type, `ROUTE` MetadataKind only — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL — `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Stage:** PLAN (lifecycle-state 2 → 3). Docs-only — ZERO production `.go`. Worktree `.worktrees/phase-71-plan`, branch `phase-71-tracing-custom-tags-metadata-route-plan`, tip **`60f12146`** (the phase-71 SPEC squash — master; production code byte-identical to the BRAINSTORM tree `fbc86063` since the SPEC was docs-only apart from the ADR-0293 §Context append), per `feedback_git_worktrees`.
>
> **Row 71 STAYS `in-progress`** — the IMPL flips it `done` at its six-gate (ADR-0106, the SOLE leg — a SINGLE FLAT ROW, §10). **ADR-0293's §Context is ALREADY DRAFTED** at the SPEC squash (`grep -n '^## ADR-0293' docs/envoy-go/DECISIONS.md`, STATUS: **PROPOSED**); the IMPL **COMPLETES ADR-0293 IN PLACE** with §Decision + §Consequences — it does NOT append a new ADR, does NOT renumber. DECISIONS tail stays **ADR-0293**, next-free **ADR-0294** (`[RUN]`: `grep -c '^## ADR-0294' docs/envoy-go/DECISIONS.md` → 0). **This PLAN adds NO ADR content; DECISIONS is UNTOUCHED at the PLAN.**
>
> **Baselines RE-DERIVED at `60f12146` (`[RUN]`, NOT copied):** fixtures **116** (`ls -d test/fixtures/[0-9]*/ | wc -l`; numeric tail `0114-tracing-custom-tags-metadata` ⇒ next fixture `0115`; tracing/OTLP fixtures TEMPLATE all ports — no in-container port to pick) · fuzzers **55** (`grep -rn '^func Fuzz' --include='*.go' internal/ test/ | wc -l`) · BackendKind tail **38** (`H2GoawayResponder`) · stat surface **1201** · DECISIONS tail **ADR-0293** (PROPOSED; next-free ADR-0294) · go.mod modules **2** (lineage figure; `corev3`/`structpb`/`type/metadata/v3`/`protojson` all already in the touched packages' dep closure — re-check `git diff go.mod` after tidy at T8).
>
> **Sentinel expectation:** check (1) prints `NOT DONE: row 71`; check (2) prints **3** via the full-phrase form ONLY — `grep -cE 'remaining deferred \(not-yet-chartered\) candidates:' docs/envoy-go/ROADMAP.md` (`reference_sentinel_deferred_sentence_live_vs_historical` — cite the command, never the adjective); check (3) prints `NEVER OPENED: gRPC/Runtime/WASM`. **No deferred-sentence edit at ANY stage of this row** (SPEC §12); the live Observability `candidates:` sentence rolls the `ROUTE` MetadataKind OUT at the IMPL row-done edit, NOT before (the phase-57 graphite precedent).
>
> **⚠️ NO PARALLEL STREAM.** Master (`60f12146`) IS the SPEC squash — the ONLY delta over the BRAINSTORM tree `fbc86063` is docs (`SPEC.md` + the ADR-0293 §Context append). So the production tree is byte-identical to what the SPEC re-derived; §1 is the structural decisions the SPEC delegated to the PLAN plus a full re-verification (which found **ZERO drift** — every SPEC §11 anchor exact at `60f12146`, 32/32 EXACT-HOLD).
>
> **⚠️ RE-DERIVE, do not execute.** A PLAN is not evidence (a PLAN's cites drift; a SPEC's do too). Where this document cites, go look; where it claims control flow, walk the call graph; default to REFUTED (`feedback_brief_citations_not_evidence`, `reference_quoting_is_not_executing`).

---

## 1. Re-derivation ledger — every SPEC §11 anchor re-opened at `60f12146`

**All SPEC §3/§9/§11 code anchors RE-DERIVED at `60f12146` by an independent read-only re-derivation agent that re-opened `internal/tracing/{config.go,resolve.go}`, `internal/filter/http/chain.go`, `internal/filter/hcm/{accesslog_emit.go,connection.go,h2dispatch.go,h3dispatch.go,fuzz_test.go}`, the go-control-plane `custom_tag.pb.go`/`metadata.pb.go`/`base.pb.go`/`struct.pb.go` proto shapes, the test files, and the `0114` fixture chassis.**

**RESULT: 32 / 32 anchor groups EXACT-HOLD — ZERO drift.** Every SPEC-cited line matched the current master tip EXACTLY. (Master `60f12146` is one commit ahead of the SPEC's cited re-derivation commit `fbc86063`, but NO line numbers shifted — the SPEC squash was docs-only apart from the DECISIONS append.) The SPEC's cites are adopted verbatim. All three new identifiers (`kindMetadataRoute` / `routeMetaLookup` / `RouteMetaLookup`) are collision-free. `go build ./...` clean. The findings below (RD*) are therefore the *structural decisions the SPEC delegated to the PLAN* (the exact code skeletons an implementer clones) plus the load-bearing re-confirmations; adversarial verification (§1.2) must confirm or refute each.

| # | Anchor / SPEC claim | RE-DERIVED at `60f12146` | Where |
|---|---|---|---|
| **RD-EXACT** | SPEC §11 cites ~30 code anchors | **ALL EXACT — ZERO drift.** 32/32 anchor groups hold at the actual line numbers; the SPEC's cites are adopted verbatim. | all |
| **RD-KIND** | SPEC §3.2: add `kindMetadataRoute` to the `customTagKind` iota | **CONFIRMED.** `customTagKind` block `config.go:53-58` = `kindLiteral`(0,`:54`) / `kindRequestHeader`(1,`:55`) / `kindEnvironment`(2,`:56`) / `kindMetadata`(3,`:57`). ADD `kindMetadataRoute` at iota==4 (a new line before the `:58` closing paren). `CustomTagSpec` `config.go:65-75` already has `MetaNamespace string`/`MetaPath []string`/`DefaultValue string`/`HasDefault bool` — **NO new field**; the ROUTE spec reuses them exactly like REQUEST. | T1 |
| **RD-REJECT** | SPEC §3.2/§3.6: REPLACE the ROUTE reject (`:245-246`) with an accept arm cloning REQUEST (`:228-244`) | **CONFIRMED — the exact ROUTE reject is `config.go:245-246`:** `case k.GetRoute() != nil:` → `return nil, fmt.Errorf("tracing: custom_tags metadata tag %q route kind unsupported", tag)`. This whole `case` is REPLACED by an accept arm building `CustomTagSpec{Kind: kindMetadataRoute, MetaNamespace, MetaPath (FULL), DefaultValue, HasDefault}` (§1.1). The `k := md.GetKind()` bind `:224` (V1-SEVERE receiver — getters on `*metadatav3.MetadataKind`), the unset-A `k==nil` reject `:226-227`, the REQUEST accept `:228-244`, the CLUSTER reject `:247-248`, the HOST reject `:249-250`, and the `default:` unset-B `:251-252` are ALL EXACT. The empty-tag reject (`:198-200`) and first-wins dedup (`:257-261`) are independent and UNCHANGED (a later same-key ROUTE tag drops after structural validation). All retained reject substrings ADR-0080-distinct. | T1 |
| **RD-NOIMPORT-CFG** | SPEC §3.1/§11: config.go needs NO new import | **CONFIRMED — and SHARPENED.** config.go import block `:7-16` does NOT import `metadatav3` and does NOT need to: `k := md.GetKind()` yields `*metadatav3.MetadataKind` by INFERENCE (never spelled), and the new ROUTE arm calls `k.GetRoute()` on the inferred `k` — exactly as the landed REQUEST/CLUSTER/HOST arms do. `kindMetadataRoute` is a package const. **The phase-71 ROUTE arm adds ZERO imports to config.go.** (`fuzz_test.go` imports `metadatav3` for the seed literals — that stays; T5.) | T1 |
| **RD-RESOLVE-4TH** | SPEC §3.3/S1: `ResolveCustomTags` grows a FOURTH nil-tolerant `routeMetaLookup` param, DISTINCT from the 3rd `metaLookup` | **CONFIRMED — the 3-param signature is exact at `resolve.go:26`:** `func ResolveCustomTags(specs []CustomTagSpec, headerLookup func(string) ([]string, bool), metaLookup func(ns, key string) (*structpb.Value, bool)) []KV`. Phase 71 appends a 4th param `routeMetaLookup func(ns string) (*structpb.Value, bool)` (ONE-arg — the namespace only, NOT `(ns, key)`; the ROUTE source yields the whole namespace struct, so there is no pre-keyed first segment). The 3rd `metaLookup` is UNTOUCHED. | T2 |
| **RD-ROUTE-ARM** | SPEC §3.3/S1: a `kindMetadataRoute` arm descending the FULL `MetaPath` (NOT `[1:]`) | **CONFIRMED — the REQUEST arm to mirror is `resolve.go:65-88`:** `metaLookup(s.MetaNamespace, s.MetaPath[0])` (`:75`) → `descend(v, s.MetaPath[1:])` (`:78`) → `structpbValueToString` (`:81`) → `HasDefault` fallback (`:86-87`). The ROUTE arm clones the *default rule* (present-empty EMITS `""`) but: (a) calls `routeMetaLookup(s.MetaNamespace)` (one-arg); (b) descends `descend(v, s.MetaPath)` — the **FULL** path (the `[1:]` slice is a REQUEST-only Bucket-pre-keying artifact — the Bucket pre-consumes `MetaPath[0]`; the ROUTE lookup returns the whole namespace struct, so all segments remain to walk). `descend`/`structpbValueToString` REUSED VERBATIM. | T2 |
| **RD-DESCEND** | SPEC §3.3: `descend` reusable verbatim | **CONFIRMED — `resolve.go:100-114`:** `func descend(v *structpb.Value, segs []string) (*structpb.Value, bool)` — StructValue-walk; non-struct intermediate / missing field / nil field → `(nil,false)`; empty `segs` → `(v, true)` (the terminal is `v` itself, so a single-segment ROUTE path resolving to the namespace struct's own field works). Proto-agnostic, source-shape-independent → reused BY THE ROUTE ARM UNCHANGED. `internal/tracing` gains NO import of a filter package (T8 cycle guard). | T2 |
| **RD-SERIALIZE** | SPEC §3.3: `structpbValueToString` reusable verbatim | **CONFIRMED — `resolve.go:123-140`:** `*structpb.Value_StringValue` → `k.StringValue` (raw, incl. `""` — present-empty EMIT); `*structpb.Value_NullValue` → `("",false)` (boundary → default/omit); else `protojson.Marshal(v)` + `json.Compact` (number canonical decimal / bool `true`/`false` / struct·list compact JSON; `json.Compact` LOAD-BEARING — strips detrand whitespace, V1-executed at phase 70). Reused by the ROUTE arm UNCHANGED. `resolve.go` imports `structpb` at `:9`; `NewStructValue` is NOT used in resolve.go (it lives on the emit side — RD-SEAM). **`resolve.go` needs NO new import for phase 71.** Run `internal/tracing` under `-race` (`reference_detrand_race_catches_protojson_value_substring`). | T2 |
| **RD-DEFAULT** | SPEC §3.4: the `request_header` default rule (present-empty EMITS `""`), NOT `environment` | **CONFIRMED — the ROUTE arm mirrors the REQUEST arm's `if s.HasDefault { … } // else omit` tail (`resolve.go:86-88`), NOT the `kindEnvironment` omit-on-empty rule.** `HasDefault = DefaultValue != ""` at parse (T1). Resolve-if-resolved (present-empty `""` → emits `""`); else `default_value` if non-empty; else omit. | T2 |
| **RD-SEAM** | SPEC §3.5/S2: a NEW `func (c *FilterChain) RouteMetaLookup(ns string) (*structpb.Value, bool)` on `internal/filter/http` + the `structpb` import | **CONFIRMED — the load-bearing S2 decision.** `chain.go` does NOT import `structpb` (`grep -c structpb chain.go` → 0; import block re-derived); the new method ADDS it. Placement: beside `DynamicMetadata()` `:1022` and `RouteMetadata()` `:1156` / `SetRouteMetadata()` `:1150`. Body: `st := c.RouteMetadata().GetFilterMetadata()[ns]; if st == nil { return nil, false }; return structpb.NewStructValue(st), true` (§1.1). `RouteMetadata()` `:1156` returns `c.routeMetadata *corev3.Metadata` (field `:199`); `GetFilterMetadata()` (`base.pb.go:851`) returns `map[string]*structpb.Struct`, nil-receiver-safe (a nil `*corev3.Metadata` → nil map → nil `*structpb.Struct` → `(nil,false)`); `structpb.NewStructValue` (`struct.pb.go:396`) is `func(*structpb.Struct) *structpb.Value`. This centralizes `structpb` to `chain.go` alone (cleaner than adding it to the three dispatch files via inline closures) and threads as a method value `chain.RouteMetaLookup`, SYMMETRIC with the landed `chain.DynamicMetadata().Get`. | T3 |
| **RD-CALLERS** | SPEC §3.5/§11: thread `routeMetaLookup` at all 18 emit callers; `nil` at the 3 no-chain 404 sites | **CONFIRMED EXACT — 5+6+7=18, exactly 3 no-chain-404 sites, `chain` var name uniform.** `emitAccessLog`/`H2`/`H3` signatures `accesslog_emit.go:27/:87/:149`; `ResolveCustomTags` calls `:57/:118/:179` (all currently THREE-arg `specs, headerLookup, metaLookup` — add a 4th `routeMetaLookup`); span-gate `:30/:89/:151` (`statusCode != 0 && f.exporter != nil && traceDecision != nil && traceDecision.Sample`). **The 3 nil sites:** `connection.go:330` / `h2dispatch.go:313` / `h3dispatch.go:130` — all BEFORE the chain is built (`traceDecision`/`c.traceDecision` nil), so `ResolveCustomTags` is provably never reached; pass a SECOND `nil`. **The 15 chain sites:** `connection.go:464/:597/:699/:777` · `h2dispatch.go:396/:530/:577/:584/:613` · `h3dispatch.go:210/:280/:341/:367/:373/:395` — all have `chain` in scope; pass `chain.RouteMetaLookup` as the NEW trailing (4th) arg. **KEY MECHANIC:** the existing 3rd arg (`nil` at the nil sites, `chain.DynamicMetadata().Get` at the 15 chain sites) is UNTOUCHED — phase 71 APPENDS the 4th arg; it does NOT rewrite the metaLookup arg. **Subtlety (harmless):** `h3dispatch.go:210` has `chain` in scope but `traceDecision` still nil there (Decide runs later) — a chain site, `chain.RouteMetaLookup` valid-but-never-reached (NOT a nil site). `accesslog_emit.go` already imports `structpb` (`:9`) — the `routeMetaLookup` param type reuses it, NO new import there. | T4 |
| **RD-SEED-SEEDED** | SPEC §1 (landed substrate): `SetRouteMetadata` seeds `RouteMetadata()` per-request at the three dispatch sites | **CONFIRMED — the ROUTE source is seeded:** `connection.go:433` `chain.SetRouteMetadata(entry.metadata)` (H1) · `h2dispatch.go:375` `chain.SetRouteMetadata(c.routeMetadata)` (H2) · `h3dispatch.go:195` `chain.SetRouteMetadata(entry.metadata)` (H3). So at the 15 span-capable emit sites `chain.RouteMetadata()` returns the matched route's static config metadata (or nil when the route carries none → `RouteMetaLookup` returns `(nil,false)` → default/omit). NO change to these seed sites (read-only substrate). | T4 (context) |
| **RD-FUZZ** | SPEC §7/S4: repurpose the `withMetaTags` seed (add a valid ROUTE accept; repoint `meta_bad` to CLUSTER); dispatch-verified | **CONFIRMED — host + seed + dispatch.** `FuzzHCMConfigParse` host; `withMetaTags` seed `fuzz_test.go:94-108`: `meta_ok` = REQUEST (`MetadataKind_Request_`, MetadataKey `{Key:"envoy.test", Path:[{Key:"k"}]}`, `DefaultValue:"fb"`, `:97-101`); **`meta_bad` = `MetadataKind_Route_` with NO MetadataKey (`:102-104`)** — TODAY it hits the ROUTE-reject arm (`:245-246`); AFTER the lift it would hit the ROUTE-ACCEPT arm then the empty-namespace reject (a DIFFERENT arm). **REPURPOSE (S4):** (a) ADD a valid ROUTE-accept tag `meta_route_ok` (`MetadataKind_Route_`, MetadataKey `{Key:"envoy.test", Path:[{Key:"k"}]}`, a default) exercising the NEW accept arm; (b) REPOINT `meta_bad`'s kind from `Route` to `Cluster` (keeping it MetadataKey-less) so a kind-unsupported reject arm stays exercised. Wrapper spellings present in the file: `CustomTag_Metadata_`/`CustomTag_Metadata`/`MetadataKind_Request_`/`MetadataKind_Route_`/`MetadataKey`/`MetadataKey_PathSegment`/`MetadataKey_PathSegment_Key` — the ROUTE ones ALREADY imported. **Dispatch CONFIRMED:** `parseCustomTags` is called at `config.go:126`, the `"provider required"` reject at `:136` (126 < 136), so a well-formed HCM tracing block reaches the metadata arm BEFORE the provider check. Count STAYS 55 (`^func Fuzz`; a seed is +0). | T5 |
| **RD-FIXTURE** | SPEC §8: ONE new OTLP `0115`, static route metadata, NO writer | **CONFIRMED — `0114` chassis re-derived.** `0114-tracing-custom-tags-metadata/` = `driver/driver.go`, `envoy.yaml`, `envoy-go.yaml`, `expectations.yaml`, `README.md`, `scripts/writer.lua`. **`0114` HAS a Lua writer (`scripts/writer.lua` does `dynamicMetadata():set("envoy.test","meta_k","v-meta-0114")`) and two REQUEST `metadata` custom_tags (`meta_hit`→the Lua value, `meta_default`→`default_value: fallback-0114`); route `rc_test`/`vh_test` prefix `/`→`c_backend`.** **`0115` clones this OTLP chassis but REMOVES the Lua filter + script + REQUEST tags** and instead: (a) puts a `metadata: {filter_metadata: {<ns>: {<key>: "<fixed>"}}}` block on the matched route (BOTH YAMLs, byte-identical); (b) two ROUTE `metadata` custom_tags — `route_hit` (path `[<key>]`→the static value) + `route_default` (path `[<absent-key>]`→`default_value`). NO writer (both sides read identical static route config → cross-side EXACT). Fixtures **116 → 117**; all ports templated (no port to pick). | T6 |
| **RD-TESTCALLERS** | SPEC §10 T2/T4: existing direct callers need a trailing `nil` | **CONFIRMED.** `resolve_test.go` has **20 direct `ResolveCustomTags(` call sites** (lines 34,72,97,105,124,131,138,146,188,198,207,216,244,254,268,276,284,292,304,311) — all 3-arg; each gains a trailing `nil` (4th `routeMetaLookup`, no ROUTE tags). Two hcm test files call the emit methods — `accesslog_emit_test.go` + `span_emit_test.go` — each caller gains a trailing `nil` (the emit methods grow the 4th param). Grep the EXACT caller lines at the IMPL tip (a rename would surface as a build error). | T2, T4 |
| **RD-IDENT** | SPEC §5: collision checks | **ALL FREE at `60f12146`.** `grep -rn 'kindMetadataRoute\|routeMetaLookup\|RouteMetaLookup' --include='*.go' internal/ test/` → ZERO each (also `RouteMetaLookup(ns` → 0). Fixture `0115-tracing-custom-tags-metadata-route` does not exist; no in-container port to claim. | T1, T2, T3, T6 |
| **RD-MOD** | SPEC §4: +0 go.mod modules | **CONFIRMED buildable.** `corev3` is already imported by `chain.go` (the `RouteMetadata()` return type) and everywhere in `internal/filter/hcm`; `structpb` resolves at the transitive `google.golang.org/protobuf` (already an `internal/dynamicmetadata`/`internal/tracing` dep); `type/metadata/v3` (`metadatav3`) resolves at the direct `go-control-plane/envoy v1.32.4` (already in `fuzz_test.go`); `protojson` already in `resolve.go`. **The ONLY new IMPORT is `structpb` into `chain.go` — an EXISTING module (`+0` go.mod).** `go mod tidy -diff` anticipated EMPTY — re-check `git diff go.mod` after tidy at T8 (`reference_new_subpackage_pulls_transitive_module`). | T3, T8 |

### 1.1 The clone skeletons the SPEC delegated to the PLAN (each RE-DERIVED verbatim, not invented)

- **The `parseCustomTags` ROUTE arm (T1, `config.go`)** — REPLACES the `:245-246` `case k.GetRoute() != nil:` reject; clones the REQUEST accept (`:228-244`) verbatim except `Kind: kindMetadataRoute`:
  ```go
  case k.GetRoute() != nil:
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
      spec = CustomTagSpec{Key: tag, Kind: kindMetadataRoute, MetaNamespace: mk.GetKey(), MetaPath: path, DefaultValue: dv, HasDefault: dv != ""}
  ```
  **IDENTICAL to the REQUEST arm except `Kind: kindMetadataRoute`** — the `MetaPath` carries the FULL path (no `[1:]` slicing; that happens REQUEST-only at RESOLVE, not parse). The `k := md.GetKind()` bind (`:224`), the unset-A `:226-227`, CLUSTER `:247-248`, HOST `:249-250`, and `default:` unset-B `:251-252` are UNCHANGED. **⚠️ The four kind-getters (`GetRequest`/`GetRoute`/`GetCluster`/`GetHost`) are on `*metadatav3.MetadataKind` (returned by `md.GetKind()`), NOT on `*tracingv3.CustomTag_Metadata` (the phase-70 V1 SEVERE, still true at tip) — the arm branches on `k.GetRoute()`, and `k` is already bound. config.go adds NO import (RD-NOIMPORT-CFG).**
- **The `kindMetadataRoute` resolve arm (T2, `resolve.go`)** — mirrors the `kindMetadata` REQUEST arm (`:65-88`) default rule, but ONE-arg lookup + FULL-path descent:
  ```go
  case kindMetadataRoute:
      // Mirror kindMetadata's default rule (present-empty EMITS ""), but the ROUTE
      // source yields the WHOLE namespace struct, so descend the FULL MetaPath
      // (the [1:] slice is a REQUEST-only Bucket-pre-keying artifact).
      // routeMetaLookup may be nil (no route metadata) → default / omit.
      var v *structpb.Value
      var ok bool
      if routeMetaLookup != nil {
          v, ok = routeMetaLookup(s.MetaNamespace)
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
  `descend`/`structpbValueToString` REUSED VERBATIM (RD-DESCEND / RD-SERIALIZE). No new import in resolve.go.
- **The `ResolveCustomTags` signature growth (T2, `resolve.go:26`)** — a FOURTH param, DISTINCT from the 3rd:
  ```go
  func ResolveCustomTags(
      specs []CustomTagSpec,
      headerLookup func(string) ([]string, bool),
      metaLookup func(ns, key string) (*structpb.Value, bool),      // REQUEST — unchanged, two-arg
      routeMetaLookup func(ns string) (*structpb.Value, bool),      // ROUTE — NEW, one-arg, nil-tolerant
  ) []KV
  ```
- **The `RouteMetaLookup` accessor (T3, `chain.go`)** — beside `DynamicMetadata()` (`:1022`):
  ```go
  // RouteMetaLookup returns the matched route's static config metadata for the
  // namespace ns, wrapped as a structpb StructValue (or (nil,false) when the
  // route carries no metadata / the namespace is absent). Added phase 71 so the
  // HCM tracing emit sites can resolve a ROUTE-kind custom_tag metadata value;
  // the ROUTE analogue of DynamicMetadata(). Centralizes the structpb import to
  // chain.go (the three dispatch files stay structpb-free).
  func (c *FilterChain) RouteMetaLookup(ns string) (*structpb.Value, bool) {
      st := c.RouteMetadata().GetFilterMetadata()[ns]
      if st == nil {
          return nil, false
      }
      return structpb.NewStructValue(st), true
  }
  ```
  ADD the import `structpb "google.golang.org/protobuf/types/known/structpb"` to `chain.go` (re-derive the exact spelling/ordering at the tip — it is an EXISTING module, RD-MOD). `RouteMetadata()` is nil-receiver-safe via `GetFilterMetadata()`; the body is panic-free on a nil `routeMetadata`.
- **The ROUTE fuzz seed repurpose (T5, `fuzz_test.go:94-108`, S4)** — add a valid ROUTE accept + repoint `meta_bad` to CLUSTER:
  ```go
  // in withMetaTags, alongside the existing meta_ok (REQUEST) tag:
  {Tag: "meta_route_ok", Type: &tracingv3.CustomTag_Metadata_{Metadata: &tracingv3.CustomTag_Metadata{
      Kind:        &metadatav3.MetadataKind{Kind: &metadatav3.MetadataKind_Route_{Route: &metadatav3.MetadataKind_Route{}}},
      MetadataKey: &metadatav3.MetadataKey{Key: "envoy.test", Path: []*metadatav3.MetadataKey_PathSegment{{Segment: &metadatav3.MetadataKey_PathSegment_Key{Key: "k"}}}},
      DefaultValue: "fb",
  }}},
  // and REPOINT meta_bad from Route to Cluster (keep it MetadataKey-less → cluster-kind reject):
  {Tag: "meta_bad", Type: &tracingv3.CustomTag_Metadata_{Metadata: &tracingv3.CustomTag_Metadata{
      Kind: &metadatav3.MetadataKind{Kind: &metadatav3.MetadataKind_Cluster_{Cluster: &metadatav3.MetadataKind_Cluster{}}},
  }}},
  ```
  (Re-derive the EXACT `metadatav3.MetadataKind_Route_`/`_Cluster_`/`MetadataKey_PathSegment_Key` wrapper spellings at the IMPL tip — the go-control-plane oneof wrappers; `metadatav3` is already imported by `fuzz_test.go`. A fuzz `f.Add` only needs to REACH the arm — the accept/reject unit coverage is T1's `config_test.go`. `meta_ok` REQUEST stays as-is.)

### 1.2 Adversarial-pass record

**TWO independent verifiers ran against the draft in PRIVATE scratch before landing** (`reference_parallel_subagents_private_scratch`; the real repo left untouched, no worktrees registered).

- **V1 — code-claims re-derivation + by-execution compile probes.** **ZERO SEVERE, ZERO MODERATE, ZERO MINOR — BUILDABLE as written.** V1 independently re-derived every load-bearing skeleton at `60f12146` and ran compile/execution probes in private scratch (no worktree registered). All CONFIRMED: (1) the ROUTE accept skeleton is byte-identical to the REQUEST arm `:228-244` except `Kind: kindMetadataRoute` + FULL `MetaPath`; the four kind-getters are on `*metadatav3.MetadataKind` (the phase-70 SEVERE still holds — EXECUTED `(&metadatav3.MetadataKind{Kind:&MetadataKind_Route_{}}).GetRoute()` compiles + returns non-nil); config.go adds NO import (RD-NOIMPORT-CFG). (2) **The pivotal S1 full-path-descent equivalence EXECUTION-CONFIRMED:** built `nsStruct={outer:{inner:"deep"}}` and ran both paths — REQUEST `descend(nsStruct.Fields["outer"], ["inner"])` → `"deep"` == ROUTE `descend(NewStructValue(nsStruct), ["outer","inner"])` → `"deep"` (IDENTICAL); single-segment `["k"]` on `{k:"v"}` → `"v"`; unresolvable segment → `false`; NO divergence. (3) the `RouteMetaLookup` body EXECUTED nil-safe on a nil `*corev3.Metadata` → `(nil,false)`; `structpb` ABSENT from chain.go (grep → 0); `NewStructValue` signature `func(*structpb.Struct) *structpb.Value` confirmed. (4) the 18-caller map EXACT (5+6+7, exactly 3 nil sites `330/313/130`); the emit methods + `ResolveCustomTags` calls currently 3-arg (the "append a 4th, leave the 3rd untouched" mechanic is correct); `accesslog_emit.go` imports `structpb` `:9`. (5) the fuzz Route→Cluster repoint EXECUTED (`MetadataKind_Cluster_`/`GetCluster()` compile + behave); `parseCustomTags` `config.go:126` < `provider required` `:136`. (6) `go build ./...` clean; +0 modules (the only new import is `structpb` into `chain.go`, existing). (7) collisions 0. **Informational heads-up (not a defect):** a naive `grep -c 'ResolveCustomTags('` over `resolve_test.go` returns 21 because line 106 is the STRING `"ResolveCustomTags(nil, ...)"` inside a `t.Errorf` — there are 20 REAL callers (RD-TESTCALLERS correct); already covered by the PLAN's "re-grep at tip; a rename surfaces as a build error" instruction.
- **V2 — process/consistency/SPEC-coverage.** **ZERO SEVERE, ZERO MODERATE, ZERO MINOR — STAGE-CLOSE MECHANICS PASS.** All eight checks PASS: (1) docs-only — `git status --short` = only `PLAN.md` + `PROGRESS.md` untracked, `git diff --stat` empty; ADR-0293 `STATUS: PROPOSED` §Context-only, ADR-0294 absent, row 71 `in-progress`. (2) SPEC §10/§11 coverage complete — every §11 edit site written by a task; ZERO cited-but-unwritten obligation (the phase-69 `TestNoNewStat` defect class avoided — SPEC §7 says NO stat guard for tracing; the PLAN states so). (3) stage-close mechanics correct — row 71 flips `done` at the IMPL six-gate; ADR-0293 completes IN PLACE (no renumber; next-free ADR-0294); the deferred-sentence narrow at the IMPL row-done, NOT the PLAN (phase-57 precedent). (4) counts consistent — baselines re-run (fixtures 116 / fuzzers 55 / DECISIONS tail ADR-0293); every `117` explicitly the IMPL post-state, never current. (5) the router rolls PLAN → phase-71 IMPL (Global Constraints), and T9 [the IMPL's final task] rolls to the phase-72 BRAINSTORM — matching the phase-70 template exactly. (6) the S1–S5 sharpenings all consistently reflected (S1 T2 / S2 T3 / S3 Global-Constraints+T1 / S4 T5 / S5 T6). (7) break protocol complete (`-count=1`, full `TestDifferential/0115-...` selector, `Errorf`-per-property, substitution rule flagged, non-firing = FINDING, SERVED-this-arm). (8) format faithful to the phase-70 PLAN. **The three sentinel commands RUN in the worktree:** (1) `NOT DONE: row 71`; (2) `grep -cE 'remaining deferred \(not-yet-chartered\) candidates:'` → **3**; (3) `NEVER OPENED: gRPC/Runtime/WASM` ⇒ does NOT fire.

The design direction — a `ROUTE`-kind metadata custom tag on the landed static-route-config metadata (`FilterChain.RouteMetadata()`), reusing the phase-70 `descend`/`structpbValueToString` verbatim, a NEW one-arg `routeMetaLookup` 4th param + a `RouteMetaLookup` method, single flat row of 9 tasks — is unchanged from the SPEC; the re-derivation found ZERO drift (32/32 EXACT-HOLD) and both verifiers found ZERO findings.

---

## Global Constraints

- **ONE stage per session.** This session: the PLAN only. No production `.go`. After it lands: roll to the phase-71 IMPL.
- **SEVEN functionally-edited production files, ZERO new packages** (SPEC §4, §15): `internal/tracing/config.go` · `internal/tracing/resolve.go` · `internal/filter/http/chain.go` · `internal/filter/hcm/accesslog_emit.go` · `internal/filter/hcm/connection.go` · `internal/filter/hcm/h2dispatch.go` · `internal/filter/hcm/h3dispatch.go`. **New exported symbols: `func (c *FilterChain) RouteMetaLookup()` on `internal/filter/http` (S2) + the `ResolveCustomTags` signature growth (a 4th param) + the `kindMetadataRoute` const (in `internal/tracing`).** `descend`/`structpbValueToString`/`CustomTagSpec` fields are REUSED (already exported/defined at phase 70). The `internal/xds` zero-new-symbol discipline is UNTOUCHED (xds not touched). **BYTE-UNTOUCHED:** `internal/xds`, `internal/tls`, `internal/boot`, `internal/listener`, `internal/bootstrap`, `validate/`, `internal/dynamicmetadata` (ROUTE reads route config, not the Bucket — NOT even consumed anew), `internal/filter/http/ratelimit`, `internal/filter/http/lua` (the `0114` writer — the `0115` fixture DROPS it, not edits it).
- **`ROUTE` MetadataKind ONLY.** `CLUSTER`/`HOST`/unset-kind reject LOUDLY with distinct substrings (envoy-go-strict DEPARTURE — the reference BOOTS them, P1). The unset-kind split (S3): sub-case A (whole `*MetadataKind` nil, `:226-227` "kind required") is a DEPARTURE (reference accepts, no PGV `required` rule); sub-case B (present, empty oneof, `default:` `:251-252` "kind required") is PARITY (PGV oneof "value is required"). Both already reject; a WORDING refinement for the BC/ADR. Empty `metadata_key.key`/empty `path`/empty segment reject (PGV-PARITY — the reference boot-rejects too).
- **The default rule is `request_header`, NOT `environment`** (RD-DEFAULT): a present-but-empty ROUTE metadata value emits `""` (does NOT fall to the default); absent + non-empty default → default; absent + empty/omitted default → omit. `HasDefault = DefaultValue != ""`.
- **Value→string serialization** (RD-SERIALIZE): string→raw (`GetStringValue()`); `NullValue`→unresolvable (boundary); else `protojson.Marshal` + `json.Compact` (`reference_detrand_race_catches_protojson_value_substring` — run `internal/tracing` under `-race`). Reused verbatim from phase 70.
- **The ROUTE resolve descends the FULL `MetaPath`** (RD-ROUTE-ARM) — NOT `[1:]`. The REQUEST `[1:]` is a Bucket-pre-keying artifact; the ROUTE lookup returns the whole namespace struct.
- **The 4th arg APPENDS; the 3rd is untouched** (RD-CALLERS) — at the 15 chain sites the existing `chain.DynamicMetadata().Get` metaLookup arg stays; append `chain.RouteMetaLookup`. At the 3 nil sites append a SECOND `nil`.
- **Counts at the IMPL:** fixtures **116 → 117** (`0115-tracing-custom-tags-metadata-route`) · fuzzers **55 (+0, a seed repurpose)** · stat surface **1201 (+0)** — a span attribute registers no stat; NO `TestNoNewStat` guard (tracing has no per-tag stat surface; 59/62/63/70 added none) · BackendKind **38 (+0)** — `0115` reuses `HTTPFixedBody`, `otlptrace` driver-owned, NO Lua writer · go.mod **+0** (SPEC metric "2" carried; re-check `git diff go.mod` after tidy — RD-MOD) · ZERO new packages · DECISIONS tail stays **ADR-0293** (completed IN PLACE at the IMPL; next-free ADR-0294).
- **The pinned §9 wording lands MECHANICALLY** — B1/B2 are named obligations with the SPEC §9 replacement text; never silent rewrites, never paraphrases. They land at T7, atomically with the row-done edit; ADR-0293 completes at T9's stage-close.
- **Per-task hygiene** (`feedback_pertask_gofmt_lint`): `gofmt -l` + `go vet` + `golangci-lint run` on every touched package.
- **Worktree discipline** (`feedback_git_worktrees` / `feedback_subagent_worktree_detach` / `feedback_subagent_worktree_path_targeting`): pin the canonical root; the controller verifies the MAIN checkout stays clean; deliberate breaks restore with **`git restore` only**; breaks run AFTER committing (`reference_break_protocol_commit_first`).
- **Subagents commit locally; the controller squash-pushes at stage-close** (`feedback_subagents_no_push`, `feedback_push_to_origin`). Subagents auto-commit per CLAUDE.md; the controller squashes at close. Locate commits by SUBJECT (`git log --grep 'phase 71'`), never by position.
- **`reference_sds_init_fetch_timeout_dial_budget_flake` / `reference_0061_ring_hash_spread_flake`** — a `TestProvider_*_Timeout` under `-race` or a `0061` spread failure is PRE-EXISTING on master (one occurrence each). Do not reflex-classify as a phase-71 regression; a SECOND occurrence justifies investigating margins.

### Break protocol (binding on every task)

- **A break must COMPILE** (`reference_plan_break_instructions_dont_compile`). Breaks flagged `[NOT pre-compiled — substitution rule applies]`: at IMPL time, if it does not compile, **substitute a compiling equivalent, REPORT the substitution, record the TRUE result**.
- **A break must DISCRIMINATE** (`reference_probe_must_discriminate`): before recording it as proof, ask what the OTHER hypothesis would have printed.
- **`-count=1` on EVERY differential break** (`reference_differential_break_protocol_count1`); caching serves a stale PASS.
- **Confirm WHICH assertion fired** (`reference_deliberate_break_wrong_assertion`) — and whether a second property's firing is ENTAILED by the first.
- **A break that does NOT fire is a FINDING** — record it honestly in PROGRESS; do not route around it.
- **Full selector only:** `-run 'TestDifferential/0115-tracing-custom-tags-metadata-route'` — never bare `0115` (`reference_differential_run_selector`).
- **`Errorf` per independent property; `Fatalf` only for broken preconditions** (`reference_fatalf_makes_assertions_unreachable`).
- **The ROUTE metadata must be SERVED-this-arm** — the span must carry the STATIC route value, not a vacuous default; a wrong-namespace control that falls to the default proves the route-metadata/namespace binding is load-bearing (the `0115` fixture has NO runtime writer — the source is the served static route config, cross-side-identical).

### Identifier roster (`reference_spec_drafted_identifier_collision_check`)

**Verified FREE repo-wide at `60f12146` (`grep -rn --include='*.go'`, `.worktrees` excluded — RD-IDENT, all 0 hits):** `kindMetadataRoute` · `routeMetaLookup` · `RouteMetaLookup` (and `RouteMetaLookup(ns`). **REUSED (already landed at phase 70, free to reference):** `descend` · `structpbValueToString` · `CustomTagSpec.MetaNamespace`/`MetaPath`/`DefaultValue`/`HasDefault` · `kindMetadata`. **`metadatav3`** is the ESTABLISHED alias (ratelimit + `fuzz_test.go`). **Fixture:** `test/fixtures/0115-*` does not exist; no in-container port to claim. **Any FURTHER name the IMPL coins** (the fixture `package driver` helpers, the route metadata namespace/key strings, any test name): grep first, record the check.

---

## File structure

```
internal/tracing/config.go                        [EDIT]  T1 (kindMetadataRoute const; the ROUTE accept arm replacing the :245-246 reject; NO new import)
internal/tracing/config_test.go                   [EDIT]  T1 (accept ROUTE; reject CLUSTER/HOST/unset-A/unset-B/empty-namespace/empty-path/empty-segment — distinct substrings; first-wins dedup with a ROUTE tag)
internal/tracing/resolve.go                       [EDIT]  T2 (routeMetaLookup 4th param; the kindMetadataRoute arm descending the FULL MetaPath; descend/structpbValueToString reused verbatim; NO new import)
internal/tracing/resolve_test.go                  [EDIT]  T2 (ROUTE path-walk single/multi/unresolvable; the serialization table json.Compact-compared; the default/omit/present-empty matrix; nil-routeMetaLookup tolerance; 20 existing calls +nil 4th arg)
internal/filter/http/chain.go                     [EDIT]  T3 (add func (c *FilterChain) RouteMetaLookup() beside DynamicMetadata() :1022; ADD the structpb import)
internal/filter/http/chain_test.go                [EDIT]  T3 (present ns → wrapped struct; absent ns → (nil,false); nil RouteMetadata → (nil,false))
internal/filter/hcm/accesslog_emit.go             [EDIT]  T4 (routeMetaLookup param on the 3 emit methods → 4th ResolveCustomTags arg; structpb already imported)
internal/filter/hcm/connection.go                 [EDIT]  T4 (5 callers — :330 append nil; :464/:597/:699/:777 append chain.RouteMetaLookup)
internal/filter/hcm/h2dispatch.go                 [EDIT]  T4 (6 callers — :313 append nil; :396/:530/:577/:584/:613 append chain.RouteMetaLookup)
internal/filter/hcm/h3dispatch.go                 [EDIT]  T4 (7 callers — :130 append nil; :210/:280/:341/:367/:373/:395 append chain.RouteMetaLookup)
internal/filter/hcm/accesslog_emit_test.go        [EDIT]  T4 (caller signatures += trailing nil routeMetaLookup)
internal/filter/hcm/span_emit_test.go             [EDIT]  T4 (caller signatures += trailing nil routeMetaLookup — grep at IMPL)
internal/filter/hcm/fuzz_test.go                  [EDIT]  T5 (withMetaTags: add meta_route_ok, repoint meta_bad to CLUSTER; dispatch-verified; +0 func Fuzz)
test/fixtures/0115-tracing-custom-tags-metadata-route/  [ADD]  T6 (driver/, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md — NO scripts/ Lua), T6 (breaks)
docs/envoy-go/BEHAVIOR_CONTRACT.md                [EDIT]  T7 (B1 flip the ROUTE line to CONSUMED; B2 narrow the departures to CLUSTER/HOST + unset-A)
docs/envoy-go/DECISIONS.md                        [EDIT]  T9 (ADR-0293 completed IN PLACE — §Decision + §Consequences)
internal/xds/** · internal/tls/** · internal/boot/** · internal/listener/** · internal/bootstrap/** · validate/** · internal/dynamicmetadata/** · internal/filter/http/ratelimit/** · internal/filter/http/lua/**  [BYTE-UNTOUCHED]
```

---

## Task 1 — `internal/tracing/config.go`: the `kindMetadataRoute` const + the ROUTE accept arm

**Files:**
- Modify: `internal/tracing/config.go` (the `customTagKind` block `:53-58`; the `parseCustomTags` ROUTE `case k.GetRoute() != nil:` `:245-246`)
- Test: `internal/tracing/config_test.go`

**Interfaces:**
- Produces: `kindMetadataRoute` const (iota==4); the ROUTE accept path building `CustomTagSpec{Kind:kindMetadataRoute, MetaNamespace, MetaPath (FULL), DefaultValue, HasDefault}`. Consumed by `resolve.go` (T2).
- Consumes: `md.GetKind()` (→ `*metadatav3.MetadataKind`, `k`) then `k.GetRoute()` **on `k`, NOT on `md`** (V1 SEVERE — §1.1); `md.GetMetadataKey()`, `mk.GetKey()`, `mk.GetPath()`, `seg.GetKey()`, `md.GetDefaultValue()`. **NO new import.**

**Entry state:** clean `60f12146`-derived branch; `go test ./internal/tracing/ -count=1` green.

- [ ] **Step 1 — write the failing unit tests (red-first).** In `config_test.go`, model on the phase-70 metadata parse tests (`grep -n 'route kind unsupported\|cluster kind unsupported\|empty namespace\|kindMetadata' internal/tracing/config_test.go` for the reject-substring + accept-shape precedents). Add:
  1. `Test…MetadataRoute_Accept` — a `CustomTag{Tag:"t", Type:CustomTag_Metadata_{Metadata:{Kind:Route, MetadataKey:{Key:"ns", Path:[{Key:"a"},{Key:"b"}]}, DefaultValue:"d"}}}` parses to `CustomTagSpec{Key:"t", Kind:kindMetadataRoute, MetaNamespace:"ns", MetaPath:["a","b"], DefaultValue:"d", HasDefault:true}`.
  2. `Test…MetadataRoute_RejectKinds` — subtests CLUSTER / HOST / unset-kind (nil `MetadataKind`) each boot-FAIL with the distinct substring (`cluster kind unsupported` / `host kind unsupported` / `kind required`). (ROUTE is now ACCEPTED — assert it NO LONGER rejects.)
  3. `Test…MetadataRoute_RejectStructural` — subtests empty namespace (`empty namespace`) / empty path (`empty path`) / empty path segment (`empty path segment`) for a ROUTE-kind tag.
  4. `Test…MetadataRoute_HasDefaultFalseWhenEmpty` — a ROUTE tag with `DefaultValue:""` → `HasDefault==false`.
  5. `Test…MetadataRoute_FirstWinsDedup` — two same-key tags (a ROUTE metadata + a later literal) → the FIRST wins (the ROUTE spec survives; the literal drops).

  Run `go test ./internal/tracing/ -run 'MetadataRoute' -count=1`. **Expected: FAIL** — `kindMetadataRoute` undefined (compile error). Record the verbatim red.

- [ ] **Step 2 — add the const.** Add `kindMetadataRoute` to the `customTagKind` iota block (`:53-58`, appends at 4 after `kindMetadata`). No struct change (`MetaNamespace`/`MetaPath`/`DefaultValue`/`HasDefault` all present — RD-KIND). No new import (RD-NOIMPORT-CFG).

- [ ] **Step 3 — replace the `:245-246` ROUTE reject** with the accept arm (§1.1 skeleton). **The kind-getter `k.GetRoute()` is on `*metadatav3.MetadataKind` — `k` is already bound at `:224`; do NOT call `md.GetRoute()` (does not compile, V1 SEVERE).** Keep the unset-A `:226-227`, REQUEST accept `:228-244`, CLUSTER `:247-248`, HOST `:249-250`, `default:` `:251-252`, empty-tag reject `:198-200`, and first-wins dedup `:257-261` UNCHANGED.

- [ ] **Step 4 — run the tests.** `go test ./internal/tracing/ -count=1`. **Expected: PASS** (the five new tests green; every pre-existing tracing test green — the literal/request_header/environment/metadata-REQUEST arms untouched; the CLUSTER/HOST/unset rejects still fire).

- [ ] **Step 5 — breaks (AFTER committing).**
  - **Break A [accept→reject]:** make the `k.GetRoute()!=nil` arm `return nil, fmt.Errorf(...)` instead of building the spec → test 1 FIRES (accept expected, got error). `git restore`; re-green.
  - **Break B [kind confusion]:** swap the CLUSTER and ROUTE arms (accept CLUSTER, reject ROUTE) → test 2's CLUSTER subtest passes-when-it-should-fail AND test 1 fails → confirm WHICH. `git restore`; re-green. `[NOT pre-compiled — substitution rule applies]`.
  - **Break C [HasDefault]:** set `HasDefault: true` unconditionally → test 4 FIRES (`HasDefault==false` expected). `git restore`; re-green.

- [ ] **Step 6 — hygiene + commit.** `gofmt -l internal/tracing` silent · `go vet ./internal/tracing/` · `golangci-lint run ./internal/tracing/`.

**Commit:** `tracing(phase 71 T1): custom_tags metadata ROUTE parse — kindMetadataRoute const + the ROUTE accept arm (cloning the REQUEST accept, MetaPath FULL, HasDefault = DefaultValue != "") replacing the :245-246 route-kind reject; CLUSTER/HOST/unset-kind rejects UNCHANGED (envoy-go-strict DEPARTURE); empty namespace/path/segment PGV-PARITY; NO new import`

---

## Task 2 — `internal/tracing/resolve.go`: the `routeMetaLookup` 4th param + the `kindMetadataRoute` arm

**Files:**
- Modify: `internal/tracing/resolve.go` (the `ResolveCustomTags` signature `:26`; a new `kindMetadataRoute` `case` beside `kindMetadata` `:65-88`)
- Test: `internal/tracing/resolve_test.go`

**Interfaces:**
- Produces: `ResolveCustomTags(specs, headerLookup, metaLookup, routeMetaLookup)` — the new nil-tolerant 4th param `routeMetaLookup func(ns string) (*structpb.Value, bool)` (ONE-arg, DISTINCT from the two-arg `metaLookup`); the `kindMetadataRoute` resolve arm.
- Consumes: `descend` (`:100`) + `structpbValueToString` (`:123`) REUSED VERBATIM; `structpb.Value`, `KV`.
- Reuses UNTOUCHED: the `kindLiteral`/`kindRequestHeader`/`kindEnvironment`/`kindMetadata` arms. **NO new import.**

**Entry state:** T1 landed; `go test ./internal/tracing/ -count=1` green.

**Design (RE-DERIVED; §1.1 skeletons):** the `kindMetadataRoute` arm mirrors the `kindMetadata` REQUEST arm (`:65-88`) for the default rule (present-empty EMITS `""`, RD-DEFAULT), but calls `routeMetaLookup(s.MetaNamespace)` (one-arg) and descends `descend(v, s.MetaPath)` — the **FULL** path (RD-ROUTE-ARM), NOT `s.MetaPath[1:]`.

- [ ] **Step 1 — write the failing tests (red-first).** In `resolve_test.go`, the 20 existing `ResolveCustomTags(...)` calls (RD-TESTCALLERS: lines 34,72,97,105,124,131,138,146,188,198,207,216,244,254,268,276,284,292,304,311 — re-grep at tip) each gain a trailing `nil` (4th arg — no ROUTE tags). Add ROUTE metadata tests driving a fake `routeMetaLookup func(ns)(*structpb.Value,bool)`:
  1. **path-walk single** — `MetaPath:["k"]`, `routeMetaLookup("ns")→(structVal{k:"v"},true)` → `KV{Key,"v"}` (descend the FULL path from the namespace struct).
  2. **path-walk multi** — `MetaPath:["a","b"]`, `routeMetaLookup("ns")→(structVal{a:{b:"deep"}},true)` → `KV{Key,"deep"}`; and an unresolvable segment → falls to default/omit.
  3. **the serialization table** — string→raw (`"x"`, no quotes); number→`42`/`3.14`; bool→`true`/`false`; struct→`{"a":"b"}`; list→`["x","y","z"]` (each `json.Compact`-compared — assert the EXACT compact string, `reference_detrand_race_catches_protojson_value_substring`); `NullValue`→unresolvable (default/omit). *(Reuses the phase-70 serializer; a thin ROUTE-arm re-assert, not a re-derivation of the table.)*
  4. **the default matrix** — present-non-empty→emit; present-EMPTY (`structVal{k:""}`)→emit `""` (NOT default); absent namespace→default (if `HasDefault`) else omit.
  5. **nil-`routeMetaLookup` tolerance** — a `kindMetadataRoute` spec with `routeMetaLookup==nil` → falls to default/omit, no panic.

  **⚠️ Distinguish from REQUEST:** the ROUTE fake returns the WHOLE namespace struct (`structVal{...}`) and the arm descends the FULL `MetaPath`; the REQUEST fake pre-keys `MetaPath[0]`. A single-segment ROUTE path `["k"]` descends one level from the namespace struct.

  Run `go test ./internal/tracing/ -run 'Resolve' -count=1`. **Expected: FAIL** — `ResolveCustomTags` arity mismatch (20 callers now pass 4 args against a 3-param signature, or the new tests reference the 4th param). Record the verbatim red.

- [ ] **Step 2 — write the arm + grow the signature.** Add the `routeMetaLookup` 4th param (`:26`) and the `kindMetadataRoute` `case` (§1.1). `descend`/`structpbValueToString` reused verbatim — **NO new import** (`structpb`/`protojson`/`encoding/json`/`bytes` all already imported from phase 70). Confirm `MetaPath` is safe to pass whole (validated non-empty at parse — T1; `descend` tolerates any length incl. an empty tail).

- [ ] **Step 3 — run the tests.** `go test ./internal/tracing/ -count=1`. **Expected: PASS**.

- [ ] **Step 4 — breaks (AFTER committing).**
  - **Break D [wrong slice]:** descend `s.MetaPath[1:]` (the REQUEST slice) instead of the full `s.MetaPath` → test 1/2's single/multi-segment assertion FIRES (a single-segment ROUTE path would resolve to the namespace struct itself, not the field). `git restore`; re-green. (Pins RD-ROUTE-ARM — the FULL-path descent.)
  - **Break E [default rule — clone the wrong arm]:** make the present-empty case OMIT (the `kindEnvironment` rule) instead of emitting `""` → test 4's present-empty assertion FIRES. `git restore`; re-green. (Pins RD-DEFAULT — the `request_header` rule.)
  - **Break F [nil-tolerance dropped]:** call `routeMetaLookup(...)` without the `!= nil` guard → test 5 panics; substitute a compiling equivalent that mis-handles nil → the nil-tolerance assertion FIRES. `git restore`; re-green. `[NOT pre-compiled — substitution rule applies]`.

- [ ] **Step 5 — `-race`.** `go test ./internal/tracing/ -race -count=1` (the protojson value-substring path; `reference_detrand_race_catches_protojson_value_substring`).

- [ ] **Step 6 — hygiene + commit.** `gofmt -l internal/tracing` silent · `go vet` · `golangci-lint run ./internal/tracing/`.

**Commit:** `tracing(phase 71 T2): custom_tags metadata ROUTE resolve — ResolveCustomTags grows a nil-tolerant one-arg routeMetaLookup 4th param + a kindMetadataRoute arm descending the FULL MetaPath (NOT [1:] — the REQUEST slice is a Bucket-pre-keying artifact); descend/structpbValueToString reused VERBATIM (the request_header default rule: present-empty EMITS ""); NO new import`

---

## Task 3 — `internal/filter/http/chain.go`: the `*FilterChain.RouteMetaLookup()` accessor (S2)

**Files:**
- Modify: `internal/filter/http/chain.go` (add the method beside `DynamicMetadata()` `:1022`; ADD the `structpb` import)
- Test: `internal/filter/http/chain_test.go`

**Interfaces:**
- Produces: `func (c *FilterChain) RouteMetaLookup(ns string) (*structpb.Value, bool)`. Consumed by `accesslog_emit.go` callers (T4).
- Consumes: the existing `c.RouteMetadata()` (`:1156`) → `GetFilterMetadata()[ns]` → `structpb.NewStructValue`.

**Entry state:** T1–T2 landed; `go test ./internal/filter/http/ -count=1` green.

- [ ] **Step 1 — write the failing test (red-first).** In `chain_test.go`, add `TestFilterChain_RouteMetaLookup`: (a) a fresh `FilterChain` (via the constructor grep'd in `chain_test.go`), `SetRouteMetadata(&corev3.Metadata{FilterMetadata: map[string]*structpb.Struct{"ns": {Fields: {"k": stringValue("v")}}}})`, assert `RouteMetaLookup("ns")` returns `(wrapped-struct, true)` and the wrapped value descends to `"v"`; (b) absent namespace → `RouteMetaLookup("missing")` returns `(nil,false)`; (c) nil `RouteMetadata` (no `SetRouteMetadata` call) → `RouteMetaLookup("ns")` returns `(nil,false)` without panic (RD-SEAM nil-safety). Run `go test ./internal/filter/http/ -run 'TestFilterChain_RouteMetaLookup' -count=1`. **Expected: FAIL** — `RouteMetaLookup` undefined. Record the verbatim red.

- [ ] **Step 2 — add the accessor + the import** (§1.1) beside `DynamicMetadata()` (`:1022`). ADD `structpb "google.golang.org/protobuf/types/known/structpb"` to the import block (re-derive exact placement/alias at tip — an EXISTING module, RD-MOD).

- [ ] **Step 3 — run the tests.** `go test ./internal/filter/http/ -count=1`. **Expected: PASS**.

- [ ] **Step 4 — break (AFTER committing).** **Break G [nil-guard dropped]:** remove the `if st == nil { return nil, false }` guard and `return structpb.NewStructValue(st), true` unconditionally → the nil-`RouteMetadata` case (c) panics on `NewStructValue(nil)` or returns a non-nil wrapper for an absent namespace → the `(nil,false)` assertion FIRES. `git restore`; re-green. `[NOT pre-compiled — substitution rule applies]`.

- [ ] **Step 5 — hygiene + commit.** `gofmt -l internal/filter/http` silent · `go vet ./internal/filter/http/` · `golangci-lint run ./internal/filter/http/`.

**Commit:** `filter/http(phase 71 T3): add func (c *FilterChain) RouteMetaLookup(ns) (*structpb.Value, bool) — the ROUTE analogue of DynamicMetadata(); wraps RouteMetadata().GetFilterMetadata()[ns] via structpb.NewStructValue, nil-safe, (nil,false) on absent; centralizes the structpb import to chain.go so the three dispatch files stay structpb-free`

---

## Task 4 — `internal/filter/hcm`: thread `routeMetaLookup` at the 3 emit methods + all 18 callers

**Files:**
- Modify: `internal/filter/hcm/accesslog_emit.go` (the 3 methods `:27/:87/:149` → 4th `ResolveCustomTags` arg `:57/:118/:179`; `structpb` already imported) · `connection.go` (5 callers) · `h2dispatch.go` (6 callers) · `h3dispatch.go` (7 callers)
- Test: `accesslog_emit_test.go` + `span_emit_test.go` (caller signatures += trailing `nil`)

**Interfaces:**
- Produces: the 3 emit methods each grow a `routeMetaLookup func(ns string) (*structpb.Value, bool)` param, passed straight to `ResolveCustomTags` as the 4th arg.
- Consumes: `chain.RouteMetaLookup` (T3) at the 15 chain-in-scope callers; `nil` at the 3 no-chain-404 sites.

**Entry state:** T1–T3 landed; `go test ./internal/filter/hcm/ -count=1` green.

**RE-DERIVED caller map (RD-CALLERS — chain var name `chain` uniform; the 3 nil sites are BEFORE the chain is built; the 4th arg APPENDS, the 3rd metaLookup arg is UNTOUCHED):**

| File | nil site (no-chain 404) — append a 2nd `nil` | `chain.RouteMetaLookup` sites (append the 4th arg) |
|---|---|---|
| `connection.go` | :330 | :464, :597, :699, :777 |
| `h2dispatch.go` | :313 | :396, :530, :577, :584, :613 |
| `h3dispatch.go` | :130 | :210, :280, :341, :367, :373, :395 |

*(H3:210 has `chain` in scope but `traceDecision` still nil — pass `chain.RouteMetaLookup`; valid-but-never-reached, NOT a nil site. Re-derive each caller's exact `chain` identifier at the IMPL tip; the map is line-anchored but a variable rename would surface as a build error. The `SetRouteMetadata` seeds `connection.go:433`/`h2dispatch.go:375`/`h3dispatch.go:195` are READ-ONLY substrate — NOT edited.)*

- [ ] **Step 1 — write/adjust the failing tests (red-first).** The existing emit-method callers in `accesslog_emit_test.go` + `span_emit_test.go` (grep for the exact call lines at tip) each gain a trailing `nil` for `routeMetaLookup` (no ROUTE tags → byte-stable behavior). Add ONE new test proving the thread is live: a `Filter` configured with a `kindMetadataRoute` custom_tag + a fake chain whose `RouteMetadata()` resolves the value, driven through `emitAccessLog` with a sampling `traceDecision`, asserts the span carries the resolved `{tag,value}` attribute (clone the phase-70 metadata-span test in `span_emit_test.go`, swapping `DynamicMetadata`/REQUEST for `RouteMetadata`/ROUTE). Run `go test ./internal/filter/hcm/ -count=1`. **Expected: FAIL** — arity mismatch (the methods don't take `routeMetaLookup` yet). Record the verbatim red.

- [ ] **Step 2 — thread the param.** Add `routeMetaLookup func(ns string) (*structpb.Value, bool)` to `emitAccessLog`/`H2`/`H3` (`:27/:87/:149`), pass it as the 4th `ResolveCustomTags` arg (`:57/:118/:179`). NO new import (`structpb` already at `accesslog_emit.go:9`).

- [ ] **Step 3 — update the 18 callers** per the RD-CALLERS map: the 3 nil sites append a 2nd `nil`; the 15 chain-in-scope sites append `chain.RouteMetaLookup` (a method value — nil-safe via RD-SEAM). **The existing 3rd metaLookup arg is UNTOUCHED at every site.** Re-derive the exact `chain` identifier per caller at the tip.

- [ ] **Step 4 — run the tests.** `go test ./internal/filter/hcm/ -count=1`. **Expected: PASS** (the new ROUTE-metadata-span test green; all pre-existing emit tests green with the trailing `nil`).

- [ ] **Step 5 — breaks (AFTER committing).**
  - **Break H [nil at a span site]:** pass `nil` for `routeMetaLookup` at a chain-in-scope caller (e.g. `connection.go:777`) → the new ROUTE-metadata-span test's resolved-value assertion FIRES (the tag falls to default/omit). `git restore`; re-green. `[NOT pre-compiled — substitution rule applies]`.
  - **Break I [wrong lookup]:** substitute a `routeMetaLookup` that always returns `(nil,false)` → the resolved-value assertion FIRES. Record. `git restore`; re-green.

- [ ] **Step 6 — hygiene + `-race` + commit.** `gofmt -l internal/filter/hcm` silent · `go vet` · `golangci-lint run ./internal/filter/hcm/` · `go test ./internal/filter/hcm/ -race -count=1` (the dispatch goroutines).

**Commit:** `hcm(phase 71 T4): thread routeMetaLookup at the 3 emit methods + all 18 callers — chain.RouteMetaLookup APPENDED as the 4th arg at the 15 span-capable sites (the 3rd metaLookup arg untouched), a 2nd nil at the 3 no-chain-404 sites (provably never reached — the traceDecision gate); the ROUTE-kind metadata custom_tag now resolves onto the ingress span`

---

## Task 5 — fuzz: repurpose the `withMetaTags` seed in `FuzzHCMConfigParse` (+0 fuzzers)

**Files:**
- Modify: `internal/filter/hcm/fuzz_test.go` (the `withMetaTags` seed `:94-108`; `metadatav3` already imported)

**Entry state:** T1–T4 landed. Fuzzer `FuzzHCMConfigParse`; seed `withMetaTags` (RD-FUZZ).

- [ ] **Step 1 — repurpose the seed.** In `withMetaTags` (§1.1, S4): (a) ADD a valid ROUTE-accept tag `meta_route_ok` (`MetadataKind_Route_`, MetadataKey `{Key:"envoy.test", Path:[{Key:"k"}]}`, a default) to exercise the new accept arm; (b) REPOINT the existing `meta_bad` tag's kind from `Route` to `Cluster` (keeping it MetadataKey-less) so a kind-unsupported reject arm stays exercised. `meta_ok` REQUEST stays. Re-derive the exact `metadatav3.MetadataKind_Route_`/`_Cluster_`/`MetadataKey_PathSegment_Key` wrapper spellings at the tip (already imported).

- [ ] **Step 2 — dispatch-verify (the named trap — RD-FUZZ / SPEC §7).** Confirm the seed REACHES the `parseCustomTags` metadata arm (`config.go:126` precedes the provider check `:136`). As a one-off scratch check (NOT committed): temporarily `t.Logf`/print which arm the seed reaches, run `go test ./internal/filter/hcm/ -run FuzzHCMConfigParse -count=1 -v`, confirm the ROUTE accept for `meta_route_ok` + the cluster-kind reject for `meta_bad` — NOT an earlier typeURL/provider reject. Restore. Record in PROGRESS.

- [ ] **Step 3 — reconcile the count.** `grep -rn '^func Fuzz' --include='*.go' internal/ test/ | wc -l` → **55** BEFORE and AFTER (`reference_fuzzer_count_docs_drift` — a seed is +0 `func Fuzz`). A short active-fuzz smoke (`go test -run FuzzHCMConfigParse -fuzz FuzzHCMConfigParse -fuzztime 10s ./internal/filter/hcm/`) — no panic; NO corpus artifacts committed.

- [ ] **Step 4 — hygiene + commit.** `gofmt -l internal/filter/hcm` silent · `go vet` · `golangci-lint run ./internal/filter/hcm/`.

**Commit:** `hcm(phase 71 T5): fuzz — repurpose the withMetaTags seed (add a valid ROUTE-accept meta_route_ok + repoint meta_bad Route→Cluster) in FuzzHCMConfigParse, dispatch-verified to reach the parseCustomTags metadata arm (config.go:126 precedes the provider check :136); +0 fuzzers, 55→55`

---

## Task 6 — fixture `0115-tracing-custom-tags-metadata-route` (OTLP) + breaks (fixtures 116 → 117)

**Files:**
- Create: `test/fixtures/0115-tracing-custom-tags-metadata-route/{driver/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}` — **NO `scripts/` Lua**

**Entry state:** T1–T5 landed. Next fixture `0115`; no port to pick (RD-FIXTURE).

**Design (SPEC §8 — ONE fixture, TWO ROUTE custom_tags in one config, NO writer; RD-FIXTURE — the CHASSIS clones from `0114`, DROPPING the Lua):**
- Both YAMLs: the `0114-tracing-custom-tags-metadata` chassis (H1 listener → HTTPFixedBody backend, HCM OTLP tracing provider → the driver-owned `test/helpers/otlptrace.Server` receiver, `random_sampling:100`, all ports templated). **REMOVE the `envoy.filters.http.lua` filter + the `scripts/writer.lua` + the two REQUEST tags.** ADD to the matched route (`rc_test`/`vh_test`, BOTH YAMLs byte-identical) a `metadata: {filter_metadata: {<ns>: {<key>: "<fixed>"}}}` block (a FIXED cross-side-identical static value).
- TWO `metadata` custom_tags in the HCM tracing block:
  - **`route_hit`** — `metadata:{kind:{route:{}}, metadata_key:{key:"<ns>", path:[{key:"<key>"}]}, default_value:"<unused>"}` → resolves to the static route value (P2/P4).
  - **`route_default`** — `metadata:{kind:{route:{}}, metadata_key:{key:"<ns>", path:[{key:"<absent-key>"}]}, default_value:"<fallback>"}` → the path is absent → emits the `default_value` (P5).
- **Cross-side EXACT value equality** (as STRONG as 0114's, WITHOUT a writer): both sides read the SAME static route metadata → the SAME resolved string. The driver asserts on EVERY span BOTH sides, by KEY (P2/S5 — intra-tag order is internal): `route_hit == "<fixed>"` AND `route_default == "<fallback>"` (key AND value), plus the `0087` baseline (span count > 0). Each an independent `Errorf`. `telemetry.sdk.*` resource + scope UNasserted cross-side (impl-specific, the 0114 precedent). Fixtures **116 → 117**.

- [ ] **Step 1–3 — write driver/YAMLs/expectations** (clone `0114` chassis; DROP the Lua filter/script; add the static route `metadata.filter_metadata` block + the two ROUTE custom_tags; the cross-side EXACT key+value assertions; the span-count baseline). Grep the `test/helpers/otlptrace` API for the span-attr accessor to read `route_hit`/`route_default` per span (reuse the `0114` driver's accessor).
- [ ] **Step 4 — run.** `go test ./test/differential/ -run 'TestDifferential/0115-tracing-custom-tags-metadata-route' -count=1`. **Expected: PASS**; fixture count 117. Confirm the ROUTE metadata is SERVED (the span carries the static route value, not a vacuous default — the `route_hit` value-equality assertion is the proof).
- [ ] **Step 5 — hygiene + commit.**

- [ ] **Step 6 — breaks (AFTER committing; `-count=1`, full selector, confirm WHICH fired).**
  - **Break J [route value swap]:** change the static route metadata value on ONE side → the `route_hit` value-equality assertion FIRES (the two sides now differ). `git restore`; re-green.
  - **Break K [default drop]:** empty `route_default`'s `default_value` on both sides → the tag OMITS → the `route_default` presence assertion FIRES. `git restore`; re-green.
  - **Break L [wrong-namespace control]:** point `route_hit`'s `metadata_key.key` at a namespace absent from the route metadata (both sides) → `route_hit` falls to its (unused) default → the `route_hit == "<fixed>"` assertion FIRES (proving the route-metadata/namespace binding is load-bearing, not a vacuous default match). `git restore`; re-green. `[NOT pre-compiled — substitution rule applies]`.

**Commit:** `differential(phase 71 T6): fixture 0115-tracing-custom-tags-metadata-route — the 0114 OTLP chassis WITHOUT the Lua writer + a static route metadata.filter_metadata block + two ROUTE metadata custom_tags (route_hit→the static value, route_default→default_value) asserted cross-side EXACT key+value (no writer — both sides read identical static route config); breaks J/K/L (fixtures 116→117, all ports templated)`

---

## Task 7 — BEHAVIOR_CONTRACT delta (B1–B2) — pinned VERBATIM

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`

**Entry state:** T1–T6 landed. Docs-only. Anchor by SYMBOL / first-clause (re-locate the tracing `custom_tags` `metadata` subsection at the IMPL tip — where the `REQUEST` MetadataKind is described CONSUMED and `ROUTE`/`CLUSTER`/`HOST` are "reject loudly").

- [ ] **B1 — flip the `ROUTE` line** (SPEC §9 B1): from "`ROUTE`/`CLUSTER`/`HOST` — reject loudly (departure)" to: the `metadata` type's **`ROUTE`** MetadataKind is CONSUMED — the matched route's static config metadata (`route.metadata.filter_metadata[ns]` namespace + `path` walk, over the landed `RouteMetadata()` accessor) is resolved, serialized to a STRING (string→raw; number/bool→scalar; struct/list→compact JSON — the `google.protobuf.Value` rendering, shared with REQUEST), emitted as a `{tag, value}` span attribute on BOTH exporters; `default_value` emits when the path is absent/unresolvable and the default is non-empty, a present-empty value emits `""`, an unresolvable-with-empty-default omits (the `request_header` rule). The `CLUSTER`/`HOST`/unset-kind-A arms reject loudly (envoy-go-strict DEPARTURE — the reference boots them); the empty-namespace/empty-path/empty-segment/unset-kind-B rejects are PGV-PARITY. Two of the four MetadataKinds (`REQUEST`@70, `ROUTE`@71) are now consumed.

- [ ] **B2 — the departures list** (SPEC §9 B2): narrow the "non-`REQUEST` MetadataKinds" departure to "`CLUSTER`/`HOST` MetadataKinds + a fully-unset kind (sub-case A) reject where the reference boots"; keep the (documented boundary) non-string value serialization beyond the P3-pinned common types (number-precision edges + `NullValue`).

- [ ] **B3 — NO edit** at the 4-empty-built-in-span-attrs neighborhood (`reference_tracing_upstream_cluster_framework_gap` — a ROUTE-kind metadata tag is populated from the SEPARATE landed `RouteMetadata()`; do NOT conflate `CLUSTER` [the framework gap] with `ROUTE` — verify byte-unchanged).

- [ ] **Verify UNCHANGED:** the `literal`/`request_header`/`environment`/`metadata`-REQUEST lines + `max_path_tag_length` + every other tracing bullet apart from the B1 ROUTE line + the B2 departures narrowing.

**Commit:** `docs(phase 71 T7): BEHAVIOR_CONTRACT B1–B2 — flip the custom_tags metadata ROUTE line to CONSUMED (RouteMetadata() path-walk → serialized string → {tag,value} on both exporters; the request_header default rule); narrow the departures (CLUSTER/HOST + unset-A reject where the reference boots; non-string serialization boundary). Two of four MetadataKinds now consumed`

---

## Task 8 — VERIFY: the six-gate + cycle guard + full differential + `-race` + counts + envelope audit

Controller-run on the frozen pre-stage-close HEAD:

- [ ] 1. `gofmt -l internal/ test/ cmd/` — SILENT
- [ ] 2. `go vet ./...` — exit 0
- [ ] 3. `go build ./...` — exit 0
- [ ] 4. `go mod tidy -diff` EMPTY + `git diff --exit-code master -- go.mod go.sum` EMPTY (**+0 modules** — RD-MOD; the only new import is `structpb` into `chain.go`, an existing module)
- [ ] 5. `golangci-lint run ./...` — exit 0
- [ ] 6. **FULL differential:** `go test ./test/differential/ -count=1` — all **117** dirs, exit 0. The 116 pre-existing dirs byte-stable. `reference_differential_fullsuite_startup_flake`: a `subject ready: EOF` on an UNRELATED fixture is a startup race — isolate-re-run; `reference_0061_ring_hash_spread_flake` on a second occurrence → investigate margins.

**Plus:**
- [ ] **Cycle guard:** `go list -deps ./internal/tracing | grep 'envoy-go/internal'` (**no `...`**) ⇒ NO `internal/filter` edge (the `descend` clone keeps `internal/tracing` filter-free; the `RouteMetaLookup` wrapping lives on the `internal/filter/http` side — RD-SEAM; `reference_xds_config_seam_transitive_cycle_guard`, TYPE-level).
- [ ] **`-race` on touched packages:** `go test ./internal/tracing/ ./internal/filter/http/ ./internal/filter/hcm/ -race -count=1` (the protojson value path + the dispatch goroutines; `reference_detrand_race_catches_protojson_value_substring`, `reference_full_suite_race_after_background_mutator`).
- [ ] **Counts MECHANICAL, never copied:** fixtures **117** (tail `0115-tracing-custom-tags-metadata-route`) · fuzzers **55** (`^func Fuzz`) · BackendKind **38** · DECISIONS tail **ADR-0293** · stat surface **1201** (+0 — a span attribute registers no stat; there is NO mechanical stat command) · go.mod diff EMPTY.
- [ ] **Envelope audit:** `git diff master --stat` shows functional production = `internal/tracing/{config.go,resolve.go}` + `internal/filter/http/chain.go` + `internal/filter/hcm/{accesslog_emit.go,connection.go,h2dispatch.go,h3dispatch.go}` ONLY; **`internal/xds`/`internal/tls`/`internal/boot`/`internal/listener`/`internal/bootstrap`/`validate/` ABSENT**; `internal/dynamicmetadata`/`internal/filter/http/ratelimit`/`internal/filter/http/lua` BYTE-UNTOUCHED. **New exported symbols ONLY** in `internal/tracing` (the `ResolveCustomTags` growth, `kindMetadataRoute`) + `internal/filter/http` (`RouteMetaLookup`); ZERO new packages/modules/stats/BackendKinds; the `internal/xds` zero-new-symbol discipline UNTOUCHED.

*(No separate commit — T8's evidence lands in PROGRESS at T9.)*

---

## Task 9 — ADR-0293 completed IN PLACE + stage-close (controller-adjacent)

- [ ] **ADR-0293: COMPLETE IN PLACE** — append §Decision + §Consequences to the EXISTING entry (the §Context landed at the SPEC squash, STATUS: PROPOSED). Flip the STATUS banner to **COMPLETE**. **Do NOT append a new ADR; do NOT renumber.** Tail stays ADR-0293; next-free ADR-0294 (`grep -c '^## ADR-0294'` → 0). §Decision records the landed mechanism (the parse ROUTE accept arm + `kindMetadataRoute`; the `routeMetaLookup`/`kindMetadataRoute` resolve arm descending the FULL `MetaPath`; the `*FilterChain.RouteMetaLookup()` accessor + the 18-caller thread; the `0115` fixture); §Consequences records the counts, the named departures (CLUSTER/HOST + unset-A reject where the reference boots; the non-string serialization boundary), and the memory updates.
- [ ] **ROADMAP row 71 → `done`** at the six-gate (ADR-0106, SOLE leg; `reference_roadmap_split_phase_row_done`). **NARROW the deferred sentence NOW (and ONLY now):** roll the `ROUTE` MetadataKind OUT of the live Observability `candidates:` sentence (the phase-57 graphite precedent — SPEC §12; the sentence STAYS a live `candidates:` match afterward — the `CLUSTER`/`HOST` MetadataKinds + `spawn_upstream_span`/`http_service`/force-trace + the `ssl` family remain). Keep EXACTLY ONE live Observability `candidates:` match; HTTP/3 + xDS untouched (three total).
- [ ] **STATE.md:** edit §Current pointer IN PLACE (lifecycle 3 → DONE; row 71 `done`); demote to §Recent lineage capped at five; update counts (fixtures 117, DECISIONS ADR-0293 COMPLETE).
- [ ] **PROGRESS.md:** finalize — every break's ACTUAL firing assertion, the verbatim red-first records, the T5 dispatch-verify trap result, any break substitutions.
- [ ] **Router roll** (`next-prompt.txt` — TRACKED despite .gitignore; edit in the stage worktree; locate by SUBJECT). Row 71 done ⇒ the sentinel's check (1) goes SILENT for row 71 ⇒ the roller SELF-PICKS the next subject at the phase-72 BRAINSTORM (the 2026-07-12 standing directive) unless the sentinel fires (it does not: checks (2)+(3) still print).
- [ ] **Sentinel re-run MECHANICALLY:** check (1) goes silent when row 71 flips (every OTHER chartered row already `done`); (2) still prints 3 via the full-phrase command (`grep -cE 'remaining deferred \(not-yet-chartered\) candidates:'` → 3 — the ROUTE narrowing does NOT drop the whole Observability sentence); (3) unchanged (`NEVER OPENED: gRPC/Runtime/WASM`) ⇒ does NOT fire; no `stop` file.
- [ ] **Memory updates owed (SPEC §13):** (i) the `*FilterChain.RouteMetaLookup()` seam — the ROUTE analogue of phase-70's `DynamicMetadata()` accessor; a consumer at the HCM emit layer wraps `RouteMetadata().GetFilterMetadata()[ns]` via `structpb.NewStructValue` to feed the shared `descend` walk (extends `reference_filterchain_dynamicmetadata_accessor`). (ii) the ROUTE resolve descends the FULL `MetaPath` (the REQUEST `[1:]` slice is a Bucket-pre-keying artifact, not a general metadata rule).
- [ ] **Squash-push by the controller** at stage-close.

**Commit (stage-close docs):** `phase 71 (tracing-custom-tags-metadata-route) IMPL: …` (controller composes at close).

---

## Self-review against SPEC-71

| SPEC obligation | Where |
|---|---|
| the `kindMetadataRoute` const (§3.2) | T1 |
| the ROUTE accept arm (cloning REQUEST, MetaPath FULL) replacing the `:245-246` reject; CLUSTER/HOST/unset rejects UNCHANGED (§3.2/§3.6) | T1 |
| the `routeMetaLookup` 4th param + `kindMetadataRoute` resolve arm, the `request_header` default rule, the FULL-path descent (§3.3/§3.4, S1) | T2 |
| `descend`/`structpbValueToString` REUSED VERBATIM (NOT re-derived — filter-free) (§3.3, RD-DESCEND/RD-SERIALIZE) | T2, T8 (cycle guard) |
| the NEW `*FilterChain.RouteMetaLookup()` accessor + the `structpb` import (§3.5, S2, RD-SEAM) | T3 |
| the `routeMetaLookup` thread at all 18 callers — `nil` at the 3 no-chain sites, `chain.RouteMetaLookup` at 15 (the 4th arg appends) (§3.5, RD-CALLERS) | T4 |
| the fuzz-seed repurpose + dispatch-verify (§7, S4, RD-FUZZ) | T5 |
| ONE OTLP fixture `0115`, two ROUTE custom_tags, cross-side EXACT key+value, NO writer (§8, RD-FIXTURE) | T6 |
| BC B1–B2 pinned wording (§9) | T7 |
| a SINGLE FLAT ROW; ADR-0045 valve armable-but-unconsumed (§10) | §1, this table |
| six-gate + cycle guard + full-117-dir + -race + counts + envelope audit (§10 T8, §15) | T8 |
| +0 packages / +0 modules / +0 stats / +0 fuzzers / +0 BackendKinds (§4, §7) | T5, T8 |
| ADR-0293 completed IN PLACE, no new ADR (§14) | T9 |
| Sentinel: narrow the sentence AT THE IMPL row-done, not before (§12) | T9 |
| Memory updates (§13) | T9 |

**Task count: 9** — matching the SPEC's ~9 anticipation (comfortably under the ADR-0045 ~15 ceiling). **ADR-0045 escape valve ARMABLE, UNCONSUMED — no split**: the parse + resolve sit on ONE landed `internal/tracing` engine; `RouteMetadata()` is a landed read-only substrate; the accessor (S2) is one small method; no second subsystem can strand a leg (`internal/xds`/`internal/tls`/`internal/boot`/`internal/listener`/`internal/bootstrap` untouched). Sequencing: T1 (parse) → T2 (resolve, consumes T1's `kindMetadataRoute`) → T3 (accessor, independent) → T4 (wires T2's 4th param + T3's accessor at the callers) → T5 (fuzz) → T6 (fixture) → T7 (BC) → T8/T9 (close).

**⚠️ The IMPL's standing instruction: a PLAN is not evidence either.** **RE-DERIVE this document; do not execute it.** Where it cites, go look; where it claims control flow, walk the call graph; default to REFUTED. Start where this PLAN is most confident (all re-derived read-only at the PLAN, §1): the ZERO-drift anchor set (RD-EXACT, 32/32), the clone skeletons (§1.1, cloning the LANDED phase-70 REQUEST arm verbatim), the `structpb`-absent-from-chain.go S2 fact (RD-SEAM), and the 18-caller map with exactly 3 nil sites (RD-CALLERS).
