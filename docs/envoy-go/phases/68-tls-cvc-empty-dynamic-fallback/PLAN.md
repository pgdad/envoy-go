# PLAN 68 — TLS `combined_validation_context` empty-dynamic fallback — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL — `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Stage:** PLAN (lifecycle-state 2 → 3). Docs-only — ZERO production `.go`. Worktree `.worktrees/phase-68-plan`, branch `phase-68-tls-cvc-empty-dynamic-fallback-plan`, tip **`fba6d385`** (the phase-68 SPEC squash — master), per `feedback_git_worktrees`.
>
> **Row 68 STAYS `in-progress`** — the IMPL flips it `done` at its six-gate (ADR-0106, the SOLE leg). **ADR-0290's §Context is ALREADY DRAFTED** at the SPEC squash (`fba6d385`, DECISIONS.md:17052, STATUS: **IN PROGRESS**); the IMPL **COMPLETES ADR-0290 IN PLACE** with §Decision/§Consequences — it does NOT append a new ADR, does NOT renumber. DECISIONS tail stays **ADR-0290**, next-free **ADR-0291** (`[RUN]`: `grep -c '^## ADR-0291' docs/envoy-go/DECISIONS.md` → 0). **This PLAN adds NO ADR content.**
>
> **Baselines RE-DERIVED at `fba6d385` (`[RUN]`, NOT copied):** fixtures **112** (`ls -d test/fixtures/[0-9]*/ | wc -l`; numeric tail `0110-tls-require-client-cert-false`) · fuzzers **55** (`grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l`) · BackendKind tail **38** (`H2GoawayResponder`) · stat surface **1201** · DECISIONS tail **ADR-0290** · go.mod modules **2** (lineage figure; the single `go.mod` requires 67 — re-check `git diff go.mod` after tidy).
>
> **Sentinel expectation:** check (1) prints `NOT DONE: row 68`; check (2) prints **3** via the full-phrase form ONLY — `grep -cE 'remaining deferred \(not-yet-chartered\) candidates:' docs/envoy-go/ROADMAP.md` (`reference_sentinel_deferred_sentence_live_vs_historical` — cite the command, never the adjective); check (3) prints `NEVER OPENED: gRPC/Runtime/WASM`. **No deferred-sentence edit at ANY stage of this row** (SPEC §12).
>
> **⚠️ NO PARALLEL STREAM.** Master (`fba6d385`) IS the SPEC squash — zero commits landed between the SPEC and this tip (`git log --oneline fba6d385...` past the SPEC is the BRAINSTORM). So — unlike phase 67 — there are no post-SPEC absorptions; §1 is line-boundary corrections from re-derivation plus the structural decisions the SPEC delegated, and THREE material re-derivation findings (RD3/RD-P5/RD-DUAL).
>
> **⚠️ RE-DERIVE, do not execute.** A PLAN is not evidence (phase-66's PLAN carried nine draft defects). Where this document cites, go look; where it claims control flow, walk the call graph; default to REFUTED (`feedback_brief_citations_not_evidence`, `reference_quoting_is_not_executing`).

---

## 1. Re-derivation + correction ledger — every place this PLAN sharpens SPEC-68

**All SPEC §3/§9/§11 code anchors RE-DERIVED at `fba6d385` by reading `internal/tls/config.go`, `internal/xds/{stream,secret,provider}.go`, `test/helpers/sdsserver/sdsserver.go`, `internal/tls/fuzz_test.go`, `internal/tls/config_test.go`, and the `0109`/`0110` drivers in full.** Every SYMBOL and CONTROL-FLOW CLAIM in the SPEC re-derived TRUE; a handful of line boundaries drifted and THREE findings sharpen SPEC prose. The findings below (RD*) are the load-bearing corrections; adversarial verification (§1.2) must confirm or refute each.

| # | SPEC-68 said | This PLAN says (RE-DERIVED at `fba6d385`) | Where |
|---|---|---|---|
| **RD-LINES** | four sub-field rejects `:442-461`; DEPARTURE comment `:185-194`; fetch-error branch inside `:136-198` | **Line boundaries sharpened (symbols unchanged):** the four sub-field rejects are **`:448-461`** (`:442-447` is the `inlineVC` selector setup that re-points them at `default_validation_context` — P4); the CVC fetch-error branch is `if err != nil {` at **`:184`**, boot-FAIL `return` at **`:194`**; the DEPARTURE comment spans **`:185-193`** (9 comment lines; the boot-FAIL `return` is `:194`); the whole `pool, err := …` fetch-and-error block that T2 replaces is **`:183-198`** (M1); the empty-both/inline no-anchor model arm is **`:199-214`** with the retained reject at **`:203`** (SPEC §3.6 cites `:203` — CORRECT); `installPool`/`clientAuthFor` `:78-96`, `loadTrustedCAPool` `:298`, E1 `:427-429`, P5 block `:158-164` — all CORRECT. | All tasks |
| **RD-DUAL** | §3.2(3): `applyValidationResponse`'s wrap at `stream.go:164` reads `%w: %v` and FLATTENS; the IMPL must preserve BOTH sentinels via `%w: %w` or `errors.Join` | **CONFIRMED by re-read AND by tracing the whole chain.** `stream.go:164` = `fmt.Errorf("%w: %v", errValidation, err)` — the `%v` flattens `err`, so a sentinel from `parseValidationSecret` is LOST to `errors.Is`. **AND** the chain SURVIVES to `config.go` ONLY because `provider.go:112` (`errors.Is(err, errValidation)`) `return nil, err`s the wrapped error UNCHANGED — so once `stream.go:164` is dual-`%w`, `ErrEmptyValidationContext` reaches `config.go` with NO provider.go edit (RD-PROV). The empty-resources branch `stream.go:160` (`%w: empty resources`) stays `errValidation`-only — NOT dual-wrapped (scope pin, SPEC §3.2 adversarial M4/A3). | T1 |
| **RD-PROV** | §3.8: `provider.go` UNCHANGED | **CONFIRMED by execution-trace, stated LOUDLY.** On S1, `fetchValidationSecret` returns the dual-wrapped error; `FetchInitialValidationContext` (`provider.go:110-114`) matches `errors.Is(err, errValidation)` (still true — `errValidation` is in the dual chain), increments `update_rejected` (D-EDF-STREAMPOSTURE option (b) — the NACK stays), and `return nil, err` returns the FULL chain. So `config.go` sees `errors.Is(err, ErrEmptyValidationContext)` == true. **`provider.go` takes NO edit** — verify byte-untouched in the envelope audit (T8). | T1, T8 |
| **RD-P5** | §3.1/§11: "the MANDATORY P5 comment block (`config.go:158-164`) moves/updates with the CVC arm INTACT"; `[MOVE INTACT]` | **The CVC arm does NOT move in phase 68 (there is NO hoist — unlike phase 67).** So "[MOVE INTACT]" re-derives to **"byte-PRESERVE in place"** — the P5 block at `:158-164` is byte-untouched (ADR-0287 §Decision calls it MANDATORY; `reference_code_comment_not_evidence` — do NOT mutate it). The §3.1-P5 "hazard REDUCED on the fallback" note lands as a SHORT clause in the NEW fallback comment at the `:184` err-branch, NOT by editing the P5 block. Verify with `git diff` that `:158-164` shows ZERO change. | T2 |
| **RD3** | §8: the `0111` deliberate breaks include "a forced-send→polite regression (must go vacuous-green and the break harness must CATCH it)" | **RE-DERIVED FINDING (EMPIRICALLY CONFIRMED — controller + V1 + V2 ran fresh-PKI TLS harnesses). TWO parts:** **(correct half)** at `require_client_certificate: true` a BARE forced-send→polite regression does NOT change the correct-impl observable — a polite Go client withholds `client_B` (issuer CA_B not advertised in the CertificateRequest), the server rejects for **no-cert** (alert 116), and the driver normalizes the alert away and records `untrusted=rejected` EITHER way (forced → alert 48 verify-fail; polite → alert 116 no-cert). So a bare polite swap is an **EXPECTED non-fire** at require=true (unlike 0110/require=FALSE, where polite → no-cert ACCEPTED → observable flips). The SPEC §8 "must go vacuous-green" is 0110 physics that does NOT transfer. **(corrected half — the draft's own second claim was WRONG, refuted by execution):** the draft claimed forced-send is load-bearing as an "UPPER BOUND on the pool" demonstrated by a permissive-pool + polite two-factor break. **FALSE:** at `RequireAndVerifyClientCert` the verification pool IS the advertised set, so any fallback pool permissive enough to ACCEPT `client_B` (`CA_A ∪ CA_B`, accept-all) NECESSARILY advertises CA_B — and then the polite client SENDS `client_B` too (confirmed: `{CA_A}` pool → polite withholds → rejected; `{CA_A,CA_B}` pool → polite SENDS → accepted). So the "vacuous green" the two-factor break predicts does NOT occur — Break G step 2 would FIRE, not hide the bug. **The honest position:** forced-send is NOT observably load-bearing for the untrusted arm at require=true — the union-vs-replace hazard is caught in BOTH send-modes. It is RETAINED per `reference_go_client_cert_withholding` for two REAL reasons: (a) without it the untrusted arm collapses into a no-cert duplicate of the `none` arm and stops exercising the verify-and-reject path (a MEANING loss, invisible to `structuralCheck`); (b) it keeps both sides symmetric and robust to reference/Go CA-hint differences. Break G is REPLACED by the union-pool upper-bound break (fires in BOTH modes) + the recorded bare-polite non-fire (§ Task 6). | T6, §Break protocol |
| **RD-SENT** | §3.2(2): wrap the sentinel when `vc.GetTrustedCa() == nil OR vc.GetTrustedCa().GetSpecifier() == nil` (the S1 shape) | **CONFIRMED precise by walking `dataSourceBytes` (`secret.go:28-49`).** Both disjuncts route to `dataSourceBytes`'s `default:` "none of inline_bytes… set" branch (`:46-47`): `trusted_ca` unset → `GetTrustedCa()==nil`; `trusted_ca:{}` (empty DataSource, no specifier) → `GetSpecifier()==nil`. S2 (`trusted_ca:{inline_bytes:""}`) has a SET `*DataSource_InlineBytes` specifier → does NOT match the disjunction → reaches `dataSourceBytes` → returns empty bytes → `AppendCertsFromPEM(nil)`==false → `:135` "parse failure" WITHOUT the sentinel → boot-FAIL. The narrow gate is exactly right. | T1 |
| **RD-IMP** | §3.8: `internal/tls/config.go` is functionally edited | **`config.go` gains a NEW `"errors"` import** (`[RUN]`: `grep -n '"errors"' internal/tls/config.go` → absent) for `errors.Is` in the fallback branch. A functional edit (already in-envelope, C2); named so the diff is expected. | T2 |
| **RD-FUZZ** | §7: add a seed + a new dispatch side/provider mode returning the classified-empty error (`&fakeProvider{vcErr: …}`); +0 fuzzers | **CONFIRMED buildable + the trap re-derived.** `fakeProvider` (config_test.go:802-808) has `vcErr error` + `pool *x509.CertPool`; `FetchInitialValidationContext` returns `(f.pool, f.vcErr)`. A new pkg-level `cvcEmptyFuzzProvider := &fakeProvider{vcErr: fmt.Errorf("…: %w", xds.ErrEmptyValidationContext)}` + a new `"downstream-sds-empty"` switch arm in the fuzz body → the fallback fires (`cvcCTC()`'s default carries `trusted_ca`) → `err == nil`. **`fuzz_test.go` gains an `xds` import** (`[RUN]`: absent today) to reference the exported sentinel. A new dispatch side is +0 `func Fuzz` (the phase-66/67 pattern). | T4 |
| **RD-DUPSITE** | §3.2(3): dual-`%w` "at `stream.go:164`"; the empty-resources wrap "at `stream.go:160`" | **MODERATE (V1 execution): the two bare strings each match TWO sites — target the `applyValidationResponse` occurrence, NOT `applyResponse`.** `fmt.Errorf("%w: %v", errValidation, err)` appears at BOTH `stream.go:98` (`applyResponse`, the leaf-cert path) AND `:164` (`applyValidationResponse`); `fmt.Errorf("%w: empty resources", errValidation)` at BOTH `:94` (`applyResponse`) AND `:160` (`applyValidationResponse`). The dual-`%w` change (T1 Step 4) and Break B (T1 Step 6) MUST edit ONLY `applyValidationResponse` (`:164`/`:160`) — key on the function, use a line-anchored or context-unique Edit, never a `replace_all`. A stray edit to `applyResponse` is behaviorally harmless (the leaf-cert path never carries `ErrEmptyValidationContext`; `FetchInitialCertificate` ≠ the CVC fallback path) but is an unintended envelope change — flag it. | T1 |

### 1.1 Structural decisions the SPEC delegated to the PLAN (each RE-DERIVED, not invented)

- **Fallback control flow (T2).** The fallback lives INSIDE the CVC arm's `if err != nil {` block (`config.go:184`) and RETURNS on every branch (so control never falls through to the post-error `installPool(pool)` at `:196`, where `pool` is nil). Exact skeleton in T2.
- **Empty-both routing (T2).** `dvc.GetTrustedCa() == nil` mirrors the inline no-anchor arm (`:199-206`) BYTE-FOR-BYTE on the reject string (`:203`) and the `NoClientCert` return — no new reject (SPEC §3.6, D-EDF-EMPTYBOTH).
- **Sentinel siting + text (T1).** `ErrEmptyValidationContext` defined in `stream.go` beside `errValidation` (`:33`; `errors` already imported — no new import, A1). Wrapped in `parseValidationSecret` (`secret.go`) AFTER the four sub-field checks (`:117-128`) and BEFORE `dataSourceBytes` (`:129`). Exact edits in T1.
- **sdsserver flag (T3).** A new `emptyVC bool` Server field (config_test-free; 0 hits) toggles the `trusted_ca`-unset shape in `buildResponse`; `WithEmptyValidationContext(name)` sets `vcSecretName` + `emptyVC`. Exact edits in T3.
- **Task granularity.** A SINGLE FLAT ROW, 9 tasks; ADR-0045 escape valve ARMABLE, UNCONSUMED (no two-package surface can strand a leg — `internal/boot`/`validate/`/`internal/listener` untouched). Sequencing: T1 (`internal/xds`) → T2 (`internal/tls`, consumes the sentinel) → T3 (`sdsserver`) → T4 (fuzz) → T5→T6 (fixture) → T7 (BC) → T8 (verify) → T9 (close).

### 1.2 Adversarial-pass record

**THREE independent verifiers ran against the draft, each in PRIVATE scratch OUTSIDE the repo** (`reference_parallel_subagents_private_scratch`; the real repo left untouched, no worktrees registered):

- **V1 — code-claims BY EXECUTION** (`git clone --local` into scratch; applied T1/T2/T3 verbatim; `go build`/`go vet` exit 0): **the production design is SOUND and classifies/falls-back exactly as claimed.** CONFIRMED by execution — the CORE classification (S1/S1b → both sentinels; S2/S3/empty-resources → `errValidation`-only, the narrow `GetTrustedCa()==nil || GetSpecifier()==nil` gate precise), the fallback end-to-end (require true/false/absent, empty-both both ways, non-sentinel → boot-FAIL), `WithEmptyValidationContext` S1 vs `WithValidationContext(name,nil)` S2, RD-PROV (dual chain reaches `config.go` with `provider.go` byte-untouched), breaks A–F compile and fire the right assertion, every RD-LINES anchor accurate, the cycle guard. Found **1 SEVERE** (RD3/Break G — below), **1 MODERATE** (RD-DUPSITE — the dual `stream.go` string sites), **2 MINOR** (Break C not an exact one-line deletion; RD-FUZZ not executed — plausible/buildable).
- **V2 — spine order-soundness + RD3** (read-only code-path walk): **task ordering, the fallback control flow (every path returns; nil-pool `installPool` unreachable; the `:203` reject byte-identical; `dvc` E1-guaranteed), the dual-`%w` chain, the empty-both cross-product, and the fuzz dispatch trap all SOUND.** Found the SAME **1 SEVERE** (RD3/Break G) independently, plus MINORs M1 (T2 range :183-198), M2 (RD-LINES count), M3 (test 3 valid default).
- **V3 — process/consistency**: **SOUND — docket-complete, counts/sentinel/collisions re-run and confirmed, no-ADR-content honored, format follows the phase-67 precedent.** Zero SEVERE/MODERATE; one wording note ("two production files" = two packages).

**THE SEVERE (V1 + V2 agreed; controller reproduced it with a fresh-PKI TLS harness):** the draft's RD3 *second half* + Break G *two-factor demonstration* were UNSOUND — at `RequireAndVerifyClientCert` a permissive `CA_A∪CA_B` fallback pool ADVERTISES CA_B, so the polite client SENDS `client_B` too and the break FIRES instead of going vacuous-green. **CORRECTED in this amendment:** RD3's first half (bare polite swap does NOT flip the require=true observable) stands; the second half is replaced with the honest position (forced-send NOT observably load-bearing at require=true; retained for meaning + symmetry); Break G is replaced by the union-pool UPPER-BOUND break (fires in both modes) + the recorded bare-polite non-fire. The MODERATE (RD-DUPSITE) and all MINORs are folded in above and at their sites. **The production design, the classifier, and the shipping fixture were unaffected — only the break justification was wrong.**

---

## Global Constraints

- **ONE stage per session.** This session: the PLAN only. No production `.go`. After it lands: roll to the phase-68 IMPL.
- **TWO functionally-edited production PACKAGES (three files)** (SPEC §3.8, C2): **`internal/tls/config.go`** (the fallback branch + the `errors` import + the `:185-193` cause-scoped comment rewrite) **AND `internal/xds/{secret.go,stream.go}`** (define + wrap `ErrEmptyValidationContext`; the dual-`%w`). *(The SPEC's "two production files" counts the two packages; `internal/xds` is two files.)* **ONE new `internal/xds` EXPORTED symbol** (`ErrEmptyValidationContext` — the FIRST break of the zero-new-`internal/xds`-symbols streak, DECISIONS.md:16973/:16950; **stated LOUDLY**, an explicit envelope change). Plus **ONE chartered test-helper Option** (`sdsserver.WithEmptyValidationContext`). **BYTE-UNTOUCHED:** `internal/xds/provider.go` (RD-PROV), `internal/boot` (incl. boot.go — the pre-scan counts the CVC SDS half require-agnostically, `boot.go:174-176`, so nothing changes), `internal/listener` (incl. quic.go), `validate/`.
- **CVC-ONLY.** The plain `validation_context_sds_secret_config` arm has NO inline default and stays boot-FAIL (a RETAINED ADR-0280-family departure, SPEC §3.6). The fallback is reachable ONLY through the `CombinedValidationContext` case.
- **NARROW classifier.** The fallback fires ONLY on S1 (specifier-unset). S2 (`inline_bytes:""`), S3 (corrupt PEM), timeout/unreachable, and empty-resources all stay boot-FAIL (SPEC §3.2). `errors.Is(err, ErrEmptyValidationContext)` is the sole gate.
- **Counts at the IMPL:** fixtures **112 → 113** (`0111-tls-cvc-empty-dynamic-fallback`) · fuzzers **55 (+0, a new dispatch side + seed only)** · stat surface **1201 (+0)** · BackendKind **38 (+0)** · go.mod **+0** (SPEC metric "2" carried; re-check `git diff go.mod` after tidy — `reference_new_subpackage_pulls_transitive_module`) · ZERO new packages · DECISIONS tail stays **ADR-0290** (completed IN PLACE; next-free ADR-0291).
- **The pinned §9 wording lands MECHANICALLY** — B1–B4 are named obligations with verbatim replacement text; never silent rewrites, never paraphrases. They land at T7, atomically with ADR-0290 completion at T9's stage-close.
- **Per-task hygiene** (`feedback_pertask_gofmt_lint`): `gofmt -l` + `go vet` + `golangci-lint run` on every touched package.
- **Worktree discipline** (`feedback_git_worktrees` / `feedback_subagent_worktree_detach` / `feedback_subagent_worktree_path_targeting`): pin the canonical root; controller verifies the MAIN checkout stays clean; deliberate breaks restore with **`git restore` only**; breaks run AFTER committing (`reference_break_protocol_commit_first`).
- **Subagents commit locally; the controller squash-pushes at stage-close** (`feedback_subagents_no_push`, `feedback_push_to_origin`). Subagents auto-commit per CLAUDE.md; the controller squashes at close. Locate commits by SUBJECT (`git log --grep 'phase 68'`), never by position.
- **`reference_sds_init_fetch_timeout_dial_budget_flake`** — a `TestProvider_*_Timeout` failure under `-race` is PRE-EXISTING on master (one occurrence, 2026-07-16). Do not reflex-classify as a phase-68 regression; a SECOND occurrence justifies widening the budget. Same for a `0061` ring-hash spread flake (`reference_0061_ring_hash_spread_flake`).

### Break protocol (binding on every task)

- **A break must COMPILE** (`reference_plan_break_instructions_dont_compile`). The code-level breaks (A–E) below are drafted as exact compiling edits; the adversarial pass (§1.2, V1) **pre-verifies each COMPILES in a throwaway clone → `go vet` exit 0 → reverts (never committed)** and records the result in §1.2. Any break the pass could NOT pre-verify is flagged `[NOT pre-compiled — substitution rule applies]`: at IMPL time, if it does not compile, **substitute a compiling equivalent, REPORT the substitution, record the TRUE result**.
- **A break must DISCRIMINATE** (`reference_probe_must_discriminate`): before recording it as proof, ask what the OTHER hypothesis would have printed.
- **`-count=1` on EVERY break** (`reference_differential_break_protocol_count1`).
- **Confirm WHICH assertion fired** (`reference_deliberate_break_wrong_assertion`) — and whether a second property's firing is ENTAILED by the first.
- **A break that does NOT fire is a FINDING** — record it honestly in PROGRESS; do not route around it. (RD3's bare polite swap is the archetype: it is EXPECTED not to fire — that is the finding, and Break G is the corrected demonstration.)
- **Full selector only:** `-run 'TestDifferential/0111-tls-cvc-empty-dynamic-fallback'` — never bare `0111` (`reference_differential_run_selector`).
- **`Errorf` per independent property; `Fatalf` only for broken preconditions** (`reference_fatalf_makes_assertions_unreachable`).

### Identifier roster (`reference_spec_drafted_identifier_collision_check`)

**Verified FREE repo-wide at `fba6d385` (`grep -rn --include='*.go'`, `.worktrees` excluded — `[RUN]`):** `ErrEmptyValidationContext` (0) · `WithEmptyValidationContext` (0) · `IsEmptyValidationContext` (0) · `emptyVC` (0) · `cvcEmptyFuzzProvider` (0) · `TestParseValidationSecret_SpecifierUnset*` (0) · `TestApplyValidationResponse_DualSentinel*` (0) · `TestNewDownstreamConfig_CVC_EmptyDynamic*` (0). **Fixture `0111`:** `test/fixtures/0111-*` does not exist; `0111` appears nowhere under `test/differential/*.go`; in-container port **10447** FREE (`grep -rn '10447' test/` → 0). **Same-name-different-package is fine:** the `0111` `package driver` re-declares the `0110` helpers (`mustCA`/`mustLeaf`/`mustAllocatePort`/`structuralCheck`/`normalizeTLSErr`/`driveSide`/`wantObservable`) without collision — own package. **Any FURTHER name the IMPL coins: grep first, record the check.**

---

## File structure

```
internal/xds/stream.go            [EDIT]  T1 (define ErrEmptyValidationContext beside errValidation :33; dual-%w at :164)
internal/xds/secret.go            [EDIT]  T1 (wrap the sentinel at the specifier-unset site in parseValidationSecret, after :128 / before :129)
internal/xds/secret_test.go       [EDIT]  T1 (S1 → sentinel present; S2/S3 → sentinel absent — parseValidationSecret unit level)
internal/xds/stream_test.go       [EDIT]  T1 (applyValidationResponse dual-sentinel: S1 → both errValidation AND ErrEmptyValidationContext; empty-resources → errValidation only)
internal/tls/config.go            [EDIT]  T2 (the fallback branch + "errors" import + the :185-193 cause-scoped comment rewrite; P5 block :158-164 BYTE-INTACT — RD-P5)
internal/tls/config_test.go       [EDIT]  T2 (empty-served fallback; empty-both × {true,false,absent}; S2/S3 boot-FAIL; require × fallback)
test/helpers/sdsserver/sdsserver.go       [EDIT]  T3 (emptyVC field + WithEmptyValidationContext Option + buildResponse branch)
test/helpers/sdsserver/sdsserver_test.go  [EDIT]  T3 (TestWithEmptyValidationContext_ServesSpecifierUnset)
internal/tls/fuzz_test.go         [EDIT]  T4 (cvcEmptyFuzzProvider + "downstream-sds-empty" dispatch arm + the fallback seed; xds import; +0 func Fuzz)
test/fixtures/0111-tls-cvc-empty-dynamic-fallback/  [ADD]  T5 (driver/, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md), T6 (breaks)
docs/envoy-go/BEHAVIOR_CONTRACT.md [EDIT]  T7 (B1–B4 pinned verbatim)
docs/envoy-go/DECISIONS.md         [EDIT]  T9 (ADR-0290 completed IN PLACE — §Decision + §Consequences)
internal/xds/provider.go · internal/boot/** · internal/listener/** · validate/**  [BYTE-UNTOUCHED]
```

---

## Task 1 — `internal/xds`: the NARROW sentinel — define, wrap at the specifier-unset site, dual-`%w` preserve

**Files:**
- Modify: `internal/xds/stream.go:33` (define), `internal/xds/stream.go:164` (dual-`%w`)
- Modify: `internal/xds/secret.go` (wrap, after `:128` / before `:129`)
- Test: `internal/xds/secret_test.go`, `internal/xds/stream_test.go`

**Interfaces:**
- Produces: `var xds.ErrEmptyValidationContext error` (exported). Property fixed for consumers: on the S1 path `errors.Is(err, errValidation) && errors.Is(err, ErrEmptyValidationContext)` both hold; on S2/S3/timeout/empty-resources `errors.Is(err, ErrEmptyValidationContext)` is FALSE (`errValidation` still holds). `internal/tls` (T2) consumes `ErrEmptyValidationContext`; `provider.go` is NOT edited (RD-PROV — its `errors.Is(err, errValidation)` branch still fires because `errValidation` is in the dual chain).

**Entry state:** clean `fba6d385`-derived branch; `go test ./internal/xds/ -count=1` green.

- [ ] **Step 1 — write the failing unit tests (red-first).**

In `internal/xds/secret_test.go`, add `TestParseValidationSecret_SpecifierUnset_YieldsEmptySentinel` (S1) and `TestParseValidationSecret_SetButEmpty_And_Corrupt_NoSentinel` (S2/S3). Build the resource with `anypb.New(&tlsv3.Secret{Name: name, Type: &tlsv3.Secret_ValidationContext{ValidationContext: vc}})` where `vc` is:
- **S1:** `&tlsv3.CertificateValidationContext{}` (`TrustedCa` UNSET). Assert `errors.Is(err, ErrEmptyValidationContext)` is **true** (and `err != nil`).
- **S1b:** `&tlsv3.CertificateValidationContext{TrustedCa: &corev3.DataSource{}}` (`trusted_ca:{}` — DataSource present, specifier unset). Assert `errors.Is(err, ErrEmptyValidationContext)` **true** (RD-SENT second disjunct).
- **S2:** `TrustedCa: &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: []byte{}}}`. Assert `err != nil` AND `errors.Is(err, ErrEmptyValidationContext)` is **false**.
- **S3:** `TrustedCa: &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: []byte("-----BEGIN CERTIFICATE-----\nnotpem\n-----END CERTIFICATE-----\n")}}`. Assert `err != nil` AND `errors.Is(err, ErrEmptyValidationContext)` is **false**.

In `internal/xds/stream_test.go`, add `TestApplyValidationResponse_DualSentinel`: wrap the S1 resource in a `*discoveryv3.DiscoveryResponse` with one resource, call `applyValidationResponse`, assert BOTH `errors.Is(err, errValidation)` AND `errors.Is(err, ErrEmptyValidationContext)`. Add `TestApplyValidationResponse_EmptyResources_NoEmptySentinel`: a response with `Resources: nil` → assert `errors.Is(err, errValidation)` true AND `errors.Is(err, ErrEmptyValidationContext)` **false** (the scope pin — SPEC §3.2 M4).

Run `go test ./internal/xds/ -run 'TestParseValidationSecret_SpecifierUnset|TestApplyValidationResponse_DualSentinel' -count=1`. **Expected: FAIL** — `ErrEmptyValidationContext` is undefined (compile error at first; then, once declared, the assertions fail because nothing wraps it yet). Record the verbatim red.

- [ ] **Step 2 — define the sentinel.** In `internal/xds/stream.go`, directly below `errValidation` (`:33`):

```go
// ErrEmptyValidationContext classifies an SDS-delivered validation_context that
// ACKs but carries no usable trusted_ca specifier (the reference's merge-empty
// shape). It is wrapped ALONGSIDE errValidation (dual-%w in applyValidationResponse)
// so the provider still NACKs (update_rejected) while internal/tls can errors.Is it
// and fall back to default_validation_context.trusted_ca (phase 68, ADR-0290). It is
// NOT wrapped for a set-but-empty (inline_bytes:"") or corrupt trusted_ca, nor for a
// zero-resource response — those stay plain errValidation boot-FAILs, matching the
// reference's NACK.
var ErrEmptyValidationContext = errors.New("xds: sds: validation_context has no usable trusted_ca specifier")
```

(`errors` is already imported in `stream.go` — no new import; the ONE new EXPORTED symbol, streak break, stated loudly.)

- [ ] **Step 3 — wrap it at the specifier-unset site.** In `internal/xds/secret.go`, in `parseValidationSecret`, AFTER the four sub-field checks (`:117-128`) and BEFORE the `dataSourceBytes` call (`:129`), insert:

```go
	if vc.GetTrustedCa() == nil || vc.GetTrustedCa().GetSpecifier() == nil {
		// S1: the validation_context ACKs with no usable trusted_ca specifier
		// (trusted_ca unset, or trusted_ca:{} with no DataSource specifier). The
		// reference merges this empty context away and falls back to the default;
		// classify it so internal/tls can (phase 68, ADR-0290). A set-but-empty
		// inline_bytes:"" has a SET specifier and falls through to the parse-failure
		// reject below — NOT a fallback trigger.
		return nil, fmt.Errorf("xds: sds: validation secret %q: %w", wantName, ErrEmptyValidationContext)
	}
```

- [ ] **Step 4 — dual-`%w` the wrap.** In `internal/xds/stream.go`, `applyValidationResponse` (`:164`), change:

```go
		return nil, fmt.Errorf("%w: %v", errValidation, err)
```
to
```go
		return nil, fmt.Errorf("%w: %w", errValidation, err)
```

**⚠️ RD-DUPSITE:** this exact string ALSO appears at `stream.go:98` in `applyResponse` (the leaf-cert path) — edit ONLY the `applyValidationResponse` occurrence (`:164`). Use a line-anchored Edit (or include the surrounding `parseValidationSecret` context), NEVER a `replace_all`. Verify `git diff internal/xds/stream.go` touches `:164` and NOT `:98`. **Do NOT touch `:160`** (`fmt.Errorf("%w: empty resources", errValidation)` — which ALSO has a twin at `:94`) — the empty-resources branch stays `errValidation`-only (scope pin).

- [ ] **Step 5 — run the tests.** `go test ./internal/xds/ -count=1`. **Expected: PASS** (the new S1/S1b assertions green; S2/S3/empty-resources green; every pre-existing xds test green — the dual-`%w` change is behavior-preserving for `errors.Is(err, errValidation)` which `provider.go` and existing NACK tests depend on).

- [ ] **Step 6 — breaks (AFTER committing; `reference_break_protocol_commit_first`).**
  - **Break A [dual-`%w` load-bearing]:** revert `:164` to `%w: %v`. Re-run `TestApplyValidationResponse_DualSentinel` → its `errors.Is(err, ErrEmptyValidationContext)` assertion FIRES (the sentinel is flattened, lost to `errors.Is`). Confirm WHICH assertion. `git restore`; re-green.
  - **Break B [scope pin]:** change the empty-resources wrap in `applyValidationResponse` (`:160`, NOT the `:94` twin in `applyResponse` — RD-DUPSITE) to also carry the sentinel (`fmt.Errorf("%w: %w", errValidation, ErrEmptyValidationContext)`). Re-run `TestApplyValidationResponse_EmptyResources_NoEmptySentinel` → its `errors.Is(...)==false` assertion FIRES. `git restore`; re-green. (Proves the sentinel does not leak into the zero-resource branch — a leak would fall back on a server that delivered NOTHING.)

- [ ] **Step 7 — hygiene + commit.** `gofmt -l internal/xds` silent · `go vet ./internal/xds/` · `golangci-lint run ./internal/xds/`. **Cycle guard:** `go list -deps ./internal/xds | grep 'envoy-go/internal'` (**no `...`**) ⇒ `internal/stats` + `internal/xds` ONLY (the export adds no import — TYPE-level, `reference_xds_config_seam_transitive_cycle_guard`).

**Commit:** `xds(phase 68 T1): NARROW empty-validation-context sentinel — define ErrEmptyValidationContext beside errValidation, wrap ONLY the specifier-unset shape in parseValidationSecret, dual-%w in applyValidationResponse so both sentinels survive to internal/tls (S2/S3/empty-resources stay errValidation-only); provider.go byte-untouched`

---

## Task 2 — `internal/tls/config.go`: the empty-dynamic fallback branch + empty-both routing

**Files:**
- Modify: `internal/tls/config.go` (add `"errors"` import; the fallback in the CVC arm's `:184` err-branch; the `:185-193` comment rewrite)
- Test: `internal/tls/config_test.go`

**Interfaces:**
- Consumes: `xds.ErrEmptyValidationContext` (T1). `loadTrustedCAPool(vc, baseDir, side)` (`:298`), `installPool(pool) error` (the `:89-96` closure — errors on nil pool, sets `ClientCAs` + `clientAuthFor(require)` adjacently), `clientAuthFor(require) stdtls.ClientAuthType` (`:78-83`).
- Produces: nothing new exported.

**Entry state:** T1 landed; `go test ./internal/xds/ ./internal/tls/ -count=1` green.

- [ ] **Step 1 — write the failing unit tests (red-first).** In `internal/tls/config_test.go`, using `cvcCTC()` / `cvcDownstreamTS` / `cvcWithDefaultVC` / `fakeProvider`:

1. `TestNewDownstreamConfig_CVC_EmptyDynamic_FallsBackToDefault` — `provider = &fakeProvider{vcErr: fmt.Errorf("xds: sds: validation secret %q: %w", "x", xds.ErrEmptyValidationContext)}`, CVC with a valid inline `default_validation_context.trusted_ca` (`cvcCTC()`), across **require=true** and **require=false/absent** subtests. Assert `err == nil`, `cfg.TLSConfig.ClientCAs != nil`, and `cfg.TLSConfig.ClientAuth == RequireAndVerifyClientCert` (true) / `VerifyClientCertIfGiven` (false/absent). *(The fake's `vcErr` models the real dual-wrapped chain on the ONE bit `config.go` reads — `errors.Is(…, ErrEmptyValidationContext)`; note in the test comment that the real chain also carries the unexported `errValidation`.)*
2. `TestNewDownstreamConfig_CVC_EmptyBoth_NoAnchor` — same `vcErr`, but the CVC's `default_validation_context` carries NO `trusted_ca` (`cvcCTC(func(c){ c.DefaultValidationContext = &tlsv3.CertificateValidationContext{} })`). Subtests: require=true → `err != nil`, substring `require_client_certificate=true requires validation_context.trusted_ca` (the `:203` message), nil cfg; require=false AND absent → `err == nil`, `cfg.TLSConfig.ClientAuth == NoClientCert`, `cfg.TLSConfig.ClientCAs == nil`.
3. `TestNewDownstreamConfig_CVC_SetButEmpty_And_Corrupt_BootFail` — `vcErr` carrying `errValidation`-shaped text WITHOUT the sentinel (e.g. `errors.New("xds: sds: validation secret \"x\": trusted_ca: parse failure")`), require=true. **Build the CVC with a VALID `default_validation_context.trusted_ca` (plain `cvcCTC()`, M3)** so the test DISCRIMINATES: with a valid default, removing the `errors.Is` gate (Break C) WOULD fall back to `err == nil`, so the boot-FAIL assertion is what catches Break C. (An empty default would boot-FAIL via the empty-both branch regardless of the gate, making Break C a vacuous non-fire.) Assert `err != nil` (boot-FAIL — NO fallback), nil cfg. (Pins the NARROW gate at the consumer: a non-sentinel fetch error does NOT fall back.)
4. `TestNewDownstreamConfig_CVC_EmptyDynamic_RequireEnforcedAgainstFallback` — the require × fallback cross-product at the unit level: require=true + sentinel + valid default ⇒ `RequireAndVerifyClientCert` + non-nil `ClientCAs` (the wire-level CA_B-reject / no-cert-reject is the fixture's job, T5; note that in the comment).

Run `go test ./internal/tls/ -run 'TestNewDownstreamConfig_CVC_Empty' -count=1`. **Expected: FAIL** — the fallback branch does not exist, so the empty-dynamic error boot-FAILs (tests 1/2/4 red on `err != nil`; test 3 already green as a regression pin — record it green, no red owed, per the phase-66 F2 discipline). Record the verbatim red.

- [ ] **Step 2 — add the `errors` import.** In `internal/tls/config.go`'s import block (`:3-14`), add `"errors"`.

- [ ] **Step 3 — the fallback branch.** In the CVC arm, replace the fetch-and-error block **`:183-198`** (M1 — the block spans the `pool, err := provider.FetchInitialValidationContext(...)` declaration at `:183` THROUGH the trailing `if err := installPool(pool); err != nil { … }` at `:196-198`; a literal "replace :184-195" would double-declare `pool` and duplicate the trailing install). The full replacement (RETURNS on every fallback path so control never reaches the post-error `installPool(pool)` with a nil pool; on the success path the trailing `installPool(pool)` at the block's tail runs as today):

```go
		pool, err := provider.FetchInitialValidationContext(context.Background(), secretName)
		if err != nil {
			if errors.Is(err, xds.ErrEmptyValidationContext) {
				// Empty-dynamic fallback (phase 68, ADR-0290 — the honored-surface
				// projection of MergeFrom, P3 restated). The SDS half ACKed a
				// validation_context with no usable trusted_ca specifier (S1); the
				// reference merges the empty context away and the DEFAULT's trusted_ca
				// survives, so envoy-go falls back to it. The default is read via
				// internal/tls's OWN loadTrustedCAPool (never across the internal/xds
				// seam), so the P5 coincidence hazard does not apply to this branch.
				// A set-but-empty (inline_bytes:"") / corrupt served trusted_ca, a
				// timeout, or an unreachable server does NOT carry the sentinel and
				// falls through to the boot-FAIL below (matching the reference's NACK /
				// per-connection fail-closed).
				dvc := vct.CombinedValidationContext.GetDefaultValidationContext() // E1-guaranteed non-nil (config.go:427-429)
				if dvc.GetTrustedCa() == nil {
					// Empty-both: the served VC AND the default both lack an anchor.
					// Route through the phase-67 no-anchor logic exactly like the inline
					// default arm below (D-EDF-EMPTYBOTH — no new reject).
					if require {
						return nil, fmt.Errorf("tls: downstream: require_client_certificate=true requires validation_context.trusted_ca")
					}
					return &DownstreamConfig{TLSConfig: cfg}, nil // false/absent + no anchor -> NoClientCert
				}
				fbPool, ferr := loadTrustedCAPool(dvc, baseDir, "downstream")
				if ferr != nil {
					return nil, ferr
				}
				if ierr := installPool(fbPool); ierr != nil {
					return nil, ierr
				}
				return &DownstreamConfig{TLSConfig: cfg}, nil
			}
			// A served validation context that is present-but-broken (inline_bytes:"",
			// corrupt PEM), a timeout, or an unreachable management server boot-FAILS
			// the listener (ADR-0280 family; the reference NACKs the broken shapes and
			// fails closed per-connection, characterization corrected at ADR-0289).
			// envoy-go refuses to boot on these causes — only the ACKed-empty S1 shape
			// above falls back and serves.
			return nil, fmt.Errorf("tls: downstream: SDS validation secret %q: %w", secretName, err)
		}
		if err := installPool(pool); err != nil {
			return nil, err
		}
```

**RD-P5 pin:** the P5 comment block `:158-164` is BYTE-UNTOUCHED — verify `git diff internal/tls/config.go` shows ZERO change in that range (the CVC arm does not move; the reduced-hazard note lives in the NEW fallback comment above, not by editing the mandatory P5 block).

- [ ] **Step 4 — run the tests.** `go test ./internal/tls/ -count=1`. **Expected: PASS** — tests 1/2/4 green; test 3 stays green; every pre-existing CVC test (`TestCVC_*`, `TestNewDownstreamConfig_RequireFalse_CVC_VerifyIfGiven`, the E1/E2/sub-field rejects) green (the success path is unchanged — the fallback is reachable only via the sentinel).

- [ ] **Step 5 — breaks (AFTER committing).**
  - **Break C [narrow gate]** `[NOT an exact one-line deletion — V1]`: make the fallback fire on ANY fetch error. A literal deletion of the `if errors.Is(err, xds.ErrEmptyValidationContext) {` line leaves a dangling `}` and will NOT compile — instead widen the condition to `if errors.Is(err, xds.ErrEmptyValidationContext) || true {` (compiles; the fallback body already returns on all paths). Re-run test 3 → it FIRES (an S2/S3-shaped non-sentinel error now wrongly falls back to its VALID default → `err == nil` where boot-FAIL expected). Confirm WHICH assertion. `git restore`; re-green.
  - **Break D [empty-both require gate]:** in the empty-both branch, drop the `if require { return … }` (always `NoClientCert`). Re-run test 2's require=true subtest → its reject-substring `Errorf` FIRES. `git restore`; re-green.
  - **Break E [install]:** replace `installPool(fbPool)` with `_ = fbPool` (skip install). Re-run test 1 → `ClientCAs != nil` FIRES (and, via `installPool`'s nil-guard being bypassed, `ClientAuth` stays zero-value). Confirm the `ClientCAs`/`ClientAuth` assertion, not an err-path abort. `git restore`; re-green.

- [ ] **Step 6 — retained-reject byte-diff + hygiene + commit.** Grep each retained substring byte-identical + count-unchanged: `:203` (`require_client_certificate=true requires validation_context.trusted_ca`) · E1 `:427-429` · the four `:448-461` sub-field rejects · the plain-SDS-VC / CVC "not supported" gates. `gofmt -l internal/tls` silent · `go vet ./internal/tls/` · `golangci-lint run ./internal/tls/`.

**Commit:** `tls(phase 68 T2): empty-dynamic fallback in the CVC arm — on errors.Is(ErrEmptyValidationContext) fall back to default_validation_context.trusted_ca via loadTrustedCAPool+installPool (empty-both routes to the phase-67 no-anchor logic, no new reject); S2/S3/timeout stay boot-FAIL; P5 block byte-intact; the :185-193 departure comment rewritten cause-scoped`

---

## Task 3 — `sdsserver.WithEmptyValidationContext` — serve the specifier-unset (S1) shape

**Files:**
- Modify: `test/helpers/sdsserver/sdsserver.go` (a new `emptyVC bool` field; the Option; the `buildResponse` branch)
- Test: `test/helpers/sdsserver/sdsserver_test.go`

**Interfaces:**
- Produces: `func WithEmptyValidationContext(name string) Option` — serves `Secret{name, validation_context:{}}` with `trusted_ca` UNSET (S1). Distinct from `WithValidationContext(name, nil)` which serves `trusted_ca:{inline_bytes:""}` (S2 — empirically confirmed at the SPEC via a go-control-plane oneof round-trip). Consumed by the `0111` driver (T5).

**Entry state:** T1–T2 landed; `go test ./test/helpers/sdsserver/ -count=1` green.

- [ ] **Step 1 — write the failing test (red-first).** In `sdsserver_test.go`, model on `TestWithValidationContext_ServesValidationSecret` (`:177`). Add `TestWithEmptyValidationContext_ServesSpecifierUnset`: `srv := New(t, WithEmptyValidationContext("empty_vc"))`, drive one `StreamSecrets` exchange, unmarshal the served resource to `*tlsv3.Secret`, assert `sec.GetValidationContext() != nil` (the VC oneof arm IS selected) AND `sec.GetValidationContext().GetTrustedCa() == nil` (specifier-unset — the S1 shape; NOT a non-nil DataSource). Run `go test ./test/helpers/sdsserver/ -run 'TestWithEmptyValidationContext' -count=1`. **Expected: FAIL** — `WithEmptyValidationContext` undefined. Record.

- [ ] **Step 2 — add the field.** In the `Server` struct, beside `vcSecretName`/`trustedCAPEM` (`:38-39`):

```go
	// emptyVC, when true with vcSecretName set, serves validation_context:{} with
	// trusted_ca UNSET (the specifier-unset S1 shape) — the reference's merge-empty
	// case. WithValidationContext(name,nil) instead yields trusted_ca:{inline_bytes:""}
	// (S2), which does NOT trigger the fallback (phase 68). See WithEmptyValidationContext.
	emptyVC bool
```

- [ ] **Step 3 — add the Option.** Below `WithValidationContext` (`:64`):

```go
// WithEmptyValidationContext configures the delivered
// Secret{name, validation_context:{}} with trusted_ca UNSET — the specifier-unset
// (S1) shape the reference ACKs, merges away, and falls back on (phase 68). Distinct
// from WithValidationContext(name, nil), which serves trusted_ca:{inline_bytes:""} (S2,
// a set-but-empty DataSource that stays a reject on both sides).
func WithEmptyValidationContext(name string) Option {
	return func(s *Server) { s.vcSecretName = name; s.emptyVC = true }
}
```

- [ ] **Step 4 — branch `buildResponse`.** Replace the `case s.vcSecretName != "":` body (`:143-148`) with:

```go
		case s.vcSecretName != "":
			// Phase 65: the validation_context arm — the SAME tls.v3.Secret message,
			// a different oneof arm. Phase 68: emptyVC serves it with trusted_ca UNSET
			// (S1); otherwise a non-nil inline trusted_ca (the phase-65 shape).
			vc := &tlsv3.CertificateValidationContext{}
			if !s.emptyVC {
				vc.TrustedCa = &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: s.trustedCAPEM}}
			}
			sec = &tlsv3.Secret{
				Name: s.vcSecretName,
				Type: &tlsv3.Secret_ValidationContext{ValidationContext: vc},
			}
```

- [ ] **Step 5 — run the test.** `go test ./test/helpers/sdsserver/ -count=1`. **Expected: PASS** (the new test green; `TestWithValidationContext_ServesValidationSecret` and all others UNCHANGED — the `!s.emptyVC` default reproduces the phase-65 shape byte-for-byte).

- [ ] **Step 6 — break (AFTER committing).** **Break F [S1 vs S2]:** in `buildResponse`, force `vc.TrustedCa = &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: nil}}` even when `emptyVC` (i.e. serve S2 instead of S1). Re-run `TestWithEmptyValidationContext_ServesSpecifierUnset` → its `GetTrustedCa() == nil` assertion FIRES (the served DataSource is now non-nil). `git restore`; re-green. (Proves the test discriminates S1 from S2 — the exact distinction the row rests on.)

- [ ] **Step 7 — hygiene + commit.** `gofmt -l test/helpers/sdsserver` silent · `go vet ./test/helpers/sdsserver/` · `golangci-lint run ./test/helpers/sdsserver/`.

**Commit:** `sdsserver(phase 68 T3): WithEmptyValidationContext serves the specifier-unset (S1) shape — trusted_ca UNSET, distinct from WithValidationContext(name,nil)'s inline_bytes:"" (S2); the chartered test-helper Option for 0111 (the "sdsserver untouched" envelope claim corrected)`

---

## Task 4 — fuzz: a new dispatch side + provider + seed exercising the fallback (+0 fuzzers)

**Files:**
- Modify: `internal/tls/fuzz_test.go` (add `xds` import; `cvcEmptyFuzzProvider`; the `"downstream-sds-empty"` dispatch arm; the fallback seed)

**Entry state:** T1–T3 landed.

- [ ] **Step 1 — add the provider + import.** In `fuzz_test.go`, add `xds "github.com/pgdad/envoy-go/internal/xds"` to the imports and, near `cvcFuzzProvider` (`:31`):

```go
// cvcEmptyFuzzProvider returns the classified empty-validation-context error so a
// CVC seed dispatched via "downstream-sds-empty" reaches NewDownstreamConfig's
// empty-dynamic FALLBACK branch (phase 68). Its vcErr satisfies
// errors.Is(_, xds.ErrEmptyValidationContext); a CVC with a valid
// default_validation_context.trusted_ca then falls back and returns nil error.
var cvcEmptyFuzzProvider = &fakeProvider{vcErr: fmt.Errorf("xds: sds: validation secret %q: %w", "fuzz_empty_vc", xds.ErrEmptyValidationContext)}
```

(`fmt` is needed — add if absent; `fakeProvider` is same-package from `config_test.go`.)

- [ ] **Step 2 — add the dispatch arm.** In the fuzz body `switch side` (`:446-458`), add:

```go
		case "downstream-sds-empty":
			// Phase 68: cvcEmptyFuzzProvider returns the classified empty-VC error, so
			// a well-formed CVC (valid default trusted_ca) reaches the empty-dynamic
			// fallback and returns nil error. See the dispatch trap in the seed comment.
			_, err = NewDownstreamConfig(ts, "", cvcEmptyFuzzProvider)
```

- [ ] **Step 3 — add the seed.** After the last CVC seed (`:437`):

```go
	// Seed (s), phase 68: a well-formed combined_validation_context (cvcCTC(): a
	// valid default_validation_context.trusted_ca + a valid
	// validation_context_sds_secret_config) + require:true, dispatched via
	// "downstream-sds-empty" so cvcEmptyFuzzProvider's classified empty-VC error
	// drives the FALLBACK: NewDownstreamConfig falls back to the default's trusted_ca,
	// installs it as ClientCAs, and returns NIL error. THE NAMED VACUITY TRAP (SPEC §7):
	// on "downstream" (nil provider) this shape dies at commonTLSContextToConfig's
	// retained "combined_validation_context is not supported in phase 03" gate; on
	// "downstream-sds" (cvcFuzzProvider — a VALID pool) the fetch SUCCEEDS and never
	// reaches the empty branch. Only "downstream-sds-empty" exercises the fallback.
	{
		inner := &tlsv3.DownstreamTlsContext{
			RequireClientCertificate: &wrapperspb.BoolValue{Value: true},
			CommonTlsContext:         cvcCTC(),
		}
		anyTC, err := anypb.New(inner)
		if err != nil {
			f.Fatalf("anypb.New: %v", err)
		}
		f.Add("downstream-sds-empty", anyTC.GetTypeUrl(), anyTC.GetValue())
	}
```

- [ ] **Step 4 — dispatch-verify (the named trap).** Temporarily add a diagnostic (`t.Logf`) in the fuzz body that records `side` + `err` for the new seed; run `go test ./internal/tls/ -run FuzzTLSContextParse -count=1 -v`; CONFIRM the seed on `"downstream-sds-empty"` yields `err == nil` (fallback fired). Then, as a one-off scratch check (NOT committed), flip the seed's side to `"downstream"` and confirm it dies with `combined_validation_context is not supported in phase 03` (the trap); flip to `"downstream-sds"` and confirm `err == nil` for a DIFFERENT reason (the pool substitution success path, not the fallback). REMOVE the diagnostic. Record all three in PROGRESS.

- [ ] **Step 5 — reconcile the count.** `grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l` → **55** BEFORE and AFTER (`reference_fuzzer_count_docs_drift` — a new dispatch side + seed is +0 `func Fuzz`). A short active-fuzz smoke (`go test -run FuzzTLSContextParse -fuzz FuzzTLSContextParse -fuzztime 10s ./internal/tls/`) — no panic; NO corpus artifacts committed.

- [ ] **Step 6 — hygiene + commit.** `gofmt -l internal/tls` silent · `go vet ./internal/tls/` · `golangci-lint run ./internal/tls/`.

**Commit:** `tls(phase 68 T4): fuzz — a "downstream-sds-empty" dispatch side + cvcEmptyFuzzProvider + a fallback seed exercising the empty-dynamic branch (+0 fuzzers, 55→55; the "downstream"/"downstream-sds" vacuity traps dispatch-verified)`

---

## Task 5 — fixture `0111-tls-cvc-empty-dynamic-fallback` (fixtures 112 → 113)

**Files:**
- Create: `test/fixtures/0111-tls-cvc-empty-dynamic-fallback/{driver/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}`

**Entry state:** T1–T4 landed; `test/fixtures/0111*` does not exist (verified).

**Design (SPEC §8 — CVC-primary at require=true, the `0110` chassis as a disciplined clone; the served dynamic VC is EMPTY so the fallback to the inline default is under test):**

- `tcp_proxy` echo; `BackendCount() == 1` (`reference_differential_backendcount_min_one`); the accepting arm genuinely echoes through it.
- **PKI (in-memory, the `0110` `mustCA`/`mustLeaf` helpers, own `package driver`):**
  - **CA_A** — the inline `combined_validation_context.default_validation_context.trusted_ca`. This is the FALLBACK anchor that MUST WIN (opposite role vs `0110`, where the inline default lost).
  - **CA_B** — a foreign CA; `client_B` chains to it and MUST be rejected.
  - Server leaf signed by **CA_A**; the driver's `RootCAs` trusts CA_A so all arms clear server verification (only the proxy's verdict on our client cert is under test).
  - `client_A` chains to CA_A → MUST be accepted (via the fallback). `client_B` chains to CA_B → MUST be rejected.
- **Both YAMLs:** CVC with inline `default_validation_context.trusted_ca` = **CA_A** + `validation_context_sds_secret_config` served an **EMPTY validation_context** (via `sdsserver.WithEmptyValidationContext(secretName)`) + **`require_client_certificate: true`** (a BARE scalar — `{value: true}` ERRORS, `reference_protojson_wrapper_scalar_not_object`). Port **10447**.
- **Per-side driver-owned `sdsserver.Server`s** started with `WithEmptyValidationContext` (T3), hard `Close` on teardown (GracefulStop deadlocks on the long-lived StreamSecrets), ARM-UNIQUE secret name, and the **served-this-arm precondition assert** (the `0110` `driveSide` :394-398 pattern; `feedback_probe_fresh_container_per_arm`).
- **THREE arms at require=true** (the `0110` structure with require=true semantics):
  - `trusted` (`client_A`, **FORCED-SEND** via `GetClientCertificate`) → ok+echo — proves the fallback anchor is CA_A and LIVE.
  - `untrusted` (`client_B`, **FORCED-SEND**, `reference_go_client_cert_withholding`) → rejected — the untrusted ARM UPPER-BOUNDS the fallback pool to CA_A: a `CA_A∪CA_B` union / accept-all pool would ACCEPT `client_B` (caught by Break G, T6). **RD3:** at require=true the forced-send is NOT the observable's discriminator (a polite dial yields the same `rejected`, and the union hazard is caught in both modes because a union pool advertises CA_B); forced-send is retained so the arm actually EXERCISES verify-and-reject rather than collapsing into a no-cert duplicate of the `none` arm, and to keep both sides symmetric. Do NOT claim forced-send flips the observable here.
  - `none` (no client cert) → rejected — proves require=true is ENFORCED against the fallback anchor (alert 116).
- `wantObservable = "trusted=ok echo=" + probePayload + "untrusted=rejected\nnone=rejected\n"`.
- `structuralCheck` (three-arm, per-side, ALL violations reported — `reference_fatalf_makes_assertions_unreachable`) + `normalizeTLSErr`; **PER-SIDE failure pins** — never cross-side string equality (`reference_differential_reference_parses_full_message`; the wire alert agrees per side, client-observed strings differ).
- **One fixture dir = ONE runner branch** — pure cross-side (`reference_differential_fixture_dispatch_constraint`); never assert `/listeners`; **never treat a docker-proxy accept as listener liveness** (SPEC §8 discipline — S2/S3 report `/ready` LIVE while every connection fail-closes; the served-this-arm assert + a real handshake are the truth). `reference_envoy_contrib_image_tagging` / `reference_host_gateway_ip_docker_desktop` apply.
- `expectations.yaml` + `README.md`: clone `0110`'s shape; state the proposition (*the empty-served dynamic VC falls back to the inline default CA_A and SERVES; require=true is enforced against the fallback anchor; the served pool is CA_A specifically — the untrusted arm upper-bounds it, rejecting a union*), name the S2/S3 boot-FAIL siblings (reference NACKs), and the forced-send rationale (RD3 — retained so the untrusted arm EXERCISES verify-and-reject and stays symmetric cross-side; NOT because it flips the require=true observable, which it does not).
- 0018/0108/0109/0110 expectations UNCHANGED.

- [ ] **Step 1 — write `driver/driver.go`** (clone `0110`, apply the deltas above; the CA roles INVERT — inline default WINS via fallback). `fixtureName = "0111-tls-cvc-empty-dynamic-fallback"`, `refListenerPort = 10447`, arm-unique `secretName`, distinct `serverName`.
- [ ] **Step 2 — write `envoy.yaml` / `envoy-go.yaml`** (clone `0110`'s CVC templates; `require_client_certificate: true`; the SDS secret config points at `{{.SDSPort}}`; inline `default_validation_context.trusted_ca` = `{{.InlineCA}}` = CA_A).
- [ ] **Step 3 — write `expectations.yaml` / `README.md`.**
- [ ] **Step 4 — run.** `go test ./test/differential/ -run 'TestDifferential/0111-tls-cvc-empty-dynamic-fallback' -count=1`. **Expected: PASS**; fixture count 113 (`ls -d test/fixtures/[0-9]*/ | wc -l`).
- [ ] **Step 5 — hygiene + commit.** Trio on the driver package.

**Commit:** `differential(phase 68 T5): fixture 0111-tls-cvc-empty-dynamic-fallback — CVC-primary three-arm verdict at require=true, empty served dynamic VC falling back to the inline default CA_A, FORCED-SEND untrusted arm (fixtures 112→113, port 10447, per-side pins, served-this-arm assert)`

---

## Task 6 — 0111 deliberate breaks (the fixture is not done until its assertions are PROVEN live)

**Entry state:** T5 committed green. Every break: `-count=1`, full selector, `git restore` only, confirm WHICH assertion fired.

- [ ] **Break G [the untrusted arm UPPER-BOUNDS the fallback pool — the CORRECTED RD3 demonstration].** RD3 (empirically confirmed): at require=true, forced-send is NOT observably load-bearing (a bare polite swap does NOT flip the observable — both send-modes → `untrusted=rejected`), and the draft's "two-factor" break was UNSOUND (a permissive `CA_A∪CA_B` pool ADVERTISES CA_B, so the polite client SENDS `client_B` too → the break FIRES, it does not go vacuous-green). So prove the untrusted arm's REAL power directly:
  1. **Symmetric union-pool break:** make BOTH proxies' inline `default_validation_context.trusted_ca` = CA_A **∪** CA_B (template both CA PEMs into the default `trusted_ca`; `x509.CertPool.AppendCertsFromPEM` accepts a concatenated PEM). The correct impl then installs the fallback pool `{CA_A, CA_B}`. `client_B` (FORCED-SEND, INTACT) → verify-OK vs `{CA_A,CA_B}` → **ACCEPTED** → `untrusted=ACCEPTED` → `structuralCheck` FIRES (per-side untrusted-arm violation), while `CompareBytes` stays EQUAL (both sides union identically — `reference_vacuous_break_receiver_normalizes`). This proves the untrusted arm UPPER-BOUNDS the fallback pool to CA_A ONLY (rejects a union). Confirm the untrusted-arm `Errorf`, not another. `git restore`; re-green. `[NOT pre-compiled — substitution rule applies for the two-CA templating; report it.]`
  2. **Record the bare-polite non-fire (the RD3 finding — a break that does NOT fire, honestly recorded, NOT routed around):** replace ONLY the untrusted arm's `cfg.GetClientCertificate = …` with polite `cfg.Certificates = []stdtls.Certificate{d.clientB}` on the CORRECT (non-union) impl. `client_B` is WITHHELD (issuer CA_B not advertised in the `{CA_A}` pool's CertificateRequest) → require=true rejects for no-cert → `untrusted=rejected` → `structuralCheck` PASSES. This confirms RD3: forced-send is not observably load-bearing at require=true. It is retained per `reference_go_client_cert_withholding` to keep the arm exercising verify-and-reject (else it degenerates into a no-cert duplicate of the `none` arm) and to keep both sides symmetric — NOT because a bare swap flips the observable. Record the PASS as the finding.

- [ ] **Break H [structuralCheck load-bearing — symmetric served/fallback-CA swap].** Change the inline default `trusted_ca` from CA_A to CA_B on BOTH sides (symmetric). Correct impl then trusts CA_B: `client_A` → REJECTED, `client_B` → ACCEPTED → the observable flips identically on both sides → `CompareBytes` still EQUAL (`reference_vacuous_break_receiver_normalizes`) but `structuralCheck` FIRES (per-side `trusted` + `untrusted` arm violations). Three-step demonstration (the `0110` T5 protocol): (a) disable `structuralCheck` + apply the symmetric swap → observe the fixture ship PASS; (b) re-enable → `structuralCheck` fires; (c) ASYMMETRIC swap (subject-side only) with the check disabled → `CompareBytes` mismatch, record the byte offset (proves `CompareBytes` independently live). Record each.

- [ ] **Break I [per-side failure-pin live].** Perturb one side's pinned untrusted/none-arm failure token by one character → that side's pin fires ALONE. `[NOT pre-compiled — substitution rule applies]`.

- [ ] **Break J [served-this-arm assert live].** Point one arm's serve at a stale/other server (or skip the serve) → the served-this-arm precondition assert fires BEFORE any verdict is compared (`feedback_probe_fresh_container_per_arm`). `[NOT pre-compiled — substitution rule applies]`.

A break that does not fire is a FINDING recorded in PROGRESS (Break G step 3 is the expected non-fire — recorded as the RD3 confirmation, not a defect).

**Commit:** `differential(phase 68 T6): 0111 liveness — Break G (symmetric union-pool) proves the untrusted arm UPPER-BOUNDS the fallback pool to CA_A (fires in both send-modes; the bare-polite non-fire recorded as the RD3 finding — forced-send is NOT observably load-bearing at require=true); symmetric fallback-CA swap structuralCheck triplet; per-side pin + served-this-arm breaks` *(tree byte-identical to T5's code unless a break exposes a real fix; record results in PROGRESS)*

---

## Task 7 — BEHAVIOR_CONTRACT delta (B1–B4) — pinned VERBATIM

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`

**Entry state:** T1–T6 landed. Docs-only. Anchor by SYMBOL / first-clause, not line (SPEC §9 lines are as of `f8416afd`; re-locate each at the IMPL tip).

- [ ] **B1 — CVC Supported ¶ (SPEC §9 B1, ~BC:918):** append the empty-dynamic fallback clause VERBATIM (SPEC §9 B1: "When the SDS half ACKs an EMPTY `validation_context`… falls back to `default_validation_context.trusted_ca`… A served `trusted_ca` that is present-but-broken (`inline_bytes:""`, corrupt PEM) is NOT a fallback trigger — it stays a reject, matching the reference's NACK.").
- [ ] **B2 — BC:924 item 3 (per-shape DEPARTURE):** (a) flips to **CLOSED** (envoy-go now MATCHES via `xds.ErrEmptyValidationContext`); name the RESIDUAL S1-only NACK/`update_rejected` stream-posture divergence (unasserted); (b)/(c) RETAINED + EXTENDED to name the SDS-delivered `inline_bytes:""` (S2) and corrupt-PEM (S3) boot-FAIL siblings the reference NACKs. Pinned wording from SPEC §9 B2.
- [ ] **B3 — BC:938 item 10 (the default's `trusted_ca` is NEVER READ) — NARROWED:** "*(NARROWED at phase 68/ADR-0290.)* The default's `trusted_ca` is read on exactly ONE branch — the empty-dynamic fallback… On the success path it is still never read (substitution)." (SPEC §9 B3 verbatim.)
- [ ] **B4 — the `0111` Differential-coverage ¶** (BC:952/:954 neighborhood): standard NEW-fixture form — CVC-primary, `default=CA_A` served EMPTY dynamic, three-arm forced-send verdict at require=true, served-this-arm + `structuralCheck` discipline, the new `WithEmptyValidationContext` Option.
- [ ] **Verify UNCHANGED:** BC:922 (merge-equivalence proof — the item's success-path text stands), BC:928 (E1/E2), BC:930 (shared gaps), BC:936 (f3/f4 UNPROBED).

**Commit:** `docs(phase 68 T7): BEHAVIOR_CONTRACT B1–B4 applied verbatim — CVC empty-dynamic fallback Supported clause, item 3(a) CLOSED + (b)/(c) naming S2/S3, item 10 NARROWED (default trusted_ca read on the fallback branch), the 0111 Differential-coverage ¶`

---

## Task 8 — VERIFY: the six-gate + cycle guard + full differential + `-race` + counts + envelope audit

Controller-run on the frozen pre-stage-close HEAD:

- [ ] 1. `gofmt -l internal/ test/ cmd/` — SILENT
- [ ] 2. `go vet ./...` — exit 0
- [ ] 3. `go build ./...` — exit 0
- [ ] 4. `go mod tidy -diff` EMPTY + `git diff --exit-code master -- go.mod go.sum` EMPTY (+0 modules — `reference_new_subpackage_pulls_transitive_module`)
- [ ] 5. `golangci-lint run ./...` — exit 0
- [ ] 6. **FULL differential:** `go test ./test/differential/ -count=1` — all **113** dirs, exit 0. The 112 pre-existing dirs byte-stable. `reference_differential_fullsuite_startup_flake`: a `subject ready: EOF` on an UNRELATED fixture is a startup race — isolate-re-run; `reference_0061_ring_hash_spread_flake` on a second occurrence → investigate margins.

**Plus:**
- [ ] **Cycle guard:** `go list -deps ./internal/xds | grep 'envoy-go/internal'` (**no `...`**) ⇒ `internal/stats` + `internal/xds` ONLY (`reference_xds_config_seam_transitive_cycle_guard` — TYPE-level; the new export adds no import — but ASSERT it).
- [ ] **`-race` on touched packages:** `go test ./internal/tls/ ./internal/xds/ ./test/helpers/sdsserver/ -race -count=1` (the `init_fetch_timeout` flake caveat stands).
- [ ] **Counts MECHANICAL, never copied:** fixtures **113** (tail `0111-tls-cvc-empty-dynamic-fallback`) · fuzzers **55** (`^func Fuzz`) · BackendKind **38** · DECISIONS tail **ADR-0290** · stat surface **1201** · go.mod diff EMPTY.
- [ ] **Envelope audit:** `git diff master --stat` shows functional production = `internal/tls/config.go` + `internal/xds/{secret.go,stream.go}` ONLY; **`internal/xds/provider.go` ABSENT** (RD-PROV); `internal/boot`/`internal/listener`/`validate/` ABSENT; `test/helpers/sdsserver/sdsserver.go` = the one chartered Option. **ONE new exported `internal/xds` symbol** confirmed (`ErrEmptyValidationContext`); ZERO new packages/modules/stats/BackendKinds.

*(No separate commit — T8's evidence lands in PROGRESS at T9.)*

---

## Task 9 — ADR-0290 completed IN PLACE + stage-close (controller-adjacent)

- [ ] **ADR-0290: COMPLETE IN PLACE** — append §Decision + §Consequences to the EXISTING entry (DECISIONS.md:17052; the §Context landed at the SPEC squash, STATUS: IN PROGRESS). Flip the STATUS banner to **COMPLETE**. **Do NOT append a new ADR; do NOT renumber.** Tail stays ADR-0290; next-free ADR-0291. §Decision records the landed mechanism (the NARROW sentinel siting + dual-`%w`, `provider.go` untouched, the `config.go` fallback + empty-both routing, the fixture's untrusted-arm upper-bound with the RD3-corrected forced-send rationale, the new `sdsserver` Option); §Consequences records the counts, the residual S1-only stream-posture divergence (option (b)), and the memory updates.
- [ ] **ROADMAP row 68 → `done`** at the six-gate (ADR-0106, SOLE leg; `reference_roadmap_split_phase_row_done`). **Deferred sentence UNTOUCHED** (SPEC §12 — do NOT fabricate a narrow).
- [ ] **STATE.md:** edit §Current pointer IN PLACE; demote to §Recent lineage capped at five; update counts (fixtures 113).
- [ ] **PROGRESS.md:** finalize — every break's ACTUAL firing assertion (including Break G step 3's expected non-fire), the verbatim red-first records, the fuzz dispatch-verification, any break substitutions.
- [ ] **Router roll** (`next-prompt.txt` — TRACKED despite .gitignore; edit in the stage worktree; locate by SUBJECT).
- [ ] **Sentinel re-run MECHANICALLY:** check (1) goes silent when row 68 flips; (2) still prints 3 via the full-phrase command; (3) unchanged ⇒ does NOT fire; no `stop` file.
- [ ] **Memory updates owed (SPEC §13 + this PLAN's):** (i) the CVC/empty-dynamic lesson — the reference distinguishes ACK (specifier-unset → merge → fall back) from NACK (set-but-empty / corrupt → reject) SHARPLY; a NARROW classifier is what the reference does, not merely the safer choice; a test helper's served-shape capability can quietly refute an "UNTOUCHED" envelope claim (re-derive the helper's actual output before pinning it). (ii) **NEW from this PLAN (RD3, empirically settled) — extend `reference_go_client_cert_withholding`:** at `require_client_certificate: true` a forced-send untrusted arm is **NOT observably load-bearing** — a bare forced→polite regression does NOT change the observable (both send-modes → `untrusted=rejected`: forced verify-fails, polite withholds→no-cert-rejected). The intuition that forced-send "upper-bounds the pool" is FALSE: at `RequireAndVerifyClientCert` the verification pool IS the advertised set, so any pool permissive enough to accept the foreign cert also advertises its CA, and the polite client then sends it too — the union hazard is caught in BOTH modes. Forced-send is retained for MEANING (keeps the arm exercising verify-and-reject vs collapsing into a no-cert duplicate of the `none` arm — a loss invisible to `structuralCheck`) and cross-side symmetry, NOT for observable coverage. The require=FALSE "polite goes vacuous-green" physics (0110) does NOT transfer to require=TRUE; there is NO observable-flipping break that isolates forced-send at require=true.
- [ ] **Squash-push by the controller** at stage-close.

**Commit (stage-close docs):** `phase 68 (tls-cvc-empty-dynamic-fallback) IMPL: …` (controller composes at close).

---

## Self-review against SPEC-68

| SPEC obligation | Where |
|---|---|
| NARROW `ErrEmptyValidationContext` scoped to specifier-unset (S1); S2/S3/empty-resources → errValidation-only (§3.2) | T1 (Steps 3/4, Breaks A/B) |
| dual-`%w` preserves BOTH sentinels; `provider.go` untouched (§3.2, §3.8, RD-DUAL/RD-PROV) | T1 Step 4, T8 envelope audit |
| the fallback via `dvc`/`loadTrustedCAPool`/`installPool`; nil-pool guard byte-intact (§3.1, §3.3) | T2 Step 3 |
| empty-both → phase-67 no-anchor routing, no new reject (§3.6, D-EDF-EMPTYBOTH) | T2 Step 3, test 2 |
| E1 covers PGV — verify byte-identical, add nothing (§3.5, C4) | T2 Step 6, T8 |
| P5 block byte-INTACT in place (§3.1 — RD-P5, no hoist) | T2 Step 3 RD-P5 pin |
| the `:185-193` DEPARTURE comment rewritten cause-scoped (§10, §11) | T2 Step 3 |
| the new `sdsserver.WithEmptyValidationContext` Option (§8, C3) | T3 |
| fuzz: new dispatch side/provider + seed, dispatch-verified, +0 fuzzers (§7) | T4 |
| fixture 0111 CVC-primary, three arms, forced-send, port 10447 (§8) | T5 |
| untrusted-arm upper-bounds the fallback pool; forced-send retained for meaning/symmetry not observable (§8 — RD3 corrected) | T6 Break G |
| B1–B4 pinned wording (§9) | T7 |
| ADR-0290 completed IN PLACE, no new ADR (§14) | T9 |
| six-gate + cycle guard + counts + envelope audit (§15) | T8 |
| Sentinel: nothing owed (§12) | header, T9 |
| Memory updates (§13 + RD3) | T9 |

**Task count: 9** — inside the SPEC's ~8-9 anticipation. **ADR-0045 escape valve ARMABLE, UNCONSUMED — no split**: no two-package surface can strand a leg. T1→T2 sequential (T2 consumes T1's sentinel); T3/T4 after T2; T5→T6 sequential on the fixture; T7 after T6; T8/T9 close.

**⚠️ The IMPL's standing instruction: a PLAN is not evidence either.** **RE-DERIVE this document; do not execute it.** Where it cites, go look; where it claims control flow, walk the call graph; default to REFUTED. Start where this PLAN is most confident (all EXECUTION-verified at the PLAN, §1.2): RD-DUAL/RD-PROV (the error-chain trace), RD-SENT (the `dataSourceBytes` walk), the S1/S2/S3 classification, and RD3 as CORRECTED (the require=true forced-send physics — the union-pool advertising result is empirically settled).
