# Phase 66 Brainstorm — `xds-sds-combined-validation-context` (the FOURTH xDS-family row; the natural follow-on phase 65 itself named — an inline `default_validation_context` MERGED with the SDS-delivered `CertificateValidationContext` phase 65 landed, installed as the downstream mTLS `ClientCAs` — lifts the `commonTLSContextToConfig` `combined_validation_context is not supported in phase 03` reject (DOWNSTREAM + live-provider + `require_client_certificate: true` ONLY; upstream + QUIC keep the byte-identical substring, ADR-0080); anticipated ZERO new symbols in `internal/xds` / +0 packages / +0 modules / +0 stats; anticipated ONE new fixture)

> **Stage:** BRAINSTORM (lifecycle-state 0 → 1). Docs-only — ZERO production `.go`. Fresh worktree off master `d545810a`, branch `phase-66-xds-sds-combined-validation-context`, worktree `.worktrees/phase-66-brainstorm`, per `feedback_git_worktrees`.
>
> **Loop re-open (AUTONOMOUS — no human pick):** the termination sentinel was re-evaluated MECHANICALLY against master `d545810a` and does NOT fire — check (2) prints THREE live "candidates:" sentences (HTTP/3 `:175`, xDS `:185`, Observability `:193`) and check (3) prints `NEVER OPENED: gRPC`, `Runtime`, `WASM`. No `stop` file. Check (1) is silent (every chartered row is `done`), so there is **NO banked mid-lifecycle work** and the directive's "advance banked work first" clause does not apply — this is a brand-new subject. The roller **SELF-PICKED** per the 2026-07-12 standing directive (§2.1), and **OVERTURNED the router's provisional pick on evidence** (§2.1.1 — the correct outcome per the router's own instruction, not a deviation).
>
> **This document was ADVERSARIALLY VERIFIED before landing** (§11). An independent re-derivation pass found **ELEVEN defects in this BRAINSTORM's own first draft** — including a `proto.Merge` semantics error that inverted §2.5's central reasoning, a silently-truncated "verbatim" proto quote that had manufactured a false central risk, and a DROPPED phase-65 §8 carry-forward. All are corrected below and recorded in §11 as the lesson. **A BRAINSTORM is not evidence either.**
>
> **Baselines re-verified against master tip `d545810a` this session (`git fetch` first; NOT copied from the router):** fixtures **110** (`ls -d test/fixtures/[0-9]*/ | wc -l`; numeric tail `0108-xds-sds-validation-context`) · fuzzers **55** (`grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l`) · DECISIONS tail **ADR-0286** (next-free **ADR-0287**) · BackendKind tail **38** (`H2GoawayResponder`) · stat surface **1201** (docs-verified; no mechanical count exists) · go.mod modules **2** (the phase-61.2 lineage figure: `quic-go v0.54.1` direct + `qpack v0.5.1` indirect — **NOT** a repo total; the single `go.mod` requires 67 modules).

---

## 1. Mission and scope confirmation (66 — a CONFIG-SHAPE row on a fully-landed applier, NOT a new resource type and NOT a new discovery machine)

### 1.1 What phase 66 delivers as a self-contained whole (an SDS-delivered CA merged onto an inline default)

Phase 65 landed the SDS-delivered downstream `validation_context`: a management server serves a `tls.v3.Secret` carrying a `CertificateValidationContext`, the phase-60.2 SotW stream fetches it, and `NewDownstreamConfig` installs the resulting `*x509.CertPool` as `cfg.ClientCAs` under `require_client_certificate: true`.

`combined_validation_context` is the sibling oneof arm that composes that SDS-delivered context with an **inline** `default_validation_context`. Its reject is the second arm of the switch in `commonTLSContextToConfig` (`internal/tls/config.go`, cited by SYMBOL per §11):

```go
case *tlsv3.CommonTlsContext_CombinedValidationContext:
    return nil, fmt.Errorf("tls: %s: combined_validation_context is not supported in phase 03", side)
```

The proto documents the merge on the oneof wrapper `CommonTlsContext_CombinedValidationContext` (go-control-plane/envoy v1.32.4, `extensions/transport_sockets/tls/v3/tls.pb.go`). **Quoted in FULL — the trailing sentence is load-bearing and was truncated in this document's first draft (§11):**

> Combines the default `CertificateValidationContext` with the SDS-provided dynamic context for certificate validation.
>
> When the SDS server returns a dynamic `CertificateValidationContext`, it is merged with the default context using `Message::MergeFrom()`. The merging rules are as follows:
>
> * **Singular Fields:** Dynamic fields override the default singular fields.
> * **Repeated Fields:** Dynamic repeated fields are concatenated with the default repeated fields.
> * **Boolean Fields:** Boolean fields are combined using a logical OR operation.
>
> **The resulting `CertificateValidationContext` is used to perform certificate validation.**

**The trailing sentence is singular — "The resulting `CertificateValidationContext`", ONE context** — which is direct documentary evidence that the reference builds ONE trust store from the merged message rather than unioning two. It substantially answers the question this document's first draft had elevated to "THE central probe."

**⚠️ The proto's three-bullet summary is INCOMPLETE for `trusted_ca`, and this is the row's real subtlety.** `go doc google.golang.org/protobuf/proto.Merge`, verbatim:

> Populated scalar fields in src are copied to dst, while **populated singular messages in src are merged into dst by recursively calling Merge**.

`trusted_ca` is **`*core.v3.DataSource` — a MESSAGE, not a scalar.** So it does **not** "singular-override": `proto.Merge` **recursively merges** it. Probed empirically this session:

```
default {filename:"/ca_y.pem", watched_directory:{path:"/watch"}}  ⊕  dynamic {inline_bytes:"CA_X"}
  ⇒  {inline_bytes:"CA_X", watched_directory:{path:"/watch"}}
```

The `specifier` **oneof** (`filename`/`inline_bytes`/`inline_string`/`environment_variable`) **does** replace — which is why the override *observable* still works. But **non-oneof sub-fields of the default's DataSource SURVIVE**, producing a **hybrid `DataSource` neither side authored**. `watched_directory` is inert in envoy-go and **live in the C++ reference (it drives rotation)** — a concrete cross-side divergence vector. **This, not the merge-vs-union question, is the row's sharpest unprobed risk** (D-COMBVC-HYBRID-DS, §2.4).

**And `proto.Merge` mutates `dst` in place** and does not copy it. Here `dst` would be `cvc.GetDefaultValidationContext()` — a pointer **into the parsed bootstrap**. **A `proto.Clone` guard is therefore MANDATORY** (D-COMBVC-CLONE, §2.4). The merge is still cheap, but "free" was an overclaim: the row owes a small hand-written wrapper, not zero code.

`CommonTlsContext_CombinedCertificateValidationContext` has four fields, but **only TWO are live**: `default_validation_context` and `validation_context_sds_secret_config`. The other two (`validation_context_certificate_provider`, `..._instance`) are BOTH proto-`[#not-implemented-hide:]` AND proto-deprecated — out of scope by construction, not by choice.

### 1.2 What phase 66 does NOT deliver (forward to §8)

Not upstream CVC (the arm is structurally dead — §2.7). Not QUIC CVC (nil provider ⇒ the reject stays, ADR-0080). **Not `require_client_certificate: false` (optional / verify-if-presented mTLS with a CVC trust anchor** — phase 66 scopes to MANDATORY mTLS, inheriting phase 65's scope; this is a **direct pickup of phase-65 §8's carried item** and it interacts with a real silent-ignore gap — §2.2/D-COMBVC-REQUIRE-FALSE). Not the compose-two edge (CVC still yields exactly ONE `SdsSecretConfig` ⇒ `seen == 1`; the `seen>1` guard is untouched — §2.6). Not SDS rotation. Not `crl` (a SHARED gap on BOTH paths — §2.9). Not the repeated-concatenate or bool-OR merge rules (structurally unreachable — §2.5, a NAMED COVERAGE BOUNDARY).

### 1.3 Phase-done as the FOURTH xDS-family row (family STAYS OPEN)

Row 66 → `done` at its IMPL six-gate (the SOLE leg — NO parent rollup, ADR-0106). The xDS family STAYS OPEN.

**The live xDS deferred sentence is UNCHANGED by this row** — `combined_validation_context` was never IN it (it is a §8-tier candidate from phase 65's deferred roster). This is precedented: phase 64's subject was likewise a §8-tier candidate, and its BRAINSTORM commit `7d724423` left the Observability sentence **byte-identical** (verified). So unlike phase 65, **this row narrows nothing at its IMPL** — a fact the SPEC and IMPL must not fabricate.

### 1.4 ADR-0045 split readiness — anticipated a SINGLE FLAT ROW (escape-valve armable) *(self-answered; SPEC confirms)*

Anticipated ~6–9 tasks across TWO files (`internal/tls/config.go`, `internal/boot/boot.go`) — materially smaller than phase 65's 11-task, four-new-symbol parallel chain. The escape-valve is armable but no split is anticipated: there is no two-package surface that could strand a leg (`internal/xds` is untouched).

### 1.5 Seed-stub alignment + package placement — ALL edits in EXISTING files/packages, ZERO new packages

- `internal/tls/config.go` — the switch arm, the sub-field reject re-point, the `NewDownstreamConfig` 3-way branch, the clone+merge. **EXISTING.**
- `internal/boot/boot.go` — the pre-scan third arm. **EXISTING.**
- `internal/xds` — **UNTOUCHED.** No new applier, no new provider method, no new stream code, no new type URL. CVC is a LISTENER-CONFIG shape, not a SERVED-SECRET shape: the wire is byte-identical to phase 65's (same `tls.v3.Secret`, same `validation_context` oneof arm, same type URL). **The management server cannot tell a CVC client from a plain-SDS client.**
- `test/helpers/sdsserver` — **UNTOUCHED** (`WithValidationContext` already serves exactly what CVC needs).

ZERO new packages. ZERO new modules.

### 1.6 No prebrainstorm-notes branch

None exists for this subject (`reference_phase_11_local_ratelimit_prebrainstorm_notes` is `local_ratelimit`-scoped and does not apply).

### 1.7 Phase 66's relationship to the existing seams (a config-shape lift on a proven applier)

Phase 60.1 built the SDS discovery substrate. Phase 60.2 wired the FIRST applier (server cert). Phase 65 proved the substrate carries a SECOND RESOURCE TYPE (validation context). **Phase 66 proves the landed applier COMPOSES with static config** — a fourth cheap increment on a substrate that keeps paying, and the first that adds ZERO new symbols to `internal/xds`.

**Contrast with phase 65:** phase 65 had to BUILD a parallel `*x509.CertPool` applier chain (four new symbols). Phase 66 builds NONE — it reuses `FetchInitialValidationContext` verbatim and spends its novelty budget on a config-shape branch plus a clone+merge.

---

## 2. Design decisions

### 2.1 Row + subject confirmation: `combined_validation_context` *(SELF-PICKED per the standing directive → row 66 registered)*

The FIRST decision, made AUTONOMOUSLY per the 2026-07-12 standing directive. Row 66 registers `in-progress` AT this BRAINSTORM commit per the ROADMAP §Schema invariant.

**Why CVC is the defensible pick:**
1. **Smallest production footprint of every candidate sized this session** — ~4 edit sites across 2 files, and **ZERO new symbols in `internal/xds`**. Strictly less than phase 65.
2. **The entire substrate is confirmed-landed and PROVEN** — `FetchInitialValidationContext`, `parseValidationSecret`, the SotW stream, `sdsserver.WithValidationContext`, and the `0108` fixture all exist and pass (each re-derived this session). Nothing is built; things are composed.
3. **Phase 65 itself named it the next step.** BRAINSTORM-65 §2.1 rejected it with exactly one clause — *"needs the merge semantics on top of the plain SDS path. Deferred (a natural follow-on ONCE plain SDS-`validation_context` lands)."* (verified verbatim). **That precondition is now satisfied** (row 65 `done`). Phase 65's own §2.1 criteria — unconditional provability, confirmed-landed substrate, bounded new production code — now all favor CVC.
4. **~95% fixture reuse.** `0108`'s driver already owns the in-memory 5-artifact PKI, the `mtlsEcho` full-round-trip probe, `structuralCheck`, `normalizeTLSErr`, per-side SDS servers, and YAML templating. A CVC fixture is a YAML delta plus a CA re-labelling.

**The genuine risks** are the hybrid-`DataSource` merge leak (D-COMBVC-HYBRID-DS) and the `require==false` silent-ignore (D-COMBVC-REQUIRE-FALSE) — both §2.4, both SPEC-probeable, neither a blocker.

#### 2.1.1 ⚠️ The router's PROVISIONAL pick (`xds-sds-upstream-server-cert`) was OVERTURNED on evidence

The router proposed upstream SDS server-cert and flagged boot-ordering as "a genuine open question — probe it." **Mechanical re-derivation this session shows the risk is categorically worse than the router framed it, and it is fatal to the "smallest" claim.** Recorded here in full so no future roller re-proposes it without reading this. *(Every sub-claim below was independently re-derived and CONFIRMED in the §11 adversarial pass.)*

- **It is a VALUE-LEVEL CONSTRUCTIBILITY CYCLE, not an ordering preference.** `grpcclient.New(mgr *cluster.Manager) *Dialer` takes the manager as a hard parameter. `main.go` runs `cm, err := cluster.NewManagerWithBaseDir(...)` → `dialer := grpcclient.New(cm)` → `boot.NewSDSProvider(dialer, ...)`, in that order. So `boot.NewSDSProvider` requires the `*cluster.Manager` **currently being constructed**, and the SDS stream is dialed *through* one of the very clusters `buildCluster` is building. At the moment `NewUpstreamConfig` runs, the provider **cannot exist** — enforced by a data dependency, not by statement order. `boot.Construct`'s own doc comment records the weaker form: *"cm must already exist before a `grpcclient.Dialer` … can be built."*
- **The import graph is FINE — and this REFINES a memory.** `internal/cluster` already reaches `internal/xds` transitively (`cluster → tls → xds`; verified with `go list -deps`), so an `xds.SecretProvider` parameter costs nothing in the import graph. `reference_xds_config_seam_transitive_cycle_guard` correctly names the *type*-level edge; **the upstream-SDS blocker is at the VALUE level, which that memory does not cover.** The router cited the memory as if it were the whole risk. It was not.
- **Two further blockers.** `boot.NewSDSProvider` pre-scans **listeners only** and never clusters. And `Cluster.upstreamCfg` is assigned eagerly (`cl.upstreamCfg = uc.TLSConfig`) then consumed at build time by `extractH2Mode(c, cl.upstreamCfg)` (which reads `NextProtos`) — so deferring it reshapes the cluster build.
- **Escaping this needs a boot-model reshape** (a two-pass cluster build, or lazy per-Dial TLS construction). That is a legitimate future row. It is not the *smallest* one, which is the directive's selection criterion.

**The router's own instruction was explicit — "If the feasibility check below does not hold up, PICK DIFFERENTLY and say why — that is the correct outcome, not a deviation."** This is that outcome. Upstream SDS carries forward to §8 with the mechanism recorded, so the next roller starts from evidence rather than re-deriving it.

**Rejected alternatives (recorded per the standing directive; each SIZED against source this session — §11):**

> **Note on deferred-sentence membership:** MOST candidates below sit in a live deferred sentence (`:175` HTTP/3, `:185` xDS, `:193` Observability) — narrowing one is therefore NOT a discriminator between them, and this document's first draft wrongly claimed it was (§11). The picked subject and DataSource `environment_variable` are the two exceptions: neither is in any sentence, so **the pick trades sentence-narrowing for size**, deliberately, per the smallest-first directive and the phase-64 precedent (§1.3).

- **`xds-sds-upstream-server-cert`** (the router's provisional pick; in the `:185` sentence) — REJECTED: a value-level constructibility cycle forces a boot-model reshape. Fully recorded §2.1.1. A real row; not a small one. **Deferred.**
- **DataSource `environment_variable`** (`loadDataSource` / `dataSourceBytes`; 2 edit sites; in NO sentence) — superficially the smallest thing in the repo, and 4 in-repo packages already honor the arm. **REJECTED on provability:** the arm that matters for TLS is *value*-carrying (the env var holds cert/key PEM), and value-equality across a subprocess subject and a Docker reference needs the **D-ENV-HARNESS** seam (`ContainerRequest.Env` + `cmd.Env` threaded per-fixture through both starters) that **SPEC-63 already re-derived and DECLINED** as "NOT one-line-per-starter" — after its own BRAINSTORM had wrongly anticipated it was one line per starter. Fixture `0106` escaped it only by asserting key-presence + value-non-empty on `PATH`; that cannot rescue a PEM (a non-empty `PATH` is not a valid certificate). It would also inherit an unadjudicated disagreement: the four landed precedents **split 2–2** on set-but-EMPTY (lua/wasm reject; jwtauthn/directresponse accept an empty payload), and the proto documents neither. Harness surgery masquerading as a 2-site lift. **Deferred.**
- **HTTP/3 `QuicProtocolOptions` tuning** (in the `:175` sentence) — the router's named fallback. **REJECTED on a bimodal cost the mechanical footprint hides.** There is **no reject to lift** — the nested `core.v3.QuicProtocolOptions` is accepted-and-silently-ignored (`GetHttp3ProtocolOptions` has ZERO hits across `internal/`), inverting phase 64's shape: no reject-string test to flip, and a latent-divergence story that must be argued rather than read off an error. Of 9 core fields only ~1 maps cleanly to quic-go v0.54.1 (`max_concurrent_streams`); 3 are un-mappable/moot downstream (`connection_options`, `client_connection_options`, and `num_timeouts_to_trigger_port_migration` — which the proto says "has no effect on server sessions"); and 4 map with **contested semantics that cannot be made to agree with QUICHE by construction** (quic-go *auto-tunes* the receive windows Envoy states flat; `max_packet_length` is a max vs quic-go's PMTUD-raised *initial*; `connection_keepalive`'s two fields collapse to one `KeepAlivePeriod`; `idle_network_timeout` defaults disagree 600s vs 30s). And the `0104` observable (status + body + two codec-agnostic stat names) **does not move when any of these knobs change** — a tuned and untuned proxy are byte-identical there. Genuine cross-side proof needs a QUIC transport-parameter inspector (`quic.Config.Tracer`) that no test uses today — phase-61.3-scale harness surgery. **Deferred; still the strongest HTTP/3 re-open candidate, but it must be scoped honestly as either "unit-tests + one inert fixture + documented departures" or "+1 harness leg".**
- **tracing `custom_tags` `metadata`** (in the `:193` sentence) — the router rejected this by intuition ("the hardest"). **CONFIRMED with numbers, though the intuition was right for the wrong reason:** the dynamic-metadata plumbing *does* exist and is extensive (`internal/dynamicmetadata.Bucket`, ADR-0190; a landed `MetadataKey` path-descent resolver in `resolveMetadataValue`/`descendStructpbValue`). The cost is **threading**: `emitAccessLog*` are methods on the boot-shared `*Filter` taking no chain/bucket, so the bucket must reach **19 call sites** (5 `connection.go` / 7 `h2dispatch.go` / 7 `h3dispatch.go`), of which **≥3 are pre-chain-construction and have no bucket at all**. Plus a `ResolveCustomTags` signature change and a probable package extraction (`internal/tracing` cannot import a filter package). **Deferred.**
- **SDS `initial_fetch_timeout` edges** (in the `:185` sentence) — **REJECTED: no gap is visible on the envoy-go side, and the reference side reads as matching.** Re-derived: `ParseSDSConfig` sets `defaultInitialFetchTimeout = 15 * time.Second` then `if d := cs.GetInitialFetchTimeout(); d != nil { timeout = d.AsDuration() }`, so an explicit `0s` (a non-nil `durationpb`) overrides to 0; `FetchInitial*` gate on `if p.timeout > 0`, so 0 ⇒ block indefinitely. The reference's proto says *"0 means no timeout - Envoy will wait indefinitely for the first xDS config… The default is 15s."* **⚠️ Honest caveat: the reference side here is a PROTO READING, not a live probe** — the same move §2.4 refuses for D-COMBVC-FETCHTIMEOUT. The rejection rests on the envoy-go side being unambiguous and on there being no candidate *behavior* to lift; if a future roller wants this row, **probe the reference first**. Reconnection-backoff is separately not small: envoy-go SDS is initial-fetch-only, so there is no retry loop to attach a backoff to — honoring it means *building* the watch/reconnect loop. **Deferred.**
- **SDS rotation** (in the `:185` sentence) — needs a live-update path into an already-built `tls.Config` (`GetCertificate`/`GetConfigForClient` callbacks). A new seam. **Deferred.**
- **The compose-two edge** — deliberately DEFERRED behind the landed `seen>1` guard; needs the single-slot provider model (ONE `secretName`, ONE `*SDSStats`) generalized, putting `0103` at risk. **Deferred** — and note CVC does NOT force it open (§2.6).
- **The downstream TLS handshake-outcome `ssl` stat family** (in the `:193` sentence; ADR-0286 C3) — explicitly recorded as a FRAMEWORK-SURGERY row (envoy-go emits ZERO `ssl.*`; the reference emits the family live). Blows a +0-stat envelope. **Deferred.**
- **`google_grpc` transport / CDS/EDS / LDS/RDS / ADS / Delta xDS / RTDS** (all in the `:185` sentence) — each a substantially larger subsystem. **Deferred.**
- **`spawn_upstream_span` / `http_service` / force-trace** (in the `:193` sentence) / **`verbose`** — `spawn_upstream_span` needs a client/egress span concept that does not exist; `verbose` needs a per-stream-event log seam that does not exist. **Deferred.**
- **`crl`** — never sized as a standalone candidate by this document; it is named only as a SHARED gap (§2.9). A future roller should size it properly rather than inherit "gap" as "not a candidate". **Deferred.**
- **Opening gRPC / Runtime / WASM** (the three never-opened families) — each is a family OPENER, categorically the largest possible move. The directive says smallest-first; these are last. **Deferred.**

### 2.2 Scope: DOWNSTREAM + live provider + `require_client_certificate: true` ONLY *(self-answered; the incremental-lift precedent)*

**There are TWO gates, not one, and this document's first draft named only the first (§11):**

1. **The `commonTLSContextToConfig` gate** — `side == "downstream" && provider != nil`, exactly as phase 65 gated its arm. Upstream and QUIC keep the **BYTE-IDENTICAL** `combined_validation_context is not supported in phase 03` substring (ADR-0080 distinct substrings). `NewQUICDownstreamConfig` passes `side == "downstream"` with a **nil** provider (QUIC carries no SDS), so **the `provider == nil` half is what keeps QUIC rejecting — and QUIC is the guard's ONLY live consumer** (the upstream arm is structurally dead, §2.7; a PLAN must not read gate 1 as "upstream emits this").
2. **The apply-point gate** — the CVC branch lives inside `NewDownstreamConfig`'s `if ctx.GetRequireClientCertificate().GetValue()` block. **A downstream listener with CVC and `require_client_certificate` false/absent therefore gets: the reject no-ops → the require block never runs → CVC is entirely IGNORED, with no trust anchor and NO ERROR** — while boot has already dialed the SDS cluster. This silent-ignore is **inherited from phase 65, not created here**, and phase-65 §8 carried the corresponding item forward (*"`require_client_certificate: false` … verify-if-presented — phase 65 scopes to mandatory mTLS. Carries forward (low value)"*). Phase 66 **inherits the same scope and the same gap**, and tags it **D-COMBVC-REQUIRE-FALSE** so it is a named boundary rather than an unrecorded silence.

### 2.3 The discovery machine is UNCHANGED — and so is the WIRE *(self-answered)*

Stronger than phase 65's equivalent claim. CVC changes only how the LISTENER declares its trust anchors; the served `tls.v3.Secret` is byte-identical to `0108`'s. `internal/xds`, `parseValidationSecret`, `FetchInitialValidationContext`, and `sdsserver` are all **untouched**. This is why the row adds zero new symbols there.

### 2.4 Reference CVC semantics — what is documented, what is INFERRED, and what is genuinely unprobed *(SPEC probes to PIN)*

- **D-COMBVC-HYBRID-DS** — **THE row's sharpest risk (§1.1).** `proto.Merge` recursively merges the `trusted_ca` **message**, so the default's non-oneof `DataSource` sub-fields (notably `watched_directory`) **survive** a dynamic override, yielding a hybrid `DataSource` neither side authored. `watched_directory` is **inert in envoy-go and LIVE in the C++ reference (rotation)**. Probe: does the reference honor a `watched_directory` that arrived only via the *default* half of a CVC whose dynamic half overrode the specifier? If yes, that is a real cross-side divergence and must be either honored, rejected, or recorded as a named departure. **Unprobed; unstated in the first draft.**
- **D-COMBVC-REQUIRE-FALSE** — the `require_client_certificate: false` silent-ignore (§2.2). Pin: does the reference honor a CVC trust anchor for verify-if-presented when `require_client_certificate` is false? envoy-go silently ignores it today (inherited from phase 65). Either scope it out EXPLICITLY as a named boundary or lift it. **A silence is not a scope decision.**
- **D-COMBVC-CLONE** — `proto.Merge` **mutates `dst` in place**; `dst` would point into the parsed bootstrap. Pin the `proto.Clone` guard + the operand order (dynamic onto a CLONE of default). **Mandatory, not optional (§1.1).**
- **D-COMBVC-REFMERGE** — does the reference build ONE cert store from the merged message, or UNION two? **DEMOTED from "the central probe" (§11).** The proto's own trailing sentence — *"**The resulting** `CertificateValidationContext` is used to perform certificate validation"*, singular — is documentary evidence for ONE store, and **both sides use the same protobuf `MergeFrom` semantics**, so the merged *message* is not in question; only what Envoy does with it afterward. Still worth ONE cheap confirming probe (fresh container, `envoyproxy/envoy:contrib-v1.37.2`, `reference_probe_fresh_container_per_arm`) — but it no longer gates the SPEC, and the first draft's claim that "everything turns on it" was a self-inflicted artifact of truncating the quote.
- **D-COMBVC-EMPTY-DYNAMIC** — probed this session: a dynamic VC with **no** `trusted_ca` leaves the default's `trusted_ca` **INTACT** — an SDS server serving an empty/degraded VC silently falls back to the default CA with no error. Pin whether that is the reference's behavior and whether envoy-go should mirror or reject it.
- **D-COMBVC-PUREINLINE** — a CVC with NO `validation_context_sds_secret_config` is legal per the proto (the field is a plain, optional message field — verified). Accepted as equivalent to a plain `validation_context`, or rejected? No in-repo precedent. Probe; drives the §2.6 nil-check.
- **D-COMBVC-SERVED-SUBFIELDS** — the reference's behavior when the served secret carries a `parseValidationSecret`-rejected sub-field AND a merged default exists. Unprobed interaction.
- **D-COMBVC-FETCHTIMEOUT** — anticipated: the ADR-0280 boot-FAIL DEPARTURE extends UNCHANGED (same `FetchInitialValidationContext` bound, no new knob, no new departure). **Confirm; do not assume it transfers.**

### 2.5 Only ONE merge rule is REACHABLE — and it is NOT the one the proto names *(a NAMED COVERAGE BOUNDARY; SPEC records)*

envoy-go's honored `CertificateValidationContext` surface is **`trusted_ca` and nothing else** (`loadTrustedCAPool` reads only `vc.GetTrustedCa()` — verified).

**The reachable rule is the proto summary's blind spot.** Because `trusted_ca` is a **message**, what actually fires is `proto.Merge`'s **recursive message merge** — *not* the "Singular Fields: dynamic overrides default" bullet (§1.1). The observable behaves override-like only because the DataSource's `specifier` **oneof** replaces. **The SPEC must record the mechanism, not the proto's bullet** — this document's first draft asserted the bullet and was wrong at the root (§11).

The **repeated-concatenate** and **bool-OR** rules are **structurally unreachable**, because envoy-go honors no repeated or bool field of `CertificateValidationContext`. Enumerated in FULL (the first draft asserted a roster it had not enumerated — §11):

| Kind | Field | envoy-go |
|---|---|---|
| repeated | `verify_certificate_spki` | **rejected** |
| repeated | `verify_certificate_hash` | **rejected** |
| repeated | `match_typed_subject_alt_names` | **rejected** |
| repeated | `match_subject_alt_names` (f9, deprecated) | **silently ignored** — not one of "the four" |
| bool | `allow_expired_certificate` | silently ignored |
| bool | `only_verify_leaf_cert_crl` (f14) | silently ignored |
| *(not a bool)* | `require_signed_certificate_timestamp` | `*wrapperspb.BoolValue` — a **message**; bool-OR would not govern it even if honored |

**The row proves ONE of three documented merge rules, and does so by a mechanism the proto's summary does not name.** The SPEC must state this as a NAMED COVERAGE BOUNDARY rather than imply `MergeFrom` fidelity — "we implement the documented merge semantics" would be exactly the uncited-prose overclaim §11 warns about.

### 2.6 The boot pre-scan MUST grow a third arm — with a nil-check that has NO phase-65 analogue *(D-COMBVC-PRESCAN; SPEC pins)*

RE-DERIVED: `NewSDSProvider`'s pre-scan type-asserts on `*tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig` **only**. `CommonTlsContext_CombinedValidationContext` is a **distinct oneof wrapper type** — the assert fails, `seen` stays 0, and `NewSDSProvider` returns **`(nil, nil)` — a silent nil, not an error**. A CVC listener would then either boot-FAIL on a nil-provider guard or, worse, **silently fall through and use only `default_validation_context` — a silently-wrong trust anchor.**

So the row MUST add a third arm. **The subtlety with no phase-65 analogue:** `validation_context_sds_secret_config` is a plain, **OPTIONAL** message field inside CVC, so the arm must nil-check the *inner* field before `seen++` — otherwise a pure-inline CVC trips `ParseSDSConfig` on a nil entry at boot. The plain-VC arm has no such hazard **in practice** (its oneof payload is set on any unmarshalled config) — though note the landed code passes `vsc.ValidationContextSdsSecretConfig` with **no nil-check**, so that is a wire-shape assumption rather than a type guarantee. The CVC arm must not inherit the assumption.

**Good news for scope:** CVC yields exactly ONE `SdsSecretConfig` ⇒ `seen == 1`. **The `seen>1` guard is untouched and the deferred compose-two edge stays shut.**

### 2.7 ⚠️ The four inline sub-field rejects are currently BYPASSED under CVC — a real correctness obligation *(D-COMBVC-REJECT-REPOINT; SPEC pins)*

The inline reject block is guarded by `if vc := c.GetValidationContext(); vc != nil`. Under a CVC the oneof makes `GetValidationContext()` return **nil** (verified against the generated getter) ⇒ **the entire block is SKIPPED**. Lifting the CVC reject without re-pointing that block at `cvc.GetDefaultValidationContext()` would **silently accept** `custom_validator_config` / `match_typed_subject_alt_names` / `verify_certificate_hash` / `verify_certificate_spki` on the default context.

This is `reference_strict_reject_sibling_typeurl_gap` in its exact shape: **lifting an envelope is not license to silently accept sub-fields envoy-go cannot honor.** ADR-0286 §Decision states the principle for the SDS path ("The CVC reject roster is held to the inline support surface"). This row owes the same for the CVC path — a correctness obligation, not a nicety. **All four are unproven under CVC today, so each needs its OWN test.**

**Anticipated recurrence (do NOT let a PLAN assert otherwise):** the upstream CVC arm is **DEAD** from today's entry points, exactly as ADR-0286 found for the plain-VC arm — `validation_context_type` is a **oneof**, so selecting CVC makes `GetValidationContext()` nil and `NewUpstreamConfig` refuses EARLIER with its own `trusted_ca is required` message (re-derived and confirmed). **QUIC is the CVC guard's only live consumer.** Phase 65 shipped this exact false claim into a code comment before catching it — §11.

### 2.8 Fixture posture: ONE new fixture, ~95% driver reuse *(self-answered direction; SPEC pins D-COMBVC-FIXTURE / -OVERRIDE-OBSERVABLE / -STRUCTURAL)*

- **D-COMBVC-OVERRIDE-OBSERVABLE** — the discriminating design: serve **CA_X** over SDS, set `default_validation_context.trusted_ca` = **CA_Y** (a different, unserved CA), then a **client_X** cert must be ACCEPTED and a **client_Y** cert REJECTED — proving the dynamic context **replaced the specifier** rather than unioning. `0108`'s existing two-arm `good=`/`bad=` verdict expresses this with **zero new observable machinery** — it is a CA re-labelling. Note it *also* happens to distinguish D-COMBVC-EMPTY-DYNAMIC, by accident rather than design; the SPEC should make that deliberate.
- **D-COMBVC-STRUCTURAL** — **MANDATORY, and the lesson must be SHOWN not asserted.** Phase 65 DEMONSTRATED that with `0108`'s `structuralCheck` disabled, a served-CA break ships **PASS**: both sides emit `good=REJECTED`/`bad=ACCEPTED` and `CompareBytes` compares EQUAL. **A pure-`CompareBytes` fixture ships green on a completely broken trust anchor.** Any CVC break changes BOTH sides identically, so the in-driver structural check is load-bearing (`reference_vacuous_break_receiver_normalizes`).
- Anticipated fixture `0109-xds-sds-combined-validation-context` ⇒ fixtures **110 → 111**. `sdsserver` untouched. `BackendCount` ≥1 per `reference_differential_backendcount_min_one`.

### 2.9 The reject narrows; envoy-go and the reference AGREE on downstream CVC *(self-answered; ADR-0080)*

Post-row, downstream CVC with a live provider under `require_client_certificate: true` is CONSUMED and the two sides agree. Upstream + QUIC keep the byte-identical substring. **`crl` stays a documented SHARED gap** — the inline block does not check it either, so rejecting it on the CVC path would introduce a NEW asymmetry. CVC does not close it (§2.5).

### 2.10 Stat surface hypothesis: +0; fuzz: +0 *(self-answered; SPEC confirms D-COMBVC-STATS / -FUZZSEED)*

+0 — the phase-60.2 `sds.*` lifecycle counters are reused verbatim (the fetch path is unchanged). Stat surface stays **1201**. Fuzz: SEEDS to the existing fuzzers (`FuzzTLSContextParse` is the natural home; the `tls: ` prefix invariant it enforces already constrains the new branch) — **NO new fuzzer**; count stays **55**.

---

## 3. Framework-survey result — a config-shape lift; ZERO new packages/modules/symbols-in-xds

### 3.1 Framework: a switch arm + a reject re-point + a 3-way branch + a clone-merge + a pre-scan arm

Four production edit sites across two files. No new seam, no new discovery machine, no new applier.

### 3.2 NEW packages: NONE

### 3.3 go.mod modules: NONE ADDED

`google.golang.org/protobuf v1.36.11` (which provides `proto.Merge`/`proto.Clone`) is **already a direct require**. The project's tracked figure **"go.mod modules 2"** is the phase-61.2 lineage count (`quic-go` direct + `qpack` indirect) — **not** a repo total; the single `go.mod` requires 67 modules. That figure stays **2 (+0)**.

### 3.4 REUSES

- `FetchInitialValidationContext` / `parseValidationSecret` / the SotW stream (phase 60.2 + 65) — **verbatim**.
- `loadTrustedCAPool` (phase 03) — for the merged context.
- `sdsserver.WithValidationContext` + `0108`'s driver (PKI, `mtlsEcho`, `structuralCheck`, `normalizeTLSErr`, per-side servers) — **~95%**.
- `proto.Clone` + `proto.Merge` — the merge, behind a mandatory clone guard (§1.1).

### 3.5 The 60.2 cycle guard STANDS — trivially

`internal/xds` is untouched; its dep set stays **exactly `internal/stats` + `internal/xds`** (re-verified this session with `go list -deps ./internal/xds`, no `...`, per `reference_xds_config_seam_transitive_cycle_guard`). `internal/boot` already imports both `internal/xds` and `internal/tls`. **No new edge is introduced anywhere.**

---

## 4. Bootstrap-level applicability — a PER-LISTENER downstream TLS sub-field

CVC lives at `listener.filter_chains[].transport_socket.typed_config.common_tls_context.combined_validation_context` — a per-listener downstream TLS sub-field, NOT a bootstrap-level knob. Same scope as phase 65.

---

## 5. Stat surface hypothesis — +0 (66)

### 5.1 Stat names (SPEC confirms)
None added. The `sds.*` lifecycle counters registered at phase 60.2 cover the fetch; CVC changes no fetch.

### 5.2 envoy-go-strict departure flags
The upstream + QUIC `combined_validation_context is not supported in phase 03` substring STAYS byte-identical (ADR-0080). `crl` + the four sub-fields stay a SHARED gap. The merge-rule reachability (§2.5), the `require==false` silent-ignore (§2.2), and any hybrid-`DataSource` divergence (§2.4) become NAMED COVERAGE BOUNDARIES.

### 5.3 Anticipated surface arithmetic
**1201 (+0).**

---

## 6. Edit-site enumeration — RE-DERIVED this session (SPEC re-derives + pins D-COMBVC-HYBRID-DS / -REQUIRE-FALSE / -CLONE / -PRESCAN / -REJECT-REPOINT / -OVERRIDE-OBSERVABLE)

**Production — `internal/tls/config.go`:**
1. `commonTLSContextToConfig` — the `*tlsv3.CommonTlsContext_CombinedValidationContext` switch arm: reject → scoped no-op gated `side == "downstream" && provider != nil`, mirroring the landed phase-65 arm. `[EDIT]`
2. `commonTLSContextToConfig` — the four inline sub-field rejects re-pointed to also cover `cvc.GetDefaultValidationContext()` (§2.7 — a correctness obligation; the block is BYPASSED under CVC today). `[EDIT]`
3. `NewDownstreamConfig` — the `require_client_certificate` if/else type-assert → a 3-way branch (SDS-VC / CVC / inline). The CVC branch: `FetchInitialValidationContext` (verbatim) + **`proto.Clone` the default** + `proto.Merge` the dynamic onto the clone + `loadTrustedCAPool` → `cfg.ClientCAs`. `[EDIT]`

**Production — `internal/boot/boot.go`:**
4. `NewSDSProvider` — the pre-scan third arm for the CVC wrapper, WITH the optional-inner nil-check (§2.6). `[EDIT]`

**Production — `internal/xds`:** NONE. `[UNTOUCHED — the row's defining property]`

**Test / harness:**
5. `internal/tls/config_test.go` — the CVC reject-flip + the merge/override unit tests + a **clone-guard test proving the parsed bootstrap is not mutated** + the four re-pointed sub-field rejects (BYPASSED today ⇒ each needs its own test). `[EDIT/ADD]`
6. `internal/boot/boot_test.go` — the pre-scan third arm + the pure-inline-CVC nil-check + a `seen==1` assertion. `[EDIT/ADD]`
7. Fuzz seeds to `FuzzTLSContextParse` (NO new fuzzer). `[EDIT]`
8. `test/helpers/sdsserver` — `[UNTOUCHED]`

**Fixture:**
9. `test/fixtures/0109-xds-sds-combined-validation-context/` — driver (~95% from `0108`), `envoy.yaml` + `envoy-go.yaml`, `expectations.yaml`, `README.md`. `[ADD]`

**BEHAVIOR_CONTRACT:**
10. The xDS/SDS section — CVC moves REJECTED → CONSUMED (downstream + provider + `require==true`); the upstream/QUIC reject, the `crl`/sub-field SHARED gap, the §2.5 merge-rule boundary, the §2.2 `require==false` boundary, and any §2.4 hybrid-DataSource departure recorded. `[EDIT]`

**ROADMAP / STATE / DECISIONS:**
11. `ROADMAP.md` — row 66 + the xDS family CHARTERED paragraph. **The deferred sentence is UNCHANGED** (§1.3). `[BRAINSTORM: row + prose]`
12. `STATE.md` — the active-phase bullet. `[BRAINSTORM]`
13. `DECISIONS.md` — ADR-0287 §Context at the SPEC (ADR-0044); §Decision/§Consequences at the IMPL. `[SPEC/IMPL]`
14. `next-prompt.txt` — the router roll (TRACKED — `reference_next_prompt_tracked_despite_gitignore`). `[BRAINSTORM]`

SPEC pins **D-COMBVC-DOCSHAPE** (this roster, RE-DERIVED) + **D-COMBVC-HYBRID-DS/-REQUIRE-FALSE/-CLONE/-REFMERGE/-EMPTY-DYNAMIC/-PUREINLINE/-SERVED-SUBFIELDS/-FETCHTIMEOUT** (§2.4) + **D-COMBVC-MERGERULES** (§2.5) + **D-COMBVC-PRESCAN/-SINGLESLOT** (§2.6) + **D-COMBVC-REJECT-REPOINT/-UPSTREAM-DEAD** (§2.7) + **D-COMBVC-FIXTURE/-OVERRIDE-OBSERVABLE/-STRUCTURAL** (§2.8) + **D-COMBVC-STATS/-FUZZSEED** (§2.10) + **D-COMBVC-SPLIT** (§1.4).

> **⚠️ D-tag mnemonic — a deliberate collision avoidance.** The obvious `D-CVC-*` is **REJECTED**: ADR-0286 already uses "CVC" throughout to mean **`CertificateValidationContext`** (verified: *"the CVC reject roster… `CertificateValidationContext` sub-fields"*; *"the CVC feature surface (`crl` + the four rejected sub-fields)"*), NOT `CombinedValidationContext`. `D-COMBVC-*` is unambiguous. This is `reference_spec_drafted_identifier_collision_check` applied to PROSE terminology rather than to a Go symbol — the same failure mode, a different namespace. The SPEC must not conflate the two senses, and **ADR-0286's "SHARED gap" sentence is about `CertificateValidationContext` sub-fields and does NOT license this row.**

---

## 7. Anticipated ADRs — 1 at the phase-66 IMPL: ADR-0287 (xDS SDS `combined_validation_context`)

§Context drafts at the SPEC per ADR-0044; §Decision/§Consequences land at the IMPL. A BRAINSTORM anchors no ADR and reserves nothing.

---

## 8. Deferred items

- **`xds-sds-upstream-server-cert`** — the value-level constructibility cycle + the listener-only pre-scan + the eager `upstreamCfg`/`extractH2Mode` coupling (§2.1.1). Needs a boot-model reshape. **Carries forward with the mechanism recorded** — the next roller must not re-derive it.
- **`require_client_certificate: false`** (optional / verify-if-presented mTLS with an SDS or CVC trust anchor) — **a DIRECT pickup of phase-65 §8**, inherited unchanged, and now tagged D-COMBVC-REQUIRE-FALSE so the silent-ignore is a named boundary (§2.2). Carries forward.
- **The hybrid-`DataSource` merge leak** — if the SPEC's probe shows the reference honors a default-supplied `watched_directory` under a dynamic override, that is a real divergence needing its own disposition (§2.4). Carries forward pending the probe.
- **HTTP/3 `QuicProtocolOptions` tuning** — must be scoped as either "unit-tests + inert fixture + documented departures" or "+1 harness leg" (§2.1). Carries forward.
- **DataSource `environment_variable`** — blocked on the D-ENV-HARNESS seam SPEC-63 declined; inherits the 2–2 set-but-EMPTY disagreement. Carries forward.
- **tracing `custom_tags` `metadata`** — ~19 call-site threading + a signature change + a probable package extraction; ≥3 emit sites are pre-chain. Carries forward.
- **The compose-two edge** — behind the `seen>1` guard; needs the single-slot provider generalized. Carries forward.
- **SDS rotation** — needs a live-update path into a built `tls.Config`. Carries forward.
- **SDS `initial_fetch_timeout` / reconnection-backoff edges** — no envoy-go gap visible; the reference side is a proto reading, NOT a probe (§2.1). Carries forward.
- **`crl` + the four `CertificateValidationContext` sub-fields** — a SHARED gap on BOTH paths (§2.9). `crl` has never been sized as a standalone candidate; a future roller should. Carries forward.
- **The repeated-concatenate + bool-OR merge rules** — structurally unreachable while `trusted_ca` is the only honored field (§2.5). A NAMED COVERAGE BOUNDARY. Carries forward.
- **The downstream TLS handshake-outcome `ssl` stat family** — a framework-surgery row (ADR-0286 C3). Carries forward.
- **CDS/EDS · LDS/RDS · ADS · Delta xDS · RTDS · `google_grpc`** — each a larger subsystem. Carry forward.
- **gRPC / Runtime / WASM** — never-opened families; each a family OPENER. Carry forward.

The termination sentinel does NOT fire: checks (2) and (3) print (see the preamble).

---

## 9. Cross-references against prior phases' deferred-items lists — pickup + sentinel maintenance

This row PICKS UP **two** phase-65 §8 items: (1) *"`combined_validation_context` — an inline `default_validation_context` merged with an SDS CA; the natural follow-on ONCE plain SDS-`validation_context` lands. Carries forward."* — the precondition is now satisfied, and this row consumes it; and (2) *"`require_client_certificate: false` optional mTLS with an SDS CA (verify-if-presented) — phase 65 scopes to mandatory mTLS. Carries forward (low value)."* — **NOT consumed**; inherited unchanged and re-carried at §8, now tagged D-COMBVC-REQUIRE-FALSE. *(This document's first draft silently DROPPED item (2) while claiming a complete cross-reference — §11.)*

**The live xDS deferred sentence (`ROADMAP.md:185`) is UNCHANGED by this row** (§1.3; `combined_validation_context` does not appear in it — verified), so check (2) keeps printing three sentences and the sentinel keeps not firing. No sentinel maintenance is owed. Check (3) is unaffected (no new family).

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution

**Ranked by blast radius (the first draft's ranking was wrong — §11):**

- **D-COMBVC-HYBRID-DS** — `proto.Merge` recursively merges the `trusted_ca` **message**, so the default's non-oneof sub-fields (notably `watched_directory`, inert in envoy-go and LIVE in the reference) survive a dynamic override into a hybrid `DataSource`. Probe the reference; honor, reject, or record as a named departure. **The row's sharpest genuine risk.** §1.1/§2.4.
- **D-COMBVC-REQUIRE-FALSE** — CVC + `require_client_certificate` false/absent is silently IGNORED (no anchor, no error), inherited from phase 65. Scope it out explicitly as a named boundary, or lift it. **A silence is not a scope decision.** §2.2.
- **D-COMBVC-CLONE** — `proto.Merge` mutates `dst` in place and `dst` points into the parsed bootstrap ⇒ a `proto.Clone` guard is MANDATORY. Pin it + the operand order, and TEST that the bootstrap is unmutated. §1.1.
- **D-COMBVC-MERGERULES** — pin that the reachable rule is `proto.Merge`'s **recursive message merge** (NOT the proto's "singular override" bullet), that only 1 of 3 documented rules is reachable, and record it as a NAMED COVERAGE BOUNDARY. The SPEC must NOT imply `MergeFrom` fidelity. §2.5.
- **D-COMBVC-REJECT-REPOINT** — re-point the four inline sub-field rejects at `GetDefaultValidationContext()`; each needs its OWN test (all four are BYPASSED under CVC today, so none is currently proven). `reference_strict_reject_sibling_typeurl_gap`. §2.7.
- **D-COMBVC-PRESCAN** — the pre-scan third arm + the optional-inner nil-check (no phase-65 analogue). Pin names (GREP-collision-checked, `reference_spec_drafted_identifier_collision_check`) + the exact guard. §2.6.
- **D-COMBVC-EMPTY-DYNAMIC** — a dynamic VC with no `trusted_ca` silently falls back to the default CA (probed). Mirror or reject? §2.4.
- **D-COMBVC-PUREINLINE** — CVC with NO `validation_context_sds_secret_config` (legal per proto): accept as equivalent to a plain `validation_context`, or reject? Drives the §2.6 nil-check. §2.4.
- **D-COMBVC-OVERRIDE-OBSERVABLE** — the CA_X-served / CA_Y-default two-arm fixture design; make its D-COMBVC-EMPTY-DYNAMIC discrimination deliberate rather than accidental. §2.8.
- **D-COMBVC-STRUCTURAL** — the in-driver structural check is MANDATORY and its necessity must be SHOWN (disable it, watch a broken-trust-anchor break ship PASS), not asserted. `reference_vacuous_break_receiver_normalizes`. §2.8.
- **D-COMBVC-UPSTREAM-DEAD** — RE-DERIVE (do not assert) that the upstream CVC arm is dead via the oneof, leaving QUIC as the guard's only live consumer. Phase 65 shipped this exact claim wrong into a code comment. §2.7.
- **D-COMBVC-SERVED-SUBFIELDS** — the reference's behavior when the served secret carries a `parseValidationSecret`-rejected sub-field AND a merged default exists. §2.4.
- **D-COMBVC-REFMERGE** — one store or two? DEMOTED: the proto's trailing sentence is singular and both sides share `MergeFrom` semantics. ONE cheap confirming probe; no longer gates the SPEC. §2.4.
- **D-COMBVC-FETCHTIMEOUT** — confirm the ADR-0280 boot-FAIL DEPARTURE extends UNCHANGED. Do not assume it transfers. §2.4.
- **D-COMBVC-SINGLESLOT** — confirm CVC yields `seen == 1` so the `seen>1` guard and the deferred compose-two edge stay untouched. §2.6.
- **D-COMBVC-FIXTURE** — pin the `0109` shape + the `0108` reuse boundary + `BackendCount` ≥1. §2.8.
- **D-COMBVC-STATS** — confirm +0 (1201). §2.10.
- **D-COMBVC-FUZZSEED** — confirm seeds-only to `FuzzTLSContextParse` (55 stays). §2.10.
- **D-COMBVC-SPLIT** — confirm a SINGLE FLAT ROW (~6–9 tasks); ADR-0045 escape-valve armable. §1.4.
- **D-COMBVC-DOCSHAPE** — RE-DERIVE the §6 roster. §6.

---

## 11. Prior-phase lessons applied

- **⚠️ THE LESSON THIS BRAINSTORM ADDS: quoting is not executing — and a BRAINSTORM is not evidence either.** This document's first draft was adversarially re-derived before landing and **ELEVEN defects were found in it**. The severe ones all trace to ONE omission — **it quoted `proto.Merge`'s documentation instead of RUNNING it**:
  - `trusted_ca` is a **message**, so `proto.Merge` **recursively merges** it rather than singular-overriding — inverting §2.5's central reasoning. Running it exposed a **hybrid `DataSource`** (`watched_directory` surviving a dynamic override) that no amount of re-reading would have surfaced.
  - `proto.Merge` **mutates `dst`**, so the "no hand-written merge code" claim was false — a `proto.Clone` guard is mandatory.
  - The "verbatim" proto quote was **silently TRUNCATED**, omitting *"The resulting `CertificateValidationContext` is used to perform certificate validation"* — the singular phrasing that largely answers the risk the draft then declared "THE central probe". **The document manufactured its own central risk by narrowing a quote it labelled verbatim.** This is `feedback_brief_citations_not_evidence` inverted: not a wrong citation, a *silently narrowed* one, in the one place a full read was decisive.
  - It **dropped** a phase-65 §8 carry-forward (`require_client_certificate: false`) while claiming a complete cross-reference — and that dropped item is exactly the row's unrecorded silent-ignore gap.
  - It claimed HTTP/3 was "the only candidate that would narrow a live deferred sentence" — **refuted by its own next-but-one bullet**.
  - It asserted a field roster it had not enumerated (§2.5 missed `match_subject_alt_names` and `only_verify_leaf_cert_crl`).
  **The generalization: the project already knows a BRIEF's and a SPEC's and a PLAN's citations are not evidence. A BRAINSTORM's are not either — and where a claim is about EXECUTABLE semantics, only execution settles it. The phase-66 SPEC must RUN `proto.Merge`/`proto.Clone`, not cite them.**
- **`feedback_brief_citations_not_evidence`** — applied to the ROUTER, and it paid: the provisional pick was **overturned on evidence** (§2.1.1); its `NewUpstreamConfig`/caller/cycle-guard citations were re-derived (all three HELD; the pick still failed for a reason none covered). **The lesson generalizes past `file:line` to uncited PROSE** — the router's "boot-ordering is a genuine open question" was the defect, not any line number.
- **Cite by SYMBOL, not by line — VINDICATED MECHANICALLY.** The CVC reject is cited as `:229-230` by BOTH ADR-0286 and BRAINSTORM-65; the real line is **`:281`**. Two landed documents carry the same stale number. This BRAINSTORM cites `commonTLSContextToConfig` / `NewDownstreamConfig` / `NewSDSProvider` / `loadTrustedCAPool` by name.
- **`reference_xds_config_seam_transitive_cycle_guard` — REFINED, not just cited.** It names the *type*-level edge. Upstream SDS's blocker is at the **VALUE** level (the provider's transport is dialed through the manager being built), which the memory does not cover. §2.1.1.
- **`reference_spec_drafted_identifier_collision_check` — applied to PROSE.** `D-CVC-*` is rejected because ADR-0286 already binds "CVC" to `CertificateValidationContext`. §6.
- **`reference_vacuous_break_receiver_normalizes` — must be DEMONSTRATED.** Phase 65 showed a `0108` break shipping PASS with `structuralCheck` off. Any CVC break changes both sides identically ⇒ same trap. §2.8.
- **`reference_plan_break_instructions_dont_compile`** — a non-compiling break shows red that proves NOTHING. The PLAN must substitute a compiling equivalent, REPORT the substitution, and record the TRUE result.
- **A break that does NOT fire is a finding** (phase-65 T3) — record it as an honest, UNCLAIMED coverage gap; do not route around it.
- **A PLAN is not evidence either** — phase 65's IMPL found ELEVEN defects in a PLAN that had itself corrected five SPEC defects. The phase-66 IMPL must re-derive the PLAN, not execute it.
- **`reference_sds_init_fetch_timeout_dial_budget_flake`** — if `TestProvider_FetchInitialCertificate_Timeout` fails under `-race`, it is PRE-EXISTING on master (observed once, 2026-07-16). Read the memory; do NOT reflex-classify it as a phase-66 regression. A SECOND occurrence justifies widening the budget.
- **`reference_fatalf_makes_assertions_unreachable`** — `Errorf` per independent property; `Fatalf` only for a broken precondition.
- **`reference_differential_run_selector`** / **`reference_differential_break_protocol_count1`** — `-run 'TestDifferential/0109-xds-sds-combined-validation-context'`; `-count=1` on every deliberate break.
- **`reference_strict_reject_sibling_typeurl_gap`** — §2.7's re-point is this memory's exact shape.
- **`feedback_git_worktrees`** / **`feedback_subagent_worktree_detach`** / **`feedback_subagent_worktree_path_targeting`** — pinned worktree root; main checkout verified clean; `git restore` only on a break.
- **`reference_next_prompt_tracked_despite_gitignore`** — the router is edited IN this worktree and folded into the squash; commits located by SUBJECT (`git log --grep`), never by position.
- **`feedback_subagents_no_push`** / **`feedback_push_to_origin`** — subagents commit locally; the controller squash-pushes at stage-close.

---

## 12. Section closeout

**Settled:** subject (`xds-sds-combined-validation-context`, **SELF-PICKED** per the standing directive as the smallest candidate whose entire substrate is confirmed-landed AND which phase 65 itself named as the next follow-on with its precondition now met — §2.1); the router's provisional upstream-SDS pick **OVERTURNED on evidence**, with the value-level cycle recorded so it is not re-derived (§2.1.1); scope = downstream + live provider + `require_client_certificate: true` — **TWO gates, not one** (§2.2); the wire and `internal/xds` are UNTOUCHED (§2.3); the merge is `proto.Merge` behind a **MANDATORY `proto.Clone` guard**, and its reachable rule is a **recursive message merge, NOT the proto's "singular override" bullet** (§1.1/§2.5); only 1 of 3 documented merge rules is reachable — a NAMED COVERAGE BOUNDARY (§2.5); the pre-scan needs a third arm with an optional-inner nil-check (§2.6); the four inline sub-field rejects are BYPASSED today and must be re-pointed (§2.7); `D-COMBVC-*` chosen over `D-CVC-*` to avoid ADR-0286's terminology binding (§6).

**Anticipated moves at the phase-66 IMPL (docs-only now):** fixtures **110 → 111** (`0109-xds-sds-combined-validation-context`) · stat surface **1201 (+0)** · fuzzers **55 (+0)** · BackendKind **38 (+0)** · DECISIONS tail **ADR-0286 → ADR-0287** · go.mod modules **2 (+0)** · ZERO new packages · ZERO new symbols in `internal/xds`. **The deferred sentence is NOT narrowed by this row** (§1.3) — a §8-tier candidate, the phase-64 precedent.

**Counts UNCHANGED at this BRAINSTORM (docs-only; re-verified against master tip `d545810a` this session, `git fetch` first):** fixtures **110** · fuzzers **55** · stat surface **1201** · BackendKind **38** · DECISIONS tail **ADR-0286** (next-free **ADR-0287**) · go.mod modules **2**.

**Next → the phase-66 SPEC** — whose FIRST obligation is to **RUN, not cite**: exercise `proto.Merge`/`proto.Clone` against real `CertificateValidationContext` messages and pin D-COMBVC-HYBRID-DS + D-COMBVC-CLONE from observed behavior. Then probe the reference (fresh container, `envoyproxy/envoy:contrib-v1.37.2`, `reference_probe_fresh_container_per_arm`) for the hybrid-`DataSource` question and dispose D-COMBVC-REQUIRE-FALSE explicitly rather than by silence. D-COMBVC-REFMERGE is a cheap confirmation, not a gate.
