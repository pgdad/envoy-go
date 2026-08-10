# SPEC 86 — validate-sds-nil-provider

**Stage:** SPEC (lifecycle-state 1 -> 2). **Date:** 2026-08-10.
**Base master:** `11f5a92cc65edea0b591d5ddf2999230a10c8476` (from `git rev-parse master`), branch `phase-86-spec`.
**Method:** the BRAINSTORM-86 named departure CONTINUES — no investigation agents; every probe run INLINE by the controller. Probes: the tip binary built with `-o` into session scratch; ELEVEN probe configs (4 arm/control + 7 negative shapes, ports 47100-47103 as TEMPLATE VALUES ONLY — validate binds nothing and every negative normal-mode boot fails before bind); ELEVEN pinned-reference `--mode validate` container runs (`s86-*`, `--rm`, all gone at close); and **a COMPILING, TEST-GREEN PROTOTYPE of the chosen mechanism in a DETACHED worktree** (`wt-86-proto` at `11f5a92c`, DELETED at close — `git worktree list` shows only `master` + this stage's worktree). The prototype diff is preserved in session scratch; every figure below cites it. Zero repo-tree edits by any probe.

---

## 1. Q1 DECIDED — THE FIX MECHANISM IS THE NO-FETCH SENTINEL PROVIDER (option a), AND IT WAS BUILT AND MEASURED, NOT ESTIMATED

### 1.1 The decision (D-86-MECH)

**A validate-only sentinel `SecretProvider` in `internal/xds`, threaded by a new `boot.NewValidateSDSProvider` that REUSES the entire boot pre-scan code path, with three fetch-site skips in `internal/tls`.** The discriminator is `xds.IsNoFetch(provider)` — a type-assertion on the sentinel — **never `provider == nil`** (the hard invariant from BRAINSTORM §1.2: the nil-reject has other consumers that keep rejecting BYTE-IDENTICAL).

The four production edit sites, all built and compile-verified in the prototype:

1. **`internal/xds/provider_nofetch.go` (NEW, 37 lines in the prototype):** unexported `noFetchProvider` implementing `SecretProvider` with defense-in-depth erroring methods (never called — every fetch site skips first), exported `NoFetchProvider()` ctor and `IsNoFetch(p)` discriminator.
2. **`internal/boot/boot.go` (+30/-7):** `NewSDSProvider`'s body becomes unexported `newSDSProviderAndClient(...) (xds.SecretProvider, *grpcclient.SDSClient, error)`; `NewSDSProvider` wraps it (client stays open — the boot path needs it); NEW `NewValidateSDSProvider` calls the SAME function — so every pre-scan arm is inherited BY CONSTRUCTION, present and future — then **`client.Close()`** on the never-dialed conn (D-86-CONN: `validate` is a LIBRARY consumed by long-running Gateway controllers; leaking one lazy `*grpc.ClientConn` per reconcile call is not acceptable hygiene) and returns `xds.NoFetchProvider()`. `(nil, nil)` at seen==0 exactly like the boot path, so a no-SDS bootstrap validates with a nil provider and every non-SDS reject stays byte-identical.
3. **`internal/tls/config.go` (+25/-5):** THREE skip sites plus one interplay fix —
   - arm A (`commonTLSContextToConfig`, the `FetchInitialCertificate` site at `:390`): `IsNoFetch` ⇒ skip the fetch, set a new `sdsCertPromised := true`;
   - ⚠️ **the `:518` interplay (D-86-NOFETCH-CERT), UNENUMERATED AT THE BRAINSTORM and found only because the prototype was EXECUTED:** with the fetch skipped, an SDS-only-cert listener reaches `if side == "downstream" && len(cfg.Certificates) == 0` and validate would WRONGLY reject `no tls_certificates configured` — a config boot accepts. The check gains `&& !sdsCertPromised`. This is the ninth-lineage under-enumeration specimen, caught at SPEC rather than IMPL;
   - VC-SDS arm (`NewDownstreamConfig`): AFTER `xds.ParseSDSConfig` runs (structural validation preserved), `IsNoFetch` ⇒ return without fetch/installPool;
   - CVC arm: same shape; the E1/E2 presence checks in `commonTLSContextToConfig` already ran.
4. **`validate/validate.go` (+10/-2):** replace the literal nil with `boot.NewValidateSDSProvider(dialer, bs, baseDir, bs.Stats)`; propagate its error (these ARE the boot-parity rejects); thread the result into `boot.Construct`.

### 1.2 Why the alternatives lose — MEASURED, not argued

- **(b) threaded mode flag: measured DEAD.** The chain `Construct -> NewManagerWithBaseDirAndAllowH2C -> buildListenerRuntimeWithCtx -> NewDownstreamConfig -> commonTLSContextToConfig` has, at this tip (mechanical `grep -rn ... | wc -l`, tests included since every signature change breaks them): `boot.Construct` **2** call sites · `NewManagerWithBaseDirAndAllowH2C` **36** · `buildListenerRuntimeWithCtx` **2** · `NewDownstreamConfig` **52** · `commonTLSContextToConfig` **16** — **~108 call sites** against option (a)'s FOUR files. It also spells "validate" into five signatures forever.
- **(c) placeholder-material provider: REJECTED on fabricated state, with the specific harm named.** A fabricated leaf would be appended to `cfg.Certificates` and an EMPTY `*x509.CertPool` would pass `installPool` and set `ClientAuth` — the validator would assert an anchor it never validated, and `configuration OK` would rest partly on invented material. Line count is comparable to (a), so there is no cost argument for it either.

### 1.3 The prototype's execution record (the row's acceptance envelope, demonstrated)

Built at `11f5a92c`; all ELEVEN shapes run against it under `--mode validate`:

| shape | tip binary (pre-fix) | prototype | boot (normal mode) |
|---|---|---|---|
| control (static TLS) | `configuration OK` 0 | `configuration OK` 0 | accepts |
| arm A `tls_certificate_sds_secret_configs` | exit 1 arm-A reject | **`configuration OK` 0** | accepts (fetches) |
| arm B `validation_context_sds_secret_config` | exit 1 phase-03 reject | **`configuration OK` 0** | accepts (fetches) |
| arm C `combined_validation_context` | exit 1 phase-03 reject | **`configuration OK` 0** | accepts (fetches) |
| n1 node absent | exit 1 **arm-A reject (masked)** | exit 1 `xds: sds: node.id and node.cluster are required for SDS` | same string, 7 ms |
| n2 two SDS positions | exit 1 masked | exit 1 `xds: sds: multiple SDS-bound downstream TLS contexts unsupported (MVP takes one)` | same string, 5 ms |
| n3 `DELTA_GRPC` | exit 1 (already the right string — `ParseSDSConfig` runs before the arm-A reject) | exit 1 same string | same string, 6 ms |
| n4 unknown SDS cluster | exit 1 masked | exit 1 `xds: sds: dial cluster "missing_cluster": grpcclient: dial "missing_cluster": unknown cluster` | same string, 5 ms |
| n5 live shape, dead SDS endpoint | exit 1 masked | **`configuration OK` 0** | exit 1 at FETCH (9 ms — refused fast; the class is I/O-dependent) |
| n6 secret name `bad/name` | exit 1 masked | exit 1 the `invalid secret name` string | same string, 6 ms |
| n7 SDS cluster without `http2_protocol_options` | exit 1 masked | exit 1 `...cluster does not have http2_protocol_options{} set (gRPC requires HTTP/2 framing)` | same string, 9 ms |

**Boot-parity strings come out BYTE-IDENTICAL by code-path reuse, not by replication.** And guard preservation was proven by execution, not inspection: `go test -count=1` on `internal/tls`, `internal/boot`, `internal/xds/...`, `internal/listener`, `validate`, `cmd/envoy-go` — **all ok on the prototype**, including the existing nil-reject pins at `config_test.go:921` (arm-A string), `:1198` (VC-SDS phase-03 string), `:1310` (`cvcRetainedReject`). `gofmt -l` empty on the four packages.

---

## 2. Q2 DISPOSED — THE BOOT-PARITY SURFACE, BY EXECUTION, IS SIX BUILD-TIME ARMS PLUS ONE FETCH-TIME CLASS

All seven negative shapes booted in NORMAL mode at this tip (§1.3 rightmost column). The discriminator between "belongs to validate" and "does not" is **MECHANISM, not latency**: everything up to and including `grpcclient.DialContext`'s synchronous checks is I/O-free (`grpc.NewClient` dials lazily — `grpcclient.go:108-129` states it and n4/n7's 5-9 ms confirm it); only `FetchInitial*` performs I/O.

- **Build-time (validate MUST replicate — and does, via `newSDSProviderAndClient` reuse):** n1 node requirement · n2 one-secret cap · n3 the `ParseSDSConfig` arms · n6 `validateSDSSecretName` · **n4 cluster-existence** and **n7 cluster-H2-options** — ⚠️ these last two live in `grpcclient.DialContext` (`d.mgr.Get` + `clu.UseH2()`), are SYNCHRONOUS and I/O-free, and **n7 was ABSENT from the BRAINSTORM's enumeration** ("node arm-7, one-secret cap, ParseSDSConfig arms") — found only by executing the surface. Sub-finding: n5's "dial cluster" error-string prefix is misleading — n4/n7 fire in it WITHOUT any dial.
- **Fetch-time (validate MUST NOT replicate — D-86-N5):** n5 — structurally valid config whose SDS endpoint is dead. Boot rejects (classified, bounded by `initial_fetch_timeout`); validate ACCEPTS (`configuration OK` on the prototype). **This is the row's contract stated precisely: validate accepts iff boot accepts, MODULO exactly the rejects that require I/O to discover.** The reference behaves the same way (its validate accepted n5, §3).
- The BRAINSTORM's open sub-question "do cluster-name resolution failures surface at build or dial?" is **ANSWERED: build** (n4, 5 ms, no server anywhere).

## 3. Q3 DISPOSED — THE REFERENCE RECORD, RE-VERIFIED PER-SIDE (observations, never expectations)

Pinned `envoyproxy/envoy:contrib-v1.37.2`, `--mode validate`, one fresh `--rm` container per shape (`s86-*`, none left at close):

| shape | reference result |
|---|---|
| control, arm A, arm B, arm C | **`configuration OK`, exit 0 — all four.** BRAINSTORM-83 §5.6's carried "the reference validates OK" is now EXECUTED, three-armed, no longer carried. |
| n1 node absent | **exit 1** — `TlsCertificateSdsApi: node 'id' and 'cluster' are required. Set it either in 'node' config or via --service-node and --service-cluster options.` ⚠️ **The reference enforces the node requirement IN VALIDATE MODE — independent corroboration that a validator can and does run SDS structural checks without dialing.** |
| n2, n3, n4, n5, n6, n7 | **exit 0, all six.** Reference-accepts. |

Per-side reading: n2/n3/n6 are the PRE-EXISTING, documented envoy-go-strict departures of the boot path (`ParseSDSConfig`'s doc comment says exactly this; the MVP cap and name-charset are recorded at ADR-0280/phase-80). n4/n7 are the `grpcclient` strictness arms, likewise boot-path behavior. **Validate inheriting all six is precisely the parity contract — the divergences are recorded ONCE, on the boot path, and validate mirrors boot, not the reference.** n5: both sides' validate accept (neither dials). NO new departure is minted by this row.

## 4. Q4 DISPOSED — THE CONTRACT SWEEP, ENUMERATED

**The load-bearing find: `BEHAVIOR_CONTRACT.md:1062` records the OPPOSITE decision as PERMANENT** — *"The `validate` path diverges, and stays that way — a recorded decision, `validate/` untouched"* (phase 66's §7). Its rationale — *"plumbing SDS into a config-validator implies dialing a management server from `validate` mode"* — is **REFUTED BY EXECUTION** (§1.3: the prototype validates all three arms with zero I/O; §3: the reference's own validate runs the node check). **ADR-0308 REVERSES that recorded decision ON THE RECORD** — the mandated contract-edit vehicle per the ADR-0052 discipline; ADR-0287 itself is append-only untouched (the sentence lives only in the contract).

The full reconcile-vs-record census for the IMPL:

| statement | disposition |
|---|---|
| `BEHAVIOR_CONTRACT.md:1062` "diverges, and stays that way" | **RECONCILE (reverse)** — rides ADR-0308 |
| `:1050` "THREE production consumers" of the CVC reject (names `validate.Bootstrap`) | **RECONCILE** — validate leaves the consumer set; count becomes two production consumers + two test-only ctors |
| `:1034` "It still rejects … for … `validate.Bootstrap`" | **RECONCILE** — validate leaves the roster |
| `:948-958` (phase-51 validate section) | **RECONCILE (extend)** — gains the SDS-acceptance + boot-parity paragraph; the phase-51 "No change to WHAT is validated" sentence is that phase's history and STAYS |
| ADR-0280 `:16708` tail: "`validate.Construct` passes a literal `nil` (validate never dials/fetches — no SDS config exists in validate fixtures)" | **RECORD-only** — append-only; ADR-0308 §Context reconciles it: the no-dial HALF survives, the literal-nil half is retired |
| phase-60.2 Task 5 / `validate.go:48` comment "validate does not dial/fetch SDS" | **SURVIVES, restated** — the decision was no-DIAL; the reject was its over-broad implementation (BRAINSTORM §1.2, now demonstrated) |
| ADR-0268 `:16434`/`:16441` "an RTDS/SDS validate companion" (Op-tooling window) | **UNTOUCHED** — ADJACENT-but-DISTINCT per BRAINSTORM §2 (feature vs repair); nothing narrows |
| production comments `internal/tls/config.go:344-346` ("nil for upstream/validate callers"), `:407-435` + `:447-452` consumer enumerations (both name `validate.Bootstrap` as a nil consumer), `listener/manager.go:569-573` | **IMPL edit sites** — the consumer rosters change; these are production comment lines and are IN the cost envelope (§8) |
| `BEHAVIOR_CONTRACT.md:1058` (E2 position-scoped reachability) | **UNCHANGED** — validate now runs the SAME pre-scan, so E2's boundary statement covers both paths as written |

## 5. Q5 DISPOSED — TEST PLACEMENT AND THE RED ANCHORS

- **RED anchors (all three re-reproduced at THIS tip, §1.3 column 2):** the three arm rejects under `--mode validate`. The negative shapes n1/n2/n4/n6/n7 are ALSO red anchors in a second sense: today they emit the MASKING arm-A string; post-fix they must emit the boot string.
- **`internal/tls/config_test.go`:** three per-arm ACCEPT tests (`NoFetchProvider` threaded, no fetch, no reject) + the D-86-NOFETCH-CERT arm (SDS-only cert + sentinel ⇒ NO `no tls_certificates configured`) + guard-preservation NCs beyond the existing pins (nil provider on each arm still yields the byte-identical string — pins `:921`/`:1198`/`:1310` STAY UNTOUCHED and green, proven on the prototype).
- **`internal/boot` tests:** `NewValidateSDSProvider` — the six build-time negatives (n1/n2/n3/n4/n6/n7 shapes as protos), seen==0 ⇒ `(nil, nil)`, arm-positive ⇒ `IsNoFetch` true.
- **`validate/validate_test.go` (432 lines, zero SDS today):** end-to-end — three arm accepts, the n5-class accept, the negatives' error-substring arms.
- **`cmd/envoy-go/main_test.go` (1502 lines, zero SDS today):** CLI subprocess arms — `--mode validate` exit 0 + `configuration OK` on arm shapes; exit 1 + boot-string on a negative.
- **QUIC nil-reject pins already exist and are the guard-preservation anchors** (measured: `config_test.go:921/:1198/:1310`; `TestNewQUICDownstreamConfig_*` at `:1126-1176`). **No differential fixture** (validate has no wire surface — phase-51 precedent, BRAINSTORM §3.2).

## 6. Q6 DISPOSED — THE ERROR-STRING CONSTRAINT SET

**Zero landed reject strings change.** The three lifted-arm strings keep firing byte-identically for every remaining `provider == nil` consumer (QUIC — including a QUIC-wrapped SDS-cert shape, whose path hardcodes nil regardless of mode; the exported test-only constructors; main.go's seen==0 pure-inline-CVC case) — enforced by the untouched pins and by the discriminator being `IsNoFetch`, not nil. The boot-parity strings validate now emits are the boot path's OWN strings by code-path reuse (§1.3 confirmed byte-identical). The ONLY new strings are the two defense-in-depth messages inside the sentinel's never-called fetch methods (`xds: sds: internal: no-fetch (validate-mode) provider asked to fetch …`) — unreachable today, ADR-0080-distinct if ever seen.

## 7. Q7 DISPOSED — ANTICIPATED COUNTS, EACH RE-CHECKED AT THIS TIP (post-change where it matters)

- **fixtures +0** (no differential surface) · **BackendKind +0** · **fuzzers 55 / 48 files** (re-counted `^func Fuzz` ON THE PROTOTYPE TREE — no new untrusted-parse boundary; the sentinel consumes no raw bytes).
- **go.mod +0** — `go mod tidy -diff` NO CHANGE on the prototype tree.
- **package edges +0** — `go list -deps ./validate` ALREADY carries `internal/xds` AND `internal/xds/xdsgrpc` (BRAINSTORM §5.2's carried claim now VERIFIED, post-change).
- **stat surface delta 0** — same-command census on base AND prototype: **145 call-site lines / 21 files both sides** (`NewCounter(|NewGauge(` over `internal/ cmd/ validate/`, tests excluded). ⚠️ The carried 208/36 figure uses a DIFFERENT counting form (ARM-1 of the 84-era gate); no absolute is corrected here — **only the delta is asserted, and it is 0** (`NewValidateSDSProvider` reuses the existing `RegisterSDSStats` call site on validate's throwaway registry; zero new registration sites).

## 8. COST — ENUMERATED BY PROTOTYPE (the lineage's demand), AND THE BRAINSTORM FLOOR IS ALREADY REFUTED AS CENTRAL

Prototype measured (`git diff --numstat` + new file, compiling, all tests green): `provider_nofetch.go` **+37 (new)** · `boot.go` **+30/-7** · `tls/config.go` **+25/-5** · `validate.go` **+10/-2** ⇒ **gross +102/-14, NET +88 production `.go`** — at the very top of the BRAINSTORM's ~50-90 floor with ZERO of the house comment premium and ZERO of the §4 consumer-comment rewrites (`config.go:344-346/:407-435/:447-452`, `manager.go:569-573` — those comments' consumer rosters are made FALSE by this row and MUST change; est. +15-30 comment lines). **IMPL production budget: ~110-160 net `.go`.** The BRAINSTORM's ~50-90 is hereby REFUTED as a central estimate — the **NINTH consecutive `reference_measured_prototype_is_a_lower_bound` firing**, cause again under-ENUMERATION (n7, the `:518` interplay, the comment rewrites, `client.Close()` plumbing — all absent from the BRAINSTORM enumeration, all found by execution).

Tests enumerated per §5: tls accepts+interplay+NCs ~100-160 · boot negatives ~120-200 · validate end-to-end ~120-200 · main_test CLI ~60-120 ⇒ **test budget ~400-680 `.go`** (BRAINSTORM's ~150-300 REFUTED as central, same cause). Docs: ADR-0308 completion + 4 contract paragraphs + pins-style per-side record. **3-4 IMPL tasks, ONE leg, no split** (D-86-SEQ mirror of D-85-SEQ: the lift, the guards, and the guard-preservation tests are ONE atomic commit — a lift whose guards land later is the `reference_lifted_reject_hidden_enforcement` shape this row exists to avoid).

## 9. DECISIONS LEDGER

- **D-86-MECH** — no-fetch sentinel provider; discriminator `xds.IsNoFetch`, never nil (§1).
- **D-86-PARITY** — boot-parity by CODE-PATH REUSE (`newSDSProviderAndClient`), not replication; future pre-scan arms inherited by construction (§1.1.2).
- **D-86-NOFETCH-CERT** — the `:518` `sdsCertPromised` interplay (§1.1.3).
- **D-86-CONN** — the never-dialed client is CLOSED in `NewValidateSDSProvider` (§1.1.2).
- **D-86-N5** — the modulo-fetch contract stated precisely: only I/O-requiring rejects are exempt (§2).
- **D-86-REF** — reference results recorded per-side, observations never expectations; no new departure minted (§3).
- **D-86-SEQ** — ONE IMPL leg, one atomic commit (§8).

## 10. REFUTATION LEDGER — WHAT THIS STAGE MOVED

1. **BRAINSTORM §5.2's three carried claims all discharged BY EXECUTION:** reference validates-OK (now three-armed, plus seven negative-shape observations); the boot-side discriminator (re-executed as the n-series normal-mode boots); `go list -deps` (verified post-change).
2. **The BRAINSTORM's boot-parity enumeration was INCOMPLETE** — n7 (cluster-H2-options) found by execution; n4's build-time classification settled (§2).
3. **The BRAINSTORM cost floor REFUTED as central** — ninth lower-bound firing, figures in §8.
4. **`BEHAVIOR_CONTRACT.md:1062`'s rationale REFUTED by execution** — no dial is implied by SDS-aware validation; the reference's own validate proves it too (§3, §4).
5. **The n3 nuance:** arm-A structural errors ALREADY had parity pre-fix (`ParseSDSConfig` runs before the arm-A reject) — the masking defect is arms n1/n2/n4/n6/n7 plus the three accept arms.

## 11. SENTINEL — RE-RUN MECHANICALLY AT THIS STAGE. IT DOES **NOT** FIRE

Input measured **236 lines / 118 data rows** first. **(1)** `NOT DONE: row 86` at `want=118` — the single expected line, denominator assertion silent · **(2)** SIX — `:196 :202 :208 :218 :224 :232` · **(3)** SILENT ⇒ conjunction fails, **no fire**; `stop` NOT created (verified at repo root and worktree). All FOUR NCs fired: row-62 doctoring ⇒ `NOT DONE: row 62` AND `row 86` (with `NC LANDED? [ in-progress ]` inspected first) · `want=117` ⇒ `GATE FAIL: examined 118 data rows, expected 117` · check-(3) doctoring (residual 2 -> 0 confirmed on the doctored copy first) ⇒ `NEVER OPENED: gRPC`, WASM silent · check-(2) one-arm **5**/**1**, union **6**. `ROADMAP.md` BYTE-UNTOUCHED by this stage (empty diff verified at close); leak axes invariant: `-family row` **95/67** · `gRPC-family row` **2** · `Operational-tooling-family row` **3**.

## 12. HYGIENE

Binary + probe configs + PKI in session scratch only. Reference containers `s86-*` all `--rm`, verified gone. The prototype worktree `wt-86-proto` (DETACHED at `11f5a92c`) was DELETED at close after the diff was captured to scratch; `git worktree list` shows only the repo root and `wt-phase-86-spec`. This stage lands **ZERO production `.go`, ZERO test `.go`** — docs only (`SPEC.md`, `PROGRESS.md`, the ADR-0308 §Context append, `STATE.md`/`STATE_HISTORY.md`, `next-prompt.txt`). Ports 47100-47103 were template values; nothing bound. `go.mod`/`go.sum` untouched.

## 13. NEXT

**PLAN.** It owes: the task decomposition of §1's four edit sites + §5's test placement under D-86-SEQ (one leg); the TDD order (RED anchors from §1.3 re-proven at the PLAN/IMPL tip); the guard-preservation NC roster (pins stay untouched AND new NCs); the §4 contract-edit text riding ADR-0308; the §8 budget carried as a FLOOR with overrun recordable; re-derivation of every count at its own tip.
