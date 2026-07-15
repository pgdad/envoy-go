# Phase 65 Brainstorm — `xds-sds-validation-context` (the THIRD xDS-family row; the SECOND SDS resource type after phase-60.2's downstream server cert — an SDS-delivered downstream mTLS trusted-CA / `validation_context`, SotW/initial-fetch — lifts the `internal/tls/config.go:227-228` `validation_context_sds_secret_config is not supported` reject and installs the SDS-served CA bundle into the downstream listener's `ClientCAs` for mandatory mTLS client-cert validation; anticipated +0 packages / +0 modules; anticipated ONE new fixture)

> **Stage:** BRAINSTORM (lifecycle-state 0 → 1). Docs-only; no `.go` changes at this stage. Fresh worktree off master, branch `phase-65-xds-sds-validation-context`, per `feedback_git_worktrees`.
>
> **Loop re-open (AUTONOMOUS — no human pick):** phase 64 (`tracing-max-path-tag-length`) landed COMPLETE (row 64 `done`, ADR-0285; the Observability family STAYS OPEN). Per the **STANDING DIRECTIVE (human, 2026-07-12)** the loop runs AUTONOMOUSLY until the termination sentinel fires; the sentinel was re-checked MECHANICALLY at the phase-64 IMPL and does NOT fire (check (1) silent — every row `done` — but check (2) prints THREE live "candidates:" sentences [HTTP/3, xDS, Observability] and check (3) prints THREE never-opened families [gRPC, Runtime, WASM], each independently blocking `stop`). No banked mid-lifecycle split legs remain (no `in-progress` ROADMAP rows). So the roller SELF-PICKED the next subject (§2.1): the **smallest cleanly-differential-provable candidate whose ENTIRE substrate is confirmed-landed** from a full source read against the current master tip — the xDS **SDS `validation_context`** resource type (an SDS-delivered downstream mTLS trusted-CA on the ALREADY-landed phase-60.2 SDS discovery-stream client + the ALREADY-landed phase-16 static-mTLS `ClientCAs` path) — over the declined larger/less-cleanly-provable alternatives (recorded §2.1). No human pause; no `stop` file.
>
> **Baselines re-verified against master tip `6f46481d` (the phase-64 IMPL squash):** stat surface **1201** · fixtures **109** (`ls -d test/fixtures/[0-9]*/ | wc -l`; tail `0107-tracing-max-path-tag-length`; the count includes the lettered `0007a`/`0007b` sub-fixtures — a `^[0-9]{4}-` grep under-counts; numeric tail `0107`) · fuzzers **55** (`grep -rn '^func Fuzz' --include='*.go' | wc -l`) · BackendKind tail **38** (`H2GoawayResponder`) · DECISIONS tail **ADR-0285** (next-free **ADR-0286**) · new Go packages **0** · go.mod modules **2** (`quic-go v0.54.1` direct + `qpack v0.5.1` indirect). Counts are UNCHANGED at a BRAINSTORM (docs-only). All `file:line` citations below were RE-DERIVED from source this session against master `6f46481d` (`feedback_brief_citations_not_evidence`) — see §11.

---

## 1. Mission and scope confirmation (65 — a SECOND SDS resource type on the landed SDS + static-mTLS substrate, NOT a new discovery machine)

### 1.1 What phase 65 delivers as a self-contained whole (an SDS-delivered downstream mTLS trusted-CA)

The downstream TLS parse today STRICT-REJECTS an SDS-bound `validation_context`:

```go
// internal/tls/config.go:225-231 (re-derived against master 6f46481d)
if c.GetValidationContextType() != nil {
    switch c.GetValidationContextType().(type) {
    case *tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig:
        return nil, fmt.Errorf("tls: %s: SDS-bound validation_context_sds_secret_config is not supported in phase 03", side)
    case *tlsv3.CommonTlsContext_CombinedValidationContext:
        return nil, fmt.Errorf("tls: %s: combined_validation_context is not supported in phase 03", side)
    }
}
```

Meanwhile the ADJACENT SDS-delivered downstream **server cert** IS honored (phase 60.2, ADR-0280) — the EXACT template phase 65 mirrors:

```go
// internal/tls/config.go:206-223 — the landed tls_certificate SDS path
var sdsCert *stdtls.Certificate
if len(c.GetTlsCertificateSdsSecretConfigs()) > 0 {
    // ... side check, xds.ParseSDSConfig(...), provider.FetchInitialCertificate(...), hold ...
    sdsCert = cert   // appended to cfg.Certificates below
}
```

And static mTLS (an INLINE `validation_context.trusted_ca`) IS honored (phase 16, ADR-0147) — the CA-pool install phase 65 reuses:

```go
// internal/tls/config.go:67-79 — require_client_certificate=true + INLINE trusted_ca → ClientCAs + mTLS
if ctx.GetRequireClientCertificate().GetValue() {
    common := ctx.GetCommonTlsContext()
    vc := common.GetValidationContext()
    if vc == nil || vc.GetTrustedCa() == nil {
        return nil, fmt.Errorf("tls: downstream: require_client_certificate=true requires validation_context.trusted_ca")
    }
    pool, err := loadTrustedCAPool(vc, baseDir, "downstream")   // → *x509.CertPool
    // cfg.ClientCAs = pool; cfg.ClientAuth = stdtls.RequireAndVerifyClientCert
}
```

Phase 65 **lifts the one downstream reject** (`config.go:227-228`) and HONORS an SDS-delivered `validation_context`: fetch the trusted-CA bundle over the SotW SDS stream (bounded by `initial_fetch_timeout`, mirroring the server-cert fetch), load it into an `*x509.CertPool`, and install it as the downstream listener's `ClientCAs` — so an operator configuring `require_client_certificate: true` with `common_tls_context.validation_context_sds_secret_config` gets mandatory mTLS whose trust anchor is delivered dynamically by an SDS management server, IDENTICAL to the reference. The genuinely NEW production work is narrow and mirrors the landed server-cert path exactly: (a) an applier arm that parses the `validation_context` oneof of a `tls.v3.Secret` into a `*x509.CertPool`; (b) a provider method that fetches it (the current `FetchInitialCertificate` returns `*stdtls.Certificate` — a cert, not a CA pool — so the provider seam widens by one parallel method, §2.5); (c) the reject lift + the `ClientCAs` apply-point wiring (relaxing the `require_client_certificate` INLINE-trusted_ca precondition to accept an SDS-delivered CA, §2.6).

The delivery is a complete, testable slice: a dynamic mTLS trust anchor, resolved at listener construction over the landed SDS stream, proven cleanly cross-side by a client-cert handshake against an SDS-served CA.

### 1.2 What phase 65 does NOT deliver (forward to §8)

NO **upstream** SDS (validating an upstream SERVER cert via SDS — the `NewUpstreamConfig` `validation_context` at `config.go:138-141` is a SEPARATE path; upstream SDS stays deferred, §8). NO `combined_validation_context` (`default_validation_context` inline + an SDS CA — the sibling reject at `config.go:229-230` STAYS loud, §2.7). NO SDS **rotation** (the provider is INITIAL-FETCH only, `provider.go:13` — no watch/re-push loop; rotation stays deferred). NO CDS/EDS/LDS/RDS/ADS/Delta-xDS/RTDS (each a whole new discovery type + applier — §8). NO `require_client_certificate: false` OPTIONAL-mTLS-with-SDS-CA path (phase 65 scopes to MANDATORY mTLS mirroring phase 16 — §2.4/D-SDSVC-REQUIRE-SCOPE; the optional/verify-if-present variant defers). NO new stat subsystem — the SDS fetch lifecycle counters (`sds.*`, phase 60.2) are anticipated REUSED (§2.10).

### 1.3 Phase-done as the THIRD xDS-family row landing (family STAYS OPEN)

Row 65 is the THIRD xDS-family row (after phase 60.1 the SDS substrate + phase 60.2 the server-cert wiring) and the SECOND SDS **resource type**. After phase 65 phase-done the family STAYS OPEN — the deferred candidates in §8 remain (SDS rotation + upstream SDS + `combined_validation_context` + CDS/EDS + LDS/RDS + ADS + Delta xDS + RTDS + reconnection-backoff/`initial_fetch_timeout` edges + google_grpc transport), so the sentinel check-(2) still prints the xDS sentence ⇒ the loop continues.

### 1.4 ADR-0045 split readiness — anticipated a SINGLE FLAT ROW (escape-valve armable) *(self-answered; SPEC confirms)*

Anticipated a SINGLE FLAT ROW (~10–16 tasks: the `validation_context` applier arm + its `*x509.CertPool` return path through `fetchSecret`/`applyResponse` [or a parallel chain, §2.5] + the `FetchInitialValidationContext` provider method + the boot-side `xdsgrpc` adapter wiring + the reject lift + the `ClientCAs` apply-point + the `require_client_certificate` precondition relax + applier/config unit tests + the fuzz seeds + the fixture + the doc/BEHAVIOR_CONTRACT edits + verify + ADR-0286). This is a touch heavier than phase 64 (the provider seam widens by a method + one applier arm + apply-point wiring), so it sits nearer the ADR-0045 `~15` ceiling. There is a NATURAL split shape IF the SPEC's task count surprises upward: **65.1** the `internal/xds` applier + provider method (unit-proven, no differential — mirroring the phase-60.1 substrate leg) / **65.2** the `internal/tls` reject-lift + `ClientCAs` wiring + the mTLS fixture (mirroring phase-60.2). The escape valve is documented ARMABLE and re-armed only if the SPEC judges the two-package surface would strand a leg. Anticipated: SINGLE FLAT ROW (the substrate — the SDS discovery machine — is ALREADY landed, so unlike phase 60 there is no substrate to BUILD; both the applier and the wiring ride the landed stream).

### 1.5 Seed-stub alignment + package placement — ALL edits in EXISTING files/packages, ZERO new packages

- Production reject-lift + `ClientCAs` wiring: `internal/tls/config.go` `commonTLSContextToConfig` (the reject at `:227-228`; the apply-point near the `require_client_certificate` block `:67-79` + the SDS-fetch block `:206-223`).
- Production applier: `internal/xds/secret.go` — a NEW `parseValidationSecret` (or a generalized `parseSecret`) reading `sec.GetValidationContext()` → `*tlsv3.CertificateValidationContext` → `.GetTrustedCa()` via the EXISTING `dataSourceBytes` (`secret.go:27-48`) → `*x509.CertPool` (mirroring `loadTrustedCAPool`, `internal/tls/config.go:163-173`, but duplicated in `internal/xds` per the 60.2 cycle guard — `internal/xds` must NOT import `internal/tls`, §3.5). Symbol names GREP-collision-checked at SPEC (`reference_spec_drafted_identifier_collision_check`).
- Production provider method: `internal/xds/provider.go` — a NEW `FetchInitialValidationContext(ctx, secretName) (*x509.CertPool, error)` on `SecretProvider`/`Provider` (parallel to `FetchInitialCertificate`, `provider.go:14-16`/`47-75`), plus the boot-side `internal/xds/xdsgrpc` adapter wiring.
- Production stream path: `internal/xds/stream.go` — either a parallel `fetchValidationSecret`/`applyValidationResponse` OR a generalization of `fetchSecret`/`applyResponse` (`stream.go:38-95`) to carry a `*x509.CertPool` alongside/instead of the `*stdtls.Certificate` (D-SDSVC-PROVIDER, §2.5).
- Fuzz SEEDS: `internal/xds/fuzz_test.go` `FuzzDiscoveryResponseParse` (`:71`, a `validation_context` Secret seed) + `internal/tls/fuzz_test.go` `FuzzTLSContextParse` (`:24`, an SDS-`validation_context` config seed) — SEEDS, NOT new fuzzers (§2.9).
- Fixture: anticipated ONE new `test/fixtures/NNNN-xds-sds-validation-context` dir (RE-DERIVE the next-free number at IMPL — `0107` is the current numeric tail; `0108` anticipated). Cloned from `0103-xds-sds-server-cert` (the SDS-fixture template) crossed with the `0018-http-rbac` `require_client_certificate` machinery.
- Docs: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (TLS/SDS section) + ROADMAP/STATE/DECISIONS.

ZERO new packages. ZERO new modules.

### 1.6 No prebrainstorm-notes branch

No off-master prebrainstorm-notes branch exists for this subject. `SDS validation_context` is a recorded deferred candidate — named EXPLICITLY in the xDS family's live deferred sentence (`"SDS validation_context/upstream SDS"`, ROADMAP §xDS) — not a stashed WIP.

### 1.7 Phase 65's relationship to the existing seams (a reject lift + a second SDS resource type — the SAME shape as phase 60.2, a DIFFERENT oneof arm)

Phase 65 is architecturally the CLEANEST possible xDS increment: it introduces NOTHING new to the SotW discovery machine. The wire dance — the initial `DiscoveryRequest`, `Recv`, ACK/NACK (`stream.go:38-95`), the `initial_fetch_timeout` bound + classified-error → counter (`provider.go:47-75`), the `ParseSDSConfig` envelope parse (`config.go` `ParseSDSConfig`, provider-neutral) — is ALL reused verbatim. The ONLY resource-type-specific pieces in the landed code are (a) `parseSecret`'s hardwired `GetTlsCertificate()` arm (`secret.go:63-66`, which explicitly rejects any other oneof) and (b) the `*stdtls.Certificate` return type threaded through `fetchSecret`/`applyResponse`/`FetchInitialCertificate`. Phase 65 adds a PARALLEL resource-type arm (the `validation_context` oneof → `*x509.CertPool`) alongside these, and lifts the one downstream `tls` reject that blocked it. The central design decisions are the reference's SDS-`validation_context` serve+apply semantics (D-SDSVC-REFSERVE / D-SDSVC-RESOURCE) and the two envoy-go seam shapes: the provider/stream generalization (D-SDSVC-PROVIDER) and the `ClientCAs` apply-point + `require_client_certificate` interaction (D-SDSVC-APPLY-POINT). Everything else — the discovery stream, ACK/NACK, timeout, `ParseSDSConfig`, both existing appliers — is UNCHANGED.

**Contrast with phase 64:** phase 64 was a pure-additive numeric knob (a scalar field + a truncation helper). Phase 65 is heavier: it widens the SDS provider seam by a method and adds a second applier + apply-point wiring across TWO packages (`internal/xds` + `internal/tls`). Framed honestly so the SPEC/PLAN does not mistake it for a phase-64-sized pure-additive row — but it is STILL a bounded single-flat-row increment because the discovery substrate (the expensive part, built at phase 60.1) is already landed.

---

## 2. Design decisions

### 2.1 Row + subject confirmation: the xDS family continues with SDS `validation_context` *(SELF-PICKED per the standing directive → phase 65 row registered)*

The FIRST decision, made AUTONOMOUSLY (no human pick) per the 2026-07-12 standing directive. Picked as the **smallest cleanly-differential-provable candidate whose ENTIRE substrate is confirmed-landed** from a full source read of the three open families' landed seams + reject surfaces this session (§11 — a subagent-driven reconnaissance sizing xDS/SDS, HTTP/3, and Observability/tracing, then RE-DERIVED by the controller against master `6f46481d`). Row 65 registers `in-progress` AT this BRAINSTORM commit per the ROADMAP §Schema invariant.

**Why SDS `validation_context` is the defensible pick:** (1) its provability is **UNCONDITIONAL** — an mTLS client-cert handshake against an SDS-served CA either succeeds (request flows → 200) or fails (handshake rejected → `ssl.fail_verify_*`), directly observable cross-side, unlike a tracing-metadata tag whose provability is conditional on machinery envoy-go lacks (§2.1-rejected); (2) its ENTIRE substrate is CONFIRMED-LANDED — the SDS discovery-stream client + ACK/NACK + `initial_fetch_timeout` (phase 60.2, `stream.go`/`provider.go`), the static-mTLS `ClientCAs` path + `loadTrustedCAPool` (phase 16, `config.go:67-79`/`:163-173`), the driver-owned SDS gRPC server (`test/helpers/sdsserver`, serves a `Secret` — extend to the `validation_context` arm), the `0103-xds-sds-server-cert` fixture template, AND the client-cert-driving harness (`0018-http-rbac` is a passing differential fixture with `require_client_certificate`) — so NO new harness/discovery infrastructure is needed; (3) it rides the FRESHEST landed seam (phase 60.2, 2026-07-13 — two days old) and is the canonical "next SDS resource type," directly advancing the xDS family's explicitly-next-named deferred candidate; (4) the genuinely-new production code is bounded (a `validation_context` applier arm + a parallel provider method + the reject-lift/apply-point wiring). The ONE genuine subtlety is the `require_client_certificate` precondition (it currently requires an INLINE `trusted_ca`; with SDS the CA arrives dynamically — the precondition relaxes, §2.6).

**Rejected alternatives (recorded per the standing directive; each SIZED against source this session — §11):**
- **tracing `custom_tags` `metadata` type** (`internal/tracing/config.go:214-215`) — the reconnaissance's initial recommendation (the literal fourth arm in the same `parseCustomTags` switch phases 62/63 extended). REJECTED because its provability is CONDITIONAL, not unconditional: `ResolveCustomTags` threads only a `headerLookup` (`resolve.go:14`), envoy-go has NO `MetadataKey` path-traversal and NO dynamic-metadata accessor at the resolve seam (confirmed at the phase-64 full read), so honoring `metadata` needs NEW traversal + metadata-plumbing machinery (a metadata-producing filter for REQUEST metadata, or route/cluster/host static-metadata plumbing) — i.e. the substrate is NOT landed. Phase 64 explicitly deferred it as "the LAST and biggest `custom_tag` source." STAYS deferred as the SOLE remaining `custom_tags` departure.
- **HTTP/3 QUIC numeric knobs** (`idle_timeout` → `quic.Config.MaxIdleTimeout`, the smallest to WIRE — `internal/listener/quic.go:37`/`:145` construct `quic.Config{}` EMPTY) — REJECTED because it is NOT cleanly differential-provable with the landed GET-only H3 harness (phase 61.3): `idle_timeout`/`max_concurrent_streams` behavior needs wall-clock idle observation or a multi-stream H3 client, both NEW harness surface. Deferred until the H3 harness gains idle/multi-stream probing.
- **SDS rotation** — the provider is INITIAL-FETCH only (`provider.go:13`); rotation needs a watch/re-push loop (a new discovery lifecycle). Larger; deferred.
- **upstream SDS** (SDS for an upstream SERVER cert / validation_context) — a SEPARATE path (`NewUpstreamConfig`, `config.go:118-156`, whose `commonTLSContextToConfig` runs with `provider == nil`, `config.go:143`). Honoring it needs the provider threaded into the upstream/cluster build path — a different, larger wiring. Deferred (the "upstream SDS" half of the deferred-sentence candidate STAYS after this row narrows the downstream half at the IMPL, §9).
- **`combined_validation_context`** (`config.go:229-230`) — an inline `default_validation_context` merged with an SDS-delivered CA; needs the merge semantics on top of the plain SDS path. Deferred (a natural follow-on ONCE plain SDS-`validation_context` lands).
- **CDS/EDS/LDS/RDS/ADS/Delta-xDS/RTDS** — each a whole new discovery TYPE + resource applier + config-swap machinery. The largest xDS follow-ons. Deferred.
- **google_grpc transport** — a second gRPC client transport alongside `envoy_grpc`. Medium; deferred.
- **OTLP-metrics stats sink / `spawn_upstream_span` / `http_service` / force-trace / OTel `sampler`/`resource_detectors`** (Observability follow-ons) — each larger than a second SDS resource type (a whole new sink / span / transport / subsystem). Deferred.
- **Opening a new family** (gRPC / Runtime / WASM never-opened; Operational-tooling OPEN) — the standing directive says smallest-defensible-first, and the xDS SDS seam holds a cheap, fully-landed-substrate candidate (`validation_context`), so smallest-first keeps us on the landed SDS engine. Deferred.

### 2.2 Scope: DOWNSTREAM `validation_context_sds_secret_config` ONLY; the sibling rejects STAY *(self-answered; the incremental-lift precedent)*

Phase 65 lifts EXACTLY ONE reject (`config.go:227-228`, `ValidationContextSdsSecretConfig`), and ONLY on the DOWNSTREAM side (`side == "downstream"` — the upstream/QUIC-nil-provider callers keep the byte-identical reject, mirroring the phase-60.2 `tls_certificate` SDS scoping at `config.go:208-210`). The sibling rejects STAY loud with their existing distinct substrings: `combined_validation_context` (`config.go:229-230`), `custom_validator_config` (`config.go:234-235`), `match_typed_subject_alt_names` (`config.go:237-238`), `verify_certificate_hash`/`verify_certificate_spki` (`config.go:240-244`), and the upstream `tls_certificate_sds_secret_configs` reject (`config.go:208-209`). This mirrors the project's landed incremental posture (the one-arm-per-row cadence of phases 60/62/63/64). A downstream-`validation_context`-SDS-only slice is a complete, useful, deterministic capability (a dynamic mTLS trust anchor), and the SPEC probe confirms the reference serves+applies it identically (D-SDSVC-REFSERVE).

### 2.3 The discovery machine is UNCHANGED: a parallel resource-type arm feeding a `*x509.CertPool` — ONE stream, TWO resource types *(self-answered; the SotW seam is landed)*

The SotW fetch (`fetchSecret`, `stream.go:38-80`) + ACK/NACK + `initial_fetch_timeout` bound (`provider.go:47-75`) are reused verbatim. The new resource-type arm parses the `validation_context` oneof of the SAME `tls.v3.Secret` (the wire type URL is generic, `secret.go:17-19`), reading `CertificateValidationContext.trusted_ca` via the EXISTING `dataSourceBytes` (`secret.go:27-48`) into an `*x509.CertPool` — so `ParseSDSConfig`, the DiscoveryRequest/Response dance, the timeout machinery, and the existing cert applier are UNTOUCHED; only a parallel parse arm + a parallel return type are added. A listener that ALSO carries an SDS `tls_certificate` (server cert) AND an SDS `validation_context` (client-CA) drives TWO independent SotW fetches (the reference issues one SDS stream per `SdsSecretConfig`; envoy-go's per-config `FetchInitial*` mirrors that) — the SPEC notes this compose-two-SDS-secrets edge (anticipated: two independent `FetchInitial*` calls in `commonTLSContextToConfig`, one per secret) but it needs no new machinery.

### 2.4 Reference SDS-`validation_context` semantics — SERVE + APPLY + REQUIRE-scope (proto + Envoy source) *(self-answered SHAPE; SPEC probes to PIN)*

`CommonTlsContext.validation_context_sds_secret_config` is a `SdsSecretConfig` (the SAME envelope as `tls_certificate_sds_secret_configs[]`, singular here). The anticipated reference behavior:
- **D-SDSVC-REFSERVE** — the management server serves a `tls.v3.Secret` whose oneof arm is `validation_context` (a `CertificateValidationContext` carrying `trusted_ca`); the reference loads it into the downstream trust store and validates presented client certs against it. The central provability probe. ONE fresh-container probe against `envoyproxy/envoy:contrib-v1.37.2` with an SDS-served CA + a client presenting a CA-signed cert, observing the handshake completes (200) vs a WRONG-CA client cert (handshake rejected). §1.1.
- **D-SDSVC-RESOURCE** — the served resource shape: `Secret{name, validation_context: CertificateValidationContext{trusted_ca: <DataSource>}}`. Confirm the wire shape (the `Secret_ValidationContext` oneof, `tlsv3.Secret`). PROBE via the `sdsserver` helper's served bytes.
- **D-SDSVC-REQUIRE-SCOPE** — the interaction with `require_client_certificate`: the reference treats `validation_context` (the trust store) and `require_client_certificate` (client-cert MANDATORY vs optional) as ORTHOGONAL. Phase 65 SCOPES to `require_client_certificate: true` (mandatory mTLS) mirroring the phase-16 static path; `require_client_certificate: false` (verify-if-presented) DEFERS. PROBE to confirm the mandatory-mTLS path is the reference default when both are set.
- **D-SDSVC-FETCHTIMEOUT** — the SDS `validation_context` fetch reuses the phase-60.2 `initial_fetch_timeout` machinery (default 15s; on timeout/mgmt-unreachable a classified error → boot-FAIL, the documented envoy-go DEPARTURE from the reference's serve-anyway, `provider.go:44-46`/`65-67`). Anticipated: identical bound + the `sds.init_fetch_timeout` counter reused. PROBE for confirmation.
- **D-SDSVC-CVC-REJECT** — the applier REJECTS the unsupported `CertificateValidationContext` sub-fields (`match_typed_subject_alt_names`, `custom_validator_config`, `verify_certificate_hash`/`verify_certificate_spki`, `crl`, `require_signed_certificate_timestamp`, …) that the INLINE static path already rejects at `config.go:234-244` — so the SDS-delivered CVC is held to the SAME support surface as the inline one (`reference_strict_reject_sibling_typeurl_gap` — lifting the SDS envelope is not a licence to silently accept CVC sub-fields envoy-go can't honor). RE-DERIVE the inline reject roster + mirror it in the `internal/xds` applier.

The SPEC live-probes each arm against `envoyproxy/envoy:contrib-v1.37.2` (fresh container per arm, `reference_probe_fresh_container_per_arm`; shared bridge + reachable SDS server, `reference_docker_probe_bridge_network`/`reference_host_gateway_ip_docker_desktop`), verifying the handshake decision ACTUALLY exercised (not a vacuous no-cert-presented capture — `reference_deliberate_break_wrong_assertion`).

### 2.5 The provider/stream seam shape — a parallel method + a `*x509.CertPool` return *(D-SDSVC-PROVIDER; SPEC pins)*

The landed chain returns `*stdtls.Certificate` end-to-end (`parseSecret` → `applyResponse` → `fetchSecret` → `FetchInitialCertificate`). A CA bundle is an `*x509.CertPool`, not a cert. Two shapes:
- **Option A (parallel chain) — LEAN:** add `parseValidationSecret(resource, wantName, baseDir) (*x509.CertPool, error)` + `applyValidationResponse` + `fetchValidationSecret` + `FetchInitialValidationContext(ctx, name) (*x509.CertPool, error)`. **PRO:** the landed `*stdtls.Certificate` chain is UNTOUCHED (zero regression risk to phase 60.2); each arm is small + independently unit-testable. **CON:** duplicates the ~40-line ACK/NACK dance (`fetchSecret`) — a DRY cost.
- **Option B (generalize the chain):** parameterize `fetchSecret`/`applyResponse` over a parse callback (Go generics on a free function: `fetchResource[T](stream, node, name, typeURL, apply func(*anypb.Any, string, string) (T, error)) (T, error)`), so the ACK/NACK dance is written once and both resource types flow through it. **PRO:** DRY. **CON:** touches the landed phase-60.2 chain (a small regression surface); the `FetchInitial*` provider methods still differ by return type.

**The decision** is D-SDSVC-PROVIDER: the SPEC weighs A vs B against the DRY-cost-vs-regression-risk tradeoff (LEAN Option A at the BRAINSTORM — the landed chain is load-bearing for phase 60.2's passing `0103` differential, and duplicating a bounded ACK/NACK dance is cheaper than risking a regression there; but B is attractive if the SPEC judges the duplication substantial). Pin the exact method/function names (GREP-collision-checked, `reference_spec_drafted_identifier_collision_check`) + signatures + the boot-side `xdsgrpc` adapter wiring.

### 2.6 The `ClientCAs` apply-point + the `require_client_certificate` precondition relax *(D-SDSVC-APPLY-POINT; SPEC pins)*

The static-mTLS block (`config.go:67-79`) currently: (i) gates on `require_client_certificate == true`, (ii) requires an INLINE `validation_context.trusted_ca` (ERRORS if absent, `:70-72`), (iii) loads it → `cfg.ClientCAs` + `cfg.ClientAuth = RequireAndVerifyClientCert`. With an SDS-delivered `validation_context`, the trusted_ca is NOT inline — it arrives via the SDS fetch. So the apply-point must:
- Fetch the CA pool in `commonTLSContextToConfig` when `ValidationContextSdsSecretConfig` is present + `side == "downstream"` + a live provider (mirroring the `tls_certificate` SDS block, `config.go:206-223`), holding the `*x509.CertPool`.
- RELAX the `require_client_certificate` precondition (ii): accept EITHER an inline `trusted_ca` OR an SDS-delivered pool. Install whichever into `cfg.ClientCAs` + set `ClientAuth = RequireAndVerifyClientCert`.
- Preserve the existing error when `require_client_certificate == true` but NEITHER an inline `trusted_ca` NOR an SDS `validation_context` is present.

The SPEC RE-DERIVES the exact block structure + pins whether the CA-pool install lives in `NewDownstreamConfig` (`config.go:33-81`, where the `require_client_certificate` block is) or is threaded out of `commonTLSContextToConfig` (where the SDS fetch for the server cert already lives) — the two functions must agree on WHERE the fetched pool is held (anticipated: `commonTLSContextToConfig` returns the pool via a field or the config, and `NewDownstreamConfig` installs `ClientCAs`, mirroring how `sdsCert` flows to `cfg.Certificates` at `config.go:206-223`/`:250-252`). D-SDSVC-APPLY-POINT.

### 2.7 The reject narrows; envoy-go and the reference now AGREE on downstream SDS-`validation_context` *(self-answered; ADR-0080)*

The downstream `validation_context_sds_secret_config` reject is LIFTED (the departure NARROWS — envoy-go now HONORS the knob, matching the reference). The sibling rejects (`combined_validation_context`, `custom_validator_config`, `match_typed_subject_alt_names`, `verify_certificate_hash`/`spki`, upstream `tls_certificate_sds_secret_configs`, and the CVC sub-field rejects mirrored into the applier, §2.4) STAY loud with their distinct substrings (`reference_strict_reject_sibling_typeurl_gap` — lifting one reject arm is an explicit per-arm change, not a fall-through). NO NEW structural reject is added beyond mirroring the inline CVC support surface into the SDS applier (§2.4/D-SDSVC-CVC-REJECT). The `ParseSDSConfig` envelope validation (arms 1-4,8,9, `config.go`) already runs for the `SdsSecretConfig` — REUSED unchanged.

### 2.8 Fixture posture: anticipated ONE new fixture (an SDS-served CA + a mandatory-mTLS client-cert handshake) *(self-answered direction; SPEC pins D-SDSVC-FIXTURE)*

The mTLS handshake decision IS an observable cross-side property (connection completes → request → status; or handshake rejected → `ssl.fail_verify_*`), so SDS-`validation_context` IS cleanly differential-provable. A NEW `test/fixtures/NNNN-xds-sds-validation-context` dir configures a downstream listener with `require_client_certificate: true` + `common_tls_context.validation_context_sds_secret_config` pointing at the driver-owned `sdsserver` (extended to serve a `validation_context` Secret carrying a CA bundle); the runner drives a client presenting a cert SIGNED by that CA and asserts the request completes cross-side (status + body), with a NEGATIVE arm (a client cert NOT signed by the served CA → handshake rejected / request fails). Per the dispatch constraint (`reference_differential_fixture_dispatch_constraint` — one dir = one runner branch; do NOT mutate `0103` [the server-cert SDS baseline] or `0018` [the static-mTLS/RBAC baseline]), it is a NEW dir. Cloned from `0103-xds-sds-server-cert` (the SDS driver-owned-server template) crossed with `0018-http-rbac`'s `require_client_certificate` + client-cert machinery.

- **D-SDSVC-FIXTURE** — the SDS-served-CA + client-cert fixture is the ANCHOR proof. CONFIRM the differential runner can PRESENT a client cert in the mTLS handshake (the `0018-http-rbac` precedent — a passing fixture with `require_client_certificate` — is the evidence the harness supports it; RE-DERIVE the exact client-cert mechanism at SPEC). Cross-side equality on the request outcome (positive: 200; negative: connection/verify failure + `ssl.fail_verify_error`). The SDS server is a driver-owned gRPC `test/helpers` server (`reference_differential_grpc_receiver_driver_owned`), NOT a runner BackendKind. Break-prove the assertion is live (`reference_differential_break_protocol_count1` — `-count=1`; `-run 'TestDifferential/<NNNN>-xds-sds-validation-context'`, NEVER bare — `reference_differential_run_selector`); the natural break is serving a WRONG CA (the positive-arm client cert then fails to verify) — but confirm WHICH assertion fires (`reference_deliberate_break_wrong_assertion`).
- **D-SDSVC-NEGATIVE** — the negative arm (unsigned client cert → verify failure) proves the SDS-served CA is actually the trust anchor (not a vacuous accept-all). The SPEC weighs whether the negative arm is a second fixture dir, a second scenario within the one dir (the `0018` multi-scenario precedent), or a driver-side sub-assertion. Anticipated: one dir, positive-primary + a negative sub-assertion.

Anticipated: fixtures **109 → 110** — SPEC pins (and re-derives the next-free number; `0107` is the current numeric tail, `0108` anticipated). **Harness note:** the fixture drives H1/H2 over TCP+TLS — NOT the H3/QUIC path (QUIC carries no SDS, `config.go:87-88`) — so `reference_differential_http_expectations_tcp_only` does not bite for the QUIC reason; the mTLS handshake + status assertion live in the runner's TLS-client Drive path (RE-DERIVE the exact seam at SPEC).

### 2.9 Fuzz posture: SEEDS to the EXISTING fuzzers — NO new fuzzer *(self-answered; count stays 55 → SPEC confirms D-SDSVC-FUZZSEED)*

The `validation_context` Secret parse is reached via `parseSecret`/`applyResponse` off a `DiscoveryResponse`, fuzzed by `FuzzDiscoveryResponseParse` (`internal/xds/fuzz_test.go:71`); the SDS-`validation_context` CONFIG parse is reached via `commonTLSContextToConfig`, fuzzed by `FuzzTLSContextParse` (`internal/tls/fuzz_test.go:24`). The new arms are exercised by ADDING seeds (a `DiscoveryResponse` carrying a `validation_context` Secret; a `CommonTlsContext` with a `validation_context_sds_secret_config`) — NOT new fuzzers. Fuzzer count STAYS **55** (`reference_fuzzer_count_docs_drift`: reconcile the documented running total against actual `^func Fuzz` before AND after — the count must NOT move). SPEC confirms D-SDSVC-FUZZSEED.

### 2.10 Stat surface hypothesis: +0 (reuse the phase-60.2 `sds.*` lifecycle counters) *(self-answered; SPEC confirms D-SDSVC-STATS)*

The SDS fetch lifecycle counters (`sds.update_attempt`/`update_success`/`update_failure`/`update_rejected`/`init_fetch_timeout` — the `SDSStats` set, `provider.go:48-73`) are provider-lifecycle counters, NOT resource-type-specific. A `validation_context` fetch through `FetchInitialValidationContext` is anticipated to REUSE the SAME `SDSStats` (incrementing the same counters), so no new stat is registered. Anticipated stat surface **1201 (+0)**, UNCHANGED. The SPEC confirms whether the counters are per-provider (reused) or whether a per-resource-type dimension is warranted (anticipated NO — a small +N only if the SPEC finds the reference distinguishes them; then recorded).

---

## 3. Framework-survey result — a reject lift + a second SDS resource applier; ZERO new packages/modules (65 anticipated)

### 3.1 Framework: a parallel resource-type arm + a provider method + apply-point wiring (no new discovery machine, no new seam)

Phase 65 introduces NOTHING structurally new to the SDS discovery engine: no new discovery type, no new stream, no new config-swap machinery, no `ParseSDSConfig` change, no ACK/NACK change, no timeout change. It adds one parallel applier arm (`validation_context` → `*x509.CertPool`), one parallel provider method (`FetchInitialValidationContext`), and the `internal/tls` reject-lift + `ClientCAs` apply-point wiring. The discovery stream, ACK/NACK, `initial_fetch_timeout`, `ParseSDSConfig`, and the existing cert applier are UNCHANGED.

### 3.2 NEW packages: NONE

All edits land in `internal/xds` (`secret.go` applier + `provider.go` method + `stream.go` fetch arm + the `xdsgrpc` boot-side adapter wiring, all existing) + `internal/tls` (`config.go` reject-lift + apply-point) + `test/helpers/sdsserver` (extend to serve a `validation_context` Secret) + `test/fixtures` + `docs/`. ZERO new packages.

### 3.3 go.mod modules: NONE

`GetValidationContextType()`/`CommonTlsContext_ValidationContextSdsSecretConfig`/`CertificateValidationContext`/`Secret_ValidationContext` are all reachable via the resolved `github.com/envoyproxy/go-control-plane v1.32.4` TLS + secret protos (already imported as `tlsv3` in both `internal/xds/secret.go:10` and `internal/tls`). `*x509.CertPool` is stdlib (already used at `config.go:163-173`). No new module import. `go mod tidy -diff` anticipated EMPTY (modules STAY **2**).

### 3.4 REUSES

- **phase 60.1/60.2** the SDS discovery engine: the SotW `fetchSecret`/`applyResponse` ACK/NACK dance (`stream.go`), the `Provider`/`SecretProvider`/`StreamOpener` seams + `initial_fetch_timeout` bound + `SDSStats` counters (`provider.go`), `ParseSDSConfig` (the `SdsSecretConfig` envelope parse), `secretTypeURL()` + `dataSourceBytes` (`secret.go`), the boot-side `xdsgrpc` adapter, the `0103-xds-sds-server-cert` fixture template, `test/helpers/sdsserver`, and `FuzzDiscoveryResponseParse`.
- **phase 16 (ADR-0147)** the static-mTLS `ClientCAs` install: `loadTrustedCAPool` (`config.go:163-173`), the `require_client_certificate` block (`config.go:67-79`), and the `0018-http-rbac` `require_client_certificate` + client-cert differential machinery.
- **phase 03** the `commonTLSContextToConfig` reject roster + the CVC sub-field reject set (`config.go:234-244`) — mirrored into the SDS applier (§2.4).

### 3.5 The 60.2 cycle guard STANDS

`internal/xds` must NOT import `internal/tls` (the phase-60.2 `reference_xds_config_seam_transitive_cycle_guard` — `internal/xds` is imported by `internal/tls`, so the reverse edge would cycle). The `validation_context` applier's CA-pool build (`x509.NewCertPool().AppendCertsFromPEM`) is therefore duplicated in `internal/xds` (mirroring how `dataSourceBytes` deliberately duplicates `internal/tls.loadDataSource`, `secret.go:21-26`) rather than calling `internal/tls.loadTrustedCAPool`. The SPEC verifies the import graph with `go list -deps` (no `...`) per the cycle-guard reference.

---

## 4. Bootstrap-level applicability — a PER-LISTENER downstream TLS sub-field (NOT bootstrap `stats_sinks[]`)

`validation_context_sds_secret_config` is a PER-LISTENER downstream HCM/filter-chain `transport_socket` → `DownstreamTlsContext.common_tls_context` sub-field, parsed by `commonTLSContextToConfig` when the listener's filter chain is built (the phase-03/16/60.2 home). No bootstrap change; the lift lands INSIDE `commonTLSContextToConfig`. The SDS management server is reached via the bootstrap `SdsSecretConfig.sds_config` (an `ApiConfigSource` gRPC cluster — the phase-60.2 wiring, REUSED). The fixture configures `validation_context_sds_secret_config` on the listener's downstream TLS block.

---

## 5. Stat surface hypothesis — +0 (65)

### 5.1 Stat names (SPEC confirms)

NONE anticipated — the SDS fetch reuses the phase-60.2 `sds.*` lifecycle counters (§2.10). The mTLS verify outcome surfaces via the EXISTING `ssl.*` downstream TLS counters (phase 03/16), not a new stat.

### 5.2 envoy-go-strict departure flags

The downstream `validation_context_sds_secret_config` reject is LIFTED (the departure NARROWS — envoy-go now HONORS the knob). The boot-FAIL-on-fetch-timeout DEPARTURE (phase 60.2, `provider.go:44-46`) EXTENDS to the `validation_context` fetch unchanged. No new stat, no new flag; a parse+apply behavior change recorded in BEHAVIOR_CONTRACT. The sibling SDS/CVC rejects (§2.2/§2.7) STAY.

### 5.3 Anticipated surface arithmetic

Stat surface **1201 → 1201 (+0)** (SPEC confirms; a small +N only if the SPEC finds a per-resource-type counter is warranted, §2.10).

---

## 6. Edit-site enumeration — RE-DERIVED this session (SPEC re-derives + pins D-SDSVC-REFSERVE / D-SDSVC-PROVIDER / D-SDSVC-APPLY-POINT / D-SDSVC-FIXTURE)

Each `file:line` RE-DERIVED against master `6f46481d` this session (`feedback_brief_citations_not_evidence`); the SPEC re-derives again.

**Production — `internal/tls/config.go`:**
1. **The reject lift** — replace the downstream case of the reject (`config.go:227-228`, `CommonTlsContext_ValidationContextSdsSecretConfig`) with: when `side == "downstream"` + a live provider → `xds.ParseSDSConfig` the `SdsSecretConfig` + `provider.FetchInitialValidationContext(...)` → hold the `*x509.CertPool`; the upstream/nil-provider path keeps the byte-identical reject (mirroring `config.go:208-210`). The sibling `combined_validation_context` + CVC rejects UNCHANGED. [EDIT]
2. **The `ClientCAs` apply-point** — near `NewDownstreamConfig`'s `require_client_certificate` block (`config.go:67-79`): relax the INLINE-`trusted_ca` precondition to accept the SDS-fetched pool; install it into `cfg.ClientCAs` + `cfg.ClientAuth`. Thread the held pool out of `commonTLSContextToConfig` (as `sdsCert` is, `config.go:206-223`/`:250-252`). D-SDSVC-APPLY-POINT. [EDIT]

**Production — `internal/xds`:**
3. **The applier arm** — `secret.go`: a NEW `parseValidationSecret` (or a generalized `parseSecret`) reading `sec.GetValidationContext()` → `.GetTrustedCa()` via `dataSourceBytes` → `*x509.CertPool`; mirror the inline CVC sub-field rejects (§2.4/D-SDSVC-CVC-REJECT). [ADD]
4. **The stream fetch arm** — `stream.go`: a parallel `fetchValidationSecret`/`applyValidationResponse` OR a generalization of `fetchSecret`/`applyResponse` (`stream.go:38-95`). D-SDSVC-PROVIDER. [ADD/EDIT]
5. **The provider method** — `provider.go`: a NEW `FetchInitialValidationContext(ctx, secretName) (*x509.CertPool, error)` on `SecretProvider` + `Provider` (parallel to `FetchInitialCertificate`, `:14-16`/`:47-75`), reusing the timeout + `SDSStats` machinery. Plus the boot-side `internal/xds/xdsgrpc` adapter wiring. [ADD/EDIT]

**Test / harness:**
6. **`test/helpers/sdsserver/sdsserver.go`** — extend `buildResponse` (`:118-137`) + add a `WithValidationContext` option (parallel to `WithSecret`, `:43`) to serve a `Secret{validation_context: CertificateValidationContext{trusted_ca}}`. [ADD]
7. **`internal/xds/secret_test.go` / `stream_test.go`** — applier unit tests: a valid `validation_context` Secret → a usable pool; an empty/invalid `trusted_ca` → a classified `errValidation`; each unsupported CVC sub-field → its distinct reject. Assert each independent property with `Errorf` (`reference_fatalf_makes_assertions_unreachable`). [ADD]
8. **`internal/tls/config_test.go`** — flip the downstream `validation_context_sds_secret_config` REJECT test to an ACCEPT (with a fake provider) asserting `cfg.ClientCAs != nil` + `ClientAuth == RequireAndVerifyClientCert`; keep the upstream-side reject + the `combined_validation_context` reject tests. [EDIT + ADD]
9. **Fuzz SEEDS** — `internal/xds/fuzz_test.go` `FuzzDiscoveryResponseParse` (`:71`) + `internal/tls/fuzz_test.go` `FuzzTLSContextParse` (`:24`): a `validation_context` Secret seed + an SDS-`validation_context` config seed. [ADD — no new fuzzer]

**Fixture:**
10. **`test/fixtures/NNNN-xds-sds-validation-context`** (new; `0108` anticipated) — a downstream listener with `require_client_certificate: true` + `validation_context_sds_secret_config`; the driver-owned `sdsserver` serves a CA; the runner presents a CA-signed client cert; assert the request completes cross-side + a negative (wrong-CA) arm. [ADD]

**BEHAVIOR_CONTRACT (`docs/envoy-go/BEHAVIOR_CONTRACT.md`):**
11. **the TLS/SDS section** — flip downstream `validation_context_sds_secret_config` from "rejected (phase 03)" to "consumed (SDS-delivered downstream mTLS trusted-CA; installed as `ClientCAs`; `require_client_certificate: true` mandatory-mTLS scope; boot-FAIL on fetch timeout; CVC sub-fields held to the inline support surface)"; the sibling SDS/CVC/upstream rejects STAY. SPEC RE-DERIVES the exact line(s). [EDIT]

**ROADMAP / STATE / DECISIONS:**
12. **ROADMAP** — row 65 `in-progress` at this BRAINSTORM (§Schema); the xDS family prose gains a "phase 65 CHARTERED and BRAINSTORMED" sentence. The LIVE xDS deferred sentence is UNCHANGED at this BRAINSTORM (narrowed at the IMPL per the observed convention — request_header left the Observability sentence at `54f52628` [row-63 done], not its BRAINSTORM; §9). [BRAINSTORM: row + prose]
13. **STATE.md** — active-phase header flips to phase 65 BRAINSTORM (this stage). [EDIT]
14. **DECISIONS.md** — ADR-0286 §Context drafts at the SPEC, §Decision/§Consequences at the IMPL (ADR-0044). NOT at this BRAINSTORM. [SPEC/IMPL]

SPEC pins **D-SDSVC-DOCSHAPE** (this full edit-site roster, RE-DERIVED) + **D-SDSVC-REFSERVE/-RESOURCE/-REQUIRE-SCOPE/-FETCHTIMEOUT/-CVC-REJECT** (§2.4) + **D-SDSVC-PROVIDER** (§2.5) + **D-SDSVC-APPLY-POINT** (§2.6) + **D-SDSVC-FIXTURE/-NEGATIVE** (§2.8) + **D-SDSVC-STATS** (§2.10) + **D-SDSVC-FUZZSEED** (§2.9) + **D-SDSVC-SPLIT** (§1.4).

---

## 7. Anticipated ADRs — 1 at the phase-65 IMPL: ADR-0286 (xDS SDS `validation_context`)

ADR-0286 (xDS SDS `validation_context` — lifting the downstream `validation_context_sds_secret_config` reject; the second SDS resource type on the landed SotW discovery machine [D-SDSVC-REFSERVE]; the `validation_context` oneof applier → `*x509.CertPool` [D-SDSVC-RESOURCE/-CVC-REJECT]; the provider/stream seam widening [D-SDSVC-PROVIDER]; the `ClientCAs` apply-point + `require_client_certificate` precondition relax [D-SDSVC-APPLY-POINT]; the mandatory-mTLS scope [D-SDSVC-REQUIRE-SCOPE]; the boot-FAIL-on-timeout DEPARTURE extension [D-SDSVC-FETCHTIMEOUT]). §Context drafted at the SPEC (provenance: the phase-60.1/60.2 SDS engine [ADR-0278/0280] + the phase-16 static-mTLS `ClientCAs` [ADR-0147] + the xDS deferred-candidate record), §Decision/§Consequences at the IMPL per ADR-0044. No separate seam ADR (the phase-60.2/62/63/64 precedent — a single row-scoped ADR; IF the SPEC arms the 65.1/65.2 escape valve, §1.4, then a second leg-scoped ADR mirrors the 60.1/60.2 ADR-0278/0280 pair). Next-free after: **ADR-0287**.

---

## 8. Deferred items

- **upstream SDS** (SDS for an upstream server cert / `validation_context`) — the `NewUpstreamConfig` path with the provider threaded to the cluster build. The other half of the deferred-sentence "SDS validation_context/upstream SDS" candidate; STAYS after this row narrows the downstream half at the IMPL. Carries forward.
- **`combined_validation_context`** (`config.go:229-230`) — an inline `default_validation_context` merged with an SDS CA; the natural follow-on once plain SDS-`validation_context` lands. Carries forward.
- **SDS rotation** — a watch/re-push loop (the provider is INITIAL-FETCH only, `provider.go:13`). Carries forward.
- **`require_client_certificate: false` optional mTLS with an SDS CA** (verify-if-presented) — phase 65 scopes to mandatory mTLS (§2.4/D-SDSVC-REQUIRE-SCOPE). Carries forward (low value).
- **CDS/EDS/LDS/RDS/ADS (muxed)/Delta xDS/RTDS** — each a whole new discovery type. Carries forward.
- **reconnection-backoff / `initial_fetch_timeout` edges** — the SDS-layer has no backoff (transport reconnect is delegated to gRPC's sub-channel state machine, §11); a dedicated backoff/edge row. Carries forward.
- **google_grpc transport** — a second gRPC client transport. Carries forward.
- **The CVC sub-fields** (`match_typed_subject_alt_names`, `custom_validator_config`, `verify_certificate_hash`/`spki`, `crl`) — rejected in BOTH the inline and (per §2.4) the SDS applier; a future CVC-feature row. Carries forward.

After row 65 the xDS family STAYS OPEN (upstream SDS + rotation + CDS/EDS/… remain) ⇒ the sentinel check-(2) STILL prints the xDS sentence ⇒ the loop continues.

---

## 9. Cross-references against prior phases' deferred-items lists — pickup + sentinel maintenance

Phase 65 PICKS UP `SDS validation_context` — named EXPLICITLY in the xDS family's LIVE deferred sentence (`"SDS validation_context/upstream SDS"`, ROADMAP §xDS) and implicit in the phase-60.2 follow-on roster. Unlike phase 64 (whose `max_path_tag_length` was a §8-tier candidate NOT in any live sentence), phase 65's candidate IS in the live xDS sentence — so this row DOES narrow it, **but at the IMPL, not this BRAINSTORM** (the observed convention: `request_header` left the Observability sentence at the phase-63 IMPL commit `54f52628`, NOT at the phase-62 BRAINSTORM; a chartered-but-not-done candidate stays in the sentence). **Sentinel maintenance (at the IMPL):** narrow the xDS sentence's `"SDS validation_context/upstream SDS"` → `"upstream SDS (server-cert + validation_context)"` (dropping the now-CONSUMED downstream `validation_context`, keeping upstream SDS deferred), and re-run the check-(2) grep to CONFIRM it still prints EXACTLY ONE live xDS match with the reduced content (`reference_sentinel_deferred_sentence_live_vs_historical` — the xDS family STAYS OPEN with its other candidates, so the sentinel keeps blocking `stop`). At THIS BRAINSTORM the sentence is UNTOUCHED.

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution

- **D-SDSVC-REFSERVE** — CONFIRM the reference serves + applies a `validation_context` via SDS (a `tls.v3.Secret` with the `validation_context` oneof carrying `trusted_ca`) and validates client certs against it. The central provability probe. ONE fresh-container probe against `envoyproxy/envoy:contrib-v1.37.2` with an SDS-served CA + a client presenting a CA-signed cert (handshake completes) vs a wrong-CA cert (rejected), observed via the handshake outcome + `ssl.fail_verify_*`. If the reference behaves unexpectedly (recall wrong), the SPEC RE-SCOPES. §1.1/§2.4.
- **D-SDSVC-RESOURCE** — the served `Secret` wire shape (`Secret_ValidationContext` oneof, `CertificateValidationContext{trusted_ca}`). §2.4.
- **D-SDSVC-REQUIRE-SCOPE** — the `require_client_certificate` interaction: scope to `true` (mandatory mTLS, anticipated) mirroring phase 16; `false` (verify-if-present) defers. Probe. §2.4.
- **D-SDSVC-FETCHTIMEOUT** — the `initial_fetch_timeout` + boot-FAIL DEPARTURE extends to the `validation_context` fetch unchanged (anticipated). Probe for confirmation. §2.4.
- **D-SDSVC-CVC-REJECT** — the applier mirrors the inline CVC sub-field reject roster (`config.go:234-244`). RE-DERIVE + confirm. §2.4.
- **D-SDSVC-PROVIDER** — the provider/stream seam shape: a parallel chain returning `*x509.CertPool` (Option A, LEAN) vs generalizing `fetchSecret`/`applyResponse` over a parse callback (Option B, DRY). Pin the names (GREP-collision-checked, `reference_spec_drafted_identifier_collision_check`) + signatures + the `xdsgrpc` adapter wiring. §2.5.
- **D-SDSVC-APPLY-POINT** — where the SDS-fetched CA pool installs into `cfg.ClientCAs` + `ClientAuth`; the `require_client_certificate` INLINE-`trusted_ca` precondition relax; the hold-and-thread structure between `commonTLSContextToConfig` and `NewDownstreamConfig`. §2.6.
- **D-SDSVC-FIXTURE** — ONE new fixture (`0108` anticipated; RE-DERIVE): an SDS-served CA + a mandatory-mTLS client-cert handshake, cross-side request-outcome equality. CONFIRM the differential runner presents a client cert (the `0018-http-rbac` precedent; RE-DERIVE the mechanism). New dir (`reference_differential_fixture_dispatch_constraint`); break-prove live (`reference_differential_break_protocol_count1`/`reference_differential_run_selector`/`reference_deliberate_break_wrong_assertion`). Fixtures **109 → 110**. §2.8.
- **D-SDSVC-NEGATIVE** — the wrong-CA negative arm (proves the SDS-served CA is the actual trust anchor): a second dir vs a second scenario vs a driver sub-assertion. §2.8.
- **D-SDSVC-STATS** — reuse the phase-60.2 `sds.*` lifecycle counters (+0, anticipated) vs a per-resource-type counter. RE-DERIVE the `SDSStats` set. §2.10.
- **D-SDSVC-FUZZSEED** — SEEDS to `FuzzDiscoveryResponseParse` + `FuzzTLSContextParse` (NOT new fuzzers); fuzzer count STAYS 55 (`reference_fuzzer_count_docs_drift` — reconcile before AND after). §2.9.
- **D-SDSVC-SPLIT** — the ADR-0045 disposition (SINGLE FLAT ROW anticipated, ~10–16 tasks; the escape valve is a 65.1 `internal/xds` applier + 65.2 `internal/tls` wiring split mirroring 60.1/60.2, re-armed only if the two-package surface would strand a leg). §1.4.

---

## 11. Prior-phase lessons applied

- **`feedback_brief_citations_not_evidence`** — EVERY `file:line` here (the `config.go:227-228` reject, the `config.go:206-223` server-cert SDS template, the `config.go:67-79` static-mTLS block, `secret.go:55-80`/`:63-66` the cert applier, `provider.go:14-16`/`:47-75` the provider seam, `stream.go:38-95` the ACK/NACK dance) was RE-DERIVED from source this session against master `6f46481d` — the controller RE-READ `secret.go`, `provider.go`, `stream.go`, and the two `config.go` regions in full rather than trusting the reconnaissance subagent's citations (a citation is not evidence). Notably CONFIRMED: the static-mTLS `ClientCAs` substrate + `loadTrustedCAPool` are landed (so this is NOT gated on building static mTLS); the SDS wire dance is generic over `tls.v3.Secret`; the `sdsserver` helper serves a `Secret_TlsCertificate` (extend, don't rebuild).
- **`reference_xds_config_seam_transitive_cycle_guard`** — `internal/xds` must NOT import `internal/tls` (the reverse edge cycles); the `validation_context` CA-pool build is DUPLICATED in `internal/xds` (mirroring `dataSourceBytes`'s deliberate duplication of `loadDataSource`, `secret.go:21-26`). The SPEC verifies with `go list -deps` (no `...`). §3.5.
- **`reference_spec_drafted_identifier_collision_check`** — the new applier/provider/stream symbol names (`parseValidationSecret`, `FetchInitialValidationContext`, `fetchValidationSecret`, …) are GREP-checked in `internal/xds` + `internal/tls` before the PLAN adopts them. §2.5/§6.
- **`reference_differential_grpc_receiver_driver_owned`** — the SDS management server is a `test/helpers/sdsserver` driver-owned gRPC server the proxy DIALS, NOT a runner `BackendKind` (BackendKind STAYS 38). §2.8.
- **`reference_probe_fresh_container_per_arm`** + **`reference_envoy_contrib_image_tagging`** — each SPEC probe arm (D-SDSVC-REFSERVE/-RESOURCE/-REQUIRE-SCOPE/-FETCHTIMEOUT) runs on a FRESH `envoyproxy/envoy:contrib-v1.37.2` container. §2.4.
- **`reference_docker_probe_bridge_network`** + **`reference_host_gateway_ip_docker_desktop`** — the SDS probe needs a shared bridge + a reachable SDS server; verify the handshake decision ACTUALLY exercised (not a vacuous no-cert-presented capture). §2.4.
- **`reference_differential_fixture_dispatch_constraint`** — a new fixture dir; do NOT mutate `0103` (the server-cert SDS baseline) or `0018` (the static-mTLS/RBAC baseline). §2.8.
- **`reference_differential_break_protocol_count1`** + **`reference_differential_run_selector`** + **`reference_deliberate_break_wrong_assertion`** — the `0108` assertion break-proof uses `-count=1` + `-run 'TestDifferential/0108-xds-sds-validation-context'` (NEVER bare); the natural break (serve a WRONG CA) must be confirmed to fire the intended assertion, not an earlier abort. §2.8.
- **`reference_fuzzer_count_docs_drift`** — SEEDS, not fuzzers; reconcile the documented running total (55) against actual `^func Fuzz` before AND after — the count must NOT move. §2.9.
- **`reference_sentinel_deferred_sentence_live_vs_historical`** — this row narrows the live xDS sentence AT THE IMPL (not this BRAINSTORM, per the observed convention §9); re-run the check-(2) grep at the IMPL to CONFIRM exactly ONE live xDS match with the reduced content. §9.
- **`reference_strict_reject_sibling_typeurl_gap`** / **ADR-0080** — the sibling SDS/CVC/upstream rejects keep their DISTINCT substrings; lifting downstream `validation_context_sds_secret_config` is an explicit per-arm change (not a fall-through), and the SDS applier mirrors the inline CVC support surface (does not silently accept CVC sub-fields envoy-go can't honor). §2.2/§2.4/§2.7.
- **`reference_fatalf_makes_assertions_unreachable`** — the applier/config unit tests assert each independent property with `Errorf` (not `Fatalf`), so a pool-build failure does not mask the CVC-reject / name-mismatch assertions. §6.
- **`feedback_git_worktrees`** + **`feedback_subagent_worktree_detach`** + **`feedback_subagent_worktree_path_targeting`** — this BRAINSTORM runs in a fresh worktree off master (`.worktrees/phase-65-brainstorm`, branch `phase-65-xds-sds-validation-context`); the controller verifies the main checkout stays clean.

---

## 12. Section closeout

**Settled:** subject (xDS SDS `validation_context`, SELF-PICKED per the standing directive as the smallest CLEANLY-DIFFERENTIAL-PROVABLE candidate whose ENTIRE substrate is confirmed-landed — a SECOND SDS resource type [a downstream mTLS trusted-CA] on the ALREADY-landed phase-60.2 SDS discovery-stream + the ALREADY-landed phase-16 static-mTLS `ClientCAs` path + the ALREADY-landed SDS-serving + client-cert harness — over tracing `metadata` [provability CONDITIONAL on traversal/plumbing machinery envoy-go lacks] and the larger declined alternatives, §2.1); scope (downstream `validation_context_sds_secret_config` lifted; the sibling SDS/CVC/upstream rejects STAY loud with distinct substrings, §2.2/§2.7); the discovery machine (UNCHANGED — a parallel resource-type arm feeding an `*x509.CertPool`; ACK/NACK/timeout/`ParseSDSConfig` reused, §2.3); the reference semantics (serve a `validation_context` Secret; apply as `ClientCAs`; mandatory-mTLS scope; boot-FAIL on fetch timeout; CVC sub-fields held to the inline surface — all anticipated, SPEC-probed, §2.4); the provider/stream seam (a parallel `*x509.CertPool` chain [LEAN] vs a generalization, §2.5); the `ClientCAs` apply-point + `require_client_certificate` precondition relax (§2.6); fixture posture (ONE new fixture — an SDS-served CA + a mandatory-mTLS client-cert handshake, cross-side request-outcome equality + a wrong-CA negative arm; SDS server driver-owned, §2.8); fuzz posture (SEEDS to `FuzzDiscoveryResponseParse` + `FuzzTLSContextParse`, no new fuzzer, §2.9); stat surface (+0, reuse `sds.*`, §2.10); the 60.2 cycle guard (STANDS — duplicate the CA-pool build in `internal/xds`, §3.5); envelope (SINGLE FLAT ROW anticipated, ~10–16 tasks, a 65.1/65.2 escape valve armable — ADR-0286, §1.4). The novel production code is the `validation_context` applier arm + a parallel provider method + the reject-lift/`ClientCAs` apply-point wiring across `internal/xds` + `internal/tls`; the row's genuinely-novel test-side piece is the SDS-served-CA mandatory-mTLS fixture (D-SDSVC-FIXTURE — provable with the landed SDS-serving + client-cert harness, NO new infra).

**Anticipated moves at the phase-65 IMPL (docs-only now):** the reject lift (`config.go:227-228`) + the `ClientCAs` apply-point + the `internal/xds` `validation_context` applier arm + the `FetchInitialValidationContext` provider method + the stream fetch arm + the `xdsgrpc` adapter wiring + the `sdsserver` `WithValidationContext` extension + applier/config unit tests + `FuzzDiscoveryResponseParse`/`FuzzTLSContextParse` seeds + the new mTLS fixture + the BEHAVIOR_CONTRACT TLS/SDS edit + ADR-0286. Counts: stat surface **1201 (+0)** · fixtures **109 → 110** · fuzzers **55 (+0, seeds only)** · BackendKind **38 (+0)** · DECISIONS tail **ADR-0286** (next-free **ADR-0287**) · new Go packages **0** · new go.mod modules **0**.

**Counts UNCHANGED at this BRAINSTORM (docs-only; re-verified against master tip `6f46481d`):** stat surface **1201** · fixtures **109** · fuzzers **55** · BackendKind **38** · DECISIONS tail **ADR-0285** (next-free **ADR-0286**) · go.mod modules **2**. Row 65 registers `in-progress` at this BRAINSTORM commit per the §Schema invariant.

**Next → the phase-65 SPEC** (the D-SDSVC-* live-probe arms against `envoyproxy/envoy:contrib-v1.37.2` — D-SDSVC-REFSERVE / -RESOURCE / -REQUIRE-SCOPE / -FETCHTIMEOUT / -CVC-REJECT; re-derive every §6 edit site + the applier/provider/stream symbol-name collision-checks + the client-cert runner mechanism; pin D-SDSVC-PROVIDER + D-SDSVC-APPLY-POINT + D-SDSVC-FIXTURE + D-SDSVC-NEGATIVE + D-SDSVC-STATS; draft ADR-0286 §Context).
