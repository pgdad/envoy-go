# SPEC 65 — xDS **SDS `validation_context`** (the THIRD xDS-family row; the SECOND SDS resource type after phase-60.2's downstream server cert — lift the `internal/tls/config.go:227-228` downstream `validation_context_sds_secret_config` reject and install the SDS-delivered CA bundle as the downstream listener's `ClientCAs` for mandatory mTLS client-cert validation, over the landed phase-60.2 SotW SDS discovery stream + the phase-16 static-mTLS `ClientCAs` path)

> **Stage:** SPEC (lifecycle-state 1 → 2). Docs-only; NO production `.go` changes (the §11 live probe built a THROWAWAY probe dir — DELETED before commit, `git status` verified clean). Fresh worktree off master, branch `phase-65-xds-sds-validation-context-spec`, per `feedback_git_worktrees`.
>
> **ANCHORS ADR-0286 §Context DRAFT** (§13). §Decision/§Consequences land at the phase-65 IMPL per ADR-0044; the DECISIONS tail STAYS **ADR-0285** at SPEC time (ADR-0286 is reserved, not yet a full entry).
>
> **Baselines RE-VERIFIED against master tip `df43d940` (the phase-65 BRAINSTORM squash):** stat surface **1201** · fixtures **109** (`ls -d test/fixtures/[0-9]*/ | wc -l`; numeric tail `0107-tracing-max-path-tag-length`) · fuzzers **55** (`grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l`) · BackendKind tail **38** (`H2GoawayResponder`) · DECISIONS tail **ADR-0285** (next-free **ADR-0286**) · new Go packages **0** · go.mod modules **2** (`quic-go` direct + `qpack` indirect). Every `file:line` below RE-DERIVED from source THIS session (`feedback_brief_citations_not_evidence`) — the controller RE-READ `internal/tls/config.go`, `internal/xds/{secret,provider,stream,stats,config}.go`, `internal/boot/boot.go`, the `sdsserver` helper, and the `0103`/`0018` fixtures in full.

---

## 1. Purpose / Mission

Phase 65 lifts ONE downstream reject and honors an SDS-delivered `validation_context`: fetch the trusted-CA bundle over the ALREADY-landed SotW SDS stream (bounded by `initial_fetch_timeout`), load it into an `*x509.CertPool`, and install it as the downstream listener's `ClientCAs` — so an operator configuring `require_client_certificate: true` with `common_tls_context.validation_context_sds_secret_config` gets mandatory mTLS whose trust anchor is delivered dynamically by an SDS management server, IDENTICAL to the reference. It is the SECOND SDS **resource type** (after phase 60.2's `tls_certificate` server cert) and the THIRD xDS-family row (after 60.1 the substrate, 60.2 the server-cert wiring).

The genuinely-new production work is narrow and mirrors the landed server-cert path: (a) an `internal/xds` applier arm parsing the `validation_context` oneof of a `tls.v3.Secret` into an `*x509.CertPool`; (b) a parallel provider method `FetchInitialValidationContext` (the landed `FetchInitialCertificate` returns a `*stdtls.Certificate`, not a CA pool); (c) the `internal/tls` reject-lift + the `ClientCAs` apply-point + the `require_client_certificate` precondition relax; (d) the `internal/boot` SDS-provider pre-scan extension (so a provider is built when a listener uses SDS for the validation_context, not just the server cert).

**D-SDSVC-* disposition index (each PINNED by a §11 live probe, DERIVED from re-read source, or DECIDED here):**

| D-question | Disposition | Evidence class |
|---|---|---|
| **D-SDSVC-REFSERVE** | HELD — the reference serves + applies an SDS `validation_context` and validates client certs against it (arm 1 → 200; arm 2 wrong-CA → `ssl.fail_verify_error`; arm 3 no-cert → `ssl.fail_verify_no_cert`) | PINNED (§11 arms 1/2/3) |
| **D-SDSVC-RESOURCE** | PINNED — served `Secret{name, validation_context: CertificateValidationContext{trusted_ca: DataSource}}`; the reference's own `config_dump` shows it under `dynamic_active_secrets` | PINNED (§11 config_dump) |
| **D-SDSVC-REQUIRE-SCOPE** | PINNED — `require_client_certificate: true` = mandatory mTLS (arm 3 no-cert rejected `certificate required`); `false` (verify-if-present) DEFERS, consistent with the inline precedent | PINNED (§11 arm 3) + DECIDED (§2.4) |
| **D-SDSVC-FETCHTIMEOUT** | DERIVED — the `validation_context` fetch reuses the phase-60.2 `FetchInitial*` timeout + boot-FAIL DEPARTURE unchanged (the reference serve-anyway was probed at phase 60.2 arm B) | DERIVED (§2.5, ADR-0280) |
| **D-SDSVC-CVC-REJECT** | RE-DERIVED — the SDS applier mirrors the inline CVC sub-field reject roster (`config.go:234-245`) with `xds: sds:`-prefixed distinct substrings | RE-DERIVED (§2.6, §6) |
| **D-SDSVC-PROVIDER** | DECIDED — Option A (parallel chain): `parseValidationSecret` / `applyValidationResponse` / `fetchValidationSecret` / `FetchInitialValidationContext` returning `*x509.CertPool`; the landed `*stdtls.Certificate` chain UNTOUCHED | DECIDED (§3.2) |
| **D-SDSVC-APPLY-POINT** | DECIDED — fetch + install in `NewDownstreamConfig`'s `require_client_certificate` block (gated on `require==true`, no wasteful fetch when absent); `commonTLSContextToConfig` lifts the reject to a NO-OP for the downstream+provider path; upstream/QUIC keep the byte-identical reject | DECIDED (§3.3) |
| **D-SDSVC-BOOT-SCAN** | DECIDED — extend `boot.NewSDSProvider`'s pre-scan (`boot.go:138`) to also detect a downstream `validation_context_sds_secret_config`; the provider is built for the ONE SDS secret the listener uses; a context using SDS for BOTH server-cert AND validation is the deferred compose-two edge (rejected by the existing `seen>1` guard) | DECIDED (§3.4) |
| **D-SDSVC-FIXTURE** | PINNED — ONE new dir `0108-xds-sds-validation-context`; driver-owned `sdsserver` (extended `WithValidationContext`) serves the CA; the driver presents a client cert via a stdlib `tls.Config{Certificates}` (the `0018` scenario-6 mechanism — NO runner API exists); cross-side request-outcome equality | PINNED (§8) |
| **D-SDSVC-NEGATIVE** | DECIDED — ONE dir, positive-primary (client_good → 200 cross-side) + a wrong-CA negative sub-assertion (client_bad → handshake reject + a subject-side `ssl.fail_verify_error` `StatsAsserter`) | DECIDED (§8) |
| **D-SDSVC-STATS** | DECIDED — +0 counter TYPES; reuse the 5-counter `SDSStats` scoped to the validation secret name → a NEW dynamic `sds.<validation-secret>.*` scope (same dynamic-scope convention as `server_cert`'s) | DECIDED (§7) |
| **D-SDSVC-FUZZSEED** | DECIDED — SEEDS to `FuzzDiscoveryResponseParse` (`xds/fuzz_test.go:71`) + `FuzzTLSContextParse` (`tls/fuzz_test.go:24`); fuzzer count STAYS **55** | DECIDED (§6) |
| **D-SDSVC-SPLIT** | DECIDED — a SINGLE FLAT ROW (~11 tasks, §10); the ADR-0045 escape-valve (65.1 applier / 65.2 wiring) is documented ARMABLE but UNCONSUMED (the substrate is landed; the two-package surface is bounded) | DECIDED (§3.0) |
| **D-SDSVC-DOCSHAPE** | RE-DERIVED — the full edit-site roster against master `df43d940` | RE-DERIVED (§12) |

### 1.1 Every BRAINSTORM anticipation HELD, with ONE apply-point refinement (ADR-0044)

The §11 live probe CONFIRMED every anticipated reference semantic (D-SDSVC-REFSERVE / -RESOURCE / -REQUIRE-SCOPE) — no re-scope. ONE design refinement over the BRAINSTORM §2.6 anticipation: the BRAINSTORM anticipated `commonTLSContextToConfig` would thread the fetched pool OUT to `NewDownstreamConfig` (mirroring how `sdsCert` flows to `cfg.Certificates`). Re-reading the boot/apply seam this session showed a cleaner design: because `require_client_certificate` lives on the `DownstreamTlsContext` (visible only in `NewDownstreamConfig`, NOT in `commonTLSContextToConfig` which sees only the `CommonTlsContext`), the fetch itself belongs in `NewDownstreamConfig`'s `require_client_certificate` block — gated on `require==true`, so no wasteful boot-time SDS fetch happens for the deferred `require==false` case. `commonTLSContextToConfig` only lifts the reject to a no-op for the downstream+provider path. This is a §2.6/§3.3 refinement, not a scope change (D-SDSVC-APPLY-POINT).

---

## 2. Non-purposes (deferred; per BRAINSTORM §1.2 + §8)

- **Upstream SDS** — validating an upstream SERVER cert / validation_context via SDS (`NewUpstreamConfig`, `config.go:118-156`, runs with `provider == nil`). The provider is not threaded to the cluster build; DEFERRED (the "upstream SDS" half of the deferred-sentence candidate STAYS after this row narrows the downstream half at the IMPL, §9).
- **`combined_validation_context`** (`config.go:229-230`) — an inline `default_validation_context` merged with an SDS CA; the sibling reject STAYS loud. DEFERRED (a natural follow-on once plain SDS `validation_context` lands).
- **SDS rotation** — the provider is INITIAL-FETCH only (`provider.go`); no watch/re-push loop. DEFERRED.
- **`require_client_certificate: false` optional/verify-if-present mTLS with an SDS CA** — phase 65 SCOPES to mandatory mTLS mirroring phase 16 (§2.4). The `false` path is INERT (the SDS validation_context is silently ignored, consistent with the inline `validation_context`-without-`require` precedent). DEFERRED.
- **Compose-two SDS secrets on one context** — a listener using SDS for BOTH the server cert AND the validation_context (two secrets, two `sds.*` scopes) exceeds the single-secret provider model (`provider.go` holds one `*SDSStats`); the existing `seen>1` boot guard (`boot.go:147-148`) REJECTS it. DEFERRED (needs per-secret provider threading, §3.4).
- **CDS/EDS/LDS/RDS/ADS/Delta-xDS/RTDS · google_grpc transport · the `crl` / CVC-feature sub-fields** — each a larger follow-on. DEFERRED (§8).

---

## 3. The change — lift the reject, a parallel SDS `validation_context` applier + provider method, the `ClientCAs` apply-point + boot pre-scan (ADR-0286)

### 3.0 Split disposition — a SINGLE FLAT ROW; the ADR-0045 escape-valve ARMABLE but UNCONSUMED

Anticipated ~11 tasks (§10) — under the ADR-0045 `~15` ceiling. Unlike phase 60 (which had to BUILD the SDS discovery substrate, ~16-18 tasks → a CONFIRMED 60.1/60.2 split), phase 65's substrate — the SotW discovery machine, ACK/NACK, `initial_fetch_timeout`, `ParseSDSConfig`, the `xdsgrpc` adapter, the `sdsserver` helper, the `0103` template, the `0018` client-cert harness — is ALL landed. Both the applier arm and the `internal/tls`+`internal/boot` wiring are bounded increments on that substrate. So a SINGLE FLAT ROW.

**The escape valve is documented ARMABLE, UNCONSUMED:** IF the PLAN's task count surprises upward past ~15, the natural cut mirrors 60.1/60.2 — **65.1** the `internal/xds` `validation_context` applier + `FetchInitialValidationContext` provider method (unit-proven, no differential — the phase-60.1 substrate-leg shape) / **65.2** the `internal/tls` reject-lift + `ClientCAs` wiring + `internal/boot` pre-scan + the mTLS fixture (the phase-60.2 observable-end-to-end shape). Re-armed ONLY if the two-package surface would strand a leg. Anticipated: SINGLE FLAT ROW (row 65 flips `done` at its one IMPL six-gate, ADR-0106).

### 3.1 The discovery machine is UNCHANGED — a parallel resource-type arm feeding an `*x509.CertPool`

The SotW fetch (`fetchSecret`, `stream.go:38-80`), ACK/NACK, the `initial_fetch_timeout` bound (`provider.go:47-75`), `ParseSDSConfig` (`config.go:22-79`), `secretTypeURL()`/`dataSourceBytes` (`secret.go:17-48`), and the `xdsgrpc.Opener` adapter (`xdsgrpc/opener.go:30-50`) are ALL reused verbatim. The new arm parses the `validation_context` oneof of the SAME `tls.v3.Secret` (the wire type URL is generic, `secret.go:17-19`) into an `*x509.CertPool`. The landed `tls_certificate` applier (`parseSecret`, `secret.go:55-80`) is UNTOUCHED — it stays `tls_certificate`-only (its `TestParseSecret_WrongOneof`, `secret_test.go:175-187`, which asserts a `Secret_ValidationContext` is rejected by `parseSecret`, STAYS valid: the new `validation_context` handling is a SEPARATE applier, not a widening of `parseSecret`).

### 3.2 The provider/stream seam — Option A (parallel chain returning `*x509.CertPool`) *(D-SDSVC-PROVIDER — DECIDED)*

The landed chain returns `*stdtls.Certificate` end-to-end. A CA bundle is an `*x509.CertPool`. Option A adds a PARALLEL chain (all four names GREP-collision-checked FREE this session in `internal/xds` + `internal/tls`, `reference_spec_drafted_identifier_collision_check`):

- **`internal/xds/secret.go`** — `parseValidationSecret(resource *anypb.Any, wantName, baseDir string) (*x509.CertPool, error)`: unmarshal to `tlsv3.Secret`; `GetName() == wantName`; `sec.GetValidationContext()` (the `*CertificateValidationContext`) non-nil (else "secret %q is not a validation_context (unsupported oneof arm)"); MIRROR the inline CVC sub-field rejects (§2.6); read `.GetTrustedCa()` via the EXISTING `dataSourceBytes` → `x509.NewCertPool().AppendCertsFromPEM(...)` (parse-failure → error). The CA-pool build is DUPLICATED in `internal/xds` (mirroring `dataSourceBytes`'s deliberate duplication of `internal/tls.loadDataSource`) — `internal/xds` must NOT import `internal/tls` (the 60.2 cycle guard, §3.6).
- **`internal/xds/stream.go`** — `fetchValidationSecret(stream Stream, node Node, secretName, baseDir string) (*x509.CertPool, error)` + `applyValidationResponse(resp, secretName, baseDir) (*x509.CertPool, error)`: byte-parallel to `fetchSecret`/`applyResponse` (`stream.go:38-95`), differing ONLY in the parse call (`parseValidationSecret`) + the return type. The ACK/NACK/`errValidation`/nonce dance is IDENTICAL (the duplication is bounded, ~40 lines, and isolates the landed phase-60.2 chain from any regression — the LEAN choice per BRAINSTORM §2.5; the DRY generalization Option B was weighed and declined because the landed chain is load-bearing for the passing `0103` differential).
- **`internal/xds/provider.go`** — `FetchInitialValidationContext(ctx context.Context, secretName string) (*x509.CertPool, error)` on the `Provider` (byte-parallel to `FetchInitialCertificate`, `provider.go:47-75`), reusing the `timeout` + `SDSStats` machinery (`p.stats.incUpdateAttempt/…`) + the `errValidation`/`ctx.Err()` classification switch verbatim. Added to the `SecretProvider` interface (`provider.go:14-16`) — so every implementer (the real `*Provider` + the `internal/tls` test fakes) gains the method (a bounded test-fake update, §6).

The boot-side `xdsgrpc.Opener` (`xdsgrpc/opener.go`) needs NO change — it opens the stream; the resource-type-specific parse is caller-side.

### 3.3 The reject-lift + the `ClientCAs` apply-point *(D-SDSVC-APPLY-POINT — DECIDED)*

**(a) `commonTLSContextToConfig` (`config.go:225-232`) — lift the reject to a no-op for the downstream+provider path:**
```go
case *tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig:
    // Phase 65 (ADR-0286): a downstream SDS-delivered validation_context is
    // honored by NewDownstreamConfig's require_client_certificate block (which
    // sees require_client_certificate + the live provider). Here it is a no-op
    // for the downstream+provider path; upstream and the QUIC/nil-provider path
    // keep the BYTE-IDENTICAL reject (ADR-0080 distinct substring).
    if side != "downstream" || provider == nil {
        return nil, fmt.Errorf("tls: %s: SDS-bound validation_context_sds_secret_config is not supported in phase 03", side)
    }
```
The `combined_validation_context` case (`:229-230`) and the inline CVC rejects (`:233-245`) are UNCHANGED. QUIC (`NewQUICDownstreamConfig`, provider=nil, `config.go:108`) and upstream (`NewUpstreamConfig`, provider=nil, `config.go:143`) keep the reject.

**(b) `NewDownstreamConfig` (`config.go:67-79`) — fetch + install, gated on `require_client_certificate`, precondition relaxed:**
```go
if ctx.GetRequireClientCertificate().GetValue() {
    common := ctx.GetCommonTlsContext()
    if sdsVC, ok := common.GetValidationContextType().(*tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig); ok {
        if provider == nil {
            return nil, fmt.Errorf("tls: downstream: SDS-delivered validation_context requires a live SDS provider (unavailable in this mode)")
        }
        secretName, _, _, err := xds.ParseSDSConfig([]*tlsv3.SdsSecretConfig{sdsVC.ValidationContextSdsSecretConfig})
        if err != nil {
            return nil, fmt.Errorf("tls: downstream: %w", err)
        }
        pool, err := provider.FetchInitialValidationContext(context.Background(), secretName)
        if err != nil {
            return nil, fmt.Errorf("tls: downstream: SDS validation secret %q: %w", secretName, err)
        }
        cfg.ClientCAs = pool
    } else {
        vc := common.GetValidationContext()
        if vc == nil || vc.GetTrustedCa() == nil {
            return nil, fmt.Errorf("tls: downstream: require_client_certificate=true requires validation_context.trusted_ca")
        }
        pool, err := loadTrustedCAPool(vc, baseDir, "downstream")
        if err != nil {
            return nil, err
        }
        cfg.ClientCAs = pool
    }
    cfg.ClientAuth = stdtls.RequireAndVerifyClientCert
}
```
The `else` branch is the byte-identical phase-16 inline path (`config.go:68-77`). `ParseSDSConfig` takes a `[]*SdsSecretConfig` (the singular `validation_context_sds_secret_config` wrapped in a 1-element slice; it enforces `len==1` at `config.go:23`). `context`/`xds`/`x509` are already imported in `config.go`.

### 3.4 The boot-side SDS-provider pre-scan extension *(D-SDSVC-BOOT-SCAN — DECIDED)*

`boot.NewSDSProvider` (`boot.go:120-165`) pre-scans downstream filter chains for `GetTlsCertificateSdsSecretConfigs()` ONLY (`boot.go:138`). A listener using SDS for the validation_context (with a STATIC/inline server cert) would pre-scan as `seen==0` → `nil` provider → `NewDownstreamConfig` gets `nil` → the §3.3(b) `provider == nil` reject fires. So the pre-scan is EXTENDED to also detect the SDS validation arm within the SAME `dtc.GetCommonTlsContext()`:
```go
ctc := dtc.GetCommonTlsContext()
if sc := ctc.GetTlsCertificateSdsSecretConfigs(); len(sc) > 0 {
    seen++
    found = sc
}
if vsc, ok := ctc.GetValidationContextType().(*tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig); ok {
    seen++
    found = []*tlsv3.SdsSecretConfig{vsc.ValidationContextSdsSecretConfig}
}
```
The `seen>1` guard (`boot.go:147-148`) then REJECTS a context using SDS for BOTH secrets (the deferred compose-two edge, §2) AND multiple SDS-bound contexts (the existing MVP-one guard) — the reject message stays as-is (a compose-two follow-on refines it). For the fixture's shape (validation via SDS, server cert static) → `seen==1`, provider built for the validation secret, `RegisterSDSStats(registry, "<validation-secret>")` → `sds.<validation-secret>.*` (`boot.go:163`). This preserves `0103` (server cert via SDS, no validation_context → `seen==1`, unchanged).

### 3.5 The `require_client_certificate == false` case is INERT (consistent with the inline precedent)

With my §3.3 design, an SDS `validation_context` present but `require_client_certificate` absent/false → `NewDownstreamConfig`'s block never fires → the SDS validation is silently ignored, `ClientCAs`/`ClientAuth` unset, no boot-time fetch. This exactly mirrors the landed inline behavior: an inline `validation_context.trusted_ca` with `require_client_certificate==false` is ALSO inert (only the `require==true` block loads `ClientCAs`). So phase 65 introduces no NEW inconsistency — the `false` path (reference: verify-if-present) is a documented deferral (§2), and the SDS provider boot-time fetch is correctly skipped (no wasted stream).

### 3.6 The 60.2 cycle guard STANDS

`internal/xds` must NOT import `internal/tls` (the reverse edge cycles — `internal/tls` imports `internal/xds`, `config.go:13`). The `parseValidationSecret` CA-pool build (`x509.NewCertPool().AppendCertsFromPEM`) is DUPLICATED in `internal/xds` (mirroring `dataSourceBytes`, `secret.go:21-26`) rather than calling `internal/tls.loadTrustedCAPool`. The IMPL verifies with `go list -deps ./internal/xds` (no `...`) — the dep set must NOT gain `internal/tls`.

---

## 4. Framework primitives — 0 new packages, 0 new go.mod modules

All edits land in EXISTING files/packages: `internal/xds/{secret,stream,provider}.go` (the applier/stream/provider arms), `internal/tls/config.go` (the reject-lift + apply-point), `internal/boot/boot.go` (the pre-scan), `test/helpers/sdsserver/sdsserver.go` (the `WithValidationContext` option), `test/fixtures/0108-…` (new dir, no new package), and `docs/`. `GetValidationContext()`/`Secret_ValidationContext`/`CertificateValidationContext`/`CommonTlsContext_ValidationContextSdsSecretConfig` are all reachable via the resolved `go-control-plane v1.32.4` TLS proto (already imported as `tlsv3` in both `internal/xds/secret.go:10` and `internal/tls/config.go:11`). `*x509.CertPool` is stdlib (already used at `config.go:163-173`). `go mod tidy -diff` anticipated EMPTY (modules STAY **2**). ZERO new packages, ZERO new modules.

---

## 5. Proto-field roster (RE-DERIVED @ go-control-plane/envoy v1.32.4, `go doc` this session)

- `tlsv3.CommonTlsContext.validation_context_type` oneof — arm `*CommonTlsContext_ValidationContextSdsSecretConfig{ValidationContextSdsSecretConfig *SdsSecretConfig}` (proto field 7). Accessor `GetValidationContextType()`. (Already referenced at `config.go:227`, `config_test.go:945`.)
- `tlsv3.Secret.type` oneof — arm `*Secret_ValidationContext{ValidationContext *CertificateValidationContext}` (proto field 4). Accessor `GetValidationContext()`. (Already used at `secret_test.go:178`.)
- `tlsv3.CertificateValidationContext` — `GetTrustedCa() *corev3.DataSource`, `GetCustomValidatorConfig()`, `GetMatchTypedSubjectAltNames()`, `GetVerifyCertificateHash()`, `GetVerifyCertificateSpki()`, `GetCrl()`. (Already consumed at `config.go:163,234-244`.)
- `tlsv3.SdsSecretConfig` — the envelope `ParseSDSConfig` (`internal/xds/config.go:22`) already validates (arms 1-4,8,9). REUSED verbatim for the singular validation config.

No new proto import; no new module.

---

## 6. PARSE/BOOT-REJECT roster + fuzzer (all ADR-0080-distinct substrings)

**The downstream reject NARROWS** (envoy-go now HONORS `validation_context_sds_secret_config` — matches the reference). The `internal/xds` applier mirrors the inline CVC support surface onto the SERVED `CertificateValidationContext` (D-SDSVC-CVC-REJECT — `reference_strict_reject_sibling_typeurl_gap`: lifting the SDS envelope is not licence to silently accept CVC sub-fields envoy-go can't honor). RE-DERIVED from `config.go:234-245`, mirrored in `parseValidationSecret` with `xds: sds:`-prefixed distinct substrings:

| Served CVC sub-field | `internal/xds` reject substring (mirrors `config.go`) |
|---|---|
| `custom_validator_config` | `xds: sds: validation secret %q: custom_validator_config is not supported` |
| `match_typed_subject_alt_names` | `xds: sds: validation secret %q: match_typed_subject_alt_names is not supported` |
| `verify_certificate_hash` | `xds: sds: validation secret %q: verify_certificate_hash is not supported` |
| `verify_certificate_spki` | `xds: sds: validation secret %q: verify_certificate_spki is not supported` |
| empty/absent `trusted_ca` | `xds: sds: validation secret %q: trusted_ca: <dataSourceBytes err>` |
| unparseable PEM | `xds: sds: validation secret %q: trusted_ca: parse failure` |

(`crl` is weighed and DEFERRED to the CVC-feature follow-on, §8 — it is rejected on BOTH the inline and SDS paths only if the inline path rejects it; the inline path does NOT currently check `crl`, so to stay parallel the SDS applier ALSO does not check it — a documented shared gap, NOT a new asymmetry.)

**The STAYING siblings** keep their distinct substrings: `combined_validation_context` (`config.go:229-230`), the inline CVC rejects (`:234-245`), upstream `tls_certificate_sds_secret_configs` (`:208-209`), upstream `validation_context_sds_secret_config` + QUIC (both via the `side != "downstream" || provider == nil` guard, §3.3a). `ParseSDSConfig`'s 9 envelope rejects (`config.go:23-73`) run unchanged for the singular validation config.

**Fuzzer: SEEDS only, count STAYS 55** (`reference_fuzzer_count_docs_drift` — reconciled to 55 before AND after; `grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l` == 55, MUST NOT move):
- `internal/xds/fuzz_test.go` `FuzzDiscoveryResponseParse` (`:71`, `f.Add([]byte, string)`) — a NEW seed: a `DiscoveryResponse` carrying a `validation_context` Secret (via a `mustValidValidationSecretAnyBytes` helper paralleling `mustValidSecretAnyBytes`, `:26-64`); the fuzz target additionally drives `applyValidationResponse`.
- `internal/tls/fuzz_test.go` `FuzzTLSContextParse` (`:24`, `f.Add(side, typeURL, value)`) — a NEW seed: a `downstream` `DownstreamTlsContext` with `require_client_certificate: true` + a `validation_context_sds_secret_config`. The invariant (every error `"tls: "`-prefixed) is UNCHANGED.

---

## 7. Stat surface — +0 counter TYPES (D-SDSVC-STATS)

The SDS fetch lifecycle counters are the 5-counter `SDSStats` set (`internal/xds/stats.go:12-18`), registered per-secret-name as `sds.<secretName>.{update_success,update_failure,update_rejected,update_attempt,init_fetch_timeout}` (`RegisterSDSStats`, `:25-40`; the operator-controlled name segment guarded by `stats.IsValidName` before `NewCounterIfAbsent`, `reference_dynamic_stat_name_charset_guard`). `FetchInitialValidationContext` REUSES this set (the provider is built for the validation secret → `sds.<validation-secret>.*`). NO new counter TYPE is registered — the surface count **1201** is UNCHANGED (the `sds.*` scope is DYNAMIC, keyed on the configured secret name, exactly as `server_cert`'s scope is; a new secret name yields new dynamic counters, not a new static-surface type — the phase-60.2 convention). The mTLS verify outcome surfaces via the EXISTING `ssl.*` downstream counters (`ssl.handshake`, `ssl.fail_verify_error`, `ssl.fail_verify_no_cert` — observed live in §11), not a new stat.

**Stat surface 1201 → 1201 (+0).**

---

## 8. Differential fixture taxonomy — +1 (D-SDSVC-FIXTURE / -NEGATIVE)

**ONE new differential dir `test/fixtures/0108-xds-sds-validation-context`** (fixtures **109 → 110**), cloned from `0103-xds-sds-server-cert` (the driver-owned-SDS template) crossed with `0018-http-rbac` (the client-cert-presenting mechanism). Per `reference_differential_fixture_dispatch_constraint` (one dir = one runner branch), do NOT mutate `0103` or `0018`.

- **The SDS server** — the driver-owned `test/helpers/sdsserver` (`reference_differential_grpc_receiver_driver_owned` — the proxy DIALS it, NOT a runner `BackendKind`; BackendKind STAYS 38), extended with a `WithValidationContext(name string, trustedCAPEM []byte)` Option paralleling `WithSecret` (`sdsserver.go:43-45`) + a `buildResponse` branch (`:118-136`) emitting `Secret{name, ValidationContext: &CertificateValidationContext{TrustedCa: &DataSource{InlineBytes: caPEM}}}`. The generic type-URL derivation (`:133`) works unchanged. Two receivers, one per side (the `0103` `ensure()` pattern, `driver.go:63-86`). `BackendCount() == 1` (`reference_differential_backendcount_min_one`).
- **The listener config** — both `envoy.yaml`/`envoy-go.yaml` configure the downstream `DownstreamTlsContext` with `require_client_certificate: true` + a STATIC/inline server cert (`tls_certificates`, the `0018` shape, `envoy-go.yaml:160-174`) + `common_tls_context.validation_context_sds_secret_config` → `api_config_source{GRPC,V3}` → a static `sds_cluster` (the `0103` SDS-cluster shape, `envoy-go.yaml:81-95`) + a `node{id,cluster}` (REQUIRED, `boot.go:151-152`).
- **The driver presents a client cert** — NO runner API exists (`test/differential/fixture/fixture.go` `DriveReference/DriveSubject(ctx, addr)` carry no TLS options; grep-confirmed zero client-cert plumbing). The driver builds its OWN `tls.Config{Certificates: []tls.Certificate{tls.LoadX509KeyPair(clientCert, clientKey)}, RootCAs: serverCAPool, ServerName: …}` and dials — the `0018` scenario-6 mechanism (`0018/inputs/driver.go:501-534`). `helpers.TLSServedLeaf` (what `0103` uses, `helpers/tls.go:63-80`) can NOT present a client cert (no `Certificates` field), so the `0108` driver inlines a custom mTLS dial (like `0018` scenario 6 / `0045` `tlsDial`) — either directly or by extending `helpers/tls.go` with a client-cert-capable variant (PLAN chooses).
- **Positive arm (cross-side)** — a client cert signed by the SDS-served CA → the handshake completes → drive a request → assert the status/body is byte-identical cross-side via the runner's CompareBytes (`0103/driver.go:171-177` returns the observable bytes). Assert each property with `Errorf`, NOT `Fatalf` (`reference_fatalf_makes_assertions_unreachable`).
- **Negative arm (D-SDSVC-NEGATIVE)** — a SECOND scenario within the SAME dir (the `0018` multi-scenario precedent): a client cert signed by a DIFFERENT, un-served CA → the handshake is REJECTED both sides (the driver normalizes the dial error to a stable byte form → cross-side equal) + a subject-side `StatsAsserter` (`fixture.StatsAsserter`, `fixture.go:70-77`; scrape `/stats/prometheus` into `map[string]int64`, the `0018` `scrapeRBACStats` idiom, `driver.go:779`) asserting `listener.…ssl.fail_verify_error >= 1`. This proves the SDS-served CA is the ACTUAL trust anchor (not a vacuous accept-all).
- **Break-proof** — prove the assertion LIVE with a deliberate `-count=1` break (`reference_differential_break_protocol_count1`) serving a WRONG CA (the positive client cert then fails to verify); confirm WHICH assertion fires (`reference_deliberate_break_wrong_assertion` — the positive-arm CompareBytes vs the negative-arm stat assertion, isolate if needed) via `-run 'TestDifferential/0108-xds-sds-validation-context'` (NEVER bare, `reference_differential_run_selector`).

**Harness note:** the fixture drives H1/H2 over TCP+TLS (NOT H3/QUIC — QUIC carries no SDS, `config.go:87-88`), so `reference_differential_http_expectations_tcp_only` does not bite; the mTLS handshake + status live in the driver's own TLS-client dial + CompareBytes, not the runner's `HTTPExpectations`.

**NOT differential dirs:** the CVC-sub-field rejects (§6) + the `provider==nil`/QUIC/upstream rejects are subject-side unit/boot-reject tests (the reference ACCEPTS the CVC sub-fields envoy-go rejects, so no cross-side dir); the `require==false`-inert case is a subject-side config unit test.

---

## 9. Behavior-contract delta (`docs/envoy-go/BEHAVIOR_CONTRACT.md`; ADR-0286 atomic landing at the IMPL)

The TLS/SDS section flips downstream `validation_context_sds_secret_config` from "rejected (phase 03)" to: **consumed** — an SDS-delivered downstream mTLS trusted-CA, fetched over the SotW SDS stream (bounded by `initial_fetch_timeout`; boot-FAIL on timeout/mgmt-unreachable, the phase-60.2 DEPARTURE, extended), installed as the listener's `ClientCAs` under `require_client_certificate: true` (mandatory-mTLS scope); the served `CertificateValidationContext` is held to the inline CVC support surface (CVC sub-fields rejected); `require==false` is inert (deferred). The sibling SDS/CVC/upstream/`combined_validation_context` rejects STAY. IMPL RE-DERIVES the exact line(s). Landed atomically with ADR-0286 at the IMPL.

---

## 10. Test plan + per-task structure (~11 tasks; PLAN decomposes)

A SINGLE FLAT ROW (§3.0). Anticipated TDD spine (the PLAN pins exact boundaries):

1. **`internal/xds` applier** — `parseValidationSecret` (`secret.go`) + the CVC-reject roster (§6) + unit tests (valid CA → usable pool; wrong name; wrong oneof; each CVC sub-field → its distinct reject; bad PEM). `Errorf` per property.
2. **`internal/xds` stream arm** — `fetchValidationSecret` + `applyValidationResponse` (`stream.go`) + unit tests (initial-request shape; ACK on success; NACK on validation failure; transport error) paralleling `stream_test.go:55-166`.
3. **`internal/xds` provider method** — `FetchInitialValidationContext` (`provider.go`) + the `SecretProvider` interface method + unit tests (success; timeout; mgmt-down; rejected) paralleling `provider_test.go:59-137`.
4. **`internal/tls` reject-lift** — the `commonTLSContextToConfig` no-op (§3.3a) + a config unit test: downstream+provider no longer rejects; upstream + QUIC (nil provider) STILL reject (byte-identical substring).
5. **`internal/tls` apply-point** — `NewDownstreamConfig` fetch+install (§3.3b) + flip `config_test.go` `"validation_context_sds_secret_config unchanged (arm 5)"` (`:936-957`) from reject→ACCEPT (fake provider → `cfg.ClientCAs != nil` + `ClientAuth == RequireAndVerifyClientCert`); keep the inline `require_client_certificate` tests (`:286-349`) + the `require==false`-inert test.
6. **`internal/boot` pre-scan** — the `validation_context_sds_secret_config` detection (§3.4) + a boot test (validation-via-SDS builds a provider; both-via-SDS hits `seen>1`; `0103` server-cert-only unchanged).
7. **`sdsserver` extension** — `WithValidationContext` + the `buildResponse` branch + a helper unit test.
8. **Fixture `0108`** — driver + `envoy.yaml`/`envoy-go.yaml` + `pki/` (CA + server leaf + a CA-signed client cert + an un-served client cert) + `expectations.yaml` + `README.md`; the positive cross-side arm + the negative sub-assertion + the client-cert dial; break-prove live.
9. **Fuzz seeds** — the two seeds (§6); reconcile the count STAYS 55.
10. **BEHAVIOR_CONTRACT** — the §9 edit.
11. **Verify + docs** — the six-gate (`gofmt -l`, `go vet`, `golangci-lint`, `go build ./...`, `go test ./... -race` on touched packages + the full `internal/xds`/`internal/tls` packages under `-race`, the 110-fixture differential) + ADR-0286 §Decision/§Consequences + STATE + ROADMAP row 65 `done` + the sentinel-sentence narrow (§9-below) + PROGRESS close + router roll.

**Sentinel maintenance (at the IMPL, NOT this SPEC):** narrow the live xDS deferred sentence's `"SDS validation_context/upstream SDS"` → `"upstream SDS (server-cert + validation_context)"` (dropping the now-CONSUMED downstream `validation_context`); re-run check-(2) to CONFIRM exactly ONE live xDS match remains (`reference_sentinel_deferred_sentence_live_vs_historical`; the xDS family STAYS OPEN). At THIS SPEC the sentence is UNTOUCHED (the phase-63 `request_header` convention — a chartered-but-not-done candidate stays in the sentence until its IMPL).

---

## 11. SPEC-time empirical-pin block (D-SDSVC-* live probes — executed IN-SESSION 2026-07-15, `envoyproxy/envoy:contrib-v1.37.2`, FRESH container per config)

**Harness.** The central question (D-SDSVC-REFSERVE) is "does the reference SERVE + APPLY an SDS-delivered `validation_context` as the mTLS client-cert trust anchor." The SDS SECRET-APPLICATION path (DiscoveryResponse → `Secret` → `validation_context` → trust store) is TRANSPORT-INDEPENDENT in Envoy — the gRPC SotW transport itself is the already-landed phase-60.2 substrate (probed at SPEC-60 §11 arm A). So this probe used FILE-based SDS (`sds_config.path_config_source.path`) to ISOLATE the secret-application semantics: a reference container with a downstream listener (`require_client_certificate: true`, an inline server leaf, `validation_context_sds_secret_config` → a served `Secret{validation_context{trusted_ca: inline_bytes}}`), plus a CA-signed client cert (`client_good`, chains to the served CA) and a wrong-CA client cert (`client_bad`, chains to an UN-served CA). Handshake decisions observed via curl + the admin `ssl.*` stats + `config_dump`. The reference's OWN `config_dump` (below) confirms the secret landed under `dynamic_active_secrets` (the DYNAMIC SDS machinery, NOT static) — so this is a faithful probe of the SDS-application code path, non-vacuous (three DISTINCT handshake outcomes prove verification actually ran). **The throwaway probe dir was DELETED after — this SPEC is docs-only, `git status` clean.**

**Config_dump (D-SDSVC-RESOURCE — the reference's own view of the SDS-delivered secret):**
```
"@type": …admin.v3.SecretsConfigDump
"dynamic_active_secrets": [{
  "name": "validation_ca",
  "secret": {
    "@type": …tls.v3.Secret,
    "name": "validation_ca",
    "validation_context": { "trusted_ca": { "inline_bytes": "LS0tLS1CRUdJTiBDRVJU…" } }
  }
}]
```
⇒ **D-SDSVC-RESOURCE**: the served resource is `Secret{name, validation_context: CertificateValidationContext{trusted_ca: <DataSource>}}` (the `Secret_ValidationContext` oneof, proto field 4). The reference accepted it via the SDS machinery (`dynamic_active_secrets`, NOT static).

**Arm 1 (D-SDSVC-REFSERVE — client cert signed by the SDS-served CA).** `curl --cert client_good --key … https://localhost:10000/`. Captured:
```
OK-MTLS
[HTTP=200]
```
⇒ **D-SDSVC-REFSERVE** (provability CONFIRMED — no re-scope): the reference LOADED the SDS-delivered `validation_context` into the downstream trust store; a client cert chaining to the served CA completes the mTLS handshake → 200.

**Arm 2 (D-SDSVC-REFSERVE / negative — wrong-CA client cert, un-served).** `curl --cert client_bad …`. Captured:
```
curl: (56) OpenSSL … tlsv1 alert unknown ca
[HTTP=000]
listener.0.0.0.0_10000.ssl.fail_verify_error: 1
```
⇒ the SDS-served CA is the ACTUAL trust anchor — a cert chaining to a DIFFERENT (un-served) CA is REJECTED (`unknown ca`, `ssl.fail_verify_error`). NOT a vacuous accept-all. Confirms the negative-arm design (§8 / D-SDSVC-NEGATIVE).

**Arm 3 (D-SDSVC-REQUIRE-SCOPE — no client cert presented).** `curl` (no `--cert`). Captured:
```
curl: (56) OpenSSL … tlsv13 alert certificate required
[HTTP=000]
listener.0.0.0.0_10000.ssl.fail_verify_no_cert: 1
```
⇒ **D-SDSVC-REQUIRE-SCOPE**: `require_client_certificate: true` + an SDS `validation_context` = MANDATORY mTLS (a missing client cert → `certificate required` / `ssl.fail_verify_no_cert`). This is the phase-65 scope (mirrors phase 16). The `false` (verify-if-present) variant DEFERS (§2). The three DISTINCT rejections (`unknown ca` vs `certificate required` vs the 200 success) prove verification is live, not masked.

**Full observed `ssl.*` roster (one success + two rejects):** `ssl.handshake: 1` · `ssl.fail_verify_error: 1` · `ssl.fail_verify_no_cert: 1` · `ssl.ciphers.TLS_AES_256_GCM_SHA384: 1` · `ssl.versions.TLSv1.3: 1` · `ssl.certificate.validation_ca.expiration_unix_time_seconds` (the SDS-named validation secret surfaces its own cert-expiry gauge). ⇒ §8's `StatsAsserter` targets `ssl.fail_verify_error` / `ssl.fail_verify_no_cert` (subject-side).

**D-SDSVC-FETCHTIMEOUT (DERIVED, not re-probed).** The reference's initial-fetch / serve-anyway behavior on an unreachable SDS source was probed at SPEC-60 §11 arm B (the reference BLOCKS server-init then SERVES CERT-LESS on timeout; envoy-go's boot-FAIL is the documented DEPARTURE, ADR-0280). The `validation_context` fetch reuses the IDENTICAL `Provider`/`FetchInitial*` timeout machinery (`provider.go:47-75`, the `errValidation`/`ctx.Err()` classification), so the DEPARTURE extends unchanged — no new reference behavior to probe.

*(Probe harness: an inline server leaf + a self-CA (`CA_good`) served as the SDS `validation_context`, `CA_bad` an un-served anchor, `client_good`/`client_bad` the two client certs; a file-SDS `path_config_source` served `vc.yaml`; container `probe65` on published ports 10000/9901; all certs/config/container written to the scratchpad and DELETED before this docs-only commit — no `.go`/probe leftovers in the tree.)*

---

## 12. Edit-site roster (D-SDSVC-DOCSHAPE — RE-DERIVED against master `df43d940`)

**Production — `internal/xds`:**
1. `secret.go` — ADD `parseValidationSecret(resource, wantName, baseDir) (*x509.CertPool, error)` + the CVC-reject roster (§6). Import `crypto/x509`. [ADD]
2. `stream.go` — ADD `fetchValidationSecret` + `applyValidationResponse` (parallel to `:38-95`). [ADD]
3. `provider.go` — ADD `FetchInitialValidationContext` (parallel to `:47-75`) + the `SecretProvider` interface method (`:14-16`). [ADD/EDIT]

**Production — `internal/tls/config.go`:**
4. The reject-lift — the `ValidationContextSdsSecretConfig` case (`:227-228`) → no-op for downstream+provider (§3.3a). [EDIT]
5. The apply-point — `NewDownstreamConfig`'s `require_client_certificate` block (`:67-79`) → the SDS/inline branch (§3.3b). [EDIT]

**Production — `internal/boot/boot.go`:**
6. `NewSDSProvider` pre-scan (`:138`) — also detect `validation_context_sds_secret_config` (§3.4). [EDIT]

**Test / harness:**
7. `test/helpers/sdsserver/sdsserver.go` — `WithValidationContext` Option (`:43-45`) + the `buildResponse` branch (`:118-136`). [ADD]
8. `internal/xds/secret_test.go` / `stream_test.go` / `provider_test.go` — the applier/stream/provider unit tests (§10 T1-T3). `TestParseSecret_WrongOneof` (`:175-187`) STAYS (parseSecret is unchanged). [ADD]
9. `internal/tls/config_test.go` — flip the `"validation_context_sds_secret_config unchanged (arm 5)"` subtest (`:936-957`) reject→ACCEPT; ADD the `require==false`-inert + the boot-scan tests; the test fakes gain `FetchInitialValidationContext`. [EDIT + ADD]
10. Fuzz SEEDS — `internal/xds/fuzz_test.go` (`:71`) + `internal/tls/fuzz_test.go` (`:24`) (§6). [ADD — no new fuzzer]

**Fixture:**
11. `test/fixtures/0108-xds-sds-validation-context` (new; RE-DERIVED next-free — `0107` is the numeric tail) — §8. [ADD]

**Docs:**
12. `docs/envoy-go/BEHAVIOR_CONTRACT.md` — the TLS/SDS edit (§9). [EDIT]
13. `docs/envoy-go/{ROADMAP,STATE,DECISIONS}.md` — ADR-0286 §Decision/§Consequences + row 65 `done` + the sentinel narrow, at the IMPL (this SPEC only drafts ADR-0286 §Context, §13). [IMPL]

---

## 13. ADR continuity — the ADR-0286 §Context DRAFT (anchored here; full entry at the phase-65 IMPL)

**ADR-0286 §Context (draft).** Phase 60 opened the xDS/dynamic-config family via SDS: 60.1 (ADR-0278) built the `internal/xds` SotW discovery-stream client — the `DiscoveryRequest`/`DiscoveryResponse` ACK/NACK version/nonce loop (`stream.go`), the `Secret` applier into a `*stdtls.Certificate` (`secret.go`), the `SecretProvider`/`StreamOpener` seams + the `initial_fetch_timeout` bound + the 5-counter `sds.<secret>.*` stats (`provider.go`/`stats.go`), and `ParseSDSConfig` (the `SdsSecretConfig` envelope, `config.go`); 60.2 (ADR-0280) threaded a blocking `SecretProvider` (built at boot from the shared dialer via the `xdsgrpc.Opener` adapter, `boot.NewSDSProvider`) into the downstream TLS build, lifting the ONE downstream `tls_certificate_sds_secret_configs` reject and installing the SDS-delivered leaf into `cfg.Certificates` — with a documented envoy-go DEPARTURE (a fetch timeout / mgmt-unreachable boot-FAILS the listener, vs the reference's serve-cert-less). Phase 16 (ADR-0147) had earlier established the static-mTLS `ClientCAs` path: `require_client_certificate: true` + an INLINE `validation_context.trusted_ca` → `loadTrustedCAPool` → `cfg.ClientCAs` + `ClientAuth = RequireAndVerifyClientCert`. The SECOND SDS resource type — an SDS-delivered `validation_context` (the downstream mTLS trusted-CA) — was carried as the xDS family's explicitly-next-named deferred candidate ("SDS validation_context/upstream SDS"). Phase 65 lifts the downstream `validation_context_sds_secret_config` reject (`config.go:227-228`, downstream+provider only; upstream/QUIC keep the byte-identical reject) and HONORS the knob: SPEC-65 live probes against `envoyproxy/envoy:contrib-v1.37.2` (§11, fresh container, `reference_probe_fresh_container_per_arm`) PINNED every anticipation (no ADR-0044 flip) — the reference serves a `Secret{validation_context{trusted_ca}}` via SDS (config_dump `dynamic_active_secrets`), loads it into the downstream trust store, and validates client certs against it (arm 1 CA-signed → 200; arm 2 wrong-CA → `ssl.fail_verify_error`; arm 3 no-cert → `ssl.fail_verify_no_cert`), with `require_client_certificate: true` = mandatory mTLS (the phase-65 scope; `false` verify-if-present defers). The design (Option A, LEAN): a PARALLEL `internal/xds` chain returning `*x509.CertPool` (`parseValidationSecret`/`applyValidationResponse`/`fetchValidationSecret`/`FetchInitialValidationContext`) that leaves the landed `*stdtls.Certificate` chain UNTOUCHED (zero regression to the passing `0103` differential); the `ClientCAs` apply-point in `NewDownstreamConfig`'s `require_client_certificate` block (D-SDSVC-APPLY-POINT — fetch+install gated on `require==true`, so no wasteful fetch for the inert `false` case; `commonTLSContextToConfig` only no-ops the reject for downstream+provider); an `internal/boot` pre-scan extension so a provider is built when a listener uses SDS for the validation_context; the served CVC held to the inline support surface (D-SDSVC-CVC-REJECT — CVC sub-fields rejected with `xds: sds:`-distinct substrings, `reference_strict_reject_sibling_typeurl_gap`); the boot-FAIL-on-timeout DEPARTURE extended unchanged (D-SDSVC-FETCHTIMEOUT); the 60.2 cycle guard STANDS (the CA-pool build DUPLICATED in `internal/xds`, `go list -deps` verified). A SINGLE FLAT ROW (ADR-0045 escape-valve armable but unconsumed — the substrate is landed); +0 stats (reuse `sds.*`) / +1 fixture (`0108`, an SDS-served CA + a mandatory-mTLS client-cert handshake, cross-side request-outcome equality + a wrong-CA negative sub-assertion) / +0 fuzzers (seeds) / +0 packages / +0 modules. §Decision/§Consequences land at the phase-65 IMPL per ADR-0044. ANCHORS ADR-0286.

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

**Counts UNCHANGED at this SPEC (docs-only; re-verified against master tip `df43d940`):** stat surface **1201** · fixtures **109** · fuzzers **55** · BackendKind **38** · DECISIONS tail **ADR-0285** (next-free **ADR-0286**) · new Go packages **0** · go.mod modules **2**.

**Anticipated at the phase-65 IMPL:** stat surface **1201 (+0)** · fixtures **109 → 110** (`0108-xds-sds-validation-context`) · fuzzers **55 (+0, seeds only)** · BackendKind **38 (+0)** · DECISIONS tail **ADR-0285 → ADR-0286** (next-free **ADR-0287**) · new Go packages **0** · new go.mod modules **0** · row 65 → `done`.

**ROADMAP/STATE at SPEC-DONE:** row 65 STAYS `in-progress` (a row flips `done` only at its IMPL six-gate, ADR-0106). The LIVE xDS deferred sentence is UNCHANGED (narrowed at the IMPL per the phase-63 convention, §10). STATE active-phase header flips to `phase 65 SPEC done` (NEXT = the phase-65 PLAN).

**Next → the phase-65 PLAN** (decompose §10's ~11-task spine into a TDD sequence: the `internal/xds` applier/stream/provider arms, the `internal/tls` reject-lift + apply-point, the `internal/boot` pre-scan, the `sdsserver` extension, the `0108` fixture + break-proof, the fuzz seeds, BEHAVIOR_CONTRACT, verify + ADR-0286 + row-done + router roll).
