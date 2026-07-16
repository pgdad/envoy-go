# PLAN 66 — xDS SDS `combined_validation_context` Implementation Plan

> **Stage:** PLAN (lifecycle-state 2 → 3). Docs-only — ZERO production `.go`. Fresh worktree off master `2c5802ee`, branch `phase-66-plan`, worktree `.worktrees/phase-66-plan`, per `feedback_git_worktrees`.
>
> **Row 66 STAYS `in-progress`** — the IMPL flips it `done` at its six-gate (ADR-0106). **ADR-0287's §Context is ALREADY DRAFTED** at the SPEC (ADR-0044); the IMPL **COMPLETES ADR-0287 IN PLACE** with §Decision/§Consequences — it does **NOT** append a new ADR (see C4).
>
> **Baselines RE-VERIFIED against master `2c5802ee` this session (`git fetch` first; NOT copied):** fixtures **110** (numeric tail `0108-xds-sds-validation-context`) · fuzzers **55** · **DECISIONS tail ADR-0288** (next-free **ADR-0289**) — *not* ADR-0286/0287 as SPEC §1/§15 state (C4) · BackendKind tail **38** · stat surface **1201** · go.mod modules **2** (repo total **67**).
>
> **Sentinel re-run MECHANICALLY — does NOT fire; `stop` NOT created.** (1) prints `NOT DONE: row 66`; (2) prints **3**; (3) prints `NEVER OPENED: gRPC, Runtime, WASM`.
>
> **⚠️ Check (2)'s exact command matters, and this PLAN's first draft cited it unverifiably (F9).** It said "three live `candidates:` sentences" — but bare `grep -c 'candidates:' docs/envoy-go/ROADMAP.md` returns **5** (and 32 repo-wide), because HISTORICAL recaps use `candidates were:` and other prose contains the word (`reference_sentinel_deferred_sentence_live_vs_historical`). **The check is the router's full-phrase form, and only it:**
> ```sh
> grep -cE 'remaining deferred \(not-yet-chartered\) candidates:' docs/envoy-go/ROADMAP.md   # => 3
> ```
> **Cite the command, never the adjective.** A count with no reproducible command is the same defect class as a citation with no source.
>
> **⚠️ THIS PLAN CORRECTS SIX SPEC-66 DEFECTS — ONE SEVERE AND SECURITY-RELEVANT (C1).** The SPEC self-recorded four of its own (§1.2) and observed that *"its errors all live in the sections where it is most confident it has already caught the error."* **That pattern held again.** C1 lives inside §3.10 — the section whose stated purpose was *"a silence is not a scope decision"* — and it **is** a silence: the row's headline scope is **unenforceable at the apply-point the SPEC chose**, and the row as specified would convert today's loud boot-FAIL into a **silently unauthenticated listener**. C2 lives inside §6 — *"the two arms the BRAINSTORM never named"* — named, but never **ordered**, and every order breaks something. C1b lives inside §5.1's *"out of scope by construction"* — a silence about two envelope fields.
>
> **⚠️ AND THIS PLAN'S OWN FIRST DRAFT CARRIED NINE DEFECTS, FOUND BY AN ADVERSARIAL PASS — ALL RECORDED IN PLACE, NONE HIDDEN.** Its headline claims (C1, C2/D2, C3–C6) **survived** and several were **confirmed by execution**. But four would have cost the IMPL real time, and **three of them are this PLAN committing the exact failure it was written to prevent**:
> - **F4** — D5's apply-point skipped `xds.ParseSDSConfig`, so a **malformed `sds_config` would be silently accepted** where the phase-65 sibling rejects it: `reference_strict_reject_sibling_typeurl_gap`, **in the fix for a strict-reject gap**. It also broke the `tls: ` fuzz invariant and dropped a nil guard the sibling deliberately keeps.
> - **F2** — T2's RED-first protocol specified a **VACUOUS red** (the unconditional reject fires first against unmodified source): `reference_deliberate_break_wrong_assertion`, **inside the task whose sole purpose is proving four assertions live**.
> - **F5** — the task order would have left the tree **transiently silently-auth-bypassed between the T1 and T3 commits, with T1's own green gate certifying it** — C1's defect, re-committed by C1's own fix.
> - **F3** — T1's first test was **impossible as specified** (it would have hit an unrelated error).
>
> **The lesson this PLAN adds: knowing a failure mode by name does not prevent committing it.** Every one of the above was made by an author who had just written the memory's name into the document. **Adversarial re-derivation is not a formality at the end; it is the only thing that caught them.**

---

## Global Constraints

- **ONE stage per session.** This session: the PLAN only. No production `.go`. After it lands: roll to the phase-66 IMPL.
- **Design A (pool substitution) is ADOPTED and its theorem is proven** (SPEC §3.2, re-derived and attacked again this session — C6). **The IMPL must NOT re-open the seam.**
- **NO `proto.Merge`. NO `proto.Clone`. Anywhere.** The BRAINSTORM's §6 still instructs them; **they do not compile** (SPEC §3.1 — `FetchInitialValidationContext` returns `*x509.CertPool`; the message never crosses the seam). **The BRAINSTORM is authoritative for the SUBJECT and ENVELOPE only.**
- **`internal/xds` is UNTOUCHED** — the row's defining property, preserved by the theorem (D5).
- **Counts at the IMPL:** fixtures **110 → 111** · fuzzers **55 (+0)** (seeds only) · stat surface **1201 (+0)** · BackendKind **38 (+0)** · go.mod **2 (+0)** · ZERO new packages.
- **Per-task hygiene** (`feedback_pertask_gofmt_lint`): `gofmt -l` + `go vet` + `golangci-lint run` on every touched package — not just `go vet`.
- **Worktree discipline** (`feedback_git_worktrees`, `feedback_subagent_worktree_detach`, `feedback_subagent_worktree_path_targeting`): pin the canonical root; worktree-relative paths; controller verifies the MAIN checkout stays clean; on a deliberate break restore with **`git restore` only** — no checkout-sha, no `--amend`, no detached HEAD.
- **Subagents commit locally; the controller squash-pushes at stage-close** (`feedback_subagents_no_push`, `feedback_push_to_origin`).
- **`reference_sds_init_fetch_timeout_dial_budget_flake`** — if `TestProvider_FetchInitialCertificate_Timeout` fails under `-race`, it is **PRE-EXISTING on master** (observed once, 2026-07-16). Do **NOT** reflex-classify it as a phase-66 regression. A SECOND occurrence justifies widening the budget.

### Break protocol (binding on every task)

- **A break must COMPILE** (`reference_plan_break_instructions_dont_compile` — phase 65 hit 2 non-compiling + 2 vacuous). A non-compiling break shows red that proves **nothing**. If an instruction below does not compile: **substitute a compiling equivalent, REPORT the substitution, record the TRUE result.**
- **A break must DISCRIMINATE** (`reference_probe_must_discriminate` — the SPEC shipped a vacuous one-input probe *inside its own pin block*). Before recording a break as proof, ask: **"what would the OTHER hypothesis have printed?"** If the same thing, it proved nothing.
- **`-count=1` on EVERY break** (`reference_differential_break_protocol_count1`).
- **Confirm WHICH assertion fired** (`reference_deliberate_break_wrong_assertion`) — a break failing ≠ your assertion is live; it can abort earlier and MASK the intended one.
- **A break that does NOT fire is a FINDING** — record it as an honest, UNCLAIMED coverage gap in PROGRESS. Do **not** route around it.
- **`-run 'TestDifferential/0109-xds-sds-combined-validation-context'`** — never bare `0109` (`reference_differential_run_selector`).

---

## ⚠️ PLAN-time corrections to SPEC-66 (RE-DERIVED against master `2c5802ee`; `feedback_brief_citations_not_evidence`)

### C1 🔴 **SEVERE, SECURITY-RELEVANT — the "`require_client_certificate: true` ONLY" scope is UNENFORCEABLE at the SPEC's apply-point, and the row as specified CREATES a silently unauthenticated listener**

**Three legs, each re-derived from source this session:**

1. **`commonTLSContextToConfig` structurally CANNOT see `require_client_certificate`.** It takes a `*tlsv3.CommonTlsContext`; `GetRequireClientCertificate()` exists **only** on `DownstreamTlsContext` (verified in the generated type — exactly one such getter exists). The SPEC *states* this fact (§3.5, and the landed comment in the SDS-VC arm) and **never draws the consequence**.
2. **The CVC arm TODAY is an UNCONDITIONAL reject** — unlike the SDS-VC arm directly above it, it carries **no `provider` conjunct at all**:
   ```go
   case *tlsv3.CommonTlsContext_CombinedValidationContext:
       return nil, fmt.Errorf("tls: %s: combined_validation_context is not supported in phase 03", side)
   ```
   **⇒ CVC + `require_client_certificate: false` BOOT-FAILS LOUDLY TODAY.**
3. **`NewDownstreamConfig` calls `commonTLSContextToConfig` FIRST and propagates its error**, only then entering `if ctx.GetRequireClientCertificate().GetValue()`.

**The consequence the SPEC missed.** Post-lift as SPEC §11 edit-site 1 specifies (gate `side == "downstream" && provider != nil`), a CVC listener with `require_client_certificate` false/absent: the pre-scan's new third arm sets `seen == 1` ⇒ provider non-nil ⇒ **the arm no-ops** ⇒ back in `NewDownstreamConfig` the require block is **skipped** ⇒ `cfg` is returned with **`ClientCAs == nil` and `ClientAuth` at its zero value, `NoClientCert`** ⇒ **BOOT SUCCEEDS.**

> **An operator who writes `combined_validation_context` — unambiguously intending mTLS — but omits `require_client_certificate: true` gets, TODAY, a clear boot failure. Under the row as specified, they get a listener that accepts EVERY client with NO certificate verification, and says NOTHING.** The row would convert a loud failure into a silent security downgrade, for a shape it declares OUT OF SCOPE.

**Why the SPEC got it wrong.** §3.10 frames `require==false` as a **pre-existing** silent-ignore that the row "does not lift". That is **accurate for the fully-inline path**, and **accurate for the SDS-VC path phase 65 landed** — and **FALSE for CVC**, where envoy-go currently **rejects**. The SPEC reasoned by analogy from two siblings and never checked that the third differs. This is `reference_strict_reject_sibling_typeurl_gap` — the memory the SPEC invokes **twice** against the BRAINSTORM — occurring inside the SPEC's own adopted design, in §3.10, the section titled *"disposed EXPLICITLY… a silence is not a scope decision."*

**CORRECTION → `E3` (D1).** Add an **explicit reject** for CVC + `require_client_certificate` false/absent, in **`NewDownstreamConfig`** — the only scope where the field is visible. This makes the headline scope **TRUE**, preserves today's loud failure (**no regression**), and keeps the divergence from the reference (which honors verify-if-presented — PROBED at the SPEC) **loud and named** rather than silent. §1/§3.10/§9-item-4/§14/ROADMAP all assert the unenforceable scope verbatim and must be reconciled at the IMPL.

**Discovered, NOT fixed (scope) — TWO pre-existing siblings of C1's exact defect class:**

- **The SDS-VC path phase 65 landed carries the identical defect.** It converted an unconditional reject into a `provider`-gated no-op, so SDS-VC + `require==false` silently ignores the anchor today, where pre-65 it boot-failed. **ADR-0286 shipped it.**
- **`NewQUICDownstreamConfig` never evaluates `require_client_certificate` AT ALL** (F7 — re-derived). So QUIC + an inline `validation_context` + `require: true` **silently installs NO `ClientCAs` today**. Same class, same function E3 does not cover. **⚠️ It also bounds C1's narrative honestly: "today the operator gets a clear boot failure" is true off the QUIC path only.** *(No CVC hole — QUIC's nil provider makes the retained reject fire first, verified.)*

Both are **out of scope** (fixing either means touching landed arms and their tests); **recorded in §Deferred and in PROGRESS as pre-existing defects this row DISCOVERED.** Do not let the IMPL quietly "fix" them and widen the row.

### C2 🔴 **E1/E2's ORDER relative to the retained gate is unspecified — and every order breaks something. Both the SPEC *and* an adversarial verifier got the reachability wrong.**

SPEC §6 requires E1/E2 in `commonTLSContextToConfig` with ADR-0080-distinct substrings, and §11 edit-sites 1+2 put them in the same arm — but **never fixes their order** against the retained `side != "downstream" || provider == nil` gate.

- **E1/E2 BEFORE the gate** ⇒ a QUIC (or `validate`, or no-SDS) listener carrying a pure-inline CVC emits **E2's** message instead of the retained substring ⇒ **breaks the §1/§14/ADR-0080 promise that upstream + QUIC keep the BYTE-IDENTICAL `combined_validation_context is not supported in phase 03`.**
- **E1/E2 AFTER the gate** ⇒ they fire only for `downstream && provider != nil`.

**⇒ AFTER the gate is correct** (ADR-0080's byte-identical promise is load-bearing and explicit; E1/E2's message quality is not). **D2 adopts it.**

**But the reachability story is subtler than either prior document, and BOTH were wrong:**

- **The SPEC** justified E1/E2 as *"envoy-go would happily accept both malformed shapes the reference refuses."* **False for E2 in the single-listener case:** a pure-inline CVC ⇒ the pre-scan's third arm skips it (no inner SDS config) ⇒ `seen == 0` ⇒ `NewSDSProvider` returns a genuine nil ⇒ the gate **already rejects it**. E2 is not needed for correctness there — only for message quality.
- **An adversarial verifier** then concluded *"E2's substring never surfaces in production / E2 essentially is not reachable."* **Also false — it assumed ONE listener.** RE-DERIVED: `NewSDSProvider`'s pre-scan iterates **`bs.Proto.GetStaticResources().GetListeners()` — ALL listeners, and all filter chains incl. `default_filter_chain`**. The provider is **GLOBAL, not per-listener**. So:
  > **Listener A carries a well-formed CVC (SDS half present) ⇒ `seen == 1` ⇒ provider LIVE. Listener B carries a pure-inline CVC ⇒ B's CVC passes the `provider != nil` gate ⇒ E2 FIRES.** `seen` stays 1, so the `seen > 1` guard does not intercept.

  **E2 is reachable. Rarely, but really — and it is exactly the path where it matters**, because without it listener B would reach the CVC branch and call `FetchInitialValidationContext` with an **empty secret name** derived from a nil `SdsSecretConfig`.

**⇒ Both arms stay, both AFTER the gate, with reachability stated precisely (D2's table) rather than asserted.**

### C1b 🟡 **The CVC envelope has FOUR fields; E1/E2 guard TWO and nothing names the other two** *(found by the adversarial pass, on this PLAN's own work)*

`CommonTlsContext_CombinedCertificateValidationContext` has **four** fields (enumerated by reflection): f1 `default_validation_context`, f2 `validation_context_sds_secret_config`, **f3 `validation_context_certificate_provider`**, **f4 `validation_context_certificate_provider_instance`** — the last two proto-`[#not-implemented-hide:]` **and** `deprecated`.

SPEC §5.1 dismissed f3/f4 as *"out of scope **by construction**"*. **That is a silence, and this row's whole C1 lesson is that a silence is not a scope decision.** E1/E2 cover f1/f2; D3's selector operates on the *default*, not the envelope; **nothing touches f3/f4.** A PGV-**valid** CVC carrying f1+f2+**f4** is **silently accepted** by envoy-go, which ignores f4 and uses SDS — `reference_strict_reject_sibling_typeurl_gap` on this PLAN's own envelope. *(Narrow: a CVC with **only** f3/f4 is caught by E1.)*

**DISPOSITION — name it as an explicit UNasserted boundary (T7 item 9); do NOT add an E4 reject.** Reason: the natural justification for a reject is *"the reference refuses deprecated fields by default"* — and **that is UNPROBED.** Adding a reject on an unprobed premise about reference behavior is precisely the move that produced the SPEC's D-COMBVC-PUREINLINE defect (*"legal per the proto — verified"*, which was false). **The boundary is named, the reference behavior is recorded as UNKNOWN, and §Deferred carries the probe obligation.** A future roller probes first, then decides.

### C3 🟡 **THREE live consumers of the nil-provider reject, not TWO — R9's own defect, one layer further out, still bound for ADR-0287**

SPEC §3.7 corrected "QUIC only" → "QUIC **and** `validate.Bootstrap`". Still short. RE-DERIVED — every path to `side == "downstream" && provider == nil`:

1. **`NewQUICDownstreamConfig`** — passes a literal `nil`.
2. **`validate.Bootstrap`/`BootstrapFile`** — `validate.go` passes `nil` to `boot.Construct` (the SPEC's R9 correction).
3. **`cmd/envoy-go/main.go`'s ORDINARY PRODUCTION PATH** — `boot.NewSDSProvider` returns **`nil, nil`** at `seen == 0`; `main.go` assigns it and passes it straight to `boot.Construct`. **Any bootstrap with no SDS-bound listener at all takes this path.**

**No typed-nil hazard** (checked explicitly — `reference_conn_wrap_method_no_promote`'s trap): `NewSDSProvider`'s signature returns the **interface** `xds.SecretProvider`, so `return nil, nil` yields a genuine nil interface and `provider == nil` is truly `true`.

Consumer (3) is not cosmetic — it is the mechanism behind C2's single-listener E2 analysis. §3.7, §6, §9-item-1, §11 item 7b and **§14's ADR-0287 §Context draft** all say "TWO". **Fix to THREE at the IMPL** (the ADR draft especially — it lands verbatim).

**⚠️ The EXACT wording to land (F8):** "**three production paths, plus two exported test-only constructors**" — `listener.NewManager` and `NewManagerWithBaseDir` are **exported** and pass a nil `sdsProvider`; only tests call them today, but they are public API. *"Three"* unqualified is what R9 was: a count that is right about what it counted and wrong about its scope. **State the scope.**

### C4 🟡 **The DECISIONS arithmetic is STALE — ADR-0288 landed after the SPEC, and ADR-0287 already exists**

SPEC §1 and §15 say *"tail ADR-0286 (next-free ADR-0287)"* and anticipate *"tail ADR-0286 → ADR-0287"* at the IMPL. **Both are now wrong.** Mechanically: tail is **ADR-0288**. Two things landed after the SPEC was drafted: its own commit anchored **ADR-0287** (§Context — exactly as §14 intends), and the STATE.md restore landed **ADR-0288**.

**⇒ At the phase-66 IMPL: the tail does NOT flip.** ADR-0287 is **already written**; the IMPL **completes it IN PLACE** with §Decision/§Consequences (ADR-0044's established pattern). Next-free stays **ADR-0289**. **The IMPL must NOT append a new ADR and must NOT re-number to 0288 (collision).**

### C5 🟢 §11 item 13 says the BEHAVIOR_CONTRACT delta is *"the §9 delta (**six** items)"*. §9 carries **SEVEN** — item 7 (the `validate`-path divergence) was added by R9 and never propagated to §11. Use **seven**, plus the C1 reconciliation ⇒ **eight** (D1).

### C6 ✅ **CONFIRMED EXACT — re-derived and, where executable, EXECUTED. Adopt as-is.**

- **The §3.2 equivalence theorem — SURVIVES a second adversarial attack; no counterexample.** P1 (`loadTrustedCAPool` touches `vc` exactly once), P3 (`dataSourceBytes`'s `default:` arm errors for both nil-`trusted_ca` and specifier-unset; getters are nil-receiver-safe) re-derived.
- **P5 — CONFIRMED, both halves, and as fragile as the SPEC says.** `main.go` passes **literally the same expression** `filepath.Dir(*cfgPath)` to `NewSDSProvider` and to `boot.Construct`. `internal/xds`'s `dataSourceBytes` and `internal/tls`'s `loadDataSource` are **arm-for-arm identical** — same four cases, `environment_variable` errors in both, `default:` errors in both; only the error-string prefix differs. **⇒ D5's apply-point comment is MANDATORY.**
- **A merge class the SPEC's pin block NEVER probed — now closed.** SPEC §3.3 rows 1–8 are all **cross-case** (`filename ⊕ inline_bytes`). If a same-case `bytes` oneof had **appended** rather than replaced, P2 — and the theorem — would be FALSE. EXECUTED: `inline_bytes(CA_Y) ⊕ inline_bytes(CA_X) → inline_bytes:"CA_X"`; `filename ⊕ filename → src`; `inline_string ⊕ inline_string → src`; `inline_bytes(CA_Y) ⊕ inline_bytes("") → inline_bytes:""`. **All REPLACE. No counterexample.** *(The theorem had been resting on an untested merge class — exactly the "a probe must discriminate" lesson, one level up: the pin block ran, but never ran the case that could have refuted it.)*
- **§5.2** — both CVC halves PGV-`required` (executed against the generated validator: `CVC{}` → `DefaultValidationContext: value is required`; `CVC{default:{}}` → `ValidationContextSdsSecretConfig: value is required`). **Zero** non-generated, non-test `Validate()`/`ValidateAll()` call sites **repo-wide**; `protoc-gen-validate` is `// indirect` and no `.go` file imports it.
- **§3.5** — the generated getter returns nil under the CVC arm (executed); the reject block is **exactly four**; `parseValidationSecret`'s served-half roster is the **identical four**.
- **§3.6** — one type-assert; silent `(nil, nil)` at `seen == 0`; `seen > 1` hard-fails. The SPEC's R10 correction is right: without the third arm it **boot-FAILs loudly**, not silently.
- **§5.3** — **15 fields exactly**, by reflection. Repeated = **exactly** f2/f3/f9/f15; plain bool = **exactly** f8/f14 (f10 is an enum; f6/f16 are messages). ⇒ **repeated-concat + bool-OR structurally unreachable — HOLDS.**
- **§3.9's envoy-go column** — all three shapes boot-FAIL, traced. (c) executed: `filepath.Join(baseDir, "")` → `baseDir` → `os.ReadFile` → `is a directory`.
- **§3.7's core** — `validate.BootstrapFile` on a plain TCP listener emits the CVC reject. The false landed comment is real and present; the contradicting correct one is real and present. §11 items 6/7 confirmed stale.

---

## PLAN-time design decisions

### D1 — `E3`: reject CVC + `require_client_certificate` false/absent, in `NewDownstreamConfig` *(resolves C1)*

**The apply-point is forced:** the field lives on `DownstreamTlsContext`, so **only `NewDownstreamConfig` can see it**. The reject goes there, immediately after `commonTLSContextToConfig` returns, before/at the require block.

```go
// C1/D1: the CVC arm's reject was lifted in commonTLSContextToConfig, which
// CANNOT see require_client_certificate (it lives on DownstreamTlsContext, not
// on CommonTlsContext). Without this arm a CVC listener with require==false
// would boot SUCCESSFULLY with ClientCAs nil and ClientAuth == NoClientCert —
// converting today's loud boot-FAIL into a silently unauthenticated listener.
// envoy-go-strict (ADR-0080): the reference HONORS the anchor here
// (verify-if-presented, PROBED at SPEC §3.10); envoy-go refuses LOUDLY rather
// than diverging silently.
if _, isCVC := ctx.GetCommonTlsContext().GetValidationContextType().(*tlsv3.CommonTlsContext_CombinedValidationContext); isCVC &&
    !ctx.GetRequireClientCertificate().GetValue() {
    return nil, fmt.Errorf("tls: downstream: combined_validation_context requires require_client_certificate: true in phase 03")
}
```

**Why REJECT and not honor.** Honoring means `ClientAuth = VerifyClientCertIfGiven` **plus** restructuring the fetch gate (the fetch currently lives *inside* the require block) — an apply-point reshape, and it would have to be done for **all three** paths at once (CVC, SDS-VC, inline) or leave two siblings divergent (SPEC §3.10's own reasoning). **Rejecting preserves today's behavior exactly, costs one arm, and makes the headline scope true.**

**Substring is ADR-0080-distinct and GREP-collision-checked** — see D2's table.

### D2 — E1/E2/E3 placement, substrings, and REACHABILITY *(resolves C2)*

**Placement: E1/E2 go INSIDE the CVC arm, AFTER the retained gate. E3 goes in `NewDownstreamConfig`.** Final shape of the CVC arm:

```go
case *tlsv3.CommonTlsContext_CombinedValidationContext:
    // Retained BYTE-IDENTICAL (ADR-0080). THREE live consumers (C3):
    // NewQUICDownstreamConfig (literal nil), validate.Bootstrap (nil), and
    // main.go's ordinary path when NewSDSProvider returns (nil,nil) at seen==0.
    // MUST precede E1/E2 or QUIC emits E2's message and the byte-identical
    // promise breaks (C2).
    if side != "downstream" || provider == nil {
        return nil, fmt.Errorf("tls: %s: combined_validation_context is not supported in phase 03", side)
    }
    cvc := c.GetCombinedValidationContext()
    if cvc.GetDefaultValidationContext() == nil {                       // E1
        return nil, fmt.Errorf("tls: %s: combined_validation_context.default_validation_context is required", side)
    }
    if cvc.GetValidationContextSdsSecretConfig() == nil {               // E2
        return nil, fmt.Errorf("tls: %s: combined_validation_context.validation_context_sds_secret_config is required", side)
    }
    // else: NO-OP. NewDownstreamConfig's require block does the work.
```

**Reachability — stated, not assumed:**

| Arm | Reachable when | Unreachable when |
|---|---|---|
| **E1** (no `default_validation_context`) | **Single listener suffices** — SDS half present ⇒ `seen==1` ⇒ provider live ⇒ gate passes ⇒ E1 fires. **A real silent-accept without it** (envoy-go would behave as plain SDS-VC on a shape the reference PGV-refuses). | — |
| **E2** (no `validation_context_sds_secret_config`) | **ONLY when ANOTHER listener supplies the global provider** (the pre-scan spans **all** listeners — C2). Listener A well-formed CVC ⇒ provider live; listener B pure-inline CVC ⇒ gate passes ⇒ E2 fires. **Without it, B reaches the CVC branch and fetches with an EMPTY secret name.** | **Single-listener pure-inline CVC** ⇒ `seen==0` ⇒ nil provider ⇒ the **retained** substring fires first. **A NAMED BOUNDARY:** the message is misleading (the feature *is* supported; the config is malformed), and that is accepted as the price of ADR-0080's byte-identical promise. |
| **E3** (CVC + require==false) | Always, once the gate passes (D1). | — |

**Substrings — all ADR-0080-distinct; GREP-collision-check each at the IMPL** (`reference_spec_drafted_identifier_collision_check`) with `grep -rn '<substring>' internal/ test/` and confirm **zero** pre-existing hits:

| id | Substring |
|---|---|
| retained | `combined_validation_context is not supported in phase 03` *(BYTE-IDENTICAL — do not touch)* |
| E1 | `combined_validation_context.default_validation_context is required` |
| E2 | `combined_validation_context.validation_context_sds_secret_config is required` |
| E3 | `combined_validation_context requires require_client_certificate: true in phase 03` |

All keep the `tls: ` prefix invariant that `FuzzTLSContextParse` enforces.

### D3 — the four re-pointed sub-field rejects: an `inlineVC` selector *(SPEC §3.5)*

The existing block is guarded by `if vc := c.GetValidationContext(); vc != nil` — **nil under CVC** (the oneof), so all four are **BYPASSED**. Re-point by selecting the effective inline context:

```go
// SPEC §3.5: under a CVC the oneof makes GetValidationContext() nil, so the
// four rejects below were BYPASSED — lifting the envelope without this would
// SILENTLY ACCEPT sub-fields envoy-go cannot honor
// (reference_strict_reject_sibling_typeurl_gap).
inlineVC := c.GetValidationContext()
if inlineVC == nil {
    if cvc := c.GetCombinedValidationContext(); cvc != nil {
        inlineVC = cvc.GetDefaultValidationContext()
    }
}
if inlineVC != nil {
    ... the four EXISTING rejects, unchanged, on inlineVC ...
}
```

**The four error strings stay BYTE-IDENTICAL** — they are shared with the inline path and are not this row's to change. **Each of the four needs its OWN test** (`reference_fatalf_makes_assertions_unreachable`: `Errorf` per property), and **each must be shown RED first** — they pass today for the wrong reason (the block is skipped), so **a green test proves nothing** (T2).

### D4 — fixture `0109`: clone `0108`, re-label the CAs; the empty-dynamic arm is SUBJECT-SIDE ONLY

**`0109-xds-sds-combined-validation-context`** = `0108`'s driver with a **CA re-labelling** and a YAML delta. `0108`'s in-memory 5-artifact PKI maps almost 1:1:

| `0108` | `0109` | Role |
|---|---|---|
| `CA_served` | **CA_X** — served over SDS | the anchor that MUST win |
| `CA_unserved` | **CA_Y** — inline `default_validation_context.trusted_ca` | the anchor that MUST LOSE (proves REPLACE, not union) |
| `client_good` | **client_X** (signed by CA_X) | **ACCEPTED** |
| `client_bad` | **client_Y** (signed by CA_Y) | **REJECTED** |
| server leaf | unchanged (signed by CA_X) | — |

**The delta from `0108` is that CA_Y is now CONFIGURED (as the inline default) rather than never delivered.** That is the whole point: `0108` proves the served CA is the anchor; `0109` proves the served CA **beats a configured competitor**.

**⚠️ This observable is ALREADY OBSERVED cross-side-satisfiable** — the SPEC's live probe produced exactly `client_x ACCEPTED / client_y rejected UNKNOWN_CA` on the reference. It is simultaneously the assertion that **refutes Design C (pool union)**, which would accept **both**. The fixture is load-bearing for the *design*, not just the code.

**`structuralCheck` is MANDATORY and its necessity must be RE-DEMONSTRATED, not asserted** (T6). Phase 65 **proved** on `0108` that with it disabled a served-CA break ships **PASS** — both sides emit `good=REJECTED`/`bad=ACCEPTED` and `CompareBytes` compares **EQUAL**. Any CVC break changes **both sides identically** ⇒ same trap (`reference_vacuous_break_receiver_normalizes`).

**The empty-dynamic departure (SPEC §3.9) must NOT appear on the cross-side path.** The sides **disagree** there — the reference serves traffic against the default CA; envoy-go boot-FAILs. A cross-side arm would **fail legitimately** (`reference_differential_fixture_dispatch_constraint`: one fixture dir = ONE runner branch). **It is a subject-side unit test in T3.**

`BackendCount` **≥1** (`reference_differential_backendcount_min_one`) — `0108` returns 1 and the positive arm genuinely echoes through it; keep that.

### D5 — the apply-point: NO merge, and a MANDATORY P5 comment

The CVC branch in `NewDownstreamConfig`'s require block:

```go
if cvc, ok := common.GetValidationContextType().(*tlsv3.CommonTlsContext_CombinedValidationContext); ok {
    // SPEC §3.2 — the EQUIVALENCE THEOREM. envoy-go does NOT implement
    // Message::MergeFrom() for CVC and MUST NOT: FetchInitialValidationContext
    // returns an *x509.CertPool, so the served CertificateValidationContext
    // message never crosses the internal/xds seam and proto.Merge has no
    // operand (SPEC §3.1). Instead the SDS-delivered pool wins outright and
    // default_validation_context.trusted_ca is NOT read — provably identical to
    // a true merge on envoy-go's honored surface, because (P1) only trusted_ca
    // is honored, (P2) the DataSource specifier oneof REPLACES, and (P3) a
    // successful fetch GUARANTEES that specifier is set (dataSourceBytes errors
    // otherwise), so the default can never contribute.
    //
    // ⚠️ P5 — this rests on a COINCIDENCE, not a guarantee: internal/xds's
    // dataSourceBytes and internal/tls's loadDataSource are arm-for-arm
    // identical, and main.go passes the SAME filepath.Dir(*cfgPath) as baseDir
    // to BOTH NewSDSProvider and boot.Construct. If either ever diverges, this
    // equivalence is SILENTLY falsified and CVC goes wrong with no test to
    // catch it.
    name := cvc.CombinedValidationContext.GetValidationContextSdsSecretConfig().GetName()
    pool, err := provider.FetchInitialValidationContext(ctx2, name)
    ...
    cfg.ClientCAs = pool
    cfg.ClientAuth = stdtls.RequireAndVerifyClientCert
}
```

**⚠️ D5 CORRECTED (F4) — the first draft of this PLAN wrote `…GetValidationContextSdsSecretConfig().GetName()` directly. That is WRONG in three ways, each of which the phase-65 sibling right beside it already gets right.** RE-DERIVED from the landed SDS-VC branch:

1. **It skips `xds.ParseSDSConfig` entirely.** The sibling wraps the singular config in a list (`ParseSDSConfig` enforces `len == 1`) and calls it — which validates **`name`, `sds_config`, `api_config_source`, `resource_api_version: V3`, `api_type: GRPC`, `envoy_grpc`, and `cluster_name`**. A bare `GetName()` validates **nothing**. ⇒ **A CVC carrying a malformed `sds_config` (non-V3, missing `cluster_name`, …) would be SILENTLY ACCEPTED where the SDS-VC sibling REJECTS it.** That is `reference_strict_reject_sibling_typeurl_gap` **again — this time in this PLAN's own apply-point.** Not defense-in-depth: a real validation gap.
2. **It breaks the `tls: ` prefix invariant.** `ParseSDSConfig` returns `xds:`-prefixed errors; the sibling wraps them (`fmt.Errorf("tls: downstream: %w", err)`) precisely to preserve the invariant **`FuzzTLSContextParse` enforces** — the fuzzer T5 seeds. An unwrapped `xds:` error escaping here would fail the fuzz invariant.
3. **It drops the nil-provider guard.** The sibling keeps an explicit `if provider == nil` arm whose landed comment says it is unreachable today but kept *"so a future caller that relaxes that reject cannot nil-deref here."* In D5's branch a nil provider is a **panic**, not an error. **This PLAN asserted safety via exactly the argument the sibling deliberately declined to trust.**

**⇒ MIRROR THE SIBLING EXACTLY:**

```go
    if provider == nil {
        // Defense-in-depth, mirroring the phase-65 SDS-VC arm: the CVC arm in
        // commonTLSContextToConfig already refuses the nil-provider shape, so
        // this is UNREACHABLE from today's entry points. Kept so a future caller
        // that relaxes that reject cannot nil-deref here.
        return nil, fmt.Errorf("tls: downstream: combined_validation_context requires a live SDS provider (unavailable in this mode)")
    }
    // ParseSDSConfig takes a LIST (it enforces len==1); the CVC's
    // validation_context_sds_secret_config is SINGULAR, so wrap it. This is what
    // validates sds_config/api_config_source/V3/GRPC/cluster_name — a bare
    // GetName() would silently accept a malformed sds_config the SDS-VC sibling
    // rejects (F4).
    secretName, _, _, err := xds.ParseSDSConfig([]*tlsv3.SdsSecretConfig{cvc.CombinedValidationContext.GetValidationContextSdsSecretConfig()})
    if err != nil {
        return nil, fmt.Errorf("tls: downstream: %w", err) // xds: -prefixed -> preserve the `tls: ` invariant (FuzzTLSContextParse)
    }
    pool, err := provider.FetchInitialValidationContext(context.Background(), secretName)
    ...
```

**Safe to dereference:** reaching this branch implies the gate passed (provider non-nil) **and** E1/E2 passed (both halves non-nil) — `commonTLSContextToConfig` always runs first and its error is propagated before the require block (**re-derived: `NewDownstreamConfig` calls it and returns on error, before the require block**). **T3 asserts that ordering directly** so a future refactor cannot silently break the precondition — **and the nil guard above means that even if it did, the result is an error, not a panic.**

### D6 — `seen == 1` preserved; the compose-two edge stays shut

The third arm fires `seen++` **once** for a CVC (one `SdsSecretConfig`). The `seen > 1` guard is **untouched** and the deferred compose-two edge stays deferred. **T4 asserts `seen == 1` explicitly.**

---

## File Structure

```
internal/tls/config.go          [EDIT]  T1 (arm+gate+E1/E2), T2 (re-point), T3 (branch+E3), T8 (stale comments)
internal/tls/config_test.go     [EDIT]  T1, T2, T3
internal/boot/boot.go           [EDIT]  T4 (pre-scan third arm)
internal/boot/boot_test.go      [EDIT]  T4
internal/tls/fuzz_test.go       [EDIT]  T5 (SEEDS only — count STAYS 55)
test/fixtures/0109-.../         [ADD]   T6 (driver/, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md)
docs/envoy-go/BEHAVIOR_CONTRACT.md [EDIT] T7 (EIGHT items — C5 + D1)
internal/xds/**                 [UNTOUCHED — the row's defining property]
test/helpers/sdsserver/**       [UNTOUCHED]
validate/**                     [UNTOUCHED — decision recorded, T7 boundary]
```

---

## Task 1 — `internal/tls/config.go`: lift the CVC reject to a gated no-op + E1/E2 + **E3**

**⚠️ E3 LANDS HERE, NOT IN T3 (F5).** The first draft put the lift in T1 and E3 in T3. **Subagents auto-commit per task** (`feedback_subagent_autocommit_claudemd`), so between those two commits the tree would contain **exactly the silently-unauthenticated listener C1 describes — and T1's own green gate would CERTIFY it.** The squash means nothing ships, but a per-task-green tree that is silently auth-bypassed is not a state this row should ever pass through. **The lift and its guard are ONE atomic task.**

**Edit:** replace the unconditional CVC reject with D2's exact shape (gate → E1 → E2 → no-op), **and** add E3 to `NewDownstreamConfig` (D1).

**Tests (`internal/tls/config_test.go`), each an independent `Errorf` property:**

1. `TestCVC_DownstreamWithProvider_Accepted` — **⚠️ CORRECTED (F3): drive `commonTLSContextToConfig` DIRECTLY; do NOT drive `NewDownstreamConfig`, and do NOT set `require: true`.** As first written this test was **impossible**: through `NewDownstreamConfig` at T1 (T3's branch not yet landed), `require: true` falls into the **`else`** arm, where `common.GetValidationContext()` is **nil** under the CVC oneof ⇒ it errors `require_client_certificate=true requires validation_context.trusted_ca` — RE-DERIVED from the landed source. And `require` is not a parameter of `commonTLSContextToConfig` at all, so asserting on it there is meaningless. **The test: `commonTLSContextToConfig(ctc, baseDir, "downstream", fakeProvider)` with a well-formed CVC ⇒ no error.** It **must** supply `tls_certificates`, or it errors `no tls_certificates configured` and proves nothing about the CVC arm.
2. `TestCVC_NilProvider_KeepsByteIdenticalReject` — CVC + `provider == nil`, `side == "downstream"` ⇒ error contains **exactly** `combined_validation_context is not supported in phase 03`. **This is the QUIC/validate/no-SDS consumer (C3) — assert the substring BYTE-IDENTICALLY.**
3. `TestCVC_Upstream_KeepsByteIdenticalReject` — `side == "upstream"` ⇒ same substring. *(Dead from today's entry points — SPEC §3.7 — but the guard is retained deliberately; the test pins the guard, not a live path. **Label it as such in a comment** so a future reader does not mistake it for evidence the upstream arm is reachable.)*
4. `TestCVC_MissingDefaultValidationContext_E1` ⇒ E1's substring.
5. `TestCVC_MissingSDSSecretConfig_E2` ⇒ E2's substring.
6. `TestCVC_E1E2_DoNotPreemptTheRetainedReject` — **the C2 regression guard**: E1-shaped and E2-shaped CVCs with `provider == nil` ⇒ the **retained** substring, **NOT** E1/E2's. *(This is the test that keeps ADR-0080's byte-identical promise honest.)*
7. **`TestCVC_RequireFalse_Rejected_E3`** and **`TestCVC_RequireFalse_NeverYieldsNoClientCert`** — **C1's guards, moved here from T3 (F5).** Both sub-cases: `require: false` **and** `require:` **absent** (nil `BoolValue` — different protos, both must be pinned; `GetValue()` on a nil `*wrapperspb.BoolValue` is nil-receiver-safe and returns `false`, **verified by execution**). The second test states the **property** rather than the mechanism: `NewDownstreamConfig` **returns an error**; it must **never** return a `cfg` with `ClientAuth == NoClientCert`.

**Breaks (COMPILING + DISCRIMINATING; `-count=1`):**
- Delete the `provider == nil` conjunct ⇒ **(2) AND (6) must BOTH fail** — **⚠️ CORRECTED (F6): the first draft named only (2).** With the conjunct gone, (2)'s nil-provider CVC no-ops instead of rejecting, **and** (6)'s E1/E2-shaped nil-provider CVCs now reach E1/E2 instead of the retained reject. **Naming one and finding two is `reference_deliberate_break_wrong_assertion` — this PLAN's own bar.** Confirm **both** fired; if only one does, the tests are not independent — a finding.
- Move E1 **above** the gate ⇒ (6) must fail with E1's substring where the retained one was expected. **This break is the whole point of C2** — if (6) does not fire, the ADR-0080 promise is unproven and that is a **FINDING**.
- Change E1's substring by one character ⇒ (4) fails; (5)/(6) must NOT. If (5) also fails, the arms are not independent — a finding.
- **Delete E3** ⇒ (7)'s **both** tests must fail. **Discriminates?** Yes — without E3, CVC+require=false returns a nil error and a `cfg`, so both the substring assertion and the `NoClientCert` property fail. **If only the substring test fires, the security property is not independently proven — a finding.**

**Verify:** `gofmt -l internal/tls` silent · `go vet ./internal/tls/` · `golangci-lint run ./internal/tls/` · `go test ./internal/tls/ -count=1`.

---

## Task 2 — `internal/tls/config.go`: re-point the four sub-field rejects at `default_validation_context`

**Edit:** D3's `inlineVC` selector. **The four error strings stay BYTE-IDENTICAL.**

**⚠️ RED-FIRST IS MANDATORY AND IS THIS TASK'S ENTIRE POINT — AND THE FIRST DRAFT OF THIS PLAN SPECIFIED A *VACUOUS* RED (F2).** It said *"run it against the UNMODIFIED `config.go` and record that it FAILS (the shape is accepted)."* **That red would have been the WRONG ASSERTION FIRING.** Against master-unmodified the shape is **not** accepted — the **unconditional CVC reject** (C1 leg ii) fires first, so the test reds with `combined_validation_context is not supported in phase 03`. It goes red, it looks like proof, and it proves **nothing** — `reference_deliberate_break_wrong_assertion`, committed by this PLAN inside the very task whose purpose is proving four assertions live. **An IMPL agent taking "UNMODIFIED" at face value would record four vacuous REDs and believe the tests are proven.**

**THE RED WINDOW IS `post-T1, pre-T2` — not "unmodified".** Only after T1 lifts the reject to a gated no-op does a CVC with a rejected sub-field actually reach (and pass) the bypassed block.

**PROTOCOL, precisely:**
1. Land T1 (the arm is now a no-op for downstream+provider).
2. Write the four tests. Run them. **Each MUST fail with `expected <sub-field> is not supported…; got nil error`.**
3. **CONFIRM THE FAILURE MESSAGE.** If any test reds with `combined_validation_context is not supported in phase 03`, the red is **VACUOUS** — T1 did not land, or the test does not reach the arm. **STOP; do not proceed.**
4. Only then make the D3 edit ⇒ all four go green.
5. Record all four **verified-non-vacuous** REDs in PROGRESS, quoting the observed `got nil error` message.

If any test is **green** at step 2, **stop** — it does not exercise CVC.

**Tests — FOUR separate ones** (`reference_fatalf_makes_assertions_unreachable` — `Errorf` per property, never one test with four `Fatalf`s):

- `TestCVC_DefaultVC_CustomValidatorConfig_Rejected` ⇒ `custom_validator_config is not supported in phase 03`
- `TestCVC_DefaultVC_MatchTypedSAN_Rejected` ⇒ `match_typed_subject_alt_names is not supported in phase 03`
- `TestCVC_DefaultVC_VerifyCertHash_Rejected` ⇒ `verify_certificate_hash is not supported in phase 03`
- `TestCVC_DefaultVC_VerifyCertSpki_Rejected` ⇒ `verify_certificate_spki is not supported in phase 03`

Plus `TestInlineVC_FourRejects_Unchanged` — the **plain** `validation_context` path still rejects all four (**no regression** from the selector rewrite).

**Break:** revert `inlineVC` to `c.GetValidationContext()` only ⇒ **all four CVC tests must fail** and the inline test must **still pass**. **Discriminates?** Yes — it isolates the CVC half from the inline half.

---

## Task 3 — `internal/tls/config.go`: the `NewDownstreamConfig` 3-way branch *(the apply-point)*

**Edit:** turn the require block's if/else type-assert into a 3-way branch (SDS-VC / **CVC** / inline) per **D5 as CORRECTED** — nil-provider guard + `xds.ParseSDSConfig` + `tls: `-wrap + `FetchInitialValidationContext` → `ClientCAs`. **NO `proto.Merge`. NO `proto.Clone`. The P5 comment is MANDATORY.** *(E3 moved to T1 — F5.)*

**Tests:**

1. `TestCVC_RequireTrue_InstallsSDSPoolAsClientCAs` — fake provider returns a known pool ⇒ `cfg.ClientCAs` is **that pool** and `cfg.ClientAuth == RequireAndVerifyClientCert`.
2. `TestCVC_ServedPoolWins_DefaultTrustedCaNotRead` — **the theorem's observable at the unit level**: served CA_X + inline default CA_Y ⇒ the resulting pool contains **X and NOT Y**. **Discriminating**: a pool *union* (Design C) would contain **both** — so this test **refutes Design C** and is not merely a happy path.
3. `TestCVC_MalformedSDSConfig_Rejected` — **F4's guard.** A CVC whose `validation_context_sds_secret_config` carries a **malformed `sds_config`** (e.g. missing `cluster_name`, or non-V3 `resource_api_version`) ⇒ **rejected**, with a **`tls: `-prefixed** error. **Two properties, two `Errorf`s:** (i) it rejects at all — without `ParseSDSConfig` a bare `GetName()` would **silently accept** it; (ii) the error keeps the **`tls: ` prefix** the fuzzer enforces (an unwrapped `xds:` error fails T5's invariant).
4. `TestCVC_GateRunsBeforeRequireBlock` — **D5's precondition**, asserted directly: CVC + `provider == nil` + `require: true` ⇒ the **retained** substring (proving `commonTLSContextToConfig`'s error is propagated **before** the require block, so the branch's dereferences are safe).
5. `TestCVC_EmptyDynamicVC_BootFails` — **SPEC §3.9's departure, SUBJECT-SIDE ONLY** (never cross-side — D4). A fake provider whose fetch errors (as `parseValidationSecret` does for a served VC with no usable `trusted_ca`) ⇒ `NewDownstreamConfig` returns that error. **Comment it with the departure**: the reference ACKs, falls back to the **default CA**, and **SERVES**; envoy-go boot-FAILs (ADR-0280 family). **Never phrase it "envoy-go rejects where the reference rejects" — FALSE for this shape.**
6. `TestSDSVC_And_Inline_Paths_Unchanged` — no regression on the two landed branches.

**Breaks:**
- Make the CVC branch read `cvc.GetDefaultValidationContext().GetTrustedCa()` and **union** it into the pool ⇒ **(2) must fail** (the pool would contain Y). **This break is Design C, EXECUTED** — if (2) does not fire, the theorem's observable is unproven and that is a **FINDING**.
- Replace `ParseSDSConfig` with a bare `GetName()` ⇒ **(3) must fail on BOTH properties.** If only (i) fires, the `tls: `-prefix property is not independently proven — a finding.
- **Do NOT break the nil-provider guard to "prove" it** — it is UNREACHABLE by construction (T1's gate rejects first), so **no break can fire it**. That is not a gap; it is defense-in-depth mirroring the landed phase-65 arm. **Record it as deliberately unproven** rather than manufacturing a test that reaches it by bypassing the gate.

---

## Task 4 — `internal/boot/boot.go`: the pre-scan third arm

**Edit:** add the CVC arm beside the existing `*tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig` assert:

```go
if cvc, ok := ctc.GetValidationContextType().(*tlsv3.CommonTlsContext_CombinedValidationContext); ok {
    // E2's condition, honored here too (D2): a pure-inline CVC contributes NO
    // secret. Skipping (rather than counting) keeps seen==0 for the
    // single-listener case, so the retained reject fires in tls/config.go
    // rather than ParseSDSConfig tripping on a nil entry.
    if sc := cvc.CombinedValidationContext.GetValidationContextSdsSecretConfig(); sc != nil {
        seen++
        found = []*tlsv3.SdsSecretConfig{sc}
    }
}
```

**Tests:** third arm detected (`seen==1`, provider non-nil) · **`seen == 1` asserted explicitly** (D6 — the compose-two edge stays shut) · pure-inline CVC ⇒ `seen==0` ⇒ **`(nil, nil)`**, no error, no panic · CVC + cert-SDS on one context ⇒ **`seen > 1`** ⇒ the existing hard-fail, **unchanged** · the landed plain-VC arm unchanged.

**⚠️ Do NOT "fix" the landed plain-VC arm's missing nil-check** (SPEC §3.6's recorded out-of-scope note). It cannot panic (`ParseSDSConfig` + nil-receiver-safe getters yield a misleading-but-safe error). Touching it widens the row.

**Break:** delete the third arm ⇒ the detection test must fail. **Confirm the failure is `provider == nil`, NOT a panic** — and note it manifests as the **loud boot-FAIL with the misleading retained message** (SPEC §3.6's R10 correction), which is a second, cheap confirmation of that correction.

---

## Task 5 — fuzz SEEDS *(count STAYS 55)*

Add seeds to **`FuzzTLSContextParse`** — **NO new fuzzer** (`reference_fuzzer_count_docs_drift`: reconcile the documented total against actual `^func Fuzz` before touching it).

Seeds: a well-formed CVC · E1's shape · E2's shape · E3's shape (require false) · a CVC whose `default_validation_context` carries each of the four rejected sub-fields · a pure-inline CVC.

**Verify the invariant holds:** every new branch's error keeps the `tls: ` prefix the fuzzer enforces. **Re-verify the count is still 55** with the canonical command — do not assume.

---

## Task 6 — fixture `0109-xds-sds-combined-validation-context` *(fixtures 110 → 111)*

Clone `0108`'s `driver/` per **D4**; re-label the CAs; YAML delta = swap `validation_context_sds_secret_config` for `combined_validation_context { default_validation_context { trusted_ca { inline_string: CA_Y } }, validation_context_sds_secret_config { … } }` on **both** sides. `require_client_certificate: true` (D1 makes this mandatory, not incidental). `sdsserver` **UNTOUCHED** — it already serves exactly what CVC needs, and **the served wire is byte-identical to `0108`'s** (the management server cannot tell a CVC client from a plain-SDS client).

**Observable:** `good=ok echo=phase66-cvc-probe\nbad=rejected\n` per side — `0108`'s two-arm shape, **zero new observable machinery**.

**⚠️ MANDATORY liveness DEMONSTRATION (not an assertion) — the task is not done without it:**
1. Disable `structuralCheck`; break the served CA (serve CA_Y instead of CA_X) ⇒ **observe the fixture ship PASS** (both sides flip identically; `CompareBytes` compares EQUAL). **Record the PASS in PROGRESS.** This is `reference_vacuous_break_receiver_normalizes` demonstrated on `0109`.
2. Restore `structuralCheck` (**`git restore` only**) ⇒ the same break now **FAILS**. Confirm it is `structuralCheck` that fired.
3. Prove `CompareBytes` is live independently: an **asymmetric** break (one side only) with `structuralCheck` disabled ⇒ mismatch. Record the byte offset.

**Harness trap the SPEC's own probe hit and this fixture inherits (SPEC §8):** a **stale SDS server** silently served the **previous arm's** config and nearly produced a false divergence — caught only because that arm's server log was empty (`bind: address already in use`). **Fresh-container-per-arm is NOT sufficient** (`reference_probe_fresh_container_per_arm` covers only the container). `0108` already uses **per-side** servers on separately-allocated ports; **confirm they cannot outlive an arm**, and prefer a hard precondition assert that *this* run's server actually served.

Also honor: `reference_differential_grpc_receiver_driver_owned` (the SDS receiver is a `test/helpers` server, **not** a `BackendKind`) · `reference_differential_backendcount_min_one` · `reference_differential_run_selector` · `reference_envoy_contrib_image_tagging` · `reference_host_gateway_ip_docker_desktop`.

`expectations.yaml` + `README.md`: clone `0108`'s (they are excellent) and state the **new** proposition — *"the SDS-delivered CA REPLACES the inline default; it does not union with it"* — plus the §3.9 departure and the §3.10/D1 boundary as **UNasserted named boundaries**.

---

## Task 7 — `BEHAVIOR_CONTRACT.md`: REJECTED → CONSUMED, **EIGHT** items *(C5 + D1)*

CVC moves **REJECTED → CONSUMED** (downstream + live provider + `require_client_certificate: true`). Record, each as a NAMED BOUNDARY:

1. The upstream + nil-provider reject — byte-identical substring; **THREE** live consumers (QUIC, `validate`, `main.go`@`seen==0`) — **not two** (C3).
2. **envoy-go does NOT implement `Message::MergeFrom()` for CVC** — a provably-equivalent substitution on the `trusted_ca`-only surface (SPEC §3.2/§5.3); only **1 of 3** documented rules is reachable; repeated-concat + bool-OR **structurally unreachable** (15-field roster, C6).
3. **The empty-dynamic DEPARTURE** (SPEC §3.9(a)) — the reference ACKs, falls back to the **default CA**, and **SERVES**; envoy-go **boot-FAILs** (ADR-0280 family). **Per-shape** phrasing: (b)/(c) agree on traffic, diverge on lifecycle.
4. **`require_client_certificate: false` — envoy-go REJECTS (E3, C1/D1).** The reference **honors** the anchor (verify-if-presented, PROBED). **A LOUD, named divergence — deliberately not a silent one.** *(Rewrite of SPEC §9 item 4, which had it as a silent-ignore.)*
5. **E1/E2** — envoy-go rejects the two PGV-`required` violations explicitly, because envoy-go runs **no PGV repo-wide** (C6). **With E2's reachability boundary** (single-listener pure-inline CVC gets the retained message instead — D2).
6. **SHARED gaps, unchanged and now fully enumerated** (C6's 15-field roster): `crl`, `trust_chain_verification` (incl. **`ACCEPT_UNTRUSTED`**), `system_root_certs`, `max_verify_depth`, `ca_certificate_provider_instance`, `match_subject_alt_names`. Rejecting any on the CVC path alone would introduce a **NEW asymmetry**.
7. **The `validate` path diverges** (SPEC §3.7/§9 item 7) — the reference's `--mode validate` **ACCEPTS** a well-formed CVC config; envoy-go's `validate.BootstrapFile` **REJECTS** it, having no provider. **Decision recorded: leave it** (plumbing SDS into a config-validator implies dialing a management server from `--mode validate`, which the reference does not do either). `validate/` is **UNTOUCHED**.
8. **DISCOVERED, NOT FIXED — TWO pre-existing siblings of C1's defect class** (C1): the **phase-65 SDS-VC path** (SDS-VC + `require==false` silently ignores the anchor where pre-65 it boot-failed; **ADR-0286 shipped it**), and **`NewQUICDownstreamConfig`, which never evaluates `require_client_certificate` at all** (QUIC + inline VC + `require: true` silently installs **no** `ClientCAs` today — F7). **Pre-existing defects this row discovered and deliberately did not widen its scope to fix.** Carry to §Deferred.
9. **UNasserted — the CVC envelope's f3/f4** (`validation_context_certificate_provider`, `…_instance`; both `[#not-implemented-hide:]` **and** deprecated). envoy-go **silently ignores** them: a PGV-valid CVC with f1+f2+f4 is accepted and SDS is used (C1b). **The reference's behavior on them is UNPROBED** — recorded as UNKNOWN, not guessed. §Deferred carries the probe obligation. *(SPEC §5.1's "out of scope by construction" was a silence; this names it.)*
10. **UNasserted — the CVC default's `trusted_ca` is NEVER READ post-lift** (a direct corollary of the theorem, D5). So a **nonexistent or garbage** `default_validation_context.trusted_ca` **boots green** on envoy-go, where the reference would resolve it. Consistent with the adopted design and harmless given E1 requires the field's *presence* — but it is a real, previously unnamed asymmetry. *(Found by the adversarial pass; named here rather than discovered at the IMPL.)*

---

## Task 8 — the THREE stale/false code comments *(SPEC §11 items 6/7/7b)*

1. `NewDownstreamConfig`'s doc block — *"the previously parse-rejected surfaces (**SDS-bound secrets**, …) remain rejected"*. **False since phase 60.2/65.**
2. `commonTLSContextToConfig`'s doc list — *"validation_context_sds_secret_config set"* / *"combined_validation_context set"* listed flatly as forbidden. Half-false today; **fully false after this row**.
3. **`commonTLSContextToConfig`'s SDS-arm comment — *"The guard below has exactly ONE live consumer: NewQUICDownstreamConfig"*. FALSE — THREE (C3).** **This comment took the SPEC in** and produced its one severe defect; a *third* comment in the same file (*"nil for upstream/**validate** callers"*) contradicts it and is correct. **Fix the false one; keep the correct one.**

*(Out of scope, recorded: `parseValidationSecret`'s doc cites a moved line range. Fold in only if free — it is `internal/xds`, and the row's defining property is that the package is untouched. **Prefer leaving it.**)*

---

## Task 9 — verify + complete ADR-0287 + STATE + ROADMAP + PROGRESS + router roll

**The six-gate, controller-run on the FROZEN HEAD:**
1. `gofmt -l internal/ test/ cmd/` — **SILENT**
2. `go vet ./...` — exit 0
3. `go build ./...` — exit 0
4. `go mod tidy -diff` **EMPTY** + `git diff --exit-code master -- go.mod go.sum` **EMPTY** *(modules **+0**; `reference_new_subpackage_pulls_transitive_module` — re-check `git diff go.mod` after tidy, do not assume)*
5. `golangci-lint run ./...` — exit 0
6. **The FULL 111-dir differential**: `go test ./test/differential/ -count=1` — exit 0. *(The 110 pre-existing dirs are byte-stable: the row LIFTS a reject and cannot change any passing fixture's bytes. `reference_differential_fullsuite_startup_flake` — a `subject ready: EOF` on an UNRELATED fixture is a startup race; isolate-re-run to discriminate, do not classify as a regression.)*

**Plus:**
- **Cycle guard:** `go list -deps ./internal/xds | grep 'envoy-go/internal'` (**no `...`**) ⇒ **`internal/stats` + `internal/xds` ONLY**; `internal/tls` must **NOT** appear. *(Trivially true — `internal/xds` is untouched — but assert it.)*
- **`-race` on the emitting packages:** `go test ./internal/tls/ ./internal/boot/ -race -count=1` (`reference_full_suite_race_after_background_mutator`; and the `reference_sds_init_fetch_timeout_dial_budget_flake` caveat above).
- **Counts re-verified MECHANICALLY**, not copied: fixtures **111** · fuzzers **55** · go.mod **+0**.
- **ADR-0287: COMPLETE IT IN PLACE** — append §Decision + §Consequences to the **existing** ADR-0287 entry (ADR-0044). **Do NOT append a new ADR. Do NOT re-number to 0288 (collision — C4).** Tail stays **ADR-0288**; next-free stays **ADR-0289**. **§Context must be reconciled with C1/C3** before landing: it currently states the unenforceable `require==true` scope and says "TWO live consumers".
- **ROADMAP row 66 → `done`** (ADR-0106, the SOLE leg — `reference_roadmap_split_phase_row_done`). **The deferred sentence is UNCHANGED** — `combined_validation_context` was never in one (SPEC §12; the phase-64 precedent). **Do NOT fabricate a narrow.**
- **STATE.md:** **EDIT §Current pointer IN PLACE — do NOT prepend** (ADR-0288). Demote the outgoing bullet into §Recent lineage and **cap that list at FIVE** (move the sixth to `STATE_HISTORY.md`). Update the counts block.
- **PROGRESS.md:** the liveness-break log — including **every break that did NOT fire** (a finding, not a footnote), the four **RED-first** results from T2, T6's **demonstrated** structuralCheck PASS, and any substituted break instruction.
- **Router roll** (`next-prompt.txt` — **TRACKED**; fold into the squash; locate commits by **SUBJECT** via `git log --grep`, never by position).
- **Sentinel:** re-run **MECHANICALLY**. Check (1) goes silent when row 66 flips `done`; checks (2)+(3) still print ⇒ **the sentinel does NOT fire; do NOT create `stop`.**
- **Memory updates owed** (SPEC §13, + this PLAN's): the five SPEC-listed ones, **plus** — *a landed code comment is not evidence* is now **twice** demonstrated in one row (it took the SPEC in at §3.7 **and** the false comment is still on disk); and **C1's shape: when a row lifts a reject, ask what ELSE that reject was silently enforcing** — here it was enforcing mTLS-actually-on.

---

## Self-review against SPEC-66

| SPEC obligation | Where |
|---|---|
| Design A; no `proto.Merge`/`proto.Clone` | Global Constraints, D5, T3 |
| The equivalence theorem's observable proven | T3 test 2 (**refutes Design C**), T6 |
| **P5** comment at the apply-point | D5, T3 |
| Four BYPASSED rejects, **RED first** (window: **post-T1, pre-T2** — F2), each its own test | **T2** |
| E1/E2 with distinct, collision-checked substrings | D2, T1 |
| `provider != nil` conjunct retained | D2, T1 tests 2/6 |
| Departure phrased **per-shape**, subject-side only | D4, T3 test 5, T7 item 3 |
| `structuralCheck` **re-DEMONSTRATED** | **T6** |
| Fixture 110 → 111; `BackendCount` ≥1 | T6 |
| +0 stats / +0 fuzzers / +0 modules / ZERO new xds symbols | Global, T5, T9 |
| ADR-0287 §Decision/§Consequences at the IMPL | T9 (**in place** — C4) |
| Row 66 → `done` at the six-gate; deferred sentence UNCHANGED | T9 |
| **PLAN-added:** E3 guards C1 **atomically with the lift** (F5) | **T1** |
| **PLAN-added:** `ParseSDSConfig` + `tls: `-wrap + nil guard (F4) | D5, T3 test 3 |
| **PLAN-added:** f3/f4 + never-read-default named as boundaries (C1b, F1) | T7 items 9/10 |

**Task count: 9** — inside the SPEC's ~8–10 anticipation. **ADR-0045 escape-valve ARMABLE but UNCONSUMED: NO split.** There is no two-package surface that could strand a leg (`internal/xds` untouched), and Design A **shrank** the row (the clone+merge and its bootstrap-mutation test are gone). T1–T3 are sequential on one file; T4–T8 are independent.

**⚠️ The IMPL's standing instruction: a PLAN is not evidence either.** Phase 65's IMPL found **ELEVEN** defects in a PLAN that had itself corrected five SPEC defects. This PLAN corrected **five** SPEC defects — one severe and security-relevant — and the SPEC had already self-corrected four. **The rate is not falling. RE-DERIVE this document; do not execute it. Default to REFUTED.** Start where this PLAN is most confident: **C1's fix** (is E3 in the right scope? does it cover `absent` as well as `false`?), **D2's reachability table** (is E2 *really* reachable only multi-listener?), and **C6's "confirmed exact" list** — that is precisely where the SPEC's severe defect hid.
