# PLAN 70 — tracing `custom_tags` `metadata` type, `REQUEST` MetadataKind only — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL — `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Stage:** PLAN (lifecycle-state 2 → 3). Docs-only — ZERO production `.go`. Worktree `.worktrees/phase-70-plan`, branch `phase-70-tracing-custom-tags-metadata-plan`, tip **`2dd6b2d0`** (the phase-70 SPEC squash — master; production code byte-identical to the BRAINSTORM tree `9338507a` since the SPEC was docs-only apart from the ADR-0292 §Context append), per `feedback_git_worktrees`.
>
> **Row 70 STAYS `in-progress`** — the IMPL flips it `done` at its six-gate (ADR-0106, the SOLE leg — a SINGLE FLAT ROW, §10). **ADR-0292's §Context is ALREADY DRAFTED** at the SPEC squash (`grep -n '^## ADR-0292' docs/envoy-go/DECISIONS.md`, STATUS: **PROPOSED**); the IMPL **COMPLETES ADR-0292 IN PLACE** with §Decision + §Consequences — it does NOT append a new ADR, does NOT renumber. DECISIONS tail stays **ADR-0292**, next-free **ADR-0293** (`[RUN]`: `grep -c '^## ADR-0293' docs/envoy-go/DECISIONS.md` → 0). **This PLAN adds NO ADR content; DECISIONS is UNTOUCHED at the PLAN.**
>
> **Baselines RE-DERIVED at `2dd6b2d0` (`[RUN]`, NOT copied):** fixtures **115** (`ls -d test/fixtures/[0-9]*/ | wc -l`; numeric tail `0113-stats-sink-otlp-knobs` ⇒ next fixture `0114`; tracing/OTLP fixtures TEMPLATE all ports — no in-container port to pick, C5/RD-PORTS) · fuzzers **55** (`grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l`) · BackendKind tail **38** (`H2GoawayResponder`) · stat surface **1201** · DECISIONS tail **ADR-0292** (PROPOSED; next-free ADR-0293) · go.mod modules **2** (lineage figure; `structpb` already an `internal/dynamicmetadata` dep, `type/metadata/v3` + `protojson` resolve at existing directs — re-check `git diff go.mod` after tidy at T8).
>
> **Sentinel expectation:** check (1) prints `NOT DONE: row 70`; check (2) prints **3** via the full-phrase form ONLY — `grep -cE 'remaining deferred \(not-yet-chartered\) candidates:' docs/envoy-go/ROADMAP.md` (`reference_sentinel_deferred_sentence_live_vs_historical` — cite the command, never the adjective); check (3) prints `NEVER OPENED: gRPC/Runtime/WASM`. **No deferred-sentence edit at ANY stage of this row** (SPEC §12); the live Observability `candidates:` sentence rolls `custom_tags (metadata)` OUT at the IMPL row-done edit, NOT before (the phase-57 graphite precedent).
>
> **⚠️ NO PARALLEL STREAM.** Master (`2dd6b2d0`) IS the SPEC squash — the ONLY delta over the BRAINSTORM tree `9338507a` is docs (`SPEC.md` + the ADR-0292 §Context append). So the production tree is byte-identical to what the SPEC re-derived; §1 is the structural decisions the SPEC delegated to the PLAN plus a full re-verification (which found **ZERO drift** — every SPEC §11 anchor exact at `2dd6b2d0`).
>
> **⚠️ RE-DERIVE, do not execute.** A PLAN is not evidence (a PLAN's cites drift; a SPEC's do too). Where this document cites, go look; where it claims control flow, walk the call graph; default to REFUTED (`feedback_brief_citations_not_evidence`, `reference_quoting_is_not_executing`).

---

## 1. Re-derivation ledger — every SPEC §11 anchor re-opened at `2dd6b2d0`

**All SPEC §3/§9/§11 code anchors RE-DERIVED at `2dd6b2d0` by an independent read-only re-derivation agent that re-opened `internal/tracing/{config.go,resolve.go}`, `internal/filter/http/chain.go`, `internal/dynamicmetadata/dynamicmetadata.go`, `internal/filter/http/ratelimit/descriptors.go`, `internal/filter/hcm/{accesslog_emit.go,connection.go,h2dispatch.go,h3dispatch.go,fuzz_test.go}`, the test files, and the fixture templates.**

**RESULT: ZERO unacknowledged drift.** Every SPEC-cited line number matched the current master tip EXACTLY (config.go :52-56/:63-71/:190-192/:194-218/:214-215/:219-223; resolve.go :14/:23-35/:36-52; chain.go :274/:837/:963/:1014; dynamicmetadata.go :33; accesslog_emit.go :25/:55/:85/:116/:147/:177 + gates :28/:87/:149; descriptors.go :1057/:1116; fuzz_test.go :28; config.go :122/:130; and all 18 caller lines). The C1 correction is CONFIRMED (`func (c *FilterChain) DynamicMetadata()` absent; `:837`/`:963` are on `*decoderCB`/`*encoderCB`). All five new identifiers are collision-free. The findings below (RD*) are therefore the *structural decisions the SPEC delegated to the PLAN* (the exact code skeletons an implementer clones) plus the load-bearing re-confirmations; adversarial verification (§1.2) must confirm or refute each.

| # | Anchor / SPEC claim | RE-DERIVED at `2dd6b2d0` | Where |
|---|---|---|---|
| **RD-EXACT** | SPEC §11 cites ~30 code anchors | **ALL EXACT — ZERO drift.** The tree tip (`2dd6b2d0`) is one commit ahead of the SPEC's cited re-derivation commit (`9338507a`), but NO line numbers shifted (the SPEC squash was docs-only). The SPEC's cites are adopted verbatim. | all |
| **RD-KIND** | SPEC §3.2: add `kindMetadata` to the `customTagKind` iota + extend `CustomTagSpec` | **CONFIRMED.** `customTagKind` block `config.go:52-56` = `kindLiteral`(0) / `kindRequestHeader`(1) / `kindEnvironment`(2); `kindMetadata` appends at iota==3. `CustomTagSpec` `config.go:63-71` has `Key/Kind/LiteralValue/HeaderName/EnvName/DefaultValue/HasDefault` — ADD `MetaNamespace string` + `MetaPath []string`, REUSE `DefaultValue`/`HasDefault` (the `kindRequestHeader` idiom: `HasDefault = DefaultValue != ""`, C2). The struct comment line `// Kind==kindRequestHeader only: …` at `:70` widens to name `kindMetadata` too. | T1 |
| **RD-REJECT** | SPEC §3.2/§3.6: REPLACE the `:214-215` metadata reject with accept-REQUEST + four rejects | **CONFIRMED — the exact reject case is `config.go:214-215`:** `case ct.GetMetadata() != nil: return nil, fmt.Errorf("tracing: custom_tags metadata type unsupported")`. This whole `case` is replaced. The switch (`:194-218`) is a `switch { case ct.GetLiteral()!=nil: … }` shape; the metadata case grows an inner `switch`/`if` on `md.GetKind()`. The empty-tag reject `:190-192` and first-wins dedup `:219-223` are UNCHANGED (a later same-key metadata tag drops after structural validation). All reject substrings ADR-0080-distinct (§3.2). | T1 |
| **RD-DEFAULT** | SPEC §3.4/C2: the `metadata` default rule mirrors `kindRequestHeader`, NOT `kindEnvironment` | **CONFIRMED — both arms re-derived verbatim (§1.1 skeleton).** `kindRequestHeader` (`resolve.go:23-35`): nil-tolerant lookup → resolve-if-present (incl. `""`, appends `KV{Key,""}`) → else `if s.HasDefault` default → else omit. This is the SHAPE the `kindMetadata` arm clones. `kindEnvironment` (`resolve.go:36-52`): `v,present := os.LookupEnv(...); if !present { v = DefaultValue }; if v != "" { append }` — the omit-iff-resolved-empty CONTRAST; the metadata arm must NOT use this. | T2 |
| **RD-DESCEND** | SPEC §3.3/C3: CLONE the ratelimit `descendStructpbValue` SHAPE (`descriptors.go:1116`), do NOT import | **CONFIRMED — the clone skeleton extracted verbatim (§1.1).** `descriptors.go:1116` `func descendStructpbValue(value *structpb.Value, rest []*metadatav3.MetadataKey_PathSegment) (string, bool)`: for each seg `st := cur.GetStructValue(); if st==nil→("",false)`; `next,ok := st.GetFields()[segKey]; if !ok||next==nil→("",false)`; `cur = next`. **Two DIVERGENCES the tracing clone applies:** (a) a `[]string` variant (the `MetaPath` segment keys are pre-extracted at parse), not `[]*MetadataKey_PathSegment`; (b) the terminal serializes non-string too (`structpbValueToString`, RD-SERIALIZE), where ratelimit requires a STRING terminal. The descent loop body transcribes directly. Package layering: `internal/tracing` gains NO import of `internal/filter/http/ratelimit` (a ~12-line local `descend`; the cycle guard `go list -deps ./internal/tracing` stays filter-free — T8). | T2 |
| **RD-SERIALIZE** | SPEC §3.3/C4: `structpbValueToString` — string→raw, non-string→`protojson.Marshal`+`json.Compact` | **CONFIRMED buildable + the boundary re-derived.** `*structpb.Value_StringValue` → `k.StringValue` (raw, even `""` → `("", true)` — the present-empty EMIT, P4-iii/C2); `*structpb.Value_NullValue` → `("", false)` (a documented boundary — treat as unresolvable → default/omit); else `protojson.Marshal(v)` then `json.Compact` → `(compact, true)`. The V1 SPEC probe EXECUTED this table (string→`"x"` WITH quotes ⇒ string MUST be `GetStringValue()`-special-cased; `42`→`42`; `3.14`→`3.14`; `true`→`true`; `{a:b}`→`{"a":"b"}`; list `["x", "y", "z"]`→`json.Compact`→`["x","y","z"]` ⇒ `json.Compact` is genuinely load-bearing). `resolve.go` currently imports ONLY `"os"` (`:3`) — ADD `structpb` + `protojson` + `encoding/json` (all existing-module — RD-MOD). Run the emitting package (`internal/tracing`) under `-race` (`reference_detrand_race_catches_protojson_value_substring`). | T2 |
| **RD-ACCESSOR** | SPEC §3.5/C1: `*FilterChain` has NO `DynamicMetadata()`; ADD one beside `SetDynamicMetadata` | **CONFIRMED — the load-bearing correction.** `grep 'func (c \*FilterChain) DynamicMetadata' internal/filter/http/chain.go` → ZERO. `SetDynamicMetadata` at `chain.go:1014` (`func (c *FilterChain) SetDynamicMetadata(b *dynamicmetadata.Bucket)`); the unexported field `dynamicMetadata *dynamicmetadata.Bucket` at `:274` (init `:314` `dynamicmetadata.NewBucket()`, Reset+nil at Destroy `:676-677`). The `:837`/`:963` accessors are on `*decoderCB`/`*encoderCB` (`return d.c.dynamicMetadata`/`return e.c.dynamicMetadata`) — NOT `*FilterChain` (the BRAINSTORM mis-cite REFUTED). ADD `func (c *FilterChain) DynamicMetadata() *dynamicmetadata.Bucket { return c.dynamicMetadata }` beside `:1014`. Import `chain.go:19` `"github.com/pgdad/envoy-go/internal/dynamicmetadata"` (no alias) is already present. | T3 |
| **RD-BUCKET** | SPEC §3.5: `Bucket.Get(ns,key)(*structpb.Value,bool)` nil-receiver-safe | **CONFIRMED.** `internal/dynamicmetadata/dynamicmetadata.go:33` `func (b *Bucket) Get(filterName, key string) (*structpb.Value, bool)` with `if b == nil || b.m == nil { return nil, false }` (`:34-35`, ADR-0085). So `chain.DynamicMetadata().Get` is safe even when the chain's bucket is nil (a nil `*Bucket` method value still dispatches — the receiver-nil guard fires). | T3, T4 |
| **RD-CALLERS** | SPEC §3.5/§11: thread `metaLookup` at all 18 emit callers; `nil` at the 3 no-chain 404 sites | **CONFIRMED EXACT — 5+6+7=18, exactly 3 no-chain-404 sites, `chain` var name uniform.** `emitAccessLog`/`H2`/`H3` signatures `accesslog_emit.go:25/:85/:147`; `ResolveCustomTags` calls `:55/:116/:177` (all TWO-arg `specs, headerLookup` — add a 3rd `metaLookup`); span-gate `:28/:87/:149` (`traceDecision != nil && traceDecision.Sample`). The chain variable is named `chain` in all three dispatch files (built `connection.go:350`/`h2dispatch.go:327`/`h3dispatch.go:148`). **The 3 nil sites:** `connection.go:330` (404, `traceDecision` nil PRE-Decide), `h2dispatch.go:313` (no-match, `c.traceDecision` nil), `h3dispatch.go:130` (404, nil) — all BEFORE the chain is built, so `ResolveCustomTags` is provably never reached (the gate). The other 15 pass `chain.DynamicMetadata().Get`. **Subtlety (harmless):** `h3dispatch.go:210` (500) has `chain` in scope but `traceDecision` STILL nil there (Decide runs `:247`) — so the span block won't fire regardless; passing `chain.DynamicMetadata().Get` is valid-but-never-reached (NOT a nil site — `chain` exists). `accesslog_emit.go` imports (`:3-14`) LACK `structpb` — ADD it (the param type). | T4 |
| **RD-FUZZ** | SPEC §7: a `FuzzHCMConfigParse` seed (accepted REQUEST + rejected ROUTE), dispatch-verified | **CONFIRMED — host + skeleton + dispatch.** `FuzzHCMConfigParse` at `internal/filter/hcm/fuzz_test.go:28`; the seed helpers `withCustomTags`(:37)/`withReqHeaderTags`(:51)/`withEnvTags`(:66) are the clone pattern (`mkHCM(func(h){ h.Tracing = &hcmv3.HttpConnectionManager_Tracing{CustomTags: []*tracingv3.CustomTag{…}} })` then `f.Add(x.GetTypeUrl(), x.GetValue())` — §1.1). **Dispatch CONFIRMED:** `config.go:122` (`parseCustomTags(t.GetCustomTags())`) precedes `:130-133` (the `provider required` check) ⇒ a well-formed HCM tracing block reaches the metadata arm BEFORE the provider check. Add `metadatav3` import to `fuzz_test.go`. Count STAYS 55 (`^func Fuzz`; a seed is +0 — `reference_fuzzer_count_docs_drift`). | T5 |
| **RD-LUA** | SPEC §8: clone `0114` from `0106` with a Lua `dynamicMetadata():set` writer | **SHARPENED — `0106` has NO Lua; the Lua-writer clone source is `0027-http-lua-full-bridge`.** `0106-tracing-custom-tags-environment/` (`driver/ envoy-go.yaml envoy.yaml expectations.yaml README.md`) is the tracing/OTLP CHASSIS to clone (H1 → backend, OTLP provider → `otlptrace.Server`, `random_sampling:100`), but it carries NO Lua filter. The Lua stanza + script clone from `test/fixtures/0027-http-lua-full-bridge/`: the script `scripts/e_dynamic_metadata.lua` does `rh:streamInfo():dynamicMetadata():set("envoy.test","k","v-fixture27")` (the exact `set(ns,key,value)` 3-arg shape, cross-side-identical with the reference `envoy.filters.http.lua`), and the http_filter stanza is `- name: envoy.filters.http.lua` / `"@type": …lua.v3.Lua` (placed BEFORE the router on BOTH YAMLs). | T6 |
| **RD-PORTS** | SPEC §3/§8/C5: no in-container port to pick | **CONFIRMED (C5 holds).** `0106`'s `envoy-go.yaml`/`envoy.yaml` template every port — `{{.AdminPort}}`/`{{.ListenerPort}}`/`{{.BackendPort}}`/`{{.OTLPPort}}` (+ `{{.BackendHost}}`/`{{.OTLPHost}}` on the reference side). The cloned `0114` inherits the templated ports; there is NO hardcoded `port_value` to increment. Drop any "next free port" line. | T6 |
| **RD-IDENT** | SPEC §5: collision checks | **ALL FREE at `2dd6b2d0`.** `grep -rn 'kindMetadata\|MetaNamespace\|MetaPath\|metaLookup\|structpbValueToString' --include='*.go' internal/` → ZERO. `grep -rn 'func.*descend' … internal/` → exactly ONE hit (`descriptors.go:1116 descendStructpbValue`, a DIFFERENT package) — a tracing-local `descend` does NOT collide. `metadatav3` is the ESTABLISHED alias (ratelimit, 3 sites) — adopt it (distinct package from `tracingv3`). Fixture `0114-tracing-custom-tags-metadata` does not exist. | T1, T2, T6 |
| **RD-MOD** | SPEC §4: +0 go.mod modules | **CONFIRMED buildable.** `structpb` is already an `internal/dynamicmetadata` dep (used by `Bucket.Get`); `type/metadata/v3` (`metadatav3`) resolves at the direct `go-control-plane/envoy v1.32.4` (used by ratelimit); `protojson`/`encoding/json` are standard/existing-module. `go mod tidy -diff` anticipated EMPTY — re-check `git diff go.mod` after tidy at T8 (`reference_new_subpackage_pulls_transitive_module`). | T8 |

### 1.1 The clone skeletons the SPEC delegated to the PLAN (each RE-DERIVED verbatim, not invented)

- **The `descend` clone + `structpbValueToString` (T2, `resolve.go`).** The ratelimit `descendStructpbValue` shape, adapted to a `[]string` path + a serializing terminal:
  ```go
  // descend walks a nested structpb StructValue by the pre-extracted segment keys.
  // Cloned in SHAPE from internal/filter/http/ratelimit descendStructpbValue
  // (NOT imported — internal/tracing stays filter-free). segs may be empty
  // (a single-segment MetaPath resolves entirely via the Bucket.Get first key).
  func descend(v *structpb.Value, segs []string) (*structpb.Value, bool) {
      cur := v
      for _, seg := range segs {
          st := cur.GetStructValue()
          if st == nil {
              return nil, false
          }
          next, ok := st.GetFields()[seg]
          if !ok || next == nil {
              return nil, false
          }
          cur = next
      }
      return cur, true
  }

  // structpbValueToString renders a resolved google.protobuf.Value to the
  // reference's string form (P3): string→raw (even ""); NullValue→unresolvable;
  // else protojson.Marshal + json.Compact (number canonical decimal, bool
  // true/false, struct/list compact JSON). json.Compact strips the detrand
  // whitespace to match the reference's compact form (V1-executed).
  func structpbValueToString(v *structpb.Value) (string, bool) {
      switch k := v.GetKind().(type) {
      case *structpb.Value_StringValue:
          return k.StringValue, true // raw, incl. "" (present-empty EMIT, P4-iii)
      case *structpb.Value_NullValue:
          return "", false // documented boundary → default/omit
      default:
          b, err := protojson.Marshal(v)
          if err != nil {
              return "", false
          }
          var buf bytes.Buffer
          if err := json.Compact(&buf, b); err != nil {
              return "", false
          }
          return buf.String(), true
      }
  }
  ```
  (Re-derive the EXACT `structpb`/`protojson`/`json`/`bytes` import spellings at the IMPL tip; the snippet is the shape. `json.Compact` needs a `*bytes.Buffer` — `bytes` is a further import; an implementer may instead re-derive whether a `json.RawMessage` round-trip is simpler, but the SPEC pins `protojson.Marshal`+`json.Compact` and the V1 probe proved it load-bearing.)
- **The `kindMetadata` resolve arm (T2, `resolve.go`)** — mirrors `kindRequestHeader` (`:23-35`), NOT `kindEnvironment`:
  ```go
  case kindMetadata:
      var v *structpb.Value
      var ok bool
      if metaLookup != nil { // nil-tolerant, like headerLookup
          v, ok = metaLookup(s.MetaNamespace, s.MetaPath[0]) // MetaPath[0] always exists (min_items 1, validated at parse)
      }
      if ok {
          v, ok = descend(v, s.MetaPath[1:])
      }
      if ok {
          if str, emit := structpbValueToString(v); emit {
              out = append(out, KV{Key: s.Key, Str: str})
              continue
          }
      }
      if s.HasDefault {
          out = append(out, KV{Key: s.Key, Str: s.DefaultValue})
      } // else omit
  ```
- **The `parseCustomTags` metadata arm (T1, `config.go`)** — REPLACES the `:214-215` `case ct.GetMetadata() != nil:` reject:
  ```go
  case ct.GetMetadata() != nil:
      md := ct.GetMetadata()
      k := md.GetKind() // *metadatav3.MetadataKind — the kind-getters live HERE, not on md (V1)
      switch {
      case k == nil:
          return nil, fmt.Errorf("tracing: custom_tags metadata tag %q kind required", tag)
      case k.GetRequest() != nil:
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
          spec = CustomTagSpec{Key: tag, Kind: kindMetadata, MetaNamespace: mk.GetKey(), MetaPath: path, DefaultValue: dv, HasDefault: dv != ""}
      case k.GetRoute() != nil:
          return nil, fmt.Errorf("tracing: custom_tags metadata route kind unsupported")
      case k.GetCluster() != nil:
          return nil, fmt.Errorf("tracing: custom_tags metadata cluster kind unsupported")
      case k.GetHost() != nil:
          return nil, fmt.Errorf("tracing: custom_tags metadata host kind unsupported")
      default:
          return nil, fmt.Errorf("tracing: custom_tags metadata tag %q kind required", tag)
      }
  ```
  **⚠️ V1 SEVERE correction (compile-proven):** the four kind-getters `GetRequest`/`GetRoute`/`GetCluster`/`GetHost` are methods on `*metadatav3.MetadataKind` (`type/metadata/v3/metadata.pb.go:164-185`), **NOT on `*tracingv3.CustomTag_Metadata`** (`custom_tag.pb.go:373-387` exposes only `GetKind()`/`GetMetadataKey()`/`GetDefaultValue()`). The skeleton binds `k := md.GetKind()` and branches on `k.GetRequest()` etc — `md.GetRequest()` does NOT compile (`type *tracingv3.CustomTag_Metadata has no field or method GetRequest`). The SPEC §3.1's `md.GetRequest()` shorthand is the same defect; the IMPL routes through `md.GetKind()`. The `k==nil` and `default` both hit "kind required" (an unset oneof yields nil kind; `default` guards an unforeseen future arm). The getters are nil-receiver-safe, but the `k==nil` guard makes the branch order explicit.
- **The accessor (T3, `chain.go`)** — beside `SetDynamicMetadata` (`:1014`):
  ```go
  // DynamicMetadata returns the chain's per-request dynamic-metadata bucket
  // (nil-tolerant via Bucket.Get). Added phase 70 so the HCM tracing emit sites
  // can resolve a REQUEST-kind custom_tag metadata value; mirrors the decoderCB/
  // encoderCB accessors, which read the same field.
  func (c *FilterChain) DynamicMetadata() *dynamicmetadata.Bucket {
      return c.dynamicMetadata
  }
  ```
- **The metadata fuzz seed (T5, `fuzz_test.go`)** — clones the `withEnvTags` shape (`:66-75`):
  ```go
  withMetaTags := mkHCM(func(h *hcmv3.HttpConnectionManager) {
      h.Tracing = &hcmv3.HttpConnectionManager_Tracing{
          CustomTags: []*tracingv3.CustomTag{
              {Tag: "meta_ok", Type: &tracingv3.CustomTag_Metadata_{Metadata: &tracingv3.CustomTag_Metadata{
                  Kind:        &metadatav3.MetadataKind{Kind: &metadatav3.MetadataKind_Request_{Request: &metadatav3.MetadataKind_Request{}}},
                  MetadataKey: &metadatav3.MetadataKey{Key: "envoy.test", Path: []*metadatav3.MetadataKey_PathSegment{{Segment: &metadatav3.MetadataKey_PathSegment_Key{Key: "k"}}}},
                  DefaultValue: "fb",
              }}},
              {Tag: "meta_bad", Type: &tracingv3.CustomTag_Metadata_{Metadata: &tracingv3.CustomTag_Metadata{
                  Kind: &metadatav3.MetadataKind{Kind: &metadatav3.MetadataKind_Route_{Route: &metadatav3.MetadataKind_Route{}}},
              }}},
          },
      }
  })
  f.Add(withMetaTags.GetTypeUrl(), withMetaTags.GetValue())
  ```
  (Re-derive the EXACT `metadatav3.MetadataKind_Request_`/`_Route_` + `MetadataKey_PathSegment_Key` wrapper spellings at the IMPL tip — the go-control-plane oneof wrapper names; the `parseCustomTags` metadata arm de-references them, so the seed must build them concretely. Two tags in ONE seed exercises BOTH the accept and a reject dispatch — but a fuzz `f.Add` only needs to REACH the arm; the accept/reject unit coverage is T1's `config_test.go`.)

### 1.2 Adversarial-pass record

**TWO independent verifiers ran against the draft in PRIVATE scratch before landing** (`reference_parallel_subagents_private_scratch`; the real repo left untouched, no worktrees registered):

- **V1 — code-claims re-derivation + by-execution compile probes.** V1 re-derived every load-bearing skeleton at tip and compile-checked the disputed proto call chains in scratch. **ONE SEVERE, ZERO MODERATE, ZERO MINOR — the SEVERE folded in.** The SEVERE: the `parseCustomTags` metadata skeleton (§1.1) and the SPEC §3.1 shorthand branched on `md.GetRequest()`/`GetRoute()`/`GetCluster()`/`GetHost()` where `md` is `*tracingv3.CustomTag_Metadata` — which exposes ONLY `GetKind()`/`GetMetadataKey()`/`GetDefaultValue()` (`custom_tag.pb.go:373-387`); the four kind-getters live on `*metadatav3.MetadataKind` (`metadata.pb.go:164-185`). Compile-proven both directions (`md.GetRequest()` → `has no field or method GetRequest`; `md.GetKind().GetRequest()` → builds). **CORRECTED here:** the §1.1 skeleton now binds `k := md.GetKind()` and branches on `k.GetRequest()` etc; the T1 Consumes list + Step 3 note carry the explicit warning. Everything else CONFIRMED by independent re-derivation / compile-probe: the `descend`/`structpbValueToString` skeleton builds (`bytes` accounted; the `structpb.Value_StringValue`/`Value_NullValue` oneof wrappers valid; the ratelimit clone faithful); the fuzz-seed wrapper spellings ALL compile (`CustomTag_Metadata_`/`MetadataKind_Request_`/`MetadataKind_Route_`/`MetadataKey_PathSegment_Key`); the C1 accessor absent + buildable (RD-ACCESSOR); the 18-caller map EXACT with exactly 3 nil sites and `h3dispatch.go:210` correctly a chain site (RD-CALLERS); the `request_header` default rule is the right clone and `kindEnvironment` the wrong one (C2); +0 go.mod modules (`protobuf v1.36.11` direct covers `structpb`+`protojson`; `metadatav3` under the direct go-control-plane; `encoding/json`/`bytes` stdlib).
- **V2 — process/consistency/SPEC-coverage.** **ZERO SEVERE, ZERO MODERATE, TWO MINOR.** All checks PASS: SPEC §10/§11 coverage complete (every edit site written by a task; ZERO cited-but-unwritten obligation — the phase-69 `TestNoNewStat` defect class is correctly avoided, SPEC §7 says NO such guard for tracing); the stage-close mechanics correct (row 70 STAYS `in-progress`; DECISIONS UNTOUCHED at the PLAN — ADR-0292 stays PROPOSED; ADR-0292 completed IN PLACE at the IMPL, no renumber; NO deferred sentence narrowed at the PLAN); counts mechanically re-run and internally consistent (fixtures 115→116, fuzzers 55, DECISIONS tail ADR-0292, go.mod 2); the sentinel does NOT fire; the envelope (7 production files + the byte-untouched list) consistent across File-structure/Global-Constraints/T8/Self-review; the break protocol complete with full `TestDifferential/0114-…` selectors; SINGLE FLAT ROW / 9 tasks / sound sequencing; format faithful to the phase-69 PLAN. The two MINOR: (i) the V1/V2 records were placeholder `[to fill]` — now populated (this block + PROGRESS); (ii) BackendKind 38 carried from SPEC, re-run mechanically at T8 — internally consistent. **The design direction — a `REQUEST`-kind metadata custom tag on the landed dynamic-metadata `Bucket`, single flat row, 9 tasks — is unchanged from SPEC §10; only the `md.GetKind()` receiver defect was a real gap, now closed.**

---

## Global Constraints

- **ONE stage per session.** This session: the PLAN only. No production `.go`. After it lands: roll to the phase-70 IMPL.
- **SEVEN functionally-edited production files, ZERO new packages** (SPEC §4, §15): `internal/tracing/config.go` · `internal/tracing/resolve.go` · `internal/filter/http/chain.go` · `internal/filter/hcm/accesslog_emit.go` · `internal/filter/hcm/connection.go` · `internal/filter/hcm/h2dispatch.go` · `internal/filter/hcm/h3dispatch.go`. **New exported symbols: `func (c *FilterChain) DynamicMetadata()` on `internal/filter/http` (the C1 accessor) + the `ResolveCustomTags` signature growth + the `CustomTagSpec` fields + the `kindMetadata` const + the `descend`/`structpbValueToString` helpers (all in `internal/tracing`).** The `internal/xds` zero-new-symbol discipline is UNTOUCHED (xds not touched). **BYTE-UNTOUCHED:** `internal/xds`, `internal/tls`, `internal/boot`, `internal/listener`, `internal/bootstrap`, `validate/`, `internal/dynamicmetadata` (CONSUMED — the new `Get` caller, not modified), `internal/filter/http/ratelimit` (the `descend` SHAPE is CLONED, not imported), `internal/filter/http/lua` (the fixture WRITER, not modified).
- **`REQUEST` MetadataKind ONLY.** `ROUTE`/`CLUSTER`/`HOST`/unset-kind reject LOUDLY with distinct substrings (envoy-go-strict DEPARTURE — the reference BOOTS them, P6/C6). Empty `metadata_key.key`/empty `path`/empty segment reject (PGV-PARITY — the reference boot-rejects too). An entirely-ABSENT `metadata_key` yields `key==""` for envoy-go ⇒ the empty-namespace arm (a minor strict departure on that one shape — named in BC).
- **The default rule is `request_header`, NOT `environment`** (C2): a present-but-empty metadata value emits `""` (does NOT fall to the default); absent + non-empty default → default; absent + empty/omitted default → omit. `HasDefault = DefaultValue != ""`.
- **Value→string serialization** (C4): string→raw (`GetStringValue()`); `NullValue`→unresolvable (boundary); else `protojson.Marshal` + `json.Compact` (`reference_detrand_race_catches_protojson_value_substring` — run `internal/tracing` under `-race`).
- **Counts at the IMPL:** fixtures **115 → 116** (`0114-tracing-custom-tags-metadata`) · fuzzers **55 (+0, a seed only)** · stat surface **1201 (+0)** — a span attribute registers no stat; NO `TestNoNewStat` guard (tracing has no per-tag stat surface; 59/62/63 added none) · BackendKind **38 (+0)** — the Lua writer + `otlptrace` receiver are existing/driver-owned · go.mod **+0** (SPEC metric "2" carried; re-check `git diff go.mod` after tidy — RD-MOD) · ZERO new packages · DECISIONS tail stays **ADR-0292** (completed IN PLACE at the IMPL; next-free ADR-0293).
- **The pinned §9 wording lands MECHANICALLY** — B1/B2 are named obligations with the SPEC §9 replacement text; never silent rewrites, never paraphrases. They land at T7, atomically with the row-done edit; ADR-0292 completes at T9's stage-close.
- **Per-task hygiene** (`feedback_pertask_gofmt_lint`): `gofmt -l` + `go vet` + `golangci-lint run` on every touched package.
- **Worktree discipline** (`feedback_git_worktrees` / `feedback_subagent_worktree_detach` / `feedback_subagent_worktree_path_targeting`): pin the canonical root; the controller verifies the MAIN checkout stays clean; deliberate breaks restore with **`git restore` only**; breaks run AFTER committing (`reference_break_protocol_commit_first`).
- **Subagents commit locally; the controller squash-pushes at stage-close** (`feedback_subagents_no_push`, `feedback_push_to_origin`). Subagents auto-commit per CLAUDE.md; the controller squashes at close. Locate commits by SUBJECT (`git log --grep 'phase 70'`), never by position.
- **`reference_sds_init_fetch_timeout_dial_budget_flake` / `reference_0061_ring_hash_spread_flake`** — a `TestProvider_*_Timeout` under `-race` or a `0061` spread failure is PRE-EXISTING on master (one occurrence each). Do not reflex-classify as a phase-70 regression; a SECOND occurrence justifies investigating margins.

### Break protocol (binding on every task)

- **A break must COMPILE** (`reference_plan_break_instructions_dont_compile`). Breaks flagged `[NOT pre-compiled — substitution rule applies]`: at IMPL time, if it does not compile, **substitute a compiling equivalent, REPORT the substitution, record the TRUE result**.
- **A break must DISCRIMINATE** (`reference_probe_must_discriminate`): before recording it as proof, ask what the OTHER hypothesis would have printed.
- **`-count=1` on EVERY differential break** (`reference_differential_break_protocol_count1`); caching serves a stale PASS.
- **Confirm WHICH assertion fired** (`reference_deliberate_break_wrong_assertion`) — and whether a second property's firing is ENTAILED by the first.
- **A break that does NOT fire is a FINDING** — record it honestly in PROGRESS; do not route around it.
- **Full selector only:** `-run 'TestDifferential/0114-tracing-custom-tags-metadata'` — never bare `0114` (`reference_differential_run_selector`).
- **`Errorf` per independent property; `Fatalf` only for broken preconditions** (`reference_fatalf_makes_assertions_unreachable`).
- **The WRITER must be served-this-arm** (`feedback_probe_fresh_container_per_arm` — driver-owned servers): the span must carry the Lua-set value, not a vacuous default; confirm the positive arm actually resolved (a wrong-namespace control that falls to the default proves the writer was consulted).

### Identifier roster (`reference_spec_drafted_identifier_collision_check`)

**Verified FREE repo-wide at `2dd6b2d0` (`grep -rn --include='*.go'`, `.worktrees` excluded — RD-IDENT, all 0 hits):** `kindMetadata` · `MetaNamespace` · `MetaPath` · `metaLookup` · `structpbValueToString`. **`descend`** is free (the only `descend*` is ratelimit's `descendStructpbValue`, a different package). **`metadatav3`** is the ESTABLISHED ratelimit alias (adopt; distinct from `tracingv3`). **Fixture:** `test/fixtures/0114-*` does not exist; no in-container port to claim (RD-PORTS). **Any FURTHER name the IMPL coins** (the fixture `package driver` helpers, the Lua namespace/key strings, any test name): grep first, record the check.

---

## File structure

```
internal/tracing/config.go                        [EDIT]  T1 (kindMetadata const; CustomTagSpec += MetaNamespace/MetaPath; the metadata accept + 4 rejects replacing :214-215; metadatav3 import)
internal/tracing/config_test.go                   [EDIT]  T1 (accept REQUEST; reject ROUTE/CLUSTER/HOST/unset-kind/empty-namespace/empty-path/empty-segment — distinct substrings; first-wins dedup with a metadata tag)
internal/tracing/resolve.go                       [EDIT]  T2 (metaLookup param; the kindMetadata arm; descend; structpbValueToString; structpb/protojson/encoding/json/bytes imports)
internal/tracing/resolve_test.go                  [EDIT]  T2 (path-walk single/multi/unresolvable; the P3 serialization table json.Compact-compared; the P4 default/omit/present-empty matrix; nil-metaLookup tolerance; existing calls +nil 3rd arg)
internal/filter/http/chain.go                     [EDIT]  T3 (add func (c *FilterChain) DynamicMetadata() beside SetDynamicMetadata :1014)
internal/filter/http/chain_test.go                [EDIT]  T3 (accessor returns the live bucket; nil-tolerant)
internal/filter/hcm/accesslog_emit.go             [EDIT]  T4 (metaLookup param on the 3 emit methods → 3rd ResolveCustomTags arg; structpb import)
internal/filter/hcm/connection.go                 [EDIT]  T4 (5 callers — :330 nil; :464/:597/:699/:777 chain.DynamicMetadata().Get)
internal/filter/hcm/h2dispatch.go                 [EDIT]  T4 (6 callers — :313 nil; :396/:530/:577/:584/:613 chain.DynamicMetadata().Get)
internal/filter/hcm/h3dispatch.go                 [EDIT]  T4 (7 callers — :130 nil; :210/:280/:341/:367/:373/:395 chain.DynamicMetadata().Get)
internal/filter/hcm/accesslog_emit_test.go        [EDIT]  T4 (caller signatures += trailing nil metaLookup)
internal/filter/hcm/span_emit_test.go             [EDIT]  T4 (caller signatures += trailing nil metaLookup — grep at IMPL)
internal/filter/hcm/fuzz_test.go                  [EDIT]  T5 (withMetaTags seed; dispatch-verified; metadatav3 import; +0 func Fuzz)
test/fixtures/0114-tracing-custom-tags-metadata/  [ADD]   T6 (driver/, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md, scripts/*.lua), T6 (breaks)
docs/envoy-go/BEHAVIOR_CONTRACT.md                [EDIT]  T7 (B1 flip the metadata line; B2 the departures list)
docs/envoy-go/DECISIONS.md                        [EDIT]  T9 (ADR-0292 completed IN PLACE — §Decision + §Consequences)
internal/xds/** · internal/tls/** · internal/boot/** · internal/listener/** · internal/bootstrap/** · validate/** · internal/dynamicmetadata/** · internal/filter/http/ratelimit/** · internal/filter/http/lua/**  [BYTE-UNTOUCHED]
```

---

## Task 1 — `internal/tracing/config.go`: the `kindMetadata` const + `CustomTagSpec` fields + the metadata accept + four rejects

**Files:**
- Modify: `internal/tracing/config.go` (the `customTagKind` block `:52-56`; `CustomTagSpec` `:63-71`; the `parseCustomTags` metadata `case` `:214-215`; the imports)
- Test: `internal/tracing/config_test.go`

**Interfaces:**
- Produces: `kindMetadata` const; `CustomTagSpec.MetaNamespace string` + `.MetaPath []string`; the metadata accept path building `CustomTagSpec{Kind:kindMetadata, MetaNamespace, MetaPath, DefaultValue, HasDefault}`. Consumed by `resolve.go` (T2).
- Consumes: `ct.GetMetadata()` (`*tracingv3.CustomTag_Metadata`), `md.GetKind()` (→ `*metadatav3.MetadataKind`) then `k.GetRequest()`/`k.GetRoute()`/`k.GetCluster()`/`k.GetHost()` **on the returned `MetadataKind`, NOT on `md`** (V1 SEVERE — see §1.1), `md.GetMetadataKey()` (`*metadatav3.MetadataKey`), `mk.GetKey()`, `mk.GetPath()` (`[]*metadatav3.MetadataKey_PathSegment`), `seg.GetKey()`, `md.GetDefaultValue()`.

**Entry state:** clean `2dd6b2d0`-derived branch; `go test ./internal/tracing/ -count=1` green.

- [ ] **Step 1 — write the failing unit tests (red-first).** In `config_test.go`, model on the existing custom_tags parse tests (`grep -n 'metadata type unsupported\|environment tag\|request_header tag' internal/tracing/config_test.go` to find the reject-substring precedent + the accept-shape precedent). Add:
  1. `Test…Metadata_RequestAccept` — a `CustomTag{Tag:"t", Type:CustomTag_Metadata_{Metadata:{Kind:Request, MetadataKey:{Key:"ns", Path:[{Key:"a"},{Key:"b"}]}, DefaultValue:"d"}}}` parses to `CustomTagSpec{Key:"t", Kind:kindMetadata, MetaNamespace:"ns", MetaPath:["a","b"], DefaultValue:"d", HasDefault:true}`.
  2. `Test…Metadata_RejectKinds` — subtests ROUTE / CLUSTER / HOST / unset-kind each boot-FAIL with the distinct substring (`route kind unsupported` / `cluster kind unsupported` / `host kind unsupported` / `kind required`).
  3. `Test…Metadata_RejectStructural` — subtests empty namespace (`empty namespace`) / empty path (`empty path`) / empty path segment (`empty path segment`).
  4. `Test…Metadata_HasDefaultFalseWhenEmpty` — a REQUEST tag with `DefaultValue:""` → `HasDefault==false`.
  5. `Test…Metadata_FirstWinsDedup` — two same-key tags (a metadata + a later literal) → the FIRST wins (the metadata spec survives; the literal drops).

  Run `go test ./internal/tracing/ -run 'Metadata' -count=1`. **Expected: FAIL** — `kindMetadata`/`MetaNamespace` undefined (compile error). Record the verbatim red.

- [ ] **Step 2 — add the const + the struct fields + the `metadatav3` import.** Add `metadatav3 "github.com/envoyproxy/go-control-plane/envoy/type/metadata/v3"` to the import block (beside `tracingv3` `:12`). Add `kindMetadata` to the iota block (`:52-56`, appends at 3; extend the `:65` `Kind` comment to name it). Add `MetaNamespace string` + `MetaPath []string` to `CustomTagSpec` (`:63-71`); widen the `:70` `HasDefault` comment to name `kindMetadata` alongside `kindRequestHeader`.

- [ ] **Step 3 — replace the `:214-215` metadata reject** with the accept+4-reject arm (§1.1 skeleton). **The four kind-getters (`GetRequest`/`GetRoute`/`GetCluster`/`GetHost`) are on `*metadatav3.MetadataKind`, so bind `k := md.GetKind()` and branch on `k.GetRequest()` etc — `md.GetRequest()` does NOT compile (V1 SEVERE, §1.1).** Keep the empty-tag reject (`:190-192`) and first-wins dedup (`:219-223`) UNCHANGED.

- [ ] **Step 4 — run the tests.** `go test ./internal/tracing/ -count=1`. **Expected: PASS** (the five new tests green; every pre-existing tracing test green — the literal/request_header/environment arms untouched).

- [ ] **Step 5 — breaks (AFTER committing).**
  - **Break A [accept→reject]:** make the `k.GetRequest()!=nil` arm `return nil, fmt.Errorf(...)` instead of building the spec → test 1 FIRES (accept expected, got error). `git restore`; re-green.
  - **Break B [kind confusion]:** swap the ROUTE and REQUEST arms (accept ROUTE, reject REQUEST) → test 2's ROUTE subtest passes-when-it-should-fail AND test 1 fails → confirm WHICH. `git restore`; re-green. `[NOT pre-compiled — substitution rule applies]`.
  - **Break C [HasDefault]:** set `HasDefault: true` unconditionally → test 4 FIRES (`HasDefault==false` expected). `git restore`; re-green.

- [ ] **Step 6 — hygiene + commit.** `gofmt -l internal/tracing` silent · `go vet ./internal/tracing/` · `golangci-lint run ./internal/tracing/`.

**Commit:** `tracing(phase 70 T1): custom_tags metadata parse — kindMetadata + CustomTagSpec MetaNamespace/MetaPath; the REQUEST accept + four distinct-substring rejects (ROUTE/CLUSTER/HOST/unset-kind envoy-go-strict DEPARTURE; empty namespace/path/segment PGV-PARITY) replacing the :214-215 wholesale reject; HasDefault = DefaultValue != "" (the request_header idiom)`

---

## Task 2 — `internal/tracing/resolve.go`: the `metaLookup` param + the `kindMetadata` arm + `descend` + `structpbValueToString`

**Files:**
- Modify: `internal/tracing/resolve.go` (the `ResolveCustomTags` signature `:14`; a new `kindMetadata` `case`; the `descend`/`structpbValueToString` helpers; imports)
- Test: `internal/tracing/resolve_test.go`

**Interfaces:**
- Produces: `ResolveCustomTags(specs, headerLookup, metaLookup)` (the new nil-tolerant 3rd param `metaLookup func(ns, key string) (*structpb.Value, bool)`); the `descend`/`structpbValueToString` package-locals.
- Consumes: `structpb.Value` (`GetKind`/`GetStructValue`/`GetFields`/`GetStringValue`), `protojson.Marshal`, `json.Compact`.
- Reuses UNTOUCHED: the `kindLiteral`/`kindRequestHeader`/`kindEnvironment` arms, `KV`.

**Entry state:** T1 landed; `go test ./internal/tracing/ -count=1` green.

**Design (RE-DERIVED; §1.1 skeletons):** the `kindMetadata` arm mirrors `kindRequestHeader` (`:23-35`) for the default rule (present-empty EMITS `""`, C2), NOT `kindEnvironment` (`:36-52`). `descend` clones the ratelimit `descendStructpbValue` descent (`descriptors.go:1116`) as a `[]string` variant; `structpbValueToString` special-cases the string terminal and serializes non-string via `protojson.Marshal`+`json.Compact` (RD-SERIALIZE — `json.Compact` load-bearing for the list case, V1-executed).

- [ ] **Step 1 — write the failing tests (red-first).** In `resolve_test.go`, the existing `ResolveCustomTags(specs, lookup)` / `(specs, nil)` calls (`:30/:68/:93/:101/:120/:127/:134/:142`) each gain a trailing `nil` (3rd arg — no metadata tags). Add metadata tests driving a fake `metaLookup func(ns,key)(*structpb.Value,bool)`:
  1. **path-walk single** — `MetaPath:["k"]`, `metaLookup("ns","k")→(strVal("v"),true)` → `KV{Key,"v"}`.
  2. **path-walk multi** — `MetaPath:["k","a","b"]`, `metaLookup("ns","k")→(structVal{a:{b:"deep"}},true)` → `KV{Key,"deep"}`; and an unresolvable middle segment → falls to default/omit.
  3. **the P3 serialization table** — string→raw (`"x"`, no quotes); number→`42`/`3.14`; bool→`true`/`false`; struct→`{"a":"b"}`; list→`["x","y","z"]` (each `json.Compact`-compared — assert the EXACT compact string, `reference_detrand_race_catches_protojson_value_substring`); `NullValue`→unresolvable (default/omit).
  4. **the P4 matrix** — present-non-empty→emit; present-EMPTY (`strVal("")`)→emit `""` (NOT default); absent + `HasDefault`→default; absent + no default→omit.
  5. **nil-metaLookup tolerance** — a `kindMetadata` spec with `metaLookup==nil` → falls to default/omit, no panic.

  Run `go test ./internal/tracing/ -run 'Resolve' -count=1`. **Expected: FAIL** — `ResolveCustomTags` arity / `structpbValueToString` undefined. Record the verbatim red.

- [ ] **Step 2 — write the arm + helpers.** Add the `metaLookup` param (`:14`), the `kindMetadata` `case`, `descend`, and `structpbValueToString` (§1.1). Add the `structpb`/`protojson`/`encoding/json`/`bytes` imports (re-derive the exact spellings; RD-MOD — all existing-module). Confirm `MetaPath[0]` is safe (`min_items 1` validated at parse — T1).

- [ ] **Step 3 — run the tests.** `go test ./internal/tracing/ -count=1`. **Expected: PASS**.

- [ ] **Step 4 — breaks (AFTER committing).**
  - **Break D [default rule — clone the wrong arm]:** make the present-empty case OMIT (the `kindEnvironment` rule) instead of emitting `""` → test 4's present-empty assertion FIRES. `git restore`; re-green. (Pins C2 — the `request_header` rule.)
  - **Break E [json.Compact dropped]:** emit `protojson.Marshal`'s output WITHOUT `json.Compact` → test 3's list/struct assertion FIRES (the detrand whitespace `["x", "y", "z"]` ≠ `["x","y","z"]`). `git restore`; re-green. (Pins RD-SERIALIZE — `json.Compact` load-bearing.)
  - **Break F [string quoting]:** serialize the string terminal via `protojson.Marshal` (yielding `"x"` WITH quotes) instead of `GetStringValue()` → test 3's string assertion FIRES. `git restore`; re-green.
  - **Break G [descend nil-guard]:** drop the `GetStructValue()==nil` guard in `descend` → a non-struct intermediate would panic; test 2's unresolvable-middle assertion FIRES (or panics — substitute a compiling equivalent that returns the wrong value). `git restore`; re-green. `[NOT pre-compiled — substitution rule applies]`.

- [ ] **Step 5 — `-race`.** `go test ./internal/tracing/ -race -count=1` (the protojson value-substring path; `reference_detrand_race_catches_protojson_value_substring`).

- [ ] **Step 6 — hygiene + commit.** `gofmt -l internal/tracing` silent · `go vet` · `golangci-lint run ./internal/tracing/`.

**Commit:** `tracing(phase 70 T2): custom_tags metadata resolve — ResolveCustomTags grows a nil-tolerant metaLookup param + a kindMetadata arm (the request_header default rule: present-empty EMITS ""); descend clones the ratelimit descendStructpbValue []string-variant path-walk (NOT imported — internal/tracing stays filter-free); structpbValueToString (string→raw / NullValue→omit / else protojson.Marshal+json.Compact, the P3 table)`

---

## Task 3 — `internal/filter/http/chain.go`: the `*FilterChain.DynamicMetadata()` accessor (C1)

**Files:**
- Modify: `internal/filter/http/chain.go` (add the getter beside `SetDynamicMetadata` `:1014`)
- Test: `internal/filter/http/chain_test.go`

**Interfaces:**
- Produces: `func (c *FilterChain) DynamicMetadata() *dynamicmetadata.Bucket`. Consumed by `accesslog_emit.go` callers (T4).
- Consumes: the existing `c.dynamicMetadata` field (`:274`).

**Entry state:** T1–T2 landed; `go test ./internal/filter/http/ -count=1` green.

- [ ] **Step 1 — write the failing test (red-first).** In `chain_test.go`, add `TestFilterChain_DynamicMetadata`: a fresh `FilterChain` (via the constructor used elsewhere in `chain_test.go` — grep for `NewFilterChain`), `SetDynamicMetadata(b)` a sentinel bucket, assert `chain.DynamicMetadata() == b`; and a nil-tolerance check (`chain.DynamicMetadata().Get("x","y")` on a chain whose bucket is nil returns `(nil,false)` without panic — proving the RD-BUCKET nil-receiver safety at the call shape T4 uses). Run `go test ./internal/filter/http/ -run 'TestFilterChain_DynamicMetadata' -count=1`. **Expected: FAIL** — `DynamicMetadata` undefined. Record the verbatim red.

- [ ] **Step 2 — add the accessor** (§1.1) beside `SetDynamicMetadata` (`:1014`). No new import (`dynamicmetadata` already imported `:19`).

- [ ] **Step 3 — run the tests.** `go test ./internal/filter/http/ -count=1`. **Expected: PASS**.

- [ ] **Step 4 — break (AFTER committing).** **Break H [wrong field]:** return a fresh `dynamicmetadata.NewBucket()` instead of `c.dynamicMetadata` → the `== b` identity assertion FIRES. `git restore`; re-green. `[NOT pre-compiled — substitution rule applies]`.

- [ ] **Step 5 — hygiene + commit.** `gofmt -l internal/filter/http` silent · `go vet ./internal/filter/http/` · `golangci-lint run ./internal/filter/http/`.

**Commit:** `filter/http(phase 70 T3): add func (c *FilterChain) DynamicMetadata() — the C1 accessor (the :837/:963 accessors are on decoderCB/encoderCB, not the chain); mirrors them, nil-tolerant via Bucket.Get, so the HCM tracing emit sites can resolve a REQUEST-kind metadata custom_tag`

---

## Task 4 — `internal/filter/hcm`: thread `metaLookup` at the 3 emit methods + all 18 callers

**Files:**
- Modify: `internal/filter/hcm/accesslog_emit.go` (the 3 methods `:25/:85/:147` → 3rd `ResolveCustomTags` arg `:55/:116/:177`; `structpb` import) · `connection.go` (5 callers) · `h2dispatch.go` (6 callers) · `h3dispatch.go` (7 callers)
- Test: `accesslog_emit_test.go` + `span_emit_test.go` (caller signatures += trailing `nil`)

**Interfaces:**
- Produces: the 3 emit methods each grow a `metaLookup func(ns, key string) (*structpb.Value, bool)` param, passed straight to `ResolveCustomTags`.
- Consumes: `chain.DynamicMetadata().Get` (T3) at the 15 chain-in-scope callers; `nil` at the 3 no-chain-404 sites.

**Entry state:** T1–T3 landed; `go test ./internal/filter/hcm/ -count=1` green.

**RE-DERIVED caller map (RD-CALLERS — chain var name `chain` uniform; the 3 nil sites are BEFORE the chain is built):**

| File | nil site (no-chain 404) | `chain.DynamicMetadata().Get` sites |
|---|---|---|
| `connection.go` | :330 | :464, :597, :699, :777 |
| `h2dispatch.go` | :313 (`c.traceDecision` nil) | :396, :530, :577, :584, :613 |
| `h3dispatch.go` | :130 | :210, :280, :341, :367, :373, :395 |

*(H3:210 has `chain` in scope but `traceDecision` still nil — pass `chain.DynamicMetadata().Get`; valid-but-never-reached, NOT a nil site. Re-derive each caller's exact `chain` identifier at the IMPL tip; the map is line-anchored but a variable rename would surface as a build error.)*

- [ ] **Step 1 — write/adjust the failing tests (red-first).** The existing emit-method callers in `accesslog_emit_test.go` (`:29/:57/:68/:78/:94/:104/:132/:149/:167/:182/:203/:219/:233`) and `span_emit_test.go` (grep) pass `...nil, nil` (last two args); each gains a trailing `nil` for `metaLookup` (no metadata tags → byte-stable behavior). Add ONE new test proving the thread is live: a `Filter` configured with a `kindMetadata` custom_tag + a fake chain whose bucket resolves the value, driven through `emitAccessLog` with a sampling `traceDecision`, asserts the span carries the resolved `{tag,value}` attribute (grep `span_emit_test.go` for the existing custom_tag span-attr assertion pattern to clone). Run `go test ./internal/filter/hcm/ -count=1`. **Expected: FAIL** — arity mismatch (the methods don't take `metaLookup` yet). Record the verbatim red.

- [ ] **Step 2 — thread the param.** Add `metaLookup func(ns, key string) (*structpb.Value, bool)` to `emitAccessLog`/`H2`/`H3` (`:25/:85/:147`), pass it as the 3rd `ResolveCustomTags` arg (`:55/:116/:177`). Add the `structpb` import to `accesslog_emit.go` (`:3-14`).

- [ ] **Step 3 — update the 18 callers** per the RD-CALLERS map: the 3 nil sites pass `nil`; the 15 chain-in-scope sites pass `chain.DynamicMetadata().Get` (a method value — nil-safe via RD-BUCKET). Re-derive the exact `chain` identifier per caller at the tip.

- [ ] **Step 4 — run the tests.** `go test ./internal/filter/hcm/ -count=1`. **Expected: PASS** (the new metadata-span test green; all pre-existing emit tests green with the trailing `nil`).

- [ ] **Step 5 — breaks (AFTER committing).**
  - **Break I [nil at a span site]:** pass `nil` for `metaLookup` at a chain-in-scope caller (e.g. `connection.go:777`) → the new metadata-span test's resolved-value assertion FIRES (the tag falls to default/omit). `git restore`; re-green. `[NOT pre-compiled — substitution rule applies]`.
  - **Break J [wrong lookup]:** pass `headerLookup`-style args swapped → build error or the wrong value; substitute a compiling equivalent (e.g. a `metaLookup` that always returns `(nil,false)`) → the resolved-value assertion FIRES. Record. `git restore`; re-green.

- [ ] **Step 6 — hygiene + `-race` + commit.** `gofmt -l internal/filter/hcm` silent · `go vet` · `golangci-lint run ./internal/filter/hcm/` · `go test ./internal/filter/hcm/ -race -count=1` (the dispatch goroutines).

**Commit:** `hcm(phase 70 T4): thread metaLookup at the 3 emit methods + all 18 callers — chain.DynamicMetadata().Get at the 15 span-capable sites, nil at the 3 no-chain-404 sites (provably never reached — the traceDecision gate); accesslog_emit.go gains the structpb param import; the REQUEST-kind metadata custom_tag now resolves onto the ingress span`

---

## Task 5 — fuzz: the `withMetaTags` seed into `FuzzHCMConfigParse` (+0 fuzzers)

**Files:**
- Modify: `internal/filter/hcm/fuzz_test.go` (the `withMetaTags` seed; `metadatav3` import)

**Entry state:** T1–T4 landed. Fuzzer `FuzzHCMConfigParse` at `:28`; seed helpers `withCustomTags`(:37)/`withReqHeaderTags`(:51)/`withEnvTags`(:66) (RD-FUZZ).

- [ ] **Step 1 — add the seed.** Clone the `withEnvTags` shape (`:66-75`, §1.1): a `mkHCM` with a tracing block carrying an ACCEPTED REQUEST metadata tag + a REJECTED ROUTE-kind tag; `f.Add(withMetaTags.GetTypeUrl(), withMetaTags.GetValue())`. Add `metadatav3` import. Re-derive the exact `metadatav3.MetadataKind_Request_`/`_Route_`/`MetadataKey_PathSegment_Key` wrapper spellings at the tip.

- [ ] **Step 2 — dispatch-verify (the named trap — RD-FUZZ / SPEC §7).** Confirm the seed REACHES the `parseCustomTags` metadata arm (`config.go:122` precedes the provider check `:130` — a well-formed HCM tracing block dispatches into `parseCustomTags` before `provider required`). As a one-off scratch check (NOT committed): temporarily `t.Logf`/print which arm the seed reaches, run `go test ./internal/filter/hcm/ -run FuzzHCMConfigParse -count=1 -v`, confirm it hits the metadata arm (accept for `meta_ok`, reject-substring for `meta_bad`) — NOT an earlier typeURL/provider reject. Restore. Record in PROGRESS.

- [ ] **Step 3 — reconcile the count.** `grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l` → **55** BEFORE and AFTER (`reference_fuzzer_count_docs_drift` — a seed is +0 `func Fuzz`). A short active-fuzz smoke (`go test -run FuzzHCMConfigParse -fuzz FuzzHCMConfigParse -fuzztime 10s ./internal/filter/hcm/`) — no panic; NO corpus artifacts committed.

- [ ] **Step 4 — hygiene + commit.** `gofmt -l internal/filter/hcm` silent · `go vet` · `golangci-lint run ./internal/filter/hcm/`.

**Commit:** `hcm(phase 70 T5): fuzz — a withMetaTags REQUEST-accept + ROUTE-reject seed into FuzzHCMConfigParse, dispatch-verified to reach the parseCustomTags metadata arm (config.go:122 precedes the provider check :130); +0 fuzzers, 55→55`

---

## Task 6 — fixture `0114-tracing-custom-tags-metadata` (OTLP) + breaks (fixtures 115 → 116)

**Files:**
- Create: `test/fixtures/0114-tracing-custom-tags-metadata/{driver/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md, scripts/<name>.lua}`

**Entry state:** T1–T5 landed. Next fixture `0114`; no port to pick (RD-PORTS).

**Design (SPEC §8 — ONE fixture, TWO custom_tags in one config; RD-LUA — the CHASSIS clones from `0106`, the Lua writer from `0027`):**
- Both YAMLs: the `0106-tracing-custom-tags-environment` chassis (H1 listener → HTTPFixedBody backend, HCM OTLP tracing provider → the driver-owned `test/helpers/otlptrace.Server` receiver, `random_sampling:100`, all ports templated). ADD a Lua `http_filter` BEFORE the router on BOTH `envoy.yaml` + `envoy-go.yaml` (the `0027-http-lua-full-bridge` stanza: `- name: envoy.filters.http.lua` / `"@type": …lua.v3.Lua`, `inline_string`/`source_code` a script that calls `handle:streamInfo():dynamicMetadata():set("<ns>","<key>","<fixed-string>")` — a FIXED cross-side-identical value).
- TWO `metadata` custom_tags in the HCM tracing block:
  - **`meta_hit`** — `metadata:{kind:{request:{}}, metadata_key:{key:"<ns>", path:[{key:"<key>"}]}, default_value:"<unused>"}` → resolves to the Lua-set fixed string (P1).
  - **`meta_default`** — `metadata:{kind:{request:{}}, metadata_key:{key:"<ns>", path:[{key:"<unset-key>"}]}, default_value:"<fallback>"}` → the path is unset → emits the `default_value` (P4-i).
- **Cross-side EXACT value equality** (STRONGER than 0106's key-only): both sides run the SAME Lua → the SAME `Bucket` write → the SAME resolved string. The driver asserts on EVERY span BOTH sides: `meta_hit == "<fixed>"` AND `meta_default == "<fallback>"` (key AND value), plus the `0087` baseline (span count > 0). Each an independent `Errorf`. `telemetry.sdk.*` resource + scope UNasserted cross-side (impl-specific, the 0106 precedent). Fixtures **115 → 116**.

- [ ] **Step 1–3 — write driver/YAMLs/expectations/Lua** (clone `0106` chassis; add the `0027` Lua stanza + a `scripts/*.lua` writer; add the two custom_tags; the cross-side EXACT key+value assertions; the span-count baseline). Grep the driver-owned `test/helpers/otlptrace` API for the span-attr accessor to read `meta_hit`/`meta_default` per span.
- [ ] **Step 4 — run.** `go test ./test/differential/ -run 'TestDifferential/0114-tracing-custom-tags-metadata' -count=1`. **Expected: PASS**; fixture count 116. Confirm the WRITER served-this-arm (the span carries the Lua value, not a vacuous default — the `meta_hit` value-equality assertion is the proof).
- [ ] **Step 5 — hygiene + commit.**

- [ ] **Step 6 — breaks (AFTER committing; `-count=1`, full selector, confirm WHICH fired).**
  - **Break K [Lua value swap]:** change the Lua-set value on ONE side → the `meta_hit` value-equality assertion FIRES (the two sides now differ). `git restore`; re-green.
  - **Break L [default drop]:** empty `meta_default`'s `default_value` on both sides → the tag OMITS → the `meta_default` presence assertion FIRES. `git restore`; re-green.
  - **Break M [wrong-namespace control]:** point `meta_hit`'s `metadata_key.key` at a namespace the Lua never wrote (both sides) → `meta_hit` falls to its (unused) default → the `meta_hit == "<fixed>"` assertion FIRES (proving the writer/namespace binding is load-bearing, not a vacuous default match). `git restore`; re-green. `[NOT pre-compiled — substitution rule applies]`.

**Commit:** `differential(phase 70 T6): fixture 0114-tracing-custom-tags-metadata — a Lua dynamicMetadata():set writer (cloned from 0027) on the 0106 OTLP chassis + two REQUEST metadata custom_tags (meta_hit→the Lua value, meta_default→default_value) asserted cross-side EXACT key+value (STRONGER than 0106 key-only); breaks K/L/M (fixtures 115→116, all ports templated)`

---

## Task 7 — BEHAVIOR_CONTRACT delta (B1–B2) — pinned VERBATIM

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`

**Entry state:** T1–T6 landed. Docs-only. Anchor by SYMBOL / first-clause (SPEC §9 lines are as of `9338507a`; re-locate the tracing `custom_tags` subsection at the IMPL tip — where `literal`/`request_header`/`environment` are described and `metadata` is currently "unsupported (rejected)").

- [ ] **B1 — flip the `metadata` line** (SPEC §9 B1): from "`metadata` — unsupported (rejected)" to the CONSUMED-for-REQUEST description — the named dynamic-metadata value (`metadata_key.key` namespace + `path` walk) resolved out of the per-request `Bucket`, serialized to a STRING (string→raw; number/bool→scalar; struct/list→compact JSON — the `google.protobuf.Value` rendering), emitted as a `{tag, value}` span attribute on BOTH exporters (appended after the built-ins on OTLP; in the `tags` map on Zipkin); `default_value` emits when the path is absent/unresolvable and the default is non-empty; a present-empty value emits `""`; an unresolvable-with-empty-default omits (the `request_header` rule). The `ROUTE`/`CLUSTER`/`HOST`/unset-kind arms reject loudly (envoy-go-strict DEPARTURE — the reference boots them); the empty-namespace/empty-path/empty-segment rejects are PGV-PARITY. This completes the four `custom_tags` SOURCE types.

- [ ] **B2 — the departures list** (SPEC §9 B2): add the two categories — (departure) non-`REQUEST` MetadataKinds + unset-kind reject where the reference boots; (documented boundary) non-string value serialization beyond the P3-pinned common types (number-precision edges + `NullValue`).

- [ ] **B3 — NO edit** at the 4-empty-built-in-span-attrs neighborhood (`reference_tracing_upstream_cluster_framework_gap` — a REQUEST-kind metadata tag is populated from the SEPARATE landed per-request `Bucket`; do NOT conflate — verify byte-unchanged).

- [ ] **Verify UNCHANGED:** the `literal`/`request_header`/`environment` lines + `max_path_tag_length` + every other tracing bullet apart from the B1 metadata line + the B2 departures addition.

**Commit:** `docs(phase 70 T7): BEHAVIOR_CONTRACT B1–B2 — flip the custom_tags metadata line to CONSUMED-for-REQUEST (Bucket path-walk → serialized string → {tag,value} on both exporters; the request_header default rule); add the departures (non-REQUEST kinds reject where the reference boots; non-string serialization boundary). Completes the four custom_tags source types`

---

## Task 8 — VERIFY: the six-gate + cycle guard + full differential + `-race` + counts + envelope audit

Controller-run on the frozen pre-stage-close HEAD:

- [ ] 1. `gofmt -l internal/ test/ cmd/` — SILENT
- [ ] 2. `go vet ./...` — exit 0
- [ ] 3. `go build ./...` — exit 0
- [ ] 4. `go mod tidy -diff` EMPTY + `git diff --exit-code master -- go.mod go.sum` EMPTY (**+0 modules** — RD-MOD; `structpb`/`type/metadata/v3`/`protojson` already in `internal/tracing`'s dep closure)
- [ ] 5. `golangci-lint run ./...` — exit 0
- [ ] 6. **FULL differential:** `go test ./test/differential/ -count=1` — all **116** dirs, exit 0. The 115 pre-existing dirs byte-stable. `reference_differential_fullsuite_startup_flake`: a `subject ready: EOF` on an UNRELATED fixture is a startup race — isolate-re-run; `reference_0061_ring_hash_spread_flake` on a second occurrence → investigate margins.

**Plus:**
- [ ] **Cycle guard:** `go list -deps ./internal/tracing | grep 'envoy-go/internal'` (**no `...`**) ⇒ NO `internal/filter` edge (the `descend` clone keeps `internal/tracing` filter-free — RD-DESCEND; `reference_xds_config_seam_transitive_cycle_guard`, TYPE-level).
- [ ] **`-race` on touched packages:** `go test ./internal/tracing/ ./internal/filter/http/ ./internal/filter/hcm/ -race -count=1` (the protojson value path + the dispatch goroutines; `reference_detrand_race_catches_protojson_value_substring`, `reference_full_suite_race_after_background_mutator`).
- [ ] **Counts MECHANICAL, never copied:** fixtures **116** (tail `0114-tracing-custom-tags-metadata`) · fuzzers **55** (`^func Fuzz`) · BackendKind **38** · DECISIONS tail **ADR-0292** · stat surface **1201** (+0 — a span attribute registers no stat; there is NO mechanical stat command) · go.mod diff EMPTY.
- [ ] **Envelope audit:** `git diff master --stat` shows functional production = `internal/tracing/{config.go,resolve.go}` + `internal/filter/http/chain.go` + `internal/filter/hcm/{accesslog_emit.go,connection.go,h2dispatch.go,h3dispatch.go}` ONLY; **`internal/xds`/`internal/tls`/`internal/boot`/`internal/listener`/`internal/bootstrap`/`validate/` ABSENT**; `internal/dynamicmetadata`/`internal/filter/http/ratelimit`/`internal/filter/http/lua` BYTE-UNTOUCHED (consumed/cloned/writer, not edited). **New exported symbols ONLY** in `internal/tracing` (the `ResolveCustomTags` growth, `CustomTagSpec` fields, `kindMetadata`, `descend`/`structpbValueToString`) + `internal/filter/http` (`DynamicMetadata`); ZERO new packages/modules/stats/BackendKinds; the `internal/xds` zero-new-symbol discipline UNTOUCHED.

*(No separate commit — T8's evidence lands in PROGRESS at T9.)*

---

## Task 9 — ADR-0292 completed IN PLACE + stage-close (controller-adjacent)

- [ ] **ADR-0292: COMPLETE IN PLACE** — append §Decision + §Consequences to the EXISTING entry (the §Context landed at the SPEC squash, STATUS: PROPOSED). Flip the STATUS banner to **COMPLETE**. **Do NOT append a new ADR; do NOT renumber.** Tail stays ADR-0292; next-free ADR-0293 (`grep -c '^## ADR-0293'` → 0). §Decision records the landed mechanism (the parse accept+4-reject arm + `CustomTagSpec` fields + `kindMetadata`; the `metaLookup`/`kindMetadata`/`descend`/`structpbValueToString` resolve arm; the `*FilterChain.DynamicMetadata()` accessor + the 18-caller thread; the `0114` fixture); §Consequences records the counts, the named departures (non-REQUEST-kind reject where the reference boots; the non-string serialization boundary), and the memory updates.
- [ ] **ROADMAP row 70 → `done`** at the six-gate (ADR-0106, SOLE leg; `reference_roadmap_split_phase_row_done`). **NARROW the deferred sentence NOW (and ONLY now):** roll `custom_tags (metadata)` OUT of the live Observability `candidates:` sentence (the phase-57 graphite precedent — SPEC §12; the sentence STAYS a live `candidates:` match afterward — the `ssl` family + `spawn_upstream_span`/`http_service`/force-trace remain, and the three non-`REQUEST` MetadataKinds become a NEW deferred entry). Keep EXACTLY ONE live Observability `candidates:` match; HTTP/3 + xDS untouched (three total).
- [ ] **STATE.md:** edit §Current pointer IN PLACE; demote to §Recent lineage capped at five; update counts (fixtures 116, DECISIONS ADR-0292 COMPLETE).
- [ ] **PROGRESS.md:** finalize — every break's ACTUAL firing assertion, the verbatim red-first records, the T5 dispatch-verify trap result, any break substitutions.
- [ ] **Router roll** (`next-prompt.txt` — TRACKED despite .gitignore; edit in the stage worktree; locate by SUBJECT). Row 70 done ⇒ the sentinel's check (1) goes SILENT for row 70 ⇒ the roller SELF-PICKS the next subject at the phase-71 BRAINSTORM (the 2026-07-12 standing directive) unless the sentinel fires (it does not: checks (2)+(3) still print).
- [ ] **Sentinel re-run MECHANICALLY:** check (1) goes silent when row 70 flips (every OTHER chartered row already `done`); (2) still prints 3 via the full-phrase command (`grep -cE 'remaining deferred \(not-yet-chartered\) candidates:'` → 3 — the metadata narrowing does NOT drop the whole Observability sentence); (3) unchanged (`NEVER OPENED: gRPC/Runtime/WASM`) ⇒ does NOT fire; no `stop` file.
- [ ] **Memory updates owed (SPEC §13):** (i) the `*FilterChain.DynamicMetadata()` gap — the `DynamicMetadata()` accessor is on the CALLBACK types (`decoderCB`/`encoderCB`), NOT on `*FilterChain`; a consumer at the HCM dispatch layer must ADD one (the BRAINSTORM mis-cited it as existing — `feedback_brief_citations_not_evidence`). (ii) the tracing `metadata` value→string serialization needs `json.Compact` after `protojson.Marshal` to strip the detrand whitespace and match the reference's compact form (`reference_detrand_race_catches_protojson_value_substring` extended to a value-EMISSION site). (iii) OPTIONAL: a "custom_tags source-type family complete" note (the four `CustomTag` source types now all consumed) if useful; else skip.
- [ ] **Squash-push by the controller** at stage-close.

**Commit (stage-close docs):** `phase 70 (tracing-custom-tags-metadata) IMPL: …` (controller composes at close).

---

## Self-review against SPEC-70

| SPEC obligation | Where |
|---|---|
| the `kindMetadata` const + `CustomTagSpec` `MetaNamespace`/`MetaPath` (§3.2) | T1 |
| the REQUEST accept + four distinct-substring rejects (ROUTE/CLUSTER/HOST/unset-kind DEPARTURE; empty ns/path/segment PARITY) (§3.2/§3.6) | T1 |
| the `metaLookup` param + `kindMetadata` resolve arm, the `request_header` default rule (present-empty EMITS "") (§3.3/§3.4, C2) | T2 |
| the `descend` clone (NOT imported — filter-free) (§3.3, C3, RD-DESCEND) | T2, T8 (cycle guard) |
| `structpbValueToString` — string→raw / non-string→protojson+json.Compact (§3.3, C4, RD-SERIALIZE) | T2 (tests 3, Breaks E/F) |
| the NEW `*FilterChain.DynamicMetadata()` accessor (§3.5, C1, RD-ACCESSOR) | T3 |
| the `metaLookup` thread at all 18 callers — `nil` at the 3 no-chain sites, `chain.DynamicMetadata().Get` at 15 (§3.5, RD-CALLERS) | T4 |
| the fuzz seed + dispatch-verify (§7, RD-FUZZ) | T5 |
| ONE OTLP fixture `0114`, two custom_tags, cross-side EXACT key+value (§8, RD-LUA/RD-PORTS) | T6 |
| BC B1–B2 pinned wording (§9) | T7 |
| a SINGLE FLAT ROW; ADR-0045 valve armable-but-unconsumed (§10) | §1, this table |
| six-gate + cycle guard + full-116-dir + -race + counts + envelope audit (§10 T8, §15) | T8 |
| +0 packages / +0 modules / +0 stats / +0 fuzzers / +0 BackendKinds (§4, §7) | T5, T8 |
| ADR-0292 completed IN PLACE, no new ADR (§14) | T9 |
| Sentinel: narrow the sentence AT THE IMPL row-done, not before (§12) | T9 |
| Memory updates (§13) | T9 |

**Task count: 9** — matching the SPEC's ~10 anticipation (comfortably under the ADR-0045 ~15 ceiling). **ADR-0045 escape valve ARMABLE, UNCONSUMED — no split**: the parse + resolve sit on ONE landed `internal/tracing` engine; the `Bucket` is a landed read-only substrate; the accessor (C1) is one small method; no second subsystem can strand a leg (`internal/xds`/`internal/tls`/`internal/boot`/`internal/listener`/`internal/bootstrap` untouched). Sequencing: T1 (parse) → T2 (resolve, consumes T1's spec fields) → T3 (accessor, independent) → T4 (wires T2's param + T3's accessor at the callers) → T5 (fuzz) → T6 (fixture) → T7 (BC) → T8/T9 (close).

**⚠️ The IMPL's standing instruction: a PLAN is not evidence either.** **RE-DERIVE this document; do not execute it.** Where it cites, go look; where it claims control flow, walk the call graph; default to REFUTED. Start where this PLAN is most confident (all re-derived read-only at the PLAN, §1): the ZERO-drift anchor set (RD-EXACT), the three clone skeletons (RD-DESCEND / RD-DEFAULT / RD-FUZZ, §1.1 verbatim), the C1 accessor absence (RD-ACCESSOR), and the 18-caller map with exactly 3 nil sites (RD-CALLERS).
