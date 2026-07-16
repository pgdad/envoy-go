# PLAN 65 — xDS SDS `validation_context` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Every task is TDD (`superpowers:test-driven-development`): red → green, with a `-count=1` liveness break where an assertion is load-bearing.

**Goal:** Lift the ONE downstream `validation_context_sds_secret_config` reject (`internal/tls/config.go:227-228`) and HONOR the knob — fetch a trusted-CA bundle over the ALREADY-landed SotW SDS stream, load it into an `*x509.CertPool`, and install it as the downstream listener's `ClientCAs` under `require_client_certificate: true`, so an operator gets mandatory mTLS whose trust anchor is delivered dynamically by an SDS management server. The SECOND SDS resource type (after phase-60.2's `tls_certificate` server cert) and the THIRD xDS-family row.

**Architecture:** A PARALLEL `internal/xds` chain returning `*x509.CertPool` (`parseValidationSecret` → `applyValidationResponse` → `fetchValidationSecret` → `FetchInitialValidationContext`), leaving the landed `*stdtls.Certificate` chain byte-untouched (zero regression risk to the passing `0103` differential). `internal/tls` no-ops the reject for the downstream+provider path only, and `NewDownstreamConfig`'s `require_client_certificate` block gains an SDS branch that fetches + installs the pool. `internal/boot`'s `NewSDSProvider` pre-scan extends to detect the validation arm, so a validation-only-SDS listener actually builds a provider. One new differential fixture (`0108`) proves the served CA is the real trust anchor via a cross-side accept/reject contrast.

**Tech Stack:** Go; `github.com/envoyproxy/go-control-plane` v1.32.4 (`tlsv3.Secret_ValidationContext` / `CertificateValidationContext` / `CommonTlsContext_ValidationContextSdsSecretConfig` — all already-resolved, `tlsv3` already imported in both `internal/xds/secret.go:10` and `internal/tls/config.go:11`); stdlib `crypto/x509` (`*x509.CertPool`); the differential harness (`test/differential/fixture`, `test/helpers/sdsserver`).

## Global Constraints

- **Lifecycle = one stage per session.** This PLAN is DOCS-ONLY (no `.go`). The NEXT session is the IMPL. Row 65 STAYS `in-progress` until the IMPL six-gate (ADR-0106, the SOLE leg — `reference_roadmap_split_phase_row_done`).
- **Subagent-driven** per `feedback_execution_style` / `feedback_subagent_autocommit_claudemd` / `feedback_subagents_no_push`: each IMPL task is a fresh subagent that commits LOCALLY only; the controller verifies each commit, cleans leak files, squashes at stage-close, re-runs the suite on the frozen HEAD, and pushes.
- **Worktree discipline** (`feedback_git_worktrees`, `feedback_subagent_worktree_path_targeting`, `feedback_subagent_worktree_detach`): the IMPL runs in `.worktrees/phase-65-impl` (branch `phase-65-xds-sds-validation-context-impl`). Pin the canonical worktree root; subagents use worktree-relative paths; the controller verifies the MAIN checkout stays clean. On a deliberate break, restore with `git restore` only (no checkout-sha/amend) and re-verify the branch each task.
- **`next-prompt.txt` IS TRACKED** (`reference_next_prompt_tracked_despite_gitignore`) — edit it inside the stage worktree and fold into the squash; locate commits by SUBJECT (`git log --grep`), never by position.
- **The 60.2 cycle guard STANDS** (`reference_xds_config_seam_transitive_cycle_guard`): `internal/xds` must NOT import `internal/tls`. The CA-pool build is DUPLICATED in `internal/xds` (mirroring `dataSourceBytes`'s deliberate duplication of `internal/tls.loadDataSource`). **Do NOT reach for `internal/tls.loadTrustedCAPool` (`config.go:163-173`) from inside `internal/xds`** — that is the cycle. Verify at T11 with `go list -deps ./internal/xds` (NO `...`); today the entire envoy-go dep set is exactly `internal/stats` + `internal/xds`, and it MUST stay that way.
- **ADR-0080** — every reject substring stays DISTINCT. The downstream `validation_context_sds_secret_config` reject NARROWS (gated, not deleted); upstream + QUIC keep the BYTE-IDENTICAL string.
- **ADR-0044** — ADR-0286 §Decision/§Consequences land at this IMPL (SPEC §13 drafted §Context); DECISIONS tail **ADR-0285 → ADR-0286** (next-free ADR-0287).
- **ADR-0045** — a SINGLE FLAT ROW (11 tasks; escape-valve UNCONSUMED, under the ~15 ceiling).
- **Per-task gates** (`feedback_pertask_gofmt_lint`): each code task runs `gofmt -l` + `golangci-lint run` on touched packages + `go build ./...` + the touched-package `go test` before its commit.
- **`reference_fatalf_makes_assertions_unreachable`** — `Errorf` per independent property; `Fatalf` only for a broken precondition.
- **`reference_deliberate_break_wrong_assertion`** — every liveness break must confirm WHICH assertion fired, not merely that *a* failure occurred.
- **`reference_differential_break_protocol_count1`** — every break runs `-count=1`.
- **`reference_differential_run_selector`** — the `0108` differential ALWAYS uses the FULL `-run 'TestDifferential/0108-xds-sds-validation-context'` selector, NEVER a bare `-run '0108'`.
- **Anticipated counts at IMPL-DONE:** stat surface **1201 (+0)** · fixtures **109 → 110** (`0108`) · fuzzers **55 (+0, seeds only)** · BackendKind **38 (+0)** · DECISIONS tail **ADR-0286** (next-free ADR-0287) · new Go packages **0** · new go.mod modules **0**.

---

## ⚠️ PLAN-time corrections to SPEC-65 (RE-DERIVED against master tip `be419023`; `feedback_brief_citations_not_evidence`)

Every SPEC §12 `file:line` was RE-DERIVED from source this PLAN session by three independent readers. **The SPEC's line numbers are mostly exact.** Five substantive defects were found; each is corrected here and the correction is load-bearing. A SPEC is not evidence — this section is why.

### C1 🔴 `ParseSDSConfig` has **14** reject arms, not 9 (SPEC §6)

SPEC §6 says "`ParseSDSConfig`'s 9 envelope rejects (`config.go:23-73`)". Mechanically counted over `internal/xds/config.go:22-79`, there are **14**: `multiple tls_certificate_sds_secret_configs unsupported` (:24) · `SdsSecretConfig name is required` (:29) · `SdsSecretConfig sds_config is required` (:33) · `ads-sourced ConfigSource unsupported` (:36) · `self-sourced ConfigSource unsupported` (:39) · `ConfigSource must be an api_config_source` (:43) · `resource_api_version must be V3` (:46) · `DELTA_GRPC api_type unsupported` (:52) · `api_type must be GRPC` (:54) · `transport_api_version must be V3` (:57) · `exactly one grpc_service required` (:61) · `google_grpc transport unsupported` (:64) · `grpc_service must be envoy_grpc` (:68) · `envoy_grpc cluster_name is required` (:72). **Nothing needs building** — `ParseSDSConfig` is REUSED verbatim and all 14 run unchanged for the singular validation config. The correction matters only so no task tries to "mirror the 9".

### C2 🔴 The T5 test-flip is VACUOUS as the SPEC specifies it

The arm-5 subtest (`internal/tls/config_test.go:936-957`, inside `TestNewDownstreamConfig_SDS` `:877`) builds `&tlsv3.SdsSecretConfig{Name: "validation-secret"}` — **with NO `sds_config`**. Once the reject is lifted and the path routes through `xds.ParseSDSConfig`, that input fires the `xds: sds: SdsSecretConfig sds_config is required` arm (`config.go:33`) — NOT the nil-provider reject, NOT the fetch path. Flipping the assertion reject→ACCEPT against that input would prove NOTHING. **T5 MUST rebuild the input with a full `sds_config`** (reuse the existing `sdsSecretConfig` helper, `config_test.go:813`). This is the single highest-risk vacuity trap in the phase.

### C3 🔴 `ssl.fail_verify_error` DOES NOT EXIST in envoy-go — the SPEC §8 negative-arm `StatsAsserter` is INFEASIBLE

SPEC §8 / D-SDSVC-NEGATIVE designs the negative arm around "a subject-side `StatsAsserter` asserting `listener.…ssl.fail_verify_error >= 1`". The SPEC observed that counter on the **reference** in its §11 arm-2 probe and assumed parity. **envoy-go emits NO `ssl.*` stats whatsoever** — grep-confirmed this session: zero `fail_verify` hits in `internal/`+`cmd/`, and the ONLY listener-scope counter is `downstream_cx_total` (`internal/listener/manager.go:353`). The assertion cannot be written.

**CORRECTION (DECIDED at this PLAN):** the negative arm asserts the **driver-observed NORMALIZED handshake verdict**, CROSS-SIDE, and drops the stat assertion entirely.

- **Why this is not a weakening.** The proof obligation is "the SDS-served CA is the ACTUAL trust anchor, not a vacuous accept-all". The **contrast** discharges it: a client cert chaining to the SERVED CA is ACCEPTED, and one chaining to an UN-SERVED CA is REJECTED, on BOTH sides, byte-identically. A subject-only stat assertion is strictly *weaker* — it proves nothing cross-side. The contrast is the stronger instrument.
- **Named coverage boundary** (the `reference_close_direction_framework_gap` precedent): envoy-go has NO downstream TLS handshake-outcome stat family (`ssl.handshake` / `ssl.fail_verify_error` / `ssl.fail_verify_no_cert` / `ssl.ciphers.*` / `ssl.versions.*`, all observed live on the reference at SPEC §11). This is a PRE-EXISTING framework gap that phase 65 does NOT open and does NOT close. T11 records it as a NEW deferred Observability/xDS candidate. Do NOT "fix" it inline — adding a stat family is a framework-surgery row of its own and would blow the +0-stat envelope and this row's scope.
- **⚠️ NEVER assert the handshake-failure TEXT.** The reference (BoringSSL) sends the TLS alert `unknown ca`; envoy-go (Go stdlib `crypto/tls` server) sends `bad certificate`. A driver asserting the error string cross-side FAILS 100% of the time. Normalize to a stable verdict token (`rejected`) — the `0045` `closeOK` idiom (`test/fixtures/0045-sni-cluster/driver/driver.go:313-323`).

### C4 🟡 Fixture-clone facts the SPEC got wrong

| SPEC §8 claim | RE-DERIVED truth |
|---|---|
| `0103` driver at `inputs/driver.go` | **`test/fixtures/0103-xds-sds-server-cert/driver/driver.go`** (`0103` uses `driver/`; `0018` uses `inputs/`; both live in the registry). `0108` uses **`driver/`** (it clones `0103`'s topology). |
| `0018` inline `tls_certificates` at `envoy-go.yaml:160-174` | The DTC block is **`:164-174`** (`:160-162` are the listener name/address) and is **`filename:`-based, NOT inline** — it templates `{{.ServerCert}}`/`{{.ServerKey}}`/`{{.CACert}}`. It also already carries a STATIC `validation_context` — the exact thing `0108` converts to SDS. |
| "the runner's CompareBytes (`0103/driver.go:171-177`)" | `CompareBytes` is **`test/differential/diff.go:19`**, `func CompareBytes(ref, subj []byte) (Verdict, error)`. `0103/driver.go:171-177` is `driveSide`, the driver's own observable-builder. Distinct things. |
| "clone `0103`×`0018` `pki/`" | The two parents use **INCOMPATIBLE PKI models**: `0103` COMMITS `pki/{ca,leaf,leaf.key}.pem` (generator not committed); `0018` GENERATES at package-load via `pki/gen.go`'s `init()` and **`.gitignore`s** every PEM (24h validity). They cannot be silently crossed — see D2. |
| `StatsAsserter` `fixture.go:70-77` | CORRECT (kept for reference; `0108` no longer needs it — C3). |
| `TLSServedLeaf` cannot present a client cert | **CORRECT, verified** (`test/helpers/tls.go:69` builds `tls.Config{ServerName, RootCAs, MinVersion, MaxVersion}` — no `Certificates` field, no parameter to supply one). `0045`'s `tlsDial` (`driver/driver.go:293`) ALSO has no `Certificates` field — its value to `0108` is the `closeOK` failure-normalization idiom, not the dial itself. |

### C5 🟡 Minor citation drift (corrected; use THESE)

- `NewQUICDownstreamConfig` is **`config.go:90-113`** (SPEC's `:87-88` lands on the doc comment). Its `commonTLSContextToConfig(…, "downstream", nil)` call at **`:108`** is EXACT — and load-bearing: **QUIC reaches the downstream arm WITH a nil provider**, so §3.3a's `|| provider == nil` clause is required for correctness, not belt-and-braces.
- The inline-CVC reject block is **`:233-246`** (header `:233`, four rejects `:234-245`, close `:246`).
- `loadDataSource` is **`internal/tls/datasource.go:20`**, not `config.go`.
- `stream_test.go` tests span **`:55-179`** (SPEC's `:166` is where the LAST test *begins*); `provider_test.go` tests span **`:59-160`** (SPEC's `:137` likewise). Both SPEC ranges are first-start→last-start — fine for orientation, WRONG as insertion anchors.
- `WithSecret` spans **`sdsserver.go:43-46`**; `ensure()` spans **`0103/driver/driver.go:63-87`** (SPEC's `:43-45`/`:63-86` miss the closing brace).
- SPEC §3.4's patch introduces a local `ctc := dtc.GetCommonTlsContext()`. There is **no such variable today** (`boot.go:138` reaches it inline via `dtc.GetCommonTlsContext()`). This is NOT a SPEC error — the patch declares it. Noted only so an IMPL subagent doesn't hunt for an existing `ctc`.
- SPEC §3.2 says "ALL test fakes (in `internal/tls`) gain the method" — the real count is **exactly ONE**: `fakeProvider` (`config_test.go:796-806`). Repo-wide there are exactly **two** `SecretProvider` implementers (`*xds.Provider` `provider.go:47` + that fake), so the interface change costs exactly two implementations.

### C6 ✅ Confirmed EXACT (adopt as-is)

`parseSecret` `secret.go:55-80` · `fetchSecret` `stream.go:38-80` · `applyResponse` `stream.go:86-95` · `errValidation` `stream.go:32` · `SecretProvider` `provider.go:14-16` · `FetchInitialCertificate` `provider.go:47-75` · `SDSStats` `stats.go:12-18` + `RegisterSDSStats` `stats.go:25-40` · `ParseSDSConfig` `config.go:22-79` · `FuzzDiscoveryResponseParse` `fuzz_test.go:71` · `mustValidSecretAnyBytes` `fuzz_test.go:26-64` · `TestParseSecret_WrongOneof` `secret_test.go:175-187` · `NewDownstreamConfig` rcc block `config.go:67-79` · `NewUpstreamConfig` `config.go:118-156` (provider=nil `:143`) · the SDS-reject case `config.go:227-228` · `combined_validation_context` `:229-230` · upstream SDS reject `:208-209` · `loadTrustedCAPool` `config.go:163-173` · arm-5 subtest `config_test.go:936-957` · rcc tests `config_test.go:286-316` + `:318-349` · `FuzzTLSContextParse` `fuzz_test.go:24` · `NewSDSProvider` `boot.go:120-165` (pre-scan `:138`, `seen>1` `:147-148`, node `:151-152`, `RegisterSDSStats` `:163`) · `buildResponse` `sdsserver.go:118-136` (type-URL `:133`) · `0103` observable `driver.go:171-177` · SDS cluster `0103/envoy-go.yaml:81-95` · `scrapeRBACStats` `0018/inputs/driver.go:779` · `0018` scenario-6 mTLS `inputs/driver.go:501-541` (tls.Config `:519-529`).

**Collision re-check (RE-GREPPED this PLAN session, `reference_spec_drafted_identifier_collision_check`):** `parseValidationSecret` · `applyValidationResponse` · `fetchValidationSecret` · `FetchInitialValidationContext` · `WithValidationContext` — **all five FREE**; zero hits in ANY `.go` file repo-wide (`grep -rn --include='*.go'` → exit 1). The only hits are prose in phase-65's own planning docs.

---

## PLAN-time design decisions (the SPEC left these to the PLAN, or C3/C4 forced them)

### D1 The `Secret` type URL is SHARED by both oneof arms — no new type URL

`secretTypeURL()` (`secret.go:17-19`) derives `"type.googleapis.com/" + proto.MessageName(&tlsv3.Secret{})` from the descriptor. A `validation_context` secret is the **same `envoy…tls.v3.Secret` message**, differing only in the `Type` oneof. So `fetchValidationSecret` reuses `secretTypeURL()` verbatim, `sdsserver`'s generic derivation (`:133`) needs NO change, and there is no second resource type on the wire. This simplifies T2 and T7.

### D2 `0108` PKI — generate IN-MEMORY in `ensure()`; NO `pki/` dir *(resolves C4)*

Neither parent's model is adopted. `0103` commits PEMs (no client-cert precedent at all); `0018` generates at `init()` and gitignores them (needs a `defaultOutputDir()` + file I/O + `.gitignore` upkeep). `0108` needs **five** artifacts (CA_served, server leaf, client_good, CA_unserved, client_bad) — committing five PEMs including two client keys is the worst option, and file-generation buys nothing here because:

- The driver returns bootstrap **strings** (`ReferenceBootstrap`/`SubjectConfig`), so it can inject the server cert as an **`inline_string:` DataSource** directly into both yamls — **no file, no `HostMount`, no Docker mount**.
- The SDS-served CA goes out as `inline_bytes` through `sdsserver` — already in memory.
- The client certs are consumed by the driver's own `tls.Config{Certificates:…}` — already in memory.

So NOTHING needs to touch disk. `ensure()` generates all five in-process (the `0018` `pki/gen.go` crypto shape: ECDSA P-256, `ExtKeyUsage: ClientAuth` on the client leaves, `ServerAuth` + a SAN on the server leaf), `sync.Once`-guarded like `0103`'s `ensure()` (`driver/driver.go:63-87`). Freshness is safe: `0108`'s observable is an accept/reject verdict, NOT `0103`'s `serial=`/`san=` identity, so per-run certs change nothing.

> **YAML note for T8:** `inline_string` (not `inline_bytes`) for the PEM — `inline_bytes` is a proto `bytes` field and protojson requires **base64**, while `inline_string` takes the PEM text directly and `dataSourceBytes`/`loadDataSource` both support it. Multi-line PEM inside YAML needs correct block-scalar indentation; build it with an explicit indent helper, not raw `fmt.Sprintf`.

### D3 `0108` serves exactly ONE secret — the flat `sdsserver` state stays flat

`0108`'s server cert is **STATIC** (inline in the yaml, D2); ONLY the `validation_context` arrives via SDS. So each side's `sdsserver` serves exactly one secret, `boot`'s pre-scan sees `seen==1`, and the deferred compose-two edge is never touched. This means:

- `sdsserver`'s single-secret flat state (`secretName`/`certPEM`/`keyPEM`, `:29-31`) does NOT need a refactor to a map/slice. T7 adds two fields + a `buildResponse` branch. `Resources` stays a 1-element slice.
- `buildResponse`'s **ignored `names` parameter** (`:118`, passed at `:107`, never read) stays ignored — harmless with one secret.
- The `first`-gating hang risk (only the FIRST request on a stream gets a Send, `:93`/`:105`) does NOT bite — one `sds_config` opens one stream and makes one request.

**Do NOT generalize `sdsserver` to multi-secret.** That is the deferred compose-two edge; a speculative refactor here is YAGNI and would put the passing `0103` at risk.

### D4 `0108` observable — a NORMALIZED two-arm verdict in ONE byte stream *(resolves C3)*

`CompareBytes` compares a single `[]byte` per side, so BOTH arms must encode into one stream. `0103`'s `driveSide` (`:171-177`) is **unusable** — it reports *what the server presented*; `0108` reports *whether the proxy accepted our client cert*. The driver:

1. **good arm** — dial with `client_good` (chains to the SERVED CA) → handshake succeeds → write a fixed payload → read the TCPEcho echo → record `good=ok echo=<payload>`.
2. **bad arm** — dial with `client_bad` (chains to the UN-SERVED CA) → handshake MUST fail → record `bad=rejected` (normalized; **never** the alert text, C3). If it unexpectedly SUCCEEDS, record `bad=ACCEPTED` — which fails cross-side comparison loudly *and* is the exact vacuous-accept-all signal this fixture exists to catch.

Both sides must emit byte-identical streams. `BackendCount()` returns **1** (`reference_differential_backendcount_min_one`); unlike `0103` the backend IS exercised (the good arm's echo).

### D5 `SecretProvider` gains the method (not a second interface)

`SecretProvider` (`provider.go:14-16`) is single-method today. Adding `FetchInitialValidationContext` costs exactly **two** implementations (C5) — the real `*Provider` (T3) and `fakeProvider` (`config_test.go:796-806`, T3). That is cheap and keeps ONE seam. A second interface would fork the seam and force `internal/tls` to type-assert. **Extend the interface.**

### D6 Stat-name collision is a REAL edge, but out of scope

`RegisterSDSStats` keys `sds.<secretName>.*` on the secret NAME only, not the resource type. Two secrets with DIFFERENT names get distinct trees for free; the SAME name would share counters (`NewCounterIfAbsent` is idempotent — no panic). `0108` uses a distinct name (`validation_ca`), and the compose-two case is already rejected by `seen>1`. **No action.** Noted so a future compose-two row does not rediscover it.

---

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `internal/xds/secret.go` | ADD `parseValidationSecret(resource, wantName, baseDir) (*x509.CertPool, error)` + the CVC-reject roster; ADD `crypto/x509` to the import block (`:3-13`) — **a new PRODUCTION import; today `crypto/x509` appears ONLY in `internal/xds` test files** | 1 |
| `internal/xds/secret_test.go` | applier unit tests (valid → usable pool; wrong name; wrong oneof; each CVC sub-field; absent `trusted_ca`; bad PEM) | 1 |
| `internal/xds/stream.go` | ADD `fetchValidationSecret` + `applyValidationResponse` (parallel to `:38-95`, sharing `errValidation` `:32`) | 2 |
| `internal/xds/stream_test.go` | stream-arm unit tests (initial-request shape; ACK; NACK; transport error) — append after `:179` | 2 |
| `internal/xds/provider.go` | ADD `FetchInitialValidationContext` (parallel to `:47-75`) + the `SecretProvider` interface method (`:14-16`) | 3 |
| `internal/xds/provider_test.go` | provider unit tests (success; timeout; mgmt-down; rejected) — append after `:160` | 3 |
| `internal/tls/config_test.go` | `fakeProvider` (`:796-806`) gains `FetchInitialValidationContext`; arm-5 subtest (`:936-957`) REBUILT with a full `sds_config` + flipped reject→ACCEPT; `require==false`-inert test | 3, 5 |
| `internal/tls/config.go` | reject-lift no-op (`:227-228`); `NewDownstreamConfig` SDS branch (`:67-79`) | 4, 5 |
| `internal/boot/boot.go` | `NewSDSProvider` pre-scan (`:138`) also detects the validation arm | 6 |
| `internal/boot/boot_test.go` | boot tests (validation-via-SDS builds a provider; both-via-SDS → `seen>1`; cert-only unchanged) | 6 |
| `test/helpers/sdsserver/sdsserver.go` | `WithValidationContext` Option + `buildResponse` branch (`:118-136`) | 7 |
| `test/helpers/sdsserver/sdsserver_test.go` | helper unit test (a validation Secret is served with the right oneof) | 7 |
| `test/fixtures/0108-xds-sds-validation-context/` | NEW: `driver/driver.go` + `driver/doc.go` + `envoy.yaml` + `envoy-go.yaml` + `expectations.yaml` + `README.md` (NO `pki/` — D2) | 8 |
| `test/differential/runner_test.go` | blank-import the `0108` driver (after the `0107` line `:134`) | 8 |
| `internal/xds/fuzz_test.go` | a `validation_context` seed for `FuzzDiscoveryResponseParse` (`:71`) + drive `applyValidationResponse` | 9 |
| `internal/tls/fuzz_test.go` | a `require_client_certificate`+SDS-validation seed for `FuzzTLSContextParse` (`:24`) | 9 |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | the TLS/SDS clause at **`:881`** (reject item 5) REJECT → CONSUMED | 10 |
| `docs/envoy-go/{DECISIONS,STATE,ROADMAP}.md`, `PROGRESS.md`, `next-prompt.txt` | ADR-0286 body; STATE header; ROADMAP row 65 `done` + the sentinel narrow at **`:185`**; PROGRESS close; router roll | 11 |

**⚠️ Build-ordering (load-bearing — surface this to every IMPL subagent).** T1/T2/T3 are **additive**: each adds new symbols with NO caller, so `go build ./...` stays green after each, and the landed `*stdtls.Certificate` chain is never touched (the `0103` differential cannot regress). The ONE exception inside that additive run is T3's **interface** change — the moment `FetchInitialValidationContext` joins `SecretProvider`, `fakeProvider` (`internal/tls/config_test.go:796-806`) STOPS COMPILING until it gains the method. **T3 must land both in the SAME commit** or `internal/tls` tests break mid-sequence. T4 (reject no-op) + T5 (apply-point) then wire the chain into `internal/tls`. T6 (boot pre-scan) is what makes a validation-only-SDS listener actually BUILD a provider — **without T6, T5's fetch path is unreachable in a real boot** (`NewSDSProvider` returns `(nil, nil)` → `seen==0` → `NewDownstreamConfig` gets `nil` → the nil-provider reject fires). So the fixture (T8) depends on **T5 AND T6 both** landing; T8 before T6 would fail with `requires a live SDS provider` and look like a fixture bug.

**⚠️ The `"tls: "` fuzz invariant.** `FuzzTLSContextParse` (`internal/tls/fuzz_test.go:78-96`) asserts EVERY error begins `"tls: "`. `xds.ParseSDSConfig` and `FetchInitialValidationContext` return `xds: `-prefixed errors. The landed cert path already wraps them (`"tls: downstream: %w"`, `config.go:213`). **T5's new arm MUST wrap identically** or T9's seed breaks the fuzzer.

---

## Task 1: `internal/xds/secret.go` — `parseValidationSecret` + the CVC-reject roster

**Files:**
- Modify: `internal/xds/secret.go` (import block `:3-13`; append the new func after `parseSecret` `:55-80`)
- Modify: `internal/xds/secret_test.go` (append; helpers `selfSignedPEM` `:93-119`, `anyOf` `:122-129`, `inlineDS` `:132-134` already exist)

**Interfaces:**
- Produces: `func parseValidationSecret(resource *anypb.Any, wantName, baseDir string) (*x509.CertPool, error)` — consumed by T2's `applyValidationResponse`.
- Consumes: `secretTypeURL()` (`:17-19`), `dataSourceBytes(ds *corev3.DataSource, baseDir string) ([]byte, error)` (`:27-48`) — both reused VERBATIM, no change.

**Design (mirrors `parseSecret` `:55-80` step-for-step; the fork is at step 3):**

| `parseSecret` step | `parseValidationSecret` |
|---|---|
| 1 `resource.UnmarshalTo(&sec)` | IDENTICAL — `"xds: sds: resource is not a %s: %w"` |
| 2 `sec.GetName() != wantName` | IDENTICAL — `"xds: sds: response secret name %q != requested %q"` |
| 3 `sec.GetTlsCertificate() == nil` | **FORKS** → `sec.GetValidationContext() == nil` |
| 4-6 chain/key/`X509KeyPair` | **NO ANALOGUE** → the CVC reject roster + `trusted_ca` → `*x509.CertPool` |

- [ ] **Step 1: Write the failing tests** in `internal/xds/secret_test.go` (append at end). One func per case (the package's existing pattern — `secret_test.go` is NOT table-driven); `Errorf` per independent property:

```go
// vcSecret builds a Secret{name, validation_context{trusted_ca: inline caPEM}},
// the phase-65 happy-path shape (SPEC-65 §11 config_dump).
func vcSecret(t *testing.T, name string, caPEM []byte) *anypb.Any {
	t.Helper()
	return anyOf(t, &tlsv3.Secret{
		Name: name,
		Type: &tlsv3.Secret_ValidationContext{ValidationContext: &tlsv3.CertificateValidationContext{
			TrustedCa: inlineDS(caPEM),
		}},
	})
}

func TestParseValidationSecret_Valid(t *testing.T) {
	caPEM, _ := selfSignedPEM(t)
	pool, err := parseValidationSecret(vcSecret(t, "validation_ca", caPEM), "validation_ca", "")
	if err != nil {
		t.Fatalf("parseValidationSecret: unexpected err %v", err)
	}
	if pool == nil {
		t.Fatal("pool is nil")
	}
	// The pool must actually carry the CA — an empty pool would silently
	// accept nothing (or, as ClientCAs, reject every client).
	if got := len(pool.Subjects()); got != 1 { //nolint:staticcheck // Subjects() is fine for a test-only count
		t.Errorf("pool holds %d subjects, want 1", got)
	}
}

func TestParseValidationSecret_WrongName(t *testing.T) {
	caPEM, _ := selfSignedPEM(t)
	_, err := parseValidationSecret(vcSecret(t, "other", caPEM), "validation_ca", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "!= requested") {
		t.Errorf("error = %q, want it to mention the name mismatch", err.Error())
	}
}

// TestParseValidationSecret_WrongOneof is the MIRROR of TestParseSecret_WrongOneof
// (secret_test.go:175): parseSecret rejects a validation_context, and
// parseValidationSecret rejects a tls_certificate. The two appliers stay disjoint.
func TestParseValidationSecret_WrongOneof(t *testing.T) {
	certPEM, keyPEM := selfSignedPEM(t)
	sec := anyOf(t, &tlsv3.Secret{
		Name: "validation_ca",
		Type: &tlsv3.Secret_TlsCertificate{TlsCertificate: &tlsv3.TlsCertificate{
			CertificateChain: inlineDS(certPEM),
			PrivateKey:       inlineDS(keyPEM),
		}},
	})
	_, err := parseValidationSecret(sec, "validation_ca", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "is not a validation_context") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "is not a validation_context")
	}
}

// TestParseValidationSecret_CVCRejects: lifting the SDS envelope is NOT licence to
// silently accept CertificateValidationContext sub-fields envoy-go cannot honor
// (reference_strict_reject_sibling_typeurl_gap). Each mirrors an inline reject
// (internal/tls/config.go:234-245) with an `xds: sds:`-prefixed DISTINCT substring
// (ADR-0080). Errorf per row so one failure does not mask the rest.
func TestParseValidationSecret_CVCRejects(t *testing.T) {
	caPEM, _ := selfSignedPEM(t)
	cases := []struct {
		name    string
		mut     func(*tlsv3.CertificateValidationContext)
		wantSub string
	}{
		{"custom_validator_config", func(v *tlsv3.CertificateValidationContext) {
			v.CustomValidatorConfig = &corev3.TypedExtensionConfig{Name: "x"}
		}, "custom_validator_config is not supported"},
		{"match_typed_subject_alt_names", func(v *tlsv3.CertificateValidationContext) {
			v.MatchTypedSubjectAltNames = []*tlsv3.SubjectAltNameMatcher{{}}
		}, "match_typed_subject_alt_names is not supported"},
		{"verify_certificate_hash", func(v *tlsv3.CertificateValidationContext) {
			v.VerifyCertificateHash = []string{"deadbeef"}
		}, "verify_certificate_hash is not supported"},
		{"verify_certificate_spki", func(v *tlsv3.CertificateValidationContext) {
			v.VerifyCertificateSpki = []string{"c3BraQ=="}
		}, "verify_certificate_spki is not supported"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vc := &tlsv3.CertificateValidationContext{TrustedCa: inlineDS(caPEM)}
			tc.mut(vc)
			sec := anyOf(t, &tlsv3.Secret{
				Name: "validation_ca",
				Type: &tlsv3.Secret_ValidationContext{ValidationContext: vc},
			})
			_, err := parseValidationSecret(sec, "validation_ca", "")
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tc.wantSub)
			}
			if !strings.HasPrefix(err.Error(), "xds: sds: ") {
				t.Errorf("error = %q, want the `xds: sds: ` prefix", err.Error())
			}
		})
	}
}

func TestParseValidationSecret_NoTrustedCa(t *testing.T) {
	sec := anyOf(t, &tlsv3.Secret{
		Name: "validation_ca",
		Type: &tlsv3.Secret_ValidationContext{ValidationContext: &tlsv3.CertificateValidationContext{}},
	})
	_, err := parseValidationSecret(sec, "validation_ca", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "trusted_ca") {
		t.Errorf("error = %q, want it to mention trusted_ca", err.Error())
	}
}

func TestParseValidationSecret_BadPEM(t *testing.T) {
	_, err := parseValidationSecret(vcSecret(t, "validation_ca", []byte("not a pem")), "validation_ca", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "parse failure") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "parse failure")
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (compile error: `parseValidationSecret` undefined):

```
cd internal/xds && go test -run 'TestParseValidationSecret' -count=1 -v .
```
Expected: FAIL — `undefined: parseValidationSecret`.

- [ ] **Step 3: Add `crypto/x509` to the import block** (`internal/xds/secret.go:3-13`) — the FIRST production `crypto/x509` import in `internal/xds` (stdlib; no module change):

```go
import (
	stdtls "crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)
```

- [ ] **Step 4: Implement `parseValidationSecret`** (append after `parseSecret`, i.e. after `secret.go:80`):

```go
// parseValidationSecret unmarshals an SDS-delivered resource into a
// tls.v3.Secret, verifies it carries the validation_context oneof arm under the
// requested name, holds the served CertificateValidationContext to the SAME
// support surface as the inline path (internal/tls/config.go:234-245 — lifting
// the SDS envelope is not licence to silently accept CVC sub-fields envoy-go
// cannot honor, reference_strict_reject_sibling_typeurl_gap), and loads
// trusted_ca into an *x509.CertPool.
//
// It is the validation_context sibling of parseSecret (which stays
// tls_certificate-only). The two are deliberately DISJOINT: each rejects the
// other's oneof arm, so a mis-served secret fails loudly rather than silently
// yielding a zero-value trust anchor.
//
// The CertPool build DUPLICATES internal/tls.loadTrustedCAPool rather than
// calling it: internal/tls imports internal/xds (config.go:13), so the reverse
// edge would cycle (ADR-0278 / reference_xds_config_seam_transitive_cycle_guard).
// This mirrors dataSourceBytes's deliberate duplication of
// internal/tls.loadDataSource. Keep internal/xds's dep set at internal/stats only.
//
// crl is NOT rejected — the inline path does not check it either
// (config.go:233-246), so rejecting here would be a NEW asymmetry. A documented
// SHARED gap, deferred to the CVC-feature follow-on (SPEC-65 §6).
func parseValidationSecret(resource *anypb.Any, wantName, baseDir string) (*x509.CertPool, error) {
	var sec tlsv3.Secret
	if err := resource.UnmarshalTo(&sec); err != nil {
		return nil, fmt.Errorf("xds: sds: resource is not a %s: %w", secretTypeURL(), err)
	}
	if sec.GetName() != wantName {
		return nil, fmt.Errorf("xds: sds: response secret name %q != requested %q", sec.GetName(), wantName)
	}
	vc := sec.GetValidationContext()
	if vc == nil {
		return nil, fmt.Errorf("xds: sds: secret %q is not a validation_context (unsupported oneof arm)", wantName)
	}
	if vc.GetCustomValidatorConfig() != nil {
		return nil, fmt.Errorf("xds: sds: validation secret %q: custom_validator_config is not supported", wantName)
	}
	if len(vc.GetMatchTypedSubjectAltNames()) > 0 {
		return nil, fmt.Errorf("xds: sds: validation secret %q: match_typed_subject_alt_names is not supported", wantName)
	}
	if len(vc.GetVerifyCertificateHash()) > 0 {
		return nil, fmt.Errorf("xds: sds: validation secret %q: verify_certificate_hash is not supported", wantName)
	}
	if len(vc.GetVerifyCertificateSpki()) > 0 {
		return nil, fmt.Errorf("xds: sds: validation secret %q: verify_certificate_spki is not supported", wantName)
	}
	caPEM, err := dataSourceBytes(vc.GetTrustedCa(), baseDir)
	if err != nil {
		return nil, fmt.Errorf("xds: sds: validation secret %q: trusted_ca: %w", wantName, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("xds: sds: validation secret %q: trusted_ca: parse failure", wantName)
	}
	return pool, nil
}
```

- [ ] **Step 5: Run — expect PASS:**

```
cd internal/xds && go test -run 'TestParseValidationSecret' -count=1 -v .
```
Expected: PASS (all subtests). Also confirm `TestParseSecret_WrongOneof` STAYS green (`parseSecret` untouched):
```
cd internal/xds && go test -run 'TestParseSecret' -count=1 -v .
```

- [ ] **Step 6: Liveness breaks** (`-count=1`; confirm WHICH assertion fires; restore byte-identical after each):
  1. Delete the `verify_certificate_hash` reject → expect ONLY `TestParseValidationSecret_CVCRejects/verify_certificate_hash` to fire (`expected error, got nil`). Confirms the roster is not vacuously passing on a shared earlier arm.
  2. Replace `AppendCertsFromPEM(caPEM)`'s guard with `_ = pool.AppendCertsFromPEM(caPEM)` (drop the `if !`) → expect ONLY `_BadPEM` to fire. **Also confirm `_Valid`'s `len(pool.Subjects()) != 1` does NOT fire** — if it does, the pool was never populated and `_Valid` was passing vacuously.
  3. Change `GetValidationContext()` → `GetTlsCertificate()` (i.e. accept the wrong arm) → expect `_WrongOneof` to fire. Confirms the arm check is live.

- [ ] **Step 7: Per-task gates + commit**

```bash
gofmt -l internal/xds/ && go vet ./internal/xds/ && golangci-lint run ./internal/xds/... && go build ./... && go test ./internal/xds/ -count=1
git add internal/xds/secret.go internal/xds/secret_test.go
git commit -m "phase 65 T1: internal/xds parseValidationSecret — the SDS validation_context applier + the CVC-reject roster"
```

---

## Task 2: `internal/xds/stream.go` — `fetchValidationSecret` + `applyValidationResponse`

**Files:**
- Modify: `internal/xds/stream.go` (append after `applyResponse` `:86-95`)
- Modify: `internal/xds/stream_test.go` (append after `:179` — NOT `:166`, which is where the last test BEGINS; see C5)

**Interfaces:**
- Consumes: `parseValidationSecret` (T1); `errValidation` (`stream.go:32`); `secretTypeURL()` (D1 — the SAME type URL, no new one); the `Stream` interface (`:25-28`) + `Node` (`:16-19`) — all unchanged.
- Produces: `func fetchValidationSecret(stream Stream, node Node, secretName, baseDir string) (*x509.CertPool, error)` + `func applyValidationResponse(resp *discoveryv3.DiscoveryResponse, secretName, baseDir string) (*x509.CertPool, error)` — consumed by T3.

**Why duplicate ~40 lines instead of generifying** (the SPEC's LEAN choice, D-SDSVC-PROVIDER Option A): the landed `fetchSecret`/`applyResponse` chain is load-bearing for the PASSING `0103` differential. A generic refactor (Option B) would touch that chain and put `0103` at risk for no behavior gain. The ACK/NACK/nonce dance is byte-identical; only the parse call and the return type differ. Duplication here is the LOW-risk choice and is explicitly sanctioned by SPEC §3.2.

- [ ] **Step 1: Write the failing tests** in `internal/xds/stream_test.go` (append at end; `fakeStream` `:17-38` and the `validSecretAny` helper `:42-53` already exist):

```go
// validVCSecretAny builds an Any-wrapped Secret{name, validation_context{trusted_ca}}
// for the stream-arm tests (the sibling of validSecretAny, stream_test.go:42).
func validVCSecretAny(t *testing.T, name string) *anypb.Any {
	t.Helper()
	caPEM, _ := selfSignedPEM(t)
	sec := &tlsv3.Secret{
		Name: name,
		Type: &tlsv3.Secret_ValidationContext{ValidationContext: &tlsv3.CertificateValidationContext{
			TrustedCa: &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: caPEM}},
		}},
	}
	any, err := anypb.New(sec)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return any
}

func TestFetchValidationSecret_InitialRequestShape(t *testing.T) {
	fs := &fakeStream{resps: []*discoveryv3.DiscoveryResponse{{
		VersionInfo: "v1", Nonce: "n1",
		Resources: []*anypb.Any{validVCSecretAny(t, "validation_ca")},
	}}}
	if _, err := fetchValidationSecret(fs, Node{ID: "id", Cluster: "cl"}, "validation_ca", ""); err != nil {
		t.Fatalf("fetchValidationSecret: %v", err)
	}
	if len(fs.sent) < 1 {
		t.Fatal("no request sent")
	}
	init := fs.sent[0]
	if got := init.GetTypeUrl(); got != secretTypeURL() {
		t.Errorf("initial TypeUrl = %q, want %q (the Secret type URL is SHARED by both oneof arms)", got, secretTypeURL())
	}
	if got := init.GetResourceNames(); len(got) != 1 || got[0] != "validation_ca" {
		t.Errorf("initial ResourceNames = %v, want [validation_ca]", got)
	}
	if init.GetVersionInfo() != "" {
		t.Errorf("initial VersionInfo = %q, want empty", init.GetVersionInfo())
	}
	if init.GetResponseNonce() != "" {
		t.Errorf("initial ResponseNonce = %q, want empty", init.GetResponseNonce())
	}
	if init.GetNode().GetId() != "id" || init.GetNode().GetCluster() != "cl" {
		t.Errorf("initial Node = %v, want {id, cl}", init.GetNode())
	}
}

func TestFetchValidationSecret_AckOnSuccess(t *testing.T) {
	fs := &fakeStream{resps: []*discoveryv3.DiscoveryResponse{{
		VersionInfo: "v7", Nonce: "n7",
		Resources: []*anypb.Any{validVCSecretAny(t, "validation_ca")},
	}}}
	pool, err := fetchValidationSecret(fs, Node{ID: "id", Cluster: "cl"}, "validation_ca", "")
	if err != nil {
		t.Fatalf("fetchValidationSecret: %v", err)
	}
	if pool == nil {
		t.Error("pool is nil on success")
	}
	if len(fs.sent) != 2 {
		t.Fatalf("sent %d requests, want 2 (initial + ACK)", len(fs.sent))
	}
	ack := fs.sent[1]
	if ack.GetVersionInfo() != "v7" {
		t.Errorf("ACK VersionInfo = %q, want v7 (echo the accepted version)", ack.GetVersionInfo())
	}
	if ack.GetResponseNonce() != "n7" {
		t.Errorf("ACK ResponseNonce = %q, want n7", ack.GetResponseNonce())
	}
	if ack.GetErrorDetail() != nil {
		t.Errorf("ACK carries ErrorDetail %v, want nil", ack.GetErrorDetail())
	}
}

func TestFetchValidationSecret_NackOnValidationFailure(t *testing.T) {
	// A tls_certificate secret served where a validation_context was requested:
	// parseValidationSecret rejects it -> errValidation -> NACK.
	fs := &fakeStream{resps: []*discoveryv3.DiscoveryResponse{{
		VersionInfo: "v2", Nonce: "n2",
		Resources: []*anypb.Any{validSecretAny(t, "validation_ca")},
	}}}
	_, err := fetchValidationSecret(fs, Node{ID: "id", Cluster: "cl"}, "validation_ca", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, errValidation) {
		t.Errorf("error = %v, want it to wrap errValidation", err)
	}
	if len(fs.sent) != 2 {
		t.Fatalf("sent %d requests, want 2 (initial + NACK)", len(fs.sent))
	}
	nack := fs.sent[1]
	if nack.GetVersionInfo() != "" {
		t.Errorf("NACK VersionInfo = %q, want empty (keep the PRIOR version on reject)", nack.GetVersionInfo())
	}
	if nack.GetResponseNonce() != "n2" {
		t.Errorf("NACK ResponseNonce = %q, want n2", nack.GetResponseNonce())
	}
	if nack.GetErrorDetail() == nil {
		t.Error("NACK ErrorDetail is nil, want the validation failure detail")
	}
}

func TestFetchValidationSecret_TransportError(t *testing.T) {
	fs := &fakeStream{recvErr: errors.New("boom")}
	_, err := fetchValidationSecret(fs, Node{ID: "id", Cluster: "cl"}, "validation_ca", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errors.Is(err, errValidation) {
		t.Errorf("error = %v, want a TRANSPORT error, not errValidation (a transport failure must not classify as rejected)", err)
	}
	if !strings.Contains(err.Error(), "recv response") {
		t.Errorf("error = %q, want it to mention recv response", err.Error())
	}
}

func TestApplyValidationResponse_EmptyResources(t *testing.T) {
	_, err := applyValidationResponse(&discoveryv3.DiscoveryResponse{VersionInfo: "v1", Nonce: "n1"}, "validation_ca", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, errValidation) {
		t.Errorf("error = %v, want it to wrap errValidation", err)
	}
	if !strings.Contains(err.Error(), "empty resources") {
		t.Errorf("error = %q, want it to mention empty resources", err.Error())
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (`undefined: fetchValidationSecret`, `undefined: applyValidationResponse`):

```
cd internal/xds && go test -run 'TestFetchValidationSecret|TestApplyValidationResponse' -count=1 -v .
```

- [ ] **Step 3: Implement both** (append after `stream.go:95`; add `"crypto/x509"` to the import block `:3-11` and DROP nothing — `stdtls` is still used by the landed chain):

```go
// fetchValidationSecret runs ONE State-of-the-World exchange for an SDS-delivered
// validation_context: send the initial DiscoveryRequest, receive one
// DiscoveryResponse, ACK on success / NACK on validation failure, and return the
// trusted-CA pool.
//
// It is byte-parallel to fetchSecret (stream.go:38) — the version/nonce ACK/NACK
// dance is IDENTICAL and errValidation is SHARED. Only the applier call and the
// return type differ. The duplication is deliberate (SPEC-65 §3.2, Option A): the
// landed *stdtls.Certificate chain is load-bearing for the passing 0103
// differential, and generifying it would put 0103 at risk for no behavior gain.
//
// The requested type URL is secretTypeURL() — the SAME as the cert path: a
// validation_context is the same tls.v3.Secret message, differing only in the
// Type oneof arm. There is no second resource type on the wire.
func fetchValidationSecret(stream Stream, node Node, secretName, baseDir string) (*x509.CertPool, error) {
	initial := &discoveryv3.DiscoveryRequest{
		VersionInfo:   "",
		ResponseNonce: "",
		ResourceNames: []string{secretName},
		TypeUrl:       secretTypeURL(),
		Node:          &corev3.Node{Id: node.ID, Cluster: node.Cluster},
	}
	if err := stream.Send(initial); err != nil {
		return nil, fmt.Errorf("xds: sds: send initial request: %w", err)
	}
	resp, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("xds: sds: recv response: %w", err)
	}
	pool, verr := applyValidationResponse(resp, secretName, baseDir)
	if verr != nil {
		nack := &discoveryv3.DiscoveryRequest{
			VersionInfo:   initial.VersionInfo, // keep the PRIOR version on reject
			ResponseNonce: resp.GetNonce(),
			ResourceNames: []string{secretName},
			TypeUrl:       secretTypeURL(),
			Node:          &corev3.Node{Id: node.ID, Cluster: node.Cluster},
			ErrorDetail:   &statuspb.Status{Message: verr.Error()},
		}
		_ = stream.Send(nack) // best-effort: the fetch already failed
		return nil, verr
	}
	ack := &discoveryv3.DiscoveryRequest{
		VersionInfo:   resp.GetVersionInfo(),
		ResponseNonce: resp.GetNonce(),
		ResourceNames: []string{secretName},
		TypeUrl:       secretTypeURL(),
		Node:          &corev3.Node{Id: node.ID, Cluster: node.Cluster},
	}
	if err := stream.Send(ack); err != nil {
		return nil, fmt.Errorf("xds: sds: send ack: %w", err)
	}
	return pool, nil
}

// applyValidationResponse validates a DiscoveryResponse carrying a
// validation_context Secret and returns the trusted-CA pool. Every failure wraps
// errValidation so the caller NACKs (and the Provider classifies it as
// update_rejected rather than update_failure). Sibling of applyResponse
// (stream.go:86); like it, only resources[0] is read.
func applyValidationResponse(resp *discoveryv3.DiscoveryResponse, secretName, baseDir string) (*x509.CertPool, error) {
	if len(resp.GetResources()) == 0 {
		return nil, fmt.Errorf("%w: empty resources", errValidation)
	}
	pool, err := parseValidationSecret(resp.GetResources()[0], secretName, baseDir)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errValidation, err)
	}
	return pool, nil
}
```

- [ ] **Step 4: Run — expect PASS:**

```
cd internal/xds && go test -run 'TestFetchValidationSecret|TestApplyValidationResponse' -count=1 -v .
```

- [ ] **Step 5: Liveness breaks** (`-count=1`; confirm WHICH fired; restore after each):
  1. Change the NACK's `VersionInfo` to `resp.GetVersionInfo()` → expect ONLY `_NackOnValidationFailure` (`want empty (keep the PRIOR version on reject)`).
  2. Drop the `ErrorDetail` field from the NACK → expect ONLY `_NackOnValidationFailure`'s ErrorDetail assertion.
  3. In `applyValidationResponse`, drop the `%w: ` on the parse-failure wrap → expect `_NackOnValidationFailure`'s `errors.Is(err, errValidation)` to fire. Confirms the NACK classification is live (and that T3's `update_rejected` accounting will work).

- [ ] **Step 6: Gates + commit** (same six commands as T1, substituting the message):

```bash
git add internal/xds/stream.go internal/xds/stream_test.go
git commit -m "phase 65 T2: internal/xds fetchValidationSecret + applyValidationResponse — the parallel SotW arm"
```

---

## Task 3: `internal/xds/provider.go` — `FetchInitialValidationContext` + the interface method

**Files:**
- Modify: `internal/xds/provider.go` (`SecretProvider` `:14-16`; append the method after `FetchInitialCertificate` `:47-75`)
- Modify: `internal/xds/provider_test.go` (append after `:160` — NOT `:137`; see C5)
- Modify: `internal/tls/config_test.go` (`fakeProvider` `:796-806` gains the method) — **SAME COMMIT, see below**

**Interfaces:**
- Consumes: `fetchValidationSecret` (T2); `p.timeout`/`p.stats`/`p.opener`/`p.node`/`p.baseDir` (`Provider` `:28-34`) — unchanged.
- Produces: `FetchInitialValidationContext(ctx context.Context, secretName string) (*x509.CertPool, error)` on `SecretProvider` — consumed by T5.

> **⚠️ THE INTERFACE CHANGE BREAKS THE BUILD MID-TASK.** The moment `FetchInitialValidationContext` joins `SecretProvider` (`provider.go:14-16`), `internal/tls`'s `fakeProvider` (`config_test.go:796-806`) no longer satisfies it and `internal/tls` tests STOP COMPILING. There are EXACTLY TWO implementers repo-wide (`*Provider` + that fake — C5). **Land the interface method, the `*Provider` method, and the `fakeProvider` method in the SAME commit.** Do not split them across tasks.

- [ ] **Step 1: Write the failing tests** in `internal/xds/provider_test.go` (append at end; `grpcTestOpener` `:24-40` and `closedAddr` `:46-57` already exist). **`sdsserver.WithValidationContext` does not exist until T7** — so these tests use a local fake opener/stream rather than the real gRPC server, keeping T3 independent of T7:

```go
// vcFakeOpener returns a Stream serving a canned validation_context response —
// keeping T3's provider tests independent of the sdsserver extension (T7).
type vcFakeOpener struct {
	resp   *discoveryv3.DiscoveryResponse
	openErr error
	block  bool // when true, Recv blocks until ctx cancels (drives the timeout path)
	ctx    context.Context
}

func (o *vcFakeOpener) StreamSecrets(ctx context.Context) (Stream, error) {
	if o.openErr != nil {
		return nil, o.openErr
	}
	o.ctx = ctx
	return &vcFakeStream{o: o}, nil
}

type vcFakeStream struct{ o *vcFakeOpener }

func (s *vcFakeStream) Send(*discoveryv3.DiscoveryRequest) error { return nil }
func (s *vcFakeStream) Recv() (*discoveryv3.DiscoveryResponse, error) {
	if s.o.block {
		<-s.o.ctx.Done()
		return nil, s.o.ctx.Err()
	}
	return s.o.resp, nil
}

func TestProvider_FetchInitialValidationContext_Success(t *testing.T) {
	reg := stats.NewRegistry()
	st := RegisterSDSStats(reg, "validation_ca")
	op := &vcFakeOpener{resp: &discoveryv3.DiscoveryResponse{
		VersionInfo: "v1", Nonce: "n1",
		Resources: []*anypb.Any{validVCSecretAny(t, "validation_ca")},
	}}
	p := NewProvider(op, Node{ID: "id", Cluster: "cl"}, "", time.Second, st)
	pool, err := p.FetchInitialValidationContext(context.Background(), "validation_ca")
	if err != nil {
		t.Fatalf("FetchInitialValidationContext: %v", err)
	}
	if pool == nil {
		t.Error("pool is nil on success")
	}
	if got := counterValue(t, reg, "sds.validation_ca.update_success"); got != 1 {
		t.Errorf("update_success = %d, want 1", got)
	}
	if got := counterValue(t, reg, "sds.validation_ca.update_attempt"); got != 1 {
		t.Errorf("update_attempt = %d, want 1", got)
	}
}

func TestProvider_FetchInitialValidationContext_Timeout(t *testing.T) {
	reg := stats.NewRegistry()
	st := RegisterSDSStats(reg, "validation_ca")
	p := NewProvider(&vcFakeOpener{block: true}, Node{ID: "id", Cluster: "cl"}, "", 50*time.Millisecond, st)
	_, err := p.FetchInitialValidationContext(context.Background(), "validation_ca")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "initial fetch timed out") {
		t.Errorf("error = %q, want it to mention the initial-fetch timeout", err.Error())
	}
	if got := counterValue(t, reg, "sds.validation_ca.init_fetch_timeout"); got != 1 {
		t.Errorf("init_fetch_timeout = %d, want 1", got)
	}
}

func TestProvider_FetchInitialValidationContext_MgmtDown(t *testing.T) {
	reg := stats.NewRegistry()
	st := RegisterSDSStats(reg, "validation_ca")
	p := NewProvider(&vcFakeOpener{openErr: errors.New("dial refused")}, Node{ID: "id", Cluster: "cl"}, "", time.Second, st)
	_, err := p.FetchInitialValidationContext(context.Background(), "validation_ca")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "open stream") {
		t.Errorf("error = %q, want it to mention open stream", err.Error())
	}
	if got := counterValue(t, reg, "sds.validation_ca.update_failure"); got != 1 {
		t.Errorf("update_failure = %d, want 1", got)
	}
}

func TestProvider_FetchInitialValidationContext_Rejected(t *testing.T) {
	reg := stats.NewRegistry()
	st := RegisterSDSStats(reg, "validation_ca")
	// A tls_certificate served where a validation_context was requested -> rejected.
	op := &vcFakeOpener{resp: &discoveryv3.DiscoveryResponse{
		VersionInfo: "v1", Nonce: "n1",
		Resources: []*anypb.Any{validSecretAny(t, "validation_ca")},
	}}
	p := NewProvider(op, Node{ID: "id", Cluster: "cl"}, "", time.Second, st)
	_, err := p.FetchInitialValidationContext(context.Background(), "validation_ca")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := counterValue(t, reg, "sds.validation_ca.update_rejected"); got != 1 {
		t.Errorf("update_rejected = %d, want 1 (a validation failure classifies as rejected, not failure)", got)
	}
	if got := counterValue(t, reg, "sds.validation_ca.update_failure"); got != 0 {
		t.Errorf("update_failure = %d, want 0 (a reject must NOT also count as a failure)", got)
	}
}
```

> **Note:** `counterValue(t, reg, name)` is the existing stats-read idiom used by `internal/xds/stats_test.go` (`TestRegisterSDSStats_FiveCounterDelta` `:20`). RE-READ `stats_test.go` and reuse its EXACT accessor — do not invent one. If it is inlined rather than a helper, extract it or inline the same read here.

- [ ] **Step 2: Run — expect FAIL** (`undefined: FetchInitialValidationContext`):

```
cd internal/xds && go test -run 'TestProvider_FetchInitialValidationContext' -count=1 -v .
```

- [ ] **Step 3: Extend the `SecretProvider` interface** (`provider.go:14-16`):

```go
// SecretProvider fetches SDS-delivered secrets. Both methods are INITIAL-FETCH
// only (no rotation/watch — SPEC-60 scope, unchanged at phase 65).
//
// The two methods are parallel chains over the SAME SotW machinery, differing in
// resource type and return type: a tls_certificate yields a *stdtls.Certificate
// (a downstream server leaf, phase 60.2 / ADR-0280); a validation_context yields
// an *x509.CertPool (a downstream mTLS trusted-CA, phase 65 / ADR-0286).
type SecretProvider interface {
	FetchInitialCertificate(ctx context.Context, secretName string) (*stdtls.Certificate, error)
	FetchInitialValidationContext(ctx context.Context, secretName string) (*x509.CertPool, error)
}
```
Add `"crypto/x509"` to `provider.go`'s import block (`:3-9`).

- [ ] **Step 4: Implement the `*Provider` method** (append after `provider.go:75`) — byte-parallel to `FetchInitialCertificate`, with the error-classification switch VERBATIM (note `errValidation` is checked BEFORE `ctx.Err()`, so a validation failure racing a deadline classifies as rejected — preserve that order):

```go
// FetchInitialValidationContext performs the ONE bounded initial fetch of an
// SDS-delivered validation_context and returns the trusted-CA pool.
//
// Byte-parallel to FetchInitialCertificate (provider.go:47): the same timeout
// bound, the same SDSStats accounting, and the same error classification
// (rejected / init-fetch-timeout / failure). A timeout or unreachable management
// server returns an error, which boot-FAILS the listener — the documented
// envoy-go DEPARTURE from the reference's serve-anyway (ADR-0280, extended
// unchanged to this resource type; SPEC-65 §11 D-SDSVC-FETCHTIMEOUT).
func (p *Provider) FetchInitialValidationContext(ctx context.Context, secretName string) (*x509.CertPool, error) {
	p.stats.incUpdateAttempt()
	if p.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}
	stream, err := p.opener.StreamSecrets(ctx)
	if err != nil {
		p.stats.incUpdateFailure()
		return nil, fmt.Errorf("xds: sds: secret %q: open stream: %w", secretName, err)
	}
	pool, err := fetchValidationSecret(stream, p.node, secretName, p.baseDir)
	if err != nil {
		switch {
		case errors.Is(err, errValidation):
			p.stats.incUpdateRejected()
			return nil, err
		case ctx.Err() != nil: // deadline / cancel during recv
			p.stats.incInitFetchTimeout()
			return nil, fmt.Errorf("xds: sds: secret %q: initial fetch timed out after %s: %w", secretName, p.timeout, ctx.Err())
		default:
			p.stats.incUpdateFailure()
			return nil, err
		}
	}
	p.stats.incUpdateSuccess()
	return pool, nil
}
```

- [ ] **Step 5: Teach `fakeProvider` the method — SAME COMMIT** (`internal/tls/config_test.go:796-806`). Add `pool`/`vcErr` fields so T5 can drive both outcomes:

```go
// fakeProvider is a test-only xds.SecretProvider whose fetches return canned
// values. It mirrors the shape of internal/xds's real Provider without any of the
// stream-opening machinery.
type fakeProvider struct {
	cert *stdtls.Certificate
	err  error

	pool  *x509.CertPool // returned by FetchInitialValidationContext
	vcErr error          // returned by FetchInitialValidationContext
}

func (f *fakeProvider) FetchInitialCertificate(ctx context.Context, secretName string) (*stdtls.Certificate, error) {
	return f.cert, f.err
}

func (f *fakeProvider) FetchInitialValidationContext(ctx context.Context, secretName string) (*x509.CertPool, error) {
	return f.pool, f.vcErr
}
```
`crypto/x509` is ALREADY imported in `internal/tls/config.go:6`; confirm it is imported in `config_test.go` too and add it if not.

- [ ] **Step 6: Run — expect PASS, BOTH packages** (the interface change is why `internal/tls` must be run here):

```
cd internal/xds && go test -run 'TestProvider_FetchInitialValidationContext' -count=1 -v .
go build ./... && go test ./internal/xds/ ./internal/tls/ -count=1
```
Expected: PASS. If `internal/tls` fails to COMPILE, Step 5 was missed.

- [ ] **Step 7: Liveness breaks** (`-count=1`; confirm WHICH fired; restore):
  1. Move `p.stats.incUpdateAttempt()` below the opener call → expect `_MgmtDown`'s `update_attempt` (if asserted) or `_Success`'s to fire. Confirms attempt is counted unconditionally, first.
  2. Swap the switch's first two cases (`ctx.Err()` before `errValidation`) → expect `_Rejected`'s `update_rejected = 0, want 1` to fire ONLY if a deadline is racing; if it does NOT fire, the ordering is untested here — **record that honestly in PROGRESS** rather than claiming coverage.
  3. Change `incUpdateRejected()` → `incUpdateFailure()` → expect `_Rejected` to fire on BOTH assertions (`update_rejected = 0` and `update_failure = 1`). This one is unambiguous; prefer it as the classification proof.

- [ ] **Step 8: Gates + commit**

```bash
gofmt -l internal/xds/ internal/tls/ && go vet ./internal/xds/ ./internal/tls/ && golangci-lint run ./internal/xds/... ./internal/tls/... && go build ./... && go test ./internal/xds/ ./internal/tls/ -count=1
git add internal/xds/provider.go internal/xds/provider_test.go internal/tls/config_test.go
git commit -m "phase 65 T3: xds.SecretProvider.FetchInitialValidationContext + the fakeProvider impl (one commit — the interface change breaks the build otherwise)"
```

---

## Task 4: `internal/tls/config.go` — lift the reject to a no-op for downstream+provider

**Files:**
- Modify: `internal/tls/config.go` (`commonTLSContextToConfig`'s `ValidationContextSdsSecretConfig` case, `:227-228`)
- Modify: `internal/tls/config_test.go` (add the upstream/QUIC still-reject tests)

**Interfaces:**
- Consumes: `side string` + `provider xds.SecretProvider` — BOTH already params of `commonTLSContextToConfig` (`:192`, exact signature `func commonTLSContextToConfig(c *tlsv3.CommonTlsContext, baseDir, side string, provider xds.SecretProvider) (*stdtls.Config, error)`).
- Produces: the downstream+provider path FALLS THROUGH (the fetch happens in `NewDownstreamConfig`, T5).

> **⚠️ Why the guard is `side != "downstream" || provider == nil` and NOT just `side != "downstream"`.** `NewQUICDownstreamConfig` (`config.go:90-113`) calls `commonTLSContextToConfig(dtc.GetCommonTlsContext(), baseDir, "downstream", nil)` at **`:108`** — i.e. QUIC reaches this arm with `side == "downstream"` AND a nil provider. Without the `|| provider == nil` clause, a QUIC listener with an SDS validation_context would fall through here and then nil-deref (or silently skip validation) downstream. The clause is CORRECTNESS, not defensiveness. QUIC carries no SDS (SPEC §8).

- [ ] **Step 1: Write the failing tests** in `internal/tls/config_test.go` (append near the other SDS tests; `sdsDownstreamTS` `:846` / `sdsUpstreamTS` `:859` / `sdsSecretConfig` `:813` already exist — RE-READ them and reuse):

```go
// TestValidationContextSDS_SiblingRejectsStay: phase 65 lifts the downstream
// reject ONLY for the live-provider path. Upstream and QUIC (both of which reach
// commonTLSContextToConfig with a nil provider) keep the BYTE-IDENTICAL phase-03
// reject substring (ADR-0080). Errorf per arm so one failure does not mask the rest.
func TestValidationContextSDS_SiblingRejectsStay(t *testing.T) {
	const wantSub = "SDS-bound validation_context_sds_secret_config is not supported in phase 03"

	t.Run("upstream still rejects", func(t *testing.T) {
		ts := makeTransportSocket(t, &tlsv3.UpstreamTlsContext{
			CommonTlsContext: &tlsv3.CommonTlsContext{
				ValidationContextType: &tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig{
					ValidationContextSdsSecretConfig: sdsSecretConfig("validation_ca", "sds_cluster"),
				},
			},
		})
		_, err := NewUpstreamConfig(ts, "")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), wantSub) {
			t.Errorf("upstream error = %q, want it to contain %q", err.Error(), wantSub)
		}
	})

	t.Run("quic downstream (nil provider) still rejects", func(t *testing.T) {
		ts := makeTransportSocket(t, &quicv3.QuicDownstreamTransport{
			DownstreamTlsContext: &tlsv3.DownstreamTlsContext{
				CommonTlsContext: &tlsv3.CommonTlsContext{
					ValidationContextType: &tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig{
						ValidationContextSdsSecretConfig: sdsSecretConfig("validation_ca", "sds_cluster"),
					},
				},
			},
		})
		_, err := NewQUICDownstreamConfig(ts, "")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), wantSub) {
			t.Errorf("quic error = %q, want it to contain %q", err.Error(), wantSub)
		}
	})

	t.Run("downstream with NIL provider still rejects", func(t *testing.T) {
		ts := sdsDownstreamTS(t, nil /* no cert SDS */, sdsSecretConfig("validation_ca", "sds_cluster"))
		_, err := NewDownstreamConfig(ts, "", nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		// Either this reject or T5's nil-provider reject is acceptable here; both
		// are `tls: `-prefixed and both refuse. Assert the REFUSAL + the prefix.
		if !strings.HasPrefix(err.Error(), "tls: ") {
			t.Errorf("error = %q, want the `tls: ` prefix (the FuzzTLSContextParse invariant)", err.Error())
		}
	})
}
```

> **⚠️ RE-READ `sdsDownstreamTS` (`config_test.go:846`) BEFORE writing this.** Its current signature is shaped for the CERT SDS path. If it does not accept a validation config, either extend it (preferred — keep ONE builder) or build the `TransportSocket` inline. Do NOT assume the signature above; DERIVE it. The same applies to `makeTransportSocket` and `quicv3` (confirm the QUIC wrapper message name from `config.go:90-113`'s unmarshal target — do not guess it).

- [ ] **Step 2: Run — expect PASS for upstream/QUIC** (they already reject today) **and FAIL/PASS for the third** depending on current behavior. This task's test is a REGRESSION FENCE, not a red test — it pins what must NOT change. Record its pre-change result in PROGRESS, then confirm it still passes after Step 3.

```
cd internal/tls && go test -run 'TestValidationContextSDS_SiblingRejectsStay' -count=1 -v .
```

- [ ] **Step 3: Apply the no-op lift** (`config.go:227-228`):

```go
		case *tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig:
			// Phase 65 (ADR-0286): a downstream SDS-delivered validation_context is
			// HONORED — fetched and installed as ClientCAs by NewDownstreamConfig's
			// require_client_certificate block (the only scope that sees
			// require_client_certificate, which lives on the DownstreamTlsContext,
			// not on this CommonTlsContext). Here it is a NO-OP for the
			// downstream+provider path.
			//
			// Upstream (NewUpstreamConfig, config.go:143) and QUIC
			// (NewQUICDownstreamConfig, config.go:108) both reach this arm with a NIL
			// provider — QUIC even with side == "downstream" — so both keep the
			// BYTE-IDENTICAL phase-03 reject (ADR-0080 distinct substring).
			if side != "downstream" || provider == nil {
				return nil, fmt.Errorf("tls: %s: SDS-bound validation_context_sds_secret_config is not supported in phase 03", side)
			}
```
The `combined_validation_context` case (`:229-230`) and the inline CVC rejects (`:233-246`) are UNCHANGED.

- [ ] **Step 4: Run — expect PASS** (the fence holds; the downstream+provider path no longer rejects HERE but T5 has not wired the fetch yet, so a downstream+provider SDS-validation context currently falls through to `NewDownstreamConfig`'s inline `else` branch and hits `require_client_certificate=true requires validation_context.trusted_ca` — that is EXPECTED between T4 and T5):

```
cd internal/tls && go test ./... -count=1
```

- [ ] **Step 5: Liveness break** (`-count=1`): drop the `|| provider == nil` clause → expect `quic downstream (nil provider) still rejects` to fire. This is the C5/QUIC proof — if it does NOT fire, the QUIC arm is not reaching this case and the test is vacuous; investigate before proceeding. Restore.

- [ ] **Step 6: Gates + commit**

```bash
gofmt -l internal/tls/ && go vet ./internal/tls/ && golangci-lint run ./internal/tls/... && go build ./... && go test ./internal/tls/ -count=1
git add internal/tls/config.go internal/tls/config_test.go
git commit -m "phase 65 T4: internal/tls — lift the downstream+provider validation_context SDS reject to a no-op (upstream/QUIC keep it)"
```

---

## Task 5: `internal/tls/config.go` — the `ClientCAs` apply-point

**Files:**
- Modify: `internal/tls/config.go` (`NewDownstreamConfig`'s `require_client_certificate` block, `:67-79`)
- Modify: `internal/tls/config_test.go` (REBUILD + flip the arm-5 subtest `:936-957`; ADD the `require==false`-inert test; KEEP `:286-316` + `:318-349`)

**Interfaces:**
- Consumes: `provider.FetchInitialValidationContext` (T3); `xds.ParseSDSConfig` (`internal/xds/config.go:22`, ALL 14 arms — C1); `loadTrustedCAPool` (`config.go:163-173`, the inline `else` branch, byte-identical).
- Produces: `cfg.ClientCAs` populated from SDS; `cfg.ClientAuth = RequireAndVerifyClientCert`.

> **⚠️ C2 — THE VACUITY TRAP. READ BEFORE WRITING THE TEST.** The arm-5 subtest (`:936-957`) currently passes `&tlsv3.SdsSecretConfig{Name: "validation-secret"}` with **NO `sds_config`**. Routed through `xds.ParseSDSConfig`, that input fires `xds: sds: SdsSecretConfig sds_config is required` (`internal/xds/config.go:33`) and NEVER reaches the provider. Flipping the assertion to ACCEPT against that input would test NOTHING. **The replacement MUST supply a full `sds_config`** (via `sdsSecretConfig`, `config_test.go:813`).

> **⚠️ The `"tls: "` fuzz invariant.** Every new error MUST be `tls: `-prefixed (`FuzzTLSContextParse`, `fuzz_test.go:78-96`). `ParseSDSConfig` and `FetchInitialValidationContext` return `xds: `-prefixed errors → **wrap them** (`"tls: downstream: %w"`), exactly as the landed cert path does at `config.go:213`.

- [ ] **Step 1: Rewrite the arm-5 subtest + add the inert test** (`internal/tls/config_test.go:936-957`, inside `TestNewDownstreamConfig_SDS` `:877`). REPLACE the whole subtest:

```go
	t.Run("validation_context_sds_secret_config fetches and installs ClientCAs (arm 5, phase 65)", func(t *testing.T) {
		// NOTE (phase 65): the pre-65 version of this subtest passed an
		// SdsSecretConfig with NO sds_config, which — now that the path routes
		// through xds.ParseSDSConfig — would fire the `sds_config is required`
		// envelope reject and never reach the provider, making the ACCEPT
		// assertion VACUOUS. A FULL sds_config is required here.
		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(pki.caPEM) {
			t.Fatal("pki.caPEM: no certificates parsed")
		}
		fp := &fakeProvider{pool: caPool}
		ts := makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
			RequireClientCertificate: &wrapperspb.BoolValue{Value: true},
			CommonTlsContext: &tlsv3.CommonTlsContext{
				TlsCertificates: []*tlsv3.TlsCertificate{
					{
						CertificateChain: inlineBytes(pki.leafCertPEM),
						PrivateKey:       inlineBytes(pki.leafKeyPEM),
					},
				},
				ValidationContextType: &tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig{
					ValidationContextSdsSecretConfig: sdsSecretConfig("validation-secret", "sds_cluster"),
				},
			},
		})
		cfg, err := NewDownstreamConfig(ts, "", fp)
		if err != nil {
			t.Fatalf("NewDownstreamConfig: unexpected err %v", err)
		}
		if cfg.ClientCAs == nil {
			t.Error("ClientCAs is nil — the SDS-delivered validation_context was not installed")
		}
		if cfg.ClientAuth != stdtls.RequireAndVerifyClientCert {
			t.Errorf("ClientAuth = %v, want RequireAndVerifyClientCert (mandatory mTLS)", cfg.ClientAuth)
		}
	})

	t.Run("validation_context_sds fetch failure boot-FAILS (the ADR-0280 departure, extended)", func(t *testing.T) {
		fp := &fakeProvider{vcErr: errors.New("xds: sds: secret \"validation-secret\": initial fetch timed out after 15s: context deadline exceeded")}
		ts := makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
			RequireClientCertificate: &wrapperspb.BoolValue{Value: true},
			CommonTlsContext: &tlsv3.CommonTlsContext{
				TlsCertificates: []*tlsv3.TlsCertificate{
					{
						CertificateChain: inlineBytes(pki.leafCertPEM),
						PrivateKey:       inlineBytes(pki.leafKeyPEM),
					},
				},
				ValidationContextType: &tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig{
					ValidationContextSdsSecretConfig: sdsSecretConfig("validation-secret", "sds_cluster"),
				},
			},
		})
		_, err := NewDownstreamConfig(ts, "", fp)
		if err == nil {
			t.Fatal("expected a boot failure, got nil (envoy-go boot-FAILS where the reference serves anyway — ADR-0280)")
		}
		if !strings.HasPrefix(err.Error(), "tls: ") {
			t.Errorf("error = %q, want the `tls: ` prefix (the FuzzTLSContextParse invariant)", err.Error())
		}
	})

	t.Run("require_client_certificate=false leaves the SDS validation_context INERT", func(t *testing.T) {
		// Mirrors the landed inline behavior: an inline validation_context with
		// require_client_certificate=false is ALSO inert (only the require==true
		// block loads ClientCAs). Phase 65 introduces no NEW inconsistency, and
		// crucially performs NO boot-time SDS fetch for this shape (SPEC-65 §3.5).
		fp := &fakeProvider{vcErr: errors.New("FETCH MUST NOT HAPPEN")}
		ts := makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
			// RequireClientCertificate deliberately absent (false).
			CommonTlsContext: &tlsv3.CommonTlsContext{
				TlsCertificates: []*tlsv3.TlsCertificate{
					{
						CertificateChain: inlineBytes(pki.leafCertPEM),
						PrivateKey:       inlineBytes(pki.leafKeyPEM),
					},
				},
				ValidationContextType: &tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig{
					ValidationContextSdsSecretConfig: sdsSecretConfig("validation-secret", "sds_cluster"),
				},
			},
		})
		cfg, err := NewDownstreamConfig(ts, "", fp)
		if err != nil {
			t.Fatalf("NewDownstreamConfig: unexpected err %v (the fetch must be SKIPPED, not attempted)", err)
		}
		if cfg.ClientCAs != nil {
			t.Error("ClientCAs is non-nil — require_client_certificate=false must leave the SDS validation_context inert")
		}
		if cfg.ClientAuth != stdtls.NoClientCert {
			t.Errorf("ClientAuth = %v, want NoClientCert (inert)", cfg.ClientAuth)
		}
	})
```

> RE-DERIVE `pki.caPEM` / `pki.leafCertPEM` / `pki.leafKeyPEM` / `inlineBytes` / `makeTransportSocket` / `sdsSecretConfig` from `config_test.go` before writing — the names above come from the arm-5 subtest as it stands (`:936-957`), but CONFIRM each. The `wrapperspb` import is already used by the rcc tests (`:286-316`).

- [ ] **Step 2: Run — expect FAIL:**

```
cd internal/tls && go test -run 'TestNewDownstreamConfig_SDS' -count=1 -v .
```
Expected: FAIL — `require_client_certificate=true requires validation_context.trusted_ca` (the inline `else` branch catches the SDS shape, because `common.GetValidationContext()` returns nil for the SDS oneof — `config.go:69-70`).

- [ ] **Step 3: Implement the apply-point** — REPLACE `config.go:67-79` in full:

```go
	if ctx.GetRequireClientCertificate().GetValue() {
		common := ctx.GetCommonTlsContext()
		if sdsVC, ok := common.GetValidationContextType().(*tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig); ok {
			// Phase 65 (ADR-0286): the trusted-CA bundle is delivered by an SDS
			// management server over the landed SotW stream, bounded by
			// initial_fetch_timeout. The fetch lives HERE (not in
			// commonTLSContextToConfig) because require_client_certificate is a
			// DownstreamTlsContext field, invisible from the CommonTlsContext — and
			// gating on it means the deferred require==false case performs NO
			// wasteful boot-time fetch (SPEC-65 §3.3b / §3.5).
			if provider == nil {
				return nil, fmt.Errorf("tls: downstream: SDS-delivered validation_context requires a live SDS provider (unavailable in this mode)")
			}
			// ParseSDSConfig takes a LIST (it enforces len==1, internal/xds/config.go:23);
			// validation_context_sds_secret_config is SINGULAR, so wrap it.
			secretName, _, _, err := xds.ParseSDSConfig([]*tlsv3.SdsSecretConfig{sdsVC.ValidationContextSdsSecretConfig})
			if err != nil {
				// xds: -prefixed -> wrap to preserve the `tls: ` invariant
				// (FuzzTLSContextParse, fuzz_test.go:78).
				return nil, fmt.Errorf("tls: downstream: %w", err)
			}
			pool, err := provider.FetchInitialValidationContext(context.Background(), secretName)
			if err != nil {
				// A timeout / unreachable management server boot-FAILS the listener —
				// the documented envoy-go DEPARTURE from the reference's serve-anyway
				// (ADR-0280, extended to this resource type).
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
The `else` branch is the byte-identical pre-65 inline path. `context`, `crypto/x509`, `xds`, and `tlsv3` are ALL already imported (`config.go:3-14`) — **no import change**.

- [ ] **Step 4: Run — expect PASS** (and the pre-existing rcc tests `:286-316`/`:318-349` STAY green — they exercise the `else` branch):

```
cd internal/tls && go test -count=1 ./...
```

- [ ] **Step 5: Liveness breaks** (`-count=1`; confirm WHICH fired; restore):
  1. **The C2 vacuity proof.** Revert the arm-5 input to the pre-65 `&tlsv3.SdsSecretConfig{Name: "validation-secret"}` (no `sds_config`) → the subtest MUST fail with `sds_config is required`. This proves the full `sds_config` is load-bearing and the ACCEPT is not vacuous. Restore the full config.
  2. Delete `cfg.ClientCAs = pool` in the SDS branch → expect ONLY the arm-5 `ClientCAs is nil` assertion.
  3. Remove the `provider == nil` guard → expect a nil-deref PANIC in the `downstream with NIL provider still rejects` arm from T4 (`config_test.go`). If instead T4's reject fires first, the guard is unreachable — **record that honestly**; it would mean `commonTLSContextToConfig` already refused, and this guard is defense-in-depth rather than the live gate.
  4. Change `vcErr`'s wrap to return the raw `xds: `-prefixed error (drop `"tls: downstream: %w"`) → expect the boot-FAIL subtest's `tls: ` prefix assertion to fire. Confirms the fuzz invariant is preserved.

- [ ] **Step 6: Gates + commit**

```bash
gofmt -l internal/tls/ && go vet ./internal/tls/ && golangci-lint run ./internal/tls/... && go build ./... && go test ./internal/tls/ -count=1
git add internal/tls/config.go internal/tls/config_test.go
git commit -m "phase 65 T5: internal/tls — fetch + install the SDS-delivered validation_context as ClientCAs (require_client_certificate gated)"
```

---

## Task 6: `internal/boot/boot.go` — the pre-scan detects the validation arm

**Files:**
- Modify: `internal/boot/boot.go` (`NewSDSProvider`'s pre-scan, `:138`, inside the `:129-142` chain loop)
- Modify: `internal/boot/boot_test.go` (append; existing tests `:225`/`:236`/`:249`/`:296`)

**Interfaces:**
- Consumes: `dtc tlsv3.DownstreamTlsContext` (a VALUE, `:134`), `xds.ParseSDSConfig` (`:154`), `xds.RegisterSDSStats` (`:163`).
- Produces: a provider built for a validation-only-SDS listener (`seen==1`).

> **⚠️ WITHOUT THIS TASK, T5'S FETCH PATH IS UNREACHABLE IN A REAL BOOT.** The pre-scan today keys SOLELY on `GetTlsCertificateSdsSecretConfigs()` (`:138`). A listener using SDS ONLY for the validation_context pre-scans as `seen==0` → `NewSDSProvider` returns `(nil, nil)` → `NewDownstreamConfig` receives `nil` → T5's `provider == nil` reject fires. **T8's fixture depends on T6 AND T5 both landing.** A T8 run before T6 fails with `requires a live SDS provider` and will look like a fixture bug.

- [ ] **Step 1: Write the failing tests** in `internal/boot/boot_test.go` (append; RE-READ `:225-320` for the bootstrap-builder idiom and reuse it — do NOT invent a builder):

```go
// TestNewSDSProvider_ValidationOnlySDS_BuildsProvider: a listener using SDS ONLY
// for the validation_context (static/inline server cert) must build a provider.
// Before phase 65 the pre-scan keyed solely on tls_certificate_sds_secret_configs,
// so this shape yielded (nil, nil) and the tls apply-point's nil-provider reject
// fired — making the whole feature unreachable in a real boot.
func TestNewSDSProvider_ValidationOnlySDS_BuildsProvider(t *testing.T) {
	bs := bootstrapWithDownstreamTLS(t, &tlsv3.DownstreamTlsContext{
		RequireClientCertificate: &wrapperspb.BoolValue{Value: true},
		CommonTlsContext: &tlsv3.CommonTlsContext{
			TlsCertificates: []*tlsv3.TlsCertificate{inlineLeaf(t)},
			ValidationContextType: &tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig{
				ValidationContextSdsSecretConfig: sdsCfg(t, "validation_ca", "sds_cluster"),
			},
		},
	})
	p, err := boot.NewSDSProvider(dialerFor(t, bs), bs, "", stats.NewRegistry())
	if err != nil {
		t.Fatalf("NewSDSProvider: unexpected err %v", err)
	}
	if p == nil {
		t.Fatal("provider is nil — a validation-only-SDS listener must build a provider")
	}
}

// TestNewSDSProvider_BothViaSDS_Rejects: a context using SDS for BOTH the server
// cert AND the validation_context is the DEFERRED compose-two edge — the
// single-slot provider model (one secretName, one *SDSStats) cannot serve it, so
// the existing seen>1 guard (boot.go:147-148) must REJECT it rather than
// silently building a provider for whichever arm won the scan.
func TestNewSDSProvider_BothViaSDS_Rejects(t *testing.T) {
	bs := bootstrapWithDownstreamTLS(t, &tlsv3.DownstreamTlsContext{
		RequireClientCertificate: &wrapperspb.BoolValue{Value: true},
		CommonTlsContext: &tlsv3.CommonTlsContext{
			TlsCertificateSdsSecretConfigs: []*tlsv3.SdsSecretConfig{sdsCfg(t, "server_cert", "sds_cluster")},
			ValidationContextType: &tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig{
				ValidationContextSdsSecretConfig: sdsCfg(t, "validation_ca", "sds_cluster"),
			},
		},
	})
	_, err := boot.NewSDSProvider(dialerFor(t, bs), bs, "", stats.NewRegistry())
	if err == nil {
		t.Fatal("expected the seen>1 reject, got nil (compose-two is DEFERRED, SPEC-65 §2)")
	}
	if !strings.Contains(err.Error(), "multiple SDS-bound downstream TLS contexts unsupported") {
		t.Errorf("error = %q, want the seen>1 guard's substring", err.Error())
	}
}

// TestNewSDSProvider_CertOnlySDS_Unchanged: the phase-60.2 shape (0103) must be
// byte-unaffected by the pre-scan extension.
func TestNewSDSProvider_CertOnlySDS_Unchanged(t *testing.T) {
	bs := bootstrapWithDownstreamTLS(t, &tlsv3.DownstreamTlsContext{
		CommonTlsContext: &tlsv3.CommonTlsContext{
			TlsCertificateSdsSecretConfigs: []*tlsv3.SdsSecretConfig{sdsCfg(t, "server_cert", "sds_cluster")},
		},
	})
	p, err := boot.NewSDSProvider(dialerFor(t, bs), bs, "", stats.NewRegistry())
	if err != nil {
		t.Fatalf("NewSDSProvider: unexpected err %v", err)
	}
	if p == nil {
		t.Fatal("provider is nil — the cert-only SDS shape must still build a provider")
	}
}
```

> The helper names (`bootstrapWithDownstreamTLS`, `inlineLeaf`, `sdsCfg`, `dialerFor`) are ILLUSTRATIVE. **RE-READ `boot_test.go:225-320` and use whatever the existing tests actually use** — `TestNewSDSProvider_Success_FetchesDeliveredCertificate` (`:296`) already builds a full SDS bootstrap + dialer and is the closest template. Do not add a parallel builder.

- [ ] **Step 2: Run — expect FAIL:**

```
cd internal/boot && go test -run 'TestNewSDSProvider_ValidationOnlySDS|TestNewSDSProvider_BothViaSDS' -count=1 -v .
```
Expected: `_ValidationOnlySDS` FAILS (`provider is nil`); `_BothViaSDS` FAILS (`expected the seen>1 reject, got nil` — today only the cert arm counts, so `seen==1`).

- [ ] **Step 3: Extend the pre-scan** — REPLACE `boot.go:138-141`:

```go
			ctc := dtc.GetCommonTlsContext()
			if sc := ctc.GetTlsCertificateSdsSecretConfigs(); len(sc) > 0 {
				seen++
				found = sc
			}
			// Phase 65 (ADR-0286): also detect an SDS-bound validation_context, so a
			// listener using SDS ONLY for the downstream mTLS trusted-CA (with a
			// static server cert) builds a provider. Without this the pre-scan
			// returns (nil, nil) and the tls apply-point's nil-provider reject fires.
			//
			// seen++ on BOTH arms is deliberate: a context using SDS for the server
			// cert AND the validation_context trips the seen>1 guard below — the
			// DEFERRED compose-two edge (the single-slot provider model holds ONE
			// secretName and ONE *SDSStats; SPEC-65 §2/§3.4).
			if vsc, ok := ctc.GetValidationContextType().(*tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig); ok {
				seen++
				found = []*tlsv3.SdsSecretConfig{vsc.ValidationContextSdsSecretConfig}
			}
```
`ctc` is a NEW local (C5 — it does not exist today; `:138` reaches the CTC inline). `dtc` is a VALUE (`var dtc tlsv3.DownstreamTlsContext`, `:134`), so `dtc.GetCommonTlsContext()` is correct as written.

- [ ] **Step 4: Run — expect PASS** (all three, plus the four pre-existing `NewSDSProvider` tests `:225`/`:236`/`:249`/`:296`):

```
cd internal/boot && go test -count=1 ./...
```

- [ ] **Step 5: Liveness breaks** (`-count=1`; confirm WHICH fired; restore):
  1. Change the validation arm's `seen++` to a bare assignment (no increment) → expect ONLY `_BothViaSDS_Rejects` to fire. Proves the compose-two guard is live and not incidentally passing.
  2. Delete the whole validation arm → expect ONLY `_ValidationOnlySDS_BuildsProvider` (`provider is nil`). Proves the detection is what builds the provider.
  3. Confirm `_CertOnlySDS_Unchanged` stays green through BOTH breaks — if it ever fires, the extension regressed the `0103` path.

- [ ] **Step 6: Gates + commit**

```bash
gofmt -l internal/boot/ && go vet ./internal/boot/ && golangci-lint run ./internal/boot/... && go build ./... && go test ./internal/boot/ -count=1
git add internal/boot/boot.go internal/boot/boot_test.go
git commit -m "phase 65 T6: internal/boot — the SDS pre-scan detects validation_context_sds_secret_config (compose-two still rejects via seen>1)"
```

---

## Task 7: `test/helpers/sdsserver` — `WithValidationContext`

**Files:**
- Modify: `test/helpers/sdsserver/sdsserver.go` (fields `:29-31`; Options `:43-46`; `buildResponse` `:118-136`)
- Create/Modify: `test/helpers/sdsserver/sdsserver_test.go` (RE-CHECK whether this file exists; create if not)

**Interfaces:**
- Produces: `func WithValidationContext(name string, trustedCAPEM []byte) Option` — consumed by T8's driver.

**Design (D1 + D3):** `0108` serves exactly ONE secret per server, so the flat single-secret state STAYS flat — add two fields and branch in `buildResponse`. `Resources` stays a 1-element slice; the ignored `names` param stays ignored; the `first`-gating stays as-is. The generic type-URL derivation (`:133`) needs NO change — a `validation_context` is the same `tls.v3.Secret` message (D1). **Do NOT refactor to multi-secret** — that is the deferred compose-two edge and would risk the passing `0103`.

- [ ] **Step 1: Write the failing test** (`test/helpers/sdsserver/sdsserver_test.go`):

```go
// TestWithValidationContext_ServesValidationSecret: the server delivers a
// Secret{name, validation_context{trusted_ca: inline}} — the phase-65 resource
// shape PINNED by the reference's own config_dump (SPEC-65 §11 D-SDSVC-RESOURCE).
func TestWithValidationContext_ServesValidationSecret(t *testing.T) {
	caPEM := []byte("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n")
	srv, err := sdsserver.New(sdsserver.WithValidationContext("validation_ca", caPEM))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(srv.Stop)

	resp := fetchOne(t, srv.Addr(), "validation_ca") // drives one StreamSecrets exchange

	if got := len(resp.GetResources()); got != 1 {
		t.Fatalf("Resources = %d, want 1", got)
	}
	var sec tlsv3.Secret
	if err := resp.GetResources()[0].UnmarshalTo(&sec); err != nil {
		t.Fatalf("UnmarshalTo Secret: %v", err)
	}
	if sec.GetName() != "validation_ca" {
		t.Errorf("Secret.Name = %q, want validation_ca", sec.GetName())
	}
	vc := sec.GetValidationContext()
	if vc == nil {
		t.Fatal("Secret is not a validation_context — wrong oneof arm served")
	}
	if got := vc.GetTrustedCa().GetInlineBytes(); !bytes.Equal(got, caPEM) {
		t.Errorf("trusted_ca inline_bytes = %q, want the configured CA PEM", got)
	}
	// The type URL is SHARED with the tls_certificate arm — same Secret message.
	if got, want := resp.GetTypeUrl(), "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.Secret"; got != want {
		t.Errorf("TypeUrl = %q, want %q", got, want)
	}
}
```

> `New` / `NewAtAddr` / `Addr` / `Stop` / `Requests` — RE-DERIVE the exact constructor + accessor set from `sdsserver.go` before writing. `fetchOne` is a NEW local helper: dial `srv.Addr()` with `grpc.NewClient(..., insecure)`, open `StreamSecrets`, `Send` one `DiscoveryRequest{ResourceNames: [name], TypeUrl: <Secret URL>}`, `Recv` one response. The `internal/xds/provider_test.go:24-40` `grpcTestOpener` is the dial template — but it lives in another package, so INLINE the few lines rather than exporting it.

- [ ] **Step 2: Run — expect FAIL** (`undefined: sdsserver.WithValidationContext`):

```
cd test/helpers/sdsserver && go test -count=1 -v .
```

- [ ] **Step 3: Add the fields** (`sdsserver.go:29-31`):

```go
	secretName string
	certPEM    []byte
	keyPEM     []byte

	// vcSecretName / trustedCAPEM configure the validation_context arm (phase 65).
	// A Server serves EITHER a tls_certificate OR a validation_context — the
	// single-secret shape every current fixture needs. Serving BOTH on one server
	// is the deferred compose-two edge (rejected by boot's seen>1 guard), so the
	// flat state deliberately stays flat rather than becoming a map.
	vcSecretName string
	trustedCAPEM []byte

	silent bool // when true, never Send a response (drives the client's initial-fetch timeout)
```

- [ ] **Step 4: Add the Option** (after `WithSecret`, `:43-46`):

```go
// WithValidationContext configures the delivered
// Secret{name, validation_context{trusted_ca: inline PEM}} — the SDS-delivered
// downstream mTLS trusted-CA (phase 65). Mutually exclusive with WithSecret: the
// last one applied wins the buildResponse branch.
func WithValidationContext(name string, trustedCAPEM []byte) Option {
	return func(s *Server) { s.vcSecretName = name; s.trustedCAPEM = trustedCAPEM }
}
```

- [ ] **Step 5: Branch `buildResponse`** (`:118-136`) — REPLACE the `sec` construction, keeping everything from `anypb.New(sec)` down BYTE-IDENTICAL:

```go
func (s *Server) buildResponse(names []string) (*discoveryv3.DiscoveryResponse, error) {
	var sec *tlsv3.Secret
	switch {
	case s.vcSecretName != "":
		// Phase 65: the validation_context arm — the SAME tls.v3.Secret message,
		// a different oneof arm, so the generic TypeUrl derivation below is
		// unchanged.
		sec = &tlsv3.Secret{
			Name: s.vcSecretName,
			Type: &tlsv3.Secret_ValidationContext{ValidationContext: &tlsv3.CertificateValidationContext{
				TrustedCa: &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: s.trustedCAPEM}},
			}},
		}
	default:
		sec = &tlsv3.Secret{
			Name: s.secretName,
			Type: &tlsv3.Secret_TlsCertificate{TlsCertificate: &tlsv3.TlsCertificate{
				CertificateChain: &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: s.certPEM}},
				PrivateKey:       &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: s.keyPEM}},
			}},
		}
	}
	// ... anypb.New(sec) ... DiscoveryResponse{VersionInfo, Nonce, TypeUrl, Resources} UNCHANGED
}
```

- [ ] **Step 6: Run — expect PASS**, and confirm `internal/xds/provider_test.go` (which uses `WithSecret` + `Silent`) STAYS green — the cert arm must be untouched:

```
cd test/helpers/sdsserver && go test -count=1 -v .
go test ./internal/xds/ -count=1
```

- [ ] **Step 7: Liveness break** (`-count=1`): swap the `switch` arms (serve the cert secret when `vcSecretName != ""`) → expect `Secret is not a validation_context — wrong oneof arm served` to fire. Restore.

- [ ] **Step 8: Gates + commit**

```bash
gofmt -l test/helpers/sdsserver/ && go vet ./test/helpers/sdsserver/ && golangci-lint run ./test/helpers/sdsserver/... && go build ./... && go test ./test/helpers/sdsserver/ ./internal/xds/ -count=1
git add test/helpers/sdsserver/
git commit -m "phase 65 T7: sdsserver.WithValidationContext — serve a Secret{validation_context{trusted_ca}}"
```

---

## Task 8: fixture `0108-xds-sds-validation-context`

**Files:**
- Create: `test/fixtures/0108-xds-sds-validation-context/driver/driver.go`, `driver/doc.go`, `envoy.yaml`, `envoy-go.yaml`, `expectations.yaml`, `README.md`
- Modify: `test/differential/runner_test.go` (blank-import after the `0107` line **`:134`**)

**DEPENDS ON T5 **AND** T6.** Without T6 the boot pre-scan returns `(nil, nil)` and this fixture fails with `requires a live SDS provider` — which looks like a fixture bug but is a missing task. Confirm both landed before starting.

**Design — the four PLAN decisions that govern this task:**
- **D2 — in-memory PKI, NO `pki/` dir.** `ensure()` generates five artifacts (CA_served, server leaf, client_good, CA_unserved, client_bad) with `crypto/x509` (the `0018` `pki/gen.go` shape: ECDSA P-256; `ExtKeyUsage: ClientAuth` on the client leaves; `ServerAuth` + a DNS SAN on the server leaf). Nothing touches disk: the server cert is injected `inline_string:` into both yamls, the CA goes out over SDS `inline_bytes`, the client certs feed the driver's own `tls.Config`. No `HostMount`, no Docker mount, no `.gitignore` upkeep.
- **D3 — one secret per side.** Each side gets its OWN `sdsserver` (`sdsserver.NewAtAddr(..., WithValidationContext("validation_ca", caServedPEM))`), the `0103` `ensure()` two-receivers pattern (`driver/driver.go:63-87`, `reference_periodic_sink_differential_two_receivers`). Server cert STATIC in the yaml ⇒ boot sees `seen==1`.
- **D4 — the observable is a normalized two-arm verdict.** See below.
- **C3 — NO `StatsAsserter`.** envoy-go has no `ssl.*` stats. The accept/reject contrast IS the proof, and it is cross-side.

> **⚠️ NEVER assert the handshake-failure text.** Reference (BoringSSL) → `unknown ca`; envoy-go (Go `crypto/tls`) → `bad certificate`. Cross-side text equality is IMPOSSIBLE. Normalize (`0045`'s `closeOK` idiom, `driver/driver.go:313-323`).

- [ ] **Step 1: Write `driver/driver.go`.** Structure (RE-READ `0103/driver/driver.go` in full first and mirror its `init()`/registration/`ensure()`/port-allocation idioms exactly):

```go
const (
	fixtureName    = "0108-xds-sds-validation-context"
	secretName     = "validation_ca"       // the SDS-served validation secret
	serverName     = "l_sds_mtls.fixture.test"
	refListenerPort = 10444                 // RE-DERIVE a free port; 0103 uses 10443
	probePayload   = "phase65-mtls-probe\n"
)

func init() { fixture.RegisterFixture(fixtureName, &sdsVCDriver{}) }

type sdsVCDriver struct {
	once sync.Once

	caServedPEM   []byte // served over SDS as the trusted_ca
	serverCertPEM []byte // injected inline_string into both yamls
	serverKeyPEM  []byte
	clientGood    stdtls.Certificate // chains to caServed -> MUST be accepted
	clientBad     stdtls.Certificate // chains to an UN-SERVED CA -> MUST be rejected
	serverCAPool  *x509.CertPool     // the driver's RootCAs (verifies the proxy's leaf)

	refSDSPort, subjSDSPort int
	refSrv, subjSrv         *sdsserver.Server
}

// ensure generates the PKI in-memory and starts one SDS receiver per side.
// sync.Once-guarded and idempotent across ReferenceBootstrap/SubjectConfig call
// order (the 0103 pattern, driver/driver.go:63-87). Two receivers, one per side:
// a shared receiver would let one side's fetch contaminate the other's view
// (reference_periodic_sink_differential_two_receivers).
func (d *sdsVCDriver) ensure() { /* generate + start; panic on error, like 0103 */ }

func (d *sdsVCDriver) BackendCount() int { return 1 } // the good arm ECHOES through it
```

**`driveSide` — the observable (D4).** Both arms fold into ONE byte stream:

```go
// driveSide returns the NORMALIZED two-arm verdict. Both sides must emit
// byte-identical output.
//
// The good/bad CONTRAST is the whole proof: a client cert chaining to the
// SDS-SERVED CA is ACCEPTED; one chaining to an UN-SERVED CA is REJECTED. That
// is what makes this fixture non-vacuous — an accept-all trust store would emit
// `bad=ACCEPTED` and fail loudly.
//
// The failure TEXT is deliberately NOT part of the observable: the reference
// (BoringSSL) sends the alert `unknown ca` while envoy-go (Go crypto/tls) sends
// `bad certificate`. Asserting it would fail cross-side 100% of the time. We
// record only the normalized verdict (the 0045 closeOK idiom).
func (d *sdsVCDriver) driveSide(ctx context.Context, addr string) ([]byte, error) {
	var out bytes.Buffer

	// Arm 1 (positive): client_good chains to the SERVED CA -> handshake OK -> echo.
	echo, err := d.mtlsEcho(ctx, addr, d.clientGood, []byte(probePayload))
	switch {
	case err != nil:
		fmt.Fprintf(&out, "good=REJECTED err=%s\n", normalizeTLSErr(err))
	default:
		fmt.Fprintf(&out, "good=ok echo=%s", echo)
	}

	// Arm 2 (negative): client_bad chains to an UN-SERVED CA -> handshake MUST fail.
	if _, err := d.mtlsEcho(ctx, addr, d.clientBad, []byte(probePayload)); err != nil {
		fmt.Fprintf(&out, "bad=rejected\n")
	} else {
		// The trust store accepted a cert it must not have -> the served CA is NOT
		// the real anchor. Byte-differs from the expected form; fails loudly.
		fmt.Fprintf(&out, "bad=ACCEPTED\n")
	}

	return out.Bytes(), nil
}

func (d *sdsVCDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.driveSide(ctx, addr)
}
func (d *sdsVCDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.driveSide(ctx, addr)
}
```

`mtlsEcho` dials with `&stdtls.Config{Certificates: []stdtls.Certificate{clientCert}, RootCAs: d.serverCAPool, ServerName: serverName, MinVersion: stdtls.VersionTLS12}` (the `0018` scenario-6 shape, `inputs/driver.go:519-529`, but a RAW dial — `0018` uses an `http.Client`; this fixture's topology is `tcp_proxy`, so use `stdtls.Client(raw, cfg)` + `Handshake()` + write/read, the `0045` `tlsDial` shape at `driver/driver.go:293`). **Neither `helpers.TLSServedLeaf` (`test/helpers/tls.go:63-80`) nor `helpers.TLSRoundTrip` (`:20-57`) can be used — verified: neither has a `Certificates` field or a parameter to supply one.** Either inline the dial in the driver (preferred — keeps `helpers` untouched) or add a `helpers.TLSClientCertDial`; **inline it** unless a second fixture needs it.

`normalizeTLSErr` collapses any handshake error to a stable token — it exists ONLY for the `good=REJECTED` diagnostic path (a failure that should never happen); the `bad` arm never records text.

- [ ] **Step 2: Write `envoy.yaml` + `envoy-go.yaml`.** Clone `0103`'s pair and change the TLS block. Both need:
  - `node: {id: envoygo-node, cluster: envoygo-cluster}` — **REQUIRED** (`boot.go:151-152`); `0103` has it at `envoy{,-go}.yaml:29-31`.
  - The SDS cluster — clone `0103/envoy-go.yaml:81-95` verbatim (incl. `typed_extension_protocol_options` → `explicit_http_config.http2_protocol_options: {}` for gRPC h2c framing).
  - Reference/subject deltas (from `0103`): reference = `STRICT_DNS` + `host.docker.internal` + `connect_timeout: 1s` + `dns_lookup_family: V4_ONLY`; subject = `STATIC` + `127.0.0.1`.
  - The DTC (the phase-65 shape — a STATIC server cert + an SDS validation_context):
```yaml
              "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext
              require_client_certificate: true
              common_tls_context:
                tls_certificates:
                  - certificate_chain: { inline_string: "<server leaf PEM, injected>" }
                    private_key: { inline_string: "<server key PEM, injected>" }
                validation_context_sds_secret_config:
                  name: validation_ca
                  sds_config:
                    resource_api_version: V3
                    api_config_source:
                      api_type: GRPC
                      transport_api_version: V3
                      grpc_services:
                        - envoy_grpc: { cluster_name: sds_cluster }
```
  **`inline_string`, NOT `inline_bytes`** (D2 — `inline_bytes` is proto `bytes` and protojson demands base64). Indent the multi-line PEM correctly with a helper; a raw `fmt.Sprintf` WILL produce invalid YAML.

- [ ] **Step 3: Register the fixture** — `test/differential/runner_test.go`, after the `0107` line (`:134`):

```go
	_ "github.com/pgdad/envoy-go/test/fixtures/0108-xds-sds-validation-context/driver"
```

- [ ] **Step 4: Run the fixture** — the FULL selector, ALWAYS (`reference_differential_run_selector`; a bare `-run '0108'` matches ZERO subtests → vacuous green):

```
go test ./test/differential/ -count=1 -run 'TestDifferential/0108-xds-sds-validation-context' -v
```
Expected: PASS, both sides emitting byte-identical:
```
good=ok echo=phase65-mtls-probe
bad=rejected
```

- [ ] **Step 5: Liveness breaks** (`-count=1`, FULL selector, confirm WHICH fired, restore):
  1. **The trust-anchor proof.** Serve a DIFFERENT CA over SDS (`WithValidationContext("validation_ca", d.caUnservedPEM)`) → `client_good` no longer verifies → BOTH sides emit `good=REJECTED…` → the run still COMPARES EQUAL cross-side (both sides break identically!) but the driver must still fail. **⚠️ This is the trap:** a symmetric break does NOT fail a pure `CompareBytes` fixture — both sides change together (`reference_vacuous_break_receiver_normalizes`). **Therefore the driver MUST also assert the SHAPE per side**, not merely compare: add a `SubjectAsserter`/in-driver check that the subject's own bytes match `good=ok` + `bad=rejected`. **Decide at IMPL:** the cleanest form is an in-`driveSide` `Errorf`-free structural check returning an error when `good` is not `ok`, so a symmetric break fails BOTH sides loudly rather than passing as "equal". Record the chosen mechanism in PROGRESS.
  2. **The negative-arm proof.** Sign `client_bad` with the SERVED CA instead → both sides emit `bad=ACCEPTED` → same symmetry caveat as (1); the structural check must catch it.
  3. **The cross-side proof.** Break ONE side only (e.g. point the subject's `sds_config` at the reference's SDS port) → `CompareBytes` fires with a byte offset. This is the assertion that IS asymmetric-break-provable.
  
  > **⚠️ Read `reference_deliberate_break_wrong_assertion` before this step.** Breaks (1) and (2) are SYMMETRIC and will NOT fire `CompareBytes`. If the IMPL cannot make them fail, the fixture is proving only "both sides agree", not "the served CA is the anchor" — and the structural check is mandatory, not optional.

- [ ] **Step 6: Write `expectations.yaml` + `README.md`.** `expectations.yaml` is PROSE-ONLY documentation (ADR-0019; `0103`'s carries no `BackendCount` field) — the driver is the enforcer. The README documents: the topology, the in-memory PKI (D2) and WHY there is no `pki/` dir, the two arms, why the failure text is normalized (C3), and the `ssl.*` coverage boundary.

- [ ] **Step 7: Gates + commit**

```bash
gofmt -l test/ && go vet ./test/... && golangci-lint run ./test/fixtures/0108-xds-sds-validation-context/... && go build ./... && go test ./test/differential/ -count=1 -run 'TestDifferential/0108-xds-sds-validation-context'
git add test/fixtures/0108-xds-sds-validation-context/ test/differential/runner_test.go
git commit -m "phase 65 T8: fixture 0108 — an SDS-served CA as the downstream mTLS trust anchor (accept/reject contrast, cross-side)"
```

---

## Task 9: fuzz SEEDS (count STAYS 55)

**Files:**
- Modify: `internal/xds/fuzz_test.go` (seed for `FuzzDiscoveryResponseParse` `:71`; helper near `mustValidSecretAnyBytes` `:26-64`)
- Modify: `internal/tls/fuzz_test.go` (seed for `FuzzTLSContextParse` `:24`)

> **NO new `Fuzz*` func.** `reference_fuzzer_count_docs_drift` — reconcile `grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l` == **55** BEFORE and AFTER. It MUST NOT move.

- [ ] **Step 1: Reconcile the count BEFORE:**

```
grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l
```
Expected: `55`.

- [ ] **Step 2: `internal/xds/fuzz_test.go` — add the seed helper + seed.** `mustValidSecretAnyBytes` (`:26-64`) inlines `selfSignedPEM` because that helper takes `*testing.T` and cannot accept `*testing.F` (comment `:24-25`). **The validation helper hits the SAME constraint** — inline the cert generation again (a third duplication) OR refactor `selfSignedPEM` to `testing.TB`. **Prefer the `testing.TB` refactor** if it is a clean one-line signature change (`func selfSignedPEM(t testing.TB) (certPEM, keyPEM []byte)`) — `*testing.T` and `*testing.F` both satisfy `testing.TB`, and it retires the existing duplication rather than adding to it. If the refactor ripples, inline and note it.

```go
// mustValidValidationSecretAnyBytes builds the marshalled bytes of a
// Secret{validation_context{trusted_ca: inline CA PEM}} — the phase-65 seed.
func mustValidValidationSecretAnyBytes(f *testing.F) []byte {
	f.Helper()
	caPEM, _ := selfSignedPEM(f) // testing.TB — see the refactor note
	sec := &tlsv3.Secret{
		Name: "validation_ca",
		Type: &tlsv3.Secret_ValidationContext{ValidationContext: &tlsv3.CertificateValidationContext{
			TrustedCa: &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: caPEM}},
		}},
	}
	any, err := anypb.New(sec)
	if err != nil {
		f.Fatalf("anypb.New: %v", err)
	}
	return any.GetValue()
}
```
Add the seed alongside the existing three (`:73-75`):
```go
	f.Add(mustValidValidationSecretAnyBytes(f), "validation_ca")
```
And extend the fuzz body to ALSO drive the new applier (the body currently calls only `applyResponse`):
```go
		_, _ = applyValidationResponse(resp, wantName, "")
```

- [ ] **Step 3: `internal/tls/fuzz_test.go` — add the seed.** The `f.Add` signature is THREE-arg (`side, typeURL string, value []byte`). Build a `DownstreamTlsContext` with `require_client_certificate: true` + a validation SDS config, marshal it, and seed:

```go
	// Phase 65: a downstream require_client_certificate + SDS validation_context.
	// The invariant (every error `tls: `-prefixed) is UNCHANGED — the arm wraps
	// xds.ParseSDSConfig's `xds: `-prefixed errors (config.go).
	vcDTC := &tlsv3.DownstreamTlsContext{
		RequireClientCertificate: &wrapperspb.BoolValue{Value: true},
		CommonTlsContext: &tlsv3.CommonTlsContext{
			ValidationContextType: &tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig{
				ValidationContextSdsSecretConfig: &tlsv3.SdsSecretConfig{Name: "validation_ca"},
			},
		},
	}
	anyVC, err := anypb.New(vcDTC)
	if err != nil {
		f.Fatalf("anypb.New: %v", err)
	}
	f.Add("downstream", anyVC.GetTypeUrl(), anyVC.GetValue())
```
Note the fuzz body dispatches `NewDownstreamConfig(ts, "", nil)` — a **nil provider** — so this seed exercises the nil-provider reject path and MUST stay `tls: `-prefixed. That is exactly the invariant worth seeding.

- [ ] **Step 4: Run both fuzzers' seed corpora + reconcile the count AFTER:**

```
cd internal/xds && go test -run 'FuzzDiscoveryResponseParse' -count=1 .
cd ../tls && go test -run 'FuzzTLSContextParse' -count=1 .
grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l
```
Expected: PASS, PASS, and `55` (UNCHANGED — seeds only).

- [ ] **Step 5: Short fuzz smoke** (optional but recommended — 10s each catches an immediate panic):

```
cd internal/xds && go test -run 'XXX' -fuzz 'FuzzDiscoveryResponseParse' -fuzztime 10s .
cd ../tls && go test -run 'XXX' -fuzz 'FuzzTLSContextParse' -fuzztime 10s .
```
Delete any `testdata/fuzz/` corpus files this generates BEFORE committing (they are run artifacts, not deliverables — `feedback_subagent_autocommit_claudemd`: the controller cleans leak files).

- [ ] **Step 6: Gates + commit**

```bash
gofmt -l internal/ && go vet ./internal/... && golangci-lint run ./internal/xds/... ./internal/tls/... && go build ./... && go test ./internal/xds/ ./internal/tls/ -count=1
git add internal/xds/fuzz_test.go internal/tls/fuzz_test.go
git commit -m "phase 65 T9: fuzz seeds for the SDS validation_context arms (fuzzers stay 55)"
```

---

## Task 10: `BEHAVIOR_CONTRACT.md` — REJECT → CONSUMED

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (**`:881`** — RE-DERIVED this PLAN session: the downstream `validation_context_sds_secret_config` reject is **item 5** in the phase-60 reject list; item 6 at `:882` is the upstream sibling and STAYS)

- [ ] **Step 1: RE-DERIVE the clause.** `:881` currently reads (verbatim as of `be419023`):
> `5. a downstream validation_context_sds_secret_config — tls: downstream: SDS-bound validation_context_sds_secret_config is not supported in phase 03 (the pre-existing phase-03 reject arm; unchanged by phase 60 — o…`

Docs drift; **confirm the line number and text before editing** (the phase-64 PROGRESS records a drifted `:686` clause as precedent for exactly this).

- [ ] **Step 2: Rewrite item 5 as CONSUMED.** Content to convey (keep the surrounding list's voice + numbering):
  - A downstream `common_tls_context.validation_context_sds_secret_config` under `require_client_certificate: true` is **consumed**: the trusted-CA bundle is fetched over the SotW SDS stream (bounded by `initial_fetch_timeout`), loaded into an `*x509.CertPool`, and installed as the listener's `ClientCAs` with `ClientAuth = RequireAndVerifyClientCert` → mandatory mTLS (SPEC-65 §11 arms 1/2/3 pin the reference's identical behavior).
  - **Departure (ADR-0280, extended):** a fetch timeout / unreachable management server **boot-FAILS the listener**, where the reference serves anyway.
  - **Scope:** `require_client_certificate: false` (verify-if-present) is **inert** — the SDS validation_context is ignored and NO fetch is performed, mirroring the landed inline `validation_context`-without-`require` behavior. DEFERRED.
  - **The served `CertificateValidationContext` is held to the inline support surface:** `custom_validator_config` / `match_typed_subject_alt_names` / `verify_certificate_hash` / `verify_certificate_spki` each reject with an `xds: sds: validation secret %q: …` substring. `crl` is unchecked on BOTH paths (a documented shared gap).
  - **Siblings STAY:** upstream `validation_context_sds_secret_config` + QUIC (both nil-provider) keep the byte-identical phase-03 reject (item 6 `:882` unchanged); `combined_validation_context` STAYS; the compose-two shape (SDS for BOTH cert and validation on one context) rejects via `xds: sds: multiple SDS-bound downstream TLS contexts unsupported (MVP takes one)`.
  - **Coverage boundary (C3):** envoy-go exposes NO downstream TLS handshake-outcome stats (`ssl.handshake` / `ssl.fail_verify_error` / `ssl.fail_verify_no_cert`), which the reference does emit. Verification outcomes are observable only via connection success/failure. Pre-existing; not opened or closed by phase 65.

- [ ] **Step 3: Verify no OTHER clause contradicts.** Re-grep and reconcile:

```
grep -n 'validation_context_sds_secret_config' docs/envoy-go/BEHAVIOR_CONTRACT.md
```
`:888` ("Phase 60 lifts exactly ONE arm — the downstream `tls_certificate_sds_secret_configs` reject") stays TRUE as a phase-60 statement but now reads as stale absolutism. Add a phase-65 clause rather than rewriting phase-60 history.

- [ ] **Step 4: Commit** (docs-only; no gates):

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md
git commit -m "phase 65 T10: BEHAVIOR_CONTRACT — downstream SDS validation_context REJECT -> CONSUMED"
```

---

## Task 11: verify (six-gate + 110-dir) + ADR-0286 + STATE + ROADMAP + PROGRESS + router roll

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0286 §Decision/§Consequences — §Context is SPEC §13)
- Modify: `docs/envoy-go/STATE.md` (active-phase header)
- Modify: `docs/envoy-go/ROADMAP.md` (row 65 `done`; the sentinel narrow at **`:185`**)
- Modify: `docs/envoy-go/phases/65-xds-sds-validation-context/PROGRESS.md` (close)
- Modify: `next-prompt.txt` (router roll — **TRACKED**, `reference_next_prompt_tracked_despite_gitignore`)

- [ ] **Step 1: The six-gate** — controller-run on the FROZEN HEAD, no-commit:

```bash
gofmt -l internal/ test/ cmd/          # expect: no output
go vet ./...                           # expect: exit 0
go build ./...                         # expect: clean
go mod tidy -diff                      # expect: EMPTY
git diff --exit-code go.mod go.sum     # expect: empty (modules STAY 2)
golangci-lint run ./...                # expect: exit 0
go test -race -count=1 ./internal/xds/... ./internal/tls/... ./internal/boot/... ./test/helpers/sdsserver/...
```
> **Run the FULL packages under `-race`, not just `-run` selections** (`reference_full_suite_race_after_background_mutator`). The `sdsserver` extension + the `0108` driver both start goroutine-backed servers.

- [ ] **Step 2: The cycle guard** (`reference_xds_config_seam_transitive_cycle_guard`) — **NO `...`**:

```bash
go list -deps ./internal/xds | grep 'envoy-go/internal' 
```
Expected EXACTLY:
```
github.com/pgdad/envoy-go/internal/stats
github.com/pgdad/envoy-go/internal/xds
```
**`internal/tls` MUST NOT appear.** If it does, T1's CA-pool build reached for `loadTrustedCAPool` — revert to the duplicated `x509.NewCertPool()`.

- [ ] **Step 3: The full 110-dir differential:**

```bash
go test ./test/differential/ -count=1
```
Expected: `ok`, EXIT 0 — the 109 pre-existing dirs byte-stable (phase 65 lifts a reject; it cannot change any passing fixture's bytes) + the new `0108`. On an unrelated `subject ready: EOF`, isolate-re-run before treating it as a regression (`reference_differential_fullsuite_startup_flake`).

- [ ] **Step 4: Re-verify the counts** on the landed tree:

```bash
ls -d test/fixtures/[0-9]*/ | wc -l                                   # expect 110
grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l              # expect 55
grep -oE '^## ADR-[0-9]{4}' docs/envoy-go/DECISIONS.md | tail -1      # expect ADR-0286
```
Plus: stat surface **1201** (+0 — reuses the dynamic `sds.*` scope; no new counter TYPE), BackendKind **38** (+0), packages **+0**, modules **2**.

- [ ] **Step 5: ADR-0286 §Decision/§Consequences** (ADR-0044; §Context is SPEC §13 — do NOT re-draft it). §Decision must record, at minimum: Option A (the parallel `*x509.CertPool` chain, landed chain untouched); the `NewDownstreamConfig` apply-point gated on `require==true`; the `commonTLSContextToConfig` no-op scoped `downstream && provider != nil` (QUIC reaches it nil-provider); the boot pre-scan extension + `seen>1` as the compose-two deferral; the CVC reject roster; the ADR-0280 boot-FAIL departure extended. §Consequences must record the THREE PLAN-time corrections that changed the shipped design:
  1. **The `ssl.*` stat gap (C3)** — the SPEC's negative-arm `StatsAsserter` was infeasible; the fixture proves the trust anchor by a cross-side accept/reject CONTRAST instead. Record the missing `ssl.*` family as a named coverage boundary.
  2. **The arm-5 vacuity trap (C2)** — the flipped test needed a full `sds_config` or it would have proven nothing.
  3. **The in-memory PKI (D2)** — neither clone-parent's model was adopted.

- [ ] **Step 6: ROADMAP** — row 65 `in-progress` → `done` (ADR-0106, the SOLE leg). **The sentinel narrow at `:185`** (`reference_sentinel_deferred_sentence_live_vs_historical`): the live xDS sentence currently reads `… SDS rotation + SDS validation_context/upstream SDS + CDS/EDS + …`. Narrow `SDS validation_context/upstream SDS` → `upstream SDS (server-cert + validation_context)` — the downstream `validation_context` is now CONSUMED. **The xDS family STAYS OPEN.** Then re-run check (2) and CONFIRM exactly ONE live xDS match remains:

```bash
grep -oE 'remaining deferred \(not-yet-chartered\) candidates:[^.]*\.' docs/envoy-go/ROADMAP.md
```
Expected: still THREE sentences (HTTP/3 `:175`, xDS `:185` — narrowed, Observability `:193`). **Line `:193` also carries a HISTORICAL `candidates were:` recap — do not confuse them.** Also ADD the `ssl.*` handshake-outcome stat family (C3) as a NEW deferred candidate on the Observability sentence.

- [ ] **Step 7: STATE** — flip the active-phase header to `phase 65 (xds-sds-validation-context) IMPL done`; NEXT = the phase-66 BRAINSTORM (the roller SELF-PICKS the subject per the standing directive).

- [ ] **Step 8: Re-run the termination sentinel MECHANICALLY** (all three checks, from the router). Anticipated: check (1) prints NOTHING for row 65 (now `done`) but the sentinel still does NOT fire — checks (2) and (3) still print (three live deferred sentences; gRPC/Runtime/WASM never opened). **Do NOT create `stop`.**

- [ ] **Step 9: PROGRESS close + router roll** — fill every `[fill at IMPL]` block in `PROGRESS.md` with VERBATIM evidence; roll `next-prompt.txt` to the phase-66 BRAINSTORM. Commit:

```bash
git add docs/ next-prompt.txt
git commit -m "phase 65 (xds-sds-validation-context) IMPL: honor the SDS-delivered downstream validation_context — fetch the CA over SotW SDS and install it as ClientCAs (ADR-0286, row 65 done)"
```

- [ ] **Step 10: Controller stage-close** — squash to a single master commit (locate by SUBJECT, `git log --grep`, NEVER by position), re-run the suite on the frozen HEAD, verify the MAIN checkout is clean, then **push** (`feedback_push_to_origin`; subagents never push — `feedback_subagents_no_push`).

---

## Self-review against SPEC-65

**Spec coverage.** SPEC §10's 11-task spine maps 1:1 onto T1–T11. §3.1 (discovery machine unchanged) → T2's duplication rationale. §3.2 (Option A) → T1/T2/T3. §3.3a → T4; §3.3b → T5. §3.4 → T6. §3.5 (`require==false` inert) → T5 Step 1's third subtest. §3.6 (cycle guard) → T1's doc comment + T11 Step 2. §6 (CVC roster + fuzz seeds) → T1 + T9. §7 (+0 stats) → held; C3 does not add one. §8 (fixture) → T8, with the C3/C4/D2/D4 corrections. §9 (BEHAVIOR_CONTRACT) → T10, site RE-DERIVED to `:881`. §13 (ADR-0286 §Context) → T11 Step 5 adds §Decision/§Consequences only. §10's sentinel-narrow → T11 Step 6, site RE-DERIVED to `:185`.

**Deviations from SPEC (each deliberate, each argued above):** C3 (the negative arm drops the infeasible `StatsAsserter`) · C2 (the arm-5 input is REBUILT, not merely flipped) · D2 (in-memory PKI, no `pki/` dir) · D4 (a normalized verdict observable, not `0103`'s `driveSide`) · C1 (14 reject arms, not 9 — reused, not rebuilt).

**Type consistency.** `parseValidationSecret(resource *anypb.Any, wantName, baseDir string) (*x509.CertPool, error)` → consumed by `applyValidationResponse(resp *discoveryv3.DiscoveryResponse, secretName, baseDir string) (*x509.CertPool, error)` → consumed by `fetchValidationSecret(stream Stream, node Node, secretName, baseDir string) (*x509.CertPool, error)` → consumed by `FetchInitialValidationContext(ctx context.Context, secretName string) (*x509.CertPool, error)` → consumed by T5's `provider.FetchInitialValidationContext(context.Background(), secretName)` → `cfg.ClientCAs`. `*x509.CertPool` end-to-end; names identical at every hop; all five RE-GREPPED collision-free.

**Known residual risk (flagged, not resolved).** T8 Step 5 breaks (1) and (2) are **symmetric** — they change both sides identically and will NOT fire `CompareBytes`. The fixture therefore needs an in-driver STRUCTURAL check (subject-side shape), or it proves only cross-side agreement rather than "the served CA is the anchor". The IMPL must resolve this and record the mechanism in PROGRESS. This is the single most likely place for phase 65 to ship a vacuous test.
